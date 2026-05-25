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
	Timestamp        string `json:"timestamp"`
	Model            string `json:"model"`
	Task             string `json:"task"`
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	TotalTokens      int    `json:"total_tokens"`
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
	TotalCalls        int                  `json:"total_calls"`
	TotalPromptTokens int                  `json:"total_prompt_tokens"`
	TotalCompTokens   int                  `json:"total_completion_tokens"`
	TotalTokens       int                  `json:"total_tokens"`
	ByTask            map[string]TaskStats `json:"by_task"`
	ByModel           map[string]TaskStats `json:"by_model"`
	ByDay             map[string]TaskStats `json:"by_day"`
	ByWeek            map[string]TaskStats `json:"by_week"`
	ByMonth           map[string]TaskStats `json:"by_month"`
	Records           []UsageRecord        `json:"-"`
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
			return &UsageStats{
				ByTask: map[string]TaskStats{}, ByModel: map[string]TaskStats{},
				ByDay: map[string]TaskStats{}, ByWeek: map[string]TaskStats{}, ByMonth: map[string]TaskStats{},
			}, nil
		}
		return nil, err
	}

	stats := &UsageStats{
		ByTask:  make(map[string]TaskStats), ByModel: make(map[string]TaskStats),
		ByDay:   make(map[string]TaskStats), ByWeek:  make(map[string]TaskStats),
		ByMonth: make(map[string]TaskStats),
	}

	for _, line := range splitLines(string(data)) {
		if line == "" {
			continue
		}

		// Handle monthly rollup lines
		var rollup struct {
			Type             string `json:"type"`
			Month            string `json:"month"`
			TotalCalls       int    `json:"total_calls"`
			TotalTokens      int    `json:"total_tokens"`
			PromptTokens     int    `json:"prompt_tokens"`
			CompletionTokens int    `json:"completion_tokens"`
		}
		if json.Unmarshal([]byte(line), &rollup) == nil && rollup.Type == "monthly_rollup" {
			stats.TotalCalls += rollup.TotalCalls
			stats.TotalPromptTokens += rollup.PromptTokens
			stats.TotalCompTokens += rollup.CompletionTokens
			stats.TotalTokens += rollup.TotalTokens
			rm := stats.ByMonth[rollup.Month]
			rm.Calls += rollup.TotalCalls
			rm.PromptTokens += rollup.PromptTokens
			rm.CompletionTokens += rollup.CompletionTokens
			rm.TotalTokens += rollup.TotalTokens
			stats.ByMonth[rollup.Month] = rm
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

		ts, _ := time.Parse(time.RFC3339, rec.Timestamp)
		dayKey := ts.Format("2006-01-02")
		weekYear, weekNum := ts.ISOWeek()
		weekKey := fmt.Sprintf("%d-W%02d", weekYear, weekNum)
		monthKey := ts.Format("2006-01")
		stats.ByDay[dayKey] = addStats(stats.ByDay[dayKey], rec)
		stats.ByWeek[weekKey] = addStats(stats.ByWeek[weekKey], rec)
		stats.ByMonth[monthKey] = addStats(stats.ByMonth[monthKey], rec)
	}
	return stats, nil
}

// CompactMonth rolls up all completed months into summary lines.
// Only the current month keeps detailed records for daily/weekly drill-down.
func CompactMonth(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	now := time.Now()
	thisMonth := now.Format("2006-01")

	type monthAgg struct {
		calls, prompt, comp int
		lines               []string
	}
	byMonth := make(map[string]*monthAgg)
	var existingRollups []string

	for _, line := range splitLines(string(data)) {
		if line == "" {
			continue
		}
		var rollup struct {
			Type  string `json:"type"`
			Month string `json:"month"`
		}
		if json.Unmarshal([]byte(line), &rollup) == nil && rollup.Type == "monthly_rollup" {
			if rollup.Month != thisMonth {
				existingRollups = append(existingRollups, line)
			}
			continue
		}
		var rec UsageRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			continue
		}
		ts, err := time.Parse(time.RFC3339, rec.Timestamp)
		if err != nil {
			continue
		}
		recMonth := ts.Format("2006-01")
		if recMonth == thisMonth {
			if byMonth[recMonth] == nil {
				byMonth[recMonth] = &monthAgg{}
			}
			byMonth[recMonth].lines = append(byMonth[recMonth].lines, line)
		} else {
			if byMonth[recMonth] == nil {
				byMonth[recMonth] = &monthAgg{}
			}
			byMonth[recMonth].calls++
			byMonth[recMonth].prompt += rec.PromptTokens
			byMonth[recMonth].comp += rec.CompletionTokens
		}
	}

	// No past months to compact
	hasPastMonth := false
	for m := range byMonth {
		if m != thisMonth {
			hasPastMonth = true
			break
		}
	}
	if !hasPastMonth {
		return nil
	}

	var out []string
	out = append(out, existingRollups...)
	for month, agg := range byMonth {
		if month == thisMonth || agg.calls == 0 {
			continue
		}
		summary := map[string]interface{}{
			"type":              "monthly_rollup",
			"month":             month,
			"total_calls":       agg.calls,
			"prompt_tokens":     agg.prompt,
			"completion_tokens": agg.comp,
			"total_tokens":      agg.prompt + agg.comp,
		}
		js, _ := json.Marshal(summary)
		out = append(out, string(js))
	}
	if byMonth[thisMonth] != nil {
		out = append(out, byMonth[thisMonth].lines...)
	}

	return os.WriteFile(path, []byte(joinLines(out)), 0644)
}

// EstimatedCost returns a rough cost estimate (DeepSeek pricing).
func (s *UsageStats) EstimatedCost() string {
	promptPrice, compPrice := GetPricing()
	promptCost := float64(s.TotalPromptTokens) / 1_000_000 * promptPrice
	compCost := float64(s.TotalCompTokens) / 1_000_000 * compPrice
	return fmt.Sprintf("¥%.4f (prompt ¥%.4f + completion ¥%.4f)", promptCost+compCost, promptCost, compCost)
}

// --- helpers ---

func addStats(t TaskStats, rec UsageRecord) TaskStats {
	t.Calls++
	t.PromptTokens += rec.PromptTokens
	t.CompletionTokens += rec.CompletionTokens
	t.TotalTokens += rec.TotalTokens
	return t
}

func joinLines(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	var b []byte
	for i, l := range lines {
		if i > 0 {
			b = append(b, '\n')
		}
		b = append(b, []byte(l)...)
	}
	return string(b)
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
