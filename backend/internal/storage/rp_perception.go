package storage

import (
	"context"
	"database/sql"

	"corerp.local/backend/internal/core"
)

type rpActorSpatialPoint struct {
	PlaceID string
	ZoneKey string
}

func readRPActorSpatialPoint(ctx context.Context, conn *sql.Conn, instance, branch, agent string) (rpActorSpatialPoint, error) {
	var point rpActorSpatialPoint
	err := conn.QueryRowContext(ctx, `SELECT p.place_id,COALESCE(z.zone_key,'main')
		FROM agent_profiles a JOIN agent_positions p ON p.agent_id=a.agent_id
		LEFT JOIN rp_actor_zones z ON z.agent_id=a.agent_id AND z.instance_id=a.instance_id AND z.branch_id=a.branch_id
		AND z.place_id=p.place_id AND z.entry_event_id=(
			SELECT m.event_id FROM agent_movements m JOIN events e ON e.event_id=m.event_id
			WHERE m.agent_id=a.agent_id AND e.instance_id=a.instance_id AND e.branch_id=a.branch_id
			ORDER BY e.event_sequence DESC LIMIT 1)
		WHERE a.agent_id=? AND a.instance_id=? AND a.branch_id=? AND a.status='active'`, agent, instance, branch).Scan(&point.PlaceID, &point.ZoneKey)
	if err != nil {
		return point, classifyMissing(err, "active actor spatial point")
	}
	return point, nil
}

// Perception is a source-backed relation, not a synonym for co-location.
// Legacy actors default to the main zone; an entry-tied zone choice never
// survives a later movement Event merely because the actor returned there.
func rpCanPerceive(ctx context.Context, conn *sql.Conn, instance, branch, observer, subject, sense, channel string) (bool, error) {
	if observer == subject {
		return true, nil
	}
	from, err := readRPActorSpatialPoint(ctx, conn, instance, branch, observer)
	if err != nil {
		return false, err
	}
	to, err := readRPActorSpatialPoint(ctx, conn, instance, branch, subject)
	if err != nil {
		return false, err
	}
	if from.PlaceID != to.PlaceID {
		return false, nil
	}
	if from.ZoneKey == to.ZoneKey {
		return true, nil
	}
	a, b := from.ZoneKey, to.ZoneKey
	if a > b {
		a, b = b, a
	}
	var kind, state string
	var distance, visualRange, audioRange int
	err = conn.QueryRowContext(ctx, `SELECT barrier_kind,barrier_state,distance_m,visual_range_m,audio_range_m
		FROM rp_perception_links WHERE instance_id=? AND branch_id=? AND place_id=? AND zone_a=? AND zone_b=?`, instance, branch, from.PlaceID, a, b).Scan(&kind, &state, &distance, &visualRange, &audioRange)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if kind == "wall" || (kind == "door" && state == "closed") || state == "closed" {
		return false, nil
	}
	if sense == "visual" {
		return distance <= visualRange, nil
	}
	if sense != "audio" {
		return false, core.NewError(core.CodeInvalidArgument, "unknown perception sense")
	}
	limit := 20
	switch channel {
	case "", "voice":
	case "whisper":
		limit = 3
	case "shout":
		limit = 50
	default:
		return false, core.NewError(core.CodeInvalidArgument, "unknown audio delivery channel")
	}
	return distance <= audioRange && distance <= limit, nil
}

func rpPerceivedEntityIDs(ctx context.Context, conn *sql.Conn, instance, branch, place, observer, sense, channel string) ([]string, error) {
	coLocated, err := rpCoLocatedEntityIDs(ctx, conn, instance, branch, place, observer)
	if err != nil {
		return nil, err
	}
	result := make([]string, 0, len(coLocated))
	for _, subject := range coLocated {
		allowed, err := rpCanPerceive(ctx, conn, instance, branch, observer, subject, sense, channel)
		if err != nil {
			return nil, err
		}
		if allowed {
			result = append(result, subject)
		}
	}
	return result, nil
}
