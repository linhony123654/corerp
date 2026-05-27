package world

import (
	"fmt"
	"os"
	"path/filepath"

	"corerp/internal/core"

	"gopkg.in/yaml.v3"
)

type worldRulesetDoc struct {
	Rules []core.WorldRule `yaml:"rules"`
}

type worldSeedDoc struct {
	Premise          string                 `yaml:"premise"`
	CurrentSituation string                 `yaml:"current_situation"`
	StartingScene    string                 `yaml:"starting_scene"`
	TimeAnchor       string                 `yaml:"time_anchor"`
	Stability        string                 `yaml:"stability"`
	Variables        map[string]interface{} `yaml:"variables"`
}

type worldFactionsDoc struct {
	Factions []core.WorldFactionConfig `yaml:"factions"`
}

type worldLocationsDoc struct {
	Locations []core.WorldLocationConfig `yaml:"locations"`
}

type worldPressuresDoc struct {
	Pressures []core.WorldPressureConfig `yaml:"pressures"`
}

type worldDirectorDoc struct {
	Director core.DirectorConfig `yaml:"director"`
}

func LoadStructure(path string) (core.WorldStructureConfig, error) {
	b, err := LoadBundle(path)
	if err != nil {
		return core.WorldStructureConfig{}, err
	}
	if b.Config.Format != "world_dir" {
		return core.WorldStructureConfig{}, fmt.Errorf("single-file world does not support world structure editing; import into directory format first")
	}
	return readDirStructure(b.Config.Path)
}

func SaveStructure(path string, cfg core.WorldStructureConfig) (core.WorldStructureConfig, error) {
	if !isDirPath(path) {
		return core.WorldStructureConfig{}, fmt.Errorf("single-file world does not support world structure editing; import into directory format first")
	}
	return saveDirStructure(path, cfg)
}

func LoadDirectorConfig(path string) (core.DirectorConfig, error) {
	if !isDirPath(path) {
		return core.DirectorConfig{}, fmt.Errorf("single-file world does not support director config editing; import into directory format first")
	}
	return readDirDirectorConfig(path)
}

func SaveDirectorConfig(path string, cfg core.DirectorConfig) (core.DirectorConfig, error) {
	if !isDirPath(path) {
		return core.DirectorConfig{}, fmt.Errorf("single-file world does not support director config editing; import into directory format first")
	}
	return saveDirDirectorConfig(path, cfg)
}

func readDirStructure(dir string) (core.WorldStructureConfig, error) {
	cfg := normalizeWorldStructureConfig(defaultWorldStructureConfig(dir), dir)
	worldDir := filepath.Join(dir, "world")

	if data, err := os.ReadFile(filepath.Join(worldDir, "ruleset.yml")); err == nil {
		var raw worldRulesetDoc
		if err := yaml.Unmarshal(data, &raw); err != nil {
			return core.WorldStructureConfig{}, err
		}
		cfg.Ruleset.Rules = raw.Rules
	} else if !os.IsNotExist(err) {
		return core.WorldStructureConfig{}, err
	}

	if data, err := os.ReadFile(filepath.Join(worldDir, "seed.yml")); err == nil {
		var raw worldSeedDoc
		if err := yaml.Unmarshal(data, &raw); err != nil {
			return core.WorldStructureConfig{}, err
		}
		cfg.Seed.Premise = raw.Premise
		cfg.Seed.CurrentSituation = raw.CurrentSituation
		cfg.Seed.StartingScene = raw.StartingScene
		cfg.Seed.TimeAnchor = raw.TimeAnchor
		cfg.Seed.Stability = raw.Stability
		cfg.Seed.Variables = raw.Variables
	} else if !os.IsNotExist(err) {
		return core.WorldStructureConfig{}, err
	}

	if data, err := os.ReadFile(filepath.Join(worldDir, "factions.yml")); err == nil {
		var raw worldFactionsDoc
		if err := yaml.Unmarshal(data, &raw); err != nil {
			return core.WorldStructureConfig{}, err
		}
		cfg.Factions = raw.Factions
	} else if !os.IsNotExist(err) {
		return core.WorldStructureConfig{}, err
	}

	if data, err := os.ReadFile(filepath.Join(worldDir, "locations.yml")); err == nil {
		var raw worldLocationsDoc
		if err := yaml.Unmarshal(data, &raw); err != nil {
			return core.WorldStructureConfig{}, err
		}
		cfg.Locations = raw.Locations
	} else if !os.IsNotExist(err) {
		return core.WorldStructureConfig{}, err
	}

	if data, err := os.ReadFile(filepath.Join(worldDir, "pressures.yml")); err == nil {
		var raw worldPressuresDoc
		if err := yaml.Unmarshal(data, &raw); err != nil {
			return core.WorldStructureConfig{}, err
		}
		cfg.Pressures = raw.Pressures
	} else if !os.IsNotExist(err) {
		return core.WorldStructureConfig{}, err
	}

	return normalizeWorldStructureConfig(cfg, dir), nil
}

func saveDirStructure(dir string, cfg core.WorldStructureConfig) (core.WorldStructureConfig, error) {
	cfg = normalizeWorldStructureConfig(cfg, dir)
	worldDir := filepath.Join(dir, "world")
	if err := os.MkdirAll(worldDir, 0755); err != nil {
		return core.WorldStructureConfig{}, err
	}
	files := []struct {
		name string
		doc  interface{}
	}{
		{name: "ruleset.yml", doc: worldRulesetDoc{Rules: cfg.Ruleset.Rules}},
		{name: "seed.yml", doc: worldSeedDoc{
			Premise:          cfg.Seed.Premise,
			CurrentSituation: cfg.Seed.CurrentSituation,
			StartingScene:    cfg.Seed.StartingScene,
			TimeAnchor:       cfg.Seed.TimeAnchor,
			Stability:        cfg.Seed.Stability,
			Variables:        cfg.Seed.Variables,
		}},
		{name: "factions.yml", doc: worldFactionsDoc{Factions: cfg.Factions}},
		{name: "locations.yml", doc: worldLocationsDoc{Locations: cfg.Locations}},
		{name: "pressures.yml", doc: worldPressuresDoc{Pressures: cfg.Pressures}},
	}
	for _, file := range files {
		data, err := yaml.Marshal(file.doc)
		if err != nil {
			return core.WorldStructureConfig{}, err
		}
		if err := os.WriteFile(filepath.Join(worldDir, file.name), data, 0644); err != nil {
			return core.WorldStructureConfig{}, err
		}
	}
	return cfg, nil
}

func readDirDirectorConfig(dir string) (core.DirectorConfig, error) {
	cfg := defaultDirectorConfig()
	data, err := os.ReadFile(filepath.Join(dir, "world", "director.yml"))
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return core.DirectorConfig{}, err
	}
	var raw worldDirectorDoc
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return core.DirectorConfig{}, err
	}
	return normalizeDirectorConfig(raw.Director), nil
}

func saveDirDirectorConfig(dir string, cfg core.DirectorConfig) (core.DirectorConfig, error) {
	cfg = normalizeDirectorConfig(cfg)
	worldDir := filepath.Join(dir, "world")
	if err := os.MkdirAll(worldDir, 0755); err != nil {
		return core.DirectorConfig{}, err
	}
	data, err := yaml.Marshal(worldDirectorDoc{Director: cfg})
	if err != nil {
		return core.DirectorConfig{}, err
	}
	if err := os.WriteFile(filepath.Join(worldDir, "director.yml"), data, 0644); err != nil {
		return core.DirectorConfig{}, err
	}
	return cfg, nil
}

func defaultWorldStructureConfig(path string) core.WorldStructureConfig {
	clean := filepath.ToSlash(filepath.Clean(path))
	return core.WorldStructureConfig{
		Path: clean,
		Ruleset: core.WorldRulesetConfig{
			Path:  clean + "/world/ruleset.yml",
			Rules: []core.WorldRule{},
		},
		Seed: core.WorldSeedConfig{
			Path:      clean + "/world/seed.yml",
			Variables: map[string]interface{}{},
		},
		Factions:  []core.WorldFactionConfig{},
		Locations: []core.WorldLocationConfig{},
		Pressures: []core.WorldPressureConfig{},
	}
}

func normalizeWorldStructureConfig(cfg core.WorldStructureConfig, path string) core.WorldStructureConfig {
	clean := filepath.ToSlash(filepath.Clean(path))
	cfg.Path = clean
	cfg.Ruleset.Path = clean + "/world/ruleset.yml"
	cfg.Seed.Path = clean + "/world/seed.yml"
	if cfg.Ruleset.Rules == nil {
		cfg.Ruleset.Rules = []core.WorldRule{}
	}
	if cfg.Seed.Variables == nil {
		cfg.Seed.Variables = map[string]interface{}{}
	}
	if cfg.Factions == nil {
		cfg.Factions = []core.WorldFactionConfig{}
	}
	if cfg.Locations == nil {
		cfg.Locations = []core.WorldLocationConfig{}
	}
	if cfg.Pressures == nil {
		cfg.Pressures = []core.WorldPressureConfig{}
	}
	return cfg
}

func normalizeDirectorConfig(cfg core.DirectorConfig) core.DirectorConfig {
	switch cfg.Mode {
	case "auto_single", "auto_chain":
	default:
		cfg.Mode = "manual"
	}
	if cfg.MaxSpeakers <= 0 {
		cfg.MaxSpeakers = 1
	}
	if cfg.Mode == "auto_single" || cfg.Mode == "manual" {
		cfg.MaxSpeakers = 1
	}
	if cfg.MaxSpeakers > 3 {
		cfg.MaxSpeakers = 3
	}
	cfg.Weights = normalizeDirectorWeights(cfg.Weights)
	return cfg
}

func normalizeDirectorWeights(weights map[string]float64) map[string]float64 {
	out := map[string]float64{
		"mentioned":       100,
		"mention_order":   12,
		"continuity":      4,
		"present":         8,
		"location_match":  9,
		"faction_match":   6,
		"pressure_match":  7,
		"hook_match":      12,
		"silence_cap":     12,
		"silence_divisor": 5,
		"trust":           0.15,
		"intimacy":        0.1,
		"fear":            0.05,
		"opened_by_user":  6,
		"tension_switch":  3,
	}
	for key, value := range weights {
		if value == 0 {
			continue
		}
		out[key] = value
	}
	return out
}

func defaultDirectorConfig() core.DirectorConfig {
	return normalizeDirectorConfig(core.DirectorConfig{
		Mode:        "auto_chain",
		MaxSpeakers: 2,
	})
}
