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

func TestRPSharedPublicNoticePublishHTTPBindsControllerAndKeepsSourcePrivate(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "shared-public-publish-http.db")
	store, _ := openHTTPTestServer(t, ctx, path)
	defer store.Close()
	setup, err := store.PrepareRPLifeDemo(ctx)
	if err != nil {
		t.Fatal(err)
	}
	head := setup.Routine.EventSequence
	binding := func(principal, key string) core.CareerBinding {
		return core.CareerBinding{PrincipalID: principal, InstanceID: storage.M2DemoInstanceID,
			BranchID: storage.M2DemoBranchID, ExpectedHead: head, IdempotencyKey: key}
	}
	org, err := store.DefineCareerOrganization(ctx, core.CareerOrganizationRequest{Binding: binding("principal_creator", "pub-share-http-org"),
		Organization: core.CareerOrganizationDefinition{OrganizationID: "actor_m2_coop_employer", DisplayName: "Co-op",
			ManagerPrincipalID: storage.M2AgentBoPrincipal, WorkplaceID: "place_m2_work_ada"}})
	if err != nil {
		t.Fatal(err)
	}
	head = org.EventSequence
	territory, err := store.DefineRPCultureTerritory(ctx, storage.CultureTerritoryRequest{Binding: binding("principal_creator", "pub-share-http-territory"),
		ScopeKind: "world", ScopeID: storage.M2DemoInstanceID, StewardID: storage.M2AgentBoID})
	if err != nil {
		t.Fatal(err)
	}
	head = territory.EventSequence
	institution, err := store.DefineRPInstitution(ctx, storage.InstitutionDefinitionRequest{Binding: binding("principal_creator", "pub-share-http-council"),
		InstitutionID: "pub-share-http-council", ScopeKind: "world", ScopeID: storage.M2DemoInstanceID,
		TreasuryOrganizationID: org.Fact.OrganizationID, LegislatorID: storage.M2AgentAdaID,
		EnforcerID: storage.M2AgentBoID, ReviewerID: storage.M2RPNPCID})
	if err != nil {
		t.Fatal(err)
	}
	head = institution.EventSequence
	proposal, err := store.ProposeRPLaw(ctx, storage.LawProposalRequest{Binding: binding(storage.M2RPNPCPrincipal, "pub-share-http-propose"),
		InstitutionID: "pub-share-http-council", ProposerID: storage.M2RPNPCID,
		Law: storage.LawDefinition{LawID: "pub-share-http-quiet", ProhibitedAction: "speak", FineMinor: 2, Text: "Square must be quiet."}})
	if err != nil {
		t.Fatal(err)
	}
	head = proposal.EventSequence
	law, err := store.EnactRPLaw(ctx, storage.LawEnactmentRequest{Binding: binding(storage.M2AgentAdaPrincipal, "pub-share-http-enact"),
		InstitutionID: "pub-share-http-council", LegislatorID: storage.M2AgentAdaID,
		ProposalEventID: proposal.EventID, EffectiveWorldTime: storage.M2AgentNoonTime})
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.ExecContext(ctx, `INSERT INTO principals(principal_id,principal_type,display_name,status) VALUES (?,'service','Law publisher','active')`, roundServiceAID); err != nil {
		t.Fatal(err)
	}
	currentHead := func() int64 {
		var n int64
		if err := db.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, storage.M2DemoInstanceID, storage.M2DemoBranchID).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	operator := func(key string) core.CareerBinding {
		return core.CareerBinding{PrincipalID: "principal_operator", InstanceID: storage.M2DemoInstanceID,
			BranchID: storage.M2DemoBranchID, ExpectedHead: currentHead(), IdempotencyKey: key}
	}
	if _, err := store.EnrollRPExternalControllerLocal(ctx, storage.RPExternalControllerEnrollmentRequest{Binding: operator("pub-share-http-enroll"),
		EntityID: storage.M2AgentAdaID, ControllerPrincipalID: roundServiceAID, ControllerInstanceID: "pub-share-http-controller"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AssignRPExternalControllerLocal(ctx, storage.RPExternalControllerAssignmentRequest{Binding: operator("pub-share-http-assign"),
		EntityID: storage.M2AgentAdaID, ExpectedGeneration: 0}); err != nil {
		t.Fatal(err)
	}
	handler := newSharedRoundHTTPHandler(t, store)
	open := func(token, entity, key string) string {
		response := performJSON(t, handler, "/api/v1/rp/sessions/open", token, core.RPSessionOpenRequest{
			InstanceID: storage.M2DemoInstanceID, BranchID: storage.M2DemoBranchID,
			EntityID: entity, POV: "second_person", IdempotencyKey: key})
		assertStatus(t, response, http.StatusOK)
		return decodeData[storage.RPSession](t, response).SessionID
	}
	human := open(rpPlayerToken, storage.M2RPPlayerID, "pub-share-http-human")
	service := open(roundServiceAToken, storage.M2AgentAdaID, "pub-share-http-service")
	var at string
	for _, participant := range []struct{ token, session string }{{rpPlayerToken, human}, {roundServiceAToken, service}} {
		response := performJSON(t, handler, "/api/v1/rp/observe", participant.token, core.RPSessionReadRequest{SessionID: participant.session})
		assertStatus(t, response, http.StatusOK)
		at = decodeData[storage.RPObservation](t, response).WorldTime
	}
	sourceRead := storage.RPNoticePublicationSourceReadRequest{SessionID: service, Channel: "public_notice"}
	assertAPIError(t, performJSON(t, handler, "/api/v1/rp/information/publication/sources", rpPlayerToken, sourceRead), http.StatusNotFound, core.CodeNotFound)
	response := performJSON(t, handler, "/api/v1/rp/information/publication/sources", roundServiceAToken, sourceRead)
	assertStatus(t, response, http.StatusOK)
	sources := decodeData[storage.RPNoticePublicationSourceList](t, response)
	if len(sources.Sources) != 1 || sources.Sources[0].SourceHandle == "" || strings.Contains(response.Body.String(), law.EventID) ||
		strings.Contains(response.Body.String(), storage.M2AgentAdaPrincipal) {
		t.Fatal("HTTP law source discovery leaked raw evidence or lacked handle", response.Body.String())
	}
	response = performJSON(t, handler, "/api/v1/rp/rounds/open", operatorToken, storage.RPSharedRoundOpenRequest{
		Binding: operator("pub-share-http-round"), HumanSessionID: human, ExternalSessionIDs: []string{service}})
	assertStatus(t, response, http.StatusOK)
	round := decodeData[storage.RPSharedRound](t, response)
	baseline := currentHead()
	action := storage.RPSharedPublicNoticePublishRequest{SessionID: service, RoundID: round.RoundID,
		MessageID: "pub-share-http-message", SourceHandle: sources.Sources[0].SourceHandle, IdempotencyKey: "pub-share-http-submit"}
	assertAPIError(t, performJSON(t, handler, "/api/v1/rp/rounds/information-public-publish", roundServiceAToken,
		map[string]any{"session_id": service, "round_id": round.RoundID, "message_id": "raw-law-forbidden",
			"law_event_id": law.EventID, "idempotency_key": "raw-law-forbidden"}), http.StatusBadRequest, core.CodeInvalidArgument)
	assertAPIError(t, performJSON(t, handler, "/api/v1/rp/rounds/information-public-publish", "", action), http.StatusUnauthorized, core.CodeUnauthenticated)
	assertAPIError(t, performJSON(t, handler, "/api/v1/rp/rounds/information-public-publish", rpPlayerToken, action), http.StatusNotFound, core.CodeNotFound)
	response = performJSON(t, handler, "/api/v1/rp/rounds/information-public-publish", roundServiceAToken, action)
	assertStatus(t, response, http.StatusOK)
	if pending := decodeData[storage.RPSharedRound](t, response); pending.Status != "open" || pending.Submitted != 1 || currentHead() != baseline {
		t.Fatal("HTTP publication proposal wrote Event", pending)
	}
	for _, secret := range []string{law.EventID, storage.M2AgentAdaPrincipal, storage.M2AgentAdaID, "Square must be quiet.", "source_event_id"} {
		if strings.Contains(response.Body.String(), secret) {
			t.Fatal("publication proposal receipt leaked source", secret)
		}
	}
	start, err := time.Parse(time.RFC3339Nano, at)
	if err != nil {
		t.Fatal(err)
	}
	response = performJSON(t, handler, "/api/v1/rp/rounds/wait", rpPlayerToken, storage.RPSharedWaitRequest{
		SessionID: human, RoundID: round.RoundID, HorizonWorldTime: start.Add(10 * time.Minute).Format(time.RFC3339Nano),
		IdempotencyKey: "pub-share-http-human-wait"})
	assertStatus(t, response, http.StatusOK)
	response = performJSON(t, handler, "/api/v1/rp/rounds/advance", roundServiceAToken, storage.RPSharedRoundAdvanceRequest{
		RPSharedRoundReadRequest: storage.RPSharedRoundReadRequest{SessionID: service, RoundID: round.RoundID}, Budget: 100})
	assertStatus(t, response, http.StatusOK)
	if settled := decodeData[storage.RPSharedRound](t, response); settled.Status != "settled" || settled.OwnDisposition != "action_accepted" || settled.EventSequence != baseline+1 || currentHead() != baseline+1 {
		t.Fatal("HTTP shared public publication not accepted", settled)
	}
	response = performJSON(t, handler, "/api/v1/rp/information/public/list", rpPlayerToken, core.RPSessionReadRequest{SessionID: human})
	assertStatus(t, response, http.StatusOK)
	if list := decodeData[storage.RPPublicNoticeList](t, response); len(list.Notices) != 1 || list.Notices[0].MessageID != action.MessageID || list.Notices[0].Accessed {
		t.Fatal("published public notice should be discoverable but not learned", list)
	}
	response = performJSON(t, handler, "/api/v1/rp/context/read", rpPlayerToken, storage.RPContextReadRequest{SessionID: human})
	assertStatus(t, response, http.StatusOK)
	for _, fact := range decodeData[storage.RPClientContext](t, response).Facts {
		if fact.MessageID == action.MessageID {
			t.Fatal("Human learned public notice from publication alone")
		}
	}
	if diff, err := store.CompareProjections(ctx, storage.M2DemoInstanceID, storage.M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("HTTP shared public publication source differs", diff, err)
	}
}
