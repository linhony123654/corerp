package storage

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/decision"
	"corerp.local/backend/internal/endpointpolicy"
)

// Transport tests need an explicitly authored character; the legacy travel
// demo intentionally has no persona and is no longer eligible for LLM RP.
func newRPAuthoredModelTestSession(t *testing.T, ctx context.Context, s *Store) (RPSession, core.RPSessionReadRequest, RPObservation) {
	t.Helper()
	if err := s.BootstrapDemo(ctx); err != nil {
		t.Fatal(err)
	}
	prepared, err := s.PrepareRPTravel(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.ConfigureStudioAccessLocal(ctx, StudioAccessRequest{Purpose: "create_world", Binding: core.CareerBinding{PrincipalID: "principal_operator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: prepared.EventSequence, IdempotencyKey: "authored-model-grant"}, TargetPrincipalID: "principal_creator", Status: "active"})
	if err != nil {
		t.Fatal(err)
	}
	declaration := studioCreateFixture("authored-model-world")
	declaration.Spec.People[1].Persona = "说话简短，先听清对方的问题，礼貌回应。不知道的事明确说不知道。"
	created, err := s.CreateStudioWorld(ctx, declaration)
	if err != nil {
		t.Fatal(err)
	}
	session, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: M2RPPlayerPrincipal, InstanceID: created.InstanceID, BranchID: created.BranchID, EntityID: created.EntityID, POV: "second_person", IdempotencyKey: "authored-model-session"})
	if err != nil {
		t.Fatal(err)
	}
	read := core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID}
	view, err := s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	return session, read, view
}

func TestRPIncompleteAuthorDataDoesNotCallModelAndSurvivesReplay(t *testing.T) {
	for _, missing := range []string{"persona", "whitespace_persona", "address"} {
		t.Run(missing, func(t *testing.T) {
			ctx := context.Background()
			path := filepath.Join(t.TempDir(), "readiness.db")
			s := openBootstrappedStore(t, ctx, path)
			defer func() { _ = s.Close() }()
			prepared, err := s.PrepareRPTravel(ctx)
			if err != nil {
				t.Fatal(err)
			}
			_, err = s.ConfigureStudioAccessLocal(ctx, StudioAccessRequest{Purpose: "create_world", Binding: core.CareerBinding{PrincipalID: "principal_operator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: prepared.EventSequence, IdempotencyKey: "readiness-author-grant"}, TargetPrincipalID: "principal_creator", Status: "active"})
			if err != nil {
				t.Fatal(err)
			}
			declaration := studioCreateFixture("missing-rp-" + missing)
			if missing == "whitespace_persona" {
				declaration.Spec.People[1].Persona = " \t\n "
			}
			if missing == "address" {
				declaration.Spec.People[1].Persona = "Cai 说话简短，先听清对方的问题。"
				declaration.Spec.Acquaintances = [][2]string{{"lin", "cai"}}
				declaration.Spec.Relationships = []core.StudioWorldRelationship{{From: "cai", To: "lin", Role: "熟人"}}
			}
			created, err := s.CreateStudioWorld(ctx, declaration)
			if err != nil {
				t.Fatal(err)
			}
			session, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: M2RPPlayerPrincipal, InstanceID: declaration.InstanceID, BranchID: M2DemoBranchID, EntityID: created.EntityID, POV: "second_person", IdempotencyKey: "not-ready-session"})
			if err != nil {
				t.Fatal(err)
			}
			read := core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID}
			view, err := s.ObserveRPSession(ctx, read)
			if err != nil {
				t.Fatal(err)
			}
			var calls atomic.Int32
			model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				_, _ = w.Write([]byte(`{"error":"incomplete character must not reach this server"}`))
			}))
			defer model.Close()
			provider, err := decision.NewChatProvider(decision.Config{Endpoint: model.URL + "/v1/chat/completions", Model: "fixture", DecisionFormat: "tool_call", EndpointPolicy: endpointpolicy.TestLocalhostPolicy()})
			if err != nil {
				t.Fatal(err)
			}
			request := core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, Text: "你好。", IdempotencyKey: "not-ready-turn"}
			turn, err := s.RunRPTurn(ctx, request, provider)
			if err != nil || turn.Status != "settled" || calls.Load() != 0 || len(turn.ProviderCalls) != 2 {
				t.Fatal("incomplete character generated a model call or failed to settle", turn.Status, calls.Load(), err)
			}
			call := turn.ProviderCalls[0]
			if call.Phase != "decision" || call.Result != "not_used" || call.Attempted || call.AttemptCount != 0 || call.FallbackKind != "rp_context_not_ready" {
				t.Fatal("missing data misreported as technical silence", call)
			}
			if strings.Contains(strings.Join(turn.NarrativeLines, "\n"), "Cai 说话简短") {
				t.Fatal("not-ready diagnostic leaked private persona")
			}
			assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_utterances u JOIN agent_profiles a ON a.agent_id=u.speaker_entity_id WHERE a.instance_id=? AND a.agent_id<>?`, []any{declaration.InstanceID, created.EntityID}, 0)
			if err := s.RebuildProjections(ctx, declaration.InstanceID, M2DemoBranchID); err != nil {
				t.Fatal(err)
			}
			_ = s.Close()
			s, err = Open(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			replayed, err := s.RunRPTurn(ctx, request, provider)
			if err != nil || !replayed.Replayed || calls.Load() != 0 || replayed.ProviderCalls[0].FallbackKind != "rp_context_not_ready" {
				t.Fatal("recovery/replay lost not-ready diagnostic or invented canon", err)
			}
		})
	}
}
