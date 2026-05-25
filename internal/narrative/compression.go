package narrative

import (
	"fmt"
	"strings"
	"time"

	"corerp/internal/core"
)

// CompressionEngine condenses large numbers of low-level events into
// higher-level narrative summaries. Rule-based only — no LLM tokens spent.
type CompressionEngine struct {
	store         EventStore
	maxEvents     int // auto-compress when canonical events exceed this
	minAge        time.Duration // only compress events older than this
	compressedIDs map[string]bool // track which events have been summarized
}

// EventStore is the subset of Store methods the compressor needs.
type EventStore interface {
	GetCanonicalEvents() ([]core.Event, error)
	Append(e core.Event) error
}

func NewCompressionEngine(store EventStore) *CompressionEngine {
	return &CompressionEngine{
		store:         store,
		maxEvents:     500,
		minAge:        10 * time.Minute,
		compressedIDs: make(map[string]bool),
	}
}

// CompressionResult captures the result of a compression pass.
type CompressionResult struct {
	GroupsFound    int            `json:"groups_found"`
	EventsCompressed int          `json:"events_compressed"`
	Summaries      []CompressGroup `json:"summaries"`
}

// CompressGroup is one compressed group of similar events.
type CompressGroup struct {
	Type       string   `json:"type"`
	Count      int      `json:"count"`
	Summary    string   `json:"summary"`
	EventIDs   []string `json:"event_ids"`
	TimeRange  string   `json:"time_range"`
}

// AutoCompress checks if the event store needs compression and applies it.
// Returns the number of events compressed.
func (c *CompressionEngine) AutoCompress() (*CompressionResult, error) {
	events, err := c.store.GetCanonicalEvents()
	if err != nil {
		return nil, err
	}

	// Filter: only consider events not yet compressed and older than minAge
	var compressible []core.Event
	for _, e := range events {
		if c.compressedIDs[e.ID] {
			continue
		}
		if time.Since(e.CreatedAt) < c.minAge {
			continue
		}
		// Only compress "small" event types — never compress user messages or critical events
		if !isCompressible(e.Type) {
			continue
		}
		compressible = append(compressible, e)
	}

	if len(compressible) < c.maxEvents {
		return &CompressionResult{}, nil
	}

	// Group by type
	groups := groupByType(compressible)
	var summaries []CompressGroup
	totalCompressed := 0

	for _, group := range groups {
		if len(group) < 3 {
			continue // Don't bother compressing groups smaller than 3
		}
		cg := c.buildCompressGroup(group)
		if cg.Count == 0 {
			continue
		}
		summaries = append(summaries, cg)

		// Mark original events as compressed
		for _, id := range cg.EventIDs {
			c.compressedIDs[id] = true
		}
		totalCompressed += cg.Count

		// Store summary as a new canonical event
		summaryEvent := core.Event{
			ID:        fmt.Sprintf("compress_%d", time.Now().UnixNano()),
			Type:      "narrative_compression",
			Actor:     "system",
			Payload: map[string]interface{}{
				"compressed_type":  group[0].Type,
				"summary":         cg.Summary,
				"original_count":  cg.Count,
				"original_ids":    cg.EventIDs,
				"time_range":      cg.TimeRange,
			},
			Canonical: true,
			Confidence: 1.0,
			CreatedAt: time.Now(),
		}
		c.store.Append(summaryEvent)
	}

	return &CompressionResult{
		GroupsFound:    len(summaries),
		EventsCompressed: totalCompressed,
		Summaries:      summaries,
	}, nil
}

// CompressRange manually compresses a specific range of events.
func (c *CompressionEngine) CompressRange(from, to int) (*CompressionResult, error) {
	events, err := c.store.GetCanonicalEvents()
	if err != nil {
		return nil, err
	}

	if from < 0 || to > len(events) || from >= to {
		return nil, fmt.Errorf("invalid range [%d, %d) for %d events", from, to, len(events))
	}

	slice := events[from:to]
	groups := groupByType(slice)
	var summaries []CompressGroup
	totalCompressed := 0

	for _, group := range groups {
		if len(group) < 2 {
			continue
		}
		cg := c.buildCompressGroup(group)
		if cg.Count == 0 {
			continue
		}
		summaries = append(summaries, cg)
		for _, id := range cg.EventIDs {
			c.compressedIDs[id] = true
		}
		totalCompressed += cg.Count

		summaryEvent := core.Event{
			ID:        fmt.Sprintf("compress_%d", time.Now().UnixNano()),
			Type:      "narrative_compression",
			Actor:     "system",
			Payload: map[string]interface{}{
				"compressed_type": group[0].Type,
				"summary":        cg.Summary,
				"original_count": cg.Count,
				"original_ids":   cg.EventIDs,
				"time_range":     cg.TimeRange,
			},
			Canonical: true,
			Confidence: 1.0,
			CreatedAt: time.Now(),
		}
		c.store.Append(summaryEvent)
	}

	return &CompressionResult{
		GroupsFound:     len(summaries),
		EventsCompressed: totalCompressed,
		Summaries:       summaries,
	}, nil
}

// SummaryStats returns compression statistics.
func (c *CompressionEngine) SummaryStats() map[string]interface{} {
	events, err := c.store.GetCanonicalEvents()
	if err != nil {
		return map[string]interface{}{"error": err.Error()}
	}

	total := len(events)
	compressed := len(c.compressedIDs)
	active := total - compressed

	// Count summary events
	var summaryCount int
	for _, e := range events {
		if e.Type == "narrative_compression" {
			summaryCount++
		}
	}

	return map[string]interface{}{
		"total_events":      total,
		"active_events":     active,
		"compressed_events": compressed,
		"summary_events":    summaryCount,
		"max_threshold":     c.maxEvents,
	}
}

// --- internal helpers ---

func (c *CompressionEngine) buildCompressGroup(group []core.Event) CompressGroup {
	if len(group) == 0 {
		return CompressGroup{}
	}

	cg := CompressGroup{
		Type:     group[0].Type,
		Count:    len(group),
		EventIDs: make([]string, len(group)),
		TimeRange: fmt.Sprintf("%s ~ %s",
			group[0].CreatedAt.Format("15:04"),
			group[len(group)-1].CreatedAt.Format("15:04"),
		),
	}

	for i, e := range group {
		cg.EventIDs[i] = e.ID
	}

	cg.Summary = c.summarize(group)
	return cg
}

func (c *CompressionEngine) summarize(group []core.Event) string {
	n := len(group)
	first := group[0]
	last := group[len(group)-1]

	switch first.Type {
	case "observe", "hide":
		if n > 5 {
			return fmt.Sprintf("在%s长时间保持警觉，持续观察周围环境。", first.SceneID)
		}
		return fmt.Sprintf("在%s观察了周围环境%d次。", first.SceneID, n)

	case "dialogue":
		actors := collectActors(group)
		if len(actors) > 1 {
			return fmt.Sprintf("与%s进行了%d轮对话。", strings.Join(actors, "、"), n)
		}
		return fmt.Sprintf("进行了%d轮对话交流。", n)

	case "move", "scene_change":
		if first.SceneID != last.SceneID {
			return fmt.Sprintf("从%s移动到了%s。", first.SceneID, last.SceneID)
		}
		return fmt.Sprintf("移动了%d次。", n)

	case "clock_advance":
		return fmt.Sprintf("时间流逝，世界时钟推进了约%d步。", n)

	case "tension_change":
		return fmt.Sprintf("世界张力经历了%d次波动。", n)

	case "trust_change", "fear_change", "intimacy_change":
		return fmt.Sprintf("角色关系经历了%d次微妙变化。", n)

	case "npc_action":
		return fmt.Sprintf("自动进行了%d次后台行动。", n)

	case "variable_set":
		return fmt.Sprintf("世界状态发生了%d次参数调整。", n)

	default:
		return fmt.Sprintf("发生了%d个%s类型的事件。", n, first.Type)
	}
}

func groupByType(events []core.Event) [][]core.Event {
	if len(events) == 0 {
		return nil
	}

	var groups [][]core.Event
	currentType := events[0].Type
	currentGroup := []core.Event{events[0]}

	for _, e := range events[1:] {
		if e.Type == currentType {
			currentGroup = append(currentGroup, e)
		} else {
			groups = append(groups, currentGroup)
			currentType = e.Type
			currentGroup = []core.Event{e}
		}
	}
	groups = append(groups, currentGroup)
	return groups
}

func collectActors(events []core.Event) []string {
	seen := make(map[string]bool)
	var actors []string
	for _, e := range events {
		if e.Actor != "" && e.Actor != "system" && !seen[e.Actor] {
			seen[e.Actor] = true
			actors = append(actors, e.Actor)
		}
	}
	return actors
}

// isCompressible returns true for event types that can be safely compressed.
func isCompressible(typ string) bool {
	switch typ {
	case "observe", "hide", "move", "clock_advance",
		"dialogue", "tension_change", "trust_change",
		"fear_change", "intimacy_change", "variable_set",
		"npc_action", "scene_change":
		return true
	}
	return false
}
