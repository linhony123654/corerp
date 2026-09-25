package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"corerp.local/backend/internal/core"
)

type RPInteractionOutcome struct {
	Kind            string `json:"kind"`
	EventID         string `json:"event_id"`
	EventSequence   int64  `json:"event_sequence"`
	SettledSequence int64  `json:"settled_sequence"`
	TurnRunID       string `json:"turn_run_id,omitempty"`
	WorldTime       string `json:"world_time,omitempty"`
}

type RPInteractionResult struct {
	InteractionID string                 `json:"interaction_id"`
	PlanKind      string                 `json:"plan_kind"`
	Status        string                 `json:"status"`
	Clarification string                 `json:"clarification,omitempty"`
	PauseReason   string                 `json:"pause_reason,omitempty"`
	NextStep      int                    `json:"next_step"`
	Outcomes      []RPInteractionOutcome `json:"outcomes"`
	Replayed      bool                   `json:"replayed"`
}

type RPInteractionResumeRequest struct {
	PrincipalID    string `json:"principal_id"`
	SessionID      string `json:"session_id"`
	IdempotencyKey string `json:"idempotency_key"`
}

type rpInteraction struct {
	ID             string
	SessionID      string
	Key            string
	RequestHash    string
	RequestJSON    string
	Plan           core.RPInteractionPlan
	Status         string
	PauseReason    string
	NextStep       int
	PendingKind    string
	PendingRequest string
	Outcomes       []RPInteractionOutcome
}

type rpInteractionQuery interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// Store's local deterministic entry retains compatibility with existing
// embedded/local callers. The HTTP product normally uses an operator-bound
// RPService so decisions still use its configured provider.
func (s *Store) RunRPInteraction(ctx context.Context, r core.RPInteractionRequest) (RPInteractionResult, error) {
	service, err := NewRPService(s, core.DeterministicRPDecisionProvider{}, "deterministic")
	if err != nil {
		return RPInteractionResult{}, err
	}
	return service.RunRPInteraction(ctx, r)
}

func (s *Store) ResumeRPInteraction(ctx context.Context, r RPInteractionResumeRequest) (RPInteractionResult, error) {
	service, err := NewRPService(s, core.DeterministicRPDecisionProvider{}, "deterministic")
	if err != nil {
		return RPInteractionResult{}, err
	}
	return service.ResumeRPInteraction(ctx, r)
}

func (s *Store) StopRPInteraction(ctx context.Context, r RPInteractionResumeRequest) (RPInteractionResult, error) {
	service, err := NewRPService(s, core.DeterministicRPDecisionProvider{}, "deterministic")
	if err != nil {
		return RPInteractionResult{}, err
	}
	return service.StopRPInteraction(ctx, r)
}

func loadRPInteraction(ctx context.Context, q rpInteractionQuery, session, key string) (rpInteraction, error) {
	var run rpInteraction
	var planJSON, outcomesJSON string
	err := q.QueryRowContext(ctx, `SELECT interaction_id,session_id,idempotency_key,request_hash,request_json,plan_json,status,pause_reason,next_step,pending_kind,pending_request_json,outcomes_json FROM rp_interactions WHERE session_id=? AND idempotency_key=?`, session, key).Scan(&run.ID, &run.SessionID, &run.Key, &run.RequestHash, &run.RequestJSON, &planJSON, &run.Status, &run.PauseReason, &run.NextStep, &run.PendingKind, &run.PendingRequest, &outcomesJSON)
	if err != nil {
		return rpInteraction{}, err
	}
	if err := json.Unmarshal([]byte(planJSON), &run.Plan); err != nil {
		return rpInteraction{}, core.WrapError(core.CodeProjectionDiverged, "decode persisted interaction plan", err)
	}
	if err := json.Unmarshal([]byte(outcomesJSON), &run.Outcomes); err != nil {
		return rpInteraction{}, core.WrapError(core.CodeProjectionDiverged, "decode persisted interaction outcomes", err)
	}
	if len(run.Outcomes) != run.NextStep || run.NextStep > len(run.Plan.Steps) || (run.Status == "settled" && run.NextStep != len(run.Plan.Steps)) || (run.Status == "clarification" && (len(run.Plan.Steps) != 0 || run.Plan.Clarification == "")) {
		return rpInteraction{}, core.NewError(core.CodeProjectionDiverged, "interaction progress differs from persisted plan")
	}
	for _, outcome := range run.Outcomes {
		if outcome.EventID == "" || outcome.EventSequence < 1 || outcome.SettledSequence < outcome.EventSequence {
			return rpInteraction{}, core.NewError(core.CodeProjectionDiverged, "interaction outcome lacks a valid child event and settled cursor")
		}
	}
	return run, nil
}

func (run rpInteraction) result(replayed bool) RPInteractionResult {
	return RPInteractionResult{InteractionID: run.ID, PlanKind: run.Plan.Kind, Status: run.Status, Clarification: run.Plan.Clarification, PauseReason: run.PauseReason, NextStep: run.NextStep, Outcomes: run.Outcomes, Replayed: replayed}
}

// RunRPInteraction owns only plan/order/recovery. Every world effect is still
// committed by the existing typed RP owner with a child key saved beforehand.
func (s *RPService) RunRPInteraction(ctx context.Context, request core.RPInteractionRequest) (RPInteractionResult, error) {
	if err := request.Validate(); err != nil {
		return RPInteractionResult{}, err
	}
	run, replayed, err := s.ensureRPInteraction(ctx, request)
	if err != nil {
		return RPInteractionResult{}, err
	}
	if run.Status != "open" {
		return run.result(replayed), nil
	}
	for run.NextStep < len(run.Plan.Steps) {
		if run.PendingKind == "" {
			run, err = s.prepareRPInteractionStep(ctx, request, run)
			if err != nil {
				if run.PendingKind == "" && core.HasCode(err, core.CodeBranchConflict) {
					run, err = s.pauseRPInteraction(ctx, run, "世界状态已变化；已完成的行动不会回滚。请结束原计划并重新选择下一步。")
					if err == nil {
						return run.result(replayed), nil
					}
				}
				return run.result(replayed), err
			}
		}
		outcome, pending, err := s.executeRPInteractionStep(ctx, run)
		if err != nil {
			return run.result(replayed), err
		}
		if pending {
			result := run.result(replayed)
			result.Status = "budget_exhausted"
			return result, nil
		}
		if s.Store.afterRPInteractionStep != nil {
			if err := s.Store.afterRPInteractionStep(run.NextStep); err != nil {
				return run.result(replayed), err
			}
		}
		run, err = s.commitRPInteractionStep(ctx, run, outcome)
		if err != nil {
			return run.result(replayed), err
		}
	}
	return run.result(replayed), nil
}

// A pause is durable and deliberately does not rebase an already accepted
// plan. Stop releases only the application plan; it never undoes child facts.
func (s *RPService) pauseRPInteraction(ctx context.Context, run rpInteraction, reason string) (rpInteraction, error) {
	tx, err := beginImmediate(ctx, s.Store.db)
	if err != nil {
		return run, err
	}
	defer tx.Rollback(ctx)
	current, err := loadRPInteraction(ctx, tx.conn, run.SessionID, run.Key)
	if err != nil {
		return run, err
	}
	if current.Status != "open" || current.NextStep != run.NextStep || current.PendingKind != "" {
		return run, core.NewError(core.CodeCommandInProgress, "interaction changed before pause")
	}
	if _, err := tx.conn.ExecContext(ctx, `UPDATE rp_interactions SET status='paused',pause_reason=?,updated_at_utc=? WHERE interaction_id=? AND status='open' AND next_step=? AND pending_kind=''`, reason, s.Store.now().UTC().Format(time.RFC3339Nano), run.ID, run.NextStep); err != nil {
		return run, err
	}
	if err := tx.Commit(ctx); err != nil {
		return run, err
	}
	current.Status, current.PauseReason = "paused", reason
	return current, nil
}

func (s *RPService) StopRPInteraction(ctx context.Context, r RPInteractionResumeRequest) (RPInteractionResult, error) {
	if r.PrincipalID == "" || r.SessionID == "" || r.IdempotencyKey == "" || len(r.IdempotencyKey) > 128 {
		return RPInteractionResult{}, core.NewError(core.CodeInvalidArgument, "principal, session and original interaction key required")
	}
	tx, err := beginImmediate(ctx, s.Store.db)
	if err != nil {
		return RPInteractionResult{}, err
	}
	defer tx.Rollback(ctx)
	session, err := loadRPSession(ctx, tx.conn, r.PrincipalID, r.SessionID)
	if err != nil {
		return RPInteractionResult{}, err
	}
	if err := authorizeRPControl(ctx, tx.conn, r.PrincipalID, session.InstanceID, session.BranchID, session.ControlledEntityID); err != nil {
		return RPInteractionResult{}, err
	}
	run, err := loadRPInteraction(ctx, tx.conn, r.SessionID, r.IdempotencyKey)
	if err != nil {
		return RPInteractionResult{}, classifyMissing(err, "original interaction")
	}
	if run.Status == "stopped" || run.Status == "settled" || run.Status == "clarification" {
		return run.result(true), nil
	}
	if run.PendingKind != "" {
		var child struct {
			PrincipalID    string `json:"principal_id"`
			SessionID      string `json:"session_id"`
			IdempotencyKey string `json:"idempotency_key"`
		}
		if err := json.Unmarshal([]byte(run.PendingRequest), &child); err != nil || child.PrincipalID != r.PrincipalID || child.SessionID != r.SessionID || child.IdempotencyKey == "" {
			return RPInteractionResult{}, core.NewError(core.CodeProjectionDiverged, "pinned interaction child identity differs")
		}
		var accepted int
		operation := run.PendingKind
		switch run.PendingKind {
		case "speech":
			operation = "dialogue"
			err = tx.conn.QueryRowContext(ctx, `SELECT 1 FROM rp_turn_runs WHERE session_id=? AND idempotency_key=?`, r.SessionID, child.IdempotencyKey).Scan(&accepted)
		case "wait":
			err = tx.conn.QueryRowContext(ctx, `SELECT 1 FROM rp_wait_intents WHERE session_id=? AND idempotency_key=?`, r.SessionID, child.IdempotencyKey).Scan(&accepted)
		case "move":
			err = tx.conn.QueryRowContext(ctx, `SELECT 1 FROM commands WHERE instance_id=? AND branch_id=? AND command_type='RPPlayerMove' AND idempotency_key=?`, session.InstanceID, session.BranchID, "rp_move:"+r.SessionID+":"+child.IdempotencyKey).Scan(&accepted)
		default:
			return RPInteractionResult{}, core.NewError(core.CodeProjectionDiverged, "unknown pinned interaction child kind")
		}
		if err == nil {
			return RPInteractionResult{}, core.NewError(core.CodeCommandInProgress, "accepted child must be recovered before stopping the plan")
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return RPInteractionResult{}, core.WrapError(core.CodeStorageFailure, "check pinned child acceptance before stop", err)
		}
		if _, err := tx.conn.ExecContext(ctx, `INSERT INTO rp_request_retirements(principal_id,operation,session_scope,idempotency_key,retired_at_utc) VALUES (?,?,?,?,?) ON CONFLICT DO NOTHING`, r.PrincipalID, operation, r.SessionID, child.IdempotencyKey, s.Store.now().UTC().Format(time.RFC3339Nano)); err != nil {
			return RPInteractionResult{}, core.WrapError(core.CodeStorageFailure, "retire unaccepted interaction child", err)
		}
		run.PendingKind, run.PendingRequest = "", "{}"
	}
	if _, err := tx.conn.ExecContext(ctx, `UPDATE rp_interactions SET status='stopped',pending_kind='',pending_request_json='{}',updated_at_utc=? WHERE interaction_id=? AND status IN ('open','paused')`, s.Store.now().UTC().Format(time.RFC3339Nano), run.ID); err != nil {
		return RPInteractionResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return RPInteractionResult{}, err
	}
	run.Status = "stopped"
	return run.result(false), nil
}

func (s *RPService) ResumeRPInteraction(ctx context.Context, r RPInteractionResumeRequest) (RPInteractionResult, error) {
	if r.PrincipalID == "" || r.SessionID == "" || r.IdempotencyKey == "" || len(r.IdempotencyKey) > 128 {
		return RPInteractionResult{}, core.NewError(core.CodeInvalidArgument, "principal, session and original interaction key required")
	}
	tx, err := beginImmediate(ctx, s.Store.db)
	if err != nil {
		return RPInteractionResult{}, err
	}
	defer tx.Rollback(ctx)
	session, err := loadRPSession(ctx, tx.conn, r.PrincipalID, r.SessionID)
	if err != nil {
		return RPInteractionResult{}, err
	}
	if err := authorizeRPControl(ctx, tx.conn, r.PrincipalID, session.InstanceID, session.BranchID, session.ControlledEntityID); err != nil {
		return RPInteractionResult{}, err
	}
	run, err := loadRPInteraction(ctx, tx.conn, r.SessionID, r.IdempotencyKey)
	if err != nil {
		return RPInteractionResult{}, classifyMissing(err, "original interaction")
	}
	var original core.RPInteractionRequest
	if err := json.Unmarshal([]byte(run.RequestJSON), &original); err != nil {
		return RPInteractionResult{}, core.WrapError(core.CodeProjectionDiverged, "decode original interaction", err)
	}
	if original.PrincipalID != r.PrincipalID || original.SessionID != r.SessionID || original.IdempotencyKey != r.IdempotencyKey {
		return RPInteractionResult{}, core.NewError(core.CodeProjectionDiverged, "original interaction identity differs")
	}
	tx.Rollback(ctx)
	return s.RunRPInteraction(ctx, original)
}

func (s *RPService) ensureRPInteraction(ctx context.Context, request core.RPInteractionRequest) (rpInteraction, bool, error) {
	hash, err := core.HashJSON(request)
	if err != nil {
		return rpInteraction{}, false, err
	}
	requestJSON, err := core.CanonicalJSON(request)
	if err != nil {
		return rpInteraction{}, false, err
	}
	check := func(tx *immediateTx) (RPSession, rpInteraction, bool, error) {
		session, err := loadRPSession(ctx, tx.conn, request.PrincipalID, request.SessionID)
		if err != nil {
			return RPSession{}, rpInteraction{}, false, err
		}
		if err := authorizeRPControl(ctx, tx.conn, request.PrincipalID, session.InstanceID, session.BranchID, session.ControlledEntityID); err != nil {
			return RPSession{}, rpInteraction{}, false, err
		}
		run, err := loadRPInteraction(ctx, tx.conn, request.SessionID, request.IdempotencyKey)
		if err == nil {
			if run.RequestHash != hash {
				return RPSession{}, rpInteraction{}, false, core.NewError(core.CodeIdempotencyMismatch, "interaction key was used with different input")
			}
			return session, run, true, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return RPSession{}, rpInteraction{}, false, err
		}
		var retired int
		err = tx.conn.QueryRowContext(ctx, `SELECT 1 FROM rp_interaction_retirements WHERE principal_id=? AND session_id=? AND idempotency_key=?`, request.PrincipalID, request.SessionID, request.IdempotencyKey).Scan(&retired)
		if err == nil {
			return RPSession{}, rpInteraction{}, false, core.NewError(core.CodeRequestRetired, "interaction key was retired before acceptance")
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return RPSession{}, rpInteraction{}, false, core.WrapError(core.CodeStorageFailure, "check interaction retirement", err)
		}
		var activeID string
		err = tx.conn.QueryRowContext(ctx, `SELECT interaction_id FROM rp_interactions WHERE session_id=? AND status IN ('open','paused')`, request.SessionID).Scan(&activeID)
		if err == nil {
			return RPSession{}, rpInteraction{}, false, core.NewError(core.CodeCommandInProgress, "another interaction must settle or be explicitly stopped first")
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return RPSession{}, rpInteraction{}, false, core.WrapError(core.CodeStorageFailure, "check active interaction", err)
		}
		if session.Status != "active" || session.ObservationCursor != request.ExpectedCursor {
			return RPSession{}, rpInteraction{}, false, core.NewError(core.CodeBranchConflict, "interaction requires an active freshly observed session")
		}
		var head int64
		if err := tx.conn.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, session.InstanceID, session.BranchID).Scan(&head); err != nil {
			return RPSession{}, rpInteraction{}, false, classifyMissing(err, "interaction world head")
		}
		if head != request.ExpectedCursor {
			return RPSession{}, rpInteraction{}, false, core.NewError(core.CodeBranchConflict, "interaction observation is stale")
		}
		return session, rpInteraction{}, false, nil
	}
	tx, err := beginImmediate(ctx, s.Store.db)
	if err != nil {
		return rpInteraction{}, false, err
	}
	_, existing, found, err := check(tx)
	var initialMode RPInteractionModeView
	if err == nil && !found && request.Mode == "" {
		initialMode, err = readRPInteractionMode(ctx, tx.conn, request.SessionID)
	}
	tx.Rollback(ctx)
	if err != nil || found {
		return existing, found, err
	}
	observation, err := s.ObserveRPSession(ctx, core.RPSessionReadRequest{PrincipalID: request.PrincipalID, SessionID: request.SessionID})
	if err != nil {
		return rpInteraction{}, false, err
	}
	if observation.ObservationCursor != request.ExpectedCursor {
		return rpInteraction{}, false, core.NewError(core.CodeBranchConflict, "world changed while resolving interaction")
	}
	places := make([]core.RPInteractionPlace, 0, len(observation.ReachablePlaces))
	for _, place := range observation.ReachablePlaces {
		if place.CanMoveNow {
			places = append(places, core.RPInteractionPlace{ID: place.PlaceID, Name: place.DisplayName})
		}
	}
	mode := request.Mode
	if mode == "" {
		mode = initialMode.Mode
	}
	plan, err := core.ParseRPInteraction(request.Text, mode, places)
	if err != nil {
		return rpInteraction{}, false, err
	}
	planJSON, err := core.CanonicalJSON(plan)
	if err != nil {
		return rpInteraction{}, false, err
	}
	keyHash, err := core.HashJSON([]string{request.SessionID, request.IdempotencyKey})
	if err != nil {
		return rpInteraction{}, false, err
	}
	id := "rpinteraction_" + keyHash[7:]
	status := "open"
	if plan.Kind == "CLARIFICATION" {
		status = "clarification"
	}
	tx, err = beginImmediate(ctx, s.Store.db)
	if err != nil {
		return rpInteraction{}, false, err
	}
	defer tx.Rollback(ctx)
	_, existing, found, err = check(tx)
	if err != nil || found {
		return existing, found, err
	}
	if request.Mode == "" {
		currentMode, err := readRPInteractionMode(ctx, tx.conn, request.SessionID)
		if err != nil {
			return rpInteraction{}, false, err
		}
		if currentMode.Revision != initialMode.Revision || currentMode.Mode != initialMode.Mode {
			return rpInteraction{}, false, core.NewError(core.CodeBranchConflict, "interaction session mode changed before plan acceptance")
		}
	}
	now := s.Store.now().UTC().Format(time.RFC3339Nano)
	_, err = tx.conn.ExecContext(ctx, `INSERT INTO rp_interactions(interaction_id,session_id,idempotency_key,request_hash,request_json,plan_json,status,created_at_utc,updated_at_utc) VALUES (?,?,?,?,?,?,?,?,?)`, id, request.SessionID, request.IdempotencyKey, hash, string(requestJSON), string(planJSON), status, now, now)
	if err != nil {
		return rpInteraction{}, false, core.WrapError(core.CodeStorageFailure, "persist interaction plan", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return rpInteraction{}, false, err
	}
	return rpInteraction{ID: id, SessionID: request.SessionID, Key: request.IdempotencyKey, RequestHash: hash, RequestJSON: string(requestJSON), Plan: plan, Status: status, PendingRequest: "{}", Outcomes: []RPInteractionOutcome{}}, false, nil
}

func (s *RPService) prepareRPInteractionStep(ctx context.Context, request core.RPInteractionRequest, run rpInteraction) (rpInteraction, error) {
	observation, err := s.ObserveRPSession(ctx, core.RPSessionReadRequest{PrincipalID: request.PrincipalID, SessionID: request.SessionID})
	if err != nil {
		return run, err
	}
	if run.NextStep == 0 && observation.ObservationCursor != request.ExpectedCursor {
		return run, core.NewError(core.CodeBranchConflict, "world changed before first interaction step")
	}
	if run.NextStep > 0 && observation.ObservationCursor != run.Outcomes[run.NextStep-1].SettledSequence {
		return run, core.NewError(core.CodeBranchConflict, "world changed after the prior interaction step; continuation needs a fresh choice")
	}
	step := run.Plan.Steps[run.NextStep]
	key := fmt.Sprintf("rpint_%s_%d", run.ID[len("rpinteraction_"):], run.NextStep)
	var child any
	switch step.Kind {
	case "move":
		found := false
		for _, place := range observation.ReachablePlaces {
			if place.PlaceID == step.TargetPlaceID && place.CanMoveNow {
				found = true
			}
		}
		if !found {
			return run, core.NewError(core.CodeBranchConflict, "planned destination is no longer reachable")
		}
		child = core.RPMoveRequest{PrincipalID: request.PrincipalID, SessionID: request.SessionID, FromPlaceID: observation.PlaceID, ToPlaceID: step.TargetPlaceID, ExpectedCursor: observation.ObservationCursor, IdempotencyKey: key}
	case "speech":
		childRequest := core.RPSpeechRequest{PrincipalID: request.PrincipalID, SessionID: request.SessionID, Text: step.SpeechText, ExpectedCursor: observation.ObservationCursor, IdempotencyKey: key}
		if request.NarrativeDensity != "" {
			childRequest.NarrativeStyle = &core.RPStylePatch{NarrativeDensity: &request.NarrativeDensity}
		}
		child = childRequest
	case "wait":
		start, err := time.Parse(time.RFC3339, observation.WorldTime)
		if err != nil {
			return run, core.WrapError(core.CodeProjectionDiverged, "invalid observed world time", err)
		}
		child = core.RPWaitRequest{PrincipalID: request.PrincipalID, SessionID: request.SessionID, TargetWorldTime: start.Add(time.Duration(step.WaitHours) * time.Hour).UTC().Format(time.RFC3339), Budget: 100, ExpectedCursor: observation.ObservationCursor, IdempotencyKey: key}
	default:
		return run, core.NewError(core.CodeProjectionDiverged, "unsupported persisted interaction step")
	}
	encoded, err := core.CanonicalJSON(child)
	if err != nil {
		return run, err
	}
	tx, err := beginImmediate(ctx, s.Store.db)
	if err != nil {
		return run, err
	}
	defer tx.Rollback(ctx)
	current, err := loadRPInteraction(ctx, tx.conn, run.SessionID, run.Key)
	if err != nil {
		return run, err
	}
	if current.NextStep != run.NextStep || current.Status != "open" {
		return run, core.NewError(core.CodeCommandInProgress, "interaction progress changed during step preparation")
	}
	if current.PendingKind != "" {
		return current, nil
	}
	if _, err := tx.conn.ExecContext(ctx, `UPDATE rp_interactions SET pending_kind=?,pending_request_json=?,updated_at_utc=? WHERE interaction_id=? AND next_step=? AND pending_kind=''`, step.Kind, string(encoded), s.Store.now().UTC().Format(time.RFC3339Nano), run.ID, run.NextStep); err != nil {
		return run, err
	}
	if err := tx.Commit(ctx); err != nil {
		return run, err
	}
	current.PendingKind, current.PendingRequest = step.Kind, string(encoded)
	return current, nil
}

func (s *RPService) executeRPInteractionStep(ctx context.Context, run rpInteraction) (RPInteractionOutcome, bool, error) {
	var out RPInteractionOutcome
	out.Kind = run.PendingKind
	switch run.PendingKind {
	case "move":
		var request core.RPMoveRequest
		if err := json.Unmarshal([]byte(run.PendingRequest), &request); err != nil {
			return out, false, core.WrapError(core.CodeProjectionDiverged, "decode pinned move", err)
		}
		result, err := s.Store.MoveRP(ctx, request)
		out.EventID, out.EventSequence, out.SettledSequence, out.WorldTime = result.EventID, result.EventSequence, result.EventSequence, result.WorldTime
		return out, false, err
	case "speech":
		var request core.RPSpeechRequest
		if err := json.Unmarshal([]byte(run.PendingRequest), &request); err != nil {
			return out, false, core.WrapError(core.CodeProjectionDiverged, "decode pinned speech", err)
		}
		result, err := s.PlayRPTurn(ctx, request)
		if err != nil {
			return out, false, err
		}
		out.EventID, out.SettledSequence, out.TurnRunID = result.PlayerEventID, result.SettledSequence, result.TurnRunID
		if err := s.Store.db.QueryRowContext(ctx, `SELECT event_sequence FROM events WHERE event_id=?`, result.PlayerEventID).Scan(&out.EventSequence); err != nil {
			return out, false, classifyMissing(err, "accepted interaction speech event")
		}
		return out, false, nil
	case "wait":
		var request core.RPWaitRequest
		if err := json.Unmarshal([]byte(run.PendingRequest), &request); err != nil {
			return out, false, core.WrapError(core.CodeProjectionDiverged, "decode pinned wait", err)
		}
		result, err := s.WaitRP(ctx, request)
		out.EventID, out.EventSequence, out.WorldTime = result.EventID, result.EventSequence, result.CurrentWorldTime
		if err != nil || result.Status == "budget_exhausted" {
			return out, result.Status == "budget_exhausted", err
		}
		out.SettledSequence, err = s.rpInteractionWaitSettledSequence(ctx, run.SessionID, result.EventID, result.EventSequence)
		return out, false, err
	default:
		return out, false, core.NewError(core.CodeProjectionDiverged, "invalid pending interaction step")
	}
}

// A completed wait can trigger warm/initiative Events after RPWaitCompleted.
// They belong to that exact wait, so they must not falsely pause its next
// planned step. The first unrelated Event remains a hard continuation fence.
func (s *RPService) rpInteractionWaitSettledSequence(ctx context.Context, sessionID, waitEventID string, waitSequence int64) (int64, error) {
	if waitEventID == "" || waitSequence < 1 {
		return 0, core.NewError(core.CodeProjectionDiverged, "interaction wait has no committed event")
	}
	rows, err := s.Store.db.QueryContext(ctx, `SELECT e.event_sequence,c.command_type,COALESCE(json_extract(e.payload,'$.trigger_event_id'),'')
 FROM rp_sessions s JOIN events e ON e.instance_id=s.instance_id AND e.branch_id=s.branch_id
 JOIN event_batches b ON b.batch_id=e.batch_id JOIN commands c ON c.command_id=b.command_id
 WHERE s.session_id=? AND e.event_sequence>? ORDER BY e.event_sequence`, sessionID, waitSequence)
	if err != nil {
		return 0, core.WrapError(core.CodeStorageFailure, "read wait-owned continuation events", err)
	}
	defer rows.Close()
	settled := waitSequence
	for rows.Next() {
		var sequence int64
		var commandType, trigger string
		if err := rows.Scan(&sequence, &commandType, &trigger); err != nil {
			return 0, core.WrapError(core.CodeStorageFailure, "scan wait-owned continuation events", err)
		}
		if sequence != settled+1 {
			return 0, core.NewError(core.CodeProjectionDiverged, "interaction wait continuation has a sequence gap")
		}
		if (commandType != "RPWarmDecision" && commandType != "RPNPCInitiative") || trigger != waitEventID {
			break
		}
		settled = sequence
	}
	if err := rows.Err(); err != nil {
		return 0, core.WrapError(core.CodeStorageFailure, "read wait-owned continuation events", err)
	}
	return settled, nil
}

func (s *RPService) commitRPInteractionStep(ctx context.Context, run rpInteraction, outcome RPInteractionOutcome) (rpInteraction, error) {
	tx, err := beginImmediate(ctx, s.Store.db)
	if err != nil {
		return run, err
	}
	defer tx.Rollback(ctx)
	current, err := loadRPInteraction(ctx, tx.conn, run.SessionID, run.Key)
	if err != nil {
		return run, err
	}
	if current.NextStep != run.NextStep || current.PendingKind != outcome.Kind || current.PendingRequest != run.PendingRequest {
		return run, core.NewError(core.CodeCommandInProgress, "interaction step changed during result recording")
	}
	if outcome.EventID == "" || outcome.EventSequence < 1 || outcome.SettledSequence < outcome.EventSequence {
		return run, core.NewError(core.CodeProjectionDiverged, "interaction child reported no committed Event")
	}
	current.Outcomes = append(current.Outcomes, outcome)
	current.NextStep++
	current.PendingKind, current.PendingRequest = "", "{}"
	if current.NextStep == len(current.Plan.Steps) {
		current.Status = "settled"
	}
	encoded, err := core.CanonicalJSON(current.Outcomes)
	if err != nil {
		return run, err
	}
	_, err = tx.conn.ExecContext(ctx, `UPDATE rp_interactions SET next_step=?,pending_kind='',pending_request_json='{}',outcomes_json=?,status=?,updated_at_utc=? WHERE interaction_id=? AND next_step=?`, current.NextStep, string(encoded), current.Status, s.Store.now().UTC().Format(time.RFC3339Nano), run.ID, run.NextStep)
	if err != nil {
		return run, err
	}
	if err := tx.Commit(ctx); err != nil {
		return run, err
	}
	return current, nil
}
