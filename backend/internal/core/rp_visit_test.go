package core

import "testing"

func TestRPVisitSourceUsesOwnMemoryAndSevenDayGap(t *testing.T) {
	source := RPVisitSource{ActorID: "ada", Kind: "old_friend_place", PlaceID: "cafe", PlaceSourceEventID: "place", MemorySourceEventID: "meeting", RememberedWorldTime: "2026-09-22T12:00:00Z", ObservationID: "observation", FriendID: "bo", RelationshipSourceEventID: "gift"}
	in := RPDecisionInput{NPCEntityID: "ada", PlaceID: "home", WorldTime: "2026-09-29T12:00:00Z", Life: &RPLifeContext{Relationships: []RPRelationship{{SubjectEntityID: "bo", Trust: 2, SourceEventIDs: []string{"gift"}}}}}
	if !RPVisitSourceEligible(in, source) {
		t.Fatal("sourced old friend missing")
	}
	in.WorldTime = "2026-09-29T11:59:59Z"
	if RPVisitSourceEligible(in, source) {
		t.Fatal("seven-day boundary rounded early")
	}
	in.WorldTime = "2026-09-29T12:00:00Z"
	for name, change := range map[string]func(*RPVisitSource){
		"other-actor":          func(s *RPVisitSource) { s.ActorID = "bo" },
		"unknown-relationship": func(s *RPVisitSource) { s.RelationshipSourceEventID = "unknown" },
		"no-observation":       func(s *RPVisitSource) { s.ObservationID = "" },
		"current-place":        func(s *RPVisitSource) { s.PlaceID = "home" },
		"future-memory":        func(s *RPVisitSource) { s.RememberedWorldTime = "2026-10-01T12:00:00Z" },
		"invented-kind":        func(s *RPVisitSource) { s.Kind = "guaranteed_reunion" },
	} {
		t.Run(name, func(t *testing.T) {
			bad := source
			change(&bad)
			if RPVisitSourceEligible(in, bad) {
				t.Fatal("invalid visit accepted")
			}
		})
	}
	in.Life.Relationships[0].Tension = 1
	if RPVisitSourceEligible(in, source) {
		t.Fatal("conflict ignored")
	}
	small := source
	small.Kind = "familiar_public_place"
	small.FriendID = ""
	small.RelationshipSourceEventID = ""
	small.ObservationID = ""
	if !RPVisitSourceEligible(in, small) {
		t.Fatal("own remembered public place requires no invented friend")
	}
}
