package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"corerp.local/backend/internal/core"
)

// Caller supplies the authorized actor's actual current place and world time.
// Reads never draw weather or manufacture persistent knowledge/observations.
func readRPLocalEnvironment(ctx context.Context, conn *sql.Conn, instance, branch, place, at string) (*core.RPLocalEnvironment, error) {
	var raw string
	err := conn.QueryRowContext(ctx, `SELECT json_extract(payload,'$.environment') FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPWaitCompleted' AND world_time<=? AND json_extract(payload,'$.environment.place_id')=? AND json_extract(payload,'$.environment.world_time')<=? AND json_extract(payload,'$.environment.until_world_time')>? ORDER BY event_sequence DESC LIMIT 1`, instance, branch, at, place, at, at).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var condition rpEnvironmentCondition
	if err := json.Unmarshal([]byte(raw), &condition); err != nil {
		return nil, err
	}
	if condition.OriginEventID == "" || (condition.Condition != "rain" && condition.Condition != "clear") {
		return nil, core.NewError(core.CodeProjectionDiverged, "invalid local environment evidence")
	}
	return &core.RPLocalEnvironment{PlaceID: condition.PlaceID, SourceEventID: condition.OriginEventID, Condition: condition.Condition, EffectiveWorldTime: condition.WorldTime, UntilWorldTime: condition.UntilWorldTime}, nil
}
