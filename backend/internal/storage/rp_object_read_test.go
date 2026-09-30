package storage

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPObjectOwnerAndAnonymousWitnessReadOnlyCommittedSafeFacts(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "object-read-boundary.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	_, player, _ := newRPWaitTestSession(t, ctx, s)
	source, _, _ := objectTestSource(t, ctx, s)
	home, err := s.DefineRPObjectAnchor(ctx, RPObjectAnchorRequest{Binding: careerTestBinding(t, s, "principal_creator", "ada-home-object-anchor"), PlaceID: "place_m2_home_ada", ZoneKey: "main", AnchorCode: "home-table", DisplayName: "近旁桌面"})
	if err != nil {
		t.Fatal(err)
	}
	stage := objectTestRequest(t, ctx, s, player, "stage", "read-stage")
	stage.SourceID = source
	staged, err := s.ObjectRP(ctx, stage)
	if err != nil {
		t.Fatal(err)
	}
	current, err := s.ObserveRPSession(ctx, player)
	if err != nil {
		t.Fatal(err)
	}
	moved, err := s.MoveRP(ctx, core.RPMoveRequest{PrincipalID: player.PrincipalID, SessionID: player.SessionID, FromPlaceID: M2AgentCafeID, ToPlaceID: "place_m2_home_ada", ExpectedCursor: current.ObservationCursor, IdempotencyKey: "bring-staged-object-to-ada"})
	if err != nil {
		t.Fatal(err)
	}
	ada := allowFixtureControl(t, ctx, s, M2AgentAdaID)
	alias, err := rpAnonymousEntityIDForTest(ctx, s, M2DemoInstanceID, M2DemoBranchID, M2AgentAdaID, M2RPPlayerID)
	if err != nil {
		t.Fatal(err)
	}
	before, err := s.ReadRPEvents(ctx, RPEventsReadRequest{PrincipalID: ada.PrincipalID, SessionID: ada.SessionID, After: moved.EventSequence})
	if err != nil || len(before.Events) != 0 {
		t.Fatalf("private stage was visible before witness action: %+v %v", before, err)
	}
	place := objectTestRequest(t, ctx, s, player, "place", "read-place")
	place.ObjectID, place.AnchorID = staged.ObjectID, home.Fact.AnchorID
	placed, err := s.ObjectRP(ctx, place)
	if err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_object_offers WHERE object_id=?`, []any{staged.ObjectID}, 0)

	own, err := s.ReadRPEvents(ctx, RPEventsReadRequest{PrincipalID: player.PrincipalID, SessionID: player.SessionID, After: moved.EventSequence})
	if err != nil || len(own.Events) != 1 || own.Events[0].EventID != placed.EventID || own.Events[0].OwnAction == nil || own.Events[0].OwnAction.Kind != "object" || own.Events[0].OwnAction.Action != "place" || own.Events[0].OwnAction.ObjectID != staged.ObjectID || own.Events[0].OwnAction.AnchorID != home.Fact.AnchorID {
		t.Fatalf("owner saw an invented or incomplete physical fact: %+v %v", own, err)
	}
	if own.Events[0].OwnAction.OfferID != "" || own.Events[0].OwnAction.TargetEntityID != "" || strings.Contains(own.Events[0].OwnAction.Text, "交到") {
		t.Fatalf("mere placement invented a consent/transfer fact: %+v", own.Events[0])
	}
	ownerView, err := s.ObserveRPSession(ctx, player)
	if err != nil {
		t.Fatal(err)
	}
	if !rpObjectHistoryHasOnlyLine(ownerView.RecentTurns, placed.EventID, placed.Description) {
		t.Fatalf("owner recent turns did not include only committed placement: %+v", ownerView.RecentTurns)
	}

	witnessContext, err := s.ReadRPContext(ctx, RPContextReadRequest{PrincipalID: ada.PrincipalID, SessionID: ada.SessionID, SubjectEntityID: alias})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReadRPContext(ctx, RPContextReadRequest{PrincipalID: ada.PrincipalID, SessionID: ada.SessionID, SubjectEntityID: M2RPPlayerID}); !core.HasCode(err, core.CodeNotFound) {
		t.Fatalf("raw actor ID bypassed anonymity: %v", err)
	}
	anonymousEvent, err := rpAnonymousEvidenceIDForTest(ctx, s, M2DemoInstanceID, M2DemoBranchID, M2AgentAdaID, placed.EventID)
	if err != nil {
		t.Fatal(err)
	}
	objectClaim := false
	for _, fact := range witnessContext.Facts {
		if fact.Kind != "object_interaction" {
			continue
		}
		objectClaim = true
		if fact.SubjectEntityID != alias || fact.SourceEventID != anonymousEvent || fact.Action != "interact" || fact.TargetEntityID != "" || fact.Text != "有人在近处做出与物品相关的动作。" {
			t.Fatalf("witness context exceeded actual neutral sight: %+v", fact)
		}
	}
	if !objectClaim {
		t.Fatalf("no sourced neutral witness claim: %+v", witnessContext)
	}
	witnessEvents, err := s.ReadRPEvents(ctx, RPEventsReadRequest{PrincipalID: ada.PrincipalID, SessionID: ada.SessionID, After: moved.EventSequence})
	if err != nil || len(witnessEvents.Events) != 1 || witnessEvents.Events[0].EventID != anonymousEvent || witnessEvents.Events[0].OwnAction != nil || len(witnessEvents.Events[0].Facts) != 1 || witnessEvents.Events[0].Facts[0].SubjectEntityID != alias || witnessEvents.Events[0].Facts[0].SourceEventID != anonymousEvent {
		t.Fatalf("witness Events exposed raw object action: %+v %v", witnessEvents, err)
	}
	witnessView, err := s.ObserveRPSession(ctx, ada)
	if err != nil {
		t.Fatal(err)
	}
	if !rpObjectHistoryHasOnlyLine(witnessView.RecentTurns, anonymousEvent, "有人在近处做出与物品相关的动作。") {
		t.Fatalf("witness recent turns leaked Event ID or object secrets: %+v", witnessView.RecentTurns)
	}
	for _, value := range []any{witnessContext, witnessEvents, witnessView.RecentTurns} {
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		for _, secret := range []string{M2RPPlayerID, staged.ObjectID, source, M2DemoSKUID, placed.EventID, "rp_offer_", "stock_rp_object_", `"offer_status"`, `"target_entity_id"`} {
			if strings.Contains(string(encoded), secret) {
				t.Fatalf("witness read leaked private object/identity fact %q in %s", secret, encoded)
			}
		}
	}

	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	recoveredOwn, err := s.ReadRPEvents(ctx, RPEventsReadRequest{PrincipalID: player.PrincipalID, SessionID: player.SessionID, After: moved.EventSequence})
	if err != nil || !reflect.DeepEqual(own, recoveredOwn) {
		t.Fatalf("owner read changed after restart/rebuild: %+v %v", recoveredOwn, err)
	}
	recoveredWitness, err := s.ReadRPEvents(ctx, RPEventsReadRequest{PrincipalID: ada.PrincipalID, SessionID: ada.SessionID, After: moved.EventSequence})
	if err != nil || !reflect.DeepEqual(witnessEvents, recoveredWitness) {
		t.Fatalf("witness anonymization changed after restart/rebuild: %+v %v", recoveredWitness, err)
	}
}

func TestRPObjectThirdPartyWitnessNeverReadsOfferOrStockFromSight(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "object-third-party-read.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	_, player, _ := newRPWaitTestSession(t, ctx, s)
	ada := allowFixtureControl(t, ctx, s, M2AgentAdaID)
	adaView, err := s.ObserveRPSession(ctx, ada)
	if err != nil {
		t.Fatal(err)
	}
	arrived, err := s.MoveRP(ctx, core.RPMoveRequest{PrincipalID: ada.PrincipalID, SessionID: ada.SessionID, FromPlaceID: "place_m2_home_ada", ToPlaceID: M2AgentCafeID, ExpectedCursor: adaView.ObservationCursor, IdempotencyKey: "witness-ada-arrives"})
	if err != nil {
		t.Fatal(err)
	}
	alias, err := rpAnonymousEntityIDForTest(ctx, s, M2DemoInstanceID, M2DemoBranchID, M2AgentAdaID, M2RPPlayerID)
	if err != nil {
		t.Fatal(err)
	}
	source, _, _ := objectTestSource(t, ctx, s)
	stage := objectTestRequest(t, ctx, s, player, "stage", "third-party-stage")
	stage.SourceID = source
	item, err := s.ObjectRP(ctx, stage)
	if err != nil {
		t.Fatal(err)
	}
	offer := objectTestRequest(t, ctx, s, player, "offer", "third-party-offer")
	offer.ObjectID, offer.TargetEntityID = item.ObjectID, M2RPNPCID
	offered, err := s.ObjectRP(ctx, offer)
	if err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_object_offers WHERE offer_id=? AND status='offered' AND response_event_id IS NULL`, []any{offered.OfferID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM stock_movements WHERE event_id=?`, []any{offered.EventID}, 0)
	own, err := s.ReadRPEvents(ctx, RPEventsReadRequest{PrincipalID: player.PrincipalID, SessionID: player.SessionID, After: arrived.EventSequence})
	if err != nil {
		t.Fatal(err)
	}
	var ownOffer bool
	for _, event := range own.Events {
		if event.EventID == offered.EventID && event.OwnAction != nil && event.OwnAction.Action == "offer" && event.OwnAction.OfferID == offered.OfferID && event.OwnAction.ObjectID == item.ObjectID {
			ownOffer = true
		}
		if event.OwnAction != nil && (event.OwnAction.Action == "accept" || event.OwnAction.Action == "give" || event.OwnAction.Action == "receive") {
			t.Fatalf("unanswered proposal appeared to owner as settled transfer: %+v", event)
		}
	}
	if !ownOffer {
		t.Fatalf("owner cannot distinguish own committed offer from transfer: %+v", own)
	}
	witness, err := s.ReadRPEvents(ctx, RPEventsReadRequest{PrincipalID: ada.PrincipalID, SessionID: ada.SessionID, After: arrived.EventSequence})
	if err != nil || len(witness.Events) != 2 {
		t.Fatalf("third-party failed to see two neutral object gestures: %+v %v", witness, err)
	}
	witnessView, err := s.ObserveRPSession(ctx, ada)
	if err != nil {
		t.Fatal(err)
	}
	contextView, err := s.ReadRPContext(ctx, RPContextReadRequest{PrincipalID: ada.PrincipalID, SessionID: ada.SessionID, SubjectEntityID: alias})
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []RPObjectResult{item, offered} {
		anonymousID, err := rpAnonymousEvidenceIDForTest(ctx, s, M2DemoInstanceID, M2DemoBranchID, M2AgentAdaID, action.EventID)
		if err != nil {
			t.Fatal(err)
		}
		if !rpObjectHistoryHasOnlyLine(witnessView.RecentTurns, anonymousID, "有人在近处做出与物品相关的动作。") {
			t.Fatalf("third-party recent turns exposed an offer/stock action: %+v", witnessView.RecentTurns)
		}
	}
	for _, event := range witness.Events {
		if event.EventID == item.EventID || event.EventID == offered.EventID || event.OwnAction != nil || len(event.Facts) != 1 || event.Facts[0].Kind != "object_interaction" || event.Facts[0].SubjectEntityID != alias || event.Facts[0].TargetEntityID != "" || event.Facts[0].Action != "interact" {
			t.Fatalf("third-party saw private offer semantics or true actor ID: %+v", event)
		}
	}
	for _, value := range []any{contextView, witness, witnessView.RecentTurns} {
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		for _, secret := range []string{item.ObjectID, offered.OfferID, source, M2DemoSKUID, M2RPPlayerID, M2RPNPCID, offered.EventID, item.EventID, "stock_rp_object_", `"offer_status"`, `"target_entity_id"`} {
			if strings.Contains(string(encoded), secret) {
				t.Fatalf("third-party read leaked %q: %s", secret, encoded)
			}
		}
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	reopened, err := s.ReadRPEvents(ctx, RPEventsReadRequest{PrincipalID: ada.PrincipalID, SessionID: ada.SessionID, After: arrived.EventSequence})
	if err != nil || !reflect.DeepEqual(witness, reopened) {
		t.Fatalf("restart changed neutral sight or anonymous references: %+v %v", reopened, err)
	}
}

func rpObjectHistoryHasOnlyLine(history []RPHistoryTurn, id, line string) bool {
	for _, turn := range history {
		if turn.TurnRunID == id && len(turn.NarrativeLines) == 1 && turn.NarrativeLines[0] == line && !turn.CanRegenerate {
			return true
		}
	}
	return false
}
