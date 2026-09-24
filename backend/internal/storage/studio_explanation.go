package storage

import (
	"context"
	"database/sql"

	"corerp.local/backend/internal/core"
)

type StudioExplanationRequest struct {
	PrincipalID        string `json:"principal_id"`
	InstanceID         string `json:"instance_id"`
	BranchID           string `json:"branch_id"`
	EventID            string `json:"event_id"`
	ObserverID         string `json:"observer_id,omitempty"`
	AfterObserverID    string `json:"after_observer_id,omitempty"`
	ThroughRecordOrder int64  `json:"through_record_order,omitempty"`
	AfterRecordOrder   int64  `json:"after_record_order,omitempty"`
	Limit              int    `json:"limit,omitempty"`
}
type StudioDiagnostic struct {
	RecordID    string `json:"record_id"`
	RecordOrder int64  `json:"record_order"`
	Authority   string `json:"authority"`
	Status      string `json:"status"`
	ReasonCode  string `json:"reason_code"`
	Action      string `json:"action,omitempty"`
	NPCID       string `json:"npc_entity_id,omitempty"`
}
type StudioObserver struct {
	EntityID          string `json:"entity_id"`
	ObservationCount  int64  `json:"observation_count"`
	FirstObservedTime string `json:"first_observed_time"`
}

// The decision and utterance rows are immutable source records. This relation
// proves a committed response to a recorded trigger, not a subjective motive.
type StudioCommittedDecision struct {
	SourceKind      string `json:"source_kind"`
	Status          string `json:"status,omitempty"`
	ReasonCode      string `json:"reason_code,omitempty"`
	DecisionID      string `json:"decision_id"`
	TriggerEventID  string `json:"trigger_event_id"`
	TriggerSequence int64  `json:"trigger_sequence"`
	NPCID           string `json:"npc_entity_id"`
	Action          string `json:"action"`
	Outcome         string `json:"outcome"`
}
type StudioExplanation struct {
	CommittedEvidence   string                   `json:"committed_evidence"`
	CommittedDecision   *StudioCommittedDecision `json:"committed_decision,omitempty"`
	DiagnosticEvidence  string                   `json:"diagnostic_evidence"`
	InstanceID          string                   `json:"instance_id"`
	BranchID            string                   `json:"branch_id"`
	EventID             string                   `json:"event_id"`
	AccessLevel         string                   `json:"access_level"`
	HeadSequence        int64                    `json:"head_sequence"`
	ThroughRecordOrder  int64                    `json:"through_record_order"`
	NextRecordOrder     int64                    `json:"next_record_order,omitempty"`
	Diagnostics         []StudioDiagnostic       `json:"diagnostics"`
	Observers           []StudioObserver         `json:"observers,omitempty"`
	NextObserverID      string                   `json:"next_observer_id,omitempty"`
	ObservationEvidence string                   `json:"observation_evidence"`
}

func (s *Store) ReadStudioExplanation(ctx context.Context, r StudioExplanationRequest) (StudioExplanation, error) {
	var out StudioExplanation
	limit, err := studioLimit(r.Limit)
	if err != nil {
		return out, err
	}
	if !studioID(r.PrincipalID) || !studioID(r.InstanceID) || !studioID(r.BranchID) || !studioID(r.EventID) || (r.ObserverID != "" && !studioID(r.ObserverID)) || (r.AfterObserverID != "" && !studioID(r.AfterObserverID)) || r.AfterRecordOrder < 0 || r.ThroughRecordOrder < 0 || r.AfterRecordOrder >= core.MaxJSONSafeInteger || r.ThroughRecordOrder >= core.MaxJSONSafeInteger {
		return out, core.NewError(core.CodeInvalidArgument, "invalid explanation scope or page")
	}
	if r.ObserverID != "" && r.AfterObserverID != "" {
		return out, core.NewError(core.CodeInvalidArgument, "specific observer lookup cannot carry an observer page cursor")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	role, err := authorizeStudio(ctx, tx, r.PrincipalID, r.InstanceID, r.BranchID)
	if err != nil {
		return out, err
	}
	var permitted int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM capability_grants g JOIN principals p ON p.principal_id=g.principal_id WHERE p.principal_id=? AND g.instance_id=? AND g.branch_id=? AND g.subject_id IN (?,'*') AND `+studioGrantPredicate+` AND EXISTS(SELECT 1 FROM json_each(g.field_scope) WHERE value='explain')`, r.PrincipalID, r.InstanceID, r.BranchID, r.BranchID).Scan(&permitted); err != nil {
		return out, err
	}
	if permitted == 0 {
		return out, core.NewError(core.CodeUnauthorized, "explicit explanation permission required")
	}
	if role == "operator" && (r.ObserverID != "" || r.AfterObserverID != "") {
		return out, core.NewError(core.CodeUnauthorized, "observer details are outside diagnostic scope")
	}
	out = StudioExplanation{DiagnosticEvidence: "npc_candidate_validation_only", InstanceID: r.InstanceID, BranchID: r.BranchID, EventID: r.EventID, AccessLevel: role, Diagnostics: []StudioDiagnostic{}, ObservationEvidence: "redacted"}
	if err := tx.QueryRowContext(ctx, `SELECT b.head_sequence FROM branches b JOIN events e ON e.instance_id=b.instance_id AND e.branch_id=b.branch_id WHERE e.instance_id=? AND e.branch_id=? AND e.event_id=?`, r.InstanceID, r.BranchID, r.EventID).Scan(&out.HeadSequence); err != nil {
		return StudioExplanation{}, classifyMissing(err, "explanation event")
	}
	var cutoff int64
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(record_order),0) FROM audit_records WHERE instance_id=? AND branch_id=?`, r.InstanceID, r.BranchID).Scan(&cutoff); err != nil {
		return StudioExplanation{}, err
	}
	if r.ThroughRecordOrder == 0 {
		r.ThroughRecordOrder = cutoff
	}
	if r.ThroughRecordOrder > cutoff || r.AfterRecordOrder > r.ThroughRecordOrder {
		return StudioExplanation{}, core.NewError(core.CodeInvalidArgument, "explanation cursor exceeds recorded audit cutoff")
	}
	out.ThroughRecordOrder = r.ThroughRecordOrder
	// Only the bounded record family actually written by auditRPDecision is admitted.
	// No raw proposals, provider error bodies, private contexts or opaque audits.
	rows, err := tx.QueryContext(ctx, `SELECT record_id,record_order,authority,COALESCE(json_extract(payload,'$.status'),''),COALESCE(json_extract(payload,'$.reason_code'),''),COALESCE(json_extract(payload,'$.proposal.action'),''),COALESCE(json_extract(payload,'$.npc_entity_id'),'')
 FROM audit_records WHERE instance_id=? AND branch_id=? AND related_event_id=? AND record_order>? AND record_order<=? AND authority='non-authoritative' AND json_extract(audience_scope,'$.kind')='rp_internal' AND instr(record_id,'audit_rp_decision_')=1 ORDER BY record_order LIMIT ?`, r.InstanceID, r.BranchID, r.EventID, r.AfterRecordOrder, r.ThroughRecordOrder, limit+1)
	if err != nil {
		return StudioExplanation{}, err
	}
	for rows.Next() {
		var d StudioDiagnostic
		if err := rows.Scan(&d.RecordID, &d.RecordOrder, &d.Authority, &d.Status, &d.ReasonCode, &d.Action, &d.NPCID); err != nil {
			rows.Close()
			return StudioExplanation{}, err
		}
		if len(out.Diagnostics) == limit {
			out.NextRecordOrder = out.Diagnostics[len(out.Diagnostics)-1].RecordOrder
			break
		}
		switch d.Status {
		case "validated", "rejected", "provider_fallback":
		default:
			d.Status = "unrecognized"
		}
		switch d.ReasonCode {
		case "action_not_legal", "invalid_speech_fields", "movement_contains_speech", "destination_not_reachable", "noop_contains_effects", "unknown_action", "provider_failure":
		default:
			d.ReasonCode = "not_recorded"
		}
		switch d.Action {
		case "respond", "refuse", "leave", "silence", "wait":
		default:
			d.Action = "unrecognized"
		}
		if role == "operator" {
			d.NPCID = ""
			d.Action = ""
		}
		out.Diagnostics = append(out.Diagnostics, d)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return StudioExplanation{}, err
	}
	rows.Close()
	out.CommittedEvidence = "redacted"
	if role == "operator" {
		return out, nil
	}
	out.CommittedEvidence = "not_recorded"
	var committed StudioCommittedDecision
	err = tx.QueryRowContext(ctx, `SELECT d.decision_id,u.event_id,p.event_sequence,d.npc_entity_id,d.action
 FROM rp_npc_decisions d JOIN rp_utterances u ON u.turn_id=d.parent_turn_id AND u.session_id=d.session_id
 JOIN events p ON p.event_id=u.event_id JOIN events e ON e.event_id=d.event_id
 WHERE d.event_id=? AND e.instance_id=? AND e.branch_id=? AND p.instance_id=e.instance_id
 AND p.branch_id=e.branch_id AND p.event_sequence<e.event_sequence AND e.event_sequence<=?`,
		r.EventID, r.InstanceID, r.BranchID, out.HeadSequence).Scan(&committed.DecisionID, &committed.TriggerEventID, &committed.TriggerSequence, &committed.NPCID, &committed.Action)
	if err != nil && err != sql.ErrNoRows {
		return StudioExplanation{}, err
	}
	committed.SourceKind = "npc_response"
	if err == sql.ErrNoRows {
		err = tx.QueryRowContext(ctx, `SELECT c.command_id,p.event_id,p.event_sequence,e.actor_id,
 json_extract(e.payload,'$.action'),json_extract(e.payload,'$.status'),COALESCE(json_extract(e.payload,'$.reason_code'),'')
 FROM events e JOIN event_batches b ON b.batch_id=e.batch_id
 JOIN commands c ON c.command_id=b.command_id
 JOIN events p ON p.event_id=json_extract(e.payload,'$.trigger_event_id')
 WHERE e.event_id=? AND e.instance_id=? AND e.branch_id=? AND c.command_type='RPNPCInitiative'
 AND c.status='committed' AND c.instance_id=e.instance_id AND c.branch_id=e.branch_id
 AND p.instance_id=e.instance_id AND p.branch_id=e.branch_id AND p.event_sequence<e.event_sequence
 AND e.event_sequence<=?`, r.EventID, r.InstanceID, r.BranchID, out.HeadSequence).
			Scan(&committed.DecisionID, &committed.TriggerEventID, &committed.TriggerSequence, &committed.NPCID, &committed.Action, &committed.Status, &committed.ReasonCode)
		if err != nil && err != sql.ErrNoRows {
			return StudioExplanation{}, err
		}
		committed.SourceKind = "npc_initiative_command"
		switch committed.Status {
		case "validated", "provider_fallback", "opportunity_quiet":
		default:
			committed.Status = "unrecognized"
		}
		switch committed.ReasonCode {
		case "action_not_legal", "invalid_speech_fields", "movement_contains_speech", "destination_not_reachable", "noop_contains_effects", "unknown_action", "provider_failure", "contact_opportunity_suppressed":
		default:
			committed.ReasonCode = "not_recorded"
		}
	}
	if err == nil {
		switch committed.Action {
		case "respond", "refuse":
			committed.Outcome = "committed_speech"
		case "leave":
			committed.Outcome = "committed_movement"
		case "silence":
			committed.Outcome = "committed_silence"
		case "wait":
			committed.Outcome = "committed_wait"
		default:
			return StudioExplanation{}, core.NewError(core.CodeProjectionDiverged, "unknown committed NPC action")
		}
		out.CommittedEvidence, out.CommittedDecision = "recorded_trigger_and_outcome", &committed
	}
	out.ObservationEvidence = "recorded_observers_only"
	if r.ObserverID != "" {
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM agent_profiles WHERE instance_id=? AND branch_id=? AND agent_id=?`, r.InstanceID, r.BranchID, r.ObserverID).Scan(&n); err != nil {
			return StudioExplanation{}, err
		}
		if n != 1 {
			return StudioExplanation{}, core.NewError(core.CodeNotFound, "observer is not in this world")
		}
		out.ObservationEvidence = "not_recorded"
	}
	rows, err = tx.QueryContext(ctx, `SELECT o.observer_agent_id,COUNT(*),MIN(o.observed_world_time) FROM observation_records o JOIN agent_profiles a ON a.agent_id=o.observer_agent_id WHERE o.source_event_id=? AND a.instance_id=? AND a.branch_id=? AND (?='' OR o.observer_agent_id=?) AND o.observer_agent_id>? GROUP BY o.observer_agent_id ORDER BY o.observer_agent_id LIMIT ?`, r.EventID, r.InstanceID, r.BranchID, r.ObserverID, r.ObserverID, r.AfterObserverID, limit+1)
	if err != nil {
		return StudioExplanation{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var o StudioObserver
		if err := rows.Scan(&o.EntityID, &o.ObservationCount, &o.FirstObservedTime); err != nil {
			return StudioExplanation{}, err
		}
		if len(out.Observers) == limit {
			out.NextObserverID = out.Observers[len(out.Observers)-1].EntityID
			break
		}
		out.Observers = append(out.Observers, o)
		if r.ObserverID != "" {
			out.ObservationEvidence = "recorded_observation"
		}
	}
	return out, rows.Err()
}
