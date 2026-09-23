package core

import (
	"strings"
	"time"
)

const PurchaseCommandType = "PurchaseCommand"

type PurchaseCommand struct {
	CommandID        string `json:"command_id"`
	InstanceID       string `json:"instance_id"`
	BranchID         string `json:"branch_id"`
	ActorID          string `json:"actor_id"`
	PrincipalID      string `json:"principal_id"`
	IdempotencyKey   string `json:"idempotency_key"`
	ExpectedHead     int64  `json:"expected_head"`
	WorldTime        string `json:"world_time"`
	BuyerAccountID   string `json:"buyer_account_id"`
	SellerAccountID  string `json:"seller_account_id"`
	BuyerLocationID  string `json:"buyer_location_id"`
	SellerLocationID string `json:"seller_location_id"`
	SKUID            string `json:"sku_id"`
	CurrencyID       string `json:"currency_id"`
	QuantityMinor    int64  `json:"quantity_minor"`
	UnitPriceMinor   int64  `json:"unit_price_minor"`
}

type PurchaseResult struct {
	CommandID     string `json:"command_id"`
	BatchID       string `json:"batch_id"`
	EventID       string `json:"event_id"`
	FirstSequence int64  `json:"first_sequence"`
	LastSequence  int64  `json:"last_sequence"`
	EventCount    int64  `json:"event_count"`
	RequestHash   string `json:"request_hash"`
	BatchHash     string `json:"batch_hash"`
	Replayed      bool   `json:"replayed"`
}

type requestHashDocument struct {
	CommandType      string `json:"command_type"`
	InstanceID       string `json:"instance_id"`
	BranchID         string `json:"branch_id"`
	ActorID          string `json:"actor_id"`
	PrincipalID      string `json:"principal_id"`
	ExpectedHead     int64  `json:"expected_head"`
	WorldTime        string `json:"world_time"`
	BuyerAccountID   string `json:"buyer_account_id"`
	SellerAccountID  string `json:"seller_account_id"`
	BuyerLocationID  string `json:"buyer_location_id"`
	SellerLocationID string `json:"seller_location_id"`
	SKUID            string `json:"sku_id"`
	CurrencyID       string `json:"currency_id"`
	QuantityMinor    int64  `json:"quantity_minor"`
	UnitPriceMinor   int64  `json:"unit_price_minor"`
}

func (c PurchaseCommand) Validate() error {
	required := map[string]string{
		"command_id": c.CommandID, "instance_id": c.InstanceID, "branch_id": c.BranchID,
		"actor_id": c.ActorID, "principal_id": c.PrincipalID, "idempotency_key": c.IdempotencyKey,
		"world_time": c.WorldTime, "buyer_account_id": c.BuyerAccountID,
		"seller_account_id": c.SellerAccountID, "buyer_location_id": c.BuyerLocationID,
		"seller_location_id": c.SellerLocationID, "sku_id": c.SKUID, "currency_id": c.CurrencyID,
	}
	for name, value := range required {
		if strings.TrimSpace(value) == "" {
			return NewError(CodeInvalidArgument, name+" is required")
		}
	}
	if c.ExpectedHead < 0 {
		return NewError(CodeInvalidArgument, "expected_head must be nonnegative")
	}
	if c.ExpectedHead >= MaxJSONSafeInteger {
		return NewError(CodeIntegerOverflow, "expected_head leaves no interoperable sequence value for a new event")
	}
	if c.QuantityMinor <= 0 || c.UnitPriceMinor <= 0 {
		return NewError(CodeInvalidArgument, "quantity_minor and unit_price_minor must be positive")
	}
	if c.QuantityMinor > MaxJSONSafeInteger || c.UnitPriceMinor > MaxJSONSafeInteger {
		return NewError(CodeIntegerOverflow, "amount or quantity exceeds the interoperable JSON integer range")
	}
	if c.BuyerAccountID == c.SellerAccountID {
		return NewError(CodeInvalidArgument, "buyer and seller accounts must differ")
	}
	if c.BuyerLocationID == c.SellerLocationID {
		return NewError(CodeInvalidArgument, "buyer and seller locations must differ")
	}
	if _, err := time.Parse(time.RFC3339, c.WorldTime); err != nil {
		return WrapError(CodeInvalidArgument, "world_time must be RFC 3339", err)
	}
	return nil
}

func RequestHash(c PurchaseCommand) (string, error) {
	doc := requestHashDocument{
		CommandType: PurchaseCommandType, InstanceID: c.InstanceID, BranchID: c.BranchID,
		ActorID: c.ActorID, PrincipalID: c.PrincipalID, ExpectedHead: c.ExpectedHead,
		WorldTime: c.WorldTime, BuyerAccountID: c.BuyerAccountID,
		SellerAccountID: c.SellerAccountID, BuyerLocationID: c.BuyerLocationID,
		SellerLocationID: c.SellerLocationID, SKUID: c.SKUID, CurrencyID: c.CurrencyID,
		QuantityMinor: c.QuantityMinor, UnitPriceMinor: c.UnitPriceMinor,
	}
	return HashJSON(doc)
}
