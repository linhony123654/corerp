package httpapi

import (
	"context"
	"database/sql"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/storage"
)

func TestRPSharedOrganizationNoticeAccessHTTPCurrentEmployeeOnly(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "shared-organization-http.db")
	s, _ := openHTTPTestServer(t, ctx, path)
	defer s.Close()
	setup, err := s.PrepareRPLifeDemo(ctx)
	if err != nil {
		t.Fatal(err)
	}
	head := setup.Routine.EventSequence
	binding := func(principal, key string) core.CareerBinding {
		return core.CareerBinding{PrincipalID: principal, InstanceID: storage.M2DemoInstanceID,
			BranchID: storage.M2DemoBranchID, ExpectedHead: head, IdempotencyKey: key}
	}
	org, err := s.DefineCareerOrganization(ctx, core.CareerOrganizationRequest{Binding: binding("principal_creator", "shared-http-org"),
		Organization: core.CareerOrganizationDefinition{OrganizationID: "actor_m2_coop_employer", DisplayName: "Co-op",
			ManagerPrincipalID: storage.M2AgentBoPrincipal, WorkplaceID: "place_m2_work_ada"}})
	if err != nil {
		t.Fatal(err)
	}
	head = org.EventSequence
	post, err := s.PostCareerPosition(ctx, core.CareerPostingRequest{Binding: binding(storage.M2AgentBoPrincipal, "shared-http-org-post"),
		Posting: core.CareerPostingDefinition{PositionID: "shared_http_org_position", OrganizationID: org.Fact.RecordID,
			Title: "Assistant", OccupationID: "operations", Grade: "entry", Capacity: 1, DailyWageMinor: 12}})
	if err != nil {
		t.Fatal(err)
	}
	head = post.EventSequence
	application, err := s.ApplyForCareerPosition(ctx, core.CareerApplicationRequest{Binding: binding(storage.M2AgentAdaPrincipal, "shared-http-org-apply"),
		ApplicationID: "shared_http_org_application", PositionID: post.Fact.RecordID,
		CandidateID: storage.M2AgentAdaID, Statement: "I can help."})
	if err != nil {
		t.Fatal(err)
	}
	head = application.EventSequence
	invited, err := s.InviteCareerInterview(ctx, core.CareerInterviewRequest{Binding: binding(storage.M2AgentBoPrincipal, "shared-http-org-invite"),
		InterviewID: "shared_http_org_interview", ApplicationID: application.Fact.RecordID, Question: "How will you open safely?"})
	if err != nil {
		t.Fatal(err)
	}
	head = invited.EventSequence
	answered, err := s.AnswerCareerInterview(ctx, core.CareerInterviewAnswerRequest{Binding: binding(storage.M2AgentAdaPrincipal, "shared-http-org-answer"),
		InterviewID: "shared_http_org_interview", Answer: "Inspect equipment and exits before opening."})
	if err != nil {
		t.Fatal(err)
	}
	head = answered.EventSequence
	evaluated, err := s.EvaluateCareerApplication(ctx, core.CareerEvaluationRequest{Binding: binding(storage.M2AgentBoPrincipal, "shared-http-org-evaluate"),
		EvaluationID: "shared_http_org_evaluation", InterviewID: "shared_http_org_interview", Decision: "advance",
		Reason: "The recorded answer covers opening safety."})
	if err != nil {
		t.Fatal(err)
	}
	head = evaluated.EventSequence
	offered, err := s.OfferCareerEmployment(ctx, core.CareerOfferRequest{Binding: binding(storage.M2AgentBoPrincipal, "shared-http-org-offer"),
		OfferID: "shared_http_org_offer", EvaluationID: "shared_http_org_evaluation", StartsOnDay: 1,
		ProbationDays: 7, ExpiresAt: "2026-09-22T23:59:00Z"})
	if err != nil {
		t.Fatal(err)
	}
	head = offered.EventSequence
	accepted, err := s.AcceptCareerOffer(ctx, core.CareerOfferAcceptRequest{Binding: binding(storage.M2AgentAdaPrincipal, "shared-http-org-accept"),
		OfferID: offered.Fact.RecordID, AfterWorkPlaceID: storage.M2AgentCafeID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.RunAgentLife(ctx, "2026-09-23T08:00:00Z", 100); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	currentHead := func() int64 {
		var n int64
		if err := db.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, storage.M2DemoInstanceID, storage.M2DemoBranchID).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	head = currentHead()
	privateText := "Personal assessment and wage terms are confidential."
	layoff, err := s.EndCareerEmployment(ctx, core.CareerExitRequest{Binding: binding(storage.M2AgentBoPrincipal, "shared-http-org-layoff"),
		ContractID: accepted.Fact.Employment.ContractID, Kind: "layoff", EffectiveFromDay: 2, Notice: privateText})
	if err != nil {
		t.Fatal(err)
	}
	head = layoff.EventSequence
	const messageID = "shared-http-organization-message"
	published, err := s.PublishRPOrganizationNotice(ctx, storage.RPOrganizationNoticePublishRequest{Binding: binding(storage.M2AgentBoPrincipal, "shared-http-org-publish"),
		MessageID: messageID, CareerEventID: layoff.EventID, SpeakerID: storage.M2AgentBoID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO principals(principal_id,principal_type,display_name,status) VALUES (?,'service','Employee service','active')`, roundServiceAID); err != nil {
		t.Fatal(err)
	}
	operator := func(key string) core.CareerBinding {
		return core.CareerBinding{PrincipalID: "principal_operator", InstanceID: storage.M2DemoInstanceID,
			BranchID: storage.M2DemoBranchID, ExpectedHead: currentHead(), IdempotencyKey: key}
	}
	if _, err := s.EnrollRPExternalControllerLocal(ctx, storage.RPExternalControllerEnrollmentRequest{Binding: operator("shared-http-org-enroll"),
		EntityID: storage.M2AgentAdaID, ControllerPrincipalID: roundServiceAID, ControllerInstanceID: "shared-http-org-controller"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AssignRPExternalControllerLocal(ctx, storage.RPExternalControllerAssignmentRequest{Binding: operator("shared-http-org-assign"),
		EntityID: storage.M2AgentAdaID, ExpectedGeneration: 0}); err != nil {
		t.Fatal(err)
	}
	handler := newSharedRoundHTTPHandler(t, s)
	open := func(token, entity, key string) string {
		response := performJSON(t, handler, "/api/v1/rp/sessions/open", token, core.RPSessionOpenRequest{
			InstanceID: storage.M2DemoInstanceID, BranchID: storage.M2DemoBranchID,
			EntityID: entity, POV: "second_person", IdempotencyKey: key})
		assertStatus(t, response, http.StatusOK)
		return decodeData[storage.RPSession](t, response).SessionID
	}
	human := open(rpPlayerToken, storage.M2RPPlayerID, "shared-http-org-human")
	service := open(roundServiceAToken, storage.M2AgentAdaID, "shared-http-org-service")
	var at string
	for _, participant := range []struct{ token, session string }{{rpPlayerToken, human}, {roundServiceAToken, service}} {
		response := performJSON(t, handler, "/api/v1/rp/observe", participant.token, core.RPSessionReadRequest{SessionID: participant.session})
		assertStatus(t, response, http.StatusOK)
		at = decodeData[storage.RPObservation](t, response).WorldTime
	}
	response := performJSON(t, handler, "/api/v1/rp/information/organization/list", roundServiceAToken, core.RPSessionReadRequest{SessionID: service})
	assertStatus(t, response, http.StatusOK)
	if list := decodeData[storage.RPOrganizationNoticeList](t, response); len(list.Notices) != 1 || list.Notices[0].Accessed {
		t.Fatal("employee listing should expose only unread metadata", list)
	}
	response = performJSON(t, handler, "/api/v1/rp/rounds/open", operatorToken, storage.RPSharedRoundOpenRequest{
		Binding: operator("shared-http-org-round"), HumanSessionID: human, ExternalSessionIDs: []string{service}})
	assertStatus(t, response, http.StatusOK)
	round := decodeData[storage.RPSharedRound](t, response)
	baseline := currentHead()
	action := storage.RPSharedOrganizationNoticeAccessRequest{SessionID: service, RoundID: round.RoundID,
		MessageID: messageID, IdempotencyKey: "shared-http-org-read"}
	assertAPIError(t, performJSON(t, handler, "/api/v1/rp/rounds/information-organization-access", "", action), http.StatusUnauthorized, core.CodeUnauthenticated)
	assertAPIError(t, performJSON(t, handler, "/api/v1/rp/rounds/information-organization-access", rpPlayerToken, action), http.StatusNotFound, core.CodeNotFound)
	response = performJSON(t, handler, "/api/v1/rp/rounds/information-organization-access", roundServiceAToken, action)
	assertStatus(t, response, http.StatusOK)
	if pending := decodeData[storage.RPSharedRound](t, response); pending.Status != "open" || pending.Submitted != 1 || currentHead() != baseline {
		t.Fatal("HTTP organization proposal wrote Event", pending)
	}
	for _, secret := range []string{privateText, published.EventID, layoff.EventID, accepted.Fact.Employment.ContractID,
		storage.M2AgentAdaID, "source_event_id", "audience_terms_event_id"} {
		if strings.Contains(response.Body.String(), secret) {
			t.Fatal("shared organization proposal leaked private source", secret)
		}
	}
	advance := storage.RPSharedRoundAdvanceRequest{RPSharedRoundReadRequest: storage.RPSharedRoundReadRequest{SessionID: service, RoundID: round.RoundID}, Budget: 100}
	assertAPIError(t, performJSON(t, handler, "/api/v1/rp/rounds/advance", roundServiceAToken, advance), http.StatusConflict, core.CodeCommandInProgress)
	start, err := time.Parse(time.RFC3339Nano, at)
	if err != nil {
		t.Fatal(err)
	}
	response = performJSON(t, handler, "/api/v1/rp/rounds/wait", rpPlayerToken, storage.RPSharedWaitRequest{
		SessionID: human, RoundID: round.RoundID, HorizonWorldTime: start.Add(10 * time.Minute).Format(time.RFC3339Nano),
		IdempotencyKey: "shared-http-org-human-wait"})
	assertStatus(t, response, http.StatusOK)
	response = performJSON(t, handler, "/api/v1/rp/rounds/advance", roundServiceAToken, advance)
	assertStatus(t, response, http.StatusOK)
	if settled := decodeData[storage.RPSharedRound](t, response); settled.Status != "settled" || settled.OwnDisposition != "action_accepted" || settled.EventSequence != baseline+1 {
		t.Fatal("HTTP organization read not accepted", settled)
	}
	response = performJSON(t, handler, "/api/v1/rp/context/read", roundServiceAToken, storage.RPContextReadRequest{SessionID: service})
	assertStatus(t, response, http.StatusOK)
	found := false
	for _, fact := range decodeData[storage.RPClientContext](t, response).Facts {
		found = found || fact.MessageID == messageID && fact.Channel == "organization_announcement" &&
			fact.Reliability == "official_statement" && !strings.Contains(fact.Text, privateText)
	}
	if !found {
		t.Fatal("actual employee reader lacks redacted notice", response.Body.String())
	}
	response = performJSON(t, handler, "/api/v1/rp/context/read", rpPlayerToken, storage.RPContextReadRequest{SessionID: human})
	assertStatus(t, response, http.StatusOK)
	for _, fact := range decodeData[storage.RPClientContext](t, response).Facts {
		if fact.MessageID == messageID {
			t.Fatal("Human learned private employee claim")
		}
	}
	if diff, err := s.CompareProjections(ctx, storage.M2DemoInstanceID, storage.M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("HTTP shared organization source differed", diff, err)
	}
}
