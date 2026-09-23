package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"corerp.local/backend/internal/core"
)

type issuanceEventPayload struct {
	PolicyID        string `json:"policy_id"`
	GrantID         string `json:"grant_id"`
	SourceAccountID string `json:"source_account_id"`
	TargetAccountID string `json:"target_account_id"`
	CurrencyID      string `json:"currency_id"`
	AmountMinor     int64  `json:"amount_minor"`
	ReasonCode      string `json:"reason_code"`
}

func (s *Store) IssueCurrency(ctx context.Context, command core.IssueCurrencyCommand) (core.IssueCurrencyResult, error) {
	if err := command.Validate(); err != nil {
		return core.IssueCurrencyResult{}, err
	}
	requestHash, err := core.IssueCurrencyRequestHash(command)
	if err != nil {
		return core.IssueCurrencyResult{}, err
	}
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return core.IssueCurrencyResult{}, core.WrapError(core.CodeStorageFailure, "begin currency issuance", err)
	}
	defer tx.Rollback(ctx)

	var existingID, existingHash, existingStatus string
	err = tx.conn.QueryRowContext(ctx, `SELECT command_id, request_hash, status FROM commands WHERE instance_id = ? AND branch_id = ? AND command_type = ? AND idempotency_key = ?`, command.InstanceID, command.BranchID, core.IssueCurrencyCommandType, command.IdempotencyKey).Scan(&existingID, &existingHash, &existingStatus)
	switch {
	case err == nil:
		if existingHash != requestHash {
			return core.IssueCurrencyResult{}, core.NewError(core.CodeIdempotencyMismatch, "idempotency key was already used with a different issuance request")
		}
		if existingStatus != "committed" {
			return core.IssueCurrencyResult{}, core.NewError(core.CodeCommandInProgress, "matching issuance command is not committed")
		}
		result, err := loadIssueCurrencyResult(ctx, tx.conn, existingID)
		if err != nil {
			return core.IssueCurrencyResult{}, err
		}
		result.Replayed = true
		return result, nil
	case !errors.Is(err, sql.ErrNoRows):
		return core.IssueCurrencyResult{}, core.WrapError(core.CodeStorageFailure, "look up issuance idempotency key", err)
	}

	var head int64
	if err := tx.conn.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id = ? AND branch_id = ?`, command.InstanceID, command.BranchID).Scan(&head); err != nil {
		return core.IssueCurrencyResult{}, classifyMissing(err, "issuance branch")
	}
	if head != command.ExpectedHead {
		return core.IssueCurrencyResult{}, core.NewError(core.CodeBranchConflict, fmt.Sprintf("expected head %d, current head %d", command.ExpectedHead, head))
	}
	sequence := head + 1
	var epochID string
	if err := tx.conn.QueryRowContext(ctx, `SELECT epoch_id FROM rule_epochs WHERE instance_id = ? AND branch_id = ? AND start_sequence <= ? AND (end_sequence IS NULL OR ? < end_sequence)`, command.InstanceID, command.BranchID, sequence, sequence).Scan(&epochID); err != nil {
		return core.IssueCurrencyResult{}, classifyMissing(err, "issuance Rule Epoch")
	}

	var grantID string
	var grantLimit sql.NullInt64
	if err := tx.conn.QueryRowContext(ctx, `
		SELECT g.grant_id, g.amount_limit_minor
		FROM capability_grants g JOIN principals p ON p.principal_id = g.principal_id
		WHERE g.principal_id = ? AND p.status = 'active' AND g.capability_id = ?
		  AND g.instance_id = ? AND g.branch_id = ? AND g.subject_id IN (?, '*') AND g.status = 'active'
		ORDER BY CASE WHEN g.subject_id = ? THEN 0 ELSE 1 END, g.grant_id LIMIT 1`,
		command.PrincipalID, command.CapabilityID, command.InstanceID, command.BranchID,
		command.TargetAccountID, command.TargetAccountID,
	).Scan(&grantID, &grantLimit); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return core.IssueCurrencyResult{}, core.NewError(core.CodeUnauthorized, "principal lacks issuance capability for this instance, branch, and target")
		}
		return core.IssueCurrencyResult{}, core.WrapError(core.CodeStorageFailure, "read issuance grant", err)
	}

	var sourceAccountID, policyCurrency, policyCapability, policyStatus string
	var perCommandLimit, cumulativeLimit, issuedTotal int64
	if err := tx.conn.QueryRowContext(ctx, `SELECT source_account_id, currency_id, capability_id, per_command_limit_minor, cumulative_limit_minor, issued_total_minor, status FROM issuance_policies WHERE policy_id = ?`, command.PolicyID).Scan(&sourceAccountID, &policyCurrency, &policyCapability, &perCommandLimit, &cumulativeLimit, &issuedTotal, &policyStatus); err != nil {
		return core.IssueCurrencyResult{}, classifyMissing(err, "issuance policy")
	}
	if policyStatus != "active" || policyCapability != command.CapabilityID || policyCurrency != command.CurrencyID {
		return core.IssueCurrencyResult{}, core.NewError(core.CodeUnauthorized, "issuance policy does not authorize the requested capability or currency")
	}
	newIssuedTotal, ok := checkedAdd(issuedTotal, command.AmountMinor)
	if !ok {
		return core.IssueCurrencyResult{}, core.NewError(core.CodeIntegerOverflow, "issuance policy usage overflows")
	}
	if command.AmountMinor > perCommandLimit || newIssuedTotal > cumulativeLimit || (grantLimit.Valid && command.AmountMinor > grantLimit.Int64) {
		return core.IssueCurrencyResult{}, core.NewError(core.CodeIssuanceLimit, "issuance exceeds the scoped grant or policy limit")
	}

	var sourceBalance, sourceVersion int64
	var sourceType, sourceCurrency string
	if err := tx.conn.QueryRowContext(ctx, `SELECT b.balance_minor, b.projection_version, a.account_type, a.currency_id FROM account_balances b JOIN accounts a ON a.account_id = b.account_id WHERE a.account_id = ?`, sourceAccountID).Scan(&sourceBalance, &sourceVersion, &sourceType, &sourceCurrency); err != nil {
		return core.IssueCurrencyResult{}, classifyMissing(err, "issuance source account")
	}
	if sourceType != "issuance_source" || sourceCurrency != command.CurrencyID {
		return core.IssueCurrencyResult{}, core.NewError(core.CodeUnauthorized, "policy source is not a matching issuance account")
	}
	var targetBalance, targetVersion int64
	var targetCurrency string
	if err := tx.conn.QueryRowContext(ctx, `SELECT b.balance_minor, b.projection_version, a.currency_id FROM account_balances b JOIN accounts a ON a.account_id = b.account_id WHERE a.account_id = ?`, command.TargetAccountID).Scan(&targetBalance, &targetVersion, &targetCurrency); err != nil {
		return core.IssueCurrencyResult{}, classifyMissing(err, "issuance target account")
	}
	if targetCurrency != command.CurrencyID || sourceAccountID == command.TargetAccountID {
		return core.IssueCurrencyResult{}, core.NewError(core.CodeInvalidArgument, "issuance source and target must be distinct accounts in the requested currency")
	}
	newSource, ok := checkedSubtract(sourceBalance, command.AmountMinor)
	if !ok {
		return core.IssueCurrencyResult{}, core.NewError(core.CodeIntegerOverflow, "issuance source balance overflows")
	}
	newTarget, ok := checkedAdd(targetBalance, command.AmountMinor)
	if !ok {
		return core.IssueCurrencyResult{}, core.NewError(core.CodeIntegerOverflow, "issuance target balance overflows")
	}

	payload := issuanceEventPayload{command.PolicyID, grantID, sourceAccountID, command.TargetAccountID, command.CurrencyID, command.AmountMinor, command.ReasonCode}
	payloadJSON, err := core.CanonicalJSON(payload)
	if err != nil {
		return core.IssueCurrencyResult{}, err
	}
	batchHash, err := core.HashJSON(struct {
		CommandID, InstanceID, BranchID, EpochID string
		ExpectedHead, Sequence                   int64
		WorldTime, EventType                     string
		Payload                                  issuanceEventPayload
	}{command.CommandID, command.InstanceID, command.BranchID, epochID, head, sequence, command.WorldTime, "CurrencyIssued", payload})
	if err != nil {
		return core.IssueCurrencyResult{}, err
	}
	scopeJSON, err := core.CanonicalJSON(struct {
		InstanceID string   `json:"instance_id"`
		BranchID   string   `json:"branch_id"`
		Subjects   []string `json:"subject_ids"`
		Fields     []string `json:"fields"`
	}{command.InstanceID, command.BranchID, []string{command.TargetAccountID}, []string{"balance_minor"}})
	if err != nil {
		return core.IssueCurrencyResult{}, err
	}
	audienceJSON, err := core.CanonicalJSON(struct {
		Kind       string `json:"kind"`
		InstanceID string `json:"instance_id"`
	}{"instance", command.InstanceID})
	if err != nil {
		return core.IssueCurrencyResult{}, err
	}
	audienceHash, err := core.HashJSON(struct {
		Kind       string `json:"kind"`
		InstanceID string `json:"instance_id"`
	}{"instance", command.InstanceID})
	if err != nil {
		return core.IssueCurrencyResult{}, err
	}

	now := s.now().UTC()
	nowText := now.Format(time.RFC3339Nano)
	attemptID := "attempt_" + command.CommandID + "_1"
	batchID := "batch_" + command.CommandID
	eventID := "event_" + command.CommandID
	entryID := "journal_" + command.CommandID
	interventionID := "intervention_" + command.CommandID
	outboxID := "outbox_" + command.CommandID
	policyDocument := fmt.Sprintf(`{"authorization":"scoped-capability","grant_id":%q,"policy_id":%q}`, grantID, command.PolicyID)
	statements := []struct {
		name, query string
		args        []any
	}{
		{"issuance command", `INSERT INTO commands(command_id, instance_id, branch_id, command_type, idempotency_key, request_hash, expected_head, principal_id, command_policy, status, created_at_utc) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'pending', ?)`, []any{command.CommandID, command.InstanceID, command.BranchID, core.IssueCurrencyCommandType, command.IdempotencyKey, requestHash, head, command.PrincipalID, policyDocument, nowText}},
		{"issuance attempt", `INSERT INTO command_attempts(command_id, attempt_no, attempt_id, status, lease_owner, lease_until_utc, proposal_hash, created_at_utc) VALUES (?, 1, ?, 'ready', 'corerp-m1', ?, ?, ?)`, []any{command.CommandID, attemptID, now.Add(30 * time.Second).Format(time.RFC3339Nano), requestHash, nowText}},
		{"issuance batch", `INSERT INTO event_batches(batch_id, command_id, attempt_no, instance_id, branch_id, epoch_id, expected_head, first_sequence, last_sequence, event_count, world_time, batch_hash, committed_at_utc) VALUES (?, ?, 1, ?, ?, ?, ?, ?, ?, 1, ?, ?, ?)`, []any{batchID, command.CommandID, command.InstanceID, command.BranchID, epochID, head, sequence, sequence, command.WorldTime, batchHash, nowText}},
		{"issuance event", `INSERT INTO events(event_id, batch_id, instance_id, branch_id, event_sequence, batch_index, event_type, actor_id, world_time, payload) VALUES (?, ?, ?, ?, ?, 0, 'CurrencyIssued', ?, ?, ?)`, []any{eventID, batchID, command.InstanceID, command.BranchID, sequence, command.PrincipalID, command.WorldTime, string(payloadJSON)}},
		{"issuance journal", `INSERT INTO journal_entries(entry_id, event_id, status, purpose) VALUES (?, ?, 'draft', 'authorized currency issuance')`, []any{entryID, eventID}},
		{"issuance source posting", `INSERT INTO postings(posting_id, entry_id, account_id, currency_id, amount_minor, memo) VALUES (?, ?, ?, ?, ?, 'authorized issuance source')`, []any{"posting_" + command.CommandID + "_source", entryID, sourceAccountID, command.CurrencyID, -command.AmountMinor}},
		{"issuance target posting", `INSERT INTO postings(posting_id, entry_id, account_id, currency_id, amount_minor, memo) VALUES (?, ?, ?, ?, ?, 'authorized issuance target')`, []any{"posting_" + command.CommandID + "_target", entryID, command.TargetAccountID, command.CurrencyID, command.AmountMinor}},
	}
	for _, statement := range statements {
		if _, err := tx.conn.ExecContext(ctx, statement.query, statement.args...); err != nil {
			return core.IssueCurrencyResult{}, core.WrapError(core.CodeStorageFailure, "insert "+statement.name, err)
		}
	}
	result, err := tx.conn.ExecContext(ctx, `UPDATE journal_entries SET status = 'posted' WHERE entry_id = ? AND status = 'draft'`, entryID)
	if err != nil {
		return core.IssueCurrencyResult{}, core.WrapError(core.CodeStorageFailure, "post issuance journal", err)
	}
	if rows, _ := result.RowsAffected(); rows != 1 {
		return core.IssueCurrencyResult{}, core.NewError(core.CodeStorageFailure, "issuance journal post transition affected an unexpected row count")
	}
	updates := []struct {
		name, query string
		args        []any
	}{
		{"issuance source projection", `UPDATE account_balances SET balance_minor = ?, projection_version = projection_version + 1, last_event_sequence = ? WHERE account_id = ? AND projection_version = ?`, []any{newSource, sequence, sourceAccountID, sourceVersion}},
		{"issuance target projection", `UPDATE account_balances SET balance_minor = ?, projection_version = projection_version + 1, last_event_sequence = ? WHERE account_id = ? AND projection_version = ?`, []any{newTarget, sequence, command.TargetAccountID, targetVersion}},
		{"issuance policy usage", `UPDATE issuance_policies SET issued_total_minor = ? WHERE policy_id = ? AND issued_total_minor = ?`, []any{newIssuedTotal, command.PolicyID, issuedTotal}},
		{"issuance branch head", `UPDATE branches SET head_sequence = ? WHERE instance_id = ? AND branch_id = ? AND head_sequence = ?`, []any{sequence, command.InstanceID, command.BranchID, head}},
	}
	for _, update := range updates {
		result, err := tx.conn.ExecContext(ctx, update.query, update.args...)
		if err != nil {
			return core.IssueCurrencyResult{}, core.WrapError(core.CodeStorageFailure, "update "+update.name, err)
		}
		if rows, _ := result.RowsAffected(); rows != 1 {
			return core.IssueCurrencyResult{}, core.NewError(core.CodeBranchConflict, update.name+" compare-and-swap failed")
		}
	}
	if _, err := tx.conn.ExecContext(ctx, `INSERT INTO intervention_records(intervention_id, event_id, command_id, principal_id, grant_id, policy_id, target_account_id, currency_id, amount_minor, reason_code, scope, event_sequence) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, interventionID, eventID, command.CommandID, command.PrincipalID, grantID, command.PolicyID, command.TargetAccountID, command.CurrencyID, command.AmountMinor, command.ReasonCode, string(scopeJSON), sequence); err != nil {
		return core.IssueCurrencyResult{}, core.WrapError(core.CodeStorageFailure, "insert InterventionRecord", err)
	}
	var recordOrder int64
	if err := tx.conn.QueryRowContext(ctx, `SELECT COALESCE(MAX(record_order), 0) + 1 FROM audit_records WHERE instance_id = ? AND branch_id = ?`, command.InstanceID, command.BranchID).Scan(&recordOrder); err != nil {
		return core.IssueCurrencyResult{}, core.WrapError(core.CodeStorageFailure, "allocate intervention audit order", err)
	}
	if _, err := tx.conn.ExecContext(ctx, `INSERT INTO audit_records(record_id, instance_id, branch_id, record_order, record_type, authority, related_event_id, command_id, attempt_id, trace_id, world_time, recorded_at_utc, audience_scope, payload) VALUES (?, ?, ?, ?, 'intervention', 'audit', ?, ?, ?, ?, ?, ?, ?, ?)`, "audit_"+command.CommandID, command.InstanceID, command.BranchID, recordOrder, eventID, command.CommandID, attemptID, "trace_"+command.CommandID, command.WorldTime, nowText, string(scopeJSON), string(payloadJSON)); err != nil {
		return core.IssueCurrencyResult{}, core.WrapError(core.CodeStorageFailure, "insert issuance audit record", err)
	}
	if _, err := tx.conn.ExecContext(ctx, `INSERT INTO outbox(outbox_id, event_id, topic, audience_scope, audience_scope_hash, payload) VALUES (?, ?, 'currency.issued', ?, ?, ?)`, outboxID, eventID, string(audienceJSON), audienceHash, string(payloadJSON)); err != nil {
		return core.IssueCurrencyResult{}, core.WrapError(core.CodeStorageFailure, "insert issuance Outbox", err)
	}
	result, err = tx.conn.ExecContext(ctx, `UPDATE command_attempts SET status = 'committed', finished_at_utc = ? WHERE command_id = ? AND attempt_no = 1 AND status = 'ready'`, nowText, command.CommandID)
	if err != nil {
		return core.IssueCurrencyResult{}, core.WrapError(core.CodeStorageFailure, "commit issuance attempt", err)
	}
	if rows, _ := result.RowsAffected(); rows != 1 {
		return core.IssueCurrencyResult{}, core.NewError(core.CodeStorageFailure, "issuance attempt status transition affected an unexpected row count")
	}
	result, err = tx.conn.ExecContext(ctx, `UPDATE commands SET status = 'committed' WHERE command_id = ? AND status = 'pending'`, command.CommandID)
	if err != nil {
		return core.IssueCurrencyResult{}, core.WrapError(core.CodeStorageFailure, "commit issuance command", err)
	}
	if rows, _ := result.RowsAffected(); rows != 1 {
		return core.IssueCurrencyResult{}, core.NewError(core.CodeStorageFailure, "issuance command status transition affected an unexpected row count")
	}
	if s.beforeCommit != nil {
		if err := s.beforeCommit(); err != nil {
			return core.IssueCurrencyResult{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return core.IssueCurrencyResult{}, core.WrapError(core.CodeStorageFailure, "commit currency issuance", err)
	}
	return core.IssueCurrencyResult{CommandID: command.CommandID, BatchID: batchID, EventID: eventID, FirstSequence: sequence, LastSequence: sequence, EventCount: 1, RequestHash: requestHash, BatchHash: batchHash}, nil
}

func loadIssueCurrencyResult(ctx context.Context, conn *sql.Conn, commandID string) (core.IssueCurrencyResult, error) {
	var result core.IssueCurrencyResult
	err := conn.QueryRowContext(ctx, `
		SELECT c.command_id, b.batch_id, e.event_id, b.first_sequence, b.last_sequence, b.event_count, c.request_hash, b.batch_hash
		FROM commands c JOIN event_batches b ON b.command_id = c.command_id
		JOIN events e ON e.batch_id = b.batch_id AND e.batch_index = 0
		WHERE c.command_id = ? AND c.status = 'committed'`, commandID,
	).Scan(&result.CommandID, &result.BatchID, &result.EventID, &result.FirstSequence, &result.LastSequence, &result.EventCount, &result.RequestHash, &result.BatchHash)
	if err != nil {
		return core.IssueCurrencyResult{}, core.WrapError(core.CodeStorageFailure, "load committed issuance result", err)
	}
	return result, nil
}
