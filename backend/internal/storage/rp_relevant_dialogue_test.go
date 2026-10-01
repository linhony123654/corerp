package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"corerp.local/backend/internal/core"
)

func TestRPRelevantDialogueRecallsOldExchangeWithHearingAndRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "relevant-dialogue.db")
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	session, player, _ := newRPWaitTestSession(t, ctx, store)
	const firstPlayer = "明天请把那本蓝皮诗集带来，好吗？"
	const firstNPC = "好，蓝皮诗集我会带来，不过其中缺了末页。"
	var firstEvent string
	for i := 0; i < 43; i++ {
		view, err := store.ObserveRPSession(ctx, player)
		if err != nil {
			t.Fatal(err)
		}
		speech, reply := "今天说说天气。", "我听见了。"
		if i == 0 {
			speech, reply = firstPlayer, firstNPC
		}
		turn, err := store.RunRPTurn(ctx, core.RPSpeechRequest{PrincipalID: player.PrincipalID, SessionID: session.SessionID, Text: speech, ExpectedCursor: view.ObservationCursor, IdempotencyKey: fmt.Sprintf("memory-%d", i)}, rpDecisionProviderFunc(func(context.Context, core.RPDecisionInput) (core.RPDecisionProposal, error) {
			return core.RPDecisionProposal{Action: "respond", Text: reply}, nil
		}))
		if err != nil || len(turn.NPCEventIDs) != 1 {
			t.Fatalf("turn %d: %+v %v", i, turn, err)
		}
		if i == 0 {
			firstEvent = turn.NPCEventIDs[0]
		}
	}
	// An offsite conversation on exactly the queried topic must never become
	// a candidate, even if it would otherwise outrank the older heard exchange.
	grantRPControlForTest(t, ctx, store, M2AgentAdaID)
	ada, err := store.OpenRPSession(ctx, rpTestOpenRequest())
	if err != nil {
		t.Fatal(err)
	}
	adaRequest := core.RPSessionReadRequest{PrincipalID: rpTestPrincipal, SessionID: ada.SessionID}
	adaView, err := store.ObserveRPSession(ctx, adaRequest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SpeakRP(ctx, core.RPSpeechRequest{PrincipalID: rpTestPrincipal, SessionID: ada.SessionID, Text: "蓝皮诗集的秘密藏在无人听见的地方。", ExpectedCursor: adaView.ObservationCursor, IdempotencyKey: "unheard-book"}); err != nil {
		t.Fatal(err)
	}
	view, err := store.ObserveRPSession(ctx, player)
	if err != nil {
		t.Fatal(err)
	}
	speech, err := store.SpeakRP(ctx, core.RPSpeechRequest{PrincipalID: player.PrincipalID, SessionID: session.SessionID, Text: "那本蓝皮诗集，你当时怎么答应我的？", ExpectedCursor: view.ObservationCursor, IdempotencyKey: "recall-book"})
	if err != nil {
		t.Fatal(err)
	}
	request := core.RPDecisionRequest{PrincipalID: player.PrincipalID, SessionID: session.SessionID, TurnID: speech.TurnID, NPCEntityID: M2RPNPCID}
	input, err := store.BuildRPDecisionInput(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range input.RecentDialogue {
		if d.Text == firstNPC {
			t.Fatal("old reply still in recent window")
		}
	}
	for _, d := range input.OwnActions {
		if d.EventID == firstEvent {
			t.Fatal("old reply still in own-action window")
		}
	}
	for _, d := range input.HeardPlayerHistory {
		if d.Excerpt == firstPlayer {
			t.Fatal("old request still in player history window")
		}
	}
	var dialogue []core.RPDecisionDialogue
	for _, exchange := range input.RelevantDialogue {
		for _, d := range exchange.Dialogue {
			if d.EventID == firstEvent {
				dialogue = exchange.Dialogue
			}
		}
	}
	// A second group can legitimately repair an exchange split by the recent
	// window. Locate the recalled topic by provenance rather than assuming it
	// must be the only relevant group.
	if len(dialogue) != 2 {
		t.Fatalf("missing old exchange: %+v", input.RelevantDialogue)
	}
	if dialogue[0].Text != firstPlayer || dialogue[1].Text != firstNPC || dialogue[1].EventID != firstEvent || dialogue[0].EventID == "" || dialogue[1].WorldTime == "" {
		t.Fatalf("lost verbatim words/order/provenance: %+v", dialogue)
	}
	if !core.RPDecisionEvidenceEventIDs(input)[firstEvent] {
		t.Fatal("retrieved source missing from allowed grounding")
	}
	// Exercise the SQL reader with only the old reply still in the recent view.
	// Its earlier request/qualification must remain available as one exchange.
	conn, err := store.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	split := input
	split.RecentDialogue = []core.RPDecisionDialogue{dialogue[1]}
	retrieved, err := readRPRelevantDialogue(ctx, conn, split)
	var splitExchange *core.RPDecisionExchange
	for i := range retrieved {
		if len(retrieved[i].Dialogue) == 2 && retrieved[i].Dialogue[1].EventID == firstEvent {
			splitExchange = &retrieved[i]
		}
	}
	if err != nil || splitExchange == nil || !splitExchange.RecentContext || splitExchange.Dialogue[0] != dialogue[0] || splitExchange.Dialogue[1] != dialogue[1] {
		t.Fatalf("SQL retrieval lost the split exchange: %+v %v", retrieved, err)
	}
	for _, scope := range []string{"other-branch", "other-instance", "before-question"} {
		bounded := split
		switch scope {
		case "other-branch":
			bounded.BranchID = "unrelated-branch"
		case "other-instance":
			bounded.InstanceID = "unrelated-instance"
		case "before-question":
			if err := conn.QueryRowContext(ctx, `SELECT event_sequence-1 FROM events WHERE event_id=?`, dialogue[0].EventID).Scan(&bounded.HeadSequence); err != nil {
				t.Fatal(err)
			}
		}
		if got, err := readRPRelevantDialogue(ctx, conn, bounded); err != nil || len(got) != 0 {
			t.Fatalf("retrieval crossed %s: %+v %v", scope, got, err)
		}
	}
	_ = conn.Close()
	providerView, err := store.rpDecisionProviderView(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(providerView)
	if strings.Contains(string(encoded), "无人听见") {
		t.Fatal("unheard speech leaked into decision packet")
	}
	assertRecentRepair := func(packet core.RPDecisionInput) {
		t.Helper()
		repairs := 0
		for _, exchange := range packet.RelevantDialogue {
			if !exchange.RecentContext {
				continue
			}
			repairs++
			if len(exchange.Dialogue) != 2 {
				t.Fatalf("recent repair lost a member: %+v", exchange)
			}
			for i, d := range exchange.Dialogue {
				words, ok := core.ResolveRPDecisionSpeech(packet, d.EventID, d.SpeakerEntityID)
				want := []string{"今天说说天气。", "我听见了。"}[i]
				if !ok || words != want {
					t.Fatalf("recent repair lost exact attributed words: %+v %q", d, words)
				}
			}
		}
		if repairs != 1 || packet.ContextSelection == nil || packet.ContextSelection.EncodedBytes > packet.ContextSelection.BudgetBytes {
			t.Fatalf("recent repair marker/budget missing: %+v", packet.ContextSelection)
		}
	}
	assertRecentRepair(input)
	assertRecentRepair(providerView)
	providerHash, err := core.HashJSON(providerView)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := core.HashJSON(input)
	for i := 0; i < 4; i++ {
		again, err := store.BuildRPDecisionInput(ctx, request)
		if err != nil {
			t.Fatal(err)
		}
		after, _ := core.HashJSON(again)
		if before != after {
			t.Fatal("same-head/restart input selection changed")
		}
		assertRecentRepair(again)
		againView, err := store.rpDecisionProviderView(ctx, again)
		if err != nil {
			t.Fatal(err)
		}
		assertRecentRepair(againView)
		againHash, err := core.HashJSON(againView)
		if err != nil || againHash != providerHash {
			t.Fatal("same-head/restart provider repair selection changed", err)
		}
	}
}

func TestRPRelevantDialoguePeerSelectionOnParaphraseAndWait(t *testing.T) {
	var candidates []rpDialogueCandidate
	for i := 0; i < 3; i++ {
		for j, speaker := range []string{"a", "npc"} {
			candidates = append(candidates, rpDialogueCandidate{dialogue: core.RPDecisionDialogue{SpeakerEntityID: speaker, Text: "保密的借书约定。", EventID: fmt.Sprintf("peer-%d-%d", i, j)}, sequence: int64(i*2 + j + 1), session: "s", turn: fmt.Sprint(i), groupSize: 2, peer: true})
		}
	}
	for _, query := range []string{"Then let's proceed.", ""} {
		input := core.RPDecisionInput{PlayerSpeechText: query, NPCEntityID: "npc", InterlocutorEntityID: "a"}
		got := selectRPRelevantDialogue(input, candidates)
		if len(got) != 2 || !got[0].PeerContext || !got[1].PeerContext || got[0].Dialogue[0].EventID != "peer-1-0" || got[1].Dialogue[0].EventID != "peer-2-0" {
			t.Fatalf("latest two peer exchanges lost on query %q: %+v", query, got)
		}
		input.SpeechEventID = "peer-2-0"
		got = selectRPRelevantDialogue(input, candidates)
		if len(got) != 2 || got[1].Dialogue[0].EventID != "peer-1-0" {
			t.Fatalf("current peer group displaced older history: %+v", got)
		}
	}
}

func TestRPRelevantDialoguePeerCandidatesPrecedeGlobalLimit(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "peer-candidates.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, schema := range []string{
		`CREATE TABLE events(event_id TEXT PRIMARY KEY, instance_id TEXT, branch_id TEXT, event_sequence INTEGER, world_time TEXT, payload TEXT)`,
		`CREATE TABLE rp_utterances(event_id TEXT, speaker_entity_id TEXT, speech_text TEXT, world_time TEXT, session_id TEXT, turn_id TEXT)`,
		`CREATE TABLE observation_records(source_event_id TEXT, observer_agent_id TEXT, subject_agent_id TEXT, claim_key TEXT, claim_payload TEXT)`,
	} {
		if _, err := db.ExecContext(ctx, schema); err != nil {
			t.Fatal(err)
		}
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	insert := func(sequence int, speaker, turn, branch string, heard bool) {
		t.Helper()
		id := fmt.Sprintf("event-%d", sequence)
		payload, _ := json.Marshal(rpSpeechEvent{ParentTurnID: turn, SpeakerEntityID: speaker, UtteranceID: "utterance-" + id, Text: "words-" + id, SpeechAct: "statement", ListenerIDs: []string{"npc"}})
		if _, err := tx.ExecContext(ctx, `INSERT INTO events VALUES (?,'world',?,?, '2026-10-01T00:00:00Z',?)`, id, branch, sequence, string(payload)); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO rp_utterances VALUES (?,?,?,'2026-10-01T00:00:00Z','session',?)`, id, speaker, "words-"+id, turn); err != nil {
			t.Fatal(err)
		}
		if heard {
			claim, _ := json.Marshal(rpSpeechClaim{ClaimType: "speaker_said", SpeakerEntityID: speaker, UtteranceID: "utterance-" + id, Text: "words-" + id, SpeechAct: "statement"})
			if _, err := tx.ExecContext(ctx, `INSERT INTO observation_records VALUES (?,'npc',?,?,?)`, id, speaker, "speech:"+id, string(claim)); err != nil {
				t.Fatal(err)
			}
		}
	}
	insert(1, "a", "a-first", "main", true)
	insert(2, "npc", "a-first", "main", false)
	insert(3, "a", "a-second", "main", true)
	insert(4, "npc", "a-second", "main", false)
	for i := 5; i < 605; i++ {
		insert(i, "npc", fmt.Sprint(i), "main", false)
	}
	// Same-peer words without hearing, beyond the head, or on another branch
	// must not supply peer membership, even with an actor-owned reply.
	insert(605, "a", "unheard", "main", false)
	insert(606, "npc", "unheard", "main", false)
	insert(607, "a", "future", "main", true)
	insert(608, "npc", "future", "main", false)
	insert(609, "a", "other-branch", "other", true)
	insert(610, "npc", "other-branch", "other", false)
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	input := core.RPDecisionInput{InstanceID: "world", BranchID: "main", NPCEntityID: "npc", InterlocutorEntityID: "a", HeadSequence: 606, PlayerSpeechText: "Then let's proceed."}
	candidates, err := readRPDialogueCandidates(ctx, conn, input)
	if err != nil || len(candidates) != rpDialogueCandidateLimit {
		t.Fatalf("candidate bound changed: %d %v", len(candidates), err)
	}
	for _, query := range []string{input.PlayerSpeechText, ""} {
		input.PlayerSpeechText = query
		got, err := readRPRelevantDialogue(ctx, conn, input)
		if err != nil || len(got) != 2 || !got[0].PeerContext || !got[1].PeerContext || len(got[0].Dialogue) != 2 || len(got[1].Dialogue) != 2 || got[0].Dialogue[0].EventID != "event-1" || got[1].Dialogue[1].EventID != "event-4" {
			t.Fatalf("peer exchanges lost behind >512 newer utterances: %+v %v", got, err)
		}
		before, _ := core.HashJSON(got)
		again, err := readRPRelevantDialogue(ctx, conn, input)
		after, _ := core.HashJSON(again)
		if err != nil || before != after {
			t.Fatal("peer selection was not deterministic", err)
		}
	}
	peerDialogue, err := readRPRelevantDialogue(ctx, conn, input)
	if err != nil {
		t.Fatal(err)
	}
	packet := input
	packet.RelevantDialogue = peerDialogue
	packet.RecentDialogue = []core.RPDecisionDialogue{peerDialogue[1].Dialogue[1]}
	selected, err := core.SelectRPDecisionContext(packet, core.DefaultRPDecisionContextBudgetBytes)
	if err != nil || len(selected.RelevantDialogue) != 2 || !selected.RelevantDialogue[0].PeerContext || !selected.RelevantDialogue[1].PeerContext {
		t.Fatalf("peer units lost in final selection: %+v %v", selected.RelevantDialogue, err)
	}
	base, err := core.SelectRPDecisionContext(input, core.DefaultRPDecisionContextBudgetBytes)
	if err != nil {
		t.Fatal(err)
	}
	// Leave room for the additional omission counters, but not even one
	// complete two-utterance group.
	limited, err := core.SelectRPDecisionContext(packet, base.ContextSelection.EncodedBytes+96)
	if err != nil || len(limited.RelevantDialogue) != 0 || len(limited.RecentDialogue) != 0 || limited.ContextSelection.Omitted.RelevantExchanges != 2 || limited.ContextSelection.EncodedBytes > limited.ContextSelection.BudgetBytes {
		t.Fatalf("byte budget substituted a lone reply for omitted peer units: %+v %v", limited, err)
	}
	input.InterlocutorEntityID = "unheard-peer"
	if got, err := readRPRelevantDialogue(ctx, conn, input); err != nil || len(got) != 0 {
		t.Fatalf("another peer inherited A's dialogue: %+v %v", got, err)
	}
}

func TestRPRelevantDialoguePeerRecallEntersActualWaitProviderView(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "peer-wait.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	session, player, initial := newRPWaitTestSession(t, ctx, s)
	const question = "那本诗集可以借，但缺页的原因请别问。"
	const reply = "好，借书的事照办，缺页的事不问。"
	turn, err := s.RunRPTurn(ctx, core.RPSpeechRequest{PrincipalID: player.PrincipalID, SessionID: session.SessionID, Text: question, ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "peer-before-wait"}, rpDecisionProviderFunc(func(context.Context, core.RPDecisionInput) (core.RPDecisionProposal, error) {
		return core.RPDecisionProposal{Action: "respond", Text: reply, Private: &core.RPDecisionPrivate{Intent: "dated-peer-sketch", RelationshipStance: "尊重边界"}}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	if turn.Status != "settled" {
		t.Fatalf("pre-wait dialogue did not settle: %+v", turn)
	}
	current, err := s.ObserveRPSession(ctx, player)
	if err != nil {
		t.Fatal(err)
	}
	wait, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: player.PrincipalID, SessionID: session.SessionID, ExpectedCursor: current.ObservationCursor, TargetWorldTime: "2026-09-22T03:00:00Z", Budget: 10, IdempotencyKey: "peer-recall-wait"})
	if err != nil {
		t.Fatal(err)
	}
	input, err := s.BuildRPInitiativeInput(ctx, core.RPInitiativeRequest{PrincipalID: player.PrincipalID, SessionID: session.SessionID, NPCEntityID: M2RPNPCID, TriggerEventID: wait.EventID})
	if err != nil {
		t.Fatal(err)
	}
	provider, err := s.rpDecisionProviderView(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if provider.PlayerSpeechText != "" || provider.SpeechEventID != "" || provider.Trigger == nil || provider.Trigger.Kind != "elapsed_time" || len(provider.RelevantDialogue) != 1 || !provider.RelevantDialogue[0].PeerContext || len(provider.RelevantDialogue[0].Dialogue) != 2 || len(provider.RecentPrivateDecisions) != 1 || provider.RecentPrivateDecisions[0].Private.Intent != "dated-peer-sketch" {
		t.Fatalf("actual wait invented speech or lost directed recall: %+v", provider)
	}
	for i, d := range provider.RelevantDialogue[0].Dialogue {
		words, ok := core.ResolveRPDecisionSpeech(provider, d.EventID, d.SpeakerEntityID)
		if !ok || words != []string{question, reply}[i] {
			t.Fatalf("wait provider lost exact attributed words: %+v %q", d, words)
		}
	}
}

func TestRPRelevantDialogueSelectionHasBoundedExactQuotes(t *testing.T) {
	input := core.RPDecisionInput{PlayerSpeechText: "蓝皮诗集"}
	var candidates []rpDialogueCandidate
	for i := 0; i < 10; i++ {
		candidates = append(candidates, rpDialogueCandidate{dialogue: core.RPDecisionDialogue{Text: "蓝皮诗集" + strings.Repeat("甲", 1996), EventID: fmt.Sprint(i)}, sequence: int64(i + 1), session: "s", turn: fmt.Sprint(i)})
	}
	// An uncommon topic in the full authorized set remains eligible.
	for i := 10; i < 40; i++ {
		candidates = append(candidates, rpDialogueCandidate{dialogue: core.RPDecisionDialogue{Text: "天气晴朗", EventID: fmt.Sprint(i)}, sequence: int64(i + 1), session: "s", turn: fmt.Sprint(i)})
	}
	got := selectRPRelevantDialogue(input, candidates)
	count := 0
	for _, exchange := range got {
		for _, d := range exchange.Dialogue {
			n := utf8.RuneCountInString(d.Text)
			count += n
			if n != 2000 {
				t.Fatal("retrieval truncated a quote")
			}
		}
	}
	if len(got) != 3 || count > rpRelevantDialogueRuneBudget {
		t.Fatalf("unexpected budget result: %d exchanges, %d runes", len(got), count)
	}
	input.PlayerSpeechText = "星际航线"
	if len(selectRPRelevantDialogue(input, candidates)) != 0 {
		t.Fatal("unrelated history selected without lexical evidence")
	}
	input.PlayerSpeechText = "蓝皮诗集"
	input.RecentDialogue = []core.RPDecisionDialogue{{EventID: "9"}}
	for _, exchange := range selectRPRelevantDialogue(input, candidates) {
		for _, d := range exchange.Dialogue {
			if d.EventID == "9" {
				t.Fatal("duplicated recent exchange")
			}
		}
	}
}

func TestRPRelevantDialogueRetainsMissingMembersOfRecentExchange(t *testing.T) {
	question := core.RPDecisionDialogue{SpeakerEntityID: "player", Text: "蓝皮诗集可以带来，但不要透露缺页的原因。", EventID: "question", WorldTime: "2026-10-01T09:00:00Z"}
	reply := core.RPDecisionDialogue{SpeakerEntityID: "npc", Text: "可以，我会保密。", EventID: "reply", WorldTime: "2026-10-01T09:01:00Z"}
	candidates := []rpDialogueCandidate{
		{dialogue: reply, sequence: 2, session: "s", turn: "old"},
		{dialogue: question, sequence: 1, session: "s", turn: "old"},
	}
	input := core.RPDecisionInput{PlayerSpeechText: "蓝皮诗集", RecentDialogue: []core.RPDecisionDialogue{reply}}
	got := selectRPRelevantDialogue(input, candidates)
	if len(got) != 1 || !got[0].RecentContext || len(got[0].Dialogue) != 2 || got[0].Dialogue[0] != question || got[0].Dialogue[1] != reply {
		t.Fatalf("recent reply suppressed its missing question/qualification: %+v", got)
	}
	input.RecentDialogue = []core.RPDecisionDialogue{question, reply}
	if got := selectRPRelevantDialogue(input, candidates); len(got) != 0 {
		t.Fatalf("fully recent exchange should not be retrieved again: %+v", got)
	}
	input.RecentDialogue = nil
	input.SpeechEventID = "question"
	if got := selectRPRelevantDialogue(input, candidates); len(got) != 0 {
		t.Fatalf("current exchange was retrieved as old history: %+v", got)
	}
	input.SpeechEventID = ""
	candidates[0].groupSize, candidates[1].groupSize = 3, 3
	if got := selectRPRelevantDialogue(input, candidates); len(got) != 0 {
		t.Fatalf("candidate-window partial exchange was presented as complete: %+v", got)
	}
}

func TestRPRelevantDialogueRepairsRecentExchangeWithoutLexicalScore(t *testing.T) {
	for _, query := range []string{"Then let's proceed.", "谢谢"} {
		t.Run(query, func(t *testing.T) {
			question := core.RPDecisionDialogue{SpeakerEntityID: "player", Text: "请带蓝皮诗集，缺页原因要保密，谢谢。", EventID: "question"}
			reply := core.RPDecisionDialogue{SpeakerEntityID: "npc", Text: "我答应。", EventID: "reply"}
			candidates := []rpDialogueCandidate{
				{dialogue: reply, sequence: 2, session: "s", turn: "split", groupSize: 2},
				{dialogue: question, sequence: 1, session: "s", turn: "split", groupSize: 2},
			}
			for i := 0; i < 5; i++ {
				candidates = append(candidates, rpDialogueCandidate{dialogue: core.RPDecisionDialogue{Text: "谢谢。", EventID: fmt.Sprintf("other-%d", i)}, sequence: int64(i + 3), session: "s", turn: fmt.Sprint(i), groupSize: 1})
			}
			input := core.RPDecisionInput{PlayerSpeechText: query, RecentDialogue: []core.RPDecisionDialogue{reply}}
			got := selectRPRelevantDialogue(input, candidates)
			if len(got) != 1 || len(got[0].Dialogue) != 2 || got[0].Dialogue[0] != question || got[0].Dialogue[1] != reply {
				t.Fatalf("missing recent sibling still depended on lexical score: %+v", got)
			}
		})
	}
}

func TestRPRelevantDialoguePrioritizesRecentRepairUnderGroupLimit(t *testing.T) {
	reply := core.RPDecisionDialogue{EventID: "reply", Text: "已答应保密。"}
	question := core.RPDecisionDialogue{EventID: "question", Text: "缺页原因不能告诉旁人。"}
	candidates := []rpDialogueCandidate{
		{dialogue: reply, sequence: 2, session: "s", turn: "split", groupSize: 2},
		{dialogue: question, sequence: 1, session: "s", turn: "split", groupSize: 2},
	}
	for i := 0; i < 10; i++ {
		text := "无关话题。"
		if i < 4 {
			text = "星际航线。"
		}
		candidates = append(candidates, rpDialogueCandidate{dialogue: core.RPDecisionDialogue{EventID: fmt.Sprint(i), Text: text}, sequence: int64(i + 3), session: "s", turn: fmt.Sprint(i), groupSize: 1})
	}
	input := core.RPDecisionInput{PlayerSpeechText: "星际航线", RecentDialogue: []core.RPDecisionDialogue{reply}}
	got := selectRPRelevantDialogue(input, candidates)
	if len(got) != rpRelevantExchangeLimit || len(got[0].Dialogue) != 2 || got[0].Dialogue[0] != question || got[0].Dialogue[1] != reply {
		t.Fatalf("lexical groups crowded out recent repair: %+v", got)
	}
}
