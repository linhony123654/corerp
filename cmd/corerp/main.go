package main

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"

	"corerp/internal/agents"
	"corerp/internal/api"
	"corerp/internal/auth"
	"corerp/internal/character"
	"corerp/internal/core"
	"corerp/internal/events"
	"corerp/internal/importer"
	"corerp/internal/llm"
	"corerp/internal/memory"
	"corerp/internal/runtime"
	"corerp/internal/world"

	"gopkg.in/yaml.v3"
)

type apiInstanceResolver struct {
	manager *runtime.Manager
}

var _ api.InstanceResolver = (*apiInstanceResolver)(nil)

var (
	buildVersion = "dev"
	buildCommit  = "unknown"
	buildTime    = "unknown"
)

type buildMetadata struct {
	Version string
	Commit  string
	Time    string
}

func (r apiInstanceResolver) DefaultInstanceID() string {
	return r.manager.DefaultID()
}

func (r apiInstanceResolver) ResolveInstance(id string) (api.RuntimeEngine, error) {
	return r.manager.Resolve(id)
}

func (r apiInstanceResolver) ListInstances() []core.RuntimeInstanceSummary {
	return r.manager.List()
}

func (r apiInstanceResolver) InstanceStatus(id string) (core.RuntimeInstanceSummary, error) {
	return r.manager.Status(id)
}

func (r apiInstanceResolver) SetDefaultInstance(id string) error {
	return r.manager.SetDefault(id)
}

func (r apiInstanceResolver) StopInstance(id string) (core.RuntimeInstanceSummary, error) {
	return r.manager.Stop(id)
}

func (r apiInstanceResolver) DeleteInstance(id string) error {
	return r.manager.Delete(id)
}

func (r apiInstanceResolver) CreateInstance(sourceID, id, label, focusCharacter string) (core.RuntimeInstanceSummary, error) {
	return r.manager.CreateFrom(sourceID, id, label, focusCharacter)
}

func resolveBuildMetadata() buildMetadata {
	meta := buildMetadata{
		Version: strings.TrimSpace(buildVersion),
		Commit:  strings.TrimSpace(buildCommit),
		Time:    strings.TrimSpace(buildTime),
	}
	if meta.Version == "" {
		meta.Version = "dev"
	}
	if meta.Commit == "" {
		meta.Commit = "unknown"
	}
	if meta.Time == "" {
		meta.Time = "unknown"
	}

	info, ok := debug.ReadBuildInfo()
	if !ok {
		return meta
	}
	if meta.Version == "dev" && info.Main.Version != "" && info.Main.Version != "(devel)" {
		meta.Version = info.Main.Version
	}

	settings := map[string]string{}
	for _, setting := range info.Settings {
		settings[setting.Key] = setting.Value
	}

	if meta.Commit == "unknown" && settings["vcs.revision"] != "" {
		meta.Commit = settings["vcs.revision"]
	}
	if meta.Time == "unknown" && settings["vcs.time"] != "" {
		meta.Time = settings["vcs.time"]
	}
	if settings["vcs.modified"] == "true" && !strings.Contains(meta.Version, "+dirty") {
		meta.Version += "+dirty"
	}

	return meta
}

func shortCommit(commit string) string {
	if len(commit) > 12 {
		return commit[:12]
	}
	return commit
}

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
	mode := fs.String("mode", "auto", "Import mode: auto | single | ensemble")
	interactive := fs.Bool("interactive", false, "Preview import result and confirm/override mode interactively")
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
		results, err := importer.ImportDirectoryWithMode(*src, *dst, *mode)
		if err != nil {
			log.Fatalf("Import failed: %v", err)
		}
		for _, r := range results {
			fmt.Println(r)
		}
		return
	}

	finalMode := *mode
	if *interactive {
		finalMode = previewAndChooseImportMode(*src, finalMode)
	}

	if strings.HasSuffix(strings.ToLower(*src), ".json") {
		charPath, worldPath, err := importer.ImportJSONWithMode(*src, *dst, finalMode)
		if err != nil {
			log.Fatalf("Import failed: %v", err)
		}
		fmt.Printf("Imported character: %s\n", charPath)
		fmt.Printf("Imported world:     %s\n", worldPath)
	} else {
		charPath, worldPath, err := importer.ImportPNGWithMode(*src, *dst, finalMode)
		if err != nil {
			log.Fatalf("Import failed: %v", err)
		}
		fmt.Printf("Imported character: %s\n", charPath)
		fmt.Printf("Imported world:     %s\n", worldPath)
	}
}

func previewAndChooseImportMode(srcPath, currentMode string) string {
	st, err := loadSillyTavernCardForPreview(srcPath)
	if err != nil {
		log.Printf("Preview skipped: %v", err)
		return currentMode
	}

	preview := importer.PreviewBundle(st, currentMode)
	fmt.Printf("Auto-detected mode: %s\n", preview.Kind)
	fmt.Printf("World: %s\n", preview.WorldName)
	fmt.Printf("Primary character: %s\n", preview.PrimaryCharacter)
	fmt.Printf("Generated characters (%d): %s\n", len(preview.CharacterNames), strings.Join(preview.CharacterNames, ", "))
	fmt.Print("Accept? [Y/n], or type 'single' / 'ensemble': ")

	return chooseImportModeFromReader(bufio.NewReader(os.Stdin), currentMode)
}

func chooseImportModeFromReader(reader *bufio.Reader, currentMode string) string {
	line, err := reader.ReadString('\n')
	if err != nil {
		return currentMode
	}
	choice := strings.ToLower(strings.TrimSpace(line))
	switch choice {
	case "", "y", "yes":
		return currentMode
	case "single", "ensemble":
		return choice
	default:
		return currentMode
	}
}

func loadSillyTavernCardForPreview(srcPath string) (importer.SillyTavernChar, error) {
	if strings.HasSuffix(strings.ToLower(srcPath), ".json") {
		data, err := os.ReadFile(srcPath)
		if err != nil {
			return importer.SillyTavernChar{}, err
		}
		var st importer.SillyTavernChar
		if err := json.Unmarshal(data, &st); err != nil {
			return importer.SillyTavernChar{}, err
		}
		return st, nil
	}

	f, err := os.Open(srcPath)
	if err != nil {
		return importer.SillyTavernChar{}, err
	}
	defer f.Close()

	charaB64, err := importer.ExtractPreviewCharaChunk(f)
	if err != nil {
		return importer.SillyTavernChar{}, err
	}
	jsonBytes, err := base64.StdEncoding.DecodeString(charaB64)
	if err != nil {
		return importer.SillyTavernChar{}, err
	}
	var st importer.SillyTavernChar
	if err := json.Unmarshal(jsonBytes, &st); err != nil {
		return importer.SillyTavernChar{}, err
	}
	return st, nil
}

// === serve subcommand ===

func runServe(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	port := fs.String("port", "8080", "HTTP server port")
	dataDir := fs.String("data", "./data", "Data directory")
	bootMode := fs.String("boot", "auto", "Bootstrap mode: auto|character|world")
	charFile := fs.String("character", "characters/anya.yml", "Single character YAML (use -characters dir for multi)")
	charDir := fs.String("characters", "", "Directory of character YAML files (loads all *.yml)")
	worldFile := fs.String("world", "worlds/cyberpunk2077/world.yml", "World YAML file or world directory")
	llmURL := fs.String("llm-url", os.Getenv("LLM_URL"), "LLM API endpoint")
	llmKey := fs.String("llm-key", os.Getenv("LLM_API_KEY"), "LLM API key")
	llmModel := fs.String("llm-model", os.Getenv("LLM_MODEL"), "LLM model name")
	summaryURL := fs.String("llm-summary-url", "", "Optional separate LLM endpoint for summaries (defaults to main)")
	summaryModel := fs.String("llm-summary-model", "", "Optional separate model for summaries (defaults to main)")
	authKey := fs.String("auth-key", os.Getenv("CORERP_AUTH_KEY"), "Access password (empty = no auth)")
	secureCookie := fs.Bool("secure-cookie", true, "Set Secure flag on session cookie (disable for localhost dev)")
	fs.Parse(args)

	if *llmURL == "" {
		*llmURL = "http://localhost:11434/v1"
	}
	if *llmModel == "" {
		*llmModel = "qwen2.5:7b"
	}
	if strings.TrimSpace(*worldFile) != "" {
		*worldFile = filepath.Clean(strings.TrimSpace(*worldFile))
	}

	buildMeta := resolveBuildMetadata()
	api.BuildVersion = buildMeta.Version
	api.BuildCommit = buildMeta.Commit
	api.BuildTime = buildMeta.Time

	os.MkdirAll(*dataDir, 0755)
	log.Printf(
		"CoreRP booting version=%s commit=%s build_time=%s data=%s port=%s",
		buildMeta.Version,
		shortCommit(buildMeta.Commit),
		buildMeta.Time,
		*dataDir,
		*port,
	)

	mode := normalizeServeBootMode(*bootMode)
	if mode == "" {
		log.Fatalf("Invalid -boot mode %q (want auto|character|world)", *bootMode)
	}

	// Load characters: directory mode takes precedence when booting from character cards.
	var chars []core.Character
	var charNames []string
	var charPaths []string
	if mode != "world" {
		if *charDir != "" {
			chars, charNames, charPaths = loadCharactersFromDir(*charDir)
		} else if _, err := os.Stat(*charFile); err == nil {
			char := loadCharacter(*charFile)
			chars = []core.Character{char}
			charNames = []string{char.Identity.Name}
			charPaths = []string{*charFile}
		}
	}
	mode = resolveServeBootMode(mode, *worldFile, len(charNames) > 0)
	if mode == "character" && len(charNames) == 0 {
		log.Fatalf("No character YAML files available for character boot")
	}
	activeName := ""
	if len(charNames) > 0 {
		activeName = charNames[0]
	}
	charPathMap := make(map[string]string, len(charNames))

	// Load per-character worlds for compatibility boot mode.
	charWorlds := make(map[string]runtime.CharWorld)
	worldPathMap := make(map[string]string, len(charNames))
	if mode == "character" {
		for i, name := range charNames {
			charPathMap[name] = charPaths[i]
			var wf string
			if *charDir != "" {
				wf = findWorldFile(charPaths[i])
			}
			if wf == "" {
				wf = *worldFile
			}
			worldPathMap[name] = wf
			bundle, err := world.LoadBundle(wf)
			if err != nil {
				log.Fatalf("Failed to load world '%s': %v", wf, err)
			}
			defaultScene := core.SceneState{}
			if len(bundle.Scenes) > 0 {
				defaultScene = bundle.Scenes[0].Scene
				for _, scene := range bundle.Scenes {
					if scene.Name == "default" {
						defaultScene = scene.Scene
						break
					}
				}
			}
			charWorlds[name] = runtime.CharWorld{
				WorldName: bundle.Config.Name,
				CoreRules: bundle.Config.CoreRules,
				Scene:     defaultScene,
			}
			log.Printf("Character '%s' → world '%s' (%s)", name, bundle.Config.Name, wf)
		}
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

	// Seed ontology per character from their own world in compatibility mode.
	if mode == "character" {
		for i, name := range charNames {
			var wf string
			if *charDir != "" {
				wf = findWorldFile(charPaths[i])
			}
			if wf == "" {
				wf = *worldFile
			}
			bundle, err := world.LoadBundle(wf)
			if err != nil {
				log.Fatalf("Failed to load world '%s': %v", wf, err)
			}
			if err := world.SeedMemory(memEngine, bundle, name); err != nil {
				log.Printf("Warning: world seed failed for '%s': %v", name, err)
			}
			log.Printf("Ontology seeded: %s into '%s'", world.SeedSummary(bundle), name)
		}
	}

	// Init agents — preload cards only in compatibility mode.
	agentsMgr := agents.NewEnvelopeManager()
	if mode == "character" {
		for i, c := range chars {
			agentsMgr.LoadCharacter(charNames[i], c)
			log.Printf("Loaded character: %s", charNames[i])
		}
	}

	// Init auth
	if *authKey != "" {
		auth.Init(*authKey)
	} else {
		auth.Init("admin") // default password
	}
	auth.SetSecureCookie(*secureCookie)
	log.Printf("Auth: enabled (访问密码: %s, 修改: POST /api/change-password)", auth.MaskPassword())

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
	engine.ConfigurePersistence(*dataDir, charPathMap, worldPathMap)

	// Load existing state from events
	if err := engine.LoadState(); err != nil {
		log.Printf("Warning: failed to load state: %v", err)
	}
	if err := gatekeeper.Causality().RebuildAll(); err != nil {
		log.Printf("Warning: failed to rebuild causality: %v", err)
	} else {
		log.Printf("Causality rebuilt")
	}

	switch mode {
	case "character":
		// Seed initial scene only when no prior scene projection exists.
		initWorld := charWorlds[activeName]
		if sceneIsEmpty(engine.GetState().Scene) {
			engine.SeedScene(core.SceneState{
				Location:    initWorld.Scene.Location,
				TimeOfDay:   initWorld.Scene.TimeOfDay,
				Weather:     initWorld.Scene.Weather,
				Characters:  initWorld.Scene.Characters,
				Description: initWorld.Scene.Description,
			})
		}
		engine.SyncActiveWorldContext()

		// Auto-seed NPC desires from character cards (idempotent)
		engine.SeedNPCDesires()
	case "world":
		preset, err := engine.EnterWorld(*worldFile)
		if err != nil {
			log.Fatalf("Failed to enter world '%s': %v", *worldFile, err)
		}
		if name := engine.GetFocusCharacter(); strings.TrimSpace(name) != "" {
			if bundle, err := world.LoadBundle(*worldFile); err == nil {
				if err := world.SeedMemory(memEngine, bundle, name); err != nil {
					log.Printf("Warning: world seed failed for '%s': %v", name, err)
				} else {
					log.Printf("Ontology seeded: %s into '%s'", world.SeedSummary(bundle), name)
				}
			}
		}
		log.Printf("World-first boot entered '%s' via preset '%s' as '%s'", engine.GetWorldName(), preset.Name, engine.GetFocusCharacter())
	}

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

	log.Printf("CoreRP started version=%s commit=%s", buildMeta.Version, shortCommit(buildMeta.Commit))
	log.Printf("Boot mode: %s", mode)
	log.Printf("Focus character: %s", engine.GetFocusCharacter())
	log.Printf("World: %s", engine.GetWorldName())
	log.Printf("LLM: %s @ %s", *llmModel, *llmURL)
	log.Printf("Listening on http://localhost:%s", *port)

	// Setup HTTP
	mux := http.NewServeMux()
	instanceManager := runtime.NewManager()
	if err := instanceManager.Register("default", "Primary Runtime", engine, true); err != nil {
		log.Fatalf("Failed to register runtime instance: %v", err)
	}
	server := api.NewServer(engine, apiInstanceResolver{manager: instanceManager})
	server.SetProofAuditRoot(filepath.Join(*dataDir, "proof-audits"))
	server.Register(mux)

	if err := http.ListenAndServe(":"+*port, mux); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

func findWorldFile(charPath string) string {
	if wf := readCharacterWorldPath(charPath); wf != "" {
		if info, err := os.Stat(wf); err == nil {
			if info.IsDir() {
				return wf
			}
			return wf
		}
	}

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

func readCharacterWorldPath(charPath string) string {
	data, err := os.ReadFile(charPath)
	if err != nil {
		return ""
	}
	var raw struct {
		WorldPath string `yaml:"world_path"`
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return ""
	}
	return strings.TrimSpace(raw.WorldPath)
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

func normalizeServeBootMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "auto":
		return "auto"
	case "character":
		return "character"
	case "world":
		return "world"
	default:
		return ""
	}
}

func resolveServeBootMode(mode, worldPath string, hasCharacters bool) string {
	mode = normalizeServeBootMode(mode)
	if mode == "" {
		return ""
	}
	if mode != "auto" {
		return mode
	}
	if hasCharacters {
		return "character"
	}
	if strings.TrimSpace(worldPath) != "" {
		return "world"
	}
	return "character"
}

func sceneIsEmpty(scene core.SceneState) bool {
	return scene.Location == "" &&
		scene.TimeOfDay == "" &&
		scene.Weather == "" &&
		scene.Description == "" &&
		len(scene.Characters) == 0
}

func loadCharacter(path string) core.Character {
	char, err := character.Load(path)
	if err != nil {
		log.Fatalf("Failed to load character YAML: %v", err)
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
		Characters []ontologyEntry `yaml:"characters"`
		Locations  []ontologyEntry `yaml:"locations"`
		Factions   []ontologyEntry `yaml:"factions"`
		Items      []ontologyEntry `yaml:"items"`
		Lore       []ontologyEntry `yaml:"lore"`
		Events     []ontologyEvent `yaml:"events"`
		Timelines  []ontologyEntry `yaml:"timelines"`
		Settings   []ontologyEntry `yaml:"settings"`
	} `yaml:"ontology"`
	DirectFacts []FactEntry
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

	// world.yml — meta + core_rules
	if data, err := os.ReadFile(filepath.Join(dir, "world.yml")); err == nil {
		var worldData struct {
			Meta struct {
				Name string `yaml:"name"`
			} `yaml:"meta"`
			CoreRules string `yaml:"core_rules"`
		}
		yaml.Unmarshal(data, &worldData)
		w.Name = worldData.Meta.Name
		if w.Name == "" {
			// Fallback: old format without meta section
			var oldFormat struct {
				Name      string `yaml:"name"`
				CoreRules string `yaml:"core_rules"`
			}
			yaml.Unmarshal(data, &oldFormat)
			w.Name = oldFormat.Name
			w.CoreRules = oldFormat.CoreRules
		} else {
			w.CoreRules = worldData.CoreRules
		}
	}

	// scenes/default.yml
	if data, err := os.ReadFile(filepath.Join(dir, "scenes", "default.yml")); err == nil {
		var sceneDoc struct {
			Scene SceneYAMLDir `yaml:"scene"`
		}
		yaml.Unmarshal(data, &sceneDoc)
		w.Scene.Location = sceneDoc.Scene.Location
		w.Scene.TimeOfDay = sceneDoc.Scene.TimeOfDay
		w.Scene.Weather = sceneDoc.Scene.Weather
		w.Scene.Characters = sceneDoc.Scene.PresentChars
		w.Scene.Description = sceneDoc.Scene.Atmosphere
	}
	// Fallback: old scene.yml at top level
	if w.Scene.Location == "" {
		if data, err := os.ReadFile(filepath.Join(dir, "scene.yml")); err == nil {
			yaml.Unmarshal(data, &w.Scene)
		}
	}

	// canon/ontology.yml
	if data, err := os.ReadFile(filepath.Join(dir, "canon", "ontology.yml")); err == nil {
		var ontoDoc struct {
			Ontology struct {
				Characters []ontologyEntry `yaml:"characters"`
				Locations  []ontologyEntry `yaml:"locations"`
				Factions   []ontologyEntry `yaml:"factions"`
				Items      []ontologyEntry `yaml:"items"`
				Lore       []ontologyEntry `yaml:"lore"`
				Events     []ontologyEvent `yaml:"events"`
				Timelines  []ontologyEntry `yaml:"timelines"`
				Settings   []ontologyEntry `yaml:"settings"`
			} `yaml:"ontology"`
		}
		yaml.Unmarshal(data, &ontoDoc)
		w.Ontology.Characters = ontoDoc.Ontology.Characters
		w.Ontology.Locations = ontoDoc.Ontology.Locations
		w.Ontology.Factions = ontoDoc.Ontology.Factions
		w.Ontology.Items = ontoDoc.Ontology.Items
		w.Ontology.Lore = ontoDoc.Ontology.Lore
		w.Ontology.Events = ontoDoc.Ontology.Events
		w.Ontology.Timelines = ontoDoc.Ontology.Timelines
		w.Ontology.Settings = ontoDoc.Ontology.Settings
	}

	// canon/facts.yml → DirectFacts
	if data, err := os.ReadFile(filepath.Join(dir, "canon", "facts.yml")); err == nil {
		var factsDoc struct {
			Facts []FactEntry `yaml:"facts"`
		}
		yaml.Unmarshal(data, &factsDoc)
		w.DirectFacts = factsDoc.Facts
	}

	return w
}

// SceneYAMLDir mirrors the scenes/default.yml structure.
type SceneYAMLDir struct {
	Location     string   `yaml:"location"`
	TimeOfDay    string   `yaml:"time_of_day"`
	Weather      string   `yaml:"weather"`
	Atmosphere   string   `yaml:"atmosphere"`
	PresentChars []string `yaml:"present_chars"`
	Tension      float64  `yaml:"tension"`
}

// FactEntry mirrors importer.FactEntry for direct fact loading.
type FactEntry struct {
	Subject    string  `yaml:"subject"`
	Predicate  string  `yaml:"predicate"`
	Object     string  `yaml:"object"`
	Confidence float64 `yaml:"confidence"`
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

	// Direct facts from canon/facts.yml (no extraction needed)
	for _, f := range world.DirectFacts {
		facts = append(facts, core.FactFrame{
			Subject:    f.Subject,
			Predicate:  f.Predicate,
			Object:     f.Object,
			Confidence: f.Confidence,
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
	n += len(world.Ontology.Characters) * 2 // each char produces ~2 facts
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
