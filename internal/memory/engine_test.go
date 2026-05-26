package memory

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"corerp/internal/core"
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

func TestPushAndGetDialogue(t *testing.T) {
	e := newTestEngine(t)

	e.PushDialogue(core.Message{Role: "user", Content: "你好"}, "Anya")
	e.PushDialogue(core.Message{Role: "assistant", Content: "嗯。"}, "Anya")

	msgs := e.GetRecentDialogue("Anya")
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Role != "user" {
		t.Errorf("first message role = %s, want user", msgs[0].Role)
	}
	if msgs[1].Role != "assistant" {
		t.Errorf("second message role = %s, want assistant", msgs[1].Role)
	}
}

func TestDialogueRingBuffer(t *testing.T) {
	e := newTestEngine(t)
	e.shortTermCap = 3

	for i := 0; i < 5; i++ {
		e.PushDialogue(core.Message{Role: "user", Content: "msg"}, "Anya")
	}

	msgs := e.GetRecentDialogue("Anya")
	if len(msgs) != 3 {
		t.Errorf("ring buffer should cap at 3, got %d", len(msgs))
	}
}

func TestWorkingMemory(t *testing.T) {
	e := newTestEngine(t)

	err := e.SetWorkingMemory("Anya", "当前在夜之城的酒吧里，外面下着雨")
	if err != nil {
		t.Fatalf("set working memory: %v", err)
	}

	content, err := e.GetWorkingMemory("Anya")
	if err != nil {
		t.Fatalf("get working memory: %v", err)
	}
	if content != "当前在夜之城的酒吧里，外面下着雨" {
		t.Errorf("content = %s", content)
	}
}

func TestRememberAndRecallFact(t *testing.T) {
	e := newTestEngine(t)

	fact := core.FactFrame{
		Subject:    "V",
		Predicate:  "是",
		Object:     "雇佣兵",
		Confidence: 1.0,
	}
	err := e.RememberFact(fact, "Anya", 1.0)
	if err != nil {
		t.Fatalf("remember fact: %v", err)
	}

	facts, err := e.RecallFacts("雇佣兵", "Anya", 10)
	if err != nil {
		t.Fatalf("recall facts: %v", err)
	}
	if len(facts) == 0 {
		t.Error("expected at least 1 fact, got 0")
	} else if facts[0].Subject != "V" {
		t.Errorf("subject = %s, want V", facts[0].Subject)
	}
}

func TestSeedFacts(t *testing.T) {
	e := newTestEngine(t)

	facts := []core.FactFrame{
		{Subject: "夜之城", Predicate: "地点", Object: "一个危险的城市", Confidence: 1.0},
		{Subject: "公司", Predicate: "势力", Object: "统治着一切", Confidence: 1.0},
	}
	err := e.SeedFacts(facts, "Anya")
	if err != nil {
		t.Fatalf("seed facts: %v", err)
	}

	// Second seed should be idempotent
	err = e.SeedFacts(facts, "Anya")
	if err != nil {
		t.Fatalf("second seed: %v", err)
	}

	count := e.CountFacts("Anya")
	if count != 2 {
		t.Errorf("fact count = %d, want 2", count)
	}
}

func TestSeedEpisodics(t *testing.T) {
	e := newTestEngine(t)

	events := []core.EventFrame{
		{EventID: "evt_1", Type: "主线", Description: "初次相遇", EmotionalWeight: 0.5},
	}
	err := e.SeedEpisodics(events, "Anya")
	if err != nil {
		t.Fatalf("seed episodics: %v", err)
	}

	count := e.CountEpisodic("Anya")
	if count != 1 {
		t.Errorf("episodic count = %d, want 1", count)
	}
}

func TestRecallAll(t *testing.T) {
	e := newTestEngine(t)

	// Seed some facts
	e.RememberFact(core.FactFrame{Subject: "V", Predicate: "信任", Object: "Jackie", Confidence: 0.9}, "Anya", 0.9)
	e.RememberFact(core.FactFrame{Subject: "V", Predicate: "恐惧", Object: "Arasaka", Confidence: 0.8}, "Anya", 0.8)

	memories := e.Recall("Arasaka", "Anya", nil)
	if len(memories) == 0 {
		t.Error("expected at least 1 memory from recall")
	}
}

func TestResetDialogue(t *testing.T) {
	e := newTestEngine(t)

	e.PushDialogue(core.Message{Role: "user", Content: "test"}, "Anya")
	e.ResetDialogue("Anya")

	msgs := e.GetRecentDialogue("Anya")
	if len(msgs) != 0 {
		t.Errorf("expected 0 messages after reset, got %d", len(msgs))
	}
}

func TestLoadRecentDialogueFromDBDoesNotMigrateLegacyRows(t *testing.T) {
	e := newTestEngine(t)

	_, err := e.db.Exec(
		`INSERT INTO dialogue_history (id, role, content, character, created_at) VALUES (?, ?, ?, ?, ?)`,
		fmt.Sprintf("legacy_%d", time.Now().UnixNano()),
		"user",
		"legacy message",
		"",
		time.Now(),
	)
	if err != nil {
		t.Fatalf("insert legacy dialogue: %v", err)
	}

	e.LoadRecentDialogueFromDB("Anya", 10)

	if msgs := e.GetRecentDialogue("Anya"); len(msgs) != 0 {
		t.Fatalf("expected no migrated legacy messages, got %#v", msgs)
	}
}

func TestCountFacts(t *testing.T) {
	e := newTestEngine(t)

	if count := e.CountFacts("nobody"); count != 0 {
		t.Errorf("expected 0 for unknown character, got %d", count)
	}

	e.RememberFact(core.FactFrame{Subject: "X", Predicate: "Y", Object: "Z"}, "Anya", 1.0)
	if count := e.CountFacts("Anya"); count != 1 {
		t.Errorf("expected 1, got %d", count)
	}
}

func TestDialogueIsolationAcrossCharacters(t *testing.T) {
	e := newTestEngine(t)

	e.PushDialogue(core.Message{Role: "user", Content: "a1"}, "Anya")
	e.PushDialogue(core.Message{Role: "assistant", Content: "a2"}, "Anya")
	e.PushDialogue(core.Message{Role: "user", Content: "v1"}, "V")

	anya := e.GetRecentDialogue("Anya")
	if len(anya) != 2 || anya[0].Content != "a1" {
		t.Fatalf("Anya dialogue = %#v, want only Anya messages", anya)
	}
	v := e.GetRecentDialogue("V")
	if len(v) != 1 || v[0].Content != "v1" {
		t.Fatalf("V dialogue = %#v, want only V messages", v)
	}

	e.ResetDialogue("Anya")
	if v = e.GetRecentDialogue("V"); len(v) != 1 || v[0].Content != "v1" {
		t.Fatalf("V dialogue after Anya reset = %#v, want untouched", v)
	}
}

func TestFactIsolationAcrossCharacters(t *testing.T) {
	e := newTestEngine(t)

	e.RememberFact(core.FactFrame{Subject: "A", Predicate: "knows", Object: "secret", Confidence: 1}, "Anya", 1)
	e.RememberFact(core.FactFrame{Subject: "V", Predicate: "knows", Object: "plan", Confidence: 1}, "V", 1)

	anyaFacts, err := e.GetAllFacts("Anya")
	if err != nil {
		t.Fatalf("GetAllFacts Anya: %v", err)
	}
	if len(anyaFacts) != 1 || anyaFacts[0].Subject != "A" {
		t.Fatalf("Anya facts = %#v, want only Anya facts", anyaFacts)
	}

	vFacts, err := e.GetAllFacts("V")
	if err != nil {
		t.Fatalf("GetAllFacts V: %v", err)
	}
	if len(vFacts) != 1 || vFacts[0].Subject != "V" {
		t.Fatalf("V facts = %#v, want only V facts", vFacts)
	}
}

func TestMemoryIsolationAcrossInstances(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "shared.db")

	alpha, err := New(dbPath)
	if err != nil {
		t.Fatalf("new alpha engine: %v", err)
	}
	defer alpha.Close()
	alpha.SetInstanceID("alpha")

	beta, err := New(dbPath)
	if err != nil {
		t.Fatalf("new beta engine: %v", err)
	}
	defer beta.Close()
	beta.SetInstanceID("beta")

	alpha.PushDialogue(core.Message{Role: "user", Content: "alpha-msg"}, "Anya")
	beta.PushDialogue(core.Message{Role: "user", Content: "beta-msg"}, "Anya")

	alpha.LoadRecentDialogueFromDB("Anya", 10)
	beta.LoadRecentDialogueFromDB("Anya", 10)

	alphaMsgs := alpha.GetRecentDialogue("Anya")
	if len(alphaMsgs) != 1 || alphaMsgs[0].Content != "alpha-msg" {
		t.Fatalf("alpha dialogue = %#v, want only alpha-msg", alphaMsgs)
	}
	betaMsgs := beta.GetRecentDialogue("Anya")
	if len(betaMsgs) != 1 || betaMsgs[0].Content != "beta-msg" {
		t.Fatalf("beta dialogue = %#v, want only beta-msg", betaMsgs)
	}

	if err := alpha.RememberFact(core.FactFrame{Subject: "A", Predicate: "is", Object: "alpha", Confidence: 1}, "Anya", 1); err != nil {
		t.Fatalf("alpha RememberFact: %v", err)
	}
	if err := beta.RememberFact(core.FactFrame{Subject: "B", Predicate: "is", Object: "beta", Confidence: 1}, "Anya", 1); err != nil {
		t.Fatalf("beta RememberFact: %v", err)
	}

	alphaFacts, err := alpha.GetAllFacts("Anya")
	if err != nil {
		t.Fatalf("alpha GetAllFacts: %v", err)
	}
	if len(alphaFacts) != 1 || alphaFacts[0].Subject != "A" {
		t.Fatalf("alpha facts = %#v, want only alpha fact", alphaFacts)
	}

	betaFacts, err := beta.GetAllFacts("Anya")
	if err != nil {
		t.Fatalf("beta GetAllFacts: %v", err)
	}
	if len(betaFacts) != 1 || betaFacts[0].Subject != "B" {
		t.Fatalf("beta facts = %#v, want only beta fact", betaFacts)
	}
}

func TestDefaultInstanceReadsLegacyMemoryButNamedInstanceDoesNot(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "shared.db")

	legacy, err := New(dbPath)
	if err != nil {
		t.Fatalf("new legacy engine: %v", err)
	}
	defer legacy.Close()

	legacy.PushDialogue(core.Message{Role: "user", Content: "legacy-msg"}, "Anya")
	if err := legacy.RememberFact(core.FactFrame{Subject: "Legacy", Predicate: "is", Object: "default", Confidence: 1}, "Anya", 1); err != nil {
		t.Fatalf("legacy RememberFact: %v", err)
	}

	defaultEngine, err := New(dbPath)
	if err != nil {
		t.Fatalf("new default engine: %v", err)
	}
	defer defaultEngine.Close()
	defaultEngine.SetInstanceID("default")
	defaultEngine.LoadRecentDialogueFromDB("Anya", 10)
	defaultMsgs := defaultEngine.GetRecentDialogue("Anya")
	if len(defaultMsgs) != 1 || defaultMsgs[0].Content != "legacy-msg" {
		t.Fatalf("default dialogue = %#v, want legacy-msg", defaultMsgs)
	}
	defaultFacts, err := defaultEngine.GetAllFacts("Anya")
	if err != nil {
		t.Fatalf("default GetAllFacts: %v", err)
	}
	if len(defaultFacts) != 1 || defaultFacts[0].Subject != "Legacy" {
		t.Fatalf("default facts = %#v, want legacy fact", defaultFacts)
	}

	alphaEngine, err := New(dbPath)
	if err != nil {
		t.Fatalf("new alpha engine: %v", err)
	}
	defer alphaEngine.Close()
	alphaEngine.SetInstanceID("alpha")
	alphaEngine.LoadRecentDialogueFromDB("Anya", 10)
	alphaMsgs := alphaEngine.GetRecentDialogue("Anya")
	if len(alphaMsgs) != 0 {
		t.Fatalf("alpha dialogue = %#v, want no legacy messages", alphaMsgs)
	}
	alphaFacts, err := alphaEngine.GetAllFacts("Anya")
	if err != nil {
		t.Fatalf("alpha GetAllFacts: %v", err)
	}
	if len(alphaFacts) != 0 {
		t.Fatalf("alpha facts = %#v, want no legacy facts", alphaFacts)
	}
}
