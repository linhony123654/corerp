package storage

import (
	"context"
	"database/sql"
	"encoding/json"

	"corerp.local/backend/internal/core"
)

type studioReadySource struct {
	EventID string
	Fact    StudioWorldReadyFact
}

func readStudioReadySource(ctx context.Context, q replayQuerier, instance, branch string, through int64) (*studioReadySource, error) {
	rows, err := q.QueryContext(ctx, `SELECT event_id,payload FROM events WHERE instance_id=? AND branch_id=? AND event_sequence<=? AND event_type='StudioWorldReady' ORDER BY event_sequence LIMIT 2`, instance, branch, through)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result *studioReadySource
	for rows.Next() {
		if result != nil {
			return nil, core.NewError(core.CodeProjectionDiverged, "multiple initial Studio ready sources")
		}
		var source studioReadySource
		var raw string
		if err := rows.Scan(&source.EventID, &raw); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(raw), &source.Fact); err != nil {
			return nil, err
		}
		genesis, _ := core.StudioWorldObjectID(instance, "event", "genesis")
		grant, _ := core.StudioWorldObjectID(instance, "grant", "player-control")
		if source.Fact.Version != "corerp.studio-ready.v1" || source.Fact.GenesisEventID != genesis || source.Fact.GrantID != grant || !studioID(source.Fact.PlayerPrincipalID) || !studioID(source.Fact.EntityID) || !studioID(source.Fact.ActivationEventID) {
			return nil, core.NewError(core.CodeProjectionDiverged, "invalid Studio ready source")
		}
		result = &source
	}
	return result, rows.Err()
}

func studioReadinessDifferences(ctx context.Context, q replayQuerier, instance, branch string, through int64) ([]ProjectionDifference, error) {
	source, err := readStudioReadySource(ctx, q, instance, branch, through)
	if err != nil || source == nil {
		return nil, err
	}
	f := source.Fact
	var exact int
	if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM capability_grants g JOIN world_instances w ON w.instance_id=g.instance_id WHERE g.grant_id=? AND g.principal_id=? AND g.capability_id='world.rp.control' AND g.instance_id=? AND g.branch_id=? AND g.subject_id=? AND g.field_scope='[]' AND g.amount_limit_minor IS NULL AND g.status='active' AND g.definition_event_id=? AND w.lifecycle_state='active'`, f.GrantID, f.PlayerPrincipalID, instance, branch, f.EntityID, source.EventID).Scan(&exact); err != nil {
		return nil, err
	}
	if exact == 1 {
		return nil, nil
	}
	return []ProjectionDifference{{Projection: "studio_readiness", Key: f.GrantID, Expected: 1, Actual: 0, ExpectedText: "exact saved player control and active lifecycle", ActualText: "saved readiness projection differs"}}, nil
}

func repairStudioReadiness(ctx context.Context, conn *sql.Conn, instance, branch string, through int64) error {
	diffs, err := studioReadinessDifferences(ctx, conn, instance, branch, through)
	if err != nil || len(diffs) == 0 {
		return err
	}
	source, err := readStudioReadySource(ctx, conn, instance, branch, through)
	if err != nil {
		return err
	}
	f := source.Fact
	if err := execAgentOne(ctx, conn, "restore saved Studio player control", `INSERT INTO capability_grants(grant_id,principal_id,capability_id,instance_id,branch_id,subject_id,field_scope,status,definition_event_id) VALUES (?,?,'world.rp.control',?,?,?,'[]','active',?) ON CONFLICT(grant_id) DO UPDATE SET principal_id=excluded.principal_id,capability_id=excluded.capability_id,instance_id=excluded.instance_id,branch_id=excluded.branch_id,subject_id=excluded.subject_id,field_scope=excluded.field_scope,status=excluded.status,definition_event_id=excluded.definition_event_id,amount_limit_minor=NULL`, f.GrantID, f.PlayerPrincipalID, instance, branch, f.EntityID, source.EventID); err != nil {
		return err
	}
	return execAgentOne(ctx, conn, "restore saved Studio lifecycle", `UPDATE world_instances SET lifecycle_state='active' WHERE instance_id=?`, instance)
}
