package storage

import (
	"context"
	"path/filepath"
	"sync"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPLocationOneSlotAcrossClientsRenameAndRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "shared-spatial.db")
	first := openBootstrappedStore(t, ctx, path)
	setup, err := first.PrepareRPTravel(ctx)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	third, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	stores := []*Store{first, second, third}
	base := RPLocationMaterializeRequest{
		Binding:          core.CareerBinding{PrincipalID: "principal_creator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: setup.EventSequence, IdempotencyKey: "slot-client-0"},
		ParentLocationID: M2AgentCafeID, SlotKey: "side-room",
		Candidate: RPLocationCandidate{DisplayName: "侧间", GeneratorVersion: "local-v1"},
	}
	// A failed write leaves no place, slot, or Event to recover incorrectly.
	first.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "location rollback") }
	if _, err := first.MaterializeRPLocation(ctx, base); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatal("expected rollback", err)
	}
	first.beforeCommit = nil
	assertM2Value(t, ctx, first, `SELECT COUNT(*) FROM rp_location_nodes WHERE parent_location_id=? AND slot_key=?`, []any{base.ParentLocationID, base.SlotKey}, 0)
	assertM2Value(t, ctx, first, `SELECT COUNT(*) FROM events WHERE event_type='RPLocationMaterialized'`, nil, 0)

	type answer struct {
		record RPLocationRecord
		err    error
	}
	answers := make([]answer, 3)
	start := make(chan struct{})
	var group sync.WaitGroup
	for i, store := range stores {
		group.Add(1)
		go func(i int, store *Store) {
			defer group.Done()
			<-start
			request := base
			request.Binding.IdempotencyKey = "slot-client-" + string(rune('0'+i))
			request.Candidate.GeneratorVersion = "local-v" + string(rune('1'+i))
			answers[i].record, answers[i].err = store.MaterializeRPLocation(ctx, request)
		}(i, store)
	}
	close(start)
	group.Wait()
	for i, out := range answers {
		if out.err != nil || out.record.Fact.LocationID == "" || out.record.Fact.LocationID != answers[0].record.Fact.LocationID || out.record.EventID != answers[0].record.EventID {
			t.Fatalf("client %d disagrees on accepted slot: %+v", i, out)
		}
	}
	id := answers[0].record.Fact.LocationID
	assertM2Value(t, ctx, first, `SELECT COUNT(*) FROM rp_location_nodes WHERE instance_id=? AND branch_id=? AND parent_location_id=? AND slot_key=?`, []any{M2DemoInstanceID, M2DemoBranchID, base.ParentLocationID, base.SlotKey}, 1)
	assertM2Value(t, ctx, first, `SELECT COUNT(*) FROM events WHERE event_type='RPLocationMaterialized'`, nil, 1)
	assertM2Value(t, ctx, first, `SELECT COUNT(*) FROM agent_positions WHERE place_id=?`, []any{id}, 0)
	assertM2Value(t, ctx, first, `SELECT COUNT(*) FROM rp_place_links WHERE from_place_id=? OR to_place_id=?`, []any{id, id}, 0)
	assertM2Value(t, ctx, first, `SELECT COUNT(*) FROM materialized_entities WHERE entity_id=?`, []any{id}, 0)

	acceptedIndex := -1
	for i, out := range answers {
		if !out.record.Replayed {
			acceptedIndex = i
		}
	}
	if acceptedIndex < 0 {
		t.Fatal("no materialization command accepted")
	}
	changed := base
	changed.Binding.IdempotencyKey = "slot-client-" + string(rune('0'+acceptedIndex))
	changed.Candidate.GeneratorVersion = "local-v" + string(rune('1'+acceptedIndex))
	changed.Candidate.DisplayName = "不应覆盖"
	if _, err := third.MaterializeRPLocation(ctx, changed); !core.HasCode(err, core.CodeIdempotencyMismatch) {
		t.Fatal("changed exact key should conflict", err)
	}
	rename, err := second.RenameRPLocation(ctx, RPLocationRenameRequest{
		Binding:    core.CareerBinding{PrincipalID: "principal_creator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: setup.EventSequence + 1, IdempotencyKey: "rename-side-room"},
		LocationID: id, NewName: "安静的侧间",
	})
	if err != nil || rename.Fact.LocationID != id {
		t.Fatal("rename stable ID", rename, err)
	}
	assertM2Value(t, ctx, first, `SELECT COUNT(*) FROM agent_places WHERE place_id=? AND display_name='安静的侧间'`, []any{id}, 1)
	for _, store := range stores {
		if err := store.Close(); err != nil {
			t.Fatal(err)
		}
	}
	restarted, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	newVersion := base
	newVersion.Binding.IdempotencyKey = "after-restart-new-generator"
	newVersion.Candidate.DisplayName = "新模型想重抽"
	newVersion.Candidate.GeneratorVersion = "local-v9"
	got, err := restarted.MaterializeRPLocation(ctx, newVersion)
	if err != nil || !got.Replayed || got.Fact.LocationID != id || got.EventID != answers[0].record.EventID {
		t.Fatal("restart or generator changed accepted object", got, err)
	}
	readOnlySlot := newVersion
	readOnlySlot.Binding.IdempotencyKey = "read-existing-without-generator"
	readOnlySlot.Candidate = RPLocationCandidate{}
	if unchanged, err := restarted.MaterializeRPLocation(ctx, readOnlySlot); err != nil || unchanged.Fact.LocationID != id {
		t.Fatal("existing slot required candidate generation", unchanged, err)
	}
	denied := newVersion
	denied.Binding.PrincipalID = "principal_operator"
	if _, err := restarted.MaterializeRPLocation(ctx, denied); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatal("unauthorized slot read", err)
	}
	if diffs, err := restarted.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diffs) != 0 {
		t.Fatal("spatial location changed existing world projections", diffs, err)
	}
	if _, err := restarted.db.ExecContext(ctx, `UPDATE agent_places SET display_name='伪造' WHERE place_id=?`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.db.ExecContext(ctx, `UPDATE rp_location_nodes SET slot_key='wrong-slot' WHERE location_id=?`, id); err != nil {
		t.Fatal(err)
	}
	if diffs, err := restarted.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diffs) != 1 || diffs[0].Projection != "rp_location" {
		t.Fatal("location projection corruption missed", diffs, err)
	}
	if err := restarted.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal("location repair", err)
	}
	if diffs, err := restarted.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diffs) != 0 {
		t.Fatal("location repair not stable", diffs, err)
	}
	assertM2Value(t, ctx, restarted, `SELECT COUNT(*) FROM agent_places p JOIN rp_location_nodes n ON n.location_id=p.place_id WHERE p.place_id=? AND p.display_name='安静的侧间' AND n.slot_key='side-room'`, []any{id}, 1)
}

func TestRPLocationMigration031To032BackfillsDeclaredRoots(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "upgrade-spatial.db")
	store := openBootstrappedStore(t, ctx, path)
	if _, err := store.PrepareRPTravel(ctx); err != nil {
		t.Fatal(err)
	}
	var places int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM agent_places WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&places); err != nil {
		t.Fatal(err)
	}
	// In this isolated fixture, remove only migration032's additive projection
	// and marker. Reopening then exercises the actual embedded 031→032 SQL.
	if _, err := store.db.ExecContext(ctx, `DROP VIEW rp_occupancy_intervals`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `DROP INDEX ix_rp_agent_movement_time`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `DROP TABLE rp_location_nodes`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `DELETE FROM schema_meta WHERE schema_version=?`, RPSpatialLocationSchemaVersion); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	upgraded, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.Close()
	assertM2Value(t, ctx, upgraded, `SELECT COUNT(*) FROM rp_location_nodes WHERE instance_id=? AND branch_id=? AND parent_location_id IS NULL AND slot_key='' AND generator_version='legacy'`, []any{M2DemoInstanceID, M2DemoBranchID}, int64(places))
	if diffs, err := upgraded.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diffs) != 0 {
		t.Fatal("upgraded roots changed world replay", diffs, err)
	}
}

func TestRPOccupancyIntervalsDoNotDoubleCountSameTimeMoves(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "occupancy.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, read, _ := newRPWaitTestSession(t, ctx, s)
	view, err := s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range []struct{ destination, key string }{{"place_m2_home_ada", "same-time-out"}, {M2AgentCafeID, "same-time-back"}} {
		if _, err := s.MoveRP(ctx, core.RPMoveRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, FromPlaceID: view.PlaceID, ToPlaceID: step.destination, ExpectedCursor: view.ObservationCursor, IdempotencyKey: step.key}); err != nil {
			t.Fatal(err)
		}
		view, err = s.ObserveRPSession(ctx, read)
		if err != nil {
			t.Fatal(err)
		}
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_occupancy_intervals WHERE instance_id=? AND branch_id=? AND agent_id=? AND exited_at IS NULL AND location_id=?`, []any{M2DemoInstanceID, M2DemoBranchID, M2RPPlayerID, M2AgentCafeID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_occupancy_intervals WHERE instance_id=? AND branch_id=? AND agent_id=? AND exited_at IS NULL`, []any{M2DemoInstanceID, M2DemoBranchID, M2RPPlayerID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_occupancy_intervals WHERE instance_id=? AND branch_id=? AND agent_id=? AND entered_at=exited_at AND entered_sequence<exited_sequence`, []any{M2DemoInstanceID, M2DemoBranchID, M2RPPlayerID}, 2)
}
