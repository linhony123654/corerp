package goalexpr

import (
	"fmt"
	"strings"

	"corerp/internal/core"
)

func ValidateCharacter(card core.Character) error {
	for i, goal := range card.Goals {
		if strings.TrimSpace(goal.ID) == "" {
			return fmt.Errorf("goal[%d]: id is required", i)
		}
		if goal.CooldownTurns < 0 {
			return fmt.Errorf("goal[%d] %q cooldown_turns must be >= 0", i, goal.ID)
		}
		if goal.Condition != "" {
			if err := Validate(goal.Condition); err != nil {
				return fmt.Errorf("goal[%d] %q condition invalid: %w", i, goal.ID, err)
			}
		}
		if goal.Type == "hidden" && goal.RevealCondition != "" {
			if err := Validate(goal.RevealCondition); err != nil {
				return fmt.Errorf("goal[%d] %q reveal_condition invalid: %w", i, goal.ID, err)
			}
		}
	}
	return nil
}
