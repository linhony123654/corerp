package storage

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPFinalCohortsFiniteOpeningRollbackAndRecovery(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "final-cohorts.db")
	s := openCareerTestWorld(t, path)
	defer func() { s.Close() }()
	initial := careerTestBinding(t, s, "principal_creator", "probe").ExpectedHead
	s.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "opening rollback") }
	if _, err := s.PrepareRPFinalCohorts(ctx); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatalf("rollback: %v", err)
	}
	s.beforeCommit = nil
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='RPFinalCohortInitialized'`, nil, 0)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM cohorts`, nil, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM accounts WHERE owner_id IN (?,?)`, []any{RPFinalBlockB, RPFinalBlockC}, 0)
	assertM2Value(t, ctx, s, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, []any{M2DemoInstanceID, M2DemoBranchID}, initial)
	commits := 0
	s.beforeCommit = func() error {
		commits++
		if commits == 2 {
			return core.NewError(core.CodeInjectedFailure, "second cohort interrupted")
		}
		return nil
	}
	partial, err := s.PrepareRPFinalCohorts(ctx)
	if !core.HasCode(err, core.CodeInjectedFailure) || len(partial) != 1 {
		t.Fatalf("partial setup: %+v %v", partial, err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM cohorts`, nil, 2)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	completed, err := s.PrepareRPFinalCohorts(ctx)
	if err != nil || len(completed) != 2 || !completed[0].Replayed || completed[1].Replayed || completed[0].EventID != partial[0].EventID {
		t.Fatalf("resume: %+v %v", completed, err)
	}
	for _, r := range completed {
		if r.Fact.FundingSourceEventID == "" || r.Fact.Population != 4 {
			t.Fatal("opening source missing")
		}
		assertM2Value(t, ctx, s, `SELECT SUM(p.amount_minor) FROM postings p JOIN journal_entries j ON j.entry_id=p.entry_id WHERE j.event_id=? AND j.status='posted'`, []any{r.EventID}, 0)
	}
	assertM2Value(t, ctx, s, `SELECT SUM(population_count) FROM cohorts`, nil, 24) // plus four already materialized people = 28
	assertM2Value(t, ctx, s, `SELECT SUM(b.balance_minor) FROM account_balances b JOIN accounts a ON a.account_id=b.account_id WHERE a.account_type='asset' AND a.currency_id=?`, []any{M2DemoCurrencyID}, 10000)
	assertM2Value(t, ctx, s, `SELECT SUM(quantity_minor) FROM inventory_balances WHERE sku_id=?`, []any{M2DemoSKUID}, 100)
	assertM2Value(t, ctx, s, `SELECT SUM(population_count) FROM population_movements WHERE movement_kind='create'`, nil, 28)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM capability_grants WHERE subject_id IN (?,?) AND definition_event_id IN (?,?)`, []any{RPFinalBlockB, RPFinalBlockC, completed[0].EventID, completed[1].EventID}, 4)
	// Exercise the second cohort's actual normal materialization owner, not just
	// its row count; then use the ordinary inverse before testing reconstruction.
	command := m2AgentMaterialization("final-roundtrip", "entity_final_roundtrip", "Visitor", 1, 90, 1, 0, 0, completed[1].EventSequence, rpLifeSetupTime)
	command.SourceCohortID = RPFinalBlockB
	materialized, err := s.MaterializeCohort(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.DematerializeCohort(ctx, core.DematerializeCohortCommand{CommandID: "cmd_final_return", MaterializationID: command.MaterializationID, InstanceID: command.InstanceID, BranchID: command.BranchID, PrincipalID: command.PrincipalID, CapabilityID: "world.cohort.dematerialize", IdempotencyKey: "final-return", ExpectedHead: materialized.LastSequence, WorldTime: rpLifeSetupTime, ReasonCode: "roundtrip"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE cohorts SET population_count=999 WHERE cohort_id=?`, RPFinalBlockB); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE account_balances SET balance_minor=999 WHERE account_id=?`, "account_"+RPFinalBlockB+"_asset"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE inventory_balances SET quantity_minor=999 WHERE location_id=?`, "location_"+RPFinalBlockB); err != nil {
		t.Fatal(err)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) < 3 {
		t.Fatalf("corruption detection %+v %v", diff, err)
	}
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatalf("rebuild %+v %v", diff, err)
	}
	assertM2Value(t, ctx, s, `SELECT population_count FROM cohorts WHERE cohort_id=?`, []any{RPFinalBlockB}, 4)
	assertM2Value(t, ctx, s, `SELECT balance_minor FROM account_balances WHERE account_id=?`, []any{"account_" + RPFinalBlockB + "_asset"}, 600)
	assertM2Value(t, ctx, s, `SELECT quantity_minor FROM inventory_balances WHERE location_id=?`, []any{"location_" + RPFinalBlockB}, 5)
	if _, err := s.RunAgentLife(ctx, M2AgentNoonTime, 100); err != nil {
		t.Fatal(err)
	}
	beforeRetry := careerTestBinding(t, s, "principal_creator", "probe").ExpectedHead
	retry, err := s.PrepareRPFinalCohorts(ctx)
	if err != nil || len(retry) != 2 {
		t.Fatalf("late retry: %+v %v", retry, err)
	}
	for i := range retry {
		if !retry[i].Replayed || retry[i].EventID != completed[i].EventID || !reflect.DeepEqual(retry[i].Fact, completed[i].Fact) {
			t.Fatal("retry rewrote opening")
		}
	}
	assertM2Value(t, ctx, s, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, []any{M2DemoInstanceID, M2DemoBranchID}, beforeRetry)
}

func TestRPFinalCohortsRejectLateRevokedAndCorruptSetup(t *testing.T) {
	for _, scenario := range []string{"session", "clock", "revoked", "cash", "stock"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			s := openCareerTestWorld(t, filepath.Join(t.TempDir(), "denied.db"))
			defer s.Close()
			code := core.CodeBranchConflict
			switch scenario {
			case "session":
				if _, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: M2RPPlayerPrincipal, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2RPPlayerID, POV: "second_person", IdempotencyKey: "started"}); err != nil {
					t.Fatal(err)
				}
			case "clock":
				if _, err := s.RunAgentLife(ctx, M2AgentNoonTime, 100); err != nil {
					t.Fatal(err)
				}
			case "revoked":
				if _, err := s.db.ExecContext(ctx, `UPDATE capability_grants SET status='revoked' WHERE grant_id='grant_m2_creator_materialize'`); err != nil {
					t.Fatal(err)
				}
				code = core.CodeUnauthorized
			case "cash":
				if _, err := s.db.ExecContext(ctx, `UPDATE account_balances SET balance_minor=balance_minor+1 WHERE account_id=?`, M2DemoCohortAssetAccountID); err != nil {
					t.Fatal(err)
				}
				code = core.CodeProjectionDiverged
			case "stock":
				if _, err := s.db.ExecContext(ctx, `UPDATE inventory_balances SET quantity_minor=quantity_minor+1 WHERE location_id=?`, M2DemoCohortLocationID); err != nil {
					t.Fatal(err)
				}
				code = core.CodeProjectionDiverged
			}
			head := careerTestBinding(t, s, "principal_creator", "probe").ExpectedHead
			if _, err := s.PrepareRPFinalCohorts(ctx); !core.HasCode(err, code) {
				t.Fatalf("%s: want %s got %v", scenario, code, err)
			}
			assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM cohorts`, nil, 1)
			assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='RPFinalCohortInitialized'`, nil, 0)
			assertM2Value(t, ctx, s, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, []any{M2DemoInstanceID, M2DemoBranchID}, head)
		})
	}
}
