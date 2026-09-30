package decision

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/endpointpolicy"
)

func interactionFixtureInput(text string) core.RPInteractionUnderstandingInput {
	return core.RPInteractionUnderstandingInput{Text: text, Mode: "AUTO", PlaceName: "门厅", WorldTime: "2026-09-27T12:00:00Z", ReachablePlaces: []core.RPInteractionPlace{{ID: "garden", Name: "花园"}}, PresentEntities: []core.RPInteractionEntity{{ID: "visible-entity", Name: "她"}}}
}

func interactionFixtureStep(kind string) map[string]any {
	return map[string]any{"kind": kind, "target_place_id": "", "target_entity_id": "", "wait_hours": 0, "wait_minutes": 0, "speech_text": "", "object_action": "", "object_id": "", "anchor_id": "", "offer_id": "", "nonverbal_action": "", "gesture_code": ""}
}

func interactionFixtureResponse(kind, action, target string, hours, minutes int, speech, clarification string) string {
	steps := []map[string]any{}
	if action != "none" {
		step := interactionFixtureStep(action)
		step["target_place_id"], step["wait_hours"], step["wait_minutes"] = target, hours, minutes
		steps = append(steps, step)
	}
	if speech != "" {
		step := interactionFixtureStep("speech")
		step["speech_text"] = speech
		steps = append(steps, step)
	}
	value, _ := json.Marshal(map[string]any{"kind": kind, "steps": steps, "clarification": clarification})
	return string(value)
}

func TestChatInteractionUnderstandsVariedInputsUsingAuthorizedCandidates(t *testing.T) {
	cases := []struct {
		text, proposal, kind, step string
	}{
		{"想你了。", interactionFixtureResponse("DIALOGUE", "none", "", 0, 0, "想你了。", ""), "DIALOGUE", "speech"},
		{"你是谁呀？", interactionFixtureResponse("DIALOGUE", "none", "", 0, 0, "你是谁呀？", ""), "DIALOGUE", "speech"},
		{"继续剧情", interactionFixtureResponse("CONTINUE", "wait", "", 0, 15, "", ""), "CONTINUE", "wait"},
		{"别停，我们接着看看接下来会怎样", interactionFixtureResponse("CONTINUE", "wait", "", 0, 15, "", ""), "CONTINUE", "wait"},
		{"等我一下，我有件事想告诉你。", interactionFixtureResponse("DIALOGUE", "none", "", 0, 0, "等我一下，我有件事想告诉你。", ""), "DIALOGUE", "speech"},
		{"能陪我走到花园吗？", interactionFixtureResponse("DIALOGUE", "none", "", 0, 0, "能陪我走到花园吗？", ""), "DIALOGUE", "speech"},
		{"我去花园，随后说：\"到了。\"", interactionFixtureResponse("MIXED", "move", "garden", 0, 0, "到了。", ""), "MIXED", "move"},
		{"*看了她一眼，没有说话。*", interactionFixtureResponse("CLARIFICATION", "none", "", 0, 0, "", "unsupported_expression"), "CLARIFICATION", ""},
		{"我把杯子推给她，说：\"喝一点吧。\"", interactionFixtureResponse("CLARIFICATION", "none", "", 0, 0, "", "unsupported_item"), "CLARIFICATION", ""},
	}
	for i, tc := range cases {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				var request struct {
					Messages       []struct{ Role, Content string } `json:"messages"`
					ResponseFormat struct {
						Type       string `json:"type"`
						JSONSchema struct {
							Strict bool `json:"strict"`
						} `json:"json_schema"`
					} `json:"response_format"`
				}
				if r.URL.Path != "/v1/chat/completions" || r.Method != http.MethodPost || json.NewDecoder(r.Body).Decode(&request) != nil || len(request.Messages) != 2 || !request.ResponseFormat.JSONSchema.Strict || request.ResponseFormat.Type != "json_schema" {
					t.Error("unstructured model request")
				}
				var input struct {
					Version         string                    `json:"version"`
					Text            string                    `json:"text"`
					ReachablePlaces []core.RPInteractionPlace `json:"reachable_places"`
				}
				if json.Unmarshal([]byte(request.Messages[1].Content), &input) != nil || input.Version != "corerp.interaction.v2" || input.Text != tc.text || len(input.ReachablePlaces) != 1 || strings.Contains(request.Messages[0].Content, tc.text) {
					t.Error("authorized scene or untrusted text boundary changed")
				}
				modelResponse(w, tc.proposal, "stop")
			}))
			defer server.Close()
			provider, err := NewChatProvider(Config{Endpoint: server.URL + "/v1/chat/completions", Model: "fixture", EndpointPolicy: endpointpolicy.TestLocalhostPolicy()})
			if err != nil {
				t.Fatal(err)
			}
			trace := &core.RPProviderTrace{}
			plan, err := provider.UnderstandInteraction(core.WithRPProviderTrace(context.Background(), trace), interactionFixtureInput(tc.text))
			if err != nil || plan.Kind != tc.kind || calls.Load() != 1 || trace.AttemptCount() != 1 {
				t.Fatalf("proposal %+v error %v calls=%d traced=%d", plan, err, calls.Load(), trace.AttemptCount())
			}
			if tc.step == "" {
				if len(plan.Steps) != 0 {
					t.Fatalf("clarification proposed effect: %+v", plan)
				}
			} else if len(plan.Steps) == 0 || plan.Steps[0].Kind != tc.step {
				t.Fatalf("incorrect first effect: %+v", plan)
			}
		})
	}
}

func TestChatInteractionRejectsUntrustedOrInvalidProposals(t *testing.T) {
	text := "我把杯子推给她，说：\"喝一点吧。\""
	for _, tc := range []struct{ name, output, finish string }{
		{"invented speech", interactionFixtureResponse("DIALOGUE", "none", "", 0, 0, "我送给你一把枪", ""), "stop"},
		{"unsupported effect as dialogue", interactionFixtureResponse("MIXED", "item", "", 0, 0, "喝一点吧。", ""), "stop"},
		{"inaccessible destination", interactionFixtureResponse("ACTION", "move", "secret", 0, 0, "", ""), "stop"},
		{"mixed without action", interactionFixtureResponse("MIXED", "none", "", 0, 0, "喝一点吧。", ""), "stop"},
		{"continue with speech", interactionFixtureResponse("CONTINUE", "wait", "", 0, 15, "喝一点吧。", ""), "stop"},
		{"invalid duration", interactionFixtureResponse("CONTINUE", "wait", "", 2, 15, "", ""), "stop"},
		{"duplicate", `{"kind":"ACTION","kind":"DIALOGUE","steps":[],"clarification":""}`, "stop"},
		{"missing", `{"kind":"ACTION"}`, "stop"},
		{"unknown", `{"kind":"ACTION","steps":[],"clarification":"","private_state":"steal"}`, "stop"},
		{"null steps", `{"kind":"ACTION","steps":null,"clarification":""}`, "stop"},
		{"nested duplicate", `{"kind":"ACTION","steps":[{"kind":"wait","kind":"speech","target_place_id":"","target_entity_id":"","wait_hours":1,"wait_minutes":0,"speech_text":"","object_action":"","object_id":"","anchor_id":"","offer_id":"","nonverbal_action":"","gesture_code":""}],"clarification":""}`, "stop"},
		{"nested missing", `{"kind":"ACTION","steps":[{"kind":"wait"}],"clarification":""}`, "stop"},
		{"nested unknown", `{"kind":"ACTION","steps":[{"kind":"wait","target_place_id":"","target_entity_id":"","wait_hours":1,"wait_minutes":0,"speech_text":"","object_action":"","object_id":"","anchor_id":"","offer_id":"","nonverbal_action":"","gesture_code":"","private_state":"steal"}],"clarification":""}`, "stop"},
		{"truncated", interactionFixtureResponse("DIALOGUE", "none", "", 0, 0, "喝一点吧。", ""), "length"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); modelResponse(w, tc.output, tc.finish) }))
			defer server.Close()
			provider, err := NewChatProvider(Config{Endpoint: server.URL + "/v1/chat/completions", Model: "fixture", EndpointPolicy: endpointpolicy.TestLocalhostPolicy()})
			if err != nil {
				t.Fatal(err)
			}
			plan, err := provider.UnderstandInteraction(context.Background(), interactionFixtureInput(text))
			if err == nil || len(plan.Steps) != 0 || calls.Load() != 1 {
				t.Fatalf("invalid proposal accepted/retried: %+v %v calls=%d", plan, err, calls.Load())
			}
		})
	}
}
