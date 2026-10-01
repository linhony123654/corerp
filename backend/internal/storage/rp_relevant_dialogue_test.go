package storage

import (
	"context"
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
	if err != nil || len(retrieved) != 1 || !retrieved[0].RecentContext || len(retrieved[0].Dialogue) != 2 || retrieved[0].Dialogue[0] != dialogue[0] || retrieved[0].Dialogue[1] != dialogue[1] {
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
