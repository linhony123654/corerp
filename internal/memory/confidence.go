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
	db         *sql.DB
	instanceID string
}

func NewConfidencePipeline(db *sql.DB) *ConfidencePipeline {
	return &ConfidencePipeline{db: db}
}

func NewConfidencePipelineForInstance(db *sql.DB, instanceID string) *ConfidencePipeline {
	return &ConfidencePipeline{db: db, instanceID: strings.TrimSpace(instanceID)}
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
	cp.db.Exec(`ALTER TABLE pending_facts ADD COLUMN instance_id TEXT DEFAULT ''`)
	cp.db.Exec(`CREATE INDEX IF NOT EXISTS idx_pending_facts_instance ON pending_facts(instance_id)`)
	return err
}

// SubmitFact adds a fact to the pending queue.
func (cp *ConfidencePipeline) SubmitFact(fact core.FactFrame, character, source string) error {
	id := fmt.Sprintf("pending_%s_%s_%s_%d", character, fact.Subject, fact.Predicate, time.Now().Unix())
	conf := cp.sourceConfidence(source)
	_, err := cp.db.Exec(
		`INSERT INTO pending_facts (id, character, subject, predicate, object, source, confidence, confirmations, instance_id, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, character, fact.Subject, fact.Predicate, fact.Object, source, conf, 0, cp.instanceID, time.Now(),
	)
	return err
}

// ConfirmPending increments confirmation for matching facts.
func (cp *ConfidencePipeline) ConfirmPending(character, subject, predicate string) error {
	_, err := cp.db.Exec(
		`UPDATE pending_facts SET confirmations = confirmations + 1, confidence = confidence + 0.15
		 WHERE character = ? AND subject = ? AND predicate = ?`+cp.instanceScopeSuffix(" AND "),
		cp.instanceScopeArgs(character, subject, predicate)...)
	return err
}

func (cp *ConfidencePipeline) ListPending(limit int, character string) ([]core.PendingFact, error) {
	if limit <= 0 {
		limit = 50
	}
	var (
		rows *sql.Rows
		err  error
	)
	if strings.TrimSpace(character) != "" {
		where, args := cp.instanceScopeArgsWithWhere(character)
		query := `SELECT id, character, subject, predicate, object, source, confidence, confirmations, created_at
			 FROM pending_facts WHERE character = ?`
		if where != "" {
			query += " AND " + where
		}
		query += " ORDER BY created_at DESC LIMIT ?"
		args = append(args, limit)
		rows, err = cp.db.Query(
			query, args...,
		)
	} else {
		where, args := cp.instanceScopeArgsWithWhere()
		query := `SELECT id, character, subject, predicate, object, source, confidence, confirmations, created_at
			 FROM pending_facts`
		if where != "" {
			query += " WHERE " + where
		}
		query += " ORDER BY created_at DESC LIMIT ?"
		args = append(args, limit)
		rows, err = cp.db.Query(
			query, args...,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []core.PendingFact
	for rows.Next() {
		var item core.PendingFact
		var createdAtRaw string
		if err := rows.Scan(
			&item.ID, &item.Character, &item.Subject, &item.Predicate, &item.Object,
			&item.Source, &item.Confidence, &item.Confirmations, &createdAtRaw,
		); err != nil {
			return nil, err
		}
		item.CreatedAt = parsePendingTime(createdAtRaw)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (cp *ConfidencePipeline) ConfirmPendingByID(id string) error {
	_, err := cp.db.Exec(
		`UPDATE pending_facts SET confirmations = confirmations + 1, confidence = confidence + 0.15 WHERE id = ?`+cp.instanceScopeSuffix(" AND "),
		cp.instanceScopeArgs(id)...,
	)
	return err
}

func (cp *ConfidencePipeline) DeletePendingByID(id string) error {
	_, err := cp.db.Exec(`DELETE FROM pending_facts WHERE id = ?`+cp.instanceScopeSuffix(" AND "), cp.instanceScopeArgs(id)...)
	return err
}

func (cp *ConfidencePipeline) PromotePendingByID(id string) (core.FactFrame, error) {
	var item core.PendingFact
	var createdAtRaw string
	err := cp.db.QueryRow(
		`SELECT id, character, subject, predicate, object, source, confidence, confirmations, created_at
		 FROM pending_facts WHERE id = ?`+cp.instanceScopeSuffix(" AND "), cp.instanceScopeArgs(id)...,
	).Scan(&item.ID, &item.Character, &item.Subject, &item.Predicate, &item.Object,
		&item.Source, &item.Confidence, &item.Confirmations, &createdAtRaw)
	if err != nil {
		return core.FactFrame{}, err
	}
	item.CreatedAt = parsePendingTime(createdAtRaw)

	fact := core.FactFrame{
		Subject:    item.Subject,
		Predicate:  item.Predicate,
		Object:     item.Object,
		Confidence: item.Confidence,
	}
	if err := cp.insertCanonicalFact(item.Character, fact); err != nil {
		return core.FactFrame{}, err
	}
	if err := cp.DeletePendingByID(id); err != nil {
		return core.FactFrame{}, err
	}
	return fact, nil
}

// ProcessPending promotes facts that meet the confidence threshold.
func (cp *ConfidencePipeline) ProcessPending(threshold float64) ([]core.FactFrame, error) {
	items, err := cp.listPromotable(threshold)
	if err != nil {
		return nil, err
	}
	var promoted []core.FactFrame
	for _, item := range items {
		f := core.FactFrame{
			Subject:    item.Subject,
			Predicate:  item.Predicate,
			Object:     item.Object,
			Confidence: item.Confidence,
		}
		if err := cp.insertCanonicalFact(item.Character, f); err != nil {
			continue
		}
		promoted = append(promoted, f)
		_ = cp.DeletePendingByID(item.ID)
	}
	return promoted, nil
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
	row := cp.db.QueryRow(`SELECT COUNT(*) FROM pending_facts`+cp.instanceScopeSuffix(" WHERE "), cp.instanceScopeArgs()...)
	if err := row.Scan(&count); err != nil {
		return 0
	}
	return count
}

func (cp *ConfidencePipeline) PendingStats() map[string]interface{} {
	stats := map[string]interface{}{
		"pending_total": 0,
		"by_source":     map[string]int{},
		"by_character":  map[string]int{},
	}
	items, err := cp.ListPending(500, "")
	if err != nil {
		return stats
	}
	stats["pending_total"] = len(items)
	bySource := stats["by_source"].(map[string]int)
	byCharacter := stats["by_character"].(map[string]int)
	for _, item := range items {
		bySource[item.Source]++
		byCharacter[item.Character]++
	}
	return stats
}

func (cp *ConfidencePipeline) listPromotable(threshold float64) ([]core.PendingFact, error) {
	rows, err := cp.db.Query(
		`SELECT id, character, subject, predicate, object, source, confidence, confirmations, created_at
		 FROM pending_facts WHERE confidence >= ?`+cp.instanceScopeSuffix(" AND ")+` ORDER BY created_at ASC`, cp.instanceScopeArgs(threshold)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []core.PendingFact
	for rows.Next() {
		var item core.PendingFact
		var createdAtRaw string
		if err := rows.Scan(&item.ID, &item.Character, &item.Subject, &item.Predicate, &item.Object, &item.Source, &item.Confidence, &item.Confirmations, &createdAtRaw); err != nil {
			return nil, err
		}
		item.CreatedAt = parsePendingTime(createdAtRaw)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (cp *ConfidencePipeline) insertCanonicalFact(character string, fact core.FactFrame) error {
	id := fmt.Sprintf("fact_%s_%s_%s_%d", character, fact.Subject, fact.Predicate, time.Now().UnixNano())
	_, err := cp.db.Exec(
		`INSERT INTO semantic_facts (id, character, subject, predicate, object, confidence, instance_id, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, character, fact.Subject, fact.Predicate, fact.Object, fact.Confidence, cp.instanceID, time.Now(),
	)
	return err
}

func (cp *ConfidencePipeline) instanceScopeSuffix(prefix string) string {
	where, _ := cp.instanceScopeArgsWithWhere()
	if where == "" {
		return ""
	}
	return prefix + where
}

func (cp *ConfidencePipeline) instanceScopeArgs(prefixArgs ...interface{}) []interface{} {
	args := append([]interface{}{}, prefixArgs...)
	switch strings.TrimSpace(cp.instanceID) {
	case "":
		return args
	case "default":
		return append(args, "default")
	default:
		return append(args, cp.instanceID)
	}
}

func (cp *ConfidencePipeline) instanceScopeArgsWithWhere(prefixArgs ...interface{}) (string, []interface{}) {
	switch strings.TrimSpace(cp.instanceID) {
	case "":
		return "", append([]interface{}{}, prefixArgs...)
	case "default":
		return `(instance_id = ? OR COALESCE(instance_id, '') = '')`, append(append([]interface{}{}, prefixArgs...), "default")
	default:
		return `instance_id = ?`, append(append([]interface{}{}, prefixArgs...), cp.instanceID)
	}
}

func parsePendingTime(raw string) time.Time {
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
	return time.Time{}
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

func ApplyFactDecayForInstance(db *sql.DB, threshold float64, instanceID string) (deleted int, err error) {
	cp := NewConfidencePipelineForInstance(db, instanceID)
	query := `UPDATE semantic_facts SET confidence = confidence * 0.99`
	if scope := cp.instanceScopeSuffix(" WHERE "); scope != "" {
		query += scope
	}
	if _, err = db.Exec(query, cp.instanceScopeArgs()...); err != nil {
		return 0, err
	}
	delQuery := `DELETE FROM semantic_facts WHERE confidence < ?` + cp.instanceScopeSuffix(" AND ")
	res, err := db.Exec(delQuery, cp.instanceScopeArgs(threshold)...)
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
