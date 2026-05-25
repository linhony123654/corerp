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

// Plan generates steps from current state and goals.
func (p *Planner) Plan(character string, state core.WorldState, goals []core.Goal, workingMem string) []PlanStep {
	var steps []PlanStep

	// Rule 1: Survival priority
	for _, g := range goals {
		if g.ID == "survive" && g.Priority >= 8 && state.Tension > 0.6 {
			steps = append(steps, PlanStep{
				Action:   "hide",
				Target:   "",
				Priority: 10,
				Reason:   "survival_priority: high tension",
			})
			break
		}
	}

	// Rule 2: Relationship repair
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

	// Rule 3: Information gathering
	if workingMem == "" && len(steps) == 0 {
		steps = append(steps, PlanStep{
			Action:   "observe",
			Target:   state.Scene.Location,
			Priority: 3,
			Reason:   "info_gathering: no working memory",
		})
	}

	// Rule 4: Scene exploration
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

	// Rule 5: Default social engagement
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

// StepsToGoals converts planner output into GoalFrames for snapshot.
func StepsToGoals(steps []PlanStep) []core.GoalFrame {
	var frames []core.GoalFrame
	for i, s := range steps {
		frames = append(frames, core.GoalFrame{
			ID:       fmt.Sprintf("plan_%d_%s", i, s.Action),
			Priority: s.Priority,
			Type:     "planned",
			Target:   s.Target,
			Condition: s.Reason,
		})
	}
	return frames
}
