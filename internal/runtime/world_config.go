package runtime

import (
	"fmt"

	"corerp/internal/core"
	"corerp/internal/world"
)

func (e *Engine) GetWorldConfig() (core.WorldConfig, error) {
	e.mu.RLock()
	path := e.worldPaths[e.activeCharacter]
	e.mu.RUnlock()
	if path == "" {
		return core.WorldConfig{}, fmt.Errorf("world path for '%s' is not configured", e.activeCharacter)
	}
	return world.LoadConfig(path)
}

func (e *Engine) UpdateWorldConfig(cfg core.WorldConfig) (core.WorldConfig, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	path := e.worldPaths[e.activeCharacter]
	if path == "" {
		return core.WorldConfig{}, fmt.Errorf("world path for '%s' is not configured", e.activeCharacter)
	}
	saved, err := world.SaveConfig(path, cfg)
	if err != nil {
		return core.WorldConfig{}, err
	}
	if err := e.reloadWorldLocked(path); err != nil {
		return core.WorldConfig{}, err
	}
	return saved, nil
}

func (e *Engine) ListSceneConfigs() (core.SceneConfigList, error) {
	e.mu.RLock()
	path := e.worldPaths[e.activeCharacter]
	e.mu.RUnlock()
	if path == "" {
		return core.SceneConfigList{}, fmt.Errorf("world path for '%s' is not configured", e.activeCharacter)
	}
	return world.ListScenes(path)
}

func (e *Engine) UpdateSceneConfig(scene core.SceneConfig) (core.SceneConfig, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	path := e.worldPaths[e.activeCharacter]
	if path == "" {
		return core.SceneConfig{}, fmt.Errorf("world path for '%s' is not configured", e.activeCharacter)
	}
	saved, err := world.SaveScene(path, scene)
	if err != nil {
		return core.SceneConfig{}, err
	}
	if saved.Name == "default" {
		if err := e.reloadWorldLocked(path); err != nil {
			return core.SceneConfig{}, err
		}
	}
	return saved, nil
}

func (e *Engine) GetCanonFactsConfig() (core.CanonFactsConfig, error) {
	e.mu.RLock()
	path := e.worldPaths[e.activeCharacter]
	e.mu.RUnlock()
	if path == "" {
		return core.CanonFactsConfig{}, fmt.Errorf("world path for '%s' is not configured", e.activeCharacter)
	}
	return world.LoadFacts(path)
}

func (e *Engine) UpdateCanonFactsConfig(cfg core.CanonFactsConfig) (core.CanonFactsConfig, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	path := e.worldPaths[e.activeCharacter]
	if path == "" {
		return core.CanonFactsConfig{}, fmt.Errorf("world path for '%s' is not configured", e.activeCharacter)
	}
	saved, err := world.SaveFacts(path, cfg)
	if err != nil {
		return core.CanonFactsConfig{}, err
	}
	if err := e.reloadWorldLocked(path); err != nil {
		return core.CanonFactsConfig{}, err
	}
	return saved, nil
}

func (e *Engine) GetPopulationConfig() (core.PopulationConfig, error) {
	e.mu.RLock()
	path := e.worldPaths[e.activeCharacter]
	e.mu.RUnlock()
	if path == "" {
		return core.PopulationConfig{}, fmt.Errorf("world path for '%s' is not configured", e.activeCharacter)
	}
	return world.LoadPopulation(path)
}

func (e *Engine) UpdatePopulationConfig(cfg core.PopulationConfig) (core.PopulationConfig, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	path := e.worldPaths[e.activeCharacter]
	if path == "" {
		return core.PopulationConfig{}, fmt.Errorf("world path for '%s' is not configured", e.activeCharacter)
	}
	return world.SavePopulation(path, cfg)
}

func (e *Engine) reloadWorldLocked(path string) error {
	bundle, err := world.LoadBundle(path)
	if err != nil {
		return err
	}
	var defaultScene core.SceneState
	for _, scene := range bundle.Scenes {
		if scene.Name == "default" {
			defaultScene = scene.Scene
			break
		}
	}
	for charName, worldPath := range e.worldPaths {
		if worldPath != path {
			continue
		}
		cw := e.charWorlds[charName]
		cw.WorldName = bundle.Config.Name
		cw.CoreRules = bundle.Config.CoreRules
		if defaultScene.Location != "" || defaultScene.Description != "" || len(defaultScene.Characters) > 0 {
			cw.Scene = defaultScene
		}
		e.charWorlds[charName] = cw
		if err := world.SeedMemory(e.memEngine, bundle, charName); err != nil {
			return err
		}
	}
	e.syncActiveWorldContextLocked()
	return nil
}
