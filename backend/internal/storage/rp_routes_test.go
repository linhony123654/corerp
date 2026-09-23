package storage

import (
	"context"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPTravelRoutesAreEventBackedIdempotentAndRecoverable(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "rp-routes.db")
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.BootstrapRPPlayDemo(ctx); err != nil {
		t.Fatal(err)
	}
	store.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "route precommit") }
	if _, err := store.PrepareRPTravel(ctx); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatalf("route setup did not roll back injected failure: %v", err)
	}
	store.beforeCommit = nil
	assertM2Value(t, ctx, store, `SELECT head_sequence FROM branches WHERE instance_id = ? AND branch_id = ?`, []any{M2DemoInstanceID, M2DemoBranchID}, 7)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM rp_place_links`, nil, 0)
	setup, err := store.PrepareRPTravel(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if setup.EventSequence != 8 || setup.LinkCount != 8 || setup.Replayed {
		t.Fatalf("unexpected route setup: %+v", setup)
	}
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM rp_place_links WHERE instance_id = ? AND branch_id = ?`, []any{M2DemoInstanceID, M2DemoBranchID}, 8)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM rp_place_links WHERE from_place_id = ? AND to_place_id = ?`, []any{"place_m2_home_ada", "place_m2_work_ada"}, 0)
	differences, err := store.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID)
	if err != nil || len(differences) != 0 {
		t.Fatalf("route definition disturbed world projections: %v, %v", differences, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	replay, err := reopened.PrepareRPTravel(ctx)
	if err != nil || !replay.Replayed || replay.EventSequence != setup.EventSequence {
		t.Fatalf("route setup retry changed authority: %+v, %v", replay, err)
	}
	assertM2Value(t, ctx, reopened, `SELECT COUNT(*) FROM rp_place_links`, nil, 8)
}
