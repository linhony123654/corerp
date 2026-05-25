package llm

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// UsageRecord is one LLM call's token consumption.
type UsageRecord struct {
	Timestamp       string `json:"timestamp"`
	Model           string `json:"model"`
	Task            string `json:"task"`
	PromptTokens    int    `json:"prompt_tokens"`
	CompletionTokens int   `json:"completion_tokens"`
	TotalTokens     int    `json:"total_tokens"`
}

// UsageLogger appends usage records to a JSONL file.
type UsageLogger struct {
	mu   sync.Mutex
	path string
}

var globalLogger *UsageLogger

func InitUsageLogger(path string) {
	globalLogger = &UsageLogger{path: path}
}

// Log records a single LLM call.
func LogUsage(task, model string, promptTokens, completionTokens int) {
	if globalLogger == nil {
		return
	}
	globalLogger.mu.Lock()
	defer globalLogger.mu.Unlock()

	rec := UsageRecord{
		Timestamp:        time.Now().Format(time.RFC3339),
		Model:            model,
		Task:             task,
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      promptTokens + completionTokens,
	}

	f, err := os.OpenFile(globalLogger.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()

	data, _ := json.Marshal(rec)
	f.Write(data)
	f.Write([]byte{'\n'})
}

// UsageStats computes aggregated statistics from the log file.
type UsageStats struct {
	TotalCalls        int            `json:"total_calls"`
	TotalPromptTokens int            `json:"total_prompt_tokens"`
	TotalCompTokens   int            `json:"total_completion_tokens"`
	TotalTokens       int            `json:"total_tokens"`
	ByTask            map[string]TaskStats `json:"by_task"`
	ByModel           map[string]TaskStats `json:"by_model"`
	Records           []UsageRecord  `json:"-"`
}

// TaskStats holds aggregated counts for a task or model.
type TaskStats struct {
	Calls            int `json:"calls"`
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ReadUsageStats reads the log file and computes statistics.
func ReadUsageStats(path string) (*UsageStats, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &UsageStats{ByTask: map[string]TaskStats{}, ByModel: map[string]TaskStats{}}, nil
		}
		return nil, err
	}

	stats := &UsageStats{
		ByTask:  make(map[string]TaskStats),
		ByModel: make(map[string]TaskStats),
	}

	lines := splitLines(string(data))
	for _, line := range lines {
		if line == "" {
			continue
		}
		var rec UsageRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			continue
		}
		stats.Records = append(stats.Records, rec)
		stats.TotalCalls++
		stats.TotalPromptTokens += rec.PromptTokens
		stats.TotalCompTokens += rec.CompletionTokens
		stats.TotalTokens += rec.TotalTokens

		t := stats.ByTask[rec.Task]
		t.Calls++
		t.PromptTokens += rec.PromptTokens
		t.CompletionTokens += rec.CompletionTokens
		t.TotalTokens += rec.TotalTokens
		stats.ByTask[rec.Task] = t

		m := stats.ByModel[rec.Model]
		m.Calls++
		m.PromptTokens += rec.PromptTokens
		m.CompletionTokens += rec.CompletionTokens
		m.TotalTokens += rec.TotalTokens
		stats.ByModel[rec.Model] = m
	}

	return stats, nil
}

// EstimatedCost returns a rough cost estimate (DeepSeek pricing).
func (s *UsageStats) EstimatedCost() string {
	// DeepSeek-V3 pricing (approx):
	// Prompt:  ¥1 / 1M tokens
	// Completion: ¥4 / 1M tokens
	promptCost := float64(s.TotalPromptTokens) / 1_000_000 * 1.0
	compCost := float64(s.TotalCompTokens) / 1_000_000 * 4.0
	total := promptCost + compCost
	return fmt.Sprintf("¥%.4f (prompt ¥%.4f + completion ¥%.4f)", total, promptCost, compCost)
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i, c := range s {
		if c == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}
