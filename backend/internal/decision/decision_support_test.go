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

func TestDecisionSupportCatalogSameEventProjectedActorsAcrossTransports(t *testing.T) {
	var baseline []decisionSourceRef
	for _, tc := range []struct{ format, channel string }{{"json_schema", "content"}, {"json_object", "content"}, {"tool_call", "content"}, {"tool_call", "function"}} {
		t.Run(tc.format+"/"+tc.channel, func(t *testing.T) {
			in := contextFixture()
			in.NPCEntityID = "self_alias"
			in.InterlocutorEntityID = "peer_alias"
			in.Relationships = []core.RPCharacterRelationship{{SourceEventID: in.SpeechEventID, SubjectEntityID: "peer_alias", Role: "祖母"}}
			in.RecentPrivateDecisions = []core.RPDecisionPrivateMemory{{SourceEventID: in.SpeechEventID, WorldTime: "old-time", InterlocutorEntityID: "previous_alias", Private: core.RPDecisionPrivate{Intent: "private-text-not-for-catalog", BasisEventIDs: []string{"ancestor"}}}}
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
				if packet.EvidenceSupportVersion != core.RPEvidenceSupportVersion || !reflect.DeepEqual(packet.Character, in) {
					t.Fatal("input contract changed")
				}
				if baseline == nil {
					baseline = packet.GroundingSources
				} else if !reflect.DeepEqual(baseline, packet.GroundingSources) {
					t.Fatal("transport changed support ranges")
				}
				raw, _ := json.Marshal(packet.GroundingSources)
				if strings.Contains(string(raw), "private-text-not-for-catalog") {
					t.Fatal("catalog copied private text")
				}
				if !strings.Contains(string(raw), "peer_alias") || !strings.Contains(string(raw), "self_alias") {
					t.Fatal("aliases lost")
				}
				args := wireDecision(`{"action":"wait","expression_code":"none"}`)
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
			if _, err = p.Propose(context.Background(), in); err != nil || calls != 1 {
				t.Fatal("existing reply contract failed", err, calls)
			}
		})
	}
}

func TestDecisionSupportMetadataCountsTowardPacketBudget(t *testing.T) {
	in := contextFixture()
	// A source-rich but bounded selected packet remains usable with its catalog.
	for i := 0; i < 80; i++ {
		in.RecentDialogue = append(in.RecentDialogue, core.RPDecisionDialogue{EventID: fmt.Sprintf("heard-%d", i), SpeakerEntityID: in.InterlocutorEntityID, Text: strings.Repeat("heard words ", 35), WorldTime: in.WorldTime})
	}
	selected, err := core.SelectRPDecisionContext(in, core.DefaultRPDecisionContextBudgetBytes)
	if err != nil {
		t.Fatal(err)
	}
	schema, err := decisionResponseSchema(selected)
	if err != nil {
		t.Fatal(err)
	}
	packet := decisionContext{Version: "corerp.decision.v3", EvidenceSupportVersion: core.RPEvidenceSupportVersion, Character: selected, GroundingSources: decisionSourceRefs(selected), ProposalSchema: schema}
	raw, err := json.Marshal(packet)
	if err != nil || len(raw) > maxContextBytes {
		t.Fatal("bounded selected packet exceeded full budget", len(raw), err)
	}
	// No metadata may escape the existing pre-network budget guard.
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		modelResponse(w, wireDecision(`{"action":"wait","expression_code":"none"}`), "stop")
	}))
	defer server.Close()
	p, err := NewChatProvider(Config{Endpoint: server.URL + "/v1/chat/completions", Model: "fixture", EndpointPolicy: endpointpolicy.TestLocalhostPolicy()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = p.Propose(context.Background(), selected); err != nil || calls != 1 {
		t.Fatal("bounded catalog prevented normal request", err, calls)
	}
	// Keep character itself below the packet limit, push only combined metadata over it.
	in.RecentDialogue = nil
	for i := 0; i < 420; i++ {
		in.RecentDialogue = append(in.RecentDialogue, core.RPDecisionDialogue{EventID: fmt.Sprintf("source-%d", i), SpeakerEntityID: in.InterlocutorEntityID, Text: "x"})
	}
	character, _ := json.Marshal(in)
	if len(character) >= maxContextBytes {
		t.Fatal("fixture character already exceeds limit")
	}
	calls = 0
	if _, err = p.Propose(context.Background(), in); err == nil || !strings.Contains(err.Error(), "context exceeds budget") || calls != 0 {
		t.Fatal("metadata bypassed packet guard", err, calls)
	}
}

func TestDecisionSupportProductionBoundsNearSelectionBudget(t *testing.T) {
	in := contextFixture()
	sizedID := func(label string, size int) string {
		return label + strings.Repeat("x", size-len(label))
	}
	eventID := func(label string) string { return sizedID("event_"+label+"_", 75) }
	actorID := func(label string) string { return sizedID("studio_actor_"+label+"_", 78) }
	in.NPCEntityID = actorID("self")
	in.InterlocutorEntityID = actorID("peer")
	in.SpeechEventID = eventID("current")
	in.WorldTime = "2026-09-22T00:00:00Z"
	// Match the storage caps: 4 exchange groups / 6000 runes, 4 sketches,
	// 16 recent utterances, 40 heard excerpts, 40 own actions, 20 knowledge.
	for i := 0; i < 4; i++ {
		basis := make([]string, 8)
		for j := range basis {
			basis[j] = eventID(fmt.Sprintf("ancestor-%d-%d", i, j))
		}
		in.RelevantDialogue = append(in.RelevantDialogue, core.RPDecisionExchange{PeerContext: true, Dialogue: []core.RPDecisionDialogue{{EventID: eventID(fmt.Sprintf("question-%d", i)), SpeakerEntityID: in.InterlocutorEntityID, Text: strings.Repeat("问", 750), WorldTime: in.WorldTime}, {EventID: eventID(fmt.Sprintf("answer-%d", i)), SpeakerEntityID: in.NPCEntityID, Text: strings.Repeat("答", 750), WorldTime: in.WorldTime}}})
		in.RecentPrivateDecisions = append(in.RecentPrivateDecisions, core.RPDecisionPrivateMemory{SourceEventID: eventID(fmt.Sprintf("private-%d", i)), WorldTime: in.WorldTime, InterlocutorEntityID: in.InterlocutorEntityID, Private: core.RPDecisionPrivate{Intent: strings.Repeat("愿", 160), Emotion: strings.Repeat("情", 80), RelationshipStance: strings.Repeat("亲", 80), BasisEventIDs: basis}})
	}
	for i := 0; i < 16; i++ {
		in.RecentDialogue = append(in.RecentDialogue, core.RPDecisionDialogue{EventID: eventID(fmt.Sprintf("recent-%d", i)), SpeakerEntityID: in.InterlocutorEntityID, Text: strings.Repeat("word ", 80), WorldTime: in.WorldTime})
	}
	for i := 0; i < 40; i++ {
		in.HeardPlayerHistory = append(in.HeardPlayerHistory, core.RPDecisionSpeechExcerpt{EventID: eventID(fmt.Sprintf("heard-%d", i)), Excerpt: strings.Repeat("字", 100), Truncated: true, WorldTime: in.WorldTime})
		in.OwnActions = append(in.OwnActions, core.RPOwnAction{EventID: eventID(fmt.Sprintf("action-%d", i)), Action: "activity", ActivityCode: "read", Status: "completed", WorldTime: in.WorldTime})
	}
	for i := 0; i < 20; i++ {
		in.Knowledge = append(in.Knowledge, core.RPDecisionKnowledge{SourceEventID: eventID(fmt.Sprintf("knowledge-%d", i)), ClaimType: "agent_presence", SubjectEntityID: actorID(fmt.Sprintf("visible-%d", i)), PlaceID: in.PlaceID})
	}
	in.NextSchedule = &core.RPDecisionSchedule{SourceEventID: eventID("schedule"), DelaySourceEventID: eventID("delay"), WorldTime: in.WorldTime, ActivityCode: "work", PlaceID: "office"}
	// Vary required snapshot size until the selected character approaches 64KiB.
	in.PersonaSourceEventID = eventID("persona")
	var selected core.RPDecisionInput
	var err error
	for n := 0; n <= 15000; n += 250 {
		in.Persona = strings.Repeat("p", n)
		selected, err = core.SelectRPDecisionContext(in, core.DefaultRPDecisionContextBudgetBytes)
		if err != nil {
			t.Fatal(err)
		}
		if selected.ContextSelection.EncodedBytes > 63000 {
			break
		}
	}
	if selected.ContextSelection.EncodedBytes < 63000 || len(selected.RelevantDialogue) != 4 || len(selected.RecentPrivateDecisions) != 4 {
		t.Fatal("fixture did not reach budget while retaining peer context", selected.ContextSelection)
	}
	schema, err := decisionResponseSchema(selected)
	if err != nil {
		t.Fatal(err)
	}
	packet := decisionContext{Version: "corerp.decision.v3", EvidenceSupportVersion: core.RPEvidenceSupportVersion, Character: selected, GroundingSources: decisionSourceRefs(selected), ProposalSchema: schema}
	raw, err := json.Marshal(packet)
	t.Logf("character_bytes=%d wrapper_bytes=%d total_packet_bytes=%d limit=%d retained_groups=%d retained_private=%d retained_recent=%d retained_heard=%d retained_own=%d retained_knowledge=%d actor_id_length=%d event_id_length=%d", selected.ContextSelection.EncodedBytes, len(raw)-selected.ContextSelection.EncodedBytes, len(raw), maxContextBytes, len(selected.RelevantDialogue), len(selected.RecentPrivateDecisions), len(selected.RecentDialogue), len(selected.HeardPlayerHistory), len(selected.OwnActions), len(selected.Knowledge), len(in.NPCEntityID), len(in.SpeechEventID))
	if err != nil || len(raw) > maxContextBytes {
		t.Fatal("production-bounded packet exceeded budget", len(raw), err)
	}
}
