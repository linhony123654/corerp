package storage

import (
	"context"
	"database/sql"
	"strings"

	"corerp.local/backend/internal/core"
)

// Replacement changes an enrollment slot only after explicit release. It is
// not a controller assignment: the released generation remains fenced until
// the operator separately assigns the new enrolled principal.
type RPExternalControllerReplacementRequest struct {
	Binding               core.CareerBinding `json:"binding"`
	EntityID              string             `json:"entity_id"`
	ExpectedGeneration    int64              `json:"expected_generation"`
	ControllerPrincipalID string             `json:"controller_principal_id"`
	ControllerInstanceID  string             `json:"controller_instance_id"`
}

type RPExternalControllerReplacementFact struct {
	Version                  string `json:"version"`
	EntityID                 string `json:"entity_id"`
	OldControllerPrincipalID string `json:"old_controller_principal_id"`
	OldControllerInstanceID  string `json:"old_controller_instance_id"`
	ControllerPrincipalID    string `json:"controller_principal_id"`
	ControllerInstanceID     string `json:"controller_instance_id"`
	ReleasedGeneration       int64  `json:"released_generation"`
}

type RPExternalControllerReplacement = privateFactRecord[RPExternalControllerReplacementFact]

func (s *Store) ReplaceRPExternalControllerLocal(ctx context.Context, r RPExternalControllerReplacementRequest) (RPExternalControllerReplacement, error) {
	var empty RPExternalControllerReplacement
	if err := r.Binding.Validate(); err != nil {
		return empty, err
	}
	if !studioID(r.EntityID) || !studioID(r.ControllerPrincipalID) || !rpLocationSlotKey.MatchString(r.ControllerInstanceID) || r.ExpectedGeneration < 2 ||
		strings.TrimSpace(r.EntityID) != r.EntityID || strings.TrimSpace(r.ControllerPrincipalID) != r.ControllerPrincipalID ||
		r.EntityID == r.ControllerPrincipalID || r.EntityID == r.ControllerInstanceID || r.ControllerPrincipalID == r.ControllerInstanceID {
		return empty, core.NewError(core.CodeInvalidArgument, "replacement requires a released Entity, generation and distinct service controller")
	}
	return executePrivateFactCommand(s, ctx, r.Binding, "ReplaceRPExternalControllerLocal", r,
		privateFactDomain{"rp_controller_replacement", "RPExternalControllerReplaced", `{"authorization":"local-operator-released-controller-replacement-v1"}`},
		func(conn *sql.Conn) error {
			var role string
			if err := conn.QueryRowContext(ctx, `SELECT principal_type FROM principals WHERE principal_id=? AND status='active'`, r.Binding.PrincipalID).Scan(&role); err != nil {
				return classifyMissing(err, "active local operator")
			}
			if role != "operator" {
				return core.NewError(core.CodeUnauthorized, "controller replacement requires local operator")
			}
			return nil
		},
		func(conn *sql.Conn, c privateFactContext) (RPExternalControllerReplacementFact, func() error, error) {
			var fact RPExternalControllerReplacementFact
			var role string
			if err := conn.QueryRowContext(ctx, `SELECT principal_type FROM principals WHERE principal_id=? AND status='active'`, r.ControllerPrincipalID).Scan(&role); err != nil || role != "service" {
				if err != nil && err != sql.ErrNoRows {
					return fact, nil, err
				}
				return fact, nil, core.NewError(core.CodeUnauthorized, "replacement requires an active service principal")
			}
			if err := validateRPBinding(ctx, conn, r.Binding.InstanceID, r.Binding.BranchID, r.EntityID); err != nil {
				return fact, nil, err
			}
			var oldPrincipal, oldController, status string
			var generation int64
			err := conn.QueryRowContext(ctx, `SELECT e.principal_id,e.controller_instance_id,a.generation,a.status FROM rp_external_controller_enrollments e JOIN rp_controller_authorities a ON a.instance_id=e.instance_id AND a.branch_id=e.branch_id AND a.entity_id=e.entity_id WHERE e.instance_id=? AND e.branch_id=? AND e.entity_id=?`, r.Binding.InstanceID, r.Binding.BranchID, r.EntityID).Scan(&oldPrincipal, &oldController, &generation, &status)
			if err != nil {
				return fact, nil, classifyMissing(err, "released external controller enrollment")
			}
			if status != "released" || generation != r.ExpectedGeneration {
				return fact, nil, core.NewError(core.CodeBranchConflict, "controller must be released at expected generation")
			}
			if err := requireNoActiveRPSharedRound(ctx, conn, r.Binding.InstanceID, r.Binding.BranchID); err != nil {
				return fact, nil, err
			}
			var activeInteractions int
			if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_interactions i JOIN rp_sessions s ON s.session_id=i.session_id WHERE s.instance_id=? AND s.branch_id=? AND i.status IN ('open','paused')`, r.Binding.InstanceID, r.Binding.BranchID).Scan(&activeInteractions); err != nil {
				return fact, nil, err
			}
			if activeInteractions != 0 {
				return fact, nil, core.NewError(core.CodeCommandInProgress, "settle RP interaction before controller replacement")
			}
			if oldPrincipal == r.ControllerPrincipalID {
				return fact, nil, core.NewError(core.CodeBranchConflict, "replacement requires a different principal")
			}
			if oldController == r.ControllerInstanceID {
				return fact, nil, core.NewError(core.CodeBranchConflict, "replacement requires a different controller instance")
			}
			var duplicate int
			if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_external_controller_enrollments WHERE instance_id=? AND branch_id=? AND entity_id<>? AND (principal_id=? OR controller_instance_id=?)`, r.Binding.InstanceID, r.Binding.BranchID, r.EntityID, r.ControllerPrincipalID, r.ControllerInstanceID).Scan(&duplicate); err != nil {
				return fact, nil, err
			}
			if duplicate != 0 {
				return fact, nil, core.NewError(core.CodeBranchConflict, "replacement controller is already enrolled in this world")
			}
			fact = RPExternalControllerReplacementFact{"corerp.controller-replacement.v1", r.EntityID, oldPrincipal, oldController, r.ControllerPrincipalID, r.ControllerInstanceID, generation}
			return fact, func() error {
				return execAgentOne(ctx, conn, "replace RP controller enrollment", `UPDATE rp_external_controller_enrollments SET principal_id=?,controller_instance_id=?,source_event_id=?,enrolled_world_time=? WHERE instance_id=? AND branch_id=? AND entity_id=? AND principal_id=? AND controller_instance_id=?`, r.ControllerPrincipalID, r.ControllerInstanceID, c.EventID, c.WorldTime, r.Binding.InstanceID, r.Binding.BranchID, r.EntityID, oldPrincipal, oldController)
			}, nil
		})
}
