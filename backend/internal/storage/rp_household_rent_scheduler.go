package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"corerp.local/backend/internal/core"
)

const rpHouseholdRentPhase = "rp_household_rent"

const (
	rpHouseholdRentAccrue  = "rp_household_rent_accrue"
	rpHouseholdRentPay     = "rp_household_rent_pay"
	rpHouseholdRentPastDue = "rp_household_rent_past_due"
)

type rpHouseholdRentSchedule struct {
	AgreementID  string
	ContractID   string
	InstanceID   string
	BranchID     string
	SourceEvent  string
	StartTime    time.Time
	StartDay     int
	PeriodDays   int
	GraceDays    int
	RentMinor    int64
	CurrencyID   string
	RentAccount  string
	LandlordCash string
}

func loadRPHouseholdRentSchedule(ctx context.Context, conn *sql.Conn, agreementID string) (rpHouseholdRentSchedule, error) {
	var row rpHouseholdRentSchedule
	var starts string
	err := conn.QueryRowContext(ctx, `SELECT a.agreement_id,a.contract_id,a.instance_id,a.branch_id,a.source_event_id,a.starts_world_time,a.starts_world_day,
		c.period_days,c.grace_days,c.rent_minor,c.currency_id,c.tenant_account_id,c.landlord_account_id
		FROM rp_household_rent_agreements a JOIN rent_contracts c ON c.contract_id=a.contract_id
		JOIN events e ON e.event_id=a.source_event_id AND e.instance_id=a.instance_id AND e.branch_id=a.branch_id
		WHERE a.agreement_id=? AND a.status='active' AND c.status='active' AND c.definition_event_id=a.source_event_id
		AND e.event_type='RPHouseholdRentAgreed'`, agreementID).Scan(
		&row.AgreementID, &row.ContractID, &row.InstanceID, &row.BranchID, &row.SourceEvent,
		&starts, &row.StartDay, &row.PeriodDays, &row.GraceDays, &row.RentMinor, &row.CurrencyID,
		&row.RentAccount, &row.LandlordCash)
	if err != nil {
		return row, classifyMissing(err, "active sourced household rent agreement")
	}
	row.StartTime, err = time.Parse(time.RFC3339, starts)
	if err != nil || row.StartDay < 0 || row.PeriodDays < 1 || row.GraceDays < 0 || row.RentMinor < 1 {
		return rpHouseholdRentSchedule{}, core.NewError(core.CodeProjectionDiverged, "invalid household rent period source")
	}
	return row, nil
}

func rpHouseholdRentItem(row rpHouseholdRentSchedule, period int, kind string) (SchedulerItem, scheduledPayload, error) {
	if period < 1 || period > 100000 || row.PeriodDays < 1 || row.StartDay < 0 || row.GraceDays < 0 {
		return SchedulerItem{}, scheduledPayload{}, core.NewError(core.CodeInvalidArgument, "invalid household rent period")
	}
	days := period * row.PeriodDays
	if days/row.PeriodDays != period || row.StartDay > 100000000-days-row.GraceDays {
		return SchedulerItem{}, scheduledPayload{}, core.NewError(core.CodeIntegerOverflow, "household rent period exceeds supported world day")
	}
	at := row.StartTime.AddDate(0, 0, days)
	day, priority := row.StartDay+days, int64(0)
	switch kind {
	case rpHouseholdRentAccrue:
	case rpHouseholdRentPay:
		priority = 1
	case rpHouseholdRentPastDue:
		at = at.AddDate(0, 0, row.GraceDays)
		day += row.GraceDays
		priority = 2
	default:
		return SchedulerItem{}, scheduledPayload{}, core.NewError(core.CodeInvalidArgument, "unsupported household rent schedule kind")
	}
	if at.Year() > 9999 || at.Before(row.StartTime) {
		return SchedulerItem{}, scheduledPayload{}, core.NewError(core.CodeIntegerOverflow, "household rent due time exceeds supported world")
	}
	payload := scheduledPayload{Kind: kind, Day: day, SubjectID: row.AgreementID}
	encoded, err := core.CanonicalJSON(payload)
	if err != nil {
		return SchedulerItem{}, scheduledPayload{}, err
	}
	item := SchedulerItem{
		SchedulerItemID: fmt.Sprintf("sched_rp_household_rent_%s_%d_%s", row.AgreementID, period, kind),
		WorldTime:       at.UTC().Format(time.RFC3339), PhaseID: rpHouseholdRentPhase,
		DeclaredPriority: priority, Status: "pending", Payload: string(encoded),
	}
	return item, payload, nil
}

func queueRPHouseholdRentPeriod(ctx context.Context, conn *sql.Conn, agreementID string, period int) error {
	row, err := loadRPHouseholdRentSchedule(ctx, conn, agreementID)
	if err != nil {
		return err
	}
	for _, kind := range []string{rpHouseholdRentAccrue, rpHouseholdRentPay, rpHouseholdRentPastDue} {
		item, _, err := rpHouseholdRentItem(row, period, kind)
		if err != nil {
			return err
		}
		if err := execAgentOne(ctx, conn, "queue household rent", `INSERT INTO scheduler_items(scheduler_item_id,instance_id,branch_id,world_time,phase_id,declared_priority,status,payload) VALUES (?,?,?,?,?,?,'pending',?)`, item.SchedulerItemID, row.InstanceID, row.BranchID, item.WorldTime, item.PhaseID, item.DeclaredPriority, item.Payload); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) executeRPHouseholdRent(ctx context.Context, tx *immediateTx, item SchedulerItem) error {
	instanceID, branchID, err := recordedSchedulerScope(ctx, tx.conn, item)
	if err != nil {
		return err
	}
	if item.PhaseID != rpHouseholdRentPhase || instanceID != M2DemoInstanceID || branchID != M2DemoBranchID {
		return core.NewError(core.CodeProjectionDiverged, "household rent scheduler scope differs")
	}
	var payload scheduledPayload
	if err := json.Unmarshal([]byte(item.Payload), &payload); err != nil {
		return err
	}
	row, err := loadRPHouseholdRentSchedule(ctx, tx.conn, payload.SubjectID)
	if err != nil {
		return err
	}
	if row.InstanceID != instanceID || row.BranchID != branchID {
		return core.NewError(core.CodeProjectionDiverged, "household rent agreement belongs to another branch")
	}
	period, remainder := 0, 0
	switch payload.Kind {
	case rpHouseholdRentAccrue, rpHouseholdRentPay:
		period, remainder = (payload.Day-row.StartDay)/row.PeriodDays, (payload.Day-row.StartDay)%row.PeriodDays
	case rpHouseholdRentPastDue:
		period, remainder = (payload.Day-row.StartDay-row.GraceDays)/row.PeriodDays, (payload.Day-row.StartDay-row.GraceDays)%row.PeriodDays
	default:
		return core.NewError(core.CodeProjectionDiverged, "invalid household rent scheduled kind")
	}
	if remainder != 0 {
		return core.NewError(core.CodeProjectionDiverged, "household rent scheduler day differs from source")
	}
	expected, _, err := rpHouseholdRentItem(row, period, payload.Kind)
	if err != nil {
		return err
	}
	if item != expected {
		return core.NewError(core.CodeProjectionDiverged, "household rent scheduled item differs from sourced period")
	}
	ledger, err := readObligationLedger(ctx, tx.conn, "rent", row.ContractID)
	if err != nil {
		return err
	}
	if ledger.CurrencyID != row.CurrencyID {
		return core.NewError(core.CodeProjectionDiverged, "household rent ledger currency differs")
	}
	for _, id := range []string{row.RentAccount, row.LandlordCash, ledger.ExpenseAccountID, ledger.PayableAccountID, ledger.ReceivableAccountID, ledger.IncomeAccountID} {
		if err := verifyScopedAccountProjection(ctx, tx.conn, instanceID, branchID, id, row.CurrencyID); err != nil {
			return err
		}
	}
	accounting := scheduledPayload{Kind: payload.Kind, Day: row.StartDay + period*row.PeriodDays, SubjectID: row.ContractID}
	var mutation scheduledMutation
	switch payload.Kind {
	case rpHouseholdRentAccrue:
		mutation, err = prepareRPHouseholdRentAccrual(ctx, tx.conn, row, period, expected.WorldTime, ledger)
	case rpHouseholdRentPay:
		mutation, err = prepareRentPayment(ctx, tx.conn, accounting)
		if err == nil {
			if mutation.EventType == "RentPaid" {
				mutation.EventType = "RPHouseholdRentPaid"
			} else {
				mutation.EventType = "RPHouseholdRentPaymentFailed"
			}
		}
	case rpHouseholdRentPastDue:
		accounting.Day += row.GraceDays
		mutation, err = prepareRentPastDue(ctx, tx.conn, accounting)
		if err == nil {
			if mutation.EventType == "RentPastDue" {
				mutation.EventType = "RPHouseholdRentPastDue"
			} else {
				mutation.EventType = "RPHouseholdRentPastDueSkipped"
			}
		}
	}
	if err != nil {
		return err
	}
	if payload.Kind != rpHouseholdRentAccrue {
		original := mutation.EventPayload
		mutation.EventPayload = struct {
			AgreementID string `json:"agreement_id"`
			PeriodIndex int    `json:"period_index"`
			Settlement  any    `json:"settlement"`
		}{row.AgreementID, period, original}
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

func prepareRPHouseholdRentAccrual(ctx context.Context, conn *sql.Conn, row rpHouseholdRentSchedule, period int, due string, ledger obligationLedger) (scheduledMutation, error) {
	startDay, endDay := row.StartDay+(period-1)*row.PeriodDays, row.StartDay+period*row.PeriodDays
	obligationID := fmt.Sprintf("rent_%s_%d_%d", row.ContractID, startDay, endDay)
	var existing int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM rent_obligations WHERE obligation_id=?`, obligationID).Scan(&existing); err != nil {
		return scheduledMutation{}, err
	}
	if existing != 0 {
		return scheduledMutation{}, core.NewError(core.CodeProjectionDiverged, "household rent obligation already accrued before its scheduled Event")
	}
	postings, balances, err := prepareAccrualAccounting(ctx, conn, ledger, row.RentMinor, "household rent accrual")
	if err != nil {
		return scheduledMutation{}, err
	}
	graceAt, err := time.Parse(time.RFC3339, due)
	if err != nil {
		return scheduledMutation{}, err
	}
	grace := graceAt.AddDate(0, 0, row.GraceDays).UTC().Format(time.RFC3339)
	return scheduledMutation{
		EventType: "RPHouseholdRentAccrued", Postings: postings, Balances: balances,
		EventPayload: struct {
			AgreementID  string `json:"agreement_id"`
			ContractID   string `json:"contract_id"`
			ObligationID string `json:"obligation_id"`
			PeriodIndex  int    `json:"period_index"`
			StartDay     int    `json:"period_start_day"`
			EndDay       int    `json:"period_end_day"`
			DueWorldTime string `json:"due_world_time"`
			GraceUntil   string `json:"grace_until_world_time"`
			AmountMinor  int64  `json:"amount_minor"`
		}{row.AgreementID, row.ContractID, obligationID, period, startDay, endDay, due, grace, row.RentMinor},
		ApplyDomainRows: func(ctx context.Context, conn *sql.Conn, _ string, sequence int64) error {
			if err := execAgentOne(ctx, conn, "accrue household rent obligation", `INSERT INTO rent_obligations(obligation_id,contract_id,period_start_day,period_end_day,due_world_time,grace_until_world_time,amount_due_minor,amount_paid_minor,status,last_event_sequence) VALUES (?,?,?,?,?,?,?,0,'due',?)`, obligationID, row.ContractID, startDay, endDay, due, grace, row.RentMinor, sequence); err != nil {
				return err
			}
			return queueRPHouseholdRentPeriod(ctx, conn, row.AgreementID, period+1)
		},
	}, nil
}
