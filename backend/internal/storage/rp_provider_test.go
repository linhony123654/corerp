package storage

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/decision"
)

func TestRPLLMProviderUsesFilteredContextCommitsAndRecoversWithoutModel(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "llm.db")
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	session, read, initial := newRPWaitTestSession(t, ctx, store)
	var calls atomic.Int32
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		raw, _ := io.ReadAll(r.Body)
		for _, forbidden := range []string{M2AgentAdaID, M2AgentBoID, "asset_account_id", "principal_id", "audit_records", "private_memory"} {
			if strings.Contains(string(raw), forbidden) {
				t.Errorf("outbound context contains forbidden %s", forbidden)
			}
		}
		if !strings.Contains(string(raw), "own_asset_minor") || !strings.Contains(string(raw), "activity_code") {
			t.Error("missing own life context")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": `{"action":"respond","text":"这句话来自配置的模型接口。","destination_place_id":""}`}}}})
	}))
	defer model.Close()
	provider, err := decision.NewChatProvider(decision.Config{Endpoint: model.URL, Model: "test-model"})
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewRPService(store, provider, "chat_completions")
	if err != nil {
		t.Fatal(err)
	}
	observed, err := service.ObserveRPSession(ctx, read)
	if err != nil || observed.DecisionMode != "chat_completions" {
		t.Fatalf("mode %+v %v", observed, err)
	}
	store.afterRPTurnStage = func(stage string) error {
		if stage == "npc_effect_committed" {
			return errors.New("simulated stop")
		}
		return nil
	}
	request := core.RPSpeechRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID, ExpectedCursor: initial.ObservationCursor, Text: "你好", IdempotencyKey: "configured-provider"}
	if _, err := service.PlayRPTurn(ctx, request); err == nil {
		t.Fatal("crash hook did not run")
	}
	if calls.Load() != 1 {
		t.Fatal("wrong model call count")
	}
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM rp_utterances`, nil, 2)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM agent_knowledge WHERE json_extract(claim_payload,'$.claim_type')='speaker_said'`, nil, 2)
	store.Close()
	store, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	service, _ = NewRPService(store, provider, "chat_completions")
	result, err := service.PlayResumeRPTurn(ctx, RPTurnResumeRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID, IdempotencyKey: request.IdempotencyKey})
	if err != nil || result.Status != "settled" || len(result.NarrativeLines) != 2 || !strings.Contains(result.NarrativeLines[1], "配置的模型接口") {
		t.Fatalf("resume %+v %v", result, err)
	}
	replay, err := service.PlayRPTurn(ctx, request)
	if err != nil || !replay.Replayed || calls.Load() != 1 {
		t.Fatalf("committed effects re-called model: %+v %v calls=%d", replay, err, calls.Load())
	}
	differences, err := store.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID)
	if err != nil || len(differences) != 0 {
		t.Fatalf("replay mismatch %v %v", differences, err)
	}
}

func TestRPLLMFailureAndIllegalOutputSettleOnlyAuditedSilence(t *testing.T) {
	for _, mode := range []string{"unavailable", "illegal", "schema", "timeout"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			store, err := Open(ctx, filepath.Join(t.TempDir(), "fallback.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			session, _, initial := newRPWaitTestSession(t, ctx, store)
			release := make(chan struct{})
			model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch mode {
				case "unavailable":
					w.WriteHeader(503)
					_, _ = w.Write([]byte("secret-upstream-details"))
				case "timeout":
					select {
					case <-r.Context().Done():
					case <-release:
					}
				default:
					proposal := `{"action":"give_money","text":"","destination_place_id":""}`
					if mode == "schema" {
						proposal = `{"action":"silence","untrusted":"secret-upstream-details"}`
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": proposal}}}})
				}
			}))
			defer func() { close(release); model.Close() }()
			provider, err := decision.NewChatProvider(decision.Config{Endpoint: model.URL, Model: "test", Timeout: 100 * time.Millisecond, Attempts: 1})
			if err != nil {
				t.Fatal(err)
			}
			service, _ := NewRPService(store, provider, "chat_completions")
			request := core.RPSpeechRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID, ExpectedCursor: initial.ObservationCursor, Text: "你好", IdempotencyKey: "failure"}
			result, err := service.PlayRPTurn(ctx, request)
			if err != nil || result.Status != "settled" || !strings.Contains(result.NarrativeLines[1], "保持沉默") {
				t.Fatalf("failure left partial turn %+v %v", result, err)
			}
			assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM rp_utterances`, nil, 1)
			assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM rp_npc_decisions WHERE action='silence'`, nil, 1)
			assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM audit_records WHERE json_extract(payload,'$.status')='provider_fallback'`, nil, 1)
			assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM audit_records WHERE payload LIKE '%secret-upstream-details%'`, nil, 0)
			assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM events WHERE event_sequence>? AND event_type NOT IN ('RPSpeechAccepted','RPNPCDecisionRecorded')`, []any{initial.ObservationCursor}, 0)
		})
	}
}
