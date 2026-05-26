package state

import (
	"sync"
	"testing"

	"corerp/internal/core"
)

// === Manager Tests ===

func TestManagerNew(t *testing.T) {
	m := New()
	s := m.Get()
	if s.Relationships == nil {
		t.Error("relationships map should be initialized")
	}
	if s.Variables == nil {
		t.Error("variables map should be initialized")
	}
	if s.Flags == nil {
		t.Error("flags map should be initialized")
	}
}

func TestManagerGetSet(t *testing.T) {
	m := New()
	expected := core.WorldState{
		Scene: core.SceneState{Location: "test-loc"},
		Tension: 0.5,
	}
	m.Set(expected)
	got := m.Get()
	if got.Scene.Location != expected.Scene.Location {
		t.Errorf("location = %s, want %s", got.Scene.Location, expected.Scene.Location)
	}
	if got.Tension != expected.Tension {
		t.Errorf("tension = %f, want %f", got.Tension, expected.Tension)
	}
}

func TestManagerSetScene(t *testing.T) {
	m := New()
	scene := core.SceneState{
		Location:    "夜之城",
		TimeOfDay:   "夜晚",
		Weather:     "酸雨",
		Characters:  []string{"V", "玩家"},
		Description: "霓虹灯闪烁",
	}
	m.SetScene(scene)
	s := m.Get()
	if s.Scene.Location != "夜之城" {
		t.Errorf("location = %s", s.Scene.Location)
	}
	if len(s.Scene.Characters) != 2 {
		t.Errorf("expected 2 characters, got %d", len(s.Scene.Characters))
	}
}

func TestManagerApplyEffects(t *testing.T) {
	m := New()
	effects := []core.StateEffect{
		{Path: "custom.var", Delta: 42.0},
		{Path: "flags.revealed", Delta: 1.0},
	}
	m.ApplyEffects(effects)
	s := m.Get()
	if v, ok := s.Variables["custom.var"]; !ok || v != 42.0 {
		t.Errorf("custom.var = %v, want 42.0", v)
	}
	if v, ok := s.Variables["flags.revealed"]; !ok || v != 1.0 {
		t.Errorf("flags.revealed = %v, want 1.0", v)
	}
}

func TestManagerUpdateFromProjection(t *testing.T) {
	m := New()
	proj := core.WorldState{
		Scene:   core.SceneState{Location: "new-loc"},
		Tension: 0.9,
	}
	m.UpdateFromProjection(proj)
	s := m.Get()
	if s.Scene.Location != "new-loc" {
		t.Errorf("after projection update: location = %s, want new-loc", s.Scene.Location)
	}
	if s.Tension != 0.9 {
		t.Errorf("after projection update: tension = %f, want 0.9", s.Tension)
	}
}

func TestManagerConcurrentAccess(t *testing.T) {
	m := New()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			m.Set(core.WorldState{Tension: 0.5})
		}()
		go func() {
			defer wg.Done()
			_ = m.Get()
		}()
	}
	wg.Wait()
	// No race = pass (verified by -race flag)
}

// === StateMachine Tests ===

func TestStateMachineCalmToTense(t *testing.T) {
	sm := NewStateMachine()
	if sm.Current() != StateCalm {
		t.Errorf("new machine should be calm, got %s", sm.Current())
	}

	// Tension 0.3 → calm→tense
	newState := sm.Transition(0.3, "conflict")
	if newState != StateTense {
		t.Errorf("tension=0.3: expected tense, got %s", newState)
	}
}

func TestStateMachineCalmToCrisis(t *testing.T) {
	sm := NewStateMachine()
	newState := sm.Transition(0.7, "ambush")
	if newState != StateCrisis {
		t.Errorf("tension=0.7: expected crisis, got %s", newState)
	}
}

func TestStateMachineTenseToCrisis(t *testing.T) {
	sm := NewStateMachine()
	sm.Transition(0.4, "warning") // calm→tense
	newState := sm.Transition(0.8, "explosion")
	if newState != StateCrisis {
		t.Errorf("tense + tension=0.8: expected crisis, got %s", newState)
	}
}

func TestStateMachineTenseToCalm(t *testing.T) {
	sm := NewStateMachine()
	sm.Transition(0.4, "warning") // calm→tense
	newState := sm.Transition(0.1, "peace")
	if newState != StateCalm {
		t.Errorf("tense + tension=0.1: expected calm, got %s", newState)
	}
}

func TestStateMachineCrisisToResolution(t *testing.T) {
	sm := NewStateMachine()
	sm.Transition(0.8, "crisis_start") // calm→crisis
	newState := sm.Transition(0.3, "de-escalation")
	if newState != StateResolution {
		t.Errorf("crisis + tension=0.3: expected resolution, got %s", newState)
	}
}

func TestStateMachineResolutionTransitions(t *testing.T) {
	sm := NewStateMachine()
	sm.Transition(0.8, "crisis_start")    // calm→crisis
	sm.Transition(0.3, "de-escalation")   // crisis→resolution

	// Resolution + high tension → tense
	if s := sm.Transition(0.6, "re-escalation"); s != StateTense {
		t.Errorf("resolution + tension=0.6: expected tense, got %s", s)
	}

	// Reset
	sm2 := NewStateMachine()
	sm2.Transition(0.8, "crisis_start")
	sm2.Transition(0.3, "de-escalation")
	// Resolution + low tension → calm
	if s := sm2.Transition(0.1, "all_clear"); s != StateCalm {
		t.Errorf("resolution + tension=0.1: expected calm, got %s", s)
	}
}

func TestStateMachineHistory(t *testing.T) {
	sm := NewStateMachine()
	sm.Transition(0.5, "first_conflict") // calm→tense
	sm.Transition(0.8, "escalation")     // tense→crisis
	sm.Transition(0.3, "resolution")     // crisis→resolution

	if len(sm.history) != 3 {
		t.Errorf("expected 3 transitions in history, got %d", len(sm.history))
	}
	if sm.history[0].From != StateCalm || sm.history[0].To != StateTense {
		t.Errorf("first transition: %s→%s, want calm→tense", sm.history[0].From, sm.history[0].To)
	}
}

func TestStateMachineAllowedActionsCalm(t *testing.T) {
	sm := NewStateMachine()
	base := []string{"speak", "move", "attack", "hide", "negotiate"}

	allowed := sm.AllowedActions(base)
	if contains(allowed, "attack") {
		t.Error("attack should be filtered in calm state")
	}
	if !contains(allowed, "observe") {
		t.Error("observe should be added in calm state")
	}
	if !contains(allowed, "speak") {
		t.Error("speak should remain in calm state")
	}
}

func TestStateMachineAllowedActionsCrisis(t *testing.T) {
	sm := NewStateMachine()
	sm.Transition(0.8, "crisis")
	base := []string{"speak", "move", "attack", "hide"}

	allowed := sm.AllowedActions(base)
	if !contains(allowed, "flee") {
		t.Error("flee should be added in crisis")
	}
	if !contains(allowed, "desperate_act") {
		t.Error("desperate_act should be added in crisis")
	}
}

func TestStateMachineAllowedActionsResolution(t *testing.T) {
	sm := NewStateMachine()
	sm.Transition(0.8, "crisis")
	sm.Transition(0.3, "de-escalation")
	base := []string{"speak", "move", "attack", "threaten", "hide"}

	allowed := sm.AllowedActions(base)
	if contains(allowed, "attack") {
		t.Error("attack should be filtered in resolution")
	}
	if contains(allowed, "threaten") {
		t.Error("threaten should be filtered in resolution")
	}
	if !contains(allowed, "recover") {
		t.Error("recover should be added in resolution")
	}
}

func TestStateMachineTransitionRemains(t *testing.T) {
	sm := NewStateMachine()
	// Stay calm: tension 0.1 is below any threshold
	newState := sm.Transition(0.1, "idle")
	if newState != StateCalm {
		t.Errorf("staying calm: expected calm, got %s", newState)
	}
	// Repeat — should also stay calm
	newState = sm.Transition(0.1, "more_idle")
	if newState != StateCalm {
		t.Errorf("still calm: expected calm, got %s", newState)
	}
	// History should NOT record idle transitions (no change)
	if len(sm.history) != 0 {
		t.Errorf("expected 0 transitions, got %d", len(sm.history))
	}
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
