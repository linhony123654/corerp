package storage

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"corerp.local/backend/internal/core"
)

type RPRequestRetireRequest struct {
	PrincipalID    string `json:"principal_id"`
	Operation      string `json:"operation"`
	SessionID      string `json:"session_id,omitempty"`
	IdempotencyKey string `json:"idempotency_key"`
}

type RPRequestOutcome struct {
	ProtocolVersion string `json:"protocol_version"`
	Operation       string `json:"operation"`
	SessionID       string `json:"session_id,omitempty"`
	IdempotencyKey  string `json:"idempotency_key"`
	Status          string `json:"status"` // retired, in_progress, completed
}

// RetireRPRequest either reports existing acceptance or permanently fences an
// unaccepted key. Absence alone is never returned as permission to discard.
// It does not cancel accepted work, advance time, or roll back world effects.
func (s *Store) RetireRPRequest(ctx context.Context, r RPRequestRetireRequest) (RPRequestOutcome, error) {
	var out RPRequestOutcome
	if strings.TrimSpace(r.PrincipalID) == "" || strings.TrimSpace(r.IdempotencyKey) == "" || len(r.IdempotencyKey) > 128 || len(r.SessionID) > 256 {
		return out, core.NewError(core.CodeInvalidArgument, "principal and bounded request key required")
	}
	switch r.Operation {
	case "open":
		if r.SessionID != "" {
			return out, core.NewError(core.CodeInvalidArgument, "open keys are principal scoped")
		}
	case "dialogue", "wait", "move", "social", "object", "nonverbal", "interaction":
		if strings.TrimSpace(r.SessionID) == "" {
			return out, core.NewError(core.CodeInvalidArgument, "session required")
		}
	default:
		return out, core.NewError(core.CodeInvalidArgument, "unsupported request operation")
	}
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	var session RPSession
	if r.Operation != "open" {
		// A former controller may inspect only the exact accepted result of
		// their own session/key. Fresh retirement still needs current control.
		session, err = loadRPSessionRecord(ctx, tx.conn, r.PrincipalID, r.SessionID)
		if err != nil {
			return out, err
		}
		// A revoked current grant is not a controller handoff. Continue to
		// enforce its denial even when the session has a committed receipt.
		currentErr := requireCurrentRPSession(ctx, tx.conn, session)
		if currentErr == nil {
			if err := authorizeRPControl(ctx, tx.conn, r.PrincipalID, session.InstanceID, session.BranchID, session.ControlledEntityID); err != nil {
				return out, err
			}
		} else if !core.HasCode(currentErr, core.CodeBranchConflict) {
			return out, currentErr
		}
	} else {
		// No proposed world binding is required: an invalid open must also be
		// retireable. This authority only disables the caller's own key.
		var principal string
		if err := tx.conn.QueryRowContext(ctx, `SELECT principal_id FROM principals WHERE principal_id=? AND status='active'`, r.PrincipalID).Scan(&principal); err != nil {
			return out, classifyMissing(err, "RP request principal")
		}
	}
	out = RPRequestOutcome{ProtocolVersion: RPClientProtocolVersion, Operation: r.Operation, SessionID: r.SessionID, IdempotencyKey: r.IdempotencyKey}
	var status string
	switch r.Operation {
	case "open":
		err = tx.conn.QueryRowContext(ctx, `SELECT session_id FROM rp_sessions WHERE principal_id=? AND idempotency_key=?`, r.PrincipalID, r.IdempotencyKey).Scan(&out.SessionID)
		status = "completed"
	case "dialogue":
		err = tx.conn.QueryRowContext(ctx, `SELECT CASE WHEN status='settled' THEN 'completed' ELSE 'in_progress' END FROM rp_turn_runs WHERE session_id=? AND idempotency_key=?`, r.SessionID, r.IdempotencyKey).Scan(&status)
	case "wait":
		err = tx.conn.QueryRowContext(ctx, `SELECT CASE WHEN status='completed' THEN 'completed' ELSE 'in_progress' END FROM rp_wait_intents WHERE session_id=? AND idempotency_key=?`, r.SessionID, r.IdempotencyKey).Scan(&status)
	case "interaction":
		err = tx.conn.QueryRowContext(ctx, `SELECT CASE WHEN status IN ('settled','stopped','clarification') THEN 'completed' ELSE 'in_progress' END FROM rp_interactions WHERE session_id=? AND idempotency_key=?`, r.SessionID, r.IdempotencyKey).Scan(&status)
	case "move", "social", "object", "nonverbal":
		commandTypes := map[string]string{"move": "RPPlayerMove", "social": "RPSocial", "object": "RPObjectInteraction", "nonverbal": "RPNonverbalAction"}
		err = tx.conn.QueryRowContext(ctx, `SELECT CASE WHEN status='committed' THEN 'completed' ELSE 'in_progress' END FROM commands WHERE instance_id=? AND branch_id=? AND command_type=? AND idempotency_key=?`, session.InstanceID, session.BranchID, commandTypes[r.Operation], "rp_"+r.Operation+":"+r.SessionID+":"+r.IdempotencyKey).Scan(&status)
	}
	if err == nil {
		out.Status = status
		return out, nil // Existing owner is untouched, including partial work.
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return RPRequestOutcome{}, core.WrapError(core.CodeStorageFailure, "read RP request acceptance", err)
	}
	if r.Operation == "wait" {
		// The Human-owned child key belongs to its round even before an intent
		// exists. An open round must not become impossible to settle.
		err = tx.conn.QueryRowContext(ctx, `SELECT CASE WHEN r.status='settled' THEN 'completed' ELSE 'in_progress' END FROM rp_shared_rounds r JOIN rp_shared_round_participants p ON p.round_id=r.round_id AND p.session_id=r.human_session_id WHERE r.human_session_id=? AND p.principal_id=? AND (r.status IN ('open','advancing') OR (r.status='settled' AND r.settlement_kind='wait')) AND ?='shared_'||r.round_id`, r.SessionID, r.PrincipalID, r.IdempotencyKey).Scan(&status)
		if err == nil {
			out.Status = status
			return out, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return RPRequestOutcome{}, core.WrapError(core.CodeStorageFailure, "read shared wait reservation", err)
		}
	}
	if r.Operation == "dialogue" || r.Operation == "move" {
		// A round reserves both potential typed action keys on opening. An
		// unsubmitted action can still be selected after this retirement call.
		err = tx.conn.QueryRowContext(ctx, `SELECT 'in_progress' FROM rp_shared_rounds r JOIN rp_shared_round_participants p ON p.round_id=r.round_id WHERE p.session_id=? AND p.principal_id=? AND r.status IN ('open','advancing') AND ?='shared_action_'||r.round_id`, r.SessionID, r.PrincipalID, r.IdempotencyKey).Scan(&status)
		if err == nil {
			out.Status = status
			return out, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return RPRequestOutcome{}, core.WrapError(core.CodeStorageFailure, "read shared action reservation", err)
		}
		kind := "speech"
		if r.Operation == "move" {
			kind = "move"
		}
		err = tx.conn.QueryRowContext(ctx, `SELECT CASE WHEN r.status='settled' THEN 'completed' ELSE 'in_progress' END FROM rp_shared_round_actions a JOIN rp_shared_rounds r ON r.round_id=a.round_id JOIN rp_shared_round_participants p ON p.round_id=a.round_id AND p.session_id=a.session_id WHERE a.session_id=? AND p.principal_id=? AND a.action_kind=? AND ?='shared_action_'||r.round_id`, r.SessionID, r.PrincipalID, kind, r.IdempotencyKey).Scan(&status)
		if err == nil {
			out.Status = status
			return out, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return RPRequestOutcome{}, core.WrapError(core.CodeStorageFailure, "read shared action acceptance", err)
		}
	}
	if r.Operation != "open" {
		if err := requireCurrentRPSession(ctx, tx.conn, session); err != nil {
			return RPRequestOutcome{}, err
		}
		if err := authorizeRPControl(ctx, tx.conn, r.PrincipalID, session.InstanceID, session.BranchID, session.ControlledEntityID); err != nil {
			return RPRequestOutcome{}, err
		}
	}
	if r.Operation == "interaction" {
		_, err = tx.conn.ExecContext(ctx, `INSERT INTO rp_interaction_retirements(principal_id,session_id,idempotency_key,retired_at_utc) VALUES (?,?,?,?) ON CONFLICT DO NOTHING`, r.PrincipalID, r.SessionID, r.IdempotencyKey, s.now().UTC().Format(time.RFC3339Nano))
	} else if r.Operation == "object" || r.Operation == "nonverbal" {
		_, err = tx.conn.ExecContext(ctx, `INSERT INTO rp_typed_action_retirements(principal_id,operation,session_id,idempotency_key,retired_at_utc) VALUES (?,?,?,?,?) ON CONFLICT DO NOTHING`, r.PrincipalID, r.Operation, r.SessionID, r.IdempotencyKey, s.now().UTC().Format(time.RFC3339Nano))
	} else {
		_, err = tx.conn.ExecContext(ctx, `INSERT INTO rp_request_retirements(principal_id,operation,session_scope,idempotency_key,retired_at_utc) VALUES (?,?,?,?,?) ON CONFLICT DO NOTHING`, r.PrincipalID, r.Operation, r.SessionID, r.IdempotencyKey, s.now().UTC().Format(time.RFC3339Nano))
	}
	if err != nil {
		return RPRequestOutcome{}, core.WrapError(core.CodeStorageFailure, "retire RP request", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return RPRequestOutcome{}, core.WrapError(core.CodeStorageFailure, "commit RP request retirement", err)
	}
	out.Status = "retired"
	return out, nil
}

// Called under each original owner's immediate acceptance transaction. This
// must never be moved to a separate preflight read (TOCTOU with retirement).
func checkRPRequestRetirement(ctx context.Context, conn *sql.Conn, principal, operation, session, key string) error {
	var found int
	err := conn.QueryRowContext(ctx, `SELECT 1 FROM rp_request_retirements WHERE principal_id=? AND operation=? AND session_scope=? AND idempotency_key=?`, principal, operation, session, key).Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return core.WrapError(core.CodeStorageFailure, "check RP request retirement", err)
	}
	return core.NewError(core.CodeRequestRetired, "request key permanently retired before acceptance")
}

// Typed owner keys live outside 027's frozen operation CHECK. Call only inside
// the owner's BEGIN IMMEDIATE acceptance transaction, before writing a command.
func checkRPTypedActionRetirement(ctx context.Context, conn *sql.Conn, principal, operation, session, key string) error {
	if operation != "object" && operation != "nonverbal" {
		return core.NewError(core.CodeInvalidArgument, "unknown typed RP action operation")
	}
	var found int
	err := conn.QueryRowContext(ctx, `SELECT 1 FROM rp_typed_action_retirements WHERE principal_id=? AND operation=? AND session_id=? AND idempotency_key=?`, principal, operation, session, key).Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return core.WrapError(core.CodeStorageFailure, "check typed RP action retirement", err)
	}
	return core.NewError(core.CodeRequestRetired, "typed RP action key retired before acceptance")
}
