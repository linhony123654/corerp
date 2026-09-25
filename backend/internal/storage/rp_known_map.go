package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"sort"

	"corerp.local/backend/internal/core"
)

// RPMapMemory is an actor's dated observation, not the current topology.
// A later rename, closure or roadworks Event never rewrites this fact.
type RPMapMemory struct {
	Version    string       `json:"version"`
	ObserverID string       `json:"observer_id"`
	PlaceID    string       `json:"place_id"`
	PlaceName  string       `json:"place_name"`
	ObservedAt string       `json:"observed_at"`
	Routes     []RPMapRoute `json:"routes"`
}

type RPMapRoute struct {
	ToPlaceID   string `json:"to_place_id"`
	DisplayName string `json:"display_name"`
	WorksUntil  string `json:"works_until,omitempty"`
}

type RPMapSurveyRequest struct {
	PrincipalID    string `json:"principal_id"`
	SessionID      string `json:"session_id"`
	ExpectedCursor int64  `json:"expected_cursor"`
	IdempotencyKey string `json:"idempotency_key"`
}

type RPMapSurveyRecord = privateFactRecord[RPMapMemory]

func (s *Store) SurveyRPMap(ctx context.Context, r RPMapSurveyRequest) (RPMapSurveyRecord, error) {
	if err := (core.RPSessionReadRequest{PrincipalID: r.PrincipalID, SessionID: r.SessionID}).Validate(); err != nil {
		return RPMapSurveyRecord{}, err
	}
	session, err := s.ReadRPSession(ctx, core.RPSessionReadRequest{PrincipalID: r.PrincipalID, SessionID: r.SessionID})
	if err != nil {
		return RPMapSurveyRecord{}, err
	}
	if session.Status != "active" {
		return RPMapSurveyRecord{}, core.NewError(core.CodeBranchConflict, "RP session is closed")
	}
	b := core.CareerBinding{PrincipalID: r.PrincipalID, InstanceID: session.InstanceID, BranchID: session.BranchID, ExpectedHead: r.ExpectedCursor, IdempotencyKey: r.IdempotencyKey}
	return executePrivateFactCommand(s, ctx, b, "SurveyRPMap", r,
		privateFactDomain{"rp_map", "RPMapSurveyed", `{"authorization":"rp-session-control"}`},
		func(conn *sql.Conn) error {
			current, err := loadRPSession(ctx, conn, r.PrincipalID, r.SessionID)
			if err != nil {
				return err
			}
			if current.InstanceID != b.InstanceID || current.BranchID != b.BranchID || current.ControlledEntityID != session.ControlledEntityID || current.Status != "active" {
				return core.NewError(core.CodeBranchConflict, "RP map session binding changed")
			}
			if err := authorizeRPControl(ctx, conn, r.PrincipalID, b.InstanceID, b.BranchID, current.ControlledEntityID); err != nil {
				return err
			}
			return validateRPBinding(ctx, conn, b.InstanceID, b.BranchID, current.ControlledEntityID)
		},
		func(conn *sql.Conn, c privateFactContext) (RPMapMemory, func() error, error) {
			var place, name string
			err := conn.QueryRowContext(ctx, `SELECT p.place_id,l.display_name FROM agent_positions p JOIN agent_places l ON l.place_id=p.place_id WHERE p.agent_id=? AND l.instance_id=? AND l.branch_id=? AND l.status='active'`, session.ControlledEntityID, b.InstanceID, b.BranchID).Scan(&place, &name)
			if err != nil {
				return RPMapMemory{}, nil, classifyMissing(err, "RP map observer position")
			}
			rows, err := conn.QueryContext(ctx, `SELECT l.to_place_id,p.display_name FROM rp_place_links l JOIN agent_places p ON p.place_id=l.to_place_id WHERE l.instance_id=? AND l.branch_id=? AND l.from_place_id=? AND p.instance_id=l.instance_id AND p.branch_id=l.branch_id AND p.status='active' ORDER BY l.to_place_id`, b.InstanceID, b.BranchID, place)
			if err != nil {
				return RPMapMemory{}, nil, err
			}
			routes := []RPMapRoute{}
			for rows.Next() {
				var route RPMapRoute
				if err := rows.Scan(&route.ToPlaceID, &route.DisplayName); err != nil {
					rows.Close()
					return RPMapMemory{}, nil, err
				}
				routes = append(routes, route)
			}
			err = rows.Err()
			rows.Close()
			if err != nil {
				return RPMapMemory{}, nil, err
			}
			for i := range routes {
				routes[i].WorksUntil, err = readRPDirectWorksEnd(ctx, conn, b.InstanceID, b.BranchID, place, routes[i].ToPlaceID, c.WorldTime)
				if err != nil {
					return RPMapMemory{}, nil, err
				}
			}
			return RPMapMemory{Version: "corerp.spatial.map-memory.v1", ObserverID: session.ControlledEntityID, PlaceID: place, PlaceName: name, ObservedAt: c.WorldTime, Routes: routes}, nil, nil
		})
}

// ReadRPMap returns the latest sourced survey per visited place. It does not
// consult live place names or works, so stale beliefs remain observable.
func (s *Store) ReadRPMap(ctx context.Context, r core.RPSessionReadRequest) ([]RPMapMemory, error) {
	if err := r.Validate(); err != nil {
		return nil, err
	}
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	session, err := loadRPSession(ctx, tx.conn, r.PrincipalID, r.SessionID)
	if err != nil {
		return nil, err
	}
	if err := authorizeRPControl(ctx, tx.conn, r.PrincipalID, session.InstanceID, session.BranchID, session.ControlledEntityID); err != nil {
		return nil, err
	}
	if err := validateRPBinding(ctx, tx.conn, session.InstanceID, session.BranchID, session.ControlledEntityID); err != nil {
		return nil, err
	}
	rows, err := tx.conn.QueryContext(ctx, `SELECT payload FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPMapSurveyed' AND json_extract(payload,'$.observer_id')=? ORDER BY event_sequence`, session.InstanceID, session.BranchID, session.ControlledEntityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	latest := map[string]RPMapMemory{}
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var memory RPMapMemory
		if err := json.Unmarshal([]byte(raw), &memory); err != nil {
			return nil, err
		}
		if memory.Version != "corerp.spatial.map-memory.v1" || memory.ObserverID != session.ControlledEntityID || memory.PlaceID == "" {
			return nil, core.NewError(core.CodeProjectionDiverged, "invalid map memory source")
		}
		latest[memory.PlaceID] = memory
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(latest))
	for key := range latest {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]RPMapMemory, 0, len(keys))
	for _, key := range keys {
		out = append(out, latest[key])
	}
	return out, nil
}
