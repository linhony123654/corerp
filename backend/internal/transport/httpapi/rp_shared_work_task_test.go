package httpapi

import (
	"context"
	"database/sql"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/storage"
)

func TestRPSharedWorkTaskHTTPAuthenticatedHumanGateAndBoundedReceipt(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "shared-work-http.db")
	s, err := storage.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.BootstrapDemo(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PrepareRPLifeDemo(ctx); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	head := func() int64 {
		var sequence int64
		if err := db.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, storage.M2DemoInstanceID, storage.M2DemoBranchID).Scan(&sequence); err != nil {
			t.Fatal(err)
		}
		return sequence
	}
	binding := func(principal, key string) core.CareerBinding {
		return core.CareerBinding{PrincipalID: principal, InstanceID: storage.M2DemoInstanceID,
			BranchID: storage.M2DemoBranchID, ExpectedHead: head(), IdempotencyKey: key}
	}
	org, err := s.DefineCareerOrganization(ctx, core.CareerOrganizationRequest{Binding: binding("principal_creator", "work-http-org"),
		Organization: core.CareerOrganizationDefinition{OrganizationID: "actor_m2_coop_employer", DisplayName: "Work HTTP employer",
			ManagerPrincipalID: storage.M2AgentBoPrincipal, WorkplaceID: "place_m2_work_ada"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.PostCareerPosition(ctx, core.CareerPostingRequest{Binding: binding(storage.M2AgentBoPrincipal, "work-http-post"),
		Posting: core.CareerPostingDefinition{PositionID: "position_work_http", OrganizationID: org.Fact.RecordID,
			Title: "Assistant", OccupationID: "operations", Grade: "entry", Capacity: 1, DailyWageMinor: 12,
			RequiredQualifications: []string{"safety_training"}}}); err != nil {
		t.Fatal(err)
	}
	app, err := s.ApplyForCareerPosition(ctx, core.CareerApplicationRequest{Binding: binding(storage.M2AgentAdaPrincipal, "work-http-apply"),
		ApplicationID: "application_work_http", PositionID: "position_work_http", CandidateID: storage.M2AgentAdaID,
		Statement: "I want the position."})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.InviteCareerInterview(ctx, core.CareerInterviewRequest{Binding: binding(storage.M2AgentBoPrincipal, "work-http-invite"),
		InterviewID: "interview_work_http", ApplicationID: app.Fact.RecordID, Question: "How would you check safety?"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AnswerCareerInterview(ctx, core.CareerInterviewAnswerRequest{Binding: binding(storage.M2AgentAdaPrincipal, "work-http-answer"),
		InterviewID: "interview_work_http", Answer: "Inspect equipment and exits."}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.EvaluateCareerApplication(ctx, core.CareerEvaluationRequest{Binding: binding(storage.M2AgentBoPrincipal, "work-http-evaluate"),
		EvaluationID: "evaluation_work_http", InterviewID: "interview_work_http", Decision: "advance", Reason: "Recorded answer",
		Assessments: []core.CareerQualificationAssessment{{Code: "safety_training", Passed: true, Reason: "Explained checks"}}}); err != nil {
		t.Fatal(err)
	}
	offer, err := s.OfferCareerEmployment(ctx, core.CareerOfferRequest{Binding: binding(storage.M2AgentBoPrincipal, "work-http-offer"),
		OfferID: "offer_work_http", EvaluationID: "evaluation_work_http", StartsOnDay: 1, ProbationDays: 7,
		ExpiresAt: "2026-09-22T23:59:00Z"})
	if err != nil {
		t.Fatal(err)
	}
	employed, err := s.AcceptCareerOffer(ctx, core.CareerOfferAcceptRequest{Binding: binding(storage.M2AgentAdaPrincipal, "work-http-accept"),
		OfferID: offer.Fact.RecordID, AfterWorkPlaceID: storage.M2AgentCafeID})
	if err != nil {
		t.Fatal(err)
	}
	contract := employed.Fact.Employment.ContractID
	if _, err := db.ExecContext(ctx, `INSERT INTO principals(principal_id,principal_type,display_name,status) VALUES (?,'service',?,'active')`, roundServiceAID, roundServiceAID); err != nil {
		t.Fatal(err)
	}
	handler := newSharedRoundHTTPHandler(t, s)
	open := func(token, entity, key string) string {
		response := performJSON(t, handler, "/api/v1/rp/sessions/open", token, core.RPSessionOpenRequest{
			InstanceID: storage.M2DemoInstanceID, BranchID: storage.M2DemoBranchID, EntityID: entity,
			POV: "second_person", IdempotencyKey: key})
		assertStatus(t, response, http.StatusOK)
		return decodeData[storage.RPSession](t, response).SessionID
	}
	human := open(rpPlayerToken, storage.M2RPPlayerID, "work-http-human")
	response := performJSON(t, handler, "/api/v1/rp/observe", rpPlayerToken, core.RPSessionReadRequest{SessionID: human})
	assertStatus(t, response, http.StatusOK)
	humanView := decodeData[storage.RPObservation](t, response)
	response = performJSON(t, handler, "/api/v1/rp/actions/wait", rpPlayerToken, core.RPWaitRequest{
		SessionID: human, ExpectedCursor: humanView.ObservationCursor, TargetWorldTime: "2026-09-23T09:00:00Z",
		Budget: 1000, IdempotencyKey: "work-http-shift"})
	assertStatus(t, response, http.StatusOK)
	if wait := decodeData[storage.RPWaitResult](t, response); wait.Status != "completed" {
		t.Fatal("did not reach active shift", wait)
	}
	if _, err := s.EnrollRPExternalControllerLocal(ctx, storage.RPExternalControllerEnrollmentRequest{Binding: binding("principal_operator", "work-http-enroll"),
		EntityID: storage.M2AgentAdaID, ControllerPrincipalID: roundServiceAID, ControllerInstanceID: "work-http-controller"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AssignRPExternalControllerLocal(ctx, storage.RPExternalControllerAssignmentRequest{Binding: binding("principal_operator", "work-http-assign"),
		EntityID: storage.M2AgentAdaID, ExpectedGeneration: 0}); err != nil {
		t.Fatal(err)
	}
	service := open(roundServiceAToken, storage.M2AgentAdaID, "work-http-service")
	for _, participant := range []struct{ token, session string }{{rpPlayerToken, human}, {roundServiceAToken, service}} {
		assertStatus(t, performJSON(t, handler, "/api/v1/rp/observe", participant.token,
			core.RPSessionReadRequest{SessionID: participant.session}), http.StatusOK)
	}
	response = performJSON(t, handler, "/api/v1/rp/rounds/open", operatorToken, storage.RPSharedRoundOpenRequest{
		Binding: binding("principal_operator", "work-http-round"), HumanSessionID: human, ExternalSessionIDs: []string{service}})
	assertStatus(t, response, http.StatusOK)
	round := decodeData[storage.RPSharedRound](t, response)
	proposal := storage.RPSharedWorkTaskRequest{SessionID: service, RoundID: round.RoundID, ContractID: contract,
		TaskCode: "routine_check", IdempotencyKey: "work-http-proposal"}
	assertAPIError(t, performJSON(t, handler, "/api/v1/rp/rounds/work-task", rpPlayerToken, proposal), http.StatusNotFound, core.CodeNotFound)
	response = performJSON(t, handler, "/api/v1/rp/rounds/work-task", roundServiceAToken, proposal)
	assertStatus(t, response, http.StatusOK)
	if submitted := decodeData[storage.RPSharedRound](t, response); submitted.Submitted != 1 {
		t.Fatal("work proposal not private", submitted)
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE event_type='RPWorkTaskAttempted'`).Scan(&count); err != nil || count != 0 {
		t.Fatal("work Event before Human", count, err)
	}
	response = performJSON(t, handler, "/api/v1/rp/rounds/wait", rpPlayerToken, storage.RPSharedWaitRequest{
		SessionID: human, RoundID: round.RoundID, HorizonWorldTime: "2026-09-23T09:10:00Z", IdempotencyKey: "work-http-human-wait"})
	assertStatus(t, response, http.StatusOK)
	response = performJSON(t, handler, "/api/v1/rp/rounds/advance", roundServiceAToken, storage.RPSharedRoundAdvanceRequest{
		RPSharedRoundReadRequest: storage.RPSharedRoundReadRequest{SessionID: service, RoundID: round.RoundID}, Budget: 1000})
	assertStatus(t, response, http.StatusOK)
	settled := decodeData[storage.RPSharedRound](t, response)
	if settled.Status != "settled" || settled.OwnDisposition != "action_accepted" || settled.CurrentWorldTime != "2026-09-23T09:00:00Z" {
		t.Fatal("HTTP work task did not settle", settled)
	}
	if strings.Contains(response.Body.String(), storage.M2AgentAdaID) || strings.Contains(response.Body.String(), "fatigue_level") || strings.Contains(response.Body.String(), "work_source_event_id") {
		t.Fatal("HTTP shared work receipt leaked private cause", response.Body.String())
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE event_type='RPWorkTaskAttempted'`).Scan(&count); err != nil || count != 1 {
		t.Fatal("typed work Event count", count, err)
	}
	if diffs, err := s.CompareProjections(ctx, storage.M2DemoInstanceID, storage.M2DemoBranchID); err != nil || len(diffs) != 0 {
		t.Fatal("HTTP shared work replay", diffs, err)
	}
}
