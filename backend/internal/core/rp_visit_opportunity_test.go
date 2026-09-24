package core

import (
	"context"
	"testing"
)

func TestRPVisitOpportunityOptionalSourcedReachableAndSubordinate(t *testing.T) {
	makeInput := func() RPDecisionInput {
		return RPDecisionInput{NPCEntityID: "nora", PlaceID: "home", WorldTime: "2026-09-23T03:15:00Z", Trigger: &RPDecisionTrigger{Kind: "elapsed_time", SourceEventID: "wait"}, ReachablePlaceIDs: []string{"cafe"}, Life: &RPLifeContext{Disposition: RPDisposition{Sociability: 1}}, ContactOpportunity: &RPContactOpportunityContext{SourceEventID: "contact", Selected: false}, VisitOpportunity: &RPVisitOpportunityContext{Selected: true, Source: RPVisitSource{ActorID: "nora", Kind: "familiar_public_place", PlaceID: "cafe", PlaceSourceEventID: "place", MemorySourceEventID: "visit", RememberedWorldTime: "2026-09-22T02:03:00Z"}}}
	}
	in := makeInput()
	out, err := (DeterministicRPDecisionProvider{}).Propose(context.Background(), in)
	if err != nil || out.Action != "leave" || out.DestinationPlaceID != "cafe" || out.Text != "" {
		t.Fatalf("visit: %+v %v", out, err)
	}
	for name, change := range map[string]func(*RPDecisionInput){
		"miss":         func(i *RPDecisionInput) { i.VisitOpportunity.Selected = false },
		"unknown":      func(i *RPDecisionInput) { i.VisitOpportunity.Source.MemorySourceEventID = "" },
		"other-actor":  func(i *RPDecisionInput) { i.VisitOpportunity.Source.ActorID = "bo" },
		"no-route":     func(i *RPDecisionInput) { i.ReachablePlaceIDs = nil },
		"quiet-person": func(i *RPDecisionInput) { i.Life.Disposition.Sociability = 0 },
		"no-life":      func(i *RPDecisionInput) { i.Life = nil },
		"working":      func(i *RPDecisionInput) { i.ActivityCode = "work" },
		"imminent-work": func(i *RPDecisionInput) {
			i.NextSchedule = &RPDecisionSchedule{ActivityCode: "work", WorldTime: "2026-09-23T04:15:00Z"}
		},
		"law": func(i *RPDecisionInput) { i.Law = &RPLawContext{LawfulActions: []string{"silence"}} },
	} {
		t.Run(name, func(t *testing.T) {
			in := makeInput()
			change(&in)
			out, err := (DeterministicRPDecisionProvider{}).Propose(context.Background(), in)
			if err != nil || out.Action != "silence" {
				t.Fatalf("ineligible visit: %+v %v", out, err)
			}
		})
	}
	in = makeInput()
	in.Life.Goals = []RPGoal{{Code: "collect_money_owed", SourceEventIDs: []string{"wage"}}}
	out, err = (DeterministicRPDecisionProvider{}).Propose(context.Background(), in)
	if err != nil || out.Action != "respond" {
		t.Fatalf("urgent need displaced: %+v %v", out, err)
	}
	in = makeInput()
	in.NextSchedule = &RPDecisionSchedule{ActivityCode: "work", WorldTime: "2026-09-23T04:15:01Z"}
	if RPSelectedVisitDestination(in) != "cafe" {
		t.Fatal("work outside guard suppressed visit")
	}
}
