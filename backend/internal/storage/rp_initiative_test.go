package storage

import (
	"context"
	"corerp.local/backend/internal/core"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestRPInitiativeMovementQuietAndProviderFailureRemainAtomic(t *testing.T) {
	for _, scenario := range []struct {
		name                   string
		proposal               core.RPDecisionProposal
		providerError          bool
		wantAction, wantStatus string
	}{
		{"quiet", core.RPDecisionProposal{Action: "silence"}, false, "silence", "validated"},
		{"leave", core.RPDecisionProposal{Action: "leave", DestinationPlaceID: "place_m2_home_ada"}, false, "leave", "validated"},
		{"illegal", core.RPDecisionProposal{Action: "leave", DestinationPlaceID: "invented-place"}, false, "silence", "provider_fallback"},
		{"unavailable", core.RPDecisionProposal{}, true, "silence", "provider_fallback"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			ctx := context.Background()
			path := filepath.Join(t.TempDir(), "initiative.db")
			s := openBootstrappedStore(t, ctx, path)
			defer func() { s.Close() }()
			session, _, view := newRPWaitTestSession(t, ctx, s)
			wait, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: "2026-09-22T03:00:00Z", Budget: 10, IdempotencyKey: "initiative"})
			if err != nil {
				t.Fatal(err)
			}
			r := core.RPInitiativeRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID, NPCEntityID: M2RPNPCID, TriggerEventID: wait.EventID}
			provider := rpDecisionProviderFunc(func(context.Context, core.RPDecisionInput) (core.RPDecisionProposal, error) {
				if scenario.providerError {
					return core.RPDecisionProposal{}, errors.New("private provider failure")
				}
				return scenario.proposal, nil
			})
			out, err := s.RunRPInitiative(ctx, r, provider)
			if err != nil || out.Action != scenario.wantAction || out.Status != scenario.wantStatus {
				t.Fatalf("result %+v %v", out, err)
			}
			wantReason := map[string]string{"illegal": "destination_not_reachable", "unavailable": "provider_failure"}[scenario.name]
			if out.ReasonCode != wantReason {
				t.Fatalf("reason %q want %q", out.ReasonCode, wantReason)
			}
			var recorded string
			if err := s.db.QueryRowContext(ctx, `SELECT payload FROM events WHERE event_id=?`, out.EventID).Scan(&recorded); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(recorded, "private provider failure") || strings.Contains(recorded, "invented-place") {
				t.Fatal("unsafe rejected candidate/provider detail persisted")
			}
			if wantReason == "" && strings.Contains(recorded, `"reason_code"`) {
				t.Fatal("empty reason changed legacy payload encoding")
			}
			assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_utterances`, nil, 0)
			if scenario.wantAction == "leave" {
				assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_positions WHERE agent_id=? AND place_id='place_m2_home_ada'`, []any{M2RPNPCID}, 1)
			} else {
				assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM outbox WHERE event_id=?`, []any{out.EventID}, 0)
			}
			retry, err := s.RunRPInitiative(ctx, r, nil)
			if err != nil || !retry.Replayed || retry.EventID != out.EventID || retry.ReasonCode != wantReason {
				t.Fatalf("retry %+v %v", retry, err)
			}
			if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) > 0 {
				t.Fatalf("projection mismatch %v %v", diff, err)
			}
			_, err = s.ConfigureStudioAccessLocal(ctx, StudioAccessRequest{Explain: true, Binding: core.CareerBinding{PrincipalID: "principal_operator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: out.EventSequence, IdempotencyKey: "initiative-inspector"}, TargetPrincipalID: "principal_creator", Status: "active"})
			if err != nil {
				t.Fatal(err)
			}
			query := StudioExplanationRequest{PrincipalID: "principal_creator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EventID: out.EventID}
			explained, err := s.ReadStudioExplanation(ctx, query)
			if err != nil || explained.CommittedDecision == nil {
				t.Fatal("initiative explanation missing", err)
			}
			c := explained.CommittedDecision
			wantExplanationReason := wantReason
			if wantExplanationReason == "" {
				wantExplanationReason = "not_recorded"
			}
			if c.SourceKind != "npc_initiative_command" || c.TriggerEventID != wait.EventID || c.Action != scenario.wantAction || c.Status != scenario.wantStatus || c.ReasonCode != wantExplanationReason {
				t.Fatal(c)
			}
			if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
				t.Fatal(err)
			}
			s.Close()
			s, err = Open(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			recovered, err := s.ReadStudioExplanation(ctx, query)
			if err != nil || recovered.CommittedDecision == nil || *recovered.CommittedDecision != *c {
				t.Fatal("initiative lineage changed after recovery", err)
			}
			retry, err = s.RunRPInitiative(ctx, r, nil)
			if err != nil || retry.ReasonCode != wantReason || !retry.Replayed {
				t.Fatal("reason lost on reopened retry", retry, err)
			}
		})
	}
}

func TestRPInitiativeCadenceSurvivesMidnight(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "midnight.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	session, read, view := newRPWaitTestSession(t, ctx, s)
	calls := 0
	provider := rpDecisionProviderFunc(func(context.Context, core.RPDecisionInput) (core.RPDecisionProposal, error) {
		calls++
		return core.RPDecisionProposal{Action: "silence"}, nil
	})
	for i, at := range []string{"2026-09-22T23:45:00Z", "2026-09-23T00:15:00Z"} {
		wait, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: read.PrincipalID, SessionID: session.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: at, Budget: 10, IdempotencyKey: at})
		if err != nil {
			t.Fatal(err)
		}
		out, err := s.RunRPInitiative(ctx, core.RPInitiativeRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, NPCEntityID: M2RPNPCID, TriggerEventID: wait.EventID}, provider)
		if err != nil {
			t.Fatal(err)
		}
		if i == 1 && (out.Status != "cooldown" || out.EventID != "") {
			t.Fatalf("midnight bypassed cadence %+v", out)
		}
		view, err = s.ObserveRPSession(ctx, read)
		if err != nil {
			t.Fatal(err)
		}
	}
	if calls != 1 {
		t.Fatalf("cooldown spent model calls: %d", calls)
	}
}

func TestRPInitiativeContextHasActualTimeTriggerWithoutPlayerSpeech(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "initiative-context.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Close() }()
	prepareRPLifeLongWorld(t, ctx, s)
	session, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: M2RPPlayerPrincipal, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2RPPlayerID, POV: "second_person", IdempotencyKey: "initiative-session"})
	if err != nil {
		t.Fatal(err)
	}
	read := core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID}
	view, err := s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	wait, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: "2026-09-22T08:00:00Z", Budget: 100, IdempotencyKey: "initiative-wait"})
	if err != nil {
		t.Fatal(err)
	}
	r := core.RPInitiativeRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, NPCEntityID: rpLifeNoraID, TriggerEventID: wait.EventID}
	input, err := s.BuildRPInitiativeInput(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	if input.Trigger == nil || input.Trigger.SourceEventID != wait.EventID || input.PlayerSpeechText != "" || input.SpeechEventID != "" || input.TurnID != "" || input.Life == nil || input.Life.Background == nil {
		t.Fatalf("false/missing context: %+v", input)
	}
	proposal, err := (core.DeterministicRPDecisionProvider{}).Propose(ctx, input)
	if err != nil || proposal.Action != "respond" {
		t.Fatalf("own real economic need failed to motivate initiative: %+v %v", proposal, err)
	}
	if err := core.ValidateRPDecisionProposal(input, proposal); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_utterances`, nil, 0)
	assertM2Value(t, ctx, s, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, []any{M2DemoInstanceID, M2DemoBranchID}, wait.EventSequence)
	for name, change := range map[string]func(*core.RPInitiativeRequest){
		"forged-trigger": func(r *core.RPInitiativeRequest) { r.TriggerEventID = m2RPSetupEventID },
		"offsite":        func(r *core.RPInitiativeRequest) { r.NPCEntityID = M2AgentAdaID },
		"player-as-npc":  func(r *core.RPInitiativeRequest) { r.NPCEntityID = M2RPPlayerID },
		"wrong-owner":    func(r *core.RPInitiativeRequest) { r.PrincipalID = "principal_creator" },
	} {
		t.Run(name, func(t *testing.T) {
			bad := r
			change(&bad)
			if _, err := s.BuildRPInitiativeInput(ctx, bad); err == nil {
				t.Fatal("invalid initiative scope accepted")
			}
		})
	}
	calls := 0
	provider := rpDecisionProviderFunc(func(ctx context.Context, input core.RPDecisionInput) (core.RPDecisionProposal, error) {
		calls++
		return (core.DeterministicRPDecisionProvider{}).Propose(ctx, input)
	})
	s.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "initiative rollback") }
	if _, err := s.RunRPInitiative(ctx, r, provider); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatalf("no rollback: %v", err)
	}
	s.beforeCommit = nil
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_utterances`, nil, 0)
	committed, err := s.RunRPInitiative(ctx, r, provider)
	if err != nil || committed.Action != "respond" || committed.Status != "validated" {
		t.Fatalf("initiative not committed: %+v %v", committed, err)
	}
	if calls != 2 {
		t.Fatalf("unexpected provider calls: %d", calls)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_utterances WHERE speaker_entity_id=?`, []any{rpLifeNoraID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_utterances WHERE speaker_entity_id=?`, []any{M2RPPlayerID}, 0)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_knowledge WHERE observer_agent_id=? AND source_event_id=? AND json_extract(claim_payload,'$.claim_type')='speaker_said'`, []any{M2RPPlayerID, committed.EventID}, 1)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := s.RunRPInitiative(ctx, r, nil)
	if err != nil || !retry.Replayed || retry.EventID != committed.EventID {
		t.Fatalf("initiative recovery: %+v %v", retry, err)
	}
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) > 0 {
		t.Fatalf("initiative replay differs: %v %v", diff, err)
	}
	view, err = s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SpeakRP(ctx, core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, Text: "现在我才开口。", IdempotencyKey: "later-speech"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.BuildRPInitiativeInput(ctx, r); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatalf("superseded time trigger accepted: %v", err)
	}
	view, err = s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	wait, err = s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: "2026-09-22T08:15:00Z", Budget: 100, IdempotencyKey: "too-soon"})
	if err != nil {
		t.Fatal(err)
	}
	r.TriggerEventID = wait.EventID
	cooldown, err := s.RunRPInitiative(ctx, r, nil)
	if err != nil || cooldown.Status != "cooldown" || cooldown.EventID != "" {
		t.Fatalf("cadence not bounded: %+v %v", cooldown, err)
	}
}
