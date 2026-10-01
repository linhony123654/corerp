package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/endpointpolicy"
)

func TestRPNarrativeOverridePropagatesReasoningControls(t *testing.T) {
	for _, prose := range []bool{false, true} {
		for _, explicit := range []bool{false, true} {
			name := "planner"
			if prose {
				name = "prose"
			}
			if explicit {
				name += "_explicit_controls"
			} else {
				name += "_service_defaults"
			}
			t.Run(name, func(t *testing.T) {
				var calls atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					var request map[string]json.RawMessage
					if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
						t.Error("invalid narrative request")
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					if explicit {
						if string(request["reasoning_effort"]) != `"low"` || string(request["enable_thinking"]) != "false" {
							t.Error("narrative provider dropped the explicitly selected reasoning controls")
						}
					} else if request["reasoning_effort"] != nil || request["enable_thinking"] != nil {
						t.Error("default narrative request gained unsolicited model controls")
					}
					content := `{"pov":"second_person","tense":"present","verbosity":"normal","dialogue_ratio":50,"description_density":50,"narrative_pack_ref":"builtin/plain@1","unsupported_instructions":false}`
					if prose {
						content = `{"version":"corerp.fact-composition.v2","register":"plain","paragraphs":[{"context":"none","beats":[{"fact_refs":["f0"],"form":"subject_first","lexical":"plain"}]},{"context":"none","beats":[{"fact_refs":["f1"],"form":"quote_first","lexical":"plain"}]}]}`
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]any{"content": content}}}})
				}))
				defer server.Close()
				override := &core.RPModelOverride{Endpoint: server.URL + "/v1/chat/completions", Model: "fixture", FullProse: prose}
				if explicit {
					override.ReasoningEffort, override.DisableThinking = "low", true
				}
				provider, err := resolveRPNarrativeOverride(override, endpointpolicy.TestLocalhostPolicy())
				if err != nil {
					t.Fatal(err)
				}
				style := core.DefaultRPStyle()
				style.ProseInstructions = "使用普通叙述。" // Make the planner actually run.
				input := core.RPNarrativeInput{ControlledEntityID: "player", Style: style, Facts: []core.RPNarrativeFact{
					{EventID: "speech-player", ActorID: "player", ActorName: "Lin", Action: "speak", Text: "你好。"},
					{EventID: "speech-npc", ActorID: "npc", ActorName: "Cai", Action: "respond", Text: "你好，Lin。"},
				}}
				view, err := provider.Render(context.Background(), input)
				if err != nil || view.FallbackReason != "" || calls.Load() != 1 {
					t.Fatalf("narrative override failed: %v fallback=%q calls=%d", err, view.FallbackReason, calls.Load())
				}
				if prose && (view.CompositionVersion != core.RPFactCompositionVersionV2 || !reflect.DeepEqual(view.FactGroups, [][]string{{"speech-player"}, {"speech-npc"}}) || len(view.Lines) != 2 || !strings.Contains(view.Lines[0], "「你好。」") || !strings.Contains(view.Lines[1], "「你好，Lin。」")) {
					t.Fatalf("v2 override lost ordered sources or immutable quotes: %+v", view)
				}
			})
		}
	}
}
