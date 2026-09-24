package storage

import (
	"context"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPTransitWorksSourceAuthorityIntervalAndRecovery(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "transit-source.db")
	s := openCareerTestWorld(t, path)
	defer func() { s.Close() }()
	r := TransitWorksRequest{Binding: careerTestBinding(t, s, "principal_creator", "roadworks"), FromPlaceID: M2AgentCafeID, ToPlaceID: "place_m2_work_ada", StartsAt: "2026-09-23T07:30:00Z", EndsAt: "2026-09-23T09:00:00Z"}
	bad := r
	bad.Binding.PrincipalID = M2AgentBoPrincipal
	if _, err := s.DefineRPTransitWorks(ctx, bad); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("actor declared works: %v", err)
	}
	bad = r
	bad.ToPlaceID = "place_m2_cafe_missing"
	if _, err := s.DefineRPTransitWorks(ctx, bad); !core.HasCode(err, core.CodeNotFound) {
		t.Fatalf("fabricated route: %v", err)
	}
	bad = r
	bad.StartsAt = "2026-09-22T07:00:00Z"
	bad.EndsAt = "2026-09-22T08:00:00Z"
	if _, err := s.DefineRPTransitWorks(ctx, bad); !core.HasCode(err, core.CodeInvalidArgument) {
		t.Fatalf("retroactive works: %v", err)
	}
	s.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "transit rollback") }
	if _, err := s.DefineRPTransitWorks(ctx, r); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatalf("rollback: %v", err)
	}
	s.beforeCommit = nil
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='RPTransitWorksDefined'`, nil, 0)
	created, err := s.DefineRPTransitWorks(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	if created.Fact.Window.SourceEventID != created.EventID || created.Fact.ForwardRouteSourceEventID != m2RPTravelEventID || created.Fact.ReverseRouteSourceEventID != m2RPTravelEventID || created.Fact.BuilderSourceEventID == "" {
		t.Fatal("missing route provenance")
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM outbox WHERE event_id=?`, []any{created.EventID}, 0)
	bad = r
	bad.EndsAt = "2026-09-23T10:00:00Z"
	if _, err := s.DefineRPTransitWorks(ctx, bad); !core.HasCode(err, core.CodeIdempotencyMismatch) {
		t.Fatalf("changed retry: %v", err)
	}
	bad = r
	bad.Binding = careerTestBinding(t, s, "principal_creator", "reversed-overlap")
	bad.FromPlaceID, bad.ToPlaceID = r.ToPlaceID, r.FromPlaceID
	bad.StartsAt = "2026-09-23T09:30:00Z"
	bad.EndsAt = "2026-09-23T10:00:00Z"
	if _, err := s.DefineRPTransitWorks(ctx, bad); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatalf("reverse-route cooldown bypass: %v", err)
	}
	bad.StartsAt = "2026-09-23T10:00:00Z"
	bad.EndsAt = "2026-09-23T10:30:00Z"
	if _, err := s.DefineRPTransitWorks(ctx, bad); err != nil {
		t.Fatalf("exact cooldown boundary: %v", err)
	}
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		t.Fatal(err)
	}
	arrival, err := readRPTransitArrival(ctx, tx.conn, M2DemoInstanceID, M2DemoBranchID, "place_m2_home_ada", "place_m2_work_ada", "2026-09-23T08:00:00Z")
	tx.Rollback(ctx)
	if err != nil || !arrival.Reachable || arrival.WorldTime != "2026-09-23T09:00:00Z" || len(arrival.Path) != 3 || len(arrival.DelaySourceEventIDs) != 1 || arrival.DelaySourceEventIDs[0] != created.EventID {
		t.Fatalf("actual topology/window planning: %+v %v", arrival, err)
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
	retry, err := s.DefineRPTransitWorks(ctx, r)
	if err != nil || !retry.Replayed || retry.EventID != created.EventID || retry.Fact != created.Fact {
		t.Fatalf("works recovery: %+v %v", retry, err)
	}
	if differences, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatalf("works projection differences: %+v %v", differences, err)
	}
}
