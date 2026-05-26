package goalexpr

import (
	"testing"

	"corerp/internal/core"
)

func TestEval(t *testing.T) {
	state := core.WorldState{
		Clock: core.WorldTime{Hour: 2, Minute: 30, Day: 3},
		Scene: core.SceneState{Location: "safehouse", TimeOfDay: "night", Weather: "rain"},
		Relationships: map[string]core.Relationship{
			"Anya_player": {Trust: 7, Debt: 1},
		},
		Variables: map[string]interface{}{
			"trust":        10.0,
			"police_alert": 6.0,
			"nested": map[string]interface{}{
				"detected": true,
			},
		},
		Flags: map[string]bool{
			"alarm": true,
		},
		Tension: 0.4,
	}

	cases := []struct {
		expr string
		want bool
	}{
		{"always", true},
		{"never", false},
		{"police_alert > 5", true},
		{"trust > 9 AND scene == safehouse", true},
		{"trust > 9 AND scene == street", false},
		{"flags.alarm", true},
		{"nested.detected == true", true},
		{"NOT alarm", false},
		{"tension < 0.5", true},
		{"hour >= 2 AND minute == 30", true},
		{"relationship.trust >= 7", true},
		{"relationships.Anya_player.debt == 1", true},
	}

	for _, tc := range cases {
		got, err := Eval(tc.expr, state)
		if err != nil {
			t.Fatalf("Eval(%q) error = %v", tc.expr, err)
		}
		if got != tc.want {
			t.Fatalf("Eval(%q) = %v, want %v", tc.expr, got, tc.want)
		}
	}
}

func TestValidateRejectsInvalidExpression(t *testing.T) {
	cases := []string{
		"trust >",
		"scene ==",
		"trust > 5 AND (scene == safehouse",
		"foo @ bar",
	}
	for _, expr := range cases {
		if err := Validate(expr); err == nil {
			t.Fatalf("Validate(%q) = nil, want error", expr)
		}
	}
}

func TestValidateCharacter(t *testing.T) {
	err := ValidateCharacter(core.Character{
		Goals: []core.Goal{
			{ID: "survive", Type: "primary", Condition: "always"},
			{ID: "secret", Type: "hidden", RevealCondition: "trust > 9 AND scene == safehouse"},
		},
	})
	if err != nil {
		t.Fatalf("ValidateCharacter(valid) error = %v", err)
	}

	err = ValidateCharacter(core.Character{
		Goals: []core.Goal{
			{ID: "broken", Type: "primary", Condition: "trust >"},
		},
	})
	if err == nil {
		t.Fatal("ValidateCharacter(invalid) = nil, want error")
	}

	err = ValidateCharacter(core.Character{
		Goals: []core.Goal{
			{ID: "broken", Type: "primary", CooldownTurns: -1},
		},
	})
	if err == nil {
		t.Fatal("ValidateCharacter(negative cooldown) = nil, want error")
	}
}
