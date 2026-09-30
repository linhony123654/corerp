package storage

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"corerp.local/backend/internal/core"
)

type rpFocusFixture struct {
	s       *Store
	path    string
	world   string
	player  string
	session RPSession
	read    core.RPSessionReadRequest
	ids     []string
	names   map[string]string
}

func TestRPConversationFocusNameCuesRespectLatinWordBoundaries(t *testing.T) {
	for _, test := range []struct {
		name, text string
		want       bool
	}{
		{"Bo", "I opened a book.", false},
		{"Ann", "Anna will be here.", false},
		{"Ada", "I have a database question.", false},
		{"Bo", "Hello, BO.", true},
		{"Ann", "@Ann: are you there?", true},
		{"Ada", "Ada，说句话吧。", true},
		{"Bo", "请Bo接着说。", true},
		{"Bo", "bo_id is not a name cue", false},
		{"Bo", "a book; Bo, now speak.", true},
		{"老祖宗", "老祖宗，您说呢？", true},
	} {
		if got := rpSpeechAddressesName(test.text, test.name); got != test.want {
			t.Errorf("%q in %q: got %v want %v", test.name, test.text, got, test.want)
		}
	}
}

func newRPFocusFixture(t *testing.T, mode string, familiar ...bool) *rpFocusFixture {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "focus.db")
	s := openBootstrappedStore(t, ctx, path)
	setup, err := s.PrepareRPTravel(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ConfigureStudioAccessLocal(ctx, StudioAccessRequest{Purpose: "create_world", Binding: core.CareerBinding{PrincipalID: "principal_operator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: setup.EventSequence, IdempotencyKey: "focus-grant"}, TargetPrincipalID: "principal_creator", Status: "active"}); err != nil {
		t.Fatal(err)
	}
	const world = "conversation-focus-world"
	create := studioCreateFixture(world)
	create.Spec.Population, create.Spec.OpeningStockMinor = 4, 4
	create.Spec.People = []core.StudioWorldPerson{{Key: "lin", Name: "Lin", Place: "home", Player: true}, {Key: "ada", Name: "Ada", Place: "home"}, {Key: "bo", Name: "Bo", Place: "home"}, {Key: "cai", Name: "Cai", Place: "home"}}
	create.Spec.Acquaintances = [][2]string{{"lin", "ada"}, {"lin", "bo"}, {"lin", "cai"}}
	if len(familiar) > 0 && !familiar[0] {
		create.Spec.Acquaintances = nil
	}
	create.SystemPackage.Content.SystemRules.RPExecutionMode = mode
	create.SystemPackage.Content.SystemRules.MaxActiveResponders = 1
	if mode == "legacy" {
		create.SystemPackage.Content.SystemRules.RPExecutionMode = ""
		create.SystemPackage.Content.SystemRules.MaxActiveResponders = 0
	}
	create.SystemPackage.Manifest.ContentHash, _ = core.HashJSON(create.SystemPackage.Content)
	if _, err := s.CreateStudioWorld(ctx, create); err != nil {
		t.Fatal(err)
	}
	player, _ := core.StudioWorldObjectID(world, "entity", "lin")
	session, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: M2RPPlayerPrincipal, InstanceID: world, BranchID: "br_main", EntityID: player, POV: "second_person", IdempotencyKey: "focus-session"})
	if err != nil {
		t.Fatal(err)
	}
	f := rpFocusFixture{s: s, path: path, world: world, player: player, session: session, read: core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID}, names: map[string]string{}}
	for _, key := range []string{"ada", "bo", "cai"} {
		id, _ := core.StudioWorldObjectID(world, "entity", key)
		f.ids = append(f.ids, id)
		f.names[id] = map[string]string{"ada": "Ada", "bo": "Bo", "cai": "Cai"}[key]
	}
	sort.Strings(f.ids)
	t.Cleanup(func() { _ = f.s.Close() })
	return &f
}

func TestRPConversationFocusUnknownIdentityUsesHeardExchangeWithoutLeakingNames(t *testing.T) {
	f := newRPFocusFixture(t, "legacy", false)
	partner := f.ids[len(f.ids)-1]
	f.turn(t, "unknown-greeting", "有人愿意听我说吗？", partner)
	result, calls := f.turn(t, "unknown-follow", "那我继续。", partner)
	if calls[0] != partner {
		t.Fatal("anonymous but personally heard speaker was lost", calls)
	}
	view, err := f.s.ReadRPObservatory(context.Background(), RPObservatoryRequest{PrincipalID: f.read.PrincipalID, SessionID: f.read.SessionID})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), partner) || strings.Contains(string(raw), f.names[partner]) {
		t.Fatal("continuation disclosed a stranger's canonical identity")
	}
	assertM2Value(t, context.Background(), f.s, `SELECT COUNT(*) FROM rp_turn_listener_activations WHERE turn_run_id=? AND reason_code='conversation_continuation'`, []any{result.TurnRunID}, 1)
}

func TestRPConversationFocusSurvivesNewClientRebuildAndInterruptedTurn(t *testing.T) {
	f := newRPFocusFixture(t, "orchestrated")
	ctx := context.Background()
	partner := f.ids[len(f.ids)-1]
	f.turn(t, "before-restart", f.names[partner]+"，我想聊聊。", partner)
	second, err := f.s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: f.read.PrincipalID, InstanceID: f.world, BranchID: "br_main", EntityID: f.player, POV: "second_person", IdempotencyKey: "focus-other-client"})
	if err != nil {
		t.Fatal(err)
	}
	f.read.SessionID = second.SessionID
	if err := f.s.RebuildProjections(ctx, f.world, "br_main"); err != nil {
		t.Fatal(err)
	}
	view, err := f.s.ObserveRPSession(ctx, f.read)
	if err != nil {
		t.Fatal(err)
	}
	req := core.RPSpeechRequest{PrincipalID: f.read.PrincipalID, SessionID: f.read.SessionID, ExpectedCursor: view.ObservationCursor, Text: "你刚才那句话，我还想听你说下去。", IdempotencyKey: "focus-interrupted"}
	calls := 0
	provider := rpDecisionProviderFunc(func(_ context.Context, in core.RPDecisionInput) (core.RPDecisionProposal, error) {
		calls++
		if in.NPCEntityID != partner {
			t.Error("new client changed partner")
		}
		return core.RPDecisionProposal{Action: "respond", Text: "好，我仍在听。"}, nil
	})
	f.s.afterRPTurnStage = func(stage string) error {
		if stage == "npc_effect_committed" {
			return core.NewError(core.CodeInjectedFailure, "focus restart boundary")
		}
		return nil
	}
	if _, err := f.s.RunRPTurn(ctx, req, provider); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatal(err)
	}
	var source string
	if err := f.s.db.QueryRowContext(ctx, `SELECT conversation_source_event_id FROM rp_turn_listener_activations a JOIN rp_turn_runs r ON r.turn_run_id=a.turn_run_id WHERE r.session_id=? AND r.idempotency_key=? AND a.reason_code='conversation_continuation'`, req.SessionID, req.IdempotencyKey).Scan(&source); err != nil {
		t.Fatal(err)
	}
	if err := f.s.Close(); err != nil {
		t.Fatal(err)
	}
	f.s, err = Open(ctx, f.path)
	if err != nil {
		t.Fatal(err)
	}
	result, err := f.s.RunRPTurn(ctx, req, provider)
	if err != nil || result.Status != "settled" || calls != 1 {
		t.Fatal("restart reselected or repeated the NPC", result, err, calls)
	}
	assertM2Value(t, ctx, f.s, `SELECT COUNT(*) FROM rp_turn_listener_activations WHERE turn_run_id=? AND npc_entity_id=? AND reason_code='conversation_continuation' AND conversation_source_event_id=?`, []any{result.TurnRunID, partner, source}, 1)
	if differences, err := f.s.CompareProjections(ctx, f.world, "br_main"); err != nil || len(differences) != 0 {
		t.Fatal(differences, err)
	}
}

func TestRPConversationFocusChapterMovementAndAgeEndPreference(t *testing.T) {
	for _, boundary := range []string{"chapter", "move-and-return", "age"} {
		t.Run(boundary, func(t *testing.T) {
			f := newRPFocusFixture(t, "orchestrated")
			ctx := context.Background()
			partner := f.ids[len(f.ids)-1]
			f.turn(t, "before-boundary", f.names[partner]+"，聊一会儿。", partner)
			paused, pauseCalls := f.turn(t, "pause-before-boundary", "我再想想。", "")
			if len(pauseCalls) != 1 || pauseCalls[0] != partner {
				t.Fatal("the established exchange was lost before the boundary", pauseCalls)
			}
			assertM2Value(t, ctx, f.s, `SELECT COUNT(*) FROM rp_turn_listener_activations WHERE turn_run_id=? AND reason_code='conversation_continuation' AND conversation_source_event_id IS NOT NULL`, []any{paused.TurnRunID}, 1)
			view, err := f.s.ObserveRPSession(ctx, f.read)
			if err != nil {
				t.Fatal(err)
			}
			switch boundary {
			case "chapter":
				_, err = f.s.StartRPChapter(ctx, RPChapterStartRequest{PrincipalID: f.read.PrincipalID, SessionID: f.read.SessionID, ExpectedCursor: view.ObservationCursor, IdempotencyKey: "focus-chapter"})
			case "age":
				at, parseErr := time.Parse(time.RFC3339, view.WorldTime)
				if parseErr != nil {
					t.Fatal(parseErr)
				}
				_, err = f.s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: f.read.PrincipalID, SessionID: f.read.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: at.Add(31 * time.Minute).Format(time.RFC3339), Budget: 100, IdempotencyKey: "focus-age"})
			case "move-and-return":
				home, _ := core.StudioWorldObjectID(f.world, "place", "home")
				square, _ := core.StudioWorldObjectID(f.world, "place", "square")
				_, err = f.s.MoveRP(ctx, core.RPMoveRequest{PrincipalID: f.read.PrincipalID, SessionID: f.read.SessionID, ExpectedCursor: view.ObservationCursor, FromPlaceID: home, ToPlaceID: square, IdempotencyKey: "focus-away"})
				if err != nil {
					t.Fatal(err)
				}
				view, err = f.s.ObserveRPSession(ctx, f.read)
				if err != nil {
					t.Fatal(err)
				}
				_, err = f.s.MoveRP(ctx, core.RPMoveRequest{PrincipalID: f.read.PrincipalID, SessionID: f.read.SessionID, ExpectedCursor: view.ObservationCursor, FromPlaceID: square, ToPlaceID: home, IdempotencyKey: "focus-return"})
			}
			if err != nil {
				t.Fatal(err)
			}
			result, calls := f.turn(t, "after-boundary", "有人愿意说话吗？", "")
			if len(calls) != 1 || calls[0] != f.ids[0] {
				t.Fatal("old scene/chapter or expired exchange retained priority", calls)
			}
			assertM2Value(t, ctx, f.s, `SELECT COUNT(*) FROM rp_turn_listener_activations WHERE turn_run_id=? AND reason_code='conversation_continuation'`, []any{result.TurnRunID}, 0)
		})
	}
}

func TestRPConversationFocusMultipleSpeakersRetainNamedPartner(t *testing.T) {
	f := newRPFocusFixture(t, "legacy")
	ctx := context.Background()
	partner := f.ids[len(f.ids)-1]
	f.turn(t, "initial-named", f.names[partner]+"，可以谈谈吗？", partner)
	view, err := f.s.ObserveRPSession(ctx, f.read)
	if err != nil {
		t.Fatal(err)
	}
	all := rpDecisionProviderFunc(func(context.Context, core.RPDecisionInput) (core.RPDecisionProposal, error) {
		return core.RPDecisionProposal{Action: "respond", Text: "我也听见了。"}, nil
	})
	_, err = f.s.RunRPTurn(ctx, core.RPSpeechRequest{PrincipalID: f.read.PrincipalID, SessionID: f.read.SessionID, ExpectedCursor: view.ObservationCursor, Text: f.names[f.ids[0]] + "，现在请你说。", IdempotencyKey: "all-respond"}, all)
	if err != nil {
		t.Fatal(err)
	}
	_, calls := f.turn(t, "follow-many", "那请接着说。", f.ids[0])
	if calls[0] != f.ids[0] {
		t.Fatal("earlier continuation defeated the latest named public reply", calls)
	}
}

func (f *rpFocusFixture) turn(t *testing.T, key, text, speaker string) (RPTurnResult, []string) {
	t.Helper()
	ctx := context.Background()
	view, err := f.s.ObserveRPSession(ctx, f.read)
	if err != nil {
		t.Fatal(err)
	}
	calls := []string{}
	result, err := f.s.RunRPTurn(ctx, core.RPSpeechRequest{PrincipalID: f.read.PrincipalID, SessionID: f.read.SessionID, ExpectedCursor: view.ObservationCursor, IdempotencyKey: key, Text: text}, rpDecisionProviderFunc(func(_ context.Context, in core.RPDecisionInput) (core.RPDecisionProposal, error) {
		calls = append(calls, in.NPCEntityID)
		if in.NPCEntityID == speaker {
			return core.RPDecisionProposal{Action: "respond", Text: "我还在听，你继续说。"}, nil
		}
		return core.RPDecisionProposal{Action: "silence"}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	return result, calls
}

func (f *rpFocusFixture) binding(t *testing.T, key string) core.CareerBinding {
	t.Helper()
	var head int64
	if err := f.s.db.QueryRowContext(context.Background(), `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id='br_main'`, f.world).Scan(&head); err != nil {
		t.Fatal(err)
	}
	return core.CareerBinding{PrincipalID: "principal_creator", InstanceID: f.world, BranchID: "br_main", ExpectedHead: head, IdempotencyKey: key}
}

func (f *rpFocusFixture) otherController(t *testing.T, actor string) core.RPSessionReadRequest {
	t.Helper()
	ctx := context.Background()
	const principal = "principal_focus_other_service"
	if _, err := f.s.db.ExecContext(ctx, `INSERT OR IGNORE INTO principals(principal_id,principal_type,display_name,status) VALUES (?,'service','Other synthetic controller','active')`, principal); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.EnrollRPExternalControllerLocal(ctx, RPExternalControllerEnrollmentRequest{Binding: f.operatorBinding(t, "other-enroll-"+actor), EntityID: actor, ControllerPrincipalID: principal, ControllerInstanceID: "focus-controller-" + actor[len(actor)-16:]}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.AssignRPExternalControllerLocal(ctx, RPExternalControllerAssignmentRequest{Binding: f.operatorBinding(t, "other-assign-"+actor), EntityID: actor, ExpectedGeneration: 0}); err != nil {
		t.Fatal(err)
	}
	session, err := f.s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: principal, InstanceID: f.world, BranchID: "br_main", EntityID: actor, POV: "second_person", IdempotencyKey: "other-controller-" + actor})
	if err != nil {
		t.Fatal(err)
	}
	return core.RPSessionReadRequest{PrincipalID: principal, SessionID: session.SessionID}
}

func (f *rpFocusFixture) operatorBinding(t *testing.T, key string) core.CareerBinding {
	t.Helper()
	binding := f.binding(t, key)
	binding.PrincipalID = "principal_operator"
	return binding
}

func TestRPConversationFocusOtherPersonsExchangeDoesNotBecomeMine(t *testing.T) {
	f := newRPFocusFixture(t, "legacy")
	ctx := context.Background()
	partner := f.ids[len(f.ids)-1]
	other := f.otherController(t, f.ids[0])
	saved := f.read
	f.read = other
	f.turn(t, "outsider-dialogue", "有人能跟我说句话吗？", partner)
	f.read = saved
	_, calls := f.turn(t, "mine-after-outsider", "现在有人愿意听我说吗？", "")
	if len(calls) != 2 || calls[0] != f.ids[1] {
		t.Fatal("overheard conversation with another player hijacked my target", calls)
	}
	assertM2Value(t, ctx, f.s, `SELECT COUNT(*) FROM rp_turn_listener_activations a JOIN rp_turn_runs r ON r.turn_run_id=a.turn_run_id WHERE r.session_id=? AND r.idempotency_key='mine-after-outsider' AND a.reason_code='conversation_continuation'`, []any{saved.SessionID}, 0)
}

func TestRPConversationFocusUncommittedPrivateDecisionDoesNotSeedPartner(t *testing.T) {
	f := newRPFocusFixture(t, "orchestrated")
	ctx := context.Background()
	partner := f.ids[len(f.ids)-1]
	view, err := f.s.ObserveRPSession(ctx, f.read)
	if err != nil {
		t.Fatal(err)
	}
	speech, err := f.s.SpeakRP(ctx, core.RPSpeechRequest{PrincipalID: f.read.PrincipalID, SessionID: f.read.SessionID, ExpectedCursor: view.ObservationCursor, Text: "我想找人说说话。", IdempotencyKey: "audit-only-player"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.s.DecideRP(ctx, core.RPDecisionRequest{PrincipalID: f.read.PrincipalID, SessionID: f.read.SessionID, TurnID: speech.TurnID, NPCEntityID: partner}, rpDecisionProviderFunc(func(context.Context, core.RPDecisionInput) (core.RPDecisionProposal, error) {
		return core.RPDecisionProposal{Action: "respond", Text: "我在听。", Private: &core.RPDecisionPrivate{Intent: "这条私有意图不应成为公开交流依据"}}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	_, calls := f.turn(t, "after-audit-only", "有人吗？", "")
	if len(calls) != 1 || calls[0] != f.ids[0] {
		t.Fatal("audit-only intent/proposal was treated as an exchange", calls)
	}
}

func TestRPConversationFocusUnheardReplyIsNotRecoveredByLaterVisibility(t *testing.T) {
	f := newRPFocusFixture(t, "orchestrated")
	ctx := context.Background()
	partner := f.ids[len(f.ids)-1]
	home, _ := core.StudioWorldObjectID(f.world, "place", "home")
	if _, err := f.s.DefineRPPerceptionLink(ctx, RPPerceptionLinkRequest{Binding: f.binding(t, "focus-closed-link"), PlaceID: home, ZoneA: "far", ZoneB: "main", BarrierKind: "wall", BarrierState: "closed", DistanceM: 1, VisualRangeM: 5, AudioRangeM: 5}); err != nil {
		t.Fatal(err)
	}
	view, err := f.s.ObserveRPSession(ctx, f.read)
	if err != nil {
		t.Fatal(err)
	}
	speech, err := f.s.SpeakRP(ctx, core.RPSpeechRequest{PrincipalID: f.read.PrincipalID, SessionID: f.read.SessionID, ExpectedCursor: view.ObservationCursor, Text: "我想说句话。", IdempotencyKey: "before-unheard"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.PlaceRPActorInZone(ctx, RPActorZoneRequest{Binding: f.binding(t, "focus-player-hidden"), AgentID: f.player, PlaceID: home, ZoneKey: "far"}); err != nil {
		t.Fatal(err)
	}
	req := core.RPDecisionRequest{PrincipalID: f.read.PrincipalID, SessionID: f.read.SessionID, TurnID: speech.TurnID, NPCEntityID: partner}
	decision, err := f.s.DecideRP(ctx, req, rpDecisionProviderFunc(func(context.Context, core.RPDecisionInput) (core.RPDecisionProposal, error) {
		return core.RPDecisionProposal{Action: "respond", Text: "这句玩家并没有听见。"}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	committed, err := f.s.CommitRPDecision(ctx, req, decision)
	if err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, f.s, `SELECT COUNT(*) FROM observation_records WHERE source_event_id=? AND observer_agent_id=?`, []any{committed.EventID, f.player}, 0)
	if _, err := f.s.PlaceRPActorInZone(ctx, RPActorZoneRequest{Binding: f.binding(t, "focus-player-visible-again"), AgentID: f.player, PlaceID: home, ZoneKey: "main"}); err != nil {
		t.Fatal(err)
	}
	_, calls := f.turn(t, "after-unheard", "有人愿意说话吗？", "")
	if len(calls) != 1 || calls[0] != f.ids[0] {
		t.Fatal("later visibility retroactively promoted unheard speech", calls)
	}
}

func TestRPConversationFocusWitnessedNPCDepartureAndReturnEndsPreference(t *testing.T) {
	f := newRPFocusFixture(t, "orchestrated")
	ctx := context.Background()
	partner := f.ids[len(f.ids)-1]
	f.turn(t, "before-npc-departure", f.names[partner]+"，说句话。", partner)
	other := f.otherController(t, partner)
	home, _ := core.StudioWorldObjectID(f.world, "place", "home")
	square, _ := core.StudioWorldObjectID(f.world, "place", "square")
	for i, places := range [][2]string{{home, square}, {square, home}} {
		view, err := f.s.ObserveRPSession(ctx, other)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.s.MoveRP(ctx, core.RPMoveRequest{PrincipalID: other.PrincipalID, SessionID: other.SessionID, ExpectedCursor: view.ObservationCursor, FromPlaceID: places[0], ToPlaceID: places[1], IdempotencyKey: []string{"npc-away", "npc-return"}[i]}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.s.CloseRPSession(ctx, other); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.ReleaseRPExternalControllerLocal(ctx, RPExternalControllerReleaseRequest{Binding: f.operatorBinding(t, "npc-return-release"), EntityID: partner, ExpectedGeneration: 1}); err != nil {
		t.Fatal(err)
	}
	_, calls := f.turn(t, "after-npc-return", "有人愿意聊天吗？", "")
	if len(calls) != 1 || calls[0] != f.ids[0] {
		t.Fatal("witnessed departure/return did not end old focus", calls)
	}
}

func TestRPConversationFocusHandoffClearsPinnedSource(t *testing.T) {
	f := newRPFocusFixture(t, "orchestrated")
	ctx := context.Background()
	partner := f.ids[len(f.ids)-1]
	f.turn(t, "before-focus-handoff", f.names[partner]+"，能聊聊吗？", partner)
	const controller = "principal_focus_handoff_service"
	if _, err := f.s.db.ExecContext(ctx, `INSERT INTO principals(principal_id,principal_type,display_name,status) VALUES (?,'service','Synthetic handoff','active')`, controller); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.EnrollRPExternalControllerLocal(ctx, RPExternalControllerEnrollmentRequest{Binding: f.operatorBinding(t, "focus-handoff-enroll"), EntityID: partner, ControllerPrincipalID: controller, ControllerInstanceID: "focus-handoff-controller"}); err != nil {
		t.Fatal(err)
	}
	view, err := f.s.ObserveRPSession(ctx, f.read)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	result, err := f.s.RunRPTurn(ctx, core.RPSpeechRequest{PrincipalID: f.read.PrincipalID, SessionID: f.read.SessionID, ExpectedCursor: view.ObservationCursor, Text: "请接着说吧。", IdempotencyKey: "focus-handoff-turn"}, rpDecisionProviderFunc(func(context.Context, core.RPDecisionInput) (core.RPDecisionProposal, error) {
		calls++
		if _, err := f.s.AssignRPExternalControllerLocal(ctx, RPExternalControllerAssignmentRequest{Binding: f.operatorBinding(t, "focus-handoff-assign"), EntityID: partner, ExpectedGeneration: 0}); err != nil {
			return core.RPDecisionProposal{}, err
		}
		return core.RPDecisionProposal{Action: "respond", Text: "这句尚未获准。"}, nil
	}))
	if err != nil || result.Status != "settled" || calls != 1 {
		t.Fatal(result, calls, err)
	}
	assertM2Value(t, ctx, f.s, `SELECT COUNT(*) FROM rp_turn_listener_activations WHERE turn_run_id=? AND npc_entity_id=? AND disposition='externally_controlled' AND reason_code='external_controller' AND activation_rank IS NULL AND conversation_source_event_id IS NULL AND owner_source_event_id IS NOT NULL`, []any{result.TurnRunID, partner}, 1)
	assertM2Value(t, ctx, f.s, `SELECT COUNT(*) FROM rp_npc_decisions WHERE parent_turn_id=? AND npc_entity_id=?`, []any{result.PlayerTurnID, partner}, 0)
	if differences, err := f.s.CompareProjections(ctx, f.world, "br_main"); err != nil || len(differences) != 0 {
		t.Fatal(differences, err)
	}
}

func TestRPConversationFocusUpgradePreservesPopulatedOldPlans(t *testing.T) {
	f := newRPFocusFixture(t, "orchestrated")
	ctx := context.Background()
	partner := f.ids[len(f.ids)-1]
	first, _ := f.turn(t, "old-plan", f.names[partner]+"，说句话。", partner)
	before, err := f.s.loadRPTurnActivations(ctx, first.TurnRunID)
	if err != nil {
		t.Fatal(err)
	}
	ledger := func() string {
		var value string
		if err := f.s.db.QueryRowContext(ctx, `SELECT json_group_array(json_array(event_sequence,event_id,event_type,payload)) FROM (SELECT event_sequence,event_id,event_type,payload FROM events WHERE instance_id=? AND branch_id='br_main' ORDER BY event_sequence)`, f.world).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	beforeLedger := ledger()
	var head int64
	if err := f.s.db.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id='br_main'`, f.world).Scan(&head); err != nil {
		t.Fatal(err)
	}
	original, err := migrationFiles.ReadFile("migrations/060_rp_turn_activation.sql")
	if err != nil {
		t.Fatal(err)
	}
	ddl := string(original)
	start, end := strings.Index(ddl, "CREATE TABLE rp_turn_listener_activations"), strings.Index(ddl, "INSERT INTO schema_meta")
	if start < 0 || end <= start {
		t.Fatal("original activation migration definition not found")
	}
	// Downgrade only this synthetic test database to the exact pre-076 table.
	tx, err := f.s.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	for _, query := range []string{
		`CREATE TEMP TABLE focus_old_plans AS SELECT turn_run_id,npc_entity_id,disposition,reason_code,activation_rank,owner_source_event_id FROM rp_turn_listener_activations`,
		`DROP TABLE rp_turn_listener_activations`,
		ddl[start:end],
		`INSERT INTO rp_turn_listener_activations SELECT * FROM focus_old_plans`,
		`DROP TABLE focus_old_plans`,
	} {
		if _, err := tx.ExecContext(ctx, query); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM schema_meta WHERE schema_version=?`, RPConversationFocusSchemaVersion); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := f.s.Close(); err != nil {
		t.Fatal(err)
	}
	f.s, err = Open(ctx, f.path)
	if err != nil {
		t.Fatal(err)
	}
	after, err := f.s.loadRPTurnActivations(ctx, first.TurnRunID)
	if err != nil || !reflect.DeepEqual(before, after) || beforeLedger != ledger() {
		t.Fatal("migration rewrote old plans or world facts", before, after, err)
	}
	assertM2Value(t, ctx, f.s, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id='br_main'`, []any{f.world}, head)
	rows, err := f.s.db.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatal(err)
	}
	if rows.Next() {
		rows.Close()
		t.Fatal("migration left invalid references")
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	_, calls := f.turn(t, "post-upgrade-focus", "那接着说吧。", partner)
	if len(calls) != 1 || calls[0] != partner {
		t.Fatal("upgraded old world did not use public continuity", calls)
	}
}

func TestRPConversationFocusInitiativeOpensAnObservedExchange(t *testing.T) {
	f := newRPFocusFixture(t, "orchestrated")
	ctx := context.Background()
	partner := f.ids[len(f.ids)-1]
	view, err := f.s.ObserveRPSession(ctx, f.read)
	if err != nil {
		t.Fatal(err)
	}
	at, err := time.Parse(time.RFC3339, view.WorldTime)
	if err != nil {
		t.Fatal(err)
	}
	wait, err := f.s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: f.read.PrincipalID, SessionID: f.read.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: at.Add(time.Minute).Format(time.RFC3339), Budget: 100, IdempotencyKey: "before-initiative"})
	if err != nil {
		t.Fatal(err)
	}
	initiative, err := f.s.RunRPInitiative(ctx, core.RPInitiativeRequest{PrincipalID: f.read.PrincipalID, SessionID: f.read.SessionID, NPCEntityID: partner, TriggerEventID: wait.EventID}, rpDecisionProviderFunc(func(context.Context, core.RPDecisionInput) (core.RPDecisionProposal, error) {
		return core.RPDecisionProposal{Action: "respond", Text: "若想说话，我愿意听。", Private: &core.RPDecisionPrivate{Intent: "私下想耐心听完"}}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	follow, calls := f.turn(t, "answer-initiative", "那我就接着说了。", partner)
	if len(calls) != 1 || calls[0] != partner {
		t.Fatal("personally heard initiative was ignored", calls)
	}
	assertM2Value(t, ctx, f.s, `SELECT COUNT(*) FROM rp_turn_listener_activations WHERE turn_run_id=? AND conversation_source_event_id=? AND reason_code='conversation_continuation'`, []any{follow.TurnRunID, initiative.EventID}, 1)
}

func TestRPConversationFocusMultipleUnaddressedSpeakersRemainAmbiguous(t *testing.T) {
	f := newRPFocusFixture(t, "legacy")
	ctx := context.Background()
	view, err := f.s.ObserveRPSession(ctx, f.read)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.s.RunRPTurn(ctx, core.RPSpeechRequest{PrincipalID: f.read.PrincipalID, SessionID: f.read.SessionID, ExpectedCursor: view.ObservationCursor, Text: "有人愿意说话吗？", IdempotencyKey: "ambiguous-multiple"}, rpDecisionProviderFunc(func(context.Context, core.RPDecisionInput) (core.RPDecisionProposal, error) {
		return core.RPDecisionProposal{Action: "respond", Text: "我也愿意听。"}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	result, calls := f.turn(t, "after-ambiguous", "那接着说吧。", "")
	if calls[0] != f.ids[0] {
		t.Fatal("an arbitrary last speaker became definite focus", calls)
	}
	assertM2Value(t, ctx, f.s, `SELECT COUNT(*) FROM rp_turn_listener_activations WHERE turn_run_id=? AND reason_code='conversation_continuation'`, []any{result.TurnRunID}, 0)
}

func TestRPConversationFocusContinuesActualPartnerAndDirectAddressOverrides(t *testing.T) {
	for _, mode := range []string{"orchestrated", "multi_agent", "legacy"} {
		t.Run(mode, func(t *testing.T) {
			f := newRPFocusFixture(t, mode)
			partner := f.ids[len(f.ids)-1]
			first, _ := f.turn(t, "establish", f.names[partner]+"，能听我说吗？", partner)
			follow, calls := f.turn(t, "follow", "那我接着说。", partner)
			if len(calls) == 0 || calls[0] != partner {
				t.Fatalf("unaddressed continuation switched partner: %v want %s", calls, partner)
			}
			wantCalls := 1
			if mode == "legacy" {
				wantCalls = 3
			}
			if len(calls) != wantCalls {
				t.Fatal("continuation changed the execution-mode responder budget", calls)
			}
			assertM2Value(t, context.Background(), f.s, `SELECT COUNT(*) FROM rp_turn_listener_activations a JOIN events e ON e.event_id=a.conversation_source_event_id WHERE a.turn_run_id=? AND a.npc_entity_id=? AND a.reason_code='conversation_continuation' AND a.activation_rank=0 AND e.event_type='RPSpeechAccepted' AND e.event_id IN (SELECT event_id FROM rp_npc_decisions WHERE parent_turn_id=?)`, []any{follow.TurnRunID, partner, first.PlayerTurnID}, 1)
			_, calls = f.turn(t, "switch", f.names[f.ids[0]]+"，这次请你说。", f.ids[0])
			if len(calls) == 0 || calls[0] != f.ids[0] {
				t.Fatal("previous partner overrode current explicit address", calls)
			}
		})
	}
}

func TestRPConversationFocusSilenceDoesNotInventEngagement(t *testing.T) {
	f := newRPFocusFixture(t, "orchestrated")
	partner := f.ids[len(f.ids)-1]
	f.turn(t, "unanswered", f.names[partner]+"，你在听吗？", "")
	result, calls := f.turn(t, "neutral", "有人愿意说话吗？", "")
	if len(calls) != 1 || calls[0] != f.ids[0] {
		t.Fatal("an unspoken proposal or silence invented a prior dialogue", calls)
	}
	assertM2Value(t, context.Background(), f.s, `SELECT COUNT(*) FROM rp_turn_listener_activations WHERE turn_run_id=? AND reason_code='conversation_continuation'`, []any{result.TurnRunID}, 0)
}

func TestRPConversationFocusSilentContinuationsRetainHeardSourceAcrossRestart(t *testing.T) {
	f := newRPFocusFixture(t, "orchestrated")
	ctx := context.Background()
	partner := f.ids[len(f.ids)-1]
	first, _ := f.turn(t, "before-pause", f.names[partner]+"，陪我聊聊好吗？", partner)
	var source string
	if err := f.s.db.QueryRowContext(ctx, `SELECT event_id FROM rp_npc_decisions WHERE session_id=? AND parent_turn_id=? AND npc_entity_id=?`, f.read.SessionID, first.PlayerTurnID, partner).Scan(&source); err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"wait", "silence"} {
		view, err := f.s.ObserveRPSession(ctx, f.read)
		if err != nil {
			t.Fatal(err)
		}
		paused, err := f.s.RunRPTurn(ctx, core.RPSpeechRequest{PrincipalID: f.read.PrincipalID, SessionID: f.read.SessionID, ExpectedCursor: view.ObservationCursor, Text: "我再想一想。", IdempotencyKey: "pause-" + action}, rpDecisionProviderFunc(func(_ context.Context, in core.RPDecisionInput) (core.RPDecisionProposal, error) {
			if in.NPCEntityID != partner {
				t.Fatal("a silent continuation switched to a fallback listener")
			}
			return core.RPDecisionProposal{Action: action}, nil
		}))
		if err != nil {
			t.Fatal(err)
		}
		assertM2Value(t, ctx, f.s, `SELECT COUNT(*) FROM rp_turn_listener_activations WHERE turn_run_id=? AND npc_entity_id=? AND reason_code='conversation_continuation' AND conversation_source_event_id=?`, []any{paused.TurnRunID, partner, source}, 1)
	}
	if err := f.s.Close(); err != nil {
		t.Fatal(err)
	}
	var err error
	f.s, err = Open(ctx, f.path)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.s.RebuildProjections(ctx, f.world, "br_main"); err != nil {
		t.Fatal(err)
	}
	follow, calls := f.turn(t, "after-pause", "接着刚才的话说吧。", partner)
	if len(calls) != 1 || calls[0] != partner {
		t.Fatal("restart lost the last personally heard exchange", calls)
	}
	assertM2Value(t, ctx, f.s, `SELECT COUNT(*) FROM rp_turn_listener_activations WHERE turn_run_id=? AND conversation_source_event_id=? AND reason_code='conversation_continuation'`, []any{follow.TurnRunID, source}, 1)
	if differences, err := f.s.CompareProjections(ctx, f.world, "br_main"); err != nil || len(differences) != 0 {
		t.Fatal(differences, err)
	}
}

func TestRPConversationFocusSilentNamedSwitchDoesNotRestoreOldPartner(t *testing.T) {
	f := newRPFocusFixture(t, "orchestrated")
	ctx := context.Background()
	partner := f.ids[len(f.ids)-1]
	f.turn(t, "old-partner", f.names[partner]+"，聊聊吧。", partner)
	f.turn(t, "unanswered-new-partner", f.names[f.ids[1]]+"，这次请你说。", "")
	result, calls := f.turn(t, "after-unanswered-switch", "有人愿意继续吗？", "")
	if len(calls) != 1 || calls[0] != f.ids[0] {
		t.Fatal("a silent named switch restored an old exchange", calls)
	}
	assertM2Value(t, ctx, f.s, `SELECT COUNT(*) FROM rp_turn_listener_activations WHERE turn_run_id=? AND reason_code='conversation_continuation'`, []any{result.TurnRunID}, 0)
}
