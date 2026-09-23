package storage

import (
	"context"
	"database/sql"
	"fmt"

	"corerp.local/backend/internal/core"
)

func prepareM2Rent(ctx context.Context, conn *sql.Conn, item SchedulerItem, payload scheduledPayload) (scheduledMutation, error) {
	if payload.Day < 1 || payload.Day > 30 || payload.SubjectID != m2EconomyRentContract ||
		(item.PhaseID == m2EconomyRentPhaseAccrue && (payload.Kind != "m2_rent_accrue" || item.WorldTime != m2WageTime(payload.Day, 7, 2))) ||
		(item.PhaseID == m2EconomyRentPhasePay && (payload.Kind != "m2_rent_pay" || item.WorldTime != m2WageTime(payload.Day, 7, 3))) {
		return scheduledMutation{}, core.NewError(core.CodeStorageFailure, "invalid M2 rent task")
	}
	obligationID := fmt.Sprintf("obligation_m2_rent_day_%d", payload.Day)
	var units, rate int64
	var currency, landlordCash string
	if err := conn.QueryRowContext(ctx, `SELECT c.participant_count, c.unit_rate_minor, c.currency_id, a.cash_account_id FROM m2_cohort_contracts c JOIN m2_economic_actors a ON a.actor_id = c.actor_id JOIN cohorts h ON h.cohort_id = c.cohort_id WHERE c.contract_id = ? AND c.kind = 'rent' AND c.cohort_id = ? AND a.instance_id = ? AND a.branch_id = ? AND h.instance_id = a.instance_id AND h.branch_id = a.branch_id AND a.kind = 'landlord' AND h.status = 'active' AND h.population_count > 0 AND c.currency_id = h.currency_id`, payload.SubjectID, M2DemoCohortID, M2DemoInstanceID, M2DemoBranchID).Scan(&units, &rate, &currency, &landlordCash); err != nil {
		return scheduledMutation{}, classifyMissing(err, "M2 rent contract")
	}
	for _, accountID := range []string{M2DemoCohortAssetAccountID, landlordCash, m2EconomyCohortRentCost, m2EconomyCohortRentDue, m2EconomyLandlordRentDue, m2EconomyLandlordIncome} {
		if err := verifyM2AccountProjection(ctx, conn, accountID, currency); err != nil {
			return scheduledMutation{}, err
		}
	}
	amount, ok := checkedMultiplyPositive(units, rate)
	if !ok {
		return scheduledMutation{}, core.NewError(core.CodeIntegerOverflow, "M2 rent exceeds integer range")
	}
	ledger := obligationLedger{m2EconomyCohortRentCost, m2EconomyCohortRentDue, m2EconomyLandlordRentDue, m2EconomyLandlordIncome, currency}
	if item.PhaseID == m2EconomyRentPhaseAccrue {
		postings, balances, err := prepareAccrualAccounting(ctx, conn, ledger, amount, "M2 household rent accrual")
		if err != nil {
			return scheduledMutation{}, err
		}
		return scheduledMutation{EventType: "M2RentAccrued", EventPayload: struct {
			ContractID string `json:"contract_id"`
			Amount     int64  `json:"amount_minor"`
		}{payload.SubjectID, amount}, Postings: postings, Balances: balances,
			ApplyDomainRows: func(ctx context.Context, conn *sql.Conn, eventID string, sequence int64) error {
				return execAgentOne(ctx, conn, "record M2 rent obligation", `INSERT INTO m2_economic_obligations(obligation_id, contract_id, period_start, period_end, kind, amount_due_minor, status, defining_event_id, last_event_sequence) VALUES (?, ?, ?, ?, 'rent', ?, 'accrued', ?, ?)`, obligationID, payload.SubjectID, m2WageTime(payload.Day-1, 7, 2), item.WorldTime, amount, eventID, sequence)
			},
		}, nil
	}
	var due, paid int64
	if err := conn.QueryRowContext(ctx, `SELECT amount_due_minor, amount_paid_minor FROM m2_economic_obligations WHERE obligation_id = ? AND contract_id = ? AND kind = 'rent'`, obligationID, payload.SubjectID).Scan(&due, &paid); err != nil {
		return scheduledMutation{}, classifyMissing(err, "M2 rent obligation")
	}
	if due != amount || paid != 0 {
		return scheduledMutation{}, core.NewError(core.CodeProjectionDiverged, "M2 rent obligation differs from contract")
	}
	cohortBalance, cohortVersion, err := readScheduledBalance(ctx, conn, M2DemoCohortAssetAccountID)
	if err != nil {
		return scheduledMutation{}, err
	}
	landlordBalance, landlordVersion, err := readScheduledBalance(ctx, conn, landlordCash)
	if err != nil {
		return scheduledMutation{}, err
	}
	payableBalance, payableVersion, err := readScheduledBalance(ctx, conn, ledger.PayableAccountID)
	if err != nil {
		return scheduledMutation{}, err
	}
	receivableBalance, receivableVersion, err := readScheduledBalance(ctx, conn, ledger.ReceivableAccountID)
	if err != nil {
		return scheduledMutation{}, err
	}
	if cohortBalance < 0 || landlordBalance < 0 || payableBalance > -due || receivableBalance < due {
		return scheduledMutation{}, core.NewError(core.CodeProjectionDiverged, "M2 rent ledger is inconsistent")
	}
	payment := amount
	if cohortBalance < payment {
		payment = cohortBalance
	}
	status, reason := "paid", ""
	if payment < amount {
		reason = "insufficient_tenant_liquidity"
		status = "partial"
		if payment == 0 {
			status = "overdue"
		}
	}
	mutation := scheduledMutation{EventType: "M2RentSettled", EventPayload: struct {
		ObligationID string `json:"obligation_id"`
		Status       string `json:"status"`
		Reason       string `json:"reason_code,omitempty"`
		Due          int64  `json:"due_minor"`
		Paid         int64  `json:"paid_minor"`
		Remaining    int64  `json:"remaining_minor"`
	}{obligationID, status, reason, amount, payment, amount - payment}}
	if payment > 0 {
		newLandlord, ok := checkedAdd(landlordBalance, payment)
		if !ok {
			return scheduledMutation{}, core.NewError(core.CodeIntegerOverflow, "M2 rent landlord credit overflows")
		}
		newPayable, ok := checkedAdd(payableBalance, payment)
		if !ok {
			return scheduledMutation{}, core.NewError(core.CodeIntegerOverflow, "M2 rent payable overflows")
		}
		newReceivable, ok := checkedSubtract(receivableBalance, payment)
		if !ok {
			return scheduledMutation{}, core.NewError(core.CodeIntegerOverflow, "M2 rent receivable overflows")
		}
		mutation.Postings = []scheduledPosting{{M2DemoCohortAssetAccountID, currency, -payment, "M2 rent paid"}, {landlordCash, currency, payment, "M2 rent received"}, {ledger.PayableAccountID, currency, payment, "M2 rent payable cleared"}, {ledger.ReceivableAccountID, currency, -payment, "M2 rent receivable cleared"}}
		mutation.Balances = []balanceMutation{{M2DemoCohortAssetAccountID, cohortVersion, cohortBalance - payment}, {landlordCash, landlordVersion, newLandlord}, {ledger.PayableAccountID, payableVersion, newPayable}, {ledger.ReceivableAccountID, receivableVersion, newReceivable}}
	}
	mutation.ApplyDomainRows = func(ctx context.Context, conn *sql.Conn, eventID string, sequence int64) error {
		if err := execAgentOne(ctx, conn, "settle M2 rent obligation", `UPDATE m2_economic_obligations SET amount_paid_minor = ?, status = ?, last_event_sequence = ? WHERE obligation_id = ? AND amount_paid_minor = 0 AND status = 'accrued'`, payment, status, sequence, obligationID); err != nil {
			return err
		}
		if payment < amount {
			return recordM2ArrearsCase(ctx, conn, obligationID, m2EconomyRentContract, "rent", payload.Day, 2, eventID, sequence)
		}
		return nil
	}
	return mutation, nil
}
