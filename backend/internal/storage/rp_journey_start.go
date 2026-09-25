package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"corerp.local/backend/internal/core"
)

type RPJourneyResult struct {
	CommandID          string `json:"command_id"`
	JourneyID          string `json:"journey_id"`
	EventID            string `json:"event_id"`
	EventSequence      int64  `json:"event_sequence"`
	WorldTime          string `json:"world_time"`
	FromPlaceID        string `json:"from_place_id"`
	ToPlaceID          string `json:"to_place_id"`
	SegmentPlaceID     string `json:"segment_place_id"`
	ScheduledArrivalAt string `json:"scheduled_arrival_at"`
	Status             string `json:"status"`
	Replayed           bool   `json:"replayed"`
}

type rpJourneyStartEvent struct {
	JourneyID          string   `json:"journey_id"`
	SessionID          string   `json:"session_id"`
	AgentID            string   `json:"agent_id"`
	EdgeID             string   `json:"edge_id"`
	FromPlaceID        string   `json:"from_place_id"`
	ToPlaceID          string   `json:"to_place_id"`
	SegmentPlaceID     string   `json:"segment_place_id"`
	ScheduledArrivalAt string   `json:"scheduled_arrival_at"`
	ArrivalScheduleID  string   `json:"arrival_schedule_id"`
	ArrivalItemID      string   `json:"arrival_item_id"`
	ArrivalActivity    string   `json:"arrival_activity"`
	ArrivalPriority    int64    `json:"arrival_priority"`
	CoLocatedEntityIDs []string `json:"co_located_entity_ids"`
}

func (s *Store) StartRPJourney(ctx context.Context, request core.RPMoveRequest) (RPJourneyResult, error) {
	if err := request.Validate(); err != nil {
		return RPJourneyResult{}, err
	}
	requestHash, err := core.HashJSON(request)
	if err != nil {
		return RPJourneyResult{}, err
	}
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return RPJourneyResult{}, core.WrapError(core.CodeStorageFailure, "begin RP journey", err)
	}
	defer tx.Rollback(ctx)
	session, err := loadRPSessionRecord(ctx, tx.conn, request.PrincipalID, request.SessionID)
	if err != nil {
		return RPJourneyResult{}, err
	}
	commandKey := "rp_journey:" + session.SessionID + ":" + request.IdempotencyKey
	var existingCommandID, existingHash, existingStatus string
	err = tx.conn.QueryRowContext(ctx, `SELECT command_id,request_hash,status FROM commands WHERE instance_id=? AND branch_id=? AND command_type='RPJourneyStart' AND idempotency_key=?`, session.InstanceID, session.BranchID, commandKey).Scan(&existingCommandID, &existingHash, &existingStatus)
	if err == nil {
		if existingHash != requestHash {
			return RPJourneyResult{}, core.NewError(core.CodeIdempotencyMismatch, "journey key was used with another request")
		}
		if existingStatus != "committed" {
			return RPJourneyResult{}, core.NewError(core.CodeCommandInProgress, "journey command is not committed")
		}
		return loadRPJourneyResult(ctx, tx.conn, existingCommandID, true)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return RPJourneyResult{}, err
	}
	if err := requireCurrentRPSession(ctx, tx.conn, session); err != nil {
		return RPJourneyResult{}, err
	}
	if err := authorizeRPControl(ctx, tx.conn, request.PrincipalID, session.InstanceID, session.BranchID, session.ControlledEntityID); err != nil {
		return RPJourneyResult{}, err
	}
	if err := requireNoActiveRPSharedRound(ctx, tx.conn, session.InstanceID, session.BranchID); err != nil {
		return RPJourneyResult{}, err
	}
	if session.Status != "active" {
		return RPJourneyResult{}, core.NewError(core.CodeBranchConflict, "RP session is closed")
	}
	if err := validateRPBinding(ctx, tx.conn, session.InstanceID, session.BranchID, session.ControlledEntityID); err != nil {
		return RPJourneyResult{}, err
	}
	var busy int
	if err := tx.conn.QueryRowContext(ctx, `SELECT
		(SELECT COUNT(*) FROM rp_wait_intents w JOIN rp_sessions h ON h.session_id=w.session_id WHERE h.instance_id=? AND h.branch_id=? AND w.status='pending')+
		(SELECT COUNT(*) FROM rp_turn_runs r JOIN rp_sessions h ON h.session_id=r.session_id WHERE h.instance_id=? AND h.branch_id=? AND r.status<>'settled')+
		(SELECT COUNT(*) FROM rp_journeys j WHERE j.instance_id=? AND j.branch_id=? AND j.agent_id=? AND j.status='active')`,
		session.InstanceID, session.BranchID, session.InstanceID, session.BranchID, session.InstanceID, session.BranchID, session.ControlledEntityID).Scan(&busy); err != nil {
		return RPJourneyResult{}, err
	}
	if busy != 0 {
		return RPJourneyResult{}, core.NewError(core.CodeCommandInProgress, "finish active RP action or journey first")
	}
	var head, positionVersion, currentDay int64
	var worldTime, actualPlace string
	err = tx.conn.QueryRowContext(ctx, `SELECT b.head_sequence,c.current_world_time,c.current_day,p.place_id,p.projection_version
		FROM branches b JOIN world_clocks c ON c.instance_id=b.instance_id AND c.branch_id=b.branch_id
		JOIN agent_profiles a ON a.instance_id=b.instance_id AND a.branch_id=b.branch_id
		JOIN agent_positions p ON p.agent_id=a.agent_id
		WHERE b.instance_id=? AND b.branch_id=? AND a.agent_id=? AND a.status='active'`,
		session.InstanceID, session.BranchID, session.ControlledEntityID).Scan(&head, &worldTime, &currentDay, &actualPlace, &positionVersion)
	if err != nil {
		return RPJourneyResult{}, classifyMissing(err, "journey actor")
	}
	if request.ExpectedCursor != head || session.ObservationCursor != head || request.FromPlaceID != actualPlace {
		return RPJourneyResult{}, core.NewError(core.CodeBranchConflict, "journey requires current observed origin")
	}
	var edgeID, segmentID string
	var minutes int
	err = tx.conn.QueryRowContext(ctx, `SELECT t.edge_id,t.segment_place_id,t.duration_minutes FROM rp_timed_edges t
		JOIN agent_places a ON a.place_id=t.from_place_id JOIN agent_places b ON b.place_id=t.to_place_id
		JOIN agent_places m ON m.place_id=t.segment_place_id
		WHERE t.instance_id=? AND t.branch_id=? AND t.from_place_id=? AND t.to_place_id=?
		AND a.instance_id=t.instance_id AND a.branch_id=t.branch_id AND a.status='active'
		AND b.instance_id=t.instance_id AND b.branch_id=t.branch_id AND b.status='active'
		AND m.instance_id=t.instance_id AND m.branch_id=t.branch_id AND m.status='active'`,
		session.InstanceID, session.BranchID, actualPlace, request.ToPlaceID).Scan(&edgeID, &segmentID, &minutes)
	if err != nil {
		return RPJourneyResult{}, classifyMissing(err, "active timed route")
	}
	worksEnd, err := readRPDirectWorksEnd(ctx, tx.conn, session.InstanceID, session.BranchID, actualPlace, request.ToPlaceID, worldTime)
	if err != nil {
		return RPJourneyResult{}, err
	}
	if worksEnd != "" {
		return RPJourneyResult{}, core.NewError(core.CodeBranchConflict, "timed route is obstructed; wait or choose another route")
	}
	if err := ensureCohortTransitionChronology(ctx, tx.conn, session.InstanceID, session.BranchID, worldTime); err != nil {
		return RPJourneyResult{}, err
	}
	start, err := time.Parse(time.RFC3339, worldTime)
	if err != nil {
		return RPJourneyResult{}, err
	}
	arrival := start.Add(time.Duration(minutes) * time.Minute).UTC().Format(time.RFC3339)
	base, _ := time.Parse(time.RFC3339, "2026-09-22T00:00:00Z")
	arrivalDay := int(start.Add(time.Duration(minutes)*time.Minute).Sub(base) / (24 * time.Hour))
	if arrivalDay < int(currentDay) || arrivalDay < 0 {
		return RPJourneyResult{}, core.NewError(core.CodeProjectionDiverged, "journey arrival day predates current world")
	}
	coLocated, err := rpCoLocatedEntityIDs(ctx, tx.conn, session.InstanceID, session.BranchID, segmentID, session.ControlledEntityID)
	if err != nil {
		return RPJourneyResult{}, err
	}
	sequence := head + 1
	var epochID string
	if err := tx.conn.QueryRowContext(ctx, `SELECT epoch_id FROM rule_epochs WHERE instance_id=? AND branch_id=? AND start_sequence<=? AND (end_sequence IS NULL OR ?<end_sequence)`, session.InstanceID, session.BranchID, sequence, sequence).Scan(&epochID); err != nil {
		return RPJourneyResult{}, classifyMissing(err, "journey Rule Epoch")
	}
	keyHash, err := core.HashJSON([]string{session.SessionID, request.IdempotencyKey})
	if err != nil {
		return RPJourneyResult{}, err
	}
	suffix := keyHash[7:]
	commandID, attemptID, batchID := "cmd_rp_journey_"+suffix, "attempt_rp_journey_"+suffix, "batch_rp_journey_"+suffix
	eventID, journeyID := "event_rp_journey_"+suffix, "journey_rp_"+suffix
	startScheduleID, startItemID := "schedule_rp_journey_start_"+suffix, "sched_rp_journey_start_"+suffix
	arrivalScheduleID, arrivalItemID := "schedule_rp_journey_arrive_"+suffix, "sched_rp_journey_arrive_"+suffix
	payload := rpJourneyStartEvent{JourneyID: journeyID, SessionID: session.SessionID, AgentID: session.ControlledEntityID, EdgeID: edgeID, FromPlaceID: actualPlace, ToPlaceID: request.ToPlaceID, SegmentPlaceID: segmentID, ScheduledArrivalAt: arrival, ArrivalScheduleID: arrivalScheduleID, ArrivalItemID: arrivalItemID, ArrivalActivity: "rp_journey_arrival", CoLocatedEntityIDs: coLocated}
	encoded, err := core.CanonicalJSON(payload)
	if err != nil {
		return RPJourneyResult{}, err
	}
	batchHash, err := core.HashJSON(struct {
		CommandID string              `json:"command_id"`
		Sequence  int64               `json:"sequence"`
		WorldTime string              `json:"world_time"`
		Payload   rpJourneyStartEvent `json:"payload"`
	}{commandID, sequence, worldTime, payload})
	if err != nil {
		return RPJourneyResult{}, err
	}
	startQueuePayload, err := core.CanonicalJSON(agentSchedulePayload{Kind: "agent_move", Day: int(currentDay), AgentID: session.ControlledEntityID, ScheduleID: startScheduleID, ToPlaceID: segmentID, ActivityCode: "rp_journey_start"})
	if err != nil {
		return RPJourneyResult{}, err
	}
	arrivalQueuePayload, err := core.CanonicalJSON(agentSchedulePayload{Kind: "rp_journey_arrival", Day: arrivalDay, AgentID: session.ControlledEntityID, ScheduleID: arrivalScheduleID, ToPlaceID: request.ToPlaceID, ActivityCode: "rp_journey_arrival", JourneyID: journeyID})
	if err != nil {
		return RPJourneyResult{}, err
	}
	now := s.now().UTC()
	nowText := now.Format(time.RFC3339Nano)
	statements := []struct {
		name, query string
		args        []any
	}{
		{"journey command", `INSERT INTO commands(command_id,instance_id,branch_id,command_type,idempotency_key,request_hash,expected_head,principal_id,command_policy,status,created_at_utc) VALUES (?,?,?,'RPJourneyStart',?,?,?,?,'{"authorization":"rp-session-control"}','pending',?)`, []any{commandID, session.InstanceID, session.BranchID, commandKey, requestHash, head, request.PrincipalID, nowText}},
		{"journey attempt", `INSERT INTO command_attempts(command_id,attempt_no,attempt_id,status,lease_owner,lease_until_utc,proposal_hash,created_at_utc) VALUES (?,1,?,'ready','corerp-f2-journey',?,?,?)`, []any{commandID, attemptID, now.Add(30 * time.Second).Format(time.RFC3339Nano), requestHash, nowText}},
		{"journey batch", `INSERT INTO event_batches(batch_id,command_id,attempt_no,instance_id,branch_id,epoch_id,expected_head,first_sequence,last_sequence,event_count,world_time,batch_hash,committed_at_utc) VALUES (?,?,1,?,?,?,?,?,?,1,?,?,?)`, []any{batchID, commandID, session.InstanceID, session.BranchID, epochID, head, sequence, sequence, worldTime, batchHash, nowText}},
		{"journey start Event", `INSERT INTO events(event_id,batch_id,instance_id,branch_id,event_sequence,batch_index,event_type,actor_id,world_time,payload) VALUES (?,?,?,?,?,0,'RPJourneyStarted',?,?,?)`, []any{eventID, batchID, session.InstanceID, session.BranchID, sequence, session.ControlledEntityID, worldTime, string(encoded)}},
		{"journey start item", `INSERT INTO scheduler_items(scheduler_item_id,instance_id,branch_id,world_time,phase_id,declared_priority,status,payload) VALUES (?,?,?,?,?,0,'completed',?)`, []any{startItemID, session.InstanceID, session.BranchID, worldTime, m2AgentPhaseID, string(startQueuePayload)}},
		{"journey start schedule", `INSERT INTO agent_schedule_entries(schedule_id,agent_id,world_time,place_id,activity_code,declared_priority,scheduler_item_id,status,definition_event_id) VALUES (?,?,?,?,'rp_journey_start',0,?,'completed',?)`, []any{startScheduleID, session.ControlledEntityID, worldTime, segmentID, startItemID, eventID}},
		{"journey segment movement", `INSERT INTO agent_movements(movement_id,event_id,agent_id,from_place_id,to_place_id,schedule_id,activity_code,world_time,movement_kind) VALUES (?,?,?,?,?,?,'rp_journey_start',?,'scheduled')`, []any{"movement_rp_journey_start_" + suffix, eventID, session.ControlledEntityID, actualPlace, segmentID, startScheduleID, worldTime}},
		{"journey arrival item", `INSERT INTO scheduler_items(scheduler_item_id,instance_id,branch_id,world_time,phase_id,declared_priority,status,payload) VALUES (?,?,?,?,?,0,'pending',?)`, []any{arrivalItemID, session.InstanceID, session.BranchID, arrival, m2AgentPhaseID, string(arrivalQueuePayload)}},
		{"journey arrival schedule", `INSERT INTO agent_schedule_entries(schedule_id,agent_id,world_time,place_id,activity_code,declared_priority,scheduler_item_id,status,definition_event_id) VALUES (?,?,?,?,'rp_journey_arrival',0,?,'active',?)`, []any{arrivalScheduleID, session.ControlledEntityID, arrival, request.ToPlaceID, arrivalItemID, eventID}},
		{"active journey", `INSERT INTO rp_journeys(journey_id,instance_id,branch_id,agent_id,edge_id,from_place_id,to_place_id,segment_place_id,started_at,scheduled_arrival_at,status,start_event_id,arrival_schedule_id,arrival_item_id) VALUES (?,?,?,?,?,?,?,?,?,?,'active',?,?,?)`, []any{journeyID, session.InstanceID, session.BranchID, session.ControlledEntityID, edgeID, actualPlace, request.ToPlaceID, segmentID, worldTime, arrival, eventID, arrivalScheduleID, arrivalItemID}},
	}
	for _, statement := range statements {
		if err := execAgentOne(ctx, tx.conn, statement.name, statement.query, statement.args...); err != nil {
			return RPJourneyResult{}, err
		}
	}
	if err := execAgentOne(ctx, tx.conn, "enter journey segment", `UPDATE agent_positions SET place_id=?,activity_code='rp_journey_start',effective_world_time=?,projection_version=projection_version+1,last_event_sequence=? WHERE agent_id=? AND projection_version=?`, segmentID, worldTime, sequence, session.ControlledEntityID, positionVersion); err != nil {
		return RPJourneyResult{}, err
	}
	for _, other := range coLocated {
		for _, pair := range [][2]string{{session.ControlledEntityID, other}, {other, session.ControlledEntityID}} {
			if err := upsertCoLocationKnowledge(ctx, tx.conn, eventID, sequence, pair[0], pair[1], segmentID, worldTime); err != nil {
				return RPJourneyResult{}, err
			}
		}
	}
	if err := execAgentOne(ctx, tx.conn, "journey clock lineage", `UPDATE world_clocks SET projection_version=projection_version+1,last_event_sequence=? WHERE instance_id=? AND branch_id=? AND current_world_time=?`, sequence, session.InstanceID, session.BranchID, worldTime); err != nil {
		return RPJourneyResult{}, err
	}
	if err := execAgentOne(ctx, tx.conn, "journey head", `UPDATE branches SET head_sequence=? WHERE instance_id=? AND branch_id=? AND head_sequence=?`, sequence, session.InstanceID, session.BranchID, head); err != nil {
		return RPJourneyResult{}, err
	}
	if err := s.insertScopedAudit(ctx, tx.conn, session.InstanceID, session.BranchID, "audit_"+commandID, "agent_decision", eventID, commandID, attemptID, worldTime, nowText, encoded); err != nil {
		return RPJourneyResult{}, err
	}
	if err := insertAgentOutbox(ctx, tx.conn, "outbox_"+commandID, eventID, "rp.journey.started", encoded); err != nil {
		return RPJourneyResult{}, err
	}
	if err := execAgentOne(ctx, tx.conn, "commit journey attempt", `UPDATE command_attempts SET status='committed',finished_at_utc=? WHERE command_id=? AND attempt_no=1 AND status='ready'`, nowText, commandID); err != nil {
		return RPJourneyResult{}, err
	}
	if err := execAgentOne(ctx, tx.conn, "commit journey command", `UPDATE commands SET status='committed' WHERE command_id=? AND status='pending'`, commandID); err != nil {
		return RPJourneyResult{}, err
	}
	if s.beforeCommit != nil {
		if err := s.beforeCommit(); err != nil {
			return RPJourneyResult{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return RPJourneyResult{}, core.WrapError(core.CodeStorageFailure, "commit RP journey start", err)
	}
	return RPJourneyResult{CommandID: commandID, JourneyID: journeyID, EventID: eventID, EventSequence: sequence, WorldTime: worldTime, FromPlaceID: actualPlace, ToPlaceID: request.ToPlaceID, SegmentPlaceID: segmentID, ScheduledArrivalAt: arrival, Status: "active"}, nil
}

func loadRPJourneyResult(ctx context.Context, conn *sql.Conn, commandID string, replayed bool) (RPJourneyResult, error) {
	var result RPJourneyResult
	var raw string
	err := conn.QueryRowContext(ctx, `SELECT e.event_id,e.event_sequence,e.world_time,e.payload FROM events e JOIN event_batches b ON b.batch_id=e.batch_id WHERE b.command_id=? AND e.event_type='RPJourneyStarted'`, commandID).Scan(&result.EventID, &result.EventSequence, &result.WorldTime, &raw)
	if err != nil {
		return result, classifyMissing(err, "accepted journey")
	}
	var payload rpJourneyStartEvent
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return result, err
	}
	result.CommandID, result.JourneyID, result.FromPlaceID, result.ToPlaceID = commandID, payload.JourneyID, payload.FromPlaceID, payload.ToPlaceID
	result.SegmentPlaceID, result.ScheduledArrivalAt, result.Replayed = payload.SegmentPlaceID, payload.ScheduledArrivalAt, replayed
	if err := conn.QueryRowContext(ctx, `SELECT status FROM rp_journeys WHERE journey_id=?`, payload.JourneyID).Scan(&result.Status); err != nil {
		return RPJourneyResult{}, classifyMissing(err, "journey state")
	}
	return result, nil
}
