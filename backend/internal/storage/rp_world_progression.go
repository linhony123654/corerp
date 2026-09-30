package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"corerp.local/backend/internal/core"
)

type RPBackgroundProgressionRequest struct {
	WorkerID      string
	InstanceID    string
	BranchID      string
	LeaseDuration time.Duration
}

type RPBackgroundProgressionResult struct {
	Status           string `json:"status"`
	FromWorldTime    string `json:"from_world_time,omitempty"`
	TargetWorldTime  string `json:"target_world_time,omitempty"`
	CurrentWorldTime string `json:"current_world_time,omitempty"`
	ProcessedItems   int    `json:"processed_items,omitempty"`
	PendingDue       int64  `json:"pending_due,omitempty"`
}

type rpBackgroundAdvanceEvent struct {
	Version         string `json:"version"`
	Kind            string `json:"kind"`
	FromWorldTime   string `json:"from_world_time"`
	TargetWorldTime string `json:"target_world_time"`
	ProcessedItems  int    `json:"processed_items"`
	LeaseGeneration int64  `json:"lease_generation"`
}

type rpBackgroundClaim struct {
	runID, from, target string
	generation          int64
	budget              int
}

func (r RPBackgroundProgressionRequest) validate() error {
	if !studioID(r.InstanceID) || !studioID(r.BranchID) || strings.TrimSpace(r.WorkerID) == "" || len(r.WorkerID) > 160 || r.LeaseDuration < 5*time.Second || r.LeaseDuration > 10*time.Minute {
		return core.NewError(core.CodeInvalidArgument, "bounded background worker, scope and 5s-10m lease are required")
	}
	return nil
}

func (s *Store) RunRPBackgroundProgression(ctx context.Context, request RPBackgroundProgressionRequest) (result RPBackgroundProgressionResult, resultErr error) {
	if err := request.validate(); err != nil {
		return result, err
	}
	claim, immediate, err := s.claimRPBackgroundProgression(ctx, request)
	if err != nil || immediate.Status != "" {
		return immediate, err
	}
	result = RPBackgroundProgressionResult{Status: "running", FromWorldTime: claim.from, TargetWorldTime: claim.target, CurrentWorldTime: claim.from}
	defer func() {
		if resultErr != nil {
			cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
			defer cancel()
			_, _ = s.db.ExecContext(cleanup, `UPDATE rp_background_runs SET status='failed',error_code='runtime_error',finished_at_utc=? WHERE run_id=? AND status='running'`, s.now().UTC().Format(time.RFC3339Nano), claim.runID)
		}
	}()
	run, err := s.runAgentLifeForScope(ctx, request.InstanceID, request.BranchID, claim.target, claim.budget)
	if err != nil {
		return result, err
	}
	result.ProcessedItems, result.PendingDue, result.CurrentWorldTime = run.ProcessedItems, run.PendingDue, run.CurrentWorldTime
	if run.PendingDue > 0 {
		result.Status = "budget_exhausted"
		if err := s.finishRPBackgroundRun(ctx, request, claim, "budget_exhausted", run.ProcessedItems, ""); err != nil {
			return result, err
		}
		return result, nil
	}
	status, current, err := s.commitRPBackgroundAdvance(ctx, request, claim, run.ProcessedItems)
	if err != nil {
		return result, err
	}
	result.Status, result.CurrentWorldTime = status, current
	if status != "completed" {
		return result, nil
	}
	if err := s.settleRPActivities(ctx, request.InstanceID, request.BranchID); err != nil {
		return result, err
	}
	return result, nil
}

func (s *Store) claimRPBackgroundProgression(ctx context.Context, request RPBackgroundProgressionRequest) (rpBackgroundClaim, RPBackgroundProgressionResult, error) {
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return rpBackgroundClaim{}, RPBackgroundProgressionResult{}, core.WrapError(core.CodeStorageFailure, "begin background progression claim", err)
	}
	defer tx.Rollback(ctx)
	packages, err := readStudioActivePackages(ctx, tx.conn, request.InstanceID, request.BranchID)
	if err != nil {
		return rpBackgroundClaim{}, RPBackgroundProgressionResult{}, err
	}
	if packages == nil || packages.System.Content.SystemRules == nil || packages.System.Content.SystemRules.BackgroundProgression == nil || !packages.System.Content.SystemRules.BackgroundProgression.Enabled {
		return rpBackgroundClaim{}, RPBackgroundProgressionResult{Status: "disabled"}, nil
	}
	rules := packages.System.Content.SystemRules.BackgroundProgression
	now := s.now().UTC()
	var owner, leaseUntilText string
	var generation int64
	err = tx.conn.QueryRowContext(ctx, `SELECT lease_owner,generation,lease_until_utc FROM rp_background_leases WHERE instance_id=? AND branch_id=?`, request.InstanceID, request.BranchID).Scan(&owner, &generation, &leaseUntilText)
	if err == nil {
		leaseUntil, parseErr := time.Parse(time.RFC3339Nano, leaseUntilText)
		if parseErr != nil {
			return rpBackgroundClaim{}, RPBackgroundProgressionResult{}, core.NewError(core.CodeProjectionDiverged, "invalid background lease time")
		}
		if leaseUntil.After(now) {
			return rpBackgroundClaim{}, RPBackgroundProgressionResult{Status: "busy"}, nil
		}
		if _, err := tx.conn.ExecContext(ctx, `UPDATE rp_background_runs SET status='failed',error_code='lease_expired',finished_at_utc=? WHERE instance_id=? AND branch_id=? AND status='running'`, now.Format(time.RFC3339Nano), request.InstanceID, request.BranchID); err != nil {
			return rpBackgroundClaim{}, RPBackgroundProgressionResult{}, core.WrapError(core.CodeStorageFailure, "expire abandoned background run", err)
		}
		generation++
	} else if errors.Is(err, sql.ErrNoRows) {
		generation = 1
	} else {
		return rpBackgroundClaim{}, RPBackgroundProgressionResult{}, core.WrapError(core.CodeStorageFailure, "read background lease", err)
	}
	leaseUntilText = now.Add(request.LeaseDuration).Format(time.RFC3339Nano)
	if _, err := tx.conn.ExecContext(ctx, `INSERT INTO rp_background_leases(instance_id,branch_id,lease_owner,generation,lease_until_utc,updated_at_utc) VALUES (?,?,?,?,?,?) ON CONFLICT(instance_id,branch_id) DO UPDATE SET lease_owner=excluded.lease_owner,generation=excluded.generation,lease_until_utc=excluded.lease_until_utc,updated_at_utc=excluded.updated_at_utc`, request.InstanceID, request.BranchID, request.WorkerID, generation, leaseUntilText, now.Format(time.RFC3339Nano)); err != nil {
		return rpBackgroundClaim{}, RPBackgroundProgressionResult{}, core.WrapError(core.CodeStorageFailure, "claim background lease", err)
	}
	var current string
	if err := tx.conn.QueryRowContext(ctx, `SELECT current_world_time FROM world_clocks WHERE instance_id=? AND branch_id=?`, request.InstanceID, request.BranchID).Scan(&current); err != nil {
		return rpBackgroundClaim{}, RPBackgroundProgressionResult{}, classifyMissing(err, "background world clock")
	}
	from, err := time.Parse(time.RFC3339, current)
	if err != nil {
		return rpBackgroundClaim{}, RPBackgroundProgressionResult{}, core.NewError(core.CodeProjectionDiverged, "invalid background world time")
	}
	target := from.Add(time.Duration(rules.StepMinutes) * time.Minute).UTC().Format(time.RFC3339)
	var unfinishedTarget string
	err = tx.conn.QueryRowContext(ctx, `SELECT target_world_time FROM rp_background_runs WHERE instance_id=? AND branch_id=? AND status='budget_exhausted' AND target_world_time>? ORDER BY started_at_utc DESC LIMIT 1`, request.InstanceID, request.BranchID, current).Scan(&unfinishedTarget)
	if err == nil {
		target = unfinishedTarget
	} else if !errors.Is(err, sql.ErrNoRows) {
		return rpBackgroundClaim{}, RPBackgroundProgressionResult{}, core.WrapError(core.CodeStorageFailure, "read unfinished background target", err)
	}
	key, err := core.HashJSON([]any{"rp_background_progression", request.InstanceID, request.BranchID, generation, current, target})
	if err != nil {
		return rpBackgroundClaim{}, RPBackgroundProgressionResult{}, err
	}
	claim := rpBackgroundClaim{runID: "rp_background_run_" + key[7:], generation: generation, from: current, target: target, budget: rules.SchedulerBudget}
	controlled, err := rpBackgroundScopeControlled(ctx, tx.conn, request.InstanceID, request.BranchID)
	if err != nil {
		return rpBackgroundClaim{}, RPBackgroundProgressionResult{}, err
	}
	status := "running"
	finished := any(nil)
	if controlled {
		status, finished = "controlled", now.Format(time.RFC3339Nano)
	}
	if _, err := tx.conn.ExecContext(ctx, `INSERT INTO rp_background_runs(run_id,instance_id,branch_id,lease_generation,from_world_time,target_world_time,scheduler_budget,status,started_at_utc,finished_at_utc) VALUES (?,?,?,?,?,?,?,?,?,?)`, claim.runID, request.InstanceID, request.BranchID, generation, current, target, claim.budget, status, now.Format(time.RFC3339Nano), finished); err != nil {
		return rpBackgroundClaim{}, RPBackgroundProgressionResult{}, core.WrapError(core.CodeStorageFailure, "start background run", err)
	}
	if controlled {
		if _, err := tx.conn.ExecContext(ctx, `UPDATE rp_background_leases SET lease_until_utc=?,updated_at_utc=? WHERE instance_id=? AND branch_id=? AND generation=? AND lease_owner=?`, now.Add(time.Second).Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), request.InstanceID, request.BranchID, generation, request.WorkerID); err != nil {
			return rpBackgroundClaim{}, RPBackgroundProgressionResult{}, core.WrapError(core.CodeStorageFailure, "release controlled background lease", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return rpBackgroundClaim{}, RPBackgroundProgressionResult{}, core.WrapError(core.CodeStorageFailure, "commit background claim", err)
	}
	if controlled {
		return claim, RPBackgroundProgressionResult{Status: "controlled", FromWorldTime: current, TargetWorldTime: target, CurrentWorldTime: current}, nil
	}
	return claim, RPBackgroundProgressionResult{}, nil
}

func rpBackgroundScopeControlled(ctx context.Context, conn *sql.Conn, instance, branch string) (bool, error) {
	checks := []string{
		`SELECT COUNT(*) FROM rp_sessions WHERE instance_id=? AND branch_id=? AND status='active'`,
		`SELECT COUNT(*) FROM rp_controller_authorities WHERE instance_id=? AND branch_id=? AND status='active'`,
		`SELECT COUNT(*) FROM rp_turn_runs r JOIN rp_sessions s ON s.session_id=r.session_id WHERE s.instance_id=? AND s.branch_id=? AND r.status<>'settled'`,
		`SELECT COUNT(*) FROM rp_wait_intents i JOIN rp_sessions s ON s.session_id=i.session_id WHERE s.instance_id=? AND s.branch_id=? AND i.status='pending'`,
		`SELECT COUNT(*) FROM rp_shared_rounds WHERE instance_id=? AND branch_id=? AND status IN ('open','advancing')`,
	}
	for _, query := range checks {
		var count int
		if err := conn.QueryRowContext(ctx, query, instance, branch).Scan(&count); err != nil {
			return false, core.WrapError(core.CodeStorageFailure, "inspect background control boundary", err)
		}
		if count != 0 {
			return true, nil
		}
	}
	return false, nil
}

func (s *Store) requireNoRunningRPBackgroundProgression(ctx context.Context, conn *sql.Conn, instance, branch string) error {
	var available int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='rp_background_runs'`).Scan(&available); err != nil {
		return core.WrapError(core.CodeStorageFailure, "inspect background progression schema", err)
	}
	if available == 0 {
		return nil
	}
	var count int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_background_runs r JOIN rp_background_leases l ON l.instance_id=r.instance_id AND l.branch_id=r.branch_id AND l.generation=r.lease_generation WHERE r.instance_id=? AND r.branch_id=? AND r.status='running' AND l.lease_until_utc>?`, instance, branch, s.now().UTC().Format(time.RFC3339Nano)).Scan(&count); err != nil {
		return core.WrapError(core.CodeStorageFailure, "inspect running background progression", err)
	}
	if count != 0 {
		return core.NewError(core.CodeCommandInProgress, "background progression is finishing; retry control acquisition")
	}
	return nil
}

func (s *Store) commitRPBackgroundAdvance(ctx context.Context, request RPBackgroundProgressionRequest, claim rpBackgroundClaim, processed int) (string, string, error) {
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return "", "", core.WrapError(core.CodeStorageFailure, "begin background advance", err)
	}
	defer tx.Rollback(ctx)
	var owner, until string
	var generation int64
	if err := tx.conn.QueryRowContext(ctx, `SELECT lease_owner,generation,lease_until_utc FROM rp_background_leases WHERE instance_id=? AND branch_id=?`, request.InstanceID, request.BranchID).Scan(&owner, &generation, &until); err != nil {
		return "", "", classifyMissing(err, "background lease")
	}
	leaseUntil, err := time.Parse(time.RFC3339Nano, until)
	if err != nil || owner != request.WorkerID || generation != claim.generation || !leaseUntil.After(s.now().UTC()) {
		return "", "", core.NewError(core.CodeBranchConflict, "background lease was fenced")
	}
	controlled, err := rpBackgroundScopeControlled(ctx, tx.conn, request.InstanceID, request.BranchID)
	if err != nil {
		return "", "", err
	}
	if controlled {
		if err := execAgentOne(ctx, tx.conn, "stop controlled background run", `UPDATE rp_background_runs SET processed_items=?,status='controlled',finished_at_utc=? WHERE run_id=? AND status='running'`, processed, s.now().UTC().Format(time.RFC3339Nano), claim.runID); err != nil {
			return "", "", err
		}
		if _, err := tx.conn.ExecContext(ctx, `UPDATE rp_background_leases SET lease_until_utc=?,updated_at_utc=? WHERE instance_id=? AND branch_id=? AND generation=? AND lease_owner=?`, s.now().UTC().Add(time.Second).Format(time.RFC3339Nano), s.now().UTC().Format(time.RFC3339Nano), request.InstanceID, request.BranchID, claim.generation, request.WorkerID); err != nil {
			return "", "", core.WrapError(core.CodeStorageFailure, "release controlled background lease", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return "", "", err
		}
		return "controlled", claim.from, nil
	}
	var head, currentDay int64
	var current string
	if err := tx.conn.QueryRowContext(ctx, `SELECT b.head_sequence,c.current_world_time,c.current_day FROM branches b JOIN world_clocks c ON c.instance_id=b.instance_id AND c.branch_id=b.branch_id WHERE b.instance_id=? AND b.branch_id=?`, request.InstanceID, request.BranchID).Scan(&head, &current, &currentDay); err != nil {
		return "", "", classifyMissing(err, "background world")
	}
	if current > claim.target {
		return "", "", core.NewError(core.CodeBranchConflict, "background target was superseded")
	}
	if current < claim.target {
		if err := s.insertRPBackgroundAdvance(ctx, tx.conn, request, claim, processed, head, currentDay, current); err != nil {
			return "", "", err
		}
		current = claim.target
	}
	if err := execAgentOne(ctx, tx.conn, "complete background run", `UPDATE rp_background_runs SET processed_items=?,status='completed',finished_at_utc=? WHERE run_id=? AND status='running'`, processed, s.now().UTC().Format(time.RFC3339Nano), claim.runID); err != nil {
		return "", "", err
	}
	if _, err := tx.conn.ExecContext(ctx, `UPDATE rp_background_leases SET lease_until_utc=?,updated_at_utc=? WHERE instance_id=? AND branch_id=? AND generation=? AND lease_owner=?`, s.now().UTC().Add(time.Second).Format(time.RFC3339Nano), s.now().UTC().Format(time.RFC3339Nano), request.InstanceID, request.BranchID, claim.generation, request.WorkerID); err != nil {
		return "", "", core.WrapError(core.CodeStorageFailure, "release completed background lease", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return "", "", core.WrapError(core.CodeStorageFailure, "commit background advance", err)
	}
	return "completed", current, nil
}

func (s *Store) insertRPBackgroundAdvance(ctx context.Context, conn *sql.Conn, request RPBackgroundProgressionRequest, claim rpBackgroundClaim, processed int, head, currentDay int64, current string) error {
	sequence := head + 1
	var epoch string
	if err := conn.QueryRowContext(ctx, `SELECT epoch_id FROM rule_epochs WHERE instance_id=? AND branch_id=? AND start_sequence<=? AND (end_sequence IS NULL OR ?<end_sequence)`, request.InstanceID, request.BranchID, sequence, sequence).Scan(&epoch); err != nil {
		return classifyMissing(err, "background Rule Epoch")
	}
	payload := rpBackgroundAdvanceEvent{Version: "corerp.rp-background-progression.v1", Kind: "package_background_progression", FromWorldTime: current, TargetWorldTime: claim.target, ProcessedItems: processed, LeaseGeneration: claim.generation}
	payloadJSON, err := core.CanonicalJSON(payload)
	if err != nil {
		return err
	}
	hash, err := core.HashJSON(payload)
	if err != nil {
		return err
	}
	suffix := hash[7:]
	commandID, attemptID, batchID, eventID := "cmd_rp_background_"+suffix, "attempt_rp_background_"+suffix, "batch_rp_background_"+suffix, "event_rp_background_"+suffix
	batchHash, err := core.HashJSON(struct {
		Sequence int64                    `json:"sequence"`
		Payload  rpBackgroundAdvanceEvent `json:"payload"`
	}{sequence, payload})
	if err != nil {
		return err
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	statements := []struct {
		name, query string
		args        []any
	}{
		{"background command", `INSERT INTO commands(command_id,instance_id,branch_id,command_type,idempotency_key,request_hash,expected_head,principal_id,command_policy,status,created_at_utc) VALUES (?,?,?,'RPBackgroundProgression',?,?,?,?, '{"authorization":"package-background-progression"}','pending',?)`, []any{commandID, request.InstanceID, request.BranchID, claim.runID, hash, head, "principal_system", now}},
		{"background attempt", `INSERT INTO command_attempts(command_id,attempt_no,attempt_id,status,lease_owner,lease_until_utc,proposal_hash,created_at_utc) VALUES (?,1,?,'ready',?,?,?,?)`, []any{commandID, attemptID, request.WorkerID, s.now().UTC().Add(30 * time.Second).Format(time.RFC3339Nano), hash, now}},
		{"background batch", `INSERT INTO event_batches(batch_id,command_id,attempt_no,instance_id,branch_id,epoch_id,expected_head,first_sequence,last_sequence,event_count,world_time,batch_hash,committed_at_utc) VALUES (?,?,1,?,?,?,?,?,?,1,?,?,?)`, []any{batchID, commandID, request.InstanceID, request.BranchID, epoch, head, sequence, sequence, claim.target, batchHash, now}},
		{"background event", `INSERT INTO events(event_id,batch_id,instance_id,branch_id,event_sequence,batch_index,event_type,actor_id,world_time,payload) VALUES (?,?,?,?,?,0,'WorldTimeAdvanced','system',?,?)`, []any{eventID, batchID, request.InstanceID, request.BranchID, sequence, claim.target, string(payloadJSON)}},
	}
	for _, statement := range statements {
		if err := execAgentOne(ctx, conn, statement.name, statement.query, statement.args...); err != nil {
			return err
		}
	}
	target, _ := time.Parse(time.RFC3339, claim.target)
	base, _ := time.Parse(time.RFC3339, "2026-09-22T00:00:00Z")
	day := int64(target.Sub(base) / (24 * time.Hour))
	if day < currentDay {
		return core.NewError(core.CodeProjectionDiverged, "background progression would reduce world day")
	}
	if err := execAgentOne(ctx, conn, "advance background clock", `UPDATE world_clocks SET current_world_time=?,current_day=?,status='running',projection_version=projection_version+1,last_event_sequence=? WHERE instance_id=? AND branch_id=? AND current_world_time=?`, claim.target, day, sequence, request.InstanceID, request.BranchID, current); err != nil {
		return err
	}
	if err := execAgentOne(ctx, conn, "advance background branch", `UPDATE branches SET head_sequence=? WHERE instance_id=? AND branch_id=? AND head_sequence=?`, sequence, request.InstanceID, request.BranchID, head); err != nil {
		return err
	}
	if err := s.insertScopedAudit(ctx, conn, request.InstanceID, request.BranchID, "audit_"+commandID, "runtime_diagnostic", eventID, commandID, attemptID, claim.target, now, payloadJSON); err != nil {
		return err
	}
	if err := insertAgentOutbox(ctx, conn, "outbox_"+commandID, eventID, "rp.background.progressed", payloadJSON); err != nil {
		return err
	}
	if err := execAgentOne(ctx, conn, "commit background attempt", `UPDATE command_attempts SET status='committed',finished_at_utc=? WHERE command_id=? AND attempt_no=1 AND status='ready'`, now, commandID); err != nil {
		return err
	}
	return execAgentOne(ctx, conn, "commit background command", `UPDATE commands SET status='committed' WHERE command_id=? AND status='pending'`, commandID)
}

func (s *Store) finishRPBackgroundRun(ctx context.Context, request RPBackgroundProgressionRequest, claim rpBackgroundClaim, status string, processed int, errorCode string) error {
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return core.WrapError(core.CodeStorageFailure, "begin background run finish", err)
	}
	defer tx.Rollback(ctx)
	now := s.now().UTC()
	if err := execAgentOne(ctx, tx.conn, "finish background run", `UPDATE rp_background_runs SET processed_items=?,status=?,error_code=NULLIF(?,''),finished_at_utc=? WHERE run_id=? AND status='running'`, processed, status, errorCode, now.Format(time.RFC3339Nano), claim.runID); err != nil {
		return err
	}
	if _, err := tx.conn.ExecContext(ctx, `UPDATE rp_background_leases SET lease_until_utc=?,updated_at_utc=? WHERE instance_id=? AND branch_id=? AND generation=? AND lease_owner=?`, now.Add(time.Second).Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), request.InstanceID, request.BranchID, claim.generation, request.WorkerID); err != nil {
		return core.WrapError(core.CodeStorageFailure, "release background lease", err)
	}
	return tx.Commit(ctx)
}

func (s *Store) RunEligibleRPBackgroundProgression(ctx context.Context, workerID string, lease time.Duration, maxWorlds int) ([]RPBackgroundProgressionResult, error) {
	if strings.TrimSpace(workerID) == "" || maxWorlds < 1 || maxWorlds > 100 {
		return nil, core.NewError(core.CodeInvalidArgument, "bounded background worker scan required")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT b.instance_id,b.branch_id FROM branches b JOIN world_instances w ON w.instance_id=b.instance_id WHERE w.world_definition_id='corerp.studio.world' AND w.lifecycle_state='active' ORDER BY b.instance_id,b.branch_id LIMIT ?`, maxWorlds)
	if err != nil {
		return nil, core.WrapError(core.CodeStorageFailure, "list background worlds", err)
	}
	type scope struct{ instance, branch string }
	var scopes []scope
	for rows.Next() {
		var item scope
		if err := rows.Scan(&item.instance, &item.branch); err != nil {
			rows.Close()
			return nil, err
		}
		scopes = append(scopes, item)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	results := make([]RPBackgroundProgressionResult, 0, len(scopes))
	for _, item := range scopes {
		result, err := s.RunRPBackgroundProgression(ctx, RPBackgroundProgressionRequest{WorkerID: workerID, InstanceID: item.instance, BranchID: item.branch, LeaseDuration: lease})
		if err != nil {
			return results, fmt.Errorf("background world %s/%s: %w", item.instance, item.branch, err)
		}
		results = append(results, result)
	}
	return results, nil
}
