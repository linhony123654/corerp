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
	Event   core.Event    `json:"event"`
	Causes  []CausalChain `json:"causes"`
	Effects []CausalChain `json:"effects"`
}

// LinkNewEvent analyzes a new event and establishes bidirectional causal links
// to recent events. Call this after the event has been inserted.
func (c *CausalityEngine) LinkNewEvent(evt core.Event) error {
	recent, err := c.getRecentPastEvents(evt, c.maxRecentEvents)
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

// RebuildAll clears and recomputes causal links for the full event log.
func (c *CausalityEngine) RebuildAll() error {
	if _, err := c.store.db.Exec(`UPDATE events SET causes = 'null', effects = 'null'`+c.store.instanceScopeSuffix(" WHERE "), func() []interface{} {
		_, args := c.store.instanceScopeArgs()
		return args
	}()...); err != nil {
		return err
	}

	rows, err := c.store.db.Query(`SELECT id, type, actor, target, payload, causes, effects, canonical, confidence, confirmations, scene_id, session_id, branch, created_at FROM events`+c.store.instanceScopeSuffix(" WHERE ")+` ORDER BY created_at ASC, id ASC`, func() []interface{} {
		_, args := c.store.instanceScopeArgs()
		return args
	}()...)
	if err != nil {
		return err
	}
	defer rows.Close()

	events, err := scanEvents(rows)
	if err != nil {
		return err
	}

	for _, evt := range events {
		if err := c.LinkNewEvent(evt); err != nil {
			return err
		}
	}
	return nil
}

// isCausedBy returns true if evt was likely caused by past.
func (c *CausalityEngine) isCausedBy(evt, past core.Event) bool {
	if !sharesCausalContext(evt, past) {
		return false
	}

	// A player message should not be caused by another player message merely
	// because they share the same generic actor name.
	if evt.Type == "user_message" && past.Type == "user_message" {
		return false
	}

	// Rule 1: type-based heuristics
	if causal, ok := causalTypeRules[past.Type]; ok {
		for _, effectType := range causal {
			if evt.Type == effectType {
				if past.Type == "user_message" && evt.Type == "dialogue" && past.Target != evt.Actor {
					return false
				}
				if past.Type == "dialogue" && evt.Type == "dialogue" && !dialogueDirectlyRelates(evt, past) {
					return false
				}
				return true
			}
		}
	}

	// Rule 2: same actor chain (actor does A then B)
	if evt.Actor != "" && past.Actor != "" && evt.Actor == past.Actor && !isGenericActor(evt.Actor) {
		return true
	}

	// Rule 3: target chain (X acts on Y, then Y reacts)
	if evt.Actor != "" && past.Target != "" && evt.Actor == past.Target {
		return true
	}

	// Rule 4: dialogue-response pattern
	if past.Type == "user_message" && evt.Type == "dialogue" && past.Target == evt.Actor {
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
				if past.Type == "user_message" && evt.Type == "dialogue" && past.Target != evt.Actor {
					continue
				}
				if past.Type == "dialogue" && evt.Type == "dialogue" && !dialogueDirectlyRelates(evt, past) {
					continue
				}
				return 0.9
			}
		}
	}
	// Actor-based
	if evt.Actor != "" && past.Actor != "" && evt.Actor == past.Actor && !isGenericActor(evt.Actor) {
		return 0.7
	}
	// Target-based
	if evt.Actor != "" && past.Target != "" && evt.Actor == past.Target {
		return 0.6
	}
	// Dialogue-response
	if past.Type == "user_message" && evt.Type == "dialogue" && past.Target == evt.Actor {
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

// GetChain recursively builds a causal chain tree with anti-cycle protection.
func (c *CausalityEngine) GetChain(eventID string, depth int) (*CausalChain, error) {
	return c.getChainWithVisited(eventID, depth, make(map[string]bool))
}

const maxCausalDepth = 6

func (c *CausalityEngine) getChainWithVisited(eventID string, depth int, visited map[string]bool) (*CausalChain, error) {
	if depth <= 0 || depth > maxCausalDepth {
		return nil, nil
	}
	if visited[eventID] {
		return nil, nil // cycle detected — stop
	}
	visited[eventID] = true

	evt, err := c.store.GetByID(eventID)
	if err != nil {
		return nil, err
	}

	chain := &CausalChain{Event: evt}

	causeEvents, _ := c.GetCauses(eventID)
	for _, ce := range causeEvents {
		if visited[ce.ID] {
			continue
		}
		sub, err := c.getChainWithVisited(ce.ID, depth-1, visited)
		if err != nil || sub == nil {
			chain.Causes = append(chain.Causes, CausalChain{Event: ce})
		} else {
			chain.Causes = append(chain.Causes, *sub)
		}
	}

	effectEvents, _ := c.GetEffects(eventID)
	for _, ee := range effectEvents {
		if visited[ee.ID] {
			continue
		}
		sub, err := c.getChainWithVisited(ee.ID, depth-1, visited)
		if err != nil || sub == nil {
			chain.Effects = append(chain.Effects, CausalChain{Event: ee})
		} else {
			chain.Effects = append(chain.Effects, *sub)
		}
	}

	return chain, nil
}

// GetChainNarrativeOnly builds a causal chain excluding system/tick/maintenance events.
func (c *CausalityEngine) GetChainNarrativeOnly(eventID string, depth int) (*CausalChain, error) {
	return c.getChainFiltered(eventID, depth, make(map[string]bool))
}

func (c *CausalityEngine) getChainFiltered(eventID string, depth int, visited map[string]bool) (*CausalChain, error) {
	if depth <= 0 || depth > maxCausalDepth {
		return nil, nil
	}
	if visited[eventID] {
		return nil, nil
	}
	visited[eventID] = true

	evt, err := c.store.GetByID(eventID)
	if err != nil {
		return nil, err
	}
	if !isNarrativeEvent(evt) {
		return c.firstNarrativeAncestor(eventID, depth, visited)
	}

	chain := &CausalChain{Event: evt}

	causeEvents, _ := c.GetCauses(eventID)
	for _, ce := range causeEvents {
		if !isNarrativeEvent(ce) {
			// Still follow their causes (they may lead to narrative events)
			sub, _ := c.getChainFiltered(ce.ID, depth-1, visited)
			if sub != nil && len(sub.Causes) > 0 {
				chain.Causes = append(chain.Causes, sub.Causes...)
			}
			continue
		}
		if visited[ce.ID] {
			continue
		}
		sub, err := c.getChainFiltered(ce.ID, depth-1, visited)
		if err != nil || sub == nil {
			chain.Causes = append(chain.Causes, CausalChain{Event: ce})
		} else {
			chain.Causes = append(chain.Causes, *sub)
		}
	}

	effectEvents, _ := c.GetEffects(eventID)
	for _, ee := range effectEvents {
		if !isNarrativeEvent(ee) {
			sub, _ := c.getChainFiltered(ee.ID, depth-1, visited)
			if sub != nil && len(sub.Effects) > 0 {
				chain.Effects = append(chain.Effects, sub.Effects...)
			}
			continue
		}
		if visited[ee.ID] {
			continue
		}
		sub, err := c.getChainFiltered(ee.ID, depth-1, visited)
		if err != nil || sub == nil {
			chain.Effects = append(chain.Effects, CausalChain{Event: ee})
		} else {
			chain.Effects = append(chain.Effects, *sub)
		}
	}

	return chain, nil
}

func (c *CausalityEngine) firstNarrativeAncestor(eventID string, depth int, visited map[string]bool) (*CausalChain, error) {
	causeEvents, _ := c.GetCauses(eventID)
	if direct := pickPreferredNarrativeEvent(causeEvents); direct != nil {
		return c.getChainFiltered(direct.ID, depth-1, visited)
	}
	for _, ce := range causeEvents {
		sub, err := c.getChainFiltered(ce.ID, depth-1, visited)
		if err != nil {
			continue
		}
		if sub != nil {
			return sub, nil
		}
	}
	return nil, nil
}

// GetChainSummary returns a human-readable summary of the causal chain.
func (c *CausalityEngine) GetChainSummary(eventID string, depth int) (string, error) {
	chain, err := c.GetChain(eventID, depth)
	if err != nil {
		return "", err
	}
	return c.renderChain(chain, 0, false), nil
}

// GetChainSummaryNarrativeOnly returns the chain with system/tick events hidden.
func (c *CausalityEngine) GetChainSummaryNarrativeOnly(eventID string, depth int) (string, error) {
	chain, err := c.GetChainNarrativeOnly(eventID, depth)
	if err != nil {
		return "", err
	}
	return c.renderChain(chain, 0, false), nil
}

func (c *CausalityEngine) renderChain(chain *CausalChain, indent int, skipSystem bool) string {
	if chain == nil {
		return ""
	}
	prefix := strings.Repeat("  ", indent)

	evt := chain.Event
	tag := evt.Tag
	if tag == "" {
		tag = "(untagged)"
	}

	// Skip rendering system/tick events at top level or when filtering
	if indent == 0 && chain.Event.ID == "" {
		return ""
	}

	s := fmt.Sprintf("%s[%s] %s\n", prefix, evt.Type, describeEvent(evt))
	for _, cause := range chain.Causes {
		s += prefix + "  ↑ 因为:\n"
		s += c.renderChain(&cause, indent+1, skipSystem)
	}
	for _, effect := range chain.Effects {
		s += prefix + "  ↓ 导致:\n"
		s += c.renderChain(&effect, indent+1, skipSystem)
	}
	return s
}

func describeEvent(evt core.Event) string {
	base := fmt.Sprintf("%s → %s", fallbackLabel(evt.Actor, "?"), fallbackLabel(evt.Target, "?"))
	if detail := eventDetail(evt); detail != "" {
		return base + " | " + detail
	}
	return base
}

func fallbackLabel(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

func eventDetail(evt core.Event) string {
	switch evt.Type {
	case "user_message", "dialogue":
		if content, ok := evt.Payload["content"].(string); ok {
			return quoteSnippet(content, 48)
		}
	case "fact_extracted":
		if narrative, ok := evt.Payload["narrative"].(string); ok {
			return quoteSnippet(narrative, 48)
		}
	case "trust_change", "fear_change", "tension_change":
		if delta, ok := asFloat(evt.Payload["delta"]); ok {
			return fmt.Sprintf("delta=%+.2f", delta)
		}
	case "dice_roll":
		if summary, ok := evt.Payload["summary"].(string); ok {
			return summary
		}
	case "scene_change":
		if location, ok := evt.Payload["location"].(string); ok && location != "" {
			return "前往 " + location
		}
	case "threat", "threaten", "attack", "negotiation", "npc_action":
		if intent, ok := evt.Payload["intent"].(string); ok && intent != "" {
			return quoteSnippet(intent, 36)
		}
	}
	return ""
}

func quoteSnippet(text string, limit int) string {
	text = strings.TrimSpace(strings.ReplaceAll(text, "\n", " "))
	if text == "" {
		return ""
	}
	rs := []rune(text)
	if len(rs) > limit {
		text = string(rs[:limit]) + "..."
	}
	return "“" + text + "”"
}

func asFloat(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	default:
		return 0, false
	}
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

func (c *CausalityEngine) getRecentPastEvents(evt core.Event, limit int) ([]core.Event, error) {
	branch := evt.Branch
	if branch == "" {
		branch = "main"
	}
	rows, err := c.store.db.Query(
		`SELECT id, type, actor, target, payload, causes, effects, canonical, confidence, confirmations, scene_id, session_id, branch, created_at
		 FROM events
		 WHERE branch = ? AND (created_at < ? OR (created_at = ? AND id < ?))
		`+c.store.instanceScopeSuffix(" AND ")+`
		 ORDER BY created_at DESC, id DESC
		 LIMIT ?`,
		func() []interface{} {
			_, args := c.store.instanceScopeArgs(branch, evt.CreatedAt, evt.CreatedAt, evt.ID, limit)
			return args
		}()...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	events, err := scanEvents(rows)
	if err != nil {
		return nil, err
	}
	for i, j := 0, len(events)-1; i < j; i, j = i+1, j-1 {
		events[i], events[j] = events[j], events[i]
	}
	return events, nil
}

func sharesCausalContext(evt, past core.Event) bool {
	if evt.SessionID != "" && past.SessionID != "" && evt.SessionID != past.SessionID {
		return false
	}
	if evt.SceneID != "" && past.SceneID != "" && evt.SceneID != past.SceneID {
		return false
	}

	// User messages address the active character, so require that target to match
	// the responding actor when linking them to dialogue.
	if past.Type == "user_message" && evt.Type == "dialogue" {
		return past.Target != "" && evt.Actor != "" && past.Target == evt.Actor
	}
	if evt.Type == "user_message" && past.Type == "dialogue" {
		return evt.Target != "" && past.Actor != "" && evt.Target == past.Actor
	}
	return true
}

func isGenericActor(actor string) bool {
	return actor == "user" || actor == "system"
}

func dialogueDirectlyRelates(evt, past core.Event) bool {
	if evt.Actor != "" && past.Actor != "" && evt.Actor == past.Actor {
		return true
	}
	if evt.Actor != "" && past.Target != "" && evt.Actor == past.Target {
		return true
	}
	if evt.Target != "" && past.Actor != "" && evt.Target == past.Actor {
		return true
	}
	return false
}

func pickPreferredNarrativeEvent(events []core.Event) *core.Event {
	var fallback *core.Event
	for i := range events {
		evt := &events[i]
		if !isNarrativeEvent(*evt) {
			continue
		}
		if evt.Type != "user_message" {
			return evt
		}
		if fallback == nil {
			fallback = evt
		}
	}
	return fallback
}

func isNarrativeEvent(evt core.Event) bool {
	switch evt.Type {
	case "user_message", "dialogue", "attack", "threat", "threaten", "trust_change",
		"fear_change", "debt_ack", "negotiation", "scene_change", "hide",
		"move", "go", "speak", "talk", "npc_action", "dice_roll":
		return true
	case "clock_advance", "scene_init", "fact_extracted", "variable_set", "observe":
		return false
	}

	switch evt.Tag {
	case core.TagSystem, core.TagTick, core.TagMaintenance:
		return false
	case core.TagNarrative, core.TagUser:
		return true
	}

	return false
}

// causalTypeRules maps event types to the types they typically cause.
var causalTypeRules = map[string][]string{
	"user_message":   {"dialogue"},
	"attack":         {"fear_change", "tension_change", "hide"},
	"threat":         {"fear_change", "tension_change", "hide", "dialogue"},
	"threaten":       {"fear_change", "tension_change", "hide"},
	"dialogue":       {"trust_change", "debt_ack", "dialogue", "negotiation"},
	"tension_change": {"hide", "threat", "attack"},
	"trust_change":   {"dialogue"},
	"fear_change":    {"hide"},
	"scene_change":   {"dialogue"},
	"clock_advance":  {},
	"npc_action":     {"dialogue", "trust_change", "hide"},
	"fact_extracted": {},
	"variable_set":   {},
}
