package storage

import (
	"context"
	"database/sql"

	"corerp.local/backend/internal/core"
)

// Both spoken and time-triggered NPC owners freeze the same witnessed fact.
// The caller has already checked legality, ownership and the current head.
func prepareRPNPCExpression(ctx context.Context, conn *sql.Conn, sessionID string, input core.RPDecisionInput, code string) (*core.RPNonverbalFact, error) {
	if code == "" {
		return nil, nil
	}
	action, gesture, target := code, "", ""
	if code == "beckon" {
		action, gesture, target = "gesture", "beckon", input.InterlocutorEntityID
		visible, err := rpCanPerceive(ctx, conn, input.InstanceID, input.BranchID, input.NPCEntityID, target, "visual", "")
		if err != nil {
			return nil, err
		}
		if !visible {
			return nil, core.NewError(core.CodeBranchConflict, "NPC expression target is no longer visible")
		}
	}
	fact := &core.RPNonverbalFact{ClaimType: "nonverbal_action", SessionID: sessionID, ActorEntityID: input.NPCEntityID, TargetEntityID: target, Action: action, GestureCode: gesture, PlaceID: input.PlaceID, Description: rpNonverbalDescription(action, gesture, target != ""), Witnesses: []core.RPNonverbalWitness{}}
	if input.Trigger != nil && input.Trigger.Kind == "elapsed_time" {
		fact.TriggerEventID = input.Trigger.SourceEventID
	}
	nearby, err := rpCoLocatedEntityIDs(ctx, conn, input.InstanceID, input.BranchID, input.PlaceID, input.NPCEntityID)
	if err != nil {
		return nil, err
	}
	for _, candidate := range nearby {
		visible, err := rpCanPerceive(ctx, conn, input.InstanceID, input.BranchID, candidate, input.NPCEntityID, "visual", "")
		if err != nil {
			return nil, err
		}
		if !visible {
			continue
		}
		witness := core.RPNonverbalWitness{ObserverEntityID: candidate}
		if target != "" {
			witness.TargetVisible, err = rpCanPerceive(ctx, conn, input.InstanceID, input.BranchID, candidate, target, "visual", "")
			if err != nil {
				return nil, err
			}
		}
		fact.Witnesses = append(fact.Witnesses, witness)
	}
	return fact, nil
}

// This runs inside the owner's existing transaction and atomic batch. Public
// claims contain only the witnessed effect and authorized identity handles.
func commitRPNPCExpression(ctx context.Context, conn *sql.Conn, input core.RPDecisionInput, fact *core.RPNonverbalFact, eventID, parentEventID, batchID string, sequence int64) error {
	if fact == nil {
		return nil
	}
	encoded, err := core.CanonicalJSON(fact)
	if err != nil {
		return err
	}
	if err := execAgentOne(ctx, conn, "NPC expression event", `INSERT INTO events(event_id,batch_id,instance_id,branch_id,event_sequence,batch_index,event_type,actor_id,world_time,causation_event_id,payload) VALUES (?,?,?,?,?,1,'RPNonverbalAction',?,?,?,?)`, eventID, batchID, input.InstanceID, input.BranchID, sequence, input.NPCEntityID, input.WorldTime, parentEventID, string(encoded)); err != nil {
		return err
	}
	for _, witness := range fact.Witnesses {
		claim := rpNonverbalWitnessClaim(*fact, witness)
		claimJSON, err := core.CanonicalJSON(claim)
		if err != nil {
			return err
		}
		observationID, claimKey := "observation_"+eventID+"_"+witness.ObserverEntityID, "nonverbal:"+eventID
		if err := execAgentOne(ctx, conn, "NPC expression visual observation", `INSERT INTO observation_records(observation_id,source_event_id,observer_agent_id,subject_agent_id,place_id,channel,observed_world_time,claim_key,claim_payload) VALUES (?,?,?,?,?,'co_location',?,?,?)`, observationID, eventID, witness.ObserverEntityID, fact.ActorEntityID, input.PlaceID, input.WorldTime, claimKey, string(claimJSON)); err != nil {
			return err
		}
		if err := execAgentOne(ctx, conn, "NPC expression witness knowledge", `INSERT INTO agent_knowledge(observer_agent_id,claim_key,subject_agent_id,place_id,source_event_id,observation_id,learned_world_time,claim_payload,projection_version,last_event_sequence) VALUES (?,?,?,?,?,?,?,?,0,?)`, witness.ObserverEntityID, claimKey, fact.ActorEntityID, input.PlaceID, eventID, observationID, input.WorldTime, string(claimJSON), sequence); err != nil {
			return err
		}
		claim.ActorEntityID, err = rpPublicEntityID(ctx, conn, input.InstanceID, input.BranchID, witness.ObserverEntityID, fact.ActorEntityID)
		if err != nil {
			return err
		}
		if claim.TargetEntityID != "" {
			claim.TargetEntityID, err = rpPublicEntityID(ctx, conn, input.InstanceID, input.BranchID, witness.ObserverEntityID, fact.TargetEntityID)
			if err != nil {
				return err
			}
		}
		publicClaim, err := core.CanonicalJSON(claim)
		if err != nil {
			return err
		}
		if err := insertRPParticipantOutbox(ctx, conn, "outbox_"+eventID+"_"+witness.ObserverEntityID, eventID, "rp.nonverbal", input.InstanceID, input.BranchID, witness.ObserverEntityID, nil, publicClaim); err != nil {
			return err
		}
	}
	return insertRPOwnAction(ctx, conn, input.NPCEntityID, eventID, "nonverbal", fact.Action, fact.Description, input.PlaceID, input.WorldTime, "", input.InstanceID, input.BranchID, sequence)
}
