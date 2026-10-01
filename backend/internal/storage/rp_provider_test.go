package storage

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/decision"
	"corerp.local/backend/internal/endpointpolicy"
)

func TestRPLLMProviderUsesFilteredContextCommitsAndRecoversWithoutModel(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "llm.db")
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	session, read, initial := newRPAuthoredModelTestSession(t, ctx, store)
	var calls atomic.Int32
	const privateIntent = "只在私有草案中考虑下一步如何安抚"
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
		var request struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.Unmarshal(raw, &request); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var packet struct {
			Version   string               `json:"version"`
			Character core.RPDecisionInput `json:"character"`
		}
		for _, message := range request.Messages {
			if message.Role == "user" {
				if err := json.Unmarshal([]byte(message.Content), &packet); err != nil {
					t.Error(err)
				}
			}
		}
		if packet.Version != "corerp.decision.v3" || packet.Character.SpeechEventID == "" {
			t.Error("configured adapter did not receive the versioned sourced context")
		}
		proposal, err := json.Marshal(map[string]any{
			"private":    core.RPDecisionPrivate{Intent: privateIntent, Emotion: "平静", RelationshipStance: "谨慎", BasisEventIDs: []string{packet.Character.SpeechEventID}},
			"observable": map[string]any{"action": "respond", "text": "这句话来自配置的模型接口。", "introduce_self": false, "expression_code": "beckon"},
		})
		if err != nil {
			t.Error(err)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": string(proposal)}}}})
	}))
	defer model.Close()
	provider, err := decision.NewChatProvider(decision.Config{Endpoint: model.URL + "/v1/chat/completions", Model: "test-model", EndpointPolicy: endpointpolicy.TestLocalhostPolicy()})
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
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM events WHERE event_type='RPNonverbalAction' AND json_extract(payload,'$.gesture_code')='beckon' AND json_extract(payload,'$.target_entity_id')=?`, []any{session.ControlledEntityID}, 1)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM events WHERE payload LIKE ?`, []any{"%" + privateIntent + "%"}, 0)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM outbox WHERE payload LIKE ?`, []any{"%" + privateIntent + "%"}, 0)
	// An unfinished turn has no settled narration to contain the gesture yet.
	// Its witnessed committed fact must remain available during recovery.
	pending, err := service.ObserveRPSession(ctx, read)
	if err != nil || len(pending.RecentTurns) != 1 || !strings.Contains(strings.Join(pending.RecentTurns[0].NarrativeLines, "\n"), "招手") {
		t.Fatalf("pending turn hid its committed gesture: %+v %v", pending.RecentTurns, err)
	}
	store.Close()
	store, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	service, _ = NewRPService(store, provider, "chat_completions")
	result, err := service.PlayResumeRPTurn(ctx, RPTurnResumeRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID, IdempotencyKey: request.IdempotencyKey})
	if err != nil || result.Status != "settled" || result.CompositionVersion != core.RPFactCompositionVersionV2 || len(result.NPCEventIDs) != 1 {
		t.Fatalf("resume %+v %v", result, err)
	}
	var expressionID string
	if err := store.db.QueryRowContext(ctx, `SELECT event_id FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPNonverbalAction' AND causation_event_id=?`, session.InstanceID, session.BranchID, result.NPCEventIDs[0]).Scan(&expressionID); err != nil {
		t.Fatal("reply lost its committed expression child", err)
	}
	expectedGroups := [][]string{{result.PlayerEventID}, {result.NPCEventIDs[0], expressionID}}
	if len(result.NarrativeLines) != len(expectedGroups) || !reflect.DeepEqual(result.FactGroups, expectedGroups) || strings.Count(strings.Join(result.NarrativeLines, "\n"), "「这句话来自配置的模型接口。」") != 1 || strings.Count(strings.Join(result.NarrativeLines, "\n"), "「你好」") != 1 {
		t.Fatal("recovery lost exact dialogue or speech/expression source grouping", result)
	}

	public, err := json.Marshal(result)
	if err != nil || strings.Contains(string(public), privateIntent) {
		t.Fatal("new decision wire leaked a private sketch through recovered public output", err)
	}
	if len(result.ProviderCalls) != 2 || !result.ProviderCalls[0].Attempted || result.ProviderCalls[0].ProviderKind != "chat_completions" || result.ProviderCalls[0].ModelID != "test-model" || result.ProviderCalls[0].Result != "success" || result.ProviderCalls[0].AttemptCount != 1 || result.ProviderCalls[1].RenderSource != "template" || result.ProviderCalls[1].Attempted || result.ProviderCalls[1].AttemptCount != 0 {
		t.Fatalf("recovered model success receipt: %+v", result.ProviderCalls)
	}
	observation, err := service.ObserveRPSession(ctx, read)
	if err != nil || len(observation.RecentTurns) != 1 || len(observation.RecentTurns[0].ProviderCalls) != 2 || observation.RecentTurns[0].ProviderCalls[0].Result != "success" {
		t.Fatalf("scoped history receipt %+v %v", observation.RecentTurns, err)
	}
	recent := observation.RecentTurns[0]
	// Both closed beckon lexical choices (招手 / 招了招手) retain this action.
	if strings.Count(strings.Join(recent.NarrativeLines, "\n"), "招手") != 1 || !reflect.DeepEqual(recent.NarrativeLines, result.NarrativeLines) || !reflect.DeepEqual(recent.FactGroups, expectedGroups) || !reflect.DeepEqual(recent.EventIDs, []string{result.PlayerEventID, result.NPCEventIDs[0], expressionID}) {
		t.Fatal("settled turn lost or duplicated its witnessed gesture or receipt", recent)
	}
	second, err := store.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: M2RPPlayerPrincipal, InstanceID: session.InstanceID, BranchID: session.BranchID, EntityID: session.ControlledEntityID, POV: "second_person", IdempotencyKey: "wire-v3-second-client"})
	if err != nil {
		t.Fatal(err)
	}
	shared, err := store.ObserveRPSession(ctx, core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: second.SessionID})
	if err != nil || len(shared.RecentTurns) != 1 || shared.RecentTurns[0].CanRegenerate {
		t.Fatalf("same observer's second session duplicated the gesture or gained regeneration authority: %+v %v", shared.RecentTurns, err)
	}
	replay, err := service.PlayRPTurn(ctx, request)
	if err != nil || !replay.Replayed || calls.Load() != 1 {
		t.Fatalf("committed effects re-called model: %+v %v calls=%d", replay, err, calls.Load())
	}
	differences, err := store.CompareProjections(ctx, session.InstanceID, session.BranchID)
	if err != nil || len(differences) != 0 {
		t.Fatalf("replay mismatch %v %v", differences, err)
	}
	if err := store.RebuildProjections(ctx, session.InstanceID, session.BranchID); err != nil {
		t.Fatal(err)
	}
	rebuilt, err := service.ObserveRPSession(ctx, read)
	if err != nil || len(rebuilt.RecentTurns) != 1 {
		t.Fatalf("history grouping changed after projection rebuild: %+v %v", rebuilt.RecentTurns, err)
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
			session, _, initial := newRPAuthoredModelTestSession(t, ctx, store)
			var calls atomic.Int32
			release := make(chan struct{})
			model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
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
			provider, err := decision.NewChatProvider(decision.Config{Endpoint: model.URL + "/v1/chat/completions", Model: "test", Timeout: 100 * time.Millisecond, Attempts: 1, EndpointPolicy: endpointpolicy.TestLocalhostPolicy()})
			if err != nil {
				t.Fatal(err)
			}
			service, _ := NewRPService(store, provider, "chat_completions")
			request := core.RPSpeechRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID, ExpectedCursor: initial.ObservationCursor, Text: "你好", IdempotencyKey: "failure"}
			result, err := service.PlayRPTurn(ctx, request)
			if err != nil || result.Status != "settled" || result.CompositionVersion != core.RPFactCompositionVersionV2 || len(result.NPCEventIDs) != 1 || calls.Load() != 1 {
				t.Fatalf("failure left partial turn %+v %v calls=%d", result, err, calls.Load())
			}
			expectedGroups := [][]string{{result.PlayerEventID}, {result.NPCEventIDs[0]}}
			if len(result.NarrativeLines) != len(expectedGroups) || !reflect.DeepEqual(result.FactGroups, expectedGroups) || !strings.Contains(result.NarrativeLines[1], "没有作答") {
				t.Fatal("fallback lost its sourced public silence", result)
			}
			saved, err := store.ReadRPNarrative(ctx, RPNarrativeReadRequest{PrincipalID: request.PrincipalID, SessionID: request.SessionID, TurnRunID: result.TurnRunID})
			if err != nil || saved.View.Artifact == nil || !reflect.DeepEqual(saved.View.Lines, result.NarrativeLines) || !reflect.DeepEqual(saved.View.EventIDs, []string{result.PlayerEventID, result.NPCEventIDs[0]}) {
				t.Fatal("fallback lost complete canonical receipt", saved, err)
			}
			facts := saved.View.Artifact.Input.Facts
			if len(facts) != 2 || facts[1].EventID != result.NPCEventIDs[0] || facts[1].Action != "silence" || facts[1].Text != "" || facts[1].ExpressionCode != "" || facts[1].CompanionEventID != "" {
				t.Fatal("failure fabricated speech or expression instead of committed silence", facts)
			}

			if len(result.ProviderCalls) != 2 || !result.ProviderCalls[0].Attempted || result.ProviderCalls[0].ProviderKind != "chat_completions" || result.ProviderCalls[0].Result == "success" || result.ProviderCalls[0].AttemptCount != 1 || result.ProviderCalls[0].FallbackKind != "silence" || result.ProviderCalls[1].RenderSource != "template" {
				t.Fatalf("technical failure disguised as ordinary silence: %+v", result.ProviderCalls)
			}
			expected := "failed"
			if mode == "timeout" {
				expected = "timeout"
			}
			if result.ProviderCalls[0].Result != expected {
				t.Fatalf("provider failure category not distinguished: %+v", result.ProviderCalls)
			}
			assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM rp_provider_calls WHERE model_id LIKE '%secret-upstream-details%' OR fallback_kind LIKE '%secret-upstream-details%'`, nil, 0)
			assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM rp_utterances`, nil, 1)
			assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM rp_npc_decisions WHERE action='silence'`, nil, 1)
			assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM audit_records WHERE json_extract(payload,'$.status')='provider_fallback'`, nil, 1)
			assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM audit_records WHERE payload LIKE '%secret-upstream-details%'`, nil, 0)
			assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM events WHERE instance_id=? AND branch_id=? AND event_sequence>? AND event_type NOT IN ('RPSpeechAccepted','RPNPCDecisionRecorded')`, []any{session.InstanceID, session.BranchID, initial.ObservationCursor}, 0)
			replay, err := service.PlayRPTurn(ctx, request)
			if err != nil || !replay.Replayed || replay.TurnRunID != result.TurnRunID || !reflect.DeepEqual(replay.NarrativeLines, result.NarrativeLines) || calls.Load() != 1 {
				t.Fatal("fallback replay repeated provider or changed silence", replay, err, calls.Load())
			}
			assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM rp_utterances`, nil, 1)
			assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM rp_npc_decisions WHERE action='silence'`, nil, 1)
			assertM2Value(t, ctx, store, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, []any{session.InstanceID, session.BranchID}, result.SettledSequence)

		})
	}
}
