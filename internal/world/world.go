package world

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"corerp/internal/core"

	"gopkg.in/yaml.v3"
)

type Bundle struct {
	Config      core.WorldConfig
	Scenes      []core.SceneConfig
	Selected    string
	Ontology    Ontology
	DirectFacts []core.FactFrame
	Population  core.PopulationConfig
}

type Ontology struct {
	Characters []OntologyEntry `yaml:"characters"`
	Locations  []OntologyEntry `yaml:"locations"`
	Factions   []OntologyEntry `yaml:"factions"`
	Items      []OntologyEntry `yaml:"items"`
	Lore       []OntologyEntry `yaml:"lore"`
	Events     []OntologyEvent `yaml:"events"`
	Timelines  []OntologyEntry `yaml:"timelines"`
	Settings   []OntologyEntry `yaml:"settings"`
}

type OntologyEntry struct {
	Name    string `yaml:"name"`
	Keys    string `yaml:"keys"`
	Content string `yaml:"content"`
}

type OntologyEvent struct {
	Name    string `yaml:"name"`
	Arc     string `yaml:"arc"`
	Keys    string `yaml:"keys"`
	Content string `yaml:"content"`
}

type sceneDoc struct {
	Scene sceneYAML `yaml:"scene"`
}

type sceneYAML struct {
	Location     string   `yaml:"location"`
	TimeOfDay    string   `yaml:"time_of_day"`
	Weather      string   `yaml:"weather"`
	Characters   []string `yaml:"characters,omitempty"`
	Description  string   `yaml:"description,omitempty"`
	Atmosphere   string   `yaml:"atmosphere,omitempty"`
	PresentChars []string `yaml:"present_chars,omitempty"`
}

type factsDoc struct {
	Facts []factYAML `yaml:"facts"`
}

type presetsDoc struct {
	Presets []core.ScenarioPreset `yaml:"presets"`
}

type factYAML struct {
	Subject    string  `yaml:"subject"`
	Predicate  string  `yaml:"predicate"`
	Object     string  `yaml:"object"`
	Confidence float64 `yaml:"confidence"`
}

func ConvertToDir(filePath string) (string, error) {
	if isDirPath(filePath) {
		return "", fmt.Errorf("already a directory world: %s", filePath)
	}
	bundle, err := loadFileBundle(filePath)
	if err != nil {
		return "", fmt.Errorf("load single-file world: %w", err)
	}

	dir := strings.TrimSuffix(filePath, filepath.Ext(filePath))
	if _, err := os.Stat(dir); err == nil {
		return "", fmt.Errorf("target directory already exists: %s", dir)
	}

	for _, sub := range []string{"world", "canon", "scenes", "population"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0755); err != nil {
			return "", fmt.Errorf("create dir %s: %w", sub, err)
		}
	}

	worldYAML := map[string]interface{}{
		"meta":       map[string]string{"name": bundle.Config.Name},
		"core_rules": bundle.Config.CoreRules,
	}
	data, _ := yaml.Marshal(worldYAML)
	if err := os.WriteFile(filepath.Join(dir, "world.yml"), data, 0644); err != nil {
		return "", err
	}

	if len(bundle.Scenes) > 0 {
		sceneData, _ := yaml.Marshal(map[string]interface{}{"scene": bundle.Scenes[0].Scene})
		if err := os.WriteFile(filepath.Join(dir, "scenes", "default.yml"), sceneData, 0644); err != nil {
			return "", err
		}
	}

	emptyYAML := []byte("[]\n")
	os.WriteFile(filepath.Join(dir, "canon", "facts.yml"), emptyYAML, 0644)
	os.WriteFile(filepath.Join(dir, "world", "factions.yml"), emptyYAML, 0644)
	os.WriteFile(filepath.Join(dir, "world", "locations.yml"), emptyYAML, 0644)
	os.WriteFile(filepath.Join(dir, "world", "pressures.yml"), emptyYAML, 0644)
	seedData, _ := yaml.Marshal(map[string]interface{}{"premise": ""})
	os.WriteFile(filepath.Join(dir, "world", "seed.yml"), seedData, 0644)

	backupPath := filePath + ".bak"
	if err := os.Rename(filePath, backupPath); err != nil {
		return dir, nil
	}

	return dir, nil
}

func CreateWorld(rootDir, name, coreRules string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("world name is required")
	}
	id := sanitizeID(name)
	if id == "" {
		id = "world"
	}
	dir := filepath.Join(rootDir, id)
	if _, err := os.Stat(dir); err == nil {
		return "", fmt.Errorf("world '%s' already exists at %s", name, dir)
	}

	for _, sub := range []string{"world", "canon", "scenes", "population"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0755); err != nil {
			return "", fmt.Errorf("create dir %s: %w", sub, err)
		}
	}

	worldYAML := map[string]interface{}{
		"meta": map[string]string{"name": name},
	}
	if strings.TrimSpace(coreRules) != "" {
		worldYAML["core_rules"] = coreRules
	}
	data, _ := yaml.Marshal(worldYAML)
	if err := os.WriteFile(filepath.Join(dir, "world.yml"), data, 0644); err != nil {
		return "", err
	}

	defaultScene := core.SceneState{Location: "起点", TimeOfDay: "白天", Weather: "晴", Characters: []string{}, Description: ""}
	sceneData, _ := yaml.Marshal(map[string]interface{}{"scene": defaultScene})
	if err := os.WriteFile(filepath.Join(dir, "scenes", "default.yml"), sceneData, 0644); err != nil {
		return "", err
	}

	emptyYAML := []byte("[]\n")
	os.WriteFile(filepath.Join(dir, "canon", "facts.yml"), emptyYAML, 0644)
	os.WriteFile(filepath.Join(dir, "world", "factions.yml"), emptyYAML, 0644)
	os.WriteFile(filepath.Join(dir, "world", "locations.yml"), emptyYAML, 0644)
	os.WriteFile(filepath.Join(dir, "world", "pressures.yml"), emptyYAML, 0644)

	seedData, _ := yaml.Marshal(map[string]interface{}{"premise": ""})
	os.WriteFile(filepath.Join(dir, "world", "seed.yml"), seedData, 0644)

	return dir, nil
}

func sanitizeID(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '_':
			b.WriteRune('_')
		}
	}
	return strings.Trim(b.String(), "_")
}

func LoadBundle(path string) (Bundle, error) {
	if isDirPath(path) {
		return loadDirBundle(path)
	}
	return loadFileBundle(path)
}

func LoadConfig(path string) (core.WorldConfig, error) {
	b, err := LoadBundle(path)
	if err != nil {
		return core.WorldConfig{}, err
	}
	return b.Config, nil
}

func SaveConfig(path string, cfg core.WorldConfig) (core.WorldConfig, error) {
	cfg.Name = strings.TrimSpace(cfg.Name)
	cfg.CoreRules = strings.TrimSpace(cfg.CoreRules)
	if cfg.Name == "" {
		return core.WorldConfig{}, fmt.Errorf("world name is required")
	}
	if isDirPath(path) {
		return saveDirConfig(path, cfg)
	}
	return saveFileConfig(path, cfg)
}

func ListScenes(path string) (core.SceneConfigList, error) {
	b, err := LoadBundle(path)
	if err != nil {
		return core.SceneConfigList{}, err
	}
	return core.SceneConfigList{Selected: b.Selected, Scenes: b.Scenes}, nil
}

func SaveScene(path string, scene core.SceneConfig) (core.SceneConfig, error) {
	scene.Name = strings.TrimSpace(scene.Name)
	if scene.Name == "" {
		scene.Name = "default"
	}
	if isDirPath(path) {
		return saveDirScene(path, scene)
	}
	return saveFileScene(path, scene)
}

func LoadFacts(path string) (core.CanonFactsConfig, error) {
	b, err := LoadBundle(path)
	if err != nil {
		return core.CanonFactsConfig{}, err
	}
	factsPath := path
	if isDirPath(path) {
		factsPath = filepath.Join(path, "canon", "facts.yml")
	}
	return core.CanonFactsConfig{Path: factsPath, Facts: b.DirectFacts}, nil
}

func LoadScenarioPresets(path string) ([]core.ScenarioPreset, error) {
	if !isDirPath(path) {
		return []core.ScenarioPreset{}, nil
	}
	data, err := os.ReadFile(filepath.Join(path, "world", "presets.yml"))
	if err != nil {
		if os.IsNotExist(err) {
			return []core.ScenarioPreset{}, nil
		}
		return nil, err
	}
	var raw presetsDoc
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	for i := range raw.Presets {
		raw.Presets[i].Name = strings.TrimSpace(raw.Presets[i].Name)
		raw.Presets[i].Branch = strings.TrimSpace(raw.Presets[i].Branch)
		raw.Presets[i].Character = strings.TrimSpace(raw.Presets[i].Character)
		raw.Presets[i].Note = strings.TrimSpace(raw.Presets[i].Note)
		raw.Presets[i].Preview = strings.TrimSpace(raw.Presets[i].Preview)
	}
	return raw.Presets, nil
}

func LoadPopulation(path string) (core.PopulationConfig, error) {
	b, err := LoadBundle(path)
	if err != nil {
		return core.PopulationConfig{}, err
	}
	return b.Population, nil
}

func EnsureSeededPopulation(path string) (core.PopulationConfig, bool, error) {
	cfg, err := LoadPopulation(path)
	if err != nil {
		return core.PopulationConfig{}, false, err
	}
	if !isDirPath(path) {
		return cfg, false, nil
	}
	if len(cfg.BackgroundNPCs) > 0 || len(cfg.PromotedNPCs) > 0 {
		return cfg, false, nil
	}
	seeded, changed, err := seedPopulationFromWorld(path, cfg)
	if err != nil || !changed {
		return seeded, changed, err
	}
	saved, err := SavePopulation(path, seeded)
	if err != nil {
		return core.PopulationConfig{}, false, err
	}
	return saved, true, nil
}

func SavePopulation(path string, cfg core.PopulationConfig) (core.PopulationConfig, error) {
	if isDirPath(path) {
		return saveDirPopulation(path, cfg)
	}
	return saveFilePopulation(path, cfg)
}

func SaveFacts(path string, cfg core.CanonFactsConfig) (core.CanonFactsConfig, error) {
	if isDirPath(path) {
		return saveDirFacts(path, cfg.Facts)
	}
	return saveFileFacts(path, cfg.Facts)
}

func isDirPath(path string) bool {
	if strings.HasSuffix(path, "/") || strings.HasSuffix(path, string(filepath.Separator)) {
		return true
	}
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func loadDirBundle(dir string) (Bundle, error) {
	cfg, err := readDirConfig(dir)
	if err != nil {
		return Bundle{}, err
	}
	scenes, err := readDirScenes(dir)
	if err != nil {
		return Bundle{}, err
	}
	onto, _ := readOntology(dir)
	facts, _ := readDirFacts(dir)
	population, _ := readDirPopulation(dir)
	return Bundle{
		Config:      cfg,
		Scenes:      scenes,
		Selected:    "default",
		Ontology:    onto,
		DirectFacts: facts,
		Population:  population,
	}, nil
}

func loadFileBundle(path string) (Bundle, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Bundle{}, err
	}
	var raw struct {
		Name      string    `yaml:"name"`
		CoreRules string    `yaml:"core_rules"`
		Scene     sceneYAML `yaml:"scene"`
		Ontology  Ontology  `yaml:"ontology"`
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return Bundle{}, err
	}
	scene := normalizeScene(raw.Scene)
	return Bundle{
		Config: core.WorldConfig{
			Name:      raw.Name,
			CoreRules: raw.CoreRules,
			Path:      path,
			Format:    "single_file",
		},
		Scenes: []core.SceneConfig{{
			Name:  "default",
			Path:  path,
			Scene: scene,
		}},
		Selected:    "default",
		Ontology:    raw.Ontology,
		DirectFacts: nil,
		Population:  defaultPopulationConfig(path),
	}, nil
}

func readDirConfig(dir string) (core.WorldConfig, error) {
	data, err := os.ReadFile(filepath.Join(dir, "world.yml"))
	if err != nil {
		return core.WorldConfig{}, err
	}
	var raw struct {
		Meta struct {
			Name string `yaml:"name"`
		} `yaml:"meta"`
		Name      string `yaml:"name"`
		CoreRules string `yaml:"core_rules"`
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return core.WorldConfig{}, err
	}
	name := strings.TrimSpace(raw.Meta.Name)
	if name == "" {
		name = strings.TrimSpace(raw.Name)
	}
	return core.WorldConfig{
		Name:      name,
		CoreRules: raw.CoreRules,
		Path:      dir,
		Format:    "world_dir",
	}, nil
}

func readDirScenes(dir string) ([]core.SceneConfig, error) {
	entries, err := os.ReadDir(filepath.Join(dir, "scenes"))
	if err != nil {
		if os.IsNotExist(err) {
			return []core.SceneConfig{}, nil
		}
		return nil, err
	}
	var scenes []core.SceneConfig
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yml") {
			continue
		}
		path := filepath.Join(dir, "scenes", entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var raw sceneDoc
		if err := yaml.Unmarshal(data, &raw); err != nil {
			return nil, err
		}
		scenes = append(scenes, core.SceneConfig{
			Name:  strings.TrimSuffix(entry.Name(), ".yml"),
			Path:  path,
			Scene: normalizeScene(raw.Scene),
		})
	}
	sort.Slice(scenes, func(i, j int) bool { return scenes[i].Name < scenes[j].Name })
	return scenes, nil
}

func readOntology(dir string) (Ontology, error) {
	data, err := os.ReadFile(filepath.Join(dir, "canon", "ontology.yml"))
	if err != nil {
		return Ontology{}, err
	}
	var raw struct {
		Ontology Ontology `yaml:"ontology"`
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return Ontology{}, err
	}
	return raw.Ontology, nil
}

func readDirFacts(dir string) ([]core.FactFrame, error) {
	data, err := os.ReadFile(filepath.Join(dir, "canon", "facts.yml"))
	if err != nil {
		if os.IsNotExist(err) {
			return []core.FactFrame{}, nil
		}
		return nil, err
	}
	var raw factsDoc
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	facts := make([]core.FactFrame, 0, len(raw.Facts))
	for _, f := range raw.Facts {
		facts = append(facts, core.FactFrame{
			Subject:    f.Subject,
			Predicate:  f.Predicate,
			Object:     f.Object,
			Confidence: f.Confidence,
		})
	}
	return facts, nil
}

func saveDirConfig(dir string, cfg core.WorldConfig) (core.WorldConfig, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return core.WorldConfig{}, err
	}
	data, err := yaml.Marshal(map[string]interface{}{
		"meta": map[string]string{
			"name":    cfg.Name,
			"version": "1.0",
			"source":  "corerp_editor",
		},
		"core_rules": cfg.CoreRules,
	})
	if err != nil {
		return core.WorldConfig{}, err
	}
	if err := os.WriteFile(filepath.Join(dir, "world.yml"), data, 0644); err != nil {
		return core.WorldConfig{}, err
	}
	cfg.Path = dir
	cfg.Format = "world_dir"
	return cfg, nil
}

func saveFileConfig(path string, cfg core.WorldConfig) (core.WorldConfig, error) {
	b, err := loadFileBundle(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return core.WorldConfig{}, err
		}
	}
	doc := map[string]interface{}{
		"name":       cfg.Name,
		"core_rules": cfg.CoreRules,
		"scene":      sceneFromState(sceneByName(b.Scenes, "default").Scene),
	}
	if len(b.Ontology.Characters)+len(b.Ontology.Locations)+len(b.Ontology.Factions)+len(b.Ontology.Items)+len(b.Ontology.Lore)+len(b.Ontology.Events)+len(b.Ontology.Timelines)+len(b.Ontology.Settings) > 0 {
		doc["ontology"] = b.Ontology
	}
	data, err := yaml.Marshal(doc)
	if err != nil {
		return core.WorldConfig{}, err
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return core.WorldConfig{}, err
	}
	cfg.Path = path
	cfg.Format = "single_file"
	return cfg, nil
}

func saveDirScene(dir string, scene core.SceneConfig) (core.SceneConfig, error) {
	if err := os.MkdirAll(filepath.Join(dir, "scenes"), 0755); err != nil {
		return core.SceneConfig{}, err
	}
	path := filepath.Join(dir, "scenes", scene.Name+".yml")
	data, err := yaml.Marshal(sceneDoc{Scene: sceneFromState(scene.Scene)})
	if err != nil {
		return core.SceneConfig{}, err
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return core.SceneConfig{}, err
	}
	scene.Path = path
	return scene, nil
}

func saveFileScene(path string, scene core.SceneConfig) (core.SceneConfig, error) {
	b, err := loadFileBundle(path)
	if err != nil {
		return core.SceneConfig{}, err
	}
	doc := map[string]interface{}{
		"name":       b.Config.Name,
		"core_rules": b.Config.CoreRules,
		"scene":      sceneFromState(scene.Scene),
	}
	if len(b.Ontology.Characters)+len(b.Ontology.Locations)+len(b.Ontology.Factions)+len(b.Ontology.Items)+len(b.Ontology.Lore)+len(b.Ontology.Events)+len(b.Ontology.Timelines)+len(b.Ontology.Settings) > 0 {
		doc["ontology"] = b.Ontology
	}
	data, err := yaml.Marshal(doc)
	if err != nil {
		return core.SceneConfig{}, err
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return core.SceneConfig{}, err
	}
	scene.Path = path
	scene.Name = "default"
	return scene, nil
}

func saveDirFacts(dir string, facts []core.FactFrame) (core.CanonFactsConfig, error) {
	if err := os.MkdirAll(filepath.Join(dir, "canon"), 0755); err != nil {
		return core.CanonFactsConfig{}, err
	}
	path := filepath.Join(dir, "canon", "facts.yml")
	data, err := yaml.Marshal(factsDoc{Facts: factsToYAML(facts)})
	if err != nil {
		return core.CanonFactsConfig{}, err
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return core.CanonFactsConfig{}, err
	}
	return core.CanonFactsConfig{Path: path, Facts: facts}, nil
}

func saveDirPopulation(dir string, cfg core.PopulationConfig) (core.PopulationConfig, error) {
	cfg = normalizePopulationConfig(cfg, dir)
	popDir := filepath.Join(dir, "population")
	if err := os.MkdirAll(popDir, 0755); err != nil {
		return core.PopulationConfig{}, err
	}
	if err := writePopulationFiles(popDir, cfg); err != nil {
		return core.PopulationConfig{}, err
	}
	return cfg, nil
}

func saveFileFacts(path string, facts []core.FactFrame) (core.CanonFactsConfig, error) {
	// Single-file worlds do not have a dedicated facts section yet.
	return core.CanonFactsConfig{}, fmt.Errorf("single-file world does not support canon/facts editing; import into directory format first")
}

func saveFilePopulation(path string, cfg core.PopulationConfig) (core.PopulationConfig, error) {
	return core.PopulationConfig{}, fmt.Errorf("single-file world does not support population editing; import into directory format first")
}

func sceneFromState(scene core.SceneState) sceneYAML {
	return sceneYAML{
		Location:     scene.Location,
		TimeOfDay:    scene.TimeOfDay,
		Weather:      scene.Weather,
		Atmosphere:   scene.Description,
		PresentChars: append([]string(nil), scene.Characters...),
	}
}

func normalizeScene(raw sceneYAML) core.SceneState {
	chars := raw.PresentChars
	if len(chars) == 0 {
		chars = raw.Characters
	}
	desc := raw.Atmosphere
	if desc == "" {
		desc = raw.Description
	}
	return core.SceneState{
		Location:    raw.Location,
		TimeOfDay:   raw.TimeOfDay,
		Weather:     raw.Weather,
		Characters:  append([]string(nil), chars...),
		Description: desc,
	}
}

func factsToYAML(facts []core.FactFrame) []factYAML {
	out := make([]factYAML, 0, len(facts))
	for _, f := range facts {
		out = append(out, factYAML{
			Subject:    f.Subject,
			Predicate:  f.Predicate,
			Object:     f.Object,
			Confidence: f.Confidence,
		})
	}
	return out
}

func sceneByName(scenes []core.SceneConfig, name string) core.SceneConfig {
	for _, scene := range scenes {
		if scene.Name == name {
			return scene
		}
	}
	return core.SceneConfig{Name: name}
}

type backgroundNPCDoc struct {
	BackgroundNPCs []core.BackgroundNPC `yaml:"background_npcs"`
}

type promotedNPCDoc struct {
	PromotedNPCs []core.PromotedNPC `yaml:"promoted_npcs"`
}

type identityCoreDoc struct {
	IdentityCores []core.IdentityCoreConfig `yaml:"identity_cores"`
}

type promotionPolicyDoc struct {
	Policy core.PromotionPolicy `yaml:"policy"`
}

func readDirPopulation(dir string) (core.PopulationConfig, error) {
	cfg := normalizePopulationConfig(defaultPopulationConfig(dir), dir)
	popDir := filepath.Join(dir, "population")

	if data, err := os.ReadFile(filepath.Join(popDir, "background_npcs.yml")); err == nil {
		var raw backgroundNPCDoc
		if err := yaml.Unmarshal(data, &raw); err != nil {
			return core.PopulationConfig{}, err
		}
		cfg.BackgroundNPCs = raw.BackgroundNPCs
	} else if !os.IsNotExist(err) {
		return core.PopulationConfig{}, err
	}

	if data, err := os.ReadFile(filepath.Join(popDir, "promoted_npcs.yml")); err == nil {
		var raw promotedNPCDoc
		if err := yaml.Unmarshal(data, &raw); err != nil {
			return core.PopulationConfig{}, err
		}
		cfg.PromotedNPCs = raw.PromotedNPCs
	} else if !os.IsNotExist(err) {
		return core.PopulationConfig{}, err
	}

	if data, err := os.ReadFile(filepath.Join(popDir, "identity_core.yml")); err == nil {
		var raw identityCoreDoc
		if err := yaml.Unmarshal(data, &raw); err != nil {
			return core.PopulationConfig{}, err
		}
		cfg.IdentityCores = raw.IdentityCores
	} else if !os.IsNotExist(err) {
		return core.PopulationConfig{}, err
	}

	if data, err := os.ReadFile(filepath.Join(popDir, "policy.yml")); err == nil {
		var raw promotionPolicyDoc
		if err := yaml.Unmarshal(data, &raw); err != nil {
			return core.PopulationConfig{}, err
		}
		cfg.Policy = normalizePromotionPolicy(raw.Policy)
	} else if !os.IsNotExist(err) {
		return core.PopulationConfig{}, err
	}

	return normalizePopulationConfig(cfg, dir), nil
}

func writePopulationFiles(popDir string, cfg core.PopulationConfig) error {
	files := []struct {
		name string
		doc  interface{}
	}{
		{name: "background_npcs.yml", doc: backgroundNPCDoc{BackgroundNPCs: cfg.BackgroundNPCs}},
		{name: "promoted_npcs.yml", doc: promotedNPCDoc{PromotedNPCs: cfg.PromotedNPCs}},
		{name: "identity_core.yml", doc: identityCoreDoc{IdentityCores: cfg.IdentityCores}},
		{name: "policy.yml", doc: promotionPolicyDoc{Policy: cfg.Policy}},
	}
	for _, file := range files {
		data, err := yaml.Marshal(file.doc)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(popDir, file.name), data, 0644); err != nil {
			return err
		}
	}
	return nil
}

func defaultPopulationConfig(path string) core.PopulationConfig {
	return core.PopulationConfig{
		Path:   filepath.ToSlash(filepath.Clean(path)),
		Policy: normalizePromotionPolicy(core.PromotionPolicy{}),
	}
}

func normalizePopulationConfig(cfg core.PopulationConfig, path string) core.PopulationConfig {
	cfg.Path = filepath.ToSlash(filepath.Clean(path))
	cfg.Policy = normalizePromotionPolicy(cfg.Policy)
	if cfg.BackgroundNPCs == nil {
		cfg.BackgroundNPCs = []core.BackgroundNPC{}
	}
	if cfg.PromotedNPCs == nil {
		cfg.PromotedNPCs = []core.PromotedNPC{}
	}
	if cfg.IdentityCores == nil {
		cfg.IdentityCores = []core.IdentityCoreConfig{}
	}
	return cfg
}

func normalizePromotionPolicy(policy core.PromotionPolicy) core.PromotionPolicy {
	if policy.PromoteThreshold <= 0 {
		policy.PromoteThreshold = 10
	}
	if policy.MajorThreshold <= 0 {
		policy.MajorThreshold = 25
	}
	if policy.InteractionWeight == 0 {
		policy.InteractionWeight = 3
	}
	if policy.MentionWeight == 0 {
		policy.MentionWeight = 1
	}
	if policy.EventWeight == 0 {
		policy.EventWeight = 5
	}
	if policy.RelationshipWeight == 0 {
		policy.RelationshipWeight = 4
	}
	if policy.SceneWeight == 0 {
		policy.SceneWeight = 2
	}
	return policy
}

func seedPopulationFromWorld(path string, cfg core.PopulationConfig) (core.PopulationConfig, bool, error) {
	bundle, err := LoadBundle(path)
	if err != nil {
		return core.PopulationConfig{}, false, err
	}

	locationNames := orderedPopulationLocations(bundle)
	if len(locationNames) == 0 {
		locationNames = []string{"场景边缘"}
	}
	factionNames := orderedPopulationFactions(bundle)

	type populationTemplate struct {
		id       string
		name     string
		role     string
		location string
		faction  string
		traits   []string
		hooks    []string
	}

	pickLocation := func(index int) string {
		if len(locationNames) == 0 {
			return ""
		}
		if index < len(locationNames) {
			return locationNames[index]
		}
		return locationNames[0]
	}
	pickFaction := func(index int) string {
		if len(factionNames) == 0 {
			return ""
		}
		if index < len(factionNames) {
			return factionNames[index]
		}
		return factionNames[0]
	}

	templates := []populationTemplate{
		{
			id:       "watcher",
			name:     buildPopulationName(pickLocation(0), pickFaction(0), "巡守"),
			role:     "巡守",
			location: pickLocation(0),
			faction:  pickFaction(0),
			traits:   []string{"克制", "警惕"},
			hooks:    []string{"维持局面稳定", "盯住最近的异常动静"},
		},
		{
			id:       "vendor",
			name:     buildPopulationName(pickLocation(0), "", "摊主"),
			role:     "摊主",
			location: pickLocation(0),
			traits:   []string{"健谈", "留心风声"},
			hooks:    []string{"想把消息换成筹码", "对谁在失势很敏感"},
		},
		{
			id:       "runner",
			name:     buildPopulationName(pickLocation(1), pickFaction(1), "跑腿"),
			role:     "跑腿",
			location: pickLocation(1),
			faction:  pickFaction(1),
			traits:   []string{"机灵", "不愿站队"},
			hooks:    []string{"想活着穿过各方夹缝", "总比别人先听到一点消息"},
		},
		{
			id:       "clerk",
			name:     buildPopulationName(pickLocation(2), "", "管事"),
			role:     "管事",
			location: pickLocation(2),
			traits:   []string{"谨慎", "记账式思维"},
			hooks:    []string{"试着维持日常秩序", "担心局势继续失衡"},
		},
	}

	seenNames := map[string]bool{}
	background := make([]core.BackgroundNPC, 0, len(templates))
	for _, tpl := range templates {
		name := strings.TrimSpace(tpl.name)
		if name == "" || seenNames[name] {
			continue
		}
		seenNames[name] = true
		background = append(background, core.BackgroundNPC{
			ID:       tpl.id,
			Name:     name,
			Role:     tpl.role,
			Location: tpl.location,
			Faction:  tpl.faction,
			Traits:   append([]string(nil), tpl.traits...),
			Hooks:    append([]string(nil), tpl.hooks...),
		})
	}
	if len(background) == 0 {
		return cfg, false, nil
	}
	cfg.BackgroundNPCs = background
	return normalizePopulationConfig(cfg, path), true, nil
}

func orderedPopulationLocations(bundle Bundle) []string {
	seen := map[string]bool{}
	var out []string
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			return
		}
		seen[value] = true
		out = append(out, value)
	}

	if scene := sceneByName(bundle.Scenes, bundle.Selected); scene.Scene.Location != "" {
		add(scene.Scene.Location)
	}
	for _, scene := range bundle.Scenes {
		add(scene.Scene.Location)
	}
	if structure, err := LoadStructure(bundle.Config.Path); err == nil {
		if structure.Seed.StartingScene != "" {
			add(structure.Seed.StartingScene)
		}
		for _, loc := range structure.Locations {
			add(loc.Name)
		}
	}
	for _, loc := range bundle.Ontology.Locations {
		add(loc.Name)
	}
	return out
}

func orderedPopulationFactions(bundle Bundle) []string {
	seen := map[string]bool{}
	var out []string
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			return
		}
		seen[value] = true
		out = append(out, value)
	}
	if structure, err := LoadStructure(bundle.Config.Path); err == nil {
		for _, faction := range structure.Factions {
			add(faction.Name)
		}
	}
	for _, faction := range bundle.Ontology.Factions {
		add(faction.Name)
	}
	return out
}

func buildPopulationName(location, faction, role string) string {
	location = strings.TrimSpace(location)
	faction = strings.TrimSpace(faction)
	role = strings.TrimSpace(role)
	if faction != "" {
		return faction + role
	}
	if location != "" {
		return location + role
	}
	return role
}
