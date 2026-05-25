package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"corerp/internal/agents"
	"corerp/internal/api"
	"corerp/internal/auth"
	"corerp/internal/core"
	"corerp/internal/events"
	"corerp/internal/importer"
	"corerp/internal/llm"
	"corerp/internal/memory"
	"corerp/internal/runtime"

	"gopkg.in/yaml.v3"
)

func main() {
	if len(os.Args) < 2 {
		args := append([]string{"serve"}, os.Args[1:]...)
		os.Args = args
	}

	switch os.Args[1] {
	case "import":
		runImport(os.Args[2:])
	case "serve":
		runServe(os.Args[2:])
	default:
		log.Fatalf("Unknown command: %s. Use: serve | import", os.Args[1])
	}
}

// === import subcommand ===

func runImport(args []string) {
	fs := flag.NewFlagSet("import", flag.ExitOnError)
	src := fs.String("src", "", "Source PNG file or directory")
	dst := fs.String("dst", "./characters", "Output directory for YAML files")
	fs.Parse(args)

	if *src == "" {
		fmt.Println("Usage: corerp import -src <png_or_dir> [-dst ./characters]")
		os.Exit(1)
	}

	info, err := os.Stat(*src)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}

	os.MkdirAll(*dst, 0755)

	if info.IsDir() {
		results, err := importer.ImportDirectory(*src, *dst)
		if err != nil {
			log.Fatalf("Import failed: %v", err)
		}
		for _, r := range results {
			fmt.Println(r)
		}
	} else if strings.HasSuffix(strings.ToLower(*src), ".json") {
		charPath, worldPath, err := importer.ImportJSON(*src, *dst)
		if err != nil {
			log.Fatalf("Import failed: %v", err)
		}
		fmt.Printf("Imported character: %s\n", charPath)
		fmt.Printf("Imported world:     %s\n", worldPath)
	} else {
		charPath, worldPath, err := importer.ImportPNG(*src, *dst)
		if err != nil {
			log.Fatalf("Import failed: %v", err)
		}
		fmt.Printf("Imported character: %s\n", charPath)
		fmt.Printf("Imported world:     %s\n", worldPath)
	}
}

// === serve subcommand ===

func runServe(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	port := fs.String("port", "8080", "HTTP server port")
	dataDir := fs.String("data", "./data", "Data directory")
	charFile := fs.String("character", "characters/anya.yml", "Single character YAML (use -characters dir for multi)")
	charDir := fs.String("characters", "", "Directory of character YAML files (loads all *.yml)")
	worldFile := fs.String("world", "worlds/cyberpunk2077/world.yml", "World YAML file")
	llmURL := fs.String("llm-url", os.Getenv("LLM_URL"), "LLM API endpoint")
	llmKey := fs.String("llm-key", os.Getenv("LLM_API_KEY"), "LLM API key")
	llmModel := fs.String("llm-model", os.Getenv("LLM_MODEL"), "LLM model name")
		summaryURL := fs.String("llm-summary-url", "", "Optional separate LLM endpoint for summaries (defaults to main)")
		summaryModel := fs.String("llm-summary-model", "", "Optional separate model for summaries (defaults to main)")
		authKey := fs.String("auth-key", os.Getenv("CORERP_AUTH_KEY"), "Access password (empty = no auth)")
	fs.Parse(args)

	if *llmURL == "" {
		*llmURL = "http://localhost:11434/v1"
	}
	if *llmModel == "" {
		*llmModel = "qwen2.5:7b"
	}

	os.MkdirAll(*dataDir, 0755)

	// Load characters: directory mode takes precedence
	var chars []core.Character
	var charNames []string
	var charPaths []string
	if *charDir != "" {
		chars, charNames, charPaths = loadCharactersFromDir(*charDir)
		if len(chars) == 0 {
			log.Fatalf("No character YAML files found in %s", *charDir)
		}
	} else {
		char := loadCharacter(*charFile)
		chars = []core.Character{char}
		charNames = []string{char.Identity.Name}
		charPaths = []string{*charFile}
	}
	activeName := charNames[0]

	// Load per-character worlds
	charWorlds := make(map[string]runtime.CharWorld)
	for i, name := range charNames {
		var wf string
		if *charDir != "" {
			wf = findWorldFile(charPaths[i])
		}
		if wf == "" {
			wf = *worldFile
		}
		var w loadedWorld
		if strings.HasSuffix(wf, "/") || strings.HasSuffix(wf, string(filepath.Separator)) {
			w = loadWorldDir(wf)
		} else {
			w = loadWorld(wf)
		}
		charWorlds[name] = runtime.CharWorld{
			WorldName: w.Name,
			CoreRules: w.CoreRules,
			Scene: core.SceneState{
				Location:    w.Scene.Location,
				TimeOfDay:   w.Scene.TimeOfDay,
				Weather:     w.Scene.Weather,
				Characters:  w.Scene.Characters,
				Description: w.Scene.Description,
			},
		}
		log.Printf("Character '%s' → world '%s' (%s)", name, w.Name, wf)
	}

	// Init stores
	dbPath := *dataDir + "/memory.db"
	eventStore, err := events.New(dbPath)
	if err != nil {
		log.Fatalf("Failed to init event store: %v", err)
	}
	defer eventStore.Close()

	gatekeeper := events.NewGatekeeper(eventStore)

	memEngine, err := memory.New(dbPath)
	if err != nil {
		log.Fatalf("Failed to init memory engine: %v", err)
	}
	defer memEngine.Close()

	decayEngine := memory.NewDecayEngine(memEngine.DB())
	if cp := memory.NewConfidencePipeline(memEngine.DB()); cp != nil {
		cp.Migrate()
	}

	// Seed ontology per character from their own world
	for i, name := range charNames {
		// Re-load full world for ontology (includes ontology section)
		var wf string
		if *charDir != "" {
			wf = findWorldFile(charPaths[i])
		}
		if wf == "" {
			wf = *worldFile
		}
		var fullWorld loadedWorld
		if strings.HasSuffix(wf, "/") || strings.HasSuffix(wf, string(filepath.Separator)) {
			fullWorld = loadWorldDir(wf)
		} else {
			fullWorld = loadWorld(wf)
		}
		seedOntology(memEngine, &fullWorld, name)
		log.Printf("Ontology seeded: %d facts, %d events into '%s'",
			countOntologyFacts(&fullWorld), countOntologyEvents(&fullWorld), name)
	}

	// Init agents — load all characters
	agentsMgr := agents.NewEnvelopeManager()
	for i, c := range chars {
		agentsMgr.LoadCharacter(charNames[i], c)
		log.Printf("Loaded character: %s", charNames[i])
	}

	// Init auth
	auth.Init(*authKey)
	if auth.IsEnabled() {
		log.Printf("Auth: enabled (set via -auth-key or CORERP_AUTH_KEY)")
	}

	// Init LLM config store
	llm.InitConfigStore(*dataDir + "/llm_configs.json")

	// Init LLM router + usage logger + compact previous month
	usagePath := *dataDir + "/llm_usage.jsonl"
	llm.InitUsageLogger(usagePath)
	llm.CompactMonth(usagePath) // nop if current month has no prior records
	llm.SetActiveConfig("default", *llmURL, *llmKey, *llmModel)
	defaultAdapter := llm.NewAdapter(*llmURL, *llmKey, *llmModel)
	llmRouter := llm.NewRouter(defaultAdapter)

	// Configure separate summary model if provided
	if *summaryURL != "" && *summaryModel != "" {
		summaryAdapter := llm.NewAdapter(*summaryURL, *llmKey, *summaryModel)
		llmRouter.AddAdapter("summary", summaryAdapter)
		llmRouter.SetRoute(llm.TaskSummary, "summary")
		log.Printf("LLM router: summary task → %s @ %s", *summaryModel, *summaryURL)
	}

	// Init runtime engine
	engine, err := runtime.New(
		eventStore,
		gatekeeper,
		memEngine,
		decayEngine,
		agentsMgr,
		llmRouter,
		activeName,
		charNames,
		charWorlds,
	)
	if err != nil {
		log.Fatalf("Failed to init runtime: %v", err)
	}

	// Load existing state from events
	if err := engine.LoadState(); err != nil {
		log.Printf("Warning: failed to load state: %v", err)
	}

	// Seed initial scene if empty
	initWorld := charWorlds[activeName]
	engine.SeedScene(core.SceneState{
		Location:    initWorld.Scene.Location,
		TimeOfDay:   initWorld.Scene.TimeOfDay,
		Weather:     initWorld.Scene.Weather,
		Characters:  initWorld.Scene.Characters,
		Description: initWorld.Scene.Description,
	})

	// Start autonomous tick loop
	engine.StartTickLoop()
	defer engine.Stop()

	// Start embedding server (best-effort, fallback to 2-gram if unavailable)
	go func() {
		cmd, err := memory.StartEmbedServer()
		if err != nil {
			log.Printf("Embed server: %v (falling back to 2-gram)", err)
			return
		}
		log.Printf("Embed server started (PID %d)", cmd.Process.Pid)
	}()

	log.Printf("CoreRP started")
	log.Printf("Active character: %s", activeName)
	log.Printf("World: %s", charWorlds[activeName].WorldName)
	log.Printf("LLM: %s @ %s", *llmModel, *llmURL)
	log.Printf("Listening on http://localhost:%s", *port)

	// Setup HTTP
	mux := http.NewServeMux()
	server := api.NewServer(engine)
	server.Register(mux)

	if err := http.ListenAndServe(":"+*port, mux); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

func findWorldFile(charPath string) string {
	base := strings.TrimSuffix(charPath, ".yml")
	fileName := filepath.Base(charPath)
	fileBase := strings.TrimSuffix(fileName, ".yml")

	// New: directory structure worlds/{name}/
	dirCandidates := []string{
		"worlds/" + fileBase + "/",
	}
	for _, c := range dirCandidates {
		if info, err := os.Stat(c); err == nil && info.IsDir() {
			return c
		}
	}

	// Old: single _world.yml files
	fileCandidates := []string{
		base + "_world.yml",
		"worlds/" + fileBase + "_world.yml",
	}
	for _, c := range fileCandidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}

func loadCharactersFromDir(dir string) (chars []core.Character, names []string, paths []string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		log.Fatalf("Failed to read characters directory: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yml") {
			continue
		}
		// Skip world files
		if strings.HasSuffix(e.Name(), "_world.yml") {
			continue
		}
		path := dir + "/" + e.Name()
		char := loadCharacter(path)
		chars = append(chars, char)
		names = append(names, char.Identity.Name)
		paths = append(paths, path)
	}
	return
}

func loadCharacter(path string) core.Character {
	charData, err := os.ReadFile(path)
	if err != nil {
		log.Fatalf("Failed to read character file: %v", err)
	}
	var charRaw struct {
		Identity struct {
			Name         string             `yaml:"name"`
			Immutable    []string           `yaml:"immutable"`
			Adaptive     map[string]float64 `yaml:"adaptive"`
			Forbidden    []string           `yaml:"forbidden"`
			Voice        struct {
				Style  string `yaml:"style"`
				Rhythm string `yaml:"rhythm"`
			} `yaml:"voice"`
			WritingGuide string `yaml:"writing_guide"`
		} `yaml:"identity"`
		Goals struct {
			Primary []struct {
				ID        string `yaml:"id"`
				Priority  int    `yaml:"priority"`
				Condition string `yaml:"condition"`
				Target    string `yaml:"target"`
			} `yaml:"primary"`
			Secondary []struct {
				ID        string `yaml:"id"`
				Priority  int    `yaml:"priority"`
				Condition string `yaml:"condition"`
				Target    string `yaml:"target"`
			} `yaml:"secondary"`
			Hidden []struct {
				ID              string   `yaml:"id"`
				Priority        int      `yaml:"priority"`
				KnownBy         []string `yaml:"known_by"`
				RevealCondition string   `yaml:"reveal_condition"`
			} `yaml:"hidden"`
		} `yaml:"goals"`
	}
	if err := yaml.Unmarshal(charData, &charRaw); err != nil {
		log.Fatalf("Failed to parse character YAML: %v", err)
	}

	var char core.Character
	char.Identity = core.IdentityEnvelope{
		Name:         charRaw.Identity.Name,
		Immutable:    charRaw.Identity.Immutable,
		Adaptive:     charRaw.Identity.Adaptive,
		Forbidden:    charRaw.Identity.Forbidden,
		Voice: core.VoiceConfig{
			Style:  charRaw.Identity.Voice.Style,
			Rhythm: charRaw.Identity.Voice.Rhythm,
		},
		WritingGuide: charRaw.Identity.WritingGuide,
	}
	for _, g := range charRaw.Goals.Primary {
		char.Goals = append(char.Goals, core.Goal{ID: g.ID, Priority: g.Priority, Type: "primary", Target: g.Target, Condition: g.Condition})
	}
	for _, g := range charRaw.Goals.Secondary {
		char.Goals = append(char.Goals, core.Goal{ID: g.ID, Priority: g.Priority, Type: "secondary", Target: g.Target, Condition: g.Condition})
	}
	for _, g := range charRaw.Goals.Hidden {
		char.Goals = append(char.Goals, core.Goal{ID: g.ID, Priority: g.Priority, Type: "hidden", KnownBy: g.KnownBy, RevealCondition: g.RevealCondition})
	}
	return char
}

// loadedWorld mirrors importer.WorldYAML but kept in main to avoid import cycle.
type loadedWorld struct {
	Name      string `yaml:"name"`
	CoreRules string `yaml:"core_rules"`
	Scene     struct {
		Location    string   `yaml:"location"`
		TimeOfDay   string   `yaml:"time_of_day"`
		Weather     string   `yaml:"weather"`
		Characters  []string `yaml:"characters"`
		Description string   `yaml:"description"`
	} `yaml:"scene"`
	Ontology struct {
		Characters []ontologyEntry  `yaml:"characters"`
		Locations  []ontologyEntry  `yaml:"locations"`
		Factions   []ontologyEntry  `yaml:"factions"`
		Items      []ontologyEntry  `yaml:"items"`
		Lore       []ontologyEntry  `yaml:"lore"`
		Events     []ontologyEvent  `yaml:"events"`
		Timelines  []ontologyEntry  `yaml:"timelines"`
			Settings   []ontologyEntry  `yaml:"settings"`
	} `yaml:"ontology"`
}

type ontologyEntry struct {
	Name    string `yaml:"name"`
	Keys    string `yaml:"keys"`
	Content string `yaml:"content"`
}

type ontologyEvent struct {
	Name    string `yaml:"name"`
	Arc     string `yaml:"arc"`
	Keys    string `yaml:"keys"`
	Content string `yaml:"content"`
}

// loadWorldDir reads the three-layer world directory structure.
func loadWorldDir(dir string) loadedWorld {
	var w loadedWorld

	// world.yml
	if data, err := os.ReadFile(filepath.Join(dir, "world.yml")); err == nil {
		var worldData struct {
			Name      string `yaml:"name"`
			CoreRules string `yaml:"core_rules"`
		}
		yaml.Unmarshal(data, &worldData)
		w.Name = worldData.Name
		w.CoreRules = worldData.CoreRules
	}

	// scene.yml
	if data, err := os.ReadFile(filepath.Join(dir, "scene.yml")); err == nil {
		yaml.Unmarshal(data, &w.Scene)
	}

	// canon/ontology.yml
	if data, err := os.ReadFile(filepath.Join(dir, "canon", "ontology.yml")); err == nil {
		yaml.Unmarshal(data, &w.Ontology)
	}

	return w
}

func loadWorld(path string) loadedWorld {
	worldData, err := os.ReadFile(path)
	if err != nil {
		log.Fatalf("Failed to read world file: %v", err)
	}
	var world loadedWorld
	if err := yaml.Unmarshal(worldData, &world); err != nil {
		log.Fatalf("Failed to parse world YAML: %v", err)
	}
	return world
}

// seedOntology converts ontology entries into semantic facts + episodic events.
func seedOntology(mem *memory.Engine, world *loadedWorld, charName string) {
	o := world.Ontology
	var facts []core.FactFrame
	var episodics []core.EventFrame

	// Characters: extract relationship lines as structured facts
	for _, c := range o.Characters {
		content := c.Content
		// Extract key identity info as facts
		for _, line := range strings.Split(content, "\n") {
			line = strings.TrimSpace(line)
			if line == "" || len(line) > 300 {
				continue
			}
			// Map lines like "身份: xxx" or "关系: xxx" to facts
			if colon := strings.IndexByte(line, ':'); colon >= 0 && colon < 40 {
				key := strings.TrimSpace(line[:colon])
				val := strings.TrimSpace(line[colon+1:])
				if val != "" && len(val) < 200 {
					facts = append(facts, core.FactFrame{
						Subject:    extractName(c.Name),
						Predicate:  cleanFactKey(key),
						Object:     val,
						Confidence: 1.0,
					})
				}
			}
		}
		// Also store full content as a single fact for keyword matching
		if len(content) > 0 {
			facts = append(facts, core.FactFrame{
				Subject:    extractName(c.Name),
				Predicate:  "完整资料",
				Object:     truncateStr(content, 500),
				Confidence: 1.0,
			})
		}
	}

	// Locations
	for _, e := range o.Locations {
		facts = append(facts, core.FactFrame{
			Subject:    extractName(e.Name),
			Predicate:  "是",
			Object:     truncateStr(e.Content, 300),
			Confidence: 1.0,
		})
	}

	// Factions
	for _, e := range o.Factions {
		facts = append(facts, core.FactFrame{
			Subject:    extractName(e.Name),
			Predicate:  "势力",
			Object:     truncateStr(e.Content, 300),
			Confidence: 1.0,
		})
	}

	// Items
	for _, e := range o.Items {
		facts = append(facts, core.FactFrame{
			Subject:    extractName(e.Name),
			Predicate:  "物品",
			Object:     truncateStr(e.Content, 200),
			Confidence: 1.0,
		})
	}

	// Lore
	for _, e := range o.Lore {
		facts = append(facts, core.FactFrame{
			Subject:    extractName(e.Name),
			Predicate:  "世界观",
			Object:     truncateStr(e.Content, 300),
			Confidence: 1.0,
		})
	}

	// Settings (体系/能力/职业)
	for _, e := range o.Settings {
		facts = append(facts, core.FactFrame{
			Subject:    extractName(e.Name),
			Predicate:  "体系设定",
			Object:     truncateStr(e.Content, 300),
			Confidence: 1.0,
		})
	}

	// Events → episodic memory
	for _, e := range o.Events {
		arc := e.Arc
		if arc == "" {
			arc = "事件"
		}
		episodics = append(episodics, core.EventFrame{
			EventID:         "ont_" + e.Name,
			Type:            arc,
			Description:     truncateStr(e.Content, 400),
			EmotionalWeight: 0.5,
		})
	}

	// Timelines
	for _, e := range o.Timelines {
		facts = append(facts, core.FactFrame{
			Subject:    "时间线",
			Predicate:  extractName(e.Name),
			Object:     truncateStr(e.Content, 300),
			Confidence: 1.0,
		})
	}

	if len(facts) > 0 {
		if err := mem.SeedFacts(facts, charName); err != nil {
			log.Printf("Warning: seed facts failed: %v", err)
		}
	}
	if len(episodics) > 0 {
		if err := mem.SeedEpisodics(episodics, charName); err != nil {
			log.Printf("Warning: seed episodics failed: %v", err)
		}
	}
}

func extractName(raw string) string {
	// Strip "角色·", "物品·", etc. prefix
	s := raw
	if idx := strings.Index(s, "·"); idx >= 0 {
		s = s[idx+len("·"):]
	}
	return strings.TrimSpace(s)
}

func cleanFactKey(key string) string {
	key = strings.TrimSpace(key)
	key = strings.TrimSuffix(key, "：")
	key = strings.TrimSuffix(key, ":")
	return key
}

func truncateStr(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "……"
}

func countOntologyFacts(world *loadedWorld) int {
	n := 0
	n += len(world.Ontology.Characters)*2  // each char produces ~2 facts
	n += len(world.Ontology.Locations)
	n += len(world.Ontology.Factions)
	n += len(world.Ontology.Items)
	n += len(world.Ontology.Lore)
	n += len(world.Ontology.Timelines)
	return n
}

func countOntologyEvents(world *loadedWorld) int {
	return len(world.Ontology.Events)
}
