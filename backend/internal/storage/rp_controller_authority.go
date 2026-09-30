package storage

import (
	"context"
	"database/sql"
	"errors"

	"corerp.local/backend/internal/core"
)

// Assignment may join one already-committed heard turn, but it cannot
// displace a live RP session controlling the target or a pending wait.
type RPExternalControllerAssignmentRequest struct {
	Binding            core.CareerBinding `json:"binding"`
	EntityID           string             `json:"entity_id"`
	ExpectedGeneration int64              `json:"expected_generation"`
}

type RPExternalControllerAssignmentFact struct {
	Version               string `json:"version"`
	EntityID              string `json:"entity_id"`
	ControllerPrincipalID string `json:"controller_principal_id"`
	ControllerInstanceID  string `json:"controller_instance_id"`
	Generation            int64  `json:"generation"`
}

type RPExternalControllerAssignment = privateFactRecord[RPExternalControllerAssignmentFact]

func (s *Store) AssignRPExternalControllerLocal(ctx context.Context, r RPExternalControllerAssignmentRequest) (RPExternalControllerAssignment, error) {
	var empty RPExternalControllerAssignment
	if err := r.Binding.Validate(); err != nil {
		return empty, err
	}
	if !studioID(r.EntityID) || r.ExpectedGeneration < 0 {
		return empty, core.NewError(core.CodeInvalidArgument, "external assignment requires an enrolled Entity and nonnegative expected generation")
	}
	return executePrivateFactCommandWithOptions(s, ctx, r.Binding, "AssignRPExternalControllerLocal", r,
		privateFactDomain{"rp_controller_authority", "RPExternalControllerAssigned", `{"authorization":"local-operator-committed-listener-assignment-v1"}`},
		privateFactOptions{pendingListenerID: r.EntityID},
		func(conn *sql.Conn) error {
			if err := s.requireNoRunningRPBackgroundProgression(ctx, conn, r.Binding.InstanceID, r.Binding.BranchID); err != nil {
				return err
			}
			var role string
			if err := conn.QueryRowContext(ctx, `SELECT principal_type FROM principals WHERE principal_id=? AND status='active'`, r.Binding.PrincipalID).Scan(&role); err != nil {
				return classifyMissing(err, "active local operator")
			}
			if role != "operator" {
				return core.NewError(core.CodeUnauthorized, "external assignment requires local operator")
			}
			return nil
		},
		func(conn *sql.Conn, c privateFactContext) (RPExternalControllerAssignmentFact, func() error, error) {
			var fact RPExternalControllerAssignmentFact
			var principal, controller, role string
			err := conn.QueryRowContext(ctx, `SELECT e.principal_id,e.controller_instance_id,p.principal_type FROM rp_external_controller_enrollments e JOIN principals p ON p.principal_id=e.principal_id AND p.status='active' WHERE e.instance_id=? AND e.branch_id=? AND e.entity_id=?`, r.Binding.InstanceID, r.Binding.BranchID, r.EntityID).Scan(&principal, &controller, &role)
			if err != nil {
				return fact, nil, classifyMissing(err, "external controller enrollment")
			}
			if role != "service" {
				return fact, nil, core.NewError(core.CodeUnauthorized, "enrolled controller is not an active service principal")
			}
			if err := validateRPBinding(ctx, conn, r.Binding.InstanceID, r.Binding.BranchID, r.EntityID); err != nil {
				return fact, nil, err
			}
			var previousGeneration int64
			var previousStatus string
			err = conn.QueryRowContext(ctx, `SELECT generation,status FROM rp_controller_authorities WHERE instance_id=? AND branch_id=? AND entity_id=?`, r.Binding.InstanceID, r.Binding.BranchID, r.EntityID).Scan(&previousGeneration, &previousStatus)
			first := errors.Is(err, sql.ErrNoRows)
			if err != nil && !first {
				return fact, nil, core.WrapError(core.CodeStorageFailure, "read controller generation before assignment", err)
			}
			if (first && r.ExpectedGeneration != 0) || (!first && (previousGeneration != r.ExpectedGeneration || previousStatus != "released")) {
				return fact, nil, core.NewError(core.CodeBranchConflict, "controller generation changed")
			}
			var sessions int
			if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_sessions WHERE instance_id=? AND branch_id=? AND controlled_entity_id=? AND control_generation=? AND status='active'`, r.Binding.InstanceID, r.Binding.BranchID, r.EntityID, r.ExpectedGeneration).Scan(&sessions); err != nil {
				return fact, nil, err
			}
			if sessions != 0 {
				return fact, nil, core.NewError(core.CodeCommandInProgress, "close target Entity RP sessions before assignment")
			}
			var activeInteractions int
			if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_interactions i JOIN rp_sessions s ON s.session_id=i.session_id WHERE s.instance_id=? AND s.branch_id=? AND s.controlled_entity_id=? AND i.status IN ('open','paused')`, r.Binding.InstanceID, r.Binding.BranchID, r.EntityID).Scan(&activeInteractions); err != nil {
				return fact, nil, core.WrapError(core.CodeStorageFailure, "check active RP interactions before assignment", err)
			}
			if activeInteractions != 0 {
				return fact, nil, core.NewError(core.CodeCommandInProgress, "finish or stop target Entity interactions before assignment")
			}
			if err := requireNoActiveRPSharedRound(ctx, conn, r.Binding.InstanceID, r.Binding.BranchID); err != nil {
				return fact, nil, err
			}
			version := "corerp.controller-assignment.v2"
			if first {
				version = "corerp.controller-assignment.v1"
			}
			fact = RPExternalControllerAssignmentFact{version, r.EntityID, principal, controller, r.ExpectedGeneration + 1}
			return fact, func() error {
				if first {
					_, err := conn.ExecContext(ctx, `INSERT INTO rp_controller_authorities(instance_id,branch_id,entity_id,principal_id,controller_instance_id,generation,source_event_id,assigned_world_time,status) VALUES (?,?,?,?,?,?,?,?,'active')`, r.Binding.InstanceID, r.Binding.BranchID, r.EntityID, principal, controller, fact.Generation, c.EventID, c.WorldTime)
					return err
				}
				return execAgentOne(ctx, conn, "reactivate RP controller", `UPDATE rp_controller_authorities SET principal_id=?,controller_instance_id=?,generation=?,source_event_id=?,assigned_world_time=?,status='active' WHERE instance_id=? AND branch_id=? AND entity_id=? AND generation=? AND status='released'`, principal, controller, fact.Generation, c.EventID, c.WorldTime, r.Binding.InstanceID, r.Binding.BranchID, r.EntityID, r.ExpectedGeneration)
			}, nil
		})
}

// No internal NPC proposal may be created or committed for an externally
// controlled Entity or a Human-controlled active session. Call within the
// same transaction as the final write.
func requireInternalRPDecisionOwner(ctx context.Context, q rpQueryer, instance, branch, entity string) error {
	source, err := rpNonInternalDecisionOwnerSource(ctx, q, instance, branch, entity)
	if err != nil {
		return err
	}
	if source != "" {
		return core.NewError(core.CodeBranchConflict, "internal NPC decision owner changed")
	}
	return nil
}

func rpNonInternalDecisionOwnerSource(ctx context.Context, q rpQueryer, instance, branch, entity string) (string, error) {
	var source string
	err := q.QueryRowContext(ctx, `SELECT source_event_id FROM rp_controller_authorities WHERE instance_id=? AND branch_id=? AND entity_id=? AND status='active'`, instance, branch, entity).Scan(&source)
	if err == nil {
		return source, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", core.WrapError(core.CodeStorageFailure, "check external RP decision owner", err)
	}
	err = q.QueryRowContext(ctx, `SELECT g.definition_event_id FROM rp_sessions s JOIN capability_grants g ON g.principal_id=s.principal_id AND g.instance_id=s.instance_id AND g.branch_id=s.branch_id AND g.subject_id=s.controlled_entity_id JOIN principals p ON p.principal_id=s.principal_id JOIN world_instances w ON w.instance_id=s.instance_id WHERE s.instance_id=? AND s.branch_id=? AND s.controlled_entity_id=? AND s.status='active' AND p.principal_type='player' AND p.status='active' AND g.capability_id='world.rp.control' AND g.status='active' AND `+studioControlPredicate+` LIMIT 1`, instance, branch, entity).Scan(&source)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", core.WrapError(core.CodeStorageFailure, "check Human RP decision owner", err)
	}
	return source, nil
}
