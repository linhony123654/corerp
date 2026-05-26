package world

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"corerp/internal/core"
)

func ListCatalog(root string, loaded map[string]string) ([]core.WorldSummary, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		root = "worlds"
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return []core.WorldSummary{}, nil
		}
		return nil, err
	}

	loadedByPath := make(map[string]string, len(loaded))
	for character, path := range loaded {
		loadedByPath[cleanPath(path)] = character
	}

	var out []core.WorldSummary
	for _, entry := range entries {
		path := filepath.Join(root, entry.Name())
		if entry.IsDir() {
			summary, ok := summarizeWorld(path, entry.Name(), loadedByPath)
			if ok {
				out = append(out, summary)
			}
			continue
		}
		if strings.HasSuffix(entry.Name(), ".yml") || strings.HasSuffix(entry.Name(), ".yaml") {
			summary, ok := summarizeWorld(path, strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name())), loadedByPath)
			if ok {
				out = append(out, summary)
			}
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].LoadedCharacter != "" && out[j].LoadedCharacter == "" {
			return true
		}
		if out[i].LoadedCharacter == "" && out[j].LoadedCharacter != "" {
			return false
		}
		if out[i].Name == out[j].Name {
			return out[i].Path < out[j].Path
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

func summarizeWorld(path, fallbackID string, loadedByPath map[string]string) (core.WorldSummary, bool) {
	bundle, err := LoadBundle(path)
	if err != nil {
		return core.WorldSummary{}, false
	}
	name := strings.TrimSpace(bundle.Config.Name)
	if name == "" {
		name = fallbackID
	}
	return core.WorldSummary{
		ID:                 fallbackID,
		Name:               name,
		Path:               filepath.ToSlash(filepath.Clean(path)),
		Format:             bundle.Config.Format,
		SceneCount:         len(bundle.Scenes),
		CharacterCount:     len(bundle.Ontology.Characters),
		LocationCount:      len(bundle.Ontology.Locations),
		FactionCount:       len(bundle.Ontology.Factions),
		ItemCount:          len(bundle.Ontology.Items),
		EventCount:         len(bundle.Ontology.Events),
		TimelineCount:      len(bundle.Ontology.Timelines),
		BackgroundNPCCount: len(bundle.Population.BackgroundNPCs),
		PromotedNPCCount:   len(bundle.Population.PromotedNPCs),
		IdentityCoreCount:  len(bundle.Population.IdentityCores),
		LoadedCharacter:    loadedByPath[cleanPath(path)],
	}, true
}

func cleanPath(path string) string {
	return filepath.ToSlash(filepath.Clean(strings.TrimSpace(path)))
}
