package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"corerp.local/backend/internal/core"
)

func prepareCareerEmploymentOffer(t *testing.T, s *Store) CareerRecord {
	return prepareCareerEmploymentOfferAtWage(t, s, 12)
}

func prepareCareerEmploymentOfferAtWage(t *testing.T, s *Store, wage int64, capabilities ...string) CareerRecord {
	t.Helper()
	ctx := context.Background()
	app := prepareCareerApplicantAtWage(t, s, wage, capabilities...)
	if _, err := s.InviteCareerInterview(ctx, core.CareerInterviewRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "invite"), InterviewID: "interview_ada", ApplicationID: app.Fact.RecordID, Question: "Explain safe opening checks."}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AnswerCareerInterview(ctx, core.CareerInterviewAnswerRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "answer"), InterviewID: "interview_ada", Answer: "Inspect equipment and exits before opening."}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.EvaluateCareerApplication(ctx, careerTestEvaluation(t, s, "evaluation", true)); err != nil {
		t.Fatal(err)
	}
	offer, err := s.OfferCareerEmployment(ctx, careerTestOffer(t, s, "offer", "evaluation"))
	if err != nil {
		t.Fatal(err)
	}
	return offer
}

func TestCareerAcceptanceRunsRealWorkAndPrivatePayroll(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "employment.db")
	s := openCareerTestWorld(t, path)
	defer func() { s.Close() }()
	offer := prepareCareerEmploymentOffer(t, s)
	r := core.CareerOfferAcceptRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "accept"), OfferID: offer.Fact.RecordID, AfterWorkPlaceID: M2AgentCafeID}
	accepted, err := s.AcceptCareerOffer(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	if accepted.Fact.Employment == nil || accepted.Fact.Offer.Status != "accepted" || accepted.Fact.Offer.ContractID != accepted.Fact.Employment.ContractID {
		t.Fatalf("acceptance did not create actual employment: %+v", accepted)
	}
	job := accepted.Fact.Employment
	market, err := s.DiscoverCareerPositions(ctx, M2AgentAdaPrincipal, M2DemoInstanceID, M2DemoBranchID, M2AgentAdaID, 0)
	if err != nil || len(market.Postings) != 1 || market.Postings[0].AvailableSlots != 0 {
		t.Fatalf("filled vacancy still advertised as available: %+v %v", market, err)
	}
	if application, err := s.ReadCareerApplication(ctx, M2AgentAdaPrincipal, M2DemoInstanceID, M2DemoBranchID, "application_ada"); err != nil || application.Fact.Application.Status != "accepted" || application.EventID != accepted.EventID {
		t.Fatalf("acceptance did not close the application: %+v %v", application, err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM employment_contracts WHERE contract_id=? AND employee_entity_id=? AND status='active' AND starts_on_day=1 AND gross_wage_minor=12`, []any{job.ContractID, M2AgentAdaID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM accounts WHERE opened_by_event_id=?`, []any{accepted.EventID}, 4)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_schedule_entries WHERE agent_id=? AND world_time=? AND status='active'`, []any{M2AgentAdaID, M2AgentMorningTime}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM scheduler_items WHERE phase_id=? AND status='pending'`, []any{careerPayrollPhase}, 2)
	assertM2Value(t, ctx, s, `SELECT balance_minor FROM account_balances WHERE account_id=?`, []any{m2EconomyEmployerCash}, 1200)
	input := readCareerTestContext(t, s, M2AgentAdaID)
	if len(input.Life.Employment) != 1 || input.Life.Employment[0].Status != "onboarding" || input.Life.Employment[0].SourceEventID != accepted.EventID {
		t.Fatalf("onboarding context: %+v", input.Life.Employment)
	}
	encoded, _ := json.Marshal(input)
	if strings.Contains(string(encoded), "An advisory system recommends") || strings.Contains(string(encoded), "Manager's assessment") {
		t.Fatal("private evaluation leaked into candidate decision")
	}
	if differences, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatalf("zero-opening employment ledger replay: %+v %v", differences, err)
	}
	if _, err := s.RunAgentLife(ctx, M2AgentMorningTime, 100); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_positions WHERE agent_id=? AND place_id=? AND activity_code='work'`, []any{M2AgentAdaID, job.WorkplaceID}, 1)
	input = readCareerTestContext(t, s, M2AgentAdaID)
	if input.Life.Employment[0].Status != "probation" || input.Life.Employment[0].PositionID != job.PositionID || input.Life.Employment[0].OccupationID != "operations" || input.Life.Employment[0].Grade != "junior" {
		t.Fatalf("probation/occupation/position/grade: %+v", input.Life.Employment)
	}
	if _, err := s.RunAgentLife(ctx, careerTime(2, 0, 0), 100); err != nil {
		t.Fatal(err)
	}
	input = readCareerTestContext(t, s, M2AgentAdaID)
	if input.Life.ReceivableMinor != 112 || input.OwnAssetMinor != 500 {
		t.Fatalf("earned but unpaid wage context: %+v", input)
	}
	if _, err := s.RunAgentLife(ctx, careerTime(2, 0, 1), 100); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT balance_minor FROM account_balances b JOIN materialized_entities n ON n.asset_account_id=b.account_id WHERE n.entity_id=?`, []any{M2AgentAdaID}, 512)
	assertM2Value(t, ctx, s, `SELECT balance_minor FROM account_balances WHERE account_id=?`, []any{m2EconomyEmployerCash}, 1008)
	assertM2Value(t, ctx, s, `SELECT amount_paid_minor FROM wage_obligations WHERE contract_id=?`, []any{job.ContractID}, 12)
	attendance, err := s.ReadCareerAttendance(ctx, M2AgentAdaPrincipal, M2DemoInstanceID, M2DemoBranchID, job.ContractID, 1)
	if err != nil || attendance.Attendance.Status != "complete" || attendance.Attendance.RecordedSeconds != 14400 || len(attendance.Attendance.WorkEventIDs) != 1 {
		t.Fatalf("actual complete work shift: %+v %v", attendance, err)
	}
	input = readCareerTestContext(t, s, M2AgentAdaID)
	if input.Life.ReceivableMinor != 100 || input.OwnAssetMinor != 512 {
		t.Fatal("paid wage did not clear own receivable")
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM outbox o JOIN events e ON e.event_id=o.event_id JOIN event_batches b ON b.batch_id=e.batch_id JOIN commands c ON c.command_id=b.command_id WHERE c.command_id LIKE 'cmd_career_wage_%'`, nil, 0)
	if differences, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatalf("career payroll replay: %+v %v", differences, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if retry, err := s.AcceptCareerOffer(ctx, r); err != nil || !retry.Replayed || retry.EventID != accepted.EventID {
		t.Fatalf("acceptance recovery: %+v %v", retry, err)
	}
	if _, err := s.RunAgentLife(ctx, careerTime(3, 0, 1), 100); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT SUM(amount_paid_minor) FROM wage_obligations WHERE contract_id=?`, []any{job.ContractID}, 24)
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT balance_minor FROM account_balances b JOIN materialized_entities n ON n.asset_account_id=b.account_id WHERE n.entity_id=?`, []any{M2AgentAdaID}, 524)
}

func TestCareerPayrollSeparatesEarnedAndNextWorkTerms(t *testing.T) {
	ctx := context.Background()
	s := openCareerTestWorld(t, filepath.Join(t.TempDir(), "effective-terms.db"))
	defer s.Close()
	offer := prepareCareerEmploymentOffer(t, s)
	accepted, err := s.AcceptCareerOffer(ctx, core.CareerOfferAcceptRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "accept"), OfferID: offer.Fact.RecordID, AfterWorkPlaceID: M2AgentCafeID})
	if err != nil {
		t.Fatal(err)
	}
	job := *accepted.Fact.Employment
	job.EffectiveFromDay, job.TermVersion = 2, 2
	job.DailyWageMinor = 20
	job.WorkEndHour = 11
	// Fixture term Event exercises the existing effective-term reader/scheduler.
	// This is not a product raise/transfer command or its authorization test.
	b := careerTestBinding(t, s, M2AgentBoPrincipal, "test-future-terms")
	changed, err := s.executeCareerCommand(ctx, b, "TestCareerEffectiveTerms", job, func(conn *sql.Conn) error {
		return authorizeCareerManager(ctx, conn, b, job.OrganizationID)
	}, func(_ *sql.Conn, _ careerCommandContext) (CareerFact, func() error, error) {
		return CareerFact{Kind: "employment", RecordID: job.ContractID, OrganizationID: job.OrganizationID, CandidateID: job.EmployeeID, Employment: &job}, func() error { return nil }, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.RunAgentLife(ctx, careerTime(2, 0, 1), 100); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT amount_due_minor FROM wage_obligations WHERE contract_id=? AND period_start_day=1`, []any{job.ContractID}, 12)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_schedule_entries WHERE agent_id=? AND world_time=? AND place_id=? AND status='active'`, []any{job.EmployeeID, careerTime(2, 11, 0), job.AfterWorkPlaceID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='WageObligationAccrued' AND json_extract(payload,'$.contract_id')=? AND json_extract(payload,'$.terms_event_id')=? AND json_extract(payload,'$.next_terms_event_id')=?`, []any{job.ContractID, accepted.EventID, changed.EventID}, 1)
	input := readCareerTestContext(t, s, job.EmployeeID)
	if len(input.Life.Employment) != 1 || input.Life.Employment[0].WageMinor != 20 || input.Life.Employment[0].SourceEventID != changed.EventID {
		t.Fatalf("new day's own employment terms: %+v", input.Life.Employment)
	}
	if _, err := s.RunAgentLife(ctx, careerTime(3, 0, 1), 100); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT amount_due_minor FROM wage_obligations WHERE contract_id=? AND period_start_day=2`, []any{job.ContractID}, 20)
	assertM2Value(t, ctx, s, `SELECT amount_due_minor FROM wage_obligations WHERE contract_id=? AND period_start_day=1`, []any{job.ContractID}, 12)
	if differences, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatalf("effective term payroll replay: %+v %v", differences, err)
	}
}

func TestCareerPayrollArrearsAndInterruptedAccrual(t *testing.T) {
	ctx := context.Background()
	s := openCareerTestWorld(t, filepath.Join(t.TempDir(), "career-arrears.db"))
	defer s.Close()
	offer := prepareCareerEmploymentOfferAtWage(t, s, 2000)
	accepted, err := s.AcceptCareerOffer(ctx, core.CareerOfferAcceptRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "accept"), OfferID: offer.Fact.RecordID, AfterWorkPlaceID: M2AgentCafeID})
	if err != nil {
		t.Fatal(err)
	}
	job := accepted.Fact.Employment
	if _, err := s.RunAgentLife(ctx, M2AgentNoonTime, 100); err != nil {
		t.Fatal(err)
	}
	s.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "interrupt accrual") }
	if _, err := s.RunAgentLife(ctx, careerTime(2, 0, 1), 100); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatalf("payroll interruption: %v", err)
	}
	s.beforeCommit = nil
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM wage_obligations WHERE contract_id=?`, []any{job.ContractID}, 0)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM scheduler_items WHERE phase_id=? AND status='pending'`, []any{careerPayrollPhase}, 2)
	if _, err := s.RunAgentLife(ctx, careerTime(2, 0, 1), 100); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT amount_paid_minor FROM wage_obligations WHERE contract_id=?`, []any{job.ContractID}, 1020)
	assertM2Value(t, ctx, s, `SELECT amount_due_minor-amount_paid_minor FROM wage_obligations WHERE contract_id=?`, []any{job.ContractID}, 980)
	input := readCareerTestContext(t, s, M2AgentAdaID)
	if input.OwnAssetMinor != 1520 || input.Life.ReceivableMinor != 1080 {
		t.Fatalf("partial wage became fake cash or lost debt: %+v", input)
	}
	if _, err := s.RunAgentLife(ctx, careerTime(3, 0, 2), 100); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT SUM(amount_due_minor-amount_paid_minor) FROM wage_obligations WHERE contract_id=?`, []any{job.ContractID}, 2980)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM obligation_settlements s JOIN wage_obligations o ON o.obligation_id=s.obligation_id WHERE o.contract_id=? AND s.status='failed'`, []any{job.ContractID}, 2)
	if differences, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatalf("arrears replay: %+v %v", differences, err)
	}
}

func TestCareerAcceptanceRejectsSupersededAssessment(t *testing.T) {
	ctx := context.Background()
	s := openCareerTestWorld(t, filepath.Join(t.TempDir(), "career-superseded.db"))
	defer s.Close()
	offer := prepareCareerEmploymentOffer(t, s)
	evaluation := careerTestEvaluation(t, s, "later-reject", true)
	evaluation.Decision = "reject"
	if _, err := s.EvaluateCareerApplication(ctx, evaluation); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AcceptCareerOffer(ctx, core.CareerOfferAcceptRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "accept"), OfferID: offer.Fact.RecordID, AfterWorkPlaceID: M2AgentCafeID}); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatalf("acceptance ignored superseding assessment: %v", err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM employment_contracts WHERE employee_entity_id=?`, []any{M2AgentAdaID}, 0)
}

func readCareerTestContext(t *testing.T, s *Store, entityID string) core.RPDecisionInput {
	t.Helper()
	ctx := context.Background()
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	input, err := readRPOwnDecisionContext(ctx, tx.conn, core.RPDecisionInput{NPCEntityID: entityID, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID})
	if err != nil {
		t.Fatal(err)
	}
	return input
}

func offerExistingCareerPositionTo(t *testing.T, s *Store, candidate, principal string, starts ...int) CareerRecord {
	t.Helper()
	ctx := context.Background()
	appID, interviewID, evalID, offerID := "application_"+candidate, "interview_"+candidate, "eval_"+candidate, "offer_"+candidate
	if _, err := s.ApplyForCareerPosition(ctx, core.CareerApplicationRequest{Binding: careerTestBinding(t, s, principal, appID), ApplicationID: appID, PositionID: "position_coop_assistant", CandidateID: candidate, Statement: "I choose to apply."}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.InviteCareerInterview(ctx, core.CareerInterviewRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, interviewID), InterviewID: interviewID, ApplicationID: appID, Question: "Explain safe work."}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AnswerCareerInterview(ctx, core.CareerInterviewAnswerRequest{Binding: careerTestBinding(t, s, principal, "answer_"+candidate), InterviewID: interviewID, Answer: "Inspect equipment and exits."}); err != nil {
		t.Fatal(err)
	}
	evaluation := careerTestEvaluation(t, s, evalID, true)
	evaluation.InterviewID = interviewID
	if _, err := s.EvaluateCareerApplication(ctx, evaluation); err != nil {
		t.Fatal(err)
	}
	request := careerTestOffer(t, s, offerID, evalID)
	if len(starts) > 0 {
		request.StartsOnDay = starts[0]
		request.ExpiresAt = careerTime(starts[0], 0, 0)
	}
	offer, err := s.OfferCareerEmployment(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	return offer
}

func TestCareerAcceptanceCapacityAndAggregateWageGuard(t *testing.T) {
	ctx := context.Background()
	s := openCareerTestWorld(t, filepath.Join(t.TempDir(), "capacity.db"))
	defer s.Close()
	ada := prepareCareerEmploymentOffer(t, s)
	bo := offerExistingCareerPositionTo(t, s, M2AgentBoID, M2AgentBoPrincipal)
	lin := offerExistingCareerPositionTo(t, s, M2RPPlayerID, M2RPPlayerPrincipal)
	if _, err := s.AcceptCareerOffer(ctx, core.CareerOfferAcceptRequest{Binding: careerTestBinding(t, s, M2RPPlayerPrincipal, "accept-lin"), OfferID: lin.Fact.RecordID, AfterWorkPlaceID: M2AgentCafeID}); !core.HasCode(err, core.CodeBranchConflict) || !strings.Contains(err.Error(), "aggregate wage") {
		t.Fatalf("aggregate participation became a second salary: %v", err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM m2_wage_participation_splits`, nil, 2)
	if _, err := s.AcceptCareerOffer(ctx, core.CareerOfferAcceptRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "accept-ada"), OfferID: ada.Fact.RecordID, AfterWorkPlaceID: M2AgentCafeID}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AcceptCareerOffer(ctx, core.CareerOfferAcceptRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "accept-bo"), OfferID: bo.Fact.RecordID, AfterWorkPlaceID: M2AgentCafeID}); !core.HasCode(err, core.CodeBranchConflict) || !strings.Contains(err.Error(), "vacancy") {
		t.Fatalf("conditional offer overfilled actual position: %v", err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM employment_contracts WHERE position_id=? AND status='active'`, []any{ada.Fact.Offer.PositionKey}, 1)
}

func TestCareerAcceptanceCreatesMissingWorkAppointments(t *testing.T) {
	ctx := context.Background()
	s := openCareerTestWorld(t, filepath.Join(t.TempDir(), "new-appointments.db"))
	defer s.Close()
	prepareCareerEmploymentOffer(t, s)
	if _, err := s.RunAgentLife(ctx, M2AgentMorningTime, 100); err != nil {
		t.Fatal(err)
	}
	r := careerTestOffer(t, s, "future-offer", "evaluation")
	r.StartsOnDay, r.ExpiresAt = 31, "2026-09-23T23:59:00Z"
	offer, err := s.OfferCareerEmployment(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := s.AcceptCareerOffer(ctx, core.CareerOfferAcceptRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "future-accept"), OfferID: offer.Fact.RecordID, AfterWorkPlaceID: M2AgentCafeID})
	if err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_schedule_entries WHERE definition_event_id=? AND status='active'`, []any{accepted.EventID}, 2)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_schedule_entries WHERE agent_id=? AND world_time=? AND place_id=? AND activity_code='work'`, []any{M2AgentAdaID, careerTime(31, 8, 0), "place_m2_work_ada"}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM wage_obligations WHERE contract_id=?`, []any{accepted.Fact.Employment.ContractID}, 0)
	leave, err := s.RequestCareerLeave(ctx, core.CareerLeaveRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "future-leave"), LeaveID: "future-leave", ContractID: accepted.Fact.Employment.ContractID, StartDay: 31, EndDay: 32, Reason: "Unavailable on the first day"})
	if err != nil {
		t.Fatal(err)
	}
	approved, err := s.ReviewCareerLeave(ctx, core.CareerLeaveReviewRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "future-leave-approval"), LeaveID: leave.Fact.RecordID, Decision: "approve", Notice: "Approved"})
	if err != nil || len(approved.Fact.Leave.CancelledSchedules) != 2 {
		t.Fatalf("generated work/after-work cancellation: %+v %v", approved, err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_schedule_entries WHERE definition_event_id=? AND status='cancelled'`, []any{accepted.EventID}, 2)
}

func TestCareerAcceptanceRollbackAndCandidateAuthority(t *testing.T) {
	ctx := context.Background()
	s := openCareerTestWorld(t, filepath.Join(t.TempDir(), "accept-rollback.db"))
	defer s.Close()
	offer := prepareCareerEmploymentOffer(t, s)
	r := core.CareerOfferAcceptRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "accept"), OfferID: offer.Fact.RecordID, AfterWorkPlaceID: M2AgentCafeID}
	if _, err := s.AcceptCareerOffer(ctx, r); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("manager accepted for candidate: %v", err)
	}
	r.Binding = careerTestBinding(t, s, M2AgentAdaPrincipal, "accept")
	s.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "rollback employment") }
	if _, err := s.AcceptCareerOffer(ctx, r); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatalf("acceptance rollback: %v", err)
	}
	s.beforeCommit = nil
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM employment_contracts WHERE employee_entity_id=?`, []any{M2AgentAdaID}, 0)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM scheduler_items WHERE phase_id=?`, []any{careerPayrollPhase}, 0)
	if got, err := s.ReadCareerRecruitmentRecord(ctx, M2AgentAdaPrincipal, M2DemoInstanceID, M2DemoBranchID, "offer", offer.Fact.RecordID); err != nil || got.Fact.Offer.Status != "offered" {
		t.Fatalf("failed acceptance altered offer: %+v %v", got, err)
	}
	if _, err := s.AcceptCareerOffer(ctx, r); err != nil {
		t.Fatal(err)
	}
	r.Binding = careerTestBinding(t, s, M2AgentAdaPrincipal, "second-accept")
	if _, err := s.AcceptCareerOffer(ctx, r); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatalf("accepted twice: %v", err)
	}
}
