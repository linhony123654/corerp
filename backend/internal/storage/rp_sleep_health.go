package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"corerp.local/backend/internal/core"
)

// The private truth is reconstructed from the actor's accepted sleep Events.
// It is not serialized into RPDecisionInput or inferred from global waiting.
type rpSleepTruth struct {
	OnsetEventID     string
	SleepDebtMinutes int64
	FatigueLevel     string
}

func readRPSleepTruth(ctx context.Context, conn *sql.Conn, instance, branch, entity, through string) (rpSleepTruth, error) {
	rows, err := conn.QueryContext(ctx, `SELECT event_id,world_time,payload FROM events WHERE instance_id=? AND branch_id=?
		AND event_type='RPSleepEnded' AND json_extract(payload,'$.entity_id')=? AND world_time<=?
		ORDER BY event_sequence DESC LIMIT 2`, instance, branch, entity, through)
	if err != nil {
		return rpSleepTruth{}, err
	}
	type source struct {
		id   string
		at   string
		fact RPSleepEndFact
	}
	var recent []source
	for rows.Next() {
		var row source
		var raw string
		if err := rows.Scan(&row.id, &row.at, &raw); err != nil {
			rows.Close()
			return rpSleepTruth{}, err
		}
		if err := json.Unmarshal([]byte(raw), &row.fact); err != nil {
			rows.Close()
			return rpSleepTruth{}, err
		}
		recent = append(recent, row)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return rpSleepTruth{}, err
	}
	for _, row := range recent {
		ended, endErr := time.Parse(time.RFC3339Nano, row.at)
		effective, effectiveErr := time.Parse(time.RFC3339Nano, row.fact.EffectiveEndWorldTime)
		started, startErr := time.Parse(time.RFC3339Nano, row.fact.StartedWorldTime)
		if endErr != nil || effectiveErr != nil || startErr != nil || row.fact.Version != "corerp.sleep.end.v1" ||
			row.fact.EntityID != entity || row.fact.StartEventID == "" || row.fact.RestMinutes < 0 || row.fact.RestMinutes > 720 ||
			started.After(effective) || effective.After(ended) || int64(effective.Sub(started)/time.Minute) != row.fact.RestMinutes {
			return rpSleepTruth{}, core.NewError(core.CodeProjectionDiverged, "invalid sourced RP sleep interval")
		}
		var valid int
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE event_id=? AND instance_id=? AND branch_id=?
			AND event_type='RPSleepStarted' AND world_time=? AND json_extract(payload,'$.entity_id')=?`,
			row.fact.StartEventID, instance, branch, row.fact.StartedWorldTime, entity).Scan(&valid); err != nil {
			return rpSleepTruth{}, err
		}
		if valid != 1 {
			return rpSleepTruth{}, core.NewError(core.CodeProjectionDiverged, "RP sleep end has no matching start")
		}
	}
	if len(recent) < 2 || recent[0].fact.RestMinutes >= 360 || recent[1].fact.RestMinutes >= 360 {
		return rpSleepTruth{}, nil
	}
	newest, _ := time.Parse(time.RFC3339Nano, recent[0].fact.EffectiveEndWorldTime)
	previous, _ := time.Parse(time.RFC3339Nano, recent[1].fact.EffectiveEndWorldTime)
	newestDay := time.Date(newest.UTC().Year(), newest.UTC().Month(), newest.UTC().Day(), 0, 0, 0, 0, time.UTC)
	previousDay := time.Date(previous.UTC().Year(), previous.UTC().Month(), previous.UTC().Day(), 0, 0, 0, 0, time.UTC)
	if newestDay.Sub(previousDay) != 24*time.Hour {
		return rpSleepTruth{}, nil
	}
	return rpSleepTruth{OnsetEventID: recent[0].id,
		SleepDebtMinutes: 720 - recent[0].fact.RestMinutes - recent[1].fact.RestMinutes, FatigueLevel: "moderate"}, nil
}

func readRPOwnSleepHealth(ctx context.Context, conn *sql.Conn, input core.RPDecisionInput) (*core.RPHealthSelf, error) {
	truth, err := readRPSleepTruth(ctx, conn, input.InstanceID, input.BranchID, input.NPCEntityID, input.WorldTime)
	if err != nil || truth.FatigueLevel == "" {
		return nil, err
	}
	return &core.RPHealthSelf{FatigueLevel: truth.FatigueLevel, SleepDebtLevel: "elevated",
		FunctionalImpact: "avoid_optional_evening_activity", Symptoms: []string{"tiredness"}}, nil
}
