package storage

import (
	"context"
	"database/sql"
	"path/filepath"
	"sync"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestM2WageIsFundedSettledInOrderAndRecoverable(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "cohort-economy.db")
	store := openM2AgentStore(t, ctx, path, false)
	setup, err := store.PrepareM2EconomicDemo(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if setup.EventSequence != 5 || setup.Replayed {
		t.Fatalf("wrong economy definition: %+v", setup)
	}
	assertM2Value(t, ctx, store, `SELECT balance_minor FROM account_balances WHERE account_id = ?`, []any{M2DemoCohortAssetAccountID}, 7700)
	assertM2Value(t, ctx, store, `SELECT balance_minor FROM account_balances WHERE account_id = ?`, []any{m2EconomyEmployerCash}, 1200)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM scheduler_items WHERE status = 'pending' AND phase_id IN (?, ?, ?, ?)`, []any{m2EconomyPhaseAccrue, m2EconomyPhasePay, m2EconomyRentPhaseAccrue, m2EconomyRentPhasePay}, 120)
	again, err := store.PrepareM2EconomicDemo(ctx)
	if err != nil || !again.Replayed || again.EventSequence != setup.EventSequence {
		t.Fatalf("repeat setup: %+v %v", again, err)
	}
	first, err := store.RunAgentLife(ctx, M2AgentNoonTime, 1)
	if err != nil {
		t.Fatal(err)
	}
	if first.HeadSequence != 6 || first.PendingDue != 11 {
		t.Fatalf("expected accrued wage first: %+v", first)
	}
	assertM2Value(t, ctx, store, `SELECT amount_due_minor FROM m2_economic_obligations WHERE obligation_id = ?`, []any{m2EconomyObligationID}, 180)
	assertM2Value(t, ctx, store, `SELECT balance_minor FROM account_balances WHERE account_id = ?`, []any{m2EconomyEmployerPayable}, -180)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	second, err := store.RunAgentLife(ctx, M2AgentNoonTime, 1)
	if err != nil {
		t.Fatal(err)
	}
	if second.HeadSequence != 7 || second.PendingDue != 10 {
		t.Fatalf("expected paid wage next: %+v", second)
	}
	assertM2Value(t, ctx, store, `SELECT amount_paid_minor FROM m2_economic_obligations WHERE obligation_id = ?`, []any{m2EconomyObligationID}, 180)
	assertM2Value(t, ctx, store, `SELECT balance_minor FROM account_balances WHERE account_id = ?`, []any{m2EconomyEmployerCash}, 1020)
	assertM2Value(t, ctx, store, `SELECT balance_minor FROM account_balances WHERE account_id = ?`, []any{M2DemoCohortAssetAccountID}, 7880)
	assertM2Value(t, ctx, store, `SELECT balance_minor FROM account_balances WHERE account_id = ?`, []any{m2EconomyEmployerPayable}, 0)
	assertM2Value(t, ctx, store, `SELECT population_count FROM cohorts WHERE cohort_id = ?`, []any{M2DemoCohortID}, 18)
	if differences, err := store.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatalf("M2 economy projection mismatch: %v %v", differences, err)
	}
	empty, err := store.Replay(ctx, M2DemoInstanceID, M2DemoBranchID, 7)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateSnapshot(ctx, M2DemoInstanceID, M2DemoBranchID, 6); err != nil {
		t.Fatal(err)
	}
	fromSnapshot, err := store.ReplayFromLatestSnapshot(ctx, M2DemoInstanceID, M2DemoBranchID, 7)
	if err != nil || fromSnapshot.StateHash != empty.StateHash {
		t.Fatalf("snapshot and empty replay differ: %v %v", fromSnapshot.StateHash, err)
	}
	remaining, err := store.RunAgentLife(ctx, M2AgentNoonTime, 10)
	if err != nil || remaining.HeadSequence != 17 || remaining.PendingDue != 0 {
		t.Fatalf("mixed Agent/economy run: %+v %v", remaining, err)
	}
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_economic_obligations`, nil, 2)
	assertM2Value(t, ctx, store, `SELECT amount_paid_minor FROM m2_economic_obligations WHERE obligation_id = 'obligation_m2_rent_day_1'`, nil, 320)
	assertM2Value(t, ctx, store, `SELECT balance_minor FROM account_balances WHERE account_id = ?`, []any{m2EconomyLandlordCash}, 320)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_purchase_outcomes WHERE scheduler_item_id = 'sched_m2_food_buy_day_1' AND status = 'purchased'`, nil, 1)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_purchase_outcomes WHERE scheduler_item_id = 'sched_m2_food_buy_budget_probe_day_1' AND reason_code = 'budget_exceeded'`, nil, 1)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_consumption_outcomes WHERE purchase_item_id = 'sched_m2_food_buy_day_1' AND status = 'consumed'`, nil, 1)
	assertM2Value(t, ctx, store, `SELECT quantity_minor FROM inventory_balances WHERE location_id = ? AND sku_id = ?`, []any{m2HouseholdLocation, M2DemoSKUID}, 0)
}

func TestM2PurchaseRollsBackAndRefusesProjectedStockShortage(t *testing.T) {
	ctx := context.Background()
	store := openM2AgentStore(t, ctx, filepath.Join(t.TempDir(), "food-repair.db"), false)
	defer store.Close()
	if _, err := store.PrepareM2EconomicDemo(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RunAgentLife(ctx, M2AgentNoonTime, 4); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE inventory_balances SET quantity_minor = 0 WHERE location_id = ? AND sku_id = ?`, m2StoreLocation, M2DemoSKUID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RunAgentLife(ctx, M2AgentNoonTime, 1); !core.HasCode(err, core.CodeProjectionDiverged) {
		t.Fatalf("corrupt store stock must not become an ordinary rejection: %v", err)
	}
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_purchase_outcomes`, nil, 0)
	if err := store.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	store.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "purchase interrupted") }
	if _, err := store.RunAgentLife(ctx, M2AgentNoonTime, 1); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatalf("purchase rollback: %v", err)
	}
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_purchase_outcomes`, nil, 0)
	assertM2Value(t, ctx, store, `SELECT quantity_minor FROM inventory_balances WHERE location_id = ? AND sku_id = ?`, []any{m2StoreLocation, M2DemoSKUID}, 25)
	store.beforeCommit = nil
	if _, err := store.RunAgentLife(ctx, M2AgentNoonTime, 2); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_purchase_outcomes WHERE status = 'purchased'`, nil, 1)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_purchase_outcomes WHERE status = 'rejected' AND reason_code = 'budget_exceeded'`, nil, 1)
	assertM2Value(t, ctx, store, `SELECT quantity_minor FROM inventory_balances WHERE location_id = ? AND sku_id = ?`, []any{m2StoreLocation, M2DemoSKUID}, 24)
	assertM2Value(t, ctx, store, `SELECT quantity_minor FROM inventory_balances WHERE location_id = ? AND sku_id = ?`, []any{m2HouseholdLocation, M2DemoSKUID}, 1)
	if differences, err := store.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatalf("purchase projections differ from facts: %v %v", differences, err)
	}
}

func TestM2ConsumptionRequiresCapabilityAndRollsBack(t *testing.T) {
	ctx := context.Background()
	store := openM2AgentStore(t, ctx, filepath.Join(t.TempDir(), "consume.db"), false)
	defer store.Close()
	if _, err := store.PrepareM2EconomicDemo(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RunAgentLife(ctx, m2WageTime(1, 7, 5), 6); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, store, `SELECT quantity_minor FROM inventory_balances WHERE location_id = ? AND sku_id = ?`, []any{m2HouseholdLocation, M2DemoSKUID}, 1)
	if _, err := store.db.ExecContext(ctx, `UPDATE stock_locations SET capability_id = 'wrong' WHERE location_id = ?`, m2FoodSinkLocation); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RunAgentLife(ctx, m2WageTime(1, 7, 6), 1); !core.HasCode(err, core.CodeProjectionDiverged) {
		t.Fatalf("changed sink capability must refuse consumption: %v", err)
	}
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_consumption_outcomes`, nil, 0)
	if _, err := store.db.ExecContext(ctx, `UPDATE stock_locations SET capability_id = ? WHERE location_id = ?`, m2FoodConsumeCapability, m2FoodSinkLocation); err != nil {
		t.Fatal(err)
	}
	store.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "consumption interrupted") }
	if _, err := store.RunAgentLife(ctx, m2WageTime(1, 7, 6), 1); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatalf("consumption rollback: %v", err)
	}
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_consumption_outcomes`, nil, 0)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM stock_movements WHERE movement_kind = 'consume' AND sku_id = ?`, []any{M2DemoSKUID}, 0)
	store.beforeCommit = nil
	if _, err := store.RunAgentLife(ctx, m2WageTime(1, 7, 6), 1); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_consumption_outcomes WHERE status = 'consumed'`, nil, 1)
	assertM2Value(t, ctx, store, `SELECT quantity_minor FROM inventory_balances WHERE location_id = ? AND sku_id = ?`, []any{m2HouseholdLocation, M2DemoSKUID}, 0)
	if differences, err := store.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatalf("consumption projection differs from facts: %v %v", differences, err)
	}
}

func TestM2FiniteRestockRejectsCorruptStockAndRollsBack(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "restock.db")
	store := openM2AgentStore(t, ctx, path, false)
	if _, err := store.PrepareM2EconomicDemo(ctx); err != nil {
		t.Fatal(err)
	}
	if run, err := store.RunAgentLife(ctx, m2WageTime(26, 7, 6), 400); err != nil || run.PendingDue != 0 {
		t.Fatalf("run through day-26 pre-restock boundary: %+v %v", run, err)
	}
	assertM2Value(t, ctx, store, `SELECT quantity_minor FROM inventory_balances WHERE location_id = ? AND sku_id = ?`, []any{m2StoreLocation, M2DemoSKUID}, 0)
	assertM2Value(t, ctx, store, `SELECT quantity_minor FROM inventory_balances WHERE location_id = ? AND sku_id = ?`, []any{m2SupplierLocation, M2DemoSKUID}, 10)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_purchase_outcomes WHERE day = 26 AND reason_code = 'insufficient_stock'`, nil, 1)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	var err error
	store, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.db.ExecContext(ctx, `UPDATE inventory_balances SET quantity_minor = 0 WHERE location_id = ? AND sku_id = ?`, m2SupplierLocation, M2DemoSKUID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RunAgentLife(ctx, m2WageTime(26, 7, 7), 1); !core.HasCode(err, core.CodeProjectionDiverged) {
		t.Fatalf("corrupt supplier stock must not create false shortage: %v", err)
	}
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_restock_outcomes`, nil, 0)
	if err := store.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	store.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "restock interrupted") }
	if _, err := store.RunAgentLife(ctx, m2WageTime(26, 7, 7), 1); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatalf("restock rollback: %v", err)
	}
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_restock_outcomes`, nil, 0)
	assertM2Value(t, ctx, store, `SELECT balance_minor FROM account_balances WHERE account_id = ?`, []any{m2SupplierCash}, 0)
	store.beforeCommit = nil
	if _, err := store.RunAgentLife(ctx, m2WageTime(26, 7, 7), 1); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_restock_outcomes WHERE status = 'restocked'`, nil, 1)
	assertM2Value(t, ctx, store, `SELECT balance_minor FROM account_balances WHERE account_id = ?`, []any{m2SupplierCash}, 30)
	assertM2Value(t, ctx, store, `SELECT quantity_minor FROM inventory_balances WHERE location_id = ? AND sku_id = ?`, []any{m2SupplierLocation, M2DemoSKUID}, 0)
	assertM2Value(t, ctx, store, `SELECT quantity_minor FROM inventory_balances WHERE location_id = ? AND sku_id = ?`, []any{m2StoreLocation, M2DemoSKUID}, 10)
	if differences, err := store.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatalf("restock projection differs from facts: %v %v", differences, err)
	}
}

func TestM2LateWageUsesOriginalObligationAndRollsBack(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "late-wage.db")
	store := openM2AgentStore(t, ctx, path, false)
	if _, err := store.PrepareM2EconomicDemo(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RunAgentLife(ctx, m2WageTime(15, 7, 6), 200); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_default_transitions WHERE obligation_id = 'obligation_m2_wage_day_7' AND decision = 'refinement_blocked_contract_split'`, nil, 1)
	assertM2Value(t, ctx, store, `SELECT amount_paid_minor FROM m2_economic_obligations WHERE obligation_id = 'obligation_m2_wage_day_7'`, nil, 120)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	var err error
	store, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.db.ExecContext(ctx, `UPDATE account_balances SET balance_minor = 0 WHERE account_id = ?`, m2EconomyLandlordCash); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RunAgentLife(ctx, m2WageTime(15, 7, 8), 1); !core.HasCode(err, core.CodeProjectionDiverged) {
		t.Fatalf("corrupted landlord cash cannot fake a maintenance failure: %v", err)
	}
	if err := store.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	store.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "maintenance interrupted") }
	if _, err := store.RunAgentLife(ctx, m2WageTime(15, 7, 8), 1); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatalf("maintenance rollback: %v", err)
	}
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_service_settlements`, nil, 0)
	assertM2Value(t, ctx, store, `SELECT balance_minor FROM account_balances WHERE account_id = ?`, []any{m2EconomyEmployerCash}, 0)
	store.beforeCommit = nil
	if _, err := store.RunAgentLife(ctx, m2WageTime(15, 7, 8), 1); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_service_settlements WHERE amount_minor = 60`, nil, 1)
	assertM2Value(t, ctx, store, `SELECT balance_minor FROM account_balances WHERE account_id = ?`, []any{m2EconomyEmployerCash}, 60)
	store.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "late wage interrupted") }
	if _, err := store.RunAgentLife(ctx, m2WageTime(15, 7, 9), 1); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatalf("late wage rollback: %v", err)
	}
	assertM2Value(t, ctx, store, `SELECT amount_paid_minor FROM m2_economic_obligations WHERE obligation_id = 'obligation_m2_wage_day_7'`, nil, 120)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_arrears_attempts WHERE day = 15 AND contract_id = ?`, []any{m2EconomyContractID}, 0)
	store.beforeCommit = nil
	if _, err := store.RunAgentLife(ctx, m2WageTime(15, 7, 9), 1); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, store, `SELECT amount_paid_minor FROM m2_economic_obligations WHERE obligation_id = 'obligation_m2_wage_day_7'`, nil, 180)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_arrears_attempts WHERE day = 15 AND obligation_id = 'obligation_m2_wage_day_7' AND status = 'paid' AND amount_minor = 60`, nil, 1)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_arrears_cases WHERE obligation_id = 'obligation_m2_wage_day_7' AND status = 'cured'`, nil, 1)
	assertM2Value(t, ctx, store, `SELECT balance_minor FROM account_balances WHERE account_id = ?`, []any{m2EconomyEmployerExpense}, 2700)
	if differences, err := store.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatalf("late wage projections differ from facts: %v %v", differences, err)
	}
}

func TestM2MaintenanceServiceTrueShortageIsTypedRejection(t *testing.T) {
	ctx := context.Background()
	store := openM2AgentStore(t, ctx, filepath.Join(t.TempDir(), "service-shortage.db"), false)
	defer store.Close()
	if _, err := store.PrepareM2EconomicDemo(ctx); err != nil {
		t.Fatal(err)
	}
	conn, err := store.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	mutation, err := prepareM2MaintenanceService(ctx, conn, SchedulerItem{SchedulerItemID: "sched_m2_maintenance_service_day_15", PhaseID: m2ServicePhase, WorldTime: m2WageTime(15, 7, 8)}, scheduledPayload{Kind: "m2_maintenance_service", Day: 15, SubjectID: m2ServiceOrderID})
	if err != nil {
		t.Fatal(err)
	}
	if mutation.EventType != "M2MaintenanceServiceRejected" || len(mutation.Postings) != 0 || len(mutation.Balances) != 0 {
		t.Fatalf("true landlord shortage must be a zero-transfer world decision: %+v", mutation)
	}
}

func TestM2WageRollsBackAndRejectsProjectionCorruption(t *testing.T) {
	ctx := context.Background()
	store := openM2AgentStore(t, ctx, filepath.Join(t.TempDir(), "wage-arrears.db"), false)
	defer store.Close()
	store.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "economy definition interrupted") }
	if _, err := store.PrepareM2EconomicDemo(ctx); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatalf("setup rollback: %v", err)
	}
	assertM2Value(t, ctx, store, `SELECT head_sequence FROM branches WHERE instance_id = ? AND branch_id = ?`, []any{M2DemoInstanceID, M2DemoBranchID}, 4)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_economic_actors`, nil, 0)
	store.beforeCommit = nil
	if _, err := store.PrepareM2EconomicDemo(ctx); err != nil {
		t.Fatal(err)
	}
	store.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "wage transaction interrupted") }
	if _, err := store.RunAgentLife(ctx, M2AgentNoonTime, 1); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatalf("accrual rollback: %v", err)
	}
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_economic_obligations`, nil, 0)
	assertM2Value(t, ctx, store, `SELECT head_sequence FROM branches WHERE instance_id = ? AND branch_id = ?`, []any{M2DemoInstanceID, M2DemoBranchID}, 5)
	store.beforeCommit = nil
	if _, err := store.RunAgentLife(ctx, M2AgentNoonTime, 1); err != nil {
		t.Fatal(err)
	}
	// Corrupting a projection must not look like legitimate cash exhaustion.
	if _, err := store.db.ExecContext(ctx, `UPDATE account_balances SET balance_minor = 0 WHERE account_id = ?`, m2EconomyEmployerCash); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RunAgentLife(ctx, M2AgentNoonTime, 1); !core.HasCode(err, core.CodeProjectionDiverged) {
		t.Fatalf("projection corruption must not become an unpaid wage: %v", err)
	}
	assertM2Value(t, ctx, store, `SELECT amount_paid_minor FROM m2_economic_obligations WHERE obligation_id = ?`, []any{m2EconomyObligationID}, 0)
}

func TestM2WageContractPreventsUnsourcedPopulationChange(t *testing.T) {
	ctx := context.Background()
	store := openM2AgentStore(t, ctx, filepath.Join(t.TempDir(), "economy-lineage.db"), false)
	defer store.Close()
	if _, err := store.PrepareM2EconomicDemo(ctx); err != nil {
		t.Fatal(err)
	}
	extra := m2AgentMaterialization("economy_extra", "entity_m2_economy_extra", "Extra", 1, 100, 1, 20, 10, 5, "2026-09-22T02:01:00Z")
	if _, err := store.MaterializeCohort(ctx, extra); !core.HasCode(err, core.CodeMaterializationConflict) {
		t.Fatalf("expected split-contract conflict, got %v", err)
	}
	assertM2Value(t, ctx, store, `SELECT population_count FROM cohorts WHERE cohort_id = ?`, []any{M2DemoCohortID}, 18)
	assertM2Value(t, ctx, store, `SELECT head_sequence FROM branches WHERE instance_id = ? AND branch_id = ?`, []any{M2DemoInstanceID, M2DemoBranchID}, 5)
}

func TestM2ActiveWageParticipationAccruesAndPaysNamedWorker(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "split-wage.db")
	store := openM2AgentStore(t, ctx, path, false)
	if _, err := store.PrepareM2EconomicDemo(ctx); err != nil {
		t.Fatal(err)
	}
	first, err := store.RunAgentLife(ctx, M2AgentNoonTime, 100)
	if err != nil {
		t.Fatal(err)
	}
	command := m2AgentMaterialization("wage_split", "entity_m2_wage_split", "Worker", 1, 0, 0, 0, 0, first.HeadSequence, "2026-09-23T20:01:00+08:00")
	store.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "split interrupted") }
	if _, err := store.MaterializeCohort(ctx, command); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatalf("materialization rollback: %v", err)
	}
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_wage_participation_splits`, nil, 0)
	store.beforeCommit = nil
	materialized, err := store.MaterializeCohort(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_wage_participation_splits WHERE materialization_id = ? AND effective_from = ?`, []any{command.MaterializationID, m2WageTime(2, 7, 0)}, 1)
	if replay, err := store.MaterializeCohort(ctx, command); err != nil || !replay.Replayed {
		t.Fatalf("split replay: %+v %v", replay, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	accrued, err := store.RunAgentLife(ctx, m2WageTime(2, 7, 0), 100)
	if err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, store, `SELECT due_minor FROM m2_wage_split_obligations WHERE obligation_id = 'obligation_m2_wage_day_2' AND claimant_kind = 'cohort'`, nil, 170)
	assertM2Value(t, ctx, store, `SELECT due_minor FROM m2_wage_split_obligations WHERE obligation_id = 'obligation_m2_wage_day_2' AND claimant_kind = 'entity'`, nil, 10)
	assertM2Value(t, ctx, store, `SELECT balance_minor FROM account_balances WHERE account_id = ?`, []any{"account_" + command.MaterializationID + "_receivable"}, 10)
	claimConn, err := store.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	accruedClaim := m2AgentMaterialization("claim_after_accrual", "entity_m2_claim_after_accrual", "Second", 1, 0, 0, 10, 0, accrued.HeadSequence, m2WageTime(2, 7, 0))
	accruedSlots, err := planM2WageClaimSlotTransfer(ctx, claimConn, accruedClaim)
	if err != nil || len(accruedSlots) != 1 || accruedSlots[0].SlotIndex != 16 || accruedSlots[0].Outstanding != 10 {
		t.Fatalf("accrued wage claim slot: %+v %v", accruedSlots, err)
	}
	if err := claimConn.Close(); err != nil {
		t.Fatal(err)
	}
	premature := m2AgentMaterialization("wage_split_premature", "entity_m2_wage_split_premature", "Premature", 1, 0, 0, 0, 0, accrued.HeadSequence, m2WageTime(2, 7, 0))
	if _, err := store.MaterializeCohort(ctx, premature); !core.HasCode(err, core.CodeConservationFailed) {
		t.Fatalf("open wage obligation must block another split: %v", err)
	}
	store.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "split payment interrupted") }
	if _, err := store.RunAgentLife(ctx, m2WageTime(2, 7, 1), 1); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatalf("split payment rollback: %v", err)
	}
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_wage_split_receipts WHERE obligation_id = 'obligation_m2_wage_day_2'`, nil, 0)
	store.beforeCommit = nil
	paid, err := store.RunAgentLife(ctx, m2WageTime(2, 7, 1), 1)
	if err != nil {
		t.Fatal(err)
	}
	if paid.HeadSequence != accrued.HeadSequence+1 || materialized.FirstSequence >= paid.HeadSequence {
		t.Fatalf("unexpected split sequence: %+v %+v %+v", materialized, accrued, paid)
	}
	assertM2Value(t, ctx, store, `SELECT SUM(amount_minor) FROM m2_wage_split_receipts WHERE obligation_id = 'obligation_m2_wage_day_2'`, nil, 180)
	assertM2Value(t, ctx, store, `SELECT amount_minor FROM m2_wage_split_receipts WHERE obligation_id = 'obligation_m2_wage_day_2' AND claimant_kind = 'cohort'`, nil, 170)
	assertM2Value(t, ctx, store, `SELECT amount_minor FROM m2_wage_split_receipts WHERE obligation_id = 'obligation_m2_wage_day_2' AND claimant_kind = 'entity'`, nil, 10)
	assertM2Value(t, ctx, store, `SELECT balance_minor FROM account_balances WHERE account_id = ?`, []any{m2EconomyEmployerCash}, 840)
	assertM2Value(t, ctx, store, `SELECT balance_minor FROM account_balances WHERE account_id = ?`, []any{"account_" + command.MaterializationID + "_asset"}, 10)
	assertM2Value(t, ctx, store, `SELECT balance_minor FROM account_balances WHERE account_id = ?`, []any{"account_" + command.MaterializationID + "_receivable"}, 0)
	assertM2Value(t, ctx, store, `SELECT balance_minor FROM account_balances WHERE account_id = ?`, []any{"account_" + command.MaterializationID + "_wage_income"}, -10)
	if differences, err := store.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatalf("split projection mismatch: %v %v", differences, err)
	}
	partial, err := store.RunAgentLife(ctx, m2WageTime(7, 7, 1), 1000)
	if err != nil {
		t.Fatalf("split partial wage: %+v %v", partial, err)
	}
	assertM2Value(t, ctx, store, `SELECT amount_paid_minor FROM m2_economic_obligations WHERE obligation_id = 'obligation_m2_wage_day_7'`, nil, 120)
	assertM2Value(t, ctx, store, `SELECT status = 'partial' FROM m2_economic_obligations WHERE obligation_id = 'obligation_m2_wage_day_7'`, nil, 1)
	assertM2Value(t, ctx, store, `SELECT amount_minor FROM m2_wage_split_receipts WHERE obligation_id = 'obligation_m2_wage_day_7' AND claimant_kind = 'cohort'`, nil, 114)
	assertM2Value(t, ctx, store, `SELECT amount_minor FROM m2_wage_split_receipts WHERE obligation_id = 'obligation_m2_wage_day_7' AND claimant_kind = 'entity'`, nil, 6)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_arrears_cases WHERE obligation_id = 'obligation_m2_wage_day_7' AND status = 'open'`, nil, 1)
	assertM2Value(t, ctx, store, `SELECT balance_minor FROM account_balances WHERE account_id = ?`, []any{m2EconomyEmployerCash}, 0)
	if differences, err := store.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatalf("partial split projection mismatch: %v %v", differences, err)
	}
	claimConn, err = store.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	partialClaim := m2AgentMaterialization("claim_after_partial", "entity_m2_claim_after_partial", "Third", 1, 0, 0, 4, 0, partial.HeadSequence, m2WageTime(7, 7, 1))
	partialSlots, err := planM2WageClaimSlotTransfer(ctx, claimConn, partialClaim)
	if err != nil || len(partialSlots) != 1 || partialSlots[0].SlotIndex != 16 || partialSlots[0].Outstanding != 4 {
		t.Fatalf("partial wage claim slot: %+v %v", partialSlots, err)
	}
	if err := claimConn.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO m2_wage_split_receipts(scheduler_item_id, obligation_id, claimant_kind, claimant_id, amount_minor, event_id, event_sequence) VALUES ('sched_m2_wage_pay_day_1', 'obligation_m2_wage_day_2', 'cohort', ?, 1, ?, ?)`, M2DemoCohortID, materialized.EventID, materialized.FirstSequence); err != nil {
		t.Fatal(err)
	}
	conn, err := store.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyM2ObligationPaid(ctx, conn, "obligation_m2_wage_day_2", "wage", 180); !core.HasCode(err, core.CodeProjectionDiverged) {
		t.Fatalf("forged split receipt must be rejected: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestM2WageRoundRobinAllocationMonotoneAndConserved(t *testing.T) {
	slices := []m2WageSlice{{kind: "cohort", claimant: M2DemoCohortID, due: 150}, {kind: "entity", claimant: "a", due: 10}, {kind: "entity", claimant: "b", due: 10}, {kind: "entity", claimant: "c", due: 10}}
	previous := make([]int64, len(slices))
	for paid := int64(0); paid <= 180; paid++ {
		current, err := allocateM2WageCumulative(m2WageAllocationPolicyVersion, slices, 10, 180, paid)
		if err != nil {
			t.Fatal(err)
		}
		var total int64
		for i, value := range current {
			if value < previous[i] || value > slices[i].due {
				t.Fatalf("non-monotone claimant allocation at %d: %v -> %v", paid, previous, current)
			}
			total += value
		}
		if total != paid {
			t.Fatalf("allocation lost money at %d: %v", paid, current)
		}
		previous = current
	}
	if _, err := allocateM2WageCumulative("unknown", slices, 10, 180, 120); !core.HasCode(err, core.CodeProjectionDiverged) {
		t.Fatalf("unknown policy accepted: %v", err)
	}
}

func TestM2LiveWageClaimTransfersAfterAccrualAndPaysCurrentOwner(t *testing.T) {
	ctx := context.Background()
	store := openM2AgentStore(t, ctx, filepath.Join(t.TempDir(), "live-claim.db"), false)
	defer store.Close()
	if _, err := store.PrepareM2EconomicDemo(ctx); err != nil {
		t.Fatal(err)
	}
	first, err := store.RunAgentLife(ctx, M2AgentNoonTime, 100)
	if err != nil {
		t.Fatal(err)
	}
	worker := m2AgentMaterialization("live_worker", "entity_m2_live_worker", "Worker", 1, 0, 0, 0, 0, first.HeadSequence, m2WageTime(1, 12, 1))
	if _, err := store.MaterializeCohort(ctx, worker); err != nil {
		t.Fatal(err)
	}
	accrued, err := store.RunAgentLife(ctx, m2WageTime(2, 7, 0), 100)
	if err != nil {
		t.Fatal(err)
	}
	claimant := m2AgentMaterialization("live_claimant", "entity_m2_live_claimant", "Claimant", 1, 0, 0, 10, 0, accrued.HeadSequence, m2WageTime(2, 7, 0))
	store.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "live claim transfer interrupted") }
	if _, err := store.MaterializeCohort(ctx, claimant); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatalf("T09 transfer must roll back atomically: %v", err)
	}
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_wage_claim_owner_transitions WHERE obligation_id = 'obligation_m2_wage_day_2'`, nil, 0)
	assertM2Value(t, ctx, store, `SELECT head_sequence FROM branches WHERE instance_id = ? AND branch_id = ?`, []any{M2DemoInstanceID, M2DemoBranchID}, accrued.HeadSequence)
	store.beforeCommit = nil
	transferred, err := store.MaterializeCohort(ctx, claimant)
	if err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, store, `SELECT outstanding_minor FROM m2_wage_claim_owner_transitions WHERE obligation_id = 'obligation_m2_wage_day_2' AND materialization_id = ?`, []any{claimant.MaterializationID}, 10)
	assertM2Value(t, ctx, store, `SELECT due_minor FROM m2_wage_split_obligations WHERE obligation_id = 'obligation_m2_wage_day_2' AND claimant_id = ?`, []any{M2DemoCohortID}, 170)
	store.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "slot wage payment interrupted") }
	if _, err := store.RunAgentLife(ctx, m2WageTime(2, 7, 1), 1); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatalf("slot payout must roll back atomically: %v", err)
	}
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_wage_slot_receipts WHERE obligation_id = 'obligation_m2_wage_day_2'`, nil, 0)
	assertM2Value(t, ctx, store, `SELECT amount_paid_minor FROM m2_economic_obligations WHERE obligation_id = 'obligation_m2_wage_day_2'`, nil, 0)
	store.beforeCommit = nil
	paidResult, err := store.RunAgentLife(ctx, m2WageTime(2, 7, 1), 1)
	if err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, store, `SELECT COALESCE(SUM(amount_minor), 0) FROM m2_wage_slot_receipts WHERE obligation_id = 'obligation_m2_wage_day_2' AND claimant_id = ?`, []any{claimant.EntityID}, 10)
	assertM2Value(t, ctx, store, `SELECT COALESCE(SUM(amount_minor), 0) FROM m2_wage_slot_receipts WHERE obligation_id = 'obligation_m2_wage_day_2' AND claimant_id = ?`, []any{worker.EntityID}, 10)
	assertM2Value(t, ctx, store, `SELECT COALESCE(SUM(amount_minor), 0) FROM m2_wage_slot_receipts WHERE obligation_id = 'obligation_m2_wage_day_2' AND claimant_id = ?`, []any{M2DemoCohortID}, 160)
	if repeated, err := store.RunAgentLife(ctx, m2WageTime(2, 7, 1), 1); err != nil || repeated.ProcessedItems != 0 || repeated.HeadSequence != paidResult.HeadSequence {
		t.Fatalf("slot wage retry duplicated payment: %+v %v", repeated, err)
	}
	if differences, err := store.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatalf("transferred claim projection: %v %v", differences, err)
	}
	if replay, err := store.MaterializeCohort(ctx, claimant); err != nil || !replay.Replayed || replay.FirstSequence != transferred.FirstSequence {
		t.Fatalf("transfer replay: %+v %v", replay, err)
	}
	returnCommand := core.DematerializeCohortCommand{CommandID: "cmd_live_claimant_paid_return", MaterializationID: claimant.MaterializationID, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, PrincipalID: "principal_creator", CapabilityID: "world.cohort.materialize", IdempotencyKey: "idem_live_claimant_paid_return", ExpectedHead: paidResult.HeadSequence, WorldTime: m2WageTime(2, 7, 1), ReasonCode: "lod_return"}
	returned, err := store.DematerializeCohort(ctx, returnCommand)
	if err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_wage_claim_owner_transitions WHERE materialization_id = ? AND transition_kind = 'dematerialize'`, []any{claimant.MaterializationID}, 0)
	assertM2Value(t, ctx, store, `SELECT balance_minor FROM account_balances WHERE account_id = ?`, []any{"account_" + claimant.MaterializationID + "_asset"}, 0)
	if returned.FirstSequence <= paidResult.HeadSequence {
		t.Fatalf("paid claimant return did not advance history: %+v", returned)
	}
	if repeated, err := store.DematerializeCohort(ctx, returnCommand); err != nil || !repeated.Replayed || repeated.FirstSequence != returned.FirstSequence {
		t.Fatalf("claim return replay duplicated authority: %+v %v", repeated, err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO m2_wage_slot_receipts(scheduler_item_id, obligation_id, slot_index, claimant_kind, claimant_id, amount_minor, event_id, event_sequence) VALUES ('sched_m2_wage_accrue_day_2', 'obligation_m2_wage_day_2', 16, 'entity', ?, 1, ?, ?)`, claimant.EntityID, transferred.EventID, transferred.FirstSequence); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReadM2WageClaimStatus(ctx, core.StateReadRequest{PrincipalID: "principal_creator", CapabilityID: "world.cohort.materialize", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID}, "obligation_m2_wage_day_2"); !core.HasCode(err, core.CodeProjectionDiverged) {
		t.Fatalf("orphan wage receipt must be rejected: %v", err)
	}
}

func TestM2WageClaimReturnAfterPartialAndRematerialize(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "live-return.db")
	store := openM2AgentStore(t, ctx, path, false)
	defer store.Close()
	if _, err := store.PrepareM2EconomicDemo(ctx); err != nil {
		t.Fatal(err)
	}
	first, err := store.RunAgentLife(ctx, M2AgentNoonTime, 100)
	if err != nil {
		t.Fatal(err)
	}
	worker := m2AgentMaterialization("return_worker", "entity_m2_return_worker", "Worker", 1, 0, 0, 0, 0, first.HeadSequence, m2WageTime(1, 12, 1))
	if _, err := store.MaterializeCohort(ctx, worker); err != nil {
		t.Fatal(err)
	}
	partial, err := store.RunAgentLife(ctx, m2WageTime(7, 7, 1), 1000)
	if err != nil {
		t.Fatal(err)
	}
	claimant := m2AgentMaterialization("return_claimant", "entity_m2_return_claimant", "Claimant", 1, 0, 0, 4, 0, partial.HeadSequence, m2WageTime(7, 7, 1))
	_, err = store.MaterializeCohort(ctx, claimant)
	if err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, store, `SELECT outstanding_minor FROM m2_wage_claim_owner_transitions WHERE materialization_id = ? AND transition_kind = 'materialize'`, []any{claimant.MaterializationID}, 4)
	settledDay, err := store.RunAgentLife(ctx, m2WageTime(7, 7, 10), 100)
	if err != nil {
		t.Fatal(err)
	}
	returned, err := store.DematerializeCohort(ctx, core.DematerializeCohortCommand{CommandID: "cmd_return_claimant", MaterializationID: claimant.MaterializationID, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, PrincipalID: "principal_creator", CapabilityID: "world.cohort.materialize", IdempotencyKey: "idem_return_claimant", ExpectedHead: settledDay.HeadSequence, WorldTime: m2WageTime(7, 7, 10), ReasonCode: "lod_return"})
	if err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, store, `SELECT outstanding_minor FROM m2_wage_claim_owner_transitions WHERE materialization_id = ? AND transition_kind = 'dematerialize'`, []any{claimant.MaterializationID}, 4)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_wage_participation_returns WHERE materialization_id = ?`, []any{claimant.MaterializationID}, 1)
	remat := m2AgentMaterialization("return_remat", "entity_m2_return_remat", "Replacement", 1, 0, 0, 4, 0, returned.FirstSequence, m2WageTime(7, 7, 10))
	rematerialized, err := store.MaterializeCohort(ctx, remat)
	if err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_wage_claim_owner_transitions WHERE obligation_id = 'obligation_m2_wage_day_7' AND slot_index = 16`, nil, 3)
	cured, err := store.RunAgentLife(ctx, m2WageTime(15, 7, 9), 1000)
	if err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, store, `SELECT amount_paid_minor FROM m2_economic_obligations WHERE obligation_id = 'obligation_m2_wage_day_7'`, nil, 180)
	assertM2Value(t, ctx, store, `SELECT COALESCE(SUM(amount_minor), 0) FROM m2_wage_slot_receipts WHERE obligation_id = 'obligation_m2_wage_day_7' AND claimant_id = ?`, []any{remat.EntityID}, 4)
	assertM2Value(t, ctx, store, `SELECT COALESCE(SUM(amount_minor), 0) FROM m2_wage_split_receipts WHERE obligation_id = 'obligation_m2_wage_day_7' AND claimant_id = ?`, []any{M2DemoCohortID}, 114)
	if differences, err := store.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatalf("returned claim projection: %v %v", differences, err)
	}
	if _, err := store.CreateSnapshot(ctx, M2DemoInstanceID, M2DemoBranchID, rematerialized.FirstSequence); err != nil {
		t.Fatal(err)
	}
	empty, err := store.Replay(ctx, M2DemoInstanceID, M2DemoBranchID, cured.HeadSequence)
	if err != nil {
		t.Fatal(err)
	}
	fromSnapshot, err := store.ReplayFromLatestSnapshot(ctx, M2DemoInstanceID, M2DemoBranchID, cured.HeadSequence)
	if err != nil || empty.StateHash != fromSnapshot.StateHash {
		t.Fatalf("transferred claim snapshot replay: %v %v", fromSnapshot.StateHash, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	status, err := store.ReadM2WageClaimStatus(ctx, core.StateReadRequest{PrincipalID: "principal_creator", CapabilityID: "world.cohort.materialize", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID}, "obligation_m2_wage_day_7")
	if err != nil {
		t.Fatal(err)
	}
	if status.Due != 180 || status.Paid != 180 || status.Outstanding != 0 || status.Current[16].ClaimantID != remat.EntityID {
		t.Fatalf("reopened claim status: %+v", status)
	}
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_wage_claim_owner_transitions WHERE obligation_id = 'obligation_m2_wage_day_7' AND slot_index = 16`, nil, 3)
}

func TestM2SplitBankruptcyFreezesNamedCreditorsAndDefersDistribution(t *testing.T) {
	ctx := context.Background()
	store := openM2AgentStore(t, ctx, filepath.Join(t.TempDir(), "split-bankruptcy.db"), false)
	defer store.Close()
	if _, err := store.PrepareM2EconomicDemo(ctx); err != nil {
		t.Fatal(err)
	}
	first, err := store.RunAgentLife(ctx, M2AgentNoonTime, 100)
	if err != nil {
		t.Fatal(err)
	}
	worker := m2AgentMaterialization("bankruptcy_worker", "entity_m2_bankruptcy_worker", "Worker", 1, 0, 0, 0, 0, first.HeadSequence, m2WageTime(1, 12, 1))
	if _, err := store.MaterializeCohort(ctx, worker); err != nil {
		t.Fatal(err)
	}
	throughDay28, err := store.RunAgentLife(ctx, m2WageTime(28, 7, 10), 1000)
	if err != nil {
		t.Fatal(err)
	}
	lateClaimant := m2AgentMaterialization("bankruptcy_late_claimant", "entity_m2_bankruptcy_late_claimant", "Late claimant", 1, 0, 0, 210, 0, throughDay28.HeadSequence, m2WageTime(28, 7, 10))
	if _, err := store.MaterializeCohort(ctx, lateClaimant); err != nil {
		t.Fatal(err)
	}
	opened, err := store.RunAgentLife(ctx, M2AgentDay30NoonTime, 1000)
	if err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_bankruptcy_proceedings WHERE proceeding_id = ? AND status = 'open'`, []any{m2ProceedingID}, 1)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_bankruptcy_claims WHERE proceeding_id = ?`, []any{m2ProceedingID}, 0)
	assertM2Value(t, ctx, store, `SELECT COALESCE(SUM(outstanding_at_open_minor), 0) FROM m2_bankruptcy_slot_claims WHERE proceeding_id = ? AND claimant_id = ?`, []any{m2ProceedingID, worker.EntityID}, 230)
	assertM2Value(t, ctx, store, `SELECT COALESCE(SUM(outstanding_at_open_minor), 0) FROM m2_bankruptcy_slot_claims WHERE proceeding_id = ? AND claimant_id = ?`, []any{m2ProceedingID, lateClaimant.EntityID}, 230)
	assertM2Value(t, ctx, store, `SELECT COALESCE(SUM(outstanding_at_open_minor), 0) FROM m2_bankruptcy_slot_claims WHERE proceeding_id = ? AND claimant_id = ?`, []any{m2ProceedingID, M2DemoCohortID}, 3680)
	readScope := core.StateReadRequest{PrincipalID: "principal_creator", CapabilityID: "world.cohort.materialize", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID}
	status, err := store.ReadM2WageClaimStatus(ctx, readScope, "obligation_m2_wage_day_8")
	if err != nil {
		t.Fatal(err)
	}
	if status.Due != 180 || status.Paid != 0 || status.Outstanding != 180 || len(status.Current) != 18 || len(status.Opening) != 18 || status.Opening[17].ClaimantID != worker.EntityID || status.Opening[16].OriginID != M2DemoCohortID || status.Opening[16].ClaimantID != lateClaimant.EntityID {
		t.Fatalf("named wage claim query: %+v", status)
	}
	readScope.PrincipalID = "principal_intruder"
	if _, err := store.ReadM2WageClaimStatus(ctx, readScope, "obligation_m2_wage_day_8"); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("claim status leaked without creator scope: %v", err)
	}
	conn, err := store.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyM2BankruptcyClaims(ctx, conn, "actor_m2_coop_employer"); err != nil {
		conn.Close()
		t.Fatal(err)
	}
	conn.Close()
	_, err = store.DematerializeCohort(ctx, core.DematerializeCohortCommand{CommandID: "cmd_bankruptcy_late_return", MaterializationID: lateClaimant.MaterializationID, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, PrincipalID: "principal_creator", CapabilityID: "world.cohort.materialize", IdempotencyKey: "idem_bankruptcy_late_return", ExpectedHead: opened.HeadSequence, WorldTime: M2AgentDay30NoonTime, ReasonCode: "lod_return"})
	if err != nil {
		t.Fatal(err)
	}
	status, err = store.ReadM2WageClaimStatus(ctx, core.StateReadRequest{PrincipalID: "principal_creator", CapabilityID: "world.cohort.materialize", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID}, "obligation_m2_wage_day_8")
	if err != nil {
		t.Fatal(err)
	}
	if status.Current[16].ClaimantID != M2DemoCohortID || status.Opening[16].ClaimantID != lateClaimant.EntityID {
		t.Fatalf("bankruptcy opening creditor was rewritten by later return: %+v", status)
	}
	if _, err := store.RunAgentLife(ctx, m2WageTime(31, 7, 1), 100); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_estate_distributions WHERE status = 'deferred' AND reason_code = 'slot_claim_liquidation_deferred' AND amount_minor = 0`, nil, 1)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_estate_distribution_receipts`, nil, 0)
}

func TestM2ConcurrentLiveClaimTransferCommitsOnce(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "concurrent-live-claim.db")
	firstStore := openM2AgentStore(t, ctx, path, false)
	defer firstStore.Close()
	if _, err := firstStore.PrepareM2EconomicDemo(ctx); err != nil {
		t.Fatal(err)
	}
	first, err := firstStore.RunAgentLife(ctx, M2AgentNoonTime, 100)
	if err != nil {
		t.Fatal(err)
	}
	worker := m2AgentMaterialization("concurrent_worker", "entity_m2_concurrent_worker", "Worker", 1, 0, 0, 0, 0, first.HeadSequence, m2WageTime(1, 12, 1))
	if _, err := firstStore.MaterializeCohort(ctx, worker); err != nil {
		t.Fatal(err)
	}
	partial, err := firstStore.RunAgentLife(ctx, m2WageTime(7, 7, 1), 1000)
	if err != nil {
		t.Fatal(err)
	}
	secondStore, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer secondStore.Close()
	commands := []core.MaterializeCohortCommand{
		m2AgentMaterialization("concurrent_claim_a", "entity_m2_concurrent_claim_a", "A", 1, 0, 0, 4, 0, partial.HeadSequence, m2WageTime(7, 7, 1)),
		m2AgentMaterialization("concurrent_claim_b", "entity_m2_concurrent_claim_b", "B", 1, 0, 0, 4, 0, partial.HeadSequence, m2WageTime(7, 7, 1)),
	}
	stores := []*Store{firstStore, secondStore}
	results := make([]error, 2)
	start := make(chan struct{})
	var wait sync.WaitGroup
	for i := range stores {
		wait.Add(1)
		go func(i int) { defer wait.Done(); <-start; _, results[i] = stores[i].MaterializeCohort(ctx, commands[i]) }(i)
	}
	close(start)
	wait.Wait()
	successes, conflicts := 0, 0
	for _, err := range results {
		if err == nil {
			successes++
		} else if core.HasCode(err, core.CodeBranchConflict) {
			conflicts++
		} else {
			t.Fatalf("unexpected concurrent T09 result: %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("one T09 must win: %v", results)
	}
	assertM2Value(t, ctx, firstStore, `SELECT COUNT(*) FROM m2_wage_claim_owner_transitions WHERE obligation_id = 'obligation_m2_wage_day_7' AND slot_index = 16 AND transition_kind = 'materialize'`, nil, 1)
	assertM2Value(t, ctx, firstStore, `SELECT amount_paid_minor FROM m2_economic_obligations WHERE obligation_id = 'obligation_m2_wage_day_7'`, nil, 120)
	if differences, err := firstStore.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatalf("concurrent T09 projection: %v %v", differences, err)
	}
}

func TestM2ForgedExtraClaimTransitionCannotCreateAnotherCreditor(t *testing.T) {
	ctx := context.Background()
	store := openM2AgentStore(t, ctx, filepath.Join(t.TempDir(), "forged-live-claim.db"), false)
	defer store.Close()
	if _, err := store.PrepareM2EconomicDemo(ctx); err != nil {
		t.Fatal(err)
	}
	first, err := store.RunAgentLife(ctx, M2AgentNoonTime, 100)
	if err != nil {
		t.Fatal(err)
	}
	worker := m2AgentMaterialization("forgery_worker", "entity_m2_forgery_worker", "Worker", 1, 0, 0, 0, 0, first.HeadSequence, m2WageTime(1, 12, 1))
	if _, err := store.MaterializeCohort(ctx, worker); err != nil {
		t.Fatal(err)
	}
	accrued, err := store.RunAgentLife(ctx, m2WageTime(2, 7, 0), 100)
	if err != nil {
		t.Fatal(err)
	}
	claimant := m2AgentMaterialization("forgery_claimant", "entity_m2_forgery_claimant", "Claimant", 1, 0, 0, 10, 0, accrued.HeadSequence, m2WageTime(2, 7, 0))
	transferred, err := store.MaterializeCohort(ctx, claimant)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE m2_wage_claim_owner_transitions SET outstanding_minor = 9 WHERE materialization_id = ?`, claimant.MaterializationID); err == nil {
		t.Fatal("immutable owner transition was editable")
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO m2_wage_claim_owner_transitions(transition_id, materialization_id, obligation_id, slot_index, transition_kind, from_kind, from_id, to_kind, to_id, outstanding_minor, event_id, event_sequence) VALUES ('forged_extra_claim', ?, 'obligation_m2_wage_day_2', 15, 'materialize', 'cohort', ?, 'entity', ?, 10, ?, ?)`, claimant.MaterializationID, M2DemoCohortID, claimant.EntityID, transferred.EventID, transferred.FirstSequence); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RunAgentLife(ctx, m2WageTime(2, 7, 1), 1); !core.HasCode(err, core.CodeProjectionDiverged) {
		t.Fatalf("forged extra creditor must stop payout: %v", err)
	}
	assertM2Value(t, ctx, store, `SELECT amount_paid_minor FROM m2_economic_obligations WHERE obligation_id = 'obligation_m2_wage_day_2'`, nil, 0)
}

func TestM2T09AfterUnsplitAccrualTransfersOnlyUnpaidClaim(t *testing.T) {
	ctx := context.Background()
	store := openM2AgentStore(t, ctx, filepath.Join(t.TempDir(), "unsplit-accrued-t09.db"), false)
	defer store.Close()
	if _, err := store.PrepareM2EconomicDemo(ctx); err != nil {
		t.Fatal(err)
	}
	accrued, err := store.RunAgentLife(ctx, m2WageTime(1, 7, 0), 1)
	if err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_wage_split_obligations WHERE obligation_id = 'obligation_m2_wage_day_1'`, nil, 0)
	claimant := m2AgentMaterialization("unsplit_accrued_claim", "entity_m2_unsplit_accrued_claim", "Claimant", 1, 0, 0, 10, 0, accrued.HeadSequence, m2WageTime(1, 7, 0))
	if _, err := store.MaterializeCohort(ctx, claimant); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, store, `SELECT slot_index FROM m2_wage_claim_owner_transitions WHERE obligation_id = 'obligation_m2_wage_day_1'`, nil, 17)
	if _, err := store.RunAgentLife(ctx, m2WageTime(1, 7, 1), 1); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, store, `SELECT amount_paid_minor FROM m2_economic_obligations WHERE obligation_id = 'obligation_m2_wage_day_1'`, nil, 180)
	assertM2Value(t, ctx, store, `SELECT COALESCE(SUM(amount_minor), 0) FROM m2_wage_slot_receipts WHERE obligation_id = 'obligation_m2_wage_day_1' AND claimant_id = ?`, []any{claimant.EntityID}, 10)
	assertM2Value(t, ctx, store, `SELECT COALESCE(SUM(amount_minor), 0) FROM m2_wage_slot_receipts WHERE obligation_id = 'obligation_m2_wage_day_1' AND claimant_id = ?`, []any{M2DemoCohortID}, 170)
	if differences, err := store.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatalf("unsplit accrual T09 projection: %v %v", differences, err)
	}
}

func TestM2OriginalNamedWorkerReturnPreservesEarnedCashAndUnpaidSlot(t *testing.T) {
	ctx := context.Background()
	store := openM2AgentStore(t, ctx, filepath.Join(t.TempDir(), "named-origin-return.db"), false)
	defer store.Close()
	if _, err := store.PrepareM2EconomicDemo(ctx); err != nil {
		t.Fatal(err)
	}
	first, err := store.RunAgentLife(ctx, M2AgentNoonTime, 100)
	if err != nil {
		t.Fatal(err)
	}
	worker := m2AgentMaterialization("origin_return", "entity_m2_origin_return", "Worker", 1, 0, 0, 0, 0, first.HeadSequence, m2WageTime(1, 12, 1))
	if _, err := store.MaterializeCohort(ctx, worker); err != nil {
		t.Fatal(err)
	}
	partial, err := store.RunAgentLife(ctx, m2WageTime(7, 7, 1), 1000)
	if err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, store, `SELECT balance_minor FROM account_balances WHERE account_id = ?`, []any{"account_" + worker.MaterializationID + "_asset"}, 56)
	assertM2Value(t, ctx, store, `SELECT balance_minor FROM account_balances WHERE account_id = ?`, []any{"account_" + worker.MaterializationID + "_receivable"}, 4)
	_, err = store.DematerializeCohort(ctx, core.DematerializeCohortCommand{CommandID: "cmd_origin_return", MaterializationID: worker.MaterializationID, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, PrincipalID: "principal_creator", CapabilityID: "world.cohort.materialize", IdempotencyKey: "idem_origin_return", ExpectedHead: partial.HeadSequence, WorldTime: m2WageTime(7, 7, 1), ReasonCode: "lod_return"})
	if err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, store, `SELECT outstanding_minor FROM m2_wage_claim_owner_transitions WHERE obligation_id = 'obligation_m2_wage_day_7' AND transition_kind = 'dematerialize'`, nil, 4)
	assertM2Value(t, ctx, store, `SELECT balance_minor FROM account_balances WHERE account_id = ?`, []any{"account_" + worker.MaterializationID + "_asset"}, 0)
	if _, err := store.RunAgentLife(ctx, m2WageTime(15, 7, 9), 1000); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, store, `SELECT COALESCE(SUM(amount_minor), 0) FROM m2_wage_slot_receipts WHERE obligation_id = 'obligation_m2_wage_day_7' AND claimant_id = ?`, []any{M2DemoCohortID}, 60)
	assertM2Value(t, ctx, store, `SELECT COALESCE(SUM(amount_minor), 0) FROM m2_wage_split_receipts WHERE obligation_id = 'obligation_m2_wage_day_7' AND claimant_id = ?`, []any{worker.EntityID}, 6)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_wage_split_obligations WHERE obligation_id = 'obligation_m2_wage_day_8'`, nil, 0)
	if differences, err := store.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatalf("named origin return projection: %v %v", differences, err)
	}
}

func TestM2SplitWageArrearsCurePreservesClaimantsAfterRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "split-arrears.db")
	store := openM2AgentStore(t, ctx, path, false)
	if _, err := store.PrepareM2EconomicDemo(ctx); err != nil {
		t.Fatal(err)
	}
	first, err := store.RunAgentLife(ctx, M2AgentNoonTime, 100)
	if err != nil {
		t.Fatal(err)
	}
	command := m2AgentMaterialization("split_cure", "entity_m2_split_cure", "Worker", 1, 0, 0, 0, 0, first.HeadSequence, m2WageTime(1, 12, 1))
	if _, err := store.MaterializeCohort(ctx, command); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RunAgentLife(ctx, m2WageTime(15, 7, 8), 1000); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, store, `SELECT amount_paid_minor FROM m2_economic_obligations WHERE obligation_id = 'obligation_m2_wage_day_7'`, nil, 120)
	assertM2Value(t, ctx, store, `SELECT balance_minor FROM account_balances WHERE account_id = ?`, []any{m2EconomyEmployerCash}, 60)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	store.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "split arrears interrupted") }
	if _, err := store.RunAgentLife(ctx, m2WageTime(15, 7, 9), 1); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatalf("split arrears rollback: %v", err)
	}
	assertM2Value(t, ctx, store, `SELECT amount_paid_minor FROM m2_economic_obligations WHERE obligation_id = 'obligation_m2_wage_day_7'`, nil, 120)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_wage_split_receipts WHERE obligation_id = 'obligation_m2_wage_day_7'`, nil, 2)
	store.beforeCommit = nil
	if _, err := store.RunAgentLife(ctx, m2WageTime(15, 7, 9), 1); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, store, `SELECT amount_paid_minor FROM m2_economic_obligations WHERE obligation_id = 'obligation_m2_wage_day_7'`, nil, 180)
	assertM2Value(t, ctx, store, `SELECT SUM(amount_minor) FROM m2_wage_split_receipts WHERE obligation_id = 'obligation_m2_wage_day_7' AND claimant_kind = 'cohort'`, nil, 170)
	assertM2Value(t, ctx, store, `SELECT SUM(amount_minor) FROM m2_wage_split_receipts WHERE obligation_id = 'obligation_m2_wage_day_7' AND claimant_kind = 'entity'`, nil, 10)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_arrears_cases WHERE obligation_id = 'obligation_m2_wage_day_7' AND status = 'cured'`, nil, 1)
	conn, err := store.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyM2ObligationPaid(ctx, conn, "obligation_m2_wage_day_7", "wage", 180); err != nil {
		t.Fatalf("split wage paid facts: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
	if differences, err := store.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatalf("split wage cure projection mismatch: %v %v", differences, err)
	}
}

func TestM2SplitWageZeroAndMultipleLatePayments(t *testing.T) {
	ctx := context.Background()
	store := openM2AgentStore(t, ctx, filepath.Join(t.TempDir(), "split-multiple.db"), false)
	defer store.Close()
	if _, err := store.PrepareM2EconomicDemo(ctx); err != nil {
		t.Fatal(err)
	}
	first, err := store.RunAgentLife(ctx, M2AgentNoonTime, 100)
	if err != nil {
		t.Fatal(err)
	}
	command := m2AgentMaterialization("split_multiple", "entity_m2_split_multiple", "Worker", 1, 0, 0, 0, 0, first.HeadSequence, m2WageTime(1, 12, 1))
	if _, err := store.MaterializeCohort(ctx, command); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RunAgentLife(ctx, m2WageTime(8, 7, 7), 1000); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, store, `SELECT amount_paid_minor FROM m2_economic_obligations WHERE obligation_id = 'obligation_m2_wage_day_8'`, nil, 0)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_wage_split_receipts WHERE obligation_id = 'obligation_m2_wage_day_8'`, nil, 0)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_arrears_cases WHERE obligation_id = 'obligation_m2_wage_day_8' AND status = 'open'`, nil, 1)
	// Test-only, posted transfer from existing landlord cash; no money is issued.
	const itemID = "sched_m2_test_wage_bridge_day_8"
	tx, err := beginImmediate(ctx, store.db)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.conn.ExecContext(ctx, `INSERT INTO scheduler_items(scheduler_item_id, instance_id, branch_id, world_time, phase_id, declared_priority, status, payload) VALUES (?, ?, ?, ?, ?, 0, 'pending', '{}')`, itemID, M2DemoInstanceID, M2DemoBranchID, m2WageTime(8, 7, 8), m2ServicePhase); err != nil {
		t.Fatal(err)
	}
	from, err := m2EstateAccount(ctx, tx.conn, m2EconomyLandlordCash, -30)
	if err != nil {
		t.Fatal(err)
	}
	to, err := m2EstateAccount(ctx, tx.conn, m2EconomyEmployerCash, 30)
	if err != nil {
		t.Fatal(err)
	}
	if from.NewBalance < 0 {
		t.Fatal("test donor has insufficient cash")
	}
	item := SchedulerItem{SchedulerItemID: itemID, WorldTime: m2WageTime(8, 7, 8), PhaseID: m2ServicePhase}
	payload := scheduledPayload{Kind: "test_wage_bridge", Day: 8, SubjectID: m2EconomyContractID}
	mutation := scheduledMutation{EventType: "M2TestWageBridge", EventPayload: struct {
		Amount int64 `json:"amount_minor"`
	}{30}, Postings: []scheduledPosting{{m2EconomyLandlordCash, M2DemoCurrencyID, -30, "test wage funding"}, {m2EconomyEmployerCash, M2DemoCurrencyID, 30, "test wage funding"}}, Balances: []balanceMutation{from, to}}
	if err := store.commitScheduledMutationForBranch(ctx, tx, item, payload, mutation, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RunAgentLife(ctx, m2WageTime(8, 7, 9), 1); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, store, `SELECT amount_paid_minor FROM m2_economic_obligations WHERE obligation_id = 'obligation_m2_wage_day_7'`, nil, 150)
	assertM2Value(t, ctx, store, `SELECT SUM(amount_minor) FROM m2_wage_split_receipts WHERE obligation_id = 'obligation_m2_wage_day_7' AND claimant_kind = 'cohort'`, nil, 142)
	assertM2Value(t, ctx, store, `SELECT SUM(amount_minor) FROM m2_wage_split_receipts WHERE obligation_id = 'obligation_m2_wage_day_7' AND claimant_kind = 'entity'`, nil, 8)
	if _, err := store.RunAgentLife(ctx, m2WageTime(8, 7, 9), 1); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_arrears_attempts WHERE scheduler_item_id = 'sched_m2_wage_arrears_retry_day_8'`, nil, 1)
	if _, err := store.RunAgentLife(ctx, m2WageTime(15, 7, 9), 1000); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, store, `SELECT amount_paid_minor FROM m2_economic_obligations WHERE obligation_id = 'obligation_m2_wage_day_7'`, nil, 180)
	assertM2Value(t, ctx, store, `SELECT SUM(amount_minor) FROM m2_wage_split_receipts WHERE obligation_id = 'obligation_m2_wage_day_7' AND claimant_kind = 'cohort'`, nil, 170)
	assertM2Value(t, ctx, store, `SELECT SUM(amount_minor) FROM m2_wage_split_receipts WHERE obligation_id = 'obligation_m2_wage_day_7' AND claimant_kind = 'entity'`, nil, 10)
	if differences, err := store.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatalf("multiple split payments diverged: %v %v", differences, err)
	}
}

func TestM2WagePolicyMigrationUpgradesExistingSplitLineage(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "schema-016-upgrade.db")
	store := openM2AgentStore(t, ctx, path, false)
	if _, err := store.PrepareM2EconomicDemo(ctx); err != nil {
		t.Fatal(err)
	}
	first, err := store.RunAgentLife(ctx, M2AgentNoonTime, 100)
	if err != nil {
		t.Fatal(err)
	}
	command := m2AgentMaterialization("split_upgrade", "entity_m2_split_upgrade", "Worker", 1, 0, 0, 0, 0, first.HeadSequence, m2WageTime(1, 12, 1))
	materialized, err := store.MaterializeCohort(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	// Reconstruct exactly the pre-017 schema boundary in this disposable DB.
	raw, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path))
	if err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"rp_sessions", "m2_bankruptcy_slot_claims", "m2_wage_slot_receipts", "m2_wage_participation_returns", "m2_wage_claim_owner_transitions", "m2_wage_allocation_policies"} {
		if _, err := raw.ExecContext(ctx, `DROP TABLE `+table); err != nil {
			raw.Close()
			t.Fatal(err)
		}
	}
	if _, err := raw.ExecContext(ctx, `DELETE FROM schema_meta WHERE schema_version IN (?, ?, ?, ?)`, SchemaVersion, BankruptcySlotSchemaVersion, WageClaimOwnershipSchemaVersion, WageAllocationSchemaVersion); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_wage_allocation_policies WHERE policy_id = ? AND contract_id = ?`, []any{m2WageAllocationPolicyID, m2EconomyContractID}, 1)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM schema_meta WHERE schema_version = ?`, []any{SchemaVersion}, 1)
	assertM2Value(t, ctx, store, `SELECT head_sequence FROM branches WHERE instance_id = ? AND branch_id = ?`, []any{M2DemoInstanceID, M2DemoBranchID}, materialized.FirstSequence)
	if _, err := store.RunAgentLife(ctx, m2WageTime(2, 7, 1), 100); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, store, `SELECT amount_paid_minor FROM m2_economic_obligations WHERE obligation_id = 'obligation_m2_wage_day_2'`, nil, 180)
	assertM2Value(t, ctx, store, `SELECT SUM(amount_minor) FROM m2_wage_split_receipts WHERE obligation_id = 'obligation_m2_wage_day_2'`, nil, 180)
}

func TestM2ThirtyDayFiniteEconomyRecordsPartialAndOverdue(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "m2-30-day-wage.db")
	store := openM2AgentStore(t, ctx, path, false)
	if _, err := store.PrepareM2EconomicDemo(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.DefineM2AgentRoutine(ctx, core.AgentRoutineRequest{
		PrincipalID: "principal_creator", CapabilityID: "world.agent.run",
		InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, Days: 30,
	}); err != nil {
		t.Fatal(err)
	}
	first, err := store.RunAgentLife(ctx, M2AgentDay30NoonTime, 1)
	if err != nil || first.HeadSequence != 7 || first.Status != "budget_exhausted" {
		t.Fatalf("first due item: %+v %v", first, err)
	}
	if _, err := store.CreateSnapshot(ctx, M2DemoInstanceID, M2DemoBranchID, 7); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	remaining, err := store.RunAgentLife(ctx, M2AgentDay30NoonTime, 420)
	if err != nil {
		t.Fatal(err)
	}
	if remaining.ProcessedItems != 392 || remaining.HeadSequence != 399 || remaining.PendingDue != 0 || remaining.CurrentWorldTime != M2AgentDay30NoonTime {
		t.Fatalf("30-day causal mixed queue did not complete: %+v", remaining)
	}
	for _, case_ := range []struct {
		status string
		count  int64
	}{{"paid", 7}, {"partial", 0}, {"overdue", 23}} {
		assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_economic_obligations WHERE kind = 'wage' AND status = ?`, []any{case_.status}, case_.count)
	}
	for _, case_ := range []struct {
		status string
		count  int64
	}{{"paid", 27}, {"partial", 1}, {"overdue", 2}} {
		assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_economic_obligations WHERE kind = 'rent' AND status = ?`, []any{case_.status}, case_.count)
	}
	assertM2Value(t, ctx, store, `SELECT SUM(amount_due_minor) FROM m2_economic_obligations WHERE kind = 'wage'`, nil, 5400)
	assertM2Value(t, ctx, store, `SELECT SUM(amount_paid_minor) FROM m2_economic_obligations WHERE kind = 'wage'`, nil, 1260)
	assertM2Value(t, ctx, store, `SELECT balance_minor FROM account_balances WHERE account_id = ?`, []any{m2EconomyEmployerExpense}, 5400)
	assertM2Value(t, ctx, store, `SELECT balance_minor FROM account_balances WHERE account_id = ?`, []any{m2EconomyCohortIncome}, -5400)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_service_settlements WHERE amount_minor = 60`, nil, 1)
	assertM2Value(t, ctx, store, `SELECT balance_minor FROM account_balances WHERE account_id = ?`, []any{m2LandlordServiceExpense}, 60)
	assertM2Value(t, ctx, store, `SELECT balance_minor FROM account_balances WHERE account_id = ?`, []any{m2EmployerServiceIncome}, -60)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_arrears_attempts WHERE obligation_id = 'obligation_m2_wage_day_7' AND status = 'paid' AND amount_minor = 60`, nil, 1)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_default_transitions WHERE obligation_id = 'obligation_m2_wage_day_7' AND decision = 'refinement_blocked_contract_split'`, nil, 1)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_default_transitions WHERE obligation_id = 'obligation_m2_rent_day_28' AND decision = 'rent_grace_expired_review'`, nil, 1)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_default_reviews`, nil, 30)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_insolvency_reviews WHERE status = 'proceeding_opened' AND reason_code = 'declared_employer_insolvency' AND cash_minor = 0 AND unpaid_wage_minor = 4140 AND expired_case_count > 0`, nil, 1)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_bankruptcy_proceedings p JOIN m2_insolvency_reviews r ON r.scheduler_item_id = p.opening_review_item_id AND r.event_id = p.opening_event_id AND r.event_sequence = p.opening_event_sequence JOIN events e ON e.event_id = p.opening_event_id AND e.event_sequence = p.opening_event_sequence WHERE p.proceeding_id = ? AND p.status = 'open' AND e.event_type = 'M2BankruptcyProceedingOpened'`, []any{m2ProceedingID}, 1)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_bankruptcy_claims WHERE proceeding_id = ?`, []any{m2ProceedingID}, 23)
	assertM2Value(t, ctx, store, `SELECT SUM(outstanding_at_open_minor) FROM m2_bankruptcy_claims WHERE proceeding_id = ?`, []any{m2ProceedingID}, 4140)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_bankruptcy_claims c JOIN m2_bankruptcy_proceedings p ON p.proceeding_id = c.proceeding_id WHERE c.opening_event_id = p.opening_event_id AND c.opening_event_sequence = p.opening_event_sequence AND c.claimant_cohort_id = ? AND c.currency_id = ?`, []any{M2DemoCohortID, M2DemoCurrencyID}, 23)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_bankruptcy_claims WHERE obligation_id = 'obligation_m2_wage_day_7'`, nil, 0)
	assertM2Value(t, ctx, store, `SELECT SUM(amount_due_minor) FROM m2_economic_obligations WHERE kind = 'rent'`, nil, 9600)
	assertM2Value(t, ctx, store, `SELECT SUM(amount_paid_minor) FROM m2_economic_obligations WHERE kind = 'rent'`, nil, 8830)
	assertM2Value(t, ctx, store, `SELECT amount_paid_minor FROM m2_economic_obligations WHERE obligation_id = 'obligation_m2_rent_day_28'`, nil, 190)
	assertM2Value(t, ctx, store, `SELECT amount_paid_minor FROM m2_economic_obligations WHERE obligation_id = 'obligation_m2_wage_day_7'`, nil, 180)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM events WHERE event_type = 'M2WageSettled' AND json_extract(payload, '$.reason_code') = 'insufficient_employer_liquidity' AND instance_id = ?`, []any{M2DemoInstanceID}, 24)
	assertM2Value(t, ctx, store, `SELECT balance_minor FROM account_balances WHERE account_id = ?`, []any{m2EconomyEmployerPayable}, -4140)
	assertM2Value(t, ctx, store, `SELECT balance_minor FROM account_balances WHERE account_id = ?`, []any{m2EconomyEmployerCash}, 0)
	assertM2Value(t, ctx, store, `SELECT balance_minor FROM account_balances WHERE account_id = ?`, []any{M2DemoCohortAssetAccountID}, 0)
	assertM2Value(t, ctx, store, `SELECT balance_minor FROM account_balances WHERE account_id = ?`, []any{m2EconomyLandlordCash}, 8770)
	assertM2Value(t, ctx, store, `SELECT balance_minor FROM account_balances WHERE account_id = ?`, []any{m2StoreCash}, 100)
	assertM2Value(t, ctx, store, `SELECT balance_minor FROM account_balances WHERE account_id = ?`, []any{m2SupplierCash}, 30)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_purchase_outcomes WHERE status = 'purchased'`, nil, 26)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_purchase_outcomes WHERE reason_code = 'insufficient_stock'`, nil, 1)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_purchase_outcomes WHERE reason_code = 'insufficient_funds'`, nil, 3)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_purchase_outcomes WHERE reason_code = 'budget_exceeded'`, nil, 1)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_consumption_outcomes WHERE status = 'consumed'`, nil, 26)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_consumption_outcomes WHERE reason_code = 'purchase_rejected'`, nil, 4)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM stock_movements WHERE movement_kind = 'consume' AND sku_id = ?`, []any{M2DemoSKUID}, 26)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_restock_outcomes WHERE status = 'restocked' AND spent_minor = 30 AND quantity_minor = 10`, nil, 1)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_restock_outcomes WHERE reason_code = 'insufficient_supplier_stock'`, nil, 1)
	assertM2Value(t, ctx, store, `SELECT quantity_minor FROM inventory_balances WHERE location_id = ? AND sku_id = ?`, []any{M2DemoCohortLocationID, M2DemoSKUID}, 54)
	assertM2Value(t, ctx, store, `SELECT quantity_minor FROM inventory_balances WHERE location_id = ? AND sku_id = ?`, []any{m2StoreLocation, M2DemoSKUID}, 9)
	assertM2Value(t, ctx, store, `SELECT quantity_minor FROM inventory_balances WHERE location_id = ? AND sku_id = ?`, []any{m2SupplierLocation, M2DemoSKUID}, 0)
	assertM2Value(t, ctx, store, `SELECT quantity_minor FROM inventory_balances WHERE location_id = ? AND sku_id = ?`, []any{m2HouseholdLocation, M2DemoSKUID}, 0)
	assertM2Value(t, ctx, store, `SELECT SUM(quantity_minor) FROM inventory_balances WHERE sku_id = ?`, []any{M2DemoSKUID}, 74)
	assertM2Value(t, ctx, store, `SELECT SUM(b.balance_minor) FROM account_balances b JOIN accounts a ON a.account_id = b.account_id WHERE a.currency_id = ? AND a.account_type = 'asset'`, []any{M2DemoCurrencyID}, 10000)
	assertM2Value(t, ctx, store, `SELECT balance_minor FROM account_balances WHERE account_id = ?`, []any{m2EconomyCohortRentDue}, -770)
	assertM2Value(t, ctx, store, `SELECT balance_minor FROM account_balances WHERE account_id = ?`, []any{m2EconomyLandlordRentDue}, 770)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM events WHERE event_type = 'M2RentSettled' AND json_extract(payload, '$.reason_code') = 'insufficient_tenant_liquidity' AND instance_id = ?`, []any{M2DemoInstanceID}, 3)
	assertM2Value(t, ctx, store, `SELECT population_count FROM cohorts WHERE cohort_id = ?`, []any{M2DemoCohortID}, 18)
	if differences, err := store.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) > 0 {
		t.Fatalf("30-day projection divergence: %v %v", differences, err)
	}
	empty, err := store.Replay(ctx, M2DemoInstanceID, M2DemoBranchID, 399)
	if err != nil {
		t.Fatal(err)
	}
	fromSnapshot, err := store.ReplayFromLatestSnapshot(ctx, M2DemoInstanceID, M2DemoBranchID, 399)
	if err != nil || fromSnapshot.StateHash != empty.StateHash {
		t.Fatalf("30-day snapshot mismatch: %v %v", fromSnapshot.StateHash, err)
	}
	repeat, err := store.RunAgentLife(ctx, M2AgentDay30NoonTime, 250)
	if err != nil || repeat.ProcessedItems != 0 || repeat.HeadSequence != 399 {
		t.Fatalf("30-day retry duplicated facts: %+v %v", repeat, err)
	}
}

func TestM2InsolvencyOpeningRollsBackAndResumes(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "insolvency-restart.db")
	store := openM2AgentStore(t, ctx, path, false)
	if _, err := store.PrepareM2EconomicDemo(ctx); err != nil {
		t.Fatal(err)
	}
	before, err := store.RunAgentLife(ctx, m2WageTime(30, 7, 11), 400)
	if err != nil || before.PendingDue != 0 {
		t.Fatalf("run through final default review: %+v %v", before, err)
	}
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_bankruptcy_proceedings`, nil, 0)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) > 0 FROM m2_arrears_cases WHERE kind = 'wage' AND status = 'grace_expired'`, nil, 1)
	store.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "insolvency opening interrupted") }
	if _, err := store.RunAgentLife(ctx, m2WageTime(30, 7, 12), 1); err == nil {
		t.Fatal("expected insolvency transaction rollback")
	}
	store.beforeCommit = nil
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_bankruptcy_proceedings`, nil, 0)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_insolvency_reviews`, nil, 0)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_bankruptcy_claims`, nil, 0)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM events WHERE event_type = 'M2BankruptcyProceedingOpened'`, nil, 0)
	assertM2Value(t, ctx, store, `SELECT head_sequence FROM branches WHERE instance_id = ? AND branch_id = ?`, []any{M2DemoInstanceID, M2DemoBranchID}, before.HeadSequence)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	after, err := store.RunAgentLife(ctx, m2WageTime(30, 7, 12), 1)
	if err != nil || after.ProcessedItems != 1 || after.HeadSequence != before.HeadSequence+1 {
		t.Fatalf("restart did not open one proceeding: %+v %v", after, err)
	}
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_bankruptcy_proceedings WHERE actor_id = 'actor_m2_coop_employer' AND status = 'open' AND unpaid_wage_at_open_minor = 4140`, nil, 1)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_bankruptcy_claims WHERE proceeding_id = ?`, []any{m2ProceedingID}, 23)
	assertM2Value(t, ctx, store, `SELECT balance_minor FROM account_balances WHERE account_id = ?`, []any{m2EconomyEmployerPayable}, -4140)
	assertM2Value(t, ctx, store, `SELECT balance_minor FROM account_balances WHERE account_id = ?`, []any{m2EconomyEmployerCash}, 0)
	conn, err := store.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyM2BankruptcyClaims(ctx, conn, "actor_m2_coop_employer"); err != nil {
		conn.Close()
		t.Fatalf("opening claims fail conservation check: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `UPDATE m2_bankruptcy_claims SET outstanding_at_open_minor = 1 WHERE proceeding_id = ?`, m2ProceedingID); err == nil {
		conn.Close()
		t.Fatal("immutable bankruptcy claim was changed")
	}
	if err := ensureM2EmployerOperating(ctx, conn, "actor_m2_coop_employer"); err == nil {
		conn.Close()
		t.Fatal("open proceeding allowed new employer service activity")
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
	repeat, err := store.RunAgentLife(ctx, m2WageTime(30, 7, 12), 1)
	if err != nil || repeat.ProcessedItems != 0 || repeat.HeadSequence != after.HeadSequence {
		t.Fatalf("opening replay created another fact: %+v %v", repeat, err)
	}
}

func TestM2InsolvencyReviewNeedsRealDistress(t *testing.T) {
	ctx := context.Background()
	store := openM2AgentStore(t, ctx, filepath.Join(t.TempDir(), "solvent-review.db"), false)
	defer store.Close()
	if _, err := store.PrepareM2EconomicDemo(ctx); err != nil {
		t.Fatal(err)
	}
	conn, err := store.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	item := SchedulerItem{SchedulerItemID: "sched_m2_insolvency_review_day_30", PhaseID: m2InsolvencyPhase, WorldTime: m2WageTime(30, 7, 12)}
	mutation, err := prepareM2InsolvencyReview(ctx, conn, item, scheduledPayload{Kind: "m2_insolvency_review", SubjectID: m2InsolvencyPolicyID, Day: 30})
	if closeErr := conn.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	if err != nil {
		t.Fatal(err)
	}
	if mutation.EventType != "M2InsolvencyReviewed" || len(mutation.Postings) != 0 || len(mutation.Balances) != 0 {
		t.Fatalf("solvent employer was opened or money moved: %+v", mutation)
	}
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_bankruptcy_proceedings`, nil, 0)
}

func TestM2PostTermClaimMaterializationConservesAndReturns(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "post-term-claims.db")
	store := openM2AgentStore(t, ctx, path, false)
	if _, err := store.PrepareM2EconomicDemo(ctx); err != nil {
		t.Fatal(err)
	}
	run, err := store.RunAgentLife(ctx, M2AgentDay30NoonTime, 400)
	if err != nil || run.PendingDue != 0 {
		t.Fatalf("30-day claim fixture did not settle: %+v %v", run, err)
	}
	var receivableBefore int64
	if err := store.db.QueryRowContext(ctx, `SELECT balance_minor FROM account_balances WHERE account_id = ?`, M2DemoCohortReceivableID).Scan(&receivableBefore); err != nil {
		t.Fatal(err)
	}
	claimant := m2AgentMaterialization("claimant", "entity_m2_wage_claimant", "Claimant", 1, 0, 0, 230, 0, run.HeadSequence, m2WageTime(30, 12, 1))
	wrong := claimant
	wrong.ReceivableMinor = 229
	if _, err := store.MaterializeCohort(ctx, wrong); !core.HasCode(err, core.CodeConservationFailed) {
		t.Fatalf("nonconserved wage claim allocation should fail: %v", err)
	}
	store.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "claim allocation interrupted") }
	if _, err := store.MaterializeCohort(ctx, claimant); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatalf("claim-bearing materialization should roll back: %v", err)
	}
	store.beforeCommit = nil
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_bankruptcy_claim_allocations`, nil, 0)
	assertM2Value(t, ctx, store, `SELECT head_sequence FROM branches WHERE instance_id = ? AND branch_id = ?`, []any{M2DemoInstanceID, M2DemoBranchID}, run.HeadSequence)
	materialized, err := store.MaterializeCohort(ctx, claimant)
	if err != nil || materialized.FirstSequence != run.HeadSequence+1 {
		t.Fatalf("post-term claim assignment failed: %+v %v", materialized, err)
	}
	assertM2Value(t, ctx, store, `SELECT population_count FROM cohorts WHERE cohort_id = ?`, []any{M2DemoCohortID}, 17)
	assertM2Value(t, ctx, store, `SELECT balance_minor FROM account_balances WHERE account_id = ?`, []any{M2DemoCohortReceivableID}, receivableBefore-230)
	assertM2Value(t, ctx, store, `SELECT balance_minor FROM account_balances WHERE account_id = 'account_mat_m2_agent_claimant_receivable'`, nil, 230)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_bankruptcy_claim_allocations WHERE materialization_id = ? AND amount_minor = 10`, []any{claimant.MaterializationID}, 23)
	assertM2Value(t, ctx, store, `SELECT SUM(amount_minor) FROM m2_bankruptcy_claim_allocations WHERE materialization_id = ?`, []any{claimant.MaterializationID}, 230)
	assertM2Value(t, ctx, store, `SELECT SUM(outstanding_at_open_minor) FROM m2_bankruptcy_claims`, nil, 4140)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM events WHERE event_id = ? AND json_extract(payload, '$.claim_allocation_count') = 23 AND length(json_extract(payload, '$.claim_allocation_hash')) > 0`, []any{materialized.EventID}, 1)
	if _, err := store.db.ExecContext(ctx, `INSERT INTO m2_bankruptcy_claim_returns(materialization_id, obligation_id, amount_minor, return_event_id, return_event_sequence) VALUES (?, 'obligation_m2_wage_day_8', 11, ?, ?)`, claimant.MaterializationID, materialized.EventID, materialized.FirstSequence); err == nil {
		t.Fatal("claim return with amount different from its assignment bypassed foreign key")
	}
	conn, err := store.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	postOpeningRetry := SchedulerItem{SchedulerItemID: "sched_m2_wage_arrears_retry_day_30", PhaseID: m2WageRetryPhase, WorldTime: m2WageTime(30, 7, 9)}
	_, payoutErr := prepareM2ArrearsRetry(ctx, conn, postOpeningRetry, scheduledPayload{Kind: "m2_wage_arrears_retry", Day: 30, SubjectID: m2EconomyContractID})
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
	if !core.HasCode(payoutErr, core.CodeBranchConflict) {
		t.Fatalf("post-opening wage retry misrouted split claims: %v", payoutErr)
	}
	if differences, err := store.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatalf("post-term claim split projection divergence: %v %v", differences, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	replayed, err := store.MaterializeCohort(ctx, claimant)
	if err != nil || !replayed.Replayed || replayed.EventID != materialized.EventID {
		t.Fatalf("claim-bearing materialization replay failed: %+v %v", replayed, err)
	}
	returnCommand := core.DematerializeCohortCommand{CommandID: "cmd_m2_claimant_return", MaterializationID: claimant.MaterializationID, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, PrincipalID: "principal_creator", CapabilityID: "world.cohort.materialize", IdempotencyKey: "idem_m2_claimant_return", ExpectedHead: materialized.FirstSequence, WorldTime: m2WageTime(30, 12, 2), ReasonCode: "lod_return"}
	returned, err := store.DematerializeCohort(ctx, returnCommand)
	if err != nil {
		t.Fatalf("claim-bearing dematerialization failed: %v", err)
	}
	assertM2Value(t, ctx, store, `SELECT population_count FROM cohorts WHERE cohort_id = ?`, []any{M2DemoCohortID}, 18)
	assertM2Value(t, ctx, store, `SELECT balance_minor FROM account_balances WHERE account_id = ?`, []any{M2DemoCohortReceivableID}, receivableBefore)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_bankruptcy_claim_returns WHERE materialization_id = ? AND amount_minor = 10`, []any{claimant.MaterializationID}, 23)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM events WHERE event_id = ? AND json_extract(payload, '$.claim_return_count') = 23`, []any{returned.EventID}, 1)
	returnConn, err := store.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	lineageErr := verifyM2ClaimAllocationLineage(ctx, returnConn, claimant.MaterializationID)
	if err := returnConn.Close(); err != nil {
		t.Fatal(err)
	}
	if lineageErr != nil {
		t.Fatalf("claim assignment/return lineage differs from events: %v", lineageErr)
	}
	returnReplay, err := store.DematerializeCohort(ctx, returnCommand)
	if err != nil || !returnReplay.Replayed || returnReplay.EventID != returned.EventID {
		t.Fatalf("claim return replay duplicated authority: %+v %v", returnReplay, err)
	}
	if differences, err := store.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatalf("post-term claim return projection divergence: %v %v", differences, err)
	}
	if _, err := store.CreateSnapshot(ctx, M2DemoInstanceID, M2DemoBranchID, materialized.FirstSequence); err != nil {
		t.Fatal(err)
	}
	empty, err := store.Replay(ctx, M2DemoInstanceID, M2DemoBranchID, returned.FirstSequence)
	if err != nil {
		t.Fatal(err)
	}
	fromSnapshot, err := store.ReplayFromLatestSnapshot(ctx, M2DemoInstanceID, M2DemoBranchID, returned.FirstSequence)
	if err != nil || empty.StateHash != fromSnapshot.StateHash {
		t.Fatalf("post-term claim return snapshot replay mismatch: %v %v", fromSnapshot.StateHash, err)
	}
}

func TestM2EstateDistributionRoutesPostedCashToClaimants(t *testing.T) {
	for _, named := range []bool{false, true} {
		label := "cohort"
		if named {
			label = "named"
		}
		t.Run(label, func(t *testing.T) {
			ctx := context.Background()
			path := filepath.Join(t.TempDir(), "estate.db")
			store := openM2AgentStore(t, ctx, path, false)
			if _, err := store.PrepareM2EconomicDemo(ctx); err != nil {
				t.Fatal(err)
			}
			base, err := store.RunAgentLife(ctx, M2AgentDay30NoonTime, 400)
			if err != nil || base.PendingDue != 0 || base.HeadSequence != 282 {
				t.Fatalf("day-30 baseline: %+v %v", base, err)
			}
			cohortCashBefore := m2TestValue(t, ctx, store, `SELECT balance_minor FROM account_balances WHERE account_id = ?`, M2DemoCohortAssetAccountID)
			cohortDueBefore := m2TestValue(t, ctx, store, `SELECT balance_minor FROM account_balances WHERE account_id = ?`, M2DemoCohortReceivableID)
			if named {
				claimant := m2AgentMaterialization("estate", "entity_m2_estate_claimant", "Estate claimant", 1, 0, 0, 230, 0, base.HeadSequence, m2WageTime(30, 12, 1))
				if _, err := store.MaterializeCohort(ctx, claimant); err != nil {
					t.Fatal(err)
				}
				cohortDueBefore -= 230
			}
			headBefore := m2TestValue(t, ctx, store, `SELECT head_sequence FROM branches WHERE instance_id = ? AND branch_id = ?`, M2DemoInstanceID, M2DemoBranchID)
			store.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "estate contribution interrupted") }
			if _, err := store.RunAgentLife(ctx, m2WageTime(31, 7, 0), 1); !core.HasCode(err, core.CodeInjectedFailure) {
				t.Fatalf("contribution should roll back: %v", err)
			}
			store.beforeCommit = nil
			assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_estate_contributions`, nil, 0)
			assertM2Value(t, ctx, store, `SELECT head_sequence FROM branches WHERE instance_id = ? AND branch_id = ?`, []any{M2DemoInstanceID, M2DemoBranchID}, headBefore)
			contributed, err := store.RunAgentLife(ctx, m2WageTime(31, 7, 0), 1)
			if err != nil || contributed.HeadSequence != headBefore+1 {
				t.Fatalf("estate funding: %+v %v", contributed, err)
			}
			assertM2Value(t, ctx, store, `SELECT balance_minor FROM account_balances WHERE account_id = ?`, []any{m2EconomyLandlordCash}, 8752)
			assertM2Value(t, ctx, store, `SELECT balance_minor FROM account_balances WHERE account_id = ?`, []any{m2EconomyEmployerCash}, 18)
			store.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "estate distribution interrupted") }
			if _, err := store.RunAgentLife(ctx, m2WageTime(31, 7, 1), 1); !core.HasCode(err, core.CodeInjectedFailure) {
				t.Fatalf("distribution should roll back: %v", err)
			}
			store.beforeCommit = nil
			assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_estate_distributions`, nil, 0)
			assertM2Value(t, ctx, store, `SELECT balance_minor FROM account_balances WHERE account_id = ?`, []any{m2EconomyEmployerCash}, 18)
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}
			store, err = Open(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			paid, err := store.RunAgentLife(ctx, m2WageTime(31, 7, 1), 1)
			if err != nil || paid.PendingDue != 0 || paid.HeadSequence != contributed.HeadSequence+1 {
				t.Fatalf("estate distribution after reopen: %+v %v", paid, err)
			}
			assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_estate_contributions WHERE status = 'paid' AND amount_minor = 18`, nil, 1)
			assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_estate_distributions WHERE status = 'paid' AND amount_minor = 18 AND obligation_id = 'obligation_m2_wage_day_8'`, nil, 1)
			assertM2Value(t, ctx, store, `SELECT amount_paid_minor FROM m2_economic_obligations WHERE obligation_id = 'obligation_m2_wage_day_8'`, nil, 18)
			assertM2Value(t, ctx, store, `SELECT balance_minor FROM account_balances WHERE account_id = ?`, []any{m2EconomyEmployerCash}, 0)
			assertM2Value(t, ctx, store, `SELECT balance_minor FROM account_balances WHERE account_id = ?`, []any{m2EconomyEmployerPayable}, -4122)
			cohortShare := int64(18)
			receiptCount := int64(1)
			if named {
				cohortShare, receiptCount = 17, 2
			}
			assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_estate_distribution_receipts`, nil, receiptCount)
			assertM2Value(t, ctx, store, `SELECT SUM(amount_minor) FROM m2_estate_distribution_receipts`, nil, 18)
			assertM2Value(t, ctx, store, `SELECT balance_minor FROM account_balances WHERE account_id = ?`, []any{M2DemoCohortAssetAccountID}, cohortCashBefore+cohortShare)
			assertM2Value(t, ctx, store, `SELECT balance_minor FROM account_balances WHERE account_id = ?`, []any{M2DemoCohortReceivableID}, cohortDueBefore-cohortShare)
			if named {
				assertM2Value(t, ctx, store, `SELECT balance_minor FROM account_balances WHERE account_id = 'account_mat_m2_agent_estate_asset'`, nil, 1)
				assertM2Value(t, ctx, store, `SELECT balance_minor FROM account_balances WHERE account_id = 'account_mat_m2_agent_estate_receivable'`, nil, 229)
			}
			if differences, err := store.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
				t.Fatalf("estate projection divergence: %v %v", differences, err)
			}
			again, err := store.RunAgentLife(ctx, m2WageTime(31, 7, 1), 1)
			if err != nil || again.HeadSequence != paid.HeadSequence || again.ProcessedItems != 0 {
				t.Fatalf("estate distribution duplicated on rerun: %+v %v", again, err)
			}
			if _, err := store.CreateSnapshot(ctx, M2DemoInstanceID, M2DemoBranchID, contributed.HeadSequence); err != nil {
				t.Fatal(err)
			}
			empty, err := store.Replay(ctx, M2DemoInstanceID, M2DemoBranchID, paid.HeadSequence)
			if err != nil {
				t.Fatal(err)
			}
			fromSnapshot, err := store.ReplayFromLatestSnapshot(ctx, M2DemoInstanceID, M2DemoBranchID, paid.HeadSequence)
			if err != nil || fromSnapshot.StateHash != empty.StateHash {
				t.Fatalf("estate snapshot replay mismatch: %v %v", fromSnapshot.StateHash, err)
			}
			if named {
				returnAfterPayment := core.DematerializeCohortCommand{CommandID: "cmd_m2_estate_return_after_payment", MaterializationID: "mat_m2_agent_estate", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, PrincipalID: "principal_creator", CapabilityID: "world.cohort.materialize", IdempotencyKey: "idem_m2_estate_return_after_payment", ExpectedHead: paid.HeadSequence, WorldTime: m2WageTime(31, 7, 2), ReasonCode: "lod_return"}
				if _, err := store.DematerializeCohort(ctx, returnAfterPayment); !core.HasCode(err, core.CodeConservationFailed) {
					t.Fatalf("paid assigned claim should not be returned as unpaid: %v", err)
				}
				tx, err := beginImmediate(ctx, store.db)
				if err != nil {
					t.Fatal(err)
				}
				var eventID string
				var sequence int64
				if err := tx.conn.QueryRowContext(ctx, `SELECT event_id, event_sequence FROM m2_estate_distributions WHERE scheduler_item_id = 'sched_m2_estate_distribution_day_31'`).Scan(&eventID, &sequence); err != nil {
					t.Fatal(err)
				}
				if _, err := tx.conn.ExecContext(ctx, `INSERT INTO m2_estate_distribution_receipts(scheduler_item_id, claimant_kind, claimant_id, asset_account_id, receivable_account_id, amount_minor, event_id, event_sequence) VALUES ('sched_m2_estate_distribution_day_31', 'entity', 'actor_m2_landlord', ?, ?, 1, ?, ?)`, m2EconomyLandlordCash, m2EconomyLandlordRentDue, eventID, sequence); err != nil {
					t.Fatal(err)
				}
				if err := verifyM2ObligationPaid(ctx, tx.conn, "obligation_m2_wage_day_8", "wage", 18); !core.HasCode(err, core.CodeProjectionDiverged) {
					t.Fatalf("fabricated estate receipt escaped lineage check: %v", err)
				}
				tx.Rollback(ctx)
			}
		})
	}
}

func TestM2EstateDefersWhenDonorCashWasPostedAway(t *testing.T) {
	ctx := context.Background()
	store := openM2AgentStore(t, ctx, filepath.Join(t.TempDir(), "unfunded-estate.db"), false)
	defer store.Close()
	if _, err := store.PrepareM2EconomicDemo(ctx); err != nil {
		t.Fatal(err)
	}
	if run, err := store.RunAgentLife(ctx, M2AgentDay30NoonTime, 400); err != nil || run.PendingDue != 0 {
		t.Fatalf("day-30 fixture: %+v %v", run, err)
	}
	// A test-only, posted gift removes the landlord's cash before its declared
	// contribution. Both sides are checked against authority; projection-only
	// manipulation would be rejected by the production writer.
	tx, err := beginImmediate(ctx, store.db)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	const itemID = "sched_m2_test_landlord_gift_day_30"
	payload := scheduledPayload{Kind: "test_landlord_gift", Day: 30, SubjectID: "actor_m2_landlord"}
	if _, err := tx.conn.ExecContext(ctx, `INSERT INTO scheduler_items(scheduler_item_id, instance_id, branch_id, world_time, phase_id, declared_priority, status, payload) VALUES (?, ?, ?, ?, ?, 0, 'pending', '{}')`, itemID, M2DemoInstanceID, M2DemoBranchID, m2WageTime(30, 12, 0), m2EstateContributionPhase); err != nil {
		t.Fatal(err)
	}
	if err := verifyM2AccountProjection(ctx, tx.conn, m2EconomyLandlordCash, M2DemoCurrencyID); err != nil {
		t.Fatal(err)
	}
	if err := verifyM2AccountProjection(ctx, tx.conn, M2DemoCohortAssetAccountID, M2DemoCurrencyID); err != nil {
		t.Fatal(err)
	}
	gift, _, err := readScheduledBalance(ctx, tx.conn, m2EconomyLandlordCash)
	if err != nil {
		t.Fatal(err)
	}
	donorUpdate, err := m2EstateAccount(ctx, tx.conn, m2EconomyLandlordCash, -gift)
	if err != nil {
		t.Fatal(err)
	}
	cohortUpdate, err := m2EstateAccount(ctx, tx.conn, M2DemoCohortAssetAccountID, gift)
	if err != nil {
		t.Fatal(err)
	}
	mutation := scheduledMutation{EventType: "M2TestLandlordGift", EventPayload: struct {
		Amount int64 `json:"amount_minor"`
	}{gift},
		Postings: []scheduledPosting{{m2EconomyLandlordCash, M2DemoCurrencyID, -gift, "test landlord gift"}, {M2DemoCohortAssetAccountID, M2DemoCurrencyID, gift, "test Cohort gift"}},
		Balances: []balanceMutation{donorUpdate, cohortUpdate}}
	if err := store.commitScheduledMutationForBranch(ctx, tx, SchedulerItem{SchedulerItemID: itemID, WorldTime: m2WageTime(30, 12, 0), PhaseID: m2EstateContributionPhase}, payload, mutation, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	run, err := store.RunAgentLife(ctx, m2WageTime(31, 7, 1), 2)
	if err != nil || run.PendingDue != 0 || run.ProcessedItems != 2 {
		t.Fatalf("unfunded estate schedule: %+v %v", run, err)
	}
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_estate_contributions WHERE status = 'deferred' AND reason_code = 'insufficient_donor_cash' AND amount_minor = 0`, nil, 1)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_estate_distributions WHERE status = 'deferred' AND reason_code = 'contribution_deferred' AND amount_minor = 0`, nil, 1)
	assertM2Value(t, ctx, store, `SELECT amount_paid_minor FROM m2_economic_obligations WHERE obligation_id = 'obligation_m2_wage_day_8'`, nil, 0)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_estate_distribution_receipts`, nil, 0)
	if differences, err := store.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatalf("unfunded estate projection divergence: %v %v", differences, err)
	}
}

func TestM2PostPaymentResidualClaimCanBeAssignedAndReturned(t *testing.T) {
	for _, priorNamed := range []bool{false, true} {
		label := "cohort_only_payout"
		if priorNamed {
			label = "prior_named_payout"
		}
		t.Run(label, func(t *testing.T) {
			ctx := context.Background()
			path := filepath.Join(t.TempDir(), "residual-claim.db")
			store := openM2AgentStore(t, ctx, path, false)
			if _, err := store.PrepareM2EconomicDemo(ctx); err != nil {
				t.Fatal(err)
			}
			base, err := store.RunAgentLife(ctx, M2AgentDay30NoonTime, 400)
			if err != nil || base.PendingDue != 0 {
				t.Fatalf("day-30 fixture: %+v %v", base, err)
			}
			if priorNamed {
				prior := m2AgentMaterialization("prior_residual", "entity_m2_prior_residual", "Prior claimant", 1, 0, 0, 230, 0, base.HeadSequence, m2WageTime(30, 12, 1))
				if _, err := store.MaterializeCohort(ctx, prior); err != nil {
					t.Fatal(err)
				}
			}
			paid, err := store.RunAgentLife(ctx, m2WageTime(31, 7, 1), 2)
			if err != nil || paid.PendingDue != 0 {
				t.Fatalf("day-31 payout: %+v %v", paid, err)
			}
			assertM2Value(t, ctx, store, `SELECT amount_paid_minor FROM m2_economic_obligations WHERE obligation_id = 'obligation_m2_wage_day_8'`, nil, 18)
			bad := m2AgentMaterialization("residual_wrong", "entity_m2_residual_wrong", "Wrong residual", 1, 0, 0, 230, 0, paid.HeadSequence, m2WageTime(31, 7, 2))
			if _, err := store.MaterializeCohort(ctx, bad); !core.HasCode(err, core.CodeConservationFailed) {
				t.Fatalf("already-paid share was assigned again: %v", err)
			}
			residual := m2AgentMaterialization("residual", "entity_m2_residual", "Residual claimant", 1, 0, 0, 229, 0, paid.HeadSequence, m2WageTime(31, 7, 2))
			store.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "residual assignment interrupted") }
			if _, err := store.MaterializeCohort(ctx, residual); !core.HasCode(err, core.CodeInjectedFailure) {
				t.Fatalf("residual assignment should roll back: %v", err)
			}
			store.beforeCommit = nil
			assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_bankruptcy_claim_allocations WHERE materialization_id = ?`, []any{residual.MaterializationID}, 0)
			assigned, err := store.MaterializeCohort(ctx, residual)
			if err != nil || assigned.FirstSequence != paid.HeadSequence+1 {
				t.Fatalf("residual assignment: %+v %v", assigned, err)
			}
			assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_bankruptcy_claim_allocations WHERE materialization_id = ?`, []any{residual.MaterializationID}, 23)
			assertM2Value(t, ctx, store, `SELECT amount_minor FROM m2_bankruptcy_claim_allocations WHERE materialization_id = ? AND obligation_id = 'obligation_m2_wage_day_8'`, []any{residual.MaterializationID}, 9)
			assertM2Value(t, ctx, store, `SELECT SUM(amount_minor) FROM m2_bankruptcy_claim_allocations WHERE materialization_id = ?`, []any{residual.MaterializationID}, 229)
			assertM2Value(t, ctx, store, `SELECT balance_minor FROM account_balances WHERE account_id = 'account_mat_m2_agent_residual_receivable'`, nil, 229)
			assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM events WHERE event_id = ? AND json_extract(payload, '$.claim_allocation_count') = 23 AND length(json_extract(payload, '$.claim_allocation_hash')) > 0`, []any{assigned.EventID}, 1)
			if differences, err := store.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
				t.Fatalf("residual allocation projection: %v %v", differences, err)
			}
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}
			store, err = Open(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			replayed, err := store.MaterializeCohort(ctx, residual)
			if err != nil || !replayed.Replayed || replayed.EventID != assigned.EventID {
				t.Fatalf("residual replay: %+v %v", replayed, err)
			}
			returnCommand := core.DematerializeCohortCommand{CommandID: "cmd_m2_residual_return", MaterializationID: residual.MaterializationID, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, PrincipalID: "principal_creator", CapabilityID: "world.cohort.materialize", IdempotencyKey: "idem_m2_residual_return", ExpectedHead: assigned.FirstSequence, WorldTime: m2WageTime(31, 7, 3), ReasonCode: "lod_return"}
			returned, err := store.DematerializeCohort(ctx, returnCommand)
			if err != nil {
				t.Fatalf("unpaid residual should return after earlier Cohort payment: %v", err)
			}
			assertM2Value(t, ctx, store, `SELECT SUM(amount_minor) FROM m2_bankruptcy_claim_returns WHERE materialization_id = ?`, []any{residual.MaterializationID}, 229)
			assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_bankruptcy_claim_returns WHERE materialization_id = ?`, []any{residual.MaterializationID}, 23)
			assertM2Value(t, ctx, store, `SELECT amount_paid_minor FROM m2_economic_obligations WHERE obligation_id = 'obligation_m2_wage_day_8'`, nil, 18)
			if differences, err := store.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
				t.Fatalf("residual return projection: %v %v", differences, err)
			}
			if _, err := store.CreateSnapshot(ctx, M2DemoInstanceID, M2DemoBranchID, assigned.FirstSequence); err != nil {
				t.Fatal(err)
			}
			empty, err := store.Replay(ctx, M2DemoInstanceID, M2DemoBranchID, returned.FirstSequence)
			if err != nil {
				t.Fatal(err)
			}
			fromSnapshot, err := store.ReplayFromLatestSnapshot(ctx, M2DemoInstanceID, M2DemoBranchID, returned.FirstSequence)
			if err != nil || fromSnapshot.StateHash != empty.StateHash {
				t.Fatalf("residual replay/snapshot: %v %v", fromSnapshot.StateHash, err)
			}
		})
	}
}

func m2TestValue(t *testing.T, ctx context.Context, store *Store, query string, args ...any) int64 {
	t.Helper()
	var value int64
	if err := store.db.QueryRowContext(ctx, query, args...).Scan(&value); err != nil {
		t.Fatal(err)
	}
	return value
}

func TestM2RentRollbackAndProjectionRepair(t *testing.T) {
	ctx := context.Background()
	store := openM2AgentStore(t, ctx, filepath.Join(t.TempDir(), "rent-repair.db"), false)
	defer store.Close()
	if _, err := store.PrepareM2EconomicDemo(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RunAgentLife(ctx, M2AgentNoonTime, 2); err != nil {
		t.Fatal(err)
	}
	store.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "rent accrual interrupted") }
	if _, err := store.RunAgentLife(ctx, M2AgentNoonTime, 1); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatalf("expected rent accrual rollback, got %v", err)
	}
	assertM2Value(t, ctx, store, `SELECT head_sequence FROM branches WHERE instance_id = ? AND branch_id = ?`, []any{M2DemoInstanceID, M2DemoBranchID}, 7)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM m2_economic_obligations WHERE kind = 'rent'`, nil, 0)
	store.beforeCommit = nil
	if _, err := store.RunAgentLife(ctx, M2AgentNoonTime, 1); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, store, `SELECT balance_minor FROM account_balances WHERE account_id = ?`, []any{m2EconomyCohortRentDue}, -320)
	if _, err := store.db.ExecContext(ctx, `UPDATE account_balances SET balance_minor = 0 WHERE account_id = ?`, M2DemoCohortAssetAccountID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RunAgentLife(ctx, M2AgentNoonTime, 1); !core.HasCode(err, core.CodeProjectionDiverged) {
		t.Fatalf("corrupt cash must not manufacture rent delinquency: %v", err)
	}
	assertM2Value(t, ctx, store, `SELECT amount_paid_minor FROM m2_economic_obligations WHERE obligation_id = 'obligation_m2_rent_day_1'`, nil, 0)
	if err := store.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RunAgentLife(ctx, M2AgentNoonTime, 1); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, store, `SELECT amount_paid_minor FROM m2_economic_obligations WHERE obligation_id = 'obligation_m2_rent_day_1'`, nil, 320)
	if differences, err := store.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatalf("projection repair failed: %v %v", differences, err)
	}
}
