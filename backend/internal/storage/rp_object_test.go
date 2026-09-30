package storage

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
)

func objectTestRequest(t *testing.T, ctx context.Context, s *Store, read core.RPSessionReadRequest, action, key string) core.RPObjectRequest {
	t.Helper()
	view, err := s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	return core.RPObjectRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, IdempotencyKey: key, Action: action}
}

func objectTestSource(t *testing.T, ctx context.Context, s *Store) (string, string, string) {
	t.Helper()
	first, err := s.DefineRPObjectAnchor(ctx, RPObjectAnchorRequest{Binding: careerTestBinding(t, s, "principal_creator", "object-hand-side"), PlaceID: M2AgentCafeID, ZoneKey: "main", AnchorCode: "near-hand", DisplayName: "手边台面"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.DefineRPObjectAnchor(ctx, RPObjectAnchorRequest{Binding: careerTestBinding(t, s, "principal_creator", "object-other-side"), PlaceID: M2AgentCafeID, ZoneKey: "main", AnchorCode: "near-cai", DisplayName: "邻座台面", NearEntityID: M2RPNPCID})
	if err != nil {
		t.Fatal(err)
	}
	// The historic staple SKU carries no intrinsic human-readable label. We
	// deliberately do not rename it to a cup to pass a scene test.
	source, err := s.DefineRPObjectSource(ctx, RPObjectSourceRequest{Binding: careerTestBinding(t, s, "principal_creator", "existing-staple-unit"), AnchorID: first.Fact.AnchorID, OwnerEntityID: M2RPPlayerID, SKUID: M2DemoSKUID, DisplayName: M2DemoSKUID})
	if err != nil {
		t.Fatal(err)
	}
	return source.Fact.SourceID, first.Fact.AnchorID, second.Fact.AnchorID
}

func TestRPObjectActualStockSeparateOfferConsentAndTransfer(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "object-actions.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	_, player, _ := newRPWaitTestSession(t, ctx, s)
	sourceID, near, neighbor := objectTestSource(t, ctx, s)
	stage := objectTestRequest(t, ctx, s, player, "stage", "stage-real-unit")
	stage.SourceID = sourceID
	s.beforeCommit = func() error { return errors.New("injected object failure") }
	if _, err := s.ObjectRP(ctx, stage); err == nil {
		t.Fatal("staging escaped failed transaction")
	}
	s.beforeCommit = nil
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_objects`, nil, 0)
	staged, err := s.ObjectRP(ctx, stage)
	if err != nil || staged.ObjectID == "" {
		t.Fatalf("stock-backed stage failed: %+v %v", staged, err)
	}
	repeat, err := s.ObjectRP(ctx, stage)
	if err != nil || !repeat.Replayed || repeat.ObjectID != staged.ObjectID {
		t.Fatalf("stage key duplicated item: %+v %v", repeat, err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM stock_movements WHERE event_id=? AND sku_id=? AND quantity_minor=1 AND movement_kind='transfer'`, []any{staged.EventID, M2DemoSKUID}, 1)
	assertM2Value(t, ctx, s, `SELECT quantity_minor FROM inventory_balances WHERE location_id=(SELECT escrow_location_id FROM rp_objects WHERE object_id=?) AND sku_id=?`, []any{staged.ObjectID, M2DemoSKUID}, 1)
	placed := objectTestRequest(t, ctx, s, player, "place", "set-near-hand")
	placed.ObjectID, placed.AnchorID = staged.ObjectID, near
	if _, err := s.ObjectRP(ctx, placed); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_objects WHERE object_id=? AND physical_state='placed' AND anchor_id=? AND holder_actor_id IS NULL`, []any{staged.ObjectID, near}, 1)
	moved := objectTestRequest(t, ctx, s, player, "move", "push-near-neighbor")
	moved.ObjectID, moved.AnchorID = staged.ObjectID, neighbor
	if _, err := s.ObjectRP(ctx, moved); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_object_offers WHERE object_id=?`, []any{staged.ObjectID}, 0)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM stock_movements m JOIN rp_objects o ON o.object_id=? WHERE m.to_location_id=o.escrow_location_id OR m.from_location_id=o.escrow_location_id`, []any{staged.ObjectID}, 1)
	taken := objectTestRequest(t, ctx, s, player, "take", "take-it-back")
	taken.ObjectID = staged.ObjectID
	if _, err := s.ObjectRP(ctx, taken); err != nil {
		t.Fatal(err)
	}
	offer := objectTestRequest(t, ctx, s, player, "offer", "offer-to-cai")
	offer.ObjectID, offer.TargetEntityID = staged.ObjectID, M2RPNPCID
	offered, err := s.ObjectRP(ctx, offer)
	if err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_object_offers WHERE offer_id=? AND status='offered' AND response_event_id IS NULL`, []any{offered.OfferID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_objects WHERE object_id=? AND owner_actor_id=? AND holder_actor_id=?`, []any{staged.ObjectID, M2RPPlayerID, M2RPPlayerID}, 1)
	premature := objectTestRequest(t, ctx, s, player, "give", "give-before-consent")
	premature.OfferID = offered.OfferID
	if _, err := s.ObjectRP(ctx, premature); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatalf("unaccepted offer became a transfer: %v", err)
	}
	// A player's claim or an NPC line does not grant control of the recipient.
	fake := objectTestRequest(t, ctx, s, player, "accept", "fake-consent")
	fake.OfferID = offered.OfferID
	if _, err := s.ObjectRP(ctx, fake); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("offerer accepted on recipient's behalf: %v", err)
	}
	cai := allowFixtureControl(t, ctx, s, M2RPNPCID)
	accept := objectTestRequest(t, ctx, s, cai, "accept", "cai-accepts")
	accept.OfferID = offered.OfferID
	accepted, err := s.ObjectRP(ctx, accept)
	if err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_object_offers WHERE offer_id=? AND status='accepted_pending_transfer' AND response_event_id=? AND terminal_event_id IS NULL`, []any{offered.OfferID, accepted.EventID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM stock_movements WHERE event_id=?`, []any{accepted.EventID}, 0)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_objects WHERE object_id=? AND owner_actor_id=? AND holder_actor_id=?`, []any{staged.ObjectID, M2RPPlayerID, M2RPPlayerID}, 1)
	give := objectTestRequest(t, ctx, s, player, "give", "physical-transfer")
	give.OfferID = offered.OfferID
	transferred, err := s.ObjectRP(ctx, give)
	if err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_object_offers WHERE offer_id=? AND status='transferred' AND terminal_event_id=?`, []any{offered.OfferID, transferred.EventID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_objects o JOIN stock_locations l ON l.location_id=o.escrow_location_id WHERE o.object_id=? AND o.owner_actor_id=? AND o.holder_actor_id=? AND l.owner_id=?`, []any{staged.ObjectID, M2RPNPCID, M2RPNPCID, M2RPNPCID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM stock_movements WHERE event_id=? AND movement_kind='transfer' AND quantity_minor=1`, []any{transferred.EventID}, 1)
	if _, err := s.ObjectRP(ctx, give); err != nil {
		t.Fatalf("repeat transfer did not replay: %v", err)
	}
	differences, err := rpObjectProjectionDifferences(ctx, s.db, M2DemoInstanceID, M2DemoBranchID, transferred.EventSequence)
	if err != nil || len(differences) != 0 {
		t.Fatalf("object Event reconstruction differs: %+v %v", differences, err)
	}
	if _, err := s.ObjectRP(ctx, core.RPObjectRequest{PrincipalID: player.PrincipalID, SessionID: player.SessionID, ExpectedCursor: give.ExpectedCursor, IdempotencyKey: give.IdempotencyKey, Action: "receive", OfferID: give.OfferID}); !core.HasCode(err, core.CodeIdempotencyMismatch) {
		t.Fatalf("key reuse changed action: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if retry, err := s.ObjectRP(ctx, give); err != nil || !retry.Replayed || retry.EventID != transferred.EventID {
		t.Fatalf("restart duplicated transfer: %+v %v", retry, err)
	}
}

func TestRPObjectSourceRejectsStapleRenamedAsCupAndUnownedItem(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "object-source.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, player, _ := newRPWaitTestSession(t, ctx, s)
	sourceID, near, _ := objectTestSource(t, ctx, s)
	if sourceID == "" {
		t.Fatal("missing sourced existing SKU")
	}
	_, err = s.DefineRPObjectSource(ctx, RPObjectSourceRequest{Binding: careerTestBinding(t, s, "principal_creator", "fake-cup"), AnchorID: near, OwnerEntityID: M2RPPlayerID, SKUID: M2DemoSKUID, DisplayName: "杯子"})
	if !core.HasCode(err, core.CodeInvalidArgument) {
		t.Fatalf("renamed staple became a cup: %v", err)
	}
	_, err = s.DefineRPObjectSource(ctx, RPObjectSourceRequest{Binding: careerTestBinding(t, s, player.PrincipalID, "unauthorized-source"), AnchorID: near, OwnerEntityID: M2RPPlayerID, SKUID: M2DemoSKUID, DisplayName: M2DemoSKUID})
	if !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("player authored own physical source: %v", err)
	}
	stage := objectTestRequest(t, ctx, s, player, "stage", "source-stock-mismatch")
	stage.SourceID = sourceID
	var inventory string
	if err := s.db.QueryRowContext(ctx, `SELECT inventory_location_id FROM materialized_entities WHERE entity_id=?`, M2RPPlayerID).Scan(&inventory); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE inventory_balances SET quantity_minor=quantity_minor+1 WHERE location_id=? AND sku_id=?`, inventory, M2DemoSKUID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ObjectRP(ctx, stage); !core.HasCode(err, core.CodeProjectionDiverged) {
		t.Fatalf("corrupted inventory asserted false item: %v", err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_objects`, nil, 0)
}
