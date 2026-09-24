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

func TestStudioExplanationExplicitGrantActualRejectionAndHearing(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "explanation.db")
	s := openBootstrappedStore(t, ctx, path)
	defer func() { s.Close() }()
	setup, err := s.PrepareRPTravel(ctx)
	if err != nil {
		t.Fatal(err)
	}
	head := setup.EventSequence
	grant := func(target, key string, explain bool) {
		t.Helper()
		record, err := s.ConfigureStudioAccessLocal(ctx, StudioAccessRequest{Explain: explain, Binding: core.CareerBinding{PrincipalID: "principal_operator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: head, IdempotencyKey: key}, TargetPrincipalID: target, Status: "active"})
		if err != nil {
			t.Fatal(err)
		}
		head = record.EventSequence
	}
	grant("principal_creator", "basic", false)
	r := StudioExplanationRequest{PrincipalID: "principal_creator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EventID: setup.EventID, Limit: 1}
	if _, err := s.ReadStudioExplanation(ctx, r); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatal("basic grant silently expanded", err)
	}
	grant("principal_creator", "explain", true)
	grant("principal_operator", "ops-explain", true)
	session, _, initial := newRPWaitTestSession(t, ctx, s)
	speech, err := s.SpeakRP(ctx, core.RPSpeechRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID, ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "explain-speech", Text: "我看见你了。"})
	if err != nil {
		t.Fatal(err)
	}
	decision := core.RPDecisionRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID, TurnID: speech.TurnID, NPCEntityID: M2RPNPCID}
	makeDecision := func(action, text string) {
		t.Helper()
		_, err := s.DecideRP(ctx, decision, rpDecisionProviderFunc(func(context.Context, core.RPDecisionInput) (core.RPDecisionProposal, error) {
			return core.RPDecisionProposal{Action: action, Text: text}, nil
		}))
		if action == "steal" || text != "" {
			if !core.HasCode(err, core.CodeInvalidArgument) {
				t.Fatal(err)
			}
		} else if err != nil {
			t.Fatal(err)
		}
	}
	makeDecision("steal", "")
	makeDecision("wait", "untrusted-private-proposal")
	r.EventID = speech.EventID
	first, err := s.ReadStudioExplanation(ctx, r)
	if err != nil || len(first.Diagnostics) != 1 || first.NextRecordOrder == 0 || first.Diagnostics[0].ReasonCode != "action_not_legal" || first.Diagnostics[0].NPCID != M2RPNPCID {
		t.Fatalf("missing real rejection: %+v %v", first, err)
	}
	if len(first.Observers) != 1 || first.Observers[0].EntityID != M2RPNPCID || first.ObservationEvidence != "recorded_observers_only" {
		t.Fatal("actual hearing absent", first)
	}
	makeDecision("silence", "")
	r.ThroughRecordOrder = first.ThroughRecordOrder
	r.AfterRecordOrder = first.NextRecordOrder
	second, err := s.ReadStudioExplanation(ctx, r)
	if err != nil || len(second.Diagnostics) != 1 || second.NextRecordOrder != 0 || second.Diagnostics[0].ReasonCode != "noop_contains_effects" {
		t.Fatal("audit page drifted with unchanged world head", err)
	}
	encoded, _ := json.Marshal(second)
	for _, forbidden := range []string{"untrusted-private-proposal", `"proposal"`, `"input_hash"`, `"claim_payload"`} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatal("raw private diagnostic leaked", forbidden)
		}
	}
	r.AfterRecordOrder = 0
	r.ThroughRecordOrder = 0
	r.Limit = 50
	r.ObserverID = M2AgentAdaID
	unheard, err := s.ReadStudioExplanation(ctx, r)
	if err != nil || unheard.ObservationEvidence != "not_recorded" || len(unheard.Observers) != 0 || len(unheard.Diagnostics) != 3 {
		t.Fatal("absence overstated or diagnostic lost", err)
	}
	r.ObserverID = M2RPNPCID
	heard, err := s.ReadStudioExplanation(ctx, r)
	if err != nil || heard.ObservationEvidence != "recorded_observation" {
		t.Fatal("specific actual hearing absent", err)
	}
	ops := r
	ops.ObserverID = ""
	ops.PrincipalID = "principal_operator"
	redacted, err := s.ReadStudioExplanation(ctx, ops)
	if err != nil || redacted.ObservationEvidence != "redacted" || len(redacted.Observers) != 0 {
		t.Fatal("ops observation leak", err)
	}
	encoded, _ = json.Marshal(redacted)
	if strings.Contains(string(encoded), M2RPNPCID) || strings.Contains(string(encoded), `"action"`) {
		t.Fatal("ops private diagnostics leak")
	}
	ops.ObserverID = M2RPNPCID
	if _, err := s.ReadStudioExplanation(ctx, ops); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatal("ops observer probe accepted", err)
	}
	player := r
	player.PrincipalID = M2RPPlayerPrincipal
	if _, err := s.ReadStudioExplanation(ctx, player); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatal("player debug knowledge leak", err)
	}
	var currentHead int64
	if err := s.db.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&currentHead); err != nil || currentHead != speech.EventSequence {
		t.Fatal("read/diagnostic mutated world", err)
	}
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := s.ReadStudioExplanation(ctx, r)
	if err != nil || !reflect.DeepEqual(heard, recovered) {
		t.Fatal("explain authority/evidence did not recover", err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE capability_grants SET field_scope='["event","rule"]' WHERE principal_id='principal_creator' AND capability_id='world.inspector.read'`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReadStudioExplanation(ctx, r); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatal("explain field revocation ignored", err)
	}
}
