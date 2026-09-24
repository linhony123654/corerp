package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"corerp.local/backend/internal/core"
)

// Shared occurrence evidence is kept separate from personal opportunity rolls.
// OriginEventID pins the first accepted result even when later waits copy it.
type rpEnvironmentCondition struct {
	PlaceID        string                 `json:"place_id"`
	SourceEventID  string                 `json:"source_event_id"`
	OriginEventID  string                 `json:"origin_event_id"`
	WorldTime      string                 `json:"world_time"`
	UntilWorldTime string                 `json:"until_world_time"`
	Condition      string                 `json:"condition"`
	CoolingDown    bool                   `json:"cooling_down"`
	Draw           core.RPOpportunityDraw `json:"draw"`
}

func materializeRPEnvironment(ctx context.Context, conn *sql.Conn, instance, branch, place, target, eventID string) (*rpEnvironmentCondition, error) {
	var sourceID, raw string
	err := conn.QueryRowContext(ctx, `SELECT event_id,payload FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPEnvironmentSourceDefined' AND json_extract(payload,'$.source.place_id')=? ORDER BY event_sequence LIMIT 1`, instance, branch, place).Scan(&sourceID, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var source EnvironmentSourceFact
	if err := json.Unmarshal([]byte(raw), &source); err != nil {
		return nil, err
	}
	if err := source.Source.validate(); err != nil {
		return nil, err
	}
	policy, err := readRPOpportunityPolicy(ctx, conn, instance, branch)
	if err != nil {
		return nil, err
	}
	if source.PolicyEventID != policy.EventID {
		return nil, core.NewError(core.CodeProjectionDiverged, "environment stream source differs")
	}
	at, err := time.Parse(time.RFC3339, target)
	if err != nil {
		return nil, err
	}
	window := at.UTC().Truncate(time.Hour)
	// Include the full current hour even when the source cooldown is short.
	start := at.Add(-time.Duration(source.Source.CooldownHours) * time.Hour).UTC().Format(time.RFC3339)
	rows, err := conn.QueryContext(ctx, `SELECT json_extract(payload,'$.environment') FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPWaitCompleted' AND world_time>=? AND world_time<=? AND json_extract(payload,'$.environment.place_id')=? ORDER BY event_sequence`, instance, branch, start, target, place)
	if err != nil {
		return nil, err
	}
	cooling := false
	var existing *rpEnvironmentCondition
	for rows.Next() {
		var encoded string
		var old rpEnvironmentCondition
		if err := rows.Scan(&encoded); err != nil {
			rows.Close()
			return nil, err
		}
		if err := json.Unmarshal([]byte(encoded), &old); err != nil {
			rows.Close()
			return nil, err
		}
		if old.SourceEventID != sourceID {
			continue
		}
		if old.Draw.WindowStart == window.Format(time.RFC3339) && existing == nil {
			copy := old
			existing = &copy
		}
		previous, err := time.Parse(time.RFC3339, old.WorldTime)
		if err != nil {
			rows.Close()
			return nil, err
		}
		if old.Condition == "rain" && at.Before(previous.Add(time.Duration(source.Source.CooldownHours)*time.Hour)) {
			cooling = true
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil
	}
	chance := source.Source.RainBasisPoints
	if cooling {
		chance = 0
	}
	// Domain-separated shared scope, not the requesting actor or session.
	placeIdentity, err := core.HashJSON([]string{"place", place})
	if err != nil {
		return nil, err
	}
	draw, err := core.DrawRPOpportunity(core.RPOpportunityDrawKey{PolicyEventID: policy.EventID, StreamSeed: policy.Fact.Policy.StreamSeed, InstanceID: instance, BranchID: branch, ActorID: placeIdentity, Kind: "local_environment", SourceEventID: sourceID, WorldTime: target}, chance)
	if err != nil {
		return nil, err
	}
	condition := "clear"
	if draw.Selected {
		condition = "rain"
	}
	return &rpEnvironmentCondition{PlaceID: place, SourceEventID: sourceID, OriginEventID: eventID, WorldTime: target, UntilWorldTime: window.Add(time.Hour).Format(time.RFC3339), Condition: condition, CoolingDown: cooling, Draw: draw}, nil
}
