package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"corerp.local/backend/internal/core"
)

const (
	M2AgentAdaID        = "entity_m2_agent_ada"
	M2AgentBoID         = "entity_m2_agent_bo"
	M2AgentAdaPrincipal = "principal_m2_agent_ada"
	M2AgentBoPrincipal  = "principal_m2_agent_bo"
	M2AgentCafeID       = "place_m2_cafe"
	M2AgentSetupTime    = "2026-09-22T02:00:00Z"
	M2AgentMorningTime  = "2026-09-23T08:00:00Z"
	M2AgentNoonTime     = "2026-09-23T12:00:00Z"
)

const (
	m2AgentSetupCommandID = "cmd_m2_agent_life_setup"
	m2AgentSetupEventID   = "event_m2_agent_life_setup"
	m2AgentPhaseID        = "m2_agent_location"
)

type AgentSetupResult struct {
	CommandID     string `json:"command_id"`
	EventID       string `json:"event_id"`
	EventSequence int64  `json:"event_sequence"`
	AgentCount    int    `json:"agent_count"`
	ScheduleCount int    `json:"schedule_count"`
	Replayed      bool   `json:"replayed"`
}

type AgentLifeRunResult struct {
	RunID            string `json:"run_id"`
	TargetWorldTime  string `json:"target_world_time"`
	CurrentWorldTime string `json:"current_world_time"`
	ProcessedItems   int    `json:"processed_items"`
	PendingDue       int64  `json:"pending_due"`
	Status           string `json:"status"`
	HeadSequence     int64  `json:"head_sequence"`
}

type agentSchedulePayload struct {
	Kind         string `json:"kind"`
	Day          int    `json:"day"`
	AgentID      string `json:"agent_id"`
	ScheduleID   string `json:"schedule_id"`
	ToPlaceID    string `json:"to_place_id"`
	ActivityCode string `json:"activity_code"`
}

type agentSetupPayload struct {
	AgentIDs    []string `json:"agent_ids"`
	PlaceIDs    []string `json:"place_ids"`
	ScheduleIDs []string `json:"schedule_ids"`
}

type agentMovementPayload struct {
	AgentID           string   `json:"agent_id"`
	ScheduleID        string   `json:"schedule_id"`
	FromPlaceID       string   `json:"from_place_id"`
	ToPlaceID         string   `json:"to_place_id"`
	ActivityCode      string   `json:"activity_code"`
	CoLocatedAgentIDs []string `json:"co_located_agent_ids"`
}

func (s *Store) BootstrapM2AgentDemo(ctx context.Context) (AgentSetupResult, error) {
	if err := s.BootstrapM2Demo(ctx); err != nil {
		return AgentSetupResult{}, err
	}
	commands := []core.MaterializeCohortCommand{
		m2AgentMaterialization("ada", M2AgentAdaID, "Ada", 1, 500, 5, 100, 75, 1, "2026-09-22T01:00:00Z"),
		m2AgentMaterialization("bo", M2AgentBoID, "Bo", 1, 600, 6, 120, 90, 2, "2026-09-22T01:01:00Z"),
	}
	for _, command := range commands {
		if _, err := s.MaterializeCohort(ctx, command); err != nil {
			return AgentSetupResult{}, err
		}
	}
	return s.setupM2AgentLife(ctx)
}

func m2AgentMaterialization(name, entityID, displayName string, population, asset, inventory, receivable, liability, expectedHead int64, worldTime string) core.MaterializeCohortCommand {
	materializationID := "mat_m2_agent_" + name
	return core.MaterializeCohortCommand{
		CommandID: "cmd_" + materializationID, MaterializationID: materializationID,
		InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID,
		PrincipalID: "principal_creator", CapabilityID: "world.cohort.materialize",
		IdempotencyKey: "idem_" + materializationID, ExpectedHead: expectedHead, WorldTime: worldTime,
		SourceCohortID: M2DemoCohortID, EntityID: entityID, DisplayName: displayName,
		PopulationCount: population, AssetMinor: asset, InventoryMinor: inventory,
		ReceivableMinor: receivable, LiabilityMinor: liability,
		AllocationAlgorithmVersion: "equal-share-v1",
	}
}

func (s *Store) setupM2AgentLife(ctx context.Context) (AgentSetupResult, error) {
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return AgentSetupResult{}, core.WrapError(core.CodeStorageFailure, "begin M2 Agent setup", err)
	}
	defer tx.Rollback(ctx)

	var status string
	err = tx.conn.QueryRowContext(ctx, `SELECT status FROM commands WHERE command_id = ?`, m2AgentSetupCommandID).Scan(&status)
	if err == nil {
		if status != "committed" {
			return AgentSetupResult{}, core.NewError(core.CodeCommandInProgress, "M2 Agent setup command is not committed")
		}
		var sequence int64
		if err := tx.conn.QueryRowContext(ctx, `SELECT event_sequence FROM events WHERE event_id = ?`, m2AgentSetupEventID).Scan(&sequence); err != nil {
			return AgentSetupResult{}, core.WrapError(core.CodeStorageFailure, "load committed M2 Agent setup", err)
		}
		return AgentSetupResult{CommandID: m2AgentSetupCommandID, EventID: m2AgentSetupEventID, EventSequence: sequence, AgentCount: 2, ScheduleCount: 4, Replayed: true}, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return AgentSetupResult{}, core.WrapError(core.CodeStorageFailure, "check M2 Agent setup", err)
	}

	var head int64
	if err := tx.conn.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id = ? AND branch_id = ?`, M2DemoInstanceID, M2DemoBranchID).Scan(&head); err != nil {
		return AgentSetupResult{}, classifyMissing(err, "M2 Agent branch")
	}
	if head != 3 {
		return AgentSetupResult{}, core.NewError(core.CodeBranchConflict, fmt.Sprintf("M2 Agent setup requires head 3 after two materializations, current head %d", head))
	}
	for _, agentID := range []string{M2AgentAdaID, M2AgentBoID} {
		var count int
		if err := tx.conn.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM materialized_entities e JOIN cohorts c ON c.cohort_id = e.source_cohort_id
			WHERE e.entity_id = ? AND e.status = 'active' AND e.population_count = 1
			  AND c.instance_id = ? AND c.branch_id = ?`, agentID, M2DemoInstanceID, M2DemoBranchID).Scan(&count); err != nil {
			return AgentSetupResult{}, core.WrapError(core.CodeStorageFailure, "validate materialized Agent", err)
		}
		if count != 1 {
			return AgentSetupResult{}, core.NewError(core.CodeConservationFailed, "Agent setup requires an active one-person materialized entity: "+agentID)
		}
	}
	sequence := head + 1
	var epochID, rulesetHash string
	if err := tx.conn.QueryRowContext(ctx, `SELECT epoch_id, ruleset_hash FROM rule_epochs WHERE instance_id = ? AND branch_id = ? AND start_sequence <= ? AND (end_sequence IS NULL OR ? < end_sequence)`, M2DemoInstanceID, M2DemoBranchID, sequence, sequence).Scan(&epochID, &rulesetHash); err != nil {
		return AgentSetupResult{}, classifyMissing(err, "M2 Agent Rule Epoch")
	}

	places := []struct{ id, name, kind string }{
		{"place_m2_home_ada", "Ada Home", "home"},
		{"place_m2_home_bo", "Bo Home", "home"},
		{"place_m2_work_ada", "Ada Workplace", "work"},
		{"place_m2_work_bo", "Bo Workplace", "work"},
		{M2AgentCafeID, "M2 Cafe", "public"},
	}
	schedules := []struct {
		id, itemID, agentID, worldTime, placeID, activity string
		priority                                          int64
	}{
		{"schedule_ada_work", "sched_m2_20260923_0800_ada_work", M2AgentAdaID, M2AgentMorningTime, "place_m2_work_ada", "work", 10},
		{"schedule_bo_work", "sched_m2_20260923_0800_bo_work", M2AgentBoID, M2AgentMorningTime, "place_m2_work_bo", "work", 10},
		{"schedule_ada_cafe", "sched_m2_20260923_1200_ada_cafe", M2AgentAdaID, M2AgentNoonTime, M2AgentCafeID, "lunch", 20},
		{"schedule_bo_cafe", "sched_m2_20260923_1200_bo_cafe", M2AgentBoID, M2AgentNoonTime, M2AgentCafeID, "lunch", 20},
	}
	if s.reverseAgentSeed {
		for left, right := 0, len(schedules)-1; left < right; left, right = left+1, right-1 {
			schedules[left], schedules[right] = schedules[right], schedules[left]
		}
	}
	payload := agentSetupPayload{
		AgentIDs:    []string{M2AgentAdaID, M2AgentBoID},
		PlaceIDs:    []string{"place_m2_home_ada", "place_m2_home_bo", "place_m2_work_ada", "place_m2_work_bo", M2AgentCafeID},
		ScheduleIDs: []string{"schedule_ada_work", "schedule_bo_work", "schedule_ada_cafe", "schedule_bo_cafe"},
	}
	payloadJSON, err := core.CanonicalJSON(payload)
	if err != nil {
		return AgentSetupResult{}, err
	}
	requestHash, err := core.HashJSON(struct {
		CommandType string            `json:"command_type"`
		InstanceID  string            `json:"instance_id"`
		BranchID    string            `json:"branch_id"`
		Payload     agentSetupPayload `json:"payload"`
	}{"InitializeM2AgentLife", M2DemoInstanceID, M2DemoBranchID, payload})
	if err != nil {
		return AgentSetupResult{}, err
	}
	batchHash, err := core.HashJSON(struct {
		CommandID string            `json:"command_id"`
		Sequence  int64             `json:"sequence"`
		WorldTime string            `json:"world_time"`
		EventType string            `json:"event_type"`
		Payload   agentSetupPayload `json:"payload"`
	}{m2AgentSetupCommandID, sequence, M2AgentSetupTime, "AgentLifeInitialized", payload})
	if err != nil {
		return AgentSetupResult{}, err
	}
	nowText := s.now().UTC().Format(time.RFC3339Nano)
	attemptID := "attempt_m2_agent_life_setup_1"
	batchID := "batch_m2_agent_life_setup"

	baseStatements := []struct {
		name, query string
		args        []any
	}{
		{"Agent setup command", `INSERT INTO commands(command_id, instance_id, branch_id, command_type, idempotency_key, request_hash, expected_head, principal_id, command_policy, status, created_at_utc) VALUES (?, ?, ?, 'InitializeM2AgentLife', 'm2-agent-life-v1', ?, ?, 'principal_system', '{"authorization":"system-bootstrap"}', 'pending', ?)`, []any{m2AgentSetupCommandID, M2DemoInstanceID, M2DemoBranchID, requestHash, head, nowText}},
		{"Agent setup attempt", `INSERT INTO command_attempts(command_id, attempt_no, attempt_id, status, lease_owner, lease_until_utc, proposal_hash, created_at_utc) VALUES (?, 1, ?, 'ready', 'corerp-m2-agent', ?, ?, ?)`, []any{m2AgentSetupCommandID, attemptID, s.now().UTC().Add(30 * time.Second).Format(time.RFC3339Nano), requestHash, nowText}},
		{"Agent setup batch", `INSERT INTO event_batches(batch_id, command_id, attempt_no, instance_id, branch_id, epoch_id, expected_head, first_sequence, last_sequence, event_count, world_time, batch_hash, committed_at_utc) VALUES (?, ?, 1, ?, ?, ?, ?, ?, ?, 1, ?, ?, ?)`, []any{batchID, m2AgentSetupCommandID, M2DemoInstanceID, M2DemoBranchID, epochID, head, sequence, sequence, M2AgentSetupTime, batchHash, nowText}},
		{"Agent setup event", `INSERT INTO events(event_id, batch_id, instance_id, branch_id, event_sequence, batch_index, event_type, actor_id, world_time, payload) VALUES (?, ?, ?, ?, ?, 0, 'AgentLifeInitialized', 'system', ?, ?)`, []any{m2AgentSetupEventID, batchID, M2DemoInstanceID, M2DemoBranchID, sequence, M2AgentSetupTime, string(payloadJSON)}},
	}
	for _, statement := range baseStatements {
		if err := execAgentOne(ctx, tx.conn, statement.name, statement.query, statement.args...); err != nil {
			return AgentSetupResult{}, err
		}
	}

	for _, principal := range []struct{ id, name string }{{M2AgentAdaPrincipal, "Ada Agent"}, {M2AgentBoPrincipal, "Bo Agent"}} {
		if _, err := tx.conn.ExecContext(ctx, `INSERT OR IGNORE INTO principals(principal_id, principal_type, display_name, status) VALUES (?, 'agent', ?, 'active')`, principal.id, principal.name); err != nil {
			return AgentSetupResult{}, core.WrapError(core.CodeStorageFailure, "seed Agent principal", err)
		}
	}
	capabilities := []struct{ id, description string }{
		{"world.agent.run", "Advance bounded Agent schedules"},
		{"world.agent.knowledge.read", "Read scoped Agent knowledge"},
		{"world.encounter.read", "Resolve a scoped state-backed encounter"},
	}
	for _, capability := range capabilities {
		if _, err := tx.conn.ExecContext(ctx, `INSERT OR IGNORE INTO capability_definitions(capability_id, description, policy_version) VALUES (?, ?, 'm2-agent-v1')`, capability.id, capability.description); err != nil {
			return AgentSetupResult{}, core.WrapError(core.CodeStorageFailure, "seed Agent capability", err)
		}
	}
	for _, place := range places {
		if err := execAgentOne(ctx, tx.conn, "insert Agent place", `INSERT INTO agent_places(place_id, instance_id, branch_id, display_name, place_kind, status, definition_event_id) VALUES (?, ?, ?, ?, ?, 'active', ?)`, place.id, M2DemoInstanceID, M2DemoBranchID, place.name, place.kind, m2AgentSetupEventID); err != nil {
			return AgentSetupResult{}, err
		}
	}
	profiles := []struct{ agentID, principalID, goal string }{
		{M2AgentAdaID, M2AgentAdaPrincipal, "maintain_daily_routine"},
		{M2AgentBoID, M2AgentBoPrincipal, "maintain_daily_routine"},
	}
	for _, profile := range profiles {
		if err := execAgentOne(ctx, tx.conn, "insert Agent profile", `INSERT INTO agent_profiles(agent_id, instance_id, branch_id, principal_id, agent_level, goal_code, action_budget_per_day, status, definition_event_id) VALUES (?, ?, ?, ?, 'L2', ?, 8, 'active', ?)`, profile.agentID, M2DemoInstanceID, M2DemoBranchID, profile.principalID, profile.goal, m2AgentSetupEventID); err != nil {
			return AgentSetupResult{}, err
		}
	}
	if _, err := tx.conn.ExecContext(ctx, `INSERT OR IGNORE INTO scheduler_phases(phase_id, description, ruleset_hash) VALUES (?, 'M2 Agent spatial transitions', ?)`, m2AgentPhaseID, rulesetHash); err != nil {
		return AgentSetupResult{}, core.WrapError(core.CodeStorageFailure, "seed Agent scheduler phase", err)
	}
	for _, schedule := range schedules {
		day := 1
		itemPayload, err := core.CanonicalJSON(agentSchedulePayload{"agent_move", day, schedule.agentID, schedule.id, schedule.placeID, schedule.activity})
		if err != nil {
			return AgentSetupResult{}, err
		}
		if err := execAgentOne(ctx, tx.conn, "insert Agent scheduler item", `INSERT INTO scheduler_items(scheduler_item_id, instance_id, branch_id, world_time, phase_id, declared_priority, status, payload) VALUES (?, ?, ?, ?, ?, ?, 'pending', ?)`, schedule.itemID, M2DemoInstanceID, M2DemoBranchID, schedule.worldTime, m2AgentPhaseID, schedule.priority, string(itemPayload)); err != nil {
			return AgentSetupResult{}, err
		}
		if err := execAgentOne(ctx, tx.conn, "insert Agent schedule", `INSERT INTO agent_schedule_entries(schedule_id, agent_id, world_time, place_id, activity_code, declared_priority, scheduler_item_id, status, definition_event_id) VALUES (?, ?, ?, ?, ?, ?, ?, 'active', ?)`, schedule.id, schedule.agentID, schedule.worldTime, schedule.placeID, schedule.activity, schedule.priority, schedule.itemID, m2AgentSetupEventID); err != nil {
			return AgentSetupResult{}, err
		}
	}
	if err := execAgentOne(ctx, tx.conn, "insert Agent world clock", `INSERT INTO world_clocks(instance_id, branch_id, current_world_time, current_day, status, projection_version, last_event_sequence) VALUES (?, ?, ?, 0, 'ready', 0, ?)`, M2DemoInstanceID, M2DemoBranchID, M2AgentSetupTime, sequence); err != nil {
		return AgentSetupResult{}, err
	}
	initialPositions := []struct{ agentID, placeID string }{{M2AgentAdaID, "place_m2_home_ada"}, {M2AgentBoID, "place_m2_home_bo"}}
	for _, position := range initialPositions {
		if err := execAgentOne(ctx, tx.conn, "insert initial Agent movement", `INSERT INTO agent_movements(movement_id, event_id, agent_id, from_place_id, to_place_id, schedule_id, activity_code, world_time, movement_kind) VALUES (?, ?, ?, NULL, ?, NULL, 'home', ?, 'initialize')`, "movement_m2_initialize_"+position.agentID, m2AgentSetupEventID, position.agentID, position.placeID, M2AgentSetupTime); err != nil {
			return AgentSetupResult{}, err
		}
		if err := execAgentOne(ctx, tx.conn, "insert initial Agent position", `INSERT INTO agent_positions(agent_id, place_id, activity_code, effective_world_time, projection_version, last_event_sequence) VALUES (?, ?, 'home', ?, 0, ?)`, position.agentID, position.placeID, M2AgentSetupTime, sequence); err != nil {
			return AgentSetupResult{}, err
		}
	}
	grants := []struct{ id, principal, capability, subject, fields string }{
		{"grant_m2_creator_agent_run", "principal_creator", "world.agent.run", M2DemoBranchID, `[]`},
		{"grant_m2_creator_agent_knowledge", "principal_creator", "world.agent.knowledge.read", "*", `["claim_key","subject_agent_id","place_id","learned_world_time","source_event_id","observation_id"]`},
		{"grant_m2_creator_encounter", "principal_creator", "world.encounter.read", "*", `["place_id","world_time","participants","activity","evidence"]`},
		{"grant_m2_ada_knowledge", M2AgentAdaPrincipal, "world.agent.knowledge.read", M2AgentAdaID, `["claim_key","subject_agent_id","place_id","learned_world_time","source_event_id"]`},
		{"grant_m2_bo_knowledge", M2AgentBoPrincipal, "world.agent.knowledge.read", M2AgentBoID, `["claim_key","subject_agent_id","place_id","learned_world_time","source_event_id"]`},
		{"grant_m2_ada_encounter", M2AgentAdaPrincipal, "world.encounter.read", M2AgentAdaID, `["place_id","world_time","participants","activity","evidence"]`},
		{"grant_m2_bo_encounter", M2AgentBoPrincipal, "world.encounter.read", M2AgentBoID, `["place_id","world_time","participants","activity","evidence"]`},
	}
	for _, grant := range grants {
		if err := execAgentOne(ctx, tx.conn, "insert Agent grant", `INSERT INTO capability_grants(grant_id, principal_id, capability_id, instance_id, branch_id, subject_id, field_scope, status, definition_event_id) VALUES (?, ?, ?, ?, ?, ?, ?, 'active', ?)`, grant.id, grant.principal, grant.capability, M2DemoInstanceID, M2DemoBranchID, grant.subject, grant.fields, m2AgentSetupEventID); err != nil {
			return AgentSetupResult{}, err
		}
	}
	if err := s.insertAgentAudit(ctx, tx.conn, "audit_"+m2AgentSetupCommandID, "agent_decision", m2AgentSetupEventID, m2AgentSetupCommandID, attemptID, M2AgentSetupTime, nowText, payloadJSON); err != nil {
		return AgentSetupResult{}, err
	}
	if err := insertAgentOutbox(ctx, tx.conn, "outbox_m2_agent_life_setup", m2AgentSetupEventID, "agent.life.initialized", payloadJSON); err != nil {
		return AgentSetupResult{}, err
	}
	if err := execAgentOne(ctx, tx.conn, "advance Agent setup branch", `UPDATE branches SET head_sequence = ? WHERE instance_id = ? AND branch_id = ? AND head_sequence = ?`, sequence, M2DemoInstanceID, M2DemoBranchID, head); err != nil {
		return AgentSetupResult{}, err
	}
	if err := execAgentOne(ctx, tx.conn, "commit Agent setup attempt", `UPDATE command_attempts SET status = 'committed', finished_at_utc = ? WHERE command_id = ? AND attempt_no = 1 AND status = 'ready'`, nowText, m2AgentSetupCommandID); err != nil {
		return AgentSetupResult{}, err
	}
	if err := execAgentOne(ctx, tx.conn, "commit Agent setup command", `UPDATE commands SET status = 'committed' WHERE command_id = ? AND status = 'pending'`, m2AgentSetupCommandID); err != nil {
		return AgentSetupResult{}, err
	}
	if s.beforeCommit != nil {
		if err := s.beforeCommit(); err != nil {
			return AgentSetupResult{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return AgentSetupResult{}, core.WrapError(core.CodeStorageFailure, "commit M2 Agent setup", err)
	}
	return AgentSetupResult{CommandID: m2AgentSetupCommandID, EventID: m2AgentSetupEventID, EventSequence: sequence, AgentCount: 2, ScheduleCount: 4}, nil
}

func (s *Store) RunAgentLifeAuthorized(ctx context.Context, request core.AgentLifeRunRequest) (AgentLifeRunResult, error) {
	if err := request.Validate(); err != nil {
		return AgentLifeRunResult{}, err
	}
	if err := s.authorizeExactScope(ctx, request.PrincipalID, request.CapabilityID, request.InstanceID, request.BranchID, request.BranchID); err != nil {
		return AgentLifeRunResult{}, err
	}
	if request.InstanceID != M2DemoInstanceID || request.BranchID != M2DemoBranchID {
		return AgentLifeRunResult{}, core.NewError(core.CodeNotFound, "bounded M2 Agent world not found")
	}
	return s.RunAgentLife(ctx, request.TargetWorldTime, request.Budget)
}

func (s *Store) RunAgentLife(ctx context.Context, targetWorldTime string, budget int) (result AgentLifeRunResult, resultErr error) {
	target, err := time.Parse(time.RFC3339, targetWorldTime)
	if err != nil {
		return AgentLifeRunResult{}, core.WrapError(core.CodeInvalidArgument, "target_world_time must be RFC 3339", err)
	}
	if budget <= 0 || budget > 10000 {
		return AgentLifeRunResult{}, core.NewError(core.CodeInvalidArgument, "budget must be between 1 and 10000")
	}
	var currentText string
	if err := s.db.QueryRowContext(ctx, `SELECT current_world_time FROM world_clocks WHERE instance_id = ? AND branch_id = ?`, M2DemoInstanceID, M2DemoBranchID).Scan(&currentText); err != nil {
		return AgentLifeRunResult{}, classifyMissing(err, "M2 Agent world clock")
	}
	current, err := time.Parse(time.RFC3339, currentText)
	if err != nil {
		return AgentLifeRunResult{}, core.WrapError(core.CodeStorageFailure, "decode M2 Agent world clock", err)
	}
	if target.Before(current) {
		return AgentLifeRunResult{}, core.NewError(core.CodeInvalidArgument, "Agent world cannot run backward")
	}
	now := s.now().UTC()
	runID := fmt.Sprintf("agent_run_%d", now.UnixNano())
	if _, err := s.db.ExecContext(ctx, `INSERT INTO agent_runs(run_id, instance_id, branch_id, target_world_time, status, started_at_utc) VALUES (?, ?, ?, ?, 'running', ?)`, runID, M2DemoInstanceID, M2DemoBranchID, targetWorldTime, now.Format(time.RFC3339Nano)); err != nil {
		return AgentLifeRunResult{}, core.WrapError(core.CodeStorageFailure, "start Agent run", err)
	}
	processed := 0
	defer func() {
		if resultErr != nil {
			cleanupContext, stopCleanup := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
			defer stopCleanup()
			_, _ = s.db.ExecContext(cleanupContext, `UPDATE agent_runs SET processed_items = ?, status = 'failed', finished_at_utc = ? WHERE run_id = ? AND status = 'running'`, processed, s.now().UTC().Format(time.RFC3339Nano), runID)
		}
	}()
	for processed < budget {
		didWork, err := s.executeNextAgentSchedule(ctx, targetWorldTime)
		if err != nil {
			return AgentLifeRunResult{}, err
		}
		if !didWork {
			break
		}
		processed++
	}
	var pending, head int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM scheduler_items WHERE instance_id = ? AND branch_id = ? AND status = 'pending' AND world_time <= ?`, M2DemoInstanceID, M2DemoBranchID, targetWorldTime).Scan(&pending); err != nil {
		return AgentLifeRunResult{}, core.WrapError(core.CodeStorageFailure, "count due M2 schedules", err)
	}
	status := "completed"
	if pending > 0 {
		status = "budget_exhausted"
	}
	if err := s.db.QueryRowContext(ctx, `SELECT current_world_time FROM world_clocks WHERE instance_id = ? AND branch_id = ?`, M2DemoInstanceID, M2DemoBranchID).Scan(&currentText); err != nil {
		return AgentLifeRunResult{}, core.WrapError(core.CodeStorageFailure, "read Agent clock after run", err)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id = ? AND branch_id = ?`, M2DemoInstanceID, M2DemoBranchID).Scan(&head); err != nil {
		return AgentLifeRunResult{}, core.WrapError(core.CodeStorageFailure, "read Agent head after run", err)
	}
	finished := s.now().UTC().Format(time.RFC3339Nano)
	if err := execAgentOne(ctx, s.db, "finish Agent run", `UPDATE agent_runs SET processed_items = ?, pending_due = ?, status = ?, finished_at_utc = ? WHERE run_id = ? AND status = 'running'`, processed, pending, status, finished, runID); err != nil {
		return AgentLifeRunResult{}, err
	}
	return AgentLifeRunResult{RunID: runID, TargetWorldTime: targetWorldTime, CurrentWorldTime: currentText, ProcessedItems: processed, PendingDue: pending, Status: status, HeadSequence: head}, nil
}

func (s *Store) executeNextAgentSchedule(ctx context.Context, targetWorldTime string) (bool, error) {
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return false, core.WrapError(core.CodeStorageFailure, "begin Agent schedule", err)
	}
	defer tx.Rollback(ctx)
	var item SchedulerItem
	err = tx.conn.QueryRowContext(ctx, `
		SELECT scheduler_item_id, world_time, phase_id, declared_priority, status, payload
		FROM scheduler_items
		WHERE instance_id = ? AND branch_id = ? AND status = 'pending' AND world_time <= ?
		ORDER BY world_time, phase_id, declared_priority, scheduler_item_id LIMIT 1`,
		M2DemoInstanceID, M2DemoBranchID, targetWorldTime,
	).Scan(&item.SchedulerItemID, &item.WorldTime, &item.PhaseID, &item.DeclaredPriority, &item.Status, &item.Payload)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, core.WrapError(core.CodeStorageFailure, "select Agent schedule", err)
	}
	var currentWorldTime string
	if err := tx.conn.QueryRowContext(ctx, `SELECT current_world_time FROM world_clocks WHERE instance_id = ? AND branch_id = ?`, M2DemoInstanceID, M2DemoBranchID).Scan(&currentWorldTime); err != nil {
		return false, classifyMissing(err, "M2 scheduler world clock")
	}
	// A late definition or corrupted queue must not retroactively settle after a
	// later fact has already been committed, even when the caller's target is valid.
	if item.WorldTime < currentWorldTime {
		return false, core.NewError(core.CodeStorageFailure, "M2 scheduler item predates committed world clock")
	}
	if item.PhaseID == m2EconomyPhaseAccrue || item.PhaseID == m2EconomyPhasePay || item.PhaseID == m2EconomyRentPhaseAccrue || item.PhaseID == m2EconomyRentPhasePay || item.PhaseID == m2StorePhaseBuy || item.PhaseID == m2FoodConsumePhase || item.PhaseID == m2RestockPhase || item.PhaseID == m2ServicePhase || item.PhaseID == m2WageRetryPhase || item.PhaseID == m2RentRetryPhase || item.PhaseID == m2DefaultReviewPhase || item.PhaseID == m2InsolvencyPhase || item.PhaseID == m2EstateContributionPhase || item.PhaseID == m2EstateDistributionPhase {
		if err := s.executeNextM2Economy(ctx, tx, item); err != nil {
			return false, err
		}
		if err := tx.Commit(ctx); err != nil {
			return false, core.WrapError(core.CodeStorageFailure, "commit M2 wage schedule", err)
		}
		return true, nil
	}
	if item.PhaseID != m2AgentPhaseID {
		return false, core.NewError(core.CodeStorageFailure, "unsupported M2 scheduler phase "+item.PhaseID)
	}
	var scheduled agentSchedulePayload
	if err := json.Unmarshal([]byte(item.Payload), &scheduled); err != nil {
		return false, core.WrapError(core.CodeStorageFailure, "decode Agent schedule", err)
	}
	if scheduled.Kind != "agent_move" || scheduled.Day < 0 {
		return false, core.NewError(core.CodeStorageFailure, "invalid Agent schedule payload")
	}
	var scheduleWorldTime, targetPlace, activity, scheduleStatus, fromPlace string
	var positionVersion int64
	if err := tx.conn.QueryRowContext(ctx, `
		SELECT s.world_time, s.place_id, s.activity_code, s.status, p.place_id, p.projection_version
		FROM agent_schedule_entries s JOIN agent_profiles a ON a.agent_id = s.agent_id
		JOIN agent_positions p ON p.agent_id = a.agent_id
		WHERE s.schedule_id = ? AND s.scheduler_item_id = ? AND s.agent_id = ?
		  AND a.instance_id = ? AND a.branch_id = ? AND a.status = 'active'`,
		scheduled.ScheduleID, item.SchedulerItemID, scheduled.AgentID, M2DemoInstanceID, M2DemoBranchID,
	).Scan(&scheduleWorldTime, &targetPlace, &activity, &scheduleStatus, &fromPlace, &positionVersion); err != nil {
		return false, classifyMissing(err, "active Agent schedule")
	}
	if scheduleStatus != "active" || scheduleWorldTime != item.WorldTime || targetPlace != scheduled.ToPlaceID || activity != scheduled.ActivityCode || fromPlace == targetPlace {
		return false, core.NewError(core.CodeProjectionDiverged, "Agent schedule, position, and scheduler item do not agree")
	}
	rows, err := tx.conn.QueryContext(ctx, `
		SELECT p.agent_id FROM agent_positions p JOIN agent_profiles a ON a.agent_id = p.agent_id
		WHERE a.instance_id = ? AND a.branch_id = ? AND a.status = 'active'
		  AND p.place_id = ? AND p.agent_id <> ? ORDER BY p.agent_id`,
		M2DemoInstanceID, M2DemoBranchID, targetPlace, scheduled.AgentID,
	)
	if err != nil {
		return false, core.WrapError(core.CodeStorageFailure, "read co-located Agents", err)
	}
	var coLocated []string
	for rows.Next() {
		var agentID string
		if err := rows.Scan(&agentID); err != nil {
			rows.Close()
			return false, core.WrapError(core.CodeStorageFailure, "scan co-located Agent", err)
		}
		coLocated = append(coLocated, agentID)
	}
	if err := rows.Close(); err != nil {
		return false, core.WrapError(core.CodeStorageFailure, "close co-located Agents", err)
	}
	if err := rows.Err(); err != nil {
		return false, core.WrapError(core.CodeStorageFailure, "iterate co-located Agents", err)
	}

	var head int64
	if err := tx.conn.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id = ? AND branch_id = ?`, M2DemoInstanceID, M2DemoBranchID).Scan(&head); err != nil {
		return false, core.WrapError(core.CodeStorageFailure, "read Agent branch head", err)
	}
	sequence := head + 1
	var epochID string
	if err := tx.conn.QueryRowContext(ctx, `SELECT epoch_id FROM rule_epochs WHERE instance_id = ? AND branch_id = ? AND start_sequence <= ? AND (end_sequence IS NULL OR ? < end_sequence)`, M2DemoInstanceID, M2DemoBranchID, sequence, sequence).Scan(&epochID); err != nil {
		return false, classifyMissing(err, "Agent schedule Rule Epoch")
	}
	eventPayload := agentMovementPayload{scheduled.AgentID, scheduled.ScheduleID, fromPlace, targetPlace, activity, coLocated}
	eventPayloadJSON, err := core.CanonicalJSON(eventPayload)
	if err != nil {
		return false, err
	}
	requestHash, err := core.HashJSON(struct {
		CommandType string `json:"command_type"`
		ItemID      string `json:"scheduler_item_id"`
		Payload     string `json:"payload"`
	}{"AgentScheduledMove", item.SchedulerItemID, item.Payload})
	if err != nil {
		return false, err
	}
	batchHash, err := core.HashJSON(struct {
		ItemID, WorldTime, EventType string
		Sequence                     int64
		Payload                      agentMovementPayload
	}{item.SchedulerItemID, item.WorldTime, "AgentMoved", sequence, eventPayload})
	if err != nil {
		return false, err
	}
	commandID := "cmd_" + item.SchedulerItemID
	attemptID := "attempt_" + item.SchedulerItemID + "_1"
	batchID := "batch_" + item.SchedulerItemID
	eventID := "event_" + item.SchedulerItemID
	nowText := s.now().UTC().Format(time.RFC3339Nano)
	statements := []struct {
		name, query string
		args        []any
	}{
		{"Agent movement command", `INSERT INTO commands(command_id, instance_id, branch_id, command_type, idempotency_key, request_hash, expected_head, principal_id, command_policy, status, created_at_utc) VALUES (?, ?, ?, 'AgentScheduledMove', ?, ?, ?, 'principal_system', '{"authorization":"ruleset-scheduler"}', 'pending', ?)`, []any{commandID, M2DemoInstanceID, M2DemoBranchID, item.SchedulerItemID, requestHash, head, nowText}},
		{"Agent movement attempt", `INSERT INTO command_attempts(command_id, attempt_no, attempt_id, status, lease_owner, lease_until_utc, proposal_hash, created_at_utc) VALUES (?, 1, ?, 'ready', 'corerp-m2-agent', ?, ?, ?)`, []any{commandID, attemptID, s.now().UTC().Add(30 * time.Second).Format(time.RFC3339Nano), requestHash, nowText}},
		{"Agent movement batch", `INSERT INTO event_batches(batch_id, command_id, attempt_no, instance_id, branch_id, epoch_id, expected_head, first_sequence, last_sequence, event_count, world_time, batch_hash, committed_at_utc) VALUES (?, ?, 1, ?, ?, ?, ?, ?, ?, 1, ?, ?, ?)`, []any{batchID, commandID, M2DemoInstanceID, M2DemoBranchID, epochID, head, sequence, sequence, item.WorldTime, batchHash, nowText}},
		{"Agent movement event", `INSERT INTO events(event_id, batch_id, instance_id, branch_id, event_sequence, batch_index, event_type, actor_id, world_time, payload) VALUES (?, ?, ?, ?, ?, 0, 'AgentMoved', ?, ?, ?)`, []any{eventID, batchID, M2DemoInstanceID, M2DemoBranchID, sequence, scheduled.AgentID, item.WorldTime, string(eventPayloadJSON)}},
		{"Agent movement fact", `INSERT INTO agent_movements(movement_id, event_id, agent_id, from_place_id, to_place_id, schedule_id, activity_code, world_time, movement_kind) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'scheduled')`, []any{"movement_" + item.SchedulerItemID, eventID, scheduled.AgentID, fromPlace, targetPlace, scheduled.ScheduleID, activity, item.WorldTime}},
	}
	for _, statement := range statements {
		if err := execAgentOne(ctx, tx.conn, statement.name, statement.query, statement.args...); err != nil {
			return false, err
		}
	}
	if err := execAgentOne(ctx, tx.conn, "update Agent position", `UPDATE agent_positions SET place_id = ?, activity_code = ?, effective_world_time = ?, projection_version = projection_version + 1, last_event_sequence = ? WHERE agent_id = ? AND projection_version = ?`, targetPlace, activity, item.WorldTime, sequence, scheduled.AgentID, positionVersion); err != nil {
		return false, err
	}
	if err := execAgentOne(ctx, tx.conn, "complete Agent schedule", `UPDATE agent_schedule_entries SET status = 'completed' WHERE schedule_id = ? AND status = 'active'`, scheduled.ScheduleID); err != nil {
		return false, err
	}
	if err := execAgentOne(ctx, tx.conn, "complete Agent scheduler item", `UPDATE scheduler_items SET status = 'completed' WHERE scheduler_item_id = ? AND status = 'pending'`, item.SchedulerItemID); err != nil {
		return false, err
	}
	for _, otherAgentID := range coLocated {
		pairs := [][2]string{{scheduled.AgentID, otherAgentID}, {otherAgentID, scheduled.AgentID}}
		for _, pair := range pairs {
			if err := upsertCoLocationKnowledge(ctx, tx.conn, eventID, sequence, pair[0], pair[1], targetPlace, item.WorldTime); err != nil {
				return false, err
			}
		}
	}
	if err := execAgentOne(ctx, tx.conn, "advance Agent clock", `UPDATE world_clocks SET current_world_time = ?, current_day = ?, status = 'running', projection_version = projection_version + 1, last_event_sequence = ? WHERE instance_id = ? AND branch_id = ? AND current_world_time <= ?`, item.WorldTime, scheduled.Day, sequence, M2DemoInstanceID, M2DemoBranchID, item.WorldTime); err != nil {
		return false, err
	}
	if err := execAgentOne(ctx, tx.conn, "advance Agent branch", `UPDATE branches SET head_sequence = ? WHERE instance_id = ? AND branch_id = ? AND head_sequence = ?`, sequence, M2DemoInstanceID, M2DemoBranchID, head); err != nil {
		return false, err
	}
	if err := s.insertAgentAudit(ctx, tx.conn, "audit_"+commandID, "agent_decision", eventID, commandID, attemptID, item.WorldTime, nowText, eventPayloadJSON); err != nil {
		return false, err
	}
	if err := insertAgentOutbox(ctx, tx.conn, "outbox_"+item.SchedulerItemID, eventID, "agent.moved", eventPayloadJSON); err != nil {
		return false, err
	}
	if err := execAgentOne(ctx, tx.conn, "commit Agent movement attempt", `UPDATE command_attempts SET status = 'committed', finished_at_utc = ? WHERE command_id = ? AND attempt_no = 1 AND status = 'ready'`, nowText, commandID); err != nil {
		return false, err
	}
	if err := execAgentOne(ctx, tx.conn, "commit Agent movement command", `UPDATE commands SET status = 'committed' WHERE command_id = ? AND status = 'pending'`, commandID); err != nil {
		return false, err
	}
	if s.beforeCommit != nil {
		if err := s.beforeCommit(); err != nil {
			return false, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return false, core.WrapError(core.CodeStorageFailure, "commit Agent schedule", err)
	}
	return true, nil
}

func upsertCoLocationKnowledge(ctx context.Context, conn *sql.Conn, eventID string, sequence int64, observerID, subjectID, placeID, worldTime string) error {
	claimKey := "presence:" + subjectID
	observationID := fmt.Sprintf("observation_%s_%s_%s", eventID, observerID, subjectID)
	claim := struct {
		ClaimType         string `json:"claim_type"`
		SubjectAgentID    string `json:"subject_agent_id"`
		PlaceID           string `json:"place_id"`
		ObservedWorldTime string `json:"observed_world_time"`
	}{"agent_presence", subjectID, placeID, worldTime}
	claimJSON, err := core.CanonicalJSON(claim)
	if err != nil {
		return err
	}
	if err := execAgentOne(ctx, conn, "insert co-location observation", `INSERT INTO observation_records(observation_id, source_event_id, observer_agent_id, subject_agent_id, place_id, channel, observed_world_time, claim_key, claim_payload) VALUES (?, ?, ?, ?, ?, 'co_location', ?, ?, ?)`, observationID, eventID, observerID, subjectID, placeID, worldTime, claimKey, string(claimJSON)); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, `
		INSERT INTO agent_knowledge(observer_agent_id, claim_key, subject_agent_id, place_id, source_event_id, observation_id, learned_world_time, claim_payload, projection_version, last_event_sequence)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 0, ?)
		ON CONFLICT(observer_agent_id, claim_key) DO UPDATE SET
		  subject_agent_id = excluded.subject_agent_id,
		  place_id = excluded.place_id,
		  source_event_id = excluded.source_event_id,
		  observation_id = excluded.observation_id,
		  learned_world_time = excluded.learned_world_time,
		  claim_payload = excluded.claim_payload,
		  projection_version = agent_knowledge.projection_version + 1,
		  last_event_sequence = excluded.last_event_sequence`,
		observerID, claimKey, subjectID, placeID, eventID, observationID, worldTime, string(claimJSON), sequence,
	); err != nil {
		return core.WrapError(core.CodeStorageFailure, "update Agent knowledge projection", err)
	}
	return nil
}

func (s *Store) insertAgentAudit(ctx context.Context, conn *sql.Conn, recordID, recordType, eventID, commandID, attemptID, worldTime, recordedAt string, payload []byte) error {
	var recordOrder int64
	if err := conn.QueryRowContext(ctx, `SELECT COALESCE(MAX(record_order), 0) + 1 FROM audit_records WHERE instance_id = ? AND branch_id = ?`, M2DemoInstanceID, M2DemoBranchID).Scan(&recordOrder); err != nil {
		return core.WrapError(core.CodeStorageFailure, "allocate Agent audit order", err)
	}
	audience := fmt.Sprintf(`{"instance_id":%q,"kind":"instance"}`, M2DemoInstanceID)
	return execAgentOne(ctx, conn, "insert Agent audit", `INSERT INTO audit_records(record_id, instance_id, branch_id, record_order, record_type, authority, related_event_id, command_id, attempt_id, trace_id, world_time, recorded_at_utc, audience_scope, payload) VALUES (?, ?, ?, ?, ?, 'audit', ?, ?, ?, ?, ?, ?, ?, ?)`, recordID, M2DemoInstanceID, M2DemoBranchID, recordOrder, recordType, eventID, commandID, attemptID, "trace_"+commandID, worldTime, recordedAt, audience, string(payload))
}

func insertAgentOutbox(ctx context.Context, conn *sql.Conn, outboxID, eventID, topic string, payload []byte) error {
	audience := struct {
		InstanceID string `json:"instance_id"`
		Kind       string `json:"kind"`
	}{M2DemoInstanceID, "instance"}
	audienceJSON, err := core.CanonicalJSON(audience)
	if err != nil {
		return err
	}
	audienceHash, err := core.HashJSON(audience)
	if err != nil {
		return err
	}
	return execAgentOne(ctx, conn, "insert Agent Outbox", `INSERT INTO outbox(outbox_id, event_id, topic, audience_scope, audience_scope_hash, payload) VALUES (?, ?, ?, ?, ?, ?)`, outboxID, eventID, topic, string(audienceJSON), audienceHash, string(payload))
}

type agentExecer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func execAgentOne(ctx context.Context, execer agentExecer, name, query string, args ...any) error {
	result, err := execer.ExecContext(ctx, query, args...)
	if err != nil {
		return core.WrapError(core.CodeStorageFailure, name, err)
	}
	rows, err := result.RowsAffected()
	if err != nil || rows != 1 {
		return core.NewError(core.CodeStorageFailure, name+" affected an unexpected row count")
	}
	return nil
}

func sortedAgentIDs(values []string) []string {
	result := append([]string(nil), values...)
	sort.Strings(result)
	return result
}
