package decision

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"corerp.local/backend/internal/endpointpolicy"
)

func TestReasoningOptionsReachAllDecisionPaths(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		t.Run(map[bool]string{false: "defaults", true: "explicit"}[explicit], func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var body map[string]json.RawMessage
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
					return
				}
				effort, hasEffort := body["reasoning_effort"]
				thinking, hasThinking := body["enable_thinking"]
				if explicit {
					if string(effort) != `"low"` || string(thinking) != "false" {
						t.Error("explicit reasoning options were lost")
					}
				} else if hasEffort || hasThinking {
					t.Error("provider-specific options were sent without configuration")
				}
				var format struct {
					JSONSchema struct {
						Name string `json:"name"`
					} `json:"json_schema"`
				}
				if err := json.Unmarshal(body["response_format"], &format); err != nil {
					t.Error(err)
					return
				}
				switch format.JSONSchema.Name {
				case "corerp_decision_v4":
					if string(body["max_completion_tokens"]) != "4096" {
						t.Error("lost decision budget")
					}
					modelResponse(w, wireDecision(`{"action":"silence","expression_code":"none"}`), "stop")
				case "corerp_interaction_v4":
					if string(body["max_completion_tokens"]) != "3072" {
						t.Error("lost interpretation budget")
					}
					modelResponse(w, interactionFixtureResponse("DIALOGUE", "none", "", 0, 0, "想你了。", ""), "stop")
				case "corerp_intent_class":
					modelResponse(w, `{"player_action":false,"player_speech":true,"meta_continue":false,"third_party_claim":false,"command_scope":"none","two_actions":false,"action_family":"none"}`, "stop")
				default:
					t.Errorf("unexpected schema %q", format.JSONSchema.Name)
				}
			}))
			defer server.Close()
			config := Config{Endpoint: server.URL + "/v1/chat/completions", Model: "fixture", DecisionMaxTokens: 4096, InteractionMaxTokens: 3072, EndpointPolicy: endpointpolicy.TestLocalhostPolicy()}
			if explicit {
				config.ReasoningEffort = "low"
				config.DisableThinking = true
			}
			provider, err := NewChatProvider(config)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := provider.Propose(context.Background(), contextFixture()); err != nil {
				t.Fatal(err)
			}
			if _, err := provider.UnderstandInteraction(context.Background(), interactionFixtureInput("想你了。")); err != nil {
				t.Fatal(err)
			}
			config.Model = "gemini-3.8-flash-high"
			staged, err := NewChatProvider(config)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := staged.UnderstandInteraction(context.Background(), interactionFixtureInput("想你了。")); err != nil {
				t.Fatal(err)
			}
			if calls != 3 {
				t.Fatalf("expected three paths, got %d calls", calls)
			}
		})
	}
}
