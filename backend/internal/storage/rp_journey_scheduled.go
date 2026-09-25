package storage

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"corerp.local/backend/internal/core"
)

// A pre-existing NPC appointment traverses the same timed edge as a player.
// The original schedule remains the provenance of the eventual activity;
// this scheduled Event changes actual occupancy to the segment and rebinds
// only its arrival queue pointer. No actor is placed at the destination early.
func (s *Store) beginScheduledRPJourneyIfTimed(ctx context.Context, tx *immediateTx, item SchedulerItem, scheduled agentSchedulePayload, from, to string, positionVersion int64, instance, branch string) (bool, error) {
	if from == to {
		return false, nil
	}
	var edgeID, segment string
	var minutes int
	err := tx.conn.QueryRowContext(ctx, `SELECT edge_id,segment_place_id,duration_minutes FROM rp_timed_edges
		WHERE instance_id=? AND branch_id=? AND from_place_id=? AND to_place_id=?`, instance, branch, from, to).Scan(&edgeID, &segment, &minutes)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	worksEnd, err := readRPDirectWorksEnd(ctx, tx.conn, instance, branch, from, to, item.WorldTime)
	if err != nil {
		return false, err
	}
	if worksEnd != "" {
		// The earlier RP5 delay step must have requeued any directly blocked
		// timed departure, even when an alternate durationless route exists.
		return false, core.NewError(core.CodeProjectionDiverged, "timed departure passed obstructed route without delay")
	}
	start, err := time.Parse(time.RFC3339, item.WorldTime)
	if err != nil {
		return false, err
	}
	arrival := start.Add(time.Duration(minutes) * time.Minute).UTC().Format(time.RFC3339)
	base, _ := time.Parse(time.RFC3339, "2026-09-22T00:00:00Z")
	arrivalDay := int(start.Add(time.Duration(minutes)*time.Minute).Sub(base) / (24 * time.Hour))
	if arrivalDay < scheduled.Day || arrivalDay < 0 {
		return false, core.NewError(core.CodeProjectionDiverged, "scheduled journey day is inconsistent")
	}
	keyHash, err := core.HashJSON([]string{instance, branch, item.SchedulerItemID, "timed-journey"})
	if err != nil {
		return false, err
	}
	suffix := keyHash[7:]
	journeyID, arrivalID := "journey_sched_"+suffix, "sched_rp_journey_arrive_"+suffix
	nextPayload := agentSchedulePayload{Kind: "rp_journey_arrival", Day: arrivalDay, AgentID: scheduled.AgentID, ScheduleID: scheduled.ScheduleID, ToPlaceID: to, ActivityCode: scheduled.ActivityCode, JourneyID: journeyID}
	encoded, err := core.CanonicalJSON(nextPayload)
	if err != nil {
		return false, err
	}
	coLocated, err := rpCoLocatedEntityIDs(ctx, tx.conn, instance, branch, segment, scheduled.AgentID)
	if err != nil {
		return false, err
	}
	fact := rpJourneyStartEvent{JourneyID: journeyID, AgentID: scheduled.AgentID, EdgeID: edgeID, FromPlaceID: from, ToPlaceID: to, SegmentPlaceID: segment, ScheduledArrivalAt: arrival, ArrivalScheduleID: scheduled.ScheduleID, ArrivalItemID: arrivalID, ArrivalActivity: scheduled.ActivityCode, ArrivalPriority: item.DeclaredPriority, CoLocatedEntityIDs: coLocated}
	mutation := scheduledMutation{Private: true, EventType: "AgentJourneyStarted", EventPayload: fact, ApplyDomainRows: func(ctx context.Context, conn *sql.Conn, eventID string, sequence int64) error {
		if err := execAgentOne(ctx, conn, "NPC journey segment movement", `INSERT INTO agent_movements(movement_id,event_id,agent_id,from_place_id,to_place_id,schedule_id,activity_code,world_time,movement_kind) VALUES (?,?,?,?,?,?,'rp_journey_start',?,'scheduled')`, "movement_"+item.SchedulerItemID, eventID, scheduled.AgentID, from, segment, scheduled.ScheduleID, item.WorldTime); err != nil {
			return err
		}
		if err := execAgentOne(ctx, conn, "NPC journey segment position", `UPDATE agent_positions SET place_id=?,activity_code='rp_journey_start',effective_world_time=?,projection_version=projection_version+1,last_event_sequence=? WHERE agent_id=? AND place_id=? AND projection_version=?`, segment, item.WorldTime, sequence, scheduled.AgentID, from, positionVersion); err != nil {
			return err
		}
		if err := execAgentOne(ctx, conn, "queue NPC journey arrival", `INSERT INTO scheduler_items(scheduler_item_id,instance_id,branch_id,world_time,phase_id,declared_priority,status,payload) VALUES (?,?,?,?,?,?,'pending',?)`, arrivalID, instance, branch, arrival, item.PhaseID, item.DeclaredPriority, string(encoded)); err != nil {
			return err
		}
		if err := execAgentOne(ctx, conn, "bind NPC journey arrival", `UPDATE agent_schedule_entries SET scheduler_item_id=? WHERE schedule_id=? AND scheduler_item_id=? AND status='active'`, arrivalID, scheduled.ScheduleID, item.SchedulerItemID); err != nil {
			return err
		}
		if err := execAgentOne(ctx, conn, "start NPC timed journey", `INSERT INTO rp_journeys(journey_id,instance_id,branch_id,agent_id,edge_id,from_place_id,to_place_id,segment_place_id,started_at,scheduled_arrival_at,status,start_event_id,arrival_schedule_id,arrival_item_id) VALUES (?,?,?,?,?,?,?,?,?,?,'active',?,?,?)`, journeyID, instance, branch, scheduled.AgentID, edgeID, from, to, segment, item.WorldTime, arrival, eventID, scheduled.ScheduleID, arrivalID); err != nil {
			return err
		}
		for _, other := range coLocated {
			for _, pair := range [][2]string{{scheduled.AgentID, other}, {other, scheduled.AgentID}} {
				if err := upsertCoLocationKnowledge(ctx, conn, eventID, sequence, pair[0], pair[1], segment, item.WorldTime); err != nil {
					return err
				}
			}
		}
		return nil
	}}
	if err := s.commitScheduledMutationForBranch(ctx, tx, item, scheduledPayload{Kind: scheduled.Kind, Day: scheduled.Day}, mutation, instance, branch); err != nil {
		return false, err
	}
	return true, nil
}
