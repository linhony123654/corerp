package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"corerp.local/backend/internal/core"
)

func enrichCareerOwnEmployment(ctx context.Context, conn *sql.Conn, entityID, worldTime string, jobs []core.RPOwnEmployment) error {
	at, err := time.Parse(time.RFC3339, worldTime)
	if err != nil {
		return err
	}
	base := time.Date(2026, time.September, 22, 0, 0, 0, 0, time.UTC)
	day := int(at.Sub(base) / (24 * time.Hour))
	for i := range jobs {
		var raw string
		err := conn.QueryRowContext(ctx, `SELECT payload FROM events WHERE event_id=? AND event_type='RPCareerFactRecorded' AND json_extract(payload,'$.employment.employee_id')=?`, jobs[i].SourceEventID, entityID).Scan(&raw)
		if err == sql.ErrNoRows {
			continue
		}
		if err != nil {
			return err
		}
		var fact CareerFact
		if err := json.Unmarshal([]byte(raw), &fact); err != nil || fact.Employment == nil {
			return core.NewError(core.CodeProjectionDiverged, "invalid own career definition")
		}
		job := *fact.Employment
		status := "onboarding"
		if day >= job.StartsOnDay {
			var source string
			job, source, err = readCareerEmploymentTerms(ctx, conn, job.ContractID, day)
			if err != nil {
				return err
			}
			jobs[i].SourceEventID = source
			status = job.LifecycleStatus
		}
		jobs[i].PositionID, jobs[i].OccupationID, jobs[i].Grade = job.PositionID, job.OccupationID, job.Grade
		jobs[i].Status, jobs[i].WorkplaceID = status, job.WorkplaceID
		jobs[i].StartsOnDay, jobs[i].PayPeriodDays, jobs[i].WageMinor = job.StartsOnDay, 1, job.DailyWageMinor
		if status != "onboarding" {
			jobs[i].PositionCapabilities = append([]string(nil), job.Capabilities...)
		}
	}
	return nil
}

// Only own accepted employment and earned claims. Internal evaluation/advisory
// records are deliberately not selected even when they name this candidate.
func appendCareerLifeEvidence(ctx context.Context, conn *sql.Conn, input core.RPDecisionInput, life *core.RPLifeContext) error {
	rows, err := conn.QueryContext(ctx, `SELECT b.balance_minor,(SELECT e.event_id FROM postings p JOIN journal_entries j ON j.entry_id=p.entry_id AND j.status='posted' JOIN events e ON e.event_id=j.event_id WHERE p.account_id=l.receivable_account_id AND e.instance_id=definition.instance_id AND e.branch_id=definition.branch_id ORDER BY e.event_sequence DESC LIMIT 1) FROM employment_contracts c JOIN events definition ON definition.event_id=c.definition_event_id JOIN obligation_ledger_accounts l ON l.obligation_kind='wage' AND l.contract_id=c.contract_id JOIN account_balances b ON b.account_id=l.receivable_account_id WHERE c.employee_entity_id=? AND definition.instance_id=? AND definition.branch_id=? AND json_extract(definition.payload,'$.employment.contract_id')=c.contract_id ORDER BY c.contract_id`, input.NPCEntityID, input.InstanceID, input.BranchID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var amount int64
		var source sql.NullString
		if err := rows.Scan(&amount, &source); err != nil {
			rows.Close()
			return err
		}
		updated, ok := checkedAdd(life.ReceivableMinor, amount)
		if !ok || amount < 0 {
			rows.Close()
			return core.NewError(core.CodeProjectionDiverged, "invalid own wage receivable")
		}
		life.ReceivableMinor = updated
		if source.Valid {
			found := false
			for _, id := range life.EconomicSourceEventIDs {
				found = found || id == source.String
			}
			if !found {
				life.EconomicSourceEventIDs = append(life.EconomicSourceEventIDs, source.String)
			}
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	if err := appendCareerAttendanceMemories(ctx, conn, input, life); err != nil {
		return err
	}
	if err := appendCareerEmploymentChangeMemories(ctx, conn, input, life); err != nil {
		return err
	}
	if err := appendCareerLeaveMemories(ctx, conn, input, life); err != nil {
		return err
	}
	if err := appendCareerOvertimeMemories(ctx, conn, input, life); err != nil {
		return err
	}
	if err := readCareerUnemployment(ctx, conn, input, life); err != nil {
		return err
	}
	if err := appendCareerAggregateExitMemories(ctx, conn, input, life); err != nil {
		return err
	}
	// Filter the event type before inspecting its JSON. The command join alone
	// does not prevent SQLite from parsing unrelated, large RP decision payloads
	// on every character-context read as world history grows.
	rows, err = conn.QueryContext(ctx, `SELECT e.event_id,e.world_time,json_extract(e.payload,'$.employment.organization_id') FROM events e JOIN event_batches b ON b.batch_id=e.batch_id JOIN commands c ON c.command_id=b.command_id WHERE e.instance_id=? AND e.branch_id=? AND e.event_type='RPCareerFactRecorded' AND c.command_type='AcceptCareerOffer' AND json_extract(e.payload,'$.employment.employee_id')=? ORDER BY e.event_sequence DESC LIMIT 5`, input.InstanceID, input.BranchID, input.NPCEntityID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		memory := core.RPLifeMemory{Kind: "own_employment_accepted", SubjectEntityID: input.NPCEntityID}
		var org string
		if err := rows.Scan(&memory.SourceEventID, &memory.WorldTime, &org); err != nil {
			return err
		}
		memory.Text = "Accepted employment with " + org
		life.SalientMemories = append(life.SalientMemories, memory)
	}
	return rows.Err()
}

func appendCareerAttendanceMemories(ctx context.Context, conn *sql.Conn, input core.RPDecisionInput, life *core.RPLifeContext) error {
	rows, err := conn.QueryContext(ctx, `SELECT event_id,world_time,json_extract(payload,'$.attendance'),COALESCE(json_extract(payload,'$.overtime_amount_minor'),0) FROM events WHERE instance_id=? AND branch_id=? AND event_type='WageObligationAccrued' AND json_extract(payload,'$.attendance.employee_id')=? ORDER BY event_sequence DESC LIMIT 5`, input.InstanceID, input.BranchID, input.NPCEntityID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		memory := core.RPLifeMemory{Kind: "own_work_attendance", SubjectEntityID: input.NPCEntityID}
		var raw string
		var overtime int64
		if err := rows.Scan(&memory.SourceEventID, &memory.WorldTime, &raw, &overtime); err != nil {
			return err
		}
		var attendance CareerAttendance
		if err := json.Unmarshal([]byte(raw), &attendance); err != nil {
			return err
		}
		memory.Text = fmt.Sprintf("Recorded work on day %d: %s, %d of %d seconds; this is not a performance evaluation.", attendance.Day, attendance.Status, attendance.RecordedSeconds, attendance.ExpectedSeconds)
		if overtime > 0 {
			memory.Text += fmt.Sprintf(" Agreed overtime earned %d minor units, which may still be unpaid.", overtime)
		}
		life.SalientMemories = append(life.SalientMemories, memory)
	}
	return rows.Err()
}

func appendCareerEmploymentChangeMemories(ctx context.Context, conn *sql.Conn, input core.RPDecisionInput, life *core.RPLifeContext) error {
	rows, err := conn.QueryContext(ctx, `SELECT event_id,world_time,json_extract(payload,'$.employment_change.notice'),json_extract(payload,'$.employment_change.kind'),json_extract(payload,'$.employment.effective_from_day'),json_extract(payload,'$.employment.daily_wage_minor') FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPCareerFactRecorded' AND json_extract(payload,'$.employment.employee_id')=? AND json_extract(payload,'$.employment_change.kind') IN ('regularized','raise','promotion','transfer','demotion','resignation','termination','layoff') ORDER BY event_sequence DESC LIMIT 5`, input.InstanceID, input.BranchID, input.NPCEntityID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		memory := core.RPLifeMemory{Kind: "own_employment_regularized", SubjectEntityID: input.NPCEntityID}
		var kind string
		var day int
		var amount int64
		if err := rows.Scan(&memory.SourceEventID, &memory.WorldTime, &memory.Text, &kind, &day, &amount); err != nil {
			return err
		}
		if kind == "raise" {
			memory.Kind = "own_raise_announced"
			memory.Text = fmt.Sprintf("Daily base wage becomes %d minor units from day %d; this is not cash already received. Manager notice: %s", amount, day, memory.Text)
		} else if kind == "resignation" || kind == "termination" || kind == "layoff" {
			memory.Kind = "own_employment_exit_notice"
			memory.Text = fmt.Sprintf("Notice of %s: employment ends from day %d; earned wages remain owed. Notice: %s", kind, day, memory.Text)
		} else if kind != "regularized" {
			memory.Kind = "own_position_change_agreed"
			memory.Text = fmt.Sprintf("Agreed %s with daily base wage %d minor units from day %d; acceptance is not cash received or early activation. Manager notice: %s", kind, amount, day, memory.Text)
		}
		life.SalientMemories = append(life.SalientMemories, memory)
	}
	return rows.Err()
}

func appendCareerLeaveMemories(ctx context.Context, conn *sql.Conn, input core.RPDecisionInput, life *core.RPLifeContext) error {
	rows, err := conn.QueryContext(ctx, `SELECT event_id,world_time,json_extract(payload,'$.leave.status'),json_extract(payload,'$.leave.start_day'),json_extract(payload,'$.leave.end_day'),json_extract(payload,'$.leave.notice') FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPCareerFactRecorded' AND json_extract(payload,'$.kind')='leave' AND json_extract(payload,'$.candidate_id')=? AND json_extract(payload,'$.leave.status') IN ('approved','rejected') ORDER BY event_sequence DESC LIMIT 5`, input.InstanceID, input.BranchID, input.NPCEntityID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		memory := core.RPLifeMemory{Kind: "own_leave_decision", SubjectEntityID: input.NPCEntityID}
		var status, notice string
		var first, end int
		if err := rows.Scan(&memory.SourceEventID, &memory.WorldTime, &status, &first, &end, &notice); err != nil {
			return err
		}
		memory.Text = fmt.Sprintf("Leave %s for days %d to %d (end exclusive): %s", status, first, end, notice)
		life.SalientMemories = append(life.SalientMemories, memory)
	}
	return rows.Err()
}

func appendCareerOvertimeMemories(ctx context.Context, conn *sql.Conn, input core.RPDecisionInput, life *core.RPLifeContext) error {
	rows, err := conn.QueryContext(ctx, `SELECT event_id,world_time,json_extract(payload,'$.overtime') FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPCareerFactRecorded' AND json_extract(payload,'$.kind')='overtime' AND json_extract(payload,'$.candidate_id')=? AND json_extract(payload,'$.overtime.status')<>'offered' ORDER BY event_sequence DESC LIMIT 5`, input.InstanceID, input.BranchID, input.NPCEntityID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		memory := core.RPLifeMemory{Kind: "own_overtime_decision", SubjectEntityID: input.NPCEntityID}
		var raw string
		if err := rows.Scan(&memory.SourceEventID, &memory.WorldTime, &raw); err != nil {
			return err
		}
		var o CareerOvertimeFact
		if err := json.Unmarshal([]byte(raw), &o); err != nil {
			return err
		}
		memory.Text = fmt.Sprintf("Overtime %s on day %d from %02d:00 to %02d:00 at %d minor units/hour, paid only for recorded work with whole-minor-unit rounding down.", o.Status, o.Day, o.StartHour, o.EndHour, o.RateMinorPerHour)
		life.SalientMemories = append(life.SalientMemories, memory)
	}
	return rows.Err()
}
