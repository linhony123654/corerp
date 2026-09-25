package storage

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"corerp.local/backend/internal/core"
)

type RPInteractionModeSetRequest struct {
	PrincipalID      string `json:"principal_id"`
	SessionID        string `json:"session_id"`
	Mode             string `json:"interaction_mode"`
	ExpectedRevision int64  `json:"expected_revision"`
	IdempotencyKey   string `json:"idempotency_key"`
}

type RPInteractionModeView struct {
	Mode     string `json:"interaction_mode"`
	Revision int64  `json:"revision"`
	Replayed bool   `json:"replayed"`
}

func readRPInteractionMode(ctx context.Context, q rpInteractionQuery, sessionID string) (RPInteractionModeView, error) {
	var view RPInteractionModeView
	err := q.QueryRowContext(ctx, `SELECT mode,revision FROM rp_interaction_mode_revisions WHERE session_id=? ORDER BY revision DESC LIMIT 1`, sessionID).Scan(&view.Mode, &view.Revision)
	if errors.Is(err, sql.ErrNoRows) {
		return RPInteractionModeView{Mode: "AUTO"}, nil
	}
	if err != nil {
		return RPInteractionModeView{}, core.WrapError(core.CodeStorageFailure, "read interaction session mode", err)
	}
	return view, nil
}

func (s *Store) ReadRPInteractionMode(ctx context.Context, r core.RPSessionReadRequest) (RPInteractionModeView, error) {
	if err := r.Validate(); err != nil {
		return RPInteractionModeView{}, err
	}
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return RPInteractionModeView{}, err
	}
	defer tx.Rollback(ctx)
	session, err := loadRPSession(ctx, tx.conn, r.PrincipalID, r.SessionID)
	if err != nil {
		return RPInteractionModeView{}, err
	}
	if err := authorizeRPControl(ctx, tx.conn, r.PrincipalID, session.InstanceID, session.BranchID, session.ControlledEntityID); err != nil {
		return RPInteractionModeView{}, err
	}
	return readRPInteractionMode(ctx, tx.conn, session.SessionID)
}

func (s *Store) SetRPInteractionMode(ctx context.Context, r RPInteractionModeSetRequest) (RPInteractionModeView, error) {
	if strings.TrimSpace(r.PrincipalID) == "" || strings.TrimSpace(r.SessionID) == "" || strings.TrimSpace(r.IdempotencyKey) == "" || len(r.IdempotencyKey) > 128 || r.ExpectedRevision < 0 || r.ExpectedRevision >= core.MaxJSONSafeInteger || (r.Mode != "AUTO" && r.Mode != "DIALOGUE" && r.Mode != "SCENE") {
		return RPInteractionModeView{}, core.NewError(core.CodeInvalidArgument, "invalid interaction mode revision or scope")
	}
	hash, err := core.HashJSON(r)
	if err != nil {
		return RPInteractionModeView{}, err
	}
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return RPInteractionModeView{}, err
	}
	defer tx.Rollback(ctx)
	session, err := loadRPSessionRecord(ctx, tx.conn, r.PrincipalID, r.SessionID)
	if err != nil {
		return RPInteractionModeView{}, err
	}
	currentErr := requireCurrentRPSession(ctx, tx.conn, session)
	if currentErr == nil {
		if err := authorizeRPControl(ctx, tx.conn, r.PrincipalID, session.InstanceID, session.BranchID, session.ControlledEntityID); err != nil {
			return RPInteractionModeView{}, err
		}
	} else if !core.HasCode(currentErr, core.CodeBranchConflict) {
		return RPInteractionModeView{}, currentErr
	}
	var prior RPInteractionModeView
	var oldHash string
	err = tx.conn.QueryRowContext(ctx, `SELECT mode,revision,request_hash FROM rp_interaction_mode_revisions WHERE session_id=? AND idempotency_key=?`, r.SessionID, r.IdempotencyKey).Scan(&prior.Mode, &prior.Revision, &oldHash)
	if err == nil {
		if oldHash != hash {
			return RPInteractionModeView{}, core.NewError(core.CodeIdempotencyMismatch, "interaction mode key was used with another request")
		}
		prior.Replayed = true
		return prior, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return RPInteractionModeView{}, core.WrapError(core.CodeStorageFailure, "look up interaction mode revision", err)
	}
	if currentErr != nil {
		return RPInteractionModeView{}, currentErr
	}
	if session.Status != "active" {
		return RPInteractionModeView{}, core.NewError(core.CodeBranchConflict, "interaction mode requires active session")
	}
	current, err := readRPInteractionMode(ctx, tx.conn, r.SessionID)
	if err != nil {
		return RPInteractionModeView{}, err
	}
	if current.Revision != r.ExpectedRevision {
		return RPInteractionModeView{}, core.NewError(core.CodeBranchConflict, "interaction session mode revision is stale")
	}
	next := current.Revision + 1
	if _, err := tx.conn.ExecContext(ctx, `INSERT INTO rp_interaction_mode_revisions(session_id,revision,principal_id,idempotency_key,request_hash,mode,created_at_utc) VALUES (?,?,?,?,?,?,?)`, r.SessionID, next, r.PrincipalID, r.IdempotencyKey, hash, r.Mode, s.now().UTC().Format(time.RFC3339Nano)); err != nil {
		return RPInteractionModeView{}, core.WrapError(core.CodeStorageFailure, "save interaction session mode", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return RPInteractionModeView{}, err
	}
	return RPInteractionModeView{Mode: r.Mode, Revision: next}, nil
}
