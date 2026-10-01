package decision

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/endpointpolicy"
)

func TestChatReasoningEffortIsExplicitForDecisionAndInterpretation(t *testing.T) {
	for _, effort := range []string{"", "low"} {
		t.Run(effort, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var body map[string]json.RawMessage
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
					return
				}
				actual, present := body["reasoning_effort"]
				if effort == "" && present || effort != "" && (!present || string(actual) != `"low"`) {
					t.Error("reasoning effort was not conditionally transmitted")
				}
				budget := "1024"
				if calls == 2 {
					budget = "3072"
				}
				if string(body["max_completion_tokens"]) != budget {
					t.Error("per-operation completion budget was not transmitted")
				}
				if calls == 1 {
					modelResponse(w, wireDecision(`{"action":"silence","expression_code":"none"}`), "stop")
				} else {
					modelResponse(w, interactionFixtureResponse("DIALOGUE", "none", "", 0, 0, "想你了。", ""), "stop")
				}
			}))
			defer server.Close()
			provider, err := NewChatProvider(Config{Endpoint: server.URL + "/v1/chat/completions", Model: "fixture", ReasoningEffort: effort, InteractionMaxTokens: 3072, EndpointPolicy: endpointpolicy.TestLocalhostPolicy()})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := provider.Propose(context.Background(), contextFixture()); err != nil {
				t.Fatal(err)
			}
			if _, err := provider.UnderstandInteraction(context.Background(), interactionFixtureInput("想你了。")); err != nil {
				t.Fatal(err)
			}
			if calls != 2 {
				t.Fatalf("expected two independently configured calls, got %d", calls)
			}
		})
	}
}

func TestDecisionFailureCodeDoesNotReturnArbitraryProviderText(t *testing.T) {
	for _, tc := range []struct{ kind, want string }{
		{"incomplete or refused response", "incomplete"},
		{"transport unavailable", "transport"},
		{"illegal proposal", "proposal"},
		{"HTTP 502", "http"},
		{"provider returned private text", "other"},
	} {
		if got := (&Error{Kind: tc.kind}).RPDecisionFailureCode(); got != tc.want {
			t.Fatalf("%q produced diagnostic %q", tc.kind, got)
		}
	}
	if got := (&Error{Kind: "illegal proposal", Detail: "kind_steps"}).RPDecisionFailureCode(); got != "proposal_kind_steps" {
		t.Fatalf("lost bounded validation category: %s", got)
	}
	if got := (&Error{Kind: "illegal proposal", Detail: "private model text"}).RPDecisionFailureCode(); got != "proposal" {
		t.Fatalf("unsafe validation detail was persisted: %s", got)
	}
}

func TestChatInteractionRepairsMalformedSchemaOnce(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			modelResponse(w, `{"kind":"ACTION","steps":[]}`, "stop")
		} else {
			modelResponse(w, interactionFixtureResponse("DIALOGUE", "none", "", 0, 0, "想你了。", ""), "stop")
		}
	}))
	defer server.Close()
	provider, err := NewChatProvider(Config{Endpoint: server.URL + "/v1/chat/completions", Model: "fixture", Attempts: 1, ProposalRepairs: 1, EndpointPolicy: endpointpolicy.TestLocalhostPolicy()})
	if err != nil {
		t.Fatal(err)
	}
	trace := &core.RPProviderTrace{}
	plan, err := provider.UnderstandInteraction(core.WithRPProviderTrace(context.Background(), trace), interactionFixtureInput("想你了。"))
	if err != nil || plan.Kind != "DIALOGUE" || calls != 2 || trace.AttemptCount() != 2 {
		t.Fatalf("malformed response was not repaired safely: %+v %v calls=%d attempts=%d", plan, err, calls, trace.AttemptCount())
	}
}

func TestChatInteractionRepairsOneInvalidProposalBeforeAnyEffects(t *testing.T) {
	for _, correctSecond := range []bool{true, false} {
		t.Run(map[bool]string{true: "corrected", false: "still_invalid"}[correctSecond], func(t *testing.T) {
			calls := 0
			bad := interactionFixtureStep("nonverbal")
			bad["nonverbal_action"] = "look_at"
			bad["target_entity_id"] = "visible-entity"
			bad["object_action"] = "offer"
			good := interactionFixtureStep("nonverbal")
			good["nonverbal_action"] = "look_at"
			good["target_entity_id"] = "visible-entity"
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var request struct {
					Messages []struct {
						Content string `json:"content"`
					} `json:"messages"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
				}
				if len(request.Messages) != calls+1 || calls > 2 {
					t.Errorf("unbounded or missing correction request: %d messages on call %d", len(request.Messages), calls)
				}
				if calls == 2 && !strings.Contains(request.Messages[2].Content, "nonverbal") {
					t.Error("bounded repair did not describe structural error")
				}
				if calls == 2 && correctSecond {
					modelResponse(w, typedInteractionReply("ACTION", good), "stop")
				} else {
					modelResponse(w, typedInteractionReply("ACTION", bad), "stop")
				}
			}))
			defer server.Close()
			provider, err := NewChatProvider(Config{Endpoint: server.URL + "/v1/chat/completions", Model: "fixture", Attempts: 1, ProposalRepairs: 1, EndpointPolicy: endpointpolicy.TestLocalhostPolicy()})
			if err != nil {
				t.Fatal(err)
			}
			input := interactionFixtureInput("*看了她一眼，没有说话。*")
			trace := &core.RPProviderTrace{}
			plan, err := provider.UnderstandInteraction(core.WithRPProviderTrace(context.Background(), trace), input)
			if calls != 2 || trace.AttemptCount() != 2 || correctSecond && (err != nil || plan.Kind != "ACTION" || len(plan.Steps) != 1) || !correctSecond && err == nil {
				t.Fatalf("repair outcome %+v err=%v calls=%d attempts=%d", plan, err, calls, trace.AttemptCount())
			}
		})
	}
}
