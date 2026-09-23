package storage

import (
	"context"
	"database/sql"

	"corerp.local/backend/internal/core"
)

// Read only announcements this employee actually heard. A later heard posting
// in another organization supersedes an earlier one. Unheard changes do not
// magically update the employee's belief; no private roster is exposed.
func careerKnownCoworkers(ctx context.Context, conn *sql.Conn, b core.CareerBinding, employee, organization string) ([]core.CareerKnownCoworker, error) {
	rows, err := conn.QueryContext(ctx, `SELECT json_extract(e.payload,'$.announcement.employee_id'),json_extract(e.payload,'$.organization_id'),e.event_id
		FROM events e WHERE e.instance_id=? AND e.branch_id=? AND e.event_type='RPCareerFactRecorded'
		AND json_extract(e.payload,'$.kind')='announcement'
		AND EXISTS (SELECT 1 FROM json_each(e.payload,'$.announcement.listener_ids') l WHERE l.value=?)
		ORDER BY e.event_sequence DESC`, b.InstanceID, b.BranchID, employee)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	seen := map[string]bool{}
	var known []core.CareerKnownCoworker
	for rows.Next() {
		var person, org, source string
		if err := rows.Scan(&person, &org, &source); err != nil {
			return nil, err
		}
		if seen[person] {
			continue
		}
		seen[person] = true
		if person != employee && org == organization {
			known = append(known, core.CareerKnownCoworker{EntityID: person, SourceEventID: source})
		}
	}
	return known, rows.Err()
}
