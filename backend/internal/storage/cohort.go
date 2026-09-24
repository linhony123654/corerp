package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"corerp.local/backend/internal/core"
)

type materializationEventPayload struct {
	MaterializationID          string `json:"materialization_id"`
	SourceCohortID             string `json:"source_cohort_id"`
	EntityID                   string `json:"entity_id"`
	DisplayName                string `json:"display_name"`
	PopulationCount            int64  `json:"population_count"`
	AssetMinor                 int64  `json:"asset_minor"`
	InventoryMinor             int64  `json:"inventory_minor"`
	ReceivableMinor            int64  `json:"receivable_minor"`
	LiabilityMinor             int64  `json:"liability_minor"`
	AllocationAlgorithmVersion string `json:"allocation_algorithm_version"`
	ClaimAllocationCount       int    `json:"claim_allocation_count,omitempty"`
	ClaimAllocationHash        string `json:"claim_allocation_hash,omitempty"`
	WageParticipationHash      string `json:"wage_participation_hash,omitempty"`
	WageClaimTransferCount     int    `json:"wage_claim_transfer_count,omitempty"`
	WageClaimTransferHash      string `json:"wage_claim_transfer_hash,omitempty"`
}

type dematerializationEventPayload struct {
	MaterializationID    string `json:"materialization_id"`
	SourceCohortID       string `json:"source_cohort_id"`
	EntityID             string `json:"entity_id"`
	PopulationCount      int64  `json:"population_count"`
	AssetMinor           int64  `json:"asset_minor"`
	InventoryMinor       int64  `json:"inventory_minor"`
	ReceivableMinor      int64  `json:"receivable_minor"`
	LiabilityMinor       int64  `json:"liability_minor"`
	ReasonCode           string `json:"reason_code"`
	ClaimReturnCount     int    `json:"claim_return_count,omitempty"`
	ClaimReturnHash      string `json:"claim_return_hash,omitempty"`
	WageClaimReturnCount int    `json:"wage_claim_return_count,omitempty"`
	WageClaimReturnHash  string `json:"wage_claim_return_hash,omitempty"`
}

func (s *Store) MaterializeCohort(ctx context.Context, command core.MaterializeCohortCommand) (core.CohortTransitionResult, error) {
	if err := command.Validate(); err != nil {
		return core.CohortTransitionResult{}, err
	}
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return core.CohortTransitionResult{}, core.WrapError(core.CodeStorageFailure, "begin Cohort materialization", err)
	}
	defer tx.Rollback(ctx)
	result, err := s.materializeCohortInTransaction(ctx, tx.conn, command, func() (string, error) {
		return authorizeCohortMaterialization(ctx, tx.conn, command)
	})
	if err != nil || result.Replayed {
		return result, err
	}
	if s.beforeCommit != nil {
		if err := s.beforeCommit(); err != nil {
			return core.CohortTransitionResult{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return core.CohortTransitionResult{}, core.WrapError(core.CodeStorageFailure, "commit Cohort materialization", err)
	}
	return result, nil
}

func authorizeCohortMaterialization(ctx context.Context, conn *sql.Conn, command core.MaterializeCohortCommand) (string, error) {
	var grantID string
	if err := conn.QueryRowContext(ctx, `
		SELECT g.grant_id FROM capability_grants g JOIN principals p ON p.principal_id = g.principal_id
		WHERE g.principal_id = ? AND p.status = 'active' AND g.capability_id = ?
		  AND g.instance_id = ? AND g.branch_id = ? AND g.subject_id IN (?, '*') AND g.status = 'active'
		ORDER BY CASE WHEN g.subject_id = ? THEN 0 ELSE 1 END, g.grant_id LIMIT 1`,
		command.PrincipalID, command.CapabilityID, command.InstanceID, command.BranchID,
		command.SourceCohortID, command.SourceCohortID,
	).Scan(&grantID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", core.NewError(core.CodeUnauthorized, "principal lacks Cohort materialization scope")
		}
		return "", core.WrapError(core.CodeStorageFailure, "read Cohort materialization grant", err)
	}

	return fmt.Sprintf(`{"authorization":"scoped-capability","grant_id":%q}`, grantID), nil
}

// The caller owns the transaction and supplies the authority appropriate to its
// workflow. Public materialization still checks its original scoped grant;
// Studio genesis binds only declared participants to a freshly authorized source.
func (s *Store) materializeCohortInTransaction(ctx context.Context, conn *sql.Conn, command core.MaterializeCohortCommand, authorize func() (string, error)) (core.CohortTransitionResult, error) {
	if err := command.Validate(); err != nil {
		return core.CohortTransitionResult{}, err
	}
	requestHash, err := core.MaterializeCohortRequestHash(command)
	if err != nil {
		return core.CohortTransitionResult{}, err
	}
	if result, found, err := lookupCohortCommandResult(ctx, conn, command.InstanceID, command.BranchID, core.MaterializeCohortCommandType, command.IdempotencyKey, requestHash); err != nil {
		return core.CohortTransitionResult{}, err
	} else if found {
		return result, nil
	}
	var materializeCommandID, materializationHash string
	err = conn.QueryRowContext(ctx, `
		SELECT m.materialize_command_id, c.request_hash
		FROM cohort_materializations m JOIN commands c ON c.command_id = m.materialize_command_id
		WHERE m.materialization_id = ?`, command.MaterializationID,
	).Scan(&materializeCommandID, &materializationHash)
	if err == nil {
		if materializationHash != requestHash {
			return core.CohortTransitionResult{}, core.NewError(core.CodeMaterializationConflict, "materialization_id was already used with a different request")
		}
		result, err := loadCohortTransitionResult(ctx, conn, materializeCommandID, command.MaterializationID)
		if err != nil {
			return core.CohortTransitionResult{}, err
		}
		result.Replayed = true
		return result, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return core.CohortTransitionResult{}, core.WrapError(core.CodeStorageFailure, "look up materialization identity", err)
	}

	var head int64
	if err := conn.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id = ? AND branch_id = ?`, command.InstanceID, command.BranchID).Scan(&head); err != nil {
		return core.CohortTransitionResult{}, classifyMissing(err, "materialization branch")
	}
	if head != command.ExpectedHead {
		return core.CohortTransitionResult{}, core.NewError(core.CodeBranchConflict, fmt.Sprintf("expected head %d, current head %d", command.ExpectedHead, head))
	}
	if err := ensureCohortTransitionChronology(ctx, conn, command.InstanceID, command.BranchID, command.WorldTime); err != nil {
		return core.CohortTransitionResult{}, err
	}
	sequence := head + 1
	var epochID string
	if err := conn.QueryRowContext(ctx, `SELECT epoch_id FROM rule_epochs WHERE instance_id = ? AND branch_id = ? AND start_sequence <= ? AND (end_sequence IS NULL OR ? < end_sequence)`, command.InstanceID, command.BranchID, sequence, sequence).Scan(&epochID); err != nil {
		return core.CohortTransitionResult{}, classifyMissing(err, "materialization Rule Epoch")
	}
	policyDocument, err := authorize()
	if err != nil {
		return core.CohortTransitionResult{}, err
	}

	type cohortProjection struct {
		population, version, asset, assetVersion, receivable, receivableVersion int64
		liability, liabilityVersion, inventory, inventoryVersion                int64
		assetAccount, receivableAccount, liabilityAccount, locationID           string
		currencyID, skuID, algorithmVersion, status                             string
	}
	var cohort cohortProjection
	if err := conn.QueryRowContext(ctx, `
		SELECT c.population_count, c.projection_version,
		       c.asset_account_id, ab.balance_minor, ab.projection_version,
		       c.receivable_account_id, rb.balance_minor, rb.projection_version,
		       c.liability_account_id, lb.balance_minor, lb.projection_version,
		       c.inventory_location_id, ib.quantity_minor, ib.projection_version,
		       c.currency_id, c.sku_id, c.allocation_algorithm_version, c.status
		FROM cohorts c
		JOIN account_balances ab ON ab.account_id = c.asset_account_id
		JOIN account_balances rb ON rb.account_id = c.receivable_account_id
		JOIN account_balances lb ON lb.account_id = c.liability_account_id
		JOIN inventory_balances ib ON ib.location_id = c.inventory_location_id AND ib.sku_id = c.sku_id
		WHERE c.cohort_id = ? AND c.instance_id = ? AND c.branch_id = ?`,
		command.SourceCohortID, command.InstanceID, command.BranchID,
	).Scan(
		&cohort.population, &cohort.version,
		&cohort.assetAccount, &cohort.asset, &cohort.assetVersion,
		&cohort.receivableAccount, &cohort.receivable, &cohort.receivableVersion,
		&cohort.liabilityAccount, &cohort.liability, &cohort.liabilityVersion,
		&cohort.locationID, &cohort.inventory, &cohort.inventoryVersion,
		&cohort.currencyID, &cohort.skuID, &cohort.algorithmVersion, &cohort.status,
	); err != nil {
		return core.CohortTransitionResult{}, classifyMissing(err, "source Cohort")
	}
	if cohort.status != "active" || cohort.algorithmVersion != command.AllocationAlgorithmVersion {
		return core.CohortTransitionResult{}, core.NewError(core.CodeConservationFailed, "Cohort is not active under the requested allocation algorithm")
	}
	newPopulation, populationOK := checkedSubtract(cohort.population, command.PopulationCount)
	newAsset, assetOK := checkedSubtract(cohort.asset, command.AssetMinor)
	newReceivable, receivableOK := checkedSubtract(cohort.receivable, command.ReceivableMinor)
	newLiability, liabilityOK := checkedAdd(cohort.liability, command.LiabilityMinor)
	newInventory, inventoryOK := checkedSubtract(cohort.inventory, command.InventoryMinor)
	if !populationOK || !assetOK || !receivableOK || !liabilityOK || !inventoryOK ||
		newPopulation < 0 || newAsset < 0 || newReceivable < 0 || newLiability > 0 || newInventory < 0 {
		return core.CohortTransitionResult{}, core.NewError(core.CodeConservationFailed, "Cohort allocation exceeds a conserved balance")
	}
	wageSplit, err := planM2WageParticipation(ctx, conn, command, cohort.population, newPopulation)
	if err != nil {
		return core.CohortTransitionResult{}, err
	}
	var wageTransfers []m2WageClaimTransition
	if wageSplit != nil {
		wageTransfers, err = planM2WageClaimSlotTransfer(ctx, conn, command)
		if err != nil {
			return core.CohortTransitionResult{}, err
		}
	}
	if err := ensureM2ContractsPermitPopulationTransition(ctx, conn, command.SourceCohortID, command.WorldTime, newPopulation, wageSplit != nil, ""); err != nil {
		return core.CohortTransitionResult{}, err
	}
	var entityCount int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM materialized_entities WHERE entity_id = ?`, command.EntityID).Scan(&entityCount); err != nil {
		return core.CohortTransitionResult{}, core.WrapError(core.CodeStorageFailure, "check stable entity identity", err)
	}
	if entityCount != 0 {
		return core.CohortTransitionResult{}, core.NewError(core.CodeMaterializationConflict, "entity_id is already materialized")
	}
	var claimShares []m2ClaimShare
	if wageSplit == nil {
		claimShares, err = planM2ClaimAllocation(ctx, conn, command.SourceCohortID, cohort.population, command.PopulationCount, command.ReceivableMinor)
		if err != nil {
			return core.CohortTransitionResult{}, err
		}
	}
	claimHash := ""
	if len(claimShares) > 0 {
		claimHash, err = core.HashJSON(claimShares)
		if err != nil {
			return core.CohortTransitionResult{}, err
		}
	}

	payload := materializationEventPayload{MaterializationID: command.MaterializationID, SourceCohortID: command.SourceCohortID,
		EntityID: command.EntityID, DisplayName: command.DisplayName, PopulationCount: command.PopulationCount,
		AssetMinor: command.AssetMinor, InventoryMinor: command.InventoryMinor, ReceivableMinor: command.ReceivableMinor,
		LiabilityMinor: command.LiabilityMinor, AllocationAlgorithmVersion: command.AllocationAlgorithmVersion,
		ClaimAllocationCount: len(claimShares), ClaimAllocationHash: claimHash}
	if wageSplit != nil {
		payload.WageParticipationHash, err = core.HashJSON(wageSplit)
		if err != nil {
			return core.CohortTransitionResult{}, err
		}
		if len(wageTransfers) > 0 {
			payload.WageClaimTransferCount = len(wageTransfers)
			payload.WageClaimTransferHash, err = core.HashJSON(wageTransfers)
			if err != nil {
				return core.CohortTransitionResult{}, err
			}
		}
	}
	payloadJSON, err := core.CanonicalJSON(payload)
	if err != nil {
		return core.CohortTransitionResult{}, err
	}
	batchHash, err := core.HashJSON(struct {
		CommandID, InstanceID, BranchID, EpochID string
		ExpectedHead, Sequence                   int64
		WorldTime, EventType                     string
		Payload                                  materializationEventPayload
	}{command.CommandID, command.InstanceID, command.BranchID, epochID, head, sequence, command.WorldTime, "CohortMaterialized", payload})
	if err != nil {
		return core.CohortTransitionResult{}, err
	}
	audience := struct {
		Kind       string `json:"kind"`
		InstanceID string `json:"instance_id"`
	}{"instance", command.InstanceID}
	audienceJSON, err := core.CanonicalJSON(audience)
	if err != nil {
		return core.CohortTransitionResult{}, err
	}
	audienceHash, err := core.HashJSON(audience)
	if err != nil {
		return core.CohortTransitionResult{}, err
	}
	scopeJSON, err := core.CanonicalJSON(struct {
		InstanceID string   `json:"instance_id"`
		BranchID   string   `json:"branch_id"`
		Subjects   []string `json:"subject_ids"`
	}{command.InstanceID, command.BranchID, []string{command.SourceCohortID, command.EntityID}})
	if err != nil {
		return core.CohortTransitionResult{}, err
	}

	now := s.now().UTC()
	nowText := now.Format(time.RFC3339Nano)
	attemptID := "attempt_" + command.CommandID + "_1"
	batchID := "batch_" + command.CommandID
	eventID := "event_" + command.CommandID
	entryID := "journal_" + command.CommandID
	populationMovementID := "population_" + command.CommandID
	stockMovementID := "movement_" + command.CommandID
	outboxID := "outbox_" + command.CommandID
	entityAssetAccount := "account_" + command.MaterializationID + "_asset"
	entityReceivableAccount := "account_" + command.MaterializationID + "_receivable"
	entityLiabilityAccount := "account_" + command.MaterializationID + "_liability"
	entityIncomeAccount := "account_" + command.MaterializationID + "_wage_income"
	entityLocation := "location_" + command.MaterializationID

	statements := []struct {
		name, query string
		args        []any
	}{
		{"materialization command", `INSERT INTO commands(command_id, instance_id, branch_id, command_type, idempotency_key, request_hash, expected_head, principal_id, command_policy, status, created_at_utc) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'pending', ?)`, []any{command.CommandID, command.InstanceID, command.BranchID, core.MaterializeCohortCommandType, command.IdempotencyKey, requestHash, head, command.PrincipalID, policyDocument, nowText}},
		{"materialization attempt", `INSERT INTO command_attempts(command_id, attempt_no, attempt_id, status, lease_owner, lease_until_utc, proposal_hash, created_at_utc) VALUES (?, 1, ?, 'ready', 'corerp-m2', ?, ?, ?)`, []any{command.CommandID, attemptID, now.Add(30 * time.Second).Format(time.RFC3339Nano), requestHash, nowText}},
		{"materialization batch", `INSERT INTO event_batches(batch_id, command_id, attempt_no, instance_id, branch_id, epoch_id, expected_head, first_sequence, last_sequence, event_count, world_time, batch_hash, committed_at_utc) VALUES (?, ?, 1, ?, ?, ?, ?, ?, ?, 1, ?, ?, ?)`, []any{batchID, command.CommandID, command.InstanceID, command.BranchID, epochID, head, sequence, sequence, command.WorldTime, batchHash, nowText}},
		{"materialization event", `INSERT INTO events(event_id, batch_id, instance_id, branch_id, event_sequence, batch_index, event_type, actor_id, world_time, payload) VALUES (?, ?, ?, ?, ?, 0, 'CohortMaterialized', ?, ?, ?)`, []any{eventID, batchID, command.InstanceID, command.BranchID, sequence, command.PrincipalID, command.WorldTime, string(payloadJSON)}},
	}
	for _, account := range []struct{ id, kind string }{{entityAssetAccount, "asset"}, {entityReceivableAccount, "receivable"}, {entityLiabilityAccount, "liability"}} {
		statements = append(statements, struct {
			name, query string
			args        []any
		}{"materialized account " + account.kind, `INSERT INTO accounts(account_id, owner_id, currency_id, account_type, overdraft_limit_minor, opened_by_event_id) VALUES (?, ?, ?, ?, 0, ?)`, []any{account.id, command.EntityID, cohort.currencyID, account.kind, eventID}})
	}
	if wageSplit != nil {
		statements = append(statements, struct {
			name, query string
			args        []any
		}{"materialized wage income account", `INSERT INTO accounts(account_id, owner_id, currency_id, account_type, overdraft_limit_minor, opened_by_event_id) VALUES (?, ?, ?, 'income', 0, ?)`, []any{entityIncomeAccount, command.EntityID, cohort.currencyID, eventID}})
	}
	for _, statement := range statements {
		if _, err := conn.ExecContext(ctx, statement.query, statement.args...); err != nil {
			return core.CohortTransitionResult{}, core.WrapError(core.CodeStorageFailure, "insert "+statement.name, err)
		}
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO stock_locations(location_id, owner_id, location_kind) VALUES (?, ?, 'holder')`, entityLocation, command.EntityID); err != nil {
		return core.CohortTransitionResult{}, core.WrapError(core.CodeStorageFailure, "insert materialized inventory location", err)
	}
	for _, balance := range []struct {
		accountID string
		amount    int64
	}{{entityAssetAccount, command.AssetMinor}, {entityReceivableAccount, command.ReceivableMinor}, {entityLiabilityAccount, -command.LiabilityMinor}} {
		if _, err := conn.ExecContext(ctx, `INSERT INTO account_balances(account_id, balance_minor, projection_version, last_event_sequence) VALUES (?, ?, 1, ?)`, balance.accountID, balance.amount, sequence); err != nil {
			return core.CohortTransitionResult{}, core.WrapError(core.CodeStorageFailure, "insert materialized account projection", err)
		}
	}
	if wageSplit != nil {
		if _, err := conn.ExecContext(ctx, `INSERT INTO account_balances(account_id, balance_minor, projection_version, last_event_sequence) VALUES (?, 0, 1, ?)`, entityIncomeAccount, sequence); err != nil {
			return core.CohortTransitionResult{}, core.WrapError(core.CodeStorageFailure, "initialize named wage income", err)
		}
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO inventory_balances(location_id, sku_id, quantity_minor, projection_version, last_event_sequence) VALUES (?, ?, ?, 1, ?)`, entityLocation, cohort.skuID, command.InventoryMinor, sequence); err != nil {
		return core.CohortTransitionResult{}, core.WrapError(core.CodeStorageFailure, "insert materialized inventory projection", err)
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO materialized_entities(entity_id, source_cohort_id, materialization_id, display_name, population_count, asset_account_id, receivable_account_id, liability_account_id, inventory_location_id, currency_id, sku_id, status, projection_version, last_event_sequence) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'active', 1, ?)`, command.EntityID, command.SourceCohortID, command.MaterializationID, command.DisplayName, command.PopulationCount, entityAssetAccount, entityReceivableAccount, entityLiabilityAccount, entityLocation, cohort.currencyID, cohort.skuID, sequence); err != nil {
		return core.CohortTransitionResult{}, core.WrapError(core.CodeStorageFailure, "insert materialized entity projection", err)
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO cohort_materializations(materialization_id, source_cohort_id, entity_id, allocation_algorithm_version, materialize_command_id, materialize_event_id, materialize_sequence, population_count, asset_minor, inventory_minor, receivable_minor, liability_minor, status) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'active')`, command.MaterializationID, command.SourceCohortID, command.EntityID, command.AllocationAlgorithmVersion, command.CommandID, eventID, sequence, command.PopulationCount, command.AssetMinor, command.InventoryMinor, command.ReceivableMinor, command.LiabilityMinor); err != nil {
		return core.CohortTransitionResult{}, core.WrapError(core.CodeStorageFailure, "insert Cohort materialization lineage", err)
	}
	if wageSplit != nil {
		if err := execAgentOne(ctx, conn, "record named wage participation", `INSERT INTO m2_wage_participation_splits(materialization_id, contract_id, cohort_id, entity_id, worker_count, effective_from, income_account_id, split_event_id, split_event_sequence) VALUES (?, ?, ?, ?, 1, ?, ?, ?, ?)`, command.MaterializationID, wageSplit.ContractID, command.SourceCohortID, command.EntityID, wageSplit.EffectiveFrom, entityIncomeAccount, eventID, sequence); err != nil {
			return core.CohortTransitionResult{}, err
		}
		if err := recordM2WageClaimTransitions(ctx, conn, command.MaterializationID, eventID, sequence, "materialize", wageTransfers); err != nil {
			return core.CohortTransitionResult{}, err
		}
	}
	if err := recordM2ClaimAllocation(ctx, conn, command.MaterializationID, command.EntityID, eventID, sequence, claimShares); err != nil {
		return core.CohortTransitionResult{}, err
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO population_movements(movement_id, event_id, materialization_id, from_owner_kind, from_owner_id, to_owner_kind, to_owner_id, population_count, movement_kind, reason_code) VALUES (?, ?, ?, 'cohort', ?, 'entity', ?, ?, 'materialize', 'cohort_materialization')`, populationMovementID, eventID, command.MaterializationID, command.SourceCohortID, command.EntityID, command.PopulationCount); err != nil {
		return core.CohortTransitionResult{}, core.WrapError(core.CodeStorageFailure, "insert materialization population movement", err)
	}

	if command.AssetMinor+command.ReceivableMinor+command.LiabilityMinor > 0 {
		if _, err := conn.ExecContext(ctx, `INSERT INTO journal_entries(entry_id, event_id, status, purpose) VALUES (?, ?, 'draft', 'Cohort materialization allocation')`, entryID, eventID); err != nil {
			return core.CohortTransitionResult{}, core.WrapError(core.CodeStorageFailure, "insert materialization journal", err)
		}
		postings := []struct {
			name, accountID string
			amount          int64
		}{
			{"cohort_asset", cohort.assetAccount, -command.AssetMinor}, {"entity_asset", entityAssetAccount, command.AssetMinor},
			{"cohort_receivable", cohort.receivableAccount, -command.ReceivableMinor}, {"entity_receivable", entityReceivableAccount, command.ReceivableMinor},
			{"cohort_liability", cohort.liabilityAccount, command.LiabilityMinor}, {"entity_liability", entityLiabilityAccount, -command.LiabilityMinor},
		}
		for _, posting := range postings {
			if posting.amount == 0 {
				continue
			}
			if _, err := conn.ExecContext(ctx, `INSERT INTO postings(posting_id, entry_id, account_id, currency_id, amount_minor, memo) VALUES (?, ?, ?, ?, ?, 'Cohort materialization')`, "posting_"+command.CommandID+"_"+posting.name, entryID, posting.accountID, cohort.currencyID, posting.amount); err != nil {
				return core.CohortTransitionResult{}, core.WrapError(core.CodeStorageFailure, "insert materialization posting", err)
			}
		}
		if _, err := conn.ExecContext(ctx, `UPDATE journal_entries SET status = 'posted' WHERE entry_id = ? AND status = 'draft'`, entryID); err != nil {
			return core.CohortTransitionResult{}, core.WrapError(core.CodeStorageFailure, "post materialization journal", err)
		}
	}
	if command.InventoryMinor > 0 {
		if _, err := conn.ExecContext(ctx, `INSERT INTO stock_movements(movement_id, event_id, sku_id, from_location_id, to_location_id, quantity_minor, movement_kind, reason_code) VALUES (?, ?, ?, ?, ?, ?, 'transfer', 'cohort_materialization')`, stockMovementID, eventID, cohort.skuID, cohort.locationID, entityLocation, command.InventoryMinor); err != nil {
			return core.CohortTransitionResult{}, core.WrapError(core.CodeStorageFailure, "insert materialization stock movement", err)
		}
	}

	cohortStatus := "active"
	if newPopulation == 0 {
		cohortStatus = "depleted"
	}
	updates := []struct {
		name, query string
		args        []any
	}{
		{"Cohort population projection", `UPDATE cohorts SET population_count = ?, status = ?, projection_version = projection_version + 1, last_event_sequence = ? WHERE cohort_id = ? AND projection_version = ?`, []any{newPopulation, cohortStatus, sequence, command.SourceCohortID, cohort.version}},
		{"Cohort asset projection", `UPDATE account_balances SET balance_minor = ?, projection_version = projection_version + 1, last_event_sequence = ? WHERE account_id = ? AND projection_version = ?`, []any{newAsset, sequence, cohort.assetAccount, cohort.assetVersion}},
		{"Cohort receivable projection", `UPDATE account_balances SET balance_minor = ?, projection_version = projection_version + 1, last_event_sequence = ? WHERE account_id = ? AND projection_version = ?`, []any{newReceivable, sequence, cohort.receivableAccount, cohort.receivableVersion}},
		{"Cohort liability projection", `UPDATE account_balances SET balance_minor = ?, projection_version = projection_version + 1, last_event_sequence = ? WHERE account_id = ? AND projection_version = ?`, []any{newLiability, sequence, cohort.liabilityAccount, cohort.liabilityVersion}},
		{"Cohort inventory projection", `UPDATE inventory_balances SET quantity_minor = ?, projection_version = projection_version + 1, last_event_sequence = ? WHERE location_id = ? AND sku_id = ? AND projection_version = ?`, []any{newInventory, sequence, cohort.locationID, cohort.skuID, cohort.inventoryVersion}},
		{"materialization branch head", `UPDATE branches SET head_sequence = ? WHERE instance_id = ? AND branch_id = ? AND head_sequence = ?`, []any{sequence, command.InstanceID, command.BranchID, head}},
	}
	for _, update := range updates {
		result, err := conn.ExecContext(ctx, update.query, update.args...)
		if err != nil {
			return core.CohortTransitionResult{}, core.WrapError(core.CodeStorageFailure, "update "+update.name, err)
		}
		if rows, _ := result.RowsAffected(); rows != 1 {
			return core.CohortTransitionResult{}, core.NewError(core.CodeBranchConflict, update.name+" compare-and-swap failed")
		}
	}
	var recordOrder int64
	if err := conn.QueryRowContext(ctx, `SELECT COALESCE(MAX(record_order), 0) + 1 FROM audit_records WHERE instance_id = ? AND branch_id = ?`, command.InstanceID, command.BranchID).Scan(&recordOrder); err != nil {
		return core.CohortTransitionResult{}, core.WrapError(core.CodeStorageFailure, "allocate materialization audit order", err)
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO audit_records(record_id, instance_id, branch_id, record_order, record_type, authority, related_event_id, command_id, attempt_id, trace_id, world_time, recorded_at_utc, audience_scope, payload) VALUES (?, ?, ?, ?, 'intervention', 'audit', ?, ?, ?, ?, ?, ?, ?, ?)`, "audit_"+command.CommandID, command.InstanceID, command.BranchID, recordOrder, eventID, command.CommandID, attemptID, "trace_"+command.CommandID, command.WorldTime, nowText, string(scopeJSON), string(payloadJSON)); err != nil {
		return core.CohortTransitionResult{}, core.WrapError(core.CodeStorageFailure, "insert materialization audit", err)
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO outbox(outbox_id, event_id, topic, audience_scope, audience_scope_hash, payload) VALUES (?, ?, 'cohort.materialized', ?, ?, ?)`, outboxID, eventID, string(audienceJSON), audienceHash, string(payloadJSON)); err != nil {
		return core.CohortTransitionResult{}, core.WrapError(core.CodeStorageFailure, "insert materialization Outbox", err)
	}
	for _, transition := range []struct {
		name, query string
		args        []any
	}{
		{"materialization attempt", `UPDATE command_attempts SET status = 'committed', finished_at_utc = ? WHERE command_id = ? AND attempt_no = 1 AND status = 'ready'`, []any{nowText, command.CommandID}},
		{"materialization command", `UPDATE commands SET status = 'committed' WHERE command_id = ? AND status = 'pending'`, []any{command.CommandID}},
	} {
		result, err := conn.ExecContext(ctx, transition.query, transition.args...)
		if err != nil {
			return core.CohortTransitionResult{}, core.WrapError(core.CodeStorageFailure, "commit "+transition.name, err)
		}
		if rows, _ := result.RowsAffected(); rows != 1 {
			return core.CohortTransitionResult{}, core.NewError(core.CodeStorageFailure, transition.name+" transition affected an unexpected row count")
		}
	}
	return core.CohortTransitionResult{
		CommandID: command.CommandID, MaterializationID: command.MaterializationID, EntityID: command.EntityID,
		BatchID: batchID, EventID: eventID, FirstSequence: sequence, LastSequence: sequence,
		EventCount: 1, RequestHash: requestHash, BatchHash: batchHash,
	}, nil
}

func (s *Store) DematerializeCohort(ctx context.Context, command core.DematerializeCohortCommand) (core.CohortTransitionResult, error) {
	if err := command.Validate(); err != nil {
		return core.CohortTransitionResult{}, err
	}
	requestHash, err := core.DematerializeCohortRequestHash(command)
	if err != nil {
		return core.CohortTransitionResult{}, err
	}
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return core.CohortTransitionResult{}, core.WrapError(core.CodeStorageFailure, "begin Cohort dematerialization", err)
	}
	defer tx.Rollback(ctx)
	if result, found, err := lookupCohortCommandResult(ctx, tx.conn, command.InstanceID, command.BranchID, core.DematerializeCohortCommandType, command.IdempotencyKey, requestHash); err != nil {
		return core.CohortTransitionResult{}, err
	} else if found {
		return result, nil
	}

	var head int64
	if err := tx.conn.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id = ? AND branch_id = ?`, command.InstanceID, command.BranchID).Scan(&head); err != nil {
		return core.CohortTransitionResult{}, classifyMissing(err, "dematerialization branch")
	}
	if head != command.ExpectedHead {
		return core.CohortTransitionResult{}, core.NewError(core.CodeBranchConflict, fmt.Sprintf("expected head %d, current head %d", command.ExpectedHead, head))
	}
	if err := ensureCohortTransitionChronology(ctx, tx.conn, command.InstanceID, command.BranchID, command.WorldTime); err != nil {
		return core.CohortTransitionResult{}, err
	}
	sequence := head + 1
	var epochID string
	if err := tx.conn.QueryRowContext(ctx, `SELECT epoch_id FROM rule_epochs WHERE instance_id = ? AND branch_id = ? AND start_sequence <= ? AND (end_sequence IS NULL OR ? < end_sequence)`, command.InstanceID, command.BranchID, sequence, sequence).Scan(&epochID); err != nil {
		return core.CohortTransitionResult{}, classifyMissing(err, "dematerialization Rule Epoch")
	}

	type dematerializationProjection struct {
		sourceCohortID, entityID, algorithmVersion, lineageStatus, entityStatus string
		population, asset, inventory, receivable, liability                     int64
		cohortPopulation, cohortVersion                                         int64
		cohortAsset, cohortAssetVersion                                         int64
		cohortReceivable, cohortReceivableVersion                               int64
		cohortLiability, cohortLiabilityVersion                                 int64
		cohortInventory, cohortInventoryVersion                                 int64
		entityPopulation, entityVersion                                         int64
		entityAsset, entityAssetVersion                                         int64
		entityReceivable, entityReceivableVersion                               int64
		entityLiability, entityLiabilityVersion                                 int64
		entityInventory, entityInventoryVersion                                 int64
		cohortAssetID, cohortReceivableID, cohortLiabilityID, cohortLocationID  string
		entityAssetID, entityReceivableID, entityLiabilityID, entityLocationID  string
		currencyID, skuID                                                       string
	}
	var projection dematerializationProjection
	err = tx.conn.QueryRowContext(ctx, `
		SELECT m.source_cohort_id, m.entity_id, m.allocation_algorithm_version, m.status,
		       m.population_count, m.asset_minor, m.inventory_minor, m.receivable_minor, m.liability_minor,
		       c.population_count, c.projection_version,
		       c.asset_account_id, cab.balance_minor, cab.projection_version,
		       c.receivable_account_id, crb.balance_minor, crb.projection_version,
		       c.liability_account_id, clb.balance_minor, clb.projection_version,
		       c.inventory_location_id, cib.quantity_minor, cib.projection_version,
		       e.status, e.population_count, e.projection_version,
		       e.asset_account_id, eab.balance_minor, eab.projection_version,
		       e.receivable_account_id, erb.balance_minor, erb.projection_version,
		       e.liability_account_id, elb.balance_minor, elb.projection_version,
		       e.inventory_location_id, eib.quantity_minor, eib.projection_version,
		       c.currency_id, c.sku_id
		FROM cohort_materializations m
		JOIN cohorts c ON c.cohort_id = m.source_cohort_id
		JOIN materialized_entities e ON e.entity_id = m.entity_id
		JOIN account_balances cab ON cab.account_id = c.asset_account_id
		JOIN account_balances crb ON crb.account_id = c.receivable_account_id
		JOIN account_balances clb ON clb.account_id = c.liability_account_id
		JOIN inventory_balances cib ON cib.location_id = c.inventory_location_id AND cib.sku_id = c.sku_id
		JOIN account_balances eab ON eab.account_id = e.asset_account_id
		JOIN account_balances erb ON erb.account_id = e.receivable_account_id
		JOIN account_balances elb ON elb.account_id = e.liability_account_id
		JOIN inventory_balances eib ON eib.location_id = e.inventory_location_id AND eib.sku_id = e.sku_id
		WHERE m.materialization_id = ? AND c.instance_id = ? AND c.branch_id = ?`,
		command.MaterializationID, command.InstanceID, command.BranchID,
	).Scan(
		&projection.sourceCohortID, &projection.entityID, &projection.algorithmVersion, &projection.lineageStatus,
		&projection.population, &projection.asset, &projection.inventory, &projection.receivable, &projection.liability,
		&projection.cohortPopulation, &projection.cohortVersion,
		&projection.cohortAssetID, &projection.cohortAsset, &projection.cohortAssetVersion,
		&projection.cohortReceivableID, &projection.cohortReceivable, &projection.cohortReceivableVersion,
		&projection.cohortLiabilityID, &projection.cohortLiability, &projection.cohortLiabilityVersion,
		&projection.cohortLocationID, &projection.cohortInventory, &projection.cohortInventoryVersion,
		&projection.entityStatus, &projection.entityPopulation, &projection.entityVersion,
		&projection.entityAssetID, &projection.entityAsset, &projection.entityAssetVersion,
		&projection.entityReceivableID, &projection.entityReceivable, &projection.entityReceivableVersion,
		&projection.entityLiabilityID, &projection.entityLiability, &projection.entityLiabilityVersion,
		&projection.entityLocationID, &projection.entityInventory, &projection.entityInventoryVersion,
		&projection.currencyID, &projection.skuID,
	)
	if err != nil {
		return core.CohortTransitionResult{}, classifyMissing(err, "active materialization lineage")
	}
	if projection.lineageStatus != "active" || projection.entityStatus != "active" {
		return core.CohortTransitionResult{}, core.NewError(core.CodeMaterializationConflict, "materialization is not active")
	}
	var activeWageSplit int64
	if err := tx.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM m2_wage_participation_splits WHERE materialization_id = ?`, command.MaterializationID).Scan(&activeWageSplit); err != nil {
		return core.CohortTransitionResult{}, core.WrapError(core.CodeStorageFailure, "check returning wage participation", err)
	}
	returnedAsset, returnedReceivable := projection.asset, projection.receivable
	var wageReturns []m2WageClaimTransition
	if activeWageSplit > 0 {
		var earnedCash int64
		wageReturns, returnedReceivable, earnedCash, err = planM2WageClaimReturn(ctx, tx.conn, projection.entityID, projection.sourceCohortID)
		if err != nil {
			return core.CohortTransitionResult{}, err
		}
		var ok bool
		returnedAsset, ok = checkedAdd(projection.asset, earnedCash)
		if !ok {
			return core.CohortTransitionResult{}, core.NewError(core.CodeIntegerOverflow, "returning wage cash overflows entity asset")
		}
	}
	if projection.entityPopulation != projection.population || projection.entityAsset != returnedAsset ||
		projection.entityInventory != projection.inventory || projection.entityReceivable != returnedReceivable ||
		projection.entityLiability != -projection.liability {
		return core.CohortTransitionResult{}, core.NewError(core.CodeConservationFailed, "named entity no longer holds exactly the materialized allocation")
	}
	var grantID string
	if err := tx.conn.QueryRowContext(ctx, `
		SELECT g.grant_id FROM capability_grants g JOIN principals p ON p.principal_id = g.principal_id
		WHERE g.principal_id = ? AND p.status = 'active' AND g.capability_id = ?
		  AND g.instance_id = ? AND g.branch_id = ? AND g.subject_id IN (?, '*') AND g.status = 'active'
		ORDER BY CASE WHEN g.subject_id = ? THEN 0 ELSE 1 END, g.grant_id LIMIT 1`,
		command.PrincipalID, command.CapabilityID, command.InstanceID, command.BranchID,
		projection.sourceCohortID, projection.sourceCohortID,
	).Scan(&grantID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return core.CohortTransitionResult{}, core.NewError(core.CodeUnauthorized, "principal lacks Cohort dematerialization scope")
		}
		return core.CohortTransitionResult{}, core.WrapError(core.CodeStorageFailure, "read Cohort dematerialization grant", err)
	}

	newPopulation, populationOK := checkedAdd(projection.cohortPopulation, projection.population)
	newAsset, assetOK := checkedAdd(projection.cohortAsset, returnedAsset)
	newReceivable, receivableOK := checkedAdd(projection.cohortReceivable, returnedReceivable)
	newLiability, liabilityOK := checkedSubtract(projection.cohortLiability, projection.liability)
	newInventory, inventoryOK := checkedAdd(projection.cohortInventory, projection.inventory)
	if !populationOK || !assetOK || !receivableOK || !liabilityOK || !inventoryOK {
		return core.CohortTransitionResult{}, core.NewError(core.CodeIntegerOverflow, "dematerialization restoration overflows a conserved balance")
	}
	if err := ensureM2ContractsPermitPopulationTransition(ctx, tx.conn, projection.sourceCohortID, command.WorldTime, newPopulation, activeWageSplit > 0, command.MaterializationID); err != nil {
		return core.CohortTransitionResult{}, err
	}
	var returnedClaims []m2ClaimShare
	if activeWageSplit == 0 {
		returnedClaims, err = planM2ClaimReturn(ctx, tx.conn, command.MaterializationID, projection.entityID, returnedReceivable)
		if err != nil {
			return core.CohortTransitionResult{}, err
		}
	}
	returnHash := ""
	if len(returnedClaims) > 0 {
		returnHash, err = core.HashJSON(returnedClaims)
		if err != nil {
			return core.CohortTransitionResult{}, err
		}
	}
	payload := dematerializationEventPayload{MaterializationID: command.MaterializationID, SourceCohortID: projection.sourceCohortID, EntityID: projection.entityID,
		PopulationCount: projection.population, AssetMinor: returnedAsset, InventoryMinor: projection.inventory, ReceivableMinor: returnedReceivable,
		LiabilityMinor: projection.liability, ReasonCode: command.ReasonCode, ClaimReturnCount: len(returnedClaims), ClaimReturnHash: returnHash}
	if len(wageReturns) > 0 {
		payload.WageClaimReturnCount = len(wageReturns)
		payload.WageClaimReturnHash, err = core.HashJSON(wageReturns)
		if err != nil {
			return core.CohortTransitionResult{}, err
		}
	}
	payloadJSON, err := core.CanonicalJSON(payload)
	if err != nil {
		return core.CohortTransitionResult{}, err
	}
	batchHash, err := core.HashJSON(struct {
		CommandID, InstanceID, BranchID, EpochID string
		ExpectedHead, Sequence                   int64
		WorldTime, EventType                     string
		Payload                                  dematerializationEventPayload
	}{command.CommandID, command.InstanceID, command.BranchID, epochID, head, sequence, command.WorldTime, "CohortDematerialized", payload})
	if err != nil {
		return core.CohortTransitionResult{}, err
	}
	audience := struct {
		Kind       string `json:"kind"`
		InstanceID string `json:"instance_id"`
	}{"instance", command.InstanceID}
	audienceJSON, err := core.CanonicalJSON(audience)
	if err != nil {
		return core.CohortTransitionResult{}, err
	}
	audienceHash, err := core.HashJSON(audience)
	if err != nil {
		return core.CohortTransitionResult{}, err
	}
	scopeJSON, err := core.CanonicalJSON(struct {
		InstanceID string   `json:"instance_id"`
		BranchID   string   `json:"branch_id"`
		Subjects   []string `json:"subject_ids"`
	}{command.InstanceID, command.BranchID, []string{projection.sourceCohortID, projection.entityID}})
	if err != nil {
		return core.CohortTransitionResult{}, err
	}

	now := s.now().UTC()
	nowText := now.Format(time.RFC3339Nano)
	attemptID := "attempt_" + command.CommandID + "_1"
	batchID := "batch_" + command.CommandID
	eventID := "event_" + command.CommandID
	entryID := "journal_" + command.CommandID
	populationMovementID := "population_" + command.CommandID
	stockMovementID := "movement_" + command.CommandID
	outboxID := "outbox_" + command.CommandID
	policyDocument := fmt.Sprintf(`{"authorization":"scoped-capability","grant_id":%q}`, grantID)
	statements := []struct {
		name, query string
		args        []any
	}{
		{"dematerialization command", `INSERT INTO commands(command_id, instance_id, branch_id, command_type, idempotency_key, request_hash, expected_head, principal_id, command_policy, status, created_at_utc) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'pending', ?)`, []any{command.CommandID, command.InstanceID, command.BranchID, core.DematerializeCohortCommandType, command.IdempotencyKey, requestHash, head, command.PrincipalID, policyDocument, nowText}},
		{"dematerialization attempt", `INSERT INTO command_attempts(command_id, attempt_no, attempt_id, status, lease_owner, lease_until_utc, proposal_hash, created_at_utc) VALUES (?, 1, ?, 'ready', 'corerp-m2', ?, ?, ?)`, []any{command.CommandID, attemptID, now.Add(30 * time.Second).Format(time.RFC3339Nano), requestHash, nowText}},
		{"dematerialization batch", `INSERT INTO event_batches(batch_id, command_id, attempt_no, instance_id, branch_id, epoch_id, expected_head, first_sequence, last_sequence, event_count, world_time, batch_hash, committed_at_utc) VALUES (?, ?, 1, ?, ?, ?, ?, ?, ?, 1, ?, ?, ?)`, []any{batchID, command.CommandID, command.InstanceID, command.BranchID, epochID, head, sequence, sequence, command.WorldTime, batchHash, nowText}},
		{"dematerialization event", `INSERT INTO events(event_id, batch_id, instance_id, branch_id, event_sequence, batch_index, event_type, actor_id, world_time, payload) VALUES (?, ?, ?, ?, ?, 0, 'CohortDematerialized', ?, ?, ?)`, []any{eventID, batchID, command.InstanceID, command.BranchID, sequence, command.PrincipalID, command.WorldTime, string(payloadJSON)}},
	}
	for _, statement := range statements {
		if _, err := tx.conn.ExecContext(ctx, statement.query, statement.args...); err != nil {
			return core.CohortTransitionResult{}, core.WrapError(core.CodeStorageFailure, "insert "+statement.name, err)
		}
	}
	if returnedAsset+returnedReceivable+projection.liability > 0 {
		if _, err := tx.conn.ExecContext(ctx, `INSERT INTO journal_entries(entry_id, event_id, status, purpose) VALUES (?, ?, 'draft', 'Cohort dematerialization restoration')`, entryID, eventID); err != nil {
			return core.CohortTransitionResult{}, core.WrapError(core.CodeStorageFailure, "insert dematerialization journal", err)
		}
		postings := []struct {
			name, accountID string
			amount          int64
		}{
			{"cohort_asset", projection.cohortAssetID, returnedAsset}, {"entity_asset", projection.entityAssetID, -returnedAsset},
			{"cohort_receivable", projection.cohortReceivableID, returnedReceivable}, {"entity_receivable", projection.entityReceivableID, -returnedReceivable},
			{"cohort_liability", projection.cohortLiabilityID, -projection.liability}, {"entity_liability", projection.entityLiabilityID, projection.liability},
		}
		for _, posting := range postings {
			if posting.amount == 0 {
				continue
			}
			if _, err := tx.conn.ExecContext(ctx, `INSERT INTO postings(posting_id, entry_id, account_id, currency_id, amount_minor, memo) VALUES (?, ?, ?, ?, ?, 'Cohort dematerialization')`, "posting_"+command.CommandID+"_"+posting.name, entryID, posting.accountID, projection.currencyID, posting.amount); err != nil {
				return core.CohortTransitionResult{}, core.WrapError(core.CodeStorageFailure, "insert dematerialization posting", err)
			}
		}
		if _, err := tx.conn.ExecContext(ctx, `UPDATE journal_entries SET status = 'posted' WHERE entry_id = ? AND status = 'draft'`, entryID); err != nil {
			return core.CohortTransitionResult{}, core.WrapError(core.CodeStorageFailure, "post dematerialization journal", err)
		}
	}
	if projection.inventory > 0 {
		if _, err := tx.conn.ExecContext(ctx, `INSERT INTO stock_movements(movement_id, event_id, sku_id, from_location_id, to_location_id, quantity_minor, movement_kind, reason_code) VALUES (?, ?, ?, ?, ?, ?, 'transfer', 'cohort_dematerialization')`, stockMovementID, eventID, projection.skuID, projection.entityLocationID, projection.cohortLocationID, projection.inventory); err != nil {
			return core.CohortTransitionResult{}, core.WrapError(core.CodeStorageFailure, "insert dematerialization stock movement", err)
		}
	}
	if _, err := tx.conn.ExecContext(ctx, `INSERT INTO population_movements(movement_id, event_id, materialization_id, from_owner_kind, from_owner_id, to_owner_kind, to_owner_id, population_count, movement_kind, reason_code) VALUES (?, ?, ?, 'entity', ?, 'cohort', ?, ?, 'dematerialize', ?)`, populationMovementID, eventID, command.MaterializationID, projection.entityID, projection.sourceCohortID, projection.population, command.ReasonCode); err != nil {
		return core.CohortTransitionResult{}, core.WrapError(core.CodeStorageFailure, "insert dematerialization population movement", err)
	}
	if err := recordM2ClaimReturn(ctx, tx.conn, command.MaterializationID, eventID, sequence, returnedClaims); err != nil {
		return core.CohortTransitionResult{}, err
	}
	if activeWageSplit > 0 {
		if err := recordM2WageClaimTransitions(ctx, tx.conn, command.MaterializationID, eventID, sequence, "dematerialize", wageReturns); err != nil {
			return core.CohortTransitionResult{}, err
		}
		if err := execAgentOne(ctx, tx.conn, "record wage participation return", `INSERT INTO m2_wage_participation_returns(materialization_id, effective_from, return_event_id, return_event_sequence) VALUES (?, ?, ?, ?)`, command.MaterializationID, command.WorldTime, eventID, sequence); err != nil {
			return core.CohortTransitionResult{}, err
		}
	}

	updates := []struct {
		name, query string
		args        []any
	}{
		{"Cohort population restoration", `UPDATE cohorts SET population_count = ?, status = 'active', projection_version = projection_version + 1, last_event_sequence = ? WHERE cohort_id = ? AND projection_version = ?`, []any{newPopulation, sequence, projection.sourceCohortID, projection.cohortVersion}},
		{"Cohort asset restoration", `UPDATE account_balances SET balance_minor = ?, projection_version = projection_version + 1, last_event_sequence = ? WHERE account_id = ? AND projection_version = ?`, []any{newAsset, sequence, projection.cohortAssetID, projection.cohortAssetVersion}},
		{"Cohort receivable restoration", `UPDATE account_balances SET balance_minor = ?, projection_version = projection_version + 1, last_event_sequence = ? WHERE account_id = ? AND projection_version = ?`, []any{newReceivable, sequence, projection.cohortReceivableID, projection.cohortReceivableVersion}},
		{"Cohort liability restoration", `UPDATE account_balances SET balance_minor = ?, projection_version = projection_version + 1, last_event_sequence = ? WHERE account_id = ? AND projection_version = ?`, []any{newLiability, sequence, projection.cohortLiabilityID, projection.cohortLiabilityVersion}},
		{"Cohort inventory restoration", `UPDATE inventory_balances SET quantity_minor = ?, projection_version = projection_version + 1, last_event_sequence = ? WHERE location_id = ? AND sku_id = ? AND projection_version = ?`, []any{newInventory, sequence, projection.cohortLocationID, projection.skuID, projection.cohortInventoryVersion}},
		{"entity population close", `UPDATE materialized_entities SET population_count = 0, status = 'dematerialized', projection_version = projection_version + 1, last_event_sequence = ? WHERE entity_id = ? AND projection_version = ? AND status = 'active'`, []any{sequence, projection.entityID, projection.entityVersion}},
		{"entity asset close", `UPDATE account_balances SET balance_minor = 0, projection_version = projection_version + 1, last_event_sequence = ? WHERE account_id = ? AND projection_version = ?`, []any{sequence, projection.entityAssetID, projection.entityAssetVersion}},
		{"entity receivable close", `UPDATE account_balances SET balance_minor = 0, projection_version = projection_version + 1, last_event_sequence = ? WHERE account_id = ? AND projection_version = ?`, []any{sequence, projection.entityReceivableID, projection.entityReceivableVersion}},
		{"entity liability close", `UPDATE account_balances SET balance_minor = 0, projection_version = projection_version + 1, last_event_sequence = ? WHERE account_id = ? AND projection_version = ?`, []any{sequence, projection.entityLiabilityID, projection.entityLiabilityVersion}},
		{"entity inventory close", `UPDATE inventory_balances SET quantity_minor = 0, projection_version = projection_version + 1, last_event_sequence = ? WHERE location_id = ? AND sku_id = ? AND projection_version = ?`, []any{sequence, projection.entityLocationID, projection.skuID, projection.entityInventoryVersion}},
		{"close materialization lineage", `UPDATE cohort_materializations SET dematerialize_command_id = ?, dematerialize_event_id = ?, dematerialize_sequence = ?, status = 'dematerialized' WHERE materialization_id = ? AND status = 'active'`, []any{command.CommandID, eventID, sequence, command.MaterializationID}},
		{"dematerialization branch head", `UPDATE branches SET head_sequence = ? WHERE instance_id = ? AND branch_id = ? AND head_sequence = ?`, []any{sequence, command.InstanceID, command.BranchID, head}},
	}
	for _, update := range updates {
		result, err := tx.conn.ExecContext(ctx, update.query, update.args...)
		if err != nil {
			return core.CohortTransitionResult{}, core.WrapError(core.CodeStorageFailure, "update "+update.name, err)
		}
		if rows, _ := result.RowsAffected(); rows != 1 {
			return core.CohortTransitionResult{}, core.NewError(core.CodeBranchConflict, update.name+" compare-and-swap failed")
		}
	}
	var recordOrder int64
	if err := tx.conn.QueryRowContext(ctx, `SELECT COALESCE(MAX(record_order), 0) + 1 FROM audit_records WHERE instance_id = ? AND branch_id = ?`, command.InstanceID, command.BranchID).Scan(&recordOrder); err != nil {
		return core.CohortTransitionResult{}, core.WrapError(core.CodeStorageFailure, "allocate dematerialization audit order", err)
	}
	if _, err := tx.conn.ExecContext(ctx, `INSERT INTO audit_records(record_id, instance_id, branch_id, record_order, record_type, authority, related_event_id, command_id, attempt_id, trace_id, world_time, recorded_at_utc, audience_scope, payload) VALUES (?, ?, ?, ?, 'intervention', 'audit', ?, ?, ?, ?, ?, ?, ?, ?)`, "audit_"+command.CommandID, command.InstanceID, command.BranchID, recordOrder, eventID, command.CommandID, attemptID, "trace_"+command.CommandID, command.WorldTime, nowText, string(scopeJSON), string(payloadJSON)); err != nil {
		return core.CohortTransitionResult{}, core.WrapError(core.CodeStorageFailure, "insert dematerialization audit", err)
	}
	if _, err := tx.conn.ExecContext(ctx, `INSERT INTO outbox(outbox_id, event_id, topic, audience_scope, audience_scope_hash, payload) VALUES (?, ?, 'cohort.dematerialized', ?, ?, ?)`, outboxID, eventID, string(audienceJSON), audienceHash, string(payloadJSON)); err != nil {
		return core.CohortTransitionResult{}, core.WrapError(core.CodeStorageFailure, "insert dematerialization Outbox", err)
	}
	for _, transition := range []struct {
		name, query string
		args        []any
	}{
		{"dematerialization attempt", `UPDATE command_attempts SET status = 'committed', finished_at_utc = ? WHERE command_id = ? AND attempt_no = 1 AND status = 'ready'`, []any{nowText, command.CommandID}},
		{"dematerialization command", `UPDATE commands SET status = 'committed' WHERE command_id = ? AND status = 'pending'`, []any{command.CommandID}},
	} {
		result, err := tx.conn.ExecContext(ctx, transition.query, transition.args...)
		if err != nil {
			return core.CohortTransitionResult{}, core.WrapError(core.CodeStorageFailure, "commit "+transition.name, err)
		}
		if rows, _ := result.RowsAffected(); rows != 1 {
			return core.CohortTransitionResult{}, core.NewError(core.CodeStorageFailure, transition.name+" transition affected an unexpected row count")
		}
	}
	if s.beforeCommit != nil {
		if err := s.beforeCommit(); err != nil {
			return core.CohortTransitionResult{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return core.CohortTransitionResult{}, core.WrapError(core.CodeStorageFailure, "commit Cohort dematerialization", err)
	}
	return core.CohortTransitionResult{
		CommandID: command.CommandID, MaterializationID: command.MaterializationID, EntityID: projection.entityID,
		BatchID: batchID, EventID: eventID, FirstSequence: sequence, LastSequence: sequence,
		EventCount: 1, RequestHash: requestHash, BatchHash: batchHash,
	}, nil
}

func lookupCohortCommandResult(ctx context.Context, conn *sql.Conn, instanceID, branchID, commandType, idempotencyKey, requestHash string) (core.CohortTransitionResult, bool, error) {
	var commandID, existingHash, status string
	err := conn.QueryRowContext(ctx, `SELECT command_id, request_hash, status FROM commands WHERE instance_id = ? AND branch_id = ? AND command_type = ? AND idempotency_key = ?`, instanceID, branchID, commandType, idempotencyKey).Scan(&commandID, &existingHash, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return core.CohortTransitionResult{}, false, nil
	}
	if err != nil {
		return core.CohortTransitionResult{}, false, core.WrapError(core.CodeStorageFailure, "look up Cohort command idempotency key", err)
	}
	if existingHash != requestHash {
		return core.CohortTransitionResult{}, false, core.NewError(core.CodeIdempotencyMismatch, "idempotency key was already used with a different Cohort request")
	}
	if status != "committed" {
		return core.CohortTransitionResult{}, false, core.NewError(core.CodeCommandInProgress, "matching Cohort command is not committed")
	}
	var materializationID string
	if err := conn.QueryRowContext(ctx, `SELECT materialization_id FROM cohort_materializations WHERE materialize_command_id = ? OR dematerialize_command_id = ?`, commandID, commandID).Scan(&materializationID); err != nil {
		return core.CohortTransitionResult{}, false, core.WrapError(core.CodeStorageFailure, "load Cohort command lineage", err)
	}
	result, err := loadCohortTransitionResult(ctx, conn, commandID, materializationID)
	if err != nil {
		return core.CohortTransitionResult{}, false, err
	}
	result.Replayed = true
	return result, true, nil
}

func loadCohortTransitionResult(ctx context.Context, conn *sql.Conn, commandID, materializationID string) (core.CohortTransitionResult, error) {
	var result core.CohortTransitionResult
	err := conn.QueryRowContext(ctx, `
		SELECT c.command_id, m.materialization_id, m.entity_id, b.batch_id, e.event_id,
		       b.first_sequence, b.last_sequence, b.event_count, c.request_hash, b.batch_hash
		FROM commands c JOIN event_batches b ON b.command_id = c.command_id
		JOIN events e ON e.batch_id = b.batch_id AND e.batch_index = 0
		JOIN cohort_materializations m ON m.materialization_id = ?
		WHERE c.command_id = ? AND c.status = 'committed'`, materializationID, commandID,
	).Scan(&result.CommandID, &result.MaterializationID, &result.EntityID, &result.BatchID, &result.EventID,
		&result.FirstSequence, &result.LastSequence, &result.EventCount, &result.RequestHash, &result.BatchHash)
	if err != nil {
		return core.CohortTransitionResult{}, core.WrapError(core.CodeStorageFailure, "load committed Cohort transition", err)
	}
	return result, nil
}
