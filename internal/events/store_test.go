package events

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"corerp/internal/core"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := New(":memory:")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	return store
}

func TestStoreAppendAndRetrieve(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()

	e := core.Event{
		ID:        "evt_001",
		Type:      "dialogue",
		Actor:     "Anya",
		Target:    "player",
		Payload:   map[string]interface{}{"content": "你好"},
		Canonical: true,
		CreatedAt: time.Now(),
	}
	if err := s.Append(e); err != nil {
		t.Fatalf("append: %v", err)
	}

	events, err := s.GetCanonicalEvents()
	if err != nil {
		t.Fatalf("get canonical: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].ID != "evt_001" {
		t.Errorf("id = %s, want evt_001", events[0].ID)
	}
}

func TestGatekeeperTreatsNPCSchedulerAsCanonicalTickEvent(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()

	gatekeeper := NewGatekeeper(s)
	if err := gatekeeper.Submit(core.Event{
		ID:        "npc_sched_tension",
		Type:      "tension_change",
		Actor:     "guard",
		Target:    "",
		Payload:   map[string]interface{}{"delta": 0.6},
		CreatedAt: time.Now(),
	}, "npc_scheduler:guard"); err != nil {
		t.Fatalf("submit npc scheduler event: %v", err)
	}

	canonical, err := s.GetCanonicalEvents()
	if err != nil {
		t.Fatalf("GetCanonicalEvents: %v", err)
	}
	if len(canonical) != 1 || canonical[0].ID != "npc_sched_tension" || !canonical[0].Canonical || canonical[0].Confidence != 1.0 {
		t.Fatalf("canonical events = %#v, want npc scheduler event promoted as tick-owned canonical event", canonical)
	}
}

func TestStoreGetAllEvents(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()

	for i := 0; i < 5; i++ {
		s.Append(core.Event{
			ID:        fmt.Sprintf("evt_%03d", i),
			Type:      "dialogue",
			Actor:     "Anya",
			Canonical: true,
			CreatedAt: time.Now().Add(time.Duration(i) * time.Second),
		})
	}

	events, err := s.GetAllEvents(3)
	if err != nil {
		t.Fatalf("get all: %v", err)
	}
	if len(events) != 3 {
		t.Errorf("expected 3 events, got %d", len(events))
	}
}

func TestStoreConfirmEvent(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()

	s.Append(core.Event{
		ID:        "evt_quar",
		Type:      "unknown_action",
		Actor:     "Anya",
		Canonical: false,
		CreatedAt: time.Now(),
	})

	if err := s.ConfirmEvent("evt_quar"); err != nil {
		t.Fatalf("confirm: %v", err)
	}

	evt, err := s.GetByID("evt_quar")
	if err != nil {
		t.Fatalf("get by id: %v", err)
	}
	if !evt.Canonical {
		t.Error("event should be canonical after confirmation")
	}
}

func TestStoreGetByIDNotFound(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()

	_, err := s.GetByID("nonexistent")
	if err == nil {
		t.Error("expected error for non-existent event")
	}
}

func TestProjectSceneInit(t *testing.T) {
	events := []core.Event{
		{
			ID:   "evt_001",
			Type: "scene_init",
			Payload: map[string]interface{}{
				"location":    "夜之城",
				"time_of_day": "夜晚",
				"weather":     "雨",
				"characters":  []interface{}{"V", "Jackie"},
				"description": "霓虹灯在雨中闪烁",
			},
			Canonical: true,
		},
	}

	state := Project(events)
	if state.Scene.Location != "夜之城" {
		t.Errorf("location = %s, want 夜之城", state.Scene.Location)
	}
	if state.Scene.TimeOfDay != "夜晚" {
		t.Errorf("time = %s, want 夜晚", state.Scene.TimeOfDay)
	}
	if state.Scene.Weather != "雨" {
		t.Errorf("weather = %s, want 雨", state.Scene.Weather)
	}
	if len(state.Scene.Characters) != 2 {
		t.Errorf("expected 2 characters, got %d", len(state.Scene.Characters))
	}
}

func TestProjectTensionChange(t *testing.T) {
	events := []core.Event{
		{
			ID:        "evt_001",
			Type:      "tension_change",
			Payload:   map[string]interface{}{"delta": 0.5},
			Canonical: true,
		},
		{
			ID:        "evt_002",
			Type:      "tension_change",
			Payload:   map[string]interface{}{"delta": 0.3},
			Canonical: true,
		},
	}

	state := Project(events)
	if state.Tension != 0.8 {
		t.Errorf("tension = %.2f, want 0.80", state.Tension)
	}
}

func TestProjectTrustChange(t *testing.T) {
	events := []core.Event{
		{
			ID:        "evt_001",
			Type:      "trust_change",
			Actor:     "V",
			Target:    "Jackie",
			Payload:   map[string]interface{}{"delta": 1.5},
			Canonical: true,
		},
	}

	state := Project(events)
	key := "V_Jackie"
	r, ok := state.Relationships[key]
	if !ok {
		t.Fatalf("relationship %s not found", key)
	}
	if r.Trust != 1.5 {
		t.Errorf("trust = %.2f, want 1.50", r.Trust)
	}
}

func TestProjectSkipsNonCanonical(t *testing.T) {
	events := []core.Event{
		{
			ID:        "evt_001",
			Type:      "tension_change",
			Payload:   map[string]interface{}{"delta": 1.0},
			Canonical: false, // quarantined
		},
		{
			ID:        "evt_002",
			Type:      "tension_change",
			Payload:   map[string]interface{}{"delta": 0.5},
			Canonical: true,
		},
	}

	state := Project(events)
	if state.Tension != 0.5 {
		t.Errorf("tension = %.2f, want 0.50 (non-canonical event should be skipped)", state.Tension)
	}
}

func TestProjectFlagSet(t *testing.T) {
	events := []core.Event{
		{
			ID:        "evt_001",
			Type:      "flag_set",
			Payload:   map[string]interface{}{"key": "detected"},
			Canonical: true,
		},
	}

	state := Project(events)
	if !state.Flags["detected"] {
		t.Error("flag 'detected' should be true")
	}
}

func TestStoreIsolationAcrossInstances(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "shared.db")

	storeA, err := New(dbPath)
	if err != nil {
		t.Fatalf("new store A: %v", err)
	}
	defer storeA.Close()
	storeA.SetInstanceID("alpha")

	storeB, err := New(dbPath)
	if err != nil {
		t.Fatalf("new store B: %v", err)
	}
	defer storeB.Close()
	storeB.SetInstanceID("beta")

	if err := storeA.Append(core.Event{
		ID:        "evt_alpha",
		Type:      "dialogue",
		Actor:     "Anya",
		Canonical: true,
		CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("append alpha: %v", err)
	}
	if err := storeB.Append(core.Event{
		ID:        "evt_beta",
		Type:      "dialogue",
		Actor:     "V",
		Canonical: true,
		CreatedAt: time.Now().Add(time.Millisecond),
	}); err != nil {
		t.Fatalf("append beta: %v", err)
	}

	alphaEvents, err := storeA.GetCanonicalEvents()
	if err != nil {
		t.Fatalf("alpha GetCanonicalEvents: %v", err)
	}
	if len(alphaEvents) != 1 || alphaEvents[0].ID != "evt_alpha" {
		t.Fatalf("alpha events = %#v, want only evt_alpha", alphaEvents)
	}

	betaEvents, err := storeB.GetCanonicalEvents()
	if err != nil {
		t.Fatalf("beta GetCanonicalEvents: %v", err)
	}
	if len(betaEvents) != 1 || betaEvents[0].ID != "evt_beta" {
		t.Fatalf("beta events = %#v, want only evt_beta", betaEvents)
	}

	if _, err := storeA.GetByID("evt_beta"); err == nil {
		t.Fatal("alpha store should not resolve beta event")
	}
	if _, err := storeB.GetByID("evt_alpha"); err == nil {
		t.Fatal("beta store should not resolve alpha event")
	}
}

func TestBranchIsolationAcrossInstances(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "shared.db")

	storeA, err := New(dbPath)
	if err != nil {
		t.Fatalf("new store A: %v", err)
	}
	defer storeA.Close()
	storeA.SetInstanceID("alpha")

	storeB, err := New(dbPath)
	if err != nil {
		t.Fatalf("new store B: %v", err)
	}
	defer storeB.Close()
	storeB.SetInstanceID("beta")

	if err := storeA.ensureBranch("alpha_branch", "main", ""); err != nil {
		t.Fatalf("ensure alpha branch: %v", err)
	}
	if err := storeB.ensureBranch("beta_branch", "main", ""); err != nil {
		t.Fatalf("ensure beta branch: %v", err)
	}

	branchesA, err := storeA.ListBranchesMetadata()
	if err != nil {
		t.Fatalf("ListBranchesMetadata alpha: %v", err)
	}
	branchesB, err := storeB.ListBranchesMetadata()
	if err != nil {
		t.Fatalf("ListBranchesMetadata beta: %v", err)
	}

	hasAlpha := false
	hasBeta := false
	for _, b := range branchesA {
		if b.Name == "alpha_branch" {
			hasAlpha = true
		}
		if b.Name == "beta_branch" {
			t.Fatalf("alpha instance should not see beta branch")
		}
	}
	for _, b := range branchesB {
		if b.Name == "beta_branch" {
			hasBeta = true
		}
		if b.Name == "alpha_branch" {
			t.Fatalf("beta instance should not see alpha branch")
		}
	}
	if !hasAlpha || !hasBeta {
		t.Fatalf("expected instance-local branches, got alpha=%v beta=%v", hasAlpha, hasBeta)
	}
}

func TestDefaultInstanceReadsLegacyEventsButNamedInstanceDoesNot(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "shared.db")

	legacyStore, err := New(dbPath)
	if err != nil {
		t.Fatalf("new legacy store: %v", err)
	}
	defer legacyStore.Close()
	if err := legacyStore.Append(core.Event{
		ID:        "evt_legacy",
		Type:      "dialogue",
		Actor:     "Anya",
		Canonical: true,
		CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("append legacy: %v", err)
	}

	defaultStore, err := New(dbPath)
	if err != nil {
		t.Fatalf("new default store: %v", err)
	}
	defer defaultStore.Close()
	defaultStore.SetInstanceID("default")

	defaultEvents, err := defaultStore.GetCanonicalEvents()
	if err != nil {
		t.Fatalf("default GetCanonicalEvents: %v", err)
	}
	if len(defaultEvents) != 1 || defaultEvents[0].ID != "evt_legacy" {
		t.Fatalf("default events = %#v, want legacy evt_legacy", defaultEvents)
	}

	alphaStore, err := New(dbPath)
	if err != nil {
		t.Fatalf("new alpha store: %v", err)
	}
	defer alphaStore.Close()
	alphaStore.SetInstanceID("alpha")

	alphaEvents, err := alphaStore.GetCanonicalEvents()
	if err != nil {
		t.Fatalf("alpha GetCanonicalEvents: %v", err)
	}
	if len(alphaEvents) != 0 {
		t.Fatalf("alpha events = %#v, want no legacy events", alphaEvents)
	}
}

func TestReopenStoreWithExistingBranchDoesNotDeadlock(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "shared.db")

	store, err := New(dbPath)
	if err != nil {
		t.Fatalf("initial New: %v", err)
	}
	if err := store.Append(core.Event{
		ID:        "evt_branch",
		Type:      "dialogue",
		Actor:     "Anya",
		Branch:    "alt",
		Canonical: true,
		CreatedAt: time.Now(),
	}); err != nil {
		store.Close()
		t.Fatalf("append: %v", err)
	}
	store.Close()

	done := make(chan error, 1)
	go func() {
		reopened, err := New(dbPath)
		if err == nil {
			reopened.Close()
		}
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("reopen New: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("reopen store timed out, possible sqlite self-deadlock")
	}
}
