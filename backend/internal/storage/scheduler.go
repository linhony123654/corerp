package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"corerp.local/backend/internal/core"
)

type SchedulerItem struct {
	SchedulerItemID  string `json:"scheduler_item_id"`
	WorldTime        string `json:"world_time"`
	PhaseID          string `json:"phase_id"`
	DeclaredPriority int64  `json:"declared_priority"`
	Status           string `json:"status"`
	Payload          string `json:"payload"`
}

type StrictRunResult struct {
	RunID          string `json:"run_id"`
	StartDay       int    `json:"start_day"`
	TargetDay      int    `json:"target_day"`
	ProcessedItems int    `json:"processed_items"`
	PendingDue     int64  `json:"pending_due"`
	Status         string `json:"status"`
	HeadSequence   int64  `json:"head_sequence"`
}

type scheduledPayload struct {
	Kind       string `json:"kind"`
	Day        int    `json:"day"`
	SubjectID  string `json:"subject_id"`
	AccountID  string `json:"account_id"`
	LocationID string `json:"location_id"`
}

type scheduledPosting struct {
	AccountID  string
	CurrencyID string
	Amount     int64
	Memo       string
}

type scheduledMovement struct {
	SKUID        string
	FromLocation string
	ToLocation   string
	Quantity     int64
	Kind         string
	Reason       string
	CapabilityID *string
}

type balanceMutation struct {
	AccountID       string
	ExpectedVersion int64
	NewBalance      int64
}

type inventoryMutation struct {
	LocationID      string
	SKUID           string
	ExpectedVersion int64
	NewQuantity     int64
}

type scheduledMutation struct {
	Private         bool // Personal effects must not use the default instance-wide Outbox.
	EventType       string
	EventPayload    any
	Postings        []scheduledPosting
	Movements       []scheduledMovement
	Balances        []balanceMutation
	Inventory       []inventoryMutation
	ApplyDomainRows func(context.Context, *sql.Conn, string, int64) error
}

type obligationLedger struct {
	ExpenseAccountID    string
	PayableAccountID    string
	ReceivableAccountID string
	IncomeAccountID     string
	CurrencyID          string
}

func (s *Store) ListDueSchedulerItems(ctx context.Context, throughDay, limit int) ([]SchedulerItem, error) {
	if throughDay < 0 || limit <= 0 || limit > 10000 {
		return nil, core.NewError(core.CodeInvalidArgument, "scheduler day/limit is outside the supported range")
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT scheduler_item_id, world_time, phase_id, declared_priority, status, payload
		FROM scheduler_items
		WHERE instance_id = ? AND branch_id = ? AND status = 'pending' AND world_time <= ?
		ORDER BY world_time, phase_id, declared_priority, scheduler_item_id
		LIMIT ?`, DemoInstanceID, DemoBranchID, strictDayTime(throughDay), limit)
	if err != nil {
		return nil, core.WrapError(core.CodeStorageFailure, "list due scheduler items", err)
	}
	defer rows.Close()
	items := make([]SchedulerItem, 0, limit)
	for rows.Next() {
		var item SchedulerItem
		if err := rows.Scan(&item.SchedulerItemID, &item.WorldTime, &item.PhaseID, &item.DeclaredPriority, &item.Status, &item.Payload); err != nil {
			return nil, core.WrapError(core.CodeStorageFailure, "scan due scheduler item", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, core.WrapError(core.CodeStorageFailure, "iterate due scheduler items", err)
	}
	return items, nil
}

func (s *Store) RunStrictWorld(ctx context.Context, targetDay, budget int) (StrictRunResult, error) {
	if targetDay < 0 || targetDay > 90 {
		return StrictRunResult{}, core.NewError(core.CodeInvalidArgument, "strict M1 target day must be between 0 and 90")
	}
	if budget <= 0 || budget > 100000 {
		return StrictRunResult{}, core.NewError(core.CodeInvalidArgument, "scheduler budget must be between 1 and 100000")
	}
	var startDay int
	if err := s.db.QueryRowContext(ctx, `SELECT current_day FROM world_clocks WHERE instance_id = ? AND branch_id = ?`, DemoInstanceID, DemoBranchID).Scan(&startDay); err != nil {
		return StrictRunResult{}, classifyMissing(err, "world clock")
	}
	if targetDay < startDay {
		return StrictRunResult{}, core.NewError(core.CodeInvalidArgument, "strict world cannot run backward")
	}
	if err := s.ensureClockCheckpoint(ctx, targetDay); err != nil {
		return StrictRunResult{}, err
	}
	now := s.now().UTC()
	runID := fmt.Sprintf("run_%03d_%d", targetDay, now.UnixNano())
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO simulation_runs(run_id, instance_id, branch_id, start_day, target_day, processed_items, status, started_at_utc)
		VALUES (?, ?, ?, ?, ?, 0, 'running', ?)`, runID, DemoInstanceID, DemoBranchID, startDay, targetDay, now.Format(time.RFC3339Nano)); err != nil {
		return StrictRunResult{}, core.WrapError(core.CodeStorageFailure, "start strict simulation run", err)
	}

	processed := 0
	for processed < budget {
		didWork, err := s.executeNextScheduledItem(ctx, targetDay)
		if err != nil {
			_, _ = s.db.ExecContext(ctx, `UPDATE simulation_runs SET processed_items = ?, status = 'failed', finished_at_utc = ? WHERE run_id = ?`, processed, s.now().UTC().Format(time.RFC3339Nano), runID)
			return StrictRunResult{}, err
		}
		if !didWork {
			break
		}
		processed++
	}
	var pending, head int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM scheduler_items WHERE instance_id = ? AND branch_id = ? AND status = 'pending' AND world_time <= ?`, DemoInstanceID, DemoBranchID, strictDayTime(targetDay)).Scan(&pending); err != nil {
		return StrictRunResult{}, core.WrapError(core.CodeStorageFailure, "count due scheduler items", err)
	}
	status := "completed"
	if pending > 0 {
		status = "budget_exhausted"
	}
	finished := s.now().UTC().Format(time.RFC3339Nano)
	if _, err := s.db.ExecContext(ctx, `UPDATE simulation_runs SET processed_items = ?, status = ?, finished_at_utc = ? WHERE run_id = ?`, processed, status, finished, runID); err != nil {
		return StrictRunResult{}, core.WrapError(core.CodeStorageFailure, "finish strict simulation run", err)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id = ? AND branch_id = ?`, DemoInstanceID, DemoBranchID).Scan(&head); err != nil {
		return StrictRunResult{}, core.WrapError(core.CodeStorageFailure, "read strict simulation head", err)
	}
	return StrictRunResult{
		RunID: runID, StartDay: startDay, TargetDay: targetDay, ProcessedItems: processed,
		PendingDue: pending, Status: status, HeadSequence: head,
	}, nil
}

func (s *Store) ensureClockCheckpoint(ctx context.Context, day int) error {
	payload, err := core.CanonicalJSON(scheduledPayload{Kind: "clock_checkpoint", Day: day, SubjectID: DemoBranchID})
	if err != nil {
		return err
	}
	itemID := fmt.Sprintf("sched_%03d_clock_checkpoint", day)
	if _, err := s.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO scheduler_items(scheduler_item_id, instance_id, branch_id, world_time, phase_id, declared_priority, status, payload)
		VALUES (?, ?, ?, ?, '90_clock_checkpoint', 0, 'pending', ?)`,
		itemID, DemoInstanceID, DemoBranchID, strictDayTime(day), string(payload)); err != nil {
		return core.WrapError(core.CodeStorageFailure, "schedule clock checkpoint", err)
	}
	return nil
}

func (s *Store) executeNextScheduledItem(ctx context.Context, targetDay int) (bool, error) {
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return false, core.WrapError(core.CodeStorageFailure, "begin scheduled item", err)
	}
	defer tx.Rollback(ctx)
	var item SchedulerItem
	err = tx.conn.QueryRowContext(ctx, `
		SELECT scheduler_item_id, world_time, phase_id, declared_priority, status, payload
		FROM scheduler_items
		WHERE instance_id = ? AND branch_id = ? AND status = 'pending' AND world_time <= ?
		ORDER BY world_time, phase_id, declared_priority, scheduler_item_id
		LIMIT 1`, DemoInstanceID, DemoBranchID, strictDayTime(targetDay)).Scan(
		&item.SchedulerItemID, &item.WorldTime, &item.PhaseID, &item.DeclaredPriority, &item.Status, &item.Payload,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, core.WrapError(core.CodeStorageFailure, "select next scheduled item", err)
	}
	var payload scheduledPayload
	if err := json.Unmarshal([]byte(item.Payload), &payload); err != nil {
		return false, core.WrapError(core.CodeStorageFailure, "decode scheduler payload", err)
	}
	if payload.Day < 0 || item.WorldTime != strictDayTime(payload.Day) {
		return false, core.NewError(core.CodeStorageFailure, "scheduler payload day does not match stable world_time")
	}
	mutation, err := prepareScheduledMutation(ctx, tx.conn, payload)
	if err != nil {
		return false, err
	}
	if err := s.commitScheduledMutation(ctx, tx, item, payload, mutation); err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, core.WrapError(core.CodeStorageFailure, "commit scheduled item", err)
	}
	return true, nil
}

func prepareScheduledMutation(ctx context.Context, conn *sql.Conn, payload scheduledPayload) (scheduledMutation, error) {
	switch payload.Kind {
	case "wage_accrual":
		return prepareWageAccrual(ctx, conn, payload)
	case "wage_payment":
		return prepareWagePayment(ctx, conn, payload)
	case "wage_retry":
		return prepareWageRetry(ctx, conn, payload)
	case "rent_accrual":
		return prepareRentAccrual(ctx, conn, payload)
	case "rent_payment":
		return prepareRentPayment(ctx, conn, payload)
	case "rent_past_due":
		return prepareRentPastDue(ctx, conn, payload)
	case "household_purchase":
		return prepareHouseholdPurchase(ctx, conn, payload)
	case "store_restock":
		return prepareStoreRestock(ctx, conn, payload)
	case "clock_checkpoint":
		return scheduledMutation{EventType: "WorldTimeAdvanced", EventPayload: payload}, nil
	default:
		return scheduledMutation{}, core.NewError(core.CodeStorageFailure, "unsupported scheduler task kind "+payload.Kind)
	}
}

func (s *Store) commitScheduledMutation(ctx context.Context, tx *immediateTx, item SchedulerItem, payload scheduledPayload, mutation scheduledMutation) error {
	return s.commitScheduledMutationForBranch(ctx, tx, item, payload, mutation, DemoInstanceID, DemoBranchID)
}

func (s *Store) commitScheduledMutationForBranch(ctx context.Context, tx *immediateTx, item SchedulerItem, payload scheduledPayload, mutation scheduledMutation, instanceID, branchID string) error {
	var head int64
	if err := tx.conn.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id = ? AND branch_id = ?`, instanceID, branchID).Scan(&head); err != nil {
		return core.WrapError(core.CodeStorageFailure, "read scheduler branch head", err)
	}
	sequence := head + 1
	var epochID string
	if err := tx.conn.QueryRowContext(ctx, `SELECT epoch_id FROM rule_epochs WHERE instance_id = ? AND branch_id = ? AND start_sequence <= ? AND (end_sequence IS NULL OR ? < end_sequence)`, instanceID, branchID, sequence, sequence).Scan(&epochID); err != nil {
		return core.WrapError(core.CodeStorageFailure, "resolve scheduler Rule Epoch", err)
	}
	commandID := "cmd_" + item.SchedulerItemID
	attemptID := "attempt_" + item.SchedulerItemID + "_1"
	batchID := "batch_" + item.SchedulerItemID
	eventID := "event_" + item.SchedulerItemID
	outboxID := "outbox_" + item.SchedulerItemID
	requestHash, err := core.HashJSON(struct {
		CommandType string `json:"command_type"`
		ItemID      string `json:"scheduler_item_id"`
		Payload     string `json:"payload"`
	}{CommandType: "ScheduledTask", ItemID: item.SchedulerItemID, Payload: item.Payload})
	if err != nil {
		return err
	}
	eventPayload, err := core.CanonicalJSON(mutation.EventPayload)
	if err != nil {
		return err
	}
	batchHash, err := core.HashJSON(struct {
		ItemID        string `json:"scheduler_item_id"`
		EventType     string `json:"event_type"`
		EventSequence int64  `json:"event_sequence"`
		WorldTime     string `json:"world_time"`
		Payload       any    `json:"payload"`
	}{ItemID: item.SchedulerItemID, EventType: mutation.EventType, EventSequence: sequence, WorldTime: item.WorldTime, Payload: mutation.EventPayload})
	if err != nil {
		return err
	}
	now := s.now().UTC()
	nowText := now.Format(time.RFC3339Nano)
	statements := []struct {
		name  string
		query string
		args  []any
	}{
		{"scheduler command", `INSERT INTO commands(command_id, instance_id, branch_id, command_type, idempotency_key, request_hash, expected_head, principal_id, command_policy, status, created_at_utc) VALUES (?, ?, ?, 'ScheduledTask', ?, ?, ?, 'principal_system', '{"authorization":"ruleset-scheduler"}', 'pending', ?)`, []any{commandID, instanceID, branchID, item.SchedulerItemID, requestHash, head, nowText}},
		{"scheduler attempt", `INSERT INTO command_attempts(command_id, attempt_no, attempt_id, status, lease_owner, lease_until_utc, proposal_hash, created_at_utc) VALUES (?, 1, ?, 'ready', 'corerp-m1-scheduler', ?, ?, ?)`, []any{commandID, attemptID, now.Add(30 * time.Second).Format(time.RFC3339Nano), requestHash, nowText}},
		{"scheduler batch", `INSERT INTO event_batches(batch_id, command_id, attempt_no, instance_id, branch_id, epoch_id, expected_head, first_sequence, last_sequence, event_count, world_time, batch_hash, committed_at_utc) VALUES (?, ?, 1, ?, ?, ?, ?, ?, ?, 1, ?, ?, ?)`, []any{batchID, commandID, instanceID, branchID, epochID, head, sequence, sequence, item.WorldTime, batchHash, nowText}},
		{"scheduler event", `INSERT INTO events(event_id, batch_id, instance_id, branch_id, event_sequence, batch_index, event_type, actor_id, world_time, payload) VALUES (?, ?, ?, ?, ?, 0, ?, 'system', ?, ?)`, []any{eventID, batchID, instanceID, branchID, sequence, mutation.EventType, item.WorldTime, string(eventPayload)}},
	}
	for _, statement := range statements {
		if _, err := tx.conn.ExecContext(ctx, statement.query, statement.args...); err != nil {
			return core.WrapError(core.CodeStorageFailure, "insert "+statement.name, err)
		}
	}
	if len(mutation.Postings) > 0 {
		entryID := "journal_" + item.SchedulerItemID
		if _, err := tx.conn.ExecContext(ctx, `INSERT INTO journal_entries(entry_id, event_id, status, purpose) VALUES (?, ?, 'draft', ?)`, entryID, eventID, mutation.EventType); err != nil {
			return core.WrapError(core.CodeStorageFailure, "insert scheduler journal", err)
		}
		for index, posting := range mutation.Postings {
			if _, err := tx.conn.ExecContext(ctx, `INSERT INTO postings(posting_id, entry_id, account_id, currency_id, amount_minor, memo) VALUES (?, ?, ?, ?, ?, ?)`, fmt.Sprintf("posting_%s_%02d", item.SchedulerItemID, index), entryID, posting.AccountID, posting.CurrencyID, posting.Amount, posting.Memo); err != nil {
				return core.WrapError(core.CodeStorageFailure, "insert scheduler posting", err)
			}
		}
		if _, err := tx.conn.ExecContext(ctx, `UPDATE journal_entries SET status = 'posted' WHERE entry_id = ? AND status = 'draft'`, entryID); err != nil {
			return core.WrapError(core.CodeStorageFailure, "post scheduler journal", err)
		}
	}
	for index, movement := range mutation.Movements {
		if _, err := tx.conn.ExecContext(ctx, `INSERT INTO stock_movements(movement_id, event_id, sku_id, from_location_id, to_location_id, quantity_minor, movement_kind, reason_code, capability_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, fmt.Sprintf("movement_%s_%02d", item.SchedulerItemID, index), eventID, movement.SKUID, movement.FromLocation, movement.ToLocation, movement.Quantity, movement.Kind, movement.Reason, movement.CapabilityID); err != nil {
			return core.WrapError(core.CodeStorageFailure, "insert scheduler stock movement", err)
		}
	}
	for _, balance := range mutation.Balances {
		result, err := tx.conn.ExecContext(ctx, `UPDATE account_balances SET balance_minor = ?, projection_version = projection_version + 1, last_event_sequence = ? WHERE account_id = ? AND projection_version = ?`, balance.NewBalance, sequence, balance.AccountID, balance.ExpectedVersion)
		if err != nil {
			return core.WrapError(core.CodeStorageFailure, "update scheduled account projection", err)
		}
		if rows, _ := result.RowsAffected(); rows != 1 {
			return core.NewError(core.CodeBranchConflict, "scheduled account projection compare-and-swap failed")
		}
	}
	for _, inventory := range mutation.Inventory {
		result, err := tx.conn.ExecContext(ctx, `UPDATE inventory_balances SET quantity_minor = ?, projection_version = projection_version + 1, last_event_sequence = ? WHERE location_id = ? AND sku_id = ? AND projection_version = ?`, inventory.NewQuantity, sequence, inventory.LocationID, inventory.SKUID, inventory.ExpectedVersion)
		if err != nil {
			return core.WrapError(core.CodeStorageFailure, "update scheduled inventory projection", err)
		}
		if rows, _ := result.RowsAffected(); rows != 1 {
			return core.NewError(core.CodeBranchConflict, "scheduled inventory projection compare-and-swap failed")
		}
	}
	if mutation.ApplyDomainRows != nil {
		if err := mutation.ApplyDomainRows(ctx, tx.conn, eventID, sequence); err != nil {
			return err
		}
	}
	result, err := tx.conn.ExecContext(ctx, `UPDATE scheduler_items SET status = 'completed' WHERE scheduler_item_id = ? AND status = 'pending'`, item.SchedulerItemID)
	if err != nil {
		return core.WrapError(core.CodeStorageFailure, "complete scheduler item", err)
	}
	if rows, _ := result.RowsAffected(); rows != 1 {
		return core.NewError(core.CodeBranchConflict, "scheduler item was claimed by another writer")
	}
	clockStatus := "running"
	if payload.Kind == "clock_checkpoint" && payload.Day == 90 {
		clockStatus = "completed"
	}
	result, err = tx.conn.ExecContext(ctx, `UPDATE world_clocks SET current_world_time = ?, current_day = ?, status = ?, projection_version = projection_version + 1, last_event_sequence = ? WHERE instance_id = ? AND branch_id = ? AND current_world_time <= ?`, item.WorldTime, payload.Day, clockStatus, sequence, instanceID, branchID, item.WorldTime)
	if err != nil {
		return core.WrapError(core.CodeStorageFailure, "advance world clock", err)
	}
	if rows, _ := result.RowsAffected(); rows != 1 {
		return core.NewError(core.CodeBranchConflict, "world clock compare-and-swap failed")
	}
	result, err = tx.conn.ExecContext(ctx, `UPDATE branches SET head_sequence = ? WHERE instance_id = ? AND branch_id = ? AND head_sequence = ?`, sequence, instanceID, branchID, head)
	if err != nil {
		return core.WrapError(core.CodeStorageFailure, "advance scheduler branch head", err)
	}
	if rows, _ := result.RowsAffected(); rows != 1 {
		return core.NewError(core.CodeBranchConflict, "scheduler Branch Head compare-and-swap failed")
	}
	audience := fmt.Sprintf(`{"instance_id":%q,"kind":"instance"}`, instanceID)
	audienceHash, err := core.HashJSON(struct {
		InstanceID string `json:"instance_id"`
		Kind       string `json:"kind"`
	}{InstanceID: instanceID, Kind: "instance"})
	if err != nil {
		return err
	}
	if !mutation.Private {
		if _, err := tx.conn.ExecContext(ctx, `INSERT INTO outbox(outbox_id, event_id, topic, audience_scope, audience_scope_hash, payload) VALUES (?, ?, 'scheduler.event', ?, ?, ?)`, outboxID, eventID, audience, audienceHash, string(eventPayload)); err != nil {
			return core.WrapError(core.CodeStorageFailure, "insert scheduler Outbox", err)
		}
	}
	if _, err := tx.conn.ExecContext(ctx, `UPDATE command_attempts SET status = 'committed', finished_at_utc = ? WHERE command_id = ? AND attempt_no = 1 AND status = 'ready'`, nowText, commandID); err != nil {
		return core.WrapError(core.CodeStorageFailure, "commit scheduler attempt", err)
	}
	if _, err := tx.conn.ExecContext(ctx, `UPDATE commands SET status = 'committed' WHERE command_id = ? AND status = 'pending'`, commandID); err != nil {
		return core.WrapError(core.CodeStorageFailure, "commit scheduler command", err)
	}
	return nil
}

func prepareWageAccrual(ctx context.Context, conn *sql.Conn, payload scheduledPayload) (scheduledMutation, error) {
	var amount, period int64
	if err := conn.QueryRowContext(ctx, `SELECT gross_wage_minor, pay_period_days FROM employment_contracts WHERE contract_id = ? AND status = 'active'`, payload.SubjectID).Scan(&amount, &period); err != nil {
		return scheduledMutation{}, classifyMissing(err, "employment contract")
	}
	return prepareWageAccrualForPeriod(ctx, conn, wageAccrualPeriod{
		ContractID: payload.SubjectID, StartDay: int64(payload.Day) - period,
		EndDay: payload.Day, DueWorldTime: strictDayTime(payload.Day), AmountMinor: amount,
	})
}

// wageAccrualPeriod is an internal, already-authorized earned-period snapshot.
// The caller resolves effective contract terms and chronology; this accounting
// boundary does not authorize employment changes or infer an epoch from a day.
type wageAccrualPeriod struct {
	ContractID   string
	StartDay     int64
	EndDay       int
	DueWorldTime string
	AmountMinor  int64
}

func prepareWageAccrualForPeriod(ctx context.Context, conn *sql.Conn, period wageAccrualPeriod) (scheduledMutation, error) {
	if period.ContractID == "" || period.StartDay < 0 || int64(period.EndDay) <= period.StartDay || period.AmountMinor <= 0 {
		return scheduledMutation{}, core.NewError(core.CodeInvalidArgument, "invalid earned wage period")
	}
	due, err := time.Parse(time.RFC3339, period.DueWorldTime)
	if err != nil {
		return scheduledMutation{}, core.NewError(core.CodeInvalidArgument, "invalid wage due time")
	}
	period.DueWorldTime = due.UTC().Format(time.RFC3339Nano)
	obligationID := fmt.Sprintf("wage_%s_%d_%d", period.ContractID, period.StartDay, period.EndDay)
	var existing int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM wage_obligations WHERE obligation_id = ?`, obligationID).Scan(&existing); err != nil {
		return scheduledMutation{}, core.WrapError(core.CodeStorageFailure, "check wage obligation idempotency", err)
	}
	if existing == 1 {
		return scheduledMutation{EventType: "SchedulerSkipLogged", EventPayload: struct {
			ObligationID string `json:"obligation_id"`
			Reason       string `json:"reason"`
		}{obligationID, "wage_already_accrued"}}, nil
	}
	ledger, err := readObligationLedger(ctx, conn, "wage", period.ContractID)
	if err != nil {
		return scheduledMutation{}, err
	}
	postings, balances, err := prepareAccrualAccounting(ctx, conn, ledger, period.AmountMinor, "wage accrual")
	if err != nil {
		return scheduledMutation{}, err
	}
	eventPayload := struct {
		ObligationID string `json:"obligation_id"`
		ContractID   string `json:"contract_id"`
		PeriodStart  int64  `json:"period_start_day"`
		PeriodEnd    int    `json:"period_end_day"`
		AmountMinor  int64  `json:"amount_minor"`
	}{obligationID, period.ContractID, period.StartDay, period.EndDay, period.AmountMinor}
	return scheduledMutation{
		EventType: "WageObligationAccrued", EventPayload: eventPayload, Postings: postings, Balances: balances,
		ApplyDomainRows: func(ctx context.Context, conn *sql.Conn, _ string, sequence int64) error {
			_, err := conn.ExecContext(ctx, `INSERT INTO wage_obligations(obligation_id, contract_id, period_start_day, period_end_day, due_world_time, amount_due_minor, amount_paid_minor, status, last_event_sequence) VALUES (?, ?, ?, ?, ?, ?, 0, 'accrued', ?)`, obligationID, period.ContractID, period.StartDay, period.EndDay, period.DueWorldTime, period.AmountMinor, sequence)
			if err != nil {
				return core.WrapError(core.CodeStorageFailure, "accrue wage obligation", err)
			}
			return nil
		},
	}, nil
}

func prepareWagePayment(ctx context.Context, conn *sql.Conn, payload scheduledPayload) (scheduledMutation, error) {
	var period int
	if err := conn.QueryRowContext(ctx, `SELECT pay_period_days FROM employment_contracts WHERE contract_id = ? AND status = 'active'`, payload.SubjectID).Scan(&period); err != nil {
		return scheduledMutation{}, classifyMissing(err, "employment contract")
	}
	start := payload.Day - period
	obligationID := fmt.Sprintf("wage_%s_%d_%d", payload.SubjectID, start, payload.Day)
	return prepareWageSettlement(ctx, conn, payload, obligationID)
}

func prepareWageRetry(ctx context.Context, conn *sql.Conn, payload scheduledPayload) (scheduledMutation, error) {
	return prepareWageSettlement(ctx, conn, payload, payload.SubjectID)
}

func prepareWageSettlement(ctx context.Context, conn *sql.Conn, payload scheduledPayload, obligationID string) (scheduledMutation, error) {
	var contractID, employerAccount, employeeAccount, currency string
	var due, paid int64
	if err := conn.QueryRowContext(ctx, `
		SELECT c.contract_id, c.employer_account_id, c.employee_account_id, c.currency_id, o.amount_due_minor, o.amount_paid_minor
		FROM wage_obligations o JOIN employment_contracts c ON c.contract_id = o.contract_id
		WHERE o.obligation_id = ?`, obligationID).Scan(&contractID, &employerAccount, &employeeAccount, &currency, &due, &paid); err != nil {
		return scheduledMutation{}, classifyMissing(err, "wage obligation")
	}
	remaining := due - paid
	if remaining == 0 {
		return scheduledMutation{EventType: "SchedulerSkipLogged", EventPayload: struct {
			ObligationID string `json:"obligation_id"`
			Reason       string `json:"reason"`
		}{obligationID, "wage_already_paid"}}, nil
	}
	employerBalance, employerVersion, err := readScheduledBalance(ctx, conn, employerAccount)
	if err != nil {
		return scheduledMutation{}, err
	}
	employeeBalance, employeeVersion, err := readScheduledBalance(ctx, conn, employeeAccount)
	if err != nil {
		return scheduledMutation{}, err
	}
	ledger, err := readObligationLedger(ctx, conn, "wage", contractID)
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
	payment := remaining
	if employerBalance < payment {
		payment = employerBalance
	}
	if payment < 0 {
		payment = 0
	}
	status := "paid"
	eventType := "WagePaid"
	settlementStatus := "paid"
	reasonCode := "settled"
	if payment < remaining {
		status = "arrears"
		eventType = "WageArrearsRecorded"
		settlementStatus = "partial"
		reasonCode = "insufficient_funds"
		if payment == 0 {
			settlementStatus = "failed"
		}
	}
	mutation := scheduledMutation{EventType: eventType}
	if payment > 0 {
		newEmployer, _ := checkedSubtract(employerBalance, payment)
		newEmployee, ok := checkedAdd(employeeBalance, payment)
		if !ok {
			return scheduledMutation{}, core.NewError(core.CodeIntegerOverflow, "employee wage balance overflows")
		}
		newPayable, ok := checkedAdd(payableBalance, payment)
		if !ok || newPayable > 0 {
			return scheduledMutation{}, core.NewError(core.CodeProjectionDiverged, "wage payable cannot be cleared beyond the accrued amount")
		}
		newReceivable, ok := checkedSubtract(receivableBalance, payment)
		if !ok || newReceivable < 0 {
			return scheduledMutation{}, core.NewError(core.CodeProjectionDiverged, "wage receivable cannot be cleared beyond the accrued amount")
		}
		mutation.Postings = []scheduledPosting{
			{ledger.PayableAccountID, currency, payment, "clear wage payable"},
			{employerAccount, currency, -payment, "wage cash payment"},
			{employeeAccount, currency, payment, "wage cash receipt"},
			{ledger.ReceivableAccountID, currency, -payment, "clear wage receivable"},
		}
		mutation.Balances = []balanceMutation{
			{ledger.PayableAccountID, payableVersion, newPayable},
			{employerAccount, employerVersion, newEmployer},
			{employeeAccount, employeeVersion, newEmployee},
			{ledger.ReceivableAccountID, receivableVersion, newReceivable},
		}
	}
	mutation.EventPayload = struct {
		ObligationID   string `json:"obligation_id"`
		PaidMinor      int64  `json:"paid_minor"`
		RemainingMinor int64  `json:"remaining_minor"`
		Status         string `json:"status"`
	}{obligationID, payment, remaining - payment, status}
	settlementID := fmt.Sprintf("settlement_wage_%s_day_%03d", obligationID, payload.Day)
	mutation.ApplyDomainRows = func(ctx context.Context, conn *sql.Conn, eventID string, sequence int64) error {
		_, err := conn.ExecContext(ctx, `UPDATE wage_obligations SET amount_paid_minor = amount_paid_minor + ?, status = ?, last_event_sequence = ? WHERE obligation_id = ?`, payment, status, sequence, obligationID)
		if err != nil {
			return core.WrapError(core.CodeStorageFailure, "settle wage obligation", err)
		}
		if _, err := conn.ExecContext(ctx, `INSERT INTO obligation_settlements(settlement_id, obligation_kind, obligation_id, attempted_minor, paid_minor, remaining_minor, status, reason_code, event_id, event_sequence) VALUES (?, 'wage', ?, ?, ?, ?, ?, ?, ?, ?)`, settlementID, obligationID, remaining, payment, remaining-payment, settlementStatus, reasonCode, eventID, sequence); err != nil {
			return core.WrapError(core.CodeStorageFailure, "record wage settlement", err)
		}
		return nil
	}
	return mutation, nil
}

func prepareRentAccrual(ctx context.Context, conn *sql.Conn, payload scheduledPayload) (scheduledMutation, error) {
	var amount, period, grace int64
	if err := conn.QueryRowContext(ctx, `SELECT rent_minor, period_days, grace_days FROM rent_contracts WHERE contract_id = ? AND status = 'active'`, payload.SubjectID).Scan(&amount, &period, &grace); err != nil {
		return scheduledMutation{}, classifyMissing(err, "rent contract")
	}
	start := int64(payload.Day) - period
	obligationID := fmt.Sprintf("rent_%s_%d_%d", payload.SubjectID, start, payload.Day)
	var existing int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM rent_obligations WHERE obligation_id = ?`, obligationID).Scan(&existing); err != nil {
		return scheduledMutation{}, core.WrapError(core.CodeStorageFailure, "check rent obligation idempotency", err)
	}
	if existing == 1 {
		return scheduledMutation{EventType: "SchedulerSkipLogged", EventPayload: struct {
			ObligationID string `json:"obligation_id"`
			Reason       string `json:"reason"`
		}{obligationID, "rent_already_accrued"}}, nil
	}
	ledger, err := readObligationLedger(ctx, conn, "rent", payload.SubjectID)
	if err != nil {
		return scheduledMutation{}, err
	}
	postings, balances, err := prepareAccrualAccounting(ctx, conn, ledger, amount, "rent accrual")
	if err != nil {
		return scheduledMutation{}, err
	}
	mutation := scheduledMutation{EventType: "RentObligationAccrued", Postings: postings, Balances: balances}
	mutation.EventPayload = struct {
		ObligationID string `json:"obligation_id"`
		ContractID   string `json:"contract_id"`
		AmountMinor  int64  `json:"amount_minor"`
		GraceDays    int64  `json:"grace_days"`
	}{obligationID, payload.SubjectID, amount, grace}
	mutation.ApplyDomainRows = func(ctx context.Context, conn *sql.Conn, _ string, sequence int64) error {
		_, err := conn.ExecContext(ctx, `INSERT INTO rent_obligations(obligation_id, contract_id, period_start_day, period_end_day, due_world_time, grace_until_world_time, amount_due_minor, amount_paid_minor, status, last_event_sequence) VALUES (?, ?, ?, ?, ?, ?, ?, 0, 'due', ?)`, obligationID, payload.SubjectID, start, payload.Day, strictDayTime(payload.Day), strictDayTime(payload.Day+int(grace)), amount, sequence)
		if err != nil {
			return core.WrapError(core.CodeStorageFailure, "accrue rent obligation", err)
		}
		return nil
	}
	return mutation, nil
}

func prepareRentPayment(ctx context.Context, conn *sql.Conn, payload scheduledPayload) (scheduledMutation, error) {
	var period int
	if err := conn.QueryRowContext(ctx, `SELECT period_days FROM rent_contracts WHERE contract_id = ? AND status = 'active'`, payload.SubjectID).Scan(&period); err != nil {
		return scheduledMutation{}, classifyMissing(err, "rent contract")
	}
	start := payload.Day - period
	obligationID := fmt.Sprintf("rent_%s_%d_%d", payload.SubjectID, start, payload.Day)
	var tenantAccount, landlordAccount, currency string
	var due, paid int64
	if err := conn.QueryRowContext(ctx, `
		SELECT c.tenant_account_id, c.landlord_account_id, c.currency_id, o.amount_due_minor, o.amount_paid_minor
		FROM rent_obligations o JOIN rent_contracts c ON c.contract_id = o.contract_id
		WHERE o.obligation_id = ?`, obligationID).Scan(&tenantAccount, &landlordAccount, &currency, &due, &paid); err != nil {
		return scheduledMutation{}, classifyMissing(err, "rent obligation")
	}
	tenantBalance, tenantVersion, err := readScheduledBalance(ctx, conn, tenantAccount)
	if err != nil {
		return scheduledMutation{}, err
	}
	ledger, err := readObligationLedger(ctx, conn, "rent", payload.SubjectID)
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
	landlordBalance, landlordVersion, err := readScheduledBalance(ctx, conn, landlordAccount)
	if err != nil {
		return scheduledMutation{}, err
	}
	remaining := due - paid
	payment := remaining
	if tenantBalance < payment {
		payment = tenantBalance
	}
	if payment < 0 {
		payment = 0
	}
	status := "paid"
	eventType := "RentPaid"
	settlementStatus := "paid"
	reasonCode := "settled"
	if payment < remaining {
		status = "partially_paid"
		eventType = "RentPaymentFailed"
		settlementStatus = "partial"
		reasonCode = "insufficient_funds"
		if payment == 0 {
			settlementStatus = "failed"
		}
	}
	mutation := scheduledMutation{EventType: eventType}
	if payment > 0 {
		newTenant, _ := checkedSubtract(tenantBalance, payment)
		newLandlord, ok := checkedAdd(landlordBalance, payment)
		if !ok {
			return scheduledMutation{}, core.NewError(core.CodeIntegerOverflow, "landlord rent balance overflows")
		}
		newPayable, ok := checkedAdd(payableBalance, payment)
		if !ok || newPayable > 0 {
			return scheduledMutation{}, core.NewError(core.CodeProjectionDiverged, "rent payable cannot be cleared beyond the accrued amount")
		}
		newReceivable, ok := checkedSubtract(receivableBalance, payment)
		if !ok || newReceivable < 0 {
			return scheduledMutation{}, core.NewError(core.CodeProjectionDiverged, "rent receivable cannot be cleared beyond the accrued amount")
		}
		mutation.Postings = []scheduledPosting{
			{ledger.PayableAccountID, currency, payment, "clear rent payable"},
			{tenantAccount, currency, -payment, "rent cash payment"},
			{landlordAccount, currency, payment, "rent cash receipt"},
			{ledger.ReceivableAccountID, currency, -payment, "clear rent receivable"},
		}
		mutation.Balances = []balanceMutation{
			{ledger.PayableAccountID, payableVersion, newPayable},
			{tenantAccount, tenantVersion, newTenant},
			{landlordAccount, landlordVersion, newLandlord},
			{ledger.ReceivableAccountID, receivableVersion, newReceivable},
		}
	}
	mutation.EventPayload = struct {
		ObligationID   string `json:"obligation_id"`
		PaidMinor      int64  `json:"paid_minor"`
		RemainingMinor int64  `json:"remaining_minor"`
		Status         string `json:"status"`
	}{obligationID, payment, remaining - payment, status}
	settlementID := fmt.Sprintf("settlement_rent_%s_day_%03d", obligationID, payload.Day)
	mutation.ApplyDomainRows = func(ctx context.Context, conn *sql.Conn, eventID string, sequence int64) error {
		_, err := conn.ExecContext(ctx, `UPDATE rent_obligations SET amount_paid_minor = amount_paid_minor + ?, status = ?, last_event_sequence = ? WHERE obligation_id = ?`, payment, status, sequence, obligationID)
		if err != nil {
			return core.WrapError(core.CodeStorageFailure, "settle rent obligation", err)
		}
		if _, err := conn.ExecContext(ctx, `INSERT INTO obligation_settlements(settlement_id, obligation_kind, obligation_id, attempted_minor, paid_minor, remaining_minor, status, reason_code, event_id, event_sequence) VALUES (?, 'rent', ?, ?, ?, ?, ?, ?, ?, ?)`, settlementID, obligationID, remaining, payment, remaining-payment, settlementStatus, reasonCode, eventID, sequence); err != nil {
			return core.WrapError(core.CodeStorageFailure, "record rent settlement", err)
		}
		return nil
	}
	return mutation, nil
}

func prepareRentPastDue(ctx context.Context, conn *sql.Conn, payload scheduledPayload) (scheduledMutation, error) {
	var period, grace int
	if err := conn.QueryRowContext(ctx, `SELECT period_days, grace_days FROM rent_contracts WHERE contract_id = ? AND status = 'active'`, payload.SubjectID).Scan(&period, &grace); err != nil {
		return scheduledMutation{}, classifyMissing(err, "rent contract")
	}
	start := payload.Day - grace - period
	end := payload.Day - grace
	obligationID := fmt.Sprintf("rent_%s_%d_%d", payload.SubjectID, start, end)
	var due, paid int64
	var status string
	if err := conn.QueryRowContext(ctx, `SELECT amount_due_minor, amount_paid_minor, status FROM rent_obligations WHERE obligation_id = ?`, obligationID).Scan(&due, &paid, &status); err != nil {
		return scheduledMutation{}, classifyMissing(err, "rent obligation")
	}
	mutation := scheduledMutation{}
	if paid < due {
		mutation.EventType = "RentPastDue"
		mutation.EventPayload = struct {
			ObligationID   string `json:"obligation_id"`
			RemainingMinor int64  `json:"remaining_minor"`
		}{obligationID, due - paid}
		mutation.ApplyDomainRows = func(ctx context.Context, conn *sql.Conn, _ string, sequence int64) error {
			_, err := conn.ExecContext(ctx, `UPDATE rent_obligations SET status = 'past_due', last_event_sequence = ? WHERE obligation_id = ? AND amount_paid_minor < amount_due_minor`, sequence, obligationID)
			return err
		}
	} else {
		mutation.EventType = "SchedulerSkipLogged"
		mutation.EventPayload = struct {
			ObligationID string `json:"obligation_id"`
			Reason       string `json:"reason"`
		}{obligationID, "rent_already_paid"}
	}
	return mutation, nil
}

func prepareHouseholdPurchase(ctx context.Context, conn *sql.Conn, payload scheduledPayload) (scheduledMutation, error) {
	var sellerAccount, sellerLocation, skuID, currency string
	var unitPrice int64
	if err := conn.QueryRowContext(ctx, `SELECT seller_account_id, seller_location_id, sku_id, currency_id, unit_price_minor FROM market_quotes WHERE quote_id = 'quote_bread_90d' AND valid_from_day <= ? AND valid_until_day >= ?`, payload.Day, payload.Day).Scan(&sellerAccount, &sellerLocation, &skuID, &currency, &unitPrice); err != nil {
		return scheduledMutation{}, classifyMissing(err, "active market quote")
	}
	var budgetStart, budgetEnd int
	var foodLimit int64
	if err := conn.QueryRowContext(ctx, `SELECT period_start_day, period_end_day, food_limit_minor FROM household_budgets WHERE household_entity_id = ? AND period_start_day <= ? AND period_end_day > ?`, payload.SubjectID, payload.Day, payload.Day).Scan(&budgetStart, &budgetEnd, &foodLimit); err != nil {
		return scheduledMutation{}, classifyMissing(err, "household budget")
	}
	var foodSpent int64
	if err := conn.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(CAST(json_extract(payload, '$.total_minor') AS INTEGER)), 0)
		FROM events
		WHERE instance_id = ? AND branch_id = ? AND event_type = 'HouseholdPurchaseCompleted'
		  AND json_extract(payload, '$.buyer_account_id') = ?
		  AND world_time >= ? AND world_time < ?`,
		DemoInstanceID, DemoBranchID, payload.AccountID, strictDayTime(budgetStart), strictDayTime(budgetEnd),
	).Scan(&foodSpent); err != nil {
		return scheduledMutation{}, core.WrapError(core.CodeStorageFailure, "read household food spending", err)
	}
	nextSpend, ok := checkedAdd(foodSpent, unitPrice)
	if !ok {
		return scheduledMutation{}, core.NewError(core.CodeIntegerOverflow, "household food spending overflows")
	}
	if nextSpend > foodLimit {
		return scheduledMutation{EventType: "PurchaseFailed", EventPayload: struct {
			BuyerAccountID       string `json:"buyer_account_id"`
			Reason               string `json:"reason"`
			RequiredMinor        int64  `json:"required_minor"`
			RemainingBudgetMinor int64  `json:"remaining_budget_minor"`
		}{payload.AccountID, "budget_exceeded", unitPrice, foodLimit - foodSpent}}, nil
	}
	buyerBalance, buyerVersion, err := readScheduledBalance(ctx, conn, payload.AccountID)
	if err != nil {
		return scheduledMutation{}, err
	}
	sellerBalance, sellerVersion, err := readScheduledBalance(ctx, conn, sellerAccount)
	if err != nil {
		return scheduledMutation{}, err
	}
	buyerStock, buyerStockVersion, err := readScheduledInventory(ctx, conn, payload.LocationID, skuID)
	if err != nil {
		return scheduledMutation{}, err
	}
	sellerStock, sellerStockVersion, err := readScheduledInventory(ctx, conn, sellerLocation, skuID)
	if err != nil {
		return scheduledMutation{}, err
	}
	if buyerBalance < unitPrice {
		return scheduledMutation{EventType: "PurchaseFailed", EventPayload: struct {
			BuyerAccountID string `json:"buyer_account_id"`
			Reason         string `json:"reason"`
			RequiredMinor  int64  `json:"required_minor"`
		}{payload.AccountID, "insufficient_funds", unitPrice}}, nil
	}
	if sellerStock < 1 {
		return scheduledMutation{EventType: "PurchaseFailed", EventPayload: struct {
			BuyerAccountID string `json:"buyer_account_id"`
			Reason         string `json:"reason"`
			RequiredMinor  int64  `json:"required_minor"`
		}{payload.AccountID, "insufficient_stock", unitPrice}}, nil
	}
	newSellerBalance, ok := checkedAdd(sellerBalance, unitPrice)
	if !ok {
		return scheduledMutation{}, core.NewError(core.CodeIntegerOverflow, "store balance overflows")
	}
	newBuyerStock, ok := checkedAdd(buyerStock, 1)
	if !ok {
		return scheduledMutation{}, core.NewError(core.CodeIntegerOverflow, "household inventory overflows")
	}
	return scheduledMutation{
		EventType: "HouseholdPurchaseCompleted",
		EventPayload: struct {
			BuyerAccountID string `json:"buyer_account_id"`
			SKUID          string `json:"sku_id"`
			Quantity       int64  `json:"quantity_minor"`
			TotalMinor     int64  `json:"total_minor"`
		}{payload.AccountID, skuID, 1, unitPrice},
		Postings:  []scheduledPosting{{payload.AccountID, currency, -unitPrice, "household food purchase"}, {sellerAccount, currency, unitPrice, "store sale"}},
		Movements: []scheduledMovement{{skuID, sellerLocation, payload.LocationID, 1, "transfer", "household_purchase", nil}},
		Balances:  []balanceMutation{{payload.AccountID, buyerVersion, buyerBalance - unitPrice}, {sellerAccount, sellerVersion, newSellerBalance}},
		Inventory: []inventoryMutation{{payload.LocationID, skuID, buyerStockVersion, newBuyerStock}, {sellerLocation, skuID, sellerStockVersion, sellerStock - 1}},
	}, nil
}

func prepareStoreRestock(ctx context.Context, conn *sql.Conn, payload scheduledPayload) (scheduledMutation, error) {
	const supplierCost int64 = 100
	quantity, version, err := readScheduledInventory(ctx, conn, payload.LocationID, DemoSKUID)
	if err != nil {
		return scheduledMutation{}, err
	}
	newQuantity, ok := checkedAdd(quantity, 5)
	if !ok {
		return scheduledMutation{}, core.NewError(core.CodeIntegerOverflow, "store restock overflows")
	}
	storeBalance, storeVersion, err := readScheduledBalance(ctx, conn, payload.AccountID)
	if err != nil {
		return scheduledMutation{}, err
	}
	if storeBalance < supplierCost {
		return scheduledMutation{EventType: "RestockFailed", EventPayload: struct {
			LocationID    string `json:"location_id"`
			RequiredMinor int64  `json:"required_minor"`
			Reason        string `json:"reason"`
		}{payload.LocationID, supplierCost, "insufficient_funds"}}, nil
	}
	employerBalance, employerVersion, err := readScheduledBalance(ctx, conn, DemoEmployerAccountID)
	if err != nil {
		return scheduledMutation{}, err
	}
	newEmployerBalance, ok := checkedAdd(employerBalance, supplierCost)
	if !ok {
		return scheduledMutation{}, core.NewError(core.CodeIntegerOverflow, "supplier balance overflows")
	}
	capability := "cap_inventory_create"
	return scheduledMutation{
		EventType: "StoreRestocked",
		EventPayload: struct {
			LocationID        string `json:"location_id"`
			SKUID             string `json:"sku_id"`
			Quantity          int64  `json:"quantity_minor"`
			SupplierCostMinor int64  `json:"supplier_cost_minor"`
		}{payload.LocationID, DemoSKUID, 5, supplierCost},
		Postings:  []scheduledPosting{{payload.AccountID, DemoCurrencyID, -supplierCost, "store restock supplier payment"}, {DemoEmployerAccountID, DemoCurrencyID, supplierCost, "enterprise supply revenue"}},
		Balances:  []balanceMutation{{payload.AccountID, storeVersion, storeBalance - supplierCost}, {DemoEmployerAccountID, employerVersion, newEmployerBalance}},
		Movements: []scheduledMovement{{DemoSKUID, DemoSourceLocationID, payload.LocationID, 5, "create", "scheduled_restock", &capability}},
		Inventory: []inventoryMutation{{payload.LocationID, DemoSKUID, version, newQuantity}},
	}, nil
}

func readObligationLedger(ctx context.Context, conn *sql.Conn, kind, contractID string) (obligationLedger, error) {
	var ledger obligationLedger
	err := conn.QueryRowContext(ctx, `
		SELECT expense_account_id, payable_account_id, receivable_account_id, income_account_id, currency_id
		FROM obligation_ledger_accounts
		WHERE obligation_kind = ? AND contract_id = ?`, kind, contractID,
	).Scan(&ledger.ExpenseAccountID, &ledger.PayableAccountID, &ledger.ReceivableAccountID, &ledger.IncomeAccountID, &ledger.CurrencyID)
	if err != nil {
		return obligationLedger{}, classifyMissing(err, kind+" obligation ledger")
	}
	return ledger, nil
}

func prepareAccrualAccounting(ctx context.Context, conn *sql.Conn, ledger obligationLedger, amount int64, memo string) ([]scheduledPosting, []balanceMutation, error) {
	accountIDs := []string{ledger.ExpenseAccountID, ledger.PayableAccountID, ledger.ReceivableAccountID, ledger.IncomeAccountID}
	deltas := []int64{amount, -amount, amount, -amount}
	postings := make([]scheduledPosting, 0, len(accountIDs))
	balances := make([]balanceMutation, 0, len(accountIDs))
	for index, accountID := range accountIDs {
		balance, version, err := readScheduledBalance(ctx, conn, accountID)
		if err != nil {
			return nil, nil, err
		}
		updated, ok := checkedAdd(balance, deltas[index])
		if !ok {
			return nil, nil, core.NewError(core.CodeIntegerOverflow, memo+" balance overflows")
		}
		postings = append(postings, scheduledPosting{accountID, ledger.CurrencyID, deltas[index], memo})
		balances = append(balances, balanceMutation{accountID, version, updated})
	}
	return postings, balances, nil
}

func readScheduledBalance(ctx context.Context, conn *sql.Conn, accountID string) (balance, version int64, err error) {
	err = conn.QueryRowContext(ctx, `SELECT balance_minor, projection_version FROM account_balances WHERE account_id = ?`, accountID).Scan(&balance, &version)
	if err != nil {
		err = classifyMissing(err, "scheduled account")
	}
	return
}

func readScheduledInventory(ctx context.Context, conn *sql.Conn, locationID, skuID string) (quantity, version int64, err error) {
	err = conn.QueryRowContext(ctx, `SELECT quantity_minor, projection_version FROM inventory_balances WHERE location_id = ? AND sku_id = ?`, locationID, skuID).Scan(&quantity, &version)
	if err != nil {
		err = classifyMissing(err, "scheduled inventory")
	}
	return
}

func stableSchedulerItemIDs(items []SchedulerItem) []string {
	ids := make([]string, len(items))
	for index, item := range items {
		ids[index] = item.SchedulerItemID
	}
	sort.Strings(ids)
	return ids
}
