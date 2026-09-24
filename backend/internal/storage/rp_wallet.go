package storage

import (
	"context"

	"corerp.local/backend/internal/core"
)

// RPWallet is an on-demand view of the controlled character's existing account.
// Decimal strings preserve exact amounts across the JavaScript transport.
type RPWallet struct {
	BalanceMinor      int64  `json:"balance_minor,string"`
	CurrencyID        string `json:"currency_id"`
	CurrencySymbol    string `json:"currency_symbol"`
	CurrencyScale     int    `json:"currency_scale"`
	WorldTime         string `json:"world_time"`
	ObservationCursor int64  `json:"observation_cursor"`
}

func (s *Store) ReadRPWallet(ctx context.Context, request core.RPSessionReadRequest) (RPWallet, error) {
	if err := request.Validate(); err != nil {
		return RPWallet{}, err
	}
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return RPWallet{}, core.WrapError(core.CodeStorageFailure, "begin wallet read", err)
	}
	defer tx.Rollback(ctx)
	session, err := loadRPSession(ctx, tx.conn, request.PrincipalID, request.SessionID)
	if err != nil {
		return RPWallet{}, err
	}
	if err := authorizeRPControl(ctx, tx.conn, request.PrincipalID, session.InstanceID, session.BranchID, session.ControlledEntityID); err != nil {
		return RPWallet{}, err
	}
	if session.Status != "active" {
		return RPWallet{}, core.NewError(core.CodeBranchConflict, "wallet requires an active session")
	}
	if err := validateRPBinding(ctx, tx.conn, session.InstanceID, session.BranchID, session.ControlledEntityID); err != nil {
		return RPWallet{}, err
	}
	var result RPWallet
	err = tx.conn.QueryRowContext(ctx, `
		SELECT balance.balance_minor, currency.currency_id, currency.symbol, currency.scale,
		       clock.current_world_time, branch.head_sequence
		FROM materialized_entities entity
		JOIN accounts account ON account.account_id=entity.asset_account_id AND account.currency_id=entity.currency_id
		JOIN account_balances balance ON balance.account_id=account.account_id
		JOIN currencies currency ON currency.currency_id=account.currency_id
		JOIN branches branch ON branch.instance_id=? AND branch.branch_id=?
		JOIN world_clocks clock ON clock.instance_id=branch.instance_id AND clock.branch_id=branch.branch_id
		WHERE entity.entity_id=? AND account.closed_by_event_id IS NULL`,
		session.InstanceID, session.BranchID, session.ControlledEntityID,
	).Scan(&result.BalanceMinor, &result.CurrencyID, &result.CurrencySymbol, &result.CurrencyScale, &result.WorldTime, &result.ObservationCursor)
	if err != nil {
		return RPWallet{}, classifyMissing(err, "controlled character wallet")
	}
	return result, nil
}
