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
	if len(input.RelevantDialogue) != 1 || len(input.RelevantDialogue[0].Dialogue) != 2 {
		t.Fatalf("missing old exchange: %+v", input.RelevantDialogue)
	}
	dialogue := input.RelevantDialogue[0].Dialogue
	if dialogue[0].Text != firstPlayer || dialogue[1].Text != firstNPC || dialogue[1].EventID != firstEvent || dialogue[0].EventID == "" || dialogue[1].WorldTime == "" {
		t.Fatalf("lost verbatim words/order/provenance: %+v", dialogue)
	}
	if !core.RPDecisionEvidenceEventIDs(input)[firstEvent] {
		t.Fatal("retrieved source missing from allowed grounding")
	}
	providerView, err := store.rpDecisionProviderView(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(providerView)
	if strings.Contains(string(encoded), "无人听见") {
		t.Fatal("unheard speech leaked into decision packet")
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
