package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"corerp.local/backend/internal/core"
)

type RPInitiativeResult struct {
	ReasonCode    string `json:"reason_code,omitempty"`
	EventID       string `json:"event_id,omitempty"`
	EventSequence int64  `json:"event_sequence"`
	NPCEntityID   string `json:"npc_entity_id"`
	Action        string `json:"action"`
	Status        string `json:"status"`
	Replayed      bool   `json:"replayed"`
}

// Keep accepted speech at the usual top-level fields. CanonicalJSON deliberately
// does not flatten embedded Go structs; authority payloads use explicit fields.
type rpInitiativeEvent struct {
	ReasonCode      string                  `json:"reason_code,omitempty"`
	SessionID       string                  `json:"session_id"`
	TurnID          string                  `json:"turn_id,omitempty"`
	UtteranceID     string                  `json:"utterance_id,omitempty"`
	SpeakerEntityID string                  `json:"speaker_entity_id,omitempty"`
	PlaceID         string                  `json:"place_id,omitempty"`
	Text            string                  `json:"text,omitempty"`
	SpeechAct       string                  `json:"speech_act,omitempty"`
	ListenerIDs     []string                `json:"listener_ids,omitempty"`
	NPCEntityID     string                  `json:"npc_entity_id"`
	TriggerEventID  string                  `json:"trigger_event_id"`
	InputHash       string                  `json:"input_hash"`
	Proposal        core.RPDecisionProposal `json:"proposal"`
	Action          string                  `json:"action"`
	Status          string                  `json:"status"`
	FromPlaceID     string                  `json:"from_place_id,omitempty"`
	ToPlaceID       string                  `json:"to_place_id,omitempty"`
}

func (e rpInitiativeEvent) speech() rpSpeechEvent {
	return rpSpeechEvent{SessionID: e.SessionID, TurnID: e.TurnID, UtteranceID: e.UtteranceID, SpeakerEntityID: e.SpeakerEntityID, PlaceID: e.PlaceID, Text: e.Text, SpeechAct: e.SpeechAct, ListenerIDs: e.ListenerIDs}
}

func initiativeKey(r core.RPInitiativeRequest) (string, error) {
	return core.HashJSON(r)
}

func readCommittedRPInitiative(ctx context.Context, conn *sql.Conn, r core.RPInitiativeRequest, key string) (RPInitiativeResult, bool, error) {
	var out RPInitiativeResult
	session, err := loadRPSession(ctx, conn, r.PrincipalID, r.SessionID)
	if err != nil {
		return out, false, err
	}
	if err := authorizeRPControl(ctx, conn, r.PrincipalID, session.InstanceID, session.BranchID, session.ControlledEntityID); err != nil {
		return out, false, err
	}
	var raw string
	err = conn.QueryRowContext(ctx, `SELECT e.event_id,e.event_sequence,e.payload FROM commands c JOIN event_batches b ON b.command_id=c.command_id JOIN events e ON e.batch_id=b.batch_id WHERE c.command_type='RPNPCInitiative' AND c.instance_id=? AND c.branch_id=? AND c.idempotency_key=? AND c.status='committed'`, session.InstanceID, session.BranchID, key).Scan(&out.EventID, &out.EventSequence, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return out, false, nil
	}
	if err != nil {
		return out, false, err
	}
	var event rpInitiativeEvent
	if err := json.Unmarshal([]byte(raw), &event); err != nil {
		return out, false, err
	}
	if event.SessionID != r.SessionID || event.NPCEntityID != r.NPCEntityID || event.TriggerEventID != r.TriggerEventID {
		return out, false, core.NewError(core.CodeProjectionDiverged, "initiative identity differs from committed request")
	}
	out.NPCEntityID, out.Action, out.Status, out.Replayed = event.NPCEntityID, event.Action, event.Status, true
	out.ReasonCode = event.ReasonCode
	return out, true, nil
}

// Initiative cadence is actor/world-scoped, not session-scoped. Opening more
// sessions cannot bypass the once-per-hour bound or the existing daily budget.
func rpInitiativeEligible(ctx context.Context, conn *sql.Conn, input core.RPDecisionInput) (bool, error) {
	at, err := time.Parse(time.RFC3339, input.WorldTime)
	if err != nil {
		return false, err
	}
	day := at.UTC().Truncate(24 * time.Hour).Format(time.RFC3339)
	hour := at.UTC().Add(-time.Hour).Format(time.RFC3339)
	var daily, recent, budget int
	start := day
	if hour < start {
		start = hour
	}
	err = conn.QueryRowContext(ctx, `SELECT COALESCE(SUM(CASE WHEN e.world_time>=? THEN 1 ELSE 0 END),0),COALESCE(SUM(CASE WHEN e.world_time>? THEN 1 ELSE 0 END),0) FROM events e JOIN event_batches b ON b.batch_id=e.batch_id JOIN commands c ON c.command_id=b.command_id WHERE c.command_type='RPNPCInitiative' AND e.instance_id=? AND e.branch_id=? AND e.actor_id=? AND e.world_time>=?`, day, hour, input.InstanceID, input.BranchID, input.NPCEntityID, start).Scan(&daily, &recent)
	if err != nil {
		return false, err
	}
	if err := conn.QueryRowContext(ctx, `SELECT action_budget_per_day FROM agent_profiles WHERE agent_id=?`, input.NPCEntityID).Scan(&budget); err != nil {
		return false, err
	}
	packages, err := readStudioActivePackages(ctx, conn, input.InstanceID, input.BranchID)
	if err != nil {
		return false, err
	}
	if packages != nil {
		budget = packages.System.Content.SystemRules.NPCDailyActionBudget
	}
	return recent == 0 && daily < budget, nil
}

// RunRPInitiative uses the same proposal/validation boundary as spoken turns.
// Its identity is the actual wait/NPC/session binding, not a synthetic utterance.
// Accepted speech/movement, hearing, audit and retry evidence commit together.
func (s *Store) RunRPInitiative(ctx context.Context, r core.RPInitiativeRequest, provider core.RPDecisionProvider) (RPInitiativeResult, error) {
	empty := RPInitiativeResult{}
	if err := r.Validate(); err != nil {
		return empty, err
	}
	key, err := initiativeKey(r)
	if err != nil {
		return empty, err
	}
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return empty, err
	}
	defer tx.Rollback(ctx)
	if out, found, err := readCommittedRPInitiative(ctx, tx.conn, r, key); err != nil || found {
		return out, err
	}
	input, err := readRPInitiativeInput(ctx, tx.conn, r)
	if err != nil {
		return empty, err
	}
	eligible, err := rpInitiativeEligible(ctx, tx.conn, input)
	if err != nil {
		return empty, err
	}
	if !eligible {
		return RPInitiativeResult{NPCEntityID: r.NPCEntityID, Action: "silence", Status: "cooldown", EventSequence: input.HeadSequence}, nil
	}
	tx.Rollback(ctx)
	if core.RPContactOpportunitySuppressed(input) {
		inputHash, err := core.HashJSON(input)
		if err != nil {
			return empty, err
		}
		return s.commitRPInitiative(ctx, r, key, inputHash, core.RPDecisionProposal{Action: "silence"}, "opportunity_quiet", "contact_opportunity_suppressed")
	}
	if provider == nil {
		return empty, core.NewError(core.CodeInvalidArgument, "initiative requires decision provider")
	}
	proposal, providerErr := provider.Propose(ctx, input)
	status := "validated"
	reason := ""
	if providerErr != nil {
		reason = "provider_failure"
	} else {
		reason, _ = core.ValidateRPDecisionProposalEvidence(input, proposal)
	}
	if reason != "" {
		proposal = core.RPDecisionProposal{Action: "silence"}
		status = "provider_fallback"
	}
	inputHash, err := core.HashJSON(input)
	if err != nil {
		return empty, err
	}
	return s.commitRPInitiative(ctx, r, key, inputHash, proposal, status, reason)
}

func (s *Store) commitRPInitiative(ctx context.Context, r core.RPInitiativeRequest, key, inputHash string, proposal core.RPDecisionProposal, status, reason string) (RPInitiativeResult, error) {
	empty := RPInitiativeResult{}
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return empty, err
	}
	defer tx.Rollback(ctx)
	if out, found, err := readCommittedRPInitiative(ctx, tx.conn, r, key); err != nil || found {
		return out, err
	}
	input, err := readRPInitiativeInput(ctx, tx.conn, r)
	if err != nil {
		return empty, err
	}
	freshHash, err := core.HashJSON(input)
	if err != nil {
		return empty, err
	}
	if freshHash != inputHash {
		return empty, core.NewError(core.CodeBranchConflict, "initiative context changed during decision")
	}
	if err := core.ValidateRPDecisionProposal(input, proposal); err != nil {
		return empty, err
	}
	eligible, err := rpInitiativeEligible(ctx, tx.conn, input)
	if err != nil {
		return empty, err
	}
	if !eligible {
		return empty, core.NewError(core.CodeBranchConflict, "initiative cadence changed")
	}
	if err := ensureCohortTransitionChronology(ctx, tx.conn, input.InstanceID, input.BranchID, input.WorldTime); err != nil {
		return empty, err
	}
	sequence := input.HeadSequence + 1
	var epoch string
	var day int64
	if err := tx.conn.QueryRowContext(ctx, `SELECT epoch_id FROM rule_epochs WHERE instance_id=? AND branch_id=? AND start_sequence<=? AND (end_sequence IS NULL OR ?<end_sequence)`, input.InstanceID, input.BranchID, sequence, sequence).Scan(&epoch); err != nil {
		return empty, err
	}
	if err := tx.conn.QueryRowContext(ctx, `SELECT current_day FROM world_clocks WHERE instance_id=? AND branch_id=?`, input.InstanceID, input.BranchID).Scan(&day); err != nil {
		return empty, err
	}
	suffix := key[7:]
	commandID, attemptID, batchID, eventID := "cmd_rp_initiative_"+suffix, "attempt_rp_initiative_"+suffix, "batch_rp_initiative_"+suffix, "event_rp_initiative_"+suffix
	eventType := "RPNPCDecisionRecorded"
	event := rpInitiativeEvent{ReasonCode: reason, SessionID: r.SessionID, NPCEntityID: r.NPCEntityID, TriggerEventID: r.TriggerEventID, InputHash: inputHash, Proposal: proposal, Action: proposal.Action, Status: status}
	if proposal.Action == "respond" {
		eventType = "RPSpeechAccepted"
		listeners, err := rpPerceivedEntityIDs(ctx, tx.conn, input.InstanceID, input.BranchID, input.PlaceID, r.NPCEntityID, "audio", "voice")
		if err != nil {
			return empty, err
		}
		event.TurnID, event.UtteranceID = "turn_rp_initiative_"+suffix, "utterance_rp_initiative_"+suffix
		event.SpeakerEntityID, event.PlaceID, event.Text, event.SpeechAct, event.ListenerIDs = r.NPCEntityID, input.PlaceID, proposal.Text, "statement", listeners
	} else if proposal.Action == "leave" {
		eventType = "RPNPCMoved"
		event.FromPlaceID = input.PlaceID
		event.ToPlaceID = proposal.DestinationPlaceID
	}
	payload, err := core.CanonicalJSON(event)
	if err != nil {
		return empty, err
	}
	batchHash, err := core.HashJSON(struct {
		CommandID string
		Sequence  int64
		WorldTime string
		Payload   rpInitiativeEvent
	}{commandID, sequence, input.WorldTime, event})
	if err != nil {
		return empty, err
	}
	proposalHash, err := core.HashJSON(proposal)
	if err != nil {
		return empty, err
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	for _, stmt := range []struct {
		name, query string
		args        []any
	}{
		{"initiative command", `INSERT INTO commands(command_id,instance_id,branch_id,command_type,idempotency_key,request_hash,expected_head,principal_id,command_policy,status,created_at_utc) VALUES (?,?,?,'RPNPCInitiative',?,?,?,?,'{"authorization":"rp-scoped-time-trigger"}','pending',?)`, []any{commandID, input.InstanceID, input.BranchID, key, key, input.HeadSequence, r.PrincipalID, now}},
		{"initiative attempt", `INSERT INTO command_attempts(command_id,attempt_no,attempt_id,status,lease_owner,lease_until_utc,proposal_hash,created_at_utc) VALUES (?,1,?,'ready','corerp-rp2',?,?,?)`, []any{commandID, attemptID, s.now().UTC().Add(30 * time.Second).Format(time.RFC3339Nano), proposalHash, now}},
		{"initiative batch", `INSERT INTO event_batches(batch_id,command_id,attempt_no,instance_id,branch_id,epoch_id,expected_head,first_sequence,last_sequence,event_count,world_time,batch_hash,committed_at_utc) VALUES (?,?,1,?,?,?,?,?,?,1,?,?,?)`, []any{batchID, commandID, input.InstanceID, input.BranchID, epoch, input.HeadSequence, sequence, sequence, input.WorldTime, batchHash, now}},
		{"initiative event", `INSERT INTO events(event_id,batch_id,instance_id,branch_id,event_sequence,batch_index,event_type,actor_id,world_time,payload) VALUES (?,?,?,?,?,0,?,?,?,?)`, []any{eventID, batchID, input.InstanceID, input.BranchID, sequence, eventType, r.NPCEntityID, input.WorldTime, string(payload)}},
	} {
		if err := execAgentOne(ctx, tx.conn, stmt.name, stmt.query, stmt.args...); err != nil {
			return empty, err
		}
	}
	if proposal.Action == "respond" {
		if err := execAgentOne(ctx, tx.conn, "initiative utterance", `INSERT INTO rp_utterances(utterance_id,event_id,session_id,turn_id,speaker_entity_id,place_id,world_time,speech_text,speech_act,listener_count) VALUES (?,?,?,?,?,?,?,?,'statement',?)`, event.UtteranceID, eventID, r.SessionID, event.TurnID, r.NPCEntityID, input.PlaceID, input.WorldTime, proposal.Text, len(event.ListenerIDs)); err != nil {
			return empty, err
		}
		if err := insertRPSpeechHearings(ctx, tx.conn, eventID, sequence, r.NPCEntityID, input.PlaceID, input.WorldTime, event.UtteranceID, proposal.Text, "statement", event.ListenerIDs); err != nil {
			return empty, err
		}
	} else if proposal.Action == "leave" {
		if err := commitRPNPCMovement(ctx, tx.conn, input.InstanceID, input.BranchID, eventID, sequence, r.NPCEntityID, input.PlaceID, proposal.DestinationPlaceID, input.WorldTime, day, "initiative_"+suffix); err != nil {
			return empty, err
		}
	}
	if err := s.insertAgentAudit(ctx, tx.conn, "audit_"+commandID, "agent_decision", eventID, commandID, attemptID, input.WorldTime, now, payload); err != nil {
		return empty, err
	}
	if proposal.Action == "respond" || proposal.Action == "leave" {
		audience := []string{input.InterlocutorEntityID}
		if proposal.Action == "respond" {
			audience = event.ListenerIDs
		}
		// Publish only the visible effect, never the private proposal/context hash.
		visible := any(rpNPCActionEvent{SessionID: r.SessionID, NPCEntityID: r.NPCEntityID, Action: proposal.Action, FromPlaceID: input.PlaceID, ToPlaceID: proposal.DestinationPlaceID})
		if proposal.Action == "respond" {
			visible = event.speech()
		}
		public, err := core.CanonicalJSON(visible)
		if err != nil {
			return empty, err
		}
		if err := insertRPParticipantOutbox(ctx, tx.conn, "outbox_"+commandID, eventID, "rp.npc.initiative", input.InstanceID, input.BranchID, r.NPCEntityID, audience, public); err != nil {
			return empty, err
		}
	}
	for _, stmt := range []struct {
		name, query string
		args        []any
	}{
		{"initiative clock", `UPDATE world_clocks SET projection_version=projection_version+1,last_event_sequence=? WHERE instance_id=? AND branch_id=? AND current_world_time=?`, []any{sequence, input.InstanceID, input.BranchID, input.WorldTime}},
		{"initiative branch", `UPDATE branches SET head_sequence=? WHERE instance_id=? AND branch_id=? AND head_sequence=?`, []any{sequence, input.InstanceID, input.BranchID, input.HeadSequence}},
		{"initiative commit attempt", `UPDATE command_attempts SET status='committed',finished_at_utc=? WHERE command_id=? AND attempt_no=1 AND status='ready'`, []any{now, commandID}},
		{"initiative commit command", `UPDATE commands SET status='committed' WHERE command_id=? AND status='pending'`, []any{commandID}},
	} {
		if err := execAgentOne(ctx, tx.conn, stmt.name, stmt.query, stmt.args...); err != nil {
			return empty, err
		}
	}
	if s.beforeCommit != nil {
		if err := s.beforeCommit(); err != nil {
			return empty, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return empty, err
	}
	return RPInitiativeResult{ReasonCode: reason, EventID: eventID, EventSequence: sequence, NPCEntityID: r.NPCEntityID, Action: proposal.Action, Status: status}, nil
}
