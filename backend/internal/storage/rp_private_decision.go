package storage

import (
	"context"
	"database/sql"
	"encoding/json"

	"corerp.local/backend/internal/core"
)

// Read the existing immutable, atomically applied decision record, never an
// audit-only proposal. Scope, owner, committed attempt, head and proposal hash
// are checked against its actual Event. No private world/projection is added.
func readRPOwnPrivateDecisionMemory(ctx context.Context, conn *sql.Conn, input core.RPDecisionInput) ([]core.RPDecisionPrivateMemory, error) {
	rows, err := conn.QueryContext(ctx, `
		SELECT d.decision_id,e.event_id,e.event_sequence,e.world_time,
		       s.controlled_entity_id,d.proposal_json,d.proposal_hash
		FROM rp_npc_decisions d JOIN events e ON e.event_id=d.event_id
		JOIN rp_sessions s ON s.session_id=d.session_id
		JOIN event_batches b ON b.batch_id=e.batch_id
		JOIN commands c ON c.command_id=b.command_id
		JOIN command_attempts a ON a.command_id=b.command_id AND a.attempt_no=b.attempt_no
	WHERE e.instance_id=? AND e.branch_id=? AND e.event_sequence<=? AND b.last_sequence<=?
		  AND d.npc_entity_id=? AND e.actor_id=d.npc_entity_id
		  AND d.parent_turn_id<>? AND c.status='committed' AND a.status='committed'
		  AND c.command_type IN ('RPNPCDecision','RPNPCInitiative')
		  AND a.proposal_hash=d.proposal_hash
		  AND json_type(d.proposal_json,'$.private')='object'
	ORDER BY e.event_sequence DESC LIMIT 3`, input.InstanceID, input.BranchID, input.HeadSequence, input.HeadSequence, input.NPCEntityID, input.TurnID)
	if err != nil {
		return nil, core.WrapError(core.CodeStorageFailure, "read own applied private decisions", err)
	}
	defer rows.Close()
	var memory []core.RPDecisionPrivateMemory
	for rows.Next() {
		var item core.RPDecisionPrivateMemory
		var raw, wantHash string
		if err := rows.Scan(&item.DecisionID, &item.SourceEventID, &item.EventSequence, &item.WorldTime, &item.InterlocutorEntityID, &raw, &wantHash); err != nil {
			return nil, core.WrapError(core.CodeStorageFailure, "scan own applied private decision", err)
		}
		var proposal core.RPDecisionProposal
		if err := json.Unmarshal([]byte(raw), &proposal); err != nil || proposal.Private == nil {
			return nil, core.NewError(core.CodeProjectionDiverged, "own private decision record cannot be decoded")
		}
		hash, err := core.HashJSON(proposal)
		if err != nil || hash != wantHash {
			return nil, core.NewError(core.CodeProjectionDiverged, "own private decision differs from its approved proposal hash")
		}
		item.Private = *proposal.Private
		memory = append(memory, item)
	}
	if err := rows.Err(); err != nil {
		return nil, core.WrapError(core.CodeStorageFailure, "iterate own applied private decisions", err)
	}
	for i, j := 0, len(memory)-1; i < j; i, j = i+1, j-1 {
		memory[i], memory[j] = memory[j], memory[i]
	}
	return memory, nil
}
