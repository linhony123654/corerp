package storage

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPPlayObservationRestoresScopedHistoryAndLegalDestinations(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "play.db")
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	session, read, initial := newRPWaitTestSession(t, ctx, store)
	if len(initial.ReachablePlaces) != 4 || len(initial.RecentTurns) != 0 {
		t.Fatalf("initial view: %+v", initial)
	}
	turn, err := store.PlayRPTurn(ctx, core.RPSpeechRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID, Text: "借我一点钱好吗？", ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "play-history"})
	if err != nil {
		t.Fatal(err)
	}
	moved, err := store.MoveRP(ctx, core.RPMoveRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID, FromPlaceID: initial.PlaceID, ToPlaceID: "place_m2_home_ada", ExpectedCursor: turn.SettledSequence, IdempotencyKey: "play-move"})
	if err != nil {
		t.Fatal(err)
	}
	afterMove, err := store.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.WaitRP(ctx, core.RPWaitRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID, TargetWorldTime: M2AgentNoonTime, Budget: 100, ExpectedCursor: moved.EventSequence, IdempotencyKey: "play-wait"})
	if err != nil {
		t.Fatal(err)
	}
	if len(afterMove.ReachablePlaces) != 1 || afterMove.ReachablePlaces[0].PlaceID != M2AgentCafeID {
		t.Fatalf("wrong routes %+v", afterMove)
	}
	store.Close()
	store, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	restored, err := store.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	if len(restored.RecentTurns) != 3 || restored.RecentTurns[0].TurnRunID != turn.TurnRunID || !strings.Contains(restored.RecentTurns[0].NarrativeLines[1], "拒绝") || !strings.Contains(restored.RecentTurns[1].NarrativeLines[0], "前往") || !strings.Contains(restored.RecentTurns[2].NarrativeLines[0], "等待") {
		t.Fatalf("history not restored: %+v", restored.RecentTurns)
	}
	other, err := store.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: M2RPPlayerPrincipal, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2RPPlayerID, POV: "second_person", IdempotencyKey: "other-play-session"})
	if err != nil {
		t.Fatal(err)
	}
	separate, err := store.ObserveRPSession(ctx, core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: other.SessionID})
	if err != nil || len(separate.RecentTurns) != 0 {
		t.Fatalf("cross-session transcript: %+v, %v", separate, err)
	}
	if _, err := store.ObserveRPSession(ctx, core.RPSessionReadRequest{PrincipalID: "another-player", SessionID: session.SessionID}); err == nil {
		t.Fatal("unauthorized history exposed")
	}
}
