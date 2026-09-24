package httpapi

import (
	"context"
	"net/http"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/storage"
)

func TestInstitutionHTTPAuthorizationEnactmentAndRevocation(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "institution-http.db")
	s, handler := openHTTPTestServer(t, ctx, path)
	defer func() { s.Close() }()
	setup, err := s.PrepareRPLifeDemo(ctx)
	if err != nil {
		t.Fatal(err)
	}
	query := storage.LawCasesRequest{InstanceID: storage.M2DemoInstanceID, BranchID: storage.M2DemoBranchID, ActorID: storage.M2AgentAdaID}
	response := performJSON(t, handler, "/api/v1/laws/cases/own", "", query)
	assertAPIError(t, response, http.StatusUnauthorized, core.CodeUnauthenticated)
	response = performJSON(t, handler, "/api/v1/laws/cases/own", boAgentToken, query)
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)
	response = performJSON(t, handler, "/api/v1/laws/cases/own", adaAgentToken, query)
	assertStatus(t, response, http.StatusOK)
	if cases := decodeData[[]core.RPLawCase](t, response); cases == nil || len(cases) != 0 {
		t.Fatalf("own empty cases %+v", cases)
	}
	forgedQuery := query
	forgedQuery.PrincipalID = storage.M2AgentAdaPrincipal
	response = performJSON(t, handler, "/api/v1/laws/cases/own", boAgentToken, forgedQuery)
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)
	for _, route := range []string{"institutions/define", "institutions/authority", "laws/propose", "laws/enact", "laws/announce", "laws/violations/record", "laws/enforce", "laws/dispute", "laws/disputes/forward", "laws/review"} {
		response := performJSON(t, handler, "/api/v1/"+route, "", map[string]any{})
		assertAPIError(t, response, http.StatusUnauthorized, core.CodeUnauthenticated)
		response = performJSON(t, handler, "/api/v1/"+route, adaAgentToken, map[string]any{"binding": map[string]any{"principal_id": storage.M2AgentBoPrincipal}})
		assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)
	}
	b := core.CareerBinding{InstanceID: storage.M2DemoInstanceID, BranchID: storage.M2DemoBranchID, ExpectedHead: setup.Routine.EventSequence, IdempotencyKey: "org"}
	org := core.CareerOrganizationRequest{Binding: b, Organization: core.CareerOrganizationDefinition{OrganizationID: "actor_m2_coop_employer", DisplayName: "Co-op", ManagerPrincipalID: storage.M2AgentBoPrincipal, WorkplaceID: "place_m2_work_ada"}}
	response = performJSON(t, handler, "/api/v1/career/organizations/define", creatorToken, org)
	assertStatus(t, response, http.StatusOK)
	defined := decodeData[storage.CareerRecord](t, response)
	b.ExpectedHead, b.IdempotencyKey = defined.EventSequence, "territory"
	response = performJSON(t, handler, "/api/v1/culture/territories/define", creatorToken, storage.CultureTerritoryRequest{Binding: b, ScopeKind: "world", ScopeID: storage.M2DemoInstanceID, StewardID: storage.M2AgentBoID})
	assertStatus(t, response, http.StatusOK)
	territory := decodeData[storage.CultureRecord](t, response)
	b.ExpectedHead, b.IdempotencyKey = territory.EventSequence, "institution"
	install := storage.InstitutionDefinitionRequest{Binding: b, InstitutionID: "council", ScopeKind: "world", ScopeID: storage.M2DemoInstanceID, TreasuryOrganizationID: org.Organization.OrganizationID, LegislatorID: storage.M2AgentAdaID, EnforcerID: storage.M2AgentBoID, ReviewerID: storage.M2RPNPCID}
	response = performJSON(t, handler, "/api/v1/institutions/define", boAgentToken, install)
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)
	response = performJSON(t, handler, "/api/v1/institutions/define", creatorToken, install)
	assertStatus(t, response, http.StatusOK)
	installed := decodeData[storage.InstitutionRecord](t, response)
	b.ExpectedHead, b.IdempotencyKey = installed.EventSequence, "proposal"
	p := storage.LawProposalRequest{Binding: b, InstitutionID: "council", ProposerID: storage.M2AgentBoID, Law: storage.LawDefinition{LawID: "quiet", ProhibitedAction: "speak", FineMinor: 2, Text: "Quiet rule."}}
	response = performJSON(t, handler, "/api/v1/laws/propose", adaAgentToken, p)
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)
	response = performJSON(t, handler, "/api/v1/laws/propose", boAgentToken, p)
	assertStatus(t, response, http.StatusOK)
	proposed := decodeData[storage.InstitutionRecord](t, response)
	b.ExpectedHead, b.IdempotencyKey = proposed.EventSequence, "enact"
	enact := storage.LawEnactmentRequest{Binding: b, InstitutionID: "council", LegislatorID: storage.M2AgentAdaID, ProposalEventID: proposed.EventID, EffectiveWorldTime: storage.M2AgentNoonTime}
	response = performJSON(t, handler, "/api/v1/laws/enact", boAgentToken, enact)
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)
	response = performJSON(t, handler, "/api/v1/laws/enact", adaAgentToken, enact)
	assertStatus(t, response, http.StatusOK)
	enacted := decodeData[storage.InstitutionRecord](t, response)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, handler = openHTTPTestServer(t, ctx, path)
	response = performJSON(t, handler, "/api/v1/laws/enact", adaAgentToken, enact)
	assertStatus(t, response, http.StatusOK)
	retry := decodeData[storage.InstitutionRecord](t, response)
	if !retry.Replayed || retry.EventID != enacted.EventID {
		t.Fatal("HTTP enact retry changed event")
	}
	b.ExpectedHead, b.IdempotencyKey = enacted.EventSequence, "announce"
	news := storage.LawAnnouncementRequest{Binding: b, SpeakerID: storage.M2AgentBoID, EnactmentEventID: enacted.EventID}
	response = performJSON(t, handler, "/api/v1/laws/announce", boAgentToken, news)
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)
	news.SpeakerID = storage.M2AgentAdaID
	response = performJSON(t, handler, "/api/v1/laws/announce", adaAgentToken, news)
	assertStatus(t, response, http.StatusOK)
	announced := decodeData[storage.InstitutionRecord](t, response)
	b.ExpectedHead, b.IdempotencyKey = announced.EventSequence, "revoke"
	revoke := storage.InstitutionAuthorityRequest{Binding: b, InstitutionID: "council", CapabilityID: "world.institution.legislate", Decision: "revoke"}
	response = performJSON(t, handler, "/api/v1/institutions/authority", adaAgentToken, revoke)
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)
	response = performJSON(t, handler, "/api/v1/institutions/authority", creatorToken, revoke)
	assertStatus(t, response, http.StatusOK)
	response = performJSON(t, handler, "/api/v1/laws/enact", adaAgentToken, enact)
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)
	// Continue through a real player act and the complete private case protocol.
	run, err := s.RunAgentLife(ctx, storage.M2AgentNoonTime, 100)
	if err != nil {
		t.Fatal(err)
	}
	b.ExpectedHead, b.IdempotencyKey = run.HeadSequence, "public-law-news"
	news.Binding = b
	response = performJSON(t, handler, "/api/v1/laws/announce", adaAgentToken, news)
	assertStatus(t, response, http.StatusOK)
	response = performJSON(t, handler, "/api/v1/rp/sessions/open", rpPlayerToken, core.RPSessionOpenRequest{InstanceID: b.InstanceID, BranchID: b.BranchID, EntityID: storage.M2RPPlayerID, POV: "second_person", IdempotencyKey: "law-player"})
	assertStatus(t, response, http.StatusOK)
	session := decodeData[storage.RPSession](t, response)
	response = performJSON(t, handler, "/api/v1/rp/observe", rpPlayerToken, core.RPSessionReadRequest{SessionID: session.SessionID})
	assertStatus(t, response, http.StatusOK)
	view := decodeData[storage.RPObservation](t, response)
	response = performJSON(t, handler, "/api/v1/rp/turns/run", rpPlayerToken, core.RPSpeechRequest{SessionID: session.SessionID, Text: "我选择在这里说话。", ExpectedCursor: view.ObservationCursor, IdempotencyKey: "law-act"})
	assertStatus(t, response, http.StatusOK)
	turn := decodeData[storage.RPTurnResult](t, response)
	if turn.Status != "settled" {
		t.Fatalf("unsettled law act %+v", turn)
	}
	b.ExpectedHead, b.IdempotencyKey = turn.SettledSequence, "violation"
	violationRequest := storage.LawViolationRequest{Binding: b, InstitutionID: "council", EnforcerID: storage.M2AgentBoID, EnactmentEventID: enacted.EventID, ActionEventID: turn.PlayerEventID}
	response = performJSON(t, handler, "/api/v1/laws/violations/record", adaAgentToken, violationRequest)
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)
	response = performJSON(t, handler, "/api/v1/laws/violations/record", boAgentToken, violationRequest)
	assertStatus(t, response, http.StatusOK)
	violation := decodeData[storage.InstitutionRecord](t, response)
	b.ExpectedHead, b.IdempotencyKey = violation.EventSequence, "fine"
	enforce := storage.LawEnforcementRequest{Binding: b, InstitutionID: "council", EnforcerID: storage.M2AgentBoID, ViolationEventID: violation.EventID}
	response = performJSON(t, handler, "/api/v1/laws/enforce", boAgentToken, enforce)
	assertStatus(t, response, http.StatusOK)
	fine := decodeData[storage.InstitutionRecord](t, response)
	query.ActorID = storage.M2RPPlayerID
	response = performJSON(t, handler, "/api/v1/laws/cases/own", boAgentToken, query)
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)
	response = performJSON(t, handler, "/api/v1/laws/cases/own", rpPlayerToken, query)
	assertStatus(t, response, http.StatusOK)
	cases := decodeData[[]core.RPLawCase](t, response)
	if len(cases) != 1 || cases[0].EnforcementEventID != fine.EventID || cases[0].FineMinor != 2 || cases[0].Status != "enforced" {
		t.Fatalf("HTTP case receipt %+v", cases)
	}
	b.ExpectedHead, b.IdempotencyKey = fine.EventSequence, "dispute"
	disputeRequest := storage.LawDisputeRequest{Binding: b, InstitutionID: "council", ActorID: storage.M2RPPlayerID, EnforcementEventID: fine.EventID, Statement: "Please reconsider this first incident."}
	response = performJSON(t, handler, "/api/v1/laws/dispute", boAgentToken, disputeRequest)
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)
	response = performJSON(t, handler, "/api/v1/laws/dispute", rpPlayerToken, disputeRequest)
	assertStatus(t, response, http.StatusOK)
	dispute := decodeData[storage.InstitutionRecord](t, response)
	b.ExpectedHead, b.IdempotencyKey = dispute.EventSequence, "appoint-reviewer"
	response = performJSON(t, handler, "/api/v1/institutions/authority", creatorToken, storage.InstitutionAuthorityRequest{Binding: b, InstitutionID: "council", CapabilityID: "world.institution.review", Decision: "appoint", EntityID: storage.M2AgentAdaID})
	assertStatus(t, response, http.StatusOK)
	appointed := decodeData[storage.InstitutionRecord](t, response)
	b.ExpectedHead, b.IdempotencyKey = appointed.EventSequence, "review"
	review := storage.LawReviewRequest{Binding: b, InstitutionID: "council", ReviewerID: storage.M2AgentAdaID, DisputeEventID: dispute.EventID, Decision: "reverse", Reason: "Discretionary first-incident reversal."}
	response = performJSON(t, handler, "/api/v1/laws/review", adaAgentToken, review)
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)
	b.IdempotencyKey = "forward"
	forward := storage.LawDisputeForwardRequest{Binding: b, InstitutionID: "council", ActorID: storage.M2RPPlayerID, DisputeEventID: dispute.EventID}
	response = performJSON(t, handler, "/api/v1/laws/disputes/forward", rpPlayerToken, forward)
	assertStatus(t, response, http.StatusOK)
	sent := decodeData[storage.InstitutionRecord](t, response)
	review.Binding.ExpectedHead = sent.EventSequence
	response = performJSON(t, handler, "/api/v1/laws/review", adaAgentToken, review)
	assertStatus(t, response, http.StatusOK)
	reviewed := decodeData[storage.InstitutionRecord](t, response)
	if reviewed.Fact.Review.RefundedMinor != 2 || reviewed.Fact.Review.ReceiptEventID != sent.EventID {
		t.Fatalf("HTTP reversal provenance %+v", reviewed)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, handler = openHTTPTestServer(t, ctx, path)
	response = performJSON(t, handler, "/api/v1/laws/review", adaAgentToken, review)
	assertStatus(t, response, http.StatusOK)
	replayed := decodeData[storage.InstitutionRecord](t, response)
	if !replayed.Replayed || replayed.EventID != reviewed.EventID {
		t.Fatal("HTTP review retry duplicated effect")
	}
	response = performJSON(t, handler, "/api/v1/laws/cases/own", rpPlayerToken, query)
	assertStatus(t, response, http.StatusOK)
	cases = decodeData[[]core.RPLawCase](t, response)
	if len(cases) != 1 || cases[0].ReviewEventID != reviewed.EventID || cases[0].RefundedMinor != 2 || cases[0].Status != "reverse" {
		t.Fatalf("recovered HTTP case %+v", cases)
	}
}
