package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"corerp.local/backend/internal/core"
)

type RPDecisionResult struct {
	ReasonCode   string                  `json:"reason_code,omitempty"`
	NPCEntityID  string                  `json:"npc_entity_id"`
	TurnID       string                  `json:"turn_id"`
	HeadSequence int64                   `json:"head_sequence"`
	InputHash    string                  `json:"input_hash"`
	Proposal     core.RPDecisionProposal `json:"proposal"`
	Status       string                  `json:"status"`
}

// BuildRPDecisionInput is deliberately narrower than the world's state. It
// requires proof that this NPC heard the committed player speech in this turn.
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
	if session.Status != "active" || session.TurnCursor != request.TurnID || (session.TurnState != "speech_committed" && session.TurnState != "npc_effects_committed") {
		return core.RPDecisionInput{}, core.NewError(core.CodeBranchConflict, "NPC decision requires the active committed speech turn")
	}
	if request.NPCEntityID == session.ControlledEntityID {
		return core.RPDecisionInput{}, core.NewError(core.CodeInvalidArgument, "player cannot be selected as NPC")
	}
	input := core.RPDecisionInput{
		InstanceID: session.InstanceID, BranchID: session.BranchID,
		TurnID: request.TurnID, NPCEntityID: request.NPCEntityID,
		VisibleEntities:   make([]core.RPDecisionVisibleEntity, 0),
		Knowledge:         make([]core.RPDecisionKnowledge, 0),
		ReachablePlaceIDs: make([]string, 0),
		LegalActions:      []string{"respond", "refuse", "silence", "wait"},
	}
	var speakerID, speechPlace string
	err = tx.conn.QueryRowContext(ctx, `
		SELECT u.event_id, u.speaker_entity_id, u.place_id, u.speech_text
		FROM rp_utterances u JOIN events e ON e.event_id = u.event_id
		WHERE u.session_id = ? AND u.turn_id = ? AND e.instance_id = ? AND e.branch_id = ?`,
		session.SessionID, request.TurnID, session.InstanceID, session.BranchID,
	).Scan(&input.SpeechEventID, &speakerID, &speechPlace, &input.PlayerSpeechText)
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
	return input, nil
}

// readRPOwnDecisionContext shares only own/visible evidence. Callers must first
// authorize the actor and establish their distinct speech or time trigger.
func readRPOwnDecisionContext(ctx context.Context, conn *sql.Conn, input core.RPDecisionInput) (core.RPDecisionInput, error) {
	err := conn.QueryRowContext(ctx, `
		SELECT b.head_sequence, c.current_world_time, e.display_name, p.place_id, l.display_name, p.activity_code,
		       a.goal_code, balances.balance_minor, e.currency_id
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
		&input.ActivityCode, &input.GoalCode, &input.OwnAssetMinor, &input.CurrencyID)
	if err != nil {
		return core.RPDecisionInput{}, classifyMissing(err, "active NPC decision state")
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
	rows, err = conn.QueryContext(ctx, `
		SELECT k.subject_agent_id, k.place_id, k.source_event_id, k.claim_payload
		FROM agent_knowledge k JOIN events e ON e.event_id=k.source_event_id
		JOIN observation_records o ON o.observation_id=k.observation_id
		WHERE k.observer_agent_id = ? AND e.instance_id=? AND e.branch_id=?
		AND e.event_type<>'RPInformationDelivered' AND o.channel NOT IN ('direct_message','rumor','organization_announcement','public_notice') AND k.claim_key NOT LIKE 'information:%'
		ORDER BY k.last_event_sequence DESC, k.claim_key LIMIT 20`, input.NPCEntityID, input.InstanceID, input.BranchID)
	if err != nil {
		return core.RPDecisionInput{}, core.WrapError(core.CodeStorageFailure, "read NPC own knowledge", err)
	}
	for rows.Next() {
		var subjectID, placeID, eventID, payloadJSON string
		if err := rows.Scan(&subjectID, &placeID, &eventID, &payloadJSON); err != nil {
			rows.Close()
			return core.RPDecisionInput{}, core.WrapError(core.CodeStorageFailure, "scan NPC own knowledge", err)
		}
		var payload struct {
			ClaimType string `json:"claim_type"`
			Text      string `json:"text"`
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
	result := RPDecisionResult{NPCEntityID: request.NPCEntityID, TurnID: request.TurnID, HeadSequence: input.HeadSequence, InputHash: inputHash}
	proposal, err := provider.Propose(ctx, input)
	auditCtx, stopAudit := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer stopAudit()
	if err != nil {
		result.Proposal = core.RPDecisionProposal{Action: "silence"}
		result.Status = "provider_fallback"
		result.ReasonCode = "provider_failure"
		if auditErr := s.auditRPDecision(auditCtx, input, result); auditErr != nil {
			return RPDecisionResult{}, auditErr
		}
		return result, nil
	}
	if reason, err := core.ValidateRPDecisionProposalEvidence(input, proposal); err != nil {
		result.Proposal = proposal
		result.Status = "rejected"
		result.ReasonCode = reason
		if auditErr := s.auditRPDecision(auditCtx, input, result); auditErr != nil {
			return RPDecisionResult{}, auditErr
		}
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
		ReasonCode string                  `json:"reason_code,omitempty"`
		TurnID     string                  `json:"turn_id"`
		NPCID      string                  `json:"npc_entity_id"`
		InputHash  string                  `json:"input_hash"`
		Status     string                  `json:"status"`
		Proposal   core.RPDecisionProposal `json:"proposal"`
	}{result.ReasonCode, result.TurnID, result.NPCEntityID, result.InputHash, result.Status, result.Proposal}
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
	if err := execAgentOne(ctx, tx.conn, "record non-authoritative NPC decision", `INSERT INTO audit_records(record_id, instance_id, branch_id, record_order, record_type, authority, related_event_id, trace_id, world_time, recorded_at_utc, audience_scope, payload) VALUES (?, ?, ?, ?, ?, 'non-authoritative', ?, ?, ?, ?, ?, ?)`, "audit_rp_decision_"+id, input.InstanceID, input.BranchID, order, recordType, input.SpeechEventID, "trace_rp_decision_"+id, input.WorldTime, now, string(scopeJSON), string(payloadJSON)); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return core.WrapError(core.CodeStorageFailure, "commit NPC decision audit", err)
	}
	return nil
}
