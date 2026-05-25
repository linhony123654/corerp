package memory

import (
	"database/sql"
	"time"

	"corerp/internal/core"
)

// DecayEngine runs periodic memory and relationship decay.
type DecayEngine struct {
	db        *sql.DB
	factThreshold float64
}

func NewDecayEngine(db *sql.DB) *DecayEngine {
	return &DecayEngine{
		db:            db,
		factThreshold: 0.25,
	}
}

// Tick applies all decay rules. Called by simulation tick loop.
func (de *DecayEngine) Tick(state core.WorldState) (core.WorldState, DecayReport) {
	var report DecayReport

	// 1. Fact decay
	deleted, err := ApplyFactDecay(de.db, de.factThreshold)
	if err == nil {
		report.FactsDeleted = deleted
	}

	// 2. Relationship decay
	before := len(state.Relationships)
	state.Relationships = ApplyRelationshipDecay(state.Relationships)
	report.RelationshipsActive = before

	// 3. Episodic decay: remove very old events
	cutoff := time.Now().Add(-30 * 24 * time.Hour).Format("2006-01-02 15:04:05")
	res, _ := de.db.Exec(`DELETE FROM episodic_events WHERE created_at < ?`, cutoff)
	if res != nil {
		n, _ := res.RowsAffected()
		report.EpisodicPruned = int(n)
	}

	// 4. Process pending facts pipeline
	if cp := NewConfidencePipeline(de.db); cp != nil {
		promoted, _ := cp.ProcessPending(0.75)
		report.FactsPromoted = len(promoted)
		for _, f := range promoted {
			// Insert promoted facts into canonical semantic memory
			id := "fact_" + f.Subject + "_" + f.Predicate + "_" + time.Now().Format("20060102")
			de.db.Exec(
				`INSERT INTO semantic_facts (id, character, subject, predicate, object, confidence, created_at)
				 VALUES (?, ?, ?, ?, ?, ?, ?)
				 ON CONFLICT(id) DO UPDATE SET confidence = excluded.confidence`,
				id, "", f.Subject, f.Predicate, f.Object, f.Confidence, time.Now(),
			)
		}
	}

	return state, report
}

// DecayReport summarizes what decay tick did.
type DecayReport struct {
	FactsDeleted        int
	FactsPromoted       int
	RelationshipsActive int
	EpisodicPruned      int
}
