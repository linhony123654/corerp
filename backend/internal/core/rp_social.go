package core

import (
	"strings"
	"time"
)

// Explicit interpersonal acts are not inferred from free-form dialogue.
type RPSocialRequest struct {
	PrincipalID      string `json:"principal_id"`
	SessionID        string `json:"session_id"`
	TargetEntityID   string `json:"target_entity_id"`
	Action           string `json:"action"`
	AmountMinor      int64  `json:"amount_minor,omitempty"`
	PromiseEventID   string `json:"promise_event_id,omitempty"`
	MeetingPlaceID   string `json:"meeting_place_id,omitempty"`
	MeetingWorldTime string `json:"meeting_world_time,omitempty"`
	ExpectedCursor   int64  `json:"expected_cursor"`
	IdempotencyKey   string `json:"idempotency_key"`
}

func (r RPSocialRequest) Validate() error {
	if strings.TrimSpace(r.PrincipalID) == "" || strings.TrimSpace(r.SessionID) == "" || strings.TrimSpace(r.TargetEntityID) == "" || r.ExpectedCursor < 1 || strings.TrimSpace(r.IdempotencyKey) == "" || len(r.IdempotencyKey) > 128 {
		return NewError(CodeInvalidArgument, "invalid interpersonal request binding")
	}
	switch r.Action {
	case "greet", "insult", "apologize", "gift", "promise_meeting", "keep_meeting":
	default:
		return NewError(CodeInvalidArgument, "unsupported interpersonal action")
	}
	if r.Action == "gift" {
		if r.AmountMinor < 1 || r.AmountMinor > MaxJSONSafeInteger {
			return NewError(CodeInvalidArgument, "gift amount outside supported range")
		}
	} else if r.AmountMinor != 0 {
		return NewError(CodeInvalidArgument, "only gift may transfer money")
	}
	if (r.Action == "keep_meeting") != (r.PromiseEventID != "") {
		return NewError(CodeInvalidArgument, "meeting fulfillment requires original promise")
	}
	if r.Action == "promise_meeting" {
		if _, err := time.Parse(time.RFC3339, r.MeetingWorldTime); err != nil || r.MeetingPlaceID == "" {
			return NewError(CodeInvalidArgument, "meeting place and RFC3339 time required")
		}
	} else if r.MeetingPlaceID != "" || r.MeetingWorldTime != "" {
		return NewError(CodeInvalidArgument, "only promise may define a meeting")
	}
	return nil
}

type RPSocialEvidence struct {
	ClaimType        string `json:"claim_type"`
	SessionID        string `json:"session_id"`
	ActorEntityID    string `json:"actor_entity_id"`
	TargetEntityID   string `json:"target_entity_id"`
	Action           string `json:"action"`
	PlaceID          string `json:"place_id"`
	AmountMinor      int64  `json:"amount_minor,omitempty"`
	PromiseEventID   string `json:"promise_event_id,omitempty"`
	MeetingPlaceID   string `json:"meeting_place_id,omitempty"`
	MeetingWorldTime string `json:"meeting_world_time,omitempty"`
	Description      string `json:"description"`
}

// Relationship dimensions are one deterministic view of observed actions.
// A gift cannot prove a claim true, and an apology cannot manufacture trust.
func ApplyRPSocialEvidence(relation *RPRelationship, observer, eventID string, e RPSocialEvidence) {
	relation.Familiarity++
	recipient := observer == e.TargetEntityID
	switch e.Action {
	case "greet":
		relation.Affinity++
	case "gift":
		if recipient {
			relation.Trust++
			relation.Affinity += 2
		}
	case "insult":
		relation.Tension += 2
		relation.Affinity -= 2
	case "apologize":
		relation.Tension--
		relation.Affinity++
	case "promise_meeting":
		relation.Role = "meeting_partner"
		if recipient {
			relation.Obligation--
		} else {
			relation.Obligation++
		}
	case "keep_meeting":
		if recipient {
			relation.Obligation++
			relation.Trust += 2
		} else {
			relation.Obligation--
			relation.Trust++
		}
	}
	clamp := func(n, min, max int) int {
		if n < min {
			return min
		}
		if n > max {
			return max
		}
		return n
	}
	relation.Familiarity = clamp(relation.Familiarity, 0, 100)
	relation.Trust = clamp(relation.Trust, -100, 100)
	relation.Affinity = clamp(relation.Affinity, -100, 100)
	relation.Tension = clamp(relation.Tension, 0, 100)
	relation.SourceEventIDs = append(relation.SourceEventIDs, eventID)
	if len(relation.SourceEventIDs) > 8 {
		relation.SourceEventIDs = relation.SourceEventIDs[len(relation.SourceEventIDs)-8:]
	}
}
