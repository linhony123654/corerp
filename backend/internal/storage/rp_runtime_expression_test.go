package storage

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPNPCExpressionIsCommittedVisiblePrivateAndReplayable(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "npc-expression.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	session, _, initial := newRPWaitTestSession(t, ctx, s)
	speech, err := s.SpeakRP(ctx, core.RPSpeechRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID, Text: "请到这边来。", ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "expression-player"})
	if err != nil {
		t.Fatal(err)
	}
	request := core.RPDecisionRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID, TurnID: speech.TurnID, NPCEntityID: M2RPNPCID}
	const secret = "只在心里计算离开时机"
	decision, err := s.DecideRP(ctx, request, rpDecisionProviderFunc(func(context.Context, core.RPDecisionInput) (core.RPDecisionProposal, error) {
		return core.RPDecisionProposal{Action: "respond", Text: "过来吧。", ExpressionCode: "beckon", Private: &core.RPDecisionPrivate{Intent: secret, Emotion: "平静", RelationshipStance: "谨慎"}}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	s.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "expression rollback") }
	if _, err := s.CommitRPDecision(ctx, request, decision); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatalf("atomic rollback failed: %v", err)
	}
	s.beforeCommit = nil
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='RPNonverbalAction'`, nil, 0)
	committed, err := s.CommitRPDecision(ctx, request, decision)
	if err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT event_count FROM event_batches b JOIN events e ON e.batch_id=b.batch_id WHERE e.event_id=?`, []any{committed.EventID}, 2)
	assertM2Value(t, ctx, s, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, []any{M2DemoInstanceID, M2DemoBranchID}, committed.EventSequence+1)
	var expressionID, raw string
	if err := s.db.QueryRowContext(ctx, `SELECT e.event_id,e.payload FROM events e JOIN events main ON main.batch_id=e.batch_id WHERE main.event_id=? AND e.event_type='RPNonverbalAction'`, committed.EventID).Scan(&expressionID, &raw); err != nil {
		t.Fatal(err)
	}
	var fact core.RPNonverbalFact
	if err := json.Unmarshal([]byte(raw), &fact); err != nil || fact.Action != "gesture" || fact.GestureCode != "beckon" || fact.TargetEntityID != M2RPPlayerID {
		t.Fatalf("wrong committed expression: %+v %v", fact, err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM observation_records WHERE source_event_id=? AND observer_agent_id=?`, []any{expressionID, M2RPPlayerID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_knowledge WHERE source_event_id=? AND observer_agent_id=?`, []any{expressionID, M2RPPlayerID}, 1)
	packet, err := s.BuildRPDecisionInput(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	remembered := false
	for _, action := range packet.OwnActions {
		if action.EventID == expressionID && action.Action == "nonverbal" {
			remembered = true
		}
	}
	if !remembered {
		t.Fatal("NPC cannot refer to its own committed expression in a later decision")
	}
	view, err := s.readRPNarrativeInput(ctx, session.SessionID, speech.TurnID, speech.EventID)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), secret) {
		t.Fatal("private NPC intent leaked into narrator input")
	}
	observatory, err := s.ReadRPObservatory(ctx, RPObservatoryRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID})
	if err != nil {
		t.Fatal(err)
	}
	publicTrace, err := json.Marshal(observatory)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(publicTrace), secret) {
		t.Fatal("private NPC intent leaked into player observatory")
	}
	found := false
	for _, f := range view.Facts {
		if f.EventID == expressionID && f.Action == "expression" && f.ExpressionCode == "beckon" {
			found = true
			if f.TargetActorID != M2RPPlayerID || f.TargetActorName == "" {
				t.Fatalf("narrative lost witnessed gesture recipient: %+v", f)
			}
		}
	}
	if !found {
		t.Fatalf("witnessed expression absent from public facts: %+v", view.Facts)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM outbox WHERE event_id=? AND payload LIKE ?`, []any{expressionID, "%" + secret + "%"}, 0)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	retry, err := s.CommitRPDecision(ctx, request, decision)
	if err != nil || !retry.Replayed || retry.EventID != committed.EventID {
		t.Fatalf("retry duplicated expression: %+v %v", retry, err)
	}
	if diffs, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diffs) != 0 {
		t.Fatalf("expression projections diverged: %v %v", diffs, err)
	}
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	if rebuilt, err := s.readRPNarrativeInput(ctx, session.SessionID, speech.TurnID, speech.EventID); err != nil {
		t.Fatal(err)
	} else if after, err := json.Marshal(rebuilt); err != nil || string(after) != string(encoded) {
		t.Fatal("witnessed recipient changed across rebuild/restart", err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM observation_records WHERE source_event_id=? AND observer_agent_id=?`, []any{expressionID, M2RPPlayerID}, 1)
}

func TestRPNPCMayAnswerSilentlyWithCommittedExpression(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "silent-expression.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	session, _, initial := newRPWaitTestSession(t, ctx, s)
	speech, err := s.SpeakRP(ctx, core.RPSpeechRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID, Text: "可以吗？", ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "silent-expression"})
	if err != nil {
		t.Fatal(err)
	}
	request := core.RPDecisionRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID, TurnID: speech.TurnID, NPCEntityID: M2RPNPCID}
	decision, err := s.DecideRP(ctx, request, rpDecisionProviderFunc(func(context.Context, core.RPDecisionInput) (core.RPDecisionProposal, error) {
		return core.RPDecisionProposal{Action: "silence", ExpressionCode: "nod"}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	committed, err := s.CommitRPDecision(ctx, request, decision)
	if err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, []any{M2DemoInstanceID, M2DemoBranchID}, committed.EventSequence+1)
	view, err := s.readRPNarrativeInput(ctx, session.SessionID, speech.TurnID, speech.EventID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, fact := range view.Facts {
		if fact.Action == "expression" && fact.ExpressionCode == "nod" {
			found = true
		}
	}
	if !found {
		t.Fatal("wordless NPC expression was not published to its witness")
	}
	if diffs, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diffs) != 0 {
		t.Fatalf("wordless expression did not replay: %v %v", diffs, err)
	}
}
