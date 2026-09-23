package storage

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestEmbeddedMigrationsMatchContracts(t *testing.T) {
	embedded, err := migrationFiles.ReadFile("migrations/001_init.sql")
	if err != nil {
		t.Fatal(err)
	}
	contract, err := os.ReadFile(filepath.Join("..", "..", "..", "docs", "m0", "schema.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(embedded, contract) {
		t.Fatal("embedded migration diverges from docs/m0/schema.sql")
	}
	embedded, err = migrationFiles.ReadFile("migrations/002_recovery.sql")
	if err != nil {
		t.Fatal(err)
	}
	contract, err = os.ReadFile(filepath.Join("..", "..", "..", "docs", "m1", "schema-002-recovery.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(embedded, contract) {
		t.Fatal("embedded migration diverges from docs/m1/schema-002-recovery.sql")
	}
	embedded, err = migrationFiles.ReadFile("migrations/003_strict_world.sql")
	if err != nil {
		t.Fatal(err)
	}
	contract, err = os.ReadFile(filepath.Join("..", "..", "..", "docs", "m1", "schema-003-strict-world.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(embedded, contract) {
		t.Fatal("embedded migration diverges from docs/m1/schema-003-strict-world.sql")
	}
	embedded, err = migrationFiles.ReadFile("migrations/004_obligation_accounting.sql")
	if err != nil {
		t.Fatal(err)
	}
	contract, err = os.ReadFile(filepath.Join("..", "..", "..", "docs", "m1", "schema-004-obligation-accounting.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(embedded, contract) {
		t.Fatal("embedded migration diverges from docs/m1/schema-004-obligation-accounting.sql")
	}
	embedded, err = migrationFiles.ReadFile("migrations/005_authorization_issuance.sql")
	if err != nil {
		t.Fatal(err)
	}
	contract, err = os.ReadFile(filepath.Join("..", "..", "..", "docs", "m1", "schema-005-authorization-issuance.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(embedded, contract) {
		t.Fatal("embedded migration diverges from docs/m1/schema-005-authorization-issuance.sql")
	}
	embedded, err = migrationFiles.ReadFile("migrations/006_cohort_materialization.sql")
	if err != nil {
		t.Fatal(err)
	}
	contract, err = os.ReadFile(filepath.Join("..", "..", "..", "docs", "m2", "schema-006-cohort-materialization.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(embedded, contract) {
		t.Fatal("embedded migration diverges from docs/m2/schema-006-cohort-materialization.sql")
	}
	embedded, err = migrationFiles.ReadFile("migrations/007_agent_life.sql")
	if err != nil {
		t.Fatal(err)
	}
	contract, err = os.ReadFile(filepath.Join("..", "..", "..", "docs", "m2", "schema-007-agent-life.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(embedded, contract) {
		t.Fatal("embedded migration diverges from docs/m2/schema-007-agent-life.sql")
	}
	embedded, err = migrationFiles.ReadFile("migrations/008_m2_economy.sql")
	if err != nil {
		t.Fatal(err)
	}
	contract, err = os.ReadFile(filepath.Join("..", "..", "..", "docs", "m2", "schema-008-background-economy.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(embedded, contract) {
		t.Fatal("embedded migration diverges from docs/m2/schema-008-background-economy.sql")
	}
	embedded, err = migrationFiles.ReadFile("migrations/009_m2_store.sql")
	if err != nil {
		t.Fatal(err)
	}
	contract, err = os.ReadFile(filepath.Join("..", "..", "..", "docs", "m2", "schema-009-finite-store.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(embedded, contract) {
		t.Fatal("embedded migration diverges from docs/m2/schema-009-finite-store.sql")
	}
	embedded, err = migrationFiles.ReadFile("migrations/010_m2_consumption_supply.sql")
	if err != nil {
		t.Fatal(err)
	}
	contract, err = os.ReadFile(filepath.Join("..", "..", "..", "docs", "m2", "schema-010-consumption-supply.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(embedded, contract) {
		t.Fatal("embedded migration diverges from docs/m2/schema-010-consumption-supply.sql")
	}
	embedded, err = migrationFiles.ReadFile("migrations/011_m2_arrears.sql")
	if err != nil {
		t.Fatal(err)
	}
	contract, err = os.ReadFile(filepath.Join("..", "..", "..", "docs", "m2", "schema-011-arrears.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(embedded, contract) {
		t.Fatal("embedded migration diverges from docs/m2/schema-011-arrears.sql")
	}
	embedded, err = migrationFiles.ReadFile("migrations/012_m2_insolvency.sql")
	if err != nil {
		t.Fatal(err)
	}
	contract, err = os.ReadFile(filepath.Join("..", "..", "..", "docs", "m2", "schema-012-insolvency.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(embedded, contract) {
		t.Fatal("embedded migration diverges from docs/m2/schema-012-insolvency.sql")
	}
	embedded, err = migrationFiles.ReadFile("migrations/013_m2_bankruptcy_claims.sql")
	if err != nil {
		t.Fatal(err)
	}
	contract, err = os.ReadFile(filepath.Join("..", "..", "..", "docs", "m2", "schema-013-bankruptcy-claims.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(embedded, contract) {
		t.Fatal("embedded migration diverges from docs/m2/schema-013-bankruptcy-claims.sql")
	}
	embedded, err = migrationFiles.ReadFile("migrations/014_m2_claim_allocations.sql")
	if err != nil {
		t.Fatal(err)
	}
	contract, err = os.ReadFile(filepath.Join("..", "..", "..", "docs", "m2", "schema-014-claim-allocations.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(embedded, contract) {
		t.Fatal("embedded migration diverges from docs/m2/schema-014-claim-allocations.sql")
	}
	embedded, err = migrationFiles.ReadFile("migrations/015_m2_estate_distribution.sql")
	if err != nil {
		t.Fatal(err)
	}
	contract, err = os.ReadFile(filepath.Join("..", "..", "..", "docs", "m2", "schema-015-estate-distribution.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(embedded, contract) {
		t.Fatal("embedded migration diverges from docs/m2/schema-015-estate-distribution.sql")
	}
	embedded, err = migrationFiles.ReadFile("migrations/016_m2_wage_participation.sql")
	if err != nil {
		t.Fatal(err)
	}
	contract, err = os.ReadFile(filepath.Join("..", "..", "..", "docs", "m2", "schema-016-wage-participation.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(embedded, contract) {
		t.Fatal("embedded migration diverges from docs/m2/schema-016-wage-participation.sql")
	}
	embedded, err = migrationFiles.ReadFile("migrations/017_m2_wage_allocation_policy.sql")
	if err != nil {
		t.Fatal(err)
	}
	contract, err = os.ReadFile(filepath.Join("..", "..", "..", "docs", "m2", "schema-017-wage-allocation-policy.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(embedded, contract) {
		t.Fatal("embedded migration diverges from docs/m2/schema-017-wage-allocation-policy.sql")
	}
	embedded, err = migrationFiles.ReadFile("migrations/018_m2_wage_claim_ownership.sql")
	if err != nil {
		t.Fatal(err)
	}
	contract, err = os.ReadFile(filepath.Join("..", "..", "..", "docs", "m2", "schema-018-wage-claim-ownership.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(embedded, contract) {
		t.Fatal("embedded migration diverges from docs/m2/schema-018-wage-claim-ownership.sql")
	}
	embedded, err = migrationFiles.ReadFile("migrations/019_m2_bankruptcy_slot_claims.sql")
	if err != nil {
		t.Fatal(err)
	}
	contract, err = os.ReadFile(filepath.Join("..", "..", "..", "docs", "m2", "schema-019-bankruptcy-slot-claims.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(embedded, contract) {
		t.Fatal("embedded migration diverges from docs/m2/schema-019-bankruptcy-slot-claims.sql")
	}
}

func TestOpenMigratesExistingM0DatabaseToRecoverySchema(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "m0-only.db")
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path))
	if err != nil {
		t.Fatal(err)
	}
	base, err := migrationFiles.ReadFile("migrations/001_init.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, string(base)); err != nil {
		t.Fatalf("apply base schema: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("open and migrate: %v", err)
	}
	defer store.Close()
	var currentVersion, payloadTable int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_meta WHERE schema_version = ?`, SchemaVersion).Scan(&currentVersion); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'snapshot_payloads'`).Scan(&payloadTable); err != nil {
		t.Fatal(err)
	}
	if currentVersion != 1 || payloadTable != 1 {
		t.Fatalf("migrations through 008 not applied: version=%d table=%d", currentVersion, payloadTable)
	}
}

func TestBootstrapRejectsLegacyProjectionOnlyDemo(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "legacy.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.db.ExecContext(ctx, `INSERT INTO world_instances(instance_id, world_definition_id, world_definition_version, created_at_utc, lifecycle_state) VALUES (?, 'legacy', '0', '2026-09-22T00:00:00Z', 'active')`, DemoInstanceID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO branches(instance_id, branch_id, label, head_sequence, created_at_utc) VALUES (?, ?, 'main', 1, '2026-09-22T00:00:00Z')`, DemoInstanceID, DemoBranchID); err != nil {
		t.Fatal(err)
	}
	if err := store.BootstrapDemo(ctx); !core.HasCode(err, core.CodeStorageFailure) {
		t.Fatalf("expected incompatible legacy demo rejection, got %v", err)
	}
}

func TestBootstrapIsDurableAndIdempotent(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "bootstrap ? #.db")
	store := openBootstrappedStore(t, ctx, path)
	assertState(t, ctx, store, initialState())
	if err := store.BootstrapDemo(ctx); err != nil {
		t.Fatalf("second bootstrap: %v", err)
	}
	assertState(t, ctx, store, initialState())
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer reopened.Close()
	if err := reopened.BootstrapDemo(ctx); err != nil {
		t.Fatalf("bootstrap after reopen: %v", err)
	}
	assertState(t, ctx, reopened, initialState())
}

func TestPurchaseCommitReplayAndReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "purchase.db")
	store := openBootstrappedStore(t, ctx, path)
	command := demoPurchase("cmd_purchase_1", "idem_purchase_1")

	result, err := store.Purchase(ctx, command)
	if err != nil {
		t.Fatalf("purchase: %v", err)
	}
	if result.Replayed || result.FirstSequence != 2 || result.LastSequence != 2 || result.EventCount != 1 {
		t.Fatalf("unexpected result: %+v", result)
	}
	expected := initialState()
	expected.HeadSequence = 2
	expected.BuyerBalance = 950
	expected.SellerBalance = 150
	expected.BuyerInventory = 2
	expected.SellerInventory = 8
	expected.Commands = 2
	expected.Events = 2
	expected.JournalEntries = 2
	expected.Postings = 7
	expected.StockMovements = 2
	expected.OutboxRows = 2
	assertState(t, ctx, store, expected)
	assertPostedJournalBalanced(t, ctx, store)
	assertSQLiteHealthy(t, ctx, store)

	retry := command
	retry.CommandID = "cmd_transport_retry"
	replayed, err := store.Purchase(ctx, retry)
	if err != nil {
		t.Fatalf("same-payload retry: %v", err)
	}
	if !replayed.Replayed || replayed.CommandID != command.CommandID || replayed.BatchID != result.BatchID {
		t.Fatalf("retry did not return original result: %+v", replayed)
	}
	assertState(t, ctx, store, expected)

	mismatch := retry
	mismatch.QuantityMinor = 3
	if _, err := store.Purchase(ctx, mismatch); !core.HasCode(err, core.CodeIdempotencyMismatch) {
		t.Fatalf("expected idempotency mismatch, got %v", err)
	}
	assertState(t, ctx, store, expected)

	stale := demoPurchase("cmd_stale", "idem_stale")
	if _, err := store.Purchase(ctx, stale); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatalf("expected branch conflict, got %v", err)
	}
	assertState(t, ctx, store, expected)

	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer reopened.Close()
	assertState(t, ctx, reopened, expected)
}

func TestReplaySnapshotAndProjectionRecovery(t *testing.T) {
	ctx := context.Background()
	store := openBootstrappedStore(t, ctx, filepath.Join(t.TempDir(), "replay.db"))
	defer store.Close()
	if _, err := store.Purchase(ctx, demoPurchase("cmd_replay", "idem_replay")); err != nil {
		t.Fatalf("purchase: %v", err)
	}

	full, err := store.Replay(ctx, DemoInstanceID, DemoBranchID, 2)
	if err != nil {
		t.Fatalf("empty-ledger replay: %v", err)
	}
	accounts, inventory, _ := replayMaps(full.State)
	if accounts[DemoIssuanceAccountID] != -2200 || accounts[DemoBuyerAccountID] != 950 || accounts[DemoSellerAccountID] != 150 || accounts[DemoEmployerAccountID] != 1000 || accounts[DemoLandlordAccountID] != 100 {
		t.Fatalf("unexpected replayed accounts: %v", accounts)
	}
	if inventory[inventoryKey(DemoBuyerLocationID, DemoSKUID)] != 2 || inventory[inventoryKey(DemoSellerLocationID, DemoSKUID)] != 8 {
		t.Fatalf("unexpected replayed inventory: %v", inventory)
	}

	snapshot, err := store.CreateSnapshot(ctx, DemoInstanceID, DemoBranchID, 1)
	if err != nil {
		t.Fatalf("create snapshot: %v", err)
	}
	repeated, err := store.CreateSnapshot(ctx, DemoInstanceID, DemoBranchID, 1)
	if err != nil {
		t.Fatalf("repeat snapshot: %v", err)
	}
	if repeated != snapshot {
		t.Fatalf("snapshot creation is not idempotent: %+v != %+v", repeated, snapshot)
	}
	fromSnapshot, err := store.ReplayFromLatestSnapshot(ctx, DemoInstanceID, DemoBranchID, 2)
	if err != nil {
		t.Fatalf("snapshot replay: %v", err)
	}
	if fromSnapshot.UsedSnapshotID != snapshot.SnapshotID || fromSnapshot.StateHash != full.StateHash {
		t.Fatalf("snapshot replay differs from empty replay: %+v vs %+v", fromSnapshot, full)
	}
	if differences, err := store.CompareProjections(ctx, DemoInstanceID, DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatalf("healthy projections differ: differences=%v err=%v", differences, err)
	}

	var eventCount, postingCount, movementCount int64
	if err := store.db.QueryRowContext(ctx, `SELECT (SELECT COUNT(*) FROM events), (SELECT COUNT(*) FROM postings), (SELECT COUNT(*) FROM stock_movements)`).Scan(&eventCount, &postingCount, &movementCount); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE account_balances SET balance_minor = 777 WHERE account_id = ?`, DemoBuyerAccountID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE inventory_balances SET quantity_minor = 99 WHERE location_id = ? AND sku_id = ?`, DemoSellerLocationID, DemoSKUID); err != nil {
		t.Fatal(err)
	}
	differences, err := store.CompareProjections(ctx, DemoInstanceID, DemoBranchID)
	if err != nil {
		t.Fatal(err)
	}
	if len(differences) != 2 {
		t.Fatalf("expected two projection differences, got %v", differences)
	}
	if err := store.RebuildProjections(ctx, DemoInstanceID, DemoBranchID); err != nil {
		t.Fatalf("rebuild projections: %v", err)
	}
	if differences, err := store.CompareProjections(ctx, DemoInstanceID, DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatalf("rebuilt projections differ: differences=%v err=%v", differences, err)
	}
	var eventsAfter, postingsAfter, movementsAfter int64
	if err := store.db.QueryRowContext(ctx, `SELECT (SELECT COUNT(*) FROM events), (SELECT COUNT(*) FROM postings), (SELECT COUNT(*) FROM stock_movements)`).Scan(&eventsAfter, &postingsAfter, &movementsAfter); err != nil {
		t.Fatal(err)
	}
	if eventsAfter != eventCount || postingsAfter != postingCount || movementsAfter != movementCount {
		t.Fatalf("projection rebuild changed authority: before=%d/%d/%d after=%d/%d/%d", eventCount, postingCount, movementCount, eventsAfter, postingsAfter, movementsAfter)
	}

	if _, err := store.db.ExecContext(ctx, `UPDATE snapshot_payloads SET payload = json_set(payload, '$.account_balances[0].balance_minor', 999) WHERE snapshot_id = ?`, snapshot.SnapshotID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReplayFromLatestSnapshot(ctx, DemoInstanceID, DemoBranchID, 2); !core.HasCode(err, core.CodeSnapshotMismatch) {
		t.Fatalf("expected snapshot hash mismatch, got %v", err)
	}
}

func TestOutboxReopenAndPublishMarkCrashRecovery(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "outbox.db")
	store := openBootstrappedStore(t, ctx, path)
	deliveries := map[string]int{}
	effects := map[string]int{}
	publisher := OutboxPublishFunc(func(_ context.Context, message OutboxMessage) error {
		deliveries[message.OutboxID]++
		if effects[message.OutboxID] == 0 {
			effects[message.OutboxID] = 1
		}
		return nil
	})

	result, err := store.DispatchOutbox(ctx, 10, publisher)
	if err != nil {
		t.Fatalf("dispatch genesis: %v", err)
	}
	if result.Eligible != 1 || result.MarkedPublished != 1 || deliveries["outbox_world_initialized"] != 1 {
		t.Fatalf("unexpected genesis dispatch: result=%+v deliveries=%v", result, deliveries)
	}
	if _, err := store.Purchase(ctx, demoPurchase("cmd_outbox_reopen", "idem_outbox_reopen")); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store, err = Open(ctx, path)
	if err != nil {
		t.Fatalf("reopen before delivery: %v", err)
	}
	result, err = store.DispatchOutbox(ctx, 10, publisher)
	if err != nil {
		store.Close()
		t.Fatalf("dispatch after reopen: %v", err)
	}
	if result.Eligible != 1 || result.MarkedPublished != 1 || deliveries["outbox_cmd_outbox_reopen"] != 1 {
		store.Close()
		t.Fatalf("committed unpublished message was not recovered: result=%+v deliveries=%v", result, deliveries)
	}

	second := demoPurchase("cmd_outbox_crash", "idem_outbox_crash")
	second.ExpectedHead = 2
	if _, err := store.Purchase(ctx, second); err != nil {
		store.Close()
		t.Fatal(err)
	}
	crashInjected := false
	store.afterPublish = func(message OutboxMessage) error {
		if message.OutboxID == "outbox_cmd_outbox_crash" && !crashInjected {
			crashInjected = true
			return core.NewError(core.CodeInjectedFailure, "crash after consumer delivery before publish mark")
		}
		return nil
	}
	result, err = store.DispatchOutbox(ctx, 10, publisher)
	if !core.HasCode(err, core.CodeInjectedFailure) {
		store.Close()
		t.Fatalf("expected publish/mark crash, got result=%+v err=%v", result, err)
	}
	if result.Delivered != 1 || result.MarkedPublished != 0 {
		store.Close()
		t.Fatalf("crash window was not observed: %+v", result)
	}
	pending, err := store.PendingOutboxCount(ctx)
	if err != nil || pending != 1 {
		store.Close()
		t.Fatalf("message should remain pending after crash: pending=%d err=%v", pending, err)
	}
	store.afterPublish = nil
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	result, err = store.DispatchOutbox(ctx, 10, publisher)
	if err != nil {
		t.Fatalf("redeliver after crash: %v", err)
	}
	if result.MarkedPublished != 1 || deliveries["outbox_cmd_outbox_crash"] != 2 || effects["outbox_cmd_outbox_crash"] != 1 {
		t.Fatalf("consumer dedupe/redelivery mismatch: result=%+v deliveries=%v effects=%v", result, deliveries, effects)
	}
	if pending, err := store.PendingOutboxCount(ctx); err != nil || pending != 0 {
		t.Fatalf("Outbox did not drain: pending=%d err=%v", pending, err)
	}
}

func TestOutboxFailureRecordsRetryWithoutRollingBackWorld(t *testing.T) {
	ctx := context.Background()
	store := openBootstrappedStore(t, ctx, filepath.Join(t.TempDir(), "outbox-retry.db"))
	defer store.Close()
	failure := errors.New("publisher unavailable")
	result, err := store.DispatchOutbox(ctx, 1, OutboxPublishFunc(func(context.Context, OutboxMessage) error {
		return failure
	}))
	if !core.HasCode(err, core.CodeOutboxDelivery) || result.Failed != 1 {
		t.Fatalf("expected recorded delivery failure, result=%+v err=%v", result, err)
	}
	var attempts int64
	var nextAttempt, published sql.NullString
	if err := store.db.QueryRowContext(ctx, `SELECT attempts, next_attempt_at_utc, published_at_utc FROM outbox WHERE outbox_id = 'outbox_world_initialized'`).Scan(&attempts, &nextAttempt, &published); err != nil {
		t.Fatal(err)
	}
	if attempts != 1 || !nextAttempt.Valid || published.Valid {
		t.Fatalf("retry state mismatch: attempts=%d next=%v published=%v", attempts, nextAttempt, published)
	}
	result, err = store.DispatchOutbox(ctx, 1, OutboxPublishFunc(func(context.Context, OutboxMessage) error { return nil }))
	if err != nil || result.Eligible != 0 {
		t.Fatalf("backoff was not respected: result=%+v err=%v", result, err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE outbox SET next_attempt_at_utc = NULL WHERE outbox_id = 'outbox_world_initialized'`); err != nil {
		t.Fatal(err)
	}
	result, err = store.DispatchOutbox(ctx, 1, OutboxPublishFunc(func(context.Context, OutboxMessage) error { return nil }))
	if err != nil || result.MarkedPublished != 1 {
		t.Fatalf("retry did not publish: result=%+v err=%v", result, err)
	}
	if err := store.db.QueryRowContext(ctx, `SELECT attempts, next_attempt_at_utc, published_at_utc FROM outbox WHERE outbox_id = 'outbox_world_initialized'`).Scan(&attempts, &nextAttempt, &published); err != nil {
		t.Fatal(err)
	}
	if attempts != 2 || nextAttempt.Valid || !published.Valid {
		t.Fatalf("successful retry state mismatch: attempts=%d next=%v published=%v", attempts, nextAttempt, published)
	}
	state, err := store.ReadDemoState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if state.HeadSequence != 1 || state.Events != 1 || state.Postings != 5 {
		t.Fatalf("publisher failure changed world authority: %+v", state)
	}
}

func TestStrictWorldSeedAndStableSchedulerOrder(t *testing.T) {
	ctx := context.Background()
	store := openBootstrappedStore(t, ctx, filepath.Join(t.TempDir(), "strict-seed.db"))
	defer store.Close()

	var entities, employment, rent, phases, scheduled, day int64
	if err := store.db.QueryRowContext(ctx, `SELECT
		(SELECT COUNT(*) FROM economic_entities),
		(SELECT COUNT(*) FROM employment_contracts),
		(SELECT COUNT(*) FROM rent_contracts),
		(SELECT COUNT(*) FROM scheduler_phases),
		(SELECT COUNT(*) FROM scheduler_items),
		(SELECT current_day FROM world_clocks WHERE instance_id = ? AND branch_id = ?)`,
		DemoInstanceID, DemoBranchID,
	).Scan(&entities, &employment, &rent, &phases, &scheduled, &day); err != nil {
		t.Fatal(err)
	}
	if entities != 6 || employment != 3 || rent != 3 || phases != 8 || scheduled != 99 || day != 0 {
		t.Fatalf("strict seed mismatch: entities=%d employment=%d rent=%d phases=%d scheduled=%d day=%d", entities, employment, rent, phases, scheduled, day)
	}

	items, err := store.ListDueSchedulerItems(ctx, 30, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 26 {
		t.Fatalf("expected 26 tasks through day 30, got %d", len(items))
	}
	previous := ""
	for _, item := range items {
		key := fmt.Sprintf("%s\x00%s\x00%020d\x00%s", item.WorldTime, item.PhaseID, item.DeclaredPriority, item.SchedulerItemID)
		if key < previous {
			t.Fatalf("scheduler order regressed: %q before %q", previous, key)
		}
		previous = key
	}
}

func TestStrictSchedulerIsIndependentOfInsertionOrder(t *testing.T) {
	ctx := context.Background()
	first := openBootstrappedStore(t, ctx, filepath.Join(t.TempDir(), "ordered.db"))
	defer first.Close()
	second := openBootstrappedStore(t, ctx, filepath.Join(t.TempDir(), "reversed.db"))
	defer second.Close()
	reverseSchedulerInsertionOrder(t, ctx, second)

	firstRun, err := first.RunStrictWorld(ctx, 65, 1000)
	if err != nil {
		t.Fatalf("ordered run: %v", err)
	}
	secondRun, err := second.RunStrictWorld(ctx, 65, 1000)
	if err != nil {
		t.Fatalf("reversed run: %v", err)
	}
	if firstRun.Status != "completed" || secondRun.Status != "completed" || firstRun.ProcessedItems != secondRun.ProcessedItems {
		t.Fatalf("run mismatch: ordered=%+v reversed=%+v", firstRun, secondRun)
	}
	if firstEvents, secondEvents := authoritativeEventDigest(t, ctx, first), authoritativeEventDigest(t, ctx, second); firstEvents != secondEvents {
		t.Fatalf("authoritative events depend on insertion order\nordered:\n%s\nreversed:\n%s", firstEvents, secondEvents)
	}
	firstReplay, err := first.Replay(ctx, DemoInstanceID, DemoBranchID, firstRun.HeadSequence)
	if err != nil {
		t.Fatal(err)
	}
	secondReplay, err := second.Replay(ctx, DemoInstanceID, DemoBranchID, secondRun.HeadSequence)
	if err != nil {
		t.Fatal(err)
	}
	if firstReplay.StateHash != secondReplay.StateHash {
		t.Fatalf("replay hashes depend on insertion order: %s != %s", firstReplay.StateHash, secondReplay.StateHash)
	}
}

func TestStrictSchedulerBudgetCheckpointResumesAfterReopen(t *testing.T) {
	ctx := context.Background()
	resumablePath := filepath.Join(t.TempDir(), "resumable.db")
	resumable := openBootstrappedStore(t, ctx, resumablePath)
	partial, err := resumable.RunStrictWorld(ctx, 35, 5)
	if err != nil {
		t.Fatalf("partial run: %v", err)
	}
	if partial.Status != "budget_exhausted" || partial.ProcessedItems != 5 || partial.PendingDue == 0 {
		t.Fatalf("expected a durable budget checkpoint, got %+v", partial)
	}
	if err := resumable.Close(); err != nil {
		t.Fatal(err)
	}
	resumable, err = Open(ctx, resumablePath)
	if err != nil {
		t.Fatal(err)
	}
	defer resumable.Close()
	resumed, err := resumable.RunStrictWorld(ctx, 35, 1000)
	if err != nil {
		t.Fatalf("resumed run: %v", err)
	}
	if resumed.Status != "completed" || resumed.PendingDue != 0 {
		t.Fatalf("resumed run did not complete: %+v", resumed)
	}
	var currentDay int
	if err := resumable.db.QueryRowContext(ctx, `SELECT current_day FROM world_clocks WHERE instance_id = ? AND branch_id = ?`, DemoInstanceID, DemoBranchID).Scan(&currentDay); err != nil {
		t.Fatal(err)
	}
	if currentDay != 35 {
		t.Fatalf("clock checkpoint did not reach day 35: %d", currentDay)
	}

	uninterrupted := openBootstrappedStore(t, ctx, filepath.Join(t.TempDir(), "uninterrupted.db"))
	defer uninterrupted.Close()
	full, err := uninterrupted.RunStrictWorld(ctx, 35, 1000)
	if err != nil {
		t.Fatalf("uninterrupted run: %v", err)
	}
	if resumed.HeadSequence != full.HeadSequence || authoritativeEventDigest(t, ctx, resumable) != authoritativeEventDigest(t, ctx, uninterrupted) {
		t.Fatal("restart changed the authoritative scheduler result")
	}
}

func TestStrictWorldNinetyDaySettlement(t *testing.T) {
	ctx := context.Background()
	store := openBootstrappedStore(t, ctx, filepath.Join(t.TempDir(), "ninety-days.db"))
	defer store.Close()

	run, err := store.RunStrictWorld(ctx, 90, 1000)
	if err != nil {
		t.Fatalf("run 90-day world: %v", err)
	}
	if run.Status != "completed" || run.ProcessedItems != 97 || run.PendingDue != 0 || run.HeadSequence != 98 {
		t.Fatalf("unexpected 90-day run: %+v", run)
	}
	var day int
	var clockStatus string
	if err := store.db.QueryRowContext(ctx, `SELECT current_day, status FROM world_clocks WHERE instance_id = ? AND branch_id = ?`, DemoInstanceID, DemoBranchID).Scan(&day, &clockStatus); err != nil {
		t.Fatal(err)
	}
	if day != 90 || clockStatus != "completed" {
		t.Fatalf("world clock mismatch: day=%d status=%s", day, clockStatus)
	}

	assertScalar(t, ctx, store, `SELECT COUNT(*) FROM wage_obligations`, 9)
	assertScalar(t, ctx, store, `SELECT COUNT(*) FROM wage_obligations WHERE status = 'paid'`, 5)
	assertScalar(t, ctx, store, `SELECT COUNT(*) FROM wage_obligations WHERE status = 'arrears'`, 4)
	assertScalar(t, ctx, store, `SELECT SUM(amount_due_minor - amount_paid_minor) FROM wage_obligations`, 1100)
	assertScalar(t, ctx, store, `SELECT amount_paid_minor FROM wage_obligations WHERE obligation_id = 'wage_employment_2_30_60'`, 300)
	assertScalar(t, ctx, store, `SELECT COUNT(*) FROM obligation_settlements WHERE obligation_kind = 'wage' AND obligation_id = 'wage_employment_2_30_60' AND paid_minor > 0`, 2)
	assertScalar(t, ctx, store, `SELECT COUNT(*) FROM rent_obligations`, 9)
	assertScalar(t, ctx, store, `SELECT COUNT(*) FROM rent_obligations WHERE status = 'paid'`, 6)
	assertScalar(t, ctx, store, `SELECT COUNT(*) FROM rent_obligations WHERE status = 'past_due'`, 1)
	assertScalar(t, ctx, store, `SELECT COUNT(*) FROM rent_obligations WHERE status = 'partially_paid'`, 2)
	assertScalar(t, ctx, store, `SELECT SUM(amount_due_minor - amount_paid_minor) FROM rent_obligations`, 200)
	assertScalar(t, ctx, store, `SELECT COUNT(*) FROM obligation_settlements`, 21)
	assertScalar(t, ctx, store, `SELECT COUNT(*) FROM obligation_settlements WHERE status IN ('partial', 'failed')`, 10)
	assertScalar(t, ctx, store, `SELECT SUM(balance_minor) FROM account_balances WHERE account_id LIKE 'account_wage_%_expense'`, 2700)
	assertScalar(t, ctx, store, `SELECT SUM(balance_minor) FROM account_balances WHERE account_id LIKE 'account_wage_%_payable'`, -1100)
	assertScalar(t, ctx, store, `SELECT SUM(balance_minor) FROM account_balances WHERE account_id LIKE 'account_wage_%_receivable'`, 1100)
	assertScalar(t, ctx, store, `SELECT SUM(balance_minor) FROM account_balances WHERE account_id LIKE 'account_wage_%_income'`, -2700)

	assertScalar(t, ctx, store, `SELECT COUNT(*) FROM events WHERE event_type = 'HouseholdPurchaseCompleted'`, 24)
	assertScalar(t, ctx, store, `SELECT COUNT(*) FROM events WHERE event_type = 'PurchaseFailed'`, 12)
	assertScalar(t, ctx, store, `SELECT COUNT(*) FROM events WHERE event_type = 'StoreRestocked'`, 6)
	assertScalar(t, ctx, store, `SELECT COUNT(*) FROM events WHERE event_type = 'RentPastDue'`, 1)
	assertScalar(t, ctx, store, `SELECT COUNT(*) FROM events WHERE event_type = 'SchedulerSkipLogged'`, 14)
	assertScalar(t, ctx, store, `SELECT COUNT(*) FROM events`, 98)
	assertScalar(t, ctx, store, `SELECT COUNT(*) FROM stock_movements`, 31)
	assertScalar(t, ctx, store, `SELECT COUNT(*) FROM inventory_balances WHERE quantity_minor < 0`, 0)
	assertScalar(t, ctx, store, `SELECT quantity_minor FROM inventory_balances WHERE location_id = 'location_seller' AND sku_id = 'sku_bread'`, 16)
	assertScalar(t, ctx, store, `SELECT balance_minor FROM account_balances WHERE account_id = 'account_enterprise'`, 0)
	assertScalar(t, ctx, store, `SELECT balance_minor FROM account_balances WHERE account_id = 'account_buyer'`, 850)
	assertScalar(t, ctx, store, `SELECT balance_minor FROM account_balances WHERE account_id = 'account_employee_2'`, 0)
	assertScalar(t, ctx, store, `SELECT balance_minor FROM account_balances WHERE account_id = 'account_employee_3'`, 0)
	assertScalar(t, ctx, store, `SELECT balance_minor FROM account_balances WHERE account_id = 'account_landlord'`, 1250)
	assertScalar(t, ctx, store, `SELECT balance_minor FROM account_balances WHERE account_id = 'account_seller'`, 100)
	assertScalar(t, ctx, store, `SELECT SUM(CAST(json_extract(payload, '$.total_minor') AS INTEGER)) FROM events WHERE event_type = 'HouseholdPurchaseCompleted' AND json_extract(payload, '$.buyer_account_id') = 'account_buyer'`, 300)
	assertScalar(t, ctx, store, `SELECT SUM(CAST(json_extract(payload, '$.total_minor') AS INTEGER)) FROM events WHERE event_type = 'HouseholdPurchaseCompleted' AND json_extract(payload, '$.buyer_account_id') = 'account_employee_2'`, 200)
	assertScalar(t, ctx, store, `SELECT SUM(CAST(json_extract(payload, '$.total_minor') AS INTEGER)) FROM events WHERE event_type = 'HouseholdPurchaseCompleted' AND json_extract(payload, '$.buyer_account_id') = 'account_employee_3'`, 100)
	assertScalar(t, ctx, store, `SELECT COUNT(*) FROM account_balances WHERE account_id IN ('account_enterprise', 'account_buyer', 'account_employee_2', 'account_employee_3', 'account_landlord', 'account_seller') AND balance_minor < 0`, 0)

	assertPostedJournalBalanced(t, ctx, store)
	assertSQLiteHealthy(t, ctx, store)
	if differences, err := store.CompareProjections(ctx, DemoInstanceID, DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatalf("90-day projections differ from authority: differences=%v err=%v", differences, err)
	}
}

func TestRepeatedWageAccrualDoesNotDuplicateObligationOrExpense(t *testing.T) {
	ctx := context.Background()
	store := openBootstrappedStore(t, ctx, filepath.Join(t.TempDir(), "duplicate-accrual.db"))
	defer store.Close()
	if _, err := store.RunStrictWorld(ctx, 30, 1000); err != nil {
		t.Fatal(err)
	}
	var postingsBefore int64
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM postings`).Scan(&postingsBefore); err != nil {
		t.Fatal(err)
	}
	payload, err := core.CanonicalJSON(scheduledPayload{
		Kind: "wage_accrual", Day: 30, SubjectID: "employment_1",
		AccountID: DemoBuyerAccountID, LocationID: DemoBuyerLocationID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO scheduler_items(scheduler_item_id, instance_id, branch_id, world_time, phase_id, declared_priority, status, payload) VALUES ('sched_030_duplicate_wage_accrual_employment_1', ?, ?, ?, '10_wage_accrual', 0, 'pending', ?)`, DemoInstanceID, DemoBranchID, strictDayTime(30), string(payload)); err != nil {
		t.Fatal(err)
	}
	run, err := store.RunStrictWorld(ctx, 30, 10)
	if err != nil {
		t.Fatal(err)
	}
	if run.ProcessedItems != 1 || run.Status != "completed" {
		t.Fatalf("duplicate accrual was not consumed idempotently: %+v", run)
	}
	assertScalar(t, ctx, store, `SELECT COUNT(*) FROM wage_obligations WHERE contract_id = 'employment_1' AND period_start_day = 0 AND period_end_day = 30`, 1)
	assertScalar(t, ctx, store, `SELECT balance_minor FROM account_balances WHERE account_id = 'account_wage_employment_1_expense'`, 300)
	assertScalar(t, ctx, store, `SELECT COUNT(*) FROM events WHERE event_type = 'SchedulerSkipLogged' AND json_extract(payload, '$.reason') = 'wage_already_accrued'`, 1)
	var postingsAfter int64
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM postings`).Scan(&postingsAfter); err != nil {
		t.Fatal(err)
	}
	if postingsAfter != postingsBefore {
		t.Fatalf("duplicate accrual changed postings: before=%d after=%d", postingsBefore, postingsAfter)
	}
}

func TestHouseholdFoodBudgetRejectsSpendBeyondLimit(t *testing.T) {
	ctx := context.Background()
	store := openBootstrappedStore(t, ctx, filepath.Join(t.TempDir(), "food-budget.db"))
	defer store.Close()
	payload, err := core.CanonicalJSON(scheduledPayload{
		Kind: "household_purchase", Day: 90, SubjectID: DemoEmployee1EntityID,
		AccountID: DemoBuyerAccountID, LocationID: DemoBuyerLocationID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO scheduler_items(scheduler_item_id, instance_id, branch_id, world_time, phase_id, declared_priority, status, payload) VALUES ('sched_090_household_purchase_budget_probe', ?, ?, ?, '50_household_purchase', 1, 'pending', ?)`, DemoInstanceID, DemoBranchID, strictDayTime(90), string(payload)); err != nil {
		t.Fatal(err)
	}
	run, err := store.RunStrictWorld(ctx, 90, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != "completed" || run.ProcessedItems != 98 {
		t.Fatalf("budget probe run mismatch: %+v", run)
	}
	assertScalar(t, ctx, store, `SELECT COUNT(*) FROM events WHERE event_type = 'PurchaseFailed' AND json_extract(payload, '$.reason') = 'budget_exceeded'`, 1)
	assertScalar(t, ctx, store, `SELECT CAST(json_extract(payload, '$.remaining_budget_minor') AS INTEGER) FROM events WHERE event_type = 'PurchaseFailed' AND json_extract(payload, '$.reason') = 'budget_exceeded'`, 0)
	assertScalar(t, ctx, store, `SELECT SUM(CAST(json_extract(payload, '$.total_minor') AS INTEGER)) FROM events WHERE event_type = 'HouseholdPurchaseCompleted' AND json_extract(payload, '$.buyer_account_id') = 'account_buyer'`, 300)
}

func TestControlledIssuanceAuthorizationLimitAccountingAndIdempotency(t *testing.T) {
	ctx := context.Background()
	store := openBootstrappedStore(t, ctx, filepath.Join(t.TempDir(), "issuance.db"))
	defer store.Close()
	base := core.IssueCurrencyCommand{
		CommandID: "cmd_issue_unauthorized", InstanceID: DemoInstanceID, BranchID: DemoBranchID,
		PrincipalID: "principal_buyer", CapabilityID: "economy.issue", PolicyID: "policy_creator_credit_issue",
		IdempotencyKey: "idem_issue_unauthorized", ExpectedHead: 1, WorldTime: "2026-01-02T00:00:00Z",
		TargetAccountID: DemoBuyerAccountID, CurrencyID: DemoCurrencyID, AmountMinor: 10000000,
		ReasonCode: "creator_test_grant",
	}
	if _, err := store.IssueCurrency(ctx, base); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("expected unauthorized issuance rejection, got %v", err)
	}
	assertState(t, ctx, store, initialState())
	assertScalar(t, ctx, store, `SELECT issued_total_minor FROM issuance_policies WHERE policy_id = 'policy_creator_credit_issue'`, 0)
	assertScalar(t, ctx, store, `SELECT COUNT(*) FROM intervention_records`, 0)

	overLimit := base
	overLimit.CommandID = "cmd_issue_over_limit"
	overLimit.PrincipalID = "principal_creator"
	overLimit.IdempotencyKey = "idem_issue_over_limit"
	overLimit.AmountMinor = 10000001
	if _, err := store.IssueCurrency(ctx, overLimit); !core.HasCode(err, core.CodeIssuanceLimit) {
		t.Fatalf("expected issuance limit rejection, got %v", err)
	}
	assertState(t, ctx, store, initialState())
	assertScalar(t, ctx, store, `SELECT COUNT(*) FROM intervention_records`, 0)

	authorized := base
	authorized.CommandID = "cmd_issue_authorized"
	authorized.PrincipalID = "principal_creator"
	authorized.IdempotencyKey = "idem_issue_authorized"
	result, err := store.IssueCurrency(ctx, authorized)
	if err != nil {
		t.Fatalf("authorized issuance: %v", err)
	}
	if result.Replayed || result.FirstSequence != 2 || result.EventCount != 1 {
		t.Fatalf("unexpected issuance result: %+v", result)
	}
	assertScalar(t, ctx, store, `SELECT balance_minor FROM account_balances WHERE account_id = 'account_buyer'`, 10001000)
	assertScalar(t, ctx, store, `SELECT balance_minor FROM account_balances WHERE account_id = 'account_issuance'`, -10002200)
	assertScalar(t, ctx, store, `SELECT issued_total_minor FROM issuance_policies WHERE policy_id = 'policy_creator_credit_issue'`, 10000000)
	assertScalar(t, ctx, store, `SELECT COUNT(*) FROM intervention_records WHERE principal_id = 'principal_creator' AND grant_id = 'grant_creator_issue_buyer' AND amount_minor = 10000000`, 1)
	assertScalar(t, ctx, store, `SELECT COUNT(*) FROM audit_records WHERE record_type = 'intervention' AND related_event_id = 'event_cmd_issue_authorized'`, 1)
	assertScalar(t, ctx, store, `SELECT unit_price_minor FROM market_quotes WHERE quote_id = 'quote_bread_90d'`, 25)
	assertPostedJournalBalanced(t, ctx, store)
	if differences, err := store.CompareProjections(ctx, DemoInstanceID, DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatalf("issuance projections differ from authority: differences=%v err=%v", differences, err)
	}

	retry := authorized
	retry.CommandID = "cmd_issue_transport_retry"
	replayed, err := store.IssueCurrency(ctx, retry)
	if err != nil {
		t.Fatalf("issuance retry: %v", err)
	}
	if !replayed.Replayed || replayed.CommandID != authorized.CommandID || replayed.EventID != result.EventID {
		t.Fatalf("issuance retry did not return original result: %+v", replayed)
	}
	assertScalar(t, ctx, store, `SELECT COUNT(*) FROM events WHERE event_type = 'CurrencyIssued'`, 1)
	assertScalar(t, ctx, store, `SELECT COUNT(*) FROM intervention_records`, 1)

	mismatch := retry
	mismatch.AmountMinor = 9999999
	if _, err := store.IssueCurrency(ctx, mismatch); !core.HasCode(err, core.CodeIdempotencyMismatch) {
		t.Fatalf("expected issuance idempotency mismatch, got %v", err)
	}
}

func TestPrivateEconomicReadScopesAndOperatorRedaction(t *testing.T) {
	ctx := context.Background()
	store := openBootstrappedStore(t, ctx, filepath.Join(t.TempDir(), "private-reads.db"))
	defer store.Close()
	run, err := store.RunStrictWorld(ctx, 65, 1000)
	if err != nil {
		t.Fatal(err)
	}

	playerRequest := core.PrivateEconomicRead{
		PrincipalID: "principal_buyer", CapabilityID: "economy.private.read",
		InstanceID: DemoInstanceID, BranchID: DemoBranchID, SubjectID: DemoEmployee1EntityID,
		Fields: []string{"account_id", "balance_minor"},
	}
	playerView, err := store.ReadPrivateEconomy(ctx, playerRequest)
	if err != nil {
		t.Fatalf("player self read: %v", err)
	}
	if playerView.AccountID != DemoBuyerAccountID || playerView.BalanceMinor == nil || playerView.OwnerID != "" || len(playerView.EvidenceRefs) != 0 || playerView.OperatorRedacted {
		t.Fatalf("player self view leaked or omitted fields: %+v", playerView)
	}
	otherPlayer := playerRequest
	otherPlayer.SubjectID = DemoEmployee2EntityID
	if _, err := store.ReadPrivateEconomy(ctx, otherPlayer); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("player read of another employee should be denied, got %v", err)
	}
	escalatedPlayer := playerRequest
	escalatedPlayer.Fields = []string{"account_id", "balance_minor", "owner_id"}
	if _, err := store.ReadPrivateEconomy(ctx, escalatedPlayer); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("player field escalation should be denied, got %v", err)
	}

	creatorRequest := core.PrivateEconomicRead{
		PrincipalID: "principal_creator", CapabilityID: "economy.private.read",
		InstanceID: DemoInstanceID, BranchID: DemoBranchID, SubjectID: DemoEmployee2EntityID,
		Fields: []string{"account_id", "balance_minor", "owner_id", "evidence"},
	}
	creatorView, err := store.ReadPrivateEconomy(ctx, creatorRequest)
	if err != nil {
		t.Fatalf("creator scoped read: %v", err)
	}
	if creatorView.AccountID != DemoEmployee2AccountID || creatorView.BalanceMinor == nil || creatorView.OwnerID != "employee_2" || len(creatorView.EvidenceRefs) != 4 || creatorView.OperatorRedacted {
		t.Fatalf("creator view mismatch: %+v", creatorView)
	}
	wrongBranch := creatorRequest
	wrongBranch.BranchID = "br_other"
	if _, err := store.ReadPrivateEconomy(ctx, wrongBranch); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("creator cross-branch read should be denied, got %v", err)
	}

	operatorRequest := core.PrivateEconomicRead{
		PrincipalID: "principal_operator", CapabilityID: "diagnostics.private.read",
		InstanceID: DemoInstanceID, BranchID: DemoBranchID, SubjectID: DemoEmployee2EntityID,
		Fields: []string{"account_id", "balance_minor", "diagnostics"},
	}
	operatorView, err := store.ReadPrivateEconomy(ctx, operatorRequest)
	if err != nil {
		t.Fatalf("operator diagnostic read: %v", err)
	}
	if !operatorView.OperatorRedacted || operatorView.OwnerID != "" || operatorView.Diagnostics == nil || operatorView.AccountID != DemoEmployee2AccountID {
		t.Fatalf("operator view was not explicitly redacted: %+v", operatorView)
	}
	operatorEscalation := operatorRequest
	operatorEscalation.Fields = []string{"account_id", "owner_id"}
	if _, err := store.ReadPrivateEconomy(ctx, operatorEscalation); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("operator owner identity escalation should be denied, got %v", err)
	}

	var headAfter int64
	if err := store.db.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id = ? AND branch_id = ?`, DemoInstanceID, DemoBranchID).Scan(&headAfter); err != nil {
		t.Fatal(err)
	}
	if headAfter != run.HeadSequence {
		t.Fatalf("read authorization changed authority: before=%d after=%d", run.HeadSequence, headAfter)
	}
}

func reverseSchedulerInsertionOrder(t *testing.T, ctx context.Context, store *Store) {
	t.Helper()
	items, err := store.ListDueSchedulerItems(ctx, 90, 10000)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `DELETE FROM scheduler_items`); err != nil {
		t.Fatal(err)
	}
	for index := len(items) - 1; index >= 0; index-- {
		item := items[index]
		if _, err := store.db.ExecContext(ctx, `INSERT INTO scheduler_items(scheduler_item_id, instance_id, branch_id, world_time, phase_id, declared_priority, status, payload) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, item.SchedulerItemID, DemoInstanceID, DemoBranchID, item.WorldTime, item.PhaseID, item.DeclaredPriority, item.Status, item.Payload); err != nil {
			t.Fatal(err)
		}
	}
}

func authoritativeEventDigest(t *testing.T, ctx context.Context, store *Store) string {
	t.Helper()
	rows, err := store.db.QueryContext(ctx, `SELECT event_sequence, event_id, event_type, world_time, payload FROM events ORDER BY event_sequence`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var result strings.Builder
	for rows.Next() {
		var sequence int64
		var eventID, eventType, worldTime, payload string
		if err := rows.Scan(&sequence, &eventID, &eventType, &worldTime, &payload); err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(&result, "%d|%s|%s|%s|%s\n", sequence, eventID, eventType, worldTime, payload)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return result.String()
}

func assertScalar(t *testing.T, ctx context.Context, store *Store, query string, expected int64) {
	t.Helper()
	var actual int64
	if err := store.db.QueryRowContext(ctx, query).Scan(&actual); err != nil {
		t.Fatal(err)
	}
	if actual != expected {
		t.Fatalf("scalar mismatch for %q: got %d, want %d", query, actual, expected)
	}
}

func TestConcurrentExpectedHeadAllowsExactlyOneCommit(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "concurrent.db")
	firstStore := openBootstrappedStore(t, ctx, path)
	defer firstStore.Close()
	run, err := firstStore.RunStrictWorld(ctx, 30, 1000)
	if err != nil {
		t.Fatalf("prepare two funded buyers: %v", err)
	}
	consumeToLast := demoPurchase("cmd_consume_to_last", "idem_consume_to_last")
	consumeToLast.ExpectedHead = run.HeadSequence
	consumeToLast.QuantityMinor = 15
	consumeToLast.UnitPriceMinor = 1
	prepared, err := firstStore.Purchase(ctx, consumeToLast)
	if err != nil {
		t.Fatalf("prepare last inventory unit: %v", err)
	}
	before, err := firstStore.ReadDemoState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if before.SellerInventory != 1 {
		t.Fatalf("last-item precondition failed: %+v", before)
	}
	secondStore, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("open second store: %v", err)
	}
	defer secondStore.Close()

	commands := []core.PurchaseCommand{
		demoPurchase("cmd_concurrent_buyer_1", "idem_concurrent_buyer_1"),
		demoPurchase("cmd_concurrent_buyer_2", "idem_concurrent_buyer_2"),
	}
	for index := range commands {
		commands[index].ExpectedHead = prepared.LastSequence
		commands[index].QuantityMinor = 1
		commands[index].UnitPriceMinor = 1
	}
	commands[1].ActorID = "employee_2"
	commands[1].PrincipalID = "principal_employee_2"
	commands[1].BuyerAccountID = DemoEmployee2AccountID
	commands[1].BuyerLocationID = DemoEmployee2LocationID
	stores := []*Store{firstStore, secondStore}
	errorsByIndex := make([]error, 2)
	start := make(chan struct{})
	var wait sync.WaitGroup
	for index := range stores {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			<-start
			_, errorsByIndex[index] = stores[index].Purchase(ctx, commands[index])
		}(index)
	}
	close(start)
	wait.Wait()

	successes, conflicts := 0, 0
	for _, err := range errorsByIndex {
		switch {
		case err == nil:
			successes++
		case core.HasCode(err, core.CodeBranchConflict):
			conflicts++
		default:
			t.Fatalf("unexpected concurrent result: %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("successes=%d conflicts=%d errors=%v", successes, conflicts, errorsByIndex)
	}
	state, err := firstStore.ReadDemoState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if state.HeadSequence != before.HeadSequence+1 || state.Commands != before.Commands+1 || state.Events != before.Events+1 || state.StockMovements != before.StockMovements+1 || state.OutboxRows != before.OutboxRows+1 || state.SellerInventory != 0 {
		t.Fatalf("concurrent commits produced a partial or duplicate write: %+v", state)
	}
	assertScalar(t, ctx, firstStore, `SELECT COUNT(*) FROM inventory_balances WHERE quantity_minor < 0`, 0)
	assertPostedJournalBalanced(t, ctx, firstStore)
}

func TestPurchasePolicyFailuresLeaveNoWrites(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*core.PurchaseCommand)
		code   core.ErrorCode
	}{
		{"unauthorized", func(command *core.PurchaseCommand) { command.PrincipalID = "principal_intruder" }, core.CodeUnauthorized},
		{"insufficient funds", func(command *core.PurchaseCommand) { command.QuantityMinor = 2; command.UnitPriceMinor = 600 }, core.CodeInsufficientFunds},
		{"insufficient stock", func(command *core.PurchaseCommand) { command.QuantityMinor = 11; command.UnitPriceMinor = 1 }, core.CodeInsufficientStock},
		{"integer overflow", func(command *core.PurchaseCommand) { command.QuantityMinor = math.MaxInt64; command.UnitPriceMinor = 2 }, core.CodeIntegerOverflow},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			store := openBootstrappedStore(t, ctx, filepath.Join(t.TempDir(), "failure.db"))
			defer store.Close()
			command := demoPurchase("cmd_failure", "idem_failure")
			test.mutate(&command)
			if _, err := store.Purchase(ctx, command); !core.HasCode(err, test.code) {
				t.Fatalf("expected %s, got %v", test.code, err)
			}
			assertState(t, ctx, store, initialState())
		})
	}
}

func TestInjectedPreCommitFailureRollsBackEverything(t *testing.T) {
	ctx := context.Background()
	store := openBootstrappedStore(t, ctx, filepath.Join(t.TempDir(), "rollback.db"))
	defer store.Close()
	command := demoPurchase("cmd_rollback", "idem_rollback")
	store.beforeCommit = func() error {
		return core.NewError(core.CodeInjectedFailure, "test failure immediately before commit")
	}
	if _, err := store.Purchase(ctx, command); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatalf("expected injected failure, got %v", err)
	}
	assertState(t, ctx, store, initialState())

	store.beforeCommit = nil
	if _, err := store.Purchase(ctx, command); err != nil {
		t.Fatalf("retry after rollback: %v", err)
	}
	state, err := store.ReadDemoState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if state.HeadSequence != 2 || state.Commands != 2 || state.Events != 2 || state.OutboxRows != 2 {
		t.Fatalf("retry did not commit one complete batch: %+v", state)
	}
}

func openBootstrappedStore(t *testing.T, ctx context.Context, path string) *Store {
	t.Helper()
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := store.BootstrapDemo(ctx); err != nil {
		store.Close()
		t.Fatalf("bootstrap: %v", err)
	}
	return store
}

func demoPurchase(commandID, idempotencyKey string) core.PurchaseCommand {
	return core.PurchaseCommand{
		CommandID: commandID, InstanceID: DemoInstanceID, BranchID: DemoBranchID,
		ActorID: "buyer", PrincipalID: "principal_buyer", IdempotencyKey: idempotencyKey,
		ExpectedHead: 1, WorldTime: "2026-09-22T09:00:00Z",
		BuyerAccountID: DemoBuyerAccountID, SellerAccountID: DemoSellerAccountID,
		BuyerLocationID: DemoBuyerLocationID, SellerLocationID: DemoSellerLocationID,
		SKUID: DemoSKUID, CurrencyID: DemoCurrencyID, QuantityMinor: 2, UnitPriceMinor: 25,
	}
}

func initialState() State {
	return State{
		HeadSequence: 1, BuyerBalance: 1000, SellerBalance: 100, SellerInventory: 10,
		Commands: 1, Events: 1, JournalEntries: 1, Postings: 5, StockMovements: 1, OutboxRows: 1,
	}
}

func assertState(t *testing.T, ctx context.Context, store *Store, expected State) {
	t.Helper()
	actual, err := store.ReadDemoState(ctx)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	if actual != expected {
		t.Fatalf("state mismatch\nactual:   %+v\nexpected: %+v", actual, expected)
	}
}

func assertPostedJournalBalanced(t *testing.T, ctx context.Context, store *Store) {
	t.Helper()
	var invalid int64
	err := store.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM (
		  SELECT j.entry_id
		  FROM journal_entries j JOIN postings p ON p.entry_id = j.entry_id
		  GROUP BY j.entry_id, j.status
		  HAVING j.status <> 'posted' OR SUM(p.amount_minor) <> 0
		)`,
	).Scan(&invalid)
	if err != nil {
		t.Fatal(err)
	}
	if invalid != 0 {
		t.Fatalf("found %d unposted or unbalanced journals", invalid)
	}
}

func assertSQLiteHealthy(t *testing.T, ctx context.Context, store *Store) {
	t.Helper()
	var integrity string
	if err := store.db.QueryRowContext(ctx, `PRAGMA integrity_check`).Scan(&integrity); err != nil {
		t.Fatal(err)
	}
	if integrity != "ok" {
		t.Fatalf("SQLite integrity check: %s", integrity)
	}
	rows, err := store.db.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatal("foreign_key_check returned a violation")
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
}

func TestErrorCodesRemainInspectable(t *testing.T) {
	wrapped := core.WrapError(core.CodeStorageFailure, "outer", errors.New("inner"))
	if !core.HasCode(wrapped, core.CodeStorageFailure) {
		t.Fatal("wrapped error lost code")
	}
}
