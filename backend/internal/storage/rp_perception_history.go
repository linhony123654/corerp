package storage

import (
	"context"
	"database/sql"
	"errors"
)

// rpCanSeeAtSequence reconstructs visual access from immutable movement and
// spatial-definition events. Narrative reads need the scene at the fact's
// sequence, not the actors' current projections after later movement.
func rpCanSeeAtSequence(ctx context.Context, conn *sql.Conn, instance, branch, observer, subject string, sequence int64) (bool, error) {
	if observer == subject {
		return true, nil
	}
	observerPlace, observerZone, err := rpSpatialPointAtSequence(ctx, conn, instance, branch, observer, sequence)
	if err != nil {
		return false, err
	}
	subjectPlace, subjectZone, err := rpSpatialPointAtSequence(ctx, conn, instance, branch, subject, sequence)
	if err != nil {
		return false, err
	}
	if observerPlace != subjectPlace {
		return false, nil
	}
	if observerZone == subjectZone {
		return true, nil
	}
	a, b := observerZone, subjectZone
	if a > b {
		a, b = b, a
	}
	var kind, state string
	var distance, visualRange int
	err = conn.QueryRowContext(ctx, `SELECT l.barrier_kind,l.barrier_state,l.distance_m,l.visual_range_m
		FROM rp_perception_links l JOIN events source ON source.event_id=l.definition_event_id
		WHERE l.instance_id=? AND l.branch_id=? AND l.place_id=? AND l.zone_a=? AND l.zone_b=?
			AND source.instance_id=? AND source.branch_id=? AND source.event_sequence<=?`,
		instance, branch, observerPlace, a, b, instance, branch, sequence).Scan(&kind, &state, &distance, &visualRange)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return kind != "wall" && state != "closed" && distance <= visualRange, nil
}

func rpSpatialPointAtSequence(ctx context.Context, conn *sql.Conn, instance, branch, actor string, sequence int64) (string, string, error) {
	var place, entryID string
	err := conn.QueryRowContext(ctx, `SELECT movement.to_place_id,movement.event_id
		FROM agent_movements movement JOIN events source ON source.event_id=movement.event_id
		WHERE movement.agent_id=? AND source.instance_id=? AND source.branch_id=? AND source.event_sequence<=?
		ORDER BY source.event_sequence DESC LIMIT 1`, actor, instance, branch, sequence).Scan(&place, &entryID)
	if err != nil {
		return "", "", classifyMissing(err, "historical RP actor position")
	}
	zone := "main"
	err = conn.QueryRowContext(ctx, `SELECT json_extract(source.payload,'$.zone_key')
		FROM events source WHERE source.instance_id=? AND source.branch_id=? AND source.event_sequence<=?
			AND source.event_type='RPActorZoneChosen'
			AND json_extract(source.payload,'$.agent_id')=?
			AND json_extract(source.payload,'$.entry_event_id')=?
			AND json_extract(source.payload,'$.place_id')=?
		ORDER BY source.event_sequence DESC LIMIT 1`, instance, branch, sequence, actor, entryID, place).Scan(&zone)
	if errors.Is(err, sql.ErrNoRows) {
		return place, "main", nil
	}
	return place, zone, err
}
