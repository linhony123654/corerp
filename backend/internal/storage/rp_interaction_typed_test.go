package storage

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPInteractionModelMovesSourcedObjectThenSpeaks(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "interaction-object.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, player, _ := newRPWaitTestSession(t, ctx, s)
	source, from, to := objectTestSource(t, ctx, s)
	stage := objectTestRequest(t, ctx, s, player, "stage", "typed-object-stage")
	stage.SourceID = source
	item, err := s.ObjectRP(ctx, stage)
	if err != nil {
		t.Fatal(err)
	}
	place := objectTestRequest(t, ctx, s, player, "place", "typed-object-place")
	place.ObjectID, place.AnchorID = item.ObjectID, from
	if _, err := s.ObjectRP(ctx, place); err != nil {
		t.Fatal(err)
	}
	view, err := s.ObserveRPSession(ctx, player)
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	model := semanticFixtureServer(t, func(text string) string {
		if !strings.Contains(text, "物件") {
			t.Errorf("unexpected text %q", text)
		}
		return semanticModelReply("MIXED", []core.RPInteractionStep{{Kind: "object", ObjectAction: "move", ObjectID: item.ObjectID, AnchorID: to}, {Kind: "speech", SpeechText: "喝一点吧。"}}, "")
	}, &calls)
	defer model.Close()
	service, err := NewRPService(s, semanticFixtureProvider(t, model), "chat_completions")
	if err != nil {
		t.Fatal(err)
	}
	request := core.RPInteractionRequest{PrincipalID: player.PrincipalID, SessionID: player.SessionID, Text: "我把桌上物件推到她近旁，说「喝一点吧。」", Mode: "AUTO", ExpectedCursor: view.ObservationCursor, IdempotencyKey: "typed-move-speech"}
	result, err := service.RunRPInteraction(ctx, request)
	if err != nil || result.Status != "settled" || len(result.Outcomes) != 2 || result.Outcomes[0].Kind != "object" || result.Outcomes[1].Kind != "speech" || calls.Load() != 1 {
		t.Fatalf("object-then-speech plan: %+v %v (interpreted %d times)", result, err, calls.Load())
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_objects WHERE object_id=? AND physical_state='placed' AND anchor_id=?`, []any{item.ObjectID, to}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_object_offers WHERE object_id=?`, []any{item.ObjectID}, 0)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_utterances WHERE speaker_entity_id=? AND speech_text='喝一点吧。'`, []any{M2RPPlayerID}, 1)
	if differences, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) > 0 {
		t.Fatalf("object interaction projection drift: %+v %v", differences, err)
	}
}

func TestRPInteractionStateChangesDoNotExecuteDependentSpeech(t *testing.T) {
	for _, changeAfterAction := range []bool{false, true} {
		name := "before object action"
		if changeAfterAction {
			name = "after object action"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			s, err := Open(ctx, filepath.Join(t.TempDir(), "interaction-state-change.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			_, player, _ := newRPWaitTestSession(t, ctx, s)
			source, from, to := objectTestSource(t, ctx, s)
			stage := objectTestRequest(t, ctx, s, player, "stage", "change-stage")
			stage.SourceID = source
			item, err := s.ObjectRP(ctx, stage)
			if err != nil {
				t.Fatal(err)
			}
			place := objectTestRequest(t, ctx, s, player, "place", "change-place")
			place.ObjectID, place.AnchorID = item.ObjectID, from
			if _, err := s.ObjectRP(ctx, place); err != nil {
				t.Fatal(err)
			}
			view, err := s.ObserveRPSession(ctx, player)
			if err != nil {
				t.Fatal(err)
			}
			var calls atomic.Int32
			model := semanticFixtureServer(t, func(string) string {
				return semanticModelReply("MIXED", []core.RPInteractionStep{{Kind: "object", ObjectAction: "move", ObjectID: item.ObjectID, AnchorID: to}, {Kind: "speech", SpeechText: "好了。"}}, "")
			}, &calls)
			defer model.Close()
			service, err := NewRPService(s, semanticFixtureProvider(t, model), "chat_completions")
			if err != nil {
				t.Fatal(err)
			}
			request := core.RPInteractionRequest{PrincipalID: player.PrincipalID, SessionID: player.SessionID, Text: "移动物件，然后说「好了。」", Mode: "AUTO", ExpectedCursor: view.ObservationCursor, IdempotencyKey: "change-before-speech"}
			run, _, err := service.ensureRPInteractionWith(ctx, request, service.provider)
			if err != nil {
				t.Fatal(err)
			}
			if changeAfterAction {
				run, err = service.prepareRPInteractionStep(ctx, request, run)
				if err != nil {
					t.Fatal(err)
				}
				outcome, pending, err := service.executeRPInteractionStep(ctx, run, service.provider)
				if err != nil || pending {
					t.Fatalf("first action: %+v %v", outcome, err)
				}
				run, err = service.commitRPInteractionStep(ctx, run, outcome)
				if err != nil || run.NextStep != 1 {
					t.Fatalf("first action receipt: %+v %v", run, err)
				}
			}
			current, err := s.ObserveRPSession(ctx, player)
			if err != nil {
				t.Fatal(err)
			}
			moved := false
			for _, destination := range current.ReachablePlaces {
				if !destination.CanMoveNow {
					continue
				}
				if _, err := s.MoveRP(ctx, core.RPMoveRequest{PrincipalID: player.PrincipalID, SessionID: player.SessionID, FromPlaceID: current.PlaceID, ToPlaceID: destination.PlaceID, ExpectedCursor: current.ObservationCursor, IdempotencyKey: "change-world-move"}); err != nil {
					t.Fatal(err)
				}
				moved = true
				break
			}
			if !moved {
				t.Fatal("fixture lacks an immediate reachable destination")
			}
			result, err := service.RunRPInteraction(ctx, request)
			if err != nil || result.Status != "paused" || len(result.Outcomes) != len(run.Outcomes) || calls.Load() != 1 {
				t.Fatalf("stale plan was not paused: %+v %v; model calls=%d", result, err, calls.Load())
			}
			assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_utterances WHERE speaker_entity_id=? AND speech_text='好了。'`, []any{M2RPPlayerID}, 0)
			wantAnchor := from
			if changeAfterAction {
				wantAnchor = to
			}
			assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_objects WHERE object_id=? AND physical_state='placed' AND anchor_id=?`, []any{item.ObjectID, wantAnchor}, 1)
		})
	}
}

func TestRPInteractionNonverbalPersistsChildAndStopCannotEraseAcceptedFact(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "interaction-nonverbal.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	_, player, view := newRPWaitTestSession(t, ctx, s)
	if len(view.PresentEntities) == 0 {
		t.Fatal("fixture must contain a visibly present target")
	}
	target := view.PresentEntities[0].EntityID
	var calls atomic.Int32
	model := semanticFixtureServer(t, func(string) string {
		return semanticModelReply("ACTION", []core.RPInteractionStep{{Kind: "nonverbal", NonverbalAction: "look_at", TargetEntityID: target}}, "")
	}, &calls)
	defer model.Close()
	service, err := NewRPService(s, semanticFixtureProvider(t, model), "chat_completions")
	if err != nil {
		t.Fatal(err)
	}
	request := core.RPInteractionRequest{PrincipalID: player.PrincipalID, SessionID: player.SessionID, Text: "我望了她一眼，没有说话。", Mode: "AUTO", ExpectedCursor: view.ObservationCursor, IdempotencyKey: "typed-look-at"}
	s.afterRPInteractionStep = func(int) error { return core.NewError(core.CodeInjectedFailure, "lost response after neutral child") }
	if _, err := service.RunRPInteraction(ctx, request); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatalf("expected child receipt loss, got %v", err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_interaction_pending_actions WHERE kind='nonverbal'`, nil, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='RPNonverbalAction' AND actor_id=?`, []any{M2RPPlayerID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_utterances WHERE speaker_entity_id=?`, []any{M2RPPlayerID}, 0)
	if _, err := service.StopRPInteraction(ctx, RPInteractionResumeRequest{PrincipalID: player.PrincipalID, SessionID: player.SessionID, IdempotencyKey: request.IdempotencyKey}); !core.HasCode(err, core.CodeCommandInProgress) {
		t.Fatalf("stop erased accepted neutral child: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	service, err = NewRPService(s, core.DeterministicRPDecisionProvider{}, "deterministic")
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.ResumeRPInteraction(ctx, RPInteractionResumeRequest{PrincipalID: player.PrincipalID, SessionID: player.SessionID, IdempotencyKey: request.IdempotencyKey})
	if err != nil || result.Status != "settled" || len(result.Outcomes) != 1 || result.Outcomes[0].Kind != "nonverbal" || calls.Load() != 1 {
		t.Fatalf("pinned child did not recover after restart: %+v %v model calls=%d", result, err, calls.Load())
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_interaction_pending_actions`, nil, 0)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='RPNonverbalAction' AND actor_id=?`, []any{M2RPPlayerID}, 1)
	events, err := s.ReadRPEvents(ctx, RPEventsReadRequest{PrincipalID: player.PrincipalID, SessionID: player.SessionID})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, event := range events.Events {
		if event.OwnAction != nil && event.OwnAction.Kind == "nonverbal" {
			found = true
			if event.OwnAction.TargetEntityID != target {
				t.Fatalf("own neutral action lost public target: %+v", event.OwnAction)
			}
		}
	}
	if !found {
		t.Fatal("client event reader omitted own neutral action")
	}
}

func TestRPInteractionStopRetiresUnacceptedTypedChild(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "interaction-typed-stop.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, player, view := newRPWaitTestSession(t, ctx, s)
	var calls atomic.Int32
	model := semanticFixtureServer(t, func(string) string {
		return semanticModelReply("ACTION", []core.RPInteractionStep{{Kind: "nonverbal", NonverbalAction: "smile"}}, "")
	}, &calls)
	defer model.Close()
	service, err := NewRPService(s, semanticFixtureProvider(t, model), "chat_completions")
	if err != nil {
		t.Fatal(err)
	}
	request := core.RPInteractionRequest{PrincipalID: player.PrincipalID, SessionID: player.SessionID, Text: "我笑了笑。", Mode: "AUTO", ExpectedCursor: view.ObservationCursor, IdempotencyKey: "typed-stop"}
	run, _, err := service.ensureRPInteractionWith(ctx, request, service.provider)
	if err != nil {
		t.Fatal(err)
	}
	run, err = service.prepareRPInteractionStep(ctx, request, run)
	if err != nil || run.PendingKind != "nonverbal" {
		t.Fatalf("child not durably pinned: %+v %v", run, err)
	}
	var child core.RPNonverbalRequest
	if err := json.Unmarshal([]byte(run.PendingRequest), &child); err != nil {
		t.Fatal(err)
	}
	stopped, err := service.StopRPInteraction(ctx, RPInteractionResumeRequest{PrincipalID: player.PrincipalID, SessionID: player.SessionID, IdempotencyKey: request.IdempotencyKey})
	if err != nil || stopped.Status != "stopped" {
		t.Fatalf("unaccepted typed child did not retire: %+v %v", stopped, err)
	}
	if _, err := s.NonverbalRP(ctx, child); !core.HasCode(err, core.CodeRequestRetired) {
		t.Fatalf("retired child became a world fact: %v", err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_interaction_pending_actions`, nil, 0)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='RPNonverbalAction'`, nil, 0)
	if calls.Load() != 1 {
		t.Fatalf("stopped plan reinterpreted %d times", calls.Load())
	}
}
