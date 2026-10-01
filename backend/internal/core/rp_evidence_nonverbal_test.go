package core

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRPWitnessSupportRetainsTypedTargetAndTimeWithoutInventingUnknowns(t *testing.T) {
	in := RPDecisionInput{NPCEntityID: "observer", Knowledge: []RPDecisionKnowledge{
		{ClaimType: "nonverbal_action", SubjectEntityID: "actor", SourceEventID: "witness", Action: "gesture", GestureCode: "beckon", TargetEntityID: "person_alias", WorldTime: "2026-09-22T10:00:00Z", Text: "招手"},
		{ClaimType: "nonverbal_action", SubjectEntityID: "actor", SourceEventID: "hidden", Action: "nod", Text: "向另一个人点头"},
		{ClaimType: "nonverbal_action", SubjectEntityID: "actor", SourceEventID: "legacy", Text: "笑了笑"},
	}}
	before, _ := HashJSON(in)
	got := RPDecisionEvidenceSupport(in)
	after, _ := HashJSON(in)
	if before != after {
		t.Fatal("support mutated selected witness input")
	}
	typed := 0
	for _, r := range got["witness"] {
		if r.ActorEntityID != "actor" || r.TargetEntityID != "person_alias" || r.WorldTime != in.Knowledge[0].WorldTime {
			t.Fatal("witness scope lost", r)
		}
		if r.Kind == "observed_nonverbal" {
			typed++
			if r.Status != "witnessed_at_time" || len(r.AllowedUses) != 1 || r.AllowedUses[0] != "recorded_nonverbal_at_time" {
				t.Fatal("historical action gained current-state authority", r)
			}
		}
	}
	if typed != 3 {
		t.Fatal("typed action/gesture/target not individually sourced", typed)
	}
	for _, r := range got["hidden"] {
		if r.TargetEntityID != "" || r.WorldTime != "" || strings.HasSuffix(r.Locator, "/target_entity_id") {
			t.Fatal("description supplied unseen target or time", r)
		}
	}
	if len(got["legacy"]) != 1 || got["legacy"][0].Kind != "observed_description" {
		t.Fatal("legacy description invented a typed action", got["legacy"])
	}
	raw, _ := json.Marshal(in.Knowledge[1])
	if strings.Contains(string(raw), "target_entity_id") || strings.Contains(string(raw), "world_time") {
		t.Fatal("missing witness details became provided")
	}
}
