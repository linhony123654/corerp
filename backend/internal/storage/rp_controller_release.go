package storage

import (
	"context"
	"database/sql"

	"corerp.local/backend/internal/core"
)

// Release is explicit operator action, not a controller disconnect timeout.
// The sourced released generation stays in the projection so an old session
// can never become current when the service owner is absent.
type RPExternalControllerReleaseRequest struct {
	Binding            core.CareerBinding `json:"binding"`
	EntityID           string             `json:"entity_id"`
	ExpectedGeneration int64              `json:"expected_generation"`
}

type RPExternalControllerReleaseFact struct {
	Version               string `json:"version"`
	EntityID              string `json:"entity_id"`
	ControllerPrincipalID string `json:"controller_principal_id"`
	ControllerInstanceID  string `json:"controller_instance_id"`
	Generation            int64  `json:"generation"`
}

type RPExternalControllerRelease = privateFactRecord[RPExternalControllerReleaseFact]

func (s *Store) ReleaseRPExternalControllerLocal(ctx context.Context, r RPExternalControllerReleaseRequest) (RPExternalControllerRelease, error) {
	var empty RPExternalControllerRelease
	if err := r.Binding.Validate(); err != nil {
		return empty, err
	}
	if !studioID(r.EntityID) || r.ExpectedGeneration < 1 {
		return empty, core.NewError(core.CodeInvalidArgument, "release requires assigned Entity and positive expected generation")
	}
	return executePrivateFactCommand(s, ctx, r.Binding, "ReleaseRPExternalControllerLocal", r,
		privateFactDomain{"rp_controller_release", "RPExternalControllerReleased", `{"authorization":"local-operator-quiescent-release-v1"}`},
		func(conn *sql.Conn) error {
			var role string
			if err := conn.QueryRowContext(ctx, `SELECT principal_type FROM principals WHERE principal_id=? AND status='active'`, r.Binding.PrincipalID).Scan(&role); err != nil {
				return classifyMissing(err, "active local operator")
			}
			if role != "operator" {
				return core.NewError(core.CodeUnauthorized, "external release requires local operator")
			}
			return nil
		},
		func(conn *sql.Conn, c privateFactContext) (RPExternalControllerReleaseFact, func() error, error) {
			var fact RPExternalControllerReleaseFact
			var principal, controller, status string
			var generation int64
			err := conn.QueryRowContext(ctx, `SELECT principal_id,controller_instance_id,generation,status FROM rp_controller_authorities WHERE instance_id=? AND branch_id=? AND entity_id=?`, r.Binding.InstanceID, r.Binding.BranchID, r.EntityID).Scan(&principal, &controller, &generation, &status)
			if err != nil {
				return fact, nil, classifyMissing(err, "active external controller")
			}
			if status != "active" || generation != r.ExpectedGeneration {
				return fact, nil, core.NewError(core.CodeBranchConflict, "controller generation changed before release")
			}
			var activeRounds, activeInteractions int
			if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_shared_round_participants p JOIN rp_shared_rounds r ON r.round_id=p.round_id WHERE r.instance_id=? AND r.branch_id=? AND p.entity_id=? AND r.status IN ('open','advancing')`, r.Binding.InstanceID, r.Binding.BranchID, r.EntityID).Scan(&activeRounds); err != nil {
				return fact, nil, core.WrapError(core.CodeStorageFailure, "check active shared rounds before release", err)
			}
			if activeRounds != 0 {
				return fact, nil, core.NewError(core.CodeCommandInProgress, "settle shared round before controller release")
			}
			if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_interactions i JOIN rp_sessions s ON s.session_id=i.session_id WHERE s.instance_id=? AND s.branch_id=? AND s.controlled_entity_id=? AND s.control_generation=? AND i.status IN ('open','paused')`, r.Binding.InstanceID, r.Binding.BranchID, r.EntityID, generation).Scan(&activeInteractions); err != nil {
				return fact, nil, core.WrapError(core.CodeStorageFailure, "check active interactions before release", err)
			}
			if activeInteractions != 0 {
				return fact, nil, core.NewError(core.CodeCommandInProgress, "finish or stop interactions before controller release")
			}
			fact = RPExternalControllerReleaseFact{"corerp.controller-release.v1", r.EntityID, principal, controller, generation + 1}
			return fact, func() error {
				if err := execAgentOne(ctx, conn, "release RP controller", `UPDATE rp_controller_authorities SET generation=?,source_event_id=?,status='released' WHERE instance_id=? AND branch_id=? AND entity_id=? AND generation=? AND status='active'`, fact.Generation, c.EventID, r.Binding.InstanceID, r.Binding.BranchID, r.EntityID, generation); err != nil {
					return err
				}
				_, err := conn.ExecContext(ctx, `UPDATE rp_sessions SET status='closed' WHERE instance_id=? AND branch_id=? AND controlled_entity_id=? AND control_generation=? AND controller_instance_id=? AND status='active'`, r.Binding.InstanceID, r.Binding.BranchID, r.EntityID, generation, controller)
				return err
			}, nil
		})
}
