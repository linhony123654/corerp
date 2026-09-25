package storage

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"corerp.local/backend/internal/core"
)

func TestRPSharedSpeechWaitsForHumanAndRecoversAcceptedEvent(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "shared-action.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Close() }()
	human, humanRead, _ := newRPWaitTestSession(t, ctx, s)
	for _, p := range []struct{ id, kind string }{{"principal_action_operator", "operator"}, {"principal_action_service", "service"}} {
		if _, err := s.db.ExecContext(ctx, `INSERT INTO principals(principal_id,principal_type,display_name,status) VALUES (?,?,?,'active')`, p.id, p.kind, p.id); err != nil {
			t.Fatal(err)
		}
	}
	head := func() int64 {
		var n int64
		if err := s.db.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	binding := func(key string) core.CareerBinding {
		return core.CareerBinding{PrincipalID: "principal_action_operator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: head(), IdempotencyKey: key}
	}
	if _, err := s.EnrollRPExternalControllerLocal(ctx, RPExternalControllerEnrollmentRequest{Binding: binding("enroll-shared-action"), EntityID: M2AgentAdaID, ControllerPrincipalID: "principal_action_service", ControllerInstanceID: "controller-shared-action"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AssignRPExternalControllerLocal(ctx, RPExternalControllerAssignmentRequest{Binding: binding("assign-shared-action"), EntityID: M2AgentAdaID, ExpectedGeneration: 0}); err != nil {
		t.Fatal(err)
	}
	external, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: "principal_action_service", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2AgentAdaID, POV: "second_person", IdempotencyKey: "shared-action-session"})
	if err != nil {
		t.Fatal(err)
	}
	externalRead := core.RPSessionReadRequest{PrincipalID: "principal_action_service", SessionID: external.SessionID}
	for _, read := range []core.RPSessionReadRequest{humanRead, externalRead} {
		if _, err := s.ObserveRPSession(ctx, read); err != nil {
			t.Fatal(err)
		}
	}
	var baseline string
	if err := s.db.QueryRowContext(ctx, `SELECT current_world_time FROM world_clocks WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&baseline); err != nil {
		t.Fatal(err)
	}
	start, err := time.Parse(time.RFC3339, baseline)
	if err != nil {
		t.Fatal(err)
	}
	round, err := s.OpenRPSharedRoundLocal(ctx, RPSharedRoundOpenRequest{Binding: binding("open-shared-action"), HumanSessionID: human.SessionID, ExternalSessionIDs: []string{external.SessionID}})
	if err != nil {
		t.Fatal(err)
	}
	initialHead := head()
	proposal := RPSharedSpeechRequest{PrincipalID: externalRead.PrincipalID, SessionID: externalRead.SessionID, RoundID: round.RoundID, Text: "我先说一句。", IdempotencyKey: "propose-speech"}
	if submitted, err := s.SubmitRPSharedSpeech(ctx, proposal); err != nil || submitted.Submitted != 1 || submitted.OwnDisposition != "submitted" {
		t.Fatal("private proposal", submitted, err)
	}
	if head() != initialHead {
		t.Fatal("proposal wrote a world Event before Human answered")
	}
	if replay, err := s.SubmitRPSharedSpeech(ctx, proposal); err != nil || !replay.Replayed {
		t.Fatal("proposal retry", replay, err)
	}
	changed := proposal
	changed.Text = "换一句。"
	if _, err := s.SubmitRPSharedSpeech(ctx, changed); !core.HasCode(err, core.CodeIdempotencyMismatch) {
		t.Fatal("changed proposal reused key", err)
	}
	advance := RPSharedRoundAdvanceRequest{RPSharedRoundReadRequest: RPSharedRoundReadRequest{PrincipalID: externalRead.PrincipalID, SessionID: externalRead.SessionID, RoundID: round.RoundID}, Budget: 100}
	if _, err := s.AdvanceRPSharedRound(ctx, advance); !core.HasCode(err, core.CodeCommandInProgress) {
		t.Fatal("external action bypassed Human", err)
	}
	if _, err := s.SpeakRP(ctx, core.RPSpeechRequest{PrincipalID: externalRead.PrincipalID, SessionID: externalRead.SessionID, Text: "偷偷说。", ExpectedCursor: initialHead, IdempotencyKey: "bypass-shared-action"}); !core.HasCode(err, core.CodeCommandInProgress) {
		t.Fatal("direct speech bypassed decision window", err)
	}
	if _, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: humanRead.PrincipalID, SessionID: humanRead.SessionID, TargetWorldTime: start.Add(10 * time.Minute).Format(time.RFC3339), ExpectedCursor: initialHead, Budget: 100, IdempotencyKey: "human-bypass"}); !core.HasCode(err, core.CodeCommandInProgress) {
		t.Fatal("direct Human wait bypassed decision window", err)
	}
	if _, err := s.SubmitRPSharedWait(ctx, RPSharedWaitRequest{PrincipalID: externalRead.PrincipalID, SessionID: externalRead.SessionID, RoundID: round.RoundID, HorizonWorldTime: start.Add(10 * time.Minute).Format(time.RFC3339), IdempotencyKey: "double-submit"}); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatal("action and wait both accepted", err)
	}
	if _, err := s.SubmitRPSharedWait(ctx, RPSharedWaitRequest{PrincipalID: humanRead.PrincipalID, SessionID: humanRead.SessionID, RoundID: round.RoundID, HorizonWorldTime: start.Add(10 * time.Minute).Format(time.RFC3339), IdempotencyKey: "human-waits-action"}); err != nil {
		t.Fatal(err)
	}
	selectionFailure := errors.New("lose response after durable selection")
	s.afterRPSharedActionStage = func(stage string) error {
		if stage == "selection_committed" {
			return selectionFailure
		}
		return nil
	}
	if _, err := s.AdvanceRPSharedRound(ctx, advance); !errors.Is(err, selectionFailure) {
		t.Fatal("did not interrupt after pinned selection", err)
	}
	if head() != initialHead {
		t.Fatal("selection alone wrote a world Event")
	}
	if _, err := s.SpeakRP(ctx, core.RPSpeechRequest{PrincipalID: externalRead.PrincipalID, SessionID: externalRead.SessionID, Text: "选中后偷换内容。", SpeechAct: "statement", ExpectedCursor: initialHead, IdempotencyKey: "shared_action_" + round.RoundID}); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatal("selected owner changed private action payload", err)
	}
	failure := errors.New("lose response after sourced speech")
	s.afterRPSharedActionStage = func(stage string) error {
		if stage == "speech_event_committed" {
			return failure
		}
		return nil
	}
	if _, err := s.AdvanceRPSharedRound(ctx, advance); !errors.Is(err, failure) {
		t.Fatal("did not interrupt after accepted Event", err)
	}
	if head() != initialHead+1 {
		t.Fatal("selected speech was not sourced exactly once")
	}
	s.afterRPSharedActionStage = nil
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewRPService(s, core.DeterministicRPDecisionProvider{}, "deterministic")
	if err != nil {
		t.Fatal(err)
	}
	settled, err := service.AdvanceRPSharedRound(ctx, advance)
	if err != nil || settled.Status != "settled" || settled.OwnDisposition != "action_accepted" || settled.EventSequence != initialHead+1 || settled.CurrentWorldTime != baseline {
		t.Fatal("restarted action settlement", settled, err)
	}
	if replay, err := service.AdvanceRPSharedRound(ctx, advance); err != nil || !replay.Replayed || replay.EventSequence != settled.EventSequence {
		t.Fatal("settled action replay", replay, err)
	}
	humanReceipt, err := s.ReadRPSharedRound(ctx, RPSharedRoundReadRequest{PrincipalID: humanRead.PrincipalID, SessionID: humanRead.SessionID, RoundID: round.RoundID})
	if err != nil || humanReceipt.OwnDisposition != "deferred_no_effect" {
		t.Fatal("Human wait was not explicitly deferred", humanReceipt, err)
	}
	assertRPWorldTime(t, ctx, s, baseline)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPSpeechAccepted' AND event_sequence=?`, []any{M2DemoInstanceID, M2DemoBranchID, initialHead + 1}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPWaitCompleted'`, []any{M2DemoInstanceID, M2DemoBranchID}, 0)
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("action projection diverged", diff, err)
	}
	for _, read := range []core.RPSessionReadRequest{humanRead, externalRead} {
		if _, err := s.ObserveRPSession(ctx, read); err != nil {
			t.Fatal(err)
		}
	}
	staleRound, err := s.OpenRPSharedRoundLocal(ctx, RPSharedRoundOpenRequest{Binding: binding("open-stale-action"), HumanSessionID: human.SessionID, ExternalSessionIDs: []string{external.SessionID}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SubmitRPSharedSpeech(ctx, RPSharedSpeechRequest{PrincipalID: externalRead.PrincipalID, SessionID: externalRead.SessionID, RoundID: staleRound.RoundID, Text: "本轮将失效。", IdempotencyKey: "stale-action"}); err != nil {
		t.Fatal(err)
	}
	// Human explicitly chooses another action, so the anti-starvation wait
	// rule must not convert this round into a time advance.
	if _, err := s.SubmitRPSharedSpeech(ctx, RPSharedSpeechRequest{PrincipalID: humanRead.PrincipalID, SessionID: humanRead.SessionID, RoundID: staleRound.RoundID, Text: "Human 也要说话。", IdempotencyKey: "stale-human-speech"}); err != nil {
		t.Fatal(err)
	}
	s.afterRPSharedActionStage = func(stage string) error {
		if stage == "selection_committed" {
			return selectionFailure
		}
		return nil
	}
	staleAdvance := advance
	staleAdvance.RoundID = staleRound.RoundID
	if _, err := s.AdvanceRPSharedRound(ctx, staleAdvance); !errors.Is(err, selectionFailure) {
		t.Fatal("did not pin stale selection", err)
	}
	s.afterRPSharedActionStage = nil
	if _, err := s.MaterializeRPLocation(ctx, RPLocationMaterializeRequest{
		Binding:          core.CareerBinding{PrincipalID: "principal_creator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: head(), IdempotencyKey: "stale-action-world-change"},
		ParentLocationID: M2AgentCafeID, SlotKey: "stale-action-world-change",
		Candidate: RPLocationCandidate{DisplayName: "窗口外变化", GeneratorVersion: "local-v1"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AdvanceRPSharedRound(ctx, staleAdvance); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatal("changed baseline did not stale unaccepted selected action", err)
	}
	staleReceipt, err := s.ReadRPSharedRound(ctx, RPSharedRoundReadRequest{PrincipalID: externalRead.PrincipalID, SessionID: externalRead.SessionID, RoundID: staleRound.RoundID})
	if err != nil || staleReceipt.Status != "stale" || staleReceipt.CurrentWorldTime != baseline {
		t.Fatal("stale action receipt and frozen time", staleReceipt, err)
	}
	for _, read := range []core.RPSessionReadRequest{humanRead, externalRead} {
		if _, err := s.ObserveRPSession(ctx, read); err != nil {
			t.Fatal(err)
		}
	}
	next, err := s.OpenRPSharedRoundLocal(ctx, RPSharedRoundOpenRequest{Binding: binding("after-stale-action"), HumanSessionID: human.SessionID, ExternalSessionIDs: []string{external.SessionID}})
	if err != nil || next.Status != "open" {
		t.Fatal("stale action retained branch slot", next, err)
	}
	if _, err := s.MaterializeRPLocation(ctx, RPLocationMaterializeRequest{
		Binding:          core.CareerBinding{PrincipalID: "principal_creator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: head(), IdempotencyKey: "close-next-action-window"},
		ParentLocationID: M2AgentCafeID, SlotKey: "close-next-action-window",
		Candidate: RPLocationCandidate{DisplayName: "另一个窗口外变化", GeneratorVersion: "local-v1"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SubmitRPSharedSpeech(ctx, RPSharedSpeechRequest{PrincipalID: externalRead.PrincipalID, SessionID: externalRead.SessionID, RoundID: next.RoundID, Text: "不能进入旧窗口", IdempotencyKey: "stale-next"}); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatal("second baseline was not marked stale", err)
	}
	if _, err := s.ReleaseRPExternalControllerLocal(ctx, RPExternalControllerReleaseRequest{Binding: binding("release-after-action"), EntityID: M2AgentAdaID, ExpectedGeneration: 1}); err != nil {
		t.Fatal("release after settled action", err)
	}
	if old, err := s.SubmitRPSharedSpeech(ctx, proposal); err != nil || !old.Replayed || old.OwnDisposition != "action_accepted" || old.CurrentWorldTime != baseline {
		t.Fatal("released controller lost exact action proposal receipt", old, err)
	}
	if old, err := s.AdvanceRPSharedRound(ctx, advance); err != nil || !old.Replayed || old.OwnDisposition != "action_accepted" {
		t.Fatal("released controller lost exact action settlement", old, err)
	}
	humanAfter, err := s.ObserveRPSession(ctx, humanRead)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: humanRead.PrincipalID, SessionID: humanRead.SessionID, TargetWorldTime: start.Add(time.Minute).Format(time.RFC3339), ExpectedCursor: humanAfter.ObservationCursor, Budget: 100, IdempotencyKey: "human-after-action-release"}); err != nil {
		t.Fatal("Human time advance after release", err)
	}
	if old, err := s.ReadRPSharedRound(ctx, RPSharedRoundReadRequest{PrincipalID: externalRead.PrincipalID, SessionID: externalRead.SessionID, RoundID: round.RoundID}); err != nil || old.CurrentWorldTime != baseline {
		t.Fatal("released controller watched later time through action receipt", old, err)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("release after action projection diverged", diff, err)
	}
	// Restore the on-disk 040 shape, then upgrade to 041 without changing
	// the accepted speech receipt after a later Human time advance.
	for _, statement := range []string{
		`ALTER TABLE rp_shared_round_actions DROP COLUMN action_kind`,
		`ALTER TABLE rp_shared_rounds DROP COLUMN selected_action_kind`,
		`DELETE FROM schema_meta WHERE schema_version='corerp-f3-shared-move-rounds-041-2026-09-25'`,
	} {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			t.Fatalf("restore 040 shape %q: %v", statement, err)
		}
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal("041 upgrade", err)
	}
	if old, err := s.ReadRPSharedRound(ctx, RPSharedRoundReadRequest{PrincipalID: externalRead.PrincipalID, SessionID: externalRead.SessionID, RoundID: round.RoundID}); err != nil || old.Status != "settled" || old.CurrentWorldTime != baseline || old.OwnDisposition != "action_accepted" {
		t.Fatal("040 speech receipt changed on 041 upgrade", old, err)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("041 upgrade projection diverged", diff, err)
	}
}

func TestRPSharedSpeechAdoptsAcceptedTurnAndRepliesOnlyFromInternalNPC(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "shared-turn.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Close() }()
	human, humanRead, _ := newRPWaitTestSession(t, ctx, s)
	for _, p := range []struct{ id, kind string }{{"principal_turn_operator", "operator"}, {"principal_turn_service_a", "service"}, {"principal_turn_service_b", "service"}} {
		if _, err := s.db.ExecContext(ctx, `INSERT INTO principals(principal_id,principal_type,display_name,status) VALUES (?,?,?,'active')`, p.id, p.kind, p.id); err != nil {
			t.Fatal(err)
		}
	}
	head := func() int64 {
		var n int64
		if err := s.db.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	binding := func(key string) core.CareerBinding {
		return core.CareerBinding{PrincipalID: "principal_turn_operator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: head(), IdempotencyKey: key}
	}
	reads := []core.RPSessionReadRequest{humanRead}
	for i, entity := range []string{M2AgentAdaID, M2AgentBoID} {
		principal := []string{"principal_turn_service_a", "principal_turn_service_b"}[i]
		controller := []string{"controller-turn-a", "controller-turn-b"}[i]
		if _, err := s.EnrollRPExternalControllerLocal(ctx, RPExternalControllerEnrollmentRequest{Binding: binding("enroll-" + controller), EntityID: entity, ControllerPrincipalID: principal, ControllerInstanceID: controller}); err != nil {
			t.Fatal(err)
		}
		if _, err := s.AssignRPExternalControllerLocal(ctx, RPExternalControllerAssignmentRequest{Binding: binding("assign-" + controller), EntityID: entity, ExpectedGeneration: 0}); err != nil {
			t.Fatal(err)
		}
		session, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: principal, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: entity, POV: "second_person", IdempotencyKey: "session-" + controller})
		if err != nil {
			t.Fatal(err)
		}
		read := core.RPSessionReadRequest{PrincipalID: principal, SessionID: session.SessionID}
		view, err := s.ObserveRPSession(ctx, read)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.MoveRP(ctx, core.RPMoveRequest{PrincipalID: principal, SessionID: session.SessionID, FromPlaceID: view.PlaceID, ToPlaceID: M2AgentCafeID, ExpectedCursor: view.ObservationCursor, IdempotencyKey: "meet-at-cafe-" + controller}); err != nil {
			t.Fatal(err)
		}
		reads = append(reads, read)
	}
	for _, read := range reads {
		if _, err := s.ObserveRPSession(ctx, read); err != nil {
			t.Fatal(err)
		}
	}
	var baseline string
	if err := s.db.QueryRowContext(ctx, `SELECT current_world_time FROM world_clocks WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&baseline); err != nil {
		t.Fatal(err)
	}
	start, err := time.Parse(time.RFC3339, baseline)
	if err != nil {
		t.Fatal(err)
	}
	round, err := s.OpenRPSharedRoundLocal(ctx, RPSharedRoundOpenRequest{Binding: binding("open-shared-npc-turn"), HumanSessionID: human.SessionID, ExternalSessionIDs: []string{reads[1].SessionID, reads[2].SessionID}})
	if err != nil {
		t.Fatal(err)
	}
	before := head()
	proposal := RPSharedSpeechRequest{PrincipalID: reads[1].PrincipalID, SessionID: reads[1].SessionID, RoundID: round.RoundID, Text: "咖啡馆里大家好。", IdempotencyKey: "turn-proposal"}
	if _, err := s.SubmitRPSharedSpeech(ctx, proposal); err != nil {
		t.Fatal(err)
	}
	derivedKey := "shared_action_" + round.RoundID
	retired, err := s.RetireRPRequest(ctx, RPRequestRetireRequest{PrincipalID: reads[1].PrincipalID, SessionID: reads[1].SessionID, Operation: "dialogue", IdempotencyKey: derivedKey})
	if err != nil || retired.Status != "in_progress" {
		t.Fatal("round-owned accepted proposal looked retireable", retired, err)
	}
	for _, read := range []core.RPSessionReadRequest{reads[2], humanRead} {
		if _, err := s.SubmitRPSharedWait(ctx, RPSharedWaitRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, RoundID: round.RoundID, HorizonWorldTime: start.Add(10 * time.Minute).Format(time.RFC3339), IdempotencyKey: "wait-" + read.PrincipalID}); err != nil {
			t.Fatal(err)
		}
	}
	turnFailure := core.NewError(core.CodeInjectedFailure, "lose selected turn after speech")
	s.afterRPTurnStage = func(stage string) error {
		if stage == "player_event_committed" {
			return turnFailure
		}
		return nil
	}
	advance := RPSharedRoundAdvanceRequest{RPSharedRoundReadRequest: RPSharedRoundReadRequest{PrincipalID: reads[2].PrincipalID, SessionID: reads[2].SessionID, RoundID: round.RoundID}, Budget: 100}
	if _, err := s.AdvanceRPSharedRound(ctx, advance); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatal("accepted speech did not interrupt before NPC response", err)
	}
	if head() != before+1 {
		t.Fatal("interrupted turn duplicated or missed speaker Event")
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_turn_runs WHERE session_id=? AND idempotency_key=? AND status='open'`, []any{reads[1].SessionID, derivedKey}, 1)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	providerCalls := 0
	provider := rpDecisionProviderFunc(func(ctx context.Context, input core.RPDecisionInput) (core.RPDecisionProposal, error) {
		providerCalls++
		return core.DeterministicRPDecisionProvider{}.Propose(ctx, input)
	})
	service, err := NewRPService(s, provider, "deterministic")
	if err != nil {
		t.Fatal(err)
	}
	settled, err := service.AdvanceRPSharedRound(ctx, advance)
	if err != nil || settled.Status != "settled" || settled.EventSequence != before+1 || settled.OwnDisposition != "deferred_no_effect" || settled.CurrentWorldTime != baseline || providerCalls != 1 {
		t.Fatal("selected turn did not settle one internal NPC reply", settled, providerCalls, err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='RPSpeechAccepted' AND actor_id=? AND event_sequence=?`, []any{M2AgentAdaID, before + 1}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_npc_decisions d JOIN rp_turn_runs t ON t.player_turn_id=d.parent_turn_id WHERE t.session_id=? AND t.idempotency_key=? AND d.npc_entity_id=?`, []any{reads[1].SessionID, derivedKey, M2RPNPCID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_npc_decisions d JOIN rp_turn_runs t ON t.player_turn_id=d.parent_turn_id WHERE t.session_id=? AND t.idempotency_key=? AND d.npc_entity_id IN (?,?)`, []any{reads[1].SessionID, derivedKey, M2AgentBoID, M2RPPlayerID}, 0)
	if replay, err := service.AdvanceRPSharedRound(ctx, advance); err != nil || !replay.Replayed || providerCalls != 1 {
		t.Fatal("round replay recalled provider or repeated effects", replay, providerCalls, err)
	}
	retired, err = s.RetireRPRequest(ctx, RPRequestRetireRequest{PrincipalID: reads[1].PrincipalID, SessionID: reads[1].SessionID, Operation: "dialogue", IdempotencyKey: derivedKey})
	if err != nil || retired.Status != "completed" {
		t.Fatal("settled selected turn receipt missing", retired, err)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("adopted NPC turn projection diverged", diff, err)
	}
	segment, err := s.MaterializeRPLocation(ctx, RPLocationMaterializeRequest{
		Binding:          core.CareerBinding{PrincipalID: "principal_creator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: head(), IdempotencyKey: "shared-turn-journey-segment"},
		ParentLocationID: M2AgentCafeID, SlotKey: "shared-turn-journey-segment",
		Candidate: RPLocationCandidate{DisplayName: "共享回合旅程路段", GeneratorVersion: "local-v1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DefineRPTimedEdge(ctx, RPTimedEdgeRequest{
		Binding:     core.CareerBinding{PrincipalID: "principal_creator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: head(), IdempotencyKey: "shared-turn-journey-edge"},
		FromPlaceID: M2AgentCafeID, ToPlaceID: "place_m2_home_ada", SegmentPlaceID: segment.Fact.LocationID, DurationMinutes: 15,
	}); err != nil {
		t.Fatal(err)
	}
	humanView, err := s.ObserveRPSession(ctx, humanRead)
	if err != nil {
		t.Fatal(err)
	}
	journey, err := s.StartRPJourney(ctx, core.RPMoveRequest{PrincipalID: humanRead.PrincipalID, SessionID: humanRead.SessionID, FromPlaceID: humanView.PlaceID, ToPlaceID: "place_m2_home_ada", ExpectedCursor: humanView.ObservationCursor, IdempotencyKey: "shared-turn-human-journey"})
	if err != nil {
		t.Fatal(err)
	}
	for _, read := range reads {
		if _, err := s.ObserveRPSession(ctx, read); err != nil {
			t.Fatal(err)
		}
	}
	fairRound, err := s.OpenRPSharedRoundLocal(ctx, RPSharedRoundOpenRequest{Binding: binding("open-scheduler-fairness"), HumanSessionID: human.SessionID, ExternalSessionIDs: []string{reads[1].SessionID, reads[2].SessionID}})
	if err != nil {
		t.Fatal(err)
	}
	beforeFair := head()
	for _, read := range reads[1:] {
		if _, err := s.SubmitRPSharedSpeech(ctx, RPSharedSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, RoundID: fairRound.RoundID, Text: "还想继续说话。", IdempotencyKey: "repeat-" + read.PrincipalID}); err != nil {
			t.Fatal(err)
		}
	}
	if head() != beforeFair {
		t.Fatal("repeated external proposals wrote an Event")
	}
	if _, err := s.SubmitRPSharedWait(ctx, RPSharedWaitRequest{PrincipalID: humanRead.PrincipalID, SessionID: humanRead.SessionID, RoundID: fairRound.RoundID, HorizonWorldTime: start.Add(30 * time.Minute).Format(time.RFC3339), IdempotencyKey: "human-waits-past-arrival"}); err != nil {
		t.Fatal(err)
	}
	fairAdvance := advance
	fairAdvance.RoundID = fairRound.RoundID
	fair, err := service.AdvanceRPSharedRound(ctx, fairAdvance)
	if err != nil || fair.Status != "settled" || fair.CurrentWorldTime != journey.ScheduledArrivalAt || fair.OwnDisposition != "deferred_no_effect" {
		t.Fatal("model action starved nearest Human-authorized scheduler boundary", fair, journey, err)
	}
	assertRPWorldTime(t, ctx, s, journey.ScheduledArrivalAt)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_journeys WHERE journey_id=? AND status='arrived'`, []any{journey.JourneyID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='RPWaitCompleted'`, nil, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='RPSpeechAccepted' AND actor_id=?`, []any{M2AgentAdaID}, 1)
	if replay, err := service.AdvanceRPSharedRound(ctx, fairAdvance); err != nil || !replay.Replayed || replay.EventSequence != fair.EventSequence {
		t.Fatal("mixed scheduler boundary replay duplicated effect", replay, err)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("mixed due-scheduler projection diverged", diff, err)
	}
}
