package core

import (
	"sort"
	"time"
)

type RPTransitEdge struct{ FromPlaceID, ToPlaceID string }

// Current adjacent road works, not distant route forecasts or private queues.
// A direct road can be obstructed while another route remains open.
type RPLocalTransitWorks struct {
	SourceEventID string `json:"source_event_id"`
	FromPlaceID   string `json:"from_place_id"`
	ToPlaceID     string `json:"to_place_id"`
	EndsAt        string `json:"ends_at"`
}
type RPTransitWindow struct {
	SourceEventID string `json:"source_event_id"`
	FromPlaceID   string `json:"from_place_id"`
	ToPlaceID     string `json:"to_place_id"`
	StartsAt      string `json:"starts_at"`
	EndsAt        string `json:"ends_at"`
}
type RPTransitArrival struct {
	Reachable           bool
	WorldTime           string
	Path                []string
	DelaySourceEventIDs []string
}

// Existing RP edges have no travel duration. This computes only waiting imposed
// by sourced, bidirectional route works. It neither moves an actor nor advances
// a clock. Scheduled paths may span several edges; discretionary moves still
// require their ordinary single-edge command validation.
func EarliestRPTransitArrival(edges []RPTransitEdge, windows []RPTransitWindow, from, to, at string) (RPTransitArrival, error) {
	start, err := time.Parse(time.RFC3339, at)
	if err != nil || from == "" || to == "" || len(edges) > 4096 || len(windows) > 4096 {
		return RPTransitArrival{}, NewError(CodeInvalidArgument, "invalid bounded transit graph/time")
	}
	type interval struct {
		from, to, source string
		start, end       time.Time
	}
	intervals := make([]interval, 0, len(windows))
	for _, window := range windows {
		s, e1 := time.Parse(time.RFC3339, window.StartsAt)
		e, e2 := time.Parse(time.RFC3339, window.EndsAt)
		if e1 != nil || e2 != nil || !e.After(s) || window.SourceEventID == "" || window.FromPlaceID == "" || window.ToPlaceID == "" || window.FromPlaceID == window.ToPlaceID {
			return RPTransitArrival{}, NewError(CodeInvalidArgument, "invalid sourced transit interval")
		}
		intervals = append(intervals, interval{window.FromPlaceID, window.ToPlaceID, window.SourceEventID, s, e})
	}
	sort.Slice(intervals, func(i, j int) bool {
		if !intervals[i].start.Equal(intervals[j].start) {
			return intervals[i].start.Before(intervals[j].start)
		}
		return intervals[i].source < intervals[j].source
	})
	graph := map[string][]string{}
	for _, edge := range edges {
		if edge.FromPlaceID == "" || edge.ToPlaceID == "" || edge.FromPlaceID == edge.ToPlaceID {
			return RPTransitArrival{}, NewError(CodeInvalidArgument, "invalid transit edge")
		}
		graph[edge.FromPlaceID] = append(graph[edge.FromPlaceID], edge.ToPlaceID)
	}
	for node := range graph {
		sort.Strings(graph[node])
	}
	times := map[string]time.Time{from: start}
	paths := map[string][]string{from: {from}}
	causes := map[string][]string{from: {}}
	visited := map[string]bool{}
	for {
		node := ""
		for candidate, due := range times {
			if visited[candidate] {
				continue
			}
			if node == "" || due.Before(times[node]) || due.Equal(times[node]) && candidate < node {
				node = candidate
			}
		}
		if node == "" {
			return RPTransitArrival{}, nil
		}
		if node == to {
			return RPTransitArrival{Reachable: true, WorldTime: times[node].UTC().Format(time.RFC3339Nano), Path: paths[node], DelaySourceEventIDs: causes[node]}, nil
		}
		visited[node] = true
		for _, next := range graph[node] {
			if visited[next] {
				continue
			}
			due := times[node]
			sources := append([]string(nil), causes[node]...)
			for _, window := range intervals {
				matches := window.from == node && window.to == next || window.from == next && window.to == node
				if matches && !due.Before(window.start) && due.Before(window.end) {
					due = window.end
					sources = append(sources, window.source)
				}
			}
			if old, exists := times[next]; !exists || due.Before(old) {
				times[next] = due
				paths[next] = append(append([]string(nil), paths[node]...), next)
				causes[next] = sources
			}
		}
	}
}
