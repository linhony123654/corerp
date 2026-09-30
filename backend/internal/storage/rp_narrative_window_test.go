package storage

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"corerp.local/backend/internal/core"
)

// The narrative package declares prose labels and the full_prose switch; both
// are presentation metadata carried by the studio narrative package.
func studioNarrativeWindowTestPackage() core.StudioPackageBundle {
	bundle := studioTestPackage("narrative")
	bundle.Content.NarrativeStyle.FullProse = true
	bundle.Content.NarrativeStyle.NarrativeDensity = "long"
	bundle.Content.ActivityLabels = map[string]string{"tend_accounts": "理账", "sweep_court": "扫院子"}
	bundle.Manifest.ContentHash, _ = core.HashJSON(bundle.Content)
	return bundle
}

func prepareNarrativeWindowWorld(t *testing.T, ctx context.Context, s *Store, world string, customize ...func(*core.StudioWorldSpec)) StudioWorldSaveRequest {
	t.Helper()
	g := StudioGenesisRequest{PrincipalID: "principal_creator", AuthorityInstanceID: M2DemoInstanceID, AuthorityBranchID: M2DemoBranchID, InstanceID: world, IdempotencyKey: world, Spec: core.StudioWorldSpec{Version: core.StudioWorldSpecVersion, Name: "叙事世界", StartWorldTime: core.StudioWorldStart, Population: 2, OpeningMoneyMinor: 10, OpeningStockMinor: 2, Places: []core.StudioWorldPlace{{Key: "home", Name: "家", Kind: "home"}, {Key: "square", Name: "广场", Kind: "public"}}, Links: []core.StudioWorldLink{{From: "home", To: "square", Minutes: 5}}, People: []core.StudioWorldPerson{{Key: "lin", Name: "Lin", Place: "home", Player: true}, {Key: "cai", Name: "Cai", Place: "home"}}}}
	if len(customize) > 0 {
		customize[0](&g.Spec)
	}
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
	if _, err := s.InstallStudioPackage(ctx, StudioPackageInstallRequest{Genesis: g, Binding: b, Bundle: studioNarrativeWindowTestPackage()}); err != nil {
		t.Fatal(err)
	}
	b.ExpectedHead = 6
	b.IdempotencyKey = "activate"
	if _, err := s.ActivateStudioPackages(ctx, StudioPackageActivationRequest{Genesis: g, Binding: b, SystemPackageID: "test.system", NarrativePackageID: "test.narrative"}); err != nil {
		t.Fatal(err)
	}
	return StudioWorldSaveRequest{Genesis: g, PlayerPrincipalID: M2RPPlayerPrincipal, Binding: core.CareerBinding{PrincipalID: g.PrincipalID, InstanceID: world, BranchID: "br_main", ExpectedHead: 7, IdempotencyKey: "save"}}
}

// The narrative fact window reaches across turns: an activity the NPC started
// on turn one and the deterministic sweeper completed during a wait must be
// material for turn two's narrative, rendered with the declared prose label.
func TestRPNarrativeWindowIncludesSettledActivityFacts(t *testing.T) {
	ctx := context.Background()
	s := openBootstrappedStore(t, ctx, filepath.Join(t.TempDir(), "rp-narrative-window.db"))
	defer func() { s.Close() }()
	setup, err := s.PrepareRPTravel(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ConfigureStudioAccessLocal(ctx, StudioAccessRequest{Purpose: "create_world", Binding: core.CareerBinding{PrincipalID: "principal_operator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: setup.EventSequence, IdempotencyKey: "window-grant"}, TargetPrincipalID: "principal_creator", Status: "active"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveStudioWorld(ctx, prepareNarrativeWindowWorld(t, ctx, s, "window-world")); err != nil {
		t.Fatal(err)
	}
	entity, _ := core.StudioWorldObjectID("window-world", "entity", "lin")
	session, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: M2RPPlayerPrincipal, InstanceID: "window-world", BranchID: "br_main", EntityID: entity, POV: "second_person", IdempotencyKey: "window-world"})
	if err != nil {
		t.Fatal(err)
	}
	read := core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID}
	observation, err := s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	acting := rpDecisionProviderFunc(func(ctx context.Context, input core.RPDecisionInput) (core.RPDecisionProposal, error) {
		return core.RPDecisionProposal{Action: "act", ActivityCode: "tend_accounts"}, nil
	})
	turn, err := s.RunRPTurn(ctx, core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, Text: "你把账理一理。", ExpectedCursor: observation.ObservationCursor, IdempotencyKey: "window-act-turn"}, acting)
	if err != nil {
		t.Fatal(err)
	}
	// Wait past start+30min: the sweeper completes the activity between turns.
	observation, err = s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	wait, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, TargetWorldTime: "2026-09-22T00:35:00Z", Budget: 8, ExpectedCursor: observation.ObservationCursor, IdempotencyKey: "window-wait"})
	if err != nil || wait.Status != "completed" {
		t.Fatal("window wait", wait, err)
	}
	responding := rpDecisionProviderFunc(func(ctx context.Context, input core.RPDecisionInput) (core.RPDecisionProposal, error) {
		return core.RPDecisionProposal{Action: "respond", Text: "账已经理清了。"}, nil
	})
	observation, err = s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	turn2, err := s.RunRPTurn(ctx, core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, Text: "辛苦你了。", ExpectedCursor: observation.ObservationCursor, IdempotencyKey: "window-second-turn"}, responding)
	if err != nil {
		t.Fatal(err)
	}
	view, err := s.renderRPTurn(ctx, session.SessionID, turn2.PlayerTurnID, turn2.PlayerEventID)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(view, "\n")
	if !strings.Contains(joined, "做完了 理账") {
		t.Fatalf("completed activity missing from narrative window: %q", joined)
	}
	if !strings.Contains(joined, "账已经理清了。") || !strings.Contains(joined, "辛苦你了。") {
		t.Fatalf("turn dialogue missing from narrative: %q", joined)
	}
	doneAt, spokenAt := strings.Index(joined, "做完了 理账"), strings.Index(joined, "辛苦你了。")
	if doneAt < 0 || spokenAt < 0 || doneAt > spokenAt {
		t.Fatalf("window facts must precede the turn dialogue: %q", joined)
	}
	if diffs, err := s.CompareProjections(ctx, "window-world", "br_main"); err != nil || len(diffs) != 0 {
		t.Fatal("narrative window projection replay", diffs, err)
	}
	_ = turn
}

func TestRPNarrativeWindowUsesHistoricalSightForActivity(t *testing.T) {
	ctx := context.Background()
	s := openBootstrappedStore(t, ctx, filepath.Join(t.TempDir(), "rp-hidden-activity.db"))
	defer s.Close()
	setup, err := s.PrepareRPTravel(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ConfigureStudioAccessLocal(ctx, StudioAccessRequest{Purpose: "create_world", Binding: core.CareerBinding{PrincipalID: "principal_operator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: setup.EventSequence, IdempotencyKey: "hidden-grant"}, TargetPrincipalID: "principal_creator", Status: "active"}); err != nil {
		t.Fatal(err)
	}
	const world = "hidden-activity-world"
	if _, err := s.SaveStudioWorld(ctx, prepareNarrativeWindowWorld(t, ctx, s, world)); err != nil {
		t.Fatal(err)
	}
	playerID, _ := core.StudioWorldObjectID(world, "entity", "lin")
	npcID, _ := core.StudioWorldObjectID(world, "entity", "cai")
	placeID, _ := core.StudioWorldObjectID(world, "place", "home")
	session, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: M2RPPlayerPrincipal, InstanceID: world, BranchID: "br_main", EntityID: playerID, POV: "second_person", IdempotencyKey: world})
	if err != nil {
		t.Fatal(err)
	}
	read := core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID}
	view, err := s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	acting := rpDecisionProviderFunc(func(context.Context, core.RPDecisionInput) (core.RPDecisionProposal, error) {
		return core.RPDecisionProposal{Action: "act", ActivityCode: "tend_accounts"}, nil
	})
	if _, err := s.RunRPTurn(ctx, core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, Text: "你先理账。", ExpectedCursor: view.ObservationCursor, IdempotencyKey: "hidden-first"}, acting); err != nil {
		t.Fatal(err)
	}
	binding := func(key string) core.CareerBinding {
		var head int64
		if err := s.db.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id='br_main'`, world).Scan(&head); err != nil {
			t.Fatal(err)
		}
		return core.CareerBinding{PrincipalID: "principal_creator", InstanceID: world, BranchID: "br_main", ExpectedHead: head, IdempotencyKey: key}
	}
	if _, err := s.DefineRPPerceptionLink(ctx, RPPerceptionLinkRequest{Binding: binding("hidden-door"), PlaceID: placeID, ZoneA: "backroom", ZoneB: "main", BarrierKind: "door", BarrierState: "closed", DistanceM: 1, VisualRangeM: 30, AudioRangeM: 30}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PlaceRPActorInZone(ctx, RPActorZoneRequest{Binding: binding("npc-backroom"), AgentID: npcID, PlaceID: placeID, ZoneKey: "backroom"}); err != nil {
		t.Fatal(err)
	}
	view, err = s.ObserveRPSession(ctx, read)
	if err != nil || len(view.PresentEntities) != 0 {
		t.Fatalf("closed door did not hide NPC: %+v %v", view.PresentEntities, err)
	}
	wait, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: read.PrincipalID, SessionID: session.SessionID, TargetWorldTime: "2026-09-22T00:35:00Z", Budget: 8, ExpectedCursor: view.ObservationCursor, IdempotencyKey: "hidden-wait"})
	if err != nil || wait.Status != "completed" {
		t.Fatalf("activity wait: %+v %v", wait, err)
	}
	var completedEventID string
	if err := s.db.QueryRowContext(ctx, `SELECT end_event_id FROM rp_activities WHERE actor_id=? AND activity_code='tend_accounts'`, npcID).Scan(&completedEventID); err != nil || completedEventID == "" {
		t.Fatalf("activity did not complete behind door: %q %v", completedEventID, err)
	}
	view, err = s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.RunRPTurn(ctx, core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, Text: "还听得到吗？", ExpectedCursor: view.ObservationCursor, IdempotencyKey: "hidden-second"}, acting)
	if err != nil {
		t.Fatal(err)
	}
	assertHidden := func() {
		t.Helper()
		input, err := s.readRPNarrativeInput(ctx, session.SessionID, second.PlayerTurnID, second.PlayerEventID)
		if err != nil {
			t.Fatal(err)
		}
		for _, fact := range input.Facts {
			if fact.EventID == completedEventID || fact.Action == "activity_done" {
				t.Fatalf("unseen activity completion entered narrative: %+v", fact)
			}
		}
	}
	assertHidden()
	if _, err := s.PlaceRPActorInZone(ctx, RPActorZoneRequest{Binding: binding("npc-back-main"), AgentID: npcID, PlaceID: placeID, ZoneKey: "main"}); err != nil {
		t.Fatal(err)
	}
	assertHidden() // A later zone change cannot rewrite a settled turn's view.
	view, err = s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	third, err := s.RunRPTurn(ctx, core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, Text: "你再理一遍。", ExpectedCursor: view.ObservationCursor, IdempotencyKey: "away-third"}, acting)
	if err != nil {
		t.Fatal(err)
	}
	view, err = s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	squareID, _ := core.StudioWorldObjectID(world, "place", "square")
	if _, err := s.MoveRP(ctx, core.RPMoveRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, FromPlaceID: placeID, ToPlaceID: squareID, ExpectedCursor: view.ObservationCursor, IdempotencyKey: "away-from-work"}); err != nil {
		t.Fatal(err)
	}
	view, err = s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	wait, err = s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, TargetWorldTime: "2026-09-22T01:15:00Z", Budget: 8, ExpectedCursor: view.ObservationCursor, IdempotencyKey: "away-wait"})
	if err != nil || wait.Status != "completed" {
		t.Fatalf("activity wait while player away: %+v %v", wait, err)
	}
	var awayCompletion string
	if err := s.db.QueryRowContext(ctx, `SELECT end_event_id FROM rp_activities WHERE actor_id=? AND start_event_id IN (SELECT event_id FROM rp_npc_decisions WHERE parent_turn_id=?)`, npcID, third.PlayerTurnID).Scan(&awayCompletion); err != nil || awayCompletion == "" {
		t.Fatalf("second activity did not complete: %q %v", awayCompletion, err)
	}
	view, err = s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.MoveRP(ctx, core.RPMoveRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, FromPlaceID: squareID, ToPlaceID: placeID, ExpectedCursor: view.ObservationCursor, IdempotencyKey: "return-to-work"}); err != nil {
		t.Fatal(err)
	}
	view, err = s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	quiet := rpDecisionProviderFunc(func(context.Context, core.RPDecisionInput) (core.RPDecisionProposal, error) {
		return core.RPDecisionProposal{Action: "silence"}, nil
	})
	fourth, err := s.RunRPTurn(ctx, core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, Text: "我回来了。", ExpectedCursor: view.ObservationCursor, IdempotencyKey: "away-fourth"}, quiet)
	if err != nil {
		t.Fatal(err)
	}
	input, err := s.readRPNarrativeInput(ctx, session.SessionID, fourth.PlayerTurnID, fourth.PlayerEventID)
	if err != nil {
		t.Fatal(err)
	}
	for _, fact := range input.Facts {
		if fact.EventID == awayCompletion {
			t.Fatalf("activity while player was elsewhere entered narrative after return: %+v", fact)
		}
	}
	if diffs, err := s.CompareProjections(ctx, world, "br_main"); err != nil || len(diffs) != 0 {
		t.Fatalf("historical sight changed replayable projections: %v %v", diffs, err)
	}
}

// Seeing an NPC does not make their normal-voice reply audible. Both the
// current turn and the cross-turn window must use frozen hearing evidence,
// even when the speaker is at the same place and shares this RP session.
func TestRPNarrativeExcludesUnheardNPCSpeech(t *testing.T) {
	ctx := context.Background()
	s := openBootstrappedStore(t, ctx, filepath.Join(t.TempDir(), "rp-unheard-narrative.db"))
	defer s.Close()
	session, read, observation := newRPWaitTestSession(t, ctx, s)
	if _, err := s.DefineRPPerceptionLink(ctx, RPPerceptionLinkRequest{
		Binding: careerTestBinding(t, s, "principal_creator", "distant-passage"), PlaceID: M2AgentCafeID,
		ZoneA: "far", ZoneB: "main", BarrierKind: "open", BarrierState: "open",
		DistanceM: 25, VisualRangeM: 30, AudioRangeM: 30,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PlaceRPActorInZone(ctx, RPActorZoneRequest{
		Binding: careerTestBinding(t, s, "principal_creator", "npc-far-away"), AgentID: M2RPNPCID,
		PlaceID: M2AgentCafeID, ZoneKey: "far",
	}); err != nil {
		t.Fatal(err)
	}
	observation, err := s.ObserveRPSession(ctx, read)
	if err != nil || len(observation.PresentEntities) == 0 {
		t.Fatalf("NPC should remain visible at this distance: %+v %v", observation, err)
	}
	responding := rpDecisionProviderFunc(func(_ context.Context, _ core.RPDecisionInput) (core.RPDecisionProposal, error) {
		return core.RPDecisionProposal{Action: "respond", Text: "远处轻声的答复。"}, nil
	})
	first, err := s.RunRPTurn(ctx, core.RPSpeechRequest{
		PrincipalID: read.PrincipalID, SessionID: session.SessionID, Text: "远处的人，听得到吗？",
		DeliveryChannel: "shout", ExpectedCursor: observation.ObservationCursor, IdempotencyKey: "distant-first",
	}, responding)
	if err != nil {
		t.Fatal(err)
	}
	firstInput, err := s.readRPNarrativeInput(ctx, session.SessionID, first.PlayerTurnID, first.PlayerEventID)
	if err != nil {
		t.Fatal(err)
	}
	for _, fact := range firstInput.Facts {
		if fact.Text == "远处轻声的答复。" {
			t.Fatalf("unheard same-turn NPC reply leaked into narration: %+v", fact)
		}
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM observation_records o JOIN rp_utterances u ON u.event_id=o.source_event_id WHERE u.speech_text=? AND o.observer_agent_id=?`, []any{"远处轻声的答复。", M2RPPlayerID}, 0)
	observation, err = s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	wait, err := s.WaitRP(ctx, core.RPWaitRequest{
		PrincipalID: read.PrincipalID, SessionID: session.SessionID, TargetWorldTime: "2026-09-22T03:00:00Z",
		Budget: 10, ExpectedCursor: observation.ObservationCursor, IdempotencyKey: "distant-wait",
	})
	if err != nil || wait.Status != "completed" {
		t.Fatalf("wait before initiative: %+v %v", wait, err)
	}
	initiative, err := s.RunRPInitiative(ctx, core.RPInitiativeRequest{
		PrincipalID: read.PrincipalID, SessionID: session.SessionID, NPCEntityID: M2RPNPCID, TriggerEventID: wait.EventID,
	}, responding)
	if err != nil || initiative.Action != "respond" {
		t.Fatalf("unheard initiative setup: %+v %v", initiative, err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM observation_records WHERE source_event_id=? AND observer_agent_id=?`, []any{initiative.EventID, M2RPPlayerID}, 0)
	observation, err = s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.RunRPTurn(ctx, core.RPSpeechRequest{
		PrincipalID: read.PrincipalID, SessionID: session.SessionID, Text: "我还是听不清。",
		DeliveryChannel: "shout", ExpectedCursor: observation.ObservationCursor, IdempotencyKey: "distant-second",
	}, responding)
	if err != nil {
		t.Fatal(err)
	}
	secondInput, err := s.readRPNarrativeInput(ctx, session.SessionID, second.PlayerTurnID, second.PlayerEventID)
	if err != nil {
		t.Fatal(err)
	}
	for _, fact := range secondInput.Facts {
		if fact.EventID == initiative.EventID || fact.Text == "远处轻声的答复。" {
			t.Fatalf("unheard cross-turn NPC speech leaked into narration: %+v", fact)
		}
	}
	if diffs, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diffs) != 0 {
		t.Fatalf("unheard speech changed replayable projections: %v %v", diffs, err)
	}
}

func TestRPNarrativeExcludesUnseenNPCSilence(t *testing.T) {
	ctx := context.Background()
	s := openBootstrappedStore(t, ctx, filepath.Join(t.TempDir(), "rp-unseen-silence.db"))
	defer s.Close()
	session, read, _ := newRPWaitTestSession(t, ctx, s)
	if _, err := s.DefineRPPerceptionLink(ctx, RPPerceptionLinkRequest{
		Binding: careerTestBinding(t, s, "principal_creator", "audible-not-visible"), PlaceID: M2AgentCafeID,
		ZoneA: "far", ZoneB: "main", BarrierKind: "open", BarrierState: "open",
		DistanceM: 25, VisualRangeM: 0, AudioRangeM: 30,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PlaceRPActorInZone(ctx, RPActorZoneRequest{
		Binding: careerTestBinding(t, s, "principal_creator", "unseen-npc"), AgentID: M2RPNPCID,
		PlaceID: M2AgentCafeID, ZoneKey: "far",
	}); err != nil {
		t.Fatal(err)
	}
	view, err := s.ObserveRPSession(ctx, read)
	if err != nil || len(view.PresentEntities) != 0 {
		t.Fatalf("NPC should be hidden but in shout range: %+v %v", view.PresentEntities, err)
	}
	quiet := rpDecisionProviderFunc(func(context.Context, core.RPDecisionInput) (core.RPDecisionProposal, error) {
		return core.RPDecisionProposal{Action: "silence"}, nil
	})
	turn, err := s.RunRPTurn(ctx, core.RPSpeechRequest{
		PrincipalID: read.PrincipalID, SessionID: session.SessionID, Text: "你听得见吗？",
		DeliveryChannel: "shout", ExpectedCursor: view.ObservationCursor, IdempotencyKey: "unseen-silence",
	}, quiet)
	if err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_npc_decisions WHERE session_id=? AND parent_turn_id=? AND action='silence'`, []any{session.SessionID, turn.PlayerTurnID}, 1)
	input, err := s.readRPNarrativeInput(ctx, session.SessionID, turn.PlayerTurnID, turn.PlayerEventID)
	if err != nil {
		t.Fatal(err)
	}
	for _, fact := range input.Facts {
		if fact.ActorID == M2RPNPCID || fact.Action == "silence" {
			t.Fatalf("private/unseen silence entered narrative: %+v", fact)
		}
	}
}

func TestRPNarrativeUsesOnlyAuthoredPublicPresentationAfterRecognition(t *testing.T) {
	ctx := context.Background()
	s := openBootstrappedStore(t, ctx, filepath.Join(t.TempDir(), "rp-public-presentation.db"))
	defer s.Close()
	setup, err := s.PrepareRPTravel(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ConfigureStudioAccessLocal(ctx, StudioAccessRequest{Purpose: "create_world", Binding: core.CareerBinding{PrincipalID: "principal_operator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: setup.EventSequence, IdempotencyKey: "presentation-grant"}, TargetPrincipalID: "principal_creator", Status: "active"}); err != nil {
		t.Fatal(err)
	}
	const world = "public-presentation-world"
	const privatePersona = "PRIVATE-CAI: 这件往事只有她本人知道。"
	const publicStyle = "措辞温和，习惯用短句。"
	if _, err := s.SaveStudioWorld(ctx, prepareNarrativeWindowWorld(t, ctx, s, world, func(spec *core.StudioWorldSpec) {
		spec.People[1].Persona = privatePersona
		spec.People[1].PublicPresentation = publicStyle
	})); err != nil {
		t.Fatal(err)
	}
	playerID, _ := core.StudioWorldObjectID(world, "entity", "lin")
	npcID, _ := core.StudioWorldObjectID(world, "entity", "cai")
	session, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: M2RPPlayerPrincipal, InstanceID: world, BranchID: "br_main", EntityID: playerID, POV: "second_person", IdempotencyKey: world})
	if err != nil {
		t.Fatal(err)
	}
	read := core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID}
	view, err := s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	firstProvider := rpDecisionProviderFunc(func(_ context.Context, input core.RPDecisionInput) (core.RPDecisionProposal, error) {
		if input.Persona != privatePersona {
			t.Fatalf("NPC decision lost its private persona: %+v", input)
		}
		return core.RPDecisionProposal{Action: "respond", Text: "稍等。"}, nil
	})
	first, err := s.RunRPTurn(ctx, core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, Text: "你好。", ExpectedCursor: view.ObservationCursor, IdempotencyKey: "presentation-unknown"}, firstProvider)
	if err != nil {
		t.Fatal(err)
	}
	unknown, err := s.readRPNarrativeInput(ctx, session.SessionID, first.PlayerTurnID, first.PlayerEventID)
	if err != nil || len(unknown.PublicPresentations) != 0 {
		t.Fatalf("unrecognized NPC presentation leaked: %+v %v", unknown.PublicPresentations, err)
	}
	view, err = s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	introducing := rpDecisionProviderFunc(func(context.Context, core.RPDecisionInput) (core.RPDecisionProposal, error) {
		return core.RPDecisionProposal{Action: "respond", Text: "我叫Cai。", IntroduceSelf: true}, nil
	})
	second, err := s.RunRPTurn(ctx, core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, Text: "你叫什么？", ExpectedCursor: view.ObservationCursor, IdempotencyKey: "presentation-known"}, introducing)
	if err != nil {
		t.Fatal(err)
	}
	known, err := s.readRPNarrativeInput(ctx, session.SessionID, second.PlayerTurnID, second.PlayerEventID)
	if err != nil {
		t.Fatal(err)
	}
	if len(known.PublicPresentations) != 1 || known.PublicPresentations[0].ActorID != npcID || known.PublicPresentations[0].Text != publicStyle {
		t.Fatalf("authored public style missing after recognition: %+v", known.PublicPresentations)
	}
	var sourceEventID string
	if err := s.db.QueryRowContext(ctx, `SELECT event_id FROM events WHERE instance_id=? AND event_type='StudioWorldPrepared'`, world).Scan(&sourceEventID); err != nil || known.PublicPresentations[0].SourceEventID != sourceEventID {
		t.Fatalf("public style provenance missing: %+v %v", known.PublicPresentations, err)
	}
	encoded, err := json.Marshal(known)
	if err != nil || strings.Contains(string(encoded), privatePersona) || !strings.Contains(string(encoded), publicStyle) {
		t.Fatalf("private persona crossed narrator boundary: %v %v", string(encoded), err)
	}
	known.Style = core.DefaultRPStyle()
	if err := known.ValidateReadBudget(); err != nil {
		t.Fatal(err)
	}
	// Persist a presentation variant through the same source validator used by
	// the service. Fact IDs stay one-to-one with the view; the authored cue is
	// recorded as context provenance in the saved render only.
	ids := make([]string, 0, len(known.Facts))
	for _, fact := range known.Facts {
		ids = append(ids, fact.EventID)
	}
	variant := core.RPNarrativeView{Lines: []string{"Cai答了话。"}, EventIDs: ids}
	request := RPNarrativeReadRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, TurnRunID: second.TurnRunID}
	if err := s.saveSelectedRPNarrative(ctx, request, known, &variant, core.RPProviderMetadata{Kind: "test"}); err != nil || variant.RenderID == "" {
		t.Fatalf("public presentation render could not be saved: %+v %v", variant, err)
	}
	var sourceJSON string
	if err := s.db.QueryRowContext(ctx, `SELECT source_event_ids_json FROM rp_narrative_renders WHERE render_id=?`, variant.RenderID).Scan(&sourceJSON); err != nil {
		t.Fatal(err)
	}
	var storedSources []string
	if err := json.Unmarshal([]byte(sourceJSON), &storedSources); err != nil || len(storedSources) != len(ids)+1 || storedSources[len(ids)] != sourceEventID {
		t.Fatalf("authored context provenance missing from saved render: %q %v", sourceJSON, err)
	}
	variant.EventIDs[0] = "fabricated-fact"
	if err := s.saveSelectedRPNarrative(ctx, request, known, &variant, core.RPProviderMetadata{Kind: "test"}); !core.HasCode(err, core.CodeProjectionDiverged) {
		t.Fatalf("fabricated render fact accepted: %v", err)
	}
	if diffs, err := s.CompareProjections(ctx, world, "br_main"); err != nil || len(diffs) != 0 {
		t.Fatalf("public style changed world projections: %v %v", diffs, err)
	}
}

// A world that declares full_prose must not silently settle for the
// deterministic renderer: the read works, the reason is queryable on the turn
// run, and it carries a summary of the input facts.
func TestRPNarrativeFullProseFallbackIsQueryable(t *testing.T) {
	ctx := context.Background()
	s := openBootstrappedStore(t, ctx, filepath.Join(t.TempDir(), "rp-narrative-fallback.db"))
	defer func() { s.Close() }()
	setup, err := s.PrepareRPTravel(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ConfigureStudioAccessLocal(ctx, StudioAccessRequest{Purpose: "create_world", Binding: core.CareerBinding{PrincipalID: "principal_operator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: setup.EventSequence, IdempotencyKey: "fallback-grant"}, TargetPrincipalID: "principal_creator", Status: "active"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveStudioWorld(ctx, prepareNarrativeWindowWorld(t, ctx, s, "fallback-world")); err != nil {
		t.Fatal(err)
	}
	entity, _ := core.StudioWorldObjectID("fallback-world", "entity", "lin")
	session, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: M2RPPlayerPrincipal, InstanceID: "fallback-world", BranchID: "br_main", EntityID: entity, POV: "second_person", IdempotencyKey: "fallback-world"})
	if err != nil {
		t.Fatal(err)
	}
	read := core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID}
	observation, err := s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	responding := rpDecisionProviderFunc(func(ctx context.Context, input core.RPDecisionInput) (core.RPDecisionProposal, error) {
		return core.RPDecisionProposal{Action: "respond", Text: "我在。"}, nil
	})
	turn, err := s.RunRPTurn(ctx, core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, Text: "在吗？", ExpectedCursor: observation.ObservationCursor, IdempotencyKey: "fallback-turn"}, responding)
	if err != nil {
		t.Fatal(err)
	}
	result, err := s.StreamRPNarrative(ctx, RPNarrativeReadRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, TurnRunID: turn.TurnRunID}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.View.FallbackReason != "world_declares_full_prose_without_prose_provider" {
		t.Fatalf("fallback reason not surfaced: %+v", result.View)
	}
	calls, err := readRPTurnProviderCalls(ctx, s.db, read.SessionID, turn.TurnRunID)
	if err != nil || len(calls) != 3 || calls[1].RenderSource != "template" || calls[2].Phase != "narrative" || calls[2].Attempted || calls[2].Result != "success" || calls[2].FallbackKind != result.View.FallbackReason || calls[2].RenderSource != "template" {
		t.Fatalf("narrative provenance missing: %+v %v", calls, err)
	}
	var raw string
	if err := s.db.QueryRowContext(ctx, `SELECT narrative_fallback FROM rp_turn_runs WHERE turn_run_id=?`, turn.TurnRunID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var record struct {
		Reason       string   `json:"reason"`
		ProviderMode string   `json:"provider_mode"`
		FactCount    int      `json:"fact_count"`
		FactEvents   []string `json:"fact_events"`
	}
	if err := json.Unmarshal([]byte(raw), &record); err != nil {
		t.Fatal(err)
	}
	if record.Reason != "world_declares_full_prose_without_prose_provider" || record.ProviderMode != "deterministic" || record.FactCount == 0 || len(record.FactEvents) == 0 {
		t.Fatalf("queryable fallback record incomplete: %q", raw)
	}
	if diffs, err := s.CompareProjections(ctx, "fallback-world", "br_main"); err != nil || len(diffs) != 0 {
		t.Fatal("fallback record projection replay", diffs, err)
	}
}
