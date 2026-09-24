package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"corerp.local/backend/internal/core"
)

type rpCommunityOpportunity struct {
	ActorID                   string                  `json:"actor_id"`
	Source                    rpCommunityChangeSource `json:"source"`
	WorldTime                 string                  `json:"world_time"`
	Busy                      bool                    `json:"busy"`
	CoolingDown               bool                    `json:"cooling_down"`
	RecentChanges             int                     `json:"recent_changes"`
	MajorChangeSourceEventIDs []string                `json:"major_change_source_event_ids,omitempty"`
	Draw                      core.RPOpportunityDraw  `json:"draw"`
}

func evaluateRPCommunityOpportunities(ctx context.Context, conn *sql.Conn, session RPSession, npcs []string, target string, pending ...rpWaitEvent) ([]rpCommunityOpportunity, error) {
	policy, err := readRPOpportunityPolicy(ctx, conn, session.InstanceID, session.BranchID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if policy.Fact.Policy.CommunityBasisPoints == 0 {
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
	var history []rpCommunityOpportunity
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
		history = append(history, old.CommunityOpportunities...)
		allHistory = append(allHistory, old)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	var result []rpCommunityOpportunity
	for _, npc := range npcs {
		sources, err := readRPCommunityChangeSources(ctx, conn, session.InstanceID, session.BranchID, npc, start, target)
		if err != nil {
			return nil, err
		}
		if len(sources) == 0 {
			continue
		}
		source := sources[0]
		for _, candidate := range sources[1:] {
			if candidate.AvailableSince > source.AvailableSince || candidate.AvailableSince == source.AvailableSince && candidate.Law.EnactmentEventID < source.Law.EnactmentEventID {
				source = candidate
			}
		}
		var existing *rpCommunityOpportunity
		cooling := false
		for i := range history {
			old := &history[i]
			if old.ActorID != npc || old.Source.Law.EnactmentEventID != source.Law.EnactmentEventID {
				continue
			}
			if old.Draw.WindowStart == window && existing == nil {
				existing = old
			}
			cooling = cooling || old.Draw.Selected
		}
		if existing != nil {
			result = append(result, *existing)
			continue
		}
		input, err := readRPOwnDecisionContext(ctx, conn, core.RPDecisionInput{InstanceID: session.InstanceID, BranchID: session.BranchID, NPCEntityID: npc, InterlocutorEntityID: session.ControlledEntityID})
		if err != nil {
			return nil, err
		}
		busy := input.ActivityCode == "work"
		if input.NextSchedule != nil && input.NextSchedule.ActivityCode == "work" {
			due, err := time.Parse(time.RFC3339, input.NextSchedule.WorldTime)
			if err != nil {
				return nil, err
			}
			busy = busy || !due.After(at.Add(time.Hour))
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
		// The known local change itself means this is not a quiet window.
		chance, err := core.RPOpportunityProbability(policy.Fact.Policy.CommunityBasisPoints, false, core.RPOpportunityPressure{Busy: busy, CoolingDown: cooling, RecentChanges: recent, RecentMajor: len(major) > 0})
		if err != nil {
			return nil, err
		}
		draw, err := core.DrawRPOpportunity(core.RPOpportunityDrawKey{PolicyEventID: policy.EventID, StreamSeed: policy.Fact.Policy.StreamSeed, InstanceID: session.InstanceID, BranchID: session.BranchID, ActorID: npc, Kind: "community_change", SourceEventID: source.Law.EnactmentEventID, WorldTime: target}, chance)
		if err != nil {
			return nil, err
		}
		result = append(result, rpCommunityOpportunity{ActorID: npc, Source: source, WorldTime: target, Busy: busy, CoolingDown: cooling, RecentChanges: recent, MajorChangeSourceEventIDs: major, Draw: draw})
	}
	return result, nil
}
