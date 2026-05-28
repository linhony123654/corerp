package agents

import (
	"fmt"
	"strings"

	"corerp/internal/core"
)

// Planner generates action plans based on goals and world state.
// P2: rule-based only, no LLM calls.
type Planner struct{}

func NewPlanner() *Planner {
	return &Planner{}
}

// PlanStep is a single planned action.
type PlanStep struct {
	Action   string `json:"action"`
	Target   string `json:"target"`
	Priority int    `json:"priority"`
	Reason   string `json:"reason"`
}

// Plan generates steps from current state, goals, and world structure.
func (p *Planner) Plan(character string, state core.WorldState, goals []core.Goal, workingMem string, structure core.WorldStructureConfig) []PlanStep {
	var steps []PlanStep

	// Resolve current location controller and active pressures for the scene
	sceneLocation := strings.TrimSpace(state.Scene.Location)
	var locationController string
	var activePressures []core.WorldPressureConfig
	for _, loc := range structure.Locations {
		if strings.TrimSpace(loc.Name) == sceneLocation {
			locationController = strings.TrimSpace(loc.Controller)
			break
		}
	}
	for _, pr := range structure.Pressures {
		if pr.Intensity > 0 && (strings.TrimSpace(pr.Target) == sceneLocation || strings.TrimSpace(pr.Target) == locationController) {
			activePressures = append(activePressures, pr)
		}
	}
	hasHighIntensityPressure := false
	for _, pr := range activePressures {
		if pr.Intensity >= 0.6 {
			hasHighIntensityPressure = true
			break
		}
	}

	// Rule 1: Survival priority — high tension or high-intensity pressure at location
	for _, g := range goals {
		if g.ID == "survive" && g.Priority >= 8 && (state.Tension > 0.6 || hasHighIntensityPressure) {
			steps = append(steps, PlanStep{
				Action:   "hide",
				Target:   "",
				Priority: 10,
				Reason:   "survival_priority: high tension or active pressure at location",
			})
			break
		}
	}

	// Rule 2: Faction conflict — if hostile faction NPC is present in controller's location
	if locationController != "" {
		charFaction := ""
		for _, fac := range structure.Factions {
			if fac.ID == locationController || fac.Name == locationController {
				// character belongs to this faction or location controlled by it
				charFaction = locationController
				break
			}
		}
		for _, other := range state.Scene.Characters {
			if other == character || other == "" {
				continue
			}
			otherFaction := characterFactionFromStructure(other, structure)
			if otherFaction != "" && charFaction != "" && otherFaction != charFaction {
				// Simplified: different faction presence triggers conflict potential
				steps = append(steps, PlanStep{
					Action:   "threaten",
					Target:   other,
					Priority: 6,
					Reason:   fmt.Sprintf("faction_conflict: %s vs %s at %s", charFaction, otherFaction, sceneLocation),
				})
				steps = append(steps, PlanStep{
					Action:   "trust",
					Target:   other,
					Priority: 5,
					Reason:   fmt.Sprintf("faction_deescalation: %s attempts trust with %s at %s", character, other, sceneLocation),
				})
				break
			}
		}
	}

	// Rule 3: Relationship repair
	for key, rel := range state.Relationships {
		if rel.Trust < -0.3 {
			parts := strings.Split(key, "_")
			target := key
			if len(parts) == 2 {
				target = parts[1]
			}
			steps = append(steps, PlanStep{
				Action:   "trust",
				Target:   target,
				Priority: 7,
				Reason:   fmt.Sprintf("relationship_repair: trust=%.2f", rel.Trust),
			})
		}
	}

	// Rule 4: Pressure response — if active pressure at location, observe or react
	if len(activePressures) > 0 && len(steps) == 0 {
		pr := activePressures[0]
		steps = append(steps, PlanStep{
			Action:   "observe",
			Target:   sceneLocation,
			Priority: 4,
			Reason:   fmt.Sprintf("pressure_response: %s (intensity %.2f) at %s", pr.Name, pr.Intensity, sceneLocation),
		})
	}

	// Rule 5: Information gathering
	if workingMem == "" && len(steps) == 0 {
		steps = append(steps, PlanStep{
			Action:   "observe",
			Target:   state.Scene.Location,
			Priority: 3,
			Reason:   "info_gathering: no working memory",
		})
	}

	// Rule 6: Scene exploration
	hasTarget := false
	for _, c := range state.Scene.Characters {
		if c != character {
			hasTarget = true
			break
		}
	}
	if !hasTarget && len(steps) == 0 {
		steps = append(steps, PlanStep{
			Action:   "move",
			Target:   "",
			Priority: 2,
			Reason:   "exploration: no other characters present",
		})
	}

	// Rule 7: Faction loyalty patrol — if character's faction controls the location
	charFaction := characterFactionFromStructure(character, structure)
	if charFaction != "" && locationController == charFaction && len(steps) == 0 {
		steps = append(steps, PlanStep{
			Action:   "patrol",
			Target:   sceneLocation,
			Priority: 5,
			Reason:   fmt.Sprintf("faction_patrol: %s guarding %s territory", character, charFaction),
		})
	}

	// Rule 8: Pressure-driven trade — if location has market/trade pressure
	if len(steps) == 0 {
		for _, pr := range activePressures {
			if strings.Contains(strings.ToLower(pr.Name), "market") || strings.Contains(strings.ToLower(pr.Name), "trade") || strings.Contains(strings.ToLower(pr.Name), "economy") {
				steps = append(steps, PlanStep{
					Action:   "trade",
					Target:   sceneLocation,
					Priority: 4,
					Reason:   fmt.Sprintf("market_pressure: %s (%.2f) at %s", pr.Name, pr.Intensity, sceneLocation),
				})
				break
			}
		}
	}

	// Rule 9: Distress aid — if scene character shows low trust (distress signal)
	if len(steps) == 0 {
		for _, other := range state.Scene.Characters {
			if other == character || other == "" {
				continue
			}
			for key, rel := range state.Relationships {
				if strings.Contains(key, other) && rel.Trust < 2 {
					steps = append(steps, PlanStep{
						Action:   "aid",
						Target:   other,
						Priority: 6,
						Reason:   fmt.Sprintf("distress_aid: %s showing low trust (%.2f)", other, rel.Trust),
					})
					break
				}
			}
		}
	}

	// Rule 10: Hostile faction observation — if different faction NPC present
	if len(steps) == 0 {
		for _, other := range state.Scene.Characters {
			if other == character || other == "" {
				continue
			}
			otherFaction := characterFactionFromStructure(other, structure)
			if otherFaction != "" && charFaction != "" && otherFaction != charFaction {
				steps = append(steps, PlanStep{
					Action:   "observe",
					Target:   other,
					Priority: 5,
					Reason:   fmt.Sprintf("hostile_faction_watch: %s (faction %s) at %s", other, otherFaction, sceneLocation),
				})
				break
			}
		}
	}

	// Rule 11: Multiple pressure warning — if location has 2+ active pressures
	if len(activePressures) >= 2 && len(steps) == 0 {
		steps = append(steps, PlanStep{
			Action:   "warn",
			Target:   sceneLocation,
			Priority: 5,
			Reason:   fmt.Sprintf("pressure_escalation: %d active pressures at %s", len(activePressures), sceneLocation),
		})
	}

	// Rule 12: Default social engagement
	if len(steps) == 0 {
		steps = append(steps, PlanStep{
			Action:   "speak",
			Target:   "user",
			Priority: 1,
			Reason:   "default: engage with user",
		})
	}

	return steps
}

func characterFactionFromStructure(name string, structure core.WorldStructureConfig) string {
	for _, fac := range structure.Factions {
		if fac.ID == name || fac.Name == name {
			return fac.ID
		}
	}
	return ""
}

// StepsToGoals converts planner output into GoalFrames for snapshot.
func StepsToGoals(steps []PlanStep) []core.GoalFrame {
	var frames []core.GoalFrame
	for i, s := range steps {
		frames = append(frames, core.GoalFrame{
			ID:        fmt.Sprintf("plan_%d_%s", i, s.Action),
			Priority:  s.Priority,
			Type:      "planned",
			Target:    s.Target,
			Condition: s.Reason,
		})
	}
	return frames
}
