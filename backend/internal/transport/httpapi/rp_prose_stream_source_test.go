package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/narrative"
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
	var factCount atomic.Int32
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if unavailable.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("private upstream failure details"))
			return
		}
		var request struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || len(request.Messages) < 2 {
			t.Error("missing narrative provider input")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var input struct {
			Facts []struct {
				Ref string `json:"fact_ref"`
			} `json:"facts"`
		}
		if err := json.Unmarshal([]byte(request.Messages[1].Content), &input); err != nil || len(input.Facts) == 0 {
			t.Error("missing authorized fact references")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		// Separate paragraphs must retain their own source set, rather than
		// claiming every source for every line in a multi-fact narrative.
		groups := make([]map[string]any, 0, len(input.Facts))
		for _, fact := range input.Facts {
			groups = append(groups, map[string]any{
				"layout": "inline", "atoms": []any{map[string]string{"fact_ref": fact.Ref, "template": "plain"}},
			})
		}
		factCount.Store(int32(len(input.Facts)))
		content, err := json.Marshal(map[string]any{"version": "corerp.fact-composition.v1", "groups": groups})
		if err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{
			"finish_reason": "stop", "message": map[string]string{"content": string(content)},
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
	wantSources := append([]string{turn.PlayerEventID}, turn.NPCEventIDs...)
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
	var firstChunks []core.RPNarrativeChunk
	var firstGroups [][]string
	seenRefs := make(map[string]bool)
	var count int
	for _, raw := range strings.Split(strings.TrimSpace(response.Body.String()), "\n") {
		var frame struct {
			Type               string                `json:"type"`
			Chunk              core.RPNarrativeChunk `json:"chunk"`
			Count              int                   `json:"count"`
			EventIDs           []string              `json:"event_ids"`
			FallbackReason     string                `json:"fallback_reason"`
			CompositionVersion string                `json:"composition_version"`
			FactGroups         [][]string            `json:"fact_groups"`
			Warnings           []string              `json:"warnings"`
		}
		if err := json.Unmarshal([]byte(raw), &frame); err != nil {
			t.Fatal(err)
		}
		switch frame.Type {
		case "line":
			if frame.Chunk.EventID != "" || len(frame.Chunk.EventIDs) != 1 || frame.Chunk.Index != count {
				t.Fatalf("synthesized prose lied about single-event provenance: %+v", frame.Chunk)
			}
			if seenRefs[frame.Chunk.EventIDs[0]] {
				t.Fatal("separate fact paragraphs duplicate a source")
			}
			seenRefs[frame.Chunk.EventIDs[0]] = true
			refs = append(refs, frame.Chunk.EventIDs...)
			firstChunks = append(firstChunks, frame.Chunk)
			count++
		case "done":
			if count == 0 || frame.Count != count || frame.FallbackReason != "" || strings.Join(frame.EventIDs, ",") != strings.Join(refs, ",") {
				t.Fatalf("prose completion mismatched source references: %+v", frame)
			}
			if !reflect.DeepEqual(frame.EventIDs, wantSources) || frame.CompositionVersion != "corerp.fact-composition.v1" || len(frame.FactGroups) != count {
				t.Fatalf("completion lost independent committed source/version evidence: %+v want=%v", frame, wantSources)
			}
			for i, group := range frame.FactGroups {
				if !reflect.DeepEqual(group, refs[i:i+1]) {
					t.Fatalf("completion group differs from its paragraph: %+v", frame)
				}
			}
			firstGroups = frame.FactGroups
			if !reflect.DeepEqual(frame.Warnings, []string{narrative.CompositionCapabilityWarning}) {
				t.Fatalf("fresh composition capability was not disclosed: %+v", frame.Warnings)
			}
		default:
			t.Fatalf("unexpected stream frame %q", frame.Type)
		}
	}
	if count != int(factCount.Load()) {
		t.Fatalf("stream lost fact coverage: paragraphs=%d facts=%d", count, factCount.Load())
	}
	if calls.Load() != 1 {
		t.Fatalf("provider was not called exactly once: %d", calls.Load())
	}
	cached := performJSON(t, handler, "/api/v1/rp/narrative/stream", rpPlayerToken, request)
	assertStatus(t, cached, http.StatusOK)
	cachedCount, cachedDone := 0, false
	for _, raw := range strings.Split(strings.TrimSpace(cached.Body.String()), "\n") {
		var frame struct {
			Type               string                `json:"type"`
			Chunk              core.RPNarrativeChunk `json:"chunk"`
			CompositionVersion string                `json:"composition_version"`
			EventIDs           []string              `json:"event_ids"`
			FactGroups         [][]string            `json:"fact_groups"`
			Warnings           []string              `json:"warnings"`
		}
		if err := json.Unmarshal([]byte(raw), &frame); err != nil {
			t.Fatal(err)
		}
		switch frame.Type {
		case "line":
			if cachedCount >= len(firstChunks) || !reflect.DeepEqual(frame.Chunk, firstChunks[cachedCount]) {
				t.Fatalf("saved stream changed public text or source attribution: %+v", frame.Chunk)
			}
			cachedCount++
		case "done":
			cachedDone = true
			if frame.CompositionVersion != "corerp.fact-composition.v1" || !reflect.DeepEqual(frame.EventIDs, wantSources) || !reflect.DeepEqual(frame.FactGroups, firstGroups) || !reflect.DeepEqual(frame.Warnings, []string{narrative.CompositionCapabilityWarning}) {
				t.Fatalf("saved completion changed composition disclosure or independent sources: %+v", frame)
			}
		default:
			t.Fatalf("unexpected cached frame %q", frame.Type)
		}
	}
	if !cachedDone || cachedCount != count || calls.Load() != 1 {
		t.Fatalf("saved narrative was regenerated or incomplete: count=%d calls=%d", cachedCount, calls.Load())
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
