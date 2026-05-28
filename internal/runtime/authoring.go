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
	"corerp/internal/world"
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

func (e *Engine) ListExperimentReports() ([]core.ExperimentReport, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	reports, err := e.readExperimentReportsLocked()
	if err != nil {
		return nil, err
	}
	sort.Slice(reports, func(i, j int) bool {
		return reports[i].CreatedAt.After(reports[j].CreatedAt)
	})
	for i := range reports {
		reports[i] = normalizeExperimentReportCompatibility(reports[i])
	}
	return reports, nil
}

func (e *Engine) CreateExperimentReport(report core.ExperimentReport) (core.ExperimentReport, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	report = normalizeExperimentReportCompatibility(report)
	report.Name = strings.TrimSpace(report.Name)
	if report.Name == "" {
		return core.ExperimentReport{}, fmt.Errorf("experiment report name is required")
	}
	report.Note = strings.TrimSpace(report.Note)
	if report.BatchCount < 0 {
		report.BatchCount = 0
	}
	if report.CreatedAt.IsZero() {
		report.CreatedAt = time.Now().UTC()
	} else {
		report.CreatedAt = report.CreatedAt.UTC()
	}

	reports, err := e.readExperimentReportsLocked()
	if err != nil {
		return core.ExperimentReport{}, err
	}
	replaced := false
	for i := range reports {
		if reports[i].Name == report.Name {
			reports[i] = report
			replaced = true
			break
		}
	}
	if !replaced {
		reports = append(reports, report)
	}
	if err := e.writeExperimentReportsLocked(reports); err != nil {
		return core.ExperimentReport{}, err
	}
	return report, nil
}

func (e *Engine) CreateCheckpoint(name, branch, note string) (core.SaveSlot, error) {
	return e.CreateSaveSlot(name, branch, note)
}

func (e *Engine) LoadCheckpoint(name string) (core.SaveSlot, error) {
	return e.LoadSaveSlot(name)
}

func (e *Engine) EnterWorld(path string) (core.ScenarioPreset, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	path = strings.TrimSpace(path)
	if path == "" {
		return core.ScenarioPreset{}, fmt.Errorf("world path is required")
	}
	bundle, err := world.LoadBundle(path)
	if err != nil {
		return core.ScenarioPreset{}, err
	}

	scene := defaultBundleScene(bundle)
	preset := core.ScenarioPreset{
		Name:           "world_default",
		Branch:         "main",
		FocusCharacter: firstSceneCharacter(scene),
		PlayerRole:     normalizePlayerRole(e.playerRole),
		Preview:        scene.Description,
		CreatedAt:      time.Now().UTC(),
		Scene:          scene,
	}
	preset = normalizeScenarioPresetCompatibility(preset)
	if presets, err := world.LoadScenarioPresets(path); err == nil && len(presets) > 0 {
		preset = normalizeScenarioPresetCompatibility(presets[0])
		if preset.Scene.Location != "" || len(preset.Scene.Characters) > 0 || preset.Scene.Description != "" {
			scene = preset.Scene
		}
	}
	if preset.FocusCharacter == "" {
		preset.FocusCharacter = firstSceneCharacter(scene)
		preset = normalizeScenarioPresetCompatibility(preset)
	}
	if preset.FocusCharacter == "" && len(bundle.Population.BackgroundNPCs) > 0 {
		preset.FocusCharacter = bundle.Population.BackgroundNPCs[0].Name
		preset = normalizeScenarioPresetCompatibility(preset)
	}
	if preset.FocusCharacter == "" {
		return core.ScenarioPreset{}, fmt.Errorf("world '%s' has no playable character or population candidate", bundle.Config.Name)
	}

	e.activeWorldPath = path
	e.loadedCharacters = []string{preset.FocusCharacter}
	e.worldPaths = map[string]string{}
	e.worldPaths[preset.FocusCharacter] = path
	e.charWorlds = map[string]CharWorld{}
	e.charWorlds[preset.FocusCharacter] = CharWorld{
		WorldName: bundle.Config.Name,
		CoreRules: bundle.Config.CoreRules,
		Scene:     scene,
	}
	e.focusCharacter = preset.FocusCharacter
	if err := e.ensureWorldCharacterLocked(preset.FocusCharacter, scene); err != nil {
		e.agents.LoadCharacter(preset.FocusCharacter, core.Character{
			WorldPath: path,
			Identity: core.IdentityEnvelope{
				Name:         preset.FocusCharacter,
				Immutable:    []string{"world entrant"},
				Adaptive:     map[string]float64{"trust": 3, "fear": 2},
				WritingGuide: "follow current world scene and committed events",
			},
		})
		if !containsString(e.loadedCharacters, preset.FocusCharacter) {
			e.loadedCharacters = append(e.loadedCharacters, preset.FocusCharacter)
		}
	}

	e.playerRole = normalizePlayerRole(preset.PlayerRole)
	e.syncActiveWorldContextLocked()

	scene = normalizeSceneForCharacter(scene, e.GetFocusCharacter(), e.playerRoleNameLocked())
	state := e.stateMgr.Get()
	state.Scene = scene
	e.stateMgr.Set(state)
	e.memEngine.LoadRecentDialogueFromDB(e.GetFocusCharacter(), 15)

	evt := events.BuildEvent("scene_init", "system", "", map[string]interface{}{
		"location":    scene.Location,
		"time_of_day": scene.TimeOfDay,
		"weather":     scene.Weather,
		"characters":  scene.Characters,
		"description": scene.Description,
		"preset":      preset.Name,
		"world":       bundle.Config.Name,
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
		Name:           name,
		Branch:         branch,
		FocusCharacter: e.GetFocusCharacter(),
		PlayerRole:     normalizePlayerRole(e.playerRole),
		Note:           strings.TrimSpace(note),
		Preview:        state.Scene.Description,
		CreatedAt:      time.Now().UTC(),
		Scene:          state.Scene,
	}
	preset = normalizeScenarioPresetCompatibility(preset)

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
		preset = normalizeScenarioPresetCompatibility(preset)
		if _, ok := e.agents.GetCharacter(preset.FocusCharacter); !ok {
			if err := e.ensureWorldCharacterLocked(preset.FocusCharacter, preset.Scene); err != nil {
				return core.ScenarioPreset{}, err
			}
		}
		if err := e.switchCharacterLocked(preset.FocusCharacter, false); err != nil {
			return core.ScenarioPreset{}, err
		}
		e.playerRole = normalizePlayerRole(preset.PlayerRole)

		scene := normalizeSceneForCharacter(preset.Scene, e.GetFocusCharacter(), e.playerRoleNameLocked())
		state := e.stateMgr.Get()
		state.Scene = scene
		e.stateMgr.Set(state)

		if cw, ok := e.charWorlds[e.GetFocusCharacter()]; ok {
			cw.Scene = preset.Scene
			e.charWorlds[e.GetFocusCharacter()] = cw
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

func (e *Engine) ensureWorldCharacterLocked(name string, scene core.SceneState) error {
	path := e.currentWorldPathLocked()
	if path == "" {
		return fmt.Errorf("world path for '%s' is not configured", name)
	}
	cfg, _, err := world.EnsureSeededPopulation(path)
	if err != nil {
		return err
	}
	activeWorld := e.charWorlds[e.GetFocusCharacter()]
	e.ensurePromotedCharactersLoadedLocked(path, cfg)
	for _, npc := range cfg.BackgroundNPCs {
		if npc.Name != name {
			continue
		}
		if scene.Location != "" {
			npc.Location = scene.Location
		}
		e.ensureBackgroundNPCLoadedLocked(path, activeWorld, npc)
		return nil
	}
	if _, ok := e.agents.GetCharacter(name); ok {
		return nil
	}
	return fmt.Errorf("character '%s' not loaded", name)
}

func (e *Engine) scenarioPresetsPathLocked() (string, error) {
	dir, err := e.instanceDataDirLocked()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "scenario_presets.json"), nil
}

func (e *Engine) readScenarioPresetsLocked() ([]core.ScenarioPreset, error) {
	presetsByName := map[string]core.ScenarioPreset{}
	if path := e.currentWorldPathLocked(); path != "" {
		worldPresets, err := world.LoadScenarioPresets(path)
		if err != nil {
			return nil, err
		}
		for _, preset := range worldPresets {
			if preset.Name == "" {
				continue
			}
			preset = normalizeScenarioPresetCompatibility(preset)
			presetsByName[preset.Name] = preset
		}
	}

	path, err := e.scenarioPresetsPathLocked()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return mapScenarioPresetsToSlice(presetsByName), nil
	}
	if err != nil {
		return nil, err
	}
	var presets []core.ScenarioPreset
	if err := json.Unmarshal(data, &presets); err != nil {
		return nil, err
	}
	for _, preset := range presets {
		if preset.Name == "" {
			continue
		}
		preset = normalizeScenarioPresetCompatibility(preset)
		presetsByName[preset.Name] = preset
	}
	return mapScenarioPresetsToSlice(presetsByName), nil
}

func (e *Engine) writeScenarioPresetsLocked(presets []core.ScenarioPreset) error {
	for i := range presets {
		presets[i] = normalizeScenarioPresetCompatibility(presets[i])
	}
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

func mapScenarioPresetsToSlice(items map[string]core.ScenarioPreset) []core.ScenarioPreset {
	out := make([]core.ScenarioPreset, 0, len(items))
	for _, preset := range items {
		out = append(out, preset)
	}
	return out
}

func defaultBundleScene(bundle world.Bundle) core.SceneState {
	if len(bundle.Scenes) == 0 {
		return core.SceneState{}
	}
	for _, scene := range bundle.Scenes {
		if scene.Name == "default" {
			return scene.Scene
		}
	}
	return bundle.Scenes[0].Scene
}

func firstSceneCharacter(scene core.SceneState) string {
	for _, name := range scene.Characters {
		name = strings.TrimSpace(name)
		if name == "" || isPlayerPlaceholder(name) {
			continue
		}
		return name
	}
	return ""
}
