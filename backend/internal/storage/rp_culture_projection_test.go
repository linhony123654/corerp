package storage

import (
	"context"
	"corerp.local/backend/internal/core"
	"path/filepath"
	"testing"
)

func TestRPCultureScopeGrantDamageDetectionAndRepair(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "culture-grants.db")
	s := openCareerTestWorld(t, path)
	defer func() { s.Close() }()
	territory, err := s.DefineRPCultureTerritory(ctx, CultureTerritoryRequest{Binding: careerTestBinding(t, s, "principal_creator", "world"), ScopeKind: "world", ScopeID: M2DemoInstanceID, StewardID: M2AgentBoID})
	if err != nil {
		t.Fatal(err)
	}
	grant := territory.Fact.Territory.GrantID
	var head int64
	if err := s.db.QueryRow(`SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&head); err != nil {
		t.Fatal(err)
	}
	for _, damage := range []string{
		`DELETE FROM capability_grants WHERE grant_id=?`,
		`UPDATE capability_grants SET principal_id='principal_m2_agent_ada',subject_id='region:wrong',field_scope='["private"]',amount_limit_minor=99,status='revoked' WHERE grant_id=?`,
		`UPDATE capability_grants SET capability_id='world.cohort.materialize' WHERE grant_id=?`,
	} {
		if _, err := s.db.Exec(damage, grant); err != nil {
			t.Fatal(err)
		}
		diffs, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, d := range diffs {
			if d.Projection == "culture_scope_grant" && d.Key == grant {
				found = true
			}
		}
		if !found {
			t.Fatalf("damage not detected: %+v", diffs)
		}
		if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
			t.Fatal(err)
		}
		diffs, err = s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID)
		if err != nil || len(diffs) != 0 {
			t.Fatalf("repair failed %+v %v", diffs, err)
		}
	}
	// Unsourced privilege must be diagnosed and removed, not promoted to truth.
	if _, err := s.db.Exec(`INSERT INTO capability_grants(grant_id,principal_id,capability_id,instance_id,branch_id,subject_id,field_scope,status,definition_event_id) VALUES ('bogus-culture-grant',?,?,?,?,'world:bogus','[]','active',?)`, M2AgentAdaPrincipal, cultureDefineCapability, M2DemoInstanceID, M2DemoBranchID, territory.EventID); err != nil {
		t.Fatal(err)
	}
	diffs, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, d := range diffs {
		if d.Key == "bogus-culture-grant" && d.ExpectedText == "" {
			found = true
		}
	}
	if !found {
		t.Fatal("unsourced grant not detected")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM capability_grants WHERE grant_id='bogus-culture-grant'`, nil, 0)
	assertM2Value(t, ctx, s, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, []any{M2DemoInstanceID, M2DemoBranchID}, head)
	// Repaired authority is actually usable by the original steward.
	_, err = s.DefineRPCulture(ctx, CultureDefinitionRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "culture"), AuthorID: M2AgentBoID, Culture: core.RPCulture{CultureID: "world-norms", ScopeKind: "world", ScopeID: M2DemoInstanceID, GroupIdentity: "world", Norms: []core.RPCultureNorm{{NormID: "gift", Action: "gift", Evaluation: 2}}}})
	if err != nil {
		t.Fatal(err)
	}
}
