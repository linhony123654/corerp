package storage

import (
	"context"
	"fmt"
	"time"

	"corerp.local/backend/internal/core"
)

func (s *Store) BootstrapDemo(ctx context.Context) error {
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return core.WrapError(core.CodeStorageFailure, "begin bootstrap", err)
	}
	defer tx.Rollback(ctx)

	rulesetHash, err := core.HashJSON(struct {
		Ruleset string `json:"ruleset"`
		Version string `json:"version"`
	}{Ruleset: "m1-demo", Version: "0"})
	if err != nil {
		return err
	}
	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT OR IGNORE INTO world_instances(instance_id, world_definition_id, world_definition_version, created_at_utc, lifecycle_state) VALUES (?, 'corerp.m1.demo', '0.1.0', '2026-09-22T00:00:00Z', 'active')`, []any{DemoInstanceID}},
		{`INSERT OR IGNORE INTO branches(instance_id, branch_id, label, head_sequence, created_at_utc) VALUES (?, ?, 'main', 0, '2026-09-22T00:00:00Z')`, []any{DemoInstanceID, DemoBranchID}},
		{`INSERT INTO rule_epochs(instance_id, branch_id, epoch_id, start_sequence, end_sequence, ruleset_hash, lock_document)
		  SELECT ?, ?, ?, 1, NULL, ?, ?
		  WHERE NOT EXISTS (
		    SELECT 1 FROM rule_epochs WHERE instance_id = ? AND branch_id = ? AND epoch_id = ?
		  )`, []any{DemoInstanceID, DemoBranchID, DemoEpochID, rulesetHash, `{"ruleset":"m1-demo","version":"0"}`, DemoInstanceID, DemoBranchID, DemoEpochID}},
		{`INSERT OR IGNORE INTO currencies(currency_id, scale, symbol, definition_event_id) VALUES (?, 0, 'cr', 'genesis_currency')`, []any{DemoCurrencyID}},
		{`INSERT OR IGNORE INTO accounts(account_id, owner_id, currency_id, account_type, overdraft_limit_minor, opened_by_event_id) VALUES (?, 'buyer', ?, 'asset', 0, 'genesis_buyer_account')`, []any{DemoBuyerAccountID, DemoCurrencyID}},
		{`INSERT OR IGNORE INTO accounts(account_id, owner_id, currency_id, account_type, overdraft_limit_minor, opened_by_event_id) VALUES (?, 'seller', ?, 'asset', 0, 'genesis_seller_account')`, []any{DemoSellerAccountID, DemoCurrencyID}},
		{`INSERT OR IGNORE INTO accounts(account_id, owner_id, currency_id, account_type, overdraft_limit_minor, opened_by_event_id) VALUES (?, 'monetary_authority', ?, 'issuance_source', 0, 'genesis_issuance_account')`, []any{DemoIssuanceAccountID, DemoCurrencyID}},
		{`INSERT OR IGNORE INTO accounts(account_id, owner_id, currency_id, account_type, overdraft_limit_minor, opened_by_event_id) VALUES (?, 'enterprise', ?, 'asset', 0, 'genesis_enterprise_account')`, []any{DemoEmployerAccountID, DemoCurrencyID}},
		{`INSERT OR IGNORE INTO accounts(account_id, owner_id, currency_id, account_type, overdraft_limit_minor, opened_by_event_id) VALUES (?, 'employee_2', ?, 'asset', 0, 'genesis_employee_2_account')`, []any{DemoEmployee2AccountID, DemoCurrencyID}},
		{`INSERT OR IGNORE INTO accounts(account_id, owner_id, currency_id, account_type, overdraft_limit_minor, opened_by_event_id) VALUES (?, 'employee_3', ?, 'asset', 0, 'genesis_employee_3_account')`, []any{DemoEmployee3AccountID, DemoCurrencyID}},
		{`INSERT OR IGNORE INTO accounts(account_id, owner_id, currency_id, account_type, overdraft_limit_minor, opened_by_event_id) VALUES (?, 'landlord', ?, 'asset', 0, 'genesis_landlord_account')`, []any{DemoLandlordAccountID, DemoCurrencyID}},
		{`INSERT OR IGNORE INTO account_balances(account_id, balance_minor, projection_version, last_event_sequence) VALUES (?, 1000, 0, 0)`, []any{DemoBuyerAccountID}},
		{`INSERT OR IGNORE INTO account_balances(account_id, balance_minor, projection_version, last_event_sequence) VALUES (?, 100, 0, 0)`, []any{DemoSellerAccountID}},
		{`INSERT OR IGNORE INTO account_balances(account_id, balance_minor, projection_version, last_event_sequence) VALUES (?, -2200, 0, 0)`, []any{DemoIssuanceAccountID}},
		{`INSERT OR IGNORE INTO account_balances(account_id, balance_minor, projection_version, last_event_sequence) VALUES (?, 1000, 0, 0)`, []any{DemoEmployerAccountID}},
		{`INSERT OR IGNORE INTO account_balances(account_id, balance_minor, projection_version, last_event_sequence) VALUES (?, 0, 0, 0)`, []any{DemoEmployee2AccountID}},
		{`INSERT OR IGNORE INTO account_balances(account_id, balance_minor, projection_version, last_event_sequence) VALUES (?, 0, 0, 0)`, []any{DemoEmployee3AccountID}},
		{`INSERT OR IGNORE INTO account_balances(account_id, balance_minor, projection_version, last_event_sequence) VALUES (?, 100, 0, 0)`, []any{DemoLandlordAccountID}},
		{`INSERT OR IGNORE INTO product_skus(sku_id, base_unit, quantity_scale, definition_event_id) VALUES (?, 'unit', 0, 'genesis_sku')`, []any{DemoSKUID}},
		{`INSERT OR IGNORE INTO stock_locations(location_id, owner_id, location_kind) VALUES (?, 'buyer', 'holder')`, []any{DemoBuyerLocationID}},
		{`INSERT OR IGNORE INTO stock_locations(location_id, owner_id, location_kind) VALUES (?, 'employee_2', 'holder')`, []any{DemoEmployee2LocationID}},
		{`INSERT OR IGNORE INTO stock_locations(location_id, owner_id, location_kind) VALUES (?, 'employee_3', 'holder')`, []any{DemoEmployee3LocationID}},
		{`INSERT OR IGNORE INTO stock_locations(location_id, owner_id, location_kind) VALUES (?, 'seller', 'holder')`, []any{DemoSellerLocationID}},
		{`INSERT OR IGNORE INTO stock_locations(location_id, owner_id, location_kind, capability_id) VALUES (?, 'system', 'source', 'cap_inventory_create')`, []any{DemoSourceLocationID}},
		{`INSERT OR IGNORE INTO inventory_balances(location_id, sku_id, quantity_minor, projection_version, last_event_sequence) VALUES (?, ?, 0, 0, 0)`, []any{DemoBuyerLocationID, DemoSKUID}},
		{`INSERT OR IGNORE INTO inventory_balances(location_id, sku_id, quantity_minor, projection_version, last_event_sequence) VALUES (?, ?, 0, 0, 0)`, []any{DemoEmployee2LocationID, DemoSKUID}},
		{`INSERT OR IGNORE INTO inventory_balances(location_id, sku_id, quantity_minor, projection_version, last_event_sequence) VALUES (?, ?, 0, 0, 0)`, []any{DemoEmployee3LocationID, DemoSKUID}},
		{`INSERT OR IGNORE INTO inventory_balances(location_id, sku_id, quantity_minor, projection_version, last_event_sequence) VALUES (?, ?, 10, 0, 0)`, []any{DemoSellerLocationID, DemoSKUID}},
	}
	for index, statement := range statements {
		if _, err := tx.conn.ExecContext(ctx, statement.query, statement.args...); err != nil {
			return core.WrapError(core.CodeStorageFailure, fmt.Sprintf("bootstrap statement %d", index+1), err)
		}
	}

	var head int64
	if err := tx.conn.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id = ? AND branch_id = ?`, DemoInstanceID, DemoBranchID).Scan(&head); err != nil {
		return core.WrapError(core.CodeStorageFailure, "verify bootstrap branch", err)
	}
	if head < 0 {
		return core.NewError(core.CodeStorageFailure, "bootstrap branch has invalid head")
	}
	if head == 0 {
		if err := s.insertDemoGenesis(ctx, tx); err != nil {
			return err
		}
	} else {
		var genesisCount int
		if err := tx.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM commands WHERE command_id = 'cmd_genesis_m1' AND status = 'committed'`).Scan(&genesisCount); err != nil {
			return core.WrapError(core.CodeStorageFailure, "verify authoritative genesis", err)
		}
		if genesisCount != 1 {
			return core.NewError(core.CodeStorageFailure, "legacy demo database has projections without the required authoritative genesis; create a new demo database")
		}
	}
	if err := s.seedStrictWorld(ctx, tx, rulesetHash); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return core.WrapError(core.CodeStorageFailure, "commit bootstrap", err)
	}
	return nil
}

func (s *Store) insertDemoGenesis(ctx context.Context, tx *immediateTx) error {
	const (
		commandID  = "cmd_genesis_m1"
		attemptID  = "attempt_cmd_genesis_m1_1"
		batchID    = "batch_cmd_genesis_m1"
		eventID    = "event_world_initialized"
		entryID    = "journal_world_initialized"
		movementID = "movement_initial_inventory"
		outboxID   = "outbox_world_initialized"
		worldTime  = "2026-09-22T00:00:00Z"
	)
	payload := struct {
		WorldDefinitionID string `json:"world_definition_id"`
		CurrencyID        string `json:"currency_id"`
		OpeningMoneyMinor int64  `json:"opening_money_minor"`
		SKUID             string `json:"sku_id"`
		OpeningStockMinor int64  `json:"opening_stock_minor"`
	}{
		WorldDefinitionID: "corerp.m1.demo",
		CurrencyID:        DemoCurrencyID,
		OpeningMoneyMinor: 2200,
		SKUID:             DemoSKUID,
		OpeningStockMinor: 10,
	}
	payloadJSON, err := core.CanonicalJSON(payload)
	if err != nil {
		return err
	}
	requestHash, err := core.HashJSON(struct {
		CommandType string `json:"command_type"`
		InstanceID  string `json:"instance_id"`
		BranchID    string `json:"branch_id"`
	}{CommandType: "InitializeWorld", InstanceID: DemoInstanceID, BranchID: DemoBranchID})
	if err != nil {
		return err
	}
	batchHash, err := core.HashJSON(struct {
		CommandID     string `json:"command_id"`
		EpochID       string `json:"epoch_id"`
		EventType     string `json:"event_type"`
		EventSequence int64  `json:"event_sequence"`
		Payload       any    `json:"payload"`
	}{
		CommandID: commandID, EpochID: DemoEpochID, EventType: "WorldInitialized",
		EventSequence: 1, Payload: payload,
	})
	if err != nil {
		return err
	}
	audience := struct {
		Kind       string `json:"kind"`
		InstanceID string `json:"instance_id"`
	}{Kind: "instance", InstanceID: DemoInstanceID}
	audienceJSON, err := core.CanonicalJSON(audience)
	if err != nil {
		return err
	}
	audienceHash, err := core.HashJSON(audience)
	if err != nil {
		return err
	}

	statements := []struct {
		name  string
		query string
		args  []any
	}{
		{"genesis command", `INSERT INTO commands(command_id, instance_id, branch_id, command_type, idempotency_key, request_hash, expected_head, principal_id, command_policy, status, created_at_utc) VALUES (?, ?, ?, 'InitializeWorld', 'genesis', ?, 0, 'principal_system', '{"authorization":"system-bootstrap"}', 'pending', ?)`, []any{commandID, DemoInstanceID, DemoBranchID, requestHash, worldTime}},
		{"genesis attempt", `INSERT INTO command_attempts(command_id, attempt_no, attempt_id, status, lease_owner, lease_until_utc, proposal_hash, created_at_utc) VALUES (?, 1, ?, 'ready', 'corerp-m1', '2026-09-22T00:01:00Z', ?, ?)`, []any{commandID, attemptID, requestHash, worldTime}},
		{"genesis batch", `INSERT INTO event_batches(batch_id, command_id, attempt_no, instance_id, branch_id, epoch_id, expected_head, first_sequence, last_sequence, event_count, world_time, batch_hash, committed_at_utc) VALUES (?, ?, 1, ?, ?, ?, 0, 1, 1, 1, ?, ?, ?)`, []any{batchID, commandID, DemoInstanceID, DemoBranchID, DemoEpochID, worldTime, batchHash, worldTime}},
		{"genesis event", `INSERT INTO events(event_id, batch_id, instance_id, branch_id, event_sequence, batch_index, event_type, actor_id, world_time, payload) VALUES (?, ?, ?, ?, 1, 0, 'WorldInitialized', 'system', ?, ?)`, []any{eventID, batchID, DemoInstanceID, DemoBranchID, worldTime, string(payloadJSON)}},
		{"genesis journal", `INSERT INTO journal_entries(entry_id, event_id, status, purpose) VALUES (?, ?, 'draft', 'traceable opening money')`, []any{entryID, eventID}},
		{"genesis issuance posting", `INSERT INTO postings(posting_id, entry_id, account_id, currency_id, amount_minor, memo) VALUES ('posting_genesis_issuance', ?, ?, ?, -2200, 'explicit monetary issuance source')`, []any{entryID, DemoIssuanceAccountID, DemoCurrencyID}},
		{"genesis buyer posting", `INSERT INTO postings(posting_id, entry_id, account_id, currency_id, amount_minor, memo) VALUES ('posting_genesis_buyer', ?, ?, ?, 1000, 'opening buyer funds')`, []any{entryID, DemoBuyerAccountID, DemoCurrencyID}},
		{"genesis seller posting", `INSERT INTO postings(posting_id, entry_id, account_id, currency_id, amount_minor, memo) VALUES ('posting_genesis_seller', ?, ?, ?, 100, 'opening seller funds')`, []any{entryID, DemoSellerAccountID, DemoCurrencyID}},
		{"genesis enterprise posting", `INSERT INTO postings(posting_id, entry_id, account_id, currency_id, amount_minor, memo) VALUES ('posting_genesis_enterprise', ?, ?, ?, 1000, 'opening enterprise funds')`, []any{entryID, DemoEmployerAccountID, DemoCurrencyID}},
		{"genesis landlord posting", `INSERT INTO postings(posting_id, entry_id, account_id, currency_id, amount_minor, memo) VALUES ('posting_genesis_landlord', ?, ?, ?, 100, 'opening landlord funds')`, []any{entryID, DemoLandlordAccountID, DemoCurrencyID}},
		{"post genesis journal", `UPDATE journal_entries SET status = 'posted' WHERE entry_id = ? AND status = 'draft'`, []any{entryID}},
		{"genesis inventory", `INSERT INTO stock_movements(movement_id, event_id, sku_id, from_location_id, to_location_id, quantity_minor, movement_kind, reason_code, capability_id) VALUES (?, ?, ?, ?, ?, 10, 'create', 'world_initialization', 'cap_inventory_create')`, []any{movementID, eventID, DemoSKUID, DemoSourceLocationID, DemoSellerLocationID}},
		{"genesis Outbox", `INSERT INTO outbox(outbox_id, event_id, topic, audience_scope, audience_scope_hash, payload) VALUES (?, ?, 'world.initialized', ?, ?, ?)`, []any{outboxID, eventID, string(audienceJSON), audienceHash, string(payloadJSON)}},
		{"advance genesis branch", `UPDATE branches SET head_sequence = 1 WHERE instance_id = ? AND branch_id = ? AND head_sequence = 0`, []any{DemoInstanceID, DemoBranchID}},
		{"commit genesis attempt", `UPDATE command_attempts SET status = 'committed', finished_at_utc = ? WHERE command_id = ? AND attempt_no = 1 AND status = 'ready'`, []any{worldTime, commandID}},
		{"commit genesis command", `UPDATE commands SET status = 'committed' WHERE command_id = ? AND status = 'pending'`, []any{commandID}},
	}
	for _, statement := range statements {
		result, err := tx.conn.ExecContext(ctx, statement.query, statement.args...)
		if err != nil {
			return core.WrapError(core.CodeStorageFailure, "insert "+statement.name, err)
		}
		rows, err := result.RowsAffected()
		if err != nil || rows != 1 {
			return core.NewError(core.CodeStorageFailure, statement.name+" affected an unexpected row count")
		}
	}
	return nil
}

func (s *Store) seedStrictWorld(ctx context.Context, tx *immediateTx, rulesetHash string) error {
	statements := []struct {
		name  string
		query string
		args  []any
	}{
		{"world clock", `INSERT OR IGNORE INTO world_clocks(instance_id, branch_id, current_world_time, current_day, status, projection_version, last_event_sequence) VALUES (?, ?, '2026-01-01T00:00:00Z', 0, 'ready', 0, 1)`, []any{DemoInstanceID, DemoBranchID}},
		{"phase wage accrual", `INSERT OR IGNORE INTO scheduler_phases(phase_id, description, ruleset_hash) VALUES ('10_wage_accrual', 'Accrue one wage obligation per contract period', ?)`, []any{rulesetHash}},
		{"phase wage payment", `INSERT OR IGNORE INTO scheduler_phases(phase_id, description, ruleset_hash) VALUES ('20_wage_payment', 'Settle accrued wage obligations', ?)`, []any{rulesetHash}},
		{"phase rent accrual", `INSERT OR IGNORE INTO scheduler_phases(phase_id, description, ruleset_hash) VALUES ('30_rent_accrual', 'Accrue one rent obligation per contract period', ?)`, []any{rulesetHash}},
		{"phase rent payment", `INSERT OR IGNORE INTO scheduler_phases(phase_id, description, ruleset_hash) VALUES ('40_rent_payment', 'Settle due rent obligations', ?)`, []any{rulesetHash}},
		{"phase rent past due", `INSERT OR IGNORE INTO scheduler_phases(phase_id, description, ruleset_hash) VALUES ('45_rent_past_due', 'Convert unpaid rent after grace into past due', ?)`, []any{rulesetHash}},
		{"phase household purchase", `INSERT OR IGNORE INTO scheduler_phases(phase_id, description, ruleset_hash) VALUES ('50_household_purchase', 'Attempt a budgeted household food purchase', ?)`, []any{rulesetHash}},
		{"phase store restock", `INSERT OR IGNORE INTO scheduler_phases(phase_id, description, ruleset_hash) VALUES ('60_store_restock', 'Create traceable store inventory from configured source', ?)`, []any{rulesetHash}},
		{"phase clock checkpoint", `INSERT OR IGNORE INTO scheduler_phases(phase_id, description, ruleset_hash) VALUES ('90_clock_checkpoint', 'Advance the authoritative world clock after due work', ?)`, []any{rulesetHash}},
		{"enterprise entity", `INSERT OR IGNORE INTO economic_entities(entity_id, entity_kind, display_name, account_id, private_finances) VALUES (?, 'enterprise', 'M1 Enterprise', ?, 1)`, []any{DemoEnterpriseEntityID, DemoEmployerAccountID}},
		{"employee 1 entity", `INSERT OR IGNORE INTO economic_entities(entity_id, entity_kind, display_name, account_id, private_finances) VALUES (?, 'employee', 'Employee One', ?, 1)`, []any{DemoEmployee1EntityID, DemoBuyerAccountID}},
		{"employee 2 entity", `INSERT OR IGNORE INTO economic_entities(entity_id, entity_kind, display_name, account_id, private_finances) VALUES (?, 'employee', 'Employee Two', ?, 1)`, []any{DemoEmployee2EntityID, DemoEmployee2AccountID}},
		{"employee 3 entity", `INSERT OR IGNORE INTO economic_entities(entity_id, entity_kind, display_name, account_id, private_finances) VALUES (?, 'employee', 'Employee Three', ?, 1)`, []any{DemoEmployee3EntityID, DemoEmployee3AccountID}},
		{"landlord entity", `INSERT OR IGNORE INTO economic_entities(entity_id, entity_kind, display_name, account_id, private_finances) VALUES (?, 'landlord', 'M1 Landlord', ?, 1)`, []any{DemoLandlordEntityID, DemoLandlordAccountID}},
		{"store entity", `INSERT OR IGNORE INTO economic_entities(entity_id, entity_kind, display_name, account_id, private_finances) VALUES (?, 'store', 'M1 Store', ?, 1)`, []any{DemoStoreEntityID, DemoSellerAccountID}},
		{"employment 1", `INSERT OR IGNORE INTO employment_contracts(contract_id, employer_entity_id, employee_entity_id, employer_account_id, employee_account_id, position_id, gross_wage_minor, currency_id, pay_period_days, starts_on_day, status, definition_event_id) VALUES ('employment_1', ?, ?, ?, ?, 'position_worker_1', 300, ?, 30, 0, 'active', 'event_world_initialized')`, []any{DemoEnterpriseEntityID, DemoEmployee1EntityID, DemoEmployerAccountID, DemoBuyerAccountID, DemoCurrencyID}},
		{"employment 2", `INSERT OR IGNORE INTO employment_contracts(contract_id, employer_entity_id, employee_entity_id, employer_account_id, employee_account_id, position_id, gross_wage_minor, currency_id, pay_period_days, starts_on_day, status, definition_event_id) VALUES ('employment_2', ?, ?, ?, ?, 'position_worker_2', 300, ?, 30, 0, 'active', 'event_world_initialized')`, []any{DemoEnterpriseEntityID, DemoEmployee2EntityID, DemoEmployerAccountID, DemoEmployee2AccountID, DemoCurrencyID}},
		{"employment 3", `INSERT OR IGNORE INTO employment_contracts(contract_id, employer_entity_id, employee_entity_id, employer_account_id, employee_account_id, position_id, gross_wage_minor, currency_id, pay_period_days, starts_on_day, status, definition_event_id) VALUES ('employment_3', ?, ?, ?, ?, 'position_worker_3', 300, ?, 30, 0, 'active', 'event_world_initialized')`, []any{DemoEnterpriseEntityID, DemoEmployee3EntityID, DemoEmployerAccountID, DemoEmployee3AccountID, DemoCurrencyID}},
		{"rent 1", `INSERT OR IGNORE INTO rent_contracts(contract_id, tenant_entity_id, landlord_entity_id, tenant_account_id, landlord_account_id, rent_minor, currency_id, period_days, grace_days, starts_on_day, status, definition_event_id) VALUES ('rent_1', ?, ?, ?, ?, 150, ?, 30, 5, 0, 'active', 'event_world_initialized')`, []any{DemoEmployee1EntityID, DemoLandlordEntityID, DemoBuyerAccountID, DemoLandlordAccountID, DemoCurrencyID}},
		{"rent 2", `INSERT OR IGNORE INTO rent_contracts(contract_id, tenant_entity_id, landlord_entity_id, tenant_account_id, landlord_account_id, rent_minor, currency_id, period_days, grace_days, starts_on_day, status, definition_event_id) VALUES ('rent_2', ?, ?, ?, ?, 150, ?, 30, 5, 0, 'active', 'event_world_initialized')`, []any{DemoEmployee2EntityID, DemoLandlordEntityID, DemoEmployee2AccountID, DemoLandlordAccountID, DemoCurrencyID}},
		{"rent 3", `INSERT OR IGNORE INTO rent_contracts(contract_id, tenant_entity_id, landlord_entity_id, tenant_account_id, landlord_account_id, rent_minor, currency_id, period_days, grace_days, starts_on_day, status, definition_event_id) VALUES ('rent_3', ?, ?, ?, ?, 150, ?, 30, 5, 0, 'active', 'event_world_initialized')`, []any{DemoEmployee3EntityID, DemoLandlordEntityID, DemoEmployee3AccountID, DemoLandlordAccountID, DemoCurrencyID}},
		{"market quote", `INSERT OR IGNORE INTO market_quotes(quote_id, region_id, seller_entity_id, seller_account_id, seller_location_id, sku_id, currency_id, unit_price_minor, valid_from_day, valid_until_day, max_quantity_minor, definition_event_id) VALUES ('quote_bread_90d', 'region_demo', ?, ?, ?, ?, ?, 25, 0, 90, 2, 'event_world_initialized')`, []any{DemoStoreEntityID, DemoSellerAccountID, DemoSellerLocationID, DemoSKUID, DemoCurrencyID}},
		{"buyer principal", `INSERT OR IGNORE INTO principals(principal_id, principal_type, display_name, status) VALUES ('principal_buyer', 'player', 'Employee One Player', 'active')`, nil},
		{"creator principal", `INSERT OR IGNORE INTO principals(principal_id, principal_type, display_name, status) VALUES ('principal_creator', 'creator', 'Demo Creator', 'active')`, nil},
		{"operator principal", `INSERT OR IGNORE INTO principals(principal_id, principal_type, display_name, status) VALUES ('principal_operator', 'operator', 'Demo Operator', 'active')`, nil},
		{"issuance capability", `INSERT OR IGNORE INTO capability_definitions(capability_id, description, policy_version) VALUES ('economy.issue', 'Issue currency under a bounded world policy', 'm1-v1')`, nil},
		{"private read capability", `INSERT OR IGNORE INTO capability_definitions(capability_id, description, policy_version) VALUES ('economy.private.read', 'Read scoped private economic fields', 'm1-v1')`, nil},
		{"diagnostic read capability", `INSERT OR IGNORE INTO capability_definitions(capability_id, description, policy_version) VALUES ('diagnostics.private.read', 'Read redacted private diagnostics', 'm1-v1')`, nil},
		{"simulation capability", `INSERT OR IGNORE INTO capability_definitions(capability_id, description, policy_version) VALUES ('world.simulate', 'Advance the strict world scheduler within a scoped branch', 'm1-v1')`, nil},
		{"state read capability", `INSERT OR IGNORE INTO capability_definitions(capability_id, description, policy_version) VALUES ('world.state.read', 'Read the compact scoped world state', 'm1-v1')`, nil},
		{"event read capability", `INSERT OR IGNORE INTO capability_definitions(capability_id, description, policy_version) VALUES ('world.events.read', 'Read scoped visible world events', 'm1-v1')`, nil},
		{"diagnostic event capability", `INSERT OR IGNORE INTO capability_definitions(capability_id, description, policy_version) VALUES ('diagnostics.events.read', 'Read redacted event diagnostics', 'm1-v1')`, nil},
		{"creator issuance grant", `INSERT OR IGNORE INTO capability_grants(grant_id, principal_id, capability_id, instance_id, branch_id, subject_id, field_scope, amount_limit_minor, status, definition_event_id) VALUES ('grant_creator_issue_buyer', 'principal_creator', 'economy.issue', ?, ?, ?, '[]', 10000000, 'active', 'event_world_initialized')`, []any{DemoInstanceID, DemoBranchID, DemoBuyerAccountID}},
		{"creator private read grant", `INSERT OR IGNORE INTO capability_grants(grant_id, principal_id, capability_id, instance_id, branch_id, subject_id, field_scope, status, definition_event_id) VALUES ('grant_creator_private_read', 'principal_creator', 'economy.private.read', ?, ?, '*', '["account_id","balance_minor","owner_id","evidence"]', 'active', 'event_world_initialized')`, []any{DemoInstanceID, DemoBranchID}},
		{"player private read grant", `INSERT OR IGNORE INTO capability_grants(grant_id, principal_id, capability_id, instance_id, branch_id, subject_id, field_scope, status, definition_event_id) VALUES ('grant_player_private_read_self', 'principal_buyer', 'economy.private.read', ?, ?, ?, '["account_id","balance_minor"]', 'active', 'event_world_initialized')`, []any{DemoInstanceID, DemoBranchID, DemoEmployee1EntityID}},
		{"operator diagnostic grant", `INSERT OR IGNORE INTO capability_grants(grant_id, principal_id, capability_id, instance_id, branch_id, subject_id, field_scope, status, definition_event_id) VALUES ('grant_operator_diagnostics', 'principal_operator', 'diagnostics.private.read', ?, ?, '*', '["account_id","balance_minor","diagnostics"]', 'active', 'event_world_initialized')`, []any{DemoInstanceID, DemoBranchID}},
		{"creator simulation grant", `INSERT OR IGNORE INTO capability_grants(grant_id, principal_id, capability_id, instance_id, branch_id, subject_id, field_scope, status, definition_event_id) VALUES ('grant_creator_simulate', 'principal_creator', 'world.simulate', ?, ?, ?, '[]', 'active', 'event_world_initialized')`, []any{DemoInstanceID, DemoBranchID, DemoBranchID}},
		{"creator state read grant", `INSERT OR IGNORE INTO capability_grants(grant_id, principal_id, capability_id, instance_id, branch_id, subject_id, field_scope, status, definition_event_id) VALUES ('grant_creator_state_read', 'principal_creator', 'world.state.read', ?, ?, ?, '["state"]', 'active', 'event_world_initialized')`, []any{DemoInstanceID, DemoBranchID, DemoBranchID}},
		{"player visible event grant", `INSERT OR IGNORE INTO capability_grants(grant_id, principal_id, capability_id, instance_id, branch_id, subject_id, field_scope, status, definition_event_id) VALUES ('grant_player_events_self', 'principal_buyer', 'world.events.read', ?, ?, ?, '["event_id","event_type","world_time","payload"]', 'active', 'event_world_initialized')`, []any{DemoInstanceID, DemoBranchID, DemoEmployee1EntityID}},
		{"creator event grant", `INSERT OR IGNORE INTO capability_grants(grant_id, principal_id, capability_id, instance_id, branch_id, subject_id, field_scope, status, definition_event_id) VALUES ('grant_creator_events', 'principal_creator', 'world.events.read', ?, ?, '*', '["event_id","event_type","world_time","payload"]', 'active', 'event_world_initialized')`, []any{DemoInstanceID, DemoBranchID}},
		{"operator event grant", `INSERT OR IGNORE INTO capability_grants(grant_id, principal_id, capability_id, instance_id, branch_id, subject_id, field_scope, status, definition_event_id) VALUES ('grant_operator_events', 'principal_operator', 'diagnostics.events.read', ?, ?, '*', '["event_id","event_type","world_time"]', 'active', 'event_world_initialized')`, []any{DemoInstanceID, DemoBranchID}},
		{"issuance policy", `INSERT OR IGNORE INTO issuance_policies(policy_id, currency_id, source_account_id, capability_id, per_command_limit_minor, cumulative_limit_minor, issued_total_minor, status, definition_event_id) VALUES ('policy_creator_credit_issue', ?, ?, 'economy.issue', 10000000, 20000000, 0, 'active', 'event_world_initialized')`, []any{DemoCurrencyID, DemoIssuanceAccountID}},
	}
	for _, statement := range statements {
		if _, err := tx.conn.ExecContext(ctx, statement.query, statement.args...); err != nil {
			return core.WrapError(core.CodeStorageFailure, "seed "+statement.name, err)
		}
	}

	employees := []struct {
		entityID, accountID, locationID, employmentID, rentID string
	}{
		{DemoEmployee1EntityID, DemoBuyerAccountID, DemoBuyerLocationID, "employment_1", "rent_1"},
		{DemoEmployee2EntityID, DemoEmployee2AccountID, DemoEmployee2LocationID, "employment_2", "rent_2"},
		{DemoEmployee3EntityID, DemoEmployee3AccountID, DemoEmployee3LocationID, "employment_3", "rent_3"},
	}
	for _, employee := range employees {
		if err := seedObligationLedger(ctx, tx, "wage", employee.employmentID, DemoEnterpriseEntityID, employee.entityID); err != nil {
			return err
		}
		if err := seedObligationLedger(ctx, tx, "rent", employee.rentID, employee.entityID, DemoLandlordEntityID); err != nil {
			return err
		}
		budgetID := "budget_" + employee.entityID + "_0_91"
		if _, err := tx.conn.ExecContext(ctx, `INSERT OR IGNORE INTO household_budgets(budget_id, household_entity_id, period_start_day, period_end_day, food_limit_minor, rent_limit_minor, currency_id, definition_event_id) VALUES (?, ?, 0, 91, 300, 450, ?, 'event_world_initialized')`, budgetID, employee.entityID, DemoCurrencyID); err != nil {
			return core.WrapError(core.CodeStorageFailure, "seed household budget", err)
		}
		for _, day := range []int{30, 60, 90} {
			dueTime := strictDayTime(day)
			tasks := []struct {
				phase, kind, contractID string
			}{
				{"10_wage_accrual", "wage_accrual", employee.employmentID},
				{"20_wage_payment", "wage_payment", employee.employmentID},
				{"30_rent_accrual", "rent_accrual", employee.rentID},
				{"40_rent_payment", "rent_payment", employee.rentID},
			}
			for _, task := range tasks {
				if err := insertScheduledTask(ctx, tx, day, dueTime, task.phase, task.kind, task.contractID, employee.accountID, employee.locationID); err != nil {
					return err
				}
			}
			if err := insertScheduledTask(ctx, tx, day+5, strictDayTime(day+5), "45_rent_past_due", "rent_past_due", employee.rentID, employee.accountID, employee.locationID); err != nil {
				return err
			}
			if day < 90 {
				obligationID := fmt.Sprintf("wage_%s_%d_%d", employee.employmentID, day-30, day)
				for _, retryDay := range []int{day + 15, day + 25} {
					if err := insertScheduledTask(ctx, tx, retryDay, strictDayTime(retryDay), "20_wage_payment", "wage_retry", obligationID, employee.accountID, employee.locationID); err != nil {
						return err
					}
				}
			}
		}
		for day := 7; day <= 90; day += 7 {
			if err := insertScheduledTask(ctx, tx, day, strictDayTime(day), "50_household_purchase", "household_purchase", employee.entityID, employee.accountID, employee.locationID); err != nil {
				return err
			}
		}
	}
	for day := 14; day <= 90; day += 14 {
		if err := insertScheduledTask(ctx, tx, day, strictDayTime(day), "60_store_restock", "store_restock", DemoStoreEntityID, DemoSellerAccountID, DemoSellerLocationID); err != nil {
			return err
		}
	}
	return nil
}

func seedObligationLedger(ctx context.Context, tx *immediateTx, kind, contractID, payerOwnerID, recipientOwnerID string) error {
	prefix := "account_" + kind + "_" + contractID
	expenseAccount := prefix + "_expense"
	payableAccount := prefix + "_payable"
	receivableAccount := prefix + "_receivable"
	incomeAccount := prefix + "_income"
	accounts := []struct {
		accountID, ownerID, accountType string
	}{
		{expenseAccount, payerOwnerID, "expense"},
		{payableAccount, payerOwnerID, "liability"},
		{receivableAccount, recipientOwnerID, "receivable"},
		{incomeAccount, recipientOwnerID, "income"},
	}
	for _, account := range accounts {
		if _, err := tx.conn.ExecContext(ctx, `INSERT OR IGNORE INTO accounts(account_id, owner_id, currency_id, account_type, overdraft_limit_minor, opened_by_event_id) VALUES (?, ?, ?, ?, 0, 'event_world_initialized')`, account.accountID, account.ownerID, DemoCurrencyID, account.accountType); err != nil {
			return core.WrapError(core.CodeStorageFailure, "seed "+kind+" ledger account", err)
		}
		if _, err := tx.conn.ExecContext(ctx, `INSERT OR IGNORE INTO account_balances(account_id, balance_minor, projection_version, last_event_sequence) VALUES (?, 0, 0, 1)`, account.accountID); err != nil {
			return core.WrapError(core.CodeStorageFailure, "seed "+kind+" ledger balance", err)
		}
	}
	if _, err := tx.conn.ExecContext(ctx, `
		INSERT OR IGNORE INTO obligation_ledger_accounts(
		  obligation_kind, contract_id, expense_account_id, payable_account_id,
		  receivable_account_id, income_account_id, currency_id, definition_event_id
		) VALUES (?, ?, ?, ?, ?, ?, ?, 'event_world_initialized')`,
		kind, contractID, expenseAccount, payableAccount, receivableAccount, incomeAccount, DemoCurrencyID,
	); err != nil {
		return core.WrapError(core.CodeStorageFailure, "seed "+kind+" ledger mapping", err)
	}
	return nil
}

func insertScheduledTask(ctx context.Context, tx *immediateTx, day int, worldTime, phaseID, kind, subjectID, accountID, locationID string) error {
	payload, err := core.CanonicalJSON(struct {
		Kind       string `json:"kind"`
		Day        int    `json:"day"`
		SubjectID  string `json:"subject_id"`
		AccountID  string `json:"account_id"`
		LocationID string `json:"location_id"`
	}{Kind: kind, Day: day, SubjectID: subjectID, AccountID: accountID, LocationID: locationID})
	if err != nil {
		return err
	}
	itemID := fmt.Sprintf("sched_%03d_%s_%s", day, kind, subjectID)
	if _, err := tx.conn.ExecContext(ctx, `
		INSERT OR IGNORE INTO scheduler_items(scheduler_item_id, instance_id, branch_id, world_time, phase_id, declared_priority, status, payload)
		VALUES (?, ?, ?, ?, ?, 0, 'pending', ?)`, itemID, DemoInstanceID, DemoBranchID, worldTime, phaseID, string(payload)); err != nil {
		return core.WrapError(core.CodeStorageFailure, "seed scheduler item "+itemID, err)
	}
	return nil
}

func strictDayTime(day int) string {
	return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, day).Format(time.RFC3339)
}
