package httpapi

import (
	"context"
	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/storage"
	"net/http"
	"path/filepath"
	"testing"
)

func TestRPDiscoveryHTTPToExistingSession(t *testing.T) {
	ctx := context.Background()
	s, handler := openHTTPTestServer(t, ctx, filepath.Join(t.TempDir(), "discovery-http.db"))
	defer s.Close()
	if _, err := s.PrepareRPTravel(ctx); err != nil {
		t.Fatal(err)
	}
	const route = "/api/v1/rp/bindings/list"
	response := performJSON(t, handler, route, "", map[string]any{})
	assertStatus(t, response, http.StatusUnauthorized)
	response = performJSON(t, handler, route, rpPlayerToken, map[string]any{"principal_id": "principal_creator"})
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)
	response = performJSON(t, handler, route, rpPlayerToken, map[string]any{"observer_entity_id": storage.M2RPNPCID})
	assertStatus(t, response, http.StatusBadRequest)
	response = performJSON(t, handler, route, creatorToken, map[string]any{})
	assertStatus(t, response, http.StatusOK)
	if len(decodeData[storage.RPDiscovery](t, response).Bindings) != 0 {
		t.Fatal("creator grant bypassed player scope")
	}
	response = performJSON(t, handler, route, rpPlayerToken, map[string]any{})
	assertStatus(t, response, http.StatusOK)
	out := decodeData[storage.RPDiscovery](t, response)
	if out.ProtocolVersion != storage.RPClientProtocolVersion || len(out.Bindings) != 1 || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("invalid discovery: %+v", out)
	}
	b := out.Bindings[0]
	response = performJSON(t, handler, "/api/v1/rp/sessions/open", rpPlayerToken, core.RPSessionOpenRequest{InstanceID: b.InstanceID, BranchID: b.BranchID, EntityID: b.EntityID, POV: "second_person", IdempotencyKey: "discover-open"})
	assertStatus(t, response, http.StatusOK)
	session := decodeData[storage.RPSession](t, response)
	response = performJSON(t, handler, "/api/v1/rp/observe", rpPlayerToken, core.RPSessionReadRequest{SessionID: session.SessionID})
	assertStatus(t, response, http.StatusOK)
	view := decodeData[storage.RPObservation](t, response)
	if view.ObservationCursor < 1 || session.ControlledEntityID != b.EntityID {
		t.Fatal("discovery session did not observe actual world")
	}
}
