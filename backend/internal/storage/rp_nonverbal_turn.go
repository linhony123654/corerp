package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"corerp.local/backend/internal/core"
)

// RunRPNonverbalTurn binds a real player action to the same durable reaction
// workflow as dialogue. This first slice supports a directed nod only. It
// neither synthesizes player speech nor interprets the nod as agreement.
func (s *Store) RunRPNonverbalTurn(ctx context.Context, request core.RPNonverbalRequest, provider core.RPDecisionProvider) (RPTurnResult, error) {
	if err := request.Validate(); err != nil {
		return RPTurnResult{}, err
	}
	if request.Action != "nod" || request.TargetEntityID == "" {
		return RPTurnResult{}, core.NewError(core.CodeInvalidArgument, "reaction turns currently require a targeted nod")
	}
	run, existing, err := s.ensureRPNonverbalTurnRun(ctx, request, provider)
	if err != nil {
		return RPTurnResult{}, err
	}
	if run.Status == "settled" {
		return s.loadRPTurnResult(ctx, run, true)
	}
	if provider == nil {
		return RPTurnResult{}, core.NewError(core.CodeInvalidArgument, "unfinished RP action turn requires a decision provider")
	}
	if err := s.afterTurnStage("turn_open"); err != nil {
		return RPTurnResult{}, err
	}
	// The existing owner remains the sole writer of the player action. Only
	// its exact pinned turn may coexist with its pending-turn prohibition.
	action, err := s.nonverbalRP(ctx, request, run.ID)
	if err != nil {
		return RPTurnResult{}, err
	}
	if err := s.afterTurnStage("player_event_committed"); err != nil {
		return RPTurnResult{}, err
	}
	if err := s.markRPNonverbalTurnPlayerCommitted(ctx, request, run.ID, action); err != nil {
		return RPTurnResult{}, err
	}
	if err := s.afterTurnStage("player_committed"); err != nil {
		return RPTurnResult{}, err
	}
	if err := s.advanceRPTurnRunStatus(ctx, run.ID, "player_committed", "npc_deciding"); err != nil {
		return RPTurnResult{}, err
	}
	activations, err := s.ensureRPNonverbalActivationPlan(ctx, run.ID, run.SessionID)
	if err != nil {
		return RPTurnResult{}, err
	}
	return s.finishRPTurn(ctx, run, request.PrincipalID, action.EventID, action.EventID, provider, activations, existing)
}

func (s *Store) ensureRPNonverbalTurnRun(ctx context.Context, request core.RPNonverbalRequest, provider core.RPDecisionProvider) (rpTurnRun, bool, error) {
	hash, err := core.HashJSON(request)
	if err != nil {
		return rpTurnRun{}, false, err
	}
	encoded, err := core.CanonicalJSON(request)
	if err != nil {
		return rpTurnRun{}, false, err
	}
	session, err := loadRPSessionRecord(ctx, s.db, request.PrincipalID, request.SessionID)
	if err != nil {
		return rpTurnRun{}, false, err
	}
	// Sweep only before a new intent. An already committed action's retry may
	// not create activity events and then adopt them as reaction continuation.
	err = s.settleRPActivitiesWhen(ctx, session.InstanceID, session.BranchID, func(conn *sql.Conn) (bool, error) {
		var count int
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_turn_runs WHERE session_id=? AND idempotency_key=?`, session.SessionID, request.IdempotencyKey).Scan(&count); err != nil {
			return false, err
		}
		if count != 0 {
			return false, nil
		}
		if provider == nil {
			return false, core.NewError(core.CodeInvalidArgument, "new RP action turn requires a decision provider")
		}
		fresh, err := loadRPSession(ctx, conn, request.PrincipalID, request.SessionID)
		if err != nil {
			return false, err
		}
		if err := authorizeRPControl(ctx, conn, request.PrincipalID, fresh.InstanceID, fresh.BranchID, fresh.ControlledEntityID); err != nil {
			return false, err
		}
		if err := checkRPTypedActionRetirement(ctx, conn, request.PrincipalID, "nonverbal", fresh.SessionID, request.IdempotencyKey); err != nil {
			return false, err
		}
		if err := validateRPBinding(ctx, conn, fresh.InstanceID, fresh.BranchID, fresh.ControlledEntityID); err != nil {
			return false, err
		}
		if _, err := validateRPNonverbalTarget(ctx, conn, fresh, request.TargetEntityID); err != nil {
			return false, err
		}
		if err := validateRPNonverbalTurnStart(ctx, conn, fresh, request.ExpectedCursor); err != nil {
			return false, err
		}
		return true, nil
	})
	if err != nil {
		return rpTurnRun{}, false, err
	}
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return rpTurnRun{}, false, err
	}
	defer tx.Rollback(ctx)
	session, err = loadRPSessionRecord(ctx, tx.conn, request.PrincipalID, request.SessionID)
	if err != nil {
		return rpTurnRun{}, false, err
	}
	var id, priorHash, kind, status string
	err = tx.conn.QueryRowContext(ctx, `SELECT turn_run_id,request_hash,trigger_kind,status FROM rp_turn_runs WHERE session_id=? AND idempotency_key=?`, session.SessionID, request.IdempotencyKey).Scan(&id, &priorHash, &kind, &status)
	if err == nil {
		if priorHash != hash || kind != "nonverbal" {
			return rpTurnRun{}, false, core.NewError(core.CodeIdempotencyMismatch, "RP action turn key payload differs")
		}
		if status != "settled" {
			if err := requireCurrentRPSession(ctx, tx.conn, session); err != nil {
				return rpTurnRun{}, false, err
			}
			if err := authorizeRPControl(ctx, tx.conn, request.PrincipalID, session.InstanceID, session.BranchID, session.ControlledEntityID); err != nil {
				return rpTurnRun{}, false, err
			}
		}
		tx.Rollback(ctx)
		run, err := s.loadRPTurnRun(ctx, id)
		return run, true, err
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return rpTurnRun{}, false, core.WrapError(core.CodeStorageFailure, "read RP action turn intent", err)
	}
	if err := requireCurrentRPSession(ctx, tx.conn, session); err != nil {
		return rpTurnRun{}, false, err
	}
	if err := authorizeRPControl(ctx, tx.conn, request.PrincipalID, session.InstanceID, session.BranchID, session.ControlledEntityID); err != nil {
		return rpTurnRun{}, false, err
	}
	if err := checkRPTypedActionRetirement(ctx, tx.conn, request.PrincipalID, "nonverbal", session.SessionID, request.IdempotencyKey); err != nil {
		return rpTurnRun{}, false, err
	}
	if err := validateRPBinding(ctx, tx.conn, session.InstanceID, session.BranchID, session.ControlledEntityID); err != nil {
		return rpTurnRun{}, false, err
	}
	if _, err := validateRPNonverbalTarget(ctx, tx.conn, session, request.TargetEntityID); err != nil {
		return rpTurnRun{}, false, err
	}
	if err := validateRPNonverbalTurnStart(ctx, tx.conn, session, request.ExpectedCursor); err != nil {
		return rpTurnRun{}, false, err
	}
	keyHash, err := core.HashJSON(struct{ SessionID, Key, Kind string }{session.SessionID, request.IdempotencyKey, "nonverbal"})
	if err != nil {
		return rpTurnRun{}, false, err
	}
	run := rpTurnRun{ID: "rpturn_" + keyHash[7:], SessionID: session.SessionID, SpeechKey: request.IdempotencyKey, Status: "open", ListenerIDsJSON: "[]", NarrativeJSON: "[]"}
	now := s.now().UTC().Format(time.RFC3339Nano)
	if err := execAgentOne(ctx, tx.conn, "pin RP nonverbal turn", `INSERT INTO rp_turn_runs(turn_run_id,session_id,idempotency_key,player_speech_key,request_hash,request_json,trigger_kind,status,created_at_utc,updated_at_utc) VALUES (?,?,?,?,?,?,'nonverbal','open',?,?)`, run.ID, run.SessionID, request.IdempotencyKey, run.SpeechKey, hash, string(encoded), now, now); err != nil {
		return rpTurnRun{}, false, err
	}
	if err := pinRPTurnStyle(ctx, tx.conn, run.ID, session, nil); err != nil {
		return rpTurnRun{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return rpTurnRun{}, false, err
	}
	return run, false, nil
}

func validateRPNonverbalTurnStart(ctx context.Context, conn *sql.Conn, session RPSession, expected int64) error {
	if session.Status != "active" {
		return core.NewError(core.CodeBranchConflict, "RP session is closed")
	}
	if err := requireNoActiveRPSharedRound(ctx, conn, session.InstanceID, session.BranchID); err != nil {
		return err
	}
	var pending, head int64
	if err := conn.QueryRowContext(ctx, `SELECT
	 (SELECT COUNT(*) FROM rp_turn_runs t JOIN rp_sessions s ON s.session_id=t.session_id WHERE s.instance_id=? AND s.branch_id=? AND t.status<>'settled')+
	 (SELECT COUNT(*) FROM rp_wait_intents w JOIN rp_sessions s ON s.session_id=w.session_id WHERE s.instance_id=? AND s.branch_id=? AND w.status='pending')`, session.InstanceID, session.BranchID, session.InstanceID, session.BranchID).Scan(&pending); err != nil {
		return err
	}
	if pending != 0 {
		return core.NewError(core.CodeCommandInProgress, "another RP action or turn is pending")
	}
	if err := conn.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, session.InstanceID, session.BranchID).Scan(&head); err != nil {
		return err
	}
	if head != expected || session.ObservationCursor != head {
		return core.NewError(core.CodeBranchConflict, "RP action turn requires a current observation cursor")
	}
	return nil
}

func (s *Store) markRPNonverbalTurnPlayerCommitted(ctx context.Context, request core.RPNonverbalRequest, runID string, action RPNonverbalResult) error {
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	session, err := loadRPSession(ctx, tx.conn, request.PrincipalID, request.SessionID)
	if err != nil {
		return err
	}
	if err := authorizeRPControl(ctx, tx.conn, request.PrincipalID, session.InstanceID, session.BranchID, session.ControlledEntityID); err != nil {
		return err
	}
	var status, eventID string
	if err := tx.conn.QueryRowContext(ctx, `SELECT status,COALESCE(player_event_id,'') FROM rp_turn_runs WHERE turn_run_id=? AND session_id=? AND trigger_kind='nonverbal'`, runID, session.SessionID).Scan(&status, &eventID); err != nil {
		return classifyMissing(err, "pinned RP action turn")
	}
	if status != "open" {
		if eventID != action.EventID {
			return core.NewError(core.CodeProjectionDiverged, "RP action turn source changed")
		}
		return nil
	}
	var raw, actor string
	var sequence int64
	if err := tx.conn.QueryRowContext(ctx, `SELECT e.payload,e.actor_id,e.event_sequence FROM events e JOIN event_batches b ON b.batch_id=e.batch_id JOIN commands c ON c.command_id=b.command_id WHERE e.event_id=? AND e.instance_id=? AND e.branch_id=? AND e.event_type='RPNonverbalAction' AND c.command_type='RPNonverbalAction' AND c.status='committed' AND c.idempotency_key=?`, action.EventID, session.InstanceID, session.BranchID, "rp_nonverbal:"+session.SessionID+":"+request.IdempotencyKey).Scan(&raw, &actor, &sequence); err != nil {
		return classifyMissing(err, "committed RP action turn source")
	}
	var fact core.RPNonverbalFact
	if json.Unmarshal([]byte(raw), &fact) != nil || actor != session.ControlledEntityID || fact.ActorEntityID != actor || fact.SessionID != session.SessionID || fact.Action != "nod" || fact.TargetEntityID == "" || sequence != action.EventSequence {
		return core.NewError(core.CodeProjectionDiverged, "RP action turn source differs from its pinned owner")
	}
	eligible := make([]string, 0, 1)
	for _, witness := range fact.Witnesses {
		if witness.ObserverEntityID == fact.TargetEntityID && witness.TargetVisible {
			eligible = append(eligible, witness.ObserverEntityID)
		}
	}
	if len(eligible) > 1 {
		return core.NewError(core.CodeProjectionDiverged, "RP action target has duplicate witness evidence")
	}
	listenersJSON, err := core.CanonicalJSON(eligible)
	if err != nil {
		return err
	}
	if err := execAgentOne(ctx, tx.conn, "bind committed RP action trigger", `UPDATE rp_turn_runs SET status='player_committed',player_event_id=?,listener_ids_json=?,updated_at_utc=? WHERE turn_run_id=? AND status='open' AND player_turn_id IS NULL`, action.EventID, string(listenersJSON), s.now().UTC().Format(time.RFC3339Nano), runID); err != nil {
		return err
	}
	if err := execAgentOne(ctx, tx.conn, "open RP action reaction stage", `UPDATE rp_sessions SET turn_cursor=?,turn_state='action_committed' WHERE session_id=? AND status='active'`, action.EventID, session.SessionID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
