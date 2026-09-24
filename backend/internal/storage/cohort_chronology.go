package storage

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"corerp.local/backend/internal/core"
)

// A population/claim transition is an external command, so it may share a
// committed world instant, but it cannot precede any committed fact or skip a
// due scheduler boundary. Check instants, not lexical RFC3339 strings: callers
// may express the same instant with different UTC offsets.
func ensureCohortTransitionChronology(ctx context.Context, conn *sql.Conn, instanceID, branchID, worldTime string) error {
	if err := requireStudioWriteRules(ctx, conn, instanceID, branchID); err != nil {
		return err
	}
	requested, err := time.Parse(time.RFC3339, worldTime)
	if err != nil {
		return core.WrapError(core.CodeInvalidArgument, "invalid Cohort transition world time", err)
	}
	events, err := conn.QueryContext(ctx, `SELECT world_time FROM events WHERE instance_id = ? AND branch_id = ?`, instanceID, branchID)
	if err != nil {
		return core.WrapError(core.CodeStorageFailure, "read Cohort transition history", err)
	}
	for events.Next() {
		var recordedText string
		if err := events.Scan(&recordedText); err != nil {
			events.Close()
			return core.WrapError(core.CodeStorageFailure, "scan Cohort transition history", err)
		}
		recorded, err := time.Parse(time.RFC3339, recordedText)
		if err != nil {
			events.Close()
			return core.WrapError(core.CodeProjectionDiverged, "invalid committed world time", err)
		}
		if recorded.After(requested) {
			events.Close()
			return core.NewError(core.CodeBranchConflict, "Cohort transition predates committed branch history")
		}
	}
	if err := events.Err(); err != nil {
		events.Close()
		return core.WrapError(core.CodeStorageFailure, "iterate Cohort transition history", err)
	}
	if err := events.Close(); err != nil {
		return core.WrapError(core.CodeStorageFailure, "close Cohort transition history", err)
	}
	var clockText string
	err = conn.QueryRowContext(ctx, `SELECT current_world_time FROM world_clocks WHERE instance_id = ? AND branch_id = ?`, instanceID, branchID).Scan(&clockText)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return core.WrapError(core.CodeStorageFailure, "read Cohort transition clock", err)
	}
	if err == nil {
		clock, err := time.Parse(time.RFC3339, clockText)
		if err != nil {
			return core.WrapError(core.CodeProjectionDiverged, "invalid committed world clock", err)
		}
		if clock.After(requested) {
			return core.NewError(core.CodeBranchConflict, "Cohort transition predates committed world clock")
		}
	}
	pending, err := conn.QueryContext(ctx, `SELECT world_time FROM scheduler_items WHERE instance_id = ? AND branch_id = ? AND status = 'pending'`, instanceID, branchID)
	if err != nil {
		return core.WrapError(core.CodeStorageFailure, "read Cohort transition pending boundaries", err)
	}
	for pending.Next() {
		var dueText string
		if err := pending.Scan(&dueText); err != nil {
			pending.Close()
			return core.WrapError(core.CodeStorageFailure, "scan Cohort transition pending boundary", err)
		}
		due, err := time.Parse(time.RFC3339, dueText)
		if err != nil {
			pending.Close()
			return core.WrapError(core.CodeProjectionDiverged, "invalid pending Cohort boundary time", err)
		}
		if !due.After(requested) {
			pending.Close()
			return core.NewError(core.CodeBranchConflict, "Cohort transition would skip a due scheduler item")
		}
	}
	if err := pending.Err(); err != nil {
		pending.Close()
		return core.WrapError(core.CodeStorageFailure, "iterate Cohort transition pending boundaries", err)
	}
	if err := pending.Close(); err != nil {
		return core.WrapError(core.CodeStorageFailure, "close Cohort transition pending boundaries", err)
	}
	return nil
}
