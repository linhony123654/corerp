package events

import (
	"database/sql"
	"fmt"

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

// ReplayTo reconstructs WorldState by replaying the effective branch lineage
// up to and including the given event.
func (r *ReplayEngine) ReplayTo(eventID string, branch string) (core.WorldState, error) {
	events, err := r.lineageEvents(branch)
	if err != nil {
		return core.WorldState{}, err
	}

	state := emptyWorldState()
	found := false
	for _, e := range events {
		state = applyEvent(state, e)
		if e.ID == eventID {
			found = true
			break
		}
	}
	if eventID != "" && !found {
		return core.WorldState{}, fmt.Errorf("event '%s' not found on branch '%s'", eventID, normalizeBranch(branch))
	}
	return state, nil
}

// ReplayAtTime reconstructs WorldState by replaying events on the effective
// branch lineage up to a specific world clock time.
func (r *ReplayEngine) ReplayAtTime(hour, minute, day int) (core.WorldState, error) {
	events, err := r.lineageEvents("main")
	if err != nil {
		return core.WorldState{}, err
	}

	state := emptyWorldState()
	for _, e := range events {
		state = applyEvent(state, e)

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

// Fork creates a branch metadata record instead of mutating existing events.
func (r *ReplayEngine) Fork(eventID string, branchName string) error {
	evt, err := r.store.GetByID(eventID)
	if err != nil {
		return err
	}
	parent := normalizeBranch(evt.Branch)
	return r.store.CreateBranch(branchName, parent, eventID)
}

// GetBranch returns all events authored directly on a specific branch.
func (r *ReplayEngine) GetBranch(branchName string) ([]core.Event, error) {
	branchName = normalizeBranch(branchName)
	rows, err := r.store.db.Query(
		`SELECT id, type, actor, target, payload, causes, effects, canonical, confidence, confirmations, scene_id, session_id, branch, created_at
		 FROM events WHERE branch = ?`+r.store.instanceScopeSuffix(" AND ")+` ORDER BY created_at ASC`, func() []interface{} {
			_, args := r.store.instanceScopeArgs(branchName)
			return args
		}()...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEvents(rows)
}

// ListBranches returns all known branch names.
func (r *ReplayEngine) ListBranches() ([]string, error) {
	records, err := r.store.ListBranchesMetadata()
	if err != nil {
		return nil, err
	}
	branches := make([]string, 0, len(records))
	for _, rec := range records {
		branches = append(branches, rec.Name)
	}
	return branches, nil
}

// EventTimeline represents a point in the event timeline for rendering.
type EventTimeline struct {
	Event      core.Event `json:"event"`
	Branch     string     `json:"branch"`
	EventIndex int        `json:"index"`
}

// GetTimeline returns the effective canonical timeline for a branch, including
// inherited ancestor events up to the fork point.
func (r *ReplayEngine) GetTimeline(branch string, limit int) ([]EventTimeline, error) {
	events, err := r.lineageEvents(branch)
	if err != nil {
		return nil, err
	}
	if limit > 0 && len(events) > limit {
		events = events[:limit]
	}

	timeline := make([]EventTimeline, 0, len(events))
	for i, e := range events {
		timeline = append(timeline, EventTimeline{
			Event:      e,
			Branch:     e.Branch,
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

	timelineA, err := r.GetTimeline(branchA, 1000000)
	if err != nil {
		return nil, fmt.Errorf("branch %s: %w", branchA, err)
	}
	timelineB, err := r.GetTimeline(branchB, 1000000)
	if err != nil {
		return nil, fmt.Errorf("branch %s: %w", branchB, err)
	}

	if atEventIndex >= 0 {
		if atEventIndex+1 < len(timelineA) {
			timelineA = timelineA[:atEventIndex+1]
		}
		if atEventIndex+1 < len(timelineB) {
			timelineB = timelineB[:atEventIndex+1]
		}
	}

	stateA := emptyWorldState()
	stateB := emptyWorldState()
	for _, t := range timelineA {
		stateA = applyEvent(stateA, t.Event)
	}
	for _, t := range timelineB {
		stateB = applyEvent(stateB, t.Event)
	}

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

func (r *ReplayEngine) lineageEvents(branch string) ([]core.Event, error) {
	branch = normalizeBranch(branch)
	records, err := r.branchLineage(branch)
	if err != nil {
		return nil, err
	}

	var all []core.Event
	for i, rec := range records {
		events, err := r.store.GetCanonicalEventsByBranch(rec.Name)
		if err != nil {
			return nil, err
		}
		if i < len(records)-1 && rec.ForkEventID != "" {
			cut := indexOfEvent(events, rec.ForkEventID)
			if cut < 0 {
				return nil, fmt.Errorf("fork event '%s' not found on branch '%s'", rec.ForkEventID, rec.Name)
			}
			events = events[:cut+1]
		}
		all = append(all, events...)
	}
	return all, nil
}

func (r *ReplayEngine) branchLineage(branch string) ([]branchRecord, error) {
	var chain []branchRecord
	seen := map[string]bool{}
	current := normalizeBranch(branch)
	for {
		if seen[current] {
			return nil, fmt.Errorf("branch cycle detected at '%s'", current)
		}
		seen[current] = true

		rec, err := r.store.GetBranch(current)
		if err != nil {
			if err == sql.ErrNoRows {
				if current == "main" {
					return []branchRecord{{Name: "main"}}, nil
				}
				return []branchRecord{{Name: current}}, nil
			}
			return nil, err
		}
		chain = append([]branchRecord{rec}, chain...)
		if rec.ParentBranch == "" {
			break
		}
		current = rec.ParentBranch
	}

	for i := 1; i < len(chain); i++ {
		chain[i-1].ForkEventID = chain[i].ForkEventID
	}
	return chain, nil
}

func emptyWorldState() core.WorldState {
	return core.WorldState{
		Relationships: make(map[string]core.Relationship),
		Variables:     make(map[string]interface{}),
		Flags:         make(map[string]bool),
	}
}

func normalizeBranch(branch string) string {
	if branch == "" {
		return "main"
	}
	return branch
}

func indexOfEvent(events []core.Event, eventID string) int {
	for i, e := range events {
		if e.ID == eventID {
			return i
		}
	}
	return -1
}
