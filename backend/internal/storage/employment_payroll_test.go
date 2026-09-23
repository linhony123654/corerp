package storage

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
)

// These tests exercise the accounting boundary, not yet a product resignation
// command. Contract edits below are deliberately isolated fixture state.
func TestEmploymentEarnedDebtSurvivesExitAndNewTerms(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "earned-wages.db")
	s := openBootstrappedStore(t, ctx, path)
	defer func() { s.Close() }()
	period := wageAccrualPeriod{ContractID: "employment_1", StartDay: 0, EndDay: 1, DueWorldTime: strictDayTime(1), AmountMinor: 1500}
	commitEmploymentAccountingProbe(t, ctx, s, "earned", 1, func(conn *sql.Conn) (scheduledMutation, error) {
		return prepareWageAccrualForPeriod(ctx, conn, period)
	})
	// Neither exiting nor changing today's terms can erase yesterday's debt.
	if _, err := s.db.ExecContext(ctx, `UPDATE employment_contracts SET status='ended',gross_wage_minor=9000 WHERE contract_id='employment_1'`); err != nil {
		t.Fatal(err)
	}
	commitEmploymentAccountingProbe(t, ctx, s, "pay_after_exit", 1, func(conn *sql.Conn) (scheduledMutation, error) {
		return prepareWageSettlement(ctx, conn, scheduledPayload{Day: 1}, "wage_employment_1_0_1")
	})
	assertScalar(t, ctx, s, `SELECT amount_due_minor FROM wage_obligations WHERE obligation_id='wage_employment_1_0_1'`, 1500)
	assertScalar(t, ctx, s, `SELECT amount_paid_minor FROM wage_obligations WHERE obligation_id='wage_employment_1_0_1'`, 1000)
	assertScalar(t, ctx, s, `SELECT COUNT(*) FROM wage_obligations WHERE status='arrears'`, 1)
	assertScalar(t, ctx, s, `SELECT balance_minor FROM account_balances WHERE account_id='account_wage_employment_1_receivable'`, 500)
	assertScalar(t, ctx, s, `SELECT balance_minor FROM account_balances WHERE account_id='account_wage_employment_1_payable'`, -500)
	var postings int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM postings`).Scan(&postings); err != nil {
		t.Fatal(err)
	}
	// Existing period authority wins even if a later caller supplies new terms.
	period.AmountMinor = 9000
	commitEmploymentAccountingProbe(t, ctx, s, "duplicate_earned", 1, func(conn *sql.Conn) (scheduledMutation, error) {
		return prepareWageAccrualForPeriod(ctx, conn, period)
	})
	assertScalar(t, ctx, s, `SELECT COUNT(*) FROM postings`, postings)
	assertScalar(t, ctx, s, `SELECT amount_due_minor FROM wage_obligations WHERE obligation_id='wage_employment_1_0_1'`, 1500)
	assertScalar(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='SchedulerSkipLogged'`, 1)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	var err error
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	commitEmploymentAccountingProbe(t, ctx, s, "retry_after_reopen", 2, func(conn *sql.Conn) (scheduledMutation, error) {
		return prepareWageSettlement(ctx, conn, scheduledPayload{Day: 2}, "wage_employment_1_0_1")
	})
	assertScalar(t, ctx, s, `SELECT COUNT(*) FROM postings`, postings)
	assertScalar(t, ctx, s, `SELECT remaining_minor FROM obligation_settlements WHERE settlement_id='settlement_wage_wage_employment_1_0_1_day_002'`, 500)
	assertScalar(t, ctx, s, `SELECT COUNT(*) FROM obligation_settlements WHERE status='failed' AND reason_code='insufficient_funds'`, 1)
	if differences, err := s.CompareProjections(ctx, DemoInstanceID, DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatalf("earned wage replay: differences=%+v err=%v", differences, err)
	}
	if err := s.RebuildProjections(ctx, DemoInstanceID, DemoBranchID); err != nil {
		t.Fatal(err)
	}
	assertScalar(t, ctx, s, `SELECT balance_minor FROM account_balances WHERE account_id='account_wage_employment_1_receivable'`, 500)
}

func TestEmploymentAccrualExplicitCalendarAndRollback(t *testing.T) {
	ctx := context.Background()
	s := openBootstrappedStore(t, ctx, filepath.Join(t.TempDir(), "calendar.db"))
	defer s.Close()
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	period := wageAccrualPeriod{ContractID: "employment_1", StartDay: 0, EndDay: 1, DueWorldTime: "2026-09-23T08:01:00+08:00", AmountMinor: 10}
	mutation, err := prepareWageAccrualForPeriod(ctx, tx.conn, period)
	if err != nil {
		t.Fatal(err)
	}
	if mutation.EventType != "WageObligationAccrued" || len(mutation.Postings) != 4 {
		t.Fatalf("unexpected accrual: %+v", mutation)
	}
	var sum int64
	for _, posting := range mutation.Postings {
		sum += posting.Amount
	}
	if sum != 0 {
		t.Fatal("accrual is not balanced")
	}
	if err := mutation.ApplyDomainRows(ctx, tx.conn, "unused-fixture-event", 1); err != nil {
		t.Fatal(err)
	}
	var due string
	if err := tx.conn.QueryRowContext(ctx, `SELECT due_world_time FROM wage_obligations WHERE obligation_id='wage_employment_1_0_1'`).Scan(&due); err != nil {
		t.Fatal(err)
	}
	if due != "2026-09-23T00:01:00Z" {
		t.Fatalf("explicit due time replaced by M1 epoch: %q", due)
	}
	// No partial obligation escapes a transaction abandoned before commit.
	tx.Rollback(ctx)
	assertScalar(t, ctx, s, `SELECT COUNT(*) FROM wage_obligations`, 0)
	assertScalar(t, ctx, s, `SELECT balance_minor FROM account_balances WHERE account_id='account_wage_employment_1_expense'`, 0)
}

func TestEmploymentAccrualRejectsInvalidPeriods(t *testing.T) {
	ctx := context.Background()
	s := openBootstrappedStore(t, ctx, filepath.Join(t.TempDir(), "invalid.db"))
	defer s.Close()
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	valid := wageAccrualPeriod{ContractID: "employment_1", StartDay: 0, EndDay: 1, DueWorldTime: strictDayTime(1), AmountMinor: 10}
	for _, name := range []string{"missing_contract", "negative_start", "empty_period", "reverse_period", "zero_amount", "negative_amount", "invalid_time"} {
		t.Run(name, func(t *testing.T) {
			p := valid
			switch name {
			case "missing_contract":
				p.ContractID = ""
			case "negative_start":
				p.StartDay = -1
			case "empty_period":
				p.EndDay = 0
			case "reverse_period":
				p.EndDay = -1
			case "zero_amount":
				p.AmountMinor = 0
			case "negative_amount":
				p.AmountMinor = -1
			case "invalid_time":
				p.DueWorldTime = "tomorrow"
			}
			if _, err := prepareWageAccrualForPeriod(ctx, tx.conn, p); !core.HasCode(err, core.CodeInvalidArgument) {
				t.Fatalf("invalid period accepted: %v", err)
			}
		})
	}
}

// Persist through the actual scheduled Event/journal/CAS boundary. This helper
// intentionally does not claim the career dispatcher or exit command exists.
func commitEmploymentAccountingProbe(t *testing.T, ctx context.Context, s *Store, suffix string, day int, prepare func(*sql.Conn) (scheduledMutation, error)) {
	t.Helper()
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	payload := scheduledPayload{Kind: "employment_accounting_probe", Day: day, SubjectID: "employment_1"}
	raw, err := core.CanonicalJSON(payload)
	if err != nil {
		t.Fatal(err)
	}
	item := SchedulerItem{SchedulerItemID: "career_probe_" + suffix, WorldTime: strictDayTime(day), PhaseID: "10_wage_accrual", Status: "pending", Payload: string(raw)}
	if _, err := tx.conn.ExecContext(ctx, `INSERT INTO scheduler_items(scheduler_item_id,instance_id,branch_id,world_time,phase_id,declared_priority,status,payload) VALUES (?,?,?,?,?,0,'pending',?)`, item.SchedulerItemID, DemoInstanceID, DemoBranchID, item.WorldTime, item.PhaseID, item.Payload); err != nil {
		t.Fatal(err)
	}
	mutation, err := prepare(tx.conn)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.commitScheduledMutationForBranch(ctx, tx, item, payload, mutation, DemoInstanceID, DemoBranchID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}
