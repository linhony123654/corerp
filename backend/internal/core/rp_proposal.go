package core

import (
	"reflect"
	"strings"
	"unicode/utf8"
)

// ValidateRPDecisionProposal is shared by provider adapters and authoritative
// commit validation. The latter always rechecks the current world state.
func ValidateRPDecisionProposal(input RPDecisionInput, proposal RPDecisionProposal) error {
	_, err := ValidateRPDecisionProposalEvidence(input, proposal)
	return err
}

// Stable, bounded validation evidence for diagnostics. The reason is produced
// at validation time, not reconstructed later from changed world state or text.
func ValidateRPDecisionProposalEvidence(input RPDecisionInput, proposal RPDecisionProposal) (string, error) {
	if proposal.Private != nil {
		private := proposal.Private
		if !boundedRPPrivateText(private.Intent, 160) || !boundedRPPrivateText(private.Emotion, 80) || !boundedRPPrivateText(private.RelationshipStance, 80) || len(private.BasisEventIDs) > 8 {
			return "invalid_private_decision", NewError(CodeInvalidArgument, "NPC private decision metadata is invalid")
		}
		allowedRefs := RPDecisionEvidenceEventIDs(input)
		seen := map[string]bool{}
		for _, ref := range private.BasisEventIDs {
			if !allowedRefs[ref] || seen[ref] {
				return "ungrounded_decision", NewError(CodeInvalidArgument, "NPC decision basis is not in authorized context")
			}
			seen[ref] = true
		}
	}
	if proposal.ExpressionCode != "" {
		switch proposal.ExpressionCode {
		case "smile", "nod", "shake_head", "turn_away", "frown", "beckon":
		default:
			return "expression_not_supported", NewError(CodeInvalidArgument, "NPC expression is not a supported observable")
		}
		// A character may answer without words. The expression is still a
		// separate observable that the owner must commit with frozen witnesses.
		if proposal.Action != "respond" && proposal.Action != "refuse" && proposal.Action != "silence" && proposal.Action != "wait" {
			return "expression_incompatible_action", NewError(CodeInvalidArgument, "NPC expression is incompatible with the proposed action")
		}
		if proposal.ExpressionCode == "beckon" && input.InterlocutorEntityID == "" {
			return "expression_target_missing", NewError(CodeInvalidArgument, "NPC beckon requires a current interlocutor")
		}
	}
	if ExplicitSelfIntroduction(proposal.Text, input.NPCName) && input.InterlocutorEntityID != "" {
		for _, relation := range input.Relationships {
			if relation.SubjectEntityID == input.InterlocutorEntityID {
				return "known_relationship_introduction", NewError(CodeInvalidArgument, "NPC cannot introduce identity to an authored known relation")
			}
		}
	}
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
		if strings.TrimSpace(proposal.Text) == "" || len([]rune(proposal.Text)) > 2000 || proposal.DestinationPlaceID != "" || proposal.ActivityCode != "" {
			return "invalid_speech_fields", NewError(CodeInvalidArgument, "NPC speech proposal requires only valid text")
		}
	case "act":
		if proposal.Text != "" || proposal.DestinationPlaceID != "" || proposal.IntroduceSelf {
			return "act_contains_effects", NewError(CodeInvalidArgument, "NPC act proposal cannot include speech or movement")
		}
		legal := false
		for _, code := range input.LegalActivities {
			if code == proposal.ActivityCode {
				legal = true
				break
			}
		}
		if !legal {
			return "activity_not_legal", NewError(CodeInvalidArgument, "NPC act activity is not declared or already in progress")
		}
	case "leave":
		if proposal.Text != "" || proposal.ActivityCode != "" {
			return "movement_contains_speech", NewError(CodeInvalidArgument, "NPC leave proposal cannot include speech or activity")
		}
		for _, placeID := range input.ReachablePlaceIDs {
			if placeID == proposal.DestinationPlaceID {
				return "", nil
			}
		}
		return "destination_not_reachable", NewError(CodeInvalidArgument, "NPC leave destination is not reachable")
	case "silence", "wait":
		if proposal.Text != "" || proposal.DestinationPlaceID != "" || proposal.ActivityCode != "" {
			return "noop_contains_effects", NewError(CodeInvalidArgument, "NPC no-op proposal cannot contain effects")
		}
	default:
		return "unknown_action", NewError(CodeInvalidArgument, "unknown NPC proposal action")
	}
	return "", nil
}

func boundedRPPrivateText(value string, limit int) bool {
	return utf8.ValidString(value) && utf8.RuneCountInString(value) <= limit && !strings.ContainsAny(value, "\x00\r\n")
}

// Evidence refs are provenance handles, not claims the model may fabricate.
// Keep this list limited to source Events actually included in the packet.
func RPDecisionEvidenceEventIDs(input RPDecisionInput) map[string]bool {
	ids := map[string]bool{}
	var walk func(reflect.Value)
	walk = func(value reflect.Value) {
		for value.IsValid() && (value.Kind() == reflect.Interface || value.Kind() == reflect.Pointer) {
			if value.IsNil() {
				return
			}
			value = value.Elem()
		}
		if !value.IsValid() {
			return
		}
		switch value.Kind() {
		case reflect.Struct:
			kind := value.Type()
			for i := 0; i < value.NumField(); i++ {
				field := kind.Field(i)
				if !field.IsExported() {
					continue
				}
				child := value.Field(i)
				if strings.HasSuffix(field.Name, "EventID") && child.Kind() == reflect.String {
					if id := child.String(); id != "" {
						ids[id] = true
					}
					continue
				}
				if strings.HasSuffix(field.Name, "EventIDs") && child.Kind() == reflect.Slice {
					for j := 0; j < child.Len(); j++ {
						if id := child.Index(j).String(); id != "" {
							ids[id] = true
						}
					}
					continue
				}
				walk(child)
			}
		case reflect.Slice, reflect.Array:
			for i := 0; i < value.Len(); i++ {
				walk(value.Index(i))
			}
		case reflect.Map:
			for _, key := range value.MapKeys() {
				walk(value.MapIndex(key))
			}
		}
	}
	walk(reflect.ValueOf(input))
	return ids
}
