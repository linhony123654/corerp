package storage

import (
	"context"
	"database/sql"

	"corerp.local/backend/internal/core"
)

// renderRPTurn is a view over committed facts. It neither calls a provider nor
// writes world state, and it never turns dialogue into objective truth.
func (s *Store) renderRPTurn(ctx context.Context, sessionID, playerTurnID, playerEventID string) ([]string, error) {
	view, err := s.renderRPTurnStyled(ctx, sessionID, playerTurnID, playerEventID, core.DefaultRPStyle())
	return view.Lines, err
}

// Only immutable accepted facts enter narrative rendering. Style is an
// independent input and is never sent back through the decision/world path.
func (s *Store) renderRPTurnStyled(ctx context.Context, sessionID, playerTurnID, playerEventID string, style core.RPStyleProfile) (core.RPNarrativeView, error) {
	input, err := s.readRPNarrativeInput(ctx, sessionID, playerTurnID, playerEventID)
	if err != nil {
		return core.RPNarrativeView{}, err
	}
	input.Style = style
	return (core.DeterministicRPNarrativeProvider{}).Render(ctx, input)
}

func (s *Store) readRPNarrativeInput(ctx context.Context, sessionID, playerTurnID, playerEventID string) (core.RPNarrativeInput, error) {
	var input core.RPNarrativeInput
	player := core.RPNarrativeFact{EventID: playerEventID, Action: "speak"}
	if err := s.db.QueryRowContext(ctx, `SELECT u.speech_text,u.speaker_entity_id,n.display_name,u.world_time,p.display_name
	FROM rp_utterances u JOIN materialized_entities n ON n.entity_id=u.speaker_entity_id JOIN agent_places p ON p.place_id=u.place_id
	WHERE u.session_id = ? AND u.turn_id = ? AND u.event_id = ?`, sessionID, playerTurnID, playerEventID).Scan(&player.Text, &player.ActorID, &player.ActorName, &player.WorldTime, &player.PlaceName); err != nil {
		return input, classifyMissing(err, "accepted player utterance for RP narrative")
	}
	input.ControlledEntityID = player.ActorID
	input.Facts = []core.RPNarrativeFact{player}
	rows, err := s.db.QueryContext(ctx, `
		SELECT d.action, e.event_type, npc.display_name, u.speech_text,e.event_id,npc.entity_id,e.world_time
		FROM rp_npc_decisions d JOIN events e ON e.event_id = d.event_id
		JOIN materialized_entities npc ON npc.entity_id = d.npc_entity_id
		LEFT JOIN rp_utterances u ON u.event_id = d.event_id
		WHERE d.session_id = ? AND d.parent_turn_id = ? ORDER BY e.event_sequence`, sessionID, playerTurnID)
	if err != nil {
		return input, core.WrapError(core.CodeStorageFailure, "read committed NPC effects for narrative", err)
	}
	defer rows.Close()
	for rows.Next() {
		var action, eventType, name string
		var speechText sql.NullString
		fact := core.RPNarrativeFact{PlaceName: player.PlaceName}
		if err := rows.Scan(&action, &eventType, &name, &speechText, &fact.EventID, &fact.ActorID, &fact.WorldTime); err != nil {
			return input, core.WrapError(core.CodeStorageFailure, "scan committed NPC effect for narrative", err)
		}
		switch action {
		case "respond":
			if eventType != "RPSpeechAccepted" || !speechText.Valid {
				return input, core.NewError(core.CodeProjectionDiverged, "NPC reply lacks accepted utterance")
			}
		case "refuse":
			if eventType != "RPSpeechAccepted" || !speechText.Valid {
				return input, core.NewError(core.CodeProjectionDiverged, "NPC refusal lacks accepted utterance")
			}
		case "silence":
			if eventType != "RPNPCDecisionRecorded" {
				return input, core.NewError(core.CodeProjectionDiverged, "NPC silence event type is inconsistent")
			}
		case "wait":
			if eventType != "RPNPCDecisionRecorded" {
				return input, core.NewError(core.CodeProjectionDiverged, "NPC wait event type is inconsistent")
			}
		case "leave":
			if eventType != "RPNPCMoved" {
				return input, core.NewError(core.CodeProjectionDiverged, "NPC leave event type is inconsistent")
			}
		default:
			return input, core.NewError(core.CodeProjectionDiverged, "unknown committed NPC action")
		}
		fact.Action, fact.ActorName, fact.Text = action, name, speechText.String
		input.Facts = append(input.Facts, fact)
	}
	if err := rows.Err(); err != nil {
		return input, core.WrapError(core.CodeStorageFailure, "iterate committed NPC effects for narrative", err)
	}
	return input, nil
}
