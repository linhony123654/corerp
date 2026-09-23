package core

import (
	"context"
	"testing"
)

func TestRPInitiativePolicyUsesSourcedLifeAndAllowsQuiet(t *testing.T) {
	input := RPDecisionInput{NPCEntityID: "npc", InterlocutorEntityID: "player", Trigger: &RPDecisionTrigger{Kind: "elapsed_time", SourceEventID: "wait"}, Life: &RPLifeContext{}}
	provider := DeterministicRPDecisionProvider{}
	quiet, err := provider.Propose(context.Background(), input)
	if err != nil || quiet.Action != "silence" {
		t.Fatalf("ordinary time forces activity: %+v %v", quiet, err)
	}
	input.Life.Goals = []RPGoal{{Code: "stabilize_income"}}
	quiet, err = provider.Propose(context.Background(), input)
	if err != nil || quiet.Action != "silence" {
		t.Fatal("unsourced need caused initiative")
	}
	input.Life.Goals[0].SourceEventIDs = []string{"actual-expense"}
	pressure, err := provider.Propose(context.Background(), input)
	if err != nil || pressure.Action != "respond" || pressure.Text == "" {
		t.Fatalf("real need has no initiative: %+v %v", pressure, err)
	}
	input.Life.Goals = []RPGoal{{Code: "avoid_conflict", SubjectEntityID: "player", SourceEventIDs: []string{"observed-insult"}}}
	input.ReachablePlaceIDs = []string{"home"}
	departure, err := provider.Propose(context.Background(), input)
	if err != nil || departure.Action != "leave" || departure.DestinationPlaceID != "home" {
		t.Fatalf("conflict did not motivate departure: %+v %v", departure, err)
	}
	input.Life.Goals = nil
	input.Life.Disposition.Sociability = 1
	input.Life.Relationships = []RPRelationship{{SubjectEntityID: "player", Trust: 2, SourceEventIDs: []string{"promise-kept"}}}
	contact, err := provider.Propose(context.Background(), input)
	if err != nil || contact.Action != "respond" {
		t.Fatalf("known relationship did not motivate contact: %+v %v", contact, err)
	}
	input.ActivityCode = "work"
	quiet, err = provider.Propose(context.Background(), input)
	if err != nil || quiet.Action != "silence" {
		t.Fatal("social initiative interrupted work")
	}
	input.PlayerSpeechText = "invented speech"
	if _, err := provider.Propose(context.Background(), input); !HasCode(err, CodeInvalidArgument) {
		t.Fatal("initiative pretended player speech occurred")
	}
}
