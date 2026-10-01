package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"corerp.local/backend/internal/core"
)

type RPDecisionResult struct {
	ReasonCode        string                  `json:"reason_code,omitempty"`
	ProviderErrorCode string                  `json:"provider_error_code,omitempty"`
	NPCEntityID       string                  `json:"npc_entity_id"`
	TurnID            string                  `json:"turn_id"`
	HeadSequence      int64                   `json:"head_sequence"`
	InputHash         string                  `json:"input_hash"`
	Proposal          core.RPDecisionProposal `json:"proposal"`
	Status            string                  `json:"status"`
}

// BuildRPDecisionInput is deliberately narrower than the world's state. It
// requires personally received evidence for the active speech or action turn.
func (s *Store) BuildRPDecisionInput(ctx context.Context, request core.RPDecisionRequest) (core.RPDecisionInput, error) {
	if err := request.Validate(); err != nil {
		return core.RPDecisionInput{}, err
	}
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return core.RPDecisionInput{}, core.WrapError(core.CodeStorageFailure, "begin NPC decision observation", err)
	}
	defer tx.Rollback(ctx)
	session, err := loadRPSession(ctx, tx.conn, request.PrincipalID, request.SessionID)
	if err != nil {
		return core.RPDecisionInput{}, err
	}
	if err := authorizeRPControl(ctx, tx.conn, request.PrincipalID, session.InstanceID, session.BranchID, session.ControlledEntityID); err != nil {
		return core.RPDecisionInput{}, err
	}
	if session.Status != "active" || session.TurnCursor != request.TurnID || (session.TurnState != "speech_committed" && session.TurnState != "action_committed" && session.TurnState != "npc_effects_committed") {
		return core.RPDecisionInput{}, core.NewError(core.CodeBranchConflict, "NPC decision requires the active committed speech or action turn")
	}
	if request.NPCEntityID == session.ControlledEntityID {
		return core.RPDecisionInput{}, core.NewError(core.CodeInvalidArgument, "player cannot be selected as NPC")
	}
	input := core.RPDecisionInput{
		ContextVersion: core.RPContextVersion,
		InstanceID:     session.InstanceID, BranchID: session.BranchID,
		TurnID: request.TurnID, NPCEntityID: request.NPCEntityID,
		VisibleEntities:   make([]core.RPDecisionVisibleEntity, 0),
		Knowledge:         make([]core.RPDecisionKnowledge, 0),
		ReachablePlaceIDs: make([]string, 0),
		LegalActions:      []string{"respond", "refuse", "silence", "wait"},
	}
	var parentType string
	err = tx.conn.QueryRowContext(ctx, `SELECT event_type FROM events WHERE event_id=? AND instance_id=? AND branch_id=?`, request.TurnID, session.InstanceID, session.BranchID).Scan(&parentType)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return core.RPDecisionInput{}, core.WrapError(core.CodeStorageFailure, "read decision parent kind", err)
	}
	if session.TurnState == "action_committed" || parentType == "RPNonverbalAction" {
		observed, err := readRPDecisionActionTrigger(ctx, tx.conn, session, request.TurnID, request.NPCEntityID)
		if err != nil {
			return core.RPDecisionInput{}, err
		}
		input.ObservedPlayerAction = observed
		input.Trigger = &core.RPDecisionTrigger{Kind: "nonverbal", SourceEventID: observed.SourceEventID}
		input.InterlocutorEntityID = observed.ActorEntityID
		input, err = readRPOwnDecisionContext(ctx, tx.conn, input)
		if err != nil {
			return core.RPDecisionInput{}, err
		}
		var playerPlace string
		if err := tx.conn.QueryRowContext(ctx, `SELECT place_id FROM agent_positions WHERE agent_id=?`, session.ControlledEntityID).Scan(&playerPlace); err != nil {
			return core.RPDecisionInput{}, classifyMissing(err, "current action actor position")
		}
		if input.PlaceID != observed.PlaceID || playerPlace != input.PlaceID {
			return core.RPDecisionInput{}, core.NewError(core.CodeBranchConflict, "NPC or player has left the action scene")
		}
		return core.SelectRPDecisionContext(input, core.DefaultRPDecisionContextBudgetBytes)
	}
	var speakerID, speechPlace string
	err = tx.conn.QueryRowContext(ctx, `
		SELECT u.event_id, u.speaker_entity_id, u.place_id, u.speech_text, u.world_time
		FROM rp_utterances u JOIN events e ON e.event_id = u.event_id
		WHERE u.session_id = ? AND u.turn_id = ? AND e.instance_id = ? AND e.branch_id = ?`,
		session.SessionID, request.TurnID, session.InstanceID, session.BranchID,
	).Scan(&input.SpeechEventID, &speakerID, &speechPlace, &input.PlayerSpeechText, &input.PlayerSpeechWorldTime)
	if err != nil {
		return core.RPDecisionInput{}, classifyMissing(err, "committed player speech")
	}
	if speakerID != session.ControlledEntityID {
		return core.RPDecisionInput{}, core.NewError(core.CodeProjectionDiverged, "turn speaker differs from session control")
	}
	var heard int
	input.InterlocutorEntityID = speakerID
	if err := tx.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM observation_records o JOIN agent_knowledge k ON k.observation_id = o.observation_id WHERE o.source_event_id = ? AND o.observer_agent_id = ? AND o.subject_agent_id = ? AND k.claim_key = 'speech:' || ?`, input.SpeechEventID, request.NPCEntityID, speakerID, input.SpeechEventID).Scan(&heard); err != nil {
		return core.RPDecisionInput{}, core.WrapError(core.CodeStorageFailure, "check NPC hearing evidence", err)
	}
	if heard != 1 {
		return core.RPDecisionInput{}, core.NewError(core.CodeNotFound, "NPC did not hear this speech")
	}
	input, err = readRPOwnDecisionContext(ctx, tx.conn, input)
	if err != nil {
		return core.RPDecisionInput{}, err
	}
	if input.PlaceID != speechPlace {
		return core.RPDecisionInput{}, core.NewError(core.CodeBranchConflict, "NPC has left the speech scene")
	}
	var playerPlace string
	if err := tx.conn.QueryRowContext(ctx, `SELECT p.place_id FROM agent_positions p JOIN agent_profiles a ON a.agent_id = p.agent_id WHERE a.agent_id = ? AND a.instance_id = ? AND a.branch_id = ? AND a.status = 'active'`, session.ControlledEntityID, session.InstanceID, session.BranchID).Scan(&playerPlace); err != nil {
		return core.RPDecisionInput{}, classifyMissing(err, "current player position")
	}
	if playerPlace != input.PlaceID {
		return core.RPDecisionInput{}, core.NewError(core.CodeBranchConflict, "player has left the speech scene")
	}
	return core.SelectRPDecisionContext(input, core.DefaultRPDecisionContextBudgetBytes)
}

// Scene objects are observed state, not NPC action authority or body posture.
// Compare with the existing event replay at this head before exposing the
// projection: a later state must never be attached to an earlier snapshot.
func readRPDecisionSceneObjects(ctx context.Context, conn *sql.Conn, input core.RPDecisionInput) ([]core.RPDecisionSceneObject, error) {
	available, err := rpSceneObjectProjectionAvailable(ctx, conn)
	if err != nil || !available {
		return nil, err
	}
	want, err := rpSceneObjectsExpected(ctx, conn, input.InstanceID, input.BranchID, input.HeadSequence)
	if err != nil {
		return nil, err
	}
	rows, err := conn.QueryContext(ctx, `SELECT object_id,object_key,display_name,object_kind,place_id,state_code,definition_event_id,state_event_id,projection_version,last_event_sequence FROM rp_scene_objects WHERE instance_id=? AND branch_id=? AND place_id=? ORDER BY object_key`, input.InstanceID, input.BranchID, input.PlaceID)
	if err != nil {
		return nil, core.WrapError(core.CodeStorageFailure, "read NPC scene objects", err)
	}
	defer rows.Close()
	objects := make([]core.RPDecisionSceneObject, 0)
	for rows.Next() {
		var got rpSceneObjectProjection
		if err := rows.Scan(&got.ObjectID, &got.ObjectKey, &got.DisplayName, &got.ObjectKind, &got.PlaceID, &got.State, &got.DefinitionEvent, &got.StateEvent, &got.ProjectionVersion, &got.LastSequence); err != nil {
			return nil, core.WrapError(core.CodeStorageFailure, "scan NPC scene object", err)
		}
		expected, ok := want[got.ObjectID]
		if !ok || !sceneObjectDefinitionEqual(got, expected) || got.State != expected.State || got.StateEvent != expected.StateEvent || got.LastSequence != expected.LastSequence || got.ProjectionVersion != expected.ProjectionVersion {
			return nil, core.NewError(core.CodeProjectionDiverged, "NPC scene object differs from its source head")
		}
		objects = append(objects, core.RPDecisionSceneObject{ObjectID: got.ObjectID, DisplayName: got.DisplayName, Kind: got.ObjectKind, State: got.State, PlaceID: got.PlaceID, WorldTime: input.WorldTime, DefinitionSourceEventID: got.DefinitionEvent, StateSourceEventID: got.StateEvent})
		delete(want, got.ObjectID)
	}
	if err := rows.Err(); err != nil {
		return nil, core.WrapError(core.CodeStorageFailure, "iterate NPC scene objects", err)
	}
	for _, missing := range want {
		if missing.PlaceID == input.PlaceID {
			return nil, core.NewError(core.CodeProjectionDiverged, "NPC scene object projection is missing")
		}
	}
	return objects, nil
}

// readRPOwnDecisionContext shares only own/visible evidence. Callers must first
// authorize the actor and establish their distinct speech or time trigger.
func readRPOwnDecisionContext(ctx context.Context, conn *sql.Conn, input core.RPDecisionInput) (core.RPDecisionInput, error) {
	err := conn.QueryRowContext(ctx, `
		SELECT b.head_sequence, c.current_world_time, e.display_name, p.place_id, l.display_name, p.activity_code,
		       a.goal_code, a.persona_text, a.definition_event_id, balances.balance_minor, e.currency_id
		FROM agent_profiles a JOIN materialized_entities e ON e.entity_id = a.agent_id
		JOIN agent_positions p ON p.agent_id = a.agent_id
		JOIN agent_places l ON l.place_id = p.place_id
		JOIN account_balances balances ON balances.account_id = e.asset_account_id
		JOIN branches b ON b.instance_id = a.instance_id AND b.branch_id = a.branch_id
		JOIN world_clocks c ON c.instance_id = b.instance_id AND c.branch_id = b.branch_id
		WHERE a.agent_id = ? AND a.instance_id = ? AND a.branch_id = ?
		  AND a.status = 'active' AND e.status = 'active' AND e.population_count = 1 AND l.status = 'active'`,
		input.NPCEntityID, input.InstanceID, input.BranchID,
	).Scan(&input.HeadSequence, &input.WorldTime, &input.NPCName, &input.PlaceID, &input.PlaceName,
		&input.ActivityCode, &input.GoalCode, &input.Persona, &input.PersonaSourceEventID, &input.OwnAssetMinor, &input.CurrencyID)
	if err != nil {
		return core.RPDecisionInput{}, classifyMissing(err, "active NPC decision state")
	}
	input.ContextVersion = core.RPContextVersion
	input.Relationships, err = readRPAuthoredRelationships(ctx, conn, input)
	if err != nil {
		return core.RPDecisionInput{}, err
	}
	input.Readiness = core.RPContextReadiness{Persona: "MISSING", RelationshipToInterlocutor: "UNKNOWN", AddressToInterlocutor: "UNKNOWN"}
	if strings.TrimSpace(input.Persona) != "" {
		input.Readiness.Persona = "READY"
	} else {
		input.PersonaSourceEventID = ""
	}
	for _, relation := range input.Relationships {
		if relation.SubjectEntityID != input.InterlocutorEntityID {
			continue
		}
		input.Readiness.RelationshipToInterlocutor = "READY"
		input.Readiness.AddressToInterlocutor = "MISSING"
		if len(relation.AddressTo) != 0 {
			input.Readiness.AddressToInterlocutor = "READY"
		}
		break
	}
	rows, err := conn.QueryContext(ctx, `
		SELECT e.entity_id, e.display_name FROM agent_profiles a
		JOIN agent_positions p ON p.agent_id = a.agent_id
		JOIN materialized_entities e ON e.entity_id = a.agent_id
		WHERE a.instance_id = ? AND a.branch_id = ? AND a.status = 'active' AND e.status = 'active'
		  AND p.place_id = ? AND a.agent_id <> ? ORDER BY e.entity_id`,
		input.InstanceID, input.BranchID, input.PlaceID, input.NPCEntityID)
	if err != nil {
		return core.RPDecisionInput{}, core.WrapError(core.CodeStorageFailure, "read NPC visible entities", err)
	}
	for rows.Next() {
		var visible core.RPDecisionVisibleEntity
		if err := rows.Scan(&visible.EntityID, &visible.DisplayName); err != nil {
			rows.Close()
			return core.RPDecisionInput{}, core.WrapError(core.CodeStorageFailure, "scan NPC visible entity", err)
		}
		input.VisibleEntities = append(input.VisibleEntities, visible)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return core.RPDecisionInput{}, core.WrapError(core.CodeStorageFailure, "iterate NPC visible entities", err)
	}
	rows.Close()
	visibleEntities := input.VisibleEntities[:0]
	for _, entity := range input.VisibleEntities {
		visible, err := rpCanPerceive(ctx, conn, input.InstanceID, input.BranchID, input.NPCEntityID, entity.EntityID, "visual", "")
		if err != nil {
			return core.RPDecisionInput{}, err
		}
		if visible {
			visibleEntities = append(visibleEntities, entity)
		}
	}
	input.VisibleEntities = visibleEntities
	// Include only accepted words the NPC spoke or has hearing evidence for.
	// A branch-scoped Event sequence fixes the order across player and NPC turns.
	rows, err = conn.QueryContext(ctx, `
		SELECT u.speaker_entity_id,u.speech_text,u.event_id,u.world_time,e.payload,COALESCE(hearing.claim_payload,'')
		FROM rp_utterances u JOIN events e ON e.event_id=u.event_id
		LEFT JOIN observation_records hearing ON hearing.source_event_id=e.event_id
		  AND hearing.observer_agent_id=? AND hearing.subject_agent_id=u.speaker_entity_id AND hearing.claim_key='speech:'||u.event_id
		WHERE e.instance_id=? AND e.branch_id=? AND e.event_sequence<=?
		  AND (u.speaker_entity_id=? OR EXISTS (
		    SELECT 1 FROM observation_records o
		    WHERE o.source_event_id=u.event_id AND o.observer_agent_id=?
		      AND o.subject_agent_id=u.speaker_entity_id
		      AND o.claim_key='speech:' || u.event_id
		      AND json_extract(o.claim_payload,'$.claim_type')='speaker_said'))
		ORDER BY e.event_sequence DESC LIMIT 16`, input.NPCEntityID, input.InstanceID, input.BranchID, input.HeadSequence, input.NPCEntityID, input.NPCEntityID)
	if err != nil {
		return core.RPDecisionInput{}, core.WrapError(core.CodeStorageFailure, "read NPC accepted dialogue", err)
	}
	for rows.Next() {
		var utterance core.RPDecisionDialogue
		var sourceJSON, hearingJSON string
		if err := rows.Scan(&utterance.SpeakerEntityID, &utterance.Text, &utterance.EventID, &utterance.WorldTime, &sourceJSON, &hearingJSON); err != nil {
			rows.Close()
			return core.RPDecisionInput{}, core.WrapError(core.CodeStorageFailure, "scan NPC accepted dialogue", err)
		}
		utterance.SpeechTone, err = recordedRPSpeechTone(sourceJSON, hearingJSON, utterance.SpeakerEntityID, input.NPCEntityID)
		if err != nil {
			rows.Close()
			return core.RPDecisionInput{}, err
		}
		input.RecentDialogue = append(input.RecentDialogue, utterance)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return core.RPDecisionInput{}, core.WrapError(core.CodeStorageFailure, "iterate NPC accepted dialogue", err)
	}
	rows.Close()
	for i, j := 0, len(input.RecentDialogue)-1; i < j; i, j = i+1, j-1 {
		input.RecentDialogue[i], input.RecentDialogue[j] = input.RecentDialogue[j], input.RecentDialogue[i]
	}
	input.RelevantDialogue, err = readRPRelevantDialogue(ctx, conn, input)
	if err != nil {
		return core.RPDecisionInput{}, err
	}
	input.RecentPrivateDecisions, err = readRPOwnPrivateDecisionMemory(ctx, conn, input)
	if err != nil {
		return core.RPDecisionInput{}, err
	}
	// The all-speaker window is intentionally short. In a crowded scene it can
	// lose an earlier player offer after only a few rounds, then falsely make
	// the NPC deny hearing it. Preserve a bounded, explicitly excerpted history
	// of this interlocutor's accepted words with personal hearing evidence.
	if input.InterlocutorEntityID != "" {
		rows, err = conn.QueryContext(ctx, `
			SELECT u.speech_text,u.event_id,u.world_time
			FROM rp_utterances u JOIN events e ON e.event_id=u.event_id
			WHERE e.instance_id=? AND e.branch_id=? AND e.event_sequence<=?
			  AND u.speaker_entity_id=? AND u.event_id<>?
			  AND EXISTS (SELECT 1 FROM observation_records o
			    WHERE o.source_event_id=u.event_id AND o.observer_agent_id=?
			      AND o.subject_agent_id=u.speaker_entity_id
			      AND o.claim_key='speech:' || u.event_id
			      AND json_extract(o.claim_payload,'$.claim_type')='speaker_said')
			ORDER BY e.event_sequence DESC LIMIT 40`, input.InstanceID, input.BranchID, input.HeadSequence,
			input.InterlocutorEntityID, input.SpeechEventID, input.NPCEntityID)
		if err != nil {
			return core.RPDecisionInput{}, core.WrapError(core.CodeStorageFailure, "read NPC heard player history", err)
		}
		for rows.Next() {
			var memory core.RPDecisionSpeechExcerpt
			if err := rows.Scan(&memory.Excerpt, &memory.EventID, &memory.WorldTime); err != nil {
				rows.Close()
				return core.RPDecisionInput{}, core.WrapError(core.CodeStorageFailure, "scan NPC heard player history", err)
			}
			characters := []rune(memory.Excerpt)
			if len(characters) > 350 {
				memory.Excerpt = string(characters[:350]) + "…"
				memory.Truncated = true
			}
			input.HeardPlayerHistory = append(input.HeardPlayerHistory, memory)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return core.RPDecisionInput{}, core.WrapError(core.CodeStorageFailure, "iterate NPC heard player history", err)
		}
		rows.Close()
		for i, j := 0, len(input.HeardPlayerHistory)-1; i < j; i, j = i+1, j-1 {
			input.HeardPlayerHistory[i], input.HeardPlayerHistory[j] = input.HeardPlayerHistory[j], input.HeardPlayerHistory[i]
		}
	}
	rows, err = conn.QueryContext(ctx, `
		SELECT k.subject_agent_id, k.place_id, k.source_event_id, k.claim_payload,
		       o.claim_payload,e.world_time,e.actor_id,e.event_type,e.payload,
		       (o.observer_agent_id=k.observer_agent_id AND o.subject_agent_id=k.subject_agent_id
		        AND o.source_event_id=k.source_event_id AND o.place_id=k.place_id
		        AND o.claim_key=k.claim_key AND o.observed_world_time=e.world_time)
		FROM agent_knowledge k JOIN events e ON e.event_id=k.source_event_id
		JOIN observation_records o ON o.observation_id=k.observation_id
		WHERE k.observer_agent_id = ? AND e.instance_id=? AND e.branch_id=? AND e.event_sequence<=?
		AND e.event_type<>'RPInformationDelivered' AND o.channel NOT IN ('direct_message','rumor','organization_announcement','public_notice') AND k.claim_key NOT LIKE 'information:%'
		ORDER BY k.last_event_sequence DESC, k.claim_key LIMIT 20`, input.NPCEntityID, input.InstanceID, input.BranchID, input.HeadSequence)
	if err != nil {
		return core.RPDecisionInput{}, core.WrapError(core.CodeStorageFailure, "read NPC own knowledge", err)
	}
	for rows.Next() {
		var subjectID, placeID, eventID, payloadJSON, observedJSON, worldTime, eventActor, eventType, sourceJSON string
		var consistent bool
		if err := rows.Scan(&subjectID, &placeID, &eventID, &payloadJSON, &observedJSON, &worldTime, &eventActor, &eventType, &sourceJSON, &consistent); err != nil {
			rows.Close()
			return core.RPDecisionInput{}, core.WrapError(core.CodeStorageFailure, "scan NPC own knowledge", err)
		}
		var payload struct {
			ClaimType   string `json:"claim_type"`
			Text        string `json:"text"`
			SpeechTone  string `json:"speech_tone"`
			Description string `json:"description"`
		}
		if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
			rows.Close()
			return core.RPDecisionInput{}, core.WrapError(core.CodeProjectionDiverged, "decode NPC knowledge claim", err)
		}
		claim := core.RPDecisionKnowledge{ClaimType: payload.ClaimType, SubjectEntityID: subjectID, SourceEventID: eventID}
		switch payload.ClaimType {
		case "agent_presence":
			claim.PlaceID = placeID
		case "speaker_said":
			claim.Text = payload.Text
			claim.SpeechTone, err = rpKnowledgeSpeechTone(eventType, sourceJSON, observedJSON, eventActor, input.NPCEntityID)
			if err != nil || claim.SpeechTone != payload.SpeechTone {
				rows.Close()
				if err != nil {
					return core.RPDecisionInput{}, err
				}
				return core.RPDecisionInput{}, narrativeDiverged("NPC known speech delivery differs from its frozen hearing")
			}
		case "nonverbal_action":
			// The observer's frozen claim, not the raw Event target, owns
			// disclosure. Later visibility cannot fill an absent old target.
			var witnessed core.RPNonverbalClaim
			var knowledge, observation map[string]any
			if json.Unmarshal([]byte(observedJSON), &witnessed) != nil || json.Unmarshal([]byte(payloadJSON), &knowledge) != nil || json.Unmarshal([]byte(observedJSON), &observation) != nil {
				rows.Close()
				return core.RPDecisionInput{}, core.NewError(core.CodeProjectionDiverged, "invalid NPC nonverbal witness claim")
			}
			knowledgeHash, knowledgeErr := core.HashJSON(knowledge)
			observationHash, observationErr := core.HashJSON(observation)
			if !consistent || eventType != "RPNonverbalAction" || eventActor != subjectID || witnessed.ClaimType != "nonverbal_action" || (witnessed.ActorEntityID != "" && witnessed.ActorEntityID != subjectID) || knowledgeErr != nil || observationErr != nil || knowledgeHash != observationHash {
				rows.Close()
				return core.RPDecisionInput{}, core.NewError(core.CodeProjectionDiverged, "NPC nonverbal knowledge differs from its observer source")
			}
			if !rpDecisionNonverbalWitnessMatchesSource(sourceJSON, observation, input.NPCEntityID, subjectID, placeID) {
				rows.Close()
				return core.RPDecisionInput{}, core.NewError(core.CodeProjectionDiverged, "NPC nonverbal claim differs from its frozen event witness")
			}
			claim.Text, claim.Action, claim.GestureCode = witnessed.Description, witnessed.Action, witnessed.GestureCode
			claim.TargetEntityID, claim.WorldTime = witnessed.TargetEntityID, worldTime
		case "object_interaction":
			claim.Text = payload.Description
		default:
			continue // Unknown claim types are not provider-visible by default.
		}
		input.Knowledge = append(input.Knowledge, claim)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return core.RPDecisionInput{}, core.WrapError(core.CodeStorageFailure, "iterate NPC own knowledge", err)
	}
	rows.Close()
	var next core.RPDecisionSchedule
	var nextID, originalTime string
	err = conn.QueryRowContext(ctx, `SELECT s.schedule_id,s.world_time,q.world_time, s.place_id, s.activity_code, s.definition_event_id FROM agent_schedule_entries s JOIN scheduler_items q ON q.scheduler_item_id=s.scheduler_item_id WHERE s.agent_id = ? AND s.status = 'active' AND q.status='pending' AND q.world_time >= ? ORDER BY q.world_time, s.declared_priority, s.scheduler_item_id LIMIT 1`, input.NPCEntityID, input.WorldTime).Scan(&nextID, &originalTime, &next.WorldTime, &next.PlaceID, &next.ActivityCode, &next.SourceEventID)
	if err == nil {
		if originalTime != next.WorldTime {
			delay, err := readLatestRPTransitDelay(ctx, conn, input.InstanceID, input.BranchID, nextID)
			if err != nil {
				return core.RPDecisionInput{}, err
			}
			if delay == nil || delay.OriginalWorldTime != originalTime || delay.Retry.WorldTime != next.WorldTime || delay.AgentID != input.NPCEntityID {
				return core.RPDecisionInput{}, core.NewError(core.CodeProjectionDiverged, "effective appointment lacks own delay evidence")
			}
			next.OriginalWorldTime = originalTime
			next.DelaySourceEventID = "event_" + delay.Previous.ID
		}
		input.NextSchedule = &next
	} else if !errors.Is(err, sql.ErrNoRows) {
		return core.RPDecisionInput{}, core.WrapError(core.CodeStorageFailure, "read NPC next schedule", err)
	}
	rows, err = conn.QueryContext(ctx, `
		SELECT link.to_place_id FROM rp_place_links link
		JOIN agent_places destination ON destination.place_id = link.to_place_id
		WHERE link.instance_id = ? AND link.branch_id = ? AND link.from_place_id = ?
		  AND destination.status = 'active' AND destination.instance_id = link.instance_id AND destination.branch_id = link.branch_id
		ORDER BY link.to_place_id`, input.InstanceID, input.BranchID, input.PlaceID)
	if err != nil {
		return core.RPDecisionInput{}, core.WrapError(core.CodeStorageFailure, "read legal NPC destinations", err)
	}
	for rows.Next() {
		var placeID string
		if err := rows.Scan(&placeID); err != nil {
			rows.Close()
			return core.RPDecisionInput{}, core.WrapError(core.CodeStorageFailure, "scan legal NPC destination", err)
		}
		input.ReachablePlaceIDs = append(input.ReachablePlaceIDs, placeID)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return core.RPDecisionInput{}, core.WrapError(core.CodeStorageFailure, "iterate legal NPC destinations", err)
	}
	rows.Close()
	openPlaces := input.ReachablePlaceIDs[:0]
	for _, place := range input.ReachablePlaceIDs {
		open, err := rpTransitAllowsImmediate(ctx, conn, input.InstanceID, input.BranchID, input.PlaceID, place, input.WorldTime)
		if err != nil {
			return core.RPDecisionInput{}, err
		}
		if open {
			openPlaces = append(openPlaces, place)
		}
	}
	input.ReachablePlaceIDs = openPlaces
	if len(input.ReachablePlaceIDs) != 0 {
		input.LegalActions = append(input.LegalActions, "leave")
	}
	rules, err := readRPActivityRules(ctx, conn, input.InstanceID, input.BranchID)
	if err != nil {
		return core.RPDecisionInput{}, err
	}
	if len(rules) != 0 {
		inProgress := make(map[string]bool, len(rules))
		rows, err := conn.QueryContext(ctx, `SELECT DISTINCT activity_code FROM rp_activities WHERE actor_id = ? AND status = 'in_progress'`, input.NPCEntityID)
		if err != nil {
			return core.RPDecisionInput{}, core.WrapError(core.CodeStorageFailure, "read NPC in-progress activities", err)
		}
		for rows.Next() {
			var code string
			if err := rows.Scan(&code); err != nil {
				rows.Close()
				return core.RPDecisionInput{}, core.WrapError(core.CodeStorageFailure, "scan NPC in-progress activity", err)
			}
			inProgress[code] = true
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return core.RPDecisionInput{}, core.WrapError(core.CodeStorageFailure, "iterate NPC in-progress activities", err)
		}
		rows.Close()
		for _, code := range sortedKeys(rules) {
			if !inProgress[code] {
				input.LegalActivities = append(input.LegalActivities, code)
			}
		}
		if len(input.LegalActivities) != 0 {
			input.LegalActions = append(input.LegalActions, "act")
		}
	}
	input.OwnActions, err = readRPSourcedOwnActions(ctx, conn, input.InstanceID, input.BranchID, input.NPCEntityID)
	if err != nil {
		return core.RPDecisionInput{}, err
	}
	input.SceneActivities, err = readRPSceneActivityContext(ctx, conn, input)
	if err != nil {
		return core.RPDecisionInput{}, err
	}
	input.SceneObjects, err = readRPDecisionSceneObjects(ctx, conn, input)
	if err != nil {
		return core.RPDecisionInput{}, err
	}
	input.Life, err = buildRPLifeContext(ctx, conn, input)
	if err != nil {
		return core.RPDecisionInput{}, err
	}
	knownLaws, err := readKnownRPLaws(ctx, conn, input.InstanceID, input.BranchID, input.NPCEntityID)
	if err != nil {
		return core.RPDecisionInput{}, err
	}
	input.Law = core.BuildRPLawContext(knownLaws, input.WorldTime, input.PlaceID, input.LegalActions)
	input.Environment, err = readRPLocalEnvironment(ctx, conn, input.InstanceID, input.BranchID, input.PlaceID, input.WorldTime)
	if err != nil {
		return core.RPDecisionInput{}, err
	}
	input.Stores, err = readRPLocalStores(ctx, conn, input.InstanceID, input.BranchID, input.PlaceID)
	if err != nil {
		return core.RPDecisionInput{}, err
	}
	input.TransitWorks, err = readRPLocalTransitWorks(ctx, conn, input.InstanceID, input.BranchID, input.PlaceID, input.WorldTime)
	if err != nil {
		return core.RPDecisionInput{}, err
	}
	return input, nil
}

// Relationships are read from the same committed Studio identity event used
// by the familiarity projection. There is no second mutable RP canon store.
// Legacy witness claims may omit typed fields. Compare every supplied field
// with the exact observer's frozen claim, but never fill omitted fields from
// the privileged Event. Matching two projections alone cannot prove a source.
func rpDecisionNonverbalWitnessMatchesSource(raw string, observed map[string]any, observer, actor, place string) bool {
	var fact core.RPNonverbalFact
	if json.Unmarshal([]byte(raw), &fact) != nil || fact.ClaimType != "nonverbal_action" || fact.ActorEntityID != actor || fact.PlaceID != place {
		return false
	}
	var expected core.RPNonverbalClaim
	found := false
	for _, witness := range fact.Witnesses {
		if witness.ObserverEntityID != observer {
			continue
		}
		if found {
			return false
		}
		found = true
		expected = rpNonverbalWitnessClaim(fact, witness)
	}
	if !found || observed["claim_type"] != expected.ClaimType || observed["description"] != expected.Description {
		return false
	}
	fields := map[string]string{"claim_type": expected.ClaimType, "description": expected.Description, "actor_entity_id": expected.ActorEntityID, "action": expected.Action, "gesture_code": expected.GestureCode, "target_entity_id": expected.TargetEntityID}
	for key, value := range observed {
		want, known := fields[key]
		if !known || value != want {
			return false
		}
	}
	return true
}

func readRPAuthoredRelationships(ctx context.Context, conn *sql.Conn, input core.RPDecisionInput) ([]core.RPCharacterRelationship, error) {
	var sourceID, raw string
	err := conn.QueryRowContext(ctx, `SELECT event_id,payload FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPIdentitiesDeclared' AND event_sequence<=? ORDER BY event_sequence LIMIT 1`, input.InstanceID, input.BranchID, input.HeadSequence).Scan(&sourceID, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil // legacy worlds have no authored identity event
	}
	if err != nil {
		return nil, core.WrapError(core.CodeStorageFailure, "read authored RP relationships", err)
	}
	var declared RPIdentitiesDeclaredFact
	if err := json.Unmarshal([]byte(raw), &declared); err != nil {
		return nil, core.WrapError(core.CodeProjectionDiverged, "decode authored RP relationships", err)
	}
	result := make([]core.RPCharacterRelationship, 0)
	for _, relation := range declared.Relationships {
		if relation.ActorEntityID != input.NPCEntityID {
			continue
		}
		if relation.SubjectEntityID == "" || relation.Role == "" {
			return nil, core.NewError(core.CodeProjectionDiverged, "authored RP relationship is incomplete")
		}
		var known int
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_identity_familiarity WHERE instance_id=? AND branch_id=? AND observer_agent_id=? AND subject_agent_id=?`, input.InstanceID, input.BranchID, input.NPCEntityID, relation.SubjectEntityID).Scan(&known); err != nil {
			return nil, core.WrapError(core.CodeStorageFailure, "check authored relationship familiarity", err)
		}
		if known != 1 {
			return nil, core.NewError(core.CodeProjectionDiverged, "authored relationship lacks declared identity")
		}
		result = append(result, core.RPCharacterRelationship{SubjectEntityID: relation.SubjectEntityID, Role: relation.Role,
			AddressTo: relation.AddressTo, SelfReference: relation.SelfReference, SourceEventID: sourceID})
	}
	return result, nil
}

// DecideRP asks a provider for a candidate only; a later turn command must
// recheck the head and legality before any proposed world effect is committed.
func (s *Store) DecideRP(ctx context.Context, request core.RPDecisionRequest, provider core.RPDecisionProvider) (RPDecisionResult, error) {
	if provider == nil {
		return RPDecisionResult{}, core.NewError(core.CodeInvalidArgument, "RP decision provider is required")
	}
	input, err := s.BuildRPDecisionInput(ctx, request)
	if err != nil {
		return RPDecisionResult{}, err
	}
	// BuildRPDecisionInput is also a read-only diagnostic. Provider invocation
	// is the first autonomous-decision boundary and must respect live control.
	if err := requireInternalRPDecisionOwner(ctx, s.db, input.InstanceID, input.BranchID, request.NPCEntityID); err != nil {
		return RPDecisionResult{}, err
	}
	inputHash, err := core.HashJSON(input)
	if err != nil {
		return RPDecisionResult{}, err
	}
	providerInput, err := s.rpDecisionProviderView(ctx, input)
	if err != nil {
		return RPDecisionResult{}, err
	}
	result := RPDecisionResult{NPCEntityID: request.NPCEntityID, TurnID: request.TurnID, HeadSequence: input.HeadSequence, InputHash: inputHash}
	var runID string
	if err := s.db.QueryRowContext(ctx, `SELECT turn_run_id FROM rp_turn_runs WHERE session_id=? AND COALESCE(player_turn_id,player_event_id)=?`, request.SessionID, request.TurnID).Scan(&runID); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return RPDecisionResult{}, core.WrapError(core.CodeStorageFailure, "find RP provider turn", err)
	}
	metadata := rpProviderMetadata(provider, "custom")
	callID, err := s.beginRPProviderCall(ctx, rpProviderCallScope{SessionID: request.SessionID, TurnRunID: runID, SubjectID: request.TurnID, NPCEntityID: request.NPCEntityID, Phase: "decision"}, metadata)
	if err != nil {
		return RPDecisionResult{}, err
	}
	var trace core.RPProviderTrace
	proposal, providerErr := provider.Propose(core.WithRPProviderTrace(ctx, &trace), providerInput)
	auditCtx, stopAudit := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer stopAudit()
	if providerErr != nil {
		result.Proposal = core.RPDecisionProposal{Action: "silence"}
		result.Status = "provider_fallback"
		result.ReasonCode = "provider_failure"
		callResult, fallback := rpProviderErrorResult(ctx, providerErr), "silence"
		var classified interface{ RPDecisionFailureCode() string }
		if errors.As(providerErr, &classified) {
			result.ProviderErrorCode = classified.RPDecisionFailureCode()
		}
		if result.ProviderErrorCode == "context_not_ready" && providerInput.Readiness.Incomplete() && trace.AttemptCount() == 0 {
			callResult, fallback = "not_used", "rp_context_not_ready"
			result.ReasonCode = "rp_context_not_ready"
		}
		if err := s.finishRPProviderCall(ctx, callID, callResult, fallback, "", trace.AttemptCount()); err != nil {
			return RPDecisionResult{}, err
		}
		if auditErr := s.auditRPDecision(auditCtx, input, result); auditErr != nil {
			return RPDecisionResult{}, auditErr
		}
		return result, nil
	}
	reason, validationErr := core.ValidateRPDecisionProposalEvidence(providerInput, proposal)
	if validationErr == nil {
		reason, validationErr = core.ValidateRPDecisionProposalEvidence(input, proposal)
	}
	if validationErr != nil {
		if recordErr := s.finishRPProviderCall(ctx, callID, "failed", "invalid_proposal", "", trace.AttemptCount()); recordErr != nil {
			return RPDecisionResult{}, recordErr
		}
		result.Proposal = proposal
		result.Status = "rejected"
		result.ReasonCode = reason
		if auditErr := s.auditRPDecision(auditCtx, input, result); auditErr != nil {
			return RPDecisionResult{}, auditErr
		}
		return RPDecisionResult{}, validationErr
	}
	if err := s.finishRPProviderCall(ctx, callID, "success", "", "", trace.AttemptCount()); err != nil {
		return RPDecisionResult{}, err
	}
	result.Proposal = proposal
	result.Status = "validated"
	if err := s.auditRPDecision(auditCtx, input, result); err != nil {
		return RPDecisionResult{}, err
	}
	return result, nil
}

func validateRPDecisionProposal(input core.RPDecisionInput, proposal core.RPDecisionProposal) error {
	return core.ValidateRPDecisionProposal(input, proposal)
}

func (s *Store) auditRPDecision(ctx context.Context, input core.RPDecisionInput, result RPDecisionResult) error {
	id, err := newRPSessionID()
	if err != nil {
		return err
	}
	payload := struct {
		ReasonCode        string                  `json:"reason_code,omitempty"`
		ProviderErrorCode string                  `json:"provider_error_code,omitempty"`
		TurnID            string                  `json:"turn_id"`
		NPCID             string                  `json:"npc_entity_id"`
		InputHash         string                  `json:"input_hash"`
		Status            string                  `json:"status"`
		Proposal          core.RPDecisionProposal `json:"proposal"`
	}{result.ReasonCode, result.ProviderErrorCode, result.TurnID, result.NPCEntityID, result.InputHash, result.Status, result.Proposal}
	payloadJSON, err := core.CanonicalJSON(payload)
	if err != nil {
		return err
	}
	scopeJSON, err := core.CanonicalJSON(struct {
		Kind       string `json:"kind"`
		InstanceID string `json:"instance_id"`
	}{"rp_internal", input.InstanceID})
	if err != nil {
		return err
	}
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return core.WrapError(core.CodeStorageFailure, "begin NPC decision audit", err)
	}
	defer tx.Rollback(ctx)
	var order int64
	if err := tx.conn.QueryRowContext(ctx, `SELECT COALESCE(MAX(record_order), 0) + 1 FROM audit_records WHERE instance_id = ? AND branch_id = ?`, input.InstanceID, input.BranchID).Scan(&order); err != nil {
		return core.WrapError(core.CodeStorageFailure, "allocate NPC decision audit order", err)
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	recordType := "agent_decision"
	if result.Status != "validated" {
		recordType = "runtime_diagnostic"
	}
	relatedEvent := input.SpeechEventID
	if input.ObservedPlayerAction != nil {
		relatedEvent = input.ObservedPlayerAction.SourceEventID
	}
	if err := execAgentOne(ctx, tx.conn, "record non-authoritative NPC decision", `INSERT INTO audit_records(record_id, instance_id, branch_id, record_order, record_type, authority, related_event_id, trace_id, world_time, recorded_at_utc, audience_scope, payload) VALUES (?, ?, ?, ?, ?, 'non-authoritative', ?, ?, ?, ?, ?, ?)`, "audit_rp_decision_"+id, input.InstanceID, input.BranchID, order, recordType, relatedEvent, "trace_rp_decision_"+id, input.WorldTime, now, string(scopeJSON), string(payloadJSON)); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return core.WrapError(core.CodeStorageFailure, "commit NPC decision audit", err)
	}
	return nil
}
