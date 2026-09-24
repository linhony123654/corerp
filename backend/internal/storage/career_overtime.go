package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"corerp.local/backend/internal/core"
)

const careerOvertimePayPolicy = "recorded_seconds_floor_minor_v1"

type CareerOvertimeFact struct {
	Choice             *core.CareerOvertimeChoice `json:"choice,omitempty"`
	ContractID         string                     `json:"contract_id"`
	Day                int                        `json:"day"`
	StartHour          int                        `json:"start_hour"`
	EndHour            int                        `json:"end_hour"`
	RateMinorPerHour   int64                      `json:"rate_minor_per_hour"`
	CurrencyID         string                     `json:"currency_id"`
	WorkplaceID        string                     `json:"workplace_id"`
	PayPolicy          string                     `json:"pay_policy"`
	Reason             string                     `json:"reason"`
	TermsEventID       string                     `json:"terms_event_id"`
	ManagerPrincipalID string                     `json:"manager_principal_id"`
	Status             string                     `json:"status"`
	AcceptedEventID    string                     `json:"accepted_event_id,omitempty"`
	ResponseReason     string                     `json:"response_reason,omitempty"`
	CancelledSchedules []CareerCancelledSchedule  `json:"cancelled_schedules,omitempty"`
}

type CareerOvertimeEarned struct {
	AgreementEventID string           `json:"agreement_event_id"`
	AmountMinor      int64            `json:"amount_minor"`
	RateMinorPerHour int64            `json:"rate_minor_per_hour"`
	PayPolicy        string           `json:"pay_policy"`
	Attendance       CareerAttendance `json:"attendance"`
}

func careerOvertimeWindow(ctx context.Context, conn *sql.Conn, b core.CareerBinding, contract string, day, start, end int, at string) (CareerEmploymentFact, string, error) {
	var empty CareerEmploymentFact
	if b.InstanceID != M2DemoInstanceID || b.BranchID != M2DemoBranchID {
		return empty, "", core.NewError(core.CodeInvalidArgument, "overtime requires supported M2 calendar")
	}
	if _, err := readCareerContractOrganization(ctx, conn, b, contract); err != nil {
		return empty, "", err
	}
	var status string
	var starts int
	if err := conn.QueryRowContext(ctx, `SELECT status,starts_on_day FROM employment_contracts WHERE contract_id=?`, contract).Scan(&status, &starts); err != nil {
		return empty, "", err
	}
	now, err := time.Parse(time.RFC3339Nano, at)
	if err != nil {
		return empty, "", err
	}
	currentDay := int(now.Sub(time.Date(2026, time.September, 22, 0, 0, 0, 0, time.UTC)) / (24 * time.Hour))
	if status != "active" || day < starts || day < currentDay || day > currentDay+30 {
		return empty, "", core.NewError(core.CodeBranchConflict, "overtime needs active employment and an upcoming day")
	}
	job, source, err := readCareerEmploymentTerms(ctx, conn, contract, day)
	if err != nil {
		return empty, "", err
	}
	if job.EndsOnDay > 0 && day >= job.EndsOnDay {
		return empty, "", core.NewError(core.CodeBranchConflict, "overtime is outside employment tenure")
	}
	from, err := time.Parse(time.RFC3339, careerTime(day, start, 0))
	if err != nil || !now.Before(from) || start < job.WorkEndHour || end <= start || end > 23 || end-start > 4 {
		return empty, "", core.NewError(core.CodeBranchConflict, "overtime must be future and outside normal work")
	}
	return job, source, nil
}

func (s *Store) OfferCareerOvertime(ctx context.Context, r core.CareerOvertimeRequest) (CareerRecord, error) {
	if err := r.Validate(); err != nil {
		return CareerRecord{}, err
	}
	b := r.Binding
	return s.executeCareerCommand(ctx, b, "OfferCareerOvertime", r, func(conn *sql.Conn) error {
		org, err := readCareerContractOrganization(ctx, conn, b, r.ContractID)
		if err != nil {
			return err
		}
		return authorizeCareerManager(ctx, conn, b, org)
	}, func(conn *sql.Conn, c careerCommandContext) (CareerFact, func() error, error) {
		if err := requireNewCareerRecord(ctx, conn, b, "overtime", r.OvertimeID); err != nil {
			return CareerFact{}, nil, err
		}
		job, source, err := careerOvertimeWindow(ctx, conn, b, r.ContractID, r.Day, r.StartHour, r.EndHour, c.WorldTime)
		if err != nil {
			return CareerFact{}, nil, err
		}
		if leave, err := careerApprovedLeave(ctx, conn, r.ContractID, r.Day); err != nil || leave != "" {
			if err == nil {
				err = core.NewError(core.CodeBranchConflict, "overtime conflicts with approved leave")
			}
			return CareerFact{}, nil, err
		}
		return CareerFact{Kind: "overtime", RecordID: r.OvertimeID, OrganizationID: job.OrganizationID, CandidateID: job.EmployeeID, Overtime: &CareerOvertimeFact{ContractID: job.ContractID, Day: r.Day, StartHour: r.StartHour, EndHour: r.EndHour, RateMinorPerHour: r.RateMinorPerHour, CurrencyID: job.CurrencyID, WorkplaceID: job.WorkplaceID, PayPolicy: careerOvertimePayPolicy, Reason: r.Reason, TermsEventID: source, ManagerPrincipalID: b.PrincipalID, Status: "offered"}}, nil, nil
	})
}

func (s *Store) RespondCareerOvertime(ctx context.Context, r core.CareerOvertimeResponseRequest) (CareerRecord, error) {
	if err := r.Validate(); err != nil {
		return CareerRecord{}, err
	}
	b := r.Binding
	return s.executeCareerCommand(ctx, b, "RespondCareerOvertime", r, func(conn *sql.Conn) error {
		err := authorizeCareerRecordCandidate(ctx, conn, b, "overtime", r.OvertimeID)
		if r.Decision == "cancel" && core.HasCode(err, core.CodeUnauthorized) {
			return authorizeCareerRecordManager(ctx, conn, b, "overtime", r.OvertimeID)
		}
		return err
	}, func(conn *sql.Conn, c careerCommandContext) (CareerFact, func() error, error) {
		record, err := readCareerRecord(ctx, conn, b.InstanceID, b.BranchID, "overtime", r.OvertimeID)
		if err != nil {
			return CareerFact{}, nil, err
		}
		o := record.Fact.Overtime
		if o == nil || (r.Decision == "cancel" && o.Status != "accepted") || (r.Decision != "cancel" && o.Status != "offered") {
			return CareerFact{}, nil, core.NewError(core.CodeBranchConflict, "overtime response does not match current status")
		}
		start, _ := time.Parse(time.RFC3339, careerTime(o.Day, o.StartHour, 0))
		now, err := time.Parse(time.RFC3339Nano, c.WorldTime)
		if err != nil || !now.Before(start) {
			return CareerFact{}, nil, core.NewError(core.CodeBranchConflict, "overtime response must precede its work window")
		}
		o.ResponseReason = r.Reason
		o.Choice = nil
		decision := r.Decision
		if decision == "consider" {
			input, err := readRPOwnDecisionContext(ctx, conn, core.RPDecisionInput{InstanceID: b.InstanceID, BranchID: b.BranchID, NPCEntityID: record.Fact.CandidateID})
			if err != nil {
				return CareerFact{}, nil, err
			}
			var manager string
			if err := conn.QueryRowContext(ctx, `SELECT agent_id FROM agent_profiles WHERE principal_id=? AND instance_id=? AND branch_id=? AND status='active'`, o.ManagerPrincipalID, b.InstanceID, b.BranchID).Scan(&manager); err != nil && err != sql.ErrNoRows {
				return CareerFact{}, nil, err
			}
			coworkers, err := careerKnownCoworkers(ctx, conn, b, record.Fact.CandidateID, record.Fact.OrganizationID)
			if err != nil {
				return CareerFact{}, nil, err
			}
			choice := core.ChooseCareerOvertime(input, manager, coworkers...)
			o.Choice = &choice
			decision = choice.Decision
			o.ResponseReason = "I choose to " + decision + " this optional overtime."
		}
		if decision == "decline" {
			o.Status = "declined"
			return record.Fact, nil, nil
		}
		if decision == "cancel" {
			o.Status = "cancelled"
			o.CancelledSchedules, err = careerOvertimeSchedules(ctx, conn, b, o.AcceptedEventID)
			if err != nil {
				return CareerFact{}, nil, err
			}
			return record.Fact, func() error {
				for _, item := range o.CancelledSchedules {
					if err := execAgentOne(ctx, conn, "cancel overtime schedule", `UPDATE agent_schedule_entries SET status='cancelled' WHERE schedule_id=? AND status='active'`, item.ScheduleID); err != nil {
						return err
					}
					if err := execAgentOne(ctx, conn, "cancel overtime queue item", `UPDATE scheduler_items SET status='cancelled' WHERE scheduler_item_id=? AND status='pending'`, item.SchedulerItemID); err != nil {
						return err
					}
				}
				return nil
			}, nil
		}
		job, source, err := careerOvertimeWindow(ctx, conn, b, o.ContractID, o.Day, o.StartHour, o.EndHour, c.WorldTime)
		if err != nil {
			return CareerFact{}, nil, err
		}
		if source != o.TermsEventID || o.PayPolicy != careerOvertimePayPolicy || job.CurrencyID != o.CurrencyID {
			return CareerFact{}, nil, core.NewError(core.CodeBranchConflict, "overtime terms were superseded or unsupported")
		}
		manager := b
		manager.PrincipalID = o.ManagerPrincipalID
		if err := authorizeCareerManager(ctx, conn, manager, job.OrganizationID); err != nil {
			return CareerFact{}, nil, err
		}
		if leave, err := careerApprovedLeave(ctx, conn, o.ContractID, o.Day); err != nil || leave != "" {
			if err == nil {
				err = core.NewError(core.CodeBranchConflict, "overtime conflicts with approved leave")
			}
			return CareerFact{}, nil, err
		}
		var conflicts int
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM agent_schedule_entries WHERE agent_id=? AND status='active' AND world_time>=? AND world_time<=?`, job.EmployeeID, careerTime(o.Day, o.StartHour, 0), careerTime(o.Day, o.EndHour, 0)).Scan(&conflicts); err != nil {
			return CareerFact{}, nil, err
		}
		if conflicts != 0 {
			return CareerFact{}, nil, core.NewError(core.CodeBranchConflict, "overtime conflicts with an existing commitment")
		}
		agreements, err := careerAcceptedOvertime(ctx, conn, o.ContractID, o.Day, o.Day+1)
		if err != nil {
			return CareerFact{}, nil, err
		}
		maximum := job.DailyWageMinor
		for _, agreement := range agreements {
			other := agreement.Fact.Overtime
			if o.StartHour < other.EndHour && o.EndHour > other.StartHour {
				return CareerFact{}, nil, core.NewError(core.CodeBranchConflict, "overtime overlaps an accepted work window")
			}
		}
		for _, agreement := range append(agreements, record) {
			terms := agreement.Fact.Overtime
			bonus, ok := checkedMultiplyPositive(int64(terms.EndHour-terms.StartHour), terms.RateMinorPerHour)
			if !ok {
				return CareerFact{}, nil, core.NewError(core.CodeIntegerOverflow, "overtime maximum pay overflow")
			}
			maximum, ok = checkedAdd(maximum, bonus)
			if !ok || maximum > core.MaxJSONSafeInteger {
				return CareerFact{}, nil, core.NewError(core.CodeInvalidArgument, "combined daily pay exceeds supported amount")
			}
		}
		var route int
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_place_links WHERE instance_id=? AND branch_id=? AND from_place_id=? AND to_place_id=?`, b.InstanceID, b.BranchID, job.AfterWorkPlaceID, job.WorkplaceID).Scan(&route); err != nil {
			return CareerFact{}, nil, err
		}
		if route == 0 && job.AfterWorkPlaceID != job.WorkplaceID {
			return CareerFact{}, nil, core.NewError(core.CodeInvalidArgument, "overtime requires an existing return-to-work route")
		}
		job.WorkStartHour, job.WorkEndHour = o.StartHour, o.EndHour
		o.Status, o.AcceptedEventID = "accepted", c.EventID
		return record.Fact, func() error {
			return queueCareerWorkdayWithKey(ctx, conn, job, o.Day, c.EventID, "career_overtime_"+c.EventID)
		}, nil
	})
}

func careerOvertimeSchedules(ctx context.Context, conn *sql.Conn, b core.CareerBinding, source string) ([]CareerCancelledSchedule, error) {
	rows, err := conn.QueryContext(ctx, `SELECT s.schedule_id,s.scheduler_item_id FROM agent_schedule_entries s JOIN scheduler_items q ON q.scheduler_item_id=s.scheduler_item_id WHERE s.definition_event_id=? AND s.status='active' AND q.status='pending' AND q.instance_id=? AND q.branch_id=? ORDER BY s.world_time,s.schedule_id`, source, b.InstanceID, b.BranchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []CareerCancelledSchedule
	for rows.Next() {
		var item CareerCancelledSchedule
		if err := rows.Scan(&item.ScheduleID, &item.SchedulerItemID); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func careerAcceptedOvertime(ctx context.Context, conn *sql.Conn, contract string, first, end int) ([]CareerRecord, error) {
	rows, err := conn.QueryContext(ctx, `SELECT e.event_id,e.event_sequence,e.world_time,e.payload FROM employment_contracts c JOIN events origin ON origin.event_id=c.definition_event_id JOIN events e ON e.instance_id=origin.instance_id AND e.branch_id=origin.branch_id WHERE c.contract_id=? AND e.event_type='RPCareerFactRecorded' AND json_extract(e.payload,'$.kind')='overtime' AND json_extract(e.payload,'$.overtime.contract_id')=c.contract_id AND json_extract(e.payload,'$.overtime.day')>=? AND json_extract(e.payload,'$.overtime.day')<? AND json_extract(e.payload,'$.overtime.status')='accepted' AND NOT EXISTS (SELECT 1 FROM events n WHERE n.instance_id=e.instance_id AND n.branch_id=e.branch_id AND n.event_type=e.event_type AND json_extract(n.payload,'$.kind')='overtime' AND json_extract(n.payload,'$.record_id')=json_extract(e.payload,'$.record_id') AND n.event_sequence>e.event_sequence) ORDER BY e.event_sequence`, contract, first, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []CareerRecord
	for rows.Next() {
		var record CareerRecord
		var raw string
		if err := rows.Scan(&record.EventID, &record.EventSequence, &record.WorldTime, &raw); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(raw), &record.Fact); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func deriveCareerOvertimePay(ctx context.Context, conn *sql.Conn, job CareerEmploymentFact, day int, at string) ([]CareerOvertimeEarned, int64, error) {
	records, err := careerAcceptedOvertime(ctx, conn, job.ContractID, day, day+1)
	if err != nil {
		return nil, 0, err
	}
	var evidence []CareerOvertimeEarned
	var total int64
	for _, record := range records {
		o := record.Fact.Overtime
		if o == nil || o.PayPolicy != careerOvertimePayPolicy {
			return nil, 0, core.NewError(core.CodeProjectionDiverged, "invalid overtime agreement")
		}
		terms := job
		terms.WorkplaceID = o.WorkplaceID
		terms.WorkStartHour, terms.WorkEndHour = o.StartHour, o.EndHour
		attendance, err := deriveCareerAttendance(ctx, conn, terms, o.TermsEventID, day, at)
		if err != nil {
			return nil, 0, err
		}
		var amount int64
		if attendance.RecordedSeconds > 0 {
			product, ok := checkedMultiplyPositive(attendance.RecordedSeconds, o.RateMinorPerHour)
			if !ok {
				return nil, 0, core.NewError(core.CodeIntegerOverflow, "overtime rate multiplication overflow")
			}
			amount = product / 3600
		}
		updated, ok := checkedAdd(total, amount)
		if !ok || updated > core.MaxJSONSafeInteger {
			return nil, 0, core.NewError(core.CodeIntegerOverflow, "overtime earnings overflow")
		}
		total = updated
		evidence = append(evidence, CareerOvertimeEarned{AgreementEventID: record.EventID, AmountMinor: amount, RateMinorPerHour: o.RateMinorPerHour, PayPolicy: o.PayPolicy, Attendance: attendance})
	}
	return evidence, total, nil
}
