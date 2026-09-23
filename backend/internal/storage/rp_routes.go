package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"corerp.local/backend/internal/core"
)

const (
	m2RPTravelCommandID = "cmd_m2_rp_travel_setup"
	m2RPTravelEventID   = "event_m2_rp_travel_setup"
)

type RPTravelSetupResult struct {
	EventID       string `json:"event_id"`
	EventSequence int64  `json:"event_sequence"`
	LinkCount     int    `json:"link_count"`
	Replayed      bool   `json:"replayed"`
}

type rpPlaceLink struct {
	From string `json:"from_place_id"`
	To   string `json:"to_place_id"`
}

func rpDemoLinks() []rpPlaceLink {
	links := make([]rpPlaceLink, 0, 8)
	for _, place := range []string{"place_m2_home_ada", "place_m2_home_bo", "place_m2_work_ada", "place_m2_work_bo"} {
		links = append(links, rpPlaceLink{M2AgentCafeID, place}, rpPlaceLink{place, M2AgentCafeID})
	}
	return links
}

// PrepareRPTravel declares the demo world's bounded route topology through
// an ordinary authoritative event. It does not duplicate anyone's location.
func (s *Store) PrepareRPTravel(ctx context.Context) (RPTravelSetupResult, error) {
	if _, err := s.BootstrapRPPlayDemo(ctx); err != nil {
		return RPTravelSetupResult{}, err
	}
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return RPTravelSetupResult{}, core.WrapError(core.CodeStorageFailure, "begin RP route setup", err)
	}
	defer tx.Rollback(ctx)
	var status string
	err = tx.conn.QueryRowContext(ctx, `SELECT status FROM commands WHERE command_id = ?`, m2RPTravelCommandID).Scan(&status)
	if err == nil {
		if status != "committed" {
			return RPTravelSetupResult{}, core.NewError(core.CodeCommandInProgress, "RP route setup is not committed")
		}
		var sequence int64
		if err := tx.conn.QueryRowContext(ctx, `SELECT event_sequence FROM events WHERE event_id = ?`, m2RPTravelEventID).Scan(&sequence); err != nil {
			return RPTravelSetupResult{}, core.WrapError(core.CodeStorageFailure, "load RP route setup", err)
		}
		return RPTravelSetupResult{EventID: m2RPTravelEventID, EventSequence: sequence, LinkCount: 8, Replayed: true}, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return RPTravelSetupResult{}, core.WrapError(core.CodeStorageFailure, "check RP route setup", err)
	}
	var head int64
	if err := tx.conn.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id = ? AND branch_id = ?`, M2DemoInstanceID, M2DemoBranchID).Scan(&head); err != nil {
		return RPTravelSetupResult{}, classifyMissing(err, "RP route branch")
	}
	if head != 7 {
		return RPTravelSetupResult{}, core.NewError(core.CodeBranchConflict, fmt.Sprintf("RP route setup requires head 7, got %d", head))
	}
	if err := ensureCohortTransitionChronology(ctx, tx.conn, M2DemoInstanceID, M2DemoBranchID, m2RPSetupTime); err != nil {
		return RPTravelSetupResult{}, err
	}
	sequence := head + 1
	var epochID string
	if err := tx.conn.QueryRowContext(ctx, `SELECT epoch_id FROM rule_epochs WHERE instance_id = ? AND branch_id = ? AND start_sequence <= ? AND (end_sequence IS NULL OR ? < end_sequence)`, M2DemoInstanceID, M2DemoBranchID, sequence, sequence).Scan(&epochID); err != nil {
		return RPTravelSetupResult{}, classifyMissing(err, "RP route Rule Epoch")
	}
	links := rpDemoLinks()
	payload := struct {
		Links []rpPlaceLink `json:"links"`
	}{links}
	payloadJSON, err := core.CanonicalJSON(payload)
	if err != nil {
		return RPTravelSetupResult{}, err
	}
	requestHash, err := core.HashJSON(payload)
	if err != nil {
		return RPTravelSetupResult{}, err
	}
	batchHash, err := core.HashJSON(struct {
		CommandID string `json:"command_id"`
		Sequence  int64  `json:"sequence"`
		WorldTime string `json:"world_time"`
		Payload   any    `json:"payload"`
	}{m2RPTravelCommandID, sequence, m2RPSetupTime, payload})
	if err != nil {
		return RPTravelSetupResult{}, err
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	attemptID := "attempt_m2_rp_travel_setup_1"
	batchID := "batch_m2_rp_travel_setup"
	statements := []struct {
		name, query string
		args        []any
	}{
		{"RP route command", `INSERT INTO commands(command_id, instance_id, branch_id, command_type, idempotency_key, request_hash, expected_head, principal_id, command_policy, status, created_at_utc) VALUES (?, ?, ?, 'DefineRPTravel', 'rp-travel-fixture-v1', ?, ?, 'principal_system', '{"authorization":"system-bootstrap"}', 'pending', ?)`, []any{m2RPTravelCommandID, M2DemoInstanceID, M2DemoBranchID, requestHash, head, now}},
		{"RP route attempt", `INSERT INTO command_attempts(command_id, attempt_no, attempt_id, status, lease_owner, lease_until_utc, proposal_hash, created_at_utc) VALUES (?, 1, ?, 'ready', 'corerp-rp1', ?, ?, ?)`, []any{m2RPTravelCommandID, attemptID, s.now().UTC().Add(30 * time.Second).Format(time.RFC3339Nano), requestHash, now}},
		{"RP route batch", `INSERT INTO event_batches(batch_id, command_id, attempt_no, instance_id, branch_id, epoch_id, expected_head, first_sequence, last_sequence, event_count, world_time, batch_hash, committed_at_utc) VALUES (?, ?, 1, ?, ?, ?, ?, ?, ?, 1, ?, ?, ?)`, []any{batchID, m2RPTravelCommandID, M2DemoInstanceID, M2DemoBranchID, epochID, head, sequence, sequence, m2RPSetupTime, batchHash, now}},
		{"RP route event", `INSERT INTO events(event_id, batch_id, instance_id, branch_id, event_sequence, batch_index, event_type, actor_id, world_time, payload) VALUES (?, ?, ?, ?, ?, 0, 'RPTravelDefined', 'system', ?, ?)`, []any{m2RPTravelEventID, batchID, M2DemoInstanceID, M2DemoBranchID, sequence, m2RPSetupTime, string(payloadJSON)}},
	}
	for _, statement := range statements {
		if err := execAgentOne(ctx, tx.conn, statement.name, statement.query, statement.args...); err != nil {
			return RPTravelSetupResult{}, err
		}
	}
	for i, link := range links {
		if err := execAgentOne(ctx, tx.conn, "insert RP place link", `INSERT INTO rp_place_links(link_id, instance_id, branch_id, from_place_id, to_place_id, definition_event_id) VALUES (?, ?, ?, ?, ?, ?)`, fmt.Sprintf("link_m2_rp_%02d", i+1), M2DemoInstanceID, M2DemoBranchID, link.From, link.To, m2RPTravelEventID); err != nil {
			return RPTravelSetupResult{}, err
		}
	}
	if err := execAgentOne(ctx, tx.conn, "advance RP route clock lineage", `UPDATE world_clocks SET projection_version = projection_version + 1, last_event_sequence = ? WHERE instance_id = ? AND branch_id = ? AND current_world_time = ?`, sequence, M2DemoInstanceID, M2DemoBranchID, m2RPSetupTime); err != nil {
		return RPTravelSetupResult{}, err
	}
	if err := s.insertAgentAudit(ctx, tx.conn, "audit_"+m2RPTravelCommandID, "agent_decision", m2RPTravelEventID, m2RPTravelCommandID, attemptID, m2RPSetupTime, now, payloadJSON); err != nil {
		return RPTravelSetupResult{}, err
	}
	if err := insertAgentOutbox(ctx, tx.conn, "outbox_m2_rp_travel_setup", m2RPTravelEventID, "rp.travel.defined", payloadJSON); err != nil {
		return RPTravelSetupResult{}, err
	}
	for _, statement := range []struct {
		name, query string
		args        []any
	}{
		{"advance RP route branch", `UPDATE branches SET head_sequence = ? WHERE instance_id = ? AND branch_id = ? AND head_sequence = ?`, []any{sequence, M2DemoInstanceID, M2DemoBranchID, head}},
		{"commit RP route attempt", `UPDATE command_attempts SET status = 'committed', finished_at_utc = ? WHERE command_id = ? AND attempt_no = 1 AND status = 'ready'`, []any{now, m2RPTravelCommandID}},
		{"commit RP route command", `UPDATE commands SET status = 'committed' WHERE command_id = ? AND status = 'pending'`, []any{m2RPTravelCommandID}},
	} {
		if err := execAgentOne(ctx, tx.conn, statement.name, statement.query, statement.args...); err != nil {
			return RPTravelSetupResult{}, err
		}
	}
	if s.beforeCommit != nil {
		if err := s.beforeCommit(); err != nil {
			return RPTravelSetupResult{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return RPTravelSetupResult{}, core.WrapError(core.CodeStorageFailure, "commit RP route setup", err)
	}
	return RPTravelSetupResult{EventID: m2RPTravelEventID, EventSequence: sequence, LinkCount: len(links)}, nil
}
