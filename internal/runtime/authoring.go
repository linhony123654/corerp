package runtime

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"corerp/internal/core"
	"corerp/internal/events"
)

func (e *Engine) ListTurnTraces(limit int) []core.TurnTrace {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if limit <= 0 || limit > len(e.turnTraces) {
		limit = len(e.turnTraces)
	}
	out := make([]core.TurnTrace, 0, limit)
	for i := len(e.turnTraces) - 1; i >= 0 && len(out) < limit; i-- {
		out = append(out, e.turnTraces[i])
	}
	return out
}

func (e *Engine) ListCheckpoints() ([]core.SaveSlot, error) {
	return e.ListSaveSlots()
}

func (e *Engine) CreateCheckpoint(name, branch, note string) (core.SaveSlot, error) {
	return e.CreateSaveSlot(name, branch, note)
}

func (e *Engine) LoadCheckpoint(name string) (core.SaveSlot, error) {
	return e.LoadSaveSlot(name)
}

func (e *Engine) ListScenarioPresets() ([]core.ScenarioPreset, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	presets, err := e.readScenarioPresetsLocked()
	if err != nil {
		return nil, err
	}
	sort.Slice(presets, func(i, j int) bool {
		return presets[i].CreatedAt.After(presets[j].CreatedAt)
	})
	return presets, nil
}

func (e *Engine) CreateScenarioPreset(name, branch, note string) (core.ScenarioPreset, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	name = strings.TrimSpace(name)
	if name == "" {
		return core.ScenarioPreset{}, fmt.Errorf("preset name is required")
	}
	if branch == "" {
		branch = "main"
	}

	state := e.stateMgr.Get()
	preset := core.ScenarioPreset{
		Name:       name,
		Branch:     branch,
		Character:  e.activeCharacter,
		PlayerRole: normalizePlayerRole(e.playerRole),
		Note:       strings.TrimSpace(note),
		Preview:    state.Scene.Description,
		CreatedAt:  time.Now().UTC(),
		Scene:      state.Scene,
	}

	presets, err := e.readScenarioPresetsLocked()
	if err != nil {
		return core.ScenarioPreset{}, err
	}
	replaced := false
	for i := range presets {
		if presets[i].Name == preset.Name {
			presets[i] = preset
			replaced = true
			break
		}
	}
	if !replaced {
		presets = append(presets, preset)
	}
	if err := e.writeScenarioPresetsLocked(presets); err != nil {
		return core.ScenarioPreset{}, err
	}
	return preset, nil
}

func (e *Engine) ApplyScenarioPreset(name string) (core.ScenarioPreset, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	presets, err := e.readScenarioPresetsLocked()
	if err != nil {
		return core.ScenarioPreset{}, err
	}
	for _, preset := range presets {
		if preset.Name != name {
			continue
		}
		if err := e.switchCharacterLocked(preset.Character, false); err != nil {
			return core.ScenarioPreset{}, err
		}
		e.playerRole = normalizePlayerRole(preset.PlayerRole)

		scene := normalizeSceneForCharacter(preset.Scene, e.activeCharacter, e.playerRoleNameLocked())
		state := e.stateMgr.Get()
		state.Scene = scene
		e.stateMgr.Set(state)

		if cw, ok := e.charWorlds[e.activeCharacter]; ok {
			cw.Scene = preset.Scene
			e.charWorlds[e.activeCharacter] = cw
			e.worldName = cw.WorldName
			e.coreRules = cw.CoreRules
		}

		evt := events.BuildEvent("scene_init", "system", "", map[string]interface{}{
			"location":    scene.Location,
			"time_of_day": scene.TimeOfDay,
			"weather":     scene.Weather,
			"characters":  scene.Characters,
			"description": scene.Description,
			"preset":      preset.Name,
		})
		evt.SessionID = e.sessionID
		evt.SceneID = scene.Location
		if err := e.gatekeeper.Submit(evt, events.SourceSystem()); err != nil {
			return core.ScenarioPreset{}, err
		}

		preset.Scene = scene
		preset.PlayerRole = e.playerRole
		return preset, nil
	}
	return core.ScenarioPreset{}, fmt.Errorf("preset '%s' not found", name)
}

func (e *Engine) scenarioPresetsPathLocked() (string, error) {
	dir, err := e.instanceDataDirLocked()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "scenario_presets.json"), nil
}

func (e *Engine) readScenarioPresetsLocked() ([]core.ScenarioPreset, error) {
	path, err := e.scenarioPresetsPathLocked()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return []core.ScenarioPreset{}, nil
	}
	if err != nil {
		return nil, err
	}
	var presets []core.ScenarioPreset
	if err := json.Unmarshal(data, &presets); err != nil {
		return nil, err
	}
	return presets, nil
}

func (e *Engine) writeScenarioPresetsLocked(presets []core.ScenarioPreset) error {
	path, err := e.scenarioPresetsPathLocked()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(presets, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
