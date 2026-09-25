package storage

import (
	"context"
	"database/sql"
	"regexp"
	"strings"

	"corerp.local/backend/internal/core"
)

var rpZoneKey = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)

type RPPerceptionLinkRequest struct {
	Binding      core.CareerBinding `json:"binding"`
	PlaceID      string             `json:"place_id"`
	ZoneA        string             `json:"zone_a"`
	ZoneB        string             `json:"zone_b"`
	BarrierKind  string             `json:"barrier_kind"`
	BarrierState string             `json:"barrier_state"`
	DistanceM    int                `json:"distance_m"`
	VisualRangeM int                `json:"visual_range_m"`
	AudioRangeM  int                `json:"audio_range_m"`
}

type RPPerceptionLinkFact struct {
	Version      string `json:"version"`
	LinkID       string `json:"link_id"`
	PlaceID      string `json:"place_id"`
	ZoneA        string `json:"zone_a"`
	ZoneB        string `json:"zone_b"`
	BarrierKind  string `json:"barrier_kind"`
	BarrierState string `json:"barrier_state"`
	DistanceM    int    `json:"distance_m"`
	VisualRangeM int    `json:"visual_range_m"`
	AudioRangeM  int    `json:"audio_range_m"`
}

type RPPerceptionLinkRecord = privateFactRecord[RPPerceptionLinkFact]

func (s *Store) DefineRPPerceptionLink(ctx context.Context, r RPPerceptionLinkRequest) (RPPerceptionLinkRecord, error) {
	if err := r.Binding.Validate(); err != nil {
		return RPPerceptionLinkRecord{}, err
	}
	if strings.TrimSpace(r.PlaceID) != r.PlaceID || r.PlaceID == "" || len(r.PlaceID) > 256 || !rpZoneKey.MatchString(r.ZoneA) || !rpZoneKey.MatchString(r.ZoneB) || r.ZoneA >= r.ZoneB ||
		(r.BarrierKind != "open" && r.BarrierKind != "door" && r.BarrierKind != "wall") || (r.BarrierState != "open" && r.BarrierState != "closed") ||
		r.DistanceM < 0 || r.DistanceM > 1000 || r.VisualRangeM < 0 || r.VisualRangeM > 1000 || r.AudioRangeM < 0 || r.AudioRangeM > 1000 {
		return RPPerceptionLinkRecord{}, core.NewError(core.CodeInvalidArgument, "invalid scoped perception link")
	}
	hash, err := core.HashJSON([]string{r.Binding.InstanceID, r.Binding.BranchID, r.PlaceID, r.ZoneA, r.ZoneB})
	if err != nil {
		return RPPerceptionLinkRecord{}, err
	}
	linkID := "rp_perception_" + hash[7:]
	return executePrivateFactCommand(s, ctx, r.Binding, "DefineRPPerceptionLink", r,
		privateFactDomain{"rp_perception", "RPPerceptionLinkDefined", `{"authorization":"scoped-world-creator"}`},
		func(conn *sql.Conn) error { return authorizeRPLocationBuilder(ctx, conn, r.Binding) },
		func(conn *sql.Conn, c privateFactContext) (RPPerceptionLinkFact, func() error, error) {
			var active int
			if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM agent_places WHERE place_id=? AND instance_id=? AND branch_id=? AND status='active'`, r.PlaceID, r.Binding.InstanceID, r.Binding.BranchID).Scan(&active); err != nil {
				return RPPerceptionLinkFact{}, nil, err
			}
			if active != 1 {
				return RPPerceptionLinkFact{}, nil, core.NewError(core.CodeNotFound, "active perception place not found")
			}
			var existing int
			if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_perception_links WHERE instance_id=? AND branch_id=? AND place_id=? AND zone_a=? AND zone_b=?`, r.Binding.InstanceID, r.Binding.BranchID, r.PlaceID, r.ZoneA, r.ZoneB).Scan(&existing); err != nil {
				return RPPerceptionLinkFact{}, nil, err
			}
			if existing != 0 {
				return RPPerceptionLinkFact{}, nil, core.NewError(core.CodeBranchConflict, "perception link already declared")
			}
			fact := RPPerceptionLinkFact{"corerp.spatial.perception-link.v1", linkID, r.PlaceID, r.ZoneA, r.ZoneB, r.BarrierKind, r.BarrierState, r.DistanceM, r.VisualRangeM, r.AudioRangeM}
			return fact, func() error {
				return execAgentOne(ctx, conn, "define perception link", `INSERT INTO rp_perception_links(link_id,instance_id,branch_id,place_id,zone_a,zone_b,barrier_kind,barrier_state,distance_m,visual_range_m,audio_range_m,definition_event_id) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`, linkID, r.Binding.InstanceID, r.Binding.BranchID, r.PlaceID, r.ZoneA, r.ZoneB, r.BarrierKind, r.BarrierState, r.DistanceM, r.VisualRangeM, r.AudioRangeM, c.EventID)
			}, nil
		})
}

type RPActorZoneRequest struct {
	Binding core.CareerBinding `json:"binding"`
	AgentID string             `json:"agent_id"`
	PlaceID string             `json:"place_id"`
	ZoneKey string             `json:"zone_key"`
}

type RPActorZoneFact struct {
	Version      string `json:"version"`
	AgentID      string `json:"agent_id"`
	PlaceID      string `json:"place_id"`
	EntryEventID string `json:"entry_event_id"`
	ZoneKey      string `json:"zone_key"`
}

type RPActorZoneRecord = privateFactRecord[RPActorZoneFact]

func (s *Store) PlaceRPActorInZone(ctx context.Context, r RPActorZoneRequest) (RPActorZoneRecord, error) {
	if err := r.Binding.Validate(); err != nil {
		return RPActorZoneRecord{}, err
	}
	if strings.TrimSpace(r.AgentID) != r.AgentID || r.AgentID == "" || len(r.AgentID) > 256 || strings.TrimSpace(r.PlaceID) != r.PlaceID || r.PlaceID == "" || len(r.PlaceID) > 256 || !rpZoneKey.MatchString(r.ZoneKey) {
		return RPActorZoneRecord{}, core.NewError(core.CodeInvalidArgument, "bounded actor, place and zone required")
	}
	return executePrivateFactCommand(s, ctx, r.Binding, "PlaceRPActorInZone", r,
		privateFactDomain{"rp_actor_zone", "RPActorZoneChosen", `{"authorization":"scoped-world-creator"}`},
		func(conn *sql.Conn) error { return authorizeRPLocationBuilder(ctx, conn, r.Binding) },
		func(conn *sql.Conn, c privateFactContext) (RPActorZoneFact, func() error, error) {
			var currentPlace, entryEventID string
			err := conn.QueryRowContext(ctx, `SELECT p.place_id,m.event_id FROM agent_profiles a JOIN agent_positions p ON p.agent_id=a.agent_id
				JOIN agent_movements m ON m.agent_id=a.agent_id AND m.to_place_id=p.place_id JOIN events e ON e.event_id=m.event_id
				WHERE a.agent_id=? AND a.instance_id=? AND a.branch_id=? AND a.status='active'
				AND e.instance_id=a.instance_id AND e.branch_id=a.branch_id ORDER BY e.event_sequence DESC LIMIT 1`, r.AgentID, r.Binding.InstanceID, r.Binding.BranchID).Scan(&currentPlace, &entryEventID)
			if err != nil {
				return RPActorZoneFact{}, nil, classifyMissing(err, "active actor spatial entry")
			}
			if currentPlace != r.PlaceID {
				return RPActorZoneFact{}, nil, core.NewError(core.CodeBranchConflict, "actor is not at requested zone's place")
			}
			if r.ZoneKey != "main" {
				var declared int
				if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_perception_links WHERE instance_id=? AND branch_id=? AND place_id=? AND (zone_a=? OR zone_b=?)`, r.Binding.InstanceID, r.Binding.BranchID, r.PlaceID, r.ZoneKey, r.ZoneKey).Scan(&declared); err != nil {
					return RPActorZoneFact{}, nil, err
				}
				if declared == 0 {
					return RPActorZoneFact{}, nil, core.NewError(core.CodeInvalidArgument, "zone was not declared in this place")
				}
			}
			fact := RPActorZoneFact{"corerp.spatial.actor-zone.v1", r.AgentID, r.PlaceID, entryEventID, r.ZoneKey}
			return fact, func() error {
				return execAgentOne(ctx, conn, "place actor in zone", `INSERT INTO rp_actor_zones(agent_id,instance_id,branch_id,place_id,entry_event_id,zone_key,source_event_id) VALUES (?,?,?,?,?,?,?) ON CONFLICT(agent_id) DO UPDATE SET instance_id=excluded.instance_id,branch_id=excluded.branch_id,place_id=excluded.place_id,entry_event_id=excluded.entry_event_id,zone_key=excluded.zone_key,source_event_id=excluded.source_event_id WHERE rp_actor_zones.instance_id=excluded.instance_id AND rp_actor_zones.branch_id=excluded.branch_id`, r.AgentID, r.Binding.InstanceID, r.Binding.BranchID, r.PlaceID, entryEventID, r.ZoneKey, c.EventID)
			}, nil
		})
}
