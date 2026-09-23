package storage

import (
	"context"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
)

func newRPWaitTestSession(t *testing.T, ctx context.Context, store *Store) (RPSession, core.RPSessionReadRequest, RPObservation) {
	t.Helper()
	if _, err := store.PrepareRPTravel(ctx); err != nil {
		t.Fatal(err)
	}
	session, err := store.OpenRPSession(ctx, core.RPSessionOpenRequest{
		PrincipalID: M2RPPlayerPrincipal, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID,
		EntityID: M2RPPlayerID, POV: "second_person", IdempotencyKey: "wait-session",
	})
	if err != nil {
		t.Fatal(err)
	}
	read := core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID}
	observation, err := store.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	return session, read, observation
}

func TestRPWaitNoDueAdvancesAuthorityOnceAndSurvivesRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "rp-wait.db")
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	session, read, initial := newRPWaitTestSession(t, ctx, store)
	request := core.RPWaitRequest{
		PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID,
		TargetWorldTime: "2026-09-22T03:00:00Z", Budget: 1,
		ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "quiet-hour",
	}
	result, err := store.WaitRP(ctx, request)
	if err != nil || result.Status != "completed" || result.CurrentWorldTime != request.TargetWorldTime || result.EventSequence != initial.ObservationCursor+1 || result.ProcessedItems != 0 {
		t.Fatalf("wait did not advance by one authoritative event: %+v, %v", result, err)
	}
	assertRPWorldTime(t, ctx, store, request.TargetWorldTime)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM events WHERE event_type = 'RPWaitCompleted'`, nil, 1)
	if _, err := store.WaitRP(ctx, core.RPWaitRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID, TargetWorldTime: "2026-09-22T04:00:00Z", Budget: 1, ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "quiet-hour"}); !core.HasCode(err, core.CodeIdempotencyMismatch) {
		t.Fatalf("mismatched wait retry accepted: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	replayed, err := store.WaitRP(ctx, request)
	if err != nil || !replayed.Replayed || replayed.EventID != result.EventID {
		t.Fatalf("wait retry after reopen duplicated effect: %+v, %v", replayed, err)
	}
	after, err := store.ObserveRPSession(ctx, read)
	if err != nil || after.WorldTime != request.TargetWorldTime || after.ObservationCursor != result.EventSequence {
		t.Fatalf("observation missed committed wait: %+v, %v", after, err)
	}
	differences, err := store.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID)
	if err != nil || len(differences) != 0 {
		t.Fatalf("wait projection did not replay: %v, %v", differences, err)
	}
}

func TestRPWaitBudgetExhaustionRetriesDueSchedulerBeforeFinalClock(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "rp-wait-budget.db")
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	session, read, initial := newRPWaitTestSession(t, ctx, store)
	request := core.RPWaitRequest{
		PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID,
		TargetWorldTime: M2AgentMorningTime, Budget: 1,
		ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "morning",
	}
	first, err := store.WaitRP(ctx, request)
	if err != nil || first.Status != "budget_exhausted" || first.PendingDue == 0 || first.ProcessedItems != 1 {
		t.Fatalf("budget exhaustion falsely completed wait: %+v, %v", first, err)
	}
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM events WHERE event_type = 'RPWaitCompleted'`, nil, 0)
	if _, err := store.MoveRP(ctx, core.RPMoveRequest{
		PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID,
		FromPlaceID: M2AgentCafeID, ToPlaceID: "place_m2_home_ada",
		ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "move-during-wait",
	}); !core.HasCode(err, core.CodeCommandInProgress) {
		t.Fatalf("move interleaved with pending wait: %v", err)
	}
	if _, err := store.CloseRPSession(ctx, read); !core.HasCode(err, core.CodeCommandInProgress) {
		t.Fatalf("closed a session with pending wait: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var final RPWaitResult
	for i := 0; i < 20; i++ {
		final, err = store.WaitRP(ctx, request)
		if err != nil {
			t.Fatal(err)
		}
		if final.Status == "completed" {
			break
		}
	}
	if final.Status != "completed" || final.CurrentWorldTime != request.TargetWorldTime || final.EventID == "" {
		t.Fatalf("wait did not resume to completion: %+v", final)
	}
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM events WHERE event_type = 'RPWaitCompleted'`, nil, 1)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM rp_wait_intents WHERE status = 'completed'`, nil, 1)
	replayed, err := store.WaitRP(ctx, request)
	if err != nil || !replayed.Replayed || replayed.EventID != final.EventID || replayed.ProcessedItems != final.ProcessedItems {
		t.Fatalf("completed wait retry duplicated effect: %+v, %v", replayed, err)
	}
	differences, err := store.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID)
	if err != nil || len(differences) != 0 {
		t.Fatalf("scheduler-backed wait did not replay: %v, %v", differences, err)
	}
}

func TestRPWaitFinalCommitFailureLeavesRecoverableIntent(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "rp-wait-failure.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, _, initial := newRPWaitTestSession(t, ctx, store)
	request := core.RPWaitRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID,
		TargetWorldTime: "2026-09-22T03:00:00Z", Budget: 1,
		ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "fail-once"}
	store.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "wait precommit") }
	if _, err := store.WaitRP(ctx, request); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatalf("expected injected wait failure: %v", err)
	}
	store.beforeCommit = nil
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM rp_wait_intents WHERE status = 'pending'`, nil, 1)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM events WHERE event_type = 'RPWaitCompleted'`, nil, 0)
	assertRPWorldTime(t, ctx, store, m2RPSetupTime)
	result, err := store.WaitRP(ctx, request)
	if err != nil || result.Status != "completed" {
		t.Fatalf("pending wait did not recover: %+v, %v", result, err)
	}
}

func assertRPWorldTime(t *testing.T, ctx context.Context, store *Store, expected string) {
	t.Helper()
	var actual string
	if err := store.db.QueryRowContext(ctx, `SELECT current_world_time FROM world_clocks WHERE instance_id = ? AND branch_id = ?`, M2DemoInstanceID, M2DemoBranchID).Scan(&actual); err != nil {
		t.Fatal(err)
	}
	if actual != expected {
		t.Fatalf("RP world time = %q, want %q", actual, expected)
	}
}
