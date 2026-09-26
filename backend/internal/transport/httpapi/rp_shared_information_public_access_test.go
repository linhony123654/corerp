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

func TestRPSharedPublicNoticeAccessHTTPAuthenticatedRecipient(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "shared-public-http.db")
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
	org, err := store.DefineCareerOrganization(ctx, core.CareerOrganizationRequest{Binding: binding("principal_creator", "shared-http-public-org"),
		Organization: core.CareerOrganizationDefinition{OrganizationID: "actor_m2_coop_employer", DisplayName: "Co-op",
			ManagerPrincipalID: storage.M2AgentBoPrincipal, WorkplaceID: "place_m2_work_ada"}})
	if err != nil {
		t.Fatal(err)
	}
	head = org.EventSequence
	territory, err := store.DefineRPCultureTerritory(ctx, storage.CultureTerritoryRequest{Binding: binding("principal_creator", "shared-http-public-territory"),
		ScopeKind: "world", ScopeID: storage.M2DemoInstanceID, StewardID: storage.M2AgentBoID})
	if err != nil {
		t.Fatal(err)
	}
	head = territory.EventSequence
	institution, err := store.DefineRPInstitution(ctx, storage.InstitutionDefinitionRequest{Binding: binding("principal_creator", "shared-http-public-council"),
		InstitutionID: "shared-http-public-council", ScopeKind: "world", ScopeID: storage.M2DemoInstanceID,
		TreasuryOrganizationID: org.Fact.OrganizationID, LegislatorID: storage.M2AgentAdaID,
		EnforcerID: storage.M2AgentBoID, ReviewerID: storage.M2RPNPCID})
	if err != nil {
		t.Fatal(err)
	}
	head = institution.EventSequence
	proposal, err := store.ProposeRPLaw(ctx, storage.LawProposalRequest{Binding: binding(storage.M2RPNPCPrincipal, "shared-http-public-propose"),
		InstitutionID: "shared-http-public-council", ProposerID: storage.M2RPNPCID,
		Law: storage.LawDefinition{LawID: "shared-http-quiet", ProhibitedAction: "speak", FineMinor: 2, Text: "Square must be quiet."}})
	if err != nil {
		t.Fatal(err)
	}
	head = proposal.EventSequence
	law, err := store.EnactRPLaw(ctx, storage.LawEnactmentRequest{Binding: binding(storage.M2AgentAdaPrincipal, "shared-http-public-enact"),
		InstitutionID: "shared-http-public-council", LegislatorID: storage.M2AgentAdaID,
		ProposalEventID: proposal.EventID, EffectiveWorldTime: storage.M2AgentNoonTime})
	if err != nil {
		t.Fatal(err)
	}
	head = law.EventSequence
	const messageID = "shared-http-public-message"
	published, err := store.PublishRPPublicNotice(ctx, storage.RPPublicNoticePublishRequest{Binding: binding(storage.M2AgentAdaPrincipal, "shared-http-public-publish"),
		MessageID: messageID, LawEventID: law.EventID, SpeakerID: storage.M2AgentAdaID})
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.ExecContext(ctx, `INSERT INTO principals(principal_id,principal_type,display_name,status) VALUES (?,'service','Public reader','active')`, roundServiceAID); err != nil {
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
	if _, err := store.EnrollRPExternalControllerLocal(ctx, storage.RPExternalControllerEnrollmentRequest{Binding: operator("shared-http-public-enroll"),
		EntityID: storage.M2RPNPCID, ControllerPrincipalID: roundServiceAID, ControllerInstanceID: "shared-http-public-controller"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AssignRPExternalControllerLocal(ctx, storage.RPExternalControllerAssignmentRequest{Binding: operator("shared-http-public-assign"),
		EntityID: storage.M2RPNPCID, ExpectedGeneration: 0}); err != nil {
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
	human := open(rpPlayerToken, storage.M2RPPlayerID, "shared-http-public-human")
	service := open(roundServiceAToken, storage.M2RPNPCID, "shared-http-public-service")
	var at string
	for _, participant := range []struct{ token, session string }{{rpPlayerToken, human}, {roundServiceAToken, service}} {
		response := performJSON(t, handler, "/api/v1/rp/observe", participant.token, core.RPSessionReadRequest{SessionID: participant.session})
		assertStatus(t, response, http.StatusOK)
		at = decodeData[storage.RPObservation](t, response).WorldTime
	}
	response := performJSON(t, handler, "/api/v1/rp/information/public/list", roundServiceAToken, core.RPSessionReadRequest{SessionID: service})
	assertStatus(t, response, http.StatusOK)
	if list := decodeData[storage.RPPublicNoticeList](t, response); len(list.Notices) != 1 || list.Notices[0].Accessed {
		t.Fatal("published board should only show metadata", list)
	}
	response = performJSON(t, handler, "/api/v1/rp/rounds/open", operatorToken, storage.RPSharedRoundOpenRequest{
		Binding: operator("shared-http-public-round"), HumanSessionID: human, ExternalSessionIDs: []string{service}})
	assertStatus(t, response, http.StatusOK)
	round := decodeData[storage.RPSharedRound](t, response)
	baseline := currentHead()
	action := storage.RPSharedPublicNoticeAccessRequest{SessionID: service, RoundID: round.RoundID,
		MessageID: messageID, IdempotencyKey: "shared-http-public-access"}
	assertAPIError(t, performJSON(t, handler, "/api/v1/rp/rounds/information-public-access", "", action), http.StatusUnauthorized, core.CodeUnauthenticated)
	assertAPIError(t, performJSON(t, handler, "/api/v1/rp/rounds/information-public-access", rpPlayerToken, action), http.StatusNotFound, core.CodeNotFound)
	response = performJSON(t, handler, "/api/v1/rp/rounds/information-public-access", roundServiceAToken, action)
	assertStatus(t, response, http.StatusOK)
	if pending := decodeData[storage.RPSharedRound](t, response); pending.Status != "open" || pending.Submitted != 1 || currentHead() != baseline {
		t.Fatal("HTTP public access proposal wrote Event", pending)
	}
	for _, secret := range []string{law.EventID, published.EventID, storage.M2RPNPCID, "Square must be quiet.", "source_event_id"} {
		if strings.Contains(response.Body.String(), secret) {
			t.Fatal("proposal receipt leaked notice source or reader", secret)
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
		IdempotencyKey: "shared-http-public-human-wait"})
	assertStatus(t, response, http.StatusOK)
	response = performJSON(t, handler, "/api/v1/rp/rounds/advance", roundServiceAToken, advance)
	assertStatus(t, response, http.StatusOK)
	if settled := decodeData[storage.RPSharedRound](t, response); settled.Status != "settled" || settled.OwnDisposition != "action_accepted" || settled.EventSequence != baseline+1 {
		t.Fatal("HTTP shared public access not accepted", settled)
	}
	if currentHead() != baseline+1 {
		t.Fatal("HTTP public access should write exactly one Event")
	}
	response = performJSON(t, handler, "/api/v1/rp/context/read", roundServiceAToken, storage.RPContextReadRequest{SessionID: service})
	assertStatus(t, response, http.StatusOK)
	found := false
	for _, fact := range decodeData[storage.RPClientContext](t, response).Facts {
		found = found || fact.MessageID == messageID && fact.Channel == "public_notice" && fact.Reliability == "official_statement"
	}
	if !found {
		t.Fatal("actual service reader lacks public notice", response.Body.String())
	}
	response = performJSON(t, handler, "/api/v1/rp/context/read", rpPlayerToken, storage.RPContextReadRequest{SessionID: human})
	assertStatus(t, response, http.StatusOK)
	for _, fact := range decodeData[storage.RPClientContext](t, response).Facts {
		if fact.MessageID == messageID {
			t.Fatal("Human learned another recipient's publication")
		}
	}
	if diff, err := store.CompareProjections(ctx, storage.M2DemoInstanceID, storage.M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("HTTP shared public access source differs", diff, err)
	}
}
