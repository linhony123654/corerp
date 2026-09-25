package storage

import (
	"context"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPLocationSlotIsolationAcrossIndependentWorlds(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "two-spatial-worlds.db")
	s := openBootstrappedStore(t, ctx, path)
	defer func() { s.Close() }()
	setup, err := s.PrepareRPTravel(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ConfigureStudioAccessLocal(ctx, StudioAccessRequest{Purpose: "create_world", Binding: core.CareerBinding{PrincipalID: "principal_operator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: setup.EventSequence, IdempotencyKey: "scope-create-grant"}, TargetPrincipalID: "principal_creator", Status: "active"}); err != nil {
		t.Fatal(err)
	}
	worlds := []string{"spatial-scope-one", "spatial-scope-two"}
	ids := make([]string, len(worlds))
	parents := make([]string, len(worlds))
	for i, world := range worlds {
		if _, err := s.CreateStudioWorld(ctx, studioCreateFixture(world)); err != nil {
			t.Fatal("create independent world", world, err)
		}
		parent, err := core.StudioWorldObjectID(world, "place", "home")
		if err != nil {
			t.Fatal(err)
		}
		parents[i] = parent
		var head int64
		if err := s.db.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id='br_main'`, world).Scan(&head); err != nil {
			t.Fatal(err)
		}
		r := RPLocationMaterializeRequest{Binding: core.CareerBinding{PrincipalID: "principal_creator", InstanceID: world, BranchID: "br_main", ExpectedHead: head, IdempotencyKey: "shared-slot-key"}, ParentLocationID: parent, SlotKey: "same-logical-slot", Candidate: RPLocationCandidate{DisplayName: "同名房间", GeneratorVersion: "local-v1"}}
		record, err := s.MaterializeRPLocation(ctx, r)
		if err != nil {
			t.Fatal("materialize world slot", world, err)
		}
		ids[i] = record.Fact.LocationID
		if diffs, err := s.CompareProjections(ctx, world, "br_main"); err != nil || len(diffs) != 0 {
			t.Fatal("scope projection", world, diffs, err)
		}
	}
	if ids[0] == ids[1] {
		t.Fatal("same slot collapsed independent world identities", ids)
	}
	var worldOneHead int64
	if err := s.db.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id='br_main'`, worlds[0]).Scan(&worldOneHead); err != nil {
		t.Fatal(err)
	}
	foreign := RPLocationMaterializeRequest{Binding: core.CareerBinding{PrincipalID: "principal_creator", InstanceID: worlds[0], BranchID: "br_main", ExpectedHead: worldOneHead, IdempotencyKey: "foreign-parent"}, ParentLocationID: parents[1], SlotKey: "foreign", Candidate: RPLocationCandidate{DisplayName: "不属于本世界", GeneratorVersion: "local-v1"}}
	if _, err := s.MaterializeRPLocation(ctx, foreign); !core.HasCode(err, core.CodeNotFound) {
		t.Fatal("cross-world parent accepted", err)
	}
	foreignBranch := foreign
	foreignBranch.Binding.BranchID = "br_shadow"
	foreignBranch.ParentLocationID = parents[0]
	foreignBranch.Binding.IdempotencyKey = "foreign-branch"
	if _, err := s.MaterializeRPLocation(ctx, foreignBranch); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatal("unprepared branch inherited parent/creator authority", err)
	}
	for i, world := range worlds {
		assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_location_nodes WHERE instance_id=? AND branch_id='br_main' AND location_id=? AND parent_location_id=?`, []any{world, ids[i], parents[i]}, 1)
	}
	if err := s.RebuildProjections(ctx, worlds[0], "br_main"); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_location_nodes WHERE location_id=? AND instance_id=?`, []any{ids[1], worlds[1]}, 1)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	for i, world := range worlds {
		r := RPLocationMaterializeRequest{Binding: core.CareerBinding{PrincipalID: "principal_creator", InstanceID: world, BranchID: "br_main", ExpectedHead: 1, IdempotencyKey: "after-restart"}, ParentLocationID: parents[i], SlotKey: "same-logical-slot", Candidate: RPLocationCandidate{DisplayName: "新模型名称", GeneratorVersion: "local-v9"}}
		got, err := s.MaterializeRPLocation(ctx, r)
		if err != nil || !got.Replayed || got.Fact.LocationID != ids[i] {
			t.Fatal("restart changed scoped slot", world, got, err)
		}
	}
}
