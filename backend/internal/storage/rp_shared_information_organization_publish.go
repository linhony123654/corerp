package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"corerp.local/backend/internal/core"
)

type RPSharedOrganizationNoticePublishRequest struct {
	PrincipalID    string `json:"principal_id"`
	SessionID      string `json:"session_id"`
	RoundID        string `json:"round_id"`
	MessageID      string `json:"message_id"`
	SourceHandle   string `json:"source_handle"`
	IdempotencyKey string `json:"idempotency_key"`
}

func (r RPSharedOrganizationNoticePublishRequest) Validate() error {
	if !studioID(r.PrincipalID) || !studioID(r.SessionID) || !studioID(r.RoundID) ||
		!studioID(r.MessageID) || !studioID(r.SourceHandle) || !studioID(r.IdempotencyKey) || len(r.IdempotencyKey) > 128 {
		return core.NewError(core.CodeInvalidArgument, "bounded shared organization publication required")
	}
	return nil
}

func (s *Store) SubmitRPSharedOrganizationNoticePublish(ctx context.Context, r RPSharedOrganizationNoticePublishRequest) (RPSharedRound, error) {
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
		if oldKind != "information_organization_publish" || oldKey != r.IdempotencyKey {
			return empty, core.NewError(core.CodeBranchConflict, "shared-round participant already submitted an action")
		}
		if oldHash != hash {
			return empty, core.NewError(core.CodeIdempotencyMismatch, "shared organization publication retry differs")
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
	sourceID, err := resolveRPNoticePublicationSource(ctx, tx.conn, session, "organization_announcement", r.SourceHandle)
	if err != nil {
		return empty, err
	}
	// Check the selected Person before parsing a private Career source. A
	// different participant cannot use error details to probe layoff facts.
	var actor string
	if err := tx.conn.QueryRowContext(ctx, `SELECT actor_id FROM events WHERE event_id=? AND instance_id=? AND branch_id=? AND event_type='RPCareerFactRecorded'`,
		sourceID, row.Instance, row.Branch).Scan(&actor); err != nil {
		return empty, classifyMissing(err, "own scoped Career publication source")
	}
	var speaker string
	if err := tx.conn.QueryRowContext(ctx, `SELECT agent_id FROM agent_profiles WHERE principal_id=? AND instance_id=? AND branch_id=? AND status='active'`,
		actor, row.Instance, row.Branch).Scan(&speaker); err != nil {
		return empty, classifyMissing(err, "layoff manager Person")
	}
	var entity string
	if err := tx.conn.QueryRowContext(ctx, `SELECT entity_id FROM rp_shared_round_participants WHERE round_id=? AND session_id=?`, row.ID, r.SessionID).Scan(&entity); err != nil {
		return empty, err
	}
	if entity != speaker {
		return empty, core.NewError(core.CodeNotFound, "own scoped Career publication source not found")
	}
	fact, sourceActor, sourceSequence, _, err := rpOrganizationLayoffSource(ctx, tx.conn, row.Instance, row.Branch, sourceID)
	if err != nil {
		return empty, err
	}
	if sourceActor != actor || sourceSequence >= row.BaselineHead+1 {
		return empty, core.NewError(core.CodeProjectionDiverged, "Career source must precede publication")
	}
	sourceBinding := core.CareerBinding{PrincipalID: actor, InstanceID: row.Instance, BranchID: row.Branch,
		ExpectedHead: row.BaselineHead, IdempotencyKey: "shared_action_" + row.ID}
	if err := authorizeCareerManager(ctx, tx.conn, sourceBinding, fact.OrganizationID); err != nil {
		return empty, err
	}
	if err := validateRPBinding(ctx, tx.conn, row.Instance, row.Branch, speaker); err != nil {
		return empty, err
	}
	sourceKey, err := core.HashJSON([]string{actor, sourceBinding.IdempotencyKey})
	if err != nil {
		return empty, err
	}
	var reserved int
	if err := tx.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM commands WHERE instance_id=? AND branch_id=? AND command_type='PublishRPOrganizationNotice' AND idempotency_key=?`,
		row.Instance, row.Branch, sourceKey).Scan(&reserved); err != nil {
		return empty, err
	}
	if reserved != 0 {
		return empty, core.NewError(core.CodeBranchConflict, "organization publication child key already has an accepted owner")
	}
	var duplicate int
	if err := tx.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPInformationSent' AND json_extract(payload,'$.message_id')=?`,
		row.Instance, row.Branch, r.MessageID).Scan(&duplicate); err != nil {
		return empty, err
	}
	if duplicate != 0 {
		return empty, core.NewError(core.CodeBranchConflict, "message ID already used")
	}
	child := RPOrganizationNoticePublishRequest{Binding: sourceBinding, MessageID: r.MessageID, CareerEventID: sourceID,
		SpeakerID: speaker, ControllerSessionID: r.SessionID, ControllerPrincipalID: r.PrincipalID}
	childJSON, err := core.CanonicalJSON(child)
	if err != nil {
		return empty, err
	}
	if _, err := tx.conn.ExecContext(ctx, `INSERT INTO rp_shared_round_actions
	 (round_id,session_id,submission_key,request_hash,request_json,submitted_at_utc,action_kind)
	 VALUES (?,?,?,?,?,?,'information_organization_publish')`, row.ID, r.SessionID, r.IdempotencyKey, hash,
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

func (s *Store) advanceRPSharedOrganizationNoticePublish(ctx context.Context, r RPSharedRoundAdvanceRequest, row rpSharedRoundRow) (RPSharedRound, error) {
	if s.afterRPSharedActionStage != nil {
		if err := s.afterRPSharedActionStage("selection_committed"); err != nil {
			return RPSharedRound{}, err
		}
	}
	var raw string
	if err := s.db.QueryRowContext(ctx, `SELECT request_json FROM rp_shared_round_actions WHERE round_id=? AND session_id=? AND action_kind='information_organization_publish'`,
		row.ID, row.SelectedSession).Scan(&raw); err != nil {
		return RPSharedRound{}, classifyMissing(err, "selected shared organization publication")
	}
	var child RPOrganizationNoticePublishRequest
	if err := json.Unmarshal([]byte(raw), &child); err != nil {
		return RPSharedRound{}, core.WrapError(core.CodeProjectionDiverged, "decode selected shared organization publication", err)
	}
	if child.ControllerSessionID != row.SelectedSession || child.Binding.InstanceID != row.Instance || child.Binding.BranchID != row.Branch ||
		child.Binding.ExpectedHead != row.BaselineHead || child.Binding.IdempotencyKey != "shared_action_"+row.ID {
		return RPSharedRound{}, core.NewError(core.CodeProjectionDiverged, "selected organization publication binding differs")
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
	var acceptedEventID string
	var acceptedSequence int64
	if head != row.BaselineHead {
		var requestHash, actor, eventType string
		var expectedHead int64
		err := check.conn.QueryRowContext(ctx, `SELECT c.request_hash,c.expected_head,e.event_id,e.event_sequence,e.actor_id,e.event_type
		 FROM commands c JOIN event_batches b ON b.command_id=c.command_id JOIN events e ON e.batch_id=b.batch_id
		 WHERE c.instance_id=? AND c.branch_id=? AND c.command_type='PublishRPOrganizationNotice'
		 AND c.idempotency_key=? AND c.status='committed'`, row.Instance, row.Branch, key).
			Scan(&requestHash, &expectedHead, &acceptedEventID, &acceptedSequence, &actor, &eventType)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			check.Rollback(ctx)
			return RPSharedRound{}, err
		}
		if errors.Is(err, sql.ErrNoRows) {
			if _, err := check.conn.ExecContext(ctx, `UPDATE rp_shared_rounds SET status='stale' WHERE round_id=? AND status='advancing' AND settlement_kind='information' AND selected_action_kind='information_organization_publish' AND selected_session_id=?`, row.ID, row.SelectedSession); err != nil {
				check.Rollback(ctx)
				return RPSharedRound{}, err
			}
			if err := check.Commit(ctx); err != nil {
				return RPSharedRound{}, err
			}
			return RPSharedRound{}, core.NewError(core.CodeBranchConflict, "shared organization publication baseline changed before typed acceptance")
		}
		pinnedHash, err := core.HashJSON(child)
		if err != nil {
			check.Rollback(ctx)
			return RPSharedRound{}, err
		}
		if requestHash != pinnedHash || expectedHead != row.BaselineHead || acceptedSequence != row.BaselineHead+1 ||
			actor != child.Binding.PrincipalID || eventType != "RPInformationSent" {
			check.Rollback(ctx)
			return RPSharedRound{}, core.NewError(core.CodeProjectionDiverged, "accepted organization publication differs from selected child")
		}
		if _, _, err := rpInformationExpected(ctx, check.conn, row.Instance, row.Branch, head); err != nil {
			check.Rollback(ctx)
			return RPSharedRound{}, err
		}
	}
	check.Rollback(ctx)
	if acceptedEventID == "" {
		accepted, err := s.PublishRPOrganizationNotice(ctx, child)
		if err != nil {
			return RPSharedRound{}, err
		}
		acceptedEventID, acceptedSequence = accepted.EventID, accepted.EventSequence
		if s.afterRPSharedActionStage != nil {
			if err := s.afterRPSharedActionStage("information_organization_publish_event_committed"); err != nil {
				return RPSharedRound{}, err
			}
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
	if current.Status != "advancing" || current.SettlementKind != "information" || current.SelectedActionKind != "information_organization_publish" ||
		current.SelectedSession != child.ControllerSessionID || acceptedSequence != row.BaselineHead+1 {
		return RPSharedRound{}, core.NewError(core.CodeBranchConflict, "shared organization publication settlement changed")
	}
	if _, err := settle.conn.ExecContext(ctx, `UPDATE rp_shared_rounds SET status='settled',completion_event_id=?,settled_sequence=?,settled_at_utc=? WHERE round_id=? AND status='advancing'`,
		acceptedEventID, acceptedSequence, s.now().UTC().Format(time.RFC3339Nano), row.ID); err != nil {
		return RPSharedRound{}, err
	}
	current.Status, current.CompletionEvent, current.SettledSequence = "settled", acceptedEventID, acceptedSequence
	out, err := rpSharedRoundViewForParticipant(ctx, settle.conn, current, r.SessionID)
	if err != nil {
		return RPSharedRound{}, err
	}
	if err := settle.Commit(ctx); err != nil {
		return RPSharedRound{}, err
	}
	return out, nil
}
