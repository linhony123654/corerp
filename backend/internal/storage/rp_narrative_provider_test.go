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
		return (core.LiteralRPNarrativeProvider{}).RenderStream(ctx, in, emit)
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
	r := RPNarrativeReadRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, TurnRunID: turn.TurnRunID, StyleOverride: &core.RPStylePatch{}}
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
	jsonRequest := r
	secondPerson := "second_person"
	jsonRequest.StyleOverride = &core.RPStylePatch{POV: &secondPerson}
	jsonView, err := service.ReadRPNarrative(ctx, jsonRequest)
	if err != nil || calls != 2 || !reflect.DeepEqual(view.Style, jsonView.Style) || !reflect.DeepEqual(view.View.Lines, jsonView.View.Lines) || view.View.RenderID == "" || jsonView.View.RenderID == "" || view.View.RenderID == jsonView.View.RenderID {
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
	firstPerson := "first_person"
	failedVariant := r
	failedVariant.StyleOverride = &core.RPStylePatch{POV: &firstPerson}
	if _, err := service.ReadRPNarrative(ctx, failedVariant); err == nil {
		t.Fatal("provider failure concealed")
	}
	observation, err := service.ObserveRPSession(ctx, read)
	selected := observation.RecentTurns[len(observation.RecentTurns)-1]
	if err != nil || !reflect.DeepEqual(selected.NarrativeLines, jsonView.View.Lines) || selected.RenderID != jsonView.View.RenderID {
		t.Fatal("provider failure erased selected presentation")
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

func TestRPOfficialNarrativePersistsAcrossSelectedVariantAndRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "official-narrative.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	_, read, initial := newRPWaitTestSession(t, ctx, s)
	turn, err := s.PlayRPTurn(ctx, core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "official-turn", Text: "请问你是谁？"})
	if err != nil {
		t.Fatal(err)
	}
	clearRPV2CanonicalFixture(t, ctx, s, turn.TurnRunID)
	providerCalls := 0
	provider := narrativeProviderFixture{call: func(_ context.Context, in core.RPNarrativeInput, emit func(core.RPNarrativeChunk) error) (core.RPNarrativeView, error) {
		providerCalls++
		eventIDs := make([]string, 0, len(in.Facts))
		for _, fact := range in.Facts {
			eventIDs = append(eventIDs, fact.EventID)
		}
		line := "正式长篇叙述保留了对白：「请问你是谁？」"
		if providerCalls > 1 {
			line = "临时改写仍保留对白：「请问你是谁？」"
		}
		if emit != nil {
			if err := emit(core.RPNarrativeChunk{Index: 0, EventIDs: eventIDs, Line: line}); err != nil {
				return core.RPNarrativeView{}, err
			}
		}
		return core.RPNarrativeView{Lines: []string{line}, EventIDs: eventIDs, Warnings: []string{}}, nil
	}}
	service, err := NewRPServiceWithNarrative(s, core.DeterministicRPDecisionProvider{}, "deterministic", provider)
	if err != nil {
		t.Fatal(err)
	}
	r := RPNarrativeReadRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, TurnRunID: turn.TurnRunID}
	official, err := service.ReadRPNarrative(ctx, r)
	if err != nil || providerCalls != 1 {
		t.Fatalf("official narrative not generated once: %+v %v calls=%d", official, err, providerCalls)
	}
	originalObservation, err := service.ObserveRPSession(ctx, read)
	if err != nil || len(originalObservation.RecentTurns) == 0 || originalObservation.RecentTurns[len(originalObservation.RecentTurns)-1].RenderID != "" || !reflect.DeepEqual(originalObservation.RecentTurns[len(originalObservation.RecentTurns)-1].NarrativeLines, official.View.Lines) {
		t.Fatalf("canonical narrative should display without a variant selection: %+v %v", originalObservation.RecentTurns, err)
	}
	secondPerson := "second_person"
	variantRequest := r
	variantRequest.StyleOverride = &core.RPStylePatch{POV: &secondPerson}
	variant, err := service.ReadRPNarrative(ctx, variantRequest)
	if err != nil || providerCalls != 2 || reflect.DeepEqual(variant.View.Lines, official.View.Lines) {
		t.Fatalf("temporary variant not generated independently: %+v %v calls=%d", variant, err, providerCalls)
	}
	observed, err := service.ObserveRPSession(ctx, read)
	if err != nil || !reflect.DeepEqual(observed.RecentTurns[len(observed.RecentTurns)-1].NarrativeLines, variant.View.Lines) {
		t.Fatalf("selected variant was not shown: %+v %v", observed.RecentTurns, err)
	}
	var presentationMode string
	if err := s.db.QueryRowContext(ctx, `SELECT narrative_presentation_mode FROM rp_turn_runs WHERE turn_run_id=?`, turn.TurnRunID).Scan(&presentationMode); err != nil || presentationMode != "custom" {
		t.Fatalf("official narrative mode not persisted: %q %v", presentationMode, err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_turn_runs WHERE turn_run_id=? AND narrative_presented_at_utc IS NOT NULL`, []any{turn.TurnRunID}, 1)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	reloadedCalls := 0
	reloadedProvider := narrativeProviderFixture{call: func(context.Context, core.RPNarrativeInput, func(core.RPNarrativeChunk) error) (core.RPNarrativeView, error) {
		reloadedCalls++
		return core.RPNarrativeView{}, errors.New("saved narrative should bypass provider")
	}}
	reloadedService, err := NewRPServiceWithNarrative(s, core.DeterministicRPDecisionProvider{}, "deterministic", reloadedProvider)
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := reloadedService.ReadRPNarrative(ctx, r)
	if err != nil || reloadedCalls != 0 || !reflect.DeepEqual(reloaded.View.Lines, official.View.Lines) {
		t.Fatalf("official narrative did not survive restart: %+v %v calls=%d", reloaded, err, reloadedCalls)
	}
}
