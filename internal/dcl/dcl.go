package dcl

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"corerp/internal/core"
	"corerp/internal/world"

	"gopkg.in/yaml.v3"
)

type Manifest struct {
	ID          string   `json:"id" yaml:"id"`
	Name        string   `json:"name" yaml:"name"`
	Version     string   `json:"version" yaml:"version"`
	TargetCore  string   `json:"target_core" yaml:"target_core"`
	Description string   `json:"description" yaml:"description"`
	EntryWorld  string   `json:"entry_world" yaml:"entry_world"`
	Author      string   `json:"author,omitempty" yaml:"author,omitempty"`
	Tags        []string `json:"tags,omitempty" yaml:"tags,omitempty"`
}

type Hook struct {
	ID          string                 `json:"id" yaml:"id"`
	Trigger     string                 `json:"trigger" yaml:"trigger"`
	Description string                 `json:"description,omitempty" yaml:"description,omitempty"`
	Effect      map[string]interface{} `json:"effect,omitempty" yaml:"effect,omitempty"`
}

type Rule struct {
	ID          string                   `json:"id" yaml:"id"`
	Description string                   `json:"description,omitempty" yaml:"description,omitempty"`
	When        map[string]interface{}   `json:"when" yaml:"when"`
	Do          []map[string]interface{} `json:"do" yaml:"do"`
}

type Mod struct {
	Path       string                `json:"path"`
	Manifest   Manifest              `json:"manifest"`
	World      WorldPatch            `json:"world"`
	Population core.PopulationConfig `json:"population"`
	Scenes     []core.SceneConfig    `json:"scenes"`
	Presets    []core.ScenarioPreset `json:"presets"`
	Hooks      []Hook                `json:"hooks"`
	Rules      []Rule                `json:"rules"`
}

type WorldPatch struct {
	CoreRules string                     `json:"core_rules" yaml:"core_rules"`
	Seed      core.WorldSeedConfig       `json:"seed" yaml:"seed"`
	Rules     []core.WorldRule           `json:"rules" yaml:"rules"`
	Factions  []core.WorldFactionConfig  `json:"factions" yaml:"factions"`
	Locations []core.WorldLocationConfig `json:"locations" yaml:"locations"`
	Pressures []core.WorldPressureConfig `json:"pressures" yaml:"pressures"`
}

type Summary struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Description string   `json:"description,omitempty"`
	EntryWorld  string   `json:"entry_world"`
	Path        string   `json:"path"`
	Tags        []string `json:"tags,omitempty"`
	Installed   bool     `json:"installed"`
	WorldPath   string   `json:"world_path,omitempty"`
}

type InstallOptions struct {
	Overwrite bool
}

type InstallResult struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Version     string    `json:"version"`
	WorldPath   string    `json:"world_path"`
	InstalledAt time.Time `json:"installed_at"`
	HookCount   int       `json:"hook_count"`
}

type Registry struct {
	Mods []RegistryEntry `json:"mods"`
}

type RegistryEntry struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Version     string    `json:"version"`
	SourcePath  string    `json:"source_path"`
	WorldPath   string    `json:"world_path"`
	InstalledAt time.Time `json:"installed_at"`
}

type UploadResult struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Version   string `json:"version"`
	Path      string `json:"path"`
	Replaced  bool   `json:"replaced"`
	FileCount int    `json:"file_count"`
}

type hooksDoc struct {
	Hooks []Hook `yaml:"hooks"`
	Rules []Rule `yaml:"rules"`
}

type scenesDoc struct {
	Scenes []scenePatch `yaml:"scenes"`
}

type scenePatch struct {
	Name  string          `yaml:"name"`
	Scene core.SceneState `yaml:"scene"`
}

type presetsDoc struct {
	Presets []core.ScenarioPreset `yaml:"presets"`
}

type populationPatchDoc struct {
	BackgroundNPCs []core.BackgroundNPC      `yaml:"background_npcs"`
	PromotedNPCs   []core.PromotedNPC        `yaml:"promoted_npcs"`
	IdentityCores  []core.IdentityCoreConfig `yaml:"identity_cores"`
	Policy         core.PromotionPolicy      `yaml:"policy"`
}

func List(root string) ([]Summary, error) {
	reg, _ := LoadRegistry(root)
	installed := make(map[string]RegistryEntry, len(reg.Mods))
	for _, entry := range reg.Mods {
		installed[entry.ID] = entry
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return []Summary{}, nil
		}
		return nil, err
	}
	out := make([]Summary, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasSuffix(entry.Name(), ".dcl") {
			continue
		}
		mod, err := Load(filepath.Join(root, entry.Name()))
		if err != nil {
			return nil, err
		}
		summary := Summary{
			ID:          mod.Manifest.ID,
			Name:        mod.Manifest.Name,
			Version:     mod.Manifest.Version,
			Description: mod.Manifest.Description,
			EntryWorld:  mod.Manifest.EntryWorld,
			Path:        filepath.ToSlash(filepath.Join(root, entry.Name())),
			Tags:        append([]string(nil), mod.Manifest.Tags...),
		}
		if regEntry, ok := installed[mod.Manifest.ID]; ok {
			summary.Installed = true
			summary.WorldPath = regEntry.WorldPath
		}
		out = append(out, summary)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func Load(path string) (Mod, error) {
	manifest, err := loadManifest(path)
	if err != nil {
		return Mod{}, err
	}
	worldPatch, err := loadWorldPatch(path)
	if err != nil {
		return Mod{}, err
	}
	population, err := loadPopulationPatch(path)
	if err != nil {
		return Mod{}, err
	}
	scenes, err := loadScenes(path)
	if err != nil {
		return Mod{}, err
	}
	presets, err := loadPresets(path)
	if err != nil {
		return Mod{}, err
	}
	hooks, err := loadHooks(path)
	if err != nil {
		return Mod{}, err
	}
	rules, err := loadRules(path)
	if err != nil {
		return Mod{}, err
	}
	return Mod{
		Path:       filepath.ToSlash(filepath.Clean(path)),
		Manifest:   manifest,
		World:      worldPatch,
		Population: population,
		Scenes:     scenes,
		Presets:    presets,
		Hooks:      hooks,
		Rules:      rules,
	}, nil
}

func Install(root, id, worldsRoot string, opts InstallOptions) (InstallResult, error) {
	modPath := filepath.Join(root, id)
	if !strings.HasSuffix(modPath, ".dcl") {
		modPath += ".dcl"
	}
	mod, err := Load(modPath)
	if err != nil {
		return InstallResult{}, err
	}
	worldID := strings.TrimSpace(mod.Manifest.EntryWorld)
	if worldID == "" {
		worldID = mod.Manifest.ID
	}
	target := filepath.Join(worldsRoot, safeID(worldID))
	if _, err := os.Stat(target); err == nil {
		if !opts.Overwrite {
			return InstallResult{}, fmt.Errorf("world already exists: %s", target)
		}
		if err := os.RemoveAll(target); err != nil {
			return InstallResult{}, err
		}
	} else if !os.IsNotExist(err) {
		return InstallResult{}, err
	}

	createdPath, err := world.CreateWorld(worldsRoot, worldID, mod.World.CoreRules)
	if err != nil {
		return InstallResult{}, err
	}
	target = createdPath
	structure := core.WorldStructureConfig{
		Ruleset:   core.WorldRulesetConfig{Rules: mod.World.Rules},
		Seed:      mod.World.Seed,
		Factions:  mod.World.Factions,
		Locations: mod.World.Locations,
		Pressures: mod.World.Pressures,
	}
	if _, err := world.SaveStructure(target, structure); err != nil {
		return InstallResult{}, err
	}
	if _, err := world.SavePopulation(target, mod.Population); err != nil {
		return InstallResult{}, err
	}
	for _, scene := range mod.Scenes {
		if _, err := world.SaveScene(target, scene); err != nil {
			return InstallResult{}, err
		}
	}
	if err := writePresets(target, mod.Presets); err != nil {
		return InstallResult{}, err
	}
	if err := writeInstalledHooks(target, mod.Manifest.ID, mod.Hooks, mod.Rules); err != nil {
		return InstallResult{}, err
	}

	result := InstallResult{
		ID:          mod.Manifest.ID,
		Name:        mod.Manifest.Name,
		Version:     mod.Manifest.Version,
		WorldPath:   filepath.ToSlash(filepath.Clean(target)),
		InstalledAt: time.Now().UTC(),
		HookCount:   len(mod.Hooks) + len(mod.Rules),
	}
	if err := upsertRegistry(root, RegistryEntry{
		ID:          result.ID,
		Name:        result.Name,
		Version:     result.Version,
		SourcePath:  filepath.ToSlash(filepath.Clean(modPath)),
		WorldPath:   result.WorldPath,
		InstalledAt: result.InstalledAt,
	}); err != nil {
		return InstallResult{}, err
	}
	return result, nil
}

func Remove(root, id string, deleteWorld bool) (RegistryEntry, error) {
	reg, err := LoadRegistry(root)
	if err != nil {
		return RegistryEntry{}, err
	}
	id = strings.TrimSpace(id)
	var removed RegistryEntry
	next := reg.Mods[:0]
	for _, entry := range reg.Mods {
		if entry.ID == id {
			removed = entry
			continue
		}
		next = append(next, entry)
	}
	if removed.ID == "" {
		return RegistryEntry{}, fmt.Errorf("dcl mod is not installed: %s", id)
	}
	reg.Mods = next
	if err := SaveRegistry(root, reg); err != nil {
		return RegistryEntry{}, err
	}
	if deleteWorld && strings.TrimSpace(removed.WorldPath) != "" {
		if err := os.RemoveAll(removed.WorldPath); err != nil {
			return RegistryEntry{}, err
		}
	}
	return removed, nil
}

func DeletePackage(root, id string) (string, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return "", fmt.Errorf("dcl id is required")
	}
	modPath := filepath.Join(root, id)
	if !strings.HasSuffix(modPath, ".dcl") {
		modPath += ".dcl"
	}
	rootClean := filepath.Clean(root)
	modClean := filepath.Clean(modPath)
	if modClean == rootClean || !strings.HasPrefix(modClean, rootClean+string(filepath.Separator)) {
		return "", fmt.Errorf("unsafe dcl path: %s", id)
	}
	if _, err := os.Stat(modClean); err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("dcl package not found: %s", id)
		}
		return "", err
	}
	if err := os.RemoveAll(modClean); err != nil {
		return "", err
	}
	return filepath.ToSlash(modClean), nil
}

func UploadZip(root, zipPath string, overwrite bool) (UploadResult, error) {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return UploadResult{}, err
	}
	defer reader.Close()

	top := ""
	for _, file := range reader.File {
		name := filepath.ToSlash(file.Name)
		if strings.TrimSpace(name) == "" || strings.HasPrefix(name, "/") || strings.Contains(name, "../") {
			return UploadResult{}, fmt.Errorf("unsafe zip path: %s", file.Name)
		}
		parts := strings.Split(strings.Trim(name, "/"), "/")
		if len(parts) == 0 || parts[0] == "" {
			continue
		}
		if top == "" {
			top = parts[0]
		} else if top != parts[0] {
			return UploadResult{}, fmt.Errorf("zip must contain a single .dcl root directory")
		}
		if !file.FileInfo().IsDir() && strings.Trim(name, "/") != top && !allowedUploadFile(name) {
			return UploadResult{}, fmt.Errorf("unsupported file in dcl zip: %s", file.Name)
		}
	}
	if !strings.HasSuffix(top, ".dcl") {
		return UploadResult{}, fmt.Errorf("zip root must end with .dcl")
	}

	if err := os.MkdirAll(root, 0755); err != nil {
		return UploadResult{}, err
	}
	target := filepath.Join(root, top)
	tempRoot, err := os.MkdirTemp(root, ".dcl-upload-*")
	if err != nil {
		return UploadResult{}, err
	}
	defer os.RemoveAll(tempRoot)
	tempTarget := filepath.Join(tempRoot, top)

	replaced := false
	if _, err := os.Stat(target); err == nil {
		if !overwrite {
			return UploadResult{}, fmt.Errorf("dcl already exists: %s", top)
		}
		replaced = true
	} else if !os.IsNotExist(err) {
		return UploadResult{}, err
	}

	count := 0
	for _, file := range reader.File {
		name := filepath.ToSlash(file.Name)
		if strings.Trim(name, "/") == top {
			continue
		}
		dest := filepath.Join(tempRoot, filepath.FromSlash(name))
		if !strings.HasPrefix(filepath.Clean(dest), filepath.Clean(tempTarget)+string(filepath.Separator)) {
			return UploadResult{}, fmt.Errorf("unsafe zip path: %s", file.Name)
		}
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(dest, 0755); err != nil {
				return UploadResult{}, err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
			return UploadResult{}, err
		}
		src, err := file.Open()
		if err != nil {
			return UploadResult{}, err
		}
		if err := writeZipFile(dest, src); err != nil {
			src.Close()
			return UploadResult{}, err
		}
		src.Close()
		count++
	}

	mod, err := Load(tempTarget)
	if err != nil {
		return UploadResult{}, err
	}
	if replaced {
		if err := os.RemoveAll(target); err != nil {
			return UploadResult{}, err
		}
	}
	if err := os.Rename(tempTarget, target); err != nil {
		return UploadResult{}, err
	}
	return UploadResult{
		ID:        mod.Manifest.ID,
		Name:      mod.Manifest.Name,
		Version:   mod.Manifest.Version,
		Path:      filepath.ToSlash(filepath.Clean(target)),
		Replaced:  replaced,
		FileCount: count,
	}, nil
}

func LoadRegistry(root string) (Registry, error) {
	data, err := os.ReadFile(filepath.Join(root, "_registry.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return Registry{Mods: []RegistryEntry{}}, nil
		}
		return Registry{}, err
	}
	var reg Registry
	if err := json.Unmarshal(data, &reg); err != nil {
		return Registry{}, err
	}
	if reg.Mods == nil {
		reg.Mods = []RegistryEntry{}
	}
	return reg, nil
}

func SaveRegistry(root string, reg Registry) error {
	if err := os.MkdirAll(root, 0755); err != nil {
		return err
	}
	sort.Slice(reg.Mods, func(i, j int) bool { return reg.Mods[i].ID < reg.Mods[j].ID })
	data, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(filepath.Join(root, "_registry.json"), data, 0644)
}

func loadManifest(path string) (Manifest, error) {
	var manifest Manifest
	if err := readYAML(filepath.Join(path, "manifest.yml"), &manifest); err != nil {
		return Manifest{}, err
	}
	manifest.ID = strings.TrimSpace(manifest.ID)
	manifest.Name = strings.TrimSpace(manifest.Name)
	manifest.Version = strings.TrimSpace(manifest.Version)
	manifest.EntryWorld = strings.TrimSpace(manifest.EntryWorld)
	if manifest.ID == "" {
		return Manifest{}, fmt.Errorf("manifest id is required: %s", path)
	}
	if manifest.Name == "" {
		manifest.Name = manifest.ID
	}
	if manifest.Version == "" {
		manifest.Version = "0.1.0"
	}
	if manifest.EntryWorld == "" {
		manifest.EntryWorld = manifest.ID
	}
	return manifest, nil
}

func loadWorldPatch(path string) (WorldPatch, error) {
	var patch WorldPatch
	for _, file := range []string{
		filepath.Join(path, "patches", "world.yml"),
		filepath.Join(path, "data", "patches", "world", "structure.yml"),
		filepath.Join(path, "data", "patches", "world", "pressures.yml"),
	} {
		if err := readOptionalYAML(file, &patch); err != nil {
			return WorldPatch{}, err
		}
	}
	return patch, nil
}

func loadPopulationPatch(path string) (core.PopulationConfig, error) {
	var raw populationPatchDoc
	for _, file := range []string{
		filepath.Join(path, "patches", "population.yml"),
		filepath.Join(path, "data", "patches", "population", "seed.yml"),
	} {
		if err := readOptionalYAML(file, &raw); err != nil {
			return core.PopulationConfig{}, err
		}
	}
	return core.PopulationConfig{
		BackgroundNPCs: raw.BackgroundNPCs,
		PromotedNPCs:   raw.PromotedNPCs,
		IdentityCores:  raw.IdentityCores,
		Policy:         raw.Policy,
	}, nil
}

func loadScenes(path string) ([]core.SceneConfig, error) {
	var raw scenesDoc
	if err := readOptionalYAML(filepath.Join(path, "patches", "scenes.yml"), &raw); err != nil {
		return nil, err
	}
	out := make([]core.SceneConfig, 0, len(raw.Scenes))
	for _, scene := range raw.Scenes {
		name := strings.TrimSpace(scene.Name)
		if name == "" {
			name = "default"
		}
		out = append(out, core.SceneConfig{Name: name, Scene: scene.Scene})
	}
	return out, nil
}

func loadPresets(path string) ([]core.ScenarioPreset, error) {
	var raw presetsDoc
	if err := readOptionalYAML(filepath.Join(path, "patches", "presets.yml"), &raw); err != nil {
		return nil, err
	}
	return raw.Presets, nil
}

func loadHooks(path string) ([]Hook, error) {
	var raw hooksDoc
	for _, file := range []string{
		filepath.Join(path, "hooks.yml"),
		filepath.Join(path, "logic", "hooks.yml"),
	} {
		if err := readOptionalYAML(file, &raw); err != nil {
			return nil, err
		}
	}
	return raw.Hooks, nil
}

func loadRules(path string) ([]Rule, error) {
	var raw hooksDoc
	for _, file := range []string{
		filepath.Join(path, "hooks.yml"),
		filepath.Join(path, "logic", "hooks.yml"),
	} {
		if err := readOptionalYAML(file, &raw); err != nil {
			return nil, err
		}
	}
	return raw.Rules, nil
}

func writePresets(worldPath string, presets []core.ScenarioPreset) error {
	if len(presets) == 0 {
		return nil
	}
	worldDir := filepath.Join(worldPath, "world")
	if err := os.MkdirAll(worldDir, 0755); err != nil {
		return err
	}
	data, err := yaml.Marshal(presetsDoc{Presets: presets})
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(worldDir, "presets.yml"), data, 0644)
}

func writeInstalledHooks(worldPath, modID string, hooks []Hook, rules []Rule) error {
	if len(hooks) == 0 && len(rules) == 0 {
		return nil
	}
	dir := filepath.Join(worldPath, "mods", safeID(modID))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := yaml.Marshal(hooksDoc{Hooks: hooks, Rules: rules})
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "hooks.yml"), data, 0644)
}

func upsertRegistry(root string, entry RegistryEntry) error {
	reg, err := LoadRegistry(root)
	if err != nil {
		return err
	}
	replaced := false
	for i := range reg.Mods {
		if reg.Mods[i].ID == entry.ID {
			reg.Mods[i] = entry
			replaced = true
			break
		}
	}
	if !replaced {
		reg.Mods = append(reg.Mods, entry)
	}
	return SaveRegistry(root, reg)
}

func readYAML(path string, out interface{}) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(data, out)
}

func readOptionalYAML(path string, out interface{}) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil
	}
	return yaml.Unmarshal(data, out)
}

func writeZipFile(path string, src io.Reader) error {
	dst, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer dst.Close()
	_, err = io.Copy(dst, src)
	return err
}

func allowedUploadFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".yml", ".yaml", ".md", ".txt", ".json":
		return true
	default:
		return false
	}
}

func safeID(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	var b strings.Builder
	for _, r := range raw {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_' || r == ' ':
			b.WriteRune('_')
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "mod_world"
	}
	return out
}
