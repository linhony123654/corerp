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
