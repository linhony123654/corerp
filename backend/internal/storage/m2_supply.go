package storage

import (
	"context"
	"database/sql"
	"fmt"

	"corerp.local/backend/internal/core"
)

func prepareM2Consumption(ctx context.Context, conn *sql.Conn, item SchedulerItem, payload scheduledPayload) (scheduledMutation, error) {
	if item.PhaseID != m2FoodConsumePhase || payload.Kind != "m2_food_consume" || payload.SubjectID != m2StoreOfferID || payload.Day < 1 || payload.Day > 30 || item.WorldTime != m2WageTime(payload.Day, 7, 6) || item.SchedulerItemID != fmt.Sprintf("sched_m2_food_consume_day_%d", payload.Day) {
		return scheduledMutation{}, core.NewError(core.CodeStorageFailure, "invalid M2 food consumption task")
	}
	purchaseID := fmt.Sprintf("sched_m2_food_buy_day_%d", payload.Day)
	var purchaseStatus, purchaseEventID, purchaseEventType string
	var purchaseQuantity int64
	if err := conn.QueryRowContext(ctx, `SELECT p.status, p.quantity_minor, p.event_id, e.event_type FROM m2_purchase_outcomes p JOIN events e ON e.event_id = p.event_id WHERE p.scheduler_item_id = ? AND p.offer_id = ? AND p.day = ? AND e.instance_id = ? AND e.branch_id = ?`, purchaseID, m2StoreOfferID, payload.Day, M2DemoInstanceID, M2DemoBranchID).Scan(&purchaseStatus, &purchaseQuantity, &purchaseEventID, &purchaseEventType); err != nil {
		return scheduledMutation{}, classifyMissing(err, "M2 same-day purchase decision")
	}
	if (purchaseStatus == "purchased" && (purchaseQuantity != 1 || purchaseEventType != "M2FoodPurchased")) || (purchaseStatus == "rejected" && (purchaseQuantity != 0 || purchaseEventType != "M2FoodPurchaseRejected")) {
		return scheduledMutation{}, core.NewError(core.CodeProjectionDiverged, "M2 purchase outcome differs from event authority")
	}
	var sinkCapability, householdOwner, sinkKind string
	if err := conn.QueryRowContext(ctx, `SELECT sink.capability_id, source.owner_id, sink.location_kind FROM stock_locations sink JOIN stock_locations source ON source.location_id = ? WHERE sink.location_id = ?`, m2HouseholdLocation, m2FoodSinkLocation).Scan(&sinkCapability, &householdOwner, &sinkKind); err != nil {
		return scheduledMutation{}, classifyMissing(err, "M2 food consumption ownership")
	}
	if sinkCapability != m2FoodConsumeCapability || householdOwner != M2DemoCohortID || sinkKind != "sink" {
		return scheduledMutation{}, core.NewError(core.CodeProjectionDiverged, "M2 food consumption capability or owner differs from definition")
	}
	if err := verifyM2InventoryProjection(ctx, conn, m2HouseholdLocation, M2DemoSKUID); err != nil {
		return scheduledMutation{}, err
	}
	stock, version, err := readScheduledInventory(ctx, conn, m2HouseholdLocation, M2DemoSKUID)
	if err != nil {
		return scheduledMutation{}, err
	}
	status, reason, quantity := "skipped", "purchase_rejected", int64(0)
	mutation := scheduledMutation{EventType: "M2FoodConsumptionSkipped"}
	if purchaseStatus == "purchased" {
		if stock < 1 {
			return scheduledMutation{}, core.NewError(core.CodeProjectionDiverged, "successful M2 purchase has no household stock to consume")
		}
		status, reason, quantity = "consumed", "none", 1
		mutation.EventType = "M2FoodConsumed"
		capability := m2FoodConsumeCapability
		mutation.Movements = []scheduledMovement{{M2DemoSKUID, m2HouseholdLocation, m2FoodSinkLocation, 1, "consume", "m2_daily_food_consumption", &capability}}
		mutation.Inventory = []inventoryMutation{{m2HouseholdLocation, M2DemoSKUID, version, stock - 1}}
	}
	mutation.EventPayload = struct {
		PurchaseEventID string `json:"purchase_event_id"`
		PurchaseItemID  string `json:"purchase_item_id"`
		Day             int    `json:"day"`
		SKUID           string `json:"sku_id"`
		Status          string `json:"status"`
		Reason          string `json:"reason_code"`
		Quantity        int64  `json:"quantity_minor"`
		SinkLocationID  string `json:"sink_location_id"`
		CapabilityID    string `json:"capability_id"`
	}{purchaseEventID, purchaseID, payload.Day, M2DemoSKUID, status, reason, quantity, m2FoodSinkLocation, m2FoodConsumeCapability}
	mutation.ApplyDomainRows = func(ctx context.Context, conn *sql.Conn, eventID string, sequence int64) error {
		return execAgentOne(ctx, conn, "record M2 food consumption", `INSERT INTO m2_consumption_outcomes(scheduler_item_id, purchase_item_id, day, status, reason_code, quantity_minor, event_id, event_sequence) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, item.SchedulerItemID, purchaseID, payload.Day, status, reason, quantity, eventID, sequence)
	}
	return mutation, nil
}

func prepareM2Restock(ctx context.Context, conn *sql.Conn, item SchedulerItem, payload scheduledPayload) (scheduledMutation, error) {
	if item.PhaseID != m2RestockPhase || payload.Kind != "m2_store_restock" || payload.SubjectID != m2SupplierQuoteID || (payload.Day != 26 && payload.Day != 27) || item.WorldTime != m2WageTime(payload.Day, 7, 7) || item.SchedulerItemID != fmt.Sprintf("sched_m2_store_restock_day_%d", payload.Day) {
		return scheduledMutation{}, core.NewError(core.CodeStorageFailure, "invalid M2 finite restock task")
	}
	var supplierCash, supplierLocation, storeCash, storeLocation, skuID, currency, effectiveFrom string
	var price, capacity int64
	err := conn.QueryRowContext(ctx, `SELECT supplier.cash_account_id, supplier.stock_location_id, store.cash_account_id, store.stock_location_id, q.sku_id, q.currency_id, q.effective_from, q.unit_price_minor, q.max_quantity_minor
		FROM m2_supplier_quotes q JOIN m2_economic_actors supplier ON supplier.actor_id = q.supplier_actor_id JOIN m2_economic_actors store ON store.actor_id = q.store_actor_id
		JOIN stock_locations source ON source.location_id = supplier.stock_location_id AND source.owner_id = supplier.actor_id AND source.location_kind = 'holder'
		JOIN stock_locations destination ON destination.location_id = store.stock_location_id AND destination.owner_id = store.actor_id AND destination.location_kind = 'holder'
		WHERE q.quote_id = ? AND supplier.kind = 'supplier' AND store.kind = 'store' AND supplier.instance_id = ? AND supplier.branch_id = ? AND store.instance_id = supplier.instance_id AND store.branch_id = supplier.branch_id AND q.effective_from <= ? AND (q.effective_until IS NULL OR ? < q.effective_until)`, m2SupplierQuoteID, M2DemoInstanceID, M2DemoBranchID, item.WorldTime, item.WorldTime).Scan(&supplierCash, &supplierLocation, &storeCash, &storeLocation, &skuID, &currency, &effectiveFrom, &price, &capacity)
	if err != nil {
		return scheduledMutation{}, classifyMissing(err, "M2 finite supplier quote")
	}
	if supplierCash != m2SupplierCash || supplierLocation != m2SupplierLocation || storeCash != m2StoreCash || storeLocation != m2StoreLocation || skuID != M2DemoSKUID || currency != M2DemoCurrencyID || effectiveFrom != m2EconomyPeriodStart || price != 3 || capacity != 10 {
		return scheduledMutation{}, core.NewError(core.CodeProjectionDiverged, "M2 supplier quote or ownership differs from definition")
	}
	for _, accountID := range []string{storeCash, supplierCash} {
		if err := verifyM2AccountProjection(ctx, conn, accountID, currency); err != nil {
			return scheduledMutation{}, err
		}
	}
	for _, locationID := range []string{storeLocation, supplierLocation} {
		if err := verifyM2InventoryProjection(ctx, conn, locationID, skuID); err != nil {
			return scheduledMutation{}, err
		}
	}
	storeBalance, storeVersion, err := readScheduledBalance(ctx, conn, storeCash)
	if err != nil {
		return scheduledMutation{}, err
	}
	supplierBalance, supplierVersion, err := readScheduledBalance(ctx, conn, supplierCash)
	if err != nil {
		return scheduledMutation{}, err
	}
	storeStock, storeStockVersion, err := readScheduledInventory(ctx, conn, storeLocation, skuID)
	if err != nil {
		return scheduledMutation{}, err
	}
	supplierStock, supplierStockVersion, err := readScheduledInventory(ctx, conn, supplierLocation, skuID)
	if err != nil {
		return scheduledMutation{}, err
	}
	cost, ok := checkedMultiplyPositive(price, capacity)
	if !ok {
		return scheduledMutation{}, core.NewError(core.CodeIntegerOverflow, "M2 supplier quote exceeds integer range")
	}
	status, reason, quantity, spent := "restocked", "none", capacity, cost
	if storeBalance < cost {
		status, reason, quantity, spent = "rejected", "insufficient_store_funds", 0, 0
	} else if supplierStock < capacity {
		status, reason, quantity, spent = "rejected", "insufficient_supplier_stock", 0, 0
	}
	mutation := scheduledMutation{EventType: "M2StoreRestockRejected", EventPayload: struct {
		QuoteID       string `json:"quote_id"`
		Day           int    `json:"day"`
		SKUID         string `json:"sku_id"`
		Status        string `json:"status"`
		Reason        string `json:"reason_code"`
		UnitPrice     int64  `json:"unit_price_minor"`
		Capacity      int64  `json:"max_quantity_minor"`
		StoreBalance  int64  `json:"store_balance_minor"`
		SupplierStock int64  `json:"supplier_stock_minor"`
		Quantity      int64  `json:"quantity_minor"`
		Spent         int64  `json:"spent_minor"`
	}{m2SupplierQuoteID, payload.Day, skuID, status, reason, price, capacity, storeBalance, supplierStock, quantity, spent}}
	if status == "restocked" {
		newSupplierBalance, ok := checkedAdd(supplierBalance, cost)
		if !ok {
			return scheduledMutation{}, core.NewError(core.CodeIntegerOverflow, "M2 supplier credit overflows")
		}
		newStoreStock, ok := checkedAdd(storeStock, capacity)
		if !ok {
			return scheduledMutation{}, core.NewError(core.CodeIntegerOverflow, "M2 store restock overflows")
		}
		mutation.EventType = "M2StoreRestocked"
		mutation.Postings = []scheduledPosting{{storeCash, currency, -cost, "M2 finite supplier payment"}, {supplierCash, currency, cost, "M2 finite supplier revenue"}}
		mutation.Balances = []balanceMutation{{storeCash, storeVersion, storeBalance - cost}, {supplierCash, supplierVersion, newSupplierBalance}}
		mutation.Movements = []scheduledMovement{{skuID, supplierLocation, storeLocation, capacity, "transfer", "m2_paid_supplier_restock", nil}}
		mutation.Inventory = []inventoryMutation{{storeLocation, skuID, storeStockVersion, newStoreStock}, {supplierLocation, skuID, supplierStockVersion, supplierStock - capacity}}
	}
	mutation.ApplyDomainRows = func(ctx context.Context, conn *sql.Conn, eventID string, sequence int64) error {
		return execAgentOne(ctx, conn, "record M2 finite restock", `INSERT INTO m2_restock_outcomes(scheduler_item_id, quote_id, day, status, reason_code, quantity_minor, spent_minor, event_id, event_sequence) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, item.SchedulerItemID, m2SupplierQuoteID, payload.Day, status, reason, quantity, spent, eventID, sequence)
	}
	return mutation, nil
}
