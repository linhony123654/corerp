package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"sort"

	"corerp.local/backend/internal/core"
)

type careerRoleProjection struct {
	GrantID        string `json:"grant_id"`
	PrincipalID    string `json:"principal_id"`
	OrganizationID string `json:"organization_id"`
	Status         string `json:"status"`
	EventID        string `json:"event_id"`
	CapabilityID   string `json:"capability_id"`
	FieldScope     string `json:"field_scope"`
	AmountLimit    *int64 `json:"amount_limit"`
}

func careerRoleProjectionDifferences(ctx context.Context, query replayQuerier, instance, branch string, through int64) ([]ProjectionDifference, error) {
	rows, err := query.QueryContext(ctx, `SELECT event_id,json_extract(payload,'$.role_grant') FROM events WHERE instance_id=? AND branch_id=? AND event_sequence<=? AND event_type='CareerEmploymentTermsActivated' AND json_type(payload,'$.role_grant')='object' ORDER BY event_sequence`, instance, branch, through)
	if err != nil {
		return nil, err
	}
	expected := map[string]careerRoleProjection{}
	for rows.Next() {
		var source, raw string
		if err := rows.Scan(&source, &raw); err != nil {
			rows.Close()
			return nil, err
		}
		var grant CareerRoleGrant
		if err := json.Unmarshal([]byte(raw), &grant); err != nil {
			rows.Close()
			return nil, err
		}
		if grant.GrantID == "" || grant.PrincipalID == "" || grant.OrganizationID == "" || (grant.Status != "active" && grant.Status != "revoked") {
			rows.Close()
			return nil, core.NewError(core.CodeProjectionDiverged, "invalid authoritative position grant")
		}
		expected[grant.GrantID] = careerRoleProjection{GrantID: grant.GrantID, PrincipalID: grant.PrincipalID, OrganizationID: grant.OrganizationID, Status: grant.Status, EventID: source, CapabilityID: core.CareerPositionManageCapability, FieldScope: "[]"}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	rows, err = query.QueryContext(ctx, `SELECT grant_id,principal_id,subject_id,status,definition_event_id,capability_id,field_scope,amount_limit_minor FROM capability_grants WHERE instance_id=? AND branch_id=? AND capability_id=?`, instance, branch, core.CareerPositionManageCapability)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	actual := map[string]careerRoleProjection{}
	for rows.Next() {
		var grant careerRoleProjection
		var limit sql.NullInt64
		if err := rows.Scan(&grant.GrantID, &grant.PrincipalID, &grant.OrganizationID, &grant.Status, &grant.EventID, &grant.CapabilityID, &grant.FieldScope, &limit); err != nil {
			return nil, err
		}
		if limit.Valid {
			amount := limit.Int64
			grant.AmountLimit = &amount
		}
		actual[grant.GrantID] = grant
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	keys := map[string]bool{}
	for k := range expected {
		keys[k] = true
	}
	for k := range actual {
		keys[k] = true
	}
	ordered := make([]string, 0, len(keys))
	for k := range keys {
		ordered = append(ordered, k)
	}
	sort.Strings(ordered)
	var result []ProjectionDifference
	for _, key := range ordered {
		d := ProjectionDifference{Projection: "career_role_grant", Key: key}
		if g, ok := expected[key]; ok {
			raw, err := core.CanonicalJSON(g)
			if err != nil {
				return nil, err
			}
			d.ExpectedText = string(raw)
		}
		if g, ok := actual[key]; ok {
			raw, err := core.CanonicalJSON(g)
			if err != nil {
				return nil, err
			}
			d.ActualText = string(raw)
		}
		if d.ExpectedText != d.ActualText {
			result = append(result, d)
		}
	}
	return result, nil
}

func repairCareerRoleProjections(ctx context.Context, conn *sql.Conn, instance, branch string, differences []ProjectionDifference) error {
	for _, d := range differences {
		if d.ExpectedText == "" {
			if _, err := conn.ExecContext(ctx, `DELETE FROM capability_grants WHERE grant_id=? AND instance_id=? AND branch_id=? AND capability_id=?`, d.Key, instance, branch, core.CareerPositionManageCapability); err != nil {
				return err
			}
			continue
		}
	}
	for _, d := range differences {
		if d.ExpectedText == "" {
			continue
		}
		var expected careerRoleProjection
		if err := json.Unmarshal([]byte(d.ExpectedText), &expected); err != nil {
			return err
		}
		if err := applyCareerRoleGrant(ctx, conn, instance, branch, expected.EventID, CareerRoleGrant{GrantID: expected.GrantID, PrincipalID: expected.PrincipalID, OrganizationID: expected.OrganizationID, Status: expected.Status}); err != nil {
			return err
		}
	}
	return nil
}
