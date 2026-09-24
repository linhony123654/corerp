package storage

import (
	"context"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPEnvironmentSourceRequiresActualPlaceStreamAndAuthority(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "environment.db")
	s := openCareerTestWorld(t, path)
	defer func() { s.Close() }()
	r := EnvironmentSourceRequest{Binding: careerTestBinding(t, s, "principal_creator", "rain-source"), Source: RPEnvironmentSource{PlaceID: M2AgentCafeID, RainBasisPoints: 2500, CooldownHours: 6}}
	if _, err := s.DefineRPEnvironmentSource(ctx, r); !core.HasCode(err, core.CodeNotFound) {
		t.Fatalf("missing stream: %v", err)
	}
	policy, err := s.DefineRPOpportunityPolicy(ctx, OpportunityPolicyRequest{Binding: careerTestBinding(t, s, "principal_creator", "stream"), Policy: RPOpportunityPolicy{StreamSeed: "shared-weather", ContactBasisPoints: 0, CooldownHours: 1, HistoryHours: 24}})
	if err != nil {
		t.Fatal(err)
	}
	r.Binding = careerTestBinding(t, s, "principal_creator", "rain-source")
	bad := r
	bad.Binding.PrincipalID = M2AgentBoPrincipal
	if _, err := s.DefineRPEnvironmentSource(ctx, bad); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("agent installed local weather: %v", err)
	}
	bad = r
	bad.Source.PlaceID = "nonexistent-place"
	if _, err := s.DefineRPEnvironmentSource(ctx, bad); !core.HasCode(err, core.CodeNotFound) {
		t.Fatalf("invented place: %v", err)
	}
	s.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "source rollback") }
	if _, err := s.DefineRPEnvironmentSource(ctx, r); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatalf("rollback: %v", err)
	}
	s.beforeCommit = nil
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='RPEnvironmentSourceDefined'`, nil, 0)
	created, err := s.DefineRPEnvironmentSource(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	if created.Fact.PolicyEventID != policy.EventID || created.Fact.PlaceSourceEventID == "" || created.Fact.BuilderSourceEventID == "" {
		t.Fatal("source lacks provenance")
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM outbox WHERE event_id=?`, []any{created.EventID}, 0)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_knowledge WHERE source_event_id=?`, []any{created.EventID}, 0)
	bad = r
	bad.Binding = careerTestBinding(t, s, "principal_creator", "fresh-source-key")
	if _, err := s.DefineRPEnvironmentSource(ctx, bad); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatalf("reinstalled place source: %v", err)
	}
	bad = r
	bad.Source.RainBasisPoints = 3000
	if _, err := s.DefineRPEnvironmentSource(ctx, bad); !core.HasCode(err, core.CodeIdempotencyMismatch) {
		t.Fatalf("changed retry: %v", err)
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
	retry, err := s.DefineRPEnvironmentSource(ctx, r)
	if err != nil || !retry.Replayed || retry.EventID != created.EventID || retry.Fact != created.Fact {
		t.Fatalf("source recovery: %+v %v", retry, err)
	}
	if differences, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatalf("source changed world projections: %+v %v", differences, err)
	}
}
