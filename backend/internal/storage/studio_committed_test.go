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

func TestStudioCommittedDecisionTriggerAndOutcome(t *testing.T) {
	for _, action := range []string{"respond", "refuse", "leave", "silence", "wait"} {
		t.Run(action, func(t *testing.T) {
			ctx := context.Background()
			path := filepath.Join(t.TempDir(), "committed.db")
			s := openBootstrappedStore(t, ctx, path)
			defer func() { s.Close() }()
			setup, err := s.PrepareRPTravel(ctx)
			if err != nil {
				t.Fatal(err)
			}
			head := setup.EventSequence
			for _, target := range []string{"principal_creator", "principal_operator"} {
				grant, err := s.ConfigureStudioAccessLocal(ctx, StudioAccessRequest{Explain: true, Binding: core.CareerBinding{PrincipalID: "principal_operator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: head, IdempotencyKey: target}, TargetPrincipalID: target, Status: "active"})
				if err != nil {
					t.Fatal(err)
				}
				head = grant.EventSequence
			}
			session, _, initial := newRPWaitTestSession(t, ctx, s)
			speech, err := s.SpeakRP(ctx, core.RPSpeechRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID, ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "trigger", Text: "你好。"})
			if err != nil {
				t.Fatal(err)
			}
			r := core.RPDecisionRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID, TurnID: speech.TurnID, NPCEntityID: M2RPNPCID}
			proposal := core.RPDecisionProposal{Action: action}
			if action == "respond" || action == "refuse" {
				proposal.Text = "私人发言不应出现在关联元数据中。"
			}
			if action == "leave" {
				proposal.DestinationPlaceID = "place_m2_home_ada"
			}
			decision, err := s.DecideRP(ctx, r, rpDecisionProviderFunc(func(context.Context, core.RPDecisionInput) (core.RPDecisionProposal, error) { return proposal, nil }))
			if err != nil {
				t.Fatal(err)
			}
			query := StudioExplanationRequest{PrincipalID: "principal_creator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EventID: speech.EventID}
			candidate, err := s.ReadStudioExplanation(ctx, query)
			if err != nil || candidate.CommittedDecision != nil || candidate.CommittedEvidence != "not_recorded" {
				t.Fatal("candidate promoted to committed effect", candidate, err)
			}
			commit, err := s.CommitRPDecision(ctx, r, decision)
			if err != nil {
				t.Fatal(err)
			}
			query.EventID = commit.EventID
			got, err := s.ReadStudioExplanation(ctx, query)
			if err != nil || got.CommittedDecision == nil {
				t.Fatal("missing committed source", got, err)
			}
			c := got.CommittedDecision
			wantOutcome := map[string]string{"respond": "committed_speech", "refuse": "committed_speech", "leave": "committed_movement", "silence": "committed_silence", "wait": "committed_wait"}[action]
			if c.TriggerEventID != speech.EventID || c.TriggerSequence != speech.EventSequence || c.NPCID != M2RPNPCID || c.Action != action || c.Outcome != wantOutcome || got.CommittedEvidence != "recorded_trigger_and_outcome" {
				t.Fatal(got)
			}
			encoded, _ := json.Marshal(got)
			if strings.Contains(string(encoded), proposal.Text) && proposal.Text != "" {
				t.Fatal("raw speech leaked")
			}
			ops := query
			ops.PrincipalID = "principal_operator"
			redacted, err := s.ReadStudioExplanation(ctx, ops)
			if err != nil || redacted.CommittedDecision != nil || redacted.CommittedEvidence != "redacted" {
				t.Fatal("ops committed relation leaked", err)
			}
			assertM2Value(t, ctx, s, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, []any{M2DemoInstanceID, M2DemoBranchID}, commit.EventSequence)
			if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
				t.Fatal(err)
			}
			s.Close()
			s, err = Open(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			recovered, err := s.ReadStudioExplanation(ctx, query)
			if err != nil || !reflect.DeepEqual(got, recovered) {
				t.Fatal("committed evidence changed on recovery", err)
			}
		})
	}
}
