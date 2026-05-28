package runtime

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"corerp/internal/core"
	"corerp/internal/emotion"
	"corerp/internal/events"
	"corerp/internal/memory"
	"corerp/internal/state"
)

func TestManagerRegisterResolveAndList(t *testing.T) {
	mgr := NewManager()
	engine := &Engine{
		stateMgr:         state.New(),
		focusCharacter:   "V",
		loadedCharacters: []string{"V", "Johnny"},
		worldName:        "Night City",
		instanceCreated:  time.Now().UTC(),
	}
	engine.stateMgr.Set(core.WorldState{
		Scene: core.SceneState{
			Location:   "Afterlife",
			Characters: []string{"Rogue", "Panam"},
		},
	})

	if err := mgr.Register("default", "Primary Runtime", engine, true); err != nil {
		t.Fatalf("register: %v", err)
	}

	resolved, err := mgr.Resolve("")
	if err != nil {
		t.Fatalf("resolve default: %v", err)
	}
	if resolved != engine {
		t.Fatalf("resolved engine mismatch")
	}

	summaries := mgr.List()
	if len(summaries) != 1 {
		t.Fatalf("len(list) = %d, want 1", len(summaries))
	}
	got := summaries[0]
	want := core.RuntimeInstanceSummary{
		ID:             "default",
		Label:          "Primary Runtime",
		WorldName:      "Night City",
		FocusCharacter: "V",
		IsDefault:      true,
	}
	if got.ID != want.ID || got.Label != want.Label || got.WorldName != want.WorldName || got.FocusCharacter != want.FocusCharacter || !got.IsDefault {
		t.Fatalf("summary = %#v, want core fields %#v", got, want)
	}
	if len(got.Participants) != 2 || got.Participants[0] != "Rogue" || got.Participants[1] != "Panam" {
		t.Fatalf("participants = %#v, want scene truth [Rogue Panam]", got.Participants)
	}
	if got.Status != InstanceStatusRunning {
		t.Fatalf("status = %q, want %q", got.Status, InstanceStatusRunning)
	}
}

func TestManagerStopAndStatus(t *testing.T) {
	mgr := NewManager()
	engine := &Engine{
		stateMgr:         state.New(),
		focusCharacter:   "V",
		loadedCharacters: []string{"V"},
		worldName:        "Night City",
	}

	if err := mgr.Register("default", "Primary Runtime", engine, true); err != nil {
		t.Fatalf("register: %v", err)
	}
	summary, err := mgr.Stop("default")
	if err != nil {
		t.Fatalf("stop: %v", err)
	}
	if summary.Status != InstanceStatusStopped {
		t.Fatalf("status = %q, want %q", summary.Status, InstanceStatusStopped)
	}
	status, err := mgr.Status("default")
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if status.Status != InstanceStatusStopped {
		t.Fatalf("status = %q, want %q", status.Status, InstanceStatusStopped)
	}
}

func TestManagerDeleteRemovesInstanceData(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(root, "memory.db")
	mem, err := memory.New(dbPath)
	if err != nil {
		t.Fatalf("memory.New: %v", err)
	}
	defer mem.Close()
	mem.SetInstanceID("alt")

	if err := mem.SetWorkingMemory("V", "todo"); err != nil {
		t.Fatalf("set working memory: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "instances", "alt"), 0755); err != nil {
		t.Fatalf("mkdir instance dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "instances", "alt", "player_role.json"), []byte(`{}`), 0644); err != nil {
		t.Fatalf("write instance file: %v", err)
	}

	defaultEngine := &Engine{stateMgr: state.New(), memEngine: mem, dataDir: root}
	altEngine := &Engine{stateMgr: state.New(), memEngine: mem, dataDir: root}
	altEngine.SetInstanceMetadata("alt", time.Now().UTC())

	mgr := NewManager()
	if err := mgr.Register("default", "Primary Runtime", defaultEngine, true); err != nil {
		t.Fatalf("register default: %v", err)
	}
	if err := mgr.Register("alt", "Alt Runtime", altEngine, false); err != nil {
		t.Fatalf("register alt: %v", err)
	}
	if err := mgr.Delete("alt"); err != nil {
		t.Fatalf("delete alt: %v", err)
	}
	if _, err := mgr.Resolve("alt"); err == nil {
		t.Fatal("expected deleted instance to be unresolved")
	}
	if _, err := mem.GetWorkingMemory("V"); err != nil {
		t.Fatalf("get working memory after delete: %v", err)
	}
	if got, _ := mem.GetWorkingMemory("V"); got != "" {
		t.Fatalf("working memory = %q, want empty after delete", got)
	}
	if _, err := os.Stat(filepath.Join(root, "instances", "alt")); !os.IsNotExist(err) {
		t.Fatalf("instance dir still exists, err=%v", err)
	}
}

func TestManagerDeleteCleansAllInstanceScopedData(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(root, "memory.db")

	defaultStore, err := events.New(dbPath)
	if err != nil {
		t.Fatalf("events.New default: %v", err)
	}
	defer defaultStore.Close()
	defaultStore.SetInstanceID("default")

	altStore, err := events.New(dbPath)
	if err != nil {
		t.Fatalf("events.New alt: %v", err)
	}
	defer altStore.Close()
	altStore.SetInstanceID("alt")

	defaultMem, err := memory.New(dbPath)
	if err != nil {
		t.Fatalf("memory.New default: %v", err)
	}
	defer defaultMem.Close()
	defaultMem.SetInstanceID("default")

	altMem, err := memory.New(dbPath)
	if err != nil {
		t.Fatalf("memory.New alt: %v", err)
	}
	defer altMem.Close()
	altMem.SetInstanceID("alt")

	if err := seedInstanceFixture(defaultStore, defaultMem, "default", root, "Neo"); err != nil {
		t.Fatalf("seed default fixture: %v", err)
	}
	if err := seedInstanceFixture(altStore, altMem, "alt", root, "Trinity"); err != nil {
		t.Fatalf("seed alt fixture: %v", err)
	}

	defaultEngine := &Engine{
		stateMgr:   state.New(),
		memEngine:  defaultMem,
		eventStore: defaultStore,
		dataDir:    root,
	}
	defaultEngine.SetInstanceMetadata("default", time.Now().UTC())

	altEngine := &Engine{
		stateMgr:   state.New(),
		memEngine:  altMem,
		eventStore: altStore,
		dataDir:    root,
	}
	altEngine.SetInstanceMetadata("alt", time.Now().UTC())

	mgr := NewManager()
	if err := mgr.Register("default", "Primary Runtime", defaultEngine, true); err != nil {
		t.Fatalf("register default: %v", err)
	}
	if err := mgr.Register("alt", "Alt Runtime", altEngine, false); err != nil {
		t.Fatalf("register alt: %v", err)
	}

	if err := mgr.Delete("alt"); err != nil {
		t.Fatalf("delete alt: %v", err)
	}

	assertInstanceRows(t, defaultMem.DB(), "default", map[string]int{
		"events":           1,
		"branches":         1,
		"dialogue_history": 1,
		"working_memory":   1,
		"semantic_facts":   1,
		"episodic_events":  1,
		"pending_facts":    1,
		"action_log":       1,
	})
	assertInstanceRows(t, defaultMem.DB(), "alt", map[string]int{
		"events":           0,
		"branches":         0,
		"dialogue_history": 0,
		"working_memory":   0,
		"semantic_facts":   0,
		"episodic_events":  0,
		"pending_facts":    0,
		"action_log":       0,
	})

	if _, err := os.Stat(filepath.Join(root, "instances", "alt")); !os.IsNotExist(err) {
		t.Fatalf("alt instance dir still exists, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "instances", "default", "player_role.json")); err != nil {
		t.Fatalf("default player_role.json missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "instances", "default", "save_slots.json")); err != nil {
		t.Fatalf("default save_slots.json missing: %v", err)
	}
}

func TestManagerDeleteRejectsDefaultAndOnlyInstance(t *testing.T) {
	mgr := NewManager()
	defaultEngine := &Engine{stateMgr: state.New()}
	if err := mgr.Register("default", "Primary Runtime", defaultEngine, true); err != nil {
		t.Fatalf("register default: %v", err)
	}
	if err := mgr.Delete("default"); !errors.Is(err, ErrInstanceConflict) {
		t.Fatalf("delete only default err = %v, want ErrInstanceConflict", err)
	}

	altEngine := &Engine{stateMgr: state.New()}
	if err := mgr.Register("alt", "Alt Runtime", altEngine, false); err != nil {
		t.Fatalf("register alt: %v", err)
	}
	if err := mgr.Delete("default"); !errors.Is(err, ErrInstanceConflict) {
		t.Fatalf("delete default err = %v, want ErrInstanceConflict", err)
	}
}

func seedInstanceFixture(store *events.Store, mem *memory.Engine, instanceID, root, playerName string) error {
	if err := store.Append(core.Event{
		ID:        "evt_" + instanceID,
		Type:      "dialogue",
		Actor:     playerName,
		Branch:    "main",
		Canonical: true,
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		return err
	}
	if _, err := mem.DB().Exec(
		`INSERT INTO branches (name, parent_branch, fork_event_id, instance_id, created_at) VALUES (?, ?, ?, ?, ?)`,
		instanceID+"_branch", "main", "evt_"+instanceID, instanceID, time.Now().UTC(),
	); err != nil {
		return err
	}
	mem.PushDialogue(core.Message{Role: "assistant", Content: "hello " + instanceID}, playerName)
	if err := mem.SetWorkingMemory(playerName, "working-"+instanceID); err != nil {
		return err
	}
	if err := mem.RememberFact(core.FactFrame{Subject: playerName, Predicate: "is", Object: instanceID, Confidence: 1}, playerName, 1); err != nil {
		return err
	}
	if err := mem.RecordEpisodic(core.EventFrame{EventID: "epi-" + instanceID, Type: "memory", Description: "episode-" + instanceID, EmotionalWeight: 0.7}, playerName); err != nil {
		return err
	}

	cp := memory.NewConfidencePipelineForInstance(mem.DB(), instanceID)
	if err := cp.Migrate(); err != nil {
		return err
	}
	if err := cp.SubmitFact(core.FactFrame{Subject: playerName, Predicate: "heard", Object: instanceID, Confidence: 0.6}, playerName, "user_input"); err != nil {
		return err
	}

	engine := &Engine{
		stateMgr:   state.New(),
		memEngine:  mem,
		eventStore: store,
		dataDir:    root,
		playerRole: core.PlayerRole{Name: playerName},
	}
	engine.SetInstanceMetadata(instanceID, time.Now().UTC())
	if _, err := engine.UpdatePlayerRole(core.PlayerRole{Name: playerName}); err != nil {
		return err
	}
	engine.actionLogger = emotion.NewActionLogger(10)
	if err := engine.actionLogger.EnablePersistence(mem.DB()); err != nil {
		return err
	}
	engine.actionLogger.SetInstanceID(instanceID)
	engine.actionLogger.Record(emotion.ActionLogEntry{Tick: 1, Character: playerName, Fired: true, ActionType: "approach"})
	if err := engine.writeSaveSlots([]core.SaveSlot{{Name: "slot-" + instanceID, Branch: "main", Character: playerName}}); err != nil {
		return err
	}
	return nil
}

func assertInstanceRows(t *testing.T, db *sql.DB, instanceID string, want map[string]int) {
	t.Helper()
	for table, expected := range want {
		var got int
		if err := db.QueryRow(`SELECT COUNT(*) FROM `+table+` WHERE instance_id = ?`, instanceID).Scan(&got); err != nil {
			t.Fatalf("count %s for %s: %v", table, instanceID, err)
		}
		if got != expected {
			t.Fatalf("%s rows for %s = %d, want %d", table, instanceID, got, expected)
		}
	}
}
