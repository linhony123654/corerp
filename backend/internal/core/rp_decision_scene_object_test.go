package core

import (
	"reflect"
	"testing"
)

func TestRPDecisionSceneObjectEvidenceDoesNotGrantActionOrUnlistedAffordance(t *testing.T) {
	in := RPDecisionInput{NPCEntityID: "observer_alias", WorldTime: "2026-10-01T12:00:00Z",
		LegalActions: []string{"respond"}, SceneObjects: []RPDecisionSceneObject{{
			ObjectID: "door", DisplayName: "前门", Kind: "door", State: "closed", PlaceID: "room",
			WorldTime: "2026-10-01T12:00:00Z", DefinitionSourceEventID: "definition", StateSourceEventID: "state",
		}}}
	support := RPDecisionEvidenceSupport(in)
	definition, state := support["definition"], support["state"]
	if len(definition) != 1 || len(state) != 1 || definition[0].Kind != "observed_object_definition" ||
		state[0].Kind != "observed_object_state" || state[0].Locator != "/scene_objects/0/state" ||
		state[0].WorldTime != in.WorldTime || state[0].ActorEntityID != "observer_alias" ||
		!reflect.DeepEqual(state[0].AllowedUses, []string{"listed_object_state_at_snapshot"}) {
		t.Fatalf("object evidence lost snapshot or mixed definition/state uses: %#v", support)
	}
	proposal := RPDecisionProposal{Action: "respond", Text: "前门关着。", Private: &RPDecisionPrivate{BasisEventIDs: []string{"definition", "state"}}}
	if reason, err := ValidateRPDecisionProposalEvidence(in, proposal); reason != "" || err != nil {
		t.Fatalf("authorized snapshot basis rejected: %q %v", reason, err)
	}
	proposal = RPDecisionProposal{Action: "open"}
	if reason, err := ValidateRPDecisionProposalEvidence(in, proposal); reason != "action_not_legal" || err == nil {
		t.Fatalf("readonly scene object granted a new NPC action: %q %v", reason, err)
	}
	in.SceneObjects = nil
	if sources := RPDecisionEvidenceSupport(in); len(sources) != 0 {
		t.Fatalf("unprovided scene invented evidence: %#v", sources)
	}
}
