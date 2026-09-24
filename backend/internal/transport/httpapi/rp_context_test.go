package httpapi

import (
	"context"
	"net/http"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/storage"
)

func TestRPClientContextHTTPAuthenticatedNarrowRead(t *testing.T) {
	ctx := context.Background()
	s, handler := openHTTPTestServer(t, ctx, filepath.Join(t.TempDir(), "context-http.db"))
	defer s.Close()
	if _, err := s.PrepareRPTravel(ctx); err != nil {
		t.Fatal(err)
	}
	response := performJSON(t, handler, "/api/v1/rp/sessions/open", rpPlayerToken, core.RPSessionOpenRequest{InstanceID: storage.M2DemoInstanceID, BranchID: storage.M2DemoBranchID, EntityID: storage.M2RPPlayerID, POV: "second_person", IdempotencyKey: "context-http"})
	assertStatus(t, response, http.StatusOK)
	session := decodeData[storage.RPSession](t, response)
	read := storage.RPContextReadRequest{SessionID: session.SessionID}
	const route = "/api/v1/rp/context/read"
	response = performJSON(t, handler, route, rpPlayerToken, read)
	assertStatus(t, response, http.StatusOK)
	out := decodeData[storage.RPClientContext](t, response)
	if out.ProtocolVersion != storage.RPClientProtocolVersion || out.ObserverEntityID != storage.M2RPPlayerID || out.Facts == nil || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("missing bounded private context envelope")
	}
	response = performJSON(t, handler, "/api/v1/rp/observe", rpPlayerToken, core.RPSessionReadRequest{SessionID: session.SessionID})
	assertStatus(t, response, http.StatusOK)
	view := decodeData[storage.RPObservation](t, response)
	response = performJSON(t, handler, "/api/v1/rp/turns/run", rpPlayerToken, core.RPSpeechRequest{SessionID: session.SessionID, ExpectedCursor: view.ObservationCursor, IdempotencyKey: "context-http-hearing", Text: "你好。"})
	assertStatus(t, response, http.StatusOK)
	response = performJSON(t, handler, route, rpPlayerToken, read)
	assertStatus(t, response, http.StatusOK)
	out = decodeData[storage.RPClientContext](t, response)
	heard := false
	for _, fact := range out.Facts {
		heard = heard || fact.Kind == "speaker_said" && fact.SubjectEntityID == storage.M2RPNPCID && fact.Text != "" && fact.SourceEventID != ""
	}
	if !heard {
		t.Fatal("actual HTTP dialogue did not enter own context")
	}
	response = performJSON(t, handler, route, "", read)
	assertStatus(t, response, http.StatusUnauthorized)
	response = performJSON(t, handler, route, creatorToken, read)
	assertAPIError(t, response, http.StatusNotFound, core.CodeNotFound)
	read.PrincipalID = "principal_creator"
	response = performJSON(t, handler, route, rpPlayerToken, read)
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)
	response = performJSON(t, handler, route, rpPlayerToken, map[string]any{"session_id": session.SessionID, "observer_entity_id": storage.M2RPNPCID})
	assertStatus(t, response, http.StatusBadRequest)
}
