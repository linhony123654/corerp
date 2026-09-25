package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"corerp.local/backend/internal/core"
)

// A sleep proposal is application state until the full F3 round selects it.
// The actor's existing sleep command remains the sole Event authority.
type RPSharedSleepRequest struct {
	PrincipalID    string `json:"principal_id"`
	SessionID      string `json:"session_id"`
	RoundID        string `json:"round_id"`
	Action         string `json:"action"`
	IdempotencyKey string `json:"idempotency_key"`
}

func rpSharedSleepKind(action string) (string, error) {
	switch action {
	case "start":
		return "sleep_start", nil
	case "end":
		return "sleep_end", nil
	default:
		return "", core.NewError(core.CodeInvalidArgument, "shared sleep action must be start or end")
	}
}

func (s *Store) SubmitRPSharedSleep(ctx context.Context, r RPSharedSleepRequest) (RPSharedRound, error) {
	var empty RPSharedRound
	if !studioID(r.PrincipalID) || !studioID(r.SessionID) || !studioID(r.RoundID) || !studioID(r.IdempotencyKey) || len(r.IdempotencyKey) > 128 {
		return empty, core.NewError(core.CodeInvalidArgument, "bounded shared-sleep identity/key required")
	}
	kind, err := rpSharedSleepKind(r.Action)
	if err != nil {
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
		if oldKind != kind || oldKey != r.IdempotencyKey {
			return empty, core.NewError(core.CodeBranchConflict, "shared-round participant already submitted an action")
		}
		if oldHash != hash {
			return empty, core.NewError(core.CodeIdempotencyMismatch, "shared-sleep retry differs")
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
	var at, place, placeKind, entity string
	if err := tx.conn.QueryRowContext(ctx, `SELECT b.head_sequence,c.current_world_time,s.observation_cursor,p.place_id,l.place_kind,s.controlled_entity_id
		FROM branches b JOIN world_clocks c ON c.instance_id=b.instance_id AND c.branch_id=b.branch_id
		JOIN rp_sessions s ON s.session_id=? JOIN agent_positions p ON p.agent_id=s.controlled_entity_id
		JOIN agent_places l ON l.place_id=p.place_id WHERE b.instance_id=? AND b.branch_id=?`,
		r.SessionID, row.Instance, row.Branch).Scan(&head, &at, &cursor, &place, &placeKind, &entity); err != nil {
		return empty, err
	}
	if head != row.BaselineHead || at != row.BaselineTime {
		return empty, staleRPSharedRound(ctx, tx, row.ID)
	}
	if cursor != head {
		return empty, core.NewError(core.CodeBranchConflict, "participant has not observed shared baseline")
	}
	open, started, _, err := openRPSleep(ctx, tx.conn, row.Instance, row.Branch, entity)
	if err != nil {
		return empty, err
	}
	if kind == "sleep_start" {
		if placeKind != "home" || open != "" {
			return empty, core.NewError(core.CodeBranchConflict, "sleep start requires home and no open interval")
		}
	} else {
		if open == "" {
			return empty, core.NewError(core.CodeNotFound, "open actor sleep interval not found")
		}
		begin, parseErr := time.Parse(time.RFC3339Nano, started)
		end, endErr := time.Parse(time.RFC3339Nano, at)
		if parseErr != nil || endErr != nil || !end.After(begin) {
			return empty, core.NewError(core.CodeBranchConflict, "sleep end requires positive elapsed world time")
		}
	}
	child := RPSleepRequest{Binding: core.CareerBinding{PrincipalID: r.PrincipalID, InstanceID: row.Instance,
		BranchID: row.Branch, ExpectedHead: row.BaselineHead, IdempotencyKey: "shared_action_" + row.ID},
		SessionID: r.SessionID}
	childJSON, err := core.CanonicalJSON(child)
	if err != nil {
		return empty, err
	}
	if _, err := tx.conn.ExecContext(ctx, `INSERT INTO rp_shared_round_actions
		(round_id,session_id,submission_key,request_hash,request_json,submitted_at_utc,action_kind)
		VALUES (?,?,?,?,?,?,?)`, row.ID, r.SessionID, r.IdempotencyKey, hash, string(childJSON),
		s.now().UTC().Format(time.RFC3339Nano), kind); err != nil {
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

func (s *Store) advanceRPSharedSleep(ctx context.Context, r RPSharedRoundAdvanceRequest, row rpSharedRoundRow) (RPSharedRound, error) {
	if s.afterRPSharedActionStage != nil {
		if err := s.afterRPSharedActionStage("selection_committed"); err != nil {
			return RPSharedRound{}, err
		}
	}
	var raw string
	if err := s.db.QueryRowContext(ctx, `SELECT request_json FROM rp_shared_round_actions WHERE round_id=? AND session_id=? AND action_kind=?`,
		row.ID, row.SelectedSession, row.SelectedActionKind).Scan(&raw); err != nil {
		return RPSharedRound{}, classifyMissing(err, "selected shared sleep")
	}
	var child RPSleepRequest
	if err := json.Unmarshal([]byte(raw), &child); err != nil {
		return RPSharedRound{}, core.WrapError(core.CodeProjectionDiverged, "decode selected shared sleep", err)
	}
	if child.SessionID != row.SelectedSession || child.Binding.InstanceID != row.Instance || child.Binding.BranchID != row.Branch ||
		child.Binding.ExpectedHead != row.BaselineHead || child.Binding.IdempotencyKey != "shared_action_"+row.ID {
		return RPSharedRound{}, core.NewError(core.CodeProjectionDiverged, "selected shared sleep binding differs")
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
	commandType := "StartRPSleep"
	if row.SelectedActionKind == "sleep_end" {
		commandType = "EndRPSleep"
	}
	key, err := core.HashJSON([]string{child.Binding.PrincipalID, child.Binding.IdempotencyKey})
	if err != nil {
		check.Rollback(ctx)
		return RPSharedRound{}, err
	}
	if head != row.BaselineHead {
		var accepted int
		if err := check.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM commands WHERE instance_id=? AND branch_id=? AND command_type=? AND idempotency_key=? AND status='committed'`,
			row.Instance, row.Branch, commandType, key).Scan(&accepted); err != nil {
			check.Rollback(ctx)
			return RPSharedRound{}, err
		}
		if accepted == 0 {
			if _, err := check.conn.ExecContext(ctx, `UPDATE rp_shared_rounds SET status='stale' WHERE round_id=? AND status='advancing' AND settlement_kind='health' AND selected_session_id=?`, row.ID, row.SelectedSession); err != nil {
				check.Rollback(ctx)
				return RPSharedRound{}, err
			}
			if err := check.Commit(ctx); err != nil {
				return RPSharedRound{}, err
			}
			return RPSharedRound{}, core.NewError(core.CodeBranchConflict, "shared sleep baseline changed before typed acceptance")
		}
	}
	check.Rollback(ctx)
	var eventID string
	var sequence int64
	if row.SelectedActionKind == "sleep_start" {
		accepted, err := s.StartRPSleep(ctx, child)
		if err != nil {
			return RPSharedRound{}, err
		}
		eventID, sequence = accepted.EventID, accepted.EventSequence
	} else {
		accepted, err := s.EndRPSleep(ctx, child)
		if err != nil {
			return RPSharedRound{}, err
		}
		eventID, sequence = accepted.EventID, accepted.EventSequence
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
	if current.Status != "advancing" || current.SettlementKind != "health" || current.SelectedActionKind != row.SelectedActionKind ||
		current.SelectedSession != child.SessionID || sequence != row.BaselineHead+1 {
		return RPSharedRound{}, core.NewError(core.CodeBranchConflict, "shared sleep settlement changed")
	}
	if _, err := settle.conn.ExecContext(ctx, `UPDATE rp_shared_rounds SET status='settled',completion_event_id=?,settled_sequence=?,settled_at_utc=? WHERE round_id=? AND status='advancing'`,
		eventID, sequence, s.now().UTC().Format(time.RFC3339Nano), row.ID); err != nil {
		return RPSharedRound{}, err
	}
	current.Status, current.CompletionEvent, current.SettledSequence = "settled", eventID, sequence
	out, err := rpSharedRoundViewForParticipant(ctx, settle.conn, current, r.SessionID)
	if err != nil {
		return RPSharedRound{}, err
	}
	if err := settle.Commit(ctx); err != nil {
		return RPSharedRound{}, err
	}
	return out, nil
}
