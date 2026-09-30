package storage

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"corerp.local/backend/internal/core"
)

// A pending interpretation is an application-level in-flight claim. It does
// not say whether HTTP was sent; an interrupted, unaccepted plan can be
// reinterpreted after the bounded lease, but no accepted plan can change.
func (s *Store) claimRPInteractionUnderstanding(ctx context.Context, request core.RPInteractionRequest, hash string, provider any) (string, error) {
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	var accepted int
	err = tx.conn.QueryRowContext(ctx, `SELECT 1 FROM rp_interactions WHERE session_id=? AND idempotency_key=?`, request.SessionID, request.IdempotencyKey).Scan(&accepted)
	if err == nil {
		return "", core.NewError(core.CodeCommandInProgress, "interaction was accepted while interpretation started; retry original request")
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", core.WrapError(core.CodeStorageFailure, "check accepted interaction before interpretation", err)
	}
	var priorHash string
	err = tx.conn.QueryRowContext(ctx, `SELECT request_hash FROM rp_interaction_interpretations WHERE session_id=? AND idempotency_key=? ORDER BY started_unix DESC,call_id DESC LIMIT 1`, request.SessionID, request.IdempotencyKey).Scan(&priorHash)
	if err == nil && priorHash != hash {
		return "", core.NewError(core.CodeIdempotencyMismatch, "interaction key was used with different input")
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", core.WrapError(core.CodeStorageFailure, "check previous interpretation", err)
	}
	now := s.now().UTC()
	// The adapter's total request budget is at most one minute. An abandoned
	// pending row older than two minutes may be retired; its outcome remains
	// unknown rather than being relabelled as no HTTP attempt.
	_, err = tx.conn.ExecContext(ctx, `UPDATE rp_interaction_interpretations SET result='failed',reason='interrupted_unknown',finished_at_utc=? WHERE session_id=? AND idempotency_key=? AND result='pending' AND started_unix<?`, now.Format(time.RFC3339Nano), request.SessionID, request.IdempotencyKey, now.Add(-2*time.Minute).Unix())
	if err != nil {
		return "", core.WrapError(core.CodeStorageFailure, "retire interrupted interpretation", err)
	}
	var pending int
	err = tx.conn.QueryRowContext(ctx, `SELECT 1 FROM rp_interaction_interpretations WHERE session_id=? AND idempotency_key=? AND result='pending'`, request.SessionID, request.IdempotencyKey).Scan(&pending)
	if err == nil {
		return "", core.NewError(core.CodeCommandInProgress, "interaction understanding is still in progress")
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", core.WrapError(core.CodeStorageFailure, "check in-flight interpretation", err)
	}
	id, err := newRPSessionID()
	if err != nil {
		return "", err
	}
	id = "rpi_" + strings.TrimPrefix(id, "rps_")
	metadata := rpProviderMetadata(provider, "custom")
	_, err = tx.conn.ExecContext(ctx, `INSERT INTO rp_interaction_interpretations(call_id,session_id,idempotency_key,request_hash,baseline_cursor,source,provider_kind,model_id,result,started_unix) VALUES (?,?,?,?,?,'model',?,?,'pending',?)`, id, request.SessionID, request.IdempotencyKey, hash, request.ExpectedCursor, metadata.Kind, metadata.Model, now.Unix())
	if err != nil {
		return "", core.WrapError(core.CodeStorageFailure, "claim RP interaction understanding", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return id, nil
}

func (s *Store) failRPInteractionUnderstanding(ctx context.Context, callID, result, reason string, count int) error {
	if count < 0 || count > 100 || (result != "failed" && result != "timeout") {
		return core.NewError(core.CodeInvalidArgument, "invalid interpretation result")
	}
	auditCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	return execAgentOne(auditCtx, s.db, "finish RP interaction understanding", `UPDATE rp_interaction_interpretations SET result=?,reason=?,attempt_count=?,finished_at_utc=? WHERE call_id=? AND result='pending'`, result, reason, count, s.now().UTC().Format(time.RFC3339Nano), callID)
}
