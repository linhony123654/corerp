package storage

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"corerp.local/backend/internal/core"
)

// RPChapterStartRequest starts a new presentation chapter for one RP session.
// It never deletes Events or changes world/agent projections.
type RPChapterStartRequest struct {
	PrincipalID    string `json:"principal_id,omitempty"`
	SessionID      string `json:"session_id"`
	ExpectedCursor int64  `json:"expected_cursor"`
	IdempotencyKey string `json:"idempotency_key"`
}

type RPChapterStartResult struct {
	SessionID            string `json:"session_id"`
	ChapterStartSequence int64  `json:"chapter_start_sequence"`
	Replayed             bool   `json:"replayed,omitempty"`
}

func rpSessionChapterAvailable(ctx context.Context, q rpQueryer) (bool, error) {
	var count int
	if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_meta WHERE schema_version=?`, RPSessionChapterSchemaVersion).Scan(&count); err != nil {
		return false, core.WrapError(core.CodeStorageFailure, "read RP session chapter schema", err)
	}
	return count == 1, nil
}

func (s *Store) StartRPChapter(ctx context.Context, r RPChapterStartRequest) (RPChapterStartResult, error) {
	if strings.TrimSpace(r.PrincipalID) == "" || strings.TrimSpace(r.SessionID) == "" || strings.TrimSpace(r.IdempotencyKey) == "" || len(r.IdempotencyKey) > 128 || r.ExpectedCursor < 0 || r.ExpectedCursor >= core.MaxJSONSafeInteger {
		return RPChapterStartResult{}, core.NewError(core.CodeInvalidArgument, "invalid RP chapter request")
	}
	hash, err := core.HashJSON(r)
	if err != nil {
		return RPChapterStartResult{}, err
	}
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return RPChapterStartResult{}, core.WrapError(core.CodeStorageFailure, "begin RP chapter start", err)
	}
	defer tx.Rollback(ctx)
	session, err := loadRPSessionRecord(ctx, tx.conn, r.PrincipalID, r.SessionID)
	if err != nil {
		return RPChapterStartResult{}, err
	}
	currentErr := requireCurrentRPSession(ctx, tx.conn, session)
	if currentErr == nil {
		if err := authorizeRPControl(ctx, tx.conn, r.PrincipalID, session.InstanceID, session.BranchID, session.ControlledEntityID); err != nil {
			return RPChapterStartResult{}, err
		}
	} else if !core.HasCode(currentErr, core.CodeBranchConflict) {
		return RPChapterStartResult{}, currentErr
	}
	var prior RPChapterStartResult
	var oldHash string
	err = tx.conn.QueryRowContext(ctx, `SELECT session_id,chapter_start_sequence,request_hash FROM rp_session_chapter_resets WHERE principal_id=? AND idempotency_key=?`, r.PrincipalID, r.IdempotencyKey).Scan(&prior.SessionID, &prior.ChapterStartSequence, &oldHash)
	if err == nil {
		if oldHash != hash || prior.SessionID != r.SessionID {
			return RPChapterStartResult{}, core.NewError(core.CodeIdempotencyMismatch, "RP chapter key was used with another request")
		}
		prior.Replayed = true
		return prior, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return RPChapterStartResult{}, core.WrapError(core.CodeStorageFailure, "look up RP chapter reset", err)
	}
	if currentErr != nil {
		return RPChapterStartResult{}, currentErr
	}
	if session.Status != "active" {
		return RPChapterStartResult{}, core.NewError(core.CodeBranchConflict, "new chapter requires an active RP session")
	}
	if session.ObservationCursor != r.ExpectedCursor {
		return RPChapterStartResult{}, core.NewError(core.CodeBranchConflict, "RP observation cursor is stale")
	}
	var pending int
	if err := tx.conn.QueryRowContext(ctx, `SELECT
		(SELECT COUNT(*) FROM rp_turn_runs WHERE session_id=? AND status<>'settled')+
		(SELECT COUNT(*) FROM rp_wait_intents WHERE session_id=? AND status='pending')+
		(SELECT COUNT(*) FROM rp_interactions WHERE session_id=? AND status IN ('open','paused'))`, r.SessionID, r.SessionID, r.SessionID).Scan(&pending); err != nil {
		return RPChapterStartResult{}, core.WrapError(core.CodeStorageFailure, "check RP chapter pending work", err)
	}
	if pending != 0 {
		return RPChapterStartResult{}, core.NewError(core.CodeCommandInProgress, "finish or recover the current RP action before starting a new chapter")
	}
	var head int64
	if err := tx.conn.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, session.InstanceID, session.BranchID).Scan(&head); err != nil {
		return RPChapterStartResult{}, core.WrapError(core.CodeStorageFailure, "read RP chapter boundary", err)
	}
	if _, err := tx.conn.ExecContext(ctx, `UPDATE rp_sessions SET chapter_start_sequence=? WHERE session_id=?`, head, r.SessionID); err != nil {
		return RPChapterStartResult{}, core.WrapError(core.CodeStorageFailure, "save RP chapter boundary", err)
	}
	if _, err := tx.conn.ExecContext(ctx, `INSERT INTO rp_session_chapter_resets(session_id,principal_id,idempotency_key,request_hash,chapter_start_sequence,created_at_utc) VALUES (?,?,?,?,?,?)`, r.SessionID, r.PrincipalID, r.IdempotencyKey, hash, head, s.now().UTC().Format(time.RFC3339Nano)); err != nil {
		return RPChapterStartResult{}, core.WrapError(core.CodeStorageFailure, "record RP chapter reset", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return RPChapterStartResult{}, core.WrapError(core.CodeStorageFailure, "commit RP chapter start", err)
	}
	return RPChapterStartResult{SessionID: r.SessionID, ChapterStartSequence: head}, nil
}
