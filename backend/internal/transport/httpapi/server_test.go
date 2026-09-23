package httpapi

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/storage"
)

const (
	buyerToken    = "token-buyer"
	creatorToken  = "token-creator"
	operatorToken = "token-operator"
	adaAgentToken = "token-agent-ada"
	boAgentToken  = "token-agent-bo"
)

func TestHealthReadinessAndStrictRequestBoundary(t *testing.T) {
	ctx := context.Background()
	store, handler := openHTTPTestServer(t, ctx, filepath.Join(t.TempDir(), "boundary.db"))

	response := performRequest(t, handler, http.MethodGet, "/healthz", "", nil)
	assertStatus(t, response, http.StatusOK)
	assertSecurityHeaders(t, response)
	response = performRequest(t, handler, http.MethodGet, "/readyz", "", nil)
	assertStatus(t, response, http.StatusOK)

	command := demoHTTPPurchase()
	response = performJSON(t, handler, "/api/v1/commands/purchase", "", command)
	assertAPIError(t, response, http.StatusUnauthorized, core.CodeUnauthenticated)
	response = performJSON(t, handler, "/api/v1/commands/purchase", "wrong-token", command)
	assertAPIError(t, response, http.StatusUnauthorized, core.CodeUnauthenticated)

	command.PrincipalID = "principal_creator"
	response = performJSON(t, handler, "/api/v1/commands/purchase", buyerToken, command)
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)
	command.PrincipalID = ""

	response = performRequest(t, handler, http.MethodGet, "/api/v1/commands/purchase", buyerToken, nil)
	assertStatus(t, response, http.StatusMethodNotAllowed)
	if response.Header().Get("Allow") != http.MethodPost {
		t.Fatalf("method response lacks Allow: %q", response.Header().Get("Allow"))
	}

	response = performRawJSON(t, handler, "/api/v1/commands/purchase", buyerToken, `{"unknown":true}`)
	assertAPIError(t, response, http.StatusBadRequest, core.CodeInvalidArgument)
	response = performRawJSON(t, handler, "/api/v1/commands/purchase", buyerToken, `{}`+` {}`)
	assertAPIError(t, response, http.StatusBadRequest, core.CodeInvalidArgument)
	response = performRequest(t, handler, http.MethodPost, "/api/v1/commands/purchase", buyerToken, strings.NewReader(`{}`))
	assertAPIError(t, response, http.StatusBadRequest, core.CodeInvalidArgument)
	oversized := `{"actor_id":"` + strings.Repeat("x", int(defaultMaxBodyBytes)) + `"}`
	response = performRawJSON(t, handler, "/api/v1/commands/purchase", buyerToken, oversized)
	assertAPIError(t, response, http.StatusRequestEntityTooLarge, core.CodeInvalidArgument)

	state, err := store.ReadDemoState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if state.HeadSequence != 1 || state.Commands != 1 || state.Events != 1 {
		t.Fatalf("rejected HTTP requests changed authority: %+v", state)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	response = performRequest(t, handler, http.MethodGet, "/readyz", "", nil)
	assertStatus(t, response, http.StatusServiceUnavailable)
}

func TestPurchaseHTTPPersistsAndReplaysIdempotently(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "purchase-http.db")
	store, handler := openHTTPTestServer(t, ctx, path)
	command := demoHTTPPurchase()

	response := performJSON(t, handler, "/api/v1/commands/purchase", buyerToken, command)
	assertStatus(t, response, http.StatusOK)
	first := decodeData[core.PurchaseResult](t, response)
	if first.Replayed || first.FirstSequence != 2 {
		t.Fatalf("unexpected first HTTP purchase: %+v", first)
	}

	retry := command
	retry.CommandID = "cmd_http_purchase_retry"
	response = performJSON(t, handler, "/api/v1/commands/purchase", buyerToken, retry)
	assertStatus(t, response, http.StatusOK)
	replayed := decodeData[core.PurchaseResult](t, response)
	if !replayed.Replayed || replayed.CommandID != command.CommandID || replayed.EventID != first.EventID {
		t.Fatalf("HTTP retry did not return original result: %+v", replayed)
	}

	mismatch := retry
	mismatch.QuantityMinor = 3
	response = performJSON(t, handler, "/api/v1/commands/purchase", buyerToken, mismatch)
	assertAPIError(t, response, http.StatusConflict, core.CodeIdempotencyMismatch)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := storage.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	state, err := reopened.ReadDemoState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if state.HeadSequence != 2 || state.Events != 2 || state.Commands != 2 || state.Postings != 7 || state.StockMovements != 2 {
		t.Fatalf("HTTP purchase did not persist one complete write set: %+v", state)
	}
}

func TestAuthorizedControlAndPrivateQueryRoutes(t *testing.T) {
	ctx := context.Background()
	store, handler := openHTTPTestServer(t, ctx, filepath.Join(t.TempDir(), "control-http.db"))
	defer store.Close()

	simulation := core.StrictSimulationRequest{
		CapabilityID: "world.simulate", InstanceID: storage.DemoInstanceID,
		BranchID: storage.DemoBranchID, TargetDay: 65, Budget: 1000,
	}
	response := performJSON(t, handler, "/api/v1/simulations/strict", buyerToken, simulation)
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)
	response = performJSON(t, handler, "/api/v1/simulations/strict", creatorToken, simulation)
	assertStatus(t, response, http.StatusOK)
	run := decodeData[storage.StrictRunResult](t, response)
	if run.Status != "completed" || run.TargetDay != 65 || run.ProcessedItems == 0 {
		t.Fatalf("unexpected authorized simulation: %+v", run)
	}

	stateRequest := core.StateReadRequest{
		CapabilityID: "world.state.read", InstanceID: storage.DemoInstanceID, BranchID: storage.DemoBranchID,
	}
	response = performJSON(t, handler, "/api/v1/state/query", buyerToken, stateRequest)
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)
	response = performJSON(t, handler, "/api/v1/state/query", creatorToken, stateRequest)
	assertStatus(t, response, http.StatusOK)
	state := decodeData[storage.State](t, response)
	if state.HeadSequence != run.HeadSequence {
		t.Fatalf("authorized state does not reflect simulation: state=%+v run=%+v", state, run)
	}

	playerRead := core.PrivateEconomicRead{
		CapabilityID: "economy.private.read", InstanceID: storage.DemoInstanceID,
		BranchID: storage.DemoBranchID, SubjectID: storage.DemoEmployee1EntityID,
		Fields: []string{"account_id", "balance_minor"},
	}
	response = performJSON(t, handler, "/api/v1/private-economy/query", buyerToken, playerRead)
	assertStatus(t, response, http.StatusOK)
	view := decodeData[storage.PrivateEconomicView](t, response)
	if view.AccountID != storage.DemoBuyerAccountID || view.BalanceMinor == nil {
		t.Fatalf("player private view mismatch: %+v", view)
	}
	playerRead.SubjectID = storage.DemoEmployee2EntityID
	response = performJSON(t, handler, "/api/v1/private-economy/query", buyerToken, playerRead)
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)
	playerRead.SubjectID = storage.DemoEmployee1EntityID
	playerRead.Fields = append(playerRead.Fields, "owner_id")
	response = performJSON(t, handler, "/api/v1/private-economy/query", buyerToken, playerRead)
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)
}

func TestIssuanceHTTPRetainsKernelAuthorizationAndAccounting(t *testing.T) {
	ctx := context.Background()
	store, handler := openHTTPTestServer(t, ctx, filepath.Join(t.TempDir(), "issuance-http.db"))
	defer store.Close()
	command := core.IssueCurrencyCommand{
		CommandID: "cmd_http_issue", InstanceID: storage.DemoInstanceID, BranchID: storage.DemoBranchID,
		CapabilityID: "economy.issue", PolicyID: "policy_creator_credit_issue",
		IdempotencyKey: "idem_http_issue", ExpectedHead: 1, WorldTime: "2026-01-02T00:00:00Z",
		TargetAccountID: storage.DemoBuyerAccountID, CurrencyID: storage.DemoCurrencyID,
		AmountMinor: 10000000, ReasonCode: "http_creator_test",
	}
	response := performJSON(t, handler, "/api/v1/commands/issue-currency", buyerToken, command)
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)
	response = performJSON(t, handler, "/api/v1/commands/issue-currency", creatorToken, command)
	assertStatus(t, response, http.StatusOK)
	result := decodeData[core.IssueCurrencyResult](t, response)
	if result.FirstSequence != 2 || result.Replayed {
		t.Fatalf("unexpected HTTP issuance: %+v", result)
	}
	state, err := store.ReadDemoState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if state.HeadSequence != 2 || state.BuyerBalance != 10001000 || state.Postings != 7 {
		t.Fatalf("HTTP issuance accounting mismatch: %+v", state)
	}
}

func TestCohortHTTPAuthorizationConservationAndRoundTrip(t *testing.T) {
	ctx := context.Background()
	store, handler := openHTTPTestServer(t, ctx, filepath.Join(t.TempDir(), "cohort-http.db"))
	defer store.Close()
	if err := store.BootstrapM2Demo(ctx); err != nil {
		t.Fatal(err)
	}
	materialize := core.MaterializeCohortCommand{
		CommandID: "cmd_http_materialize", MaterializationID: "mat_http_person",
		InstanceID: storage.M2DemoInstanceID, BranchID: storage.M2DemoBranchID,
		CapabilityID: "world.cohort.materialize", IdempotencyKey: "idem_http_materialize",
		ExpectedHead: 1, WorldTime: "2026-09-22T01:00:00Z",
		SourceCohortID: storage.M2DemoCohortID, EntityID: "entity_http_person", DisplayName: "HTTP Person",
		PopulationCount: 1, AssetMinor: 1000, InventoryMinor: 10,
		ReceivableMinor: 200, LiabilityMinor: 150, AllocationAlgorithmVersion: "equal-share-v1",
	}
	overAllocation := materialize
	overAllocation.CommandID = "cmd_http_materialize_over"
	overAllocation.MaterializationID = "mat_http_over"
	overAllocation.IdempotencyKey = "idem_http_materialize_over"
	overAllocation.EntityID = "entity_http_over"
	overAllocation.PopulationCount = 21
	response := performJSON(t, handler, "/api/v1/commands/materialize-cohort", creatorToken, overAllocation)
	assertAPIError(t, response, http.StatusUnprocessableEntity, core.CodeConservationFailed)
	response = performJSON(t, handler, "/api/v1/commands/materialize-cohort", buyerToken, materialize)
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)
	response = performJSON(t, handler, "/api/v1/commands/materialize-cohort", creatorToken, materialize)
	assertStatus(t, response, http.StatusOK)
	materialized := decodeData[core.CohortTransitionResult](t, response)
	if materialized.FirstSequence != 2 || materialized.MaterializationID != materialize.MaterializationID {
		t.Fatalf("unexpected HTTP materialization: %+v", materialized)
	}

	conflict := materialize
	conflict.CommandID = "cmd_http_materialize_conflict"
	conflict.IdempotencyKey = "idem_http_materialize_conflict"
	conflict.AssetMinor++
	response = performJSON(t, handler, "/api/v1/commands/materialize-cohort", creatorToken, conflict)
	assertAPIError(t, response, http.StatusConflict, core.CodeMaterializationConflict)

	dematerialize := core.DematerializeCohortCommand{
		CommandID: "cmd_http_dematerialize", MaterializationID: materialize.MaterializationID,
		InstanceID: storage.M2DemoInstanceID, BranchID: storage.M2DemoBranchID,
		CapabilityID: "world.cohort.dematerialize", IdempotencyKey: "idem_http_dematerialize",
		ExpectedHead: 2, WorldTime: "2026-09-22T02:00:00Z", ReasonCode: "http_round_trip",
	}
	response = performJSON(t, handler, "/api/v1/commands/dematerialize-cohort", buyerToken, dematerialize)
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)
	response = performJSON(t, handler, "/api/v1/commands/dematerialize-cohort", creatorToken, dematerialize)
	assertStatus(t, response, http.StatusOK)
	dematerialized := decodeData[core.CohortTransitionResult](t, response)
	if dematerialized.FirstSequence != 3 || dematerialized.MaterializationID != materialize.MaterializationID {
		t.Fatalf("unexpected HTTP dematerialization: %+v", dematerialized)
	}
	replayed, err := store.Replay(ctx, storage.M2DemoInstanceID, storage.M2DemoBranchID, 3)
	if err != nil {
		t.Fatal(err)
	}
	var cohortPopulation, entityPopulation int64
	for _, population := range replayed.State.PopulationBalances {
		switch {
		case population.OwnerKind == "cohort" && population.OwnerID == storage.M2DemoCohortID:
			cohortPopulation = population.PopulationCount
		case population.OwnerKind == "entity" && population.OwnerID == materialize.EntityID:
			entityPopulation = population.PopulationCount
		}
	}
	if cohortPopulation != 20 || entityPopulation != 0 {
		t.Fatalf("HTTP round trip broke population conservation: cohort=%d entity=%d", cohortPopulation, entityPopulation)
	}
}

func TestAgentHTTPRunKnowledgeAndEncounterRemainScoped(t *testing.T) {
	ctx := context.Background()
	store, handler := openHTTPTestServer(t, ctx, filepath.Join(t.TempDir(), "agent-http.db"))
	defer store.Close()
	if _, err := store.BootstrapM2AgentDemo(ctx); err != nil {
		t.Fatal(err)
	}
	runRequest := core.AgentLifeRunRequest{
		CapabilityID: "world.agent.run", InstanceID: storage.M2DemoInstanceID,
		BranchID: storage.M2DemoBranchID, TargetWorldTime: storage.M2AgentNoonTime, Budget: 4,
	}
	response := performJSON(t, handler, "/api/v1/simulations/agents", buyerToken, runRequest)
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)
	response = performJSON(t, handler, "/api/v1/simulations/agents", creatorToken, runRequest)
	assertStatus(t, response, http.StatusOK)
	run := decodeData[storage.AgentLifeRunResult](t, response)
	if run.Status != "completed" || run.ProcessedItems != 4 || run.HeadSequence != 8 {
		t.Fatalf("unexpected HTTP Agent run: %+v", run)
	}

	knowledgeRequest := core.AgentKnowledgeRead{
		CapabilityID: "world.agent.knowledge.read", InstanceID: storage.M2DemoInstanceID,
		BranchID: storage.M2DemoBranchID, ObserverAgentID: storage.M2AgentAdaID,
		Fields: []string{"claim_key", "subject_agent_id", "place_id", "learned_world_time", "source_event_id"},
	}
	response = performJSON(t, handler, "/api/v1/agent-knowledge/query", adaAgentToken, knowledgeRequest)
	assertStatus(t, response, http.StatusOK)
	knowledge := decodeData[storage.AgentKnowledgeView](t, response)
	if len(knowledge.Facts) != 1 || knowledge.Facts[0].SubjectAgentID != storage.M2AgentBoID {
		t.Fatalf("unexpected HTTP Agent knowledge: %+v", knowledge)
	}
	crossRead := knowledgeRequest
	crossRead.ObserverAgentID = storage.M2AgentBoID
	response = performJSON(t, handler, "/api/v1/agent-knowledge/query", adaAgentToken, crossRead)
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)

	encounterRequest := core.EncounterRead{
		CapabilityID: "world.encounter.read", InstanceID: storage.M2DemoInstanceID,
		BranchID: storage.M2DemoBranchID, ObserverAgentID: storage.M2AgentAdaID,
		Fields: []string{"place_id", "world_time", "participants", "activity", "evidence"},
	}
	response = performJSON(t, handler, "/api/v1/encounters/query", adaAgentToken, encounterRequest)
	assertStatus(t, response, http.StatusOK)
	encounter := decodeData[storage.EncounterView](t, response)
	if encounter.PlaceID != storage.M2AgentCafeID || len(encounter.Participants) != 1 || encounter.Participants[0].AgentID != storage.M2AgentBoID {
		t.Fatalf("unexpected HTTP encounter: %+v", encounter)
	}
	wrongPrincipal := encounterRequest
	wrongPrincipal.PrincipalID = storage.M2AgentBoPrincipal
	response = performJSON(t, handler, "/api/v1/encounters/query", adaAgentToken, wrongPrincipal)
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)
}

func TestAgentRoutineHTTPAuthorizationAndRepeat(t *testing.T) {
	ctx := context.Background()
	store, handler := openHTTPTestServer(t, ctx, filepath.Join(t.TempDir(), "agent-routine-http.db"))
	defer store.Close()
	if _, err := store.BootstrapM2AgentDemo(ctx); err != nil {
		t.Fatal(err)
	}
	request := core.AgentRoutineRequest{
		CapabilityID: "world.agent.run", InstanceID: storage.M2DemoInstanceID,
		BranchID: storage.M2DemoBranchID, Days: 30,
	}
	response := performJSON(t, handler, "/api/v1/commands/define-agent-routine", adaAgentToken, request)
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)
	request.PrincipalID = storage.M2AgentAdaPrincipal
	response = performJSON(t, handler, "/api/v1/commands/define-agent-routine", creatorToken, request)
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)
	request.PrincipalID = ""
	response = performJSON(t, handler, "/api/v1/commands/define-agent-routine", creatorToken, request)
	assertStatus(t, response, http.StatusOK)
	defined := decodeData[storage.AgentRoutineResult](t, response)
	if defined.EventSequence != 5 || defined.ScheduleCount != 116 || defined.Replayed {
		t.Fatalf("unexpected HTTP routine definition: %+v", defined)
	}
	response = performJSON(t, handler, "/api/v1/commands/define-agent-routine", creatorToken, request)
	assertStatus(t, response, http.StatusOK)
	if repeated := decodeData[storage.AgentRoutineResult](t, response); !repeated.Replayed || repeated.EventSequence != 5 {
		t.Fatalf("HTTP routine retry changed authority: %+v", repeated)
	}
}

func TestVisibleEventHTTPPaginationAuthorizationAndRedaction(t *testing.T) {
	ctx := context.Background()
	store, handler := openHTTPTestServer(t, ctx, filepath.Join(t.TempDir(), "events-http.db"))
	defer store.Close()
	if _, err := store.RunStrictWorld(ctx, 65, 1000); err != nil {
		t.Fatal(err)
	}

	creatorPath := eventPath("world.events.read", storage.DemoBranchID, 2, "")
	response := performRequest(t, handler, http.MethodGet, creatorPath, creatorToken, nil)
	assertStatus(t, response, http.StatusOK)
	if strings.Contains(response.Body.String(), "event_sequence") {
		t.Fatalf("event authority sequence leaked through HTTP: %s", response.Body.String())
	}
	first := decodeData[visibleEventResponse](t, response)
	if len(first.Events) != 2 || first.NextCursor == "" {
		t.Fatalf("unexpected first event page: %+v", first)
	}

	response = performRequest(t, handler, http.MethodGet, eventPath("world.events.read", storage.DemoBranchID, 2, first.NextCursor), creatorToken, nil)
	assertStatus(t, response, http.StatusOK)
	second := decodeData[visibleEventResponse](t, response)
	if len(second.Events) == 0 || second.Events[0].EventID == first.Events[len(first.Events)-1].EventID {
		t.Fatalf("event cursor replayed the page boundary: first=%+v second=%+v", first, second)
	}

	response = performRequest(t, handler, http.MethodGet, eventPath("world.events.read", storage.DemoEmployee1EntityID, 2, first.NextCursor), buyerToken, nil)
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)

	response = performRequest(t, handler, http.MethodGet, eventPath("world.events.read", storage.DemoEmployee1EntityID, 100, ""), buyerToken, nil)
	assertStatus(t, response, http.StatusOK)
	player := decodeData[visibleEventResponse](t, response)
	if len(player.Events) == 0 {
		t.Fatal("player event page is empty")
	}
	for _, event := range player.Events {
		if event.EventType == "WorldInitialized" || event.EventType == "StoreRestocked" {
			t.Fatalf("player received hidden event over HTTP: %+v", event)
		}
	}

	response = performRequest(t, handler, http.MethodGet, eventPath("diagnostics.events.read", storage.DemoBranchID, 100, ""), operatorToken, nil)
	assertStatus(t, response, http.StatusOK)
	operator := decodeData[visibleEventResponse](t, response)
	if len(operator.Events) == 0 {
		t.Fatal("operator event page is empty")
	}
	for _, event := range operator.Events {
		if !event.Redacted || string(event.Payload) != `{"redacted":true}` {
			t.Fatalf("operator HTTP event was not redacted: %+v", event)
		}
	}
}

func TestVisibleEventSSEReconnectsWithoutLeakingAuthoritySequence(t *testing.T) {
	ctx := context.Background()
	store, handler := openHTTPTestServer(t, ctx, filepath.Join(t.TempDir(), "events-sse.db"))
	defer store.Close()
	if _, err := store.RunStrictWorld(ctx, 65, 1000); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	path := eventStreamPath("world.events.read", storage.DemoEmployee1EntityID, 1)

	first := readOneSSEEvent(t, server.URL+path, buyerToken, "")
	if first.ID == "" || first.Event != "world_event" || strings.Contains(first.Data, "event_sequence") {
		t.Fatalf("invalid first SSE frame: %+v", first)
	}
	var firstEvent storage.VisibleEvent
	if err := json.Unmarshal([]byte(first.Data), &firstEvent); err != nil {
		t.Fatal(err)
	}
	if firstEvent.EventType == "WorldInitialized" || firstEvent.EventType == "StoreRestocked" {
		t.Fatalf("SSE leaked hidden event: %+v", firstEvent)
	}

	second := readOneSSEEvent(t, server.URL+path, buyerToken, first.ID)
	if second.ID == "" || second.ID == first.ID || strings.Contains(second.Data, "event_sequence") {
		t.Fatalf("SSE reconnect did not advance safely: first=%+v second=%+v", first, second)
	}
}

func TestVisibleEventSSEHeartbeatsAfterCursorCatchesUp(t *testing.T) {
	ctx := context.Background()
	store, handler := openHTTPTestServer(t, ctx, filepath.Join(t.TempDir(), "events-heartbeat.db"))
	defer store.Close()
	if _, err := store.RunStrictWorld(ctx, 65, 1000); err != nil {
		t.Fatal(err)
	}
	api, ok := handler.(*Server)
	if !ok {
		t.Fatalf("test handler has unexpected type %T", handler)
	}
	api.eventPollInterval = 5 * time.Millisecond
	api.heartbeatInterval = 10 * time.Millisecond

	response := performRequest(t, handler, http.MethodGet, eventPath("world.events.read", storage.DemoEmployee1EntityID, 100, ""), buyerToken, nil)
	assertStatus(t, response, http.StatusOK)
	page := decodeData[visibleEventResponse](t, response)
	if page.NextCursor == "" {
		t.Fatal("caught-up SSE test requires an end cursor")
	}

	server := httptest.NewServer(handler)
	defer server.Close()
	request, err := http.NewRequest(http.MethodGet, server.URL+eventStreamPath("world.events.read", storage.DemoEmployee1EntityID, 100), nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+buyerToken)
	request.Header.Set("Last-Event-ID", page.NextCursor)
	stream, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Body.Close()
	reader := bufio.NewReader(stream.Body)
	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if line != ": keepalive\n" {
		t.Fatalf("expected SSE heartbeat after catch-up, got %q", line)
	}
}

type sseFrame struct {
	ID    string
	Event string
	Data  string
}

func readOneSSEEvent(t *testing.T, endpoint, token, lastEventID string) sseFrame {
	t.Helper()
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, endpoint, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	if lastEventID != "" {
		request.Header.Set("Last-Event-ID", lastEventID)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("SSE status=%d body=%s", response.StatusCode, body)
	}
	if contentType := response.Header.Get("Content-Type"); !strings.HasPrefix(contentType, "text/event-stream") {
		t.Fatalf("unexpected SSE content type %q", contentType)
	}
	var frame sseFrame
	scanner := bufio.NewScanner(response.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			return frame
		}
		switch {
		case strings.HasPrefix(line, "id: "):
			frame.ID = strings.TrimPrefix(line, "id: ")
		case strings.HasPrefix(line, "event: "):
			frame.Event = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			frame.Data = strings.TrimPrefix(line, "data: ")
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	t.Fatal("SSE stream ended before one event frame")
	return sseFrame{}
}

func eventPath(capabilityID, subjectID string, limit int, cursor string) string {
	query := url.Values{
		"capability_id": {capabilityID},
		"instance_id":   {storage.DemoInstanceID},
		"branch_id":     {storage.DemoBranchID},
		"subject_id":    {subjectID},
		"limit":         {strconv.Itoa(limit)},
	}
	if cursor != "" {
		query.Set("cursor", cursor)
	}
	return "/api/v1/events?" + query.Encode()
}

func eventStreamPath(capabilityID, subjectID string, limit int) string {
	return strings.Replace(eventPath(capabilityID, subjectID, limit, ""), "/api/v1/events?", "/api/v1/events/stream?", 1)
}

func openHTTPTestServer(t *testing.T, ctx context.Context, path string) (*storage.Store, http.Handler) {
	t.Helper()
	store, err := storage.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.BootstrapDemo(ctx); err != nil {
		store.Close()
		t.Fatal(err)
	}
	authenticator, err := NewStaticTokenAuthenticator(map[string]string{
		buyerToken: "principal_buyer", creatorToken: "principal_creator", operatorToken: "principal_operator",
		adaAgentToken: storage.M2AgentAdaPrincipal, boAgentToken: storage.M2AgentBoPrincipal,
	})
	if err != nil {
		store.Close()
		t.Fatal(err)
	}
	cursors, err := NewCursorCodec([]byte("test-cursor-secret-must-be-at-least-32-bytes"))
	if err != nil {
		store.Close()
		t.Fatal(err)
	}
	server, err := New(store, authenticator, cursors)
	if err != nil {
		store.Close()
		t.Fatal(err)
	}
	return store, server.Handler()
}

func demoHTTPPurchase() core.PurchaseCommand {
	return core.PurchaseCommand{
		CommandID: "cmd_http_purchase", InstanceID: storage.DemoInstanceID, BranchID: storage.DemoBranchID,
		ActorID: "buyer", IdempotencyKey: "idem_http_purchase", ExpectedHead: 1,
		WorldTime: "2026-09-22T09:00:00Z", BuyerAccountID: storage.DemoBuyerAccountID,
		SellerAccountID: storage.DemoSellerAccountID, BuyerLocationID: storage.DemoBuyerLocationID,
		SellerLocationID: storage.DemoSellerLocationID, SKUID: storage.DemoSKUID,
		CurrencyID: storage.DemoCurrencyID, QuantityMinor: 2, UnitPriceMinor: 25,
	}
}

func performJSON(t *testing.T, handler http.Handler, path, token string, value any) *httptest.ResponseRecorder {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return performRawJSON(t, handler, path, token, string(encoded))
}

func performRawJSON(t *testing.T, handler http.Handler, path, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func performRequest(t *testing.T, handler http.Handler, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	switch value := body.(type) {
	case nil:
		reader = bytes.NewReader(nil)
	case string:
		reader = bytes.NewReader([]byte(value))
	case *strings.Reader:
		content, err := io.ReadAll(value)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(content)
	default:
		t.Fatalf("unsupported request body type %T", body)
	}
	request := httptest.NewRequest(method, path, reader)
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func assertStatus(t *testing.T, response *httptest.ResponseRecorder, expected int) {
	t.Helper()
	if response.Code != expected {
		t.Fatalf("status=%d want=%d body=%s", response.Code, expected, response.Body.String())
	}
}

func assertSecurityHeaders(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	if response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("X-Content-Type-Options") != "nosniff" || response.Header().Get("X-Request-ID") == "" {
		t.Fatalf("security/request headers missing: %v", response.Header())
	}
}

func assertAPIError(t *testing.T, response *httptest.ResponseRecorder, expectedStatus int, expectedCode core.ErrorCode) {
	t.Helper()
	assertStatus(t, response, expectedStatus)
	var envelope struct {
		Error struct {
			Code      string `json:"code"`
			Message   string `json:"message"`
			RequestID string `json:"request_id"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Error.Code != string(expectedCode) || envelope.Error.Message == "" || envelope.Error.RequestID == "" {
		t.Fatalf("error envelope mismatch: %+v", envelope.Error)
	}
}

func decodeData[T any](t *testing.T, response *httptest.ResponseRecorder) T {
	t.Helper()
	var envelope struct {
		Data T `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	return envelope.Data
}
