package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"corerp.local/backend/internal/core"
)

type RPSharedInformationRelayRequest struct {
	PrincipalID        string `json:"principal_id"`
	SessionID          string `json:"session_id"`
	RoundID            string `json:"round_id"`
	MessageID          string `json:"message_id"`
	ForwardedMessageID string `json:"forwarded_message_id"`
	RecipientEntityID  string `json:"recipient_entity_id"`
	IdempotencyKey     string `json:"idempotency_key"`
}

func (r RPSharedInformationRelayRequest) Validate() error {
	if !studioID(r.PrincipalID) || !studioID(r.SessionID) || !studioID(r.RoundID) ||
		!studioID(r.MessageID) || !studioID(r.ForwardedMessageID) || !studioID(r.RecipientEntityID) ||
		!studioID(r.IdempotencyKey) || len(r.IdempotencyKey) > 128 || r.MessageID == r.ForwardedMessageID {
		return core.NewError(core.CodeInvalidArgument, "bounded shared information relay required")
	}
	return nil
}

// The proposal is not a rumor Event. The original RelayRPInformation command
// rechecks consent, receipt and audience inside the selected Event transaction.
func (s *Store) SubmitRPSharedInformationRelay(ctx context.Context, r RPSharedInformationRelayRequest) (RPSharedRound, error) {
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
		if oldKind != "information_relay" || oldKey != r.IdempotencyKey {
			return empty, core.NewError(core.CodeBranchConflict, "shared-round participant already submitted an action")
		}
		if oldHash != hash {
			return empty, core.NewError(core.CodeIdempotencyMismatch, "shared relay retry differs")
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
	parentSendID, _, err := rpInformationRecipientSource(ctx, tx.conn, row.Instance, row.Branch,
		session.ControlledEntityID, r.ForwardedMessageID, row.BaselineHead)
	if err != nil {
		return empty, err
	}
	var raw string
	if err := tx.conn.QueryRowContext(ctx, `SELECT payload FROM events WHERE event_id=? AND instance_id=? AND branch_id=? AND event_type='RPInformationSent'`,
		parentSendID, row.Instance, row.Branch).Scan(&raw); err != nil {
		return empty, core.NewError(core.CodeProjectionDiverged, "relay source missing")
	}
	var parent RPInformationFact
	if err := json.Unmarshal([]byte(raw), &parent); err != nil {
		return empty, core.NewError(core.CodeProjectionDiverged, "relay source invalid")
	}
	if parent.Channel != "direct_message" || !parent.AllowRelay || parent.ForwardedFromSendEventID != "" ||
		parent.RecipientID != session.ControlledEntityID {
		return empty, core.NewError(core.CodeUnauthorized, "private claim has no relay permission")
	}
	recipient, err := rpResolvePublicEntityID(ctx, tx.conn, row.Instance, row.Branch, session.ControlledEntityID, r.RecipientEntityID)
	if err != nil {
		return empty, err
	}
	known, err := rpIdentityKnown(ctx, tx.conn, row.Instance, row.Branch, session.ControlledEntityID, recipient)
	if err != nil {
		return empty, err
	}
	if !known || recipient == session.ControlledEntityID {
		return empty, core.NewError(core.CodeNotFound, "known distinct relay recipient required")
	}
	if err := validateRPBinding(ctx, tx.conn, row.Instance, row.Branch, recipient); err != nil {
		return empty, core.NewError(core.CodeNotFound, "relay recipient unavailable")
	}
	var duplicate int
	if err := tx.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPInformationSent' AND json_extract(payload,'$.message_id')=?`,
		row.Instance, row.Branch, r.MessageID).Scan(&duplicate); err != nil {
		return empty, err
	}
	if duplicate != 0 {
		return empty, core.NewError(core.CodeBranchConflict, "message ID already used")
	}
	child := RPInformationRelayRequest{Binding: core.CareerBinding{PrincipalID: r.PrincipalID,
		InstanceID: row.Instance, BranchID: row.Branch, ExpectedHead: row.BaselineHead,
		IdempotencyKey: "shared_action_" + row.ID}, SessionID: r.SessionID,
		MessageID: r.MessageID, ForwardedMessageID: r.ForwardedMessageID, RecipientEntityID: r.RecipientEntityID}
	if err := child.Validate(); err != nil {
		return empty, err
	}
	childJSON, err := core.CanonicalJSON(child)
	if err != nil {
		return empty, err
	}
	if _, err := tx.conn.ExecContext(ctx, `INSERT INTO rp_shared_round_actions
	 (round_id,session_id,submission_key,request_hash,request_json,submitted_at_utc,action_kind)
	 VALUES (?,?,?,?,?,?,'information_relay')`, row.ID, r.SessionID, r.IdempotencyKey, hash, string(childJSON),
		s.now().UTC().Format(time.RFC3339Nano)); err != nil {
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

func requireRPInformationRelayWindow(ctx context.Context, conn *sql.Conn, r RPInformationRelayRequest) error {
	var roundID, raw string
	var baseline int64
	err := conn.QueryRowContext(ctx, `SELECT r.round_id,r.baseline_head,a.request_json FROM rp_shared_rounds r
	 JOIN rp_shared_round_actions a ON a.round_id=r.round_id AND a.session_id=r.selected_session_id
	 WHERE r.instance_id=? AND r.branch_id=? AND r.status='advancing' AND r.settlement_kind='information'
	 AND r.selected_action_kind='information_relay' AND a.action_kind='information_relay' AND r.selected_session_id=?`,
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
			return core.NewError(core.CodeCommandInProgress, "relay requires a shared action window with external residents")
		}
		return nil
	}
	if err != nil {
		return err
	}
	if r.Binding.IdempotencyKey != "shared_action_"+roundID || r.Binding.ExpectedHead != baseline {
		return core.NewError(core.CodeUnauthorized, "information relay differs from selected shared child")
	}
	pinned, err := core.CanonicalJSON(r)
	if err != nil {
		return err
	}
	if string(pinned) != raw {
		return core.NewError(core.CodeUnauthorized, "information relay payload differs from selected shared child")
	}
	return nil
}

func (s *Store) advanceRPSharedInformationRelay(ctx context.Context, r RPSharedRoundAdvanceRequest, row rpSharedRoundRow) (RPSharedRound, error) {
	if s.afterRPSharedActionStage != nil {
		if err := s.afterRPSharedActionStage("selection_committed"); err != nil {
			return RPSharedRound{}, err
		}
	}
	var raw string
	if err := s.db.QueryRowContext(ctx, `SELECT request_json FROM rp_shared_round_actions WHERE round_id=? AND session_id=? AND action_kind='information_relay'`,
		row.ID, row.SelectedSession).Scan(&raw); err != nil {
		return RPSharedRound{}, classifyMissing(err, "selected shared information relay")
	}
	var child RPInformationRelayRequest
	if err := json.Unmarshal([]byte(raw), &child); err != nil {
		return RPSharedRound{}, core.WrapError(core.CodeProjectionDiverged, "decode selected shared information relay", err)
	}
	if child.SessionID != row.SelectedSession || child.Binding.InstanceID != row.Instance || child.Binding.BranchID != row.Branch ||
		child.Binding.ExpectedHead != row.BaselineHead || child.Binding.IdempotencyKey != "shared_action_"+row.ID {
		return RPSharedRound{}, core.NewError(core.CodeProjectionDiverged, "selected shared information relay binding differs")
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
		if err := check.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM commands WHERE instance_id=? AND branch_id=? AND command_type='RelayRPInformation' AND idempotency_key=? AND status='committed'`,
			row.Instance, row.Branch, key).Scan(&accepted); err != nil {
			check.Rollback(ctx)
			return RPSharedRound{}, err
		}
		if accepted == 0 {
			if _, err := check.conn.ExecContext(ctx, `UPDATE rp_shared_rounds SET status='stale' WHERE round_id=? AND status='advancing' AND settlement_kind='information' AND selected_action_kind='information_relay' AND selected_session_id=?`, row.ID, row.SelectedSession); err != nil {
				check.Rollback(ctx)
				return RPSharedRound{}, err
			}
			if err := check.Commit(ctx); err != nil {
				return RPSharedRound{}, err
			}
			return RPSharedRound{}, core.NewError(core.CodeBranchConflict, "shared information relay baseline changed before typed acceptance")
		}
	}
	check.Rollback(ctx)
	accepted, err := s.RelayRPInformation(ctx, child)
	if err != nil {
		return RPSharedRound{}, err
	}
	if s.afterRPSharedActionStage != nil {
		if err := s.afterRPSharedActionStage("information_relay_event_committed"); err != nil {
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
	if current.Status != "advancing" || current.SettlementKind != "information" || current.SelectedActionKind != "information_relay" ||
		current.SelectedSession != child.SessionID || accepted.EventSequence != row.BaselineHead+1 {
		return RPSharedRound{}, core.NewError(core.CodeBranchConflict, "shared information relay settlement changed")
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
