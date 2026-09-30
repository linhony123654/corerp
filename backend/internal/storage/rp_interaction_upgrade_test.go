package storage

import (
	"context"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPInteraction049UpgradePreservesAcceptedInterpretationFKAndLegacyChildren(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "upgrade-legacy-interactions.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	_, player, firstView := newRPWaitTestSession(t, ctx, s)
	first := core.RPInteractionRequest{PrincipalID: player.PrincipalID, SessionID: player.SessionID, Text: "原先已说的话。", Mode: "DIALOGUE", ExpectedCursor: firstView.ObservationCursor, IdempotencyKey: "legacy-settled"}
	service, err := NewRPService(s, core.DeterministicRPDecisionProvider{}, "deterministic")
	if err != nil {
		t.Fatal(err)
	}
	settled, err := service.RunRPInteraction(ctx, first)
	if err != nil || settled.Status != "settled" {
		t.Fatalf("legacy accepted plan setup failed: %+v %v", settled, err)
	}
	view, err := s.ObserveRPSession(ctx, player)
	if err != nil {
		t.Fatal(err)
	}
	second := core.RPInteractionRequest{PrincipalID: player.PrincipalID, SessionID: player.SessionID, Text: "旧版本尚待回执的话。", Mode: "DIALOGUE", ExpectedCursor: view.ObservationCursor, IdempotencyKey: "legacy-pending"}
	s.afterRPInteractionStep = func(int) error { return core.NewError(core.CodeInjectedFailure, "lost old child receipt") }
	if _, err := service.RunRPInteraction(ctx, second); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatalf("legacy pending child setup failed: %v", err)
	}
	s.afterRPInteractionStep = nil
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_interaction_interpretations i JOIN rp_interactions r ON r.interaction_id=i.interaction_id WHERE i.result='success' AND r.session_id=?`, []any{player.SessionID}, 2)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_interactions WHERE session_id=? AND status='open' AND pending_kind='speech'`, []any{player.SessionID}, 1)
	stripEmptyRPTypedMigrationsForTest(t, ctx, s)
	if err := s.Ready(ctx); err == nil {
		t.Fatal("049 fixture incorrectly reports latest schema readiness")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	for restart := 0; restart < 2; restart++ {
		s, err = Open(ctx, path)
		if err != nil {
			t.Fatalf("049→051 migration with 048 success foreign key failed on restart %d: %v", restart, err)
		}
		if err := s.Ready(ctx); err != nil {
			t.Fatal(err)
		}
		service, err = NewRPService(s, core.DeterministicRPDecisionProvider{}, "deterministic")
		if err != nil {
			t.Fatal(err)
		}
		if replay, err := service.RunRPInteraction(ctx, first); err != nil || replay.Status != "settled" || !replay.Replayed || replay.InteractionID != settled.InteractionID {
			t.Fatalf("old success FK or key changed: %+v %v", replay, err)
		}
		if resumed, err := service.ResumeRPInteraction(ctx, RPInteractionResumeRequest{PrincipalID: player.PrincipalID, SessionID: player.SessionID, IdempotencyKey: second.IdempotencyKey}); err != nil || resumed.Status != "settled" || len(resumed.Outcomes) != 1 {
			t.Fatalf("old speech child could not recover: %+v %v", resumed, err)
		}
		assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_utterances WHERE speaker_entity_id=? AND speech_text IN (?,?)`, []any{M2RPPlayerID, first.Text, second.Text}, 2)
		assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_interaction_interpretations i JOIN rp_interactions r ON r.interaction_id=i.interaction_id WHERE i.result='success' AND r.session_id=?`, []any{player.SessionID}, 2)
		if differences, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
			t.Fatalf("upgraded world has projection drift: %+v %v", differences, err)
		}
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
