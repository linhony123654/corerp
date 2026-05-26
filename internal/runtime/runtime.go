package runtime

import (
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
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
)

// CharWorld binds a character to their world context.
type CharWorld struct {
	WorldName string
	CoreRules string
	Scene     core.SceneState
}

// Engine orchestrates the narrative loop.
type Engine struct {
	mu           sync.RWMutex
	stateMgr     *state.Manager
	eventStore   *events.Store
	gatekeeper   *events.Gatekeeper
	memEngine    *memory.Engine
	agents       *agents.EnvelopeManager
	compiler     *context.Compiler
	llmRouter    *llm.Router
	executor     *actions.Executor
	tickLoop     *simulation.Loop
	decayEngine  *memory.DecayEngine
	tensionEng   *narrative.TensionEngine
	stateMachine *state.StateMachine
	compressEng  *narrative.CompressionEngine
	planner      *agents.Planner
	scheduler    *agents.Scheduler
	emotionEng   *emotion.Engine
	desireStore  *emotion.DesireStore
	actionLogger *emotion.ActionLogger
	actionBudget *emotion.ActionBudget
	directorCfg  core.DirectorConfig
	lastPlan     core.DirectorPlan
	turnTraces   []core.TurnTrace

	// Config
	instanceID       string
	instanceCreated  time.Time
	activeCharacter  string
	loadedCharacters []string
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
	tickCount       int
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
		stateMachine:     state.NewStateMachine(),
		compressEng:      narrative.NewCompressionEngine(eventStore),
		planner:          agents.NewPlanner(),
		scheduler:        agents.NewScheduler(),
		agents:           agentsMgr,
		emotionEng:       emotionEng,
		desireStore:      emotion.NewDesireStore(memEngine.DB()),
		actionLogger:     actionLogger,
		actionBudget:     emotion.DefaultBudget(),
		directorCfg:      core.DirectorConfig{Mode: "manual", MaxSpeakers: 1},
		turnTraces:       make([]core.TurnTrace, 0, 32),
		compiler:         context.NewCompiler("budgets.yml"),
		llmRouter:        llmRouter,
		executor:         actions.NewExecutor(),
		instanceID:       fmt.Sprintf("inst_%d", time.Now().UnixNano()),
		instanceCreated:  time.Now().UTC(),
		activeCharacter:  activeChar,
		loadedCharacters: loadedChars,
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
	e.memEngine.LoadRecentDialogueFromDB(e.activeCharacter, 15)
	return nil
}

// SyncActiveWorldContext applies the active character's world metadata and
// scene to the in-memory state without appending a new canonical event.
func (e *Engine) SyncActiveWorldContext() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.syncActiveWorldContextLocked()
}

func (e *Engine) syncActiveWorldContextLocked() {
	cw, ok := e.charWorlds[e.activeCharacter]
	if !ok {
		return
	}

	e.worldName = cw.WorldName
	e.coreRules = cw.CoreRules

	state := e.stateMgr.Get()
	state.Scene = normalizeSceneForCharacter(cw.Scene, e.activeCharacter, e.playerRoleNameLocked())
	e.stateMgr.Set(state)
}

func normalizeSceneForCharacter(scene core.SceneState, activeCharacter, playerName string) core.SceneState {
	if activeCharacter == "" {
		return scene
	}
	if strings.TrimSpace(playerName) == "" {
		playerName = "玩家"
	}

	chars := append([]string(nil), scene.Characters...)
	if len(chars) == 0 {
		scene.Characters = []string{activeCharacter, playerName}
		return scene
	}

	activeSeen := false
	playerSeen := false
	leadReplaced := false
	for i, name := range chars {
		if name == activeCharacter {
			activeSeen = true
			continue
		}
		if isPlayerPlaceholder(name) || name == playerName {
			chars[i] = playerName
			playerSeen = true
			continue
		}
		if !leadReplaced {
			chars[i] = activeCharacter
			activeSeen = true
			leadReplaced = true
		}
	}

	if !activeSeen {
		chars = append([]string{activeCharacter}, chars...)
	}
	if !playerSeen {
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
		previousSpeaker := e.activeCharacter
		plan := e.directTurnLocked(userInput, worldState)
		e.mu.Unlock()

		if len(plan.Steps) == 0 {
			ch <- "[ERROR] director produced no turn steps\n"
			return
		}

		trace := core.TurnTrace{
			Turn:         turnNumber,
			Character:    plan.Steps[0].Speaker,
			UserInput:    userInput,
			DirectorPlan: plan,
			CreatedAt:    time.Now().UTC(),
		}
		defer func() {
			e.mu.Lock()
			e.recordTraceLocked(trace)
			e.mu.Unlock()
		}()

		userMsg := core.Message{Role: "user", Content: userInput}
		e.mu.Lock()
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
				trace.Character = stepTrace.Character
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
	scene = normalizeSceneForCharacter(scene, e.activeCharacter, e.playerRoleName())
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
	return e.agents.GetCharacter(e.activeCharacter)
}

func (e *Engine) GetCharacterName() string {
	return e.activeCharacter
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
	return core.RuntimeInstanceSummary{
		ID:               e.instanceID,
		Label:            e.worldName,
		WorldName:        e.worldName,
		ActiveCharacter:  e.activeCharacter,
		LoadedCharacters: append([]string(nil), e.loadedCharacters...),
		CreatedAt:        e.instanceCreated,
		Status:           InstanceStatusRunning,
	}
}

// SwitchCharacter changes the active character. Saves working memory for
// the old character and loads dialogue history + world context for the new one.
func (e *Engine) SwitchCharacter(name string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	return e.setActiveCharacterLocked(name, true, true)
}

func (e *Engine) switchCharacterLocked(name string, syncWorld bool) error {
	return e.setActiveCharacterLocked(name, syncWorld, true)
}

func (e *Engine) setActiveCharacterLocked(name string, syncWorld, resetTurn bool) error {
	if _, ok := e.agents.GetCharacter(name); !ok {
		return fmt.Errorf("character '%s' not loaded", name)
	}

	if name == e.activeCharacter {
		if syncWorld {
			e.syncActiveWorldContextLocked()
		}
		return nil
	}

	if resetTurn {
		e.dialogueHistory = nil
		e.compiler.SetMode("full_load")
	}
	e.activeCharacter = name
	e.memEngine.LoadRecentDialogueFromDB(e.activeCharacter, 15)

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
	char, _ := e.agents.GetCharacter(e.activeCharacter)
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
	rollEvent := events.BuildEvent("dice_roll", e.activeCharacter, "",
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
	e.memEngine.PushDialogue(rollMsg, e.activeCharacter)
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
	return e.memEngine.GetRecentDialogue(e.activeCharacter)
}

// GetDialogueLimit returns up to N recent dialogue messages.
func (e *Engine) GetDialogueLimit(limit int) []core.Message {
	e.memEngine.LoadRecentDialogueFromDB(e.activeCharacter, limit)
	return e.memEngine.GetRecentDialogue(e.activeCharacter)
}

// ResetDialogue clears the current character's dialogue.
func (e *Engine) ResetDialogue() {
	e.memEngine.ResetDialogue(e.activeCharacter)
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
	state.Scene = normalizeSceneForCharacter(state.Scene, e.activeCharacter, e.playerRoleNameLocked())
	e.stateMgr.Set(state)
	return e.playerRole, nil
}

// DebugInfo returns internal state for debugging.
func (e *Engine) DebugInfo() map[string]interface{} {
	e.mu.RLock()
	defer e.mu.RUnlock()

	state := e.stateMgr.Get()
	recent := e.memEngine.GetRecentDialogue(e.activeCharacter)
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
		"vector_search":      e.memEngine.CountFacts(e.activeCharacter) >= 100,
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

func (e *Engine) onTick() {
	e.mu.Lock()
	defer e.mu.Unlock()

	state := e.stateMgr.Get()

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

	// 3. Auto-promote quarantined events
	if n, err := e.gatekeeper.AutoPromote(); err == nil && n > 0 {
		// Promoted n events to canonical
	}

	// 4. Memory & Relationship Decay
	if e.decayEngine != nil {
		state, _ = e.decayEngine.Tick(state)
	}

	// 5. Reload state from canonical events to pick up promoted events
	if eventList, err := e.eventStore.GetCanonicalEvents(); err == nil {
		projected := events.Project(eventList)
		// Preserve current clock since Project doesn't have clock state persistence yet
		projected.Clock = state.Clock
		projected.Relationships = state.Relationships
		e.stateMgr.UpdateFromProjection(projected)
	}

	// 5. Tension Engine check
	if e.tensionEng != nil {
		pressureEvents := e.tensionEng.Tick(state, e.turnCount)
		for _, evt := range pressureEvents {
			e.gatekeeper.Submit(evt, events.SourceTick())
			if evt.Type == "tension_change" {
				if delta, ok := evt.Payload["delta"].(float64); ok {
					state.Tension += delta
				}
			}
		}
	}

	// 6. Narrative State Machine transition
	if e.stateMachine != nil {
		e.stateMachine.Transition(state.Tension, "tick")
	}

	// 7. Update state manager with all tick changes
	e.stateMgr.Set(state)

	// 8. NPC Desire-driven autonomous actions (non-active characters)
	e.tickCount++
	npcScenes := make(map[string]core.SceneState)
	for _, name := range e.loadedCharacters {
		if cw, ok := e.charWorlds[name]; ok {
			npcScenes[name] = cw.Scene
		}
	}

	// For each non-active NPC, attempt desire-driven autonomous action
	for _, name := range e.loadedCharacters {
		if name == e.activeCharacter {
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
		pressure := emotion.CalculatePressure(vec, threads, nil, e.tickCount)

		action := emotion.TryAutonomousAction(name, pressure, desires, vec, e.actionBudget, e.tickCount, e.actionLogger)
		if action != nil {
			e.actionBudget.Record(name, e.tickCount)
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
				for _, evt := range evts {
					evt.Actor = name
					e.gatekeeper.Submit(evt, events.SourceTick())
				}
			}
		}
	}

	// Fallback: rule-based scheduler for NPCs that didn't act
	e.scheduler.Tick(
		e.loadedCharacters,
		e.activeCharacter,
		npcScenes,
		state,
		e.agents,
		e.executor,
		e.gatekeeper,
		e.tickCount,
	)

	// 9. Narrative compression: every 20 ticks, check if compaction needed
	if e.tickCount%20 == 0 {
		result, err := e.compressEng.AutoCompress()
		if err == nil && result.EventsCompressed > 0 {
			log.Printf("Compression: %d events across %d groups compressed",
				result.EventsCompressed, result.GroupsFound)
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
		if name == e.activeCharacter {
			continue // skip active character (they're driven by player interaction)
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
