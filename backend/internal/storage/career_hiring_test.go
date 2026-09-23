package storage

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"corerp.local/backend/internal/core"
)

func prepareCareerApplicant(t *testing.T, s *Store) CareerRecord {
	return prepareCareerApplicantAtWage(t, s, 12)
}

func prepareCareerApplicantAtWage(t *testing.T, s *Store, wage int64, capabilities ...string) CareerRecord {
	t.Helper()
	ctx := context.Background()
	if _, err := s.DefineCareerOrganization(ctx, careerTestOrg(t, s)); err != nil {
		t.Fatal(err)
	}
	posting := careerTestPosting(t, s)
	posting.Posting.DailyWageMinor = wage
	posting.Posting.Capabilities = append([]string(nil), capabilities...)
	if _, err := s.PostCareerPosition(ctx, posting); err != nil {
		t.Fatal(err)
	}
	application, err := s.ApplyForCareerPosition(ctx, core.CareerApplicationRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "ada-apply"), ApplicationID: "application_ada", PositionID: posting.Posting.PositionID, CandidateID: M2AgentAdaID, Statement: "I claim I already meet all qualifications."})
	if err != nil {
		t.Fatal(err)
	}
	return application
}

func careerTestEvaluation(t *testing.T, s *Store, id string, passed bool) core.CareerEvaluationRequest {
	t.Helper()
	return core.CareerEvaluationRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, id), EvaluationID: id, InterviewID: "interview_ada", Decision: "advance", Reason: "Manager's assessment of the recorded answer", Assessments: []core.CareerQualificationAssessment{{Code: "safety_training", Passed: passed, Reason: "Assessment of the safety procedure explanation"}}, AdvisoryNote: "An advisory system recommends hiring; this note has no authority."}
}

func careerTestOffer(t *testing.T, s *Store, id, evaluation string) core.CareerOfferRequest {
	t.Helper()
	return core.CareerOfferRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, id), OfferID: id, EvaluationID: evaluation, StartsOnDay: 1, ProbationDays: 7, ExpiresAt: "2026-09-22T23:59:00Z"}
}

func TestCareerInterviewQualificationsOfferDeclineRecovery(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "hiring.db")
	s := openCareerTestWorld(t, path)
	defer func() { s.Close() }()
	application := prepareCareerApplicant(t, s)
	inviteRequest := core.CareerInterviewRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "invite"), InterviewID: "interview_ada", ApplicationID: application.Fact.RecordID, Question: "Describe the safety checks before opening the workshop."}
	invited, err := s.InviteCareerInterview(ctx, inviteRequest)
	if err != nil {
		t.Fatal(err)
	}
	// An application claim plus manager/AI recommendation is not an answer.
	if _, err := s.EvaluateCareerApplication(ctx, careerTestEvaluation(t, s, "premature", true)); !core.HasCode(err, core.CodeInvalidArgument) {
		t.Fatalf("evaluated without candidate response: %v", err)
	}
	answerRequest := core.CareerInterviewAnswerRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "answer-for-candidate"), InterviewID: inviteRequest.InterviewID, Answer: "I will answer on Ada's behalf."}
	if _, err := s.AnswerCareerInterview(ctx, answerRequest); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("manager forged candidate answer: %v", err)
	}
	answerRequest.Binding = careerTestBinding(t, s, M2AgentAdaPrincipal, "answer")
	answerRequest.Answer = "Check exits and equipment, report hazards, and do not open an unsafe station."
	answered, err := s.AnswerCareerInterview(ctx, answerRequest)
	if err != nil {
		t.Fatal(err)
	}
	if answered.Fact.Interview.InvitationEventID != invited.EventID || answered.Fact.Interview.ResponseEventID != answered.EventID {
		t.Fatal("interview lost question/answer lineage")
	}
	failedRequest := careerTestEvaluation(t, s, "eval_failed", false)
	failed, err := s.EvaluateCareerApplication(ctx, failedRequest)
	if err != nil || failed.Fact.Evaluation.InterviewEventID != answered.EventID {
		t.Fatalf("evaluation: %+v %v", failed, err)
	}
	if _, err := s.OfferCareerEmployment(ctx, careerTestOffer(t, s, "unqualified_offer", failed.Fact.RecordID)); !core.HasCode(err, core.CodeInvalidArgument) {
		t.Fatalf("advisory note bypassed failed qualification: %v", err)
	}
	passingRequest := careerTestEvaluation(t, s, "eval_passed", true)
	passing, err := s.EvaluateCareerApplication(ctx, passingRequest)
	if err != nil {
		t.Fatal(err)
	}
	offerRequest := careerTestOffer(t, s, "offer_ada", passing.Fact.RecordID)
	offerRequest.ExpiresAt = "2026-09-23T07:59:00+08:00"
	offered, err := s.OfferCareerEmployment(ctx, offerRequest)
	if err != nil {
		t.Fatal(err)
	}
	if offered.Fact.Offer.DailyWageMinor != 12 || offered.Fact.Offer.EvaluationEventID != passing.EventID || offered.Fact.Offer.Status != "offered" || offered.Fact.Offer.PositionKey == offered.Fact.Offer.PositionID {
		t.Fatalf("offer lost terms/scope/evidence: %+v", offered)
	}
	if offered.Fact.Offer.ExpiresAt != "2026-09-22T23:59:00Z" {
		t.Fatal("offer expiry did not preserve the requested instant")
	}
	if _, err := s.OfferCareerEmployment(ctx, careerTestOffer(t, s, "duplicate_offer", passing.Fact.RecordID)); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatalf("duplicate active offer: %v", err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM employment_contracts WHERE employee_entity_id=?`, []any{M2AgentAdaID}, 0)
	assertM2Value(t, ctx, s, `SELECT balance_minor FROM account_balances WHERE account_id=?`, []any{m2EconomyEmployerCash}, 1200)
	for _, record := range []CareerRecord{answered, offered} {
		got, err := s.ReadCareerRecruitmentRecord(ctx, M2AgentAdaPrincipal, M2DemoInstanceID, M2DemoBranchID, record.Fact.Kind, record.Fact.RecordID)
		if err != nil || !reflect.DeepEqual(got, record) {
			t.Fatalf("candidate record access: %+v %v", got, err)
		}
		if _, err := s.ReadCareerRecruitmentRecord(ctx, M2RPPlayerPrincipal, M2DemoInstanceID, M2DemoBranchID, record.Fact.Kind, record.Fact.RecordID); !core.HasCode(err, core.CodeUnauthorized) {
			t.Fatalf("private hiring record leaked: %v", err)
		}
	}
	if got, err := s.ReadCareerRecruitmentRecord(ctx, M2AgentBoPrincipal, M2DemoInstanceID, M2DemoBranchID, "evaluation", passing.Fact.RecordID); err != nil || !reflect.DeepEqual(got, passing) {
		t.Fatalf("manager evaluation read: %+v %v", got, err)
	}
	if _, err := s.ReadCareerRecruitmentRecord(ctx, M2AgentAdaPrincipal, M2DemoInstanceID, M2DemoBranchID, "evaluation", passing.Fact.RecordID); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("candidate read internal evaluator advice: %v", err)
	}
	declineRequest := core.CareerOfferDeclineRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "decline-for-candidate"), OfferID: offerRequest.OfferID, Reason: "Manager cannot choose for candidate."}
	if _, err := s.DeclineCareerOffer(ctx, declineRequest); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("manager declined for candidate: %v", err)
	}
	declineRequest.Binding = careerTestBinding(t, s, M2AgentAdaPrincipal, "decline")
	declined, err := s.DeclineCareerOffer(ctx, declineRequest)
	if err != nil || declined.Fact.Offer.Status != "declined" || declined.Fact.Offer.OfferEventID != offered.EventID {
		t.Fatalf("decline: %+v %v", declined, err)
	}
	// A historical offered snapshot must not block a new offer after decline.
	if _, err := s.OfferCareerEmployment(ctx, careerTestOffer(t, s, "second_offer", passing.Fact.RecordID)); err != nil {
		t.Fatalf("historical offer never closed: %v", err)
	}
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM events`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	for _, retry := range []func() (CareerRecord, error){
		func() (CareerRecord, error) { return s.InviteCareerInterview(ctx, inviteRequest) },
		func() (CareerRecord, error) { return s.AnswerCareerInterview(ctx, answerRequest) },
		func() (CareerRecord, error) { return s.EvaluateCareerApplication(ctx, passingRequest) },
		func() (CareerRecord, error) { return s.OfferCareerEmployment(ctx, offerRequest) },
		func() (CareerRecord, error) { return s.DeclineCareerOffer(ctx, declineRequest) },
	} {
		got, err := retry()
		if err != nil || !got.Replayed {
			t.Fatalf("recovery: %+v %v", got, err)
		}
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events`, nil, int64(count))
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM outbox o JOIN events e ON e.event_id=o.event_id WHERE e.event_type='RPCareerFactRecorded'`, nil, 0)
	if differences, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatalf("hiring replay: %+v %v", differences, err)
	}
}

func TestCareerEvaluationSupersessionRequirementsAndOfferExpiry(t *testing.T) {
	ctx := context.Background()
	s := openCareerTestWorld(t, filepath.Join(t.TempDir(), "gates.db"))
	defer s.Close()
	app := prepareCareerApplicant(t, s)
	if _, err := s.InviteCareerInterview(ctx, core.CareerInterviewRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "invite"), InterviewID: "interview_ada", ApplicationID: app.Fact.RecordID, Question: "Safety procedure?"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AnswerCareerInterview(ctx, core.CareerInterviewAnswerRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "answer"), InterviewID: "interview_ada", Answer: "Check the workplace."}); err != nil {
		t.Fatal(err)
	}
	missing := careerTestEvaluation(t, s, "missing", true)
	missing.Assessments = nil
	if _, err := s.EvaluateCareerApplication(ctx, missing); !core.HasCode(err, core.CodeInvalidArgument) {
		t.Fatalf("missing qualifications accepted: %v", err)
	}
	wrong := careerTestEvaluation(t, s, "wrong", true)
	wrong.Assessments[0].Code = "unrelated_skill"
	if _, err := s.EvaluateCareerApplication(ctx, wrong); !core.HasCode(err, core.CodeInvalidArgument) {
		t.Fatalf("unrelated qualifications accepted: %v", err)
	}
	if _, err := s.EvaluateCareerApplication(ctx, careerTestEvaluation(t, s, "old_pass", true)); err != nil {
		t.Fatal(err)
	}
	rejected := careerTestEvaluation(t, s, "new_reject", true)
	rejected.Decision = "reject"
	if _, err := s.EvaluateCareerApplication(ctx, rejected); err != nil {
		t.Fatal(err)
	}
	if _, err := s.OfferCareerEmployment(ctx, careerTestOffer(t, s, "old_offer", "old_pass")); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatalf("older favorable evaluation bypassed rejection: %v", err)
	}
	if _, err := s.OfferCareerEmployment(ctx, careerTestOffer(t, s, "reject_offer", "new_reject")); !core.HasCode(err, core.CodeInvalidArgument) {
		t.Fatalf("rejected evaluation generated offer: %v", err)
	}
	if _, err := s.EvaluateCareerApplication(ctx, careerTestEvaluation(t, s, "latest_pass", true)); err != nil {
		t.Fatal(err)
	}
	invalid := careerTestOffer(t, s, "past_offer", "latest_pass")
	invalid.ExpiresAt = rpLifeSetupTime
	if _, err := s.OfferCareerEmployment(ctx, invalid); !core.HasCode(err, core.CodeInvalidArgument) {
		t.Fatalf("already expired offer accepted: %v", err)
	}
	short := careerTestOffer(t, s, "short_offer", "latest_pass")
	short.ExpiresAt = "2026-09-22T07:04:00Z"
	if _, err := s.OfferCareerEmployment(ctx, short); err != nil {
		t.Fatal(err)
	}
	session, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: M2RPPlayerPrincipal, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2RPPlayerID, POV: "second_person", IdempotencyKey: "expiry-session"})
	if err != nil {
		t.Fatal(err)
	}
	observation, err := s.ObserveRPSession(ctx, core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID, TargetWorldTime: short.ExpiresAt, Budget: 10, ExpectedCursor: observation.ObservationCursor, IdempotencyKey: "wait-for-offer-expiry"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DeclineCareerOffer(ctx, core.CareerOfferDeclineRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "late_decline"), OfferID: short.OfferID, Reason: "Late reply"}); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatalf("expired offer remained actionable: %v", err)
	}
	if _, err := s.AcceptCareerOffer(ctx, core.CareerOfferAcceptRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "late_accept"), OfferID: short.OfferID, AfterWorkPlaceID: M2AgentCafeID}); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatalf("expired offer accepted: %v", err)
	}
	if _, err := s.OfferCareerEmployment(ctx, careerTestOffer(t, s, "fresh_offer", "latest_pass")); err != nil {
		t.Fatalf("expired offer blocked reoffer: %v", err)
	}
}
