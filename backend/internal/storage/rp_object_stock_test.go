package storage

import (
	"context"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"corerp.local/backend/internal/core"
)

func studioCupWorld(t *testing.T, ctx context.Context, s *Store) (string, string, string, func(string) core.CareerBinding) {
	t.Helper()
	setup, err := s.PrepareRPTravel(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.ConfigureStudioAccessLocal(ctx, StudioAccessRequest{Purpose: "create_world", Binding: core.CareerBinding{PrincipalID: "principal_operator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: setup.EventSequence, IdempotencyKey: "cup-world-grant"}, TargetPrincipalID: "principal_creator", Status: "active"})
	if err != nil {
		t.Fatal(err)
	}
	world := "rp-cup-world"
	if _, err := s.CreateStudioWorld(ctx, studioCreateFixture(world)); err != nil {
		t.Fatal(err)
	}
	player, err := core.StudioWorldObjectID(world, "entity", "lin")
	if err != nil {
		t.Fatal(err)
	}
	place, err := core.StudioWorldObjectID(world, "place", "home")
	if err != nil {
		t.Fatal(err)
	}
	binding := func(key string) core.CareerBinding {
		t.Helper()
		var head int64
		if err := s.db.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id='br_main'`, world).Scan(&head); err != nil {
			t.Fatal(err)
		}
		return core.CareerBinding{PrincipalID: "principal_creator", InstanceID: world, BranchID: "br_main", ExpectedHead: head, IdempotencyKey: key}
	}
	return world, player, place, binding
}

func TestRPObjectStockCreatorSourcesRealCupAndModelMovesItWithoutTransfer(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "cup-world.db")
	s := openBootstrappedStore(t, ctx, path)
	world, playerID, placeID, binding := studioCupWorld(t, ctx, s)
	stockRequest := RPObjectStockRequest{Binding: binding("real-cup-stock"), SKUCode: "cup", DisplayName: "杯子", OwnerEntityID: playerID}
	if _, err := s.DefineRPObjectStock(ctx, RPObjectStockRequest{Binding: core.CareerBinding{PrincipalID: "principal_operator", InstanceID: world, BranchID: "br_main", ExpectedHead: stockRequest.Binding.ExpectedHead, IdempotencyKey: "forge-cup"}, SKUCode: "forged-cup", DisplayName: "杯子", OwnerEntityID: playerID}); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("noncreator minted inventory: %v", err)
	}
	stock, err := s.DefineRPObjectStock(ctx, stockRequest)
	if err != nil || stock.Fact.SKUID == "" || stock.Fact.SKUDisplayName != "杯子" || stock.Fact.QuantityMinor != 1 {
		t.Fatalf("real cup SKU and stock: %+v %v", stock, err)
	}
	if duplicate, err := s.DefineRPObjectStock(ctx, stockRequest); err != nil || !duplicate.Replayed || duplicate.EventID != stock.EventID {
		t.Fatalf("creator exact receipt changed: %+v %v", duplicate, err)
	}
	changed := stockRequest
	changed.DisplayName = "碗"
	if _, err := s.DefineRPObjectStock(ctx, changed); !core.HasCode(err, core.CodeIdempotencyMismatch) {
		t.Fatalf("changed SKU receipt accepted: %v", err)
	}
	if _, err := s.DefineRPObjectStock(ctx, RPObjectStockRequest{Binding: binding("duplicate-cup"), SKUCode: "cup", DisplayName: "杯子", OwnerEntityID: playerID}); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatalf("a second command created the same SKU twice: %v", err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM product_skus k JOIN events e ON e.event_id=k.definition_event_id WHERE k.sku_id=? AND k.base_unit='unit' AND k.quantity_scale=0 AND e.instance_id=? AND e.branch_id='br_main' AND json_extract(e.payload,'$.sku_display_name')='杯子'`, []any{stock.Fact.SKUID, world}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM stock_movements WHERE event_id=? AND sku_id=? AND movement_kind='create' AND quantity_minor=1 AND to_location_id=?`, []any{stock.EventID, stock.Fact.SKUID, stock.Fact.InventoryLocationID}, 1)
	anchor := func(key, code string) RPObjectAnchorRecord {
		t.Helper()
		record, err := s.DefineRPObjectAnchor(ctx, RPObjectAnchorRequest{Binding: binding(key), PlaceID: placeID, ZoneKey: "main", AnchorCode: code, DisplayName: code})
		if err != nil {
			t.Fatal(err)
		}
		return record
	}
	from, to := anchor("cup-table", "table-side"), anchor("cup-nearby", "table-nearby")
	source, err := s.DefineRPObjectSource(ctx, RPObjectSourceRequest{Binding: binding("cup-source"), AnchorID: from.Fact.AnchorID, OwnerEntityID: playerID, SKUID: stock.Fact.SKUID, DisplayName: "杯子"})
	if err != nil || source.Fact.SKUEventID != stock.EventID || source.Fact.StockEventID != stock.EventID {
		t.Fatalf("cup source has no real authored stock: %+v %v", source, err)
	}
	opened, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: M2RPPlayerPrincipal, InstanceID: world, BranchID: "br_main", EntityID: playerID, POV: "second_person", IdempotencyKey: "cup-session"})
	if err != nil {
		t.Fatal(err)
	}
	read := core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: opened.SessionID}
	request := func(action, key string) core.RPObjectRequest {
		t.Helper()
		view, err := s.ObserveRPSession(ctx, read)
		if err != nil {
			t.Fatal(err)
		}
		return core.RPObjectRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, IdempotencyKey: key, Action: action}
	}
	stage := request("stage", "cup-stage")
	stage.SourceID = source.Fact.SourceID
	item, err := s.ObjectRP(ctx, stage)
	if err != nil {
		t.Fatal(err)
	}
	place := request("place", "cup-place")
	place.ObjectID, place.AnchorID = item.ObjectID, from.Fact.AnchorID
	if _, err := s.ObjectRP(ctx, place); err != nil {
		t.Fatal(err)
	}
	view, err := s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	model := semanticFixtureServer(t, func(text string) string {
		if !strings.Contains(text, "杯子") {
			t.Errorf("cup intent missing: %q", text)
		}
		return semanticModelReply("MIXED", []core.RPInteractionStep{{Kind: "object", ObjectAction: "move", ObjectID: item.ObjectID, AnchorID: to.Fact.AnchorID}, {Kind: "speech", SpeechText: "喝一点吧。"}}, "")
	}, &calls)
	defer model.Close()
	service, err := NewRPService(s, semanticFixtureProvider(t, model), "chat_completions")
	if err != nil {
		t.Fatal(err)
	}
	input := core.RPInteractionRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, Mode: "AUTO", Text: "我把桌上的杯子推到近旁，然后说「喝一点吧。」", ExpectedCursor: view.ObservationCursor, IdempotencyKey: "cup-move-and-speech"}
	result, err := service.RunRPInteraction(ctx, input)
	if err != nil || result.Status != "settled" || len(result.Outcomes) != 2 || result.Outcomes[0].Kind != "object" || result.Outcomes[1].Kind != "speech" || calls.Load() != 1 {
		t.Fatalf("sourced cup D slice: %+v %v model calls=%d", result, err, calls.Load())
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_objects WHERE object_id=? AND sku_id=? AND display_name='杯子' AND physical_state='placed' AND anchor_id=? AND owner_actor_id=?`, []any{item.ObjectID, stock.Fact.SKUID, to.Fact.AnchorID, playerID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_object_offers WHERE object_id=?`, []any{item.ObjectID}, 0)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_utterances WHERE speaker_entity_id=? AND speech_text='喝一点吧。'`, []any{playerID}, 1)
	if differences, err := s.CompareProjections(ctx, world, "br_main"); err != nil || len(differences) != 0 {
		t.Fatalf("real cup read/replay drift: %+v %v", differences, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if differences, err := s.CompareProjections(ctx, world, "br_main"); err != nil || len(differences) != 0 {
		t.Fatalf("restarted cup world drift: %+v %v", differences, err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE product_skus SET base_unit='fiction' WHERE sku_id=?`, stock.Fact.SKUID); err != nil {
		t.Fatal(err)
	}
	if differences, err := s.CompareProjections(ctx, world, "br_main"); err != nil || len(differences) == 0 {
		t.Fatalf("corrupted cup catalog went unnoticed: %+v %v", differences, err)
	}
	if err := s.RebuildProjections(ctx, world, "br_main"); err != nil {
		t.Fatal(err)
	}
	if differences, err := s.CompareProjections(ctx, world, "br_main"); err != nil || len(differences) != 0 {
		t.Fatalf("cup catalog did not rebuild from Event: %+v %v", differences, err)
	}
}

func TestRPObjectStockCannotMintOnRollbackOrCrossWorldAndRejectsBrokenLedger(t *testing.T) {
	ctx := context.Background()
	s := openBootstrappedStore(t, ctx, filepath.Join(t.TempDir(), "cup-stock-rollback.db"))
	defer s.Close()
	world, player, _, binding := studioCupWorld(t, ctx, s)
	request := RPObjectStockRequest{Binding: binding("rollback-cup"), SKUCode: "cup", DisplayName: "杯子", OwnerEntityID: player}
	foreign := request
	foreign.SKUCode = "foreign-cup"
	foreign.Binding = binding("foreign-cup")
	foreign.OwnerEntityID = M2RPPlayerID
	if _, err := s.DefineRPObjectStock(ctx, foreign); !core.HasCode(err, core.CodeNotFound) {
		t.Fatalf("other-world owner gained new inventory: %v", err)
	}
	s.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "rollback stock creation") }
	if _, err := s.DefineRPObjectStock(ctx, request); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatalf("injected rollback did not stop stock: %v", err)
	}
	s.beforeCommit = nil
	sku, err := core.StudioWorldObjectID(world, "sku", "cup")
	if err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM product_skus WHERE sku_id=?`, []any{sku}, 0)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE instance_id=? AND event_type='RPObjectStockDefined'`, []any{world}, 0)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM stock_movements WHERE sku_id=?`, []any{sku}, 0)
	created, err := s.DefineRPObjectStock(ctx, request)
	if err != nil || created.Replayed {
		t.Fatalf("same key could not recover after rollback: %+v %v", created, err)
	}
	assertM2Value(t, ctx, s, `SELECT quantity_minor FROM inventory_balances WHERE location_id=? AND sku_id=?`, []any{created.Fact.InventoryLocationID, sku}, 1)
	if _, err := s.db.ExecContext(ctx, `UPDATE stock_movements SET quantity_minor=2 WHERE event_id=?`, created.EventID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CompareProjections(ctx, world, "br_main"); !core.HasCode(err, core.CodeProjectionDiverged) {
		t.Fatalf("corrupt factual movement passed replay: %v", err)
	}
	if err := s.RebuildProjections(ctx, world, "br_main"); !core.HasCode(err, core.CodeProjectionDiverged) {
		t.Fatalf("rebuild manufactured stock over broken fact: %v", err)
	}
}
