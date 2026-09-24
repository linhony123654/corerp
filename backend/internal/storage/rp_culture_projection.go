package storage

import (
	"context"
	"corerp.local/backend/internal/core"
	"database/sql"
	"encoding/json"
	"sort"
)

type cultureGrantProjection struct {
	GrantID      string `json:"grant_id"`
	PrincipalID  string `json:"principal_id"`
	InstanceID   string `json:"instance_id"`
	BranchID     string `json:"branch_id"`
	SubjectID    string `json:"subject_id"`
	CapabilityID string `json:"capability_id"`
	Status       string `json:"status"`
	EventID      string `json:"event_id"`
	FieldScope   string `json:"field_scope"`
	AmountLimit  *int64 `json:"amount_limit"`
}

func cultureGrantProjectionDifferences(ctx context.Context, q replayQuerier, instance, branch string, through int64) ([]ProjectionDifference, error) {
	rows, err := q.QueryContext(ctx, `SELECT event_id,payload FROM events WHERE instance_id=? AND branch_id=? AND event_sequence<=? AND event_type='RPCultureFactRecorded' AND json_extract(payload,'$.kind') IN ('territory','territory_authority') ORDER BY event_sequence`, instance, branch, through)
	if err != nil {
		return nil, err
	}
	expected := map[string]cultureGrantProjection{}
	for rows.Next() {
		var id, raw string
		var fact CultureFact
		if err := rows.Scan(&id, &raw); err != nil {
			rows.Close()
			return nil, err
		}
		if err := json.Unmarshal([]byte(raw), &fact); err != nil {
			rows.Close()
			return nil, err
		}
		t := fact.Territory
		if t == nil || (fact.Kind == "territory" && t.GrantID != "grant_"+id) || t.StewardPrincipalID == "" || t.ScopeID == "" || (t.ScopeKind != "world" && t.ScopeKind != "region") {
			rows.Close()
			return nil, core.NewError(core.CodeProjectionDiverged, "invalid culture territory grant source")
		}
		status := "active"
		if fact.Kind == "territory_authority" {
			previous, ok := expected[t.GrantID]
			if !ok || previous.SubjectID != t.ScopeKind+":"+t.ScopeID || (t.GrantStatus != "active" && t.GrantStatus != "revoked") {
				rows.Close()
				return nil, core.NewError(core.CodeProjectionDiverged, "culture authority change lacks installed scope")
			}
			status = t.GrantStatus
		}
		expected[t.GrantID] = cultureGrantProjection{GrantID: t.GrantID, PrincipalID: t.StewardPrincipalID, InstanceID: instance, BranchID: branch, SubjectID: t.ScopeKind + ":" + t.ScopeID, CapabilityID: cultureDefineCapability, Status: status, EventID: id, FieldScope: "[]"}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	// Include authoritative IDs even when corruption changes capability or scope.
	rows, err = q.QueryContext(ctx, `SELECT grant_id,principal_id,instance_id,branch_id,subject_id,capability_id,status,definition_event_id,field_scope,amount_limit_minor FROM capability_grants WHERE (instance_id=? AND branch_id=? AND capability_id=?) OR grant_id IN (SELECT json_extract(payload,'$.territory.grant_id') FROM events WHERE instance_id=? AND branch_id=? AND event_sequence<=? AND event_type='RPCultureFactRecorded' AND json_extract(payload,'$.kind')='territory')`, instance, branch, cultureDefineCapability, instance, branch, through)
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
		d := ProjectionDifference{Projection: "culture_scope_grant", Key: id}
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

func repairCultureGrantProjections(ctx context.Context, conn *sql.Conn, differences []ProjectionDifference) error {
	// Remove only exact diagnosed projection rows first, allowing swapped/corrupt
	// unique subjects to be restored together inside the outer rebuild transaction.
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
		if _, err := conn.ExecContext(ctx, `INSERT INTO capability_definitions(capability_id,description,policy_version) VALUES (?,'Define culture only within an explicitly installed cultural scope','culture-v1') ON CONFLICT(capability_id) DO NOTHING`, cultureDefineCapability); err != nil {
			return err
		}
		if _, err := conn.ExecContext(ctx, `INSERT INTO capability_grants(grant_id,principal_id,capability_id,instance_id,branch_id,subject_id,field_scope,status,definition_event_id) VALUES (?,?,?,?,?,?,'[]',?,?)`, g.GrantID, g.PrincipalID, g.CapabilityID, g.InstanceID, g.BranchID, g.SubjectID, g.Status, g.EventID); err != nil {
			return err
		}
	}
	return nil
}
