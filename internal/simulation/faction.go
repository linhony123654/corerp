package simulation

import (
	"fmt"
	"strings"
	"time"

	"corerp/internal/core"
)

// FactionEngine tracks dynamic tension between world factions.
type FactionEngine struct {
	tensions  map[string]float64            // faction_id -> tension [0,1]
	relations map[string]map[string]float64 // faction_id -> other_id -> relation [-1,1]
	parsed    bool
}

func NewFactionEngine() *FactionEngine {
	return &FactionEngine{
		tensions:  make(map[string]float64),
		relations: make(map[string]map[string]float64),
	}
}

func (f *FactionEngine) Tick(structure core.WorldStructureConfig, state core.WorldState, tick int) []core.Event {
	if len(structure.Factions) == 0 {
		return nil
	}

	// Lazy-parse relationships on first tick with factions
	if !f.parsed {
		f.parseRelations(structure)
		f.parsed = true
	}

	now := time.Now()
	var events []core.Event

	// 1. Accumulate tension from pressures targeting factions
	for _, pressure := range structure.Pressures {
		if pressure.Intensity <= 0 {
			continue
		}
		target := strings.TrimSpace(pressure.Target)
		if target == "" {
			continue
		}
		for _, fac := range structure.Factions {
			facID := strings.TrimSpace(fac.ID)
			facName := strings.TrimSpace(fac.Name)
			if facID == "" {
				continue
			}
			if target == facID || target == facName {
				boost := pressure.Intensity * 0.03
				if strings.Contains(strings.ToLower(pressure.Kind), "conflict") {
					boost += 0.05
				}
				f.tensions[facID] += boost
				if f.tensions[facID] > 1.0 {
					f.tensions[facID] = 1.0
				}
			}
		}
	}

	// 2. Natural decay for all factions
	for id := range f.tensions {
		f.tensions[id] -= 0.01
		if f.tensions[id] < 0 {
			f.tensions[id] = 0
		}
	}

	// 3. Threshold events
	for _, fac := range structure.Factions {
		facID := strings.TrimSpace(fac.ID)
		if facID == "" {
			continue
		}
		tension := f.tensions[facID]
		if tension >= 0.6 && tension < 0.8 {
			events = append(events, core.Event{
				ID:        fmt.Sprintf("evt_faction_pressure_%d_%s", now.UnixNano(), facID),
				Type:      "tension_change",
				Actor:     "system",
				Target:    facID,
				Payload:   map[string]interface{}{"delta": 0.1, "reason": "faction_pressure:" + facID},
				Canonical: true,
				Tag:       core.TagTick,
				CreatedAt: now,
			})
		}
		if tension >= 0.8 {
			events = append(events, core.Event{
				ID:        fmt.Sprintf("evt_faction_conflict_%d_%s", now.UnixNano(), facID),
				Type:      "tension_change",
				Actor:     "system",
				Target:    facID,
				Payload:   map[string]interface{}{"delta": 0.2, "reason": "faction_conflict:" + facID},
				Canonical: true,
				Tag:       core.TagTick,
				CreatedAt: now,
			})
			events = append(events, core.Event{
				ID:        fmt.Sprintf("evt_faction_conflict_var_%d_%s", now.UnixNano(), facID),
				Type:      "variable_set",
				Actor:     "system",
				Target:    facID,
				Payload:   map[string]interface{}{"key": "world.faction." + facID + ".conflict", "value": true},
				Canonical: true,
				Tag:       core.TagTick,
				CreatedAt: now,
			})
		}
	}

	// 4. Rivalry events: two hostile factions both above 0.6 tension
	highTensionFactions := make([]string, 0, len(structure.Factions))
	for _, fac := range structure.Factions {
		facID := strings.TrimSpace(fac.ID)
		if facID != "" && f.tensions[facID] >= 0.6 {
			highTensionFactions = append(highTensionFactions, facID)
		}
	}
	for i, a := range highTensionFactions {
		for _, b := range highTensionFactions[i+1:] {
			if f.isHostile(a, b) {
				events = append(events, core.Event{
					ID:        fmt.Sprintf("evt_faction_rivalry_%d_%s_%s", now.UnixNano(), a, b),
					Type:      "faction_rivalry",
					Actor:     "system",
					Target:    a,
					Payload:   map[string]interface{}{"rival": b, "reason": fmt.Sprintf("hostile_factions_high_tension:%s_vs_%s", a, b)},
					Canonical: true,
					Tag:       core.TagTick,
					CreatedAt: now,
				})
			}
		}
	}

	return events
}

func (f *FactionEngine) parseRelations(structure core.WorldStructureConfig) {
	for _, fac := range structure.Factions {
		facID := strings.TrimSpace(fac.ID)
		if facID == "" {
			continue
		}
		if f.relations[facID] == nil {
			f.relations[facID] = make(map[string]float64)
		}
		for _, rel := range fac.Relationships {
			rel = strings.TrimSpace(rel)
			if rel == "" {
				continue
			}
			score := relationScoreFromText(rel)
			for _, other := range structure.Factions {
				otherID := strings.TrimSpace(other.ID)
				if otherID == "" || otherID == facID {
					continue
				}
				if strings.Contains(rel, other.Name) || strings.Contains(rel, otherID) {
					f.relations[facID][otherID] = score
				}
			}
		}
	}
}

func relationScoreFromText(text string) float64 {
	lower := strings.ToLower(text)
	hostile := []string{"敌对", "对立", "冲突", "竞争", "hostile", "rival", "enemy", "conflict", "opposed"}
	friendly := []string{"合作", "同盟", "友好", "联盟", " allied", "friend", "cooperate", "partner", "alliance"}
	for _, w := range hostile {
		if strings.Contains(lower, w) {
			return -0.5
		}
	}
	for _, w := range friendly {
		if strings.Contains(lower, w) {
			return 0.5
		}
	}
	return 0.0
}

func (f *FactionEngine) isHostile(a, b string) bool {
	if rels, ok := f.relations[a]; ok {
		if score, ok := rels[b]; ok && score < 0 {
			return true
		}
	}
	if rels, ok := f.relations[b]; ok {
		if score, ok := rels[a]; ok && score < 0 {
			return true
		}
	}
	return false
}

// Tensions returns a copy of current faction tensions for observation.
func (f *FactionEngine) Tensions() map[string]float64 {
	out := make(map[string]float64, len(f.tensions))
	for id, v := range f.tensions {
		out[id] = v
	}
	return out
}
