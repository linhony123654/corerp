package emotion

import (
	"testing"
	"time"
)

// === Desire Store tests ===

func TestDesireStoreAddAndGet(t *testing.T) {
	e := newTestEngine(t)
	ds := NewDesireStore(e.DB())

	ds.Add(Desire{ID: "d1", Character: "V", Type: DesireAffection, Target: "玩家", Intensity: 0.8, Reason: "长期相处产生的好感", CreatedAt: time.Now()})
	ds.Add(Desire{ID: "d2", Character: "V", Type: DesireAmbition, Target: "成为夜之城传奇", Intensity: 0.6, Reason: "生存压力", CreatedAt: time.Now()})

	desires, err := ds.GetByCharacter("V")
	if err != nil {
		t.Fatalf("get desires: %v", err)
	}
	if len(desires) != 2 {
		t.Fatalf("expected 2 desires, got %d", len(desires))
	}
	if desires[0].Type != DesireAffection {
		t.Errorf("highest intensity desire = %s, want affection", desires[0].Type)
	}
}

func TestDesireStoreEmpty(t *testing.T) {
	e := newTestEngine(t)
	ds := NewDesireStore(e.DB())

	desires, err := ds.GetByCharacter("nobody")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(desires) != 0 {
		t.Errorf("expected 0 desires, got %d", len(desires))
	}
}

// === Pressure Calculation tests ===

func TestCalculatePressureLow(t *testing.T) {
	vec := EmotionVector{Joy: 0.7, Attachment: 0.8, Trust: 0.6}
	p := CalculatePressure(vec, nil, nil, 2)
	if p.Total >= pressureThreshold {
		t.Errorf("total pressure = %.2f, should be below %.2f for low-tension state", p.Total, pressureThreshold)
	}
}

func TestCalculatePressureHigh(t *testing.T) {
	vec := EmotionVector{
		Fear: 0.7, Resentment: 0.8, Guilt: 0.6, Gratitude: 0.5,
		Attachment: 0.2, // low attachment → high loneliness
	}
	threads := []UnresolvedThread{
		{Topic: "未回应的告白", EmotionalWeight: 0.9, Status: "unresolved"},
		{Topic: "背叛的沉默", EmotionalWeight: 0.7, Status: "unresolved"},
	}
	p := CalculatePressure(vec, threads, nil, 15)
	if p.Total < 0.5 {
		t.Errorf("total pressure = %.2f, should be high for anxious state", p.Total)
	}
	if p.Loneliness < 0.5 {
		t.Errorf("loneliness = %.2f, should be high after 15 turns of no interaction", p.Loneliness)
	}
}

func TestCalculatePressureWithResidue(t *testing.T) {
	vec := EmotionVector{Attachment: 0.4, Fear: 0.5, Resentment: 0.6}
	residues := []EmotionalResidue{
		{Type: "disappointment", Current: 0.9},
		{Type: "hurt", Current: 0.7},
	}
	// Residues increase pressure through emotional vector
	p := CalculatePressure(vec, nil, residues, 5)
	_ = p.Total
}

// === Autonomous Action tests ===

func TestGenerateActionBelowThreshold(t *testing.T) {
	vec := EmotionVector{Joy: 0.8, Attachment: 0.9}
	p := EmotionalPressure{Total: 0.3}
	desires := []Desire{{ID: "d1", Type: DesireAffection, Target: "玩家", Intensity: 0.5}}

	action := GenerateAutonomousAction("V", p, desires, vec, DefaultBudget(), 0)
	if action != nil {
		t.Error("should not generate action below threshold")
	}
}

func TestGenerateActionAffection(t *testing.T) {
	vec := EmotionVector{Attachment: 0.6, Longing: 0.7}
	p := EmotionalPressure{
		Character: "V", Loneliness: 0.9, Total: 0.85,
	}
	desires := []Desire{
		{ID: "d1", Type: DesireAffection, Target: "玩家", Intensity: 0.9, Reason: "长期相处产生依赖"},
	}

	action := GenerateAutonomousAction("V", p, desires, vec, DefaultBudget(), 0)
	if action == nil {
		t.Fatal("should generate action above threshold")
	}
	if action.ActionType != "approach" {
		t.Errorf("action = %s, want approach", action.ActionType)
	}
	if action.Target != "玩家" {
		t.Errorf("target = %s, want 玩家", action.Target)
	}
	if action.Urgency != 0.85 {
		t.Errorf("urgency = %.2f, want 0.85", action.Urgency)
	}
}

func TestGenerateActionRevenge(t *testing.T) {
	vec := EmotionVector{Anger: 0.8, Resentment: 0.7}
	p := EmotionalPressure{Total: 0.9}
	desires := []Desire{
		{ID: "d1", Type: DesireRevenge, Target: "betrayer", Intensity: 0.85, Reason: "被背叛"},
	}

	action := GenerateAutonomousAction("V", p, desires, vec, DefaultBudget(), 0)
	if action == nil {
		t.Fatal("should generate action")
	}
	if action.ActionType != "confront" {
		t.Errorf("action = %s, want confront", action.ActionType)
	}
}

func TestGenerateActionAvoidance(t *testing.T) {
	vec := EmotionVector{Fear: 0.9}
	p := EmotionalPressure{Total: 0.8}
	desires := []Desire{
		{ID: "d1", Type: DesireAvoidance, Target: "stalker", Intensity: 0.7, Reason: "感到被跟踪"},
	}

	action := GenerateAutonomousAction("V", p, desires, vec, DefaultBudget(), 0)
	if action == nil {
		t.Fatal("should generate action")
	}
	if action.ActionType != "avoid" {
		t.Errorf("action = %s, want avoid", action.ActionType)
	}
}

func TestGenerateActionFallbackFromEmotion(t *testing.T) {
	vec := EmotionVector{Anger: 0.9}
	p := EmotionalPressure{Total: 0.8}
	var desires []Desire // empty desires → fallback to dominant emotion

	action := GenerateAutonomousAction("V", p, desires, vec, DefaultBudget(), 0)
	if action == nil {
		t.Fatal("should generate fallback action from emotion")
	}
	if action.ActionType != "confront" {
		t.Errorf("anger should map to confront, got %s", action.ActionType)
	}
}

func TestGenerateActionFearFallback(t *testing.T) {
	vec := EmotionVector{Fear: 0.9}
	p := EmotionalPressure{Total: 0.75}

	action := GenerateAutonomousAction("V", p, nil, vec, DefaultBudget(), 0)
	if action == nil {
		t.Fatal("should generate fallback")
	}
	if action.ActionType != "avoid" {
		t.Errorf("fear should map to avoid, got %s", action.ActionType)
	}
}

func TestDesireTypeMapping(t *testing.T) {
	tests := []struct {
		dtype DesireType
		want  string
	}{
		{DesireAffection, "approach"},
		{DesireAvoidance, "avoid"},
		{DesireAmbition, "seek"},
		{DesireProtection, "protect"},
		{DesireRecognition, "confront"},
		{DesireAutonomy, "withdraw"},
		{DesireRevenge, "confront"},
		{DesireSecrets, "investigate"},
	}
	for _, tc := range tests {
		p := EmotionalPressure{Total: 0.8}
		vec := EmotionVector{}
		desires := []Desire{{ID: "d1", Type: tc.dtype, Target: "t", Intensity: 0.8, Reason: "test"}}
		action := GenerateAutonomousAction("V", p, desires, vec, DefaultBudget(), 0)
		if action == nil {
			t.Errorf("%s → no action generated", tc.dtype)
			continue
		}
		if action.ActionType != tc.want {
			t.Errorf("%s → %s, want %s", tc.dtype, action.ActionType, tc.want)
		}
	}
}

// === Action Budget tests ===

func TestBudgetAllowFirstAction(t *testing.T) {
	b := DefaultBudget()
	allowed, reason := b.Allow("V", 0.8, 0)
	if !allowed {
		t.Errorf("first action should be allowed, got: %s", reason)
	}
}

func TestBudgetCooldownBlocks(t *testing.T) {
	b := DefaultBudget()
	b.Record("V", 0)

	// Same NPC at tick 2 (within cooldown of 5)
	allowed, reason := b.Allow("V", 0.8, 2)
	if allowed {
		t.Error("should be blocked by cooldown")
	}
	if reason != "cooldown" {
		t.Errorf("reason = %s, want cooldown", reason)
	}
}

func TestBudgetCooldownExpires(t *testing.T) {
	b := DefaultBudget()
	b.Record("V", 0)

	// After cooldown expires (tick 5, cooldown is 5, so diff=5 >= 5)
	allowed, _ := b.Allow("V", 0.8, 5)
	if !allowed {
		t.Error("should be allowed after cooldown expires")
	}
}

func TestBudgetSceneCap(t *testing.T) {
	b := DefaultBudget()
	b.SetMaxPerScene(2)

	b.Record("A", 0)
	b.Record("B", 0)

	// Third NPC should be blocked
	allowed, reason := b.Allow("C", 0.8, 1)
	if allowed {
		t.Error("should be blocked by scene cap")
	}
	if reason != "scene_cap" {
		t.Errorf("reason = %s, want scene_cap", reason)
	}
}

func TestBudgetUrgencyBypassCooldown(t *testing.T) {
	b := DefaultBudget()
	b.Record("V", 0)

	// Urgency 0.95 >= bypass 0.9 → allowed even on cooldown
	allowed, _ := b.Allow("V", 0.95, 2)
	if !allowed {
		t.Error("high urgency should bypass cooldown")
	}
}

func TestBudgetUrgencyBypassSceneCap(t *testing.T) {
	b := DefaultBudget()
	b.SetMaxPerScene(1)
	b.Record("A", 0)

	// Urgency 0.9 → not quite at bypass (>=0.9)
	allowed, _ := b.Allow("B", 0.9, 1)
	if !allowed {
		t.Error("urgency >= bypass threshold should skip scene cap")
	}

	// Urgency 0.85 → below bypass, should be blocked
	allowed2, reason2 := b.Allow("C", 0.85, 1)
	if allowed2 {
		t.Error("sub-bypass urgency should be blocked by scene cap")
	}
	_ = reason2
}

func TestBudgetDifferentCharacters(t *testing.T) {
	b := DefaultBudget()
	b.Record("V", 0)

	// Different character should not be affected by V's cooldown
	allowed, _ := b.Allow("Jackie", 0.8, 1)
	if !allowed {
		t.Error("different character should not be affected by other's cooldown")
	}
}

func TestBudgetResetScene(t *testing.T) {
	b := DefaultBudget()
	b.SetMaxPerScene(1)
	b.Record("A", 0)

	b.ResetScene()

	allowed, _ := b.Allow("B", 0.8, 1)
	if !allowed {
		t.Error("scene reset should clear action counter")
	}
}

func TestGenerateActionBudgetBlocks(t *testing.T) {
	b := DefaultBudget()
	vec := EmotionVector{Attachment: 0.6, Longing: 0.7}
	p := EmotionalPressure{Total: 0.85}
	desires := []Desire{{ID: "d1", Type: DesireAffection, Target: "玩家", Intensity: 0.9, Reason: "test"}}

	// First call: allowed
	action := GenerateAutonomousAction("V", p, desires, vec, b, 0)
	if action == nil {
		t.Fatal("first action should be allowed")
	}
	b.Record("V", 0)

	// Second call at tick 1: blocked by cooldown
	action2 := GenerateAutonomousAction("V", p, desires, vec, b, 1)
	if action2 != nil {
		t.Error("second call within cooldown should be blocked")
	}
}

func TestGenerateActionBudgetUrgencyBypass(t *testing.T) {
	b := DefaultBudget()
	vec := EmotionVector{Attachment: 0.6, Longing: 0.7}
	desires := []Desire{{ID: "d1", Type: DesireAffection, Target: "玩家", Intensity: 0.9, Reason: "test"}}

	// Regular pressure
	p := EmotionalPressure{Total: 0.85}
	action := GenerateAutonomousAction("V", p, desires, vec, b, 0)
	if action == nil {
		t.Fatal("should generate action")
	}
	b.Record("V", 0)

	// Very high pressure: bypass cooldown
	pHigh := EmotionalPressure{Total: 0.95}
	action2 := GenerateAutonomousAction("V", pHigh, desires, vec, b, 1)
	if action2 == nil {
		t.Error("urgency >= 0.9 should bypass cooldown")
	}
}

func TestClamp(t *testing.T) {
	if clamp(1.5) != 1.0 {
		t.Error("clamp(1.5) should be 1.0")
	}
	if clamp(-0.5) != 0 {
		t.Error("clamp(-0.5) should be 0")
	}
	if clamp(0.5) != 0.5 {
		t.Error("clamp(0.5) should be 0.5")
	}
}
