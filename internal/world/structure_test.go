package world

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"corerp/internal/core"
)

func TestLoadAndSaveWorldStructureConfig(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "world.yml"), []byte("meta:\n  name: test\ncore_rules: seed\n"), 0644); err != nil {
		t.Fatalf("write world.yml: %v", err)
	}

	cfg, err := SaveStructure(dir, core.WorldStructureConfig{
		Ruleset: core.WorldRulesetConfig{
			Rules: []core.WorldRule{{
				ID:          "magic_cost",
				Title:       "施法代价",
				Summary:     "高阶施法会消耗寿命",
				Constraints: []string{"高阶法术不能无代价释放"},
				Effects:     []string{"角色会规避滥用魔法"},
			}},
		},
		Seed: core.WorldSeedConfig{
			Premise:          "王朝末年，边境失控。",
			CurrentSituation: "都城谣言四起。",
			StartingScene:    "皇城外街",
			TimeAnchor:       "秋末",
			Stability:        "fragile",
			Variables: map[string]interface{}{
				"dynasty": "大梁",
			},
		},
		Factions: []core.WorldFactionConfig{{
			ID:            "court",
			Name:          "朝廷",
			Role:          "central_power",
			Description:   "名义上的最高权力",
			Goals:         []string{"维持统治"},
			Relationships: []string{"与边军互相猜忌"},
		}},
		Locations: []core.WorldLocationConfig{{
			ID:          "outer_city",
			Name:        "外城",
			Kind:        "district",
			Description: "流民与商贩混杂",
			Controller:  "朝廷",
			Tags:        []string{"拥挤", "危险"},
		}},
		Pressures: []core.WorldPressureConfig{{
			ID:          "grain_crisis",
			Name:        "粮荒",
			Kind:        "scarcity",
			Description: "粮价持续上涨",
			Intensity:   0.8,
			Target:      "外城",
			Escalates:   []string{"骚乱", "抢粮"},
		}},
	})
	if err != nil {
		t.Fatalf("SaveStructure: %v", err)
	}
	if cfg.Path != filepath.ToSlash(filepath.Clean(dir)) {
		t.Fatalf("path = %q", cfg.Path)
	}

	loaded, err := LoadStructure(dir)
	if err != nil {
		t.Fatalf("LoadStructure: %v", err)
	}
	if len(loaded.Ruleset.Rules) != 1 || loaded.Ruleset.Rules[0].ID != "magic_cost" {
		t.Fatalf("ruleset = %#v", loaded.Ruleset)
	}
	if loaded.Seed.StartingScene != "皇城外街" || loaded.Seed.Variables["dynasty"] != "大梁" {
		t.Fatalf("seed = %#v", loaded.Seed)
	}
	if len(loaded.Factions) != 1 || loaded.Factions[0].ID != "court" {
		t.Fatalf("factions = %#v", loaded.Factions)
	}
	if len(loaded.Locations) != 1 || loaded.Locations[0].ID != "outer_city" {
		t.Fatalf("locations = %#v", loaded.Locations)
	}
	if len(loaded.Pressures) != 1 || loaded.Pressures[0].ID != "grain_crisis" {
		t.Fatalf("pressures = %#v", loaded.Pressures)
	}
}

func TestLoadStructureDefaultsWhenMissing(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "world.yml"), []byte("meta:\n  name: test\ncore_rules: seed\n"), 0644); err != nil {
		t.Fatalf("write world.yml: %v", err)
	}

	cfg, err := LoadStructure(dir)
	if err != nil {
		t.Fatalf("LoadStructure: %v", err)
	}
	if cfg.Ruleset.Path == "" || cfg.Seed.Path == "" {
		t.Fatalf("paths = %#v", cfg)
	}
	if cfg.Ruleset.Rules == nil || cfg.Factions == nil || cfg.Locations == nil || cfg.Pressures == nil {
		t.Fatalf("expected empty slices, got %#v", cfg)
	}
	if cfg.Seed.Variables == nil {
		t.Fatalf("expected seed variables map, got %#v", cfg.Seed)
	}
}

func TestLoadAndSaveDirectorConfig(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "world.yml"), []byte("meta:\n  name: test\ncore_rules: seed\n"), 0644); err != nil {
		t.Fatalf("write world.yml: %v", err)
	}

	saved, err := SaveDirectorConfig(dir, core.DirectorConfig{
		Mode:        "auto_chain",
		MaxSpeakers: 2,
		Weights: map[string]float64{
			"mentioned":      77,
			"pressure_match": 11,
		},
	})
	if err != nil {
		t.Fatalf("SaveDirectorConfig: %v", err)
	}
	if saved.Weights["mentioned"] != 77 {
		t.Fatalf("saved weights = %#v", saved.Weights)
	}

	loaded, err := LoadDirectorConfig(dir)
	if err != nil {
		t.Fatalf("LoadDirectorConfig: %v", err)
	}
	if loaded.Mode != "auto_chain" || loaded.MaxSpeakers != 2 {
		t.Fatalf("loaded director = %#v", loaded)
	}
	if loaded.Weights["mentioned"] != 77 || loaded.Weights["pressure_match"] != 11 {
		t.Fatalf("loaded weights = %#v", loaded.Weights)
	}
	if loaded.Weights["present"] == 0 {
		t.Fatalf("expected default weights merged, got %#v", loaded.Weights)
	}
}

func TestNeonBlockWorldBundleLoadsDemoContent(t *testing.T) {
	src := filepath.Join("..", "..", "worlds", "neon_block")
	dir := filepath.Join(t.TempDir(), "neon-block")
	copyWorldDir(t, src, dir)

	bundle, err := LoadBundle(dir)
	if err != nil {
		t.Fatalf("LoadBundle: %v", err)
	}
	if bundle.Config.Name != "霓虹里街区" {
		t.Fatalf("bundle.Config.Name = %q", bundle.Config.Name)
	}
	if len(bundle.DirectFacts) < 4 {
		t.Fatalf("direct facts = %#v, want seeded demo facts", bundle.DirectFacts)
	}
	if len(bundle.Ontology.Characters) < 2 || len(bundle.Ontology.Locations) < 2 {
		t.Fatalf("ontology = %#v, want demo ontology content", bundle.Ontology)
	}
	if len(bundle.Scenes) == 0 || len(bundle.Scenes[0].Scene.Characters) < 2 {
		t.Fatalf("scene = %#v, want opening cast", bundle.Scenes)
	}

	director, err := LoadDirectorConfig(dir)
	if err != nil {
		t.Fatalf("LoadDirectorConfig: %v", err)
	}
	if director.Weights["pressure_match"] != 11 || director.MaxSpeakers != 2 {
		t.Fatalf("director = %#v, want neon block director defaults", director)
	}

	presets, err := LoadScenarioPresets(dir)
	if err != nil {
		t.Fatalf("LoadScenarioPresets: %v", err)
	}
	if len(presets) == 0 || presets[0].Name != "opening_witness_conflict" {
		t.Fatalf("presets = %#v, want neon block opening preset", presets)
	}
}

func copyWorldDir(t *testing.T, src, dst string) {
	t.Helper()
	if _, err := os.Stat(src); err != nil {
		if os.IsNotExist(err) {
			t.Skipf("local world fixture %s is not present", src)
		}
		t.Fatalf("stat local world fixture %s: %v", src, err)
	}
	if err := filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.Create(target)
		if err != nil {
			return err
		}
		defer out.Close()
		if _, err := io.Copy(out, in); err != nil {
			return err
		}
		return out.Chmod(info.Mode())
	}); err != nil {
		t.Fatalf("copy world dir %s -> %s: %v", src, dst, err)
	}
}
