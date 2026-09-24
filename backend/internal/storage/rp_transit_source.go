package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"corerp.local/backend/internal/core"
)

// Creator construction contract consumed by all RP movement owners.
// Road works are an explicit conditional source, not a random loss of routes.
type TransitWorksRequest struct {
	Binding     core.CareerBinding `json:"binding"`
	FromPlaceID string             `json:"from_place_id"`
	ToPlaceID   string             `json:"to_place_id"`
	StartsAt    string             `json:"starts_at"`
	EndsAt      string             `json:"ends_at"`
}
type TransitWorksFact struct {
	Version                   string               `json:"version"`
	BuilderSourceEventID      string               `json:"builder_source_event_id"`
	ForwardRouteSourceEventID string               `json:"forward_route_source_event_id"`
	ReverseRouteSourceEventID string               `json:"reverse_route_source_event_id"`
	Window                    core.RPTransitWindow `json:"window"`
}
type TransitWorksRecord = privateFactRecord[TransitWorksFact]

func (s *Store) DefineRPTransitWorks(ctx context.Context, r TransitWorksRequest) (TransitWorksRecord, error) {
	for _, place := range []string{r.FromPlaceID, r.ToPlaceID} {
		if strings.TrimSpace(place) == "" || len(place) > 256 {
			return TransitWorksRecord{}, core.NewError(core.CodeInvalidArgument, "bounded transit endpoints required")
		}
	}
	start, e1 := time.Parse(time.RFC3339, r.StartsAt)
	end, e2 := time.Parse(time.RFC3339, r.EndsAt)
	if r.FromPlaceID == r.ToPlaceID || e1 != nil || e2 != nil || start.Nanosecond() != 0 || end.Nanosecond() != 0 || !end.After(start) || end.Sub(start) > 6*time.Hour {
		return TransitWorksRecord{}, core.NewError(core.CodeInvalidArgument, "route works require distinct endpoints and a positive interval up to six hours")
	}
	r.StartsAt, r.EndsAt = start.UTC().Format(time.RFC3339), end.UTC().Format(time.RFC3339)
	var builder string
	return executePrivateFactCommand(s, ctx, r.Binding, "DefineRPTransitWorks", r,
		privateFactDomain{"transit", "RPTransitWorksDefined", `{"authorization":"transit-builder-v1"}`},
		func(conn *sql.Conn) error {
			var err error
			builder, err = authorizeCultureBuilder(ctx, conn, r.Binding)
			return err
		},
		func(conn *sql.Conn, c privateFactContext) (TransitWorksFact, func() error, error) {
			fact := TransitWorksFact{Version: "corerp.transit.works.v1", BuilderSourceEventID: builder, Window: core.RPTransitWindow{SourceEventID: c.EventID, FromPlaceID: r.FromPlaceID, ToPlaceID: r.ToPlaceID, StartsAt: r.StartsAt, EndsAt: r.EndsAt}}
			now, err := time.Parse(time.RFC3339, c.WorldTime)
			if err != nil {
				return fact, nil, err
			}
			if !start.After(now) || start.After(now.Add(30*24*time.Hour)) {
				return fact, nil, core.NewError(core.CodeInvalidArgument, "route works must start in the next thirty days, strictly after current time")
			}
			for _, route := range []struct {
				from, to string
				source   *string
			}{{r.FromPlaceID, r.ToPlaceID, &fact.ForwardRouteSourceEventID}, {r.ToPlaceID, r.FromPlaceID, &fact.ReverseRouteSourceEventID}} {
				if err := conn.QueryRowContext(ctx, `SELECT l.definition_event_id FROM rp_place_links l JOIN agent_places a ON a.place_id=l.from_place_id JOIN agent_places b ON b.place_id=l.to_place_id WHERE l.instance_id=? AND l.branch_id=? AND l.from_place_id=? AND l.to_place_id=? AND a.instance_id=l.instance_id AND a.branch_id=l.branch_id AND b.instance_id=l.instance_id AND b.branch_id=l.branch_id AND a.status='active' AND b.status='active'`, r.Binding.InstanceID, r.Binding.BranchID, route.from, route.to).Scan(route.source); err != nil {
					return fact, nil, classifyMissing(err, "actual two-way transit route")
				}
			}
			rows, err := conn.QueryContext(ctx, `SELECT payload FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPTransitWorksDefined' AND ((json_extract(payload,'$.window.from_place_id')=? AND json_extract(payload,'$.window.to_place_id')=?) OR (json_extract(payload,'$.window.from_place_id')=? AND json_extract(payload,'$.window.to_place_id')=?))`, r.Binding.InstanceID, r.Binding.BranchID, r.FromPlaceID, r.ToPlaceID, r.ToPlaceID, r.FromPlaceID)
			if err != nil {
				return fact, nil, err
			}
			defer rows.Close()
			for rows.Next() {
				var raw string
				var previous TransitWorksFact
				if err := rows.Scan(&raw); err != nil {
					return fact, nil, err
				}
				if err := json.Unmarshal([]byte(raw), &previous); err != nil {
					return fact, nil, err
				}
				oldStart, err := time.Parse(time.RFC3339, previous.Window.StartsAt)
				if err != nil {
					return fact, nil, err
				}
				oldEnd, err := time.Parse(time.RFC3339, previous.Window.EndsAt)
				if err != nil {
					return fact, nil, err
				}
				if start.Before(oldEnd.Add(time.Hour)) && end.Add(time.Hour).After(oldStart) {
					return fact, nil, core.NewError(core.CodeBranchConflict, "route works overlap or violate one-hour route cooldown")
				}
			}
			if err := rows.Err(); err != nil {
				return fact, nil, err
			}
			return fact, nil, nil
		})
}
