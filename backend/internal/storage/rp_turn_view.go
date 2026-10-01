package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"sort"

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
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return core.RPNarrativeInput{}, err
	}
	defer tx.Rollback(ctx)
	return readRPNarrativeInputOnConn(ctx, tx.conn, sessionID, playerTurnID, playerEventID)
}

// One short snapshot gathers facts, identity masking and full-batch provenance.
// No connection survives rendering, provider calls or stream emission.
func readRPNarrativeInputOnConn(ctx context.Context, conn *sql.Conn, sessionID, playerTurnID, playerEventID string) (core.RPNarrativeInput, error) {
	return readRPNarrativeInputAtHead(ctx, conn, sessionID, playerTurnID, playerEventID, 0)
}

func readRPNarrativeInputAtHead(ctx context.Context, conn *sql.Conn, sessionID, playerTurnID, playerEventID string, head int64) (core.RPNarrativeInput, error) {
	var input core.RPNarrativeInput
	if head > 0 {
		var outside int
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_npc_decisions d JOIN events e ON e.event_id=d.event_id JOIN event_batches b ON b.batch_id=e.batch_id WHERE d.session_id=? AND d.parent_turn_id=? AND b.last_sequence>?`, sessionID, playerTurnID, head).Scan(&outside); err != nil {
			return input, err
		}
		if outside != 0 {
			return input, narrativeDiverged("turn includes NPC batches beyond historical narrative boundary")
		}
	}
	player := core.RPNarrativeFact{EventID: playerEventID, Action: "speak"}
	var utterancePlaceID string
	var triggerKind string
	if err := conn.QueryRowContext(ctx, `SELECT event_type FROM events WHERE event_id=?`, playerEventID).Scan(&triggerKind); err != nil {
		return input, classifyMissing(err, "committed RP narrative trigger")
	}
	if triggerKind == "RPNonverbalAction" {
		var raw, controlled, instance, branch string
		var sourceHead int64
		if err := conn.QueryRowContext(ctx, `SELECT e.payload,e.actor_id,n.display_name,e.world_time,p.display_name,json_extract(e.payload,'$.place_id'),s.controlled_entity_id,s.instance_id,s.branch_id,b.head_sequence
		 FROM rp_turn_runs r JOIN rp_sessions s ON s.session_id=r.session_id
		 JOIN events e ON e.event_id=r.player_event_id AND e.instance_id=s.instance_id AND e.branch_id=s.branch_id
		 JOIN materialized_entities n ON n.entity_id=e.actor_id
		 JOIN agent_places p ON p.place_id=json_extract(e.payload,'$.place_id')
		 JOIN branches b ON b.instance_id=s.instance_id AND b.branch_id=s.branch_id
		 WHERE r.session_id=? AND r.trigger_kind='nonverbal' AND r.player_turn_id IS NULL AND r.player_event_id=?`, sessionID, playerEventID).Scan(&raw, &player.ActorID, &player.ActorName, &player.WorldTime, &player.PlaceName, &utterancePlaceID, &controlled, &instance, &branch, &sourceHead); err != nil {
			return input, classifyMissing(err, "committed player action for RP narrative")
		}
		var fact core.RPNonverbalFact
		if playerTurnID != playerEventID || json.Unmarshal([]byte(raw), &fact) != nil || fact.ActorEntityID != controlled || player.ActorID != controlled || fact.SessionID != sessionID || core.RPNonverbalExpressionCode(fact.Action, fact.GestureCode) == "" || fact.Action == "look_at" && fact.TargetEntityID == "" || fact.PlaceID != utterancePlaceID {
			return input, narrativeDiverged("RP narrative action trigger differs from its owner")
		}
		if head > 0 {
			sourceHead = head
		}
		if _, err := loadRPNarrativeSource(ctx, conn, instance, branch, playerEventID, sourceHead); err != nil {
			return input, err
		}
		player.Action, player.ExpressionCode, player.TargetActorID = "expression", core.RPNonverbalExpressionCode(fact.Action, fact.GestureCode), fact.TargetEntityID
	} else if triggerKind != "RPSpeechAccepted" {
		return input, narrativeDiverged("RP narrative trigger is not accepted speech or an approved player action")
	} else if err := conn.QueryRowContext(ctx, `SELECT u.speech_text,u.speaker_entity_id,n.display_name,u.world_time,p.display_name,u.place_id
		FROM rp_utterances u JOIN materialized_entities n ON n.entity_id=u.speaker_entity_id JOIN agent_places p ON p.place_id=u.place_id
		WHERE u.session_id = ? AND u.turn_id = ? AND u.event_id = ?`, sessionID, playerTurnID, playerEventID).Scan(&player.Text, &player.ActorID, &player.ActorName, &player.WorldTime, &player.PlaceName, &utterancePlaceID); err != nil {
		return input, classifyMissing(err, "accepted player utterance for RP narrative")
	}
	input.ControlledEntityID = player.ActorID
	var playerEventSeq int64
	if err := conn.QueryRowContext(ctx, `SELECT event_sequence FROM events WHERE event_id = ?`, playerEventID).Scan(&playerEventSeq); err != nil {
		return input, classifyMissing(err, "accepted player utterance sequence")
	}
	seen := map[string]bool{playerEventID: true}
	var facts []core.RPNarrativeFact
	sequenceByEvent := map[string]int64{}
	activityAvailable, err := rpActivityContinuityTableAvailable(ctx, conn, "rp_activities")
	if err != nil {
		return input, err
	}
	activityCode, activityJoin := `''`, ``
	if activityAvailable {
		activityCode = `COALESCE(a.activity_code, '')`
		activityJoin = `LEFT JOIN rp_activities a ON a.start_event_id = d.event_id`
	}
	rows, err := conn.QueryContext(ctx, `
		SELECT d.action, e.event_type, npc.display_name, u.speech_text,e.event_id,npc.entity_id,e.world_time,`+activityCode+`,e.event_sequence
		FROM rp_npc_decisions d JOIN events e ON e.event_id = d.event_id
		JOIN materialized_entities npc ON npc.entity_id = d.npc_entity_id
		LEFT JOIN rp_utterances u ON u.event_id = d.event_id
		`+activityJoin+`
		WHERE d.session_id = ? AND d.parent_turn_id = ?
			AND (d.action NOT IN ('respond','refuse') OR EXISTS (
				SELECT 1 FROM observation_records heard
				WHERE heard.source_event_id=e.event_id AND heard.observer_agent_id=?
					AND heard.subject_agent_id=e.actor_id AND heard.claim_key='speech:' || e.event_id
					AND json_extract(heard.claim_payload,'$.claim_type')='speaker_said'
			))
		ORDER BY e.event_sequence`, sessionID, playerTurnID, player.ActorID)
	if err != nil {
		return input, core.WrapError(core.CodeStorageFailure, "read committed NPC effects for narrative", err)
	}
	for rows.Next() {
		var action, eventType, name string
		var speechText sql.NullString
		fact := core.RPNarrativeFact{PlaceName: player.PlaceName}
		var seq int64
		if err := rows.Scan(&action, &eventType, &name, &speechText, &fact.EventID, &fact.ActorID, &fact.WorldTime, &fact.ActivityCode, &seq); err != nil {
			rows.Close()
			return input, core.WrapError(core.CodeStorageFailure, "scan committed NPC effect for narrative", err)
		}
		switch action {
		case "respond":
			if eventType != "RPSpeechAccepted" || !speechText.Valid {
				rows.Close()
				return input, core.NewError(core.CodeProjectionDiverged, "NPC reply lacks accepted utterance")
			}
		case "refuse":
			if eventType != "RPSpeechAccepted" || !speechText.Valid {
				rows.Close()
				return input, core.NewError(core.CodeProjectionDiverged, "NPC refusal lacks accepted utterance")
			}
		case "silence":
			if eventType != "RPNPCDecisionRecorded" {
				rows.Close()
				return input, core.NewError(core.CodeProjectionDiverged, "NPC silence event type is inconsistent")
			}
		case "wait":
			if eventType != "RPNPCDecisionRecorded" {
				rows.Close()
				return input, core.NewError(core.CodeProjectionDiverged, "NPC wait event type is inconsistent")
			}
		case "leave":
			if eventType != "RPNPCMoved" {
				rows.Close()
				return input, core.NewError(core.CodeProjectionDiverged, "NPC leave event type is inconsistent")
			}
		case "act":
			if eventType != "AgentActivityStarted" || fact.ActivityCode == "" {
				rows.Close()
				return input, core.NewError(core.CodeProjectionDiverged, "NPC activity lacks started activity evidence")
			}
		default:
			rows.Close()
			return input, core.NewError(core.CodeProjectionDiverged, "unknown committed NPC action")
		}
		fact.Action, fact.ActorName, fact.Text = action, name, speechText.String
		seen[fact.EventID] = true
		sequenceByEvent[fact.EventID] = seq
		facts = append(facts, fact)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return input, core.WrapError(core.CodeStorageFailure, "iterate committed NPC effects for narrative", err)
	}
	if err := rows.Close(); err != nil {
		return input, err
	}
	// An expression is public only if its separate committed Event has a
	// frozen observation for this player. Private decision metadata never enters
	// this projection, even though it shares the decision's batch.
	expressionRows, err := conn.QueryContext(ctx, `
		SELECT e.event_id,e.event_sequence,e.world_time,e.actor_id,npc.display_name,
			json_extract(o.claim_payload,'$.action'),COALESCE(json_extract(o.claim_payload,'$.gesture_code'),''),
			COALESCE(json_extract(o.claim_payload,'$.target_entity_id'),'')
		FROM rp_npc_decisions d
		JOIN events speech ON speech.event_id=d.event_id
		JOIN events e ON e.batch_id=speech.batch_id AND e.event_type='RPNonverbalAction'
		JOIN observation_records o ON o.source_event_id=e.event_id AND o.observer_agent_id=?
			AND o.subject_agent_id=e.actor_id AND o.claim_key='nonverbal:' || e.event_id
			AND json_extract(o.claim_payload,'$.claim_type')='nonverbal_action'
		JOIN materialized_entities npc ON npc.entity_id=e.actor_id
		WHERE d.session_id=? AND d.parent_turn_id=? ORDER BY e.event_sequence`, player.ActorID, sessionID, playerTurnID)
	if err != nil {
		return input, core.WrapError(core.CodeStorageFailure, "read witnessed NPC expressions", err)
	}
	for expressionRows.Next() {
		var fact core.RPNarrativeFact
		var action, gesture string
		var sequence int64
		if err := expressionRows.Scan(&fact.EventID, &sequence, &fact.WorldTime, &fact.ActorID, &fact.ActorName, &action, &gesture, &fact.TargetActorID); err != nil {
			expressionRows.Close()
			return input, core.WrapError(core.CodeStorageFailure, "scan witnessed NPC expression", err)
		}
		fact.Action, fact.PlaceName, fact.ExpressionCode = "expression", player.PlaceName, action
		if action == "gesture" {
			fact.ExpressionCode = gesture
		}
		seen[fact.EventID] = true
		sequenceByEvent[fact.EventID] = sequence
		facts = append(facts, fact)
	}
	if err := expressionRows.Err(); err != nil {
		expressionRows.Close()
		return input, core.WrapError(core.CodeStorageFailure, "iterate witnessed NPC expressions", err)
	}
	if err := expressionRows.Close(); err != nil {
		return input, err
	}
	sort.SliceStable(facts, func(i, j int) bool { return sequenceByEvent[facts[i].EventID] < sequenceByEvent[facts[j].EventID] })
	var instance, branch string
	var chapterStart, openedSequence int64
	openedColumn := `0`
	openedAvailable, err := rpSessionOpenedWindowAvailable(ctx, conn)
	if err != nil {
		return input, err
	}
	if openedAvailable {
		openedColumn = `opened_sequence`
	}
	chapterColumn := `0`
	chapterAvailable, err := rpSessionChapterAvailable(ctx, conn)
	if err != nil {
		return input, err
	}
	if chapterAvailable {
		chapterColumn = `chapter_start_sequence`
	}
	if err := conn.QueryRowContext(ctx, `SELECT instance_id,branch_id,`+chapterColumn+`,`+openedColumn+` FROM rp_sessions WHERE session_id=?`, sessionID).Scan(&instance, &branch, &chapterStart, &openedSequence); err != nil {
		return input, err
	}
	if chapterAvailable {
		var declaredLatest, historicalChapter int64
		if err := conn.QueryRowContext(ctx, `SELECT COALESCE(MAX(chapter_start_sequence),0),COALESCE(MAX(CASE WHEN chapter_start_sequence<? THEN chapter_start_sequence END),0) FROM rp_session_chapter_resets WHERE session_id=?`, playerEventSeq, sessionID).Scan(&declaredLatest, &historicalChapter); err != nil {
			return input, err
		}
		if chapterStart != declaredLatest {
			return input, narrativeDiverged("chapter projection lacks its durable reset receipt")
		}
		chapterStart = historicalChapter
	}
	if err := conn.QueryRowContext(ctx, `SELECT b.head_sequence FROM rp_sessions s JOIN branches b ON b.instance_id=s.instance_id AND b.branch_id=s.branch_id WHERE s.session_id=?`, sessionID).Scan(&input.SourceHead); err != nil {
		return input, err
	}
	// Decision rows may be committed even when the player cannot see the NPC.
	// Only the witnessed expression rows and heard speech bypass this visual
	// check. Silence, activity and departure must be perceptible at commit time.
	visibleFacts := facts[:0]
	for _, fact := range facts {
		if fact.Action != "silence" && fact.Action != "wait" && fact.Action != "act" && fact.Action != "leave" {
			visibleFacts = append(visibleFacts, fact)
			continue
		}
		at := sequenceByEvent[fact.EventID]
		if fact.Action == "leave" {
			at--
		}
		visible, err := rpCanSeeAtSequence(ctx, conn, instance, branch, player.ActorID, fact.ActorID, at)
		if err != nil {
			return input, err
		}
		if visible {
			visibleFacts = append(visibleFacts, fact)
		}
	}
	facts = visibleFacts
	// Activity prose labels are declared by the studio narrative package;
	// presentation metadata, never narrative facts.
	var labels map[string]string
	if head > 0 {
		input.SourceHead = head
	}
	historicalPlace, err := rpNarrativePlaceNameAtHead(ctx, conn, instance, branch, utterancePlaceID, input.SourceHead)
	if err != nil {
		return input, err
	}
	player.PlaceName = historicalPlace
	for i := range facts {
		facts[i].PlaceName = historicalPlace
	}

	packages, err := readRPNarrativePackagesAtHead(ctx, conn, instance, branch, input.SourceHead)
	if err != nil {
		return input, err
	}
	if packages != nil && len(packages.Narrative.Content.ActivityLabels) > 0 {
		labels = packages.Narrative.Content.ActivityLabels
	}
	input.ActivityLabels = labels
	// Cross-turn window: everything the player could perceive since the
	// previous settled turn of this session — activity lifecycle, movement
	// overheard speech and frozen witnessed expressions at the utterance place. The window is bounded by
	// committed sequences and a 24 h world-time cutoff, so a settled turn
	// renders identically forever.
	var windowFloor int64
	if err := conn.QueryRowContext(ctx, `SELECT COALESCE(MAX(settled_sequence),0) FROM rp_turn_runs WHERE session_id=? AND status='settled' AND player_event_id IS NOT NULL AND settled_sequence<?`, sessionID, playerEventSeq).Scan(&windowFloor); err != nil {
		return input, core.WrapError(core.CodeStorageFailure, "read narrative window floor", err)
	}
	// A fresh session starts its narration at its immutable opening head.
	// Shared history remains visible; genuinely new offline events after open
	// remain eligible even when observation_cursor has since advanced.
	if openedSequence > windowFloor {
		windowFloor = openedSequence
	}
	if chapterStart > windowFloor {
		windowFloor = chapterStart
	}
	windowRows, err := conn.QueryContext(ctx, `
		SELECT e.event_id,e.event_type,e.actor_id,e.world_time,ent.display_name,
			COALESCE(json_extract(e.payload,'$.to_place_id'),''),
			COALESCE(json_extract(e.payload,'$.from_place_id'),''),
			COALESCE(json_extract(e.payload,'$.place_id'),''),
			COALESCE(json_extract(e.payload,'$.activity_code'),''),
			COALESCE(u.speech_text,''),COALESCE(u.place_id,''),
			COALESCE(json_extract(e.payload,'$.display_name'),''),
			COALESCE(CASE WHEN e.event_type='RPNonverbalAction' THEN json_extract(expression.claim_payload,'$.action') ELSE json_extract(e.payload,'$.action') END,''),
			COALESCE(json_extract(e.payload,'$.state'),''),COALESCE(json_extract(expression.claim_payload,'$.gesture_code'),''),
			COALESCE(json_extract(expression.claim_payload,'$.target_entity_id'),''),e.event_sequence
		FROM events e JOIN materialized_entities ent ON ent.entity_id=e.actor_id
		LEFT JOIN rp_utterances u ON u.event_id=e.event_id AND u.session_id=?
		LEFT JOIN observation_records expression ON expression.source_event_id=e.event_id AND expression.observer_agent_id=?
			AND expression.claim_key='nonverbal:' || e.event_id AND json_extract(expression.claim_payload,'$.claim_type')='nonverbal_action'
		WHERE e.instance_id=? AND e.branch_id=? AND e.event_sequence>? AND e.event_sequence<?
			AND (e.actor_id<>? OR e.event_type='RPSceneObjectStateChanged') AND datetime(e.world_time)>=datetime(?,'-24 hours')
			AND e.event_type IN ('AgentActivityStarted','AgentActivityCompleted','AgentActivityCancelled','RPNPCMoved','AgentMoved','RPSpeechAccepted','RPSceneObjectStateChanged','RPNonverbalAction')
			AND (e.event_type<>'RPSpeechAccepted' OR EXISTS (
				SELECT 1 FROM observation_records heard
				WHERE heard.source_event_id=e.event_id AND heard.observer_agent_id=?
					AND heard.subject_agent_id=e.actor_id AND heard.claim_key='speech:' || e.event_id
					AND json_extract(heard.claim_payload,'$.claim_type')='speaker_said'
			))
			AND (
				(e.event_type='AgentActivityStarted' AND json_extract(e.payload,'$.to_place_id')=?) OR
				(e.event_type IN ('AgentActivityCompleted','AgentActivityCancelled') AND json_extract(e.payload,'$.place_id')=?) OR
				(e.event_type IN ('RPNPCMoved','AgentMoved') AND (json_extract(e.payload,'$.to_place_id')=? OR json_extract(e.payload,'$.from_place_id')=?)) OR
				(e.event_type='RPSpeechAccepted' AND u.place_id=?) OR
				(e.event_type='RPSceneObjectStateChanged' AND json_extract(e.payload,'$.place_id')=? AND EXISTS (SELECT 1 FROM json_each(e.payload,'$.observer_entity_ids') WHERE value=?)) OR
				(e.event_type='RPNonverbalAction' AND expression.observation_id IS NOT NULL AND json_extract(e.payload,'$.place_id')=?
					AND (COALESCE(json_extract(e.payload,'$.target_entity_id'),'')='' OR json_extract(expression.claim_payload,'$.target_entity_id')=?)
					AND (json_extract(expression.claim_payload,'$.action') IN ('smile','nod','shake_head','turn_away','frown')
						OR (json_extract(expression.claim_payload,'$.action')='gesture' AND json_extract(expression.claim_payload,'$.gesture_code')='beckon')))
			)
		ORDER BY e.event_sequence DESC LIMIT 64`, sessionID, player.ActorID, instance, branch, windowFloor, playerEventSeq, player.ActorID, player.WorldTime, player.ActorID, utterancePlaceID, utterancePlaceID, utterancePlaceID, utterancePlaceID, utterancePlaceID, utterancePlaceID, player.ActorID, utterancePlaceID, player.ActorID)
	if err != nil {
		return input, core.WrapError(core.CodeStorageFailure, "read perceived cross-turn narrative facts", err)
	}
	window := make([]core.RPNarrativeFact, 0, 16)
	for windowRows.Next() {
		var eventType, actorID, actorName, toPlace, fromPlace, endPlace, code, speech, speechPlace, objectName, objectAction, objectState, expressionGesture string
		var sequence int64
		fact := core.RPNarrativeFact{PlaceName: player.PlaceName}
		if err := windowRows.Scan(&fact.EventID, &eventType, &actorID, &fact.WorldTime, &actorName, &toPlace, &fromPlace, &endPlace, &code, &speech, &speechPlace, &objectName, &objectAction, &objectState, &expressionGesture, &fact.TargetActorID, &sequence); err != nil {
			windowRows.Close()
			return input, core.WrapError(core.CodeStorageFailure, "scan perceived cross-turn narrative fact", err)
		}
		if seen[fact.EventID] {
			continue
		}
		fact.ActorID, fact.ActorName = actorID, actorName
		switch eventType {
		case "AgentActivityStarted":
			if toPlace != utterancePlaceID || code == "" {
				continue
			}
			fact.Action, fact.ActivityCode = "act", code
		case "AgentActivityCompleted":
			if endPlace != utterancePlaceID || code == "" {
				continue
			}
			fact.Action, fact.ActivityCode = "activity_done", code
		case "AgentActivityCancelled":
			if endPlace != utterancePlaceID || code == "" {
				continue
			}
			fact.Action, fact.ActivityCode = "activity_interrupted", code
		case "RPNPCMoved", "AgentMoved":
			switch {
			case toPlace == utterancePlaceID && fromPlace != utterancePlaceID:
				fact.Action = "arrive"
			case fromPlace == utterancePlaceID && toPlace != utterancePlaceID:
				fact.Action = "depart"
			default:
				continue
			}
		case "RPSpeechAccepted":
			if speech == "" || speechPlace != utterancePlaceID {
				continue
			}
			fact.Action, fact.Text = "speak", speech
		case "RPNonverbalAction":
			fact.Action, fact.ExpressionCode = "expression", objectAction
			if objectAction == "gesture" {
				fact.ExpressionCode = expressionGesture
			}
		case "RPSceneObjectStateChanged":
			if objectName == "" || objectState == "" {
				continue
			}
			fact.Action, fact.ObjectName, fact.ObjectState = "object_"+objectAction, objectName, objectState
		default:
			continue
		}
		if fact.Action == "act" || fact.Action == "activity_done" || fact.Action == "activity_interrupted" || fact.Action == "arrive" || fact.Action == "depart" {
			at := sequence
			if fact.Action == "depart" {
				at-- // Before the move, while the actor is still in view.
			}
			visible, err := rpCanSeeAtSequence(ctx, conn, instance, branch, player.ActorID, actorID, at)
			if err != nil {
				windowRows.Close()
				return input, err
			}
			if !visible {
				continue
			}
		}
		if label, ok := labels[fact.ActivityCode]; ok && label != "" {
			fact.ActivityLabel = label
		}
		seen[fact.EventID] = true
		window = append(window, fact)
	}
	if err := windowRows.Err(); err != nil {
		windowRows.Close()
		return input, core.WrapError(core.CodeStorageFailure, "iterate perceived cross-turn narrative facts", err)
	}
	if err := windowRows.Close(); err != nil {
		return input, err
	}
	// Oldest first: the whole window precedes this turn's dialogue.
	for i, j := 0, len(window)-1; i < j; i, j = i+1, j-1 {
		window[i], window[j] = window[j], window[i]
	}
	input.Facts = append(append(window, player), facts...)
	publicPresentations, err := readRPPublicPresentations(ctx, conn, instance, branch)
	if err != nil {
		return input, err
	}
	addedPresentation := map[string]bool{}
	for i := range input.Facts {
		fact := &input.Facts[i]
		if fact.Action == "speak" || fact.Action == "respond" || fact.Action == "refuse" {
			fact.SpeechTone, err = readRPRecordedSpeechTone(ctx, conn, fact.EventID, player.ActorID)
			if err != nil {
				return input, err
			}
		}
		// A visible target is still not necessarily an identified person. Apply
		// the same public identity policy as the acting character, including when
		// that character itself is anonymous.
		if fact.TargetActorID != "" {
			if fact.TargetActorID == player.ActorID {
				fact.TargetActorName = player.ActorName
			} else {
				known, err := rpNarrativeIdentityKnownAtHead(ctx, conn, instance, branch, player.ActorID, fact.TargetActorID, input.SourceHead)
				if err != nil {
					return input, err
				}
				if known {
					if err := conn.QueryRowContext(ctx, `SELECT display_name FROM materialized_entities WHERE entity_id=?`, fact.TargetActorID).Scan(&fact.TargetActorName); err != nil {
						return input, classifyMissing(err, "witnessed expression target")
					}
				} else {
					fact.TargetActorName = "陌生人"
					fact.TargetActorID, err = rpAnonymousEntityID(ctx, conn, instance, branch, player.ActorID, fact.TargetActorID)
					if err != nil {
						return input, err
					}
				}
			}
		}
		if fact.ActorID == player.ActorID {
			continue
		}
		known, err := rpNarrativeIdentityKnownAtHead(ctx, conn, instance, branch, player.ActorID, fact.ActorID, input.SourceHead)
		if err != nil {
			return input, err
		}
		if !known {
			fact.ActorName = "陌生人"
			fact.ActorID, err = rpAnonymousEntityID(ctx, conn, instance, branch, player.ActorID, fact.ActorID)
			if err != nil {
				return input, err
			}
			continue
		}
		if cue, exists := publicPresentations[fact.ActorID]; exists && !addedPresentation[fact.ActorID] {
			input.PublicPresentations = append(input.PublicPresentations, core.RPPublicPresentation{
				ActorID: fact.ActorID, ActorName: fact.ActorName, Text: cue.Text, SourceEventID: cue.SourceEventID,
			})
			addedPresentation[fact.ActorID] = true
		}
	}
	if err := annotateRPNarrativeCompanions(ctx, conn, instance, branch, player.ActorID, &input); err != nil {
		return input, err
	}
	return input, nil
}
