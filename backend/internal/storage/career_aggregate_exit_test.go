package storage

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestCareerAggregateExitNoticeAuthorityRecoveryAndNoPrematureEffects(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "aggregate-exit.db")
	s := openCareerTestWorld(t, path)
	defer func() { s.Close() }()
	r := core.CareerAggregateExitRequest{Binding: careerTestBinding(t, s, M2RPNPCPrincipal, "aggregate-exit"), CandidateID: M2RPNPCID, ContractID: m2EconomyContractID, FinalEarnedDay: 2, Notice: "I intend to leave after the final earned period."}
	var population, cash, pending int64
	if err := s.db.QueryRow(`SELECT population_count FROM cohorts WHERE cohort_id=?`, M2DemoCohortID).Scan(&population); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRow(`SELECT balance_minor FROM account_balances WHERE account_id=?`, m2EconomyEmployerCash).Scan(&cash); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM scheduler_items WHERE status='pending'`).Scan(&pending); err != nil {
		t.Fatal(err)
	}
	for _, principal := range []string{M2AgentBoPrincipal, "principal_creator"} {
		bad := r
		bad.Binding = careerTestBinding(t, s, principal, "not-own-exit")
		if _, err := s.RequestCareerAggregateExit(ctx, bad); err == nil {
			t.Fatal("another principal requested worker exit")
		}
	}
	bad := r
	bad.Binding = careerTestBinding(t, s, M2AgentAdaPrincipal, "not-aggregate")
	bad.CandidateID = M2AgentAdaID
	if _, err := s.RequestCareerAggregateExit(ctx, bad); !core.HasCode(err, core.CodeNotFound) {
		t.Fatalf("nonparticipant: %v", err)
	}
	s.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "exit notice interrupted") }
	if _, err := s.RequestCareerAggregateExit(ctx, r); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatalf("rollback: %v", err)
	}
	assertM2Value(t, ctx, s, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, []any{M2DemoInstanceID, M2DemoBranchID}, r.Binding.ExpectedHead)
	s.beforeCommit = nil
	got, err := s.RequestCareerAggregateExit(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	f := got.Fact.AggregateExit
	if f == nil || f.Status != "notice_recorded" || f.SplitEventID == "" || f.MaterializationID == "" || f.FinalPeriodEnd != m2WageTime(2, 7, 0) || f.EarliestIndependentStartDay != 3 {
		t.Fatalf("notice source/boundary: %+v", got)
	}
	assertM2Value(t, ctx, s, `SELECT population_count FROM cohorts WHERE cohort_id=?`, []any{M2DemoCohortID}, population)
	assertM2Value(t, ctx, s, `SELECT balance_minor FROM account_balances WHERE account_id=?`, []any{m2EconomyEmployerCash}, cash)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM scheduler_items WHERE status='pending'`, nil, pending+1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM m2_wage_participation_returns WHERE materialization_id=?`, []any{f.MaterializationID}, 0)
	for _, entity := range []string{M2RPNPCID, M2AgentBoID} {
		input := readCareerTestContext(t, s, entity)
		found := false
		for _, memory := range input.Life.SalientMemories {
			found = found || memory.SourceEventID == got.EventID && memory.Kind == "own_aggregate_exit_notice"
		}
		if found != (entity == M2RPNPCID) {
			t.Fatalf("private notice memory for %s: %v", entity, found)
		}
		if input.Life.Unemployment != nil {
			t.Fatal("notice caused premature unemployment")
		}
	}
	bad = r
	bad.Notice = "changed payload"
	if _, err := s.RequestCareerAggregateExit(ctx, bad); err == nil {
		t.Fatal("idempotency mismatch accepted")
	}
	bad = r
	bad.Binding = careerTestBinding(t, s, M2RPNPCPrincipal, "another-exit")
	if _, err := s.RequestCareerAggregateExit(ctx, bad); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatalf("duplicate exit: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := s.RequestCareerAggregateExit(ctx, r)
	if err != nil || !replay.Replayed || replay.EventID != got.EventID {
		t.Fatalf("restart retry: %+v %v", replay, err)
	}
	if differences, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatalf("notice projections: %v %v", differences, err)
	}
}

func TestCareerAggregateAllWorkersExitWithoutLosingPopulation(t *testing.T) {
	ctx := context.Background()
	s := openCareerTestWorld(t, filepath.Join(t.TempDir(), "all-workers-exit.db"))
	defer s.Close()
	materialize := func(index int) string {
		t.Helper()
		id := fmt.Sprintf("entity_all_exit_%02d", index)
		b := careerTestBinding(t, s, "principal_creator", "materialize")
		var now string
		if err := s.db.QueryRow(`SELECT current_world_time FROM world_clocks WHERE instance_id=? AND branch_id=?`, b.InstanceID, b.BranchID).Scan(&now); err != nil {
			t.Fatal(err)
		}
		command := m2AgentMaterialization(fmt.Sprintf("all_exit_%02d", index), id, "Worker", 1, 0, 0, 0, 0, b.ExpectedHead, now)
		if _, err := s.MaterializeCohort(ctx, command); err != nil {
			t.Fatal(err)
		}
		b = careerTestBinding(t, s, "principal_creator", "background")
		schedule := []core.RPBackgroundSchedule{{WorldTime: careerTime(6, 12, 0), PlaceID: "place_m2_home_bo", ActivityCode: "home"}}
		if index == 0 {
			schedule = append([]core.RPBackgroundSchedule{{EmploymentContractID: m2EconomyContractID, WorldTime: careerTime(3, 8, 0), PlaceID: "place_m2_work_bo", ActivityCode: "work"}, {WorldTime: careerTime(3, 12, 0), PlaceID: M2AgentCafeID, ActivityCode: "lunch"}}, schedule...)
		}
		_, err := s.MaterializeRPBackground(ctx, core.RPBackgroundRequest{PrincipalID: b.PrincipalID, InstanceID: b.InstanceID, BranchID: b.BranchID, EntityID: id, ExpectedHead: b.ExpectedHead, IdempotencyKey: "background_" + id, AgeMin: 25, AgeMax: 35, ResidencePlaceID: "place_m2_home_bo", InitialPlaceID: M2AgentCafeID, Schedule: schedule})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	exit := func(id string, day int) {
		t.Helper()
		var principal string
		if err := s.db.QueryRow(`SELECT principal_id FROM agent_profiles WHERE agent_id=?`, id).Scan(&principal); err != nil {
			t.Fatal(err)
		}
		_, err := s.RequestCareerAggregateExit(ctx, core.CareerAggregateExitRequest{Binding: careerTestBinding(t, s, principal, "exit_"+id), CandidateID: id, ContractID: m2EconomyContractID, FinalEarnedDay: day, Notice: "Leave this wage pool."})
		if err != nil {
			t.Fatal(err)
		}
	}
	returnPerson := func(id string) {
		t.Helper()
		b := careerTestBinding(t, s, "principal_creator", "return")
		var materialization, now string
		if err := s.db.QueryRow(`SELECT materialization_id FROM materialized_entities WHERE entity_id=?`, id).Scan(&materialization); err != nil {
			t.Fatal(err)
		}
		if err := s.db.QueryRow(`SELECT current_world_time FROM world_clocks WHERE instance_id=? AND branch_id=?`, b.InstanceID, b.BranchID).Scan(&now); err != nil {
			t.Fatal(err)
		}
		_, err := s.DematerializeCohort(ctx, core.DematerializeCohortCommand{CommandID: "cmd_return_" + id, MaterializationID: materialization, InstanceID: b.InstanceID, BranchID: b.BranchID, PrincipalID: b.PrincipalID, CapabilityID: "world.cohort.materialize", IdempotencyKey: "return_" + id, ExpectedHead: b.ExpectedHead, WorldTime: now, ReasonCode: "lod_return"})
		if err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 15; i++ {
		exit(materialize(i), 2)
	}
	exit(M2RPNPCID, 2)
	exit(M2RPPlayerID, 2)
	if _, err := s.RunAgentLife(ctx, careerTime(2, 7, 2), 1000); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_schedule_entries s JOIN scheduler_items q ON q.scheduler_item_id=s.scheduler_item_id WHERE s.agent_id='entity_all_exit_00' AND s.activity_code='work' AND s.status='cancelled' AND q.status='cancelled'`, nil, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_schedule_entries WHERE agent_id='entity_all_exit_00' AND activity_code IN ('home','lunch') AND status='active'`, nil, 2)
	returnPerson(M2RPNPCID)
	last := materialize(15)
	exit(last, 3)
	returnPerson(M2RPPlayerID)
	nonworker := materialize(16)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM m2_wage_participation_splits WHERE entity_id=?`, []any{nonworker}, 0)
	if _, err := s.RunAgentLife(ctx, careerTime(3, 7, 2), 1000); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT amount_due_minor FROM m2_economic_obligations WHERE obligation_id='obligation_m2_wage_day_3'`, nil, 10)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM m2_wage_split_obligations WHERE obligation_id='obligation_m2_wage_day_3' AND claimant_kind='cohort'`, nil, 0)
	assertM2Value(t, ctx, s, `SELECT amount_minor FROM m2_wage_split_receipts WHERE obligation_id='obligation_m2_wage_day_3' AND claimant_id=?`, []any{last}, 10)
	if _, err := s.RunAgentLife(ctx, careerTime(4, 7, 2), 1000); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM m2_economic_obligations WHERE obligation_id='obligation_m2_wage_day_4'`, nil, 0)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='M2WagePeriodInactive'`, nil, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='M2WageSettlementSkipped'`, nil, 1)
	assertM2Value(t, ctx, s, `SELECT population_count FROM cohorts WHERE cohort_id=?`, []any{M2DemoCohortID}, 1)
	assertM2Value(t, ctx, s, `SELECT SUM(population_count) FROM materialized_entities WHERE status='active'`, nil, 19)
	if _, err := s.RunAgentLife(ctx, careerTime(6, 12, 0), 1000); err != nil {
		t.Fatal(err)
	}
	returnPerson(nonworker)
	assertM2Value(t, ctx, s, `SELECT population_count FROM cohorts WHERE cohort_id=?`, []any{M2DemoCohortID}, 2)
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	if differences, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatalf("all-ended projections: %v %v", differences, err)
	}
	if _, err := s.RunAgentLife(ctx, careerTime(30, 7, 12), 10000); err != nil {
		t.Fatal(err)
	}
	var donor, employer int64
	if err := s.db.QueryRow(`SELECT balance_minor FROM account_balances WHERE account_id=?`, m2EconomyLandlordCash).Scan(&donor); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRow(`SELECT balance_minor FROM account_balances WHERE account_id=?`, m2EconomyEmployerCash).Scan(&employer); err != nil {
		t.Fatal(err)
	}
	s.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "zero estate task interrupted") }
	if _, err := s.RunAgentLife(ctx, careerTime(31, 7, 0), 1); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatalf("zero estate rollback: %v", err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM m2_estate_contributions`, nil, 0)
	s.beforeCommit = nil
	if _, err := s.RunAgentLife(ctx, careerTime(31, 7, 1), 100); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT balance_minor FROM account_balances WHERE account_id=?`, []any{m2EconomyLandlordCash}, donor)
	assertM2Value(t, ctx, s, `SELECT balance_minor FROM account_balances WHERE account_id=?`, []any{m2EconomyEmployerCash}, employer)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM m2_estate_contributions WHERE status='deferred' AND reason_code='no_bankruptcy_proceeding' AND amount_minor=0`, nil, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM m2_estate_distributions WHERE status='deferred' AND reason_code='no_bankruptcy_proceeding' AND amount_minor=0`, nil, 1)
	if differences, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatalf("no-estate projections: %v %v", differences, err)
	}
}

func TestCareerAggregateExitFinalWageAndIndependentEmployment(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "aggregate-handoff.db")
	s := openCareerTestWorld(t, path)
	defer func() { s.Close() }()
	prepareCareerEmploymentOffer(t, s)
	offer := offerExistingCareerPositionTo(t, s, M2RPNPCID, M2RPNPCPrincipal, 3)
	notice, err := s.RequestCareerAggregateExit(ctx, core.CareerAggregateExitRequest{Binding: careerTestBinding(t, s, M2RPNPCPrincipal, "exit"), CandidateID: M2RPNPCID, ContractID: m2EconomyContractID, FinalEarnedDay: 2, Notice: "Leave the original wage pool."})
	if err != nil {
		t.Fatal(err)
	}
	accept := core.CareerOfferAcceptRequest{Binding: careerTestBinding(t, s, M2RPNPCPrincipal, "accept-new-job"), OfferID: offer.Fact.RecordID, AfterWorkPlaceID: M2AgentCafeID}
	if _, err := s.AcceptCareerOffer(ctx, accept); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatalf("notice alone permitted overlap: %v", err)
	}
	if _, err := s.RunAgentLife(ctx, careerTime(2, 7, 1), 100); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT due_minor FROM m2_wage_split_obligations WHERE obligation_id='obligation_m2_wage_day_2' AND claimant_id=?`, []any{M2RPNPCID}, 10)
	s.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "activation interrupted") }
	if _, err := s.RunAgentLife(ctx, careerTime(2, 7, 2), 1); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatalf("activation rollback: %v", err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='CareerAggregateExitActivated'`, nil, 0)
	s.beforeCommit = nil
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.RunAgentLife(ctx, careerTime(2, 7, 2), 100); err != nil {
		t.Fatal(err)
	}
	own := readCareerTestContext(t, s, M2RPNPCID)
	if len(own.Life.Employment) != 0 || own.Life.Unemployment == nil || own.Life.Unemployment.Kind != "resignation" {
		t.Fatalf("effective aggregate exit life: %+v", own.Life)
	}
	accept.Binding = careerTestBinding(t, s, M2RPNPCPrincipal, "accept-new-job")
	accepted, err := s.AcceptCareerOffer(ctx, accept)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.RunAgentLife(ctx, careerTime(4, 0, 1), 100); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT amount_due_minor FROM m2_economic_obligations WHERE obligation_id='obligation_m2_wage_day_3'`, nil, 170)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM m2_wage_split_obligations WHERE obligation_id='obligation_m2_wage_day_3' AND claimant_id=?`, []any{M2RPNPCID}, 0)
	assertM2Value(t, ctx, s, `SELECT amount_paid_minor FROM wage_obligations WHERE contract_id=? AND period_start_day=3`, []any{accepted.Fact.Employment.ContractID}, 12)
	assertM2Value(t, ctx, s, `SELECT population_count FROM materialized_entities WHERE entity_id=? AND status='active'`, []any{M2RPNPCID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM m2_wage_participation_returns WHERE materialization_id=?`, []any{notice.Fact.AggregateExit.MaterializationID}, 0)
	if replay, err := s.RequestCareerAggregateExit(ctx, core.CareerAggregateExitRequest{Binding: core.CareerBinding{PrincipalID: M2RPNPCPrincipal, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, IdempotencyKey: "exit", ExpectedHead: notice.EventSequence - 1}, CandidateID: M2RPNPCID, ContractID: m2EconomyContractID, FinalEarnedDay: 2, Notice: "Leave the original wage pool."}); err != nil || !replay.Replayed {
		t.Fatalf("historical notice replay: %+v %v", replay, err)
	}
	if differences, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatalf("handoff projections: %v %v", differences, err)
	}
	if run, err := s.RunAgentLife(ctx, careerTime(31, 7, 1), 10000); err != nil || run.PendingDue != 0 {
		t.Fatalf("reduced-workforce long run: %+v %v", run, err)
	}
	assertM2Value(t, ctx, s, `SELECT amount_due_minor FROM m2_economic_obligations WHERE obligation_id='obligation_m2_wage_day_30'`, nil, 170)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM m2_wage_split_obligations WHERE claimant_id=?`, []any{M2RPNPCID}, 2)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM m2_estate_distributions WHERE status='deferred' AND reason_code='slot_claim_liquidation_deferred'`, nil, 1)
	assertM2Value(t, ctx, s, `SELECT balance_minor FROM account_balances WHERE account_id=?`, []any{m2EconomyEmployerCash}, 18)
}

func TestCareerAggregateNamedOnlyAllocation(t *testing.T) {
	slices := []m2WageSlice{{kind: "entity", claimant: "a", due: 10}, {kind: "entity", claimant: "b", due: 10}}
	for paid := int64(0); paid <= 20; paid++ {
		shares, err := allocateM2WageCumulative(m2WageAllocationPolicyVersion, slices, 10, 20, paid)
		if err != nil || shares[0]+shares[1] != paid || shares[0] > 10 || shares[1] > 10 {
			t.Fatalf("named-only allocation %d: %v %v", paid, shares, err)
		}
	}
	slices[0], slices[1] = slices[1], slices[0]
	if _, err := allocateM2WageCumulative(m2WageAllocationPolicyVersion, slices, 10, 20, 1); !core.HasCode(err, core.CodeProjectionDiverged) {
		t.Fatal("unsorted named-only slots accepted")
	}
}

func TestCareerAggregateExitDemographicReturnDoesNotRestoreWage(t *testing.T) {
	ctx := context.Background()
	s := openCareerTestWorld(t, filepath.Join(t.TempDir(), "aggregate-return.db"))
	defer s.Close()
	notice, err := s.RequestCareerAggregateExit(ctx, core.CareerAggregateExitRequest{Binding: careerTestBinding(t, s, M2RPNPCPrincipal, "exit-return"), CandidateID: M2RPNPCID, ContractID: m2EconomyContractID, FinalEarnedDay: 2, Notice: "Leave before returning to the cohort."})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.RunAgentLife(ctx, careerTime(2, 7, 2), 100); err != nil {
		t.Fatal(err)
	}
	b := careerTestBinding(t, s, "principal_creator", "dematerialize")
	returned, err := s.DematerializeCohort(ctx, core.DematerializeCohortCommand{CommandID: "cmd_exit_return", MaterializationID: notice.Fact.AggregateExit.MaterializationID, InstanceID: b.InstanceID, BranchID: b.BranchID, PrincipalID: b.PrincipalID, CapabilityID: "world.cohort.materialize", IdempotencyKey: "exit_return", ExpectedHead: b.ExpectedHead, WorldTime: careerTime(2, 7, 2), ReasonCode: "lod_return"})
	if err != nil {
		t.Fatal(err)
	}
	newWorker := m2AgentMaterialization("after_exit_return", "entity_after_exit_return", "Another worker", 1, 0, 0, 0, 0, returned.FirstSequence, careerTime(2, 7, 2))
	if _, err := s.MaterializeCohort(ctx, newWorker); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RunAgentLife(ctx, careerTime(3, 7, 1), 100); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT amount_due_minor FROM m2_economic_obligations WHERE obligation_id='obligation_m2_wage_day_3'`, nil, 170)
	assertM2Value(t, ctx, s, `SELECT due_minor FROM m2_wage_split_obligations WHERE obligation_id='obligation_m2_wage_day_3' AND claimant_kind='cohort'`, nil, 150)
	assertM2Value(t, ctx, s, `SELECT due_minor FROM m2_wage_split_obligations WHERE obligation_id='obligation_m2_wage_day_3' AND claimant_id=?`, []any{newWorker.EntityID}, 10)
	assertM2Value(t, ctx, s, `SELECT population_count FROM cohorts WHERE cohort_id=?`, []any{M2DemoCohortID}, 16)
	if differences, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatalf("demographic return projections: %v %v", differences, err)
	}
}

func TestCareerAggregateExitUnpaidFinalClaimSurvivesRecoveryAndLatePayment(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "aggregate-unpaid-exit.db")
	s := openCareerTestWorld(t, path)
	defer func() { s.Close() }()
	request := core.CareerAggregateExitRequest{Binding: careerTestBinding(t, s, M2RPNPCPrincipal, "unpaid-exit"), CandidateID: M2RPNPCID, ContractID: m2EconomyContractID, FinalEarnedDay: 7, Notice: "Leaving does not waive earned wages."}
	if _, err := s.RequestCareerAggregateExit(ctx, request); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RunAgentLife(ctx, careerTime(7, 7, 2), 1000); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT amount_paid_minor FROM m2_economic_obligations WHERE obligation_id='obligation_m2_wage_day_7'`, nil, 120)
	assertM2Value(t, ctx, s, `SELECT SUM(amount_minor) FROM m2_wage_split_receipts WHERE obligation_id='obligation_m2_wage_day_7' AND claimant_id=?`, []any{M2RPNPCID}, 6)
	before := readCareerTestContext(t, s, M2RPNPCID)
	if before.Life.Unemployment == nil || before.Life.ReceivableMinor != 4 {
		t.Fatalf("final unpaid claim or unemployment missing: %+v", before.Life)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	var err error
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	after := readCareerTestContext(t, s, M2RPNPCID)
	if after.Life.Unemployment == nil || after.Life.Unemployment.SourceEventID != before.Life.Unemployment.SourceEventID || after.Life.ReceivableMinor != 4 {
		t.Fatalf("rebuild changed sourced exit/debt: %+v", after.Life)
	}
	if _, err := s.RunAgentLife(ctx, careerTime(15, 7, 8), 1000); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT balance_minor FROM account_balances WHERE account_id=?`, []any{m2EconomyEmployerCash}, 60)
	s.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "former worker late payment interrupted") }
	if _, err := s.RunAgentLife(ctx, careerTime(15, 7, 9), 1); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatalf("late payment rollback: %v", err)
	}
	assertM2Value(t, ctx, s, `SELECT SUM(amount_minor) FROM m2_wage_split_receipts WHERE obligation_id='obligation_m2_wage_day_7' AND claimant_id=?`, []any{M2RPNPCID}, 6)
	s.beforeCommit = nil
	if _, err := s.RunAgentLife(ctx, careerTime(15, 7, 9), 100); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT SUM(amount_minor) FROM m2_wage_split_receipts WHERE obligation_id='obligation_m2_wage_day_7' AND claimant_id=?`, []any{M2RPNPCID}, 10)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM m2_arrears_cases WHERE obligation_id='obligation_m2_wage_day_7' AND status='cured'`, nil, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM m2_wage_split_obligations WHERE claimant_id=?`, []any{M2RPNPCID}, 7)
	after = readCareerTestContext(t, s, M2RPNPCID)
	if after.Life.ReceivableMinor != 0 || after.Life.Unemployment == nil {
		t.Fatalf("late payment changed employment or left debt: %+v", after.Life)
	}
	if replay, err := s.RequestCareerAggregateExit(ctx, request); err != nil || !replay.Replayed {
		t.Fatalf("old notice retry after late payment: %+v %v", replay, err)
	}
	if differences, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatalf("late payment projections: %v %v", differences, err)
	}
}
