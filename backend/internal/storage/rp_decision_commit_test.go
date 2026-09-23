package storage

import (
	"context"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPNPCRefusalCommitsSpeechKnowledgeAndRetryAfterRollback(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "rp-npc-refuse.db")
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	session, read, initial := newRPWaitTestSession(t, ctx, store)
	if _, err := store.MoveRP(ctx, core.RPMoveRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID,
		FromPlaceID: M2AgentCafeID, ToPlaceID: "place_m2_home_ada", ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "visit-ada-refusal"}); err != nil {
		t.Fatal(err)
	}
	seen, err := store.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	speech, err := store.SpeakRP(ctx, core.RPSpeechRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID,
		Text: "请借我一些钱。", ExpectedCursor: seen.ObservationCursor, IdempotencyKey: "request-loan"})
	if err != nil {
		t.Fatal(err)
	}
	request := core.RPDecisionRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID, TurnID: speech.TurnID, NPCEntityID: M2AgentAdaID}
	decision, err := store.DecideRP(ctx, request, core.DeterministicRPDecisionProvider{})
	if err != nil || decision.Proposal.Action != "refuse" {
		t.Fatalf("NPC failed to choose refusal: %+v, %v", decision, err)
	}
	store.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "NPC precommit") }
	if _, err := store.CommitRPDecision(ctx, request, decision); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatalf("expected NPC effect rollback: %v", err)
	}
	store.beforeCommit = nil
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM rp_npc_decisions`, nil, 0)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM events WHERE event_type = 'RPSpeechAccepted'`, nil, 1)
	assertM2Value(t, ctx, store, `SELECT head_sequence FROM branches WHERE instance_id = ? AND branch_id = ?`, []any{M2DemoInstanceID, M2DemoBranchID}, speech.EventSequence)
	committed, err := store.CommitRPDecision(ctx, request, decision)
	if err != nil || committed.Action != "refuse" || committed.EventSequence != speech.EventSequence+1 || committed.Replayed {
		t.Fatalf("NPC refusal did not commit: %+v, %v", committed, err)
	}
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM agent_knowledge WHERE source_event_id = ? AND observer_agent_id = ?`, []any{committed.EventID, M2RPPlayerID}, 1)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM rp_utterances WHERE event_id = ? AND speaker_entity_id = ?`, []any{committed.EventID, M2AgentAdaID}, 1)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM outbox WHERE event_id = ? AND topic = 'rp.speech.accepted'`, []any{committed.EventID}, 1)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM events WHERE event_type IN ('PurchaseCommitted', 'CurrencyIssued') AND event_sequence > ?`, []any{speech.EventSequence}, 0)
	state, err := store.ReadRPSession(ctx, read)
	if err != nil || state.TurnState != "npc_effects_committed" || state.TurnCursor != speech.TurnID {
		t.Fatalf("NPC effect did not advance turn stage: %+v, %v", state, err)
	}
	changed := decision
	changed.Proposal.Text = "不同回答。"
	if _, err := store.CommitRPDecision(ctx, request, changed); !core.HasCode(err, core.CodeIdempotencyMismatch) {
		t.Fatalf("different NPC answer reused same turn: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	replayed, err := store.CommitRPDecision(ctx, request, decision)
	if err != nil || !replayed.Replayed || replayed.EventID != committed.EventID {
		t.Fatalf("NPC refusal retry after restart duplicated effect: %+v, %v", replayed, err)
	}
	differences, err := store.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID)
	if err != nil || len(differences) != 0 {
		t.Fatalf("NPC refusal did not replay: %v, %v", differences, err)
	}
}

func TestRPNPCLeaveAndSilenceAreCommittedWithoutInventingPlayerEffects(t *testing.T) {
	for _, action := range []string{"leave", "silence", "wait"} {
		t.Run(action, func(t *testing.T) {
			ctx := context.Background()
			store, err := Open(ctx, filepath.Join(t.TempDir(), "rp-npc-action.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			session, read, initial := newRPWaitTestSession(t, ctx, store)
			speech, err := store.SpeakRP(ctx, core.RPSpeechRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID,
				Text: "你好。", ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "npc-action-prompt"})
			if err != nil {
				t.Fatal(err)
			}
			request := core.RPDecisionRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID, TurnID: speech.TurnID, NPCEntityID: M2RPNPCID}
			proposal := core.RPDecisionProposal{Action: action}
			if action == "leave" {
				proposal.DestinationPlaceID = "place_m2_home_ada"
			}
			decision, err := store.DecideRP(ctx, request, rpDecisionProviderFunc(func(context.Context, core.RPDecisionInput) (core.RPDecisionProposal, error) { return proposal, nil }))
			if err != nil {
				t.Fatal(err)
			}
			committed, err := store.CommitRPDecision(ctx, request, decision)
			if err != nil || committed.Action != action {
				t.Fatalf("NPC action was not committed: %+v, %v", committed, err)
			}
			observed, err := store.ObserveRPSession(ctx, read)
			if err != nil {
				t.Fatal(err)
			}
			if action == "leave" {
				if len(observed.PresentEntities) != 0 {
					t.Fatalf("NPC remained at cafe after leave: %+v", observed)
				}
				assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM agent_movements WHERE event_id = ? AND agent_id = ?`, []any{committed.EventID, M2RPNPCID}, 1)
			} else {
				if len(observed.PresentEntities) != 1 || observed.PresentEntities[0].EntityID != M2RPNPCID {
					t.Fatalf("NPC no-op changed location: %+v", observed)
				}
				assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM agent_movements WHERE event_id = ?`, []any{committed.EventID}, 0)
			}
			assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM outbox WHERE event_id = ? AND topic = 'rp.npc.decision'`, []any{committed.EventID}, 1)
			differences, err := store.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID)
			if err != nil || len(differences) != 0 {
				t.Fatalf("NPC %s effect did not replay: %v, %v", action, differences, err)
			}
		})
	}
}

func TestRPNPCDecisionsForThreeListenersCommitSequentially(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "rp-npc-three.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, read, initial := newRPWaitTestSession(t, ctx, store)
	wait, err := store.WaitRP(ctx, core.RPWaitRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID,
		TargetWorldTime: M2AgentNoonTime, Budget: 10, ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "three-listeners-noon"})
	if err != nil || wait.Status != "completed" {
		t.Fatalf("could not reach shared scene: %+v, %v", wait, err)
	}
	observed, err := store.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	speech, err := store.SpeakRP(ctx, core.RPSpeechRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID,
		Text: "大家好。", ExpectedCursor: observed.ObservationCursor, IdempotencyKey: "three-listeners-greeting"})
	if err != nil || len(speech.ListenerIDs) != 3 {
		t.Fatalf("did not hear three NPCs: %+v, %v", speech, err)
	}
	for _, npcID := range speech.ListenerIDs {
		request := core.RPDecisionRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID, TurnID: speech.TurnID, NPCEntityID: npcID}
		decision, err := store.DecideRP(ctx, request, core.DeterministicRPDecisionProvider{})
		if err != nil || decision.Status != "validated" {
			t.Fatalf("NPC %s did not propose: %+v, %v", npcID, decision, err)
		}
		if _, err := store.CommitRPDecision(ctx, request, decision); err != nil {
			t.Fatalf("NPC %s did not commit: %v", npcID, err)
		}
	}
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM rp_npc_decisions WHERE parent_turn_id = ?`, []any{speech.TurnID}, 3)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM agent_knowledge k JOIN rp_npc_decisions d ON d.event_id = k.source_event_id WHERE d.parent_turn_id = ? AND k.observer_agent_id = ?`, []any{speech.TurnID, M2RPPlayerID}, 3)
	differences, err := store.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID)
	if err != nil || len(differences) != 0 {
		t.Fatalf("three NPC effects did not replay: %v, %v", differences, err)
	}
}

func TestRPNPCDecision023UpgradePreservesTurnAndRetryMismatch(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "rp-npc-upgrade.db")
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	session, read, initial := newRPWaitTestSession(t, ctx, store)
	speech, err := store.SpeakRP(ctx, core.RPSpeechRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID,
		Text: "你好。", ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "upgrade-turn"})
	if err != nil {
		t.Fatal(err)
	}
	request := core.RPDecisionRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID, TurnID: speech.TurnID, NPCEntityID: M2RPNPCID}
	decision, err := store.DecideRP(ctx, request, core.DeterministicRPDecisionProvider{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `DROP TABLE rp_npc_decisions`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `DELETE FROM schema_meta WHERE schema_version = ?`, SchemaVersion); err != nil {
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
	resumed, err := store.ResumeRPSession(ctx, read)
	if err != nil || resumed.TurnCursor != speech.TurnID {
		t.Fatalf("023→024 upgrade lost player turn: %+v, %v", resumed, err)
	}
	if _, err := store.CommitRPDecision(ctx, request, decision); err != nil {
		t.Fatalf("upgraded world could not commit NPC decision: %v", err)
	}
	// A different proposal cannot replace the already-committed NPC effect.
	other := decision
	other.Proposal = core.RPDecisionProposal{Action: "wait"}
	if _, err := store.CommitRPDecision(ctx, request, other); !core.HasCode(err, core.CodeIdempotencyMismatch) {
		t.Fatalf("reusing committed NPC slot with a new effect was accepted: %v", err)
	}
}

func TestRPNPCDecisionRejectsStaleWorldBeforeCommit(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "rp-npc-stale.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, read, initial := newRPWaitTestSession(t, ctx, store)
	speech, err := store.SpeakRP(ctx, core.RPSpeechRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID,
		Text: "你好。", ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "stale-turn"})
	if err != nil {
		t.Fatal(err)
	}
	request := core.RPDecisionRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID, TurnID: speech.TurnID, NPCEntityID: M2RPNPCID}
	decision, err := store.DecideRP(ctx, request, core.DeterministicRPDecisionProvider{})
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := store.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.MoveRP(ctx, core.RPMoveRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID,
		FromPlaceID: M2AgentCafeID, ToPlaceID: "place_m2_home_ada", ExpectedCursor: fresh.ObservationCursor, IdempotencyKey: "leave-before-npc"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitRPDecision(ctx, request, decision); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatalf("stale NPC decision crossed player movement: %v", err)
	}
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM rp_npc_decisions`, nil, 0)
}
