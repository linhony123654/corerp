package storage

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPNarrativeBudgetCannotBlockSettlementOrDiscardFacts(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "budget.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, read, initial := newRPWaitTestSession(t, ctx, s)
	low := 4096
	_, err = s.SetRPStyle(ctx, RPStyleSetRequest{PrincipalID: read.PrincipalID, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, Scope: "session", SessionID: read.SessionID, IdempotencyKey: "small-budget", Patch: core.RPStylePatch{ContextBudgetBytes: &low}})
	if err != nil {
		t.Fatal(err)
	}
	speech := core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "budget-speech", Text: strings.Repeat("你", 1800)}
	turn, err := s.PlayRPTurn(ctx, speech)
	if err != nil || turn.Status != "settled" || !strings.Contains(turn.NarrativeLines[0], speech.Text) {
		t.Fatalf("budget blocked settlement or dropped accepted text: %v", err)
	}
	r := RPNarrativeReadRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, TurnRunID: turn.TurnRunID}
	emitted := 0
	saved, err := s.StreamRPNarrative(ctx, r, func(core.RPNarrativeChunk) error { emitted++; return nil })
	if err != nil || emitted == 0 || saved.View.CompositionVersion != core.RPFactCompositionVersionV2 {
		t.Fatal("small budget blocked saved canonical", err)
	}
	// Only an explicitly requested fresh render is bounded by the read budget.
	emitted = 0
	r.StyleOverride = &core.RPStylePatch{ContextBudgetBytes: &low}
	_, err = s.StreamRPNarrative(ctx, r, func(core.RPNarrativeChunk) error { emitted++; return nil })
	if !core.HasCode(err, core.CodeInvalidArgument) || emitted != 0 {
		t.Fatalf("budget must reject before first emitted fact: %d %v", emitted, err)
	}
	observation, err := s.ObserveRPSession(ctx, read)
	if err != nil || !strings.Contains(observation.RecentTurns[len(observation.RecentTurns)-1].NarrativeLines[0], speech.Text) {
		t.Fatal("saved original unavailable after budget error")
	}
	high := 65536
	r.StyleOverride = &core.RPStylePatch{ContextBudgetBytes: &high}
	view, err := s.ReadRPNarrative(ctx, r)
	if err != nil || !strings.Contains(view.View.Lines[0], speech.Text) {
		t.Fatalf("explicit larger read budget failed: %v", err)
	}
	retry, err := s.PlayRPTurn(ctx, speech)
	if err != nil || !retry.Replayed || retry.TurnRunID != turn.TurnRunID {
		t.Fatalf("canonical recovery affected by budget: %v", err)
	}
	assertM2Value(t, ctx, s, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, []any{M2DemoInstanceID, M2DemoBranchID}, turn.SettledSequence)
}
