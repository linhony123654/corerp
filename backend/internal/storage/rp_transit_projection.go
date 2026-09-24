package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"

	"corerp.local/backend/internal/core"
)

type rpTransitScheduleProjection struct {
	AgentID    string `json:"agent_id"`
	WorldTime  string `json:"world_time"`
	PlaceID    string `json:"place_id"`
	Activity   string `json:"activity"`
	Priority   int    `json:"priority"`
	ItemID     string `json:"item_id"`
	Status     string `json:"status"`
	Definition string `json:"definition"`
}

// Focused replay of transit-touched queues/appointments. Original appointment
// definitions remain authoritative; accepted later arrivals/cancellations win.
func transitProjectionDifferences(ctx context.Context, q replayQuerier, instance, branch string, through int64) ([]ProjectionDifference, error) {
	rows, err := q.QueryContext(ctx, `SELECT event_sequence,payload FROM events WHERE instance_id=? AND branch_id=? AND event_sequence<=? AND event_type='AgentTravelDelayed' ORDER BY event_sequence`, instance, branch, through)
	if err != nil {
		return nil, err
	}
	queues := map[string]rpTransitQueue{}
	schedules := map[string]rpTransitScheduleProjection{}
	sequences := map[string]int64{}
	for rows.Next() {
		var sequence int64
		var raw string
		var d rpTransitDelay
		if err := rows.Scan(&sequence, &raw); err != nil {
			rows.Close()
			return nil, err
		}
		if err := json.Unmarshal([]byte(raw), &d); err != nil {
			rows.Close()
			return nil, err
		}
		if d.Previous.ID == "" || d.Retry.ID == "" || d.ScheduleID == "" || d.ScheduleSourceEventID == "" {
			rows.Close()
			return nil, core.NewError(core.CodeProjectionDiverged, "invalid transit delay provenance")
		}
		queues[d.Previous.ID] = d.Previous
		queues[d.Retry.ID] = d.Retry
		schedules[d.ScheduleID] = rpTransitScheduleProjection{AgentID: d.AgentID, WorldTime: d.OriginalWorldTime, PlaceID: d.ToPlaceID, Activity: d.ActivityCode, Priority: d.SchedulePriority, ItemID: d.Retry.ID, Status: "active", Definition: d.ScheduleSourceEventID}
		sequences[d.ScheduleID] = sequence
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	if len(queues) == 0 {
		return nil, nil
	}
	rows, err = q.QueryContext(ctx, `SELECT event_id,event_sequence,event_type,payload FROM events WHERE instance_id=? AND branch_id=? AND event_sequence<=? AND event_type IN ('AgentMoved','AgentActivityStarted','AgentTravelSuperseded','RPCareerFactRecorded','CareerEmploymentTermsActivated','CareerAggregateExitActivated') ORDER BY event_sequence`, instance, branch, through)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id, kind, raw string
		var sequence int64
		if err := rows.Scan(&id, &sequence, &kind, &raw); err != nil {
			rows.Close()
			return nil, err
		}
		if kind == "AgentMoved" || kind == "AgentActivityStarted" || kind == "AgentTravelSuperseded" {
			var move agentMovementPayload
			if err := json.Unmarshal([]byte(raw), &move); err != nil {
				rows.Close()
				return nil, err
			}
			state, ok := schedules[move.ScheduleID]
			if !ok || sequence <= sequences[move.ScheduleID] || id != "event_"+state.ItemID {
				continue
			}
			state.Status = "completed"
			if kind == "AgentTravelSuperseded" {
				state.Status = "cancelled"
			}
			schedules[move.ScheduleID] = state
			queue := queues[state.ItemID]
			queue.Status = "completed"
			queues[state.ItemID] = queue
			continue
		}
		var cancellations struct {
			CancelledSchedules []CareerCancelledSchedule `json:"cancelled_schedules"`
			Leave              *CareerLeaveFact          `json:"leave"`
			Overtime           *CareerOvertimeFact       `json:"overtime"`
		}
		if err := json.Unmarshal([]byte(raw), &cancellations); err != nil {
			rows.Close()
			return nil, err
		}
		items := cancellations.CancelledSchedules
		if cancellations.Leave != nil {
			items = append(items, cancellations.Leave.CancelledSchedules...)
		}
		if cancellations.Overtime != nil {
			items = append(items, cancellations.Overtime.CancelledSchedules...)
		}
		for _, cancel := range items {
			state, ok := schedules[cancel.ScheduleID]
			if !ok || sequence <= sequences[cancel.ScheduleID] || cancel.SchedulerItemID != state.ItemID {
				continue
			}
			state.Status = "cancelled"
			schedules[cancel.ScheduleID] = state
			queue := queues[state.ItemID]
			queue.Status = "cancelled"
			queues[state.ItemID] = queue
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	var differences []ProjectionDifference
	compare := func(projection, key string, expected, actual any, readErr error) error {
		if readErr != nil && !errors.Is(readErr, sql.ErrNoRows) {
			return readErr
		}
		want, err := core.CanonicalJSON(expected)
		if err != nil {
			return err
		}
		got, err := core.CanonicalJSON(actual)
		if err != nil {
			return err
		}
		actualText := string(got)
		if errors.Is(readErr, sql.ErrNoRows) {
			actualText = "missing"
		}
		if string(want) != actualText {
			differences = append(differences, ProjectionDifference{Projection: projection, Key: key, ExpectedText: string(want), ActualText: actualText})
		}
		return nil
	}
	keys := make([]string, 0, len(queues))
	for id := range queues {
		keys = append(keys, id)
	}
	sort.Strings(keys)
	for _, id := range keys {
		actual := rpTransitQueue{ID: id}
		err := q.QueryRowContext(ctx, `SELECT world_time,phase_id,declared_priority,payload,status FROM scheduler_items WHERE scheduler_item_id=? AND instance_id=? AND branch_id=?`, id, instance, branch).Scan(&actual.WorldTime, &actual.PhaseID, &actual.Priority, &actual.Payload, &actual.Status)
		if err := compare("transit_queue", id, queues[id], actual, err); err != nil {
			return nil, err
		}
	}
	keys = keys[:0]
	for id := range schedules {
		keys = append(keys, id)
	}
	sort.Strings(keys)
	for _, id := range keys {
		var actual rpTransitScheduleProjection
		err := q.QueryRowContext(ctx, `SELECT agent_id,world_time,place_id,activity_code,declared_priority,scheduler_item_id,status,definition_event_id FROM agent_schedule_entries WHERE schedule_id=?`, id).Scan(&actual.AgentID, &actual.WorldTime, &actual.PlaceID, &actual.Activity, &actual.Priority, &actual.ItemID, &actual.Status, &actual.Definition)
		if err := compare("transit_schedule", id, schedules[id], actual, err); err != nil {
			return nil, err
		}
	}
	return differences, nil
}

func repairTransitProjections(ctx context.Context, conn *sql.Conn, instance, branch string, differences []ProjectionDifference) error {
	for _, d := range differences {
		if d.Projection == "transit_queue" {
			var row rpTransitQueue
			if err := json.Unmarshal([]byte(d.ExpectedText), &row); err != nil {
				return err
			}
			if err := execAgentOne(ctx, conn, "repair transit queue", `INSERT INTO scheduler_items(scheduler_item_id,instance_id,branch_id,world_time,phase_id,declared_priority,status,payload) VALUES (?,?,?,?,?,?,?,?) ON CONFLICT(scheduler_item_id) DO UPDATE SET world_time=excluded.world_time,phase_id=excluded.phase_id,declared_priority=excluded.declared_priority,status=excluded.status,payload=excluded.payload WHERE scheduler_items.instance_id=excluded.instance_id AND scheduler_items.branch_id=excluded.branch_id`, row.ID, instance, branch, row.WorldTime, row.PhaseID, row.Priority, row.Status, row.Payload); err != nil {
				return err
			}
		} else if d.Projection == "transit_schedule" {
			var row rpTransitScheduleProjection
			if err := json.Unmarshal([]byte(d.ExpectedText), &row); err != nil {
				return err
			}
			if err := execAgentOne(ctx, conn, "repair transit appointment", `INSERT INTO agent_schedule_entries(schedule_id,agent_id,world_time,place_id,activity_code,declared_priority,scheduler_item_id,status,definition_event_id) VALUES (?,?,?,?,?,?,?,?,?) ON CONFLICT(schedule_id) DO UPDATE SET agent_id=excluded.agent_id,world_time=excluded.world_time,place_id=excluded.place_id,activity_code=excluded.activity_code,declared_priority=excluded.declared_priority,scheduler_item_id=excluded.scheduler_item_id,status=excluded.status,definition_event_id=excluded.definition_event_id`, d.Key, row.AgentID, row.WorldTime, row.PlaceID, row.Activity, row.Priority, row.ItemID, row.Status, row.Definition); err != nil {
				return err
			}
		}
	}
	return nil
}
