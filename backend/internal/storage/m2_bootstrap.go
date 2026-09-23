package storage

import (
	"context"
	"fmt"

	"corerp.local/backend/internal/core"
)

const (
	M2DemoInstanceID               = "inst_m2_t09"
	M2DemoBranchID                 = "br_main"
	M2DemoEpochID                  = "epoch_0"
	M2DemoCohortID                 = "cohort_block_a"
	M2DemoCurrencyID               = "m2_credit"
	M2DemoSKUID                    = "m2_staple"
	M2DemoCohortAssetAccountID     = "account_m2_cohort_asset"
	M2DemoCohortReceivableID       = "account_m2_cohort_receivable"
	M2DemoCohortLiabilityID        = "account_m2_cohort_liability"
	M2DemoIssuanceAccountID        = "account_m2_issuance"
	M2DemoCounterpartyPayableID    = "account_m2_counterparty_payable"
	M2DemoCounterpartyReceivableID = "account_m2_counterparty_receivable"
	M2DemoCohortLocationID         = "location_m2_cohort"
	M2DemoSourceLocationID         = "location_m2_source"
)

func (s *Store) BootstrapM2Demo(ctx context.Context) error {
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return core.WrapError(core.CodeStorageFailure, "begin M2 bootstrap", err)
	}
	defer tx.Rollback(ctx)

	rulesetHash, err := core.HashJSON(struct {
		Ruleset string `json:"ruleset"`
		Version string `json:"version"`
	}{Ruleset: "m2-t09-demo", Version: "0"})
	if err != nil {
		return err
	}
	if _, err := tx.conn.ExecContext(ctx, `INSERT OR IGNORE INTO world_instances(instance_id, world_definition_id, world_definition_version, created_at_utc, lifecycle_state) VALUES (?, 'corerp.m2.t09.demo', '0.1.0', '2026-09-22T00:00:00Z', 'active')`, M2DemoInstanceID); err != nil {
		return core.WrapError(core.CodeStorageFailure, "seed M2 instance", err)
	}
	if _, err := tx.conn.ExecContext(ctx, `INSERT OR IGNORE INTO branches(instance_id, branch_id, label, head_sequence, created_at_utc) VALUES (?, ?, 'main', 0, '2026-09-22T00:00:00Z')`, M2DemoInstanceID, M2DemoBranchID); err != nil {
		return core.WrapError(core.CodeStorageFailure, "seed M2 branch", err)
	}
	if _, err := tx.conn.ExecContext(ctx, `
		INSERT INTO rule_epochs(instance_id, branch_id, epoch_id, start_sequence, end_sequence, ruleset_hash, lock_document)
		SELECT ?, ?, ?, 1, NULL, ?, '{"ruleset":"m2-t09-demo","version":"0"}'
		WHERE NOT EXISTS (SELECT 1 FROM rule_epochs WHERE instance_id = ? AND branch_id = ? AND epoch_id = ?)`,
		M2DemoInstanceID, M2DemoBranchID, M2DemoEpochID, rulesetHash,
		M2DemoInstanceID, M2DemoBranchID, M2DemoEpochID,
	); err != nil {
		return core.WrapError(core.CodeStorageFailure, "seed M2 Rule Epoch", err)
	}

	var head int64
	if err := tx.conn.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id = ? AND branch_id = ?`, M2DemoInstanceID, M2DemoBranchID).Scan(&head); err != nil {
		return core.WrapError(core.CodeStorageFailure, "verify M2 bootstrap branch", err)
	}
	if head == 0 {
		if err := s.insertM2Genesis(ctx, tx); err != nil {
			return err
		}
	} else {
		var genesisCount int
		if err := tx.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM commands WHERE command_id = 'cmd_genesis_m2_t09' AND status = 'committed'`).Scan(&genesisCount); err != nil {
			return core.WrapError(core.CodeStorageFailure, "verify M2 authoritative genesis", err)
		}
		if genesisCount != 1 {
			return core.NewError(core.CodeStorageFailure, "M2 demo branch has state without the required authoritative genesis")
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return core.WrapError(core.CodeStorageFailure, "commit M2 bootstrap", err)
	}
	return nil
}

func (s *Store) insertM2Genesis(ctx context.Context, tx *immediateTx) error {
	const (
		commandID    = "cmd_genesis_m2_t09"
		attemptID    = "attempt_cmd_genesis_m2_t09_1"
		batchID      = "batch_cmd_genesis_m2_t09"
		eventID      = "event_m2_cohort_initialized"
		entryID      = "journal_m2_cohort_initialized"
		stockID      = "movement_m2_initial_inventory"
		populationID = "population_m2_initial_cohort"
		outboxID     = "outbox_m2_cohort_initialized"
		worldTime    = "2026-09-22T00:00:00Z"
	)

	seedStatements := []struct {
		name, query string
		args        []any
	}{
		{"M2 currency", `INSERT OR IGNORE INTO currencies(currency_id, scale, symbol, definition_event_id) VALUES (?, 0, 'm2c', ?)`, []any{M2DemoCurrencyID, eventID}},
		{"M2 SKU", `INSERT OR IGNORE INTO product_skus(sku_id, base_unit, quantity_scale, definition_event_id) VALUES (?, 'unit', 0, ?)`, []any{M2DemoSKUID, eventID}},
		{"M2 cohort location", `INSERT OR IGNORE INTO stock_locations(location_id, owner_id, location_kind) VALUES (?, ?, 'holder')`, []any{M2DemoCohortLocationID, M2DemoCohortID}},
		{"M2 source location", `INSERT OR IGNORE INTO stock_locations(location_id, owner_id, location_kind, capability_id) VALUES (?, 'system', 'source', 'cap_inventory_create')`, []any{M2DemoSourceLocationID}},
		{"M2 cohort inventory", `INSERT OR IGNORE INTO inventory_balances(location_id, sku_id, quantity_minor, projection_version, last_event_sequence) VALUES (?, ?, 100, 0, 1)`, []any{M2DemoCohortLocationID, M2DemoSKUID}},
		{"M2 source inventory", `INSERT OR IGNORE INTO inventory_balances(location_id, sku_id, quantity_minor, projection_version, last_event_sequence) VALUES (?, ?, 0, 0, 1)`, []any{M2DemoSourceLocationID, M2DemoSKUID}},
		{"M2 creator principal", `INSERT OR IGNORE INTO principals(principal_id, principal_type, display_name, status) VALUES ('principal_creator', 'creator', 'Demo Creator', 'active')`, nil},
		{"M2 materialize capability", `INSERT OR IGNORE INTO capability_definitions(capability_id, description, policy_version) VALUES ('world.cohort.materialize', 'Materialize a named entity from a scoped Cohort', 'm2-v1')`, nil},
		{"M2 dematerialize capability", `INSERT OR IGNORE INTO capability_definitions(capability_id, description, policy_version) VALUES ('world.cohort.dematerialize', 'Return an unchanged named allocation to its source Cohort', 'm2-v1')`, nil},
		{"M2 events capability", `INSERT OR IGNORE INTO capability_definitions(capability_id, description, policy_version) VALUES ('world.events.read', 'Read scoped visible world events', 'm1-v1')`, nil},
	}
	accounts := []struct {
		id, owner, kind string
		balance         int64
	}{
		{M2DemoCohortAssetAccountID, M2DemoCohortID, "asset", 10000},
		{M2DemoCohortReceivableID, M2DemoCohortID, "receivable", 2000},
		{M2DemoCohortLiabilityID, M2DemoCohortID, "liability", -1500},
		{M2DemoIssuanceAccountID, "m2_monetary_authority", "issuance_source", -10000},
		{M2DemoCounterpartyPayableID, "m2_counterparty", "liability", -2000},
		{M2DemoCounterpartyReceivableID, "m2_counterparty", "receivable", 1500},
	}
	for _, account := range accounts {
		seedStatements = append(seedStatements,
			struct {
				name, query string
				args        []any
			}{"M2 account " + account.id, `INSERT OR IGNORE INTO accounts(account_id, owner_id, currency_id, account_type, overdraft_limit_minor, opened_by_event_id) VALUES (?, ?, ?, ?, 0, ?)`, []any{account.id, account.owner, M2DemoCurrencyID, account.kind, eventID}},
			struct {
				name, query string
				args        []any
			}{"M2 account balance " + account.id, `INSERT OR IGNORE INTO account_balances(account_id, balance_minor, projection_version, last_event_sequence) VALUES (?, ?, 0, 1)`, []any{account.id, account.balance}},
		)
	}
	seedStatements = append(seedStatements,
		struct {
			name, query string
			args        []any
		}{"M2 Cohort", `INSERT INTO cohorts(cohort_id, instance_id, branch_id, display_name, population_count, asset_account_id, receivable_account_id, liability_account_id, inventory_location_id, currency_id, sku_id, allocation_algorithm_version, status, projection_version, last_event_sequence, definition_event_id) VALUES (?, ?, ?, 'Block A Background Cohort', 20, ?, ?, ?, ?, ?, ?, 'equal-share-v1', 'active', 0, 1, ?)`, []any{M2DemoCohortID, M2DemoInstanceID, M2DemoBranchID, M2DemoCohortAssetAccountID, M2DemoCohortReceivableID, M2DemoCohortLiabilityID, M2DemoCohortLocationID, M2DemoCurrencyID, M2DemoSKUID, eventID}},
		struct {
			name, query string
			args        []any
		}{"M2 materialize grant", `INSERT INTO capability_grants(grant_id, principal_id, capability_id, instance_id, branch_id, subject_id, field_scope, status, definition_event_id) VALUES ('grant_m2_creator_materialize', 'principal_creator', 'world.cohort.materialize', ?, ?, ?, '[]', 'active', ?)`, []any{M2DemoInstanceID, M2DemoBranchID, M2DemoCohortID, eventID}},
		struct {
			name, query string
			args        []any
		}{"M2 dematerialize grant", `INSERT INTO capability_grants(grant_id, principal_id, capability_id, instance_id, branch_id, subject_id, field_scope, status, definition_event_id) VALUES ('grant_m2_creator_dematerialize', 'principal_creator', 'world.cohort.dematerialize', ?, ?, ?, '[]', 'active', ?)`, []any{M2DemoInstanceID, M2DemoBranchID, M2DemoCohortID, eventID}},
		struct {
			name, query string
			args        []any
		}{"M2 creator event grant", `INSERT INTO capability_grants(grant_id, principal_id, capability_id, instance_id, branch_id, subject_id, field_scope, status, definition_event_id) VALUES ('grant_m2_creator_events', 'principal_creator', 'world.events.read', ?, ?, '*', '["event_id","event_type","world_time","payload"]', 'active', ?)`, []any{M2DemoInstanceID, M2DemoBranchID, eventID}},
	)
	for _, statement := range seedStatements {
		if _, err := tx.conn.ExecContext(ctx, statement.query, statement.args...); err != nil {
			return core.WrapError(core.CodeStorageFailure, "seed "+statement.name, err)
		}
	}

	payload := struct {
		CohortID         string `json:"cohort_id"`
		PopulationCount  int64  `json:"population_count"`
		AssetMinor       int64  `json:"asset_minor"`
		InventoryMinor   int64  `json:"inventory_minor"`
		ReceivableMinor  int64  `json:"receivable_minor"`
		LiabilityMinor   int64  `json:"liability_minor"`
		AlgorithmVersion string `json:"allocation_algorithm_version"`
	}{M2DemoCohortID, 20, 10000, 100, 2000, 1500, "equal-share-v1"}
	payloadJSON, err := core.CanonicalJSON(payload)
	if err != nil {
		return err
	}
	requestHash, err := core.HashJSON(struct {
		CommandType string `json:"command_type"`
		InstanceID  string `json:"instance_id"`
		BranchID    string `json:"branch_id"`
	}{"InitializeM2Cohort", M2DemoInstanceID, M2DemoBranchID})
	if err != nil {
		return err
	}
	batchHash, err := core.HashJSON(struct {
		CommandID string `json:"command_id"`
		EventType string `json:"event_type"`
		Payload   any    `json:"payload"`
	}{commandID, "CohortInitialized", payload})
	if err != nil {
		return err
	}
	audience := struct {
		Kind       string `json:"kind"`
		InstanceID string `json:"instance_id"`
	}{"instance", M2DemoInstanceID}
	audienceJSON, err := core.CanonicalJSON(audience)
	if err != nil {
		return err
	}
	audienceHash, err := core.HashJSON(audience)
	if err != nil {
		return err
	}

	statements := []struct {
		name, query string
		args        []any
	}{
		{"M2 genesis command", `INSERT INTO commands(command_id, instance_id, branch_id, command_type, idempotency_key, request_hash, expected_head, principal_id, command_policy, status, created_at_utc) VALUES (?, ?, ?, 'InitializeM2Cohort', 'genesis', ?, 0, 'principal_system', '{"authorization":"system-bootstrap"}', 'pending', ?)`, []any{commandID, M2DemoInstanceID, M2DemoBranchID, requestHash, worldTime}},
		{"M2 genesis attempt", `INSERT INTO command_attempts(command_id, attempt_no, attempt_id, status, lease_owner, lease_until_utc, proposal_hash, created_at_utc) VALUES (?, 1, ?, 'ready', 'corerp-m2', '2026-09-22T00:01:00Z', ?, ?)`, []any{commandID, attemptID, requestHash, worldTime}},
		{"M2 genesis batch", `INSERT INTO event_batches(batch_id, command_id, attempt_no, instance_id, branch_id, epoch_id, expected_head, first_sequence, last_sequence, event_count, world_time, batch_hash, committed_at_utc) VALUES (?, ?, 1, ?, ?, ?, 0, 1, 1, 1, ?, ?, ?)`, []any{batchID, commandID, M2DemoInstanceID, M2DemoBranchID, M2DemoEpochID, worldTime, batchHash, worldTime}},
		{"M2 genesis event", `INSERT INTO events(event_id, batch_id, instance_id, branch_id, event_sequence, batch_index, event_type, actor_id, world_time, payload) VALUES (?, ?, ?, ?, 1, 0, 'CohortInitialized', 'system', ?, ?)`, []any{eventID, batchID, M2DemoInstanceID, M2DemoBranchID, worldTime, string(payloadJSON)}},
		{"M2 genesis journal", `INSERT INTO journal_entries(entry_id, event_id, status, purpose) VALUES (?, ?, 'draft', 'traceable M2 Cohort opening state')`, []any{entryID, eventID}},
		{"M2 issuance posting", `INSERT INTO postings(posting_id, entry_id, account_id, currency_id, amount_minor, memo) VALUES ('posting_m2_genesis_issuance', ?, ?, ?, -10000, 'opening Cohort assets source')`, []any{entryID, M2DemoIssuanceAccountID, M2DemoCurrencyID}},
		{"M2 asset posting", `INSERT INTO postings(posting_id, entry_id, account_id, currency_id, amount_minor, memo) VALUES ('posting_m2_genesis_asset', ?, ?, ?, 10000, 'opening Cohort assets')`, []any{entryID, M2DemoCohortAssetAccountID, M2DemoCurrencyID}},
		{"M2 counterparty payable posting", `INSERT INTO postings(posting_id, entry_id, account_id, currency_id, amount_minor, memo) VALUES ('posting_m2_genesis_counterparty_payable', ?, ?, ?, -2000, 'counterparty payable')`, []any{entryID, M2DemoCounterpartyPayableID, M2DemoCurrencyID}},
		{"M2 receivable posting", `INSERT INTO postings(posting_id, entry_id, account_id, currency_id, amount_minor, memo) VALUES ('posting_m2_genesis_receivable', ?, ?, ?, 2000, 'opening Cohort receivable')`, []any{entryID, M2DemoCohortReceivableID, M2DemoCurrencyID}},
		{"M2 liability posting", `INSERT INTO postings(posting_id, entry_id, account_id, currency_id, amount_minor, memo) VALUES ('posting_m2_genesis_liability', ?, ?, ?, -1500, 'opening Cohort liability')`, []any{entryID, M2DemoCohortLiabilityID, M2DemoCurrencyID}},
		{"M2 counterparty receivable posting", `INSERT INTO postings(posting_id, entry_id, account_id, currency_id, amount_minor, memo) VALUES ('posting_m2_genesis_counterparty_receivable', ?, ?, ?, 1500, 'counterparty receivable')`, []any{entryID, M2DemoCounterpartyReceivableID, M2DemoCurrencyID}},
		{"post M2 genesis journal", `UPDATE journal_entries SET status = 'posted' WHERE entry_id = ? AND status = 'draft'`, []any{entryID}},
		{"M2 genesis inventory", `INSERT INTO stock_movements(movement_id, event_id, sku_id, from_location_id, to_location_id, quantity_minor, movement_kind, reason_code, capability_id) VALUES (?, ?, ?, ?, ?, 100, 'create', 'm2_cohort_initialization', 'cap_inventory_create')`, []any{stockID, eventID, M2DemoSKUID, M2DemoSourceLocationID, M2DemoCohortLocationID}},
		{"M2 genesis population", `INSERT INTO population_movements(movement_id, event_id, from_owner_kind, from_owner_id, to_owner_kind, to_owner_id, population_count, movement_kind, reason_code) VALUES (?, ?, NULL, NULL, 'cohort', ?, 20, 'create', 'm2_cohort_initialization')`, []any{populationID, eventID, M2DemoCohortID}},
		{"M2 genesis Outbox", `INSERT INTO outbox(outbox_id, event_id, topic, audience_scope, audience_scope_hash, payload) VALUES (?, ?, 'cohort.initialized', ?, ?, ?)`, []any{outboxID, eventID, string(audienceJSON), audienceHash, string(payloadJSON)}},
		{"advance M2 genesis branch", `UPDATE branches SET head_sequence = 1 WHERE instance_id = ? AND branch_id = ? AND head_sequence = 0`, []any{M2DemoInstanceID, M2DemoBranchID}},
		{"commit M2 genesis attempt", `UPDATE command_attempts SET status = 'committed', finished_at_utc = ? WHERE command_id = ? AND attempt_no = 1 AND status = 'ready'`, []any{worldTime, commandID}},
		{"commit M2 genesis command", `UPDATE commands SET status = 'committed' WHERE command_id = ? AND status = 'pending'`, []any{commandID}},
	}
	for _, statement := range statements {
		result, err := tx.conn.ExecContext(ctx, statement.query, statement.args...)
		if err != nil {
			return core.WrapError(core.CodeStorageFailure, "insert "+statement.name, err)
		}
		rows, err := result.RowsAffected()
		if err != nil || rows != 1 {
			return core.NewError(core.CodeStorageFailure, fmt.Sprintf("%s affected an unexpected row count", statement.name))
		}
	}
	return nil
}
