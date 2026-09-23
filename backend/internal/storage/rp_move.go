package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"corerp.local/backend/internal/core"
)

type RPMoveResult struct {
	CommandID     string `json:"command_id"`
	EventID       string `json:"event_id"`
	EventSequence int64  `json:"event_sequence"`
	WorldTime     string `json:"world_time"`
	FromPlaceID   string `json:"from_place_id"`
	ToPlaceID     string `json:"to_place_id"`
	Replayed      bool   `json:"replayed"`
}

type rpMoveEvent struct {
	SessionID          string   `json:"session_id"`
	EntityID           string   `json:"entity_id"`
	FromPlaceID        string   `json:"from_place_id"`
	ToPlaceID          string   `json:"to_place_id"`
	CoLocatedEntityIDs []string `json:"co_located_entity_ids"`
}

// MoveRP is a player command, not a UI location update. The existing Agent
// movement fact/projection is authoritative; an already-completed immediate
// scheduler entry satisfies schema 007's movement lineage in the same batch.
func (s *Store) MoveRP(ctx context.Context, request core.RPMoveRequest) (RPMoveResult, error) {
	if err := request.Validate(); err != nil {
		return RPMoveResult{}, err
	}
	requestHash, err := core.HashJSON(request)
	if err != nil {
		return RPMoveResult{}, err
	}
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return RPMoveResult{}, core.WrapError(core.CodeStorageFailure, "begin RP move", err)
	}
	defer tx.Rollback(ctx)
	session, err := loadRPSession(ctx, tx.conn, request.PrincipalID, request.SessionID)
	if err != nil {
		return RPMoveResult{}, err
	}
	if err := authorizeRPControl(ctx, tx.conn, request.PrincipalID, session.InstanceID, session.BranchID, session.ControlledEntityID); err != nil {
		return RPMoveResult{}, err
	}
	commandKey := "rp_move:" + session.SessionID + ":" + request.IdempotencyKey
	var existingCommandID, existingHash, existingStatus string
	err = tx.conn.QueryRowContext(ctx, `SELECT command_id, request_hash, status FROM commands WHERE instance_id = ? AND branch_id = ? AND command_type = 'RPPlayerMove' AND idempotency_key = ?`, session.InstanceID, session.BranchID, commandKey).Scan(&existingCommandID, &existingHash, &existingStatus)
	if err == nil {
		if existingHash != requestHash {
			return RPMoveResult{}, core.NewError(core.CodeIdempotencyMismatch, "RP move key was used with another request")
		}
		if existingStatus != "committed" {
			return RPMoveResult{}, core.NewError(core.CodeCommandInProgress, "RP move command is not committed")
		}
		return loadRPMoveResult(ctx, tx.conn, existingCommandID, true)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return RPMoveResult{}, core.WrapError(core.CodeStorageFailure, "look up RP move", err)
	}
	if session.Status != "active" {
		return RPMoveResult{}, core.NewError(core.CodeBranchConflict, "RP session is closed")
	}
	var pendingWaits int
	if err := tx.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_wait_intents i JOIN rp_sessions s ON s.session_id = i.session_id WHERE s.instance_id = ? AND s.branch_id = ? AND i.status = 'pending'`, session.InstanceID, session.BranchID).Scan(&pendingWaits); err != nil {
		return RPMoveResult{}, core.WrapError(core.CodeStorageFailure, "check pending RP wait before move", err)
	}
	if pendingWaits != 0 {
		return RPMoveResult{}, core.NewError(core.CodeCommandInProgress, "RP wait must complete before another player action")
	}
	var pendingTurns int
	if err := tx.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_turn_runs r JOIN rp_sessions s ON s.session_id = r.session_id WHERE s.instance_id = ? AND s.branch_id = ? AND r.status <> 'settled'`, session.InstanceID, session.BranchID).Scan(&pendingTurns); err != nil {
		return RPMoveResult{}, core.WrapError(core.CodeStorageFailure, "check pending RP turn before move", err)
	}
	if pendingTurns != 0 {
		return RPMoveResult{}, core.NewError(core.CodeCommandInProgress, "RP turn must settle before another player action")
	}
	if err := validateRPBinding(ctx, tx.conn, session.InstanceID, session.BranchID, session.ControlledEntityID); err != nil {
		return RPMoveResult{}, err
	}
	var head, positionVersion, currentDay int64
	var worldTime, actualPlace string
	err = tx.conn.QueryRowContext(ctx, `
		SELECT b.head_sequence, c.current_world_time, c.current_day, p.place_id, p.projection_version
		FROM branches b JOIN world_clocks c ON c.instance_id = b.instance_id AND c.branch_id = b.branch_id
		JOIN agent_profiles a ON a.instance_id = b.instance_id AND a.branch_id = b.branch_id
		JOIN agent_positions p ON p.agent_id = a.agent_id
		WHERE b.instance_id = ? AND b.branch_id = ? AND a.agent_id = ? AND a.status = 'active'`,
		session.InstanceID, session.BranchID, session.ControlledEntityID,
	).Scan(&head, &worldTime, &currentDay, &actualPlace, &positionVersion)
	if err != nil {
		return RPMoveResult{}, classifyMissing(err, "RP mover state")
	}
	if request.ExpectedCursor != head || session.ObservationCursor != head {
		return RPMoveResult{}, core.NewError(core.CodeBranchConflict, "RP move requires a current observation cursor")
	}
	if request.FromPlaceID != actualPlace {
		return RPMoveResult{}, core.NewError(core.CodeBranchConflict, "RP mover is no longer at the observed origin")
	}
	var reachable int
	if err := tx.conn.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM rp_place_links link
		JOIN agent_places source ON source.place_id = link.from_place_id
		JOIN agent_places destination ON destination.place_id = link.to_place_id
		WHERE link.instance_id = ? AND link.branch_id = ?
		  AND link.from_place_id = ? AND link.to_place_id = ?
		  AND source.status = 'active' AND destination.status = 'active'
		  AND source.instance_id = link.instance_id AND source.branch_id = link.branch_id
		  AND destination.instance_id = link.instance_id AND destination.branch_id = link.branch_id`,
		session.InstanceID, session.BranchID, actualPlace, request.ToPlaceID,
	).Scan(&reachable); err != nil {
		return RPMoveResult{}, core.WrapError(core.CodeStorageFailure, "check RP reachability", err)
	}
	if reachable != 1 {
		return RPMoveResult{}, core.NewError(core.CodeInvalidArgument, "destination is not reachable from the current place")
	}
	if err := ensureCohortTransitionChronology(ctx, tx.conn, session.InstanceID, session.BranchID, worldTime); err != nil {
		return RPMoveResult{}, err
	}
	coLocated, err := rpCoLocatedEntityIDs(ctx, tx.conn, session.InstanceID, session.BranchID, request.ToPlaceID, session.ControlledEntityID)
	if err != nil {
		return RPMoveResult{}, err
	}
	sequence := head + 1
	var epochID string
	if err := tx.conn.QueryRowContext(ctx, `SELECT epoch_id FROM rule_epochs WHERE instance_id = ? AND branch_id = ? AND start_sequence <= ? AND (end_sequence IS NULL OR ? < end_sequence)`, session.InstanceID, session.BranchID, sequence, sequence).Scan(&epochID); err != nil {
		return RPMoveResult{}, classifyMissing(err, "RP move Rule Epoch")
	}
	keyHash, err := core.HashJSON(struct {
		SessionID string `json:"session_id"`
		Key       string `json:"key"`
	}{session.SessionID, request.IdempotencyKey})
	if err != nil {
		return RPMoveResult{}, err
	}
	suffix := keyHash[7:]
	commandID := "cmd_rp_move_" + suffix
	attemptID := "attempt_rp_move_" + suffix
	batchID := "batch_rp_move_" + suffix
	eventID := "event_rp_move_" + suffix
	scheduleID := "schedule_rp_move_" + suffix
	itemID := "sched_rp_move_" + suffix
	payload := rpMoveEvent{SessionID: session.SessionID, EntityID: session.ControlledEntityID, FromPlaceID: actualPlace, ToPlaceID: request.ToPlaceID, CoLocatedEntityIDs: coLocated}
	payloadJSON, err := core.CanonicalJSON(payload)
	if err != nil {
		return RPMoveResult{}, err
	}
	batchHash, err := core.HashJSON(struct {
		CommandID string      `json:"command_id"`
		Sequence  int64       `json:"sequence"`
		WorldTime string      `json:"world_time"`
		Payload   rpMoveEvent `json:"payload"`
	}{commandID, sequence, worldTime, payload})
	if err != nil {
		return RPMoveResult{}, err
	}
	itemPayloadJSON, err := core.CanonicalJSON(agentSchedulePayload{Kind: "agent_move", Day: int(currentDay), AgentID: session.ControlledEntityID, ScheduleID: scheduleID, ToPlaceID: request.ToPlaceID, ActivityCode: "rp_move"})
	if err != nil {
		return RPMoveResult{}, err
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	statements := []struct {
		name, query string
		args        []any
	}{
		{"RP move command", `INSERT INTO commands(command_id, instance_id, branch_id, command_type, idempotency_key, request_hash, expected_head, principal_id, command_policy, status, created_at_utc) VALUES (?, ?, ?, 'RPPlayerMove', ?, ?, ?, ?, '{"authorization":"rp-session-control"}', 'pending', ?)`, []any{commandID, session.InstanceID, session.BranchID, commandKey, requestHash, head, request.PrincipalID, now}},
		{"RP move attempt", `INSERT INTO command_attempts(command_id, attempt_no, attempt_id, status, lease_owner, lease_until_utc, proposal_hash, created_at_utc) VALUES (?, 1, ?, 'ready', 'corerp-rp1', ?, ?, ?)`, []any{commandID, attemptID, s.now().UTC().Add(30 * time.Second).Format(time.RFC3339Nano), requestHash, now}},
		{"RP move batch", `INSERT INTO event_batches(batch_id, command_id, attempt_no, instance_id, branch_id, epoch_id, expected_head, first_sequence, last_sequence, event_count, world_time, batch_hash, committed_at_utc) VALUES (?, ?, 1, ?, ?, ?, ?, ?, ?, 1, ?, ?, ?)`, []any{batchID, commandID, session.InstanceID, session.BranchID, epochID, head, sequence, sequence, worldTime, batchHash, now}},
		{"RP move event", `INSERT INTO events(event_id, batch_id, instance_id, branch_id, event_sequence, batch_index, event_type, actor_id, world_time, payload) VALUES (?, ?, ?, ?, ?, 0, 'RPPlayerMoved', ?, ?, ?)`, []any{eventID, batchID, session.InstanceID, session.BranchID, sequence, session.ControlledEntityID, worldTime, string(payloadJSON)}},
		{"RP move completed scheduler item", `INSERT INTO scheduler_items(scheduler_item_id, instance_id, branch_id, world_time, phase_id, declared_priority, status, payload) VALUES (?, ?, ?, ?, ?, 0, 'completed', ?)`, []any{itemID, session.InstanceID, session.BranchID, worldTime, m2AgentPhaseID, string(itemPayloadJSON)}},
		{"RP move completed schedule", `INSERT INTO agent_schedule_entries(schedule_id, agent_id, world_time, place_id, activity_code, declared_priority, scheduler_item_id, status, definition_event_id) VALUES (?, ?, ?, ?, 'rp_move', 0, ?, 'completed', ?)`, []any{scheduleID, session.ControlledEntityID, worldTime, request.ToPlaceID, itemID, eventID}},
		{"RP move fact", `INSERT INTO agent_movements(movement_id, event_id, agent_id, from_place_id, to_place_id, schedule_id, activity_code, world_time, movement_kind) VALUES (?, ?, ?, ?, ?, ?, 'rp_move', ?, 'scheduled')`, []any{"movement_rp_move_" + suffix, eventID, session.ControlledEntityID, actualPlace, request.ToPlaceID, scheduleID, worldTime}},
	}
	for _, statement := range statements {
		if err := execAgentOne(ctx, tx.conn, statement.name, statement.query, statement.args...); err != nil {
			return RPMoveResult{}, err
		}
	}
	if err := execAgentOne(ctx, tx.conn, "update RP mover position", `UPDATE agent_positions SET place_id = ?, activity_code = 'rp_move', effective_world_time = ?, projection_version = projection_version + 1, last_event_sequence = ? WHERE agent_id = ? AND projection_version = ?`, request.ToPlaceID, worldTime, sequence, session.ControlledEntityID, positionVersion); err != nil {
		return RPMoveResult{}, err
	}
	for _, otherID := range coLocated {
		for _, pair := range [][2]string{{session.ControlledEntityID, otherID}, {otherID, session.ControlledEntityID}} {
			if err := upsertCoLocationKnowledge(ctx, tx.conn, eventID, sequence, pair[0], pair[1], request.ToPlaceID, worldTime); err != nil {
				return RPMoveResult{}, err
			}
		}
	}
	if err := execAgentOne(ctx, tx.conn, "advance RP move clock lineage", `UPDATE world_clocks SET projection_version = projection_version + 1, last_event_sequence = ? WHERE instance_id = ? AND branch_id = ? AND current_world_time = ?`, sequence, session.InstanceID, session.BranchID, worldTime); err != nil {
		return RPMoveResult{}, err
	}
	if err := execAgentOne(ctx, tx.conn, "advance RP move branch", `UPDATE branches SET head_sequence = ? WHERE instance_id = ? AND branch_id = ? AND head_sequence = ?`, sequence, session.InstanceID, session.BranchID, head); err != nil {
		return RPMoveResult{}, err
	}
	if err := s.insertAgentAudit(ctx, tx.conn, "audit_"+commandID, "agent_decision", eventID, commandID, attemptID, worldTime, now, payloadJSON); err != nil {
		return RPMoveResult{}, err
	}
	if err := insertAgentOutbox(ctx, tx.conn, "outbox_"+commandID, eventID, "rp.player.moved", payloadJSON); err != nil {
		return RPMoveResult{}, err
	}
	if err := execAgentOne(ctx, tx.conn, "commit RP move attempt", `UPDATE command_attempts SET status = 'committed', finished_at_utc = ? WHERE command_id = ? AND attempt_no = 1 AND status = 'ready'`, now, commandID); err != nil {
		return RPMoveResult{}, err
	}
	if err := execAgentOne(ctx, tx.conn, "commit RP move command", `UPDATE commands SET status = 'committed' WHERE command_id = ? AND status = 'pending'`, commandID); err != nil {
		return RPMoveResult{}, err
	}
	if s.beforeCommit != nil {
		if err := s.beforeCommit(); err != nil {
			return RPMoveResult{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return RPMoveResult{}, core.WrapError(core.CodeStorageFailure, "commit RP move", err)
	}
	return RPMoveResult{CommandID: commandID, EventID: eventID, EventSequence: sequence, WorldTime: worldTime, FromPlaceID: actualPlace, ToPlaceID: request.ToPlaceID}, nil
}

func rpCoLocatedEntityIDs(ctx context.Context, conn *sql.Conn, instanceID, branchID, placeID, excludeID string) ([]string, error) {
	rows, err := conn.QueryContext(ctx, `
		SELECT a.agent_id FROM agent_profiles a JOIN agent_positions p ON p.agent_id = a.agent_id
		JOIN materialized_entities e ON e.entity_id = a.agent_id
		WHERE a.instance_id = ? AND a.branch_id = ? AND a.status = 'active'
		  AND e.status = 'active' AND p.place_id = ? AND a.agent_id <> ? ORDER BY a.agent_id`,
		instanceID, branchID, placeID, excludeID)
	if err != nil {
		return nil, core.WrapError(core.CodeStorageFailure, "read RP move destination presence", err)
	}
	defer rows.Close()
	result := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, core.WrapError(core.CodeStorageFailure, "scan RP move destination presence", err)
		}
		result = append(result, id)
	}
	if err := rows.Err(); err != nil {
		return nil, core.WrapError(core.CodeStorageFailure, "iterate RP move destination presence", err)
	}
	return result, nil
}

func loadRPMoveResult(ctx context.Context, conn *sql.Conn, commandID string, replayed bool) (RPMoveResult, error) {
	var result RPMoveResult
	var payloadJSON string
	err := conn.QueryRowContext(ctx, `
		SELECT e.event_id, e.event_sequence, e.world_time, e.payload
		FROM events e JOIN event_batches b ON b.batch_id = e.batch_id
		WHERE b.command_id = ? AND e.event_type = 'RPPlayerMoved'`, commandID,
	).Scan(&result.EventID, &result.EventSequence, &result.WorldTime, &payloadJSON)
	if err != nil {
		return RPMoveResult{}, classifyMissing(err, "committed RP move event")
	}
	var payload rpMoveEvent
	if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
		return RPMoveResult{}, core.WrapError(core.CodeStorageFailure, "decode committed RP move", err)
	}
	result.CommandID = commandID
	result.FromPlaceID = payload.FromPlaceID
	result.ToPlaceID = payload.ToPlaceID
	result.Replayed = replayed
	return result, nil
}
