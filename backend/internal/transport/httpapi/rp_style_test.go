package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"reflect"
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
	style := decodeData[storage.RPResolvedStyle](t, response)
	if style.Profile.POV != first || style.SessionRevision == nil || *style.SessionRevision != 1 {
		t.Fatal("HTTP style not applied")
	}
	response = performJSON(t, handler, "/api/v1/rp/observe", rpPlayerToken, read)
	assertStatus(t, response, http.StatusOK)
	view := decodeData[storage.RPObservation](t, response)
	response = performJSON(t, handler, "/api/v1/rp/turns/run", rpPlayerToken, core.RPSpeechRequest{SessionID: session.SessionID, ExpectedCursor: view.ObservationCursor, Text: "你好", IdempotencyKey: "styled-http-turn"})
	assertStatus(t, response, http.StatusOK)
	turn := decodeData[storage.RPTurnResult](t, response)
	response = performJSON(t, handler, "/api/v1/rp/observe", rpPlayerToken, read)
	assertStatus(t, response, http.StatusOK)
	observed := decodeData[storage.RPObservation](t, response)
	if len(observed.RecentTurns) == 0 || !observed.RecentTurns[len(observed.RecentTurns)-1].CanRegenerate {
		t.Fatal("settled dialogue must advertise read-only regeneration")
	}
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
	if variant.View.RenderID == "" {
		t.Fatal("successful render did not return durable version ID")
	}
	if variant.Style.SessionRevision != nil {
		t.Fatal("live revision leaked into pinned narrative style")
	}
	if !strings.HasPrefix(variant.View.Lines[0], "你说") || variant.View.EventIDs[0] != turn.PlayerEventID {
		t.Fatal("variant lacks same attributed event")
	}
	response = performJSON(t, handler, "/api/v1/rp/narrative/stream", creatorToken, render)
	assertAPIError(t, response, http.StatusNotFound, core.CodeNotFound)
	if response.Flushed {
		t.Fatal("unauthorized stream emitted content")
	}
	response = performJSON(t, handler, "/api/v1/rp/narrative/stream", rpPlayerToken, render)
	assertStatus(t, response, http.StatusOK)
	if !response.Flushed || !strings.HasPrefix(response.Header().Get("Content-Type"), "application/x-ndjson") || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("narrative stream not flushed/private NDJSON")
	}
	frames := strings.Split(strings.TrimSpace(response.Body.String()), "\n")
	if len(frames) != len(variant.View.Lines)+1 {
		t.Fatal("missing line or completion frame")
	}
	for i, raw := range frames {
		var frame struct {
			Type     string                `json:"type"`
			Chunk    core.RPNarrativeChunk `json:"chunk"`
			Count    int                   `json:"count"`
			EventIDs []string              `json:"event_ids"`
		}
		if err := json.Unmarshal([]byte(raw), &frame); err != nil {
			t.Fatal(err)
		}
		if i == len(variant.View.Lines) {
			if frame.Type != "done" || frame.Count != i || len(frame.EventIDs) != len(variant.View.EventIDs) {
				t.Fatal("invalid completion marker or source set")
			}
			for j, id := range frame.EventIDs {
				if id != variant.View.EventIDs[j] {
					t.Fatal("completion source differs from accepted facts")
				}
			}
		} else if frame.Type != "line" || frame.Chunk.Index != i || frame.Chunk.Line != variant.View.Lines[i] || frame.Chunk.EventID != "" || !reflect.DeepEqual(frame.Chunk.EventIDs, variant.View.FactGroups[i]) {
			t.Fatal("stream lost ordering or attributed fact")
		}
	}
	wantSources := append([]string{turn.PlayerEventID}, turn.NPCEventIDs...)
	var flattened []string
	for _, group := range variant.View.FactGroups {
		flattened = append(flattened, group...)
	}
	if variant.View.CompositionVersion != core.RPFactCompositionVersionV2 || !reflect.DeepEqual(flattened, wantSources) || !reflect.DeepEqual(variant.View.EventIDs, wantSources) || !strings.Contains(variant.View.Lines[0], "「你好」") {
		t.Fatalf("styled v2 lost ordered committed sources or player quote: %+v", variant.View)
	}
	selection := storage.RPNarrativeSelectRequest{SessionID: session.SessionID, TurnRunID: turn.TurnRunID, RenderID: variant.View.RenderID}
	response = performJSON(t, handler, "/api/v1/rp/narrative/select", creatorToken, selection)
	assertAPIError(t, response, http.StatusNotFound, core.CodeNotFound)
	response = performJSON(t, handler, "/api/v1/rp/narrative/select", rpPlayerToken, selection)
	assertStatus(t, response, http.StatusOK)
	chosen := decodeData[storage.RPNarrativeSelectResult](t, response)
	if chosen.RenderID != variant.View.RenderID || strings.Join(chosen.Lines, "\n") != strings.Join(variant.View.Lines, "\n") {
		t.Fatal("selected render did not return saved lines")
	}
	response = performJSON(t, handler, "/api/v1/rp/observe", rpPlayerToken, read)
	assertStatus(t, response, http.StatusOK)
	observed = decodeData[storage.RPObservation](t, response)
	last := observed.RecentTurns[len(observed.RecentTurns)-1]
	if last.RenderID != selection.RenderID || strings.Join(last.NarrativeLines, "\n") != strings.Join(chosen.Lines, "\n") {
		t.Fatal("observe did not use selected render")
	}
	selection.RenderID = ""
	response = performJSON(t, handler, "/api/v1/rp/narrative/select", rpPlayerToken, selection)
	assertStatus(t, response, http.StatusOK)
	restored := decodeData[storage.RPNarrativeSelectResult](t, response)
	if strings.Join(restored.Lines, "\n") != strings.Join(turn.NarrativeLines, "\n") {
		t.Fatal("restore did not return canonical turn narrative")
	}
}
