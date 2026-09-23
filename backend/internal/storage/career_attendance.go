package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"corerp.local/backend/internal/core"
)

// Recorded work is evidence of activity, not a quality score, approved leave,
// authorized overtime, or a reason to deduct guaranteed base pay.
type CareerAttendance struct {
	ContractID      string   `json:"contract_id"`
	EmployeeID      string   `json:"employee_id"`
	Day             int      `json:"day"`
	WorkplaceID     string   `json:"workplace_id"`
	StartsAt        string   `json:"starts_at"`
	EndsAt          string   `json:"ends_at"`
	ExpectedSeconds int64    `json:"expected_seconds"`
	RecordedSeconds int64    `json:"recorded_seconds"`
	Status          string   `json:"status"`
	TermsEventID    string   `json:"terms_event_id"`
	WorkEventIDs    []string `json:"work_event_ids"`
	LeaveEventID    string   `json:"leave_event_id,omitempty"`
}

type CareerAttendanceRecord struct {
	EventID             string                 `json:"event_id"`
	EventSequence       int64                  `json:"event_sequence"`
	WorldTime           string                 `json:"world_time"`
	Attendance          CareerAttendance       `json:"attendance"`
	BaseEarnedMinor     int64                  `json:"base_earned_minor"`
	OvertimeEarnedMinor int64                  `json:"overtime_earned_minor"`
	Overtime            []CareerOvertimeEarned `json:"overtime,omitempty"`
}

type careerActivityPoint struct {
	at       time.Time
	eventID  string
	place    string
	activity string
}

func deriveCareerAttendance(ctx context.Context, conn *sql.Conn, job CareerEmploymentFact, source string, day int, asOf string) (CareerAttendance, error) {
	var empty CareerAttendance
	if day < job.StartsOnDay || job.WorkStartHour < 0 || job.WorkEndHour > 24 || job.WorkStartHour >= job.WorkEndHour {
		return empty, core.NewError(core.CodeProjectionDiverged, "invalid career attendance work window")
	}
	startText, endText := careerTime(day, job.WorkStartHour, 0), careerTime(day, job.WorkEndHour, 0)
	start, _ := time.Parse(time.RFC3339, startText)
	end, _ := time.Parse(time.RFC3339, endText)
	through, err := time.Parse(time.RFC3339Nano, asOf)
	if err != nil || through.Before(end) {
		return empty, core.NewError(core.CodeInvalidArgument, "cannot finalize an unfinished work window")
	}
	// Same authoritative sources as position replay, including voluntary RP
	// moves and same-place activity changes. Neither schedule intent nor the
	// mutable latest-position projection establishes historical attendance.
	rows, err := conn.QueryContext(ctx, `SELECT e.event_sequence,e.event_id,e.world_time,m.to_place_id,m.activity_code
	 FROM agent_movements m JOIN events e ON e.event_id=m.event_id WHERE m.agent_id=? AND e.instance_id=? AND e.branch_id=?
	 UNION ALL SELECT e.event_sequence,e.event_id,e.world_time,json_extract(e.payload,'$.to_place_id'),json_extract(e.payload,'$.activity_code')
	 FROM events e WHERE e.actor_id=? AND e.instance_id=? AND e.branch_id=? AND e.event_type='AgentActivityStarted' ORDER BY 1`, job.EmployeeID, M2DemoInstanceID, M2DemoBranchID, job.EmployeeID, M2DemoInstanceID, M2DemoBranchID)
	if err != nil {
		return empty, err
	}
	defer rows.Close()
	var baseline *careerActivityPoint
	var changes []careerActivityPoint
	var last time.Time
	for rows.Next() {
		var point careerActivityPoint
		var seq int64
		var at string
		if err := rows.Scan(&seq, &point.eventID, &at, &point.place, &point.activity); err != nil {
			return empty, err
		}
		point.at, err = time.Parse(time.RFC3339Nano, at)
		if err != nil || (!last.IsZero() && point.at.Before(last)) {
			return empty, core.NewError(core.CodeProjectionDiverged, "career attendance activity chronology differs")
		}
		last = point.at
		if !point.at.Before(end) {
			break
		}
		if !point.at.After(start) {
			copy := point
			baseline = &copy
		} else {
			changes = append(changes, point)
		}
	}
	if err := rows.Err(); err != nil {
		return empty, err
	}
	rows.Close()
	if baseline == nil {
		return empty, core.NewError(core.CodeProjectionDiverged, "career attendance lacks initial position evidence")
	}
	result := CareerAttendance{ContractID: job.ContractID, EmployeeID: job.EmployeeID, Day: day, WorkplaceID: job.WorkplaceID, StartsAt: startText, EndsAt: endText, ExpectedSeconds: int64(end.Sub(start) / time.Second), TermsEventID: source, WorkEventIDs: []string{}}
	current, cursor := *baseline, start
	var duration time.Duration
	seen := map[string]bool{}
	accumulate := func(until time.Time) {
		if until.After(cursor) && current.place == job.WorkplaceID && current.activity == "work" {
			duration += until.Sub(cursor)
			if !seen[current.eventID] {
				result.WorkEventIDs = append(result.WorkEventIDs, current.eventID)
				seen[current.eventID] = true
			}
		}
		cursor = until
	}
	for _, point := range changes {
		accumulate(point.at)
		current = point
	}
	accumulate(end)
	result.RecordedSeconds = int64(duration / time.Second)
	switch {
	case duration == end.Sub(start):
		result.Status = "complete"
	case duration > 0:
		result.Status = "partial"
	default:
		result.Status = "no_recorded_work"
	}
	leave, err := careerApprovedLeave(ctx, conn, job.ContractID, day)
	if err != nil {
		return empty, err
	}
	if leave != "" {
		result.LeaveEventID, result.Status, result.ExpectedSeconds = leave, "approved_leave", 0
	}
	return result, nil
}

func (s *Store) ReadCareerAttendance(ctx context.Context, principal, instance, branch, contract string, day int) (CareerAttendanceRecord, error) {
	var result CareerAttendanceRecord
	if principal == "" || instance == "" || branch == "" || contract == "" || day < 0 {
		return result, core.NewError(core.CodeInvalidArgument, "attendance scope, contract and day required")
	}
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return result, err
	}
	defer tx.Rollback(ctx)
	var employee, organization string
	if err := tx.conn.QueryRowContext(ctx, `SELECT c.employee_entity_id,c.employer_entity_id FROM employment_contracts c JOIN events e ON e.event_id=c.definition_event_id WHERE c.contract_id=? AND e.instance_id=? AND e.branch_id=? AND e.event_type='RPCareerFactRecorded' AND json_extract(e.payload,'$.employment.contract_id')=c.contract_id`, contract, instance, branch).Scan(&employee, &organization); err != nil {
		return result, classifyMissing(err, "scoped career employment")
	}
	b := core.CareerBinding{PrincipalID: principal, InstanceID: instance, BranchID: branch}
	if err := authorizeCareerCandidate(ctx, tx.conn, b, employee); err != nil {
		if !core.HasCode(err, core.CodeUnauthorized) {
			return result, err
		}
		if err := authorizeCareerManager(ctx, tx.conn, b, organization); err != nil {
			return result, err
		}
	}
	var raw, overtime string
	if err := tx.conn.QueryRowContext(ctx, `SELECT event_id,event_sequence,world_time,json_extract(payload,'$.attendance'),COALESCE(json_extract(payload,'$.base_amount_minor'),json_extract(payload,'$.amount_minor')),COALESCE(json_extract(payload,'$.overtime_amount_minor'),0),COALESCE(json_extract(payload,'$.overtime'),'[]') FROM events WHERE instance_id=? AND branch_id=? AND event_type='WageObligationAccrued' AND json_extract(payload,'$.contract_id')=? AND json_extract(payload,'$.attendance.day')=? ORDER BY event_sequence LIMIT 1`, instance, branch, contract, day).Scan(&result.EventID, &result.EventSequence, &result.WorldTime, &raw, &result.BaseEarnedMinor, &result.OvertimeEarnedMinor, &overtime); err != nil {
		return result, classifyMissing(err, "completed career attendance")
	}
	if err := json.Unmarshal([]byte(raw), &result.Attendance); err != nil {
		return result, err
	}
	if err := json.Unmarshal([]byte(overtime), &result.Overtime); err != nil {
		return result, err
	}
	return result, nil
}
