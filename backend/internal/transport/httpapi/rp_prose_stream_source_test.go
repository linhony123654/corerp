package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/storage"
)

func TestRPFullProseStreamCitesBoundedFactsAndReceipts(t *testing.T) {
	ctx := context.Background()
	store, handler := openHTTPTestServer(t, ctx, filepath.Join(t.TempDir(), "prose-stream.db"))
	defer store.Close()
	if _, err := store.PrepareRPTravel(ctx); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	var unavailable atomic.Bool
	var content string
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if unavailable.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("private upstream failure details"))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{
			"finish_reason": "stop", "message": map[string]string{"content": content},
		}}})
	}))
	defer model.Close()
	open := performJSON(t, handler, "/api/v1/rp/sessions/open", rpPlayerToken, core.RPSessionOpenRequest{
		InstanceID: storage.M2DemoInstanceID, BranchID: storage.M2DemoBranchID,
		EntityID: storage.M2RPPlayerID, POV: "second_person", IdempotencyKey: "prose-source-session",
	})
	assertStatus(t, open, http.StatusOK)
	session := decodeData[storage.RPSession](t, open)
	read := core.RPSessionReadRequest{SessionID: session.SessionID}
	observed := decodeData[storage.RPObservation](t, performJSON(t, handler, "/api/v1/rp/observe", rpPlayerToken, read))
	turnResponse := performJSON(t, handler, "/api/v1/rp/turns/run", rpPlayerToken, core.RPSpeechRequest{
		SessionID: session.SessionID, ExpectedCursor: observed.ObservationCursor, Text: "你好", IdempotencyKey: "prose-source-turn",
	})
	assertStatus(t, turnResponse, http.StatusOK)
	turn := decodeData[storage.RPTurnResult](t, turnResponse)
	content = strings.Join(turn.NarrativeLines, "\n")
	endpoint := model.URL + "/v1/chat/completions"
	handler.(*Server).endpointPolicy = handler.(*Server).endpointPolicy.WithLocalOperatorEndpoint(endpoint)
	request := map[string]any{
		"session_id": session.SessionID, "turn_run_id": turn.TurnRunID,
		"model": map[string]any{"endpoint": endpoint, "model": "prose-fixture", "api_key": "private-prose-test-key", "full_prose": true},
	}
	unauthorized := performJSON(t, handler, "/api/v1/rp/narrative/stream", creatorToken, request)
	assertAPIError(t, unauthorized, http.StatusNotFound, core.CodeNotFound)
	if unauthorized.Flushed || calls.Load() != 0 {
		t.Fatal("unauthorized actor received prose stream or triggered provider")
	}
	response := performJSON(t, handler, "/api/v1/rp/narrative/stream", rpPlayerToken, request)
	assertStatus(t, response, http.StatusOK)
	if !response.Flushed || !strings.HasPrefix(response.Header().Get("Content-Type"), "application/x-ndjson") {
		t.Fatal("prose source stream not flushed as NDJSON")
	}
	var refs []string
	var count int
	for _, raw := range strings.Split(strings.TrimSpace(response.Body.String()), "\n") {
		var frame struct {
			Type           string                `json:"type"`
			Chunk          core.RPNarrativeChunk `json:"chunk"`
			Count          int                   `json:"count"`
			EventIDs       []string              `json:"event_ids"`
			FallbackReason string                `json:"fallback_reason"`
		}
		if err := json.Unmarshal([]byte(raw), &frame); err != nil {
			t.Fatal(err)
		}
		switch frame.Type {
		case "line":
			if frame.Chunk.EventID != "" || len(frame.Chunk.EventIDs) == 0 || frame.Chunk.Index != count {
				t.Fatalf("synthesized prose lied about single-event provenance: %+v", frame.Chunk)
			}
			if refs == nil {
				refs = frame.Chunk.EventIDs
			} else if strings.Join(frame.Chunk.EventIDs, ",") != strings.Join(refs, ",") {
				t.Fatal("paragraphs cite inconsistent fact sets")
			}
			count++
		case "done":
			if count == 0 || frame.Count != count || frame.FallbackReason != "" || strings.Join(frame.EventIDs, ",") != strings.Join(refs, ",") {
				t.Fatalf("prose completion mismatched source references: %+v", frame)
			}
		default:
			t.Fatalf("unexpected stream frame %q", frame.Type)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("provider was not called exactly once: %d", calls.Load())
	}
	observed = decodeData[storage.RPObservation](t, performJSON(t, handler, "/api/v1/rp/observe", rpPlayerToken, read))
	if len(observed.RecentTurns) != 1 {
		t.Fatalf("scoped history missing: %+v", observed.RecentTurns)
	}
	var found bool
	for _, receipt := range observed.RecentTurns[0].ProviderCalls {
		if receipt.Phase == "narrative" && receipt.ProviderKind == "full_prose" && receipt.ModelID == "prose-fixture" && receipt.Attempted && receipt.AttemptCount == 1 && receipt.Result == "success" && receipt.RenderSource == "live_prose" {
			found = true
		}
	}
	if !found {
		t.Fatalf("model success incorrectly attributed or absent: %+v", observed.RecentTurns[0].ProviderCalls)
	}
	if strings.Contains(response.Body.String(), "private-prose-test-key") {
		t.Fatal("private model credential leaked to narrative stream")
	}
	unavailable.Store(true)
	request["style_override"] = map[string]any{"pov": "second_person"}
	fallback := performJSON(t, handler, "/api/v1/rp/narrative/stream", rpPlayerToken, request)
	assertStatus(t, fallback, http.StatusOK)
	if strings.Contains(fallback.Body.String(), "private upstream failure details") || strings.Contains(fallback.Body.String(), "private-prose-test-key") {
		t.Fatal("upstream error or credential escaped in fallback")
	}
	var completed bool
	for _, raw := range strings.Split(strings.TrimSpace(fallback.Body.String()), "\n") {
		var frame struct {
			Type           string                `json:"type"`
			Chunk          core.RPNarrativeChunk `json:"chunk"`
			FallbackReason string                `json:"fallback_reason"`
		}
		if err := json.Unmarshal([]byte(raw), &frame); err != nil {
			t.Fatal(err)
		}
		if frame.Type == "line" && (frame.Chunk.EventID == "" || len(frame.Chunk.EventIDs) != 0) {
			t.Fatalf("fallback lost deterministic attribution: %+v", frame.Chunk)
		}
		if frame.Type == "done" {
			completed = true
			if frame.FallbackReason != "prose_unavailable" {
				t.Fatalf("failure rendered as model success: %+v", frame)
			}
		}
	}
	if !completed {
		t.Fatal("fallback stream lacks completion evidence")
	}
	again := decodeData[storage.RPObservation](t, performJSON(t, handler, "/api/v1/rp/observe", rpPlayerToken, read))
	if again.ObservationCursor != observed.ObservationCursor {
		t.Fatal("narrative fallback changed world head")
	}
	var failed bool
	for _, receipt := range again.RecentTurns[0].ProviderCalls {
		if receipt.Phase == "narrative" && receipt.ProviderKind == "full_prose" && receipt.Attempted && receipt.AttemptCount == 2 && receipt.Result == "failed" && receipt.FallbackKind == "prose_unavailable" && receipt.RenderSource == "template" {
			failed = true
		}
	}
	if !failed {
		t.Fatalf("narrative technical fallback untraceable: %+v", again.RecentTurns[0].ProviderCalls)
	}
}
