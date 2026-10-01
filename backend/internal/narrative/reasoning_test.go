package narrative

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestNarrativeReasoningConfigurationIsExplicitAndIndependent(t *testing.T) {
	for _, mode := range []string{"style_planner", "full_prose"} {
		for _, explicit := range []bool{false, true} {
			name := mode + "_decision_options_only"
			if explicit {
				name = mode + "_explicit_narrative_options"
			}
			t.Run(name, func(t *testing.T) {
				var calls atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					if r.Header.Get("Authorization") != "Bearer narrative-fixture-key" {
						t.Error("narrative request inherited the decision credential")
					}
					var request map[string]json.RawMessage
					if json.NewDecoder(r.Body).Decode(&request) != nil {
						t.Error("invalid narrative request")
						return
					}
					if explicit {
						if string(request["reasoning_effort"]) != `"low"` || string(request["enable_thinking"]) != "false" {
							t.Error("explicit narrator reasoning settings did not reach the wire")
						}
					} else if request["reasoning_effort"] != nil || request["enable_thinking"] != nil {
						t.Error("decision reasoning settings leaked into the narrator")
					}
					content := validPlan
					if mode == "full_prose" {
						content = "你：[[corerp-speech:0]]"
					}
					_, _ = io.WriteString(w, envelope(content))
				}))
				defer server.Close()
				values := map[string]string{
					"CORERP_NARRATIVE_PROVIDER": mode, "CORERP_NARRATIVE_ENDPOINT": server.URL + "/v1/chat/completions",
					"CORERP_NARRATIVE_MODEL": "fixture", "CORERP_NARRATIVE_API_KEY": "narrative-fixture-key",
					"CORERP_PROVIDER_LOCAL_ALLOWLIST": server.URL,
					"CORERP_LLM_ENDPOINT":             "https://decision-only.invalid", "CORERP_LLM_API_KEY": "decision-fixture-key",
					"CORERP_LLM_REASONING_EFFORT": "high", "CORERP_LLM_DISABLE_THINKING": "true",
				}
				if explicit {
					values["CORERP_NARRATIVE_REASONING_EFFORT"] = "low"
					values["CORERP_NARRATIVE_DISABLE_THINKING"] = "true"
				}
				provider, gotMode, err := FromEnvironment(func(key string) string { return values[key] })
				if err != nil || gotMode != mode {
					t.Fatal("explicit narrator configuration rejected", err)
				}
				style := core.DefaultRPStyle()
				style.ProseInstructions = "用第一人称。"
				view, err := provider.Render(context.Background(), core.RPNarrativeInput{ControlledEntityID: "player", Style: style,
					Facts: []core.RPNarrativeFact{{EventID: "speech", ActorID: "player", ActorName: "Lin", Action: "speak", Text: "你好。"}}})
				if err != nil || view.FallbackReason != "" || calls.Load() != 1 {
					t.Fatal("narrative request failed", err, view.FallbackReason, calls.Load())
				}
			})
		}
	}
}

func TestNarrativeReasoningConfigurationRejectsInvalidAndUnusedOptions(t *testing.T) {
	for _, mode := range []string{"", "style_planner", "full_prose"} {
		for _, option := range []string{"CORERP_NARRATIVE_REASONING_EFFORT", "CORERP_NARRATIVE_DISABLE_THINKING"} {
			values := map[string]string{"CORERP_NARRATIVE_PROVIDER": mode,
				"CORERP_NARRATIVE_ENDPOINT": "http://127.0.0.1:1/v1/chat/completions", "CORERP_NARRATIVE_MODEL": "fixture",
				"CORERP_PROVIDER_LOCAL_ALLOWLIST": "http://127.0.0.1:1", option: "unsupported"}
			if _, _, err := FromEnvironment(func(key string) string { return values[key] }); err == nil {
				t.Fatal("invalid or unused narrator option accepted", mode, option)
			}
		}
	}
}
