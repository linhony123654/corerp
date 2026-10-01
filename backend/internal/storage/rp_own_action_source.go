package storage

import (
	"context"
	"database/sql"
	"encoding/json"

	"corerp.local/backend/internal/core"
)

// Decision memory is projected straight from committed Events. The legacy
// rp_own_actions table remains a transactional audit/index, but losing one of
// its rows cannot silently erase an NPC's autobiographical context. Older
// no-op decisions have no place in their Event payload and are deliberately
// omitted rather than reconstructed from today's position.
func readRPSourcedOwnActions(ctx context.Context, conn *sql.Conn, instance, branch, actor string) ([]core.RPOwnAction, error) {
	rows, err := conn.QueryContext(ctx, `
		SELECT event_id,event_type,world_time,payload FROM events
		WHERE instance_id=? AND branch_id=? AND actor_id=? AND event_type IN
		('RPSpeechAccepted','RPNPCMoved','AgentActivityStarted','AgentActivityCompleted','AgentActivityCancelled','RPNonverbalAction','RPObjectInteracted')
		ORDER BY event_sequence DESC LIMIT 120`, instance, branch, actor)
	if err != nil {
		return nil, core.WrapError(core.CodeStorageFailure, "read sourced NPC actions", err)
	}
	defer rows.Close()
	actions := make([]core.RPOwnAction, 0, 40)
	for rows.Next() {
		var id, kind, worldTime, raw string
		if err := rows.Scan(&id, &kind, &worldTime, &raw); err != nil {
			return nil, core.WrapError(core.CodeStorageFailure, "scan sourced NPC action", err)
		}
		action := core.RPOwnAction{EventID: id, WorldTime: worldTime}
		switch kind {
		case "RPSpeechAccepted":
			var fact rpSpeechEvent
			if err := json.Unmarshal([]byte(raw), &fact); err != nil || fact.SpeakerEntityID != actor || fact.PlaceID == "" || !core.ValidRPSpeechTone(fact.SpeechTone) {
				return nil, core.NewError(core.CodeProjectionDiverged, "invalid sourced NPC speech")
			}
			action.Action, action.Text, action.PlaceID = "speech", fact.Text, fact.PlaceID
			action.SpeechTone = fact.SpeechTone
		case "RPNPCMoved":
			var fact struct {
				ToPlaceID string `json:"to_place_id"`
			}
			if err := json.Unmarshal([]byte(raw), &fact); err != nil || fact.ToPlaceID == "" {
				return nil, core.NewError(core.CodeProjectionDiverged, "invalid sourced NPC movement")
			}
			action.Action, action.PlaceID = "leave", fact.ToPlaceID
		case "AgentActivityStarted":
			var fact rpActivityStartedEvent
			if err := json.Unmarshal([]byte(raw), &fact); err != nil {
				return nil, core.WrapError(core.CodeProjectionDiverged, "decode sourced NPC activity", err)
			}
			if fact.ActivityID == "" {
				continue
			} // Scheduled-agent event, not RP activity.
			if fact.ActorID != actor || fact.ToPlaceID == "" || fact.ActivityCode == "" {
				return nil, core.NewError(core.CodeProjectionDiverged, "invalid sourced NPC activity")
			}
			action.Action, action.ActivityCode, action.PlaceID, action.Status = "activity", fact.ActivityCode, fact.ToPlaceID, "in_progress"
		case "AgentActivityCompleted", "AgentActivityCancelled":
			var fact rpActivityEndedEvent
			if err := json.Unmarshal([]byte(raw), &fact); err != nil {
				return nil, core.WrapError(core.CodeProjectionDiverged, "decode sourced NPC activity end", err)
			}
			if fact.ActivityID == "" {
				continue
			}
			if fact.ActorID != actor || fact.PlaceID == "" || fact.ActivityCode == "" {
				return nil, core.NewError(core.CodeProjectionDiverged, "invalid sourced NPC activity end")
			}
			action.Action, action.ActivityCode, action.PlaceID, action.Status = "activity", fact.ActivityCode, fact.PlaceID, fact.Outcome
		case "RPNonverbalAction":
			var fact core.RPNonverbalFact
			if err := json.Unmarshal([]byte(raw), &fact); err != nil || fact.ActorEntityID != actor || fact.PlaceID == "" {
				return nil, core.NewError(core.CodeProjectionDiverged, "invalid sourced NPC nonverbal action")
			}
			action.Action, action.ActivityCode, action.Text, action.PlaceID = "nonverbal", fact.Action, fact.Description, fact.PlaceID
		case "RPObjectInteracted":
			var fact core.RPObjectEvidence
			if err := json.Unmarshal([]byte(raw), &fact); err != nil || fact.ActorEntityID != actor || fact.PlaceID == "" {
				return nil, core.NewError(core.CodeProjectionDiverged, "invalid sourced NPC object action")
			}
			action.Action, action.ActivityCode, action.Text, action.PlaceID, action.Status = fact.Action, "rp_object", fact.Description, fact.PlaceID, "completed"
		}
		actions = append(actions, action)
		if len(actions) == 40 {
			break
		}
	}
	if err := rows.Err(); err != nil {
		return nil, core.WrapError(core.CodeStorageFailure, "iterate sourced NPC actions", err)
	}
	return actions, nil
}
