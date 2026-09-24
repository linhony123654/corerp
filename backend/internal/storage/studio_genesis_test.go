package storage

import (
	"context"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestStudioGenesisAtomicConservedIsolatedAndRecoverable(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "genesis.db")
	s := openBootstrappedStore(t, ctx, path)
	defer func() { s.Close() }()
	setup, err := s.PrepareRPTravel(ctx)
	if err != nil {
		t.Fatal(err)
	}
	spec := core.StudioWorldSpec{Version: core.StudioWorldSpecVersion, Name: "新世界", StartWorldTime: core.StudioWorldStart, Population: 20, OpeningMoneyMinor: 10000, OpeningStockMinor: 100, Places: []core.StudioWorldPlace{{Key: "home", Name: "家", Kind: "home"}, {Key: "square", Name: "广场", Kind: "public"}}, Links: []core.StudioWorldLink{{From: "home", To: "square", Minutes: 5}}, People: []core.StudioWorldPerson{{Key: "lin", Name: "Lin", Place: "home", Player: true}, {Key: "cai", Name: "Cai", Place: "square"}}}
	r := StudioGenesisRequest{PrincipalID: "principal_creator", AuthorityInstanceID: M2DemoInstanceID, AuthorityBranchID: M2DemoBranchID, InstanceID: "world-first", IdempotencyKey: "first", Spec: spec}
	if _, err := s.PrepareStudioWorld(ctx, r); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatal("creation without explicit grant", err)
	}
	grant, err := s.ConfigureStudioAccessLocal(ctx, StudioAccessRequest{Purpose: "create_world", Binding: core.CareerBinding{PrincipalID: "principal_operator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: setup.EventSequence, IdempotencyKey: "grant"}, TargetPrincipalID: "principal_creator", Status: "active"})
	if err != nil {
		t.Fatal(err)
	}
	s.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "rollback genesis") }
	if _, err := s.PrepareStudioWorld(ctx, r); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatal("expected rollback", err)
	}
	s.beforeCommit = nil
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM world_instances WHERE instance_id=?`, []any{r.InstanceID}, 0)
	first, err := s.PrepareStudioWorld(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, []any{M2DemoInstanceID, M2DemoBranchID}, grant.EventSequence)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM world_instances WHERE instance_id=? AND lifecycle_state='paused'`, []any{r.InstanceID}, 1)
	assertM2Value(t, ctx, s, `SELECT SUM(p.amount_minor) FROM postings p JOIN journal_entries j ON j.entry_id=p.entry_id WHERE j.event_id=?`, []any{first.EventID}, 0)
	assertM2Value(t, ctx, s, `SELECT population_count FROM cohorts WHERE cohort_id=?`, []any{first.CohortID}, spec.Population)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM outbox WHERE event_id=?`, []any{first.EventID}, 0)
	asset, _ := core.StudioWorldObjectID(r.InstanceID, "account", "asset")
	if _, err := s.db.ExecContext(ctx, `UPDATE account_balances SET balance_minor=1 WHERE account_id=?`, asset); err != nil {
		t.Fatal(err)
	}
	if diffs, err := s.CompareProjections(ctx, r.InstanceID, "br_main"); err != nil || len(diffs) == 0 {
		t.Fatal("corrupt genesis projection unnoticed", diffs, err)
	}
	if err := s.RebuildProjections(ctx, r.InstanceID, "br_main"); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT balance_minor FROM account_balances WHERE account_id=?`, []any{asset}, spec.OpeningMoneyMinor)
	for _, world := range []string{"world-first", "world-zero"} {
		if world == "world-zero" {
			second := r
			second.InstanceID = world
			second.IdempotencyKey = "zero"
			second.Spec.OpeningMoneyMinor = 0
			second.Spec.OpeningStockMinor = 0
			if _, err := s.PrepareStudioWorld(ctx, second); err != nil {
				t.Fatal(err)
			}
		}
		if diffs, err := s.CompareProjections(ctx, world, "br_main"); err != nil || len(diffs) != 0 {
			t.Fatal("genesis replay mismatch", world, diffs, err)
		}
		if err := s.RebuildProjections(ctx, world, "br_main"); err != nil {
			t.Fatal(err)
		}
	}
	changed := r
	changed.Spec.Population++
	if _, err := s.PrepareStudioWorld(ctx, changed); !core.HasCode(err, core.CodeIdempotencyMismatch) {
		t.Fatal("changed specification accepted", err)
	}
	changed = r
	changed.InstanceID = "other-target"
	if _, err := s.PrepareStudioWorld(ctx, changed); !core.HasCode(err, core.CodeIdempotencyMismatch) {
		t.Fatal("changed target accepted", err)
	}
	changed = r
	changed.IdempotencyKey = "collision"
	if _, err := s.PrepareStudioWorld(ctx, changed); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatal("existing target overwritten", err)
	}
	s.Close()
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := s.PrepareStudioWorld(ctx, r)
	if err != nil || !retry.Replayed || retry.EventID != first.EventID {
		t.Fatal("recovery duplicated genesis", retry, err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE instance_id=?`, []any{r.InstanceID}, 1)
}
