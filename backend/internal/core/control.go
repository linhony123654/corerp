package core

import "strings"

type StrictSimulationRequest struct {
	PrincipalID  string `json:"principal_id"`
	CapabilityID string `json:"capability_id"`
	InstanceID   string `json:"instance_id"`
	BranchID     string `json:"branch_id"`
	TargetDay    int    `json:"target_day"`
	Budget       int    `json:"budget"`
}

func (r StrictSimulationRequest) Validate() error {
	if err := validateScopedControl(r.PrincipalID, r.CapabilityID, r.InstanceID, r.BranchID); err != nil {
		return err
	}
	if r.TargetDay < 0 || r.TargetDay > 90 {
		return NewError(CodeInvalidArgument, "target_day must be between 0 and 90")
	}
	if r.Budget <= 0 || r.Budget > 100000 {
		return NewError(CodeInvalidArgument, "budget must be between 1 and 100000")
	}
	return nil
}

type StateReadRequest struct {
	PrincipalID  string `json:"principal_id"`
	CapabilityID string `json:"capability_id"`
	InstanceID   string `json:"instance_id"`
	BranchID     string `json:"branch_id"`
}

func (r StateReadRequest) Validate() error {
	return validateScopedControl(r.PrincipalID, r.CapabilityID, r.InstanceID, r.BranchID)
}

func validateScopedControl(principalID, capabilityID, instanceID, branchID string) error {
	required := map[string]string{
		"principal_id": principalID, "capability_id": capabilityID,
		"instance_id": instanceID, "branch_id": branchID,
	}
	for name, value := range required {
		if strings.TrimSpace(value) == "" {
			return NewError(CodeInvalidArgument, name+" is required")
		}
	}
	return nil
}
