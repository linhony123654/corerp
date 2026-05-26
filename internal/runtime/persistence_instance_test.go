package runtime

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"corerp/internal/core"
)

func TestInstanceScopedPersistencePaths(t *testing.T) {
	engine := &Engine{
		dataDir:    t.TempDir(),
		instanceID: "alpha",
		playerRole: core.PlayerRole{Name: "玩家"},
		turnTraces: nil,
		charWorlds: map[string]CharWorld{},
		worldPaths: map[string]string{},
		charPaths:  map[string]string{},
	}

	rolePath, err := engine.playerRolePathLocked()
	if err != nil {
		t.Fatalf("playerRolePathLocked: %v", err)
	}
	savePath, err := engine.saveSlotsPath()
	if err != nil {
		t.Fatalf("saveSlotsPath: %v", err)
	}
	wantDir := filepath.Join(engine.dataDir, "instances", "alpha")
	if rolePath != filepath.Join(wantDir, "player_role.json") {
		t.Fatalf("role path = %q, want %q", rolePath, filepath.Join(wantDir, "player_role.json"))
	}
	if savePath != filepath.Join(wantDir, "save_slots.json") {
		t.Fatalf("save path = %q, want %q", savePath, filepath.Join(wantDir, "save_slots.json"))
	}
}

func TestDefaultInstanceReadPlayerRoleFallsBackToLegacyRootPath(t *testing.T) {
	root := t.TempDir()
	role := core.PlayerRole{Name: "旧玩家"}
	data, err := json.Marshal(role)
	if err != nil {
		t.Fatalf("marshal role: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "player_role.json"), data, 0644); err != nil {
		t.Fatalf("write legacy role: %v", err)
	}

	engine := &Engine{dataDir: root, instanceID: "default"}
	got, err := engine.readPlayerRoleLocked()
	if err != nil {
		t.Fatalf("readPlayerRoleLocked: %v", err)
	}
	if got.Name != "旧玩家" {
		t.Fatalf("role name = %q, want 旧玩家", got.Name)
	}
}

func TestDefaultInstanceReadSaveSlotsFallsBackToLegacyRootPath(t *testing.T) {
	root := t.TempDir()
	slots := []core.SaveSlot{{Name: "legacy", Branch: "main"}}
	data, err := json.Marshal(slots)
	if err != nil {
		t.Fatalf("marshal slots: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "save_slots.json"), data, 0644); err != nil {
		t.Fatalf("write legacy slots: %v", err)
	}

	engine := &Engine{dataDir: root, instanceID: "default"}
	got, err := engine.readSaveSlots()
	if err != nil {
		t.Fatalf("readSaveSlots: %v", err)
	}
	if len(got) != 1 || got[0].Name != "legacy" {
		t.Fatalf("slots = %#v, want legacy slot", got)
	}
}

func TestNamedInstanceDoesNotReadLegacyRootFiles(t *testing.T) {
	root := t.TempDir()

	role := core.PlayerRole{Name: "旧默认玩家"}
	roleData, err := json.Marshal(role)
	if err != nil {
		t.Fatalf("marshal role: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "player_role.json"), roleData, 0644); err != nil {
		t.Fatalf("write legacy role: %v", err)
	}

	slots := []core.SaveSlot{{Name: "legacy-default", Branch: "main"}}
	slotData, err := json.Marshal(slots)
	if err != nil {
		t.Fatalf("marshal slots: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "save_slots.json"), slotData, 0644); err != nil {
		t.Fatalf("write legacy slots: %v", err)
	}

	alphaEngine := &Engine{dataDir: root, instanceID: "alpha"}
	alphaRole, err := alphaEngine.readPlayerRoleLocked()
	if err != nil {
		t.Fatalf("alpha readPlayerRoleLocked: %v", err)
	}
	if alphaRole.Name != "玩家" {
		t.Fatalf("alpha role = %#v, want default player role", alphaRole)
	}
	alphaSlots, err := alphaEngine.readSaveSlots()
	if err != nil {
		t.Fatalf("alpha readSaveSlots: %v", err)
	}
	if len(alphaSlots) != 0 {
		t.Fatalf("alpha slots = %#v, want empty", alphaSlots)
	}

	defaultEngine := &Engine{dataDir: root, instanceID: "default"}
	gotRole, err := defaultEngine.readPlayerRoleLocked()
	if err != nil {
		t.Fatalf("default readPlayerRoleLocked: %v", err)
	}
	if gotRole.Name != "旧默认玩家" {
		t.Fatalf("default role = %#v, want legacy default role", gotRole)
	}
	gotSlots, err := defaultEngine.readSaveSlots()
	if err != nil {
		t.Fatalf("default readSaveSlots: %v", err)
	}
	if len(gotSlots) != 1 || gotSlots[0].Name != "legacy-default" {
		t.Fatalf("default slots = %#v, want legacy-default", gotSlots)
	}
}
