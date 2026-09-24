package httpapi

import (
	"context"
	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/storage"
	"net/http"
	"path/filepath"
	"testing"
)

func TestCultureHTTPAuthorshipTransmissionPrivacyAndRecovery(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "culture-http.db")
	s, handler := openHTTPTestServer(t, ctx, path)
	defer func() { s.Close() }()
	if _, err := s.PrepareRPLifeDemo(ctx); err != nil {
		t.Fatal(err)
	}
	run, err := s.RunAgentLife(ctx, storage.M2AgentNoonTime, 100)
	if err != nil {
		t.Fatal(err)
	}
	binding := core.CareerBinding{InstanceID: storage.M2DemoInstanceID, BranchID: storage.M2DemoBranchID, ExpectedHead: run.HeadSequence, IdempotencyKey: "definition"}
	define := storage.CultureDefinitionRequest{Binding: binding, AuthorID: storage.M2AgentBoID, Culture: core.RPCulture{CultureID: "custom", ScopeID: "custom", ScopeKind: "community", GroupIdentity: "sharing", Norms: []core.RPCultureNorm{{NormID: "gift", Action: "gift", Evaluation: 2}}}}
	response := performJSON(t, handler, "/api/v1/culture/define", "", define)
	assertAPIError(t, response, http.StatusUnauthorized, core.CodeUnauthenticated)
	response = performJSON(t, handler, "/api/v1/culture/define", adaAgentToken, define)
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)
	forged := define
	forged.Binding.PrincipalID = storage.M2AgentBoPrincipal
	response = performJSON(t, handler, "/api/v1/culture/define", adaAgentToken, forged)
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)
	response = performJSON(t, handler, "/api/v1/culture/define", boAgentToken, define)
	assertStatus(t, response, http.StatusOK)
	definition := decodeData[storage.CultureRecord](t, response)
	binding.ExpectedHead = definition.EventSequence
	binding.IdempotencyKey = "own-stance"
	own := storage.CultureStanceRequest{Binding: binding, EntityID: storage.M2AgentBoID, DefinitionEventID: definition.EventID, Stance: "partial"}
	stolen := own
	stolen.EntityID = storage.M2AgentAdaID
	response = performJSON(t, handler, "/api/v1/culture/internalize", adaAgentToken, stolen)
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)
	response = performJSON(t, handler, "/api/v1/culture/internalize", boAgentToken, own)
	assertStatus(t, response, http.StatusOK)
	authored := decodeData[storage.CultureRecord](t, response)
	if authored.Fact.Internalization.KnowledgeKind != "authored" || authored.Fact.Internalization.TransmissionEventID != "" {
		t.Fatal("invented self-hearing")
	}
	binding.ExpectedHead = authored.EventSequence
	binding.IdempotencyKey = "news"
	newsRequest := storage.CultureTransmissionRequest{Binding: binding, SpeakerID: storage.M2AgentBoID, DefinitionEventID: definition.EventID}
	response = performJSON(t, handler, "/api/v1/culture/transmit", boAgentToken, newsRequest)
	assertStatus(t, response, http.StatusOK)
	news := decodeData[storage.CultureRecord](t, response)
	binding.ExpectedHead = news.EventSequence
	binding.IdempotencyKey = "received-stance"
	received := storage.CultureStanceRequest{Binding: binding, EntityID: storage.M2AgentAdaID, TransmissionEventID: news.EventID, Stance: "oppose"}
	response = performJSON(t, handler, "/api/v1/culture/internalize", adaAgentToken, received)
	assertStatus(t, response, http.StatusOK)
	stance := decodeData[storage.CultureRecord](t, response)
	response = performJSON(t, handler, "/api/v1/culture/internalize", boAgentToken, received)
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, handler = openHTTPTestServer(t, ctx, path)
	response = performJSON(t, handler, "/api/v1/culture/internalize", adaAgentToken, received)
	assertStatus(t, response, http.StatusOK)
	retry := decodeData[storage.CultureRecord](t, response)
	if !retry.Replayed || retry.EventID != stance.EventID {
		t.Fatal("duplicate stance after reopen")
	}
	response = performJSON(t, handler, "/api/v1/culture/internalize", boAgentToken, own)
	assertStatus(t, response, http.StatusOK)
	retry = decodeData[storage.CultureRecord](t, response)
	if !retry.Replayed || retry.EventID != authored.EventID {
		t.Fatal("authored stance retry changed")
	}
	binding.ExpectedHead = stance.EventSequence
	binding.IdempotencyKey = "membership"
	join := storage.CultureAffiliationRequest{Binding: binding, EntityID: storage.M2AgentAdaID, DefinitionEventID: definition.EventID, TransmissionEventID: news.EventID, Decision: "join"}
	response = performJSON(t, handler, "/api/v1/culture/affiliate", boAgentToken, join)
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)
	response = performJSON(t, handler, "/api/v1/culture/affiliate", adaAgentToken, join)
	assertStatus(t, response, http.StatusOK)
	membership := decodeData[storage.CultureRecord](t, response)
	if membership.Fact.Affiliation.Status != "joined" || membership.Fact.Affiliation.EntityID != storage.M2AgentAdaID {
		t.Fatal("membership endpoint lost actor/status")
	}
	binding.ExpectedHead = membership.EventSequence
	binding.IdempotencyKey = "family-proposal"
	family := storage.CultureFamilyProposalRequest{Binding: binding, FamilyID: "chosen-family", ProposerID: storage.M2AgentBoID, InviteeID: storage.M2AgentAdaID}
	response = performJSON(t, handler, "/api/v1/culture/families/propose", boAgentToken, family)
	assertStatus(t, response, http.StatusOK)
	proposal := decodeData[storage.CultureRecord](t, response)
	binding.ExpectedHead = proposal.EventSequence
	binding.IdempotencyKey = "family-accept"
	accept := storage.CultureFamilyAcceptRequest{Binding: binding, EntityID: storage.M2AgentAdaID, ProposalEventID: proposal.EventID}
	response = performJSON(t, handler, "/api/v1/culture/families/accept", boAgentToken, accept)
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)
	response = performJSON(t, handler, "/api/v1/culture/families/accept", adaAgentToken, accept)
	assertStatus(t, response, http.StatusOK)
	accepted := decodeData[storage.CultureRecord](t, response)
	if accepted.Fact.Family.Status != "accepted" || accepted.Fact.Family.ProposalEventID != proposal.EventID {
		t.Fatal("family acceptance lost source")
	}
	binding.ExpectedHead = accepted.EventSequence
	binding.IdempotencyKey = "territory"
	territory := storage.CultureTerritoryRequest{Binding: binding, ScopeKind: "region", ScopeID: "cafe-region", StewardID: storage.M2AgentBoID, PlaceIDs: []string{storage.M2AgentCafeID}}
	response = performJSON(t, handler, "/api/v1/culture/territories/define", boAgentToken, territory)
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)
	response = performJSON(t, handler, "/api/v1/culture/territories/define", creatorToken, territory)
	assertStatus(t, response, http.StatusOK)
	region := decodeData[storage.CultureRecord](t, response)
	if region.Fact.Territory.ScopeID != "cafe-region" || region.Fact.Territory.StewardID != storage.M2AgentBoID {
		t.Fatal("territory route lost scope")
	}
	response = performJSON(t, handler, "/api/v1/culture/territories/define", creatorToken, territory)
	assertStatus(t, response, http.StatusOK)
	replayedRegion := decodeData[storage.CultureRecord](t, response)
	if !replayedRegion.Replayed || replayedRegion.EventID != region.EventID {
		t.Fatal("territory retry duplicated grant")
	}
	binding.ExpectedHead = region.EventSequence
	binding.IdempotencyKey = "revoke-culture"
	revoke := storage.CultureAuthorityRequest{Binding: binding, ScopeKind: "region", ScopeID: "cafe-region", Decision: "revoke"}
	response = performJSON(t, handler, "/api/v1/culture/territories/authority", boAgentToken, revoke)
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)
	response = performJSON(t, handler, "/api/v1/culture/territories/authority", creatorToken, revoke)
	assertStatus(t, response, http.StatusOK)
	revoked := decodeData[storage.CultureRecord](t, response)
	if revoked.Fact.Territory.GrantStatus != "revoked" {
		t.Fatal("authority endpoint failed to revoke")
	}
	revision := define
	revision.Binding.ExpectedHead = revoked.EventSequence
	revision.Binding.IdempotencyKey = "culture-revision"
	revision.PreviousDefinitionEventID = definition.EventID
	revision.Culture.Norms = []core.RPCultureNorm{{NormID: "gift", Action: "gift", Evaluation: -2}}
	response = performJSON(t, handler, "/api/v1/culture/define", boAgentToken, revision)
	assertStatus(t, response, http.StatusOK)
	revised := decodeData[storage.CultureRecord](t, response)
	if revised.Fact.PreviousDefinitionEventID != definition.EventID || revised.Fact.Definition.Norms[0].Evaluation != -2 {
		t.Fatal("culture revision route lost lineage")
	}
}
