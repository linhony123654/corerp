package decision

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/endpointpolicy"
)

func TestDecisionContextReadinessStopsHTTPWithoutInferringRelationships(t *testing.T) {
	for _, format := range []string{"json_schema", "json_object", "tool_call"} {
		for _, tc := range []struct {
			name      string
			readiness core.RPContextReadiness
			wantCalls int
		}{
			{"persona missing", core.RPContextReadiness{Persona: "MISSING", RelationshipToInterlocutor: "UNKNOWN", AddressToInterlocutor: "UNKNOWN"}, 0},
			{"known relation missing address", core.RPContextReadiness{Persona: "READY", RelationshipToInterlocutor: "READY", AddressToInterlocutor: "MISSING"}, 0},
			{"ready stranger", core.RPContextReadiness{Persona: "READY", RelationshipToInterlocutor: "UNKNOWN", AddressToInterlocutor: "UNKNOWN"}, 1},
			{"ready known relation", core.RPContextReadiness{Persona: "READY", RelationshipToInterlocutor: "READY", AddressToInterlocutor: "READY"}, 1},
		} {
			t.Run(format+"/"+tc.name, func(t *testing.T) {
				count := 0
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					count++
					proposal := wireDecision(`{"action":"wait","expression_code":"none"}`)
					if format == "tool_call" {
						_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]any{"content": "", "tool_calls": []any{proposalTool(proposal)}}}}})
					} else {
						modelResponse(w, proposal, "stop")
					}
				}))
				defer server.Close()
				p, err := NewChatProvider(Config{Endpoint: server.URL + "/v1/chat/completions", Model: "fixture", DecisionFormat: format, EndpointPolicy: endpointpolicy.TestLocalhostPolicy()})
				if err != nil {
					t.Fatal(err)
				}
				input := contextFixture()
				input.Readiness = tc.readiness
				proposal, err := p.Propose(context.Background(), input)
				if count != tc.wantCalls {
					t.Fatalf("incomplete context sent to a model or stranger blocked: calls=%d", count)
				}
				if tc.wantCalls == 0 {
					var classified *Error
					if !errors.As(err, &classified) || classified.RPDecisionFailureCode() != "context_not_ready" || proposal.Action != "" || proposal.Private != nil {
						t.Fatal("not-ready context became a fabricated proposal", err)
					}
				} else if err != nil || proposal.Action != "wait" {
					t.Fatal("ready character rejected", err)
				}
			})
		}
	}
}
