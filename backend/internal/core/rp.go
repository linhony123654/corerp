package core

import (
	"strings"
	"time"
)

// RPSessionOpenRequest binds a player principal to an existing world entity.
// The idempotency key identifies one requested session, not a world command.
type RPSessionOpenRequest struct {
	PrincipalID    string `json:"principal_id"`
	InstanceID     string `json:"instance_id"`
	BranchID       string `json:"branch_id"`
	EntityID       string `json:"entity_id"`
	POV            string `json:"pov"`
	IdempotencyKey string `json:"idempotency_key"`
}

func (r RPSessionOpenRequest) Validate() error {
	for name, value := range map[string]string{
		"principal_id": r.PrincipalID, "instance_id": r.InstanceID,
		"branch_id": r.BranchID, "entity_id": r.EntityID,
		"idempotency_key": r.IdempotencyKey,
	} {
		if strings.TrimSpace(value) == "" {
			return NewError(CodeInvalidArgument, name+" is required")
		}
	}
	if len(r.IdempotencyKey) > 128 {
		return NewError(CodeInvalidArgument, "idempotency_key is too long")
	}
	if r.POV != "first_person" && r.POV != "second_person" {
		return NewError(CodeInvalidArgument, "pov must be first_person or second_person")
	}
	return nil
}

type RPSessionReadRequest struct {
	PrincipalID string `json:"principal_id"`
	SessionID   string `json:"session_id"`
}

type RPMoveRequest struct {
	PrincipalID    string `json:"principal_id"`
	SessionID      string `json:"session_id"`
	FromPlaceID    string `json:"from_place_id"`
	ToPlaceID      string `json:"to_place_id"`
	ExpectedCursor int64  `json:"expected_cursor"`
	IdempotencyKey string `json:"idempotency_key"`
}

func (r RPMoveRequest) Validate() error {
	for name, value := range map[string]string{
		"principal_id": r.PrincipalID, "session_id": r.SessionID,
		"from_place_id": r.FromPlaceID, "to_place_id": r.ToPlaceID,
		"idempotency_key": r.IdempotencyKey,
	} {
		if strings.TrimSpace(value) == "" {
			return NewError(CodeInvalidArgument, name+" is required")
		}
	}
	if len(r.IdempotencyKey) > 128 {
		return NewError(CodeInvalidArgument, "idempotency_key is too long")
	}
	if r.FromPlaceID == r.ToPlaceID {
		return NewError(CodeInvalidArgument, "move destination must differ from origin")
	}
	if r.ExpectedCursor < 1 {
		return NewError(CodeInvalidArgument, "expected_cursor must be a positive observation version")
	}
	return nil
}

type RPWaitRequest struct {
	PrincipalID     string `json:"principal_id"`
	SessionID       string `json:"session_id"`
	TargetWorldTime string `json:"target_world_time"`
	Budget          int    `json:"budget"`
	ExpectedCursor  int64  `json:"expected_cursor"`
	IdempotencyKey  string `json:"idempotency_key"`
}

func (r RPWaitRequest) Validate() error {
	if strings.TrimSpace(r.PrincipalID) == "" || strings.TrimSpace(r.SessionID) == "" || strings.TrimSpace(r.IdempotencyKey) == "" {
		return NewError(CodeInvalidArgument, "principal_id, session_id and idempotency_key are required")
	}
	if len(r.IdempotencyKey) > 128 || r.Budget < 1 || r.Budget > 10000 || r.ExpectedCursor < 1 {
		return NewError(CodeInvalidArgument, "invalid RP wait key, budget or observation cursor")
	}
	if _, err := time.Parse(time.RFC3339, r.TargetWorldTime); err != nil {
		return WrapError(CodeInvalidArgument, "target_world_time must be RFC 3339", err)
	}
	return nil
}

type RPSpeechRequest struct {
	PrincipalID    string `json:"principal_id"`
	SessionID      string `json:"session_id"`
	Text           string `json:"text"`
	SpeechAct      string `json:"speech_act,omitempty"`
	ExpectedCursor int64  `json:"expected_cursor"`
	IdempotencyKey string `json:"idempotency_key"`
}

func (r RPSpeechRequest) Validate() error {
	if strings.TrimSpace(r.PrincipalID) == "" || strings.TrimSpace(r.SessionID) == "" || strings.TrimSpace(r.IdempotencyKey) == "" {
		return NewError(CodeInvalidArgument, "principal_id, session_id and idempotency_key are required")
	}
	if len(r.IdempotencyKey) > 128 || r.ExpectedCursor < 1 {
		return NewError(CodeInvalidArgument, "invalid RP speech key or observation cursor")
	}
	if strings.TrimSpace(r.Text) == "" || len([]rune(r.Text)) > 2000 {
		return NewError(CodeInvalidArgument, "speech text must have 1–2000 characters")
	}
	if r.SpeechAct != "" && r.SpeechAct != "statement" && r.SpeechAct != "question" && r.SpeechAct != "request" {
		return NewError(CodeInvalidArgument, "speech_act must be statement, question or request")
	}
	return nil
}

func (r RPSessionReadRequest) Validate() error {
	if strings.TrimSpace(r.PrincipalID) == "" || strings.TrimSpace(r.SessionID) == "" {
		return NewError(CodeInvalidArgument, "principal_id and session_id are required")
	}
	return nil
}
