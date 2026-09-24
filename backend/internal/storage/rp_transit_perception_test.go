package storage

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPTransitPerceptionIsCurrentAdjacentAndShared(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "transit-perception.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, read, _ := newRPWaitTestSession(t, ctx, s)
	works, err := s.DefineRPTransitWorks(ctx, TransitWorksRequest{Binding: careerTestBinding(t, s, "principal_creator", "visible-works"), FromPlaceID: M2AgentCafeID, ToPlaceID: "place_m2_work_ada", StartsAt: "2026-09-22T03:00:00Z", EndsAt: "2026-09-22T04:00:00Z"})
	if err != nil {
		t.Fatal(err)
	}
	view, err := s.ObserveRPSession(ctx, read)
	if err != nil || len(view.TransitWorks) != 0 {
		t.Fatalf("future works visible: %+v %v", view.TransitWorks, err)
	}
	assertRPMapRoute(t, view, "place_m2_work_ada", true)
	wait, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: "2026-09-22T03:15:00Z", Budget: 100, IdempotencyKey: "works-start"})
	if err != nil {
		t.Fatal(err)
	}
	view, err = s.ObserveRPSession(ctx, read)
	if err != nil || len(view.TransitWorks) != 1 || view.TransitWorks[0].SourceEventID != works.EventID || view.TransitWorks[0].FromPlaceID != view.PlaceID || view.TransitWorks[0].EndsAt != "2026-09-22T04:00:00Z" {
		t.Fatalf("local works: %+v %v", view.TransitWorks, err)
	}
	assertRPMapRoute(t, view, "place_m2_work_ada", false)
	assertRPMapRoute(t, view, "place_m2_home_bo", true)
	input, err := s.BuildRPInitiativeInput(ctx, core.RPInitiativeRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, NPCEntityID: M2RPNPCID, TriggerEventID: wait.EventID})
	if err != nil || len(input.TransitWorks) != 1 || input.TransitWorks[0] != view.TransitWorks[0] {
		t.Fatalf("NPC/player works differ: %+v %v", input.TransitWorks, err)
	}
	raw, err := json.Marshal(view.TransitWorks)
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{"builder_source", "scheduler", "retry", "original_world_time"} {
		if strings.Contains(string(raw), private) {
			t.Fatalf("private transit data: %s", raw)
		}
	}
	if _, err := s.MoveRP(ctx, core.RPMoveRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, FromPlaceID: view.PlaceID, ToPlaceID: "place_m2_home_bo", ExpectedCursor: view.ObservationCursor, IdempotencyKey: "leave-works"}); err != nil {
		t.Fatal(err)
	}
	view, err = s.ObserveRPSession(ctx, read)
	if err != nil || len(view.TransitWorks) != 0 {
		t.Fatalf("distant works visible: %+v %v", view.TransitWorks, err)
	}
	if _, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: "2026-09-22T04:00:00Z", Budget: 100, IdempotencyKey: "works-expire"}); err != nil {
		t.Fatal(err)
	}
	view, err = s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.MoveRP(ctx, core.RPMoveRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, FromPlaceID: view.PlaceID, ToPlaceID: M2AgentCafeID, ExpectedCursor: view.ObservationCursor, IdempotencyKey: "return-after-works"}); err != nil {
		t.Fatal(err)
	}
	view, err = s.ObserveRPSession(ctx, read)
	if err != nil || len(view.TransitWorks) != 0 {
		t.Fatalf("expired works visible: %+v %v", view.TransitWorks, err)
	}
	assertRPMapRoute(t, view, "place_m2_work_ada", true)
}

func assertRPMapRoute(t *testing.T, view RPObservation, target string, allowed bool) {
	t.Helper()
	for _, place := range view.ReachablePlaces {
		if place.PlaceID == target {
			if place.CanMoveNow != allowed {
				t.Fatalf("map route disagrees with current transit: %+v", place)
			}
			return
		}
	}
	t.Fatalf("map route missing: %s", target)
}
