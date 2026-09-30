package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPInitiativePrivateDecisionNeverBecomesAnObservableEvent(t *testing.T) {
	for _, action := range []string{"respond", "silence"} {
		t.Run(action, func(t *testing.T) {
			ctx := context.Background()
			path := filepath.Join(t.TempDir(), "private-initiative.db")
			s := openBootstrappedStore(t, ctx, path)
			defer func() { _ = s.Close() }()
			session, _, view := newRPWaitTestSession(t, ctx, s)
			wait, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: "2026-09-22T03:00:00Z", Budget: 10, IdempotencyKey: "private-initiative"})
			if err != nil {
				t.Fatal(err)
			}
			request := core.RPInitiativeRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID, NPCEntityID: M2RPNPCID, TriggerEventID: wait.EventID}
			const secret = "只在心里斟酌如何安抚，尚未公开"
			proposal := core.RPDecisionProposal{Action: action, Private: &core.RPDecisionPrivate{Intent: secret, Emotion: "担心", RelationshipStance: "谨慎"}}
			if action == "respond" {
				proposal.Text = "若有话，不妨慢慢说。"
			}
			out, err := s.RunRPInitiative(ctx, request, rpDecisionProviderFunc(func(context.Context, core.RPDecisionInput) (core.RPDecisionProposal, error) { return proposal, nil }))
			if err != nil || out.Status != "validated" {
				t.Fatalf("private proposal failed: %+v %v", out, err)
			}
			var raw string
			if err := s.db.QueryRowContext(ctx, `SELECT payload FROM events WHERE event_id=?`, out.EventID).Scan(&raw); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(raw, secret) || strings.Contains(raw, `"private"`) {
				t.Fatal("NPC private decision became an observable Event payload")
			}
			if err := s.db.QueryRowContext(ctx, `SELECT proposal_json FROM rp_npc_decisions WHERE event_id=?`, out.EventID).Scan(&raw); err != nil || !strings.Contains(raw, secret) {
				t.Fatalf("restricted committed decision lost its private metadata: %v", err)
			}
			assertM2Value(t, ctx, s, `SELECT count(*) FROM outbox WHERE event_id=? AND payload LIKE ?`, []any{out.EventID, "%" + secret + "%"}, 0)
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
			if err != nil || !retry.Replayed || retry.EventID != out.EventID {
				t.Fatalf("private metadata change broke initiative recovery: %+v %v", retry, err)
			}
			if err := s.db.QueryRowContext(ctx, `SELECT proposal_json FROM rp_npc_decisions WHERE event_id=?`, out.EventID).Scan(&raw); err != nil || !strings.Contains(raw, secret) {
				t.Fatalf("rebuild/restart lost restricted metadata: %v", err)
			}
			view, err = s.ObserveRPSession(ctx, core.RPSessionReadRequest{PrincipalID: request.PrincipalID, SessionID: request.SessionID})
			if err != nil {
				t.Fatal(err)
			}
			speech, err := s.SpeakRP(ctx, core.RPSpeechRequest{PrincipalID: request.PrincipalID, SessionID: request.SessionID, Text: "你刚才在想什么？", ExpectedCursor: view.ObservationCursor, IdempotencyKey: "after-private-initiative"})
			if err != nil {
				t.Fatal(err)
			}
			packet, err := s.BuildRPDecisionInput(ctx, core.RPDecisionRequest{PrincipalID: request.PrincipalID, SessionID: request.SessionID, TurnID: speech.TurnID, NPCEntityID: request.NPCEntityID})
			if err != nil || len(packet.RecentPrivateDecisions) != 1 || packet.RecentPrivateDecisions[0].Private.Intent != secret {
				t.Fatalf("own applied initiative sketch did not survive into the next decision: %+v %v", packet.RecentPrivateDecisions, err)
			}
		})
	}
}

func TestRPOwnPrivateMemoryExcludesUnappliedProposalsAndSurvivesReplay(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "private-memory.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	session, player, initial := newRPWaitTestSession(t, ctx, s)
	speech, err := s.SpeakRP(ctx, core.RPSpeechRequest{PrincipalID: player.PrincipalID, SessionID: session.SessionID, Text: "我想慢慢说。", ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "private-first"})
	if err != nil {
		t.Fatal(err)
	}
	request := core.RPDecisionRequest{PrincipalID: player.PrincipalID, SessionID: session.SessionID, TurnID: speech.TurnID, NPCEntityID: M2RPNPCID}
	const secret = "先耐心听完，再决定怎样安抚"
	decision, err := s.DecideRP(ctx, request, rpDecisionProviderFunc(func(context.Context, core.RPDecisionInput) (core.RPDecisionProposal, error) {
		return core.RPDecisionProposal{Action: "respond", Text: "好，我听着。", Private: &core.RPDecisionPrivate{Intent: secret, Emotion: "关切", RelationshipStance: "耐心", BasisEventIDs: []string{speech.EventID}}}, nil
	}))
	if err != nil || decision.Status != "validated" {
		t.Fatal(decision, err)
	}
	checkNoMemory := func() {
		t.Helper()
		packet, err := s.BuildRPDecisionInput(ctx, request)
		if err != nil || len(packet.RecentPrivateDecisions) != 0 {
			t.Fatalf("audit-only/current-turn proposal became prior memory: %+v %v", packet.RecentPrivateDecisions, err)
		}
	}
	checkNoMemory()
	s.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "private rollback") }
	if _, err := s.CommitRPDecision(ctx, request, decision); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatal(err)
	}
	s.beforeCommit = nil
	checkNoMemory()
	committed, err := s.CommitRPDecision(ctx, request, decision)
	if err != nil {
		t.Fatal(err)
	}
	checkNoMemory() // The current turn is not presented as an earlier intention.
	view, err := s.ObserveRPSession(ctx, player)
	if err != nil {
		t.Fatal(err)
	}
	next, err := s.SpeakRP(ctx, core.RPSpeechRequest{PrincipalID: player.PrincipalID, SessionID: session.SessionID, Text: "你还愿意听吗？", ExpectedCursor: view.ObservationCursor, IdempotencyKey: "private-next"})
	if err != nil {
		t.Fatal(err)
	}
	request.TurnID = next.TurnID
	packet, err := s.BuildRPDecisionInput(ctx, request)
	if err != nil || len(packet.RecentPrivateDecisions) != 1 {
		t.Fatalf("applied private sketch missing from next decision: %+v %v", packet.RecentPrivateDecisions, err)
	}
	item := packet.RecentPrivateDecisions[0]
	if item.Private.Intent != secret || item.SourceEventID != committed.EventID || item.EventSequence != committed.EventSequence || item.DecisionID != committed.DecisionID || item.InterlocutorEntityID != M2RPPlayerID {
		t.Fatalf("private sketch lost actor/source attribution: %+v", item)
	}
	// A new validated but unapplied thought must not replace the prior one.
	const stagedSecret = "这条还没有获准的想法不得进入记忆"
	if _, err := s.DecideRP(ctx, request, rpDecisionProviderFunc(func(context.Context, core.RPDecisionInput) (core.RPDecisionProposal, error) {
		return core.RPDecisionProposal{Action: "respond", Text: "我在听。", Private: &core.RPDecisionPrivate{Intent: stagedSecret}}, nil
	})); err != nil {
		t.Fatal(err)
	}
	stableHash, err := core.HashJSON(packet)
	if err != nil {
		t.Fatal(err)
	}
	recheck, err := s.BuildRPDecisionInput(ctx, request)
	gotHash, hashErr := core.HashJSON(recheck)
	if err != nil || hashErr != nil || gotHash != stableHash {
		t.Fatal("audit-only metadata changed the committed-head context", err, hashErr)
	}
	conn, err := s.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, scope := range []string{"other-actor", "other-instance", "other-branch", "before-source"} {
		input := packet
		switch scope {
		case "other-actor":
			input.NPCEntityID = M2AgentAdaID
		case "other-instance":
			input.InstanceID = "other-instance"
		case "other-branch":
			input.BranchID = "other-branch"
		case "before-source":
			input.HeadSequence = committed.EventSequence - 1
		}
		memory, err := readRPOwnPrivateDecisionMemory(ctx, conn, input)
		if err != nil || len(memory) != 0 {
			t.Fatalf("private memory crossed %s boundary: %+v %v", scope, memory, err)
		}
	}
	_ = conn.Close()
	public, err := s.readRPNarrativeInput(ctx, session.SessionID, next.TurnID, next.EventID)
	if err != nil {
		t.Fatal(err)
	}
	observatory, err := s.ReadRPObservatory(ctx, RPObservatoryRequest{PrincipalID: player.PrincipalID, SessionID: session.SessionID})
	if err != nil {
		t.Fatal(err)
	}
	for _, publicView := range []any{public, observatory, view} {
		encoded, err := json.Marshal(publicView)
		if err != nil || strings.Contains(string(encoded), secret) || strings.Contains(string(encoded), stagedSecret) {
			t.Fatal("private history leaked into a public consumer", err)
		}
	}
	assertM2Value(t, ctx, s, `SELECT count(*) FROM events WHERE payload LIKE ?`, []any{"%" + secret + "%"}, 0)
	if err := s.RebuildProjections(ctx, packet.InstanceID, packet.BranchID); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	recheck, err = s.BuildRPDecisionInput(ctx, request)
	gotHash, hashErr = core.HashJSON(recheck)
	if err != nil || hashErr != nil || gotHash != stableHash {
		t.Fatalf("private context changed after rebuild/restart: %s %s %v %v", gotHash, stableHash, err, hashErr)
	}
}

func TestRPOwnPrivateDecisionHistoryIsBoundedAndChronological(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "bounded-private-memory.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	session, player, _ := newRPWaitTestSession(t, ctx, s)
	for i := 0; i < 6; i++ {
		view, err := s.ObserveRPSession(ctx, player)
		if err != nil {
			t.Fatal(err)
		}
		_, err = s.RunRPTurn(ctx, core.RPSpeechRequest{PrincipalID: player.PrincipalID, SessionID: session.SessionID, Text: "接着聊。", ExpectedCursor: view.ObservationCursor, IdempotencyKey: fmt.Sprintf("private-round-%d", i)}, rpDecisionProviderFunc(func(_ context.Context, input core.RPDecisionInput) (core.RPDecisionProposal, error) {
			if len(input.RecentPrivateDecisions) != min(i, 3) {
				t.Errorf("round %d: history is unbounded or missing", i)
			}
			for j, prior := range input.RecentPrivateDecisions {
				if prior.Private.Intent != fmt.Sprintf("intent-%d", max(0, i-3)+j) || prior.SourceEventID == "" || (j > 0 && prior.EventSequence <= input.RecentPrivateDecisions[j-1].EventSequence) {
					t.Errorf("round %d: history lost order or provenance: %+v", i, prior)
				}
			}
			return core.RPDecisionProposal{Action: "respond", Text: "我在听。", Private: &core.RPDecisionPrivate{Intent: fmt.Sprintf("intent-%d", i)}}, nil
		}))
		if err != nil {
			t.Fatal(err)
		}
	}
}
