package narrative

import (
	"fmt"
	"time"

	"corerp/internal/core"
)

// TensionEngine detects narrative heat death and injects pressure.
type TensionEngine struct {
	lastConflictTurn   int
	lastEmotionChange  time.Time
	heatDeathThreshold int // turns without conflict before heat death
}

func NewTensionEngine() *TensionEngine {
	return &TensionEngine{
		lastConflictTurn:   0,
		lastEmotionChange:  time.Now(),
		heatDeathThreshold: 8,
	}
}

// Tick analyzes world state and returns pressure events if heat death detected.
func (te *TensionEngine) Tick(state core.WorldState, turnCount int) []core.Event {
	var events []core.Event

	// Detect conflict events that reset heat death timer
	if state.Tension > 0.3 {
		te.lastConflictTurn = turnCount
	}

	// Check heat death: too many calm turns
	turnsSinceConflict := turnCount - te.lastConflictTurn
	if turnsSinceConflict >= te.heatDeathThreshold {
		// Inject pressure event
		events = append(events, core.Event{
			ID:        fmt.Sprintf("evt_pressure_%d", time.Now().UnixNano()),
			Type:      "tension_change",
			Actor:     "system",
			Payload:   map[string]interface{}{"delta": 0.2, "reason": "heat_death_detected"},
			Canonical: true,
			Tag:       core.TagTick,
			CreatedAt: time.Now(),
		})
		// Reset counter after injection
		te.lastConflictTurn = turnCount
	}

	// Passive tension decay: tension naturally drifts toward 0 over time
	if state.Tension > 0 {
		events = append(events, core.Event{
			ID:        fmt.Sprintf("evt_tension_decay_%d", time.Now().UnixNano()),
			Type:      "tension_change",
			Actor:     "system",
			Payload:   map[string]interface{}{"delta": -0.05, "reason": "natural_decay"},
			Canonical: true,
			Tag:       core.TagTick,
			CreatedAt: time.Now(),
		})
	}

	return events
}

// ResetConflictTimer should be called when a conflict-type action occurs.
func (te *TensionEngine) ResetConflictTimer(turnCount int) {
	te.lastConflictTurn = turnCount
	te.lastEmotionChange = time.Now()
}

// IsHeatDeath reports whether the narrative has gone cold.
func (te *TensionEngine) IsHeatDeath(turnCount int) bool {
	return turnCount-te.lastConflictTurn >= te.heatDeathThreshold
}
