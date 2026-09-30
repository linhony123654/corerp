package decision

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"corerp.local/backend/internal/endpointpolicy"
)

func TestDecisionJSONModeCarriesClosedSchemaAndUnchangedCharacterContext(t *testing.T) {
	in := contextFixture()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages       []struct{ Role, Content string }
			ResponseFormat map[string]json.RawMessage `json:"response_format"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if len(body.ResponseFormat) != 1 || string(body.ResponseFormat["type"]) != `"json_object"` || len(body.Messages) != 2 {
			t.Fatal("JSON Mode request lacks its explicit transport contract")
		}
		var got decisionContext
		if err := json.Unmarshal([]byte(body.Messages[1].Content), &got); err != nil || !reflect.DeepEqual(got.Character, in) {
			t.Fatal("transport format changed the authorized character context", err)
		}
		if strings.Contains(body.Messages[0].Content, in.PlayerSpeechText) {
			t.Fatal("in-world player input became system instructions")
		}
		schema := got.ProposalSchema
		if schema == nil || schema["additionalProperties"] != false || len(got.GroundingSources) != 1 || got.GroundingSources[0].SourceEventID != in.SpeechEventID {
			t.Fatal("JSON Mode lost the formal proposal schema or authorized ref binding")
		}
		properties := schema["properties"].(map[string]any)
		basis := properties["private"].(map[string]any)["properties"].(map[string]any)["basis_event_ids"].(map[string]any)
		if !reflect.DeepEqual(basis["items"].(map[string]any)["enum"], []any{got.GroundingSources[0].Ref}) {
			t.Fatal("JSON Mode no longer supplies only the bound permitted source refs")
		}
		if properties["observable"].(map[string]any)["anyOf"] == nil {
			t.Fatal("JSON Mode omitted the mutually exclusive observable variants")
		}
		modelResponse(w, wireDecision(`{"action":"respond","text":"今天不方便。","introduce_self":false,"expression_code":"none"}`), "stop")
	}))
	defer server.Close()
	provider, err := NewChatProvider(Config{Endpoint: server.URL + "/v1/chat/completions", Model: "fixture", DecisionFormat: "json_object", EndpointPolicy: endpointpolicy.TestLocalhostPolicy()})
	if err != nil {
		t.Fatal(err)
	}
	p, err := provider.Propose(context.Background(), in)
	if err != nil || p.Action != "respond" || p.Private == nil || p.ExpressionCode != "" {
		t.Fatal("JSON Mode did not pass the existing typed proposal gate", err)
	}
}

func TestDecisionJSONModeStillRejectsExtraEffectsBadSourcesAndIllegalActions(t *testing.T) {
	for _, tc := range []struct{ name, raw string }{
		{"extra effect", wireDecision(`{"action":"respond","text":"好。","introduce_self":false,"expression_code":"none","destination_place_id":"home"}`)},
		{"unheard source", strings.Replace(wireDecision(`{"action":"wait","expression_code":"none"}`), "event-player", "event-unheard", 1)},
		{"unreachable destination", wireDecision(`{"action":"leave","destination_place_id":"private-vault"}`)},
		{"duplicate action", wireDecision(`{"action":"wait","action":"silence","expression_code":"none"}`)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				modelResponse(w, tc.raw, "stop")
			}))
			defer server.Close()
			provider, err := NewChatProvider(Config{Endpoint: server.URL + "/v1/chat/completions", Model: "fixture", DecisionFormat: "json_object", EndpointPolicy: endpointpolicy.TestLocalhostPolicy()})
			if err != nil {
				t.Fatal(err)
			}
			p, err := provider.Propose(context.Background(), contextFixture())
			if err == nil || p.Action != "" || calls.Load() > 2 {
				t.Fatalf("JSON Mode accepted an invalid effect/source or exceeded bounded repair: calls=%d err=%v", calls.Load(), err)
			}
		})
	}
}

func TestDecisionFormatConfigurationIsExplicit(t *testing.T) {
	for _, mode := range []string{"", "json_schema", "json_object", "tool_call", "auto", "text"} {
		env := map[string]string{"CORERP_DECISION_PROVIDER": "chat_completions", "CORERP_LLM_ENDPOINT": "http://127.0.0.1:9090/v1/chat/completions", "CORERP_PROVIDER_LOCAL_ALLOWLIST": "http://127.0.0.1:9090/v1/chat/completions", "CORERP_LLM_MODEL": "fixture", "CORERP_LLM_DECISION_FORMAT": mode}
		p, _, err := FromEnvironment(func(k string) string { return env[k] })
		valid := mode == "" || mode == "json_schema" || mode == "json_object" || mode == "tool_call"
		if (err == nil) != valid {
			t.Fatalf("invalid decision format acceptance: %q %v", mode, err)
		}
		if valid {
			want := mode
			if want == "" {
				want = "json_schema"
			}
			if p.(*ChatProvider).config.DecisionFormat != want {
				t.Fatal("explicit format did not reach provider")
			}
		}
	}
	if _, _, err := FromEnvironment(func(k string) string {
		if k == "CORERP_LLM_DECISION_FORMAT" {
			return "json_object"
		}
		return ""
	}); err == nil {
		t.Fatal("deterministic mode accepted a hidden LLM configuration")
	}
}

func TestDecisionJSONModeRepairsInvalidRootOnceWithoutChangingContextOrFormat(t *testing.T) {
	for _, repaired := range []bool{false, true} {
		t.Run(map[bool]string{false: "still_invalid", true: "valid_second_object"}[repaired], func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var body struct {
					Messages       []struct{ Content string }
					ResponseFormat struct{ Type string } `json:"response_format"`
				}
				if json.NewDecoder(r.Body).Decode(&body) != nil || body.ResponseFormat.Type != "json_object" {
					t.Fatal("repair changed or lost the configured transport format")
				}
				var in decisionContext
				if json.Unmarshal([]byte(body.Messages[1].Content), &in) != nil || !reflect.DeepEqual(in.Character, contextFixture()) {
					t.Fatal("repair added facts or changed the received context")
				}
				if calls == 2 && (len(body.Messages) != 3 || !strings.Contains(body.Messages[2].Content, "rejected response")) {
					t.Fatal("shape repair lacks bounded feedback")
				}
				if calls == 2 && repaired {
					modelResponse(w, wireDecision(`{"action":"respond","text":"今天不方便。","introduce_self":false,"expression_code":"none"}`), "stop")
				} else {
					modelResponse(w, `{"text":"unapproved"}`, "stop")
				}
			}))
			defer server.Close()
			provider, err := NewChatProvider(Config{Endpoint: server.URL + "/v1/chat/completions", Model: "fixture", DecisionFormat: "json_object", EndpointPolicy: endpointpolicy.TestLocalhostPolicy()})
			if err != nil {
				t.Fatal(err)
			}
			p, err := provider.Propose(context.Background(), contextFixture())
			if calls != 2 || (err == nil) != repaired || !repaired && p.Action != "" {
				t.Fatalf("format repair was unbounded or accepted the first bad proposal: calls=%d err=%v", calls, err)
			}
		})
	}
}
