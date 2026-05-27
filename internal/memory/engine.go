package memory

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"corerp/internal/core"

	_ "github.com/mattn/go-sqlite3"
)

type Engine struct {
	db           *sql.DB
	mu           sync.RWMutex
	instanceID   string
	shortTerm    map[string][]core.Message // per-character ring buffer
	shortTermCap int
}

func New(dbPath string) (*Engine, error) {
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

	e := &Engine{
		db:           db,
		shortTermCap: 15,
		shortTerm:    make(map[string][]core.Message),
	}
	if err := e.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return e, nil
}

func (e *Engine) migrate() error {
	schema := `
CREATE TABLE IF NOT EXISTS working_memory (
    id TEXT PRIMARY KEY,
    character TEXT,
    content TEXT NOT NULL,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS semantic_facts (
    id TEXT PRIMARY KEY,
    character TEXT,
    subject TEXT,
    predicate TEXT,
    object TEXT,
    confidence REAL DEFAULT 1.0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS episodic_events (
    id TEXT PRIMARY KEY,
    character TEXT,
    event_id TEXT,
    event_type TEXT,
    description TEXT,
    emotional_weight REAL DEFAULT 0.0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS dialogue_history (
    id TEXT PRIMARY KEY,
    role TEXT NOT NULL,
    content TEXT NOT NULL,
    session_id TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_dialogue_session ON dialogue_history(session_id);
CREATE INDEX IF NOT EXISTS idx_dialogue_created ON dialogue_history(created_at);
`
	_, err := e.db.Exec(schema)
	if err != nil {
		return err
	}

	// Migration: add character column to dialogue_history (P3 multi-character)
	e.db.Exec(`ALTER TABLE dialogue_history ADD COLUMN character TEXT DEFAULT ''`)
	e.db.Exec(`CREATE INDEX IF NOT EXISTS idx_dialogue_character ON dialogue_history(character)`)
	e.db.Exec(`ALTER TABLE dialogue_history ADD COLUMN instance_id TEXT DEFAULT ''`)
	e.db.Exec(`CREATE INDEX IF NOT EXISTS idx_dialogue_instance ON dialogue_history(instance_id)`)
	e.db.Exec(`ALTER TABLE working_memory ADD COLUMN instance_id TEXT DEFAULT ''`)
	e.db.Exec(`CREATE INDEX IF NOT EXISTS idx_working_memory_instance ON working_memory(instance_id)`)
	e.db.Exec(`ALTER TABLE semantic_facts ADD COLUMN instance_id TEXT DEFAULT ''`)
	e.db.Exec(`CREATE INDEX IF NOT EXISTS idx_semantic_facts_instance ON semantic_facts(instance_id)`)
	e.db.Exec(`ALTER TABLE episodic_events ADD COLUMN instance_id TEXT DEFAULT ''`)
	e.db.Exec(`CREATE INDEX IF NOT EXISTS idx_episodic_events_instance ON episodic_events(instance_id)`)

	return nil
}

// --- Short-term Memory (in-memory ring buffer) ---

func (e *Engine) PushDialogue(msg core.Message, character string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	buf := e.shortTerm[character]
	if len(buf) >= e.shortTermCap {
		buf = buf[1:]
	}
	buf = append(buf, msg)
	e.shortTerm[character] = buf

	// Persist to SQLite for cross-session recall
	id := fmt.Sprintf("dlg_%d_%s", time.Now().UnixNano(), msg.Role)
	e.db.Exec(
		`INSERT INTO dialogue_history (id, role, content, character, instance_id, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		id, msg.Role, msg.Content, character, e.instanceID, time.Now(),
	)
}

func (e *Engine) GetRecentDialogue(character string) []core.Message {
	e.mu.RLock()
	defer e.mu.RUnlock()

	buf := e.shortTerm[character]
	result := make([]core.Message, len(buf))
	copy(result, buf)
	return result
}

// ResetDialogue clears short-term memory and DB dialogue for a character.
func (e *Engine) ResetDialogue(character string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.shortTerm, character)
	query := `DELETE FROM dialogue_history WHERE character = ?` + e.instanceScopeSuffix(" AND ")
	_, args := e.instanceScopeArgs(character)
	e.db.Exec(query, args...)
}

// LoadRecentDialogueFromDB restores the last N messages for a character from SQLite into short-term memory.
func (e *Engine) LoadRecentDialogueFromDB(character string, limit int) {
	e.mu.Lock()
	defer e.mu.Unlock()

	where, args := e.instanceScopeArgs(character)
	query := `SELECT role, content FROM dialogue_history WHERE character = ?`
	if where != "" {
		query += " AND " + where
	}
	query += " ORDER BY created_at DESC LIMIT ?"
	args = append(args, limit)
	rows, err := e.db.Query(query, args...)
	if err != nil {
		return
	}
	defer rows.Close()

	var msgs []core.Message
	for rows.Next() {
		var msg core.Message
		if err := rows.Scan(&msg.Role, &msg.Content); err == nil {
			msgs = append([]core.Message{msg}, msgs...) // prepend to restore chronological order
		}
	}

	if len(msgs) > e.shortTermCap {
		msgs = msgs[len(msgs)-e.shortTermCap:]
	}
	e.shortTerm[character] = msgs
}

// --- Working Memory ---

func (e *Engine) SetWorkingMemory(character, content string) error {
	_, err := e.db.Exec(
		`INSERT INTO working_memory (id, character, content, instance_id, updated_at) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET content=excluded.content, instance_id=excluded.instance_id, updated_at=excluded.updated_at`,
		fmt.Sprintf("wm_%s_%s", e.instanceScopeKey(), character), character, content, e.instanceID, time.Now(),
	)
	return err
}

func (e *Engine) GetWorkingMemory(character string) (string, error) {
	var content string
	query := `SELECT content FROM working_memory WHERE character = ?` + e.instanceScopeSuffix(" AND ")
	_, args := e.instanceScopeArgs(character)
	err := e.db.QueryRow(query, args...).Scan(&content)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return content, err
}

// --- Semantic Memory ---

func (e *Engine) RememberFact(fact core.FactFrame, character string, confidence float64) error {
	id := fmt.Sprintf("fact_%s_%s_%s_%d", character, fact.Subject, fact.Predicate, time.Now().Unix())
	_, err := e.db.Exec(
		`INSERT INTO semantic_facts (id, character, subject, predicate, object, confidence, instance_id, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, character, fact.Subject, fact.Predicate, fact.Object, confidence, e.instanceID, time.Now(),
	)
	return err
}

func (e *Engine) RecallFacts(query string, character string, limit int) ([]core.FactFrame, error) {
	// P1: simple keyword matching, no vector search yet
	scope, args := e.instanceScopeArgs(character, "%"+query+"%", "%"+query+"%", "%"+query+"%")
	sqlQuery := `SELECT subject, predicate, object, confidence FROM semantic_facts
		WHERE character = ? AND (subject LIKE ? OR predicate LIKE ? OR object LIKE ?)`
	if scope != "" {
		sqlQuery += " AND " + scope
	}
	sqlQuery += " ORDER BY confidence DESC LIMIT ?"
	args = append(args, limit)
	rows, err := e.db.Query(sqlQuery, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var facts []core.FactFrame
	for rows.Next() {
		var f core.FactFrame
		if err := rows.Scan(&f.Subject, &f.Predicate, &f.Object, &f.Confidence); err != nil {
			return nil, err
		}
		facts = append(facts, f)
	}
	return facts, rows.Err()
}

func (e *Engine) GetAllFacts(character string) ([]core.FactFrame, error) {
	scope, args := e.instanceScopeArgs(character)
	query := `SELECT subject, predicate, object, confidence FROM semantic_facts WHERE character = ?`
	if scope != "" {
		query += " AND " + scope
	}
	query += " ORDER BY created_at DESC"
	rows, err := e.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var facts []core.FactFrame
	for rows.Next() {
		var f core.FactFrame
		if err := rows.Scan(&f.Subject, &f.Predicate, &f.Object, &f.Confidence); err != nil {
			return nil, err
		}
		facts = append(facts, f)
	}
	return facts, rows.Err()
}

// SeedFacts inserts ontology facts with high confidence (canonical truth).
func (e *Engine) SeedFacts(facts []core.FactFrame, character string) error {
	// Skip if ontology seed IDs already exist for this character.
	var count int
	query := `SELECT COUNT(*) FROM semantic_facts WHERE id LIKE ?` + e.instanceScopeSuffix(" AND ")
	_, args := e.instanceScopeArgs(fmt.Sprintf("ont_%s_%%", character))
	e.db.QueryRow(query, args...).Scan(&count)
	if count > 0 {
		return nil
	}

	tx, err := e.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`INSERT OR IGNORE INTO semantic_facts (id, character, subject, predicate, object, confidence, instance_id, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	now := time.Now()
	for i, f := range facts {
		id := fmt.Sprintf("ont_%s_%d", character, i)
		if _, err := stmt.Exec(id, character, f.Subject, f.Predicate, f.Object, f.Confidence, e.instanceID, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (e *Engine) ReplaceSeedFacts(facts []core.FactFrame, character string) error {
	tx, err := e.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	query := `DELETE FROM semantic_facts WHERE id LIKE ?` + e.instanceScopeSuffix(" AND ")
	_, args := e.instanceScopeArgs(fmt.Sprintf("ont_%s_%%", character))
	if _, err := tx.Exec(query, args...); err != nil {
		return err
	}
	stmt, err := tx.Prepare(`INSERT INTO semantic_facts (id, character, subject, predicate, object, confidence, instance_id, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	now := time.Now()
	for i, f := range facts {
		id := fmt.Sprintf("ont_%s_%d", character, i)
		if _, err := stmt.Exec(id, character, f.Subject, f.Predicate, f.Object, f.Confidence, e.instanceID, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// SeedEpisodics inserts ontology events as episodic memory.
func (e *Engine) SeedEpisodics(events []core.EventFrame, character string) error {
	var count int
	query := `SELECT COUNT(*) FROM episodic_events WHERE id LIKE ?` + e.instanceScopeSuffix(" AND ")
	_, args := e.instanceScopeArgs(fmt.Sprintf("ont_epi_%s_%%", character))
	e.db.QueryRow(query, args...).Scan(&count)
	if count > 0 {
		return nil
	}

	tx, err := e.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`INSERT OR IGNORE INTO episodic_events (id, character, event_id, event_type, description, emotional_weight, instance_id, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	now := time.Now()
	for i, evt := range events {
		id := fmt.Sprintf("ont_epi_%s_%d", character, i)
		if _, err := stmt.Exec(id, character, evt.EventID, evt.Type, evt.Description, evt.EmotionalWeight, e.instanceID, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (e *Engine) ReplaceSeedEpisodics(events []core.EventFrame, character string) error {
	tx, err := e.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	query := `DELETE FROM episodic_events WHERE id LIKE ?` + e.instanceScopeSuffix(" AND ")
	_, args := e.instanceScopeArgs(fmt.Sprintf("ont_epi_%s_%%", character))
	if _, err := tx.Exec(query, args...); err != nil {
		return err
	}
	stmt, err := tx.Prepare(`INSERT INTO episodic_events (id, character, event_id, event_type, description, emotional_weight, instance_id, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	now := time.Now()
	for i, evt := range events {
		id := fmt.Sprintf("ont_epi_%s_%d", character, i)
		if _, err := stmt.Exec(id, character, evt.EventID, evt.Type, evt.Description, evt.EmotionalWeight, e.instanceID, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// --- Episodic Memory ---

func (e *Engine) RecordEpisodic(event core.EventFrame, character string) error {
	id := fmt.Sprintf("epi_%s_%s_%d", character, event.EventID, time.Now().Unix())
	_, err := e.db.Exec(
		`INSERT INTO episodic_events (id, character, event_id, event_type, description, emotional_weight, instance_id, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, character, event.EventID, event.Type, event.Description, event.EmotionalWeight, e.instanceID, time.Now(),
	)
	return err
}

func (e *Engine) RecallEpisodic(query string, character string, limit int) ([]core.EventFrame, error) {
	// P1: keyword matching on description
	scope, args := e.instanceScopeArgs(character, "%"+query+"%")
	sqlQuery := `SELECT event_id, event_type, description, emotional_weight FROM episodic_events
		WHERE character = ? AND description LIKE ?`
	if scope != "" {
		sqlQuery += " AND " + scope
	}
	sqlQuery += " ORDER BY emotional_weight DESC, created_at DESC LIMIT ?"
	args = append(args, limit)
	rows, err := e.db.Query(sqlQuery, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []core.EventFrame
	for rows.Next() {
		var ef core.EventFrame
		if err := rows.Scan(&ef.EventID, &ef.Type, &ef.Description, &ef.EmotionalWeight); err != nil {
			return nil, err
		}
		events = append(events, ef)
	}
	return events, rows.Err()
}

func (e *Engine) GetRecentEpisodic(character string, limit int) ([]core.EventFrame, error) {
	scope, args := e.instanceScopeArgs(character)
	query := `SELECT event_id, event_type, description, emotional_weight FROM episodic_events
		WHERE character = ?`
	if scope != "" {
		query += " AND " + scope
	}
	query += " ORDER BY created_at DESC LIMIT ?"
	args = append(args, limit)
	rows, err := e.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []core.EventFrame
	for rows.Next() {
		var ef core.EventFrame
		if err := rows.Scan(&ef.EventID, &ef.Type, &ef.Description, &ef.EmotionalWeight); err != nil {
			return nil, err
		}
		events = append(events, ef)
	}
	// Reverse to chronological order
	for i, j := 0, len(events)-1; i < j; i, j = i+1, j-1 {
		events[i], events[j] = events[j], events[i]
	}
	return events, rows.Err()
}

// --- Unified Recall (keyword or vector, auto-switching) ---

func (e *Engine) Recall(query string, character string, goals []core.Goal) []core.Memory {
	var candidates []core.Memory

	// 1. Semantic recall
	totalFacts := e.CountFacts(character)
	if ShouldUseVector(totalFacts) {
		// Vector search: load all facts, run similarity
		allFacts, _ := e.GetAllFacts(character)
		vs := NewVectorStore()
		results := vs.SearchFacts(query, allFacts, 10)
		for _, r := range results {
			candidates = append(candidates, core.Memory{
				ID:        fmt.Sprintf("vec_%s", r.ID),
				Type:      "semantic",
				Content:   r.Content,
				Character: character,
				Score:     r.Score,
			})
		}
	} else {
		// Keyword fallback for small datasets
		facts, _ := e.RecallFacts(query, character, 10)
		for _, f := range facts {
			content := fmt.Sprintf("%s %s %s", f.Subject, f.Predicate, f.Object)
			candidates = append(candidates, core.Memory{
				ID:        fmt.Sprintf("fact_%s", content),
				Type:      "semantic",
				Content:   content,
				Character: character,
				Score:     f.Confidence,
			})
		}
	}

	// 2. Episodic recall
	totalEpisodic := e.CountEpisodic(character)
	if ShouldUseVector(totalEpisodic) {
		allEvents, _ := e.GetAllEpisodic(character)
		vs := NewVectorStore()
		results := vs.SearchEpisodic(query, allEvents, 5)
		for _, r := range results {
			candidates = append(candidates, core.Memory{
				ID:        r.ID,
				Type:      "episodic",
				Content:   r.Content,
				Character: character,
				Score:     r.Score,
			})
		}
	} else {
		events, _ := e.RecallEpisodic(query, character, 5)
		for _, ev := range events {
			candidates = append(candidates, core.Memory{
				ID:        ev.EventID,
				Type:      "episodic",
				Content:   ev.Description,
				Character: character,
				Score:     ev.EmotionalWeight,
			})
		}
	}

	// 3. Goal weight adjustment
	for _, g := range goals {
		for i := range candidates {
			if semanticMatch(candidates[i].Content, g.ID) {
				candidates[i].Score *= 1.2 + float64(g.Priority)/10
			}
		}
	}

	// 4. Sort and budget
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Score > candidates[j].Score
	})

	return candidates
}

func semanticMatch(content, keyword string) bool {
	return strings.Contains(content, keyword)
}

// Budget-aware take
func TakeUntilBudget(memories []core.Memory, maxTokens int) []core.Memory {
	// P1: rough estimation: 1 token ≈ 1.5 Chinese chars or 4 English chars
	var result []core.Memory
	used := 0
	for _, m := range memories {
		est := len(m.Content) / 2 // rough
		if used+est > maxTokens {
			break
		}
		result = append(result, m)
		used += est
	}
	return result
}

// CountFacts returns the total number of semantic facts for a character.
func (e *Engine) CountFacts(character string) int {
	var count int
	query := `SELECT COUNT(*) FROM semantic_facts WHERE character = ?` + e.instanceScopeSuffix(" AND ")
	_, args := e.instanceScopeArgs(character)
	e.db.QueryRow(query, args...).Scan(&count)
	return count
}

// CountEpisodic returns the total number of episodic events for a character.
func (e *Engine) CountEpisodic(character string) int {
	var count int
	query := `SELECT COUNT(*) FROM episodic_events WHERE character = ?` + e.instanceScopeSuffix(" AND ")
	_, args := e.instanceScopeArgs(character)
	e.db.QueryRow(query, args...).Scan(&count)
	return count
}

// GetAllEpisodic returns all episodic events for a character (used by vector search).
func (e *Engine) GetAllEpisodic(character string) ([]core.EventFrame, error) {
	scope, args := e.instanceScopeArgs(character)
	query := `SELECT event_id, event_type, description, emotional_weight FROM episodic_events
		WHERE character = ?`
	if scope != "" {
		query += " AND " + scope
	}
	query += " ORDER BY created_at DESC"
	rows, err := e.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []core.EventFrame
	for rows.Next() {
		var ef core.EventFrame
		if err := rows.Scan(&ef.EventID, &ef.Type, &ef.Description, &ef.EmotionalWeight); err != nil {
			return nil, err
		}
		events = append(events, ef)
	}
	return events, rows.Err()
}

func (e *Engine) Close() error {
	return e.db.Close()
}

// DB exposes the underlying database connection for other modules.
func (e *Engine) DB() *sql.DB {
	return e.db
}

func (e *Engine) LastAssistantAt(character string) (time.Time, bool) {
	var raw string
	query := `SELECT created_at FROM dialogue_history WHERE character = ? AND role = 'assistant'` + e.instanceScopeSuffix(" AND ") + ` ORDER BY created_at DESC LIMIT 1`
	_, args := e.instanceScopeArgs(character)
	err := e.db.QueryRow(query, args...).Scan(&raw)
	if err != nil {
		return time.Time{}, false
	}
	for _, layout := range []string{
		"2006-01-02 15:04:05.999999999Z07:00",
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05",
		time.RFC3339Nano,
	} {
		if ts, err := time.Parse(layout, raw); err == nil {
			return ts, true
		}
	}
	return time.Time{}, false
}

func (e *Engine) SetInstanceID(id string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.instanceID = strings.TrimSpace(id)
}

func (e *Engine) InstanceID() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.instanceID
}

func (e *Engine) instanceScopeKey() string {
	if strings.TrimSpace(e.instanceID) == "" {
		return "legacy"
	}
	return e.instanceID
}

func (e *Engine) instanceScopeSuffix(prefix string) string {
	where, _ := e.instanceScopeArgs()
	if where == "" {
		return ""
	}
	return prefix + where
}

func (e *Engine) instanceScopeArgs(prefixArgs ...interface{}) (string, []interface{}) {
	args := append([]interface{}{}, prefixArgs...)
	switch strings.TrimSpace(e.instanceID) {
	case "":
		return "", args
	case "default":
		args = append(args, "default")
		return `(instance_id = ? OR COALESCE(instance_id, '') = '')`, args
	default:
		args = append(args, e.instanceID)
		return `instance_id = ?`, args
	}
}
