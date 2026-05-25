package events

import (
	"fmt"
	"time"

	"corerp/internal/core"
)

// Gatekeeper routes events into canonical or quarantine based on source.
type Gatekeeper struct {
	store     *Store
	causality *CausalityEngine
}

func NewGatekeeper(store *Store) *Gatekeeper {
	return &Gatekeeper{
		store:     store,
		causality: NewCausalityEngine(store),
	}
}

// Submit decides canonical vs quarantine based on event source.
// After storing, it automatically establishes causal links to recent events.
func (g *Gatekeeper) Submit(e core.Event, source string) error {
	switch source {
	case "user_input", "system", "action_result", "tick":
		e.Canonical = true
		e.Confidence = 1.0
	case "llm_extracted", "inferred":
		e.Canonical = false
		if e.Confidence == 0 {
			e.Confidence = 0.5
		}
	default:
		e.Canonical = false
	}
	if err := g.store.Append(e); err != nil {
		return err
	}
	// Auto-link causal relationships
	go g.causality.LinkNewEvent(e)
	return nil
}

// Causality returns the causality engine for querying.
func (g *Gatekeeper) Causality() *CausalityEngine {
	return g.causality
}

// Review manually confirms or rejects a quarantined event.
func (g *Gatekeeper) Review(eventID string, confirm bool) error {
	if confirm {
		return g.store.ConfirmEvent(eventID)
	}
	_, err := g.store.db.Exec(`DELETE FROM events WHERE id = ? AND canonical = 0`, eventID)
	return err
}

// AutoPromote promotes quarantined events that meet confidence thresholds.
// Rules: confidence >= 0.7 AND (confirmations >= 1 OR age >= 60s)
func (g *Gatekeeper) AutoPromote() (int, error) {
	cutoff := time.Now().Add(-60 * time.Second).Format("2006-01-02 15:04:05")
	result, err := g.store.db.Exec(
		`UPDATE events SET canonical = 1 WHERE canonical = 0
		 AND (confidence >= 0.7 AND (confirmations >= 1 OR created_at <= ?))`,
		cutoff,
	)
	if err != nil {
		return 0, err
	}
	n, _ := result.RowsAffected()
	return int(n), nil
}

// ListPending returns quarantined events pending review.
func (g *Gatekeeper) ListPending(limit int) ([]core.Event, error) {
	rows, err := g.store.db.Query(
		`SELECT id, type, actor, target, payload, causes, effects, canonical, confidence, confirmations, scene_id, session_id, created_at
		 FROM events WHERE canonical = 0 ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEvents(rows)
}

// IncrementConfirmation bumps confirmation count for an event (e.g. when multiple sources agree).
func (g *Gatekeeper) IncrementConfirmation(eventID string) error {
	_, err := g.store.db.Exec(
		`UPDATE events SET confirmations = confirmations + 1 WHERE id = ?`, eventID)
	return err
}

// Stats returns counts of canonical vs quarantined events.
func (g *Gatekeeper) Stats() (canonical int, quarantined int, err error) {
	row := g.store.db.QueryRow(`SELECT COUNT(*) FROM events WHERE canonical = 1`)
	if err := row.Scan(&canonical); err != nil {
		return 0, 0, err
	}
	row = g.store.db.QueryRow(`SELECT COUNT(*) FROM events WHERE canonical = 0`)
	if err := row.Scan(&quarantined); err != nil {
		return 0, 0, err
	}
	return canonical, quarantined, nil
}

// source helpers for runtime
func SourceUserInput() string   { return "user_input" }
func SourceSystem() string      { return "system" }
func SourceActionResult() string { return "action_result" }
func SourceLLMExtracted() string { return "llm_extracted" }
func SourceTick() string        { return "tick" }

// BuildEvent is a convenience constructor.
func BuildEvent(typ, actor, target string, payload map[string]interface{}) core.Event {
	return core.Event{
		ID:        fmt.Sprintf("evt_%d", time.Now().UnixNano()),
		Type:      typ,
		Actor:     actor,
		Target:    target,
		Payload:   payload,
		CreatedAt: time.Now(),
	}
}
