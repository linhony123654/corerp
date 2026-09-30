package core

import "testing"

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

func TestRPProposalRejectsSourcedRelationSelfIntroductionWithoutHint(t *testing.T) {
	input := RPDecisionInput{NPCName: "贾母", InterlocutorEntityID: "baoyu", LegalActions: []string{"respond"},
		Relationships: []RPCharacterRelationship{{SubjectEntityID: "baoyu", Role: "grandmother", SourceEventID: "identity-event"}}}
	proposal := RPDecisionProposal{Action: "respond", Text: "我是贾母，你是谁？"}
	if reason, err := ValidateRPDecisionProposalEvidence(input, proposal); reason != "known_relationship_introduction" || !HasCode(err, CodeInvalidArgument) {
		t.Fatalf("unflagged self introduction passed known relationship: %q %v", reason, err)
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
