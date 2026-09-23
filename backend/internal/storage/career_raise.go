package storage

import (
	"context"
	"database/sql"
	"encoding/json"

	"corerp.local/backend/internal/core"
)

func (s *Store) RaiseCareerWage(ctx context.Context, r core.CareerRaiseRequest) (CareerRecord, error) {
	if err := r.Validate(); err != nil {
		return CareerRecord{}, err
	}
	b := r.Binding
	return s.executeCareerCommand(ctx, b, "RaiseCareerWage", r, func(conn *sql.Conn) error {
		org, err := readCareerContractOrganization(ctx, conn, b, r.ContractID)
		if err != nil {
			return err
		}
		return authorizeCareerManager(ctx, conn, b, org)
	}, func(conn *sql.Conn, c careerCommandContext) (CareerFact, func() error, error) {
		job, source, day, err := currentCareerEmployment(ctx, conn, b, r.ContractID, c.WorldTime)
		if err != nil {
			return CareerFact{}, nil, err
		}
		if r.DailyWageMinor <= job.DailyWageMinor || r.EffectiveFromDay <= day || r.EffectiveFromDay > day+30 {
			return CareerFact{}, nil, core.NewError(core.CodeInvalidArgument, "raise requires a higher wage and a future day within thirty days")
		}
		var pending int
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPCareerFactRecorded' AND json_extract(payload,'$.employment.contract_id')=? AND json_extract(payload,'$.employment.effective_from_day')>?`, b.InstanceID, b.BranchID, job.ContractID, day).Scan(&pending); err != nil {
			return CareerFact{}, nil, err
		}
		if pending != 0 {
			return CareerFact{}, nil, core.NewError(core.CodeBranchConflict, "resolve pending employment terms before another raise")
		}
		agreements, err := careerAcceptedOvertime(ctx, conn, job.ContractID, r.EffectiveFromDay, day+31)
		if err != nil {
			return CareerFact{}, nil, err
		}
		maxima := map[int]int64{}
		for _, agreement := range agreements {
			o := agreement.Fact.Overtime
			amount, exists := maxima[o.Day]
			if !exists {
				amount = r.DailyWageMinor
			}
			bonus, ok := checkedMultiplyPositive(int64(o.EndHour-o.StartHour), o.RateMinorPerHour)
			if !ok {
				return CareerFact{}, nil, core.NewError(core.CodeIntegerOverflow, "accepted overtime maximum overflow")
			}
			amount, ok = checkedAdd(amount, bonus)
			if !ok || amount > core.MaxJSONSafeInteger {
				return CareerFact{}, nil, core.NewError(core.CodeInvalidArgument, "raise and agreed overtime exceed supported daily pay")
			}
			maxima[o.Day] = amount
		}
		job.DailyWageMinor, job.EffectiveFromDay = r.DailyWageMinor, r.EffectiveFromDay
		job.TermVersion++
		fact := CareerFact{Kind: "employment", RecordID: job.ContractID, OrganizationID: job.OrganizationID, CandidateID: job.EmployeeID, Employment: &job, EmploymentChange: &CareerEmploymentChange{Kind: "raise", PreviousTermsEventID: source, ManagerPrincipalID: b.PrincipalID, Notice: r.Notice}}
		return fact, func() error {
			return queueCareerPayrollItem(ctx, conn, c.EventID, r.EffectiveFromDay, 0, "career_terms_effective")
		}, nil
	})
}

func (s *Store) executeCareerTermActivation(ctx context.Context, tx *immediateTx, item SchedulerItem, payload scheduledPayload) error {
	if payload.Day < 1 || item.WorldTime != careerTime(payload.Day, 0, 0) {
		return core.NewError(core.CodeProjectionDiverged, "career term activation time differs")
	}
	var raw string
	if err := tx.conn.QueryRowContext(ctx, `SELECT payload FROM events WHERE event_id=? AND instance_id=? AND branch_id=? AND event_type='RPCareerFactRecorded'`, payload.SubjectID, M2DemoInstanceID, M2DemoBranchID).Scan(&raw); err != nil {
		return classifyMissing(err, "career term source")
	}
	var fact CareerFact
	if err := json.Unmarshal([]byte(raw), &fact); err != nil || fact.Employment == nil || fact.Employment.EffectiveFromDay != payload.Day {
		return core.NewError(core.CodeProjectionDiverged, "invalid employment term source")
	}
	initial := fact.EmploymentChange == nil && fact.Offer != nil && fact.Offer.Status == "accepted" && fact.Employment.TermVersion == 1 && fact.Employment.StartsOnDay == payload.Day
	exiting := fact.Exit != nil && fact.EmploymentChange != nil && fact.Exit.Kind == fact.EmploymentChange.Kind && fact.Exit.EffectiveFromDay == payload.Day && fact.Employment.EndsOnDay == payload.Day && fact.Employment.LifecycleStatus == "ended"
	if !initial && !exiting && (fact.EmploymentChange == nil || (fact.EmploymentChange.Kind != "raise" && (fact.PositionChange == nil || fact.PositionChange.Status != "accepted" || fact.PositionChange.Kind != fact.EmploymentChange.Kind))) {
		return core.NewError(core.CodeProjectionDiverged, "employment change lacks accepted position source")
	}
	job := fact.Employment
	_, latest, err := readCareerEmploymentTerms(ctx, tx.conn, job.ContractID, payload.Day)
	if err != nil {
		return err
	}
	if latest != payload.SubjectID {
		return core.NewError(core.CodeProjectionDiverged, "queued career term source was superseded")
	}
	var previous int64
	var previousPosition string
	previousSource := payload.SubjectID
	if !initial {
		previousSource = fact.EmploymentChange.PreviousTermsEventID
	}
	if err := tx.conn.QueryRowContext(ctx, `SELECT json_extract(payload,'$.employment.daily_wage_minor'),json_extract(payload,'$.employment.position_key') FROM events WHERE event_id=? AND instance_id=? AND branch_id=? AND json_extract(payload,'$.employment.contract_id')=?`, previousSource, M2DemoInstanceID, M2DemoBranchID, job.ContractID).Scan(&previous, &previousPosition); err != nil {
		return classifyMissing(err, "previous wage terms")
	}
	roleGrant, err := prepareCareerRoleGrant(ctx, tx.conn, *job)
	if err != nil {
		return err
	}
	status := "active"
	var cancelled []CareerCancelledSchedule
	if exiting {
		status = "ended"
		cancelled, err = careerOwnedSchedules(ctx, tx.conn, core.CareerBinding{InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID}, *job, payload.Day, 0, fact.AdoptedWorkScheduleIDs)
		if err != nil {
			return err
		}
	}
	mutation := scheduledMutation{Private: true, EventType: "CareerEmploymentTermsActivated", EventPayload: struct {
		ContractID         string                    `json:"contract_id"`
		TermsEventID       string                    `json:"terms_event_id"`
		EffectiveFromDay   int                       `json:"effective_from_day"`
		DailyWageMinor     int64                     `json:"daily_wage_minor"`
		PositionKey        string                    `json:"position_key"`
		RoleGrant          *CareerRoleGrant          `json:"role_grant,omitempty"`
		ContractStatus     string                    `json:"contract_status"`
		CancelledSchedules []CareerCancelledSchedule `json:"cancelled_schedules,omitempty"`
	}{job.ContractID, payload.SubjectID, payload.Day, job.DailyWageMinor, job.PositionKey, roleGrant, status, cancelled}, ApplyDomainRows: func(ctx context.Context, conn *sql.Conn, eventID string, _ int64) error {
		if err := execAgentOne(ctx, conn, "apply employment terms at effective boundary", `UPDATE employment_contracts SET gross_wage_minor=?,position_id=?,status=? WHERE contract_id=? AND status='active' AND gross_wage_minor=? AND position_id=?`, job.DailyWageMinor, job.PositionKey, status, job.ContractID, previous, previousPosition); err != nil {
			return err
		}
		for _, schedule := range cancelled {
			if err := execAgentOne(ctx, conn, "cancel ended employment work", `UPDATE agent_schedule_entries SET status='cancelled' WHERE schedule_id=? AND agent_id=? AND status='active'`, schedule.ScheduleID, job.EmployeeID); err != nil {
				return err
			}
			if err := execAgentOne(ctx, conn, "cancel ended employment schedule item", `UPDATE scheduler_items SET status='cancelled' WHERE scheduler_item_id=? AND instance_id=? AND branch_id=? AND status='pending'`, schedule.SchedulerItemID, M2DemoInstanceID, M2DemoBranchID); err != nil {
				return err
			}
		}
		if roleGrant != nil {
			return applyCareerRoleGrant(ctx, conn, M2DemoInstanceID, M2DemoBranchID, eventID, *roleGrant)
		}
		return nil
	}}
	if err := s.commitScheduledMutationForBranch(ctx, tx, item, payload, mutation, M2DemoInstanceID, M2DemoBranchID); err != nil {
		return err
	}
	if s.beforeCommit != nil {
		return s.beforeCommit()
	}
	return nil
}
