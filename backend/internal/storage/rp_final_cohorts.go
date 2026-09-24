package storage

import (
	"context"
	"database/sql"
	"errors"

	"corerp.local/backend/internal/core"
)

// Explicit local acceptance-world declarations, not a runtime immigration or
// cohort-split API. Population is created by sourced opening facts; all money
// and goods come from the existing finite Block A balances.
const RPFinalBlockB = "cohort_final_block_b"
const RPFinalBlockC = "cohort_final_block_c"

type RPFinalCohortDefinition struct {
	Version              string `json:"version"`
	CohortID             string `json:"cohort_id"`
	DisplayName          string `json:"display_name"`
	Population           int64  `json:"population_count"`
	FundingCohortID      string `json:"funding_cohort_id"`
	FundingSourceEventID string `json:"funding_source_event_id"`
	CashMinor            int64  `json:"cash_minor"`
	StockMinor           int64  `json:"stock_minor"`
}

type RPFinalCohortResult = privateFactRecord[RPFinalCohortDefinition]

// PrepareRPFinalCohorts is opt-in local demo setup, like PrepareRPLifeDemo. It
// intentionally has no HTTP route. Two independently atomic receipts make an
// interrupted setup resumable; callers must finish setup before opening Play.
func (s *Store) PrepareRPFinalCohorts(ctx context.Context) ([]RPFinalCohortResult, error) {
	result := make([]RPFinalCohortResult, 0, 2)
	for _, cohort := range []struct{ id, name string }{{RPFinalBlockB, "Block B households"}, {RPFinalBlockC, "Block C households"}} {
		r, err := s.prepareRPFinalCohort(ctx, cohort.id, cohort.name)
		if err != nil {
			return result, err
		}
		result = append(result, r)
	}
	return result, nil
}

func (s *Store) prepareRPFinalCohort(ctx context.Context, cohortID, name string) (RPFinalCohortResult, error) {
	var empty RPFinalCohortResult
	const commandType = "PrepareRPFinalCohortLocal"
	b := core.CareerBinding{PrincipalID: "principal_creator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, IdempotencyKey: "final-opening-v1:" + cohortID}
	key, err := core.HashJSON([]string{b.PrincipalID, b.IdempotencyKey})
	if err != nil {
		return empty, err
	}
	// Keep the original frozen request on retry, even after the simulation moves.
	err = s.db.QueryRowContext(ctx, `SELECT expected_head FROM commands WHERE instance_id=? AND branch_id=? AND command_type=? AND idempotency_key=? AND status='committed'`, b.InstanceID, b.BranchID, commandType, key).Scan(&b.ExpectedHead)
	if errors.Is(err, sql.ErrNoRows) {
		err = s.db.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, b.InstanceID, b.BranchID).Scan(&b.ExpectedHead)
	}
	if err != nil {
		return empty, err
	}
	request := struct {
		Binding     core.CareerBinding `json:"binding"`
		CohortID    string             `json:"cohort_id"`
		DisplayName string             `json:"display_name"`
		Version     string             `json:"version"`
	}{b, cohortID, name, "corerp.final-opening.v1"}
	return executePrivateFactCommand(s, ctx, b, commandType, request, privateFactDomain{"final_cohort", "RPFinalCohortInitialized", `{"authorization":"final-local-opening-v1"}`}, func(conn *sql.Conn) error {
		var allowed int
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM principals p JOIN capability_grants g ON g.principal_id=p.principal_id WHERE p.principal_id=? AND p.principal_type='creator' AND p.status='active' AND g.instance_id=? AND g.branch_id=? AND g.capability_id='world.cohort.materialize' AND g.subject_id=? AND g.status='active'`, b.PrincipalID, b.InstanceID, b.BranchID, M2DemoCohortID).Scan(&allowed); err != nil {
			return err
		}
		if allowed == 0 {
			return core.NewError(core.CodeUnauthorized, "Final local opening requires active demo creator authority")
		}
		return nil
	}, func(conn *sql.Conn, c privateFactContext) (RPFinalCohortDefinition, func() error, error) {
		var fact RPFinalCohortDefinition
		if c.WorldTime != rpLifeSetupTime {
			return fact, nil, core.NewError(core.CodeBranchConflict, "Final cohorts must be declared at the life setup clock")
		}
		var sessions int
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_sessions WHERE instance_id=? AND branch_id=?`, b.InstanceID, b.BranchID).Scan(&sessions); err != nil {
			return fact, nil, err
		}
		if sessions != 0 {
			return fact, nil, core.NewError(core.CodeBranchConflict, "Final cohorts must be declared before opening RP")
		}
		var source string
		if err := conn.QueryRowContext(ctx, `SELECT c.definition_event_id FROM cohorts c JOIN events e ON e.event_id=c.definition_event_id WHERE c.cohort_id=? AND c.instance_id=? AND c.branch_id=? AND e.instance_id=c.instance_id AND e.branch_id=c.branch_id`, M2DemoCohortID, b.InstanceID, b.BranchID).Scan(&source); err != nil {
			return fact, nil, err
		}
		if err := verifyM2AccountProjection(ctx, conn, M2DemoCohortAssetAccountID, M2DemoCurrencyID); err != nil {
			return fact, nil, err
		}
		if err := verifyM2InventoryProjection(ctx, conn, M2DemoCohortLocationID, M2DemoSKUID); err != nil {
			return fact, nil, err
		}
		cash, cashVersion, err := readScheduledBalance(ctx, conn, M2DemoCohortAssetAccountID)
		if err != nil {
			return fact, nil, err
		}
		stock, stockVersion, err := readScheduledInventory(ctx, conn, M2DemoCohortLocationID, M2DemoSKUID)
		if err != nil {
			return fact, nil, err
		}
		if cash < 600 || stock < 5 {
			return fact, nil, core.NewError(core.CodeConservationFailed, "Final opening exceeds finite funding resources")
		}
		fact = RPFinalCohortDefinition{request.Version, cohortID, name, 4, M2DemoCohortID, source, 600, 5}
		return fact, func() error {
			exec := func(query string, args ...any) error {
				return execAgentOne(ctx, conn, "Final sourced cohort opening", query, args...)
			}
			account := func(kind string) string { return "account_" + cohortID + "_" + kind }
			location, entry := "location_"+cohortID, "journal_"+c.EventID
			for _, kind := range []string{"asset", "receivable", "liability"} {
				if err := exec(`INSERT INTO accounts(account_id,owner_id,currency_id,account_type,opened_by_event_id) VALUES (?,?,?,?,?)`, account(kind), cohortID, M2DemoCurrencyID, kind, c.EventID); err != nil {
					return err
				}
				balance := int64(0)
				if kind == "asset" {
					balance = fact.CashMinor
				}
				if err := exec(`INSERT INTO account_balances(account_id,balance_minor,projection_version,last_event_sequence) VALUES (?,?,0,?)`, account(kind), balance, c.Sequence); err != nil {
					return err
				}
			}
			if err := exec(`INSERT INTO journal_entries(entry_id,event_id,status,purpose) VALUES (?,?,'draft','finite Final opening allocation')`, entry, c.EventID); err != nil {
				return err
			}
			for _, p := range []struct {
				id     string
				amount int64
			}{{M2DemoCohortAssetAccountID, -fact.CashMinor}, {account("asset"), fact.CashMinor}} {
				if err := exec(`INSERT INTO postings(posting_id,entry_id,account_id,currency_id,amount_minor) VALUES (?,?,?,?,?)`, "posting_"+c.EventID+"_"+p.id, entry, p.id, M2DemoCurrencyID, p.amount); err != nil {
					return err
				}
			}
			if err := exec(`UPDATE journal_entries SET status='posted' WHERE entry_id=? AND status='draft'`, entry); err != nil {
				return err
			}
			if err := exec(`UPDATE account_balances SET balance_minor=?,projection_version=projection_version+1,last_event_sequence=? WHERE account_id=? AND projection_version=?`, cash-fact.CashMinor, c.Sequence, M2DemoCohortAssetAccountID, cashVersion); err != nil {
				return err
			}
			if err := exec(`INSERT INTO stock_locations(location_id,owner_id,location_kind) VALUES (?,?,'holder')`, location, cohortID); err != nil {
				return err
			}
			if err := exec(`INSERT INTO inventory_balances(location_id,sku_id,quantity_minor,projection_version,last_event_sequence) VALUES (?,?,?,0,?)`, location, M2DemoSKUID, fact.StockMinor, c.Sequence); err != nil {
				return err
			}
			if err := exec(`INSERT INTO stock_movements(movement_id,event_id,sku_id,from_location_id,to_location_id,quantity_minor,movement_kind,reason_code) VALUES (?,?,?,?,?,?,'transfer','final_opening_allocation')`, "stock_"+c.EventID, c.EventID, M2DemoSKUID, M2DemoCohortLocationID, location, fact.StockMinor); err != nil {
				return err
			}
			if err := exec(`UPDATE inventory_balances SET quantity_minor=?,projection_version=projection_version+1,last_event_sequence=? WHERE location_id=? AND sku_id=? AND projection_version=?`, stock-fact.StockMinor, c.Sequence, M2DemoCohortLocationID, M2DemoSKUID, stockVersion); err != nil {
				return err
			}
			if err := exec(`INSERT INTO cohorts(cohort_id,instance_id,branch_id,display_name,population_count,asset_account_id,receivable_account_id,liability_account_id,inventory_location_id,currency_id,sku_id,allocation_algorithm_version,status,projection_version,last_event_sequence,definition_event_id) VALUES (?,?,?,?,?,?,?,?,?,?,?,'equal-share-v1','active',0,?,?)`, cohortID, b.InstanceID, b.BranchID, name, fact.Population, account("asset"), account("receivable"), account("liability"), location, M2DemoCurrencyID, M2DemoSKUID, c.Sequence, c.EventID); err != nil {
				return err
			}
			if err := exec(`INSERT INTO population_movements(movement_id,event_id,to_owner_kind,to_owner_id,population_count,movement_kind,reason_code) VALUES (?,?,'cohort',?,?,'create','final_initial_population')`, "population_"+c.EventID, c.EventID, cohortID, fact.Population); err != nil {
				return err
			}
			for _, capability := range []string{"world.cohort.materialize", "world.cohort.dematerialize"} {
				if err := exec(`INSERT INTO capability_grants(grant_id,principal_id,capability_id,instance_id,branch_id,subject_id,field_scope,status,definition_event_id) VALUES (?,?,?,?,?,?,'[]','active',?)`, "grant_"+cohortID+"_"+capability, b.PrincipalID, capability, b.InstanceID, b.BranchID, cohortID, c.EventID); err != nil {
					return err
				}
			}
			return nil
		}, nil
	})
}
