package httpapi

import (
	"context"
	"net/http"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/storage"
)

func TestRPObjectStowHTTPUsesOwnerControlAndExactReceipt(t *testing.T) {
	ctx := context.Background()
	store, handler := openHTTPTestServer(t, ctx, filepath.Join(t.TempDir(), "stow-http.db"))
	defer store.Close()
	setup, err := store.PrepareRPTravel(ctx)
	if err != nil {
		t.Fatal(err)
	}
	anchor, err := store.DefineRPObjectAnchor(ctx, storage.RPObjectAnchorRequest{Binding: core.CareerBinding{PrincipalID: "principal_creator", InstanceID: storage.M2DemoInstanceID, BranchID: storage.M2DemoBranchID, ExpectedHead: setup.EventSequence, IdempotencyKey: "stow-http-anchor"}, PlaceID: storage.M2AgentCafeID, ZoneKey: "main", AnchorCode: "stow-shelf", DisplayName: "物品架"})
	if err != nil {
		t.Fatal(err)
	}
	source, err := store.DefineRPObjectSource(ctx, storage.RPObjectSourceRequest{Binding: core.CareerBinding{PrincipalID: "principal_creator", InstanceID: storage.M2DemoInstanceID, BranchID: storage.M2DemoBranchID, ExpectedHead: anchor.EventSequence, IdempotencyKey: "stow-http-source"}, AnchorID: anchor.Fact.AnchorID, OwnerEntityID: storage.M2RPPlayerID, SKUID: storage.M2DemoSKUID, DisplayName: storage.M2DemoSKUID})
	if err != nil {
		t.Fatal(err)
	}
	response := performJSON(t, handler, "/api/v1/rp/sessions/open", rpPlayerToken, core.RPSessionOpenRequest{InstanceID: storage.M2DemoInstanceID, BranchID: storage.M2DemoBranchID, EntityID: storage.M2RPPlayerID, POV: "second_person", IdempotencyKey: "stow-http-session"})
	assertStatus(t, response, http.StatusOK)
	session := decodeData[storage.RPSession](t, response)
	observe := func() storage.RPObservation {
		t.Helper()
		response := performJSON(t, handler, "/api/v1/rp/observe", rpPlayerToken, core.RPSessionReadRequest{SessionID: session.SessionID})
		assertStatus(t, response, http.StatusOK)
		return decodeData[storage.RPObservation](t, response)
	}
	view := observe()
	stage := core.RPObjectRequest{SessionID: session.SessionID, ExpectedCursor: view.ObservationCursor, IdempotencyKey: "stow-http-stage", Action: "stage", SourceID: source.Fact.SourceID}
	response = performJSON(t, handler, "/api/v1/rp/actions/object", rpPlayerToken, stage)
	assertStatus(t, response, http.StatusOK)
	item := decodeData[storage.RPObjectResult](t, response)
	view = observe()
	stow := core.RPObjectRequest{SessionID: session.SessionID, ExpectedCursor: view.ObservationCursor, IdempotencyKey: "stow-http-commit", Action: "stow", ObjectID: item.ObjectID}
	assertStatus(t, performJSON(t, handler, "/api/v1/rp/actions/object", "", stow), http.StatusUnauthorized)
	assertAPIError(t, performJSON(t, handler, "/api/v1/rp/actions/object", creatorToken, stow), http.StatusNotFound, core.CodeNotFound)
	response = performJSON(t, handler, "/api/v1/rp/actions/object", rpPlayerToken, stow)
	assertStatus(t, response, http.StatusOK)
	returned := decodeData[storage.RPObjectResult](t, response)
	if returned.Replayed || returned.ObjectID != item.ObjectID {
		t.Fatalf("HTTP object did not leave escrow: %+v", returned)
	}
	response = performJSON(t, handler, "/api/v1/rp/actions/object", rpPlayerToken, stow)
	assertStatus(t, response, http.StatusOK)
	if repeat := decodeData[storage.RPObjectResult](t, response); !repeat.Replayed || repeat.EventID != returned.EventID {
		t.Fatalf("HTTP stow exact retry lost receipt: %+v", repeat)
	}
	changed := stow
	changed.ObjectID = "different-object"
	assertAPIError(t, performJSON(t, handler, "/api/v1/rp/actions/object", rpPlayerToken, changed), http.StatusConflict, core.CodeIdempotencyMismatch)
	if differences, err := store.CompareProjections(ctx, storage.M2DemoInstanceID, storage.M2DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatalf("HTTP stow disagreed with replay: %+v %v", differences, err)
	}
}
