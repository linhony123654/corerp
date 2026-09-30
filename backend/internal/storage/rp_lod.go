package storage

import (
	"context"
	"database/sql"

	"corerp.local/backend/internal/core"
)

func readRPHotInitiativeRoster(ctx context.Context, conn *sql.Conn, instance, branch, place, player string) ([]string, error) {
	present, err := rpPerceivedEntityIDs(ctx, conn, instance, branch, place, player, "visual", "")
	if err != nil {
		return nil, err
	}
	// Only the actual scene's actors are considered. History remains world-
	// scoped across sessions, and aggregates never enter the provider roster.
	rows, err := conn.QueryContext(ctx, `SELECT e.actor_id,MAX(e.event_sequence)
		FROM events e JOIN event_batches b ON b.batch_id=e.batch_id JOIN commands c ON c.command_id=b.command_id
		JOIN agent_positions p ON p.agent_id=e.actor_id JOIN agent_profiles a ON a.agent_id=p.agent_id
		WHERE e.instance_id=? AND e.branch_id=? AND c.command_type='RPNPCInitiative' AND e.batch_index=0
		AND a.instance_id=e.instance_id AND a.branch_id=e.branch_id AND a.status='active'
		AND p.place_id=? AND a.agent_id<>? GROUP BY e.actor_id`, instance, branch, place, player)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	history := map[string]int64{}
	for rows.Next() {
		var actor string
		var sequence int64
		if err := rows.Scan(&actor, &sequence); err != nil {
			return nil, err
		}
		history[actor] = sequence
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return core.SelectRPHotInitiatives(present, history)
}
