package storage

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"corerp.local/backend/internal/core"
)

// These tests use the public store orchestration entry and real command owners,
// never a fabricated utterance or a manually pinned action run.
func rpNodAcceptanceRequest(t *testing.T, ctx context.Context, s *Store, key string) (core.RPNonverbalRequest, core.RPSessionReadRequest) {
	t.Helper()
	_, read, initial := newRPWaitTestSession(t, ctx, s)
	return core.RPNonverbalRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, Action: "nod", TargetEntityID: M2RPNPCID, ExpectedCursor: initial.ObservationCursor, IdempotencyKey: key}, read
}
func rpNodAcceptanceCounts(t *testing.T, ctx context.Context, s *Store) string {
	t.Helper()
	var result string
	if err := s.db.QueryRowContext(ctx, `SELECT (SELECT COUNT(*) FROM events)||':'||(SELECT COUNT(*) FROM rp_utterances)||':'||(SELECT COUNT(*) FROM rp_npc_decisions)||':'||(SELECT COUNT(*) FROM rp_provider_calls)`).Scan(&result); err != nil {
		t.Fatal(err)
	}
	return result
}
func TestRPNonverbalTurnAcceptanceCrashRestartExactlyOnce(t *testing.T) {
	for _, cut := range []string{"turn_open", "player_event_committed", "player_committed", "npc_effect_committed", "npc_effects_committed", "narrative_ready"} {
		t.Run(cut, func(t *testing.T) {
			ctx := context.Background()
			path := filepath.Join(t.TempDir(), "nod-recover.db")
			s, err := Open(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = s.Close() }()
			request, read := rpNodAcceptanceRequest(t, ctx, s, "nod-recover")
			calls := 0
			const private = "private-nod-intent-never-public"
			provider := rpDecisionProviderFunc(func(_ context.Context, in core.RPDecisionInput) (core.RPDecisionProposal, error) {
				calls++
				if in.NPCEntityID != M2RPNPCID || in.Trigger == nil || in.Trigger.Kind != "nonverbal" || in.ObservedPlayerAction == nil || in.ObservedPlayerAction.Action != "nod" || in.ObservedPlayerAction.TargetEntityID != in.NPCEntityID || in.PlayerSpeechText != "" || in.SpeechEventID != "" || in.PlayerSpeechWorldTime != "" {
					t.Fatalf("incorrect witnessed action packet: %+v", in)
				}
				return core.RPDecisionProposal{Action: "respond", Text: "我看见你向我点头了。", ExpressionCode: "nod", Private: &core.RPDecisionPrivate{Intent: private, BasisEventIDs: []string{in.Trigger.SourceEventID}}}, nil
			})
			injected := false
			s.afterRPTurnStage = func(stage string) error {
				if stage == cut && !injected {
					injected = true
					return errors.New("injected nod stage crash")
				}
				return nil
			}
			if _, err := s.RunRPNonverbalTurn(ctx, request, provider); err == nil || !injected {
				t.Fatalf("cut %s not reached: injected=%v err=%v", cut, injected, err)
			}
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			s, err = Open(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			result, err := s.ResumeRPTurn(ctx, RPTurnResumeRequest{PrincipalID: request.PrincipalID, SessionID: request.SessionID, IdempotencyKey: request.IdempotencyKey}, provider)
			if err != nil || result.Status != "settled" || result.PlayerEventID == "" || calls != 1 {
				t.Fatalf("recovery: %+v calls=%d err=%v", result, calls, err)
			}
			assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_utterances WHERE speaker_entity_id=?`, []any{M2RPPlayerID}, 0)
			assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='RPNonverbalAction' AND actor_id=?`, []any{M2RPPlayerID}, 1)
			assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_npc_decisions WHERE parent_turn_id=? AND npc_entity_id=?`, []any{result.PlayerEventID, M2RPNPCID}, 1)
			assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_turn_runs WHERE turn_run_id=? AND trigger_kind='nonverbal' AND player_turn_id IS NULL AND player_event_id=?`, []any{result.TurnRunID, result.PlayerEventID}, 1)
			assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_turn_listener_activations WHERE turn_run_id=? AND npc_entity_id=? AND reason_code='direct_action' AND disposition='activated'`, []any{result.TurnRunID, M2RPNPCID}, 1)
			public := strings.Join(result.NarrativeLines, "\n")
			if len(result.NarrativeLines) == 0 || !strings.Contains(public, "点头") || !strings.Contains(public, "我看见你向我点头了。") || strings.Contains(public, private) {
				t.Fatalf("missing sourced public narrative or leaked private: %s", public)
			}
			observation, err := s.ObserveRPSession(ctx, read)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, turn := range observation.RecentTurns {
				if turn.TurnRunID == result.TurnRunID {
					found = true
					if !reflect.DeepEqual(turn.NarrativeLines, result.NarrativeLines) {
						t.Fatal("history changed nod narrative")
					}
				}
			}
			if !found {
				t.Fatal("nod turn absent from history")
			}
			counts := rpNodAcceptanceCounts(t, ctx, s)
			replayed, err := s.RunRPNonverbalTurn(ctx, request, nil)
			if err != nil || !replayed.Replayed || !reflect.DeepEqual(replayed.NarrativeLines, result.NarrativeLines) || calls != 1 || rpNodAcceptanceCounts(t, ctx, s) != counts {
				t.Fatalf("settled retry changed effects/provider: %+v %v", replayed, err)
			}
			changed := request
			changed.TargetEntityID = M2AgentAdaID
			if _, err := s.RunRPNonverbalTurn(ctx, changed, provider); !core.HasCode(err, core.CodeIdempotencyMismatch) {
				t.Fatalf("same key changed target: %v", err)
			}
		})
	}
}
func TestRPNonverbalTurnAcceptanceBystandersRemainWitnessesNotResponders(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "nod-bystanders.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	request, read := rpNodAcceptanceRequest(t, ctx, s, "nod-bystanders")
	if _, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, TargetWorldTime: M2AgentNoonTime, Budget: 10, ExpectedCursor: request.ExpectedCursor, IdempotencyKey: "nod-noon"}); err != nil {
		t.Fatal(err)
	}
	view, err := s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	if len(view.PresentEntities) != 3 {
		t.Fatal("fixture lacks three actual witnesses")
	}
	request.ExpectedCursor = view.ObservationCursor
	calls := 0
	result, err := s.RunRPNonverbalTurn(ctx, request, rpDecisionProviderFunc(func(_ context.Context, in core.RPDecisionInput) (core.RPDecisionProposal, error) {
		calls++
		if in.NPCEntityID != M2RPNPCID {
			t.Fatal("bystander activated")
		}
		return core.RPDecisionProposal{Action: "respond", Text: "我看见这个点头了。"}, nil
	}))
	if err != nil || result.Status != "settled" || calls != 1 {
		t.Fatal(result, calls, err)
	}
	var raw string
	if err := s.db.QueryRowContext(ctx, `SELECT payload FROM events WHERE event_id=?`, result.PlayerEventID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var fact core.RPNonverbalFact
	if err := json.Unmarshal([]byte(raw), &fact); err != nil {
		t.Fatal(err)
	}
	if len(fact.Witnesses) != 3 {
		t.Fatal("reaction eligibility replaced frozen witnesses", fact)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM observation_records WHERE source_event_id=?`, []any{result.PlayerEventID}, 3)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_npc_decisions WHERE parent_turn_id=?`, []any{result.PlayerEventID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_utterances WHERE speaker_entity_id=?`, []any{M2RPPlayerID}, 0)
}
func TestRPNonverbalTurnAcceptanceHumanTargetNeverInternallyActed(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "nod-human.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	request, read := rpNodAcceptanceRequest(t, ctx, s, "nod-human")
	grantRPControlForTest(t, ctx, s, M2RPNPCID)
	own := rpTestOpenRequest()
	own.EntityID = M2RPNPCID
	own.IdempotencyKey = "human-cai"
	if _, err := s.OpenRPSession(ctx, own); err != nil {
		t.Fatal(err)
	}
	view, err := s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	request.ExpectedCursor = view.ObservationCursor
	result, err := s.RunRPNonverbalTurn(ctx, request, rpDecisionProviderFunc(func(context.Context, core.RPDecisionInput) (core.RPDecisionProposal, error) {
		t.Fatal("internal provider acted for human target")
		return core.RPDecisionProposal{}, nil
	}))
	if err != nil || result.Status != "settled" {
		t.Fatal(result, err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_turn_listener_activations WHERE turn_run_id=? AND npc_entity_id=? AND disposition='externally_controlled'`, []any{result.TurnRunID, M2RPNPCID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_npc_decisions WHERE parent_turn_id=?`, []any{result.PlayerEventID}, 0)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='RPNonverbalAction' AND actor_id=?`, []any{M2RPPlayerID}, 1)
}
func TestRPNonverbalTurnAcceptanceFailureKeepsActionAndReportsSilence(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "nod-failure.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	request, _ := rpNodAcceptanceRequest(t, ctx, s, "nod-failed-provider")
	calls := 0
	provider := rpDecisionProviderFunc(func(context.Context, core.RPDecisionInput) (core.RPDecisionProposal, error) {
		calls++
		return core.RPDecisionProposal{}, errors.New("fixture unavailable")
	})
	result, err := s.RunRPNonverbalTurn(ctx, request, provider)
	if err != nil || result.Status != "settled" || calls != 1 {
		t.Fatal(result, calls, err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='RPNonverbalAction' AND actor_id=?`, []any{M2RPPlayerID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_utterances WHERE speaker_entity_id=?`, []any{M2RPPlayerID}, 0)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_npc_decisions WHERE parent_turn_id=? AND action='silence'`, []any{result.PlayerEventID}, 1)
	failures := 0
	for _, call := range result.ProviderCalls {
		if call.Phase == "decision" {
			failures++
			if call.Result == "success" || call.FallbackKind != "silence" {
				t.Fatal("failure hidden", call)
			}
		}
	}
	if failures != 1 {
		t.Fatal("missing failure receipt", result.ProviderCalls)
	}
	counts := rpNodAcceptanceCounts(t, ctx, s)
	if _, err := s.RunRPNonverbalTurn(ctx, request, provider); err != nil || calls != 1 || rpNodAcceptanceCounts(t, ctx, s) != counts {
		t.Fatal("failed-provider settled retry generated new outcomes", calls, err)
	}
}
