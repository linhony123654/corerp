package actions

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"
)

// DiceResult holds the outcome of a dice roll.
type DiceResult struct {
	Expression string `json:"expression"` // e.g. "2d6+trust"
	Rolls      []int  `json:"rolls"`       // individual dice results
	Modifier   int    `json:"modifier"`     // stat modifier
	Total      int    `json:"total"`        // sum + modifier
	Difficulty int    `json:"difficulty"`   // target number (0 = no check)
	Success    *bool  `json:"success"`      // nil if no difficulty set
	Summary    string `json:"summary"`      // human-readable
}

// StatValue maps a stat key to the character's adaptive values.
type StatValue func(key string) int

// RollDice parses an expression like "2d6+trust" or "3d6" and returns the result.
func RollDice(expr string, statFn StatValue, difficulty int) (*DiceResult, error) {
	expr = strings.ToLower(strings.TrimSpace(expr))
	if expr == "" {
		return nil, fmt.Errorf("empty dice expression")
	}

	// Parse: [N]d[S]+[mod]
	count, sides := 2, 6
	rest := expr

	// Parse "XdY" prefix
	if idx := strings.Index(rest, "d"); idx >= 0 {
		if idx > 0 {
			c, err := strconv.Atoi(rest[:idx])
			if err == nil && c > 0 && c <= 100 {
				count = c
			}
		}
		rest = rest[idx+1:] // after "d"

		// Parse sides
		numEnd := 0
		for numEnd < len(rest) && rest[numEnd] >= '0' && rest[numEnd] <= '9' {
			numEnd++
		}
		if numEnd > 0 {
			s, err := strconv.Atoi(rest[:numEnd])
			if err == nil && s > 0 && s <= 1000 {
				sides = s
			}
			rest = rest[numEnd:]
		} else {
			sides = 6 // default: just "d" means d6
		}
	} else {
		// No "d", treat the whole thing as a stat name
		count, sides = 2, 6
		rest = "+" + expr
	}

	// Parse modifier: +stat or +N or -N
	modifier := 0
	for len(rest) > 0 && (rest[0] == '+' || rest[0] == '-') {
		sign := 1
		if rest[0] == '-' {
			sign = -1
		}
		rest = rest[1:]

		// Try numeric first
		numEnd := 0
		for numEnd < len(rest) && ((rest[numEnd] >= '0' && rest[numEnd] <= '9') || rest[numEnd] == '.') {
			numEnd++
		}
		if numEnd > 0 {
			n, err := strconv.Atoi(rest[:numEnd])
			if err == nil {
				modifier += sign * n
				rest = rest[numEnd:]
				continue
			}
		}

		// Try stat name
		statEnd := strings.IndexAny(rest, "+-")
		if statEnd == -1 {
			statEnd = len(rest)
		}
		statName := strings.TrimSpace(rest[:statEnd])
		if statName != "" && statFn != nil {
			modifier += sign * statFn(statName)
		}
		rest = rest[statEnd:]
	}

	// Roll
	var rolls []int
	total := modifier
	for i := 0; i < count; i++ {
		r := rand.Intn(sides) + 1
		rolls = append(rolls, r)
		total += r
	}

	result := &DiceResult{
		Expression: expr,
		Rolls:      rolls,
		Modifier:   modifier,
		Total:      total,
		Difficulty: difficulty,
	}

	// Build summary
	rollStrs := make([]string, len(rolls))
	for i, r := range rolls {
		rollStrs[i] = strconv.Itoa(r)
	}
	summary := fmt.Sprintf("%dd%d = [%s]", count, sides, strings.Join(rollStrs, ", "))
	if modifier != 0 {
		if modifier >= 0 {
			summary += fmt.Sprintf(" + %d", modifier)
		} else {
			summary += fmt.Sprintf(" - %d", -modifier)
		}
	}
	summary += fmt.Sprintf(" = %d", total)

	if difficulty > 0 {
		success := total >= difficulty
		result.Success = &success
		if success {
			summary += fmt.Sprintf("  ≥ %d → ✓ 成功", difficulty)
		} else {
			summary += fmt.Sprintf("  < %d → ✗ 失败", difficulty)
		}
	}

	result.Summary = summary
	return result, nil
}

// StatToModifier converts a character's adaptive stat value to a dice modifier.
// Stat ranges from 0-10, yielding modifiers from -3 to +5.
func StatToModifier(value float64) int {
	switch {
	case value >= 10:
		return 5
	case value >= 8:
		return 3
	case value >= 6:
		return 2
	case value >= 4:
		return 1
	case value >= 2:
		return 0
	case value >= 1:
		return -1
	case value >= 0.5:
		return -2
	default:
		return -3
	}
}
