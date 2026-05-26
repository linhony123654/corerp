package emotion

import (
	"testing"
	"time"
)

func newTestEngine(t *testing.T) *Engine {
	t.Helper()
	e, err := New(":memory:")
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	t.Cleanup(func() { e.Close() })
	return e
}

// === EmotionVector tests ===

func TestDominantEmotion(t *testing.T) {
	vec := EmotionVector{Joy: 0.3, Sadness: 0.1, Anger: 0.7, Fear: 0.2, Attachment: 0.5}
	name, intensity := vec.DominantEmotion()
	if name != "anger" {
		t.Errorf("dominant = %s, want anger", name)
	}
	if intensity != 0.7 {
		t.Errorf("intensity = %.2f, want 0.70", intensity)
	}
}

func TestDominantEmotionEmpty(t *testing.T) {
	vec := EmotionVector{}
	name, intensity := vec.DominantEmotion()
	if name == "" {
		t.Error("should return something even for zero vector")
	}
	if intensity != 0 {
		t.Errorf("empty vector intensity = %.2f, want 0", intensity)
	}
}

func TestContradictionsAttachmentResentment(t *testing.T) {
	vec := EmotionVector{Attachment: 0.8, Resentment: 0.4}
	cs := vec.Contradictions()
	found := false
	for _, c := range cs {
		if c == "attachment+resentment" {
			found = true
		}
	}
	if !found {
		t.Errorf("should detect attachment+resentment contradiction, got %v", cs)
	}
}

func TestContradictionsTrustFear(t *testing.T) {
	vec := EmotionVector{Trust: 0.7, Fear: 0.5}
	cs := vec.Contradictions()
	found := false
	for _, c := range cs {
		if c == "trust+fear" {
			found = true
		}
	}
	if !found {
		t.Errorf("should detect trust+fear contradiction, got %v", cs)
	}
}

func TestContradictionsJoySadness(t *testing.T) {
	vec := EmotionVector{Joy: 0.6, Sadness: 0.5}
	cs := vec.Contradictions()
	found := false
	for _, c := range cs {
		if c == "joy+sadness" {
			found = true
		}
	}
	if !found {
		t.Errorf("should detect joy+sadness contradiction, got %v", cs)
	}
}

func TestContradictionsAttachmentFearOfLoss(t *testing.T) {
	vec := EmotionVector{Attachment: 0.7, Fear: 0.5}
	cs := vec.Contradictions()
	found := false
	for _, c := range cs {
		if c == "attachment+fear_of_loss" {
			found = true
		}
	}
	if !found {
		t.Errorf("should detect attachment+fear_of_loss, got %v", cs)
	}
}

func TestContradictionsNone(t *testing.T) {
	vec := EmotionVector{Joy: 0.8, Trust: 0.7, Attachment: 0.9} // all positive, low negative
	cs := vec.Contradictions()
	if len(cs) != 0 {
		t.Errorf("expected no contradictions, got %v", cs)
	}
}

func TestContradictionsGratitudeResentment(t *testing.T) {
	vec := EmotionVector{Gratitude: 0.6, Resentment: 0.4}
	cs := vec.Contradictions()
	found := false
	for _, c := range cs {
		if c == "gratitude+resentment" {
			found = true
		}
	}
	if !found {
		t.Errorf("should detect gratitude+resentment contradiction, got %v", cs)
	}
}

// === EmotionalResidue tests ===

func TestResidueDecay(t *testing.T) {
	r := EmotionalResidue{
		Intensity: 1.0,
		Current:   1.0,
		DecayRate: 0.2,
		CreatedAt: time.Now().Add(-3 * 24 * time.Hour), // 3 days ago
	}
	r.DecayTo(time.Now())
	// After 3 days at 0.2/day: 1.0 - 0.6 = 0.4
	if r.Current < 0.35 || r.Current > 0.45 {
		t.Errorf("after 3 days decay, current = %.3f, want ~0.40", r.Current)
	}
}

func TestResidueDecayToZero(t *testing.T) {
	r := EmotionalResidue{
		Intensity: 0.5,
		Current:   0.5,
		DecayRate: 0.3,
		CreatedAt: time.Now().Add(-10 * 24 * time.Hour), // 10 days ago
	}
	r.DecayTo(time.Now())
	if r.Current != 0 {
		t.Errorf("decay should floor at 0, got %.3f", r.Current)
	}
}

func TestResidueIsActive(t *testing.T) {
	r := EmotionalResidue{Current: 0.1, Intensity: 0.5, DecayRate: 0.2}
	if !r.IsActive() {
		t.Error("current=0.1 should be active")
	}
	r.Current = 0.01
	if r.IsActive() {
		t.Error("current=0.01 should be inactive")
	}
}

func TestResidueDecayFuture(t *testing.T) {
	r := EmotionalResidue{
		Intensity: 1.0,
		Current:   1.0,
		DecayRate: 0.2,
		CreatedAt: time.Now(), // just created
	}
	r.DecayTo(time.Now().Add(-1 * time.Hour)) // past
	if r.Current != 1.0 {
		t.Errorf("should not decay to past time, got %.3f", r.Current)
	}
}

// === Engine: Residues ===

func TestEngineAddAndGetResidues(t *testing.T) {
	e := newTestEngine(t)
	now := time.Now()

	r1 := EmotionalResidue{ID: "r1", Character: "V", Type: "disappointment", SourceEvent: "ev_1", Target: "玩家", Intensity: 0.8, DecayRate: 0.1, CreatedAt: now}
	r2 := EmotionalResidue{ID: "r2", Character: "V", Type: "warmth", SourceEvent: "ev_2", Target: "玩家", Intensity: 0.5, DecayRate: 0.2, CreatedAt: now}

	e.AddResidue(r1)
	e.AddResidue(r2)

	residues, err := e.GetActiveResidues("V")
	if err != nil {
		t.Fatalf("get residues: %v", err)
	}
	if len(residues) != 2 {
		t.Fatalf("expected 2 residues, got %d", len(residues))
	}
	if residues[0].Type != "disappointment" {
		t.Errorf("first residue type = %s, want disappointment", residues[0].Type)
	}
}

func TestEngineDecayAllResidues(t *testing.T) {
	e := newTestEngine(t)
	// 10 days ago, 0.5/day → should be completely gone
	r := EmotionalResidue{
		ID: "r1", Character: "V", Type: "hurt", SourceEvent: "ev_1", Target: "玩家",
		Intensity: 1.0, Current: 1.0, DecayRate: 3.0, CreatedAt: time.Now().Add(-10 * 24 * time.Hour),
	}
	e.AddResidue(r)

	e.DecayAllResidues("V", time.Now())
	// Re-read to confirm
	residues, _ := e.GetActiveResidues("V")
	// 10 days * 3.0 = -30 → floor 0, should be inactive
	if len(residues) > 0 && residues[0].Current > 0.05 {
		t.Errorf("residue with decay=3.0 over 10 days should be inactive, current=%.2f", residues[0].Current)
	}
}

func TestEngineClearResidue(t *testing.T) {
	e := newTestEngine(t)
	r := EmotionalResidue{ID: "r1", Character: "V", Type: "guilt", SourceEvent: "ev_1", Target: "玩家", Intensity: 0.8, DecayRate: 0.1, CreatedAt: time.Now()}
	e.AddResidue(r)

	e.ClearResidue("r1")
	residues, _ := e.GetActiveResidues("V")
	if len(residues) != 0 {
		t.Error("residue should be cleared")
	}
}

func TestEngineDefaultDecayRate(t *testing.T) {
	e := newTestEngine(t)
	r := EmotionalResidue{ID: "r1", Character: "V", Type: "warmth", SourceEvent: "ev_1", Target: "玩家", Intensity: 0.8, CreatedAt: time.Now()}
	// DecayRate=0 should default to 0.2
	e.AddResidue(r)

	residues, _ := e.GetActiveResidues("V")
	if len(residues) != 1 {
		t.Fatal("residue not found")
	}
	// It was created just now so current should equal intensity
	if residues[0].Current < 0.7 {
		t.Errorf("fresh residue current = %.2f, want ~0.80", residues[0].Current)
	}
}

// === Engine: Unresolved Threads ===

func TestEngineOpenAndGetThreads(t *testing.T) {
	e := newTestEngine(t)
	now := time.Now()

	e.OpenThread(UnresolvedThread{ID: "t1", Character: "V", Topic: "未回应的告白", Involving: "玩家", OpenedAt: "ev_55", EmotionalWeight: 0.84, Status: "unresolved", CreatedAt: now})
	e.OpenThread(UnresolvedThread{ID: "t2", Character: "V", Topic: "背叛的沉默", Involving: "Jackie", OpenedAt: "ev_60", EmotionalWeight: 0.65, Status: "unresolved", CreatedAt: now})

	threads, err := e.GetUnresolvedThreads("V")
	if err != nil {
		t.Fatalf("get threads: %v", err)
	}
	if len(threads) != 2 {
		t.Fatalf("expected 2 threads, got %d", len(threads))
	}
	if threads[0].Topic != "未回应的告白" {
		t.Errorf("highest weight thread = %s, want 未回应的告白", threads[0].Topic)
	}
}

func TestEngineHintThread(t *testing.T) {
	e := newTestEngine(t)
	e.OpenThread(UnresolvedThread{ID: "t1", Character: "V", Topic: "未回应的告白", Involving: "玩家", OpenedAt: "ev_55", EmotionalWeight: 0.84, Status: "unresolved", CreatedAt: time.Now()})

	e.HintThread("t1", "ev_100")
	e.HintThread("t1", "ev_150")

	threads, _ := e.GetUnresolvedThreads("V")
	if threads[0].HintCount != 2 {
		t.Errorf("hint count = %d, want 2", threads[0].HintCount)
	}
	if threads[0].LastReferenced != "ev_150" {
		t.Errorf("last referenced = %s, want ev_150", threads[0].LastReferenced)
	}
}

func TestEngineResolveThread(t *testing.T) {
	e := newTestEngine(t)
	e.OpenThread(UnresolvedThread{ID: "t1", Character: "V", Topic: "test", Involving: "玩家", OpenedAt: "ev_1", EmotionalWeight: 0.5, Status: "unresolved", CreatedAt: time.Now()})

	e.ResolveThread("t1")
	threads, _ := e.GetUnresolvedThreads("V")
	if len(threads) != 0 {
		t.Error("resolved threads should not appear in unresolved list")
	}
}

func TestEngineAddressThread(t *testing.T) {
	e := newTestEngine(t)
	e.OpenThread(UnresolvedThread{ID: "t1", Character: "V", Topic: "test", Involving: "玩家", OpenedAt: "ev_1", EmotionalWeight: 0.5, Status: "unresolved", CreatedAt: time.Now()})

	e.AddressThread("t1", "ev_200")

	threads, _ := e.GetUnresolvedThreads("V")
	if len(threads) != 1 {
		t.Fatal("addressed thread should still appear")
	}
	if threads[0].Status != "addressed" {
		t.Errorf("status = %s, want addressed", threads[0].Status)
	}
}

// === Engine: Delayed Reactions ===

func TestEngineAddAndCheckReaction(t *testing.T) {
	e := newTestEngine(t)
	now := time.Now()

	dr := DelayedReaction{
		ID: "dr1", Character: "V", TriggerEvent: "ev_10", ReactionType: "realization",
		Intensity: 0.7, Target: "玩家", DelayEvents: 5, CreatedAt: now,
	}
	e.AddDelayedReaction(dr)

	// Event count 3 → not triggered
	triggered, err := e.CheckAndTriggerReactions("V", 3, now)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if len(triggered) != 0 {
		t.Error("should not trigger before delay")
	}

	// Event count 7 → triggered
	triggered, err = e.CheckAndTriggerReactions("V", 7, now)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if len(triggered) != 1 {
		t.Fatalf("expected 1 triggered, got %d", len(triggered))
	}
	if !triggered[0].Triggered {
		t.Error("reaction should be triggered")
	}
	if triggered[0].ReactionType != "realization" {
		t.Errorf("reaction type = %s, want realization", triggered[0].ReactionType)
	}
}

func TestEngineReactionTimeDelay(t *testing.T) {
	e := newTestEngine(t)
	created := time.Now().Add(-3 * 24 * time.Hour)

	dr := DelayedReaction{
		ID: "dr1", Character: "V", TriggerEvent: "ev_10", ReactionType: "longing",
		Intensity: 0.6, Target: "玩家", DelayDuration: 2 * 24 * time.Hour, DelayEvents: 0, CreatedAt: created,
	}
	e.AddDelayedReaction(dr)

	// 3 days later → past 2-day delay → triggered
	triggered, err := e.CheckAndTriggerReactions("V", 0, time.Now())
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if len(triggered) != 1 {
		t.Fatalf("expected 1 triggered by time delay, got %d", len(triggered))
	}
}

func TestEngineShouldTriggerNotDoubleFire(t *testing.T) {
	dr := DelayedReaction{Triggered: true}
	if dr.ShouldTrigger(100, time.Now()) {
		t.Error("triggered reaction should not fire again")
	}
}

func TestEnginePendingReactions(t *testing.T) {
	e := newTestEngine(t)
	now := time.Now()

	e.AddDelayedReaction(DelayedReaction{ID: "dr1", Character: "V", TriggerEvent: "ev_1", ReactionType: "anger", Intensity: 0.5, DelayEvents: 10, CreatedAt: now})
	e.AddDelayedReaction(DelayedReaction{ID: "dr2", Character: "V", TriggerEvent: "ev_2", ReactionType: "sadness", Intensity: 0.4, DelayEvents: 3, CreatedAt: now})

	pending, err := e.GetPendingReactions("V")
	if err != nil {
		t.Fatalf("get pending: %v", err)
	}
	if len(pending) != 2 {
		t.Errorf("expected 2 pending, got %d", len(pending))
	}
}

// === EmotionalSnapshot tests ===

func TestComputeSnapshot(t *testing.T) {
	e := newTestEngine(t)
	now := time.Now()

	// Seed data
	e.AddResidue(EmotionalResidue{ID: "r1", Character: "V", Type: "disappointment", SourceEvent: "ev_1", Target: "玩家", Intensity: 0.7, Current: 0.7, DecayRate: 0.2, CreatedAt: now})
	e.OpenThread(UnresolvedThread{ID: "t1", Character: "V", Topic: "未回应的告白", Involving: "玩家", OpenedAt: "ev_55", EmotionalWeight: 0.84, Status: "unresolved", CreatedAt: now})
	e.AddDelayedReaction(DelayedReaction{ID: "dr1", Character: "V", TriggerEvent: "ev_10", ReactionType: "realization", Intensity: 0.7, DelayEvents: 5, CreatedAt: now})

	vec := EmotionVector{
		Attachment: 0.8, Resentment: 0.35, Fear: 0.5,
		Trust: 0.6, Joy: 0.2, Sadness: 0.3,
	}

	snap, err := e.ComputeSnapshot("V", vec, 3, now)
	if err != nil {
		t.Fatalf("compute snapshot: %v", err)
	}

	if snap.DominantEmotion != "attachment" {
		t.Errorf("dominant = %s, want attachment", snap.DominantEmotion)
	}
	if snap.DominantIntensity != 0.8 {
		t.Errorf("intensity = %.2f, want 0.80", snap.DominantIntensity)
	}
	if len(snap.ActiveResidues) != 1 {
		t.Errorf("residues = %d, want 1", len(snap.ActiveResidues))
	}
	if len(snap.UnresolvedThreads) != 1 {
		t.Errorf("threads = %d, want 1", len(snap.UnresolvedThreads))
	}
	if len(snap.PendingReactions) != 1 {
		t.Errorf("pending reactions = %d, want 1", len(snap.PendingReactions))
	}
	if len(snap.Contradictions) == 0 {
		t.Error("should have contradictions (attachment=0.8 + resentment=0.35)")
	}
}

func TestComputeSnapshotEmptyCharacter(t *testing.T) {
	e := newTestEngine(t)
	vec := EmotionVector{Joy: 0.5}

	snap, err := e.ComputeSnapshot("newbie", vec, 0, time.Now())
	if err != nil {
		t.Fatalf("compute snapshot: %v", err)
	}

	if snap.DominantEmotion != "joy" {
		t.Errorf("dominant = %s, want joy", snap.DominantEmotion)
	}
	if snap.ActiveResidues == nil {
		t.Error("residues should be empty slice, not nil")
	}
	if snap.UnresolvedThreads == nil {
		t.Error("threads should be empty slice, not nil")
	}
}

func TestComputeSnapshotDecaysBeforeReturn(t *testing.T) {
	e := newTestEngine(t)
	oldTime := time.Now().Add(-30 * 24 * time.Hour)
	e.AddResidue(EmotionalResidue{ID: "r_old", Character: "V", Type: "hurt", SourceEvent: "ev_1", Target: "玩家", Intensity: 0.5, Current: 0.5, DecayRate: 3.0, CreatedAt: oldTime})

	vec := EmotionVector{}
	snap, err := e.ComputeSnapshot("V", vec, 0, time.Now())
	if err != nil {
		t.Fatalf("compute: %v", err)
	}
	// 30 days * 3.0/day → floor at 0, so no active residues
	if len(snap.ActiveResidues) != 0 {
		t.Errorf("old residue should be gone, got %d (current=%.2f)", len(snap.ActiveResidues), snap.ActiveResidues[0].Current)
	}
}

func TestEmotionVectorAllDimensions(t *testing.T) {
	vec := EmotionVector{
		Joy: 0.1, Sadness: 0.2, Anger: 0.3, Fear: 0.4, Trust: 0.5,
		Disgust: 0.1, Surprise: 0.2, Attachment: 0.6, Resentment: 0.1,
		Gratitude: 0.3, Guilt: 0.2, Longing: 0.7,
	}
	name, _ := vec.DominantEmotion()
	if name != "longing" {
		t.Errorf("dominant should be longing (0.7), got %s (%.2f)", name, vec.Longing)
	}
}

func TestResidueCurrentDefaultToIntensity(t *testing.T) {
	e := newTestEngine(t)
	r := EmotionalResidue{ID: "r1", Character: "V", Type: "warmth", SourceEvent: "ev_1", Target: "玩家", Intensity: 0.8, DecayRate: 0.2, CreatedAt: time.Now()}
	// Current is 0 → should default to Intensity
	e.AddResidue(r)

	residues, _ := e.GetActiveResidues("V")
	if len(residues) != 1 {
		t.Fatal("residue not found")
	}
	if residues[0].Current < 0.7 {
		t.Errorf("current should default to intensity (0.8), got %.2f", residues[0].Current)
	}
}
