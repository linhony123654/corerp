package storage

import (
	"context"
	"database/sql"
	"fmt"

	"corerp.local/backend/internal/core"
)

// This index is derived from the immutable posted journal, never from account
// balances or an NPC's model context. Compare and rebuild can repair it without
// changing any economic fact.
func rpAccountEconomicSourceDifferences(ctx context.Context, q replayQuerier, instanceID, branchID string) ([]ProjectionDifference, error) {
	var expected, actual, missing, extra int64
	err := q.QueryRowContext(ctx, `WITH expected_rows AS (
 SELECT DISTINCT p.account_id,j.entry_id,j.event_id,e.instance_id,e.branch_id,e.event_sequence
 FROM postings p JOIN journal_entries j ON j.entry_id=p.entry_id AND j.status='posted'
 JOIN events e ON e.event_id=j.event_id WHERE e.instance_id=? AND e.branch_id=?),
 actual_rows AS (
 SELECT account_id,entry_id,event_id,instance_id,branch_id,event_sequence
 FROM rp_account_economic_sources WHERE instance_id=? AND branch_id=?),
 missing_rows AS (SELECT * FROM expected_rows EXCEPT SELECT * FROM actual_rows),
 extra_rows AS (SELECT * FROM actual_rows EXCEPT SELECT * FROM expected_rows)
 SELECT (SELECT COUNT(*) FROM expected_rows),(SELECT COUNT(*) FROM actual_rows),
        (SELECT COUNT(*) FROM missing_rows),(SELECT COUNT(*) FROM extra_rows)`,
		instanceID, branchID, instanceID, branchID).Scan(&expected, &actual, &missing, &extra)
	if err != nil {
		return nil, core.WrapError(core.CodeStorageFailure, "compare posted account sources", err)
	}
	if missing == 0 && extra == 0 {
		return nil, nil
	}
	return []ProjectionDifference{{
		Projection: "rp_account_economic_sources", Key: instanceID + "/" + branchID,
		Expected: expected, Actual: actual,
		ExpectedText: fmt.Sprintf("missing=%d", missing), ActualText: fmt.Sprintf("extra=%d", extra),
	}}, nil
}

func repairRPAccountEconomicSources(ctx context.Context, conn *sql.Conn, instanceID, branchID string, differences []ProjectionDifference) error {
	if len(differences) == 0 {
		return nil
	}
	if _, err := conn.ExecContext(ctx, `DELETE FROM rp_account_economic_sources WHERE instance_id=? AND branch_id=?`, instanceID, branchID); err != nil {
		return core.WrapError(core.CodeStorageFailure, "clear posted account source index", err)
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO rp_account_economic_sources(account_id,entry_id,event_id,instance_id,branch_id,event_sequence)
 SELECT DISTINCT p.account_id,j.entry_id,j.event_id,e.instance_id,e.branch_id,e.event_sequence
 FROM postings p JOIN journal_entries j ON j.entry_id=p.entry_id AND j.status='posted'
 JOIN events e ON e.event_id=j.event_id WHERE e.instance_id=? AND e.branch_id=?`, instanceID, branchID); err != nil {
		return core.WrapError(core.CodeStorageFailure, "rebuild posted account source index", err)
	}
	return nil
}
