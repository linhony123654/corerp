package storage

import (
	"context"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPObjectStowAfterConsentCreditsNewOwnersFirstOrdinarySKU(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "received-cup.db")
	s := openBootstrappedStore(t, ctx, path)
	world, playerID, placeID, binding := studioCupWorld(t, ctx, s)
	neighborID, err := core.StudioWorldObjectID(world, "entity", "cai")
	if err != nil {
		t.Fatal(err)
	}
	stock, err := s.DefineRPObjectStock(ctx, RPObjectStockRequest{Binding: binding("stock-received-cup"), SKUCode: "cup", DisplayName: "杯子", OwnerEntityID: playerID})
	if err != nil {
		t.Fatal(err)
	}
	anchor, err := s.DefineRPObjectAnchor(ctx, RPObjectAnchorRequest{Binding: binding("anchor-received-cup"), PlaceID: placeID, ZoneKey: "main", AnchorCode: "cup-side", DisplayName: "杯子旁"})
	if err != nil {
		t.Fatal(err)
	}
	source, err := s.DefineRPObjectSource(ctx, RPObjectSourceRequest{Binding: binding("source-received-cup"), AnchorID: anchor.Fact.AnchorID, OwnerEntityID: playerID, SKUID: stock.Fact.SKUID, DisplayName: "杯子"})
	if err != nil {
		t.Fatal(err)
	}
	opened, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: M2RPPlayerPrincipal, InstanceID: world, BranchID: "br_main", EntityID: playerID, POV: "second_person", IdempotencyKey: "received-cup-player"})
	if err != nil {
		t.Fatal(err)
	}
	player := core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: opened.SessionID}
	const recipientPrincipal = "principal_received_cup_service"
	if _, err := s.db.ExecContext(ctx, `INSERT INTO principals(principal_id,principal_type,display_name,status) VALUES (?,'service','Cup recipient','active')`, recipientPrincipal); err != nil {
		t.Fatal(err)
	}
	operatorBinding := func(key string) core.CareerBinding {
		b := binding(key)
		b.PrincipalID = "principal_operator"
		return b
	}
	if _, err := s.EnrollRPExternalControllerLocal(ctx, RPExternalControllerEnrollmentRequest{Binding: operatorBinding("enroll-cup-recipient"), EntityID: neighborID, ControllerPrincipalID: recipientPrincipal, ControllerInstanceID: "controller-cup-recipient"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AssignRPExternalControllerLocal(ctx, RPExternalControllerAssignmentRequest{Binding: operatorBinding("assign-cup-recipient"), EntityID: neighborID, ExpectedGeneration: 0}); err != nil {
		t.Fatal(err)
	}
	openedNeighbor, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: recipientPrincipal, InstanceID: world, BranchID: "br_main", EntityID: neighborID, POV: "second_person", IdempotencyKey: "received-cup-neighbor"})
	if err != nil {
		t.Fatal(err)
	}
	neighbor := core.RPSessionReadRequest{PrincipalID: recipientPrincipal, SessionID: openedNeighbor.SessionID}
	stage := objectTestRequest(t, ctx, s, player, "stage", "received-cup-stage")
	stage.SourceID = source.Fact.SourceID
	cup, err := s.ObjectRP(ctx, stage)
	if err != nil {
		t.Fatal(err)
	}
	view, err := s.ObserveRPSession(ctx, player)
	if err != nil {
		t.Fatal(err)
	}
	var publicNeighbor string
	for _, entity := range view.PresentEntities {
		if entity.EntityID != playerID {
			publicNeighbor = entity.EntityID
		}
	}
	if publicNeighbor == "" {
		t.Fatal("recipient not visible to the actual player")
	}
	offer := objectTestRequest(t, ctx, s, player, "offer", "received-cup-offer")
	offer.ObjectID, offer.TargetEntityID = cup.ObjectID, publicNeighbor
	offered, err := s.ObjectRP(ctx, offer)
	if err != nil {
		t.Fatal(err)
	}
	accept := objectTestRequest(t, ctx, s, neighbor, "accept", "received-cup-accept")
	accept.OfferID = offered.OfferID
	if _, err := s.ObjectRP(ctx, accept); err != nil {
		t.Fatal(err)
	}
	give := objectTestRequest(t, ctx, s, player, "give", "received-cup-give")
	give.OfferID = offered.OfferID
	if _, err := s.ObjectRP(ctx, give); err != nil {
		t.Fatal(err)
	}
	var neighborInventory string
	if err := s.db.QueryRowContext(ctx, `SELECT inventory_location_id FROM materialized_entities WHERE entity_id=?`, neighborID).Scan(&neighborInventory); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM inventory_balances WHERE location_id=? AND sku_id=?`, []any{neighborInventory, stock.Fact.SKUID}, 0)
	stow := objectTestRequest(t, ctx, s, neighbor, "stow", "recipient-stows-cup")
	stow.ObjectID = cup.ObjectID
	returned, err := s.ObjectRP(ctx, stow)
	if err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT quantity_minor FROM inventory_balances WHERE location_id=? AND sku_id=?`, []any{neighborInventory, stock.Fact.SKUID}, 1)
	assertM2Value(t, ctx, s, `SELECT quantity_minor FROM inventory_balances WHERE location_id=? AND sku_id=?`, []any{stock.Fact.InventoryLocationID, stock.Fact.SKUID}, 0)
	assertM2Value(t, ctx, s, `SELECT quantity_minor FROM inventory_balances WHERE location_id=(SELECT escrow_location_id FROM rp_objects WHERE object_id=?) AND sku_id=?`, []any{cup.ObjectID, stock.Fact.SKUID}, 0)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_objects WHERE object_id=? AND owner_actor_id=? AND physical_state='stowed'`, []any{cup.ObjectID, neighborID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM stock_movements WHERE event_id=? AND to_location_id=? AND reason_code='rp_object_stow'`, []any{returned.EventID, neighborInventory}, 1)
	if differences, err := s.CompareProjections(ctx, world, "br_main"); err != nil || len(differences) != 0 {
		t.Fatalf("recipient's first custom-SKU balance disagrees with replay: %+v %v", differences, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.RebuildProjections(ctx, world, "br_main"); err != nil {
		t.Fatal(err)
	}
	if differences, err := s.CompareProjections(ctx, world, "br_main"); err != nil || len(differences) != 0 {
		t.Fatalf("rebuild lost first recipient balance: %+v %v", differences, err)
	}
}
