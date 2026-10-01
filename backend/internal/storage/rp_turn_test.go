package storage

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPTurnRunsTwentyDeterministicTurnsWithTimeMoveRestartAndReplay(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "rp-turn-20.db")
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	session, read, initial := newRPWaitTestSession(t, ctx, store)
	cursor := initial.ObservationCursor
	var firstRequest core.RPSpeechRequest
	var firstResult RPTurnResult
	for index := 0; index < 20; index++ {
		request := core.RPSpeechRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID,
			Text: fmt.Sprintf("第 %d 回合，你好。", index+1), ExpectedCursor: cursor, IdempotencyKey: fmt.Sprintf("play-turn-%02d", index+1)}
		result, err := store.PlayRPTurn(ctx, request)
		if err != nil || result.Status != "settled" || result.PlayerEventID == "" || result.SettledSequence <= cursor || len(result.NarrativeLines) == 0 {
			t.Fatalf("turn %d did not settle: %+v, %v", index+1, result, err)
		}
		if index == 0 {
			firstRequest, firstResult = request, result
		}
		cursor = result.SettledSequence
		if index == 9 {
			if _, err := store.CreateSnapshot(ctx, M2DemoInstanceID, M2DemoBranchID, cursor); err != nil {
				t.Fatalf("ten-turn snapshot failed: %v", err)
			}
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}
			store, err = Open(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			resumed, err := store.ResumeRPSession(ctx, read)
			if err != nil || resumed.ObservationCursor != cursor {
				t.Fatalf("session did not resume after ten turns: %+v, %v", resumed, err)
			}
			wait, err := store.WaitRP(ctx, core.RPWaitRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID,
				TargetWorldTime: M2AgentNoonTime, Budget: 10, ExpectedCursor: cursor, IdempotencyKey: "between-turns-noon"})
			if err != nil || wait.Status != "completed" {
				t.Fatalf("scheduler-backed time failed between turns: %+v, %v", wait, err)
			}
			observed, err := store.ObserveRPSession(ctx, read)
			if err != nil || observed.WorldTime != M2AgentNoonTime || len(observed.PresentEntities) != 3 {
				t.Fatalf("time advance did not yield real presence: %+v, %v", observed, err)
			}
			cursor = observed.ObservationCursor
		}
		if index == 14 {
			if _, err := store.MoveRP(ctx, core.RPMoveRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID,
				FromPlaceID: M2AgentCafeID, ToPlaceID: "place_m2_home_ada", ExpectedCursor: cursor, IdempotencyKey: "between-turns-home"}); err != nil {
				t.Fatal(err)
			}
			observed, err := store.ObserveRPSession(ctx, read)
			if err != nil || observed.PlaceID != "place_m2_home_ada" || len(observed.PresentEntities) != 0 {
				t.Fatalf("between-turn move did not change presence: %+v, %v", observed, err)
			}
			cursor = observed.ObservationCursor
		}
	}
	defer store.Close()
	providerCalls := 0
	counting := rpDecisionProviderFunc(func(ctx context.Context, input core.RPDecisionInput) (core.RPDecisionProposal, error) {
		providerCalls++
		return core.DeterministicRPDecisionProvider{}.Propose(ctx, input)
	})
	replayed, err := store.RunRPTurn(ctx, firstRequest, counting)
	if err != nil || !replayed.Replayed || replayed.PlayerEventID != firstResult.PlayerEventID || replayed.SettledSequence != firstResult.SettledSequence || providerCalls != 0 {
		t.Fatalf("settled turn retry called provider or changed result: %+v, calls=%d, %v", replayed, providerCalls, err)
	}
	withoutProvider, err := store.RunRPTurn(ctx, firstRequest, nil)
	if err != nil || !withoutProvider.Replayed || withoutProvider.PlayerEventID != firstResult.PlayerEventID {
		t.Fatalf("settled world facts still required a provider: %+v, %v", withoutProvider, err)
	}
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM rp_turn_runs WHERE status = 'settled'`, nil, 20)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM rp_utterances WHERE speaker_entity_id = ?`, []any{M2RPPlayerID}, 20)
	differences, err := store.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID)
	if err != nil || len(differences) != 0 {
		t.Fatalf("20-turn current projections diverged: %v, %v", differences, err)
	}
	full, err := store.Replay(ctx, M2DemoInstanceID, M2DemoBranchID, cursor)
	if err != nil {
		t.Fatal(err)
	}
	fromSnapshot, err := store.ReplayFromLatestSnapshot(ctx, M2DemoInstanceID, M2DemoBranchID, cursor)
	if err != nil || fromSnapshot.StateHash != full.StateHash {
		t.Fatalf("20-turn snapshot replay disagrees: %v, full=%s, snap=%s", err, full.StateHash, fromSnapshot.StateHash)
	}
}

func TestRPTurnProviderFailureSettlesSilenceWithoutPromotingFalseClaim(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "rp-turn-fallback.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, _, initial := newRPWaitTestSession(t, ctx, store)
	failing := rpDecisionProviderFunc(func(context.Context, core.RPDecisionInput) (core.RPDecisionProposal, error) {
		return core.RPDecisionProposal{}, errors.New("provider unavailable")
	})
	result, err := store.RunRPTurn(ctx, core.RPSpeechRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID,
		Text: "我有一百万。", ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "fallback-turn"}, failing)
	if err != nil || result.Status != "settled" || result.CompositionVersion != core.RPFactCompositionVersionV2 || len(result.NarrativeLines) != 2 || !strings.Contains(result.NarrativeLines[0], "你说：「我有一百万。」") || !strings.Contains(result.NarrativeLines[1], "没有作答") {
		t.Fatalf("provider failure did not settle to safe narrative: %+v, %v", result, err)
	}
	if len(result.NPCEventIDs) != 1 || !reflect.DeepEqual(result.FactGroups, [][]string{{result.PlayerEventID}, {result.NPCEventIDs[0]}}) {
		t.Fatal("false claim or silence lost source coverage", result)
	}
	saved, err := store.ReadRPNarrative(ctx, RPNarrativeReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID, TurnRunID: result.TurnRunID})
	if err != nil || saved.View.Artifact == nil || len(saved.View.Artifact.Input.Facts) != 2 {
		t.Fatal("fallback canonical artifact missing", saved, err)
	}
	fact := saved.View.Artifact.Input.Facts[1]
	if fact.EventID != result.NPCEventIDs[0] || fact.Action != "silence" || fact.Text != "" || fact.ExpressionCode != "" {
		t.Fatal("failure promoted speech or expression instead of silence", fact)
	}
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM rp_npc_decisions WHERE parent_turn_id = ? AND action = 'silence'`, []any{result.PlayerTurnID}, 1)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM events WHERE event_type IN ('PurchaseCommitted', 'CurrencyIssued') AND event_sequence > ?`, []any{initial.ObservationCursor}, 0)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM audit_records WHERE record_type = 'runtime_diagnostic' AND authority = 'non-authoritative'`, nil, 1)
}

func TestRPTurnSchemaUpgradeFrom024PreservesCommittedSpeechAndNPC(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "rp-turn-upgrade.db")
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	session, read, initial := newRPWaitTestSession(t, ctx, store)
	speech, err := store.SpeakRP(ctx, core.RPSpeechRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID,
		Text: "升级前你好。", ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "before-turn-schema"})
	if err != nil {
		t.Fatal(err)
	}
	decisionRequest := core.RPDecisionRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID, TurnID: speech.TurnID, NPCEntityID: M2RPNPCID}
	decision, err := store.DecideRP(ctx, decisionRequest, core.DeterministicRPDecisionProvider{})
	if err != nil {
		t.Fatal(err)
	}
	committed, err := store.CommitRPDecision(ctx, decisionRequest, decision)
	if err != nil {
		t.Fatal(err)
	}
	removeRPStyleSchemaForUpgradeTest(t, ctx, store)
	if _, err := store.db.ExecContext(ctx, `DROP TABLE rp_turn_listener_activations`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `DELETE FROM schema_meta WHERE schema_version IN (?,?)`, RPTurnActivationSchemaVersion, RPConversationFocusSchemaVersion); err != nil {
		t.Fatal(err)
	}
	// Removing the receipt owner also removes its later dependent presentation schema.
	for _, statement := range []string{`DROP TABLE rp_narrative_selections`, `DROP TABLE rp_narrative_renders`, `DELETE FROM schema_meta WHERE schema_version IN ('corerp-rp-narrative-renders-073-2026-09-28','corerp-rp-narrative-composition-077-2026-10-01','corerp-rp-narrative-artifacts-078-2026-10-01')`} {
		if _, err := store.db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.db.ExecContext(ctx, `DROP TABLE rp_turn_runs`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `DELETE FROM schema_meta WHERE schema_version = ?`, RPTurnSchemaVersion); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM rp_utterances WHERE event_id = ?`, []any{speech.EventID}, 1)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM rp_npc_decisions WHERE event_id = ?`, []any{committed.EventID}, 1)
	observed, err := store.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	result, err := store.PlayRPTurn(ctx, core.RPSpeechRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID,
		Text: "升级后你好。", ExpectedCursor: observed.ObservationCursor, IdempotencyKey: "after-turn-schema"})
	if err != nil || result.Status != "settled" {
		t.Fatalf("024→025 upgrade cannot run a new turn: %+v, %v", result, err)
	}
}

func TestRPTurnRecoversEveryDurableStageWithoutDuplicatingEffects(t *testing.T) {
	for _, stage := range []string{"turn_open", "player_event_committed", "player_committed", "npc_effect_committed", "npc_effects_committed", "narrative_ready"} {
		t.Run(stage, func(t *testing.T) {
			ctx := context.Background()
			path := filepath.Join(t.TempDir(), "rp-turn-crash.db")
			store, err := Open(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			session, read, initial := newRPWaitTestSession(t, ctx, store)
			request := core.RPSpeechRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID,
				Text: "你好，Cai。", ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "crash-turn"}
			store.afterRPTurnStage = func(actual string) error {
				if actual == stage {
					return core.NewError(core.CodeInjectedFailure, "RP turn crash after "+stage)
				}
				return nil
			}
			if _, err := store.PlayRPTurn(ctx, request); !core.HasCode(err, core.CodeInjectedFailure) {
				t.Fatalf("did not inject at stage %s: %v", stage, err)
			}
			if _, err := store.CloseRPSession(ctx, read); !core.HasCode(err, core.CodeCommandInProgress) {
				t.Fatalf("closed session with pending turn: %v", err)
			}
			if _, err := store.MoveRP(ctx, core.RPMoveRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID,
				FromPlaceID: M2AgentCafeID, ToPlaceID: "place_m2_home_ada", ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "interleave"}); !core.HasCode(err, core.CodeCommandInProgress) {
				t.Fatalf("interleaved move with pending turn: %v", err)
			}
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}
			store, err = Open(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			providerCalls := 0
			counting := rpDecisionProviderFunc(func(ctx context.Context, input core.RPDecisionInput) (core.RPDecisionProposal, error) {
				providerCalls++
				return core.DeterministicRPDecisionProvider{}.Propose(ctx, input)
			})
			result, err := store.ResumeRPTurn(ctx, RPTurnResumeRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID, IdempotencyKey: "crash-turn"}, counting)
			if err != nil || result.Status != "settled" || len(result.NPCEventIDs) != 1 || len(result.NarrativeLines) != 2 {
				t.Fatalf("turn did not resume after %s: %+v, %v", stage, result, err)
			}
			if stage == "npc_effect_committed" || stage == "npc_effects_committed" || stage == "narrative_ready" {
				if providerCalls != 0 {
					t.Fatalf("recovery repeated committed NPC provider call at %s: %d", stage, providerCalls)
				}
			}
			assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM rp_utterances WHERE speaker_entity_id = ?`, []any{M2RPPlayerID}, 1)
			assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM rp_npc_decisions WHERE parent_turn_id = ?`, []any{result.PlayerTurnID}, 1)
			assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM agent_knowledge k JOIN rp_npc_decisions d ON d.event_id = k.source_event_id WHERE d.parent_turn_id = ? AND k.observer_agent_id = ?`, []any{result.PlayerTurnID, M2RPPlayerID}, 1)
			assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM outbox WHERE event_id = ? AND published_at_utc IS NULL`, []any{result.PlayerEventID}, 1)
			for _, line := range result.NarrativeLines {
				if strings.Contains(line, "一百万") {
					t.Fatalf("narrative invented a financial claim: %s", line)
				}
			}
			differences, err := store.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID)
			if err != nil || len(differences) != 0 {
				t.Fatalf("recovered turn diverged after %s: %v, %v", stage, differences, err)
			}
		})
	}
}
