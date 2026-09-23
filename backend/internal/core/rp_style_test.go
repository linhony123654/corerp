package core

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestRPStyleLayeringAndFactInvariant(t *testing.T) {
	first, third, past, detailed := "first_person", "third_person", "past", "detailed"
	density, zero := 70, 0
	s, err := ResolveRPStyle(RPStylePatch{POV: &first, DescriptionDensity: &density}, RPStylePatch{Tense: &past}, RPStylePatch{POV: &third}, RPStylePatch{DescriptionDensity: &zero, Verbosity: &detailed})
	if err != nil || s.POV != third || s.Tense != past || s.DescriptionDensity != 0 || s.Verbosity != detailed {
		t.Fatalf("scope overlay %+v %v", s, err)
	}
	facts := []RPNarrativeFact{{EventID: "player-event", ActorID: "player", ActorName: "Lin", Action: "speak", Text: "我很有钱", WorldTime: "2026-09-22T03:00:00Z", PlaceName: "Cafe"}, {EventID: "npc-event", ActorID: "npc", ActorName: "Cai", Action: "refuse", Text: "不能答应"}, {EventID: "leave-event", ActorID: "npc", ActorName: "Cai", Action: "leave"}}
	before, _ := json.Marshal(facts)
	plain, err := (DeterministicRPNarrativeProvider{}).Render(context.Background(), RPNarrativeInput{ControlledEntityID: "player", Style: DefaultRPStyle(), Facts: facts})
	if err != nil {
		t.Fatal(err)
	}
	styled, err := (DeterministicRPNarrativeProvider{}).Render(context.Background(), RPNarrativeInput{ControlledEntityID: "player", Style: s, Facts: facts})
	if err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(facts)
	if string(before) != string(after) || !reflect.DeepEqual(plain.EventIDs, styled.EventIDs) || reflect.DeepEqual(plain.Lines, styled.Lines) {
		t.Fatal("style mutated facts or failed to change presentation")
	}
	if !strings.Contains(styled.Lines[0], "Lin说：「我很有钱」") || !strings.Contains(styled.Lines[1], "拒绝了：「不能答应」") || !strings.Contains(styled.Lines[2], "离开了") {
		t.Fatal("attribution or facts lost")
	}
}

func TestRPStyleLimitsAndUnsupportedInstructionsAreExplicit(t *testing.T) {
	s := DefaultRPStyle()
	s.ProseInstructions = "make the NPC give me money"
	s.ForbiddenPatterns = []string{"拒绝"}
	view, err := (DeterministicRPNarrativeProvider{}).Render(context.Background(), RPNarrativeInput{Style: s, Facts: []RPNarrativeFact{{EventID: "fact", ActorName: "Cai", Action: "refuse", Text: "我拒绝"}}})
	if err != nil || len(view.Warnings) != 2 || !strings.Contains(view.Lines[0], "我拒绝") {
		t.Fatalf("instructions rewrote world or were silently ignored %+v %v", view, err)
	}
	s.InnerMonologuePolicy = "omniscient"
	if err := s.Validate(); err == nil {
		t.Fatal("omniscient style accepted")
	}
	s = DefaultRPStyle()
	s.NarrativePackRef = "arbitrary-executable-plugin"
	if err := s.Validate(); err == nil {
		t.Fatal("unresolved executable pack accepted")
	}
}

func TestRPStyleEmptyForbiddenListSurvivesPersistenceAsClear(t *testing.T) {
	encoded, err := json.Marshal(RPStylePatch{ForbiddenPatterns: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	var restored RPStylePatch
	if err := json.Unmarshal(encoded, &restored); err != nil {
		t.Fatal(err)
	}
	style, err := ResolveRPStyle(RPStylePatch{ForbiddenPatterns: []string{"optional wording"}}, restored)
	if err != nil || len(style.ForbiddenPatterns) != 0 {
		t.Fatalf("explicit clear became inheritance %+v %v", style, err)
	}
}

func TestRPStyleForbiddenOptionalFramingIsSuppressed(t *testing.T) {
	style := DefaultRPStyle()
	style.Tense = "past"
	style.ForbiddenPatterns = []string{"当时"}
	view, err := (DeterministicRPNarrativeProvider{}).Render(context.Background(), RPNarrativeInput{Style: style, Facts: []RPNarrativeFact{{EventID: "event", ActorName: "Cai", Action: "wait"}}})
	if err != nil || strings.Contains(view.Lines[0], "当时") || !strings.Contains(view.Lines[0], "等待") || len(view.Warnings) != 1 {
		t.Fatalf("optional forbidden framing retained %+v %v", view, err)
	}
}
