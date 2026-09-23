package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"corerp.local/backend/internal/core"
)

const (
	m2EconomyCommandID        = "cmd_m2_cohort_economy_setup"
	m2EconomyEventID          = "event_m2_cohort_economy_setup"
	m2EconomyContractID       = "contract_m2_cohort_wage_18"
	m2EconomyEmployerCash     = "account_m2_coop_employer_cash"
	m2EconomyEmployerExpense  = "account_m2_coop_wage_expense"
	m2EconomyEmployerPayable  = "account_m2_coop_wage_payable"
	m2EconomyCohortIncome     = "account_m2_cohort_wage_income"
	m2EconomyRentContract     = "contract_m2_cohort_household_rent"
	m2EconomyLandlordCash     = "account_m2_landlord_cash"
	m2EconomyLandlordIncome   = "account_m2_landlord_rent_income"
	m2EconomyLandlordRentDue  = "account_m2_landlord_rent_receivable"
	m2EconomyCohortRentCost   = "account_m2_cohort_rent_expense"
	m2EconomyCohortRentDue    = "account_m2_cohort_rent_payable"
	m2EconomyPhaseAccrue      = "m2_10_wage_accrual"
	m2EconomyPhasePay         = "m2_20_wage_payment"
	m2EconomyRentPhaseAccrue  = "m2_30_rent_accrual"
	m2EconomyRentPhasePay     = "m2_40_rent_payment"
	m2StoreActorID            = "actor_m2_food_store"
	m2StoreCash               = "account_m2_food_store_cash"
	m2StoreLocation           = "location_m2_food_store"
	m2HouseholdLocation       = "location_m2_household_food"
	m2StoreOfferID            = "offer_m2_staple_daily"
	m2StorePhaseBuy           = "m2_50_food_purchase"
	m2FoodConsumePhase        = "m2_60_food_consume"
	m2RestockPhase            = "m2_70_store_restock"
	m2SupplierActorID         = "actor_m2_food_supplier"
	m2SupplierCash            = "account_m2_food_supplier_cash"
	m2SupplierLocation        = "location_m2_food_supplier"
	m2FoodSinkLocation        = "location_m2_food_consumed"
	m2FoodConsumeCapability   = "cap_m2_food_consume"
	m2SupplierQuoteID         = "quote_m2_supplier_staple"
	m2ServicePhase            = "m2_80_maintenance_service"
	m2WageRetryPhase          = "m2_90_wage_arrears_retry"
	m2RentRetryPhase          = "m2_91_rent_arrears_retry"
	m2DefaultReviewPhase      = "m2_92_default_review"
	m2InsolvencyPhase         = "m2_93_insolvency_review"
	m2InsolvencyPolicyID      = "policy_m2_coop_day30_insolvency"
	m2EstatePolicyID          = "policy_m2_day31_voluntary_estate_distribution"
	m2EstateContributionPhase = "m2_94_estate_contribution"
	m2EstateDistributionPhase = "m2_95_estate_distribution"
	m2ProceedingID            = "proceeding_m2_coop_day30"
	m2ServiceOrderID          = "order_m2_landlord_coop_maintenance"
	m2LandlordServiceExpense  = "account_m2_landlord_service_expense"
	m2EmployerServiceIncome   = "account_m2_coop_service_income"
	m2EconomyPeriodStart      = "2026-09-22T07:00:00Z"
	m2EconomyAccrualTime      = "2026-09-23T07:00:00Z"
	m2EconomyPaymentTime      = "2026-09-23T07:01:00Z"
	m2EconomyObligationID     = "obligation_m2_wage_day_1"
)

type M2EconomicSetupResult struct {
	EventSequence int64 `json:"event_sequence"`
	Replayed      bool  `json:"replayed"`
}

// PrepareM2EconomicDemo explicitly funds a finite, distinct employer from the
// 18-person Cohort. This is a demo economy definition, never a startup side effect.
func (s *Store) PrepareM2EconomicDemo(ctx context.Context) (M2EconomicSetupResult, error) {
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return M2EconomicSetupResult{}, core.WrapError(core.CodeStorageFailure, "begin M2 economy setup", err)
	}
	defer tx.Rollback(ctx)
	var sequence int64
	err = tx.conn.QueryRowContext(ctx, `SELECT e.event_sequence FROM events e JOIN commands c ON c.command_id = ? AND c.status = 'committed' WHERE e.event_id = ? AND e.instance_id = ? AND e.branch_id = ?`, m2EconomyCommandID, m2EconomyEventID, M2DemoInstanceID, M2DemoBranchID).Scan(&sequence)
	if err == nil {
		var offers int64
		if err := tx.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM m2_store_offers WHERE offer_id = ? AND definition_event_id = ?`, m2StoreOfferID, m2EconomyEventID).Scan(&offers); err != nil {
			return M2EconomicSetupResult{}, core.WrapError(core.CodeStorageFailure, "verify M2 economy fixture revision", err)
		}
		if offers != 1 {
			return M2EconomicSetupResult{}, core.NewError(core.CodeBranchConflict, "legacy M2 economy fixture has no finite store; create a fresh demo database")
		}
		var quotes int64
		if err := tx.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM m2_supplier_quotes WHERE quote_id = ? AND definition_event_id = ?`, m2SupplierQuoteID, m2EconomyEventID).Scan(&quotes); err != nil {
			return M2EconomicSetupResult{}, core.WrapError(core.CodeStorageFailure, "verify M2 supplier fixture revision", err)
		}
		if quotes != 1 {
			return M2EconomicSetupResult{}, core.NewError(core.CodeBranchConflict, "legacy M2 economy fixture has no finite supplier; create a fresh demo database")
		}
		var terms int64
		if err := tx.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM m2_arrears_terms WHERE definition_event_id = ? AND contract_id IN (?, ?)`, m2EconomyEventID, m2EconomyContractID, m2EconomyRentContract).Scan(&terms); err != nil {
			return M2EconomicSetupResult{}, core.WrapError(core.CodeStorageFailure, "verify M2 arrears fixture revision", err)
		}
		if terms != 2 {
			return M2EconomicSetupResult{}, core.NewError(core.CodeBranchConflict, "legacy M2 economy fixture has no arrears terms; create a fresh demo database")
		}
		var policies int64
		if err := tx.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM m2_insolvency_policies WHERE policy_id = ? AND definition_event_id = ?`, m2InsolvencyPolicyID, m2EconomyEventID).Scan(&policies); err != nil {
			return M2EconomicSetupResult{}, core.WrapError(core.CodeStorageFailure, "verify M2 insolvency fixture revision", err)
		}
		if policies != 1 {
			return M2EconomicSetupResult{}, core.NewError(core.CodeBranchConflict, "legacy M2 economy fixture has no insolvency policy; create a fresh demo database")
		}
		var wageEnds sql.NullString
		if err := tx.conn.QueryRowContext(ctx, `SELECT effective_until FROM m2_cohort_contracts WHERE contract_id = ? AND definition_event_id = ?`, m2EconomyContractID, m2EconomyEventID).Scan(&wageEnds); err != nil {
			return M2EconomicSetupResult{}, classifyMissing(err, "M2 wage contract term")
		}
		if !wageEnds.Valid || wageEnds.String != m2WageTime(30, 7, 12) {
			return M2EconomicSetupResult{}, core.NewError(core.CodeBranchConflict, "legacy M2 wage contract has no declared fixed term; create a fresh demo database")
		}
		var allocationPolicies int64
		if err := tx.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM m2_wage_allocation_policies WHERE policy_id = ? AND contract_id = ? AND effective_from = ? AND policy_version = ? AND definition_event_id = ?`, m2WageAllocationPolicyID, m2EconomyContractID, m2EconomyPeriodStart, m2WageAllocationPolicyVersion, m2EconomyEventID).Scan(&allocationPolicies); err != nil {
			return M2EconomicSetupResult{}, core.WrapError(core.CodeStorageFailure, "verify M2 wage allocation policy", err)
		}
		if allocationPolicies != 1 {
			return M2EconomicSetupResult{}, core.NewError(core.CodeProjectionDiverged, "M2 wage allocation policy is missing")
		}
		var estatePolicies int64
		if err := tx.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM m2_estate_policies WHERE policy_id = ? AND definition_event_id = ? AND contribution_minor = 18 AND per_worker_minor = 1`, m2EstatePolicyID, m2EconomyEventID).Scan(&estatePolicies); err != nil {
			return M2EconomicSetupResult{}, core.WrapError(core.CodeStorageFailure, "verify M2 estate fixture revision", err)
		}
		if estatePolicies != 1 {
			return M2EconomicSetupResult{}, core.NewError(core.CodeBranchConflict, "legacy M2 economy fixture has no declared estate contribution; create a fresh demo database")
		}
		return M2EconomicSetupResult{sequence, true}, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return M2EconomicSetupResult{}, core.WrapError(core.CodeStorageFailure, "check M2 economy setup", err)
	}
	var head, population, balance, balanceVersion int64
	var clock string
	if err := tx.conn.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id = ? AND branch_id = ?`, M2DemoInstanceID, M2DemoBranchID).Scan(&head); err != nil {
		return M2EconomicSetupResult{}, classifyMissing(err, "M2 economic branch")
	}
	if err := tx.conn.QueryRowContext(ctx, `SELECT current_world_time FROM world_clocks WHERE instance_id = ? AND branch_id = ?`, M2DemoInstanceID, M2DemoBranchID).Scan(&clock); err != nil {
		return M2EconomicSetupResult{}, classifyMissing(err, "M2 economic clock")
	}
	if clock >= m2EconomyAccrualTime {
		return M2EconomicSetupResult{}, core.NewError(core.CodeBranchConflict, "M2 economy setup is too late for its first wage")
	}
	if err := tx.conn.QueryRowContext(ctx, `SELECT population_count FROM cohorts WHERE cohort_id = ? AND instance_id = ? AND branch_id = ? AND status = 'active'`, M2DemoCohortID, M2DemoInstanceID, M2DemoBranchID).Scan(&population); err != nil {
		return M2EconomicSetupResult{}, classifyMissing(err, "M2 economic Cohort")
	}
	if population != 18 {
		return M2EconomicSetupResult{}, core.NewError(core.CodeBranchConflict, "M2 demo wage definition requires the 18-person Cohort")
	}
	if balance, balanceVersion, err = readScheduledBalance(ctx, tx.conn, M2DemoCohortAssetAccountID); err != nil {
		return M2EconomicSetupResult{}, err
	}
	if balance < 1200 {
		return M2EconomicSetupResult{}, core.NewError(core.CodeInsufficientFunds, "M2 Cohort cannot fund employer capital")
	}
	cohortStock, cohortStockVersion, err := readScheduledInventory(ctx, tx.conn, M2DemoCohortLocationID, M2DemoSKUID)
	if err != nil {
		return M2EconomicSetupResult{}, err
	}
	if err := verifyM2InventoryProjection(ctx, tx.conn, M2DemoCohortLocationID, M2DemoSKUID); err != nil {
		return M2EconomicSetupResult{}, err
	}
	if cohortStock < 35 {
		return M2EconomicSetupResult{}, core.NewError(core.CodeInsufficientStock, "M2 Cohort cannot supply finite store and supplier stock")
	}
	sequence = head + 1
	var epochID string
	if err := tx.conn.QueryRowContext(ctx, `SELECT epoch_id FROM rule_epochs WHERE instance_id = ? AND branch_id = ? AND start_sequence <= ? AND (end_sequence IS NULL OR ? < end_sequence)`, M2DemoInstanceID, M2DemoBranchID, sequence, sequence).Scan(&epochID); err != nil {
		return M2EconomicSetupResult{}, classifyMissing(err, "M2 economic epoch")
	}
	payload := struct {
		CohortID                string `json:"cohort_id"`
		ActorID                 string `json:"actor_id"`
		Capital                 int64  `json:"capital_minor"`
		Workers                 int64  `json:"background_workers"`
		Rate                    int64  `json:"wage_per_worker_minor"`
		LandlordID              string `json:"landlord_id"`
		HouseholdRent           int64  `json:"daily_household_rent_minor"`
		StoreID                 string `json:"store_id"`
		StoreStock              int64  `json:"store_stock_minor"`
		SKUID                   string `json:"sku_id"`
		Price                   int64  `json:"unit_price_minor"`
		DailyBudget             int64  `json:"daily_food_budget_minor"`
		OfferID                 string `json:"offer_id"`
		CurrencyID              string `json:"currency_id"`
		StoreLocation           string `json:"store_location_id"`
		HouseholdLocation       string `json:"household_location_id"`
		EffectiveFrom           string `json:"effective_from"`
		SupplierID              string `json:"supplier_id"`
		SupplierLocation        string `json:"supplier_location_id"`
		SupplierStock           int64  `json:"supplier_stock_minor"`
		SupplierQuoteID         string `json:"supplier_quote_id"`
		SupplierUnitPrice       int64  `json:"supplier_unit_price_minor"`
		SupplyCapacity          int64  `json:"supplier_max_quantity_minor"`
		FoodSinkLocation        string `json:"food_sink_location_id"`
		ConsumeCapability       string `json:"food_consume_capability_id"`
		WageGraceDays           int64  `json:"wage_grace_days"`
		RentGraceDays           int64  `json:"rent_grace_days"`
		LateFee                 int64  `json:"late_fee_minor"`
		ServiceOrderID          string `json:"service_order_id"`
		ServicePrice            int64  `json:"service_price_minor"`
		ServiceDueAt            string `json:"service_due_at"`
		InsolvencyPolicy        string `json:"insolvency_policy_id"`
		ReviewAt                string `json:"insolvency_review_at"`
		MinUnpaidWage           int64  `json:"min_unpaid_wage_minor"`
		WageTermEnds            string `json:"wage_effective_until"`
		EstatePolicyID          string `json:"estate_policy_id"`
		EstateContributionMinor int64  `json:"estate_contribution_minor"`
		EstateContributionAt    string `json:"estate_contribution_at"`
		EstateDistributionAt    string `json:"estate_distribution_at"`
		EstatePerWorkerMinor    int64  `json:"estate_per_worker_minor"`
	}{M2DemoCohortID, "actor_m2_coop_employer", 1200, population, 10, "actor_m2_landlord", 320, m2StoreActorID, 25, M2DemoSKUID, 5, 5, m2StoreOfferID, M2DemoCurrencyID, m2StoreLocation, m2HouseholdLocation, m2EconomyPeriodStart, m2SupplierActorID, m2SupplierLocation, 10, m2SupplierQuoteID, 3, 10, m2FoodSinkLocation, m2FoodConsumeCapability, 3, 2, 0, m2ServiceOrderID, 60, m2WageTime(15, 7, 8), m2InsolvencyPolicyID, m2WageTime(30, 7, 12), 360, m2WageTime(30, 7, 12), m2EstatePolicyID, 18, m2WageTime(31, 7, 0), m2WageTime(31, 7, 1), 1}
	payloadJSON, err := core.CanonicalJSON(payload)
	if err != nil {
		return M2EconomicSetupResult{}, err
	}
	requestHash, err := core.HashJSON(payload)
	if err != nil {
		return M2EconomicSetupResult{}, err
	}
	batchHash, err := core.HashJSON(struct {
		Sequence int64 `json:"sequence"`
		Payload  any   `json:"payload"`
	}{sequence, payload})
	if err != nil {
		return M2EconomicSetupResult{}, err
	}
	now := s.now().UTC()
	nowText := now.Format(time.RFC3339Nano)
	for _, step := range []struct {
		name, query string
		args        []any
	}{
		{"command", `INSERT INTO commands(command_id, instance_id, branch_id, command_type, idempotency_key, request_hash, expected_head, principal_id, command_policy, status, created_at_utc) VALUES (?, ?, ?, 'DefineM2BackgroundEconomy', 'demo-v1', ?, ?, 'principal_system', '{"authorization":"explicit-demo-preparation"}', 'pending', ?)`, []any{m2EconomyCommandID, M2DemoInstanceID, M2DemoBranchID, requestHash, head, nowText}},
		{"attempt", `INSERT INTO command_attempts(command_id, attempt_no, attempt_id, status, lease_owner, lease_until_utc, proposal_hash, created_at_utc) VALUES (?, 1, ?, 'ready', 'corerp-m2-economy', ?, ?, ?)`, []any{m2EconomyCommandID, "attempt_" + m2EconomyCommandID, now.Add(30 * time.Second).Format(time.RFC3339Nano), requestHash, nowText}},
		{"batch", `INSERT INTO event_batches(batch_id, command_id, attempt_no, instance_id, branch_id, epoch_id, expected_head, first_sequence, last_sequence, event_count, world_time, batch_hash, committed_at_utc) VALUES (?, ?, 1, ?, ?, ?, ?, ?, ?, 1, ?, ?, ?)`, []any{"batch_" + m2EconomyCommandID, m2EconomyCommandID, M2DemoInstanceID, M2DemoBranchID, epochID, head, sequence, sequence, clock, batchHash, nowText}},
		{"event", `INSERT INTO events(event_id, batch_id, instance_id, branch_id, event_sequence, batch_index, event_type, actor_id, world_time, payload) VALUES (?, ?, ?, ?, ?, 0, 'M2BackgroundEconomyDefined', 'system', ?, ?)`, []any{m2EconomyEventID, "batch_" + m2EconomyCommandID, M2DemoInstanceID, M2DemoBranchID, sequence, clock, string(payloadJSON)}},
	} {
		if err := execAgentOne(ctx, tx.conn, "define M2 economy "+step.name, step.query, step.args...); err != nil {
			return M2EconomicSetupResult{}, err
		}
	}
	for _, account := range []struct{ id, owner, kind string }{
		{m2EconomyEmployerCash, "actor_m2_coop_employer", "asset"},
		{m2EconomyEmployerExpense, "actor_m2_coop_employer", "expense"},
		{m2EconomyEmployerPayable, "actor_m2_coop_employer", "liability"},
		{m2EconomyCohortIncome, M2DemoCohortID, "income"},
		{m2EconomyLandlordCash, "actor_m2_landlord", "asset"},
		{m2EconomyLandlordIncome, "actor_m2_landlord", "income"},
		{m2EconomyLandlordRentDue, "actor_m2_landlord", "receivable"},
		{m2EconomyCohortRentCost, M2DemoCohortID, "expense"},
		{m2EconomyCohortRentDue, M2DemoCohortID, "liability"},
		{m2StoreCash, m2StoreActorID, "asset"},
		{m2SupplierCash, m2SupplierActorID, "asset"},
		{m2LandlordServiceExpense, "actor_m2_landlord", "expense"},
		{m2EmployerServiceIncome, "actor_m2_coop_employer", "income"},
	} {
		if err := execAgentOne(ctx, tx.conn, "open M2 economy account", `INSERT INTO accounts(account_id, owner_id, currency_id, account_type, opened_by_event_id) VALUES (?, ?, ?, ?, ?)`, account.id, account.owner, M2DemoCurrencyID, account.kind, m2EconomyEventID); err != nil {
			return M2EconomicSetupResult{}, err
		}
		initial := int64(0)
		if account.id == m2EconomyEmployerCash {
			initial = 1200
		}
		if err := execAgentOne(ctx, tx.conn, "project M2 economy account", `INSERT INTO account_balances(account_id, balance_minor, projection_version, last_event_sequence) VALUES (?, ?, 0, ?)`, account.id, initial, sequence); err != nil {
			return M2EconomicSetupResult{}, err
		}
	}
	if err := execAgentOne(ctx, tx.conn, "post M2 capital journal", `INSERT INTO journal_entries(entry_id, event_id, status, purpose) VALUES (?, ?, 'draft', 'finite Cohort employer capital allocation')`, "journal_"+m2EconomyCommandID, m2EconomyEventID); err != nil {
		return M2EconomicSetupResult{}, err
	}
	for _, posting := range []struct {
		id, account string
		amount      int64
	}{{"posting_m2_cohort_capital", M2DemoCohortAssetAccountID, -1200}, {"posting_m2_employer_capital", m2EconomyEmployerCash, 1200}} {
		if err := execAgentOne(ctx, tx.conn, "post M2 capital transfer", `INSERT INTO postings(posting_id, entry_id, account_id, currency_id, amount_minor, memo) VALUES (?, ?, ?, ?, ?, 'fund employer from existing Cohort cash')`, posting.id, "journal_"+m2EconomyCommandID, posting.account, M2DemoCurrencyID, posting.amount); err != nil {
			return M2EconomicSetupResult{}, err
		}
	}
	if err := execAgentOne(ctx, tx.conn, "balance M2 capital journal", `UPDATE journal_entries SET status = 'posted' WHERE entry_id = ? AND status = 'draft'`, "journal_"+m2EconomyCommandID); err != nil {
		return M2EconomicSetupResult{}, err
	}
	if err := execAgentOne(ctx, tx.conn, "debit Cohort capital", `UPDATE account_balances SET balance_minor = ?, projection_version = projection_version + 1, last_event_sequence = ? WHERE account_id = ? AND projection_version = ?`, balance-1200, sequence, M2DemoCohortAssetAccountID, balanceVersion); err != nil {
		return M2EconomicSetupResult{}, err
	}
	if err := execAgentOne(ctx, tx.conn, "define employer", `INSERT INTO m2_economic_actors(actor_id, instance_id, branch_id, kind, cash_account_id, definition_event_id) VALUES ('actor_m2_coop_employer', ?, ?, 'employer', ?, ?)`, M2DemoInstanceID, M2DemoBranchID, m2EconomyEmployerCash, m2EconomyEventID); err != nil {
		return M2EconomicSetupResult{}, err
	}
	if err := execAgentOne(ctx, tx.conn, "define M2 employer insolvency policy", `INSERT INTO m2_insolvency_policies(policy_id, actor_id, decision_at, min_unpaid_wage_minor, require_zero_cash, definition_event_id) VALUES (?, 'actor_m2_coop_employer', ?, 360, 1, ?)`, m2InsolvencyPolicyID, m2WageTime(30, 7, 12), m2EconomyEventID); err != nil {
		return M2EconomicSetupResult{}, err
	}
	if err := execAgentOne(ctx, tx.conn, "define wage contract", `INSERT INTO m2_cohort_contracts(contract_id, cohort_id, actor_id, kind, participant_count, unit_rate_minor, currency_id, effective_from, effective_until, definition_event_id) VALUES (?, ?, 'actor_m2_coop_employer', 'wage', ?, 10, ?, ?, ?, ?)`, m2EconomyContractID, M2DemoCohortID, population, M2DemoCurrencyID, m2EconomyPeriodStart, m2WageTime(30, 7, 12), m2EconomyEventID); err != nil {
		return M2EconomicSetupResult{}, err
	}
	if err := execAgentOne(ctx, tx.conn, "define M2 wage allocation policy", `INSERT INTO m2_wage_allocation_policies(policy_id, contract_id, effective_from, policy_version, definition_event_id) VALUES (?, ?, ?, ?, ?)`, m2WageAllocationPolicyID, m2EconomyContractID, m2EconomyPeriodStart, m2WageAllocationPolicyVersion, m2EconomyEventID); err != nil {
		return M2EconomicSetupResult{}, err
	}
	if err := execAgentOne(ctx, tx.conn, "define landlord", `INSERT INTO m2_economic_actors(actor_id, instance_id, branch_id, kind, cash_account_id, definition_event_id) VALUES ('actor_m2_landlord', ?, ?, 'landlord', ?, ?)`, M2DemoInstanceID, M2DemoBranchID, m2EconomyLandlordCash, m2EconomyEventID); err != nil {
		return M2EconomicSetupResult{}, err
	}
	if err := execAgentOne(ctx, tx.conn, "define M2 voluntary estate policy", `INSERT INTO m2_estate_policies(policy_id, actor_id, donor_actor_id, contribution_minor, contribution_at, distribution_at, per_worker_minor, definition_event_id) VALUES (?, 'actor_m2_coop_employer', 'actor_m2_landlord', 18, ?, ?, 1, ?)`, m2EstatePolicyID, m2WageTime(31, 7, 0), m2WageTime(31, 7, 1), m2EconomyEventID); err != nil {
		return M2EconomicSetupResult{}, err
	}
	if err := execAgentOne(ctx, tx.conn, "define household rent contract", `INSERT INTO m2_cohort_contracts(contract_id, cohort_id, actor_id, kind, participant_count, unit_rate_minor, currency_id, effective_from, definition_event_id) VALUES (?, ?, 'actor_m2_landlord', 'rent', 1, 320, ?, ?, ?)`, m2EconomyRentContract, M2DemoCohortID, M2DemoCurrencyID, m2EconomyPeriodStart, m2EconomyEventID); err != nil {
		return M2EconomicSetupResult{}, err
	}
	for _, terms := range []struct {
		contractID string
		graceDays  int64
	}{{m2EconomyContractID, 3}, {m2EconomyRentContract, 2}} {
		if err := execAgentOne(ctx, tx.conn, "define M2 contract-local arrears terms", `INSERT INTO m2_arrears_terms(contract_id, grace_days, late_fee_minor, definition_event_id) VALUES (?, ?, 0, ?)`, terms.contractID, terms.graceDays, m2EconomyEventID); err != nil {
			return M2EconomicSetupResult{}, err
		}
	}
	if err := execAgentOne(ctx, tx.conn, "define M2 funded maintenance order", `INSERT INTO m2_service_orders(order_id, buyer_actor_id, seller_actor_id, service_code, price_minor, currency_id, due_at, definition_event_id) VALUES (?, 'actor_m2_landlord', 'actor_m2_coop_employer', 'property_maintenance_demo', 60, ?, ?, ?)`, m2ServiceOrderID, M2DemoCurrencyID, m2WageTime(15, 7, 8), m2EconomyEventID); err != nil {
		return M2EconomicSetupResult{}, err
	}
	for _, location := range []struct{ id, owner string }{{m2StoreLocation, m2StoreActorID}, {m2HouseholdLocation, M2DemoCohortID}, {m2SupplierLocation, m2SupplierActorID}} {
		if err := execAgentOne(ctx, tx.conn, "define M2 store location", `INSERT INTO stock_locations(location_id, owner_id, location_kind) VALUES (?, ?, 'holder')`, location.id, location.owner); err != nil {
			return M2EconomicSetupResult{}, err
		}
		initial := int64(0)
		if location.id == m2StoreLocation {
			initial = 25
		}
		if location.id == m2SupplierLocation {
			initial = 10
		}
		if err := execAgentOne(ctx, tx.conn, "project M2 store inventory", `INSERT INTO inventory_balances(location_id, sku_id, quantity_minor, projection_version, last_event_sequence) VALUES (?, ?, ?, 0, ?)`, location.id, M2DemoSKUID, initial, sequence); err != nil {
			return M2EconomicSetupResult{}, err
		}
	}
	if err := execAgentOne(ctx, tx.conn, "define M2 food consumption sink", `INSERT INTO stock_locations(location_id, owner_id, location_kind, capability_id) VALUES (?, 'system', 'sink', ?)`, m2FoodSinkLocation, m2FoodConsumeCapability); err != nil {
		return M2EconomicSetupResult{}, err
	}
	if err := execAgentOne(ctx, tx.conn, "project M2 food consumption sink", `INSERT INTO inventory_balances(location_id, sku_id, quantity_minor, projection_version, last_event_sequence) VALUES (?, ?, 0, 0, ?)`, m2FoodSinkLocation, M2DemoSKUID, sequence); err != nil {
		return M2EconomicSetupResult{}, err
	}
	if err := execAgentOne(ctx, tx.conn, "source M2 store stock", `INSERT INTO stock_movements(movement_id, event_id, sku_id, from_location_id, to_location_id, quantity_minor, movement_kind, reason_code) VALUES ('movement_m2_store_opening', ?, ?, ?, ?, 25, 'transfer', 'finite_cohort_store_allocation')`, m2EconomyEventID, M2DemoSKUID, M2DemoCohortLocationID, m2StoreLocation); err != nil {
		return M2EconomicSetupResult{}, err
	}
	if err := execAgentOne(ctx, tx.conn, "source M2 supplier stock", `INSERT INTO stock_movements(movement_id, event_id, sku_id, from_location_id, to_location_id, quantity_minor, movement_kind, reason_code) VALUES ('movement_m2_supplier_opening', ?, ?, ?, ?, 10, 'transfer', 'finite_cohort_supplier_allocation')`, m2EconomyEventID, M2DemoSKUID, M2DemoCohortLocationID, m2SupplierLocation); err != nil {
		return M2EconomicSetupResult{}, err
	}
	if err := execAgentOne(ctx, tx.conn, "debit M2 Cohort stock", `UPDATE inventory_balances SET quantity_minor = ?, projection_version = projection_version + 1, last_event_sequence = ? WHERE location_id = ? AND sku_id = ? AND projection_version = ?`, cohortStock-35, sequence, M2DemoCohortLocationID, M2DemoSKUID, cohortStockVersion); err != nil {
		return M2EconomicSetupResult{}, err
	}
	if err := execAgentOne(ctx, tx.conn, "define M2 store", `INSERT INTO m2_economic_actors(actor_id, instance_id, branch_id, kind, cash_account_id, stock_location_id, definition_event_id) VALUES (?, ?, ?, 'store', ?, ?, ?)`, m2StoreActorID, M2DemoInstanceID, M2DemoBranchID, m2StoreCash, m2StoreLocation, m2EconomyEventID); err != nil {
		return M2EconomicSetupResult{}, err
	}
	if err := execAgentOne(ctx, tx.conn, "define M2 food offer", `INSERT INTO m2_store_offers(offer_id, actor_id, cohort_id, sku_id, currency_id, unit_price_minor, daily_budget_minor, household_location_id, effective_from, definition_event_id) VALUES (?, ?, ?, ?, ?, 5, 5, ?, ?, ?)`, m2StoreOfferID, m2StoreActorID, M2DemoCohortID, M2DemoSKUID, M2DemoCurrencyID, m2HouseholdLocation, m2EconomyPeriodStart, m2EconomyEventID); err != nil {
		return M2EconomicSetupResult{}, err
	}
	if err := execAgentOne(ctx, tx.conn, "define M2 supplier", `INSERT INTO m2_economic_actors(actor_id, instance_id, branch_id, kind, cash_account_id, stock_location_id, definition_event_id) VALUES (?, ?, ?, 'supplier', ?, ?, ?)`, m2SupplierActorID, M2DemoInstanceID, M2DemoBranchID, m2SupplierCash, m2SupplierLocation, m2EconomyEventID); err != nil {
		return M2EconomicSetupResult{}, err
	}
	if err := execAgentOne(ctx, tx.conn, "define M2 supplier quote", `INSERT INTO m2_supplier_quotes(quote_id, supplier_actor_id, store_actor_id, sku_id, currency_id, unit_price_minor, max_quantity_minor, effective_from, definition_event_id) VALUES (?, ?, ?, ?, ?, 3, 10, ?, ?)`, m2SupplierQuoteID, m2SupplierActorID, m2StoreActorID, M2DemoSKUID, M2DemoCurrencyID, m2EconomyPeriodStart, m2EconomyEventID); err != nil {
		return M2EconomicSetupResult{}, err
	}
	var rulesetHash string
	if err := tx.conn.QueryRowContext(ctx, `SELECT ruleset_hash FROM rule_epochs WHERE instance_id = ? AND branch_id = ? AND epoch_id = ?`, M2DemoInstanceID, M2DemoBranchID, epochID).Scan(&rulesetHash); err != nil {
		return M2EconomicSetupResult{}, classifyMissing(err, "M2 economy ruleset")
	}
	for _, phase := range []string{m2EconomyPhaseAccrue, m2EconomyPhasePay, m2EconomyRentPhaseAccrue, m2EconomyRentPhasePay, m2StorePhaseBuy, m2FoodConsumePhase, m2RestockPhase, m2ServicePhase, m2WageRetryPhase, m2RentRetryPhase, m2DefaultReviewPhase, m2InsolvencyPhase, m2EstateContributionPhase, m2EstateDistributionPhase} {
		if err := execAgentOne(ctx, tx.conn, "register M2 wage phase", `INSERT INTO scheduler_phases(phase_id, description, ruleset_hash) VALUES (?, 'M2 finite Cohort wage', ?)`, phase, rulesetHash); err != nil {
			return M2EconomicSetupResult{}, err
		}
	}
	for day := 1; day <= 30; day++ {
		for _, step := range []struct{ phase, kind, at, item string }{
			{m2EconomyPhaseAccrue, "m2_wage_accrue", m2WageTime(day, 7, 0), fmt.Sprintf("sched_m2_wage_accrue_day_%d", day)},
			{m2EconomyPhasePay, "m2_wage_pay", m2WageTime(day, 7, 1), fmt.Sprintf("sched_m2_wage_pay_day_%d", day)},
			{m2EconomyRentPhaseAccrue, "m2_rent_accrue", m2WageTime(day, 7, 2), fmt.Sprintf("sched_m2_rent_accrue_day_%d", day)},
			{m2EconomyRentPhasePay, "m2_rent_pay", m2WageTime(day, 7, 3), fmt.Sprintf("sched_m2_rent_pay_day_%d", day)},
			{m2StorePhaseBuy, "m2_food_buy", m2WageTime(day, 7, 4), fmt.Sprintf("sched_m2_food_buy_day_%d", day)},
			{m2FoodConsumePhase, "m2_food_consume", m2WageTime(day, 7, 6), fmt.Sprintf("sched_m2_food_consume_day_%d", day)},
		} {
			contractID := m2EconomyContractID
			if step.phase == m2EconomyRentPhaseAccrue || step.phase == m2EconomyRentPhasePay {
				contractID = m2EconomyRentContract
			}
			if step.phase == m2StorePhaseBuy {
				contractID = m2StoreOfferID
			}
			if step.phase == m2FoodConsumePhase {
				contractID = m2StoreOfferID
			}
			itemJSON, err := core.CanonicalJSON(scheduledPayload{Kind: step.kind, Day: day, SubjectID: contractID})
			if err != nil {
				return M2EconomicSetupResult{}, err
			}
			if err := execAgentOne(ctx, tx.conn, "schedule M2 wage", `INSERT INTO scheduler_items(scheduler_item_id, instance_id, branch_id, world_time, phase_id, declared_priority, status, payload) VALUES (?, ?, ?, ?, ?, 0, 'pending', ?)`, step.item, M2DemoInstanceID, M2DemoBranchID, step.at, step.phase, string(itemJSON)); err != nil {
				return M2EconomicSetupResult{}, err
			}
		}
	}
	probeJSON, err := core.CanonicalJSON(scheduledPayload{Kind: "m2_food_buy", Day: 1, SubjectID: m2StoreOfferID})
	if err != nil {
		return M2EconomicSetupResult{}, err
	}
	if err := execAgentOne(ctx, tx.conn, "schedule M2 food budget boundary", `INSERT INTO scheduler_items(scheduler_item_id, instance_id, branch_id, world_time, phase_id, declared_priority, status, payload) VALUES ('sched_m2_food_buy_budget_probe_day_1', ?, ?, ?, ?, 0, 'pending', ?)`, M2DemoInstanceID, M2DemoBranchID, m2WageTime(1, 7, 5), m2StorePhaseBuy, string(probeJSON)); err != nil {
		return M2EconomicSetupResult{}, err
	}
	for _, day := range []int{26, 27} {
		itemJSON, err := core.CanonicalJSON(scheduledPayload{Kind: "m2_store_restock", Day: day, SubjectID: m2SupplierQuoteID})
		if err != nil {
			return M2EconomicSetupResult{}, err
		}
		if err := execAgentOne(ctx, tx.conn, "schedule M2 finite restock", `INSERT INTO scheduler_items(scheduler_item_id, instance_id, branch_id, world_time, phase_id, declared_priority, status, payload) VALUES (?, ?, ?, ?, ?, 0, 'pending', ?)`, fmt.Sprintf("sched_m2_store_restock_day_%d", day), M2DemoInstanceID, M2DemoBranchID, m2WageTime(day, 7, 7), m2RestockPhase, string(itemJSON)); err != nil {
			return M2EconomicSetupResult{}, err
		}
	}
	for day := 1; day <= 30; day++ {
		steps := []struct {
			phase, kind, subject string
			hour, minute         int
		}{
			{m2DefaultReviewPhase, "m2_default_review", M2DemoCohortID, 7, 11},
		}
		if day >= 2 {
			steps = append(steps,
				struct {
					phase, kind, subject string
					hour, minute         int
				}{m2WageRetryPhase, "m2_wage_arrears_retry", m2EconomyContractID, 7, 9},
				struct {
					phase, kind, subject string
					hour, minute         int
				}{m2RentRetryPhase, "m2_rent_arrears_retry", m2EconomyRentContract, 7, 10},
			)
		}
		if day == 15 {
			steps = append(steps, struct {
				phase, kind, subject string
				hour, minute         int
			}{m2ServicePhase, "m2_maintenance_service", m2ServiceOrderID, 7, 8})
		}
		for _, step := range steps {
			itemJSON, err := core.CanonicalJSON(scheduledPayload{Kind: step.kind, Day: day, SubjectID: step.subject})
			if err != nil {
				return M2EconomicSetupResult{}, err
			}
			if err := execAgentOne(ctx, tx.conn, "schedule M2 arrears and default", `INSERT INTO scheduler_items(scheduler_item_id, instance_id, branch_id, world_time, phase_id, declared_priority, status, payload) VALUES (?, ?, ?, ?, ?, 0, 'pending', ?)`, fmt.Sprintf("sched_%s_day_%d", step.kind, day), M2DemoInstanceID, M2DemoBranchID, m2WageTime(day, step.hour, step.minute), step.phase, string(itemJSON)); err != nil {
				return M2EconomicSetupResult{}, err
			}
		}
	}
	insolvencyJSON, err := core.CanonicalJSON(scheduledPayload{Kind: "m2_insolvency_review", Day: 30, SubjectID: m2InsolvencyPolicyID})
	if err != nil {
		return M2EconomicSetupResult{}, err
	}
	if err := execAgentOne(ctx, tx.conn, "schedule M2 employer insolvency review", `INSERT INTO scheduler_items(scheduler_item_id, instance_id, branch_id, world_time, phase_id, declared_priority, status, payload) VALUES ('sched_m2_insolvency_review_day_30', ?, ?, ?, ?, 0, 'pending', ?)`, M2DemoInstanceID, M2DemoBranchID, m2WageTime(30, 7, 12), m2InsolvencyPhase, string(insolvencyJSON)); err != nil {
		return M2EconomicSetupResult{}, err
	}
	for _, step := range []struct{ itemID, phase, kind, at string }{
		{"sched_m2_estate_contribution_day_31", m2EstateContributionPhase, "m2_estate_contribution", m2WageTime(31, 7, 0)},
		{"sched_m2_estate_distribution_day_31", m2EstateDistributionPhase, "m2_estate_distribution", m2WageTime(31, 7, 1)},
	} {
		itemJSON, err := core.CanonicalJSON(scheduledPayload{Kind: step.kind, Day: 31, SubjectID: m2EstatePolicyID})
		if err != nil {
			return M2EconomicSetupResult{}, err
		}
		if err := execAgentOne(ctx, tx.conn, "schedule M2 estate action", `INSERT INTO scheduler_items(scheduler_item_id, instance_id, branch_id, world_time, phase_id, declared_priority, status, payload) VALUES (?, ?, ?, ?, ?, 0, 'pending', ?)`, step.itemID, M2DemoInstanceID, M2DemoBranchID, step.at, step.phase, string(itemJSON)); err != nil {
			return M2EconomicSetupResult{}, err
		}
	}
	if err := execAgentOne(ctx, tx.conn, "advance M2 economy head", `UPDATE branches SET head_sequence = ? WHERE instance_id = ? AND branch_id = ? AND head_sequence = ?`, sequence, M2DemoInstanceID, M2DemoBranchID, head); err != nil {
		return M2EconomicSetupResult{}, err
	}
	if err := insertAgentOutbox(ctx, tx.conn, "outbox_"+m2EconomyCommandID, m2EconomyEventID, "economy.defined", payloadJSON); err != nil {
		return M2EconomicSetupResult{}, err
	}
	if err := execAgentOne(ctx, tx.conn, "commit M2 economy attempt", `UPDATE command_attempts SET status = 'committed', finished_at_utc = ? WHERE command_id = ? AND attempt_no = 1 AND status = 'ready'`, nowText, m2EconomyCommandID); err != nil {
		return M2EconomicSetupResult{}, err
	}
	if err := execAgentOne(ctx, tx.conn, "commit M2 economy command", `UPDATE commands SET status = 'committed' WHERE command_id = ? AND status = 'pending'`, m2EconomyCommandID); err != nil {
		return M2EconomicSetupResult{}, err
	}
	if s.beforeCommit != nil {
		if err := s.beforeCommit(); err != nil {
			return M2EconomicSetupResult{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return M2EconomicSetupResult{}, core.WrapError(core.CodeStorageFailure, "commit M2 economy setup", err)
	}
	return M2EconomicSetupResult{sequence, false}, nil
}

func prepareM2Wage(ctx context.Context, conn *sql.Conn, item SchedulerItem, payload scheduledPayload) (scheduledMutation, error) {
	if payload.Day < 1 || payload.Day > 30 || payload.SubjectID != m2EconomyContractID ||
		(item.PhaseID == m2EconomyPhaseAccrue && (payload.Kind != "m2_wage_accrue" || item.WorldTime != m2WageTime(payload.Day, 7, 0))) ||
		(item.PhaseID == m2EconomyPhasePay && (payload.Kind != "m2_wage_pay" || item.WorldTime != m2WageTime(payload.Day, 7, 1))) {
		return scheduledMutation{}, core.NewError(core.CodeStorageFailure, "invalid M2 wage task")
	}
	obligationID := fmt.Sprintf("obligation_m2_wage_day_%d", payload.Day)
	var workers, rate int64
	var currency, employer string
	if err := conn.QueryRowContext(ctx, `SELECT c.participant_count, c.unit_rate_minor, c.currency_id, a.cash_account_id FROM m2_cohort_contracts c JOIN m2_economic_actors a ON a.actor_id = c.actor_id JOIN cohorts h ON h.cohort_id = c.cohort_id WHERE c.contract_id = ? AND c.kind = 'wage' AND c.cohort_id = ? AND a.instance_id = ? AND a.branch_id = ? AND h.instance_id = a.instance_id AND h.branch_id = a.branch_id AND a.kind = 'employer' AND c.currency_id = h.currency_id`, payload.SubjectID, M2DemoCohortID, M2DemoInstanceID, M2DemoBranchID).Scan(&workers, &rate, &currency, &employer); err != nil {
		return scheduledMutation{}, classifyMissing(err, "M2 wage contract")
	}
	if err := verifyM2WageParticipationReturns(ctx, conn); err != nil {
		return scheduledMutation{}, err
	}
	if item.PhaseID == m2EconomyPhasePay {
		var inactive int
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE instance_id=? AND branch_id=? AND event_type='M2WagePeriodInactive' AND world_time=? AND json_extract(payload,'$.contract_id')=?`, M2DemoInstanceID, M2DemoBranchID, m2WageTime(payload.Day, 7, 0), payload.SubjectID).Scan(&inactive); err != nil {
			return scheduledMutation{}, err
		}
		if inactive == 1 {
			return scheduledMutation{EventType: "M2WageSettlementSkipped", EventPayload: struct {
				ContractID string `json:"contract_id"`
				Day        int    `json:"day"`
				Reason     string `json:"reason_code"`
			}{payload.SubjectID, payload.Day, "no_active_workers"}}, nil
		}
		hasTransfers, err := hasM2WageOwnerTransitions(ctx, conn, obligationID)
		if err != nil {
			return scheduledMutation{}, err
		}
		if hasTransfers {
			due, _, _, _, err := readM2WageOrigin(ctx, conn, obligationID)
			if err != nil {
				return scheduledMutation{}, err
			}
			return prepareM2SlotAwareWageDue(ctx, conn, item, payload, obligationID, due)
		}
	}
	var activeSplits, obligationSlices int64
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM m2_wage_participation_splits s JOIN cohort_materializations m ON m.materialization_id = s.materialization_id AND m.status = 'active' WHERE s.contract_id = ? AND s.effective_from <= ?`, payload.SubjectID, m2WageTime(payload.Day, 7, 0)).Scan(&activeSplits); err != nil {
		return scheduledMutation{}, core.WrapError(core.CodeStorageFailure, "count active wage participants", err)
	}
	if item.PhaseID == m2EconomyPhasePay {
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM m2_wage_split_obligations WHERE obligation_id = ?`, obligationID).Scan(&obligationSlices); err != nil {
			return scheduledMutation{}, core.WrapError(core.CodeStorageFailure, "count wage claimant slices", err)
		}
	}
	departures, err := readM2WageDepartures(ctx, conn, m2WageTime(payload.Day, 7, 0))
	if err != nil {
		return scheduledMutation{}, err
	}
	if activeSplits > 0 || obligationSlices > 0 || len(departures) > 0 {
		if item.PhaseID == m2EconomyPhasePay && obligationSlices == 0 {
			return scheduledMutation{}, core.NewError(core.CodeProjectionDiverged, "wage split missing claimant slices")
		}
		return prepareM2SplitWage(ctx, conn, item, payload, workers, rate, currency, employer)
	}
	var population int64
	if err := conn.QueryRowContext(ctx, `SELECT population_count FROM cohorts WHERE cohort_id = ?`, M2DemoCohortID).Scan(&population); err != nil {
		return scheduledMutation{}, classifyMissing(err, "M2 wage Cohort")
	}
	if item.PhaseID == m2EconomyPhaseAccrue && population != workers {
		return scheduledMutation{}, core.NewError(core.CodeProjectionDiverged, "wage population differs from unsplit contract")
	}
	if item.PhaseID == m2EconomyPhasePay {
		var err error
		_, _, rate, workers, err = readM2WageOrigin(ctx, conn, obligationID)
		if err != nil {
			return scheduledMutation{}, err
		}
	}
	for _, accountID := range []string{employer, M2DemoCohortAssetAccountID, m2EconomyEmployerExpense, m2EconomyEmployerPayable, M2DemoCohortReceivableID, m2EconomyCohortIncome} {
		if err := verifyM2AccountProjection(ctx, conn, accountID, currency); err != nil {
			return scheduledMutation{}, err
		}
	}
	amount, ok := checkedMultiplyPositive(workers, rate)
	if !ok {
		return scheduledMutation{}, core.NewError(core.CodeIntegerOverflow, "M2 wage exceeds integer range")
	}
	ledger := obligationLedger{m2EconomyEmployerExpense, m2EconomyEmployerPayable, M2DemoCohortReceivableID, m2EconomyCohortIncome, currency}
	if item.PhaseID == m2EconomyPhaseAccrue {
		postings, balances, err := prepareAccrualAccounting(ctx, conn, ledger, amount, "M2 aggregate wage accrual")
		if err != nil {
			return scheduledMutation{}, err
		}
		return scheduledMutation{EventType: "M2WageAccrued", EventPayload: struct {
			ContractID string `json:"contract_id"`
			Workers    int64  `json:"worker_count"`
			Rate       int64  `json:"unit_wage_minor"`
			Amount     int64  `json:"amount_minor"`
		}{payload.SubjectID, workers, rate, amount}, Postings: postings, Balances: balances,
			ApplyDomainRows: func(ctx context.Context, conn *sql.Conn, eventID string, sequence int64) error {
				return execAgentOne(ctx, conn, "record M2 wage obligation", `INSERT INTO m2_economic_obligations(obligation_id, contract_id, period_start, period_end, kind, amount_due_minor, status, defining_event_id, last_event_sequence) VALUES (?, ?, ?, ?, 'wage', ?, 'accrued', ?, ?)`, obligationID, payload.SubjectID, m2WageTime(payload.Day-1, 7, 0), item.WorldTime, amount, eventID, sequence)
			},
		}, nil
	}
	var due, paid int64
	if err := conn.QueryRowContext(ctx, `SELECT amount_due_minor, amount_paid_minor FROM m2_economic_obligations WHERE obligation_id = ? AND contract_id = ? AND kind = 'wage'`, obligationID, payload.SubjectID).Scan(&due, &paid); err != nil {
		return scheduledMutation{}, classifyMissing(err, "M2 wage obligation")
	}
	if due != amount || paid != 0 {
		return scheduledMutation{}, core.NewError(core.CodeProjectionDiverged, "M2 wage obligation differs from contract")
	}
	employerBalance, employerVersion, err := readScheduledBalance(ctx, conn, employer)
	if err != nil {
		return scheduledMutation{}, err
	}
	cohortBalance, cohortVersion, err := readScheduledBalance(ctx, conn, M2DemoCohortAssetAccountID)
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
	if employerBalance < 0 || cohortBalance < 0 || payableBalance > -due || receivableBalance < due {
		return scheduledMutation{}, core.NewError(core.CodeProjectionDiverged, "M2 wage ledger is inconsistent")
	}
	payment := amount
	if employerBalance < payment {
		payment = employerBalance
	}
	status, reason := "paid", ""
	if payment < amount {
		reason = "insufficient_employer_liquidity"
		status = "partial"
		if payment == 0 {
			status = "overdue"
		}
	}
	mutation := scheduledMutation{EventType: "M2WageSettled", EventPayload: struct {
		ObligationID string `json:"obligation_id"`
		Status       string `json:"status"`
		Reason       string `json:"reason_code,omitempty"`
		Due          int64  `json:"due_minor"`
		Paid         int64  `json:"paid_minor"`
		Remaining    int64  `json:"remaining_minor"`
	}{obligationID, status, reason, amount, payment, amount - payment}}
	if payment > 0 {
		newCohort, ok := checkedAdd(cohortBalance, payment)
		if !ok {
			return scheduledMutation{}, core.NewError(core.CodeIntegerOverflow, "M2 wage credit overflows")
		}
		newPayable, ok := checkedAdd(payableBalance, payment)
		if !ok {
			return scheduledMutation{}, core.NewError(core.CodeIntegerOverflow, "M2 wage payable overflows")
		}
		newReceivable, ok := checkedSubtract(receivableBalance, payment)
		if !ok {
			return scheduledMutation{}, core.NewError(core.CodeIntegerOverflow, "M2 wage receivable overflows")
		}
		mutation.Postings = []scheduledPosting{{employer, currency, -payment, "M2 wage paid"}, {M2DemoCohortAssetAccountID, currency, payment, "M2 wage received"}, {ledger.PayableAccountID, currency, payment, "M2 wage payable cleared"}, {ledger.ReceivableAccountID, currency, -payment, "M2 wage receivable cleared"}}
		mutation.Balances = []balanceMutation{{employer, employerVersion, employerBalance - payment}, {M2DemoCohortAssetAccountID, cohortVersion, newCohort}, {ledger.PayableAccountID, payableVersion, newPayable}, {ledger.ReceivableAccountID, receivableVersion, newReceivable}}
	}
	mutation.ApplyDomainRows = func(ctx context.Context, conn *sql.Conn, eventID string, sequence int64) error {
		if err := execAgentOne(ctx, conn, "settle M2 wage obligation", `UPDATE m2_economic_obligations SET amount_paid_minor = ?, status = ?, last_event_sequence = ? WHERE obligation_id = ? AND amount_paid_minor = 0 AND status = 'accrued'`, payment, status, sequence, obligationID); err != nil {
			return err
		}
		if payment < amount {
			return recordM2ArrearsCase(ctx, conn, obligationID, m2EconomyContractID, "wage", payload.Day, 3, eventID, sequence)
		}
		return nil
	}
	return mutation, nil
}

func m2WageTime(day, hour, minute int) string {
	return time.Date(2026, time.September, 22+day, hour, minute, 0, 0, time.UTC).Format(time.RFC3339)
}

func (s *Store) executeNextM2Economy(ctx context.Context, tx *immediateTx, item SchedulerItem) error {
	var payload scheduledPayload
	if err := json.Unmarshal([]byte(item.Payload), &payload); err != nil {
		return core.WrapError(core.CodeStorageFailure, "decode M2 economic task", err)
	}
	var mutation scheduledMutation
	var err error
	if item.PhaseID == m2EstateContributionPhase {
		mutation, err = prepareM2EstateContribution(ctx, tx.conn, item, payload)
	} else if item.PhaseID == m2EstateDistributionPhase {
		mutation, err = prepareM2EstateDistribution(ctx, tx.conn, item, payload)
	} else if item.PhaseID == m2ServicePhase {
		mutation, err = prepareM2MaintenanceService(ctx, tx.conn, item, payload)
	} else if item.PhaseID == m2WageRetryPhase || item.PhaseID == m2RentRetryPhase {
		mutation, err = prepareM2ArrearsRetry(ctx, tx.conn, item, payload)
	} else if item.PhaseID == m2DefaultReviewPhase {
		mutation, err = prepareM2DefaultReview(ctx, tx.conn, item, payload)
	} else if item.PhaseID == m2InsolvencyPhase {
		mutation, err = prepareM2InsolvencyReview(ctx, tx.conn, item, payload)
	} else if item.PhaseID == m2FoodConsumePhase {
		mutation, err = prepareM2Consumption(ctx, tx.conn, item, payload)
	} else if item.PhaseID == m2RestockPhase {
		mutation, err = prepareM2Restock(ctx, tx.conn, item, payload)
	} else if item.PhaseID == m2StorePhaseBuy {
		mutation, err = prepareM2Purchase(ctx, tx.conn, item, payload)
	} else if item.PhaseID == m2EconomyRentPhaseAccrue || item.PhaseID == m2EconomyRentPhasePay {
		mutation, err = prepareM2Rent(ctx, tx.conn, item, payload)
	} else {
		mutation, err = prepareM2Wage(ctx, tx.conn, item, payload)
	}
	if err != nil {
		return err
	}
	if err := s.commitScheduledMutationForBranch(ctx, tx, item, payload, mutation, M2DemoInstanceID, M2DemoBranchID); err != nil {
		return err
	}
	if s.beforeCommit != nil {
		if err := s.beforeCommit(); err != nil {
			return err
		}
	}
	return nil
}

// A missing source of cash is a world fact, not a guessed projection value.
// Compare the account projection to posted authority before deciding arrears.
func verifyM2AccountProjection(ctx context.Context, conn *sql.Conn, accountID, currencyID string) error {
	var projected, authoritative int64
	var accountCurrency string
	if err := conn.QueryRowContext(ctx, `SELECT b.balance_minor, a.currency_id FROM account_balances b JOIN accounts a ON a.account_id = b.account_id WHERE b.account_id = ?`, accountID).Scan(&projected, &accountCurrency); err != nil {
		return classifyMissing(err, "M2 wage account projection")
	}
	if accountCurrency != currencyID {
		return core.NewError(core.CodeProjectionDiverged, "M2 wage account currency differs from contract")
	}
	if err := conn.QueryRowContext(ctx, `SELECT COALESCE(SUM(p.amount_minor), 0) FROM postings p JOIN journal_entries j ON j.entry_id = p.entry_id AND j.status = 'posted' JOIN events e ON e.event_id = j.event_id WHERE e.instance_id = ? AND e.branch_id = ? AND p.account_id = ?`, M2DemoInstanceID, M2DemoBranchID, accountID).Scan(&authoritative); err != nil {
		return core.WrapError(core.CodeStorageFailure, "replay M2 wage account postings", err)
	}
	if projected != authoritative {
		return core.NewError(core.CodeProjectionDiverged, "M2 wage balance differs from posted facts: "+accountID)
	}
	return nil
}

// Population changes must preserve active wage shares or prove that only
// unemployed capacity is affected. The one-party rent household must remain.
func ensureM2ContractsPermitPopulationTransition(ctx context.Context, conn *sql.Conn, cohortID, worldTime string, remainingPopulation int64, wageSplit bool, returningMaterialization string) error {
	at, err := time.Parse(time.RFC3339, worldTime)
	if err != nil {
		return core.WrapError(core.CodeInvalidArgument, "invalid M2 contract transition time", err)
	}
	noEmployedCohort := false
	if !wageSplit && cohortID == M2DemoCohortID && remainingPopulation > 0 {
		ended, err := readM2WageDepartures(ctx, conn, at.UTC().Format(time.RFC3339))
		if err != nil {
			return err
		}
		if len(ended) > 0 {
			var baseline, employedNamed int64
			if err := conn.QueryRowContext(ctx, `SELECT participant_count FROM m2_cohort_contracts WHERE contract_id=?`, m2EconomyContractID).Scan(&baseline); err != nil {
				return err
			}
			if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM m2_wage_participation_splits s JOIN cohort_materializations m ON m.materialization_id=s.materialization_id AND m.status='active' WHERE s.contract_id=? AND NOT EXISTS (SELECT 1 FROM events e WHERE e.instance_id=? AND e.branch_id=? AND e.event_type='CareerAggregateExitActivated' AND json_extract(e.payload,'$.materialization_id')=s.materialization_id AND e.world_time<=?)`, m2EconomyContractID, M2DemoInstanceID, M2DemoBranchID, at.UTC().Format(time.RFC3339)).Scan(&employedNamed); err != nil {
				return err
			}
			noEmployedCohort = baseline == int64(len(ended))+employedNamed
			if noEmployedCohort && returningMaterialization != "" {
				var belongs int
				if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM cohort_materializations m JOIN m2_cohort_contracts w ON w.cohort_id=m.source_cohort_id JOIN events e ON e.event_id=w.definition_event_id WHERE m.materialization_id=? AND w.contract_id=? AND m.materialize_sequence>e.event_sequence`, returningMaterialization, m2EconomyContractID).Scan(&belongs); err != nil {
					return err
				}
				noEmployedCohort = belongs == 1
			}
		}
	}
	rows, err := conn.QueryContext(ctx, `SELECT kind, participant_count, effective_from, effective_until FROM m2_cohort_contracts WHERE cohort_id = ?`, cohortID)
	if err != nil {
		return core.WrapError(core.CodeStorageFailure, "check M2 Cohort economic contracts", err)
	}
	for rows.Next() {
		var kind, starts string
		var participants int64
		var ends sql.NullString
		if err := rows.Scan(&kind, &participants, &starts, &ends); err != nil {
			rows.Close()
			return core.WrapError(core.CodeStorageFailure, "scan M2 Cohort economic contract", err)
		}
		start, err := time.Parse(time.RFC3339, starts)
		if err != nil {
			rows.Close()
			return core.WrapError(core.CodeProjectionDiverged, "invalid M2 contract start", err)
		}
		active := true
		if ends.Valid {
			end, err := time.Parse(time.RFC3339, ends.String)
			if err != nil || !end.After(start) {
				rows.Close()
				return core.NewError(core.CodeProjectionDiverged, "invalid M2 contract end")
			}
			active = at.Before(end)
		}
		if !active {
			continue
		}
		if kind == "rent" && participants == 1 && remainingPopulation > 0 {
			continue
		}
		if kind == "wage" && (wageSplit || noEmployedCohort) && remainingPopulation > 0 {
			continue
		}
		rows.Close()
		return core.NewError(core.CodeMaterializationConflict, "active aggregate economic contract must be split before changing Cohort population")
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return core.WrapError(core.CodeStorageFailure, "iterate M2 Cohort economic contracts", err)
	}
	return rows.Close()
}
