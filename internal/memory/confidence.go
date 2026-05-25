package memory

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"corerp/internal/core"
)

// ConfidencePipeline gates facts before they enter canonical semantic memory.
type ConfidencePipeline struct {
	db *sql.DB
}

func NewConfidencePipeline(db *sql.DB) *ConfidencePipeline {
	return &ConfidencePipeline{db: db}
}

// Migrate creates the pending_facts table.
func (cp *ConfidencePipeline) Migrate() error {
	schema := `
CREATE TABLE IF NOT EXISTS pending_facts (
    id TEXT PRIMARY KEY,
    character TEXT,
    subject TEXT,
    predicate TEXT,
    object TEXT,
    source TEXT,
    confidence REAL DEFAULT 0.5,
    confirmations INT DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
`
	_, err := cp.db.Exec(schema)
	return err
}

// SubmitFact adds a fact to the pending queue.
func (cp *ConfidencePipeline) SubmitFact(fact core.FactFrame, character, source string) error {
	id := fmt.Sprintf("pending_%s_%s_%s_%d", character, fact.Subject, fact.Predicate, time.Now().Unix())
	conf := cp.sourceConfidence(source)
	_, err := cp.db.Exec(
		`INSERT INTO pending_facts (id, character, subject, predicate, object, source, confidence, confirmations, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, character, fact.Subject, fact.Predicate, fact.Object, source, conf, 0, time.Now(),
	)
	return err
}

// ConfirmPending increments confirmation for matching facts.
func (cp *ConfidencePipeline) ConfirmPending(character, subject, predicate string) error {
	_, err := cp.db.Exec(
		`UPDATE pending_facts SET confirmations = confirmations + 1, confidence = confidence + 0.15
		 WHERE character = ? AND subject = ? AND predicate = ?`,
		character, subject, predicate)
	return err
}

// ProcessPending promotes facts that meet the confidence threshold.
func (cp *ConfidencePipeline) ProcessPending(threshold float64) ([]core.FactFrame, error) {
	rows, err := cp.db.Query(
		`SELECT subject, predicate, object, confidence FROM pending_facts
		 WHERE confidence >= ?`, threshold)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var promoted []core.FactFrame
	for rows.Next() {
		var f core.FactFrame
		if err := rows.Scan(&f.Subject, &f.Predicate, &f.Object, &f.Confidence); err != nil {
			continue
		}
		promoted = append(promoted, f)
	}

	// Delete promoted from pending
	cp.db.Exec(`DELETE FROM pending_facts WHERE confidence >= ?`, threshold)

	return promoted, rows.Err()
}

// sourceConfidence assigns base confidence by source type.
func (cp *ConfidencePipeline) sourceConfidence(source string) float64 {
	switch source {
	case "user_input":
		return 0.9
	case "action_result":
		return 0.85
	case "llm_extracted":
		return 0.4
	case "inferred":
		return 0.3
	default:
		return 0.5
	}
}

// PendingCount returns number of facts awaiting confirmation.
func (cp *ConfidencePipeline) PendingCount() int {
	var count int
	row := cp.db.QueryRow(`SELECT COUNT(*) FROM pending_facts`)
	if err := row.Scan(&count); err != nil {
		return 0
	}
	return count
}

// ApplyFactDecay lowers confidence of old facts and deletes those below threshold.
func ApplyFactDecay(db *sql.DB, threshold float64) (deleted int, err error) {
	// Decay: older facts lose confidence faster
	_, err = db.Exec(`UPDATE semantic_facts SET confidence = confidence * 0.99`)
	if err != nil {
		return 0, err
	}
	res, err := db.Exec(`DELETE FROM semantic_facts WHERE confidence < ?`, threshold)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// ApplyRelationshipDecay cools relationship dimensions over time.
func ApplyRelationshipDecay(rel map[string]core.Relationship) map[string]core.Relationship {
	for k, r := range rel {
		// Natural cooling: all dimensions drift toward neutral (0)
		r.Trust *= 0.995
		r.Intimacy *= 0.99
		r.Fear *= 0.998
		r.Respect *= 0.995
		// Clamp to reasonable bounds
		if r.Trust < -1 {
			r.Trust = -1
		}
		if r.Trust > 1 {
			r.Trust = 1
		}
		if r.Intimacy < 0 {
			r.Intimacy = 0
		}
		if r.Fear < 0 {
			r.Fear = 0
		}
		if r.Respect < -1 {
			r.Respect = -1
		}
		if r.Respect > 1 {
			r.Respect = 1
		}
		rel[k] = r
	}
	return rel
}

// IsGoalExpired checks if a goal's condition is no longer relevant.
func IsGoalExpired(g core.Goal, state core.WorldState) bool {
	// Simple rule: hidden goals never expire by time
	if g.Type == "hidden" {
		return false
	}
	// Check if condition keyword appears in world state variables/flags
	if g.Condition == "" {
		return false
	}
	// If condition mentions a flag that is unset, goal may be expired
	for flag := range state.Flags {
		if strings.Contains(g.Condition, flag) && !state.Flags[flag] {
			return true
		}
	}
	return false
}
