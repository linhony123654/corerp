package storage

import (
	"context"
	"database/sql"
	"strings"
	"unicode"
	"unicode/utf8"

	"corerp.local/backend/internal/core"
)

type RPObjectAnchorRequest struct {
	Binding      core.CareerBinding `json:"binding"`
	PlaceID      string             `json:"place_id"`
	ZoneKey      string             `json:"zone_key"`
	AnchorCode   string             `json:"anchor_code"`
	DisplayName  string             `json:"display_name"`
	NearEntityID string             `json:"near_entity_id,omitempty"`
}

type RPObjectAnchorFact struct {
	Version      string `json:"version"`
	AnchorID     string `json:"anchor_id"`
	PlaceID      string `json:"place_id"`
	ZoneKey      string `json:"zone_key"`
	AnchorCode   string `json:"anchor_code"`
	DisplayName  string `json:"display_name"`
	NearEntityID string `json:"near_entity_id,omitempty"`
}

type RPObjectAnchorRecord = privateFactRecord[RPObjectAnchorFact]

func validRPObjectName(name string) bool {
	if name == "" || len(name) > 128 || strings.TrimSpace(name) != name || !utf8.ValidString(name) {
		return false
	}
	for _, ch := range name {
		if unicode.IsControl(ch) {
			return false
		}
	}
	return true
}

// A creator authors a fixed, scene-local reach point; a visual range is not a
// reach range. NearEntityID is a descriptive association, not a moving anchor.
func (s *Store) DefineRPObjectAnchor(ctx context.Context, r RPObjectAnchorRequest) (RPObjectAnchorRecord, error) {
	if err := r.Binding.Validate(); err != nil {
		return RPObjectAnchorRecord{}, err
	}
	if r.PlaceID == "" || len(r.PlaceID) > 256 || strings.TrimSpace(r.PlaceID) != r.PlaceID || !rpZoneKey.MatchString(r.ZoneKey) || !rpLocationSlotKey.MatchString(r.AnchorCode) || !validRPObjectName(r.DisplayName) || len(r.NearEntityID) > 256 || strings.TrimSpace(r.NearEntityID) != r.NearEntityID {
		return RPObjectAnchorRecord{}, core.NewError(core.CodeInvalidArgument, "object anchor needs a bounded place, zone, code and description")
	}
	idHash, err := core.HashJSON([]string{r.Binding.InstanceID, r.Binding.BranchID, r.PlaceID, r.ZoneKey, r.AnchorCode})
	if err != nil {
		return RPObjectAnchorRecord{}, err
	}
	anchorID := "rp_anchor_" + idHash[7:]
	return executePrivateFactCommand(s, ctx, r.Binding, "DefineRPObjectAnchor", r,
		privateFactDomain{"rp_object_anchor", "RPObjectAnchorDefined", `{"authorization":"scoped-world-creator"}`},
		func(conn *sql.Conn) error { return authorizeRPLocationBuilder(ctx, conn, r.Binding) },
		func(conn *sql.Conn, c privateFactContext) (RPObjectAnchorFact, func() error, error) {
			fact := RPObjectAnchorFact{"corerp.object.anchor.v1", anchorID, r.PlaceID, r.ZoneKey, r.AnchorCode, r.DisplayName, r.NearEntityID}
			var active int
			if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM agent_places WHERE place_id=? AND instance_id=? AND branch_id=? AND status='active'`, r.PlaceID, r.Binding.InstanceID, r.Binding.BranchID).Scan(&active); err != nil {
				return fact, nil, err
			}
			if active != 1 {
				return fact, nil, core.NewError(core.CodeNotFound, "object anchor place is not active")
			}
			if r.ZoneKey != "main" {
				var declared int
				if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_perception_links WHERE instance_id=? AND branch_id=? AND place_id=? AND (zone_a=? OR zone_b=?)`, r.Binding.InstanceID, r.Binding.BranchID, r.PlaceID, r.ZoneKey, r.ZoneKey).Scan(&declared); err != nil {
					return fact, nil, err
				}
				if declared == 0 {
					return fact, nil, core.NewError(core.CodeNotFound, "object anchor zone has no authored source")
				}
			}
			if r.NearEntityID != "" {
				point, err := readRPActorSpatialPoint(ctx, conn, r.Binding.InstanceID, r.Binding.BranchID, r.NearEntityID)
				if err != nil || point.PlaceID != r.PlaceID || point.ZoneKey != r.ZoneKey {
					return fact, nil, core.NewError(core.CodeNotFound, "object anchor's named neighbor is not actually at its reach point")
				}
			}
			var existing int
			if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_object_anchors WHERE instance_id=? AND branch_id=? AND place_id=? AND zone_key=? AND anchor_code=?`, r.Binding.InstanceID, r.Binding.BranchID, r.PlaceID, r.ZoneKey, r.AnchorCode).Scan(&existing); err != nil {
				return fact, nil, err
			}
			if existing != 0 {
				return fact, nil, core.NewError(core.CodeBranchConflict, "object reach point already declared")
			}
			return fact, func() error {
				return execAgentOne(ctx, conn, "define object reach point", `INSERT INTO rp_object_anchors(anchor_id,instance_id,branch_id,place_id,zone_key,anchor_code,display_name,near_entity_id,definition_event_id) VALUES (?,?,?,?,?,?,?,?,?)`, anchorID, r.Binding.InstanceID, r.Binding.BranchID, r.PlaceID, r.ZoneKey, r.AnchorCode, r.DisplayName, nullRPObjectString(r.NearEntityID), c.EventID)
			}, nil
		})
}

type RPObjectSourceRequest struct {
	Binding       core.CareerBinding `json:"binding"`
	AnchorID      string             `json:"anchor_id"`
	OwnerEntityID string             `json:"owner_entity_id"`
	SKUID         string             `json:"sku_id"`
	DisplayName   string             `json:"display_name"`
}

type RPObjectSourceFact struct {
	Version             string `json:"version"`
	SourceID            string `json:"source_id"`
	AnchorID            string `json:"anchor_id"`
	AnchorEventID       string `json:"anchor_event_id"`
	OwnerEntityID       string `json:"owner_entity_id"`
	InventoryLocationID string `json:"inventory_location_id"`
	SKUID               string `json:"sku_id"`
	SKUEventID          string `json:"sku_event_id"`
	UnitMinor           int64  `json:"unit_minor"`
	DisplayName         string `json:"display_name"`
	StockEventID        string `json:"stock_event_id"`
}

type RPObjectSourceRecord = privateFactRecord[RPObjectSourceFact]

// Existing finite personal inventory is the only source of an individual
// object. Only discrete unit SKUs can be staged as single physical objects.
func (s *Store) DefineRPObjectSource(ctx context.Context, r RPObjectSourceRequest) (RPObjectSourceRecord, error) {
	if err := r.Binding.Validate(); err != nil {
		return RPObjectSourceRecord{}, err
	}
	for _, value := range []string{r.AnchorID, r.OwnerEntityID, r.SKUID} {
		if value == "" || len(value) > 256 || strings.TrimSpace(value) != value {
			return RPObjectSourceRecord{}, core.NewError(core.CodeInvalidArgument, "object source needs an anchor, owner and existing SKU")
		}
	}
	if !validRPObjectName(r.DisplayName) {
		return RPObjectSourceRecord{}, core.NewError(core.CodeInvalidArgument, "object source needs a bounded item name")
	}
	return executePrivateFactCommand(s, ctx, r.Binding, "DefineRPObjectSource", r,
		privateFactDomain{"rp_object_source", "RPObjectSourceDefined", `{"authorization":"scoped-world-creator"}`},
		func(conn *sql.Conn) error { return authorizeRPLocationBuilder(ctx, conn, r.Binding) },
		func(conn *sql.Conn, c privateFactContext) (RPObjectSourceFact, func() error, error) {
			fact := RPObjectSourceFact{Version: "corerp.object.source.v1", AnchorID: r.AnchorID, OwnerEntityID: r.OwnerEntityID, SKUID: r.SKUID, DisplayName: r.DisplayName}
			if err := conn.QueryRowContext(ctx, `SELECT a.definition_event_id FROM rp_object_anchors a JOIN agent_places p ON p.place_id=a.place_id AND p.instance_id=a.instance_id AND p.branch_id=a.branch_id AND p.status='active' WHERE a.anchor_id=? AND a.instance_id=? AND a.branch_id=?`, r.AnchorID, r.Binding.InstanceID, r.Binding.BranchID).Scan(&fact.AnchorEventID); err != nil {
				return fact, nil, classifyMissing(err, "declared object reach point")
			}
			if err := conn.QueryRowContext(ctx, `SELECT m.inventory_location_id FROM materialized_entities m JOIN cohorts c ON c.cohort_id=m.source_cohort_id JOIN agent_profiles a ON a.agent_id=m.entity_id AND a.status='active' JOIN stock_locations l ON l.location_id=m.inventory_location_id AND l.owner_id=m.entity_id AND l.location_kind='holder' WHERE m.entity_id=? AND m.status='active' AND m.population_count=1 AND a.instance_id=? AND a.branch_id=? AND c.instance_id=a.instance_id AND c.branch_id=a.branch_id`, r.OwnerEntityID, r.Binding.InstanceID, r.Binding.BranchID).Scan(&fact.InventoryLocationID); err != nil {
				return fact, nil, classifyMissing(err, "source actor's own finite inventory")
			}
			var scale int
			var unit string
			if err := conn.QueryRowContext(ctx, `SELECT base_unit,quantity_scale,definition_event_id FROM product_skus WHERE sku_id=?`, r.SKUID).Scan(&unit, &scale, &fact.SKUEventID); err != nil {
				return fact, nil, classifyMissing(err, "real product SKU")
			}
			var authoredName sql.NullString
			if err := conn.QueryRowContext(ctx, `SELECT CASE WHEN json_extract(payload,'$.sku_id')=? THEN json_extract(payload,'$.sku_display_name') END FROM events WHERE event_id=? AND instance_id=? AND branch_id=?`, r.SKUID, fact.SKUEventID, r.Binding.InstanceID, r.Binding.BranchID).Scan(&authoredName); err != nil {
				return fact, nil, classifyMissing(err, "branch-local SKU definition Event")
			}
			// Old SKU definitions carry no product name. Their only safe label is
			// the SKU ID; a source declaration must not relabel bread as a cup.
			skuName := r.SKUID
			if authoredName.Valid && authoredName.String != "" {
				skuName = authoredName.String
			}
			if r.DisplayName != skuName || !validRPObjectName(skuName) {
				return fact, nil, core.NewError(core.CodeInvalidArgument, "object label must match the SKU definition")
			}
			if unit != "unit" || scale != 0 {
				return fact, nil, core.NewError(core.CodeInvalidArgument, "only discrete unit SKUs can become individual objects")
			}
			fact.UnitMinor = 1
			var err error
			var available int64
			available, fact.StockEventID, err = readRPStoreStock(ctx, conn, r.Binding.InstanceID, r.Binding.BranchID, fact.InventoryLocationID, r.SKUID)
			if err != nil {
				return fact, nil, err
			}
			if available < fact.UnitMinor {
				return fact, nil, core.NewError(core.CodeBranchConflict, "no unescrowed single SKU unit for object source")
			}
			idHash, err := core.HashJSON([]string{r.Binding.InstanceID, r.Binding.BranchID, r.Binding.PrincipalID, r.Binding.IdempotencyKey})
			if err != nil {
				return fact, nil, err
			}
			fact.SourceID = "rp_source_" + idHash[7:]
			return fact, func() error {
				return execAgentOne(ctx, conn, "declare existing object source", `INSERT INTO rp_object_sources(source_id,instance_id,branch_id,anchor_id,owner_actor_id,inventory_location_id,sku_id,unit_minor,display_name,definition_event_id) VALUES (?,?,?,?,?,?,?,?,?,?)`, fact.SourceID, r.Binding.InstanceID, r.Binding.BranchID, r.AnchorID, r.OwnerEntityID, fact.InventoryLocationID, r.SKUID, fact.UnitMinor, r.DisplayName, c.EventID)
			}, nil
		})
}

func nullRPObjectString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
