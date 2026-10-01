package decision

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/endpointpolicy"
)

func TestDecisionNonverbalTriggerReachesEveryTransportWithoutPlayerSpeech(t *testing.T) {
	var baseline []decisionWireSourceRef
	for _, tc := range []struct{ format, channel string }{{"json_schema", "content"}, {"json_object", "content"}, {"tool_call", "content"}, {"tool_call", "function"}} {
		t.Run(tc.format+"/"+tc.channel, func(t *testing.T) {
			in := contextFixture()
			in.InterlocutorEntityID = "actor_alias"
			in.NPCEntityID = "target_alias"
			in.TurnID = "event-player-nod"
			in.SpeechEventID = ""
			in.PlayerSpeechText = ""
			in.PlayerSpeechWorldTime = ""
			in.Trigger = &core.RPDecisionTrigger{Kind: "nonverbal", SourceEventID: in.TurnID}
			in.ObservedPlayerAction = &core.RPDecisionObservedAction{SourceEventID: in.TurnID, ActorEntityID: in.InterlocutorEntityID, TargetEntityID: in.NPCEntityID, Action: "nod", PlaceID: in.PlaceID, WorldTime: "then"}
			before, _ := core.HashJSON(in)
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var req struct{ Messages []struct{ Content string } }
				if json.NewDecoder(r.Body).Decode(&req) != nil {
					t.Fatal("bad request")
				}
				var packet decisionContext
				if json.Unmarshal([]byte(req.Messages[1].Content), &packet) != nil {
					t.Fatal("bad packet")
				}
				assertDecisionCharacter(t, packet.Character, in)
				verifyDecisionSupportPaths(t, in, packet)
				action := presentedValue(t, packet.Character.CurrentTurn, "observed_player_action").(map[string]any)
				if action["actor_entity_id"] != "actor_alias" || action["target_entity_id"] != "target_alias" || action["action"] != "nod" || presentedValue(t, packet.Character.CurrentTurn, "player_speech_text") != "" || presentedValue(t, packet.Character.CurrentTurn, "speech_event_id") != "" {
					t.Fatal("action mutated aliases or synthesized speech")
				}
				if _, duplicated := packet.Character.TypedDomains.Data["observed_player_action"]; duplicated {
					t.Fatal("action duplicated as generic domain")
				}
				if baseline == nil {
					baseline = packet.GroundingSources
				} else if !reflect.DeepEqual(baseline, packet.GroundingSources) {
					t.Fatal("transport changed action evidence")
				}
				if len(packet.GroundingSources) != 1 || packet.GroundingSources[0].SourceEventID != in.TurnID {
					t.Fatal("lost current action anchor")
				}
				uses := 0
				for i := range packet.GroundingSources[0].SupportRanges {
					scope := expandDecisionSupportRange(t, packet, packet.GroundingSources[0], i)
					if scope.Kind == "accepted_speech" {
						t.Fatal("nod classified as speech")
					}
					if scope.Kind == "observed_player_action" {
						uses++
						if scope.Locator != "/current_turn/data/observed_player_action" {
							t.Fatal("wrong actual layout path", scope)
						}
					}
				}
				if uses != 1 {
					t.Fatal("action scope missing")
				}
				args := strings.Replace(wireDecision(`{"action":"silence","expression_code":"nod"}`), "event-player", packet.GroundingSources[0].Ref, 1)
				if tc.channel == "function" {
					_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": "tool_calls", "message": map[string]any{"tool_calls": []any{proposalTool(args)}}}}})
				} else {
					modelResponse(w, args, "stop")
				}
			}))
			defer server.Close()
			p, err := NewChatProvider(Config{Endpoint: server.URL + "/v1/chat/completions", Model: "fixture", DecisionFormat: tc.format, EndpointPolicy: endpointpolicy.TestLocalhostPolicy()})
			if err != nil {
				t.Fatal(err)
			}
			got, err := p.Propose(context.Background(), in)
			if err != nil || calls != 1 || got.Action != "silence" || got.ExpressionCode != "nod" || got.Text != "" || got.Private == nil || !reflect.DeepEqual(got.Private.BasisEventIDs, []string{in.TurnID}) || core.ValidateRPDecisionProposal(in, got) != nil {
				t.Fatal("zero-speech action response invalid", got, err, calls)
			}
			after, _ := core.HashJSON(in)
			if before != after {
				t.Fatal("adapter mutated authorized action input")
			}
		})
	}
}

func TestDecisionNonverbalBudgetKeepsRequiredWitness(t *testing.T) {
	in := contextFixture()
	in.NPCEntityID = "target_alias_" + strings.Repeat("n", 65)
	in.InterlocutorEntityID = "actor_alias_" + strings.Repeat("p", 66)
	in.TurnID = "event_" + strings.Repeat("a", 69)
	in.Trigger = &core.RPDecisionTrigger{Kind: "nonverbal", SourceEventID: in.TurnID}
	in.SpeechEventID = ""
	in.PlayerSpeechText = ""
	in.PlayerSpeechWorldTime = ""
	in.ObservedPlayerAction = &core.RPDecisionObservedAction{SourceEventID: in.TurnID, ActorEntityID: in.InterlocutorEntityID, TargetEntityID: in.NPCEntityID, Action: "nod", PlaceID: in.PlaceID, WorldTime: "2026-09-22T00:00:00Z"}
	for i := 0; i < 300; i++ {
		id := fmt.Sprintf("event_%03d_", i)
		id += strings.Repeat("x", 75-len(id))
		in.RecentDialogue = append(in.RecentDialogue, core.RPDecisionDialogue{EventID: id, SpeakerEntityID: in.InterlocutorEntityID, Text: strings.Repeat("old words ", 55), WorldTime: "2026-09-22T00:00:00Z"})
	}
	selected, err := core.SelectRPDecisionContext(in, core.DefaultRPDecisionContextBudgetBytes)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(selected.ObservedPlayerAction, in.ObservedPlayerAction) || !reflect.DeepEqual(selected.Trigger, in.Trigger) {
		t.Fatal("required witness dropped under history pressure")
	}
	schema, err := decisionResponseSchema(selected)
	if err != nil {
		t.Fatal(err)
	}
	packet, err := buildDecisionContext(selected, schema)
	if err != nil {
		t.Fatal(err)
	}
	verifyDecisionSupportPaths(t, selected, packet)
	assertDecisionCharacter(t, packet.Character, selected)
	raw, err := json.Marshal(packet)
	if err != nil || len(raw) > maxContextBytes {
		t.Fatal("long-ID action packet exceeded full budget", len(raw), err)
	}
	t.Logf("selected_bytes=%d full_packet_bytes=%d remaining_history=%d", selected.ContextSelection.EncodedBytes, len(raw), len(selected.RecentDialogue))
}
