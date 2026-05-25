package events

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"corerp/internal/core"

	_ "github.com/mattn/go-sqlite3"
)

type Store struct {
	db *sql.DB
}

func New(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
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
`
	_, err := s.db.Exec(schema)
	return err
}

func (s *Store) Append(e core.Event) error {
	payload, _ := json.Marshal(e.Payload)
	causes, _ := json.Marshal(e.Causes)
	effects, _ := json.Marshal(e.Effects)

	_, err := s.db.Exec(
		`INSERT INTO events (id, type, actor, target, payload, causes, effects, canonical, confidence, confirmations, scene_id, session_id, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ID, e.Type, e.Actor, e.Target, payload, causes, effects,
		e.Canonical, e.Confidence, e.Confirmations, e.SceneID, e.SessionID, e.CreatedAt,
	)
	return err
}

func (s *Store) GetCanonicalEvents() ([]core.Event, error) {
	rows, err := s.db.Query(`SELECT id, type, actor, target, payload, causes, effects, canonical, confidence, confirmations, scene_id, session_id, created_at FROM events WHERE canonical = 1 ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanEvents(rows)
}

func (s *Store) GetAllEvents(limit int) ([]core.Event, error) {
	rows, err := s.db.Query(`SELECT id, type, actor, target, payload, causes, effects, canonical, confidence, confirmations, scene_id, session_id, created_at FROM events ORDER BY created_at DESC LIMIT ?`, limit)
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
	_, err := s.db.Exec(`UPDATE events SET canonical = 1 WHERE id = ?`, eventID)
	return err
}

func scanEvents(rows *sql.Rows) ([]core.Event, error) {
	var events []core.Event
	for rows.Next() {
		var e core.Event
		var payloadJSON, causesJSON, effectsJSON []byte
		var createdAtStr string

		err := rows.Scan(&e.ID, &e.Type, &e.Actor, &e.Target, &payloadJSON, &causesJSON, &effectsJSON,
			&e.Canonical, &e.Confidence, &e.Confirmations, &e.SceneID, &e.SessionID, &createdAtStr)
		if err != nil {
			return nil, err
		}

		json.Unmarshal(payloadJSON, &e.Payload)
		json.Unmarshal(causesJSON, &e.Causes)
		json.Unmarshal(effectsJSON, &e.Effects)
		e.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAtStr)

		events = append(events, e)
	}
	return events, rows.Err()
}

func (s *Store) GetByID(eventID string) (core.Event, error) {
	var e core.Event
	var payloadJSON, causesJSON, effectsJSON []byte
	var createdAtStr string

	err := s.db.QueryRow(
		`SELECT id, type, actor, target, payload, causes, effects, canonical, confidence, confirmations, scene_id, session_id, created_at FROM events WHERE id = ?`,
		eventID,
	).Scan(&e.ID, &e.Type, &e.Actor, &e.Target, &payloadJSON, &causesJSON, &effectsJSON,
		&e.Canonical, &e.Confidence, &e.Confirmations, &e.SceneID, &e.SessionID, &createdAtStr)
	if err != nil {
		return core.Event{}, err
	}

	json.Unmarshal(payloadJSON, &e.Payload)
	json.Unmarshal(causesJSON, &e.Causes)
	json.Unmarshal(effectsJSON, &e.Effects)
	e.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAtStr)

	return e, nil
}

// GetRecentEvents returns the last N events (canonical and quarantined).
func (s *Store) GetRecentEvents(limit int) ([]core.Event, error) {
	rows, err := s.db.Query(
		`SELECT id, type, actor, target, payload, causes, effects, canonical, confidence, confirmations, scene_id, session_id, created_at FROM events ORDER BY created_at DESC LIMIT ?`,
		limit,
	)
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
