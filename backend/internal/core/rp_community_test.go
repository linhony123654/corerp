package core

import "testing"

func TestRPCommunityChangesRequireKnownCurrentLocalRevision(t *testing.T) {
	v1 := RPKnownLaw{InstitutionID: "council", LawID: "quiet", EnactmentEventID: "v1", KnowledgeEventID: "heard1", ScopeKind: "region", PlaceIDs: []string{"cafe"}, EffectiveWorldTime: "2026-09-23T13:00:00Z"}
	v2 := v1
	v2.EnactmentEventID, v2.PreviousEnactmentEventID, v2.KnowledgeEventID, v2.EffectiveWorldTime = "v2", "v1", "heard2", "2026-09-23T14:00:00Z"
	v3 := v2
	v3.EnactmentEventID, v3.PreviousEnactmentEventID, v3.KnowledgeEventID, v3.EffectiveWorldTime, v3.Repealed = "v3", "v2", "heard3", "2026-09-23T15:00:00Z", true
	for _, tc := range []struct {
		at, place, want string
		known           []RPKnownLaw
	}{
		{"2026-09-23T15:00:00Z", "cafe", "", []RPKnownLaw{v1}},
		{"2026-09-23T13:59:59Z", "cafe", "", []RPKnownLaw{v1, v2, v3}},
		{"2026-09-23T14:00:00Z", "cafe", "v2", []RPKnownLaw{v3, v2, v1}},
		{"2026-09-23T15:00:00Z", "cafe", "v3", []RPKnownLaw{v1, v3, v2}},
		{"2026-09-23T15:00:00Z", "home", "", []RPKnownLaw{v1, v2, v3}},
		{"2026-09-23T15:00:00Z", "cafe", "v2", []RPKnownLaw{v1, v2}},
	} {
		got := CurrentKnownRPCommunityChanges(tc.known, tc.at, tc.place)
		if tc.want == "" {
			if len(got) != 0 {
				t.Fatalf("unexpected change: %+v", got)
			}
		} else if len(got) != 1 || got[0].EnactmentEventID != tc.want {
			t.Fatalf("change: %+v want %s", got, tc.want)
		}
	}
	unknown := v2
	unknown.KnowledgeEventID = ""
	if len(CurrentKnownRPCommunityChanges([]RPKnownLaw{unknown}, "2026-09-23T15:00:00Z", "cafe")) != 0 {
		t.Fatal("unsourced knowledge eligible")
	}
	world := v2
	world.ScopeKind = "world"
	if len(CurrentKnownRPCommunityChanges([]RPKnownLaw{world}, "2026-09-23T15:00:00Z", "cafe")) != 0 {
		t.Fatal("world rule mislabeled neighborhood change")
	}
	got := CurrentKnownRPCommunityChanges([]RPKnownLaw{v2}, "2026-09-23T14:00:00Z", "cafe")
	got[0].PlaceIDs[0] = "elsewhere"
	if v2.PlaceIDs[0] != "cafe" {
		t.Fatal("selector mutated knowledge")
	}
}
