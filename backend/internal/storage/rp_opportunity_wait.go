package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"corerp.local/backend/internal/core"
)

// Private audit evidence, not a public occurrence or automatic NPC knowledge.
type rpContactOpportunity struct {
	Seeking                   bool                   `json:"seeking,omitempty"`
	Quiet                     bool                   `json:"quiet,omitempty"`
	MajorChangeSourceEventIDs []string               `json:"major_change_source_event_ids,omitempty"`
	ActorID                   string                 `json:"actor_id"`
	TargetID                  string                 `json:"target_id"`
	SourceEventID             string                 `json:"source_event_id"`
	PolicyEventID             string                 `json:"policy_event_id"`
	WorldTime                 string                 `json:"world_time"`
	Busy                      bool                   `json:"busy"`
	CoolingDown               bool                   `json:"cooling_down"`
	RecentChanges             int                    `json:"recent_changes"`
	Draw                      core.RPOpportunityDraw `json:"draw"`
}

func evaluateRPContactOpportunities(ctx context.Context, conn *sql.Conn, session RPSession, npcs []string, target string, pending ...rpWaitEvent) ([]rpContactOpportunity, error) {
	policy, err := readRPOpportunityPolicy(ctx, conn, session.InstanceID, session.BranchID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	at, err := time.Parse(time.RFC3339, target)
	if err != nil {
		return nil, err
	}
	window := at.UTC().Truncate(time.Hour).Format(time.RFC3339)
	start := at.Add(-time.Duration(policy.Fact.Policy.HistoryHours) * time.Hour).Format(time.RFC3339)
	rows, err := conn.QueryContext(ctx, `SELECT payload FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPWaitCompleted' AND world_time>=? AND world_time<=? ORDER BY event_sequence`, session.InstanceID, session.BranchID, start, target)
	if err != nil {
		return nil, err
	}
	var history []rpContactOpportunity
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
		history = append(history, old.ContactOpportunities...)
		allHistory = append(allHistory, old)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	var receipts []rpContactOpportunity
	for _, npc := range npcs {
		input, err := readRPOwnDecisionContext(ctx, conn, core.RPDecisionInput{InstanceID: session.InstanceID, BranchID: session.BranchID, NPCEntityID: npc, InterlocutorEntityID: session.ControlledEntityID})
		if err != nil {
			return nil, err
		}
		if input.Life == nil {
			continue
		}
		var source string
		for _, relation := range input.Life.Relationships {
			if relation.SubjectEntityID == session.ControlledEntityID && relation.Trust >= 2 && relation.Tension == 0 && len(relation.SourceEventIDs) > 0 {
				source = relation.SourceEventIDs[0]
				break
			}
		}
		if source == "" {
			continue
		}
		var existing *rpContactOpportunity
		cooldown := false
		for i := range history {
			old := &history[i]
			if old.ActorID != npc || old.WorldTime < start {
				continue
			}
			if old.SourceEventID == source && old.TargetID == session.ControlledEntityID && old.Draw.WindowStart == window && existing == nil {
				existing = old
			}
			if old.Draw.Selected {
				previous, err := time.Parse(time.RFC3339, old.WorldTime)
				if err != nil {
					return nil, err
				}
				if at.Before(previous.Add(time.Duration(policy.Fact.Policy.CooldownHours) * time.Hour)) {
					cooldown = true
				}
			}
		}
		if existing != nil {
			receipts = append(receipts, *existing)
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
		majorSources, err := readRPOpportunityMajorChanges(ctx, conn, session.InstanceID, session.BranchID, npc, start, target)
		if err != nil {
			return nil, err
		}
		recent, sharedCooldown, err := rpOpportunityHistoryPressure(allHistory, npc, at, policy.Fact.Policy)
		if err != nil {
			return nil, err
		}
		cooldown = cooldown || sharedCooldown
		quiet, err := readRPQuietOpportunityHistory(ctx, conn, input, policy, at, recent, len(majorSources))
		if err != nil {
			return nil, err
		}
		// Only the current authorized wait's explicit intent qualifies. Old
		// intents do not persist, and existing hourly receipts bypass this path.
		seeking := len(pending) > 0 && pending[0].SessionID == session.SessionID && pending[0].EntityID == session.ControlledEntityID && pending[0].OpportunityIntent == "social"
		pressure := core.RPOpportunityPressure{Seeking: seeking, Quiet: quiet, Busy: busy, CoolingDown: cooldown, RecentChanges: recent, RecentMajor: len(majorSources) > 0}
		chance, err := core.RPOpportunityProbability(policy.Fact.Policy.ContactBasisPoints, false, pressure)
		if err != nil {
			return nil, err
		}
		draw, err := core.DrawRPOpportunity(core.RPOpportunityDrawKey{PolicyEventID: policy.EventID, StreamSeed: policy.Fact.Policy.StreamSeed, InstanceID: session.InstanceID, BranchID: session.BranchID, ActorID: npc, Kind: "friend_contact", SourceEventID: source, WorldTime: target}, chance)
		if err != nil {
			return nil, err
		}
		receipts = append(receipts, rpContactOpportunity{Seeking: seeking, Quiet: quiet, MajorChangeSourceEventIDs: majorSources, ActorID: npc, TargetID: session.ControlledEntityID, SourceEventID: source, PolicyEventID: policy.EventID, WorldTime: target, Busy: busy, CoolingDown: cooldown, RecentChanges: recent, Draw: draw})
	}
	return receipts, nil
}
