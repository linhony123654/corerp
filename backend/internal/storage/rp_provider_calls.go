package storage

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"corerp.local/backend/internal/core"
)

// RPProviderCall is application evidence, never a world event or a source of
// character knowledge. Player-facing views omit internal NPC and source IDs.
type RPProviderCall struct {
	Phase        string `json:"phase"`
	ProviderKind string `json:"provider_kind"`
	ModelID      string `json:"model_id,omitempty"`
	Attempted    bool   `json:"attempted"`
	AttemptCount int    `json:"attempt_count"`
	Result       string `json:"result"`
	FallbackKind string `json:"fallback_kind,omitempty"`
	RenderSource string `json:"render_source,omitempty"`
	StartedAt    string `json:"started_at_utc"`
}

type rpProviderCallScope struct {
	SessionID   string
	TurnRunID   string
	SubjectID   string
	NPCEntityID string
	Phase       string
}

func rpProviderMetadata(provider any, fallback string) core.RPProviderMetadata {
	metadata := core.RPProviderMetadata{Kind: fallback}
	if source, ok := provider.(interface {
		ProviderMetadata() core.RPProviderMetadata
	}); ok {
		metadata = source.ProviderMetadata()
	}
	switch metadata.Kind {
	case "chat_completions", "full_prose", "style_planner", "deterministic", "not_configured":
	default:
		metadata.Kind = "custom"
	}
	if len(metadata.Model) > 200 || strings.ContainsAny(metadata.Model, "\r\n\x00") {
		metadata.Model = ""
	}
	return metadata
}

func (s *Store) beginRPProviderCall(ctx context.Context, scope rpProviderCallScope, metadata core.RPProviderMetadata) (string, error) {
	available, err := rpProviderReceiptsAvailable(ctx, s.db)
	if err != nil || !available {
		return "", err
	}
	id, err := newRPSessionID()
	if err != nil {
		return "", err
	}
	id = "rpp_" + strings.TrimPrefix(id, "rps_")
	var turnID any
	if scope.TurnRunID != "" {
		turnID = scope.TurnRunID
	}
	// A pending record proves only that the adapter is about to be invoked.
	// Until it settles, an interrupted process cannot prove whether HTTP ran.
	if err := execAgentOne(ctx, s.db, "begin RP provider receipt", `INSERT INTO rp_provider_calls(call_id,session_id,turn_run_id,subject_id,npc_entity_id,phase,provider_kind,model_id,attempted,result,started_at_utc) VALUES (?,?,?,?,?,?,?,?,0,'pending',?)`, id, scope.SessionID, turnID, scope.SubjectID, scope.NPCEntityID, scope.Phase, metadata.Kind, metadata.Model, s.now().UTC().Format(time.RFC3339Nano)); err != nil {
		return "", err
	}
	return id, nil
}

func (s *Store) finishRPProviderCall(ctx context.Context, id, result, fallback, source string, attemptCount int) error {
	if id == "" {
		return nil
	}
	if attemptCount < 0 || attemptCount > 100 {
		return core.NewError(core.CodeInvalidArgument, "invalid RP provider request count")
	}
	auditCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	return execAgentOne(auditCtx, s.db, "finish RP provider receipt", `UPDATE rp_provider_calls SET attempted=CASE WHEN ? > 0 THEN 1 ELSE 0 END,attempt_count=?,result=?,fallback_kind=?,render_source=?,finished_at_utc=? WHERE call_id=? AND result='pending'`, attemptCount, attemptCount, result, fallback, source, s.now().UTC().Format(time.RFC3339Nano), id)
}

func (s *Store) recordUnusedRPTurnDecision(ctx context.Context, runID, sessionID, playerTurnID string, provider core.RPDecisionProvider) error {
	available, err := rpProviderReceiptsAvailable(ctx, s.db)
	if err != nil || !available {
		return err
	}
	id, err := newRPSessionID()
	if err != nil {
		return err
	}
	metadata := rpProviderMetadata(provider, "custom")
	now := s.now().UTC().Format(time.RFC3339Nano)
	_, err = s.db.ExecContext(ctx, `INSERT INTO rp_provider_calls(call_id,session_id,turn_run_id,subject_id,phase,provider_kind,model_id,attempted,result,fallback_kind,started_at_utc,finished_at_utc)
		SELECT ?,?,?,?,'decision',?,?,0,'not_used','no_eligible_listener',?,?
		WHERE NOT EXISTS (SELECT 1 FROM rp_provider_calls WHERE session_id=? AND turn_run_id=? AND phase='decision')`, "rpp_"+strings.TrimPrefix(id, "rps_"), sessionID, runID, playerTurnID, metadata.Kind, metadata.Model, now, now, sessionID, runID)
	if err != nil {
		return core.WrapError(core.CodeStorageFailure, "record unused RP decision provider", err)
	}
	return nil
}

func rpProviderErrorResult(ctx context.Context, err error) string {
	if err == nil {
		return "success"
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) || strings.Contains(strings.ToLower(err.Error()), "timeout") {
		return "timeout"
	}
	return "failed"
}

type rpProviderCallQuerier interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func readRPTurnProviderCalls(ctx context.Context, q rpProviderCallQuerier, sessionID, turnRunID string) ([]RPProviderCall, error) {
	available, err := rpProviderReceiptsAvailable(ctx, q)
	if err != nil || !available {
		return nil, err
	}
	rows, err := q.QueryContext(ctx, `SELECT phase,provider_kind,model_id,attempted,attempt_count,result,fallback_kind,render_source,started_at_utc FROM rp_provider_calls WHERE session_id=? AND turn_run_id=? ORDER BY started_at_utc,rowid LIMIT 129`, sessionID, turnRunID)
	if err != nil {
		return nil, core.WrapError(core.CodeStorageFailure, "read scoped RP provider receipts", err)
	}
	defer rows.Close()
	calls := make([]RPProviderCall, 0)
	for rows.Next() {
		var call RPProviderCall
		var attempted int
		if err := rows.Scan(&call.Phase, &call.ProviderKind, &call.ModelID, &attempted, &call.AttemptCount, &call.Result, &call.FallbackKind, &call.RenderSource, &call.StartedAt); err != nil {
			return nil, core.WrapError(core.CodeStorageFailure, "scan RP provider receipt", err)
		}
		call.Attempted = attempted != 0
		calls = append(calls, call)
		if len(calls) > 128 {
			return nil, core.NewError(core.CodeStorageFailure, "RP provider receipt limit exceeded")
		}
	}
	if err := rows.Err(); err != nil {
		return nil, core.WrapError(core.CodeStorageFailure, "iterate RP provider receipts", err)
	}
	return calls, nil
}

func rpProviderReceiptsAvailable(ctx context.Context, q replayQuerier) (bool, error) {
	return rpTableAvailableBeforeMigration(ctx, q, "rp_provider_calls", RPProviderReceiptsSchemaVersion)
}
