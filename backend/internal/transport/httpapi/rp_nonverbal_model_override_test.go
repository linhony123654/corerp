package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/endpointpolicy"
	"corerp.local/backend/internal/storage"
)

func TestRPNonverbalModelOverrideReactionAuthorizationAndReplay(t *testing.T) {
	ctx := context.Background()
	var calls atomic.Int32
	const key = "nonverbal-request-only-secret"
	const words = "我亲眼看见你向我点头了。"
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var request struct {
			Model    string `json:"model"`
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
			ReasoningEffort string `json:"reasoning_effort"`
			MaxTokens       int    `json:"max_completion_tokens"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		if request.Model != "nod-selected-model" || r.Header.Get("Authorization") != "Bearer "+key || request.ReasoningEffort != "low" || request.MaxTokens != 4096 {
			t.Error("selected override/tuning did not reach nod provider")
		}
		var packet struct {
			Character struct {
				CurrentTurn struct {
					Data struct {
						Speech      string                         `json:"player_speech_text"`
						SpeechEvent string                         `json:"speech_event_id"`
						Action      *core.RPDecisionObservedAction `json:"observed_player_action"`
					} `json:"data"`
				} `json:"current_turn"`
			} `json:"character"`
		}
		if len(request.Messages) != 2 || json.Unmarshal([]byte(request.Messages[1].Content), &packet) != nil {
			t.Error("missing grouped character packet")
		}
		action := packet.Character.CurrentTurn.Data.Action
		if action == nil || action.Action != "nod" || action.SourceEventID == "" || packet.Character.CurrentTurn.Data.Speech != "" || packet.Character.CurrentTurn.Data.SpeechEvent != "" {
			t.Error("nod lacked witnessed action or invented speech")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]any{"content": `{"private":{"intent":"回应点头","emotion":"平和","relationship_stance":"礼貌","basis_event_ids":[]},"observable":{"action":"respond","text":"` + words + `","introduce_self":false,"expression_code":"none","speech_tone":"gentle"}}`}}}})
	}))
	defer model.Close()
	path := filepath.Join(t.TempDir(), "nod-model-override.db")
	store, handler := openHTTPTestServer(t, ctx, path)
	defer store.Close()
	policy, err := endpointpolicy.FromEnvironment("", model.URL+"/v1/chat/completions")
	if err != nil {
		t.Fatal(err)
	}
	handler.(*Server).endpointPolicy = policy
	session := openAuthoredRPModelHTTPSession(t, ctx, store, handler)
	response := performJSON(t, handler, "/api/v1/rp/observe", rpPlayerToken, core.RPSessionReadRequest{SessionID: session.SessionID})
	assertStatus(t, response, http.StatusOK)
	initial := decodeData[storage.RPObservation](t, response)
	// Resolve the actual visible authored target; never hardcode a raw unseen ID.
	if len(initial.PresentEntities) != 1 {
		t.Fatalf("fixture requires one target: %+v", initial.PresentEntities)
	}
	request := map[string]any{"session_id": session.SessionID, "expected_cursor": initial.ObservationCursor, "idempotency_key": "nod-override", "action": "nod", "target_entity_id": initial.PresentEntities[0].EntityID, "model": map[string]any{"endpoint": model.URL + "/v1/chat/completions", "model": "nod-selected-model", "api_key": key, "reasoning_effort": "low", "decision_max_tokens": 4096, "decision_format": "json_object"}}
	response = performJSON(t, handler, "/api/v1/rp/actions/nonverbal", "", request)
	assertAPIError(t, response, http.StatusUnauthorized, core.CodeUnauthenticated)
	response = performJSON(t, handler, "/api/v1/rp/actions/nonverbal", creatorToken, request)
	assertAPIError(t, response, http.StatusNotFound, core.CodeNotFound)
	if calls.Load() != 0 {
		t.Fatal("unauthorized request invoked provider")
	}
	response = performJSON(t, handler, "/api/v1/rp/actions/nonverbal", rpPlayerToken, request)
	assertStatus(t, response, http.StatusOK)
	result := decodeData[storage.RPNonverbalResult](t, response)
	if result.Turn == nil || result.Turn.Status != "settled" || !strings.Contains(strings.Join(result.Turn.NarrativeLines, "\n"), words) || calls.Load() != 1 {
		t.Fatalf("override failed to react once: %+v calls=%d", result, calls.Load())
	}
	if strings.Contains(response.Body.String(), key) {
		t.Fatal("response leaked model key")
	}
	response = performJSON(t, handler, "/api/v1/rp/actions/nonverbal", rpPlayerToken, request)
	assertStatus(t, response, http.StatusOK)
	replay := decodeData[storage.RPNonverbalResult](t, response)
	if !replay.Replayed || replay.Turn == nil || !replay.Turn.Replayed || replay.EventID != result.EventID || calls.Load() != 1 {
		t.Fatal("nod retry repeated model/action", replay, calls.Load())
	}
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var leaked, playerUtterances int
	if err := db.QueryRowContext(ctx, `SELECT (SELECT COUNT(*) FROM rp_turn_runs WHERE instr(request_json,?)>0)+(SELECT COUNT(*) FROM events WHERE instr(payload,?)>0)+(SELECT COUNT(*) FROM audit_records WHERE instr(payload,?)>0)`, key, key, key).Scan(&leaked); err != nil {
		t.Fatal(err)
	}
	if leaked != 0 {
		t.Fatal("per-request key persisted in action/turn/audit facts")
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_utterances WHERE speaker_entity_id=?`, session.ControlledEntityID).Scan(&playerUtterances); err != nil {
		t.Fatal(err)
	}
	if playerUtterances != 0 {
		t.Fatal("nod synthesized player speech")
	}
}

func TestRPNonverbalModelOverrideRejectsBeforeAction(t *testing.T) {
	ctx := context.Background()
	var calls atomic.Int32
	model := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	defer model.Close()
	path := filepath.Join(t.TempDir(), "nod-invalid-override.db")
	store, handler := openHTTPTestServer(t, ctx, path)
	defer store.Close()
	policy, err := endpointpolicy.FromEnvironment("", model.URL+"/v1/chat/completions")
	if err != nil {
		t.Fatal(err)
	}
	handler.(*Server).endpointPolicy = policy
	session := openAuthoredRPModelHTTPSession(t, ctx, store, handler)
	response := performJSON(t, handler, "/api/v1/rp/observe", rpPlayerToken, core.RPSessionReadRequest{SessionID: session.SessionID})
	assertStatus(t, response, http.StatusOK)
	initial := decodeData[storage.RPObservation](t, response)
	if len(initial.PresentEntities) != 1 {
		t.Fatal("missing authored target")
	}
	for name, override := range map[string]map[string]any{
		"endpoint": {"endpoint": "http://127.0.0.1:1/v1/chat/completions", "model": "m", "api_key": "fixture-key"},
		"remote":   {"endpoint": "http://203.0.113.10/v1/chat/completions", "model": "m", "api_key": "fixture-key"},
		"format":   {"endpoint": model.URL + "/v1/chat/completions", "model": "m", "api_key": "fixture-key", "decision_format": "text"},
		"effort":   {"endpoint": model.URL + "/v1/chat/completions", "model": "m", "api_key": "fixture-key", "reasoning_effort": "invalid"},
		"budget":   {"endpoint": model.URL + "/v1/chat/completions", "model": "m", "api_key": "fixture-key", "decision_max_tokens": 8193},
	} {
		response = performJSON(t, handler, "/api/v1/rp/actions/nonverbal", rpPlayerToken, map[string]any{"session_id": session.SessionID, "expected_cursor": initial.ObservationCursor, "idempotency_key": "invalid-nod-" + name, "action": "nod", "target_entity_id": initial.PresentEntities[0].EntityID, "model": override})
		assertAPIError(t, response, http.StatusBadRequest, core.CodeInvalidArgument)
	}
	response = performJSON(t, handler, "/api/v1/rp/observe", rpPlayerToken, core.RPSessionReadRequest{SessionID: session.SessionID})
	assertStatus(t, response, http.StatusOK)
	after := decodeData[storage.RPObservation](t, response)
	if after.ObservationCursor != initial.ObservationCursor || calls.Load() != 0 {
		t.Fatal("invalid override committed action or called provider")
	}
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var runs int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_turn_runs WHERE session_id=?`, session.SessionID).Scan(&runs); err != nil {
		t.Fatal(err)
	}
	if runs != 0 {
		t.Fatal("invalid override pinned an action intent")
	}
}
