package actions

import (
	"encoding/json"
	"fmt"
	"time"

	"corerp/internal/core"
)

// Executor runs ActionFrames and produces Events.
type Executor struct{}

func NewExecutor() *Executor {
	return &Executor{}
}

func (ex *Executor) Execute(frame core.ActionFrame, state core.WorldState) ([]core.Event, error) {
	var events []core.Event
	now := time.Now()

	switch frame.Action {
	case "speak", "talk":
		events = append(events, core.Event{
			ID:        fmt.Sprintf("evt_%d", now.UnixNano()),
			Type:      "dialogue",
			Actor:     frame.Actor,
			Target:    frame.Target,
			Payload:   map[string]interface{}{"content": frame.SuggestedLine, "emotion": frame.Emotion},
			Canonical: true,
			CreatedAt: now,
		})

	case "threaten":
		state.Tension += float64(frame.Intensity) / 10
		events = append(events, core.Event{
			ID:        fmt.Sprintf("evt_%d", now.UnixNano()),
			Type:      "threat",
			Actor:     frame.Actor,
			Target:    frame.Target,
			Payload:   map[string]interface{}{"intensity": frame.Intensity, "intent": frame.Intent},
			Canonical: true,
			CreatedAt: now,
		})
		// Emit tension_change event for persistence
		events = append(events, core.Event{
			ID:        fmt.Sprintf("evt_%d_tension", now.UnixNano()),
			Type:      "tension_change",
			Actor:     frame.Actor,
			Payload:   map[string]interface{}{"delta": float64(frame.Intensity) / 10},
			Canonical: true,
			CreatedAt: now,
		})
		if frame.Target != "" {
			events = append(events, core.Event{
				ID:        fmt.Sprintf("evt_%d_fear", now.UnixNano()),
				Type:      "fear_change",
				Actor:     frame.Actor,
				Target:    frame.Target,
				Payload:   map[string]interface{}{"delta": float64(frame.Intensity) / 5},
				Canonical: true,
				CreatedAt: now,
			})
		}

	case "trust", "trust_building":
		if frame.Target != "" {
			events = append(events, core.Event{
				ID:        fmt.Sprintf("evt_%d_trust", now.UnixNano()),
				Type:      "trust_change",
				Actor:     frame.Actor,
				Target:    frame.Target,
				Payload:   map[string]interface{}{"delta": 0.5},
				Canonical: true,
				CreatedAt: now,
			})
		}

	case "attack", "fight":
		events = append(events, core.Event{
			ID:        fmt.Sprintf("evt_%d", now.UnixNano()),
			Type:      "attack",
			Actor:     frame.Actor,
			Target:    frame.Target,
			Payload:   map[string]interface{}{"intensity": frame.Intensity},
			Canonical: true,
			CreatedAt: now,
		})
		state.Tension += 1.0
		// Emit tension_change event for persistence
		events = append(events, core.Event{
			ID:        fmt.Sprintf("evt_%d_tension", now.UnixNano()),
			Type:      "tension_change",
			Actor:     frame.Actor,
			Payload:   map[string]interface{}{"delta": 1.0},
			Canonical: true,
			CreatedAt: now,
		})

	case "hide", "conceal":
		events = append(events, core.Event{
			ID:        fmt.Sprintf("evt_%d", now.UnixNano()),
			Type:      "hide",
			Actor:     frame.Actor,
			Payload:   map[string]interface{}{"location": state.Scene.Location},
			Canonical: true,
			CreatedAt: now,
		})

	case "move", "go":
		if len(frame.Effects) > 0 {
			loc := frame.Effects[0].Path
			// Path contains the destination in P1 simplified form
			events = append(events, core.Event{
				ID:        fmt.Sprintf("evt_%d", now.UnixNano()),
				Type:      "scene_change",
				Actor:     frame.Actor,
				Payload:   map[string]interface{}{"location": loc},
				Canonical: true,
				CreatedAt: now,
			})
		}

	case "debt_acknowledge":
		events = append(events, core.Event{
			ID:        fmt.Sprintf("evt_%d", now.UnixNano()),
			Type:      "debt_ack",
			Actor:     frame.Actor,
			Target:    frame.Target,
			Payload:   map[string]interface{}{"acknowledged": true},
			Effects:   []core.StateEffect{{Path: fmt.Sprintf("relationships.%s_%s.debt", frame.Actor, frame.Target), Delta: 1}},
			Canonical: true,
			CreatedAt: now,
		})

	case "negotiate", "bargain":
		events = append(events, core.Event{
			ID:        fmt.Sprintf("evt_%d", now.UnixNano()),
			Type:      "negotiation",
			Actor:     frame.Actor,
			Target:    frame.Target,
			Payload:   map[string]interface{}{"intent": frame.Intent},
			Canonical: true,
			CreatedAt: now,
		})

	default:
		// Generic action: just record it
		events = append(events, core.Event{
			ID:        fmt.Sprintf("evt_%d", now.UnixNano()),
			Type:      frame.Action,
			Actor:     frame.Actor,
			Target:    frame.Target,
			Payload:   map[string]interface{}{"intent": frame.Intent, "intensity": frame.Intensity, "emotion": frame.Emotion},
			Canonical: false, // Quarantine for unknown actions
			CreatedAt: now,
		})
	}

	// Apply effects from the frame
	for _, eff := range frame.Effects {
		events = append(events, core.Event{
			ID:        fmt.Sprintf("evt_%d_eff", now.UnixNano()),
			Type:      "variable_set",
			Actor:     frame.Actor,
			Payload:   map[string]interface{}{"key": eff.Path, "value": eff.Delta},
			Canonical: true,
			CreatedAt: now,
		})
	}

	return events, nil
}

// ParseActionFrame parses LLM output into an ActionFrame.
// P1: expects JSON output from LLM.
func ParseActionFrame(raw string) (core.ActionFrame, error) {
	var frame core.ActionFrame
	if err := json.Unmarshal([]byte(raw), &frame); err != nil {
		return core.ActionFrame{}, fmt.Errorf("failed to parse action frame: %w", err)
	}
	return frame, nil
}

// AllowedActionsFor returns a list of action types available in current state.
func AllowedActionsFor(state core.WorldState, goals []core.Goal) []string {
	base := []string{"speak", "trust", "negotiate", "hide", "move"}

	if state.Tension > 0.5 {
		base = append(base, "threaten", "attack")
	}

	// Goal-based restrictions
	for _, g := range goals {
		if g.ID == "survive" && g.Priority >= 8 {
			// High survival priority: remove risky actions
			base = filterOut(base, []string{"attack"})
		}
	}

	return base
}

func filterOut(actions, forbidden []string) []string {
	var result []string
	for _, a := range actions {
		found := false
		for _, f := range forbidden {
			if a == f {
				found = true
				break
			}
		}
		if !found {
			result = append(result, a)
		}
	}
	return result
}
