package storage

import (
	"context"
	"database/sql"
	"time"

	"corerp.local/backend/internal/core"
)

type rpJourneyArrivalEvent struct {
	JourneyID          string   `json:"journey_id"`
	AgentID            string   `json:"agent_id"`
	EdgeID             string   `json:"edge_id"`
	FromPlaceID        string   `json:"from_place_id"`
	ToPlaceID          string   `json:"to_place_id"`
	CoLocatedEntityIDs []string `json:"co_located_entity_ids"`
}

type rpJourneyDelayEvent struct {
	JourneyID           string   `json:"journey_id"`
	AgentID             string   `json:"agent_id"`
	OldItemID           string   `json:"old_item_id"`
	NewItemID           string   `json:"new_item_id"`
	OldArrivalAt        string   `json:"old_arrival_at"`
	NewArrivalAt        string   `json:"new_arrival_at"`
	SegmentPlaceID      string   `json:"segment_place_id"`
	WorksSourceEventIDs []string `json:"works_source_event_ids"`
}

func readRPDirectWorksEvidence(ctx context.Context, conn *sql.Conn, instance, branch, from, to, at string) (string, []string, error) {
	rows, err := conn.QueryContext(ctx, `SELECT event_id,json_extract(payload,'$.window.ends_at') FROM events
		WHERE instance_id=? AND branch_id=? AND event_type='RPTransitWorksDefined'
		AND json_extract(payload,'$.window.starts_at')<=? AND json_extract(payload,'$.window.ends_at')>?
		AND ((json_extract(payload,'$.window.from_place_id')=? AND json_extract(payload,'$.window.to_place_id')=?)
		OR (json_extract(payload,'$.window.from_place_id')=? AND json_extract(payload,'$.window.to_place_id')=?)) ORDER BY event_sequence`,
		instance, branch, at, at, from, to, to, from)
	if err != nil {
		return "", nil, err
	}
	defer rows.Close()
	var end string
	var sources []string
	for rows.Next() {
		var id, until string
		if err := rows.Scan(&id, &until); err != nil {
			return "", nil, err
		}
		if until > end {
			end = until
		}
		sources = append(sources, id)
	}
	return end, sources, rows.Err()
}

func (s *Store) executeRPJourneyArrival(ctx context.Context, tx *immediateTx, item SchedulerItem, scheduled agentSchedulePayload, instance, branch string) error {
	if scheduled.JourneyID == "" || scheduled.AgentID == "" || scheduled.ScheduleID == "" || scheduled.ActivityCode == "" || scheduled.Day < 0 {
		return core.NewError(core.CodeProjectionDiverged, "invalid journey arrival queue payload")
	}
	var edgeID, from, to, segment, arrivalAt, startEventID, scheduleStatus, scheduleItemID, scheduleDestination, scheduleActivity, positionPlace string
	var positionVersion int64
	err := tx.conn.QueryRowContext(ctx, `SELECT j.edge_id,j.from_place_id,j.to_place_id,j.segment_place_id,j.scheduled_arrival_at,j.start_event_id,
		a.status,a.scheduler_item_id,a.place_id,a.activity_code,p.place_id,p.projection_version
		FROM rp_journeys j JOIN agent_schedule_entries a ON a.schedule_id=j.arrival_schedule_id
		JOIN agent_positions p ON p.agent_id=j.agent_id
		WHERE j.journey_id=? AND j.instance_id=? AND j.branch_id=? AND j.agent_id=? AND j.status='active'
		AND j.arrival_schedule_id=? AND j.arrival_item_id=?`, scheduled.JourneyID, instance, branch, scheduled.AgentID, scheduled.ScheduleID, item.SchedulerItemID).Scan(
		&edgeID, &from, &to, &segment, &arrivalAt, &startEventID, &scheduleStatus, &scheduleItemID, &scheduleDestination, &scheduleActivity, &positionPlace, &positionVersion)
	if err != nil {
		return classifyMissing(err, "active journey arrival")
	}
	if arrivalAt != item.WorldTime || scheduleStatus != "active" || scheduleItemID != item.SchedulerItemID || scheduleDestination != to || scheduleActivity != scheduled.ActivityCode || scheduled.ToPlaceID != to || positionPlace != segment {
		return core.NewError(core.CodeProjectionDiverged, "journey, segment position and arrival queue differ")
	}
	worksEnd, worksSources, err := readRPDirectWorksEvidence(ctx, tx.conn, instance, branch, from, to, item.WorldTime)
	if err != nil {
		return err
	}
	if worksEnd != "" {
		if worksEnd <= item.WorldTime {
			return core.NewError(core.CodeProjectionDiverged, "journey works delay does not advance time")
		}
		nextTime, err := time.Parse(time.RFC3339, worksEnd)
		if err != nil {
			return err
		}
		base, _ := time.Parse(time.RFC3339, "2026-09-22T00:00:00Z")
		nextPayload := scheduled
		nextPayload.Day = int(nextTime.Sub(base) / (24 * time.Hour))
		if nextPayload.Day < scheduled.Day {
			return core.NewError(core.CodeProjectionDiverged, "journey works delay moves backward")
		}
		encoded, err := core.CanonicalJSON(nextPayload)
		if err != nil {
			return err
		}
		hash, err := core.HashJSON([]string{scheduled.JourneyID, item.SchedulerItemID, worksEnd})
		if err != nil {
			return err
		}
		nextID := "sched_rp_journey_delay_" + hash[7:]
		fact := rpJourneyDelayEvent{JourneyID: scheduled.JourneyID, AgentID: scheduled.AgentID, OldItemID: item.SchedulerItemID, NewItemID: nextID, OldArrivalAt: item.WorldTime, NewArrivalAt: worksEnd, SegmentPlaceID: segment, WorksSourceEventIDs: worksSources}
		mutation := scheduledMutation{Private: true, EventType: "RPJourneyDelayed", EventPayload: fact, ApplyDomainRows: func(ctx context.Context, conn *sql.Conn, _ string, _ int64) error {
			if err := execAgentOne(ctx, conn, "queue obstructed journey arrival", `INSERT INTO scheduler_items(scheduler_item_id,instance_id,branch_id,world_time,phase_id,declared_priority,status,payload) VALUES (?,?,?,?,?,?,'pending',?)`, nextID, instance, branch, worksEnd, item.PhaseID, item.DeclaredPriority, string(encoded)); err != nil {
				return err
			}
			if err := execAgentOne(ctx, conn, "rebind journey arrival schedule", `UPDATE agent_schedule_entries SET scheduler_item_id=? WHERE schedule_id=? AND scheduler_item_id=? AND status='active'`, nextID, scheduled.ScheduleID, item.SchedulerItemID); err != nil {
				return err
			}
			return execAgentOne(ctx, conn, "delay active journey", `UPDATE rp_journeys SET arrival_item_id=?,scheduled_arrival_at=? WHERE journey_id=? AND arrival_item_id=? AND status='active'`, nextID, worksEnd, scheduled.JourneyID, item.SchedulerItemID)
		}}
		return s.commitScheduledMutationForBranch(ctx, tx, item, scheduledPayload{Kind: scheduled.Kind, Day: scheduled.Day}, mutation, instance, branch)
	}
	coLocated, err := rpCoLocatedEntityIDs(ctx, tx.conn, instance, branch, to, scheduled.AgentID)
	if err != nil {
		return err
	}
	fact := rpJourneyArrivalEvent{JourneyID: scheduled.JourneyID, AgentID: scheduled.AgentID, EdgeID: edgeID, FromPlaceID: segment, ToPlaceID: to, CoLocatedEntityIDs: coLocated}
	mutation := scheduledMutation{Private: true, EventType: "RPJourneyArrived", EventPayload: fact, ApplyDomainRows: func(ctx context.Context, conn *sql.Conn, eventID string, sequence int64) error {
		if err := execAgentOne(ctx, conn, "journey arrival movement", `INSERT INTO agent_movements(movement_id,event_id,agent_id,from_place_id,to_place_id,schedule_id,activity_code,world_time,movement_kind) VALUES (?,?,?,?,?,?,?,?,'scheduled')`, "movement_"+item.SchedulerItemID, eventID, scheduled.AgentID, segment, to, scheduled.ScheduleID, scheduled.ActivityCode, item.WorldTime); err != nil {
			return err
		}
		if err := execAgentOne(ctx, conn, "journey destination position", `UPDATE agent_positions SET place_id=?,activity_code=?,effective_world_time=?,projection_version=projection_version+1,last_event_sequence=? WHERE agent_id=? AND place_id=? AND projection_version=?`, to, scheduled.ActivityCode, item.WorldTime, sequence, scheduled.AgentID, segment, positionVersion); err != nil {
			return err
		}
		if err := execAgentOne(ctx, conn, "complete journey appointment", `UPDATE agent_schedule_entries SET status='completed' WHERE schedule_id=? AND scheduler_item_id=? AND status='active'`, scheduled.ScheduleID, item.SchedulerItemID); err != nil {
			return err
		}
		if err := execAgentOne(ctx, conn, "complete journey", `UPDATE rp_journeys SET status='arrived',resolved_event_id=? WHERE journey_id=? AND status='active' AND arrival_item_id=? AND start_event_id=?`, eventID, scheduled.JourneyID, item.SchedulerItemID, startEventID); err != nil {
			return err
		}
		for _, other := range coLocated {
			for _, pair := range [][2]string{{scheduled.AgentID, other}, {other, scheduled.AgentID}} {
				if err := upsertCoLocationKnowledge(ctx, conn, eventID, sequence, pair[0], pair[1], to, item.WorldTime); err != nil {
					return err
				}
			}
		}
		return nil
	}}
	return s.commitScheduledMutationForBranch(ctx, tx, item, scheduledPayload{Kind: scheduled.Kind, Day: scheduled.Day}, mutation, instance, branch)
}
