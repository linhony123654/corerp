package core

import "testing"

func TestRPTransitEarliestArrivalTraversesActualRoutes(t *testing.T) {
	edges := []RPTransitEdge{{"home", "cafe"}, {"cafe", "home"}, {"cafe", "work"}, {"work", "cafe"}}
	windows := []RPTransitWindow{{"works", "cafe", "work", "2026-09-23T08:00:00Z", "2026-09-23T09:00:00Z"}}
	for _, pair := range [][2]string{{"home", "work"}, {"work", "home"}} {
		arrival, err := EarliestRPTransitArrival(edges, windows, pair[0], pair[1], "2026-09-23T08:00:00Z")
		if err != nil || !arrival.Reachable || arrival.WorldTime != "2026-09-23T09:00:00Z" || len(arrival.Path) != 3 || len(arrival.DelaySourceEventIDs) != 1 || arrival.DelaySourceEventIDs[0] != "works" {
			t.Fatalf("scheduled path bypassed works: %+v %v", arrival, err)
		}
	}
	for _, at := range []string{"2026-09-23T07:59:59Z", "2026-09-23T09:00:00Z"} {
		arrival, err := EarliestRPTransitArrival(edges, windows, "home", "work", at)
		if err != nil || arrival.WorldTime != at || len(arrival.DelaySourceEventIDs) != 0 {
			t.Fatalf("half-open interval: %+v %v", arrival, err)
		}
	}
	edges = append(edges, RPTransitEdge{"home", "side"}, RPTransitEdge{"side", "work"})
	arrival, err := EarliestRPTransitArrival(edges, windows, "home", "work", "2026-09-23T08:00:00Z")
	if err != nil || arrival.WorldTime != "2026-09-23T08:00:00Z" || len(arrival.Path) != 3 || arrival.Path[1] != "side" || len(arrival.DelaySourceEventIDs) != 0 {
		t.Fatalf("open alternative ignored: %+v %v", arrival, err)
	}
	arrival, err = EarliestRPTransitArrival(edges, windows, "work", "unknown", "2026-09-23T08:00:00Z")
	if err != nil || arrival.Reachable {
		t.Fatal("invented route")
	}
}

func TestRPTransitChainedIntervalsAndSourceValidation(t *testing.T) {
	edges := []RPTransitEdge{{"a", "b"}}
	windows := []RPTransitWindow{{"later", "a", "b", "2026-09-23T09:00:00Z", "2026-09-23T10:00:00Z"}, {"first", "a", "b", "2026-09-23T08:00:00Z", "2026-09-23T09:00:00Z"}}
	arrival, err := EarliestRPTransitArrival(edges, windows, "a", "b", "2026-09-23T08:30:00Z")
	if err != nil || arrival.WorldTime != "2026-09-23T10:00:00Z" || len(arrival.DelaySourceEventIDs) != 2 || arrival.DelaySourceEventIDs[0] != "first" {
		t.Fatalf("chained wait: %+v %v", arrival, err)
	}
	windows[0].SourceEventID = ""
	if _, err := EarliestRPTransitArrival(edges, windows, "a", "b", "2026-09-23T08:30:00Z"); !HasCode(err, CodeInvalidArgument) {
		t.Fatal("unsourced interval accepted")
	}
}
