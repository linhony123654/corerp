package storage

import (
	"context"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"corerp.local/backend/internal/core"
)

func TestRPInteractionModelFailureLeavesNoEffectAndSameKeyCanRetry(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "invalid-proposal.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, read, initial := newRPWaitTestSession(t, ctx, store)
	var calls atomic.Int32
	server := semanticFixtureServer(t, func(text string) string {
		if calls.Load() == 1 {
			return semanticModelReply("DIALOGUE", []core.RPInteractionStep{{Kind: "speech", SpeechText: "模型捏造的台词"}}, "")
		}
		return semanticModelReply("DIALOGUE", []core.RPInteractionStep{{Kind: "speech", SpeechText: "等我一下，我有件事想告诉你。"}}, "")
	}, &calls)
	defer server.Close()
	service, err := NewRPService(store, semanticFixtureProvider(t, server), "chat_completions")
	if err != nil {
		t.Fatal(err)
	}
	request := core.RPInteractionRequest{PrincipalID: read.PrincipalID, SessionID: session.SessionID, Text: "等我一下，我有件事想告诉你。", Mode: "AUTO", ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "failed-then-valid"}
	if _, err := service.RunRPInteraction(ctx, request); !core.HasCode(err, core.CodeStorageFailure) {
		t.Fatalf("invalid model proposal accepted: %v", err)
	}
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM rp_interactions WHERE session_id=? AND idempotency_key=?`, []any{session.SessionID, request.IdempotencyKey}, 0)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM rp_utterances WHERE speaker_entity_id=?`, []any{M2RPPlayerID}, 0)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM rp_interaction_interpretations WHERE session_id=? AND idempotency_key=? AND result='failed' AND reason='provider_proposal_speech' AND attempt_count=1`, []any{session.SessionID, request.IdempotencyKey}, 1)
	result, err := service.RunRPInteraction(ctx, request)
	if err != nil || result.Status != "settled" || result.InterpretationSource != "model" || calls.Load() != 2 {
		t.Fatalf("unaccepted retry could not recover: %+v %v calls=%d", result, err, calls.Load())
	}
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM rp_utterances WHERE speaker_entity_id=? AND speech_text=?`, []any{M2RPPlayerID, request.Text}, 1)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM rp_interaction_interpretations WHERE session_id=? AND idempotency_key=? AND result='success'`, []any{session.SessionID, request.IdempotencyKey}, 1)
	mismatch := request
	mismatch.Text = "改过的话。"
	if _, err := service.RunRPInteraction(ctx, mismatch); !core.HasCode(err, core.CodeIdempotencyMismatch) {
		t.Fatalf("accepted plan was reinterpreted: %v", err)
	}
	if calls.Load() != 2 {
		t.Fatal("mismatched retry reached model")
	}
}

func TestRPInteractionMisgroundedTravelNeverAcceptsPlanOrSpeech(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "wrong-travel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, read, initial := newRPWaitTestSession(t, ctx, store)
	text := "我把桌上的杯子推到邻居近旁，然后说「喝一点吧。」"
	var destination RPVisiblePlace
	for _, place := range initial.ReachablePlaces {
		if place.CanMoveNow && !strings.Contains(text, place.DisplayName) {
			destination = place
			break
		}
	}
	if destination.PlaceID == "" || destination.DisplayName == "" {
		t.Fatal("fixture lacks an authorized but unmentioned travel destination")
	}
	var calls atomic.Int32
	server := semanticFixtureServer(t, func(string) string {
		return semanticModelReply("MIXED", []core.RPInteractionStep{
			{Kind: "move", TargetPlaceID: destination.PlaceID}, {Kind: "speech", SpeechText: "喝一点吧。"},
		}, "")
	}, &calls)
	defer server.Close()
	service, err := NewRPService(store, semanticFixtureProvider(t, server), "chat_completions")
	if err != nil {
		t.Fatal(err)
	}
	request := core.RPInteractionRequest{PrincipalID: read.PrincipalID, SessionID: session.SessionID, Text: text, Mode: "AUTO", ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "wrong-travel"}
	if _, err := service.RunRPInteraction(ctx, request); !core.HasCode(err, core.CodeStorageFailure) {
		t.Fatalf("misgrounded character move reached the world: %v", err)
	}
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM rp_interactions WHERE session_id=?`, []any{session.SessionID}, 0)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM events WHERE event_type IN ('RPPlayerMoved','RPSpeechAccepted')`, nil, 0)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM rp_interaction_interpretations WHERE session_id=? AND result='failed' AND reason='provider_proposal_movement' AND attempt_count=1`, []any{session.SessionID}, 1)
	if calls.Load() != 1 {
		t.Fatalf("unexpected provider attempt count: %d", calls.Load())
	}
}

func TestRPInteractionModelConcurrentSameKeyOnlyInterpretedOnce(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "concurrent-model.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, read, initial := newRPWaitTestSession(t, ctx, store)
	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	server := semanticFixtureServer(t, func(text string) string {
		if calls.Load() == 1 {
			close(entered)
		}
		<-release
		return semanticModelReply("CLARIFICATION", nil, "ambiguous_target")
	}, &calls)
	defer server.Close()
	service, err := NewRPService(store, semanticFixtureProvider(t, server), "chat_completions")
	if err != nil {
		t.Fatal(err)
	}
	request := core.RPInteractionRequest{PrincipalID: read.PrincipalID, SessionID: session.SessionID, Text: "我想靠近她", Mode: "AUTO", ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "same-in-flight"}
	first := make(chan error, 1)
	go func() { _, err := service.RunRPInteraction(ctx, request); first <- err }()
	<-entered
	otherCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if _, err := service.RunRPInteraction(otherCtx, request); !core.HasCode(err, core.CodeCommandInProgress) {
		close(release)
		t.Fatalf("concurrent interpreter did not hold key: %v", err)
	}
	close(release)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	result, err := service.RunRPInteraction(ctx, request)
	if err != nil || !result.Replayed || result.Status != "clarification" || calls.Load() != 1 {
		t.Fatalf("replay called model again: %+v %v calls=%d", result, err, calls.Load())
	}
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM rp_utterances WHERE speaker_entity_id=?`, []any{M2RPPlayerID}, 0)
}

func TestRPInteractionInterpretation048UpgradeFromPreviousMigration(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "upgrade047.db")
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	// Recreate the empty 047 application-only schema in this disposable DB.
	// 046 had no event lineage columns; 048 and 049 must both append cleanly.
	stripEmptyRPTypedMigrationsForTest(t, ctx, store)
	for _, query := range []string{
		`DROP TABLE rp_interaction_interpretations`,
		`DROP TABLE rp_wait_activity_settlements`,
		`CREATE TABLE rp_wait_activity_settlements (intent_id TEXT PRIMARY KEY REFERENCES rp_wait_intents(intent_id),status TEXT NOT NULL CHECK (status IN ('pending','complete')),completed_at_utc TEXT,CHECK ((status='pending')=(completed_at_utc IS NULL))) STRICT`,
		`DELETE FROM schema_meta WHERE schema_version IN ('corerp-rp-interaction-interpretations-068-2026-09-27','corerp-rp-wait-settle-lineage-069-2026-09-27')`,
	} {
		if _, err := store.db.ExecContext(ctx, query); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Ready(ctx); err == nil {
		t.Fatal("047 fixture incorrectly reports 049 readiness")
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Ready(ctx); err != nil {
		t.Fatalf("048 upgrade failed: %v", err)
	}
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM schema_meta WHERE schema_version=?`, []any{RPInteractionInterpretationVersion}, 1)
}
