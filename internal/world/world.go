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

type factYAML struct {
	Subject    string  `yaml:"subject"`
	Predicate  string  `yaml:"predicate"`
	Object     string  `yaml:"object"`
	Confidence float64 `yaml:"confidence"`
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

func LoadPopulation(path string) (core.PopulationConfig, error) {
	b, err := LoadBundle(path)
	if err != nil {
		return core.PopulationConfig{}, err
	}
	return b.Population, nil
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
