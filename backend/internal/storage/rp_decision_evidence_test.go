package storage

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"corerp.local/backend/internal/core"
)

type rpDiagnosticTestError struct{}

func (rpDiagnosticTestError) Error() string                 { return "private-provider-error-token" }
func (rpDiagnosticTestError) RPDecisionFailureCode() string { return "proposal_ungrounded_decision" }

func TestRPDecisionRecordedReasonNoWorldEffectAndRecovery(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "decision-evidence.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Close() }()
	session, _, initial := newRPWaitTestSession(t, ctx, s)
	speech, err := s.SpeakRP(ctx, core.RPSpeechRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID, ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "evidence-speech", Text: "你好。"})
	if err != nil {
		t.Fatal(err)
	}
	r := core.RPDecisionRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID, TurnID: speech.TurnID, NPCEntityID: M2RPNPCID}
	for _, tc := range []struct {
		proposal       core.RPDecisionProposal
		failure        bool
		status, reason string
	}{
		{core.RPDecisionProposal{Action: "steal"}, false, "rejected", "action_not_legal"},
		{core.RPDecisionProposal{Action: "leave", DestinationPlaceID: "invented"}, false, "rejected", "destination_not_reachable"},
		{core.RPDecisionProposal{Action: "wait", Text: "forbidden"}, false, "rejected", "noop_contains_effects"},
		{core.RPDecisionProposal{}, true, "provider_fallback", "provider_failure"},
		{core.RPDecisionProposal{Action: "silence"}, false, "validated", ""},
	} {
		result, err := s.DecideRP(ctx, r, rpDecisionProviderFunc(func(context.Context, core.RPDecisionInput) (core.RPDecisionProposal, error) {
			if tc.failure {
				return tc.proposal, rpDiagnosticTestError{}
			}
			return tc.proposal, nil
		}))
		if tc.status == "rejected" {
			if !core.HasCode(err, core.CodeInvalidArgument) {
				t.Fatal("expected actual rejection", err)
			}
		} else if err != nil || result.Status != tc.status || result.ReasonCode != tc.reason || tc.failure && result.ProviderErrorCode != "proposal_ungrounded_decision" {
			t.Fatal("wrong decision diagnostic", err)
		}
		var raw string
		if err := s.db.QueryRowContext(ctx, `SELECT payload FROM audit_records WHERE related_event_id=? AND authority='non-authoritative' ORDER BY record_order DESC LIMIT 1`, speech.EventID).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		var record struct {
			Status            string `json:"status"`
			Reason            string `json:"reason_code"`
			ProviderErrorCode string `json:"provider_error_code"`
		}
		if err := json.Unmarshal([]byte(raw), &record); err != nil || record.Status != tc.status || record.Reason != tc.reason || tc.failure && record.ProviderErrorCode != "proposal_ungrounded_decision" || strings.Contains(raw, "private-provider-error-token") {
			t.Fatalf("unsafe/wrong audit: %s %v", raw, err)
		}
	}
	var head int64
	if err := s.db.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&head); err != nil || head != speech.EventSequence {
		t.Fatal("diagnostic produced world effect", err)
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
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_records WHERE related_event_id=? AND json_extract(payload,'$.reason_code') IN ('action_not_legal','destination_not_reachable','noop_contains_effects','provider_failure')`, speech.EventID).Scan(&count); err != nil || count != 4 {
		t.Fatal("reason evidence did not survive recovery", err)
	}
}
