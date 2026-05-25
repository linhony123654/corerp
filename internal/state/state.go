package state

import (
	"sync"

	"corerp/internal/core"
)

// Manager holds the in-memory world state projection.
type Manager struct {
	mu    sync.RWMutex
	state core.WorldState
}

func New() *Manager {
	return &Manager{
		state: core.WorldState{
			Relationships: make(map[string]core.Relationship),
			Variables:     make(map[string]interface{}),
			Flags:         make(map[string]bool),
		},
	}
}

func (m *Manager) Get() core.WorldState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.state
}

func (m *Manager) Set(s core.WorldState) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state = s
}

func (m *Manager) ApplyEffects(effects []core.StateEffect) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, eff := range effects {
		// Simple path resolution: "relationships.actor.target.trust"
		// Phase 1: only handle a few known paths
		switch eff.Path {
		default:
			// Store in variables for unknown paths
			m.state.Variables[eff.Path] = eff.Delta
		}
	}
}

func (m *Manager) UpdateFromProjection(state core.WorldState) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state = state
}
