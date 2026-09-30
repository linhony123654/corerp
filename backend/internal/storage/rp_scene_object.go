package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"corerp.local/backend/internal/core"
)

type RPVisibleSceneObject struct {
	ObjectID    string   `json:"object_id"`
	Key         string   `json:"key"`
	DisplayName string   `json:"display_name"`
	Kind        string   `json:"kind"`
	State       string   `json:"state"`
	Actions     []string `json:"actions"`
}

type RPSceneObjectActionResult struct {
	CommandID     string               `json:"command_id"`
	EventID       string               `json:"event_id"`
	EventSequence int64                `json:"event_sequence"`
	WorldTime     string               `json:"world_time"`
	Object        RPVisibleSceneObject `json:"object"`
	PreviousState string               `json:"previous_state"`
	Replayed      bool                 `json:"replayed"`
}

type rpSceneObjectStateChanged struct {
	Version           string   `json:"version"`
	SessionID         string   `json:"session_id"`
	ObjectID          string   `json:"object_id"`
	ObjectKey         string   `json:"object_key"`
	DisplayName       string   `json:"display_name"`
	ObjectKind        string   `json:"object_kind"`
	PlaceID           string   `json:"place_id"`
	Action            string   `json:"action"`
	PreviousState     string   `json:"previous_state"`
	State             string   `json:"state"`
	ObserverEntityIDs []string `json:"observer_entity_ids"`
}

func sceneObjectTransition(kind, state, action string) (string, bool) {
	switch kind {
	case "door", "container":
		if state == "closed" && action == "open" {
			return "open", true
		}
		if state == "open" && action == "close" {
			return "closed", true
		}
	case "light":
		if state == "off" && action == "switch_on" {
			return "on", true
		}
		if state == "on" && action == "switch_off" {
			return "off", true
		}
	}
	return "", false
}

func sceneObjectActions(kind, state string) []string {
	switch kind {
	case "door", "container":
		if state == "closed" {
			return []string{"open"}
		}
		if state == "open" {
			return []string{"close"}
		}
	case "light":
		if state == "off" {
			return []string{"switch_on"}
		}
		if state == "on" {
			return []string{"switch_off"}
		}
	}
	return []string{}
}

func (s *Store) InteractRPSceneObject(ctx context.Context, request core.RPSceneObjectActionRequest) (RPSceneObjectActionResult, error) {
	if err := request.Validate(); err != nil {
		return RPSceneObjectActionResult{}, err
	}
	requestHash, err := core.HashJSON(request)
	if err != nil {
		return RPSceneObjectActionResult{}, err
	}
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return RPSceneObjectActionResult{}, core.WrapError(core.CodeStorageFailure, "begin scene object action", err)
	}
	defer tx.Rollback(ctx)
	session, err := loadRPSessionRecord(ctx, tx.conn, request.PrincipalID, request.SessionID)
	if err != nil {
		return RPSceneObjectActionResult{}, err
	}
	commandKey := "rp_scene_object:" + session.SessionID + ":" + request.IdempotencyKey
	var commandID, oldHash, status string
	err = tx.conn.QueryRowContext(ctx, `SELECT command_id,request_hash,status FROM commands WHERE instance_id=? AND branch_id=? AND command_type='RPSceneObjectAction' AND idempotency_key=?`, session.InstanceID, session.BranchID, commandKey).Scan(&commandID, &oldHash, &status)
	if err == nil {
		if oldHash != requestHash {
			return RPSceneObjectActionResult{}, core.NewError(core.CodeIdempotencyMismatch, "scene object action key was used with another request")
		}
		if status != "committed" {
			return RPSceneObjectActionResult{}, core.NewError(core.CodeCommandInProgress, "scene object action is not committed")
		}
		return loadRPSceneObjectActionResult(ctx, tx.conn, commandID, true)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return RPSceneObjectActionResult{}, core.WrapError(core.CodeStorageFailure, "look up scene object action", err)
	}
	if err := requireCurrentRPSession(ctx, tx.conn, session); err != nil {
		return RPSceneObjectActionResult{}, err
	}
	if err := authorizeRPControl(ctx, tx.conn, request.PrincipalID, session.InstanceID, session.BranchID, session.ControlledEntityID); err != nil {
		return RPSceneObjectActionResult{}, err
	}
	if err := requireNoActiveRPSharedRound(ctx, tx.conn, session.InstanceID, session.BranchID); err != nil {
		return RPSceneObjectActionResult{}, err
	}
	var pending int
	if err := tx.conn.QueryRowContext(ctx, `SELECT (SELECT COUNT(*) FROM rp_wait_intents w JOIN rp_sessions h ON h.session_id=w.session_id WHERE h.instance_id=? AND h.branch_id=? AND w.status='pending')+(SELECT COUNT(*) FROM rp_turn_runs r JOIN rp_sessions h ON h.session_id=r.session_id WHERE h.instance_id=? AND h.branch_id=? AND r.status<>'settled')`, session.InstanceID, session.BranchID, session.InstanceID, session.BranchID).Scan(&pending); err != nil {
		return RPSceneObjectActionResult{}, err
	}
	if pending != 0 {
		return RPSceneObjectActionResult{}, core.NewError(core.CodeCommandInProgress, "finish the active RP action before interacting with an object")
	}
	var head, objectVersion int64
	var worldTime, placeID string
	if err := tx.conn.QueryRowContext(ctx, `SELECT b.head_sequence,c.current_world_time,p.place_id FROM branches b JOIN world_clocks c ON c.instance_id=b.instance_id AND c.branch_id=b.branch_id JOIN agent_positions p ON p.agent_id=? WHERE b.instance_id=? AND b.branch_id=?`, session.ControlledEntityID, session.InstanceID, session.BranchID).Scan(&head, &worldTime, &placeID); err != nil {
		return RPSceneObjectActionResult{}, classifyMissing(err, "scene object actor state")
	}
	if request.ExpectedCursor != head || session.ObservationCursor != head {
		return RPSceneObjectActionResult{}, core.NewError(core.CodeBranchConflict, "scene object action requires a current observation cursor")
	}
	var object RPVisibleSceneObject
	if err := tx.conn.QueryRowContext(ctx, `SELECT object_id,object_key,display_name,object_kind,state_code,projection_version FROM rp_scene_objects WHERE object_id=? AND instance_id=? AND branch_id=? AND place_id=?`, request.ObjectID, session.InstanceID, session.BranchID, placeID).Scan(&object.ObjectID, &object.Key, &object.DisplayName, &object.Kind, &object.State, &objectVersion); err != nil {
		return RPSceneObjectActionResult{}, classifyMissing(err, "scene object at current place")
	}
	nextState, ok := sceneObjectTransition(object.Kind, object.State, request.Action)
	if !ok {
		return RPSceneObjectActionResult{}, core.NewError(core.CodeBranchConflict, "scene object action is not legal from its current state")
	}
	if err := ensureCohortTransitionChronology(ctx, tx.conn, session.InstanceID, session.BranchID, worldTime); err != nil {
		return RPSceneObjectActionResult{}, err
	}
	observers, err := rpCoLocatedEntityIDs(ctx, tx.conn, session.InstanceID, session.BranchID, placeID, session.ControlledEntityID)
	if err != nil {
		return RPSceneObjectActionResult{}, err
	}
	observers = append(observers, session.ControlledEntityID)
	sort.Strings(observers)
	sequence := head + 1
	var epoch string
	if err := tx.conn.QueryRowContext(ctx, `SELECT epoch_id FROM rule_epochs WHERE instance_id=? AND branch_id=? AND start_sequence<=? AND (end_sequence IS NULL OR ?<end_sequence)`, session.InstanceID, session.BranchID, sequence, sequence).Scan(&epoch); err != nil {
		return RPSceneObjectActionResult{}, classifyMissing(err, "scene object Rule Epoch")
	}
	keyHash, err := core.HashJSON([]string{session.SessionID, request.IdempotencyKey})
	if err != nil {
		return RPSceneObjectActionResult{}, err
	}
	suffix := keyHash[7:]
	commandID = "cmd_rp_scene_object_" + suffix
	attemptID, batchID, eventID := "attempt_rp_scene_object_"+suffix, "batch_rp_scene_object_"+suffix, "event_rp_scene_object_"+suffix
	payload := rpSceneObjectStateChanged{Version: "corerp.scene-object-state.v1", SessionID: session.SessionID, ObjectID: object.ObjectID, ObjectKey: object.Key, DisplayName: object.DisplayName, ObjectKind: object.Kind, PlaceID: placeID, Action: request.Action, PreviousState: object.State, State: nextState, ObserverEntityIDs: observers}
	payloadJSON, err := core.CanonicalJSON(payload)
	if err != nil {
		return RPSceneObjectActionResult{}, err
	}
	batchHash, err := core.HashJSON(struct {
		CommandID string                    `json:"command_id"`
		Sequence  int64                     `json:"sequence"`
		WorldTime string                    `json:"world_time"`
		Payload   rpSceneObjectStateChanged `json:"payload"`
	}{commandID, sequence, worldTime, payload})
	if err != nil {
		return RPSceneObjectActionResult{}, err
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	statements := []struct {
		name, query string
		args        []any
	}{
		{"scene object command", `INSERT INTO commands(command_id,instance_id,branch_id,command_type,idempotency_key,request_hash,expected_head,principal_id,command_policy,status,created_at_utc) VALUES (?,?,?,'RPSceneObjectAction',?,?,?,?,'{"authorization":"rp-session-control"}','pending',?)`, []any{commandID, session.InstanceID, session.BranchID, commandKey, requestHash, head, request.PrincipalID, now}},
		{"scene object attempt", `INSERT INTO command_attempts(command_id,attempt_no,attempt_id,status,lease_owner,lease_until_utc,proposal_hash,created_at_utc) VALUES (?,1,?,'ready','corerp-rp-scene',?,?,?)`, []any{commandID, attemptID, s.now().UTC().Add(30 * time.Second).Format(time.RFC3339Nano), requestHash, now}},
		{"scene object batch", `INSERT INTO event_batches(batch_id,command_id,attempt_no,instance_id,branch_id,epoch_id,expected_head,first_sequence,last_sequence,event_count,world_time,batch_hash,committed_at_utc) VALUES (?,?,1,?,?,?,?,?,?,1,?,?,?)`, []any{batchID, commandID, session.InstanceID, session.BranchID, epoch, head, sequence, sequence, worldTime, batchHash, now}},
		{"scene object event", `INSERT INTO events(event_id,batch_id,instance_id,branch_id,event_sequence,batch_index,event_type,actor_id,world_time,payload) VALUES (?,?,?,?,?,0,'RPSceneObjectStateChanged',?,?,?)`, []any{eventID, batchID, session.InstanceID, session.BranchID, sequence, session.ControlledEntityID, worldTime, string(payloadJSON)}},
	}
	for _, statement := range statements {
		if err := execAgentOne(ctx, tx.conn, statement.name, statement.query, statement.args...); err != nil {
			return RPSceneObjectActionResult{}, err
		}
	}
	if err := execAgentOne(ctx, tx.conn, "update scene object state", `UPDATE rp_scene_objects SET state_code=?,state_event_id=?,projection_version=projection_version+1,last_event_sequence=? WHERE object_id=? AND instance_id=? AND branch_id=? AND state_code=? AND projection_version=?`, nextState, eventID, sequence, object.ObjectID, session.InstanceID, session.BranchID, object.State, objectVersion); err != nil {
		return RPSceneObjectActionResult{}, err
	}
	if err := insertRPOwnAction(ctx, tx.conn, session.ControlledEntityID, eventID, "object_"+request.Action, "", object.DisplayName, placeID, worldTime, nextState, session.InstanceID, session.BranchID, sequence); err != nil {
		return RPSceneObjectActionResult{}, err
	}
	if err := execAgentOne(ctx, tx.conn, "advance scene object clock lineage", `UPDATE world_clocks SET projection_version=projection_version+1,last_event_sequence=? WHERE instance_id=? AND branch_id=? AND current_world_time=?`, sequence, session.InstanceID, session.BranchID, worldTime); err != nil {
		return RPSceneObjectActionResult{}, err
	}
	if err := execAgentOne(ctx, tx.conn, "advance scene object branch", `UPDATE branches SET head_sequence=? WHERE instance_id=? AND branch_id=? AND head_sequence=?`, sequence, session.InstanceID, session.BranchID, head); err != nil {
		return RPSceneObjectActionResult{}, err
	}
	if err := insertRPParticipantOutbox(ctx, tx.conn, "outbox_"+commandID, eventID, "rp.scene_object.changed", session.InstanceID, session.BranchID, session.ControlledEntityID, observers, payloadJSON); err != nil {
		return RPSceneObjectActionResult{}, err
	}
	if err := execAgentOne(ctx, tx.conn, "commit scene object attempt", `UPDATE command_attempts SET status='committed',finished_at_utc=? WHERE command_id=? AND attempt_no=1 AND status='ready'`, now, commandID); err != nil {
		return RPSceneObjectActionResult{}, err
	}
	if err := execAgentOne(ctx, tx.conn, "commit scene object command", `UPDATE commands SET status='committed' WHERE command_id=? AND status='pending'`, commandID); err != nil {
		return RPSceneObjectActionResult{}, err
	}
	if s.beforeCommit != nil {
		if err := s.beforeCommit(); err != nil {
			return RPSceneObjectActionResult{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return RPSceneObjectActionResult{}, core.WrapError(core.CodeStorageFailure, "commit scene object action", err)
	}
	object.State, object.Actions = nextState, sceneObjectActions(object.Kind, nextState)
	return RPSceneObjectActionResult{CommandID: commandID, EventID: eventID, EventSequence: sequence, WorldTime: worldTime, Object: object, PreviousState: payload.PreviousState}, nil
}

func loadRPSceneObjectActionResult(ctx context.Context, conn *sql.Conn, commandID string, replayed bool) (RPSceneObjectActionResult, error) {
	var out RPSceneObjectActionResult
	var raw string
	if err := conn.QueryRowContext(ctx, `SELECT e.event_id,e.event_sequence,e.world_time,e.payload FROM event_batches b JOIN events e ON e.batch_id=b.batch_id WHERE b.command_id=? AND e.event_type='RPSceneObjectStateChanged'`, commandID).Scan(&out.EventID, &out.EventSequence, &out.WorldTime, &raw); err != nil {
		return out, classifyMissing(err, "scene object action result")
	}
	var payload rpSceneObjectStateChanged
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return out, core.WrapError(core.CodeProjectionDiverged, "decode scene object action result", err)
	}
	out.CommandID, out.PreviousState, out.Replayed = commandID, payload.PreviousState, replayed
	out.Object = RPVisibleSceneObject{ObjectID: payload.ObjectID, Key: payload.ObjectKey, DisplayName: payload.DisplayName, Kind: payload.ObjectKind, State: payload.State, Actions: sceneObjectActions(payload.ObjectKind, payload.State)}
	return out, nil
}
