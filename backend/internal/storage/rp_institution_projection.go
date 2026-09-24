package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"sort"

	"corerp.local/backend/internal/core"
)

func institutionGrantProjectionDifferences(ctx context.Context, q replayQuerier, instance, branch string, through int64) ([]ProjectionDifference, error) {
	rows, err := q.QueryContext(ctx, `SELECT event_id,payload FROM events WHERE instance_id=? AND branch_id=? AND event_sequence<=? AND event_type='RPInstitutionFactRecorded' AND json_extract(payload,'$.kind') IN ('institution','institution_authority') ORDER BY event_sequence`, instance, branch, through)
	if err != nil {
		return nil, err
	}
	expected := map[string]cultureGrantProjection{}
	for rows.Next() {
		var id, raw string
		var fact InstitutionFact
		if err := rows.Scan(&id, &raw); err != nil {
			rows.Close()
			return nil, err
		}
		if err := json.Unmarshal([]byte(raw), &fact); err != nil {
			rows.Close()
			return nil, err
		}
		if fact.Kind == "institution_authority" {
			a := fact.Authority
			if a == nil {
				rows.Close()
				return nil, core.NewError(core.CodeProjectionDiverged, "missing institution authority")
			}
			old, ok := expected[a.Role.GrantID]
			if !ok || old.EventID != a.PreviousEventID || old.SubjectID != fact.InstitutionID || old.CapabilityID != a.Role.CapabilityID || a.Role.PrincipalID == "" || a.Role.EntityID == "" || (a.Status != "active" && a.Status != "revoked") {
				rows.Close()
				return nil, core.NewError(core.CodeProjectionDiverged, "invalid institution authority chain")
			}
			old.PrincipalID = a.Role.PrincipalID
			old.Status = a.Status
			old.EventID = id
			expected[a.Role.GrantID] = old
			continue
		}
		if fact.Definition == nil || fact.InstitutionID == "" || fact.Definition.InstitutionID != fact.InstitutionID || len(fact.Definition.Roles) != 3 {
			rows.Close()
			return nil, core.NewError(core.CodeProjectionDiverged, "institution grant lacks definition")
		}
		seen := map[string]bool{}
		for _, role := range fact.Definition.Roles {
			if role.GrantID != "grant_"+id+"_"+role.CapabilityID || role.PrincipalID == "" || role.EntityID == "" || seen[role.CapabilityID] || (role.CapabilityID != institutionLegislate && role.CapabilityID != institutionEnforce && role.CapabilityID != institutionReview) {
				rows.Close()
				return nil, core.NewError(core.CodeProjectionDiverged, "invalid institution role source")
			}
			seen[role.CapabilityID] = true
			expected[role.GrantID] = cultureGrantProjection{GrantID: role.GrantID, PrincipalID: role.PrincipalID, InstanceID: instance, BranchID: branch, SubjectID: fact.InstitutionID, CapabilityID: role.CapabilityID, Status: "active", EventID: id, FieldScope: "[]"}
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	// Include source-owned IDs even if their projection capability/scope was corrupted.
	rows, err = q.QueryContext(ctx, `SELECT grant_id,principal_id,instance_id,branch_id,subject_id,capability_id,status,definition_event_id,field_scope,amount_limit_minor FROM capability_grants WHERE (instance_id=? AND branch_id=? AND capability_id IN (?,?,?)) OR grant_id IN (SELECT json_extract(r.value,'$.grant_id') FROM events e JOIN json_each(e.payload,'$.definition.roles') r WHERE e.instance_id=? AND e.branch_id=? AND e.event_sequence<=? AND e.event_type='RPInstitutionFactRecorded' AND json_extract(e.payload,'$.kind')='institution')`, instance, branch, institutionLegislate, institutionEnforce, institutionReview, instance, branch, through)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	actual := map[string]cultureGrantProjection{}
	for rows.Next() {
		var g cultureGrantProjection
		var amount sql.NullInt64
		if err := rows.Scan(&g.GrantID, &g.PrincipalID, &g.InstanceID, &g.BranchID, &g.SubjectID, &g.CapabilityID, &g.Status, &g.EventID, &g.FieldScope, &amount); err != nil {
			return nil, err
		}
		if amount.Valid {
			value := amount.Int64
			g.AmountLimit = &value
		}
		actual[g.GrantID] = g
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	keys := map[string]bool{}
	for id := range expected {
		keys[id] = true
	}
	for id := range actual {
		keys[id] = true
	}
	ordered := []string{}
	for id := range keys {
		ordered = append(ordered, id)
	}
	sort.Strings(ordered)
	var result []ProjectionDifference
	for _, id := range ordered {
		d := ProjectionDifference{Projection: "institution_role_grant", Key: id}
		if g, ok := expected[id]; ok {
			raw, err := core.CanonicalJSON(g)
			if err != nil {
				return nil, err
			}
			d.ExpectedText = string(raw)
		}
		if g, ok := actual[id]; ok {
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

func repairInstitutionGrantProjections(ctx context.Context, conn *sql.Conn, differences []ProjectionDifference) error {
	for _, d := range differences {
		if d.ActualText != "" {
			if _, err := conn.ExecContext(ctx, `DELETE FROM capability_grants WHERE grant_id=?`, d.Key); err != nil {
				return err
			}
		}
	}
	for _, d := range differences {
		if d.ExpectedText == "" {
			continue
		}
		var g cultureGrantProjection
		if err := json.Unmarshal([]byte(d.ExpectedText), &g); err != nil {
			return err
		}
		if _, err := conn.ExecContext(ctx, `INSERT INTO capability_definitions(capability_id,description,policy_version) VALUES (?,'Scoped institutional role, separate from cultural and economic authority','institution-v1') ON CONFLICT(capability_id) DO NOTHING`, g.CapabilityID); err != nil {
			return err
		}
		if _, err := conn.ExecContext(ctx, `INSERT INTO capability_grants(grant_id,principal_id,capability_id,instance_id,branch_id,subject_id,field_scope,status,definition_event_id) VALUES (?,?,?,?,?,?,'[]',?,?)`, g.GrantID, g.PrincipalID, g.CapabilityID, g.InstanceID, g.BranchID, g.SubjectID, g.Status, g.EventID); err != nil {
			return err
		}
	}
	return nil
}
