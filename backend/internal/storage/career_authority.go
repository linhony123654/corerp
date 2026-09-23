package storage

import (
	"context"
	"database/sql"

	"corerp.local/backend/internal/core"
)

// Stored in the same private activation Event as position/pay. This is a
// projection instruction for existing capability_grants, not a second ACL.
type CareerRoleGrant struct {
	GrantID        string `json:"grant_id"`
	PrincipalID    string `json:"principal_id"`
	OrganizationID string `json:"organization_id"`
	ContractID     string `json:"contract_id"`
	Status         string `json:"status"`
}

func prepareCareerRoleGrant(ctx context.Context, conn *sql.Conn, job CareerEmploymentFact) (*CareerRoleGrant, error) {
	if len(job.Capabilities) > 1 || (len(job.Capabilities) == 1 && job.Capabilities[0] != core.CareerPositionManageCapability) {
		return nil, core.NewError(core.CodeProjectionDiverged, "unsupported position authority")
	}
	var principal string
	if err := conn.QueryRowContext(ctx, `SELECT principal_id FROM agent_profiles WHERE agent_id=? AND instance_id=? AND branch_id=?`, job.EmployeeID, M2DemoInstanceID, M2DemoBranchID).Scan(&principal); err != nil {
		return nil, classifyMissing(err, "position principal")
	}
	hash, err := core.HashJSON([]string{M2DemoInstanceID, M2DemoBranchID, principal, job.OrganizationID})
	if err != nil {
		return nil, err
	}
	grant := &CareerRoleGrant{GrantID: "grant_career_role_" + hash, PrincipalID: principal, OrganizationID: job.OrganizationID, ContractID: job.ContractID, Status: "revoked"}
	if len(job.Capabilities) == 1 {
		grant.Status = "active"
	}
	var exists int
	if err := conn.QueryRowContext(ctx, `SELECT (SELECT COUNT(*) FROM capability_grants WHERE grant_id=?) + (SELECT COUNT(*) FROM events WHERE instance_id=? AND branch_id=? AND event_type='CareerEmploymentTermsActivated' AND json_extract(payload,'$.role_grant.grant_id')=?)`, grant.GrantID, M2DemoInstanceID, M2DemoBranchID, grant.GrantID).Scan(&exists); err != nil {
		return nil, err
	}
	if grant.Status == "revoked" && exists == 0 {
		return nil, nil
	}
	return grant, nil
}

func applyCareerRoleGrant(ctx context.Context, conn *sql.Conn, instance, branch, eventID string, grant CareerRoleGrant) error {
	if grant.Status != "active" && grant.Status != "revoked" {
		return core.NewError(core.CodeProjectionDiverged, "invalid role grant status")
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO capability_definitions(capability_id,description,policy_version) VALUES (?,'Career management delegated only by an effective position within its organization','career-position-v1') ON CONFLICT(capability_id) DO NOTHING`, core.CareerPositionManageCapability); err != nil {
		return err
	}
	_, err := conn.ExecContext(ctx, `INSERT INTO capability_grants(grant_id,principal_id,capability_id,instance_id,branch_id,subject_id,field_scope,status,definition_event_id) VALUES (?,?,?,?,?,?,'[]',?,?) ON CONFLICT(grant_id) DO UPDATE SET principal_id=excluded.principal_id,capability_id=excluded.capability_id,instance_id=excluded.instance_id,branch_id=excluded.branch_id,subject_id=excluded.subject_id,field_scope='[]',amount_limit_minor=NULL,status=excluded.status,definition_event_id=excluded.definition_event_id`, grant.GrantID, grant.PrincipalID, core.CareerPositionManageCapability, instance, branch, grant.OrganizationID, grant.Status, eventID)
	return err
}
