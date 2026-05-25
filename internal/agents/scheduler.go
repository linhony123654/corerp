package agents

import (
	"fmt"
	"math/rand"
	"time"

	"corerp/internal/core"
)

// Scheduler drives autonomous actions for non-active characters.
// P3: rule-based only, no LLM — token budget stays with the active character.
type Scheduler struct {
	lastActionTick map[string]int
	npcActions     []NPCActionLog // recent action summaries
	actionInterval int            // ticks between NPC actions (default 3)
	maxSummaryLog  int
}

// NPCActionLog records a human-readable summary of an NPC action.
type NPCActionLog struct {
	Character string `json:"character"`
	Action    string `json:"action"`
	Target    string `json:"target"`
	Summary   string `json:"summary"`
	Tick      int    `json:"tick"`
	Timestamp time.Time `json:"timestamp"`
}

func NewScheduler() *Scheduler {
	return &Scheduler{
		lastActionTick: make(map[string]int),
		actionInterval: 3,
		maxSummaryLog:  50,
	}
}

// Tick processes one NPC action cycle. Returns events to commit and summaries.
func (s *Scheduler) Tick(
	characters []string,
	activeCharacter string,
	charWorlds map[string]core.SceneState,
	worldState core.WorldState,
	agentsMgr *EnvelopeManager,
	executor ActionExecutor,
	gatekeeper EventSubmitter,
	currentTick int,
) {
	for _, name := range characters {
		if name == activeCharacter {
			continue
		}
		lastTick := s.lastActionTick[name]
		if currentTick-lastTick < s.actionInterval {
			continue
		}
		s.lastActionTick[name] = currentTick

		char, ok := agentsMgr.GetCharacter(name)
		if !ok {
			continue
		}

		// Build NPC-local state snapshot using their own world scene
		npcScene := charWorlds[name]
		npcState := worldState
		if npcScene.Location != "" {
			npcState.Scene = npcScene
		}

		goals := agentsMgr.ActiveGoals(name, npcState)
		planner := NewPlanner()
		steps := planner.Plan(name, npcState, goals, "")
		if len(steps) == 0 {
			continue
		}

		// Pick highest-priority step
		best := steps[0]
		for _, s := range steps {
			if s.Priority > best.Priority {
				best = s
			}
		}
		// Add noise: 20% chance to pick a random step for variety
		if rand.Float64() < 0.2 && len(steps) > 1 {
			best = steps[rand.Intn(len(steps))]
		}

		// Build ActionFrame
		frame := core.ActionFrame{
			Actor:  name,
			Action: best.Action,
			Target: best.Target,
			Intensity: best.Priority,
			Emotion: core.EmotionState{
				Primary:   "neutral",
				Intensity: 0.3,
			},
			Intent: best.Reason,
		}

		// Execute
		evts, err := executor.Execute(frame, npcState)
		if err != nil {
			continue
		}

		// Submit events through gatekeeper
		for _, evt := range evts {
			evt.Actor = name
			gatekeeper.Submit(evt, "npc_scheduler:"+name)
		}

		// Build human-readable summary
		summary := s.buildSummary(name, best, char)
		s.npcActions = append(s.npcActions, NPCActionLog{
			Character: name,
			Action:    best.Action,
			Target:    best.Target,
			Summary:   summary,
			Tick:      currentTick,
			Timestamp: time.Now(),
		})

		// Trim log
		if len(s.npcActions) > s.maxSummaryLog {
			s.npcActions = s.npcActions[len(s.npcActions)-s.maxSummaryLog:]
		}
	}
}

func (s *Scheduler) buildSummary(name string, step PlanStep, char core.Character) string {
	switch step.Action {
	case "hide":
		return fmt.Sprintf("%s 躲藏了起来，保持警觉。", name)
	case "move":
		return fmt.Sprintf("%s 移动到了新的位置。", name)
	case "observe":
		return fmt.Sprintf("%s 观察着周围的环境。", name)
	case "trust":
		target := step.Target
		if target == "" {
			target = "某人"
		}
		return fmt.Sprintf("%s 尝试与%s建立信任。", name, target)
	case "speak":
		return fmt.Sprintf("%s 开口说话，打破了沉默。", name)
	case "attack":
		return fmt.Sprintf("%s 发起了攻击！", name)
	case "threaten":
		return fmt.Sprintf("%s 发出威胁。", name)
	default:
		return fmt.Sprintf("%s 做了一些事（%s）。", name, step.Action)
	}
}

// RecentActions returns summaries since the given tick.
func (s *Scheduler) RecentActions(sinceTick int) []NPCActionLog {
	var result []NPCActionLog
	for _, a := range s.npcActions {
		if a.Tick >= sinceTick {
			result = append(result, a)
		}
	}
	return result
}

// RecentActionsForCharacter returns recent summaries for a specific character.
func (s *Scheduler) RecentActionsForCharacter(name string, sinceTick int) []NPCActionLog {
	var result []NPCActionLog
	for _, a := range s.npcActions {
		if a.Character == name && a.Tick >= sinceTick {
			result = append(result, a)
		}
	}
	return result
}

// ActionExecutor is the interface the scheduler needs from actions.Executor.
type ActionExecutor interface {
	Execute(frame core.ActionFrame, state core.WorldState) ([]core.Event, error)
}

// EventSubmitter is the interface the scheduler needs from the gatekeeper.
type EventSubmitter interface {
	Submit(evt core.Event, source string) error
}
