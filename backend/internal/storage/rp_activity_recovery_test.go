package storage

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
)

func openConcurrentActivitySession(t *testing.T, ctx context.Context, s *Store, world string) core.RPSessionReadRequest {
	t.Helper()
	setup, err := s.PrepareRPTravel(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ConfigureStudioAccessLocal(ctx, StudioAccessRequest{
		Purpose:           "create_world",
		Binding:           core.CareerBinding{PrincipalID: "principal_operator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: setup.EventSequence, IdempotencyKey: world + "-grant"},
		TargetPrincipalID: "principal_creator", Status: "active",
	}); err != nil {
		t.Fatal(err)
	}
	genesis := StudioGenesisRequest{
		PrincipalID: "principal_creator", AuthorityInstanceID: M2DemoInstanceID, AuthorityBranchID: M2DemoBranchID, InstanceID: world, IdempotencyKey: world,
		Spec: core.StudioWorldSpec{
			Version: core.StudioWorldSpecVersion, Name: "多活动世界", StartWorldTime: core.StudioWorldStart,
			Population: 3, OpeningMoneyMinor: 10, OpeningStockMinor: 2,
			Places: []core.StudioWorldPlace{{Key: "home", Name: "家", Kind: "home"}, {Key: "square", Name: "广场", Kind: "public"}},
			Links:  []core.StudioWorldLink{{From: "home", To: "square", Minutes: 5}},
			People: []core.StudioWorldPerson{{Key: "lin", Name: "Lin", Place: "home", Player: true}, {Key: "cai", Name: "Cai", Place: "home"}, {Key: "mei", Name: "Mei", Place: "home"}},
		},
	}
	if _, err := s.PrepareStudioWorld(ctx, genesis); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PrepareStudioParticipants(ctx, genesis); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PrepareStudioSpatial(ctx, genesis); err != nil {
		t.Fatal(err)
	}
	binding := core.CareerBinding{PrincipalID: genesis.PrincipalID, InstanceID: world, BranchID: "br_main"}
	setBinding := func(key string) core.CareerBinding {
		if err := s.db.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id='br_main'`, world).Scan(&binding.ExpectedHead); err != nil {
			t.Fatal(err)
		}
		binding.IdempotencyKey = key
		return binding
	}
	if _, err := s.InstallStudioPackage(ctx, StudioPackageInstallRequest{Genesis: genesis, Binding: setBinding("system"), Bundle: studioActivityTestPackage()}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.InstallStudioPackage(ctx, StudioPackageInstallRequest{Genesis: genesis, Binding: setBinding("narrative"), Bundle: studioTestPackage("narrative")}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ActivateStudioPackages(ctx, StudioPackageActivationRequest{Genesis: genesis, Binding: setBinding("activate"), SystemPackageID: "test.system", NarrativePackageID: "test.narrative"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveStudioWorld(ctx, StudioWorldSaveRequest{Genesis: genesis, PlayerPrincipalID: M2RPPlayerPrincipal, Binding: setBinding("save")}); err != nil {
		t.Fatal(err)
	}
	player, _ := core.StudioWorldObjectID(world, "entity", "lin")
	session, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: M2RPPlayerPrincipal, InstanceID: world, BranchID: "br_main", EntityID: player, POV: "second_person", IdempotencyKey: world + "-session"})
	if err != nil {
		t.Fatal(err)
	}
	return core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID}
}

func startConcurrentRPActivities(t *testing.T, ctx context.Context, s *Store, read core.RPSessionReadRequest) {
	t.Helper()
	view, err := s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	provider := rpDecisionProviderFunc(func(context.Context, core.RPDecisionInput) (core.RPDecisionProposal, error) {
		calls++
		return core.RPDecisionProposal{Action: "act", ActivityCode: "sweep_court"}, nil
	})
	if _, err := s.RunRPTurn(ctx, core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, Text: "一起扫院子。", IdempotencyKey: "start-concurrent"}, provider); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("expected two independent NPC activities, got %d", calls)
	}
}

func TestRPActivitySettleUsesBatchIndicesForConcurrentTerminations(t *testing.T) {
	ctx := context.Background()
	s := openBootstrappedStore(t, ctx, filepath.Join(t.TempDir(), "concurrent-activity.db"))
	defer s.Close()
	world := "concurrent-activity-world"
	read := openConcurrentActivitySession(t, ctx, s, world)
	startConcurrentRPActivities(t, ctx, s, read)
	view, err := s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	request := core.RPWaitRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: "2026-09-22T01:05:00Z", Budget: 8, IdempotencyKey: "settle-concurrent"}
	result, err := s.WaitRP(ctx, request)
	if err != nil || result.Status != "completed" {
		t.Fatalf("concurrent settle did not finish: %+v %v", result, err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_activities WHERE instance_id=? AND status='completed'`, []any{world}, 2)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE instance_id=? AND event_type='AgentActivityCompleted' AND batch_index IN (0,1)`, []any{world}, 2)
	assertM2Value(t, ctx, s, `SELECT COUNT(DISTINCT batch_id) FROM events WHERE instance_id=? AND event_type='AgentActivityCompleted'`, []any{world}, 1)
	replay, err := s.WaitRP(ctx, request)
	if err != nil || !replay.Replayed {
		t.Fatalf("committed wait did not replay: %+v %v", replay, err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE instance_id=? AND event_type='AgentActivityCompleted'`, []any{world}, 2)
	if diffs, err := s.CompareProjections(ctx, world, "br_main"); err != nil || len(diffs) != 0 {
		t.Fatalf("concurrent activity events are not replayable: %v %v", diffs, err)
	}
}

func TestRPWaitResumesInterruptedDerivedActivitySettle(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "wait-derived-recovery.db")
	s := openBootstrappedStore(t, ctx, path)
	world := "wait-derived-recovery-world"
	read := openConcurrentActivitySession(t, ctx, s, world)
	startConcurrentRPActivities(t, ctx, s, read)
	view, err := s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	request := core.RPWaitRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: "2026-09-22T01:05:00Z", Budget: 8, IdempotencyKey: "recover-derived-settle"}
	hash, err := core.HashJSON(request)
	if err != nil {
		t.Fatal(err)
	}
	intent, replayed, err := s.ensureRPWaitIntent(ctx, request, hash)
	if err != nil || replayed {
		t.Fatalf("new wait intent: %+v replayed=%v err=%v", intent, replayed, err)
	}
	run, err := s.runAgentLifeForScope(ctx, world, "br_main", request.TargetWorldTime, request.Budget)
	if err != nil || run.PendingDue != 0 {
		t.Fatalf("scheduler before committed wait: %+v %v", run, err)
	}
	committed, err := s.finishRPWait(ctx, request, hash, intent.IntentID, run.ProcessedItems)
	if err != nil || committed.Status != "completed" {
		t.Fatalf("wait did not commit before interrupted sweep: %+v %v", committed, err)
	}
	assertStudioString(t, ctx, s, `SELECT status FROM rp_wait_activity_settlements WHERE intent_id=?`, []any{intent.IntentID}, "pending")
	s.beforeCommit = func() error { return errors.New("injected activity sweep interruption") }
	if err := s.settleCommittedRPWaitActivities(ctx, intent.IntentID); err == nil {
		t.Fatal("derived settle did not exercise crash boundary")
	}
	s.beforeCommit = nil
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE instance_id=? AND event_type='AgentActivityCompleted'`, []any{world}, 0)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	replayedWait, err := s.WaitRP(ctx, request)
	if err != nil || !replayedWait.Replayed || replayedWait.EventID != committed.EventID {
		t.Fatalf("committed wait did not recover its derived settlement: %+v %v", replayedWait, err)
	}
	assertStudioString(t, ctx, s, `SELECT status FROM rp_wait_activity_settlements WHERE intent_id=?`, []any{intent.IntentID}, "complete")
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_activities WHERE instance_id=? AND status='completed'`, []any{world}, 2)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE instance_id=? AND event_type='RPWaitCompleted'`, []any{world}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE instance_id=? AND event_type='AgentActivityCompleted'`, []any{world}, 2)
	if diffs, err := s.CompareProjections(ctx, world, "br_main"); err != nil || len(diffs) != 0 {
		t.Fatalf("recovered wait projections diverged: %v %v", diffs, err)
	}
}

func TestRPTurnActivitySweepNeverStrandsFreshIntent(t *testing.T) {
	ctx := context.Background()
	s := openBootstrappedStore(t, ctx, filepath.Join(t.TempDir(), "turn-sweep.db"))
	defer s.Close()
	world := "turn-sweep-world"
	read := openConcurrentActivitySession(t, ctx, s, world)
	startConcurrentRPActivities(t, ctx, s, read)
	view, err := s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	waitRequest := core.RPWaitRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: "2026-09-22T01:05:00Z", Budget: 8, IdempotencyKey: "turn-sweep-wait"}
	hash, err := core.HashJSON(waitRequest)
	if err != nil {
		t.Fatal(err)
	}
	intent, _, err := s.ensureRPWaitIntent(ctx, waitRequest, hash)
	if err != nil {
		t.Fatal(err)
	}
	run, err := s.runAgentLifeForScope(ctx, world, "br_main", waitRequest.TargetWorldTime, waitRequest.Budget)
	if err != nil || run.PendingDue != 0 {
		t.Fatalf("world-time advance: %+v %v", run, err)
	}
	if _, err := s.finishRPWait(ctx, waitRequest, hash, intent.IntentID, run.ProcessedItems); err != nil {
		t.Fatal(err)
	}
	view, err = s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	request := core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, Text: "活动结束以后再聊。", IdempotencyKey: "turn-after-sweep"}
	if _, err := s.RunRPTurn(ctx, request, core.DeterministicRPDecisionProvider{}); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatalf("unseen sweep should require fresh observation: %v", err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_turn_runs WHERE session_id=? AND idempotency_key=?`, []any{read.SessionID, request.IdempotencyKey}, 0)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_activities WHERE instance_id=? AND status='completed'`, []any{world}, 2)
	view, err = s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	request.ExpectedCursor = view.ObservationCursor
	turn, err := s.RunRPTurn(ctx, request, core.DeterministicRPDecisionProvider{})
	if err != nil || turn.Status != "settled" {
		t.Fatalf("same turn key could not recover after fresh observation: %+v %v", turn, err)
	}
	if _, err := s.WaitRP(ctx, waitRequest); err != nil {
		t.Fatalf("committed wait's derived marker did not recover: %v", err)
	}
	assertStudioString(t, ctx, s, `SELECT status FROM rp_wait_activity_settlements WHERE intent_id=?`, []any{intent.IntentID}, "complete")
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE instance_id=? AND event_type='AgentActivityCompleted'`, []any{world}, 2)
	if diffs, err := s.CompareProjections(ctx, world, "br_main"); err != nil || len(diffs) != 0 {
		t.Fatalf("turn after activity sweep diverged: %v %v", diffs, err)
	}
}

func TestRPExplicitRetryRecoversLegacyOpenTurnAfterActivitySweep(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "legacy-open-turn.db")
	s := openBootstrappedStore(t, ctx, path)
	world := "legacy-open-turn-world"
	read := openConcurrentActivitySession(t, ctx, s, world)
	startConcurrentRPActivities(t, ctx, s, read)
	view, err := s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	wait := core.RPWaitRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: "2026-09-22T01:05:00Z", Budget: 8, IdempotencyKey: "legacy-turn-wait"}
	hash, err := core.HashJSON(wait)
	if err != nil {
		t.Fatal(err)
	}
	intent, _, err := s.ensureRPWaitIntent(ctx, wait, hash)
	if err != nil {
		t.Fatal(err)
	}
	life, err := s.runAgentLifeForScope(ctx, world, "br_main", wait.TargetWorldTime, wait.Budget)
	if err != nil || life.PendingDue != 0 {
		t.Fatalf("wait scheduler: %+v %v", life, err)
	}
	if _, err := s.finishRPWait(ctx, wait, hash, intent.IntentID, life.ProcessedItems); err != nil {
		t.Fatal(err)
	}
	view, err = s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	request := core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, Text: "我刚才说的话。", IdempotencyKey: "legacy-open-sweep"}
	request.SpeechAct = "statement"
	run, existing, err := s.ensureRPTurnRun(ctx, request)
	if err != nil || existing || run.Status != "open" {
		t.Fatalf("create legacy open intent: %+v existing=%v err=%v", run, existing, err)
	}
	if err := s.settleRPActivities(ctx, world, "br_main"); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.RunRPTurn(ctx, request, core.DeterministicRPDecisionProvider{}); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatalf("stale original retry should not silently rebind: %v", err)
	}
	fresh, err := s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	request.ExpectedCursor = fresh.ObservationCursor
	changedText := request
	changedText.Text = "偷换发言内容"
	if _, err := s.RunRPTurn(ctx, changedText, core.DeterministicRPDecisionProvider{}); !core.HasCode(err, core.CodeIdempotencyMismatch) {
		t.Fatalf("rebase changed accepted text: %v", err)
	}
	turn, err := s.RunRPTurn(ctx, request, core.DeterministicRPDecisionProvider{})
	if err != nil || turn.Status != "settled" || turn.TurnRunID != run.ID {
		t.Fatalf("explicit fresh retry failed to recover original turn: %+v %v", turn, err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_turn_explicit_rebases WHERE turn_run_id=? AND old_cursor=? AND new_cursor=? AND reason='explicit_current_observation'`, []any{run.ID, view.ObservationCursor, fresh.ObservationCursor}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_utterances WHERE session_id=? AND speech_text=?`, []any{read.SessionID, request.Text}, 1)
	replay, err := s.RunRPTurn(ctx, request, core.DeterministicRPDecisionProvider{})
	if err != nil || !replay.Replayed || replay.PlayerEventID != turn.PlayerEventID {
		t.Fatalf("rebound turn retry duplicated speech: %+v %v", replay, err)
	}
	if diffs, err := s.CompareProjections(ctx, world, "br_main"); err != nil || len(diffs) != 0 {
		t.Fatalf("explicit turn rebase changed world projections: %v %v", diffs, err)
	}
}
