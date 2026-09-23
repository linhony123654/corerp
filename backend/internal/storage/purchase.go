package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"time"

	"corerp.local/backend/internal/core"
)

type purchaseEventPayload struct {
	BuyerAccountID   string `json:"buyer_account_id"`
	SellerAccountID  string `json:"seller_account_id"`
	BuyerLocationID  string `json:"buyer_location_id"`
	SellerLocationID string `json:"seller_location_id"`
	SKUID            string `json:"sku_id"`
	CurrencyID       string `json:"currency_id"`
	QuantityMinor    int64  `json:"quantity_minor"`
	UnitPriceMinor   int64  `json:"unit_price_minor"`
	TotalMinor       int64  `json:"total_minor"`
}

type batchHashDocument struct {
	CommandID     string               `json:"command_id"`
	InstanceID    string               `json:"instance_id"`
	BranchID      string               `json:"branch_id"`
	EpochID       string               `json:"epoch_id"`
	ExpectedHead  int64                `json:"expected_head"`
	FirstSequence int64                `json:"first_sequence"`
	LastSequence  int64                `json:"last_sequence"`
	WorldTime     string               `json:"world_time"`
	EventType     string               `json:"event_type"`
	ActorID       string               `json:"actor_id"`
	Payload       purchaseEventPayload `json:"payload"`
}

func (s *Store) Purchase(ctx context.Context, command core.PurchaseCommand) (core.PurchaseResult, error) {
	if err := command.Validate(); err != nil {
		return core.PurchaseResult{}, err
	}
	requestHash, err := core.RequestHash(command)
	if err != nil {
		return core.PurchaseResult{}, err
	}

	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return core.PurchaseResult{}, core.WrapError(core.CodeStorageFailure, "begin purchase", err)
	}
	defer tx.Rollback(ctx)

	var existingCommandID, existingHash, existingStatus string
	err = tx.conn.QueryRowContext(ctx, `
		SELECT command_id, request_hash, status
		FROM commands
		WHERE instance_id = ? AND branch_id = ? AND command_type = ? AND idempotency_key = ?`,
		command.InstanceID, command.BranchID, core.PurchaseCommandType, command.IdempotencyKey,
	).Scan(&existingCommandID, &existingHash, &existingStatus)
	switch {
	case err == nil:
		if existingHash != requestHash {
			return core.PurchaseResult{}, core.NewError(core.CodeIdempotencyMismatch, "idempotency key was already used with a different request")
		}
		if existingStatus != "committed" {
			return core.PurchaseResult{}, core.NewError(core.CodeCommandInProgress, "matching command is not committed")
		}
		result, err := loadPurchaseResult(ctx, tx.conn, existingCommandID)
		if err != nil {
			return core.PurchaseResult{}, err
		}
		result.Replayed = true
		return result, nil
	case !errors.Is(err, sql.ErrNoRows):
		return core.PurchaseResult{}, core.WrapError(core.CodeStorageFailure, "look up idempotency key", err)
	}

	var head int64
	if err := tx.conn.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id = ? AND branch_id = ?`, command.InstanceID, command.BranchID).Scan(&head); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return core.PurchaseResult{}, core.NewError(core.CodeNotFound, "instance branch not found")
		}
		return core.PurchaseResult{}, core.WrapError(core.CodeStorageFailure, "read branch head", err)
	}
	if head != command.ExpectedHead {
		return core.PurchaseResult{}, core.NewError(core.CodeBranchConflict, fmt.Sprintf("expected head %d, current head %d", command.ExpectedHead, head))
	}
	firstSequence := head + 1
	lastSequence := firstSequence

	var epochID string
	if err := tx.conn.QueryRowContext(ctx, `
		SELECT epoch_id FROM rule_epochs
		WHERE instance_id = ? AND branch_id = ? AND start_sequence <= ?
		  AND (end_sequence IS NULL OR ? < end_sequence)`,
		command.InstanceID, command.BranchID, firstSequence, firstSequence,
	).Scan(&epochID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return core.PurchaseResult{}, core.NewError(core.CodeNotFound, "no Rule Epoch covers the next event sequence")
		}
		return core.PurchaseResult{}, core.WrapError(core.CodeStorageFailure, "resolve Rule Epoch", err)
	}

	var buyerBalance, buyerBalanceVersion, buyerOverdraft int64
	var buyerCurrency, buyerOwner string
	if err := tx.conn.QueryRowContext(ctx, `
		SELECT b.balance_minor, b.projection_version, a.currency_id, a.overdraft_limit_minor, a.owner_id
		FROM account_balances b JOIN accounts a ON a.account_id = b.account_id
		WHERE a.account_id = ?`, command.BuyerAccountID,
	).Scan(&buyerBalance, &buyerBalanceVersion, &buyerCurrency, &buyerOverdraft, &buyerOwner); err != nil {
		return core.PurchaseResult{}, classifyMissing(err, "buyer account")
	}
	var sellerBalance, sellerBalanceVersion int64
	var sellerCurrency string
	if err := tx.conn.QueryRowContext(ctx, `
		SELECT b.balance_minor, b.projection_version, a.currency_id
		FROM account_balances b JOIN accounts a ON a.account_id = b.account_id
		WHERE a.account_id = ?`, command.SellerAccountID,
	).Scan(&sellerBalance, &sellerBalanceVersion, &sellerCurrency); err != nil {
		return core.PurchaseResult{}, classifyMissing(err, "seller account")
	}
	if buyerCurrency != command.CurrencyID || sellerCurrency != command.CurrencyID {
		return core.PurchaseResult{}, core.NewError(core.CodeInvalidArgument, "purchase accounts do not use the requested currency")
	}
	if command.ActorID != buyerOwner || command.PrincipalID != "principal_"+buyerOwner {
		return core.PurchaseResult{}, core.NewError(core.CodeUnauthorized, "principal cannot spend from the buyer account")
	}

	var buyerStock, buyerStockVersion int64
	if err := tx.conn.QueryRowContext(ctx, `SELECT quantity_minor, projection_version FROM inventory_balances WHERE location_id = ? AND sku_id = ?`, command.BuyerLocationID, command.SKUID).Scan(&buyerStock, &buyerStockVersion); err != nil {
		return core.PurchaseResult{}, classifyMissing(err, "buyer inventory")
	}
	var sellerStock, sellerStockVersion int64
	if err := tx.conn.QueryRowContext(ctx, `SELECT quantity_minor, projection_version FROM inventory_balances WHERE location_id = ? AND sku_id = ?`, command.SellerLocationID, command.SKUID).Scan(&sellerStock, &sellerStockVersion); err != nil {
		return core.PurchaseResult{}, classifyMissing(err, "seller inventory")
	}

	total, ok := checkedMultiplyPositive(command.QuantityMinor, command.UnitPriceMinor)
	if !ok || total > core.MaxJSONSafeInteger {
		return core.PurchaseResult{}, core.NewError(core.CodeIntegerOverflow, "purchase total exceeds the interoperable JSON integer range")
	}
	newBuyerBalance, ok := checkedSubtract(buyerBalance, total)
	if !ok {
		return core.PurchaseResult{}, core.NewError(core.CodeIntegerOverflow, "buyer balance subtraction overflows")
	}
	if newBuyerBalance < -buyerOverdraft {
		return core.PurchaseResult{}, core.NewError(core.CodeInsufficientFunds, "buyer balance would exceed overdraft policy")
	}
	newSellerBalance, ok := checkedAdd(sellerBalance, total)
	if !ok {
		return core.PurchaseResult{}, core.NewError(core.CodeIntegerOverflow, "seller balance addition overflows")
	}
	newSellerStock, ok := checkedSubtract(sellerStock, command.QuantityMinor)
	if !ok || newSellerStock < 0 {
		return core.PurchaseResult{}, core.NewError(core.CodeInsufficientStock, "seller inventory is below requested quantity")
	}
	newBuyerStock, ok := checkedAdd(buyerStock, command.QuantityMinor)
	if !ok {
		return core.PurchaseResult{}, core.NewError(core.CodeIntegerOverflow, "buyer inventory addition overflows")
	}

	clockTime := s.now().UTC()
	now := clockTime.Format(time.RFC3339Nano)
	leaseUntil := clockTime.Add(30 * time.Second).Format(time.RFC3339Nano)
	attemptID := "attempt_" + command.CommandID + "_1"
	batchID := "batch_" + command.CommandID
	eventID := "event_" + command.CommandID
	entryID := "journal_" + command.CommandID
	movementID := "movement_" + command.CommandID
	outboxID := "outbox_" + command.CommandID
	payload := purchaseEventPayload{
		BuyerAccountID: command.BuyerAccountID, SellerAccountID: command.SellerAccountID,
		BuyerLocationID: command.BuyerLocationID, SellerLocationID: command.SellerLocationID,
		SKUID: command.SKUID, CurrencyID: command.CurrencyID, QuantityMinor: command.QuantityMinor,
		UnitPriceMinor: command.UnitPriceMinor, TotalMinor: total,
	}
	payloadJSON, err := core.CanonicalJSON(payload)
	if err != nil {
		return core.PurchaseResult{}, core.WrapError(core.CodeInvalidArgument, "encode purchase event", err)
	}
	batchHash, err := core.HashJSON(batchHashDocument{
		CommandID: command.CommandID, InstanceID: command.InstanceID, BranchID: command.BranchID,
		EpochID: epochID, ExpectedHead: command.ExpectedHead, FirstSequence: firstSequence,
		LastSequence: lastSequence, WorldTime: command.WorldTime, EventType: "PurchaseCompleted",
		ActorID: command.ActorID, Payload: payload,
	})
	if err != nil {
		return core.PurchaseResult{}, err
	}
	audience := struct {
		Kind       string `json:"kind"`
		InstanceID string `json:"instance_id"`
	}{Kind: "instance", InstanceID: command.InstanceID}
	audienceJSON, err := core.CanonicalJSON(audience)
	if err != nil {
		return core.PurchaseResult{}, err
	}
	audienceHash, err := core.HashJSON(audience)
	if err != nil {
		return core.PurchaseResult{}, err
	}

	if _, err := tx.conn.ExecContext(ctx, `
		INSERT INTO commands(command_id, instance_id, branch_id, command_type, idempotency_key, request_hash, expected_head, principal_id, command_policy, status, created_at_utc)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, '{"authorization":"demo-owner"}', 'pending', ?)`,
		command.CommandID, command.InstanceID, command.BranchID, core.PurchaseCommandType,
		command.IdempotencyKey, requestHash, command.ExpectedHead, command.PrincipalID, now,
	); err != nil {
		return core.PurchaseResult{}, core.WrapError(core.CodeStorageFailure, "insert command", err)
	}
	if _, err := tx.conn.ExecContext(ctx, `
		INSERT INTO command_attempts(command_id, attempt_no, attempt_id, status, lease_owner, lease_until_utc, proposal_hash, created_at_utc)
		VALUES (?, 1, ?, 'ready', 'corerp-m1', ?, ?, ?)`,
		command.CommandID, attemptID, leaseUntil, requestHash, now,
	); err != nil {
		return core.PurchaseResult{}, core.WrapError(core.CodeStorageFailure, "insert command attempt", err)
	}
	if _, err := tx.conn.ExecContext(ctx, `
		INSERT INTO event_batches(batch_id, command_id, attempt_no, instance_id, branch_id, epoch_id, expected_head, first_sequence, last_sequence, event_count, world_time, batch_hash, committed_at_utc)
		VALUES (?, ?, 1, ?, ?, ?, ?, ?, ?, 1, ?, ?, ?)`,
		batchID, command.CommandID, command.InstanceID, command.BranchID, epochID,
		command.ExpectedHead, firstSequence, lastSequence, command.WorldTime, batchHash, now,
	); err != nil {
		return core.PurchaseResult{}, core.WrapError(core.CodeStorageFailure, "insert Event Batch", err)
	}
	if _, err := tx.conn.ExecContext(ctx, `
		INSERT INTO events(event_id, batch_id, instance_id, branch_id, event_sequence, batch_index, event_type, actor_id, world_time, payload)
		VALUES (?, ?, ?, ?, ?, 0, 'PurchaseCompleted', ?, ?, ?)`,
		eventID, batchID, command.InstanceID, command.BranchID, firstSequence,
		command.ActorID, command.WorldTime, string(payloadJSON),
	); err != nil {
		return core.PurchaseResult{}, core.WrapError(core.CodeStorageFailure, "insert purchase event", err)
	}
	if _, err := tx.conn.ExecContext(ctx, `INSERT INTO journal_entries(entry_id, event_id, status, purpose) VALUES (?, ?, 'draft', 'purchase settlement')`, entryID, eventID); err != nil {
		return core.PurchaseResult{}, core.WrapError(core.CodeStorageFailure, "insert journal entry", err)
	}
	if _, err := tx.conn.ExecContext(ctx, `INSERT INTO postings(posting_id, entry_id, account_id, currency_id, amount_minor, memo) VALUES (?, ?, ?, ?, ?, 'buyer payment')`, "posting_"+command.CommandID+"_buyer", entryID, command.BuyerAccountID, command.CurrencyID, -total); err != nil {
		return core.PurchaseResult{}, core.WrapError(core.CodeStorageFailure, "insert buyer posting", err)
	}
	if _, err := tx.conn.ExecContext(ctx, `INSERT INTO postings(posting_id, entry_id, account_id, currency_id, amount_minor, memo) VALUES (?, ?, ?, ?, ?, 'seller receipt')`, "posting_"+command.CommandID+"_seller", entryID, command.SellerAccountID, command.CurrencyID, total); err != nil {
		return core.PurchaseResult{}, core.WrapError(core.CodeStorageFailure, "insert seller posting", err)
	}
	result, err := tx.conn.ExecContext(ctx, `UPDATE journal_entries SET status = 'posted' WHERE entry_id = ? AND status = 'draft'`, entryID)
	if err != nil {
		return core.PurchaseResult{}, core.WrapError(core.CodeStorageFailure, "post balanced journal", err)
	}
	if rows, err := result.RowsAffected(); err != nil || rows != 1 {
		return core.PurchaseResult{}, core.NewError(core.CodeStorageFailure, "journal post transition affected an unexpected row count")
	}
	if _, err := tx.conn.ExecContext(ctx, `
		INSERT INTO stock_movements(movement_id, event_id, sku_id, from_location_id, to_location_id, quantity_minor, movement_kind, reason_code)
		VALUES (?, ?, ?, ?, ?, ?, 'transfer', 'purchase')`, movementID, eventID, command.SKUID, command.SellerLocationID, command.BuyerLocationID, command.QuantityMinor); err != nil {
		return core.PurchaseResult{}, core.WrapError(core.CodeStorageFailure, "insert stock movement", err)
	}

	updates := []struct {
		name  string
		query string
		args  []any
	}{
		{"buyer balance", `UPDATE account_balances SET balance_minor = ?, projection_version = projection_version + 1, last_event_sequence = ? WHERE account_id = ? AND projection_version = ?`, []any{newBuyerBalance, lastSequence, command.BuyerAccountID, buyerBalanceVersion}},
		{"seller balance", `UPDATE account_balances SET balance_minor = ?, projection_version = projection_version + 1, last_event_sequence = ? WHERE account_id = ? AND projection_version = ?`, []any{newSellerBalance, lastSequence, command.SellerAccountID, sellerBalanceVersion}},
		{"buyer inventory", `UPDATE inventory_balances SET quantity_minor = ?, projection_version = projection_version + 1, last_event_sequence = ? WHERE location_id = ? AND sku_id = ? AND projection_version = ?`, []any{newBuyerStock, lastSequence, command.BuyerLocationID, command.SKUID, buyerStockVersion}},
		{"seller inventory", `UPDATE inventory_balances SET quantity_minor = ?, projection_version = projection_version + 1, last_event_sequence = ? WHERE location_id = ? AND sku_id = ? AND projection_version = ?`, []any{newSellerStock, lastSequence, command.SellerLocationID, command.SKUID, sellerStockVersion}},
		{"branch head", `UPDATE branches SET head_sequence = ? WHERE instance_id = ? AND branch_id = ? AND head_sequence = ?`, []any{lastSequence, command.InstanceID, command.BranchID, command.ExpectedHead}},
	}
	for _, update := range updates {
		result, err := tx.conn.ExecContext(ctx, update.query, update.args...)
		if err != nil {
			return core.PurchaseResult{}, core.WrapError(core.CodeStorageFailure, "update "+update.name, err)
		}
		rows, err := result.RowsAffected()
		if err != nil || rows != 1 {
			return core.PurchaseResult{}, core.NewError(core.CodeBranchConflict, update.name+" compare-and-swap failed")
		}
	}

	if _, err := tx.conn.ExecContext(ctx, `
		INSERT INTO outbox(outbox_id, event_id, topic, audience_scope, audience_scope_hash, payload)
		VALUES (?, ?, 'purchase.completed', ?, ?, ?)`, outboxID, eventID, string(audienceJSON), audienceHash, string(payloadJSON)); err != nil {
		return core.PurchaseResult{}, core.WrapError(core.CodeStorageFailure, "insert Outbox row", err)
	}
	result, err = tx.conn.ExecContext(ctx, `UPDATE command_attempts SET status = 'committed', finished_at_utc = ? WHERE command_id = ? AND attempt_no = 1 AND status = 'ready'`, now, command.CommandID)
	if err != nil {
		return core.PurchaseResult{}, core.WrapError(core.CodeStorageFailure, "commit command attempt", err)
	}
	if rows, err := result.RowsAffected(); err != nil || rows != 1 {
		return core.PurchaseResult{}, core.NewError(core.CodeStorageFailure, "command attempt status transition affected an unexpected row count")
	}
	result, err = tx.conn.ExecContext(ctx, `UPDATE commands SET status = 'committed' WHERE command_id = ? AND status = 'pending'`, command.CommandID)
	if err != nil {
		return core.PurchaseResult{}, core.WrapError(core.CodeStorageFailure, "commit command", err)
	}
	if rows, err := result.RowsAffected(); err != nil || rows != 1 {
		return core.PurchaseResult{}, core.NewError(core.CodeStorageFailure, "command status transition affected an unexpected row count")
	}

	if s.beforeCommit != nil {
		if err := s.beforeCommit(); err != nil {
			return core.PurchaseResult{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return core.PurchaseResult{}, core.WrapError(core.CodeStorageFailure, "commit purchase", err)
	}
	return core.PurchaseResult{
		CommandID: command.CommandID, BatchID: batchID, EventID: eventID,
		FirstSequence: firstSequence, LastSequence: lastSequence, EventCount: 1,
		RequestHash: requestHash, BatchHash: batchHash,
	}, nil
}

func loadPurchaseResult(ctx context.Context, conn *sql.Conn, commandID string) (core.PurchaseResult, error) {
	var result core.PurchaseResult
	err := conn.QueryRowContext(ctx, `
		SELECT c.command_id, b.batch_id, e.event_id, b.first_sequence, b.last_sequence,
		       b.event_count, c.request_hash, b.batch_hash
		FROM commands c
		JOIN event_batches b ON b.command_id = c.command_id
		JOIN events e ON e.batch_id = b.batch_id AND e.batch_index = 0
		WHERE c.command_id = ? AND c.status = 'committed'`, commandID,
	).Scan(&result.CommandID, &result.BatchID, &result.EventID, &result.FirstSequence,
		&result.LastSequence, &result.EventCount, &result.RequestHash, &result.BatchHash)
	if err != nil {
		return core.PurchaseResult{}, core.WrapError(core.CodeStorageFailure, "load committed purchase result", err)
	}
	return result, nil
}

func classifyMissing(err error, resource string) error {
	if errors.Is(err, sql.ErrNoRows) {
		return core.NewError(core.CodeNotFound, resource+" not found")
	}
	return core.WrapError(core.CodeStorageFailure, "read "+resource, err)
}

func checkedMultiplyPositive(left, right int64) (int64, bool) {
	if left <= 0 || right <= 0 || left > math.MaxInt64/right {
		return 0, false
	}
	return left * right, true
}

func checkedAdd(left, right int64) (int64, bool) {
	if (right > 0 && left > math.MaxInt64-right) || (right < 0 && left < math.MinInt64-right) {
		return 0, false
	}
	return left + right, true
}

func checkedSubtract(left, right int64) (int64, bool) {
	if right > 0 && left < math.MinInt64+right {
		return 0, false
	}
	if right < 0 && left > math.MaxInt64+right {
		return 0, false
	}
	return left - right, true
}
