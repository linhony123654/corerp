package storage

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestMaterializeCohortConservesAllDimensionsAndReplays(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "materialize.db")
	store := openM2Store(t, ctx, path)
	command := demoMaterializeCommand()

	result, err := store.MaterializeCohort(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	if result.Replayed || result.FirstSequence != 2 || result.MaterializationID != command.MaterializationID || result.EntityID != command.EntityID {
		t.Fatalf("unexpected materialization result: %+v", result)
	}
	assertMaterializedState(t, ctx, store, command)

	retry := command
	retry.CommandID = "cmd_materialize_retry"
	replayed, err := store.MaterializeCohort(ctx, retry)
	if err != nil {
		t.Fatal(err)
	}
	if !replayed.Replayed || replayed.CommandID != command.CommandID || replayed.EventID != result.EventID {
		t.Fatalf("command-key retry did not return original result: %+v", replayed)
	}
	byMaterialization := command
	byMaterialization.CommandID = "cmd_materialize_identity_retry"
	byMaterialization.IdempotencyKey = "idem_materialize_identity_retry"
	replayed, err = store.MaterializeCohort(ctx, byMaterialization)
	if err != nil {
		t.Fatal(err)
	}
	if !replayed.Replayed || replayed.CommandID != command.CommandID {
		t.Fatalf("materialization identity retry did not return original result: %+v", replayed)
	}
	assertMaterializedState(t, ctx, store, command)

	mismatch := byMaterialization
	mismatch.CommandID = "cmd_materialize_identity_conflict"
	mismatch.IdempotencyKey = "idem_materialize_identity_conflict"
	mismatch.AssetMinor++
	if _, err := store.MaterializeCohort(ctx, mismatch); !core.HasCode(err, core.CodeMaterializationConflict) {
		t.Fatalf("materialization payload conflict should fail explicitly, got %v", err)
	}
	idempotencyMismatch := retry
	idempotencyMismatch.AssetMinor++
	if _, err := store.MaterializeCohort(ctx, idempotencyMismatch); !core.HasCode(err, core.CodeIdempotencyMismatch) {
		t.Fatalf("idempotency payload conflict should fail explicitly, got %v", err)
	}
	assertMaterializedState(t, ctx, store, command)

	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	assertMaterializedState(t, ctx, reopened, command)
}

func TestCohortTransitionsRejectBackdatingAndSkippedDueWork(t *testing.T) {
	ctx := context.Background()
	store := openM2AgentStore(t, ctx, filepath.Join(t.TempDir(), "cohort-chronology.db"), false)
	defer store.Close()
	backdated := m2AgentMaterialization("backdated", "entity_m2_backdated", "Backdated", 1, 100, 1, 20, 10, 4, "2026-09-22T01:59:00Z")
	if _, err := store.MaterializeCohort(ctx, backdated); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatalf("materialization predating Agent setup should conflict: %v", err)
	}
	due := m2AgentMaterialization("due", "entity_m2_due", "Due", 1, 100, 1, 20, 10, 4, M2AgentMorningTime)
	if _, err := store.MaterializeCohort(ctx, due); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatalf("materialization must not skip pending same-time movement: %v", err)
	}
	assertM2Value(t, ctx, store, `SELECT head_sequence FROM branches WHERE instance_id = ? AND branch_id = ?`, []any{M2DemoInstanceID, M2DemoBranchID}, 4)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM materialized_entities WHERE entity_id IN (?, ?)`, []any{backdated.EntityID, due.EntityID}, 0)
	equalInstant := m2AgentMaterialization("offset", "entity_m2_offset", "Offset", 1, 100, 1, 20, 10, 4, "2026-09-22T10:00:00+08:00")
	if _, err := store.MaterializeCohort(ctx, equalInstant); err != nil {
		t.Fatalf("equivalent RFC3339 offset should share the committed instant: %v", err)
	}
	assertM2Value(t, ctx, store, `SELECT head_sequence FROM branches WHERE instance_id = ? AND branch_id = ?`, []any{M2DemoInstanceID, M2DemoBranchID}, 5)

	plain := openM2Store(t, ctx, filepath.Join(t.TempDir(), "cohort-dematerialize-chronology.db"))
	defer plain.Close()
	if _, err := plain.MaterializeCohort(ctx, demoMaterializeCommand()); err != nil {
		t.Fatal(err)
	}
	undo := demoDematerializeCommand()
	undo.WorldTime = "2026-09-22T00:59:00Z"
	if _, err := plain.DematerializeCohort(ctx, undo); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatalf("dematerialization predating materialization should conflict: %v", err)
	}
	assertM2Value(t, ctx, plain, `SELECT head_sequence FROM branches WHERE instance_id = ? AND branch_id = ?`, []any{M2DemoInstanceID, M2DemoBranchID}, 2)
}

func TestMaterializeCohortFailuresAndInjectedRollbackLeaveNoWrites(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*core.MaterializeCohortCommand)
		code   core.ErrorCode
	}{
		{"unauthorized", func(command *core.MaterializeCohortCommand) { command.PrincipalID = "principal_intruder" }, core.CodeUnauthorized},
		{"stale head", func(command *core.MaterializeCohortCommand) { command.ExpectedHead = 0 }, core.CodeBranchConflict},
		{"population", func(command *core.MaterializeCohortCommand) { command.PopulationCount = 21 }, core.CodeConservationFailed},
		{"asset", func(command *core.MaterializeCohortCommand) { command.AssetMinor = 10001 }, core.CodeConservationFailed},
		{"inventory", func(command *core.MaterializeCohortCommand) { command.InventoryMinor = 101 }, core.CodeConservationFailed},
		{"receivable", func(command *core.MaterializeCohortCommand) { command.ReceivableMinor = 2001 }, core.CodeConservationFailed},
		{"liability", func(command *core.MaterializeCohortCommand) { command.LiabilityMinor = 1501 }, core.CodeConservationFailed},
		{"algorithm", func(command *core.MaterializeCohortCommand) { command.AllocationAlgorithmVersion = "unknown-v2" }, core.CodeConservationFailed},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			store := openM2Store(t, ctx, filepath.Join(t.TempDir(), "failure.db"))
			defer store.Close()
			command := demoMaterializeCommand()
			test.mutate(&command)
			if _, err := store.MaterializeCohort(ctx, command); !core.HasCode(err, test.code) {
				t.Fatalf("got %v want %s", err, test.code)
			}
			assertUnmaterializedState(t, ctx, store)
		})
	}

	t.Run("pre-commit rollback", func(t *testing.T) {
		ctx := context.Background()
		store := openM2Store(t, ctx, filepath.Join(t.TempDir(), "rollback.db"))
		defer store.Close()
		store.beforeCommit = func() error {
			return core.NewError(core.CodeInjectedFailure, "materialization pre-commit failure")
		}
		if _, err := store.MaterializeCohort(ctx, demoMaterializeCommand()); !core.HasCode(err, core.CodeInjectedFailure) {
			t.Fatalf("got %v want injected failure", err)
		}
		store.beforeCommit = nil
		assertUnmaterializedState(t, ctx, store)
	})
}

func TestDematerializeCohortRestoresConservationAndSnapshotReplay(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "dematerialize.db")
	store := openM2Store(t, ctx, path)
	materialize := demoMaterializeCommand()
	if _, err := store.MaterializeCohort(ctx, materialize); err != nil {
		t.Fatal(err)
	}
	before, err := store.Replay(ctx, M2DemoInstanceID, M2DemoBranchID, 2)
	if err != nil {
		t.Fatal(err)
	}
	if differences, err := store.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatalf("materialized projections differ before snapshot: differences=%v err=%v", differences, err)
	}
	if _, err := store.CreateSnapshot(ctx, M2DemoInstanceID, M2DemoBranchID, 2); err != nil {
		t.Fatal(err)
	}

	command := demoDematerializeCommand()
	result, err := store.DematerializeCohort(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	if result.Replayed || result.FirstSequence != 3 || result.MaterializationID != materialize.MaterializationID {
		t.Fatalf("unexpected dematerialization result: %+v", result)
	}
	assertDematerializedState(t, ctx, store, materialize)
	full, err := store.Replay(ctx, M2DemoInstanceID, M2DemoBranchID, 3)
	if err != nil {
		t.Fatal(err)
	}
	fromSnapshot, err := store.ReplayFromLatestSnapshot(ctx, M2DemoInstanceID, M2DemoBranchID, 3)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(full.State, fromSnapshot.State) || full.StateHash != fromSnapshot.StateHash {
		t.Fatalf("snapshot continuation differs from empty replay: full=%+v snapshot=%+v", full, fromSnapshot)
	}
	_, _, populations := replayMaps(full.State)
	if populations[populationKey("cohort", M2DemoCohortID)] != 20 || populations[populationKey("entity", materialize.EntityID)] != 0 {
		t.Fatalf("unexpected replayed populations: %v", populations)
	}
	if before.StateHash == full.StateHash {
		t.Fatal("materialized and dematerialized authority states unexpectedly have the same high-water state hash")
	}

	retry := command
	retry.CommandID = "cmd_dematerialize_retry"
	replayed, err := store.DematerializeCohort(ctx, retry)
	if err != nil {
		t.Fatal(err)
	}
	if !replayed.Replayed || replayed.CommandID != command.CommandID || replayed.EventID != result.EventID {
		t.Fatalf("dematerialization retry did not return original result: %+v", replayed)
	}
	idempotencyMismatch := retry
	idempotencyMismatch.ReasonCode = "different_reason"
	if _, err := store.DematerializeCohort(ctx, idempotencyMismatch); !core.HasCode(err, core.CodeIdempotencyMismatch) {
		t.Fatalf("dematerialization idempotency mismatch should fail, got %v", err)
	}
	second := command
	second.CommandID = "cmd_dematerialize_second"
	second.IdempotencyKey = "idem_dematerialize_second"
	second.ExpectedHead = 3
	if _, err := store.DematerializeCohort(ctx, second); !core.HasCode(err, core.CodeMaterializationConflict) {
		t.Fatalf("second dematerialization should be rejected, got %v", err)
	}
	assertDematerializedState(t, ctx, store, materialize)

	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	assertDematerializedState(t, ctx, reopened, materialize)
}

func TestDematerializeAuthorizationAndHeadFailuresLeaveNoWrites(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*core.DematerializeCohortCommand)
		code   core.ErrorCode
	}{
		{"unauthorized", func(command *core.DematerializeCohortCommand) { command.PrincipalID = "principal_intruder" }, core.CodeUnauthorized},
		{"stale head", func(command *core.DematerializeCohortCommand) { command.ExpectedHead = 1 }, core.CodeBranchConflict},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			store := openM2Store(t, ctx, filepath.Join(t.TempDir(), "dematerialize-failure.db"))
			defer store.Close()
			materialize := demoMaterializeCommand()
			if _, err := store.MaterializeCohort(ctx, materialize); err != nil {
				t.Fatal(err)
			}
			command := demoDematerializeCommand()
			test.mutate(&command)
			if _, err := store.DematerializeCohort(ctx, command); !core.HasCode(err, test.code) {
				t.Fatalf("got %v want %s", err, test.code)
			}
			assertMaterializedState(t, ctx, store, materialize)
		})
	}
}

func TestDematerializeRejectsChangedNamedHoldingsAndRollsBack(t *testing.T) {
	mutations := []struct {
		name, query string
		args        func(core.MaterializeCohortCommand) []any
	}{
		{"population", `UPDATE materialized_entities SET population_count = population_count + 1 WHERE entity_id = ?`, func(c core.MaterializeCohortCommand) []any { return []any{c.EntityID} }},
		{"asset", `UPDATE account_balances SET balance_minor = balance_minor + 1 WHERE account_id = ?`, func(c core.MaterializeCohortCommand) []any { return []any{"account_" + c.MaterializationID + "_asset"} }},
		{"receivable", `UPDATE account_balances SET balance_minor = balance_minor + 1 WHERE account_id = ?`, func(c core.MaterializeCohortCommand) []any {
			return []any{"account_" + c.MaterializationID + "_receivable"}
		}},
		{"liability", `UPDATE account_balances SET balance_minor = balance_minor - 1 WHERE account_id = ?`, func(c core.MaterializeCohortCommand) []any {
			return []any{"account_" + c.MaterializationID + "_liability"}
		}},
		{"inventory", `UPDATE inventory_balances SET quantity_minor = quantity_minor + 1 WHERE location_id = ? AND sku_id = ?`, func(c core.MaterializeCohortCommand) []any {
			return []any{"location_" + c.MaterializationID, M2DemoSKUID}
		}},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			ctx := context.Background()
			store := openM2Store(t, ctx, filepath.Join(t.TempDir(), "changed.db"))
			defer store.Close()
			materialize := demoMaterializeCommand()
			if _, err := store.MaterializeCohort(ctx, materialize); err != nil {
				t.Fatal(err)
			}
			if _, err := store.db.ExecContext(ctx, mutation.query, mutation.args(materialize)...); err != nil {
				t.Fatal(err)
			}
			if _, err := store.DematerializeCohort(ctx, demoDematerializeCommand()); !core.HasCode(err, core.CodeConservationFailed) {
				t.Fatalf("changed %s should block dematerialization, got %v", mutation.name, err)
			}
			assertM2Value(t, ctx, store, `SELECT head_sequence FROM branches WHERE instance_id = ? AND branch_id = ?`, []any{M2DemoInstanceID, M2DemoBranchID}, 2)
			assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM events WHERE instance_id = ? AND branch_id = ?`, []any{M2DemoInstanceID, M2DemoBranchID}, 2)
			assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM cohort_materializations WHERE status = 'active'`, nil, 1)
		})
	}

	t.Run("pre-commit rollback", func(t *testing.T) {
		ctx := context.Background()
		store := openM2Store(t, ctx, filepath.Join(t.TempDir(), "rollback.db"))
		defer store.Close()
		materialize := demoMaterializeCommand()
		if _, err := store.MaterializeCohort(ctx, materialize); err != nil {
			t.Fatal(err)
		}
		store.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "dematerialization pre-commit failure") }
		if _, err := store.DematerializeCohort(ctx, demoDematerializeCommand()); !core.HasCode(err, core.CodeInjectedFailure) {
			t.Fatalf("got %v want injected failure", err)
		}
		store.beforeCommit = nil
		assertMaterializedState(t, ctx, store, materialize)
	})
}

func TestM2PopulationProjectionDivergenceIsDetectedAndRepaired(t *testing.T) {
	ctx := context.Background()
	store := openM2Store(t, ctx, filepath.Join(t.TempDir(), "population-repair.db"))
	defer store.Close()
	if _, err := store.MaterializeCohort(ctx, demoMaterializeCommand()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE cohorts SET population_count = 999, status = 'depleted' WHERE cohort_id = ?`, M2DemoCohortID); err != nil {
		t.Fatal(err)
	}
	differences, err := store.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, difference := range differences {
		if difference.Projection == "population_balance" && difference.Key == populationKey("cohort", M2DemoCohortID) && difference.Expected == 19 && difference.Actual == 999 {
			found = true
		}
	}
	if !found {
		t.Fatalf("population divergence was not reported: %+v", differences)
	}
	statusFound := false
	for _, difference := range differences {
		if difference.Projection == "population_status" && difference.Key == populationKey("cohort", M2DemoCohortID) && difference.ExpectedText == "active" && difference.ActualText == "depleted" {
			statusFound = true
		}
	}
	if !statusFound {
		t.Fatalf("population lifecycle divergence was not reported: %+v", differences)
	}
	if err := store.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	if differences, err := store.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatalf("population projection repair incomplete: differences=%v err=%v", differences, err)
	}
}

func openM2Store(t *testing.T, ctx context.Context, path string) *Store {
	t.Helper()
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.BootstrapM2Demo(ctx); err != nil {
		store.Close()
		t.Fatal(err)
	}
	return store
}

func demoMaterializeCommand() core.MaterializeCohortCommand {
	return core.MaterializeCohortCommand{
		CommandID: "cmd_materialize_1", MaterializationID: "mat_block_a_person_1",
		InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID,
		PrincipalID: "principal_creator", CapabilityID: "world.cohort.materialize",
		IdempotencyKey: "idem_materialize_1", ExpectedHead: 1, WorldTime: "2026-09-22T01:00:00Z",
		SourceCohortID: M2DemoCohortID, EntityID: "entity_m2_person_1", DisplayName: "M2 Person One",
		PopulationCount: 1, AssetMinor: 1000, InventoryMinor: 10,
		ReceivableMinor: 200, LiabilityMinor: 150, AllocationAlgorithmVersion: "equal-share-v1",
	}
}

func demoDematerializeCommand() core.DematerializeCohortCommand {
	return core.DematerializeCohortCommand{
		CommandID: "cmd_dematerialize_1", MaterializationID: "mat_block_a_person_1",
		InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID,
		PrincipalID: "principal_creator", CapabilityID: "world.cohort.dematerialize",
		IdempotencyKey: "idem_dematerialize_1", ExpectedHead: 2,
		WorldTime: "2026-09-22T02:00:00Z", ReasonCode: "lod_return",
	}
}

func assertUnmaterializedState(t *testing.T, ctx context.Context, store *Store) {
	t.Helper()
	assertM2Value(t, ctx, store, `SELECT head_sequence FROM branches WHERE instance_id = ? AND branch_id = ?`, []any{M2DemoInstanceID, M2DemoBranchID}, 1)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM commands WHERE instance_id = ? AND branch_id = ?`, []any{M2DemoInstanceID, M2DemoBranchID}, 1)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM events WHERE instance_id = ? AND branch_id = ?`, []any{M2DemoInstanceID, M2DemoBranchID}, 1)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM cohort_materializations`, nil, 0)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM materialized_entities`, nil, 0)
	assertM2Value(t, ctx, store, `SELECT population_count FROM cohorts WHERE cohort_id = ?`, []any{M2DemoCohortID}, 20)
	assertM2Value(t, ctx, store, `SELECT balance_minor FROM account_balances WHERE account_id = ?`, []any{M2DemoCohortAssetAccountID}, 10000)
	assertM2Value(t, ctx, store, `SELECT quantity_minor FROM inventory_balances WHERE location_id = ? AND sku_id = ?`, []any{M2DemoCohortLocationID, M2DemoSKUID}, 100)
	assertSQLiteHealthy(t, ctx, store)
}

func assertMaterializedState(t *testing.T, ctx context.Context, store *Store, command core.MaterializeCohortCommand) {
	t.Helper()
	entityAsset := "account_" + command.MaterializationID + "_asset"
	entityReceivable := "account_" + command.MaterializationID + "_receivable"
	entityLiability := "account_" + command.MaterializationID + "_liability"
	entityLocation := "location_" + command.MaterializationID
	checks := []struct {
		query    string
		args     []any
		expected int64
	}{
		{`SELECT head_sequence FROM branches WHERE instance_id = ? AND branch_id = ?`, []any{M2DemoInstanceID, M2DemoBranchID}, 2},
		{`SELECT population_count FROM cohorts WHERE cohort_id = ?`, []any{M2DemoCohortID}, 19},
		{`SELECT population_count FROM materialized_entities WHERE entity_id = ? AND status = 'active'`, []any{command.EntityID}, 1},
		{`SELECT balance_minor FROM account_balances WHERE account_id = ?`, []any{M2DemoCohortAssetAccountID}, 9000},
		{`SELECT balance_minor FROM account_balances WHERE account_id = ?`, []any{entityAsset}, 1000},
		{`SELECT balance_minor FROM account_balances WHERE account_id = ?`, []any{M2DemoCohortReceivableID}, 1800},
		{`SELECT balance_minor FROM account_balances WHERE account_id = ?`, []any{entityReceivable}, 200},
		{`SELECT balance_minor FROM account_balances WHERE account_id = ?`, []any{M2DemoCohortLiabilityID}, -1350},
		{`SELECT balance_minor FROM account_balances WHERE account_id = ?`, []any{entityLiability}, -150},
		{`SELECT quantity_minor FROM inventory_balances WHERE location_id = ? AND sku_id = ?`, []any{M2DemoCohortLocationID, M2DemoSKUID}, 90},
		{`SELECT quantity_minor FROM inventory_balances WHERE location_id = ? AND sku_id = ?`, []any{entityLocation, M2DemoSKUID}, 10},
		{`SELECT COUNT(*) FROM cohort_materializations WHERE materialization_id = ? AND status = 'active'`, []any{command.MaterializationID}, 1},
		{`SELECT COUNT(*) FROM population_movements WHERE materialization_id = ? AND movement_kind = 'materialize'`, []any{command.MaterializationID}, 1},
		{`SELECT COUNT(*) FROM commands WHERE instance_id = ? AND branch_id = ?`, []any{M2DemoInstanceID, M2DemoBranchID}, 2},
		{`SELECT COUNT(*) FROM events WHERE instance_id = ? AND branch_id = ?`, []any{M2DemoInstanceID, M2DemoBranchID}, 2},
	}
	for _, check := range checks {
		assertM2Value(t, ctx, store, check.query, check.args, check.expected)
	}
	assertM2Value(t, ctx, store, `SELECT
		(SELECT population_count FROM cohorts WHERE cohort_id = ?) +
		(SELECT population_count FROM materialized_entities WHERE entity_id = ?)`, []any{M2DemoCohortID, command.EntityID}, 20)
	assertM2Value(t, ctx, store, `SELECT
		(SELECT balance_minor FROM account_balances WHERE account_id = ?) +
		(SELECT balance_minor FROM account_balances WHERE account_id = ?)`, []any{M2DemoCohortAssetAccountID, entityAsset}, 10000)
	assertM2Value(t, ctx, store, `SELECT
		(SELECT balance_minor FROM account_balances WHERE account_id = ?) +
		(SELECT balance_minor FROM account_balances WHERE account_id = ?)`, []any{M2DemoCohortReceivableID, entityReceivable}, 2000)
	assertM2Value(t, ctx, store, `SELECT
		(SELECT balance_minor FROM account_balances WHERE account_id = ?) +
		(SELECT balance_minor FROM account_balances WHERE account_id = ?)`, []any{M2DemoCohortLiabilityID, entityLiability}, -1500)
	assertM2Value(t, ctx, store, `SELECT
		(SELECT quantity_minor FROM inventory_balances WHERE location_id = ? AND sku_id = ?) +
		(SELECT quantity_minor FROM inventory_balances WHERE location_id = ? AND sku_id = ?)`, []any{M2DemoCohortLocationID, M2DemoSKUID, entityLocation, M2DemoSKUID}, 100)
	assertPostedJournalBalanced(t, ctx, store)
	assertSQLiteHealthy(t, ctx, store)
}

func assertDematerializedState(t *testing.T, ctx context.Context, store *Store, command core.MaterializeCohortCommand) {
	t.Helper()
	entityAsset := "account_" + command.MaterializationID + "_asset"
	entityReceivable := "account_" + command.MaterializationID + "_receivable"
	entityLiability := "account_" + command.MaterializationID + "_liability"
	entityLocation := "location_" + command.MaterializationID
	checks := []struct {
		query    string
		args     []any
		expected int64
	}{
		{`SELECT head_sequence FROM branches WHERE instance_id = ? AND branch_id = ?`, []any{M2DemoInstanceID, M2DemoBranchID}, 3},
		{`SELECT population_count FROM cohorts WHERE cohort_id = ?`, []any{M2DemoCohortID}, 20},
		{`SELECT population_count FROM materialized_entities WHERE entity_id = ? AND status = 'dematerialized'`, []any{command.EntityID}, 0},
		{`SELECT balance_minor FROM account_balances WHERE account_id = ?`, []any{M2DemoCohortAssetAccountID}, 10000},
		{`SELECT balance_minor FROM account_balances WHERE account_id = ?`, []any{entityAsset}, 0},
		{`SELECT balance_minor FROM account_balances WHERE account_id = ?`, []any{M2DemoCohortReceivableID}, 2000},
		{`SELECT balance_minor FROM account_balances WHERE account_id = ?`, []any{entityReceivable}, 0},
		{`SELECT balance_minor FROM account_balances WHERE account_id = ?`, []any{M2DemoCohortLiabilityID}, -1500},
		{`SELECT balance_minor FROM account_balances WHERE account_id = ?`, []any{entityLiability}, 0},
		{`SELECT quantity_minor FROM inventory_balances WHERE location_id = ? AND sku_id = ?`, []any{M2DemoCohortLocationID, M2DemoSKUID}, 100},
		{`SELECT quantity_minor FROM inventory_balances WHERE location_id = ? AND sku_id = ?`, []any{entityLocation, M2DemoSKUID}, 0},
		{`SELECT COUNT(*) FROM cohort_materializations WHERE materialization_id = ? AND status = 'dematerialized' AND dematerialize_sequence = 3`, []any{command.MaterializationID}, 1},
		{`SELECT COUNT(*) FROM population_movements WHERE materialization_id = ?`, []any{command.MaterializationID}, 2},
		{`SELECT COUNT(*) FROM commands WHERE instance_id = ? AND branch_id = ?`, []any{M2DemoInstanceID, M2DemoBranchID}, 3},
		{`SELECT COUNT(*) FROM events WHERE instance_id = ? AND branch_id = ?`, []any{M2DemoInstanceID, M2DemoBranchID}, 3},
	}
	for _, check := range checks {
		assertM2Value(t, ctx, store, check.query, check.args, check.expected)
	}
	assertPostedJournalBalanced(t, ctx, store)
	assertSQLiteHealthy(t, ctx, store)
}

func assertM2Value(t *testing.T, ctx context.Context, store *Store, query string, args []any, expected int64) {
	t.Helper()
	var actual int64
	if err := store.db.QueryRowContext(ctx, query, args...).Scan(&actual); err != nil {
		t.Fatalf("query %q: %v", query, err)
	}
	if actual != expected {
		t.Fatalf("query %q: got %d want %d", query, actual, expected)
	}
}
