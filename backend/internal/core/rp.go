package core

import "strings"

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

func (r RPSessionReadRequest) Validate() error {
	if strings.TrimSpace(r.PrincipalID) == "" || strings.TrimSpace(r.SessionID) == "" {
		return NewError(CodeInvalidArgument, "principal_id and session_id are required")
	}
	return nil
}
