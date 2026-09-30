package storage

import (
	"context"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPChapterStartHidesOnlyThisSessionsPriorStoryAndSurvivesRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "rp-chapter.db")
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	session, read, initial := newRPWaitTestSession(t, ctx, store)
	first, err := store.PlayRPTurn(ctx, core.RPSpeechRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID, Text: "这是旧篇的问候。", ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "chapter-old-turn"})
	if err != nil {
		t.Fatal(err)
	}
	before, err := store.ObserveRPSession(ctx, read)
	if err != nil || len(before.RecentTurns) != 1 {
		t.Fatalf("old chapter not visible before reset: %+v %v", before.RecentTurns, err)
	}
	other, err := store.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: M2RPPlayerPrincipal, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2RPPlayerID, POV: "second_person", IdempotencyKey: "chapter-other-session"})
	if err != nil {
		t.Fatal(err)
	}
	request := RPChapterStartRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID, ExpectedCursor: before.ObservationCursor, IdempotencyKey: "chapter-reset-1"}
	reset, err := store.StartRPChapter(ctx, request)
	if err != nil || reset.Replayed || reset.ChapterStartSequence != first.SettledSequence {
		t.Fatalf("new chapter boundary not committed at current head: %+v %v", reset, err)
	}
	replay, err := store.StartRPChapter(ctx, request)
	if err != nil || !replay.Replayed || replay.ChapterStartSequence != reset.ChapterStartSequence {
		t.Fatalf("new chapter retry changed its boundary: %+v %v", replay, err)
	}
	bad := request
	bad.ExpectedCursor++
	if _, err := store.StartRPChapter(ctx, bad); !core.HasCode(err, core.CodeIdempotencyMismatch) {
		t.Fatalf("mismatched chapter retry accepted: %v", err)
	}
	fresh, err := store.ObserveRPSession(ctx, read)
	if err != nil || len(fresh.RecentTurns) != 0 {
		t.Fatalf("old chapter remained visible: %+v %v", fresh.RecentTurns, err)
	}
	otherView, err := store.ObserveRPSession(ctx, core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: other.SessionID})
	if err != nil || len(otherView.RecentTurns) != 1 || otherView.RecentTurns[0].TurnRunID != first.TurnRunID {
		t.Fatalf("reset leaked into another session: %+v %v", otherView.RecentTurns, err)
	}
	second, err := store.PlayRPTurn(ctx, core.RPSpeechRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID, Text: "这是新篇的开场。", ExpectedCursor: fresh.ObservationCursor, IdempotencyKey: "chapter-new-turn"})
	if err != nil {
		t.Fatal(err)
	}
	input, err := store.readRPNarrativeInput(ctx, session.SessionID, second.PlayerTurnID, second.PlayerEventID)
	if err != nil {
		t.Fatal(err)
	}
	for _, fact := range input.Facts {
		if fact.EventID == first.PlayerEventID || fact.Text == "这是旧篇的问候。" {
			t.Fatalf("old chapter fact entered new prose context: %+v", fact)
		}
	}
	after, err := store.ObserveRPSession(ctx, read)
	if err != nil || len(after.RecentTurns) != 1 || after.RecentTurns[0].TurnRunID != second.TurnRunID {
		t.Fatalf("new chapter history is not isolated: %+v %v", after.RecentTurns, err)
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
	if err != nil || resumed.ChapterStartSequence != reset.ChapterStartSequence {
		t.Fatalf("chapter boundary did not survive restart: %+v %v", resumed, err)
	}
	afterRestart, err := store.ObserveRPSession(ctx, read)
	if err != nil || len(afterRestart.RecentTurns) != 1 || afterRestart.RecentTurns[0].TurnRunID != second.TurnRunID {
		t.Fatalf("chapter transcript changed after restart: %+v %v", afterRestart.RecentTurns, err)
	}
}

func TestRPChapterStartRejectsUnsettledTurn(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "rp-chapter-pending.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, _, initial := newRPWaitTestSession(t, ctx, store)
	if _, err := store.db.ExecContext(ctx, `INSERT INTO rp_turn_runs(turn_run_id,session_id,idempotency_key,player_speech_key,request_hash,request_json,status,created_at_utc,updated_at_utc) VALUES ('pending_chapter_turn',?,?,?,'hash','{}','open',?,?)`, session.SessionID, "pending-key", "pending-speech", "2026-09-27T00:00:00Z", "2026-09-27T00:00:00Z"); err != nil {
		t.Fatal(err)
	}
	_, err = store.StartRPChapter(ctx, RPChapterStartRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID, ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "blocked-reset"})
	if !core.HasCode(err, core.CodeCommandInProgress) {
		t.Fatalf("new chapter accepted with an unsettled turn: %v", err)
	}
}
