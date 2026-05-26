package agents

import (
	"testing"

	"corerp/internal/core"
)

func TestValidateSpeak(t *testing.T) {
	em := NewEnvelopeManager()
	em.LoadCharacter("Anya", core.Character{
		Identity: core.IdentityEnvelope{
			Name:      "Anya",
			Forbidden: []string{"cartoon_behavior"},
			Adaptive:  map[string]float64{"trust": 3},
		},
	})

	frame := core.ActionFrame{Actor: "Anya", Action: "speak", Target: "player"}
	err := em.Validate(frame, "你好。", "Anya")
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestValidateForbiddenAction(t *testing.T) {
	em := NewEnvelopeManager()
	em.LoadCharacter("Anya", core.Character{
		Identity: core.IdentityEnvelope{
			Name:      "Anya",
			Forbidden: []string{"cartoon_behavior"},
		},
	})

	frame := core.ActionFrame{Actor: "Anya", Action: "cartoon_behavior"}
	err := em.Validate(frame, "", "Anya")
	if err == nil {
		t.Error("expected error for forbidden action")
	}
}

func TestValidateForbiddenKeyword(t *testing.T) {
	em := NewEnvelopeManager()
	em.LoadCharacter("Anya", core.Character{
		Identity: core.IdentityEnvelope{
			Name:      "Anya",
			Forbidden: []string{"cartoon_behavior"},
		},
	})

	frame := core.ActionFrame{Actor: "Anya", Action: "speak"}
	err := em.Validate(frame, "我可爱地眨了眨眼，俏皮地吐了吐舌头", "Anya")
	if err == nil {
		t.Error("expected error for cartoon_behavior keywords")
	}
}

func TestValidateAggressiveWithHighTrust(t *testing.T) {
	em := NewEnvelopeManager()
	em.LoadCharacter("V", core.Character{
		Identity: core.IdentityEnvelope{
			Name:      "V",
			Forbidden: []string{},
			Adaptive:  map[string]float64{"trust": 9},
		},
	})

	frame := core.ActionFrame{Actor: "V", Action: "attack", Target: "friend"}
	err := em.Validate(frame, "", "V")
	if err == nil {
		t.Error("expected error for attack with trust=9")
	}
}

func TestValidateUnknownCharacter(t *testing.T) {
	em := NewEnvelopeManager()
	frame := core.ActionFrame{Actor: "Stranger", Action: "attack"}
	// Unknown character — allow everything in P1
	err := em.Validate(frame, "", "Stranger")
	if err != nil {
		t.Errorf("unknown char should pass, got: %v", err)
	}
}

func TestValidateInfoDumpBlocked(t *testing.T) {
	em := NewEnvelopeManager()
	em.LoadCharacter("V", core.Character{
		Identity: core.IdentityEnvelope{
			Name:      "V",
			Forbidden: []string{"info_dump"},
		},
	})

	frame := core.ActionFrame{Actor: "V", Action: "speak"}
	err := em.Validate(frame, "其实我一直瞒着你，我真正的身份是公司特工", "V")
	if err == nil {
		t.Error("expected error for info_dump keyword")
	}
}

func TestGetPersonaFrame(t *testing.T) {
	em := NewEnvelopeManager()
	em.LoadCharacter("V", core.Character{
		Identity: core.IdentityEnvelope{
			Name:      "V",
			Immutable: []string{"雇佣兵", "夜之城本地人"},
			Adaptive:  map[string]float64{"trust": 5},
			Forbidden: []string{"cartoon_behavior"},
			Voice:     core.VoiceConfig{Style: "冷淡", Rhythm: "短促"},
		},
	})

	pf := em.GetPersonaFrame("V")
	if pf.Name != "V" {
		t.Errorf("name = %s, want V", pf.Name)
	}
	if len(pf.Immutable) != 2 {
		t.Errorf("expected 2 immutable traits, got %d", len(pf.Immutable))
	}
	if pf.VoiceStyle != "冷淡" {
		t.Errorf("voice style = %s, want 冷淡", pf.VoiceStyle)
	}
}

func TestValidateFourthWallBreakBlocked(t *testing.T) {
	em := NewEnvelopeManager()
	em.LoadCharacter("V", core.Character{
		Identity: core.IdentityEnvelope{
			Name:      "V",
			Forbidden: []string{"fourth_wall_break"},
		},
	})

	frame := core.ActionFrame{Actor: "V", Action: "speak"}
	err := em.Validate(frame, "你是玩家，我只是程序里的人物。", "V")
	if err == nil {
		t.Error("expected error for fourth_wall_break keyword")
	}
}

func TestGetPersonaFrameUnknownCharacterIsEmpty(t *testing.T) {
	em := NewEnvelopeManager()
	pf := em.GetPersonaFrame("missing")
	if pf.Name != "" || len(pf.Immutable) != 0 || len(pf.Adaptive) != 0 || len(pf.Forbidden) != 0 || pf.VoiceStyle != "" || pf.VoiceRhythm != "" || pf.WritingGuide != "" {
		t.Fatalf("persona frame = %#v, want zero-value fields", pf)
	}
}

func TestActiveGoalsRespectConditionExpressions(t *testing.T) {
	em := NewEnvelopeManager()
	em.LoadCharacter("Anya", core.Character{
		Goals: []core.Goal{
			{ID: "survive", Type: "primary", Priority: 10, Condition: "always"},
			{ID: "escape", Type: "primary", Priority: 9, Condition: "police_alert > 5"},
			{ID: "repay", Type: "secondary", Priority: 6, Condition: "trust > 5"},
		},
	})

	goals := em.ActiveGoals("Anya", core.WorldState{
		Variables: map[string]interface{}{
			"police_alert": 6.0,
			"trust":        4.0,
		},
	}, 1)

	if len(goals) != 2 {
		t.Fatalf("active goals = %#v, want 2", goals)
	}
	if goals[0].ID != "survive" || goals[1].ID != "escape" {
		t.Fatalf("active goals = %#v, want survive + escape", goals)
	}
}

func TestHiddenGoalRevealUsesExpression(t *testing.T) {
	em := NewEnvelopeManager()
	em.LoadCharacter("Anya", core.Character{
		Goals: []core.Goal{
			{
				ID:              "recover_blackbox",
				Type:            "hidden",
				Priority:        8,
				Condition:       "trust > 8",
				RevealCondition: "trust > 9 AND scene == safehouse",
			},
		},
	})

	state := core.WorldState{
		Scene: core.SceneState{Location: "safehouse"},
		Variables: map[string]interface{}{
			"trust": 10.0,
		},
	}
	goals := em.ActiveGoals("Anya", state, 1)
	if len(goals) != 1 || goals[0].ID != "recover_blackbox" {
		t.Fatalf("hidden goals = %#v, want revealed hidden goal", goals)
	}

	state.Scene.Location = "street"
	goals = em.ActiveGoals("Anya", state, 2)
	if len(goals) != 0 {
		t.Fatalf("hidden goals after reveal condition false = %#v, want none", goals)
	}
}

func TestGoalCooldownTurns(t *testing.T) {
	em := NewEnvelopeManager()
	em.LoadCharacter("Anya", core.Character{
		Goals: []core.Goal{
			{ID: "scan_room", Type: "secondary", Priority: 5, Condition: "always", CooldownTurns: 3},
		},
	})
	state := core.WorldState{}

	goals := em.ActiveGoals("Anya", state, 1)
	if len(goals) != 1 {
		t.Fatalf("turn 1 goals = %#v, want 1", goals)
	}

	goals = em.ActiveGoals("Anya", state, 2)
	if len(goals) != 0 {
		t.Fatalf("turn 2 goals = %#v, want cooldown suppressed", goals)
	}

	goals = em.ActiveGoals("Anya", state, 4)
	if len(goals) != 1 {
		t.Fatalf("turn 4 goals = %#v, want active again after cooldown", goals)
	}
}
