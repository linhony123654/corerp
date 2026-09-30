package core

import "strings"

type RPSceneObjectActionRequest struct {
	PrincipalID    string `json:"principal_id"`
	SessionID      string `json:"session_id"`
	ExpectedCursor int64  `json:"expected_cursor"`
	IdempotencyKey string `json:"idempotency_key"`
	ObjectID       string `json:"object_id"`
	Action         string `json:"action"`
}

func (r RPSceneObjectActionRequest) Validate() error {
	if strings.TrimSpace(r.PrincipalID) == "" || strings.TrimSpace(r.SessionID) == "" || strings.TrimSpace(r.ObjectID) == "" || strings.TrimSpace(r.IdempotencyKey) == "" || len(r.IdempotencyKey) > 128 {
		return NewError(CodeInvalidArgument, "principal_id, session_id, object_id and bounded idempotency_key are required")
	}
	switch r.Action {
	case "open", "close", "switch_on", "switch_off":
		return nil
	default:
		return NewError(CodeInvalidArgument, "unsupported scene object action")
	}
}
