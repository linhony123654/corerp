package storage

import (
	"context"
	"database/sql"
	"strings"

	"corerp.local/backend/internal/core"
)

// Enrollment reserves a dedicated external credential for an existing
// resident. It is not a decision-controller handoff or an RP control grant.
type RPExternalControllerEnrollmentRequest struct {
	Binding               core.CareerBinding `json:"binding"`
	EntityID              string             `json:"entity_id"`
	ControllerPrincipalID string             `json:"controller_principal_id"`
	ControllerInstanceID  string             `json:"controller_instance_id"`
}

type RPExternalControllerEnrollmentFact struct {
	Version               string `json:"version"`
	EntityID              string `json:"entity_id"`
	ControllerPrincipalID string `json:"controller_principal_id"`
	ControllerInstanceID  string `json:"controller_instance_id"`
}

type RPExternalControllerEnrollment = privateFactRecord[RPExternalControllerEnrollmentFact]

func (s *Store) EnrollRPExternalControllerLocal(ctx context.Context, r RPExternalControllerEnrollmentRequest) (RPExternalControllerEnrollment, error) {
	var empty RPExternalControllerEnrollment
	if err := r.Binding.Validate(); err != nil {
		return empty, err
	}
	if !studioID(r.EntityID) || !studioID(r.ControllerPrincipalID) || !rpLocationSlotKey.MatchString(r.ControllerInstanceID) ||
		strings.TrimSpace(r.EntityID) != r.EntityID || strings.TrimSpace(r.ControllerPrincipalID) != r.ControllerPrincipalID ||
		r.EntityID == r.ControllerPrincipalID || r.EntityID == r.ControllerInstanceID || r.ControllerPrincipalID == r.ControllerInstanceID {
		return empty, core.NewError(core.CodeInvalidArgument, "distinct bounded Entity, service principal and controller instance required")
	}
	return executePrivateFactCommand(s, ctx, r.Binding, "EnrollRPExternalControllerLocal", r,
		privateFactDomain{"rp_external_controller_enrollment", "RPExternalControllerEnrolled", `{"authorization":"local-operator-external-controller-enrollment-v1"}`},
		func(conn *sql.Conn) error {
			var role string
			if err := conn.QueryRowContext(ctx, `SELECT principal_type FROM principals WHERE principal_id=? AND status='active'`, r.Binding.PrincipalID).Scan(&role); err != nil || role != "operator" {
				if err != nil && err != sql.ErrNoRows {
					return err
				}
				return core.NewError(core.CodeUnauthorized, "external controller enrollment requires the local operator")
			}
			return nil
		},
		func(conn *sql.Conn, c privateFactContext) (RPExternalControllerEnrollmentFact, func() error, error) {
			var fact RPExternalControllerEnrollmentFact
			var role string
			if err := conn.QueryRowContext(ctx, `SELECT principal_type FROM principals WHERE principal_id=? AND status='active'`, r.ControllerPrincipalID).Scan(&role); err != nil || role != "service" {
				if err != nil && err != sql.ErrNoRows {
					return fact, nil, err
				}
				return fact, nil, core.NewError(core.CodeUnauthorized, "external controller requires a distinct active service principal")
			}
			if err := validateRPBinding(ctx, conn, r.Binding.InstanceID, r.Binding.BranchID, r.EntityID); err != nil {
				return fact, nil, err
			}
			if err := requireNoActiveRPSharedRound(ctx, conn, r.Binding.InstanceID, r.Binding.BranchID); err != nil {
				return fact, nil, err
			}
			var count, duplicate int
			if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_external_controller_enrollments WHERE instance_id=? AND branch_id=?`, r.Binding.InstanceID, r.Binding.BranchID).Scan(&count); err != nil {
				return fact, nil, err
			}
			if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_external_controller_enrollments WHERE instance_id=? AND branch_id=? AND (entity_id=? OR principal_id=? OR controller_instance_id=?)`, r.Binding.InstanceID, r.Binding.BranchID, r.EntityID, r.ControllerPrincipalID, r.ControllerInstanceID).Scan(&duplicate); err != nil {
				return fact, nil, err
			}
			if duplicate != 0 {
				return fact, nil, core.NewError(core.CodeBranchConflict, "external controller Entity, principal or instance is already enrolled")
			}
			if count >= 2 {
				return fact, nil, core.NewError(core.CodeBranchConflict, "bounded world already has two external controller enrollments")
			}
			fact = RPExternalControllerEnrollmentFact{"corerp.controller-enrollment.v1", r.EntityID, r.ControllerPrincipalID, r.ControllerInstanceID}
			return fact, func() error {
				_, err := conn.ExecContext(ctx, `INSERT INTO rp_external_controller_enrollments(instance_id,branch_id,entity_id,principal_id,controller_instance_id,source_event_id,enrolled_world_time) VALUES (?,?,?,?,?,?,?)`, r.Binding.InstanceID, r.Binding.BranchID, r.EntityID, r.ControllerPrincipalID, r.ControllerInstanceID, c.EventID, c.WorldTime)
				return err
			}, nil
		})
}
