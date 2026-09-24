package storage

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
)

func prepareStudioReadyTestWorld(t *testing.T, ctx context.Context, s *Store, world string) StudioWorldSaveRequest {
	t.Helper()
	g := StudioGenesisRequest{PrincipalID: "principal_creator", AuthorityInstanceID: M2DemoInstanceID, AuthorityBranchID: M2DemoBranchID, InstanceID: world, IdempotencyKey: world, Spec: core.StudioWorldSpec{Version: core.StudioWorldSpecVersion, Name: "新世界", StartWorldTime: core.StudioWorldStart, Population: 2, OpeningMoneyMinor: 10, OpeningStockMinor: 2, Places: []core.StudioWorldPlace{{Key: "home", Name: "家", Kind: "home"}, {Key: "square", Name: "广场", Kind: "public"}}, Links: []core.StudioWorldLink{{From: "home", To: "square", Minutes: 5}}, People: []core.StudioWorldPerson{{Key: "lin", Name: "Lin", Place: "home", Player: true}, {Key: "cai", Name: "Cai", Place: "home"}}}}
	if _, err := s.PrepareStudioWorld(ctx, g); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PrepareStudioParticipants(ctx, g); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PrepareStudioSpatial(ctx, g); err != nil {
		t.Fatal(err)
	}
	b := core.CareerBinding{PrincipalID: g.PrincipalID, InstanceID: world, BranchID: "br_main", ExpectedHead: 4, IdempotencyKey: "system"}
	if _, err := s.InstallStudioPackage(ctx, StudioPackageInstallRequest{Genesis: g, Binding: b, Bundle: studioTestPackage("system")}); err != nil {
		t.Fatal(err)
	}
	b.ExpectedHead = 5
	b.IdempotencyKey = "narrative"
	if _, err := s.InstallStudioPackage(ctx, StudioPackageInstallRequest{Genesis: g, Binding: b, Bundle: studioTestPackage("narrative")}); err != nil {
		t.Fatal(err)
	}
	b.ExpectedHead = 6
	b.IdempotencyKey = "activate"
	if _, err := s.ActivateStudioPackages(ctx, StudioPackageActivationRequest{Genesis: g, Binding: b, SystemPackageID: "test.system", NarrativePackageID: "test.narrative"}); err != nil {
		t.Fatal(err)
	}
	b.ExpectedHead = 7
	b.IdempotencyKey = "save"
	return StudioWorldSaveRequest{Genesis: g, Binding: b, PlayerPrincipalID: M2RPPlayerPrincipal}
}

func TestStudioReadyPlayerBoundaryActualPlayAndRecovery(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "ready.db")
	s := openBootstrappedStore(t, ctx, path)
	defer func() { s.Close() }()
	setup, err := s.PrepareRPTravel(ctx)
	if err != nil {
		t.Fatal(err)
	}
	access := StudioAccessRequest{Purpose: "create_world", Binding: core.CareerBinding{PrincipalID: "principal_operator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: setup.EventSequence, IdempotencyKey: "ready-grant"}, TargetPrincipalID: "principal_creator", Status: "active"}
	grant, err := s.ConfigureStudioAccessLocal(ctx, access)
	if err != nil {
		t.Fatal(err)
	}
	var firstRequest StudioWorldSaveRequest
	var first privateFactRecord[StudioWorldReadyFact]
	for _, world := range []string{"ready-first", "ready-second"} {
		r := prepareStudioReadyTestWorld(t, ctx, s, world)
		entity, _ := core.StudioWorldObjectID(world, "entity", "lin")
		open := core.RPSessionOpenRequest{PrincipalID: M2RPPlayerPrincipal, InstanceID: world, BranchID: "br_main", EntityID: entity, POV: "second_person", IdempotencyKey: world}
		if _, err := s.OpenRPSession(ctx, open); !core.HasCode(err, core.CodeUnauthorized) {
			t.Fatal("prepared world exposed to player", err)
		}
		for _, principal := range []string{"principal_creator", "principal_operator", "missing-player"} {
			wrong := r
			wrong.PlayerPrincipalID = principal
			if _, err := s.SaveStudioWorld(ctx, wrong); !core.HasCode(err, core.CodeUnauthorized) {
				t.Fatal("invalid player target", principal, err)
			}
		}
		s.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "ready rollback") }
		if _, err := s.SaveStudioWorld(ctx, r); !core.HasCode(err, core.CodeInjectedFailure) {
			t.Fatal(err)
		}
		s.beforeCommit = nil
		assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM world_instances WHERE instance_id=? AND lifecycle_state='paused'`, []any{world}, 1)
		assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM capability_grants WHERE instance_id=?`, []any{world}, 0)
		ready, err := s.SaveStudioWorld(ctx, r)
		if err != nil || ready.EventSequence != 8 || ready.Fact.PlayerPrincipalID != M2RPPlayerPrincipal {
			t.Fatal(ready, err)
		}
		if world == "ready-first" {
			firstRequest, first = r, ready
		}
		creator := open
		creator.PrincipalID = "principal_creator"
		if _, err := s.OpenRPSession(ctx, creator); !core.HasCode(err, core.CodeUnauthorized) {
			t.Fatal("creator gained player control", err)
		}
		session, err := s.OpenRPSession(ctx, open)
		if err != nil {
			t.Fatal(err)
		}
		read := core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID}
		observation, err := s.ObserveRPSession(ctx, read)
		if err != nil || observation.ControlledEntity.EntityID != entity {
			t.Fatal(observation, err)
		}
		speech, err := s.SpeakRP(ctx, core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, Text: "你好", SpeechAct: "statement", ExpectedCursor: observation.ObservationCursor, IdempotencyKey: "hello"})
		if err != nil || len(speech.ListenerIDs) != 1 {
			t.Fatal("new world speech", speech, err)
		}
		observation, err = s.ObserveRPSession(ctx, read)
		if err != nil {
			t.Fatal(err)
		}
		to, _ := core.StudioWorldObjectID(world, "place", "square")
		moved, err := s.MoveRP(ctx, core.RPMoveRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, FromPlaceID: observation.PlaceID, ToPlaceID: to, ExpectedCursor: observation.ObservationCursor, IdempotencyKey: "walk"})
		if err != nil || moved.ToPlaceID != to {
			t.Fatal("new world move", moved, err)
		}
		observation, err = s.ObserveRPSession(ctx, read)
		if err != nil {
			t.Fatal(err)
		}
		waitRequest := core.RPWaitRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, TargetWorldTime: "2026-09-22T00:05:00Z", Budget: 4, ExpectedCursor: moved.EventSequence, IdempotencyKey: "wait"}
		waited, err := s.WaitRP(ctx, waitRequest)
		if err != nil || waited.Status != "completed" || waited.CurrentWorldTime != waitRequest.TargetWorldTime {
			t.Fatal("new world Wait", waited, err)
		}
		waitRetry, err := s.WaitRP(ctx, waitRequest)
		if err != nil || !waitRetry.Replayed || waitRetry.EventID != waited.EventID {
			t.Fatal("Wait retry", waitRetry, err)
		}
		observation, err = s.ObserveRPSession(ctx, read)
		if err != nil {
			t.Fatal(err)
		}
		style, err := s.ReadRPStyle(ctx, read)
		if err != nil || style.Profile.Verbosity != "terse" {
			t.Fatal("installed style in actual session", style, err)
		}
		// An unsupported due phase must not strand a newly persisted Wait intent.
		item := "unsupported_" + world
		if _, err := s.db.ExecContext(ctx, `INSERT INTO scheduler_items(scheduler_item_id,instance_id,branch_id,world_time,phase_id,declared_priority,status,payload) SELECT ?,?,'br_main','2026-09-22T00:06:00Z',phase_id,0,'pending','{}' FROM scheduler_phases WHERE phase_id<>? LIMIT 1`, item, world, m2AgentPhaseID); err != nil {
			t.Fatal(err)
		}
		unsupported := waitRequest
		unsupported.IdempotencyKey = "unsupported"
		unsupported.ExpectedCursor = waited.EventSequence
		unsupported.TargetWorldTime = "2026-09-22T00:10:00Z"
		if _, err := s.WaitRP(ctx, unsupported); !core.HasCode(err, core.CodeInvalidArgument) {
			t.Fatal("foreign phase accepted", err)
		}
		assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_wait_intents WHERE session_id=?`, []any{read.SessionID}, 1)
		if _, err := s.db.ExecContext(ctx, `DELETE FROM scheduler_items WHERE scheduler_item_id=?`, item); err != nil {
			t.Fatal(err)
		}
		// Rules unavailable: new writes fail, but historical replay is intact.
		if _, err := s.db.ExecContext(ctx, `UPDATE rule_epochs SET lock_document='{}' WHERE instance_id=? AND end_sequence IS NULL`, world); err != nil {
			t.Fatal(err)
		}
		if _, err := s.SpeakRP(ctx, core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, Text: "不应提交", ExpectedCursor: waited.EventSequence, IdempotencyKey: "missing-rules"}); !core.HasCode(err, core.CodeProjectionDiverged) {
			t.Fatal("write with unavailable rules", err)
		}
		if _, err := s.Replay(ctx, world, "br_main", waited.EventSequence); err != nil {
			t.Fatal("read-only replay requires active rules", err)
		}
		if err := s.RebuildProjections(ctx, world, "br_main"); err != nil {
			t.Fatal(err)
		}
		if diffs, err := s.CompareProjections(ctx, world, "br_main"); err != nil || len(diffs) != 0 {
			t.Fatal("Play replay", diffs, err)
		}
	}
	assertM2Value(t, ctx, s, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, []any{M2DemoInstanceID, M2DemoBranchID}, grant.EventSequence)
	discovery, err := s.DiscoverRPBindings(ctx, RPDiscoverRequest{PrincipalID: M2RPPlayerPrincipal})
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, b := range discovery.Bindings {
		found[b.InstanceID] = true
	}
	if !found["ready-first"] || !found["ready-second"] {
		t.Fatal("saved worlds not discoverable", discovery)
	}
	for _, q := range []string{`UPDATE events SET payload='{}' WHERE event_id=?`, `DELETE FROM events WHERE event_id=?`, `INSERT OR REPLACE INTO events SELECT * FROM events WHERE event_id=?`} {
		if _, err := s.db.ExecContext(ctx, q, first.EventID); err == nil {
			t.Fatal("ready source mutable", q)
		}
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE capability_grants SET definition_event_id=? WHERE grant_id=?`, first.Fact.GenesisEventID, first.Fact.GrantID); err != nil {
		t.Fatal(err)
	}
	discovery, err = s.DiscoverRPBindings(ctx, RPDiscoverRequest{PrincipalID: M2RPPlayerPrincipal})
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range discovery.Bindings {
		if b.InstanceID == "ready-first" {
			t.Fatal("unsourced grant discovered")
		}
	}
	if diffs, err := s.CompareProjections(ctx, "ready-first", "br_main"); err != nil || len(diffs) == 0 {
		t.Fatal("control corruption unnoticed", diffs, err)
	}
	if err := s.RebuildProjections(ctx, "ready-first", "br_main"); err != nil {
		t.Fatal(err)
	}
	if diffs, err := s.CompareProjections(ctx, "ready-first", "br_main"); err != nil || len(diffs) != 0 {
		t.Fatal("ready recovery", diffs, err)
	}
	s.Close()
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := s.SaveStudioWorld(ctx, firstRequest)
	if err != nil || !retry.Replayed || retry.EventID != first.EventID {
		t.Fatal("save reopen retry", retry, err)
	}
	access.Status = "revoked"
	access.Binding.ExpectedHead = grant.EventSequence
	access.Binding.IdempotencyKey = "revoke-save"
	if _, err := s.ConfigureStudioAccessLocal(ctx, access); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveStudioWorld(ctx, firstRequest); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatal("revoked save retry", err)
	}
}

func TestStudioReadyMigration029Upgrade(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "upgrade.db")
	s := openBootstrappedStore(t, ctx, path)
	embedded, err := migrationFiles.ReadFile("migrations/030_studio_world_ready.sql")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := os.ReadFile("../../../docs/rp8/schema-030-world-ready.sql")
	if err != nil || !bytes.Equal(embedded, doc) {
		t.Fatal("readiness migration contract", err)
	}
	for _, q := range []string{`DROP TRIGGER studio_ready_event_no_update`, `DROP TRIGGER studio_ready_event_no_delete`, `DROP TRIGGER studio_ready_event_no_replace`, `DELETE FROM schema_meta WHERE schema_version='corerp-rp8-world-ready-030-2026-09-24'`} {
		if _, err := s.db.ExecContext(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	s.Close()
	for i := 0; i < 2; i++ {
		s, err = Open(ctx, path)
		if err != nil {
			t.Fatal(err)
		}
		assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM schema_meta WHERE schema_version=?`, []any{StudioReadySchemaVersion}, 1)
		assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM sqlite_master WHERE type='trigger' AND name LIKE 'studio_ready_event_no_%'`, nil, 3)
		s.Close()
	}
}
