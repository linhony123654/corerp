package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"corerp.local/backend/internal/core"
)

func readStudioActivationSource(ctx context.Context, q replayQuerier, instance, branch string, through int64) (*StudioActivationFact, error) {
	rows, err := q.QueryContext(ctx, `SELECT event_id,event_sequence,payload FROM events WHERE instance_id=? AND branch_id=? AND event_sequence<=? AND event_type='StudioPackagesActivated' ORDER BY event_sequence LIMIT 2`, instance, branch, through)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result *StudioActivationFact
	for rows.Next() {
		if result != nil {
			return nil, core.NewError(core.CodeProjectionDiverged, "multiple initial Studio activations")
		}
		var id, raw string
		var sequence int64
		var fact StudioActivationFact
		if err := rows.Scan(&id, &sequence, &raw); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(raw), &fact); err != nil {
			return nil, err
		}
		hash, err := core.HashJSON(fact.Lock)
		if err != nil {
			return nil, err
		}
		epoch, err := core.StudioWorldObjectID(instance, "epoch", "packages")
		if err != nil {
			return nil, err
		}
		if fact.Version != "corerp.studio-activation.v1" || fact.EpochID != epoch || fact.Lock.ActivationEventID != id || fact.StartSequence != sequence+1 || fact.RulesetHash != hash {
			return nil, core.NewError(core.CodeProjectionDiverged, "invalid Studio activation source")
		}
		result = &fact
	}
	return result, rows.Err()
}

func studioActivationDifferences(ctx context.Context, q replayQuerier, instance, branch string, through int64) ([]ProjectionDifference, error) {
	fact, err := readStudioActivationSource(ctx, q, instance, branch, through)
	if err != nil || fact == nil {
		return nil, err
	}
	raw, err := core.CanonicalJSON(fact.Lock)
	if err != nil {
		return nil, err
	}
	var count int
	if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM rule_epochs initial JOIN rule_epochs active ON active.instance_id=initial.instance_id AND active.branch_id=initial.branch_id WHERE initial.instance_id=? AND initial.branch_id=? AND initial.epoch_id='epoch_0' AND initial.start_sequence=1 AND initial.end_sequence=? AND active.epoch_id=? AND active.start_sequence=? AND active.end_sequence IS NULL AND active.ruleset_hash=? AND active.lock_document=? AND active.activation_event_id=?`, instance, branch, fact.StartSequence, fact.EpochID, fact.StartSequence, fact.RulesetHash, string(raw), fact.Lock.ActivationEventID).Scan(&count); err != nil {
		return nil, err
	}
	if count == 1 {
		return nil, nil
	}
	return []ProjectionDifference{{Projection: "studio_rule_epoch", Key: fact.EpochID, Expected: 1, Actual: 0, ExpectedText: "exact immutable activation source", ActualText: "epoch range or pinned lock differs"}}, nil
}

func repairStudioActivation(ctx context.Context, conn *sql.Conn, instance, branch string, through int64) error {
	diffs, err := studioActivationDifferences(ctx, conn, instance, branch, through)
	if err != nil || len(diffs) == 0 {
		return err
	}
	fact, err := readStudioActivationSource(ctx, conn, instance, branch, through)
	if err != nil {
		return err
	}
	raw, err := core.CanonicalJSON(fact.Lock)
	if err != nil {
		return err
	}
	if err := execAgentOne(ctx, conn, "repair Studio preparation range", `UPDATE rule_epochs SET start_sequence=1,end_sequence=? WHERE instance_id=? AND branch_id=? AND epoch_id='epoch_0'`, fact.StartSequence, instance, branch); err != nil {
		return err
	}
	var existing int
	err = conn.QueryRowContext(ctx, `SELECT 1 FROM rule_epochs WHERE instance_id=? AND branch_id=? AND epoch_id=?`, instance, branch, fact.EpochID).Scan(&existing)
	if errors.Is(err, sql.ErrNoRows) {
		return execAgentOne(ctx, conn, "restore Studio active epoch", `INSERT INTO rule_epochs(instance_id,branch_id,epoch_id,start_sequence,ruleset_hash,lock_document,activation_event_id) VALUES (?,?,?,?,?,?,?)`, instance, branch, fact.EpochID, fact.StartSequence, fact.RulesetHash, string(raw), fact.Lock.ActivationEventID)
	}
	if err != nil {
		return err
	}
	return execAgentOne(ctx, conn, "repair Studio active epoch", `UPDATE rule_epochs SET start_sequence=?,end_sequence=NULL,ruleset_hash=?,lock_document=?,activation_event_id=? WHERE instance_id=? AND branch_id=? AND epoch_id=?`, fact.StartSequence, fact.RulesetHash, string(raw), fact.Lock.ActivationEventID, instance, branch, fact.EpochID)
}
