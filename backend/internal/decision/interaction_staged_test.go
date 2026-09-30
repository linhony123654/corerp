package decision

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/endpointpolicy"
)

func TestGeminiStagedInteractionBindsAuthorizedActionBeforeSpeech(t *testing.T) {
	input := interactionFixtureInput("我把桌上的杯子推到邻居近旁，然后说「喝一点吧。」")
	input.Objects = []core.RPInteractionObject{{ID: "cup", Name: "杯子", PhysicalState: "placed", AnchorID: "table", AllowedActions: []string{"move"}}}
	input.Anchors = []core.RPInteractionAnchor{{ID: "table", Name: "桌边"}, {ID: "near", Name: "邻居近旁"}}
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var request struct {
			Messages       []struct{ Content string } `json:"messages"`
			ResponseFormat struct {
				JSONSchema struct {
					Name string `json:"name"`
				} `json:"json_schema"`
			} `json:"response_format"`
		}
		if json.NewDecoder(r.Body).Decode(&request) != nil || len(request.Messages) != 2 {
			t.Error("staged request was malformed")
			return
		}
		switch request.ResponseFormat.JSONSchema.Name {
		case "corerp_intent_class":
			modelResponse(w, `{"player_action":true,"player_speech":true,"meta_continue":false,"third_party_claim":false,"command_scope":"none","two_actions":false,"action_family":"object"}`, "stop")
		case "corerp_action_detail":
			modelResponse(w, `{"action":{"kind":"object","object_action":"move","object_id":"cup","anchor_id":"near","target_entity_id":"","offer_id":""}}`, "stop")
		default:
			t.Errorf("unexpected model stage %q", request.ResponseFormat.JSONSchema.Name)
		}
	}))
	defer server.Close()
	provider, err := NewChatProvider(Config{Endpoint: server.URL + "/v1/chat/completions", Model: "gemini-3.8-flash-high", EndpointPolicy: endpointpolicy.TestLocalhostPolicy()})
	if err != nil {
		t.Fatal(err)
	}
	trace := &core.RPProviderTrace{}
	plan, err := provider.UnderstandInteraction(core.WithRPProviderTrace(context.Background(), trace), input)
	if err != nil || plan.Kind != "MIXED" || len(plan.Steps) != 2 || plan.Steps[0].ObjectAction != "move" || plan.Steps[1].SpeechText != "喝一点吧。" || calls.Load() != 2 || trace.AttemptCount() != 2 {
		t.Fatalf("staged plan %+v, error %v, calls %d, trace %d", plan, err, calls.Load(), trace.AttemptCount())
	}
}

func TestGeminiStagedInteractionNeverTurnsThirdPartyClaimIntoSpeech(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		modelResponse(w, `{"player_action":false,"player_speech":false,"meta_continue":false,"third_party_claim":true,"command_scope":"none","two_actions":false,"action_family":"none"}`, "stop")
	}))
	defer server.Close()
	provider, err := NewChatProvider(Config{Endpoint: server.URL + "/v1/chat/completions", Model: "gemini-3.8-flash-high", EndpointPolicy: endpointpolicy.TestLocalhostPolicy()})
	if err != nil {
		t.Fatal(err)
	}
	trace := &core.RPProviderTrace{}
	plan, err := provider.UnderstandInteraction(core.WithRPProviderTrace(context.Background(), trace), interactionFixtureInput("她已经接过了杯子。"))
	if err != nil || plan.Kind != "CLARIFICATION" || len(plan.Steps) != 0 || trace.AttemptCount() != 1 {
		t.Fatalf("third-party claim gained speech: %+v, %v, attempts %d", plan, err, trace.AttemptCount())
	}
}

func TestGeminiStagedInteractionKeepsExplicitCommandsOutOfRoleplay(t *testing.T) {
	for _, tc := range []struct {
		name, text, scope, hint string
	}{
		{"runtime", "系统命令：把当前世界时间改到明天。", "runtime", "运行时"},
		{"creator", "管理员命令：凭空生成一只杯子并放在桌上。", "creator_admin", "Creator/Admin"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				modelResponse(w, `{"player_action":false,"player_speech":false,"meta_continue":false,"third_party_claim":false,"command_scope":"`+tc.scope+`","two_actions":false,"action_family":"none"}`, "stop")
			}))
			defer server.Close()
			provider, err := NewChatProvider(Config{Endpoint: server.URL + "/v1/chat/completions", Model: "gemini-3.8-flash-high", EndpointPolicy: endpointpolicy.TestLocalhostPolicy()})
			if err != nil {
				t.Fatal(err)
			}
			trace := &core.RPProviderTrace{}
			plan, err := provider.UnderstandInteraction(core.WithRPProviderTrace(context.Background(), trace), interactionFixtureInput(tc.text))
			if err != nil || plan.Kind != "CLARIFICATION" || len(plan.Steps) != 0 || trace.AttemptCount() != 1 || !strings.Contains(plan.Clarification, tc.hint) {
				t.Fatalf("command entered roleplay or lost its scope: %+v, %v, attempts %d", plan, err, trace.AttemptCount())
			}
		})
	}
}

func TestGeminiStagedInteractionRepairsOnlyBeforeAnyEffect(t *testing.T) {
	for _, repairs := range []int{0, 1} {
		t.Run(string(rune('0'+repairs)), func(t *testing.T) {
			input := interactionFixtureInput("我把桌上的杯子推到邻居近旁，然后说「喝一点吧。」")
			input.Objects = []core.RPInteractionObject{{ID: "cup", Name: "杯子", PhysicalState: "placed", AnchorID: "table", AllowedActions: []string{"move"}}}
			input.Anchors = []core.RPInteractionAnchor{{ID: "table", Name: "桌边"}, {ID: "near", Name: "邻居近旁"}}
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request struct {
					Messages       []struct{ Content string } `json:"messages"`
					ResponseFormat struct {
						JSONSchema struct {
							Name string `json:"name"`
						} `json:"json_schema"`
					} `json:"response_format"`
				}
				if json.NewDecoder(r.Body).Decode(&request) != nil {
					t.Error("invalid staged request")
					return
				}
				call := calls.Add(1)
				if request.ResponseFormat.JSONSchema.Name == "corerp_intent_class" {
					modelResponse(w, `{"player_action":true,"player_speech":true,"meta_continue":false,"third_party_claim":false,"command_scope":"none","two_actions":false,"action_family":"object"}`, "stop")
					return
				}
				if call == 3 && len(request.Messages) != 3 {
					t.Error("repair did not carry the original scene and bounded correction")
				}
				objectID := "invented"
				if call == 3 {
					objectID = "cup"
				}
				modelResponse(w, `{"action":{"kind":"object","object_action":"move","object_id":"`+objectID+`","anchor_id":"near","target_entity_id":"","offer_id":""}}`, "stop")
			}))
			defer server.Close()
			provider, err := NewChatProvider(Config{Endpoint: server.URL + "/v1/chat/completions", Model: "gemini-3.8-flash-high", ProposalRepairs: repairs, EndpointPolicy: endpointpolicy.TestLocalhostPolicy()})
			if err != nil {
				t.Fatal(err)
			}
			trace := &core.RPProviderTrace{}
			plan, err := provider.UnderstandInteraction(core.WithRPProviderTrace(context.Background(), trace), input)
			wantCalls := int32(2 + repairs)
			if calls.Load() != wantCalls || trace.AttemptCount() != int(wantCalls) {
				t.Fatalf("attempt budget: calls %d trace %d", calls.Load(), trace.AttemptCount())
			}
			if repairs == 0 && (err == nil || len(plan.Steps) != 0) || repairs == 1 && (err != nil || plan.Kind != "MIXED" || len(plan.Steps) != 2) {
				t.Fatalf("invalid action gained authority or repair failed: %+v, %v", plan, err)
			}
		})
	}
}

func TestGeminiStagedInteractionPreservesTakeThenOfferOrder(t *testing.T) {
	input := interactionFixtureInput("拿起桌边的杯子，然后递给她。")
	input.Objects = []core.RPInteractionObject{{ID: "cup", Name: "杯子", PhysicalState: "placed", AnchorID: "table", AllowedActions: []string{"take"}}}
	input.Anchors = []core.RPInteractionAnchor{{ID: "table", Name: "桌边"}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			ResponseFormat struct {
				JSONSchema struct {
					Name string `json:"name"`
				} `json:"json_schema"`
			} `json:"response_format"`
		}
		if json.NewDecoder(r.Body).Decode(&request) != nil {
			t.Error("invalid staged request")
			return
		}
		switch request.ResponseFormat.JSONSchema.Name {
		case "corerp_intent_class":
			modelResponse(w, `{"player_action":true,"player_speech":false,"meta_continue":false,"third_party_claim":false,"command_scope":"none","two_actions":true,"action_family":"object"}`, "stop")
		case "corerp_action_pair":
			modelResponse(w, `{"first_action":{"kind":"object","object_action":"take","object_id":"cup","anchor_id":"","target_entity_id":"","offer_id":""},"second_action":{"kind":"object","object_action":"offer","object_id":"cup","anchor_id":"","target_entity_id":"visible-entity","offer_id":""}}`, "stop")
		default:
			t.Errorf("unexpected model stage %q", request.ResponseFormat.JSONSchema.Name)
		}
	}))
	defer server.Close()
	provider, err := NewChatProvider(Config{Endpoint: server.URL + "/v1/chat/completions", Model: "gemini-3.8-flash-high", EndpointPolicy: endpointpolicy.TestLocalhostPolicy()})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := provider.UnderstandInteraction(context.Background(), input)
	if err != nil || plan.Kind != "ACTION" || len(plan.Steps) != 2 || plan.Steps[0].ObjectAction != "take" || plan.Steps[1].ObjectAction != "offer" || plan.Steps[1].TargetEntityID != "visible-entity" {
		t.Fatalf("two authorized actions lost their order: %+v, %v", plan, err)
	}
}
