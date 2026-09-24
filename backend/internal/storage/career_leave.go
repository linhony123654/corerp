package storage

import (
	"context"
	"database/sql"
	"time"

	"corerp.local/backend/internal/core"
)

type CareerCancelledSchedule struct {
	ScheduleID      string `json:"schedule_id"`
	SchedulerItemID string `json:"scheduler_item_id"`
}

type CareerLeaveFact struct {
	AutoReview          *CareerLeaveAutoReview       `json:"auto_review,omitempty"`
	ReviewAssessment    *CareerLeaveReviewAssessment `json:"review_assessment,omitempty"`
	ContractID          string                       `json:"contract_id"`
	StartDay            int                          `json:"start_day"`
	EndDay              int                          `json:"end_day"`
	Reason              string                       `json:"reason"`
	Status              string                       `json:"status"`
	RequestEventID      string                       `json:"request_event_id"`
	TermsEventID        string                       `json:"terms_event_id"`
	ReviewerPrincipalID string                       `json:"reviewer_principal_id,omitempty"`
	Notice              string                       `json:"notice,omitempty"`
	CancelledSchedules  []CareerCancelledSchedule    `json:"cancelled_schedules,omitempty"`
}

func careerLeaveWindow(ctx context.Context, conn *sql.Conn, b core.CareerBinding, contract string, first, end int, worldTime string) (CareerEmploymentFact, string, error) {
	var empty CareerEmploymentFact
	if b.InstanceID != M2DemoInstanceID || b.BranchID != M2DemoBranchID {
		return empty, "", core.NewError(core.CodeInvalidArgument, "career leave requires supported M2 calendar")
	}
	if _, err := readCareerContractOrganization(ctx, conn, b, contract); err != nil {
		return empty, "", err
	}
	var status string
	var starts int
	if err := conn.QueryRowContext(ctx, `SELECT status,starts_on_day FROM employment_contracts WHERE contract_id=?`, contract).Scan(&status, &starts); err != nil {
		return empty, "", err
	}
	now, err := time.Parse(time.RFC3339Nano, worldTime)
	if err != nil {
		return empty, "", err
	}
	day := int(now.Sub(time.Date(2026, time.September, 22, 0, 0, 0, 0, time.UTC)) / (24 * time.Hour))
	if status != "active" || first < starts || first < day || first > day+30 || end <= first || end-first > 30 {
		return empty, "", core.NewError(core.CodeBranchConflict, "leave requires active employment and a bounded upcoming work window")
	}
	job, source, err := readCareerEmploymentTerms(ctx, conn, contract, first)
	if err != nil {
		return empty, "", err
	}
	ends, err := careerPlannedEndDay(ctx, conn, contract)
	if err != nil {
		return empty, "", err
	}
	if ends > 0 && end > ends {
		return empty, "", core.NewError(core.CodeBranchConflict, "leave extends beyond employment exit")
	}
	shift, err := time.Parse(time.RFC3339, careerTime(first, job.WorkStartHour, 0))
	if err != nil || !now.Before(shift) {
		return empty, "", core.NewError(core.CodeBranchConflict, "leave must be decided before the first shift")
	}
	return job, source, nil
}

func (s *Store) RequestCareerLeave(ctx context.Context, r core.CareerLeaveRequest) (CareerRecord, error) {
	if err := r.Validate(); err != nil {
		return CareerRecord{}, err
	}
	b := r.Binding
	return s.executeCareerCommand(ctx, b, "RequestCareerLeave", r, func(conn *sql.Conn) error {
		return authorizeCareerRecordCandidate(ctx, conn, b, "employment", r.ContractID)
	}, func(conn *sql.Conn, c careerCommandContext) (CareerFact, func() error, error) {
		if err := requireNewCareerRecord(ctx, conn, b, "leave", r.LeaveID); err != nil {
			return CareerFact{}, nil, err
		}
		job, source, err := careerLeaveWindow(ctx, conn, b, r.ContractID, r.StartDay, r.EndDay, c.WorldTime)
		if err != nil {
			return CareerFact{}, nil, err
		}
		var overlap int
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM events e WHERE e.instance_id=? AND e.branch_id=? AND e.event_type='RPCareerFactRecorded' AND json_extract(e.payload,'$.kind')='leave' AND json_extract(e.payload,'$.leave.contract_id')=? AND json_extract(e.payload,'$.leave.start_day')<? AND json_extract(e.payload,'$.leave.end_day')>? AND json_extract(e.payload,'$.leave.status') IN ('requested','approved') AND NOT EXISTS (SELECT 1 FROM events n WHERE n.instance_id=e.instance_id AND n.branch_id=e.branch_id AND n.event_type=e.event_type AND json_extract(n.payload,'$.kind')='leave' AND json_extract(n.payload,'$.record_id')=json_extract(e.payload,'$.record_id') AND n.event_sequence>e.event_sequence)`, b.InstanceID, b.BranchID, job.ContractID, r.EndDay, r.StartDay).Scan(&overlap); err != nil {
			return CareerFact{}, nil, err
		}
		if overlap != 0 {
			return CareerFact{}, nil, core.NewError(core.CodeBranchConflict, "leave overlaps a pending or approved request")
		}
		// The employee's explicit request also pins compatible work ownership
		// for older accepted contracts without adoption metadata. No old Event
		// is rewritten and merely sharing an employee ID does not imply ownership.
		adopted, err := careerAdoptableWorkSchedules(ctx, conn, job, r.StartDay, r.EndDay)
		if err != nil {
			return CareerFact{}, nil, err
		}
		auto, err := planCareerLeaveAutoReview(ctx, conn, b, job, c.EventID, c.WorldTime, r.StartDay)
		if err != nil {
			return CareerFact{}, nil, err
		}
		fact := CareerFact{Kind: "leave", RecordID: r.LeaveID, OrganizationID: job.OrganizationID, CandidateID: job.EmployeeID, AdoptedWorkScheduleIDs: adopted, Leave: &CareerLeaveFact{ContractID: job.ContractID, StartDay: r.StartDay, EndDay: r.EndDay, Reason: r.Reason, Status: "requested", RequestEventID: c.EventID, TermsEventID: source, AutoReview: auto}}
		if auto == nil {
			return fact, nil, nil
		}
		return fact, func() error { return queueCareerLeaveReview(ctx, conn, b, c.EventID, *auto) }, nil
	})
}

func (s *Store) ReviewCareerLeave(ctx context.Context, r core.CareerLeaveReviewRequest) (CareerRecord, error) {
	if err := r.Validate(); err != nil {
		return CareerRecord{}, err
	}
	b := r.Binding
	return s.executeCareerCommand(ctx, b, "ReviewCareerLeave", r, func(conn *sql.Conn) error {
		return authorizeCareerRecordManager(ctx, conn, b, "leave", r.LeaveID)
	}, func(conn *sql.Conn, c careerCommandContext) (CareerFact, func() error, error) {
		return prepareCareerLeaveReview(ctx, conn, b, r.LeaveID, r.Decision, r.Notice, c.WorldTime)
	})
}

func prepareCareerLeaveReview(ctx context.Context, conn *sql.Conn, b core.CareerBinding, leaveID, decision, notice, worldTime string) (CareerFact, func() error, error) {
	record, err := readCareerRecord(ctx, conn, b.InstanceID, b.BranchID, "leave", leaveID)
	if err != nil {
		return CareerFact{}, nil, err
	}
	leave := record.Fact.Leave
	if leave == nil || leave.Status != "requested" {
		return CareerFact{}, nil, core.NewError(core.CodeBranchConflict, "leave request is no longer pending")
	}
	leave.ReviewerPrincipalID, leave.Notice = b.PrincipalID, notice
	if decision == "consider" {
		if _, _, err := careerLeaveWindow(ctx, conn, b, leave.ContractID, leave.StartDay, leave.EndDay, worldTime); err != nil {
			return CareerFact{}, nil, err
		}
		assessment, err := considerCareerLeave(ctx, conn, b, record.Fact)
		if err != nil {
			return CareerFact{}, nil, err
		}
		leave.ReviewAssessment = &assessment
		decision = assessment.Decision
	}
	if decision == "reject" {
		// Closing an expired request changes no past schedule or pay and
		// must not leave its future days reserved forever.
		leave.Status = "rejected"
		return record.Fact, nil, nil
	}
	job, source, err := careerLeaveWindow(ctx, conn, b, leave.ContractID, leave.StartDay, leave.EndDay, worldTime)
	if err != nil {
		return CareerFact{}, nil, err
	}
	leave.TermsEventID = source
	overtime, err := careerAcceptedOvertime(ctx, conn, job.ContractID, leave.StartDay, leave.EndDay)
	if err != nil {
		return CareerFact{}, nil, err
	}
	if len(overtime) != 0 {
		return CareerFact{}, nil, core.NewError(core.CodeBranchConflict, "cancel accepted overtime before approving leave")
	}
	leave.Status = "approved"
	leave.CancelledSchedules, err = careerOwnedSchedules(ctx, conn, b, job, leave.StartDay, leave.EndDay, record.Fact.AdoptedWorkScheduleIDs)
	if err != nil {
		return CareerFact{}, nil, err
	}
	return record.Fact, func() error {
		for _, schedule := range leave.CancelledSchedules {
			if err := execAgentOne(ctx, conn, "cancel approved-leave work entry", `UPDATE agent_schedule_entries SET status='cancelled' WHERE schedule_id=? AND agent_id=? AND status='active'`, schedule.ScheduleID, job.EmployeeID); err != nil {
				return err
			}
			if err := execAgentOne(ctx, conn, "cancel approved-leave scheduler item", `UPDATE scheduler_items SET status='cancelled' WHERE scheduler_item_id=? AND instance_id=? AND branch_id=? AND status='pending'`, schedule.SchedulerItemID, b.InstanceID, b.BranchID); err != nil {
				return err
			}
		}
		return nil
	}, nil
}

func careerApprovedLeave(ctx context.Context, conn *sql.Conn, contract string, day int) (string, error) {
	var event string
	err := conn.QueryRowContext(ctx, `SELECT event_id FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPCareerFactRecorded' AND json_extract(payload,'$.kind')='leave' AND json_extract(payload,'$.leave.contract_id')=? AND json_extract(payload,'$.leave.status')='approved' AND json_extract(payload,'$.leave.start_day')<=? AND json_extract(payload,'$.leave.end_day')>? ORDER BY event_sequence DESC LIMIT 1`, M2DemoInstanceID, M2DemoBranchID, contract, day, day).Scan(&event)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return event, err
}

// Record adoption once in the acceptance Event. Later cancellation must not
// guess ownership from merely being an employee's scheduled appointment.
func careerAdoptableWorkSchedules(ctx context.Context, conn *sql.Conn, job CareerEmploymentFact, first, end int) ([]string, error) {
	rows, err := conn.QueryContext(ctx, `SELECT schedule_id,world_time FROM agent_schedule_entries WHERE agent_id=? AND status='active' AND place_id=? AND activity_code='work' ORDER BY world_time,schedule_id`, job.EmployeeID, job.WorkplaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id, raw string
		if err := rows.Scan(&id, &raw); err != nil {
			return nil, err
		}
		at, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			return nil, err
		}
		day := int(at.Sub(time.Date(2026, time.September, 22, 0, 0, 0, 0, time.UTC)) / (24 * time.Hour))
		if day >= job.StartsOnDay && day >= first && (end == 0 || day < end) && raw == careerTime(day, job.WorkStartHour, 0) {
			ids = append(ids, id)
		}
	}
	return ids, rows.Err()
}

func careerOwnedSchedules(ctx context.Context, conn *sql.Conn, b core.CareerBinding, job CareerEmploymentFact, first, end int, requestedIDs []string) ([]CareerCancelledSchedule, error) {
	requested, err := core.CanonicalJSON(requestedIDs)
	if err != nil {
		return nil, err
	}
	rows, err := conn.QueryContext(ctx, `SELECT s.schedule_id,s.scheduler_item_id FROM agent_schedule_entries s JOIN scheduler_items q ON q.scheduler_item_id=s.scheduler_item_id JOIN events definition ON definition.event_id=s.definition_event_id
	 WHERE s.agent_id=? AND s.status='active' AND q.status='pending' AND q.instance_id=? AND q.branch_id=? AND s.world_time>=? AND (?=0 OR s.world_time<?) AND (
	 (json_extract(definition.payload,'$.employment.contract_id')=? OR (definition.event_type='WageObligationAccrued' AND json_extract(definition.payload,'$.contract_id')=?))
	 OR EXISTS (SELECT 1 FROM employment_contracts c JOIN events accepted ON accepted.event_id=c.definition_event_id JOIN json_each(accepted.payload,'$.adopted_work_schedule_ids') owned WHERE c.contract_id=? AND owned.value=s.schedule_id)
	 OR EXISTS (SELECT 1 FROM json_each(?) requested WHERE requested.value=s.schedule_id)) ORDER BY s.world_time,s.schedule_id`, job.EmployeeID, b.InstanceID, b.BranchID, careerTime(first, 0, 0), end, careerTime(end, 0, 0), job.ContractID, job.ContractID, job.ContractID, string(requested))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var schedules []CareerCancelledSchedule
	for rows.Next() {
		var schedule CareerCancelledSchedule
		if err := rows.Scan(&schedule.ScheduleID, &schedule.SchedulerItemID); err != nil {
			return nil, err
		}
		schedules = append(schedules, schedule)
	}
	return schedules, rows.Err()
}
