package storage

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPLifeUsesOwnFactsAndRemembersRepeatedContactAcrossReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "life.db")
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	session, read, initial := newRPWaitTestSession(t, ctx, store)
	var first core.RPDisposition
	cursor := initial.ObservationCursor
	for i := 0; i < 3; i++ {
		request := core.RPSpeechRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID, ExpectedCursor: cursor, Text: "你好", IdempotencyKey: string(rune('a' + i))}
		result, err := store.PlayRPTurn(ctx, request)
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 && !strings.Contains(result.NarrativeLines[1], "你好。") {
			t.Fatalf("initial choice %+v", result)
		}
		if i == 2 && !strings.Contains(result.NarrativeLines[1], "又见面") {
			t.Fatalf("repeated contact did not affect choice %+v", result)
		}
		cursor = result.SettledSequence
	}
	speech, err := store.SpeakRP(ctx, core.RPSpeechRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID, ExpectedCursor: cursor, Text: "我有一百万", IdempotencyKey: "inspect-life"})
	if err != nil {
		t.Fatal(err)
	}
	decisionRequest := core.RPDecisionRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID, TurnID: speech.TurnID, NPCEntityID: M2RPNPCID}
	input, err := store.BuildRPDecisionInput(ctx, decisionRequest)
	if err != nil {
		t.Fatal(err)
	}
	if input.Life == nil || input.Life.LiabilityMinor != 60 || input.Life.ReceivableMinor != 80 || input.OwnAssetMinor != 400 || len(input.Life.EconomicSourceEventIDs) == 0 || len(input.Life.Employment) != 0 {
		t.Fatalf("life does not match own authoritative facts %+v", input.Life)
	}
	first = input.Life.Disposition
	if len(input.Life.SalientMemories) == 0 || input.Life.SalientMemories[0].Kind != "speaker_said" || input.Life.SalientMemories[0].Text != "我有一百万" {
		t.Fatalf("attributed memory missing %+v", input.Life.SalientMemories)
	}
	raw, _ := json.Marshal(input.Life)
	for _, forbidden := range []string{M2AgentAdaID, M2AgentBoID, "account_id", "principal_id"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("private life context leaked %s", forbidden)
		}
	}
	observed, err := store.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	public, _ := json.Marshal(observed)
	if strings.Contains(string(public), "disposition") || strings.Contains(string(public), "liability_minor") {
		t.Fatal("private life leaked to player")
	}
	store.Close()
	store, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	restored, err := store.BuildRPDecisionInput(ctx, decisionRequest)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, restored.Life.Disposition) || !reflect.DeepEqual(input.Life, restored.Life) {
		t.Fatal("life context changed on reopen")
	}
	if err := store.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	replayed, err := store.BuildRPDecisionInput(ctx, decisionRequest)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(restored.Life, replayed.Life) {
		t.Fatal("life context changed after projection rebuild")
	}
}
