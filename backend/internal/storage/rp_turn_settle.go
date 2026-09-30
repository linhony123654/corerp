package storage

import (
	"context"
	"database/sql"

	"corerp.local/backend/internal/core"
)

// For a fresh ordinary turn, authorize and fence the activity sweep under the
// same world write lock, before a turn intent can pin a soon-to-be-stale cursor.
// ensureRPTurnRun checks the fences again before saving the intent; if this
// sweep advanced the head, the caller must observe again with the same key.
func (s *Store) settleBeforeRPTurnIntent(ctx context.Context, request core.RPSpeechRequest, provider core.RPDecisionProvider) (bool, error) {
	session, err := loadRPSessionRecord(ctx, s.db, request.PrincipalID, request.SessionID)
	if err != nil {
		return false, err
	}
	fresh := false
	err = s.settleRPActivitiesWhen(ctx, session.InstanceID, session.BranchID, func(conn *sql.Conn) (bool, error) {
		scoped, err := loadRPSessionRecord(ctx, conn, request.PrincipalID, request.SessionID)
		if err != nil {
			return false, err
		}
		if scoped.InstanceID != session.InstanceID || scoped.BranchID != session.BranchID {
			return false, core.NewError(core.CodeBranchConflict, "RP session scope changed before activity sweep")
		}
		var existing int
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_turn_runs WHERE session_id=? AND idempotency_key=?`, scoped.SessionID, request.IdempotencyKey).Scan(&existing); err != nil {
			return false, core.WrapError(core.CodeStorageFailure, "check RP turn before activity sweep", err)
		}
		if existing != 0 {
			return false, nil
		}
		if provider == nil {
			return false, core.NewError(core.CodeInvalidArgument, "new RP turn requires a decision provider")
		}
		if err := requireCurrentRPSession(ctx, conn, scoped); err != nil {
			return false, err
		}
		if err := authorizeRPControl(ctx, conn, request.PrincipalID, scoped.InstanceID, scoped.BranchID, scoped.ControlledEntityID); err != nil {
			return false, err
		}
		if scoped.Status != "active" {
			return false, core.NewError(core.CodeBranchConflict, "RP session is closed")
		}
		hash, err := core.HashJSON(request)
		if err != nil {
			return false, err
		}
		adopted, err := adoptedRPSharedSpeech(ctx, conn, scoped, request, hash)
		if err != nil || adopted {
			return false, err
		}
		if err := checkRPRequestRetirement(ctx, conn, request.PrincipalID, "dialogue", scoped.SessionID, request.IdempotencyKey); err != nil {
			return false, err
		}
		if err := requireNoActiveRPSharedRound(ctx, conn, scoped.InstanceID, scoped.BranchID); err != nil {
			return false, err
		}
		if scoped.InstanceID != M2DemoInstanceID || scoped.BranchID != M2DemoBranchID {
			packages, err := readStudioActivePackages(ctx, conn, scoped.InstanceID, scoped.BranchID)
			if err != nil {
				return false, err
			}
			if packages == nil {
				return false, core.NewError(core.CodeNotFound, "supported RP world not found")
			}
		}
		var pendingWaits, pendingTurns int
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_wait_intents i JOIN rp_sessions s ON s.session_id=i.session_id WHERE s.instance_id=? AND s.branch_id=? AND i.status='pending'`, scoped.InstanceID, scoped.BranchID).Scan(&pendingWaits); err != nil {
			return false, core.WrapError(core.CodeStorageFailure, "check pending wait before RP activity sweep", err)
		}
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_turn_runs r JOIN rp_sessions s ON s.session_id=r.session_id WHERE s.instance_id=? AND s.branch_id=? AND r.status<>'settled'`, scoped.InstanceID, scoped.BranchID).Scan(&pendingTurns); err != nil {
			return false, core.WrapError(core.CodeStorageFailure, "check pending turn before RP activity sweep", err)
		}
		if pendingWaits != 0 || pendingTurns != 0 {
			return false, core.NewError(core.CodeCommandInProgress, "another RP action or turn is pending")
		}
		var head int64
		if err := conn.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, scoped.InstanceID, scoped.BranchID).Scan(&head); err != nil {
			return false, classifyMissing(err, "RP turn branch")
		}
		if request.ExpectedCursor != head || scoped.ObservationCursor != head {
			return false, core.NewError(core.CodeBranchConflict, "RP turn requires a current observation cursor")
		}
		fresh = true
		return true, nil
	})
	return fresh, err
}
