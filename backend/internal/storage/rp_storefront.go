package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"corerp.local/backend/internal/core"
)

type RPStorefrontSource struct {
	NoticeBasisPoints int    `json:"notice_basis_points,omitempty"`
	PlaceID           string `json:"place_id"`
	StoreActorID      string `json:"store_actor_id"`
	SKUID             string `json:"sku_id"`
}
type StorefrontSourceRequest struct {
	Binding core.CareerBinding `json:"binding"`
	Source  RPStorefrontSource `json:"source"`
}
type StorefrontSourceFact struct {
	PolicyEventID        string             `json:"policy_event_id,omitempty"`
	Version              string             `json:"version"`
	BuilderSourceEventID string             `json:"builder_source_event_id"`
	PlaceSourceEventID   string             `json:"place_source_event_id"`
	StoreSourceEventID   string             `json:"store_source_event_id"`
	StockLocationID      string             `json:"stock_location_id"`
	Source               RPStorefrontSource `json:"source"`
}
type StorefrontSourceRecord = privateFactRecord[StorefrontSourceFact]

// Explicit creator declaration publishes one existing store's shelf availability
// at one actual place. It neither creates stock nor grants purchase authority.
func (s *Store) DefineRPStorefrontSource(ctx context.Context, r StorefrontSourceRequest) (StorefrontSourceRecord, error) {
	if _, err := core.RPOpportunityProbability(r.Source.NoticeBasisPoints, false, core.RPOpportunityPressure{}); err != nil {
		return StorefrontSourceRecord{}, err
	}
	for _, value := range []string{r.Source.PlaceID, r.Source.StoreActorID, r.Source.SKUID} {
		if strings.TrimSpace(value) == "" || len(value) > 256 {
			return StorefrontSourceRecord{}, core.NewError(core.CodeInvalidArgument, "storefront requires bounded place, store and SKU")
		}
	}
	var builder string
	return executePrivateFactCommand(s, ctx, r.Binding, "DefineRPStorefrontSource", r,
		privateFactDomain{"storefront", "RPStorefrontSourceDefined", `{"authorization":"storefront-builder-v1"}`},
		func(conn *sql.Conn) error {
			var err error
			builder, err = authorizeCultureBuilder(ctx, conn, r.Binding)
			return err
		},
		func(conn *sql.Conn, _ privateFactContext) (StorefrontSourceFact, func() error, error) {
			fact := StorefrontSourceFact{Version: "corerp.storefront.source.v1", BuilderSourceEventID: builder, Source: r.Source}
			if r.Source.NoticeBasisPoints > 0 {
				policy, err := readRPOpportunityPolicy(ctx, conn, r.Binding.InstanceID, r.Binding.BranchID)
				if err != nil {
					return fact, nil, classifyMissing(err, "installed shortage opportunity stream")
				}
				fact.PolicyEventID = policy.EventID
			}
			if err := conn.QueryRowContext(ctx, `SELECT definition_event_id FROM agent_places WHERE place_id=? AND instance_id=? AND branch_id=? AND status='active'`, r.Source.PlaceID, r.Binding.InstanceID, r.Binding.BranchID).Scan(&fact.PlaceSourceEventID); err != nil {
				return fact, nil, classifyMissing(err, "actual storefront place")
			}
			if err := conn.QueryRowContext(ctx, `SELECT a.definition_event_id,a.stock_location_id FROM m2_economic_actors a JOIN stock_locations l ON l.location_id=a.stock_location_id AND l.owner_id=a.actor_id AND l.location_kind='holder' JOIN inventory_balances i ON i.location_id=l.location_id AND i.sku_id=? WHERE a.actor_id=? AND a.kind='store' AND a.instance_id=? AND a.branch_id=?`, r.Source.SKUID, r.Source.StoreActorID, r.Binding.InstanceID, r.Binding.BranchID).Scan(&fact.StoreSourceEventID, &fact.StockLocationID); err != nil {
				return fact, nil, classifyMissing(err, "actual stocked branch store")
			}
			var duplicates, localCount int
			if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPStorefrontSourceDefined' AND json_extract(payload,'$.source.store_actor_id')=? AND json_extract(payload,'$.source.sku_id')=?`, r.Binding.InstanceID, r.Binding.BranchID, r.Source.StoreActorID, r.Source.SKUID).Scan(&duplicates); err != nil {
				return fact, nil, err
			}
			if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPStorefrontSourceDefined' AND json_extract(payload,'$.source.place_id')=?`, r.Binding.InstanceID, r.Binding.BranchID, r.Source.PlaceID).Scan(&localCount); err != nil {
				return fact, nil, err
			}
			if duplicates != 0 || localCount >= 8 {
				return fact, nil, core.NewError(core.CodeBranchConflict, "storefront already declared or place capacity reached")
			}
			if _, _, err := readRPStoreStock(ctx, conn, r.Binding.InstanceID, r.Binding.BranchID, fact.StockLocationID, r.Source.SKUID); err != nil {
				return fact, nil, err
			}
			return fact, nil, nil
		})
}

// Facts remain the stock authority; a corrupted projection cannot invent a
// shortage. Scope comes from the declared branch store, not caller-owned IDs.
func readRPStoreStock(ctx context.Context, conn *sql.Conn, instance, branch, location, sku string) (int64, string, error) {
	var projected, actual int64
	if err := conn.QueryRowContext(ctx, `SELECT quantity_minor FROM inventory_balances WHERE location_id=? AND sku_id=?`, location, sku).Scan(&projected); err != nil {
		return 0, "", classifyMissing(err, "store stock")
	}
	err := conn.QueryRowContext(ctx, `SELECT COALESCE(SUM(CASE WHEN m.to_location_id=? AND m.movement_kind IN ('transfer','create') THEN m.quantity_minor ELSE 0 END - CASE WHEN m.from_location_id=? AND m.movement_kind IN ('transfer','consume','destroy') THEN m.quantity_minor ELSE 0 END),0) FROM stock_movements m JOIN events e ON e.event_id=m.event_id WHERE e.instance_id=? AND e.branch_id=? AND m.sku_id=? AND (m.to_location_id=? OR m.from_location_id=?)`, location, location, instance, branch, sku, location, location).Scan(&actual)
	if err != nil {
		return 0, "", err
	}
	if actual < 0 || actual != projected {
		return 0, "", core.NewError(core.CodeProjectionDiverged, "store shelf differs from stock facts")
	}
	var source string
	if err := conn.QueryRowContext(ctx, `SELECT e.event_id FROM stock_movements m JOIN events e ON e.event_id=m.event_id WHERE e.instance_id=? AND e.branch_id=? AND m.sku_id=? AND ((m.to_location_id=? AND m.movement_kind IN ('transfer','create')) OR (m.from_location_id=? AND m.movement_kind IN ('transfer','consume','destroy'))) ORDER BY e.event_sequence DESC LIMIT 1`, instance, branch, sku, location, location).Scan(&source); err != nil {
		return 0, "", classifyMissing(err, "store stock provenance")
	}
	return actual, source, nil
}

func readRPLocalStores(ctx context.Context, conn *sql.Conn, instance, branch, place string) ([]core.RPStoreAvailability, error) {
	rows, err := conn.QueryContext(ctx, `SELECT event_id,payload FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPStorefrontSourceDefined' AND json_extract(payload,'$.source.place_id')=? ORDER BY event_sequence LIMIT 8`, instance, branch, place)
	if err != nil {
		return nil, err
	}
	type declared struct {
		id   string
		fact StorefrontSourceFact
	}
	var sources []declared
	for rows.Next() {
		var row declared
		var raw string
		if err := rows.Scan(&row.id, &raw); err != nil {
			rows.Close()
			return nil, err
		}
		if err := json.Unmarshal([]byte(raw), &row.fact); err != nil {
			rows.Close()
			return nil, err
		}
		sources = append(sources, row)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	var result []core.RPStoreAvailability
	for _, source := range sources {
		quantity, stockSource, err := readRPStoreStock(ctx, conn, instance, branch, source.fact.StockLocationID, source.fact.Source.SKUID)
		if err != nil {
			return nil, err
		}
		result = append(result, core.RPStoreAvailability{PlaceID: place, StoreActorID: source.fact.Source.StoreActorID, SKUID: source.fact.Source.SKUID, Available: quantity > 0, StorefrontSourceEventID: source.id, StockSourceEventID: stockSource})
	}
	return result, nil
}
