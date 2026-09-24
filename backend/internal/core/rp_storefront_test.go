package core

import (
	"context"
	"testing"
)

func TestRPStoreShortageDecisionRequiresSelectedCurrentOwnObservation(t *testing.T) {
	shelf := RPStoreAvailability{PlaceID: "store", StoreActorID: "shop", SKUID: "food", StorefrontSourceEventID: "declaration", StockSourceEventID: "last-sale"}
	makeInput := func() RPDecisionInput {
		return RPDecisionInput{PlaceID: "store", Trigger: &RPDecisionTrigger{Kind: "elapsed_time", SourceEventID: "wait"}, Stores: []RPStoreAvailability{shelf}, StoreOpportunities: []RPStoreOpportunityContext{{Store: shelf, Selected: true}}, ContactOpportunity: &RPContactOpportunityContext{SourceEventID: "friend", Selected: false}, Life: &RPLifeContext{Disposition: RPDisposition{Sociability: 1, SourceEventID: "origin"}}}
	}
	input := makeInput()
	out, err := (DeterministicRPDecisionProvider{}).Propose(context.Background(), input)
	if err != nil || out.Action != "respond" || out.Text != "这家店有东西缺货了。" {
		t.Fatalf("shortage reaction: %+v %v", out, err)
	}
	for name, mutate := range map[string]func(*RPDecisionInput){
		"miss":         func(i *RPDecisionInput) { i.StoreOpportunities[0].Selected = false },
		"replenished":  func(i *RPDecisionInput) { i.Stores[0].Available = true },
		"distant":      func(i *RPDecisionInput) { i.PlaceID = "home" },
		"unknown":      func(i *RPDecisionInput) { i.Stores = nil },
		"unsourced":    func(i *RPDecisionInput) { i.StoreOpportunities[0].Store.StockSourceEventID = "" },
		"work":         func(i *RPDecisionInput) { i.ActivityCode = "work" },
		"quiet-person": func(i *RPDecisionInput) { i.Life.Disposition.Sociability = 0 },
	} {
		t.Run(name, func(t *testing.T) {
			in := makeInput()
			mutate(&in)
			out, err := (DeterministicRPDecisionProvider{}).Propose(context.Background(), in)
			if err != nil || out.Action != "silence" {
				t.Fatalf("invalid shortage reaction: %+v %v", out, err)
			}
		})
	}
	input.Life.Goals = []RPGoal{{Code: "stabilize_income", SourceEventIDs: []string{"bill"}}}
	out, err = (DeterministicRPDecisionProvider{}).Propose(context.Background(), input)
	if err != nil || out.Text == "这家店有东西缺货了。" {
		t.Fatal("optional shortage displaced urgent need")
	}
}
