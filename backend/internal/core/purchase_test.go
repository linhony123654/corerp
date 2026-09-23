package core

import (
	"math"
	"testing"
)

func TestCanonicalJSONUsesRFC8785PropertyAndStringRules(t *testing.T) {
	value := map[string]any{
		"z":          "<>&\u2028",
		"a":          int64(1),
		"arr":        []any{true, nil, "line\nfeed"},
		"\U0001f600": "supplementary",
		"\ue000":     "bmp-private-use",
	}
	encoded, err := CanonicalJSON(value)
	if err != nil {
		t.Fatal(err)
	}
	expected := "{\"a\":1,\"arr\":[true,null,\"line\\nfeed\"],\"z\":\"<>&\u2028\",\"😀\":\"supplementary\",\"\ue000\":\"bmp-private-use\"}"
	if string(encoded) != expected {
		t.Fatalf("canonical bytes mismatch\nactual:   %s\nexpected: %s", encoded, expected)
	}
	if _, err := CanonicalJSON(math.MaxFloat64); !HasCode(err, CodeInvalidArgument) {
		t.Fatalf("expected floats to be rejected, got %v", err)
	}
	if _, err := CanonicalJSON(MaxJSONSafeInteger + 1); !HasCode(err, CodeInvalidArgument) {
		t.Fatalf("expected unsafe integers to be rejected, got %v", err)
	}
}

func TestRequestHashIgnoresTransportIdentityButPinsPayload(t *testing.T) {
	command := validPurchaseCommand()
	first, err := RequestHash(command)
	if err != nil {
		t.Fatal(err)
	}
	command.CommandID = "cmd_retry"
	command.IdempotencyKey = "same-logical-key"
	second, err := RequestHash(command)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("transport identity changed request hash: %s != %s", first, second)
	}
	const nodeParityHash = "sha256:5c863c637d3d840ba10db8af011a6492f34596ca97d3d01b6fbdd0876d157140"
	if first != nodeParityHash {
		t.Fatalf("request hash does not match independent Node canonicalizer: %s", first)
	}
	command.QuantityMinor++
	changed, err := RequestHash(command)
	if err != nil {
		t.Fatal(err)
	}
	if changed == first {
		t.Fatal("payload change did not change request hash")
	}
}

func TestPurchaseValidation(t *testing.T) {
	command := validPurchaseCommand()
	command.WorldTime = "not-a-time"
	if err := command.Validate(); !HasCode(err, CodeInvalidArgument) {
		t.Fatalf("expected invalid argument, got %v", err)
	}
}

func validPurchaseCommand() PurchaseCommand {
	return PurchaseCommand{
		CommandID: "cmd_1", InstanceID: "inst_m1", BranchID: "br_main",
		ActorID: "buyer", PrincipalID: "principal_buyer", IdempotencyKey: "idem",
		ExpectedHead: 0, WorldTime: "2026-09-22T09:00:00Z",
		BuyerAccountID: "account_buyer", SellerAccountID: "account_seller",
		BuyerLocationID: "location_buyer", SellerLocationID: "location_seller",
		SKUID: "sku_bread", CurrencyID: "credit", QuantityMinor: 2, UnitPriceMinor: 25,
	}
}
