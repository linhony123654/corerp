package core

import "strings"

// ValidateRPDecisionProposal is shared by provider adapters and authoritative
// commit validation. The latter always rechecks the current world state.
func ValidateRPDecisionProposal(input RPDecisionInput, proposal RPDecisionProposal) error {
	_, err := ValidateRPDecisionProposalEvidence(input, proposal)
	return err
}

// Stable, bounded validation evidence for diagnostics. The reason is produced
// at validation time, not reconstructed later from changed world state or text.
func ValidateRPDecisionProposalEvidence(input RPDecisionInput, proposal RPDecisionProposal) (string, error) {
	allowed := false
	for _, action := range input.LegalActions {
		if action == proposal.Action {
			allowed = true
			break
		}
	}
	if !allowed {
		return "action_not_legal", NewError(CodeInvalidArgument, "NPC proposal action is not legal in this scene")
	}
	switch proposal.Action {
	case "respond", "refuse":
		if strings.TrimSpace(proposal.Text) == "" || len([]rune(proposal.Text)) > 2000 || proposal.DestinationPlaceID != "" {
			return "invalid_speech_fields", NewError(CodeInvalidArgument, "NPC speech proposal requires only valid text")
		}
	case "leave":
		if proposal.Text != "" {
			return "movement_contains_speech", NewError(CodeInvalidArgument, "NPC leave proposal cannot include speech")
		}
		for _, placeID := range input.ReachablePlaceIDs {
			if placeID == proposal.DestinationPlaceID {
				return "", nil
			}
		}
		return "destination_not_reachable", NewError(CodeInvalidArgument, "NPC leave destination is not reachable")
	case "silence", "wait":
		if proposal.Text != "" || proposal.DestinationPlaceID != "" {
			return "noop_contains_effects", NewError(CodeInvalidArgument, "NPC no-op proposal cannot contain effects")
		}
	default:
		return "unknown_action", NewError(CodeInvalidArgument, "unknown NPC proposal action")
	}
	return "", nil
}
