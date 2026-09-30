package core

import "strings"

// ValidateRPInteractionProposal checks an interpreter's bounded proposal
// against the player's own candidate set. Storage checks the live world again
// before each typed command, so this is never authority for an effect.
func ValidateRPInteractionProposal(input RPInteractionUnderstandingInput, plan RPInteractionPlan) error {
	if input.Mode != "AUTO" || plan.Mode != input.Mode {
		return NewError(CodeInvalidArgument, "interaction proposal mode differs from authorized input")
	}
	if len(plan.Steps) > 3 {
		return NewError(CodeInvalidArgument, "interaction proposal exceeds step budget")
	}
	if plan.Kind == "CLARIFICATION" {
		if len(plan.Steps) != 0 || strings.TrimSpace(plan.Clarification) == "" || len([]rune(plan.Clarification)) > 300 {
			return NewError(CodeInvalidArgument, "invalid interaction clarification")
		}
		return nil
	}
	if plan.Clarification != "" || len(plan.Steps) == 0 {
		return NewError(CodeInvalidArgument, "interaction proposal lacks executable steps")
	}
	actions, speeches := 0, 0
	for index, step := range plan.Steps {
		switch step.Kind {
		case "speech":
			if strings.TrimSpace(step.SpeechText) == "" || len([]rune(step.SpeechText)) > 2000 || !strings.Contains(input.Text, step.SpeechText) || step.TargetPlaceID != "" || step.TargetEntityID != "" || step.WaitHours != 0 || step.WaitMinutes != 0 || step.ObjectAction != "" || step.ObjectID != "" || step.AnchorID != "" || step.OfferID != "" || step.NonverbalAction != "" || step.GestureCode != "" {
				return NewError(CodeInvalidArgument, "speech proposal must quote the player's own input")
			}
			speeches++
		case "move":
			if step.TargetPlaceID == "" || step.TargetEntityID != "" || step.WaitHours != 0 || step.WaitMinutes != 0 || step.SpeechText != "" || step.ObjectAction != "" || step.ObjectID != "" || step.AnchorID != "" || step.OfferID != "" || step.NonverbalAction != "" || step.GestureCode != "" {
				return NewError(CodeInvalidArgument, "invalid proposed movement")
			}
			matched, grounded := false, false
			for _, place := range input.ReachablePlaces {
				if place.ID == step.TargetPlaceID {
					matched = true
					name := strings.TrimSpace(place.Name)
					grounded = name != "" && strings.Contains(strings.ToLower(input.Text), strings.ToLower(name))
				}
			}
			if !matched {
				return NewError(CodeInvalidArgument, "proposed destination is not reachable")
			}
			if !grounded {
				return NewError(CodeInvalidArgument, "proposed destination is not named by the player")
			}
			actions++
		case "wait":
			if step.TargetPlaceID != "" || step.TargetEntityID != "" || step.SpeechText != "" || step.ObjectAction != "" || step.ObjectID != "" || step.AnchorID != "" || step.OfferID != "" || step.NonverbalAction != "" || step.GestureCode != "" || (step.WaitMinutes != 0 && step.WaitHours != 0) || (step.WaitMinutes != 15 && step.WaitHours != 1 && step.WaitHours != 2 && step.WaitHours != 4) {
				return NewError(CodeInvalidArgument, "invalid proposed wait duration")
			}
			actions++
		case "object":
			predictiveOffer := index == 1 && rpInteractionTakeThenOffer(plan.Steps[0], step)
			if err := validateRPInteractionObject(input, step, predictiveOffer); err != nil {
				return err
			}
			actions++
		case "nonverbal":
			if step.TargetPlaceID != "" || step.WaitHours != 0 || step.WaitMinutes != 0 || step.SpeechText != "" || step.ObjectAction != "" || step.ObjectID != "" || step.AnchorID != "" || step.OfferID != "" {
				return NewError(CodeInvalidArgument, "nonverbal proposal includes an unrelated effect")
			}
			switch step.NonverbalAction {
			case "look_at":
				if step.TargetEntityID == "" || step.GestureCode != "" {
					return NewError(CodeInvalidArgument, "looking requires a visible target")
				}
			case "smile", "nod", "shake_head", "turn_away":
				if step.GestureCode != "" {
					return NewError(CodeInvalidArgument, "nonverbal proposal has an unexpected gesture")
				}
			case "gesture":
				if step.GestureCode != "wave" && step.GestureCode != "shrug" && step.GestureCode != "raise_hand" {
					return NewError(CodeInvalidArgument, "unsupported neutral gesture")
				}
			default:
				return NewError(CodeInvalidArgument, "unsupported nonverbal action")
			}
			if step.TargetEntityID != "" && !rpInteractionHasEntity(input, step.TargetEntityID) {
				return NewError(CodeInvalidArgument, "nonverbal target was not observed")
			}
			actions++
		default:
			return NewError(CodeInvalidArgument, "unsupported interaction proposal")
		}
	}
	switch plan.Kind {
	case "DIALOGUE":
		if speeches != 1 || actions != 0 {
			return NewError(CodeInvalidArgument, "dialogue proposal contains non-speech effects")
		}
		if plan.Steps[0].SpeechText != strings.TrimSpace(input.Text) {
			return NewError(CodeInvalidArgument, "dialogue proposal must preserve the full player's speech")
		}
	case "ACTION", "MIXED":
		if plan.Kind == "ACTION" && (speeches != 0 || actions == 0) || plan.Kind == "MIXED" && (speeches != 1 || actions == 0 || plan.Steps[len(plan.Steps)-1].Kind != "speech") {
			return NewError(CodeInvalidArgument, "action proposal has incompatible steps")
		}
		if actions > 2 || len(plan.Steps) != actions+speeches || (actions == 2 && !rpInteractionTakeThenOffer(plan.Steps[0], plan.Steps[1])) {
			return NewError(CodeInvalidArgument, "interaction action order is unsupported")
		}
		if plan.Kind == "MIXED" {
			quoted, ok := rpInteractionSingleQuotedSpeech(input.Text)
			if !ok || plan.Steps[len(plan.Steps)-1].SpeechText != quoted {
				return NewError(CodeInvalidArgument, "mixed proposal must quote the complete explicit player speech")
			}
		}
		for _, step := range plan.Steps[:actions] {
			if step.Kind == "wait" && step.WaitMinutes != 0 {
				return NewError(CodeInvalidArgument, "fifteen-minute wait is only a continue intent")
			}
		}
	case "CONTINUE":
		if len(plan.Steps) != 1 || speeches != 0 || actions != 1 || plan.Steps[0].Kind != "wait" || plan.Steps[0].WaitMinutes != 15 {
			return NewError(CodeInvalidArgument, "continue may only advance fifteen minutes")
		}
	default:
		return NewError(CodeInvalidArgument, "unknown interaction proposal kind")
	}
	return nil
}

// BindRPInteractionSpeech fills only an empty provider speech slot from the
// player's own input. It never changes speech that a provider explicitly wrote.
func BindRPInteractionSpeech(input RPInteractionUnderstandingInput, plan RPInteractionPlan) RPInteractionPlan {
	if plan.Kind != "DIALOGUE" && plan.Kind != "MIXED" {
		return plan
	}
	speech := strings.TrimSpace(input.Text)
	if plan.Kind == "MIXED" {
		var ok bool
		speech, ok = rpInteractionSingleQuotedSpeech(input.Text)
		if !ok {
			return plan
		}
	}
	for i := range plan.Steps {
		if plan.Steps[i].Kind == "speech" && plan.Steps[i].SpeechText == "" {
			plan.Steps[i].SpeechText = speech
		}
	}
	return plan
}

func rpInteractionSingleQuotedSpeech(text string) (string, bool) {
	var speech string
	found := 0
	for _, pair := range [][2]string{{"「", "」"}, {"“", "”"}, {`"`, `"`}} {
		openCount, closeCount := strings.Count(text, pair[0]), strings.Count(text, pair[1])
		if pair[0] == pair[1] {
			if openCount != 0 && openCount != 2 {
				return "", false
			}
			if openCount == 0 {
				continue
			}
		} else if openCount != closeCount || openCount > 1 {
			return "", false
		} else if openCount == 0 {
			continue
		}
		start := strings.Index(text, pair[0]) + len(pair[0])
		end := strings.Index(text[start:], pair[1])
		if end < 0 {
			return "", false
		}
		speech = text[start : start+end]
		found++
	}
	return speech, found == 1 && strings.TrimSpace(speech) != ""
}

func rpInteractionHasEntity(input RPInteractionUnderstandingInput, id string) bool {
	for _, entity := range input.PresentEntities {
		if entity.ID == id {
			return true
		}
	}
	return false
}

func rpInteractionTakeThenOffer(take, offer RPInteractionStep) bool {
	return take.Kind == "object" && take.ObjectAction == "take" && take.ObjectID != "" && offer.Kind == "object" && offer.ObjectAction == "offer" && offer.ObjectID == take.ObjectID
}

func validateRPInteractionObject(input RPInteractionUnderstandingInput, step RPInteractionStep, predictiveOffer bool) error {
	if step.TargetPlaceID != "" || step.WaitHours != 0 || step.WaitMinutes != 0 || step.SpeechText != "" || step.NonverbalAction != "" || step.GestureCode != "" || step.ObjectAction == "" {
		return NewError(CodeInvalidArgument, "object proposal includes an unrelated effect")
	}
	if step.ObjectAction == "stage" || step.ObjectAction == "cancel_offer" || step.ObjectAction == "stow" {
		return NewError(CodeInvalidArgument, "object staging and cancellation require explicit typed authority")
	}
	allowed := false
	if step.OfferID == "" {
		if step.ObjectID == "" {
			return NewError(CodeInvalidArgument, "object proposal lacks a known item")
		}
		for _, object := range input.Objects {
			if object.ID == step.ObjectID {
				for _, action := range object.AllowedActions {
					allowed = allowed || action == step.ObjectAction || predictiveOffer && step.ObjectAction == "offer" && action == "take"
				}
			}
		}
	} else {
		for _, offer := range input.Offers {
			if offer.ID == step.OfferID && (step.ObjectID == "" || step.ObjectID == offer.ObjectID) {
				for _, action := range offer.AllowedActions {
					allowed = allowed || action == step.ObjectAction
				}
			}
		}
	}
	if !allowed {
		return NewError(CodeInvalidArgument, "object action is not an authorized candidate")
	}
	switch step.ObjectAction {
	case "take":
		if step.AnchorID != "" || step.OfferID != "" || step.TargetEntityID != "" {
			return NewError(CodeInvalidArgument, "invalid item pick-up target")
		}
	case "place", "move":
		if step.AnchorID == "" || step.OfferID != "" || step.TargetEntityID != "" {
			return NewError(CodeInvalidArgument, "object placement requires only a known item and observed anchor")
		}
		found := false
		for _, anchor := range input.Anchors {
			found = found || anchor.ID == step.AnchorID
		}
		if !found {
			return NewError(CodeInvalidArgument, "object anchor is not observable")
		}
		if step.ObjectAction == "move" {
			for _, object := range input.Objects {
				if object.ID == step.ObjectID && object.AnchorID != "" && object.AnchorID == step.AnchorID {
					return NewError(CodeInvalidArgument, "object move must target a different observed anchor")
				}
			}
		}
	case "offer":
		if step.AnchorID != "" || step.OfferID != "" || !rpInteractionHasEntity(input, step.TargetEntityID) {
			return NewError(CodeInvalidArgument, "offer requires a visible target")
		}
	case "accept", "refuse", "give", "receive":
		if step.AnchorID != "" || step.ObjectID != "" || step.OfferID == "" || step.TargetEntityID != "" {
			return NewError(CodeInvalidArgument, "offer response requires only an exact offer candidate")
		}
	default:
		return NewError(CodeInvalidArgument, "unsupported object action")
	}
	return nil
}
