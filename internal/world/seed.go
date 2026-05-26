package world

import (
	"fmt"
	"strings"

	"corerp/internal/core"
	"corerp/internal/memory"
)

func SeedMemory(mem *memory.Engine, bundle Bundle, character string) error {
	facts, episodics := ExtractMemorySeed(bundle)
	if err := mem.ReplaceSeedFacts(facts, character); err != nil {
		return err
	}
	if err := mem.ReplaceSeedEpisodics(episodics, character); err != nil {
		return err
	}
	return nil
}

func ExtractMemorySeed(bundle Bundle) ([]core.FactFrame, []core.EventFrame) {
	o := bundle.Ontology
	var facts []core.FactFrame
	var episodics []core.EventFrame

	for _, c := range o.Characters {
		content := c.Content
		for _, line := range strings.Split(content, "\n") {
			line = strings.TrimSpace(line)
			if line == "" || len(line) > 300 {
				continue
			}
			if colon := strings.IndexByte(line, ':'); colon >= 0 && colon < 40 {
				key := cleanFactKey(line[:colon])
				val := strings.TrimSpace(line[colon+1:])
				if val != "" && len(val) < 200 {
					facts = append(facts, core.FactFrame{
						Subject:    extractName(c.Name),
						Predicate:  key,
						Object:     val,
						Confidence: 1.0,
					})
				}
			}
		}
		if content != "" {
			facts = append(facts, core.FactFrame{
				Subject:    extractName(c.Name),
				Predicate:  "完整资料",
				Object:     truncateStr(content, 500),
				Confidence: 1.0,
			})
		}
	}

	addEntryFacts := func(entries []OntologyEntry, predicate string, max int) {
		for _, entry := range entries {
			facts = append(facts, core.FactFrame{
				Subject:    extractName(entry.Name),
				Predicate:  predicate,
				Object:     truncateStr(entry.Content, max),
				Confidence: 1.0,
			})
		}
	}
	addEntryFacts(o.Locations, "是", 300)
	addEntryFacts(o.Factions, "势力", 300)
	addEntryFacts(o.Items, "物品", 200)
	addEntryFacts(o.Lore, "世界观", 300)
	addEntryFacts(o.Settings, "体系设定", 300)

	for _, event := range o.Events {
		arc := event.Arc
		if arc == "" {
			arc = "事件"
		}
		episodics = append(episodics, core.EventFrame{
			EventID:         "ont_" + event.Name,
			Type:            arc,
			Description:     truncateStr(event.Content, 400),
			EmotionalWeight: 0.5,
		})
	}

	for _, entry := range o.Timelines {
		facts = append(facts, core.FactFrame{
			Subject:    "时间线",
			Predicate:  extractName(entry.Name),
			Object:     truncateStr(entry.Content, 300),
			Confidence: 1.0,
		})
	}

	facts = append(facts, bundle.DirectFacts...)
	return facts, episodics
}

func countSeed(bundle Bundle) (facts int, episodics int) {
	f, e := ExtractMemorySeed(bundle)
	return len(f), len(e)
}

func extractName(raw string) string {
	if idx := strings.Index(raw, "·"); idx >= 0 {
		raw = raw[idx+len("·"):]
	}
	return strings.TrimSpace(raw)
}

func cleanFactKey(key string) string {
	key = strings.TrimSpace(key)
	key = strings.TrimSuffix(key, "：")
	key = strings.TrimSuffix(key, ":")
	return key
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return strings.TrimSpace(s[:n]) + "..."
}

func SeedSummary(bundle Bundle) string {
	facts, episodics := countSeed(bundle)
	return fmt.Sprintf("%d facts, %d events", facts, episodics)
}
