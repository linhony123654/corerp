package decision

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"corerp.local/backend/internal/endpointpolicy"
)

func proposalTool(arguments string) map[string]any {
	return map[string]any{"id": "fixture-call", "type": "function", "function": map[string]any{"name": decisionProposalFunction, "arguments": arguments}}
}

func TestDecisionNativeToolUsesOnePrivateProposalAndUnchangedContext(t *testing.T) {
	for _, finish := range []string{"stop", "tool_calls"} {
		t.Run(finish, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]json.RawMessage
				if json.NewDecoder(r.Body).Decode(&body) != nil {
					t.Fatal("bad request")
				}
				if body["response_format"] != nil {
					t.Fatal("native tool still sent a conflicting content-format constraint")
				}
				var tools []struct {
					Type     string
					Function struct {
						Name       string
						Parameters map[string]any
					}
				}
				if json.Unmarshal(body["tools"], &tools) != nil || len(tools) != 1 || tools[0].Type != "function" || tools[0].Function.Name != decisionProposalFunction || tools[0].Function.Parameters["additionalProperties"] != false {
					t.Fatal("wrong or open function contract")
				}
				var messages []struct{ Content string }
				if json.Unmarshal(body["messages"], &messages) != nil || len(messages) != 2 {
					t.Fatal("context message missing")
				}
				var got decisionContext
				if json.Unmarshal([]byte(messages[1].Content), &got) != nil || !decisionCharacterMatches(t, got.Character, contextFixture()) {
					t.Fatal("native function changed the authorized context")
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": finish, "message": map[string]any{"content": "Unapproved narration: I moved to a new place.", "tool_calls": []any{proposalTool(wireDecision(`{"action":"respond","text":"我在。","introduce_self":false,"expression_code":"none"}`))}}}}})
			}))
			defer server.Close()
			p, err := NewChatProvider(Config{Endpoint: server.URL + "/v1/chat/completions", Model: "fixture", DecisionFormat: "tool_call", EndpointPolicy: endpointpolicy.TestLocalhostPolicy()})
			if err != nil {
				t.Fatal(err)
			}
			proposal, err := p.Propose(context.Background(), contextFixture())
			if err != nil || proposal.Text != "我在。" || proposal.Private == nil || proposal.DestinationPlaceID != "" {
				t.Fatal("function metadata or ignored content became a world effect", err)
			}
		})
	}
}

func TestDecisionNativeToolRejectsUnknownMultipleLegacyAndUngroundedCalls(t *testing.T) {
	valid := proposalTool(wireDecision(`{"action":"wait","expression_code":"none"}`))
	wrong := proposalTool(wireDecision(`{"action":"wait","expression_code":"none"}`))
	wrong["function"].(map[string]any)["name"] = "delete_world"
	for _, tc := range []struct {
		name   string
		calls  []any
		finish string
	}{
		{"no call", nil, "stop"}, {"unknown function", []any{wrong}, "stop"}, {"multiple", []any{valid, valid}, "tool_calls"},
		{"legacy lacks private", []any{proposalTool(`{"action":"respond","text":"unapproved","introduce_self":false,"destination_place_id":"","activity_code":""}`)}, "stop"},
		{"bad source", []any{proposalTool(strings.Replace(wireDecision(`{"action":"wait","expression_code":"none"}`), "event-player", "event-unheard", 1))}, "stop"},
		{"extra effect", []any{proposalTool(wireDecision(`{"action":"respond","text":"hi","introduce_self":false,"expression_code":"none","activity_code":"uncommitted"}`))}, "stop"},
		{"truncated", []any{valid}, "length"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			count := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				count++
				_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": tc.finish, "message": map[string]any{"content": "", "tool_calls": tc.calls}}}})
			}))
			defer server.Close()
			p, err := NewChatProvider(Config{Endpoint: server.URL + "/v1/chat/completions", Model: "fixture", DecisionFormat: "tool_call", EndpointPolicy: endpointpolicy.TestLocalhostPolicy()})
			if err != nil {
				t.Fatal(err)
			}
			proposal, err := p.Propose(context.Background(), contextFixture())
			if err == nil || proposal.Action != "" || count > 2 {
				t.Fatalf("unapproved call was accepted or retry unbounded: calls=%d err=%v", count, err)
			}
		})
	}
}

func TestDecisionToolContentCompatibilityKeepsFullV3AndWorldValidation(t *testing.T) {
	valid := wireDecision(`{"action":"respond","text":"我在。","introduce_self":false,"expression_code":"none"}`)
	for _, tc := range []struct {
		name, content string
		wantSuccess   bool
	}{
		{"complete v3 proposal", valid, true},
		{"ordinary roleplay", "我在这里，已经递给你一杯茶了。", false},
		{"fenced proposal", "```json\n" + valid + "\n```", false},
		{"partial object", valid[:len(valid)-1], false},
		{"legacy root", `{"action":"respond","text":"我在。","introduce_self":false,"destination_place_id":"","activity_code":""}`, false},
		{"unheard basis", strings.Replace(valid, "event-player", "event-unheard", 1), false},
		{"extra world effect", strings.Replace(valid, `"expression_code":"none"`, `"expression_code":"none","activity_code":"uncommitted"`, 1), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls++
				modelResponse(w, tc.content, "stop")
			}))
			defer server.Close()
			provider, err := NewChatProvider(Config{Endpoint: server.URL + "/v1/chat/completions", Model: "fixture", DecisionFormat: "tool_call", EndpointPolicy: endpointpolicy.TestLocalhostPolicy()})
			if err != nil {
				t.Fatal(err)
			}
			proposal, err := provider.Propose(context.Background(), contextFixture())
			if tc.wantSuccess {
				if err != nil || proposal.Text != "我在。" || calls != 1 {
					t.Fatal("complete proposal from a compatible channel rejected", err)
				}
			} else if err == nil || proposal.Action != "" || calls > 2 {
				t.Fatal("content bypassed v3/grounding/world validation", err)
			}
		})
	}
}
