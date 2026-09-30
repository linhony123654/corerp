package storage

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPObjectCrossZoneWitnessSeesNeutralActionWithoutReach(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "object-witness-reach.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	_, player, _ := newRPWaitTestSession(t, ctx, s)
	source, _, _ := objectTestSource(t, ctx, s)
	home, err := s.DefineRPObjectAnchor(ctx, RPObjectAnchorRequest{Binding: careerTestBinding(t, s, "principal_creator", "cross-zone-home-anchor"), PlaceID: "place_m2_home_ada", ZoneKey: "main", AnchorCode: "home-main-table", DisplayName: "桌面"})
	if err != nil {
		t.Fatal(err)
	}
	stage := objectTestRequest(t, ctx, s, player, "stage", "cross-zone-staged")
	stage.SourceID = source
	staged, err := s.ObjectRP(ctx, stage)
	if err != nil {
		t.Fatal(err)
	}
	view, err := s.ObserveRPSession(ctx, player)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.MoveRP(ctx, core.RPMoveRequest{PrincipalID: player.PrincipalID, SessionID: player.SessionID, FromPlaceID: M2AgentCafeID, ToPlaceID: "place_m2_home_ada", ExpectedCursor: view.ObservationCursor, IdempotencyKey: "cross-zone-arrival"}); err != nil {
		t.Fatal(err)
	}
	ada := allowFixtureControl(t, ctx, s, M2AgentAdaID)
	if _, err := s.DefineRPPerceptionLink(ctx, RPPerceptionLinkRequest{Binding: careerTestBinding(t, s, "principal_creator", "home-perception-to-porch"), PlaceID: "place_m2_home_ada", ZoneA: "main", ZoneB: "porch", BarrierKind: "open", BarrierState: "open", DistanceM: 2, VisualRangeM: 15, AudioRangeM: 15}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PlaceRPActorInZone(ctx, RPActorZoneRequest{Binding: careerTestBinding(t, s, "principal_creator", "ada-on-porch"), AgentID: M2AgentAdaID, PlaceID: "place_m2_home_ada", ZoneKey: "porch"}); err != nil {
		t.Fatal(err)
	}
	begin, err := s.ReadRPEvents(ctx, RPEventsReadRequest{PrincipalID: ada.PrincipalID, SessionID: ada.SessionID})
	if err != nil {
		t.Fatal(err)
	}
	place := objectTestRequest(t, ctx, s, player, "place", "cross-zone-place")
	place.ObjectID, place.AnchorID = staged.ObjectID, home.Fact.AnchorID
	placed, err := s.ObjectRP(ctx, place)
	if err != nil {
		t.Fatal(err)
	}
	witness, err := s.ReadRPEvents(ctx, RPEventsReadRequest{PrincipalID: ada.PrincipalID, SessionID: ada.SessionID, After: begin.HeadSequence})
	if err != nil || len(witness.Events) != 1 || witness.Events[0].OwnAction != nil || len(witness.Events[0].Facts) != 1 || witness.Events[0].Facts[0].Kind != "object_interaction" || witness.Events[0].Facts[0].Action != "interact" {
		t.Fatalf("cross-zone visual witness did not get neutral fact: %+v %v", witness, err)
	}
	candidates, err := s.ReadRPObjectCandidates(ctx, ada)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range candidates {
		if c.ObjectID == staged.ObjectID || c.ID == staged.ObjectID || c.ID == home.Fact.AnchorID || c.Kind == "offer" {
			t.Fatalf("visual sight manufactured touch/consent candidate: %+v", c)
		}
	}
	steal := objectTestRequest(t, ctx, s, ada, "take", "visual-is-not-reach")
	steal.ObjectID = staged.ObjectID
	if _, err := s.ObjectRP(ctx, steal); !core.HasCode(err, core.CodeNotFound) {
		t.Fatalf("cross-zone witness touched another's object: %v", err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_objects WHERE object_id=? AND physical_state='placed' AND anchor_id=?`, []any{staged.ObjectID, home.Fact.AnchorID}, 1)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	recovered, err := s.ReadRPEvents(ctx, RPEventsReadRequest{PrincipalID: ada.PrincipalID, SessionID: ada.SessionID, After: begin.HeadSequence})
	if err != nil || !reflect.DeepEqual(witness, recovered) {
		t.Fatalf("cross-zone historical witness changed after restart: %+v %v", recovered, err)
	}
	if diffs, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diffs) != 0 {
		t.Fatalf("cross-zone witnessed object did not replay: %+v %v", diffs, err)
	}
	if placed.EventID == "" {
		t.Fatal("placed object lacks Event")
	}
}
