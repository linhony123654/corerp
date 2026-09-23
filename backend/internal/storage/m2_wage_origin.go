package storage

import (
	"context"
	"database/sql"
	"encoding/json"

	"corerp.local/backend/internal/core"
)

// Historical claims use the sealed accrual, never today's workforce size.
func readM2WageOrigin(ctx context.Context, conn *sql.Conn, obligationID string) (due, paid, rate, workers int64, err error) {
	var payload string
	err = conn.QueryRowContext(ctx, `SELECT o.amount_due_minor, o.amount_paid_minor, e.payload
		FROM m2_economic_obligations o JOIN events e ON e.event_id = o.defining_event_id
		WHERE o.obligation_id = ? AND o.contract_id = ? AND o.kind = 'wage'
		AND e.instance_id = ? AND e.branch_id = ? AND e.event_type = 'M2WageAccrued'
		AND e.world_time = o.period_end`, obligationID, m2EconomyContractID, M2DemoInstanceID, M2DemoBranchID).Scan(&due, &paid, &payload)
	if err != nil {
		if err == sql.ErrNoRows {
			err = core.NewError(core.CodeProjectionDiverged, "wage obligation lacks its original accrual")
		} else {
			err = core.WrapError(core.CodeStorageFailure, "read original wage accrual", err)
		}
		return
	}
	var origin struct {
		ContractID string `json:"contract_id"`
		Workers    int64  `json:"worker_count"`
		Rate       int64  `json:"unit_wage_minor"`
		Amount     int64  `json:"amount_minor"`
	}
	if json.Unmarshal([]byte(payload), &origin) != nil {
		err = core.NewError(core.CodeProjectionDiverged, "invalid original wage accrual")
		return
	}
	rate, workers = origin.Rate, origin.Workers
	amount, ok := checkedMultiplyPositive(workers, rate)
	if !ok || workers > 10000 || origin.ContractID != m2EconomyContractID || origin.Amount != amount || due != amount || paid < 0 || paid > due {
		err = core.NewError(core.CodeProjectionDiverged, "wage obligation differs from original accrual")
	}
	return
}
