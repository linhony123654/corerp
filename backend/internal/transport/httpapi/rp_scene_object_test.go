package httpapi

import (
	"context"
	"net/http"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/storage"
)

func TestRPSceneObjectHTTPBindsPlayerAndPreservesExactRetry(t *testing.T) {
	ctx := context.Background()
	store, handler := openHTTPTestServer(t, ctx, filepath.Join(t.TempDir(), "scene-object-http.db"))
	defer store.Close()
	setup, err := store.PrepareRPTravel(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ConfigureStudioAccessLocal(ctx, storage.StudioAccessRequest{
		Purpose: "create_world",
		Binding: core.CareerBinding{
			PrincipalID: "principal_operator", InstanceID: storage.M2DemoInstanceID, BranchID: storage.M2DemoBranchID,
			ExpectedHead: setup.EventSequence, IdempotencyKey: "scene-object-http-grant",
		},
		TargetPrincipalID: "principal_creator", Status: "active",
	}); err != nil {
		t.Fatal(err)
	}
	const world = "scene-object-http-world"
	create := storage.StudioCreateRequest{
		PrincipalID: "principal_creator", AuthorityInstanceID: storage.M2DemoInstanceID, AuthorityBranchID: storage.M2DemoBranchID,
		InstanceID: world, IdempotencyKey: world, PlayerPrincipalID: storage.M2RPPlayerPrincipal,
		SystemPackage: httpStudioPackage("system"), NarrativePackage: httpStudioPackage("narrative"),
		Spec: core.StudioWorldSpec{
			Version: core.StudioWorldSpecVersion, Name: "物件 HTTP 世界", StartWorldTime: core.StudioWorldStart,
			Population: 2, OpeningMoneyMinor: 20, OpeningStockMinor: 2,
			Places:  []core.StudioWorldPlace{{Key: "home", Name: "家", Kind: "home"}, {Key: "square", Name: "广场", Kind: "public"}},
			Links:   []core.StudioWorldLink{{From: "home", To: "square", Minutes: 5}},
			People:  []core.StudioWorldPerson{{Key: "lin", Name: "Lin", Place: "home", Player: true}, {Key: "cai", Name: "Cai", Place: "home"}},
			Objects: []core.StudioWorldObject{{Key: "front_door", Name: "前门", Place: "home", Kind: "door", InitialState: "closed"}},
		},
	}
	created, err := store.CreateStudioWorld(ctx, create)
	if err != nil {
		t.Fatal(err)
	}
	response := performJSON(t, handler, "/api/v1/rp/sessions/open", rpPlayerToken, core.RPSessionOpenRequest{
		InstanceID: world, BranchID: created.BranchID, EntityID: created.EntityID, POV: "second_person", IdempotencyKey: "scene-object-http-session",
	})
	assertStatus(t, response, http.StatusOK)
	session := decodeData[storage.RPSession](t, response)
	response = performJSON(t, handler, "/api/v1/rp/observe", rpPlayerToken, core.RPSessionReadRequest{SessionID: session.SessionID})
	assertStatus(t, response, http.StatusOK)
	view := decodeData[storage.RPObservation](t, response)
	if len(view.SceneObjects) != 1 || view.SceneObjects[0].DisplayName != "前门" || view.SceneObjects[0].State != "closed" {
		t.Fatalf("HTTP observation omitted scene object: %+v", view.SceneObjects)
	}
	request := core.RPSceneObjectActionRequest{
		SessionID: session.SessionID, ObjectID: view.SceneObjects[0].ObjectID, Action: "open",
		ExpectedCursor: view.ObservationCursor, IdempotencyKey: "scene-object-http-open",
	}
	response = performJSON(t, handler, "/api/v1/rp/actions/object", creatorToken, request)
	assertAPIError(t, response, http.StatusNotFound, core.CodeNotFound)
	spoof := request
	spoof.PrincipalID = storage.M2RPPlayerPrincipal
	response = performJSON(t, handler, "/api/v1/rp/actions/object", creatorToken, spoof)
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)
	response = performJSON(t, handler, "/api/v1/rp/actions/object", rpPlayerToken, request)
	assertStatus(t, response, http.StatusOK)
	first := decodeData[storage.RPSceneObjectActionResult](t, response)
	if first.Replayed || first.PreviousState != "closed" || first.Object.State != "open" || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("HTTP scene object action was not committed privately: %+v", first)
	}
	response = performJSON(t, handler, "/api/v1/rp/actions/object", rpPlayerToken, request)
	assertStatus(t, response, http.StatusOK)
	replayed := decodeData[storage.RPSceneObjectActionResult](t, response)
	if !replayed.Replayed || replayed.EventID != first.EventID {
		t.Fatalf("HTTP scene object retry duplicated action: %+v", replayed)
	}
	response = performJSON(t, handler, "/api/v1/rp/observe", rpPlayerToken, core.RPSessionReadRequest{SessionID: session.SessionID})
	assertStatus(t, response, http.StatusOK)
	after := decodeData[storage.RPObservation](t, response)
	if len(after.SceneObjects) != 1 || after.SceneObjects[0].State != "open" || len(after.SceneObjects[0].Actions) != 1 || after.SceneObjects[0].Actions[0] != "close" {
		t.Fatalf("HTTP observation did not expose committed legal transition: %+v", after.SceneObjects)
	}
	assertStatus(t, performRequest(t, handler, http.MethodGet, "/api/v1/rp/actions/object", rpPlayerToken, nil), http.StatusMethodNotAllowed)
}
