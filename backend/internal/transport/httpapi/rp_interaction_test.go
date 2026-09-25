package httpapi

import (
	"context"
	"net/http"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/storage"
)

func TestRPInteractionHTTPMixedAuthenticationAndExactRecovery(t *testing.T) {
	ctx := context.Background()
	store, handler := openHTTPTestServer(t, ctx, filepath.Join(t.TempDir(), "interaction-http.db"))
	defer store.Close()
	if _, err := store.PrepareRPTravel(ctx); err != nil {
		t.Fatal(err)
	}
	response := performJSON(t, handler, "/api/v1/rp/sessions/open", rpPlayerToken, core.RPSessionOpenRequest{InstanceID: storage.M2DemoInstanceID, BranchID: storage.M2DemoBranchID, EntityID: storage.M2RPPlayerID, POV: "second_person", IdempotencyKey: "interaction-http-session"})
	assertStatus(t, response, http.StatusOK)
	session := decodeData[storage.RPSession](t, response)
	read := core.RPSessionReadRequest{SessionID: session.SessionID}
	response = performJSON(t, handler, "/api/v1/rp/interactions/default/read", creatorToken, read)
	assertAPIError(t, response, http.StatusNotFound, core.CodeNotFound)
	response = performJSON(t, handler, "/api/v1/rp/interactions/default/read", rpPlayerToken, read)
	assertStatus(t, response, http.StatusOK)
	mode := decodeData[storage.RPInteractionModeView](t, response)
	if mode.Mode != "AUTO" || mode.Revision != 0 {
		t.Fatalf("wrong HTTP default mode: %+v", mode)
	}
	setting := storage.RPInteractionModeSetRequest{SessionID: session.SessionID, Mode: "DIALOGUE", ExpectedRevision: 0, IdempotencyKey: "http-default"}
	response = performJSON(t, handler, "/api/v1/rp/interactions/default/set", rpPlayerToken, setting)
	assertStatus(t, response, http.StatusOK)
	mode = decodeData[storage.RPInteractionModeView](t, response)
	if mode.Mode != "DIALOGUE" || mode.Revision != 1 {
		t.Fatalf("HTTP mode write not persisted: %+v", mode)
	}
	response = performJSON(t, handler, "/api/v1/rp/observe", rpPlayerToken, read)
	assertStatus(t, response, http.StatusOK)
	observation := decodeData[storage.RPObservation](t, response)
	destination := ""
	for _, place := range observation.ReachablePlaces {
		if place.PlaceID == "place_m2_home_ada" && place.CanMoveNow {
			destination = place.DisplayName
		}
	}
	if destination == "" {
		t.Fatal("no fixture destination")
	}
	request := core.RPInteractionRequest{SessionID: session.SessionID, ExpectedCursor: observation.ObservationCursor, IdempotencyKey: "interaction-http-original", Mode: "SCENE", Text: "去" + destination + "，随后说「你好，Ada。」"}
	response = performJSON(t, handler, "/api/v1/rp/interactions/run", creatorToken, request)
	assertAPIError(t, response, http.StatusNotFound, core.CodeNotFound)
	response = performJSON(t, handler, "/api/v1/rp/interactions/run", rpPlayerToken, request)
	assertStatus(t, response, http.StatusOK)
	result := decodeData[storage.RPInteractionResult](t, response)
	if result.Status != "settled" || result.PlanKind != "MIXED" || len(result.Outcomes) != 2 || result.Outcomes[0].EventSequence >= result.Outcomes[1].EventSequence || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("HTTP mixed interaction not ordered/private: %+v", result)
	}
	response = performJSON(t, handler, "/api/v1/rp/interactions/run", rpPlayerToken, request)
	assertStatus(t, response, http.StatusOK)
	replayed := decodeData[storage.RPInteractionResult](t, response)
	if !replayed.Replayed || replayed.Outcomes[0].EventID != result.Outcomes[0].EventID || replayed.Outcomes[1].EventID != result.Outcomes[1].EventID {
		t.Fatalf("HTTP exact retry differed: %+v", replayed)
	}
	response = performJSON(t, handler, "/api/v1/rp/interactions/resume", rpPlayerToken, storage.RPInteractionResumeRequest{SessionID: session.SessionID, IdempotencyKey: request.IdempotencyKey})
	assertStatus(t, response, http.StatusOK)
	resumed := decodeData[storage.RPInteractionResult](t, response)
	if !resumed.Replayed || resumed.Outcomes[1].EventID != result.Outcomes[1].EventID {
		t.Fatalf("HTTP resume changed accepted world: %+v", resumed)
	}
	request.Text = "去" + destination + "，随后说「不同的内容」"
	response = performJSON(t, handler, "/api/v1/rp/interactions/run", rpPlayerToken, request)
	assertAPIError(t, response, http.StatusConflict, core.CodeIdempotencyMismatch)
	stop := storage.RPInteractionResumeRequest{SessionID: session.SessionID, IdempotencyKey: request.IdempotencyKey}
	response = performJSON(t, handler, "/api/v1/rp/interactions/stop", creatorToken, stop)
	assertAPIError(t, response, http.StatusNotFound, core.CodeNotFound)
	response = performJSON(t, handler, "/api/v1/rp/interactions/stop", rpPlayerToken, stop)
	assertStatus(t, response, http.StatusOK)
	if settled := decodeData[storage.RPInteractionResult](t, response); settled.Status != "settled" || !settled.Replayed || settled.Outcomes[1].EventID != result.Outcomes[1].EventID {
		t.Fatalf("stop rewrote already settled interaction: %+v", settled)
	}
}
