package core

import "strings"

type PrivateEconomicRead struct {
	PrincipalID  string   `json:"principal_id"`
	CapabilityID string   `json:"capability_id"`
	InstanceID   string   `json:"instance_id"`
	BranchID     string   `json:"branch_id"`
	SubjectID    string   `json:"subject_id"`
	Fields       []string `json:"fields"`
}

func (r PrivateEconomicRead) Validate() error {
	required := map[string]string{
		"principal_id": r.PrincipalID, "capability_id": r.CapabilityID,
		"instance_id": r.InstanceID, "branch_id": r.BranchID, "subject_id": r.SubjectID,
	}
	for name, value := range required {
		if strings.TrimSpace(value) == "" {
			return NewError(CodeInvalidArgument, name+" is required")
		}
	}
	if len(r.Fields) == 0 {
		return NewError(CodeInvalidArgument, "at least one private read field is required")
	}
	seen := map[string]bool{}
	for _, field := range r.Fields {
		if strings.TrimSpace(field) == "" || seen[field] {
			return NewError(CodeInvalidArgument, "private read fields must be nonempty and unique")
		}
		seen[field] = true
	}
	return nil
}
