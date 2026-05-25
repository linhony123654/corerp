package events

import (
	"encoding/json"
	"fmt"
	"strings"

	"corerp/internal/core"
)

// CausalityEngine tracks cause-effect relationships between events.
// Links are stored in the existing causes/effects JSON columns on the events table.
type CausalityEngine struct {
	store           *Store
	maxRecentEvents int // how many recent events to scan for causal links
}

func NewCausalityEngine(store *Store) *CausalityEngine {
	return &CausalityEngine{
		store:           store,
		maxRecentEvents: 20,
	}
}

// CausalChain represents a tree of causes and effects.
type CausalChain struct {
	Event    core.Event     `json:"event"`
	Causes   []CausalChain  `json:"causes"`
	Effects  []CausalChain  `json:"effects"`
}

// LinkNewEvent analyzes a new event and establishes bidirectional causal links
// to recent events. Call this after the event has been inserted.
func (c *CausalityEngine) LinkNewEvent(evt core.Event) error {
	recent, err := c.store.GetRecentEvents(c.maxRecentEvents)
	if err != nil {
		return err
	}

	var causes []core.Cause
	for _, past := range recent {
		if past.ID == evt.ID {
			continue
		}
		if c.isCausedBy(evt, past) {
			causes = append(causes, core.Cause{EventID: past.ID, Weight: c.causalWeight(evt, past)})
			// Update the cause event's effects to point to this new event
			c.addEffect(past.ID, evt.ID)
		}
	}

	if len(causes) > 0 {
		return c.setCauses(evt.ID, causes)
	}
	return nil
}

// isCausedBy returns true if evt was likely caused by past.
func (c *CausalityEngine) isCausedBy(evt, past core.Event) bool {
	// Rule 1: type-based heuristics
	if causal, ok := causalTypeRules[past.Type]; ok {
		for _, effectType := range causal {
			if evt.Type == effectType {
				return true
			}
		}
	}

	// Rule 2: same actor chain (actor does A then B)
	if evt.Actor != "" && past.Actor != "" && evt.Actor == past.Actor {
		return true
	}

	// Rule 3: target chain (X acts on Y, then Y reacts)
	if evt.Actor != "" && past.Target != "" && evt.Actor == past.Target {
		return true
	}

	// Rule 4: dialogue-response pattern
	if past.Type == "user_message" && evt.Type == "dialogue" {
		return true
	}

	return false
}

// causalWeight returns a confidence weight for the causal link.
func (c *CausalityEngine) causalWeight(evt, past core.Event) float64 {
	// Type-based rules are strongest
	if causal, ok := causalTypeRules[past.Type]; ok {
		for _, effectType := range causal {
			if evt.Type == effectType {
				return 0.9
			}
		}
	}
	// Actor-based
	if evt.Actor != "" && past.Actor != "" && evt.Actor == past.Actor {
		return 0.7
	}
	// Target-based
	if evt.Actor != "" && past.Target != "" && evt.Actor == past.Target {
		return 0.6
	}
	// Dialogue-response
	if past.Type == "user_message" && evt.Type == "dialogue" {
		return 0.85
	}
	return 0.3
}

// GetCauses returns events that directly caused this event.
func (c *CausalityEngine) GetCauses(eventID string) ([]core.Event, error) {
	evt, err := c.store.GetByID(eventID)
	if err != nil {
		return nil, err
	}

	var causes []core.Event
	for _, cause := range evt.Causes {
		ce, err := c.store.GetByID(cause.EventID)
		if err != nil {
			continue
		}
		causes = append(causes, ce)
	}
	return causes, nil
}

// GetEffects returns events directly caused by this event.
func (c *CausalityEngine) GetEffects(eventID string) ([]core.Event, error) {
	evt, err := c.store.GetByID(eventID)
	if err != nil {
		return nil, err
	}

	var effects []core.Event
	for _, eff := range evt.Effects {
		ee, err := c.store.GetByID(causeEffectID(eff))
		if err != nil {
			continue
		}
		effects = append(effects, ee)
	}
	return effects, nil
}

// GetChain recursively builds a causal chain tree.
func (c *CausalityEngine) GetChain(eventID string, depth int) (*CausalChain, error) {
	if depth <= 0 {
		return nil, nil
	}

	evt, err := c.store.GetByID(eventID)
	if err != nil {
		return nil, err
	}

	chain := &CausalChain{Event: evt}

	// Get direct causes
	causeEvents, _ := c.GetCauses(eventID)
	for _, ce := range causeEvents {
		sub, err := c.GetChain(ce.ID, depth-1)
		if err != nil || sub == nil {
			chain.Causes = append(chain.Causes, CausalChain{Event: ce})
		} else {
			chain.Causes = append(chain.Causes, *sub)
		}
	}

	// Get direct effects
	effectEvents, _ := c.GetEffects(eventID)
	for _, ee := range effectEvents {
		sub, err := c.GetChain(ee.ID, depth-1)
		if err != nil || sub == nil {
			chain.Effects = append(chain.Effects, CausalChain{Event: ee})
		} else {
			chain.Effects = append(chain.Effects, *sub)
		}
	}

	return chain, nil
}

// GetChainSummary returns a human-readable summary of the causal chain.
func (c *CausalityEngine) GetChainSummary(eventID string, depth int) (string, error) {
	chain, err := c.GetChain(eventID, depth)
	if err != nil {
		return "", err
	}
	return c.renderChain(chain, 0), nil
}

func (c *CausalityEngine) renderChain(chain *CausalChain, indent int) string {
	if chain == nil {
		return ""
	}
	prefix := strings.Repeat("  ", indent)
	s := fmt.Sprintf("%s[%s] %s → %s\n", prefix, chain.Event.Type, chain.Event.Actor, chain.Event.Target)
	for _, cause := range chain.Causes {
		s += prefix + "  ↑ 因为:\n"
		s += c.renderChain(&cause, indent+1)
	}
	for _, effect := range chain.Effects {
		s += prefix + "  ↓ 导致:\n"
		s += c.renderChain(&effect, indent+1)
	}
	return s
}

// --- Internal helpers ---

func (c *CausalityEngine) setCauses(eventID string, causes []core.Cause) error {
	jsonCauses, _ := json.Marshal(causes)
	_, err := c.store.db.Exec(`UPDATE events SET causes = ? WHERE id = ?`, jsonCauses, eventID)
	return err
}

func (c *CausalityEngine) addEffect(eventID string, effectID string) error {
	evt, err := c.store.GetByID(eventID)
	if err != nil {
		return err
	}
	// Convert existing effects to set of IDs to avoid duplicates
	existing := make(map[string]bool)
	for _, eff := range evt.Effects {
		existing[causeEffectID(eff)] = true
	}
	if existing[effectID] {
		return nil
	}
	evt.Effects = append(evt.Effects, core.StateEffect{
		Path:  effectID,
		Delta: 0, // delta unused for effect links
	})
	jsonEffects, _ := json.Marshal(evt.Effects)
	_, err = c.store.db.Exec(`UPDATE events SET effects = ? WHERE id = ?`, jsonEffects, eventID)
	return err
}

func causeEffectID(eff core.StateEffect) string {
	return eff.Path
}

// causalTypeRules maps event types to the types they typically cause.
var causalTypeRules = map[string][]string{
	"user_message":    {"dialogue"},
	"attack":          {"fear_change", "tension_change", "hide"},
	"threat":          {"fear_change", "tension_change", "hide", "dialogue"},
	"threaten":        {"fear_change", "tension_change", "hide"},
	"dialogue":        {"trust_change", "debt_ack", "dialogue", "negotiation"},
	"tension_change":  {"hide", "threat", "attack"},
	"trust_change":     {"dialogue"},
	"fear_change":      {"hide"},
	"scene_change":     {"dialogue"},
	"clock_advance":    {},
	"npc_action":       {"dialogue", "trust_change", "hide"},
	"fact_extracted":   {},
	"variable_set":     {},
}
