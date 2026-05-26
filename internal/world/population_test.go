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
