package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"corerp.local/backend/internal/core"
)

type rpWorkOpportunity struct {
	Quiet                     bool                   `json:"quiet,omitempty"`
	ActorID                   string                 `json:"actor_id"`
	Memory                    core.RPLifeMemory      `json:"memory"`
	WorldTime                 string                 `json:"world_time"`
	Busy                      bool                   `json:"busy"`
	CoolingDown               bool                   `json:"cooling_down"`
	RecentChanges             int                    `json:"recent_changes"`
	MajorChangeSourceEventIDs []string               `json:"major_change_source_event_ids,omitempty"`
	Draw                      core.RPOpportunityDraw `json:"draw"`
}

func evaluateRPWorkOpportunities(ctx context.Context, conn *sql.Conn, session RPSession, npcs []string, target string, pending ...rpWaitEvent) ([]rpWorkOpportunity, error) {
	policy, err := readRPOpportunityPolicy(ctx, conn, session.InstanceID, session.BranchID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if policy.Fact.Policy.WorkBasisPoints == 0 {
		return nil, nil
	}
	at, err := time.Parse(time.RFC3339, target)
	if err != nil {
		return nil, err
	}
	start := at.Add(-time.Duration(policy.Fact.Policy.HistoryHours) * time.Hour).Format(time.RFC3339)
	window := at.UTC().Truncate(time.Hour).Format(time.RFC3339)
	rows, err := conn.QueryContext(ctx, `SELECT payload FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPWaitCompleted' AND world_time>=? AND world_time<=? ORDER BY event_sequence`, session.InstanceID, session.BranchID, start, target)
	if err != nil {
		return nil, err
	}
	var history []rpWorkOpportunity
	allHistory := append([]rpWaitEvent(nil), pending...)
	for rows.Next() {
		var raw string
		var old rpWaitEvent
		if err := rows.Scan(&raw); err != nil {
			rows.Close()
			return nil, err
		}
		if err := json.Unmarshal([]byte(raw), &old); err != nil {
			rows.Close()
			return nil, err
		}
		history = append(history, old.WorkOpportunities...)
		allHistory = append(allHistory, old)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	var result []rpWorkOpportunity
	for _, npc := range npcs {
		input, err := readRPOwnDecisionContext(ctx, conn, core.RPDecisionInput{InstanceID: session.InstanceID, BranchID: session.BranchID, NPCEntityID: npc, InterlocutorEntityID: session.ControlledEntityID})
		if err != nil {
			return nil, err
		}
		if input.Life == nil {
			continue
		}
		// One latest eligible change per actor/window. Selection never reads
		// private manager judgments or another worker's employment facts.
		var memory core.RPLifeMemory
		for _, candidate := range input.Life.SalientMemories {
			if !core.RPWorkChangeMemory(candidate, npc) || candidate.WorldTime < start || candidate.WorldTime > target {
				continue
			}
			if memory.SourceEventID == "" || candidate.WorldTime > memory.WorldTime || candidate.WorldTime == memory.WorldTime && candidate.SourceEventID < memory.SourceEventID {
				memory = candidate
			}
		}
		if memory.SourceEventID == "" {
			continue
		}
		busy := input.ActivityCode == "work"
		if input.NextSchedule != nil && input.NextSchedule.ActivityCode == "work" {
			due, err := time.Parse(time.RFC3339, input.NextSchedule.WorldTime)
			if err != nil {
				return nil, err
			}
			busy = busy || !due.After(at.Add(time.Hour))
		}
		var existing *rpWorkOpportunity
		cooling := false
		for i := range history {
			old := &history[i]
			if old.ActorID != npc || old.WorldTime < start {
				continue
			}
			same := old.Memory.SourceEventID == memory.SourceEventID
			if same && old.Draw.WindowStart == window && existing == nil {
				existing = old
			}
			if old.Draw.Selected {
				previous, err := time.Parse(time.RFC3339, old.WorldTime)
				if err != nil {
					return nil, err
				}
				cooling = cooling || same || at.Before(previous.Add(time.Duration(policy.Fact.Policy.CooldownHours)*time.Hour))
			}
		}
		if existing != nil {
			result = append(result, *existing)
			continue
		}
		major, err := readRPOpportunityMajorChanges(ctx, conn, session.InstanceID, session.BranchID, npc, start, target)
		if err != nil {
			return nil, err
		}
		recent, sharedCooldown, err := rpOpportunityHistoryPressure(allHistory, npc, at, policy.Fact.Policy)
		if err != nil {
			return nil, err
		}
		cooling = cooling || sharedCooldown
		quiet, err := readRPQuietOpportunityHistory(ctx, conn, input, policy, at, recent, len(major))
		if err != nil {
			return nil, err
		}
		chance, err := core.RPOpportunityProbability(policy.Fact.Policy.WorkBasisPoints, false, core.RPOpportunityPressure{Quiet: quiet, Busy: busy, CoolingDown: cooling, RecentChanges: recent, RecentMajor: len(major) > 0})
		if err != nil {
			return nil, err
		}
		draw, err := core.DrawRPOpportunity(core.RPOpportunityDrawKey{PolicyEventID: policy.EventID, StreamSeed: policy.Fact.Policy.StreamSeed, InstanceID: session.InstanceID, BranchID: session.BranchID, ActorID: npc, Kind: "work_change", SourceEventID: memory.SourceEventID, WorldTime: target}, chance)
		if err != nil {
			return nil, err
		}
		result = append(result, rpWorkOpportunity{Quiet: quiet, ActorID: npc, Memory: memory, WorldTime: target, Busy: busy, CoolingDown: cooling, RecentChanges: recent, MajorChangeSourceEventIDs: major, Draw: draw})
	}
	return result, nil
}
