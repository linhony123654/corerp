package storage

import (
	"context"
	"database/sql"
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

func TestRPOwnPrivatePeerMemoryAndDialogueSurviveABAReturn(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "peer-private-memory.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	session, player, _ := newRPWaitTestSession(t, ctx, s)
	var peerEvents []string
	for i := 0; i < 2; i++ {
		view, err := s.ObserveRPSession(ctx, player)
		if err != nil {
			t.Fatal(err)
		}
		turn, err := s.RunRPTurn(ctx, core.RPSpeechRequest{PrincipalID: player.PrincipalID, SessionID: session.SessionID, Text: fmt.Sprintf("借书约定%d，缺页原因不要提。", i), ExpectedCursor: view.ObservationCursor, IdempotencyKey: fmt.Sprintf("peer-a-%d", i)}, rpDecisionProviderFunc(func(_ context.Context, input core.RPDecisionInput) (core.RPDecisionProposal, error) {
			return core.RPDecisionProposal{Action: "respond", Text: "书可以借，缺页的事不说。", Private: &core.RPDecisionPrivate{Intent: fmt.Sprintf("a-private-%d", i), RelationshipStance: "谨慎", BasisEventIDs: []string{input.SpeechEventID}}}, nil
		}))
		if err != nil || len(turn.NPCEventIDs) != 1 {
			t.Fatal(turn, err)
		}
		peerEvents = append(peerEvents, turn.NPCEventIDs[0])
	}
	grantRPControlForTest(t, ctx, s, M2AgentAdaID)
	otherSession, err := s.OpenRPSession(ctx, rpTestOpenRequest())
	if err != nil {
		t.Fatal(err)
	}
	other := core.RPSessionReadRequest{PrincipalID: rpTestPrincipal, SessionID: otherSession.SessionID}
	otherView, err := s.ObserveRPSession(ctx, other)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.MoveRP(ctx, core.RPMoveRequest{PrincipalID: other.PrincipalID, SessionID: other.SessionID, FromPlaceID: otherView.PlaceID, ToPlaceID: M2AgentCafeID, ExpectedCursor: otherView.ObservationCursor, IdempotencyKey: "peer-b-arrives"}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		view, err := s.ObserveRPSession(ctx, other)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.RunRPTurn(ctx, core.RPSpeechRequest{PrincipalID: other.PrincipalID, SessionID: other.SessionID, Text: "今天聊聊天气。", ExpectedCursor: view.ObservationCursor, IdempotencyKey: fmt.Sprintf("peer-b-%d", i)}, rpDecisionProviderFunc(func(context.Context, core.RPDecisionInput) (core.RPDecisionProposal, error) {
			return core.RPDecisionProposal{Action: "respond", Text: "天气不错。", Private: &core.RPDecisionPrivate{Intent: fmt.Sprintf("b-private-%d", i), RelationshipStance: "热情"}}, nil
		})); err != nil {
			t.Fatal(err)
		}
	}
	view, err := s.ObserveRPSession(ctx, player)
	if err != nil {
		t.Fatal(err)
	}
	speech, err := s.SpeakRP(ctx, core.RPSpeechRequest{PrincipalID: player.PrincipalID, SessionID: session.SessionID, Text: "Then let's proceed.", ExpectedCursor: view.ObservationCursor, IdempotencyKey: "peer-a-returns"})
	if err != nil {
		t.Fatal(err)
	}
	request := core.RPDecisionRequest{PrincipalID: player.PrincipalID, SessionID: session.SessionID, NPCEntityID: M2RPNPCID, TurnID: speech.TurnID}
	packet, err := s.BuildRPDecisionInput(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	assertRecall := func(input core.RPDecisionInput) {
		t.Helper()
		if len(input.RecentPrivateDecisions) != 4 {
			t.Fatalf("latest peer + global three not retained: %+v", input.RecentPrivateDecisions)
		}
		for i, memory := range input.RecentPrivateDecisions {
			if memory.SourceEventID == "" || memory.WorldTime == "" || (i > 0 && memory.EventSequence <= input.RecentPrivateDecisions[i-1].EventSequence) {
				t.Fatal("private peer recall lost date/source order")
			}
			if i == 0 {
				if memory.Private.Intent != "a-private-1" || memory.InterlocutorEntityID != input.InterlocutorEntityID || memory.Private.RelationshipStance != "谨慎" {
					t.Fatal("A's historical stance was replaced by B")
				}
			} else if memory.Private.Intent != fmt.Sprintf("b-private-%d", i) || memory.InterlocutorEntityID == input.InterlocutorEntityID {
				t.Fatal("global private history changed its directed peer")
			}
		}
		peers := 0
		for _, exchange := range input.RelevantDialogue {
			if !exchange.PeerContext {
				continue
			}
			if peers >= len(peerEvents) || len(exchange.Dialogue) != 2 || exchange.Dialogue[1].EventID != peerEvents[peers] {
				t.Fatalf("return peer's latest complete exchanges lost: %+v", exchange)
			}
			words, ok := core.ResolveRPDecisionSpeech(input, exchange.Dialogue[1].EventID, exchange.Dialogue[1].SpeakerEntityID)
			if !ok || words != "书可以借，缺页的事不说。" {
				t.Fatal("peer qualification lost in provider shape")
			}
			peers++
		}
		if peers != 2 || input.ContextSelection == nil || input.ContextSelection.EncodedBytes > input.ContextSelection.BudgetBytes {
			t.Fatalf("peer recall/budget missing: %d %+v", peers, input.ContextSelection)
		}
	}
	assertRecall(packet)
	provider, err := s.rpDecisionProviderView(ctx, packet)
	if err != nil {
		t.Fatal(err)
	}
	assertRecall(provider)
	providerHash, err := core.HashJSON(provider)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := s.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	waitInput := packet
	waitInput.PlayerSpeechText, waitInput.SpeechEventID, waitInput.TurnID = "", "", ""
	waitInput.RecentDialogue = nil
	waitInput.Trigger = &core.RPDecisionTrigger{Kind: "elapsed_time", SourceEventID: "sourced-wait"}
	waitDialogue, err := readRPRelevantDialogue(ctx, conn, waitInput)
	if err != nil || len(waitDialogue) != 2 || !waitDialogue[0].PeerContext || !waitDialogue[1].PeerContext {
		t.Fatalf("wait query lost existing peer conversation: %+v %v", waitDialogue, err)
	}
	_ = conn.Close()
	public, err := s.readRPNarrativeInput(ctx, session.SessionID, speech.TurnID, speech.EventID)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(public)
	if err != nil || strings.Contains(string(encoded), "a-private") || strings.Contains(string(encoded), "b-private") {
		t.Fatal("peer-private sketch leaked into narrator", err)
	}
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
	again, err := s.BuildRPDecisionInput(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	assertRecall(again)
	againProvider, err := s.rpDecisionProviderView(ctx, again)
	if err != nil {
		t.Fatal(err)
	}
	assertRecall(againProvider)
	againHash, err := core.HashJSON(againProvider)
	if err != nil || againHash != providerHash {
		t.Fatal("peer recall changed after rebuild/restart", err)
	}
}

func TestRPOwnPrivatePeerUnionPreservesApprovalAndCompleteHead(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "private-peer-source.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, schema := range []string{
		`CREATE TABLE events(event_id TEXT, instance_id TEXT, branch_id TEXT, actor_id TEXT, event_sequence INTEGER, world_time TEXT, batch_id TEXT)`,
		`CREATE TABLE rp_npc_decisions(decision_id TEXT, event_id TEXT, session_id TEXT, npc_entity_id TEXT, parent_turn_id TEXT, proposal_json TEXT, proposal_hash TEXT)`,
		`CREATE TABLE rp_sessions(session_id TEXT, controlled_entity_id TEXT)`,
		`CREATE TABLE event_batches(batch_id TEXT, command_id TEXT, attempt_no INTEGER, last_sequence INTEGER)`,
		`CREATE TABLE commands(command_id TEXT, status TEXT, command_type TEXT)`,
		`CREATE TABLE command_attempts(command_id TEXT, attempt_no INTEGER, status TEXT, proposal_hash TEXT)`,
	} {
		if _, err := db.ExecContext(ctx, schema); err != nil {
			t.Fatal(err)
		}
	}
	for i, peer := range []string{"a", "b", "b", "b"} {
		key := fmt.Sprintf("record-%d", i)
		proposal := core.RPDecisionProposal{Action: "silence", Private: &core.RPDecisionPrivate{Intent: key, RelationshipStance: "dated-subjective-stance"}}
		raw, err := core.CanonicalJSON(proposal)
		if err != nil {
			t.Fatal(err)
		}
		hash, err := core.HashJSON(proposal)
		if err != nil {
			t.Fatal(err)
		}
		for _, statement := range []struct {
			query string
			args  []any
		}{
			{`INSERT INTO events VALUES (?,'world','main','npc',?,'2026-10-01T00:00:00Z',?)`, []any{key, i*2 + 1, key}},
			{`INSERT INTO rp_npc_decisions VALUES (?,?,?,'npc',?,?,?)`, []any{key, key, key, key, string(raw), hash}},
			{`INSERT INTO rp_sessions VALUES (?,?)`, []any{key, peer}},
			{`INSERT INTO event_batches VALUES (?,?,1,?)`, []any{key, key, i*2 + 2}},
			{`INSERT INTO commands VALUES (?,'committed','RPNPCDecision')`, []any{key}},
			{`INSERT INTO command_attempts VALUES (?,1,'committed',?)`, []any{key, hash}},
		} {
			if _, err := db.ExecContext(ctx, statement.query, statement.args...); err != nil {
				t.Fatal(err)
			}
		}
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	input := core.RPDecisionInput{InstanceID: "world", BranchID: "main", NPCEntityID: "npc", InterlocutorEntityID: "a", HeadSequence: 8, TurnID: "new-turn"}
	memory, err := readRPOwnPrivateDecisionMemory(ctx, conn, input)
	if err != nil || len(memory) != 4 || memory[0].Private.Intent != "record-0" || memory[0].InterlocutorEntityID != "a" {
		t.Fatalf("peer predicate did not precede global-three limit: %+v %v", memory, err)
	}
	for _, scope := range []string{"peer-b", "no-peer", "current-turn", "partial-batch", "actor", "branch", "instance"} {
		bounded := input
		want := 0
		switch scope {
		case "peer-b":
			bounded.InterlocutorEntityID, want = "b", 3
		case "no-peer":
			bounded.InterlocutorEntityID, want = "", 3
		case "current-turn":
			bounded.TurnID, want = "record-0", 3
		case "partial-batch":
			bounded.HeadSequence = 1
		case "actor":
			bounded.NPCEntityID = "other"
		case "branch":
			bounded.BranchID = "other"
		case "instance":
			bounded.InstanceID = "other"
		}
		got, err := readRPOwnPrivateDecisionMemory(ctx, conn, bounded)
		if err != nil || len(got) != want {
			t.Fatalf("private union crossed %s: %+v %v", scope, got, err)
		}
	}
	for _, boundary := range []struct{ update, restore string }{
		{`UPDATE commands SET status='pending' WHERE command_id='record-0'`, `UPDATE commands SET status='committed' WHERE command_id='record-0'`},
		{`UPDATE command_attempts SET status='ready' WHERE command_id='record-0'`, `UPDATE command_attempts SET status='committed' WHERE command_id='record-0'`},
		{`UPDATE command_attempts SET proposal_hash='unapproved' WHERE command_id='record-0'`, `UPDATE command_attempts SET proposal_hash=(SELECT proposal_hash FROM rp_npc_decisions WHERE decision_id='record-0') WHERE command_id='record-0'`},
	} {
		if _, err := conn.ExecContext(ctx, boundary.update); err != nil {
			t.Fatal(err)
		}
		got, err := readRPOwnPrivateDecisionMemory(ctx, conn, input)
		if err != nil || len(got) != 3 {
			t.Fatalf("unapproved peer row entered memory: %+v %v", got, err)
		}
		if _, err := conn.ExecContext(ctx, boundary.restore); err != nil {
			t.Fatal(err)
		}
	}
	// These deliberately mutable, isolated source tables test the reader's
	// fail-closed hash boundary; the real ledger is never modified by this case.
	if _, err := conn.ExecContext(ctx, `UPDATE rp_npc_decisions SET proposal_json=json_set(proposal_json,'$.private.intent','changed') WHERE decision_id='record-0'`); err != nil {
		t.Fatal(err)
	}
	if _, err := readRPOwnPrivateDecisionMemory(ctx, conn, input); !core.HasCode(err, core.CodeProjectionDiverged) {
		t.Fatalf("peer-only historical record skipped approved hash verification: %v", err)
	}
}
