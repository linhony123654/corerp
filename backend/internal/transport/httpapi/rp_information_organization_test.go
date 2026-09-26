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

func TestRPInformationHTTPOrganizationNoticeFromRealCareerLayoff(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "organization-http.db")
	s, handler := openHTTPTestServer(t, ctx, path)
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
	org, err := s.DefineCareerOrganization(ctx, core.CareerOrganizationRequest{Binding: binding("principal_creator", "org-http-career"),
		Organization: core.CareerOrganizationDefinition{OrganizationID: "actor_m2_coop_employer", DisplayName: "Co-op",
			ManagerPrincipalID: storage.M2AgentBoPrincipal, WorkplaceID: "place_m2_work_ada"}})
	if err != nil {
		t.Fatal(err)
	}
	head = org.EventSequence
	post, err := s.PostCareerPosition(ctx, core.CareerPostingRequest{Binding: binding(storage.M2AgentBoPrincipal, "org-http-post"),
		Posting: core.CareerPostingDefinition{PositionID: "org_http_position", OrganizationID: org.Fact.RecordID,
			Title: "Assistant", OccupationID: "operations", Grade: "entry", Capacity: 1, DailyWageMinor: 12}})
	if err != nil {
		t.Fatal(err)
	}
	head = post.EventSequence
	application, err := s.ApplyForCareerPosition(ctx, core.CareerApplicationRequest{Binding: binding(storage.M2AgentAdaPrincipal, "org-http-apply"),
		ApplicationID: "org_http_application", PositionID: post.Fact.RecordID,
		CandidateID: storage.M2AgentAdaID, Statement: "I can help."})
	if err != nil {
		t.Fatal(err)
	}
	head = application.EventSequence
	invited, err := s.InviteCareerInterview(ctx, core.CareerInterviewRequest{Binding: binding(storage.M2AgentBoPrincipal, "org-http-invite"),
		InterviewID: "org_http_interview", ApplicationID: application.Fact.RecordID, Question: "How will you open safely?"})
	if err != nil {
		t.Fatal(err)
	}
	head = invited.EventSequence
	answered, err := s.AnswerCareerInterview(ctx, core.CareerInterviewAnswerRequest{Binding: binding(storage.M2AgentAdaPrincipal, "org-http-answer"),
		InterviewID: "org_http_interview", Answer: "Inspect equipment and exits before opening."})
	if err != nil {
		t.Fatal(err)
	}
	head = answered.EventSequence
	evaluated, err := s.EvaluateCareerApplication(ctx, core.CareerEvaluationRequest{Binding: binding(storage.M2AgentBoPrincipal, "org-http-evaluate"),
		EvaluationID: "org_http_evaluation", InterviewID: "org_http_interview", Decision: "advance",
		Reason: "The recorded answer covers opening safety."})
	if err != nil {
		t.Fatal(err)
	}
	head = evaluated.EventSequence
	offered, err := s.OfferCareerEmployment(ctx, core.CareerOfferRequest{Binding: binding(storage.M2AgentBoPrincipal, "org-http-offer"),
		OfferID: "org_http_offer", EvaluationID: "org_http_evaluation", StartsOnDay: 1,
		ProbationDays: 7, ExpiresAt: "2026-09-22T23:59:00Z"})
	if err != nil {
		t.Fatal(err)
	}
	head = offered.EventSequence
	accepted, err := s.AcceptCareerOffer(ctx, core.CareerOfferAcceptRequest{Binding: binding(storage.M2AgentAdaPrincipal, "org-http-accept"),
		OfferID: "org_http_offer", AfterWorkPlaceID: storage.M2AgentCafeID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.RunAgentLife(ctx, "2026-09-23T08:00:00Z", 100); err != nil {
		t.Fatal(err)
	}
	// EndCareerEmployment is the real private business source; the HTTP
	// publication must not echo its personal notice or contract terms.
	var currentHead int64
	fixture, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer fixture.Close()
	if err := fixture.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`,
		storage.M2DemoInstanceID, storage.M2DemoBranchID).Scan(&currentHead); err != nil {
		t.Fatal(err)
	}
	head = currentHead
	privateText := "Personal assessment and wage terms are confidential."
	layoff, err := s.EndCareerEmployment(ctx, core.CareerExitRequest{Binding: binding(storage.M2AgentBoPrincipal, "org-http-layoff"),
		ContractID: accepted.Fact.Employment.ContractID, Kind: "layoff", EffectiveFromDay: 2, Notice: privateText})
	if err != nil {
		t.Fatal(err)
	}
	publication := storage.RPOrganizationNoticePublishRequest{Binding: core.CareerBinding{InstanceID: storage.M2DemoInstanceID,
		BranchID: storage.M2DemoBranchID, ExpectedHead: layoff.EventSequence, IdempotencyKey: "org-http-publish"},
		MessageID: "org-http-layoff-notice", CareerEventID: layoff.EventID, SpeakerID: storage.M2AgentBoID}
	assertAPIError(t, performJSON(t, handler, "/api/v1/rp/information/organization/publish", "", publication), http.StatusUnauthorized, core.CodeUnauthenticated)
	assertAPIError(t, performJSON(t, handler, "/api/v1/rp/information/organization/publish", adaAgentToken, publication), http.StatusNotFound, core.CodeNotFound)
	response := performJSON(t, handler, "/api/v1/rp/information/organization/publish", boAgentToken, publication)
	assertStatus(t, response, http.StatusOK)
	published := decodeData[rpInformationNoticeReceipt](t, response)
	if published.MessageID != publication.MessageID || published.Replayed {
		t.Fatalf("organization publish receipt: %+v", published)
	}
	for _, secret := range []string{privateText, accepted.Fact.Employment.ContractID, storage.M2AgentAdaID,
		layoff.EventID, "career_event_id", "organization_id", "speaker_id"} {
		if strings.Contains(response.Body.String(), secret) {
			t.Fatalf("publication receipt exposed %q", secret)
		}
	}
	if _, err := fixture.ExecContext(ctx, `INSERT INTO capability_grants(grant_id,principal_id,capability_id,instance_id,branch_id,subject_id,field_scope,status,definition_event_id)
	 SELECT 'organization_http_ada_control',principal_id,capability_id,instance_id,branch_id,?,'[]','active',definition_event_id
	 FROM capability_grants WHERE grant_id='grant_m2_rp_player_control'`, storage.M2AgentAdaID); err != nil {
		t.Fatal(err)
	}
	response = performJSON(t, handler, "/api/v1/rp/sessions/open", rpPlayerToken, core.RPSessionOpenRequest{
		InstanceID: storage.M2DemoInstanceID, BranchID: storage.M2DemoBranchID, EntityID: storage.M2AgentAdaID,
		POV: "second_person", IdempotencyKey: "org-http-ada-session"})
	assertStatus(t, response, http.StatusOK)
	ada := decodeData[storage.RPSession](t, response)
	read := core.RPSessionReadRequest{SessionID: ada.SessionID}
	assertAPIError(t, performJSON(t, handler, "/api/v1/rp/information/organization/list", creatorToken, read), http.StatusNotFound, core.CodeNotFound)
	response = performJSON(t, handler, "/api/v1/rp/information/organization/list", rpPlayerToken, read)
	assertStatus(t, response, http.StatusOK)
	listed := decodeData[storage.RPOrganizationNoticeList](t, response)
	if len(listed.Notices) != 1 || listed.Notices[0].MessageID != publication.MessageID || listed.Notices[0].Accessed ||
		strings.Contains(response.Body.String(), privateText) || strings.Contains(response.Body.String(), layoff.EventID) {
		t.Fatalf("bounded organization listing: %+v", listed)
	}
	response = performJSON(t, handler, "/api/v1/rp/observe", rpPlayerToken, read)
	assertStatus(t, response, http.StatusOK)
	view := decodeData[storage.RPObservation](t, response)
	access := storage.RPOrganizationNoticeAccessRequest{Binding: core.CareerBinding{InstanceID: storage.M2DemoInstanceID,
		BranchID: storage.M2DemoBranchID, ExpectedHead: view.ObservationCursor, IdempotencyKey: "org-http-access"},
		SessionID: ada.SessionID, MessageID: publication.MessageID}
	assertAPIError(t, performJSON(t, handler, "/api/v1/rp/information/organization/access", boAgentToken, access), http.StatusNotFound, core.CodeNotFound)
	response = performJSON(t, handler, "/api/v1/rp/information/organization/access", rpPlayerToken, access)
	assertStatus(t, response, http.StatusOK)
	accessed := decodeData[rpInformationNoticeReceipt](t, response)
	if accessed.MessageID != publication.MessageID || accessed.Replayed ||
		strings.Contains(response.Body.String(), privateText) || strings.Contains(response.Body.String(), layoff.EventID) {
		t.Fatalf("bounded access receipt: %+v", accessed)
	}
	response = performJSON(t, handler, "/api/v1/rp/context/read", rpPlayerToken, storage.RPContextReadRequest{SessionID: ada.SessionID})
	assertStatus(t, response, http.StatusOK)
	contextView := decodeData[storage.RPClientContext](t, response)
	found := false
	for _, fact := range contextView.Facts {
		found = found || fact.MessageID == publication.MessageID && fact.Channel == "organization_announcement" &&
			fact.Reliability == "official_statement" && !strings.Contains(fact.Text, privateText)
	}
	if !found {
		t.Fatalf("actual HTTP recipient lacks redacted notice: %+v", contextView.Facts)
	}
	if diff, err := s.CompareProjections(ctx, storage.M2DemoInstanceID, storage.M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatalf("HTTP organization notice replay: %+v %v", diff, err)
	}
}
