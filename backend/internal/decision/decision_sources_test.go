package decision

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"

	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/endpointpolicy"
)

func TestDecisionBoundRefsAndSameContractReachEveryTransport(t *testing.T) {
	for _, format := range []string{"json_schema", "json_object", "tool_call"} {
		for _, channel := range []string{"content", "function"} {
			if channel == "function" && format != "tool_call" {
				continue
			}
			t.Run(format+"/"+channel, func(t *testing.T) {
				input := contextFixture()
				input.SpeechEventID = "event_rp_speech_" + strings.Repeat("abcdef01", 8)
				input.PlayerSpeechText = "我说的文本含有 " + input.SpeechEventID + "，不应被引用表改写。"
				before, _ := json.Marshal(input)
				calls := 0
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls++
					var request map[string]any
					if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
						t.Fatal(err)
					}
					messages := request["messages"].([]any)
					var packet decisionContext
					if json.Unmarshal([]byte(messages[1].(map[string]any)["content"].(string)), &packet) != nil || !reflect.DeepEqual(packet.Character, input) {
						t.Fatal("provider view changed canonical data or heard speech")
					}
					if packet.EvidenceSupportVersion != core.RPEvidenceSupportVersion || !reflect.DeepEqual(packet.GroundingSources, decisionSourceRefs(input)) || len(packet.GroundingSources) != 1 || packet.GroundingSources[0].SourceEventID != input.SpeechEventID || packet.GroundingSources[0].Ref != "src_1" {
						t.Fatal("ref binding missing or outside this authorized packet")
					}
					var declared any
					if format == "tool_call" {
						declared = request["tools"].([]any)[0].(map[string]any)["function"].(map[string]any)["parameters"]
					} else if format == "json_schema" {
						declared = request["response_format"].(map[string]any)["json_schema"].(map[string]any)["schema"]
					} else {
						declared = packet.ProposalSchema
					}
					if !reflect.DeepEqual(declared, packet.ProposalSchema) || packet.ProposalSchema["additionalProperties"] != false {
						t.Fatal("message and transport received competing proposal contracts")
					}
					refs := packet.ProposalSchema["properties"].(map[string]any)["private"].(map[string]any)["properties"].(map[string]any)["basis_event_ids"].(map[string]any)["items"].(map[string]any)["enum"]
					if !reflect.DeepEqual(refs, []any{"src_1"}) {
						t.Fatal("model must still copy a long opaque Event ID")
					}
					args := strings.Replace(wireDecision(`{"action":"respond","text":"我听见了。","introduce_self":false,"expression_code":"none"}`), "event-player", "src_1", 1)
					if channel == "function" {
						_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": "tool_calls", "message": map[string]any{"content": "unapproved private narration", "tool_calls": []any{proposalTool(args)}}}}})
					} else {
						modelResponse(w, args, "stop")
					}
				}))
				defer server.Close()
				p, err := NewChatProvider(Config{Endpoint: server.URL + "/v1/chat/completions", Model: "fixture", DecisionFormat: format, EndpointPolicy: endpointpolicy.TestLocalhostPolicy()})
				if err != nil {
					t.Fatal(err)
				}
				proposal, err := p.Propose(context.Background(), input)
				if err != nil || calls != 1 || proposal.Private == nil || !reflect.DeepEqual(proposal.Private.BasisEventIDs, []string{input.SpeechEventID}) || core.ValidateRPDecisionProposal(input, proposal) != nil {
					t.Fatal("short ref did not resolve through unchanged canonical validation", err)
				}
				after, _ := json.Marshal(input)
				if string(before) != string(after) {
					t.Fatal("source binding mutated the shared context")
				}
			})
		}
	}
}

func TestDecisionSourceRefsRejectUnknownDuplicatesAndNamespaceCollisions(t *testing.T) {
	input := contextFixture()
	input.SpeechEventID = "src_1" // An actual source must never be rebound as an alias.
	input.RelevantDialogue = []core.RPDecisionExchange{{Dialogue: []core.RPDecisionDialogue{{EventID: "event-before"}}}}
	refs := decisionSourceRefs(input)
	if len(refs) != 2 || refs[0].Ref != "src_2" || refs[1].Ref != "src_3" || !reflect.DeepEqual(refs, decisionSourceRefs(input)) {
		t.Fatal("source handles collide or depend on map iteration")
	}
	for _, tc := range []struct {
		name  string
		basis []string
		valid bool
	}{
		{"bound", []string{"src_2"}, true},
		{"raw authorized compatibility", []string{"src_1"}, true},
		{"unknown handle", []string{"src_9999"}, false},
		{"unheard real event", []string{"event-unheard"}, false},
		{"repeated handle", []string{"src_2", "src_2"}, false},
		{"same source through alias and raw", []string{"src_2", "event-before"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			proposal := core.RPDecisionProposal{Action: "wait", Private: &core.RPDecisionPrivate{BasisEventIDs: tc.basis}}
			resolved := resolveDecisionSourceRefs(input, proposal)
			if (core.ValidateRPDecisionProposal(input, resolved) == nil) != tc.valid || !reflect.DeepEqual(proposal.Private.BasisEventIDs, tc.basis) {
				t.Fatal("mapping weakened evidence checks or modified the caller's proposal")
			}
		})
	}
}

func TestDecisionSourceDiagnosticsContainOnlyShapeAndBoundedCodes(t *testing.T) {
	t.Setenv("CORERP_DECISION_DEBUG", "1")
	log, err := os.CreateTemp(t.TempDir(), "source-shape-*.log")
	if err != nil {
		t.Fatal(err)
	}
	previous := os.Stderr
	os.Stderr = log
	defer func() { os.Stderr = previous; _ = log.Close() }()
	const secret = "fixture-secret-private-text"
	debugDecisionProposalShape(`{"private":{"intent":"` + secret + `"},"observable":{},"` + secret + `":"hidden"}`)
	for _, basis := range []string{`["` + secret + `"]`, `["src_1","src_1"]`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			modelResponse(w, `{"private":{"intent":"`+secret+`","emotion":"","relationship_stance":"","basis_event_ids":`+basis+`},"observable":{"action":"wait","expression_code":"none"}}`, "stop")
		}))
		p, err := NewChatProvider(Config{Endpoint: server.URL + "/v1/chat/completions", Model: "fixture", DecisionFormat: "tool_call", EndpointPolicy: endpointpolicy.TestLocalhostPolicy()})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := p.Propose(context.Background(), contextFixture()); err == nil {
			t.Fatal("invalid basis accepted")
		}
		server.Close()
	}
	data, err := os.ReadFile(log.Name())
	if err != nil || strings.Contains(string(data), secret) || !strings.Contains(string(data), "unknown_count=1") || !strings.Contains(string(data), "repeated_count=1") || !strings.Contains(string(data), "private_missing=3") {
		t.Fatal("diagnostic leaked values or did not distinguish unsupported and duplicate refs")
	}
	if (&Error{Kind: "invalid proposal schema"}).RPDecisionFailureCode() != "schema" {
		t.Fatal("shape failure still reported as unclassified other")
	}
}
