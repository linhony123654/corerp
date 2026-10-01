package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"sort"
	"time"

	"corerp.local/backend/internal/core"
)

func rpNonverbalEligibleWitnesses(fact core.RPNonverbalFact) ([]string, error) {
	eligible := make([]string, 0, len(fact.Witnesses))
	seen := map[string]bool{}
	for _, w := range fact.Witnesses {
		if w.ObserverEntityID == "" || w.ObserverEntityID == fact.ActorEntityID || seen[w.ObserverEntityID] {
			return nil, core.NewError(core.CodeProjectionDiverged, "invalid frozen nonverbal witness set")
		}
		seen[w.ObserverEntityID] = true
		if fact.TargetEntityID == "" || w.ObserverEntityID == fact.TargetEntityID && w.TargetVisible {
			eligible = append(eligible, w.ObserverEntityID)
		}
	}
	sort.Strings(eligible)
	return eligible, nil
}

// Directed actions select only their witnessed target. Untargeted actions
// use the existing bounded mode/focus/stable policy over actual witnesses.
func (s *Store) ensureRPNonverbalActivationPlan(ctx context.Context, runID, sessionID string) ([]rpTurnListenerActivation, error) {
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var status, raw, parent string
	session := RPSession{SessionID: sessionID}
	if err := tx.conn.QueryRowContext(ctx, `SELECT r.status,r.listener_ids_json,r.player_event_id,h.instance_id,h.branch_id,h.controlled_entity_id
 FROM rp_turn_runs r JOIN rp_sessions h ON h.session_id=r.session_id
 WHERE r.turn_run_id=? AND r.session_id=? AND r.trigger_kind='nonverbal' AND r.player_event_id IS NOT NULL AND r.player_turn_id IS NULL`, runID, sessionID).Scan(&status, &raw, &parent, &session.InstanceID, &session.BranchID, &session.ControlledEntityID); err != nil {
		return nil, classifyMissing(err, "committed nonverbal turn activation")
	}
	fact, cutoff, at, err := readRPActionTurnSource(ctx, tx.conn, session, parent)
	if err != nil {
		return nil, err
	}
	eligible, err := rpNonverbalEligibleWitnesses(fact)
	if err != nil {
		return nil, err
	}
	expected, err := core.CanonicalJSON(eligible)
	if err != nil {
		return nil, err
	}
	var supplied []string
	if json.Unmarshal([]byte(raw), &supplied) != nil {
		return nil, core.NewError(core.CodeProjectionDiverged, "invalid nonverbal listener list")
	}
	encoded, err := core.CanonicalJSON(supplied)
	if err != nil {
		return nil, err
	}
	if string(encoded) != string(expected) {
		return nil, core.NewError(core.CodeProjectionDiverged, "nonverbal listeners differ from frozen witness eligibility")
	}
	var existing, matching int
	if err := tx.conn.QueryRowContext(ctx, `SELECT COUNT(*),COALESCE(SUM(npc_entity_id IN (SELECT value FROM json_each(?))),0) FROM rp_turn_listener_activations WHERE turn_run_id=?`, raw, runID).Scan(&existing, &matching); err != nil {
		return nil, err
	}
	if existing != 0 || status != "npc_deciding" {
		if existing != len(eligible) || matching != existing {
			return nil, core.NewError(core.CodeProjectionDiverged, "nonverbal activation accounting is incomplete")
		}
		tx.Rollback(ctx)
		return s.loadRPTurnActivations(ctx, runID)
	}
	if err := validateRPActionTurnContinuation(ctx, tx.conn, session, parent); err != nil {
		return nil, err
	}
	mode, limit, err := readRPTurnExecutionMode(ctx, tx.conn, session.InstanceID, session.BranchID, len(eligible))
	if err != nil {
		return nil, err
	}
	directed := fact.TargetEntityID != ""
	focusAvailable, err := rpConversationFocusAvailable(ctx, tx.conn)
	if err != nil {
		return nil, err
	}
	canInterpret := mode == "orchestrated" || mode == "multi_agent" || mode == "legacy" && focusAvailable
	var focus rpConversationFocus
	if !directed && len(eligible) > 0 && canInterpret && focusAvailable {
		focus, err = readRPNonverbalConversationFocus(ctx, tx.conn, session, fact.PlaceID, cutoff, at)
		if err != nil {
			return nil, err
		}
	}
	candidates := make([]rpActivationCandidate, 0, len(eligible))
	for _, id := range eligible {
		owner, err := rpNonInternalDecisionOwnerSource(ctx, tx.conn, session.InstanceID, session.BranchID, id)
		if err != nil {
			return nil, err
		}
		if owner != "" {
			if _, err := tx.conn.ExecContext(ctx, `INSERT INTO rp_turn_listener_activations(turn_run_id,npc_entity_id,disposition,reason_code,owner_source_event_id) VALUES (?,?,'externally_controlled','external_controller',?)`, runID, id, owner); err != nil {
				return nil, err
			}
			if _, err := tx.conn.ExecContext(ctx, `INSERT OR IGNORE INTO rp_turn_listener_skips(turn_run_id,npc_entity_id,owner_source_event_id) VALUES (?,?,?)`, runID, id, owner); err != nil {
				return nil, err
			}
			continue
		}
		candidates = append(candidates, rpActivationCandidate{id: id, addressed: directed, continuing: canInterpret && focus.actorID == id})
	}
	if canInterpret {
		sort.Slice(candidates, func(i, j int) bool {
			if candidates[i].continuing != candidates[j].continuing {
				return candidates[i].continuing
			}
			return candidates[i].id < candidates[j].id
		})
	}
	activeCount := rpTurnActiveCandidateCount(mode, limit, candidates)
	for rank, candidate := range candidates {
		if rank >= activeCount {
			if _, err := tx.conn.ExecContext(ctx, `INSERT INTO rp_turn_listener_activations(turn_run_id,npc_entity_id,disposition,reason_code) VALUES (?,?,'not_activated','responder_limit')`, runID, candidate.id); err != nil {
				return nil, err
			}
			continue
		}
		reason := "stable_fallback"
		var source any
		if directed {
			reason = "direct_action"
		} else if candidate.continuing {
			reason = "conversation_continuation"
			source = focus.sourceEventID
		} else if mode == "legacy" {
			reason = "legacy_compatible"
		}
		if _, err := tx.conn.ExecContext(ctx, `INSERT INTO rp_turn_listener_activations(turn_run_id,npc_entity_id,disposition,reason_code,activation_rank,conversation_source_event_id) VALUES (?,?,'activated',?,?,?)`, runID, candidate.id, reason, rank, source); err != nil {
			return nil, err
		}
	}
	if err := execAgentOne(ctx, tx.conn, "pin RP action execution mode", `UPDATE rp_turn_runs SET execution_mode=?,responder_limit=?,updated_at_utc=? WHERE turn_run_id=? AND status='npc_deciding'`, mode, limit, s.now().UTC().Format(time.RFC3339Nano), runID); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return s.loadRPTurnActivations(ctx, runID)
}

// Apply the existing personally-heard public-exchange focus policy at the
// actual action cutoff, without private intent or a surrogate utterance.
func readRPNonverbalConversationFocus(ctx context.Context, conn *sql.Conn, session RPSession, place string, cutoff int64, nowText string) (rpConversationFocus, error) {
	var empty rpConversationFocus
	instance, branch, observer := session.InstanceID, session.BranchID, session.ControlledEntityID
	var chapter int64
	available, err := rpSessionChapterAvailable(ctx, conn)
	if err != nil {
		return empty, err
	}
	if available {
		if err := conn.QueryRowContext(ctx, `SELECT chapter_start_sequence FROM rp_sessions WHERE session_id=?`, session.SessionID).Scan(&chapter); err != nil {
			return empty, err
		}
	}
	return readRPConversationFocusAt(ctx, conn, instance, branch, observer, place, nowText, cutoff, chapter)
}
