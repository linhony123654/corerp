package storage

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPSessionOpenedWindowExcludesReplayPreservesNewInitiativeAndHistory(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "session-window.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	_, oldRead, observation := newRPWaitTestSession(t, ctx, s)
	responding := rpDecisionProviderFunc(func(context.Context, core.RPDecisionInput) (core.RPDecisionProposal, error) {
		return core.RPDecisionProposal{Action: "respond", Text: "这是旧会话的答复。", ExpressionCode: "smile"}, nil
	})
	var oldTurns []RPTurnResult
	var oldExpressions []string
	for _, key := range []string{"old-one", "old-two"} {
		turn, err := s.RunRPTurn(ctx, core.RPSpeechRequest{PrincipalID: oldRead.PrincipalID, SessionID: oldRead.SessionID, ExpectedCursor: observation.ObservationCursor, Text: "旧会话问候。", IdempotencyKey: key}, responding)
		if err != nil {
			t.Fatal(err)
		}
		oldTurns = append(oldTurns, turn)
		input, err := s.readRPNarrativeInput(ctx, oldRead.SessionID, turn.PlayerTurnID, turn.PlayerEventID)
		if err != nil {
			t.Fatal(err)
		}
		for _, fact := range input.Facts {
			if fact.Action == "expression" {
				oldExpressions = append(oldExpressions, fact.EventID)
			}
		}
		observation, err = s.ObserveRPSession(ctx, oldRead)
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(oldExpressions) != 2 {
		t.Fatalf("old public expression setup: %v", oldExpressions)
	}
	request := core.RPSessionOpenRequest{PrincipalID: oldRead.PrincipalID, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2RPPlayerID, POV: "second_person", IdempotencyKey: "new-window-session"}
	opened, err := s.OpenRPSession(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	read := core.RPSessionReadRequest{PrincipalID: oldRead.PrincipalID, SessionID: opened.SessionID}
	fresh, err := s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	// Opening a session retains the shared public transcript; it is not a chapter reset.
	if len(fresh.RecentTurns) != 2 {
		t.Fatalf("new session erased prior public history: %+v", fresh.RecentTurns)
	}
	first, err := s.RunRPTurn(ctx, core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: fresh.ObservationCursor, Text: "新会话问候。", IdempotencyKey: "new-one"}, core.DeterministicRPDecisionProvider{})
	if err != nil {
		t.Fatal(err)
	}
	input, err := s.readRPNarrativeInput(ctx, read.SessionID, first.PlayerTurnID, first.PlayerEventID)
	if err != nil {
		t.Fatal(err)
	}
	for _, fact := range input.Facts {
		for _, oldID := range oldExpressions {
			if fact.EventID == oldID {
				t.Fatalf("previous session expression replayed as new action: %s", oldID)
			}
		}
		if fact.Text == "这是旧会话的答复。" {
			t.Fatal("old speech replayed")
		}
	}
	var boundary int64
	if err := s.db.QueryRowContext(ctx, `SELECT opened_sequence FROM rp_sessions WHERE session_id=?`, read.SessionID).Scan(&boundary); err != nil || boundary != oldTurns[1].SettledSequence {
		t.Fatalf("open boundary not frozen: %d %v", boundary, err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE rp_sessions SET opened_sequence=opened_sequence+1 WHERE session_id=?`, read.SessionID); err == nil {
		t.Fatal("session opening boundary was mutable")
	}
	if opened.ChapterStartSequence != 0 {
		t.Fatal("open changed chapter history boundary")
	}
	// A genuine public initiative after opening remains available even after Observe advances.
	current, err := s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	wait, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: current.ObservationCursor, TargetWorldTime: "2026-09-22T03:00:00Z", Budget: 10, IdempotencyKey: "window-wait"})
	if err != nil {
		t.Fatal(err)
	}
	initiative, err := s.RunRPInitiative(ctx, core.RPInitiativeRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, NPCEntityID: M2RPNPCID, TriggerEventID: wait.EventID}, rpDecisionProviderFunc(func(context.Context, core.RPDecisionInput) (core.RPDecisionProposal, error) {
		return core.RPDecisionProposal{Action: "respond", Text: "这是打开后发生的主动招呼。", ExpressionCode: "nod"}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	current, err = s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	// Interrupt after accepted speech, then recover after restart with the same open boundary.
	nextRequest := core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: current.ObservationCursor, Text: "我听见招呼了。", IdempotencyKey: "new-two"}
	s.afterRPTurnStage = func(stage string) error {
		if stage == "player_event_committed" {
			return core.NewError(core.CodeInjectedFailure, "session window recovery probe")
		}
		return nil
	}
	if _, err := s.RunRPTurn(ctx, nextRequest, core.DeterministicRPDecisionProvider{}); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatal("pending recovery probe missing", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.ResumeRPTurn(ctx, RPTurnResumeRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, IdempotencyKey: nextRequest.IdempotencyKey}, core.DeterministicRPDecisionProvider{})
	if err != nil {
		t.Fatal(err)
	}
	secondInput, err := s.readRPNarrativeInput(ctx, read.SessionID, second.PlayerTurnID, second.PlayerEventID)
	if err != nil {
		t.Fatal(err)
	}
	foundSpeech, foundGesture := false, false
	for _, fact := range secondInput.Facts {
		if fact.EventID == initiative.EventID && fact.Text == "这是打开后发生的主动招呼。" {
			foundSpeech = true
		}
		if fact.Action == "expression" && fact.CompanionEventID == initiative.EventID {
			foundGesture = true
		}
	}
	if !foundSpeech || !foundGesture {
		t.Fatalf("open/Observe/recovery dropped real inter-turn initiative: %+v", secondInput.Facts)
	}
	var savedBefore string
	if err := s.db.QueryRowContext(ctx, `SELECT narrative_artifact_json FROM rp_turn_runs WHERE turn_run_id=?`, second.TurnRunID).Scan(&savedBefore); err != nil {
		t.Fatal(err)
	}
	// Another new session must not alter existing saved artifacts, their source IDs or world head.
	request.IdempotencyKey = "third-window-session"
	third, err := s.OpenRPSession(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if third.ChapterStartSequence != 0 {
		t.Fatal("new open hid shared history")
	}
	cached, err := s.ReadRPNarrative(ctx, RPNarrativeReadRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, TurnRunID: second.TurnRunID})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cached.View.Lines, second.NarrativeLines) || !reflect.DeepEqual(cached.View.FactGroups, second.FactGroups) {
		t.Fatal("old window artifact changed", cached.View)
	}
	var savedAfter string
	if err := s.db.QueryRowContext(ctx, `SELECT narrative_artifact_json FROM rp_turn_runs WHERE turn_run_id=?`, second.TurnRunID).Scan(&savedAfter); err != nil || savedAfter != savedBefore {
		t.Fatal("opening another session rewrote artifact", err)
	}
	request.IdempotencyKey = "new-window-session"
	replay, err := s.OpenRPSession(ctx, request)
	if err != nil || !replay.Replayed || replay.SessionID != opened.SessionID {
		t.Fatal("open replay changed session", replay, err)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT opened_sequence FROM rp_sessions WHERE session_id=?`, read.SessionID).Scan(&boundary); err != nil || boundary != oldTurns[1].SettledSequence {
		t.Fatal("open replay advanced boundary", boundary, err)
	}
	for _, old := range oldTurns {
		cached, err := s.ReadRPNarrative(ctx, RPNarrativeReadRequest{PrincipalID: oldRead.PrincipalID, SessionID: oldRead.SessionID, TurnRunID: old.TurnRunID})
		if err != nil || !reflect.DeepEqual(cached.View.Lines, old.NarrativeLines) || !reflect.DeepEqual(cached.View.FactGroups, old.FactGroups) {
			t.Fatal("old canonical artifact became invalid", old.TurnRunID, err)
		}
	}
	history, err := s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	if len(history.RecentTurns) < 4 {
		t.Fatalf("shared long history lost: %+v", history.RecentTurns)
	}
	var head int64
	if err := s.db.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&head); err != nil || head != second.SettledSequence {
		t.Fatal("metadata open changed Canon head", head, err)
	}
}

func TestRPSessionOpenedWindowLegacySchemaAndMigrationPreserveExistingSession(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "legacy-session.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	// Deliberately remove only the new application metadata to emulate a manual 078 schema.
	if _, err := s.db.ExecContext(ctx, `DELETE FROM schema_meta WHERE schema_version='corerp-rp-session-opened-window-079-2026-10-01'`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `DROP TRIGGER rp_session_opened_sequence_immutable`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `ALTER TABLE rp_sessions DROP COLUMN opened_sequence`); err != nil {
		t.Fatal(err)
	}
	session, read, initial := newRPWaitTestSession(t, ctx, s)
	raw, err := json.Marshal(session)
	if err != nil {
		t.Fatal(err)
	}
	var public map[string]any
	if err := json.Unmarshal(raw, &public); err != nil {
		t.Fatal(err)
	}
	if _, exists := public["opened_sequence"]; exists {
		t.Fatal("internal boundary exposed to session wire")
	}
	resumed, err := s.ResumeRPSession(ctx, read)
	if err != nil || resumed.SessionID != session.SessionID {
		t.Fatal("old schema session load failed", err)
	}
	oldTurn, err := s.RunRPTurn(ctx, core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: initial.ObservationCursor, Text: "旧schema问候。", IdempotencyKey: "legacy-first"}, rpDecisionProviderFunc(func(context.Context, core.RPDecisionInput) (core.RPDecisionProposal, error) {
		return core.RPDecisionProposal{Action: "respond", Text: "旧schema答复。", ExpressionCode: "smile"}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	legacyWindow, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: read.PrincipalID, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2RPPlayerID, POV: "second_person", IdempotencyKey: "legacy-window"})
	if err != nil {
		t.Fatal(err)
	}
	legacyRead := core.RPSessionReadRequest{PrincipalID: read.PrincipalID, SessionID: legacyWindow.SessionID}
	legacyView, err := s.ObserveRPSession(ctx, legacyRead)
	if err != nil {
		t.Fatal(err)
	}
	legacyTurn, err := s.PlayRPTurn(ctx, core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: legacyRead.SessionID, ExpectedCursor: legacyView.ObservationCursor, Text: "旧schema历史窗口。", IdempotencyKey: "legacy-window-turn"})
	if err != nil {
		t.Fatal(err)
	}
	legacyInput, err := s.readRPNarrativeInput(ctx, legacyRead.SessionID, legacyTurn.PlayerTurnID, legacyTurn.PlayerEventID)
	if err != nil {
		t.Fatal(err)
	}
	foundLegacyWindow := false
	for _, fact := range legacyInput.Facts {
		if fact.Action == "expression" && fact.CompanionEventID == "" && fact.WorldTime != "" {
			foundLegacyWindow = true
		}
	}
	if !foundLegacyWindow || oldTurn.SettledSequence >= legacyTurn.SettledSequence {
		t.Fatal("legacy window artifact setup missing", legacyInput.Facts)
	}
	var legacyRawBefore string
	if err := s.db.QueryRowContext(ctx, `SELECT narrative_artifact_json FROM rp_turn_runs WHERE turn_run_id=?`, legacyTurn.TurnRunID).Scan(&legacyRawBefore); err != nil {
		t.Fatal(err)
	}
	if err := s.applyMigration(ctx, "079_rp_session_opened_window.sql"); err != nil {
		t.Fatal(err)
	}
	var boundary int64
	if err := s.db.QueryRowContext(ctx, `SELECT opened_sequence FROM rp_sessions WHERE session_id=?`, session.SessionID).Scan(&boundary); err != nil || boundary != 0 {
		t.Fatal("migration retroactively changed legacy window", boundary, err)
	}
	legacyCached, err := s.ReadRPNarrative(ctx, RPNarrativeReadRequest{PrincipalID: read.PrincipalID, SessionID: legacyRead.SessionID, TurnRunID: legacyTurn.TurnRunID})
	if err != nil || !reflect.DeepEqual(legacyCached.View.Lines, legacyTurn.NarrativeLines) || !reflect.DeepEqual(legacyCached.View.FactGroups, legacyTurn.FactGroups) {
		t.Fatal("079 invalidated old saved window artifact", legacyCached, err)
	}
	var legacyRawAfter string
	if err := s.db.QueryRowContext(ctx, `SELECT narrative_artifact_json FROM rp_turn_runs WHERE turn_run_id=?`, legacyTurn.TurnRunID).Scan(&legacyRawAfter); err != nil || legacyRawAfter != legacyRawBefore {
		t.Fatal("079 rewrote legacy artifact", err)
	}

}
