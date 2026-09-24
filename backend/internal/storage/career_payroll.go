package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"corerp.local/backend/internal/core"
)

const careerPayrollPhase = "career_daily_payroll"

func queueCareerPayrollItem(ctx context.Context, conn *sql.Conn, subject string, day, minute int, kind string) error {
	// Follow immutable ownership instead of assigning every future payroll to M2.
	var instanceID, branchID string
	var scopeQuery string
	switch kind {
	case "career_terms_effective":
		scopeQuery = `SELECT instance_id,branch_id FROM events WHERE event_id=? AND event_type='RPCareerFactRecorded' AND json_extract(payload,'$.employment.contract_id') IS NOT NULL`
	case "career_wage_accrue", "career_wage_pay":
		scopeQuery = `SELECT e.instance_id,e.branch_id FROM employment_contracts c JOIN events e ON e.event_id=c.definition_event_id WHERE c.contract_id=?`
	case "career_wage_retry":
		scopeQuery = `SELECT e.instance_id,e.branch_id FROM wage_obligations w JOIN employment_contracts c ON c.contract_id=w.contract_id JOIN events e ON e.event_id=c.definition_event_id WHERE w.obligation_id=?`
	default:
		return core.NewError(core.CodeInvalidArgument, "unsupported career payroll queue kind")
	}
	if err := conn.QueryRowContext(ctx, scopeQuery, subject).Scan(&instanceID, &branchID); err != nil {
		return classifyMissing(err, "career queue source")
	}
	payload, err := core.CanonicalJSON(scheduledPayload{Kind: kind, Day: day, SubjectID: subject})
	if err != nil {
		return err
	}
	id := fmt.Sprintf("%s_%s_%d", kind, subject, day)
	_, err = conn.ExecContext(ctx, `INSERT INTO scheduler_items(scheduler_item_id,instance_id,branch_id,world_time,phase_id,declared_priority,status,payload) VALUES (?,?,?,?,?,0,'pending',?)`, id, instanceID, branchID, careerTime(day, 0, minute), careerPayrollPhase, string(payload))
	return err
}

func queueCareerPayroll(ctx context.Context, conn *sql.Conn, contractID string, day int) error {
	if err := queueCareerPayrollItem(ctx, conn, contractID, day, 0, "career_wage_accrue"); err != nil {
		return err
	}
	return queueCareerPayrollItem(ctx, conn, contractID, day, 1, "career_wage_pay")
}

func (s *Store) executeCareerPayroll(ctx context.Context, tx *immediateTx, item SchedulerItem) error {
	instanceID, branchID, err := recordedSchedulerScope(ctx, tx.conn, item)
	if err != nil {
		return err
	}
	var payload scheduledPayload
	if err := json.Unmarshal([]byte(item.Payload), &payload); err != nil {
		return err
	}
	if payload.Kind == "career_terms_effective" {
		return s.executeCareerTermActivation(ctx, tx, item, payload)
	}
	if payload.Kind == "career_aggregate_exit" {
		return s.executeCareerAggregateExit(ctx, tx, item, payload)
	}
	minute := 0
	switch payload.Kind {
	case "career_wage_accrue":
	case "career_wage_pay":
		minute = 1
	case "career_wage_retry":
		minute = 2
	default:
		return core.NewError(core.CodeProjectionDiverged, "invalid career payroll kind")
	}
	if payload.Day < 1 || item.WorldTime != careerTime(payload.Day, 0, minute) {
		return core.NewError(core.CodeProjectionDiverged, "career payroll time differs from period")
	}
	contractID := payload.SubjectID
	obligationID := fmt.Sprintf("wage_%s_%d_%d", contractID, payload.Day-1, payload.Day)
	if payload.Kind == "career_wage_retry" {
		obligationID = payload.SubjectID
		if err := tx.conn.QueryRowContext(ctx, `SELECT contract_id FROM wage_obligations WHERE obligation_id=?`, obligationID).Scan(&contractID); err != nil {
			return classifyMissing(err, "career wage retry obligation")
		}
	}
	var status, employerCash, employeeCash, currency string
	var starts int
	if err := tx.conn.QueryRowContext(ctx, `SELECT c.status,c.starts_on_day,c.employer_account_id,c.employee_account_id,c.currency_id FROM employment_contracts c JOIN events e ON e.event_id=c.definition_event_id WHERE c.contract_id=? AND e.instance_id=? AND e.branch_id=? AND json_extract(e.payload,'$.employment.contract_id')=c.contract_id`, contractID, instanceID, branchID).Scan(&status, &starts, &employerCash, &employeeCash, &currency); err != nil {
		return classifyMissing(err, "scoped career payroll contract")
	}
	ledger, err := readObligationLedger(ctx, tx.conn, "wage", contractID)
	if err != nil {
		return err
	}
	for _, id := range []string{employerCash, employeeCash, ledger.ExpenseAccountID, ledger.PayableAccountID, ledger.ReceivableAccountID, ledger.IncomeAccountID} {
		if err := verifyScopedAccountProjection(ctx, tx.conn, instanceID, branchID, id, currency); err != nil {
			return err
		}
	}
	var mutation scheduledMutation
	mayAccrue := status == "active"
	if payload.Kind == "career_wage_accrue" && status == "ended" {
		ended, _, err := readCareerEmploymentTerms(ctx, tx.conn, contractID, payload.Day)
		if err != nil {
			return err
		}
		mayAccrue = ended.EndsOnDay >= payload.Day && payload.Day > starts
	}
	if payload.Kind == "career_wage_accrue" && mayAccrue {
		if payload.Day <= starts {
			return core.NewError(core.CodeProjectionDiverged, "cannot accrue unearned career period")
		}
		job, source, err := readCareerEmploymentTerms(ctx, tx.conn, contractID, payload.Day-1)
		if err != nil {
			return err
		}
		overtime, extra, err := deriveCareerOvertimePay(ctx, tx.conn, job, payload.Day-1, item.WorldTime)
		if err != nil {
			return err
		}
		amount, ok := checkedAdd(job.DailyWageMinor, extra)
		if !ok || amount > core.MaxJSONSafeInteger {
			return core.NewError(core.CodeIntegerOverflow, "daily career pay overflow")
		}
		mutation, err = prepareWageAccrualForPeriod(ctx, tx.conn, wageAccrualPeriod{ContractID: contractID, StartDay: int64(payload.Day - 1), EndDay: payload.Day, DueWorldTime: careerTime(payload.Day, 0, 1), AmountMinor: amount})
		if err != nil {
			return err
		}
		if mutation.EventType == "WageObligationAccrued" {
			attendance, err := deriveCareerAttendance(ctx, tx.conn, job, source, payload.Day-1, item.WorldTime)
			if err != nil {
				return err
			}
			// Yesterday's earned wage and today's shift can belong to different
			// effective terms. Never apply yesterday's workplace to a new day.
			nextJob, nextSource, err := readCareerEmploymentTerms(ctx, tx.conn, contractID, payload.Day)
			if err != nil {
				return err
			}
			compatible, err := careerWorkdayCompatible(ctx, tx.conn, nextJob, payload.Day)
			if err != nil {
				return err
			}
			workStatus := "scheduled"
			if !compatible {
				workStatus = "conflict" // Preserve commitments; do not halt payroll or teleport.
			}
			leave, err := careerApprovedLeave(ctx, tx.conn, contractID, payload.Day)
			if err != nil {
				return err
			}
			if leave != "" {
				workStatus = "approved_leave"
			}
			continuing := nextJob.EndsOnDay == 0 || payload.Day < nextJob.EndsOnDay
			if !continuing {
				workStatus = "employment_ended"
			}
			mutation.EventPayload = struct {
				ObligationID        string                 `json:"obligation_id"`
				ContractID          string                 `json:"contract_id"`
				PeriodStart         int                    `json:"period_start_day"`
				PeriodEnd           int                    `json:"period_end_day"`
				AmountMinor         int64                  `json:"amount_minor"`
				TermsEventID        string                 `json:"terms_event_id"`
				NextTermsEventID    string                 `json:"next_terms_event_id"`
				NextWorkStatus      string                 `json:"next_work_status"`
				Attendance          CareerAttendance       `json:"attendance"`
				BaseAmountMinor     int64                  `json:"base_amount_minor"`
				OvertimeAmountMinor int64                  `json:"overtime_amount_minor"`
				Overtime            []CareerOvertimeEarned `json:"overtime,omitempty"`
			}{obligationID, contractID, payload.Day - 1, payload.Day, amount, source, nextSource, workStatus, attendance, job.DailyWageMinor, extra, overtime}
			accrue := mutation.ApplyDomainRows
			mutation.ApplyDomainRows = func(ctx context.Context, conn *sql.Conn, eventID string, seq int64) error {
				if err := accrue(ctx, conn, eventID, seq); err != nil {
					return err
				}
				if continuing && compatible && leave == "" {
					if err := queueCareerWorkday(ctx, conn, nextJob, payload.Day, eventID); err != nil {
						return err
					}
				}
				if continuing {
					return queueCareerPayroll(ctx, conn, contractID, payload.Day+1)
				}
				return nil
			}
		}
	} else if payload.Kind == "career_wage_accrue" {
		mutation = careerPayrollSkip(contractID, "employment_ended")
	} else {
		var exists int
		if err := tx.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM wage_obligations WHERE obligation_id=?`, obligationID).Scan(&exists); err != nil {
			return err
		}
		if exists == 0 && status == "ended" {
			mutation = careerPayrollSkip(contractID, "no_earned_obligation")
		} else {
			mutation, err = prepareWageSettlement(ctx, tx.conn, payload, obligationID)
			if err != nil {
				return err
			}
			if mutation.EventType == "WageArrearsRecorded" {
				settle := mutation.ApplyDomainRows
				mutation.ApplyDomainRows = func(ctx context.Context, conn *sql.Conn, eventID string, seq int64) error {
					if err := settle(ctx, conn, eventID, seq); err != nil {
						return err
					}
					return queueCareerPayrollItem(ctx, conn, obligationID, payload.Day+1, 2, "career_wage_retry")
				}
			}
		}
	}
	mutation.Private = true
	if err := s.commitScheduledMutationForBranch(ctx, tx, item, payload, mutation, instanceID, branchID); err != nil {
		return err
	}
	if s.beforeCommit != nil {
		return s.beforeCommit()
	}
	return nil
}

func careerPayrollSkip(contractID, reason string) scheduledMutation {
	return scheduledMutation{Private: true, EventType: "SchedulerSkipLogged", EventPayload: struct {
		ContractID string `json:"contract_id"`
		Reason     string `json:"reason"`
	}{contractID, reason}}
}
