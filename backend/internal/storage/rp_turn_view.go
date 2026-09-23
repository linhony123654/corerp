package storage

import (
	"context"
	"database/sql"
	"fmt"

	"corerp.local/backend/internal/core"
)

// renderRPTurn is a view over committed facts. It neither calls a provider nor
// writes world state, and it never turns dialogue into objective truth.
func (s *Store) renderRPTurn(ctx context.Context, sessionID, playerTurnID, playerEventID string) ([]string, error) {
	var playerText string
	if err := s.db.QueryRowContext(ctx, `SELECT speech_text FROM rp_utterances WHERE session_id = ? AND turn_id = ? AND event_id = ?`, sessionID, playerTurnID, playerEventID).Scan(&playerText); err != nil {
		return nil, classifyMissing(err, "accepted player utterance for RP narrative")
	}
	lines := []string{fmt.Sprintf("你说：「%s」", playerText)}
	rows, err := s.db.QueryContext(ctx, `
		SELECT d.action, e.event_type, npc.display_name, u.speech_text
		FROM rp_npc_decisions d JOIN events e ON e.event_id = d.event_id
		JOIN materialized_entities npc ON npc.entity_id = d.npc_entity_id
		LEFT JOIN rp_utterances u ON u.event_id = d.event_id
		WHERE d.session_id = ? AND d.parent_turn_id = ? ORDER BY e.event_sequence`, sessionID, playerTurnID)
	if err != nil {
		return nil, core.WrapError(core.CodeStorageFailure, "read committed NPC effects for narrative", err)
	}
	defer rows.Close()
	for rows.Next() {
		var action, eventType, name string
		var speechText sql.NullString
		if err := rows.Scan(&action, &eventType, &name, &speechText); err != nil {
			return nil, core.WrapError(core.CodeStorageFailure, "scan committed NPC effect for narrative", err)
		}
		switch action {
		case "respond":
			if eventType != "RPSpeechAccepted" || !speechText.Valid {
				return nil, core.NewError(core.CodeProjectionDiverged, "NPC reply lacks accepted utterance")
			}
			lines = append(lines, fmt.Sprintf("%s 回应：「%s」", name, speechText.String))
		case "refuse":
			if eventType != "RPSpeechAccepted" || !speechText.Valid {
				return nil, core.NewError(core.CodeProjectionDiverged, "NPC refusal lacks accepted utterance")
			}
			lines = append(lines, fmt.Sprintf("%s 拒绝了：「%s」", name, speechText.String))
		case "silence":
			if eventType != "RPNPCDecisionRecorded" {
				return nil, core.NewError(core.CodeProjectionDiverged, "NPC silence event type is inconsistent")
			}
			lines = append(lines, name+" 保持沉默。")
		case "wait":
			if eventType != "RPNPCDecisionRecorded" {
				return nil, core.NewError(core.CodeProjectionDiverged, "NPC wait event type is inconsistent")
			}
			lines = append(lines, name+" 选择等待。")
		case "leave":
			if eventType != "RPNPCMoved" {
				return nil, core.NewError(core.CodeProjectionDiverged, "NPC leave event type is inconsistent")
			}
			lines = append(lines, name+" 离开了。")
		default:
			return nil, core.NewError(core.CodeProjectionDiverged, "unknown committed NPC action")
		}
	}
	if err := rows.Err(); err != nil {
		return nil, core.WrapError(core.CodeStorageFailure, "iterate committed NPC effects for narrative", err)
	}
	return lines, nil
}
