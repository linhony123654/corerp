package main

import (
	"bufio"
	"runtime/debug"
	"strings"
	"testing"

	"corerp/internal/core"
)

func TestSceneIsEmpty(t *testing.T) {
	tests := []struct {
		name  string
		scene core.SceneState
		want  bool
	}{
		{name: "zero value", scene: core.SceneState{}, want: true},
		{name: "with location", scene: core.SceneState{Location: "station"}, want: false},
		{name: "with characters", scene: core.SceneState{Characters: []string{"Anya"}}, want: false},
	}

	for _, tc := range tests {
		if got := sceneIsEmpty(tc.scene); got != tc.want {
			t.Fatalf("%s: sceneIsEmpty() = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestNormalizeServeBootMode(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "", want: "auto"},
		{in: "AUTO", want: "auto"},
		{in: "character", want: "character"},
		{in: "world", want: "world"},
		{in: "bad", want: ""},
	}
	for _, tc := range tests {
		if got := normalizeServeBootMode(tc.in); got != tc.want {
			t.Fatalf("normalizeServeBootMode(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestResolveServeBootMode(t *testing.T) {
	tests := []struct {
		name          string
		mode          string
		worldPath     string
		hasCharacters bool
		want          string
	}{
		{name: "explicit world", mode: "world", worldPath: "worlds/neon_block", hasCharacters: true, want: "world"},
		{name: "explicit character", mode: "character", worldPath: "worlds/neon_block", hasCharacters: false, want: "character"},
		{name: "auto prefers character when cards exist", mode: "auto", worldPath: "worlds/neon_block", hasCharacters: true, want: "character"},
		{name: "auto falls back to world when no cards", mode: "auto", worldPath: "worlds/neon_block", hasCharacters: false, want: "world"},
		{name: "invalid mode stays invalid", mode: "bad", worldPath: "worlds/neon_block", hasCharacters: true, want: ""},
	}
	for _, tc := range tests {
		if got := resolveServeBootMode(tc.mode, tc.worldPath, tc.hasCharacters); got != tc.want {
			t.Fatalf("%s: resolveServeBootMode(%q, %q, %v) = %q, want %q", tc.name, tc.mode, tc.worldPath, tc.hasCharacters, got, tc.want)
		}
	}
}

func TestChooseImportModeAcceptsOverride(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader("ensemble\n"))
	if got := chooseImportModeFromReader(reader, "auto"); got != "ensemble" {
		t.Fatalf("chooseImportModeFromReader = %q, want ensemble", got)
	}
}

func TestChooseImportModeDefaultsToCurrent(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader("\n"))
	if got := chooseImportModeFromReader(reader, "auto"); got != "auto" {
		t.Fatalf("chooseImportModeFromReader = %q, want auto", got)
	}
}

func TestResolveBuildMetadataPrefersInjectedValues(t *testing.T) {
	oldVersion, oldCommit, oldTime := buildVersion, buildCommit, buildTime
	buildVersion = "1.2.3"
	buildCommit = "abc123"
	buildTime = "2026-05-26T09:30:00Z"
	t.Cleanup(func() {
		buildVersion, buildCommit, buildTime = oldVersion, oldCommit, oldTime
	})

	meta := resolveBuildMetadata()
	if meta.Version != "1.2.3" {
		t.Fatalf("version = %q, want 1.2.3", meta.Version)
	}
	if meta.Commit != "abc123" {
		t.Fatalf("commit = %q, want abc123", meta.Commit)
	}
	if meta.Time != "2026-05-26T09:30:00Z" {
		t.Fatalf("time = %q, want injected time", meta.Time)
	}
}

func TestResolveBuildMetadataFallsBackToDefaults(t *testing.T) {
	oldVersion, oldCommit, oldTime := buildVersion, buildCommit, buildTime
	buildVersion, buildCommit, buildTime = "", "", ""
	t.Cleanup(func() {
		buildVersion, buildCommit, buildTime = oldVersion, oldCommit, oldTime
	})

	meta := resolveBuildMetadata()
	if meta.Version == "" {
		t.Fatalf("version should not be empty")
	}
	if meta.Commit == "" {
		t.Fatalf("commit should not be empty")
	}
	if meta.Time == "" {
		t.Fatalf("time should not be empty")
	}

	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" && setting.Value != "" && meta.Commit != "unknown" && meta.Commit != setting.Value {
				t.Fatalf("commit = %q, want unknown or vcs revision %q", meta.Commit, setting.Value)
			}
		}
	}
}
