package storage

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"corerp.local/backend/internal/core"
)

// RPSession contains application cursors, never a copy of world state.
type RPSession struct {
	SessionID          string `json:"session_id"`
	InstanceID         string `json:"instance_id"`
	BranchID           string `json:"branch_id"`
	ControlledEntityID string `json:"controlled_entity_id"`
	POV                string `json:"pov"`
	ObservationCursor  int64  `json:"observation_cursor"`
	TurnCursor         string `json:"turn_cursor"`
	TurnState          string `json:"turn_state"`
	Status             string `json:"status"`
	CreatedAtUTC       string `json:"created_at_utc"`
	ResumedAtUTC       string `json:"resumed_at_utc"`
	Replayed           bool   `json:"replayed,omitempty"`
}

type RPVisibleEntity struct {
	EntityID    string `json:"entity_id"`
	DisplayName string `json:"display_name"`
}

type RPObservation struct {
	Environment       *core.RPLocalEnvironment   `json:"environment,omitempty"`
	Stores            []core.RPStoreAvailability `json:"stores,omitempty"`
	TransitWorks      []core.RPLocalTransitWorks `json:"transit_works,omitempty"`
	DecisionMode      string                     `json:"decision_mode"`
	SessionID         string                     `json:"session_id"`
	ControlledEntity  RPVisibleEntity            `json:"controlled_entity"`
	WorldTime         string                     `json:"world_time"`
	PlaceID           string                     `json:"place_id"`
	PlaceName         string                     `json:"place_name"`
	PlaceKind         string                     `json:"place_kind"`
	PresentEntities   []RPVisibleEntity          `json:"present_entities"`
	ObservationCursor int64                      `json:"observation_cursor"`
	ReachablePlaces   []RPVisiblePlace           `json:"reachable_places"`
	RecentTurns       []RPHistoryTurn            `json:"recent_turns"`
}

type RPVisiblePlace struct {
	PlaceID     string `json:"place_id"`
	DisplayName string `json:"display_name"`
}

type RPHistoryTurn struct {
	TurnRunID      string   `json:"turn_run_id"`
	NarrativeLines []string `json:"narrative_lines"`
}

func (s *Store) OpenRPSession(ctx context.Context, request core.RPSessionOpenRequest) (RPSession, error) {
	if err := request.Validate(); err != nil {
		return RPSession{}, err
	}
	requestHash, err := core.HashJSON(request)
	if err != nil {
		return RPSession{}, err
	}
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return RPSession{}, core.WrapError(core.CodeStorageFailure, "begin RP session open", err)
	}
	defer tx.Rollback(ctx)
	var existingID, existingHash string
	err = tx.conn.QueryRowContext(ctx, `SELECT session_id, request_hash FROM rp_sessions WHERE principal_id = ? AND idempotency_key = ?`, request.PrincipalID, request.IdempotencyKey).Scan(&existingID, &existingHash)
	if err == nil && existingHash != requestHash {
		return RPSession{}, core.NewError(core.CodeIdempotencyMismatch, "RP session idempotency key was used with another binding")
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return RPSession{}, core.WrapError(core.CodeStorageFailure, "look up RP session idempotency key", err)
	}
	if err := authorizeRPControl(ctx, tx.conn, request.PrincipalID, request.InstanceID, request.BranchID, request.EntityID); err != nil {
		return RPSession{}, err
	}
	if err := validateRPBinding(ctx, tx.conn, request.InstanceID, request.BranchID, request.EntityID); err != nil {
		return RPSession{}, err
	}
	if err == nil {
		session, err := loadRPSession(ctx, tx.conn, request.PrincipalID, existingID)
		if err != nil {
			return RPSession{}, err
		}
		session.Replayed = true
		return session, nil
	}
	id, err := newRPSessionID()
	if err != nil {
		return RPSession{}, err
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	_, err = tx.conn.ExecContext(ctx, `
		INSERT INTO rp_sessions(session_id, principal_id, instance_id, branch_id, controlled_entity_id, pov, idempotency_key, request_hash, created_at_utc, resumed_at_utc)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, id, request.PrincipalID, request.InstanceID, request.BranchID, request.EntityID, request.POV, request.IdempotencyKey, requestHash, now, now)
	if err != nil {
		return RPSession{}, core.WrapError(core.CodeStorageFailure, "insert RP session", err)
	}
	if s.beforeCommit != nil {
		if err := s.beforeCommit(); err != nil {
			return RPSession{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return RPSession{}, core.WrapError(core.CodeStorageFailure, "commit RP session open", err)
	}
	return RPSession{SessionID: id, InstanceID: request.InstanceID, BranchID: request.BranchID, ControlledEntityID: request.EntityID, POV: request.POV, TurnState: "idle", Status: "active", CreatedAtUTC: now, ResumedAtUTC: now}, nil
}

func (s *Store) ReadRPSession(ctx context.Context, request core.RPSessionReadRequest) (RPSession, error) {
	if err := request.Validate(); err != nil {
		return RPSession{}, err
	}
	session, err := loadRPSession(ctx, s.db, request.PrincipalID, request.SessionID)
	if err != nil {
		return RPSession{}, err
	}
	if err := authorizeRPControl(ctx, s.db, request.PrincipalID, session.InstanceID, session.BranchID, session.ControlledEntityID); err != nil {
		return RPSession{}, err
	}
	if err := validateRPBinding(ctx, s.db, session.InstanceID, session.BranchID, session.ControlledEntityID); err != nil {
		return RPSession{}, err
	}
	return session, nil
}

func (s *Store) ResumeRPSession(ctx context.Context, request core.RPSessionReadRequest) (RPSession, error) {
	if err := request.Validate(); err != nil {
		return RPSession{}, err
	}
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return RPSession{}, core.WrapError(core.CodeStorageFailure, "begin RP session resume", err)
	}
	defer tx.Rollback(ctx)
	session, err := loadRPSession(ctx, tx.conn, request.PrincipalID, request.SessionID)
	if err != nil {
		return RPSession{}, err
	}
	if err := authorizeRPControl(ctx, tx.conn, request.PrincipalID, session.InstanceID, session.BranchID, session.ControlledEntityID); err != nil {
		return RPSession{}, err
	}
	if err := validateRPBinding(ctx, tx.conn, session.InstanceID, session.BranchID, session.ControlledEntityID); err != nil {
		return RPSession{}, err
	}
	if session.Status != "active" {
		return RPSession{}, core.NewError(core.CodeBranchConflict, "RP session is closed")
	}
	session.ResumedAtUTC = s.now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.conn.ExecContext(ctx, `UPDATE rp_sessions SET resumed_at_utc = ? WHERE session_id = ? AND status = 'active'`, session.ResumedAtUTC, session.SessionID); err != nil {
		return RPSession{}, core.WrapError(core.CodeStorageFailure, "resume RP session", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return RPSession{}, core.WrapError(core.CodeStorageFailure, "commit RP session resume", err)
	}
	return session, nil
}

func (s *Store) CloseRPSession(ctx context.Context, request core.RPSessionReadRequest) (RPSession, error) {
	if err := request.Validate(); err != nil {
		return RPSession{}, err
	}
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return RPSession{}, core.WrapError(core.CodeStorageFailure, "begin RP session close", err)
	}
	defer tx.Rollback(ctx)
	session, err := loadRPSession(ctx, tx.conn, request.PrincipalID, request.SessionID)
	if err != nil {
		return RPSession{}, err
	}
	if err := authorizeRPControl(ctx, tx.conn, request.PrincipalID, session.InstanceID, session.BranchID, session.ControlledEntityID); err != nil {
		return RPSession{}, err
	}
	if session.Status == "closed" {
		return session, nil
	}
	var pendingWaits int
	if err := tx.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_wait_intents WHERE session_id = ? AND status = 'pending'`, session.SessionID).Scan(&pendingWaits); err != nil {
		return RPSession{}, core.WrapError(core.CodeStorageFailure, "check pending RP wait before close", err)
	}
	if pendingWaits != 0 {
		return RPSession{}, core.NewError(core.CodeCommandInProgress, "RP wait must complete before closing the session")
	}
	var pendingTurns int
	if err := tx.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_turn_runs WHERE session_id = ? AND status <> 'settled'`, session.SessionID).Scan(&pendingTurns); err != nil {
		return RPSession{}, core.WrapError(core.CodeStorageFailure, "check pending RP turn before close", err)
	}
	if pendingTurns != 0 {
		return RPSession{}, core.NewError(core.CodeCommandInProgress, "RP turn must settle before closing the session")
	}
	if _, err := tx.conn.ExecContext(ctx, `UPDATE rp_sessions SET status = 'closed' WHERE session_id = ? AND status = 'active'`, session.SessionID); err != nil {
		return RPSession{}, core.WrapError(core.CodeStorageFailure, "close RP session", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return RPSession{}, core.WrapError(core.CodeStorageFailure, "commit RP session close", err)
	}
	session.Status = "closed"
	return session, nil
}

func (s *Store) ObserveRPSession(ctx context.Context, request core.RPSessionReadRequest) (RPObservation, error) {
	if err := request.Validate(); err != nil {
		return RPObservation{}, err
	}
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return RPObservation{}, core.WrapError(core.CodeStorageFailure, "begin RP observation", err)
	}
	defer tx.Rollback(ctx)
	session, err := loadRPSession(ctx, tx.conn, request.PrincipalID, request.SessionID)
	if err != nil {
		return RPObservation{}, err
	}
	if err := authorizeRPControl(ctx, tx.conn, request.PrincipalID, session.InstanceID, session.BranchID, session.ControlledEntityID); err != nil {
		return RPObservation{}, err
	}
	if session.Status != "active" {
		return RPObservation{}, core.NewError(core.CodeBranchConflict, "RP session is closed")
	}
	var view RPObservation
	view.DecisionMode = "deterministic"
	view.SessionID = session.SessionID
	view.ControlledEntity.EntityID = session.ControlledEntityID
	err = tx.conn.QueryRowContext(ctx, `
		SELECT e.display_name, p.place_id, l.display_name, l.place_kind, c.current_world_time, b.head_sequence
		FROM materialized_entities e
		JOIN cohorts source ON source.cohort_id = e.source_cohort_id
		JOIN agent_profiles a ON a.agent_id = e.entity_id
		JOIN agent_positions p ON p.agent_id = a.agent_id
		JOIN agent_places l ON l.place_id = p.place_id
		JOIN world_clocks c ON c.instance_id = a.instance_id AND c.branch_id = a.branch_id
		JOIN branches b ON b.instance_id = a.instance_id AND b.branch_id = a.branch_id
		WHERE e.entity_id = ? AND e.status = 'active' AND e.population_count = 1 AND a.status = 'active'
		  AND a.instance_id = ? AND a.branch_id = ? AND source.instance_id = a.instance_id AND source.branch_id = a.branch_id`,
		session.ControlledEntityID, session.InstanceID, session.BranchID,
	).Scan(&view.ControlledEntity.DisplayName, &view.PlaceID, &view.PlaceName, &view.PlaceKind, &view.WorldTime, &view.ObservationCursor)
	if err != nil {
		return RPObservation{}, classifyMissing(err, "RP observer world state")
	}
	rows, err := tx.conn.QueryContext(ctx, `
		SELECT a.agent_id, e.display_name
		FROM agent_profiles a JOIN agent_positions p ON p.agent_id = a.agent_id
		JOIN materialized_entities e ON e.entity_id = a.agent_id
		JOIN cohorts source ON source.cohort_id = e.source_cohort_id
		WHERE a.instance_id = ? AND a.branch_id = ? AND a.status = 'active'
		  AND source.instance_id = a.instance_id AND source.branch_id = a.branch_id
		  AND e.status = 'active' AND p.place_id = ? AND a.agent_id <> ?
		ORDER BY a.agent_id`, session.InstanceID, session.BranchID, view.PlaceID, session.ControlledEntityID)
	if err != nil {
		return RPObservation{}, core.WrapError(core.CodeStorageFailure, "read RP presence", err)
	}
	view.PresentEntities = make([]RPVisibleEntity, 0)
	for rows.Next() {
		var entity RPVisibleEntity
		if err := rows.Scan(&entity.EntityID, &entity.DisplayName); err != nil {
			rows.Close()
			return RPObservation{}, core.WrapError(core.CodeStorageFailure, "scan RP presence", err)
		}
		view.PresentEntities = append(view.PresentEntities, entity)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return RPObservation{}, core.WrapError(core.CodeStorageFailure, "iterate RP presence", err)
	}
	if err := rows.Close(); err != nil {
		return RPObservation{}, core.WrapError(core.CodeStorageFailure, "close RP presence cursor", err)
	}
	view.ReachablePlaces = make([]RPVisiblePlace, 0)
	rows, err = tx.conn.QueryContext(ctx, `SELECT p.place_id, p.display_name FROM rp_place_links l
		JOIN agent_places p ON p.place_id = l.to_place_id
		WHERE l.instance_id = ? AND l.branch_id = ? AND l.from_place_id = ?
		AND p.instance_id = l.instance_id AND p.branch_id = l.branch_id AND p.status = 'active'
		ORDER BY p.place_id`, session.InstanceID, session.BranchID, view.PlaceID)
	if err != nil {
		return RPObservation{}, err
	}
	for rows.Next() {
		var place RPVisiblePlace
		if err := rows.Scan(&place.PlaceID, &place.DisplayName); err != nil {
			rows.Close()
			return RPObservation{}, err
		}
		view.ReachablePlaces = append(view.ReachablePlaces, place)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return RPObservation{}, err
	}
	rows.Close()
	// Only this authenticated session's settled view, never raw events or NPC private state.
	view.RecentTurns = make([]RPHistoryTurn, 0)
	rows, err = tx.conn.QueryContext(ctx, `SELECT turn_run_id, narrative_json FROM
		(SELECT turn_run_id, narrative_json, settled_sequence FROM rp_turn_runs
		 WHERE session_id = ? AND status = 'settled'
		 UNION ALL
		 SELECT e.event_id, json_array(CASE e.event_type
		 WHEN 'RPPlayerMoved' THEN '你前往了 ' || p.display_name || '。'
		 WHEN 'RPInterpersonalAction' THEN json_extract(e.payload,'$.description')
		 ELSE '你等待至 ' || json_extract(e.payload, '$.target_world_time') || '。' END), e.event_sequence
		 FROM events e LEFT JOIN agent_places p ON p.place_id = json_extract(e.payload, '$.to_place_id')
		 WHERE e.instance_id = ? AND e.branch_id = ? AND e.actor_id = ?
		 AND e.event_type IN ('RPPlayerMoved', 'RPWaitCompleted', 'RPInterpersonalAction') AND json_extract(e.payload, '$.session_id') = ?
		 UNION ALL
		 SELECT e.event_id,json_array(CASE e.event_type
		 WHEN 'RPSpeechAccepted' THEN n.display_name || '说：“' || u.speech_text || '”'
		 ELSE n.display_name || '前往了 ' || p.display_name || '。' END),e.event_sequence
		 FROM events e JOIN event_batches b ON b.batch_id=e.batch_id JOIN commands c ON c.command_id=b.command_id
		 JOIN materialized_entities n ON n.entity_id=e.actor_id
		 LEFT JOIN rp_utterances u ON u.event_id=e.event_id
		 LEFT JOIN agent_places p ON p.place_id=json_extract(e.payload,'$.to_place_id')
		 WHERE c.command_type='RPNPCInitiative' AND e.instance_id=? AND e.branch_id=? AND json_extract(e.payload,'$.session_id')=?
		 AND (e.event_type='RPNPCMoved' OR (e.event_type='RPSpeechAccepted' AND EXISTS (SELECT 1 FROM observation_records o WHERE o.source_event_id=e.event_id AND o.observer_agent_id=?)))
		 ORDER BY settled_sequence DESC LIMIT 50)
		ORDER BY settled_sequence`, session.SessionID, session.InstanceID, session.BranchID, session.ControlledEntityID, session.SessionID, session.InstanceID, session.BranchID, session.SessionID, session.ControlledEntityID)
	if err != nil {
		return RPObservation{}, err
	}
	for rows.Next() {
		var turn RPHistoryTurn
		var raw string
		if err := rows.Scan(&turn.TurnRunID, &raw); err != nil {
			rows.Close()
			return RPObservation{}, err
		}
		if err := json.Unmarshal([]byte(raw), &turn.NarrativeLines); err != nil {
			rows.Close()
			return RPObservation{}, err
		}
		view.RecentTurns = append(view.RecentTurns, turn)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return RPObservation{}, err
	}
	rows.Close()
	view.Environment, err = readRPLocalEnvironment(ctx, tx.conn, session.InstanceID, session.BranchID, view.PlaceID, view.WorldTime)
	if err != nil {
		return RPObservation{}, err
	}
	view.Stores, err = readRPLocalStores(ctx, tx.conn, session.InstanceID, session.BranchID, view.PlaceID)
	if err != nil {
		return RPObservation{}, err
	}
	view.TransitWorks, err = readRPLocalTransitWorks(ctx, tx.conn, session.InstanceID, session.BranchID, view.PlaceID, view.WorldTime)
	if err != nil {
		return RPObservation{}, err
	}
	if _, err := tx.conn.ExecContext(ctx, `UPDATE rp_sessions SET observation_cursor = ? WHERE session_id = ?`, view.ObservationCursor, session.SessionID); err != nil {
		return RPObservation{}, core.WrapError(core.CodeStorageFailure, "advance RP observation cursor", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return RPObservation{}, core.WrapError(core.CodeStorageFailure, "commit RP observation cursor", err)
	}
	return view, nil
}

type rpQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func loadRPSession(ctx context.Context, q rpQueryer, principalID, sessionID string) (RPSession, error) {
	var session RPSession
	err := q.QueryRowContext(ctx, `
		SELECT session_id, instance_id, branch_id, controlled_entity_id, pov, observation_cursor,
		       turn_cursor, turn_state, status, created_at_utc, resumed_at_utc
		FROM rp_sessions WHERE session_id = ? AND principal_id = ?`, sessionID, principalID,
	).Scan(&session.SessionID, &session.InstanceID, &session.BranchID, &session.ControlledEntityID,
		&session.POV, &session.ObservationCursor, &session.TurnCursor, &session.TurnState,
		&session.Status, &session.CreatedAtUTC, &session.ResumedAtUTC)
	if err != nil {
		return RPSession{}, classifyMissing(err, "RP session")
	}
	return session, nil
}

func authorizeRPControl(ctx context.Context, q rpQueryer, principalID, instanceID, branchID, entityID string) error {
	var count int
	err := q.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM capability_grants g JOIN principals p ON p.principal_id = g.principal_id
		WHERE g.principal_id = ? AND p.principal_type = 'player' AND p.status = 'active'
		  AND g.capability_id = 'world.rp.control' AND g.instance_id = ? AND g.branch_id = ?
		  AND g.subject_id = ? AND g.status = 'active'`, principalID, instanceID, branchID, entityID).Scan(&count)
	if err != nil {
		return core.WrapError(core.CodeStorageFailure, "check RP control grant", err)
	}
	if count != 1 {
		return core.NewError(core.CodeUnauthorized, "player lacks control of this entity in this world branch")
	}
	return nil
}

func validateRPBinding(ctx context.Context, q rpQueryer, instanceID, branchID, entityID string) error {
	var count int
	err := q.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM materialized_entities e
		JOIN cohorts source ON source.cohort_id = e.source_cohort_id
		JOIN agent_profiles a ON a.agent_id = e.entity_id
		JOIN agent_positions p ON p.agent_id = a.agent_id
		WHERE e.entity_id = ? AND e.status = 'active' AND e.population_count = 1
		  AND source.instance_id = ? AND source.branch_id = ?
		  AND a.instance_id = source.instance_id AND a.branch_id = source.branch_id AND a.status = 'active'`,
		entityID, instanceID, branchID).Scan(&count)
	if err != nil {
		return core.WrapError(core.CodeStorageFailure, "validate RP entity binding", err)
	}
	if count != 1 {
		return core.NewError(core.CodeNotFound, "active spatial entity not found in this world branch")
	}
	return nil
}

func newRPSessionID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", core.WrapError(core.CodeStorageFailure, "create RP session ID", err)
	}
	return "rps_" + hex.EncodeToString(value[:]), nil
}
