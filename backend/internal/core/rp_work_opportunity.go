package core

// A reaction to an already-known work change, not authority to change a job.
type RPWorkOpportunityContext struct {
	Memory   RPLifeMemory `json:"memory"`
	Selected bool         `json:"selected"`
}

func RPWorkChangeMemory(memory RPLifeMemory, actor string) bool {
	if memory.SubjectEntityID != actor || memory.SourceEventID == "" || memory.WorldTime == "" {
		return false
	}
	switch memory.Kind {
	case "own_employment_accepted", "own_employment_regularized", "own_raise_announced", "own_employment_exit_notice", "own_position_change_agreed", "own_leave_decision", "own_overtime_decision", "own_aggregate_exit_notice", "own_aggregate_employment_ended":
		return true
	}
	return false
}

func RPSelectedWorkChange(input RPDecisionInput) bool {
	if input.WorkOpportunity == nil || !input.WorkOpportunity.Selected || input.Life == nil || !RPWorkChangeMemory(input.WorkOpportunity.Memory, input.NPCEntityID) {
		return false
	}
	for _, memory := range input.Life.SalientMemories {
		if memory == input.WorkOpportunity.Memory {
			return true
		}
	}
	return false
}
