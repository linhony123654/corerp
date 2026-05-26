package narrative

import (
	"testing"

	"corerp/internal/core"
)

func TestNewTensionEngine(t *testing.T) {
	te := NewTensionEngine()
	if te.heatDeathThreshold != 8 {
		t.Errorf("threshold = %d, want 8", te.heatDeathThreshold)
	}
	if te.IsHeatDeath(0) {
		t.Error("should not detect heat death at turn 0")
	}
}

func TestTensionEngineResetConflictTimer(t *testing.T) {
	te := NewTensionEngine()
	te.ResetConflictTimer(5)
	if te.lastConflictTurn != 5 {
		t.Errorf("lastConflictTurn = %d, want 5", te.lastConflictTurn)
	}
}

func TestTensionEngineHeatDeathDetection(t *testing.T) {
	te := NewTensionEngine()

	// Turn 0-6: no conflict
	for i := 0; i < 7; i++ {
		if te.IsHeatDeath(i) {
			t.Errorf("heat death at turn %d (threshold=8)", i)
		}
	}

	// Turn 8+ should trigger
	if !te.IsHeatDeath(8) {
		t.Error("heat death should be detected at turn 8")
	}
	if !te.IsHeatDeath(15) {
		t.Error("heat death should persist beyond threshold")
	}
}

func TestTensionEngineTickPressureInjection(t *testing.T) {
	te := NewTensionEngine()
	state := core.WorldState{Tension: 0.05} // low tension, no conflict

	// Simulate many calm turns
	for i := 0; i < 9; i++ {
		events := te.Tick(state, i)
		if i >= 8 {
			// Should inject pressure event
			foundPressure := false
			for _, e := range events {
				if e.Type == "tension_change" && e.Actor == "system" {
					foundPressure = true
					reason, ok := e.Payload["reason"].(string)
					if ok && reason == "heat_death_detected" {
						break
					}
				}
			}
			if !foundPressure {
				t.Errorf("expected pressure injection at turn %d", i)
			}
		}
	}
}

func TestTensionEngineNaturalDecay(t *testing.T) {
	te := NewTensionEngine()
	state := core.WorldState{Tension: 0.5}

	events := te.Tick(state, 0)

	hasDecay := false
	hasPressure := false
	for _, e := range events {
		if e.Type == "tension_change" {
			if delta, ok := e.Payload["delta"].(float64); ok {
				if delta < 0 {
					hasDecay = true
				}
				if reason, ok := e.Payload["reason"].(string); ok && reason == "heat_death_detected" {
					hasPressure = true
				}
			}
		}
	}
	if !hasDecay {
		t.Error("expected natural decay for tension > 0")
	}
	if hasPressure {
		t.Error("should not trigger heat death at turn 0")
	}
}

func TestTensionEngineConflictResetsTimer(t *testing.T) {
	te := NewTensionEngine()
	state := core.WorldState{Tension: 0.5} // above 0.3 → conflict detected

	// Tick with high tension → resets conflict timer to current turn
	te.Tick(state, 3)
	if te.lastConflictTurn != 3 {
		t.Errorf("conflict timer should reset to turn 3, got %d", te.lastConflictTurn)
	}

	// At turn 10: 10-3=7 < 8 → no heat death
	if te.IsHeatDeath(10) {
		t.Error("should not detect heat death at turn 10 (7 turns since conflict)")
	}
	// At turn 11: 11-3=8 >= 8 → heat death
	if !te.IsHeatDeath(11) {
		t.Error("should detect heat death at turn 11 (8 turns since conflict)")
	}
}

func TestTensionEngineZeroTensionNoDecay(t *testing.T) {
	te := NewTensionEngine()
	state := core.WorldState{Tension: 0} // exactly zero

	events := te.Tick(state, 0)

	for _, e := range events {
		if delta, ok := e.Payload["delta"].(float64); ok && delta < 0 {
			t.Error("should not decay when tension is zero")
		}
	}
}

func TestTensionEngineTickEventCount(t *testing.T) {
	te := NewTensionEngine()

	// Zero tension, early turn → no events
	events := te.Tick(core.WorldState{Tension: 0}, 0)
	if len(events) != 0 {
		t.Errorf("expected 0 events for zero tension, got %d", len(events))
	}

	// Positive tension, early turn → only decay
	events = te.Tick(core.WorldState{Tension: 0.5}, 0)
	if len(events) != 1 {
		t.Errorf("expected 1 event (decay), got %d", len(events))
	}

	// Heat death turn → decay + pressure
	// Need to set conflict timer back for the decay+during_threshold situation
	te2 := NewTensionEngine()
	events = te2.Tick(core.WorldState{Tension: 0.1}, 8)
	if len(events) != 2 {
		t.Errorf("expected 2 events (decay + pressure at heat death), got %d", len(events))
	}
}
