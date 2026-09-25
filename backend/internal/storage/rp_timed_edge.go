package storage

import (
	"context"
	"database/sql"
	"strings"

	"corerp.local/backend/internal/core"
)

type RPTimedEdgeRequest struct {
	Binding         core.CareerBinding `json:"binding"`
	FromPlaceID     string             `json:"from_place_id"`
	ToPlaceID       string             `json:"to_place_id"`
	SegmentPlaceID  string             `json:"segment_place_id"`
	DurationMinutes int                `json:"duration_minutes"`
}

type RPTimedEdgeFact struct {
	Version         string `json:"version"`
	EdgeID          string `json:"edge_id"`
	RouteSourceID   string `json:"route_source_id"`
	FromPlaceID     string `json:"from_place_id"`
	ToPlaceID       string `json:"to_place_id"`
	SegmentPlaceID  string `json:"segment_place_id"`
	DurationMinutes int    `json:"duration_minutes"`
}

type RPTimedEdgeRecord = privateFactRecord[RPTimedEdgeFact]

// A timed overlay preserves RP5's declared route as the topology owner.
// Its midpoint is an already accepted empty location, not generated in this
// write transaction. Scheduled and player travel must both use this edge.
func (s *Store) DefineRPTimedEdge(ctx context.Context, r RPTimedEdgeRequest) (RPTimedEdgeRecord, error) {
	if err := r.Binding.Validate(); err != nil {
		return RPTimedEdgeRecord{}, err
	}
	for _, id := range []string{r.FromPlaceID, r.ToPlaceID, r.SegmentPlaceID} {
		if strings.TrimSpace(id) != id || id == "" || len(id) > 256 {
			return RPTimedEdgeRecord{}, core.NewError(core.CodeInvalidArgument, "bounded timed route endpoints required")
		}
	}
	if r.DurationMinutes < 1 || r.DurationMinutes > 360 || r.FromPlaceID == r.ToPlaceID || r.SegmentPlaceID == r.FromPlaceID || r.SegmentPlaceID == r.ToPlaceID {
		return RPTimedEdgeRecord{}, core.NewError(core.CodeInvalidArgument, "timed route needs distinct endpoints, segment and duration 1–360 minutes")
	}
	identity, err := core.HashJSON([]string{r.Binding.InstanceID, r.Binding.BranchID, r.FromPlaceID, r.ToPlaceID})
	if err != nil {
		return RPTimedEdgeRecord{}, err
	}
	edgeID := "rp_timed_edge_" + identity[7:]
	return executePrivateFactCommand(s, ctx, r.Binding, "DefineRPTimedEdge", r,
		privateFactDomain{"rp_timed_edge", "RPTimedEdgeDefined", `{"authorization":"scoped-world-creator"}`},
		func(conn *sql.Conn) error { return authorizeRPLocationBuilder(ctx, conn, r.Binding) },
		func(conn *sql.Conn, c privateFactContext) (RPTimedEdgeFact, func() error, error) {
			var routeSource string
			err := conn.QueryRowContext(ctx, `SELECT l.definition_event_id FROM rp_place_links l
				JOIN agent_places a ON a.place_id=l.from_place_id JOIN agent_places b ON b.place_id=l.to_place_id
				JOIN rp_location_nodes n ON n.location_id=?
				JOIN agent_places segment ON segment.place_id=n.location_id
				WHERE l.instance_id=? AND l.branch_id=? AND l.from_place_id=? AND l.to_place_id=?
				AND a.instance_id=l.instance_id AND a.branch_id=l.branch_id AND a.status='active'
				AND b.instance_id=l.instance_id AND b.branch_id=l.branch_id AND b.status='active'
				AND n.instance_id=l.instance_id AND n.branch_id=l.branch_id AND n.parent_location_id=l.from_place_id
				AND segment.instance_id=l.instance_id AND segment.branch_id=l.branch_id AND segment.status='active'`,
				r.SegmentPlaceID, r.Binding.InstanceID, r.Binding.BranchID, r.FromPlaceID, r.ToPlaceID).Scan(&routeSource)
			if err != nil {
				return RPTimedEdgeFact{}, nil, classifyMissing(err, "actual route and scoped segment")
			}
			var existing int
			if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_timed_edges WHERE instance_id=? AND branch_id=? AND from_place_id=? AND to_place_id=?`, r.Binding.InstanceID, r.Binding.BranchID, r.FromPlaceID, r.ToPlaceID).Scan(&existing); err != nil {
				return RPTimedEdgeFact{}, nil, err
			}
			if existing != 0 {
				return RPTimedEdgeFact{}, nil, core.NewError(core.CodeBranchConflict, "timed route already defined")
			}
			fact := RPTimedEdgeFact{Version: "corerp.spatial.timed-edge.v1", EdgeID: edgeID, RouteSourceID: routeSource, FromPlaceID: r.FromPlaceID, ToPlaceID: r.ToPlaceID, SegmentPlaceID: r.SegmentPlaceID, DurationMinutes: r.DurationMinutes}
			return fact, func() error {
				if err := execAgentOne(ctx, conn, "define timed transit edge", `INSERT INTO rp_timed_edges(edge_id,instance_id,branch_id,from_place_id,to_place_id,segment_place_id,duration_minutes,definition_event_id) VALUES (?,?,?,?,?,?,?,?)`, edgeID, r.Binding.InstanceID, r.Binding.BranchID, r.FromPlaceID, r.ToPlaceID, r.SegmentPlaceID, r.DurationMinutes, c.EventID); err != nil {
					return err
				}
				return execAgentOne(ctx, conn, "define segment return link", `INSERT INTO rp_place_links(link_id,instance_id,branch_id,from_place_id,to_place_id,definition_event_id) VALUES (?,?,?,?,?,?)`, "rp_return_"+identity[7:], r.Binding.InstanceID, r.Binding.BranchID, r.SegmentPlaceID, r.FromPlaceID, c.EventID)
			}, nil
		})
}

func readRPDirectWorksEnd(ctx context.Context, conn *sql.Conn, instance, branch, from, to, at string) (string, error) {
	var end sql.NullString
	err := conn.QueryRowContext(ctx, `SELECT MAX(json_extract(payload,'$.window.ends_at')) FROM events
		WHERE instance_id=? AND branch_id=? AND event_type='RPTransitWorksDefined'
		AND json_extract(payload,'$.window.starts_at')<=? AND json_extract(payload,'$.window.ends_at')>?
		AND ((json_extract(payload,'$.window.from_place_id')=? AND json_extract(payload,'$.window.to_place_id')=?)
		OR (json_extract(payload,'$.window.from_place_id')=? AND json_extract(payload,'$.window.to_place_id')=?))`,
		instance, branch, at, at, from, to, to, from).Scan(&end)
	if err != nil {
		return "", err
	}
	return end.String, nil
}
