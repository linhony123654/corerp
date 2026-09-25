package storage

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"corerp.local/backend/internal/core"
)

func TestRPMapMemoryIsScopedAndCanBecomeStale(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "map.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	_, read, view := newRPWaitTestSession(t, ctx, s)
	request := RPMapSurveyRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, IdempotencyKey: "survey-cafe"}
	bad := request
	bad.PrincipalID = "principal_creator"
	if _, err := s.SurveyRPMap(ctx, bad); !core.HasCode(err, core.CodeUnauthorized) && !core.HasCode(err, core.CodeNotFound) {
		t.Fatal("foreign principal surveyed map", err)
	}
	survey, err := s.SurveyRPMap(ctx, request)
	if err != nil || survey.Fact.ObserverID != M2RPPlayerID || survey.Fact.PlaceID != M2AgentCafeID || len(survey.Fact.Routes) == 0 {
		t.Fatal("missing local map", survey, err)
	}
	retry, err := s.SurveyRPMap(ctx, request)
	if err != nil || !retry.Replayed || retry.EventID != survey.EventID {
		t.Fatal("map survey retry duplicated", retry, err)
	}
	start, err := time.Parse(time.RFC3339, view.WorldTime)
	if err != nil {
		t.Fatal(err)
	}
	endpoints := [2]string{M2AgentCafeID, "place_m2_home_ada"}
	works, err := s.DefineRPTransitWorks(ctx, TransitWorksRequest{Binding: core.CareerBinding{PrincipalID: "principal_creator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: survey.EventSequence, IdempotencyKey: "map-works"}, FromPlaceID: endpoints[0], ToPlaceID: endpoints[1], StartsAt: start.Add(time.Minute).Format(time.RFC3339), EndsAt: start.Add(20 * time.Minute).Format(time.RFC3339)})
	if err != nil {
		t.Fatal(err)
	}
	view, err = s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	wait, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: start.Add(2 * time.Minute).Format(time.RFC3339), Budget: 20, IdempotencyKey: "map-closure-time"})
	if err != nil || wait.Status != "completed" {
		t.Fatal("advance into roadworks", wait, err)
	}
	view, err = s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	var blocked bool
	for _, route := range view.ReachablePlaces {
		if route.PlaceID == endpoints[1] {
			blocked = !route.CanMoveNow && !route.CanStartJourney
		}
	}
	if !blocked {
		t.Fatal("authoritative route was not blocked")
	}
	memories, err := s.ReadRPMap(ctx, read)
	if err != nil || len(memories) != 1 || memories[0].ObservedAt != survey.Fact.ObservedAt || memories[0].PlaceName != survey.Fact.PlaceName || memories[0].Routes[0].WorksUntil != "" {
		t.Fatal("old map changed after roadworks", memories, err)
	}
	if survey.Fact.ObservedAt == view.WorldTime || works.EventID == "" {
		t.Fatal("test did not reach later sourced state")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	memories, err = s.ReadRPMap(ctx, read)
	if err != nil || len(memories) != 1 || memories[0].ObservedAt != survey.Fact.ObservedAt {
		t.Fatal("map memory changed after restart", memories, err)
	}
	if diffs, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diffs) != 0 {
		t.Fatal("map survey broke replay", diffs, err)
	}
}
