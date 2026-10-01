package storage

import (
	"context"
	"encoding/json"
	"time"

	"corerp.local/backend/internal/core"
)

// A directed action selects its observed target, not a name parsed from text.
// Bystanders keep their frozen observations but are not activated in this
// first slice. Controller ownership and responder limits remain in force.
func (s *Store) ensureRPNonverbalActivationPlan(ctx context.Context, runID, sessionID string) ([]rpTurnListenerActivation, error) {
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var status, raw, instance, branch string
	if err := tx.conn.QueryRowContext(ctx, `SELECT r.status,r.listener_ids_json,s.instance_id,s.branch_id FROM rp_turn_runs r JOIN rp_sessions s ON s.session_id=r.session_id WHERE r.turn_run_id=? AND r.session_id=? AND r.trigger_kind='nonverbal' AND r.player_event_id IS NOT NULL AND r.player_turn_id IS NULL`, runID, sessionID).Scan(&status, &raw, &instance, &branch); err != nil {
		return nil, classifyMissing(err, "committed nonverbal turn activation")
	}
	var targets []string
	if json.Unmarshal([]byte(raw), &targets) != nil || len(targets) > 1 {
		return nil, core.NewError(core.CodeProjectionDiverged, "nonverbal activation eligibility is invalid")
	}
	var existing int
	if err := tx.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_turn_listener_activations WHERE turn_run_id=?`, runID).Scan(&existing); err != nil {
		return nil, err
	}
	if existing != 0 || status != "npc_deciding" {
		if existing != len(targets) {
			return nil, core.NewError(core.CodeProjectionDiverged, "nonverbal activation accounting is incomplete")
		}
		tx.Rollback(ctx)
		return s.loadRPTurnActivations(ctx, runID)
	}
	mode, limit, err := readRPTurnExecutionMode(ctx, tx.conn, instance, branch, len(targets))
	if err != nil {
		return nil, err
	}
	for _, target := range targets {
		owner, err := rpNonInternalDecisionOwnerSource(ctx, tx.conn, instance, branch, target)
		if err != nil {
			return nil, err
		}
		if owner != "" {
			if _, err := tx.conn.ExecContext(ctx, `INSERT INTO rp_turn_listener_activations(turn_run_id,npc_entity_id,disposition,reason_code,owner_source_event_id) VALUES (?,?,'externally_controlled','external_controller',?)`, runID, target, owner); err != nil {
				return nil, err
			}
			if _, err := tx.conn.ExecContext(ctx, `INSERT OR IGNORE INTO rp_turn_listener_skips(turn_run_id,npc_entity_id,owner_source_event_id) VALUES (?,?,?)`, runID, target, owner); err != nil {
				return nil, err
			}
			continue
		}
		if _, err := tx.conn.ExecContext(ctx, `INSERT INTO rp_turn_listener_activations(turn_run_id,npc_entity_id,disposition,reason_code,activation_rank) VALUES (?,?,'activated','direct_action',0)`, runID, target); err != nil {
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
