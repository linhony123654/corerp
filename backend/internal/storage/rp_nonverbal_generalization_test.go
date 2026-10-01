package storage

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"corerp.local/backend/internal/core"
)

func TestRPNonverbalGeneralizedEveryLegalActionPreservesActualTrigger(t *testing.T) {
	for _, kind := range []struct{ action, gesture string }{{"look_at", ""}, {"smile", ""}, {"nod", ""}, {"shake_head", ""}, {"turn_away", ""}, {"frown", ""}, {"gesture", "wave"}, {"gesture", "shrug"}, {"gesture", "raise_hand"}, {"gesture", "beckon"}} {
		for _, directed := range []bool{true, false} {
			if kind.action == "look_at" && !directed {
				continue
			}
			t.Run(kind.action+"-"+kind.gesture+map[bool]string{true: "-directed", false: "-untargeted"}[directed], func(t *testing.T) {
				ctx := context.Background()
				s, err := Open(ctx, filepath.Join(t.TempDir(), "nonverb.db"))
				if err != nil {
					t.Fatal(err)
				}
				defer s.Close()
				_, read, initial := newRPWaitTestSession(t, ctx, s)
				request := core.RPNonverbalRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, Action: kind.action, GestureCode: kind.gesture, ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "generalized-action"}
				if directed {
					request.TargetEntityID = M2RPNPCID
				}
				calls := 0
				provider := rpDecisionProviderFunc(func(_ context.Context, in core.RPDecisionInput) (core.RPDecisionProposal, error) {
					calls++
					observed := in.ObservedPlayerAction
					if observed == nil || observed.Action != kind.action || observed.GestureCode != kind.gesture || observed.SourceEventID != in.TurnID || in.Trigger == nil || in.Trigger.Kind != "nonverbal" || in.Trigger.SourceEventID != in.TurnID || in.PlayerSpeechText != "" || in.SpeechEventID != "" || in.PlayerSpeechWorldTime != "" || observed.WorldTime != initial.WorldTime {
						t.Fatalf("incorrect physical trigger: %+v", in)
					}
					if directed && (in.NPCEntityID != M2RPNPCID || observed.TargetEntityID != M2RPNPCID) || !directed && observed.TargetEntityID != "" {
						t.Fatalf("invented/replaced recipient: %+v", observed)
					}
					return core.RPDecisionProposal{Action: "silence", Private: &core.RPDecisionPrivate{Intent: "private-nonverbal-token", BasisEventIDs: []string{observed.SourceEventID}}}, nil
				})
				service, err := NewRPService(s, provider, "deterministic")
				if err != nil {
					t.Fatal(err)
				}
				out, err := service.NonverbalRP(ctx, request)
				if err != nil || out.Turn == nil || out.Turn.Status != "settled" {
					t.Fatalf("closed action did not react: %+v %v", out, err)
				}
				if calls == 0 || directed && calls != 1 {
					t.Fatalf("wrong responder count %d", calls)
				}
				assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_utterances WHERE speaker_entity_id=?`, []any{M2RPPlayerID}, 0)
				assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='RPNonverbalAction' AND actor_id=?`, []any{M2RPPlayerID}, 1)
				var clock string
				if err := s.db.QueryRowContext(ctx, `SELECT current_world_time FROM world_clocks WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&clock); err != nil || clock != initial.WorldTime {
					t.Fatalf("gesture advanced time: %s %v", clock, err)
				}
				if strings.Contains(strings.Join(out.Turn.NarrativeLines, "\n"), "private-nonverbal-token") {
					t.Fatal("private continuation leaked")
				}
				replayed, err := service.NonverbalRPWith(ctx, request, rpDecisionProviderFunc(func(context.Context, core.RPDecisionInput) (core.RPDecisionProposal, error) {
					t.Fatal("settled action called provider again")
					return core.RPDecisionProposal{}, nil
				}))
				if err != nil || replayed.Turn == nil || !replayed.Replayed || replayed.EventID != out.EventID || !reflect.DeepEqual(replayed.Turn.NarrativeLines, out.Turn.NarrativeLines) {
					t.Fatalf("saved action changed: %+v %v", replayed, err)
				}
			})
		}
	}
}

func TestRPNonverbalGeneralizedUntargetedUsesBoundedPublicFocus(t *testing.T) {
	ctx := context.Background()
	f := newRPFocusFixture(t, "orchestrated")
	partner := f.ids[len(f.ids)-1]
	f.turn(t, "prior-public-exchange", f.names[partner]+"，说句话好吗？", partner)
	view, err := f.s.ObserveRPSession(ctx, f.read)
	if err != nil {
		t.Fatal(err)
	}
	request := core.RPNonverbalRequest{PrincipalID: f.read.PrincipalID, SessionID: f.read.SessionID, Action: "smile", ExpectedCursor: view.ObservationCursor, IdempotencyKey: "untargeted-focused-smile"}
	calls := []string{}
	out, err := f.s.RunRPNonverbalTurn(ctx, request, rpDecisionProviderFunc(func(_ context.Context, in core.RPDecisionInput) (core.RPDecisionProposal, error) {
		calls = append(calls, in.NPCEntityID)
		if in.ObservedPlayerAction == nil || in.ObservedPlayerAction.TargetEntityID != "" {
			t.Fatal("focus invented target")
		}
		return core.RPDecisionProposal{Action: "silence"}, nil
	}))
	if err != nil || !reflect.DeepEqual(calls, []string{partner}) {
		t.Fatalf("focus not grounded/bounded: calls=%v err=%v", calls, err)
	}
	assertM2Value(t, ctx, f.s, `SELECT COUNT(*) FROM rp_turn_listener_activations WHERE turn_run_id=?`, []any{out.TurnRunID}, 3)
	assertM2Value(t, ctx, f.s, `SELECT COUNT(*) FROM rp_turn_listener_activations WHERE turn_run_id=? AND disposition='activated' AND reason_code='conversation_continuation' AND conversation_source_event_id IS NOT NULL`, []any{out.TurnRunID}, 1)
	assertM2Value(t, ctx, f.s, `SELECT COUNT(*) FROM rp_turn_listener_activations WHERE turn_run_id=? AND disposition='not_activated'`, []any{out.TurnRunID}, 2)
	// A public exchange older than the existing 30-minute window cannot
	// silently turn a later untargeted gesture into a directed continuation.
	view, err = f.s.ObserveRPSession(ctx, f.read)
	if err != nil {
		t.Fatal(err)
	}
	at, err := time.Parse(time.RFC3339, view.WorldTime)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: f.read.PrincipalID, SessionID: f.read.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: at.Add(31 * time.Minute).Format(time.RFC3339), Budget: 100, IdempotencyKey: "focus-expired"}); err != nil {
		t.Fatal(err)
	}
	view, err = f.s.ObserveRPSession(ctx, f.read)
	if err != nil {
		t.Fatal(err)
	}
	request.ExpectedCursor, request.IdempotencyKey = view.ObservationCursor, "untargeted-expired-smile"
	calls = nil
	_, err = f.s.RunRPNonverbalTurn(ctx, request, rpDecisionProviderFunc(func(_ context.Context, in core.RPDecisionInput) (core.RPDecisionProposal, error) {
		calls = append(calls, in.NPCEntityID)
		return core.RPDecisionProposal{Action: "silence"}, nil
	}))
	if err != nil || !reflect.DeepEqual(calls, []string{f.ids[0]}) {
		t.Fatalf("expired focus was retained: %v %v", calls, err)
	}
}

func TestRPNonverbalGeneralizedUntargetedHiddenWitnessAndAnonymousActor(t *testing.T) {
	for _, hidden := range []bool{false, true} {
		t.Run(map[bool]string{false: "anonymous actor", true: "no visual witness"}[hidden], func(t *testing.T) {
			ctx := context.Background()
			f := newRPFocusFixture(t, "orchestrated", false)
			if hidden {
				home, _ := core.StudioWorldObjectID(f.world, "place", "home")
				if _, err := f.s.DefineRPPerceptionLink(ctx, RPPerceptionLinkRequest{Binding: f.binding(t, "hidden-action-link"), PlaceID: home, ZoneA: "far", ZoneB: "main", BarrierKind: "wall", BarrierState: "closed", DistanceM: 1, VisualRangeM: 5, AudioRangeM: 5}); err != nil {
					t.Fatal(err)
				}
				for i, id := range f.ids {
					if _, err := f.s.PlaceRPActorInZone(ctx, RPActorZoneRequest{Binding: f.binding(t, "hidden-action-zone-"+string(rune('a'+i))), AgentID: id, PlaceID: home, ZoneKey: "far"}); err != nil {
						t.Fatal(err)
					}
				}
			}
			view, err := f.s.ObserveRPSession(ctx, f.read)
			if err != nil {
				t.Fatal(err)
			}
			calls := 0
			out, err := f.s.RunRPNonverbalTurn(ctx, core.RPNonverbalRequest{PrincipalID: f.read.PrincipalID, SessionID: f.read.SessionID, ExpectedCursor: view.ObservationCursor, IdempotencyKey: "untargeted-private-scene", Action: "frown"}, rpDecisionProviderFunc(func(_ context.Context, in core.RPDecisionInput) (core.RPDecisionProposal, error) {
				calls++
				if hidden {
					t.Fatal("unseen gesture called NPC")
				}
				if in.ObservedPlayerAction == nil || in.ObservedPlayerAction.TargetEntityID != "" || in.ObservedPlayerAction.ActorEntityID == f.player || in.ObservedPlayerAction.ActorEntityID != in.InterlocutorEntityID {
					t.Fatalf("unknown actor canonicalized: %+v", in.ObservedPlayerAction)
				}
				return core.RPDecisionProposal{Action: "silence"}, nil
			}))
			if err != nil || out.Status != "settled" || hidden && calls != 0 || !hidden && calls != 1 {
				t.Fatalf("scoped gesture: calls=%d err=%v", calls, err)
			}
			var raw string
			if err := f.s.db.QueryRowContext(ctx, `SELECT payload FROM events WHERE event_id=?`, out.PlayerEventID).Scan(&raw); err != nil {
				t.Fatal(err)
			}
			var fact core.RPNonverbalFact
			if err := json.Unmarshal([]byte(raw), &fact); err != nil {
				t.Fatal(err)
			}
			if fact.TargetEntityID != "" || hidden && len(fact.Witnesses) != 0 {
				t.Fatalf("invented frozen recipients: %+v", fact)
			}
		})
	}
}

func TestRPNonverbalGeneralizedUntargetedRespectsExistingExecutionModes(t *testing.T) {
	for _, mode := range []string{"deterministic", "orchestrated", "multi_agent", "legacy"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			f := newRPFocusFixture(t, mode)
			view, err := f.s.ObserveRPSession(ctx, f.read)
			if err != nil {
				t.Fatal(err)
			}
			calls := []string{}
			result, err := f.s.RunRPNonverbalTurn(ctx, core.RPNonverbalRequest{PrincipalID: f.read.PrincipalID, SessionID: f.read.SessionID, Action: "nod", ExpectedCursor: view.ObservationCursor, IdempotencyKey: "mode-nod"}, rpDecisionProviderFunc(func(_ context.Context, in core.RPDecisionInput) (core.RPDecisionProposal, error) {
				calls = append(calls, in.NPCEntityID)
				if in.ObservedPlayerAction.TargetEntityID != "" {
					t.Fatal("policy invented a directed nod")
				}
				return core.RPDecisionProposal{Action: "silence"}, nil
			}))
			want := f.ids[:1]
			if mode == "legacy" {
				want = f.ids
			}
			if err != nil || !reflect.DeepEqual(calls, want) {
				t.Fatalf("mode %s changed responder policy: %v %v", mode, calls, err)
			}
			assertM2Value(t, ctx, f.s, `SELECT COUNT(*) FROM rp_turn_listener_activations WHERE turn_run_id=?`, []any{result.TurnRunID}, 3)
		})
	}
}

func TestRPNonverbalGeneralizedConcurrentSettledWinnerSurvivesLosingStage(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "concurrent-winner.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, read, initial := newRPWaitTestSession(t, ctx, s)
	request := core.RPNonverbalRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, Action: "smile", ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "concurrent-smile"}
	calls := 0
	provider := rpDecisionProviderFunc(func(context.Context, core.RPDecisionInput) (core.RPDecisionProposal, error) {
		calls++
		return core.RPDecisionProposal{Action: "silence"}, nil
	})
	var winner RPTurnResult
	s.afterRPTurnStage = func(stage string) error {
		if stage != "turn_open" {
			return nil
		}
		// Interleave a complete same-key retry while the first invocation
		// holds its stale open-stage snapshot, with no concurrent data race.
		s.afterRPTurnStage = nil
		var err error
		winner, err = s.RunRPNonverbalTurn(ctx, request, provider)
		if err != nil {
			t.Fatal(err)
		}
		return errors.New("losing worker interrupted after the saved winner")
	}
	loser, err := s.RunRPNonverbalTurn(ctx, request, provider)
	if err != nil || !loser.Replayed || loser.TurnRunID != winner.TurnRunID || !reflect.DeepEqual(loser.NarrativeLines, winner.NarrativeLines) || calls == 0 {
		t.Fatalf("completed winner was lost: %+v %v", loser, err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='RPNonverbalAction' AND actor_id=?`, []any{M2RPPlayerID}, 1)
}

func TestRPNonverbalGeneralizedRawReceiptDoesNotGainRetroactiveReactions(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "raw-receipt.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, read, initial := newRPWaitTestSession(t, ctx, s)
	request := core.RPNonverbalRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, Action: "frown", ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "pre-upgrade-raw"}
	raw, err := s.NonverbalRP(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewRPService(s, rpDecisionProviderFunc(func(context.Context, core.RPDecisionInput) (core.RPDecisionProposal, error) {
		t.Fatal("old raw receipt gained provider call")
		return core.RPDecisionProposal{}, nil
	}), "deterministic")
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := service.NonverbalRP(ctx, request)
	if err != nil || !replayed.Replayed || replayed.Turn != nil || replayed.EventID != raw.EventID {
		t.Fatalf("old receipt upgraded: %+v %v", replayed, err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_turn_runs WHERE session_id=?`, []any{read.SessionID}, 0)
	if _, _, err := s.ensureRPNonverbalTurnRun(ctx, request, core.DeterministicRPDecisionProvider{}); !errors.Is(err, errRPNonverbalRawReceipt) {
		t.Fatalf("intent transaction did not fence raw command: %v", err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_turn_runs WHERE session_id=?`, []any{read.SessionID}, 0)
}

func TestRPNonverbalGeneralizedUntargetedRestartRetainsPinnedPlan(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "untargeted-restart.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Close() }()
	_, read, initial := newRPWaitTestSession(t, ctx, s)
	request := core.RPNonverbalRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, Action: "gesture", GestureCode: "shrug", ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "untargeted-recovery"}
	calls := 0
	provider := rpDecisionProviderFunc(func(_ context.Context, in core.RPDecisionInput) (core.RPDecisionProposal, error) {
		calls++
		if in.ObservedPlayerAction == nil || in.ObservedPlayerAction.TargetEntityID != "" {
			t.Fatal("shrug invented target")
		}
		return core.RPDecisionProposal{Action: "silence"}, nil
	})
	s.afterRPTurnStage = func(stage string) error {
		if stage == "npc_effects_committed" {
			return errors.New("injected completed effects crash")
		}
		return nil
	}
	if _, err := s.RunRPNonverbalTurn(ctx, request, provider); err == nil {
		t.Fatal("recovery cut not reached")
	}
	before := calls
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	result, err := s.ResumeRPTurn(ctx, RPTurnResumeRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, IdempotencyKey: request.IdempotencyKey}, provider)
	if err != nil || result.Status != "settled" || calls != before || before == 0 {
		t.Fatalf("restart regenerated reactions: calls=%d before=%d err=%v", calls, before, err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='RPNonverbalAction' AND actor_id=?`, []any{M2RPPlayerID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_utterances WHERE speaker_entity_id=?`, []any{M2RPPlayerID}, 0)
}
