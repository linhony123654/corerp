package storage

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/decision"
	"corerp.local/backend/internal/endpointpolicy"
)

func semanticFixtureServer(t *testing.T, respond func(string) string, calls *atomic.Int32) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" || json.NewDecoder(r.Body).Decode(&request) != nil || len(request.Messages) != 2 {
			t.Error("invalid model transport")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var payload struct {
			Version string `json:"version"`
			Text    string `json:"text"`
		}
		if json.Unmarshal([]byte(request.Messages[1].Content), &payload) != nil {
			t.Error("invalid model context")
		}
		content := `{"action":"silence","text":"","destination_place_id":"","activity_code":"","introduce_self":false}`
		if payload.Version == "corerp.interaction.v2" {
			calls.Add(1)
			content = respond(payload.Text)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]any{"content": content}}}})
	}))
}

func semanticModelReply(kind string, steps []core.RPInteractionStep, reason string) string {
	encodedSteps := make([]map[string]any, 0, len(steps))
	for _, step := range steps {
		encodedSteps = append(encodedSteps, map[string]any{
			"kind": step.Kind, "target_place_id": step.TargetPlaceID, "target_entity_id": step.TargetEntityID,
			"wait_hours": step.WaitHours, "wait_minutes": step.WaitMinutes, "speech_text": step.SpeechText,
			"object_action": step.ObjectAction, "object_id": step.ObjectID, "anchor_id": step.AnchorID,
			"offer_id": step.OfferID, "nonverbal_action": step.NonverbalAction, "gesture_code": step.GestureCode,
		})
	}
	encoded, _ := json.Marshal(map[string]any{"kind": kind, "steps": encodedSteps, "clarification": reason})
	return string(encoded)
}

func semanticFixtureProvider(t *testing.T, server *httptest.Server) *decision.ChatProvider {
	t.Helper()
	provider, err := decision.NewChatProvider(decision.Config{Endpoint: server.URL + "/v1/chat/completions", Model: "fixture", EndpointPolicy: endpointpolicy.TestLocalhostPolicy()})
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

func TestRPInteractionModelPlanPersistsBeforeEffectAndNeverReinterprets(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "semantic.db")
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	session, read, initial := newRPWaitTestSession(t, ctx, store)
	var calls atomic.Int32
	server := semanticFixtureServer(t, func(text string) string {
		if text != "想你了。" {
			t.Errorf("unexpected player text %q", text)
		}
		return semanticModelReply("DIALOGUE", []core.RPInteractionStep{{Kind: "speech", SpeechText: "想你了。"}}, "")
	}, &calls)
	defer server.Close()
	provider := semanticFixtureProvider(t, server)
	service, err := NewRPService(store, provider, "chat_completions")
	if err != nil {
		t.Fatal(err)
	}
	request := core.RPInteractionRequest{PrincipalID: read.PrincipalID, SessionID: session.SessionID, Text: "想你了。", Mode: "AUTO", ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "semantic-recover"}
	store.afterRPInteractionStep = func(int) error { return core.NewError(core.CodeInjectedFailure, "lost response after child effect") }
	if _, err := service.RunRPInteraction(ctx, request); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatalf("expected injected child result loss, got %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("parser called %d times", calls.Load())
	}
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM rp_utterances WHERE speaker_entity_id=? AND speech_text='想你了。'`, []any{M2RPPlayerID}, 1)
	var source string
	var attempts int
	if err := store.db.QueryRowContext(ctx, `SELECT source,attempt_count FROM rp_interaction_interpretations WHERE session_id=? AND idempotency_key=? AND result='success'`, session.SessionID, request.IdempotencyKey).Scan(&source, &attempts); err != nil || source != "model" || attempts != 1 {
		t.Fatalf("persisted model receipt: %s/%d %v", source, attempts, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	service, err = NewRPService(store, core.DeterministicRPDecisionProvider{}, "deterministic")
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.ResumeRPInteraction(ctx, RPInteractionResumeRequest{PrincipalID: read.PrincipalID, SessionID: session.SessionID, IdempotencyKey: request.IdempotencyKey})
	if err != nil || result.Status != "settled" || result.InterpretationSource != "model" || result.InterpretationAttempts != 1 || len(result.Outcomes) != 1 || calls.Load() != 1 {
		t.Fatalf("resume changed accepted plan: %+v %v parser calls=%d", result, err, calls.Load())
	}
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM rp_utterances WHERE speaker_entity_id=? AND speech_text='想你了。'`, []any{M2RPPlayerID}, 1)
	if differences, err := store.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) > 0 {
		t.Fatalf("world projection drift: %v %v", differences, err)
	}
}

func TestRPInteractionModelContinueAndUnsupportedItemNeverSpeak(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "continue.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, read, initial := newRPWaitTestSession(t, ctx, store)
	var calls atomic.Int32
	server := semanticFixtureServer(t, func(text string) string {
		if text == "继续剧情" {
			return semanticModelReply("CONTINUE", []core.RPInteractionStep{{Kind: "wait", WaitMinutes: 15}}, "")
		}
		return semanticModelReply("CLARIFICATION", nil, "unsupported_item")
	}, &calls)
	defer server.Close()
	service, err := NewRPService(store, semanticFixtureProvider(t, server), "chat_completions")
	if err != nil {
		t.Fatal(err)
	}
	request := core.RPInteractionRequest{PrincipalID: read.PrincipalID, SessionID: session.SessionID, Text: "继续剧情", Mode: "AUTO", ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "semantic-continue"}
	result, err := service.RunRPInteraction(ctx, request)
	if err != nil || result.Status != "settled" || result.PlanKind != "CONTINUE" || len(result.Outcomes) != 1 || result.Outcomes[0].Kind != "wait" || result.InterpretationSource != "model" || result.InterpretationAttempts != 1 {
		t.Fatalf("continue did not execute a pinned wait: %+v %v", result, err)
	}
	var target, intent string
	if err := store.db.QueryRowContext(ctx, `SELECT i.target_world_time,json_extract(e.payload,'$.opportunity_intent') FROM rp_wait_intents i JOIN event_batches b ON b.command_id=i.command_id JOIN events e ON e.batch_id=b.batch_id WHERE i.session_id=? AND i.idempotency_key LIKE 'rpint_%' AND e.event_type='RPWaitCompleted'`, session.SessionID).Scan(&target, &intent); err != nil {
		t.Fatal(err)
	}
	start, err := time.Parse(time.RFC3339, initial.WorldTime)
	if err != nil || target != start.Add(15*time.Minute).UTC().Format(time.RFC3339) || intent != "social" {
		t.Fatalf("wrong wait identity %s %s %v", target, intent, err)
	}
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM rp_utterances WHERE speaker_entity_id=?`, []any{M2RPPlayerID}, 0)
	current, err := store.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	item := core.RPInteractionRequest{PrincipalID: read.PrincipalID, SessionID: session.SessionID, Text: "我把杯子推给她，说：\"喝一点吧。\"", Mode: "AUTO", ExpectedCursor: current.ObservationCursor, IdempotencyKey: "unsupported-item"}
	clarify, err := service.RunRPInteraction(ctx, item)
	if err != nil || clarify.Status != "clarification" || clarify.InterpretationSource != "model" || !strings.Contains(clarify.Clarification, "物品") || len(clarify.Outcomes) != 0 || calls.Load() != 2 {
		t.Fatalf("unsupported item became a fact or speech: %+v %v", clarify, err)
	}
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM rp_utterances WHERE speaker_entity_id=?`, []any{M2RPPlayerID}, 0)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM stock_movements WHERE event_id IN (SELECT event_id FROM events WHERE event_type LIKE 'RP%')`, nil, 0)
}
