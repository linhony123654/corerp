package core

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func rpNonverbalDecisionFixture() RPDecisionInput {
	in := rpSelectionFixture()
	in.TurnID = "event-player-nod"
	in.SpeechEventID = ""
	in.PlayerSpeechText = ""
	in.PlayerSpeechWorldTime = ""
	in.Trigger = &RPDecisionTrigger{Kind: "nonverbal", SourceEventID: in.TurnID}
	in.ObservedPlayerAction = &RPDecisionObservedAction{SourceEventID: in.TurnID, ActorEntityID: in.InterlocutorEntityID, TargetEntityID: in.NPCEntityID, Action: "nod", PlaceID: in.PlaceID, WorldTime: in.WorldTime}
	return in
}

func TestRPNonverbalTriggerDeterministicUsesOnlyLegalOwnReaction(t *testing.T) {
	in := rpNonverbalDecisionFixture()
	in.OwnAssetMinor = 0
	proposal, err := (DeterministicRPDecisionProvider{}).Propose(context.Background(), in)
	if err != nil || proposal.Action != "silence" || proposal.ExpressionCode != "nod" || proposal.Text != "" || ValidateRPDecisionProposal(in, proposal) != nil {
		t.Fatal("nod became speech/refusal/agreement or initiative", proposal, err)
	}
	for _, kind := range []string{"undirected", "other_target", "unknown_action", "gesture_nod"} {
		candidate := in
		observed := *in.ObservedPlayerAction
		candidate.ObservedPlayerAction = &observed
		expectedExpression := ""
		switch kind {
		case "undirected":
			observed.TargetEntityID = ""
		case "other_target":
			observed.TargetEntityID = "other"
		case "unknown_action":
			observed.Action = "smile"
		case "gesture_nod":
			observed.Action = "gesture"
			observed.GestureCode = "nod"
		}
		got, err := (DeterministicRPDecisionProvider{}).Propose(context.Background(), candidate)
		if err != nil || got.Action != "silence" || got.ExpressionCode != expectedExpression || got.Text != "" || ValidateRPDecisionProposal(candidate, got) != nil {
			t.Fatal("reaction added a meaning/target", kind, got, err)
		}
	}
	in.LegalActions = []string{"wait"}
	got, err := (DeterministicRPDecisionProvider{}).Propose(context.Background(), in)
	if err != nil || got.Action != "wait" || got.ExpressionCode != "" || got.Text != "" {
		t.Fatal("illegal silent response", got, err)
	}
	in.LegalActions = []string{"respond"}
	if _, err := (DeterministicRPDecisionProvider{}).Propose(context.Background(), in); err == nil {
		t.Fatal("invented a speech fallback")
	}
}

func TestRPNonverbalTriggerDoesNotPretendSpeechOrElapsedTime(t *testing.T) {
	for _, kind := range []string{"missing_action", "wrong_source", "wrong_turn", "speech_id", "words", "speech_time"} {
		in := rpNonverbalDecisionFixture()
		switch kind {
		case "missing_action":
			in.ObservedPlayerAction = nil
		case "wrong_source":
			in.ObservedPlayerAction.SourceEventID = "different"
		case "wrong_turn":
			in.TurnID = "different"
		case "speech_id":
			in.SpeechEventID = "fake"
		case "words":
			in.PlayerSpeechText = "(点头)"
		case "speech_time":
			in.PlayerSpeechWorldTime = in.WorldTime
		}
		if _, err := (DeterministicRPDecisionProvider{}).Propose(context.Background(), in); err == nil {
			t.Fatal("contradictory action trigger accepted", kind)
		}
	}
	in := rpNonverbalDecisionFixture()
	in.Trigger = &RPDecisionTrigger{Kind: "elapsed_time", SourceEventID: "elapsed"}
	in.TurnID = ""
	in.ObservedPlayerAction = nil
	got, err := (DeterministicRPDecisionProvider{}).Propose(context.Background(), in)
	if err != nil || got.Action != "silence" {
		t.Fatal("elapsed initiative changed", got, err)
	}
	in = rpSelectionFixture()
	in.Trigger = nil
	in.OwnAssetMinor = 200
	in.PlayerSpeechText = "你好"
	got, err = (DeterministicRPDecisionProvider{}).Propose(context.Background(), in)
	if err != nil || got.Action != "respond" || got.Text != "你好。" {
		t.Fatal("speech fallback changed", got, err)
	}
}

func TestRPNonverbalTriggerIsRequiredInSelectionAndTypedEvidence(t *testing.T) {
	in := rpNonverbalDecisionFixture()
	minimal, err := SelectRPDecisionContext(in, DefaultRPDecisionContextBudgetBytes)
	if err != nil {
		t.Fatal(err)
	}
	budget := minimal.ContextSelection.EncodedBytes + 64
	in.RecentDialogue = []RPDecisionDialogue{{EventID: "crowded", SpeakerEntityID: "other", Text: strings.Repeat("irrelevant", 2000)}}
	selected, err := SelectRPDecisionContext(in, budget)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(selected.ObservedPlayerAction, in.ObservedPlayerAction) || !reflect.DeepEqual(selected.Trigger, in.Trigger) || selected.PlayerSpeechText != "" || selected.SpeechEventID != "" || selected.PlayerSpeechWorldTime != "" {
		t.Fatal("required action dropped or made into speech")
	}
	raw, _ := json.Marshal(selected)
	if len(raw) > budget || selected.ContextSelection.EncodedBytes != len(raw) {
		t.Fatal("new required action not counted")
	}
	if _, err := SelectRPDecisionContext(in, 100); err == nil {
		t.Fatal("required action overflow silently omitted")
	}
	before, _ := HashJSON(selected)
	again, err := SelectRPDecisionContext(selected, budget)
	after, _ := HashJSON(again)
	if err != nil || before != after {
		t.Fatal("nonverbal selection unstable", err)
	}
	ranges := RPDecisionEvidenceSupport(selected)[in.TurnID]
	typed := 0
	for _, r := range ranges {
		if r.Kind == "accepted_speech" {
			t.Fatal("action source classified as words")
		}
		if r.Kind == "observed_player_action" {
			typed++
			if r.Locator != "/observed_player_action" || r.ActorEntityID != in.InterlocutorEntityID || r.TargetEntityID != in.NPCEntityID || r.WorldTime != in.WorldTime || r.Status != "witnessed_at_time" || !reflect.DeepEqual(r.AllowedUses, []string{"observed_nonverbal_action_at_time"}) {
				t.Fatal("action range invented semantics", r)
			}
		}
	}
	if typed != 1 || !RPDecisionEvidenceEventIDs(selected)[in.TurnID] {
		t.Fatal("typed action/evidence anchor missing", ranges)
	}
	if text, complete := ResolveRPDecisionSpeech(selected, in.TurnID, in.InterlocutorEntityID); text != "" || complete {
		t.Fatal("action acquired an utterance")
	}
	proposal := RPDecisionProposal{Action: "silence", Private: &RPDecisionPrivate{BasisEventIDs: []string{in.TurnID}}}
	if ValidateRPDecisionProposal(selected, proposal) != nil {
		t.Fatal("actual action evidence rejected")
	}
	proposal.Private.BasisEventIDs = []string{"unwitnessed"}
	if ValidateRPDecisionProposal(selected, proposal) == nil {
		t.Fatal("unwitnessed action admitted")
	}
}
