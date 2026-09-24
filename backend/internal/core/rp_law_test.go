package core

import "testing"

func TestRPLawVersionSelectionUsesOnlySuppliedKnowledge(t *testing.T) {
	v1 := RPKnownLaw{InstitutionID: "one", LawID: "quiet", EnactmentEventID: "v1", ScopeKind: "region", PlaceIDs: []string{"cafe"}, EffectiveWorldTime: "2026-09-23T13:00:00Z", ProhibitedAction: "speak", FineMinor: 2}
	v2 := v1
	v2.EnactmentEventID, v2.PreviousEnactmentEventID, v2.EffectiveWorldTime, v2.FineMinor = "v2", "v1", "2026-09-23T14:00:00Z", 5
	repeal := v2
	repeal.EnactmentEventID, repeal.PreviousEnactmentEventID, repeal.EffectiveWorldTime, repeal.Repealed = "v3", "v2", "2026-09-23T15:00:00Z", true
	for _, tc := range []struct {
		name, at, place, want string
		known                 []RPKnownLaw
	}{
		{"old-only", "2026-09-23T16:00:00Z", "cafe", "v1", []RPKnownLaw{v1}},
		{"future-amendment", "2026-09-23T13:00:00Z", "cafe", "v1", []RPKnownLaw{v2, v1}},
		{"future-repeal", "2026-09-23T14:00:00Z", "cafe", "v2", []RPKnownLaw{repeal, v1, v2}},
		{"heard-repeal-without-intermediate", "2026-09-23T15:00:00Z", "cafe", "", []RPKnownLaw{repeal, v1}},
		{"outside-scope", "2026-09-23T14:00:00Z", "home", "", []RPKnownLaw{v1, v2}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := EffectiveRPLaws(tc.known, tc.at, tc.place)
			if tc.want == "" {
				if len(got) != 0 {
					t.Fatalf("unexpected law %+v", got)
				}
				return
			}
			if len(got) != 1 || got[0].EnactmentEventID != tc.want {
				t.Fatalf("got %+v want %s", got, tc.want)
			}
		})
	}
	other := v1
	other.InstitutionID, other.EnactmentEventID = "two", "other"
	got := EffectiveRPLaws([]RPKnownLaw{v1, repeal, other}, "2026-09-23T16:00:00Z", "cafe")
	if len(got) != 1 || got[0].EnactmentEventID != "other" {
		t.Fatalf("repeal crossed institution: %+v", got)
	}
}
