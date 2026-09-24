package storage

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"time"

	"corerp.local/backend/internal/core"
)

// Read own visited public places and last actual encounters. Do not join a
// friend's current position, schedule or private residence to choose a visit.
func readRPVisitSources(ctx context.Context, conn *sql.Conn, input core.RPDecisionInput, start string) ([]core.RPVisitSource, error) {
	from, err := time.Parse(time.RFC3339, start)
	if err != nil {
		return nil, err
	}
	at, err := time.Parse(time.RFC3339, input.WorldTime)
	if err != nil {
		return nil, err
	}
	if from.After(at) {
		return nil, core.NewError(core.CodeInvalidArgument, "visit history starts after endpoint")
	}
	if input.Life == nil {
		return nil, nil
	}
	rows, err := conn.QueryContext(ctx, `WITH own_visits AS (
	 SELECT m.event_id,m.world_time,m.to_place_id,p.definition_event_id,e.event_sequence,
	 ROW_NUMBER() OVER (PARTITION BY m.to_place_id ORDER BY e.event_sequence DESC) AS recency
	 FROM agent_movements m JOIN events e ON e.event_id=m.event_id
	 JOIN agent_places p ON p.place_id=m.to_place_id AND p.instance_id=e.instance_id AND p.branch_id=e.branch_id
	 WHERE m.agent_id=? AND e.instance_id=? AND e.branch_id=? AND m.world_time>=? AND m.world_time<=?
	 AND p.status='active' AND p.place_kind='public' AND p.place_id<>?)
	 SELECT event_id,world_time,to_place_id,definition_event_id FROM own_visits WHERE recency=1 ORDER BY event_sequence DESC LIMIT 16`, input.NPCEntityID, input.InstanceID, input.BranchID, from.UTC().Format(time.RFC3339), at.UTC().Format(time.RFC3339), input.PlaceID)
	if err != nil {
		return nil, err
	}
	var ordinary []core.RPVisitSource
	for rows.Next() {
		source := core.RPVisitSource{ActorID: input.NPCEntityID, Kind: "familiar_public_place"}
		if err := rows.Scan(&source.MemorySourceEventID, &source.RememberedWorldTime, &source.PlaceID, &source.PlaceSourceEventID); err != nil {
			rows.Close()
			return nil, err
		}
		if core.RPVisitSourceEligible(input, source) {
			ordinary = append(ordinary, source)
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	var rare []core.RPVisitSource
	for _, relationship := range input.Life.Relationships {
		if relationship.Trust < 2 || relationship.Tension != 0 || len(relationship.SourceEventIDs) == 0 {
			continue
		}
		source := core.RPVisitSource{ActorID: input.NPCEntityID, Kind: "old_friend_place", FriendID: relationship.SubjectEntityID, RelationshipSourceEventID: relationship.SourceEventIDs[len(relationship.SourceEventIDs)-1]}
		var placeKind, placeStatus string
		// Select the latest encounter BEFORE checking public scope. A recent
		// private encounter must not resurrect an older public meeting.
		err := conn.QueryRowContext(ctx, `SELECT o.observation_id,o.source_event_id,o.observed_world_time,o.place_id,p.definition_event_id,p.place_kind,p.status
		 FROM observation_records o JOIN events e ON e.event_id=o.source_event_id
		 JOIN agent_places p ON p.place_id=o.place_id AND p.instance_id=e.instance_id AND p.branch_id=e.branch_id
		 WHERE o.observer_agent_id=? AND o.subject_agent_id=? AND e.instance_id=? AND e.branch_id=?
		 AND o.observed_world_time>=? AND o.observed_world_time<=?
		 ORDER BY o.observed_world_time DESC,e.event_sequence DESC,o.observation_id LIMIT 1`, input.NPCEntityID, source.FriendID, input.InstanceID, input.BranchID, from.UTC().Format(time.RFC3339), at.UTC().Format(time.RFC3339)).Scan(&source.ObservationID, &source.MemorySourceEventID, &source.RememberedWorldTime, &source.PlaceID, &source.PlaceSourceEventID, &placeKind, &placeStatus)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if placeKind != "public" || placeStatus != "active" || !core.RPVisitSourceEligible(input, source) {
			continue
		}
		var sourced int
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE event_id=? AND instance_id=? AND branch_id=? AND world_time<=?`, source.RelationshipSourceEventID, input.InstanceID, input.BranchID, at.UTC().Format(time.RFC3339)).Scan(&sourced); err != nil {
			return nil, err
		}
		if sourced != 1 {
			return nil, core.NewError(core.CodeProjectionDiverged, "visit relationship source differs from own scope/time")
		}
		rare = append(rare, source)
	}
	sort.Slice(rare, func(i, j int) bool {
		if rare[i].RememberedWorldTime != rare[j].RememberedWorldTime {
			return rare[i].RememberedWorldTime < rare[j].RememberedWorldTime
		}
		return rare[i].FriendID < rare[j].FriendID
	})
	if len(rare) > 16 {
		rare = rare[:16]
	}
	return append(rare, ordinary...), nil
}
