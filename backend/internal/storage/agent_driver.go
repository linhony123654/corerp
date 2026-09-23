package storage

import (
	"context"
	"time"

	"corerp.local/backend/internal/core"
)

type AgentDriverResult struct {
	Status         string             `json:"status"`
	Ticks          int                `json:"ticks"`
	ProcessedItems int                `json:"processed_items"`
	LastRun        AgentLifeRunResult `json:"last_run"`
}

func (s *Store) DriveM2AgentRoutine(ctx context.Context, interval time.Duration, batch int) (AgentDriverResult, error) {
	if interval < time.Millisecond || interval > 24*time.Hour {
		return AgentDriverResult{}, core.NewError(core.CodeInvalidArgument, "Agent driver interval must be between 1ms and 24h")
	}
	if batch < 1 || batch > 10000 {
		return AgentDriverResult{}, core.NewError(core.CodeInvalidArgument, "Agent driver batch must be between 1 and 10000")
	}
	var declared, scheduleCount int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM commands WHERE command_id = ? AND instance_id = ? AND branch_id = ? AND status = 'committed'`, m2RoutineCommandID, M2DemoInstanceID, M2DemoBranchID).Scan(&declared); err != nil {
		return AgentDriverResult{}, core.WrapError(core.CodeStorageFailure, "check Agent routine definition", err)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM agent_schedule_entries WHERE definition_event_id IN (?, ?)`, m2AgentSetupEventID, m2RoutineEventID).Scan(&scheduleCount); err != nil {
		return AgentDriverResult{}, core.WrapError(core.CodeStorageFailure, "count Agent routine schedules", err)
	}
	if declared != 1 || scheduleCount != 120 {
		return AgentDriverResult{}, core.NewError(core.CodeBranchConflict, "Agent routine must be explicitly defined before unattended execution")
	}
	request := core.AgentLifeRunRequest{
		PrincipalID: "principal_creator", CapabilityID: "world.agent.run",
		InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID,
		TargetWorldTime: M2AgentDay30NoonTime, Budget: batch,
	}
	result := AgentDriverResult{Status: "running"}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			result.Status = "interrupted"
			return result, nil
		}
		run, err := s.RunAgentLifeAuthorized(ctx, request)
		if err != nil {
			if ctx.Err() != nil {
				result.Status = "interrupted"
				return result, nil
			}
			return result, err
		}
		result.Ticks++
		result.ProcessedItems += run.ProcessedItems
		result.LastRun = run
		if run.Status == "completed" {
			var completed int
			if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM agent_schedule_entries WHERE definition_event_id IN (?, ?) AND status = 'completed'`, m2AgentSetupEventID, m2RoutineEventID).Scan(&completed); err != nil {
				return result, core.WrapError(core.CodeStorageFailure, "verify completed Agent routine", err)
			}
			if completed != 120 || run.CurrentWorldTime != M2AgentDay30NoonTime {
				return result, core.NewError(core.CodeProjectionDiverged, "Agent routine ended before all declared movements were committed")
			}
			result.Status = "completed"
			return result, nil
		}
		select {
		case <-ctx.Done():
			result.Status = "interrupted"
			return result, nil
		case <-ticker.C:
		}
	}
}
