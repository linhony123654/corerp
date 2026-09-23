package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/storage"
)

func TestCareerHTTPReferralInterviewAssessmentOfferAndRecovery(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "hiring-http.db")
	s, handler := openHTTPTestServer(t, ctx, path)
	defer func() { s.Close() }()
	setup, err := s.PrepareRPLifeDemo(ctx)
	if err != nil {
		t.Fatal(err)
	}
	binding := core.CareerBinding{PrincipalID: "principal_creator", InstanceID: storage.M2DemoInstanceID, BranchID: storage.M2DemoBranchID, ExpectedHead: setup.Routine.EventSequence, IdempotencyKey: "org"}
	org, err := s.DefineCareerOrganization(ctx, core.CareerOrganizationRequest{Binding: binding, Organization: core.CareerOrganizationDefinition{OrganizationID: "actor_m2_coop_employer", DisplayName: "Co-op", ManagerPrincipalID: storage.M2AgentBoPrincipal, WorkplaceID: "place_m2_work_ada"}})
	if err != nil {
		t.Fatal(err)
	}
	binding.PrincipalID, binding.ExpectedHead, binding.IdempotencyKey = storage.M2AgentBoPrincipal, org.EventSequence, "post"
	if _, err := s.PostCareerPosition(ctx, core.CareerPostingRequest{Binding: binding, Posting: core.CareerPostingDefinition{PositionID: "position_http_hiring", OrganizationID: org.Fact.RecordID, Title: "Assistant", OccupationID: "operations", Grade: "entry", Capacity: 1, DailyWageMinor: 12, RequiredQualifications: []string{"safety"}}}); err != nil {
		t.Fatal(err)
	}
	run, err := s.RunAgentLife(ctx, storage.M2AgentNoonTime, 100)
	if err != nil {
		t.Fatal(err)
	}
	response := performJSON(t, handler, "/api/v1/agent-knowledge/query", boAgentToken, core.AgentKnowledgeRead{CapabilityID: "world.agent.knowledge.read", InstanceID: binding.InstanceID, BranchID: binding.BranchID, ObserverAgentID: storage.M2AgentBoID, Fields: []string{"subject_agent_id", "source_event_id"}})
	assertStatus(t, response, http.StatusOK)
	knowledge := decodeData[storage.AgentKnowledgeView](t, response)
	var source string
	for _, fact := range knowledge.Facts {
		if fact.SubjectAgentID == storage.M2AgentAdaID {
			source = fact.SourceEventID
		}
	}
	if source == "" {
		t.Fatal("real lunch encounter did not yield permitted referral evidence")
	}
	binding.PrincipalID, binding.ExpectedHead, binding.IdempotencyKey = "", run.HeadSequence, "refer"
	referral := core.CareerReferralRequest{Binding: binding, ReferralID: "http_referral", PositionID: "position_http_hiring", ReferrerID: storage.M2AgentBoID, CandidateID: storage.M2AgentAdaID, SourceEventID: source, Note: "We met at lunch; please consider an interview."}
	response = performJSON(t, handler, "/api/v1/career/referrals/submit", boAgentToken, referral)
	assertStatus(t, response, http.StatusOK)
	referred := decodeData[storage.CareerRecord](t, response)
	binding.ExpectedHead, binding.IdempotencyKey = referred.EventSequence, "apply"
	application := core.CareerApplicationRequest{Binding: binding, ApplicationID: "http_referred_application", PositionID: referral.PositionID, CandidateID: storage.M2AgentAdaID, Statement: "I would like to apply.", ReferralID: referral.ReferralID}
	response = performJSON(t, handler, "/api/v1/career/applications/submit", adaAgentToken, application)
	assertStatus(t, response, http.StatusOK)
	applied := decodeData[storage.CareerRecord](t, response)
	if applied.Fact.Application.ReferralEventID != referred.EventID {
		t.Fatal("application omitted sourced referral")
	}
	binding.ExpectedHead, binding.IdempotencyKey = applied.EventSequence, "invite"
	invite := core.CareerInterviewRequest{Binding: binding, InterviewID: "http_interview", ApplicationID: application.ApplicationID, Question: "How would you open the workshop safely?"}
	response = performJSON(t, handler, "/api/v1/career/interviews/invite", boAgentToken, invite)
	assertStatus(t, response, http.StatusOK)
	invited := decodeData[storage.CareerRecord](t, response)
	binding.ExpectedHead, binding.IdempotencyKey = invited.EventSequence, "answer"
	answer := core.CareerInterviewAnswerRequest{Binding: binding, InterviewID: invite.InterviewID, Answer: "Check equipment and exits, report hazards and keep unsafe equipment closed."}
	response = performJSON(t, handler, "/api/v1/career/interviews/answer", boAgentToken, answer)
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)
	response = performJSON(t, handler, "/api/v1/career/interviews/answer", adaAgentToken, answer)
	assertStatus(t, response, http.StatusOK)
	answered := decodeData[storage.CareerRecord](t, response)
	binding.ExpectedHead, binding.IdempotencyKey = answered.EventSequence, "evaluate"
	evaluation := core.CareerEvaluationRequest{Binding: binding, EvaluationID: "http_evaluation", InterviewID: invite.InterviewID, Decision: "advance", Reason: "Organization review of the candidate's recorded answer", Assessments: []core.CareerQualificationAssessment{{Code: "safety", Passed: true, Reason: "Explains required checks"}}, AdvisoryNote: "Suggestion only, not hiring authority"}
	response = performJSON(t, handler, "/api/v1/career/evaluations/record", adaAgentToken, evaluation)
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)
	response = performJSON(t, handler, "/api/v1/career/evaluations/record", boAgentToken, evaluation)
	assertStatus(t, response, http.StatusOK)
	evaluated := decodeData[storage.CareerRecord](t, response)
	if evaluated.Fact.Evaluation.InterviewEventID != answered.EventID {
		t.Fatal("evaluation lacks actual response evidence")
	}
	response = performJSON(t, handler, "/api/v1/career/records/read", adaAgentToken, careerRecordRead{InstanceID: binding.InstanceID, BranchID: binding.BranchID, Kind: "evaluation", RecordID: evaluation.EvaluationID})
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)
	binding.ExpectedHead, binding.IdempotencyKey = evaluated.EventSequence, "offer"
	offer := core.CareerOfferRequest{Binding: binding, OfferID: "http_offer", EvaluationID: evaluation.EvaluationID, StartsOnDay: 2, ProbationDays: 7, ExpiresAt: "2026-09-23T23:59:00Z"}
	response = performJSON(t, handler, "/api/v1/career/offers/make", adaAgentToken, offer)
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)
	response = performJSON(t, handler, "/api/v1/career/offers/make", boAgentToken, offer)
	assertStatus(t, response, http.StatusOK)
	offered := decodeData[storage.CareerRecord](t, response)
	read := careerRecordRead{InstanceID: binding.InstanceID, BranchID: binding.BranchID, Kind: "offer", RecordID: offer.OfferID}
	response = performJSON(t, handler, "/api/v1/career/records/read", rpPlayerToken, read)
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)
	response = performJSON(t, handler, "/api/v1/career/records/read", adaAgentToken, read)
	assertStatus(t, response, http.StatusOK)
	binding.ExpectedHead, binding.IdempotencyKey = offered.EventSequence, "decline"
	decline := core.CareerOfferDeclineRequest{Binding: binding, OfferID: offer.OfferID, Reason: "The terms do not fit my plans."}
	response = performJSON(t, handler, "/api/v1/career/offers/decline", boAgentToken, decline)
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)
	response = performJSON(t, handler, "/api/v1/career/offers/decline", adaAgentToken, decline)
	assertStatus(t, response, http.StatusOK)
	declined := decodeData[storage.CareerRecord](t, response)
	if declined.Fact.Offer.Status != "declined" {
		t.Fatal("candidate decline did not change offer state")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, handler = openHTTPTestServer(t, ctx, path)
	for _, retry := range []struct {
		path, token string
		body        any
		eventID     string
	}{
		{"/api/v1/career/referrals/submit", boAgentToken, referral, referred.EventID},
		{"/api/v1/career/interviews/invite", boAgentToken, invite, invited.EventID},
		{"/api/v1/career/interviews/answer", adaAgentToken, answer, answered.EventID},
		{"/api/v1/career/evaluations/record", boAgentToken, evaluation, evaluated.EventID},
		{"/api/v1/career/offers/make", boAgentToken, offer, offered.EventID},
		{"/api/v1/career/offers/decline", adaAgentToken, decline, declined.EventID},
	} {
		response = performJSON(t, handler, retry.path, retry.token, retry.body)
		assertStatus(t, response, http.StatusOK)
		got := decodeData[storage.CareerRecord](t, response)
		if !got.Replayed || got.EventID != retry.eventID {
			t.Fatalf("recovery re-executed %s: %+v", retry.path, got)
		}
	}
	response = performJSON(t, handler, "/api/v1/career/records/read", adaAgentToken, read)
	assertStatus(t, response, http.StatusOK)
	if got := decodeData[storage.CareerRecord](t, response); got.EventID != declined.EventID || got.Fact.Offer.Status != "declined" {
		t.Fatal("read latest offer returned historical offered snapshot")
	}
	// Candidate may accept a subsequent conditional offer. This must create
	// actual payroll, not merely change the application label.
	offer.Binding.ExpectedHead, offer.Binding.IdempotencyKey, offer.OfferID = declined.EventSequence, "replacement-offer", "http_replacement_offer"
	response = performJSON(t, handler, "/api/v1/career/offers/make", boAgentToken, offer)
	assertStatus(t, response, http.StatusOK)
	replacement := decodeData[storage.CareerRecord](t, response)
	accept := core.CareerOfferAcceptRequest{Binding: core.CareerBinding{InstanceID: binding.InstanceID, BranchID: binding.BranchID, ExpectedHead: replacement.EventSequence, IdempotencyKey: "accept-replacement"}, OfferID: offer.OfferID, AfterWorkPlaceID: storage.M2AgentCafeID}
	response = performJSON(t, handler, "/api/v1/career/offers/accept", boAgentToken, accept)
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)
	response = performJSON(t, handler, "/api/v1/career/offers/accept", adaAgentToken, accept)
	assertStatus(t, response, http.StatusOK)
	accepted := decodeData[storage.CareerRecord](t, response)
	if accepted.Fact.Employment == nil || accepted.Fact.Offer.Status != "accepted" {
		t.Fatal("HTTP acceptance lacks actual contract")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, handler = openHTTPTestServer(t, ctx, path)
	response = performJSON(t, handler, "/api/v1/career/offers/accept", adaAgentToken, accept)
	assertStatus(t, response, http.StatusOK)
	if got := decodeData[storage.CareerRecord](t, response); !got.Replayed || got.EventID != accepted.EventID {
		t.Fatal("restarted acceptance repeated employment")
	}
	if _, err := s.RunAgentLife(ctx, "2026-09-25T00:01:00Z", 100); err != nil {
		t.Fatal(err)
	}
	events, err := s.ListVisibleEvents(ctx, core.VisibleEventRequest{PrincipalID: "principal_creator", CapabilityID: "world.events.read", InstanceID: binding.InstanceID, BranchID: binding.BranchID, SubjectID: "*", AfterSequence: accepted.EventSequence, Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	var paid int64
	for _, event := range events.Events {
		if event.EventType != "WagePaid" {
			continue
		}
		var payment struct {
			ObligationID string `json:"obligation_id"`
			PaidMinor    int64  `json:"paid_minor"`
		}
		if err := json.Unmarshal(event.Payload, &payment); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(payment.ObligationID, accepted.Fact.Employment.ContractID) {
			paid += payment.PaidMinor
		}
	}
	if paid != 12 {
		t.Fatalf("HTTP acceptance did not lead to actual wage payment: %d", paid)
	}
	attendanceRead := careerAttendanceRead{InstanceID: binding.InstanceID, BranchID: binding.BranchID, ContractID: accepted.Fact.Employment.ContractID, Day: 2}
	for _, token := range []string{adaAgentToken, boAgentToken} {
		response = performJSON(t, handler, "/api/v1/career/attendance/read", token, attendanceRead)
		assertStatus(t, response, http.StatusOK)
		if got := decodeData[storage.CareerAttendanceRecord](t, response); got.Attendance.Status != "complete" || got.Attendance.RecordedSeconds != 14400 {
			t.Fatalf("authenticated attendance report: %+v", got)
		}
	}
	response = performJSON(t, handler, "/api/v1/career/attendance/read", rpPlayerToken, attendanceRead)
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)
	attendanceRead.PrincipalID = storage.M2AgentAdaPrincipal
	response = performJSON(t, handler, "/api/v1/career/attendance/read", boAgentToken, attendanceRead)
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)
	attendanceRead.PrincipalID = ""
	response = performJSON(t, handler, "/api/v1/career/attendance/read", boAgentToken, attendanceRead)
	assertStatus(t, response, http.StatusOK)
	attendance := decodeData[storage.CareerAttendanceRecord](t, response)
	run, err = s.RunAgentLife(ctx, "2026-10-01T00:01:00Z", 1000)
	if err != nil {
		t.Fatal(err)
	}
	performance := core.CareerPerformanceRequest{Binding: core.CareerBinding{InstanceID: binding.InstanceID, BranchID: binding.BranchID, ExpectedHead: run.HeadSequence, IdempotencyKey: "performance"}, ReviewID: "http_review", ContractID: accepted.Fact.Employment.ContractID, EvidenceEventIDs: []string{attendance.EventID}, Assessment: "meets_expectations", Reason: "Private organization assessment"}
	response = performJSON(t, handler, "/api/v1/career/performance/record", adaAgentToken, performance)
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)
	response = performJSON(t, handler, "/api/v1/career/performance/record", boAgentToken, performance)
	assertStatus(t, response, http.StatusOK)
	reviewed := decodeData[storage.CareerRecord](t, response)
	regularize := core.CareerRegularizationRequest{Binding: core.CareerBinding{InstanceID: binding.InstanceID, BranchID: binding.BranchID, ExpectedHead: reviewed.EventSequence, IdempotencyKey: "regularize"}, ReviewID: performance.ReviewID, Notice: "Your probation is complete. Your position and pay remain unchanged."}
	response = performJSON(t, handler, "/api/v1/career/employment/regularize", adaAgentToken, regularize)
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)
	response = performJSON(t, handler, "/api/v1/career/employment/regularize", boAgentToken, regularize)
	assertStatus(t, response, http.StatusOK)
	regularized := decodeData[storage.CareerRecord](t, response)
	if regularized.Fact.Employment.LifecycleStatus != "regular" || regularized.Fact.EmploymentChange.PerformanceEventID != reviewed.EventID {
		t.Fatalf("HTTP regularization: %+v", regularized)
	}
	response = performJSON(t, handler, "/api/v1/career/records/read", adaAgentToken, careerRecordRead{InstanceID: binding.InstanceID, BranchID: binding.BranchID, Kind: "performance", RecordID: performance.ReviewID})
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, handler = openHTTPTestServer(t, ctx, path)
	response = performJSON(t, handler, "/api/v1/career/employment/regularize", boAgentToken, regularize)
	assertStatus(t, response, http.StatusOK)
	if got := decodeData[storage.CareerRecord](t, response); !got.Replayed || got.EventID != regularized.EventID {
		t.Fatal("HTTP recovery repeated regularization")
	}
	response = performJSON(t, handler, "/api/v1/career/performance/record", boAgentToken, performance)
	assertStatus(t, response, http.StatusOK)
	if got := decodeData[storage.CareerRecord](t, response); !got.Replayed || got.EventID != reviewed.EventID {
		t.Fatal("HTTP recovery repeated performance review")
	}
	response = performJSON(t, handler, "/api/v1/career/records/read", adaAgentToken, careerRecordRead{InstanceID: binding.InstanceID, BranchID: binding.BranchID, Kind: "employment", RecordID: accepted.Fact.Employment.ContractID})
	assertStatus(t, response, http.StatusOK)
	if got := decodeData[storage.CareerRecord](t, response); got.EventID != regularized.EventID || got.Fact.Performance != nil || got.Fact.EmploymentChange.Notice != regularize.Notice {
		t.Fatalf("employee notice or privacy: %+v", got)
	}
	leave := core.CareerLeaveRequest{Binding: core.CareerBinding{InstanceID: binding.InstanceID, BranchID: binding.BranchID, ExpectedHead: regularized.EventSequence, IdempotencyKey: "leave"}, LeaveID: "http_leave", ContractID: accepted.Fact.Employment.ContractID, StartDay: 10, EndDay: 12, Reason: "Personal appointment"}
	response = performJSON(t, handler, "/api/v1/career/leave/request", boAgentToken, leave)
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)
	response = performJSON(t, handler, "/api/v1/career/leave/request", adaAgentToken, leave)
	assertStatus(t, response, http.StatusOK)
	requestedLeave := decodeData[storage.CareerRecord](t, response)
	approveLeave := core.CareerLeaveReviewRequest{Binding: core.CareerBinding{InstanceID: binding.InstanceID, BranchID: binding.BranchID, ExpectedHead: requestedLeave.EventSequence, IdempotencyKey: "approve-leave"}, LeaveID: leave.LeaveID, Decision: "approve", Notice: "Approved with base pay unchanged."}
	response = performJSON(t, handler, "/api/v1/career/leave/review", adaAgentToken, approveLeave)
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)
	response = performJSON(t, handler, "/api/v1/career/leave/review", boAgentToken, approveLeave)
	assertStatus(t, response, http.StatusOK)
	approvedLeave := decodeData[storage.CareerRecord](t, response)
	if approvedLeave.Fact.Leave.Status != "approved" || len(approvedLeave.Fact.Leave.CancelledSchedules) != 2 {
		t.Fatalf("HTTP leave lacked actual schedule cancellation: %+v", approvedLeave)
	}
	response = performJSON(t, handler, "/api/v1/career/records/read", rpPlayerToken, careerRecordRead{InstanceID: binding.InstanceID, BranchID: binding.BranchID, Kind: "leave", RecordID: leave.LeaveID})
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, handler = openHTTPTestServer(t, ctx, path)
	response = performJSON(t, handler, "/api/v1/career/leave/review", boAgentToken, approveLeave)
	assertStatus(t, response, http.StatusOK)
	if got := decodeData[storage.CareerRecord](t, response); !got.Replayed || got.EventID != approvedLeave.EventID {
		t.Fatal("restart repeated leave approval")
	}
	run, err = s.RunAgentLife(ctx, "2026-10-03T00:01:00Z", 100)
	if err != nil {
		t.Fatal(err)
	}
	attendanceRead.Day = 10
	response = performJSON(t, handler, "/api/v1/career/attendance/read", adaAgentToken, attendanceRead)
	assertStatus(t, response, http.StatusOK)
	if got := decodeData[storage.CareerAttendanceRecord](t, response); got.Attendance.Status != "approved_leave" || got.Attendance.LeaveEventID != approvedLeave.EventID || got.Attendance.RecordedSeconds != 0 {
		t.Fatalf("leave failed after recurring payroll: %+v", got)
	}
	consideredOffer := core.CareerOvertimeRequest{Binding: core.CareerBinding{InstanceID: binding.InstanceID, BranchID: binding.BranchID, ExpectedHead: run.HeadSequence, IdempotencyKey: "offer-considered-overtime"}, OvertimeID: "http_considered_overtime", ContractID: accepted.Fact.Employment.ContractID, Day: 12, StartHour: 13, EndHour: 15, RateMinorPerHour: 7, Reason: "An optional proposal to consider."}
	response = performJSON(t, handler, "/api/v1/career/overtime/offer", boAgentToken, consideredOffer)
	assertStatus(t, response, http.StatusOK)
	consider := core.CareerOvertimeResponseRequest{Binding: core.CareerBinding{InstanceID: binding.InstanceID, BranchID: binding.BranchID, ExpectedHead: decodeData[storage.CareerRecord](t, response).EventSequence, IdempotencyKey: "consider-overtime"}, OvertimeID: consideredOffer.OvertimeID, Decision: "consider", Reason: "Consider using my own experience."}
	assertAPIError(t, performJSON(t, handler, "/api/v1/career/overtime/respond", boAgentToken, consider), http.StatusForbidden, core.CodeUnauthorized)
	response = performJSON(t, handler, "/api/v1/career/overtime/respond", adaAgentToken, consider)
	assertStatus(t, response, http.StatusOK)
	considered := decodeData[storage.CareerRecord](t, response)
	if considered.Fact.Overtime.Status != "declined" || considered.Fact.Overtime.Choice == nil {
		t.Fatalf("HTTP own choice: %+v", considered)
	}
	response = performJSON(t, handler, "/api/v1/career/records/read", boAgentToken, careerRecordRead{InstanceID: binding.InstanceID, BranchID: binding.BranchID, Kind: "overtime", RecordID: consideredOffer.OvertimeID})
	assertStatus(t, response, http.StatusOK)
	if got := decodeData[storage.CareerRecord](t, response); got.Fact.Overtime.Choice != nil {
		t.Fatal("HTTP manager saw private choice evidence")
	}
	run.HeadSequence = considered.EventSequence
	overtime := core.CareerOvertimeRequest{Binding: core.CareerBinding{InstanceID: binding.InstanceID, BranchID: binding.BranchID, ExpectedHead: run.HeadSequence, IdempotencyKey: "offer-overtime"}, OvertimeID: "http_overtime", ContractID: accepted.Fact.Employment.ContractID, Day: 12, StartHour: 13, EndHour: 15, RateMinorPerHour: 7, Reason: "Optional extra work after leave"}
	response = performJSON(t, handler, "/api/v1/career/overtime/offer", adaAgentToken, overtime)
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)
	response = performJSON(t, handler, "/api/v1/career/overtime/offer", boAgentToken, overtime)
	assertStatus(t, response, http.StatusOK)
	offeredOvertime := decodeData[storage.CareerRecord](t, response)
	acceptOvertime := core.CareerOvertimeResponseRequest{Binding: core.CareerBinding{InstanceID: binding.InstanceID, BranchID: binding.BranchID, ExpectedHead: offeredOvertime.EventSequence, IdempotencyKey: "accept-overtime"}, OvertimeID: overtime.OvertimeID, Decision: "accept", Reason: "I agree to the specified window and rate."}
	response = performJSON(t, handler, "/api/v1/career/overtime/respond", boAgentToken, acceptOvertime)
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)
	response = performJSON(t, handler, "/api/v1/career/overtime/respond", adaAgentToken, acceptOvertime)
	assertStatus(t, response, http.StatusOK)
	agreedOvertime := decodeData[storage.CareerRecord](t, response)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, handler = openHTTPTestServer(t, ctx, path)
	response = performJSON(t, handler, "/api/v1/career/overtime/respond", adaAgentToken, acceptOvertime)
	assertStatus(t, response, http.StatusOK)
	if got := decodeData[storage.CareerRecord](t, response); !got.Replayed || got.EventID != agreedOvertime.EventID {
		t.Fatal("recovery repeated overtime consent")
	}
	run, err = s.RunAgentLife(ctx, "2026-10-05T00:01:00Z", 1000)
	if err != nil {
		t.Fatal(err)
	}
	attendanceRead.Day = 12
	response = performJSON(t, handler, "/api/v1/career/attendance/read", adaAgentToken, attendanceRead)
	assertStatus(t, response, http.StatusOK)
	if got := decodeData[storage.CareerAttendanceRecord](t, response); got.BaseEarnedMinor != 12 || got.OvertimeEarnedMinor != 14 || len(got.Overtime) != 1 || got.Overtime[0].AgreementEventID != agreedOvertime.EventID || got.Overtime[0].Attendance.RecordedSeconds != 7200 {
		t.Fatalf("real overtime accrual report: %+v", got)
	}
	raise := core.CareerRaiseRequest{Binding: core.CareerBinding{InstanceID: binding.InstanceID, BranchID: binding.BranchID, ExpectedHead: run.HeadSequence, IdempotencyKey: "raise"}, ContractID: accepted.Fact.Employment.ContractID, DailyWageMinor: 20, EffectiveFromDay: 14, Notice: "Your daily base increases to20 from day14."}
	response = performJSON(t, handler, "/api/v1/career/employment/raise", adaAgentToken, raise)
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)
	response = performJSON(t, handler, "/api/v1/career/employment/raise", boAgentToken, raise)
	assertStatus(t, response, http.StatusOK)
	raised := decodeData[storage.CareerRecord](t, response)
	if raised.Fact.Employment.DailyWageMinor != 20 || raised.Fact.Employment.EffectiveFromDay != 14 {
		t.Fatalf("HTTP future raise: %+v", raised)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, handler = openHTTPTestServer(t, ctx, path)
	response = performJSON(t, handler, "/api/v1/career/employment/raise", boAgentToken, raise)
	assertStatus(t, response, http.StatusOK)
	if got := decodeData[storage.CareerRecord](t, response); !got.Replayed || got.EventID != raised.EventID {
		t.Fatal("recovery repeated raise")
	}
	run, err = s.RunAgentLife(ctx, "2026-10-07T00:01:00Z", 1000)
	if err != nil {
		t.Fatal(err)
	}
	for day, amount := range map[int]int64{13: 12, 14: 20} {
		attendanceRead.Day = day
		response = performJSON(t, handler, "/api/v1/career/attendance/read", adaAgentToken, attendanceRead)
		assertStatus(t, response, http.StatusOK)
		if got := decodeData[storage.CareerAttendanceRecord](t, response); got.BaseEarnedMinor != amount {
			t.Fatalf("HTTP effective pay day %d: %+v", day, got)
		}
	}
	// An actual work report after the raise supplies current-term evidence;
	// the earlier probation review must not authorize the new position.
	attendanceRead.Day = 14
	response = performJSON(t, handler, "/api/v1/career/attendance/read", boAgentToken, attendanceRead)
	assertStatus(t, response, http.StatusOK)
	recent := decodeData[storage.CareerAttendanceRecord](t, response)
	positionBinding := core.CareerBinding{InstanceID: binding.InstanceID, BranchID: binding.BranchID, ExpectedHead: run.HeadSequence, IdempotencyKey: "position-review"}
	positionReview := core.CareerPerformanceRequest{Binding: positionBinding, ReviewID: "position-review", ContractID: accepted.Fact.Employment.ContractID, EvidenceEventIDs: []string{recent.EventID}, Assessment: "meets_expectations", Reason: "Private current-term position review"}
	response = performJSON(t, handler, "/api/v1/career/performance/record", boAgentToken, positionReview)
	assertStatus(t, response, http.StatusOK)
	positionBinding.ExpectedHead, positionBinding.IdempotencyKey = decodeData[storage.CareerRecord](t, response).EventSequence, "position-scale"
	response = performJSON(t, handler, "/api/v1/career/grades/define", boAgentToken, core.CareerGradeScaleRequest{Binding: positionBinding, Scale: core.CareerGradeScale{OrganizationID: org.Fact.OrganizationID, Grades: []string{"entry", "lead"}}})
	assertStatus(t, response, http.StatusOK)
	positionBinding.ExpectedHead, positionBinding.IdempotencyKey = decodeData[storage.CareerRecord](t, response).EventSequence, "lead-posting"
	response = performJSON(t, handler, "/api/v1/career/positions/post", boAgentToken, core.CareerPostingRequest{Binding: positionBinding, Posting: core.CareerPostingDefinition{PositionID: "lead-position", OrganizationID: org.Fact.OrganizationID, Title: "Lead", OccupationID: "operations", Grade: "lead", Capacity: 1, DailyWageMinor: 30, RequiredQualifications: []string{"coordination"}, Capabilities: []string{core.CareerPositionManageCapability}}})
	assertStatus(t, response, http.StatusOK)
	positionBinding.ExpectedHead, positionBinding.IdempotencyKey = decodeData[storage.CareerRecord](t, response).EventSequence, "position-change"
	positionOffer := core.CareerPositionOfferRequest{Binding: positionBinding, ChangeID: "http-position-change", ReviewID: positionReview.ReviewID, PositionID: "lead-position", EffectiveFromDay: 16, Assessments: []core.CareerQualificationAssessment{{Code: "coordination", Passed: true, Reason: "Private qualification assessment"}}, Notice: "Proposed lead role from day16, if you agree."}
	assertAPIError(t, performJSON(t, handler, "/api/v1/career/position-changes/offer", adaAgentToken, positionOffer), http.StatusForbidden, core.CodeUnauthorized)
	response = performJSON(t, handler, "/api/v1/career/position-changes/offer", boAgentToken, positionOffer)
	assertStatus(t, response, http.StatusOK)
	proposed := decodeData[storage.CareerRecord](t, response)
	positionRead := careerRecordRead{InstanceID: binding.InstanceID, BranchID: binding.BranchID, Kind: "position_change", RecordID: positionOffer.ChangeID}
	response = performJSON(t, handler, "/api/v1/career/records/read", adaAgentToken, positionRead)
	assertStatus(t, response, http.StatusOK)
	if strings.Contains(response.Body.String(), "Private qualification") || strings.Contains(response.Body.String(), "performance_event_id") {
		t.Fatal("position proposal leaked private assessment")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, handler = openHTTPTestServer(t, ctx, path)
	response = performJSON(t, handler, "/api/v1/career/position-changes/offer", boAgentToken, positionOffer)
	assertStatus(t, response, http.StatusOK)
	if got := decodeData[storage.CareerRecord](t, response); !got.Replayed || got.EventID != proposed.EventID {
		t.Fatal("restart repeated proposal")
	}
	positionBinding.ExpectedHead, positionBinding.IdempotencyKey = proposed.EventSequence, "decline-position"
	declinePosition := core.CareerPositionDeclineRequest{Binding: positionBinding, ChangeID: positionOffer.ChangeID, Reason: "I prefer the current role."}
	assertAPIError(t, performJSON(t, handler, "/api/v1/career/position-changes/decline", boAgentToken, declinePosition), http.StatusForbidden, core.CodeUnauthorized)
	response = performJSON(t, handler, "/api/v1/career/position-changes/decline", adaAgentToken, declinePosition)
	assertStatus(t, response, http.StatusOK)
	if got := decodeData[storage.CareerRecord](t, response); got.Fact.PositionAssessment != nil || got.Fact.PositionChange.Status != "declined" {
		t.Fatalf("employee response privacy: %+v", got)
	}
	positionOffer.Binding.ExpectedHead = decodeData[storage.CareerRecord](t, response).EventSequence
	positionOffer.Binding.IdempotencyKey, positionOffer.ChangeID = "replacement-position", "replacement-position"
	response = performJSON(t, handler, "/api/v1/career/position-changes/offer", boAgentToken, positionOffer)
	assertStatus(t, response, http.StatusOK)
	positionBinding.ExpectedHead, positionBinding.IdempotencyKey = decodeData[storage.CareerRecord](t, response).EventSequence, "accept-position"
	acceptPosition := core.CareerPositionAcceptRequest{Binding: positionBinding, ChangeID: positionOffer.ChangeID}
	assertAPIError(t, performJSON(t, handler, "/api/v1/career/position-changes/accept", boAgentToken, acceptPosition), http.StatusForbidden, core.CodeUnauthorized)
	response = performJSON(t, handler, "/api/v1/career/position-changes/accept", adaAgentToken, acceptPosition)
	assertStatus(t, response, http.StatusOK)
	changed := decodeData[storage.CareerRecord](t, response)
	if changed.Fact.PositionAssessment != nil || changed.Fact.Employment.PositionID != "lead-position" {
		t.Fatalf("accept source/privacy: %+v", changed)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, handler = openHTTPTestServer(t, ctx, path)
	response = performJSON(t, handler, "/api/v1/career/position-changes/accept", adaAgentToken, acceptPosition)
	assertStatus(t, response, http.StatusOK)
	if got := decodeData[storage.CareerRecord](t, response); !got.Replayed || got.EventID != changed.EventID {
		t.Fatal("restart repeated position consent")
	}
	probe := core.CareerPostingRequest{Binding: core.CareerBinding{InstanceID: binding.InstanceID, BranchID: binding.BranchID, ExpectedHead: changed.EventSequence, IdempotencyKey: "authority-probe"}, Posting: core.CareerPostingDefinition{PositionID: "authority-probe", OrganizationID: org.Fact.OrganizationID, Title: "Assistant", OccupationID: "operations", Grade: "entry", Capacity: 1, DailyWageMinor: 12}}
	assertAPIError(t, performJSON(t, handler, "/api/v1/career/positions/post", adaAgentToken, probe), http.StatusForbidden, core.CodeUnauthorized)
	run, err = s.RunAgentLife(ctx, "2026-10-09T00:01:00Z", 1000)
	if err != nil {
		t.Fatal(err)
	}
	for day, amount := range map[int]int64{15: 20, 16: 30} {
		attendanceRead.Day = day
		response = performJSON(t, handler, "/api/v1/career/attendance/read", adaAgentToken, attendanceRead)
		assertStatus(t, response, http.StatusOK)
		if got := decodeData[storage.CareerAttendanceRecord](t, response); got.BaseEarnedMinor != amount {
			t.Fatalf("position effective pay day %d: %+v", day, got)
		}
	}
	probe.Binding.ExpectedHead = run.HeadSequence
	response = performJSON(t, handler, "/api/v1/career/positions/post", adaAgentToken, probe)
	assertStatus(t, response, http.StatusOK)
	positionBinding.ExpectedHead, positionBinding.IdempotencyKey = decodeData[storage.CareerRecord](t, response).EventSequence, "role-demotion-review"
	attendanceRead.Day = 16
	response = performJSON(t, handler, "/api/v1/career/attendance/read", boAgentToken, attendanceRead)
	assertStatus(t, response, http.StatusOK)
	roleReview := core.CareerPerformanceRequest{Binding: positionBinding, ReviewID: "role-demotion-review", ContractID: accepted.Fact.Employment.ContractID, EvidenceEventIDs: []string{decodeData[storage.CareerAttendanceRecord](t, response).EventID}, Assessment: "needs_improvement", Reason: "Role review based on actual lead work"}
	response = performJSON(t, handler, "/api/v1/career/performance/record", boAgentToken, roleReview)
	assertStatus(t, response, http.StatusOK)
	positionBinding.ExpectedHead, positionBinding.IdempotencyKey = decodeData[storage.CareerRecord](t, response).EventSequence, "role-demotion"
	demotion := core.CareerPositionOfferRequest{Binding: positionBinding, ChangeID: "role-demotion", ReviewID: roleReview.ReviewID, PositionID: "position_http_hiring", EffectiveFromDay: 18, Assessments: []core.CareerQualificationAssessment{{Code: "safety", Passed: true, Reason: "Still qualified for assistant work"}}, Notice: "Return to assistant role without management authority"}
	response = performJSON(t, handler, "/api/v1/career/position-changes/offer", boAgentToken, demotion)
	assertStatus(t, response, http.StatusOK)
	positionBinding.ExpectedHead, positionBinding.IdempotencyKey = decodeData[storage.CareerRecord](t, response).EventSequence, "accept-demotion"
	response = performJSON(t, handler, "/api/v1/career/position-changes/accept", adaAgentToken, core.CareerPositionAcceptRequest{Binding: positionBinding, ChangeID: demotion.ChangeID})
	assertStatus(t, response, http.StatusOK)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, handler = openHTTPTestServer(t, ctx, path)
	run, err = s.RunAgentLife(ctx, "2026-10-11T00:01:00Z", 1000)
	if err != nil {
		t.Fatal(err)
	}
	probe.Binding.ExpectedHead, probe.Binding.IdempotencyKey, probe.Posting.PositionID = run.HeadSequence, "revoked-probe", "revoked-probe"
	assertAPIError(t, performJSON(t, handler, "/api/v1/career/positions/post", adaAgentToken, probe), http.StatusForbidden, core.CodeUnauthorized)
	announcement := core.CareerAnnouncementRequest{Binding: core.CareerBinding{InstanceID: binding.InstanceID, BranchID: binding.BranchID, ExpectedHead: run.HeadSequence, IdempotencyKey: "career-announcement"}, AnnouncementID: "career-announcement", ContractID: accepted.Fact.Employment.ContractID, SpeakerID: storage.M2AgentBoID}
	assertAPIError(t, performJSON(t, handler, "/api/v1/career/announcements/speak", adaAgentToken, announcement), http.StatusForbidden, core.CodeUnauthorized)
	forgedAnnouncement := announcement
	forgedAnnouncement.SpeakerID = storage.M2AgentAdaID
	assertAPIError(t, performJSON(t, handler, "/api/v1/career/announcements/speak", boAgentToken, forgedAnnouncement), http.StatusForbidden, core.CodeUnauthorized)
	response = performJSON(t, handler, "/api/v1/career/announcements/speak", boAgentToken, announcement)
	assertStatus(t, response, http.StatusOK)
	news := decodeData[storage.CareerRecord](t, response)
	if news.Fact.Announcement == nil || !strings.Contains(news.Fact.Announcement.Text, "grade entry") || strings.Contains(news.Fact.Announcement.Text, "Role review") {
		t.Fatalf("current public role announcement: %+v", news)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, handler = openHTTPTestServer(t, ctx, path)
	response = performJSON(t, handler, "/api/v1/career/announcements/speak", boAgentToken, announcement)
	assertStatus(t, response, http.StatusOK)
	if got := decodeData[storage.CareerRecord](t, response); !got.Replayed || got.EventID != news.EventID {
		t.Fatal("restart repeated career announcement")
	}
	exit := core.CareerExitRequest{Binding: core.CareerBinding{InstanceID: binding.InstanceID, BranchID: binding.BranchID, ExpectedHead: news.EventSequence, IdempotencyKey: "layoff"}, ContractID: accepted.Fact.Employment.ContractID, Kind: "layoff", EffectiveFromDay: 20, Notice: "Workforce reduction from day20; earned wages remain owed"}
	assertAPIError(t, performJSON(t, handler, "/api/v1/career/employment/end", adaAgentToken, exit), http.StatusForbidden, core.CodeUnauthorized)
	response = performJSON(t, handler, "/api/v1/career/employment/end", boAgentToken, exit)
	assertStatus(t, response, http.StatusOK)
	exitNotice := decodeData[storage.CareerRecord](t, response)
	if exitNotice.Fact.Exit == nil || exitNotice.Fact.Employment.EndsOnDay != 20 {
		t.Fatalf("exit notice: %+v", exitNotice)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, handler = openHTTPTestServer(t, ctx, path)
	response = performJSON(t, handler, "/api/v1/career/employment/end", boAgentToken, exit)
	assertStatus(t, response, http.StatusOK)
	if got := decodeData[storage.CareerRecord](t, response); !got.Replayed || got.EventID != exitNotice.EventID {
		t.Fatal("restart repeated exit notice")
	}
	if _, err := s.RunAgentLife(ctx, "2026-10-12T00:01:00Z", 1000); err != nil {
		t.Fatal(err)
	}
	attendanceRead.Day = 19
	response = performJSON(t, handler, "/api/v1/career/attendance/read", adaAgentToken, attendanceRead)
	assertStatus(t, response, http.StatusOK)
	if got := decodeData[storage.CareerAttendanceRecord](t, response); got.BaseEarnedMinor != 12 {
		t.Fatalf("final earned day lost: %+v", got)
	}
	response = performJSON(t, handler, "/api/v1/career/positions/query", adaAgentToken, careerMarketRead{InstanceID: binding.InstanceID, BranchID: binding.BranchID, CandidateID: storage.M2AgentAdaID})
	assertStatus(t, response, http.StatusOK)
	foundVacancy := false
	for _, p := range decodeData[storage.CareerMarket](t, response).Postings {
		if p.Posting.PositionID == "position_http_hiring" && p.AvailableSlots == 1 {
			foundVacancy = true
		}
	}
	if !foundVacancy {
		t.Fatal("exit did not reopen vacancy via API")
	}
}
