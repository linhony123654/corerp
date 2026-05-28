package emotion

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// DesireType categorizes NPC internal drives.
type DesireType string

const (
	DesireAffection   DesireType = "affection"   // want closeness with someone
	DesireAvoidance   DesireType = "avoidance"   // want distance from someone
	DesireAmbition    DesireType = "ambition"    // want to achieve something
	DesireProtection  DesireType = "protection"  // want to protect someone
	DesireRecognition DesireType = "recognition" // want to be seen/valued
	DesireAutonomy    DesireType = "autonomy"    // want freedom/independence
	DesireRevenge     DesireType = "revenge"     // want payback
	DesireSecrets     DesireType = "secrets"     // want to know hidden truth
)

// Desire is an internal drive that motivates autonomous NPC behavior.
type Desire struct {
	ID        string     `json:"id"`
	Character string     `json:"character"`
	Type      DesireType `json:"type"`
	Target    string     `json:"target"`    // who or what this is directed at
	Intensity float64    `json:"intensity"` // 0-1, how strong
	Reason    string     `json:"reason"`    // why (from unresolved thread or emotional residue)
	CreatedAt time.Time  `json:"created_at"`
}

// EmotionalPressure is the accumulation of unresolved emotional tension.
// When pressure exceeds threshold, it triggers autonomous action.
type EmotionalPressure struct {
	Character  string  `json:"character"`
	Loneliness float64 `json:"loneliness"` // from lack of interaction
	Jealousy   float64 `json:"jealousy"`   // from third-party interaction
	Guilt      float64 `json:"guilt"`      // from unresolved debts/promises
	Anxiety    float64 `json:"anxiety"`    // from pending threats/unresolved threads
	Total      float64 `json:"total"`      // sum, normalized
}

const pressureThreshold = 0.55 // Lower default so scene-adjacent NPCs can enter play earlier

// AutonomousAction is what an NPC does when pressure exceeds threshold.
type AutonomousAction struct {
	Character  string  `json:"character"`
	ActionType string  `json:"action_type"` // approach, confront, avoid, seek, confess, protect
	Target     string  `json:"target"`
	Reason     string  `json:"reason"`
	Urgency    float64 `json:"urgency"` // 0-1
}

// === Desire Store ===

// DesireStore persists NPC desires in SQLite.
type DesireStore struct {
	db *sql.DB
}

func NewDesireStore(db *sql.DB) *DesireStore {
	db.Exec(`CREATE TABLE IF NOT EXISTS npc_desires (
		id TEXT PRIMARY KEY,
		character TEXT NOT NULL,
		type TEXT NOT NULL,
		target TEXT,
		intensity REAL DEFAULT 0.5,
		reason TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	return &DesireStore{db: db}
}

func (ds *DesireStore) Add(d Desire) error {
	_, err := ds.db.Exec(
		`INSERT INTO npc_desires (id, character, type, target, intensity, reason, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		d.ID, d.Character, string(d.Type), d.Target, d.Intensity, d.Reason, d.CreatedAt,
	)
	return err
}

func (ds *DesireStore) GetByCharacter(character string) ([]Desire, error) {
	rows, err := ds.db.Query(
		`SELECT id, character, type, target, intensity, reason, created_at FROM npc_desires WHERE character = ? ORDER BY intensity DESC`,
		character,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Desire
	for rows.Next() {
		var d Desire
		var typeStr string
		if err := rows.Scan(&d.ID, &d.Character, &typeStr, &d.Target, &d.Intensity, &d.Reason, &d.CreatedAt); err != nil {
			return nil, err
		}
		d.Type = DesireType(typeStr)
		out = append(out, d)
	}
	return out, rows.Err()
}

// HasDesires returns true if the character already has seeded desires.
func (ds *DesireStore) HasDesires(character string) bool {
	var count int
	ds.db.QueryRow(`SELECT COUNT(*) FROM npc_desires WHERE character = ?`, character).Scan(&count)
	return count > 0
}

// DeleteByCharacter removes all stored desires for a character.
func (ds *DesireStore) DeleteByCharacter(character string) error {
	_, err := ds.db.Exec(`DELETE FROM npc_desires WHERE character = ?`, character)
	return err
}

// SeedDesires generates initial desires from character card data.
// Only seeds if the character has no existing desires (idempotent).
func SeedDesires(store *DesireStore, name string, immutable []string, adaptive map[string]float64, goals []GoalSeed, hidden []HiddenGoalSeed) []Desire {
	if store.HasDesires(name) {
		return nil
	}

	desires := buildSeedDesires(name, immutable, adaptive, goals, hidden)
	for _, d := range desires {
		store.Add(d)
	}
	return desires
}

// ReplaceDesires rebuilds a character's seeded desires from current identity/goals.
func ReplaceDesires(store *DesireStore, name string, immutable []string, adaptive map[string]float64, goals []GoalSeed, hidden []HiddenGoalSeed) []Desire {
	_ = store.DeleteByCharacter(name)
	desires := buildSeedDesires(name, immutable, adaptive, goals, hidden)
	for _, d := range desires {
		store.Add(d)
	}
	return desires
}

func buildSeedDesires(name string, immutable []string, adaptive map[string]float64, goals []GoalSeed, hidden []HiddenGoalSeed) []Desire {
	var desires []Desire
	now := time.Now()

	// Rule 1: traits indicating protection/affection
	for _, trait := range immutable {
		lower := strings.ToLower(trait)
		if strings.Contains(lower, "保护") || strings.Contains(lower, "守护") || strings.Contains(lower, "在乎") {
			addDesire(&desires, name, DesireProtection, "在意的人", 0.7, trait, now)
		}
		if strings.Contains(lower, "爱") || strings.Contains(lower, "喜欢") || strings.Contains(lower, "依赖") {
			addDesire(&desires, name, DesireAffection, "亲近的人", 0.75, trait, now)
		}
	}

	// Rule 2: High trust/intimacy → affection
	if v, ok := adaptive["trust"]; ok && v >= 6 {
		addDesire(&desires, name, DesireAffection, "信任的人", clamp(v/10), "高信任值", now)
	}
	if v, ok := adaptive["intimacy"]; ok && v >= 5 {
		addDesire(&desires, name, DesireAffection, "亲密的人", clamp(v/10), "高亲密值", now)
	}

	// Rule 3: High fear → avoidance
	if v, ok := adaptive["fear"]; ok && v >= 5 {
		addDesire(&desires, name, DesireAvoidance, "感到恐惧的对象", clamp(v/10), "高恐惧值", now)
	}

	// Rule 4: Hidden goals → secrets
	for _, g := range hidden {
		if g.ID != "" {
			addDesire(&desires, name, DesireSecrets, "真相", 0.6+float64(g.Priority)*0.05, g.ID, now)
		}
	}

	// Rule 5: Goals → ambition/autonomy/revenge
	for _, g := range goals {
		switch g.ID {
		case "survive", "survival":
			addDesire(&desires, name, DesireAutonomy, "活下去", 0.8, "生存本能", now)
		case "revenge", "复仇":
			addDesire(&desires, name, DesireRevenge, g.Target, 0.7+float64(g.Priority)*0.03, g.ID, now)
		case "protect", "保护":
			addDesire(&desires, name, DesireProtection, g.Target, 0.7+float64(g.Priority)*0.03, g.ID, now)
		default:
			if g.Priority >= 7 {
				addDesire(&desires, name, DesireAmbition, g.Target, 0.6+float64(g.Priority)*0.04, g.ID, now)
			}
		}
	}

	// Rule 6: Independence traits → autonomy
	for _, trait := range immutable {
		lower := strings.ToLower(trait)
		if strings.Contains(lower, "独立") || strings.Contains(lower, "自由") || strings.Contains(lower, "反抗") || strings.Contains(lower, "不信任") {
			addDesire(&desires, name, DesireAutonomy, "自由", 0.65, trait, now)
			break
		}
	}

	// Rule 7: Recognition-seeking
	for _, trait := range immutable {
		lower := strings.ToLower(trait)
		if strings.Contains(lower, "证明") || strings.Contains(lower, "认可") || strings.Contains(lower, "成名") || strings.Contains(lower, "野心") {
			addDesire(&desires, name, DesireRecognition, "世人", 0.7, trait, now)
			break
		}
	}

	// Rule 8: Every character gets at least one baseline desire
	if len(desires) == 0 {
		addDesire(&desires, name, DesireAutonomy, "活下去", 0.5, "默认生存欲望", now)
	}
	return desires
}

// GoalSeed is a simplified goal for desire seeding (avoids import cycle).
type GoalSeed struct {
	ID       string
	Priority int
	Target   string
}

// HiddenGoalSeed is a simplified hidden goal for desire seeding.
type HiddenGoalSeed struct {
	ID       string
	Priority int
}

func addDesire(desires *[]Desire, name string, dtype DesireType, target string, intensity float64, reason string, now time.Time) {
	id := fmt.Sprintf("sd_%s_%s_%d", name, dtype, time.Now().UnixNano())
	*desires = append(*desires, Desire{
		ID: id, Character: name, Type: dtype, Target: target,
		Intensity: clamp(intensity), Reason: reason, CreatedAt: now,
	})
}

// === Pressure Calculator ===

// CalculatePressure computes emotional pressure from the current emotional state.
func CalculatePressure(vec EmotionVector, threads []UnresolvedThread, residues []EmotionalResidue, sinceLastInteraction int) EmotionalPressure {
	p := EmotionalPressure{}

	// Loneliness: from low attachment + time since last interaction
	p.Loneliness = clamp(0.5 + 0.05*float64(sinceLastInteraction) - vec.Attachment)
	// Jealousy: from resentment + fear + third-party relationship tension
	p.Jealousy = clamp(vec.Resentment*0.6 + vec.Fear*0.4)
	// Guilt: from guilt vector + gratitude (feeling indebted)
	p.Guilt = clamp(vec.Guilt*0.7 + vec.Gratitude*0.3)
	// Anxiety: from unresolved threads + fear
	threadPressure := 0.0
	for _, t := range threads {
		if t.Status != "resolved" {
			threadPressure += t.EmotionalWeight * 0.3
		}
	}
	p.Anxiety = clamp(vec.Fear*0.5 + threadPressure)

	p.Total = clamp((p.Loneliness + p.Jealousy + p.Guilt + p.Anxiety) / 4)
	return p
}

// === Autonomous Action Generator ===

// GenerateAutonomousAction decides if an NPC should act autonomously.
// Returns nil if pressure is below threshold, on cooldown, or scene budget exhausted.
func GenerateAutonomousAction(character string, pressure EmotionalPressure, desires []Desire, vec EmotionVector, budget *ActionBudget, tick int) *AutonomousAction {
	if pressure.Total < pressureThreshold {
		return nil
	}

	// Check budget (urgency bypasses limits)
	if budget != nil {
		if allowed, _ := budget.Allow(character, pressure.Total, tick); !allowed {
			return nil // silenced by budget (cooldown or scene cap)
		}
	}

	// Pick the strongest desire as motivation
	var strongest Desire
	for _, d := range desires {
		if d.Intensity > strongest.Intensity {
			strongest = d
		}
	}

	// Map desire type → action type
	actionMap := map[DesireType]string{
		DesireAffection:   "approach",
		DesireAvoidance:   "avoid",
		DesireAmbition:    "seek",
		DesireProtection:  "protect",
		DesireRecognition: "confront",
		DesireAutonomy:    "withdraw",
		DesireRevenge:     "confront",
		DesireSecrets:     "investigate",
	}

	actionType := actionMap[strongest.Type]
	if actionType == "" {
		// Fallback: derive from dominant emotion
		dom, _ := vec.DominantEmotion()
		switch dom {
		case "anger", "resentment":
			actionType = "confront"
		case "fear", "anxiety":
			actionType = "avoid"
		case "sadness", "longing":
			actionType = "approach"
		case "attachment", "gratitude":
			actionType = "protect"
		default:
			actionType = "approach"
		}
	}

	return &AutonomousAction{
		Character:  character,
		ActionType: actionType,
		Target:     strongest.Target,
		Reason:     fmt.Sprintf("%s (pressure=%.2f)", strongest.Reason, pressure.Total),
		Urgency:    pressure.Total,
	}
}

// === Convenience: generate + log in one call ===

// TryAutonomousAction generates an action, logs the attempt, and returns the result.
// This is the primary entry point for the simulation tick loop.
func TryAutonomousAction(character string, pressure EmotionalPressure, desires []Desire, vec EmotionVector, budget *ActionBudget, tick int, logger *ActionLogger) *AutonomousAction {
	entry := ActionLogEntry{
		Tick:               tick,
		Character:          character,
		PressureTotal:      pressure.Total,
		PressureLoneliness: pressure.Loneliness,
		PressureJealousy:   pressure.Jealousy,
		PressureGuilt:      pressure.Guilt,
		PressureAnxiety:    pressure.Anxiety,
	}
	entry.DominantEmotion, _ = vec.DominantEmotion()

	// Find strongest desire before budget check
	var strongest Desire
	for _, d := range desires {
		if d.Intensity > strongest.Intensity {
			strongest = d
		}
	}
	if strongest.ID != "" {
		entry.StrongestDesire = strongest.Reason
		entry.StrongestDesireType = string(strongest.Type)
		entry.StrongestDesireTarget = strongest.Target
	}

	if pressure.Total < pressureThreshold {
		entry.Fired = false
		entry.BlockedBy = "below_threshold"
		if logger != nil {
			logger.Record(entry)
		}
		return nil
	}

	if budget != nil {
		if allowed, reason := budget.Allow(character, pressure.Total, tick); !allowed {
			entry.Fired = false
			entry.BlockedBy = reason
			if logger != nil {
				logger.Record(entry)
			}
			return nil
		}
	}

	action := GenerateAutonomousAction(character, pressure, desires, vec, budget, tick)
	if action != nil {
		entry.Fired = true
		entry.ActionType = action.ActionType
		entry.Target = action.Target
		entry.Urgency = action.Urgency
		entry.Reason = action.Reason
	}

	if logger != nil {
		logger.Record(entry)
	}
	return action
}

// === Action Budget (prevents NPC spam) ===

// ActionBudget limits autonomous actions to prevent chaos.
// Per-NPC cooldown + global scene cap + urgency bypass.
type ActionBudget struct {
	// Per-NPC state
	lastActionTick map[string]int // character → last tick they acted
	cooldownTicks  int            // min ticks between actions for same NPC

	// Global state
	maxPerScene int // max total autonomous actions per scene
	sceneCount  int // actions so far in current scene

	// Urgency bypass: actions with urgency >= this skip all limits
	urgencyBypass float64
}

// DefaultBudget returns a sensible default budget.
func DefaultBudget() *ActionBudget {
	return &ActionBudget{
		lastActionTick: make(map[string]int),
		cooldownTicks:  3,
		maxPerScene:    4,
		urgencyBypass:  0.8,
	}
}

// Allow checks if a character is allowed to act at this tick.
// Returns (allowed, reason).
func (b *ActionBudget) Allow(character string, urgency float64, tick int) (bool, string) {
	// Urgency bypass: always allow
	if urgency >= b.urgencyBypass {
		return true, ""
	}

	// Per-NPC cooldown
	if lastTick, ok := b.lastActionTick[character]; ok {
		if tick-lastTick < b.cooldownTicks {
			return false, "cooldown"
		}
	}

	// Global scene cap
	if b.sceneCount >= b.maxPerScene {
		return false, "scene_cap"
	}

	return true, ""
}

// Record marks that a character acted at this tick.
func (b *ActionBudget) Record(character string, tick int) {
	b.lastActionTick[character] = tick
	b.sceneCount++
}

// ResetScene clears the scene action counter (use on scene change).
func (b *ActionBudget) ResetScene() {
	b.sceneCount = 0
}

// SetCooldown overrides the per-NPC cooldown.
func (b *ActionBudget) SetCooldown(ticks int) {
	b.cooldownTicks = ticks
}

// SetMaxPerScene overrides the global scene cap.
func (b *ActionBudget) SetMaxPerScene(max int) {
	b.maxPerScene = max
}

func clamp(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
