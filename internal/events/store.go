package events

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"corerp/internal/core"
	"corerp/internal/narrative"

	_ "github.com/mattn/go-sqlite3"
)

type Store struct {
	db         *sql.DB
	instanceID string
}

var _ narrative.EventStore = (*Store)(nil)

type branchRecord struct {
	Name         string
	ParentBranch string
	ForkEventID  string
	CreatedAt    time.Time
}

func New(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(2)
	if _, err := db.Exec(`PRAGMA busy_timeout = 5000`); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec(`PRAGMA journal_mode = WAL`); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec(`PRAGMA synchronous = NORMAL`); err != nil {
		db.Close()
		return nil, err
	}

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) migrate() error {
	schema := `
CREATE TABLE IF NOT EXISTS events (
    id TEXT PRIMARY KEY,
    type TEXT NOT NULL,
    actor TEXT,
    target TEXT,
    payload JSON,
    causes JSON,
    effects JSON,
    canonical BOOLEAN DEFAULT 0,
    confidence REAL DEFAULT 1.0,
    confirmations INT DEFAULT 0,
    scene_id TEXT,
    session_id TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_events_canonical ON events(canonical);
CREATE INDEX IF NOT EXISTS idx_events_type ON events(type);
CREATE INDEX IF NOT EXISTS idx_events_created ON events(created_at);
CREATE TABLE IF NOT EXISTS branches (
    name TEXT PRIMARY KEY,
    parent_branch TEXT,
    fork_event_id TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
`
	_, err := s.db.Exec(schema)
	if err != nil {
		return err
	}

	// Migration: add branch column for timeline forking (P3)
	s.db.Exec(`ALTER TABLE events ADD COLUMN branch TEXT DEFAULT 'main'`)
	s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_events_branch ON events(branch)`)
	s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_branches_parent ON branches(parent_branch)`)
	s.db.Exec(`ALTER TABLE events ADD COLUMN instance_id TEXT DEFAULT ''`)
	s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_events_instance ON events(instance_id)`)
	s.db.Exec(`ALTER TABLE branches ADD COLUMN instance_id TEXT DEFAULT ''`)
	s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_branches_instance ON branches(instance_id)`)

	if err := s.ensureBranch("main", "", ""); err != nil {
		return err
	}
	if err := s.seedBranchesFromEvents(); err != nil {
		return err
	}

	return nil
}

func (s *Store) Append(e core.Event) error {
	payload, _ := json.Marshal(e.Payload)
	causes, _ := json.Marshal(e.Causes)
	effects, _ := json.Marshal(e.Effects)

	branch := e.Branch
	if branch == "" {
		branch = "main"
	}
	if err := s.ensureBranch(branch, "", ""); err != nil {
		return err
	}
	_, err := s.db.Exec(
		`INSERT INTO events (id, type, actor, target, payload, causes, effects, canonical, confidence, confirmations, scene_id, session_id, branch, instance_id, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ID, e.Type, e.Actor, e.Target, payload, causes, effects,
		e.Canonical, e.Confidence, e.Confirmations, e.SceneID, e.SessionID, branch, s.instanceID, e.CreatedAt,
	)
	return err
}

func (s *Store) GetCanonicalEvents() ([]core.Event, error) {
	where, args := s.instanceScopeArgs()
	query := `SELECT id, type, actor, target, payload, causes, effects, canonical, confidence, confirmations, scene_id, session_id, branch, created_at FROM events WHERE canonical = 1`
	if where != "" {
		query += " AND " + where
	}
	query += " ORDER BY created_at ASC"
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanEvents(rows)
}

func (s *Store) GetCanonicalEventsByBranch(branch string) ([]core.Event, error) {
	if branch == "" {
		branch = "main"
	}
	where, args := s.instanceScopeArgs(branch)
	query := `SELECT id, type, actor, target, payload, causes, effects, canonical, confidence, confirmations, scene_id, session_id, branch, created_at FROM events WHERE canonical = 1 AND branch = ?`
	if where != "" {
		query += " AND " + where
	}
	query += " ORDER BY created_at ASC"
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEvents(rows)
}

func (s *Store) ensureBranch(name, parentBranch, forkEventID string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "main"
	}
	_, err := s.db.Exec(
		`INSERT INTO branches (name, parent_branch, fork_event_id, instance_id, created_at)
		 VALUES (?, NULLIF(?, ''), NULLIF(?, ''), ?, ?)
		 ON CONFLICT(name) DO NOTHING`,
		name, parentBranch, forkEventID, s.instanceID, time.Now().UTC(),
	)
	return err
}

func (s *Store) CreateBranch(name, parentBranch, forkEventID string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("branch name is required")
	}
	if parentBranch == "" {
		parentBranch = "main"
	}
	if _, err := s.GetBranch(parentBranch); err != nil {
		return fmt.Errorf("parent branch '%s' not found", parentBranch)
	}
	if forkEventID == "" {
		return fmt.Errorf("fork event id is required")
	}
	if _, err := s.GetByID(forkEventID); err != nil {
		return fmt.Errorf("fork event '%s' not found", forkEventID)
	}
	_, err := s.db.Exec(
		`INSERT INTO branches (name, parent_branch, fork_event_id, instance_id, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		name, parentBranch, forkEventID, s.instanceID, time.Now().UTC(),
	)
	return err
}

func (s *Store) GetBranch(name string) (branchRecord, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "main"
	}
	var rec branchRecord
	var parent, fork sql.NullString
	var createdAtRaw string
	err := s.db.QueryRow(
		`SELECT name, parent_branch, fork_event_id, created_at FROM branches WHERE name = ?`+s.branchScopeSuffix(" AND "),
		func() []interface{} {
			_, args := s.branchScopeArgs(name)
			return args
		}()...,
	).Scan(&rec.Name, &parent, &fork, &createdAtRaw)
	if err != nil {
		return branchRecord{}, err
	}
	rec.ParentBranch = parent.String
	rec.ForkEventID = fork.String
	rec.CreatedAt = parseEventTime(createdAtRaw)
	return rec, nil
}

func (s *Store) ListBranchesMetadata() ([]branchRecord, error) {
	rows, err := s.db.Query(
		`SELECT name, parent_branch, fork_event_id, created_at FROM branches`+s.branchScopeSuffix(" WHERE ")+` ORDER BY name`,
		func() []interface{} {
			_, args := s.branchScopeArgs()
			return args
		}()...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []branchRecord
	for rows.Next() {
		var rec branchRecord
		var parent, fork sql.NullString
		var createdAtRaw string
		if err := rows.Scan(&rec.Name, &parent, &fork, &createdAtRaw); err != nil {
			return nil, err
		}
		rec.ParentBranch = parent.String
		rec.ForkEventID = fork.String
		rec.CreatedAt = parseEventTime(createdAtRaw)
		records = append(records, rec)
	}
	return records, rows.Err()
}

func (s *Store) seedBranchesFromEvents() error {
	where, args := s.instanceScopeArgs()
	query := `SELECT DISTINCT COALESCE(branch, 'main') FROM events`
	if where != "" {
		query += " WHERE " + where
	}
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return err
	}

	var branches []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return err
		}
		branches = append(branches, name)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, name := range branches {
		if err := s.ensureBranch(name, "", ""); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) GetAllEvents(limit int) ([]core.Event, error) {
	where, args := s.instanceScopeArgs()
	query := `SELECT id, type, actor, target, payload, causes, effects, canonical, confidence, confirmations, scene_id, session_id, branch, created_at FROM events`
	if where != "" {
		query += " WHERE " + where
	}
	query += " ORDER BY created_at DESC LIMIT ?"
	args = append(args, limit)
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	events, err := scanEvents(rows)
	if err != nil {
		return nil, err
	}
	// Reverse to chronological order
	for i, j := 0, len(events)-1; i < j; i, j = i+1, j-1 {
		events[i], events[j] = events[j], events[i]
	}
	return events, nil
}

func (s *Store) ConfirmEvent(eventID string) error {
	where, args := s.instanceScopeArgs(eventID)
	query := `UPDATE events SET canonical = 1 WHERE id = ?`
	if where != "" {
		query += " AND " + where
	}
	_, err := s.db.Exec(query, args...)
	return err
}

func scanEvents(rows *sql.Rows) ([]core.Event, error) {
	var events []core.Event
	for rows.Next() {
		var e core.Event
		var payloadJSON, causesJSON, effectsJSON []byte
		var createdAtStr, branch string
		var actor, target, sceneID, sessionID sql.NullString

		err := rows.Scan(&e.ID, &e.Type, &actor, &target, &payloadJSON, &causesJSON, &effectsJSON,
			&e.Canonical, &e.Confidence, &e.Confirmations, &sceneID, &sessionID, &branch, &createdAtStr)
		if err != nil {
			return nil, err
		}

		e.Actor = actor.String
		e.Target = target.String
		e.SceneID = sceneID.String
		e.SessionID = sessionID.String
		e.Branch = branch

		json.Unmarshal(payloadJSON, &e.Payload)
		json.Unmarshal(causesJSON, &e.Causes)
		json.Unmarshal(effectsJSON, &e.Effects)
		e.CreatedAt = parseEventTime(createdAtStr)

		events = append(events, e)
	}
	return events, rows.Err()
}

func (s *Store) GetByID(eventID string) (core.Event, error) {
	var e core.Event
	var payloadJSON, causesJSON, effectsJSON []byte
	var createdAtStr, branch string
	var actor, target, sceneID, sessionID sql.NullString

	err := s.db.QueryRow(
		`SELECT id, type, actor, target, payload, causes, effects, canonical, confidence, confirmations, scene_id, session_id, branch, created_at FROM events WHERE id = ?`+s.instanceScopeSuffix(" AND "),
		func() []interface{} {
			_, args := s.instanceScopeArgs(eventID)
			return args
		}()...,
	).Scan(&e.ID, &e.Type, &actor, &target, &payloadJSON, &causesJSON, &effectsJSON,
		&e.Canonical, &e.Confidence, &e.Confirmations, &sceneID, &sessionID, &branch, &createdAtStr)
	if err != nil {
		return core.Event{}, err
	}

	e.Actor = actor.String
	e.Target = target.String
	e.SceneID = sceneID.String
	e.SessionID = sessionID.String
	e.Branch = branch

	json.Unmarshal(payloadJSON, &e.Payload)
	json.Unmarshal(causesJSON, &e.Causes)
	json.Unmarshal(effectsJSON, &e.Effects)
	e.CreatedAt = parseEventTime(createdAtStr)

	return e, nil
}

// GetRecentEvents returns the last N events (canonical and quarantined).
func (s *Store) GetRecentEvents(limit int) ([]core.Event, error) {
	where, args := s.instanceScopeArgs()
	query := `SELECT id, type, actor, target, payload, causes, effects, canonical, confidence, confirmations, scene_id, session_id, branch, created_at FROM events`
	if where != "" {
		query += " WHERE " + where
	}
	query += " ORDER BY created_at DESC LIMIT ?"
	args = append(args, limit)
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	events, err := scanEvents(rows)
	if err != nil {
		return nil, err
	}
	// Reverse to chronological order
	for i, j := 0, len(events)-1; i < j; i, j = i+1, j-1 {
		events[i], events[j] = events[j], events[i]
	}
	return events, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) SetInstanceID(id string) {
	s.instanceID = strings.TrimSpace(id)
}

func (s *Store) InstanceID() string {
	return s.instanceID
}

func (s *Store) instanceScopeSuffix(prefix string) string {
	where, _ := s.instanceScopeArgs()
	if where == "" {
		return ""
	}
	return prefix + where
}

func (s *Store) instanceScopeArgs(prefixArgs ...interface{}) (string, []interface{}) {
	args := append([]interface{}{}, prefixArgs...)
	switch strings.TrimSpace(s.instanceID) {
	case "":
		return "", args
	case "default":
		args = append(args, "default")
		return `(instance_id = ? OR COALESCE(instance_id, '') = '')`, args
	default:
		args = append(args, s.instanceID)
		return `instance_id = ?`, args
	}
}

func (s *Store) branchScopeSuffix(prefix string) string {
	where, _ := s.branchScopeArgs()
	if where == "" {
		return ""
	}
	return prefix + where
}

func (s *Store) branchScopeArgs(prefixArgs ...interface{}) (string, []interface{}) {
	args := append([]interface{}{}, prefixArgs...)
	switch strings.TrimSpace(s.instanceID) {
	case "":
		return "", args
	case "default":
		args = append(args, "default")
		return `(instance_id = ? OR COALESCE(instance_id, '') = '')`, args
	default:
		args = append(args, s.instanceID)
		return `instance_id = ?`, args
	}
}

func parseEventTime(raw string) time.Time {
	layouts := []string{
		"2006-01-02 15:04:05.999999999Z07:00",
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05",
		time.RFC3339Nano,
	}
	for _, layout := range layouts {
		if ts, err := time.Parse(layout, raw); err == nil {
			return ts
		}
	}
	if strings.Contains(raw, " ") {
		if ts, err := time.Parse(time.RFC3339Nano, strings.Replace(raw, " ", "T", 1)); err == nil {
			return ts
		}
	}
	return time.Time{}
}

// Project reconstructs WorldState from canonical events
func Project(eventStream []core.Event) core.WorldState {
	state := core.WorldState{
		Relationships: make(map[string]core.Relationship),
		Variables:     make(map[string]interface{}),
		Flags:         make(map[string]bool),
	}

	for _, e := range eventStream {
		if !e.Canonical {
			continue
		}
		state = applyEvent(state, e)
	}
	return state
}

func applyEvent(state core.WorldState, e core.Event) core.WorldState {
	switch e.Type {
	case "scene_init":
		// Full scene override — higher priority than partial changes
		if loc, ok := e.Payload["location"].(string); ok {
			state.Scene.Location = loc
		}
		if tod, ok := e.Payload["time_of_day"].(string); ok {
			state.Scene.TimeOfDay = tod
		}
		if weather, ok := e.Payload["weather"].(string); ok {
			state.Scene.Weather = weather
		}
		if chars, ok := e.Payload["characters"].([]interface{}); ok {
			state.Scene.Characters = nil
			for _, c := range chars {
				if s, ok := c.(string); ok {
					state.Scene.Characters = append(state.Scene.Characters, s)
				}
			}
		}
		if desc, ok := e.Payload["description"].(string); ok {
			state.Scene.Description = desc
		}

	case "scene_change":
		if loc, ok := e.Payload["location"].(string); ok {
			state.Scene.Location = loc
		}
		if tod, ok := e.Payload["time_of_day"].(string); ok {
			state.Scene.TimeOfDay = tod
		}
		if weather, ok := e.Payload["weather"].(string); ok {
			state.Scene.Weather = weather
		}
		if chars, ok := e.Payload["characters"].([]interface{}); ok {
			state.Scene.Characters = nil
			for _, c := range chars {
				if s, ok := c.(string); ok {
					state.Scene.Characters = append(state.Scene.Characters, s)
				}
			}
		}
		if desc, ok := e.Payload["description"].(string); ok {
			state.Scene.Description = desc
		}

	case "clock_advance":
		if h, ok := e.Payload["hour"].(float64); ok {
			state.Clock.Hour = int(h)
		}
		if m, ok := e.Payload["minute"].(float64); ok {
			state.Clock.Minute = int(m)
		}
		if d, ok := e.Payload["day"].(float64); ok {
			state.Clock.Day = int(d)
		}

	case "trust_change":
		key := fmt.Sprintf("%s_%s", e.Actor, e.Target)
		r := state.Relationships[key]
		if delta, ok := e.Payload["delta"].(float64); ok {
			r.Trust += delta
		}
		state.Relationships[key] = r

	case "intimacy_change":
		key := fmt.Sprintf("%s_%s", e.Actor, e.Target)
		r := state.Relationships[key]
		if delta, ok := e.Payload["delta"].(float64); ok {
			r.Intimacy += delta
		}
		state.Relationships[key] = r

	case "fear_change":
		key := fmt.Sprintf("%s_%s", e.Actor, e.Target)
		r := state.Relationships[key]
		if delta, ok := e.Payload["delta"].(float64); ok {
			r.Fear += delta
		}
		state.Relationships[key] = r

	case "flag_set":
		if key, ok := e.Payload["key"].(string); ok {
			state.Flags[key] = true
		}

	case "flag_unset":
		if key, ok := e.Payload["key"].(string); ok {
			state.Flags[key] = false
		}

	case "variable_set":
		if key, ok := e.Payload["key"].(string); ok {
			state.Variables[key] = e.Payload["value"]
		}

	case "tension_change":
		if delta, ok := e.Payload["delta"].(float64); ok {
			state.Tension += delta
		}
	}

	return state
}
