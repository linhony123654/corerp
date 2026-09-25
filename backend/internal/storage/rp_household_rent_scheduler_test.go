package storage

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"corerp.local/backend/internal/core"
)

func TestRPHouseholdRentSchedulerAccruesPaysPartiallyAndRecurs(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "household-rent-scheduler.db")
	s := openM2AgentStore(t, ctx, path, false)
	defer func() { s.Close() }()
	if _, err := s.BootstrapRPPlayDemo(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO principals(principal_id,principal_type,display_name,status) VALUES ('principal_f4_schedule_operator','operator','F4 schedule operator','active')`); err != nil {
		t.Fatal(err)
	}
	binding := func(key string) core.CareerBinding {
		var head int64
		if err := s.db.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&head); err != nil {
			t.Fatal(err)
		}
		return core.CareerBinding{PrincipalID: "principal_f4_schedule_operator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: head, IdempotencyKey: key}
	}
	home, err := s.FoundRPHouseholdLocal(ctx, RPHouseholdFoundRequest{Binding: binding("schedule-home"), HouseholdKey: "schedule-home", DisplayName: "Rent home", ResidencePlaceID: M2AgentCafeID, AdultEntityIDs: [2]string{M2AgentAdaID, M2AgentBoID}})
	if err != nil {
		t.Fatal(err)
	}
	agreement, err := s.AgreeRPHouseholdRentLocal(ctx, RPHouseholdRentAgreementRequest{
		Binding: binding("schedule-rent"), HouseholdID: home.Fact.HouseholdID, AgreementKey: "daily-rent",
		LandlordName: "Schedule landlord", RentMinor: 300, PeriodDays: 1, GraceDays: 1,
		Shares: [2]RPHouseholdRentShare{{M2AgentAdaID, 180}, {M2AgentBoID, 120}},
	})
	if err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM scheduler_items WHERE phase_id=? AND status='pending'`, []any{rpHouseholdRentPhase}, 3)
	read := allowFixtureControl(t, ctx, s, M2AgentAdaID)
	observed, err := s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	paid, err := s.ContributeRPHouseholdRent(ctx, RPHouseholdRentContributionRequest{
		Binding:   core.CareerBinding{PrincipalID: read.PrincipalID, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: observed.ObservationCursor, IdempotencyKey: "schedule-ada-100"},
		SessionID: read.SessionID, AgreementID: agreement.Fact.AgreementID, AmountMinor: 100,
	})
	if err != nil || paid.Fact.AmountMinor != 100 {
		t.Fatal("fund household", paid, err)
	}
	start, err := time.Parse(time.RFC3339, agreement.WorldTime)
	if err != nil {
		t.Fatal(err)
	}
	due := start.AddDate(0, 0, 1)
	firstObligation := "rent_" + agreement.Fact.ContractID + "_0_1"
	if _, err := s.RunAgentLife(ctx, due.Add(-time.Second).Format(time.RFC3339), 1000); err != nil {
		t.Fatal("run before rent due", err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rent_obligations WHERE obligation_id=?`, []any{firstObligation}, 0)
	s.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "rent scheduled rollback") }
	if _, err := s.RunAgentLife(ctx, due.Format(time.RFC3339), 1000); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatal("scheduled rollback", err)
	}
	s.beforeCommit = nil
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rent_obligations WHERE obligation_id=?`, []any{firstObligation}, 0)
	if _, err := s.RunAgentLife(ctx, due.Format(time.RFC3339), 1000); err != nil {
		t.Fatal("settle first rent period", err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rent_obligations WHERE obligation_id=? AND amount_due_minor=300 AND amount_paid_minor=100 AND status='partially_paid' AND due_world_time=?`, []any{firstObligation, due.Format(time.RFC3339)}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM obligation_settlements WHERE obligation_kind='rent' AND obligation_id=? AND paid_minor=100 AND remaining_minor=200 AND status='partial'`, []any{firstObligation}, 1)
	assertM2Value(t, ctx, s, `SELECT balance_minor FROM account_balances WHERE account_id=?`, []any{home.Fact.RentAccountID}, 0)
	assertM2Value(t, ctx, s, `SELECT balance_minor FROM account_balances WHERE account_id=?`, []any{agreement.Fact.LandlordAccountID}, 100)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM scheduler_items WHERE phase_id=? AND status='pending'`, []any{rpHouseholdRentPhase}, 4)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM outbox o JOIN events e ON e.event_id=o.event_id WHERE e.event_type IN ('RPHouseholdRentAccrued','RPHouseholdRentPaid','RPHouseholdRentPaymentFailed')`, nil, 0)
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("first rent replay", diff, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	secondDue := due.AddDate(0, 0, 1)
	if _, err := s.RunAgentLife(ctx, secondDue.Format(time.RFC3339), 1000); err != nil {
		t.Fatal("second due and first past-due", err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rent_obligations WHERE obligation_id=? AND status='past_due' AND amount_paid_minor=100`, []any{firstObligation}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rent_obligations WHERE contract_id=? AND period_start_day=1 AND period_end_day=2 AND amount_due_minor=300`, []any{agreement.Fact.ContractID}, 1)
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("recurring rent replay", diff, err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE rent_obligations SET amount_paid_minor=1 WHERE obligation_id=?`, firstObligation); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE obligation_settlements SET remaining_minor=1 WHERE obligation_id=?`, firstObligation); err != nil {
		t.Fatal(err)
	}
	queueID := "sched_rp_household_rent_" + agreement.Fact.AgreementID + "_3_" + rpHouseholdRentAccrue
	if _, err := s.db.ExecContext(ctx, `UPDATE scheduler_items SET status='cancelled' WHERE scheduler_item_id=?`, queueID); err != nil {
		t.Fatal(err)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) < 3 {
		t.Fatal("rent runtime corruption not detected", diff, err)
	}
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal("rebuild rent runtime", err)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("rent runtime repair", diff, err)
	}
	thirdDue := secondDue.AddDate(0, 0, 1)
	if _, err := s.RunAgentLife(ctx, thirdDue.Format(time.RFC3339), 1000); err != nil {
		t.Fatal("third period after queue repair", err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rent_obligations WHERE contract_id=? AND period_start_day=2 AND period_end_day=3 AND amount_due_minor=300`, []any{agreement.Fact.ContractID}, 1)
	var noMoneyEvent string
	if err := s.db.QueryRowContext(ctx, `SELECT event_id FROM events WHERE event_type='RPHouseholdRentPaymentFailed' AND json_extract(payload,'$.period_index')=2`).Scan(&noMoneyEvent); err != nil {
		t.Fatal("find no-money rent Event", err)
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO journal_entries(entry_id,event_id,status,purpose) VALUES ('journal_rogue_rp_rent',?,'draft','unexpected rent journal')`, noMoneyEvent); err != nil {
		t.Fatal(err)
	}
	diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID)
	if err != nil {
		t.Fatal(err)
	}
	foundJournalDifference := false
	for _, row := range diff {
		foundJournalDifference = foundJournalDifference || row.Projection == "rp_household_rent_journal"
	}
	if !foundJournalDifference {
		t.Fatal("unsourced rent journal not detected", diff)
	}
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); !core.HasCode(err, core.CodeProjectionDiverged) {
		t.Fatal("unsourced rent journal must require manual audit", err)
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM journal_entries WHERE entry_id='journal_rogue_rp_rent' AND status='draft'`); err != nil {
		t.Fatal(err)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("rent journal audit after fixture cleanup", diff, err)
	}
}

func TestRPHouseholdRentSchedulePreservesLateNightWorldDay(t *testing.T) {
	start, err := time.Parse(time.RFC3339, "2026-09-25T23:59:00Z")
	if err != nil {
		t.Fatal(err)
	}
	row := rpHouseholdRentSchedule{AgreementID: "agreement_late", StartTime: start, StartDay: 5, PeriodDays: 1}
	for index, kind := range []string{rpHouseholdRentAccrue, rpHouseholdRentPay, rpHouseholdRentPastDue} {
		item, payload, err := rpHouseholdRentItem(row, 1, kind)
		if err != nil {
			t.Fatal(err)
		}
		if item.WorldTime != "2026-09-26T23:59:00Z" || payload.Day != 6 || item.DeclaredPriority != int64(index) {
			t.Fatalf("late-night schedule %s: item=%+v payload=%+v", kind, item, payload)
		}
	}
}
