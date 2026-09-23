package storage

import (
	"context"
	"database/sql"
)

// Accepted, unactivated moves reserve the destination while retaining the
// current position. Offers alone never reserve; each contract is counted once.
func careerPositionOccupancy(ctx context.Context, conn *sql.Conn, key string) (int, error) {
	var count int
	err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM employment_contracts c WHERE c.status='active' AND (c.position_id=? OR EXISTS (
	 SELECT 1 FROM events e JOIN events d ON d.event_id=c.definition_event_id AND d.instance_id=e.instance_id AND d.branch_id=e.branch_id
	 WHERE e.event_type='RPCareerFactRecorded' AND json_extract(e.payload,'$.employment.contract_id')=c.contract_id
	 AND json_extract(e.payload,'$.position_change.status')='accepted' AND json_extract(e.payload,'$.employment.position_key')=?
	 AND NOT EXISTS (SELECT 1 FROM events a WHERE a.instance_id=e.instance_id AND a.branch_id=e.branch_id AND a.event_type='CareerEmploymentTermsActivated' AND json_extract(a.payload,'$.terms_event_id')=e.event_id)))`, key, key).Scan(&count)
	return count, err
}
