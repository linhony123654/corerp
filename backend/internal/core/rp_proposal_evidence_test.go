package core

import (
	"reflect"
	"testing"
)

func TestRPProposalAllowsKnownCharacterIdentityDialogue(t *testing.T) {
	for _, tc := range []struct {
		name, request, text string
		introduceSelf       bool
	}{
		{"explicit_name_request", "您叫什么？请告诉我姓名。", "我是贾母。", true},
		{"rehearsal", "我们排练一遍自我介绍。", "我是贾母，这一句这样说可好？", false},
		{"reported_words", "你刚才是怎么向客人报姓名的？", "我是贾母——方才我这样告诉那位客人。", false},
		{"repeated_name", "您再说一遍自己的姓名吧。", "我是贾母。", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := RPDecisionInput{NPCName: "贾母", InterlocutorEntityID: "baoyu", PlayerSpeechText: tc.request, LegalActions: []string{"respond"},
				Relationships: []RPCharacterRelationship{{SubjectEntityID: "baoyu", Role: "grandmother", SourceEventID: "identity-event"}}}
			proposal := RPDecisionProposal{Action: "respond", Text: tc.text, IntroduceSelf: tc.introduceSelf, ExpressionCode: "nod"}
			before := proposal
			if reason, err := ValidateRPDecisionProposalEvidence(input, proposal); reason != "" || err != nil {
				t.Fatalf("valid identity dialogue rejected: %q %v", reason, err)
			}
			if err := ValidateRPDecisionProposal(input, proposal); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(proposal, before) {
				t.Fatal("validation changed the proposed observable")
			}
		})
	}
}

func TestRPProposalEvidencePreservesValidation(t *testing.T) {
	input := RPDecisionInput{LegalActions: []string{"respond", "refuse", "leave", "silence", "wait", "unknown"}, ReachablePlaceIDs: []string{"home"}}
	for _, tc := range []struct {
		proposal RPDecisionProposal
		reason   string
	}{
		{RPDecisionProposal{Action: "steal"}, "action_not_legal"},
		{RPDecisionProposal{Action: "respond"}, "invalid_speech_fields"},
		{RPDecisionProposal{Action: "leave", Text: "secret"}, "movement_contains_speech"},
		{RPDecisionProposal{Action: "leave", DestinationPlaceID: "invented"}, "destination_not_reachable"},
		{RPDecisionProposal{Action: "silence", Text: "effect"}, "noop_contains_effects"},
		{RPDecisionProposal{Action: "unknown"}, "unknown_action"},
		{RPDecisionProposal{Action: "leave", DestinationPlaceID: "home"}, ""},
		{RPDecisionProposal{Action: "respond", Text: "hello"}, ""},
		{RPDecisionProposal{Action: "wait"}, ""},
	} {
		reason, err := ValidateRPDecisionProposalEvidence(input, tc.proposal)
		ordinary := ValidateRPDecisionProposal(input, tc.proposal)
		if reason != tc.reason || (err == nil) != (ordinary == nil) || (tc.reason != "" && !HasCode(err, CodeInvalidArgument)) {
			t.Fatalf("reason=%s err=%v ordinary=%v", reason, err, ordinary)
		}
		if err != nil && err.Error() != ordinary.Error() {
			t.Fatal("evidence changed public validation error")
		}
	}
}

func TestRPProposalAcceptsSourcedRelationSelfIntroductionWithoutHint(t *testing.T) {
	input := RPDecisionInput{NPCName: "贾母", InterlocutorEntityID: "baoyu", LegalActions: []string{"respond"},
		Relationships: []RPCharacterRelationship{{SubjectEntityID: "baoyu", Role: "grandmother", SourceEventID: "identity-event"}}}
	proposal := RPDecisionProposal{Action: "respond", Text: "我是贾母，你是谁？"}
	if reason, err := ValidateRPDecisionProposalEvidence(input, proposal); reason != "" || err != nil {
		t.Fatalf("known relationship imposed a semantic speech prohibition: %q %v", reason, err)
	}
	input.Relationships = nil
	if reason, err := ValidateRPDecisionProposalEvidence(input, proposal); reason != "" || err != nil {
		t.Fatalf("absent canon fabricated a relationship: %q %v", reason, err)
	}
}

func TestRPProposalAcceptsAuthorizedNestedSourceReferences(t *testing.T) {
	input := RPDecisionInput{LegalActions: []string{"respond"}, Environment: &RPLocalEnvironment{SourceEventID: "weather-source"},
		Life: &RPLifeContext{EconomicSourceEventIDs: []string{"account-source"}}}
	for _, source := range []string{"weather-source", "account-source"} {
		proposal := RPDecisionProposal{Action: "respond", Text: "好。", Private: &RPDecisionPrivate{BasisEventIDs: []string{source}}}
		if reason, err := ValidateRPDecisionProposalEvidence(input, proposal); err != nil || reason != "" {
			t.Fatalf("authorized nested source %q was rejected: %s %v", source, reason, err)
		}
	}
	proposal := RPDecisionProposal{Action: "respond", Text: "好。", Private: &RPDecisionPrivate{BasisEventIDs: []string{"model-invented"}}}
	if reason, err := ValidateRPDecisionProposalEvidence(input, proposal); reason != "ungrounded_decision" || !HasCode(err, CodeInvalidArgument) {
		t.Fatalf("unsourced basis passed: %s %v", reason, err)
	}
}

func TestRPProposalAllowsObservableWithoutSpeech(t *testing.T) {
	input := RPDecisionInput{LegalActions: []string{"silence", "wait", "leave"}, ReachablePlaceIDs: []string{"hall"}}
	for _, action := range []string{"silence", "wait"} {
		if reason, err := ValidateRPDecisionProposalEvidence(input, RPDecisionProposal{Action: action, ExpressionCode: "nod"}); err != nil || reason != "" {
			t.Fatalf("%s blocked sourced wordless expression: %q %v", action, reason, err)
		}
	}
	if reason, err := ValidateRPDecisionProposalEvidence(input, RPDecisionProposal{Action: "leave", DestinationPlaceID: "hall", ExpressionCode: "nod"}); reason != "expression_incompatible_action" || !HasCode(err, CodeInvalidArgument) {
		t.Fatalf("movement plus expression should require a separate proposal: %q %v", reason, err)
	}
}
