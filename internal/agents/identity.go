package agents

import (
	"fmt"
	"strings"

	"corerp/internal/core"
)

type EnvelopeManager struct {
	characters map[string]core.Character
}

func NewEnvelopeManager() *EnvelopeManager {
	return &EnvelopeManager{
		characters: make(map[string]core.Character),
	}
}

func (em *EnvelopeManager) LoadCharacter(name string, c core.Character) {
	em.characters[name] = c
}

func (em *EnvelopeManager) GetCharacter(name string) (core.Character, bool) {
	c, ok := em.characters[name]
	return c, ok
}

func (em *EnvelopeManager) Validate(frame core.ActionFrame, text string, charName string) error {
	c, ok := em.characters[charName]
	if !ok {
		return nil // Unknown character, allow everything in P1
	}

	// 1. Action-level interception
	for _, forbidden := range c.Identity.Forbidden {
		if frame.Action == forbidden {
			return fmt.Errorf("forbidden action: %s", frame.Action)
		}
	}

	// 2. Text-level interception (keyword based, P1 simple)
	forbiddenKeywords := em.extractForbiddenKeywords(c.Identity.Forbidden)
	for _, kw := range forbiddenKeywords {
		if strings.Contains(text, kw) {
			return fmt.Errorf("forbidden tone detected: %s", kw)
		}
	}

	// 3. State consistency check (simple P1)
	if frame.Action == "attack" || frame.Action == "threaten" {
		if trust, ok := c.Identity.Adaptive["trust"]; ok && trust > 7 {
			return fmt.Errorf("aggressive action inconsistent with trust=%.1f", trust)
		}
	}

	return nil
}

func (em *EnvelopeManager) extractForbiddenKeywords(forbidden []string) []string {
	// Map forbidden actions to Chinese keywords
	var keywords []string
	for _, f := range forbidden {
		switch f {
		case "cartoon_behavior":
			keywords = append(keywords, "卖萌", "吐舌头", "可爱地", "俏皮")
		case "unconditional_love":
			keywords = append(keywords, "无条件", "永远爱你", "什么都愿意")
		case "info_dump":
			keywords = append(keywords, "其实我一直", "我真正的身份是")
		case "fourth_wall_break":
			keywords = append(keywords, "玩家", "LLM", "AI", "程序")
		}
	}
	return keywords
}

// ActiveGoals returns goals that currently apply based on state
func (em *EnvelopeManager) ActiveGoals(charName string, state core.WorldState) []core.Goal {
	c, ok := em.characters[charName]
	if !ok {
		return nil
	}

	var active []core.Goal
	for _, g := range c.Goals {
		// P1: simplified condition evaluation
		if g.Type == "hidden" && !em.checkRevealCondition(g, state) {
			continue
		}
		// P1: skip complex condition parsing, activate all non-hidden
		if g.Type != "hidden" {
			active = append(active, g)
		}
	}
	return active
}

func (em *EnvelopeManager) checkRevealCondition(g core.Goal, state core.WorldState) bool {
	// P1: simple string matching on flags
	// e.g. "trust > 9 AND scene == safehouse" -> just check if "revealed_xxx" flag exists
	// This is a placeholder for P2/P3 proper condition parser
	if revealed, ok := state.Flags["revealed_"+g.ID]; ok && revealed {
		return true
	}
	return false
}

// GetPersonaFrame builds the persona snapshot for a character
func (em *EnvelopeManager) GetPersonaFrame(charName string) core.PersonaFrame {
	c, ok := em.characters[charName]
	if !ok {
		return core.PersonaFrame{}
	}

	return core.PersonaFrame{
		Name:        charName,
		Immutable:   c.Identity.Immutable,
		Adaptive:    c.Identity.Adaptive,
		Forbidden:   c.Identity.Forbidden,
		VoiceStyle:  c.Identity.Voice.Style,
		VoiceRhythm: c.Identity.Voice.Rhythm,
	}
}
