package runtime

import (
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"corerp/internal/actions"
	"corerp/internal/agents"
	"corerp/internal/context"
	"corerp/internal/core"
	"corerp/internal/emotion"
	"corerp/internal/events"
	"corerp/internal/llm"
	"corerp/internal/memory"
	"corerp/internal/narrative"
	"corerp/internal/simulation"
	"corerp/internal/state"
	"corerp/internal/world"
)

// CharWorld binds a character to their world context.
type CharWorld struct {
	WorldName string
	CoreRules string
	Scene     core.SceneState
}

// Engine orchestrates the narrative loop.
type Engine struct {
	mu              sync.RWMutex
	stateMgr        *state.Manager
	eventStore      *events.Store
	gatekeeper      *events.Gatekeeper
	memEngine       *memory.Engine
	agents          *agents.EnvelopeManager
	compiler        *context.Compiler
	llmRouter       *llm.Router
	executor        *actions.Executor
	tickLoop        *simulation.Loop
	decayEngine     *memory.DecayEngine
	tensionEng      *narrative.TensionEngine
	pulseEng        *simulation.PulseEngine
	factionEng      *simulation.FactionEngine
	stateMachine    *state.StateMachine
	compressEng     *narrative.CompressionEngine
	planner         *agents.Planner
	scheduler       *agents.Scheduler
	emotionEng      *emotion.Engine
	desireStore     *emotion.DesireStore
	actionLogger    *emotion.ActionLogger
	actionBudget    *emotion.ActionBudget
	directorCfg     core.DirectorConfig
	lastPlan        core.DirectorPlan
	turnTraces      []core.TurnTrace
	lastTickSummary []string
	tickHistory     []core.TickSnapshot

	// Config
	instanceID       string
	instanceCreated  time.Time
	focusCharacter   string
	activeWorldPath  string
	loadedCharacters []string
	sceneShells      map[string]bool
	charWorlds       map[string]CharWorld // per-character world context
	charPaths        map[string]string
	worldPaths       map[string]string
	dataDir          string
	worldName        string
	coreRules        string
	sessionID        string
	playerRole       core.PlayerRole

	// Working state
	dialogueHistory []core.Message
	turnCount       int
	tickCount       atomic.Int64

	npcTickExposure map[string]int
}

func New(
	eventStore *events.Store,
	gatekeeper *events.Gatekeeper,
	memEngine *memory.Engine,
	decayEngine *memory.DecayEngine,
	agentsMgr *agents.EnvelopeManager,
	llmRouter *llm.Router,
	activeChar string, loadedChars []string,
	charWorlds map[string]CharWorld,
) (*Engine, error) {
	cw := charWorlds[activeChar]
	emotionEng, err := emotion.NewWithDB(memEngine.DB())
	if err != nil {
		return nil, err
	}
	actionLogger := emotion.NewActionLogger(200)
	if err := actionLogger.EnablePersistence(memEngine.DB()); err != nil {
		return nil, err
	}
	if err := actionLogger.LoadFromDB(200); err != nil {
		return nil, err
	}
	return &Engine{
		stateMgr:         state.New(),
		eventStore:       eventStore,
		gatekeeper:       gatekeeper,
		memEngine:        memEngine,
		decayEngine:      decayEngine,
		tensionEng:       narrative.NewTensionEngine(),
		pulseEng:         simulation.NewPulseEngine(),
		stateMachine:     state.NewStateMachine(),
		compressEng:      narrative.NewCompressionEngine(eventStore),
		planner:          agents.NewPlanner(),
		scheduler:        agents.NewScheduler(),
		agents:           agentsMgr,
		emotionEng:       emotionEng,
		desireStore:      emotion.NewDesireStore(memEngine.DB()),
		actionLogger:     actionLogger,
		actionBudget:     emotion.DefaultBudget(),
		directorCfg:      core.DirectorConfig{Mode: "auto_chain", MaxSpeakers: 2},
		turnTraces:       make([]core.TurnTrace, 0, 32),
		tickHistory:      make([]core.TickSnapshot, 0, 12),
		npcTickExposure:  make(map[string]int),
		factionEng:       simulation.NewFactionEngine(),
		compiler:         context.NewCompiler("budgets.yml"),
		llmRouter:        llmRouter,
		executor:         actions.NewExecutor(),
		instanceID:       fmt.Sprintf("inst_%d", time.Now().UnixNano()),
		instanceCreated:  time.Now().UTC(),
		focusCharacter:   activeChar,
		activeWorldPath:  "",
		loadedCharacters: loadedChars,
		sceneShells:      map[string]bool{},
		charWorlds:       charWorlds,
		worldName:        cw.WorldName,
		coreRules:        cw.CoreRules,
		sessionID:        fmt.Sprintf("sess_%d", time.Now().Unix()),
		playerRole:       defaultPlayerRole(),
	}, nil
}

// LoadState reconstructs WorldState from canonical events and restores dialogue history.
func (e *Engine) LoadState() error {
	eventList, err := e.eventStore.GetCanonicalEvents()
	if err != nil {
		return err
	}
	projected := events.Project(eventList)
	e.stateMgr.UpdateFromProjection(projected)

	// Restore cross-session dialogue history
	e.memEngine.LoadRecentDialogueFromDB(e.GetFocusCharacter(), 15)
	return nil
}

// SyncActiveWorldContext applies the focus character's world metadata and
// scene to the in-memory state without appending a new canonical event.
func (e *Engine) SyncActiveWorldContext() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.syncActiveWorldContextLocked()
}

func (e *Engine) syncActiveWorldContextLocked() {
	e.refreshActiveWorldPathLocked()
	cw, ok := e.charWorlds[e.GetFocusCharacter()]
	if !ok {
		return
	}

	e.worldName = cw.WorldName
	e.coreRules = cw.CoreRules
	if path := e.currentWorldPathLocked(); path != "" {
		if cfg, err := world.LoadDirectorConfig(path); err == nil {
			e.directorCfg = cfg
		}
	}

	state := e.stateMgr.Get()
	state.Scene = normalizeSceneForCharacter(cw.Scene, e.GetFocusCharacter(), e.playerRoleNameLocked())
	e.stateMgr.Set(state)
}

func (e *Engine) currentWorldPathLocked() string {
	if path := strings.TrimSpace(e.activeWorldPath); path != "" {
		return path
	}
	if path := strings.TrimSpace(e.worldPaths[e.GetFocusCharacter()]); path != "" {
		return path
	}
	return ""
}

func (e *Engine) currentWorldPathFor(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	if path := strings.TrimSpace(e.worldPaths[name]); path != "" {
		return path
	}
	return ""
}

func (e *Engine) refreshActiveWorldPathLocked() {
	if path := e.currentWorldPathFor(e.GetFocusCharacter()); path != "" {
		e.activeWorldPath = path
	}
}

func normalizeSceneForCharacter(scene core.SceneState, focusCharacter, playerName string) core.SceneState {
	if focusCharacter == "" {
		return scene
	}
	if strings.TrimSpace(playerName) == "" {
		playerName = "玩家"
	}

	chars := append([]string(nil), scene.Characters...)
	if len(chars) == 0 {
		if focusCharacter == playerName {
			scene.Characters = []string{focusCharacter}
			return scene
		}
		scene.Characters = []string{focusCharacter, playerName}
		return scene
	}

	focusSeen := false
	playerSeen := false
	for i, name := range chars {
		if name == focusCharacter {
			focusSeen = true
			if playerName == focusCharacter {
				playerSeen = true
			}
			continue
		}
		if isPlayerPlaceholder(name) || name == playerName {
			if playerName == focusCharacter {
				chars[i] = focusCharacter
				focusSeen = true
			} else {
				chars[i] = playerName
			}
			playerSeen = true
		}
	}

	if !focusSeen {
		chars = append(chars, focusCharacter)
	}
	if !playerSeen && playerName != focusCharacter {
		chars = append(chars, playerName)
	}
	scene.Characters = dedupeSceneCharacters(chars)
	return scene
}

func defaultPlayerRole() core.PlayerRole {
	return core.PlayerRole{Name: "玩家"}
}

func normalizePlayerRole(role core.PlayerRole) core.PlayerRole {
	role.Name = strings.TrimSpace(role.Name)
	role.Description = strings.TrimSpace(role.Description)
	role.BoundCharacter = strings.TrimSpace(role.BoundCharacter)
	if role.Name == "" {
		if role.BoundCharacter != "" {
			role.Name = role.BoundCharacter
		} else {
			role.Name = "玩家"
		}
	}
	return role
}

func isPlayerPlaceholder(name string) bool {
	switch strings.TrimSpace(name) {
	case "", "用户", "玩家", "player", "Player", "USER":
		return true
	default:
		return false
	}
}

func dedupeSceneCharacters(chars []string) []string {
	var out []string
	seen := make(map[string]bool, len(chars))
	for _, name := range chars {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	return out
}

func (e *Engine) getPlayerRole() core.PlayerRole {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.playerRole
}

func (e *Engine) playerRoleName() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.playerRoleNameLocked()
}

func (e *Engine) playerRoleNameLocked() string {
	return normalizePlayerRole(e.playerRole).Name
}

// ProcessTurn handles one user input and returns a channel of SSE chunks.
func (e *Engine) ProcessTurn(userInput string) (<-chan string, error) {
	ch := make(chan string, 32)

	go func() {
		defer close(ch)

		e.mu.Lock()
		e.turnCount++
		turnNumber := e.turnCount
		worldState := e.stateMgr.Get()
		previousSpeaker := e.GetFocusCharacter()
		plan := e.directTurnLocked(userInput, worldState)
		e.mu.Unlock()

		if len(plan.Steps) == 0 {
			ch <- "[ERROR] director produced no turn steps\n"
			return
		}

		trace := core.TurnTrace{
			Turn:           turnNumber,
			FocusCharacter: e.GetFocusCharacter(),
			UserInput:      userInput,
			DirectorPlan:   plan,
			WorldMetrics: core.WorldMetrics{
				Tension:              worldState.Tension,
				PressureStates:       e.pulseEng.PressureStates(),
				FactionTensions:      e.factionEng.Tensions(),
				NPCExposure:          cloneNPCExposure(e.npcTickExposure),
				PopulationHighlights: e.populationHighlightsLocked(),
			},
			CreatedAt: time.Now().UTC(),
		}
		defer func() {
			e.mu.Lock()
			e.recordTraceLocked(trace)
			e.mu.Unlock()
		}()

		userMsg := core.Message{Role: "user", Content: userInput}
		e.mu.Lock()
		trace.ParticipantDetails = e.sceneParticipantDetailsLocked()
		for _, speaker := range uniqueTurnSpeakers(plan.Steps) {
			e.memEngine.PushDialogue(userMsg, speaker)
		}
		e.dialogueHistory = append(e.dialogueHistory, userMsg)
		leadSpeaker := plan.Steps[0].Speaker
		_ = e.setActiveCharacterLocked(leadSpeaker, true, false)
		worldState = e.stateMgr.Get()
		userEvent := events.BuildEvent("user_message", "user", leadSpeaker,
			map[string]interface{}{"content": userInput})
		userEvent.SessionID = e.sessionID
		userEvent.SceneID = worldState.Scene.Location
		e.gatekeeper.Submit(userEvent, events.SourceUserInput())
		e.mu.Unlock()

		// 1.5 Intercept /roll command (dice check, no LLM)
		if strings.HasPrefix(strings.ToLower(userInput), "/roll") || strings.HasPrefix(strings.ToLower(userInput), "/r ") {
			e.handleRollCommand(userInput, ch)
			return
		}

		if plan.Switched {
			ch <- fmt.Sprintf("[导演切换] %s -> %s\n", previousSpeaker, leadSpeaker)
		}

		var handoff *core.StepHandoff
		for i, step := range plan.Steps {
			stepTrace := e.executeTurnStep(step, userInput, turnNumber, handoff)

			trace.StepTraces = append(trace.StepTraces, stepTrace)
			if i == 0 {
				trace.ActiveGoals = append(trace.ActiveGoals, stepTrace.ActiveGoals...)
				trace.AllowedActions = append(trace.AllowedActions, stepTrace.AllowedActions...)
				trace.Memories = append(trace.Memories, stepTrace.Memories...)
				trace.SemanticFacts = append(trace.SemanticFacts, stepTrace.SemanticFacts...)
				trace.EpisodicEvents = append(trace.EpisodicEvents, stepTrace.EpisodicEvents...)
				trace.WorkingMemory = stepTrace.WorkingMemory
				trace.ActionFrame = stepTrace.ActionFrame
				trace.Validator = stepTrace.Validator
			}
			if stepTrace.Error != "" {
				if trace.Narrative == "" {
					trace.Narrative = fmt.Sprintf("[ERROR] %s", stepTrace.Error)
				}
				ch <- fmt.Sprintf("[ERROR] %s\n", stepTrace.Error)
				return
			}
			if i > 0 {
				ch <- fmt.Sprintf("\n\n[%s]\n", step.Speaker)
			}
			for _, r := range stepTrace.Narrative {
				ch <- string(r)
			}
			if stepTrace.Narrative != "" {
				if trace.Narrative != "" {
					trace.Narrative += "\n\n"
				}
				trace.Narrative += stepTrace.Narrative
			}
			handoff = buildStepHandoff(stepTrace)
		}

		e.mu.Lock()
		_ = e.setActiveCharacterLocked(leadSpeaker, true, false)
		if len(e.dialogueHistory)%15 == 0 {
			for _, speaker := range uniqueTurnSpeakers(plan.Steps) {
				go e.updateWorkingMemoryFor(speaker)
			}
		}
		if turnNumber%5 == 0 {
			go func() {
				if n, err := e.gatekeeper.AutoPromote(); err == nil && n > 0 {
					_ = n
				}
			}()
		}
		e.reconcilePopulationLocked()
		e.mu.Unlock()

		ch <- "\n"
	}()

	return ch, nil
}

func (e *Engine) updateWorkingMemoryFor(character string) {
	dialogue := e.memEngine.GetRecentDialogue(character)
	var dialogueText strings.Builder
	for _, m := range dialogue {
		role := "角色"
		if m.Role == "user" {
			role = e.playerRoleName()
		}
		dialogueText.WriteString(fmt.Sprintf("%s: %s\n", role, m.Content))
	}

	prompt := fmt.Sprintf("请用300字以内总结以下对话的关键信息（场景、人物关系变化、重要决定）：\n\n%s", dialogueText.String())
	summary, err := e.llmRouter.GenerateNonStream(llm.TaskSummary, []core.LLMMessage{
		{Role: "system", Content: "你是一个对话摘要助手。请用简洁的中文总结。"},
		{Role: "user", Content: prompt},
	})
	if err != nil {
		return
	}
	e.memEngine.SetWorkingMemory(character, summary)
}

func (e *Engine) extractAndStoreFactsFor(character, narrative string, turnNumber int) {
	if narrative == "" {
		return
	}
	evt := events.BuildEvent("fact_extracted", character, "",
		map[string]interface{}{"narrative": narrative, "turn": turnNumber})
	e.gatekeeper.Submit(evt, events.SourceLLMExtracted())
}

func (e *Engine) GetState() core.WorldState {
	return e.stateMgr.Get()
}

func (e *Engine) SeedScene(scene core.SceneState) {
	// Write scene_init event to Event Store — always overrides stale state
	scene = normalizeSceneForCharacter(scene, e.GetFocusCharacter(), e.playerRoleName())
	evt := events.BuildEvent("scene_init", "system", "",
		map[string]interface{}{
			"location":    scene.Location,
			"time_of_day": scene.TimeOfDay,
			"weather":     scene.Weather,
			"characters":  scene.Characters,
			"description": scene.Description,
		})
	e.gatekeeper.Submit(evt, events.SourceSystem())
	e.stateMgr.SetScene(scene)
}

func (e *Engine) GetCharacter() (core.Character, bool) {
	return e.agents.GetCharacter(e.GetFocusCharacter())
}

// GetFocusDefinition returns the current focus persona definition.
func (e *Engine) GetFocusDefinition() (core.Character, bool) {
	return e.GetCharacter()
}

// GetFocusCharacter is the primary semantic accessor for the instance viewpoint.
func (e *Engine) GetFocusCharacter() string {
	return e.focusCharacter
}

func (e *Engine) GetPlayerRole() core.PlayerRole {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return normalizePlayerRole(e.playerRole)
}

func (e *Engine) GetLoadedCharacters() []string {
	names := make([]string, len(e.loadedCharacters))
	copy(names, e.loadedCharacters)
	return names
}

func (e *Engine) GetSceneParticipants() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	participants := dedupeSceneCharacters(append([]string(nil), e.stateMgr.Get().Scene.Characters...))
	return participants
}

func (e *Engine) GetSceneParticipantDetails() []core.ParticipantSummary {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.sceneParticipantDetailsLocked()
}

func (e *Engine) participantSummaryLocked(name string, present bool) core.ParticipantSummary {
	detail := core.ParticipantSummary{
		Name:       name,
		WorldPath:  e.currentWorldPathFor(name),
		Loaded:     containsString(e.loadedCharacters, name),
		Switchable: true,
		Present:    present,
		Focus:      name == e.focusCharacter,
	}
	if e.agents != nil {
		if _, ok := e.agents.GetCharacter(name); ok {
			detail.Loaded = true
		}
	}
	playerName := e.playerRoleNameLocked()
	switch {
	case isPlayerPlaceholder(name) || name == playerName:
		detail.Kind = "player"
		detail.Source = "player_role"
		detail.Switchable = false
	case strings.TrimSpace(e.charPaths[name]) != "":
		detail.Kind = "persona"
		detail.Source = "character_definition"
	case e.sceneShells[name]:
		detail.Kind = "persona"
		detail.Source = "scene_shell"
	case detail.Loaded:
		detail.Kind = "persona"
		detail.Source = "character_definition"
	default:
		detail.Kind = "npc"
		detail.Source = "scene_presence"
	}
	return detail
}

func (e *Engine) sceneParticipantDetailsLocked() []core.ParticipantSummary {
	sceneParticipants := dedupeSceneCharacters(append([]string(nil), e.stateMgr.Get().Scene.Characters...))
	names := append([]string(nil), sceneParticipants...)

	background := map[string]bool{}
	promoted := map[string]bool{}
	if path := strings.TrimSpace(e.currentWorldPathLocked()); path != "" {
		if cfg, _, err := world.EnsureSeededPopulation(path); err == nil {
			for _, npc := range cfg.BackgroundNPCs {
				background[npc.Name] = true
			}
			for _, npc := range cfg.PromotedNPCs {
				promoted[npc.Name] = true
			}
		}
	}

	playerName := e.playerRoleNameLocked()
	details := make([]core.ParticipantSummary, 0, len(names))
	for _, name := range names {
		detail := e.participantSummaryLocked(name, containsString(sceneParticipants, name))
		switch {
		case isPlayerPlaceholder(name) || name == playerName:
			detail.Kind = "player"
			detail.Source = "player_role"
			detail.Switchable = false
		case promoted[name]:
			detail.Kind = "persona"
			detail.Source = "promoted_population"
		case background[name]:
			detail.Kind = "npc"
			detail.Source = "background_population"
		}
		details = append(details, detail)
	}
	return details
}

func (e *Engine) SetInstanceMetadata(id string, createdAt time.Time) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if strings.TrimSpace(id) != "" {
		e.instanceID = strings.TrimSpace(id)
	}
	if !createdAt.IsZero() {
		e.instanceCreated = createdAt.UTC()
	}
	if e.eventStore != nil {
		e.eventStore.SetInstanceID(e.instanceID)
	}
	if e.memEngine != nil {
		e.memEngine.SetInstanceID(e.instanceID)
	}
	if e.decayEngine != nil {
		e.decayEngine.SetInstanceID(e.instanceID)
	}
	if e.actionLogger != nil {
		e.actionLogger.SetInstanceID(e.instanceID)
		_ = e.actionLogger.LoadFromDB(200)
	}
	if e.dataDir != "" {
		if dir, err := e.instanceDataDirLocked(); err == nil {
			_ = os.MkdirAll(dir, 0755)
		}
		if role, err := e.readPlayerRoleLocked(); err == nil {
			e.playerRole = role
		}
	}
}

func (e *Engine) GetInstanceID() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.instanceID
}

func (e *Engine) InstanceSummary() core.RuntimeInstanceSummary {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return normalizeRuntimeInstanceSummaryCompatibility(core.RuntimeInstanceSummary{
		ID:                 e.instanceID,
		Label:              e.worldName,
		WorldName:          e.worldName,
		FocusCharacter:     e.GetFocusCharacter(),
		Participants:       dedupeSceneCharacters(append([]string(nil), e.stateMgr.Get().Scene.Characters...)),
		ParticipantDetails: e.sceneParticipantDetailsLocked(),
		CreatedAt:          e.instanceCreated,
		Status:             InstanceStatusRunning,
	})
}

// SwitchFocusCharacter changes the focus character. It saves working memory for
// the old viewpoint and loads dialogue history + world context for the new one.
func (e *Engine) SwitchFocusCharacter(name string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	return e.setActiveCharacterLocked(name, true, true)
}

func (e *Engine) switchCharacterLocked(name string, syncWorld bool) error {
	return e.setActiveCharacterLocked(name, syncWorld, true)
}

func (e *Engine) ensureSwitchableCharacterLocked(name string) error {
	if _, ok := e.agents.GetCharacter(name); ok {
		return nil
	}

	scene := e.stateMgr.Get().Scene
	if containsString(scene.Characters, name) {
		if err := e.ensureWorldCharacterLocked(name, scene); err == nil {
			return nil
		}
		e.ensureSceneParticipantLoadedLocked(name, scene)
		return nil
	}

	return fmt.Errorf("character '%s' not loaded", name)
}

func (e *Engine) ensureSceneParticipantLoadedLocked(name string, scene core.SceneState) {
	path := e.currentWorldPathLocked()
	activeWorld := e.charWorlds[e.GetFocusCharacter()]
	if e.sceneShells == nil {
		e.sceneShells = map[string]bool{}
	}
	if _, ok := e.agents.GetCharacter(name); !ok {
		e.agents.LoadCharacter(name, core.Character{
			WorldPath: path,
			Identity: core.IdentityEnvelope{
				Name:         name,
				Adaptive:     map[string]float64{"trust": 3, "fear": 2},
				Voice:        core.VoiceConfig{},
				WritingGuide: "scene participant shell",
			},
		})
	}
	if !containsString(e.loadedCharacters, name) {
		e.loadedCharacters = append(e.loadedCharacters, name)
	}
	if e.worldPaths == nil {
		e.worldPaths = map[string]string{}
	}
	if path != "" {
		e.worldPaths[name] = path
	}
	if e.charWorlds == nil {
		e.charWorlds = map[string]CharWorld{}
	}
	e.charWorlds[name] = CharWorld{
		WorldName: activeWorld.WorldName,
		CoreRules: activeWorld.CoreRules,
		Scene:     normalizeSceneForCharacter(scene, name, e.playerRoleNameLocked()),
	}
	e.sceneShells[name] = true
}

func (e *Engine) setActiveCharacterLocked(name string, syncWorld, resetTurn bool) error {
	if err := e.ensureSwitchableCharacterLocked(name); err != nil {
		return err
	}

	if name == e.focusCharacter {
		if syncWorld {
			e.syncActiveWorldContextLocked()
		}
		return nil
	}

	if resetTurn {
		e.dialogueHistory = nil
		e.compiler.SetMode("full_load")
	}
	e.focusCharacter = name
	e.refreshActiveWorldPathLocked()
	e.memEngine.LoadRecentDialogueFromDB(e.focusCharacter, 15)

	if syncWorld {
		e.syncActiveWorldContextLocked()
	}

	return nil
}

func (e *Engine) handleRollCommand(input string, ch chan<- string) {
	// Parse: /roll <expr> [difficulty]
	// Examples: /roll trust, /roll 2d6+trust, /r d20-2 15
	input = strings.TrimSpace(input)
	if strings.HasPrefix(input, "/r ") {
		input = "/roll" + input[2:]
	}
	parts := strings.Fields(input) // ["/roll", "2d6+trust", "15"]
	if len(parts) < 2 {
		ch <- "[用法] /roll <表达式> [难度]\n  例: /roll trust  /roll 2d6+trust  /roll d20-1 15\n"
		return
	}

	expr := parts[1]
	difficulty := 0
	if len(parts) >= 3 {
		fmt.Sscanf(parts[2], "%d", &difficulty)
	}

	// Build stat function from character's adaptive values
	char, _ := e.agents.GetCharacter(e.GetFocusCharacter())
	statFn := func(key string) int {
		if v, ok := char.Identity.Adaptive[key]; ok {
			return actions.StatToModifier(v)
		}
		return 0
	}

	result, err := actions.RollDice(expr, statFn, difficulty)
	if err != nil {
		ch <- fmt.Sprintf("[骰子错误] %v\n", err)
		return
	}

	// Format output
	ch <- "╔══ 判定 ══╗\n"
	ch <- fmt.Sprintf("║ %s\n", result.Summary)
	if result.Difficulty > 0 && result.Success != nil {
		if *result.Success {
			ch <- "║ ✓ 成功 — 行动达成预期效果\n"
		} else {
			ch <- "║ ✗ 失败 — 行动受阻或产生代价\n"
		}
	}
	ch <- "╚══════════╝\n"

	// Record roll as event for narrative context
	rollEvent := events.BuildEvent("dice_roll", e.GetFocusCharacter(), "",
		map[string]interface{}{
			"expression": result.Expression,
			"total":      result.Total,
			"difficulty": result.Difficulty,
			"summary":    result.Summary,
		})
	rollEvent.SessionID = e.sessionID
	rollEvent.SceneID = e.stateMgr.Get().Scene.Location
	e.gatekeeper.Submit(rollEvent, events.SourceUserInput())

	// Store in dialogue so LLM sees it next turn
	rollMsg := core.Message{Role: "system", Content: fmt.Sprintf("[判定] %s", result.Summary)}
	e.memEngine.PushDialogue(rollMsg, e.GetFocusCharacter())
}

// GetNPCActions returns recent autonomous actions for a character.
func (e *Engine) GetNPCActions(name string, sinceTick int) []agents.NPCActionLog {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.scheduler.RecentActionsForCharacter(name, sinceTick)
}

// GetCausalityChain returns the causal chain for an event.
func (e *Engine) GetCausalityChain(eventID string, depth int) (interface{}, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.gatekeeper.Causality().GetChain(eventID, depth)
}

// GetCausalitySummary returns a human-readable chain summary.
func (e *Engine) GetCausalitySummary(eventID string, depth int) (string, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.gatekeeper.Causality().GetChainSummary(eventID, depth)
}

// ReplayTo reconstructs world state at a given event.
func (e *Engine) ReplayTo(eventID string) (core.WorldState, error) {
	return e.gatekeeper.Replay().ReplayTo(eventID, "main")
}

// ReplayAtTime reconstructs world state at a given world time.
func (e *Engine) ReplayAtTime(hour, minute, day int) (core.WorldState, error) {
	return e.gatekeeper.Replay().ReplayAtTime(hour, minute, day)
}

// ForkTimeline creates a new timeline branch from an event.
func (e *Engine) ForkTimeline(eventID, branchName string) error {
	return e.gatekeeper.Replay().Fork(eventID, branchName)
}

// GetTimeline returns the timeline for a branch.
func (e *Engine) GetTimeline(branch string, limit int) ([]events.EventTimeline, error) {
	return e.gatekeeper.Replay().GetTimeline(branch, limit)
}

// ListBranches returns all branch names.
func (e *Engine) ListBranches() ([]string, error) {
	return e.gatekeeper.Replay().ListBranches()
}

// CompareBranches compares world state across two branches.
func (e *Engine) CompareBranches(branchA, branchB string, index int) (map[string]interface{}, error) {
	return e.gatekeeper.Replay().CompareStates(branchA, branchB, index)
}

// CompressEvents manually triggers narrative compression.
func (e *Engine) CompressEvents(from, to int) (*narrative.CompressionResult, error) {
	return e.compressEng.CompressRange(from, to)
}

// CompressionStats returns compression engine statistics.
func (e *Engine) CompressionStats() map[string]interface{} {
	return e.compressEng.SummaryStats()
}

// LLMRoutes returns the current LLM routing configuration.
func (e *Engine) LLMRoutes() map[string]interface{} {
	return map[string]interface{}{
		"routes":   e.llmRouter.Routes(),
		"adapters": e.llmRouter.Adapters(),
	}
}

// GetDialogue returns recent dialogue messages (default limit).
func (e *Engine) GetDialogue() []core.Message {
	return e.memEngine.GetRecentDialogue(e.GetFocusCharacter())
}

// GetDialogueLimit returns up to N recent dialogue messages.
func (e *Engine) GetDialogueLimit(limit int) []core.Message {
	e.memEngine.LoadRecentDialogueFromDB(e.GetFocusCharacter(), limit)
	return e.memEngine.GetRecentDialogue(e.GetFocusCharacter())
}

// ResetDialogue clears the current character's dialogue.
func (e *Engine) ResetDialogue() {
	e.memEngine.ResetDialogue(e.GetFocusCharacter())
	e.dialogueHistory = nil
}

func (e *Engine) GetWorldName() string {
	return e.worldName
}

func (e *Engine) GetWorldPaths() map[string]string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make(map[string]string, len(e.worldPaths))
	for name, path := range e.worldPaths {
		out[name] = path
	}
	return out
}

func (e *Engine) UpdatePlayerRole(role core.PlayerRole) (core.PlayerRole, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	role = normalizePlayerRole(role)
	e.playerRole = role
	if err := e.writePlayerRoleLocked(); err != nil {
		return core.PlayerRole{}, err
	}
	state := e.stateMgr.Get()
	state.Scene = normalizeSceneForCharacter(state.Scene, e.GetFocusCharacter(), e.playerRoleNameLocked())
	e.stateMgr.Set(state)
	return e.playerRole, nil
}

// DebugInfo returns internal state for debugging.
func (e *Engine) DebugInfo() map[string]interface{} {
	e.mu.RLock()
	defer e.mu.RUnlock()

	state := e.stateMgr.Get()
	recent := e.memEngine.GetRecentDialogue(e.GetFocusCharacter())
	var dialoguePreview []map[string]string
	for _, m := range recent {
		dialoguePreview = append(dialoguePreview, map[string]string{
			"role":    m.Role,
			"content": m.Content,
		})
	}

	canon, quarantined, _ := e.gatekeeper.Stats()

	return map[string]interface{}{
		"turn_count":         e.turnCount,
		"dialogue_in_memory": len(recent),
		"dialogue_history":   dialoguePreview,
		"world_clock":        state.Clock,
		"scene":              state.Scene,
		"tension":            state.Tension,
		"narrative_state":    e.stateMachine.Current(),
		"canonical_events":   canon,
		"quarantined_events": quarantined,
		"npc_actions":        e.scheduler.RecentActions(0),
		"vector_search":      e.memEngine.CountFacts(e.GetFocusCharacter()) >= 100,
		"director_config":    normalizeDirectorConfig(e.directorCfg),
		"director_plan":      e.lastPlan,
	}
}

// StartTickLoop starts the autonomous simulation tick.
func (e *Engine) StartTickLoop() {
	e.tickLoop = simulation.NewLoop(60 * time.Second)
	e.tickLoop.OnTick(e.onTick)
	e.tickLoop.Start()
}

// Stop halts the tick loop.
func (e *Engine) Stop() {
	if e.tickLoop != nil {
		e.tickLoop.Stop()
	}
}

// TickStatus returns current tick loop state.
func (e *Engine) TickStatus() map[string]interface{} {
	e.mu.RLock()
	defer e.mu.RUnlock()

	status := map[string]interface{}{
		"turn_count":    e.turnCount,
		"tick_interval": "60s",
		"world_ratio":   "5m",
	}
	if e.tickLoop != nil {
		status["running"] = !e.tickLoop.IsPaused()
		status["paused"] = e.tickLoop.IsPaused()
		status["tick_count"] = e.tickLoop.TickCount()
		status["world_advance"] = e.tickLoop.WorldAdvancement().String()
	} else {
		status["running"] = false
		status["paused"] = false
		status["tick_count"] = 0
		status["world_advance"] = "0s"
	}
	if e.pulseEng != nil {
		status["pressure_states"] = e.pulseEng.PressureStates()
	}
	if e.factionEng != nil {
		status["faction_tensions"] = e.factionEng.Tensions()
	}
	if len(e.npcTickExposure) > 0 {
		exposureCopy := make(map[string]int, len(e.npcTickExposure))
		for k, v := range e.npcTickExposure {
			exposureCopy[k] = v
		}
		status["npc_tick_exposure"] = exposureCopy
	}
	if highlights := e.populationHighlightsLocked(); len(highlights) > 0 {
		status["population_highlights"] = highlights
	}
	if len(e.lastTickSummary) > 0 {
		status["last_tick_summary"] = append([]string(nil), e.lastTickSummary...)
	}
	if len(e.tickHistory) > 0 {
		history := make([]core.TickSnapshot, 0, len(e.tickHistory))
		for _, item := range e.tickHistory {
			history = append(history, cloneTickSnapshot(item))
		}
		status["tick_history"] = history
	}

	// Authoring diagnostics: actionable insights for world operators
	state := e.stateMgr.Get()
	status["tension"] = state.Tension
	diagnostics := e.buildTickDiagnosticsLocked(state)
	if len(diagnostics) > 0 {
		status["diagnostics"] = diagnostics
	}
	if summary := e.buildTrajectorySummaryLocked(state); len(summary) > 0 {
		status["trajectory_summary"] = summary
	}

	return status
}

func (e *Engine) structureAuthoringDiagnosticsLocked(state core.WorldState) []map[string]interface{} {
	path := strings.TrimSpace(e.currentWorldPathLocked())
	if path == "" {
		return nil
	}
	structure, err := world.LoadStructure(path)
	if err != nil {
		return nil
	}
	cfg, _, err := world.EnsureSeededPopulation(path)
	if err != nil {
		return nil
	}

	sceneLocation := strings.TrimSpace(state.Scene.Location)
	var diagnostics []map[string]interface{}
	if sceneLocation != "" {
		for _, location := range structure.Locations {
			if strings.TrimSpace(location.Name) != sceneLocation {
				continue
			}
			controller := strings.TrimSpace(location.Controller)
			if controller != "" {
				diagnostics = append(diagnostics, map[string]interface{}{
					"level":   "info",
					"metric":  "scene_control",
					"target":  sceneLocation,
					"message": fmt.Sprintf("当前 scene 位于 '%s' 控制区，director / planner 会更偏向相关 faction 与 NPC", controller),
				})
			}
			break
		}
	}

	activePressureCount := 0
	for _, pressure := range structure.Pressures {
		target := strings.TrimSpace(pressure.Target)
		if target == "" || pressure.Intensity < 0.5 {
			continue
		}
		if target == sceneLocation || target == factionControllingScene(sceneLocation, structure) {
			activePressureCount++
			diagnostics = append(diagnostics, map[string]interface{}{
				"level":   "info",
				"metric":  "active_pressure",
				"target":  pressure.ID,
				"value":   pressure.Intensity,
				"message": fmt.Sprintf("pressure '%s' 正在命中当前 scene/faction，后续 tick 与 director 可能继续放大它的影响", pressure.Name),
			})
		}
	}

	relevantNPCs := make([]string, 0, 3)
	relevantNPCCount := 0
	for _, npc := range cfg.BackgroundNPCs {
		candidate := buildPopulationSceneCandidate(npc, state, structure, "")
		if !populationCandidateRelevant(candidate) {
			continue
		}
		relevantNPCCount++
		if len(relevantNPCs) < 3 {
			relevantNPCs = append(relevantNPCs, npc.Name)
		}
	}
	if relevantNPCCount > 0 {
		message := fmt.Sprintf("%d 个 background NPC 已被 scene/location/pressure 命中，可被 director 拉入候选：%s", relevantNPCCount, strings.Join(relevantNPCs, ", "))
		diagnostics = append(diagnostics, map[string]interface{}{
			"level":   "info",
			"metric":  "scene_population_candidates",
			"value":   relevantNPCCount,
			"message": message,
		})
	} else if activePressureCount > 0 {
		diagnostics = append(diagnostics, map[string]interface{}{
			"level":   "warning",
			"metric":  "scene_population_candidates",
			"value":   0,
			"message": "当前 structure 已对 scene 施加压力，但还没有命中的 background NPC，人口层可能偏空",
		})
	}

	return diagnostics
}

func factionControllingScene(sceneLocation string, structure core.WorldStructureConfig) string {
	sceneLocation = strings.TrimSpace(sceneLocation)
	if sceneLocation == "" {
		return ""
	}
	for _, location := range structure.Locations {
		if strings.TrimSpace(location.Name) != sceneLocation {
			continue
		}
		return strings.TrimSpace(location.Controller)
	}
	return ""
}

// ManualTick forces one tick cycle immediately.
func (e *Engine) ManualTick() {
	e.onTick()
}

// PauseTick suspends the tick loop handlers.
func (e *Engine) PauseTick() {
	if e.tickLoop != nil {
		e.tickLoop.Pause()
	}
}

// ResumeTick resumes the tick loop handlers.
func (e *Engine) ResumeTick() {
	if e.tickLoop != nil {
		e.tickLoop.Resume()
	}
}

func (e *Engine) onTick() {
	e.mu.Lock()
	defer e.mu.Unlock()

	state := e.stateMgr.Get()
	summary := make([]string, 0, 8)
	beforeTension := state.Tension
	beforePressure := map[string]float64{}
	beforeFaction := map[string]float64{}
	if e.pulseEng != nil {
		beforePressure = e.pulseEng.PressureStates()
	}
	if e.factionEng != nil {
		beforeFaction = e.factionEng.Tensions()
	}

	// 1. Advance world clock: 1min real = 5min world
	state.Clock.Minute += 5
	if state.Clock.Minute >= 60 {
		state.Clock.Minute -= 60
		state.Clock.Hour++
	}
	if state.Clock.Hour >= 24 {
		state.Clock.Hour -= 24
		state.Clock.Day++
	}

	// 2. Record clock advance event (canonical)
	clockEvent := events.BuildEvent("clock_advance", "system", "",
		map[string]interface{}{"hour": state.Clock.Hour, "minute": state.Clock.Minute, "day": state.Clock.Day})
	e.gatekeeper.Submit(clockEvent, events.SourceTick())
	summary = append(summary, fmt.Sprintf("world clock -> day %d %02d:%02d", state.Clock.Day, state.Clock.Hour, state.Clock.Minute))

	// 3. Auto-promote quarantined events
	if n, err := e.gatekeeper.AutoPromote(); err != nil {
		log.Printf("[tick] AutoPromote failed: %v", err)
	} else if n > 0 {
		// Promoted n events to canonical
	}

	// 4. Memory & Relationship Decay
	if e.decayEngine != nil {
		var report memory.DecayReport
		state, report = e.decayEngine.Tick(state)
		if report.FactsDeleted > 0 || report.EpisodicPruned > 0 || report.FactsPromoted > 0 {
			log.Printf("[tick] decay: facts_deleted=%d episodic=%d facts_promoted=%d", report.FactsDeleted, report.EpisodicPruned, report.FactsPromoted)
		}
	}

	// 5. Reload state from canonical events to pick up promoted events
	if eventList, err := e.eventStore.GetCanonicalEvents(); err != nil {
		log.Printf("[tick] GetCanonicalEvents failed: %v", err)
	} else {
		projected := events.Project(eventList)
		// Preserve current clock since Project doesn't have clock state persistence yet
		projected.Clock = state.Clock
		projected.Relationships = state.Relationships
		e.stateMgr.UpdateFromProjection(projected)
	}

	// 6. World pulse / pressure injection from world structure
	if e.pulseEng != nil {
		if path := e.currentWorldPathLocked(); path != "" {
			if structure, err := world.LoadStructure(path); err == nil {
				pulseEvents := e.pulseEng.Tick(structure, state, int(e.tickCount.Load()))
				if len(pulseEvents) > 0 {
					summary = append(summary, summarizeTickEvents("pressure", pulseEvents))
				}
				for _, evt := range pulseEvents {
					e.gatekeeper.Submit(evt, events.SourceTick())
					applyTickEvent(&state, evt)
				}
			}
		}
	}

	// 6.5 Faction tension engine
	if e.factionEng != nil {
		if path := e.currentWorldPathLocked(); path != "" {
			if structure, err := world.LoadStructure(path); err == nil {
				factionEvents := e.factionEng.Tick(structure, state, int(e.tickCount.Load()))
				if len(factionEvents) > 0 {
					summary = append(summary, summarizeTickEvents("faction", factionEvents))
				}
				for _, evt := range factionEvents {
					e.gatekeeper.Submit(evt, events.SourceTick())
					applyTickEvent(&state, evt)
				}
			}
		}
	}

	// 7. Tension Engine check
	if e.tensionEng != nil {
		pressureEvents := e.tensionEng.Tick(state, e.turnCount)
		if len(pressureEvents) > 0 {
			summary = append(summary, summarizeTickEvents("narrative", pressureEvents))
		}
		for _, evt := range pressureEvents {
			e.gatekeeper.Submit(evt, events.SourceTick())
			applyTickEvent(&state, evt)
		}
	}

	// 8. Narrative State Machine transition
	if e.stateMachine != nil {
		e.stateMachine.Transition(state.Tension, "tick")
	}

	// 9. Update state manager with all tick changes
	e.stateMgr.Set(state)

	if added := e.syncAutonomousScenePopulationLocked(&state); len(added) > 0 {
		summary = append(summary, fmt.Sprintf("world pulled %d background NPCs into current scene: %s", len(added), strings.Join(added, ", ")))
		e.stateMgr.Set(state)
	}

	// 10. NPC desire-driven autonomous actions for non-focus participants
	e.tickCount.Add(1)
	npcScenes := make(map[string]core.SceneState)
	for _, name := range e.loadedCharacters {
		if cw, ok := e.charWorlds[name]; ok {
			npcScenes[name] = cw.Scene
		}
	}

	// Accumulate tick exposure for present NPCs (drives population growth without user input)
	for _, name := range e.loadedCharacters {
		if name == e.focusCharacter {
			continue
		}
		participant := e.participantSummaryLocked(name, containsString(state.Scene.Characters, name))
		if participant.Present {
			e.npcTickExposure[name]++
		}
	}
	presentNPCs := 0
	for _, name := range e.loadedCharacters {
		if name == e.focusCharacter {
			continue
		}
		if containsString(state.Scene.Characters, name) {
			presentNPCs++
		}
	}
	if presentNPCs > 0 {
		summary = append(summary, fmt.Sprintf("%d present NPCs accumulated exposure", presentNPCs))
	}

	// For each non-focus NPC, attempt desire-driven autonomous action
	autonomousActions := 0
	for _, name := range e.loadedCharacters {
		if name == e.focusCharacter {
			continue
		}
		// Compute emotional state
		char, ok := e.agents.GetCharacter(name)
		if !ok {
			continue
		}
		vec := emotionVectorFromAdaptive(char.Identity.Adaptive)
		desires, _ := e.desireStore.GetByCharacter(name)
		var threads []emotion.UnresolvedThread
		if e.emotionEng != nil {
			threads, _ = e.emotionEng.GetUnresolvedThreads(name)
		}
		pressure := emotion.CalculatePressure(vec, threads, nil, int(e.tickCount.Load()))

		action := emotion.TryAutonomousAction(name, pressure, desires, vec, e.actionBudget, int(e.tickCount.Load()), e.actionLogger)
		if action != nil {
			e.actionBudget.Record(name, int(e.tickCount.Load()))
			// Execute the autonomous action
			frame := core.ActionFrame{
				Actor:     name,
				Action:    action.ActionType,
				Target:    action.Target,
				Intensity: int(action.Urgency * 10),
				Emotion:   core.EmotionState{Primary: vec.Dominant(), Intensity: action.Urgency},
				Intent:    action.Reason,
			}
			evts, err := e.executor.Execute(frame, state)
			if err == nil {
				autonomousActions++
				for _, evt := range evts {
					evt.Actor = name
					e.gatekeeper.Submit(evt, events.SourceTick())
				}
			}
		}
	}

	// Fallback: rule-based scheduler for NPCs that didn't act
	schedulerStructure := core.WorldStructureConfig{}
	if path := e.currentWorldPathLocked(); path != "" {
		if s, err := world.LoadStructure(path); err == nil {
			schedulerStructure = s
		}
	}
	e.scheduler.Tick(
		e.loadedCharacters,
		e.focusCharacter,
		npcScenes,
		state,
		e.agents,
		e.executor,
		e.gatekeeper,
		int(e.tickCount.Load()),
		schedulerStructure,
	)
	e.reprojectStateAfterTickLocked(state)
	if autonomousActions > 0 {
		summary = append(summary, fmt.Sprintf("%d autonomous npc actions executed", autonomousActions))
	}

	// 11. Narrative compression: every 20 ticks, check if compaction needed
	if e.tickCount.Load()%20 == 0 {
		result, err := e.compressEng.AutoCompress()
		if err == nil && result.EventsCompressed > 0 {
			log.Printf("Compression: %d events across %d groups compressed",
				result.EventsCompressed, result.GroupsFound)
		}
	}

	e.reconcilePopulationLocked()
	appendTickDeltaSummary(&summary, beforeTension, state.Tension, beforePressure, beforeFaction, e.pulseEng, e.factionEng)
	if highlights := e.populationHighlightsLocked(); len(highlights) > 0 {
		summary = append(summary, highlights...)
	}
	e.lastTickSummary = summary
	e.appendTickHistoryLocked(state, summary)
}

func (e *Engine) reprojectStateAfterTickLocked(fallback core.WorldState) {
	eventList, err := e.eventStore.GetCanonicalEvents()
	if err != nil {
		return
	}
	projected := events.Project(eventList)
	projected.Clock = fallback.Clock
	if projected.Scene.Location == "" && projected.Scene.Description == "" && len(projected.Scene.Characters) == 0 {
		projected.Scene = fallback.Scene
	}
	e.stateMgr.Set(projected)
}

func summarizeTickEvents(label string, events []core.Event) string {
	if len(events) == 0 {
		return ""
	}
	typeCounts := map[string]int{}
	for _, evt := range events {
		typeCounts[evt.Type]++
	}
	parts := make([]string, 0, len(typeCounts))
	for kind, count := range typeCounts {
		parts = append(parts, fmt.Sprintf("%s:%d", kind, count))
	}
	sort.Strings(parts)
	return fmt.Sprintf("%s events -> %s", label, strings.Join(parts, ", "))
}

func appendTickDeltaSummary(summary *[]string, beforeTension, afterTension float64, beforePressure, beforeFaction map[string]float64, pulseEng *simulation.PulseEngine, factionEng *simulation.FactionEngine) {
	if summary == nil {
		return
	}
	if beforeTension != afterTension {
		*summary = append(*summary, fmt.Sprintf("tension %.2f -> %.2f", beforeTension, afterTension))
	}
	if pulseEng != nil {
		if line := summarizeFloatMapDelta("pressure", beforePressure, pulseEng.PressureStates()); line != "" {
			*summary = append(*summary, line)
		}
	}
	if factionEng != nil {
		if line := summarizeFloatMapDelta("faction", beforeFaction, factionEng.Tensions()); line != "" {
			*summary = append(*summary, line)
		}
	}
}

func (e *Engine) appendTickHistoryLocked(state core.WorldState, summary []string) {
	var pressureStates map[string]float64
	if e.pulseEng != nil {
		pressureStates = cloneFloatMap(e.pulseEng.PressureStates())
	}
	var factionTensions map[string]float64
	if e.factionEng != nil {
		factionTensions = cloneFloatMap(e.factionEng.Tensions())
	}
	snapshot := core.TickSnapshot{
		Tick:                 e.tickCount.Load(),
		Tension:              state.Tension,
		PressureStates:       pressureStates,
		FactionTensions:      factionTensions,
		PopulationHighlights: append([]string(nil), e.populationHighlightsLocked()...),
		Diagnostics:          cloneDiagnostics(e.buildTickDiagnosticsLocked(state)),
		Summary:              append([]string(nil), summary...),
		CreatedAt:            time.Now().UTC(),
	}
	e.tickHistory = append(e.tickHistory, snapshot)
	if len(e.tickHistory) > 12 {
		e.tickHistory = append([]core.TickSnapshot(nil), e.tickHistory[len(e.tickHistory)-12:]...)
	}
}

func cloneTickSnapshot(snapshot core.TickSnapshot) core.TickSnapshot {
	snapshot.PressureStates = cloneFloatMap(snapshot.PressureStates)
	snapshot.FactionTensions = cloneFloatMap(snapshot.FactionTensions)
	snapshot.PopulationHighlights = append([]string(nil), snapshot.PopulationHighlights...)
	snapshot.Diagnostics = cloneDiagnostics(snapshot.Diagnostics)
	snapshot.Summary = append([]string(nil), snapshot.Summary...)
	return snapshot
}

func cloneDiagnostics(src []map[string]interface{}) []map[string]interface{} {
	if len(src) == 0 {
		return nil
	}
	out := make([]map[string]interface{}, 0, len(src))
	for _, item := range src {
		copied := make(map[string]interface{}, len(item))
		for k, v := range item {
			copied[k] = v
		}
		out = append(out, copied)
	}
	return out
}

func (e *Engine) buildTickDiagnosticsLocked(state core.WorldState) []map[string]interface{} {
	var diagnostics []map[string]interface{}
	if state.Tension >= 0.8 {
		diagnostics = append(diagnostics, map[string]interface{}{
			"level":   "critical",
			"metric":  "tension",
			"value":   state.Tension,
			"message": "世界张力极高，建议降低压力源强度或增加缓冲事件",
		})
	} else if state.Tension >= 0.6 {
		diagnostics = append(diagnostics, map[string]interface{}{
			"level":   "warning",
			"metric":  "tension",
			"value":   state.Tension,
			"message": "世界张力偏高，注意监控压力演化趋势",
		})
	}
	if e.factionEng != nil {
		for facID, tension := range e.factionEng.Tensions() {
			if tension >= 0.8 {
				diagnostics = append(diagnostics, map[string]interface{}{
					"level":   "critical",
					"metric":  "faction_tension",
					"target":  facID,
					"value":   tension,
					"message": fmt.Sprintf("势力 '%s' 紧张度极高，建议调整派系关系或引入调解事件", facID),
				})
			} else if tension >= 0.6 {
				diagnostics = append(diagnostics, map[string]interface{}{
					"level":   "warning",
					"metric":  "faction_tension",
					"target":  facID,
					"value":   tension,
					"message": fmt.Sprintf("势力 '%s' 紧张度上升，建议关注派系动态", facID),
				})
			}
		}
	}
	if len(e.npcTickExposure) > 0 {
		fastGrowing := 0
		for _, exposure := range e.npcTickExposure {
			if exposure >= 50 {
				fastGrowing++
			}
		}
		if fastGrowing >= 3 {
			diagnostics = append(diagnostics, map[string]interface{}{
				"level":   "info",
				"metric":  "population_growth",
				"value":   fastGrowing,
				"message": fmt.Sprintf("%d 个背景 NPC 正在快速成长，预计很快会触发晋升", fastGrowing),
			})
		}
	}
	diagnostics = append(diagnostics, e.structureAuthoringDiagnosticsLocked(state)...)
	return diagnostics
}

func (e *Engine) buildTrajectorySummaryLocked(state core.WorldState) []string {
	if len(e.tickHistory) == 0 {
		return nil
	}

	first := e.tickHistory[0]
	last := e.tickHistory[len(e.tickHistory)-1]
	lines := make([]string, 0, 5)
	lines = append(lines, fmt.Sprintf("tension trend: %.2f -> %.2f (%s)", first.Tension, last.Tension, tensionTrendLabel(first.Tension, last.Tension)))

	if pressureID, pressureValue := dominantFloatMapEntry(last.PressureStates); pressureID != "" {
		lines = append(lines, fmt.Sprintf("dominant pressure: %s %.2f", pressureID, pressureValue))
	}
	if factionID, factionValue := dominantFloatMapEntry(last.FactionTensions); factionID != "" {
		lines = append(lines, fmt.Sprintf("dominant faction tension: %s %.2f", factionID, factionValue))
	}

	if promotedCount, promotedLine := e.populationOutcomeSummaryLocked(); promotedCount > 0 {
		lines = append(lines, promotedLine)
	} else if len(last.PopulationHighlights) > 0 {
		lines = append(lines, fmt.Sprintf("population outcome: %s", last.PopulationHighlights[0]))
	}

	if diagLine := diagnosticsSummaryLine(e.tickHistory); diagLine != "" {
		lines = append(lines, diagLine)
	}
	return lines
}

func tensionTrendLabel(first, last float64) string {
	delta := last - first
	switch {
	case delta > 0.08:
		return "rising"
	case delta < -0.08:
		return "falling"
	default:
		return "stable"
	}
}

func dominantFloatMapEntry(values map[string]float64) (string, float64) {
	bestKey := ""
	bestValue := 0.0
	for key, value := range values {
		if bestKey == "" || value > bestValue || (value == bestValue && key < bestKey) {
			bestKey = key
			bestValue = value
		}
	}
	return bestKey, bestValue
}

func (e *Engine) populationOutcomeSummaryLocked() (int, string) {
	path := strings.TrimSpace(e.currentWorldPathLocked())
	if path == "" {
		return 0, ""
	}
	cfg, _, err := world.EnsureSeededPopulation(path)
	if err != nil || len(cfg.PromotedNPCs) == 0 {
		return 0, ""
	}
	promoted := append([]core.PromotedNPC(nil), cfg.PromotedNPCs...)
	sort.Slice(promoted, func(i, j int) bool {
		if promoted[i].Attention.Score == promoted[j].Attention.Score {
			return promoted[i].Name < promoted[j].Name
		}
		return promoted[i].Attention.Score > promoted[j].Attention.Score
	})
	top := promoted[0]
	return len(promoted), fmt.Sprintf("population outcome: %d promoted, top %s(%.1f)", len(promoted), top.Name, top.Attention.Score)
}

func diagnosticsSummaryLine(history []core.TickSnapshot) string {
	if len(history) == 0 {
		return ""
	}
	counts := map[string]int{}
	for _, snapshot := range history {
		seen := map[string]bool{}
		for _, item := range snapshot.Diagnostics {
			metric, _ := item["metric"].(string)
			if metric == "" || seen[metric] {
				continue
			}
			seen[metric] = true
			counts[metric]++
		}
	}
	if len(counts) == 0 {
		return ""
	}
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if counts[keys[i]] == counts[keys[j]] {
			return keys[i] < keys[j]
		}
		return counts[keys[i]] > counts[keys[j]]
	})
	parts := make([]string, 0, minInt(3, len(keys)))
	for _, key := range keys[:minInt(3, len(keys))] {
		parts = append(parts, fmt.Sprintf("%s x%d", key, counts[key]))
	}
	return "recent diagnostics: " + strings.Join(parts, " · ")
}

func cloneFloatMap(src map[string]float64) map[string]float64 {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]float64, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func summarizeFloatMapDelta(label string, before, after map[string]float64) string {
	type delta struct {
		key    string
		before float64
		after  float64
		change float64
	}
	deltas := make([]delta, 0)
	keys := map[string]bool{}
	for key := range before {
		keys[key] = true
	}
	for key := range after {
		keys[key] = true
	}
	for key := range keys {
		b := before[key]
		a := after[key]
		if b == a {
			continue
		}
		change := a - b
		if change < 0 {
			change = -change
		}
		deltas = append(deltas, delta{key: key, before: b, after: a, change: change})
	}
	if len(deltas) == 0 {
		return ""
	}
	sort.Slice(deltas, func(i, j int) bool {
		if deltas[i].change == deltas[j].change {
			return deltas[i].key < deltas[j].key
		}
		return deltas[i].change > deltas[j].change
	})
	parts := make([]string, 0, minInt(len(deltas), 3))
	for _, item := range deltas[:minInt(len(deltas), 3)] {
		parts = append(parts, fmt.Sprintf("%s %.2f->%.2f", item.key, item.before, item.after))
	}
	return fmt.Sprintf("%s delta -> %s", label, strings.Join(parts, ", "))
}

func applyTickEvent(state *core.WorldState, evt core.Event) {
	switch evt.Type {
	case "tension_change":
		if delta, ok := evt.Payload["delta"].(float64); ok {
			state.Tension += delta
		}
	case "variable_set":
		if key, ok := evt.Payload["key"].(string); ok {
			if state.Variables == nil {
				state.Variables = map[string]interface{}{}
			}
			state.Variables[key] = evt.Payload["value"]
		}
	}
}

// SetTension directly sets tension for testing/directing.
func (e *Engine) SetTension(v float64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	state := e.stateMgr.Get()
	state.Tension = v
	e.stateMgr.Set(state)
	if e.stateMachine != nil {
		e.stateMachine.Transition(v, "director_inject")
	}
}

// emotionVectorFromAdaptive converts character adaptive stats to an emotion vector.
func emotionVectorFromAdaptive(adaptive map[string]float64) emotion.EmotionVector {
	vec := emotion.EmotionVector{}
	if v, ok := adaptive["trust"]; ok {
		vec.Trust = v / 10
		vec.Attachment = v / 10
	}
	if v, ok := adaptive["intimacy"]; ok {
		vec.Attachment = clamp01((vec.Attachment + v/10) / 2)
	}
	if v, ok := adaptive["fear"]; ok {
		vec.Fear = v / 10
	}
	// Default: mild positive disposition
	if vec.Trust == 0 && vec.Attachment == 0 {
		vec.Trust = 0.3
		vec.Attachment = 0.3
	}
	return vec
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// QueryActionLog returns action log entries filtered by character and mode.
func (e *Engine) QueryActionLog(character string, firedOnly, blockedOnly bool, limit int) []interface{} {
	if e.actionLogger == nil {
		return nil
	}
	entries, _ := e.actionLogger.QueryDB(character, firedOnly, blockedOnly, limit)
	out := make([]interface{}, len(entries))
	for i, ent := range entries {
		out[i] = ent
	}
	return out
}

// ActionLogStats returns aggregate statistics for the action log.
func (e *Engine) ActionLogStats() map[string]interface{} {
	if e.actionLogger == nil {
		return map[string]interface{}{"total_entries": 0}
	}
	return e.actionLogger.Stats()
}

// GetCausalityChainNarrative returns chain filtered to narrative events only.
func (e *Engine) GetCausalityChainNarrative(eventID string, depth int) (interface{}, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.gatekeeper.Causality().GetChainNarrativeOnly(eventID, depth)
}

// GetCausalitySummaryNarrative returns summary with system/tick events filtered out.
func (e *Engine) GetCausalitySummaryNarrative(eventID string, depth int) (string, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.gatekeeper.Causality().GetChainSummaryNarrativeOnly(eventID, depth)
}

// SeedNPCDesires generates initial desires for all loaded characters.
// Safe to call multiple times — only seeds characters with zero desires.
func (e *Engine) SeedNPCDesires() {
	for _, name := range e.loadedCharacters {
		if name == e.focusCharacter {
			continue // skip focus character (they're driven by player interaction)
		}
		char, ok := e.agents.GetCharacter(name)
		if !ok {
			continue
		}

		var goals []emotion.GoalSeed
		var hidden []emotion.HiddenGoalSeed
		for _, g := range char.Goals {
			if g.Type == "hidden" {
				hidden = append(hidden, emotion.HiddenGoalSeed{ID: g.ID, Priority: g.Priority})
			} else {
				goals = append(goals, emotion.GoalSeed{ID: g.ID, Priority: g.Priority, Target: g.Target})
			}
		}

		desires := emotion.SeedDesires(e.desireStore, name, char.Identity.Immutable, char.Identity.Adaptive, goals, hidden)
		if len(desires) > 0 {
			log.Printf("Seeded %d desires for NPC '%s'", len(desires), name)
		}
	}
}

// SwitchLLM hot-swaps the active LLM adapter without restart.
func (e *Engine) SwitchLLM(name, endpoint, apiKey, model string) {
	if endpoint == "" || model == "" {
		log.Printf("LLM switch rejected: incomplete config for '%s' (model=%q endpoint=%q)", name, model, endpoint)
		return
	}
	e.llmRouter.UpdateAdapter("default", endpoint, apiKey, model)
	log.Printf("LLM switched to %s @ %s", model, endpoint)
}

func cloneNPCExposure(src map[string]int) map[string]int {
	if src == nil {
		return nil
	}
	out := make(map[string]int, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}
