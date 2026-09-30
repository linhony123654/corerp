package core

import (
	"encoding/json"
	"os"
	"testing"
)

func testStudioPackage(kind string) StudioPackageBundle {
	m := PackageManifest{SchemaVersion: StudioPackageManifestVersion, ID: "test." + kind, Kind: kind, Version: "1.0.0", EngineAPI: StudioPackageManifestVersion, Requires: []PackageDependency{}, Optional: []PackageDependency{}, SchemaHash: StudioPackageSchemaHash}
	c := StudioPackageContent{Version: StudioPackageContentVersion}
	if kind == "system" {
		c.SystemRules = &StudioSystemRules{NPCDailyActionBudget: 8}
		m.Capabilities = []string{"rules.npc.daily_budget"}
		m.ContentFiles = []string{"system.json"}
	} else {
		style := DefaultRPStyle()
		c.NarrativeStyle = &style
		m.Capabilities = []string{"narrative.style"}
		m.ContentFiles = []string{"narrative.json"}
	}
	m.ContentHash, _ = HashJSON(c)
	return StudioPackageBundle{Manifest: m, Content: c}
}

func TestStudioPackageManifestAndBoundedContent(t *testing.T) {
	raw, err := os.ReadFile("../../../docs/m0/examples/metro/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var original PackageManifest
	if err := json.Unmarshal(raw, &original); err != nil {
		t.Fatal(err)
	}
	if original.SchemaHash != StudioPackageSchemaHash || original.EngineAPI != StudioPackageManifestVersion || original.SchemaVersion != StudioPackageManifestVersion {
		t.Fatal("M0 contract drift")
	}
	for _, kind := range []string{"system", "narrative"} {
		if err := testStudioPackage(kind).Validate(); err != nil {
			t.Fatal(err)
		}
	}
	for name, mutate := range map[string]func(*StudioPackageBundle){
		"schema":         func(b *StudioPackageBundle) { b.Manifest.SchemaHash = "sha256:wrong" },
		"engine":         func(b *StudioPackageBundle) { b.Manifest.EngineAPI = "latest" },
		"version range":  func(b *StudioPackageBundle) { b.Manifest.Version = "1.x" },
		"unbounded id":   func(b *StudioPackageBundle) { b.Manifest.ID = "bad/id" },
		"missing arrays": func(b *StudioPackageBundle) { b.Manifest.Requires = nil },
		"self": func(b *StudioPackageBundle) {
			b.Manifest.Requires = []PackageDependency{{ID: b.Manifest.ID, Version: "1.0.0"}}
		},
		"duplicate deps": func(b *StudioPackageBundle) {
			b.Manifest.Requires = []PackageDependency{{ID: "other", Version: "1.0.0"}}
			b.Manifest.Optional = b.Manifest.Requires
		},
		"capability": func(b *StudioPackageBundle) { b.Manifest.Capabilities = []string{"db.write"} },
		"extra capability": func(b *StudioPackageBundle) {
			b.Manifest.Capabilities = append(b.Manifest.Capabilities, "arbitrary.js")
		},
		"path traversal": func(b *StudioPackageBundle) { b.Manifest.ContentFiles = []string{"../system.json"} },
		"absolute path":  func(b *StudioPackageBundle) { b.Manifest.ContentFiles = []string{"/system.json"} },
		"extra file":     func(b *StudioPackageBundle) { b.Manifest.ContentFiles = append(b.Manifest.ContentFiles, "script.js") },
		"hash":           func(b *StudioPackageBundle) { b.Content.SystemRules.NPCDailyActionBudget++ },
		"out of bound": func(b *StudioPackageBundle) {
			b.Content.SystemRules.NPCDailyActionBudget = 65
			b.Manifest.ContentHash, _ = HashJSON(b.Content)
		},
		"unknown execution mode": func(b *StudioPackageBundle) {
			b.Content.SystemRules.RPExecutionMode = "automatic"
			b.Content.SystemRules.MaxActiveResponders = 1
			b.Manifest.ContentHash, _ = HashJSON(b.Content)
		},
		"unbounded responders": func(b *StudioPackageBundle) {
			b.Content.SystemRules.RPExecutionMode = "orchestrated"
			b.Content.SystemRules.MaxActiveResponders = 9
			b.Manifest.ContentHash, _ = HashJSON(b.Content)
		},
		"implicit mode with limit": func(b *StudioPackageBundle) {
			b.Content.SystemRules.MaxActiveResponders = 1
			b.Manifest.ContentHash, _ = HashJSON(b.Content)
		},
		"wrong content": func(b *StudioPackageBundle) {
			b.Content.SystemRules = nil
			b.Manifest.ContentHash, _ = HashJSON(b.Content)
		},
		"code adapter": func(b *StudioPackageBundle) { b.Manifest.Kind = "adapter" },
	} {
		t.Run(name, func(t *testing.T) {
			b := testStudioPackage("system")
			mutate(&b)
			if err := b.Validate(); err == nil {
				t.Fatal("invalid package accepted")
			}
		})
	}
	b := testStudioPackage("narrative")
	b.Content.NarrativeStyle.NarrativePackRef = "untrusted/code@1"
	b.Manifest.ContentHash, _ = HashJSON(b.Content)
	if err := b.Validate(); err == nil {
		t.Fatal("unsupported narrative renderer accepted")
	}
}

func TestStudioPackageAcceptsBoundedRPExecutionModes(t *testing.T) {
	for _, mode := range []string{"deterministic", "orchestrated", "multi_agent"} {
		b := testStudioPackage("system")
		b.Content.SystemRules.RPExecutionMode = mode
		b.Content.SystemRules.MaxActiveResponders = 3
		if mode == "deterministic" {
			b.Content.SystemRules.MaxActiveResponders = 1
		}
		b.Manifest.ContentHash, _ = HashJSON(b.Content)
		if err := b.Validate(); err != nil {
			t.Fatalf("%s: %v", mode, err)
		}
	}
	deterministic := testStudioPackage("system")
	deterministic.Content.SystemRules.RPExecutionMode = "deterministic"
	deterministic.Content.SystemRules.MaxActiveResponders = 2
	deterministic.Manifest.ContentHash, _ = HashJSON(deterministic.Content)
	if err := deterministic.Validate(); err == nil {
		t.Fatal("deterministic mode accepted more than one responder")
	}
}

func TestStudioPackageBackgroundProgressionIsExplicitAndBounded(t *testing.T) {
	valid := testStudioPackage("system")
	valid.Content.SystemRules.BackgroundProgression = &StudioBackgroundProgression{Enabled: true, StepMinutes: 15, SchedulerBudget: 32}
	valid.Manifest.ContentHash, _ = HashJSON(valid.Content)
	if err := valid.Validate(); err != nil {
		t.Fatal("valid background progression", err)
	}
	for _, mutate := range []func(*StudioBackgroundProgression){
		func(p *StudioBackgroundProgression) { p.Enabled = false },
		func(p *StudioBackgroundProgression) { p.StepMinutes = 0 },
		func(p *StudioBackgroundProgression) { p.StepMinutes = 1441 },
		func(p *StudioBackgroundProgression) { p.SchedulerBudget = 0 },
		func(p *StudioBackgroundProgression) { p.SchedulerBudget = 10001 },
	} {
		bad := testStudioPackage("system")
		bad.Content.SystemRules.BackgroundProgression = &StudioBackgroundProgression{Enabled: true, StepMinutes: 15, SchedulerBudget: 32}
		mutate(bad.Content.SystemRules.BackgroundProgression)
		bad.Manifest.ContentHash, _ = HashJSON(bad.Content)
		if err := bad.Validate(); err == nil {
			t.Fatal("invalid background progression accepted")
		}
	}
	disabled := testStudioPackage("system")
	disabled.Content.SystemRules.BackgroundProgression = &StudioBackgroundProgression{Enabled: false}
	disabled.Manifest.ContentHash, _ = HashJSON(disabled.Content)
	if err := disabled.Validate(); err != nil {
		t.Fatal("explicit disabled progression", err)
	}
}

func TestStudioPackageSetExactDependenciesAndCycles(t *testing.T) {
	a, b := testStudioPackage("system"), testStudioPackage("narrative")
	b.Manifest.Requires = []PackageDependency{{ID: a.Manifest.ID, Version: a.Manifest.Version}}
	if err := ValidateStudioPackageSet([]StudioPackageBundle{b}); err == nil {
		t.Fatal("missing dependency")
	}
	if err := ValidateStudioPackageSet([]StudioPackageBundle{b, a}); err != nil {
		t.Fatal("order-independent exact set", err)
	}
	if err := ValidateStudioPackageSet([]StudioPackageBundle{a, a}); err == nil {
		t.Fatal("duplicate identity")
	}
	a.Manifest.Optional = []PackageDependency{{ID: b.Manifest.ID, Version: b.Manifest.Version}}
	if err := ValidateStudioPackageSet([]StudioPackageBundle{a}); err != nil {
		t.Fatal("absent optional", err)
	}
	if err := ValidateStudioPackageSet([]StudioPackageBundle{a, b}); err == nil {
		t.Fatal("optional/required cycle")
	}
	b.Manifest.Requires = []PackageDependency{}
	b.Manifest.Version = "2.0.0"
	if err := ValidateStudioPackageSet([]StudioPackageBundle{a, b}); err == nil {
		t.Fatal("late optional dependency conflict")
	}
}
