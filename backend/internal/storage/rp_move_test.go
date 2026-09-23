package storage

import (
	"context"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPMoveUsesRealRouteMovementKnowledgeAndRetry(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "rp-move.db")
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.PrepareRPTravel(ctx); err != nil {
		t.Fatal(err)
	}
	session, err := store.OpenRPSession(ctx, core.RPSessionOpenRequest{
		PrincipalID: M2RPPlayerPrincipal, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID,
		EntityID: M2RPPlayerID, POV: "second_person", IdempotencyKey: "move-session",
	})
	if err != nil {
		t.Fatal(err)
	}
	read := core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID}
	initial, err := store.ObserveRPSession(ctx, read)
	if err != nil || initial.PlaceID != M2AgentCafeID || initial.ObservationCursor != 8 {
		t.Fatalf("wrong initial RP observation: %+v, %v", initial, err)
	}
	move := core.RPMoveRequest{
		PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID,
		FromPlaceID: M2AgentCafeID, ToPlaceID: "place_m2_home_ada",
		ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "move-home-ada",
	}
	result, err := store.MoveRP(ctx, move)
	if err != nil {
		t.Fatal(err)
	}
	if result.EventSequence != 9 || result.FromPlaceID != M2AgentCafeID || result.ToPlaceID != "place_m2_home_ada" || result.Replayed {
		t.Fatalf("unexpected real move: %+v", result)
	}
	after, err := store.ObserveRPSession(ctx, read)
	if err != nil || after.PlaceID != "place_m2_home_ada" || after.ObservationCursor != 9 || len(after.PresentEntities) != 1 || after.PresentEntities[0].EntityID != M2AgentAdaID {
		t.Fatalf("observation did not follow player location: %+v, %v", after, err)
	}
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM agent_movements WHERE agent_id = ? AND event_id = ?`, []any{M2RPPlayerID, result.EventID}, 1)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM observation_records WHERE source_event_id = ?`, []any{result.EventID}, 2)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM agent_knowledge WHERE observer_agent_id = ? AND subject_agent_id = ?`, []any{M2AgentAdaID, M2RPPlayerID}, 1)
	replayed, err := store.MoveRP(ctx, move)
	if err != nil || !replayed.Replayed || replayed.EventID != result.EventID || replayed.EventSequence != result.EventSequence {
		t.Fatalf("move retry duplicated effect: %+v, %v", replayed, err)
	}
	mismatch := move
	mismatch.ToPlaceID = "place_m2_home_bo"
	if _, err := store.MoveRP(ctx, mismatch); !core.HasCode(err, core.CodeIdempotencyMismatch) {
		t.Fatalf("same key/different move did not conflict: %v", err)
	}
	unreachable := core.RPMoveRequest{
		PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID,
		FromPlaceID: "place_m2_home_ada", ToPlaceID: "place_m2_work_ada",
		ExpectedCursor: after.ObservationCursor, IdempotencyKey: "illegal-shortcut",
	}
	if _, err := store.MoveRP(ctx, unreachable); !core.HasCode(err, core.CodeInvalidArgument) {
		t.Fatalf("unreachable destination accepted: %v", err)
	}
	back := unreachable
	back.ToPlaceID = M2AgentCafeID
	back.IdempotencyKey = "return-cafe"
	store.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "move precommit") }
	if _, err := store.MoveRP(ctx, back); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatalf("expected precommit rollback, got %v", err)
	}
	store.beforeCommit = nil
	assertM2Value(t, ctx, store, `SELECT head_sequence FROM branches WHERE instance_id = ? AND branch_id = ?`, []any{M2DemoInstanceID, M2DemoBranchID}, 9)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM events WHERE event_type = 'RPPlayerMoved'`, nil, 1)
	if _, err := store.MoveRP(ctx, back); err != nil {
		t.Fatal(err)
	}
	differences, err := store.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID)
	if err != nil || len(differences) != 0 {
		t.Fatalf("RP move did not replay to current projections: %v, %v", differences, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	final, err := reopened.ObserveRPSession(ctx, read)
	if err != nil || final.PlaceID != M2AgentCafeID || final.ObservationCursor != 10 {
		t.Fatalf("reopened move lost location/cursor: %+v, %v", final, err)
	}
}
