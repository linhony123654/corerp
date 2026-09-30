package httpapi

import (
	"context"
	"net/http"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/storage"
)

func TestRPNonverbalHTTPControlAndExactRetry(t *testing.T) {
	ctx := context.Background()
	store, handler := openHTTPTestServer(t, ctx, filepath.Join(t.TempDir(), "nonverbal-http.db"))
	defer store.Close()
	if _, err := store.PrepareRPTravel(ctx); err != nil {
		t.Fatal(err)
	}
	response := performJSON(t, handler, "/api/v1/rp/sessions/open", rpPlayerToken, core.RPSessionOpenRequest{InstanceID: storage.M2DemoInstanceID, BranchID: storage.M2DemoBranchID, EntityID: storage.M2RPPlayerID, POV: "second_person", IdempotencyKey: "nonverbal-http"})
	assertStatus(t, response, http.StatusOK)
	session := decodeData[storage.RPSession](t, response)
	response = performJSON(t, handler, "/api/v1/rp/observe", rpPlayerToken, core.RPSessionReadRequest{SessionID: session.SessionID})
	assertStatus(t, response, http.StatusOK)
	view := decodeData[storage.RPObservation](t, response)
	request := core.RPNonverbalRequest{SessionID: session.SessionID, ExpectedCursor: view.ObservationCursor, IdempotencyKey: "http-look", Action: "look_at", TargetEntityID: storage.M2RPNPCID}
	response = performJSON(t, handler, "/api/v1/rp/actions/nonverbal", "", request)
	assertAPIError(t, response, http.StatusUnauthorized, core.CodeUnauthenticated)
	response = performJSON(t, handler, "/api/v1/rp/actions/nonverbal", creatorToken, request)
	assertAPIError(t, response, http.StatusNotFound, core.CodeNotFound)
	spoof := request
	spoof.PrincipalID = storage.M2RPPlayerPrincipal
	response = performJSON(t, handler, "/api/v1/rp/actions/nonverbal", creatorToken, spoof)
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)
	response = performJSON(t, handler, "/api/v1/rp/actions/nonverbal", rpPlayerToken, request)
	assertStatus(t, response, http.StatusOK)
	accepted := decodeData[storage.RPNonverbalResult](t, response)
	if accepted.Replayed || accepted.EventSequence != view.ObservationCursor+1 || accepted.WorldTime != view.WorldTime {
		t.Fatal("HTTP expression did not commit once without advancing clock", accepted)
	}
	response = performJSON(t, handler, "/api/v1/rp/actions/nonverbal", rpPlayerToken, request)
	assertStatus(t, response, http.StatusOK)
	if replay := decodeData[storage.RPNonverbalResult](t, response); !replay.Replayed || replay.EventID != accepted.EventID {
		t.Fatal("HTTP retry created another action", replay)
	}
	changed := request
	changed.Action = "nod"
	response = performJSON(t, handler, "/api/v1/rp/actions/nonverbal", rpPlayerToken, changed)
	assertAPIError(t, response, http.StatusConflict, core.CodeIdempotencyMismatch)
}
