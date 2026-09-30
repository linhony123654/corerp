package storage

import (
	"context"
	"path/filepath"
	"sort"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPTurnActivationDirectAddressIsBoundedAndRestartStable(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "activation.db")
	s := openBootstrappedStore(t, ctx, path)
	setup, err := s.PrepareRPTravel(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ConfigureStudioAccessLocal(ctx, StudioAccessRequest{Purpose: "create_world", Binding: core.CareerBinding{PrincipalID: "principal_operator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: setup.EventSequence, IdempotencyKey: "activation-grant"}, TargetPrincipalID: "principal_creator", Status: "active"}); err != nil {
		t.Fatal(err)
	}

	const world = "activation-world"
	create := studioCreateFixture(world)
	create.Spec.Population = 4
	create.Spec.OpeningStockMinor = 4
	create.Spec.People = []core.StudioWorldPerson{
		{Key: "lin", Name: "Lin", Place: "home", Player: true},
		{Key: "cai", Name: "Cai", Place: "home"},
		{Key: "bo", Name: "Bo", Place: "home"},
		{Key: "ada", Name: "Ada", Place: "home"},
	}
	create.Spec.Acquaintances = [][2]string{{"lin", "bo"}}
	create.Spec.Relationships = []core.StudioWorldRelationship{
		{From: "lin", To: "bo", Role: "student", AddressTo: []string{"师傅"}},
		{From: "bo", To: "lin", Role: "teacher", AddressTo: []string{"队长"}},
	}
	create.SystemPackage.Content.SystemRules.RPExecutionMode = "orchestrated"
	create.SystemPackage.Content.SystemRules.MaxActiveResponders = 1
	create.SystemPackage.Manifest.ContentHash, _ = core.HashJSON(create.SystemPackage.Content)
	if _, err := s.CreateStudioWorld(ctx, create); err != nil {
		t.Fatal(err)
	}
	player, _ := core.StudioWorldObjectID(world, "entity", "lin")
	bo, _ := core.StudioWorldObjectID(world, "entity", "bo")
	cai, _ := core.StudioWorldObjectID(world, "entity", "cai")
	ada, _ := core.StudioWorldObjectID(world, "entity", "ada")
	session, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: M2RPPlayerPrincipal, InstanceID: world, BranchID: "br_main", EntityID: player, POV: "second_person", IdempotencyKey: "activation-session"})
	if err != nil {
		t.Fatal(err)
	}
	read := core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID}
	view, err := s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	request := core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, IdempotencyKey: "address-bo", Text: "Bo，请只由你回答。"}
	calls := []string{}
	provider := rpDecisionProviderFunc(func(_ context.Context, input core.RPDecisionInput) (core.RPDecisionProposal, error) {
		calls = append(calls, input.NPCEntityID)
		return core.RPDecisionProposal{Action: "silence"}, nil
	})
	injected := false
	s.afterRPTurnStage = func(stage string) error {
		if stage == "npc_effect_committed" && !injected {
			injected = true
			return core.NewError(core.CodeInjectedFailure, "restart after selected NPC effect")
		}
		return nil
	}
	if _, err := s.RunRPTurn(ctx, request, provider); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatalf("expected restart boundary: %v", err)
	}
	if len(calls) != 1 || calls[0] != bo {
		t.Fatalf("direct address selected wrong listeners: %v want %s", calls, bo)
	}
	var pinned string
	if err := s.db.QueryRowContext(ctx, `SELECT execution_mode || ':' || responder_limit || ':' || json_array_length(listener_ids_json) FROM rp_turn_runs WHERE session_id=? AND idempotency_key=?`, session.SessionID, request.IdempotencyKey).Scan(&pinned); err != nil || pinned != "orchestrated:1:3" {
		t.Fatalf("pinned activation policy = %q, %v", pinned, err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_turn_listener_activations a JOIN rp_turn_runs r ON r.turn_run_id=a.turn_run_id WHERE r.session_id=? AND r.idempotency_key=? AND a.npc_entity_id=? AND a.disposition='activated' AND a.reason_code='direct_address' AND a.activation_rank=0`, []any{session.SessionID, request.IdempotencyKey, bo}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_turn_listener_activations a JOIN rp_turn_runs r ON r.turn_run_id=a.turn_run_id WHERE r.session_id=? AND r.idempotency_key=? AND a.disposition='not_activated' AND a.reason_code='responder_limit'`, []any{session.SessionID, request.IdempotencyKey}, 2)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM observation_records WHERE source_event_id=(SELECT player_event_id FROM rp_turn_runs WHERE session_id=? AND idempotency_key=?)`, []any{session.SessionID, request.IdempotencyKey}, 3)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_npc_decisions WHERE session_id=?`, []any{session.SessionID}, 1)

	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	result, err := s.RunRPTurn(ctx, request, provider)
	if err != nil || result.Status != "settled" {
		t.Fatalf("resume bounded turn: %+v %v", result, err)
	}
	if len(calls) != 1 {
		t.Fatalf("restart re-ran provider: %v", calls)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_npc_decisions WHERE session_id=?`, []any{session.SessionID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_turn_listener_activations a JOIN rp_turn_runs r ON r.turn_run_id=a.turn_run_id WHERE r.session_id=? AND r.idempotency_key=?`, []any{session.SessionID, request.IdempotencyKey}, 3)

	view, err = s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	calls = nil
	aliasRequest := core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, IdempotencyKey: "address-authored-alias", Text: "师傅，请你回答。"}
	if _, err := s.RunRPTurn(ctx, aliasRequest, provider); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 || calls[0] != bo {
		t.Fatalf("authored player-to-NPC address did not select Bo: %v", calls)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_turn_listener_activations a JOIN rp_turn_runs r ON r.turn_run_id=a.turn_run_id WHERE r.session_id=? AND r.idempotency_key=? AND a.npc_entity_id=? AND a.disposition='activated' AND a.reason_code='direct_address'`, []any{session.SessionID, aliasRequest.IdempotencyKey, bo}, 1)
	// The reverse row tells Bo what to call Lin, not what Lin calls Bo.
	view, err = s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	reverseRequest := core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, IdempotencyKey: "reverse-is-not-address", Text: "队长，请你回答。"}
	if _, err := s.RunRPTurn(ctx, reverseRequest, provider); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_turn_listener_activations a JOIN rp_turn_runs r ON r.turn_run_id=a.turn_run_id WHERE r.session_id=? AND r.idempotency_key=? AND a.reason_code='direct_address'`, []any{session.SessionID, reverseRequest.IdempotencyKey}, 0)

	view, err = s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	calls = nil
	fallbackRequest := core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, IdempotencyKey: "stable-fallback", Text: "有人愿意回答吗？"}
	if _, err := s.RunRPTurn(ctx, fallbackRequest, provider); err != nil {
		t.Fatal(err)
	}
	expected := []string{ada, bo, cai}
	sort.Strings(expected)
	if len(calls) != 1 || calls[0] != expected[0] {
		t.Fatalf("stable fallback selected %v, want %s", calls, expected[0])
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_turn_listener_activations a JOIN rp_turn_runs r ON r.turn_run_id=a.turn_run_id WHERE r.session_id=? AND r.idempotency_key=? AND a.npc_entity_id=? AND a.disposition='activated' AND a.reason_code='stable_fallback' AND a.activation_rank=0`, []any{session.SessionID, fallbackRequest.IdempotencyKey, expected[0]}, 1)
}

func TestRPTurnExecutionModesSelectMateriallyDifferentRosters(t *testing.T) {
	base := []rpActivationCandidate{{id: "actor_a"}, {id: "actor_b", addressed: true}, {id: "actor_c"}}
	for _, test := range []struct {
		mode  string
		limit int
		want  int
	}{
		{mode: "legacy", limit: 0, want: 3},
		{mode: "deterministic", limit: 1, want: 1},
		{mode: "orchestrated", limit: 2, want: 1},
		{mode: "multi_agent", limit: 2, want: 2},
	} {
		candidates := append([]rpActivationCandidate(nil), base...)
		if test.mode == "orchestrated" || test.mode == "multi_agent" {
			sort.Slice(candidates, func(i, j int) bool {
				if candidates[i].addressed != candidates[j].addressed {
					return candidates[i].addressed
				}
				return candidates[i].id < candidates[j].id
			})
		}
		if got := rpTurnActiveCandidateCount(test.mode, test.limit, candidates); got != test.want {
			t.Fatalf("mode %s selected %d candidates, want %d", test.mode, got, test.want)
		}
		if test.mode == "deterministic" && candidates[0].id != "actor_a" {
			t.Fatalf("deterministic mode interpreted direct address: %+v", candidates)
		}
		if (test.mode == "orchestrated" || test.mode == "multi_agent") && candidates[0].id != "actor_b" {
			t.Fatalf("mode %s did not prioritize direct address: %+v", test.mode, candidates)
		}
	}
}
