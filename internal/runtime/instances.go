package runtime

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"corerp/internal/core"
)

const (
	InstanceStatusRunning = "running"
	InstanceStatusStopped = "stopped"
)

var (
	ErrInstanceIDRequired = errors.New("instance id required")
	ErrInstanceNotFound   = errors.New("instance not found")
	ErrInstanceConflict   = errors.New("instance conflict")
)

type managedInstance struct {
	engine    *Engine
	label     string
	createdAt time.Time
	status    string
}

// Manager tracks named runtime instances while preserving a deterministic default.
type Manager struct {
	mu        sync.RWMutex
	defaultID string
	instances map[string]*managedInstance
}

func NewManager() *Manager {
	return &Manager{
		instances: make(map[string]*managedInstance),
	}
}

func (m *Manager) Register(id, label string, engine *Engine, isDefault bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	id = strings.TrimSpace(id)
	label = strings.TrimSpace(label)
	if id == "" {
		return ErrInstanceIDRequired
	}
	if engine == nil {
		return fmt.Errorf("engine required")
	}
	if _, exists := m.instances[id]; exists {
		return fmt.Errorf("instance already exists: %s", id)
	}
	if label == "" {
		label = id
	}

	createdAt := time.Now().UTC()
	engine.SetInstanceMetadata(id, createdAt)
	m.instances[id] = &managedInstance{
		engine:    engine,
		label:     label,
		createdAt: createdAt,
		status:    InstanceStatusRunning,
	}
	if isDefault || m.defaultID == "" {
		m.defaultID = id
	}
	return nil
}

func (m *Manager) DefaultID() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.defaultID
}

func (m *Manager) Resolve(id string) (*Engine, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	id = strings.TrimSpace(id)
	if id == "" {
		id = m.defaultID
	}
	instance, ok := m.instances[id]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrInstanceNotFound, id)
	}
	return instance.engine, nil
}

func (m *Manager) List() []core.RuntimeInstanceSummary {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ids := make([]string, 0, len(m.instances))
	for id := range m.instances {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	out := make([]core.RuntimeInstanceSummary, 0, len(ids))
	for _, id := range ids {
		out = append(out, m.summaryLocked(id, m.instances[id]))
	}
	return out
}

func (m *Manager) Status(id string) (core.RuntimeInstanceSummary, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	id = strings.TrimSpace(id)
	if id == "" {
		id = m.defaultID
	}
	instance, ok := m.instances[id]
	if !ok {
		return core.RuntimeInstanceSummary{}, fmt.Errorf("%w: %s", ErrInstanceNotFound, id)
	}
	return m.summaryLocked(id, instance), nil
}

func (m *Manager) SetDefault(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	id = strings.TrimSpace(id)
	if id == "" {
		return ErrInstanceIDRequired
	}
	if _, ok := m.instances[id]; !ok {
		return fmt.Errorf("%w: %s", ErrInstanceNotFound, id)
	}
	m.defaultID = id
	return nil
}

func (m *Manager) Stop(id string) (core.RuntimeInstanceSummary, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	id = strings.TrimSpace(id)
	if id == "" {
		id = m.defaultID
	}
	instance, ok := m.instances[id]
	if !ok {
		return core.RuntimeInstanceSummary{}, fmt.Errorf("%w: %s", ErrInstanceNotFound, id)
	}
	if instance.status != InstanceStatusStopped {
		instance.engine.Stop()
		instance.status = InstanceStatusStopped
	}
	return m.summaryLocked(id, instance), nil
}

func (m *Manager) Delete(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	id = strings.TrimSpace(id)
	if id == "" {
		return ErrInstanceIDRequired
	}
	instance, ok := m.instances[id]
	if !ok {
		return fmt.Errorf("%w: %s", ErrInstanceNotFound, id)
	}
	if len(m.instances) == 1 {
		return fmt.Errorf("%w: cannot delete the only instance", ErrInstanceConflict)
	}
	if id == m.defaultID {
		return fmt.Errorf("%w: cannot delete default instance: set another default first", ErrInstanceConflict)
	}

	instance.engine.Stop()
	if err := deleteInstanceData(instance.engine, id); err != nil {
		return err
	}
	delete(m.instances, id)
	return nil
}

func (m *Manager) CreateFrom(sourceID, id, label, focusCharacter string) (core.RuntimeInstanceSummary, error) {
	source, err := m.Resolve(sourceID)
	if err != nil {
		return core.RuntimeInstanceSummary{}, err
	}
	engine, err := source.SpawnInstance(id, focusCharacter)
	if err != nil {
		return core.RuntimeInstanceSummary{}, err
	}
	if err := m.Register(id, label, engine, false); err != nil {
		engine.Stop()
		return core.RuntimeInstanceSummary{}, err
	}
	engine.StartTickLoop()
	return m.Status(id)
}

func (m *Manager) summaryLocked(id string, instance *managedInstance) core.RuntimeInstanceSummary {
	summary := instance.engine.InstanceSummary()
	summary.ID = id
	summary.Label = instance.label
	summary.CreatedAt = instance.createdAt
	summary.IsDefault = id == m.defaultID
	summary.Status = instance.status
	return summary
}

func deleteInstanceData(engine *Engine, instanceID string) error {
	if engine == nil {
		return fmt.Errorf("engine required")
	}
	if err := deleteInstanceRows(engine.memEngine.DB(), instanceID); err != nil {
		return err
	}
	if strings.TrimSpace(engine.dataDir) == "" {
		return nil
	}
	return os.RemoveAll(filepath.Join(engine.dataDir, "instances", instanceID))
}

func deleteInstanceRows(db *sql.DB, instanceID string) error {
	if db == nil {
		return fmt.Errorf("instance database unavailable")
	}
	instanceID = strings.TrimSpace(instanceID)
	if instanceID == "" {
		return fmt.Errorf("instance id required")
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	statements := []string{
		`DELETE FROM dialogue_history WHERE instance_id = ?`,
		`DELETE FROM working_memory WHERE instance_id = ?`,
		`DELETE FROM semantic_facts WHERE instance_id = ?`,
		`DELETE FROM episodic_events WHERE instance_id = ?`,
		`DELETE FROM pending_facts WHERE instance_id = ?`,
		`DELETE FROM action_log WHERE instance_id = ?`,
		`DELETE FROM events WHERE instance_id = ?`,
		`DELETE FROM branches WHERE instance_id = ?`,
	}
	for _, stmt := range statements {
		if _, err := tx.Exec(stmt, instanceID); err != nil {
			if strings.Contains(err.Error(), "no such table") {
				continue
			}
			return err
		}
	}
	return tx.Commit()
}
