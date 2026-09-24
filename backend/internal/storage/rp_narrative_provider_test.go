package storage

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"corerp.local/backend/internal/core"
)

type narrativeProviderFixture struct {
	call func(context.Context, core.RPNarrativeInput, func(core.RPNarrativeChunk) error) (core.RPNarrativeView, error)
}

func (p narrativeProviderFixture) Render(ctx context.Context, in core.RPNarrativeInput) (core.RPNarrativeView, error) {
	return p.RenderStream(ctx, in, nil)
}
func (p narrativeProviderFixture) RenderStream(ctx context.Context, in core.RPNarrativeInput, emit func(core.RPNarrativeChunk) error) (core.RPNarrativeView, error) {
	return p.call(ctx, in, emit)
}

func TestRPNarrativeProviderIndependentAuthorizedAndOutsideTransaction(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "narrative-provider.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, read, initial := newRPWaitTestSession(t, ctx, s)
	calls := 0
	var captured core.RPNarrativeInput
	fail := false
	provider := narrativeProviderFixture{call: func(ctx context.Context, in core.RPNarrativeInput, emit func(core.RPNarrativeChunk) error) (core.RPNarrativeView, error) {
		calls++
		captured = in
		// Store has one connection. A provider must not be invoked inside a
		// retained transaction, even if it is slow or sends external requests.
		probe, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		var head int64
		if err := s.db.QueryRowContext(probe, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&head); err != nil {
			t.Fatalf("provider held storage transaction: %v", err)
		}
		if fail {
			return core.RPNarrativeView{}, errors.New("test narrative service unavailable")
		}
		in.Style.POV = "first_person"
		return (core.DeterministicRPNarrativeProvider{}).RenderStream(ctx, in, emit)
	}}
	service, err := NewRPServiceWithNarrative(s, core.DeterministicRPDecisionProvider{}, "deterministic", provider)
	if err != nil {
		t.Fatal(err)
	}
	request := core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "narrative-independent", Text: "你好，今天还好吗？"}
	turn, err := service.PlayRPTurn(ctx, request)
	if err != nil || turn.Status != "settled" || calls != 0 {
		t.Fatalf("presentation affected settlement: %+v %v calls=%d", turn, err, calls)
	}
	r := RPNarrativeReadRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, TurnRunID: turn.TurnRunID}
	var chunks []core.RPNarrativeChunk
	view, err := service.StreamRPNarrative(ctx, r, func(c core.RPNarrativeChunk) error { chunks = append(chunks, c); return nil })
	if err != nil || calls != 1 || len(chunks) == 0 || len(chunks) != len(view.View.Lines) {
		t.Fatalf("injected stream not used: %+v %v", view, err)
	}
	if captured.ControlledEntityID != M2RPPlayerID || captured.Facts[0].Text != request.Text || captured.Style.POV != "second_person" {
		t.Fatalf("wrong pinned/committed input: %+v", captured)
	}
	if view.View.Lines[0] == turn.NarrativeLines[0] {
		t.Fatal("fixture style did not produce actual variant")
	}
	jsonView, err := service.ReadRPNarrative(ctx, r)
	if err != nil || calls != 2 || !reflect.DeepEqual(view, jsonView) {
		t.Fatalf("JSON and stream do not share provider: %v", err)
	}
	foreign := r
	foreign.PrincipalID = "principal_creator"
	if _, err := service.ReadRPNarrative(ctx, foreign); !core.HasCode(err, core.CodeNotFound) || calls != 2 {
		t.Fatalf("unauthorized data reached provider: %v calls=%d", err, calls)
	}
	limited := r
	budget, prose := 4096, strings.Repeat("细", 1800)
	limited.StyleOverride = &core.RPStylePatch{ContextBudgetBytes: &budget, ProseInstructions: &prose}
	if _, err := service.ReadRPNarrative(ctx, limited); !core.HasCode(err, core.CodeInvalidArgument) || calls != 2 {
		t.Fatalf("oversized context reached provider: %v calls=%d", err, calls)
	}
	fail = true
	if _, err := service.ReadRPNarrative(ctx, r); err == nil {
		t.Fatal("provider failure concealed")
	}
	observation, err := service.ObserveRPSession(ctx, read)
	if err != nil || !reflect.DeepEqual(observation.RecentTurns[len(observation.RecentTurns)-1].NarrativeLines, turn.NarrativeLines) {
		t.Fatal("provider failure erased original")
	}
	retry, err := service.PlayRPTurn(ctx, request)
	if err != nil || !retry.Replayed || calls != 3 {
		t.Fatalf("presentation failure reran world or presentation on command retry: %v calls=%d", err, calls)
	}
	assertM2Value(t, ctx, s, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, []any{M2DemoInstanceID, M2DemoBranchID}, turn.SettledSequence)
	if _, err := NewRPServiceWithNarrative(s, core.DeterministicRPDecisionProvider{}, "deterministic", nil); !core.HasCode(err, core.CodeInvalidArgument) {
		t.Fatalf("nil narrative provider accepted: %v", err)
	}
}
