package memory

import (
	"database/sql"
	"time"

	"corerp/internal/core"
)

// DecayEngine runs periodic memory and relationship decay.
type DecayEngine struct {
	db            *sql.DB
	instanceID    string
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
	deleted, err := ApplyFactDecayForInstance(de.db, de.factThreshold, de.instanceID)
	if err == nil {
		report.FactsDeleted = deleted
	}

	// 2. Relationship decay
	before := len(state.Relationships)
	state.Relationships = ApplyRelationshipDecay(state.Relationships)
	report.RelationshipsActive = before

	// 3. Episodic decay: remove very old events
	cutoff := time.Now().Add(-30 * 24 * time.Hour).Format("2006-01-02 15:04:05")
	cp := NewConfidencePipelineForInstance(de.db, de.instanceID)
	res, _ := de.db.Exec(`DELETE FROM episodic_events WHERE created_at < ?`+cp.instanceScopeSuffix(" AND "), cp.instanceScopeArgs(cutoff)...)
	if res != nil {
		n, _ := res.RowsAffected()
		report.EpisodicPruned = int(n)
	}

	// 4. Process pending facts pipeline
	if cp := NewConfidencePipelineForInstance(de.db, de.instanceID); cp != nil {
		promoted, _ := cp.ProcessPending(0.75)
		report.FactsPromoted = len(promoted)
	}

	return state, report
}

func (de *DecayEngine) SetInstanceID(id string) {
	de.instanceID = id
}

// DecayReport summarizes what decay tick did.
type DecayReport struct {
	FactsDeleted        int
	FactsPromoted       int
	RelationshipsActive int
	EpisodicPruned      int
}
