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
	"time"

	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/decision"
	"corerp.local/backend/internal/endpointpolicy"
	"corerp.local/backend/internal/storage"
)

// A player-supplied model override must reach the decision provider as an
// in-memory per-request configuration: the turn uses the supplied endpoint,
// model and credential, while a request without an override stays on the
// operator default and never touches the player endpoint.
func TestRPModelOverrideRoutesTurnToPlayerEndpoint(t *testing.T) {
	ctx := context.Background()
	var calls atomic.Int32
	var gotModel, gotAuth atomic.Value
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var body struct {
			Model           string `json:"model"`
			ReasoningEffort string `json:"reasoning_effort"`
			EnableThinking  *bool  `json:"enable_thinking"`
			MaxTokens       int    `json:"max_completion_tokens"`
			ResponseFormat  struct {
				Type string `json:"type"`
			} `json:"response_format"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.ReasoningEffort != "low" || body.EnableThinking == nil || *body.EnableThinking || body.MaxTokens != 4096 || body.ResponseFormat.Type != "json_object" {
			t.Error("player decision tuning did not reach the model request")
		}
		gotModel.Store(body.Model)
		gotAuth.Store(r.Header.Get("Authorization"))
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{
			"finish_reason": "stop",
			"message":       map[string]any{"content": `{"private":{"intent":"回应来访者","emotion":"平和","relationship_stance":"礼貌","basis_event_ids":[]},"observable":{"action":"respond","text":"这是覆盖模型的原话。","speech_tone":"none","introduce_self":false,"expression_code":"none"}}`},
		}}})
	}))
	defer model.Close()
	store, handler := openHTTPTestServer(t, ctx, filepath.Join(t.TempDir(), "rp-model-override.db"))
	defer store.Close()
	api := handler.(*Server)
	// This exact local origin represents the operator's explicit fixture
	// allowlist; player overrides cannot choose other destinations.
	policy, err := endpointpolicy.FromEnvironment("", model.URL+"/v1/chat/completions")
	if err != nil {
		t.Fatal(err)
	}
	api.endpointPolicy = policy
	session := openAuthoredRPModelHTTPSession(t, ctx, store, handler)
	response := performJSON(t, handler, "/api/v1/rp/observe", rpPlayerToken, core.RPSessionReadRequest{SessionID: session.SessionID})
	assertStatus(t, response, http.StatusOK)
	initial := decodeData[storage.RPObservation](t, response)

	turn := map[string]any{
		"session_id": session.SessionID, "text": "你好，Cai。", "expected_cursor": initial.ObservationCursor,
		"idempotency_key": "http-override-turn",
		"model":           map[string]any{"endpoint": model.URL + "/v1/chat/completions", "model": "player-model", "api_key": "player-key", "timeout_seconds": 120, "reasoning_effort": "low", "disable_thinking": true, "decision_max_tokens": 4096, "interaction_max_tokens": 3072, "decision_format": "json_object"},
	}
	response = performJSON(t, handler, "/api/v1/rp/turns/run", rpPlayerToken, turn)
	assertStatus(t, response, http.StatusOK)
	result := decodeData[storage.RPTurnResult](t, response)
	if result.Status != "settled" || !strings.Contains(strings.Join(result.NarrativeLines, "\n"), "这是覆盖模型的原话。") {
		t.Fatalf("override turn did not use player model text: %+v", result)
	}
	if calls.Load() == 0 || gotModel.Load() != "player-model" || gotAuth.Load() != "Bearer player-key" {
		t.Fatalf("player endpoint did not receive the override decision: calls=%d model=%v auth=%v", calls.Load(), gotModel.Load(), gotAuth.Load())
	}

	// A replayed turn must not re-charge the player endpoint.
	response = performJSON(t, handler, "/api/v1/rp/turns/run", rpPlayerToken, turn)
	assertStatus(t, response, http.StatusOK)
	if got := decodeData[storage.RPTurnResult](t, response); !got.Replayed {
		t.Fatalf("override turn retry was not a replay: %+v", got)
	}
	if calls.Load() != 1 {
		t.Fatalf("replayed turn called the player endpoint again: %d", calls.Load())
	}

	// Without the override the operator default (deterministic) runs instead.
	response = performJSON(t, handler, "/api/v1/rp/observe", rpPlayerToken, core.RPSessionReadRequest{SessionID: session.SessionID})
	assertStatus(t, response, http.StatusOK)
	after := decodeData[storage.RPObservation](t, response)
	response = performJSON(t, handler, "/api/v1/rp/turns/run", rpPlayerToken, map[string]any{
		"session_id": session.SessionID, "text": "再说一句。", "expected_cursor": after.ObservationCursor, "idempotency_key": "http-default-turn",
	})
	assertStatus(t, response, http.StatusOK)
	if calls.Load() != 1 {
		t.Fatalf("default turn reached the player endpoint: %d", calls.Load())
	}
}

func TestRPModelOverrideRejectsInvalidDecisionTuning(t *testing.T) {
	for _, tc := range []struct {
		name      string
		configure func(*core.RPModelOverride)
	}{
		{"effort", func(o *core.RPModelOverride) { o.ReasoningEffort = "unknown" }},
		{"format", func(o *core.RPModelOverride) { o.DecisionFormat = "text" }},
		{"decision low", func(o *core.RPModelOverride) { o.DecisionMaxTokens = 511 }},
		{"decision high", func(o *core.RPModelOverride) { o.DecisionMaxTokens = 8193 }},
		{"interaction low", func(o *core.RPModelOverride) { o.InteractionMaxTokens = 511 }},
		{"interaction high", func(o *core.RPModelOverride) { o.InteractionMaxTokens = 4097 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := core.RPModelOverride{Endpoint: "http://127.0.0.1:1/v1/chat/completions", Model: "fixture"}
			tc.configure(&o)
			if _, err := resolveRPDecisionOverride(&o, endpointpolicy.TestLocalhostPolicy()); err == nil {
				t.Fatal("invalid tuning was accepted")
			}
		})
	}
}

func TestRPModelOverrideTunesAutoInterpretation(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		if string(body["reasoning_effort"]) != `"low"` || string(body["enable_thinking"]) != "false" || string(body["max_completion_tokens"]) != "3072" {
			t.Error("override interpretation lost its explicit tuning")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{
			"finish_reason": "stop",
			"message":       map[string]any{"content": `{"kind":"DIALOGUE","steps":[{"kind":"speech","speech_text":""}],"clarification":""}`},
		}}})
	}))
	defer server.Close()
	p, err := resolveRPDecisionOverride(&core.RPModelOverride{
		Endpoint: server.URL + "/v1/chat/completions", Model: "fixture", ReasoningEffort: "low",
		DisableThinking: true, DecisionMaxTokens: 4096, InteractionMaxTokens: 3072,
	}, endpointpolicy.TestLocalhostPolicy())
	if err != nil {
		t.Fatal(err)
	}
	plan, err := p.(core.RPInteractionUnderstandingProvider).UnderstandInteraction(context.Background(), core.RPInteractionUnderstandingInput{Text: "你好。", Mode: "AUTO"})
	if err != nil || plan.Kind != "DIALOGUE" || len(plan.Steps) != 1 || plan.Steps[0].SpeechText != "你好。" || calls != 1 {
		t.Fatalf("override interpretation failed: %+v %v calls=%d", plan, err, calls)
	}
}

func TestRPModelOverrideTimeoutSharesDecisionBudget(t *testing.T) {
	for _, tc := range []struct {
		seconds int
		want    time.Duration
	}{
		{0, 0}, {60, time.Minute}, {90, 90 * time.Second}, {120, 2 * time.Minute},
		{300, 2 * time.Minute}, {int(^uint(0) >> 1), 2 * time.Minute},
	} {
		if got := rpOverrideTimeout(tc.seconds, decision.MaxRequestTimeout); got != tc.want {
			t.Errorf("timeout %d: got %v want %v", tc.seconds, got, tc.want)
		}
	}
}

// An incomplete or invalid override is an explicit client error, never a
// silent fallback to the operator default.
func TestRPModelOverrideRejectsInvalidConfiguration(t *testing.T) {
	ctx := context.Background()
	var calls atomic.Int32
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
	}))
	defer model.Close()
	store, handler := openHTTPTestServer(t, ctx, filepath.Join(t.TempDir(), "rp-model-invalid.db"))
	defer store.Close()
	handler.(*Server).endpointPolicy = endpointpolicy.Policy{}
	if _, err := store.PrepareRPTravel(ctx); err != nil {
		t.Fatal(err)
	}
	response := performJSON(t, handler, "/api/v1/rp/sessions/open", rpPlayerToken, core.RPSessionOpenRequest{
		InstanceID: storage.M2DemoInstanceID, BranchID: storage.M2DemoBranchID,
		EntityID: storage.M2RPPlayerID, POV: "second_person", IdempotencyKey: "http-invalid-session",
	})
	assertStatus(t, response, http.StatusOK)
	session := decodeData[storage.RPSession](t, response)
	response = performJSON(t, handler, "/api/v1/rp/observe", rpPlayerToken, core.RPSessionReadRequest{SessionID: session.SessionID})
	assertStatus(t, response, http.StatusOK)
	initial := decodeData[storage.RPObservation](t, response)
	for name, override := range map[string]map[string]any{
		"missing model":        {"endpoint": model.URL + "/v1/chat/completions", "api_key": "player-key"},
		"unallowlisted origin": {"endpoint": model.URL + "/v1/chat/completions", "model": "m", "api_key": "k"},
		"plaintext remote":     {"endpoint": "http://203.0.113.10/v1/chat/completions", "model": "m", "api_key": "k"},
		"endpoint with user":   {"endpoint": "https://user@example.com/v1/chat/completions", "model": "m", "api_key": "k"},
	} {
		response = performJSON(t, handler, "/api/v1/rp/turns/run", rpPlayerToken, map[string]any{
			"session_id": session.SessionID, "text": "你好。", "expected_cursor": initial.ObservationCursor,
			"idempotency_key": "http-invalid-" + name, "model": override,
		})
		assertAPIError(t, response, http.StatusBadRequest, core.CodeInvalidArgument)
	}
	if calls.Load() != 0 {
		t.Fatalf("invalid overrides reached the player endpoint: %d", calls.Load())
	}
}
