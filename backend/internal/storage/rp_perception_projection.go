package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"

	"corerp.local/backend/internal/core"
)

type rpPerceptionLinkProjection struct {
	LinkID   string `json:"link_id"`
	Instance string `json:"instance"`
	Branch   string `json:"branch"`
	PlaceID  string `json:"place_id"`
	ZoneA    string `json:"zone_a"`
	ZoneB    string `json:"zone_b"`
	Kind     string `json:"kind"`
	State    string `json:"state"`
	Distance int    `json:"distance"`
	Visual   int    `json:"visual"`
	Audio    int    `json:"audio"`
	SourceID string `json:"source_id"`
}

type rpActorZoneProjection struct {
	AgentID  string `json:"agent_id"`
	Instance string `json:"instance"`
	Branch   string `json:"branch"`
	PlaceID  string `json:"place_id"`
	EntryID  string `json:"entry_id"`
	ZoneKey  string `json:"zone_key"`
	SourceID string `json:"source_id"`
}

func rpPerceptionExpected(ctx context.Context, q replayQuerier, instance, branch string, through int64) (map[string]rpPerceptionLinkProjection, map[string]rpActorZoneProjection, error) {
	rows, err := q.QueryContext(ctx, `SELECT event_id,event_type,payload FROM events WHERE instance_id=? AND branch_id=? AND event_sequence<=? AND event_type IN ('RPPerceptionLinkDefined','RPActorZoneChosen') ORDER BY event_sequence`, instance, branch, through)
	if err != nil {
		return nil, nil, err
	}
	links := map[string]rpPerceptionLinkProjection{}
	zones := map[string]rpActorZoneProjection{}
	for rows.Next() {
		var id, kind, raw string
		if err := rows.Scan(&id, &kind, &raw); err != nil {
			rows.Close()
			return nil, nil, err
		}
		if kind == "RPPerceptionLinkDefined" {
			var fact RPPerceptionLinkFact
			if err := json.Unmarshal([]byte(raw), &fact); err != nil {
				rows.Close()
				return nil, nil, err
			}
			hash, err := core.HashJSON([]string{instance, branch, fact.PlaceID, fact.ZoneA, fact.ZoneB})
			if err != nil {
				rows.Close()
				return nil, nil, err
			}
			if fact.Version != "corerp.spatial.perception-link.v1" || fact.LinkID != "rp_perception_"+hash[7:] || !rpZoneKey.MatchString(fact.ZoneA) || !rpZoneKey.MatchString(fact.ZoneB) || fact.ZoneA >= fact.ZoneB ||
				(fact.BarrierKind != "open" && fact.BarrierKind != "door" && fact.BarrierKind != "wall") || (fact.BarrierState != "open" && fact.BarrierState != "closed") ||
				fact.DistanceM < 0 || fact.DistanceM > 1000 || fact.VisualRangeM < 0 || fact.VisualRangeM > 1000 || fact.AudioRangeM < 0 || fact.AudioRangeM > 1000 {
				rows.Close()
				return nil, nil, core.NewError(core.CodeProjectionDiverged, "invalid perception link source")
			}
			if _, exists := links[fact.LinkID]; exists {
				rows.Close()
				return nil, nil, core.NewError(core.CodeProjectionDiverged, "duplicate perception link source")
			}
			links[fact.LinkID] = rpPerceptionLinkProjection{fact.LinkID, instance, branch, fact.PlaceID, fact.ZoneA, fact.ZoneB, fact.BarrierKind, fact.BarrierState, fact.DistanceM, fact.VisualRangeM, fact.AudioRangeM, id}
			continue
		}
		var fact RPActorZoneFact
		if err := json.Unmarshal([]byte(raw), &fact); err != nil {
			rows.Close()
			return nil, nil, err
		}
		if fact.Version != "corerp.spatial.actor-zone.v1" || fact.AgentID == "" || fact.PlaceID == "" || fact.EntryEventID == "" || !rpZoneKey.MatchString(fact.ZoneKey) {
			rows.Close()
			return nil, nil, core.NewError(core.CodeProjectionDiverged, "invalid actor zone source")
		}
		zones[fact.AgentID] = rpActorZoneProjection{fact.AgentID, instance, branch, fact.PlaceID, fact.EntryEventID, fact.ZoneKey, id}
	}
	err = rows.Err()
	rows.Close()
	return links, zones, err
}

func rpPerceptionProjectionDifferences(ctx context.Context, q replayQuerier, instance, branch string, through int64) ([]ProjectionDifference, error) {
	links, zones, err := rpPerceptionExpected(ctx, q, instance, branch, through)
	if err != nil {
		return nil, err
	}
	var differences []ProjectionDifference
	compare := func(kind, id string, want, got any, readErr error) error {
		if readErr != nil && !errors.Is(readErr, sql.ErrNoRows) {
			return readErr
		}
		expected, err := core.CanonicalJSON(want)
		if err != nil {
			return err
		}
		actual := "missing"
		if readErr == nil {
			encoded, err := core.CanonicalJSON(got)
			if err != nil {
				return err
			}
			actual = string(encoded)
		}
		if string(expected) != actual {
			differences = append(differences, ProjectionDifference{Projection: kind, Key: id, ExpectedText: string(expected), ActualText: actual})
		}
		return nil
	}
	keys := make([]string, 0, len(links))
	for id := range links {
		keys = append(keys, id)
	}
	sort.Strings(keys)
	for _, id := range keys {
		var actual rpPerceptionLinkProjection
		err := q.QueryRowContext(ctx, `SELECT link_id,instance_id,branch_id,place_id,zone_a,zone_b,barrier_kind,barrier_state,distance_m,visual_range_m,audio_range_m,definition_event_id FROM rp_perception_links WHERE link_id=?`, id).Scan(&actual.LinkID, &actual.Instance, &actual.Branch, &actual.PlaceID, &actual.ZoneA, &actual.ZoneB, &actual.Kind, &actual.State, &actual.Distance, &actual.Visual, &actual.Audio, &actual.SourceID)
		if err := compare("rp_perception_link", id, links[id], actual, err); err != nil {
			return nil, err
		}
	}
	keys = keys[:0]
	for id := range zones {
		keys = append(keys, id)
	}
	sort.Strings(keys)
	for _, id := range keys {
		var actual rpActorZoneProjection
		err := q.QueryRowContext(ctx, `SELECT agent_id,instance_id,branch_id,place_id,entry_event_id,zone_key,source_event_id FROM rp_actor_zones WHERE agent_id=?`, id).Scan(&actual.AgentID, &actual.Instance, &actual.Branch, &actual.PlaceID, &actual.EntryID, &actual.ZoneKey, &actual.SourceID)
		if err := compare("rp_actor_zone", id, zones[id], actual, err); err != nil {
			return nil, err
		}
	}
	for _, scope := range []struct {
		query, projection string
		known             func(string) bool
	}{
		{`SELECT link_id FROM rp_perception_links WHERE instance_id=? AND branch_id=? ORDER BY link_id`, "rp_perception_link_extra", func(id string) bool { _, ok := links[id]; return ok }},
		{`SELECT agent_id FROM rp_actor_zones WHERE instance_id=? AND branch_id=? ORDER BY agent_id`, "rp_actor_zone_extra", func(id string) bool { _, ok := zones[id]; return ok }},
	} {
		rows, err := q.QueryContext(ctx, scope.query, instance, branch)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return nil, err
			}
			if !scope.known(id) {
				differences = append(differences, ProjectionDifference{Projection: scope.projection, Key: id, ExpectedText: "absent", ActualText: "unsourced projection row"})
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	return differences, nil
}

func repairRPPerceptionProjections(ctx context.Context, conn *sql.Conn, instance, branch string, differences []ProjectionDifference) error {
	for _, difference := range differences {
		switch difference.Projection {
		case "rp_perception_link_extra", "rp_actor_zone_extra":
			return core.NewError(core.CodeProjectionDiverged, "cannot infer authority for extra perception projection row")
		case "rp_perception_link":
			var row rpPerceptionLinkProjection
			if err := json.Unmarshal([]byte(difference.ExpectedText), &row); err != nil {
				return err
			}
			if row.Instance != instance || row.Branch != branch || row.LinkID != difference.Key {
				return core.NewError(core.CodeProjectionDiverged, "perception link repair scope differs")
			}
			if err := execAgentOne(ctx, conn, "repair perception link", `INSERT INTO rp_perception_links(link_id,instance_id,branch_id,place_id,zone_a,zone_b,barrier_kind,barrier_state,distance_m,visual_range_m,audio_range_m,definition_event_id) VALUES (?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(link_id) DO UPDATE SET place_id=excluded.place_id,zone_a=excluded.zone_a,zone_b=excluded.zone_b,barrier_kind=excluded.barrier_kind,barrier_state=excluded.barrier_state,distance_m=excluded.distance_m,visual_range_m=excluded.visual_range_m,audio_range_m=excluded.audio_range_m,definition_event_id=excluded.definition_event_id WHERE rp_perception_links.instance_id=excluded.instance_id AND rp_perception_links.branch_id=excluded.branch_id`, row.LinkID, instance, branch, row.PlaceID, row.ZoneA, row.ZoneB, row.Kind, row.State, row.Distance, row.Visual, row.Audio, row.SourceID); err != nil {
				return err
			}
		case "rp_actor_zone":
			var row rpActorZoneProjection
			if err := json.Unmarshal([]byte(difference.ExpectedText), &row); err != nil {
				return err
			}
			if row.Instance != instance || row.Branch != branch || row.AgentID != difference.Key {
				return core.NewError(core.CodeProjectionDiverged, "actor zone repair scope differs")
			}
			if err := execAgentOne(ctx, conn, "repair actor zone", `INSERT INTO rp_actor_zones(agent_id,instance_id,branch_id,place_id,entry_event_id,zone_key,source_event_id) VALUES (?,?,?,?,?,?,?) ON CONFLICT(agent_id) DO UPDATE SET place_id=excluded.place_id,entry_event_id=excluded.entry_event_id,zone_key=excluded.zone_key,source_event_id=excluded.source_event_id WHERE rp_actor_zones.instance_id=excluded.instance_id AND rp_actor_zones.branch_id=excluded.branch_id`, row.AgentID, instance, branch, row.PlaceID, row.EntryID, row.ZoneKey, row.SourceID); err != nil {
				return err
			}
		}
	}
	return nil
}
