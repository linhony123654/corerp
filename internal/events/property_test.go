package events

import (
	"fmt"
	"testing"
	"testing/quick"

	"corerp/internal/core"
)

// === Property-Based Tests using testing/quick ===
// All functions use simple types ([]float64) that testing/quick can generate.
// core.Event has unexported time.Time fields so cannot be auto-generated.

// TestPropertyReplayDeterministic: any sequence of tension deltas,
// projected twice, produces the same hash.
func TestPropertyReplayDeterministic(t *testing.T) {
	fn := func(tensionDeltas []float64) bool {
		events := deltasToTensionEvents(tensionDeltas)
		h1 := CanonicalHashV1(Project(events))
		h2 := CanonicalHashV1(Project(events))
		return h1 == h2
	}
	if err := quick.Check(fn, &quick.Config{MaxCount: 200}); err != nil {
		t.Fatal(err)
	}
}

// TestPropertyHashMapOrder: same deltas in different insertion order
// into event payload produce same hash.
func TestPropertyHashMapOrder(t *testing.T) {
	fn := func(delta float64) bool {
		if delta > 10 {
			delta = 10
		}
		if delta < -10 {
			delta = -10
		}
		e1 := core.Event{
			ID:        "e",
			Type:      "tension_change",
			Payload:   map[string]interface{}{"delta": delta, "reason": "test", "source": "quick"},
			Canonical: true,
		}
		e2 := core.Event{
			ID:        "e",
			Type:      "tension_change",
			Payload:   map[string]interface{}{"source": "quick", "reason": "test", "delta": delta},
			Canonical: true,
		}
		h1 := CanonicalHashV1(Project([]core.Event{e1}))
		h2 := CanonicalHashV1(Project([]core.Event{e2}))
		return h1 == h2
	}
	if err := quick.Check(fn, &quick.Config{MaxCount: 200}); err != nil {
		t.Fatal(err)
	}
}

// TestPropertyTensionIsDeterministic: same deltas always produce same tension.
func TestPropertyTensionIsDeterministic(t *testing.T) {
	fn := func(deltas []float64) bool {
		events := deltasToTensionEvents(deltas)
		s1 := Project(events)
		s2 := Project(events)
		return s1.Tension == s2.Tension
	}
	if err := quick.Check(fn, &quick.Config{MaxCount: 200}); err != nil {
		t.Fatal(err)
	}
}

// TestPropertyProjectionIsPure: no side effects from Project().
func TestPropertyProjectionIsPure(t *testing.T) {
	fn := func(deltas []float64) bool {
		events := deltasToTensionEvents(deltas)
		s1 := Project(events)
		_ = Project(events)
		// Project() must not mutate its input events slice
		for i := range events {
			if !events[i].Canonical {
				return false
			}
		}
		_ = s1
		return true
	}
	if err := quick.Check(fn, &quick.Config{MaxCount: 200}); err != nil {
		t.Fatal(err)
	}
}

// TestPropertyForkCleanMainBranch proves fork isolation with deterministic verification.
func TestPropertyForkCleanMainBranch(t *testing.T) {
	base := makeTestEventStream()
	baseHash := CanonicalHashV1(Project(base))

	for seed := int64(0); seed < 200; seed++ {
		altEvents := make([]core.Event, len(base))
		copy(altEvents, base)
		altEvents = append(altEvents, core.Event{
			ID:        fmt.Sprintf("alt_%d", seed),
			Type:      "tension_change",
			Payload:   map[string]interface{}{"delta": float64(seed%10) / 10.0},
			Canonical: true,
		})

		if CanonicalHashV1(Project(base)) != baseHash {
			t.Fatalf("MAIN BRANCH CONTAMINATED at seed %d", seed)
		}

		// Alt branch must differ from main for non-zero delta
		if seed%10 != 0 {
			altHash := CanonicalHashV1(Project(altEvents))
			if altHash == baseHash {
				t.Errorf("alt branch should differ from main at seed %d (delta=%.1f)", seed, float64(seed%10)/10.0)
			}
		}
	}
}

// TestPropertyReplayDeterministicWithTrust: combined trust + tension must be deterministic.
func TestPropertyReplayDeterministicWithTrust(t *testing.T) {
	fn := func(tensionDeltas, trustDeltas []float64) bool {
		events := deltasToTensionEvents(tensionDeltas)
		for i, d := range trustDeltas {
			if d > 10 {
				d = 10
			}
			if d < -10 {
				d = -10
			}
			events = append(events, core.Event{
				ID:        fmt.Sprintf("t%d", i),
				Type:      "trust_change",
				Actor:     "A",
				Target:    "B",
				Payload:   map[string]interface{}{"delta": d},
				Canonical: true,
			})
		}
		h1 := CanonicalHashV1(Project(events))
		h2 := CanonicalHashV1(Project(events))
		return h1 == h2
	}
	if err := quick.Check(fn, &quick.Config{MaxCount: 100}); err != nil {
		t.Fatal(err)
	}
}

// === helpers for property tests ===

func deltasToTensionEvents(deltas []float64) []core.Event {
	events := make([]core.Event, len(deltas))
	for i, d := range deltas {
		if d > 10 {
			d = 10
		}
		if d < -10 {
			d = -10
		}
		events[i] = core.Event{
			ID:        fmt.Sprintf("pe%d", i),
			Type:      "tension_change",
			Payload:   map[string]interface{}{"delta": d},
			Canonical: true,
		}
	}
	return events
}
