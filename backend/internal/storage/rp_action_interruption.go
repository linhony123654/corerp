package storage

import (
	"context"
	"corerp.local/backend/internal/core"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"
	"time"
)

// Public outcome deliberately excludes private fence, controller, and listener metadata.
type RPActionInterruption struct {
	Code string `json:"code"`
}

type rpActionInterruptionBoundary struct {
	eventID   string
	head      int64
	listeners []string
}

func readRPActionInterruptionBoundary(ctx context.Context, conn *sql.Conn, session RPSession, parent, runID string) (*rpActionInterruptionBoundary, error) {
	fact, sequence, _, err := readRPActionTurnSource(ctx, conn, session, parent)
	if err != nil {
		return nil, err
	}
	eligible, err := rpNonverbalEligibleWitnesses(fact)
	if err != nil {
		return nil, err
	}
	var roster string
	if err := conn.QueryRowContext(ctx, `SELECT listener_ids_json FROM rp_turn_runs WHERE turn_run_id=?`, runID).Scan(&roster); err != nil {
		return nil, err
	}
	var stored []string
	if json.Unmarshal([]byte(roster), &stored) != nil || !reflect.DeepEqual(stored, eligible) {
		return nil, narrativeDiverged("interruption listener roster differs from frozen source")
	}
	// Immutable ownership cannot disappear when a decision projection is lost.
	// Recognize each selected NPC's original deterministic command identity as
	// well as payload lineage before classifying an Event as unrelated.
	for _, npc := range eligible {
		key, err := core.HashJSON(struct{ SessionID, TurnID, NPCID string }{session.SessionID, parent, npc})
		if err != nil {
			return nil, err
		}
		var orphan int
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM commands c LEFT JOIN event_batches b ON b.command_id=c.command_id LEFT JOIN events n ON n.batch_id=b.batch_id AND n.batch_index=0 LEFT JOIN rp_npc_decisions d ON d.event_id=n.event_id AND d.session_id=? AND d.parent_turn_id=? AND d.npc_entity_id=? WHERE c.command_id=? AND c.instance_id=? AND c.branch_id=? AND c.command_type='RPNPCDecision' AND c.status='committed' AND d.event_id IS NULL`, session.SessionID, parent, npc, "cmd_rp_npc_"+key[7:], session.InstanceID, session.BranchID).Scan(&orphan); err != nil {
			return nil, err
		}
		if orphan != 0 {
			return nil, narrativeDiverged("committed action decision lacks its owner projection")
		}
	}
	var rawOrphan int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM events n JOIN event_batches b ON b.batch_id=n.batch_id JOIN commands c ON c.command_id=b.command_id LEFT JOIN rp_npc_decisions d ON d.event_id=n.event_id AND d.session_id=? AND d.parent_turn_id=? AND d.npc_entity_id=n.actor_id WHERE n.instance_id=? AND n.branch_id=? AND n.event_sequence>? AND n.batch_index=0 AND c.command_type='RPNPCDecision' AND json_extract(n.payload,'$.session_id')=? AND json_extract(n.payload,'$.parent_turn_id')=? AND d.event_id IS NULL`, session.SessionID, parent, session.InstanceID, session.BranchID, sequence, session.SessionID, parent).Scan(&rawOrphan); err != nil {
		return nil, err
	}
	if rawOrphan != 0 {
		return nil, narrativeDiverged("immutable action decision lineage lacks its projection")
	}
	var first sql.NullInt64
	err = conn.QueryRowContext(ctx, `SELECT MIN(e.event_sequence) FROM events e
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
	 AND r.trigger_kind='nonverbal' AND p.npc_entity_id=d.npc_entity_id AND p.disposition='activated'))`, session.InstanceID, session.BranchID, sequence, session.SessionID, parent).Scan(&first)
	if err != nil || !first.Valid {
		return nil, err
	}
	var planned int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_turn_listener_activations WHERE turn_run_id=?`, runID).Scan(&planned); err != nil {
		return nil, err
	}
	if planned != 0 && planned != len(eligible) {
		return nil, narrativeDiverged("interruption has incomplete activation accounting")
	}
	for _, id := range eligible {
		if planned == 0 {
			break
		}
		var found int
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_turn_listener_activations WHERE turn_run_id=? AND npc_entity_id=?`, runID, id).Scan(&found); err != nil {
			return nil, err
		}
		if found != 1 {
			return nil, narrativeDiverged("interruption activation differs from frozen source")
		}
	}
	var eventID string
	var currentHead int64
	if err := conn.QueryRowContext(ctx, `SELECT e.event_id,b.head_sequence FROM events e JOIN branches b ON b.instance_id=e.instance_id AND b.branch_id=e.branch_id WHERE e.instance_id=? AND e.branch_id=? AND e.event_sequence=?`, session.InstanceID, session.BranchID, first.Int64).Scan(&eventID, &currentHead); err != nil {
		return nil, err
	}
	source, err := loadRPNarrativeSource(ctx, conn, session.InstanceID, session.BranchID, eventID, currentHead)
	if err != nil {
		return nil, err
	}
	var batchFirst, count, after int64
	if err := conn.QueryRowContext(ctx, `SELECT first_sequence FROM event_batches WHERE batch_id=?`, source.Batch).Scan(&batchFirst); err != nil {
		return nil, err
	}
	if source.Index != 0 || batchFirst != first.Int64 {
		return nil, narrativeDiverged("interruption is not a complete batch boundary")
	}
	boundary := first.Int64 - 1
	if boundary < sequence {
		return nil, narrativeDiverged("interruption precedes accepted action")
	}
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE instance_id=? AND branch_id=? AND event_sequence>? AND event_sequence<=?`, session.InstanceID, session.BranchID, sequence, boundary).Scan(&count); err != nil {
		return nil, err
	}
	if count != boundary-sequence {
		return nil, narrativeDiverged("action continuation has a sequence gap")
	}
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_npc_decisions d JOIN events e ON e.event_id=d.event_id JOIN event_batches b ON b.batch_id=e.batch_id WHERE d.session_id=? AND d.parent_turn_id=? AND b.last_sequence>?`, session.SessionID, parent, boundary).Scan(&after); err != nil {
		return nil, err
	}
	if after != 0 {
		return nil, narrativeDiverged("committed action effects cross the interruption boundary")
	}
	rows, err := conn.QueryContext(ctx, `SELECT a.npc_entity_id FROM rp_turn_listener_activations a WHERE a.turn_run_id=? AND a.disposition='activated' AND NOT EXISTS(SELECT 1 FROM rp_npc_decisions d WHERE d.session_id=? AND d.parent_turn_id=? AND d.npc_entity_id=a.npc_entity_id) AND NOT EXISTS(SELECT 1 FROM rp_turn_listener_skips s WHERE s.turn_run_id=a.turn_run_id AND s.npc_entity_id=a.npc_entity_id) ORDER BY a.npc_entity_id`, runID, session.SessionID, parent)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	listeners := []string{}
	if planned == 0 {
		listeners = append(listeners, eligible...)
	}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		listeners = append(listeners, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return &rpActionInterruptionBoundary{eventID, boundary, listeners}, nil
}

// Classify and settle under one immediate transaction. Never permit a new
// reaction after the fence or manufacture a canonical decision to close a turn.
func (s *Store) settleRPActionInterruption(ctx context.Context, request core.RPNonverbalRequest, runID string) (RPTurnResult, bool, error) {
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return RPTurnResult{}, false, err
	}
	defer tx.Rollback(ctx)
	session, err := loadRPSession(ctx, tx.conn, request.PrincipalID, request.SessionID)
	if err != nil {
		return RPTurnResult{}, false, err
	}
	if err := authorizeRPControl(ctx, tx.conn, request.PrincipalID, session.InstanceID, session.BranchID, session.ControlledEntityID); err != nil {
		return RPTurnResult{}, false, err
	}
	var status, parent, hash, trigger string
	if err := tx.conn.QueryRowContext(ctx, `SELECT status,COALESCE(player_event_id,''),request_hash,trigger_kind FROM rp_turn_runs WHERE turn_run_id=? AND session_id=?`, runID, session.SessionID).Scan(&status, &parent, &hash, &trigger); err != nil {
		return RPTurnResult{}, false, err
	}
	expected, err := core.HashJSON(request)
	if err != nil || hash != expected || trigger != "nonverbal" {
		return RPTurnResult{}, false, narrativeDiverged("interrupted action request differs from owner")
	}
	if status == "settled" || status == "narrative_ready" {
		return RPTurnResult{}, false, nil
	}
	// Locate only the exact original raw owner's committed receipt. Hashing is
	// performed before public target resolution by the existing typed owner.
	var commandID, commandHash, commandStatus string
	err = tx.conn.QueryRowContext(ctx, `SELECT command_id,request_hash,status FROM commands WHERE instance_id=? AND branch_id=? AND command_type='RPNonverbalAction' AND idempotency_key=?`, session.InstanceID, session.BranchID, "rp_nonverbal:"+session.SessionID+":"+request.IdempotencyKey).Scan(&commandID, &commandHash, &commandStatus)
	if errors.Is(err, sql.ErrNoRows) && parent == "" {
		return RPTurnResult{}, false, nil
	}
	if err != nil {
		return RPTurnResult{}, false, narrativeDiverged("interrupted action lacks its exact owner command")
	}
	if commandHash != expected || commandStatus != "committed" {
		return RPTurnResult{}, false, narrativeDiverged("interrupted action command differs from original request")
	}
	receipt, err := loadRPNonverbalResult(ctx, tx.conn, commandID, true)
	if err != nil {
		return RPTurnResult{}, false, err
	}
	if parent != "" && parent != receipt.EventID {
		return RPTurnResult{}, false, narrativeDiverged("interrupted action parent differs from original command receipt")
	}
	parent = receipt.EventID
	fact, _, _, err := readRPActionTurnSource(ctx, tx.conn, session, parent)
	if err != nil {
		return RPTurnResult{}, false, err
	}
	if fact.Action != request.Action || fact.GestureCode != request.GestureCode || (fact.TargetEntityID == "") != (request.TargetEntityID == "") {
		return RPTurnResult{}, false, narrativeDiverged("interrupted action typed source differs from request")
	}
	if status == "open" {
		if session.Status != "active" {
			return RPTurnResult{}, false, narrativeDiverged("raw interrupted action session is inactive")
		}
		eligible, err := rpNonverbalEligibleWitnesses(fact)
		if err != nil {
			return RPTurnResult{}, false, err
		}
		roster, err := core.CanonicalJSON(eligible)
		if err != nil {
			return RPTurnResult{}, false, err
		}
		// Bind the already accepted effect only within this recovery snapshot.
		// A no-fence return rolls back this binding and leaves ordinary stages intact.
		if err := execAgentOne(ctx, tx.conn, "bind interrupted raw action", `UPDATE rp_turn_runs SET status='player_committed',player_event_id=?,listener_ids_json=? WHERE turn_run_id=? AND status='open' AND player_event_id IS NULL`, parent, string(roster), runID); err != nil {
			return RPTurnResult{}, false, err
		}
		if err := execAgentOne(ctx, tx.conn, "bind interrupted raw action session", `UPDATE rp_sessions SET turn_cursor=?,turn_state='action_committed' WHERE session_id=? AND status='active'`, parent, session.SessionID); err != nil {
			return RPTurnResult{}, false, err
		}
		status = "player_committed"
		session.TurnCursor = parent
	}
	boundary, err := readRPActionInterruptionBoundary(ctx, tx.conn, session, parent, runID)
	if err != nil {
		return RPTurnResult{}, false, err
	}
	if boundary == nil {
		return RPTurnResult{}, false, nil
	}
	if session.Status != "active" || session.TurnCursor != parent {
		return RPTurnResult{}, true, narrativeDiverged("interrupted action is not current session turn")
	}
	if err := validateRPInterruptedPublicEvidence(ctx, tx.conn, session, parent, boundary.head); err != nil {
		return RPTurnResult{}, true, err
	}
	input, err := readRPNarrativeInputAtHead(ctx, tx.conn, session.SessionID, parent, parent, boundary.head)
	if err != nil {
		return RPTurnResult{}, true, err
	}
	var profile string
	if err := tx.conn.QueryRowContext(ctx, `SELECT profile_json FROM rp_turn_styles WHERE turn_run_id=?`, runID).Scan(&profile); err != nil {
		return RPTurnResult{}, true, err
	}
	if json.Unmarshal([]byte(profile), &input.Style) != nil {
		return RPTurnResult{}, true, narrativeDiverged("interrupted turn style is invalid")
	}
	view, err := (core.DeterministicRPNarrativeProvider{}).Render(ctx, input)
	if err != nil {
		return RPTurnResult{}, true, err
	}
	artifact, err := encodeRPNarrativeArtifact(view)
	if err != nil {
		return RPTurnResult{}, true, err
	}
	if _, err := validateRPNarrativeArtifactOnConn(ctx, tx.conn, session.InstanceID, session.BranchID, session.ControlledEntityID, runID, true, artifact, view); err != nil {
		return RPTurnResult{}, true, err
	}
	lines, err := core.CanonicalJSON(view.Lines)
	if err != nil {
		return RPTurnResult{}, true, err
	}
	groups, err := core.CanonicalJSON(view.FactGroups)
	if err != nil {
		return RPTurnResult{}, true, err
	}
	ids, err := core.CanonicalJSON(view.EventIDs)
	if err != nil {
		return RPTurnResult{}, true, err
	}
	listeners, err := core.CanonicalJSON(boundary.listeners)
	if err != nil {
		return RPTurnResult{}, true, err
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	// The immediate transaction pins the observed actual head while the public
	// receipt retains its earlier complete historical source head.
	if err := execAgentOne(ctx, tx.conn, "settle interrupted action", `UPDATE rp_turn_runs SET status='settled',interruption_event_id=?,interrupted_listener_ids_json=?,narrative_json=?,narrative_composition_version=?,narrative_fact_groups_json=?,narrative_fact_event_ids_json=?,narrative_artifact_json=?,settled_sequence=?,settled_at_utc=?,updated_at_utc=? WHERE turn_run_id=? AND status=? AND player_event_id=?`, boundary.eventID, string(listeners), string(lines), view.CompositionVersion, string(groups), string(ids), artifact, boundary.head, now, now, runID, status, parent); err != nil {
		return RPTurnResult{}, true, err
	}
	if err := execAgentOne(ctx, tx.conn, "settle interrupted action session", `UPDATE rp_sessions SET turn_state='settled',observation_cursor=? WHERE session_id=? AND turn_cursor=? AND status='active'`, boundary.head, session.SessionID, parent); err != nil {
		return RPTurnResult{}, true, err
	}
	if s.beforeCommit != nil {
		if err := s.beforeCommit(); err != nil {
			return RPTurnResult{}, true, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return RPTurnResult{}, true, err
	}
	run, err := s.loadRPTurnRun(ctx, runID)
	if err != nil {
		return RPTurnResult{}, true, err
	}
	result, err := s.loadRPTurnResult(ctx, run, false)
	return result, true, err
}

func (s *Store) readRPActionInterruption(ctx context.Context, run rpTurnRun) (*RPActionInterruption, error) {
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var fence sql.NullString
	var listeners string
	if err := tx.conn.QueryRowContext(ctx, `SELECT interruption_event_id,interrupted_listener_ids_json FROM rp_turn_runs WHERE turn_run_id=?`, run.ID).Scan(&fence, &listeners); err != nil {
		return nil, err
	}
	if !fence.Valid {
		if listeners != "[]" {
			return nil, narrativeDiverged("ordinary turn contains interrupted listeners")
		}
		return nil, nil
	}
	var principal string
	if err := tx.conn.QueryRowContext(ctx, `SELECT principal_id FROM rp_sessions WHERE session_id=?`, run.SessionID).Scan(&principal); err != nil {
		return nil, err
	}
	session, err := loadRPSessionRecord(ctx, tx.conn, principal, run.SessionID)
	if err != nil {
		return nil, err
	}
	var requestJSON, requestHash, key, trigger string
	if err := tx.conn.QueryRowContext(ctx, `SELECT request_json,request_hash,idempotency_key,trigger_kind FROM rp_turn_runs WHERE turn_run_id=?`, run.ID).Scan(&requestJSON, &requestHash, &key, &trigger); err != nil {
		return nil, err
	}
	var request core.RPNonverbalRequest
	if json.Unmarshal([]byte(requestJSON), &request) != nil || request.Validate() != nil || request.PrincipalID != principal || request.SessionID != run.SessionID || request.IdempotencyKey != key || trigger != "nonverbal" {
		return nil, narrativeDiverged("saved interruption request lacks original authority")
	}
	expected, err := core.HashJSON(request)
	if err != nil || expected != requestHash {
		return nil, narrativeDiverged("saved interruption request hash differs")
	}
	var commandID, commandHash string
	if err := tx.conn.QueryRowContext(ctx, `SELECT command_id,request_hash FROM commands WHERE instance_id=? AND branch_id=? AND command_type='RPNonverbalAction' AND idempotency_key=? AND status='committed'`, session.InstanceID, session.BranchID, "rp_nonverbal:"+run.SessionID+":"+key).Scan(&commandID, &commandHash); err != nil || commandHash != expected {
		return nil, narrativeDiverged("saved interruption differs from exact owner command")
	}
	receipt, err := loadRPNonverbalResult(ctx, tx.conn, commandID, true)
	if err != nil || receipt.EventID != run.PlayerEventID {
		return nil, narrativeDiverged("saved interruption parent differs from original receipt")
	}
	boundary, err := readRPActionInterruptionBoundary(ctx, tx.conn, session, run.PlayerEventID, run.ID)
	if err != nil {
		return nil, err
	}
	var saved []string
	if boundary == nil || boundary.eventID != fence.String || boundary.head != run.SettledSequence.Int64 || json.Unmarshal([]byte(listeners), &saved) != nil || !reflect.DeepEqual(saved, boundary.listeners) {
		return nil, narrativeDiverged("saved interruption differs from immutable boundary or listener accounting")
	}
	return &RPActionInterruption{Code: "world_changed"}, nil
}

// Check immutable expected audiences before the projection builder can filter
// an absent observation. Missing hearing/witness evidence is corruption, not
// proof that an already committed public effect was unperceived.
func validateRPInterruptedPublicEvidence(ctx context.Context, conn *sql.Conn, session RPSession, parent string, head int64) error {
	rows, err := conn.QueryContext(ctx, `SELECT e.event_id,e.event_type,e.actor_id,e.world_time,e.payload FROM rp_npc_decisions d JOIN events n ON n.event_id=d.event_id JOIN events e ON e.batch_id=n.batch_id WHERE d.session_id=? AND d.parent_turn_id=? AND e.event_sequence<=? ORDER BY e.event_sequence`, session.SessionID, parent, head)
	if err != nil {
		return err
	}
	type publicSource struct{ id, kind, actor, at, raw string }
	sources := []publicSource{}
	for rows.Next() {
		var source publicSource
		if err := rows.Scan(&source.id, &source.kind, &source.actor, &source.at, &source.raw); err != nil {
			rows.Close()
			return err
		}
		sources = append(sources, source)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, source := range sources {
		if _, err := loadRPNarrativeSource(ctx, conn, session.InstanceID, session.BranchID, source.id, head); err != nil {
			return err
		}
		switch source.kind {
		case "RPSpeechAccepted":
			var speech rpSpeechEvent
			if json.Unmarshal([]byte(source.raw), &speech) != nil || speech.SpeakerEntityID != source.actor || speech.SessionID != session.SessionID || speech.ParentTurnID != parent {
				return narrativeDiverged("interrupted speech lacks immutable owner lineage")
			}
			heard := false
			for _, id := range speech.ListenerIDs {
				heard = heard || id == session.ControlledEntityID
			}
			if !heard {
				continue
			}
			var raw string
			if err := conn.QueryRowContext(ctx, `SELECT claim_payload FROM observation_records WHERE source_event_id=? AND observer_agent_id=? AND subject_agent_id=? AND claim_key='speech:'||? AND place_id=? AND observed_world_time=?`, source.id, session.ControlledEntityID, source.actor, source.id, speech.PlaceID, source.at).Scan(&raw); err != nil {
				return narrativeDiverged("committed interrupted speech lost frozen hearing")
			}
			if _, err := recordedRPSpeechTone(source.raw, raw, source.actor, session.ControlledEntityID); err != nil {
				return err
			}
		case "RPNonverbalAction":
			var fact core.RPNonverbalFact
			if json.Unmarshal([]byte(source.raw), &fact) != nil || fact.ActorEntityID != source.actor || fact.SessionID != session.SessionID {
				return narrativeDiverged("interrupted expression lacks immutable owner lineage")
			}
			witnessed := false
			for _, w := range fact.Witnesses {
				witnessed = witnessed || w.ObserverEntityID == session.ControlledEntityID
			}
			if !witnessed {
				continue
			}
			var raw string
			if err := conn.QueryRowContext(ctx, `SELECT claim_payload FROM observation_records WHERE source_event_id=? AND observer_agent_id=? AND subject_agent_id=? AND claim_key='nonverbal:'||? AND place_id=? AND observed_world_time=?`, source.id, session.ControlledEntityID, source.actor, source.id, fact.PlaceID, source.at).Scan(&raw); err != nil {
				return narrativeDiverged("committed interrupted expression lost frozen witness")
			}
			var observed map[string]any
			if json.Unmarshal([]byte(raw), &observed) != nil || !rpDecisionNonverbalWitnessMatchesSource(source.raw, observed, session.ControlledEntityID, source.actor, fact.PlaceID) {
				return narrativeDiverged("interrupted expression differs from frozen witness")
			}
		}
	}
	return nil
}
