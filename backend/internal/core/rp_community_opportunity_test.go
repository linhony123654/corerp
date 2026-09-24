package core

import (
	"context"
	"testing"
)

func TestRPCommunityReactionRequiresSelectedKnownCurrentChange(t *testing.T) {
	makeInput := func() RPDecisionInput {
		law := RPKnownLaw{InstitutionID: "council", LawID: "quiet", ScopeKind: "region", ScopeID: "local", PlaceIDs: []string{"cafe"}, EnactmentEventID: "repeal", PreviousEnactmentEventID: "old", KnowledgeEventID: "first-hearing", EffectiveWorldTime: "2026-09-23T13:00:00Z", Repealed: true}
		in := RPDecisionInput{NPCEntityID: "ada", WorldTime: "2026-09-23T13:15:00Z", PlaceID: "cafe", Trigger: &RPDecisionTrigger{Kind: "elapsed_time", SourceEventID: "wait"}, ContactOpportunity: &RPContactOpportunityContext{SourceEventID: "friend", Selected: false}, Life: &RPLifeContext{Disposition: RPDisposition{Sociability: 1}}, CommunityOpportunity: &RPCommunityOpportunityContext{Law: law, Selected: true}}
		law.KnowledgeEventID = "repeat-hearing"
		in.Law = BuildRPLawContext([]RPKnownLaw{law}, in.WorldTime, in.PlaceID, []string{"respond", "silence"})
		return in
	}
	in := makeInput()
	out, err := (DeterministicRPDecisionProvider{}).Propose(context.Background(), in)
	if err != nil || out.Action != "respond" || out.Text != "这里的规矩有了变化，得重新留意一下。" {
		t.Fatalf("community reaction: %+v %v", out, err)
	}
	for name, mutate := range map[string]func(*RPDecisionInput){
		"miss":              func(i *RPDecisionInput) { i.CommunityOpportunity.Selected = false },
		"unknown":           func(i *RPDecisionInput) { i.Law = nil },
		"forged-text":       func(i *RPDecisionInput) { i.CommunityOpportunity.Law.Text = "Invented" },
		"distant":           func(i *RPDecisionInput) { i.PlaceID = "home" },
		"future":            func(i *RPDecisionInput) { i.WorldTime = "2026-09-23T12:00:00Z" },
		"busy":              func(i *RPDecisionInput) { i.ActivityCode = "work" },
		"quiet-person":      func(i *RPDecisionInput) { i.Life.Disposition.Sociability = 0 },
		"speech-prohibited": func(i *RPDecisionInput) { i.Law.LawfulActions = []string{"silence"} },
	} {
		t.Run(name, func(t *testing.T) {
			in := makeInput()
			mutate(&in)
			out, err := (DeterministicRPDecisionProvider{}).Propose(context.Background(), in)
			if err != nil || out.Action != "silence" {
				t.Fatalf("invalid reaction: %+v %v", out, err)
			}
		})
	}
	in.Life.Goals = []RPGoal{{Code: "collect_money_owed", SourceEventIDs: []string{"wage"}}}
	out, err = (DeterministicRPDecisionProvider{}).Propose(context.Background(), in)
	if err != nil || out.Text == "这里的规矩有了变化，得重新留意一下。" {
		t.Fatalf("displaced urgent goal: %+v %v", out, err)
	}
}
