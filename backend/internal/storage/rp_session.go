package storage

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"corerp.local/backend/internal/core"
)

// RPSession contains application cursors, never a copy of world state.
type RPSession struct {
	SessionID            string `json:"session_id"`
	InstanceID           string `json:"instance_id"`
	BranchID             string `json:"branch_id"`
	ControlledEntityID   string `json:"controlled_entity_id"`
	ControlGeneration    int64  `json:"control_generation"`
	ControllerInstanceID string `json:"controller_instance_id,omitempty"`
	POV                  string `json:"pov"`
	ObservationCursor    int64  `json:"observation_cursor"`
	ChapterStartSequence int64  `json:"chapter_start_sequence"`
	TurnCursor           string `json:"turn_cursor"`
	TurnState            string `json:"turn_state"`
	Status               string `json:"status"`
	CreatedAtUTC         string `json:"created_at_utc"`
	ResumedAtUTC         string `json:"resumed_at_utc"`
	Replayed             bool   `json:"replayed,omitempty"`
}

type RPVisibleEntity struct {
	EntityID    string `json:"entity_id"`
	DisplayName string `json:"display_name"`
	Identified  bool   `json:"identified,omitempty"`
}

type RPObservation struct {
	Environment       *core.RPLocalEnvironment   `json:"environment,omitempty"`
	Stores            []core.RPStoreAvailability `json:"stores,omitempty"`
	TransitWorks      []core.RPLocalTransitWorks `json:"transit_works,omitempty"`
	DecisionMode      string                     `json:"decision_mode"`
	NarrativeMode     string                     `json:"narrative_mode"`
	SessionID         string                     `json:"session_id"`
	ControlledEntity  RPVisibleEntity            `json:"controlled_entity"`
	WorldTime         string                     `json:"world_time"`
	PlaceID           string                     `json:"place_id"`
	PlaceName         string                     `json:"place_name"`
	PlaceKind         string                     `json:"place_kind"`
	PresentEntities   []RPVisibleEntity          `json:"present_entities"`
	ObservationCursor int64                      `json:"observation_cursor"`
	ReachablePlaces   []RPVisiblePlace           `json:"reachable_places"`
	SceneObjects      []RPVisibleSceneObject     `json:"scene_objects"`
	ActiveJourney     *RPJourneyView             `json:"active_journey,omitempty"`
	RecentTurns       []RPHistoryTurn            `json:"recent_turns"`
}

type RPJourneyView struct {
	JourneyID          string `json:"journey_id"`
	FromPlaceID        string `json:"from_place_id"`
	ToPlaceID          string `json:"to_place_id"`
	SegmentPlaceID     string `json:"segment_place_id"`
	ScheduledArrivalAt string `json:"scheduled_arrival_at"`
}

type RPVisiblePlace struct {
	PlaceID         string `json:"place_id"`
	DisplayName     string `json:"display_name"`
	CanMoveNow      bool   `json:"can_move_now"`
	CanStartJourney bool   `json:"can_start_journey,omitempty"`
	TravelMinutes   int    `json:"travel_minutes,omitempty"`
}

type RPHistoryTurn struct {
	TurnRunID      string           `json:"turn_run_id"`
	NarrativeLines []string         `json:"narrative_lines"`
	RenderID       string           `json:"render_id,omitempty"`
	ProviderCalls  []RPProviderCall `json:"provider_calls,omitempty"`
	CanRegenerate  bool             `json:"can_regenerate"`
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
	if err := checkRPRequestRetirement(ctx, tx.conn, request.PrincipalID, "open", "", request.IdempotencyKey); err != nil {
		return RPSession{}, err
	}
	err = tx.conn.QueryRowContext(ctx, `SELECT session_id, request_hash FROM rp_sessions WHERE principal_id = ? AND idempotency_key = ?`, request.PrincipalID, request.IdempotencyKey).Scan(&existingID, &existingHash)
	if err == nil && existingHash != requestHash {
		return RPSession{}, core.NewError(core.CodeIdempotencyMismatch, "RP session idempotency key was used with another binding")
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return RPSession{}, core.WrapError(core.CodeStorageFailure, "look up RP session idempotency key", err)
	}
	existing := err == nil
	if existing {
		session, err := loadRPSessionRecord(ctx, tx.conn, request.PrincipalID, existingID)
		if err != nil {
			return RPSession{}, err
		}
		session.Replayed = true
		return session, nil
	}
	if err := s.requireNoRunningRPBackgroundProgression(ctx, tx.conn, request.InstanceID, request.BranchID); err != nil {
		return RPSession{}, err
	}
	if err := authorizeRPControl(ctx, tx.conn, request.PrincipalID, request.InstanceID, request.BranchID, request.EntityID); err != nil {
		return RPSession{}, err
	}
	if err := validateRPBinding(ctx, tx.conn, request.InstanceID, request.BranchID, request.EntityID); err != nil {
		return RPSession{}, err
	}
	generation, controller, err := rpControlGeneration(ctx, tx.conn, request.InstanceID, request.BranchID, request.EntityID)
	if err != nil {
		return RPSession{}, err
	}
	id, err := newRPSessionID()
	if err != nil {
		return RPSession{}, err
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	_, err = tx.conn.ExecContext(ctx, `
		INSERT INTO rp_sessions(session_id, principal_id, instance_id, branch_id, controlled_entity_id, pov, idempotency_key, request_hash, created_at_utc, resumed_at_utc, control_generation, controller_instance_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, id, request.PrincipalID, request.InstanceID, request.BranchID, request.EntityID, request.POV, request.IdempotencyKey, requestHash, now, now, generation, controller)
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
	return RPSession{SessionID: id, InstanceID: request.InstanceID, BranchID: request.BranchID, ControlledEntityID: request.EntityID, ControlGeneration: generation, ControllerInstanceID: controller, POV: request.POV, TurnState: "idle", Status: "active", CreatedAtUTC: now, ResumedAtUTC: now}, nil
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
	var activeInteractions int
	if err := tx.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_interactions WHERE session_id=? AND status IN ('open','paused')`, session.SessionID).Scan(&activeInteractions); err != nil {
		return RPSession{}, core.WrapError(core.CodeStorageFailure, "check active RP interaction before close", err)
	}
	if activeInteractions != 0 {
		return RPSession{}, core.NewError(core.CodeCommandInProgress, "interaction must settle or stop before closing the session")
	}
	var activeSharedRounds int
	if err := tx.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_shared_round_participants p JOIN rp_shared_rounds r ON r.round_id=p.round_id WHERE p.session_id=? AND r.status IN ('open','advancing')`, session.SessionID).Scan(&activeSharedRounds); err != nil {
		return RPSession{}, core.WrapError(core.CodeStorageFailure, "check active shared round before close", err)
	}
	if activeSharedRounds != 0 {
		return RPSession{}, core.NewError(core.CodeCommandInProgress, "shared round must settle before closing the session")
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
	view.NarrativeMode = "deterministic"
	view.SessionID = session.SessionID
	view.ControlledEntity.EntityID = session.ControlledEntityID
	view.ControlledEntity.Identified = true
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
	visible := make([]RPVisibleEntity, 0, len(view.PresentEntities))
	for _, entity := range view.PresentEntities {
		canSee, err := rpCanPerceive(ctx, tx.conn, session.InstanceID, session.BranchID, session.ControlledEntityID, entity.EntityID, "visual", "")
		if err != nil {
			return RPObservation{}, err
		}
		if canSee {
			known, err := rpIdentityKnown(ctx, tx.conn, session.InstanceID, session.BranchID, session.ControlledEntityID, entity.EntityID)
			if err != nil {
				return RPObservation{}, err
			}
			entity.Identified = known
			if !known {
				entity.DisplayName = "陌生人"
				entity.EntityID, err = rpAnonymousEntityID(ctx, tx.conn, session.InstanceID, session.BranchID, session.ControlledEntityID, entity.EntityID)
				if err != nil {
					return RPObservation{}, err
				}
			}
			visible = append(visible, entity)
		}
	}
	view.PresentEntities = visible
	view.ReachablePlaces = make([]RPVisiblePlace, 0)
	view.SceneObjects = make([]RPVisibleSceneObject, 0)
	sceneObjectsAvailable, err := rpSceneObjectProjectionAvailable(ctx, tx.conn)
	if err != nil {
		return RPObservation{}, err
	}
	if sceneObjectsAvailable {
		rows, err = tx.conn.QueryContext(ctx, `SELECT object_id,object_key,display_name,object_kind,state_code FROM rp_scene_objects WHERE instance_id=? AND branch_id=? AND place_id=? ORDER BY object_key`, session.InstanceID, session.BranchID, view.PlaceID)
		if err != nil {
			return RPObservation{}, err
		}
		for rows.Next() {
			var object RPVisibleSceneObject
			if err := rows.Scan(&object.ObjectID, &object.Key, &object.DisplayName, &object.Kind, &object.State); err != nil {
				rows.Close()
				return RPObservation{}, err
			}
			object.Actions = sceneObjectActions(object.Kind, object.State)
			view.SceneObjects = append(view.SceneObjects, object)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return RPObservation{}, err
		}
		rows.Close()
	}
	var activeJourney int
	if err := tx.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_journeys WHERE instance_id=? AND branch_id=? AND agent_id=? AND status='active'`, session.InstanceID, session.BranchID, session.ControlledEntityID).Scan(&activeJourney); err != nil {
		return RPObservation{}, err
	}
	if activeJourney > 1 {
		return RPObservation{}, core.NewError(core.CodeProjectionDiverged, "multiple active journeys for one actor")
	}
	if activeJourney == 1 {
		var journey RPJourneyView
		if err := tx.conn.QueryRowContext(ctx, `SELECT journey_id,from_place_id,to_place_id,segment_place_id,scheduled_arrival_at FROM rp_journeys WHERE instance_id=? AND branch_id=? AND agent_id=? AND status='active'`, session.InstanceID, session.BranchID, session.ControlledEntityID).Scan(&journey.JourneyID, &journey.FromPlaceID, &journey.ToPlaceID, &journey.SegmentPlaceID, &journey.ScheduledArrivalAt); err != nil {
			return RPObservation{}, err
		}
		if journey.SegmentPlaceID != view.PlaceID {
			return RPObservation{}, core.NewError(core.CodeProjectionDiverged, "active journey position differs from segment")
		}
		view.ActiveJourney = &journey
	}
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
	// Topological adjacency is not proof of immediate travel. Use the move
	// owner's physical route check, including alternate paths, in this snapshot.
	for i := range view.ReachablePlaces {
		place := &view.ReachablePlaces[i]
		if activeJourney != 0 {
			continue
		}
		err = tx.conn.QueryRowContext(ctx, `SELECT duration_minutes FROM rp_timed_edges WHERE instance_id=? AND branch_id=? AND from_place_id=? AND to_place_id=?`, session.InstanceID, session.BranchID, view.PlaceID, place.PlaceID).Scan(&place.TravelMinutes)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return RPObservation{}, err
		}
		if err == nil {
			worksEnd, worksErr := readRPDirectWorksEnd(ctx, tx.conn, session.InstanceID, session.BranchID, view.PlaceID, place.PlaceID, view.WorldTime)
			if worksErr != nil {
				return RPObservation{}, worksErr
			}
			place.CanStartJourney = worksEnd == ""
			continue
		}
		place.CanMoveNow, err = rpTransitAllowsImmediate(ctx, tx.conn, session.InstanceID, session.BranchID, view.PlaceID, place.PlaceID, view.WorldTime)
		if err != nil {
			return RPObservation{}, err
		}
	}
	// Settled history belongs to the controlled observer, not a particular client.
	// Other sessions' prose can be read but only this session's turns are offered
	// for regeneration; narrative mutation endpoints retain their own authorization.
	// A witnessed NPC expression in an already-settled turn belongs to that
	// observer's turn narration. Keep the standalone receipt for unfinished
	// turns and other observers, who cannot read that turn's narration.
	view.RecentTurns = make([]RPHistoryTurn, 0)
	turnSessions := make([]string, 0)
	rows, err = tx.conn.QueryContext(ctx, `SELECT turn_run_id, narrative_json, can_regenerate, origin_session_id FROM
		(SELECT r.turn_run_id, r.narrative_json, r.settled_sequence, CASE WHEN r.session_id=? THEN 1 ELSE 0 END AS can_regenerate, r.session_id AS origin_session_id
		 FROM rp_turn_runs r JOIN rp_sessions h ON h.session_id=r.session_id
		 WHERE h.instance_id=? AND h.branch_id=? AND h.controlled_entity_id=? AND r.status='settled' AND r.settled_sequence>?
		 UNION ALL
		 SELECT e.event_id, json_array(CASE e.event_type
		 WHEN 'RPPlayerMoved' THEN '你前往了 ' || p.display_name || '。'
		 WHEN 'RPInterpersonalAction' THEN json_extract(e.payload,'$.description')
			 WHEN 'RPObjectInteracted' THEN json_extract(e.payload,'$.description')
			 WHEN 'RPNonverbalAction' THEN json_extract(e.payload,'$.description')
		 ELSE '你等待至 ' || json_extract(e.payload, '$.target_world_time') || '。' END), e.event_sequence, 0, ''
		 FROM events e LEFT JOIN agent_places p ON p.place_id = json_extract(e.payload, '$.to_place_id')
		 WHERE e.instance_id = ? AND e.branch_id = ? AND e.actor_id = ? AND e.event_sequence>?
		 AND e.event_type IN ('RPPlayerMoved', 'RPWaitCompleted', 'RPInterpersonalAction', 'RPObjectInteracted', 'RPNonverbalAction')
		 UNION ALL
		 SELECT e.event_id,json_array(CASE e.event_type
		 WHEN 'RPSpeechAccepted' THEN n.display_name || '说：“' || u.speech_text || '”'
		 ELSE n.display_name || '前往了 ' || p.display_name || '。' END),e.event_sequence,0,''
		 FROM events e JOIN event_batches b ON b.batch_id=e.batch_id JOIN commands c ON c.command_id=b.command_id
		 JOIN rp_sessions h ON h.session_id=json_extract(e.payload,'$.session_id') AND h.instance_id=e.instance_id AND h.branch_id=e.branch_id
		 JOIN materialized_entities n ON n.entity_id=e.actor_id
		 LEFT JOIN rp_utterances u ON u.event_id=e.event_id
		 LEFT JOIN agent_places p ON p.place_id=json_extract(e.payload,'$.to_place_id')
		 WHERE c.command_type='RPNPCInitiative' AND e.instance_id=? AND e.branch_id=? AND h.controlled_entity_id=? AND e.event_sequence>?
		 AND (e.event_type='RPNPCMoved' OR (e.event_type='RPSpeechAccepted' AND EXISTS (SELECT 1 FROM observation_records o WHERE o.source_event_id=e.event_id AND o.observer_agent_id=?)))
		 UNION ALL
			 SELECT e.event_id,json_array(json_extract(o.claim_payload,'$.description')),e.event_sequence,0,'witness:'||o.subject_agent_id
			 FROM observation_records o JOIN events e ON e.event_id=o.source_event_id
			 WHERE e.instance_id=? AND e.branch_id=? AND o.observer_agent_id=? AND e.event_sequence>?
			 AND e.event_type IN ('RPNonverbalAction','RPObjectInteracted')
			 AND json_extract(o.claim_payload,'$.claim_type') IN ('nonverbal_action','object_interaction')
			 AND o.observed_world_time<=? AND e.world_time<=?
			 AND NOT (e.event_type='RPNonverbalAction' AND EXISTS (
				 SELECT 1 FROM events parent
				 JOIN rp_npc_decisions d ON d.event_id=parent.event_id
				 JOIN rp_turn_runs r ON r.session_id=d.session_id AND r.player_turn_id=d.parent_turn_id
				 JOIN rp_sessions h ON h.session_id=r.session_id
				 WHERE parent.batch_id=e.batch_id AND parent.event_id=e.causation_event_id
				 AND parent.actor_id=e.actor_id AND parent.instance_id=e.instance_id AND parent.branch_id=e.branch_id
				 AND h.instance_id=e.instance_id AND h.branch_id=e.branch_id AND h.controlled_entity_id=o.observer_agent_id
				 AND r.status='settled' AND r.settled_sequence>?
			 ))
			 ORDER BY settled_sequence DESC LIMIT 50)
		ORDER BY settled_sequence`, session.SessionID, session.InstanceID, session.BranchID, session.ControlledEntityID, session.ChapterStartSequence,
		session.InstanceID, session.BranchID, session.ControlledEntityID, session.ChapterStartSequence,
		session.InstanceID, session.BranchID, session.ControlledEntityID, session.ChapterStartSequence, session.ControlledEntityID,
		session.InstanceID, session.BranchID, session.ControlledEntityID, session.ChapterStartSequence, view.WorldTime, view.WorldTime, session.ChapterStartSequence)
	if err != nil {
		return RPObservation{}, err
	}
	for rows.Next() {
		var turn RPHistoryTurn
		var raw, originSessionID string
		if err := rows.Scan(&turn.TurnRunID, &raw, &turn.CanRegenerate, &originSessionID); err != nil {
			rows.Close()
			return RPObservation{}, err
		}
		if err := json.Unmarshal([]byte(raw), &turn.NarrativeLines); err != nil {
			rows.Close()
			return RPObservation{}, err
		}
		view.RecentTurns = append(view.RecentTurns, turn)
		turnSessions = append(turnSessions, originSessionID)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return RPObservation{}, err
	}
	rows.Close()
	renderSelectionsAvailable, err := rpTableAvailableBeforeMigration(ctx, tx.conn, "rp_narrative_selections", RPNarrativeRendersSchemaVersion)
	if err != nil {
		return RPObservation{}, err
	}
	for i, originSessionID := range turnSessions {
		if strings.HasPrefix(originSessionID, "witness:") {
			subject := strings.TrimPrefix(originSessionID, "witness:")
			known, err := rpIdentityKnown(ctx, tx.conn, session.InstanceID, session.BranchID, session.ControlledEntityID, subject)
			if err != nil {
				return RPObservation{}, err
			}
			if !known {
				view.RecentTurns[i].TurnRunID, err = rpAnonymousEvidenceID(ctx, tx.conn, session.InstanceID, session.BranchID, session.ControlledEntityID, view.RecentTurns[i].TurnRunID)
				if err != nil {
					return RPObservation{}, err
				}
			}
			continue
		}
		if originSessionID == "" {
			continue
		}
		if renderSelectionsAvailable {
			var selectedID, selectedJSON string
			err = tx.conn.QueryRowContext(ctx, `SELECT r.render_id,r.lines_json FROM rp_narrative_selections s JOIN rp_narrative_renders r ON r.render_id=s.render_id AND r.turn_run_id=s.turn_run_id WHERE s.turn_run_id=?`, view.RecentTurns[i].TurnRunID).Scan(&selectedID, &selectedJSON)
			if err == nil {
				if err := json.Unmarshal([]byte(selectedJSON), &view.RecentTurns[i].NarrativeLines); err != nil {
					return RPObservation{}, core.WrapError(core.CodeProjectionDiverged, "decode selected narrative history", err)
				}
				view.RecentTurns[i].RenderID = selectedID
			} else if !errors.Is(err, sql.ErrNoRows) {
				return RPObservation{}, core.WrapError(core.CodeStorageFailure, "read selected narrative history", err)
			}
		}
		view.RecentTurns[i].ProviderCalls, err = readRPTurnProviderCalls(ctx, tx.conn, originSessionID, view.RecentTurns[i].TurnRunID)
		if err != nil {
			return RPObservation{}, err
		}
	}
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
	session, err := loadRPSessionRecord(ctx, q, principalID, sessionID)
	if err != nil {
		return RPSession{}, err
	}
	if err := requireCurrentRPSession(ctx, q, session); err != nil {
		return RPSession{}, err
	}
	return session, nil
}

// Exact completed-command receipts remain readable by their original
// principal/session after a controller handoff. This loader alone does not
// authorize a new world effect or access to the current observation.
func loadRPSessionRecord(ctx context.Context, q rpQueryer, principalID, sessionID string) (RPSession, error) {
	var session RPSession
	chapterColumn := `0`
	chapterAvailable, err := rpSessionChapterAvailable(ctx, q)
	if err != nil {
		return RPSession{}, err
	}
	if chapterAvailable {
		chapterColumn = `chapter_start_sequence`
	}
	err = q.QueryRowContext(ctx, `
		SELECT session_id, instance_id, branch_id, controlled_entity_id, pov, observation_cursor, `+chapterColumn+`,
		       turn_cursor, turn_state, status, created_at_utc, resumed_at_utc, control_generation, controller_instance_id
		FROM rp_sessions WHERE session_id = ? AND principal_id = ?`, sessionID, principalID,
	).Scan(&session.SessionID, &session.InstanceID, &session.BranchID, &session.ControlledEntityID,
		&session.POV, &session.ObservationCursor, &session.ChapterStartSequence, &session.TurnCursor, &session.TurnState,
		&session.Status, &session.CreatedAtUTC, &session.ResumedAtUTC, &session.ControlGeneration, &session.ControllerInstanceID)
	if err != nil {
		return RPSession{}, classifyMissing(err, "RP session")
	}
	return session, nil
}

func requireCurrentRPSession(ctx context.Context, q rpQueryer, session RPSession) error {
	generation, controller, err := rpControlGeneration(ctx, q, session.InstanceID, session.BranchID, session.ControlledEntityID)
	if err != nil {
		return err
	}
	if session.ControlGeneration != generation || session.ControllerInstanceID != controller {
		return core.NewError(core.CodeBranchConflict, "RP session controller generation is stale")
	}
	return nil
}

func rpControlGeneration(ctx context.Context, q rpQueryer, instanceID, branchID, entityID string) (int64, string, error) {
	var generation int64
	var controller, status string
	err := q.QueryRowContext(ctx, `SELECT generation,controller_instance_id,status FROM rp_controller_authorities WHERE instance_id=? AND branch_id=? AND entity_id=?`, instanceID, branchID, entityID).Scan(&generation, &controller, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", nil
	}
	if err != nil {
		return 0, "", core.WrapError(core.CodeStorageFailure, "read RP control generation", err)
	}
	if status == "released" {
		controller = ""
	}
	return generation, controller, nil
}

func authorizeRPControl(ctx context.Context, q rpQueryer, principalID, instanceID, branchID, entityID string) error {
	var owner string
	err := q.QueryRowContext(ctx, `SELECT a.principal_id FROM rp_controller_authorities a JOIN principals p ON p.principal_id=a.principal_id WHERE a.instance_id=? AND a.branch_id=? AND a.entity_id=? AND a.status='active' AND p.principal_type='service' AND p.status='active'`, instanceID, branchID, entityID).Scan(&owner)
	if err == nil {
		if owner != principalID {
			return core.NewError(core.CodeUnauthorized, "Entity belongs to another RP controller")
		}
		return nil
	}
	if err != sql.ErrNoRows {
		return core.WrapError(core.CodeStorageFailure, "check external RP controller", err)
	}
	var assigned int
	if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_controller_authorities WHERE instance_id=? AND branch_id=? AND entity_id=? AND status='active'`, instanceID, branchID, entityID).Scan(&assigned); err != nil {
		return core.WrapError(core.CodeStorageFailure, "check RP assignment integrity", err)
	}
	if assigned != 0 {
		return core.NewError(core.CodeUnauthorized, "assigned RP controller is inactive")
	}
	var count int
	err = q.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM capability_grants g JOIN principals p ON p.principal_id = g.principal_id
		JOIN world_instances w ON w.instance_id=g.instance_id
		WHERE g.principal_id = ? AND p.principal_type = 'player' AND p.status = 'active'
		  AND g.capability_id = 'world.rp.control' AND g.instance_id = ? AND g.branch_id = ?
		  AND g.subject_id = ? AND g.status = 'active' AND `+studioControlPredicate, principalID, instanceID, branchID, entityID).Scan(&count)
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
