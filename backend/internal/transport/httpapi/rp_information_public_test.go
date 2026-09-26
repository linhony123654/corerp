package httpapi

import (
	"context"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/storage"
)

func TestRPInformationHTTPPublicNoticeFromRealLaw(t *testing.T) {
	ctx := context.Background()
	s, handler := openHTTPTestServer(t, ctx, filepath.Join(t.TempDir(), "public-law-http.db"))
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
	org, err := s.DefineCareerOrganization(ctx, core.CareerOrganizationRequest{Binding: binding("principal_creator", "public-http-org"),
		Organization: core.CareerOrganizationDefinition{OrganizationID: "actor_m2_coop_employer", DisplayName: "Co-op",
			ManagerPrincipalID: storage.M2AgentBoPrincipal, WorkplaceID: "place_m2_work_ada"}})
	if err != nil {
		t.Fatal(err)
	}
	head = org.EventSequence
	territory, err := s.DefineRPCultureTerritory(ctx, storage.CultureTerritoryRequest{Binding: binding("principal_creator", "public-http-territory"),
		ScopeKind: "world", ScopeID: storage.M2DemoInstanceID, StewardID: storage.M2AgentBoID})
	if err != nil {
		t.Fatal(err)
	}
	head = territory.EventSequence
	institution, err := s.DefineRPInstitution(ctx, storage.InstitutionDefinitionRequest{Binding: binding("principal_creator", "public-http-council"),
		InstitutionID: "public-http-council", ScopeKind: "world", ScopeID: storage.M2DemoInstanceID,
		TreasuryOrganizationID: org.Fact.OrganizationID, LegislatorID: storage.M2AgentAdaID,
		EnforcerID: storage.M2AgentBoID, ReviewerID: storage.M2RPNPCID})
	if err != nil {
		t.Fatal(err)
	}
	head = institution.EventSequence
	proposal, err := s.ProposeRPLaw(ctx, storage.LawProposalRequest{Binding: binding(storage.M2RPNPCPrincipal, "public-http-propose"),
		InstitutionID: "public-http-council", ProposerID: storage.M2RPNPCID,
		Law: storage.LawDefinition{LawID: "quiet-square", ProhibitedAction: "speak", FineMinor: 2, Text: "Square must be quiet."}})
	if err != nil {
		t.Fatal(err)
	}
	head = proposal.EventSequence
	law, err := s.EnactRPLaw(ctx, storage.LawEnactmentRequest{Binding: binding(storage.M2AgentAdaPrincipal, "public-http-enact"),
		InstitutionID: "public-http-council", LegislatorID: storage.M2AgentAdaID,
		ProposalEventID: proposal.EventID, EffectiveWorldTime: storage.M2AgentNoonTime})
	if err != nil {
		t.Fatal(err)
	}
	head = law.EventSequence
	response := performJSON(t, handler, "/api/v1/rp/sessions/open", rpPlayerToken, core.RPSessionOpenRequest{
		InstanceID: storage.M2DemoInstanceID, BranchID: storage.M2DemoBranchID, EntityID: storage.M2RPPlayerID,
		POV: "second_person", IdempotencyKey: "public-http-lin-session"})
	assertStatus(t, response, http.StatusOK)
	session := decodeData[storage.RPSession](t, response)
	publication := storage.RPPublicNoticePublishRequest{Binding: core.CareerBinding{
		InstanceID: storage.M2DemoInstanceID, BranchID: storage.M2DemoBranchID,
		ExpectedHead: head, IdempotencyKey: "public-http-publish"},
		MessageID: "public-http-quiet-law", LawEventID: law.EventID, SpeakerID: storage.M2AgentAdaID}
	assertAPIError(t, performJSON(t, handler, "/api/v1/rp/information/public/publish", "", publication), http.StatusUnauthorized, core.CodeUnauthenticated)
	assertAPIError(t, performJSON(t, handler, "/api/v1/rp/information/public/publish", boAgentToken, publication), http.StatusNotFound, core.CodeNotFound)
	response = performJSON(t, handler, "/api/v1/rp/information/public/publish", adaAgentToken, publication)
	assertStatus(t, response, http.StatusOK)
	receipt := decodeData[rpInformationNoticeReceipt](t, response)
	if receipt.MessageID != publication.MessageID || receipt.Replayed || strings.Contains(response.Body.String(), law.EventID) ||
		strings.Contains(response.Body.String(), publication.SpeakerID) || strings.Contains(response.Body.String(), "Square must be quiet.") {
		t.Fatal("public publication receipt leaked source or content", response.Body.String())
	}
	response = performJSON(t, handler, "/api/v1/rp/information/public/list", rpPlayerToken, core.RPSessionReadRequest{SessionID: session.SessionID})
	assertStatus(t, response, http.StatusOK)
	list := decodeData[storage.RPPublicNoticeList](t, response)
	if len(list.Notices) != 1 || list.Notices[0].MessageID != publication.MessageID || list.Notices[0].Accessed ||
		strings.Contains(response.Body.String(), "Square must be quiet.") || strings.Contains(response.Body.String(), law.EventID) {
		t.Fatal("public discovery exposed law claim before access", response.Body.String())
	}
	response = performJSON(t, handler, "/api/v1/rp/context/read", rpPlayerToken, storage.RPContextReadRequest{SessionID: session.SessionID})
	assertStatus(t, response, http.StatusOK)
	if view := decodeData[storage.RPClientContext](t, response); len(view.Facts) != 0 {
		t.Fatal("publication became actor context without access", view.Facts)
	}
	response = performJSON(t, handler, "/api/v1/rp/observe", rpPlayerToken, core.RPSessionReadRequest{SessionID: session.SessionID})
	assertStatus(t, response, http.StatusOK)
	view := decodeData[storage.RPObservation](t, response)
	access := storage.RPPublicNoticeAccessRequest{Binding: core.CareerBinding{
		InstanceID: storage.M2DemoInstanceID, BranchID: storage.M2DemoBranchID,
		ExpectedHead: view.ObservationCursor, IdempotencyKey: "public-http-access"},
		SessionID: session.SessionID, MessageID: publication.MessageID}
	assertAPIError(t, performJSON(t, handler, "/api/v1/rp/information/public/access", boAgentToken, access), http.StatusNotFound, core.CodeNotFound)
	response = performJSON(t, handler, "/api/v1/rp/information/public/access", rpPlayerToken, access)
	assertStatus(t, response, http.StatusOK)
	if got := decodeData[rpInformationNoticeReceipt](t, response); got.MessageID != publication.MessageID || got.Replayed ||
		strings.Contains(response.Body.String(), law.EventID) || strings.Contains(response.Body.String(), "Square must be quiet.") {
		t.Fatal("bounded public access receipt", response.Body.String())
	}
	response = performJSON(t, handler, "/api/v1/rp/context/read", rpPlayerToken, storage.RPContextReadRequest{SessionID: session.SessionID})
	assertStatus(t, response, http.StatusOK)
	contextView := decodeData[storage.RPClientContext](t, response)
	found := false
	for _, fact := range contextView.Facts {
		found = found || fact.MessageID == publication.MessageID && fact.Channel == "public_notice" &&
			fact.Reliability == "official_statement" && strings.Contains(fact.Text, "Square must be quiet.")
	}
	if !found {
		t.Fatal("accessed law notice absent from HTTP context", contextView.Facts)
	}
	if diff, err := s.CompareProjections(ctx, storage.M2DemoInstanceID, storage.M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("HTTP public notice source differs", diff, err)
	}
}
