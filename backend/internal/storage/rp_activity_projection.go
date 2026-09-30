package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"corerp.local/backend/internal/core"
)

// rpActivitiesExpected rebuilds the activity lifecycle projection from committed
// events only: every AgentActivityStarted yields an in_progress row (started
// fields pinned to the start event), and the matching terminal event — the
// deterministic sweeper's completion or cancellation — updates status,
// end_event_id, and last_event_sequence. Terminal rows therefore never depend
// on when a sweep happened to run.
func rpActivitiesExpected(ctx context.Context, q replayQuerier, instance, branch string, through int64) (map[string]rpActivityProjection, error) {
	rows, err := q.QueryContext(ctx, `SELECT e.event_id,e.event_sequence,e.event_type,e.actor_id,e.world_time,e.payload FROM events e WHERE e.instance_id=? AND e.branch_id=? AND e.event_sequence<=? AND e.event_type IN ('AgentActivityStarted','AgentActivityCompleted','AgentActivityCancelled') ORDER BY e.event_sequence`, instance, branch, through)
	if err != nil {
		return nil, err
	}
	want := map[string]rpActivityProjection{}
	defer rows.Close()
	for rows.Next() {
		var id, kind, actor, worldTime, raw string
		var sequence int64
		if err := rows.Scan(&id, &sequence, &kind, &actor, &worldTime, &raw); err != nil {
			return nil, err
		}
		switch kind {
		case "AgentActivityStarted":
			var fact rpActivityStartedEvent
			if err := json.Unmarshal([]byte(raw), &fact); err != nil {
				return nil, err
			}
			if fact.ActivityID == "" {
				// Scheduled-agent lifecycle events reuse this type with a
				// movement payload and no activity id; they are not RP
				// activities and never produce rp_activities rows.
				continue
			}
			if fact.ActorID != actor || fact.ToPlaceID == "" || fact.ActivityCode == "" || fact.StartedWorldTime == "" || fact.DurationMinutes < 1 {
				return nil, core.NewError(core.CodeProjectionDiverged, "invalid activity start event payload")
			}
			want[fact.ActivityID] = rpActivityProjection{ActivityID: fact.ActivityID, ActorID: fact.ActorID, PlaceID: fact.ToPlaceID, ActivityCode: fact.ActivityCode, StartedWorldTime: fact.StartedWorldTime, DurationMinutes: fact.DurationMinutes, Status: "in_progress", StartEventID: id, LastSequence: sequence}
		default:
			var fact rpActivityEndedEvent
			if err := json.Unmarshal([]byte(raw), &fact); err != nil {
				return nil, err
			}
			if fact.ActivityID == "" {
				continue
			}
			outcome := "completed"
			if kind == "AgentActivityCancelled" {
				outcome = "cancelled"
			}
			if fact.ActivityID == "" || fact.ActorID != actor || fact.Outcome != outcome {
				return nil, core.NewError(core.CodeProjectionDiverged, "invalid activity terminal event payload")
			}
			row, ok := want[fact.ActivityID]
			if !ok {
				return nil, core.NewError(core.CodeProjectionDiverged, "activity terminal event without start event")
			}
			if fact.ActorID != row.ActorID || fact.PlaceID != row.PlaceID || fact.ActivityCode != row.ActivityCode || fact.StartedWorldTime != row.StartedWorldTime {
				return nil, core.NewError(core.CodeProjectionDiverged, "activity terminal event contradicts start event")
			}
			row.Status, row.EndEventID, row.LastSequence = outcome, id, sequence
			want[fact.ActivityID] = row
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return want, nil
}

type rpActivityProjection struct {
	ActivityID       string
	ActorID          string
	PlaceID          string
	ActivityCode     string
	StartedWorldTime string
	DurationMinutes  int
	Status           string
	StartEventID     string
	EndEventID       string
	LastSequence     int64
}

func rpActivitiesProjectionDifferences(ctx context.Context, q replayQuerier, instance, branch string, through int64) ([]ProjectionDifference, error) {
	want, err := rpActivitiesExpected(ctx, q, instance, branch, through)
	if err != nil {
		return nil, err
	}
	rows, err := q.QueryContext(ctx, `SELECT activity_id,actor_id,place_id,activity_code,started_world_time,duration_minutes,status,start_event_id,end_event_id,last_event_sequence FROM rp_activities WHERE instance_id=? AND branch_id=? ORDER BY activity_id`, instance, branch)
	if err != nil {
		return nil, err
	}
	got := map[string]rpActivityProjection{}
	defer rows.Close()
	for rows.Next() {
		var row rpActivityProjection
		var end sql.NullString
		if err := rows.Scan(&row.ActivityID, &row.ActorID, &row.PlaceID, &row.ActivityCode, &row.StartedWorldTime, &row.DurationMinutes, &row.Status, &row.StartEventID, &end, &row.LastSequence); err != nil {
			return nil, err
		}
		if end.Valid {
			row.EndEventID = end.String
		}
		got[row.ActivityID] = row
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	var differences []ProjectionDifference
	for _, id := range sortedKeys(want) {
		expected := want[id]
		expectedJSON, err := core.CanonicalJSON(expected)
		if err != nil {
			return nil, err
		}
		actualRow, ok := got[id]
		actual := "missing"
		if ok {
			actualJSON, err := core.CanonicalJSON(rpActivityProjection{ActivityID: actualRow.ActivityID, ActorID: actualRow.ActorID, PlaceID: actualRow.PlaceID, ActivityCode: actualRow.ActivityCode, StartedWorldTime: actualRow.StartedWorldTime, DurationMinutes: actualRow.DurationMinutes, Status: actualRow.Status, StartEventID: actualRow.StartEventID, EndEventID: actualRow.EndEventID, LastSequence: actualRow.LastSequence})
			if err != nil {
				return nil, err
			}
			actual = string(actualJSON)
		}
		if actual == "missing" || actual != string(expectedJSON) {
			kind := "rp_activities"
			if ok && !rpActivityStartedFieldsEqual(expected, actualRow) {
				// Terminal status drift is reparable (the sweeper's update is
				// derived from the terminal event); started-field drift is not.
				kind = "rp_activities_history"
			}
			differences = append(differences, ProjectionDifference{Projection: kind, Key: id, ExpectedText: string(expectedJSON), ActualText: actual})
		}
	}
	for _, id := range sortedKeys(got) {
		if _, ok := want[id]; !ok {
			differences = append(differences, ProjectionDifference{Projection: "rp_activities_extra", Key: id, ExpectedText: "absent", ActualText: "unsourced projection row"})
		}
	}
	return differences, nil
}

func rpActivityStartedFieldsEqual(want, got rpActivityProjection) bool {
	return want.ActorID == got.ActorID && want.PlaceID == got.PlaceID && want.ActivityCode == got.ActivityCode && want.StartedWorldTime == got.StartedWorldTime && want.DurationMinutes == got.DurationMinutes && want.StartEventID == got.StartEventID
}

// repairRPActivitiesProjections restores rows the events fully determine.
// Missing rows are reinserted from the expected state; terminal drift
// (status/end/sequence) is rewritten to match the terminal event. It refuses
// to touch started fields or unsourced rows: those divergences need a human.
func repairRPActivitiesProjections(ctx context.Context, conn *sql.Conn, instance, branch string, differences []ProjectionDifference) error {
	for _, difference := range differences {
		switch difference.Projection {
		case "rp_activities":
			var want rpActivityProjection
			if err := json.Unmarshal([]byte(difference.ExpectedText), &want); err != nil {
				return err
			}
			if want.ActivityID != difference.Key {
				return core.NewError(core.CodeProjectionDiverged, "activity repair scope differs")
			}
			if difference.ActualText == "missing" {
				if err := execAgentOne(ctx, conn, "repair RP activity row", `INSERT INTO rp_activities(activity_id,actor_id,place_id,activity_code,started_world_time,duration_minutes,status,start_event_id,end_event_id,instance_id,branch_id,last_event_sequence) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`, want.ActivityID, want.ActorID, want.PlaceID, want.ActivityCode, want.StartedWorldTime, want.DurationMinutes, want.Status, want.StartEventID, nullable(want.EndEventID), instance, branch, want.LastSequence); err != nil {
					return err
				}
				continue
			}
			var got rpActivityProjection
			if err := json.Unmarshal([]byte(difference.ActualText), &got); err != nil {
				return err
			}
			if !rpActivityStartedFieldsEqual(want, got) {
				return core.NewError(core.CodeProjectionDiverged, "cannot rewrite activity history")
			}
			if err := execAgentOne(ctx, conn, "repair RP activity terminal state", `UPDATE rp_activities SET status=?,end_event_id=?,last_event_sequence=? WHERE activity_id=? AND instance_id=? AND branch_id=?`, want.Status, nullable(want.EndEventID), want.LastSequence, want.ActivityID, instance, branch); err != nil {
				return err
			}
		case "rp_activities_history", "rp_activities_extra":
			return core.NewError(core.CodeProjectionDiverged, fmt.Sprintf("cannot repair %s for activity %s", difference.Projection, difference.Key))
		}
	}
	return nil
}
