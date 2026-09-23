package core

import (
	"context"
	"reflect"
	"testing"
)

func TestRPLifeEconomicPressureCreatesSourcedGoalAndChangesChoice(t *testing.T) {
	input := RPDecisionInput{NPCEntityID: "person", InterlocutorEntityID: "player", PlayerSpeechText: "你好", OwnAssetMinor: 500, GoalCode: "keep_daily_routine", LegalActions: []string{"respond", "refuse", "silence", "wait"}}
	life := &RPLifeContext{Disposition: DeriveRPDisposition("person", "materialization-event"), RoutineSourceEventID: "routine-event", LiabilityMinor: 200, ReceivableMinor: 100, EconomicSourceEventIDs: []string{"economic-event"}}
	input.Life = life
	DeriveRPLifeGoals(input, life)
	before, err := (DeterministicRPDecisionProvider{}).Propose(context.Background(), input)
	if err != nil || before.Action != "respond" {
		t.Fatalf("unexpected comfortable state %+v %v", before, err)
	}
	input.OwnAssetMinor = 50
	DeriveRPLifeGoals(input, life)
	if life.Needs[0].Code != "cash_security" || life.Goals[0].Code != "collect_money_owed" || !reflect.DeepEqual(life.Goals[0].SourceEventIDs, []string{"economic-event"}) || len(life.Goals[0].ConflictsWith) == 0 {
		t.Fatalf("no causal pressure chain %+v", life)
	}
	after, err := (DeterministicRPDecisionProvider{}).Propose(context.Background(), input)
	if err != nil || after.Action != "refuse" || after.Text == before.Text {
		t.Fatalf("life did not alter choice %+v %v", after, err)
	}
	// An expected receipt is not money that can be spent today.
	life.ReceivableMinor = 1000000
	DeriveRPLifeGoals(input, life)
	if life.Needs[0].Code != "cash_security" {
		t.Fatal("receivable incorrectly erased cash pressure")
	}
}

func TestRPLifeDispositionStableAndScheduleGoalSourced(t *testing.T) {
	seed := DeriveRPDisposition("person", "origin")
	if !reflect.DeepEqual(seed, DeriveRPDisposition("person", "origin")) || seed.Version != "corerp.disposition.v1" || len(seed.Values) != 2 || seed.Values[0] == seed.Values[1] {
		t.Fatalf("invalid stable temperament %+v", seed)
	}
	input := RPDecisionInput{OwnAssetMinor: 1000, GoalCode: "routine", NextSchedule: &RPDecisionSchedule{ActivityCode: "work", SourceEventID: "schedule-event"}}
	life := &RPLifeContext{Disposition: seed, RoutineSourceEventID: "routine-event"}
	DeriveRPLifeGoals(input, life)
	if life.Goals[0].Code != "keep_work_schedule" || life.Goals[0].SourceEventIDs[0] != "schedule-event" {
		t.Fatalf("schedule chain %+v", life)
	}
}
