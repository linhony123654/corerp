package core

import (
	"context"
	"testing"
)

func TestDeterministicRPDecisionProviderUsesEconomyScheduleAndStableInput(t *testing.T) {
	provider := DeterministicRPDecisionProvider{}
	input := RPDecisionInput{PlayerSpeechText: "你好。", OwnAssetMinor: 500}
	first, err := provider.Propose(context.Background(), input)
	second, errAgain := provider.Propose(context.Background(), input)
	if err != nil || errAgain != nil || first != second || first.Action != "respond" {
		t.Fatalf("same input did not yield stable response: %+v, %+v, %v, %v", first, second, err, errAgain)
	}
	input.OwnAssetMinor = 50
	if poor, err := provider.Propose(context.Background(), input); err != nil || poor.Action != "refuse" {
		t.Fatalf("own economic status did not affect policy: %+v, %v", poor, err)
	}
	input.OwnAssetMinor = 500
	input.NextSchedule = &RPDecisionSchedule{ActivityCode: "work"}
	if busy, err := provider.Propose(context.Background(), input); err != nil || busy.Action != "refuse" {
		t.Fatalf("own schedule did not affect policy: %+v, %v", busy, err)
	}
}
