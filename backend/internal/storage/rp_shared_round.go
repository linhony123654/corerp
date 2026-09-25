package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"corerp.local/backend/internal/core"
)

// Shared rounds coordinate decisions; the Human's one RPWaitCompleted Event
// remains the only authority for advancing world time.
type RPSharedRoundOpenRequest struct {
	Binding            core.CareerBinding `json:"binding"`
	HumanSessionID     string             `json:"human_session_id"`
	ExternalSessionIDs []string           `json:"external_session_ids"`
}

type RPSharedWaitRequest struct {
	PrincipalID      string `json:"principal_id"`
	SessionID        string `json:"session_id"`
	RoundID          string `json:"round_id"`
	HorizonWorldTime string `json:"horizon_world_time"`
	IdempotencyKey   string `json:"idempotency_key"`
}

// A submitted speech is private application state until every participant,
// including the Human, has answered the same observed decision window.
type RPSharedSpeechRequest struct {
	PrincipalID     string `json:"principal_id"`
	SessionID       string `json:"session_id"`
	RoundID         string `json:"round_id"`
	Text            string `json:"text"`
	SpeechAct       string `json:"speech_act,omitempty"`
	DeliveryChannel string `json:"delivery_channel,omitempty"`
	IntroduceSelf   bool   `json:"introduce_self,omitempty"`
	IdempotencyKey  string `json:"idempotency_key"`
}

// A move proposal pins the observed origin and destination. The ordinary
// MoveRP owner checks the route again when the proposal is selected.
type RPSharedMoveRequest struct {
	PrincipalID    string `json:"principal_id"`
	SessionID      string `json:"session_id"`
	RoundID        string `json:"round_id"`
	FromPlaceID    string `json:"from_place_id"`
	ToPlaceID      string `json:"to_place_id"`
	IdempotencyKey string `json:"idempotency_key"`
}

type RPSharedRoundReadRequest struct {
	PrincipalID string `json:"principal_id"`
	SessionID   string `json:"session_id"`
	RoundID     string `json:"round_id"`
}

type RPSharedRoundAdvanceRequest struct {
	RPSharedRoundReadRequest
	Budget int `json:"budget"`
}

type RPSharedRound struct {
	RoundID           string `json:"round_id"`
	Status            string `json:"status"`
	BaselineWorldTime string `json:"baseline_world_time"`
	CurrentWorldTime  string `json:"current_world_time"`
	Required          int    `json:"required"`
	Submitted         int    `json:"submitted"`
	EventSequence     int64  `json:"event_sequence,omitempty"`
	PendingDue        int64  `json:"pending_due,omitempty"`
	Replayed          bool   `json:"replayed,omitempty"`
	OwnDisposition    string `json:"own_disposition,omitempty"`
}

type rpSharedRoundRow struct {
	ID, Instance, Branch, HumanSession, BaselineTime, Status, AdvanceTarget, WaitKey, CompletionEvent, SettlementKind, SelectedSession, SelectedActionKind string
	BaselineHead, SettledSequence                                                                                                                          int64
}

// Fresh typed actions must not change the pinned baseline outside the
// selected child. Call this after exact committed-key replay checks.
func requireNoActiveRPSharedRound(ctx context.Context, q rpQueryer, instanceID, branchID string) error {
	var count int
	if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_shared_rounds WHERE instance_id=? AND branch_id=? AND status IN ('open','advancing')`, instanceID, branchID).Scan(&count); err != nil {
		return err
	}
	if count != 0 {
		return core.NewError(core.CodeCommandInProgress, "shared decision window owns new RP actions")
	}
	return nil
}

// A turn may adopt only the selected round speech that was already accepted
// by its typed command owner. This is not authority for a fresh turn/speech.
func adoptedRPSharedSpeech(ctx context.Context, conn *sql.Conn, session RPSession, request core.RPSpeechRequest, requestHash string) (bool, error) {
	var roundID, raw string
	var baseline int64
	err := conn.QueryRowContext(ctx, `SELECT r.round_id,r.baseline_head,a.request_json FROM rp_shared_rounds r JOIN rp_shared_round_actions a ON a.round_id=r.round_id AND a.session_id=r.selected_session_id WHERE r.instance_id=? AND r.branch_id=? AND r.status='advancing' AND r.settlement_kind='speech' AND r.selected_action_kind='speech' AND a.action_kind='speech' AND r.selected_session_id=?`, session.InstanceID, session.BranchID, session.SessionID).Scan(&roundID, &baseline, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if request.IdempotencyKey != "shared_action_"+roundID || request.ExpectedCursor != baseline {
		return false, core.NewError(core.CodeUnauthorized, "turn differs from selected shared action")
	}
	var pinned core.RPSpeechRequest
	if err := json.Unmarshal([]byte(raw), &pinned); err != nil {
		return false, core.WrapError(core.CodeProjectionDiverged, "decode selected shared action", err)
	}
	pinnedHash, err := core.HashJSON(pinned)
	if err != nil {
		return false, err
	}
	if pinnedHash != requestHash {
		return false, core.NewError(core.CodeUnauthorized, "turn payload differs from selected shared action")
	}
	var commandHash string
	var expectedHead, eventSequence int64
	commandKey := "rp_speech:" + session.SessionID + ":" + request.IdempotencyKey
	err = conn.QueryRowContext(ctx, `SELECT c.request_hash,c.expected_head,e.event_sequence FROM commands c JOIN event_batches b ON b.command_id=c.command_id JOIN events e ON e.batch_id=b.batch_id WHERE c.instance_id=? AND c.branch_id=? AND c.command_type='RPSpeak' AND c.idempotency_key=? AND c.status='committed' AND e.event_type='RPSpeechAccepted' AND e.actor_id=?`, session.InstanceID, session.BranchID, commandKey, session.ControlledEntityID).Scan(&commandHash, &expectedHead, &eventSequence)
	if errors.Is(err, sql.ErrNoRows) {
		return false, core.NewError(core.CodeCommandInProgress, "selected shared speech is not yet accepted")
	}
	if err != nil {
		return false, err
	}
	if commandHash != requestHash || expectedHead != baseline || eventSequence != baseline+1 {
		return false, core.NewError(core.CodeProjectionDiverged, "selected shared speech source differs")
	}
	return true, nil
}

// Only the pinned move request may cross an active decision window. A caller
// cannot steal the derived child key to alter its destination after selection.
func selectedRPSharedMove(ctx context.Context, conn *sql.Conn, session RPSession, request core.RPMoveRequest, requestHash string) (bool, error) {
	var roundID, raw string
	var baseline int64
	err := conn.QueryRowContext(ctx, `SELECT r.round_id,r.baseline_head,a.request_json FROM rp_shared_rounds r JOIN rp_shared_round_actions a ON a.round_id=r.round_id AND a.session_id=r.selected_session_id WHERE r.instance_id=? AND r.branch_id=? AND r.status='advancing' AND r.settlement_kind='speech' AND r.selected_action_kind='move' AND a.action_kind='move' AND r.selected_session_id=?`, session.InstanceID, session.BranchID, session.SessionID).Scan(&roundID, &baseline, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if request.IdempotencyKey != "shared_action_"+roundID || request.ExpectedCursor != baseline {
		return false, core.NewError(core.CodeUnauthorized, "move differs from selected shared action")
	}
	var pinned core.RPMoveRequest
	if err := json.Unmarshal([]byte(raw), &pinned); err != nil {
		return false, core.WrapError(core.CodeProjectionDiverged, "decode selected shared move", err)
	}
	pinnedHash, err := core.HashJSON(pinned)
	if err != nil {
		return false, err
	}
	if pinnedHash != requestHash {
		return false, core.NewError(core.CodeUnauthorized, "move payload differs from selected shared action")
	}
	return true, nil
}

func readRPSharedRoundRow(ctx context.Context, q rpQueryer, roundID string) (rpSharedRoundRow, error) {
	var row rpSharedRoundRow
	var sequence sql.NullInt64
	err := q.QueryRowContext(ctx, `SELECT round_id,instance_id,branch_id,human_session_id,baseline_world_time,baseline_head,status,advance_target,wait_key,COALESCE(completion_event_id,''),settled_sequence,settlement_kind,selected_session_id,selected_action_kind FROM rp_shared_rounds WHERE round_id=?`, roundID).Scan(&row.ID, &row.Instance, &row.Branch, &row.HumanSession, &row.BaselineTime, &row.BaselineHead, &row.Status, &row.AdvanceTarget, &row.WaitKey, &row.CompletionEvent, &sequence, &row.SettlementKind, &row.SelectedSession, &row.SelectedActionKind)
	if err != nil {
		return row, classifyMissing(err, "RP shared round")
	}
	row.SettledSequence = sequence.Int64
	return row, nil
}

func rpSharedRoundView(ctx context.Context, q rpQueryer, row rpSharedRoundRow) (RPSharedRound, error) {
	var view RPSharedRound
	view.RoundID, view.Status, view.BaselineWorldTime, view.EventSequence = row.ID, row.Status, row.BaselineTime, row.SettledSequence
	if err := q.QueryRowContext(ctx, `SELECT COUNT(*),COUNT(NULLIF(horizon_world_time,''))+(SELECT COUNT(*) FROM rp_shared_round_actions WHERE round_id=?) FROM rp_shared_round_participants WHERE round_id=?`, row.ID, row.ID).Scan(&view.Required, &view.Submitted); err != nil {
		return RPSharedRound{}, err
	}
	switch row.Status {
	case "settled":
		// A terminal receipt must not become an observation channel for a
		// controller that was released after its accepted round settled.
		eventType := "RPWaitCompleted"
		if row.SettlementKind == "speech" {
			eventType = "RPSpeechAccepted"
			if row.SelectedActionKind == "move" {
				eventType = "RPPlayerMoved"
			}
		}
		if err := q.QueryRowContext(ctx, `SELECT world_time FROM events WHERE event_id=? AND instance_id=? AND branch_id=? AND event_sequence=? AND event_type=?`, row.CompletionEvent, row.Instance, row.Branch, row.SettledSequence, eventType).Scan(&view.CurrentWorldTime); err != nil {
			return RPSharedRound{}, core.WrapError(core.CodeProjectionDiverged, "settled shared round lacks its typed Event", err)
		}
	case "stale":
		view.CurrentWorldTime = row.BaselineTime
	default:
		if err := q.QueryRowContext(ctx, `SELECT current_world_time FROM world_clocks WHERE instance_id=? AND branch_id=?`, row.Instance, row.Branch).Scan(&view.CurrentWorldTime); err != nil {
			return RPSharedRound{}, err
		}
	}
	return view, nil
}

func rpSharedRoundViewForParticipant(ctx context.Context, q rpQueryer, row rpSharedRoundRow, sessionID string) (RPSharedRound, error) {
	view, err := rpSharedRoundView(ctx, q, row)
	if err != nil {
		return view, err
	}
	var waitKey string
	if err := q.QueryRowContext(ctx, `SELECT submission_key FROM rp_shared_round_participants WHERE round_id=? AND session_id=?`, row.ID, sessionID).Scan(&waitKey); err != nil {
		return RPSharedRound{}, err
	}
	var actionCount int
	if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_shared_round_actions WHERE round_id=? AND session_id=?`, row.ID, sessionID).Scan(&actionCount); err != nil {
		return RPSharedRound{}, err
	}
	if waitKey == "" && actionCount == 0 {
		return view, nil
	}
	view.OwnDisposition = "submitted"
	if row.Status == "settled" {
		if row.SettlementKind == "wait" && waitKey != "" {
			view.OwnDisposition = "wait_completed"
		} else if row.SelectedSession == sessionID && actionCount == 1 {
			view.OwnDisposition = "action_accepted"
		} else {
			view.OwnDisposition = "deferred_no_effect"
		}
	}
	return view, nil
}

func (s *Store) OpenRPSharedRoundLocal(ctx context.Context, r RPSharedRoundOpenRequest) (RPSharedRound, error) {
	var empty RPSharedRound
	if err := r.Binding.Validate(); err != nil {
		return empty, err
	}
	if !studioID(r.HumanSessionID) || len(r.ExternalSessionIDs) < 1 || len(r.ExternalSessionIDs) > 2 {
		return empty, core.NewError(core.CodeInvalidArgument, "one Human and one or two external sessions required")
	}
	for _, id := range r.ExternalSessionIDs {
		if !studioID(id) || id == r.HumanSessionID {
			return empty, core.NewError(core.CodeInvalidArgument, "distinct bounded participant sessions required")
		}
	}
	hash, err := core.HashJSON(r)
	if err != nil {
		return empty, err
	}
	idHash, err := core.HashJSON([]string{r.Binding.InstanceID, r.Binding.BranchID, r.Binding.PrincipalID, r.Binding.IdempotencyKey})
	if err != nil {
		return empty, err
	}
	roundID := "rpr_" + idHash[7:]
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return empty, err
	}
	defer tx.Rollback(ctx)
	var role string
	if err := tx.conn.QueryRowContext(ctx, `SELECT principal_type FROM principals WHERE principal_id=? AND status='active'`, r.Binding.PrincipalID).Scan(&role); err != nil {
		return empty, classifyMissing(err, "active shared-round operator")
	}
	if role != "operator" {
		return empty, core.NewError(core.CodeUnauthorized, "shared round requires local operator")
	}
	var oldHash string
	err = tx.conn.QueryRowContext(ctx, `SELECT request_hash FROM rp_shared_rounds WHERE round_id=?`, roundID).Scan(&oldHash)
	if err == nil {
		if oldHash != hash {
			return empty, core.NewError(core.CodeIdempotencyMismatch, "shared-round open key differs")
		}
		row, err := readRPSharedRoundRow(ctx, tx.conn, roundID)
		if err != nil {
			return empty, err
		}
		out, err := rpSharedRoundView(ctx, tx.conn, row)
		out.Replayed = true
		return out, err
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return empty, err
	}
	var active int
	if err := tx.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_shared_rounds WHERE instance_id=? AND branch_id=? AND status IN ('open','advancing')`, r.Binding.InstanceID, r.Binding.BranchID).Scan(&active); err != nil {
		return empty, err
	}
	if active != 0 {
		return empty, core.NewError(core.CodeCommandInProgress, "another shared round is active")
	}
	var head int64
	var at string
	if err := tx.conn.QueryRowContext(ctx, `SELECT b.head_sequence,c.current_world_time FROM branches b JOIN world_clocks c ON c.instance_id=b.instance_id AND c.branch_id=b.branch_id WHERE b.instance_id=? AND b.branch_id=?`, r.Binding.InstanceID, r.Binding.BranchID).Scan(&head, &at); err != nil {
		return empty, classifyMissing(err, "shared-round world")
	}
	if head != r.Binding.ExpectedHead {
		return empty, core.NewError(core.CodeBranchConflict, "shared-round baseline head changed")
	}
	var pending int
	if err := tx.conn.QueryRowContext(ctx, `SELECT (SELECT COUNT(*) FROM rp_turn_runs t JOIN rp_sessions s ON s.session_id=t.session_id WHERE s.instance_id=? AND s.branch_id=? AND t.status<>'settled')+(SELECT COUNT(*) FROM rp_wait_intents w JOIN rp_sessions s ON s.session_id=w.session_id WHERE s.instance_id=? AND s.branch_id=? AND w.status='pending')`, r.Binding.InstanceID, r.Binding.BranchID, r.Binding.InstanceID, r.Binding.BranchID).Scan(&pending); err != nil {
		return empty, err
	}
	if pending != 0 {
		return empty, core.NewError(core.CodeCommandInProgress, "settle RP work before shared round")
	}
	if err := tx.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_interactions i JOIN rp_sessions s ON s.session_id=i.session_id WHERE s.instance_id=? AND s.branch_id=? AND i.status IN ('open','paused')`, r.Binding.InstanceID, r.Binding.BranchID).Scan(&pending); err != nil {
		return empty, err
	}
	if pending != 0 {
		return empty, core.NewError(core.CodeCommandInProgress, "settle RP interaction before shared round")
	}
	var assigned int
	if err := tx.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_controller_authorities WHERE instance_id=? AND branch_id=? AND status='active'`, r.Binding.InstanceID, r.Binding.BranchID).Scan(&assigned); err != nil {
		return empty, err
	}
	if assigned != len(r.ExternalSessionIDs) {
		return empty, core.NewError(core.CodeBranchConflict, "shared round must include every external controller")
	}
	participants := make([]RPSession, 0, 1+assigned)
	seenEntity, seenPrincipal := map[string]bool{}, map[string]bool{}
	for i, id := range append([]string{r.HumanSessionID}, r.ExternalSessionIDs...) {
		var principal, kind string
		err := tx.conn.QueryRowContext(ctx, `SELECT s.principal_id,p.principal_type FROM rp_sessions s JOIN principals p ON p.principal_id=s.principal_id AND p.status='active' WHERE s.session_id=?`, id).Scan(&principal, &kind)
		if err != nil {
			return empty, classifyMissing(err, "active shared-round participant")
		}
		wantKind := "service"
		if i == 0 {
			wantKind = "player"
		}
		if kind != wantKind {
			return empty, core.NewError(core.CodeUnauthorized, "shared-round participant role differs")
		}
		session, err := loadRPSession(ctx, tx.conn, principal, id)
		if err != nil {
			return empty, err
		}
		if err := authorizeRPControl(ctx, tx.conn, principal, session.InstanceID, session.BranchID, session.ControlledEntityID); err != nil {
			return empty, err
		}
		if session.Status != "active" || session.InstanceID != r.Binding.InstanceID || session.BranchID != r.Binding.BranchID || session.ObservationCursor != head || seenEntity[session.ControlledEntityID] || seenPrincipal[principal] {
			return empty, core.NewError(core.CodeBranchConflict, "shared-round participant binding or observation changed")
		}
		if i > 0 && (session.ControlGeneration == 0 || session.ControllerInstanceID == "") {
			return empty, core.NewError(core.CodeUnauthorized, "external participant lacks assigned generation")
		}
		seenEntity[session.ControlledEntityID], seenPrincipal[principal] = true, true
		participants = append(participants, session)
	}
	created := s.now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.conn.ExecContext(ctx, `INSERT INTO rp_shared_rounds(round_id,instance_id,branch_id,operator_principal_id,idempotency_key,request_hash,human_session_id,baseline_world_time,baseline_head,status,created_at_utc) VALUES (?,?,?,?,?,?,?,?,?,'open',?)`, roundID, r.Binding.InstanceID, r.Binding.BranchID, r.Binding.PrincipalID, r.Binding.IdempotencyKey, hash, r.HumanSessionID, at, head, created); err != nil {
		return empty, core.WrapError(core.CodeStorageFailure, "open RP shared round", err)
	}
	for i, session := range participants {
		role := "external"
		if i == 0 {
			role = "human"
		}
		principal := r.Binding.PrincipalID
		if err := tx.conn.QueryRowContext(ctx, `SELECT principal_id FROM rp_sessions WHERE session_id=?`, session.SessionID).Scan(&principal); err != nil {
			return empty, err
		}
		childKey := "shared_action_" + roundID
		for _, operation := range []string{"dialogue", "move"} {
			if err := checkRPRequestRetirement(ctx, tx.conn, principal, operation, session.SessionID, childKey); err != nil {
				return empty, err
			}
		}
		var accepted int
		if err := tx.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM commands WHERE instance_id=? AND branch_id=? AND ((command_type='RPSpeak' AND idempotency_key=?) OR (command_type='RPPlayerMove' AND idempotency_key=?))`, session.InstanceID, session.BranchID, "rp_speech:"+session.SessionID+":"+childKey, "rp_move:"+session.SessionID+":"+childKey).Scan(&accepted); err != nil {
			return empty, err
		}
		if accepted != 0 {
			return empty, core.NewError(core.CodeBranchConflict, "shared-round action child key already has an accepted owner")
		}
		if err := tx.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_turn_runs WHERE session_id=? AND idempotency_key=?`, session.SessionID, childKey).Scan(&accepted); err != nil {
			return empty, err
		}
		if accepted != 0 {
			return empty, core.NewError(core.CodeBranchConflict, "shared-round turn child key already has an accepted owner")
		}
		if i == 0 {
			waitKey := "shared_" + roundID
			if err := checkRPRequestRetirement(ctx, tx.conn, principal, "wait", session.SessionID, waitKey); err != nil {
				return empty, err
			}
			if err := tx.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_wait_intents WHERE session_id=? AND idempotency_key=?`, session.SessionID, waitKey).Scan(&accepted); err != nil {
				return empty, err
			}
			if accepted != 0 {
				return empty, core.NewError(core.CodeBranchConflict, "shared-round wait child key already has an accepted owner")
			}
		}
		if _, err := tx.conn.ExecContext(ctx, `INSERT INTO rp_shared_round_participants(round_id,session_id,principal_id,entity_id,role,control_generation,controller_instance_id) VALUES (?,?,?,?,?,?,?)`, roundID, session.SessionID, principal, session.ControlledEntityID, role, session.ControlGeneration, session.ControllerInstanceID); err != nil {
			return empty, err
		}
	}
	if s.beforeCommit != nil {
		if err := s.beforeCommit(); err != nil {
			return empty, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return empty, err
	}
	return RPSharedRound{RoundID: roundID, Status: "open", BaselineWorldTime: at, CurrentWorldTime: at, Required: len(participants)}, nil
}

func (s *Store) ReadRPSharedRound(ctx context.Context, r RPSharedRoundReadRequest) (RPSharedRound, error) {
	var empty RPSharedRound
	if !studioID(r.PrincipalID) || !studioID(r.SessionID) || !studioID(r.RoundID) {
		return empty, core.NewError(core.CodeInvalidArgument, "bounded principal/session/round required")
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
	if err := authorizeRPSharedParticipant(ctx, tx.conn, row, r, row.Status == "open" || row.Status == "advancing"); err != nil {
		return empty, err
	}
	return rpSharedRoundViewForParticipant(ctx, tx.conn, row, r.SessionID)
}

func authorizeRPSharedParticipant(ctx context.Context, conn *sql.Conn, row rpSharedRoundRow, r RPSharedRoundReadRequest, requireCurrent bool) error {
	var entity, controller string
	var generation int64
	if err := conn.QueryRowContext(ctx, `SELECT entity_id,control_generation,controller_instance_id FROM rp_shared_round_participants WHERE round_id=? AND session_id=? AND principal_id=?`, row.ID, r.SessionID, r.PrincipalID).Scan(&entity, &generation, &controller); err != nil {
		return classifyMissing(err, "own shared-round membership")
	}
	session, err := loadRPSessionRecord(ctx, conn, r.PrincipalID, r.SessionID)
	if err != nil {
		return err
	}
	if session.InstanceID != row.Instance || session.BranchID != row.Branch || session.ControlledEntityID != entity || session.ControlGeneration != generation || session.ControllerInstanceID != controller {
		return core.NewError(core.CodeBranchConflict, "shared-round controller binding changed")
	}
	if requireCurrent {
		if err := requireCurrentRPSession(ctx, conn, session); err != nil {
			return err
		}
		if err := authorizeRPControl(ctx, conn, r.PrincipalID, session.InstanceID, session.BranchID, session.ControlledEntityID); err != nil {
			return err
		}
		if session.Status != "active" {
			return core.NewError(core.CodeBranchConflict, "shared-round participant session is closed")
		}
	}
	return nil
}

func staleRPSharedRound(ctx context.Context, tx *immediateTx, roundID string) error {
	if _, err := tx.conn.ExecContext(ctx, `UPDATE rp_shared_rounds SET status='stale' WHERE round_id=? AND status='open'`, roundID); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return core.NewError(core.CodeBranchConflict, "shared-round baseline changed; open a new round")
}

func staleRPSharedWaitBeforeAcceptance(ctx context.Context, tx *immediateTx, roundID string) error {
	if _, err := tx.conn.ExecContext(ctx, `UPDATE rp_shared_rounds SET status='stale' WHERE round_id=? AND status='advancing' AND settlement_kind='wait' AND completion_event_id IS NULL`, roundID); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return core.NewError(core.CodeBranchConflict, "shared-round wait child was not accepted at its baseline")
}

func (s *Store) SubmitRPSharedWait(ctx context.Context, r RPSharedWaitRequest) (RPSharedRound, error) {
	var empty RPSharedRound
	if !studioID(r.PrincipalID) || !studioID(r.SessionID) || !studioID(r.RoundID) || !studioID(r.IdempotencyKey) || len(r.IdempotencyKey) > 128 {
		return empty, core.NewError(core.CodeInvalidArgument, "bounded shared-wait identity/key required")
	}
	horizon, err := time.Parse(time.RFC3339, r.HorizonWorldTime)
	if err != nil {
		return empty, core.WrapError(core.CodeInvalidArgument, "shared wait horizon must be RFC3339", err)
	}
	r.HorizonWorldTime = horizon.UTC().Format(time.RFC3339)
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
	var existingKey, existingHash string
	if err := tx.conn.QueryRowContext(ctx, `SELECT submission_key,request_hash FROM rp_shared_round_participants WHERE round_id=? AND session_id=?`, row.ID, r.SessionID).Scan(&existingKey, &existingHash); err != nil {
		return empty, err
	}
	if existingKey != "" {
		if existingKey != r.IdempotencyKey {
			return empty, core.NewError(core.CodeBranchConflict, "shared-round participant already submitted")
		}
		if existingHash != hash {
			return empty, core.NewError(core.CodeIdempotencyMismatch, "shared-wait retry differs")
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
	if row.Status != "open" {
		return empty, core.NewError(core.CodeBranchConflict, "shared round no longer accepts submissions")
	}
	var actionCount int
	if err := tx.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_shared_round_actions WHERE round_id=? AND session_id=?`, row.ID, r.SessionID).Scan(&actionCount); err != nil {
		return empty, err
	}
	if actionCount != 0 {
		return empty, core.NewError(core.CodeBranchConflict, "shared-round participant already submitted an action")
	}
	if err := authorizeRPSharedParticipant(ctx, tx.conn, row, identity, true); err != nil {
		return empty, err
	}
	var head int64
	var at string
	if err := tx.conn.QueryRowContext(ctx, `SELECT b.head_sequence,c.current_world_time FROM branches b JOIN world_clocks c ON c.instance_id=b.instance_id AND c.branch_id=b.branch_id WHERE b.instance_id=? AND b.branch_id=?`, row.Instance, row.Branch).Scan(&head, &at); err != nil {
		return empty, err
	}
	if head != row.BaselineHead || at != row.BaselineTime {
		return empty, staleRPSharedRound(ctx, tx, row.ID)
	}
	var cursor int64
	if err := tx.conn.QueryRowContext(ctx, `SELECT observation_cursor FROM rp_sessions WHERE session_id=?`, r.SessionID).Scan(&cursor); err != nil {
		return empty, err
	}
	if cursor != head {
		return empty, core.NewError(core.CodeBranchConflict, "participant has not observed shared baseline")
	}
	baseline, err := time.Parse(time.RFC3339, row.BaselineTime)
	if err != nil {
		return empty, err
	}
	if !horizon.After(baseline) || horizon.After(baseline.Add(24*time.Hour)) {
		return empty, core.NewError(core.CodeInvalidArgument, "shared wait must be within 24h after baseline")
	}
	if r.SessionID == row.HumanSession {
		if err := checkRPRequestRetirement(ctx, tx.conn, r.PrincipalID, "wait", r.SessionID, "shared_"+row.ID); err != nil {
			return empty, err
		}
	}
	if _, err := tx.conn.ExecContext(ctx, `UPDATE rp_shared_round_participants SET horizon_world_time=?,submission_key=?,request_hash=?,submitted_at_utc=? WHERE round_id=? AND session_id=? AND horizon_world_time=''`, r.HorizonWorldTime, r.IdempotencyKey, hash, s.now().UTC().Format(time.RFC3339Nano), row.ID, r.SessionID); err != nil {
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

func (s *Store) SubmitRPSharedSpeech(ctx context.Context, r RPSharedSpeechRequest) (RPSharedRound, error) {
	var empty RPSharedRound
	if !studioID(r.PrincipalID) || !studioID(r.SessionID) || !studioID(r.RoundID) || !studioID(r.IdempotencyKey) || len(r.IdempotencyKey) > 128 {
		return empty, core.NewError(core.CodeInvalidArgument, "bounded shared-speech identity/key required")
	}
	if err := (core.RPSpeechRequest{PrincipalID: r.PrincipalID, SessionID: r.SessionID, Text: r.Text, SpeechAct: r.SpeechAct, DeliveryChannel: r.DeliveryChannel, IntroduceSelf: r.IntroduceSelf, ExpectedCursor: 1, IdempotencyKey: r.IdempotencyKey}).Validate(); err != nil {
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
		if oldKind != "speech" || oldKey != r.IdempotencyKey {
			return empty, core.NewError(core.CodeBranchConflict, "shared-round participant already submitted an action")
		}
		if oldHash != hash {
			return empty, core.NewError(core.CodeIdempotencyMismatch, "shared-speech retry differs")
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
	if err := tx.conn.QueryRowContext(ctx, `SELECT b.head_sequence,c.current_world_time,s.observation_cursor FROM branches b JOIN world_clocks c ON c.instance_id=b.instance_id AND c.branch_id=b.branch_id JOIN rp_sessions s ON s.session_id=? WHERE b.instance_id=? AND b.branch_id=?`, r.SessionID, row.Instance, row.Branch).Scan(&head, &at, &cursor); err != nil {
		return empty, err
	}
	if head != row.BaselineHead || at != row.BaselineTime {
		return empty, staleRPSharedRound(ctx, tx, row.ID)
	}
	if cursor != head {
		return empty, core.NewError(core.CodeBranchConflict, "participant has not observed shared baseline")
	}
	speechAct := r.SpeechAct
	if speechAct == "" {
		speechAct = "statement"
	}
	child := core.RPSpeechRequest{PrincipalID: r.PrincipalID, SessionID: r.SessionID, Text: r.Text, SpeechAct: speechAct, DeliveryChannel: r.DeliveryChannel, IntroduceSelf: r.IntroduceSelf, ExpectedCursor: row.BaselineHead, IdempotencyKey: "shared_action_" + row.ID}
	if err := checkRPRequestRetirement(ctx, tx.conn, r.PrincipalID, "dialogue", r.SessionID, child.IdempotencyKey); err != nil {
		return empty, err
	}
	childJSON, err := core.CanonicalJSON(child)
	if err != nil {
		return empty, err
	}
	if _, err := tx.conn.ExecContext(ctx, `INSERT INTO rp_shared_round_actions(round_id,session_id,submission_key,request_hash,request_json,submitted_at_utc) VALUES (?,?,?,?,?,?)`, row.ID, r.SessionID, r.IdempotencyKey, hash, string(childJSON), s.now().UTC().Format(time.RFC3339Nano)); err != nil {
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

func (s *Store) SubmitRPSharedMove(ctx context.Context, r RPSharedMoveRequest) (RPSharedRound, error) {
	var empty RPSharedRound
	if !studioID(r.PrincipalID) || !studioID(r.SessionID) || !studioID(r.RoundID) || !studioID(r.IdempotencyKey) || len(r.IdempotencyKey) > 128 {
		return empty, core.NewError(core.CodeInvalidArgument, "bounded shared-move identity/key required")
	}
	if err := (core.RPMoveRequest{PrincipalID: r.PrincipalID, SessionID: r.SessionID, FromPlaceID: r.FromPlaceID, ToPlaceID: r.ToPlaceID, ExpectedCursor: 1, IdempotencyKey: r.IdempotencyKey}).Validate(); err != nil {
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
		if oldKind != "move" || oldKey != r.IdempotencyKey {
			return empty, core.NewError(core.CodeBranchConflict, "shared-round participant already submitted an action")
		}
		if oldHash != hash {
			return empty, core.NewError(core.CodeIdempotencyMismatch, "shared-move retry differs")
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
	var at, place string
	if err := tx.conn.QueryRowContext(ctx, `SELECT b.head_sequence,c.current_world_time,s.observation_cursor,p.place_id FROM branches b JOIN world_clocks c ON c.instance_id=b.instance_id AND c.branch_id=b.branch_id JOIN rp_sessions s ON s.session_id=? JOIN agent_positions p ON p.agent_id=s.controlled_entity_id WHERE b.instance_id=? AND b.branch_id=?`, r.SessionID, row.Instance, row.Branch).Scan(&head, &at, &cursor, &place); err != nil {
		return empty, err
	}
	if head != row.BaselineHead || at != row.BaselineTime {
		return empty, staleRPSharedRound(ctx, tx, row.ID)
	}
	if cursor != head || place != r.FromPlaceID {
		return empty, core.NewError(core.CodeBranchConflict, "shared move needs the observed origin and cursor")
	}
	var reachable, activeJourney int
	if err := tx.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_place_links l JOIN agent_places a ON a.place_id=l.from_place_id AND a.instance_id=l.instance_id AND a.branch_id=l.branch_id AND a.status='active' JOIN agent_places b ON b.place_id=l.to_place_id AND b.instance_id=l.instance_id AND b.branch_id=l.branch_id AND b.status='active' WHERE l.instance_id=? AND l.branch_id=? AND l.from_place_id=? AND l.to_place_id=?`, row.Instance, row.Branch, r.FromPlaceID, r.ToPlaceID).Scan(&reachable); err != nil {
		return empty, err
	}
	if reachable != 1 {
		return empty, core.NewError(core.CodeInvalidArgument, "destination is not an observed reachable immediate route")
	}
	allowed, err := rpTransitAllowsImmediate(ctx, tx.conn, row.Instance, row.Branch, r.FromPlaceID, r.ToPlaceID, at)
	if err != nil {
		return empty, err
	}
	if !allowed {
		return empty, core.NewError(core.CodeBranchConflict, "route is obstructed or requires a timed journey")
	}
	if err := tx.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_journeys WHERE instance_id=? AND branch_id=? AND agent_id=(SELECT controlled_entity_id FROM rp_sessions WHERE session_id=?) AND status='active'`, row.Instance, row.Branch, r.SessionID).Scan(&activeJourney); err != nil {
		return empty, err
	}
	if activeJourney != 0 {
		return empty, core.NewError(core.CodeCommandInProgress, "finish or cancel the active journey before moving")
	}
	child := core.RPMoveRequest{PrincipalID: r.PrincipalID, SessionID: r.SessionID, FromPlaceID: r.FromPlaceID, ToPlaceID: r.ToPlaceID, ExpectedCursor: row.BaselineHead, IdempotencyKey: "shared_action_" + row.ID}
	if err := checkRPRequestRetirement(ctx, tx.conn, r.PrincipalID, "move", r.SessionID, child.IdempotencyKey); err != nil {
		return empty, err
	}
	childJSON, err := core.CanonicalJSON(child)
	if err != nil {
		return empty, err
	}
	if _, err := tx.conn.ExecContext(ctx, `INSERT INTO rp_shared_round_actions(round_id,session_id,submission_key,request_hash,request_json,submitted_at_utc,action_kind) VALUES (?,?,?,?,?,?,'move')`, row.ID, r.SessionID, r.IdempotencyKey, hash, string(childJSON), s.now().UTC().Format(time.RFC3339Nano)); err != nil {
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

func rpSharedRoundTarget(ctx context.Context, conn *sql.Conn, row rpSharedRoundRow) (string, error) {
	var horizon sql.NullString
	if err := conn.QueryRowContext(ctx, `SELECT MIN(NULLIF(horizon_world_time,'')) FROM rp_shared_round_participants WHERE round_id=?`, row.ID).Scan(&horizon); err != nil {
		return "", err
	}
	if !horizon.Valid {
		return "", core.NewError(core.CodeProjectionDiverged, "shared wait boundary lacks a submitted horizon")
	}
	target := horizon.String
	var due string
	err := conn.QueryRowContext(ctx, `SELECT world_time FROM scheduler_items WHERE instance_id=? AND branch_id=? AND status='pending' AND world_time<=? ORDER BY world_time,phase_id,declared_priority,scheduler_item_id LIMIT 1`, row.Instance, row.Branch, target).Scan(&due)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	if err == nil {
		if due < row.BaselineTime {
			return "", core.NewError(core.CodeProjectionDiverged, "scheduler item predates shared baseline")
		}
		if due < target {
			target = due
		}
	}
	return target, nil
}

func rpSharedRoundHumanWaitPriority(ctx context.Context, conn *sql.Conn, row rpSharedRoundRow) (bool, error) {
	var humanWait string
	if err := conn.QueryRowContext(ctx, `SELECT submission_key FROM rp_shared_round_participants WHERE round_id=? AND session_id=? AND role='human'`, row.ID, row.HumanSession).Scan(&humanWait); err != nil {
		return false, classifyMissing(err, "Human shared-round submission")
	}
	if humanWait == "" {
		return false, nil
	}
	var priorAtSameTime, dueNow int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_shared_rounds WHERE instance_id=? AND branch_id=? AND status='settled' AND settlement_kind='speech' AND baseline_world_time=?`, row.Instance, row.Branch, row.BaselineTime).Scan(&priorAtSameTime); err != nil {
		return false, err
	}
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM scheduler_items WHERE instance_id=? AND branch_id=? AND status='pending' AND world_time<=?`, row.Instance, row.Branch, row.BaselineTime).Scan(&dueNow); err != nil {
		return false, err
	}
	return priorAtSameTime != 0 || dueNow != 0, nil
}

// AdvanceRPSharedRound is safe to retry after scheduler budget exhaustion,
// response loss or process restart. It calls exactly one Human-owned WaitRP
// key, never a separate wait command for each external participant.
func (s *Store) AdvanceRPSharedRound(ctx context.Context, r RPSharedRoundAdvanceRequest) (RPSharedRound, error) {
	return s.advanceRPSharedRoundWithProvider(ctx, r, core.DeterministicRPDecisionProvider{})
}

func (s *Store) advanceRPSharedRoundWithProvider(ctx context.Context, r RPSharedRoundAdvanceRequest, provider core.RPDecisionProvider) (RPSharedRound, error) {
	var empty RPSharedRound
	if r.Budget < 1 || r.Budget > 10000 {
		return empty, core.NewError(core.CodeInvalidArgument, "shared-round scheduler budget must be 1..10000")
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
	if err := authorizeRPSharedParticipant(ctx, tx.conn, row, r.RPSharedRoundReadRequest, row.Status != "settled"); err != nil {
		return empty, err
	}
	if row.Status == "settled" {
		out, err := rpSharedRoundViewForParticipant(ctx, tx.conn, row, r.SessionID)
		out.Replayed = true
		return out, err
	}
	if row.Status == "stale" {
		return empty, core.NewError(core.CodeBranchConflict, "shared-round baseline is stale")
	}
	if row.Status == "open" {
		view, err := rpSharedRoundViewForParticipant(ctx, tx.conn, row, r.SessionID)
		if err != nil {
			return empty, err
		}
		if view.Submitted != view.Required {
			return view, core.NewError(core.CodeCommandInProgress, "shared round waits for every participant including Human")
		}
		var head int64
		var at string
		if err := tx.conn.QueryRowContext(ctx, `SELECT b.head_sequence,c.current_world_time FROM branches b JOIN world_clocks c ON c.instance_id=b.instance_id AND c.branch_id=b.branch_id WHERE b.instance_id=? AND b.branch_id=?`, row.Instance, row.Branch).Scan(&head, &at); err != nil {
			return empty, err
		}
		if head != row.BaselineHead || at != row.BaselineTime {
			return empty, staleRPSharedRound(ctx, tx, row.ID)
		}
		var actionCount int
		if err := tx.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_shared_round_actions WHERE round_id=?`, row.ID).Scan(&actionCount); err != nil {
			return empty, err
		}
		forceWait := false
		if actionCount > 0 {
			forceWait, err = rpSharedRoundHumanWaitPriority(ctx, tx.conn, row)
			if err != nil {
				return empty, err
			}
		}
		if actionCount > 0 && !forceWait {
			// Rotate the first eligible Entity after the prior selected Entity.
			// This is independent of arrival/submission order and pins the choice
			// before any typed world command is attempted.
			var priorEntity string
			err := tx.conn.QueryRowContext(ctx, `SELECT p.entity_id FROM rp_shared_rounds r JOIN rp_shared_round_participants p ON p.round_id=r.round_id AND p.session_id=r.selected_session_id WHERE r.instance_id=? AND r.branch_id=? AND r.status='settled' AND r.settlement_kind='speech' ORDER BY r.settled_sequence DESC LIMIT 1`, row.Instance, row.Branch).Scan(&priorEntity)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return empty, err
			}
			if err := tx.conn.QueryRowContext(ctx, `SELECT p.session_id,a.action_kind FROM rp_shared_round_actions a JOIN rp_shared_round_participants p ON p.round_id=a.round_id AND p.session_id=a.session_id WHERE a.round_id=? ORDER BY CASE WHEN p.entity_id>? THEN 0 ELSE 1 END,p.entity_id LIMIT 1`, row.ID, priorEntity).Scan(&row.SelectedSession, &row.SelectedActionKind); err != nil {
				return empty, err
			}
			if _, err := tx.conn.ExecContext(ctx, `UPDATE rp_shared_rounds SET status='advancing',settlement_kind='speech',selected_session_id=?,selected_action_kind=? WHERE round_id=? AND status='open'`, row.SelectedSession, row.SelectedActionKind, row.ID); err != nil {
				return empty, err
			}
			row.Status, row.SettlementKind = "advancing", "speech"
		} else {
			target, err := rpSharedRoundTarget(ctx, tx.conn, row)
			if err != nil {
				return empty, err
			}
			key := "shared_" + row.ID
			if len(key) > 128 {
				return empty, core.NewError(core.CodeStorageFailure, "shared-round wait key is too long")
			}
			if _, err := tx.conn.ExecContext(ctx, `UPDATE rp_shared_rounds SET status='advancing',settlement_kind='wait',advance_target=?,wait_key=? WHERE round_id=? AND status='open'`, target, key, row.ID); err != nil {
				return empty, err
			}
			row.Status, row.SettlementKind, row.AdvanceTarget, row.WaitKey = "advancing", "wait", target, key
		}
	}
	if row.SettlementKind == "speech" {
		if err := tx.Commit(ctx); err != nil {
			return empty, err
		}
		if row.SelectedActionKind == "move" {
			return s.advanceRPSharedMove(ctx, r, row)
		}
		if row.SelectedActionKind != "speech" {
			return empty, core.NewError(core.CodeProjectionDiverged, "unknown selected shared action")
		}
		return s.advanceRPSharedSpeech(ctx, r, row, provider)
	}
	var humanPrincipal string
	if err := tx.conn.QueryRowContext(ctx, `SELECT principal_id FROM rp_shared_round_participants WHERE round_id=? AND session_id=? AND role='human'`, row.ID, row.HumanSession).Scan(&humanPrincipal); err != nil {
		return empty, classifyMissing(err, "Human shared-round participant")
	}
	var existingHash string
	err = tx.conn.QueryRowContext(ctx, `SELECT request_hash FROM rp_wait_intents WHERE session_id=? AND idempotency_key=?`, row.HumanSession, row.WaitKey).Scan(&existingHash)
	if err == nil {
		pinned := core.RPWaitRequest{PrincipalID: humanPrincipal, SessionID: row.HumanSession, TargetWorldTime: row.AdvanceTarget, Budget: 1, ExpectedCursor: row.BaselineHead, IdempotencyKey: row.WaitKey}
		pinnedHash, hashErr := core.HashJSON(pinned)
		if hashErr != nil {
			return empty, hashErr
		}
		if existingHash != pinnedHash {
			matches, hashErr := matchesLegacyRPSharedWaitHash(pinned, existingHash)
			if hashErr != nil {
				return empty, hashErr
			}
			if !matches {
				return empty, staleRPSharedWaitBeforeAcceptance(ctx, tx, row.ID)
			}
		}
	} else if errors.Is(err, sql.ErrNoRows) {
		var head int64
		var at string
		if err := tx.conn.QueryRowContext(ctx, `SELECT b.head_sequence,c.current_world_time FROM branches b JOIN world_clocks c ON c.instance_id=b.instance_id AND c.branch_id=b.branch_id WHERE b.instance_id=? AND b.branch_id=?`, row.Instance, row.Branch).Scan(&head, &at); err != nil {
			return empty, err
		}
		if head != row.BaselineHead || at != row.BaselineTime {
			return empty, staleRPSharedWaitBeforeAcceptance(ctx, tx, row.ID)
		}
	} else if err != nil {
		return empty, err
	}
	if err := tx.Commit(ctx); err != nil {
		return empty, err
	}
	if s.afterRPSharedActionStage != nil {
		if err := s.afterRPSharedActionStage("wait_selection_committed"); err != nil {
			return empty, err
		}
	}
	wait, err := s.waitRPForSharedRound(ctx, core.RPWaitRequest{PrincipalID: humanPrincipal, SessionID: row.HumanSession, TargetWorldTime: row.AdvanceTarget, Budget: r.Budget, ExpectedCursor: row.BaselineHead, IdempotencyKey: row.WaitKey}, row.ID)
	if err != nil {
		return empty, err
	}
	if wait.Status != "completed" {
		view, err := s.ReadRPSharedRound(ctx, r.RPSharedRoundReadRequest)
		view.PendingDue = wait.PendingDue
		return view, err
	}
	settle, err := beginImmediate(ctx, s.db)
	if err != nil {
		return empty, err
	}
	defer settle.Rollback(ctx)
	current, err := readRPSharedRoundRow(ctx, settle.conn, row.ID)
	if err != nil {
		return empty, err
	}
	if current.Status == "settled" {
		out, err := rpSharedRoundViewForParticipant(ctx, settle.conn, current, r.SessionID)
		out.Replayed = true
		return out, err
	}
	if current.Status != "advancing" || current.WaitKey != row.WaitKey || current.AdvanceTarget != row.AdvanceTarget {
		return empty, core.NewError(core.CodeBranchConflict, "shared-round settlement changed")
	}
	if _, err := settle.conn.ExecContext(ctx, `UPDATE rp_shared_rounds SET status='settled',completion_event_id=?,settled_sequence=?,settled_at_utc=? WHERE round_id=? AND status='advancing'`, wait.EventID, wait.EventSequence, s.now().UTC().Format(time.RFC3339Nano), row.ID); err != nil {
		return empty, err
	}
	current.Status, current.CompletionEvent, current.SettledSequence = "settled", wait.EventID, wait.EventSequence
	out, err := rpSharedRoundViewForParticipant(ctx, settle.conn, current, r.SessionID)
	if err != nil {
		return empty, err
	}
	if err := settle.Commit(ctx); err != nil {
		return empty, err
	}
	return out, nil
}

func (s *Store) advanceRPSharedMove(ctx context.Context, r RPSharedRoundAdvanceRequest, row rpSharedRoundRow) (RPSharedRound, error) {
	if s.afterRPSharedActionStage != nil {
		if err := s.afterRPSharedActionStage("selection_committed"); err != nil {
			return RPSharedRound{}, err
		}
	}
	var raw string
	if err := s.db.QueryRowContext(ctx, `SELECT request_json FROM rp_shared_round_actions WHERE round_id=? AND session_id=? AND action_kind='move'`, row.ID, row.SelectedSession).Scan(&raw); err != nil {
		return RPSharedRound{}, classifyMissing(err, "selected shared move")
	}
	var request core.RPMoveRequest
	if err := json.Unmarshal([]byte(raw), &request); err != nil {
		return RPSharedRound{}, core.WrapError(core.CodeProjectionDiverged, "decode selected shared move", err)
	}
	if request.SessionID != row.SelectedSession || request.ExpectedCursor != row.BaselineHead || request.IdempotencyKey != "shared_action_"+row.ID {
		return RPSharedRound{}, core.NewError(core.CodeProjectionDiverged, "selected shared move binding differs")
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
	if head != row.BaselineHead {
		var accepted int
		if err := check.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM commands WHERE instance_id=? AND branch_id=? AND command_type='RPPlayerMove' AND idempotency_key=? AND status='committed'`, row.Instance, row.Branch, "rp_move:"+request.SessionID+":"+request.IdempotencyKey).Scan(&accepted); err != nil {
			check.Rollback(ctx)
			return RPSharedRound{}, err
		}
		if accepted == 0 {
			if _, err := check.conn.ExecContext(ctx, `UPDATE rp_shared_rounds SET status='stale' WHERE round_id=? AND status='advancing' AND selected_action_kind='move' AND selected_session_id=?`, row.ID, row.SelectedSession); err != nil {
				check.Rollback(ctx)
				return RPSharedRound{}, err
			}
			if err := check.Commit(ctx); err != nil {
				return RPSharedRound{}, err
			}
			return RPSharedRound{}, core.NewError(core.CodeBranchConflict, "shared move baseline changed before typed acceptance")
		}
	}
	check.Rollback(ctx)
	move, err := s.MoveRP(ctx, request)
	if err != nil {
		return RPSharedRound{}, err
	}
	if s.afterRPSharedActionStage != nil {
		if err := s.afterRPSharedActionStage("move_event_committed"); err != nil {
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
	if current.Status != "advancing" || current.SettlementKind != "speech" || current.SelectedActionKind != "move" || current.SelectedSession != request.SessionID || move.EventSequence != row.BaselineHead+1 {
		return RPSharedRound{}, core.NewError(core.CodeBranchConflict, "shared move settlement changed")
	}
	if _, err := settle.conn.ExecContext(ctx, `UPDATE rp_shared_rounds SET status='settled',completion_event_id=?,settled_sequence=?,settled_at_utc=? WHERE round_id=? AND status='advancing'`, move.EventID, move.EventSequence, s.now().UTC().Format(time.RFC3339Nano), row.ID); err != nil {
		return RPSharedRound{}, err
	}
	current.Status, current.CompletionEvent, current.SettledSequence = "settled", move.EventID, move.EventSequence
	out, err := rpSharedRoundViewForParticipant(ctx, settle.conn, current, r.SessionID)
	if err != nil {
		return RPSharedRound{}, err
	}
	if err := settle.Commit(ctx); err != nil {
		return RPSharedRound{}, err
	}
	return out, nil
}

func (s *Store) advanceRPSharedSpeech(ctx context.Context, r RPSharedRoundAdvanceRequest, row rpSharedRoundRow, provider core.RPDecisionProvider) (RPSharedRound, error) {
	if s.afterRPSharedActionStage != nil {
		if err := s.afterRPSharedActionStage("selection_committed"); err != nil {
			return RPSharedRound{}, err
		}
	}
	var raw string
	if err := s.db.QueryRowContext(ctx, `SELECT request_json FROM rp_shared_round_actions WHERE round_id=? AND session_id=?`, row.ID, row.SelectedSession).Scan(&raw); err != nil {
		return RPSharedRound{}, classifyMissing(err, "selected shared speech")
	}
	var request core.RPSpeechRequest
	if err := json.Unmarshal([]byte(raw), &request); err != nil {
		return RPSharedRound{}, core.WrapError(core.CodeProjectionDiverged, "decode selected shared speech", err)
	}
	if request.SessionID != row.SelectedSession || request.ExpectedCursor != row.BaselineHead || request.IdempotencyKey != "shared_action_"+row.ID {
		return RPSharedRound{}, core.NewError(core.CodeProjectionDiverged, "selected shared speech binding differs")
	}
	// A creator/system Event may have changed the head after selection. Do
	// not hold the branch slot forever if the selected typed child was never
	// accepted. If it was accepted, its exact command receipt must still drain.
	check, err := beginImmediate(ctx, s.db)
	if err != nil {
		return RPSharedRound{}, err
	}
	var head int64
	if err := check.conn.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, row.Instance, row.Branch).Scan(&head); err != nil {
		check.Rollback(ctx)
		return RPSharedRound{}, err
	}
	if head != row.BaselineHead {
		var accepted int
		commandKey := "rp_speech:" + request.SessionID + ":" + request.IdempotencyKey
		if err := check.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM commands WHERE instance_id=? AND branch_id=? AND command_type='RPSpeak' AND idempotency_key=? AND status='committed'`, row.Instance, row.Branch, commandKey).Scan(&accepted); err != nil {
			check.Rollback(ctx)
			return RPSharedRound{}, err
		}
		if accepted == 0 {
			if _, err := check.conn.ExecContext(ctx, `UPDATE rp_shared_rounds SET status='stale' WHERE round_id=? AND status='advancing' AND settlement_kind='speech' AND selected_session_id=?`, row.ID, row.SelectedSession); err != nil {
				check.Rollback(ctx)
				return RPSharedRound{}, err
			}
			if err := check.Commit(ctx); err != nil {
				return RPSharedRound{}, err
			}
			return RPSharedRound{}, core.NewError(core.CodeBranchConflict, "shared action baseline changed before typed acceptance")
		}
	}
	check.Rollback(ctx)
	speech, err := s.SpeakRP(ctx, request)
	if err != nil {
		return RPSharedRound{}, err
	}
	if s.afterRPSharedActionStage != nil {
		if err := s.afterRPSharedActionStage("speech_event_committed"); err != nil {
			return RPSharedRound{}, err
		}
	}
	turn, err := s.RunRPTurn(ctx, request, provider)
	if err != nil {
		return RPSharedRound{}, err
	}
	if turn.Status != "settled" || turn.PlayerEventID != speech.EventID || turn.SettledSequence < speech.EventSequence {
		return RPSharedRound{}, core.NewError(core.CodeProjectionDiverged, "selected shared speech turn did not settle its accepted Event")
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
	if current.Status != "advancing" || current.SettlementKind != "speech" || current.SelectedSession != request.SessionID {
		return RPSharedRound{}, core.NewError(core.CodeBranchConflict, "shared action settlement changed")
	}
	if _, err := settle.conn.ExecContext(ctx, `UPDATE rp_shared_rounds SET status='settled',completion_event_id=?,settled_sequence=?,settled_at_utc=? WHERE round_id=? AND status='advancing'`, speech.EventID, speech.EventSequence, s.now().UTC().Format(time.RFC3339Nano), row.ID); err != nil {
		return RPSharedRound{}, err
	}
	current.Status, current.CompletionEvent, current.SettledSequence = "settled", speech.EventID, speech.EventSequence
	out, err := rpSharedRoundViewForParticipant(ctx, settle.conn, current, r.SessionID)
	if err != nil {
		return RPSharedRound{}, err
	}
	if err := settle.Commit(ctx); err != nil {
		return RPSharedRound{}, err
	}
	return out, nil
}
