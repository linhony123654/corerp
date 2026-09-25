package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"corerp.local/backend/internal/core"
)

type RPSocialResult struct {
	EventID       string `json:"event_id"`
	EventSequence int64  `json:"event_sequence"`
	Description   string `json:"description"`
	Replayed      bool   `json:"replayed"`
}

// SocialRP commits actual gestures, gifts and explicit meeting commitments.
// These are typed actions, not claims extracted from prose. Financial gifts
// use the existing posted journal/balance authority; both participants receive
// matching observation/Knowledge in the same transaction.
func (s *Store) SocialRP(ctx context.Context, r core.RPSocialRequest) (RPSocialResult, error) {
	var empty RPSocialResult
	if err := r.Validate(); err != nil {
		return empty, err
	}
	if r.MeetingWorldTime != "" {
		instant, _ := time.Parse(time.RFC3339, r.MeetingWorldTime)
		r.MeetingWorldTime = instant.UTC().Format(time.RFC3339)
	}
	requestHash, err := core.HashJSON(r)
	if err != nil {
		return empty, err
	}
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return empty, err
	}
	defer tx.Rollback(ctx)
	session, err := loadRPSessionRecord(ctx, tx.conn, r.PrincipalID, r.SessionID)
	if err != nil {
		return empty, err
	}
	key := "rp_social:" + session.SessionID + ":" + r.IdempotencyKey
	if err := checkRPRequestRetirement(ctx, tx.conn, r.PrincipalID, "social", session.SessionID, r.IdempotencyKey); err != nil {
		return empty, err
	}
	var oldHash, payload string
	var prior RPSocialResult
	err = tx.conn.QueryRowContext(ctx, `SELECT c.request_hash,e.event_id,e.event_sequence,e.payload FROM commands c
 JOIN event_batches b ON b.command_id=c.command_id JOIN events e ON e.batch_id=b.batch_id
 WHERE c.instance_id=? AND c.branch_id=? AND c.command_type='RPSocial' AND c.idempotency_key=?`, session.InstanceID, session.BranchID, key).Scan(&oldHash, &prior.EventID, &prior.EventSequence, &payload)
	if err == nil {
		if oldHash != requestHash {
			return empty, core.NewError(core.CodeIdempotencyMismatch, "interpersonal key payload differs")
		}
		var evidence core.RPSocialEvidence
		if err := json.Unmarshal([]byte(payload), &evidence); err != nil {
			return empty, err
		}
		prior.Description = evidence.Description
		prior.Replayed = true
		return prior, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return empty, err
	}
	if err := requireCurrentRPSession(ctx, tx.conn, session); err != nil {
		return empty, err
	}
	if err := authorizeRPControl(ctx, tx.conn, r.PrincipalID, session.InstanceID, session.BranchID, session.ControlledEntityID); err != nil {
		return empty, err
	}
	if err := requireNoActiveRPSharedRound(ctx, tx.conn, session.InstanceID, session.BranchID); err != nil {
		return empty, err
	}
	if !strings.HasPrefix(r.TargetEntityID, "person_") {
		known, err := rpIdentityKnown(ctx, tx.conn, session.InstanceID, session.BranchID, session.ControlledEntityID, r.TargetEntityID)
		if err != nil {
			return empty, err
		}
		if !known {
			return empty, core.NewError(core.CodeNotFound, "unidentified interpersonal target")
		}
	}
	r.TargetEntityID, err = rpResolvePublicEntityID(ctx, tx.conn, session.InstanceID, session.BranchID, session.ControlledEntityID, r.TargetEntityID)
	if err != nil {
		return empty, err
	}
	if session.Status != "active" || session.ControlledEntityID == r.TargetEntityID {
		return empty, core.NewError(core.CodeInvalidArgument, "active session and distinct target required")
	}
	if err := validateRPBinding(ctx, tx.conn, session.InstanceID, session.BranchID, session.ControlledEntityID); err != nil {
		return empty, err
	}
	var pending int
	if err := tx.conn.QueryRowContext(ctx, `SELECT
 (SELECT COUNT(*) FROM rp_turn_runs t JOIN rp_sessions s ON s.session_id=t.session_id WHERE s.instance_id=? AND s.branch_id=? AND t.status<>'settled')+
 (SELECT COUNT(*) FROM rp_wait_intents w JOIN rp_sessions s ON s.session_id=w.session_id WHERE s.instance_id=? AND s.branch_id=? AND w.status='pending')`, session.InstanceID, session.BranchID, session.InstanceID, session.BranchID).Scan(&pending); err != nil {
		return empty, err
	}
	if pending != 0 {
		return empty, core.NewError(core.CodeCommandInProgress, "finish pending RP action first")
	}
	var head int64
	var worldTime, place string
	err = tx.conn.QueryRowContext(ctx, `SELECT b.head_sequence,c.current_world_time,p.place_id
 FROM branches b JOIN world_clocks c ON c.instance_id=b.instance_id AND c.branch_id=b.branch_id
 JOIN agent_positions p ON p.agent_id=?
 WHERE b.instance_id=? AND b.branch_id=?`, session.ControlledEntityID, session.InstanceID, session.BranchID).Scan(&head, &worldTime, &place)
	if err != nil {
		return empty, err
	}
	if head != r.ExpectedCursor || session.ObservationCursor != r.ExpectedCursor {
		return empty, core.NewError(core.CodeBranchConflict, "observe current world before interpersonal action")
	}
	var present int
	err = tx.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM agent_profiles a JOIN agent_positions p ON p.agent_id=a.agent_id
	JOIN materialized_entities n ON n.entity_id=a.agent_id JOIN cohorts source ON source.cohort_id=n.source_cohort_id
	WHERE a.agent_id=? AND a.instance_id=? AND a.branch_id=? AND source.instance_id=a.instance_id AND source.branch_id=a.branch_id
	AND a.status='active' AND n.status='active' AND n.population_count=1 AND p.place_id=?`, r.TargetEntityID, session.InstanceID, session.BranchID, place).Scan(&present)
	if err != nil {
		return empty, err
	}
	if present != 1 {
		return empty, core.NewError(core.CodeNotFound, "present interpersonal target not found")
	}
	visible, err := rpCanPerceive(ctx, tx.conn, session.InstanceID, session.BranchID, session.ControlledEntityID, r.TargetEntityID, "visual", "")
	if err != nil {
		return empty, err
	}
	if !visible {
		return empty, core.NewError(core.CodeNotFound, "interpersonal target is not visible")
	}
	if err := ensureCohortTransitionChronology(ctx, tx.conn, session.InstanceID, session.BranchID, worldTime); err != nil {
		return empty, err
	}
	evidence := core.RPSocialEvidence{ClaimType: "interpersonal_action", SessionID: session.SessionID, ActorEntityID: session.ControlledEntityID, TargetEntityID: r.TargetEntityID, Action: r.Action, PlaceID: place, AmountMinor: r.AmountMinor, PromiseEventID: r.PromiseEventID, MeetingPlaceID: r.MeetingPlaceID, MeetingWorldTime: r.MeetingWorldTime}
	switch r.Action {
	case "greet":
		evidence.Description = "有人向另一人挥手致意。"
	case "insult":
		evidence.Description = "有人对另一人做出了冒犯的手势。"
	case "apologize":
		evidence.Description = "有人向另一人表达歉意。"
	case "gift":
		evidence.Description = "有人赠予另一人一笔钱。"
	case "promise_meeting":
		now, _ := time.Parse(time.RFC3339, worldTime)
		at, _ := time.Parse(time.RFC3339, r.MeetingWorldTime)
		if !at.After(now) || at.After(now.Add(7*24*time.Hour)) {
			return empty, core.NewError(core.CodeInvalidArgument, "meeting must be within the next seven days")
		}
		var reachable int
		if err := tx.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM agent_places p WHERE p.place_id=? AND p.instance_id=? AND p.branch_id=? AND p.status='active'
  AND (p.place_id=? OR EXISTS(SELECT 1 FROM rp_place_links l WHERE l.instance_id=p.instance_id AND l.branch_id=p.branch_id AND l.from_place_id=? AND l.to_place_id=p.place_id))`, r.MeetingPlaceID, session.InstanceID, session.BranchID, place, place).Scan(&reachable); err != nil {
			return empty, err
		}
		if reachable != 1 {
			return empty, core.NewError(core.CodeInvalidArgument, "meeting place is not known/reachable")
		}
		evidence.Description = "有人向另一人作出见面承诺。"
	case "keep_meeting":
		var raw string
		if err := tx.conn.QueryRowContext(ctx, `SELECT payload FROM events WHERE event_id=? AND instance_id=? AND branch_id=? AND event_type='RPInterpersonalAction' AND actor_id=?`, r.PromiseEventID, session.InstanceID, session.BranchID, session.ControlledEntityID).Scan(&raw); err != nil {
			return empty, classifyMissing(err, "own meeting promise")
		}
		var promise core.RPSocialEvidence
		if err := json.Unmarshal([]byte(raw), &promise); err != nil {
			return empty, err
		}
		at, parseErr := time.Parse(time.RFC3339, promise.MeetingWorldTime)
		now, _ := time.Parse(time.RFC3339, worldTime)
		if parseErr != nil || promise.Action != "promise_meeting" || promise.TargetEntityID != r.TargetEntityID || promise.MeetingPlaceID != place || now.Before(at) || now.After(at.Add(time.Hour)) {
			return empty, core.NewError(core.CodeInvalidArgument, "meeting fulfillment lacks actual place/time/presence evidence")
		}
		var fulfilled int
		if err := tx.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPInterpersonalAction' AND json_extract(payload,'$.promise_event_id')=?`, session.InstanceID, session.BranchID, r.PromiseEventID).Scan(&fulfilled); err != nil {
			return empty, err
		}
		if fulfilled != 0 {
			return empty, core.NewError(core.CodeBranchConflict, "meeting promise already fulfilled")
		}
		evidence.MeetingPlaceID = promise.MeetingPlaceID
		evidence.MeetingWorldTime = promise.MeetingWorldTime
		evidence.Description = "两人如约见面。"
	}
	var giverAccount, recipientAccount, currency string
	var giverBalance, recipientBalance int64
	if r.Action == "gift" {
		err = tx.conn.QueryRowContext(ctx, `SELECT a.asset_account_id,b.asset_account_id,a.currency_id,ab.balance_minor,bb.balance_minor
  FROM materialized_entities a JOIN materialized_entities b ON b.entity_id=? AND b.currency_id=a.currency_id
  JOIN account_balances ab ON ab.account_id=a.asset_account_id JOIN account_balances bb ON bb.account_id=b.asset_account_id
  WHERE a.entity_id=?`, r.TargetEntityID, session.ControlledEntityID).Scan(&giverAccount, &recipientAccount, &currency, &giverBalance, &recipientBalance)
		if err != nil {
			return empty, err
		}
		if giverAccount == recipientAccount || giverBalance < r.AmountMinor || recipientBalance > core.MaxJSONSafeInteger-r.AmountMinor {
			return empty, core.NewError(core.CodeInvalidArgument, "gift exceeds actual available funds or recipient capacity")
		}
	}
	sequence := head + 1
	var epoch string
	if err := tx.conn.QueryRowContext(ctx, `SELECT epoch_id FROM rule_epochs WHERE instance_id=? AND branch_id=? AND start_sequence<=? AND (end_sequence IS NULL OR ?<end_sequence)`, session.InstanceID, session.BranchID, sequence, sequence).Scan(&epoch); err != nil {
		return empty, err
	}
	hash, err := core.HashJSON(key)
	if err != nil {
		return empty, err
	}
	suffix := hash[7:]
	commandID, batchID, eventID, attemptID := "cmd_rp_social_"+suffix, "batch_rp_social_"+suffix, "event_rp_social_"+suffix, "attempt_rp_social_"+suffix
	encoded, err := core.CanonicalJSON(evidence)
	if err != nil {
		return empty, err
	}
	batchHash, err := core.HashJSON(struct {
		RequestHash string
		Sequence    int64
		WorldTime   string
		Evidence    core.RPSocialEvidence
	}{requestHash, sequence, worldTime, evidence})
	if err != nil {
		return empty, err
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	statements := []struct {
		name, query string
		args        []any
	}{
		{"social command", `INSERT INTO commands(command_id,instance_id,branch_id,command_type,idempotency_key,request_hash,expected_head,principal_id,command_policy,status,created_at_utc) VALUES (?,?,?,'RPSocial',?,?,?,?, '{"authorization":"rp-session-control"}','pending',?)`, []any{commandID, session.InstanceID, session.BranchID, key, requestHash, head, r.PrincipalID, now}},
		{"social attempt", `INSERT INTO command_attempts(command_id,attempt_no,attempt_id,status,lease_owner,lease_until_utc,proposal_hash,created_at_utc) VALUES (?,1,?,'ready','corerp-rp2',?,?,?)`, []any{commandID, attemptID, s.now().UTC().Add(30 * time.Second).Format(time.RFC3339Nano), requestHash, now}},
		{"social batch", `INSERT INTO event_batches(batch_id,command_id,attempt_no,instance_id,branch_id,epoch_id,expected_head,first_sequence,last_sequence,event_count,world_time,batch_hash,committed_at_utc) VALUES (?,?,1,?,?,?,?,?,?,1,?,?,?)`, []any{batchID, commandID, session.InstanceID, session.BranchID, epoch, head, sequence, sequence, worldTime, batchHash, now}},
		{"social event", `INSERT INTO events(event_id,batch_id,instance_id,branch_id,event_sequence,batch_index,event_type,actor_id,world_time,payload) VALUES (?,?,?,?,?,0,'RPInterpersonalAction',?,?,?)`, []any{eventID, batchID, session.InstanceID, session.BranchID, sequence, session.ControlledEntityID, worldTime, string(encoded)}},
	}
	for _, statement := range statements {
		if err := execAgentOne(ctx, tx.conn, statement.name, statement.query, statement.args...); err != nil {
			return empty, err
		}
	}
	if r.Action == "gift" {
		entryID := "journal_" + eventID
		if err := execAgentOne(ctx, tx.conn, "gift journal", `INSERT INTO journal_entries(entry_id,event_id,status,purpose) VALUES (?,?,'draft','RP interpersonal gift')`, entryID, eventID); err != nil {
			return empty, err
		}
		for i, posting := range []struct {
			account         string
			amount, balance int64
		}{{giverAccount, -r.AmountMinor, giverBalance - r.AmountMinor}, {recipientAccount, r.AmountMinor, recipientBalance + r.AmountMinor}} {
			if err := execAgentOne(ctx, tx.conn, "gift posting", `INSERT INTO postings(posting_id,entry_id,account_id,currency_id,amount_minor) VALUES (?,?,?,?,?)`, fmt.Sprintf("posting_%s_%d", eventID, i), entryID, posting.account, currency, posting.amount); err != nil {
				return empty, err
			}
			if err := execAgentOne(ctx, tx.conn, "gift balance", `UPDATE account_balances SET balance_minor=?,projection_version=projection_version+1,last_event_sequence=? WHERE account_id=?`, posting.balance, sequence, posting.account); err != nil {
				return empty, err
			}
		}
		if err := execAgentOne(ctx, tx.conn, "post gift journal", `UPDATE journal_entries SET status='posted' WHERE entry_id=?`, entryID); err != nil {
			return empty, err
		}
	}
	for _, pair := range [][2]string{{session.ControlledEntityID, r.TargetEntityID}, {r.TargetEntityID, session.ControlledEntityID}} {
		observationID := "observation_" + eventID + "_" + pair[0]
		claimKey := "social:" + eventID
		if err := execAgentOne(ctx, tx.conn, "social observation", `INSERT INTO observation_records(observation_id,source_event_id,observer_agent_id,subject_agent_id,place_id,channel,observed_world_time,claim_key,claim_payload) VALUES (?,?,?,?,?,'co_location',?,?,?)`, observationID, eventID, pair[0], pair[1], place, worldTime, claimKey, string(encoded)); err != nil {
			return empty, err
		}
		if err := execAgentOne(ctx, tx.conn, "social knowledge", `INSERT INTO agent_knowledge(observer_agent_id,claim_key,subject_agent_id,place_id,source_event_id,observation_id,learned_world_time,claim_payload,projection_version,last_event_sequence) VALUES (?,?,?,?,?,?,?,?,0,?)`, pair[0], claimKey, pair[1], place, eventID, observationID, worldTime, string(encoded), sequence); err != nil {
			return empty, err
		}
	}
	if err := execAgentOne(ctx, tx.conn, "social clock lineage", `UPDATE world_clocks SET projection_version=projection_version+1,last_event_sequence=? WHERE instance_id=? AND branch_id=? AND current_world_time=?`, sequence, session.InstanceID, session.BranchID, worldTime); err != nil {
		return empty, err
	}
	if err := execAgentOne(ctx, tx.conn, "social branch", `UPDATE branches SET head_sequence=? WHERE instance_id=? AND branch_id=? AND head_sequence=?`, sequence, session.InstanceID, session.BranchID, head); err != nil {
		return empty, err
	}
	if err := s.insertAgentAudit(ctx, tx.conn, "audit_"+commandID, "agent_decision", eventID, commandID, attemptID, worldTime, now, encoded); err != nil {
		return empty, err
	}
	if err := insertRPParticipantOutbox(ctx, tx.conn, "outbox_"+commandID, eventID, "rp.interpersonal", session.InstanceID, session.BranchID, session.ControlledEntityID, []string{r.TargetEntityID}, encoded); err != nil {
		return empty, err
	}
	if err := execAgentOne(ctx, tx.conn, "commit social command", `UPDATE commands SET status='committed' WHERE command_id=?`, commandID); err != nil {
		return empty, err
	}
	if err := execAgentOne(ctx, tx.conn, "commit social attempt", `UPDATE command_attempts SET status='committed',finished_at_utc=? WHERE attempt_id=? AND status='ready'`, now, attemptID); err != nil {
		return empty, err
	}
	if s.beforeCommit != nil {
		if err := s.beforeCommit(); err != nil {
			return empty, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return empty, err
	}
	return RPSocialResult{EventID: eventID, EventSequence: sequence, Description: evidence.Description}, nil
}
