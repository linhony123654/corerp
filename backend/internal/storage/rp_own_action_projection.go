package storage

import (
	"context"
	"database/sql"
	"encoding/json"

	"corerp.local/backend/internal/core"
)

type rpOwnActionProjection struct {
	AgentID      string
	EventID      string
	Action       string
	ActivityCode string
	Text         string
	PlaceID      string
	WorldTime    string
	Status       string
	LastSequence int64
}

type rpOwnActionEventPayload struct {
	ActorID          string `json:"actor_id"`
	NPCEntityID      string `json:"npc_entity_id"`
	SpeakerEntityID  string `json:"speaker_entity_id"`
	Action           string `json:"action"`
	Text             string `json:"text"`
	PlaceID          string `json:"place_id"`
	FromPlaceID      string `json:"from_place_id"`
	ToPlaceID        string `json:"to_place_id"`
	ActivityID       string `json:"activity_id"`
	ActivityCode     string `json:"activity_code"`
	StartedWorldTime string `json:"started_world_time"`
	Outcome          string `json:"outcome"`
	DisplayName      string `json:"display_name"`
	State            string `json:"state"`
}

func rpOwnActionsExpected(ctx context.Context, q replayQuerier, instance, branch string, through int64) (map[string]rpOwnActionProjection, error) {
	rows, err := q.QueryContext(ctx, `SELECT event_id,event_sequence,event_type,actor_id,world_time,payload FROM events WHERE instance_id=? AND branch_id=? AND event_sequence<=? AND event_type IN ('RPSpeechAccepted','RPNPCMoved','RPNPCDecisionRecorded','AgentActivityStarted','AgentActivityCompleted','AgentActivityCancelled','RPSceneObjectStateChanged','RPNonverbalAction','RPObjectInteracted') ORDER BY event_sequence`, instance, branch, through)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	want := map[string]rpOwnActionProjection{}
	for rows.Next() {
		var eventID, eventType, actorID, worldTime, raw string
		var sequence int64
		if err := rows.Scan(&eventID, &sequence, &eventType, &actorID, &worldTime, &raw); err != nil {
			return nil, err
		}
		var fact rpOwnActionEventPayload
		if err := json.Unmarshal([]byte(raw), &fact); err != nil {
			return nil, err
		}
		row := rpOwnActionProjection{AgentID: actorID, EventID: eventID, WorldTime: worldTime, LastSequence: sequence}
		switch eventType {
		case "RPSpeechAccepted":
			if fact.SpeakerEntityID != actorID || fact.PlaceID == "" || fact.Text == "" {
				return nil, core.NewError(core.CodeProjectionDiverged, "invalid own speech event payload")
			}
			row.Action, row.Text, row.PlaceID = "speech", fact.Text, fact.PlaceID
		case "RPNPCMoved":
			if fact.NPCEntityID != actorID || fact.Action != "leave" || fact.ToPlaceID == "" {
				return nil, core.NewError(core.CodeProjectionDiverged, "invalid own movement event payload")
			}
			row.Action, row.PlaceID = "leave", fact.ToPlaceID
		case "RPNPCDecisionRecorded":
			if fact.NPCEntityID != actorID || (fact.Action != "silence" && fact.Action != "wait") {
				return nil, core.NewError(core.CodeProjectionDiverged, "invalid own decision event payload")
			}
			row.Action, row.PlaceID = fact.Action, fact.FromPlaceID
			if row.PlaceID == "" {
				row.PlaceID = fact.PlaceID
			}
		case "AgentActivityStarted":
			if fact.ActivityID == "" {
				continue
			}
			if fact.ActivityCode == "" || (fact.ActorID != "" && fact.ActorID != actorID) || (fact.NPCEntityID != "" && fact.NPCEntityID != actorID) {
				return nil, core.NewError(core.CodeProjectionDiverged, "invalid own activity start event payload")
			}
			row.Action, row.ActivityCode, row.PlaceID, row.Status = "activity", fact.ActivityCode, fact.ToPlaceID, "in_progress"
			if row.PlaceID == "" {
				row.PlaceID = fact.PlaceID
			}
		case "RPSceneObjectStateChanged":
			if fact.Action == "" || fact.DisplayName == "" || fact.PlaceID == "" || fact.State == "" {
				return nil, core.NewError(core.CodeProjectionDiverged, "invalid own scene object event payload")
			}
			row.Action, row.Text, row.PlaceID, row.Status = "object_"+fact.Action, fact.DisplayName, fact.PlaceID, fact.State
		case "RPNonverbalAction":
			var action core.RPNonverbalFact
			if err := json.Unmarshal([]byte(raw), &action); err != nil || action.ActorEntityID != actorID || action.PlaceID == "" {
				return nil, core.NewError(core.CodeProjectionDiverged, "invalid own nonverbal event payload")
			}
			row.Action, row.ActivityCode, row.Text, row.PlaceID = "nonverbal", action.Action, action.Description, action.PlaceID
		case "RPObjectInteracted":
			var action core.RPObjectEvidence
			if err := json.Unmarshal([]byte(raw), &action); err != nil || action.ActorEntityID != actorID || action.PlaceID == "" {
				return nil, core.NewError(core.CodeProjectionDiverged, "invalid own object event payload")
			}
			row.Action, row.ActivityCode, row.Text, row.PlaceID, row.Status = action.Action, "rp_object", action.Description, action.PlaceID, "completed"
		default:
			if fact.ActivityID == "" || fact.ActorID != actorID || fact.PlaceID == "" || fact.ActivityCode == "" || fact.Outcome == "" {
				return nil, core.NewError(core.CodeProjectionDiverged, "invalid own activity terminal event payload")
			}
			row.Action, row.ActivityCode, row.PlaceID, row.Status = "activity", fact.ActivityCode, fact.PlaceID, fact.Outcome
		}
		want[actorID+"\x00"+eventID] = row
	}
	return want, rows.Err()
}

func rpOwnActionsProjectionDifferences(ctx context.Context, q replayQuerier, instance, branch string, through int64) ([]ProjectionDifference, error) {
	want, err := rpOwnActionsExpected(ctx, q, instance, branch, through)
	if err != nil {
		return nil, err
	}
	rows, err := q.QueryContext(ctx, `SELECT agent_id,event_id,action,COALESCE(activity_code,''),COALESCE(text,''),COALESCE(place_id,''),world_time,COALESCE(status,''),last_event_sequence FROM rp_own_actions WHERE instance_id=? AND branch_id=? ORDER BY agent_id,event_id`, instance, branch)
	if err != nil {
		return nil, err
	}
	got := map[string]rpOwnActionProjection{}
	for rows.Next() {
		var row rpOwnActionProjection
		if err := rows.Scan(&row.AgentID, &row.EventID, &row.Action, &row.ActivityCode, &row.Text, &row.PlaceID, &row.WorldTime, &row.Status, &row.LastSequence); err != nil {
			rows.Close()
			return nil, err
		}
		got[row.AgentID+"\x00"+row.EventID] = row
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	wantJSON, err := core.CanonicalJSON(want)
	if err != nil {
		return nil, err
	}
	gotJSON, err := core.CanonicalJSON(got)
	if err != nil {
		return nil, err
	}
	if string(wantJSON) == string(gotJSON) {
		return nil, nil
	}
	return []ProjectionDifference{{Projection: "rp_own_actions", Key: instance + ":" + branch, ExpectedText: string(wantJSON), ActualText: string(gotJSON)}}, nil
}

func repairRPOwnActionsProjection(ctx context.Context, conn *sql.Conn, instance, branch string, through int64, differences []ProjectionDifference) error {
	if len(differences) == 0 {
		return nil
	}
	want, err := rpOwnActionsExpected(ctx, conn, instance, branch, through)
	if err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, `DELETE FROM rp_own_actions WHERE instance_id=? AND branch_id=?`, instance, branch); err != nil {
		return err
	}
	for _, key := range sortedKeys(want) {
		row := want[key]
		if _, err := conn.ExecContext(ctx, `INSERT INTO rp_own_actions(agent_id,event_id,action,activity_code,text,place_id,world_time,status,instance_id,branch_id,last_event_sequence) VALUES (?,?,?,?,?,?,?,?,?,?,?)`, row.AgentID, row.EventID, row.Action, nullable(row.ActivityCode), nullable(row.Text), nullable(row.PlaceID), row.WorldTime, nullable(row.Status), instance, branch, row.LastSequence); err != nil {
			return err
		}
	}
	return nil
}
