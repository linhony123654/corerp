package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"

	"corerp.local/backend/internal/core"
)

type rpTimedEdgeProjection struct {
	EdgeID   string `json:"edge_id"`
	Instance string `json:"instance"`
	Branch   string `json:"branch"`
	From     string `json:"from"`
	To       string `json:"to"`
	Segment  string `json:"segment"`
	Minutes  int    `json:"minutes"`
	SourceID string `json:"source_id"`
}

type rpJourneyProjection struct {
	JourneyID  string `json:"journey_id"`
	Instance   string `json:"instance"`
	Branch     string `json:"branch"`
	AgentID    string `json:"agent_id"`
	EdgeID     string `json:"edge_id"`
	From       string `json:"from"`
	To         string `json:"to"`
	Segment    string `json:"segment"`
	StartedAt  string `json:"started_at"`
	ArrivalAt  string `json:"arrival_at"`
	Status     string `json:"status"`
	StartID    string `json:"start_id"`
	ScheduleID string `json:"schedule_id"`
	ItemID     string `json:"item_id"`
	ResolvedID string `json:"resolved_id"`
}

type rpReturnLinkProjection struct {
	LinkID   string `json:"link_id"`
	Instance string `json:"instance"`
	Branch   string `json:"branch"`
	From     string `json:"from"`
	To       string `json:"to"`
	SourceID string `json:"source_id"`
}

func rpJourneyExpected(ctx context.Context, q replayQuerier, instance, branch string, through int64) (map[string]rpTimedEdgeProjection, map[string]rpJourneyProjection, map[string]rpReturnLinkProjection, error) {
	rows, err := q.QueryContext(ctx, `SELECT event_id,event_type,world_time,payload FROM events
		WHERE instance_id=? AND branch_id=? AND event_sequence<=? AND event_type IN
		('RPTimedEdgeDefined','RPJourneyStarted','AgentJourneyStarted','RPJourneyDelayed','RPJourneyArrived','RPJourneyCancelled') ORDER BY event_sequence`, instance, branch, through)
	if err != nil {
		return nil, nil, nil, err
	}
	edges := map[string]rpTimedEdgeProjection{}
	journeys := map[string]rpJourneyProjection{}
	links := map[string]rpReturnLinkProjection{}
	for rows.Next() {
		var id, kind, at, raw string
		if err := rows.Scan(&id, &kind, &at, &raw); err != nil {
			rows.Close()
			return nil, nil, nil, err
		}
		switch kind {
		case "RPTimedEdgeDefined":
			var fact RPTimedEdgeFact
			if err := json.Unmarshal([]byte(raw), &fact); err != nil {
				rows.Close()
				return nil, nil, nil, err
			}
			hash, err := core.HashJSON([]string{instance, branch, fact.FromPlaceID, fact.ToPlaceID})
			if err != nil {
				rows.Close()
				return nil, nil, nil, err
			}
			if fact.Version != "corerp.spatial.timed-edge.v1" || fact.EdgeID != "rp_timed_edge_"+hash[7:] || fact.FromPlaceID == fact.ToPlaceID || fact.SegmentPlaceID == fact.FromPlaceID || fact.SegmentPlaceID == fact.ToPlaceID || fact.RouteSourceID == "" || fact.DurationMinutes < 1 || fact.DurationMinutes > 360 {
				rows.Close()
				return nil, nil, nil, core.NewError(core.CodeProjectionDiverged, "invalid timed-edge source")
			}
			if _, exists := edges[fact.EdgeID]; exists {
				rows.Close()
				return nil, nil, nil, core.NewError(core.CodeProjectionDiverged, "duplicate timed-edge source")
			}
			edges[fact.EdgeID] = rpTimedEdgeProjection{fact.EdgeID, instance, branch, fact.FromPlaceID, fact.ToPlaceID, fact.SegmentPlaceID, fact.DurationMinutes, id}
			links["rp_return_"+hash[7:]] = rpReturnLinkProjection{"rp_return_" + hash[7:], instance, branch, fact.SegmentPlaceID, fact.FromPlaceID, id}
		case "RPJourneyStarted", "AgentJourneyStarted":
			var fact rpJourneyStartEvent
			if err := json.Unmarshal([]byte(raw), &fact); err != nil {
				rows.Close()
				return nil, nil, nil, err
			}
			edge, exists := edges[fact.EdgeID]
			if !exists || fact.JourneyID == "" || fact.AgentID == "" || fact.ArrivalScheduleID == "" || fact.ArrivalItemID == "" || fact.ScheduledArrivalAt <= at || fact.FromPlaceID != edge.From || fact.ToPlaceID != edge.To || fact.SegmentPlaceID != edge.Segment {
				rows.Close()
				return nil, nil, nil, core.NewError(core.CodeProjectionDiverged, "invalid journey start source")
			}
			if _, exists := journeys[fact.JourneyID]; exists {
				rows.Close()
				return nil, nil, nil, core.NewError(core.CodeProjectionDiverged, "duplicate journey start source")
			}
			journeys[fact.JourneyID] = rpJourneyProjection{fact.JourneyID, instance, branch, fact.AgentID, fact.EdgeID, fact.FromPlaceID, fact.ToPlaceID, fact.SegmentPlaceID, at, fact.ScheduledArrivalAt, "active", id, fact.ArrivalScheduleID, fact.ArrivalItemID, ""}
		case "RPJourneyDelayed":
			var fact rpJourneyDelayEvent
			if err := json.Unmarshal([]byte(raw), &fact); err != nil {
				rows.Close()
				return nil, nil, nil, err
			}
			journey, exists := journeys[fact.JourneyID]
			if !exists || journey.Status != "active" || journey.AgentID != fact.AgentID || journey.ItemID != fact.OldItemID || journey.ArrivalAt != fact.OldArrivalAt || journey.Segment != fact.SegmentPlaceID || fact.NewItemID == "" || fact.NewArrivalAt <= fact.OldArrivalAt || len(fact.WorksSourceEventIDs) == 0 {
				rows.Close()
				return nil, nil, nil, core.NewError(core.CodeProjectionDiverged, "invalid journey delay source")
			}
			journey.ItemID, journey.ArrivalAt = fact.NewItemID, fact.NewArrivalAt
			journeys[fact.JourneyID] = journey
		case "RPJourneyArrived":
			var fact rpJourneyArrivalEvent
			if err := json.Unmarshal([]byte(raw), &fact); err != nil {
				rows.Close()
				return nil, nil, nil, err
			}
			journey, exists := journeys[fact.JourneyID]
			if !exists || journey.Status != "active" || journey.AgentID != fact.AgentID || journey.EdgeID != fact.EdgeID || journey.Segment != fact.FromPlaceID || journey.To != fact.ToPlaceID || journey.ArrivalAt != at {
				rows.Close()
				return nil, nil, nil, core.NewError(core.CodeProjectionDiverged, "invalid journey arrival source")
			}
			journey.Status, journey.ResolvedID = "arrived", id
			journeys[fact.JourneyID] = journey
		case "RPJourneyCancelled":
			var fact RPJourneyCancelFact
			if err := json.Unmarshal([]byte(raw), &fact); err != nil {
				rows.Close()
				return nil, nil, nil, err
			}
			journey, exists := journeys[fact.JourneyID]
			if !exists || journey.Status != "active" || journey.AgentID != fact.AgentID || journey.Segment != fact.SegmentPlaceID || journey.ItemID != fact.ArrivalItemID || journey.ScheduleID != fact.ScheduleID {
				rows.Close()
				return nil, nil, nil, core.NewError(core.CodeProjectionDiverged, "invalid journey cancellation source")
			}
			journey.Status, journey.ResolvedID = "cancelled", id
			journeys[fact.JourneyID] = journey
		}
	}
	err = rows.Err()
	rows.Close()
	return edges, journeys, links, err
}

func rpJourneyProjectionDifferences(ctx context.Context, q replayQuerier, instance, branch string, through int64) ([]ProjectionDifference, error) {
	edges, journeys, links, err := rpJourneyExpected(ctx, q, instance, branch, through)
	if err != nil {
		return nil, err
	}
	var differences []ProjectionDifference
	compare := func(projection, id string, expected, actual any, readErr error) error {
		if readErr != nil && !errors.Is(readErr, sql.ErrNoRows) {
			return readErr
		}
		want, err := core.CanonicalJSON(expected)
		if err != nil {
			return err
		}
		actualText := "missing"
		if readErr == nil {
			got, err := core.CanonicalJSON(actual)
			if err != nil {
				return err
			}
			actualText = string(got)
		}
		if string(want) != actualText {
			differences = append(differences, ProjectionDifference{Projection: projection, Key: id, ExpectedText: string(want), ActualText: actualText})
		}
		return nil
	}
	keys := make([]string, 0, len(edges))
	for id := range edges {
		keys = append(keys, id)
	}
	sort.Strings(keys)
	for _, id := range keys {
		var actual rpTimedEdgeProjection
		err := q.QueryRowContext(ctx, `SELECT edge_id,instance_id,branch_id,from_place_id,to_place_id,segment_place_id,duration_minutes,definition_event_id FROM rp_timed_edges WHERE edge_id=?`, id).Scan(&actual.EdgeID, &actual.Instance, &actual.Branch, &actual.From, &actual.To, &actual.Segment, &actual.Minutes, &actual.SourceID)
		if err := compare("rp_timed_edge", id, edges[id], actual, err); err != nil {
			return nil, err
		}
	}
	keys = keys[:0]
	for id := range links {
		keys = append(keys, id)
	}
	sort.Strings(keys)
	for _, id := range keys {
		var actual rpReturnLinkProjection
		err := q.QueryRowContext(ctx, `SELECT link_id,instance_id,branch_id,from_place_id,to_place_id,definition_event_id FROM rp_place_links WHERE link_id=?`, id).Scan(&actual.LinkID, &actual.Instance, &actual.Branch, &actual.From, &actual.To, &actual.SourceID)
		if err := compare("rp_timed_return", id, links[id], actual, err); err != nil {
			return nil, err
		}
	}
	keys = keys[:0]
	for id := range journeys {
		keys = append(keys, id)
	}
	sort.Strings(keys)
	for _, id := range keys {
		var actual rpJourneyProjection
		err := q.QueryRowContext(ctx, `SELECT journey_id,instance_id,branch_id,agent_id,edge_id,from_place_id,to_place_id,segment_place_id,started_at,scheduled_arrival_at,status,start_event_id,arrival_schedule_id,arrival_item_id,COALESCE(resolved_event_id,'') FROM rp_journeys WHERE journey_id=?`, id).Scan(&actual.JourneyID, &actual.Instance, &actual.Branch, &actual.AgentID, &actual.EdgeID, &actual.From, &actual.To, &actual.Segment, &actual.StartedAt, &actual.ArrivalAt, &actual.Status, &actual.StartID, &actual.ScheduleID, &actual.ItemID, &actual.ResolvedID)
		if err := compare("rp_journey", id, journeys[id], actual, err); err != nil {
			return nil, err
		}
	}
	for _, scope := range []struct {
		query, projection string
		known             func(string) bool
	}{
		{`SELECT edge_id FROM rp_timed_edges WHERE instance_id=? AND branch_id=? ORDER BY edge_id`, "rp_timed_edge_extra", func(id string) bool { _, ok := edges[id]; return ok }},
		{`SELECT journey_id FROM rp_journeys WHERE instance_id=? AND branch_id=? ORDER BY journey_id`, "rp_journey_extra", func(id string) bool { _, ok := journeys[id]; return ok }},
		{`SELECT link_id FROM rp_place_links WHERE instance_id=? AND branch_id=? AND substr(link_id,1,10)='rp_return_' ORDER BY link_id`, "rp_timed_return_extra", func(id string) bool { _, ok := links[id]; return ok }},
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

func repairRPJourneyProjections(ctx context.Context, conn *sql.Conn, instance, branch string, differences []ProjectionDifference) error {
	for _, difference := range differences {
		switch difference.Projection {
		case "rp_timed_edge_extra", "rp_journey_extra", "rp_timed_return_extra":
			return core.NewError(core.CodeProjectionDiverged, "cannot infer authority for extra spatial projection row")
		case "rp_timed_edge":
			var row rpTimedEdgeProjection
			if err := json.Unmarshal([]byte(difference.ExpectedText), &row); err != nil {
				return err
			}
			if row.Instance != instance || row.Branch != branch || row.EdgeID != difference.Key {
				return core.NewError(core.CodeProjectionDiverged, "timed-edge repair scope differs")
			}
			if err := execAgentOne(ctx, conn, "repair timed edge", `INSERT INTO rp_timed_edges(edge_id,instance_id,branch_id,from_place_id,to_place_id,segment_place_id,duration_minutes,definition_event_id) VALUES (?,?,?,?,?,?,?,?) ON CONFLICT(edge_id) DO UPDATE SET from_place_id=excluded.from_place_id,to_place_id=excluded.to_place_id,segment_place_id=excluded.segment_place_id,duration_minutes=excluded.duration_minutes,definition_event_id=excluded.definition_event_id WHERE rp_timed_edges.instance_id=excluded.instance_id AND rp_timed_edges.branch_id=excluded.branch_id`, row.EdgeID, instance, branch, row.From, row.To, row.Segment, row.Minutes, row.SourceID); err != nil {
				return err
			}
		case "rp_timed_return":
			var row rpReturnLinkProjection
			if err := json.Unmarshal([]byte(difference.ExpectedText), &row); err != nil {
				return err
			}
			if row.Instance != instance || row.Branch != branch || row.LinkID != difference.Key {
				return core.NewError(core.CodeProjectionDiverged, "return-link repair scope differs")
			}
			if err := execAgentOne(ctx, conn, "repair segment return link", `INSERT INTO rp_place_links(link_id,instance_id,branch_id,from_place_id,to_place_id,definition_event_id) VALUES (?,?,?,?,?,?) ON CONFLICT(link_id) DO UPDATE SET from_place_id=excluded.from_place_id,to_place_id=excluded.to_place_id,definition_event_id=excluded.definition_event_id WHERE rp_place_links.instance_id=excluded.instance_id AND rp_place_links.branch_id=excluded.branch_id`, row.LinkID, instance, branch, row.From, row.To, row.SourceID); err != nil {
				return err
			}
		case "rp_journey":
			var row rpJourneyProjection
			if err := json.Unmarshal([]byte(difference.ExpectedText), &row); err != nil {
				return err
			}
			if row.Instance != instance || row.Branch != branch || row.JourneyID != difference.Key {
				return core.NewError(core.CodeProjectionDiverged, "journey repair scope differs")
			}
			var resolved any
			if row.ResolvedID != "" {
				resolved = row.ResolvedID
			}
			if err := execAgentOne(ctx, conn, "repair journey state", `INSERT INTO rp_journeys(journey_id,instance_id,branch_id,agent_id,edge_id,from_place_id,to_place_id,segment_place_id,started_at,scheduled_arrival_at,status,start_event_id,arrival_schedule_id,arrival_item_id,resolved_event_id) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(journey_id) DO UPDATE SET agent_id=excluded.agent_id,edge_id=excluded.edge_id,from_place_id=excluded.from_place_id,to_place_id=excluded.to_place_id,segment_place_id=excluded.segment_place_id,started_at=excluded.started_at,scheduled_arrival_at=excluded.scheduled_arrival_at,status=excluded.status,start_event_id=excluded.start_event_id,arrival_schedule_id=excluded.arrival_schedule_id,arrival_item_id=excluded.arrival_item_id,resolved_event_id=excluded.resolved_event_id WHERE rp_journeys.instance_id=excluded.instance_id AND rp_journeys.branch_id=excluded.branch_id`, row.JourneyID, instance, branch, row.AgentID, row.EdgeID, row.From, row.To, row.Segment, row.StartedAt, row.ArrivalAt, row.Status, row.StartID, row.ScheduleID, row.ItemID, resolved); err != nil {
				return err
			}
		}
	}
	return nil
}
