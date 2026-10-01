package storage

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"corerp.local/backend/internal/core"
)

const rpInteractionNodPrivate = "private-interaction-nod-never-public"

type rpInteractionNodProvider struct {
	t                          *testing.T
	target                     string
	interpretations, decisions int
}

func (p *rpInteractionNodProvider) UnderstandInteraction(_ context.Context, in core.RPInteractionUnderstandingInput) (core.RPInteractionPlan, error) {
	p.interpretations++
	visible := false
	for _, entity := range in.PresentEntities {
		if entity.ID == M2RPNPCID {
			visible = true
		}
	}
	if in.Mode != "AUTO" || !visible {
		p.t.Fatalf("AUTO fixture lacks visible Cai: %+v", in)
	}
	return core.RPInteractionPlan{Mode: "AUTO", Kind: "ACTION", Steps: []core.RPInteractionStep{{Kind: "nonverbal", NonverbalAction: "nod", TargetEntityID: p.target}}}, nil
}

func (p *rpInteractionNodProvider) Propose(_ context.Context, in core.RPDecisionInput) (core.RPDecisionProposal, error) {
	p.decisions++
	a := in.ObservedPlayerAction
	if in.NPCEntityID != M2RPNPCID || in.Trigger == nil || in.Trigger.Kind != "nonverbal" || a == nil || a.Action != "nod" || a.ActorEntityID != M2RPPlayerID || a.TargetEntityID != M2RPNPCID || a.SourceEventID == "" || a.SourceEventID != in.Trigger.SourceEventID || a.SourceEventID != in.TurnID || in.PlayerSpeechText != "" || in.SpeechEventID != "" || in.PlayerSpeechWorldTime != "" {
		p.t.Fatalf("incorrect actual nonverbal trigger: %+v", in)
	}
	return core.RPDecisionProposal{Action: "respond", Text: "我看见你向我点头了。", ExpressionCode: "nod", Private: &core.RPDecisionPrivate{Intent: rpInteractionNodPrivate, BasisEventIDs: []string{a.SourceEventID}}}, nil
}

func TestRPNonverbalInteractionAUTORecoveryExactlyOnce(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "interaction-nod.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	_, read, initial := newRPWaitTestSession(t, ctx, s)
	p := &rpInteractionNodProvider{t: t, target: M2RPNPCID}
	service, err := NewRPService(s, p, "chat_completions")
	if err != nil {
		t.Fatal(err)
	}
	request := core.RPInteractionRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, Text: "向Cai点头。", Mode: "AUTO", ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "auto-nod-recovery"}
	resume := RPInteractionResumeRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, IdempotencyKey: request.IdempotencyKey}
	cut := errors.New("injected player_event_committed")
	s.afterRPTurnStage = func(stage string) error {
		if stage == "player_event_committed" {
			return cut
		}
		return nil
	}
	if _, err := service.RunRPInteraction(ctx, request); !errors.Is(err, cut) {
		t.Fatalf("missing committed-action cut: %v", err)
	}
	if p.interpretations != 1 || p.decisions != 0 {
		t.Fatalf("unexpected calls before resume: %+v", p)
	}
	var interactionID, pending string
	if err := s.db.QueryRowContext(ctx, `SELECT i.interaction_id,p.request_json FROM rp_interactions i JOIN rp_interaction_pending_actions p ON p.interaction_id=i.interaction_id WHERE i.session_id=? AND i.idempotency_key=? AND i.status='open' AND i.next_step=0 AND p.kind='nonverbal'`, read.SessionID, request.IdempotencyKey).Scan(&interactionID, &pending); err != nil {
		t.Fatal(err)
	}
	var child core.RPNonverbalRequest
	if err := json.Unmarshal([]byte(pending), &child); err != nil || child.IdempotencyKey == "" {
		t.Fatal("missing child", err)
	}
	assertPinned := func() {
		t.Helper()
		assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_interaction_pending_actions WHERE interaction_id=? AND step_index=0 AND kind='nonverbal' AND request_json=?`, []any{interactionID, pending}, 1)
		assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_interactions WHERE interaction_id=? AND status='open' AND next_step=0 AND pending_kind='' AND pending_request_json='{}'`, []any{interactionID}, 1)
	}
	assertPinned()
	if _, err := service.StopRPInteraction(ctx, resume); !core.HasCode(err, core.CodeCommandInProgress) {
		t.Fatalf("stop erased accepted child: %v", err)
	}
	retired, err := s.RetireRPRequest(ctx, RPRequestRetireRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, Operation: "nonverbal", IdempotencyKey: child.IdempotencyKey})
	if err != nil || retired.Status != "in_progress" {
		t.Fatalf("retire accepted child: %+v %v", retired, err)
	}
	assertPinned()
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_typed_action_retirements WHERE session_id=? AND idempotency_key=?`, []any{read.SessionID, child.IdempotencyKey}, 0)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	service, err = NewRPService(s, p, "chat_completions")
	if err != nil {
		t.Fatal(err)
	}
	assertPinned()
	result, err := service.ResumeRPInteraction(ctx, resume)
	if err != nil || result.Status != "settled" || result.InteractionID != interactionID || result.PlanKind != "ACTION" || result.NextStep != 1 || len(result.Outcomes) != 1 || p.interpretations != 1 || p.decisions != 1 {
		t.Fatalf("resume original plan: %+v calls=%d/%d err=%v", result, p.interpretations, p.decisions, err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_interaction_interpretations WHERE session_id=? AND idempotency_key=?`, []any{read.SessionID, request.IdempotencyKey}, 1)
	out := result.Outcomes[0]
	if out.Kind != "nonverbal" || out.TurnRunID == "" || out.EventID == "" || out.SettledSequence <= out.EventSequence {
		t.Fatalf("missing NPC batch settlement: %+v", out)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_turn_runs WHERE turn_run_id=? AND player_event_id=? AND player_turn_id IS NULL AND status='settled' AND settled_sequence=?`, []any{out.TurnRunID, out.EventID, out.SettledSequence}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_sequence=? AND actor_id=?`, []any{out.SettledSequence, M2RPNPCID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_interaction_pending_actions WHERE interaction_id=?`, []any{interactionID}, 0)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_interactions WHERE interaction_id=? AND pending_kind='' AND pending_request_json='{}'`, []any{interactionID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_utterances WHERE speaker_entity_id=?`, []any{M2RPPlayerID}, 0)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='RPNonverbalAction' AND actor_id=?`, []any{M2RPPlayerID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_npc_decisions WHERE parent_turn_id=? AND npc_entity_id=?`, []any{out.EventID, M2RPNPCID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_own_actions WHERE agent_id=? AND action='nonverbal' AND activity_code='nod'`, []any{M2RPNPCID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_npc_decisions WHERE parent_turn_id=? AND json_extract(proposal_json,'$.private.intent')=?`, []any{out.EventID, rpInteractionNodPrivate}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='RPNonverbalAction' AND actor_id=?`, []any{M2RPNPCID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM observation_records o JOIN events e ON e.event_id=o.source_event_id WHERE e.event_type='RPNonverbalAction' AND e.actor_id=? AND o.observer_agent_id=?`, []any{M2RPNPCID, M2RPPlayerID}, 1)
	counts := rpNodAcceptanceCounts(t, ctx, s)
	for _, retry := range []func() (RPInteractionResult, error){func() (RPInteractionResult, error) { return service.RunRPInteraction(ctx, request) }, func() (RPInteractionResult, error) { return service.ResumeRPInteraction(ctx, resume) }} {
		replay, err := retry()
		if err != nil || !reflect.DeepEqual(replay.Outcomes, result.Outcomes) || replay.Status != "settled" || p.interpretations != 1 || p.decisions != 1 || counts != rpNodAcceptanceCounts(t, ctx, s) {
			t.Fatalf("retry changed effects: %+v %v", replay, err)
		}
	}
	observation, err := s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	turns := 0
	for _, turn := range observation.RecentTurns {
		if turn.TurnRunID == out.TurnRunID {
			turns++
			public := strings.Join(turn.NarrativeLines, "\n")
			if !strings.Contains(public, "点头") || !strings.Contains(public, "我看见你向我点头了。") || strings.Contains(public, rpInteractionNodPrivate) {
				t.Fatalf("incorrect public history: %s", public)
			}
		}
	}
	if turns != 1 {
		t.Fatalf("history duplicated turn: %d", turns)
	}
	if differences, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatalf("projection replay: %+v %v", differences, err)
	}
}

func TestRPNonverbalInteractionAUTOIllegalFreshTargetCreatesNoIntent(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "illegal-nod.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, read, initial := newRPWaitTestSession(t, ctx, s)
	p := &rpInteractionNodProvider{t: t, target: "entity_not_visible"}
	service, err := NewRPService(s, p, "chat_completions")
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.RunRPInteraction(ctx, core.RPInteractionRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, Text: "向不存在的目标点头。", Mode: "AUTO", ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "illegal-fresh-nod"})
	if err == nil || p.interpretations != 1 || p.decisions != 0 {
		t.Fatalf("illegal target accepted: %v calls=%d/%d", err, p.interpretations, p.decisions)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_interactions`, nil, 0)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_interaction_pending_actions`, nil, 0)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_turn_runs`, nil, 0)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM commands WHERE command_type='RPNonverbalAction'`, nil, 0)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_own_actions WHERE action='nonverbal'`, nil, 0)
	// A rejected interpretation may retain its audit receipt, but creates no world intent/effect.
	var playerActions int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE event_type='RPNonverbalAction'`).Scan(&playerActions); err != nil || playerActions != 0 {
		t.Fatal(playerActions, err)
	}
}
