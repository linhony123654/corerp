package storage

import (
	"context"
	"corerp.local/backend/internal/core"
	"database/sql"
	"encoding/json"
)

type ownCultureRevision struct {
	sequence   int64
	definition core.RPCulture
	stance     core.RPCultureInternalization
}

// Load only this actor's committed stances and their exact definitions. Later
// internalization cannot retroactively change how an earlier gift was evaluated.
func readOwnCultureHistory(ctx context.Context, conn *sql.Conn, observer string) ([]ownCultureRevision, error) {
	rows, err := conn.QueryContext(ctx, `SELECT e.event_sequence,e.payload,d.payload FROM events e JOIN agent_profiles a ON a.agent_id=? AND a.instance_id=e.instance_id AND a.branch_id=e.branch_id JOIN events d ON d.event_id=json_extract(e.payload,'$.definition_event_id') AND d.instance_id=e.instance_id AND d.branch_id=e.branch_id AND d.event_type='RPCultureFactRecorded' WHERE e.event_type='RPCultureFactRecorded' AND json_extract(e.payload,'$.kind')='internalization' AND json_extract(e.payload,'$.actor_id')=? ORDER BY e.event_sequence`, observer, observer)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []ownCultureRevision
	for rows.Next() {
		var revision ownCultureRevision
		var raw, source string
		var stance, definition CultureFact
		if err := rows.Scan(&revision.sequence, &raw, &source); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(raw), &stance); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(source), &definition); err != nil {
			return nil, err
		}
		if stance.Internalization == nil || definition.Definition == nil {
			return nil, core.NewError(core.CodeProjectionDiverged, "culture history lacks typed source")
		}
		revision.stance = *stance.Internalization
		revision.definition = *definition.Definition
		if _, err := core.EvaluateRPCulture(revision.definition, revision.stance, observer, "gift"); err != nil {
			return nil, err
		}
		result = append(result, revision)
	}
	return result, rows.Err()
}

func evaluateOwnCultureAt(history []ownCultureRevision, sequence int64, observer, action string) ([]core.RPCultureEvaluation, error) {
	latest := map[string]int{}
	order := []string{}
	for i, r := range history {
		if r.sequence >= sequence {
			break
		}
		id := r.stance.CultureID
		if _, ok := latest[id]; !ok {
			order = append(order, id)
		}
		latest[id] = i
	}
	var result []core.RPCultureEvaluation
	for _, id := range order {
		r := history[latest[id]]
		evaluations, err := core.EvaluateRPCulture(r.definition, r.stance, observer, action)
		if err != nil {
			return nil, err
		}
		result = append(result, evaluations...)
	}
	return result, nil
}
