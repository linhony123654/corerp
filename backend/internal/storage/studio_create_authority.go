package storage

import (
	"context"
	"database/sql"

	"corerp.local/backend/internal/core"
)

// Source scope already exists; the target world must not exist yet. Creation
// never borrows authority from the proposed target or from an inspector grant.
func authorizeStudioWorldCreation(ctx context.Context, conn *sql.Conn, principal, instance, branch string) error {
	if !studioID(principal) || !studioID(instance) || !studioID(branch) {
		return core.NewError(core.CodeInvalidArgument, "bounded creation authority scope required")
	}
	var count int
	err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM capability_grants g
 JOIN principals p ON p.principal_id=g.principal_id
 JOIN world_instances w ON w.instance_id=g.instance_id
 JOIN branches b ON b.instance_id=g.instance_id AND b.branch_id=g.branch_id
 JOIN events e ON e.event_id=g.definition_event_id AND e.instance_id=g.instance_id AND e.branch_id=g.branch_id
 WHERE p.principal_id=? AND p.principal_type='creator' AND p.status='active' AND w.lifecycle_state='active'
 AND g.instance_id=? AND g.branch_id=? AND g.subject_id=g.branch_id AND g.capability_id='world.create' AND g.status='active'
 AND g.field_scope='["genesis"]' AND g.amount_limit_minor IS NULL
 AND e.event_type='StudioAccessConfigured' AND e.event_sequence<=b.head_sequence
 AND json_extract(e.payload,'$.version')='corerp.studio-access.v1'
 AND json_extract(e.payload,'$.grant_id')=g.grant_id AND json_extract(e.payload,'$.principal_id')=g.principal_id
 AND json_extract(e.payload,'$.capability_id')='world.create' AND json_extract(e.payload,'$.status')='active'
 AND NOT EXISTS(SELECT 1 FROM events n WHERE n.instance_id=e.instance_id AND n.branch_id=e.branch_id
 AND n.event_type='StudioAccessConfigured' AND n.event_sequence>e.event_sequence
 AND json_extract(n.payload,'$.grant_id')=g.grant_id)`, principal, instance, branch).Scan(&count)
	if err != nil {
		return err
	}
	if count != 1 {
		return core.NewError(core.CodeUnauthorized, "explicit sourced world creation authority required")
	}
	return nil
}
