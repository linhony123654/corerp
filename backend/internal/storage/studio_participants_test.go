package storage

import (
	"context"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestStudioParticipantsConservedAtomicIsolatedAndRecoverable(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "participants.db")
	s := openBootstrappedStore(t, ctx, path)
	defer func() { s.Close() }()
	setup, err := s.PrepareRPTravel(ctx)
	if err != nil {
		t.Fatal(err)
	}
	access := StudioAccessRequest{Purpose: "create_world", Binding: core.CareerBinding{PrincipalID: "principal_operator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: setup.EventSequence, IdempotencyKey: "grant"}, TargetPrincipalID: "principal_creator", Status: "active"}
	grant, err := s.ConfigureStudioAccessLocal(ctx, access)
	if err != nil {
		t.Fatal(err)
	}
	spec := core.StudioWorldSpec{Version: core.StudioWorldSpecVersion, Name: "新世界", StartWorldTime: core.StudioWorldStart, Population: 3, OpeningMoneyMinor: 11, OpeningStockMinor: 5, Places: []core.StudioWorldPlace{{Key: "home", Name: "家", Kind: "home"}, {Key: "square", Name: "广场", Kind: "public"}}, Links: []core.StudioWorldLink{{From: "home", To: "square", Minutes: 5}}, People: []core.StudioWorldPerson{{Key: "lin", Name: "Lin", Place: "home", Player: true}, {Key: "cai", Name: "Cai", Place: "square"}}}
	r := StudioGenesisRequest{PrincipalID: "principal_creator", AuthorityInstanceID: M2DemoInstanceID, AuthorityBranchID: M2DemoBranchID, InstanceID: "participants-first", IdempotencyKey: "first", Spec: spec}
	first, err := s.PrepareStudioWorld(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	changed := r
	changed.Spec.Population++
	if _, err := s.PrepareStudioParticipants(ctx, changed); !core.HasCode(err, core.CodeIdempotencyMismatch) {
		t.Fatal("changed spec", err)
	}
	changed = r
	changed.PrincipalID = "principal_operator"
	if _, err := s.PrepareStudioParticipants(ctx, changed); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatal("role bypass", err)
	}
	asset, _ := core.StudioWorldObjectID(r.InstanceID, "account", "asset")
	if _, err := s.db.ExecContext(ctx, `UPDATE account_balances SET balance_minor=12 WHERE account_id=?`, asset); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PrepareStudioParticipants(ctx, r); !core.HasCode(err, core.CodeProjectionDiverged) {
		t.Fatal("corrupt initial projection", err)
	}
	if err := s.RebuildProjections(ctx, r.InstanceID, "br_main"); err != nil {
		t.Fatal(err)
	}
	// Fail on the second participant after the first has written its complete
	// lineage, balances and Outbox. No partial batch may survive.
	if _, err := s.db.ExecContext(ctx, `CREATE TRIGGER fail_second_studio_participant BEFORE INSERT ON materialized_entities WHEN NEW.display_name='Cai' BEGIN SELECT RAISE(ABORT,'second participant fault'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PrepareStudioParticipants(ctx, r); err == nil {
		t.Fatal("missing second-person failure")
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM materialized_entities WHERE source_cohort_id=?`, []any{first.CohortID}, 0)
	assertM2Value(t, ctx, s, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id='br_main'`, []any{r.InstanceID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM population_movements p JOIN events e ON e.event_id=p.event_id WHERE e.instance_id=?`, []any{r.InstanceID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM outbox o JOIN events e ON e.event_id=o.event_id WHERE e.instance_id=?`, []any{r.InstanceID}, 0)
	if _, err := s.db.ExecContext(ctx, `DROP TRIGGER fail_second_studio_participant`); err != nil {
		t.Fatal(err)
	}
	s.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "rollback participant set") }
	if _, err := s.PrepareStudioParticipants(ctx, r); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatal("commit failure", err)
	}
	s.beforeCommit = nil
	assertM2Value(t, ctx, s, `SELECT population_count FROM cohorts WHERE cohort_id=?`, []any{first.CohortID}, 3)
	results, err := s.PrepareStudioParticipants(ctx, r)
	if err != nil || len(results) != 2 {
		t.Fatal(results, err)
	}
	for i, result := range results {
		if result.Replayed || result.FirstSequence != int64(i+2) {
			t.Fatal(result)
		}
		assertM2Value(t, ctx, s, `SELECT b.balance_minor FROM materialized_entities e JOIN account_balances b ON b.account_id=e.asset_account_id WHERE e.entity_id=?`, []any{result.EntityID}, int64(i+3))
		assertM2Value(t, ctx, s, `SELECT b.quantity_minor FROM materialized_entities e JOIN inventory_balances b ON b.location_id=e.inventory_location_id AND b.sku_id=e.sku_id WHERE e.entity_id=?`, []any{result.EntityID}, int64(i+1))
	}
	assertM2Value(t, ctx, s, `SELECT population_count FROM cohorts WHERE cohort_id=?`, []any{first.CohortID}, 1)
	assertM2Value(t, ctx, s, `SELECT balance_minor FROM account_balances WHERE account_id=?`, []any{asset}, 4)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM capability_grants WHERE instance_id=?`, []any{r.InstanceID}, 0)
	assertM2Value(t, ctx, s, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, []any{M2DemoInstanceID, M2DemoBranchID}, grant.EventSequence)
	// Same local keys in another world cannot alias people, accounts or receipts.
	second := r
	second.InstanceID = "participants-zero"
	second.IdempotencyKey = "zero"
	second.Spec.Population = 2
	second.Spec.OpeningMoneyMinor = 0
	second.Spec.OpeningStockMinor = 0
	zero, err := s.PrepareStudioWorld(ctx, second)
	if err != nil {
		t.Fatal(err)
	}
	other, err := s.PrepareStudioParticipants(ctx, second)
	if err != nil || len(other) != 2 || other[0].EntityID == results[0].EntityID {
		t.Fatal(other, err)
	}
	assertM2Value(t, ctx, s, `SELECT population_count FROM cohorts WHERE cohort_id=? AND status='depleted'`, []any{zero.CohortID}, 0)
	full := r
	full.InstanceID = "participants-full"
	full.IdempotencyKey = "full"
	full.Spec.Population = 2
	fullGenesis, err := s.PrepareStudioWorld(ctx, full)
	if err != nil {
		t.Fatal(err)
	}
	fullPeople, err := s.PrepareStudioParticipants(ctx, full)
	if err != nil || len(fullPeople) != 2 {
		t.Fatal(fullPeople, err)
	}
	assertM2Value(t, ctx, s, `SELECT population_count FROM cohorts WHERE cohort_id=? AND status='depleted'`, []any{fullGenesis.CohortID}, 0)
	assertM2Value(t, ctx, s, `SELECT SUM(b.balance_minor) FROM materialized_entities e JOIN account_balances b ON b.account_id=e.asset_account_id WHERE e.source_cohort_id=?`, []any{fullGenesis.CohortID}, 11)
	assertM2Value(t, ctx, s, `SELECT SUM(b.quantity_minor) FROM materialized_entities e JOIN inventory_balances b ON b.location_id=e.inventory_location_id AND b.sku_id=e.sku_id WHERE e.source_cohort_id=?`, []any{fullGenesis.CohortID}, 5)
	for _, world := range []string{r.InstanceID, second.InstanceID, full.InstanceID} {
		if diffs, err := s.CompareProjections(ctx, world, "br_main"); err != nil || len(diffs) != 0 {
			t.Fatal("participant replay", world, diffs, err)
		}
		if err := s.RebuildProjections(ctx, world, "br_main"); err != nil {
			t.Fatal(err)
		}
		if diffs, err := s.CompareProjections(ctx, world, "br_main"); err != nil || len(diffs) != 0 {
			t.Fatal("rebuilt participant projection", world, diffs, err)
		}
	}
	s.Close()
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := s.PrepareStudioParticipants(ctx, r)
	if err != nil || len(retry) != 2 {
		t.Fatal(retry, err)
	}
	for i, result := range retry {
		if !result.Replayed || result.EventID != results[i].EventID {
			t.Fatal("duplicate on retry", result)
		}
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE instance_id=?`, []any{r.InstanceID}, 3)
	access.Status = "revoked"
	access.Binding.ExpectedHead = grant.EventSequence
	access.Binding.IdempotencyKey = "revoke"
	if _, err := s.ConfigureStudioAccessLocal(ctx, access); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PrepareStudioParticipants(ctx, r); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatal("revoked retry", err)
	}
}
