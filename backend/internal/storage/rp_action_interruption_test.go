package storage

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"corerp.local/backend/internal/core"
)

func TestRPActionInterruptionControllerBarrierSettles(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "action-interruption.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	_, read, _ := newRPWaitTestSession(t, ctx, s)
	for _, row := range []struct{ id, kind string }{{"principal_interrupt_operator", "operator"}, {"principal_interrupt_service", "service"}} {
		if _, err := s.db.ExecContext(ctx, `INSERT INTO principals(principal_id,principal_type,display_name,status) VALUES (?,?,?,'active')`, row.id, row.kind, row.id); err != nil {
			t.Fatal(err)
		}
	}
	binding := func(key string) core.CareerBinding {
		var head int64
		if err := s.db.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&head); err != nil {
			t.Fatal(err)
		}
		return core.CareerBinding{PrincipalID: "principal_interrupt_operator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: head, IdempotencyKey: key}
	}
	if _, err := s.EnrollRPExternalControllerLocal(ctx, RPExternalControllerEnrollmentRequest{Binding: binding("enroll-interrupt"), EntityID: M2RPNPCID, ControllerPrincipalID: "principal_interrupt_service", ControllerInstanceID: "controller-interrupt"}); err != nil {
		t.Fatal(err)
	}
	view, err := s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	request := core.RPNonverbalRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, Action: "nod", TargetEntityID: M2RPNPCID, ExpectedCursor: view.ObservationCursor, IdempotencyKey: "barrier-action"}
	entered, release := make(chan struct{}), make(chan struct{})
	type result struct {
		turn RPTurnResult
		err  error
	}
	done := make(chan result, 1)
	go func() {
		turn, err := s.RunRPNonverbalTurn(ctx, request, rpDecisionProviderFunc(func(ctx context.Context, in core.RPDecisionInput) (core.RPDecisionProposal, error) {
			close(entered)
			select {
			case <-release:
			case <-ctx.Done():
				return core.RPDecisionProposal{}, ctx.Err()
			}
			return core.RPDecisionProposal{Action: "respond", Text: "这句话不能提交。", SpeechTone: "firm"}, nil
		}))
		done <- result{turn, err}
	}()
	select {
	case <-entered:
	case <-time.After(20 * time.Second):
		t.Fatal("provider barrier not reached")
	}
	assignment, assignErr := s.AssignRPExternalControllerLocal(ctx, RPExternalControllerAssignmentRequest{Binding: binding("assign-interrupt"), EntityID: M2RPNPCID, ExpectedGeneration: 0})
	close(release)
	var out result
	select {
	case out = <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("action did not leave provider barrier")
	}
	if assignErr != nil {
		t.Fatal("authorized handoff failed", assignErr)
	}
	t.Logf("assignment=%s sequence=%d turn_status=%s error=%v", assignment.EventID, assignment.EventSequence, out.turn.Status, out.err)
	retired, retireErr := s.RetireRPRequest(ctx, RPRequestRetireRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, Operation: "nonverbal", IdempotencyKey: request.IdempotencyKey})
	_, closeErr := s.CloseRPSession(ctx, read)
	t.Logf("retirement=%s error=%v close_error=%v", retired.Status, retireErr, closeErr)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='RPNonverbalAction' AND actor_id=?`, []any{M2RPPlayerID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_npc_decisions WHERE npc_entity_id=?`, []any{M2RPNPCID}, 0)
	if out.err != nil || out.turn.Status != "settled" {
		t.Fatalf("accepted action must settle committed observables after handoff: %+v %v", out.turn, out.err)
	}
	if out.turn.Interruption == nil || out.turn.Interruption.Code != "world_changed" || retired.Status != "completed" || closeErr != nil {
		t.Fatal("missing terminal recovery", out.turn, retired, closeErr)
	}
	public, _ := json.Marshal(out.turn)
	for _, secret := range []string{assignment.EventID, "RPExternalControllerAssigned", "principal_interrupt_service", "controller-interrupt", "这句话不能提交。"} {
		if strings.Contains(string(public), secret) {
			t.Fatalf("public interruption exposed %q", secret)
		}
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		saved, err := s.ResumeRPTurn(ctx, RPTurnResumeRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, IdempotencyKey: request.IdempotencyKey}, nil)
		if err != nil || !saved.Replayed || saved.Interruption == nil || !reflect.DeepEqual(saved.NarrativeLines, out.turn.NarrativeLines) {
			t.Fatal("restart/repeat changed interrupted receipt", saved, err)
		}
	}
	rp080AssertFK(t, ctx, s)

}

func rpInterruptionEnroll(t *testing.T, ctx context.Context, s *Store, world, branch, target string) func() (RPExternalControllerAssignment, error) {
	t.Helper()
	for _, row := range []struct{ id, kind string }{{"principal_interrupt_operator", "operator"}, {"principal_interrupt_service", "service"}} {
		if _, err := s.db.ExecContext(ctx, `INSERT INTO principals(principal_id,principal_type,display_name,status) VALUES (?,?,?,'active')`, row.id, row.kind, row.id); err != nil {
			t.Fatal(err)
		}
	}
	binding := func(key string) core.CareerBinding {
		var head int64
		if err := s.db.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, world, branch).Scan(&head); err != nil {
			t.Fatal(err)
		}
		return core.CareerBinding{PrincipalID: "principal_interrupt_operator", InstanceID: world, BranchID: branch, ExpectedHead: head, IdempotencyKey: key}
	}
	if _, err := s.EnrollRPExternalControllerLocal(ctx, RPExternalControllerEnrollmentRequest{Binding: binding("interrupt-enroll"), EntityID: target, ControllerPrincipalID: "principal_interrupt_service", ControllerInstanceID: "controller-interrupt"}); err != nil {
		t.Fatal(err)
	}
	return func() (RPExternalControllerAssignment, error) {
		return s.AssignRPExternalControllerLocal(ctx, RPExternalControllerAssignmentRequest{Binding: binding("interrupt-assign"), EntityID: target, ExpectedGeneration: 0})
	}
}

func TestRPActionInterruptionPreservesCompleteSpeechExpressionToneAndAnonymousIdentity(t *testing.T) {
	ctx := context.Background()
	f := newRPFocusFixture(t, "legacy", false)
	assign := rpInterruptionEnroll(t, ctx, f.s, f.world, "br_main", f.ids[1])
	view, err := f.s.ObserveRPSession(ctx, f.read)
	if err != nil {
		t.Fatal(err)
	}
	request := core.RPNonverbalRequest{PrincipalID: f.read.PrincipalID, SessionID: f.read.SessionID, Action: "smile", ExpectedCursor: view.ObservationCursor, IdempotencyKey: "partial-legacy"}
	calls := 0
	out, err := f.s.RunRPNonverbalTurn(ctx, request, rpDecisionProviderFunc(func(_ context.Context, in core.RPDecisionInput) (core.RPDecisionProposal, error) {
		calls++
		if calls == 1 {
			return core.RPDecisionProposal{Action: "respond", Text: "已经听见的话。", SpeechTone: "gentle", ExpressionCode: "nod", Private: &core.RPDecisionPrivate{Intent: "INTERRUPTION_PRIVATE_SECRET", BasisEventIDs: []string{in.ObservedPlayerAction.SourceEventID}}}, nil
		}
		if _, err := assign(); err != nil {
			return core.RPDecisionProposal{}, err
		}
		return core.RPDecisionProposal{Action: "respond", Text: "不能提交的新话。"}, nil
	}))
	if err != nil || out.Interruption == nil || calls != 2 || len(out.NPCEventIDs) != 1 || out.CompositionVersion != core.RPFactCompositionVersionV3 {
		t.Fatal("partial settlement", out, calls, err)
	}
	assertM2Value(t, ctx, f.s, `SELECT COUNT(*) FROM events WHERE event_type='RPNonverbalAction' AND actor_id=?`, []any{f.ids[0]}, 1)
	assertM2Value(t, ctx, f.s, `SELECT COUNT(*) FROM rp_npc_decisions WHERE session_id=?`, []any{f.session.SessionID}, 1)
	assertM2Value(t, ctx, f.s, `SELECT json_array_length(interrupted_listener_ids_json) FROM rp_turn_runs WHERE turn_run_id=?`, []any{out.TurnRunID}, 1)
	var artifactJSON string
	if err := f.s.db.QueryRowContext(ctx, `SELECT narrative_artifact_json FROM rp_turn_runs WHERE turn_run_id=?`, out.TurnRunID).Scan(&artifactJSON); err != nil {
		t.Fatal(err)
	}
	var artifact core.RPNarrativeArtifact
	if err := json.Unmarshal([]byte(artifactJSON), &artifact); err != nil {
		t.Fatal(err)
	}
	heard, expressed := false, false
	for _, fact := range artifact.Input.Facts {
		heard = heard || fact.Text == "已经听见的话。" && fact.SpeechTone == "gentle"
		expressed = expressed || fact.ExpressionCode == "nod"
	}
	if !heard || !expressed {
		t.Fatal("lost pre-fence public compound effect", artifact.Input.Facts)
	}
	public, _ := json.Marshal(out)
	for _, secret := range append([]string{"INTERRUPTION_PRIVATE_SECRET", "不能提交的新话。", "controller-interrupt"}, f.ids...) {
		if strings.Contains(string(public), secret) {
			t.Fatalf("public anonymous turn leaked %q", secret)
		}
	}
	for i := 0; i < 2; i++ {
		saved, err := f.s.RunRPNonverbalTurn(ctx, request, nil)
		if err != nil || !saved.Replayed || !reflect.DeepEqual(saved.NarrativeLines, out.NarrativeLines) {
			t.Fatal("partial replay", saved, err)
		}
	}
	rp080AssertFK(t, ctx, f.s)
}

type rpInterruptionPlanProvider struct{ propose rpDecisionProviderFunc }

func (p rpInterruptionPlanProvider) Propose(ctx context.Context, in core.RPDecisionInput) (core.RPDecisionProposal, error) {
	return p.propose(ctx, in)
}
func (p rpInterruptionPlanProvider) UnderstandInteraction(_ context.Context, in core.RPInteractionUnderstandingInput) (core.RPInteractionPlan, error) {
	return core.RPInteractionPlan{Mode: "AUTO", Kind: "MIXED", Steps: []core.RPInteractionStep{{Kind: "nonverbal", NonverbalAction: "nod", TargetEntityID: M2RPNPCID}, {Kind: "speech", SpeechText: "后续不能说。"}}}, nil
}

func TestRPActionInterruptionInteractionPausesThenStops(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "interrupt-plan.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, read, _ := newRPWaitTestSession(t, ctx, s)
	assign := rpInterruptionEnroll(t, ctx, s, M2DemoInstanceID, M2DemoBranchID, M2RPNPCID)
	view, err := s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	provider := rpInterruptionPlanProvider{propose: func(_ context.Context, in core.RPDecisionInput) (core.RPDecisionProposal, error) {
		calls++
		if _, err := assign(); err != nil {
			return core.RPDecisionProposal{}, err
		}
		return core.RPDecisionProposal{Action: "silence"}, nil
	}}
	service, err := NewRPService(s, provider, "deterministic")
	if err != nil {
		t.Fatal(err)
	}
	request := core.RPInteractionRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, Text: "点头然后说「后续不能说。」", Mode: "AUTO", ExpectedCursor: view.ObservationCursor, IdempotencyKey: "interrupt-plan"}
	out, err := service.RunRPInteraction(ctx, request)
	if err != nil || out.Status != "paused" || len(out.Outcomes) != 1 || out.Outcomes[0].Interruption == nil || calls != 1 {
		t.Fatal("interrupted child did not pause plan", out, calls, err)
	}
	resume := RPInteractionResumeRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, IdempotencyKey: request.IdempotencyKey}
	saved, err := service.ResumeRPInteraction(ctx, resume)
	if err != nil || saved.Status != "paused" || calls != 1 {
		t.Fatal("paused plan resumed unsafely", saved, err)
	}
	stopped, err := service.StopRPInteraction(ctx, resume)
	if err != nil || stopped.Status != "stopped" || len(stopped.Outcomes) != 1 {
		t.Fatal("stop lost committed child", stopped, err)
	}
	if _, err := s.CloseRPSession(ctx, read); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_utterances WHERE speaker_entity_id=?`, []any{M2RPPlayerID}, 0)
}

func TestRPActionInterruptionPopulated080UpgradeAndRollback(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "upgrade080.db")
	s, _, _ := rp079UpgradeFixture(t, ctx, path)
	if err := s.applyMigration(ctx, "080_rp_action_triggers.sql"); err != nil {
		t.Fatal(err)
	}
	cols := rp080ParentColumns(t, ctx, s)
	before := rp080Snapshot(t, ctx, s, "rp_turn_runs", cols)
	events := rp080Snapshot(t, ctx, s, "events", "*")
	if _, err := s.db.ExecContext(ctx, `CREATE TRIGGER fail_081 BEFORE INSERT ON schema_meta WHEN NEW.schema_version='corerp-rp-action-interruption-081-2026-10-02' BEGIN SELECT RAISE(ABORT,'injected migration failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err := s.applyMigration(ctx, "081_rp_action_interruption.sql"); err == nil {
		t.Fatal("injected migration did not fail")
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM pragma_table_info('rp_turn_runs') WHERE name='interruption_event_id'`, nil, 0)
	if before != rp080Snapshot(t, ctx, s, "rp_turn_runs", cols) || events != rp080Snapshot(t, ctx, s, "events", "*") {
		t.Fatal("rollback changed accepted history")
	}
	rp080AssertFK(t, ctx, s)
	if _, err := s.db.ExecContext(ctx, `DROP TRIGGER fail_081`); err != nil {
		t.Fatal(err)
	}
	if err := s.applyMigration(ctx, "081_rp_action_interruption.sql"); err != nil {
		t.Fatal(err)
	}
	if before != rp080Snapshot(t, ctx, s, "rp_turn_runs", cols) || events != rp080Snapshot(t, ctx, s, "events", "*") {
		t.Fatal("upgrade changed accepted history")
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_turn_runs WHERE interruption_event_id IS NOT NULL OR interrupted_listener_ids_json<>'[]'`, nil, 0)
	if _, err := s.db.ExecContext(ctx, `UPDATE rp_turn_runs SET interruption_event_id='missing-event'`); err == nil {
		t.Fatal("interruption FK not enforced")
	}
	rp080AssertFK(t, ctx, s)
}

func TestRPActionInterruptionSettlementRollbackThenRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "interruption-rollback.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	_, read, _ := newRPWaitTestSession(t, ctx, s)
	assign := rpInterruptionEnroll(t, ctx, s, M2DemoInstanceID, M2DemoBranchID, M2RPNPCID)
	view, err := s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	request := core.RPNonverbalRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, Action: "nod", TargetEntityID: M2RPNPCID, ExpectedCursor: view.ObservationCursor, IdempotencyKey: "rollback-action"}
	_, err = s.RunRPNonverbalTurn(ctx, request, rpDecisionProviderFunc(func(_ context.Context, in core.RPDecisionInput) (core.RPDecisionProposal, error) {
		if _, err := assign(); err != nil {
			return core.RPDecisionProposal{}, err
		}
		s.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "lost historical settlement") }
		return core.RPDecisionProposal{Action: "silence"}, nil
	}))
	if !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatal("settlement failure not returned", err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_turn_runs WHERE interruption_event_id IS NOT NULL OR status='settled'`, nil, 0)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='RPNonverbalAction' AND actor_id=?`, []any{M2RPPlayerID}, 1)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	out, err := s.ResumeRPTurn(ctx, RPTurnResumeRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, IdempotencyKey: request.IdempotencyKey}, nil)
	if err != nil || out.Status != "settled" || out.Interruption == nil {
		t.Fatal("restart did not settle without provider", out, err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_npc_decisions`, nil, 0)
	rp080AssertFK(t, ctx, s)
}

func TestRPActionInterruptionOrphanCommittedBatchFailsClosed(t *testing.T) {
	ctx := context.Background()
	f := newRPFocusFixture(t, "legacy")
	assign := rpInterruptionEnroll(t, ctx, f.s, f.world, "br_main", f.ids[1])
	view, err := f.s.ObserveRPSession(ctx, f.read)
	if err != nil {
		t.Fatal(err)
	}
	request := core.RPNonverbalRequest{PrincipalID: f.read.PrincipalID, SessionID: f.read.SessionID, Action: "nod", ExpectedCursor: view.ObservationCursor, IdempotencyKey: "orphan-action"}
	f.s.afterRPTurnStage = func(stage string) error {
		if stage != "npc_effect_committed" {
			return nil
		}
		if _, err := assign(); err != nil {
			return err
		}
		// Corruption fixture only: production immutability remains enforced.
		if _, err := f.s.db.ExecContext(ctx, `DROP TRIGGER rp_npc_decisions_no_delete`); err != nil {
			return err
		}
		if _, err := f.s.db.ExecContext(ctx, `DELETE FROM rp_npc_decisions WHERE session_id=?`, f.session.SessionID); err != nil {
			return err
		}
		return core.NewError(core.CodeBranchConflict, "interrupt after projection corruption")
	}
	_, err = f.s.RunRPNonverbalTurn(ctx, request, rpDecisionProviderFunc(func(_ context.Context, in core.RPDecisionInput) (core.RPDecisionProposal, error) {
		return core.RPDecisionProposal{Action: "respond", Text: "这句已经提交。", ExpressionCode: "nod"}, nil
	}))
	if !core.HasCode(err, core.CodeProjectionDiverged) {
		t.Fatal("orphan committed batch became unrelated fence", err)
	}
	assertM2Value(t, ctx, f.s, `SELECT COUNT(*) FROM rp_turn_runs WHERE status='settled' OR interruption_event_id IS NOT NULL`, nil, 0)
	assertM2Value(t, ctx, f.s, `SELECT COUNT(*) FROM rp_utterances WHERE speaker_entity_id=? AND speech_text='这句已经提交。'`, []any{f.ids[0]}, 1)
}

func TestRPActionInterruptionRawCommitCrashThenForeignSpeechRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "raw-crash.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	_, read, _ := newRPWaitTestSession(t, ctx, s)
	grantRPControlForTest(t, ctx, s, M2AgentAdaID)
	other, err := s.OpenRPSession(ctx, rpTestOpenRequest())
	if err != nil {
		t.Fatal(err)
	}
	initial, err := s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	request := core.RPNonverbalRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, Action: "nod", TargetEntityID: M2RPNPCID, ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "raw-crash-action"}
	s.afterRPTurnStage = func(stage string) error {
		if stage != "player_event_committed" {
			return nil
		}
		visible, err := s.ObserveRPSession(ctx, core.RPSessionReadRequest{PrincipalID: rpTestPrincipal, SessionID: other.SessionID})
		if err != nil {
			return err
		}
		if _, err := s.SpeakRP(ctx, core.RPSpeechRequest{PrincipalID: rpTestPrincipal, SessionID: other.SessionID, Text: "另一间屋子的真实对白。", ExpectedCursor: visible.ObservationCursor, IdempotencyKey: "foreign-speech"}); err != nil {
			return err
		}
		return core.NewError(core.CodeInjectedFailure, "crash before player action projection binding")
	}
	_, err = s.RunRPNonverbalTurn(ctx, request, core.DeterministicRPDecisionProvider{})
	if !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatal("raw crash/fence not reached", err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_turn_runs WHERE status='open' AND player_event_id IS NULL`, nil, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_turn_listener_activations`, nil, 0)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	out, err := s.ResumeRPTurn(ctx, RPTurnResumeRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, IdempotencyKey: request.IdempotencyKey}, nil)
	if err != nil || out.Status != "settled" || out.Interruption == nil {
		t.Fatal("accepted raw action remained stranded after crash and fence", out, err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_turn_listener_activations`, nil, 0)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_npc_decisions`, nil, 0)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='RPNonverbalAction' AND actor_id=?`, []any{M2RPPlayerID}, 1)
	if _, err := s.CloseRPSession(ctx, read); err != nil {
		t.Fatal(err)
	}
}

func TestRPActionInterruptionMissingPublicHearingOrWitnessFailsClosed(t *testing.T) {
	for _, kind := range []string{"RPSpeechAccepted", "RPNonverbalAction"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			f := newRPFocusFixture(t, "legacy")
			assign := rpInterruptionEnroll(t, ctx, f.s, f.world, "br_main", f.ids[1])
			view, err := f.s.ObserveRPSession(ctx, f.read)
			if err != nil {
				t.Fatal(err)
			}
			request := core.RPNonverbalRequest{PrincipalID: f.read.PrincipalID, SessionID: f.read.SessionID, Action: "smile", ExpectedCursor: view.ObservationCursor, IdempotencyKey: "missing-public-evidence"}
			f.s.afterRPTurnStage = func(stage string) error {
				if stage != "npc_effect_committed" {
					return nil
				}
				if _, err := assign(); err != nil {
					return err
				}
				_, err := f.s.db.ExecContext(ctx, `DELETE FROM agent_knowledge WHERE observer_agent_id=? AND source_event_id IN(SELECT e.event_id FROM rp_npc_decisions d JOIN events n ON n.event_id=d.event_id JOIN events e ON e.batch_id=n.batch_id WHERE d.session_id=? AND e.event_type=?)`, f.player, f.session.SessionID, kind)
				if err != nil {
					return err
				}
				result, err := f.s.db.ExecContext(ctx, `DELETE FROM observation_records WHERE observer_agent_id=? AND source_event_id IN(SELECT e.event_id FROM rp_npc_decisions d JOIN events n ON n.event_id=d.event_id JOIN events e ON e.batch_id=n.batch_id WHERE d.session_id=? AND e.event_type=?)`, f.player, f.session.SessionID, kind)
				if err != nil {
					return err
				}
				count, _ := result.RowsAffected()
				if count != 1 {
					return core.NewError(core.CodeInjectedFailure, "public projection corruption fixture did not remove exact evidence")
				}
				return core.NewError(core.CodeBranchConflict, "fenced after public observation corruption")
			}
			_, err = f.s.RunRPNonverbalTurn(ctx, request, rpDecisionProviderFunc(func(_ context.Context, in core.RPDecisionInput) (core.RPDecisionProposal, error) {
				return core.RPDecisionProposal{Action: "respond", Text: "保留真实已提交话语。", SpeechTone: "gentle", ExpressionCode: "nod"}, nil
			}))
			if !core.HasCode(err, core.CodeProjectionDiverged) {
				t.Fatal("missing immutable-audience evidence silently dropped", kind, err)
			}
			assertM2Value(t, ctx, f.s, `SELECT COUNT(*) FROM rp_turn_runs WHERE status='settled' OR interruption_event_id IS NOT NULL`, nil, 0)
			assertM2Value(t, ctx, f.s, `SELECT COUNT(*) FROM rp_npc_decisions WHERE session_id=?`, []any{f.session.SessionID}, 1)
		})
	}
}

func TestRPActionInterruptionLaterChapterResetKeepsHistoricalWindow(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "chapter-interruption.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	_, read, _ := newRPWaitTestSession(t, ctx, s)
	assign := rpInterruptionEnroll(t, ctx, s, M2DemoInstanceID, M2DemoBranchID, M2RPNPCID)
	view, err := s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	prior, err := s.SpeakRP(ctx, core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, Text: "听见的话请说出来。", ExpectedCursor: view.ObservationCursor, IdempotencyKey: "window-player-speech"})
	if err != nil {
		t.Fatal(err)
	}
	decisionRequest := core.RPDecisionRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, TurnID: prior.TurnID, NPCEntityID: M2RPNPCID}
	decision, err := s.DecideRP(ctx, decisionRequest, rpDecisionProviderFunc(func(_ context.Context, in core.RPDecisionInput) (core.RPDecisionProposal, error) {
		return core.RPDecisionProposal{Action: "respond", Text: "动作前亲耳听见的窗口事实。", SpeechTone: "gentle"}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	spoken, err := s.CommitRPDecision(ctx, decisionRequest, decision)
	if err != nil {
		t.Fatal(err)
	}
	view, err = s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	request := core.RPNonverbalRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, Action: "nod", TargetEntityID: M2RPNPCID, ExpectedCursor: view.ObservationCursor, IdempotencyKey: "chapter-action"}
	out, err := s.RunRPNonverbalTurn(ctx, request, rpDecisionProviderFunc(func(_ context.Context, in core.RPDecisionInput) (core.RPDecisionProposal, error) {
		if _, err := assign(); err != nil {
			return core.RPDecisionProposal{}, err
		}
		return core.RPDecisionProposal{Action: "silence"}, nil
	}))
	if err != nil || out.Interruption == nil {
		t.Fatal(out, err)
	}
	var raw string
	if err := s.db.QueryRowContext(ctx, `SELECT narrative_artifact_json FROM rp_turn_runs WHERE turn_run_id=?`, out.TurnRunID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var artifact core.RPNarrativeArtifact
	if err := json.Unmarshal([]byte(raw), &artifact); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, fact := range artifact.Input.Facts {
		found = found || fact.EventID == spoken.EventID
	}
	if !found {
		t.Fatal("before-fence window fact absent", artifact.Input.Facts)
	}
	view, err = s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	reset, err := s.StartRPChapter(ctx, RPChapterStartRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, IdempotencyKey: "later-reset"})
	if err != nil || reset.ChapterStartSequence <= out.SettledSequence {
		t.Fatal(reset, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := s.RunRPNonverbalTurn(ctx, request, nil)
	if err != nil || !reflect.DeepEqual(saved.NarrativeLines, out.NarrativeLines) || saved.Interruption == nil {
		t.Fatal("later chapter erased frozen window", saved, err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_session_chapter_resets WHERE session_id=? AND idempotency_key='later-reset'`, []any{read.SessionID}, 1)
}

func TestRPActionInterruptionRejectsDifferentAcceptedParent(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "wrong-parent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, read, view := newRPWaitTestSession(t, ctx, s)
	old, err := s.NonverbalRP(ctx, core.RPNonverbalRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, Action: "nod", TargetEntityID: M2RPNPCID, ExpectedCursor: view.ObservationCursor, IdempotencyKey: "different-accepted-owner"})
	if err != nil {
		t.Fatal(err)
	}
	view, err = s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	request := core.RPNonverbalRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, Action: "nod", TargetEntityID: M2RPNPCID, ExpectedCursor: view.ObservationCursor, IdempotencyKey: "exact-original-owner"}
	s.afterRPTurnStage = func(stage string) error {
		if stage != "player_committed" {
			return nil
		}
		if _, err := s.db.ExecContext(ctx, `UPDATE rp_turn_runs SET player_event_id=? WHERE session_id=? AND idempotency_key=?`, old.EventID, read.SessionID, request.IdempotencyKey); err != nil {
			return err
		}
		return core.NewError(core.CodeBranchConflict, "corrupt parent points to another legal same-session action")
	}
	_, err = s.RunRPNonverbalTurn(ctx, request, core.DeterministicRPDecisionProvider{})
	if !core.HasCode(err, core.CodeProjectionDiverged) {
		t.Fatal("different legal action adopted as request parent", err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_turn_runs WHERE status='settled' OR interruption_event_id IS NOT NULL`, nil, 0)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='RPNonverbalAction' AND actor_id=?`, []any{M2RPPlayerID}, 2)
}
