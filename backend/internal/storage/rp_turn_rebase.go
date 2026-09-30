package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"corerp.local/backend/internal/core"
)

// A legacy open run may have pinned a cursor before an automatic activity
// sweep. Only the player, after observing a newer head, may explicitly retry
// the SAME speech and style under the SAME key. Never rebind committed speech,
// provider work, or a semantic interaction plan.
func (s *Store) rebaseUncommittedRPTurn(ctx context.Context, conn *sql.Conn, session RPSession, run rpTurnRun, oldHash, oldJSON, newHash string, newJSON []byte, request core.RPSpeechRequest) error {
	if run.Status != "open" || run.PlayerTurnID != "" || run.PlayerEventID != "" {
		return core.NewError(core.CodeIdempotencyMismatch, "accepted RP turn cannot change its request")
	}
	var original core.RPSpeechRequest
	if err := json.Unmarshal([]byte(oldJSON), &original); err != nil {
		return core.WrapError(core.CodeProjectionDiverged, "decode uncommitted RP turn", err)
	}
	if request.ExpectedCursor <= original.ExpectedCursor {
		return core.NewError(core.CodeIdempotencyMismatch, "RP turn retry changed more than its cursor")
	}
	oldCursor := original.ExpectedCursor
	original.ExpectedCursor = request.ExpectedCursor
	matchingHash, err := core.HashJSON(original)
	if err != nil {
		return err
	}
	if matchingHash != newHash {
		return core.NewError(core.CodeIdempotencyMismatch, "RP turn retry changed more than its cursor")
	}
	var head int64
	if err := conn.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, session.InstanceID, session.BranchID).Scan(&head); err != nil {
		return classifyMissing(err, "RP turn rebase branch")
	}
	if head != request.ExpectedCursor || session.ObservationCursor != head || session.Status != "active" {
		return core.NewError(core.CodeBranchConflict, "RP turn rebase requires a fresh observation")
	}
	var effects int
	if err := conn.QueryRowContext(ctx, `SELECT
		(SELECT COUNT(*) FROM commands WHERE instance_id=? AND branch_id=? AND command_type='RPSpeak' AND idempotency_key=?)
		+(SELECT COUNT(*) FROM rp_provider_calls WHERE turn_run_id=?)
		+(SELECT COUNT(*) FROM rp_turn_listener_skips WHERE turn_run_id=?)`, session.InstanceID, session.BranchID, "rp_speech:"+session.SessionID+":"+run.SpeechKey, run.ID, run.ID).Scan(&effects); err != nil {
		return core.WrapError(core.CodeStorageFailure, "check uncommitted RP turn effects", err)
	}
	if effects != 0 {
		return core.NewError(core.CodeIdempotencyMismatch, "RP turn has accepted work and cannot be rebound")
	}
	var revision int
	if err := conn.QueryRowContext(ctx, `SELECT COALESCE(MAX(revision),0)+1 FROM rp_turn_explicit_rebases WHERE turn_run_id=?`, run.ID).Scan(&revision); err != nil {
		return core.WrapError(core.CodeStorageFailure, "read RP turn rebase revision", err)
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	if err := execAgentOne(ctx, conn, "record explicit RP turn rebase", `INSERT INTO rp_turn_explicit_rebases(turn_run_id,revision,old_request_hash,new_request_hash,old_cursor,new_cursor,reason,created_at_utc) VALUES (?,?,?,?,?,?,'explicit_current_observation',?)`, run.ID, revision, oldHash, newHash, oldCursor, request.ExpectedCursor, now); err != nil {
		return err
	}
	return execAgentOne(ctx, conn, "rebind uncommitted RP turn cursor", `UPDATE rp_turn_runs SET request_hash=?,request_json=?,updated_at_utc=? WHERE turn_run_id=? AND status='open' AND request_hash=? AND player_event_id IS NULL`, newHash, string(newJSON), now, run.ID, oldHash)
}
