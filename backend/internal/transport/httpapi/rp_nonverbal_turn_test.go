package httpapi

import (
	"context"
	"net/http"
	"path/filepath"
	"reflect"
	"testing"

	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/storage"
)

func TestRPNonverbalHTTPTargetedNodCommitsReactionAndNarrative(t *testing.T) {
	ctx := context.Background()
	store, handler := openHTTPTestServer(t, ctx, filepath.Join(t.TempDir(), "nod-turn-http.db"))
	defer store.Close()
	if _, err := store.PrepareRPTravel(ctx); err != nil {
		t.Fatal(err)
	}
	response := performJSON(t, handler, "/api/v1/rp/sessions/open", rpPlayerToken, core.RPSessionOpenRequest{InstanceID: storage.M2DemoInstanceID, BranchID: storage.M2DemoBranchID, EntityID: storage.M2RPPlayerID, POV: "second_person", IdempotencyKey: "http-nod-turn"})
	assertStatus(t, response, http.StatusOK)
	session := decodeData[storage.RPSession](t, response)
	response = performJSON(t, handler, "/api/v1/rp/observe", rpPlayerToken, core.RPSessionReadRequest{SessionID: session.SessionID})
	assertStatus(t, response, http.StatusOK)
	before := decodeData[storage.RPObservation](t, response)
	request := core.RPNonverbalRequest{SessionID: session.SessionID, Action: "nod", TargetEntityID: storage.M2RPNPCID, ExpectedCursor: before.ObservationCursor, IdempotencyKey: "http-directed-nod"}
	response = performJSON(t, handler, "/api/v1/rp/actions/nonverbal", rpPlayerToken, request)
	assertStatus(t, response, http.StatusOK)
	result := decodeData[storage.RPNonverbalResult](t, response)
	if result.Replayed || result.Turn == nil || result.Turn.Status != "settled" || result.Turn.PlayerEventID != result.EventID || result.Turn.PlayerTurnID != result.EventID || result.Turn.SettledSequence <= result.EventSequence || result.WorldTime != before.WorldTime || len(result.Turn.NPCEventIDs) != 1 || result.Turn.CompositionVersion != core.RPFactCompositionVersionV2 {
		t.Fatalf("HTTP action did not finish a real sourced reaction turn: %+v", result)
	}
	if len(result.Turn.ProviderCalls) != 2 || result.Turn.ProviderCalls[0].Result != "success" || result.Turn.ProviderCalls[0].ProviderKind != "deterministic" {
		t.Fatalf("missing reaction receipt: %+v", result.Turn.ProviderCalls)
	}
	response = performJSON(t, handler, "/api/v1/rp/observe", rpPlayerToken, core.RPSessionReadRequest{SessionID: session.SessionID})
	assertStatus(t, response, http.StatusOK)
	shown := decodeData[storage.RPObservation](t, response)
	if shown.WorldTime != before.WorldTime || shown.ObservationCursor != result.Turn.SettledSequence || len(shown.RecentTurns) != 1 || shown.RecentTurns[0].TurnRunID != result.Turn.TurnRunID || !reflect.DeepEqual(shown.RecentTurns[0].NarrativeLines, result.Turn.NarrativeLines) {
		t.Fatalf("HTTP history duplicated the action or lost its reaction: %+v", shown.RecentTurns)
	}
	response = performJSON(t, handler, "/api/v1/rp/requests/retire", rpPlayerToken, storage.RPRequestRetireRequest{Operation: "nonverbal", SessionID: session.SessionID, IdempotencyKey: request.IdempotencyKey})
	assertStatus(t, response, http.StatusOK)
	if status := decodeData[storage.RPRequestOutcome](t, response); status.Status != "completed" {
		t.Fatal("completed action reaction was not reported", status)
	}
	response = performJSON(t, handler, "/api/v1/rp/actions/nonverbal", rpPlayerToken, request)
	assertStatus(t, response, http.StatusOK)
	replayed := decodeData[storage.RPNonverbalResult](t, response)
	if !replayed.Replayed || replayed.EventID != result.EventID || replayed.Turn == nil || !reflect.DeepEqual(replayed.Turn.NarrativeLines, result.Turn.NarrativeLines) || !reflect.DeepEqual(replayed.Turn.ProviderCalls, result.Turn.ProviderCalls) {
		t.Fatal("HTTP exact replay re-ran the reaction", replayed)
	}
	if differences, err := store.CompareProjections(ctx, storage.M2DemoInstanceID, storage.M2DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatalf("action reaction drifted event-derived world projections: %+v %v", differences, err)
	}
}
