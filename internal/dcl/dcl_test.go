package dcl

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"corerp/internal/world"
)

func TestLoadAndInstallDeclarativeDCL(t *testing.T) {
	root := t.TempDir()
	modDir := filepath.Join(root, "looping_isekai_return.dcl")
	writeFile(t, filepath.Join(modDir, "manifest.yml"), `
id: looping_isekai_return
name: Looping Isekai Return
version: 0.1.0
entry_world: looping_isekai_return
description: Return-by-death inspired loop scenario.
`)
	writeFile(t, filepath.Join(modDir, "patches", "world.yml"), `
core_rules: |
  The world advances around a checkpoint loop.
seed:
  premise: "A stranded outsider repeats the same mansion crisis after death."
factions:
  - id: mansion_household
    name: Mansion Household
    role: sanctuary
locations:
  - id: mansion_hall
    name: Mansion Hall
    kind: mansion
    controller: mansion_household
pressures:
  - id: witch_scent
    name: Witch Scent
    kind: suspicion
    intensity: 0.35
    target: mansion_household
`)
	writeFile(t, filepath.Join(modDir, "patches", "population.yml"), `
background_npcs:
  - id: silver_heir
    name: Silver Heir
    role: protected_candidate
    location: Mansion Hall
    faction: mansion_household
    traits: [kind, guarded]
    hooks: [contract, suspicion]
policy:
  promote_threshold: 6.5
  major_threshold: 10
  interaction_weight: 3
  mention_weight: 1
  event_weight: 2
  relationship_weight: 3
  scene_weight: 2
`)
	writeFile(t, filepath.Join(modDir, "patches", "scenes.yml"), `
scenes:
  - name: default
    scene:
      location: Mansion Hall
      time_of_day: midnight
      weather: rain
      characters: [Outsider, Player]
      description: The first safe room before the loop breaks.
`)
	writeFile(t, filepath.Join(modDir, "logic", "hooks.yml"), `
hooks:
  - id: return_by_death
    trigger: focus_death
    effect:
      restore_checkpoint: last_safe_checkpoint
      add_pressure:
        id: witch_scent
        intensity: 0.15
`)

	mod, err := Load(modDir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if mod.Manifest.ID != "looping_isekai_return" || len(mod.Hooks) != 1 || len(mod.Population.BackgroundNPCs) != 1 {
		t.Fatalf("mod = %#v, want manifest, hook, and population patch", mod)
	}

	worldsRoot := filepath.Join(root, "worlds")
	result, err := Install(root, "looping_isekai_return", worldsRoot, InstallOptions{})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if result.HookCount != 1 {
		t.Fatalf("HookCount = %d, want 1", result.HookCount)
	}
	structure, err := world.LoadStructure(result.WorldPath)
	if err != nil {
		t.Fatalf("LoadStructure: %v", err)
	}
	if len(structure.Pressures) != 1 || structure.Pressures[0].ID != "witch_scent" {
		t.Fatalf("pressures = %#v, want witch_scent", structure.Pressures)
	}
	population, err := world.LoadPopulation(result.WorldPath)
	if err != nil {
		t.Fatalf("LoadPopulation: %v", err)
	}
	if len(population.BackgroundNPCs) != 1 || population.BackgroundNPCs[0].Name != "Silver Heir" {
		t.Fatalf("background npcs = %#v, want Silver Heir", population.BackgroundNPCs)
	}
	if _, err := os.Stat(filepath.Join(result.WorldPath, "mods", "looping_isekai_return", "hooks.yml")); err != nil {
		t.Fatalf("installed hooks missing: %v", err)
	}
	reg, err := LoadRegistry(root)
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	if len(reg.Mods) != 1 || reg.Mods[0].ID != "looping_isekai_return" {
		t.Fatalf("registry = %#v, want installed mod", reg)
	}
}

func TestListShowsInstalledDCL(t *testing.T) {
	root := t.TempDir()
	modDir := filepath.Join(root, "sample.dcl")
	writeFile(t, filepath.Join(modDir, "manifest.yml"), `
id: sample
name: Sample
version: 0.1.0
entry_world: sample_world
`)
	if _, err := Install(root, "sample", filepath.Join(root, "worlds"), InstallOptions{}); err != nil {
		t.Fatalf("Install: %v", err)
	}
	mods, err := List(root)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(mods) != 1 || !mods[0].Installed || mods[0].WorldPath == "" {
		t.Fatalf("mods = %#v, want installed sample summary", mods)
	}
}

func TestDeletePackageRemovesDCLDirectory(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "sample.dcl", "manifest.yml"), `
id: sample
name: Sample
`)
	deleted, err := DeletePackage(root, "sample")
	if err != nil {
		t.Fatalf("DeletePackage: %v", err)
	}
	if deleted == "" {
		t.Fatalf("deleted path is empty")
	}
	if _, err := os.Stat(filepath.Join(root, "sample.dcl")); !os.IsNotExist(err) {
		t.Fatalf("sample.dcl still exists or stat failed unexpectedly: %v", err)
	}
}

func TestUploadZipRejectsScriptsAndInstallsSingleDCLRoot(t *testing.T) {
	root := t.TempDir()
	badZip := filepath.Join(root, "bad.zip")
	createZip(t, badZip, map[string]string{
		"bad.dcl/manifest.yml":          "id: bad\nname: Bad\n",
		"bad.dcl/logic/scripts/run.lua": "print('no')",
	})
	if _, err := UploadZip(root, badZip, false); err == nil {
		t.Fatalf("UploadZip accepted script file, want rejection")
	}

	goodZip := filepath.Join(root, "good.zip")
	createZip(t, goodZip, map[string]string{
		"good.dcl/manifest.yml":      "id: good\nname: Good\nversion: 0.1.0\n",
		"good.dcl/patches/world.yml": "core_rules: good\n",
	})
	result, err := UploadZip(root, goodZip, false)
	if err != nil {
		t.Fatalf("UploadZip good: %v", err)
	}
	if result.ID != "good" || result.FileCount != 2 {
		t.Fatalf("result = %#v, want good with two files", result)
	}
}

func TestUploadZipOverwriteRejectPreservesExistingPackage(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "good.dcl", "manifest.yml"), "id: good\nname: Existing\n")
	badZip := filepath.Join(root, "bad-overwrite.zip")
	createZip(t, badZip, map[string]string{
		"good.dcl/manifest.yml":          "id: good\nname: Bad\n",
		"good.dcl/logic/scripts/run.lua": "print('no')",
	})
	if _, err := UploadZip(root, badZip, true); err == nil {
		t.Fatalf("UploadZip accepted script file with overwrite, want rejection")
	}
	data, err := os.ReadFile(filepath.Join(root, "good.dcl", "manifest.yml"))
	if err != nil {
		t.Fatalf("existing manifest missing after rejected overwrite: %v", err)
	}
	if !strings.Contains(string(data), "Existing") {
		t.Fatalf("existing package was overwritten after rejected upload: %s", string(data))
	}

	invalidZip := filepath.Join(root, "invalid-overwrite.zip")
	createZip(t, invalidZip, map[string]string{
		"good.dcl/manifest.yml": "name: Missing ID\n",
	})
	if _, err := UploadZip(root, invalidZip, true); err == nil {
		t.Fatalf("UploadZip accepted invalid manifest with overwrite, want rejection")
	}
	data, err = os.ReadFile(filepath.Join(root, "good.dcl", "manifest.yml"))
	if err != nil {
		t.Fatalf("existing manifest missing after rejected invalid manifest overwrite: %v", err)
	}
	if !strings.Contains(string(data), "Existing") {
		t.Fatalf("existing package was overwritten after invalid upload: %s", string(data))
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func createZip(t *testing.T, path string, files map[string]string) {
	t.Helper()
	out, err := os.Create(path)
	if err != nil {
		t.Fatalf("Create zip: %v", err)
	}
	zw := zip.NewWriter(out)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("zip Create: %v", err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("zip Write: %v", err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip Close: %v", err)
	}
	if err := out.Close(); err != nil {
		t.Fatalf("zip file Close: %v", err)
	}
}
