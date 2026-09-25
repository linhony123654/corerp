package storage

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"corerp.local/backend/internal/core"
)

type rpSharedRecoveryFixture struct {
	store    *Store
	path     string
	human    core.RPSessionReadRequest
	external core.RPSessionReadRequest
	baseline string
}

func newRPSharedRecoveryFixture(t *testing.T) rpSharedRecoveryFixture {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "shared-recovery.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	_, humanRead, _ := newRPWaitTestSession(t, ctx, s)
	for _, row := range []struct{ id, kind string }{{"principal_recovery_operator", "operator"}, {"principal_recovery_service", "service"}} {
		if _, err := s.db.ExecContext(ctx, `INSERT INTO principals(principal_id,principal_type,display_name,status) VALUES (?,?,?,'active')`, row.id, row.kind, row.id); err != nil {
			t.Fatal(err)
		}
	}
	binding := rpSharedRecoveryBinding(t, s, "enroll-recovery")
	if _, err := s.EnrollRPExternalControllerLocal(ctx, RPExternalControllerEnrollmentRequest{Binding: binding, EntityID: M2AgentAdaID, ControllerPrincipalID: "principal_recovery_service", ControllerInstanceID: "controller-recovery"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AssignRPExternalControllerLocal(ctx, RPExternalControllerAssignmentRequest{Binding: rpSharedRecoveryBinding(t, s, "assign-recovery"), EntityID: M2AgentAdaID, ExpectedGeneration: 0}); err != nil {
		t.Fatal(err)
	}
	external, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: "principal_recovery_service", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2AgentAdaID, POV: "second_person", IdempotencyKey: "open-recovery"})
	if err != nil {
		t.Fatal(err)
	}
	externalRead := core.RPSessionReadRequest{PrincipalID: "principal_recovery_service", SessionID: external.SessionID}
	for _, read := range []core.RPSessionReadRequest{humanRead, externalRead} {
		if _, err := s.ObserveRPSession(ctx, read); err != nil {
			t.Fatal(err)
		}
	}
	var baseline string
	if err := s.db.QueryRowContext(ctx, `SELECT current_world_time FROM world_clocks WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&baseline); err != nil {
		t.Fatal(err)
	}
	return rpSharedRecoveryFixture{s, path, humanRead, externalRead, baseline}
}

func rpSharedRecoveryBinding(t *testing.T, s *Store, key string) core.CareerBinding {
	t.Helper()
	var head int64
	if err := s.db.QueryRowContext(context.Background(), `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&head); err != nil {
		t.Fatal(err)
	}
	return core.CareerBinding{PrincipalID: "principal_recovery_operator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: head, IdempotencyKey: key}
}

func openRPSharedRecoveryWait(t *testing.T, f rpSharedRecoveryFixture, key string) (RPSharedRound, RPSharedRoundAdvanceRequest) {
	t.Helper()
	ctx := context.Background()
	round, err := f.store.OpenRPSharedRoundLocal(ctx, RPSharedRoundOpenRequest{Binding: rpSharedRecoveryBinding(t, f.store, key), HumanSessionID: f.human.SessionID, ExternalSessionIDs: []string{f.external.SessionID}})
	if err != nil {
		t.Fatal(err)
	}
	start, err := time.Parse(time.RFC3339, f.baseline)
	if err != nil {
		t.Fatal(err)
	}
	horizon := start.Add(10 * time.Minute).Format(time.RFC3339)
	for _, read := range []core.RPSessionReadRequest{f.external, f.human} {
		if _, err := f.store.SubmitRPSharedWait(ctx, RPSharedWaitRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, RoundID: round.RoundID, HorizonWorldTime: horizon, IdempotencyKey: "horizon-" + read.PrincipalID}); err != nil {
			t.Fatal(err)
		}
	}
	return round, RPSharedRoundAdvanceRequest{RPSharedRoundReadRequest: RPSharedRoundReadRequest{PrincipalID: f.external.PrincipalID, SessionID: f.external.SessionID, RoundID: round.RoundID}, Budget: 1}
}

func TestRPSharedRoundWaitSelectionStalesBeforeIntentAfterWorldChange(t *testing.T) {
	ctx := context.Background()
	f := newRPSharedRecoveryFixture(t)
	defer f.store.Close()
	round, advance := openRPSharedRecoveryWait(t, f, "round-stale-before-intent")
	lost := errors.New("selection response lost")
	f.store.afterRPSharedActionStage = func(stage string) error {
		if stage == "wait_selection_committed" {
			return lost
		}
		return nil
	}
	if _, err := f.store.AdvanceRPSharedRound(ctx, advance); !errors.Is(err, lost) {
		t.Fatal("wait selection was not persisted", err)
	}
	assertM2Value(t, ctx, f.store, `SELECT COUNT(*) FROM rp_wait_intents WHERE session_id=? AND idempotency_key=?`, []any{f.human.SessionID, "shared_" + round.RoundID}, 0)
	if err := f.store.Close(); err != nil {
		t.Fatal(err)
	}
	s, err := Open(ctx, f.path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.MaterializeRPLocation(ctx, RPLocationMaterializeRequest{Binding: core.CareerBinding{PrincipalID: "principal_creator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: rpSharedRecoveryBinding(t, s, "changed-world").ExpectedHead, IdempotencyKey: "changed-world"}, ParentLocationID: M2AgentCafeID, SlotKey: "changed-world", Candidate: RPLocationCandidate{DisplayName: "轮次外变化", GeneratorVersion: "local-v1"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AdvanceRPSharedRound(ctx, advance); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatal("unaccepted wait kept the active slot", err)
	}
	if view, err := s.ReadRPSharedRound(ctx, advance.RPSharedRoundReadRequest); err != nil || view.Status != "stale" || view.CurrentWorldTime != f.baseline {
		t.Fatal("stale receipt", view, err)
	}
	for _, read := range []core.RPSessionReadRequest{f.human, f.external} {
		if _, err := s.ObserveRPSession(ctx, read); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.OpenRPSharedRoundLocal(ctx, RPSharedRoundOpenRequest{Binding: rpSharedRecoveryBinding(t, s, "new-after-stale"), HumanSessionID: f.human.SessionID, ExternalSessionIDs: []string{f.external.SessionID}}); err != nil {
		t.Fatal("stale wait blocked new decision window", err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPWaitCompleted'`, []any{M2DemoInstanceID, M2DemoBranchID}, 0)
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("recovery projection divergence", diff, err)
	}
}

func TestRPSharedRoundChildKeysReservedBeforeSubmission(t *testing.T) {
	ctx := context.Background()
	f := newRPSharedRecoveryFixture(t)
	defer f.store.Close()
	round, err := f.store.OpenRPSharedRoundLocal(ctx, RPSharedRoundOpenRequest{Binding: rpSharedRecoveryBinding(t, f.store, "round-reserve-keys"), HumanSessionID: f.human.SessionID, ExternalSessionIDs: []string{f.external.SessionID}})
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range []RPRequestRetireRequest{
		{PrincipalID: f.human.PrincipalID, SessionID: f.human.SessionID, Operation: "wait", IdempotencyKey: "shared_" + round.RoundID},
		{PrincipalID: f.external.PrincipalID, SessionID: f.external.SessionID, Operation: "move", IdempotencyKey: "shared_action_" + round.RoundID},
		{PrincipalID: f.external.PrincipalID, SessionID: f.external.SessionID, Operation: "dialogue", IdempotencyKey: "shared_action_" + round.RoundID},
	} {
		if outcome, err := f.store.RetireRPRequest(ctx, candidate); err != nil || outcome.Status != "in_progress" {
			t.Fatal("reserved child key was retired before a proposal", candidate, outcome, err)
		}
	}
	if _, err := f.store.SubmitRPSharedMove(ctx, RPSharedMoveRequest{PrincipalID: f.external.PrincipalID, SessionID: f.external.SessionID, RoundID: round.RoundID, FromPlaceID: "place_m2_home_ada", ToPlaceID: M2AgentCafeID, IdempotencyKey: "move-after-reservation"}); err != nil {
		t.Fatal(err)
	}
	start, _ := time.Parse(time.RFC3339, f.baseline)
	if _, err := f.store.SubmitRPSharedWait(ctx, RPSharedWaitRequest{PrincipalID: f.human.PrincipalID, SessionID: f.human.SessionID, RoundID: round.RoundID, HorizonWorldTime: start.Add(10 * time.Minute).Format(time.RFC3339), IdempotencyKey: "human-after-reservation"}); err != nil {
		t.Fatal(err)
	}
	settled, err := f.store.AdvanceRPSharedRound(ctx, RPSharedRoundAdvanceRequest{RPSharedRoundReadRequest: RPSharedRoundReadRequest{PrincipalID: f.external.PrincipalID, SessionID: f.external.SessionID, RoundID: round.RoundID}, Budget: 100})
	if err != nil || settled.Status != "settled" || settled.OwnDisposition != "action_accepted" {
		t.Fatal("selected move was stranded by retirement", settled, err)
	}
	assertM2Value(t, ctx, f.store, `SELECT COUNT(*) FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPPlayerMoved'`, []any{M2DemoInstanceID, M2DemoBranchID}, 1)
	for _, read := range []core.RPSessionReadRequest{f.human, f.external} {
		if _, err := f.store.ObserveRPSession(ctx, read); err != nil {
			t.Fatal(err)
		}
	}
	binding := rpSharedRecoveryBinding(t, f.store, "pre-retired-future-round")
	idHash, err := core.HashJSON([]string{binding.InstanceID, binding.BranchID, binding.PrincipalID, binding.IdempotencyKey})
	if err != nil {
		t.Fatal(err)
	}
	childKey := "shared_action_rpr_" + idHash[7:]
	if outcome, err := f.store.RetireRPRequest(ctx, RPRequestRetireRequest{PrincipalID: f.external.PrincipalID, SessionID: f.external.SessionID, Operation: "move", IdempotencyKey: childKey}); err != nil || outcome.Status != "retired" {
		t.Fatal("pre-retire future child", outcome, err)
	}
	if _, err := f.store.OpenRPSharedRoundLocal(ctx, RPSharedRoundOpenRequest{Binding: binding, HumanSessionID: f.human.SessionID, ExternalSessionIDs: []string{f.external.SessionID}}); !core.HasCode(err, core.CodeRequestRetired) {
		t.Fatal("open accepted a pre-retired child key", err)
	}
}

func TestRPSharedRoundWaitBudgetRetryAndLegacyIntent(t *testing.T) {
	ctx := context.Background()
	for _, legacy := range []bool{false, true} {
		t.Run(map[bool]string{false: "current", true: "legacy"}[legacy], func(t *testing.T) {
			f := newRPSharedRecoveryFixture(t)
			defer f.store.Close()
			round, advance := openRPSharedRecoveryWait(t, f, "round-budget")
			if legacy {
				lost := errors.New("selected before legacy intent")
				f.store.afterRPSharedActionStage = func(stage string) error {
					if stage == "wait_selection_committed" {
						return lost
					}
					return nil
				}
				if _, err := f.store.AdvanceRPSharedRound(ctx, advance); !errors.Is(err, lost) {
					t.Fatal("wait selection", err)
				}
				f.store.afterRPSharedActionStage = nil
				var target, key, principal string
				var cursor int64
				if err := f.store.db.QueryRowContext(ctx, `SELECT r.advance_target,r.wait_key,r.baseline_head,p.principal_id FROM rp_shared_rounds r JOIN rp_shared_round_participants p ON p.round_id=r.round_id AND p.session_id=r.human_session_id WHERE r.round_id=?`, round.RoundID).Scan(&target, &key, &cursor, &principal); err != nil {
					t.Fatal(err)
				}
				original := core.RPWaitRequest{PrincipalID: principal, SessionID: f.human.SessionID, TargetWorldTime: target, Budget: 7, ExpectedCursor: cursor, IdempotencyKey: key}
				hash, err := core.HashJSON(original)
				if err != nil {
					t.Fatal(err)
				}
				if intent, replayed, err := f.store.ensureRPWaitIntentForRound(ctx, original, hash, round.RoundID); err != nil || replayed || intent.Status != "pending" {
					t.Fatal("legacy accepted intent", intent, replayed, err)
				}
				altered := original
				start, err := time.Parse(time.RFC3339, f.baseline)
				if err != nil {
					t.Fatal(err)
				}
				altered.TargetWorldTime = start.Add(20 * time.Minute).Format(time.RFC3339)
				if _, err := f.store.waitRPForSharedRound(ctx, altered, round.RoundID); !core.HasCode(err, core.CodeIdempotencyMismatch) {
					t.Fatal("legacy hash accepted another target", err)
				}
				advance.Budget = 100
			} else {
				advance.Budget = 1
			}
			service, err := NewRPService(f.store, core.DeterministicRPDecisionProvider{}, "deterministic")
			if err != nil {
				t.Fatal(err)
			}
			settled, err := service.AdvanceRPSharedRound(ctx, advance)
			if err != nil || settled.Status != "settled" {
				t.Fatal("shared wait with changed execution budget", settled, err)
			}
			if err := f.store.Close(); err != nil {
				t.Fatal(err)
			}
			s, err := Open(ctx, f.path)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			service, err = NewRPService(s, core.DeterministicRPDecisionProvider{}, "deterministic")
			if err != nil {
				t.Fatal(err)
			}
			advance.Budget = 999
			if replay, err := service.AdvanceRPSharedRound(ctx, advance); err != nil || !replay.Replayed || replay.EventSequence != settled.EventSequence {
				t.Fatal("fresh budget failed to replay settled round", replay, err)
			}
			assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPWaitCompleted'`, []any{M2DemoInstanceID, M2DemoBranchID}, 1)
			if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
				t.Fatal("shared wait projection divergence", diff, err)
			}
		})
	}
}

func TestRPSharedRoundRejectsPreviouslyAcceptedChildKeys(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "preowned-child.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, human, _ := newRPWaitTestSession(t, ctx, s)
	for _, row := range []struct{ id, kind string }{{"principal_recovery_operator", "operator"}, {"principal_recovery_service", "service"}} {
		if _, err := s.db.ExecContext(ctx, `INSERT INTO principals(principal_id,principal_type,display_name,status) VALUES (?,?,?,'active')`, row.id, row.kind, row.id); err != nil {
			t.Fatal(err)
		}
	}
	futureKey := func(openKey, prefix string) string {
		hash, err := core.HashJSON([]string{M2DemoInstanceID, M2DemoBranchID, "principal_recovery_operator", openKey})
		if err != nil {
			t.Fatal(err)
		}
		return prefix + "rpr_" + hash[7:]
	}
	var baseline string
	if err := s.db.QueryRowContext(ctx, `SELECT current_world_time FROM world_clocks WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&baseline); err != nil {
		t.Fatal(err)
	}
	start, err := time.Parse(time.RFC3339, baseline)
	if err != nil {
		t.Fatal(err)
	}
	observed, err := s.ObserveRPSession(ctx, human)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: human.PrincipalID, SessionID: human.SessionID, TargetWorldTime: start.Add(5 * time.Minute).Format(time.RFC3339), Budget: 100, ExpectedCursor: observed.ObservationCursor, IdempotencyKey: futureKey("preowned-wait", "shared_")}); err != nil {
		t.Fatal("ordinary Human wait before external assignment", err)
	}
	if _, err := s.EnrollRPExternalControllerLocal(ctx, RPExternalControllerEnrollmentRequest{Binding: rpSharedRecoveryBinding(t, s, "enroll-preowned"), EntityID: M2AgentAdaID, ControllerPrincipalID: "principal_recovery_service", ControllerInstanceID: "controller-preowned"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AssignRPExternalControllerLocal(ctx, RPExternalControllerAssignmentRequest{Binding: rpSharedRecoveryBinding(t, s, "assign-preowned"), EntityID: M2AgentAdaID, ExpectedGeneration: 0}); err != nil {
		t.Fatal(err)
	}
	external, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: "principal_recovery_service", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2AgentAdaID, POV: "second_person", IdempotencyKey: "open-preowned"})
	if err != nil {
		t.Fatal(err)
	}
	externalRead := core.RPSessionReadRequest{PrincipalID: "principal_recovery_service", SessionID: external.SessionID}
	for _, read := range []core.RPSessionReadRequest{human, externalRead} {
		if _, err := s.ObserveRPSession(ctx, read); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.OpenRPSharedRoundLocal(ctx, RPSharedRoundOpenRequest{Binding: rpSharedRecoveryBinding(t, s, "preowned-wait"), HumanSessionID: human.SessionID, ExternalSessionIDs: []string{external.SessionID}}); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatal("round reused an unrelated accepted Human wait", err)
	}
	moveView, err := s.ObserveRPSession(ctx, externalRead)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.MoveRP(ctx, core.RPMoveRequest{PrincipalID: externalRead.PrincipalID, SessionID: externalRead.SessionID, FromPlaceID: moveView.PlaceID, ToPlaceID: M2AgentCafeID, ExpectedCursor: moveView.ObservationCursor, IdempotencyKey: futureKey("preowned-move", "shared_action_")}); err != nil {
		t.Fatal("ordinary service move before round", err)
	}
	for _, read := range []core.RPSessionReadRequest{human, externalRead} {
		if _, err := s.ObserveRPSession(ctx, read); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.OpenRPSharedRoundLocal(ctx, RPSharedRoundOpenRequest{Binding: rpSharedRecoveryBinding(t, s, "preowned-move"), HumanSessionID: human.SessionID, ExternalSessionIDs: []string{external.SessionID}}); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatal("round reused an unrelated accepted movement", err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_shared_rounds WHERE status IN ('open','advancing')`, nil, 0)
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("preowned child projection divergence", diff, err)
	}
}
