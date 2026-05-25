package runtime

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"corerp/internal/actions"
	"corerp/internal/agents"
	"corerp/internal/context"
	"corerp/internal/core"
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
	mu          sync.RWMutex
	stateMgr    *state.Manager
	eventStore  *events.Store
	gatekeeper  *events.Gatekeeper
	memEngine   *memory.Engine
	agents      *agents.EnvelopeManager
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

	// Config
	activeCharacter  string
	loadedCharacters []string
	charWorlds       map[string]CharWorld // per-character world context
	worldName       string
	coreRules       string
	sessionID       string

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
	return &Engine{
		stateMgr:        state.New(),
		eventStore:      eventStore,
		gatekeeper:      gatekeeper,
		memEngine:       memEngine,
		decayEngine:     decayEngine,
		tensionEng:      narrative.NewTensionEngine(),
		stateMachine:    state.NewStateMachine(),
		compressEng:     narrative.NewCompressionEngine(eventStore),
		planner:         agents.NewPlanner(),
		scheduler:       agents.NewScheduler(),
		agents:          agentsMgr,
		compiler:        context.NewCompiler("budgets.yml"),
		llmRouter:       llmRouter,
		executor:        actions.NewExecutor(),
		activeCharacter: activeChar,
		loadedCharacters: loadedChars,
		charWorlds:      charWorlds,
		worldName:       cw.WorldName,
		coreRules:       cw.CoreRules,
		sessionID:       fmt.Sprintf("sess_%d", time.Now().Unix()),
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

// ProcessTurn handles one user input and returns a channel of SSE chunks.
func (e *Engine) ProcessTurn(userInput string) (<-chan string, error) {
	ch := make(chan string, 32)

	go func() {
		defer close(ch)

		e.turnCount++

		// 1. Record user input as event
		userMsg := core.Message{Role: "user", Content: userInput}
		e.memEngine.PushDialogue(userMsg, e.activeCharacter)
		e.dialogueHistory = append(e.dialogueHistory, userMsg)

		// Record user input as canonical event
		userEvent := events.BuildEvent("user_message", "user", e.activeCharacter,
			map[string]interface{}{"content": userInput})
		e.gatekeeper.Submit(userEvent, events.SourceUserInput())

		// 2. Load current state
		worldState := e.stateMgr.Get()

		// 3. Get character info
		_, ok := e.agents.GetCharacter(e.activeCharacter)
		if !ok {
			ch <- fmt.Sprintf("[ERROR] Character '%s' not loaded\n", e.activeCharacter)
			return
		}

		// 4. Active goals + planner
		goals := e.agents.ActiveGoals(e.activeCharacter, worldState)
		workingMem, _ := e.memEngine.GetWorkingMemory(e.activeCharacter)
		planSteps := e.planner.Plan(e.activeCharacter, worldState, goals, workingMem)
		planGoalFrames := agents.StepsToGoals(planSteps)
		// Merge planner goals into snapshot goals
		allGoals := e.compiler.GoalsToFrames(goals)
		allGoals = append(allGoals, planGoalFrames...)

		// 5. Memory retrieval
		memories := e.memEngine.Recall(userInput, e.activeCharacter, goals)
		semanticFacts := make([]core.FactFrame, 0)
		episodicEvents := make([]core.EventFrame, 0)
		for _, m := range memories {
			switch m.Type {
			case "semantic":
				// Convert memory content back to fact (P1 simplified)
				parts := strings.Split(m.Content, " ")
				if len(parts) >= 3 {
					semanticFacts = append(semanticFacts, core.FactFrame{
						Subject:   parts[0],
						Predicate: parts[1],
						Object:    strings.Join(parts[2:], " "),
						Confidence: m.Score,
					})
				}
			case "episodic":
				episodicEvents = append(episodicEvents, core.EventFrame{
					EventID:         m.ID,
					Type:            "memory",
					Description:     m.Content,
					EmotionalWeight: m.Score,
				})
			}
		}

		// Also get stored facts from DB
		storedFacts, _ := e.memEngine.GetAllFacts(e.activeCharacter)
		semanticFacts = append(semanticFacts, storedFacts...)

		// Get recent episodic
		recentEpi, _ := e.memEngine.GetRecentEpisodic(e.activeCharacter, 5)
		episodicEvents = append(episodicEvents, recentEpi...)

		// Get working memory
		workingMem, _ = e.memEngine.GetWorkingMemory(e.activeCharacter)

		// 6. Allowed actions (goal-based + state machine filtered)
		baseActions := actions.AllowedActionsFor(worldState, goals)
		allowedActions := e.stateMachine.AllowedActions(baseActions)

		// 7. Persona frame
		personaFrame := e.agents.GetPersonaFrame(e.activeCharacter)

		// 8. Compile snapshot (with token budget enforcement)
		snapshot, err := e.compiler.Compile(
			worldState,
			personaFrame,
			workingMem,
			semanticFacts,
			episodicEvents,
			e.memEngine.GetRecentDialogue(e.activeCharacter),
			allGoals,
			allowedActions,
			e.coreRules,
		)
		if err != nil {
			ch <- fmt.Sprintf("[ERROR] Snapshot compile failed: %v\n", err)
			return
		}

		// Reset to normal budget after first post-switch turn
		e.compiler.SetMode("normal")

		// 9. Render prompt
		prompt := e.compiler.RenderSnapshot(snapshot)

		// 10. LLM generation (collect full output server-side)
		var llmOutput strings.Builder
		err = e.llmRouter.Generate(llm.TaskNarrative, prompt, func(chunk core.LLMStreamChunk) {
			if chunk.Done {
				return
			}
			llmOutput.WriteString(chunk.Content)
		})
		if err != nil {
			ch <- fmt.Sprintf("[ERROR] LLM generation failed: %v\n", err)
			return
		}

		rawOutput := llmOutput.String()

		// 11. Extract Action Frame + narrative
		actionFrame, narrative, _ := llm.ExtractActionFrame(rawOutput)
		if narrative == "" {
			narrative = rawOutput // Fallback
		}

		// 12. Validator (on full output)
		if actionFrame.Action != "" {
			if err := e.agents.Validate(actionFrame, narrative, e.activeCharacter); err != nil {
				ch <- fmt.Sprintf("[系统拦截: %v]\n", err)
				actionFrame.Action = "speak"
				actionFrame.Intensity = 1
			}
		}

		// 13. Stream only the narrative text to user (typing effect)
		for _, r := range narrative {
			ch <- string(r)
		}

		// 13. Execute action (via gatekeeper — action results are canonical)
		if actionFrame.Action != "" {
			evts, execErr := e.executor.Execute(actionFrame, worldState)
			if execErr != nil {
				ch <- fmt.Sprintf("\n[ERROR] Action execution failed: %v\n", execErr)
			} else {
				for _, evt := range evts {
					evt.SessionID = e.sessionID
					evt.SceneID = worldState.Scene.Location
					if err := e.gatekeeper.Submit(evt, events.SourceActionResult()); err != nil {
						// Log but don't fail
					}
				}
					// Reset tension heat-death timer on conflict actions
					if actionFrame.Action == "attack" || actionFrame.Action == "threaten" {
						e.tensionEng.ResetConflictTimer(e.turnCount)
					}
			}
		}

		// 14. Auto-promote quarantine events periodically
		if e.turnCount%5 == 0 {
			go func() {
				if n, err := e.gatekeeper.AutoPromote(); err == nil && n > 0 {
					// Silently promoted n events
				}
			}()
		}

		// 15. Record assistant dialogue
		if narrative != "" {
			assistantMsg := core.Message{Role: "assistant", Content: narrative}
			e.memEngine.PushDialogue(assistantMsg, e.activeCharacter)
			e.dialogueHistory = append(e.dialogueHistory, assistantMsg)
		}

		// 15. Update working memory every 15 turns
		if len(e.dialogueHistory)%15 == 0 {
			go e.updateWorkingMemory()
		}

		// 16. Store semantic facts from narrative (P1: simplified, quarantined)
		go e.extractAndStoreFacts(narrative)

		ch <- "\n"
	}()

	return ch, nil
}

func (e *Engine) updateWorkingMemory() {
	dialogue := e.memEngine.GetRecentDialogue(e.activeCharacter)
	var dialogueText strings.Builder
	for _, m := range dialogue {
		role := "角色"
		if m.Role == "user" {
			role = "用户"
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
	e.memEngine.SetWorkingMemory(e.activeCharacter, summary)
}

func (e *Engine) extractAndStoreFacts(narrative string) {
	if narrative == "" {
		return
	}
	// P2: store narrative as quarantined fact event
	// Structured extraction will be added later
	evt := events.BuildEvent("fact_extracted", e.activeCharacter, "",
		map[string]interface{}{"narrative": narrative, "turn": e.turnCount})
	e.gatekeeper.Submit(evt, events.SourceLLMExtracted())
}

func (e *Engine) GetState() core.WorldState {
	return e.stateMgr.Get()
}

func (e *Engine) SeedScene(scene core.SceneState) {
	state := e.stateMgr.Get()
	if state.Scene.Location == "" {
		state.Scene = scene
		e.stateMgr.Set(state)
	}
}

func (e *Engine) GetCharacter() (core.Character, bool) {
	return e.agents.GetCharacter(e.activeCharacter)
}

func (e *Engine) GetCharacterName() string {
	return e.activeCharacter
}

func (e *Engine) GetLoadedCharacters() []string {
	names := make([]string, len(e.loadedCharacters))
	copy(names, e.loadedCharacters)
	return names
}

// SwitchCharacter changes the active character. Saves working memory for
// the old character and loads dialogue history + world context for the new one.
func (e *Engine) SwitchCharacter(name string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if _, ok := e.agents.GetCharacter(name); !ok {
		return fmt.Errorf("character '%s' not loaded", name)
	}

	if name == e.activeCharacter {
		return nil
	}

	e.dialogueHistory = nil
	e.turnCount = 0
	e.activeCharacter = name
	e.memEngine.LoadRecentDialogueFromDB(e.activeCharacter, 15)

	// Use full_load budget for the first turn after switch
	e.compiler.SetMode("full_load")

	// Switch world context
	if cw, ok := e.charWorlds[name]; ok {
		e.worldName = cw.WorldName
		e.coreRules = cw.CoreRules
		// Update scene in world state
		state := e.stateMgr.Get()
		state.Scene = cw.Scene
		e.stateMgr.Set(state)
	}

	return nil
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

func (e *Engine) GetWorldName() string {
	return e.worldName
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
		"turn_count":          e.turnCount,
		"dialogue_in_memory":  len(recent),
		"dialogue_history":    dialoguePreview,
		"world_clock":         state.Clock,
		"scene":               state.Scene,
		"tension":             state.Tension,
		"narrative_state":     e.stateMachine.Current(),
		"canonical_events":    canon,
		"quarantined_events":  quarantined,
			"npc_actions":         e.scheduler.RecentActions(0),
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

	// 8. NPC Scheduler: run autonomous actions for non-active characters
	e.tickCount++
	// Build scene map for NPC world contexts
	npcScenes := make(map[string]core.SceneState)
	for _, name := range e.loadedCharacters {
		if cw, ok := e.charWorlds[name]; ok {
			npcScenes[name] = cw.Scene
		}
	}
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
