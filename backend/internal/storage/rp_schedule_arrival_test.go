package storage

import (
	"context"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPEarlyWorkArrivalDoesNotBlockLaterSchedule(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "early-work.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Close() }()
	session, read, initial := newRPWaitTestSession(t, ctx, s)
	if _, err := s.DefineM2AgentRoutine(ctx, demoRoutineRequest()); err != nil {
		t.Fatal(err)
	}
	initial, err = s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: read.PrincipalID, SessionID: session.SessionID, ExpectedCursor: initial.ObservationCursor, TargetWorldTime: M2AgentNoonTime, Budget: 20, IdempotencyKey: "noon"}); err != nil {
		t.Fatal(err)
	}
	view, err := s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	provider := rpDecisionProviderFunc(func(_ context.Context, input core.RPDecisionInput) (core.RPDecisionProposal, error) {
		if input.NPCEntityID == M2AgentAdaID {
			return core.RPDecisionProposal{Action: "leave", DestinationPlaceID: "place_m2_work_ada"}, nil
		}
		return core.RPDecisionProposal{Action: "silence"}, nil
	})
	early, err := s.RunRPTurn(ctx, core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, Text: "接下来有什么安排？", IdempotencyKey: "early-work"}, provider)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateSnapshot(ctx, M2DemoInstanceID, M2DemoBranchID, early.SettledSequence); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RunAgentLife(ctx, "2026-09-24T08:00:00Z", 20); err != nil {
		t.Fatalf("legal early arrival blocked tomorrow's work: %v", err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_positions WHERE agent_id=? AND place_id='place_m2_work_ada' AND activity_code='work'`, []any{M2AgentAdaID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE actor_id=? AND event_type='AgentActivityStarted'`, []any{M2AgentAdaID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_movements WHERE from_place_id=to_place_id`, nil, 0)
	var head int64
	if err := s.db.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&head); err != nil {
		t.Fatal(err)
	}
	full, err := s.Replay(ctx, M2DemoInstanceID, M2DemoBranchID, head)
	if err != nil {
		t.Fatal(err)
	}
	partial, err := s.ReplayFromLatestSnapshot(ctx, M2DemoInstanceID, M2DemoBranchID, head)
	if err != nil || partial.StateHash != full.StateHash {
		t.Fatalf("snapshot activity replay differs: %s %s %v", full.StateHash, partial.StateHash, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) > 0 {
		t.Fatalf("schedule activity replay differs: %v %v", diff, err)
	}
}

func TestRPEarlyArrivalDoesNotAcceptCorruptedPosition(t *testing.T) {
	ctx := context.Background()
	s := openM2AgentStore(t, ctx, filepath.Join(t.TempDir(), "corrupt-position.db"), false)
	defer s.Close()
	// Deliberately damage only a disposable projection. No matching arrival
	// fact exists, so being at the work target must not waive validation.
	if _, err := s.db.ExecContext(ctx, `UPDATE agent_positions SET place_id='place_m2_work_ada' WHERE agent_id=?`, M2AgentAdaID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RunAgentLife(ctx, M2AgentMorningTime, 1); !core.HasCode(err, core.CodeProjectionDiverged) {
		t.Fatalf("corrupt projection accepted as early arrival: %v", err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='AgentActivityStarted'`, nil, 0)
}
