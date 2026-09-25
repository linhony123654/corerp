package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"corerp.local/backend/internal/core"
)

// The proposal contains only the employee's chosen bounded task. Its outcome
// is computed by AttemptRPWorkTask after the full shared round selects it.
type RPSharedWorkTaskRequest struct {
	PrincipalID    string `json:"principal_id"`
	SessionID      string `json:"session_id"`
	RoundID        string `json:"round_id"`
	ContractID     string `json:"contract_id"`
	TaskCode       string `json:"task_code"`
	IdempotencyKey string `json:"idempotency_key"`
}

func (s *Store) SubmitRPSharedWorkTask(ctx context.Context, r RPSharedWorkTaskRequest) (RPSharedRound, error) {
	var empty RPSharedRound
	if !studioID(r.PrincipalID) || !studioID(r.SessionID) || !studioID(r.RoundID) ||
		!studioID(r.IdempotencyKey) || len(r.IdempotencyKey) > 128 || !studioID(r.ContractID) || r.TaskCode != "routine_check" {
		return empty, core.NewError(core.CodeInvalidArgument, "bounded shared work task request required")
	}
	hash, err := core.HashJSON(r)
	if err != nil {
		return empty, err
	}
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return empty, err
	}
	defer tx.Rollback(ctx)
	row, err := readRPSharedRoundRow(ctx, tx.conn, r.RoundID)
	if err != nil {
		return empty, err
	}
	identity := RPSharedRoundReadRequest{PrincipalID: r.PrincipalID, SessionID: r.SessionID, RoundID: r.RoundID}
	if err := authorizeRPSharedParticipant(ctx, tx.conn, row, identity, false); err != nil {
		return empty, err
	}
	var oldKey, oldHash, oldKind string
	err = tx.conn.QueryRowContext(ctx, `SELECT submission_key,request_hash,action_kind FROM rp_shared_round_actions WHERE round_id=? AND session_id=?`, row.ID, r.SessionID).Scan(&oldKey, &oldHash, &oldKind)
	if err == nil {
		if oldKind != "work_task" || oldKey != r.IdempotencyKey {
			return empty, core.NewError(core.CodeBranchConflict, "shared-round participant already submitted an action")
		}
		if oldHash != hash {
			return empty, core.NewError(core.CodeIdempotencyMismatch, "shared work task retry differs")
		}
		if row.Status == "open" || row.Status == "advancing" {
			if err := authorizeRPSharedParticipant(ctx, tx.conn, row, identity, true); err != nil {
				return empty, err
			}
		}
		out, err := rpSharedRoundViewForParticipant(ctx, tx.conn, row, r.SessionID)
		out.Replayed = true
		return out, err
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return empty, err
	}
	if row.Status != "open" {
		return empty, core.NewError(core.CodeBranchConflict, "shared round no longer accepts submissions")
	}
	if err := authorizeRPSharedParticipant(ctx, tx.conn, row, identity, true); err != nil {
		return empty, err
	}
	var waitKey string
	if err := tx.conn.QueryRowContext(ctx, `SELECT submission_key FROM rp_shared_round_participants WHERE round_id=? AND session_id=?`, row.ID, r.SessionID).Scan(&waitKey); err != nil {
		return empty, err
	}
	if waitKey != "" {
		return empty, core.NewError(core.CodeBranchConflict, "shared-round participant already submitted a wait")
	}
	var head, cursor int64
	var at string
	if err := tx.conn.QueryRowContext(ctx, `SELECT b.head_sequence,c.current_world_time,s.observation_cursor
		FROM branches b JOIN world_clocks c ON c.instance_id=b.instance_id AND c.branch_id=b.branch_id
		JOIN rp_sessions s ON s.session_id=? WHERE b.instance_id=? AND b.branch_id=?`,
		r.SessionID, row.Instance, row.Branch).Scan(&head, &at, &cursor); err != nil {
		return empty, err
	}
	if head != row.BaselineHead || at != row.BaselineTime {
		return empty, staleRPSharedRound(ctx, tx, row.ID)
	}
	if cursor != head {
		return empty, core.NewError(core.CodeBranchConflict, "participant has not observed shared baseline")
	}
	// Reject plainly impossible proposals before a selected round could become
	// stuck. The typed Event owner checks these facts again at acceptance.
	childBinding := core.CareerBinding{PrincipalID: r.PrincipalID, InstanceID: row.Instance,
		BranchID: row.Branch, ExpectedHead: row.BaselineHead, IdempotencyKey: "shared_action_" + row.ID}
	session, err := loadRPSessionRecord(ctx, tx.conn, r.PrincipalID, r.SessionID)
	if err != nil {
		return empty, err
	}
	var ownContract int
	if err := tx.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM employment_contracts c
		JOIN events e ON e.event_id=c.definition_event_id
		WHERE c.contract_id=? AND c.employee_entity_id=? AND e.instance_id=? AND e.branch_id=?`,
		r.ContractID, session.ControlledEntityID, row.Instance, row.Branch).Scan(&ownContract); err != nil {
		return empty, err
	}
	if ownContract != 1 {
		return empty, core.NewError(core.CodeNotFound, "own scoped work contract not found")
	}
	job, _, day, err := currentCareerEmployment(ctx, tx.conn, childBinding, r.ContractID, at)
	if err != nil {
		return empty, err
	}
	if job.EmployeeID != session.ControlledEntityID {
		return empty, core.NewError(core.CodeProjectionDiverged, "own work contract differs from sourced terms")
	}
	now, err := time.Parse(time.RFC3339Nano, at)
	if err != nil {
		return empty, err
	}
	start, _ := time.Parse(time.RFC3339, careerTime(day, job.WorkStartHour, 0))
	end, _ := time.Parse(time.RFC3339, careerTime(day, job.WorkEndHour, 0))
	if job.WorkStartHour < 0 || job.WorkEndHour > 24 || job.WorkStartHour >= job.WorkEndHour || now.Before(start) || !now.Before(end) {
		return empty, core.NewError(core.CodeBranchConflict, "work sample requires the current shift")
	}
	var place, activity string
	if err := tx.conn.QueryRowContext(ctx, `SELECT place_id,activity_code FROM agent_positions WHERE agent_id=?`, job.EmployeeID).Scan(&place, &activity); err != nil {
		return empty, err
	}
	if place != job.WorkplaceID || activity != "work" {
		return empty, core.NewError(core.CodeBranchConflict, "employee is not performing sourced work at this workplace")
	}
	if open, _, _, err := openRPSleep(ctx, tx.conn, row.Instance, row.Branch, job.EmployeeID); err != nil {
		return empty, err
	} else if open != "" {
		return empty, core.NewError(core.CodeBranchConflict, "end actor sleep before work sample")
	}
	var duplicate int
	if err := tx.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPWorkTaskAttempted'
		AND json_extract(payload,'$.contract_id')=? AND json_extract(payload,'$.day')=? AND json_extract(payload,'$.task_code')=?`,
		row.Instance, row.Branch, r.ContractID, day, r.TaskCode).Scan(&duplicate); err != nil {
		return empty, err
	}
	if duplicate != 0 {
		return empty, core.NewError(core.CodeBranchConflict, "work sample already recorded for this shift and task")
	}
	conditions, err := readRPActiveConditions(ctx, tx.conn, row.Instance, row.Branch, job.EmployeeID, at)
	if err != nil {
		return empty, err
	}
	for _, condition := range conditions {
		if condition.Kind == "minor_injury" && condition.Severity >= 3 {
			return empty, core.NewError(core.CodeBranchConflict, "current injury limits this work task")
		}
	}
	child := RPWorkTaskRequest{Binding: childBinding, SessionID: r.SessionID, ContractID: r.ContractID, TaskCode: r.TaskCode}
	childJSON, err := core.CanonicalJSON(child)
	if err != nil {
		return empty, err
	}
	if _, err := tx.conn.ExecContext(ctx, `INSERT INTO rp_shared_round_actions
		(round_id,session_id,submission_key,request_hash,request_json,submitted_at_utc,action_kind)
		VALUES (?,?,?,?,?,?,'work_task')`, row.ID, r.SessionID, r.IdempotencyKey, hash, string(childJSON), s.now().UTC().Format(time.RFC3339Nano)); err != nil {
		return empty, err
	}
	out, err := rpSharedRoundViewForParticipant(ctx, tx.conn, row, r.SessionID)
	if err != nil {
		return empty, err
	}
	if err := tx.Commit(ctx); err != nil {
		return empty, err
	}
	return out, nil
}

func (s *Store) advanceRPSharedWorkTask(ctx context.Context, r RPSharedRoundAdvanceRequest, row rpSharedRoundRow) (RPSharedRound, error) {
	if s.afterRPSharedActionStage != nil {
		if err := s.afterRPSharedActionStage("selection_committed"); err != nil {
			return RPSharedRound{}, err
		}
	}
	var raw string
	if err := s.db.QueryRowContext(ctx, `SELECT request_json FROM rp_shared_round_actions WHERE round_id=? AND session_id=? AND action_kind='work_task'`,
		row.ID, row.SelectedSession).Scan(&raw); err != nil {
		return RPSharedRound{}, classifyMissing(err, "selected shared work task")
	}
	var child RPWorkTaskRequest
	if err := json.Unmarshal([]byte(raw), &child); err != nil {
		return RPSharedRound{}, core.WrapError(core.CodeProjectionDiverged, "decode selected shared work task", err)
	}
	if child.SessionID != row.SelectedSession || child.Binding.InstanceID != row.Instance || child.Binding.BranchID != row.Branch ||
		child.Binding.ExpectedHead != row.BaselineHead || child.Binding.IdempotencyKey != "shared_action_"+row.ID {
		return RPSharedRound{}, core.NewError(core.CodeProjectionDiverged, "selected shared work task binding differs")
	}
	check, err := beginImmediate(ctx, s.db)
	if err != nil {
		return RPSharedRound{}, err
	}
	var head int64
	if err := check.conn.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, row.Instance, row.Branch).Scan(&head); err != nil {
		check.Rollback(ctx)
		return RPSharedRound{}, err
	}
	key, err := core.HashJSON([]string{child.Binding.PrincipalID, child.Binding.IdempotencyKey})
	if err != nil {
		check.Rollback(ctx)
		return RPSharedRound{}, err
	}
	if head != row.BaselineHead {
		var accepted int
		if err := check.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM commands WHERE instance_id=? AND branch_id=? AND command_type='AttemptRPWorkTask' AND idempotency_key=? AND status='committed'`,
			row.Instance, row.Branch, key).Scan(&accepted); err != nil {
			check.Rollback(ctx)
			return RPSharedRound{}, err
		}
		if accepted == 0 {
			if _, err := check.conn.ExecContext(ctx, `UPDATE rp_shared_rounds SET status='stale' WHERE round_id=? AND status='advancing' AND settlement_kind='health' AND selected_action_kind='work_task' AND selected_session_id=?`, row.ID, row.SelectedSession); err != nil {
				check.Rollback(ctx)
				return RPSharedRound{}, err
			}
			if err := check.Commit(ctx); err != nil {
				return RPSharedRound{}, err
			}
			return RPSharedRound{}, core.NewError(core.CodeBranchConflict, "shared work baseline changed before typed acceptance")
		}
	}
	check.Rollback(ctx)
	accepted, err := s.AttemptRPWorkTask(ctx, child)
	if err != nil {
		return RPSharedRound{}, err
	}
	if s.afterRPSharedActionStage != nil {
		if err := s.afterRPSharedActionStage("health_event_committed"); err != nil {
			return RPSharedRound{}, err
		}
	}
	settle, err := beginImmediate(ctx, s.db)
	if err != nil {
		return RPSharedRound{}, err
	}
	defer settle.Rollback(ctx)
	current, err := readRPSharedRoundRow(ctx, settle.conn, row.ID)
	if err != nil {
		return RPSharedRound{}, err
	}
	if current.Status == "settled" {
		out, err := rpSharedRoundViewForParticipant(ctx, settle.conn, current, r.SessionID)
		out.Replayed = true
		return out, err
	}
	if current.Status != "advancing" || current.SettlementKind != "health" || current.SelectedActionKind != "work_task" ||
		current.SelectedSession != child.SessionID || accepted.EventSequence != row.BaselineHead+1 {
		return RPSharedRound{}, core.NewError(core.CodeBranchConflict, "shared work task settlement changed")
	}
	if _, err := settle.conn.ExecContext(ctx, `UPDATE rp_shared_rounds SET status='settled',completion_event_id=?,settled_sequence=?,settled_at_utc=? WHERE round_id=? AND status='advancing'`,
		accepted.EventID, accepted.EventSequence, s.now().UTC().Format(time.RFC3339Nano), row.ID); err != nil {
		return RPSharedRound{}, err
	}
	current.Status, current.CompletionEvent, current.SettledSequence = "settled", accepted.EventID, accepted.EventSequence
	out, err := rpSharedRoundViewForParticipant(ctx, settle.conn, current, r.SessionID)
	if err != nil {
		return RPSharedRound{}, err
	}
	if err := settle.Commit(ctx); err != nil {
		return RPSharedRound{}, err
	}
	return out, nil
}
