package storage

import (
	"context"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
)

// Activities declared by the studio system package: durations are rule-bound,
// the model only names the code.
func studioActivityTestPackage() core.StudioPackageBundle {
	bundle := studioTestPackage("system")
	bundle.Content.SystemRules.Activities = map[string]core.StudioActivityRule{
		"tend_accounts": {DurationMinutes: 30},
		"sweep_court":   {DurationMinutes: 60},
	}
	bundle.Manifest.ContentHash, _ = core.HashJSON(bundle.Content)
	return bundle
}

func prepareStudioActivityWorld(t *testing.T, ctx context.Context, s *Store, world string) StudioWorldSaveRequest {
	t.Helper()
	g := StudioGenesisRequest{PrincipalID: "principal_creator", AuthorityInstanceID: M2DemoInstanceID, AuthorityBranchID: M2DemoBranchID, InstanceID: world, IdempotencyKey: world, Spec: core.StudioWorldSpec{Version: core.StudioWorldSpecVersion, Name: "活动世界", StartWorldTime: core.StudioWorldStart, Population: 2, OpeningMoneyMinor: 10, OpeningStockMinor: 2, Places: []core.StudioWorldPlace{{Key: "home", Name: "家", Kind: "home"}, {Key: "square", Name: "广场", Kind: "public"}}, Links: []core.StudioWorldLink{{From: "home", To: "square", Minutes: 5}}, People: []core.StudioWorldPerson{{Key: "lin", Name: "Lin", Place: "home", Player: true}, {Key: "cai", Name: "Cai", Place: "home"}}}}
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
	if _, err := s.InstallStudioPackage(ctx, StudioPackageInstallRequest{Genesis: g, Binding: b, Bundle: studioActivityTestPackage()}); err != nil {
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
	return StudioWorldSaveRequest{Genesis: g, PlayerPrincipalID: M2RPPlayerPrincipal, Binding: core.CareerBinding{PrincipalID: g.PrincipalID, InstanceID: world, BranchID: "br_main", ExpectedHead: 7, IdempotencyKey: "save"}}
}

func openStudioActivitySession(t *testing.T, ctx context.Context, s *Store, world string) (core.RPSessionReadRequest, string) {
	t.Helper()
	if _, err := s.SaveStudioWorld(ctx, prepareStudioActivityWorld(t, ctx, s, world)); err != nil {
		t.Fatal(err)
	}
	entity, _ := core.StudioWorldObjectID(world, "entity", "lin")
	session, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: M2RPPlayerPrincipal, InstanceID: world, BranchID: "br_main", EntityID: entity, POV: "second_person", IdempotencyKey: world})
	if err != nil {
		t.Fatal(err)
	}
	read := core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID}
	npc, _ := core.StudioWorldObjectID(world, "entity", "cai")
	return read, npc
}

func TestRPActCommitStartsRuleBoundActivityAndEnrichesOwnContext(t *testing.T) {
	ctx := context.Background()
	s := openBootstrappedStore(t, ctx, filepath.Join(t.TempDir(), "rp-act.db"))
	defer func() { s.Close() }()
	setup, err := s.PrepareRPTravel(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ConfigureStudioAccessLocal(ctx, StudioAccessRequest{Purpose: "create_world", Binding: core.CareerBinding{PrincipalID: "principal_operator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: setup.EventSequence, IdempotencyKey: "act-grant"}, TargetPrincipalID: "principal_creator", Status: "active"}); err != nil {
		t.Fatal(err)
	}
	read, npc := openStudioActivitySession(t, ctx, s, "act-world")
	observation, err := s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	var captured core.RPDecisionInput
	provider := rpDecisionProviderFunc(func(ctx context.Context, input core.RPDecisionInput) (core.RPDecisionProposal, error) {
		captured = input
		for _, code := range input.LegalActivities {
			if code == "tend_accounts" {
				return core.RPDecisionProposal{Action: "act", ActivityCode: "tend_accounts"}, nil
			}
		}
		return core.RPDecisionProposal{Action: "respond", Text: "好的。"}, nil
	})
	_, err = s.RunRPTurn(ctx, core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, Text: "你把账理一理。", ExpectedCursor: observation.ObservationCursor, IdempotencyKey: "act-turn"}, provider)
	if err != nil {
		t.Fatal(err)
	}
	// The model declared only the code; duration and lifecycle came from rules.
	assertStudioString(t, ctx, s, `SELECT status FROM rp_activities WHERE actor_id=? AND activity_code='tend_accounts'`, []any{npc}, "in_progress")
	assertM2Value(t, ctx, s, `SELECT duration_minutes FROM rp_activities WHERE actor_id=?`, []any{npc}, 30)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='AgentActivityStarted' AND actor_id=?`, []any{npc}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_own_actions WHERE agent_id=? AND action='activity' AND status='in_progress'`, []any{npc}, 1)
	assertStudioString(t, ctx, s, `SELECT activity_code FROM agent_positions WHERE agent_id=?`, []any{npc}, "tend_accounts")

	// Next turn: the in-progress activity leaves the legal vocabulary, the
	// autobiographical channel and the scene channel both carry it.
	observation, err = s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	introducing := rpDecisionProviderFunc(func(ctx context.Context, input core.RPDecisionInput) (core.RPDecisionProposal, error) {
		captured = input
		return core.RPDecisionProposal{Action: "respond", Text: "我是 Cai，账已理清一半。", IntroduceSelf: true}, nil
	})
	if _, err := s.RunRPTurn(ctx, core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, Text: "辛苦你了。", ExpectedCursor: observation.ObservationCursor, IdempotencyKey: "intro-turn"}, introducing); err != nil {
		t.Fatal(err)
	}
	for _, code := range captured.LegalActivities {
		if code == "tend_accounts" {
			t.Fatal("in-progress activity still legal:", captured.LegalActivities)
		}
	}
	foundOwn, foundScene := false, false
	for _, action := range captured.OwnActions {
		if action.Action == "activity" && action.ActivityCode == "tend_accounts" && action.Status == "in_progress" {
			foundOwn = true
		}
	}
	for _, activity := range captured.SceneActivities {
		if activity.ActorID == npc && activity.ActivityCode == "tend_accounts" && activity.Status == "in_progress" {
			foundScene = true
		}
	}
	if !foundOwn || !foundScene {
		t.Fatalf("own/scene context missing activity: own=%v scene=%v input=%+v", foundOwn, foundScene, captured)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_identity_familiarity WHERE observer_agent_id=? AND subject_agent_id=? AND origin_kind='introduction'`, []any{observation.ControlledEntity.EntityID, npc}, 1)
	if diffs, err := s.CompareProjections(ctx, "act-world", "br_main"); err != nil || len(diffs) != 0 {
		t.Fatal("activity turn projection replay", diffs, err)
	}
}

func TestRPActivitySettleCompletesByRuleAndCancelsOnDeparture(t *testing.T) {
	ctx := context.Background()
	s := openBootstrappedStore(t, ctx, filepath.Join(t.TempDir(), "rp-settle.db"))
	defer func() { s.Close() }()
	setup, err := s.PrepareRPTravel(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ConfigureStudioAccessLocal(ctx, StudioAccessRequest{Purpose: "create_world", Binding: core.CareerBinding{PrincipalID: "principal_operator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: setup.EventSequence, IdempotencyKey: "settle-grant"}, TargetPrincipalID: "principal_creator", Status: "active"}); err != nil {
		t.Fatal(err)
	}
	read, npc := openStudioActivitySession(t, ctx, s, "settle-world")
	observation, err := s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	acting := rpDecisionProviderFunc(func(ctx context.Context, input core.RPDecisionInput) (core.RPDecisionProposal, error) {
		return core.RPDecisionProposal{Action: "act", ActivityCode: "sweep_court"}, nil
	})
	if _, err := s.RunRPTurn(ctx, core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, Text: "扫扫院子。", ExpectedCursor: observation.ObservationCursor, IdempotencyKey: "sweep-turn"}, acting); err != nil {
		t.Fatal(err)
	}
	// Waiting past start+60min lets the deterministic sweeper complete it.
	observation, err = s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	wait, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, TargetWorldTime: "2026-09-22T01:05:00Z", Budget: 8, ExpectedCursor: observation.ObservationCursor, IdempotencyKey: "sweep-wait"})
	if err != nil || wait.Status != "completed" {
		t.Fatal("settle wait", wait, err)
	}
	assertStudioString(t, ctx, s, `SELECT status FROM rp_activities WHERE actor_id=? AND activity_code='sweep_court'`, []any{npc}, "completed")
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='AgentActivityCompleted' AND actor_id=?`, []any{npc}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_own_actions WHERE agent_id=? AND action='activity' AND status='completed'`, []any{npc}, 1)

	// A fresh activity is cancelled when the actor leaves the place mid-way.
	observation, err = s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.RunRPTurn(ctx, core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, Text: "再扫一扫。", ExpectedCursor: observation.ObservationCursor, IdempotencyKey: "second-sweep"}, acting); err != nil {
		t.Fatal(err)
	}
	leaving := rpDecisionProviderFunc(func(ctx context.Context, input core.RPDecisionInput) (core.RPDecisionProposal, error) {
		if len(input.ReachablePlaceIDs) == 0 {
			return core.RPDecisionProposal{Action: "silence"}, nil
		}
		return core.RPDecisionProposal{Action: "leave", DestinationPlaceID: input.ReachablePlaceIDs[0]}, nil
	})
	observation, err = s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.RunRPTurn(ctx, core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, Text: "你去广场看看。", ExpectedCursor: observation.ObservationCursor, IdempotencyKey: "leave-turn"}, leaving); err != nil {
		t.Fatal(err)
	}
	// The sweep she left behind is still in progress; the next sweep cancels it.
	observation, err = s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	wait, err = s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, TargetWorldTime: "2026-09-22T01:10:00Z", Budget: 8, ExpectedCursor: observation.ObservationCursor, IdempotencyKey: "cancel-wait"})
	if err != nil || wait.Status != "completed" {
		t.Fatal("cancellation sweep wait", wait, err)
	}
	assertStudioString(t, ctx, s, `SELECT status FROM rp_activities WHERE actor_id=? AND activity_code='sweep_court' ORDER BY started_world_time DESC LIMIT 1`, []any{npc}, "cancelled")
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='AgentActivityCancelled' AND actor_id=?`, []any{npc}, 1)
	if diffs, err := s.CompareProjections(ctx, "settle-world", "br_main"); err != nil || len(diffs) != 0 {
		t.Fatal("settle projection replay", diffs, err)
	}
}

func TestRPInitiativeWritesDecisionAuditRow(t *testing.T) {
	ctx := context.Background()
	s := openBootstrappedStore(t, ctx, filepath.Join(t.TempDir(), "rp-initiative-audit.db"))
	defer func() { s.Close() }()
	setup, err := s.PrepareRPTravel(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ConfigureStudioAccessLocal(ctx, StudioAccessRequest{Purpose: "create_world", Binding: core.CareerBinding{PrincipalID: "principal_operator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: setup.EventSequence, IdempotencyKey: "initiative-grant"}, TargetPrincipalID: "principal_creator", Status: "active"}); err != nil {
		t.Fatal(err)
	}
	read, npc := openStudioActivitySession(t, ctx, s, "initiative-world")
	initial, err := s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	wait, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, TargetWorldTime: "2026-09-22T00:05:00Z", Budget: 8, ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "initiative-wait"})
	if err != nil || wait.EventID == "" {
		t.Fatal("initiative trigger wait", wait, err)
	}
	responding := rpDecisionProviderFunc(func(ctx context.Context, input core.RPDecisionInput) (core.RPDecisionProposal, error) {
		return core.RPDecisionProposal{Action: "respond", Text: "我是 Cai，你来了。", IntroduceSelf: true}, nil
	})
	initiative, err := s.RunRPInitiative(ctx, core.RPInitiativeRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, NPCEntityID: npc, TriggerEventID: wait.EventID}, responding)
	if err != nil || initiative.EventID == "" {
		t.Fatal("initiative run", initiative, err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_npc_decisions WHERE event_id=? AND action='respond' AND parent_turn_id LIKE 'turn_rp_initiative_%'`, []any{initiative.EventID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_own_actions WHERE agent_id=? AND action='speech'`, []any{npc}, 1)
	entity, _ := core.StudioWorldObjectID("initiative-world", "entity", "lin")
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_identity_familiarity WHERE observer_agent_id=? AND subject_agent_id=? AND origin_kind='introduction'`, []any{entity, npc}, 1)
}
