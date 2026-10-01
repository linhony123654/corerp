package storage

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"corerp.local/backend/internal/core"
)

const rpConversationFocusMaxAge = 30 * time.Minute

type rpConversationFocus struct {
	actorID, sourceEventID string
}

func rpConversationFocusAvailable(ctx context.Context, q rpQueryer) (bool, error) {
	var count int
	err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_meta WHERE schema_version=?`, RPConversationFocusSchemaVersion).Scan(&count)
	return count == 1, err
}

// Focus is a preference derived from the observer's last public exchange, not
// a relationship or an NPC intention. A committed silent continuation retains
// its pinned, personally heard speech source; silence cannot establish one.
// All source cutoffs use accepted speech; later facts and retry timing cannot
// rewrite an already-pinned plan.
func readRPConversationFocus(ctx context.Context, conn *sql.Conn, runID string) (rpConversationFocus, error) {
	var empty rpConversationFocus
	chapterColumn := `0`
	chapterAvailable, err := rpSessionChapterAvailable(ctx, conn)
	if err != nil {
		return empty, err
	}
	if chapterAvailable {
		chapterColumn = `h.chapter_start_sequence`
	}
	var instance, branch, observer, place, nowText string
	var cutoff, chapter int64
	err = conn.QueryRowContext(ctx, `SELECT h.instance_id,h.branch_id,h.controlled_entity_id,u.place_id,e.world_time,e.event_sequence,`+chapterColumn+`
		FROM rp_turn_runs r JOIN rp_sessions h ON h.session_id=r.session_id
		JOIN rp_utterances u ON u.event_id=r.player_event_id AND u.session_id=h.session_id AND u.speaker_entity_id=h.controlled_entity_id
		JOIN events e ON e.event_id=u.event_id AND e.instance_id=h.instance_id AND e.branch_id=h.branch_id AND e.actor_id=u.speaker_entity_id
		WHERE r.turn_run_id=?`, runID).Scan(&instance, &branch, &observer, &place, &nowText, &cutoff, &chapter)
	if err != nil {
		return empty, classifyMissing(err, "accepted speech for conversation focus")
	}
	return readRPConversationFocusAt(ctx, conn, instance, branch, observer, place, nowText, cutoff, chapter)
}

// Apply the public exchange policy to an already validated source snapshot.
func readRPConversationFocusAt(ctx context.Context, conn *sql.Conn, instance, branch, observer, place, nowText string, cutoff, chapter int64) (rpConversationFocus, error) {
	var empty rpConversationFocus
	now, err := time.Parse(time.RFC3339, nowText)
	if err != nil {
		return empty, core.NewError(core.CodeProjectionDiverged, "invalid conversation focus time")
	}
	var priorSession, priorTurn, priorPlace string
	var priorSequence int64
	err = conn.QueryRowContext(ctx, `SELECT u.session_id,u.turn_id,u.place_id,e.event_sequence
		FROM rp_utterances u JOIN events e ON e.event_id=u.event_id AND e.actor_id=u.speaker_entity_id
		WHERE e.instance_id=? AND e.branch_id=? AND u.speaker_entity_id=? AND e.event_type='RPSpeechAccepted'
		AND e.event_sequence<? AND e.event_sequence>? ORDER BY e.event_sequence DESC LIMIT 1`, instance, branch, observer, cutoff, chapter).Scan(&priorSession, &priorTurn, &priorPlace, &priorSequence)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return empty, err
	}

	// A personally heard initiative in this observer's wait episode can open
	// an exchange before the player speaks. Private initiative metadata is never read.
	var initiativeTrigger string
	err = conn.QueryRowContext(ctx, `SELECT json_extract(e.payload,'$.trigger_event_id')
		FROM events e JOIN rp_utterances u ON u.event_id=e.event_id
		JOIN event_batches b ON b.batch_id=e.batch_id JOIN commands c ON c.command_id=b.command_id
		JOIN rp_sessions h ON h.session_id=u.session_id AND h.instance_id=e.instance_id AND h.branch_id=e.branch_id
		WHERE e.instance_id=? AND e.branch_id=? AND e.event_type='RPSpeechAccepted' AND c.command_type='RPNPCInitiative'
		AND h.controlled_entity_id=? AND u.place_id=? AND e.event_sequence>? AND e.event_sequence<?
		AND json_extract(e.payload,'$.trigger_event_id') IS NOT NULL
		AND EXISTS (SELECT 1 FROM observation_records o WHERE o.source_event_id=e.event_id AND o.observer_agent_id=?
		 AND o.subject_agent_id=e.actor_id AND o.claim_key='speech:'||e.event_id AND json_extract(o.claim_payload,'$.claim_type')='speaker_said')
		ORDER BY e.event_sequence DESC LIMIT 1`, instance, branch, observer, place, max(priorSequence, chapter), cutoff, observer).Scan(&initiativeTrigger)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return empty, err
	}
	if initiativeTrigger == "" && (priorTurn == "" || priorPlace != place) {
		return empty, nil
	}
	var rows *sql.Rows
	if initiativeTrigger != "" {
		rows, err = conn.QueryContext(ctx, `SELECT e.actor_id,e.event_id,e.world_time,e.event_sequence,0
		 FROM events e JOIN rp_utterances u ON u.event_id=e.event_id
		 JOIN event_batches b ON b.batch_id=e.batch_id JOIN commands c ON c.command_id=b.command_id
		 JOIN rp_sessions h ON h.session_id=u.session_id AND h.instance_id=e.instance_id AND h.branch_id=e.branch_id
		 WHERE e.instance_id=? AND e.branch_id=? AND e.event_type='RPSpeechAccepted' AND c.command_type='RPNPCInitiative'
		 AND h.controlled_entity_id=? AND u.place_id=? AND json_extract(e.payload,'$.trigger_event_id')=?
		 AND e.event_sequence>? AND e.event_sequence<?
		 AND EXISTS (SELECT 1 FROM observation_records o WHERE o.source_event_id=e.event_id AND o.observer_agent_id=?
		  AND o.subject_agent_id=e.actor_id AND o.claim_key='speech:'||e.event_id AND json_extract(o.claim_payload,'$.claim_type')='speaker_said')
		 ORDER BY e.event_sequence DESC`, instance, branch, observer, place, initiativeTrigger, chapter, cutoff, observer)
	} else {
		rows, err = conn.QueryContext(ctx, `SELECT e.actor_id,e.event_id,e.world_time,e.event_sequence,
		 COALESCE((SELECT MAX(CASE a.reason_code WHEN 'direct_address' THEN 2 WHEN 'conversation_continuation' THEN 1 ELSE 0 END) FROM rp_turn_listener_activations a JOIN rp_turn_runs r ON r.turn_run_id=a.turn_run_id
		  WHERE r.session_id=d.session_id AND r.player_turn_id=d.parent_turn_id AND a.npc_entity_id=d.npc_entity_id
		  AND a.disposition='activated'),0)
		 FROM rp_npc_decisions d JOIN rp_utterances u ON u.event_id=d.event_id AND u.speaker_entity_id=d.npc_entity_id
		 JOIN events e ON e.event_id=u.event_id AND e.actor_id=d.npc_entity_id
		 WHERE d.session_id=? AND d.parent_turn_id=? AND d.action IN ('respond','refuse')
		 AND e.instance_id=? AND e.branch_id=? AND e.event_type='RPSpeechAccepted' AND u.place_id=?
		 AND e.event_sequence>? AND e.event_sequence<?
		 AND EXISTS (SELECT 1 FROM observation_records o WHERE o.source_event_id=e.event_id AND o.observer_agent_id=?
		  AND o.subject_agent_id=e.actor_id AND o.claim_key='speech:'||e.event_id AND json_extract(o.claim_payload,'$.claim_type')='speaker_said')
		 UNION ALL
		 SELECT e.actor_id,e.event_id,e.world_time,e.event_sequence,1
		 FROM rp_npc_decisions d JOIN rp_turn_runs r ON r.session_id=d.session_id AND r.player_turn_id=d.parent_turn_id
		 JOIN rp_turn_listener_activations a ON a.turn_run_id=r.turn_run_id AND a.npc_entity_id=d.npc_entity_id
		 JOIN rp_utterances u ON u.event_id=a.conversation_source_event_id AND u.speaker_entity_id=d.npc_entity_id
		 JOIN events e ON e.event_id=u.event_id AND e.actor_id=d.npc_entity_id
		 WHERE d.session_id=? AND d.parent_turn_id=? AND d.action IN ('wait','silence')
		 AND a.disposition='activated' AND a.reason_code='conversation_continuation'
		 AND e.instance_id=? AND e.branch_id=? AND e.event_type='RPSpeechAccepted' AND u.place_id=?
		 AND e.event_sequence>? AND e.event_sequence<?
		 AND EXISTS (SELECT 1 FROM observation_records o WHERE o.source_event_id=e.event_id AND o.observer_agent_id=?
		  AND o.subject_agent_id=e.actor_id AND o.claim_key='speech:'||e.event_id AND json_extract(o.claim_payload,'$.claim_type')='speaker_said')
		 ORDER BY 4 DESC`, priorSession, priorTurn, instance, branch, place, priorSequence, cutoff, observer,
			priorSession, priorTurn, instance, branch, place, chapter, priorSequence, observer)
	}
	if err != nil {
		return empty, err
	}
	type source struct {
		rpConversationFocus
		worldTime  string
		sequence   int64
		preference int
	}
	sources := map[string]source{}
	for rows.Next() {
		var item source
		if err := rows.Scan(&item.actorID, &item.sourceEventID, &item.worldTime, &item.sequence, &item.preference); err != nil {
			rows.Close()
			return empty, err
		}
		if _, exists := sources[item.actorID]; !exists {
			sources[item.actorID] = item
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return empty, err
	}
	if err := rows.Close(); err != nil {
		return empty, err
	}
	var chosen source
	preferred, preference := 0, 0
	for _, item := range sources {
		if item.preference > preference {
			chosen = item
			preference, preferred = item.preference, 1
		} else if item.preference == preference && preference > 0 {
			preferred++
		}
	}
	if preferred != 1 {
		if len(sources) != 1 {
			return empty, nil
		}
		for _, item := range sources {
			chosen = item
		}
	}
	at, err := time.Parse(time.RFC3339, chosen.worldTime)
	if err != nil {
		return empty, core.NewError(core.CodeProjectionDiverged, "invalid conversation source time")
	}
	if now.Before(at) || now.Sub(at) > rpConversationFocusMaxAge {
		return empty, nil
	}
	var moved int
	err = conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM events e
		WHERE e.instance_id=? AND e.branch_id=? AND e.event_sequence>? AND e.event_sequence<?
		AND e.event_type IN ('RPPlayerMoved','RPNPCMoved','AgentMoved','RPJourneyStarted','RPJourneyArrived')
		AND (e.actor_id=? OR (e.actor_id=? AND EXISTS (SELECT 1 FROM observation_records o
		 WHERE o.source_event_id=e.event_id AND o.observer_agent_id=? AND o.subject_agent_id=e.actor_id)))`, instance, branch, chosen.sequence, cutoff, observer, chosen.actorID, observer).Scan(&moved)
	if err != nil {
		return empty, err
	}
	if moved != 0 {
		return empty, nil
	}
	return chosen.rpConversationFocus, nil
}
