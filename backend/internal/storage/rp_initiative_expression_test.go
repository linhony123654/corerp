package storage

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPInitiativeExpressionIsAtomicWitnessedPrivateAndReplayable(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "initiative-expression.db")
	s := openBootstrappedStore(t, ctx, path)
	defer func() { _ = s.Close() }()
	session, player, view := newRPWaitTestSession(t, ctx, s)
	wait, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: player.PrincipalID, SessionID: player.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: "2026-09-22T03:00:00Z", Budget: 10, IdempotencyKey: "initiative-expression"})
	if err != nil {
		t.Fatal(err)
	}
	request := core.RPInitiativeRequest{PrincipalID: player.PrincipalID, SessionID: player.SessionID, NPCEntityID: M2RPNPCID, TriggerEventID: wait.EventID}
	const secret = "先让对方近前，私下想听完再作打算"
	provider := rpDecisionProviderFunc(func(context.Context, core.RPDecisionInput) (core.RPDecisionProposal, error) {
		return core.RPDecisionProposal{Action: "respond", Text: "过来吧，我听着。", SpeechTone: "gentle", ExpressionCode: "beckon", Private: &core.RPDecisionPrivate{Intent: secret}}, nil
	})
	s.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "initiative expression rollback") }
	if _, err := s.RunRPInitiative(ctx, request, provider); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatal(err)
	}
	s.beforeCommit = nil
	assertM2Value(t, ctx, s, `SELECT count(*) FROM events WHERE event_type='RPNonverbalAction'`, nil, 0)
	result, err := s.RunRPInitiative(ctx, request, provider)
	if err != nil || result.Status != "validated" {
		t.Fatal(result, err)
	}
	connForTone, err := s.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	recordedTone, toneErr := readRPRecordedSpeechTone(ctx, connForTone, result.EventID, session.ControlledEntityID)
	_ = connForTone.Close()
	if toneErr != nil || recordedTone != "gentle" {
		t.Fatal("initiative speech lost its accepted observable delivery", recordedTone, toneErr)
	}
	var expressionID, raw string
	if err := s.db.QueryRowContext(ctx, `SELECT e.event_id,e.payload FROM events e JOIN events main ON main.batch_id=e.batch_id WHERE main.event_id=? AND e.event_type='RPNonverbalAction'`, result.EventID).Scan(&expressionID, &raw); err != nil {
		t.Fatalf("approved initiative gesture has no committed observable: %v", err)
	}
	var fact core.RPNonverbalFact
	if err := json.Unmarshal([]byte(raw), &fact); err != nil || fact.GestureCode != "beckon" || fact.TargetEntityID != session.ControlledEntityID || strings.Contains(raw, secret) {
		t.Fatalf("gesture lost public attribution or leaked private intent: %+v %v", fact, err)
	}
	assertM2Value(t, ctx, s, `SELECT event_count FROM event_batches b JOIN events e ON e.batch_id=b.batch_id WHERE e.event_id=?`, []any{result.EventID}, 2)
	assertM2Value(t, ctx, s, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, []any{M2DemoInstanceID, M2DemoBranchID}, result.EventSequence+1)
	assertM2Value(t, ctx, s, `SELECT count(*) FROM observation_records WHERE source_event_id=? AND observer_agent_id=?`, []any{expressionID, session.ControlledEntityID}, 1)
	if fact.TriggerEventID != wait.EventID {
		t.Fatal("gesture lost the source wait lineage")
	}
	// A second effect in the same initiative must not consume two decisions.
	if _, err := s.db.ExecContext(ctx, `UPDATE agent_profiles SET action_budget_per_day=2 WHERE agent_id=?`, M2RPNPCID); err != nil {
		t.Fatal(err)
	}
	conn, err := s.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	eligible, err := rpInitiativeEligible(ctx, conn, core.RPDecisionInput{InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, NPCEntityID: M2RPNPCID, WorldTime: "2026-09-22T04:00:00Z"})
	if err != nil || !eligible {
		t.Fatal("compound initiative consumed two daily decisions", err)
	}
	partial, err := readRPOwnPrivateDecisionMemory(ctx, conn, core.RPDecisionInput{InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, NPCEntityID: M2RPNPCID, HeadSequence: result.EventSequence})
	if err != nil || len(partial) != 0 {
		t.Fatal("private memory crossed an incomplete atomic-batch head", err)
	}
	_ = conn.Close()
	settled, err := (&RPService{Store: s}).rpInteractionWaitSettledSequence(ctx, session.SessionID, wait.EventID, wait.EventSequence)
	if err != nil || settled != result.EventSequence+1 {
		t.Fatalf("wait continuation lost its own gesture lineage: %d %v", settled, err)
	}
	assertM2Value(t, ctx, s, `SELECT count(*) FROM events WHERE payload LIKE ?`, []any{"%" + secret + "%"}, 0)
	assertM2Value(t, ctx, s, `SELECT count(*) FROM outbox WHERE payload LIKE ?`, []any{"%" + secret + "%"}, 0)
	if differences, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatal(differences, err)
	}
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := s.RunRPInitiative(ctx, request, nil)
	if err != nil || !retry.Replayed || retry.EventID != result.EventID {
		t.Fatalf("compound initiative replay did not find its main Event: %+v %v", retry, err)
	}
	view, err = s.ObserveRPSession(ctx, player)
	if err != nil {
		t.Fatal(err)
	}
	speech, err := s.SpeakRP(ctx, core.RPSpeechRequest{PrincipalID: player.PrincipalID, SessionID: player.SessionID, Text: "你刚才招手叫我，有话要说吗？", ExpectedCursor: view.ObservationCursor, IdempotencyKey: "reference-initiative-gesture"})
	if err != nil {
		t.Fatal(err)
	}
	packet, err := s.BuildRPDecisionInput(ctx, core.RPDecisionRequest{PrincipalID: player.PrincipalID, SessionID: player.SessionID, TurnID: speech.TurnID, NPCEntityID: M2RPNPCID})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, action := range packet.OwnActions {
		found = found || action.EventID == expressionID && action.Action == "nonverbal"
	}
	if !found || len(packet.RecentPrivateDecisions) != 1 || packet.RecentPrivateDecisions[0].Private.Intent != secret {
		t.Fatal("next decision cannot continue its own committed gesture and private intent")
	}
	var head int64
	if err := s.db.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&head); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ConfigureStudioAccessLocal(ctx, StudioAccessRequest{Explain: true, Binding: core.CareerBinding{PrincipalID: "principal_operator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: head, IdempotencyKey: "expression-inspector"}, TargetPrincipalID: "principal_creator", Status: "active"}); err != nil {
		t.Fatal(err)
	}
	explained, err := s.ReadStudioExplanation(ctx, StudioExplanationRequest{PrincipalID: "principal_creator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EventID: expressionID})
	if err != nil || explained.CommittedDecision == nil || explained.CommittedDecision.SourceKind != "npc_initiative_command" || explained.CommittedDecision.TriggerEventID != wait.EventID || explained.CommittedDecision.Status != "validated" {
		t.Fatalf("gesture explanation lost its approved parent decision and trigger: %+v %v", explained, err)
	}
	encoded, err := json.Marshal(explained)
	if err != nil || strings.Contains(string(encoded), secret) {
		t.Fatal("gesture explanation exposed private motivation", err)
	}
	public, err := s.readRPNarrativeInput(ctx, player.SessionID, speech.TurnID, speech.EventID)
	if err != nil {
		t.Fatal(err)
	}
	found = false
	gestureIndex, playerIndex := -1, -1
	for i, observable := range public.Facts {
		if observable.EventID == expressionID && observable.ExpressionCode == "beckon" {
			found, gestureIndex = true, i
			if observable.TargetActorID != session.ControlledEntityID || observable.TargetActorName == "" {
				t.Fatalf("next-turn gesture lost its witnessed recipient: %+v", observable)
			}
		}
		if observable.EventID == speech.EventID {
			playerIndex = i
		}
	}
	if !found || gestureIndex >= playerIndex {
		t.Fatal("witnessed initiative gesture disappeared from the next public narrative")
	}
}

func TestRPNPCExpressionNotRetroactivelyVisibleInNarration(t *testing.T) {
	ctx := context.Background()
	s := openBootstrappedStore(t, ctx, filepath.Join(t.TempDir(), "hidden-initiative-expression.db"))
	defer s.Close()
	session, player, _ := newRPWaitTestSession(t, ctx, s)
	link := RPPerceptionLinkRequest{Binding: careerTestBinding(t, s, "principal_creator", "hidden-expression-link"), PlaceID: M2AgentCafeID, ZoneA: "far", ZoneB: "main", BarrierKind: "open", BarrierState: "open", DistanceM: 25, VisualRangeM: 0, AudioRangeM: 30}
	if _, err := s.DefineRPPerceptionLink(ctx, link); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PlaceRPActorInZone(ctx, RPActorZoneRequest{Binding: careerTestBinding(t, s, "principal_creator", "hidden-expression-actor"), AgentID: M2RPNPCID, PlaceID: M2AgentCafeID, ZoneKey: "far"}); err != nil {
		t.Fatal(err)
	}
	view, err := s.ObserveRPSession(ctx, player)
	if err != nil || len(view.PresentEntities) != 0 {
		t.Fatal("fixture NPC should not be visible", err)
	}
	first, err := s.SpeakRP(ctx, core.RPSpeechRequest{PrincipalID: player.PrincipalID, SessionID: session.SessionID, Text: "远处的人，听得见吗？", DeliveryChannel: "shout", ExpectedCursor: view.ObservationCursor, IdempotencyKey: "hidden-expression-speech"})
	if err != nil {
		t.Fatal(err)
	}
	const secret = "私下觉得对方烦扰，但没有说出口"
	provider := rpDecisionProviderFunc(func(context.Context, core.RPDecisionInput) (core.RPDecisionProposal, error) {
		return core.RPDecisionProposal{Action: "silence", ExpressionCode: "frown", Private: &core.RPDecisionPrivate{Intent: secret}}, nil
	})
	request := core.RPDecisionRequest{PrincipalID: player.PrincipalID, SessionID: session.SessionID, NPCEntityID: M2RPNPCID, TurnID: first.TurnID}
	decision, err := s.DecideRP(ctx, request, provider)
	if err != nil || decision.Status != "validated" {
		t.Fatal(decision, err)
	}
	result, err := s.CommitRPDecision(ctx, request, decision)
	if err != nil || result.EventID == "" {
		t.Fatal(result, err)
	}
	var expressionID string
	if err := s.db.QueryRowContext(ctx, `SELECT e.event_id FROM events e JOIN events main ON main.batch_id=e.batch_id WHERE main.event_id=? AND e.event_type='RPNonverbalAction'`, result.EventID).Scan(&expressionID); err != nil {
		t.Fatal("hidden approved expression was not committed", err)
	}
	assertM2Value(t, ctx, s, `SELECT count(*) FROM observation_records WHERE source_event_id=? AND observer_agent_id=?`, []any{expressionID, session.ControlledEntityID}, 0)
	current, err := s.readRPNarrativeInput(ctx, session.SessionID, first.TurnID, first.EventID)
	if err != nil {
		t.Fatal(err)
	}
	for _, fact := range current.Facts {
		if fact.EventID == expressionID || fact.ExpressionCode == "frown" {
			t.Fatalf("unwitnessed same-turn expression entered narration: %+v", fact)
		}
	}
	// Later visibility cannot grant observation of an earlier expression.
	if _, err := s.PlaceRPActorInZone(ctx, RPActorZoneRequest{Binding: careerTestBinding(t, s, "principal_creator", "reveal-expression-actor"), AgentID: M2RPNPCID, PlaceID: M2AgentCafeID, ZoneKey: "main"}); err != nil {
		t.Fatal(err)
	}
	view, err = s.ObserveRPSession(ctx, player)
	if err != nil || len(view.PresentEntities) != 1 {
		t.Fatal("fixture NPC should now be visible", err)
	}
	speech, err := s.SpeakRP(ctx, core.RPSpeechRequest{PrincipalID: player.PrincipalID, SessionID: session.SessionID, Text: "现在看得见你了。", ExpectedCursor: view.ObservationCursor, IdempotencyKey: "after-hidden-expression"})
	if err != nil {
		t.Fatal(err)
	}
	readPublic := func() core.RPNarrativeInput {
		t.Helper()
		public, err := s.readRPNarrativeInput(ctx, session.SessionID, speech.TurnID, speech.EventID)
		if err != nil {
			t.Fatal(err)
		}
		for _, fact := range public.Facts {
			if fact.EventID == expressionID || fact.ExpressionCode == "frown" {
				t.Fatalf("unwitnessed past expression entered narration: %+v", fact)
			}
		}
		encoded, err := json.Marshal(public)
		if err != nil || strings.Contains(string(encoded), secret) {
			t.Fatal("private intent entered narration", err)
		}
		return public
	}
	before, _ := core.HashJSON(readPublic())
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	after, _ := core.HashJSON(readPublic())
	if before != after {
		t.Fatal("replay changed historical expression visibility")
	}
}
