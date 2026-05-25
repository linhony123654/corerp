package events

import (
	"fmt"
	"time"

	"corerp/internal/core"
)

// ReplayEngine reconstructs world state at any point in time and
// supports timeline forking for "what if" exploration.
type ReplayEngine struct {
	store *Store
}

func NewReplayEngine(store *Store) *ReplayEngine {
	return &ReplayEngine{store: store}
}

// ReplayTo reconstructs WorldState by replaying all canonical events
// on the main branch up to and including the given event.
func (r *ReplayEngine) ReplayTo(eventID string, branch string) (core.WorldState, error) {
	if branch == "" {
		branch = "main"
	}

	events, err := r.store.GetCanonicalEvents()
	if err != nil {
		return core.WorldState{}, err
	}

	state := core.WorldState{
		Relationships: make(map[string]core.Relationship),
		Variables:     make(map[string]interface{}),
		Flags:         make(map[string]bool),
	}

	for _, e := range events {
		// Skip events from other branches
		// (branch info not on core.Event, use a separate query)
		if !e.Canonical {
			continue
		}
		state = applyEvent(state, e)
		if e.ID == eventID {
			break
		}
	}

	return state, nil
}

// ReplayAtTime reconstructs WorldState by replaying events up to a
// specific world clock time.
func (r *ReplayEngine) ReplayAtTime(hour, minute, day int) (core.WorldState, error) {
	events, err := r.store.GetCanonicalEvents()
	if err != nil {
		return core.WorldState{}, err
	}

	state := core.WorldState{
		Relationships: make(map[string]core.Relationship),
		Variables:     make(map[string]interface{}),
		Flags:         make(map[string]bool),
	}

	for _, e := range events {
		if !e.Canonical {
			continue
		}
		state = applyEvent(state, e)

		// Check if we've reached the target time
		if state.Clock.Day > day {
			break
		}
		if state.Clock.Day == day && state.Clock.Hour > hour {
			break
		}
		if state.Clock.Day == day && state.Clock.Hour == hour && state.Clock.Minute >= minute {
			break
		}
	}

	return state, nil
}

// ForkPoint marks an event as the start of a new timeline branch.
// Events after this point on the new branch will diverge from the original.
func (r *ReplayEngine) Fork(eventID string, branchName string) error {
	// Tag the fork point event and all subsequent events can be branched
	_, err := r.store.db.Exec(
		`UPDATE events SET branch = ? WHERE id = ?`, branchName, eventID,
	)
	return err
}

// GetBranch returns all events on a specific branch in order.
func (r *ReplayEngine) GetBranch(branchName string) ([]core.Event, error) {
	rows, err := r.store.db.Query(
		`SELECT id, type, actor, target, payload, causes, effects, canonical, confidence, confirmations, scene_id, session_id, branch, created_at
		 FROM events WHERE branch = ? ORDER BY created_at ASC`, branchName,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEvents(rows)
}

// ListBranches returns all unique branch names.
func (r *ReplayEngine) ListBranches() ([]string, error) {
	rows, err := r.store.db.Query(
		`SELECT DISTINCT branch FROM events ORDER BY branch`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var branches []string
	for rows.Next() {
		var b string
		if err := rows.Scan(&b); err != nil {
			return nil, err
		}
		branches = append(branches, b)
	}
	return branches, nil
}

// EventTimeline represents a point in the event timeline for rendering.
type EventTimeline struct {
	Event      core.Event `json:"event"`
	Branch     string     `json:"branch"`
	EventIndex int        `json:"index"`
}

// GetTimeline returns the full event timeline for a branch.
func (r *ReplayEngine) GetTimeline(branch string, limit int) ([]EventTimeline, error) {
	if branch == "" {
		branch = "main"
	}

	rows, err := r.store.db.Query(
		`SELECT id, type, actor, target, payload, causes, effects, canonical, confidence, confirmations, scene_id, session_id, branch, created_at
		 FROM events WHERE branch = ? AND canonical = 1 ORDER BY created_at ASC LIMIT ?`, branch, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	events, err := scanEvents(rows)
	if err != nil {
		return nil, err
	}

	var timeline []EventTimeline
	for i, e := range events {
		timeline = append(timeline, EventTimeline{
			Event:      e,
			Branch:     branch,
			EventIndex: i,
		})
	}
	return timeline, nil
}

// CompareStates compares the world state at the same event index
// across two branches and returns the differences.
func (r *ReplayEngine) CompareStates(branchA, branchB string, atEventIndex int) (map[string]interface{}, error) {
	result := map[string]interface{}{
		"branch_a": branchA,
		"branch_b": branchB,
	}

	timelineA, err := r.GetTimeline(branchA, atEventIndex+1)
	if err != nil {
		return nil, fmt.Errorf("branch %s: %w", branchA, err)
	}
	timelineB, err := r.GetTimeline(branchB, atEventIndex+1)
	if err != nil {
		return nil, fmt.Errorf("branch %s: %w", branchB, err)
	}

	stateA := core.WorldState{
		Relationships: make(map[string]core.Relationship),
		Variables:     make(map[string]interface{}),
		Flags:         make(map[string]bool),
	}
	stateB := stateA

	for _, t := range timelineA {
		stateA = applyEvent(stateA, t.Event)
	}
	for _, t := range timelineB {
		stateB = applyEvent(stateB, t.Event)
	}

	// Diff key fields
	diffs := make(map[string]interface{})
	if stateA.Tension != stateB.Tension {
		diffs["tension"] = map[string]float64{"a": stateA.Tension, "b": stateB.Tension}
	}
	if stateA.Scene.Location != stateB.Scene.Location {
		diffs["location"] = map[string]string{"a": stateA.Scene.Location, "b": stateB.Scene.Location}
	}
	if stateA.Clock != stateB.Clock {
		diffs["clock"] = map[string]core.WorldTime{"a": stateA.Clock, "b": stateB.Clock}
	}

	// Relationship diffs
	relDiffs := make(map[string]interface{})
	allKeys := make(map[string]bool)
	for k := range stateA.Relationships {
		allKeys[k] = true
	}
	for k := range stateB.Relationships {
		allKeys[k] = true
	}
	for k := range allKeys {
		ra := stateA.Relationships[k]
		rb := stateB.Relationships[k]
		if ra != rb {
			relDiffs[k] = map[string]core.Relationship{"a": ra, "b": rb}
		}
	}
	if len(relDiffs) > 0 {
		diffs["relationships"] = relDiffs
	}

	result["diffs"] = diffs
	result["state_a"] = stateA
	result["state_b"] = stateB
	return result, nil
}

// BuildTimeline creates a simple timeline that can be rendered as a log.
func BuildTimeline(events []core.Event, maxPerType int) string {
	typeCounts := make(map[string]int)
	var out string
	for i, e := range events {
		typeCounts[e.Type]++
		if typeCounts[e.Type] > maxPerType && maxPerType > 0 {
			continue
		}
		ts := e.CreatedAt.Format("15:04:05")
		out += fmt.Sprintf("[%s] #%d %s: %s → %s\n", ts, i, e.Type, e.Actor, e.Target)
	}
	return out
}

// ReplaySummary returns a compact summary of a replay.
func (r *ReplayEngine) ReplaySummary(eventID string) (string, error) {
	state, err := r.ReplayTo(eventID, "main")
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(
		"世界时间: 第%d天 %02d:%02d | 场景: %s | 张力: %.2f | 关系数: %d",
		state.Clock.Day, state.Clock.Hour, state.Clock.Minute,
		state.Scene.Location, state.Tension,
		len(state.Relationships),
	), nil
}

// --- helpers ---

func init() {
	// ensure time package is used (for BuildTimeline's CreatedAt.Format)
	_ = time.Now()
}
