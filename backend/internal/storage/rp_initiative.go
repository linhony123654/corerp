package storage

import (
	"context"
	"corerp.local/backend/internal/core"
	"database/sql"
)

// BuildRPInitiativeInput accepts only an actual completed wait in the caller's
// controlled session. It does not create a turn, utterance, memory or event.
func (s *Store) BuildRPInitiativeInput(ctx context.Context, r core.RPInitiativeRequest) (core.RPDecisionInput, error) {
	if err := r.Validate(); err != nil {
		return core.RPDecisionInput{}, err
	}
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return core.RPDecisionInput{}, err
	}
	defer tx.Rollback(ctx)
	return readRPInitiativeInput(ctx, tx.conn, r)
}

func readRPInitiativeInput(ctx context.Context, conn *sql.Conn, r core.RPInitiativeRequest) (core.RPDecisionInput, error) {
	empty := core.RPDecisionInput{}
	session, err := loadRPSession(ctx, conn, r.PrincipalID, r.SessionID)
	if err != nil {
		return empty, err
	}
	if err := authorizeRPControl(ctx, conn, r.PrincipalID, session.InstanceID, session.BranchID, session.ControlledEntityID); err != nil {
		return empty, err
	}
	if session.Status != "active" || r.NPCEntityID == session.ControlledEntityID {
		return empty, core.NewError(core.CodeInvalidArgument, "initiative requires an active session and an NPC")
	}
	var pending int
	if err := conn.QueryRowContext(ctx, `SELECT (SELECT COUNT(*) FROM rp_turn_runs t JOIN rp_sessions s ON s.session_id=t.session_id WHERE s.instance_id=? AND s.branch_id=? AND t.status<>'settled')+(SELECT COUNT(*) FROM rp_wait_intents w JOIN rp_sessions s ON s.session_id=w.session_id WHERE s.instance_id=? AND s.branch_id=? AND w.status='pending')`, session.InstanceID, session.BranchID, session.InstanceID, session.BranchID).Scan(&pending); err != nil {
		return empty, err
	}
	if pending != 0 {
		return empty, core.NewError(core.CodeCommandInProgress, "finish current RP action before initiative")
	}
	var at string
	var triggerSequence int64
	if err := conn.QueryRowContext(ctx, `SELECT world_time,event_sequence FROM events WHERE event_id=? AND instance_id=? AND branch_id=? AND event_type='RPWaitCompleted' AND json_extract(payload,'$.session_id')=? AND json_extract(payload,'$.entity_id')=? AND EXISTS (SELECT 1 FROM json_each(events.payload,'$.initiative_npc_ids') WHERE value=?)`, r.TriggerEventID, session.InstanceID, session.BranchID, session.SessionID, session.ControlledEntityID, r.NPCEntityID).Scan(&at, &triggerSequence); err != nil {
		return empty, classifyMissing(err, "own completed wait trigger")
	}
	input, err := readRPOwnDecisionContext(ctx, conn, core.RPDecisionInput{
		InstanceID: session.InstanceID, BranchID: session.BranchID, NPCEntityID: r.NPCEntityID, InterlocutorEntityID: session.ControlledEntityID,
		Trigger:      &core.RPDecisionTrigger{Kind: "elapsed_time", SourceEventID: r.TriggerEventID},
		LegalActions: []string{"respond", "silence", "wait"}, VisibleEntities: []core.RPDecisionVisibleEntity{}, Knowledge: []core.RPDecisionKnowledge{}, ReachablePlaceIDs: []string{},
	})
	if err != nil {
		return empty, err
	}
	if input.WorldTime != at {
		return empty, core.NewError(core.CodeBranchConflict, "initiative trigger is no longer current")
	}
	visible := false
	for _, person := range input.VisibleEntities {
		if person.EntityID == session.ControlledEntityID {
			visible = true
		}
	}
	if !visible {
		return empty, core.NewError(core.CodeNotFound, "initiative NPC is not in the player's current scene")
	}
	// Only other initiative commits may intervene while draining one wait.
	var intervening int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM events e JOIN event_batches b ON b.batch_id=e.batch_id JOIN commands c ON c.command_id=b.command_id WHERE e.instance_id=? AND e.branch_id=? AND e.event_sequence>? AND c.command_type<>'RPNPCInitiative'`, session.InstanceID, session.BranchID, triggerSequence).Scan(&intervening); err != nil {
		return empty, err
	}
	if intervening != 0 {
		return empty, core.NewError(core.CodeBranchConflict, "another world action superseded the initiative trigger")
	}
	return input, nil
}
