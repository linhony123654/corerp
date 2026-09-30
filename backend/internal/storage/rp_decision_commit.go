package storage

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"corerp.local/backend/internal/core"
)

type RPNPCDecisionCommitResult struct {
	DecisionID    string `json:"decision_id"`
	EventID       string `json:"event_id"`
	EventSequence int64  `json:"event_sequence"`
	ParentTurnID  string `json:"parent_turn_id"`
	NPCEntityID   string `json:"npc_entity_id"`
	Action        string `json:"action"`
	WorldTime     string `json:"world_time"`
	Replayed      bool   `json:"replayed"`
}

type rpNPCActionEvent struct {
	SessionID    string `json:"session_id"`
	ParentTurnID string `json:"parent_turn_id"`
	NPCEntityID  string `json:"npc_entity_id"`
	Action       string `json:"action"`
	FromPlaceID  string `json:"from_place_id,omitempty"`
	ToPlaceID    string `json:"to_place_id,omitempty"`
}

// CommitRPDecision revalidates a proposal against a fresh world snapshot and
// commits one NPC decision for this player turn/NPC pair. Speech and any
// observable expression are separate sourced events in the same atomic batch.
// Provider output
// itself never writes an Event; this method owns the authority boundary.
func (s *Store) CommitRPDecision(ctx context.Context, request core.RPDecisionRequest, decision RPDecisionResult) (RPNPCDecisionCommitResult, error) {
	if err := request.Validate(); err != nil {
		return RPNPCDecisionCommitResult{}, err
	}
	if decision.NPCEntityID != request.NPCEntityID || decision.TurnID != request.TurnID || (decision.Status != "validated" && decision.Status != "provider_fallback") {
		return RPNPCDecisionCommitResult{}, core.NewError(core.CodeInvalidArgument, "NPC decision does not match requested turn or lacks validation")
	}
	proposalHash, err := core.HashJSON(decision.Proposal)
	if err != nil {
		return RPNPCDecisionCommitResult{}, err
	}
	if result, found, err := s.findCommittedRPDecision(ctx, request, decision.InputHash, proposalHash); err != nil || found {
		return result, err
	}
	input, err := s.BuildRPDecisionInput(ctx, request)
	if err != nil {
		return RPNPCDecisionCommitResult{}, err
	}
	inputHash, err := core.HashJSON(input)
	if err != nil {
		return RPNPCDecisionCommitResult{}, err
	}
	if input.HeadSequence != decision.HeadSequence || inputHash != decision.InputHash {
		return RPNPCDecisionCommitResult{}, core.NewError(core.CodeBranchConflict, "NPC decision input is stale")
	}
	if err := validateRPDecisionProposal(input, decision.Proposal); err != nil {
		return RPNPCDecisionCommitResult{}, err
	}
	if decision.Status == "provider_fallback" && decision.Proposal.Action != "silence" {
		return RPNPCDecisionCommitResult{}, core.NewError(core.CodeInvalidArgument, "provider fallback may only choose silence")
	}
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return RPNPCDecisionCommitResult{}, core.WrapError(core.CodeStorageFailure, "begin NPC decision commit", err)
	}
	defer tx.Rollback(ctx)
	session, err := loadRPSession(ctx, tx.conn, request.PrincipalID, request.SessionID)
	if err != nil {
		return RPNPCDecisionCommitResult{}, err
	}
	if err := authorizeRPControl(ctx, tx.conn, request.PrincipalID, session.InstanceID, session.BranchID, session.ControlledEntityID); err != nil {
		return RPNPCDecisionCommitResult{}, err
	}
	if err := requireInternalRPDecisionOwner(ctx, tx.conn, session.InstanceID, session.BranchID, request.NPCEntityID); err != nil {
		return RPNPCDecisionCommitResult{}, err
	}
	if result, found, err := findCommittedRPDecisionOnConn(ctx, tx.conn, session.SessionID, request, decision.InputHash, proposalHash); err != nil || found {
		return result, err
	}
	if session.Status != "active" || session.TurnCursor != request.TurnID || (session.TurnState != "speech_committed" && session.TurnState != "npc_effects_committed") {
		return RPNPCDecisionCommitResult{}, core.NewError(core.CodeBranchConflict, "NPC decision turn is no longer active")
	}
	var runStatus string
	err = tx.conn.QueryRowContext(ctx, `SELECT status FROM rp_turn_runs WHERE session_id = ? AND player_turn_id = ?`, session.SessionID, request.TurnID).Scan(&runStatus)
	if err == nil && runStatus != "npc_deciding" {
		return RPNPCDecisionCommitResult{}, core.NewError(core.CodeBranchConflict, "orchestrated NPC decision stage is no longer open")
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return RPNPCDecisionCommitResult{}, core.WrapError(core.CodeStorageFailure, "check NPC decision turn stage", err)
	}
	var head, currentDay int64
	var worldTime, npcPlace, playerPlace string
	if err := tx.conn.QueryRowContext(ctx, `SELECT b.head_sequence, c.current_world_time, c.current_day FROM branches b JOIN world_clocks c ON c.instance_id = b.instance_id AND c.branch_id = b.branch_id WHERE b.instance_id = ? AND b.branch_id = ?`, session.InstanceID, session.BranchID).Scan(&head, &worldTime, &currentDay); err != nil {
		return RPNPCDecisionCommitResult{}, classifyMissing(err, "NPC decision world")
	}
	if head != input.HeadSequence || worldTime != input.WorldTime {
		return RPNPCDecisionCommitResult{}, core.NewError(core.CodeBranchConflict, "NPC decision world changed after observation")
	}
	if err := tx.conn.QueryRowContext(ctx, `SELECT p.place_id FROM agent_positions p JOIN agent_profiles a ON a.agent_id = p.agent_id WHERE a.agent_id = ? AND a.instance_id = ? AND a.branch_id = ? AND a.status = 'active'`, request.NPCEntityID, session.InstanceID, session.BranchID).Scan(&npcPlace); err != nil {
		return RPNPCDecisionCommitResult{}, classifyMissing(err, "NPC decision position")
	}
	if err := tx.conn.QueryRowContext(ctx, `SELECT p.place_id FROM agent_positions p JOIN agent_profiles a ON a.agent_id = p.agent_id WHERE a.agent_id = ? AND a.instance_id = ? AND a.branch_id = ? AND a.status = 'active'`, session.ControlledEntityID, session.InstanceID, session.BranchID).Scan(&playerPlace); err != nil {
		return RPNPCDecisionCommitResult{}, classifyMissing(err, "player decision position")
	}
	if npcPlace != input.PlaceID || playerPlace != input.PlaceID {
		return RPNPCDecisionCommitResult{}, core.NewError(core.CodeBranchConflict, "NPC or player has left the decision scene")
	}
	var pendingWaits int
	if err := tx.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_wait_intents i JOIN rp_sessions s ON s.session_id = i.session_id WHERE s.instance_id = ? AND s.branch_id = ? AND i.status = 'pending'`, session.InstanceID, session.BranchID).Scan(&pendingWaits); err != nil {
		return RPNPCDecisionCommitResult{}, core.WrapError(core.CodeStorageFailure, "check pending wait before NPC effect", err)
	}
	if pendingWaits != 0 {
		return RPNPCDecisionCommitResult{}, core.NewError(core.CodeCommandInProgress, "RP wait must complete before NPC effect")
	}
	if err := ensureCohortTransitionChronology(ctx, tx.conn, session.InstanceID, session.BranchID, worldTime); err != nil {
		return RPNPCDecisionCommitResult{}, err
	}
	sequence := head + 1
	var epochID string
	if err := tx.conn.QueryRowContext(ctx, `SELECT epoch_id FROM rule_epochs WHERE instance_id = ? AND branch_id = ? AND start_sequence <= ? AND (end_sequence IS NULL OR ? < end_sequence)`, session.InstanceID, session.BranchID, sequence, sequence).Scan(&epochID); err != nil {
		return RPNPCDecisionCommitResult{}, classifyMissing(err, "NPC decision Rule Epoch")
	}
	keyHash, err := core.HashJSON(struct{ SessionID, TurnID, NPCID string }{session.SessionID, request.TurnID, request.NPCEntityID})
	if err != nil {
		return RPNPCDecisionCommitResult{}, err
	}
	suffix := keyHash[7:]
	decisionID, commandID := "decision_rp_npc_"+suffix, "cmd_rp_npc_"+suffix
	attemptID, batchID, eventID := "attempt_rp_npc_"+suffix, "batch_rp_npc_"+suffix, "event_rp_npc_"+suffix
	action := decision.Proposal.Action
	eventType := "RPNPCDecisionRecorded"
	var payload any = rpNPCActionEvent{SessionID: session.SessionID, ParentTurnID: request.TurnID, NPCEntityID: request.NPCEntityID, Action: action, FromPlaceID: input.PlaceID}
	var listeners []string
	childTurnID, utteranceID := "turn_rp_npc_"+suffix, "utterance_rp_npc_"+suffix
	activityID := "activity_rp_npc_" + suffix
	activityDuration := 0
	expression, err := prepareRPNPCExpression(ctx, tx.conn, session.SessionID, input, decision.Proposal.ExpressionCode)
	if err != nil {
		return RPNPCDecisionCommitResult{}, err
	}
	expressionID := "event_rp_npc_expression_" + suffix
	lastSequence, eventCount := sequence, 1
	if expression != nil {
		lastSequence, eventCount = sequence+1, 2
		var lastEpoch string
		if err := tx.conn.QueryRowContext(ctx, `SELECT epoch_id FROM rule_epochs WHERE instance_id = ? AND branch_id = ? AND start_sequence <= ? AND (end_sequence IS NULL OR ? < end_sequence)`, session.InstanceID, session.BranchID, lastSequence, lastSequence).Scan(&lastEpoch); err != nil {
			return RPNPCDecisionCommitResult{}, classifyMissing(err, "NPC expression Rule Epoch")
		}
		if lastEpoch != epochID {
			return RPNPCDecisionCommitResult{}, core.NewError(core.CodeBranchConflict, "NPC expression crosses a rule epoch boundary")
		}
	}
	acceptedIntroduction := decision.Proposal.IntroduceSelf && core.ExplicitSelfIntroduction(decision.Proposal.Text, input.NPCName)
	if action == "respond" || action == "refuse" {
		eventType = "RPSpeechAccepted"
		listeners, err = rpPerceivedEntityIDs(ctx, tx.conn, session.InstanceID, session.BranchID, input.PlaceID, request.NPCEntityID, "audio", "voice")
		if err != nil {
			return RPNPCDecisionCommitResult{}, err
		}
		payload = rpSpeechEvent{SessionID: session.SessionID, TurnID: childTurnID, ParentTurnID: request.TurnID, UtteranceID: utteranceID, SpeakerEntityID: request.NPCEntityID, PlaceID: input.PlaceID, Text: decision.Proposal.Text, SpeechAct: "statement", ListenerIDs: listeners, IntroduceSelf: acceptedIntroduction}
	} else if action == "leave" {
		eventType = "RPNPCMoved"
		payload = rpNPCActionEvent{SessionID: session.SessionID, ParentTurnID: request.TurnID, NPCEntityID: request.NPCEntityID, Action: action, FromPlaceID: input.PlaceID, ToPlaceID: decision.Proposal.DestinationPlaceID}
	} else if action == "act" {
		rules, err := readRPActivityRules(ctx, tx.conn, session.InstanceID, session.BranchID)
		if err != nil {
			return RPNPCDecisionCommitResult{}, err
		}
		duration, legal := rules[decision.Proposal.ActivityCode]
		if !legal {
			return RPNPCDecisionCommitResult{}, core.NewError(core.CodeInvalidArgument, "activity is not legal at decision commit")
		}
		eventType = "AgentActivityStarted"
		activityDuration = duration
		payload = rpActivityStartedEvent{ActivityID: activityID, ActorID: request.NPCEntityID, ToPlaceID: input.PlaceID, ActivityCode: decision.Proposal.ActivityCode, StartedWorldTime: worldTime, DurationMinutes: duration}
	}
	payloadJSON, err := core.CanonicalJSON(payload)
	if err != nil {
		return RPNPCDecisionCommitResult{}, err
	}
	proposalJSON, err := core.CanonicalJSON(decision.Proposal)
	if err != nil {
		return RPNPCDecisionCommitResult{}, err
	}
	requestHash, err := core.HashJSON(struct {
		InputHash string                  `json:"input_hash"`
		Proposal  core.RPDecisionProposal `json:"proposal"`
	}{inputHash, decision.Proposal})
	if err != nil {
		return RPNPCDecisionCommitResult{}, err
	}
	batchHash, err := core.HashJSON(struct {
		CommandID string `json:"command_id"`
		Sequence  int64  `json:"sequence"`
		EventType string `json:"event_type"`
		Payload   any    `json:"payload"`
	}{commandID, sequence, eventType, payload})
	if err != nil {
		return RPNPCDecisionCommitResult{}, err
	}
	if expression != nil {
		batchHash, err = core.HashJSON(struct {
			CommandID  string                `json:"command_id"`
			Sequence   int64                 `json:"sequence"`
			EventType  string                `json:"event_type"`
			Payload    any                   `json:"payload"`
			Expression *core.RPNonverbalFact `json:"expression"`
		}{commandID, sequence, eventType, payload, expression})
		if err != nil {
			return RPNPCDecisionCommitResult{}, err
		}
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	statements := []struct {
		name, query string
		args        []any
	}{
		{"NPC effect command", `INSERT INTO commands(command_id, instance_id, branch_id, command_type, idempotency_key, request_hash, expected_head, principal_id, command_policy, status, created_at_utc) VALUES (?, ?, ?, 'RPNPCDecision', ?, ?, ?, ?, '{"authorization":"rp-npc-scoped-decision"}', 'pending', ?)`, []any{commandID, session.InstanceID, session.BranchID, decisionID, requestHash, head, request.PrincipalID, now}},
		{"NPC effect attempt", `INSERT INTO command_attempts(command_id, attempt_no, attempt_id, status, lease_owner, lease_until_utc, proposal_hash, created_at_utc) VALUES (?, 1, ?, 'ready', 'corerp-rp1', ?, ?, ?)`, []any{commandID, attemptID, s.now().UTC().Add(30 * time.Second).Format(time.RFC3339Nano), proposalHash, now}},
		{"NPC effect batch", `INSERT INTO event_batches(batch_id, command_id, attempt_no, instance_id, branch_id, epoch_id, expected_head, first_sequence, last_sequence, event_count, world_time, batch_hash, committed_at_utc) VALUES (?, ?, 1, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, []any{batchID, commandID, session.InstanceID, session.BranchID, epochID, head, sequence, lastSequence, eventCount, worldTime, batchHash, now}},
		{"NPC effect event", `INSERT INTO events(event_id, batch_id, instance_id, branch_id, event_sequence, batch_index, event_type, actor_id, world_time, payload) VALUES (?, ?, ?, ?, ?, 0, ?, ?, ?, ?)`, []any{eventID, batchID, session.InstanceID, session.BranchID, sequence, eventType, request.NPCEntityID, worldTime, string(payloadJSON)}},
		{"NPC committed decision", `INSERT INTO rp_npc_decisions(decision_id, session_id, parent_turn_id, npc_entity_id, event_id, action, input_hash, proposal_hash, proposal_json) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, []any{decisionID, session.SessionID, request.TurnID, request.NPCEntityID, eventID, action, inputHash, proposalHash, string(proposalJSON)}},
	}
	for _, statement := range statements {
		if err := execAgentOne(ctx, tx.conn, statement.name, statement.query, statement.args...); err != nil {
			return RPNPCDecisionCommitResult{}, err
		}
	}
	if action == "respond" || action == "refuse" {
		if err := execAgentOne(ctx, tx.conn, "NPC immutable utterance", `INSERT INTO rp_utterances(utterance_id, event_id, session_id, turn_id, speaker_entity_id, place_id, world_time, speech_text, speech_act, listener_count) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'statement', ?)`, utteranceID, eventID, session.SessionID, childTurnID, request.NPCEntityID, input.PlaceID, worldTime, decision.Proposal.Text, len(listeners)); err != nil {
			return RPNPCDecisionCommitResult{}, err
		}
		if err := insertRPSpeechHearings(ctx, tx.conn, eventID, sequence, request.NPCEntityID, input.PlaceID, worldTime, utteranceID, decision.Proposal.Text, "statement", listeners); err != nil {
			return RPNPCDecisionCommitResult{}, err
		}
		if acceptedIntroduction {
			for _, listener := range listeners {
				if _, err := tx.conn.ExecContext(ctx, `INSERT OR IGNORE INTO rp_identity_familiarity(observer_agent_id,subject_agent_id,instance_id,branch_id,source_event_id,learned_world_time,origin_kind) VALUES (?,?,?,?,?,?,'introduction')`, listener, request.NPCEntityID, session.InstanceID, session.BranchID, eventID, worldTime); err != nil {
					return RPNPCDecisionCommitResult{}, core.WrapError(core.CodeStorageFailure, "record heard NPC self-introduction", err)
				}
			}
		}
	} else if action == "leave" {
		if err := commitRPNPCMovement(ctx, tx.conn, session.InstanceID, session.BranchID, eventID, sequence, request.NPCEntityID, input.PlaceID, decision.Proposal.DestinationPlaceID, worldTime, currentDay, suffix); err != nil {
			return RPNPCDecisionCommitResult{}, err
		}
	} else if action == "act" {
		if err := execAgentOne(ctx, tx.conn, "NPC activity state", `INSERT INTO rp_activities(activity_id, actor_id, place_id, activity_code, started_world_time, duration_minutes, status, start_event_id, instance_id, branch_id, last_event_sequence) VALUES (?, ?, ?, ?, ?, ?, 'in_progress', ?, ?, ?, ?)`, activityID, request.NPCEntityID, input.PlaceID, decision.Proposal.ActivityCode, worldTime, activityDuration, eventID, session.InstanceID, session.BranchID, sequence); err != nil {
			return RPNPCDecisionCommitResult{}, err
		}
		if err := execAgentOne(ctx, tx.conn, "NPC activity position", `UPDATE agent_positions SET activity_code = ?, effective_world_time = ?, projection_version = projection_version + 1, last_event_sequence = ? WHERE agent_id = ? AND place_id = ?`, decision.Proposal.ActivityCode, worldTime, sequence, request.NPCEntityID, input.PlaceID); err != nil {
			return RPNPCDecisionCommitResult{}, err
		}
	}
	switch action {
	case "respond", "refuse":
		if err := insertRPOwnAction(ctx, tx.conn, request.NPCEntityID, eventID, "speech", "", decision.Proposal.Text, input.PlaceID, worldTime, "", session.InstanceID, session.BranchID, sequence); err != nil {
			return RPNPCDecisionCommitResult{}, err
		}
	case "leave":
		if err := insertRPOwnAction(ctx, tx.conn, request.NPCEntityID, eventID, "leave", "", "", decision.Proposal.DestinationPlaceID, worldTime, "", session.InstanceID, session.BranchID, sequence); err != nil {
			return RPNPCDecisionCommitResult{}, err
		}
	case "act":
		if err := insertRPOwnAction(ctx, tx.conn, request.NPCEntityID, eventID, "activity", decision.Proposal.ActivityCode, "", input.PlaceID, worldTime, "in_progress", session.InstanceID, session.BranchID, sequence); err != nil {
			return RPNPCDecisionCommitResult{}, err
		}
	default:
		if err := insertRPOwnAction(ctx, tx.conn, request.NPCEntityID, eventID, action, "", "", input.PlaceID, worldTime, "", session.InstanceID, session.BranchID, sequence); err != nil {
			return RPNPCDecisionCommitResult{}, err
		}
	}
	if err := commitRPNPCExpression(ctx, tx.conn, input, expression, expressionID, eventID, batchID, lastSequence); err != nil {
		return RPNPCDecisionCommitResult{}, err
	}
	if err := execAgentOne(ctx, tx.conn, "advance NPC effect clock lineage", `UPDATE world_clocks SET projection_version = projection_version + 1, last_event_sequence = ? WHERE instance_id = ? AND branch_id = ? AND current_world_time = ?`, lastSequence, session.InstanceID, session.BranchID, worldTime); err != nil {
		return RPNPCDecisionCommitResult{}, err
	}
	if err := execAgentOne(ctx, tx.conn, "advance NPC effect branch", `UPDATE branches SET head_sequence = ? WHERE instance_id = ? AND branch_id = ? AND head_sequence = ?`, lastSequence, session.InstanceID, session.BranchID, head); err != nil {
		return RPNPCDecisionCommitResult{}, err
	}
	if err := execAgentOne(ctx, tx.conn, "advance NPC effect turn stage", `UPDATE rp_sessions SET turn_state = 'npc_effects_committed' WHERE session_id = ? AND turn_cursor = ? AND status = 'active'`, session.SessionID, request.TurnID); err != nil {
		return RPNPCDecisionCommitResult{}, err
	}
	if err := s.insertAgentAudit(ctx, tx.conn, "audit_"+commandID, "agent_decision", eventID, commandID, attemptID, worldTime, now, payloadJSON); err != nil {
		return RPNPCDecisionCommitResult{}, err
	}
	outboxAudience := []string{session.ControlledEntityID}
	if action == "respond" || action == "refuse" {
		outboxAudience = listeners
	}
	outboxTopic := "rp.npc.decision"
	if action == "respond" || action == "refuse" {
		outboxTopic = "rp.speech.accepted"
	}
	if err := insertRPParticipantOutbox(ctx, tx.conn, "outbox_"+commandID, eventID, outboxTopic, session.InstanceID, session.BranchID, request.NPCEntityID, outboxAudience, payloadJSON); err != nil {
		return RPNPCDecisionCommitResult{}, err
	}
	if err := execAgentOne(ctx, tx.conn, "commit NPC effect attempt", `UPDATE command_attempts SET status = 'committed', finished_at_utc = ? WHERE command_id = ? AND attempt_no = 1 AND status = 'ready'`, now, commandID); err != nil {
		return RPNPCDecisionCommitResult{}, err
	}
	if err := execAgentOne(ctx, tx.conn, "commit NPC effect command", `UPDATE commands SET status = 'committed' WHERE command_id = ? AND status = 'pending'`, commandID); err != nil {
		return RPNPCDecisionCommitResult{}, err
	}
	if s.beforeCommit != nil {
		if err := s.beforeCommit(); err != nil {
			return RPNPCDecisionCommitResult{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return RPNPCDecisionCommitResult{}, core.WrapError(core.CodeStorageFailure, "commit NPC effect", err)
	}
	return RPNPCDecisionCommitResult{DecisionID: decisionID, EventID: eventID, EventSequence: sequence, ParentTurnID: request.TurnID, NPCEntityID: request.NPCEntityID, Action: action, WorldTime: worldTime}, nil
}

func (s *Store) findCommittedRPDecision(ctx context.Context, request core.RPDecisionRequest, inputHash, proposalHash string) (RPNPCDecisionCommitResult, bool, error) {
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return RPNPCDecisionCommitResult{}, false, core.WrapError(core.CodeStorageFailure, "begin NPC decision retry lookup", err)
	}
	defer tx.Rollback(ctx)
	session, err := loadRPSession(ctx, tx.conn, request.PrincipalID, request.SessionID)
	if err != nil {
		return RPNPCDecisionCommitResult{}, false, err
	}
	if err := authorizeRPControl(ctx, tx.conn, request.PrincipalID, session.InstanceID, session.BranchID, session.ControlledEntityID); err != nil {
		return RPNPCDecisionCommitResult{}, false, err
	}
	return findCommittedRPDecisionOnConn(ctx, tx.conn, session.SessionID, request, inputHash, proposalHash)
}

func findCommittedRPDecisionOnConn(ctx context.Context, conn *sql.Conn, sessionID string, request core.RPDecisionRequest, inputHash, proposalHash string) (RPNPCDecisionCommitResult, bool, error) {
	var result RPNPCDecisionCommitResult
	var storedInputHash, storedProposalHash string
	err := conn.QueryRowContext(ctx, `
		SELECT d.decision_id, d.event_id, e.event_sequence, e.world_time, d.action, d.input_hash, d.proposal_hash
		FROM rp_npc_decisions d JOIN events e ON e.event_id = d.event_id
		WHERE d.session_id = ? AND d.parent_turn_id = ? AND d.npc_entity_id = ?`, sessionID, request.TurnID, request.NPCEntityID,
	).Scan(&result.DecisionID, &result.EventID, &result.EventSequence, &result.WorldTime, &result.Action, &storedInputHash, &storedProposalHash)
	if errors.Is(err, sql.ErrNoRows) {
		return RPNPCDecisionCommitResult{}, false, nil
	}
	if err != nil {
		return RPNPCDecisionCommitResult{}, false, core.WrapError(core.CodeStorageFailure, "load committed NPC decision", err)
	}
	if inputHash != storedInputHash || proposalHash != storedProposalHash {
		return RPNPCDecisionCommitResult{}, true, core.NewError(core.CodeIdempotencyMismatch, "NPC decision for this turn was already committed differently")
	}
	result.ParentTurnID = request.TurnID
	result.NPCEntityID = request.NPCEntityID
	result.Replayed = true
	return result, true, nil
}

func commitRPNPCMovement(ctx context.Context, conn *sql.Conn, instanceID, branchID, eventID string, sequence int64, npcID, fromPlaceID, toPlaceID, worldTime string, currentDay int64, suffix string) error {
	var positionVersion int64
	if err := conn.QueryRowContext(ctx, `SELECT projection_version FROM agent_positions WHERE agent_id = ? AND place_id = ?`, npcID, fromPlaceID).Scan(&positionVersion); err != nil {
		return classifyMissing(err, "NPC movement origin")
	}
	var reachable int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_place_links link JOIN agent_places destination ON destination.place_id = link.to_place_id WHERE link.instance_id = ? AND link.branch_id = ? AND link.from_place_id = ? AND link.to_place_id = ? AND destination.status = 'active'`, instanceID, branchID, fromPlaceID, toPlaceID).Scan(&reachable); err != nil {
		return core.WrapError(core.CodeStorageFailure, "recheck NPC movement route", err)
	}
	if reachable != 1 {
		return core.NewError(core.CodeBranchConflict, "NPC destination route is no longer valid")
	}
	open, err := rpTransitAllowsImmediate(ctx, conn, instanceID, branchID, fromPlaceID, toPlaceID, worldTime)
	if err != nil {
		return err
	}
	if !open {
		return core.NewError(core.CodeBranchConflict, "NPC route is temporarily obstructed")
	}
	scheduleID, itemID := "schedule_rp_npc_"+suffix, "sched_rp_npc_"+suffix
	itemPayloadJSON, err := core.CanonicalJSON(agentSchedulePayload{Kind: "agent_move", Day: int(currentDay), AgentID: npcID, ScheduleID: scheduleID, ToPlaceID: toPlaceID, ActivityCode: "rp_npc_leave"})
	if err != nil {
		return err
	}
	if err := execAgentOne(ctx, conn, "NPC completed movement item", `INSERT INTO scheduler_items(scheduler_item_id, instance_id, branch_id, world_time, phase_id, declared_priority, status, payload) VALUES (?, ?, ?, ?, ?, 0, 'completed', ?)`, itemID, instanceID, branchID, worldTime, m2AgentPhaseID, string(itemPayloadJSON)); err != nil {
		return err
	}
	if err := execAgentOne(ctx, conn, "NPC completed movement schedule", `INSERT INTO agent_schedule_entries(schedule_id, agent_id, world_time, place_id, activity_code, declared_priority, scheduler_item_id, status, definition_event_id) VALUES (?, ?, ?, ?, 'rp_npc_leave', 0, ?, 'completed', ?)`, scheduleID, npcID, worldTime, toPlaceID, itemID, eventID); err != nil {
		return err
	}
	if err := execAgentOne(ctx, conn, "NPC movement fact", `INSERT INTO agent_movements(movement_id, event_id, agent_id, from_place_id, to_place_id, schedule_id, activity_code, world_time, movement_kind) VALUES (?, ?, ?, ?, ?, ?, 'rp_npc_leave', ?, 'scheduled')`, "movement_rp_npc_"+suffix, eventID, npcID, fromPlaceID, toPlaceID, scheduleID, worldTime); err != nil {
		return err
	}
	if err := execAgentOne(ctx, conn, "NPC movement position", `UPDATE agent_positions SET place_id = ?, activity_code = 'rp_npc_leave', effective_world_time = ?, projection_version = projection_version + 1, last_event_sequence = ? WHERE agent_id = ? AND projection_version = ?`, toPlaceID, worldTime, sequence, npcID, positionVersion); err != nil {
		return err
	}
	coLocated, err := rpCoLocatedEntityIDs(ctx, conn, instanceID, branchID, toPlaceID, npcID)
	if err != nil {
		return err
	}
	for _, otherID := range coLocated {
		for _, pair := range [][2]string{{npcID, otherID}, {otherID, npcID}} {
			if err := upsertCoLocationKnowledge(ctx, conn, eventID, sequence, pair[0], pair[1], toPlaceID, worldTime); err != nil {
				return err
			}
		}
	}
	return nil
}
