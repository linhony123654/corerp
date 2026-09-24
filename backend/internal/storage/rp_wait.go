package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"corerp.local/backend/internal/core"
)

type RPWaitResult struct {
	WarmNPCIDs       []string             `json:"-"`
	InitiativeNPCIDs []string             `json:"initiative_npc_ids,omitempty"`
	Initiatives      []RPInitiativeResult `json:"initiatives,omitempty"`
	IntentID         string               `json:"intent_id"`
	Status           string               `json:"status"`
	TargetWorldTime  string               `json:"target_world_time"`
	CurrentWorldTime string               `json:"current_world_time"`
	ProcessedItems   int                  `json:"processed_items"`
	PendingDue       int64                `json:"pending_due"`
	CommandID        string               `json:"command_id,omitempty"`
	EventID          string               `json:"event_id,omitempty"`
	EventSequence    int64                `json:"event_sequence,omitempty"`
	Replayed         bool                 `json:"replayed"`
}

type rpWaitEvent struct {
	VisitOpportunities     []rpVisitOpportunity     `json:"visit_opportunities,omitempty"`
	CommunityOpportunities []rpCommunityOpportunity `json:"community_opportunities,omitempty"`
	WarmCandidates         []rpWarmCandidate        `json:"warm_candidates,omitempty"`
	OpportunityIntent      string                   `json:"opportunity_intent,omitempty"`
	WorkOpportunities      []rpWorkOpportunity      `json:"work_opportunities,omitempty"`
	StoreOpportunities     []rpStoreOpportunity     `json:"store_opportunities,omitempty"`
	Environment            *rpEnvironmentCondition  `json:"environment,omitempty"`
	ContactOpportunities   []rpContactOpportunity   `json:"contact_opportunities,omitempty"`
	InitiativeNPCIDs       []string                 `json:"initiative_npc_ids,omitempty"`
	SessionID              string                   `json:"session_id"`
	EntityID               string                   `json:"entity_id"`
	FromWorldTime          string                   `json:"from_world_time"`
	TargetWorldTime        string                   `json:"target_world_time"`
	ProcessedItems         int                      `json:"processed_items"`
}

// WaitRP stores only retry intent before invoking the existing M2 scheduler.
// The final clock and event are one authority commit after all due work drains.
func (s *Store) WaitRP(ctx context.Context, request core.RPWaitRequest) (RPWaitResult, error) {
	if err := request.Validate(); err != nil {
		return RPWaitResult{}, err
	}
	target, _ := time.Parse(time.RFC3339, request.TargetWorldTime)
	request.TargetWorldTime = target.UTC().Format(time.RFC3339)
	requestHash, err := core.HashJSON(request)
	if err != nil {
		return RPWaitResult{}, err
	}
	intent, replayed, err := s.ensureRPWaitIntent(ctx, request, requestHash)
	if err != nil || replayed {
		return intent, err
	}
	run, err := s.RunAgentLife(ctx, request.TargetWorldTime, request.Budget)
	if err != nil {
		return RPWaitResult{}, err
	}
	if run.PendingDue > 0 {
		intent.Status = "budget_exhausted"
		intent.CurrentWorldTime = run.CurrentWorldTime
		intent.ProcessedItems = run.ProcessedItems
		intent.PendingDue = run.PendingDue
		return intent, nil
	}
	return s.finishRPWait(ctx, request, requestHash, intent.IntentID, run.ProcessedItems)
}

func (s *Store) ensureRPWaitIntent(ctx context.Context, request core.RPWaitRequest, requestHash string) (RPWaitResult, bool, error) {
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return RPWaitResult{}, false, core.WrapError(core.CodeStorageFailure, "begin RP wait intent", err)
	}
	defer tx.Rollback(ctx)
	session, err := loadRPSession(ctx, tx.conn, request.PrincipalID, request.SessionID)
	if err != nil {
		return RPWaitResult{}, false, err
	}
	if err := authorizeRPControl(ctx, tx.conn, request.PrincipalID, session.InstanceID, session.BranchID, session.ControlledEntityID); err != nil {
		return RPWaitResult{}, false, err
	}
	if session.InstanceID != M2DemoInstanceID || session.BranchID != M2DemoBranchID {
		return RPWaitResult{}, false, core.NewError(core.CodeNotFound, "bounded M2 RP world not found")
	}
	var intentID, existingHash, status string
	var commandID sql.NullString
	err = tx.conn.QueryRowContext(ctx, `SELECT intent_id, request_hash, status, command_id FROM rp_wait_intents WHERE session_id = ? AND idempotency_key = ?`, session.SessionID, request.IdempotencyKey).Scan(&intentID, &existingHash, &status, &commandID)
	if err == nil {
		if existingHash != requestHash {
			return RPWaitResult{}, false, core.NewError(core.CodeIdempotencyMismatch, "RP wait key was used with another request")
		}
		if status == "completed" {
			result, err := loadRPWaitResult(ctx, tx.conn, intentID, commandID.String, true)
			return result, true, err
		}
		if session.Status != "active" {
			return RPWaitResult{}, false, core.NewError(core.CodeBranchConflict, "RP session is closed")
		}
		if err := validateRPBinding(ctx, tx.conn, session.InstanceID, session.BranchID, session.ControlledEntityID); err != nil {
			return RPWaitResult{}, false, err
		}
		return RPWaitResult{IntentID: intentID, Status: "pending", TargetWorldTime: request.TargetWorldTime}, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return RPWaitResult{}, false, core.WrapError(core.CodeStorageFailure, "look up RP wait intent", err)
	}
	if session.Status != "active" {
		return RPWaitResult{}, false, core.NewError(core.CodeBranchConflict, "RP session is closed")
	}
	if err := validateRPBinding(ctx, tx.conn, session.InstanceID, session.BranchID, session.ControlledEntityID); err != nil {
		return RPWaitResult{}, false, err
	}
	var pending int
	if err := tx.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_wait_intents i JOIN rp_sessions s ON s.session_id = i.session_id WHERE s.instance_id = ? AND s.branch_id = ? AND i.status = 'pending'`, session.InstanceID, session.BranchID).Scan(&pending); err != nil {
		return RPWaitResult{}, false, core.WrapError(core.CodeStorageFailure, "check pending RP wait", err)
	}
	if pending != 0 {
		return RPWaitResult{}, false, core.NewError(core.CodeCommandInProgress, "another RP wait is pending in this world")
	}
	var pendingTurns int
	if err := tx.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_turn_runs r JOIN rp_sessions s ON s.session_id = r.session_id WHERE s.instance_id = ? AND s.branch_id = ? AND r.status <> 'settled'`, session.InstanceID, session.BranchID).Scan(&pendingTurns); err != nil {
		return RPWaitResult{}, false, core.WrapError(core.CodeStorageFailure, "check pending RP turn before wait", err)
	}
	if pendingTurns != 0 {
		return RPWaitResult{}, false, core.NewError(core.CodeCommandInProgress, "RP turn must settle before waiting")
	}
	var head int64
	var currentText string
	if err := tx.conn.QueryRowContext(ctx, `SELECT b.head_sequence, c.current_world_time FROM branches b JOIN world_clocks c ON c.instance_id = b.instance_id AND c.branch_id = b.branch_id WHERE b.instance_id = ? AND b.branch_id = ?`, session.InstanceID, session.BranchID).Scan(&head, &currentText); err != nil {
		return RPWaitResult{}, false, classifyMissing(err, "RP wait world")
	}
	current, err := time.Parse(time.RFC3339, currentText)
	if err != nil {
		return RPWaitResult{}, false, core.WrapError(core.CodeProjectionDiverged, "invalid RP clock", err)
	}
	target, _ := time.Parse(time.RFC3339, request.TargetWorldTime)
	if !target.After(current) {
		return RPWaitResult{}, false, core.NewError(core.CodeInvalidArgument, "RP wait target must be after current world time")
	}
	if request.ExpectedCursor != head || session.ObservationCursor != head {
		return RPWaitResult{}, false, core.NewError(core.CodeBranchConflict, "RP wait requires a current observation cursor")
	}
	keyHash, err := core.HashJSON(struct{ SessionID, Key string }{session.SessionID, request.IdempotencyKey})
	if err != nil {
		return RPWaitResult{}, false, err
	}
	intentID = "intent_rp_wait_" + keyHash[7:]
	if err := execAgentOne(ctx, tx.conn, "save RP wait intent", `INSERT INTO rp_wait_intents(intent_id, session_id, idempotency_key, request_hash, target_world_time, status, created_at_utc) VALUES (?, ?, ?, ?, ?, 'pending', ?)`, intentID, session.SessionID, request.IdempotencyKey, requestHash, request.TargetWorldTime, s.now().UTC().Format(time.RFC3339Nano)); err != nil {
		return RPWaitResult{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return RPWaitResult{}, false, core.WrapError(core.CodeStorageFailure, "commit RP wait intent", err)
	}
	return RPWaitResult{IntentID: intentID, Status: "pending", TargetWorldTime: request.TargetWorldTime}, false, nil
}

func (s *Store) finishRPWait(ctx context.Context, request core.RPWaitRequest, requestHash, intentID string, processed int) (RPWaitResult, error) {
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return RPWaitResult{}, core.WrapError(core.CodeStorageFailure, "begin RP wait completion", err)
	}
	defer tx.Rollback(ctx)
	session, err := loadRPSession(ctx, tx.conn, request.PrincipalID, request.SessionID)
	if err != nil {
		return RPWaitResult{}, err
	}
	if err := authorizeRPControl(ctx, tx.conn, request.PrincipalID, session.InstanceID, session.BranchID, session.ControlledEntityID); err != nil {
		return RPWaitResult{}, err
	}
	var existingHash, status string
	var existingCommand sql.NullString
	if err := tx.conn.QueryRowContext(ctx, `SELECT request_hash, status, command_id FROM rp_wait_intents WHERE intent_id = ? AND session_id = ?`, intentID, session.SessionID).Scan(&existingHash, &status, &existingCommand); err != nil {
		return RPWaitResult{}, classifyMissing(err, "RP wait intent")
	}
	if existingHash != requestHash {
		return RPWaitResult{}, core.NewError(core.CodeIdempotencyMismatch, "RP wait intent changed")
	}
	if status == "completed" {
		return loadRPWaitResult(ctx, tx.conn, intentID, existingCommand.String, true)
	}
	if session.Status != "active" {
		return RPWaitResult{}, core.NewError(core.CodeBranchConflict, "RP session is closed")
	}
	var head, currentDay, pending int64
	var currentText string
	if err := tx.conn.QueryRowContext(ctx, `SELECT b.head_sequence, c.current_world_time, c.current_day FROM branches b JOIN world_clocks c ON c.instance_id = b.instance_id AND c.branch_id = b.branch_id WHERE b.instance_id = ? AND b.branch_id = ?`, session.InstanceID, session.BranchID).Scan(&head, &currentText, &currentDay); err != nil {
		return RPWaitResult{}, classifyMissing(err, "RP wait final clock")
	}
	if err := tx.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM scheduler_items WHERE instance_id = ? AND branch_id = ? AND status = 'pending' AND world_time <= ?`, session.InstanceID, session.BranchID, request.TargetWorldTime).Scan(&pending); err != nil {
		return RPWaitResult{}, core.WrapError(core.CodeStorageFailure, "check due work before RP wait completion", err)
	}
	if pending != 0 {
		return RPWaitResult{}, core.NewError(core.CodeCommandInProgress, "RP wait still has due scheduler work")
	}
	current, err := time.Parse(time.RFC3339, currentText)
	if err != nil {
		return RPWaitResult{}, core.WrapError(core.CodeProjectionDiverged, "invalid RP clock", err)
	}
	target, _ := time.Parse(time.RFC3339, request.TargetWorldTime)
	if target.Before(current) {
		return RPWaitResult{}, core.NewError(core.CodeBranchConflict, "RP wait target predates committed world time")
	}
	if err := ensureCohortTransitionChronology(ctx, tx.conn, session.InstanceID, session.BranchID, request.TargetWorldTime); err != nil {
		return RPWaitResult{}, err
	}
	sequence := head + 1
	var epochID string
	if err := tx.conn.QueryRowContext(ctx, `SELECT epoch_id FROM rule_epochs WHERE instance_id = ? AND branch_id = ? AND start_sequence <= ? AND (end_sequence IS NULL OR ? < end_sequence)`, session.InstanceID, session.BranchID, sequence, sequence).Scan(&epochID); err != nil {
		return RPWaitResult{}, classifyMissing(err, "RP wait Rule Epoch")
	}
	suffix := intentID[len("intent_rp_wait_"):]
	commandID, attemptID, batchID, eventID := "cmd_rp_wait_"+suffix, "attempt_rp_wait_"+suffix, "batch_rp_wait_"+suffix, "event_rp_wait_"+suffix
	var playerPlace string
	if err := tx.conn.QueryRowContext(ctx, `SELECT place_id FROM agent_positions WHERE agent_id=?`, session.ControlledEntityID).Scan(&playerPlace); err != nil {
		return RPWaitResult{}, err
	}
	npcs, err := readRPHotInitiativeRoster(ctx, tx.conn, session.InstanceID, session.BranchID, playerPlace, session.ControlledEntityID)
	if err != nil {
		return RPWaitResult{}, err
	}
	// A bounded nearby cohort for this request, not an unbounded background loop.
	// Persist identities before calling providers so recovery cannot select a
	// different scene after an earlier initiative has moved an actor.
	payload := rpWaitEvent{InitiativeNPCIDs: npcs, SessionID: session.SessionID, EntityID: session.ControlledEntityID, FromWorldTime: currentText, TargetWorldTime: request.TargetWorldTime, ProcessedItems: processed}
	payload.OpportunityIntent = request.OpportunityIntent
	payload.WarmCandidates, err = readRPWarmCandidates(ctx, tx.conn, session, playerPlace, request.TargetWorldTime)
	if err != nil {
		return RPWaitResult{}, err
	}
	payload.Environment, err = materializeRPEnvironment(ctx, tx.conn, session.InstanceID, session.BranchID, playerPlace, request.TargetWorldTime, eventID)
	if err != nil {
		return RPWaitResult{}, err
	}
	payload.ContactOpportunities, err = evaluateRPContactOpportunities(ctx, tx.conn, session, npcs, request.TargetWorldTime, payload)
	if err != nil {
		return RPWaitResult{}, err
	}
	payload.StoreOpportunities, err = evaluateRPStoreOpportunities(ctx, tx.conn, session, npcs, request.TargetWorldTime, payload)
	if err != nil {
		return RPWaitResult{}, err
	}
	payload.WorkOpportunities, err = evaluateRPWorkOpportunities(ctx, tx.conn, session, npcs, request.TargetWorldTime, payload)
	if err != nil {
		return RPWaitResult{}, err
	}
	payload.CommunityOpportunities, err = evaluateRPCommunityOpportunities(ctx, tx.conn, session, npcs, request.TargetWorldTime, payload)
	if err != nil {
		return RPWaitResult{}, err
	}
	payload.VisitOpportunities, err = evaluateRPVisitOpportunities(ctx, tx.conn, session, npcs, request.TargetWorldTime, payload)
	if err != nil {
		return RPWaitResult{}, err
	}
	payloadJSON, err := core.CanonicalJSON(payload)
	if err != nil {
		return RPWaitResult{}, err
	}
	batchHash, err := core.HashJSON(struct {
		CommandID string      `json:"command_id"`
		Sequence  int64       `json:"sequence"`
		Payload   rpWaitEvent `json:"payload"`
	}{commandID, sequence, payload})
	if err != nil {
		return RPWaitResult{}, err
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	statements := []struct {
		name, query string
		args        []any
	}{
		{"RP wait command", `INSERT INTO commands(command_id, instance_id, branch_id, command_type, idempotency_key, request_hash, expected_head, principal_id, command_policy, status, created_at_utc) VALUES (?, ?, ?, 'RPPlayerWait', ?, ?, ?, ?, '{"authorization":"rp-session-control"}', 'pending', ?)`, []any{commandID, session.InstanceID, session.BranchID, "rp_wait:" + session.SessionID + ":" + request.IdempotencyKey, requestHash, head, request.PrincipalID, now}},
		{"RP wait attempt", `INSERT INTO command_attempts(command_id, attempt_no, attempt_id, status, lease_owner, lease_until_utc, proposal_hash, created_at_utc) VALUES (?, 1, ?, 'ready', 'corerp-rp1', ?, ?, ?)`, []any{commandID, attemptID, s.now().UTC().Add(30 * time.Second).Format(time.RFC3339Nano), requestHash, now}},
		{"RP wait batch", `INSERT INTO event_batches(batch_id, command_id, attempt_no, instance_id, branch_id, epoch_id, expected_head, first_sequence, last_sequence, event_count, world_time, batch_hash, committed_at_utc) VALUES (?, ?, 1, ?, ?, ?, ?, ?, ?, 1, ?, ?, ?)`, []any{batchID, commandID, session.InstanceID, session.BranchID, epochID, head, sequence, sequence, request.TargetWorldTime, batchHash, now}},
		{"RP wait event", `INSERT INTO events(event_id, batch_id, instance_id, branch_id, event_sequence, batch_index, event_type, actor_id, world_time, payload) VALUES (?, ?, ?, ?, ?, 0, 'RPWaitCompleted', ?, ?, ?)`, []any{eventID, batchID, session.InstanceID, session.BranchID, sequence, session.ControlledEntityID, request.TargetWorldTime, string(payloadJSON)}},
	}
	for _, statement := range statements {
		if err := execAgentOne(ctx, tx.conn, statement.name, statement.query, statement.args...); err != nil {
			return RPWaitResult{}, err
		}
	}
	base, _ := time.Parse(time.RFC3339, "2026-09-22T00:00:00Z")
	day := int64(target.Sub(base) / (24 * time.Hour))
	if day < currentDay {
		return RPWaitResult{}, core.NewError(core.CodeProjectionDiverged, "RP wait would reduce world day")
	}
	if err := execAgentOne(ctx, tx.conn, "advance RP wait clock", `UPDATE world_clocks SET current_world_time = ?, current_day = ?, projection_version = projection_version + 1, last_event_sequence = ? WHERE instance_id = ? AND branch_id = ? AND current_world_time = ?`, request.TargetWorldTime, day, sequence, session.InstanceID, session.BranchID, currentText); err != nil {
		return RPWaitResult{}, err
	}
	if err := execAgentOne(ctx, tx.conn, "advance RP wait branch", `UPDATE branches SET head_sequence = ? WHERE instance_id = ? AND branch_id = ? AND head_sequence = ?`, sequence, session.InstanceID, session.BranchID, head); err != nil {
		return RPWaitResult{}, err
	}
	if err := s.insertAgentAudit(ctx, tx.conn, "audit_"+commandID, "agent_decision", eventID, commandID, attemptID, request.TargetWorldTime, now, payloadJSON); err != nil {
		return RPWaitResult{}, err
	}
	publicPayload := payload
	publicPayload.Environment = nil
	publicPayload.ContactOpportunities = nil
	publicPayload.StoreOpportunities = nil
	publicPayload.WorkOpportunities = nil
	publicPayload.CommunityOpportunities = nil
	publicPayload.VisitOpportunities = nil
	publicPayload.OpportunityIntent = ""
	publicPayload.WarmCandidates = nil
	publicJSON, err := core.CanonicalJSON(publicPayload)
	if err != nil {
		return RPWaitResult{}, err
	}
	if err := insertAgentOutbox(ctx, tx.conn, "outbox_"+commandID, eventID, "rp.player.wait_completed", publicJSON); err != nil {
		return RPWaitResult{}, err
	}
	if err := execAgentOne(ctx, tx.conn, "commit RP wait attempt", `UPDATE command_attempts SET status = 'committed', finished_at_utc = ? WHERE command_id = ? AND attempt_no = 1 AND status = 'ready'`, now, commandID); err != nil {
		return RPWaitResult{}, err
	}
	if err := execAgentOne(ctx, tx.conn, "commit RP wait command", `UPDATE commands SET status = 'committed' WHERE command_id = ? AND status = 'pending'`, commandID); err != nil {
		return RPWaitResult{}, err
	}
	if err := execAgentOne(ctx, tx.conn, "complete RP wait intent", `UPDATE rp_wait_intents SET status = 'completed', command_id = ?, completed_at_utc = ? WHERE intent_id = ? AND status = 'pending'`, commandID, now, intentID); err != nil {
		return RPWaitResult{}, err
	}
	if s.beforeCommit != nil {
		if err := s.beforeCommit(); err != nil {
			return RPWaitResult{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return RPWaitResult{}, core.WrapError(core.CodeStorageFailure, "commit RP wait completion", err)
	}
	return RPWaitResult{WarmNPCIDs: rpWarmActorIDs(payload.WarmCandidates), InitiativeNPCIDs: npcs, IntentID: intentID, Status: "completed", TargetWorldTime: request.TargetWorldTime, CurrentWorldTime: request.TargetWorldTime, ProcessedItems: processed, CommandID: commandID, EventID: eventID, EventSequence: sequence}, nil
}

func loadRPWaitResult(ctx context.Context, conn *sql.Conn, intentID, commandID string, replayed bool) (RPWaitResult, error) {
	var result RPWaitResult
	var payloadJSON string
	if err := conn.QueryRowContext(ctx, `SELECT e.event_id, e.event_sequence, e.world_time, e.payload FROM events e JOIN event_batches b ON b.batch_id = e.batch_id WHERE b.command_id = ? AND e.event_type = 'RPWaitCompleted'`, commandID).Scan(&result.EventID, &result.EventSequence, &result.CurrentWorldTime, &payloadJSON); err != nil {
		return RPWaitResult{}, classifyMissing(err, "committed RP wait event")
	}
	var payload rpWaitEvent
	if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
		return RPWaitResult{}, core.WrapError(core.CodeStorageFailure, "decode committed RP wait", err)
	}
	result.IntentID = intentID
	result.Status = "completed"
	result.TargetWorldTime = payload.TargetWorldTime
	result.ProcessedItems = payload.ProcessedItems
	result.CommandID = commandID
	result.Replayed = replayed
	result.InitiativeNPCIDs = payload.InitiativeNPCIDs
	result.WarmNPCIDs = rpWarmActorIDs(payload.WarmCandidates)
	return result, nil
}

func rpWarmActorIDs(candidates []rpWarmCandidate) []string {
	var ids []string
	for _, c := range candidates {
		ids = append(ids, c.ActorID)
	}
	return ids
}
