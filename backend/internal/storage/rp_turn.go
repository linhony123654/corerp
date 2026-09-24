package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"corerp.local/backend/internal/core"
)

type RPTurnResult struct {
	NarrativeStyle    core.RPStyleProfile `json:"narrative_style"`
	NarrativeWarnings []string            `json:"narrative_warnings"`
	TurnRunID         string              `json:"turn_run_id"`
	PlayerTurnID      string              `json:"player_turn_id"`
	PlayerEventID     string              `json:"player_event_id"`
	NPCEventIDs       []string            `json:"npc_event_ids"`
	NarrativeLines    []string            `json:"narrative_lines"`
	SettledSequence   int64               `json:"settled_sequence"`
	Status            string              `json:"status"`
	Replayed          bool                `json:"replayed"`
}

type RPTurnResumeRequest struct {
	PrincipalID    string `json:"principal_id"`
	SessionID      string `json:"session_id"`
	IdempotencyKey string `json:"idempotency_key"`
}

type rpTurnRun struct {
	ID              string
	SessionID       string
	SpeechKey       string
	Status          string
	PlayerTurnID    string
	PlayerEventID   string
	ListenerIDsJSON string
	NarrativeJSON   string
	SettledSequence sql.NullInt64
}

// PlayRPTurn is the local deterministic product path. A deployment with a
// trusted LLM adapter can use RunRPTurn with another provider implementation.
func (s *Store) PlayRPTurn(ctx context.Context, request core.RPSpeechRequest) (RPTurnResult, error) {
	return s.RunRPTurn(ctx, request, core.DeterministicRPDecisionProvider{})
}

func (s *Store) PlayResumeRPTurn(ctx context.Context, request RPTurnResumeRequest) (RPTurnResult, error) {
	return s.ResumeRPTurn(ctx, request, core.DeterministicRPDecisionProvider{})
}

func (s *Store) ResumeRPTurn(ctx context.Context, request RPTurnResumeRequest, provider core.RPDecisionProvider) (RPTurnResult, error) {
	if strings.TrimSpace(request.PrincipalID) == "" || strings.TrimSpace(request.SessionID) == "" || strings.TrimSpace(request.IdempotencyKey) == "" || len(request.IdempotencyKey) > 128 {
		return RPTurnResult{}, core.NewError(core.CodeInvalidArgument, "principal_id, session_id and idempotency_key are required")
	}
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return RPTurnResult{}, core.WrapError(core.CodeStorageFailure, "begin RP turn resume", err)
	}
	defer tx.Rollback(ctx)
	session, err := loadRPSession(ctx, tx.conn, request.PrincipalID, request.SessionID)
	if err != nil {
		return RPTurnResult{}, err
	}
	if err := authorizeRPControl(ctx, tx.conn, request.PrincipalID, session.InstanceID, session.BranchID, session.ControlledEntityID); err != nil {
		return RPTurnResult{}, err
	}
	var raw string
	if err := tx.conn.QueryRowContext(ctx, `SELECT request_json FROM rp_turn_runs WHERE session_id = ? AND idempotency_key = ?`, session.SessionID, request.IdempotencyKey).Scan(&raw); err != nil {
		return RPTurnResult{}, classifyMissing(err, "durable RP turn")
	}
	var original core.RPSpeechRequest
	if err := json.Unmarshal([]byte(raw), &original); err != nil {
		return RPTurnResult{}, core.WrapError(core.CodeProjectionDiverged, "decode durable RP turn request", err)
	}
	if original.PrincipalID != request.PrincipalID || original.SessionID != request.SessionID || original.IdempotencyKey != request.IdempotencyKey {
		return RPTurnResult{}, core.NewError(core.CodeProjectionDiverged, "durable RP turn request identity mismatch")
	}
	// Release the read transaction before the long, multi-stage workflow.
	tx.Rollback(ctx)
	return s.RunRPTurn(ctx, original, provider)
}

// RunRPTurn is idempotent across process restarts. Each world effect is owned
// by its existing command/event key; this table only tracks orchestration.
func (s *Store) RunRPTurn(ctx context.Context, request core.RPSpeechRequest, provider core.RPDecisionProvider) (RPTurnResult, error) {
	if err := request.Validate(); err != nil {
		return RPTurnResult{}, err
	}
	if request.SpeechAct == "" {
		request.SpeechAct = "statement"
	}
	run, wasExisting, err := s.ensureRPTurnRun(ctx, request)
	if err != nil {
		return RPTurnResult{}, err
	}
	if run.Status == "settled" {
		return s.loadRPTurnResult(ctx, run, true)
	}
	if provider == nil {
		return RPTurnResult{}, core.NewError(core.CodeInvalidArgument, "unfinished RP turn requires a decision provider")
	}
	if err := s.afterTurnStage("turn_open"); err != nil {
		return RPTurnResult{}, err
	}
	speechRequest := request
	speechRequest.NarrativeStyle = nil
	speechRequest.IdempotencyKey = run.SpeechKey
	speech, err := s.SpeakRP(ctx, speechRequest)
	if err != nil {
		return RPTurnResult{}, err
	}
	if err := s.afterTurnStage("player_event_committed"); err != nil {
		return RPTurnResult{}, err
	}
	if err := s.markRPTurnPlayerCommitted(ctx, run.ID, speech); err != nil {
		return RPTurnResult{}, err
	}
	if err := s.afterTurnStage("player_committed"); err != nil {
		return RPTurnResult{}, err
	}
	if err := s.advanceRPTurnRunStatus(ctx, run.ID, "player_committed", "npc_deciding"); err != nil {
		return RPTurnResult{}, err
	}
	for _, npcID := range speech.ListenerIDs {
		committed, err := s.hasRPNPCDecision(ctx, run.SessionID, speech.TurnID, npcID)
		if err != nil {
			return RPTurnResult{}, err
		}
		if committed {
			continue
		}
		decisionRequest := core.RPDecisionRequest{PrincipalID: request.PrincipalID, SessionID: request.SessionID, TurnID: speech.TurnID, NPCEntityID: npcID}
		decision, err := s.DecideRP(ctx, decisionRequest, provider)
		if err != nil {
			return RPTurnResult{}, err
		}
		if _, err := s.CommitRPDecision(ctx, decisionRequest, decision); err != nil {
			return RPTurnResult{}, err
		}
		if err := s.afterTurnStage("npc_effect_committed"); err != nil {
			return RPTurnResult{}, err
		}
	}
	if err := s.markRPTurnNPCsCommitted(ctx, run.ID, run.SessionID, speech.TurnID); err != nil {
		return RPTurnResult{}, err
	}
	if err := s.afterTurnStage("npc_effects_committed"); err != nil {
		return RPTurnResult{}, err
	}
	current, err := s.loadRPTurnRun(ctx, run.ID)
	if err != nil {
		return RPTurnResult{}, err
	}
	if current.Status != "narrative_ready" && current.Status != "settled" {
		style, err := s.loadRPTurnStyle(ctx, run.ID)
		if err != nil {
			return RPTurnResult{}, err
		}
		view, err := s.renderRPTurnStyled(ctx, run.SessionID, speech.TurnID, speech.EventID, style.Profile)
		if err != nil {
			return RPTurnResult{}, err
		}
		observation, err := s.ObserveRPSession(ctx, core.RPSessionReadRequest{PrincipalID: request.PrincipalID, SessionID: request.SessionID})
		if err != nil {
			return RPTurnResult{}, err
		}
		if err := s.markRPTurnNarrativeReady(ctx, run.ID, view.Lines, observation.ObservationCursor); err != nil {
			return RPTurnResult{}, err
		}
	}
	if err := s.afterTurnStage("narrative_ready"); err != nil {
		return RPTurnResult{}, err
	}
	if err := s.settleRPTurn(ctx, run.ID, run.SessionID, speech.TurnID); err != nil {
		return RPTurnResult{}, err
	}
	current, err = s.loadRPTurnRun(ctx, run.ID)
	if err != nil {
		return RPTurnResult{}, err
	}
	return s.loadRPTurnResult(ctx, current, wasExisting)
}

func (s *Store) afterTurnStage(stage string) error {
	if s.afterRPTurnStage != nil {
		return s.afterRPTurnStage(stage)
	}
	return nil
}

func (s *Store) ensureRPTurnRun(ctx context.Context, request core.RPSpeechRequest) (rpTurnRun, bool, error) {
	requestHash, err := core.HashJSON(request)
	if err != nil {
		return rpTurnRun{}, false, err
	}
	requestJSON, err := core.CanonicalJSON(request)
	if err != nil {
		return rpTurnRun{}, false, err
	}
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return rpTurnRun{}, false, core.WrapError(core.CodeStorageFailure, "begin RP turn intent", err)
	}
	defer tx.Rollback(ctx)
	session, err := loadRPSession(ctx, tx.conn, request.PrincipalID, request.SessionID)
	if err != nil {
		return rpTurnRun{}, false, err
	}
	if err := authorizeRPControl(ctx, tx.conn, request.PrincipalID, session.InstanceID, session.BranchID, session.ControlledEntityID); err != nil {
		return rpTurnRun{}, false, err
	}
	var run rpTurnRun
	var existingHash string
	if err := checkRPRequestRetirement(ctx, tx.conn, request.PrincipalID, "dialogue", session.SessionID, request.IdempotencyKey); err != nil {
		return rpTurnRun{}, false, err
	}
	var playerTurnID, playerEventID sql.NullString
	err = tx.conn.QueryRowContext(ctx, `SELECT turn_run_id, session_id, player_speech_key, status, player_turn_id, player_event_id, listener_ids_json, narrative_json, settled_sequence, request_hash FROM rp_turn_runs WHERE session_id = ? AND idempotency_key = ?`, session.SessionID, request.IdempotencyKey).Scan(&run.ID, &run.SessionID, &run.SpeechKey, &run.Status, &playerTurnID, &playerEventID, &run.ListenerIDsJSON, &run.NarrativeJSON, &run.SettledSequence, &existingHash)
	if err == nil {
		if existingHash != requestHash {
			return rpTurnRun{}, false, core.NewError(core.CodeIdempotencyMismatch, "RP turn key was used with another input")
		}
		run.PlayerTurnID = playerTurnID.String
		run.PlayerEventID = playerEventID.String
		return run, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return rpTurnRun{}, false, core.WrapError(core.CodeStorageFailure, "look up RP turn intent", err)
	}
	if session.Status != "active" {
		return rpTurnRun{}, false, core.NewError(core.CodeBranchConflict, "RP session is closed")
	}
	if session.InstanceID != M2DemoInstanceID || session.BranchID != M2DemoBranchID {
		packages, err := readStudioActivePackages(ctx, tx.conn, session.InstanceID, session.BranchID)
		if err != nil {
			return rpTurnRun{}, false, err
		}
		if packages == nil {
			return rpTurnRun{}, false, core.NewError(core.CodeNotFound, "supported RP world not found")
		}
	}
	var pendingWaits, pendingTurns int
	if err := tx.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_wait_intents i JOIN rp_sessions s ON s.session_id = i.session_id WHERE s.instance_id = ? AND s.branch_id = ? AND i.status = 'pending'`, session.InstanceID, session.BranchID).Scan(&pendingWaits); err != nil {
		return rpTurnRun{}, false, core.WrapError(core.CodeStorageFailure, "check pending wait before RP turn", err)
	}
	if err := tx.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_turn_runs r JOIN rp_sessions s ON s.session_id = r.session_id WHERE s.instance_id = ? AND s.branch_id = ? AND r.status <> 'settled'`, session.InstanceID, session.BranchID).Scan(&pendingTurns); err != nil {
		return rpTurnRun{}, false, core.WrapError(core.CodeStorageFailure, "check pending RP turn", err)
	}
	if pendingWaits != 0 || pendingTurns != 0 {
		return rpTurnRun{}, false, core.NewError(core.CodeCommandInProgress, "another RP action or turn is pending")
	}
	var head int64
	if err := tx.conn.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id = ? AND branch_id = ?`, session.InstanceID, session.BranchID).Scan(&head); err != nil {
		return rpTurnRun{}, false, classifyMissing(err, "RP turn branch")
	}
	if request.ExpectedCursor != head || session.ObservationCursor != head {
		return rpTurnRun{}, false, core.NewError(core.CodeBranchConflict, "RP turn requires a current observation cursor")
	}
	keyHash, err := core.HashJSON(struct{ SessionID, Key string }{session.SessionID, request.IdempotencyKey})
	if err != nil {
		return rpTurnRun{}, false, err
	}
	suffix := keyHash[7:]
	run = rpTurnRun{ID: "rpturn_" + suffix, SessionID: session.SessionID, SpeechKey: "turn_" + suffix, Status: "open", ListenerIDsJSON: "[]", NarrativeJSON: "[]"}
	now := s.now().UTC().Format(time.RFC3339Nano)
	if err := execAgentOne(ctx, tx.conn, "save RP turn intent", `INSERT INTO rp_turn_runs(turn_run_id, session_id, idempotency_key, player_speech_key, request_hash, request_json, status, created_at_utc, updated_at_utc) VALUES (?, ?, ?, ?, ?, ?, 'open', ?, ?)`, run.ID, session.SessionID, request.IdempotencyKey, run.SpeechKey, requestHash, string(requestJSON), now, now); err != nil {
		return rpTurnRun{}, false, err
	}
	if err := pinRPTurnStyle(ctx, tx.conn, run.ID, session, request.NarrativeStyle); err != nil {
		return rpTurnRun{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return rpTurnRun{}, false, core.WrapError(core.CodeStorageFailure, "commit RP turn intent", err)
	}
	return run, false, nil
}

func (s *Store) markRPTurnPlayerCommitted(ctx context.Context, runID string, speech RPSpeechResult) error {
	listenersJSON, err := core.CanonicalJSON(speech.ListenerIDs)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE rp_turn_runs SET status = 'player_committed', player_turn_id = ?, player_event_id = ?, listener_ids_json = ?, updated_at_utc = ? WHERE turn_run_id = ? AND status = 'open'`, speech.TurnID, speech.EventID, string(listenersJSON), s.now().UTC().Format(time.RFC3339Nano), runID)
	if err != nil {
		return core.WrapError(core.CodeStorageFailure, "record committed player turn", err)
	}
	return nil
}

func (s *Store) advanceRPTurnRunStatus(ctx context.Context, runID, from, to string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE rp_turn_runs SET status = ?, updated_at_utc = ? WHERE turn_run_id = ? AND status = ?`, to, s.now().UTC().Format(time.RFC3339Nano), runID, from)
	if err != nil {
		return core.WrapError(core.CodeStorageFailure, "advance RP turn stage", err)
	}
	return nil
}

func (s *Store) hasRPNPCDecision(ctx context.Context, sessionID, turnID, npcID string) (bool, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_npc_decisions WHERE session_id = ? AND parent_turn_id = ? AND npc_entity_id = ?`, sessionID, turnID, npcID).Scan(&count); err != nil {
		return false, core.WrapError(core.CodeStorageFailure, "check committed NPC effect", err)
	}
	return count == 1, nil
}

func (s *Store) markRPTurnNPCsCommitted(ctx context.Context, runID, sessionID, playerTurnID string) error {
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return core.WrapError(core.CodeStorageFailure, "begin RP turn NPC stage", err)
	}
	defer tx.Rollback(ctx)
	var status, listenersJSON string
	if err := tx.conn.QueryRowContext(ctx, `SELECT status, listener_ids_json FROM rp_turn_runs WHERE turn_run_id = ?`, runID).Scan(&status, &listenersJSON); err != nil {
		return classifyMissing(err, "RP turn NPC stage")
	}
	if status == "npc_effects_committed" || status == "narrative_ready" || status == "settled" {
		return nil
	}
	if status != "npc_deciding" {
		return core.NewError(core.CodeBranchConflict, "RP turn is not ready for NPC stage completion")
	}
	var listeners []string
	if err := json.Unmarshal([]byte(listenersJSON), &listeners); err != nil {
		return core.WrapError(core.CodeProjectionDiverged, "decode committed RP turn listeners", err)
	}
	var committedCount int
	if err := tx.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_npc_decisions WHERE session_id = ? AND parent_turn_id = ?`, sessionID, playerTurnID).Scan(&committedCount); err != nil {
		return core.WrapError(core.CodeStorageFailure, "count committed NPC effects", err)
	}
	if committedCount != len(listeners) {
		return core.NewError(core.CodeCommandInProgress, "RP turn still has undecided listeners")
	}
	if err := execAgentOne(ctx, tx.conn, "complete RP turn NPC stage", `UPDATE rp_turn_runs SET status = 'npc_effects_committed', updated_at_utc = ? WHERE turn_run_id = ? AND status = 'npc_deciding'`, s.now().UTC().Format(time.RFC3339Nano), runID); err != nil {
		return err
	}
	if err := execAgentOne(ctx, tx.conn, "complete RP session NPC stage", `UPDATE rp_sessions SET turn_state = 'npc_effects_committed' WHERE session_id = ? AND turn_cursor = ? AND status = 'active'`, sessionID, playerTurnID); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return core.WrapError(core.CodeStorageFailure, "commit RP turn NPC stage", err)
	}
	return nil
}

func (s *Store) markRPTurnNarrativeReady(ctx context.Context, runID string, lines []string, sequence int64) error {
	encoded, err := core.CanonicalJSON(lines)
	if err != nil {
		return err
	}
	if err := execAgentOne(ctx, s.db, "save RP narrative view", `UPDATE rp_turn_runs SET status = 'narrative_ready', narrative_json = ?, settled_sequence = ?, updated_at_utc = ? WHERE turn_run_id = ? AND status = 'npc_effects_committed'`, string(encoded), sequence, s.now().UTC().Format(time.RFC3339Nano), runID); err != nil {
		return err
	}
	return nil
}

func (s *Store) settleRPTurn(ctx context.Context, runID, sessionID, playerTurnID string) error {
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return core.WrapError(core.CodeStorageFailure, "begin RP turn settlement", err)
	}
	defer tx.Rollback(ctx)
	var status string
	if err := tx.conn.QueryRowContext(ctx, `SELECT status FROM rp_turn_runs WHERE turn_run_id = ?`, runID).Scan(&status); err != nil {
		return classifyMissing(err, "RP turn settlement")
	}
	if status == "settled" {
		return nil
	}
	if status != "narrative_ready" {
		return core.NewError(core.CodeBranchConflict, "RP turn narrative is not ready")
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	if err := execAgentOne(ctx, tx.conn, "settle RP turn", `UPDATE rp_turn_runs SET status = 'settled', updated_at_utc = ?, settled_at_utc = ? WHERE turn_run_id = ? AND status = 'narrative_ready'`, now, now, runID); err != nil {
		return err
	}
	if err := execAgentOne(ctx, tx.conn, "settle RP session turn", `UPDATE rp_sessions SET turn_state = 'settled' WHERE session_id = ? AND turn_cursor = ? AND status = 'active'`, sessionID, playerTurnID); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return core.WrapError(core.CodeStorageFailure, "commit RP turn settlement", err)
	}
	return nil
}

func (s *Store) loadRPTurnRun(ctx context.Context, runID string) (rpTurnRun, error) {
	var run rpTurnRun
	var playerTurnID, playerEventID sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT turn_run_id, session_id, player_speech_key, status, player_turn_id, player_event_id, listener_ids_json, narrative_json, settled_sequence FROM rp_turn_runs WHERE turn_run_id = ?`, runID).Scan(&run.ID, &run.SessionID, &run.SpeechKey, &run.Status, &playerTurnID, &playerEventID, &run.ListenerIDsJSON, &run.NarrativeJSON, &run.SettledSequence)
	if err != nil {
		return rpTurnRun{}, classifyMissing(err, "RP turn run")
	}
	run.PlayerTurnID, run.PlayerEventID = playerTurnID.String, playerEventID.String
	return run, nil
}

func (s *Store) loadRPTurnResult(ctx context.Context, run rpTurnRun, replayed bool) (RPTurnResult, error) {
	if run.Status != "settled" || !run.SettledSequence.Valid {
		return RPTurnResult{}, core.NewError(core.CodeCommandInProgress, "RP turn has not settled")
	}
	result := RPTurnResult{TurnRunID: run.ID, PlayerTurnID: run.PlayerTurnID, PlayerEventID: run.PlayerEventID, NPCEventIDs: make([]string, 0), SettledSequence: run.SettledSequence.Int64, Status: "settled", Replayed: replayed}
	if err := json.Unmarshal([]byte(run.NarrativeJSON), &result.NarrativeLines); err != nil {
		return RPTurnResult{}, core.WrapError(core.CodeProjectionDiverged, "decode RP narrative view", err)
	}
	style, err := s.loadRPTurnStyle(ctx, run.ID)
	if err != nil {
		return RPTurnResult{}, err
	}
	result.NarrativeStyle = style.Profile
	result.NarrativeWarnings = []string{}
	if style.Profile.ProseInstructions != "" || len(style.Profile.ForbiddenPatterns) > 0 {
		view, err := s.renderRPTurnStyled(ctx, run.SessionID, run.PlayerTurnID, run.PlayerEventID, style.Profile)
		if err != nil {
			return RPTurnResult{}, err
		}
		result.NarrativeWarnings = view.Warnings
	}
	rows, err := s.db.QueryContext(ctx, `SELECT d.event_id FROM rp_npc_decisions d JOIN events e ON e.event_id = d.event_id WHERE d.session_id = ? AND d.parent_turn_id = ? ORDER BY e.event_sequence`, run.SessionID, run.PlayerTurnID)
	if err != nil {
		return RPTurnResult{}, core.WrapError(core.CodeStorageFailure, "read RP turn NPC events", err)
	}
	defer rows.Close()
	for rows.Next() {
		var eventID string
		if err := rows.Scan(&eventID); err != nil {
			return RPTurnResult{}, core.WrapError(core.CodeStorageFailure, "scan RP turn NPC event", err)
		}
		result.NPCEventIDs = append(result.NPCEventIDs, eventID)
	}
	if err := rows.Err(); err != nil {
		return RPTurnResult{}, core.WrapError(core.CodeStorageFailure, "iterate RP turn NPC events", err)
	}
	return result, nil
}
