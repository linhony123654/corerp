package actions

import (
	"testing"
)

func TestRollDiceBasic(t *testing.T) {
	result, err := RollDice("2d6", nil, 0)
	if err != nil {
		t.Fatalf("roll failed: %v", err)
	}
	if len(result.Rolls) != 2 {
		t.Errorf("expected 2 rolls, got %d", len(result.Rolls))
	}
	for _, r := range result.Rolls {
		if r < 1 || r > 6 {
			t.Errorf("roll %d out of range [1,6]", r)
		}
	}
	if result.Success != nil {
		t.Error("success should be nil when no difficulty set")
	}
}

func TestRollDiceWithModifier(t *testing.T) {
	statFn := func(key string) int {
		if key == "trust" {
			return 3
		}
		return 0
	}
	result, err := RollDice("2d6+trust", statFn, 0)
	if err != nil {
		t.Fatalf("roll failed: %v", err)
	}
	if result.Modifier != 3 {
		t.Errorf("modifier = %d, want 3", result.Modifier)
	}
	if result.Total < 5 || result.Total > 15 {
		t.Errorf("total = %d, expected ~[5,15]", result.Total)
	}
}

func TestRollDiceWithDifficulty(t *testing.T) {
	result, err := RollDice("2d6", nil, 10)
	if err != nil {
		t.Fatalf("roll failed: %v", err)
	}
	if result.Success == nil {
		t.Fatal("success should not be nil when difficulty set")
	}
	if result.Total >= 10 != *result.Success {
		t.Errorf("total=%d, success=%v, mismatch", result.Total, *result.Success)
	}
}

func TestRollDiceNegativeModifier(t *testing.T) {
	statFn := func(key string) int {
		return -2
	}
	result, err := RollDice("2d6+fear", statFn, 0)
	if err != nil {
		t.Fatalf("roll failed: %v", err)
	}
	if result.Modifier != -2 {
		t.Errorf("modifier = %d, want -2", result.Modifier)
	}
}

func TestRollDiceEmptyExpression(t *testing.T) {
	_, err := RollDice("", nil, 0)
	if err == nil {
		t.Error("expected error for empty expression")
	}
}

func TestStatToModifier(t *testing.T) {
	tests := []struct {
		value float64
		want  int
	}{
		{10, 5},
		{9, 3},
		{7, 2},
		{5, 1},
		{3, 0},
		{1.5, -1},
		{0.8, -2},
		{0.3, -3},
		{0, -3},
	}
	for _, tc := range tests {
		got := StatToModifier(tc.value)
		if got != tc.want {
			t.Errorf("StatToModifier(%.1f) = %d, want %d", tc.value, got, tc.want)
		}
	}
}
