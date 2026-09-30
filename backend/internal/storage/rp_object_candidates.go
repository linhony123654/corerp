package storage

import (
	"context"
	"database/sql"

	"corerp.local/backend/internal/core"
)

// Narrow proposal/observation surface. IDs are scoped to this controlled actor
// and place; no other actor's inventory or unoffered object is exposed.
type RPObjectCandidate struct {
	Kind           string   `json:"kind"`
	ID             string   `json:"id"`
	DisplayName    string   `json:"display_name"`
	ObjectID       string   `json:"object_id,omitempty"`
	AnchorID       string   `json:"anchor_id,omitempty"`
	PhysicalState  string   `json:"physical_state,omitempty"`
	TargetEntityID string   `json:"target_entity_id,omitempty"`
	OfferStatus    string   `json:"offer_status,omitempty"`
	AllowedActions []string `json:"allowed_actions"`
}

// readRPObjectCandidates is a read-only hint for the interpreter and observed
// UI. ObjectRP always rechecks the entire reach/stock/consent state in its own
// BEGIN IMMEDIATE transaction; a candidate is never an execution capability.
func readRPObjectCandidates(ctx context.Context, conn *sql.Conn, session RPSession) ([]RPObjectCandidate, error) {
	point, err := readRPActorSpatialPoint(ctx, conn, session.InstanceID, session.BranchID, session.ControlledEntityID)
	if err != nil {
		return nil, err
	}
	result := make([]RPObjectCandidate, 0)
	rows, err := conn.QueryContext(ctx, `SELECT a.anchor_id,a.display_name,COALESCE(a.near_entity_id,'') FROM rp_object_anchors a JOIN events e ON e.event_id=a.definition_event_id AND e.instance_id=a.instance_id AND e.branch_id=a.branch_id AND e.event_type='RPObjectAnchorDefined' AND json_extract(e.payload,'$.anchor_id')=a.anchor_id AND json_extract(e.payload,'$.display_name')=a.display_name AND json_extract(e.payload,'$.place_id')=a.place_id AND json_extract(e.payload,'$.zone_key')=a.zone_key WHERE a.instance_id=? AND a.branch_id=? AND a.place_id=? AND a.zone_key=? ORDER BY a.anchor_id LIMIT 24`, session.InstanceID, session.BranchID, point.PlaceID, point.ZoneKey)
	if err != nil {
		return nil, err
	}
	type anchor struct{ id, name, near string }
	var anchors []anchor
	for rows.Next() {
		var a anchor
		if err := rows.Scan(&a.id, &a.name, &a.near); err != nil {
			rows.Close()
			return nil, err
		}
		anchors = append(anchors, a)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	for _, a := range anchors {
		if a.near != "" {
			other, err := readRPActorSpatialPoint(ctx, conn, session.InstanceID, session.BranchID, a.near)
			if err != nil || other != point {
				continue
			}
			visible, err := rpCanPerceive(ctx, conn, session.InstanceID, session.BranchID, session.ControlledEntityID, a.near, "visual", "")
			if err != nil {
				return nil, err
			}
			if !visible {
				continue
			}
			known, err := rpIdentityKnown(ctx, conn, session.InstanceID, session.BranchID, session.ControlledEntityID, a.near)
			if err != nil {
				return nil, err
			}
			if !known {
				a.name = "近旁位置"
			}
		}
		result = append(result, RPObjectCandidate{Kind: "anchor", ID: a.id, AnchorID: a.id, DisplayName: a.name, AllowedActions: []string{"place", "move"}})
	}
	rows, err = conn.QueryContext(ctx, `SELECT s.source_id,s.display_name,s.sku_id,s.unit_minor,s.inventory_location_id FROM rp_object_sources s JOIN rp_object_anchors a ON a.anchor_id=s.anchor_id AND a.instance_id=s.instance_id AND a.branch_id=s.branch_id JOIN materialized_entities m ON m.entity_id=s.owner_actor_id AND m.inventory_location_id=s.inventory_location_id AND m.status='active' AND m.population_count=1 JOIN stock_locations l ON l.location_id=s.inventory_location_id AND l.owner_id=s.owner_actor_id AND l.location_kind='holder' JOIN events e ON e.event_id=s.definition_event_id AND e.instance_id=s.instance_id AND e.branch_id=s.branch_id AND e.event_type='RPObjectSourceDefined' AND json_extract(e.payload,'$.source_id')=s.source_id AND json_extract(e.payload,'$.owner_entity_id')=s.owner_actor_id AND json_extract(e.payload,'$.sku_id')=s.sku_id AND json_extract(e.payload,'$.display_name')=s.display_name WHERE s.instance_id=? AND s.branch_id=? AND s.owner_actor_id=? AND a.place_id=? AND a.zone_key=? ORDER BY s.source_id LIMIT 24`, session.InstanceID, session.BranchID, session.ControlledEntityID, point.PlaceID, point.ZoneKey)
	if err != nil {
		return nil, err
	}
	type source struct {
		id, name, sku, location string
		unit                    int64
	}
	var sources []source
	for rows.Next() {
		var item source
		if err := rows.Scan(&item.id, &item.name, &item.sku, &item.unit, &item.location); err != nil {
			rows.Close()
			return nil, err
		}
		sources = append(sources, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	for _, item := range sources {
		available, _, err := readRPStoreStock(ctx, conn, session.InstanceID, session.BranchID, item.location, item.sku)
		if err != nil {
			return nil, err
		}
		if available >= item.unit {
			result = append(result, RPObjectCandidate{Kind: "source", ID: item.id, DisplayName: item.name, AllowedActions: []string{"stage"}})
		}
	}
	rows, err = conn.QueryContext(ctx, `SELECT DISTINCT item.object_id FROM rp_objects item LEFT JOIN rp_object_offers o ON o.object_id=item.object_id AND o.status IN ('offered','accepted_pending_transfer') WHERE item.instance_id=? AND item.branch_id=? AND item.physical_state IN ('held','placed') AND (item.owner_actor_id=? OR o.target_actor_id=?) ORDER BY item.object_id LIMIT 32`, session.InstanceID, session.BranchID, session.ControlledEntityID, session.ControlledEntityID)
	if err != nil {
		return nil, err
	}
	var objectIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		objectIDs = append(objectIDs, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	for _, id := range objectIDs {
		item, err := loadRPObject(ctx, conn, session, id)
		if err != nil {
			return nil, err
		}
		local := item.Place == point.PlaceID && item.Zone == point.ZoneKey
		if item.State == "held" {
			holderPoint, err := readRPActorSpatialPoint(ctx, conn, session.InstanceID, session.BranchID, item.Holder)
			if err != nil {
				return nil, err
			}
			local = holderPoint == point
		}
		var activeOfferID string
		if err := conn.QueryRowContext(ctx, `SELECT offer_id FROM rp_object_offers WHERE object_id=? AND status IN ('offered','accepted_pending_transfer')`, id).Scan(&activeOfferID); err != nil && err != sql.ErrNoRows {
			return nil, err
		}
		if item.Owner == session.ControlledEntityID && local {
			allowed := []string{}
			if activeOfferID == "" {
				if item.State == "held" {
					allowed = []string{"place", "offer", "stow"}
				} else {
					allowed = []string{"take", "move"}
				}
			}
			result = append(result, RPObjectCandidate{Kind: "object", ID: item.ID, ObjectID: item.ID, AnchorID: item.Anchor, PhysicalState: item.State, DisplayName: item.Name, AllowedActions: allowed})
		}
		if activeOfferID == "" {
			continue
		}
		offer, err := loadRPObjectOffer(ctx, conn, session, activeOfferID)
		if err != nil {
			return nil, err
		}
		if !local || (offer.From != session.ControlledEntityID && offer.Target != session.ControlledEntityID) {
			continue
		}
		var allowed []string
		if offer.Target == session.ControlledEntityID {
			switch offer.Status {
			case "offered":
				allowed = []string{"refuse"}
				if local {
					allowed = append(allowed, "accept")
				}
			case "accepted_pending_transfer":
				if local {
					allowed = []string{"receive"}
				}
			}
		} else if offer.Status == "accepted_pending_transfer" && local {
			allowed = []string{"cancel_offer", "give"}
		} else {
			allowed = []string{"cancel_offer"}
		}
		publicTarget, err := rpPublicEntityID(ctx, conn, session.InstanceID, session.BranchID, session.ControlledEntityID, offer.Target)
		if err != nil {
			return nil, err
		}
		result = append(result, RPObjectCandidate{Kind: "offer", ID: offer.ID, ObjectID: item.ID, TargetEntityID: publicTarget, DisplayName: item.Name, OfferStatus: offer.Status, AllowedActions: allowed})
	}
	others, err := rpCoLocatedEntityIDs(ctx, conn, session.InstanceID, session.BranchID, point.PlaceID, session.ControlledEntityID)
	if err != nil {
		return nil, err
	}
	for _, id := range others {
		other, err := readRPActorSpatialPoint(ctx, conn, session.InstanceID, session.BranchID, id)
		if err != nil || other != point {
			continue
		}
		visible, err := rpCanPerceive(ctx, conn, session.InstanceID, session.BranchID, session.ControlledEntityID, id, "visual", "")
		if err != nil {
			return nil, err
		}
		if !visible {
			continue
		}
		public, err := rpPublicEntityID(ctx, conn, session.InstanceID, session.BranchID, session.ControlledEntityID, id)
		if err != nil {
			return nil, err
		}
		result = append(result, RPObjectCandidate{Kind: "target", ID: public, TargetEntityID: public, DisplayName: public, AllowedActions: []string{"offer"}})
	}
	return result, nil
}

// ReadRPObjectCandidates authorizes the RP controller for user-facing reads.
// The internal helper above accepts only a session loaded by an authorized caller.
func (s *Store) ReadRPObjectCandidates(ctx context.Context, r core.RPSessionReadRequest) ([]RPObjectCandidate, error) {
	if err := r.Validate(); err != nil {
		return nil, err
	}
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	session, err := loadRPSessionRecord(ctx, conn, r.PrincipalID, r.SessionID)
	if err != nil {
		return nil, err
	}
	if session.Status != "active" {
		return nil, core.NewError(core.CodeBranchConflict, "object candidates require active session")
	}
	if err := requireCurrentRPSession(ctx, conn, session); err != nil {
		return nil, err
	}
	if err := authorizeRPControl(ctx, conn, r.PrincipalID, session.InstanceID, session.BranchID, session.ControlledEntityID); err != nil {
		return nil, err
	}
	return readRPObjectCandidates(ctx, conn, session)
}
