package core

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestRPEvidenceSupportSeparatesSameEventAndAncestralPrivateBasis(t *testing.T) {
	in := rpSelectionFixture()
	in.PersonaSourceEventID = "same"
	in.Relationships = []RPCharacterRelationship{{SourceEventID: "same", SubjectEntityID: "player", Role: "祖母", AddressTo: []string{"玉儿"}, SelfReference: "老祖宗"}}
	in.RecentDialogue = []RPDecisionDialogue{{EventID: "same", SpeakerEntityID: "player", Text: "我已经把书还了。", WorldTime: "then"}}
	in.RecentPrivateDecisions = []RPDecisionPrivateMemory{{SourceEventID: "same", WorldTime: "earlier", InterlocutorEntityID: "other", Private: RPDecisionPrivate{Intent: "准备还书", BasisEventIDs: []string{"ancestor", "same", "ancestor"}}}}
	before, _ := HashJSON(in)
	got := RPDecisionEvidenceSupport(in)
	kinds := map[string]int{}
	for _, r := range got["same"] {
		kinds[r.Kind]++
		if r.Kind == "accepted_speech" && (r.ActorEntityID != "player" || r.Status != "said" || !reflect.DeepEqual(r.AllowedUses, []string{"attributed_speech", "exact_quote"})) {
			t.Fatal("speech became fulfillment", r)
		}
	}
	if kinds["authored_persona"] != 1 || kinds["authored_relationship"] != 3 || kinds["accepted_speech"] != 1 || kinds["own_private"] != 1 || kinds["provenance_only"] != 1 {
		t.Fatal("same event collapsed", kinds)
	}
	for _, r := range got["ancestor"] {
		if r.Kind != "provenance_only" || !reflect.DeepEqual(r.AllowedUses, []string{"provenance"}) {
			t.Fatal("ancestral ID became evidence", r)
		}
	}
	after, _ := HashJSON(in)
	if before != after || !reflect.DeepEqual(got, RPDecisionEvidenceSupport(in)) {
		t.Fatal("compiler mutated context or is nondeterministic")
	}
	// Repeated pointers remain distinct ranges, not duplicated entries at one locator.
	seen := map[string]bool{}
	for _, ranges := range got {
		for _, r := range ranges {
			if seen[r.Locator] {
				t.Fatal("duplicate locator", r)
			}
			seen[r.Locator] = true
		}
	}
}

func TestRPEvidenceSupportRetainsPartialUnknownAndPlannedBoundaries(t *testing.T) {
	in := rpSelectionFixture()
	in.HeardPlayerHistory = []RPDecisionSpeechExcerpt{{EventID: "partial", Excerpt: "我答应……", Truncated: true}, {EventID: "reference", TextFromEvent: true}}
	in.Knowledge = []RPDecisionKnowledge{{SourceEventID: "unknown-domain", ClaimType: "future_unrecognized_fact", SubjectEntityID: "other", Text: "这不是可自动信任的完成事实"}}
	in.NextSchedule = &RPDecisionSchedule{SourceEventID: "schedule", DelaySourceEventID: "delay", WorldTime: "later", ActivityCode: "read"}
	in.OwnActions = []RPOwnAction{{EventID: "started", Action: "activity", Status: "in_progress", WorldTime: "then"}, {EventID: "completed", Action: "activity", Status: "completed", WorldTime: "later"}, {EventID: "unknown-action", Action: "future_action"}}
	got := RPDecisionEvidenceSupport(in)
	for _, id := range []string{"unknown-domain", "delay", "unknown-action"} {
		if got[id][0].Kind != "provenance_only" {
			t.Fatal("unknown reference promoted", id, got[id])
		}
	}
	for _, id := range []string{"partial", "reference"} {
		for _, use := range got[id][0].AllowedUses {
			if use == "exact_quote" {
				t.Fatal("incomplete speech became complete", id)
			}
		}
	}
	if got["schedule"][0].Status != "planned" || got["started"][0].Status != "in_progress" || got["completed"][0].Status != "completed" {
		t.Fatal("time/status collapsed", got)
	}
	if got["reference"][0].Completeness != "reference_only" {
		t.Fatal("missing payload treated as full text")
	}
}

func TestRPEvidenceSupportUsesOnlySelectedProjectedFields(t *testing.T) {
	in := rpSelectionFixture()
	in.NPCEntityID = "actor_alias"
	in.InterlocutorEntityID = "peer_alias"
	in.Relationships = nil
	in.RecentDialogue = []RPDecisionDialogue{{EventID: "kept", SpeakerEntityID: "peer_alias", Text: "word", WorldTime: "then"}, {EventID: "dropped", SpeakerEntityID: "canonical-secret", Text: strings.Repeat("too large", 1000)}}
	got, err := SelectRPDecisionContext(in, 2500)
	if err != nil {
		t.Fatal(err)
	}
	support := RPDecisionEvidenceSupport(got)
	if _, exists := support["dropped"]; exists {
		t.Fatal("catalog retained discarded evidence")
	}
	raw, _ := json.Marshal(support)
	if strings.Contains(string(raw), "canonical-secret") || !strings.Contains(string(raw), "actor_alias") || !strings.Contains(string(raw), "peer_alias") {
		t.Fatal("compiler bypassed projection", string(raw))
	}
	for id := range RPDecisionEvidenceEventIDs(got) {
		if len(support[id]) == 0 {
			t.Fatal("source missing conservative scope", id)
		}
	}
}

func TestRPEvidenceSupportCoversKnownKnowledgeAndOwnActionProjection(t *testing.T) {
	in := rpSelectionFixture()
	in.Knowledge = []RPDecisionKnowledge{
		{SourceEventID: "presence", ClaimType: "agent_presence", SubjectEntityID: "peer_alias", PlaceID: "hall"},
		{SourceEventID: "gesture", ClaimType: "nonverbal_action", SubjectEntityID: "peer_alias", Text: "点头"},
		{SourceEventID: "object", ClaimType: "object_interaction", SubjectEntityID: "peer_alias", Text: "把书放在桌上"},
	}
	in.OwnActions = []RPOwnAction{
		{EventID: "speech", Action: "speech", Text: "我会读书。"},
		{EventID: "move", Action: "leave", PlaceID: "hall"},
		{EventID: "start", Action: "activity", ActivityCode: "read", Status: "in_progress"},
		{EventID: "end", Action: "activity", ActivityCode: "read", Status: "completed"},
		{EventID: "cancel", Action: "activity", ActivityCode: "read", Status: "cancelled"},
		{EventID: "nonverbal", Action: "nonverbal", ActivityCode: "nod"},
		{EventID: "offer", Action: "offer", ActivityCode: "rp_object", Status: "completed"},
		{EventID: "receive", Action: "receive", ActivityCode: "rp_object", Status: "completed"},
	}
	support := RPDecisionEvidenceSupport(in)
	if r := support["presence"][0]; r.Kind != "observed_presence" || r.WorldTime != "" || r.ActorEntityID != "peer_alias" || r.Locator != "/knowledge/0/place_id" {
		t.Fatal("presence acquired unsupported time", r)
	}
	for _, id := range []string{"gesture", "object"} {
		if support[id][0].Kind != "observed_description" {
			t.Fatal("typed observation lost", id)
		}
	}
	for _, id := range []string{"move", "start", "end", "cancel", "nonverbal", "offer", "receive"} {
		if support[id][0].Kind != "own_observable" {
			t.Fatal("own observable lost", id)
		}
	}
	if support["speech"][0].Kind != "accepted_speech" {
		t.Fatal("own promise promoted to outcome")
	}
}
