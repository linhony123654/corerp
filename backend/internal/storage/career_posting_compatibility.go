package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"reflect"

	"corerp.local/backend/internal/core"
)

// Capacity and recruitment status are operational decisions, not new job
// terms. Keep the original application/offer evidence while checking the latest
// vacancy separately. Every other field remains part of the agreed terms.
func careerPostingTermsEqual(a, b core.CareerPostingDefinition) bool {
	a.Capacity, b.Capacity = 0, 0
	a.Status, b.Status = "", ""
	return reflect.DeepEqual(a, b)
}

func requireCareerPostingCompatibility(ctx context.Context, conn *sql.Conn, b core.CareerBinding, sourceID string, current CareerRecord) error {
	posting := current.Fact.Posting
	if posting == nil {
		return core.NewError(core.CodeProjectionDiverged, "posting lacks terms")
	}
	if posting.Status != "" && posting.Status != "active" {
		return core.NewError(core.CodeBranchConflict, "position recruitment is not active")
	}
	var raw string
	var sequence int64
	if err := conn.QueryRowContext(ctx, `SELECT payload,event_sequence FROM events WHERE instance_id=? AND branch_id=? AND event_id=? AND event_type='RPCareerFactRecorded'`, b.InstanceID, b.BranchID, sourceID).Scan(&raw, &sequence); err != nil {
		return classifyMissing(err, "posting source")
	}
	var source CareerFact
	if err := json.Unmarshal([]byte(raw), &source); err != nil || source.Posting == nil || (source.Kind != "posting" && source.Kind != "organization_review") || source.OrganizationID != current.Fact.OrganizationID || sequence > current.EventSequence {
		return core.NewError(core.CodeProjectionDiverged, "invalid posting source")
	}
	if !careerPostingTermsEqual(*source.Posting, *posting) {
		return core.NewError(core.CodeBranchConflict, "posting terms changed")
	}
	return nil
}
