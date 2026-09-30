package storage

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"corerp.local/backend/internal/core"
)

// The wait Event owns the world clock; this non-authoritative marker binds only
// the exact derived activity event range to that wait. Historical complete
// markers with no range remain unknown rather than acquiring invented lineage.
func (s *Store) settleCommittedRPWaitActivities(ctx context.Context, intentID string) error {
	var status, instance, branch string
	err := s.db.QueryRowContext(ctx, `SELECT a.status,s.instance_id,s.branch_id
		FROM rp_wait_activity_settlements a
		JOIN rp_wait_intents i ON i.intent_id=a.intent_id AND i.status='completed'
		JOIN rp_sessions s ON s.session_id=i.session_id
		WHERE a.intent_id=?`, intentID).Scan(&status, &instance, &branch)
	if errors.Is(err, sql.ErrNoRows) {
		// A completed wait from before migration 046 has no recoverable marker.
		return nil
	}
	if err != nil {
		return core.WrapError(core.CodeStorageFailure, "read RP wait activity settlement", err)
	}
	if status == "complete" {
		return nil
	}
	return s.settleRPActivitiesWhenFinal(ctx, instance, branch, func(conn *sql.Conn) (bool, error) {
		var target, current, currentStatus string
		err := conn.QueryRowContext(ctx, `SELECT a.status,i.target_world_time,c.current_world_time
			FROM rp_wait_activity_settlements a
			JOIN rp_wait_intents i ON i.intent_id=a.intent_id AND i.status='completed'
			JOIN rp_sessions s ON s.session_id=i.session_id
			JOIN world_clocks c ON c.instance_id=s.instance_id AND c.branch_id=s.branch_id
			WHERE a.intent_id=? AND s.instance_id=? AND s.branch_id=?`, intentID, instance, branch).Scan(&currentStatus, &target, &current)
		if err != nil {
			return false, classifyMissing(err, "committed wait activity settlement")
		}
		if currentStatus == "complete" {
			return false, nil
		}
		if current != target {
			return false, core.NewError(core.CodeBranchConflict, "RP wait activity settlement needs its committed world time")
		}
		return true, nil
	}, func(conn *sql.Conn, first, last int64) error {
		var firstValue, lastValue any
		if first != 0 {
			firstValue, lastValue = first, last
		}
		result, err := conn.ExecContext(ctx, `UPDATE rp_wait_activity_settlements SET status='complete',completed_at_utc=?,first_sequence=?,last_sequence=? WHERE intent_id=? AND status='pending'`, s.now().UTC().Format(time.RFC3339Nano), firstValue, lastValue, intentID)
		if err != nil {
			return core.WrapError(core.CodeStorageFailure, "complete RP wait activity settlement", err)
		}
		if count, err := result.RowsAffected(); err != nil || count != 1 {
			return core.NewError(core.CodeCommandInProgress, "RP wait activity settlement changed before commit")
		}
		return nil
	})
}
