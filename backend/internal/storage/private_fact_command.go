package storage

import (
	"context"
	"corerp.local/backend/internal/core"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

// Shared private Event transaction; domain-specific authorization and typed
// preparation remain mandatory. No public outbox or implicit knowledge grant.
type privateFactDomain struct{ namespace, eventType, policy string }
type privateFactOptions struct {
	// Only the sourced controller handoff uses this exception. The pending
	// turn must already have committed speech heard by this Entity.
	pendingListenerID string
	// A command-specific check for reading its own committed receipt. Fresh
	// commands still run the ordinary authorizer after the exact-key lookup.
	replayAuthorize func(*sql.Conn) error
}
type privateFactContext struct {
	EventID   string
	Sequence  int64
	WorldTime string
}
type privateFactRecord[T any] struct {
	EventID       string `json:"event_id"`
	EventSequence int64  `json:"event_sequence"`
	WorldTime     string `json:"world_time"`
	Fact          T      `json:"fact"`
	Replayed      bool   `json:"replayed"`
}

func executePrivateFactCommand[T any](s *Store, ctx context.Context, b core.CareerBinding, commandType string, request any, domain privateFactDomain, authorize func(*sql.Conn) error, prepare func(*sql.Conn, privateFactContext) (T, func() error, error)) (privateFactRecord[T], error) {
	return executePrivateFactCommandWithOptions(s, ctx, b, commandType, request, domain, privateFactOptions{}, authorize, prepare)
}

func executePrivateFactCommandWithOptions[T any](s *Store, ctx context.Context, b core.CareerBinding, commandType string, request any, domain privateFactDomain, options privateFactOptions, authorize func(*sql.Conn) error, prepare func(*sql.Conn, privateFactContext) (T, func() error, error)) (privateFactRecord[T], error) {
	var empty privateFactRecord[T]
	if err := b.Validate(); err != nil {
		return empty, err
	}
	hash, err := core.HashJSON(request)
	if err != nil {
		return empty, err
	}
	key, err := core.HashJSON([]string{b.PrincipalID, b.IdempotencyKey})
	if err != nil {
		return empty, err
	}
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return empty, err
	}
	defer tx.Rollback(ctx)
	initialAuthorize := authorize
	if options.replayAuthorize != nil {
		initialAuthorize = options.replayAuthorize
	}
	if err := initialAuthorize(tx.conn); err != nil {
		return empty, err
	}
	var previous privateFactRecord[T]
	var oldHash, raw string
	err = tx.conn.QueryRowContext(ctx, `SELECT c.request_hash,e.event_id,e.event_sequence,e.world_time,e.payload FROM commands c JOIN event_batches b ON b.command_id=c.command_id JOIN events e ON e.batch_id=b.batch_id
	 WHERE c.instance_id=? AND c.branch_id=? AND c.command_type=? AND c.idempotency_key=? AND c.status='committed'`, b.InstanceID, b.BranchID, commandType, key).Scan(&oldHash, &previous.EventID, &previous.EventSequence, &previous.WorldTime, &raw)
	if err == nil {
		if oldHash != hash {
			return empty, core.NewError(core.CodeIdempotencyMismatch, "career retry differs")
		}
		if err := json.Unmarshal([]byte(raw), &previous.Fact); err != nil {
			return empty, err
		}
		previous.Replayed = true
		return previous, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return empty, err
	}
	if options.replayAuthorize != nil {
		if err := authorize(tx.conn); err != nil {
			return empty, err
		}
	}
	var head int64
	var worldTime, epoch string
	if err := tx.conn.QueryRowContext(ctx, `SELECT b.head_sequence,c.current_world_time FROM branches b JOIN world_clocks c ON c.instance_id=b.instance_id AND c.branch_id=b.branch_id WHERE b.instance_id=? AND b.branch_id=?`, b.InstanceID, b.BranchID).Scan(&head, &worldTime); err != nil {
		return empty, classifyMissing(err, "career world")
	}
	if head != b.ExpectedHead {
		return empty, core.NewError(core.CodeBranchConflict, "career expected head differs")
	}
	var pending int
	if err := tx.conn.QueryRowContext(ctx, `SELECT (SELECT COUNT(*) FROM rp_turn_runs t JOIN rp_sessions s ON s.session_id=t.session_id WHERE s.instance_id=? AND s.branch_id=? AND t.status<>'settled')+(SELECT COUNT(*) FROM rp_wait_intents w JOIN rp_sessions s ON s.session_id=w.session_id WHERE s.instance_id=? AND s.branch_id=? AND w.status='pending')`, b.InstanceID, b.BranchID, b.InstanceID, b.BranchID).Scan(&pending); err != nil {
		return empty, err
	}
	if pending != 0 {
		if options.pendingListenerID == "" {
			return empty, core.NewError(core.CodeCommandInProgress, "finish active RP action before career command")
		}
		var waits, turns, eligible int
		if err := tx.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_wait_intents w JOIN rp_sessions s ON s.session_id=w.session_id WHERE s.instance_id=? AND s.branch_id=? AND w.status='pending'`, b.InstanceID, b.BranchID).Scan(&waits); err != nil {
			return empty, err
		}
		if err := tx.conn.QueryRowContext(ctx, `SELECT COUNT(*),COALESCE(SUM(CASE WHEN t.player_event_id IS NOT NULL AND EXISTS(SELECT 1 FROM json_each(t.listener_ids_json) WHERE value=?) THEN 1 ELSE 0 END),0) FROM rp_turn_runs t JOIN rp_sessions s ON s.session_id=t.session_id WHERE s.instance_id=? AND s.branch_id=? AND t.status<>'settled'`, options.pendingListenerID, b.InstanceID, b.BranchID).Scan(&turns, &eligible); err != nil {
			return empty, err
		}
		if waits != 0 || turns != 1 || eligible != 1 {
			return empty, core.NewError(core.CodeCommandInProgress, "controller handoff requires one committed heard turn and no pending wait")
		}
	}
	if err := ensureCohortTransitionChronology(ctx, tx.conn, b.InstanceID, b.BranchID, worldTime); err != nil {
		return empty, err
	}
	id, err := core.HashJSON([]string{b.InstanceID, b.BranchID, commandType, key})
	if err != nil {
		return empty, err
	}
	suffix := id[7:]
	commandID, batchID, attemptID := "cmd_"+domain.namespace+"_"+suffix, "batch_"+domain.namespace+"_"+suffix, "attempt_"+domain.namespace+"_"+suffix
	c := privateFactContext{EventID: "event_" + domain.namespace + "_" + suffix, Sequence: head + 1, WorldTime: worldTime}
	fact, apply, err := prepare(tx.conn, c)
	if err != nil {
		return empty, err
	}
	// Domain preparation supplies the immutable versioned fact.
	encoded, err := core.CanonicalJSON(fact)
	if err != nil {
		return empty, err
	}
	batchHash, err := core.HashJSON(struct {
		RequestHash string `json:"request_hash"`
		Sequence    int64  `json:"sequence"`
		WorldTime   string `json:"world_time"`
		Fact        T      `json:"fact"`
	}{hash, c.Sequence, worldTime, fact})
	if err != nil {
		return empty, err
	}
	if err := tx.conn.QueryRowContext(ctx, `SELECT epoch_id FROM rule_epochs WHERE instance_id=? AND branch_id=? AND start_sequence<=? AND (end_sequence IS NULL OR ?<end_sequence)`, b.InstanceID, b.BranchID, c.Sequence, c.Sequence).Scan(&epoch); err != nil {
		return empty, err
	}
	now := s.now().UTC()
	nowText := now.Format(time.RFC3339Nano)
	for _, st := range []struct {
		query string
		args  []any
	}{
		{`INSERT INTO commands(command_id,instance_id,branch_id,command_type,idempotency_key,request_hash,expected_head,principal_id,command_policy,status,created_at_utc) VALUES (?,?,?,?,?,?,?,?, ?,'pending',?)`, []any{commandID, b.InstanceID, b.BranchID, commandType, key, hash, head, b.PrincipalID, domain.policy, nowText}},
		{`INSERT INTO command_attempts(command_id,attempt_no,attempt_id,status,lease_owner,lease_until_utc,proposal_hash,created_at_utc) VALUES (?,1,?,'ready',?,?,?,?)`, []any{commandID, attemptID, "corerp-" + domain.namespace, now.Add(30 * time.Second).Format(time.RFC3339Nano), hash, nowText}},
		{`INSERT INTO event_batches(batch_id,command_id,attempt_no,instance_id,branch_id,epoch_id,expected_head,first_sequence,last_sequence,event_count,world_time,batch_hash,committed_at_utc) VALUES (?,?,1,?,?,?,?,?,?,1,?,?,?)`, []any{batchID, commandID, b.InstanceID, b.BranchID, epoch, head, c.Sequence, c.Sequence, worldTime, batchHash, nowText}},
		{`INSERT INTO events(event_id,batch_id,instance_id,branch_id,event_sequence,batch_index,event_type,actor_id,world_time,payload) VALUES (?,?,?,?,?,0,?,?,?,?)`, []any{c.EventID, batchID, b.InstanceID, b.BranchID, c.Sequence, domain.eventType, b.PrincipalID, worldTime, string(encoded)}},
	} {
		if _, err := tx.conn.ExecContext(ctx, st.query, st.args...); err != nil {
			return empty, core.WrapError(core.CodeStorageFailure, "record career command", err)
		}
	}
	if apply != nil {
		if err := apply(); err != nil {
			return empty, err
		}
	}
	auditType := "agent_decision"
	if commandType == "ConfigureStudioAccessLocal" || commandType == "PrepareRPFinalCohortLocal" || commandType == "EnrollRPExternalControllerLocal" || commandType == "AssignRPExternalControllerLocal" || commandType == "ReleaseRPExternalControllerLocal" || commandType == "ReplaceRPExternalControllerLocal" {
		auditType = "runtime_diagnostic"
	}
	if err := s.insertScopedAudit(ctx, tx.conn, b.InstanceID, b.BranchID, "audit_"+commandID, auditType, c.EventID, commandID, attemptID, worldTime, nowText, encoded); err != nil {
		return empty, err
	}
	// No public Outbox: candidates' statements/assessments are not world facts
	// known to all clients. Public discovery is a separate filtered read.
	for _, st := range []struct {
		query string
		args  []any
	}{
		{`UPDATE world_clocks SET projection_version=projection_version+1,last_event_sequence=? WHERE instance_id=? AND branch_id=? AND current_world_time=?`, []any{c.Sequence, b.InstanceID, b.BranchID, worldTime}},
		{`UPDATE branches SET head_sequence=? WHERE instance_id=? AND branch_id=? AND head_sequence=?`, []any{c.Sequence, b.InstanceID, b.BranchID, head}},
		{`UPDATE commands SET status='committed' WHERE command_id=? AND status='pending'`, []any{commandID}},
		{`UPDATE command_attempts SET status='committed',finished_at_utc=? WHERE attempt_id=? AND status='ready'`, []any{nowText, attemptID}},
	} {
		if err := execAgentOne(ctx, tx.conn, "career compare-and-swap", st.query, st.args...); err != nil {
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
	return privateFactRecord[T]{EventID: c.EventID, EventSequence: c.Sequence, WorldTime: worldTime, Fact: fact}, nil
}
