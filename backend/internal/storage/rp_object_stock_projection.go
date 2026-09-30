package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"corerp.local/backend/internal/core"
)

type rpObjectStockSKUProjection struct {
	ID, Unit, Event string
	Scale           int
}

// The SKU catalog is a projection of its creator Event. The movement remains
// an independently checked inventory fact; rebuilding must not manufacture a
// missing movement or change its quantity merely to satisfy the Event.
func rpObjectStockProjectionDifferences(ctx context.Context, q replayQuerier, instance, branch string, through int64) ([]ProjectionDifference, error) {
	rows, err := q.QueryContext(ctx, `SELECT event_id,actor_id,payload FROM events WHERE instance_id=? AND branch_id=? AND event_sequence<=? AND event_type='RPObjectStockDefined' ORDER BY event_sequence`, instance, branch, through)
	if err != nil {
		return nil, err
	}
	type stockEvent struct{ id, actor, raw string }
	var events []stockEvent
	for rows.Next() {
		var event stockEvent
		if err := rows.Scan(&event.id, &event.actor, &event.raw); err != nil {
			rows.Close()
			return nil, err
		}
		events = append(events, event)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	var differences []ProjectionDifference
	expected := make(map[string]rpObjectStockSKUProjection)
	for _, event := range events {
		var fact RPObjectStockFact
		if err := json.Unmarshal([]byte(event.raw), &fact); err != nil {
			return nil, err
		}
		sku, err := core.StudioWorldObjectID(instance, "sku", fact.SKUCode)
		if err != nil {
			return nil, core.NewError(core.CodeProjectionDiverged, "invalid authored stock SKU code")
		}
		source, err := core.StudioWorldObjectID(instance, "location", "source")
		if err != nil {
			return nil, err
		}
		if fact.Version != "corerp.object.stock.v1" || sku != fact.SKUID || source != fact.SourceLocationID || fact.SKUCode == "staple" || !validRPObjectName(fact.SKUDisplayName) || fact.BaseUnit != "unit" || fact.QuantityScale != 0 || fact.QuantityMinor != 1 || fact.OwnerEntityID == "" || fact.InventoryLocationID == "" || fact.GenesisEventID == "" {
			return nil, core.NewError(core.CodeProjectionDiverged, "invalid authored stock definition Event")
		}
		if _, duplicate := expected[sku]; duplicate {
			return nil, core.NewError(core.CodeProjectionDiverged, "duplicate authored stock SKU")
		}
		var sourceValid, movementTotal, movementExact int
		if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM events e JOIN stock_locations l ON l.location_id=? AND l.owner_id=? AND l.location_kind='source' AND l.capability_id='cap_inventory_create' WHERE e.event_id=? AND e.instance_id=? AND e.branch_id=? AND e.actor_id=? AND e.event_sequence=1 AND e.event_type='StudioWorldPrepared'`, source, instance, fact.GenesisEventID, instance, branch, event.actor).Scan(&sourceValid); err != nil {
			return nil, err
		}
		if sourceValid != 1 {
			return nil, core.NewError(core.CodeProjectionDiverged, "authored stock has no scoped genesis source")
		}
		if err := q.QueryRowContext(ctx, `SELECT COUNT(*),COALESCE(SUM(CASE WHEN sku_id=? AND from_location_id=? AND to_location_id=? AND quantity_minor=1 AND movement_kind='create' AND reason_code='authored_object_stock' AND capability_id='cap_inventory_create' THEN 1 ELSE 0 END),0) FROM stock_movements WHERE event_id=?`, sku, source, fact.InventoryLocationID, event.id).Scan(&movementTotal, &movementExact); err != nil {
			return nil, err
		}
		if movementTotal != 1 || movementExact != 1 {
			return nil, core.NewError(core.CodeProjectionDiverged, "authored SKU has no unique finite stock creation fact")
		}
		want := rpObjectStockSKUProjection{sku, "unit", event.id, 0}
		expected[sku] = want
		var got rpObjectStockSKUProjection
		err = q.QueryRowContext(ctx, `SELECT sku_id,base_unit,definition_event_id,quantity_scale FROM product_skus WHERE sku_id=?`, sku).Scan(&got.ID, &got.Unit, &got.Event, &got.Scale)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		if err == nil && got == want {
			continue
		}
		encodedWant, _ := core.CanonicalJSON(want)
		actual := "missing"
		if err == nil {
			encodedGot, _ := core.CanonicalJSON(got)
			actual = string(encodedGot)
		}
		differences = append(differences, ProjectionDifference{Projection: "rp_object_stock_sku", Key: sku, ExpectedText: string(encodedWant), ActualText: actual})
	}
	rows, err = q.QueryContext(ctx, `SELECT k.sku_id FROM product_skus k JOIN events e ON e.event_id=k.definition_event_id WHERE e.instance_id=? AND e.branch_id=? AND e.event_type='RPObjectStockDefined'`, instance, branch)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var sku string
		if err := rows.Scan(&sku); err != nil {
			rows.Close()
			return nil, err
		}
		if _, ok := expected[sku]; !ok {
			differences = append(differences, ProjectionDifference{Projection: "rp_object_stock_sku_extra", Key: sku, ExpectedText: "absent", ActualText: "unsourced catalog SKU"})
		}
	}
	err = rows.Err()
	rows.Close()
	return differences, err
}

func repairRPObjectStockProjections(ctx context.Context, conn *sql.Conn, instance, branch string, differences []ProjectionDifference) error {
	for _, difference := range differences {
		if difference.Projection == "rp_object_stock_sku_extra" {
			return core.NewError(core.CodeProjectionDiverged, "cannot erase an unsourced SKU")
		}
		if difference.Projection != "rp_object_stock_sku" {
			continue
		}
		var row rpObjectStockSKUProjection
		if json.Unmarshal([]byte(difference.ExpectedText), &row) != nil || row.ID != difference.Key || row.Unit != "unit" || row.Scale != 0 {
			return core.NewError(core.CodeProjectionDiverged, "invalid SKU repair fact")
		}
		var scope string
		err := conn.QueryRowContext(ctx, `SELECT e.instance_id || ':' || e.branch_id FROM product_skus k LEFT JOIN events e ON e.event_id=k.definition_event_id WHERE k.sku_id=?`, row.ID).Scan(&scope)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if err == nil && scope != instance+":"+branch {
			return core.NewError(core.CodeProjectionDiverged, "cannot overwrite a SKU sourced to another world")
		}
		if _, err := conn.ExecContext(ctx, `INSERT INTO product_skus(sku_id,base_unit,quantity_scale,definition_event_id) VALUES (?,?,?,?) ON CONFLICT(sku_id) DO UPDATE SET base_unit=excluded.base_unit,quantity_scale=excluded.quantity_scale,definition_event_id=excluded.definition_event_id`, row.ID, row.Unit, row.Scale, row.Event); err != nil {
			return err
		}
	}
	return nil
}
