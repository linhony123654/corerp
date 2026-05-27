package runtime

import (
	"fmt"
	"strings"

	"corerp/internal/actions"
	"corerp/internal/agents"
	"corerp/internal/core"
	"corerp/internal/events"
	"corerp/internal/llm"
	"corerp/internal/world"
)

func uniqueTurnSpeakers(steps []core.TurnStep) []string {
	seen := map[string]bool{}
	var speakers []string
	for _, step := range steps {
		if step.Speaker == "" || seen[step.Speaker] {
			continue
		}
		seen[step.Speaker] = true
		speakers = append(speakers, step.Speaker)
	}
	return speakers
}

func (e *Engine) executeTurnStep(step core.TurnStep, userInput string, turnNumber int, handoff *core.StepHandoff) core.TurnStepTrace {
	trace := core.TurnStepTrace{
		Step:      step,
		Character: step.Speaker,
		Handoff:   cloneStepHandoff(handoff),
	}

	e.mu.Lock()
	if err := e.setActiveCharacterLocked(step.Speaker, true, false); err != nil {
		e.mu.Unlock()
		trace.Error = err.Error()
		return trace
	}
	playerRole := normalizePlayerRole(e.playerRole)
	coreRules := e.coreRules
	sessionID := e.sessionID
	e.mu.Unlock()

	worldState := e.stateMgr.Get()
	if _, ok := e.agents.GetCharacter(step.Speaker); !ok {
		trace.Error = fmt.Sprintf("character '%s' not loaded", step.Speaker)
		return trace
	}

	goals := e.agents.ActiveGoals(step.Speaker, worldState, turnNumber)
	workingMem, _ := e.memEngine.GetWorkingMemory(step.Speaker)
	structure := core.WorldStructureConfig{}
	if path := e.currentWorldPathLocked(); path != "" {
		if s, err := world.LoadStructure(path); err == nil {
			structure = s
		}
	}
	planSteps := e.planner.Plan(step.Speaker, worldState, goals, workingMem, structure)
	allGoals := e.compiler.GoalsToFrames(goals)
	allGoals = append(allGoals, agents.StepsToGoals(planSteps)...)
	trace.ActiveGoals = traceGoalsFromFrames(allGoals)

	memories := e.memEngine.Recall(userInput, step.Speaker, goals)
	semanticFacts, episodicEvents, memoryTrace := convertMemories(memories)
	trace.Memories = memoryTrace

	storedFacts, _ := e.memEngine.GetAllFacts(step.Speaker)
	semanticFacts = append(semanticFacts, storedFacts...)
	recentEpi, _ := e.memEngine.GetRecentEpisodic(step.Speaker, 5)
	episodicEvents = append(episodicEvents, recentEpi...)

	workingMem, _ = e.memEngine.GetWorkingMemory(step.Speaker)
	trace.WorkingMemory = workingMem

	baseActions := actions.AllowedActionsFor(worldState, goals)
	allowedActions := e.stateMachine.AllowedActions(baseActions)
	allowedActions = filterAllowedActionsForStep(step, allowedActions)
	trace.AllowedActions = append([]string(nil), allowedActions...)

	personaFrame := e.agents.GetPersonaFrame(step.Speaker)
	e.mu.Lock()
	restoreMode := e.compiler.BudgetMode()
	if step.BudgetMode != "" {
		e.compiler.SetMode(step.BudgetMode)
	}
	snapshot, err := e.compiler.Compile(
		worldState,
		personaFrame,
		playerRole,
		workingMem,
		semanticFacts,
		episodicEvents,
		e.memEngine.GetRecentDialogue(step.Speaker),
		allGoals,
		allowedActions,
		coreRules,
	)
	e.compiler.SetMode(restoreMode)
	e.mu.Unlock()
	if err != nil {
		trace.Error = fmt.Sprintf("snapshot compile failed: %v", err)
		return trace
	}
	trace.TokenBudget = snapshot.TokenBudget
	trace.UsedTokens = snapshot.UsedTokens
	trace.SemanticFacts = traceFactsFromFrames(snapshot.SemanticFacts)
	trace.EpisodicEvents = append(trace.EpisodicEvents, snapshot.EpisodicEvents...)

	prompt := composeTurnPrompt(e.compiler.RenderSnapshot(snapshot), step, playerRole.Name, handoff)
	var llmOutput strings.Builder
	err = e.llmRouter.Generate(llm.TaskNarrative, prompt, func(chunk core.LLMStreamChunk) {
		if chunk.Done {
			return
		}
		llmOutput.WriteString(chunk.Content)
	})
	if err != nil {
		trace.Error = fmt.Sprintf("LLM generation failed: %v", err)
		return trace
	}

	rawOutput := llmOutput.String()
	actionFrame, narrative, _ := llm.ExtractActionFrame(rawOutput)
	if narrative == "" {
		narrative = rawOutput
	}
	actionFrame = normalizeActionForStep(step, actionFrame, allowedActions)

	if actionFrame.Action != "" {
		if err := e.agents.Validate(actionFrame, narrative, step.Speaker); err != nil {
			trace.Validator = core.ValidatorTrace{Blocked: true, Reason: err.Error()}
			actionFrame.Action = "speak"
			actionFrame.Intensity = 1
		}
	}

	trace.ActionFrame = actionFrame
	trace.Narrative = narrative

	if actionFrame.Action != "" {
		evts, execErr := e.executor.Execute(actionFrame, worldState)
		if execErr != nil {
			trace.Error = fmt.Sprintf("action execution failed: %v", execErr)
		} else {
			for _, evt := range evts {
				evt.SessionID = sessionID
				evt.SceneID = worldState.Scene.Location
				if err := e.gatekeeper.Submit(evt, events.SourceActionResult()); err != nil {
					continue
				}
				trace.Events = append(trace.Events, core.TraceEvent{
					ID:        evt.ID,
					Type:      evt.Type,
					Actor:     evt.Actor,
					Target:    evt.Target,
					Canonical: evt.Canonical,
					Branch:    evt.Branch,
					CreatedAt: evt.CreatedAt,
				})
			}
			if actionFrame.Action == "attack" || actionFrame.Action == "threaten" {
				e.tensionEng.ResetConflictTimer(turnNumber)
			}
		}
	}

	if narrative != "" {
		assistantMsg := core.Message{Role: "assistant", Content: narrative}
		e.memEngine.PushDialogue(assistantMsg, step.Speaker)
		e.mu.Lock()
		e.dialogueHistory = append(e.dialogueHistory, assistantMsg)
		e.mu.Unlock()
	}

	go e.extractAndStoreFactsFor(step.Speaker, narrative, turnNumber)
	return trace
}

func convertMemories(memories []core.Memory) ([]core.FactFrame, []core.EventFrame, []core.TraceMemory) {
	semanticFacts := make([]core.FactFrame, 0)
	episodicEvents := make([]core.EventFrame, 0)
	traceMemories := make([]core.TraceMemory, 0, len(memories))
	for _, m := range memories {
		traceMemories = append(traceMemories, core.TraceMemory{
			Type:    m.Type,
			Content: m.Content,
			Score:   m.Score,
		})
		switch m.Type {
		case "semantic":
			parts := strings.Split(m.Content, " ")
			if len(parts) >= 3 {
				semanticFacts = append(semanticFacts, core.FactFrame{
					Subject:    parts[0],
					Predicate:  parts[1],
					Object:     strings.Join(parts[2:], " "),
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
	return semanticFacts, episodicEvents, traceMemories
}

func traceGoalsFromFrames(goals []core.GoalFrame) []core.TraceGoal {
	out := make([]core.TraceGoal, 0, len(goals))
	for _, goal := range goals {
		out = append(out, core.TraceGoal{
			ID:        goal.ID,
			Type:      goal.Type,
			Priority:  goal.Priority,
			Condition: goal.Condition,
		})
	}
	return out
}

func traceFactsFromFrames(facts []core.FactFrame) []core.TraceFact {
	out := make([]core.TraceFact, 0, len(facts))
	for _, fact := range facts {
		out = append(out, core.TraceFact{
			Subject:   fact.Subject,
			Predicate: fact.Predicate,
			Object:    fact.Object,
			Score:     fact.Confidence,
		})
	}
	return out
}

func composeTurnPrompt(basePrompt string, step core.TurnStep, playerName string, handoff *core.StepHandoff) string {
	var b strings.Builder
	b.WriteString(basePrompt)
	if !strings.HasSuffix(basePrompt, "\n") {
		b.WriteString("\n")
	}
	b.WriteString("\n=== 回合职责 ===\n")
	b.WriteString(fmt.Sprintf("当前 step: #%d | speaker=%s | kind=%s | budget=%s\n", step.Index+1, step.Speaker, step.Kind, step.BudgetMode))
	for _, line := range stepPromptDirectives(step, playerName) {
		b.WriteString("- ")
		b.WriteString(line)
		b.WriteString("\n")
	}
	if handoff != nil {
		b.WriteString("\n=== 上一步交接 ===\n")
		b.WriteString(fmt.Sprintf("from: %s | step=%d | kind=%s\n", handoff.FromSpeaker, handoff.StepIndex+1, handoff.Kind))
		if handoff.Action != "" {
			b.WriteString(fmt.Sprintf("action: %s", handoff.Action))
			if handoff.Target != "" {
				b.WriteString(fmt.Sprintf(" -> %s", handoff.Target))
			}
			b.WriteString("\n")
		}
		if handoff.OutcomeSummary != "" {
			b.WriteString(fmt.Sprintf("summary: %s\n", handoff.OutcomeSummary))
		}
		if handoff.Narrative != "" {
			b.WriteString(fmt.Sprintf("narrative: %s\n", truncateForPrompt(handoff.Narrative, 220)))
		}
		if len(handoff.Events) > 0 {
			b.WriteString("events:\n")
			for _, evt := range handoff.Events[:minInt(len(handoff.Events), 4)] {
				b.WriteString(fmt.Sprintf("- %s", evt.Type))
				if evt.Target != "" {
					b.WriteString(fmt.Sprintf(" -> %s", evt.Target))
				}
				b.WriteString("\n")
			}
		}
		b.WriteString("你必须把这段交接当成当前 step 的直接前情，不要忽略。\n")
	}
	return b.String()
}

func stepPromptDirectives(step core.TurnStep, playerName string) []string {
	if strings.TrimSpace(playerName) == "" {
		playerName = "用户"
	}
	base := []string{
		"你只代表当前 speaker 发言，不替其他在场角色代说。",
		"叙事必须与当前 step 的职责一致，不要越权完成其他角色的反应。",
	}
	switch step.Kind {
	case "lead":
		base = append(base,
			fmt.Sprintf("这一拍由你正面回应 %s 的输入，必须直接接住对话主问题。", playerName),
			"优先推进主叙事，不要把篇幅浪费在旁支角色反应上。",
		)
	case "addressed_reply":
		base = append(base,
			fmt.Sprintf("你是被 %s 明确点名的补充回应者，先短促回应被点名内容，再允许补一小段态度或动作。", playerName),
			"篇幅应短于 lead，不要抢走主导权。",
		)
	case "support_response":
		base = append(base,
			"你的职责是补充你与 lead 的关系、态度或站位，不要重说 lead 已经说过的信息。",
			"优先输出立场、情绪余波、维护/拆台，而不是重新开题。",
		)
	case "tension_response":
		base = append(base,
			"当前是高张力反应位，你的输出必须体现紧张、对抗、戒备或降压后的余波。",
			"允许更强动作，但不要脱离当前场景冲突。",
		)
	case "followup":
		base = append(base,
			"这是 lead 之后的顺承补位，补充新信息或明确立场，不重复 lead 的主体叙述。",
		)
	default:
		base = append(base, "按当前场景自然回应，但保持 step 职责边界。")
	}
	allowed := filterAllowedActionsForStep(step, []string{"speak", "trust", "negotiate", "move", "hide", "threaten", "attack"})
	if len(allowed) > 0 {
		base = append(base, fmt.Sprintf("这一拍优先使用这些动作：%s。", strings.Join(allowed, ", ")))
	}
	return base
}

func filterAllowedActionsForStep(step core.TurnStep, allowed []string) []string {
	preferred := stepPreferredActions(step)
	if len(preferred) == 0 {
		return append([]string(nil), allowed...)
	}
	allowedSet := make(map[string]bool, len(allowed))
	for _, action := range allowed {
		allowedSet[action] = true
	}
	var filtered []string
	for _, action := range preferred {
		if allowedSet[action] {
			filtered = append(filtered, action)
		}
	}
	if len(filtered) > 0 {
		return filtered
	}
	return append([]string(nil), allowed...)
}

func stepPreferredActions(step core.TurnStep) []string {
	switch step.Kind {
	case "lead":
		return []string{"speak", "negotiate", "trust", "move", "hide", "threaten", "attack"}
	case "addressed_reply":
		return []string{"speak", "trust", "negotiate"}
	case "support_response":
		return []string{"trust", "speak", "negotiate"}
	case "tension_response":
		return []string{"threaten", "hide", "attack", "speak", "negotiate"}
	case "followup":
		return []string{"speak", "trust", "negotiate"}
	default:
		return nil
	}
}

func normalizeActionForStep(step core.TurnStep, frame core.ActionFrame, allowed []string) core.ActionFrame {
	if frame.Action == "" {
		return frame
	}
	for _, action := range allowed {
		if frame.Action == action {
			return frame
		}
	}
	if containsString(allowed, "speak") {
		frame.Action = "speak"
		frame.Intensity = 1
		if frame.Intent == "" {
			frame.Intent = step.Kind
		}
		return frame
	}
	if len(allowed) > 0 {
		frame.Action = allowed[0]
		frame.Intensity = 1
		if frame.Intent == "" {
			frame.Intent = step.Kind
		}
	}
	return frame
}

func buildStepHandoff(trace core.TurnStepTrace) *core.StepHandoff {
	if trace.Character == "" {
		return nil
	}
	return &core.StepHandoff{
		FromSpeaker:    trace.Character,
		StepIndex:      trace.Step.Index,
		Kind:           trace.Step.Kind,
		Action:         trace.ActionFrame.Action,
		Target:         trace.ActionFrame.Target,
		OutcomeSummary: summarizeStepOutcome(trace),
		Narrative:      truncateForPrompt(trace.Narrative, 220),
		Events:         append([]core.TraceEvent(nil), trace.Events...),
	}
}

func summarizeStepOutcome(trace core.TurnStepTrace) string {
	parts := make([]string, 0, 3)
	if trace.ActionFrame.Action != "" {
		part := trace.ActionFrame.Action
		if trace.ActionFrame.Target != "" {
			part += "->" + trace.ActionFrame.Target
		}
		parts = append(parts, part)
	}
	if len(trace.Events) > 0 {
		evs := make([]string, 0, minInt(len(trace.Events), 3))
		for _, evt := range trace.Events[:minInt(len(trace.Events), 3)] {
			evs = append(evs, evt.Type)
		}
		parts = append(parts, "events:"+strings.Join(evs, ","))
	}
	if trace.Validator.Blocked {
		parts = append(parts, "validator_blocked")
	}
	if len(parts) == 0 {
		return "no committed outcome"
	}
	return strings.Join(parts, " | ")
}

func cloneStepHandoff(handoff *core.StepHandoff) *core.StepHandoff {
	if handoff == nil {
		return nil
	}
	cp := *handoff
	cp.Events = append([]core.TraceEvent(nil), handoff.Events...)
	return &cp
}

func truncateForPrompt(text string, limit int) string {
	clean := strings.TrimSpace(strings.ReplaceAll(text, "\n", " "))
	if len(clean) <= limit {
		return clean
	}
	return clean[:limit] + "..."
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
