package storage

import (
	"context"
	"database/sql"
	"fmt"

	"corerp.local/backend/internal/core"
)

// Verify the finite-stock decision against movement facts, not a possibly
// corrupted projection that could otherwise create a false stock-out or sale.
func verifyM2InventoryProjection(ctx context.Context, conn *sql.Conn, locationID, skuID string) error {
	var projected, authoritative int64
	if err := conn.QueryRowContext(ctx, `SELECT quantity_minor FROM inventory_balances WHERE location_id = ? AND sku_id = ?`, locationID, skuID).Scan(&projected); err != nil {
		return classifyMissing(err, "M2 store inventory")
	}
	err := conn.QueryRowContext(ctx, `SELECT COALESCE(SUM(delta), 0) FROM (
		SELECT m.quantity_minor AS delta FROM stock_movements m JOIN events e ON e.event_id = m.event_id WHERE e.instance_id = ? AND e.branch_id = ? AND m.sku_id = ? AND m.to_location_id = ? AND m.movement_kind IN ('transfer', 'create')
		UNION ALL
		SELECT -m.quantity_minor AS delta FROM stock_movements m JOIN events e ON e.event_id = m.event_id WHERE e.instance_id = ? AND e.branch_id = ? AND m.sku_id = ? AND m.from_location_id = ? AND m.movement_kind IN ('transfer', 'consume', 'destroy')
	)`, M2DemoInstanceID, M2DemoBranchID, skuID, locationID, M2DemoInstanceID, M2DemoBranchID, skuID, locationID).Scan(&authoritative)
	if err != nil {
		return core.WrapError(core.CodeStorageFailure, "replay M2 store inventory", err)
	}
	if projected != authoritative {
		return core.NewError(core.CodeProjectionDiverged, "M2 inventory differs from stock facts: "+locationID)
	}
	return nil
}

func prepareM2Purchase(ctx context.Context, conn *sql.Conn, item SchedulerItem, payload scheduledPayload) (scheduledMutation, error) {
	validTime := item.WorldTime == m2WageTime(payload.Day, 7, 4) && item.SchedulerItemID == fmt.Sprintf("sched_m2_food_buy_day_%d", payload.Day)
	if payload.Day == 1 && item.WorldTime == m2WageTime(1, 7, 5) && item.SchedulerItemID == "sched_m2_food_buy_budget_probe_day_1" {
		validTime = true
	}
	if item.PhaseID != m2StorePhaseBuy || payload.Kind != "m2_food_buy" || payload.SubjectID != m2StoreOfferID || payload.Day < 1 || payload.Day > 30 || !validTime {
		return scheduledMutation{}, core.NewError(core.CodeStorageFailure, "invalid M2 food purchase task")
	}
	var storeCash, storeLocation, skuID, currency, householdLocation, cohortAccount, effectiveFrom string
	var price, budget int64
	err := conn.QueryRowContext(ctx, `SELECT a.cash_account_id, a.stock_location_id, o.sku_id, o.currency_id, o.household_location_id, h.asset_account_id, o.effective_from, o.unit_price_minor, o.daily_budget_minor
		FROM m2_store_offers o JOIN m2_economic_actors a ON a.actor_id = o.actor_id JOIN cohorts h ON h.cohort_id = o.cohort_id
		JOIN stock_locations seller ON seller.location_id = a.stock_location_id AND seller.owner_id = a.actor_id AND seller.location_kind = 'holder'
		JOIN stock_locations buyer ON buyer.location_id = o.household_location_id AND buyer.owner_id = h.cohort_id AND buyer.location_kind = 'holder'
		WHERE o.offer_id = ? AND a.kind = 'store' AND a.instance_id = ? AND a.branch_id = ? AND h.instance_id = a.instance_id AND h.branch_id = a.branch_id AND h.status = 'active' AND h.population_count > 0 AND h.currency_id = o.currency_id AND h.sku_id = o.sku_id AND o.effective_from <= ? AND (o.effective_until IS NULL OR ? < o.effective_until)`, m2StoreOfferID, M2DemoInstanceID, M2DemoBranchID, item.WorldTime, item.WorldTime).Scan(&storeCash, &storeLocation, &skuID, &currency, &householdLocation, &cohortAccount, &effectiveFrom, &price, &budget)
	if err != nil {
		return scheduledMutation{}, classifyMissing(err, "M2 store offer")
	}
	if storeCash != m2StoreCash || storeLocation != m2StoreLocation || cohortAccount != M2DemoCohortAssetAccountID || householdLocation != m2HouseholdLocation || skuID != M2DemoSKUID || currency != M2DemoCurrencyID || price != 5 || budget != 5 || effectiveFrom != m2EconomyPeriodStart {
		return scheduledMutation{}, core.NewError(core.CodeProjectionDiverged, "M2 store offer ownership or amount differs from declared fixture")
	}
	for _, accountID := range []string{cohortAccount, storeCash} {
		if err := verifyM2AccountProjection(ctx, conn, accountID, currency); err != nil {
			return scheduledMutation{}, err
		}
	}
	for _, locationID := range []string{storeLocation, householdLocation} {
		if err := verifyM2InventoryProjection(ctx, conn, locationID, skuID); err != nil {
			return scheduledMutation{}, err
		}
	}
	var spent int64
	if err := conn.QueryRowContext(ctx, `SELECT COALESCE(SUM(spent_minor), 0) FROM m2_purchase_outcomes WHERE offer_id = ? AND day = ? AND status = 'purchased'`, m2StoreOfferID, payload.Day).Scan(&spent); err != nil {
		return scheduledMutation{}, core.WrapError(core.CodeStorageFailure, "read M2 daily food budget", err)
	}
	nextSpent, ok := checkedAdd(spent, price)
	if !ok {
		return scheduledMutation{}, core.NewError(core.CodeIntegerOverflow, "M2 daily purchase budget overflows")
	}
	buyerBalance, buyerVersion, err := readScheduledBalance(ctx, conn, cohortAccount)
	if err != nil {
		return scheduledMutation{}, err
	}
	sellerBalance, sellerVersion, err := readScheduledBalance(ctx, conn, storeCash)
	if err != nil {
		return scheduledMutation{}, err
	}
	sellerStock, sellerStockVersion, err := readScheduledInventory(ctx, conn, storeLocation, skuID)
	if err != nil {
		return scheduledMutation{}, err
	}
	buyerStock, buyerStockVersion, err := readScheduledInventory(ctx, conn, householdLocation, skuID)
	if err != nil {
		return scheduledMutation{}, err
	}
	status, reason := "purchased", "none"
	if nextSpent > budget {
		status, reason = "rejected", "budget_exceeded"
	} else if buyerBalance < price {
		status, reason = "rejected", "insufficient_funds"
	} else if sellerStock < 1 {
		status, reason = "rejected", "insufficient_stock"
	}
	quantity, paid := int64(0), int64(0)
	if status == "purchased" {
		quantity, paid = 1, price
	}
	mutation := scheduledMutation{EventType: "M2FoodPurchaseRejected", EventPayload: struct {
		OfferID      string `json:"offer_id"`
		Day          int    `json:"day"`
		SKUID        string `json:"sku_id"`
		Price        int64  `json:"unit_price_minor"`
		Status       string `json:"status"`
		Reason       string `json:"reason_code"`
		BuyerBalance int64  `json:"buyer_balance_minor"`
		SellerStock  int64  `json:"seller_stock_minor"`
		DailySpent   int64  `json:"daily_spent_minor"`
		DailyBudget  int64  `json:"daily_budget_minor"`
	}{m2StoreOfferID, payload.Day, skuID, price, status, reason, buyerBalance, sellerStock, spent, budget}}
	if status == "purchased" {
		newSellerBalance, ok := checkedAdd(sellerBalance, price)
		if !ok {
			return scheduledMutation{}, core.NewError(core.CodeIntegerOverflow, "M2 store credit overflows")
		}
		newBuyerStock, ok := checkedAdd(buyerStock, 1)
		if !ok {
			return scheduledMutation{}, core.NewError(core.CodeIntegerOverflow, "M2 household stock overflows")
		}
		mutation.EventType = "M2FoodPurchased"
		mutation.Postings = []scheduledPosting{{cohortAccount, currency, -price, "M2 household food purchase"}, {storeCash, currency, price, "M2 store sale"}}
		mutation.Movements = []scheduledMovement{{skuID, storeLocation, householdLocation, 1, "transfer", "m2_household_purchase", nil}}
		mutation.Balances = []balanceMutation{{cohortAccount, buyerVersion, buyerBalance - price}, {storeCash, sellerVersion, newSellerBalance}}
		mutation.Inventory = []inventoryMutation{{storeLocation, skuID, sellerStockVersion, sellerStock - 1}, {householdLocation, skuID, buyerStockVersion, newBuyerStock}}
	}
	mutation.ApplyDomainRows = func(ctx context.Context, conn *sql.Conn, eventID string, sequence int64) error {
		return execAgentOne(ctx, conn, "record M2 food purchase outcome", `INSERT INTO m2_purchase_outcomes(scheduler_item_id, offer_id, day, status, reason_code, quantity_minor, spent_minor, event_id, event_sequence) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, item.SchedulerItemID, m2StoreOfferID, payload.Day, status, reason, quantity, paid, eventID, sequence)
	}
	return mutation, nil
}
