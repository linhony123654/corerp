package state

import (
	"corerp/internal/core"
)

// NarrativeState constrains the narrative space.
type NarrativeState string

const (
	StateCalm       NarrativeState = "calm"
	StateTense      NarrativeState = "tense"
	StateCrisis     NarrativeState = "crisis"
	StateResolution NarrativeState = "resolution"
)

// StateMachine drives narrative state transitions based on world tension.
type StateMachine struct {
	current   NarrativeState
	prev      NarrativeState
	history   []StateTransition
}

type StateTransition struct {
	From   NarrativeState
	To     NarrativeState
	Reason string
}

func NewStateMachine() *StateMachine {
	return &StateMachine{
		current: StateCalm,
	}
}

// Transition evaluates tension and updates state.
func (sm *StateMachine) Transition(tension float64, trigger string) NarrativeState {
	sm.prev = sm.current

	switch sm.current {
	case StateCalm:
		if tension >= 0.7 {
			sm.current = StateCrisis
		} else if tension >= 0.3 {
			sm.current = StateTense
		}
	case StateTense:
		if tension >= 0.7 {
			sm.current = StateCrisis
		} else if tension < 0.2 {
			sm.current = StateCalm
		}
	case StateCrisis:
		if tension < 0.4 {
			sm.current = StateResolution
		}
	case StateResolution:
		if tension >= 0.5 {
			sm.current = StateTense
		} else if tension < 0.2 {
			sm.current = StateCalm
		}
	}

	if sm.current != sm.prev {
		sm.history = append(sm.history, StateTransition{
			From:   sm.prev,
			To:     sm.current,
			Reason: trigger,
		})
	}

	return sm.current
}

// Current returns the active narrative state.
func (sm *StateMachine) Current() NarrativeState {
	return sm.current
}

// AllowedActions returns action list filtered by narrative state.
func (sm *StateMachine) AllowedActions(base []string) []string {
	switch sm.current {
	case StateCalm:
		// In calm: remove violent actions, add peaceful ones
		return filter(base, []string{"attack"}, []string{"observe", "wait"})
	case StateTense:
		// In tense: all base actions available
		return base
	case StateCrisis:
		// In crisis: add desperate actions
		return append(base, "flee", "surrender", "desperate_act")
	case StateResolution:
		// In resolution: focus on recovery
		return filter(base, []string{"attack", "threaten"}, []string{"recover", "reflect"})
	}
	return base
}

// filter removes forbidden and appends suggested actions.
func filter(actions, forbidden, suggested []string) []string {
	m := make(map[string]bool)
	for _, a := range actions {
		m[a] = true
	}
	for _, f := range forbidden {
		delete(m, f)
	}
	for _, s := range suggested {
		m[s] = true
	}
	result := make([]string, 0, len(m))
	for a := range m {
		result = append(result, a)
	}
	return result
}

// UpdateManager applies state machine constraints to the allowed actions list.
func (sm *StateMachine) UpdateManager(state *core.WorldState, baseActions []string) []string {
	return sm.AllowedActions(baseActions)
}
