package core

import "testing"

func TestCareerOvertimeChoiceKnownCoworkerBoundaries(t *testing.T) {
	input := RPDecisionInput{NPCEntityID: "self", Life: &RPLifeContext{Relationships: []RPRelationship{{SubjectEntityID: "peer", Trust: 2, Affinity: 2, SourceEventIDs: []string{"gift"}}}}}
	for _, peer := range []CareerKnownCoworker{{EntityID: "peer"}, {EntityID: "stranger", SourceEventID: "heard"}, {EntityID: "self", SourceEventID: "heard"}} {
		if got := ChooseCareerOvertime(input, "boss", peer); got.Decision != "decline" {
			t.Fatalf("unsupported coworker: %+v", got)
		}
	}
	peer := CareerKnownCoworker{EntityID: "peer", SourceEventID: "heard"}
	if got := ChooseCareerOvertime(input, "boss", peer); got.Decision != "accept" || got.CounterpartyEntityID != "peer" || len(got.SourceEventIDs) != 2 {
		t.Fatal(got)
	}
	input.Life.Relationships[0].Trust = -1
	input.Life.Relationships = append(input.Life.Relationships, RPRelationship{SubjectEntityID: "boss", Trust: 2, Affinity: 2, SourceEventIDs: []string{"boss_gift"}})
	if got := ChooseCareerOvertime(input, "boss", peer); got.Decision != "decline" || got.ReasonCode != "avoid_strained_workplace_relationship" {
		t.Fatal(got)
	}
	input.Life.Relationships[0].SourceEventIDs = nil
	if got := ChooseCareerOvertime(input, "boss", peer); got.ReasonCode != "cooperate_with_trusted_manager" {
		t.Fatal(got)
	}
}

func TestCareerOvertimeChoiceUsesOnlySourcedManagerRelationship(t *testing.T) {
	input := RPDecisionInput{Life: &RPLifeContext{Disposition: RPDisposition{Patience: 1, SourceEventID: "identity"}}}
	if got := ChooseCareerOvertime(input, "boss"); got.Decision != "decline" {
		t.Fatal(got)
	}
	input.Life.Relationships = []RPRelationship{{SubjectEntityID: "someone_else", Trust: 2, Affinity: 2, SourceEventIDs: []string{"unrelated"}}}
	if got := ChooseCareerOvertime(input, "boss"); got.Decision != "decline" {
		t.Fatal("unrelated relationship controlled choice")
	}
	input.Life.Relationships[0].SubjectEntityID = "boss"
	if got := ChooseCareerOvertime(input, "boss"); got.Decision != "accept" || got.SourceEventIDs[0] != "unrelated" {
		t.Fatal(got)
	}
	input.Life.Relationships[0].SourceEventIDs = nil
	if got := ChooseCareerOvertime(input, "boss"); got.Decision != "decline" {
		t.Fatal("unsourced relationship controlled choice")
	}
	input.Life.Needs = []RPNeed{{Code: "cash_security", Urgency: "high", SourceEventIDs: []string{"own_money"}}}
	if got := ChooseCareerOvertime(input, "boss"); got.Decision != "accept" || got.ReasonCode != "seek_optional_earned_income" {
		t.Fatal(got)
	}
	input.Life.Relationships[0].SourceEventIDs = []string{"experienced_conflict"}
	input.Life.Relationships[0].Tension = 6
	if got := ChooseCareerOvertime(input, "boss"); got.Decision != "decline" || got.ReasonCode != "preserve_boundaries_with_manager" {
		t.Fatal(got)
	}
}
