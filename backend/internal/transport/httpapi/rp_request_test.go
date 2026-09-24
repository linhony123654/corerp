package httpapi

import (
	"context"
	"net/http"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/storage"
)

func TestRPRequestRetirementHTTPAuthenticatedFence(t *testing.T) {
	ctx := context.Background()
	s, handler := openHTTPTestServer(t, ctx, filepath.Join(t.TempDir(), "retirement-http.db"))
	defer s.Close()
	if _, err := s.PrepareRPTravel(ctx); err != nil {
		t.Fatal(err)
	}
	const route = "/api/v1/rp/requests/retire"
	r := storage.RPRequestRetireRequest{Operation: "open", IdempotencyKey: "never-open"}
	response := performJSON(t, handler, route, "", r)
	assertStatus(t, response, http.StatusUnauthorized)
	spoof := r
	spoof.PrincipalID = "principal_creator"
	response = performJSON(t, handler, route, rpPlayerToken, spoof)
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)
	response = performJSON(t, handler, route, rpPlayerToken, map[string]any{"operation": "open", "idempotency_key": "never-open", "world_override": "bad"})
	assertStatus(t, response, http.StatusBadRequest)
	for i := 0; i < 2; i++ {
		response = performJSON(t, handler, route, rpPlayerToken, r)
		assertStatus(t, response, http.StatusOK)
		out := decodeData[storage.RPRequestOutcome](t, response)
		if out.Status != "retired" || out.ProtocolVersion != storage.RPClientProtocolVersion || response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("bad outcome: %+v", out)
		}
	}
	open := core.RPSessionOpenRequest{InstanceID: storage.M2DemoInstanceID, BranchID: storage.M2DemoBranchID, EntityID: storage.M2RPPlayerID, POV: "first_person", IdempotencyKey: r.IdempotencyKey}
	response = performJSON(t, handler, "/api/v1/rp/sessions/open", rpPlayerToken, open)
	assertAPIError(t, response, http.StatusConflict, core.CodeRequestRetired)
	open.IdempotencyKey = "accepted-open"
	response = performJSON(t, handler, "/api/v1/rp/sessions/open", rpPlayerToken, open)
	assertStatus(t, response, http.StatusOK)
	session := decodeData[storage.RPSession](t, response)
	r.IdempotencyKey = open.IdempotencyKey
	response = performJSON(t, handler, route, rpPlayerToken, r)
	assertStatus(t, response, http.StatusOK)
	out := decodeData[storage.RPRequestOutcome](t, response)
	if out.Status != "completed" || out.SessionID != session.SessionID {
		t.Fatalf("missing open receipt: %+v", out)
	}
	r = storage.RPRequestRetireRequest{Operation: "dialogue", SessionID: session.SessionID, IdempotencyKey: "no-dialogue"}
	response = performJSON(t, handler, route, creatorToken, r)
	assertAPIError(t, response, http.StatusNotFound, core.CodeNotFound)
	response = performJSON(t, handler, route, rpPlayerToken, r)
	assertStatus(t, response, http.StatusOK)
	response = performJSON(t, handler, "/api/v1/rp/observe", rpPlayerToken, core.RPSessionReadRequest{SessionID: session.SessionID})
	assertStatus(t, response, http.StatusOK)
	view := decodeData[storage.RPObservation](t, response)
	response = performJSON(t, handler, "/api/v1/rp/turns/run", rpPlayerToken, core.RPSpeechRequest{SessionID: session.SessionID, Text: "迟到的请求", ExpectedCursor: view.ObservationCursor, IdempotencyKey: r.IdempotencyKey})
	assertAPIError(t, response, http.StatusConflict, core.CodeRequestRetired)
}
