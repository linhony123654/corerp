package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"sort"
	"strings"

	"corerp.local/backend/internal/core"
)

type StudioAccessRequest struct {
	Purpose           string             `json:"purpose,omitempty"`
	Explain           bool               `json:"explain,omitempty"`
	Binding           core.CareerBinding `json:"binding"`
	TargetPrincipalID string             `json:"target_principal_id"`
	Status            string             `json:"status"`
}

type StudioAccessFact struct {
	Explain      bool   `json:"explain,omitempty"`
	Version      string `json:"version"`
	GrantID      string `json:"grant_id"`
	PrincipalID  string `json:"principal_id"`
	CapabilityID string `json:"capability_id"`
	Status       string `json:"status"`
}

// ConfigureStudioAccessLocal is an explicit filesystem-administrator operation.
// It is deliberately absent from HTTP Service and MCP. The existing operator
// principal supplies attribution; possession of its bearer token is insufficient.
func (s *Store) ConfigureStudioAccessLocal(ctx context.Context, r StudioAccessRequest) (privateFactRecord[StudioAccessFact], error) {
	if (r.Purpose != "" && r.Purpose != "create_world") || (r.Purpose == "create_world" && r.Explain) {
		return privateFactRecord[StudioAccessFact]{}, core.NewError(core.CodeInvalidArgument, "unsupported Studio access purpose or fields")
	}
	if strings.TrimSpace(r.TargetPrincipalID) == "" || len(r.TargetPrincipalID) > 256 || (r.Status != "active" && r.Status != "revoked") {
		return privateFactRecord[StudioAccessFact]{}, core.NewError(core.CodeInvalidArgument, "existing target and active/revoked status required")
	}
	return executePrivateFactCommand(s, ctx, r.Binding, "ConfigureStudioAccessLocal", r, privateFactDomain{"studio_access", "StudioAccessConfigured", `{"authorization":"local-filesystem-operator-v1"}`}, func(conn *sql.Conn) error {
		var n int
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM principals WHERE principal_id=? AND principal_type='operator' AND status='active'`, r.Binding.PrincipalID).Scan(&n); err != nil {
			return err
		}
		if n != 1 {
			return core.NewError(core.CodeUnauthorized, "local setup requires an existing active operator identity")
		}
		return nil
	}, func(conn *sql.Conn, c privateFactContext) (StudioAccessFact, func() error, error) {
		var role string
		if err := conn.QueryRowContext(ctx, `SELECT principal_type FROM principals WHERE principal_id=? AND status='active'`, r.TargetPrincipalID).Scan(&role); err != nil {
			return StudioAccessFact{}, nil, classifyMissing(err, "active inspector target")
		}
		capability := "world.inspector.read"
		if r.Purpose == "create_world" {
			if role != "creator" {
				return StudioAccessFact{}, nil, core.NewError(core.CodeUnauthorized, "world creation target must be creator")
			}
			capability = "world.create"
		} else if role == "operator" {
			capability = "diagnostics.inspector.read"
		} else if role != "creator" {
			return StudioAccessFact{}, nil, core.NewError(core.CodeUnauthorized, "inspector target must be creator or operator")
		}
		hash, err := core.HashJSON([]string{r.Binding.InstanceID, r.Binding.BranchID, r.TargetPrincipalID, capability})
		if err != nil {
			return StudioAccessFact{}, nil, err
		}
		fact := StudioAccessFact{Explain: r.Explain, Version: "corerp.studio-access.v1", GrantID: "grant_studio_" + hash[7:], PrincipalID: r.TargetPrincipalID, CapabilityID: capability, Status: r.Status}
		return fact, func() error {
			return applyStudioAccess(ctx, conn, r.Binding.InstanceID, r.Binding.BranchID, c.EventID, fact)
		}, nil
	})
}

func validateStudioAccessFact(instance, branch string, f StudioAccessFact) error {
	if strings.TrimSpace(f.PrincipalID) == "" || len(f.PrincipalID) > 256 || f.Version != "corerp.studio-access.v1" || (f.CapabilityID != "world.inspector.read" && f.CapabilityID != "diagnostics.inspector.read" && f.CapabilityID != "world.create") || (f.CapabilityID == "world.create" && f.Explain) || (f.Status != "active" && f.Status != "revoked") {
		return core.NewError(core.CodeProjectionDiverged, "invalid inspector authority fact")
	}
	hash, err := core.HashJSON([]string{instance, branch, f.PrincipalID, f.CapabilityID})
	if err != nil {
		return err
	}
	if f.GrantID != "grant_studio_"+hash[7:] {
		return core.NewError(core.CodeProjectionDiverged, "inspector grant identity differs")
	}
	return nil
}

func applyStudioAccess(ctx context.Context, conn *sql.Conn, instance, branch, event string, f StudioAccessFact) error {
	if err := validateStudioAccessFact(instance, branch, f); err != nil {
		return err
	}
	description := "Read scoped Studio event and rule evidence"
	if f.CapabilityID == "world.create" {
		description = "Create new worlds from an explicitly delegated administrative scope"
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO capability_definitions VALUES (?,?,'studio-v1') ON CONFLICT(capability_id) DO NOTHING`, f.CapabilityID, description); err != nil {
		return err
	}
	_, err := conn.ExecContext(ctx, `INSERT INTO capability_grants(grant_id,principal_id,capability_id,instance_id,branch_id,subject_id,field_scope,status,definition_event_id)
	 VALUES (?,?,?,?,?,?,?,?,?) ON CONFLICT(grant_id) DO UPDATE SET
	 principal_id=excluded.principal_id,capability_id=excluded.capability_id,instance_id=excluded.instance_id,branch_id=excluded.branch_id,subject_id=excluded.subject_id,field_scope=excluded.field_scope,status=excluded.status,definition_event_id=excluded.definition_event_id,amount_limit_minor=NULL`, f.GrantID, f.PrincipalID, f.CapabilityID, instance, branch, branch, studioFactFields(f), f.Status, event)
	return err
}

func studioAccessFields(explain bool) string {
	if explain {
		return `["event","rule","explain"]`
	}
	return `["event","rule"]`
}

func studioFactFields(f StudioAccessFact) string {
	if f.CapabilityID == "world.create" {
		return `["genesis"]`
	}
	return studioAccessFields(f.Explain)
}

type studioGrantProjection struct {
	Fact        StudioAccessFact `json:"fact"`
	EventID     string           `json:"event_id"`
	SubjectID   string           `json:"subject_id"`
	FieldScope  string           `json:"field_scope"`
	AmountLimit *int64           `json:"amount_limit"`
}

func studioAccessDifferences(ctx context.Context, q replayQuerier, instance, branch string, through int64) ([]ProjectionDifference, error) {
	rows, err := q.QueryContext(ctx, `SELECT event_id,payload FROM events WHERE instance_id=? AND branch_id=? AND event_sequence<=? AND event_type='StudioAccessConfigured' ORDER BY event_sequence`, instance, branch, through)
	if err != nil {
		return nil, err
	}
	expected := map[string]string{}
	for rows.Next() {
		var id, raw string
		if err := rows.Scan(&id, &raw); err != nil {
			rows.Close()
			return nil, err
		}
		var f StudioAccessFact
		if err := json.Unmarshal([]byte(raw), &f); err != nil {
			rows.Close()
			return nil, err
		}
		if err := validateStudioAccessFact(instance, branch, f); err != nil {
			rows.Close()
			return nil, err
		}
		encoded, err := core.CanonicalJSON(studioGrantProjection{Fact: f, EventID: id, SubjectID: branch, FieldScope: studioFactFields(f)})
		if err != nil {
			rows.Close()
			return nil, err
		}
		expected[f.GrantID] = string(encoded)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	rows, err = q.QueryContext(ctx, `SELECT grant_id,principal_id,capability_id,status,definition_event_id,subject_id,field_scope,amount_limit_minor FROM capability_grants WHERE instance_id=? AND branch_id=? AND capability_id IN ('world.inspector.read','diagnostics.inspector.read','world.create')`, instance, branch)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	actual := map[string]string{}
	for rows.Next() {
		g := studioGrantProjection{Fact: StudioAccessFact{Version: "corerp.studio-access.v1"}}
		var limit sql.NullInt64
		if err := rows.Scan(&g.Fact.GrantID, &g.Fact.PrincipalID, &g.Fact.CapabilityID, &g.Fact.Status, &g.EventID, &g.SubjectID, &g.FieldScope, &limit); err != nil {
			return nil, err
		}
		if limit.Valid {
			g.AmountLimit = &limit.Int64
		}
		var fields []string
		if json.Unmarshal([]byte(g.FieldScope), &fields) == nil {
			for _, field := range fields {
				if field == "explain" {
					g.Fact.Explain = true
				}
			}
		}
		raw, err := core.CanonicalJSON(g)
		if err != nil {
			return nil, err
		}
		actual[g.Fact.GrantID] = string(raw)
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
	ordered := []string{}
	for k := range keys {
		ordered = append(ordered, k)
	}
	sort.Strings(ordered)
	var differences []ProjectionDifference
	for _, key := range ordered {
		if expected[key] != actual[key] {
			differences = append(differences, ProjectionDifference{Projection: "studio_access", Key: key, ExpectedText: expected[key], ActualText: actual[key]})
		}
	}
	return differences, nil
}

func repairStudioAccess(ctx context.Context, conn *sql.Conn, instance, branch string, differences []ProjectionDifference) error {
	for _, d := range differences {
		if d.ExpectedText == "" {
			if _, err := conn.ExecContext(ctx, `DELETE FROM capability_grants WHERE grant_id=? AND instance_id=? AND branch_id=? AND capability_id IN ('world.inspector.read','diagnostics.inspector.read','world.create')`, d.Key, instance, branch); err != nil {
				return err
			}
			continue
		}
	}
	for _, d := range differences {
		if d.ExpectedText == "" {
			continue
		}
		var g studioGrantProjection
		if err := json.Unmarshal([]byte(d.ExpectedText), &g); err != nil {
			return err
		}
		if err := applyStudioAccess(ctx, conn, instance, branch, g.EventID, g.Fact); err != nil {
			return err
		}
	}
	return nil
}
