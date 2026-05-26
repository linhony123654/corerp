package actions

import (
	"encoding/json"
	"testing"

	"corerp/internal/core"
)

func TestParseActionFrame(t *testing.T) {
	raw := `{"actor":"Anya","action":"speak","target":"player","intensity":3,"intent":"greet","suggested_line":"你好","emotion":{"primary":"calm","secondary":"curious","intensity":0.3},"effects":[]}`
	frame, err := ParseActionFrame(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if frame.Actor != "Anya" {
		t.Errorf("actor = %s, want Anya", frame.Actor)
	}
	if frame.Action != "speak" {
		t.Errorf("action = %s, want speak", frame.Action)
	}
	if frame.Target != "player" {
		t.Errorf("target = %s, want player", frame.Target)
	}
}

func TestParseActionFrameInvalid(t *testing.T) {
	_, err := ParseActionFrame("not json")
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestExecutorSpeak(t *testing.T) {
	ex := NewExecutor()
	frame := core.ActionFrame{
		Actor:         "Anya",
		Action:        "speak",
		Target:        "player",
		Intensity:     3,
		SuggestedLine: "你好，玩家。",
		Emotion:       core.EmotionState{Primary: "calm"},
	}
	state := core.WorldState{}
	events, err := ex.Execute(frame, state)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != "dialogue" {
		t.Errorf("event type = %s, want dialogue", events[0].Type)
	}
	if !events[0].Canonical {
		t.Error("speak event should be canonical")
	}
}

func TestExecutorThreaten(t *testing.T) {
	ex := NewExecutor()
	frame := core.ActionFrame{
		Actor:     "Anya",
		Action:    "threaten",
		Target:    "player",
		Intensity: 5,
		Emotion:   core.EmotionState{Primary: "angry"},
	}
	state := core.WorldState{}
	events, err := ex.Execute(frame, state)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if len(events) < 2 {
		t.Fatalf("expected at least 2 events, got %d", len(events))
	}
	hasThreat := false
	hasTension := false
	for _, e := range events {
		if e.Type == "threat" {
			hasThreat = true
		}
		if e.Type == "tension_change" {
			hasTension = true
		}
	}
	if !hasThreat {
		t.Error("missing threat event")
	}
	if !hasTension {
		t.Error("missing tension_change event")
	}
}

func TestExecutorUnknownAction(t *testing.T) {
	ex := NewExecutor()
	frame := core.ActionFrame{
		Actor:     "Anya",
		Action:    "invent_new_thing",
		Target:    "player",
		Intensity: 1,
	}
	state := core.WorldState{}
	events, err := ex.Execute(frame, state)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Canonical {
		t.Error("unknown action should be quarantined (canonical=false)")
	}
}

func TestExecutorEffectsApplied(t *testing.T) {
	ex := NewExecutor()
	frame := core.ActionFrame{
		Actor:  "Anya",
		Action: "speak",
		Effects: []core.StateEffect{
			{Path: "flags.revealed", Delta: 1},
		},
	}
	state := core.WorldState{}
	events, err := ex.Execute(frame, state)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	hasVarSet := false
	for _, e := range events {
		if e.Type == "variable_set" && e.Payload["key"] == "flags.revealed" {
			hasVarSet = true
		}
	}
	if !hasVarSet {
		t.Error("missing variable_set event for frame effect")
	}
}

func TestAllowedActionsFor(t *testing.T) {
	state := core.WorldState{Tension: 0.2}
	goals := []core.Goal{}

	actions := AllowedActionsFor(state, goals)
	if !contains(actions, "speak") {
		t.Error("speak should be allowed")
	}

	// Tension > 0.5 unlocks combat actions
	state.Tension = 0.8
	actions = AllowedActionsFor(state, goals)
	if !contains(actions, "attack") {
		t.Error("attack should be allowed when tension > 0.5")
	}
	if !contains(actions, "threaten") {
		t.Error("threaten should be allowed when tension > 0.5")
	}
}

func TestAllowedActionsForSurviveRestriction(t *testing.T) {
	state := core.WorldState{Tension: 0.8}
	goals := []core.Goal{
		{ID: "survive", Priority: 10, Type: "primary"},
	}
	actions := AllowedActionsFor(state, goals)
	if contains(actions, "attack") {
		t.Error("attack should be removed when survive priority >= 8")
	}
}

func TestParseActionFrameRoundTrip(t *testing.T) {
	frame := core.ActionFrame{
		Actor:         "V",
		Action:        "negotiate",
		Target:        "fixer",
		Intensity:     4,
		Emotion:       core.EmotionState{Primary: "wary", Secondary: "amused", Intensity: 0.5},
		Intent:        "get_info",
		SuggestedLine: "我需要情报。你开价。",
	}
	data, _ := json.Marshal(frame)
	parsed, err := ParseActionFrame(string(data))
	if err != nil {
		t.Fatalf("round-trip parse failed: %v", err)
	}
	if parsed.Action != frame.Action || parsed.Actor != frame.Actor {
		t.Error("round-trip mismatch")
	}
}

func TestEventTaggingNarrative(t *testing.T) {
	ex := NewExecutor()
	frame := core.ActionFrame{Actor: "V", Action: "speak", Target: "player", SuggestedLine: "hi", Emotion: core.EmotionState{Primary: "neutral"}}
	events, err := ex.Execute(frame, core.WorldState{})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	for _, e := range events {
		if e.Tag != core.TagNarrative {
			t.Errorf("event %s (%s): tag=%s, want narrative", e.ID, e.Type, e.Tag)
		}
	}
}

func TestEventTaggingUnknownAction(t *testing.T) {
	ex := NewExecutor()
	frame := core.ActionFrame{Actor: "V", Action: "unknown_thing", Intensity: 1}
	events, err := ex.Execute(frame, core.WorldState{})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(events) != 1 {
		t.Fatal("expected 1 event")
	}
	if events[0].Tag != core.TagNarrative {
		t.Errorf("even unknown actions should be tagged narrative, got %s", events[0].Tag)
	}
	if events[0].Canonical {
		t.Error("unknown action should not be canonical")
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
