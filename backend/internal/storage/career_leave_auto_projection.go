package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"corerp.local/backend/internal/core"
)

// Derive review queues from accepted requests and their one terminal scheduled
// Event. Rebuild does not invoke a review or invent a fresh review deadline.
func careerLeaveQueueDifferences(ctx context.Context, q replayQuerier, instance, branch string, through int64) ([]ProjectionDifference, error) {
	rows, err := q.QueryContext(ctx, `SELECT e.event_id,e.payload,
	 EXISTS (SELECT 1 FROM events n WHERE n.instance_id=e.instance_id AND n.branch_id=e.branch_id AND n.event_sequence<=?
	 AND n.event_id='event_'||json_extract(e.payload,'$.leave.auto_review.scheduler_item_id')
	 AND ((n.event_type='RPCareerFactRecorded' AND json_extract(n.payload,'$.leave.request_event_id')=e.event_id)
	 OR (n.event_type='CareerLeaveAutoReviewSkipped' AND json_extract(n.payload,'$.request_event_id')=e.event_id)))
	 FROM events e WHERE e.instance_id=? AND e.branch_id=? AND e.event_sequence<=? AND e.event_type='RPCareerFactRecorded'
	 AND json_extract(e.payload,'$.kind')='leave' AND json_extract(e.payload,'$.leave.status')='requested'
	 AND json_extract(e.payload,'$.leave.auto_review') IS NOT NULL ORDER BY e.event_sequence`, through, instance, branch, through)
	if err != nil {
		return nil, err
	}
	var expected []SchedulerItem
	for rows.Next() {
		var source, raw string
		var complete bool
		if err := rows.Scan(&source, &raw, &complete); err != nil {
			rows.Close()
			return nil, err
		}
		var fact CareerFact
		if err := json.Unmarshal([]byte(raw), &fact); err != nil {
			rows.Close()
			return nil, err
		}
		if fact.Leave == nil || fact.Leave.AutoReview == nil || fact.Leave.RequestEventID != source || fact.Leave.AutoReview.SchedulerItemID != "career_leave_review_"+source {
			rows.Close()
			return nil, core.NewError(core.CodeProjectionDiverged, "invalid automatic review queue source")
		}
		plan := fact.Leave.AutoReview
		payload, err := core.CanonicalJSON(careerLeaveReviewPayload(source))
		if err != nil {
			rows.Close()
			return nil, err
		}
		status := "pending"
		if complete {
			status = "completed"
		}
		expected = append(expected, SchedulerItem{SchedulerItemID: plan.SchedulerItemID, WorldTime: plan.WorldTime, PhaseID: careerLeaveReviewPhase, Status: status, Payload: string(payload)})
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	var differences []ProjectionDifference
	for _, want := range expected {
		actual := SchedulerItem{SchedulerItemID: want.SchedulerItemID}
		err := q.QueryRowContext(ctx, `SELECT world_time,phase_id,declared_priority,status,payload FROM scheduler_items WHERE scheduler_item_id=? AND instance_id=? AND branch_id=?`, want.SchedulerItemID, instance, branch).Scan(&actual.WorldTime, &actual.PhaseID, &actual.DeclaredPriority, &actual.Status, &actual.Payload)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		wantJSON, encodeErr := core.CanonicalJSON(want)
		if encodeErr != nil {
			return nil, encodeErr
		}
		gotJSON, encodeErr := core.CanonicalJSON(actual)
		if encodeErr != nil {
			return nil, encodeErr
		}
		got := string(gotJSON)
		if errors.Is(err, sql.ErrNoRows) {
			got = "missing"
		}
		if string(wantJSON) != got {
			differences = append(differences, ProjectionDifference{Projection: "career_leave_review_queue", Key: want.SchedulerItemID, ExpectedText: string(wantJSON), ActualText: got})
		}
	}
	return differences, nil
}

func repairCareerLeaveQueues(ctx context.Context, conn *sql.Conn, instance, branch string, differences []ProjectionDifference) error {
	for _, d := range differences {
		var row SchedulerItem
		if err := json.Unmarshal([]byte(d.ExpectedText), &row); err != nil {
			return err
		}
		if err := execAgentOne(ctx, conn, "repair automatic leave review queue", `INSERT INTO scheduler_items(scheduler_item_id,instance_id,branch_id,world_time,phase_id,declared_priority,status,payload) VALUES (?,?,?,?,?,?,?,?) ON CONFLICT(scheduler_item_id) DO UPDATE SET world_time=excluded.world_time,phase_id=excluded.phase_id,declared_priority=excluded.declared_priority,status=excluded.status,payload=excluded.payload WHERE scheduler_items.instance_id=excluded.instance_id AND scheduler_items.branch_id=excluded.branch_id`, row.SchedulerItemID, instance, branch, row.WorldTime, row.PhaseID, row.DeclaredPriority, row.Status, row.Payload); err != nil {
			return err
		}
	}
	return nil
}
