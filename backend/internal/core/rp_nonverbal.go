package core

import "strings"

// RPNonverbalExpressionCode names only an already supported observable. It
// does not authorize a target, fabricate a witness, or interpret the gesture.
func RPNonverbalExpressionCode(action, gesture string) string {
	if action == "gesture" {
		switch gesture {
		case "wave", "shrug", "raise_hand", "beckon":
			return gesture
		}
		return ""
	}
	if gesture != "" {
		return ""
	}
	switch action {
	case "look_at", "smile", "nod", "shake_head", "turn_away", "frown":
		return action
	}
	return ""
}

// RPNonverbalRequest is a bounded physical expression, not an interpretation
// of the target's feelings, consent, relationship or response.
type RPNonverbalRequest struct {
	PrincipalID    string `json:"principal_id"`
	SessionID      string `json:"session_id"`
	Action         string `json:"action"`
	TargetEntityID string `json:"target_entity_id,omitempty"`
	GestureCode    string `json:"gesture_code,omitempty"`
	ExpectedCursor int64  `json:"expected_cursor"`
	IdempotencyKey string `json:"idempotency_key"`
}

func (r RPNonverbalRequest) Validate() error {
	if strings.TrimSpace(r.PrincipalID) == "" || strings.TrimSpace(r.SessionID) == "" || len(r.SessionID) > 256 || r.ExpectedCursor < 1 || strings.TrimSpace(r.IdempotencyKey) == "" || len(r.IdempotencyKey) > 128 {
		return NewError(CodeInvalidArgument, "invalid nonverbal action binding")
	}
	if r.Action == "look_at" && r.TargetEntityID == "" {
		return NewError(CodeInvalidArgument, "look_at requires a visible target")
	}
	if r.TargetEntityID != "" && (strings.TrimSpace(r.TargetEntityID) != r.TargetEntityID || len(r.TargetEntityID) > 256) {
		return NewError(CodeInvalidArgument, "invalid nonverbal target")
	}
	if RPNonverbalExpressionCode(r.Action, r.GestureCode) == "" {
		return NewError(CodeInvalidArgument, "unsupported nonverbal action or gesture")
	}
	return nil
}

type RPNonverbalWitness struct {
	ObserverEntityID string `json:"observer_entity_id"`
	TargetVisible    bool   `json:"target_visible"`
}

// RPNonverbalFact is the committed Event payload. Witnesses and their view of
// the target are frozen at commit, never inferred from a later position.
type RPNonverbalFact struct {
	// Optional source lineage for a gesture in a wait-triggered NPC batch.
	TriggerEventID string               `json:"trigger_event_id,omitempty"`
	ClaimType      string               `json:"claim_type"`
	SessionID      string               `json:"session_id"`
	ActorEntityID  string               `json:"actor_entity_id"`
	TargetEntityID string               `json:"target_entity_id,omitempty"`
	Action         string               `json:"action"`
	GestureCode    string               `json:"gesture_code,omitempty"`
	PlaceID        string               `json:"place_id"`
	Description    string               `json:"description"`
	Witnesses      []RPNonverbalWitness `json:"witnesses"`
}

// Each witness knows the actor acted. The target exists in their claim only
// when that witness could see the target as well as the actor at commit time.
type RPNonverbalClaim struct {
	ClaimType      string `json:"claim_type"`
	ActorEntityID  string `json:"actor_entity_id"`
	TargetEntityID string `json:"target_entity_id,omitempty"`
	Action         string `json:"action"`
	GestureCode    string `json:"gesture_code,omitempty"`
	Description    string `json:"description"`
}
