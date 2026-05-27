package actions

import (
	"encoding/json"
	"fmt"
	"time"

	"corerp/internal/agents"
	"corerp/internal/core"
)

// Executor runs ActionFrames and produces Events.
type Executor struct{}

var _ agents.ActionExecutor = (*Executor)(nil)

func NewExecutor() *Executor {
	return &Executor{}
}

// newEvent is a helper that creates events tagged as narrative.
func newEvent(eventType, actor, target, idSuffix string, payload map[string]interface{}, canonical bool) core.Event {
	id := fmt.Sprintf("evt_%d", time.Now().UnixNano())
	if idSuffix != "" {
		id += "_" + idSuffix
	}
	return core.Event{
		ID:        id,
		Type:      eventType,
		Actor:     actor,
		Target:    target,
		Payload:   payload,
		Canonical: canonical,
		Tag:       core.TagNarrative,
		CreatedAt: time.Now(),
	}
}

func (ex *Executor) Execute(frame core.ActionFrame, state core.WorldState) ([]core.Event, error) {
	var events []core.Event

	switch frame.Action {
	case "speak", "talk":
		events = append(events, newEvent("dialogue", frame.Actor, frame.Target, "",
			map[string]interface{}{"content": frame.SuggestedLine, "emotion": frame.Emotion}, true))

	case "threaten":
		state.Tension += float64(frame.Intensity) / 10
		events = append(events, newEvent("threat", frame.Actor, frame.Target, "",
			map[string]interface{}{"intensity": frame.Intensity, "intent": frame.Intent}, true))
		events = append(events, newEvent("tension_change", frame.Actor, "", "tension",
			map[string]interface{}{"delta": float64(frame.Intensity) / 10}, true))
		if frame.Target != "" {
			events = append(events, newEvent("fear_change", frame.Actor, frame.Target, "fear",
				map[string]interface{}{"delta": float64(frame.Intensity) / 5}, true))
		}

	case "trust", "trust_building":
		if frame.Target != "" {
			events = append(events, newEvent("trust_change", frame.Actor, frame.Target, "trust",
				map[string]interface{}{"delta": 0.5}, true))
		}

	case "attack", "fight":
		state.Tension += 1.0
		events = append(events, newEvent("attack", frame.Actor, frame.Target, "",
			map[string]interface{}{"intensity": frame.Intensity}, true))
		events = append(events, newEvent("tension_change", frame.Actor, "", "tension",
			map[string]interface{}{"delta": 1.0}, true))

	case "hide", "conceal":
		events = append(events, newEvent("hide", frame.Actor, "", "",
			map[string]interface{}{"location": state.Scene.Location}, true))

	case "move", "go":
		if len(frame.Effects) > 0 {
			loc := frame.Effects[0].Path
			events = append(events, newEvent("scene_change", frame.Actor, "", "",
				map[string]interface{}{"location": loc}, true))
		}

	case "debt_acknowledge":
		evt := newEvent("debt_ack", frame.Actor, frame.Target, "",
			map[string]interface{}{"acknowledged": true}, true)
		evt.Effects = []core.StateEffect{{Path: fmt.Sprintf("relationships.%s_%s.debt", frame.Actor, frame.Target), Delta: 1}}
		events = append(events, evt)

	case "negotiate", "bargain":
		events = append(events, newEvent("negotiation", frame.Actor, frame.Target, "",
			map[string]interface{}{"intent": frame.Intent}, true))

	default:
		events = append(events, newEvent(frame.Action, frame.Actor, frame.Target, "",
			map[string]interface{}{"intent": frame.Intent, "intensity": frame.Intensity, "emotion": frame.Emotion}, false))
	}

	// Apply effects from the frame
	for _, eff := range frame.Effects {
		events = append(events, newEvent("variable_set", frame.Actor, "", "eff",
			map[string]interface{}{"key": eff.Path, "value": eff.Delta}, true))
	}

	return events, nil
}

// ParseActionFrame parses LLM output into an ActionFrame.
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

	for _, g := range goals {
		if g.ID == "survive" && g.Priority >= 8 {
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
