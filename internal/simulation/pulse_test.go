package simulation

import (
	"testing"

	"corerp/internal/core"
)

func TestPulseEngineNoPressures(t *testing.T) {
	eng := NewPulseEngine()
	events := eng.Tick(core.WorldStructureConfig{}, core.WorldState{}, 0)
	if len(events) != 0 {
		t.Fatalf("events = %#v, want none", events)
	}
}

func TestPulseEngineEmitsStrongestPressure(t *testing.T) {
	eng := NewPulseEngine()
	events := eng.Tick(core.WorldStructureConfig{
		Pressures: []core.WorldPressureConfig{
			{ID: "rumor", Name: "流言", Intensity: 0.4},
			{ID: "riot", Name: "骚乱", Intensity: 0.9, Target: "外城"},
		},
	}, core.WorldState{Tension: 0.2}, 1)
	if len(events) != 3 {
		t.Fatalf("event count = %d, want 3", len(events))
	}
	if events[0].Type != "world_pressure" {
		t.Fatalf("first event = %s, want world_pressure", events[0].Type)
	}
	if got := events[0].Payload["pressure_id"]; got != "riot" {
		t.Fatalf("pressure_id = %#v, want riot", got)
	}
	if events[1].Type != "tension_change" {
		t.Fatalf("second event = %s, want tension_change", events[1].Type)
	}
	if delta, _ := events[1].Payload["delta"].(float64); delta <= 0 {
		t.Fatalf("delta = %#v, want positive", events[1].Payload["delta"])
	}
}

func TestPulseEngineRespectsCadence(t *testing.T) {
	eng := NewPulseEngine()
	structure := core.WorldStructureConfig{
		Pressures: []core.WorldPressureConfig{
			{ID: "guard", Name: "戒严", Intensity: 0.6},
		},
	}
	first := eng.Tick(structure, core.WorldState{}, 1)
	if len(first) == 0 {
		t.Fatal("expected first tick to emit pressure")
	}
	second := eng.Tick(structure, core.WorldState{}, 2)
	if len(second) != 0 {
		t.Fatalf("second tick events = %#v, want none due to cadence", second)
	}
	third := eng.Tick(structure, core.WorldState{}, 3)
	if len(third) != 0 {
		t.Fatalf("third tick events = %#v, want none due to cadence", third)
	}
	fourth := eng.Tick(structure, core.WorldState{}, 4)
	if len(fourth) == 0 {
		t.Fatal("expected fourth tick to emit pressure again")
	}
}
