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

func TestRPStyleHTTPPermissionsAndFactPreservingView(t *testing.T) {
	ctx := context.Background()
	s, handler := openHTTPTestServer(t, ctx, filepath.Join(t.TempDir(), "style-http.db"))
	defer s.Close()
	if _, err := s.PrepareRPTravel(ctx); err != nil {
		t.Fatal(err)
	}
	response := performJSON(t, handler, "/api/v1/rp/sessions/open", rpPlayerToken, core.RPSessionOpenRequest{InstanceID: storage.M2DemoInstanceID, BranchID: storage.M2DemoBranchID, EntityID: storage.M2RPPlayerID, POV: "second_person", IdempotencyKey: "style-http"})
	assertStatus(t, response, http.StatusOK)
	session := decodeData[storage.RPSession](t, response)
	read := core.RPSessionReadRequest{SessionID: session.SessionID}
	first := "first_person"
	setting := storage.RPStyleSetRequest{InstanceID: storage.M2DemoInstanceID, BranchID: storage.M2DemoBranchID, Scope: "world", IdempotencyKey: "world-style", Patch: core.RPStylePatch{POV: &first}}
	response = performJSON(t, handler, "/api/v1/rp/style/set", rpPlayerToken, setting)
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)
	response = performJSON(t, handler, "/api/v1/rp/style/set", creatorToken, setting)
	assertStatus(t, response, http.StatusOK)
	setting.Scope = "session"
	setting.SessionID = session.SessionID
	setting.IdempotencyKey = "session-style"
	response = performJSON(t, handler, "/api/v1/rp/style/set", rpPlayerToken, setting)
	assertStatus(t, response, http.StatusOK)
	response = performJSON(t, handler, "/api/v1/rp/style/read", rpPlayerToken, read)
	assertStatus(t, response, http.StatusOK)
	if decodeData[storage.RPResolvedStyle](t, response).Profile.POV != first {
		t.Fatal("HTTP style not applied")
	}
	response = performJSON(t, handler, "/api/v1/rp/observe", rpPlayerToken, read)
	assertStatus(t, response, http.StatusOK)
	view := decodeData[storage.RPObservation](t, response)
	response = performJSON(t, handler, "/api/v1/rp/turns/run", rpPlayerToken, core.RPSpeechRequest{SessionID: session.SessionID, ExpectedCursor: view.ObservationCursor, Text: "你好", IdempotencyKey: "styled-http-turn"})
	assertStatus(t, response, http.StatusOK)
	turn := decodeData[storage.RPTurnResult](t, response)
	if !strings.HasPrefix(turn.NarrativeLines[0], "我说") {
		t.Fatal("style not used by actual turn")
	}
	second := "second_person"
	render := storage.RPNarrativeReadRequest{SessionID: session.SessionID, TurnRunID: turn.TurnRunID, StyleOverride: &core.RPStylePatch{POV: &second}}
	response = performJSON(t, handler, "/api/v1/rp/narrative/render", creatorToken, render)
	assertAPIError(t, response, http.StatusNotFound, core.CodeNotFound)
	response = performJSON(t, handler, "/api/v1/rp/narrative/render", rpPlayerToken, render)
	assertStatus(t, response, http.StatusOK)
	variant := decodeData[storage.RPNarrativeReadResult](t, response)
	if !strings.HasPrefix(variant.View.Lines[0], "你说") || variant.View.EventIDs[0] != turn.PlayerEventID {
		t.Fatal("variant lacks same attributed event")
	}
}
