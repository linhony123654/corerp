package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"corerp.local/backend/internal/core"
)

type rpStoreOpportunity struct {
	Quiet                     bool                     `json:"quiet,omitempty"`
	MajorChangeSourceEventIDs []string                 `json:"major_change_source_event_ids,omitempty"`
	ActorID                   string                   `json:"actor_id"`
	Store                     core.RPStoreAvailability `json:"store"`
	WorldTime                 string                   `json:"world_time"`
	CoolingDown               bool                     `json:"cooling_down"`
	Busy                      bool                     `json:"busy"`
	RecentChanges             int                      `json:"recent_changes"`
	Draw                      core.RPOpportunityDraw   `json:"draw"`
}

// Shortage is an existing shelf fact, never a chance to destroy inventory.
// Only the optional reaction is drawn. Repeated windows retain quiet outcomes;
// selected reactions to the same stock source are suppressed within history.
func evaluateRPStoreOpportunities(ctx context.Context, conn *sql.Conn, session RPSession, npcs []string, target string, pending ...rpWaitEvent) ([]rpStoreOpportunity, error) {
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
	start := at.Add(-time.Duration(policy.Fact.Policy.HistoryHours) * time.Hour).Format(time.RFC3339)
	window := at.UTC().Truncate(time.Hour).Format(time.RFC3339)
	rows, err := conn.QueryContext(ctx, `SELECT payload FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPWaitCompleted' AND world_time>=? AND world_time<=? ORDER BY event_sequence`, session.InstanceID, session.BranchID, start, target)
	if err != nil {
		return nil, err
	}
	var history []rpStoreOpportunity
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
		history = append(history, old.StoreOpportunities...)
		allHistory = append(allHistory, old)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	var result []rpStoreOpportunity
	for _, npc := range npcs {
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
		for _, shelf := range input.Stores {
			if shelf.Available {
				continue
			}
			var raw string
			if err := conn.QueryRowContext(ctx, `SELECT payload FROM events WHERE event_id=? AND instance_id=? AND branch_id=? AND event_type='RPStorefrontSourceDefined'`, shelf.StorefrontSourceEventID, session.InstanceID, session.BranchID).Scan(&raw); err != nil {
				return nil, err
			}
			var source StorefrontSourceFact
			if err := json.Unmarshal([]byte(raw), &source); err != nil {
				return nil, err
			}
			if source.Source.NoticeBasisPoints == 0 {
				continue
			}
			if source.PolicyEventID != policy.EventID {
				return nil, core.NewError(core.CodeProjectionDiverged, "store opportunity stream differs")
			}
			var existing *rpStoreOpportunity
			cooling := false
			for i := range history {
				old := &history[i]
				if old.ActorID != npc || old.WorldTime < start {
					continue
				}
				same := old.Store.StorefrontSourceEventID == shelf.StorefrontSourceEventID && old.Store.StockSourceEventID == shelf.StockSourceEventID
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
			recent, sharedCooldown, err := rpOpportunityHistoryPressure(append(allHistory, rpWaitEvent{StoreOpportunities: result}), npc, at, policy.Fact.Policy)
			if err != nil {
				return nil, err
			}
			cooling = cooling || sharedCooldown
			major, err := readRPOpportunityMajorChanges(ctx, conn, session.InstanceID, session.BranchID, npc, start, target)
			if err != nil {
				return nil, err
			}
			quiet, err := readRPQuietOpportunityHistory(ctx, conn, input, policy, at, recent, len(major))
			if err != nil {
				return nil, err
			}
			chance, err := core.RPOpportunityProbability(source.Source.NoticeBasisPoints, false, core.RPOpportunityPressure{Quiet: quiet, Busy: busy, CoolingDown: cooling, RecentChanges: recent, RecentMajor: len(major) > 0})
			if err != nil {
				return nil, err
			}
			identity, err := core.HashJSON([]string{shelf.StorefrontSourceEventID, shelf.StockSourceEventID})
			if err != nil {
				return nil, err
			}
			draw, err := core.DrawRPOpportunity(core.RPOpportunityDrawKey{PolicyEventID: policy.EventID, StreamSeed: policy.Fact.Policy.StreamSeed, InstanceID: session.InstanceID, BranchID: session.BranchID, ActorID: npc, Kind: "store_shortage", SourceEventID: identity, WorldTime: target}, chance)
			if err != nil {
				return nil, err
			}
			result = append(result, rpStoreOpportunity{Quiet: quiet, MajorChangeSourceEventIDs: major, ActorID: npc, Store: shelf, WorldTime: target, Busy: busy, CoolingDown: cooling, RecentChanges: recent, Draw: draw})
		}
	}
	return result, nil
}
