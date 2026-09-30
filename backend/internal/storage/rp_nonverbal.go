package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"corerp.local/backend/internal/core"
)

type RPNonverbalResult struct {
	EventID       string `json:"event_id"`
	EventSequence int64  `json:"event_sequence"`
	WorldTime     string `json:"world_time"`
	Description   string `json:"description"`
	Replayed      bool   `json:"replayed"`
}

// NonverbalRP commits only the actor's visible expression. It does not assert
// that a target responded, interpret their feelings, or change a relationship.
func (s *Store) NonverbalRP(ctx context.Context, request core.RPNonverbalRequest) (RPNonverbalResult, error) {
	var empty RPNonverbalResult
	if err := request.Validate(); err != nil {
		return empty, err
	}
	requestHash, err := core.HashJSON(request)
	if err != nil {
		return empty, err
	}
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return empty, err
	}
	defer tx.Rollback(ctx)
	session, err := loadRPSessionRecord(ctx, tx.conn, request.PrincipalID, request.SessionID)
	if err != nil {
		return empty, err
	}
	key := "rp_nonverbal:" + session.SessionID + ":" + request.IdempotencyKey
	if err := checkRPTypedActionRetirement(ctx, tx.conn, request.PrincipalID, "nonverbal", session.SessionID, request.IdempotencyKey); err != nil {
		return empty, err
	}
	var commandID, oldHash, status string
	err = tx.conn.QueryRowContext(ctx, `SELECT command_id,request_hash,status FROM commands WHERE instance_id=? AND branch_id=? AND command_type='RPNonverbalAction' AND idempotency_key=?`, session.InstanceID, session.BranchID, key).Scan(&commandID, &oldHash, &status)
	if err == nil {
		if oldHash != requestHash {
			return empty, core.NewError(core.CodeIdempotencyMismatch, "nonverbal action key payload differs")
		}
		if status != "committed" {
			return empty, core.NewError(core.CodeCommandInProgress, "nonverbal action command is not committed")
		}
		return loadRPNonverbalResult(ctx, tx.conn, commandID, true)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return empty, core.WrapError(core.CodeStorageFailure, "read nonverbal command", err)
	}
	if err := requireCurrentRPSession(ctx, tx.conn, session); err != nil {
		return empty, err
	}
	if err := authorizeRPControl(ctx, tx.conn, request.PrincipalID, session.InstanceID, session.BranchID, session.ControlledEntityID); err != nil {
		return empty, err
	}
	if session.Status != "active" {
		return empty, core.NewError(core.CodeBranchConflict, "RP session is closed")
	}
	if err := requireNoActiveRPSharedRound(ctx, tx.conn, session.InstanceID, session.BranchID); err != nil {
		return empty, err
	}
	if err := validateRPBinding(ctx, tx.conn, session.InstanceID, session.BranchID, session.ControlledEntityID); err != nil {
		return empty, err
	}
	var pending int
	if err := tx.conn.QueryRowContext(ctx, `SELECT
	 (SELECT COUNT(*) FROM rp_turn_runs t JOIN rp_sessions s ON s.session_id=t.session_id WHERE s.instance_id=? AND s.branch_id=? AND t.status<>'settled')+
	 (SELECT COUNT(*) FROM rp_wait_intents w JOIN rp_sessions s ON s.session_id=w.session_id WHERE s.instance_id=? AND s.branch_id=? AND w.status='pending')`, session.InstanceID, session.BranchID, session.InstanceID, session.BranchID).Scan(&pending); err != nil {
		return empty, core.WrapError(core.CodeStorageFailure, "check pending RP action", err)
	}
	if pending != 0 {
		return empty, core.NewError(core.CodeCommandInProgress, "finish pending RP action before nonverbal action")
	}
	var head int64
	var worldTime, placeID string
	err = tx.conn.QueryRowContext(ctx, `SELECT b.head_sequence,c.current_world_time,p.place_id FROM branches b
	 JOIN world_clocks c ON c.instance_id=b.instance_id AND c.branch_id=b.branch_id
	 JOIN agent_positions p ON p.agent_id=? WHERE b.instance_id=? AND b.branch_id=?`, session.ControlledEntityID, session.InstanceID, session.BranchID).Scan(&head, &worldTime, &placeID)
	if err != nil {
		return empty, classifyMissing(err, "nonverbal actor state")
	}
	if head != request.ExpectedCursor || session.ObservationCursor != head {
		return empty, core.NewError(core.CodeBranchConflict, "observe current world before nonverbal action")
	}
	if request.TargetEntityID != "" {
		// Public aliases are resolvable. Raw IDs must already be known, rather
		// than letting an unobserved actor be guessed into an action.
		if !strings.HasPrefix(request.TargetEntityID, "person_") {
			known, err := rpIdentityKnown(ctx, tx.conn, session.InstanceID, session.BranchID, session.ControlledEntityID, request.TargetEntityID)
			if err != nil {
				return empty, err
			}
			if !known {
				return empty, core.NewError(core.CodeNotFound, "unidentified nonverbal target")
			}
		}
		request.TargetEntityID, err = rpResolvePublicEntityID(ctx, tx.conn, session.InstanceID, session.BranchID, session.ControlledEntityID, request.TargetEntityID)
		if err != nil {
			return empty, err
		}
		if request.TargetEntityID == session.ControlledEntityID {
			return empty, core.NewError(core.CodeInvalidArgument, "nonverbal target must be another actor")
		}
		if err := validateRPBinding(ctx, tx.conn, session.InstanceID, session.BranchID, request.TargetEntityID); err != nil {
			return empty, err
		}
		visible, err := rpCanPerceive(ctx, tx.conn, session.InstanceID, session.BranchID, session.ControlledEntityID, request.TargetEntityID, "visual", "")
		if err != nil {
			return empty, err
		}
		if !visible {
			return empty, core.NewError(core.CodeNotFound, "nonverbal target is not visible")
		}
	}
	if err := ensureCohortTransitionChronology(ctx, tx.conn, session.InstanceID, session.BranchID, worldTime); err != nil {
		return empty, err
	}
	nearby, err := rpCoLocatedEntityIDs(ctx, tx.conn, session.InstanceID, session.BranchID, placeID, session.ControlledEntityID)
	if err != nil {
		return empty, err
	}
	fact := core.RPNonverbalFact{ClaimType: "nonverbal_action", SessionID: session.SessionID, ActorEntityID: session.ControlledEntityID, TargetEntityID: request.TargetEntityID, Action: request.Action, GestureCode: request.GestureCode, PlaceID: placeID, Description: rpNonverbalDescription(request.Action, request.GestureCode, request.TargetEntityID != ""), Witnesses: []core.RPNonverbalWitness{}}
	for _, candidate := range nearby {
		actorVisible, err := rpCanPerceive(ctx, tx.conn, session.InstanceID, session.BranchID, candidate, session.ControlledEntityID, "visual", "")
		if err != nil {
			return empty, err
		}
		if !actorVisible {
			continue
		}
		witness := core.RPNonverbalWitness{ObserverEntityID: candidate}
		if fact.TargetEntityID != "" {
			witness.TargetVisible, err = rpCanPerceive(ctx, tx.conn, session.InstanceID, session.BranchID, candidate, fact.TargetEntityID, "visual", "")
			if err != nil {
				return empty, err
			}
		}
		fact.Witnesses = append(fact.Witnesses, witness)
	}
	sequence := head + 1
	var epochID string
	if err := tx.conn.QueryRowContext(ctx, `SELECT epoch_id FROM rule_epochs WHERE instance_id=? AND branch_id=? AND start_sequence<=? AND (end_sequence IS NULL OR ?<end_sequence)`, session.InstanceID, session.BranchID, sequence, sequence).Scan(&epochID); err != nil {
		return empty, classifyMissing(err, "nonverbal rule epoch")
	}
	keyHash, err := core.HashJSON(key)
	if err != nil {
		return empty, err
	}
	suffix := keyHash[7:]
	commandID, attemptID := "cmd_rp_nonverbal_"+suffix, "attempt_rp_nonverbal_"+suffix
	batchID, eventID := "batch_rp_nonverbal_"+suffix, "event_rp_nonverbal_"+suffix
	encoded, err := core.CanonicalJSON(fact)
	if err != nil {
		return empty, err
	}
	batchHash, err := core.HashJSON(struct {
		RequestHash string               `json:"request_hash"`
		Sequence    int64                `json:"sequence"`
		WorldTime   string               `json:"world_time"`
		Fact        core.RPNonverbalFact `json:"fact"`
	}{requestHash, sequence, worldTime, fact})
	if err != nil {
		return empty, err
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	for _, statement := range []struct {
		name, query string
		args        []any
	}{
		{"nonverbal command", `INSERT INTO commands(command_id,instance_id,branch_id,command_type,idempotency_key,request_hash,expected_head,principal_id,command_policy,status,created_at_utc) VALUES (?,?,?,'RPNonverbalAction',?,?,?,?,'{"authorization":"rp-session-control"}','pending',?)`, []any{commandID, session.InstanceID, session.BranchID, key, requestHash, head, request.PrincipalID, now}},
		{"nonverbal attempt", `INSERT INTO command_attempts(command_id,attempt_no,attempt_id,status,lease_owner,lease_until_utc,proposal_hash,created_at_utc) VALUES (?,1,?,'ready','corerp-rp2',?,?,?)`, []any{commandID, attemptID, s.now().UTC().Add(30 * time.Second).Format(time.RFC3339Nano), requestHash, now}},
		{"nonverbal batch", `INSERT INTO event_batches(batch_id,command_id,attempt_no,instance_id,branch_id,epoch_id,expected_head,first_sequence,last_sequence,event_count,world_time,batch_hash,committed_at_utc) VALUES (?,?,1,?,?,?,?,?,?,1,?,?,?)`, []any{batchID, commandID, session.InstanceID, session.BranchID, epochID, head, sequence, sequence, worldTime, batchHash, now}},
		{"nonverbal event", `INSERT INTO events(event_id,batch_id,instance_id,branch_id,event_sequence,batch_index,event_type,actor_id,world_time,payload) VALUES (?,?,?,?,?,0,'RPNonverbalAction',?,?,?)`, []any{eventID, batchID, session.InstanceID, session.BranchID, sequence, session.ControlledEntityID, worldTime, string(encoded)}},
	} {
		if err := execAgentOne(ctx, tx.conn, statement.name, statement.query, statement.args...); err != nil {
			return empty, err
		}
	}
	for _, witness := range fact.Witnesses {
		claim := rpNonverbalWitnessClaim(fact, witness)
		claimJSON, err := core.CanonicalJSON(claim)
		if err != nil {
			return empty, err
		}
		observationID, claimKey := "observation_"+eventID+"_"+witness.ObserverEntityID, "nonverbal:"+eventID
		if err := execAgentOne(ctx, tx.conn, "nonverbal visual observation", `INSERT INTO observation_records(observation_id,source_event_id,observer_agent_id,subject_agent_id,place_id,channel,observed_world_time,claim_key,claim_payload) VALUES (?,?,?,?,?,'co_location',?,?,?)`, observationID, eventID, witness.ObserverEntityID, fact.ActorEntityID, placeID, worldTime, claimKey, string(claimJSON)); err != nil {
			return empty, err
		}
		if err := execAgentOne(ctx, tx.conn, "nonverbal witness knowledge", `INSERT INTO agent_knowledge(observer_agent_id,claim_key,subject_agent_id,place_id,source_event_id,observation_id,learned_world_time,claim_payload,projection_version,last_event_sequence) VALUES (?,?,?,?,?,?,?,?,0,?)`, witness.ObserverEntityID, claimKey, fact.ActorEntityID, placeID, eventID, observationID, worldTime, string(claimJSON), sequence); err != nil {
			return empty, err
		}
		// Outbox payloads are individually addressed and use only public
		// identity handles. An unseen target is absent, not merely renamed.
		claim.ActorEntityID, err = rpPublicEntityID(ctx, tx.conn, session.InstanceID, session.BranchID, witness.ObserverEntityID, fact.ActorEntityID)
		if err != nil {
			return empty, err
		}
		if claim.TargetEntityID != "" {
			claim.TargetEntityID, err = rpPublicEntityID(ctx, tx.conn, session.InstanceID, session.BranchID, witness.ObserverEntityID, fact.TargetEntityID)
			if err != nil {
				return empty, err
			}
		}
		publicClaim, err := core.CanonicalJSON(claim)
		if err != nil {
			return empty, err
		}
		if err := insertRPParticipantOutbox(ctx, tx.conn, "outbox_"+eventID+"_"+witness.ObserverEntityID, eventID, "rp.nonverbal", session.InstanceID, session.BranchID, witness.ObserverEntityID, nil, publicClaim); err != nil {
			return empty, err
		}
	}
	if err := insertRPOwnAction(ctx, tx.conn, fact.ActorEntityID, eventID, "nonverbal", fact.Action, fact.Description, placeID, worldTime, "", session.InstanceID, session.BranchID, sequence); err != nil {
		return empty, err
	}
	if err := execAgentOne(ctx, tx.conn, "nonverbal clock lineage", `UPDATE world_clocks SET projection_version=projection_version+1,last_event_sequence=? WHERE instance_id=? AND branch_id=? AND current_world_time=?`, sequence, session.InstanceID, session.BranchID, worldTime); err != nil {
		return empty, err
	}
	if err := execAgentOne(ctx, tx.conn, "nonverbal branch head", `UPDATE branches SET head_sequence=? WHERE instance_id=? AND branch_id=? AND head_sequence=?`, sequence, session.InstanceID, session.BranchID, head); err != nil {
		return empty, err
	}
	if err := s.insertAgentAudit(ctx, tx.conn, "audit_"+commandID, "observation", eventID, commandID, attemptID, worldTime, now, encoded); err != nil {
		return empty, err
	}
	// The actor need not see every witness: never route the privileged Event
	// payload (which freezes all witnesses' IDs) through the participant outbox.
	ownClaim := core.RPNonverbalClaim{ClaimType: fact.ClaimType, ActorEntityID: fact.ActorEntityID, TargetEntityID: fact.TargetEntityID, Action: fact.Action, GestureCode: fact.GestureCode, Description: fact.Description}
	if ownClaim.TargetEntityID != "" {
		ownClaim.TargetEntityID, err = rpPublicEntityID(ctx, tx.conn, session.InstanceID, session.BranchID, fact.ActorEntityID, fact.TargetEntityID)
		if err != nil {
			return empty, err
		}
	}
	ownPayload, err := core.CanonicalJSON(ownClaim)
	if err != nil {
		return empty, err
	}
	if err := insertRPParticipantOutbox(ctx, tx.conn, "outbox_"+eventID+"_"+fact.ActorEntityID, eventID, "rp.nonverbal", session.InstanceID, session.BranchID, fact.ActorEntityID, nil, ownPayload); err != nil {
		return empty, err
	}
	if err := execAgentOne(ctx, tx.conn, "commit nonverbal command", `UPDATE commands SET status='committed' WHERE command_id=? AND status='pending'`, commandID); err != nil {
		return empty, err
	}
	if err := execAgentOne(ctx, tx.conn, "commit nonverbal attempt", `UPDATE command_attempts SET status='committed',finished_at_utc=? WHERE attempt_id=? AND status='ready'`, now, attemptID); err != nil {
		return empty, err
	}
	if s.beforeCommit != nil {
		if err := s.beforeCommit(); err != nil {
			return empty, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return empty, core.WrapError(core.CodeStorageFailure, "commit nonverbal action", err)
	}
	return RPNonverbalResult{EventID: eventID, EventSequence: sequence, WorldTime: worldTime, Description: fact.Description}, nil
}

func loadRPNonverbalResult(ctx context.Context, conn *sql.Conn, commandID string, replayed bool) (RPNonverbalResult, error) {
	var result RPNonverbalResult
	var raw string
	if err := conn.QueryRowContext(ctx, `SELECT e.event_id,e.event_sequence,e.world_time,e.payload FROM events e JOIN event_batches b ON b.batch_id=e.batch_id WHERE b.command_id=? AND e.event_type='RPNonverbalAction'`, commandID).Scan(&result.EventID, &result.EventSequence, &result.WorldTime, &raw); err != nil {
		return RPNonverbalResult{}, classifyMissing(err, "committed nonverbal action")
	}
	var fact core.RPNonverbalFact
	if err := json.Unmarshal([]byte(raw), &fact); err != nil {
		return RPNonverbalResult{}, core.WrapError(core.CodeProjectionDiverged, "decode nonverbal receipt", err)
	}
	result.Description, result.Replayed = fact.Description, replayed
	return result, nil
}

func rpNonverbalWitnessClaim(fact core.RPNonverbalFact, witness core.RPNonverbalWitness) core.RPNonverbalClaim {
	claim := core.RPNonverbalClaim{ClaimType: "nonverbal_action", ActorEntityID: fact.ActorEntityID, Action: fact.Action, GestureCode: fact.GestureCode}
	if witness.TargetVisible {
		claim.TargetEntityID = fact.TargetEntityID
	}
	claim.Description = rpNonverbalDescription(fact.Action, fact.GestureCode, claim.TargetEntityID != "")
	return claim
}

func rpNonverbalDescription(action, gesture string, targetVisible bool) string {
	switch action {
	case "look_at":
		if targetVisible {
			return "有人看向另一人。"
		}
		return "有人看向某处。"
	case "smile":
		return "有人露出微笑。"
	case "nod":
		return "有人点了点头。"
	case "shake_head":
		return "有人摇了摇头。"
	case "turn_away":
		return "有人转过身。"
	case "frown":
		return "有人皱了皱眉。"
	case "gesture":
		switch gesture {
		case "beckon":
			if targetVisible {
				return "有人向另一人招手。"
			}
			return "有人招了招手。"
		case "wave":
			return "有人挥了挥手。"
		case "shrug":
			return "有人耸了耸肩。"
		case "raise_hand":
			return "有人举起手。"
		}
	}
	return "有人做出一个动作。"
}
