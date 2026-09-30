package storage

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPDecisionDialogueUsesOnlyAcceptedPersonallyHeardWordsAfterRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "npc-dialogue.db")
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	session, player, initial := newRPWaitTestSession(t, ctx, store)
	first := core.RPSpeechRequest{PrincipalID: player.PrincipalID, SessionID: session.SessionID, Text: "你还记得我刚才说的话吗？", ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "dialogue-first"}
	firstTurn, err := store.RunRPTurn(ctx, first, rpDecisionProviderFunc(func(context.Context, core.RPDecisionInput) (core.RPDecisionProposal, error) {
		return core.RPDecisionProposal{Action: "respond", Text: "我记得你刚才的问题。"}, nil
	}))
	if err != nil || firstTurn.Status != "settled" || len(firstTurn.NPCEventIDs) != 1 {
		t.Fatal(firstTurn, err)
	}
	// The old autobiographical index is not authoritative decision memory.
	if _, err := store.db.ExecContext(ctx, `DELETE FROM rp_own_actions WHERE event_id=?`, firstTurn.NPCEventIDs[0]); err != nil {
		t.Fatal(err)
	}
	grantRPControlForTest(t, ctx, store, M2AgentAdaID)
	ada, err := store.OpenRPSession(ctx, rpTestOpenRequest())
	if err != nil {
		t.Fatal(err)
	}
	adaView, err := store.ObserveRPSession(ctx, core.RPSessionReadRequest{PrincipalID: rpTestPrincipal, SessionID: ada.SessionID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SpeakRP(ctx, core.RPSpeechRequest{PrincipalID: rpTestPrincipal, SessionID: ada.SessionID, Text: "不在场的私下话语。", ExpectedCursor: adaView.ObservationCursor, IdempotencyKey: "dialogue-offsite"}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	view, err := store.ObserveRPSession(ctx, player)
	if err != nil {
		t.Fatal(err)
	}
	seen := false
	second := core.RPSpeechRequest{PrincipalID: player.PrincipalID, SessionID: session.SessionID, Text: "那你怎么回答？", ExpectedCursor: view.ObservationCursor, IdempotencyKey: "dialogue-second"}
	secondTurn, err := store.RunRPTurn(ctx, second, rpDecisionProviderFunc(func(_ context.Context, input core.RPDecisionInput) (core.RPDecisionProposal, error) {
		seen = true
		foundOwn := false
		for _, action := range input.OwnActions {
			text, complete := core.ResolveRPDecisionSpeech(input, action.EventID, input.NPCEntityID)
			if action.EventID == firstTurn.NPCEventIDs[0] && action.Action == "speech" && complete && text == "我记得你刚才的问题。" && action.PlaceID != "" {
				foundOwn = true
			}
		}
		if !foundOwn {
			t.Errorf("accepted NPC speech disappeared from sourced own memory: %+v", input.OwnActions)
		}
		if len(input.RecentDialogue) != 3 {
			t.Errorf("wanted player, NPC, player accepted utterances, got %+v", input.RecentDialogue)
		} else {
			for i, want := range []struct{ actor, text string }{{M2RPPlayerID, first.Text}, {M2RPNPCID, "我记得你刚才的问题。"}, {M2RPPlayerID, second.Text}} {
				got := input.RecentDialogue[i]
				text, complete := core.ResolveRPDecisionSpeech(input, got.EventID, got.SpeakerEntityID)
				if got.SpeakerEntityID != want.actor || !complete || text != want.text || got.EventID == "" || got.WorldTime == "" {
					t.Errorf("dialogue %d lacks accepted provenance or order: %+v", i, got)
				}
			}
		}
		return core.RPDecisionProposal{Action: "respond", Text: "你问我记不记得，我记得。"}, nil
	}))
	if err != nil || secondTurn.Status != "settled" || !seen {
		t.Fatal(secondTurn, seen, err)
	}
}

func TestRPDecisionKeepsEarlierHeardPlayerOfferBeyondDialogueWindow(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "npc-older-offer.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, player, _ := newRPWaitTestSession(t, ctx, store)
	provider := rpDecisionProviderFunc(func(context.Context, core.RPDecisionInput) (core.RPDecisionProposal, error) {
		return core.RPDecisionProposal{Action: "silence"}, nil
	})
	for i := 0; i < 18; i++ {
		view, err := store.ObserveRPSession(ctx, player)
		if err != nil {
			t.Fatal(err)
		}
		spoken := "今天聊点别的。"
		if i == 0 {
			spoken = "如果你需要，我愿意帮忙。"
		}
		request := core.RPSpeechRequest{PrincipalID: player.PrincipalID, SessionID: session.SessionID, Text: spoken, ExpectedCursor: view.ObservationCursor, IdempotencyKey: "older-offer-" + string(rune('a'+i))}
		if _, err := store.RunRPTurn(ctx, request, provider); err != nil {
			t.Fatal(err)
		}
	}
	view, err := store.ObserveRPSession(ctx, player)
	if err != nil {
		t.Fatal(err)
	}
	speech, err := store.SpeakRP(ctx, core.RPSpeechRequest{PrincipalID: player.PrincipalID, SessionID: session.SessionID, Text: "你还记得我说过想帮忙吗？", ExpectedCursor: view.ObservationCursor, IdempotencyKey: "older-offer-question"})
	if err != nil {
		t.Fatal(err)
	}
	input, err := store.BuildRPDecisionInput(ctx, core.RPDecisionRequest{PrincipalID: player.PrincipalID, SessionID: session.SessionID, TurnID: speech.TurnID, NPCEntityID: M2RPNPCID})
	if err != nil {
		t.Fatal(err)
	}
	for _, utterance := range input.RecentDialogue {
		if utterance.Text == "如果你需要，我愿意帮忙。" {
			t.Fatal("test did not push the earlier offer outside the recent dialogue window")
		}
	}
	if len(input.HeardPlayerHistory) != 18 {
		t.Fatalf("earlier heard history candidates disappeared: %+v", input.HeardPlayerHistory)
	}
	text, complete := core.ResolveRPDecisionSpeech(input, input.HeardPlayerHistory[0].EventID, input.InterlocutorEntityID)
	if !complete || text != "如果你需要，我愿意帮忙。" || input.HeardPlayerHistory[0].EventID == "" || input.HeardPlayerHistory[0].Truncated {
		t.Fatalf("earlier heard offer was lost or lost provenance: %+v", input.HeardPlayerHistory)
	}
}

func TestRPNPCProposalCannotSeeUnknownInterlocutorCanonicalIdentity(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "npc-unknown-identity.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, player, initial := newRPWaitTestSession(t, ctx, store)
	if _, err := store.MoveRP(ctx, core.RPMoveRequest{PrincipalID: player.PrincipalID, SessionID: session.SessionID, FromPlaceID: initial.PlaceID, ToPlaceID: "place_m2_home_ada", ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "npc-unknown-visit"}); err != nil {
		t.Fatal(err)
	}
	view, err := store.ObserveRPSession(ctx, player)
	if err != nil {
		t.Fatal(err)
	}
	speech, err := store.SpeakRP(ctx, core.RPSpeechRequest{PrincipalID: player.PrincipalID, SessionID: session.SessionID, Text: "你是谁呀？", ExpectedCursor: view.ObservationCursor, IdempotencyKey: "npc-unknown-question"})
	if err != nil {
		t.Fatal(err)
	}
	request := core.RPDecisionRequest{PrincipalID: player.PrincipalID, SessionID: session.SessionID, TurnID: speech.TurnID, NPCEntityID: M2AgentAdaID}
	var rawName string
	if err := store.db.QueryRowContext(ctx, `SELECT display_name FROM materialized_entities WHERE entity_id=?`, M2RPPlayerID).Scan(&rawName); err != nil {
		t.Fatal(err)
	}
	var observed core.RPDecisionInput
	decision, err := store.DecideRP(ctx, request, rpDecisionProviderFunc(func(_ context.Context, input core.RPDecisionInput) (core.RPDecisionProposal, error) {
		observed = input
		return core.RPDecisionProposal{Action: "respond", Text: "我们还不熟。"}, nil
	}))
	if err != nil || decision.Status != "validated" {
		t.Fatal(decision, err)
	}
	alias, err := rpAnonymousEntityIDForTest(ctx, store, M2DemoInstanceID, M2DemoBranchID, M2AgentAdaID, M2RPPlayerID)
	if err != nil {
		t.Fatal(err)
	}
	if observed.InterlocutorEntityID != alias || len(observed.VisibleEntities) != 1 || observed.VisibleEntities[0].EntityID != alias || observed.VisibleEntities[0].DisplayName != "陌生人" || len(observed.RecentDialogue) != 1 || observed.RecentDialogue[0].SpeakerEntityID != alias {
		t.Fatalf("NPC model received canonical identity or lost heard speech: %+v", observed)
	}
	encoded, err := json.Marshal(observed)
	if err != nil || strings.Contains(string(encoded), M2RPPlayerID) || strings.Contains(string(encoded), rawName) {
		t.Fatalf("NPC provider context leaked canonical stranger: %s, %v", encoded, err)
	}
}
