package emotion

import (
	"database/sql"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// Engine manages the emotional layer: residues, threads, delayed reactions.
// Lives alongside canonical state but allows fuzziness and contradiction.
type Engine struct {
	db *sql.DB
}

func New(dbPath string) (*Engine, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}
	return NewWithDB(db)
}

func NewWithDB(db *sql.DB) (*Engine, error) {
	e := &Engine{db: db}
	if err := e.migrate(); err != nil {
		return nil, err
	}
	return e, nil
}

func (e *Engine) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS emotional_residues (
		id TEXT PRIMARY KEY,
		character TEXT NOT NULL,
		type TEXT NOT NULL,
		source_event TEXT,
		target TEXT,
		intensity REAL DEFAULT 0.0,
		current REAL DEFAULT 0.0,
		decay_rate REAL DEFAULT 0.2,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE IF NOT EXISTS unresolved_threads (
		id TEXT PRIMARY KEY,
		character TEXT NOT NULL,
		topic TEXT NOT NULL,
		involving TEXT,
		opened_at TEXT,
		emotional_weight REAL DEFAULT 0.5,
		status TEXT DEFAULT 'unresolved',
		last_referenced TEXT,
		hint_count INT DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE IF NOT EXISTS delayed_reactions (
		id TEXT PRIMARY KEY,
		character TEXT NOT NULL,
		trigger_event TEXT,
		reaction_type TEXT,
		intensity REAL DEFAULT 0.5,
		target TEXT,
		delay_events INT DEFAULT 0,
		delay_seconds INT DEFAULT 0,
		triggered BOOLEAN DEFAULT 0,
		triggered_at DATETIME,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_residues_character ON emotional_residues(character);
	CREATE INDEX IF NOT EXISTS idx_residues_active ON emotional_residues(character, current);
	CREATE INDEX IF NOT EXISTS idx_threads_character ON unresolved_threads(character);
	CREATE INDEX IF NOT EXISTS idx_threads_status ON unresolved_threads(status);
	CREATE INDEX IF NOT EXISTS idx_reactions_character ON delayed_reactions(character);
	CREATE INDEX IF NOT EXISTS idx_reactions_pending ON delayed_reactions(triggered);
	`
	_, err := e.db.Exec(schema)
	return err
}

// === Residues ===

func (e *Engine) AddResidue(r EmotionalResidue) error {
	if r.Current == 0 {
		r.Current = r.Intensity
	}
	if r.DecayRate == 0 {
		r.DecayRate = 0.2 // default: gone in ~5 days
	}
	_, err := e.db.Exec(
		`INSERT INTO emotional_residues (id, character, type, source_event, target, intensity, current, decay_rate, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.Character, r.Type, r.SourceEvent, r.Target, r.Intensity, r.Current, r.DecayRate, r.CreatedAt,
	)
	return err
}

func (e *Engine) DecayAllResidues(character string, now time.Time) error {
	rows, err := e.db.Query(
		`SELECT id, intensity, decay_rate, created_at FROM emotional_residues WHERE character = ? AND current > 0`, character,
	)
	if err != nil {
		return err
	}

	type decayRow struct {
		id        string
		intensity float64
		decayRate float64
		createdAt time.Time
	}
	var toUpdate []decayRow
	for rows.Next() {
		var dr decayRow
		if err := rows.Scan(&dr.id, &dr.intensity, &dr.decayRate, &dr.createdAt); err != nil {
			rows.Close()
			return err
		}
		toUpdate = append(toUpdate, dr)
	}
	rows.Close()

	for _, dr := range toUpdate {
		days := now.Sub(dr.createdAt).Hours() / 24
		current := dr.intensity - dr.decayRate*days
		if current < 0 {
			current = 0
		}
		if _, err := e.db.Exec(`UPDATE emotional_residues SET current = ? WHERE id = ?`, current, dr.id); err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) GetActiveResidues(character string) ([]EmotionalResidue, error) {
	rows, err := e.db.Query(
		`SELECT id, character, type, source_event, target, intensity, current, decay_rate, created_at
		FROM emotional_residues WHERE character = ? AND current > 0.05 ORDER BY current DESC LIMIT 10`, character,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []EmotionalResidue
	for rows.Next() {
		var r EmotionalResidue
		if err := rows.Scan(&r.ID, &r.Character, &r.Type, &r.SourceEvent, &r.Target, &r.Intensity, &r.Current, &r.DecayRate, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ClearResidue manually removes a residue (e.g., after resolution).
func (e *Engine) ClearResidue(id string) error {
	_, err := e.db.Exec(`UPDATE emotional_residues SET current = 0 WHERE id = ?`, id)
	return err
}

// === Unresolved Threads ===

func (e *Engine) OpenThread(t UnresolvedThread) error {
	_, err := e.db.Exec(
		`INSERT INTO unresolved_threads (id, character, topic, involving, opened_at, emotional_weight, status, last_referenced, hint_count, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.Character, t.Topic, t.Involving, t.OpenedAt, t.EmotionalWeight, t.Status, t.LastReferenced, t.HintCount, t.CreatedAt,
	)
	return err
}

func (e *Engine) HintThread(id string, eventID string) error {
	_, err := e.db.Exec(
		`UPDATE unresolved_threads SET hint_count = hint_count + 1, last_referenced = ? WHERE id = ?`, eventID, id,
	)
	return err
}

func (e *Engine) ResolveThread(id string) error {
	_, err := e.db.Exec(`UPDATE unresolved_threads SET status = 'resolved' WHERE id = ?`, id)
	return err
}

func (e *Engine) AddressThread(id string, eventID string) error {
	_, err := e.db.Exec(
		`UPDATE unresolved_threads SET status = 'addressed', last_referenced = ? WHERE id = ?`, eventID, id,
	)
	return err
}

func (e *Engine) GetUnresolvedThreads(character string) ([]UnresolvedThread, error) {
	rows, err := e.db.Query(
		`SELECT id, character, topic, involving, opened_at, emotional_weight, status, last_referenced, hint_count, created_at
		FROM unresolved_threads WHERE character = ? AND status != 'resolved' ORDER BY emotional_weight DESC`, character,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []UnresolvedThread
	for rows.Next() {
		var t UnresolvedThread
		if err := rows.Scan(&t.ID, &t.Character, &t.Topic, &t.Involving, &t.OpenedAt, &t.EmotionalWeight, &t.Status, &t.LastReferenced, &t.HintCount, &t.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// === Delayed Reactions ===

func (e *Engine) AddDelayedReaction(dr DelayedReaction) error {
	delaySecs := int(dr.DelayDuration.Seconds())
	_, err := e.db.Exec(
		`INSERT INTO delayed_reactions (id, character, trigger_event, reaction_type, intensity, target, delay_events, delay_seconds, triggered, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 0, ?)`,
		dr.ID, dr.Character, dr.TriggerEvent, dr.ReactionType, dr.Intensity, dr.Target, dr.DelayEvents, delaySecs, dr.CreatedAt,
	)
	return err
}

func (e *Engine) CheckAndTriggerReactions(character string, eventCount int, now time.Time) ([]DelayedReaction, error) {
	// Get all untriggered reactions
	rows, err := e.db.Query(
		`SELECT id, character, trigger_event, reaction_type, intensity, target, delay_events, delay_seconds, triggered, created_at
		FROM delayed_reactions WHERE character = ? AND triggered = 0`, character,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var triggered []DelayedReaction
	for rows.Next() {
		var dr DelayedReaction
		var delaySecs int
		if err := rows.Scan(&dr.ID, &dr.Character, &dr.TriggerEvent, &dr.ReactionType, &dr.Intensity, &dr.Target, &dr.DelayEvents, &delaySecs, &dr.Triggered, &dr.CreatedAt); err != nil {
			return nil, err
		}
		dr.DelayDuration = time.Duration(delaySecs) * time.Second

		if dr.ShouldTrigger(eventCount, now) {
			dr.Triggered = true
			dr.TriggeredAt = now
			e.db.Exec(`UPDATE delayed_reactions SET triggered = 1, triggered_at = ? WHERE id = ?`, now, dr.ID)
			triggered = append(triggered, dr)
		}
	}
	return triggered, rows.Err()
}

func (e *Engine) GetPendingReactions(character string) ([]DelayedReaction, error) {
	rows, err := e.db.Query(
		`SELECT id, character, trigger_event, reaction_type, intensity, target, delay_events, delay_seconds, triggered, created_at
		FROM delayed_reactions WHERE character = ? AND triggered = 0 ORDER BY created_at ASC`, character,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []DelayedReaction
	for rows.Next() {
		var dr DelayedReaction
		var delaySecs int
		if err := rows.Scan(&dr.ID, &dr.Character, &dr.TriggerEvent, &dr.ReactionType, &dr.Intensity, &dr.Target, &dr.DelayEvents, &delaySecs, &dr.Triggered, &dr.CreatedAt); err != nil {
			return nil, err
		}
		dr.DelayDuration = time.Duration(delaySecs) * time.Second
		out = append(out, dr)
	}
	return out, rows.Err()
}

// === Snapshot ===

// ComputeSnapshot builds the emotional view for the LLM.
func (e *Engine) ComputeSnapshot(character string, vec EmotionVector, eventCount int, now time.Time) (EmotionalSnapshot, error) {
	dom, domIntensity := vec.DominantEmotion()

	e.DecayAllResidues(character, now)

	residues, err := e.GetActiveResidues(character)
	if err != nil {
		return EmotionalSnapshot{}, err
	}

	threads, err := e.GetUnresolvedThreads(character)
	if err != nil {
		return EmotionalSnapshot{}, err
	}

	reactions, err := e.GetPendingReactions(character)
	if err != nil {
		return EmotionalSnapshot{}, err
	}

	snap := EmotionalSnapshot{
		DominantEmotion:   dom,
		DominantIntensity: domIntensity,
		Vector:            vec,
		Contradictions:    vec.Contradictions(),
		ActiveResidues:    residues,
		UnresolvedThreads: threads,
		PendingReactions:  reactions,
	}
	if snap.ActiveResidues == nil {
		snap.ActiveResidues = []EmotionalResidue{}
	}
	if snap.UnresolvedThreads == nil {
		snap.UnresolvedThreads = []UnresolvedThread{}
	}
	if snap.PendingReactions == nil {
		snap.PendingReactions = []DelayedReaction{}
	}
	return snap, nil
}

// DB exposes the underlying database connection.
func (e *Engine) DB() *sql.DB { return e.db }

func (e *Engine) Close() error {
	return e.db.Close()
}
