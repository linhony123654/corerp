package storage

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"corerp.local/backend/internal/core"
)

func TestRPSharedMoveHumanBarrierAcceptedRecoveryAndTimeBoundary(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "shared-move.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Close() }()
	human, humanRead, _ := newRPWaitTestSession(t, ctx, s)
	for _, p := range []struct{ id, kind string }{{"principal_move_operator", "operator"}, {"principal_move_service", "service"}} {
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
		return core.CareerBinding{PrincipalID: "principal_move_operator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: head(), IdempotencyKey: key}
	}
	if _, err := s.EnrollRPExternalControllerLocal(ctx, RPExternalControllerEnrollmentRequest{Binding: binding("enroll-move"), EntityID: M2AgentAdaID, ControllerPrincipalID: "principal_move_service", ControllerInstanceID: "controller-move"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AssignRPExternalControllerLocal(ctx, RPExternalControllerAssignmentRequest{Binding: binding("assign-move"), EntityID: M2AgentAdaID, ExpectedGeneration: 0}); err != nil {
		t.Fatal(err)
	}
	external, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: "principal_move_service", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2AgentAdaID, POV: "second_person", IdempotencyKey: "open-move"})
	if err != nil {
		t.Fatal(err)
	}
	externalRead := core.RPSessionReadRequest{PrincipalID: "principal_move_service", SessionID: external.SessionID}
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
	round, err := s.OpenRPSharedRoundLocal(ctx, RPSharedRoundOpenRequest{Binding: binding("round-move"), HumanSessionID: human.SessionID, ExternalSessionIDs: []string{external.SessionID}})
	if err != nil {
		t.Fatal(err)
	}
	before := head()
	proposal := RPSharedMoveRequest{PrincipalID: externalRead.PrincipalID, SessionID: externalRead.SessionID, RoundID: round.RoundID, FromPlaceID: "place_m2_home_ada", ToPlaceID: M2AgentCafeID, IdempotencyKey: "propose-move"}
	if _, err := s.SubmitRPSharedMove(ctx, proposal); err != nil {
		t.Fatal("valid private move", err)
	}
	if head() != before {
		t.Fatal("proposal changed world before Human authorization")
	}
	advance := RPSharedRoundAdvanceRequest{RPSharedRoundReadRequest: RPSharedRoundReadRequest{PrincipalID: externalRead.PrincipalID, SessionID: externalRead.SessionID, RoundID: round.RoundID}, Budget: 100}
	if _, err := s.AdvanceRPSharedRound(ctx, advance); !core.HasCode(err, core.CodeCommandInProgress) {
		t.Fatal("advanced without Human", err)
	}
	if _, err := s.MoveRP(ctx, core.RPMoveRequest{PrincipalID: externalRead.PrincipalID, SessionID: externalRead.SessionID, FromPlaceID: proposal.FromPlaceID, ToPlaceID: proposal.ToPlaceID, ExpectedCursor: before, IdempotencyKey: "direct-bypass"}); !core.HasCode(err, core.CodeCommandInProgress) {
		t.Fatal("direct move bypassed round", err)
	}
	if _, err := s.SubmitRPSharedMove(ctx, RPSharedMoveRequest{PrincipalID: humanRead.PrincipalID, SessionID: humanRead.SessionID, RoundID: round.RoundID, FromPlaceID: proposal.FromPlaceID, ToPlaceID: proposal.ToPlaceID, IdempotencyKey: "wrong-human-origin"}); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatal("Human claimed another resident's origin", err)
	}
	if _, err := s.SubmitRPSharedWait(ctx, RPSharedWaitRequest{PrincipalID: humanRead.PrincipalID, SessionID: humanRead.SessionID, RoundID: round.RoundID, HorizonWorldTime: start.Add(10 * time.Minute).Format(time.RFC3339), IdempotencyKey: "human-wait-move"}); err != nil {
		t.Fatal(err)
	}
	selectionLost := errors.New("selection response lost")
	s.afterRPSharedActionStage = func(stage string) error {
		if stage == "selection_committed" {
			return selectionLost
		}
		return nil
	}
	if _, err := s.AdvanceRPSharedRound(ctx, advance); !errors.Is(err, selectionLost) || head() != before {
		t.Fatal("selection must persist without movement", err)
	}
	if _, err := s.MoveRP(ctx, core.RPMoveRequest{PrincipalID: proposal.PrincipalID, SessionID: proposal.SessionID, FromPlaceID: proposal.FromPlaceID, ToPlaceID: "place_m2_home_bo", ExpectedCursor: before, IdempotencyKey: "shared_action_" + round.RoundID}); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatal("selected move key altered destination", err)
	}
	if receipt, err := s.RetireRPRequest(ctx, RPRequestRetireRequest{PrincipalID: proposal.PrincipalID, SessionID: proposal.SessionID, Operation: "move", IdempotencyKey: "shared_action_" + round.RoundID}); err != nil || receipt.Status != "in_progress" {
		t.Fatal("selected move key retired before typed acceptance", receipt, err)
	}
	acceptedLost := errors.New("move response lost")
	s.afterRPSharedActionStage = func(stage string) error {
		if stage == "move_event_committed" {
			return acceptedLost
		}
		return nil
	}
	if _, err := s.AdvanceRPSharedRound(ctx, advance); !errors.Is(err, acceptedLost) || head() != before+1 {
		t.Fatal("accepted move interruption", err)
	}
	s.afterRPSharedActionStage = nil
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	settled, err := s.AdvanceRPSharedRound(ctx, advance)
	if err != nil || settled.Status != "settled" || settled.EventSequence != before+1 || settled.CurrentWorldTime != baseline || settled.OwnDisposition != "action_accepted" {
		t.Fatal("recover accepted move", settled, err)
	}
	if again, err := s.SubmitRPSharedMove(ctx, proposal); err != nil || !again.Replayed || again.EventSequence != settled.EventSequence {
		t.Fatal("original proposal receipt", again, err)
	}
	if again, err := s.AdvanceRPSharedRound(ctx, advance); err != nil || !again.Replayed || again.EventSequence != settled.EventSequence {
		t.Fatal("settled exact replay", again, err)
	}
	if humanReceipt, err := s.ReadRPSharedRound(ctx, RPSharedRoundReadRequest{PrincipalID: humanRead.PrincipalID, SessionID: humanRead.SessionID, RoundID: round.RoundID}); err != nil || humanReceipt.OwnDisposition != "deferred_no_effect" {
		t.Fatal("Human wait disposition", humanReceipt, err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='RPPlayerMoved' AND actor_id=? AND event_sequence=?`, []any{M2AgentAdaID, before + 1}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='RPWaitCompleted'`, nil, 0)
	assertRPWorldTime(t, ctx, s, baseline)
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("move projection diverged", diff, err)
	}
	for _, read := range []core.RPSessionReadRequest{humanRead, externalRead} {
		if _, err := s.ObserveRPSession(ctx, read); err != nil {
			t.Fatal(err)
		}
	}
	next, err := s.OpenRPSharedRoundLocal(ctx, RPSharedRoundOpenRequest{Binding: binding("move-then-wait"), HumanSessionID: human.SessionID, ExternalSessionIDs: []string{external.SessionID}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SubmitRPSharedMove(ctx, RPSharedMoveRequest{PrincipalID: externalRead.PrincipalID, SessionID: externalRead.SessionID, RoundID: next.RoundID, FromPlaceID: M2AgentCafeID, ToPlaceID: "place_m2_home_ada", IdempotencyKey: "propose-second-move"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SubmitRPSharedWait(ctx, RPSharedWaitRequest{PrincipalID: humanRead.PrincipalID, SessionID: humanRead.SessionID, RoundID: next.RoundID, HorizonWorldTime: start.Add(10 * time.Minute).Format(time.RFC3339), IdempotencyKey: "human-waits-again"}); err != nil {
		t.Fatal(err)
	}
	advance.RoundID = next.RoundID
	waited, err := s.AdvanceRPSharedRound(ctx, advance)
	if err != nil || waited.Status != "settled" || waited.OwnDisposition != "deferred_no_effect" || waited.CurrentWorldTime != start.Add(10*time.Minute).Format(time.RFC3339) {
		t.Fatal("Human time boundary must win repeated move", waited, err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='RPPlayerMoved' AND actor_id=?`, []any{M2AgentAdaID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='RPWaitCompleted'`, nil, 1)
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("move/wait projections diverged", diff, err)
	}
}
