package storage

import (
	"context"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPObjectStowReturnsOneUnitAndPermanentlyRetiresPhysicalObject(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "stow.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	_, player, _ := newRPWaitTestSession(t, ctx, s)
	source, _, _ := objectTestSource(t, ctx, s)
	var ordinary string
	if err := s.db.QueryRowContext(ctx, `SELECT inventory_location_id FROM materialized_entities WHERE entity_id=?`, M2RPPlayerID).Scan(&ordinary); err != nil {
		t.Fatal(err)
	}
	var before int64
	if err := s.db.QueryRowContext(ctx, `SELECT quantity_minor FROM inventory_balances WHERE location_id=? AND sku_id=?`, ordinary, M2DemoSKUID).Scan(&before); err != nil {
		t.Fatal(err)
	}
	stage := objectTestRequest(t, ctx, s, player, "stage", "stow-stage")
	stage.SourceID = source
	item, err := s.ObjectRP(ctx, stage)
	if err != nil {
		t.Fatal(err)
	}
	stow := objectTestRequest(t, ctx, s, player, "stow", "return-held-unit")
	stow.ObjectID = item.ObjectID
	s.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "rollback stow") }
	if _, err := s.ObjectRP(ctx, stow); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatalf("stow escaped rollback: %v", err)
	}
	s.beforeCommit = nil
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM stock_movements WHERE reason_code='rp_object_stow'`, nil, 0)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_objects WHERE object_id=? AND physical_state='held'`, []any{item.ObjectID}, 1)
	if _, err := s.db.ExecContext(ctx, `UPDATE inventory_balances SET quantity_minor=quantity_minor+1 WHERE location_id=? AND sku_id=?`, ordinary, M2DemoSKUID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ObjectRP(ctx, stow); !core.HasCode(err, core.CodeProjectionDiverged) {
		t.Fatalf("stow credited a forged ordinary inventory balance: %v", err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM stock_movements WHERE reason_code='rp_object_stow'`, nil, 0)
	if _, err := s.db.ExecContext(ctx, `UPDATE inventory_balances SET quantity_minor=? WHERE location_id=? AND sku_id=?`, before-1, ordinary, M2DemoSKUID); err != nil {
		t.Fatal(err)
	}
	returned, err := s.ObjectRP(ctx, stow)
	if err != nil || returned.ObjectID != item.ObjectID {
		t.Fatalf("cannot return held object: %+v %v", returned, err)
	}
	if repeated, err := s.ObjectRP(ctx, stow); err != nil || !repeated.Replayed || repeated.EventID != returned.EventID {
		t.Fatalf("stow key duplicated stock: %+v %v", repeated, err)
	}
	changed := stow
	changed.ObjectID = "another_object"
	if _, err := s.ObjectRP(ctx, changed); !core.HasCode(err, core.CodeIdempotencyMismatch) {
		t.Fatalf("changed stow key accepted: %v", err)
	}
	assertM2Value(t, ctx, s, `SELECT quantity_minor FROM inventory_balances WHERE location_id=? AND sku_id=?`, []any{ordinary, M2DemoSKUID}, before)
	assertM2Value(t, ctx, s, `SELECT quantity_minor FROM inventory_balances WHERE location_id=(SELECT escrow_location_id FROM rp_objects WHERE object_id=?) AND sku_id=?`, []any{item.ObjectID, M2DemoSKUID}, 0)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM stock_movements WHERE event_id=? AND from_location_id=(SELECT escrow_location_id FROM rp_objects WHERE object_id=?) AND to_location_id=? AND quantity_minor=1 AND movement_kind='transfer' AND reason_code='rp_object_stow'`, []any{returned.EventID, item.ObjectID, ordinary}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_objects WHERE object_id=? AND physical_state='stowed' AND holder_actor_id IS NULL AND anchor_id IS NULL`, []any{item.ObjectID}, 1)
	candidates, err := s.ReadRPObjectCandidates(ctx, player)
	if err != nil {
		t.Fatal(err)
	}
	sourceReturned := false
	for _, candidate := range candidates {
		if candidate.ID == item.ObjectID {
			t.Fatalf("stowed physical object returned as candidate: %+v", candidate)
		}
		if candidate.ID == source {
			sourceReturned = true
		}
	}
	if !sourceReturned {
		t.Fatal("restored stock cannot be staged again from existing declaration")
	}
	if differences, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatalf("stow replay differences: %+v %v", differences, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if differences, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatalf("restarted stow differences: %+v %v", differences, err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE rp_objects SET physical_state='held',holder_actor_id=? WHERE object_id=?`, M2RPPlayerID, item.ObjectID); err != nil {
		t.Fatal(err)
	}
	if differences, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) == 0 {
		t.Fatalf("stowed item was resurrected without detection: %+v %v", differences, err)
	}
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_objects WHERE object_id=? AND physical_state='stowed'`, []any{item.ObjectID}, 1)
	if _, err := s.db.ExecContext(ctx, `UPDATE stock_movements SET quantity_minor=2 WHERE event_id=?`, returned.EventID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); !core.HasCode(err, core.CodeProjectionDiverged) {
		t.Fatalf("forged stow movement survived comparison: %v", err)
	}
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); !core.HasCode(err, core.CodeProjectionDiverged) {
		t.Fatalf("rebuild manufactured a return over corrupt movement: %v", err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE stock_movements SET quantity_minor=1 WHERE event_id=?`, returned.EventID); err != nil {
		t.Fatal(err)
	}
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	if repeat, err := s.ObjectRP(ctx, stow); err != nil || !repeat.Replayed || repeat.EventID != returned.EventID {
		t.Fatalf("rebuild changed stow receipt: %+v %v", repeat, err)
	}
	attempt := objectTestRequest(t, ctx, s, player, "take", "resurrect-stowed")
	attempt.ObjectID = item.ObjectID
	if _, err := s.ObjectRP(ctx, attempt); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatalf("terminal object became physical again: %v", err)
	}
	again := objectTestRequest(t, ctx, s, player, "stage", "new-physical-id")
	again.SourceID = source
	fresh, err := s.ObjectRP(ctx, again)
	if err != nil || fresh.ObjectID == item.ObjectID {
		t.Fatalf("returned stock could not stage as a new object: %+v %v", fresh, err)
	}
}

func TestRPObjectStowRequiresWithdrawnOffer(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "stow-offer.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, player, _ := newRPWaitTestSession(t, ctx, s)
	source, _, _ := objectTestSource(t, ctx, s)
	stage := objectTestRequest(t, ctx, s, player, "stage", "stow-offer-stage")
	stage.SourceID = source
	item, err := s.ObjectRP(ctx, stage)
	if err != nil {
		t.Fatal(err)
	}
	offer := objectTestRequest(t, ctx, s, player, "offer", "stow-active-offer")
	offer.ObjectID, offer.TargetEntityID = item.ObjectID, M2RPNPCID
	offered, err := s.ObjectRP(ctx, offer)
	if err != nil {
		t.Fatal(err)
	}
	request := objectTestRequest(t, ctx, s, player, "stow", "stow-locked")
	request.ObjectID = item.ObjectID
	if _, err := s.ObjectRP(ctx, request); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatalf("object stowed under active offer: %v", err)
	}
	cai := allowFixtureControl(t, ctx, s, M2RPNPCID)
	accept := objectTestRequest(t, ctx, s, cai, "accept", "stow-accepted")
	accept.OfferID = offered.OfferID
	if _, err := s.ObjectRP(ctx, accept); err != nil {
		t.Fatal(err)
	}
	request = objectTestRequest(t, ctx, s, player, "stow", "stow-accepted-locked")
	request.ObjectID = item.ObjectID
	if _, err := s.ObjectRP(ctx, request); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatalf("accepted but undelivered offer stowed: %v", err)
	}
	cancel := objectTestRequest(t, ctx, s, player, "cancel_offer", "withdraw-then-stow")
	cancel.OfferID = offered.OfferID
	if _, err := s.ObjectRP(ctx, cancel); err != nil {
		t.Fatal(err)
	}
	request = objectTestRequest(t, ctx, s, player, "stow", "after-withdrawal")
	request.ObjectID = item.ObjectID
	if _, err := s.ObjectRP(ctx, request); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_object_offers WHERE offer_id=? AND status='cancelled' AND response_event_id IS NOT NULL AND terminal_event_id IS NOT NULL`, []any{offered.OfferID}, 1)
	if differences, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatalf("offer remained inconsistent after stow: %+v %v", differences, err)
	}
}
