package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"corerp.local/backend/internal/core"
)

// Called only after the same builder has applied current visual perception.
// Filter before the scene limit so invisible rows cannot displace known facts.
// The existing activity sweeper does not record others' terminal witnesses;
// neither later visibility nor prior sight of a start proves that hidden end.
func readRPSceneActivityContext(ctx context.Context, conn *sql.Conn, input core.RPDecisionInput) ([]core.RPSceneActivity, error) {
	visibleIDs := make([]string, 0, len(input.VisibleEntities))
	for _, entity := range input.VisibleEntities {
		visibleIDs = append(visibleIDs, entity.EntityID)
	}
	visibleJSON, err := json.Marshal(visibleIDs)
	if err != nil {
		return nil, err
	}
	window := input.WorldTime
	if at, err := time.Parse(time.RFC3339, input.WorldTime); err == nil {
		window = at.Add(-24 * time.Hour).UTC().Format(time.RFC3339)
	}
	rows, err := conn.QueryContext(ctx, `
	 SELECT a.activity_id,a.actor_id,a.activity_code,a.status,a.last_event_sequence,
	  COALESCE(e.event_id,''),COALESCE(e.event_type,''),COALESCE(e.actor_id,''),
	  COALESCE(e.world_time,''),COALESCE(e.event_sequence,0),COALESCE(e.payload,'')
	 FROM rp_activities a LEFT JOIN events e ON e.instance_id=a.instance_id AND e.branch_id=a.branch_id
	  AND e.event_id=CASE WHEN a.status='in_progress' THEN a.start_event_id ELSE a.end_event_id END
	 WHERE a.instance_id=? AND a.branch_id=? AND a.place_id=?
	  AND (a.actor_id=? OR (a.status='in_progress' AND a.actor_id IN (SELECT value FROM json_each(?))))
	  AND (a.status='in_progress' OR COALESCE(e.world_time,a.started_world_time)>=?)
	 ORDER BY a.status='in_progress' DESC,a.actor_id=? DESC,COALESCE(e.world_time,a.started_world_time) DESC,a.activity_id
	 LIMIT 10`, input.InstanceID, input.BranchID, input.PlaceID, input.NPCEntityID, string(visibleJSON), window, input.NPCEntityID)
	if err != nil {
		return nil, core.WrapError(core.CodeStorageFailure, "read authorized scene activities", err)
	}
	defer rows.Close()
	activities := make([]core.RPSceneActivity, 0)
	for rows.Next() {
		var activity core.RPSceneActivity
		var kind, actor, payload string
		var sourceSequence, lastSequence int64
		if err := rows.Scan(&activity.ActivityID, &activity.ActorID, &activity.ActivityCode, &activity.Status, &lastSequence,
			&activity.SourceEventID, &kind, &actor, &activity.WorldTime, &sourceSequence, &payload); err != nil {
			return nil, core.WrapError(core.CodeStorageFailure, "scan authorized scene activity", err)
		}
		if sourceSequence <= 0 || sourceSequence > input.HeadSequence || sourceSequence != lastSequence || actor != activity.ActorID {
			return nil, core.NewError(core.CodeProjectionDiverged, "scene activity source does not match the context version")
		}
		if activity.Status == "in_progress" {
			var fact rpActivityStartedEvent
			if kind != "AgentActivityStarted" || json.Unmarshal([]byte(payload), &fact) != nil || fact.ActivityID != activity.ActivityID || fact.ActorID != activity.ActorID || fact.ActivityCode != activity.ActivityCode || fact.ToPlaceID != input.PlaceID || fact.StartedWorldTime != activity.WorldTime {
				return nil, core.NewError(core.CodeProjectionDiverged, "scene activity differs from its committed start")
			}
		} else {
			var fact rpActivityEndedEvent
			expectedKind := "AgentActivityCompleted"
			if activity.Status == "cancelled" {
				expectedKind = "AgentActivityCancelled"
			}
			if (activity.Status != "completed" && activity.Status != "cancelled") || kind != expectedKind || json.Unmarshal([]byte(payload), &fact) != nil || fact.ActivityID != activity.ActivityID || fact.ActorID != activity.ActorID || fact.ActivityCode != activity.ActivityCode || fact.PlaceID != input.PlaceID || fact.Outcome != activity.Status || fact.EndedWorldTime != activity.WorldTime {
				return nil, core.NewError(core.CodeProjectionDiverged, "scene activity differs from its committed outcome")
			}
		}
		activity.ObservationBasis = "own_action"
		if activity.ActorID != input.NPCEntityID {
			activity.ObservationBasis = "current_visibility"
			activity.WorldTime = input.WorldTime
		}
		activities = append(activities, activity)
	}
	if err := rows.Err(); err != nil {
		return nil, core.WrapError(core.CodeStorageFailure, "iterate authorized scene activities", err)
	}
	return activities, nil
}
