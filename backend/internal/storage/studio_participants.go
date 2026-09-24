package storage

import (
	"context"

	"corerp.local/backend/internal/core"
)

// PrepareStudioParticipants consumes the exact saved genesis declaration. It
// creates conserved economic entities, not yet spatial actors or a Play binding.
// The complete participant set commits atomically, independently of genesis.
func (s *Store) PrepareStudioParticipants(ctx context.Context, r StudioGenesisRequest) ([]core.CohortTransitionResult, error) {
	if err := validateStudioGenesisRequest(r); err != nil {
		return nil, err
	}
	id := func(kind, key string) string { v, _ := core.StudioWorldObjectID(r.InstanceID, kind, key); return v }
	event, cohort := id("event", "genesis"), id("cohort", "population")
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if err := authorizeSavedStudioGenesis(ctx, tx.conn, r); err != nil {
		return nil, err
	}
	commands := make([]core.MaterializeCohortCommand, 0, len(r.Spec.People))
	money, stock, population := r.Spec.OpeningMoneyMinor, r.Spec.OpeningStockMinor, r.Spec.Population
	for i, person := range r.Spec.People {
		assetShare, stockShare := money/population, stock/population
		commands = append(commands, core.MaterializeCohortCommand{
			CommandID: id("materialize_command", person.Key), MaterializationID: id("materialization", person.Key),
			InstanceID: r.InstanceID, BranchID: "br_main", PrincipalID: r.PrincipalID,
			CapabilityID: "world.create", IdempotencyKey: id("materialize_key", person.Key),
			ExpectedHead: int64(i + 1), WorldTime: r.Spec.StartWorldTime, SourceCohortID: cohort,
			EntityID: id("entity", person.Key), DisplayName: person.Name, PopulationCount: 1,
			AssetMinor: assetShare, InventoryMinor: stockShare, AllocationAlgorithmVersion: "equal-share-v1",
		})
		money -= assetShare
		stock -= stockShare
		population--
	}
	results := make([]core.CohortTransitionResult, 0, len(commands))
	for _, command := range commands {
		hash, err := core.MaterializeCohortRequestHash(command)
		if err != nil {
			return nil, err
		}
		result, found, err := lookupCohortCommandResult(ctx, tx.conn, r.InstanceID, "br_main", core.MaterializeCohortCommandType, command.IdempotencyKey, hash)
		if err != nil {
			return nil, err
		}
		if found {
			results = append(results, result)
		}
	}
	if len(results) == len(commands) {
		return results, nil
	}
	if len(results) != 0 {
		return nil, core.NewError(core.CodeProjectionDiverged, "partial atomic Studio participant preparation")
	}
	var pristine int
	if err := tx.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM world_instances w JOIN branches b ON b.instance_id=w.instance_id JOIN world_clocks c ON c.instance_id=b.instance_id AND c.branch_id=b.branch_id WHERE w.instance_id=? AND b.branch_id='br_main' AND w.lifecycle_state='paused' AND b.head_sequence=1 AND c.current_world_time=? AND c.status='paused'`,
		r.InstanceID, r.Spec.StartWorldTime).Scan(&pristine); err != nil {
		return nil, err
	}
	if pristine != 1 {
		return nil, core.NewError(core.CodeBranchConflict, "participant preparation requires untouched paused genesis")
	}
	if err := tx.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM cohorts c JOIN account_balances a ON a.account_id=c.asset_account_id JOIN account_balances r ON r.account_id=c.receivable_account_id JOIN account_balances l ON l.account_id=c.liability_account_id JOIN inventory_balances i ON i.location_id=c.inventory_location_id AND i.sku_id=c.sku_id WHERE c.cohort_id=? AND c.instance_id=? AND c.branch_id='br_main' AND c.definition_event_id=? AND c.population_count=? AND c.status='active' AND a.balance_minor=? AND r.balance_minor=0 AND l.balance_minor=0 AND i.quantity_minor=?`,
		cohort, r.InstanceID, event, r.Spec.Population, r.Spec.OpeningMoneyMinor, r.Spec.OpeningStockMinor).Scan(&pristine); err != nil {
		return nil, err
	}
	if pristine != 1 {
		return nil, core.NewError(core.CodeProjectionDiverged, "initial Cohort differs from saved genesis resources")
	}
	policy, err := core.CanonicalJSON(struct {
		Authorization       string `json:"authorization"`
		GenesisEventID      string `json:"genesis_event_id"`
		AuthorityInstanceID string `json:"authority_instance_id"`
		AuthorityBranchID   string `json:"authority_branch_id"`
	}{"sourced-world-create", event, r.AuthorityInstanceID, r.AuthorityBranchID})
	if err != nil {
		return nil, err
	}
	for _, command := range commands {
		result, err := s.materializeCohortInTransaction(ctx, tx.conn, command, func() (string, error) { return string(policy), nil })
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	if s.beforeCommit != nil {
		if err := s.beforeCommit(); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return results, nil
}
