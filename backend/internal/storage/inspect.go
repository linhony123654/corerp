package storage

import (
	"context"

	"corerp.local/backend/internal/core"
)

func (s *Store) ReadDemoState(ctx context.Context) (State, error) {
	var state State
	queries := []struct {
		name  string
		query string
		args  []any
		dest  *int64
	}{
		{"branch head", `SELECT head_sequence FROM branches WHERE instance_id = ? AND branch_id = ?`, []any{DemoInstanceID, DemoBranchID}, &state.HeadSequence},
		{"buyer balance", `SELECT balance_minor FROM account_balances WHERE account_id = ?`, []any{DemoBuyerAccountID}, &state.BuyerBalance},
		{"seller balance", `SELECT balance_minor FROM account_balances WHERE account_id = ?`, []any{DemoSellerAccountID}, &state.SellerBalance},
		{"buyer inventory", `SELECT quantity_minor FROM inventory_balances WHERE location_id = ? AND sku_id = ?`, []any{DemoBuyerLocationID, DemoSKUID}, &state.BuyerInventory},
		{"seller inventory", `SELECT quantity_minor FROM inventory_balances WHERE location_id = ? AND sku_id = ?`, []any{DemoSellerLocationID, DemoSKUID}, &state.SellerInventory},
		{"commands", `SELECT COUNT(*) FROM commands WHERE instance_id = ? AND branch_id = ?`, []any{DemoInstanceID, DemoBranchID}, &state.Commands},
		{"events", `SELECT COUNT(*) FROM events WHERE instance_id = ? AND branch_id = ?`, []any{DemoInstanceID, DemoBranchID}, &state.Events},
		{"journal entries", `SELECT COUNT(*) FROM journal_entries j JOIN events e ON e.event_id = j.event_id WHERE e.instance_id = ? AND e.branch_id = ?`, []any{DemoInstanceID, DemoBranchID}, &state.JournalEntries},
		{"postings", `SELECT COUNT(*) FROM postings p JOIN journal_entries j ON j.entry_id = p.entry_id JOIN events e ON e.event_id = j.event_id WHERE e.instance_id = ? AND e.branch_id = ?`, []any{DemoInstanceID, DemoBranchID}, &state.Postings},
		{"stock movements", `SELECT COUNT(*) FROM stock_movements m JOIN events e ON e.event_id = m.event_id WHERE e.instance_id = ? AND e.branch_id = ?`, []any{DemoInstanceID, DemoBranchID}, &state.StockMovements},
		{"Outbox", `SELECT COUNT(*) FROM outbox o JOIN events e ON e.event_id = o.event_id WHERE e.instance_id = ? AND e.branch_id = ?`, []any{DemoInstanceID, DemoBranchID}, &state.OutboxRows},
	}
	for _, query := range queries {
		if err := s.db.QueryRowContext(ctx, query.query, query.args...).Scan(query.dest); err != nil {
			return State{}, core.WrapError(core.CodeStorageFailure, "read "+query.name, err)
		}
	}
	return state, nil
}
