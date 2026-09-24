package storage

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestStudioSpatialSourceIsolationRollbackAndRecovery(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "spatial.db")
	s := openBootstrappedStore(t, ctx, path)
	defer func() { s.Close() }()
	setup, err := s.PrepareRPTravel(ctx)
	if err != nil {
		t.Fatal(err)
	}
	access := StudioAccessRequest{Purpose: "create_world", Binding: core.CareerBinding{PrincipalID: "principal_operator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: setup.EventSequence, IdempotencyKey: "spatial-create-grant"}, TargetPrincipalID: "principal_creator", Status: "active"}
	grant, err := s.ConfigureStudioAccessLocal(ctx, access)
	if err != nil {
		t.Fatal(err)
	}
	spec := core.StudioWorldSpec{Version: core.StudioWorldSpecVersion, Name: "新世界", StartWorldTime: core.StudioWorldStart, Population: 3, OpeningMoneyMinor: 11, OpeningStockMinor: 5, Places: []core.StudioWorldPlace{{Key: "home", Name: "家", Kind: "home"}, {Key: "square", Name: "广场", Kind: "public"}}, Links: []core.StudioWorldLink{{From: "home", To: "square", Minutes: 5}}, People: []core.StudioWorldPerson{{Key: "lin", Name: "Lin", Place: "home", Player: true}, {Key: "cai", Name: "Cai", Place: "square"}}}
	r := StudioGenesisRequest{PrincipalID: "principal_creator", AuthorityInstanceID: M2DemoInstanceID, AuthorityBranchID: M2DemoBranchID, InstanceID: "spatial-first", IdempotencyKey: "first", Spec: spec}
	var first privateFactRecord[StudioSpatialFact]
	for _, world := range []string{"spatial-first", "spatial-second"} {
		request := r
		request.InstanceID = world
		request.IdempotencyKey = world
		if world == r.InstanceID {
			request = r
		}
		if _, err := s.PrepareStudioWorld(ctx, request); err != nil {
			t.Fatal(err)
		}
		if _, err := s.PrepareStudioSpatial(ctx, request); !core.HasCode(err, core.CodeBranchConflict) {
			t.Fatal("spatial actors before conserved people", err)
		}
		if _, err := s.PrepareStudioParticipants(ctx, request); err != nil {
			t.Fatal(err)
		}
		if world == r.InstanceID {
			s.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "spatial rollback") }
			if _, err := s.PrepareStudioSpatial(ctx, request); !core.HasCode(err, core.CodeInjectedFailure) {
				t.Fatal(err)
			}
			s.beforeCommit = nil
			assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_places WHERE instance_id=?`, []any{world}, 0)
			assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_profiles WHERE instance_id=?`, []any{world}, 0)
			assertM2Value(t, ctx, s, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id='br_main'`, []any{world}, 3)
			principal, _ := core.StudioWorldObjectID(world, "principal", "lin")
			assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM principals WHERE principal_id=?`, []any{principal}, 0)
		}
		result, err := s.PrepareStudioSpatial(ctx, request)
		if err != nil || result.Replayed || result.EventSequence != 4 {
			t.Fatal(result, err)
		}
		if world == r.InstanceID {
			first = result
		} else if result.Fact.PlayerEntityID == first.Fact.PlayerEntityID || result.Fact.PlayerPrincipalID == first.Fact.PlayerPrincipalID {
			t.Fatal("world identity collision")
		}
		assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_places WHERE instance_id=?`, []any{world}, 2)
		assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_place_links WHERE instance_id=?`, []any{world}, 2)
		assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_profiles a JOIN agent_positions p ON p.agent_id=a.agent_id JOIN agent_places l ON l.place_id=p.place_id WHERE a.instance_id=? AND l.instance_id=a.instance_id AND l.branch_id=a.branch_id`, []any{world}, 2)
		assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM capability_grants WHERE instance_id=?`, []any{world}, 0)
		assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM outbox WHERE event_id=?`, []any{result.EventID}, 0)
		assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM observation_records WHERE source_event_id=?`, []any{result.EventID}, 0)
		for _, person := range spec.People {
			entity, _ := core.StudioWorldObjectID(world, "entity", person.Key)
			place, _ := core.StudioWorldObjectID(world, "place", person.Place)
			assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_positions WHERE agent_id=? AND place_id=?`, []any{entity, place}, 1)
		}
		if diffs, err := s.CompareProjections(ctx, world, "br_main"); err != nil || len(diffs) != 0 {
			t.Fatal("initial spatial replay", diffs, err)
		}
		if _, err := s.db.ExecContext(ctx, `UPDATE agent_positions SET activity_code='corrupted' WHERE agent_id=?`, result.Fact.PlayerEntityID); err != nil {
			t.Fatal(err)
		}
		if diffs, err := s.CompareProjections(ctx, world, "br_main"); err != nil || len(diffs) == 0 {
			t.Fatal("corruption missed", diffs, err)
		}
		if err := s.RebuildProjections(ctx, world, "br_main"); err != nil {
			t.Fatal(err)
		}
		if diffs, err := s.CompareProjections(ctx, world, "br_main"); err != nil || len(diffs) != 0 {
			t.Fatal("rebuilt spatial replay", diffs, err)
		}
	}
	assertM2Value(t, ctx, s, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, []any{M2DemoInstanceID, M2DemoBranchID}, grant.EventSequence)
	changed := r
	changed.Spec.Name = "改名"
	if _, err := s.PrepareStudioSpatial(ctx, changed); !core.HasCode(err, core.CodeIdempotencyMismatch) {
		t.Fatal("altered genesis", err)
	}
	changed = r
	changed.PrincipalID = "principal_operator"
	if _, err := s.PrepareStudioSpatial(ctx, changed); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatal("operator bypass", err)
	}
	s.Close()
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	// Exercise the real scoped movement owner with a source-backed test schedule,
	// not a production player grant or a claim of package activation/Play readiness.
	testStudioScheduledMovement(t, ctx, s, r, first)
	retry, err := s.PrepareStudioSpatial(ctx, r)
	if err != nil || !retry.Replayed || retry.EventID != first.EventID {
		t.Fatal("retry after reopen", retry, err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE instance_id=?`, []any{r.InstanceID}, 6)
	assertM2Value(t, ctx, s, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, []any{M2DemoInstanceID, M2DemoBranchID}, grant.EventSequence)
	assertM2Value(t, ctx, s, `SELECT head_sequence FROM branches WHERE instance_id='spatial-second' AND branch_id='br_main'`, nil, 4)
	access.Status = "revoked"
	access.Binding.ExpectedHead = grant.EventSequence
	access.Binding.IdempotencyKey = "revoke-spatial"
	if _, err := s.ConfigureStudioAccessLocal(ctx, access); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PrepareStudioSpatial(ctx, r); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatal("revoked spatial retry", err)
	}
}

func testStudioScheduledMovement(t *testing.T, ctx context.Context, s *Store, r StudioGenesisRequest, spatial privateFactRecord[StudioSpatialFact]) {
	t.Helper()
	id := func(kind, key string) string { v, _ := core.StudioWorldObjectID(r.InstanceID, kind, key); return v }
	at := "2026-09-22T00:05:00Z"
	item, schedule := id("queue", "test-walk"), id("schedule", "test-walk")
	payload := agentSchedulePayload{Kind: "agent_move", Day: 0, AgentID: spatial.Fact.PlayerEntityID, ScheduleID: schedule, ToPlaceID: id("place", "square"), ActivityCode: "visit"}
	encoded, err := core.CanonicalJSON(payload)
	if err != nil {
		t.Fatal(err)
	}
	_, err = executePrivateFactCommand(s, ctx, core.CareerBinding{PrincipalID: r.PrincipalID, InstanceID: r.InstanceID, BranchID: "br_main", ExpectedHead: 4, IdempotencyKey: "test-walk"}, "TestDefineStudioSchedule", payload,
		privateFactDomain{"test_studio_schedule", "TestStudioScheduleDefined", `{"authorization":"test-fixture"}`},
		func(conn *sql.Conn) error { return authorizeSavedStudioGenesis(ctx, conn, r) },
		func(conn *sql.Conn, c privateFactContext) (agentSchedulePayload, func() error, error) {
			return payload, func() error {
				if err := execAgentOne(ctx, conn, "test schedule queue", `INSERT INTO scheduler_items(scheduler_item_id,instance_id,branch_id,world_time,phase_id,declared_priority,status,payload) VALUES (?,?,'br_main',?,?,0,'pending',?)`, item, r.InstanceID, at, m2AgentPhaseID, string(encoded)); err != nil {
					return err
				}
				return execAgentOne(ctx, conn, "test schedule definition", `INSERT INTO agent_schedule_entries(schedule_id,agent_id,world_time,place_id,activity_code,declared_priority,scheduler_item_id,status,definition_event_id) VALUES (?,?,?,?,'visit',0,?,'active',?)`, schedule, payload.AgentID, at, payload.ToPlaceID, item, c.EventID)
			}, nil
		})
	if err != nil {
		t.Fatal(err)
	}
	run, err := s.runAgentLifeForScope(ctx, r.InstanceID, "br_main", at, 1)
	if err != nil || run.ProcessedItems != 1 || run.HeadSequence != 6 || run.CurrentWorldTime != at {
		t.Fatal("real second-world scheduled movement", run, err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_positions WHERE agent_id=? AND place_id=? AND effective_world_time=?`, []any{payload.AgentID, payload.ToPlaceID, at}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM observation_records WHERE source_event_id=?`, []any{"event_" + item}, 2)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM outbox WHERE event_id=? AND json_extract(audience_scope,'$.instance_id')=?`, []any{"event_" + item, r.InstanceID}, 1)
	if work, err := s.executeNextAgentScheduleForScope(ctx, r.InstanceID, "br_main", at); err != nil || work {
		t.Fatal("movement executed twice", work, err)
	}
	if diffs, err := s.CompareProjections(ctx, r.InstanceID, "br_main"); err != nil || len(diffs) != 0 {
		t.Fatal("moved world replay", diffs, err)
	}
	if err := s.RebuildProjections(ctx, r.InstanceID, "br_main"); err != nil {
		t.Fatal(err)
	}
	if diffs, err := s.CompareProjections(ctx, r.InstanceID, "br_main"); err != nil || len(diffs) != 0 {
		t.Fatal("moved world rebuild", diffs, err)
	}
}
