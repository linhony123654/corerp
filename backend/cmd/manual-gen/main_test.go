package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManualGenerationOutputAndIntegrity(t *testing.T) {
	tempDist := t.TempDir()
	manualSourceDir := filepath.Join("..", "..", "..", "docs", "manual")

	// Ensure source files exist
	expectedFiles := []string{"index.md", "player.md", "creator.md", "external-agent.md", "maintainer.md"}
	for _, f := range expectedFiles {
		p := filepath.Join(manualSourceDir, f)
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("missing required manual source file: %s (%v)", p, err)
		}
	}

	docFiles := []struct {
		id       string
		title    string
		filename string
	}{
		{"index", "Overview & Quickstart", "index.md"},
		{"player", "Player Guide", "player.md"},
		{"creator", "Creator Guide", "creator.md"},
		{"external-agent", "External Agent & MCP", "external-agent.md"},
		{"maintainer", "Maintainer Architecture & Ops", "maintainer.md"},
	}

	var sections []DocumentSection
	for _, df := range docFiles {
		path := filepath.Join(manualSourceDir, df.filename)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("failed to read %s: %v", path, err)
		}
		_, headings := parseMarkdownHeadings(string(data))
		sections = append(sections, DocumentSection{
			ID:       df.id,
			Title:    df.title,
			Filename: df.filename,
			Content:  string(data),
			Headings: headings,
		})
	}

	htmlContent := buildHTMLPage(sections)
	if len(htmlContent) == 0 {
		t.Fatal("empty HTML output generated")
	}

	// 1. Check for required features
	requiredSnippets := []string{
		"<!DOCTYPE html>",
		"id=\"sidebar\"",
		"id=\"search-input\"",
		"class=\"code-container\"",
		"class=\"copy-btn\"",
		"onclick=\"copyCode(this)\"",
		"class=\"anchor-link\"",
		"aria-label=\"Search documentation\"",
		"toggleSidebar()",
		"filterNav",
	}
	for _, snippet := range requiredSnippets {
		if !strings.Contains(htmlContent, snippet) {
			t.Fatalf("HTML output missing required snippet: %s", snippet)
		}
	}

	// 2. Check that no external fonts or external CDN resources are used
	disallowedSnippets := []string{
		"fonts.googleapis.com",
		"fonts.gstatic.com",
		"cdn.jsdelivr.net",
		"cdnjs.cloudflare.com",
		"unpkg.com",
	}
	for _, dis := range disallowedSnippets {
		if strings.Contains(htmlContent, dis) {
			t.Fatalf("HTML output violates offline requirement by referencing external CDN: %s", dis)
		}
	}

	// 3. Write and verify file on disk
	outPath := filepath.Join(tempDist, "index.html")
	if err := os.WriteFile(outPath, []byte(htmlContent), 0644); err != nil {
		t.Fatalf("failed to write test HTML: %v", err)
	}

	stat, err := os.Stat(outPath)
	if err != nil || stat.Size() < 50000 {
		t.Fatalf("generated file unexpectedly small: %d bytes", stat.Size())
	}
}

func TestSlugifyAndMarkdownParsing(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"Hello World", "hello-world"},
		{"1. Installation & Starting", "1-installation-starting"},
		{"Turn / Speech / Knowledge", "turn-speech-knowledge"},
		{"   Multiple   Spaces   ", "multiple-spaces"},
		{"Special!@#Characters$%", "special-characters"},
	}

	for _, c := range cases {
		actual := slugify(c.input)
		if actual != c.expected {
			t.Fatalf("slugify(%q) = %q, want %q", c.input, actual, c.expected)
		}
	}
}
