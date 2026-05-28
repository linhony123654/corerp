package simulation

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"corerp/internal/core"
)

// PulseEngine turns world pressures into tick-driven runtime events.
type PulseEngine struct {
	lastTriggered  map[string]int
	maxPerTick     int
	pressureStates map[string]*pressureState
}

type pressureState struct {
	CurrentIntensity float64
	TriggerCount     int
	MissedTicks      int
	CooldownTicks    int // hysteresis: minimum ticks before next trigger
}

func NewPulseEngine() *PulseEngine {
	return &PulseEngine{
		lastTriggered:  make(map[string]int),
		maxPerTick:     1,
		pressureStates: make(map[string]*pressureState),
	}
}

func (p *PulseEngine) Tick(structure core.WorldStructureConfig, state core.WorldState, tick int) []core.Event {
	if len(structure.Pressures) == 0 {
		return nil
	}

	// Update states for all pressures (triggered or not)
	for _, pressure := range structure.Pressures {
		if pressure.ID == "" {
			continue
		}
		ps := p.ensurePressureState(pressure.ID)
		if ps.CooldownTicks > 0 {
			ps.CooldownTicks--
		}
		cadence := pressureCadence(ps.CurrentIntensity)
		lastTick, seen := p.lastTriggered[pressure.ID]
		if seen && tick-lastTick < cadence {
			// Not eligible this tick — increment missed
			ps.MissedTicks++
			if ps.MissedTicks >= 3 {
				ps.CurrentIntensity -= 0.02
				if ps.CurrentIntensity < 0.1 {
					ps.CurrentIntensity = 0.1
				}
				ps.MissedTicks = 0
			}
		} else {
			// Eligible — will be handled below
		}
	}

	eligible := make([]core.WorldPressureConfig, 0, len(structure.Pressures))
	for _, pressure := range structure.Pressures {
		if pressure.ID == "" || pressure.Intensity <= 0 {
			continue
		}
		ps := p.ensurePressureState(pressure.ID)
		if ps.CurrentIntensity <= 0 {
			continue
		}
		cadence := pressureCadence(ps.CurrentIntensity)
		lastTick, seen := p.lastTriggered[pressure.ID]
		if seen && tick-lastTick < cadence {
			continue
		}
		if ps.CooldownTicks > 0 {
			continue
		}
		// Use dynamic intensity for eligibility sorting
		dyn := pressure
		dyn.Intensity = ps.CurrentIntensity
		eligible = append(eligible, dyn)
	}
	if len(eligible) == 0 {
		return nil
	}

	sort.Slice(eligible, func(i, j int) bool {
		if eligible[i].Intensity == eligible[j].Intensity {
			return eligible[i].ID < eligible[j].ID
		}
		return eligible[i].Intensity > eligible[j].Intensity
	})

	if len(eligible) > p.maxPerTick {
		eligible = eligible[:p.maxPerTick]
	}

	now := time.Now()
	events := make([]core.Event, 0, len(eligible)*3+2)

	// Build ID lookup for escalate targets
	pressureByID := make(map[string]core.WorldPressureConfig, len(structure.Pressures))
	for _, pr := range structure.Pressures {
		pressureByID[pr.ID] = pr
	}

	for _, pressure := range eligible {
		ps := p.ensurePressureState(pressure.ID)
		p.lastTriggered[pressure.ID] = tick
		ps.TriggerCount++
		ps.MissedTicks = 0
		ps.CooldownTicks = 2 // hysteresis: prevent rapid re-triggering

		// Escalation: intensity boost after consecutive triggers
		if ps.TriggerCount >= 3 {
			ps.CurrentIntensity += 0.03
			if ps.CurrentIntensity > 0.95 {
				ps.CurrentIntensity = 0.95
			}
			ps.TriggerCount = 0
		}

		reason := "world_pressure:" + pressure.ID
		delta := pressureDelta(ps.CurrentIntensity, state.Tension)

		events = append(events, core.Event{
			ID:     fmt.Sprintf("evt_world_pressure_%d_%s", now.UnixNano(), pressure.ID),
			Type:   "world_pressure",
			Actor:  "system",
			Target: pressure.Target,
			Payload: map[string]interface{}{
				"pressure_id":      pressure.ID,
				"name":             pressure.Name,
				"kind":             pressure.Kind,
				"description":      pressure.Description,
				"target":           pressure.Target,
				"intensity":        ps.CurrentIntensity,
				"config_intensity": pressureByID[pressure.ID].Intensity,
				"escalates":        append([]string(nil), pressure.Escalates...),
				"reason":           reason,
			},
			Canonical: true,
			Tag:       core.TagTick,
			CreatedAt: now,
		})
		events = append(events, core.Event{
			ID:        fmt.Sprintf("evt_world_pressure_tension_%d_%s", now.UnixNano(), pressure.ID),
			Type:      "tension_change",
			Actor:     "system",
			Target:    pressure.Target,
			Payload:   map[string]interface{}{"delta": delta, "reason": reason},
			Canonical: true,
			Tag:       core.TagTick,
			CreatedAt: now,
		})
		events = append(events, core.Event{
			ID:     fmt.Sprintf("evt_world_pressure_var_%d_%s", now.UnixNano(), pressure.ID),
			Type:   "variable_set",
			Actor:  "system",
			Target: pressure.Target,
			Payload: map[string]interface{}{
				"key":   "world.pressure." + pressure.ID + ".last_tick",
				"value": float64(tick),
			},
			Canonical: true,
			Tag:       core.TagTick,
			CreatedAt: now,
		})

		// Escalate chain: if intensity is high, boost or spawn escalated pressures
		if ps.CurrentIntensity >= 0.7 && len(pressure.Escalates) > 0 {
			for _, escID := range pressure.Escalates {
				escID = strings.TrimSpace(escID)
				if escID == "" {
					continue
				}
				if _, ok := pressureByID[escID]; ok {
					// Boost existing escalated pressure
					escPs := p.ensurePressureState(escID)
					escPs.CurrentIntensity += 0.05
					if escPs.CurrentIntensity > 0.95 {
						escPs.CurrentIntensity = 0.95
					}
					// Force it to be eligible soon by resetting lastTriggered
					p.lastTriggered[escID] = tick - pressureCadence(escPs.CurrentIntensity)
				} else {
					// Escalated pressure does not exist yet — spawn potential pressure event
					events = append(events, core.Event{
						ID:     fmt.Sprintf("evt_potential_pressure_%d_%s", now.UnixNano(), escID),
						Type:   "potential_pressure",
						Actor:  "system",
						Target: pressure.Target,
						Payload: map[string]interface{}{
							"potential_id":    escID,
							"source_pressure": pressure.ID,
							"reason":          fmt.Sprintf("escalated from %s", pressure.ID),
						},
						Canonical: true,
						Tag:       core.TagTick,
						CreatedAt: now,
					})
				}
			}
		}
	}

	return events
}

func (p *PulseEngine) ensurePressureState(id string) *pressureState {
	if ps, ok := p.pressureStates[id]; ok {
		return ps
	}
	ps := &pressureState{CurrentIntensity: 0.5, TriggerCount: 0, MissedTicks: 0}
	p.pressureStates[id] = ps
	return ps
}

// PressureStates returns a copy of current dynamic pressure states for observation.
func (p *PulseEngine) PressureStates() map[string]float64 {
	out := make(map[string]float64, len(p.pressureStates))
	for id, ps := range p.pressureStates {
		out[id] = ps.CurrentIntensity
	}
	return out
}

func pressureCadence(intensity float64) int {
	switch {
	case intensity >= 0.85:
		return 1
	case intensity >= 0.65:
		return 2
	case intensity >= 0.45:
		return 3
	default:
		return 4
	}
}

func pressureDelta(intensity, currentTension float64) float64 {
	delta := intensity * 0.12
	if currentTension >= 0.8 {
		delta *= 0.5
	}
	if delta < 0.03 {
		delta = 0.03
	}
	if delta > 0.18 {
		delta = 0.18
	}
	return delta
}
