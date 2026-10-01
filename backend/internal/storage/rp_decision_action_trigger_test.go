package storage

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
)

// Pin application orchestration around a real already-committed action.
// Parent orchestration tests own the public route and recovery transitions.
func rpActionDecisionFixture(t *testing.T) (*Store, RPSession, RPNonverbalResult, core.RPDecisionRequest) {
	t.Helper()
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "action-decision.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	session, read, initial := newRPWaitTestSession(t, ctx, s)
	action, err := s.NonverbalRP(ctx, core.RPNonverbalRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "real-targeted-nod", Action: "nod", TargetEntityID: M2RPNPCID})
	if err != nil {
		t.Fatal(err)
	}
	listeners, _ := json.Marshal([]string{M2RPNPCID})
	if _, err := s.db.ExecContext(ctx, `INSERT INTO rp_turn_runs(turn_run_id,session_id,idempotency_key,player_speech_key,request_hash,request_json,status,player_event_id,listener_ids_json,created_at_utc,updated_at_utc,execution_mode,responder_limit,trigger_kind) VALUES ('action-fixture-run',?,'action-fixture-key','unused-action-speech-key','fixture-hash','{}','npc_deciding',?,?,?,?,'orchestrated',1,'nonverbal')`, session.SessionID, action.EventID, string(listeners), action.WorldTime, action.WorldTime); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO rp_turn_listener_activations(turn_run_id,npc_entity_id,disposition,reason_code,activation_rank) VALUES ('action-fixture-run',?,'activated','direct_action',0)`, M2RPNPCID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE rp_sessions SET turn_cursor=?,turn_state='action_committed' WHERE session_id=?`, action.EventID, session.SessionID); err != nil {
		t.Fatal(err)
	}
	session.TurnCursor, session.TurnState = action.EventID, "action_committed"
	return s, session, action, core.RPDecisionRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, TurnID: action.EventID, NPCEntityID: M2RPNPCID}
}

func TestRPDecisionActionTriggerRealSourceCommitRollbackAndReplay(t *testing.T) {
	ctx := context.Background()
	s, session, action, request := rpActionDecisionFixture(t)
	input, err := s.BuildRPDecisionInput(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if input.Trigger == nil || input.Trigger.Kind != "nonverbal" || input.Trigger.SourceEventID != action.EventID || input.TurnID != action.EventID || input.ObservedPlayerAction == nil || input.SpeechEventID != "" || input.PlayerSpeechText != "" || input.PlayerSpeechWorldTime != "" {
		t.Fatalf("action invented speech or lost trigger: %+v", input)
	}
	observed := *input.ObservedPlayerAction
	if observed != (core.RPDecisionObservedAction{SourceEventID: action.EventID, ActorEntityID: session.ControlledEntityID, TargetEntityID: M2RPNPCID, Action: "nod", PlaceID: M2AgentCafeID, WorldTime: action.WorldTime}) {
		t.Fatalf("action provenance: %+v", observed)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_utterances`, nil, 0)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='RPSpeechAccepted'`, nil, 0)
	var offered core.RPDecisionInput
	decision, err := s.DecideRP(ctx, request, rpDecisionProviderFunc(func(_ context.Context, in core.RPDecisionInput) (core.RPDecisionProposal, error) {
		offered = in
		return core.RPDecisionProposal{Action: "respond", Text: "我看见你向我点头了。", ExpressionCode: "nod", Private: &core.RPDecisionPrivate{Intent: "回应亲眼看见的点头", BasisEventIDs: []string{action.EventID}}}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	if offered.ObservedPlayerAction == nil || offered.ObservedPlayerAction.SourceEventID != action.EventID || offered.PlayerSpeechText != "" || offered.Presentation == nil || !core.RPDecisionEvidenceEventIDs(offered)[action.EventID] {
		t.Fatalf("actual provider boundary lost action: %+v", offered)
	}
	s.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "action rollback") }
	if _, err := s.CommitRPDecision(ctx, request, decision); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatalf("missing rollback: %v", err)
	}
	s.beforeCommit = nil
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_npc_decisions`, nil, 0)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_id=?`, []any{action.EventID}, 1)
	committed, err := s.CommitRPDecision(ctx, request, decision)
	if err != nil {
		t.Fatal(err)
	}
	if committed.ParentTurnID != action.EventID || committed.EventSequence != action.EventSequence+1 {
		t.Fatalf("NPC action parent: %+v", committed)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_utterances WHERE speaker_entity_id=?`, []any{session.ControlledEntityID}, 0)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_utterances WHERE speaker_entity_id=?`, []any{M2RPNPCID}, 1)
	conn, err := s.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateRPActionTurnContinuation(ctx, conn, session, action.EventID); err != nil {
		t.Fatalf("own complete NPC+expression batch fenced: %v", err)
	}
	conn.Close()
	replayed, err := s.CommitRPDecision(ctx, request, decision)
	if err != nil || !replayed.Replayed || replayed.EventID != committed.EventID {
		t.Fatalf("action replay duplicated effects: %+v %v", replayed, err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_npc_decisions WHERE parent_turn_id=?`, []any{action.EventID}, 1)
}

func TestRPDecisionActionTriggerRejectsUnpinnedAndForgedWitness(t *testing.T) {
	ctx := context.Background()
	s, session, action, _ := rpActionDecisionFixture(t)
	for _, damage := range []struct {
		name, sql string
		args      []any
	}{
		{"wrong phase", `UPDATE rp_turn_runs SET status='player_committed' WHERE turn_run_id='action-fixture-run'`, nil},
		{"inactive responder", `UPDATE rp_turn_listener_activations SET disposition='not_activated',reason_code='responder_limit',activation_rank=NULL WHERE turn_run_id='action-fixture-run'`, nil},
		{"wrong kind", `UPDATE rp_turn_runs SET trigger_kind='speech',player_turn_id=NULL,status='open',player_event_id=NULL WHERE turn_run_id='action-fixture-run'`, nil},
		{"claim mismatch", `UPDATE agent_knowledge SET claim_payload=json_set(claim_payload,'$.action','shake_head') WHERE source_event_id=? AND observer_agent_id=?`, []any{action.EventID, M2RPNPCID}},
		{"wrong source session", `UPDATE events SET payload=json_set(payload,'$.session_id','other-session') WHERE event_id=?`, []any{action.EventID}},
		{"wrong source actor", `UPDATE events SET actor_id='entity_m2_agent_bo' WHERE event_id=?`, []any{action.EventID}},
	} {
		t.Run(damage.name, func(t *testing.T) {
			tx, err := beginImmediate(ctx, s.db)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			if _, err := tx.conn.ExecContext(ctx, damage.sql, damage.args...); err != nil {
				t.Fatal(err)
			}
			if _, err := readRPDecisionActionTrigger(ctx, tx.conn, session, action.EventID, M2RPNPCID); err == nil {
				t.Fatal("invalid pinned trigger accepted")
			}
		})
	}
	t.Run("both claims forged", func(t *testing.T) {
		tx, err := beginImmediate(ctx, s.db)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		for _, table := range []string{"agent_knowledge", "observation_records"} {
			if _, err := tx.conn.ExecContext(ctx, `UPDATE `+table+` SET claim_payload=json_set(claim_payload,'$.action','shake_head','$.description','有人摇了摇头。') WHERE source_event_id=? AND observer_agent_id=?`, action.EventID, M2RPNPCID); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := readRPDecisionActionTrigger(ctx, tx.conn, session, action.EventID, M2RPNPCID); !core.HasCode(err, core.CodeProjectionDiverged) {
			t.Fatalf("both forged trigger claims accepted: %v", err)
		}
	})
	t.Run("unactivated bystander", func(t *testing.T) {
		conn, err := s.db.Conn(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		if _, err := readRPDecisionActionTrigger(ctx, conn, session, action.EventID, M2AgentBoID); !core.HasCode(err, core.CodeBranchConflict) {
			t.Fatalf("bystander activated: %v", err)
		}
	})
	t.Run("no recorded target remains unknown", func(t *testing.T) {
		tx, err := beginImmediate(ctx, s.db)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		for _, table := range []string{"agent_knowledge", "observation_records"} {
			if _, err := tx.conn.ExecContext(ctx, `UPDATE `+table+` SET claim_payload=json_remove(claim_payload,'$.target_entity_id') WHERE source_event_id=? AND observer_agent_id=?`, action.EventID, M2RPNPCID); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := readRPDecisionActionTrigger(ctx, tx.conn, session, action.EventID, M2RPNPCID); !core.HasCode(err, core.CodeProjectionDiverged) {
			t.Fatalf("source raw target filled missing observation: %v", err)
		}
	})
}

func TestRPDecisionActionTriggerFencesUnrelatedEventBeforeCommit(t *testing.T) {
	ctx := context.Background()
	s, session, action, request := rpActionDecisionFixture(t)
	decision, err := s.DecideRP(ctx, request, rpDecisionProviderFunc(func(context.Context, core.RPDecisionInput) (core.RPDecisionProposal, error) {
		return core.RPDecisionProposal{Action: "silence"}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	// Normal owners reject intervening writes while this application turn is
	// pending. Simulate stale orchestration after another owner legitimately
	// committed: release only the fixture's application fence, commit a real
	// creator fact, then restore the stale stage. No world Event is fabricated.
	if _, err := s.db.ExecContext(ctx, `UPDATE rp_turn_runs SET status='settled',settled_sequence=?,settled_at_utc=? WHERE turn_run_id='action-fixture-run'`, action.EventSequence, action.WorldTime); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DefineRPPerceptionLink(ctx, RPPerceptionLinkRequest{Binding: careerTestBinding(t, s, "principal_creator", "action-unrelated-fence"), PlaceID: M2AgentCafeID, ZoneA: "far", ZoneB: "main", BarrierKind: "open", BarrierState: "open", DistanceM: 1, VisualRangeM: 5, AudioRangeM: 5}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE rp_turn_runs SET status='npc_deciding',settled_sequence=NULL,settled_at_utc=NULL WHERE turn_run_id='action-fixture-run'`); err != nil {
		t.Fatal(err)
	}
	conn, err := s.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateRPActionTurnContinuation(ctx, conn, session, action.EventID); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatalf("unrelated fact did not fence: %v", err)
	}
	conn.Close()
	if _, err := s.CommitRPDecision(ctx, request, decision); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatalf("stale action decision committed: %v", err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_npc_decisions WHERE parent_turn_id=?`, []any{action.EventID}, 0)
}
