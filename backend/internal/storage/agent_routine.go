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
	m2RoutineCommandID   = "cmd_m2_agent_routine_30"
	m2RoutineEventID     = "event_m2_agent_routine_30"
	M2AgentDay30NoonTime = "2026-10-22T12:00:00Z"
)

type AgentRoutineResult struct {
	CommandID     string `json:"command_id"`
	EventID       string `json:"event_id"`
	EventSequence int64  `json:"event_sequence"`
	Days          int    `json:"days"`
	ScheduleCount int    `json:"schedule_count"`
	Replayed      bool   `json:"replayed"`
}

type routineEntry struct {
	ScheduleID   string `json:"schedule_id"`
	ItemID       string `json:"scheduler_item_id"`
	AgentID      string `json:"agent_id"`
	WorldTime    string `json:"world_time"`
	PlaceID      string `json:"place_id"`
	ActivityCode string `json:"activity_code"`
	Priority     int64  `json:"declared_priority"`
	Day          int    `json:"day"`
}

type routineDefinition struct {
	Algorithm string         `json:"algorithm"`
	Days      int            `json:"days"`
	Entries   []routineEntry `json:"entries"`
}

func buildM2Routine() routineDefinition {
	definition := routineDefinition{Algorithm: "daily-morning-noon-v1", Days: 30, Entries: make([]routineEntry, 0, 116)}
	for day := 2; day <= definition.Days; day++ {
		for _, transition := range []struct {
			name, agentID, placeID, activity string
			hour                             int
			priority                         int64
		}{
			{"ada_work", M2AgentAdaID, "place_m2_work_ada", "work", 8, 10},
			{"bo_work", M2AgentBoID, "place_m2_work_bo", "work", 8, 10},
			{"ada_cafe", M2AgentAdaID, M2AgentCafeID, "lunch", 12, 20},
			{"bo_cafe", M2AgentBoID, M2AgentCafeID, "lunch", 12, 20},
		} {
			worldTime := time.Date(2026, time.September, 22+day, transition.hour, 0, 0, 0, time.UTC)
			label := fmt.Sprintf("%s_%s", worldTime.Format("20060102_1504"), transition.name)
			definition.Entries = append(definition.Entries, routineEntry{
				ScheduleID: "schedule_m2_" + label, ItemID: "sched_m2_" + label,
				AgentID: transition.agentID, WorldTime: worldTime.Format(time.RFC3339),
				PlaceID: transition.placeID, ActivityCode: transition.activity,
				Priority: transition.priority, Day: day,
			})
		}
	}
	return definition
}

func (s *Store) DefineM2AgentRoutine(ctx context.Context, request core.AgentRoutineRequest) (AgentRoutineResult, error) {
	if err := request.Validate(); err != nil {
		return AgentRoutineResult{}, err
	}
	if err := s.authorizeExactScope(ctx, request.PrincipalID, request.CapabilityID, request.InstanceID, request.BranchID, request.BranchID); err != nil {
		return AgentRoutineResult{}, err
	}
	if request.InstanceID != M2DemoInstanceID || request.BranchID != M2DemoBranchID {
		return AgentRoutineResult{}, core.NewError(core.CodeNotFound, "M2 Agent world not found")
	}
	definition := buildM2Routine()
	requestHash, err := core.HashJSON(struct {
		InstanceID string            `json:"instance_id"`
		BranchID   string            `json:"branch_id"`
		Definition routineDefinition `json:"definition"`
	}{request.InstanceID, request.BranchID, definition})
	if err != nil {
		return AgentRoutineResult{}, err
	}
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return AgentRoutineResult{}, core.WrapError(core.CodeStorageFailure, "begin Agent routine definition", err)
	}
	defer tx.Rollback(ctx)
	var status, existingHash string
	err = tx.conn.QueryRowContext(ctx, `SELECT status, request_hash FROM commands WHERE command_id = ?`, m2RoutineCommandID).Scan(&status, &existingHash)
	if err == nil {
		if existingHash != requestHash {
			return AgentRoutineResult{}, core.NewError(core.CodeIdempotencyMismatch, "Agent routine payload changed")
		}
		if status != "committed" {
			return AgentRoutineResult{}, core.NewError(core.CodeCommandInProgress, "Agent routine definition is not committed")
		}
		var sequence int64
		if err := tx.conn.QueryRowContext(ctx, `SELECT event_sequence FROM events WHERE event_id = ?`, m2RoutineEventID).Scan(&sequence); err != nil {
			return AgentRoutineResult{}, core.WrapError(core.CodeStorageFailure, "load Agent routine event", err)
		}
		return AgentRoutineResult{m2RoutineCommandID, m2RoutineEventID, sequence, 30, len(definition.Entries), true}, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return AgentRoutineResult{}, core.WrapError(core.CodeStorageFailure, "check Agent routine definition", err)
	}
	var head int64
	var clock string
	if err := tx.conn.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id = ? AND branch_id = ?`, request.InstanceID, request.BranchID).Scan(&head); err != nil {
		return AgentRoutineResult{}, classifyMissing(err, "M2 Agent branch")
	}
	if err := tx.conn.QueryRowContext(ctx, `SELECT current_world_time FROM world_clocks WHERE instance_id = ? AND branch_id = ?`, request.InstanceID, request.BranchID).Scan(&clock); err != nil {
		return AgentRoutineResult{}, classifyMissing(err, "M2 Agent clock")
	}
	if clock >= definition.Entries[0].WorldTime {
		return AgentRoutineResult{}, core.NewError(core.CodeBranchConflict, "Agent clock reached the first new routine item")
	}
	var activeAgents, initialItems int
	if err := tx.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM agent_profiles WHERE instance_id = ? AND branch_id = ? AND status = 'active' AND agent_id IN (?, ?)`, request.InstanceID, request.BranchID, M2AgentAdaID, M2AgentBoID).Scan(&activeAgents); err != nil {
		return AgentRoutineResult{}, core.WrapError(core.CodeStorageFailure, "validate Agent routine profiles", err)
	}
	if err := tx.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM agent_schedule_entries WHERE definition_event_id = ?`, m2AgentSetupEventID).Scan(&initialItems); err != nil {
		return AgentRoutineResult{}, core.WrapError(core.CodeStorageFailure, "validate Agent day-one schedule", err)
	}
	if activeAgents != 2 || initialItems != 4 {
		return AgentRoutineResult{}, core.NewError(core.CodeBranchConflict, "routine requires the active two-Agent day-one setup")
	}
	sequence := head + 1
	var epochID string
	if err := tx.conn.QueryRowContext(ctx, `SELECT epoch_id FROM rule_epochs WHERE instance_id = ? AND branch_id = ? AND start_sequence <= ? AND (end_sequence IS NULL OR ? < end_sequence)`, request.InstanceID, request.BranchID, sequence, sequence).Scan(&epochID); err != nil {
		return AgentRoutineResult{}, classifyMissing(err, "M2 Agent Rule Epoch")
	}
	batchHash, err := core.HashJSON(struct {
		CommandID string            `json:"command_id"`
		Sequence  int64             `json:"sequence"`
		WorldTime string            `json:"world_time"`
		EventType string            `json:"event_type"`
		Payload   routineDefinition `json:"payload"`
	}{m2RoutineCommandID, sequence, clock, "AgentRoutineDefined", definition})
	if err != nil {
		return AgentRoutineResult{}, err
	}
	payload, err := core.CanonicalJSON(definition)
	if err != nil {
		return AgentRoutineResult{}, err
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	attemptID := "attempt_m2_agent_routine_30_1"
	for _, statement := range []struct {
		name, query string
		args        []any
	}{
		{"create Agent routine command", `INSERT INTO commands(command_id, instance_id, branch_id, command_type, idempotency_key, request_hash, expected_head, principal_id, command_policy, status, created_at_utc) VALUES (?, ?, ?, 'DefineAgentRoutine', ?, ?, ?, ?, '{"authorization":"capability"}', 'pending', ?)`, []any{m2RoutineCommandID, request.InstanceID, request.BranchID, m2RoutineCommandID, requestHash, head, request.PrincipalID, now}},
		{"create Agent routine attempt", `INSERT INTO command_attempts(command_id, attempt_no, attempt_id, status, lease_owner, lease_until_utc, proposal_hash, created_at_utc) VALUES (?, 1, ?, 'ready', 'corerp-m2-agent', ?, ?, ?)`, []any{m2RoutineCommandID, attemptID, s.now().UTC().Add(30 * time.Second).Format(time.RFC3339Nano), requestHash, now}},
		{"create Agent routine batch", `INSERT INTO event_batches(batch_id, command_id, attempt_no, instance_id, branch_id, epoch_id, expected_head, first_sequence, last_sequence, event_count, world_time, batch_hash, committed_at_utc) VALUES (?, ?, 1, ?, ?, ?, ?, ?, ?, 1, ?, ?, ?)`, []any{"batch_m2_agent_routine_30", m2RoutineCommandID, request.InstanceID, request.BranchID, epochID, head, sequence, sequence, clock, batchHash, now}},
		{"create Agent routine event", `INSERT INTO events(event_id, batch_id, instance_id, branch_id, event_sequence, batch_index, event_type, actor_id, world_time, payload) VALUES (?, ?, ?, ?, ?, 0, 'AgentRoutineDefined', ?, ?, ?)`, []any{m2RoutineEventID, "batch_m2_agent_routine_30", request.InstanceID, request.BranchID, sequence, request.PrincipalID, clock, string(payload)}},
	} {
		if err := execAgentOne(ctx, tx.conn, statement.name, statement.query, statement.args...); err != nil {
			return AgentRoutineResult{}, err
		}
	}
	for _, entry := range definition.Entries {
		itemPayload, err := core.CanonicalJSON(agentSchedulePayload{Kind: "agent_move", Day: entry.Day, AgentID: entry.AgentID, ScheduleID: entry.ScheduleID, ToPlaceID: entry.PlaceID, ActivityCode: entry.ActivityCode})
		if err != nil {
			return AgentRoutineResult{}, err
		}
		if err := execAgentOne(ctx, tx.conn, "declare Agent routine item", `INSERT INTO scheduler_items(scheduler_item_id, instance_id, branch_id, world_time, phase_id, declared_priority, status, payload) VALUES (?, ?, ?, ?, ?, ?, 'pending', ?)`, entry.ItemID, request.InstanceID, request.BranchID, entry.WorldTime, m2AgentPhaseID, entry.Priority, string(itemPayload)); err != nil {
			return AgentRoutineResult{}, err
		}
		if err := execAgentOne(ctx, tx.conn, "declare Agent routine schedule", `INSERT INTO agent_schedule_entries(schedule_id, agent_id, world_time, place_id, activity_code, declared_priority, scheduler_item_id, status, definition_event_id) VALUES (?, ?, ?, ?, ?, ?, ?, 'active', ?)`, entry.ScheduleID, entry.AgentID, entry.WorldTime, entry.PlaceID, entry.ActivityCode, entry.Priority, entry.ItemID, m2RoutineEventID); err != nil {
			return AgentRoutineResult{}, err
		}
	}
	if err := s.insertAgentAudit(ctx, tx.conn, "audit_"+m2RoutineCommandID, "agent_decision", m2RoutineEventID, m2RoutineCommandID, attemptID, clock, now, payload); err != nil {
		return AgentRoutineResult{}, err
	}
	if err := insertAgentOutbox(ctx, tx.conn, "outbox_m2_agent_routine_30", m2RoutineEventID, "agent.routine.defined", payload); err != nil {
		return AgentRoutineResult{}, err
	}
	if err := execAgentOne(ctx, tx.conn, "advance Agent routine branch", `UPDATE branches SET head_sequence = ? WHERE instance_id = ? AND branch_id = ? AND head_sequence = ?`, sequence, request.InstanceID, request.BranchID, head); err != nil {
		return AgentRoutineResult{}, err
	}
	if err := execAgentOne(ctx, tx.conn, "commit Agent routine attempt", `UPDATE command_attempts SET status = 'committed', finished_at_utc = ? WHERE command_id = ? AND attempt_no = 1 AND status = 'ready'`, now, m2RoutineCommandID); err != nil {
		return AgentRoutineResult{}, err
	}
	if err := execAgentOne(ctx, tx.conn, "commit Agent routine command", `UPDATE commands SET status = 'committed' WHERE command_id = ? AND status = 'pending'`, m2RoutineCommandID); err != nil {
		return AgentRoutineResult{}, err
	}
	if s.beforeCommit != nil {
		if err := s.beforeCommit(); err != nil {
			return AgentRoutineResult{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return AgentRoutineResult{}, core.WrapError(core.CodeStorageFailure, "commit Agent routine definition", err)
	}
	return AgentRoutineResult{m2RoutineCommandID, m2RoutineEventID, sequence, 30, len(definition.Entries), false}, nil
}
