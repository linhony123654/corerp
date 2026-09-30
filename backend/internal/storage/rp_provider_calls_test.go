package storage

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPProviderReceiptIsDurableScopedAndNeverWorldAuthority(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "receipts.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	session, read, initial := newRPWaitTestSession(t, ctx, s)
	var calls int
	provider := rpDecisionProviderFunc(func(_ context.Context, _ core.RPDecisionInput) (core.RPDecisionProposal, error) {
		calls++
		var pending int
		if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_provider_calls WHERE session_id=? AND phase='decision' AND attempted=0 AND result='pending'`, session.SessionID).Scan(&pending); err != nil || pending != 1 {
			t.Fatalf("provider called without pending receipt: count=%d err=%v", pending, err)
		}
		return core.RPDecisionProposal{Action: "respond", Text: "我听见了。"}, nil
	})
	request := core.RPSpeechRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID, ExpectedCursor: initial.ObservationCursor, Text: "你好", IdempotencyKey: "receipt-turn"}
	turn, err := s.RunRPTurn(ctx, request, provider)
	if err != nil || calls != 1 || len(turn.ProviderCalls) != 2 || turn.ProviderCalls[0].Result != "success" || turn.ProviderCalls[0].Attempted || turn.ProviderCalls[0].AttemptCount != 0 || turn.ProviderCalls[1].AttemptCount != 0 {
		t.Fatalf("settled receipt: turn=%+v err=%v calls=%d", turn, err, calls)
	}
	initialHead := turn.SettledSequence
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	turn, err = s.RunRPTurn(ctx, request, core.DeterministicRPDecisionProvider{})
	if err != nil || !turn.Replayed || calls != 1 || len(turn.ProviderCalls) != 2 || turn.ProviderCalls[0].Result != "success" {
		t.Fatalf("replay receipt: turn=%+v err=%v calls=%d", turn, err, calls)
	}
	observation, err := s.ObserveRPSession(ctx, read)
	if err != nil || len(observation.RecentTurns) != 1 || len(observation.RecentTurns[0].ProviderCalls) != 2 {
		t.Fatalf("own history receipt: %+v err=%v", observation.RecentTurns, err)
	}
	raw, err := json.Marshal(observation.RecentTurns[0].ProviderCalls)
	if err != nil || strings.Contains(string(raw), "npc_entity_id") || strings.Contains(string(raw), "subject_id") || strings.Contains(string(raw), "call_id") {
		t.Fatalf("private provider identifiers leaked: %s err=%v", raw, err)
	}
	if _, err := s.ObserveRPSession(ctx, core.RPSessionReadRequest{PrincipalID: "some_other_principal", SessionID: session.SessionID}); err == nil {
		t.Fatal("another principal read scoped provider receipts")
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_provider_calls WHERE session_id=? AND result='success'`, session.SessionID).Scan(&calls); err != nil || calls != 2 {
		t.Fatalf("replay added a provider call: %d %v", calls, err)
	}
	if turn.SettledSequence != initialHead {
		t.Fatal("receipt changed the world head")
	}
	if diffs, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diffs) != 0 {
		t.Fatalf("provider diagnostics changed projections: %v %v", diffs, err)
	}
}

func TestRPProviderPendingAfterCrashAndCompletedReceiptImmutable(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "pending.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	session, _, _ := newRPWaitTestSession(t, ctx, s)
	scope := rpProviderCallScope{SessionID: session.SessionID, SubjectID: "event_example", Phase: "decision"}
	pendingID, err := s.beginRPProviderCall(ctx, scope, core.RPProviderMetadata{Kind: "chat_completions", Model: "safe-model"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var result, model string
	var attempted, attemptCount int
	if err := s.db.QueryRowContext(ctx, `SELECT result,model_id,attempted,attempt_count FROM rp_provider_calls WHERE call_id=?`, pendingID).Scan(&result, &model, &attempted, &attemptCount); err != nil || result != "pending" || model != "safe-model" || attempted != 0 || attemptCount != 0 {
		t.Fatalf("unknown crash outcome misrepresented: %s %s attempts=%d/%d err=%v", result, model, attempted, attemptCount, err)
	}
	if err := s.finishRPProviderCall(ctx, pendingID, "timeout", "silence", "", 1); err != nil {
		t.Fatal(err)
	}
	if err := s.finishRPProviderCall(ctx, pendingID, "success", "", "", 0); err == nil {
		t.Fatal("completed receipt was overwritten")
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM rp_provider_calls WHERE call_id=?`, pendingID); err == nil {
		t.Fatal("completed receipt was deleted")
	}
	if err := s.db.QueryRowContext(ctx, `SELECT result,attempted,attempt_count FROM rp_provider_calls WHERE call_id=?`, pendingID).Scan(&result, &attempted, &attemptCount); err != nil || result != "timeout" || attempted != 1 || attemptCount != 1 {
		t.Fatalf("receipt changed after completion: %s attempts=%d/%d err=%v", result, attempted, attemptCount, err)
	}
}

func TestRPProviderReceipt045UpgradesPreviousSchemaAndReadiness(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "upgrade-from-044.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	// This disposable fixture rolls back the appended application-only
	// schemas to 044; no existing migration or user DB is rewritten.
	stripEmptyRPTypedMigrationsForTest(t, ctx, s)
	for _, stmt := range []string{
		`DELETE FROM schema_meta WHERE schema_version='corerp-rp-wait-settle-lineage-069-2026-09-27'`,
		`DROP TABLE rp_interaction_interpretations`,
		`DELETE FROM schema_meta WHERE schema_version='corerp-rp-interaction-interpretations-068-2026-09-27'`,
		`DROP TRIGGER rp_turn_explicit_rebases_no_update`,
		`DROP TRIGGER rp_turn_explicit_rebases_no_delete`,
		`DROP TABLE rp_turn_explicit_rebases`,
		`DELETE FROM schema_meta WHERE schema_version='corerp-rp-turn-explicit-rebases-067-2026-09-27'`,
		`DROP TABLE rp_wait_activity_settlements`,
		`DELETE FROM schema_meta WHERE schema_version='corerp-rp-wait-derived-settle-066-2026-09-27'`,
		`DROP TRIGGER rp_provider_calls_no_delete`,
		`DROP TRIGGER rp_provider_calls_no_update_finished`,
		`DROP TABLE rp_provider_calls`,
		`DELETE FROM schema_meta WHERE schema_version='corerp-rp-provider-receipts-065-2026-09-27'`,
	} {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Ready(ctx); err == nil {
		t.Fatal("044 database incorrectly reports 045 readiness")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Ready(ctx); err != nil {
		t.Fatal("045–049 upgrade not ready", err)
	}
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_meta WHERE schema_version=?`, SchemaVersion).Scan(&count); err != nil || count != 1 {
		t.Fatalf("latest migration absent: %d %v", count, err)
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO rp_provider_calls(call_id,session_id,subject_id,phase,provider_kind,attempted,result,started_at_utc) VALUES ('invalid','other-session','event','decision','custom',0,'pending','2026-09-27T00:00:00Z')`); err == nil {
		t.Fatal("045 foreign-key scope not enforced")
	}
}

func TestRPProviderReceiptRejectsUntrustedFallback(t *testing.T) {
	for _, reason := range []string{"prose_transport unavailable: private-key", "http://internal.example/token", "world_declares_full_prose_without_prose_provider"} {
		got := sanitizeRPNarrativeFallback(reason)
		if strings.Contains(got, "private-key") || strings.Contains(got, "internal.example") {
			t.Fatalf("fallback contains remote information: %s", got)
		}
	}
}
