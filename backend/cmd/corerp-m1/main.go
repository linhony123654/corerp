package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/storage"
)

func main() {
	databasePath := flag.String("db", "corerp-m1.db", "SQLite database path")
	action := flag.String("action", "purchase", "action: purchase, simulate90, or inspect")
	flag.Parse()

	if err := run(context.Background(), *databasePath, *action); err != nil {
		encoded, _ := json.Marshal(struct {
			Error string `json:"error"`
		}{Error: err.Error()})
		fmt.Fprintln(os.Stderr, string(encoded))
		os.Exit(1)
	}
}

func run(ctx context.Context, databasePath, action string) error {
	store, err := storage.Open(ctx, databasePath)
	if err != nil {
		return err
	}
	defer store.Close()
	if err := store.BootstrapDemo(ctx); err != nil {
		return err
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	switch action {
	case "purchase":
		result, err := store.Purchase(ctx, core.PurchaseCommand{
			CommandID:        "cmd_demo_purchase_001",
			InstanceID:       storage.DemoInstanceID,
			BranchID:         storage.DemoBranchID,
			ActorID:          "buyer",
			PrincipalID:      "principal_buyer",
			IdempotencyKey:   "demo-purchase-001",
			ExpectedHead:     1,
			WorldTime:        "2026-09-22T09:00:00Z",
			BuyerAccountID:   storage.DemoBuyerAccountID,
			SellerAccountID:  storage.DemoSellerAccountID,
			BuyerLocationID:  storage.DemoBuyerLocationID,
			SellerLocationID: storage.DemoSellerLocationID,
			SKUID:            storage.DemoSKUID,
			CurrencyID:       storage.DemoCurrencyID,
			QuantityMinor:    2,
			UnitPriceMinor:   25,
		})
		if err != nil {
			return err
		}
		state, err := store.ReadDemoState(ctx)
		if err != nil {
			return err
		}
		return encoder.Encode(struct {
			Result core.PurchaseResult `json:"result"`
			State  storage.State       `json:"state"`
		}{Result: result, State: state})
	case "inspect":
		state, err := store.ReadDemoState(ctx)
		if err != nil {
			return err
		}
		return encoder.Encode(state)
	case "simulate90":
		result, err := store.RunStrictWorld(ctx, 90, 1000)
		if err != nil {
			return err
		}
		state, err := store.ReadDemoState(ctx)
		if err != nil {
			return err
		}
		return encoder.Encode(struct {
			Result storage.StrictRunResult `json:"result"`
			State  storage.State           `json:"state"`
		}{Result: result, State: state})
	default:
		return core.NewError(core.CodeInvalidArgument, "action must be purchase, simulate90, or inspect")
	}
}
