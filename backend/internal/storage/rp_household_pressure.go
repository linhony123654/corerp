package storage

import (
	"context"
	"database/sql"
	"time"

	"corerp.local/backend/internal/core"
)

// Household pressure is a forecast, not authority to move anyone's money.
// Member IDs and wage sources stay inside this read; only aggregate amounts
// and the caller's agreed share enter the decision context.
func readRPHouseholdPressure(ctx context.Context, conn *sql.Conn, input core.RPDecisionInput) (*core.RPHouseholdPressure, error) {
	var householdID, agreementID, contractID, starts string
	var startDay, periodDays int
	var rentMinor, ownShare, rentFund int64
	err := conn.QueryRowContext(ctx, `SELECT h.household_id,a.agreement_id,a.contract_id,a.starts_world_time,a.starts_world_day,c.period_days,c.rent_minor,sh.amount_minor,b.balance_minor
		FROM rp_household_memberships m JOIN rp_households h ON h.household_id=m.household_id AND h.instance_id=m.instance_id AND h.branch_id=m.branch_id
		JOIN rp_household_rent_agreements a ON a.household_id=h.household_id AND a.instance_id=h.instance_id AND a.branch_id=h.branch_id
		JOIN rent_contracts c ON c.contract_id=a.contract_id AND c.definition_event_id=a.source_event_id
		JOIN rp_household_rent_shares sh ON sh.agreement_id=a.agreement_id AND sh.membership_id=m.membership_id
		JOIN account_balances b ON b.account_id=h.rent_account_id
		JOIN events e ON e.event_id=a.source_event_id AND e.instance_id=a.instance_id AND e.branch_id=a.branch_id AND e.event_type='RPHouseholdRentAgreed'
		WHERE m.member_entity_id=? AND m.instance_id=? AND m.branch_id=? AND m.started_world_time<=? AND (m.ended_world_time IS NULL OR m.ended_world_time>?)
		AND h.status='active' AND a.status='active' AND c.status='active' AND a.starts_world_time<=?`, input.NPCEntityID, input.InstanceID, input.BranchID, input.WorldTime, input.WorldTime, input.WorldTime).Scan(
		&householdID, &agreementID, &contractID, &starts, &startDay, &periodDays, &rentMinor, &ownShare, &rentFund)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var currentDay int
	if err := conn.QueryRowContext(ctx, `SELECT current_day FROM world_clocks WHERE instance_id=? AND branch_id=?`, input.InstanceID, input.BranchID).Scan(&currentDay); err != nil {
		return nil, err
	}
	start, err := time.Parse(time.RFC3339, starts)
	if err != nil || currentDay < startDay || periodDays < 1 || rentMinor < 1 || ownShare < 1 || rentFund < 0 {
		return nil, core.NewError(core.CodeProjectionDiverged, "invalid household budget source")
	}
	period := (currentDay-startDay)/periodDays + 1
	item, _, err := rpHouseholdRentItem(rpHouseholdRentSchedule{AgreementID: agreementID, StartTime: start, StartDay: startDay, PeriodDays: periodDays}, period, rpHouseholdRentPay)
	if err != nil {
		return nil, err
	}
	daysRemaining := startDay + period*periodDays - currentDay
	if daysRemaining < 1 || daysRemaining > periodDays {
		return nil, core.NewError(core.CodeProjectionDiverged, "invalid household budget horizon")
	}
	rows, err := conn.QueryContext(ctx, `SELECT member_entity_id FROM rp_household_memberships WHERE household_id=? AND instance_id=? AND branch_id=? AND started_world_time<=? AND (ended_world_time IS NULL OR ended_world_time>?) ORDER BY member_entity_id`, householdID, input.InstanceID, input.BranchID, input.WorldTime, input.WorldTime)
	if err != nil {
		return nil, err
	}
	var members []string
	for rows.Next() {
		var member string
		if err := rows.Scan(&member); err != nil {
			rows.Close()
			return nil, err
		}
		members = append(members, member)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	var expectedIncome int64
	for _, member := range members {
		jobs, err := readRPOwnEmployment(ctx, conn, input.InstanceID, input.BranchID, member, input.WorldTime)
		if err != nil {
			return nil, err
		}
		for _, job := range jobs {
			// Aggregate M2 participation is a daily unit rate; independent
			// employment terms set PayPeriodDays explicitly.
			payDays := job.PayPeriodDays
			if payDays == 0 {
				payDays = 1
			}
			if job.WageMinor < 0 || payDays < 1 {
				return nil, core.NewError(core.CodeProjectionDiverged, "invalid household wage forecast source")
			}
			periodIncome, ok := checkedMultiplyPositive(job.WageMinor, int64(daysRemaining))
			if !ok {
				return nil, core.NewError(core.CodeIntegerOverflow, "household wage forecast overflows")
			}
			periodIncome /= int64(payDays)
			expectedIncome, ok = checkedAdd(expectedIncome, periodIncome)
			if !ok {
				return nil, core.NewError(core.CodeIntegerOverflow, "household income forecast overflows")
			}
		}
	}
	var outstanding int64
	if err := conn.QueryRowContext(ctx, `SELECT COALESCE(SUM(amount_due_minor-amount_paid_minor),0) FROM rent_obligations WHERE contract_id=? AND amount_due_minor>amount_paid_minor`, contractID).Scan(&outstanding); err != nil {
		return nil, err
	}
	required, ok := checkedAdd(rentMinor, outstanding)
	if !ok || outstanding < 0 {
		return nil, core.NewError(core.CodeIntegerOverflow, "household rent requirement overflows")
	}
	available, ok := checkedAdd(rentFund, expectedIncome)
	if !ok {
		return nil, core.NewError(core.CodeIntegerOverflow, "household rent coverage overflows")
	}
	gap := int64(0)
	level := "covered"
	if available < required {
		gap = required - available
		level = "at_risk"
	}
	return &core.RPHouseholdPressure{RentMinor: rentMinor, OwnShareMinor: ownShare, RentFundMinor: rentFund,
		ExpectedIncomeMinor: expectedIncome, OutstandingMinor: outstanding, CoverageGapMinor: gap, PressureLevel: level,
		NextDueWorldTime: item.WorldTime}, nil
}
