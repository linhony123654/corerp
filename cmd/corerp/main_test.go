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
