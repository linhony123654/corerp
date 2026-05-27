package world

import (
	"os"
	"path/filepath"
	"testing"

	"corerp/internal/core"
)

func TestLoadAndSavePopulationConfig(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "world.yml"), []byte("meta:\n  name: test\ncore_rules: seed\n"), 0644); err != nil {
		t.Fatalf("write world.yml: %v", err)
	}
	cfg, err := SavePopulation(dir, core.PopulationConfig{
		BackgroundNPCs: []core.BackgroundNPC{{
			ID:       "tea_vendor",
			Name:     "茶摊老板",
			Role:     "商贩",
			Location: "镇口",
			Traits:   []string{"健谈", "精明"},
		}},
		PromotedNPCs: []core.PromotedNPC{{
			ID:           "tea_vendor",
			Name:         "茶摊老板",
			From:         "background",
			Status:       "promoted",
			IdentityCore: "tea_vendor_core",
		}},
		IdentityCores: []core.IdentityCoreConfig{{
			ID:          "tea_vendor_core",
			Name:        "茶摊老板",
			Immutable:   []string{"不愿惹事"},
			SpeechHints: []string{"市井", "圆滑"},
			Drives:      []string{"保住茶摊"},
		}},
		Policy: core.PromotionPolicy{
			PromoteThreshold:  12,
			MajorThreshold:    30,
			InteractionWeight: 4,
		},
	})
	if err != nil {
		t.Fatalf("SavePopulation: %v", err)
	}
	if cfg.Path != filepath.ToSlash(filepath.Clean(dir)) {
		t.Fatalf("path = %q", cfg.Path)
	}

	loaded, err := LoadPopulation(dir)
	if err != nil {
		t.Fatalf("LoadPopulation: %v", err)
	}
	if len(loaded.BackgroundNPCs) != 1 || loaded.BackgroundNPCs[0].Name != "茶摊老板" {
		t.Fatalf("background npcs = %#v", loaded.BackgroundNPCs)
	}
	if len(loaded.PromotedNPCs) != 1 || loaded.PromotedNPCs[0].Status != "promoted" {
		t.Fatalf("promoted npcs = %#v", loaded.PromotedNPCs)
	}
	if len(loaded.IdentityCores) != 1 || loaded.IdentityCores[0].ID != "tea_vendor_core" {
		t.Fatalf("identity cores = %#v", loaded.IdentityCores)
	}
	if loaded.Policy.PromoteThreshold != 12 || loaded.Policy.MajorThreshold != 30 {
		t.Fatalf("policy = %#v", loaded.Policy)
	}
}

func TestLoadPopulationDefaultsWhenMissing(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "world.yml"), []byte("meta:\n  name: test\ncore_rules: seed\n"), 0644); err != nil {
		t.Fatalf("write world.yml: %v", err)
	}
	cfg, err := LoadPopulation(dir)
	if err != nil {
		t.Fatalf("LoadPopulation: %v", err)
	}
	if cfg.Policy.PromoteThreshold != 10 || cfg.Policy.MajorThreshold != 25 {
		t.Fatalf("defaults = %#v", cfg.Policy)
	}
	if cfg.BackgroundNPCs == nil || cfg.PromotedNPCs == nil || cfg.IdentityCores == nil {
		t.Fatalf("expected empty slices, got %#v", cfg)
	}
}

func TestEnsureSeededPopulationCreatesBackgroundNPCs(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "scenes"), 0755); err != nil {
		t.Fatalf("mkdir scenes: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "world"), 0755); err != nil {
		t.Fatalf("mkdir world: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "world.yml"), []byte("meta:\n  name: test\ncore_rules: seed\n"), 0644); err != nil {
		t.Fatalf("write world.yml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "scenes", "default.yml"), []byte("scene:\n  location: 镇口\n  time_of_day: 午后\n  weather: 晴\n  present_chars:\n    - 111\n    - 玩家\n"), 0644); err != nil {
		t.Fatalf("write scene: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "world", "factions.yml"), []byte("factions:\n  - id: guard\n    name: 巡城司\n"), 0644); err != nil {
		t.Fatalf("write factions: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "world", "locations.yml"), []byte("locations:\n  - id: east_gate\n    name: 东门\n"), 0644); err != nil {
		t.Fatalf("write locations: %v", err)
	}

	cfg, changed, err := EnsureSeededPopulation(dir)
	if err != nil {
		t.Fatalf("EnsureSeededPopulation: %v", err)
	}
	if !changed {
		t.Fatalf("changed = false, want true")
	}
	if len(cfg.BackgroundNPCs) < 3 {
		t.Fatalf("background npcs = %#v, want seeded population", cfg.BackgroundNPCs)
	}
	if cfg.BackgroundNPCs[0].Location == "" {
		t.Fatalf("seeded npc missing location: %#v", cfg.BackgroundNPCs[0])
	}

	loaded, err := LoadPopulation(dir)
	if err != nil {
		t.Fatalf("LoadPopulation: %v", err)
	}
	if len(loaded.BackgroundNPCs) != len(cfg.BackgroundNPCs) {
		t.Fatalf("loaded background npcs = %d, want %d", len(loaded.BackgroundNPCs), len(cfg.BackgroundNPCs))
	}
}
