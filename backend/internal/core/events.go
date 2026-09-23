package core

import "strings"

type VisibleEventRequest struct {
	PrincipalID   string `json:"principal_id"`
	CapabilityID  string `json:"capability_id"`
	InstanceID    string `json:"instance_id"`
	BranchID      string `json:"branch_id"`
	SubjectID     string `json:"subject_id"`
	Limit         int    `json:"limit"`
	AfterSequence int64  `json:"-"`
}

func (r VisibleEventRequest) Validate() error {
	required := map[string]string{
		"principal_id": r.PrincipalID, "capability_id": r.CapabilityID,
		"instance_id": r.InstanceID, "branch_id": r.BranchID, "subject_id": r.SubjectID,
	}
	for name, value := range required {
		if strings.TrimSpace(value) == "" {
			return NewError(CodeInvalidArgument, name+" is required")
		}
	}
	if r.Limit <= 0 || r.Limit > 100 {
		return NewError(CodeInvalidArgument, "event limit must be between 1 and 100")
	}
	if r.AfterSequence < 0 {
		return NewError(CodeInvalidArgument, "event cursor sequence must be nonnegative")
	}
	return nil
}
