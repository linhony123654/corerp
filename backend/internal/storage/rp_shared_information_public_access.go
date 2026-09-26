package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"corerp.local/backend/internal/core"
)

type RPSharedPublicNoticeAccessRequest struct {
	PrincipalID    string `json:"principal_id"`
	SessionID      string `json:"session_id"`
	RoundID        string `json:"round_id"`
	MessageID      string `json:"message_id"`
	IdempotencyKey string `json:"idempotency_key"`
}

func (r RPSharedPublicNoticeAccessRequest) Validate() error {
	if !studioID(r.PrincipalID) || !studioID(r.SessionID) || !studioID(r.RoundID) ||
		!studioID(r.MessageID) || !studioID(r.IdempotencyKey) || len(r.IdempotencyKey) > 128 {
		return core.NewError(core.CodeInvalidArgument, "bounded shared public notice access required")
	}
	return nil
}

// A proposal does not teach the reader. The selected child calls the original
// public-access Event owner, which rechecks source, control and duplicate read.
func (s *Store) SubmitRPSharedPublicNoticeAccess(ctx context.Context, r RPSharedPublicNoticeAccessRequest) (RPSharedRound, error) {
	var empty RPSharedRound
	if err := r.Validate(); err != nil {
		return empty, err
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
		if oldKind != "information_public_access" || oldKey != r.IdempotencyKey {
			return empty, core.NewError(core.CodeBranchConflict, "shared-round participant already submitted an action")
		}
		if oldHash != hash {
			return empty, core.NewError(core.CodeIdempotencyMismatch, "shared public access retry differs")
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
	session, err := loadRPSessionRecord(ctx, tx.conn, r.PrincipalID, r.SessionID)
	if err != nil {
		return empty, err
	}
	published, sendID, err := rpPublicPublishedSource(ctx, tx.conn, row.Instance, row.Branch, r.MessageID)
	if err != nil {
		return empty, err
	}
	if published.SenderID == session.ControlledEntityID {
		return empty, core.NewError(core.CodeNotFound, "notice author is not a recipient")
	}
	var duplicate int
	if err := tx.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPInformationDelivered'
	 AND json_extract(payload,'$.source_event_id')=? AND json_extract(payload,'$.recipient_id')=?`,
		row.Instance, row.Branch, sendID, session.ControlledEntityID).Scan(&duplicate); err != nil {
		return empty, err
	}
	if duplicate != 0 {
		return empty, core.NewError(core.CodeBranchConflict, "public notice already received")
	}
	child := RPPublicNoticeAccessRequest{Binding: core.CareerBinding{PrincipalID: r.PrincipalID,
		InstanceID: row.Instance, BranchID: row.Branch, ExpectedHead: row.BaselineHead,
		IdempotencyKey: "shared_action_" + row.ID}, SessionID: r.SessionID, MessageID: r.MessageID}
	if err := child.Validate(); err != nil {
		return empty, err
	}
	childJSON, err := core.CanonicalJSON(child)
	if err != nil {
		return empty, err
	}
	if _, err := tx.conn.ExecContext(ctx, `INSERT INTO rp_shared_round_actions
	 (round_id,session_id,submission_key,request_hash,request_json,submitted_at_utc,action_kind)
	 VALUES (?,?,?,?,?,?,'information_public_access')`, row.ID, r.SessionID, r.IdempotencyKey, hash,
		string(childJSON), s.now().UTC().Format(time.RFC3339Nano)); err != nil {
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

func requireRPPublicNoticeAccessWindow(ctx context.Context, conn *sql.Conn, r RPPublicNoticeAccessRequest) error {
	var roundID, raw string
	var baseline int64
	err := conn.QueryRowContext(ctx, `SELECT r.round_id,r.baseline_head,a.request_json FROM rp_shared_rounds r
	 JOIN rp_shared_round_actions a ON a.round_id=r.round_id AND a.session_id=r.selected_session_id
	 WHERE r.instance_id=? AND r.branch_id=? AND r.status='advancing' AND r.settlement_kind='information'
	 AND r.selected_action_kind='information_public_access' AND a.action_kind='information_public_access' AND r.selected_session_id=?`,
		r.Binding.InstanceID, r.Binding.BranchID, r.SessionID).Scan(&roundID, &baseline, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		if err := requireNoActiveRPSharedRound(ctx, conn, r.Binding.InstanceID, r.Binding.BranchID); err != nil {
			return err
		}
		var external int
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_controller_authorities WHERE instance_id=? AND branch_id=? AND status='active'`,
			r.Binding.InstanceID, r.Binding.BranchID).Scan(&external); err != nil {
			return err
		}
		if external != 0 {
			return core.NewError(core.CodeCommandInProgress, "public notice access requires a shared action window with external residents")
		}
		return nil
	}
	if err != nil {
		return err
	}
	if r.Binding.IdempotencyKey != "shared_action_"+roundID || r.Binding.ExpectedHead != baseline {
		return core.NewError(core.CodeUnauthorized, "public notice access differs from selected shared child")
	}
	pinned, err := core.CanonicalJSON(r)
	if err != nil {
		return err
	}
	if string(pinned) != raw {
		return core.NewError(core.CodeUnauthorized, "public notice access payload differs from selected shared child")
	}
	return nil
}

func (s *Store) advanceRPSharedPublicNoticeAccess(ctx context.Context, r RPSharedRoundAdvanceRequest, row rpSharedRoundRow) (RPSharedRound, error) {
	if s.afterRPSharedActionStage != nil {
		if err := s.afterRPSharedActionStage("selection_committed"); err != nil {
			return RPSharedRound{}, err
		}
	}
	var raw string
	if err := s.db.QueryRowContext(ctx, `SELECT request_json FROM rp_shared_round_actions WHERE round_id=? AND session_id=? AND action_kind='information_public_access'`,
		row.ID, row.SelectedSession).Scan(&raw); err != nil {
		return RPSharedRound{}, classifyMissing(err, "selected shared public access")
	}
	var child RPPublicNoticeAccessRequest
	if err := json.Unmarshal([]byte(raw), &child); err != nil {
		return RPSharedRound{}, core.WrapError(core.CodeProjectionDiverged, "decode selected shared public access", err)
	}
	if child.SessionID != row.SelectedSession || child.Binding.InstanceID != row.Instance || child.Binding.BranchID != row.Branch ||
		child.Binding.ExpectedHead != row.BaselineHead || child.Binding.IdempotencyKey != "shared_action_"+row.ID {
		return RPSharedRound{}, core.NewError(core.CodeProjectionDiverged, "selected shared public access binding differs")
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
		if err := check.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM commands WHERE instance_id=? AND branch_id=? AND command_type='AccessRPPublicNotice' AND idempotency_key=? AND status='committed'`,
			row.Instance, row.Branch, key).Scan(&accepted); err != nil {
			check.Rollback(ctx)
			return RPSharedRound{}, err
		}
		if accepted == 0 {
			if _, err := check.conn.ExecContext(ctx, `UPDATE rp_shared_rounds SET status='stale' WHERE round_id=? AND status='advancing' AND settlement_kind='information' AND selected_action_kind='information_public_access' AND selected_session_id=?`, row.ID, row.SelectedSession); err != nil {
				check.Rollback(ctx)
				return RPSharedRound{}, err
			}
			if err := check.Commit(ctx); err != nil {
				return RPSharedRound{}, err
			}
			return RPSharedRound{}, core.NewError(core.CodeBranchConflict, "shared public access baseline changed before typed acceptance")
		}
	}
	check.Rollback(ctx)
	accepted, err := s.AccessRPPublicNotice(ctx, child)
	if err != nil {
		return RPSharedRound{}, err
	}
	if s.afterRPSharedActionStage != nil {
		if err := s.afterRPSharedActionStage("information_public_access_event_committed"); err != nil {
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
	if current.Status != "advancing" || current.SettlementKind != "information" || current.SelectedActionKind != "information_public_access" ||
		current.SelectedSession != child.SessionID || accepted.EventSequence != row.BaselineHead+1 {
		return RPSharedRound{}, core.NewError(core.CodeBranchConflict, "shared public access settlement changed")
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
