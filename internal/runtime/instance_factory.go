package runtime

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"corerp/internal/core"
	"corerp/internal/events"
	"corerp/internal/memory"
	"corerp/internal/world"
)

func (e *Engine) SpawnInstance(instanceID, activeCharacter string) (*Engine, error) {
	e.mu.RLock()
	dataDir := e.dataDir
	loadedCharacters := append([]string(nil), e.loadedCharacters...)
	charWorlds := make(map[string]CharWorld, len(e.charWorlds))
	for k, v := range e.charWorlds {
		charWorlds[k] = v
	}
	charPaths := make(map[string]string, len(e.charPaths))
	for k, v := range e.charPaths {
		charPaths[k] = v
	}
	worldPaths := make(map[string]string, len(e.worldPaths))
	for k, v := range e.worldPaths {
		worldPaths[k] = v
	}
	agentsMgr := e.agents
	llmRouter := e.llmRouter
	playerRole := e.playerRole
	currentActive := e.activeCharacter
	e.mu.RUnlock()

	if strings.TrimSpace(instanceID) == "" {
		return nil, fmt.Errorf("instance id required")
	}
	if strings.TrimSpace(activeCharacter) == "" {
		activeCharacter = currentActive
	}

	dbPath := filepath.Join(dataDir, "memory.db")
	store, err := events.New(dbPath)
	if err != nil {
		return nil, err
	}
	store.SetInstanceID(instanceID)

	memEngine, err := memory.New(dbPath)
	if err != nil {
		store.Close()
		return nil, err
	}
	memEngine.SetInstanceID(instanceID)

	decayEngine := memory.NewDecayEngine(memEngine.DB())
	decayEngine.SetInstanceID(instanceID)
	gatekeeper := events.NewGatekeeper(store)

	engine, err := New(
		store,
		gatekeeper,
		memEngine,
		decayEngine,
		agentsMgr,
		llmRouter,
		activeCharacter,
		loadedCharacters,
		charWorlds,
	)
	if err != nil {
		store.Close()
		memEngine.Close()
		return nil, err
	}
	engine.SetInstanceMetadata(instanceID, time.Now().UTC())
	engine.ConfigurePersistence(dataDir, charPaths, worldPaths)

	for name, path := range worldPaths {
		if strings.TrimSpace(path) == "" {
			continue
		}
		bundle, err := world.LoadBundle(path)
		if err != nil {
			continue
		}
		_ = world.SeedMemory(memEngine, bundle, name)
	}

	if err := engine.LoadState(); err != nil {
		return nil, err
	}
	if err := gatekeeper.Causality().RebuildAll(); err != nil {
		return nil, err
	}
	if instanceSceneIsEmpty(engine.GetState().Scene) {
		if cw, ok := charWorlds[activeCharacter]; ok {
			engine.SeedScene(cw.Scene)
		}
	}
	engine.SyncActiveWorldContext()
	if _, err := engine.UpdatePlayerRole(playerRole); err != nil {
		return nil, err
	}
	return engine, nil
}

func instanceSceneIsEmpty(scene core.SceneState) bool {
	return scene.Location == "" &&
		scene.TimeOfDay == "" &&
		scene.Weather == "" &&
		scene.Description == "" &&
		len(scene.Characters) == 0
}
