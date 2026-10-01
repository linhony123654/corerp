package storage

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sort"
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
	const offsiteText = "不在场的私下话语。"
	offsite, err := store.SpeakRP(ctx, core.RPSpeechRequest{PrincipalID: rpTestPrincipal, SessionID: ada.SessionID, Text: offsiteText, ExpectedCursor: adaView.ObservationCursor, IdempotencyKey: "dialogue-offsite"})
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
		// V3 retains the older question/reply as one peer unit, rather than
		// duplicating its members in RecentDialogue. Verify the unified sources
		// against the actual Events; field placement must not imply word loss.
		wantDialogue := []struct{ actor, text, eventID string }{
			{M2RPPlayerID, first.Text, firstTurn.PlayerEventID},
			{M2RPNPCID, "我记得你刚才的问题。", firstTurn.NPCEventIDs[0]},
			{M2RPPlayerID, second.Text, input.SpeechEventID},
		}
		sequences, times := map[string]int64{}, map[string]string{}
		for _, want := range wantDialogue {
			var sequence int64
			var worldTime, actor string
			if err := store.db.QueryRowContext(ctx, `SELECT event_sequence,world_time,actor_id FROM events WHERE event_id=? AND instance_id=? AND branch_id=? AND event_type='RPSpeechAccepted'`, want.eventID, input.InstanceID, input.BranchID).Scan(&sequence, &worldTime, &actor); err != nil {
				t.Errorf("accepted dialogue source missing: %v", err)
			} else if actor != want.actor || worldTime == "" || sequence > input.HeadSequence {
				t.Errorf("dialogue source crossed actor/time/head: %s", want.eventID)
			}
			sequences[want.eventID], times[want.eventID] = sequence, worldTime
		}
		var unified []core.RPDecisionDialogue
		seenEvents := map[string]bool{}
		collect := func(dialogue []core.RPDecisionDialogue) {
			var previous int64
			for _, d := range dialogue {
				sequence, known := sequences[d.EventID]
				if !known || seenEvents[d.EventID] || sequence <= previous {
					t.Errorf("dialogue has an extra/duplicate/out-of-order source: %+v", d)
				}
				previous = sequence
				seenEvents[d.EventID] = true
				unified = append(unified, d)
			}
		}
		collect(input.RecentDialogue)
		peerPairs := 0
		for _, exchange := range input.RelevantDialogue {
			collect(exchange.Dialogue)
			if exchange.PeerContext && len(exchange.Dialogue) == 2 && exchange.Dialogue[0].EventID == firstTurn.PlayerEventID && exchange.Dialogue[1].EventID == firstTurn.NPCEventIDs[0] {
				peerPairs++
			}
		}
		if peerPairs != 1 {
			t.Errorf("earlier heard question/reply no longer form one complete peer unit: %+v", input.RelevantDialogue)
		}
		sort.Slice(unified, func(i, j int) bool { return sequences[unified[i].EventID] < sequences[unified[j].EventID] })
		if len(unified) != len(wantDialogue) {
			t.Errorf("wanted three unique accepted utterances across recent/peer dialogue, got %+v", unified)
		} else {
			for i, want := range wantDialogue {
				got := unified[i]
				text, complete := core.ResolveRPDecisionSpeech(input, got.EventID, got.SpeakerEntityID)
				if got.EventID != want.eventID || got.SpeakerEntityID != want.actor || !complete || text != want.text || got.WorldTime != times[want.eventID] || got.WorldTime == "" {
					t.Errorf("dialogue %d lacks exact accepted words/provenance/time/order: %+v", i, got)
				}
			}
		}
		encoded, encodeErr := json.Marshal(input)
		if encodeErr != nil || strings.Contains(string(encoded), offsiteText) || strings.Contains(string(encoded), offsite.EventID) || core.RPDecisionEvidenceEventIDs(input)[offsite.EventID] {
			t.Errorf("offsite speech leaked into provider words or grounding: %v", encodeErr)
		}
		if _, complete := core.ResolveRPDecisionSpeech(input, offsite.EventID, M2AgentAdaID); complete {
			t.Error("unheard offsite utterance resolved as accepted dialogue")
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
