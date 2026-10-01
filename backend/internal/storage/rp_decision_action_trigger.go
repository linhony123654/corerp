package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"corerp.local/backend/internal/core"
)

// The parent is an existing player-owned, single-event nonverbal command.
// No speech row, current perception, or new world fact substitutes for it.
func readRPActionTurnSource(ctx context.Context, conn *sql.Conn, session RPSession, parent string) (core.RPNonverbalFact, int64, string, error) {
	var fact core.RPNonverbalFact
	var raw, actor, at string
	var sequence, first, last, head int64
	err := conn.QueryRowContext(ctx, `SELECT e.payload,e.actor_id,e.world_time,e.event_sequence,b.first_sequence,b.last_sequence,r.head_sequence
	 FROM events e JOIN event_batches b ON b.batch_id=e.batch_id
	 JOIN commands c ON c.command_id=b.command_id
	 JOIN command_attempts a ON a.command_id=b.command_id AND a.attempt_no=b.attempt_no
	 JOIN branches r ON r.instance_id=e.instance_id AND r.branch_id=e.branch_id
	 WHERE e.event_id=? AND e.instance_id=? AND e.branch_id=? AND e.event_type='RPNonverbalAction'
	 AND c.instance_id=e.instance_id AND c.branch_id=e.branch_id
	 AND e.batch_index=0 AND (SELECT COUNT(*) FROM events x WHERE x.batch_id=b.batch_id)=1
	 AND c.command_type='RPNonverbalAction' AND c.status='committed' AND a.status='committed'`, parent, session.InstanceID, session.BranchID).Scan(&raw, &actor, &at, &sequence, &first, &last, &head)
	if err != nil {
		return fact, 0, "", classifyMissing(err, "committed action turn source")
	}
	if json.Unmarshal([]byte(raw), &fact) != nil || fact.ClaimType != "nonverbal_action" || fact.SessionID != session.SessionID || actor != session.ControlledEntityID || fact.ActorEntityID != actor || fact.PlaceID == "" || sequence != first || sequence != last || last > head {
		return fact, 0, "", core.NewError(core.CodeProjectionDiverged, "action turn source differs from its committed session batch")
	}
	return fact, sequence, at, nil
}

// Only effects of this exact action's NPC decisions may follow its parent.
// Unrelated committed events fence continuation even if the room is unchanged.
func validateRPActionTurnContinuation(ctx context.Context, conn *sql.Conn, session RPSession, parent string) error {
	_, sequence, _, err := readRPActionTurnSource(ctx, conn, session, parent)
	if err != nil {
		return err
	}
	var unrelated int
	err = conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM events e
	 WHERE e.instance_id=? AND e.branch_id=? AND e.event_sequence>?
	 AND NOT EXISTS (SELECT 1 FROM rp_npc_decisions d JOIN events n ON n.event_id=d.event_id
	 JOIN event_batches b ON b.batch_id=n.batch_id JOIN commands c ON c.command_id=b.command_id
	 JOIN command_attempts a ON a.command_id=b.command_id AND a.attempt_no=b.attempt_no
	 WHERE d.session_id=? AND d.parent_turn_id=? AND n.batch_id=e.batch_id
	 AND n.instance_id=e.instance_id AND n.branch_id=e.branch_id AND n.actor_id=d.npc_entity_id
	 AND c.command_type='RPNPCDecision' AND c.status='committed' AND a.status='committed'
	 AND c.instance_id=e.instance_id AND c.branch_id=e.branch_id
	 AND a.proposal_hash=d.proposal_hash AND n.event_sequence=b.first_sequence AND n.batch_index=0
	 AND b.last_sequence<=(SELECT head_sequence FROM branches WHERE instance_id=e.instance_id AND branch_id=e.branch_id)
	 AND (SELECT COUNT(*) FROM events x WHERE x.batch_id=b.batch_id)=b.last_sequence-b.first_sequence+1
	 AND NOT EXISTS (SELECT 1 FROM events x WHERE x.batch_id=b.batch_id
	  AND (x.instance_id<>e.instance_id OR x.branch_id<>e.branch_id OR x.actor_id<>d.npc_entity_id
	   OR x.event_sequence<b.first_sequence OR x.event_sequence>b.last_sequence OR x.batch_index<>x.event_sequence-b.first_sequence))
	 AND EXISTS (SELECT 1 FROM rp_turn_runs r JOIN rp_turn_listener_activations p ON p.turn_run_id=r.turn_run_id
	 WHERE r.session_id=d.session_id AND r.player_turn_id IS NULL AND r.player_event_id=d.parent_turn_id
	 AND r.trigger_kind='nonverbal' AND p.npc_entity_id=d.npc_entity_id AND p.disposition='activated'))`, session.InstanceID, session.BranchID, sequence, session.SessionID, parent).Scan(&unrelated)
	if err != nil {
		return core.WrapError(core.CodeStorageFailure, "check action turn continuation", err)
	}
	if unrelated != 0 {
		return core.NewError(core.CodeBranchConflict, "unrelated events fence the action turn")
	}
	return nil
}

func readRPDecisionActionTrigger(ctx context.Context, conn *sql.Conn, session RPSession, parent, npc string) (*core.RPDecisionObservedAction, error) {
	if session.Status != "active" || session.TurnCursor != parent || (session.TurnState != "action_committed" && session.TurnState != "npc_effects_committed") {
		return nil, core.NewError(core.CodeBranchConflict, "action decision requires the active action turn")
	}
	var activated int
	err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_turn_runs r JOIN rp_turn_listener_activations a ON a.turn_run_id=r.turn_run_id
	 WHERE r.session_id=? AND r.player_turn_id IS NULL AND r.player_event_id=? AND r.trigger_kind='nonverbal'
	 AND r.status='npc_deciding' AND a.npc_entity_id=? AND a.disposition='activated'
	 AND EXISTS (SELECT 1 FROM json_each(r.listener_ids_json) WHERE value=a.npc_entity_id)`, session.SessionID, parent, npc).Scan(&activated)
	if err != nil {
		return nil, core.WrapError(core.CodeStorageFailure, "read pinned action responder", err)
	}
	if activated != 1 {
		return nil, core.NewError(core.CodeBranchConflict, "NPC is not activated in the pinned action turn")
	}
	if err := validateRPActionTurnContinuation(ctx, conn, session, parent); err != nil {
		return nil, err
	}
	fact, _, at, err := readRPActionTurnSource(ctx, conn, session, parent)
	if err != nil {
		return nil, err
	}
	var knowledgeJSON, observedJSON string
	var consistent bool
	err = conn.QueryRowContext(ctx, `SELECT k.claim_payload,o.claim_payload,
	 (k.subject_agent_id=? AND k.place_id=? AND k.claim_key=? AND o.source_event_id=k.source_event_id
	 AND o.observer_agent_id=k.observer_agent_id AND o.subject_agent_id=k.subject_agent_id
	 AND o.place_id=k.place_id AND o.claim_key=k.claim_key AND o.observed_world_time=?)
	 FROM agent_knowledge k JOIN observation_records o ON o.observation_id=k.observation_id
	 WHERE k.source_event_id=? AND k.observer_agent_id=?`, session.ControlledEntityID, fact.PlaceID, "nonverbal:"+parent, at, parent, npc).Scan(&knowledgeJSON, &observedJSON, &consistent)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, core.NewError(core.CodeNotFound, "NPC did not witness the action turn")
	}
	if err != nil {
		return nil, core.WrapError(core.CodeStorageFailure, "read action turn witness", err)
	}
	var knowledge, observation map[string]any
	var observed core.RPNonverbalClaim
	if !consistent || json.Unmarshal([]byte(knowledgeJSON), &knowledge) != nil || json.Unmarshal([]byte(observedJSON), &observation) != nil || json.Unmarshal([]byte(observedJSON), &observed) != nil {
		return nil, core.NewError(core.CodeProjectionDiverged, "invalid action turn witness")
	}
	knowledgeHash, err := core.HashJSON(knowledge)
	if err != nil {
		return nil, err
	}
	observationHash, err := core.HashJSON(observation)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(fact)
	if err != nil {
		return nil, err
	}
	if knowledgeHash != observationHash || !rpDecisionNonverbalWitnessMatchesSource(string(raw), observation, npc, session.ControlledEntityID, fact.PlaceID) || observed.Action == "" || observed.TargetEntityID != npc {
		return nil, core.NewError(core.CodeProjectionDiverged, "action trigger differs from its frozen witness")
	}
	return &core.RPDecisionObservedAction{SourceEventID: parent, ActorEntityID: session.ControlledEntityID, TargetEntityID: observed.TargetEntityID, Action: observed.Action, GestureCode: observed.GestureCode, PlaceID: fact.PlaceID, WorldTime: at}, nil
}
