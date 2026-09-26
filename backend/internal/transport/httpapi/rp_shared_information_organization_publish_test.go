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

func TestRPSharedOrganizationNoticePublishHTTPKeepsPrivateLayoffSource(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "shared-organization-publish-http.db")
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
	org, err := s.DefineCareerOrganization(ctx, core.CareerOrganizationRequest{Binding: binding("principal_creator", "org-pub-http-org"),
		Organization: core.CareerOrganizationDefinition{OrganizationID: "actor_m2_coop_employer", DisplayName: "Co-op",
			ManagerPrincipalID: storage.M2AgentBoPrincipal, WorkplaceID: "place_m2_work_ada"}})
	if err != nil {
		t.Fatal(err)
	}
	head = org.EventSequence
	post, err := s.PostCareerPosition(ctx, core.CareerPostingRequest{Binding: binding(storage.M2AgentBoPrincipal, "org-pub-http-post"),
		Posting: core.CareerPostingDefinition{PositionID: "org_pub_http_position", OrganizationID: org.Fact.RecordID,
			Title: "Assistant", OccupationID: "operations", Grade: "entry", Capacity: 1, DailyWageMinor: 12}})
	if err != nil {
		t.Fatal(err)
	}
	head = post.EventSequence
	application, err := s.ApplyForCareerPosition(ctx, core.CareerApplicationRequest{Binding: binding(storage.M2AgentAdaPrincipal, "org-pub-http-apply"),
		ApplicationID: "org_pub_http_application", PositionID: post.Fact.RecordID,
		CandidateID: storage.M2AgentAdaID, Statement: "I can help."})
	if err != nil {
		t.Fatal(err)
	}
	head = application.EventSequence
	invited, err := s.InviteCareerInterview(ctx, core.CareerInterviewRequest{Binding: binding(storage.M2AgentBoPrincipal, "org-pub-http-invite"),
		InterviewID: "org_pub_http_interview", ApplicationID: application.Fact.RecordID, Question: "How will you open safely?"})
	if err != nil {
		t.Fatal(err)
	}
	head = invited.EventSequence
	answered, err := s.AnswerCareerInterview(ctx, core.CareerInterviewAnswerRequest{Binding: binding(storage.M2AgentAdaPrincipal, "org-pub-http-answer"),
		InterviewID: "org_pub_http_interview", Answer: "Inspect equipment and exits before opening."})
	if err != nil {
		t.Fatal(err)
	}
	head = answered.EventSequence
	evaluated, err := s.EvaluateCareerApplication(ctx, core.CareerEvaluationRequest{Binding: binding(storage.M2AgentBoPrincipal, "org-pub-http-evaluate"),
		EvaluationID: "org_pub_http_evaluation", InterviewID: "org_pub_http_interview", Decision: "advance",
		Reason: "The recorded answer covers opening safety."})
	if err != nil {
		t.Fatal(err)
	}
	head = evaluated.EventSequence
	offered, err := s.OfferCareerEmployment(ctx, core.CareerOfferRequest{Binding: binding(storage.M2AgentBoPrincipal, "org-pub-http-offer"),
		OfferID: "org_pub_http_offer", EvaluationID: "org_pub_http_evaluation", StartsOnDay: 1,
		ProbationDays: 7, ExpiresAt: "2026-09-22T23:59:00Z"})
	if err != nil {
		t.Fatal(err)
	}
	head = offered.EventSequence
	accepted, err := s.AcceptCareerOffer(ctx, core.CareerOfferAcceptRequest{Binding: binding(storage.M2AgentAdaPrincipal, "org-pub-http-accept"),
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
	layoff, err := s.EndCareerEmployment(ctx, core.CareerExitRequest{Binding: binding(storage.M2AgentBoPrincipal, "org-pub-http-layoff"),
		ContractID: accepted.Fact.Employment.ContractID, Kind: "layoff", EffectiveFromDay: 2, Notice: privateText})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO principals(principal_id,principal_type,display_name,status) VALUES (?,'service','Manager service','active')`, roundServiceAID); err != nil {
		t.Fatal(err)
	}
	operator := func(key string) core.CareerBinding {
		return core.CareerBinding{PrincipalID: "principal_operator", InstanceID: storage.M2DemoInstanceID,
			BranchID: storage.M2DemoBranchID, ExpectedHead: currentHead(), IdempotencyKey: key}
	}
	if _, err := s.EnrollRPExternalControllerLocal(ctx, storage.RPExternalControllerEnrollmentRequest{Binding: operator("org-pub-http-enroll"),
		EntityID: storage.M2AgentBoID, ControllerPrincipalID: roundServiceAID, ControllerInstanceID: "org-pub-http-controller"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AssignRPExternalControllerLocal(ctx, storage.RPExternalControllerAssignmentRequest{Binding: operator("org-pub-http-assign"),
		EntityID: storage.M2AgentBoID, ExpectedGeneration: 0}); err != nil {
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
	human := open(rpPlayerToken, storage.M2RPPlayerID, "org-pub-http-human")
	service := open(roundServiceAToken, storage.M2AgentBoID, "org-pub-http-service")
	var at string
	for _, participant := range []struct{ token, session string }{{rpPlayerToken, human}, {roundServiceAToken, service}} {
		response := performJSON(t, handler, "/api/v1/rp/observe", participant.token, core.RPSessionReadRequest{SessionID: participant.session})
		assertStatus(t, response, http.StatusOK)
		at = decodeData[storage.RPObservation](t, response).WorldTime
	}
	sourceRead := storage.RPNoticePublicationSourceReadRequest{SessionID: service, Channel: "organization_announcement"}
	assertAPIError(t, performJSON(t, handler, "/api/v1/rp/information/publication/sources", rpPlayerToken, sourceRead), http.StatusNotFound, core.CodeNotFound)
	response := performJSON(t, handler, "/api/v1/rp/information/publication/sources", roundServiceAToken, sourceRead)
	assertStatus(t, response, http.StatusOK)
	sources := decodeData[storage.RPNoticePublicationSourceList](t, response)
	if len(sources.Sources) != 1 || sources.Sources[0].SourceHandle == "" || strings.Contains(response.Body.String(), layoff.EventID) ||
		strings.Contains(response.Body.String(), accepted.Fact.Employment.ContractID) || strings.Contains(response.Body.String(), privateText) ||
		strings.Contains(response.Body.String(), storage.M2AgentAdaID) {
		t.Fatal("HTTP Career source discovery leaked private evidence or lacked handle", response.Body.String())
	}
	response = performJSON(t, handler, "/api/v1/rp/rounds/open", operatorToken, storage.RPSharedRoundOpenRequest{
		Binding: operator("org-pub-http-round"), HumanSessionID: human, ExternalSessionIDs: []string{service}})
	assertStatus(t, response, http.StatusOK)
	round := decodeData[storage.RPSharedRound](t, response)
	baseline := currentHead()
	action := storage.RPSharedOrganizationNoticePublishRequest{SessionID: service, RoundID: round.RoundID,
		MessageID: "org-pub-http-day-two", SourceHandle: sources.Sources[0].SourceHandle, IdempotencyKey: "org-pub-http-submit"}
	assertAPIError(t, performJSON(t, handler, "/api/v1/rp/rounds/information-organization-publish", roundServiceAToken,
		map[string]any{"session_id": service, "round_id": round.RoundID, "message_id": "raw-career-forbidden",
			"career_event_id": layoff.EventID, "idempotency_key": "raw-career-forbidden"}), http.StatusBadRequest, core.CodeInvalidArgument)
	assertAPIError(t, performJSON(t, handler, "/api/v1/rp/rounds/information-organization-publish", "", action), http.StatusUnauthorized, core.CodeUnauthenticated)
	assertAPIError(t, performJSON(t, handler, "/api/v1/rp/rounds/information-organization-publish", rpPlayerToken, action), http.StatusNotFound, core.CodeNotFound)
	response = performJSON(t, handler, "/api/v1/rp/rounds/information-organization-publish", roundServiceAToken, action)
	assertStatus(t, response, http.StatusOK)
	if pending := decodeData[storage.RPSharedRound](t, response); pending.Status != "open" || pending.Submitted != 1 || currentHead() != baseline {
		t.Fatal("HTTP organization publication proposal wrote Event", pending)
	}
	for _, secret := range []string{layoff.EventID, accepted.Fact.Employment.ContractID, storage.M2AgentAdaID, privateText, "source_event_id"} {
		if strings.Contains(response.Body.String(), secret) {
			t.Fatal("organization publication receipt leaked private source", secret)
		}
	}
	start, err := time.Parse(time.RFC3339Nano, at)
	if err != nil {
		t.Fatal(err)
	}
	response = performJSON(t, handler, "/api/v1/rp/rounds/wait", rpPlayerToken, storage.RPSharedWaitRequest{
		SessionID: human, RoundID: round.RoundID, HorizonWorldTime: start.Add(10 * time.Minute).Format(time.RFC3339Nano),
		IdempotencyKey: "org-pub-http-human-wait"})
	assertStatus(t, response, http.StatusOK)
	response = performJSON(t, handler, "/api/v1/rp/rounds/advance", roundServiceAToken, storage.RPSharedRoundAdvanceRequest{
		RPSharedRoundReadRequest: storage.RPSharedRoundReadRequest{SessionID: service, RoundID: round.RoundID}, Budget: 100})
	assertStatus(t, response, http.StatusOK)
	if settled := decodeData[storage.RPSharedRound](t, response); settled.Status != "settled" || settled.OwnDisposition != "action_accepted" || settled.EventSequence != baseline+1 || currentHead() != baseline+1 {
		t.Fatal("HTTP shared organization publication not accepted", settled)
	}
	response = performJSON(t, handler, "/api/v1/rp/information/organization/list", rpPlayerToken, core.RPSessionReadRequest{SessionID: human})
	assertStatus(t, response, http.StatusOK)
	if list := decodeData[storage.RPOrganizationNoticeList](t, response); len(list.Notices) != 0 {
		t.Fatal("nonemployee saw private organization notice", list)
	}
	response = performJSON(t, handler, "/api/v1/rp/context/read", rpPlayerToken, storage.RPContextReadRequest{SessionID: human})
	assertStatus(t, response, http.StatusOK)
	for _, fact := range decodeData[storage.RPClientContext](t, response).Facts {
		if fact.MessageID == action.MessageID {
			t.Fatal("Human learned organization notice from publication alone")
		}
	}
	if diff, err := s.CompareProjections(ctx, storage.M2DemoInstanceID, storage.M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("HTTP shared organization publication source differs", diff, err)
	}
}
