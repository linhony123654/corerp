package decision

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"

	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/endpointpolicy"
)

func TestChatInteractionCompactSchemaAndSilentAction(t *testing.T) {
	input := interactionFixtureInput("*看了她一眼，没有说话。*")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
			ResponseFormat struct {
				JSONSchema struct {
					Name   string `json:"name"`
					Strict bool   `json:"strict"`
					Schema struct {
						Properties struct {
							Clarification json.RawMessage `json:"clarification"`
							Steps         struct {
								Items struct {
									AnyOf []struct {
										Required   []string                   `json:"required"`
										Properties map[string]json.RawMessage `json:"properties"`
									} `json:"anyOf"`
								} `json:"items"`
							} `json:"steps"`
						} `json:"properties"`
					} `json:"schema"`
				} `json:"json_schema"`
			} `json:"response_format"`
		}
		wire, err := io.ReadAll(r.Body)
		if err != nil || json.Unmarshal(wire, &request) != nil {
			t.Error("invalid structured model request")
			return
		}
		if len(request.Messages) != 2 || request.Messages[1].Role != "user" {
			t.Error("authorized interaction packet missing")
			return
		}
		var packet struct {
			core.RPInteractionUnderstandingInput
			ProposalSchema any `json:"proposal_schema"`
		}
		var transport struct {
			ResponseFormat struct {
				JSONSchema struct {
					Schema any `json:"schema"`
				} `json:"json_schema"`
			} `json:"response_format"`
		}
		if json.Unmarshal([]byte(request.Messages[1].Content), &packet) != nil || json.Unmarshal(wire, &transport) != nil || packet.ProposalSchema == nil || !reflect.DeepEqual(packet.ProposalSchema, transport.ResponseFormat.JSONSchema.Schema) || !reflect.DeepEqual(packet.RPInteractionUnderstandingInput, input) {
			t.Error("model contract differs from transport or changed authorized candidates")
		}
		if bytes.Count(wire, []byte(`"properties":{"kind":`)) != 6 {
			t.Error("schema property order lost intent before auxiliary fields")
		}
		schema := request.ResponseFormat.JSONSchema
		if !schema.Strict || schema.Name != "corerp_interaction_v4" || len(schema.Schema.Properties.Steps.Items.AnyOf) != 5 {
			t.Error("interaction schema must expose compact step variants")
		}
		var clarification struct {
			Type string   `json:"type"`
			Enum []string `json:"enum"`
		}
		if json.Unmarshal(schema.Schema.Properties.Clarification, &clarification) != nil || clarification.Type != "string" || len(clarification.Enum) != 0 {
			t.Error("clarification schema is incompatible with providers that reject empty enum members")
		}
		for _, variant := range schema.Schema.Properties.Steps.Items.AnyOf {
			if len(variant.Required) != len(variant.Properties) || len(variant.Required) >= len(interactionStepFields) {
				t.Error("step variant includes unrelated fields")
			}
			for _, field := range []string{"speech_text", "gesture_code"} {
				raw, ok := variant.Properties[field]
				if !ok {
					continue
				}
				var value struct {
					Type string   `json:"type"`
					Enum []string `json:"enum"`
				}
				if json.Unmarshal(raw, &value) != nil || value.Type != "string" || len(value.Enum) != 0 {
					t.Errorf("%s schema is incompatible with providers that reject empty enum members", field)
				}
			}
		}
		modelResponse(w, `{"kind":"ACTION","steps":[{"kind":"nonverbal","nonverbal_action":"look_at","target_entity_id":"visible-entity","gesture_code":""}],"clarification":""}`, "stop")
	}))
	defer server.Close()
	provider, err := NewChatProvider(Config{Endpoint: server.URL + "/v1/chat/completions", Model: "fixture", EndpointPolicy: endpointpolicy.TestLocalhostPolicy()})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := provider.UnderstandInteraction(context.Background(), input)
	if err != nil || plan.Kind != "ACTION" || len(plan.Steps) != 1 || plan.Steps[0].Kind != "nonverbal" || plan.Steps[0].SpeechText != "" {
		t.Fatalf("silent compact proposal: %+v, %v", plan, err)
	}
}

func TestInteractionShapeDiagnosticsNeverPrintValues(t *testing.T) {
	t.Setenv("CORERP_DECISION_DEBUG", "1")
	log, err := os.CreateTemp(t.TempDir(), "interaction-shape-*.log")
	if err != nil {
		t.Fatal(err)
	}
	previous := os.Stderr
	os.Stderr = log
	defer func() { os.Stderr = previous; _ = log.Close() }()
	const secret = "fixture-secret-player-text-and-id"
	debugInteractionProposalShape(core.RPInteractionPlan{Steps: []core.RPInteractionStep{{Kind: secret, ObjectID: secret, ObjectAction: secret, TargetPlaceID: secret, SpeechText: secret}, {Kind: "object", AnchorID: secret}}})
	data, err := os.ReadFile(log.Name())
	if err != nil || strings.Contains(string(data), secret) || !strings.Contains(string(data), "kind=unknown") || !strings.Contains(string(data), "place=true") || !strings.Contains(string(data), "anchor=true") {
		t.Fatal("interaction diagnostic exposed values or lost field shape")
	}
}

func TestChatInteractionBindsOnlyEmptySpeechFromPlayerInput(t *testing.T) {
	for _, tc := range []struct {
		name, text, reply, want string
		valid                   bool
	}{
		{"dialogue", "想你了。", `{"kind":"DIALOGUE","steps":[{"kind":"speech","speech_text":""}],"clarification":""}`, "想你了。", true},
		{"mixed", "我去花园，然后说「到了。」", `{"kind":"MIXED","steps":[{"kind":"move","target_place_id":"garden"},{"kind":"speech","speech_text":""}],"clarification":""}`, "到了。", true},
		{"no explicit quote", "我去花园，然后说到了。", `{"kind":"MIXED","steps":[{"kind":"move","target_place_id":"garden"},{"kind":"speech","speech_text":""}],"clarification":""}`, "", false},
		{"model invents text", "想你了。", `{"kind":"DIALOGUE","steps":[{"kind":"speech","speech_text":"别的话。"}],"clarification":""}`, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				modelResponse(w, tc.reply, "stop")
			}))
			defer server.Close()
			provider, err := NewChatProvider(Config{Endpoint: server.URL + "/v1/chat/completions", Model: "fixture", EndpointPolicy: endpointpolicy.TestLocalhostPolicy()})
			if err != nil {
				t.Fatal(err)
			}
			plan, err := provider.UnderstandInteraction(context.Background(), interactionFixtureInput(tc.text))
			if !tc.valid && err == nil || tc.valid && (err != nil || len(plan.Steps) == 0 || plan.Steps[len(plan.Steps)-1].SpeechText != tc.want) {
				t.Fatalf("speech source was incorrect: %+v, %v", plan, err)
			}
		})
	}
}

func TestCompactInteractionStepsRejectExtraOrMissingFields(t *testing.T) {
	for _, proposal := range []string{
		`{"kind":"ACTION","steps":[{"kind":"nonverbal","nonverbal_action":"look_at","target_entity_id":"visible-entity","gesture_code":"","speech_text":"invented"}],"clarification":""}`,
		`{"kind":"ACTION","steps":[{"kind":"nonverbal","nonverbal_action":"look_at","target_entity_id":"visible-entity"}],"clarification":""}`,
		`{"kind":"ACTION","steps":[{"kind":"nonverbal","kind":"speech","nonverbal_action":"look_at","target_entity_id":"visible-entity","gesture_code":""}],"clarification":""}`,
	} {
		if _, err := parseInteractionProposal(proposal); err == nil {
			t.Fatalf("accepted malformed compact proposal: %s", proposal)
		}
	}
	for _, proposal := range []string{
		`{"kind":"DIALOGUE","steps":[{"kind":"speech","speech_text":"想你了。"}],"clarification":""}`,
		`{"kind":"MIXED","steps":[{"kind":"object","object_action":"move","object_id":"cup","anchor_id":"near","target_entity_id":"","offer_id":""},{"kind":"speech","speech_text":"喝一点吧。"}],"clarification":""}`,
	} {
		if _, err := parseInteractionProposal(proposal); err != nil {
			t.Fatalf("rejected valid compact proposal: %v", err)
		}
	}
}
