package core

import (
	"fmt"
	"testing"
)

func TestRPContactOpportunityKeepsIndependentMotivations(t *testing.T) {
	input := RPDecisionInput{Trigger: &RPDecisionTrigger{Kind: "elapsed_time", SourceEventID: "wait"}, ContactOpportunity: &RPContactOpportunityContext{SourceEventID: "known-gift"}, InterlocutorEntityID: "friend", Life: &RPLifeContext{}}
	if !RPContactOpportunitySuppressed(input) {
		t.Fatal("miss did not suppress optional contact")
	}
	input.Life.Goals = []RPGoal{{Code: "stabilize_income", SourceEventIDs: []string{"own-contract"}}}
	if RPContactOpportunitySuppressed(input) {
		t.Fatal("contact lottery suppressed economic need")
	}
	input.Life.Goals[0].SourceEventIDs = nil
	if !RPContactOpportunitySuppressed(input) {
		t.Fatal("unsourced goal bypassed opportunity")
	}
	input.ContactOpportunity.Selected = true
	if RPContactOpportunitySuppressed(input) {
		t.Fatal("selected opportunity suppressed")
	}
	input.ContactOpportunity = nil
	if RPContactOpportunitySuppressed(input) {
		t.Fatal("legacy context changed")
	}
}

func TestRPOpportunityPressureNeverDirectsGuaranteedDrama(t *testing.T) {
	for _, tt := range []struct {
		name     string
		base     int
		rare     bool
		pressure RPOpportunityPressure
		want     int
	}{
		{"ordinary", 1000, false, RPOpportunityPressure{}, 1000},
		{"quiet-small", 1000, false, RPOpportunityPressure{Quiet: true}, 1250},
		{"busy", 1000, false, RPOpportunityPressure{Busy: true, Quiet: true, Seeking: true}, 0},
		{"cooldown", 1000, false, RPOpportunityPressure{CoolingDown: true}, 0},
		{"recent-major", 1000, false, RPOpportunityPressure{RecentMajor: true, Quiet: true}, 250},
		{"recent-density", 1000, false, RPOpportunityPressure{RecentChanges: 3}, 250},
		{"seek-related", 1000, false, RPOpportunityPressure{Seeking: true}, 1500},
		{"rare-no-floor", 0, true, RPOpportunityPressure{Quiet: true, Seeking: true}, 0},
		{"rare-no-quiet-boost", 100, true, RPOpportunityPressure{Quiet: true, Seeking: true}, 100},
		{"minor-cap", 5000, false, RPOpportunityPressure{Quiet: true, Seeking: true}, 7500},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := RPOpportunityProbability(tt.base, tt.rare, tt.pressure)
			if err != nil || got != tt.want {
				t.Fatalf("got %d %v, want %d", got, err, tt.want)
			}
		})
	}
	if _, err := RPOpportunityProbability(101, true, RPOpportunityPressure{}); err == nil {
		t.Fatal("unbounded rare probability accepted")
	}
	if _, err := RPOpportunityProbability(1, false, RPOpportunityPressure{RecentChanges: -1}); err == nil {
		t.Fatal("invalid history accepted")
	}
}

func TestRPOpportunityDrawStableWindowAndNoEvent(t *testing.T) {
	k := RPOpportunityDrawKey{PolicyEventID: "policy", StreamSeed: "run-stream", InstanceID: "world", BranchID: "branch", ActorID: "actor", Kind: "friend_contact", SourceEventID: "observed-contact", WorldTime: "2026-09-24T08:01:00Z"}
	first, err := DrawRPOpportunity(k, 2500)
	if err != nil {
		t.Fatal(err)
	}
	k.WorldTime = "2026-09-24T16:59:59+08:00"
	retry, err := DrawRPOpportunity(k, 2500)
	if err != nil || retry != first {
		t.Fatalf("same world window rerolled: %+v %+v %v", first, retry, err)
	}
	zero, err := DrawRPOpportunity(k, 0)
	if err != nil || zero.Selected || zero.IdentityHash != first.IdentityHash || zero.RollBasisPoints != first.RollBasisPoints {
		t.Fatal("probability changed draw identity or zero selected")
	}
	k.WorldTime = "2026-09-24T09:00:00Z"
	next, err := DrawRPOpportunity(k, 2500)
	if err != nil || next.IdentityHash == first.IdentityHash {
		t.Fatal("next window failed to separate draw")
	}
	selected := 0
	for i := 0; i < 128; i++ {
		k.StreamSeed = fmt.Sprintf("stream-%d", i)
		draw, err := DrawRPOpportunity(k, 2500)
		if err != nil {
			t.Fatal(err)
		}
		if draw.Selected {
			selected++
		}
	}
	if selected == 0 || selected == 128 {
		t.Fatalf("fixed fixture streams lack hit/miss outcomes: %d", selected)
	}
	k.SourceEventID = ""
	if _, err := DrawRPOpportunity(k, 2500); err == nil {
		t.Fatal("unsourced draw accepted")
	}
}
