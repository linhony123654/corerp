package agents

import (
	"fmt"
	"strings"

	"corerp/internal/core"
	"corerp/internal/goalexpr"
)

type EnvelopeManager struct {
	characters     map[string]core.Character
	goalLastActive map[string]map[string]int
}

func NewEnvelopeManager() *EnvelopeManager {
	return &EnvelopeManager{
		characters:     make(map[string]core.Character),
		goalLastActive: make(map[string]map[string]int),
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

// ActiveGoals returns goals that currently apply based on state.
func (em *EnvelopeManager) ActiveGoals(charName string, state core.WorldState, turn int) []core.Goal {
	c, ok := em.characters[charName]
	if !ok {
		return nil
	}

	var active []core.Goal
	for _, g := range c.Goals {
		if g.Type == "hidden" && !em.checkRevealCondition(g, state) {
			continue
		}
		ok := true
		if strings.TrimSpace(g.Condition) != "" {
			evaluated, err := goalexpr.Eval(g.Condition, state)
			if err != nil {
				ok = false
			} else {
				ok = evaluated
			}
		}
		if ok && em.goalOnCooldown(charName, g, turn) {
			ok = false
		}
		if ok {
			active = append(active, g)
			em.markGoalActive(charName, g.ID, turn)
		}
	}
	return active
}

func (em *EnvelopeManager) checkRevealCondition(g core.Goal, state core.WorldState) bool {
	if strings.TrimSpace(g.RevealCondition) == "" {
		return false
	}
	revealed, err := goalexpr.Eval(g.RevealCondition, state)
	return err == nil && revealed
}

func (em *EnvelopeManager) goalOnCooldown(charName string, g core.Goal, turn int) bool {
	if g.CooldownTurns <= 0 || turn <= 0 {
		return false
	}
	byChar := em.goalLastActive[charName]
	if byChar == nil {
		return false
	}
	lastTurn, ok := byChar[g.ID]
	if !ok {
		return false
	}
	return turn-lastTurn < g.CooldownTurns
}

func (em *EnvelopeManager) markGoalActive(charName, goalID string, turn int) {
	if turn <= 0 {
		return
	}
	byChar := em.goalLastActive[charName]
	if byChar == nil {
		byChar = make(map[string]int)
		em.goalLastActive[charName] = byChar
	}
	byChar[goalID] = turn
}

// GetPersonaFrame builds the persona snapshot for a character
func (em *EnvelopeManager) GetPersonaFrame(charName string) core.PersonaFrame {
	c, ok := em.characters[charName]
	if !ok {
		return core.PersonaFrame{}
	}

	return core.PersonaFrame{
		Name:         charName,
		Immutable:    c.Identity.Immutable,
		Adaptive:     c.Identity.Adaptive,
		Forbidden:    c.Identity.Forbidden,
		VoiceStyle:   c.Identity.Voice.Style,
		VoiceRhythm:  c.Identity.Voice.Rhythm,
		WritingGuide: c.Identity.WritingGuide,
	}
}
