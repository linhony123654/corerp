package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"corerp.local/backend/internal/core"
)

type rpTransitQueue struct {
	ID        string `json:"id"`
	WorldTime string `json:"world_time"`
	PhaseID   string `json:"phase_id"`
	Priority  int64  `json:"priority"`
	Payload   string `json:"payload"`
	Status    string `json:"status"`
}
type rpTransitDelay struct {
	AgentID               string         `json:"agent_id"`
	ScheduleID            string         `json:"schedule_id"`
	OriginalWorldTime     string         `json:"original_world_time"`
	FromPlaceID           string         `json:"from_place_id"`
	ToPlaceID             string         `json:"to_place_id"`
	ActivityCode          string         `json:"activity_code"`
	ScheduleSourceEventID string         `json:"schedule_source_event_id"`
	SchedulePriority      int            `json:"schedule_priority"`
	Previous              rpTransitQueue `json:"previous"`
	Retry                 rpTransitQueue `json:"retry"`
	Path                  []string       `json:"path"`
	DelaySourceEventIDs   []string       `json:"delay_source_event_ids"`
}

func readLatestRPTransitDelay(ctx context.Context, q replayQuerier, instance, branch, schedule string) (*rpTransitDelay, error) {
	var raw string
	err := q.QueryRowContext(ctx, `SELECT payload FROM events WHERE instance_id=? AND branch_id=? AND event_type='AgentTravelDelayed' AND json_extract(payload,'$.schedule_id')=? ORDER BY event_sequence DESC LIMIT 1`, instance, branch, schedule).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var delay rpTransitDelay
	if err := json.Unmarshal([]byte(raw), &delay); err != nil {
		return nil, err
	}
	return &delay, nil
}

func (s *Store) supersedeRPDelayedTravel(ctx context.Context, tx *immediateTx, item SchedulerItem, scheduled agentSchedulePayload, delay rpTransitDelay) (bool, error) {
	var later string
	// Actual arrival time is not appointment chronology: an older delayed
	// arrival must never supersede a newer original intent.
	err := tx.conn.QueryRowContext(ctx, `SELECT e.event_id FROM events e
		JOIN agent_schedule_entries a ON a.schedule_id=json_extract(e.payload,'$.schedule_id') AND a.agent_id=e.actor_id
		WHERE e.instance_id=? AND e.branch_id=? AND e.event_type IN ('AgentMoved','AgentActivityStarted')
		AND e.actor_id=? AND a.world_time>? AND a.world_time<=? AND e.world_time<=?
		AND a.schedule_id<>? ORDER BY a.world_time DESC,e.event_sequence DESC LIMIT 1`, M2DemoInstanceID, M2DemoBranchID, scheduled.AgentID, delay.OriginalWorldTime, item.WorldTime, item.WorldTime, scheduled.ScheduleID).Scan(&later)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	var laterDelayID, laterScheduleID string
	if later == "" {
		// Only an accepted latest delay with a still-active, pending queue can
		// supersede while waiting. Cancelled appointments are not intentions.
		var raw, original, place, activity, definition, queueID, queueTime, phase, payload string
		var priority int64
		err = tx.conn.QueryRowContext(ctx, `SELECT e.event_id,e.payload,a.schedule_id,a.world_time,a.place_id,a.activity_code,a.definition_event_id,q.scheduler_item_id,q.world_time,q.phase_id,q.declared_priority,q.payload
			FROM events e JOIN agent_schedule_entries a ON a.schedule_id=json_extract(e.payload,'$.schedule_id')
			JOIN scheduler_items q ON q.scheduler_item_id=a.scheduler_item_id
			WHERE e.instance_id=? AND e.branch_id=? AND e.event_type='AgentTravelDelayed'
			AND json_extract(e.payload,'$.agent_id')=? AND a.agent_id=? AND a.status='active' AND q.status='pending'
			AND q.instance_id=e.instance_id AND q.branch_id=e.branch_id
			AND json_extract(e.payload,'$.original_world_time')>? AND json_extract(e.payload,'$.original_world_time')<=?
			AND e.event_sequence=(SELECT MAX(n.event_sequence) FROM events n WHERE n.instance_id=e.instance_id AND n.branch_id=e.branch_id AND n.event_type='AgentTravelDelayed' AND json_extract(n.payload,'$.schedule_id')=a.schedule_id)
			ORDER BY json_extract(e.payload,'$.original_world_time') DESC,e.event_sequence DESC LIMIT 1`, M2DemoInstanceID, M2DemoBranchID, scheduled.AgentID, scheduled.AgentID, delay.OriginalWorldTime, item.WorldTime).Scan(&laterDelayID, &raw, &laterScheduleID, &original, &place, &activity, &definition, &queueID, &queueTime, &phase, &priority, &payload)
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		var next rpTransitDelay
		if err := json.Unmarshal([]byte(raw), &next); err != nil {
			return false, err
		}
		if next.ScheduleID != laterScheduleID || next.AgentID != scheduled.AgentID || next.OriginalWorldTime != original || next.ToPlaceID != place || next.ActivityCode != activity || next.ScheduleSourceEventID != definition || next.Retry.ID != queueID || next.Retry.WorldTime != queueTime || next.Retry.PhaseID != phase || next.Retry.Priority != priority || next.Retry.Payload != payload {
			return false, core.NewError(core.CodeProjectionDiverged, "later pending appointment differs from accepted transit evidence")
		}
	}
	event := struct {
		AgentID              string `json:"agent_id"`
		ScheduleID           string `json:"schedule_id"`
		DelayEventID         string `json:"delay_event_id"`
		LaterActivityEventID string `json:"later_activity_event_id"`
		LaterScheduleID      string `json:"later_schedule_id,omitempty"`
		LaterDelayEventID    string `json:"later_delay_event_id,omitempty"`
	}{scheduled.AgentID, scheduled.ScheduleID, "event_" + delay.Previous.ID, later, laterScheduleID, laterDelayID}
	mutation := scheduledMutation{Private: true, EventType: "AgentTravelSuperseded", EventPayload: event, ApplyDomainRows: func(ctx context.Context, conn *sql.Conn, _ string, _ int64) error {
		return execAgentOne(ctx, conn, "supersede delayed appointment", `UPDATE agent_schedule_entries SET status='cancelled' WHERE schedule_id=? AND scheduler_item_id=? AND status='active'`, scheduled.ScheduleID, item.SchedulerItemID)
	}}
	if err := s.commitScheduledMutationForBranch(ctx, tx, item, scheduledPayload{Day: scheduled.Day}, mutation, M2DemoInstanceID, M2DemoBranchID); err != nil {
		return false, err
	}
	return true, nil
}

// Original appointment time/identity is retained. Only its current execution
// queue pointer changes; original and retry queue metadata are replayable facts.
func (s *Store) delayRPAgentTransit(ctx context.Context, tx *immediateTx, item SchedulerItem, scheduled agentSchedulePayload, from, originalTime string) (bool, error) {
	if from == scheduled.ToPlaceID {
		return false, nil
	}
	var configured int
	if err := tx.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPTransitWorksDefined' AND json_extract(payload,'$.window.ends_at')>?`, M2DemoInstanceID, M2DemoBranchID, item.WorldTime).Scan(&configured); err != nil {
		return false, err
	}
	if configured == 0 {
		return false, nil
	}
	arrival, err := readRPTransitArrival(ctx, tx.conn, M2DemoInstanceID, M2DemoBranchID, from, scheduled.ToPlaceID, item.WorldTime)
	if err != nil {
		return false, err
	}
	// Legacy schedules may predate explicit RP topology; only declared paths
	// have physical closure evidence. Do not invent missing graph connections.
	if !arrival.Reachable || arrival.WorldTime == item.WorldTime {
		return false, nil
	}
	hash, err := core.HashJSON([]string{M2DemoInstanceID, M2DemoBranchID, item.SchedulerItemID, arrival.WorldTime})
	if err != nil {
		return false, err
	}
	due, err := time.Parse(time.RFC3339, arrival.WorldTime)
	if err != nil {
		return false, err
	}
	base, _ := time.Parse(time.RFC3339, "2026-09-22T00:00:00Z")
	retryPayload := scheduled
	retryPayload.Day = int(due.Sub(base) / (24 * time.Hour))
	encoded, err := core.CanonicalJSON(retryPayload)
	if err != nil {
		return false, err
	}
	delay := rpTransitDelay{AgentID: scheduled.AgentID, ScheduleID: scheduled.ScheduleID, OriginalWorldTime: originalTime, FromPlaceID: from, ToPlaceID: scheduled.ToPlaceID, ActivityCode: scheduled.ActivityCode, Path: arrival.Path, DelaySourceEventIDs: arrival.DelaySourceEventIDs,
		Previous: rpTransitQueue{ID: item.SchedulerItemID, WorldTime: item.WorldTime, PhaseID: item.PhaseID, Priority: item.DeclaredPriority, Payload: item.Payload, Status: "completed"},
		Retry:    rpTransitQueue{ID: "sched_transit_" + hash[7:], WorldTime: arrival.WorldTime, PhaseID: item.PhaseID, Priority: item.DeclaredPriority, Payload: string(encoded), Status: "pending"}}
	if err := tx.conn.QueryRowContext(ctx, `SELECT definition_event_id,declared_priority FROM agent_schedule_entries WHERE schedule_id=?`, scheduled.ScheduleID).Scan(&delay.ScheduleSourceEventID, &delay.SchedulePriority); err != nil {
		return false, err
	}
	mutation := scheduledMutation{Private: true, EventType: "AgentTravelDelayed", EventPayload: delay, ApplyDomainRows: func(ctx context.Context, conn *sql.Conn, _ string, _ int64) error {
		if err := execAgentOne(ctx, conn, "queue delayed arrival", `INSERT INTO scheduler_items(scheduler_item_id,instance_id,branch_id,world_time,phase_id,declared_priority,status,payload) VALUES (?,?,?,?,?,?,'pending',?)`, delay.Retry.ID, M2DemoInstanceID, M2DemoBranchID, delay.Retry.WorldTime, delay.Retry.PhaseID, delay.Retry.Priority, delay.Retry.Payload); err != nil {
			return err
		}
		return execAgentOne(ctx, conn, "bind delayed appointment queue", `UPDATE agent_schedule_entries SET scheduler_item_id=? WHERE schedule_id=? AND scheduler_item_id=? AND world_time=? AND status='active'`, delay.Retry.ID, delay.ScheduleID, delay.Previous.ID, originalTime)
	}}
	if err := s.commitScheduledMutationForBranch(ctx, tx, item, scheduledPayload{Day: scheduled.Day}, mutation, M2DemoInstanceID, M2DemoBranchID); err != nil {
		return false, err
	}
	return true, nil
}
