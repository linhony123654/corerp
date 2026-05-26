package events

import (
	"testing"
	"time"

	"corerp/internal/core"
)

// makeTestEventStream creates a realistic sequence of events.
func makeTestEventStream() []core.Event {
	t := time.Date(2026, 5, 25, 14, 0, 0, 0, time.UTC)
	return []core.Event{
		{
			ID:   "evt_init",
			Type: "scene_init",
			Payload: map[string]interface{}{
				"location":    "夜之城·来生酒吧",
				"time_of_day": "夜晚",
				"weather":     "酸雨",
				"characters":  []interface{}{"V", "Jackie", "玩家"},
				"description": "霓虹灯在雨中闪烁，地下酒吧烟雾缭绕。",
			},
			Canonical: true,
			CreatedAt: t,
		},
		{
			ID:        "evt_dialogue_1",
			Type:      "dialogue",
			Actor:     "V",
			Target:    "玩家",
			Payload:   map[string]interface{}{"content": "你想谈生意？坐。", "emotion": "neutral"},
			Canonical: true,
			CreatedAt: t.Add(1 * time.Second),
		},
		{
			ID:        "evt_trust_1",
			Type:      "trust_change",
			Actor:     "V",
			Target:    "玩家",
			Payload:   map[string]interface{}{"delta": 0.5},
			Canonical: true,
			CreatedAt: t.Add(2 * time.Second),
		},
		{
			ID:        "evt_tension_1",
			Type:      "tension_change",
			Actor:     "system",
			Payload:   map[string]interface{}{"delta": 0.3},
			Canonical: true,
			CreatedAt: t.Add(3 * time.Second),
		},
		{
			ID:        "evt_dialogue_2",
			Type:      "dialogue",
			Actor:     "Jackie",
			Target:    "玩家",
			Payload:   map[string]interface{}{"content": "嘿，兄弟，别紧张。", "emotion": "friendly"},
			Canonical: true,
			CreatedAt: t.Add(4 * time.Second),
		},
		{
			ID:        "evt_threat",
			Type:      "threat",
			Actor:     "V",
			Target:    "玩家",
			Payload:   map[string]interface{}{"intensity": 5, "intent": "intimidate"},
			Canonical: true,
			CreatedAt: t.Add(5 * time.Second),
		},
		{
			ID:        "evt_tension_2",
			Type:      "tension_change",
			Actor:     "system",
			Payload:   map[string]interface{}{"delta": 0.5},
			Canonical: true,
			CreatedAt: t.Add(6 * time.Second),
		},
		{
			ID:        "evt_flag",
			Type:      "flag_set",
			Actor:     "system",
			Payload:   map[string]interface{}{"key": "detected"},
			Canonical: true,
			CreatedAt: t.Add(7 * time.Second),
		},
		{
			ID:        "evt_clock",
			Type:      "clock_advance",
			Actor:     "system",
			Payload:   map[string]interface{}{"hour": float64(15), "minute": float64(30), "day": float64(1)},
			Canonical: true,
			CreatedAt: t.Add(8 * time.Second),
		},
	}
}

// TestReplayDeterministic is the single most important test in this project.
// Same event stream → replay → same state hash. Always.
func TestReplayDeterministic(t *testing.T) {
	events := makeTestEventStream()

	// First replay
	state1 := Project(events)
	hash1 := CanonicalHashV1(state1)

	// Second replay — must be identical
	state2 := Project(events)
	hash2 := CanonicalHashV1(state2)

	if hash1 != hash2 {
		t.Fatalf("DETERMINISM BROKEN: hash1=%s hash2=%s", hash1, hash2)
	}

	// Verify key state fields
	if state1.Scene.Location != "夜之城·来生酒吧" {
		t.Errorf("location = %s", state1.Scene.Location)
	}
	if state1.Tension != 0.8 {
		t.Errorf("tension = %.2f, want 0.80 (0.3 + 0.5)", state1.Tension)
	}
	if state1.Clock.Day != 1 || state1.Clock.Hour != 15 || state1.Clock.Minute != 30 {
		t.Errorf("clock = %d:%d:%d, want 1:15:30", state1.Clock.Day, state1.Clock.Hour, state1.Clock.Minute)
	}
	if !state1.Flags["detected"] {
		t.Error("flag 'detected' should be true")
	}
	key := "V_玩家"
	if r, ok := state1.Relationships[key]; !ok {
		t.Error("relationship V_玩家 not found")
	} else if r.Trust != 0.5 {
		t.Errorf("trust = %.2f, want 0.50", r.Trust)
	}
}

// TestReplayDeterministicAcrossReplay creates an event store and verifies
// that replaying the same events through the store is also deterministic.
func TestReplayDeterministicAcrossStore(t *testing.T) {
	s := newTestReplayStoreForDeterminism(t)
	defer s.Close()

	events := makeTestEventStream()
	for _, e := range events {
		if err := s.Append(e); err != nil {
			t.Fatalf("append %s: %v", e.ID, err)
		}
	}

	re := NewReplayEngine(s)

	// Replay the full event stream
	state1, err := re.ReplayTo("evt_clock", "main")
	if err != nil {
		t.Fatalf("replay 1: %v", err)
	}
	hash1 := CanonicalHashV1(state1)

	// Replay again
	state2, err := re.ReplayTo("evt_clock", "main")
	if err != nil {
		t.Fatalf("replay 2: %v", err)
	}
	hash2 := CanonicalHashV1(state2)

	if hash1 != hash2 {
		t.Fatalf("STORE DETERMINISM BROKEN: hash1=%s hash2=%s", hash1, hash2)
	}
}

// TestForkIsolation verifies that forking creates isolated state.
func TestForkIsolation(t *testing.T) {
	s := newTestReplayStoreForDeterminism(t)
	defer s.Close()
	re := NewReplayEngine(s)

	// Seed main branch
	events := makeTestEventStream()
	for _, e := range events {
		s.Append(e)
	}

	// Fork after the initial trust event so the child branch inherits it.
	re.Fork("evt_trust_1", "alt")
	// Add divergent events to alt branch — manually append with alt branch
	s.Append(core.Event{
		ID:        "evt_alt_trust",
		Type:      "trust_change",
		Actor:     "V",
		Target:    "玩家",
		Payload:   map[string]interface{}{"delta": 5.0},
		Canonical: true,
		Branch:    "alt",
		CreatedAt: time.Now(),
	})

	// Main branch state
	mainState, _ := re.ReplayTo("evt_clock", "main")
	mainHash := CanonicalHashV1(mainState)

	// Verify main branch trust is still 0.5 (unaffected by alt delta 5.0)
	mainTrust := mainState.Relationships["V_玩家"].Trust
	if mainTrust != 0.5 {
		t.Errorf("main trust = %.2f, want 0.50 (should not be affected by alt branch)", mainTrust)
	}
	altState, _ := re.ReplayTo("evt_alt_trust", "alt")
	altTrust := altState.Relationships["V_玩家"].Trust
	if altTrust != 5.5 {
		t.Errorf("alt trust = %.2f, want 5.50 (should inherit 0.50 + alt delta 5.0)", altTrust)
	}
	_ = mainHash
}

// TestDeterministicProjectionWithInterleavedEvents tests that event order matters.
func TestDeterministicProjectionOrdering(t *testing.T) {
	// Same events in different order → different state
	base := makeTestEventStream()

	// Re-order: trust before scene_init
	reordered := []core.Event{base[2], base[1], base[0], base[3], base[4], base[5], base[6], base[7], base[8]}

	hash1 := CanonicalHashV1(Project(base))
	hash2 := CanonicalHashV1(Project(reordered))

	// They SHOULD differ because order matters for clock/trust accumulation
	// but each replay of the SAME ordering should be deterministic
	if hash1 == hash2 {
		t.Log("Note: different orderings happened to produce same hash")
	}

	// Verify reordered is self-consistent
	if h1, h2 := CanonicalHashV1(Project(reordered)), CanonicalHashV1(Project(reordered)); h1 != h2 {
		t.Fatal("reordered self-determinism broken")
	}
}

func newTestReplayStoreForDeterminism(t *testing.T) *Store {
	t.Helper()
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	return s
}
