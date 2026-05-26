package emotion

import (
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"
)

// ActionLogEntry records the full decision context for one autonomous action attempt.
// Answers two questions: "why did she act?" and "why didn't she act?"
type ActionLogEntry struct {
	Tick      int       `json:"tick"`
	Timestamp time.Time `json:"timestamp"`

	// Who
	Character string `json:"character"`

	// Decision
	Fired     bool   `json:"fired"`                // did the action actually execute?
	BlockedBy string `json:"blocked_by,omitempty"` // "", "below_threshold", "cooldown", "scene_cap"

	// What (only populated if fired)
	ActionType string  `json:"action_type,omitempty"`
	Target     string  `json:"target,omitempty"`
	Urgency    float64 `json:"urgency"`

	// Why
	Reason string `json:"reason,omitempty"`

	// Pressure sources
	PressureTotal      float64 `json:"pressure_total"`
	PressureLoneliness float64 `json:"pressure_loneliness"`
	PressureJealousy   float64 `json:"pressure_jealousy"`
	PressureGuilt      float64 `json:"pressure_guilt"`
	PressureAnxiety    float64 `json:"pressure_anxiety"`

	// Desire source
	StrongestDesire       string `json:"strongest_desire,omitempty"`
	StrongestDesireType   string `json:"strongest_desire_type,omitempty"`
	StrongestDesireTarget string `json:"strongest_desire_target,omitempty"`

	// Dominant emotion at time of decision
	DominantEmotion string `json:"dominant_emotion"`
}

// ActionLogger is a two-layer log of autonomous action attempts.
// Memory ring buffer for real-time queries; optional SQLite for persistence.
type ActionLogger struct {
	mu         sync.RWMutex
	entries    []ActionLogEntry
	capacity   int
	head       int     // write position
	count      int     // total entries ever written
	db         *sql.DB // optional persistence
	instanceID string
}

// NewActionLogger creates a logger with the given ring buffer capacity.
func NewActionLogger(capacity int) *ActionLogger {
	if capacity <= 0 {
		capacity = 200
	}
	return &ActionLogger{
		entries:  make([]ActionLogEntry, capacity),
		capacity: capacity,
	}
}

// EnablePersistence sets up SQLite backing for the logger.
// When enabled, every Record() call also writes to the DB.
func (l *ActionLogger) EnablePersistence(db *sql.DB) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.db = db
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS action_log (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		tick INTEGER,
		character TEXT NOT NULL,
		fired BOOLEAN,
		blocked_by TEXT,
		action_type TEXT,
		target TEXT,
		urgency REAL,
		reason TEXT,
		pressure_total REAL,
		pressure_loneliness REAL,
		pressure_jealousy REAL,
		pressure_guilt REAL,
		pressure_anxiety REAL,
		strongest_desire TEXT,
		strongest_desire_type TEXT,
		strongest_desire_target TEXT,
		dominant_emotion TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		return err
	}
	_, _ = db.Exec(`ALTER TABLE action_log ADD COLUMN instance_id TEXT DEFAULT ''`)
	_, _ = db.Exec(`CREATE INDEX IF NOT EXISTS idx_action_log_instance ON action_log(instance_id)`)
	return err
}

// Record stores an action attempt. If persistence is enabled, also writes to DB.
func (l *ActionLogger) Record(entry ActionLogEntry) {
	l.mu.Lock()
	defer l.mu.Unlock()

	entry.Timestamp = time.Now()
	l.entries[l.head] = entry
	l.head = (l.head + 1) % l.capacity
	l.count++

	if l.db != nil {
		l.db.Exec(`INSERT INTO action_log (tick, character, fired, blocked_by, action_type, target, urgency, reason,
			pressure_total, pressure_loneliness, pressure_jealousy, pressure_guilt, pressure_anxiety,
			strongest_desire, strongest_desire_type, strongest_desire_target, dominant_emotion, instance_id)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			entry.Tick, entry.Character, entry.Fired, entry.BlockedBy, entry.ActionType, entry.Target,
			entry.Urgency, entry.Reason, entry.PressureTotal, entry.PressureLoneliness, entry.PressureJealousy,
			entry.PressureGuilt, entry.PressureAnxiety, entry.StrongestDesire, entry.StrongestDesireType,
			entry.StrongestDesireTarget, entry.DominantEmotion, l.instanceID,
		)
	}
}

// LoadFromDB reads recent entries from SQLite into the ring buffer.
func (l *ActionLogger) LoadFromDB(limit int) error {
	if l.db == nil {
		return fmt.Errorf("persistence not enabled")
	}
	l.mu.Lock()
	l.entries = make([]ActionLogEntry, l.capacity)
	l.head = 0
	l.count = 0
	l.mu.Unlock()

	query := `SELECT tick, character, fired, COALESCE(blocked_by,''), COALESCE(action_type,''),
		COALESCE(target,''), urgency, COALESCE(reason,''), pressure_total, pressure_loneliness, pressure_jealousy,
		pressure_guilt, pressure_anxiety, COALESCE(strongest_desire,''), COALESCE(strongest_desire_type,''),
		COALESCE(strongest_desire_target,''), COALESCE(dominant_emotion,'')
		FROM action_log`
	var args []interface{}
	if where, scopedArgs := l.instanceScopeArgs(); where != "" {
		query += ` WHERE ` + where
		args = append(args, scopedArgs...)
	}
	query += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := l.db.Query(query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	var entries []ActionLogEntry
	for rows.Next() {
		var e ActionLogEntry
		rows.Scan(&e.Tick, &e.Character, &e.Fired, &e.BlockedBy, &e.ActionType, &e.Target,
			&e.Urgency, &e.Reason, &e.PressureTotal, &e.PressureLoneliness, &e.PressureJealousy,
			&e.PressureGuilt, &e.PressureAnxiety, &e.StrongestDesire, &e.StrongestDesireType,
			&e.StrongestDesireTarget, &e.DominantEmotion)
		entries = append(entries, e)
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	// Reverse to chronological order
	for i := len(entries) - 1; i >= 0; i-- {
		if l.count < l.capacity {
			l.entries[l.count] = entries[i]
			l.count++
			l.head = l.count % l.capacity
		} else {
			break
		}
	}
	return rows.Err()
}

// QueryDB runs an arbitrary filter query against the persisted log.
func (l *ActionLogger) QueryDB(character string, firedOnly, blockedOnly bool, limit int) ([]ActionLogEntry, error) {
	if l.db == nil {
		return nil, fmt.Errorf("persistence not enabled")
	}
	query := `SELECT tick, character, fired, COALESCE(blocked_by,''), COALESCE(action_type,''),
		COALESCE(target,''), urgency, COALESCE(reason,''), pressure_total, pressure_loneliness, pressure_jealousy,
		pressure_guilt, pressure_anxiety, COALESCE(strongest_desire,''), COALESCE(strongest_desire_type,''),
		COALESCE(strongest_desire_target,''), COALESCE(dominant_emotion,'')
		FROM action_log WHERE 1=1`
	args := make([]interface{}, 0, 4)
	if where, scopedArgs := l.instanceScopeArgs(); where != "" {
		query += " AND " + where
		args = append(args, scopedArgs...)
	}

	if character != "" {
		query += " AND character = ?"
		args = append(args, character)
	}
	if firedOnly {
		query += " AND fired = 1"
	}
	if blockedOnly {
		query += " AND fired = 0 AND blocked_by != '' AND blocked_by != 'below_threshold'"
	}
	query += " ORDER BY id DESC LIMIT ?"
	args = append(args, limit)

	rows, err := l.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ActionLogEntry
	for rows.Next() {
		var e ActionLogEntry
		rows.Scan(&e.Tick, &e.Character, &e.Fired, &e.BlockedBy, &e.ActionType, &e.Target,
			&e.Urgency, &e.Reason, &e.PressureTotal, &e.PressureLoneliness, &e.PressureJealousy,
			&e.PressureGuilt, &e.PressureAnxiety, &e.StrongestDesire, &e.StrongestDesireType,
			&e.StrongestDesireTarget, &e.DominantEmotion)
		out = append(out, e)
	}
	// Reverse to chronological
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, rows.Err()
}

// Recent returns the most recent N entries in chronological order.
func (l *ActionLogger) Recent(n int) []ActionLogEntry {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if n <= 0 || n > l.capacity {
		n = l.capacity
	}

	total := l.capacity
	if l.count < l.capacity {
		total = l.count
	}
	if n > total {
		n = total
	}

	out := make([]ActionLogEntry, n)
	for i := 0; i < n; i++ {
		// Read from (head - n + i) wrapping around
		idx := (l.head - n + i + l.capacity) % l.capacity
		out[i] = l.entries[idx]
	}
	return out
}

// ByCharacter returns recent entries for a specific character.
func (l *ActionLogger) ByCharacter(character string, n int) []ActionLogEntry {
	all := l.Recent(l.capacity)
	var out []ActionLogEntry
	for i := len(all) - 1; i >= 0; i-- {
		if all[i].Character == character {
			out = append(out, all[i])
			if len(out) >= n && n > 0 {
				break
			}
		}
	}
	// Reverse to chronological
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

// FiredOnly returns only entries where the action actually executed.
func (l *ActionLogger) FiredOnly(n int) []ActionLogEntry {
	all := l.Recent(l.capacity)
	var out []ActionLogEntry
	for _, e := range all {
		if e.Fired {
			out = append(out, e)
			if len(out) >= n && n > 0 {
				break
			}
		}
	}
	return out
}

// BlockedOnly returns only entries where the action was blocked.
func (l *ActionLogger) BlockedOnly(n int) []ActionLogEntry {
	all := l.Recent(l.capacity)
	var out []ActionLogEntry
	for _, e := range all {
		if !e.Fired && e.BlockedBy != "" && e.BlockedBy != "below_threshold" {
			out = append(out, e)
			if len(out) >= n && n > 0 {
				break
			}
		}
	}
	return out
}

// Stats returns aggregate statistics.
func (l *ActionLogger) Stats() map[string]interface{} {
	l.mu.RLock()
	defer l.mu.RUnlock()

	all := l.Recent(l.capacity)
	var fired, blocked, belowThreshold int
	byBlockReason := map[string]int{}
	byActionType := map[string]int{}
	byCharacter := map[string]int{}

	for _, e := range all {
		if e.Fired {
			fired++
			byActionType[e.ActionType]++
			byCharacter[e.Character]++
		} else if e.BlockedBy == "below_threshold" {
			belowThreshold++
		} else {
			blocked++
			byBlockReason[e.BlockedBy]++
		}
	}

	return map[string]interface{}{
		"total_entries":   len(all),
		"fired":           fired,
		"blocked":         blocked,
		"below_threshold": belowThreshold,
		"by_block_reason": byBlockReason,
		"by_action_type":  byActionType,
		"by_character":    byCharacter,
	}
}

// Total returns total entries ever written.
func (l *ActionLogger) Total() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.count
}

func (l *ActionLogger) SetInstanceID(id string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.instanceID = strings.TrimSpace(id)
}

func (l *ActionLogger) InstanceID() string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.instanceID
}

func (l *ActionLogger) instanceScopeArgs() (string, []interface{}) {
	switch strings.TrimSpace(l.instanceID) {
	case "":
		return "", nil
	case "default":
		return `(instance_id = ? OR COALESCE(instance_id, '') = '')`, []interface{}{"default"}
	default:
		return `instance_id = ?`, []interface{}{l.instanceID}
	}
}
