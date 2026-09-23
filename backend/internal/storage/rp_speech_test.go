package storage

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPSpeechAcceptedHearersKnowAttributionWithoutMakingClaimTrue(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "rp-speech.db")
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	session, read, initial := newRPWaitTestSession(t, ctx, store)
	request := core.RPSpeechRequest{
		PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID,
		Text: "我有一百万。", SpeechAct: "statement",
		ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "first-claim",
	}
	result, err := store.SpeakRP(ctx, request)
	if err != nil || result.EventSequence != initial.ObservationCursor+1 || len(result.ListenerIDs) != 1 || result.ListenerIDs[0] != M2RPNPCID {
		t.Fatalf("speech did not select only same-place listener: %+v, %v", result, err)
	}
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM observation_records WHERE source_event_id = ?`, []any{result.EventID}, 1)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM agent_knowledge WHERE source_event_id = ? AND observer_agent_id = ?`, []any{result.EventID, M2RPNPCID}, 1)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM agent_knowledge WHERE source_event_id = ? AND observer_agent_id IN (?, ?)`, []any{result.EventID, M2AgentAdaID, M2AgentBoID}, 0)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM events WHERE event_type = 'RPSpeechAccepted'`, nil, 1)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM outbox WHERE event_id = ? AND published_at_utc IS NULL`, []any{result.EventID}, 1)
	var audienceJSON string
	if err := store.db.QueryRowContext(ctx, `SELECT audience_scope FROM outbox WHERE event_id = ?`, result.EventID).Scan(&audienceJSON); err != nil {
		t.Fatal(err)
	}
	var audience struct {
		Kind      string   `json:"kind"`
		EntityIDs []string `json:"entity_ids"`
	}
	if err := json.Unmarshal([]byte(audienceJSON), &audience); err != nil {
		t.Fatal(err)
	}
	if audience.Kind != "rp_participants" || len(audience.EntityIDs) != 2 || audience.EntityIDs[0] != M2RPPlayerID || audience.EntityIDs[1] != M2RPNPCID {
		t.Fatalf("speech Outbox audience leaked to offsite entities: %+v", audience)
	}
	var claimJSON string
	if err := store.db.QueryRowContext(ctx, `SELECT claim_payload FROM agent_knowledge WHERE source_event_id = ? AND observer_agent_id = ?`, result.EventID, M2RPNPCID).Scan(&claimJSON); err != nil {
		t.Fatal(err)
	}
	var claim rpSpeechClaim
	if err := json.Unmarshal([]byte(claimJSON), &claim); err != nil {
		t.Fatal(err)
	}
	if claim.ClaimType != "speaker_said" || claim.SpeakerEntityID != M2RPPlayerID || claim.Text != request.Text || claim.UtteranceID != result.UtteranceID {
		t.Fatalf("speech claim lost attribution: %+v", claim)
	}
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM events WHERE event_type IN ('PurchaseCommitted', 'CurrencyIssued') AND event_sequence > ?`, []any{initial.ObservationCursor}, 0)
	state, err := store.ReadRPSession(ctx, read)
	if err != nil || state.TurnState != "speech_committed" || state.TurnCursor != result.TurnID {
		t.Fatalf("turn stage was not committed with speech: %+v, %v", state, err)
	}
	replayed, err := store.SpeakRP(ctx, request)
	if err != nil || !replayed.Replayed || replayed.EventID != result.EventID || len(replayed.ListenerIDs) != 1 {
		t.Fatalf("speech retry duplicated acceptance: %+v, %v", replayed, err)
	}
	mismatch := request
	mismatch.Text = "别的话。"
	if _, err := store.SpeakRP(ctx, mismatch); !core.HasCode(err, core.CodeIdempotencyMismatch) {
		t.Fatalf("same key/different utterance accepted: %v", err)
	}
	var deliveredSpeechIDs []string
	publisher := OutboxPublishFunc(func(_ context.Context, message OutboxMessage) error {
		if message.Topic == "rp.speech.accepted" {
			deliveredSpeechIDs = append(deliveredSpeechIDs, message.OutboxID)
		}
		return nil
	})
	store.afterPublish = func(message OutboxMessage) error {
		if message.Topic == "rp.speech.accepted" {
			return core.NewError(core.CodeInjectedFailure, "speech delivered before publish mark")
		}
		return nil
	}
	if _, err := store.DispatchOutbox(ctx, 1000, publisher); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatalf("expected post-publish crash injection: %v", err)
	}
	if len(deliveredSpeechIDs) != 1 {
		t.Fatalf("speech Outbox was not handed to publisher before crash: %v", deliveredSpeechIDs)
	}
	store.afterPublish = nil
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if again, err := store.SpeakRP(ctx, request); err != nil || !again.Replayed || again.EventID != result.EventID {
		t.Fatalf("reopen did not preserve accepted speech: %+v, %v", again, err)
	}
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM outbox WHERE event_id = ? AND published_at_utc IS NULL`, []any{result.EventID}, 1)
	if _, err := store.DispatchOutbox(ctx, 1000, publisher); err != nil {
		t.Fatalf("pending speech notification did not recover: %v", err)
	}
	if len(deliveredSpeechIDs) != 2 || deliveredSpeechIDs[0] != deliveredSpeechIDs[1] {
		t.Fatalf("notification retry changed Outbox identity: %v", deliveredSpeechIDs)
	}
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM outbox WHERE event_id = ? AND published_at_utc IS NOT NULL`, []any{result.EventID}, 1)
	differences, err := store.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID)
	if err != nil || len(differences) != 0 {
		t.Fatalf("speech knowledge did not replay: %v, %v", differences, err)
	}
	if err := store.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatalf("speech knowledge could not rebuild: %v", err)
	}
	differences, err = store.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID)
	if err != nil || len(differences) != 0 {
		t.Fatalf("rebuilt speech knowledge diverged: %v, %v", differences, err)
	}
}

func TestRPSpeechTwoSamePlaceListenersAndPrecommitRollback(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "rp-speech-many.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, read, initial := newRPWaitTestSession(t, ctx, store)
	wait, err := store.WaitRP(ctx, core.RPWaitRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID,
		TargetWorldTime: M2AgentNoonTime, Budget: 10, ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "wait-noon"})
	if err != nil || wait.Status != "completed" {
		t.Fatalf("could not reach shared cafe scene: %+v, %v", wait, err)
	}
	current, err := store.ObserveRPSession(ctx, read)
	if err != nil || len(current.PresentEntities) < 2 {
		t.Fatalf("expected two co-located NPCs: %+v, %v", current, err)
	}
	request := core.RPSpeechRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID,
		Text: "大家好。", ExpectedCursor: current.ObservationCursor, IdempotencyKey: "greeting"}
	store.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "speech precommit") }
	if _, err := store.SpeakRP(ctx, request); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatalf("expected speech rollback: %v", err)
	}
	store.beforeCommit = nil
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM events WHERE event_type = 'RPSpeechAccepted'`, nil, 0)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM rp_utterances`, nil, 0)
	state, err := store.ReadRPSession(ctx, read)
	if err != nil || state.TurnState != "idle" || state.TurnCursor != "" {
		t.Fatalf("rolled-back speech advanced turn: %+v, %v", state, err)
	}
	result, err := store.SpeakRP(ctx, request)
	if err != nil || len(result.ListenerIDs) < 2 {
		t.Fatalf("speech did not reach same-place NPCs: %+v, %v", result, err)
	}
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM agent_knowledge WHERE source_event_id = ?`, []any{result.EventID}, int64(len(result.ListenerIDs)))
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM observation_records WHERE source_event_id = ?`, []any{result.EventID}, int64(len(result.ListenerIDs)))
	if _, err := store.db.ExecContext(ctx, `UPDATE rp_utterances SET speech_text = 'edited' WHERE utterance_id = ?`, result.UtteranceID); err == nil {
		t.Fatal("accepted utterance was mutable")
	}
	differences, err := store.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID)
	if err != nil || len(differences) != 0 {
		t.Fatalf("multi-listener speech did not replay: %v, %v", differences, err)
	}
}

func TestRPSpeechSchemaUpgradeFrom022PreservesSessionAndWorld(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "rp-speech-upgrade.db")
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	session, read, initial := newRPWaitTestSession(t, ctx, store)
	removeRPStyleSchemaForUpgradeTest(t, ctx, store)
	if _, err := store.db.ExecContext(ctx, `DROP TABLE rp_turn_runs`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `DROP TABLE rp_npc_decisions`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `DROP TABLE rp_utterances`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `DELETE FROM schema_meta WHERE schema_version IN (?, ?, ?)`, RPTurnSchemaVersion, RPNPCDecisionSchemaVersion, RPSpeechSchemaVersion); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	resumed, err := store.ResumeRPSession(ctx, read)
	if err != nil || resumed.SessionID != session.SessionID || resumed.ObservationCursor != initial.ObservationCursor {
		t.Fatalf("022→023 upgrade lost player session: %+v, %v", resumed, err)
	}
	result, err := store.SpeakRP(ctx, core.RPSpeechRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID,
		Text: "升级后仍能说话。", ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "post-upgrade"})
	if err != nil || result.EventID == "" {
		t.Fatalf("upgraded world cannot accept speech: %+v, %v", result, err)
	}
}
