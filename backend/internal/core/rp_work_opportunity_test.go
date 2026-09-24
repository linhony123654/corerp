package core

import (
	"context"
	"testing"
)

func TestRPWorkOpportunityRequiresSelectedOwnKnownChange(t *testing.T) {
	makeInput := func() RPDecisionInput {
		memory := RPLifeMemory{Kind: "own_raise_announced", SubjectEntityID: "ada", SourceEventID: "notice", WorldTime: "2026-09-23T12:00:00Z"}
		return RPDecisionInput{NPCEntityID: "ada", Trigger: &RPDecisionTrigger{Kind: "elapsed_time", SourceEventID: "wait"}, ContactOpportunity: &RPContactOpportunityContext{SourceEventID: "friend", Selected: false}, WorkOpportunity: &RPWorkOpportunityContext{Memory: memory, Selected: true}, Life: &RPLifeContext{Disposition: RPDisposition{Sociability: 1}, SalientMemories: []RPLifeMemory{memory}}}
	}
	in := makeInput()
	out, err := (DeterministicRPDecisionProvider{}).Propose(context.Background(), in)
	if err != nil || out.Action != "respond" || out.Text != "工作上有些变化，我还得安排一下。" {
		t.Fatalf("work reaction: %+v %v", out, err)
	}
	for name, mutate := range map[string]func(*RPDecisionInput){
		"miss":         func(i *RPDecisionInput) { i.WorkOpportunity.Selected = false },
		"unknown":      func(i *RPDecisionInput) { i.Life.SalientMemories = nil },
		"other-worker": func(i *RPDecisionInput) { i.NPCEntityID = "bo" },
		"internal-evaluation": func(i *RPDecisionInput) {
			i.WorkOpportunity.Memory.Kind = "manager_assessment"
			i.Life.SalientMemories[0] = i.WorkOpportunity.Memory
		},
		"ordinary-attendance": func(i *RPDecisionInput) {
			i.WorkOpportunity.Memory.Kind = "own_work_attendance"
			i.Life.SalientMemories[0] = i.WorkOpportunity.Memory
		},
		"working":      func(i *RPDecisionInput) { i.ActivityCode = "work" },
		"quiet-person": func(i *RPDecisionInput) { i.Life.Disposition.Sociability = 0 },
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
	in.Life.Goals = []RPGoal{{Code: "collect_money_owed", SourceEventIDs: []string{"unpaid-wage"}}}
	out, err = (DeterministicRPDecisionProvider{}).Propose(context.Background(), in)
	if err != nil || out.Text == "工作上有些变化，我还得安排一下。" {
		t.Fatalf("work opportunity displaced urgent need: %+v %v", out, err)
	}
}
