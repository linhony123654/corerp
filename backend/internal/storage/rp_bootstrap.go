package storage

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"corerp.local/backend/internal/core"
)

const (
	M2RPPlayerID        = "entity_m2_rp_lin"
	M2RPPlayerPrincipal = "principal_m2_rp_player"
	M2RPNPCID           = "entity_m2_rp_cai"
	M2RPNPCPrincipal    = "principal_m2_rp_cai"
	m2RPSetupCommandID  = "cmd_m2_rp_participants_setup"
	m2RPSetupEventID    = "event_m2_rp_participants_setup"
	m2RPSetupTime       = "2026-09-22T02:03:00Z"
)

type RPPlaySetupResult struct {
	EventID       string `json:"event_id"`
	EventSequence int64  `json:"event_sequence"`
	PlayerID      string `json:"player_id"`
	NPCCount      int    `json:"npc_count"`
	PlaceCount    int    `json:"place_count"`
	Replayed      bool   `json:"replayed"`
}

// BootstrapRPPlayDemo extends the existing M2/T09 world with one player and a
// third NPC. It never creates a separate RP character or spatial authority.
func (s *Store) BootstrapRPPlayDemo(ctx context.Context) (RPPlaySetupResult, error) {
	if _, err := s.BootstrapM2AgentDemo(ctx); err != nil {
		return RPPlaySetupResult{}, err
	}
	for _, command := range []core.MaterializeCohortCommand{
		m2AgentMaterialization("rp_cai", M2RPNPCID, "Cai", 1, 400, 3, 80, 60, 4, "2026-09-22T02:01:00Z"),
		m2AgentMaterialization("rp_lin", M2RPPlayerID, "Lin", 1, 300, 3, 60, 45, 5, "2026-09-22T02:02:00Z"),
	} {
		if _, err := s.MaterializeCohort(ctx, command); err != nil {
			return RPPlaySetupResult{}, err
		}
	}
	return s.setupRPParticipants(ctx)
}

func (s *Store) setupRPParticipants(ctx context.Context) (RPPlaySetupResult, error) {
	return s.setupRPParticipantsAt(ctx, m2RPSetupTime)
}

func (s *Store) setupRPParticipantsAt(ctx context.Context, setupTime string) (RPPlaySetupResult, error) {
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return RPPlaySetupResult{}, core.WrapError(core.CodeStorageFailure, "begin RP participants setup", err)
	}
	defer tx.Rollback(ctx)
	var status string
	err = tx.conn.QueryRowContext(ctx, `SELECT status FROM commands WHERE command_id = ?`, m2RPSetupCommandID).Scan(&status)
	if err == nil {
		if status != "committed" {
			return RPPlaySetupResult{}, core.NewError(core.CodeCommandInProgress, "RP participants setup is not committed")
		}
		var sequence int64
		if err := tx.conn.QueryRowContext(ctx, `SELECT event_sequence FROM events WHERE event_id = ?`, m2RPSetupEventID).Scan(&sequence); err != nil {
			return RPPlaySetupResult{}, core.WrapError(core.CodeStorageFailure, "load RP participants setup", err)
		}
		return RPPlaySetupResult{EventID: m2RPSetupEventID, EventSequence: sequence, PlayerID: M2RPPlayerID, NPCCount: 3, PlaceCount: 5, Replayed: true}, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return RPPlaySetupResult{}, core.WrapError(core.CodeStorageFailure, "check RP participants setup", err)
	}
	var head int64
	if err := tx.conn.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id = ? AND branch_id = ?`, M2DemoInstanceID, M2DemoBranchID).Scan(&head); err != nil {
		return RPPlaySetupResult{}, classifyMissing(err, "RP participants branch")
	}
	var participants int
	if err := tx.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM materialized_entities n JOIN cohorts c ON c.cohort_id=n.source_cohort_id WHERE n.entity_id IN (?,?) AND n.population_count=1 AND n.status='active' AND c.instance_id=? AND c.branch_id=?`, M2RPPlayerID, M2RPNPCID, M2DemoInstanceID, M2DemoBranchID).Scan(&participants); err != nil {
		return RPPlaySetupResult{}, err
	}
	if participants != 2 {
		return RPPlaySetupResult{}, core.NewError(core.CodeBranchConflict, "RP participants require actual player and NPC materializations")
	}
	if err := ensureCohortTransitionChronology(ctx, tx.conn, M2DemoInstanceID, M2DemoBranchID, setupTime); err != nil {
		return RPPlaySetupResult{}, err
	}
	sequence := head + 1
	var epochID string
	if err := tx.conn.QueryRowContext(ctx, `SELECT epoch_id FROM rule_epochs WHERE instance_id = ? AND branch_id = ? AND start_sequence <= ? AND (end_sequence IS NULL OR ? < end_sequence)`, M2DemoInstanceID, M2DemoBranchID, sequence, sequence).Scan(&epochID); err != nil {
		return RPPlaySetupResult{}, classifyMissing(err, "RP participants Rule Epoch")
	}
	payload := struct {
		PlayerEntityID string   `json:"player_entity_id"`
		NPCEntityIDs   []string `json:"npc_entity_ids"`
		InitialPlaceID string   `json:"initial_place_id"`
	}{M2RPPlayerID, []string{M2AgentAdaID, M2AgentBoID, M2RPNPCID}, M2AgentCafeID}
	payloadJSON, err := core.CanonicalJSON(payload)
	if err != nil {
		return RPPlaySetupResult{}, err
	}
	requestHash, err := core.HashJSON(payload)
	if err != nil {
		return RPPlaySetupResult{}, err
	}
	batchHash, err := core.HashJSON(struct {
		CommandID string `json:"command_id"`
		Sequence  int64  `json:"sequence"`
		WorldTime string `json:"world_time"`
		Payload   any    `json:"payload"`
	}{m2RPSetupCommandID, sequence, setupTime, payload})
	if err != nil {
		return RPPlaySetupResult{}, err
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	attemptID := "attempt_m2_rp_participants_setup_1"
	batchID := "batch_m2_rp_participants_setup"
	statements := []struct {
		name, query string
		args        []any
	}{
		{"RP setup command", `INSERT INTO commands(command_id, instance_id, branch_id, command_type, idempotency_key, request_hash, expected_head, principal_id, command_policy, status, created_at_utc) VALUES (?, ?, ?, 'InitializeRPPlayParticipants', 'rp-play-fixture-v1', ?, ?, 'principal_system', '{"authorization":"system-bootstrap"}', 'pending', ?)`, []any{m2RPSetupCommandID, M2DemoInstanceID, M2DemoBranchID, requestHash, head, now}},
		{"RP setup attempt", `INSERT INTO command_attempts(command_id, attempt_no, attempt_id, status, lease_owner, lease_until_utc, proposal_hash, created_at_utc) VALUES (?, 1, ?, 'ready', 'corerp-rp1', ?, ?, ?)`, []any{m2RPSetupCommandID, attemptID, s.now().UTC().Add(30 * time.Second).Format(time.RFC3339Nano), requestHash, now}},
		{"RP setup batch", `INSERT INTO event_batches(batch_id, command_id, attempt_no, instance_id, branch_id, epoch_id, expected_head, first_sequence, last_sequence, event_count, world_time, batch_hash, committed_at_utc) VALUES (?, ?, 1, ?, ?, ?, ?, ?, ?, 1, ?, ?, ?)`, []any{batchID, m2RPSetupCommandID, M2DemoInstanceID, M2DemoBranchID, epochID, head, sequence, sequence, setupTime, batchHash, now}},
		{"RP setup event", `INSERT INTO events(event_id, batch_id, instance_id, branch_id, event_sequence, batch_index, event_type, actor_id, world_time, payload) VALUES (?, ?, ?, ?, ?, 0, 'RPParticipantsInitialized', 'system', ?, ?)`, []any{m2RPSetupEventID, batchID, M2DemoInstanceID, M2DemoBranchID, sequence, setupTime, string(payloadJSON)}},
	}
	for _, statement := range statements {
		if err := execAgentOne(ctx, tx.conn, statement.name, statement.query, statement.args...); err != nil {
			return RPPlaySetupResult{}, err
		}
	}
	for _, principal := range []struct{ id, kind, name string }{
		{M2RPPlayerPrincipal, "player", "Lin Player"}, {M2RPNPCPrincipal, "agent", "Cai Agent"},
	} {
		if err := execAgentOne(ctx, tx.conn, "insert RP principal", `INSERT INTO principals(principal_id, principal_type, display_name, status) VALUES (?, ?, ?, 'active')`, principal.id, principal.kind, principal.name); err != nil {
			return RPPlaySetupResult{}, err
		}
	}
	if _, err := tx.conn.ExecContext(ctx, `INSERT OR IGNORE INTO capability_definitions(capability_id, description, policy_version) VALUES ('world.rp.control', 'Control an existing entity for RP', 'rp1-v1')`); err != nil {
		return RPPlaySetupResult{}, core.WrapError(core.CodeStorageFailure, "insert RP control capability", err)
	}
	for _, participant := range []struct{ id, principal, goal string }{
		{M2RPPlayerID, M2RPPlayerPrincipal, "player_controlled"},
		{M2RPNPCID, M2RPNPCPrincipal, "keep_daily_routine"},
	} {
		if err := execAgentOne(ctx, tx.conn, "insert RP spatial profile", `INSERT INTO agent_profiles(agent_id, instance_id, branch_id, principal_id, agent_level, goal_code, action_budget_per_day, status, definition_event_id) VALUES (?, ?, ?, ?, 'L2', ?, 8, 'active', ?)`, participant.id, M2DemoInstanceID, M2DemoBranchID, participant.principal, participant.goal, m2RPSetupEventID); err != nil {
			return RPPlaySetupResult{}, err
		}
		if err := execAgentOne(ctx, tx.conn, "insert RP initial movement", `INSERT INTO agent_movements(movement_id, event_id, agent_id, from_place_id, to_place_id, schedule_id, activity_code, world_time, movement_kind) VALUES (?, ?, ?, NULL, ?, NULL, 'present', ?, 'initialize')`, "movement_m2_rp_initialize_"+participant.id, m2RPSetupEventID, participant.id, M2AgentCafeID, setupTime); err != nil {
			return RPPlaySetupResult{}, err
		}
		if err := execAgentOne(ctx, tx.conn, "insert RP initial position", `INSERT INTO agent_positions(agent_id, place_id, activity_code, effective_world_time, projection_version, last_event_sequence) VALUES (?, ?, 'present', ?, 0, ?)`, participant.id, M2AgentCafeID, setupTime, sequence); err != nil {
			return RPPlaySetupResult{}, err
		}
	}
	if err := execAgentOne(ctx, tx.conn, "insert player control grant", `INSERT INTO capability_grants(grant_id, principal_id, capability_id, instance_id, branch_id, subject_id, field_scope, status, definition_event_id) VALUES ('grant_m2_rp_player_control', ?, 'world.rp.control', ?, ?, ?, '[]', 'active', ?)`, M2RPPlayerPrincipal, M2DemoInstanceID, M2DemoBranchID, M2RPPlayerID, m2RPSetupEventID); err != nil {
		return RPPlaySetupResult{}, err
	}
	if err := execAgentOne(ctx, tx.conn, "advance RP world clock", `UPDATE world_clocks SET current_world_time = ?, projection_version = projection_version + 1, last_event_sequence = ? WHERE instance_id = ? AND branch_id = ?`, setupTime, sequence, M2DemoInstanceID, M2DemoBranchID); err != nil {
		return RPPlaySetupResult{}, err
	}
	if err := s.insertAgentAudit(ctx, tx.conn, "audit_"+m2RPSetupCommandID, "agent_decision", m2RPSetupEventID, m2RPSetupCommandID, attemptID, setupTime, now, payloadJSON); err != nil {
		return RPPlaySetupResult{}, err
	}
	if err := insertAgentOutbox(ctx, tx.conn, "outbox_m2_rp_participants_setup", m2RPSetupEventID, "rp.participants.initialized", payloadJSON); err != nil {
		return RPPlaySetupResult{}, err
	}
	for _, statement := range []struct {
		name, query string
		args        []any
	}{
		{"advance RP branch", `UPDATE branches SET head_sequence = ? WHERE instance_id = ? AND branch_id = ? AND head_sequence = ?`, []any{sequence, M2DemoInstanceID, M2DemoBranchID, head}},
		{"commit RP attempt", `UPDATE command_attempts SET status = 'committed', finished_at_utc = ? WHERE command_id = ? AND attempt_no = 1 AND status = 'ready'`, []any{now, m2RPSetupCommandID}},
		{"commit RP command", `UPDATE commands SET status = 'committed' WHERE command_id = ? AND status = 'pending'`, []any{m2RPSetupCommandID}},
	} {
		if err := execAgentOne(ctx, tx.conn, statement.name, statement.query, statement.args...); err != nil {
			return RPPlaySetupResult{}, err
		}
	}
	if s.beforeCommit != nil {
		if err := s.beforeCommit(); err != nil {
			return RPPlaySetupResult{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return RPPlaySetupResult{}, core.WrapError(core.CodeStorageFailure, "commit RP participants setup", err)
	}
	return RPPlaySetupResult{EventID: m2RPSetupEventID, EventSequence: sequence, PlayerID: M2RPPlayerID, NPCCount: 3, PlaceCount: 5}, nil
}
