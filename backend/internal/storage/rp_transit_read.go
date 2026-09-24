package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"corerp.local/backend/internal/core"
)

func readRPLocalTransitWorks(ctx context.Context, conn *sql.Conn, instance, branch, place, at string) ([]core.RPLocalTransitWorks, error) {
	rows, err := conn.QueryContext(ctx, `SELECT event_id,json_extract(payload,'$.window.from_place_id'),json_extract(payload,'$.window.to_place_id'),json_extract(payload,'$.window.ends_at') FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPTransitWorksDefined' AND json_extract(payload,'$.window.starts_at')<=? AND json_extract(payload,'$.window.ends_at')>? AND (json_extract(payload,'$.window.from_place_id')=? OR json_extract(payload,'$.window.to_place_id')=?) ORDER BY event_sequence LIMIT 16`, instance, branch, at, at, place, place)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []core.RPLocalTransitWorks
	for rows.Next() {
		var item core.RPLocalTransitWorks
		if err := rows.Scan(&item.SourceEventID, &item.FromPlaceID, &item.ToPlaceID, &item.EndsAt); err != nil {
			return nil, err
		}
		if item.ToPlaceID == place {
			item.FromPlaceID, item.ToPlaceID = item.ToPlaceID, item.FromPlaceID
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func rpTransitAllowsImmediate(ctx context.Context, conn *sql.Conn, instance, branch, from, to, at string) (bool, error) {
	var count int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPTransitWorksDefined' AND json_extract(payload,'$.window.ends_at')>?`, instance, branch, at).Scan(&count); err != nil {
		return false, err
	}
	if count == 0 {
		return true, nil
	}
	arrival, err := readRPTransitArrival(ctx, conn, instance, branch, from, to, at)
	if err != nil {
		return false, err
	}
	start, err := time.Parse(time.RFC3339, at)
	if err != nil {
		return false, err
	}
	if !arrival.Reachable {
		return false, nil
	}
	end, err := time.Parse(time.RFC3339, arrival.WorldTime)
	if err != nil {
		return false, err
	}
	return !end.After(start), nil
}

// Internal physical route calculation, not an omniscient actor-facing feed.
// Close each result set before opening the next on the single SQLite connection.
func readRPTransitArrival(ctx context.Context, conn *sql.Conn, instance, branch, from, to, at string) (core.RPTransitArrival, error) {
	target, err := time.Parse(time.RFC3339, at)
	if err != nil {
		return core.RPTransitArrival{}, core.NewError(core.CodeInvalidArgument, "invalid transit time")
	}
	rows, err := conn.QueryContext(ctx, `SELECT l.from_place_id,l.to_place_id FROM rp_place_links l JOIN agent_places a ON a.place_id=l.from_place_id JOIN agent_places b ON b.place_id=l.to_place_id WHERE l.instance_id=? AND l.branch_id=? AND a.instance_id=l.instance_id AND a.branch_id=l.branch_id AND b.instance_id=l.instance_id AND b.branch_id=l.branch_id AND a.status='active' AND b.status='active' ORDER BY l.from_place_id,l.to_place_id LIMIT 4097`, instance, branch)
	if err != nil {
		return core.RPTransitArrival{}, err
	}
	var edges []core.RPTransitEdge
	for rows.Next() {
		var edge core.RPTransitEdge
		if err := rows.Scan(&edge.FromPlaceID, &edge.ToPlaceID); err != nil {
			rows.Close()
			return core.RPTransitArrival{}, err
		}
		edges = append(edges, edge)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return core.RPTransitArrival{}, err
	}
	rows, err = conn.QueryContext(ctx, `SELECT event_id,payload FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPTransitWorksDefined' AND json_extract(payload,'$.window.ends_at')>? ORDER BY event_sequence LIMIT 4097`, instance, branch, target.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return core.RPTransitArrival{}, err
	}
	var windows []core.RPTransitWindow
	for rows.Next() {
		var id, raw string
		var fact TransitWorksFact
		if err := rows.Scan(&id, &raw); err != nil {
			rows.Close()
			return core.RPTransitArrival{}, err
		}
		if err := json.Unmarshal([]byte(raw), &fact); err != nil {
			rows.Close()
			return core.RPTransitArrival{}, err
		}
		if fact.Window.SourceEventID != id {
			rows.Close()
			return core.RPTransitArrival{}, core.NewError(core.CodeProjectionDiverged, "transit window source differs")
		}
		windows = append(windows, fact.Window)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return core.RPTransitArrival{}, err
	}
	return core.EarliestRPTransitArrival(edges, windows, from, to, at)
}
