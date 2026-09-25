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

type rpJourneyQueueRow struct {
	ID       string `json:"id"`
	Instance string `json:"instance"`
	Branch   string `json:"branch"`
	At       string `json:"at"`
	Phase    string `json:"phase"`
	Priority int64  `json:"priority"`
	Status   string `json:"status"`
	Payload  string `json:"payload"`
}

type rpJourneyScheduleRow struct {
	ID       string `json:"id"`
	AgentID  string `json:"agent_id"`
	PlaceID  string `json:"place_id"`
	Activity string `json:"activity"`
	ItemID   string `json:"item_id"`
	Status   string `json:"status"`
}

func rpJourneyArrivalQueue(instance, branch string, start rpJourneyStartEvent, itemID, at string) (rpJourneyQueueRow, error) {
	when, err := time.Parse(time.RFC3339, at)
	if err != nil {
		return rpJourneyQueueRow{}, err
	}
	base := time.Date(2026, 9, 22, 0, 0, 0, 0, time.UTC)
	day := int(when.Sub(base) / (24 * time.Hour))
	if day < 0 || start.ArrivalPriority < 0 || start.ArrivalActivity == "" {
		return rpJourneyQueueRow{}, core.NewError(core.CodeProjectionDiverged, "invalid journey arrival queue source")
	}
	payload, err := core.CanonicalJSON(agentSchedulePayload{Kind: "rp_journey_arrival", Day: day, AgentID: start.AgentID, ScheduleID: start.ArrivalScheduleID, ToPlaceID: start.ToPlaceID, ActivityCode: start.ArrivalActivity, JourneyID: start.JourneyID})
	if err != nil {
		return rpJourneyQueueRow{}, err
	}
	return rpJourneyQueueRow{ID: itemID, Instance: instance, Branch: branch, At: at, Phase: m2AgentPhaseID, Priority: start.ArrivalPriority, Status: "pending", Payload: string(payload)}, nil
}

func rpJourneyExpectedQueues(ctx context.Context, q replayQuerier, instance, branch string, through int64) (map[string]rpJourneyQueueRow, map[string]rpJourneyScheduleRow, error) {
	rows, err := q.QueryContext(ctx, `SELECT event_type,payload FROM events WHERE instance_id=? AND branch_id=? AND event_sequence<=? AND event_type IN ('RPJourneyStarted','AgentJourneyStarted','RPJourneyDelayed','RPJourneyArrived','RPJourneyCancelled') ORDER BY event_sequence`, instance, branch, through)
	if err != nil {
		return nil, nil, err
	}
	queues := map[string]rpJourneyQueueRow{}
	schedules := map[string]rpJourneyScheduleRow{}
	starts := map[string]rpJourneyStartEvent{}
	current := map[string]string{}
	for rows.Next() {
		var kind, raw string
		if err := rows.Scan(&kind, &raw); err != nil {
			rows.Close()
			return nil, nil, err
		}
		switch kind {
		case "RPJourneyStarted", "AgentJourneyStarted":
			var fact rpJourneyStartEvent
			if err := json.Unmarshal([]byte(raw), &fact); err != nil {
				rows.Close()
				return nil, nil, err
			}
			if fact.JourneyID == "" || fact.ArrivalScheduleID == "" || fact.ArrivalItemID == "" || fact.AgentID == "" || fact.ToPlaceID == "" {
				rows.Close()
				return nil, nil, core.NewError(core.CodeProjectionDiverged, "invalid journey queue start")
			}
			if _, exists := starts[fact.JourneyID]; exists {
				rows.Close()
				return nil, nil, core.NewError(core.CodeProjectionDiverged, "duplicate journey queue start")
			}
			queue, err := rpJourneyArrivalQueue(instance, branch, fact, fact.ArrivalItemID, fact.ScheduledArrivalAt)
			if err != nil {
				rows.Close()
				return nil, nil, err
			}
			queues[queue.ID] = queue
			schedules[fact.ArrivalScheduleID] = rpJourneyScheduleRow{fact.ArrivalScheduleID, fact.AgentID, fact.ToPlaceID, fact.ArrivalActivity, queue.ID, "active"}
			starts[fact.JourneyID] = fact
			current[fact.JourneyID] = queue.ID
		case "RPJourneyDelayed":
			var fact rpJourneyDelayEvent
			if err := json.Unmarshal([]byte(raw), &fact); err != nil {
				rows.Close()
				return nil, nil, err
			}
			start, ok := starts[fact.JourneyID]
			if !ok || current[fact.JourneyID] != fact.OldItemID || fact.NewItemID == "" {
				rows.Close()
				return nil, nil, core.NewError(core.CodeProjectionDiverged, "invalid journey queue delay")
			}
			old := queues[fact.OldItemID]
			old.Status = "completed"
			queues[old.ID] = old
			queue, err := rpJourneyArrivalQueue(instance, branch, start, fact.NewItemID, fact.NewArrivalAt)
			if err != nil {
				rows.Close()
				return nil, nil, err
			}
			queues[queue.ID] = queue
			schedule := schedules[start.ArrivalScheduleID]
			schedule.ItemID = queue.ID
			schedules[schedule.ID] = schedule
			current[fact.JourneyID] = queue.ID
		case "RPJourneyArrived", "RPJourneyCancelled":
			var fact struct {
				JourneyID string `json:"journey_id"`
			}
			if err := json.Unmarshal([]byte(raw), &fact); err != nil {
				rows.Close()
				return nil, nil, err
			}
			start, ok := starts[fact.JourneyID]
			if !ok || current[fact.JourneyID] == "" {
				rows.Close()
				return nil, nil, core.NewError(core.CodeProjectionDiverged, "invalid journey queue resolution")
			}
			queue := queues[current[fact.JourneyID]]
			schedule := schedules[start.ArrivalScheduleID]
			queue.Status = "completed"
			schedule.Status = "completed"
			if kind == "RPJourneyCancelled" {
				queue.Status = "cancelled"
				schedule.Status = "cancelled"
			}
			queues[queue.ID] = queue
			schedules[schedule.ID] = schedule
			current[fact.JourneyID] = ""
		}
	}
	err = rows.Err()
	rows.Close()
	return queues, schedules, err
}

func rpJourneyQueueDifferences(ctx context.Context, q replayQuerier, instance, branch string, through int64) ([]ProjectionDifference, error) {
	queues, schedules, err := rpJourneyExpectedQueues(ctx, q, instance, branch, through)
	if err != nil {
		return nil, err
	}
	var differences []ProjectionDifference
	compare := func(kind, key string, want, got any, readErr error) error {
		if readErr != nil && !errors.Is(readErr, sql.ErrNoRows) {
			return readErr
		}
		expected, err := core.CanonicalJSON(want)
		if err != nil {
			return err
		}
		actual := "missing"
		if readErr == nil {
			encoded, err := core.CanonicalJSON(got)
			if err != nil {
				return err
			}
			actual = string(encoded)
		}
		if string(expected) != actual {
			differences = append(differences, ProjectionDifference{Projection: kind, Key: key, ExpectedText: string(expected), ActualText: actual})
		}
		return nil
	}
	keys := make([]string, 0, len(queues))
	for key := range queues {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		var got rpJourneyQueueRow
		err := q.QueryRowContext(ctx, `SELECT scheduler_item_id,instance_id,branch_id,world_time,phase_id,declared_priority,status,payload FROM scheduler_items WHERE scheduler_item_id=?`, key).Scan(&got.ID, &got.Instance, &got.Branch, &got.At, &got.Phase, &got.Priority, &got.Status, &got.Payload)
		if err := compare("rp_journey_queue", key, queues[key], got, err); err != nil {
			return nil, err
		}
	}
	keys = keys[:0]
	for key := range schedules {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		var got rpJourneyScheduleRow
		err := q.QueryRowContext(ctx, `SELECT schedule_id,agent_id,place_id,activity_code,scheduler_item_id,status FROM agent_schedule_entries WHERE schedule_id=?`, key).Scan(&got.ID, &got.AgentID, &got.PlaceID, &got.Activity, &got.ItemID, &got.Status)
		if err := compare("rp_journey_schedule", key, schedules[key], got, err); err != nil {
			return nil, err
		}
	}
	rows, err := q.QueryContext(ctx, `SELECT scheduler_item_id FROM scheduler_items WHERE instance_id=? AND branch_id=? AND (scheduler_item_id GLOB 'sched_rp_journey_arrive_*' OR scheduler_item_id GLOB 'sched_rp_journey_delay_*') ORDER BY scheduler_item_id`, instance, branch)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		if _, ok := queues[id]; !ok {
			differences = append(differences, ProjectionDifference{Projection: "rp_journey_queue_extra", Key: id, ExpectedText: "absent", ActualText: "unsourced queue"})
		}
	}
	err = rows.Err()
	rows.Close()
	return differences, err
}

func repairRPJourneyQueues(ctx context.Context, conn *sql.Conn, instance, branch string, differences []ProjectionDifference) error {
	for _, difference := range differences {
		switch difference.Projection {
		case "rp_journey_queue_extra":
			return core.NewError(core.CodeProjectionDiverged, "cannot infer authority for extra journey queue")
		case "rp_journey_queue":
			var row rpJourneyQueueRow
			if err := json.Unmarshal([]byte(difference.ExpectedText), &row); err != nil {
				return err
			}
			if row.ID != difference.Key || row.Instance != instance || row.Branch != branch {
				return core.NewError(core.CodeProjectionDiverged, "journey queue repair scope differs")
			}
			if err := execAgentOne(ctx, conn, "repair journey queue", `INSERT INTO scheduler_items(scheduler_item_id,instance_id,branch_id,world_time,phase_id,declared_priority,status,payload) VALUES (?,?,?,?,?,?,?,?) ON CONFLICT(scheduler_item_id) DO UPDATE SET world_time=excluded.world_time,phase_id=excluded.phase_id,declared_priority=excluded.declared_priority,status=excluded.status,payload=excluded.payload WHERE scheduler_items.instance_id=excluded.instance_id AND scheduler_items.branch_id=excluded.branch_id`, row.ID, instance, branch, row.At, row.Phase, row.Priority, row.Status, row.Payload); err != nil {
				return err
			}
		case "rp_journey_schedule":
			if difference.ActualText == "missing" {
				return core.NewError(core.CodeProjectionDiverged, "cannot infer original journey appointment definition")
			}
			var row rpJourneyScheduleRow
			if err := json.Unmarshal([]byte(difference.ExpectedText), &row); err != nil {
				return err
			}
			if row.ID != difference.Key {
				return core.NewError(core.CodeProjectionDiverged, "journey schedule repair key differs")
			}
			if err := execAgentOne(ctx, conn, "repair journey schedule pointer", `UPDATE agent_schedule_entries SET scheduler_item_id=?,status=? WHERE schedule_id=? AND agent_id=? AND place_id=? AND activity_code=? AND EXISTS (SELECT 1 FROM agent_profiles a WHERE a.agent_id=agent_schedule_entries.agent_id AND a.instance_id=? AND a.branch_id=?)`, row.ItemID, row.Status, row.ID, row.AgentID, row.PlaceID, row.Activity, instance, branch); err != nil {
				return err
			}
		}
	}
	return nil
}
