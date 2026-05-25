package context

import (
	"fmt"
	"strings"

	"corerp/internal/core"
)

// Compiler assembles WorldSnapshot with hard token budget.
type Compiler struct {
	budget Budget
}

type Budget struct {
	Total         int
	CoreRules     int
	PersonaState  int
	SceneState    int
	WorkingMemory int
	SemanticFacts int
	EpisodicEvents int
	RecentDialogue int
}

func NewCompiler(modelTokenLimit int) *Compiler {
	// P1: hardcoded percentages
	return &Compiler{
		budget: Budget{
			Total:          modelTokenLimit,
			CoreRules:      int(float64(modelTokenLimit) * 0.05),
			PersonaState:   int(float64(modelTokenLimit) * 0.15),
			SceneState:     int(float64(modelTokenLimit) * 0.10),
			WorkingMemory:  int(float64(modelTokenLimit) * 0.15),
			SemanticFacts:  int(float64(modelTokenLimit) * 0.15),
			EpisodicEvents: int(float64(modelTokenLimit) * 0.10),
			RecentDialogue: int(float64(modelTokenLimit) * 0.30),
		},
	}
}

func (c *Compiler) Compile(
	state core.WorldState,
	persona core.PersonaFrame,
	workingMem string,
	semanticFacts []core.FactFrame,
	episodicEvents []core.EventFrame,
	dialogue []core.Message,
	goals []core.GoalFrame,
	allowedActions []string,
	coreRules string,
) (core.WorldSnapshot, error) {
	snapshot := core.WorldSnapshot{
		TokenBudget:    c.budget.Total,
		UsedTokens:     0,
		CoreRules:      c.truncate(coreRules, c.budget.CoreRules),
		PersonaState:   persona,
		SceneState:     state.Scene,
		ActiveGoals:    goals,
		WorkingMemory:  c.truncate(workingMem, c.budget.WorkingMemory),
		SemanticFacts:  c.truncateFacts(semanticFacts, c.budget.SemanticFacts),
		EpisodicEvents: c.truncateEvents(episodicEvents, c.budget.EpisodicEvents),
		RecentDialogue: c.truncateDialogue(dialogue, c.budget.RecentDialogue),
		AllowedActions: allowedActions,
	}

	snapshot.UsedTokens = c.estimateSnapshotTokens(snapshot)

	if snapshot.UsedTokens > c.budget.Total {
		return snapshot, fmt.Errorf("SNAPSHOT OVER BUDGET: used %d / %d tokens", snapshot.UsedTokens, c.budget.Total)
	}

	return snapshot, nil
}

func (c *Compiler) GoalsToFrames(goals []core.Goal) []core.GoalFrame {
	var frames []core.GoalFrame
	for _, g := range goals {
		frames = append(frames, core.GoalFrame{
			ID:       g.ID,
			Priority: g.Priority,
			Type:     g.Type,
			Target:   g.Target,
			Condition: g.Condition,
		})
	}
	return frames
}

func (c *Compiler) truncate(text string, maxTokens int) string {
	// Rough: 1 token ≈ 1.5 CJK chars or 4 ASCII chars
	maxChars := maxTokens * 3 // conservative for mixed
	if len(text) <= maxChars {
		return text
	}
	return text[:maxChars] + "..."
}

func (c *Compiler) truncateFacts(facts []core.FactFrame, maxTokens int) []core.FactFrame {
	var result []core.FactFrame
	used := 0
	for _, f := range facts {
		est := len(f.Subject+f.Predicate+f.Object)/2 + 5
		if used+est > maxTokens {
			break
		}
		result = append(result, f)
		used += est
	}
	return result
}

func (c *Compiler) truncateEvents(events []core.EventFrame, maxTokens int) []core.EventFrame {
	var result []core.EventFrame
	used := 0
	for _, e := range events {
		est := len(e.Description)/2 + 10
		if used+est > maxTokens {
			break
		}
		result = append(result, e)
		used += est
	}
	return result
}

func (c *Compiler) truncateDialogue(dialogue []core.Message, maxTokens int) []core.Message {
	var result []core.Message
	used := 0
	// Add from most recent backwards
	for i := len(dialogue) - 1; i >= 0; i-- {
		est := len(dialogue[i].Content)/2 + 5
		if used+est > maxTokens {
			break
		}
		result = append([]core.Message{dialogue[i]}, result...)
		used += est
	}
	return result
}

func (c *Compiler) estimateSnapshotTokens(s core.WorldSnapshot) int {
	total := 0
	total += len(s.CoreRules) / 2
	total += len(s.PersonaState.Name) * 2
	total += len(strings.Join(s.PersonaState.Immutable, " ")) / 2
	total += len(s.SceneState.Location+s.SceneState.Description) / 2
	total += len(s.WorkingMemory) / 2
	for _, f := range s.SemanticFacts {
		total += (len(f.Subject) + len(f.Predicate) + len(f.Object)) / 2
	}
	for _, e := range s.EpisodicEvents {
		total += len(e.Description) / 2
	}
	for _, m := range s.RecentDialogue {
		total += len(m.Content) / 2
	}
	return total
}

// RenderSnapshot converts WorldSnapshot into a prompt string for LLM.
func (c *Compiler) RenderSnapshot(s core.WorldSnapshot) string {
	var b strings.Builder

	b.WriteString("=== 世界规则 ===\n")
	b.WriteString(s.CoreRules)
	b.WriteString("\n\n")

	b.WriteString("=== 角色状态 ===\n")
	b.WriteString(fmt.Sprintf("名称: %s\n", s.PersonaState.Name))
	b.WriteString(fmt.Sprintf("不可变特质: %s\n", strings.Join(s.PersonaState.Immutable, ", ")))
	b.WriteString("自适应状态:\n")
	for k, v := range s.PersonaState.Adaptive {
		b.WriteString(fmt.Sprintf("  %s: %.1f\n", k, v))
	}
	b.WriteString(fmt.Sprintf("文风: %s\n", s.PersonaState.VoiceStyle))
	b.WriteString(fmt.Sprintf("节奏: %s\n", s.PersonaState.VoiceRhythm))
	b.WriteString(fmt.Sprintf("禁止: %s\n", strings.Join(s.PersonaState.Forbidden, ", ")))
	b.WriteString("\n")

	b.WriteString("=== 场景状态 ===\n")
	b.WriteString(fmt.Sprintf("地点: %s\n", s.SceneState.Location))
	b.WriteString(fmt.Sprintf("时间: %s\n", s.SceneState.TimeOfDay))
	b.WriteString(fmt.Sprintf("天气: %s\n", s.SceneState.Weather))
	b.WriteString(fmt.Sprintf("在场: %s\n", strings.Join(s.SceneState.Characters, ", ")))
	if s.SceneState.Description != "" {
		b.WriteString(fmt.Sprintf("描述: %s\n", s.SceneState.Description))
	}
	b.WriteString("\n")

	if len(s.ActiveGoals) > 0 {
		b.WriteString("=== 当前目标 ===\n")
		for _, g := range s.ActiveGoals {
			b.WriteString(fmt.Sprintf("[%s] %s (P%d)\n", g.Type, g.ID, g.Priority))
		}
		b.WriteString("\n")
	}

	if s.WorkingMemory != "" {
		b.WriteString("=== 场景摘要 ===\n")
		b.WriteString(s.WorkingMemory)
		b.WriteString("\n\n")
	}

	if len(s.SemanticFacts) > 0 {
		b.WriteString("=== 已知事实 ===\n")
		for _, f := range s.SemanticFacts {
			b.WriteString(fmt.Sprintf("- %s %s %s\n", f.Subject, f.Predicate, f.Object))
		}
		b.WriteString("\n")
	}

	if len(s.EpisodicEvents) > 0 {
		b.WriteString("=== 相关事件 ===\n")
		for _, e := range s.EpisodicEvents {
			b.WriteString(fmt.Sprintf("- [%s] %s\n", e.Type, e.Description))
		}
		b.WriteString("\n")
	}

	b.WriteString("=== 最近对话 ===\n")
	for _, m := range s.RecentDialogue {
		role := "你"
		if m.Role == "user" {
			role = "用户"
		}
		b.WriteString(fmt.Sprintf("%s: %s\n", role, m.Content))
	}
	b.WriteString("\n")

	b.WriteString("=== 可用动作 ===\n")
	b.WriteString(strings.Join(s.AllowedActions, ", "))
	b.WriteString("\n\n")

	actorName := s.PersonaState.Name
	actorKey := strings.ReplaceAll(actorName, " ", "_")

	b.WriteString("=== 指令 ===\n")
	b.WriteString("你必须严格按以下格式输出，不要有任何前缀说明，不要省略任何字段：\n\n")
	b.WriteString(fmt.Sprintf("1. 先输出 Action Frame，放在 ```json 代码块中。actor 必须是 \"%s\"，action 必填。effects 必须是对象数组 [{\"path\":\"...\",\"delta\":数字}]，不能是字符串数组。示例：\n", actorName))
	b.WriteString("```json\n")
	b.WriteString(fmt.Sprintf("{\"actor\":\"%s\",\"action\":\"speak\",\"target\":\"用户\",\"intensity\":5,\"emotion\":{\"primary\":\"警惕\",\"secondary\":\"冷淡\",\"intensity\":0.7},\"intent\":\"试探对方意图\",\"suggested_line\":\"什么事？\",\"effects\":[{\"path\":\"relationships.%s_用户.trust\",\"delta\":-0.5}]}\n", actorName, actorKey))
	b.WriteString("```\n\n")
	b.WriteString("2. 然后输出叙事文本，放在 ```text 代码块中。不要包含 Action Frame 的任何内容：\n")
	b.WriteString("```text\n")
	b.WriteString("（只写叙事文本，符合角色文风和节奏，禁止卖萌、打破第四面墙等被禁止的行为）\n")
	b.WriteString("```\n")
	b.WriteString("\n警告：如果 JSON 缺少 actor 或 action 字段，或 JSON 格式不合法，输出会被丢弃。\n")

	return b.String()
}
