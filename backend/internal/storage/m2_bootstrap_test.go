package storage

import (
	"context"
	"path/filepath"
	"testing"
)

func TestM2BootstrapIsAuthoritativeIsolatedAndIdempotent(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "m2-bootstrap.db")
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.BootstrapM2Demo(ctx); err != nil {
		t.Fatal(err)
	}
	assertM2BootstrapState(t, ctx, store)
	if err := store.BootstrapM2Demo(ctx); err != nil {
		t.Fatalf("repeat M2 bootstrap: %v", err)
	}
	assertM2BootstrapState(t, ctx, store)
	if err := store.BootstrapDemo(ctx); err != nil {
		t.Fatalf("M1 bootstrap after M2 bootstrap: %v", err)
	}
	assertM2BootstrapState(t, ctx, store)
	state, err := store.ReadDemoState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if state != initialState() {
		t.Fatalf("M2 instance contaminated scoped M1 state: got %+v want %+v", state, initialState())
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if err := reopened.BootstrapM2Demo(ctx); err != nil {
		t.Fatalf("M2 bootstrap after reopen: %v", err)
	}
	assertM2BootstrapState(t, ctx, reopened)
}

func assertM2BootstrapState(t *testing.T, ctx context.Context, store *Store) {
	t.Helper()
	checks := []struct {
		query    string
		args     []any
		expected int64
	}{
		{`SELECT head_sequence FROM branches WHERE instance_id = ? AND branch_id = ?`, []any{M2DemoInstanceID, M2DemoBranchID}, 1},
		{`SELECT COUNT(*) FROM events WHERE instance_id = ? AND branch_id = ? AND event_type = 'CohortInitialized'`, []any{M2DemoInstanceID, M2DemoBranchID}, 1},
		{`SELECT population_count FROM cohorts WHERE cohort_id = ?`, []any{M2DemoCohortID}, 20},
		{`SELECT balance_minor FROM account_balances WHERE account_id = ?`, []any{M2DemoCohortAssetAccountID}, 10000},
		{`SELECT balance_minor FROM account_balances WHERE account_id = ?`, []any{M2DemoCohortReceivableID}, 2000},
		{`SELECT balance_minor FROM account_balances WHERE account_id = ?`, []any{M2DemoCohortLiabilityID}, -1500},
		{`SELECT quantity_minor FROM inventory_balances WHERE location_id = ? AND sku_id = ?`, []any{M2DemoCohortLocationID, M2DemoSKUID}, 100},
		{`SELECT COUNT(*) FROM population_movements WHERE movement_kind = 'create' AND to_owner_id = ?`, []any{M2DemoCohortID}, 1},
		{`SELECT COUNT(*) FROM capability_grants WHERE instance_id = ? AND branch_id = ? AND subject_id = ?`, []any{M2DemoInstanceID, M2DemoBranchID, M2DemoCohortID}, 2},
	}
	for _, check := range checks {
		var actual int64
		if err := store.db.QueryRowContext(ctx, check.query, check.args...).Scan(&actual); err != nil {
			t.Fatalf("query %q: %v", check.query, err)
		}
		if actual != check.expected {
			t.Fatalf("query %q: got %d want %d", check.query, actual, check.expected)
		}
	}
	assertPostedJournalBalanced(t, ctx, store)
	assertSQLiteHealthy(t, ctx, store)
}
