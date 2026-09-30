package storage

import (
	"context"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPObjectVisualSightNeverGrantsCrossZoneReachOrConsent(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "object-reach.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, player, _ := newRPWaitTestSession(t, ctx, s)
	source, near, _ := objectTestSource(t, ctx, s)
	stage := objectTestRequest(t, ctx, s, player, "stage", "near-reach-stage")
	stage.SourceID = source
	item, err := s.ObjectRP(ctx, stage)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DefineRPPerceptionLink(ctx, RPPerceptionLinkRequest{Binding: careerTestBinding(t, s, "principal_creator", "open-but-distant"), PlaceID: M2AgentCafeID, ZoneA: "main", ZoneB: "porch", BarrierKind: "open", BarrierState: "open", DistanceM: 2, VisualRangeM: 15, AudioRangeM: 15}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PlaceRPActorInZone(ctx, RPActorZoneRequest{Binding: careerTestBinding(t, s, "principal_creator", "player-on-porch"), AgentID: M2RPPlayerID, PlaceID: M2AgentCafeID, ZoneKey: "porch"}); err != nil {
		t.Fatal(err)
	}
	conn, err := s.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	visible, err := rpCanPerceive(ctx, conn, M2DemoInstanceID, M2DemoBranchID, M2RPPlayerID, M2RPNPCID, "visual", "")
	conn.Close()
	if err != nil || !visible {
		t.Fatalf("fixture must see across zones: %v %v", visible, err)
	}
	place := objectTestRequest(t, ctx, s, player, "place", "place-across-visible-gap")
	place.ObjectID, place.AnchorID = item.ObjectID, near
	if _, err := s.ObjectRP(ctx, place); !core.HasCode(err, core.CodeInvalidArgument) {
		t.Fatalf("visual-only reach allowed placement: %v", err)
	}
	offer := objectTestRequest(t, ctx, s, player, "offer", "offer-across-visible-gap")
	offer.ObjectID, offer.TargetEntityID = item.ObjectID, M2RPNPCID
	if _, err := s.ObjectRP(ctx, offer); !core.HasCode(err, core.CodeNotFound) {
		t.Fatalf("seeing recipient became physical handoff reach: %v", err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_object_offers WHERE object_id=?`, []any{item.ObjectID}, 0)
}
