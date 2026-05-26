package runtime

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"corerp/internal/character"
	"corerp/internal/core"
	"corerp/internal/goalexpr"
)

func (e *Engine) ConfigurePersistence(dataDir string, charPaths map[string]string, worldPaths map[string]string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.dataDir = dataDir
	e.charPaths = make(map[string]string, len(charPaths))
	for k, v := range charPaths {
		e.charPaths[k] = v
	}
	e.worldPaths = make(map[string]string, len(worldPaths))
	for k, v := range worldPaths {
		e.worldPaths[k] = v
	}
	if dir, err := e.instanceDataDirLocked(); err == nil {
		_ = os.MkdirAll(dir, 0755)
	}
	if role, err := e.readPlayerRoleLocked(); err == nil {
		e.playerRole = role
	}
}

func (e *Engine) GetMemorySnapshot(characterName string, factLimit, episodicLimit, dialogueLimit int) (core.MemorySnapshot, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	name := characterName
	if name == "" {
		name = e.activeCharacter
	}
	if _, ok := e.agents.GetCharacter(name); !ok {
		return core.MemorySnapshot{}, fmt.Errorf("character '%s' not loaded", name)
	}

	working, _ := e.memEngine.GetWorkingMemory(name)
	facts, _ := e.memEngine.GetAllFacts(name)
	episodic, _ := e.memEngine.GetRecentEpisodic(name, episodicLimit)
	e.memEngine.LoadRecentDialogueFromDB(name, dialogueLimit)
	dialogue := e.memEngine.GetRecentDialogue(name)

	if factLimit > 0 && len(facts) > factLimit {
		facts = facts[:factLimit]
	}
	return core.MemorySnapshot{
		Character:     name,
		WorkingMemory: working,
		Facts:         facts,
		Episodic:      episodic,
		Dialogue:      dialogue,
	}, nil
}

func (e *Engine) GetCharacterConfig(characterName string) (core.CharacterConfig, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	name := characterName
	if name == "" {
		name = e.activeCharacter
	}
	card, ok := e.agents.GetCharacter(name)
	if !ok {
		return core.CharacterConfig{}, fmt.Errorf("character '%s' not loaded", name)
	}
	return core.CharacterConfig{
		Character: name,
		Path:      e.charPaths[name],
		WorldPath: e.worldPaths[name],
		Card:      card,
	}, nil
}

func (e *Engine) UpdateCharacterConfig(characterName string, card core.Character) (core.CharacterConfig, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	name := characterName
	if name == "" {
		name = e.activeCharacter
	}
	path := e.charPaths[name]
	if path == "" {
		return core.CharacterConfig{}, fmt.Errorf("character path for '%s' is not configured", name)
	}
	card.Identity.Name = name
	if err := goalexpr.ValidateCharacter(card); err != nil {
		return core.CharacterConfig{}, err
	}
	if err := character.Save(path, card); err != nil {
		return core.CharacterConfig{}, err
	}

	e.agents.LoadCharacter(name, card)
	return core.CharacterConfig{
		Character: name,
		Path:      path,
		WorldPath: e.worldPaths[name],
		Card:      card,
	}, nil
}

func (e *Engine) ListSaveSlots() ([]core.SaveSlot, error) {
	slots, err := e.readSaveSlots()
	if err != nil {
		return nil, err
	}
	sort.Slice(slots, func(i, j int) bool {
		return slots[i].CreatedAt.After(slots[j].CreatedAt)
	})
	return slots, nil
}

func (e *Engine) CreateSaveSlot(name, branch, note string) (core.SaveSlot, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if strings.TrimSpace(name) == "" {
		return core.SaveSlot{}, fmt.Errorf("save name is required")
	}
	if branch == "" {
		branch = "main"
	}
	timeline, err := e.gatekeeper.Replay().GetTimeline(branch, 1000000)
	if err != nil {
		return core.SaveSlot{}, err
	}
	var eventID string
	if len(timeline) > 0 {
		eventID = timeline[len(timeline)-1].Event.ID
	}
	slot := core.SaveSlot{
		Name:       name,
		Branch:     branch,
		EventID:    eventID,
		Character:  e.activeCharacter,
		PlayerRole: e.playerRole,
		Note:       note,
		Preview:    e.stateMgr.Get().Scene.Description,
		CreatedAt:  time.Now().UTC(),
		WorldState: e.stateMgr.Get(),
	}

	slots, err := e.readSaveSlots()
	if err != nil {
		return core.SaveSlot{}, err
	}

	replaced := false
	for i := range slots {
		if slots[i].Name == slot.Name {
			slots[i] = slot
			replaced = true
			break
		}
	}
	if !replaced {
		slots = append(slots, slot)
	}
	if err := e.writeSaveSlots(slots); err != nil {
		return core.SaveSlot{}, err
	}
	return slot, nil
}

func (e *Engine) LoadSaveSlot(name string) (core.SaveSlot, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	slots, err := e.readSaveSlots()
	if err != nil {
		return core.SaveSlot{}, err
	}
	for _, slot := range slots {
		if slot.Name != name {
			continue
		}
		state := slot.WorldState
		if strings.TrimSpace(slot.EventID) != "" {
			replayed, err := e.gatekeeper.Replay().ReplayTo(slot.EventID, slot.Branch)
			if err != nil {
				return core.SaveSlot{}, err
			}
			state = replayed
		}
		if err := e.switchCharacterLocked(slot.Character, false); err != nil {
			return core.SaveSlot{}, err
		}
		e.playerRole = normalizePlayerRole(slot.PlayerRole)
		e.dialogueHistory = nil
		e.memEngine.LoadRecentDialogueFromDB(e.activeCharacter, 15)
		state.Scene = normalizeSceneForCharacter(state.Scene, e.activeCharacter, e.playerRoleNameLocked())
		e.stateMgr.Set(state)
		if cw, ok := e.charWorlds[e.activeCharacter]; ok {
			e.worldName = cw.WorldName
			e.coreRules = cw.CoreRules
		}
		slot.WorldState = state
		return slot, nil
	}
	return core.SaveSlot{}, fmt.Errorf("save '%s' not found", name)
}

func (e *Engine) playerRolePathLocked() (string, error) {
	dir, err := e.instanceDataDirLocked()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "player_role.json"), nil
}

func (e *Engine) legacyPlayerRolePathLocked() (string, error) {
	if e.dataDir == "" {
		return "", fmt.Errorf("data directory is not configured")
	}
	return filepath.Join(e.dataDir, "player_role.json"), nil
}

func (e *Engine) readPlayerRoleLocked() (core.PlayerRole, error) {
	path, err := e.playerRolePathLocked()
	if err != nil {
		return core.PlayerRole{}, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		if !e.shouldReadLegacyRootLocked() {
			return defaultPlayerRole(), nil
		}
		legacyPath, legacyErr := e.legacyPlayerRolePathLocked()
		if legacyErr != nil {
			return defaultPlayerRole(), nil
		}
		data, err = os.ReadFile(legacyPath)
		if os.IsNotExist(err) {
			return defaultPlayerRole(), nil
		}
	}
	if err != nil {
		return core.PlayerRole{}, err
	}
	var role core.PlayerRole
	if err := json.Unmarshal(data, &role); err != nil {
		return core.PlayerRole{}, err
	}
	return normalizePlayerRole(role), nil
}

func (e *Engine) writePlayerRoleLocked() error {
	if e.dataDir == "" {
		return nil
	}
	path, err := e.playerRolePathLocked()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(normalizePlayerRole(e.playerRole), "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func (e *Engine) readSaveSlots() ([]core.SaveSlot, error) {
	path, err := e.saveSlotsPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		if !e.shouldReadLegacyRootLocked() {
			return []core.SaveSlot{}, nil
		}
		legacyPath, legacyErr := e.legacySaveSlotsPath()
		if legacyErr != nil {
			return []core.SaveSlot{}, nil
		}
		data, err = os.ReadFile(legacyPath)
		if os.IsNotExist(err) {
			return []core.SaveSlot{}, nil
		}
	}
	if err != nil {
		return nil, err
	}
	var slots []core.SaveSlot
	if err := json.Unmarshal(data, &slots); err != nil {
		return nil, err
	}
	return slots, nil
}

func (e *Engine) writeSaveSlots(slots []core.SaveSlot) error {
	path, err := e.saveSlotsPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(slots, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func (e *Engine) saveSlotsPath() (string, error) {
	dir, err := e.instanceDataDirLocked()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "save_slots.json"), nil
}

func (e *Engine) legacySaveSlotsPath() (string, error) {
	if e.dataDir == "" {
		return "", fmt.Errorf("data directory is not configured")
	}
	return filepath.Join(e.dataDir, "save_slots.json"), nil
}

func (e *Engine) shouldReadLegacyRootLocked() bool {
	return strings.TrimSpace(e.instanceID) == "default"
}

func (e *Engine) instanceDataDirLocked() (string, error) {
	if e.dataDir == "" {
		return "", fmt.Errorf("data directory is not configured")
	}
	instanceID := strings.TrimSpace(e.instanceID)
	if instanceID == "" {
		instanceID = "default"
	}
	return filepath.Join(e.dataDir, "instances", instanceID), nil
}
