package events

import (
	"fmt"
	"testing"
	"time"

	"corerp/internal/core"
)

func newTestReplayStore(t *testing.T) *Store {
	t.Helper()
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	return s
}

func TestReplayTo(t *testing.T) {
	s := newTestReplayStore(t)
	defer s.Close()
	re := NewReplayEngine(s)

	// Seed events
	events := []core.Event{
		{ID: "evt_1", Type: "scene_init", Payload: map[string]interface{}{"location": "起始地"}, Canonical: true, CreatedAt: time.Now()},
		{ID: "evt_2", Type: "scene_change", Payload: map[string]interface{}{"location": "中途"}, Canonical: true, CreatedAt: time.Now().Add(time.Second)},
		{ID: "evt_3", Type: "scene_change", Payload: map[string]interface{}{"location": "终点"}, Canonical: true, CreatedAt: time.Now().Add(2 * time.Second)},
	}
	for _, e := range events {
		s.Append(e)
	}

	// Replay to evt_2 (stop after applying evt_2)
	state, err := re.ReplayTo("evt_2", "main")
	if err != nil {
		t.Fatalf("replay to: %v", err)
	}
	if state.Scene.Location != "中途" {
		t.Errorf("location = %s, want 中途", state.Scene.Location)
	}
}

func TestFork(t *testing.T) {
	s := newTestReplayStore(t)
	defer s.Close()
	re := NewReplayEngine(s)

	base := time.Now()
	s.Append(core.Event{ID: "evt_1", Type: "scene_init", Payload: map[string]interface{}{"location": "Main"}, Canonical: true, CreatedAt: base})
	s.Append(core.Event{ID: "evt_2", Type: "trust_change", Actor: "V", Target: "玩家", Payload: map[string]interface{}{"delta": 1.0}, Canonical: true, CreatedAt: base.Add(time.Second)})

	if err := re.Fork("evt_1", "alt_timeline"); err != nil {
		t.Fatalf("fork: %v", err)
	}
	if err := s.Append(core.Event{
		ID:        "evt_3",
		Type:      "scene_change",
		Payload:   map[string]interface{}{"location": "Alt"},
		Canonical: true,
		Branch:    "alt_timeline",
		CreatedAt: base.Add(2 * time.Second),
	}); err != nil {
		t.Fatalf("append alt event: %v", err)
	}

	branches, err := re.ListBranches()
	if err != nil {
		t.Fatalf("list branches: %v", err)
	}

	hasAlt := false
	hasMain := false
	for _, b := range branches {
		if b == "alt_timeline" {
			hasAlt = true
		}
		if b == "main" {
			hasMain = true
		}
	}
	if !hasAlt {
		t.Error("missing alt_timeline branch")
	}
	if !hasMain {
		t.Error("missing main branch")
	}

	altState, err := re.ReplayTo("evt_3", "alt_timeline")
	if err != nil {
		t.Fatalf("replay alt: %v", err)
	}
	if altState.Scene.Location != "Alt" {
		t.Fatalf("alt location = %q, want Alt", altState.Scene.Location)
	}
	if got := altState.Relationships["V_玩家"].Trust; got != 0 {
		t.Fatalf("alt trust = %.2f, want 0.0 because forked before evt_2", got)
	}
}

func TestGetTimeline(t *testing.T) {
	s := newTestReplayStore(t)
	defer s.Close()
	re := NewReplayEngine(s)

	for i := 0; i < 5; i++ {
		s.Append(core.Event{
			ID:        fmt.Sprintf("evt_%d", i),
			Type:      "dialogue",
			Actor:     "V",
			Canonical: true,
			CreatedAt: time.Now().Add(time.Duration(i) * time.Second),
		})
	}

	tl, err := re.GetTimeline("main", 10)
	if err != nil {
		t.Fatalf("get timeline: %v", err)
	}
	if len(tl) != 5 {
		t.Errorf("expected 5 timeline entries, got %d", len(tl))
	}
}

func TestReplayAtTime(t *testing.T) {
	s := newTestReplayStore(t)
	defer s.Close()
	re := NewReplayEngine(s)

	s.Append(core.Event{
		ID:        "evt_1",
		Type:      "scene_init",
		Payload:   map[string]interface{}{"location": "A"},
		Canonical: true,
		CreatedAt: time.Now(),
	})
	s.Append(core.Event{
		ID:        "evt_2",
		Type:      "clock_advance",
		Payload:   map[string]interface{}{"hour": float64(8), "minute": float64(30), "day": float64(1)},
		Canonical: true,
		CreatedAt: time.Now(),
	})
	s.Append(core.Event{
		ID:        "evt_3",
		Type:      "scene_change",
		Payload:   map[string]interface{}{"location": "B"},
		Canonical: true,
		CreatedAt: time.Now(),
	})

	state, err := re.ReplayAtTime(8, 0, 1)
	if err != nil {
		t.Fatalf("replay at time: %v", err)
	}
	// Should break before or at 8:00 day 1
	if state.Scene.Location != "A" {
		t.Errorf("location = %s, want A (stopped before scene change)", state.Scene.Location)
	}
}

func TestReplayToHonorsBranchIsolation(t *testing.T) {
	s := newTestReplayStore(t)
	defer s.Close()
	re := NewReplayEngine(s)

	base := time.Now()
	events := []core.Event{
		{ID: "main_scene", Type: "scene_init", Payload: map[string]interface{}{"location": "Main"}, Canonical: true, Branch: "main", CreatedAt: base},
		{ID: "main_trust", Type: "trust_change", Actor: "V", Target: "玩家", Payload: map[string]interface{}{"delta": 1.0}, Canonical: true, Branch: "main", CreatedAt: base.Add(time.Second)},
	}
	for _, evt := range events {
		if err := s.Append(evt); err != nil {
			t.Fatalf("append %s: %v", evt.ID, err)
		}
	}
	if err := re.Fork("main_scene", "alt"); err != nil {
		t.Fatalf("fork alt: %v", err)
	}
	for _, evt := range []core.Event{
		{ID: "alt_scene", Type: "scene_change", Payload: map[string]interface{}{"location": "Alt"}, Canonical: true, Branch: "alt", CreatedAt: base.Add(2 * time.Second)},
		{ID: "alt_trust", Type: "trust_change", Actor: "V", Target: "玩家", Payload: map[string]interface{}{"delta": 9.0}, Canonical: true, Branch: "alt", CreatedAt: base.Add(3 * time.Second)},
	} {
		if err := s.Append(evt); err != nil {
			t.Fatalf("append %s: %v", evt.ID, err)
		}
	}

	mainState, err := re.ReplayTo("main_trust", "main")
	if err != nil {
		t.Fatalf("replay main: %v", err)
	}
	if mainState.Scene.Location != "Main" {
		t.Fatalf("main location = %q, want Main", mainState.Scene.Location)
	}
	if got := mainState.Relationships["V_玩家"].Trust; got != 1.0 {
		t.Fatalf("main trust = %.2f, want 1.0", got)
	}

	altState, err := re.ReplayTo("alt_trust", "alt")
	if err != nil {
		t.Fatalf("replay alt: %v", err)
	}
	if altState.Scene.Location != "Alt" {
		t.Fatalf("alt location = %q, want Alt", altState.Scene.Location)
	}
	if got := altState.Relationships["V_玩家"].Trust; got != 9.0 {
		t.Fatalf("alt trust = %.2f, want 9.0", got)
	}
}

func TestReplayToSupportsLegacyIsolatedBranch(t *testing.T) {
	s := newTestReplayStore(t)
	defer s.Close()
	re := NewReplayEngine(s)

	base := time.Now()
	for _, evt := range []core.Event{
		{ID: "main_scene", Type: "scene_init", Payload: map[string]interface{}{"location": "Main"}, Canonical: true, Branch: "main", CreatedAt: base},
		{ID: "legacy_scene", Type: "scene_init", Payload: map[string]interface{}{"location": "Legacy"}, Canonical: true, Branch: "legacy", CreatedAt: base.Add(time.Second)},
	} {
		if err := s.Append(evt); err != nil {
			t.Fatalf("append %s: %v", evt.ID, err)
		}
	}

	state, err := re.ReplayTo("legacy_scene", "legacy")
	if err != nil {
		t.Fatalf("replay legacy branch: %v", err)
	}
	if state.Scene.Location != "Legacy" {
		t.Fatalf("legacy location = %q, want Legacy", state.Scene.Location)
	}
}

func TestBuildTimeline(t *testing.T) {
	events := []core.Event{
		{ID: "e1", Type: "dialogue", Actor: "V", Target: "Jackie", CreatedAt: time.Now()},
		{ID: "e2", Type: "threat", Actor: "V", Target: "enemy", CreatedAt: time.Now()},
		{ID: "e3", Type: "trust_change", Actor: "V", Target: "Jackie", CreatedAt: time.Now()},
	}

	out := BuildTimeline(events, 10)
	if out == "" {
		t.Error("timeline should not be empty")
	}
}

func TestBuildTimelineWithTypeLimit(t *testing.T) {
	events := []core.Event{
		{ID: "e1", Type: "dialogue", Actor: "V", Target: "A", CreatedAt: time.Now()},
		{ID: "e2", Type: "dialogue", Actor: "V", Target: "B", CreatedAt: time.Now()},
		{ID: "e3", Type: "dialogue", Actor: "V", Target: "C", CreatedAt: time.Now()},
	}

	out := BuildTimeline(events, 1)
	if out == "" {
		t.Error("timeline should not be empty")
	}
}
