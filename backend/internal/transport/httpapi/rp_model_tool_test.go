package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/endpointpolicy"
	"corerp.local/backend/internal/storage"
)

func TestRPModelNativeFunctionProposalCommitsOnlyObservableAndReplays(t *testing.T) {
	for _, channel := range []string{"function", "complete_content"} {
		t.Run(channel, func(t *testing.T) {
			ctx := context.Background()
			var calls atomic.Int32
			const private = "fixture-private-role-intent"
			const words = "这是原生函数提案中的获准台词。"
			model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				var request struct {
					Tools    []struct{ Function struct{ Name string } }
					Messages []struct{ Content string }
				}
				if json.NewDecoder(r.Body).Decode(&request) != nil || len(request.Tools) != 1 || request.Tools[0].Function.Name != "propose_rp_decision" {
					t.Error("HTTP override did not select native proposal function")
				}
				var packet struct {
					Character struct {
						CurrentTurn struct {
							Data struct {
								SpeechEventID string `json:"speech_event_id"`
								PlayerSpeech  string `json:"player_speech_text"`
							} `json:"data"`
						} `json:"current_turn"`
					} `json:"character"`
					Sources []struct {
						Ref           string `json:"ref"`
						SourceEventID string `json:"source_event_id"`
					} `json:"grounding_sources"`
					Schema map[string]any `json:"proposal_schema"`
				}
				if len(request.Messages) != 2 || json.Unmarshal([]byte(request.Messages[1].Content), &packet) != nil || packet.Schema["additionalProperties"] != false {
					t.Error("proposal contract did not reach the model message")
				}
				if packet.Character.CurrentTurn.Data.SpeechEventID == "" || packet.Character.CurrentTurn.Data.PlayerSpeech != "你好。" {
					t.Error("actual player speech did not reach grouped current-turn data")
				}
				ref := ""
				for _, source := range packet.Sources {
					if source.SourceEventID == packet.Character.CurrentTurn.Data.SpeechEventID {
						ref = source.Ref
					}
				}
				if ref == "" {
					t.Error("actual player speech has no authorized source handle")
				}
				args := `{"private":{"intent":"` + private + `","emotion":"平和","relationship_stance":"礼貌","basis_event_ids":["` + ref + `"]},"observable":{"action":"respond","text":"` + words + `","speech_tone":"none","introduce_self":false,"expression_code":"none"}}`
				finish := "tool_calls"
				message := map[string]any{"content": "unapproved-new-world-action", "tool_calls": []any{map[string]any{"id": "fixture-call", "type": "function", "function": map[string]any{"name": "propose_rp_decision", "arguments": args}}}}
				if channel == "complete_content" {
					finish, message = "stop", map[string]any{"content": args}
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": finish, "message": message}}})
			}))
			defer model.Close()
			store, handler := openHTTPTestServer(t, ctx, filepath.Join(t.TempDir(), "native-proposal.db"))
			defer store.Close()
			policy, err := endpointpolicy.FromEnvironment("", model.URL+"/v1/chat/completions")
			if err != nil {
				t.Fatal(err)
			}
			handler.(*Server).endpointPolicy = policy
			session := openAuthoredRPModelHTTPSession(t, ctx, store, handler)
			response := performJSON(t, handler, "/api/v1/rp/observe", rpPlayerToken, core.RPSessionReadRequest{SessionID: session.SessionID})
			assertStatus(t, response, http.StatusOK)
			view := decodeData[storage.RPObservation](t, response)
			request := map[string]any{"session_id": session.SessionID, "text": "你好。", "expected_cursor": view.ObservationCursor, "idempotency_key": "native-tool-turn", "model": map[string]any{"endpoint": model.URL + "/v1/chat/completions", "model": "fixture", "api_key": "fixture-key", "decision_format": "tool_call"}}
			response = performJSON(t, handler, "/api/v1/rp/turns/run", rpPlayerToken, request)
			assertStatus(t, response, http.StatusOK)
			turn := decodeData[storage.RPTurnResult](t, response)
			public := strings.Join(turn.NarrativeLines, "\n")
			if turn.Status != "settled" || !strings.Contains(public, words) || strings.Contains(public, private) || strings.Contains(public, "unapproved-new-world-action") {
				t.Fatal("private metadata or unapproved model content became public")
			}
			response = performJSON(t, handler, "/api/v1/rp/turns/run", rpPlayerToken, request)
			assertStatus(t, response, http.StatusOK)
			if !decodeData[storage.RPTurnResult](t, response).Replayed || calls.Load() != 1 {
				t.Fatal("replayed native proposal created another provider call")
			}
			observatory, err := store.ReadRPObservatory(ctx, storage.RPObservatoryRequest{PrincipalID: storage.M2RPPlayerPrincipal, SessionID: session.SessionID})
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := json.Marshal(observatory)
			if err != nil || strings.Contains(string(encoded), private) || strings.Contains(string(encoded), "unapproved-new-world-action") {
				t.Fatal("private proposal channel leaked to public observatory")
			}
		})
	}
}
