package storage

import (
	"context"
	"math"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestM2WageClaimOriginIsSealedAndProjectionChecked(t *testing.T) {
	ctx := context.Background()
	store := openM2AgentStore(t, ctx, filepath.Join(t.TempDir(), "wage-origin.db"), false)
	defer store.Close()
	if _, err := store.PrepareM2EconomicDemo(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RunAgentLife(ctx, m2WageTime(1, 7, 0), 100); err != nil {
		t.Fatal(err)
	}
	conn, err := store.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	for _, tc := range []struct {
		name, mutation string
		wantError      bool
	}{
		{"original", "", false},
		// Diagnostic mutation only: not a supported contract amendment API.
		{"different current workforce and rate", "UPDATE m2_cohort_contracts SET participant_count = 17, unit_rate_minor = 11 WHERE contract_id = '" + m2EconomyContractID + "'", false},
		{"changed amount", "UPDATE m2_economic_obligations SET amount_due_minor = 170 WHERE obligation_id = 'obligation_m2_wage_day_1'", true},
		{"wrong accrual period", "UPDATE m2_economic_obligations SET period_end = '2026-09-24T07:00:00Z' WHERE obligation_id = 'obligation_m2_wage_day_1'", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := conn.ExecContext(ctx, "BEGIN"); err != nil {
				t.Fatal(err)
			}
			defer func() {
				if _, err := conn.ExecContext(ctx, "ROLLBACK"); err != nil {
					t.Error(err)
				}
			}()
			if tc.mutation != "" {
				if _, err := conn.ExecContext(ctx, tc.mutation); err != nil {
					t.Fatal(err)
				}
			}
			slots, err := loadM2WageClaimSlots(ctx, conn, "obligation_m2_wage_day_1", math.MaxInt64)
			if tc.wantError {
				if !core.HasCode(err, core.CodeProjectionDiverged) {
					t.Fatalf("must reject altered accrual projection: %v", err)
				}
				return
			}
			if err != nil || len(slots) != 18 {
				t.Fatalf("original workforce must retain 18 shares: %d %v", len(slots), err)
			}
			for _, slot := range slots {
				if slot.Due != 10 || slot.Paid != 0 || slot.OwnerID != M2DemoCohortID {
					t.Fatalf("original earned share changed: %+v", slot)
				}
			}
		})
	}
}

func TestM2WageReceiptHistoryUsesEarnedRate(t *testing.T) {
	ctx := context.Background()
	store := openM2AgentStore(t, ctx, filepath.Join(t.TempDir(), "receipt-origin.db"), false)
	defer store.Close()
	if _, err := store.PrepareM2EconomicDemo(ctx); err != nil {
		t.Fatal(err)
	}
	first, err := store.RunAgentLife(ctx, M2AgentNoonTime, 100)
	if err != nil {
		t.Fatal(err)
	}
	worker := m2AgentMaterialization("origin_worker", "entity_origin_worker", "Worker", 1, 0, 0, 0, 0, first.HeadSequence, m2WageTime(1, 12, 1))
	if _, err := store.MaterializeCohort(ctx, worker); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RunAgentLife(ctx, m2WageTime(2, 7, 1), 100); err != nil {
		t.Fatal(err)
	}
	conn, err := store.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, "BEGIN"); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := conn.ExecContext(ctx, "ROLLBACK"); err != nil {
			t.Error(err)
		}
	}()
	// Simulate changed current terms only inside a rolled-back diagnostic transaction.
	if _, err := conn.ExecContext(ctx, `UPDATE m2_cohort_contracts SET participant_count = 17, unit_rate_minor = 11 WHERE contract_id = ?`, m2EconomyContractID); err != nil {
		t.Fatal(err)
	}
	if err := verifyM2WageSplitReceipts(ctx, conn, "obligation_m2_wage_day_2", 180); err != nil {
		t.Fatalf("actual old receipts must still match the earned rate: %v", err)
	}
	if err := verifyM2WageSlotHistory(ctx, conn, "obligation_m2_wage_day_2", 180); err != nil {
		t.Fatalf("slot history must retain the same earned rate: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO m2_wage_split_obligations
		(obligation_id, claimant_kind, claimant_id, due_minor, asset_account_id, receivable_account_id, income_account_id, accrual_event_id, accrual_event_sequence)
		SELECT s.obligation_id, 'entity', s.claimant_id, 10, s.asset_account_id, s.receivable_account_id, s.income_account_id, e.event_id, e.event_sequence
		FROM m2_wage_split_obligations s
		JOIN m2_economic_obligations old ON old.obligation_id = 'obligation_m2_wage_day_1'
		JOIN events e ON e.event_id = old.defining_event_id
		WHERE s.obligation_id = 'obligation_m2_wage_day_2' AND s.claimant_kind = 'cohort'`); err != nil {
		t.Fatal(err)
	}
	if _, err := loadM2WageObligationSlices(ctx, conn, "obligation_m2_wage_day_2"); !core.HasCode(err, core.CodeProjectionDiverged) {
		t.Fatalf("slice citing another period's real accrual must fail: %v", err)
	}
}

func TestM2SplitPaymentUsesAccruedRoster(t *testing.T) {
	ctx := context.Background()
	store := openM2AgentStore(t, ctx, filepath.Join(t.TempDir(), "payment-origin.db"), false)
	defer store.Close()
	if _, err := store.PrepareM2EconomicDemo(ctx); err != nil {
		t.Fatal(err)
	}
	first, err := store.RunAgentLife(ctx, M2AgentNoonTime, 100)
	if err != nil {
		t.Fatal(err)
	}
	worker := m2AgentMaterialization("pay_origin_worker", "entity_pay_origin_worker", "Worker", 1, 0, 0, 0, 0, first.HeadSequence, m2WageTime(1, 12, 1))
	if _, err := store.MaterializeCohort(ctx, worker); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RunAgentLife(ctx, m2WageTime(2, 7, 0), 100); err != nil {
		t.Fatal(err)
	}
	// Diagnostic projection change, not a public amendment: an already earned
	// period must be paid from its original roster and rate.
	if _, err := store.db.ExecContext(ctx, `UPDATE m2_cohort_contracts SET participant_count = 17, unit_rate_minor = 11 WHERE contract_id = ?`, m2EconomyContractID); err != nil {
		t.Fatal(err)
	}
	store.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "old wage payment interrupted") }
	if _, err := store.RunAgentLife(ctx, m2WageTime(2, 7, 1), 1); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatalf("old-roster payment must reach atomic commit: %v", err)
	}
	assertM2Value(t, ctx, store, `SELECT amount_paid_minor FROM m2_economic_obligations WHERE obligation_id = 'obligation_m2_wage_day_2'`, nil, 0)
	store.beforeCommit = nil
	paid, err := store.RunAgentLife(ctx, m2WageTime(2, 7, 1), 1)
	if err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, store, `SELECT amount_paid_minor FROM m2_economic_obligations WHERE obligation_id = 'obligation_m2_wage_day_2'`, nil, 180)
	assertM2Value(t, ctx, store, `SELECT amount_minor FROM m2_wage_split_receipts WHERE obligation_id = 'obligation_m2_wage_day_2' AND claimant_id = ?`, []any{worker.EntityID}, 10)
	if repeated, err := store.RunAgentLife(ctx, m2WageTime(2, 7, 1), 1); err != nil || repeated.ProcessedItems != 0 || repeated.HeadSequence != paid.HeadSequence {
		t.Fatalf("old-roster payment duplicated: %+v %v", repeated, err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE m2_cohort_contracts SET participant_count = 18, unit_rate_minor = 10 WHERE contract_id = ?`, m2EconomyContractID); err != nil {
		t.Fatal(err)
	}
	if differences, err := store.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatalf("old-roster payment changed projections: %v %v", differences, err)
	}
}

func TestM2UnsplitPaymentUsesAccruedTerms(t *testing.T) {
	ctx := context.Background()
	store := openM2AgentStore(t, ctx, filepath.Join(t.TempDir(), "unsplit-origin.db"), false)
	defer store.Close()
	if _, err := store.PrepareM2EconomicDemo(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RunAgentLife(ctx, m2WageTime(1, 7, 0), 100); err != nil {
		t.Fatal(err)
	}
	// Diagnostic only: future terms cannot rewrite an already accrued period.
	if _, err := store.db.ExecContext(ctx, `UPDATE m2_cohort_contracts SET participant_count = 17, unit_rate_minor = 11 WHERE contract_id = ?`, m2EconomyContractID); err != nil {
		t.Fatal(err)
	}
	store.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "unsplit payment interrupted") }
	if _, err := store.RunAgentLife(ctx, m2WageTime(1, 7, 1), 1); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatalf("historical payment must reach atomic commit: %v", err)
	}
	assertM2Value(t, ctx, store, `SELECT amount_paid_minor FROM m2_economic_obligations WHERE obligation_id = 'obligation_m2_wage_day_1'`, nil, 0)
	store.beforeCommit = nil
	paid, err := store.RunAgentLife(ctx, m2WageTime(1, 7, 1), 1)
	if err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, store, `SELECT amount_paid_minor FROM m2_economic_obligations WHERE obligation_id = 'obligation_m2_wage_day_1'`, nil, 180)
	if repeated, err := store.RunAgentLife(ctx, m2WageTime(1, 7, 1), 1); err != nil || repeated.ProcessedItems != 0 || repeated.HeadSequence != paid.HeadSequence {
		t.Fatalf("unsplit payment duplicated: %+v %v", repeated, err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE m2_cohort_contracts SET participant_count = 18, unit_rate_minor = 10 WHERE contract_id = ?`, m2EconomyContractID); err != nil {
		t.Fatal(err)
	}
	if differences, err := store.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatalf("unsplit original-term payment projection: %v %v", differences, err)
	}
}
