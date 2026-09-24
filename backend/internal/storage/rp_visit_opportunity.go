package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"corerp.local/backend/internal/core"
)

type rpVisitOpportunity struct {
	ActorID                   string                 `json:"actor_id"`
	Source                    core.RPVisitSource     `json:"source"`
	WorldTime                 string                 `json:"world_time"`
	Rare                      bool                   `json:"rare"`
	Quiet                     bool                   `json:"quiet"`
	Busy                      bool                   `json:"busy"`
	RouteAvailable            bool                   `json:"route_available"`
	CoolingDown               bool                   `json:"cooling_down"`
	RecentChanges             int                    `json:"recent_changes"`
	MajorChangeSourceEventIDs []string               `json:"major_change_source_event_ids,omitempty"`
	Draw                      core.RPOpportunityDraw `json:"draw"`
}

func evaluateRPVisitOpportunities(ctx context.Context, conn *sql.Conn, session RPSession, npcs []string, target string, pending ...rpWaitEvent) ([]rpVisitOpportunity, error) {
	policy, err := readRPOpportunityPolicy(ctx, conn, session.InstanceID, session.BranchID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	p := policy.Fact.Policy
	if p.VisitBasisPoints == 0 && p.RareVisitBasisPoints == 0 {
		return nil, nil
	}
	at, err := time.Parse(time.RFC3339, target)
	if err != nil {
		return nil, err
	}
	start := at.Add(-time.Duration(p.HistoryHours) * time.Hour).Format(time.RFC3339)
	window := at.UTC().Truncate(time.Hour).Format(time.RFC3339)
	rows, err := conn.QueryContext(ctx, `SELECT payload FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPWaitCompleted' AND world_time>=? AND world_time<=? ORDER BY event_sequence`, session.InstanceID, session.BranchID, start, target)
	if err != nil {
		return nil, err
	}
	history := append([]rpWaitEvent(nil), pending...)
	var visits []rpVisitOpportunity
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
		history = append(history, old)
		visits = append(visits, old.VisitOpportunities...)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	var result []rpVisitOpportunity
	for _, npc := range npcs {
		// Pin the actor's first visit evaluation for the whole hour, including
		// the chosen source/kind. A miss cannot fall back to another source.
		var existing *rpVisitOpportunity
		for i := range visits {
			if visits[i].ActorID == npc && visits[i].Draw.WindowStart == window {
				existing = &visits[i]
				break
			}
		}
		if existing != nil {
			result = append(result, *existing)
			continue
		}
		input, err := readRPOwnDecisionContext(ctx, conn, core.RPDecisionInput{InstanceID: session.InstanceID, BranchID: session.BranchID, NPCEntityID: npc, InterlocutorEntityID: session.ControlledEntityID})
		if err != nil {
			return nil, err
		}
		input.WorldTime = target
		sources, err := readRPVisitSources(ctx, conn, input, start)
		if err != nil {
			return nil, err
		}
		var source *core.RPVisitSource
		for i := range sources {
			if sources[i].Kind == "old_friend_place" && p.RareVisitBasisPoints > 0 || sources[i].Kind == "familiar_public_place" && p.VisitBasisPoints > 0 {
				source = &sources[i]
				break
			}
		}
		if source == nil {
			continue
		}
		rare := source.Kind == "old_friend_place"
		base := p.VisitBasisPoints
		if rare {
			base = p.RareVisitBasisPoints
		}
		busy := input.ActivityCode == "work"
		if input.NextSchedule != nil && input.NextSchedule.ActivityCode == "work" {
			due, err := time.Parse(time.RFC3339, input.NextSchedule.WorldTime)
			if err != nil {
				return nil, err
			}
			busy = busy || !due.After(at.Add(time.Hour))
		}
		// Own context was read before Wait's final clock commit. Its filtered
		// route list may reflect an earlier instant, so bind both adjacency and
		// physical traversal to the actual evaluation endpoint instead.
		var adjacent int
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_place_links WHERE instance_id=? AND branch_id=? AND from_place_id=? AND to_place_id=?`, session.InstanceID, session.BranchID, input.PlaceID, source.PlaceID).Scan(&adjacent); err != nil {
			return nil, err
		}
		reachable := false
		if adjacent > 0 {
			reachable, err = rpTransitAllowsImmediate(ctx, conn, session.InstanceID, session.BranchID, input.PlaceID, source.PlaceID, target)
			if err != nil {
				return nil, err
			}
		}
		recent, cooling, err := rpOpportunityHistoryPressure(history, npc, at, p)
		if err != nil {
			return nil, err
		}
		for _, old := range visits {
			if old.ActorID == npc && old.Draw.Selected && old.Source.Kind == source.Kind && old.Source.PlaceID == source.PlaceID && old.Source.FriendID == source.FriendID {
				cooling = true
			}
		}
		major, err := readRPOpportunityMajorChanges(ctx, conn, session.InstanceID, session.BranchID, npc, start, target)
		if err != nil {
			return nil, err
		}
		quiet := false
		if !rare {
			quiet, err = readRPQuietOpportunityHistory(ctx, conn, input, policy, at, recent, len(major))
			if err != nil {
				return nil, err
			}
		}
		chance, err := core.RPOpportunityProbability(base, rare, core.RPOpportunityPressure{Quiet: quiet, Busy: busy || !reachable, CoolingDown: cooling, RecentChanges: recent, RecentMajor: len(major) > 0})
		if err != nil {
			return nil, err
		}
		sourceHash, err := core.HashJSON(*source)
		if err != nil {
			return nil, err
		}
		draw, err := core.DrawRPOpportunity(core.RPOpportunityDrawKey{PolicyEventID: policy.EventID, StreamSeed: p.StreamSeed, InstanceID: session.InstanceID, BranchID: session.BranchID, ActorID: npc, Kind: source.Kind, SourceEventID: sourceHash, WorldTime: target}, chance)
		if err != nil {
			return nil, err
		}
		result = append(result, rpVisitOpportunity{ActorID: npc, Source: *source, WorldTime: target, Rare: rare, Quiet: quiet, Busy: busy, RouteAvailable: reachable, CoolingDown: cooling, RecentChanges: recent, MajorChangeSourceEventIDs: major, Draw: draw})
	}
	return result, nil
}
