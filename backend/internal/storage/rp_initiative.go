package storage

import (
	"context"
	"corerp.local/backend/internal/core"
	"database/sql"
	"encoding/json"
	"reflect"
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
	var at, triggerPayload string
	var triggerSequence int64
	if err := conn.QueryRowContext(ctx, `SELECT world_time,event_sequence,payload FROM events WHERE event_id=? AND instance_id=? AND branch_id=? AND event_type='RPWaitCompleted' AND json_extract(payload,'$.session_id')=? AND json_extract(payload,'$.entity_id')=? AND EXISTS (SELECT 1 FROM json_each(events.payload,'$.initiative_npc_ids') WHERE value=?)`, r.TriggerEventID, session.InstanceID, session.BranchID, session.SessionID, session.ControlledEntityID, r.NPCEntityID).Scan(&at, &triggerSequence, &triggerPayload); err != nil {
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
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM events e JOIN event_batches b ON b.batch_id=e.batch_id JOIN commands c ON c.command_id=b.command_id WHERE e.instance_id=? AND e.branch_id=? AND e.event_sequence>? AND NOT (c.command_type='RPNPCInitiative' OR (c.command_type='RPWarmDecision' AND COALESCE(json_extract(e.payload,'$.trigger_event_id'),'')=?))`, session.InstanceID, session.BranchID, triggerSequence, r.TriggerEventID).Scan(&intervening); err != nil {
		return empty, err
	}
	if intervening != 0 {
		return empty, core.NewError(core.CodeBranchConflict, "another world action superseded the initiative trigger")
	}
	var trigger rpWaitEvent
	if err := json.Unmarshal([]byte(triggerPayload), &trigger); err != nil {
		return empty, err
	}
	for _, receipt := range trigger.ContactOpportunities {
		if receipt.ActorID != r.NPCEntityID || receipt.TargetID != session.ControlledEntityID {
			continue
		}
		// Evidence was selected from own-known relationships at wait completion.
		// Recheck that it still belongs to this actor's filtered relationship view.
		known := false
		if input.Life != nil {
			for _, relation := range input.Life.Relationships {
				if relation.SubjectEntityID != receipt.TargetID {
					continue
				}
				for _, source := range relation.SourceEventIDs {
					if source == receipt.SourceEventID {
						known = true
					}
				}
			}
		}
		if !known || input.ContactOpportunity != nil {
			return empty, core.NewError(core.CodeProjectionDiverged, "invalid contact receipt knowledge")
		}
		input.ContactOpportunity = &core.RPContactOpportunityContext{SourceEventID: receipt.SourceEventID, Selected: receipt.Draw.Selected}
	}
	seenStores := map[string]bool{}
	if _, err := attachRPVisitReceipt(ctx, conn, &input, trigger.VisitOpportunities); err != nil {
		return empty, err
	}
	for _, receipt := range trigger.CommunityOpportunities {
		if receipt.ActorID != r.NPCEntityID {
			continue
		}
		sources, err := readRPCommunityChangeSources(ctx, conn, session.InstanceID, session.BranchID, r.NPCEntityID, receipt.Source.AvailableSince, input.WorldTime)
		if err != nil {
			return empty, err
		}
		known := false
		for _, source := range sources {
			known = known || reflect.DeepEqual(source, receipt.Source)
		}
		if !known || input.CommunityOpportunity != nil {
			return empty, core.NewError(core.CodeProjectionDiverged, "invalid community receipt knowledge")
		}
		input.CommunityOpportunity = &core.RPCommunityOpportunityContext{Law: receipt.Source.Law, Selected: receipt.Draw.Selected}
	}
	for _, receipt := range trigger.WorkOpportunities {
		if receipt.ActorID != r.NPCEntityID {
			continue
		}
		known := false
		if input.Life != nil {
			for _, memory := range input.Life.SalientMemories {
				known = known || memory == receipt.Memory
			}
		}
		if !known || !core.RPWorkChangeMemory(receipt.Memory, r.NPCEntityID) || input.WorkOpportunity != nil {
			return empty, core.NewError(core.CodeProjectionDiverged, "invalid work receipt knowledge")
		}
		input.WorkOpportunity = &core.RPWorkOpportunityContext{Memory: receipt.Memory, Selected: receipt.Draw.Selected}
	}
	for _, receipt := range trigger.StoreOpportunities {
		if receipt.ActorID != r.NPCEntityID {
			continue
		}
		known := false
		for _, shelf := range input.Stores {
			known = known || shelf == receipt.Store
		}
		if !known || receipt.Store.Available || seenStores[receipt.Store.StorefrontSourceEventID] {
			return empty, core.NewError(core.CodeProjectionDiverged, "invalid store receipt knowledge")
		}
		seenStores[receipt.Store.StorefrontSourceEventID] = true
		input.StoreOpportunities = append(input.StoreOpportunities, core.RPStoreOpportunityContext{Store: receipt.Store, Selected: receipt.Draw.Selected})
	}
	return input, nil
}
