package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"corerp.local/backend/internal/core"
)

type RPSpeechResult struct {
	CommandID     string   `json:"command_id"`
	EventID       string   `json:"event_id"`
	EventSequence int64    `json:"event_sequence"`
	TurnID        string   `json:"turn_id"`
	UtteranceID   string   `json:"utterance_id"`
	WorldTime     string   `json:"world_time"`
	PlaceID       string   `json:"place_id"`
	ListenerIDs   []string `json:"listener_ids"`
	Replayed      bool     `json:"replayed"`
}

type rpSpeechEvent struct {
	SessionID       string   `json:"session_id"`
	TurnID          string   `json:"turn_id"`
	ParentTurnID    string   `json:"parent_turn_id,omitempty"`
	UtteranceID     string   `json:"utterance_id"`
	SpeakerEntityID string   `json:"speaker_entity_id"`
	PlaceID         string   `json:"place_id"`
	Text            string   `json:"text"`
	SpeechAct       string   `json:"speech_act"`
	ListenerIDs     []string `json:"listener_ids"`
}

type rpSpeechClaim struct {
	ClaimType       string `json:"claim_type"`
	SpeakerEntityID string `json:"speaker_entity_id"`
	UtteranceID     string `json:"utterance_id"`
	Text            string `json:"text"`
	SpeechAct       string `json:"speech_act"`
}

// SpeakRP accepts one utterance as a world fact. Its co-located listeners and
// attributed knowledge are committed with the Event and turn stage, not later
// by transcript rendering, vector indexing or notification delivery.
func (s *Store) SpeakRP(ctx context.Context, request core.RPSpeechRequest) (RPSpeechResult, error) {
	if err := request.Validate(); err != nil {
		return RPSpeechResult{}, err
	}
	if request.SpeechAct == "" {
		request.SpeechAct = "statement"
	}
	requestHash, err := core.HashJSON(request)
	if err != nil {
		return RPSpeechResult{}, err
	}
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return RPSpeechResult{}, core.WrapError(core.CodeStorageFailure, "begin RP speech", err)
	}
	defer tx.Rollback(ctx)
	session, err := loadRPSession(ctx, tx.conn, request.PrincipalID, request.SessionID)
	if err != nil {
		return RPSpeechResult{}, err
	}
	if err := authorizeRPControl(ctx, tx.conn, request.PrincipalID, session.InstanceID, session.BranchID, session.ControlledEntityID); err != nil {
		return RPSpeechResult{}, err
	}
	commandKey := "rp_speech:" + session.SessionID + ":" + request.IdempotencyKey
	var existingCommandID, existingHash, existingStatus string
	err = tx.conn.QueryRowContext(ctx, `SELECT command_id, request_hash, status FROM commands WHERE instance_id = ? AND branch_id = ? AND command_type = 'RPSpeak' AND idempotency_key = ?`, session.InstanceID, session.BranchID, commandKey).Scan(&existingCommandID, &existingHash, &existingStatus)
	if err == nil {
		if existingHash != requestHash {
			return RPSpeechResult{}, core.NewError(core.CodeIdempotencyMismatch, "RP speech key was used with another request")
		}
		if existingStatus != "committed" {
			return RPSpeechResult{}, core.NewError(core.CodeCommandInProgress, "RP speech command is not committed")
		}
		return loadRPSpeechResult(ctx, tx.conn, existingCommandID, true)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return RPSpeechResult{}, core.WrapError(core.CodeStorageFailure, "look up RP speech", err)
	}
	if session.Status != "active" {
		return RPSpeechResult{}, core.NewError(core.CodeBranchConflict, "RP session is closed")
	}
	if err := validateRPBinding(ctx, tx.conn, session.InstanceID, session.BranchID, session.ControlledEntityID); err != nil {
		return RPSpeechResult{}, err
	}
	var pendingWaits int
	if err := tx.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_wait_intents i JOIN rp_sessions s ON s.session_id = i.session_id WHERE s.instance_id = ? AND s.branch_id = ? AND i.status = 'pending'`, session.InstanceID, session.BranchID).Scan(&pendingWaits); err != nil {
		return RPSpeechResult{}, core.WrapError(core.CodeStorageFailure, "check pending wait before RP speech", err)
	}
	if pendingWaits != 0 {
		return RPSpeechResult{}, core.NewError(core.CodeCommandInProgress, "RP wait must complete before speech")
	}
	var head int64
	var worldTime, placeID string
	err = tx.conn.QueryRowContext(ctx, `
		SELECT b.head_sequence, c.current_world_time, p.place_id
		FROM branches b JOIN world_clocks c ON c.instance_id = b.instance_id AND c.branch_id = b.branch_id
		JOIN agent_profiles a ON a.instance_id = b.instance_id AND a.branch_id = b.branch_id
		JOIN agent_positions p ON p.agent_id = a.agent_id
		WHERE b.instance_id = ? AND b.branch_id = ? AND a.agent_id = ? AND a.status = 'active'`,
		session.InstanceID, session.BranchID, session.ControlledEntityID).Scan(&head, &worldTime, &placeID)
	if err != nil {
		return RPSpeechResult{}, classifyMissing(err, "RP speaker state")
	}
	if request.ExpectedCursor != head || session.ObservationCursor != head {
		return RPSpeechResult{}, core.NewError(core.CodeBranchConflict, "RP speech requires a current observation cursor")
	}
	if err := ensureCohortTransitionChronology(ctx, tx.conn, session.InstanceID, session.BranchID, worldTime); err != nil {
		return RPSpeechResult{}, err
	}
	listeners, err := rpCoLocatedEntityIDs(ctx, tx.conn, session.InstanceID, session.BranchID, placeID, session.ControlledEntityID)
	if err != nil {
		return RPSpeechResult{}, err
	}
	sequence := head + 1
	var epochID string
	if err := tx.conn.QueryRowContext(ctx, `SELECT epoch_id FROM rule_epochs WHERE instance_id = ? AND branch_id = ? AND start_sequence <= ? AND (end_sequence IS NULL OR ? < end_sequence)`, session.InstanceID, session.BranchID, sequence, sequence).Scan(&epochID); err != nil {
		return RPSpeechResult{}, classifyMissing(err, "RP speech Rule Epoch")
	}
	keyHash, err := core.HashJSON(struct{ SessionID, Key string }{session.SessionID, request.IdempotencyKey})
	if err != nil {
		return RPSpeechResult{}, err
	}
	suffix := keyHash[7:]
	commandID, attemptID := "cmd_rp_speech_"+suffix, "attempt_rp_speech_"+suffix
	batchID, eventID := "batch_rp_speech_"+suffix, "event_rp_speech_"+suffix
	turnID, utteranceID := "turn_rp_speech_"+suffix, "utterance_rp_speech_"+suffix
	payload := rpSpeechEvent{SessionID: session.SessionID, TurnID: turnID, UtteranceID: utteranceID, SpeakerEntityID: session.ControlledEntityID, PlaceID: placeID, Text: request.Text, SpeechAct: request.SpeechAct, ListenerIDs: listeners}
	payloadJSON, err := core.CanonicalJSON(payload)
	if err != nil {
		return RPSpeechResult{}, err
	}
	batchHash, err := core.HashJSON(struct {
		CommandID string        `json:"command_id"`
		Sequence  int64         `json:"sequence"`
		WorldTime string        `json:"world_time"`
		Payload   rpSpeechEvent `json:"payload"`
	}{commandID, sequence, worldTime, payload})
	if err != nil {
		return RPSpeechResult{}, err
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	statements := []struct {
		name, query string
		args        []any
	}{
		{"RP speech command", `INSERT INTO commands(command_id, instance_id, branch_id, command_type, idempotency_key, request_hash, expected_head, principal_id, command_policy, status, created_at_utc) VALUES (?, ?, ?, 'RPSpeak', ?, ?, ?, ?, '{"authorization":"rp-session-control"}', 'pending', ?)`, []any{commandID, session.InstanceID, session.BranchID, commandKey, requestHash, head, request.PrincipalID, now}},
		{"RP speech attempt", `INSERT INTO command_attempts(command_id, attempt_no, attempt_id, status, lease_owner, lease_until_utc, proposal_hash, created_at_utc) VALUES (?, 1, ?, 'ready', 'corerp-rp1', ?, ?, ?)`, []any{commandID, attemptID, s.now().UTC().Add(30 * time.Second).Format(time.RFC3339Nano), requestHash, now}},
		{"RP speech batch", `INSERT INTO event_batches(batch_id, command_id, attempt_no, instance_id, branch_id, epoch_id, expected_head, first_sequence, last_sequence, event_count, world_time, batch_hash, committed_at_utc) VALUES (?, ?, 1, ?, ?, ?, ?, ?, ?, 1, ?, ?, ?)`, []any{batchID, commandID, session.InstanceID, session.BranchID, epochID, head, sequence, sequence, worldTime, batchHash, now}},
		{"RP speech event", `INSERT INTO events(event_id, batch_id, instance_id, branch_id, event_sequence, batch_index, event_type, actor_id, world_time, payload) VALUES (?, ?, ?, ?, ?, 0, 'RPSpeechAccepted', ?, ?, ?)`, []any{eventID, batchID, session.InstanceID, session.BranchID, sequence, session.ControlledEntityID, worldTime, string(payloadJSON)}},
		{"RP immutable utterance", `INSERT INTO rp_utterances(utterance_id, event_id, session_id, turn_id, speaker_entity_id, place_id, world_time, speech_text, speech_act, listener_count) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, []any{utteranceID, eventID, session.SessionID, turnID, session.ControlledEntityID, placeID, worldTime, request.Text, request.SpeechAct, len(listeners)}},
	}
	for _, statement := range statements {
		if err := execAgentOne(ctx, tx.conn, statement.name, statement.query, statement.args...); err != nil {
			return RPSpeechResult{}, err
		}
	}
	if err := insertRPSpeechHearings(ctx, tx.conn, eventID, sequence, session.ControlledEntityID, placeID, worldTime, utteranceID, request.Text, request.SpeechAct, listeners); err != nil {
		return RPSpeechResult{}, err
	}
	if err := execAgentOne(ctx, tx.conn, "advance RP speech clock lineage", `UPDATE world_clocks SET projection_version = projection_version + 1, last_event_sequence = ? WHERE instance_id = ? AND branch_id = ? AND current_world_time = ?`, sequence, session.InstanceID, session.BranchID, worldTime); err != nil {
		return RPSpeechResult{}, err
	}
	if err := execAgentOne(ctx, tx.conn, "advance RP speech branch", `UPDATE branches SET head_sequence = ? WHERE instance_id = ? AND branch_id = ? AND head_sequence = ?`, sequence, session.InstanceID, session.BranchID, head); err != nil {
		return RPSpeechResult{}, err
	}
	if err := execAgentOne(ctx, tx.conn, "commit RP speech turn stage", `UPDATE rp_sessions SET turn_cursor = ?, turn_state = 'speech_committed' WHERE session_id = ? AND status = 'active'`, turnID, session.SessionID); err != nil {
		return RPSpeechResult{}, err
	}
	if err := s.insertAgentAudit(ctx, tx.conn, "audit_"+commandID, "observation", eventID, commandID, attemptID, worldTime, now, payloadJSON); err != nil {
		return RPSpeechResult{}, err
	}
	if err := insertRPParticipantOutbox(ctx, tx.conn, "outbox_"+commandID, eventID, "rp.speech.accepted", session.InstanceID, session.BranchID, session.ControlledEntityID, listeners, payloadJSON); err != nil {
		return RPSpeechResult{}, err
	}
	if err := execAgentOne(ctx, tx.conn, "commit RP speech attempt", `UPDATE command_attempts SET status = 'committed', finished_at_utc = ? WHERE command_id = ? AND attempt_no = 1 AND status = 'ready'`, now, commandID); err != nil {
		return RPSpeechResult{}, err
	}
	if err := execAgentOne(ctx, tx.conn, "commit RP speech command", `UPDATE commands SET status = 'committed' WHERE command_id = ? AND status = 'pending'`, commandID); err != nil {
		return RPSpeechResult{}, err
	}
	if s.beforeCommit != nil {
		if err := s.beforeCommit(); err != nil {
			return RPSpeechResult{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return RPSpeechResult{}, core.WrapError(core.CodeStorageFailure, "commit RP speech", err)
	}
	return RPSpeechResult{commandID, eventID, sequence, turnID, utteranceID, worldTime, placeID, listeners, false}, nil
}

func insertRPSpeechHearings(ctx context.Context, conn *sql.Conn, eventID string, sequence int64, speakerID, placeID, worldTime, utteranceID, speechText, speechAct string, listeners []string) error {
	claim := rpSpeechClaim{"speaker_said", speakerID, utteranceID, speechText, speechAct}
	claimJSON, err := core.CanonicalJSON(claim)
	if err != nil {
		return err
	}
	for _, listenerID := range listeners {
		observationID := fmt.Sprintf("observation_%s_%s_%s", eventID, listenerID, speakerID)
		claimKey := "speech:" + eventID
		if err := execAgentOne(ctx, conn, "RP speech hearing evidence", `INSERT INTO observation_records(observation_id, source_event_id, observer_agent_id, subject_agent_id, place_id, channel, observed_world_time, claim_key, claim_payload) VALUES (?, ?, ?, ?, ?, 'co_location', ?, ?, ?)`, observationID, eventID, listenerID, speakerID, placeID, worldTime, claimKey, string(claimJSON)); err != nil {
			return err
		}
		if err := execAgentOne(ctx, conn, "RP listener knowledge", `INSERT INTO agent_knowledge(observer_agent_id, claim_key, subject_agent_id, place_id, source_event_id, observation_id, learned_world_time, claim_payload, projection_version, last_event_sequence) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 0, ?)`, listenerID, claimKey, speakerID, placeID, eventID, observationID, worldTime, string(claimJSON), sequence); err != nil {
			return err
		}
	}
	return nil
}

func insertRPParticipantOutbox(ctx context.Context, conn *sql.Conn, outboxID, eventID, topic, instanceID, branchID, speakerID string, listeners []string, payload []byte) error {
	participants := append([]string{speakerID}, listeners...)
	scope := struct {
		Kind       string   `json:"kind"`
		InstanceID string   `json:"instance_id"`
		BranchID   string   `json:"branch_id"`
		EntityIDs  []string `json:"entity_ids"`
	}{"rp_participants", instanceID, branchID, participants}
	scopeJSON, err := core.CanonicalJSON(scope)
	if err != nil {
		return err
	}
	scopeHash, err := core.HashJSON(scope)
	if err != nil {
		return err
	}
	return execAgentOne(ctx, conn, "insert scoped RP participant Outbox", `INSERT INTO outbox(outbox_id, event_id, topic, audience_scope, audience_scope_hash, payload) VALUES (?, ?, ?, ?, ?, ?)`, outboxID, eventID, topic, string(scopeJSON), scopeHash, string(payload))
}

func loadRPSpeechResult(ctx context.Context, conn *sql.Conn, commandID string, replayed bool) (RPSpeechResult, error) {
	var result RPSpeechResult
	var payloadJSON string
	if err := conn.QueryRowContext(ctx, `SELECT e.event_id, e.event_sequence, e.world_time, e.payload FROM events e JOIN event_batches b ON b.batch_id = e.batch_id WHERE b.command_id = ? AND e.event_type = 'RPSpeechAccepted'`, commandID).Scan(&result.EventID, &result.EventSequence, &result.WorldTime, &payloadJSON); err != nil {
		return RPSpeechResult{}, classifyMissing(err, "committed RP speech")
	}
	var payload rpSpeechEvent
	if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
		return RPSpeechResult{}, core.WrapError(core.CodeStorageFailure, "decode committed RP speech", err)
	}
	result.CommandID = commandID
	result.TurnID = payload.TurnID
	result.UtteranceID = payload.UtteranceID
	result.PlaceID = payload.PlaceID
	result.ListenerIDs = payload.ListenerIDs
	result.Replayed = replayed
	return result, nil
}
