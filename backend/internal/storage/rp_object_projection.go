package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"

	"corerp.local/backend/internal/core"
)

type rpObjectAnchorProjection struct {
	ID, Instance, Branch, Place, Zone, Code, Name, Near, Event string
}
type rpObjectSourceProjection struct {
	ID, Instance, Branch, Anchor, Owner, Location, SKU, Name, Event string
	Unit                                                            int64
}
type rpObjectItemProjection struct {
	ID, Instance, Branch, Source, SKU, Name, Owner, Escrow, State, Holder, Anchor, Place, Zone, StageEvent, LastEvent string
	Unit, Sequence                                                                                                    int64
}
type rpObjectOfferProjection struct {
	ID, Instance, Branch, Object, From, Target, Status, OfferEvent, ResponseEvent, TerminalEvent string
	Sequence                                                                                     int64
}
type rpObjectEscrowProjection struct {
	Object, Owner, Location string
}
type rpObjectStockLocationProjection struct {
	ID, Owner, Kind string
}

type rpObjectExpectedState struct {
	anchors   map[string]rpObjectAnchorProjection
	sources   map[string]rpObjectSourceProjection
	items     map[string]rpObjectItemProjection
	offers    map[string]rpObjectOfferProjection
	escrows   map[string]rpObjectEscrowProjection
	locations map[string]rpObjectStockLocationProjection
}

func rpObjectExpected(ctx context.Context, q replayQuerier, instance, branch string, through int64) (rpObjectExpectedState, error) {
	state := rpObjectExpectedState{map[string]rpObjectAnchorProjection{}, map[string]rpObjectSourceProjection{}, map[string]rpObjectItemProjection{}, map[string]rpObjectOfferProjection{}, map[string]rpObjectEscrowProjection{}, map[string]rpObjectStockLocationProjection{}}
	rows, err := q.QueryContext(ctx, `SELECT event_id,event_sequence,event_type,actor_id,payload FROM events WHERE instance_id=? AND branch_id=? AND event_sequence<=? AND event_type IN ('RPObjectAnchorDefined','RPObjectSourceDefined','RPObjectInteracted') ORDER BY event_sequence`, instance, branch, through)
	if err != nil {
		return state, err
	}
	type event struct {
		id, kind, actor, raw string
		sequence             int64
	}
	var events []event
	for rows.Next() {
		var e event
		if err := rows.Scan(&e.id, &e.sequence, &e.kind, &e.actor, &e.raw); err != nil {
			rows.Close()
			return state, err
		}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return state, err
	}
	rows.Close()
	for _, e := range events {
		id, kind, actor, raw, sequence := e.id, e.kind, e.actor, e.raw, e.sequence
		switch kind {
		case "RPObjectAnchorDefined":
			var f RPObjectAnchorFact
			if err := json.Unmarshal([]byte(raw), &f); err != nil {
				rows.Close()
				return state, err
			}
			if f.Version != "corerp.object.anchor.v1" || f.AnchorID == "" || !rpZoneKey.MatchString(f.ZoneKey) || !validRPObjectName(f.DisplayName) {
				rows.Close()
				return state, core.NewError(core.CodeProjectionDiverged, "invalid authored object anchor Event")
			}
			if _, exists := state.anchors[f.AnchorID]; exists {
				rows.Close()
				return state, core.NewError(core.CodeProjectionDiverged, "duplicate object anchor Event")
			}
			state.anchors[f.AnchorID] = rpObjectAnchorProjection{f.AnchorID, instance, branch, f.PlaceID, f.ZoneKey, f.AnchorCode, f.DisplayName, f.NearEntityID, id}
		case "RPObjectSourceDefined":
			var f RPObjectSourceFact
			if err := json.Unmarshal([]byte(raw), &f); err != nil {
				rows.Close()
				return state, err
			}
			anchor, present := state.anchors[f.AnchorID]
			if f.Version != "corerp.object.source.v1" || f.SourceID == "" || !present || anchor.Event != f.AnchorEventID || f.OwnerEntityID == "" || f.InventoryLocationID == "" || f.SKUID == "" || f.SKUEventID == "" || f.StockEventID == "" || f.UnitMinor < 1 || !validRPObjectName(f.DisplayName) {
				rows.Close()
				return state, core.NewError(core.CodeProjectionDiverged, "invalid authored object stock source Event")
			}
			if _, exists := state.sources[f.SourceID]; exists {
				rows.Close()
				return state, core.NewError(core.CodeProjectionDiverged, "duplicate object stock source Event")
			}
			state.sources[f.SourceID] = rpObjectSourceProjection{f.SourceID, instance, branch, f.AnchorID, f.OwnerEntityID, f.InventoryLocationID, f.SKUID, f.DisplayName, id, f.UnitMinor}
		case "RPObjectInteracted":
			var f core.RPObjectEvidence
			if err := json.Unmarshal([]byte(raw), &f); err != nil {
				rows.Close()
				return state, err
			}
			if f.Version != "corerp.object.interaction.v1" || f.ClaimType != "object_interaction" || f.ActorEntityID != actor || f.ObjectID == "" || f.SKUID == "" || f.UnitMinor < 1 || !validRPObjectName(f.DisplayName) || f.PlaceID == "" || !rpZoneKey.MatchString(f.ZoneKey) {
				rows.Close()
				return state, core.NewError(core.CodeProjectionDiverged, "invalid physical object Event")
			}
			if f.Action == "stage" {
				source, present := state.sources[f.SourceID]
				anchor := state.anchors[source.Anchor]
				if !present || f.SourceEventID != source.Event || f.OwnerEntityID != actor || f.HolderEntityID != actor || f.AnchorID != "" || f.PreviousEvent != "" || f.SKUID != source.SKU || f.UnitMinor != source.Unit || f.DisplayName != source.Name || f.PlaceID != anchor.Place || f.ZoneKey != anchor.Zone || f.FromLocationID != source.Location || f.ToLocationID != rpObjectEscrowID(f.ObjectID, actor) {
					rows.Close()
					return state, core.NewError(core.CodeProjectionDiverged, "staged object lacks declared source or dedicated escrow")
				}
				if _, exists := state.items[f.ObjectID]; exists {
					rows.Close()
					return state, core.NewError(core.CodeProjectionDiverged, "duplicate staged object Event")
				}
				state.items[f.ObjectID] = rpObjectItemProjection{f.ObjectID, instance, branch, f.SourceID, f.SKUID, f.DisplayName, actor, f.ToLocationID, "held", actor, "", f.PlaceID, f.ZoneKey, id, id, f.UnitMinor, sequence}
				if err := rpObjectExpectedEscrow(&state, f.ObjectID, actor, f.ToLocationID); err != nil {
					rows.Close()
					return state, err
				}
				if err := rpObjectExpectedMovement(ctx, q, id, f); err != nil {
					rows.Close()
					return state, err
				}
				continue
			}
			item, present := state.items[f.ObjectID]
			if !present || item.LastEvent != f.PreviousEvent || item.Source != f.SourceID || item.SKU != f.SKUID || item.Name != f.DisplayName || item.Unit != f.UnitMinor {
				rows.Close()
				return state, core.NewError(core.CodeProjectionDiverged, "physical object transition has no matching predecessor")
			}
			if item.State == "stowed" {
				rows.Close()
				return state, core.NewError(core.CodeProjectionDiverged, "stowed object cannot return to the scene")
			}
			previous := item
			if f.Action == "stow" {
				if f.AnchorID != "" || f.HolderEntityID != "" {
					rows.Close()
					return state, core.NewError(core.CodeProjectionDiverged, "stowed object retains a physical position")
				}
				item.State, item.Anchor, item.Holder = "stowed", "", ""
			} else if f.AnchorID != "" {
				anchor, present := state.anchors[f.AnchorID]
				if !present || anchor.Place != f.PlaceID || anchor.Zone != f.ZoneKey || f.HolderEntityID != "" || f.OwnerEntityID != item.Owner {
					rows.Close()
					return state, core.NewError(core.CodeProjectionDiverged, "placed item lacks authored reachable anchor")
				}
				item.State, item.Anchor, item.Holder = "placed", f.AnchorID, ""
			} else {
				if f.HolderEntityID != f.OwnerEntityID || f.HolderEntityID == "" {
					rows.Close()
					return state, core.NewError(core.CodeProjectionDiverged, "held item has no current owner-holder")
				}
				item.State, item.Anchor, item.Holder = "held", "", f.HolderEntityID
			}
			if err := rpObjectExpectedOffer(&state, f, id, sequence, previous, &item); err != nil {
				rows.Close()
				return state, err
			}
			if f.Action == "give" || f.Action == "receive" || f.Action == "stow" {
				if f.Action == "stow" {
					if err := rpObjectExpectedStowDestination(ctx, q, instance, branch, f); err != nil {
						rows.Close()
						return state, err
					}
				}
				if err := rpObjectExpectedMovement(ctx, q, id, f); err != nil {
					rows.Close()
					return state, err
				}
			} else {
				var unexpected int
				if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM stock_movements WHERE event_id=?`, id).Scan(&unexpected); err != nil {
					rows.Close()
					return state, err
				}
				if unexpected != 0 {
					rows.Close()
					return state, core.NewError(core.CodeProjectionDiverged, "non-transfer object Event moved stock")
				}
			}
			item.Owner, item.Place, item.Zone, item.LastEvent, item.Sequence = f.OwnerEntityID, f.PlaceID, f.ZoneKey, id, sequence
			state.items[f.ObjectID] = item
		}
	}
	return state, nil
}

func rpObjectExpectedEscrow(state *rpObjectExpectedState, object, owner, location string) error {
	if location != rpObjectEscrowID(object, owner) {
		return core.NewError(core.CodeProjectionDiverged, "object escrow ID does not belong to owner")
	}
	key := object + ":" + owner
	if old, exists := state.escrows[key]; exists && old.Location != location {
		return core.NewError(core.CodeProjectionDiverged, "object has contradictory escrow locations")
	}
	state.escrows[key] = rpObjectEscrowProjection{object, owner, location}
	state.locations[location] = rpObjectStockLocationProjection{location, owner, "holder"}
	return nil
}

func rpObjectExpectedOffer(state *rpObjectExpectedState, f core.RPObjectEvidence, id string, sequence int64, previous rpObjectItemProjection, item *rpObjectItemProjection) error {
	var active bool
	var activeOffer rpObjectOfferProjection
	for _, o := range state.offers {
		if o.Object == f.ObjectID && (o.Status == "offered" || o.Status == "accepted_pending_transfer") {
			active, activeOffer = true, o
		}
	}
	switch f.Action {
	case "stow":
		if active || previous.State != "held" || previous.Owner != f.ActorEntityID || previous.Holder != f.ActorEntityID || item.State != "stowed" || f.OwnerEntityID != previous.Owner || f.AnchorID != "" || f.HolderEntityID != "" || f.FromLocationID != previous.Escrow || f.ToLocationID == "" || f.ToLocationID == previous.Escrow || f.OfferID != "" || f.TargetEntityID != "" {
			return core.NewError(core.CodeProjectionDiverged, "stow did not return the held owner's physical unit")
		}
	case "take":
		if active || previous.State != "placed" || item.State != "held" || previous.Owner != f.ActorEntityID || f.AnchorID != "" || f.OwnerEntityID != previous.Owner {
			return core.NewError(core.CodeProjectionDiverged, "invalid pickup transition")
		}
	case "place":
		if active || previous.State != "held" || item.State != "placed" || previous.Owner != f.ActorEntityID || f.AnchorID == "" {
			return core.NewError(core.CodeProjectionDiverged, "invalid placement transition")
		}
	case "move":
		if active || previous.State != "placed" || item.State != "placed" || previous.Owner != f.ActorEntityID || f.AnchorID == "" || previous.Anchor == f.AnchorID {
			return core.NewError(core.CodeProjectionDiverged, "invalid placed-item move")
		}
	case "offer":
		if active || item.State != "held" || item.Owner != f.ActorEntityID || f.OwnerEntityID != item.Owner || f.TargetEntityID == "" || f.TargetEntityID == item.Owner || f.OfferID == "" || f.OfferEventID != id || f.OfferStatus != "offered" {
			return core.NewError(core.CodeProjectionDiverged, "offer falsely became ownership")
		}
		if _, exists := state.offers[f.OfferID]; exists {
			return core.NewError(core.CodeProjectionDiverged, "duplicate offer ID")
		}
		state.offers[f.OfferID] = rpObjectOfferProjection{f.OfferID, item.Instance, item.Branch, item.ID, item.Owner, f.TargetEntityID, "offered", id, "", "", sequence}
	case "accept", "refuse", "cancel_offer", "give", "receive":
		offer, exists := state.offers[f.OfferID]
		if !exists || !active || activeOffer.ID != offer.ID || offer.Object != item.ID || f.OfferEventID != offer.OfferEvent || f.TargetEntityID != offer.Target || item.Owner != offer.From || item.State != "held" {
			return core.NewError(core.CodeProjectionDiverged, "offer response has no active original")
		}
		switch f.Action {
		case "accept", "refuse":
			if f.ActorEntityID != offer.Target || offer.Status != "offered" || f.OwnerEntityID != offer.From || f.HolderEntityID != offer.From {
				return core.NewError(core.CodeProjectionDiverged, "unauthorized offer response")
			}
			offer.ResponseEvent = id
			if f.Action == "accept" {
				offer.Status = "accepted_pending_transfer"
			} else {
				offer.Status, offer.TerminalEvent = "refused", id
			}
		case "cancel_offer":
			if f.ActorEntityID != offer.From || f.OwnerEntityID != offer.From || f.HolderEntityID != offer.From {
				return core.NewError(core.CodeProjectionDiverged, "only original owner may withdraw offer")
			}
			offer.Status, offer.TerminalEvent = "cancelled", id
		case "give", "receive":
			if offer.Status != "accepted_pending_transfer" || offer.ResponseEvent == "" || (f.Action == "give" && f.ActorEntityID != offer.From) || (f.Action == "receive" && f.ActorEntityID != offer.Target) || f.OwnerEntityID != offer.Target || f.HolderEntityID != offer.Target || f.FromLocationID != item.Escrow || f.ToLocationID != rpObjectEscrowID(item.ID, offer.Target) {
				return core.NewError(core.CodeProjectionDiverged, "physical transfer has no separate consent or stock")
			}
			if err := rpObjectExpectedEscrow(state, item.ID, offer.Target, f.ToLocationID); err != nil {
				return err
			}
			item.Escrow = f.ToLocationID
			offer.Status, offer.TerminalEvent = "transferred", id
		}
		if f.OfferStatus != offer.Status {
			return core.NewError(core.CodeProjectionDiverged, "offer state disagrees with response Event")
		}
		offer.Sequence = sequence
		state.offers[f.OfferID] = offer
	default:
		return core.NewError(core.CodeProjectionDiverged, "unknown object interaction Event")
	}
	return nil
}

func rpObjectExpectedStowDestination(ctx context.Context, q replayQuerier, instance, branch string, f core.RPObjectEvidence) error {
	var owners int
	if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM materialized_entities m JOIN cohorts c ON c.cohort_id=m.source_cohort_id JOIN stock_locations l ON l.location_id=m.inventory_location_id AND l.owner_id=m.entity_id AND l.location_kind='holder' WHERE m.entity_id=? AND m.inventory_location_id=? AND m.population_count=1 AND c.instance_id=? AND c.branch_id=?`, f.ActorEntityID, f.ToLocationID, instance, branch).Scan(&owners); err != nil {
		return err
	}
	if owners != 1 {
		return core.NewError(core.CodeProjectionDiverged, "stowed item was not credited to its owner's ordinary inventory")
	}
	return nil
}

func rpObjectExpectedMovement(ctx context.Context, q replayQuerier, eventID string, f core.RPObjectEvidence) error {
	var exact, total int
	err := q.QueryRowContext(ctx, `SELECT COUNT(*),COALESCE(SUM(CASE WHEN sku_id=? AND quantity_minor=? AND from_location_id=? AND to_location_id=? AND movement_kind='transfer' AND reason_code=? THEN 1 ELSE 0 END),0) FROM stock_movements WHERE event_id=?`, f.SKUID, f.UnitMinor, f.FromLocationID, f.ToLocationID, "rp_object_"+f.Action, eventID).Scan(&total, &exact)
	if err != nil {
		return err
	}
	if total != 1 || exact != 1 {
		return core.NewError(core.CodeProjectionDiverged, "object transfer has no unique matching finite stock movement")
	}
	return nil
}

func rpObjectProjectionDifferences(ctx context.Context, q replayQuerier, instance, branch string, through int64) ([]ProjectionDifference, error) {
	expected, err := rpObjectExpected(ctx, q, instance, branch, through)
	if err != nil {
		return nil, err
	}
	var differences []ProjectionDifference
	compare := func(kind, key string, want, got any, readErr error) error {
		if readErr != nil && !errors.Is(readErr, sql.ErrNoRows) {
			return readErr
		}
		expectedJSON, err := core.CanonicalJSON(want)
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
		if actual != string(expectedJSON) {
			differences = append(differences, ProjectionDifference{Projection: kind, Key: key, ExpectedText: string(expectedJSON), ActualText: actual})
		}
		return nil
	}
	for _, key := range rpObjectSortedKeys(expected.anchors) {
		var got rpObjectAnchorProjection
		var near sql.NullString
		err := q.QueryRowContext(ctx, `SELECT anchor_id,instance_id,branch_id,place_id,zone_key,anchor_code,display_name,near_entity_id,definition_event_id FROM rp_object_anchors WHERE anchor_id=?`, key).Scan(&got.ID, &got.Instance, &got.Branch, &got.Place, &got.Zone, &got.Code, &got.Name, &near, &got.Event)
		got.Near = near.String
		if err := compare("rp_object_anchor", key, expected.anchors[key], got, err); err != nil {
			return nil, err
		}
	}
	for _, key := range rpObjectSortedKeys(expected.sources) {
		var got rpObjectSourceProjection
		err := q.QueryRowContext(ctx, `SELECT source_id,instance_id,branch_id,anchor_id,owner_actor_id,inventory_location_id,sku_id,display_name,definition_event_id,unit_minor FROM rp_object_sources WHERE source_id=?`, key).Scan(&got.ID, &got.Instance, &got.Branch, &got.Anchor, &got.Owner, &got.Location, &got.SKU, &got.Name, &got.Event, &got.Unit)
		if err := compare("rp_object_source", key, expected.sources[key], got, err); err != nil {
			return nil, err
		}
	}
	for _, key := range rpObjectSortedKeys(expected.locations) {
		var got rpObjectStockLocationProjection
		err := q.QueryRowContext(ctx, `SELECT location_id,owner_id,location_kind FROM stock_locations WHERE location_id=?`, key).Scan(&got.ID, &got.Owner, &got.Kind)
		if err := compare("rp_object_escrow_location", key, expected.locations[key], got, err); err != nil {
			return nil, err
		}
	}
	for _, key := range rpObjectSortedKeys(expected.items) {
		var got rpObjectItemProjection
		var holder, anchor sql.NullString
		err := q.QueryRowContext(ctx, `SELECT object_id,instance_id,branch_id,source_id,sku_id,display_name,owner_actor_id,escrow_location_id,physical_state,holder_actor_id,anchor_id,place_id,zone_key,stage_event_id,last_event_id,unit_minor,last_event_sequence FROM rp_objects WHERE object_id=?`, key).Scan(&got.ID, &got.Instance, &got.Branch, &got.Source, &got.SKU, &got.Name, &got.Owner, &got.Escrow, &got.State, &holder, &anchor, &got.Place, &got.Zone, &got.StageEvent, &got.LastEvent, &got.Unit, &got.Sequence)
		got.Holder, got.Anchor = holder.String, anchor.String
		if err := compare("rp_object", key, expected.items[key], got, err); err != nil {
			return nil, err
		}
	}
	for _, key := range rpObjectSortedKeys(expected.escrows) {
		var got rpObjectEscrowProjection
		want := expected.escrows[key]
		err := q.QueryRowContext(ctx, `SELECT object_id,owner_actor_id,location_id FROM rp_object_escrows WHERE object_id=? AND owner_actor_id=?`, want.Object, want.Owner).Scan(&got.Object, &got.Owner, &got.Location)
		if err := compare("rp_object_escrow", key, want, got, err); err != nil {
			return nil, err
		}
	}
	for _, key := range rpObjectSortedKeys(expected.offers) {
		var got rpObjectOfferProjection
		var response, terminal sql.NullString
		err := q.QueryRowContext(ctx, `SELECT offer_id,instance_id,branch_id,object_id,from_actor_id,target_actor_id,status,offer_event_id,response_event_id,terminal_event_id,last_event_sequence FROM rp_object_offers WHERE offer_id=?`, key).Scan(&got.ID, &got.Instance, &got.Branch, &got.Object, &got.From, &got.Target, &got.Status, &got.OfferEvent, &response, &terminal, &got.Sequence)
		got.ResponseEvent, got.TerminalEvent = response.String, terminal.String
		if err := compare("rp_object_offer", key, expected.offers[key], got, err); err != nil {
			return nil, err
		}
	}
	for _, scope := range []struct {
		query, kind string
		known       func(string) bool
	}{
		{`SELECT anchor_id FROM rp_object_anchors WHERE instance_id=? AND branch_id=?`, "rp_object_anchor_extra", func(id string) bool { _, ok := expected.anchors[id]; return ok }},
		{`SELECT source_id FROM rp_object_sources WHERE instance_id=? AND branch_id=?`, "rp_object_source_extra", func(id string) bool { _, ok := expected.sources[id]; return ok }},
		{`SELECT object_id FROM rp_objects WHERE instance_id=? AND branch_id=?`, "rp_object_extra", func(id string) bool { _, ok := expected.items[id]; return ok }},
		{`SELECT offer_id FROM rp_object_offers WHERE instance_id=? AND branch_id=?`, "rp_object_offer_extra", func(id string) bool { _, ok := expected.offers[id]; return ok }},
		{`SELECT location_id FROM rp_object_escrows WHERE object_id IN (SELECT object_id FROM rp_objects WHERE instance_id=? AND branch_id=?)`, "rp_object_escrow_extra", func(id string) bool { _, ok := expected.locations[id]; return ok }},
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
				differences = append(differences, ProjectionDifference{Projection: scope.kind, Key: id, ExpectedText: "absent", ActualText: "unsourced object projection"})
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

func rpObjectSortedKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// Restore the projection rows in FK order. Unexpected extra rows are rejected
// rather than deleted: an unexplained object must not silently disappear.
func repairRPObjectProjections(ctx context.Context, conn *sql.Conn, instance, branch string, differences []ProjectionDifference) error {
	for _, d := range differences {
		if len(d.Projection) >= len("rp_object") && d.Projection[:len("rp_object")] == "rp_object" && len(d.Projection) >= len("_extra") && d.Projection[len(d.Projection)-len("_extra"):] == "_extra" {
			return core.NewError(core.CodeProjectionDiverged, "cannot repair unsourced physical object projection")
		}
	}
	for _, kind := range []string{"rp_object_anchor", "rp_object_source", "rp_object_escrow_location", "rp_object", "rp_object_escrow", "rp_object_offer"} {
		for _, d := range differences {
			if d.Projection != kind {
				continue
			}
			switch kind {
			case "rp_object_anchor":
				var row rpObjectAnchorProjection
				if err := json.Unmarshal([]byte(d.ExpectedText), &row); err != nil || row.Instance != instance || row.Branch != branch || row.ID != d.Key {
					return core.NewError(core.CodeProjectionDiverged, "anchor repair fact or scope invalid")
				}
				if _, err := conn.ExecContext(ctx, `INSERT INTO rp_object_anchors(anchor_id,instance_id,branch_id,place_id,zone_key,anchor_code,display_name,near_entity_id,definition_event_id) VALUES (?,?,?,?,?,?,?,?,?) ON CONFLICT(anchor_id) DO UPDATE SET place_id=excluded.place_id,zone_key=excluded.zone_key,anchor_code=excluded.anchor_code,display_name=excluded.display_name,near_entity_id=excluded.near_entity_id,definition_event_id=excluded.definition_event_id WHERE rp_object_anchors.instance_id=excluded.instance_id AND rp_object_anchors.branch_id=excluded.branch_id`, row.ID, instance, branch, row.Place, row.Zone, row.Code, row.Name, nullRPObjectString(row.Near), row.Event); err != nil {
					return err
				}
			case "rp_object_source":
				var row rpObjectSourceProjection
				if err := json.Unmarshal([]byte(d.ExpectedText), &row); err != nil || row.Instance != instance || row.Branch != branch || row.ID != d.Key {
					return core.NewError(core.CodeProjectionDiverged, "source repair fact or scope invalid")
				}
				if _, err := conn.ExecContext(ctx, `INSERT INTO rp_object_sources(source_id,instance_id,branch_id,anchor_id,owner_actor_id,inventory_location_id,sku_id,unit_minor,display_name,definition_event_id) VALUES (?,?,?,?,?,?,?,?,?,?) ON CONFLICT(source_id) DO UPDATE SET anchor_id=excluded.anchor_id,owner_actor_id=excluded.owner_actor_id,inventory_location_id=excluded.inventory_location_id,sku_id=excluded.sku_id,unit_minor=excluded.unit_minor,display_name=excluded.display_name,definition_event_id=excluded.definition_event_id WHERE rp_object_sources.instance_id=excluded.instance_id AND rp_object_sources.branch_id=excluded.branch_id`, row.ID, instance, branch, row.Anchor, row.Owner, row.Location, row.SKU, row.Unit, row.Name, row.Event); err != nil {
					return err
				}
			case "rp_object_escrow_location":
				var row rpObjectStockLocationProjection
				if err := json.Unmarshal([]byte(d.ExpectedText), &row); err != nil || row.ID != d.Key || row.Kind != "holder" {
					return core.NewError(core.CodeProjectionDiverged, "escrow holder repair fact invalid")
				}
				if _, err := conn.ExecContext(ctx, `INSERT INTO stock_locations(location_id,owner_id,location_kind) VALUES (?,?,'holder') ON CONFLICT(location_id) DO UPDATE SET owner_id=excluded.owner_id,location_kind=excluded.location_kind,capability_id=NULL`, row.ID, row.Owner); err != nil {
					return err
				}
			case "rp_object":
				var row rpObjectItemProjection
				if err := json.Unmarshal([]byte(d.ExpectedText), &row); err != nil || row.Instance != instance || row.Branch != branch || row.ID != d.Key {
					return core.NewError(core.CodeProjectionDiverged, "object repair fact or scope invalid")
				}
				if _, err := conn.ExecContext(ctx, `INSERT INTO rp_objects(object_id,instance_id,branch_id,source_id,sku_id,unit_minor,display_name,owner_actor_id,escrow_location_id,physical_state,holder_actor_id,anchor_id,place_id,zone_key,stage_event_id,last_event_id,last_event_sequence) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(object_id) DO UPDATE SET source_id=excluded.source_id,sku_id=excluded.sku_id,unit_minor=excluded.unit_minor,display_name=excluded.display_name,owner_actor_id=excluded.owner_actor_id,escrow_location_id=excluded.escrow_location_id,physical_state=excluded.physical_state,holder_actor_id=excluded.holder_actor_id,anchor_id=excluded.anchor_id,place_id=excluded.place_id,zone_key=excluded.zone_key,stage_event_id=excluded.stage_event_id,last_event_id=excluded.last_event_id,last_event_sequence=excluded.last_event_sequence WHERE rp_objects.instance_id=excluded.instance_id AND rp_objects.branch_id=excluded.branch_id`, row.ID, instance, branch, row.Source, row.SKU, row.Unit, row.Name, row.Owner, row.Escrow, row.State, nullRPObjectString(row.Holder), nullRPObjectString(row.Anchor), row.Place, row.Zone, row.StageEvent, row.LastEvent, row.Sequence); err != nil {
					return err
				}
			case "rp_object_escrow":
				var row rpObjectEscrowProjection
				if err := json.Unmarshal([]byte(d.ExpectedText), &row); err != nil || row.Object+":"+row.Owner != d.Key {
					return core.NewError(core.CodeProjectionDiverged, "object escrow repair fact invalid")
				}
				if _, err := conn.ExecContext(ctx, `INSERT INTO rp_object_escrows(object_id,owner_actor_id,location_id) VALUES (?,?,?) ON CONFLICT(object_id,owner_actor_id) DO UPDATE SET location_id=excluded.location_id`, row.Object, row.Owner, row.Location); err != nil {
					return err
				}
			case "rp_object_offer":
				var row rpObjectOfferProjection
				if err := json.Unmarshal([]byte(d.ExpectedText), &row); err != nil || row.Instance != instance || row.Branch != branch || row.ID != d.Key {
					return core.NewError(core.CodeProjectionDiverged, "offer repair fact or scope invalid")
				}
				if _, err := conn.ExecContext(ctx, `INSERT INTO rp_object_offers(offer_id,instance_id,branch_id,object_id,from_actor_id,target_actor_id,status,offer_event_id,response_event_id,terminal_event_id,last_event_sequence) VALUES (?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(offer_id) DO UPDATE SET object_id=excluded.object_id,from_actor_id=excluded.from_actor_id,target_actor_id=excluded.target_actor_id,status=excluded.status,offer_event_id=excluded.offer_event_id,response_event_id=excluded.response_event_id,terminal_event_id=excluded.terminal_event_id,last_event_sequence=excluded.last_event_sequence WHERE rp_object_offers.instance_id=excluded.instance_id AND rp_object_offers.branch_id=excluded.branch_id`, row.ID, instance, branch, row.Object, row.From, row.Target, row.Status, row.OfferEvent, nullRPObjectString(row.ResponseEvent), nullRPObjectString(row.TerminalEvent), row.Sequence); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
