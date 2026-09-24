package storage

import (
	"context"
	"database/sql"
	"encoding/json"

	"corerp.local/backend/internal/core"
)

type InstitutionAuthorityRequest struct {
	Binding       core.CareerBinding `json:"binding"`
	InstitutionID string             `json:"institution_id"`
	CapabilityID  string             `json:"capability_id"`
	Decision      string             `json:"decision"`
	EntityID      string             `json:"entity_id,omitempty"`
}
type InstitutionAuthorityFact struct {
	Role                 InstitutionRole `json:"role"`
	Status               string          `json:"status"`
	PreviousEventID      string          `json:"previous_event_id"`
	BuilderSourceEventID string          `json:"builder_source_event_id"`
}

func readInstitutionRole(ctx context.Context, conn *sql.Conn, b core.CareerBinding, institution, capability string) (InstitutionRole, string, string, error) {
	definition, source, err := readInstitutionDefinition(ctx, conn, b, institution)
	if err != nil {
		return InstitutionRole{}, "", "", err
	}
	var role InstitutionRole
	for _, r := range definition.Roles {
		if r.CapabilityID == capability {
			role = r
		}
	}
	if role.GrantID == "" {
		return role, "", "", core.NewError(core.CodeInvalidArgument, "unknown institutional role")
	}
	var id, raw string
	err = conn.QueryRowContext(ctx, `SELECT event_id,payload FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPInstitutionFactRecorded' AND json_extract(payload,'$.kind')='institution_authority' AND json_extract(payload,'$.institution_id')=? AND json_extract(payload,'$.authority.role.capability_id')=? ORDER BY event_sequence DESC LIMIT 1`, b.InstanceID, b.BranchID, institution, capability).Scan(&id, &raw)
	if err == sql.ErrNoRows {
		return role, "active", source, nil
	}
	if err != nil {
		return role, "", "", err
	}
	var fact InstitutionFact
	if err := json.Unmarshal([]byte(raw), &fact); err != nil {
		return role, "", "", err
	}
	if fact.Authority == nil || fact.Authority.Role.GrantID != role.GrantID {
		return role, "", "", core.NewError(core.CodeProjectionDiverged, "invalid institutional authority source")
	}
	return fact.Authority.Role, fact.Authority.Status, id, nil
}

func (s *Store) ChangeRPInstitutionAuthority(ctx context.Context, r InstitutionAuthorityRequest) (InstitutionRecord, error) {
	if (r.CapabilityID != institutionLegislate && r.CapabilityID != institutionEnforce && r.CapabilityID != institutionReview) || (r.Decision != "revoke" && r.Decision != "appoint") || (r.Decision == "revoke" && r.EntityID != "") || (r.Decision == "appoint" && r.EntityID == "") {
		return InstitutionRecord{}, core.NewError(core.CodeInvalidArgument, "invalid institutional authority transition")
	}
	var builder string
	return executePrivateFactCommand(s, ctx, r.Binding, "ChangeRPInstitutionAuthority", r, privateFactDomain{"institution", "RPInstitutionFactRecorded", `{"authorization":"institution-builder-v1"}`}, func(conn *sql.Conn) error {
		var err error
		builder, err = authorizeCultureBuilder(ctx, conn, r.Binding)
		return err
	}, func(conn *sql.Conn, c privateFactContext) (InstitutionFact, func() error, error) {
		role, status, source, err := readInstitutionRole(ctx, conn, r.Binding, r.InstitutionID, r.CapabilityID)
		if err != nil {
			return InstitutionFact{}, nil, err
		}
		if r.Decision == "revoke" {
			if status == "revoked" {
				return InstitutionFact{}, nil, core.NewError(core.CodeBranchConflict, "role already revoked")
			}
			status = "revoked"
		} else {
			if status == "active" && role.EntityID == r.EntityID {
				return InstitutionFact{}, nil, core.NewError(core.CodeBranchConflict, "same role holder already appointed")
			}
			if err := validateRPBinding(ctx, conn, r.Binding.InstanceID, r.Binding.BranchID, r.EntityID); err != nil {
				return InstitutionFact{}, nil, err
			}
			role.EntityID = r.EntityID
			status = "active"
			if err := conn.QueryRowContext(ctx, `SELECT principal_id FROM agent_profiles WHERE agent_id=?`, r.EntityID).Scan(&role.PrincipalID); err != nil {
				return InstitutionFact{}, nil, err
			}
		}
		fact := InstitutionFact{Version: "corerp.institution.v1", Kind: "institution_authority", InstitutionID: r.InstitutionID, Authority: &InstitutionAuthorityFact{Role: role, Status: status, PreviousEventID: source, BuilderSourceEventID: builder}}
		return fact, func() error {
			diffs, err := institutionGrantProjectionDifferences(ctx, conn, r.Binding.InstanceID, r.Binding.BranchID, c.Sequence)
			if err != nil {
				return err
			}
			var scoped []ProjectionDifference
			for _, d := range diffs {
				if d.Key == role.GrantID {
					scoped = append(scoped, d)
				}
			}
			return repairInstitutionGrantProjections(ctx, conn, scoped)
		}, nil
	})
}
