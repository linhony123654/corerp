package core

import (
	"context"
	"testing"
)

func TestRPWeatherChoiceRespectsKnowledgePersonalityAndWork(t *testing.T) {
	makeInput := func() RPDecisionInput {
		return RPDecisionInput{NPCEntityID: "npc", PlaceID: "cafe", WorldTime: "2026-09-22T03:15:00Z", Trigger: &RPDecisionTrigger{Kind: "elapsed_time", SourceEventID: "wait"}, ContactOpportunity: &RPContactOpportunityContext{SourceEventID: "gift", Selected: false}, LegalActions: []string{"silence", "leave"}, ReachablePlaceIDs: []string{"home"}, Environment: &RPLocalEnvironment{PlaceID: "cafe", SourceEventID: "rain-event", Condition: "rain", EffectiveWorldTime: "2026-09-22T03:00:00Z", UntilWorldTime: "2026-09-22T04:00:00Z"}, Life: &RPLifeContext{Disposition: RPDisposition{Caution: 1, SourceEventID: "materialized"}, Background: &RPBackground{EntityID: "npc", ResidencePlaceID: "home", ResidenceSourceEventID: "background"}}}
	}
	input := makeInput()
	proposal, err := (DeterministicRPDecisionProvider{}).Propose(context.Background(), input)
	if err != nil || proposal.Action != "leave" || proposal.DestinationPlaceID != "home" {
		t.Fatalf("rain choice: %+v %v", proposal, err)
	}
	for name, mutate := range map[string]func(*RPDecisionInput){
		"clear":      func(i *RPDecisionInput) { i.Environment.Condition = "clear" },
		"unknown":    func(i *RPDecisionInput) { i.Environment.SourceEventID = "" },
		"distant":    func(i *RPDecisionInput) { i.Environment.PlaceID = "elsewhere" },
		"expired":    func(i *RPDecisionInput) { i.WorldTime = "2026-09-22T04:00:00Z" },
		"uncautious": func(i *RPDecisionInput) { i.Life.Disposition.Caution = 0 },
		"work":       func(i *RPDecisionInput) { i.ActivityCode = "work" },
		"upcoming-work": func(i *RPDecisionInput) {
			i.NextSchedule = &RPDecisionSchedule{ActivityCode: "work", WorldTime: "2026-09-22T04:00:00Z"}
		},
		"unreachable":    func(i *RPDecisionInput) { i.ReachablePlaceIDs = nil },
		"unexecutable":   func(i *RPDecisionInput) { i.LegalActions = []string{"silence"} },
		"unsourced-home": func(i *RPDecisionInput) { i.Life.Background.ResidenceSourceEventID = "" },
	} {
		t.Run(name, func(t *testing.T) {
			input := makeInput()
			mutate(&input)
			if home := RPWeatherHomeDestination(input); home != "" {
				t.Fatalf("invalid weather departure to%s", home)
			}
		})
	}
	input.Life.Goals = []RPGoal{{Code: "stabilize_income", SourceEventIDs: []string{"own-expense"}}}
	proposal, err = (DeterministicRPDecisionProvider{}).Propose(context.Background(), input)
	if err != nil || proposal.Action != "respond" {
		t.Fatal("weather displaced urgent economic motivation")
	}
}
