package httpapi

import (
	"context"
	"net/http"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/storage"
)

func TestRPObjectStockHTTPRequiresScopedCreatorAndExactKey(t *testing.T) {
	ctx := context.Background()
	s, handler := openHTTPTestServer(t, ctx, filepath.Join(t.TempDir(), "object-stock-http.db"))
	defer s.Close()
	setup, err := s.PrepareRPTravel(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.ConfigureStudioAccessLocal(ctx, storage.StudioAccessRequest{Purpose: "create_world", Binding: core.CareerBinding{PrincipalID: "principal_operator", InstanceID: storage.M2DemoInstanceID, BranchID: storage.M2DemoBranchID, ExpectedHead: setup.EventSequence, IdempotencyKey: "stock-http-grant"}, TargetPrincipalID: "principal_creator", Status: "active"})
	if err != nil {
		t.Fatal(err)
	}
	const world = "object-stock-http"
	created, err := s.CreateStudioWorld(ctx, storage.StudioCreateRequest{PrincipalID: "principal_creator", AuthorityInstanceID: storage.M2DemoInstanceID, AuthorityBranchID: storage.M2DemoBranchID, InstanceID: world, IdempotencyKey: "stock-http-world", PlayerPrincipalID: storage.M2RPPlayerPrincipal, SystemPackage: httpStudioPackage("system"), NarrativePackage: httpStudioPackage("narrative"), Spec: core.StudioWorldSpec{Version: core.StudioWorldSpecVersion, Name: "物件来源", StartWorldTime: core.StudioWorldStart, Population: 2, OpeningMoneyMinor: 2, OpeningStockMinor: 2, Places: []core.StudioWorldPlace{{Key: "home", Name: "家", Kind: "home"}, {Key: "square", Name: "广场", Kind: "public"}}, Links: []core.StudioWorldLink{{From: "home", To: "square", Minutes: 5}}, People: []core.StudioWorldPerson{{Key: "lin", Name: "Lin", Place: "home", Player: true}, {Key: "cai", Name: "Cai", Place: "home"}}}})
	if err != nil {
		t.Fatal(err)
	}
	owner := created.EntityID
	request := storage.RPObjectStockRequest{Binding: core.CareerBinding{InstanceID: world, BranchID: "br_main", ExpectedHead: created.EventSequence, IdempotencyKey: "stock-http-cup"}, SKUCode: "cup", DisplayName: "杯子", OwnerEntityID: owner}
	const route = "/api/v1/rp/objects/stock/define"
	assertStatus(t, performJSON(t, handler, route, "", request), http.StatusUnauthorized)
	assertStatus(t, performJSON(t, handler, route, rpPlayerToken, request), http.StatusForbidden)
	assertStatus(t, performJSON(t, handler, route, operatorToken, request), http.StatusForbidden)
	assertStatus(t, performJSON(t, handler, route, creatorToken, map[string]any{"binding": request.Binding, "sku_code": "cup", "display_name": "杯子", "owner_entity_id": owner, "quantity_minor": 100}), http.StatusBadRequest)
	assertStatus(t, performRequest(t, handler, http.MethodGet, route, creatorToken, nil), http.StatusMethodNotAllowed)
	response := performJSON(t, handler, route, creatorToken, request)
	assertStatus(t, response, http.StatusOK)
	stock := decodeData[storage.RPObjectStockRecord](t, response)
	if stock.Replayed || stock.Fact.SKUDisplayName != "杯子" || stock.Fact.QuantityMinor != 1 || stock.Fact.OwnerEntityID != owner {
		t.Fatalf("creator inventory Event not authoritative: %+v", stock)
	}
	response = performJSON(t, handler, route, creatorToken, request)
	assertStatus(t, response, http.StatusOK)
	if retry := decodeData[storage.RPObjectStockRecord](t, response); !retry.Replayed || retry.EventID != stock.EventID {
		t.Fatalf("exact creator retry did not replay: %+v", retry)
	}
	changed := request
	changed.DisplayName = "碗"
	assertAPIError(t, performJSON(t, handler, route, creatorToken, changed), http.StatusConflict, core.CodeIdempotencyMismatch)
	if differences, err := s.CompareProjections(ctx, world, "br_main"); err != nil || len(differences) != 0 {
		t.Fatalf("creator HTTP stock drift: %+v %v", differences, err)
	}
}
