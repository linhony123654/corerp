package storage

import (
	"context"
	"corerp.local/backend/internal/core"
	"path/filepath"
	"testing"
)

func TestRPCultureAuthorityRevocationAndReassignmentSurviveRepair(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "culture-authority.db")
	s := openCareerTestWorld(t, path)
	defer func() { s.Close() }()
	scope, err := s.DefineRPCultureTerritory(ctx, CultureTerritoryRequest{Binding: careerTestBinding(t, s, "principal_creator", "scope"), ScopeKind: "world", ScopeID: M2DemoInstanceID, StewardID: M2AgentBoID})
	if err != nil {
		t.Fatal(err)
	}
	definition := CultureDefinitionRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "old-definition"), AuthorID: M2AgentBoID, Culture: core.RPCulture{CultureID: "old-custom", ScopeKind: "world", ScopeID: M2DemoInstanceID, GroupIdentity: "custom", Norms: []core.RPCultureNorm{{NormID: "gift", Action: "gift", Evaluation: 2}}}}
	original, err := s.DefineRPCulture(ctx, definition)
	if err != nil {
		t.Fatal(err)
	}
	change := CultureAuthorityRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "revoke"), ScopeKind: "world", ScopeID: M2DemoInstanceID, Decision: "revoke"}
	if _, err := s.ChangeRPCultureAuthority(ctx, change); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("steward altered builder authority: %v", err)
	}
	change.Binding = careerTestBinding(t, s, "principal_creator", "revoke")
	s.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "role rollback") }
	if _, err := s.ChangeRPCultureAuthority(ctx, change); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatalf("rollback: %v", err)
	}
	s.beforeCommit = nil
	if retry, err := s.DefineRPCulture(ctx, definition); err != nil || !retry.Replayed {
		t.Fatalf("rolled-back revocation took effect %+v %v", retry, err)
	}
	revoked, err := s.ChangeRPCultureAuthority(ctx, change)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DefineRPCulture(ctx, definition); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("old retry bypassed revocation: %v", err)
	}
	// Corrupt projection back to original installation: latest Event still wins
	// even before an explicit rebuild, so stale authority cannot be exercised.
	if _, err := s.db.Exec(`UPDATE capability_grants SET status='active',definition_event_id=? WHERE grant_id=?`, scope.EventID, scope.Fact.Territory.GrantID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DefineRPCulture(ctx, definition); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("stale installed grant bypassed later revocation: %v", err)
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
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM capability_grants WHERE grant_id=? AND status='revoked' AND definition_event_id=?`, []any{scope.Fact.Territory.GrantID, revoked.EventID}, 1)
	change.Binding = careerTestBinding(t, s, "principal_creator", "appoint")
	change.Decision = "appoint"
	change.StewardID = M2AgentAdaID
	appointed, err := s.ChangeRPCultureAuthority(ctx, change)
	if err != nil {
		t.Fatal(err)
	}
	next := definition
	next.Binding = careerTestBinding(t, s, M2AgentAdaPrincipal, "new-definition")
	next.AuthorID = M2AgentAdaID
	next.Culture.CultureID = "new-custom"
	if _, err := s.DefineRPCulture(ctx, next); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DefineRPCulture(ctx, definition); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("old steward regained authority: %v", err)
	}
	// Losing authority does not erase previously authored knowledge.
	if _, err := s.TransmitRPCulture(ctx, CultureTransmissionRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "old-news"), SpeakerID: M2AgentBoID, DefinitionEventID: original.EventID}); err != nil {
		t.Fatal(err)
	}
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM capability_grants WHERE grant_id=? AND principal_id=? AND status='active' AND definition_event_id=?`, []any{scope.Fact.Territory.GrantID, M2AgentAdaPrincipal, appointed.EventID}, 1)
	diffs, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID)
	if err != nil || len(diffs) != 0 {
		t.Fatalf("authority replay %+v %v", diffs, err)
	}
}
