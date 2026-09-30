package storage

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPNarrativeRenderSelectionPersistsAndDoesNotChangeWorld(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "selected-render.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	_, read, initial := newRPWaitTestSession(t, ctx, s)
	service, err := NewRPServiceWithNarrative(s, core.DeterministicRPDecisionProvider{}, "deterministic", core.DeterministicRPNarrativeProvider{})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := service.PlayRPTurn(ctx, core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "render-first", Text: "你好。"})
	if err != nil || turn.Status != "settled" {
		t.Fatal(turn, err)
	}
	var head, eventCount int64
	if err := s.db.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&head); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&eventCount); err != nil {
		t.Fatal(err)
	}
	firstPOV, secondPOV := "first_person", "second_person"
	request := RPNarrativeReadRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, TurnRunID: turn.TurnRunID, StyleOverride: &core.RPStylePatch{POV: &firstPOV}}
	first, err := service.ReadRPNarrative(ctx, request)
	if err != nil || first.View.RenderID == "" || reflect.DeepEqual(first.View.Lines, turn.NarrativeLines) {
		t.Fatal("first variant", first, err)
	}
	request.StyleOverride.POV = &secondPOV
	second, err := service.ReadRPNarrative(ctx, request)
	if err != nil || second.View.RenderID == "" || second.View.RenderID == first.View.RenderID {
		t.Fatal("second variant", second, err)
	}
	selected, err := service.SelectRPNarrative(ctx, RPNarrativeSelectRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, TurnRunID: turn.TurnRunID, RenderID: first.View.RenderID})
	if err != nil || !reflect.DeepEqual(selected.Lines, first.View.Lines) {
		t.Fatal("select first", selected, err)
	}
	assertM2Value(t, ctx, s, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, []any{M2DemoInstanceID, M2DemoBranchID}, head)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE instance_id=? AND branch_id=?`, []any{M2DemoInstanceID, M2DemoBranchID}, eventCount)
	if _, err := service.SelectRPNarrative(ctx, RPNarrativeSelectRequest{PrincipalID: "principal_creator", SessionID: read.SessionID, TurnRunID: turn.TurnRunID, RenderID: second.View.RenderID}); !core.HasCode(err, core.CodeNotFound) {
		t.Fatal("foreign principal could select", err)
	}
	observation, err := service.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	other, err := service.PlayRPTurn(ctx, core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: observation.ObservationCursor, IdempotencyKey: "render-second", Text: "再见。"})
	if err != nil || other.Status != "settled" {
		t.Fatal(other, err)
	}
	if _, err := service.SelectRPNarrative(ctx, RPNarrativeSelectRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, TurnRunID: other.TurnRunID, RenderID: first.View.RenderID}); !core.HasCode(err, core.CodeNotFound) {
		t.Fatal("cross-turn render selectable", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	observation, err = reopened.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, recent := range observation.RecentTurns {
		if recent.TurnRunID == turn.TurnRunID {
			found = true
			if recent.RenderID != first.View.RenderID || !reflect.DeepEqual(recent.NarrativeLines, first.View.Lines) {
				t.Fatal("selected render lost after restart", recent)
			}
		}
	}
	if !found {
		t.Fatal("rendered turn missing after restart")
	}
	baseline, err := reopened.SelectRPNarrative(ctx, RPNarrativeSelectRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, TurnRunID: turn.TurnRunID})
	if err != nil || baseline.RenderID != "" || !reflect.DeepEqual(baseline.Lines, turn.NarrativeLines) {
		t.Fatal("canonical restore", baseline, err)
	}
	observation, err = reopened.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	for _, recent := range observation.RecentTurns {
		if recent.TurnRunID == turn.TurnRunID && (recent.RenderID != "" || !reflect.DeepEqual(recent.NarrativeLines, turn.NarrativeLines)) {
			t.Fatal("restore lost after refresh", recent)
		}
	}
	assertM2Value(t, ctx, reopened, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, []any{M2DemoInstanceID, M2DemoBranchID}, other.SettledSequence)
	assertM2Value(t, ctx, reopened, `SELECT COUNT(*) FROM rp_narrative_renders WHERE turn_run_id=?`, []any{turn.TurnRunID}, 2)
	var canonical string
	if err := reopened.db.QueryRowContext(ctx, `SELECT narrative_json FROM rp_turn_runs WHERE turn_run_id=?`, turn.TurnRunID).Scan(&canonical); err != nil {
		t.Fatal(err)
	}
	var original []string
	if err := json.Unmarshal([]byte(canonical), &original); err != nil || !reflect.DeepEqual(original, turn.NarrativeLines) {
		t.Fatal("canonical turn changed", err)
	}
}

func TestRPNarrativeRenderMigrationUpgradesPopulated052(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "pre-render.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	_, read, initial := newRPWaitTestSession(t, ctx, s)
	turn, err := s.RunRPTurn(ctx, core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "pre-render-turn", Text: "你好。"}, core.DeterministicRPDecisionProvider{})
	if err != nil || turn.Status != "settled" {
		t.Fatal(turn, err)
	}
	for _, statement := range []string{
		`DROP TABLE rp_narrative_selections`,
		`DROP TABLE rp_narrative_renders`,
		`DELETE FROM schema_meta WHERE schema_version='corerp-rp-narrative-renders-073-2026-09-28'`,
	} {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(ctx, path)
	if err != nil {
		t.Fatal("upgrade populated 052", err)
	}
	defer reopened.Close()
	assertM2Value(t, ctx, reopened, `SELECT COUNT(*) FROM schema_meta WHERE schema_version=?`, []any{RPNarrativeRendersSchemaVersion}, 1)
	observation, err := reopened.ObserveRPSession(ctx, read)
	if err != nil || len(observation.RecentTurns) == 0 {
		t.Fatal("pre-upgrade turn missing", err)
	}
	view, err := reopened.ReadRPNarrative(ctx, RPNarrativeReadRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, TurnRunID: turn.TurnRunID})
	if err != nil || view.View.RenderID == "" {
		t.Fatal("upgraded DB cannot save render", err)
	}
}
