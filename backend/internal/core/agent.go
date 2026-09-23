package core

import (
	"strings"
	"time"
)

type AgentLifeRunRequest struct {
	PrincipalID     string `json:"principal_id"`
	CapabilityID    string `json:"capability_id"`
	InstanceID      string `json:"instance_id"`
	BranchID        string `json:"branch_id"`
	TargetWorldTime string `json:"target_world_time"`
	Budget          int    `json:"budget"`
}

type AgentRoutineRequest struct {
	PrincipalID  string `json:"principal_id"`
	CapabilityID string `json:"capability_id"`
	InstanceID   string `json:"instance_id"`
	BranchID     string `json:"branch_id"`
	Days         int    `json:"days"`
}

func (r AgentRoutineRequest) Validate() error {
	if err := validateAgentScope(r.PrincipalID, r.CapabilityID, r.InstanceID, r.BranchID); err != nil {
		return err
	}
	if r.CapabilityID != "world.agent.run" {
		return NewError(CodeUnauthorized, "defining an Agent routine requires world.agent.run")
	}
	if r.Days != 30 {
		return NewError(CodeInvalidArgument, "the M2 Agent routine spans exactly 30 days")
	}
	return nil
}

func (r AgentLifeRunRequest) Validate() error {
	if err := validateAgentScope(r.PrincipalID, r.CapabilityID, r.InstanceID, r.BranchID); err != nil {
		return err
	}
	if _, err := time.Parse(time.RFC3339, r.TargetWorldTime); err != nil {
		return WrapError(CodeInvalidArgument, "target_world_time must be RFC 3339", err)
	}
	if r.Budget <= 0 || r.Budget > 10000 {
		return NewError(CodeInvalidArgument, "budget must be between 1 and 10000")
	}
	return nil
}

type AgentKnowledgeRead struct {
	PrincipalID     string   `json:"principal_id"`
	CapabilityID    string   `json:"capability_id"`
	InstanceID      string   `json:"instance_id"`
	BranchID        string   `json:"branch_id"`
	ObserverAgentID string   `json:"observer_agent_id"`
	Fields          []string `json:"fields"`
}

func (r AgentKnowledgeRead) Validate() error {
	if err := validateAgentScope(r.PrincipalID, r.CapabilityID, r.InstanceID, r.BranchID); err != nil {
		return err
	}
	if strings.TrimSpace(r.ObserverAgentID) == "" {
		return NewError(CodeInvalidArgument, "observer_agent_id is required")
	}
	return validateAgentFields(r.Fields)
}

type EncounterRead struct {
	PrincipalID     string   `json:"principal_id"`
	CapabilityID    string   `json:"capability_id"`
	InstanceID      string   `json:"instance_id"`
	BranchID        string   `json:"branch_id"`
	ObserverAgentID string   `json:"observer_agent_id"`
	Fields          []string `json:"fields"`
}

func (r EncounterRead) Validate() error {
	if err := validateAgentScope(r.PrincipalID, r.CapabilityID, r.InstanceID, r.BranchID); err != nil {
		return err
	}
	if strings.TrimSpace(r.ObserverAgentID) == "" {
		return NewError(CodeInvalidArgument, "observer_agent_id is required")
	}
	return validateAgentFields(r.Fields)
}

func validateAgentScope(principalID, capabilityID, instanceID, branchID string) error {
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

func validateAgentFields(fields []string) error {
	if len(fields) == 0 || len(fields) > 8 {
		return NewError(CodeInvalidArgument, "between 1 and 8 unique fields are required")
	}
	seen := make(map[string]bool, len(fields))
	for _, field := range fields {
		if strings.TrimSpace(field) == "" || seen[field] {
			return NewError(CodeInvalidArgument, "fields must be nonempty and unique")
		}
		seen[field] = true
	}
	return nil
}
