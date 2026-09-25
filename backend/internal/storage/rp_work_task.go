package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"corerp.local/backend/internal/core"
)

// An attempt is an in-world work sample, not attendance, payroll or a
// manager's performance review. Only its bounded outcome is manager-readable.
type RPWorkTaskRequest struct {
	Binding    core.CareerBinding `json:"binding"`
	SessionID  string             `json:"session_id"`
	ContractID string             `json:"contract_id"`
	TaskCode   string             `json:"task_code"`
}

type RPWorkTaskFact struct {
	Version           string `json:"version"`
	EntityID          string `json:"entity_id"`
	ContractID        string `json:"contract_id"`
	Day               int    `json:"day"`
	TaskCode          string `json:"task_code"`
	Outcome           string `json:"outcome"`
	WorkplaceID       string `json:"workplace_id"`
	TermsEventID      string `json:"terms_event_id"`
	WorkSourceEventID string `json:"work_source_event_id"`
	FatigueLevel      string `json:"fatigue_level,omitempty"`
	ConditionImpact   string `json:"condition_impact,omitempty"`
}

type RPWorkTaskRecord = privateFactRecord[RPWorkTaskFact]

type RPWorkTaskObservation struct {
	EventID    string `json:"event_id"`
	WorldTime  string `json:"world_time"`
	ContractID string `json:"contract_id"`
	Day        int    `json:"day"`
	TaskCode   string `json:"task_code"`
	Outcome    string `json:"outcome"`
}

func (s *Store) AttemptRPWorkTask(ctx context.Context, r RPWorkTaskRequest) (RPWorkTaskRecord, error) {
	if err := r.Binding.Validate(); err != nil {
		return RPWorkTaskRecord{}, err
	}
	if !studioID(r.SessionID) || strings.TrimSpace(r.SessionID) != r.SessionID ||
		!studioID(r.ContractID) || r.TaskCode != "routine_check" {
		return RPWorkTaskRecord{}, core.NewError(core.CodeInvalidArgument, "bounded RP work session, contract and routine_check task required")
	}
	replay, fresh := rpActorActionAuthorizers(ctx, r.Binding, r.SessionID)
	return executePrivateFactCommandWithOptions(s, ctx, r.Binding, "AttemptRPWorkTask", r,
		privateFactDomain{"rp_work_task", "RPWorkTaskAttempted", `{"authorization":"current-employee-work-sample-v1"}`},
		privateFactOptions{replayAuthorize: replay}, fresh,
		func(conn *sql.Conn, c privateFactContext) (RPWorkTaskFact, func() error, error) {
			var fact RPWorkTaskFact
			if err := requireRPHealthChildWindow(ctx, conn, r.Binding, r.SessionID, r, "work_task"); err != nil {
				return fact, nil, err
			}
			session, err := loadRPSessionRecord(ctx, conn, r.Binding.PrincipalID, r.SessionID)
			if err != nil {
				return fact, nil, err
			}
			job, terms, day, err := currentCareerEmployment(ctx, conn, r.Binding, r.ContractID, c.WorldTime)
			if err != nil {
				return fact, nil, err
			}
			if job.EmployeeID != session.ControlledEntityID {
				return fact, nil, core.NewError(core.CodeUnauthorized, "work sample belongs to another employee")
			}
			if job.WorkStartHour < 0 || job.WorkEndHour > 24 || job.WorkStartHour >= job.WorkEndHour {
				return fact, nil, core.NewError(core.CodeProjectionDiverged, "invalid sourced work shift")
			}
			now, err := time.Parse(time.RFC3339Nano, c.WorldTime)
			if err != nil {
				return fact, nil, err
			}
			start, _ := time.Parse(time.RFC3339, careerTime(day, job.WorkStartHour, 0))
			end, _ := time.Parse(time.RFC3339, careerTime(day, job.WorkEndHour, 0))
			if now.Before(start) || !now.Before(end) {
				return fact, nil, core.NewError(core.CodeBranchConflict, "work sample requires the current shift")
			}
			var place, activity, effective string
			var positionSequence int64
			if err := conn.QueryRowContext(ctx, `SELECT place_id,activity_code,effective_world_time,last_event_sequence FROM agent_positions WHERE agent_id=?`, job.EmployeeID).Scan(&place, &activity, &effective, &positionSequence); err != nil {
				return fact, nil, classifyMissing(err, "employee work position")
			}
			effectiveAt, parseErr := time.Parse(time.RFC3339Nano, effective)
			if parseErr != nil {
				return fact, nil, core.NewError(core.CodeProjectionDiverged, "invalid sourced work position time")
			}
			if place != job.WorkplaceID || activity != "work" || effectiveAt.After(now) {
				return fact, nil, core.NewError(core.CodeBranchConflict, "employee is not performing sourced work at this workplace")
			}
			var workSource string
			if err := conn.QueryRowContext(ctx, `SELECT e.event_id FROM events e WHERE e.instance_id=? AND e.branch_id=?
				AND e.event_sequence=? AND e.actor_id=? AND e.world_time=? AND
				(EXISTS(SELECT 1 FROM agent_movements m WHERE m.event_id=e.event_id AND m.agent_id=? AND m.to_place_id=? AND m.activity_code='work')
				 OR (e.event_type='AgentActivityStarted' AND json_extract(e.payload,'$.to_place_id')=? AND json_extract(e.payload,'$.activity_code')='work'))`,
				r.Binding.InstanceID, r.Binding.BranchID, positionSequence, job.EmployeeID, effective, job.EmployeeID, place, place).Scan(&workSource); err != nil {
				return fact, nil, classifyMissing(err, "sourced employee work position")
			}
			if open, _, _, err := openRPSleep(ctx, conn, r.Binding.InstanceID, r.Binding.BranchID, job.EmployeeID); err != nil {
				return fact, nil, err
			} else if open != "" {
				return fact, nil, core.NewError(core.CodeBranchConflict, "end actor sleep before work sample")
			}
			var existing int
			if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPWorkTaskAttempted'
				AND json_extract(payload,'$.contract_id')=? AND json_extract(payload,'$.day')=? AND json_extract(payload,'$.task_code')=?`,
				r.Binding.InstanceID, r.Binding.BranchID, r.ContractID, day, r.TaskCode).Scan(&existing); err != nil {
				return fact, nil, err
			}
			if existing != 0 {
				return fact, nil, core.NewError(core.CodeBranchConflict, "work sample already recorded for this shift and task")
			}
			truth, err := readRPSleepTruth(ctx, conn, r.Binding.InstanceID, r.Binding.BranchID, job.EmployeeID, c.WorldTime)
			if err != nil {
				return fact, nil, err
			}
			conditions, err := readRPActiveConditions(ctx, conn, r.Binding.InstanceID, r.Binding.BranchID, job.EmployeeID, c.WorldTime)
			if err != nil {
				return fact, nil, err
			}
			outcome := "completed"
			conditionImpact := ""
			for _, condition := range conditions {
				if condition.Kind == "minor_injury" && condition.Severity >= 3 {
					return fact, nil, core.NewError(core.CodeBranchConflict, "current injury limits this work task")
				}
				if condition.Severity >= 2 {
					conditionImpact = condition.FunctionalImpact
					outcome = "recheck_required"
				}
			}
			if truth.FatigueLevel == "moderate" {
				outcome = "recheck_required"
			}
			fact = RPWorkTaskFact{Version: "corerp.work_task.v1", EntityID: job.EmployeeID, ContractID: job.ContractID,
				Day: day, TaskCode: r.TaskCode, Outcome: outcome, WorkplaceID: place,
				TermsEventID: terms, WorkSourceEventID: workSource, FatigueLevel: truth.FatigueLevel,
				ConditionImpact: conditionImpact}
			return fact, nil, nil
		})
}

// A manager sees a task outcome, never its private fatigue cause or sleep IDs.
// No MCP/HTTP route exposes this internal business read yet.
func (s *Store) ReadRPWorkTask(ctx context.Context, principal, instance, branch, eventID string) (RPWorkTaskObservation, error) {
	var result RPWorkTaskObservation
	if principal == "" || instance == "" || branch == "" || eventID == "" {
		return result, core.NewError(core.CodeInvalidArgument, "work task reader and scope required")
	}
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return result, err
	}
	defer tx.Rollback(ctx)
	var raw string
	if err := tx.conn.QueryRowContext(ctx, `SELECT event_id,world_time,payload FROM events WHERE event_id=? AND instance_id=? AND branch_id=? AND event_type='RPWorkTaskAttempted'`,
		eventID, instance, branch).Scan(&result.EventID, &result.WorldTime, &raw); err != nil {
		return RPWorkTaskObservation{}, classifyMissing(err, "scoped work task Event")
	}
	var fact RPWorkTaskFact
	if err := json.Unmarshal([]byte(raw), &fact); err != nil {
		return RPWorkTaskObservation{}, err
	}
	if fact.Version != "corerp.work_task.v1" || fact.ContractID == "" || fact.EntityID == "" || fact.TaskCode != "routine_check" ||
		(fact.Outcome != "completed" && fact.Outcome != "recheck_required") {
		return RPWorkTaskObservation{}, core.NewError(core.CodeProjectionDiverged, "invalid sourced work task")
	}
	var employee, organization string
	if err := tx.conn.QueryRowContext(ctx, `SELECT c.employee_entity_id,c.employer_entity_id FROM employment_contracts c JOIN events e ON e.event_id=c.definition_event_id
		WHERE c.contract_id=? AND e.instance_id=? AND e.branch_id=?`, fact.ContractID, instance, branch).Scan(&employee, &organization); err != nil {
		return RPWorkTaskObservation{}, classifyMissing(err, "scoped work task employment")
	}
	if employee != fact.EntityID {
		return RPWorkTaskObservation{}, core.NewError(core.CodeProjectionDiverged, "work task employee differs from contract")
	}
	b := core.CareerBinding{PrincipalID: principal, InstanceID: instance, BranchID: branch}
	if err := authorizeCareerCandidate(ctx, tx.conn, b, employee); err != nil {
		if !core.HasCode(err, core.CodeUnauthorized) {
			return RPWorkTaskObservation{}, err
		}
		if err := authorizeCareerManager(ctx, tx.conn, b, organization); err != nil {
			return RPWorkTaskObservation{}, err
		}
	}
	return RPWorkTaskObservation{EventID: result.EventID, WorldTime: result.WorldTime, ContractID: fact.ContractID,
		Day: fact.Day, TaskCode: fact.TaskCode, Outcome: fact.Outcome}, nil
}
