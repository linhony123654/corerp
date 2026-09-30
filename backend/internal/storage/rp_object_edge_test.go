package storage

import (
	"context"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPObjectOfferWithdrawalDoesNotTransferAndCanReoffer(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "object-withdrawal.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, player, _ := newRPWaitTestSession(t, ctx, s)
	source, _, _ := objectTestSource(t, ctx, s)
	stage := objectTestRequest(t, ctx, s, player, "stage", "withdraw-stage")
	stage.SourceID = source
	item, err := s.ObjectRP(ctx, stage)
	if err != nil {
		t.Fatal(err)
	}
	offer := objectTestRequest(t, ctx, s, player, "offer", "withdraw-offer")
	offer.ObjectID, offer.TargetEntityID = item.ObjectID, M2RPNPCID
	offered, err := s.ObjectRP(ctx, offer)
	if err != nil {
		t.Fatal(err)
	}
	withdraw := objectTestRequest(t, ctx, s, player, "cancel_offer", "withdraw-consent")
	withdraw.OfferID = offered.OfferID
	if _, err := s.ObjectRP(ctx, withdraw); err != nil {
		t.Fatalf("offer could not be withdrawn: %v", err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_object_offers WHERE offer_id=? AND status='cancelled' AND terminal_event_id IS NOT NULL`, []any{offered.OfferID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM stock_movements m WHERE m.event_id IN (SELECT event_id FROM events WHERE event_type='RPObjectInteracted')`, nil, 1)
	cai := allowFixtureControl(t, ctx, s, M2RPNPCID)
	accept := objectTestRequest(t, ctx, s, cai, "accept", "accept-cancelled")
	accept.OfferID = offered.OfferID
	if _, err := s.ObjectRP(ctx, accept); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatalf("withdrawn consent was accepted: %v", err)
	}
	reoffer := objectTestRequest(t, ctx, s, player, "offer", "new-offer")
	reoffer.ObjectID, reoffer.TargetEntityID = item.ObjectID, M2RPNPCID
	newOffer, err := s.ObjectRP(ctx, reoffer)
	if err != nil || newOffer.OfferID == offered.OfferID {
		t.Fatalf("new offer after cancellation failed: %+v %v", newOffer, err)
	}
}

func TestRPObjectEscrowAndOfferProjectionCannotForgeConsent(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "object-escrow-corruption.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, player, _ := newRPWaitTestSession(t, ctx, s)
	source, anchor, _ := objectTestSource(t, ctx, s)
	stage := objectTestRequest(t, ctx, s, player, "stage", "corrupt-escrow-stage")
	stage.SourceID = source
	item, err := s.ObjectRP(ctx, stage)
	if err != nil {
		t.Fatal(err)
	}
	place := objectTestRequest(t, ctx, s, player, "place", "corrupt-escrow-place")
	place.ObjectID, place.AnchorID = item.ObjectID, anchor
	if _, err := s.db.ExecContext(ctx, `UPDATE inventory_balances SET quantity_minor=2 WHERE location_id=(SELECT escrow_location_id FROM rp_objects WHERE object_id=?) AND sku_id=?`, item.ObjectID, M2DemoSKUID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ObjectRP(ctx, place); !core.HasCode(err, core.CodeProjectionDiverged) {
		t.Fatalf("forged escrow quantity accepted: %v", err)
	}
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ObjectRP(ctx, place); err != nil {
		t.Fatalf("finite escrow did not recover from Event: %v", err)
	}
	take := objectTestRequest(t, ctx, s, player, "take", "recover-take")
	take.ObjectID = item.ObjectID
	if _, err := s.ObjectRP(ctx, take); err != nil {
		t.Fatal(err)
	}
	offer := objectTestRequest(t, ctx, s, player, "offer", "forge-offer")
	offer.ObjectID, offer.TargetEntityID = item.ObjectID, M2RPNPCID
	offered, err := s.ObjectRP(ctx, offer)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE rp_object_offers SET status='accepted_pending_transfer' WHERE offer_id=?`, offered.OfferID); err != nil {
		t.Fatal(err)
	}
	give := objectTestRequest(t, ctx, s, player, "give", "forge-transfer")
	give.OfferID = offered.OfferID
	if _, err := s.ObjectRP(ctx, give); !core.HasCode(err, core.CodeProjectionDiverged) {
		t.Fatalf("corrupted offer projection manufactured recipient consent: %v", err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM stock_movements WHERE reason_code='rp_object_give'`, nil, 0)
}
