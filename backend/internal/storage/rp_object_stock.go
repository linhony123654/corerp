package storage

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"corerp.local/backend/internal/core"
)

// RPObjectStockRequest authors one discrete SKU and one finite unit in a
// participant's inventory. It is a scoped creator command, never an AUTO
// interaction or an interpretation of a player's words.
type RPObjectStockRequest struct {
	Binding       core.CareerBinding `json:"binding"`
	SKUCode       string             `json:"sku_code"`
	DisplayName   string             `json:"display_name"`
	OwnerEntityID string             `json:"owner_entity_id"`
}

type RPObjectStockFact struct {
	Version             string `json:"version"`
	SKUCode             string `json:"sku_code"`
	SKUID               string `json:"sku_id"`
	SKUDisplayName      string `json:"sku_display_name"`
	BaseUnit            string `json:"base_unit"`
	QuantityScale       int    `json:"quantity_scale"`
	QuantityMinor       int64  `json:"quantity_minor"`
	OwnerEntityID       string `json:"owner_entity_id"`
	InventoryLocationID string `json:"inventory_location_id"`
	SourceLocationID    string `json:"source_location_id"`
	GenesisEventID      string `json:"genesis_event_id"`
}

type RPObjectStockRecord = privateFactRecord[RPObjectStockFact]

// DefineRPObjectStock gives a new Studio-world product an authored label and
// exactly one stock-backed unit. A separate owner still has to stage that
// existing inventory into object escrow before it is a physical RP object.
func (s *Store) DefineRPObjectStock(ctx context.Context, r RPObjectStockRequest) (RPObjectStockRecord, error) {
	if err := r.Binding.Validate(); err != nil {
		return RPObjectStockRecord{}, err
	}
	if !rpLocationSlotKey.MatchString(r.SKUCode) || r.SKUCode == "staple" || !validRPObjectName(r.DisplayName) || r.OwnerEntityID == "" || len(r.OwnerEntityID) > 256 || strings.TrimSpace(r.OwnerEntityID) != r.OwnerEntityID {
		return RPObjectStockRecord{}, core.NewError(core.CodeInvalidArgument, "new object stock needs a distinct SKU code, product name and owner")
	}
	sku, err := core.StudioWorldObjectID(r.Binding.InstanceID, "sku", r.SKUCode)
	if err != nil {
		return RPObjectStockRecord{}, err
	}
	source, err := core.StudioWorldObjectID(r.Binding.InstanceID, "location", "source")
	if err != nil {
		return RPObjectStockRecord{}, err
	}
	return executePrivateFactCommand(s, ctx, r.Binding, "DefineRPObjectStock", r,
		privateFactDomain{"rp_object_stock", "RPObjectStockDefined", `{"authorization":"scoped-world-creator"}`},
		func(conn *sql.Conn) error {
			if r.Binding.InstanceID == M2DemoInstanceID {
				return core.NewError(core.CodeUnauthorized, "object stock creation is limited to authored Studio worlds")
			}
			return authorizeRPLocationBuilder(ctx, conn, r.Binding)
		},
		func(conn *sql.Conn, c privateFactContext) (RPObjectStockFact, func() error, error) {
			fact := RPObjectStockFact{Version: "corerp.object.stock.v1", SKUCode: r.SKUCode, SKUID: sku, SKUDisplayName: r.DisplayName, BaseUnit: "unit", QuantityMinor: 1, OwnerEntityID: r.OwnerEntityID, SourceLocationID: source}
			if err := requireNoActiveRPSharedRound(ctx, conn, r.Binding.InstanceID, r.Binding.BranchID); err != nil {
				return fact, nil, err
			}
			var active int
			if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM world_instances w JOIN branches b ON b.instance_id=w.instance_id JOIN world_clocks c ON c.instance_id=b.instance_id AND c.branch_id=b.branch_id WHERE w.instance_id=? AND b.branch_id=? AND w.lifecycle_state='active' AND c.status='ready'`, r.Binding.InstanceID, r.Binding.BranchID).Scan(&active); err != nil {
				return fact, nil, err
			}
			if active != 1 {
				return fact, nil, core.NewError(core.CodeBranchConflict, "object stock requires a ready authored world")
			}
			if err := conn.QueryRowContext(ctx, `SELECT e.event_id FROM events e JOIN stock_locations l ON l.location_id=? AND l.owner_id=? AND l.location_kind='source' AND l.capability_id='cap_inventory_create' WHERE e.instance_id=? AND e.branch_id=? AND e.event_sequence=1 AND e.event_type='StudioWorldPrepared'`, source, r.Binding.InstanceID, r.Binding.InstanceID, r.Binding.BranchID).Scan(&fact.GenesisEventID); err != nil {
				return fact, nil, classifyMissing(err, "Studio inventory creation source")
			}
			if err := conn.QueryRowContext(ctx, `SELECT m.inventory_location_id FROM materialized_entities m JOIN cohorts c ON c.cohort_id=m.source_cohort_id JOIN agent_profiles a ON a.agent_id=m.entity_id AND a.status='active' JOIN stock_locations l ON l.location_id=m.inventory_location_id AND l.owner_id=m.entity_id AND l.location_kind='holder' WHERE m.entity_id=? AND m.status='active' AND m.population_count=1 AND a.instance_id=? AND a.branch_id=? AND c.instance_id=a.instance_id AND c.branch_id=a.branch_id`, r.OwnerEntityID, r.Binding.InstanceID, r.Binding.BranchID).Scan(&fact.InventoryLocationID); err != nil {
				return fact, nil, classifyMissing(err, "single resident inventory for authored stock")
			}
			var existing int
			if err := conn.QueryRowContext(ctx, `SELECT 1 FROM product_skus WHERE sku_id=?`, sku).Scan(&existing); err == nil {
				return fact, nil, core.NewError(core.CodeBranchConflict, "SKU already has a definition; stock cannot be recreated")
			} else if !errors.Is(err, sql.ErrNoRows) {
				return fact, nil, err
			}
			return fact, func() error {
				for _, st := range []struct {
					query string
					args  []any
				}{
					{`INSERT INTO product_skus(sku_id,base_unit,quantity_scale,definition_event_id) VALUES (?,'unit',0,?)`, []any{sku, c.EventID}},
					{`INSERT INTO inventory_balances(location_id,sku_id,quantity_minor,projection_version,last_event_sequence) VALUES (?,?,0,0,?)`, []any{source, sku, c.Sequence}},
					{`INSERT INTO inventory_balances(location_id,sku_id,quantity_minor,projection_version,last_event_sequence) VALUES (?,?,1,1,?)`, []any{fact.InventoryLocationID, sku, c.Sequence}},
					{`INSERT INTO stock_movements(movement_id,event_id,sku_id,from_location_id,to_location_id,quantity_minor,movement_kind,reason_code,capability_id) VALUES (?,?,?,?,?,1,'create','authored_object_stock','cap_inventory_create')`, []any{"stock_" + c.EventID, c.EventID, sku, source, fact.InventoryLocationID}},
				} {
					if err := execAgentOne(ctx, conn, "create sourced discrete stock", st.query, st.args...); err != nil {
						return err
					}
				}
				return nil
			}, nil
		})
}
