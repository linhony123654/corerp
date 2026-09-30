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

func TestRPObservatoryHTTPIsSessionScopedAndRedacted(t *testing.T) {
	ctx := context.Background()
	store, handler := openHTTPTestServer(t, ctx, filepath.Join(t.TempDir(), "observatory-http.db"))
	defer store.Close()
	setup, err := store.PrepareRPTravel(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ConfigureStudioAccessLocal(ctx, storage.StudioAccessRequest{Purpose: "create_world", Binding: core.CareerBinding{PrincipalID: "principal_operator", InstanceID: storage.M2DemoInstanceID, BranchID: storage.M2DemoBranchID, ExpectedHead: setup.EventSequence, IdempotencyKey: "observatory-http-grant"}, TargetPrincipalID: "principal_creator", Status: "active"}); err != nil {
		t.Fatal(err)
	}
	const world = "observatory-http-world"
	create := storage.StudioCreateRequest{
		PrincipalID: "principal_creator", AuthorityInstanceID: storage.M2DemoInstanceID, AuthorityBranchID: storage.M2DemoBranchID,
		InstanceID: world, IdempotencyKey: world, PlayerPrincipalID: storage.M2RPPlayerPrincipal,
		SystemPackage: httpStudioPackage("system"), NarrativePackage: httpStudioPackage("narrative"),
		Spec: core.StudioWorldSpec{
			Version: core.StudioWorldSpecVersion, Name: "观测台 HTTP 世界", StartWorldTime: core.StudioWorldStart,
			Population: 2, OpeningMoneyMinor: 20, OpeningStockMinor: 2,
			Places:        []core.StudioWorldPlace{{Key: "home", Name: "家", Kind: "home"}, {Key: "square", Name: "广场", Kind: "public"}},
			Links:         []core.StudioWorldLink{{From: "home", To: "square", Minutes: 5}},
			People:        []core.StudioWorldPerson{{Key: "lin", Name: "Lin", Place: "home", Player: true}, {Key: "bo", Name: "Bo", Place: "home"}},
			Acquaintances: [][2]string{{"lin", "bo"}},
		},
	}
	create.SystemPackage.Content.SystemRules.RPExecutionMode = "orchestrated"
	create.SystemPackage.Content.SystemRules.MaxActiveResponders = 1
	create.SystemPackage.Manifest.ContentHash, _ = core.HashJSON(create.SystemPackage.Content)
	created, err := store.CreateStudioWorld(ctx, create)
	if err != nil {
		t.Fatal(err)
	}
	response := performJSON(t, handler, "/api/v1/rp/sessions/open", rpPlayerToken, core.RPSessionOpenRequest{InstanceID: world, BranchID: created.BranchID, EntityID: created.EntityID, POV: "second_person", IdempotencyKey: "observatory-http-session"})
	assertStatus(t, response, http.StatusOK)
	session := decodeData[storage.RPSession](t, response)
	response = performJSON(t, handler, "/api/v1/rp/observe", rpPlayerToken, core.RPSessionReadRequest{SessionID: session.SessionID})
	assertStatus(t, response, http.StatusOK)
	observed := decodeData[storage.RPObservation](t, response)
	response = performJSON(t, handler, "/api/v1/rp/turns/run", rpPlayerToken, core.RPSpeechRequest{SessionID: session.SessionID, ExpectedCursor: observed.ObservationCursor, IdempotencyKey: "observatory-http-turn", Text: "Bo，你在吗？"})
	assertStatus(t, response, http.StatusOK)
	request := storage.RPObservatoryRequest{SessionID: session.SessionID, Limit: 10}
	response = performJSON(t, handler, "/api/v1/rp/observatory/read", creatorToken, request)
	assertAPIError(t, response, http.StatusNotFound, core.CodeNotFound)
	spoof := request
	spoof.PrincipalID = storage.M2RPPlayerPrincipal
	response = performJSON(t, handler, "/api/v1/rp/observatory/read", creatorToken, spoof)
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)
	response = performJSON(t, handler, "/api/v1/rp/observatory/read", rpPlayerToken, request)
	assertStatus(t, response, http.StatusOK)
	view := decodeData[storage.RPObservatoryView](t, response)
	if len(view.Traces) != 1 || view.Traces[0].Stage != "settled" || len(view.Traces[0].Activations) != 1 || view.Traces[0].Activations[0].DisplayName != "Bo" || view.ProjectionHealth.Status != "healthy" {
		t.Fatalf("wrong HTTP observatory view: %+v", view)
	}
	bo, _ := core.StudioWorldObjectID(world, "entity", "bo")
	for _, hidden := range []string{created.EntityID, bo, "event_", "turn_rp_", "principal_", "proposal_json", "input_hash", "goal_code"} {
		if strings.Contains(response.Body.String(), hidden) {
			t.Fatalf("HTTP observatory leaked %q: %s", hidden, response.Body.String())
		}
	}
	assertStatus(t, performRequest(t, handler, http.MethodGet, "/api/v1/rp/observatory/read", rpPlayerToken, nil), http.StatusMethodNotAllowed)
}
