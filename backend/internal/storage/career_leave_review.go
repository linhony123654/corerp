package storage

import (
	"context"
	"database/sql"

	"corerp.local/backend/internal/core"
)

type CareerLeaveReviewAssessment struct {
	PolicySourceEventID    string   `json:"policy_source_event_id"`
	MaxConcurrentEmployees int      `json:"max_concurrent_employees"`
	PeakOtherEmployees     int      `json:"peak_other_employees"`
	ApprovalSourceEventIDs []string `json:"approval_source_event_ids,omitempty"`
	Decision               string   `json:"decision"`
	ReasonCode             string   `json:"reason_code"`
}

// Called inside the authorized review transaction: policy, prior approvals and
// the new decision share one head. No provider or independent scheduler owner.
func considerCareerLeave(ctx context.Context, conn *sql.Conn, b core.CareerBinding, fact CareerFact) (CareerLeaveReviewAssessment, error) {
	var out CareerLeaveReviewAssessment
	org, err := readCareerRecord(ctx, conn, b.InstanceID, b.BranchID, "organization", fact.OrganizationID)
	if err != nil {
		return out, err
	}
	if org.Fact.Organization == nil || org.Fact.Organization.Definition.LeaveReviewPolicy == nil {
		return out, core.NewError(core.CodeBranchConflict, "organization has no leave review policy")
	}
	policy := *org.Fact.Organization.Definition.LeaveReviewPolicy
	if err := policy.Validate(); err != nil {
		return out, err
	}
	out.PolicySourceEventID = org.EventID
	out.MaxConcurrentEmployees = policy.MaxConcurrentEmployees
	leave := fact.Leave
	rows, err := conn.QueryContext(ctx, `SELECT e.event_id,json_extract(e.payload,'$.candidate_id'),json_extract(e.payload,'$.leave.start_day'),json_extract(e.payload,'$.leave.end_day')
	 FROM events e WHERE e.instance_id=? AND e.branch_id=? AND e.event_type='RPCareerFactRecorded'
	 AND json_extract(e.payload,'$.kind')='leave' AND json_extract(e.payload,'$.organization_id')=?
	 AND json_extract(e.payload,'$.leave.status')='approved' AND json_extract(e.payload,'$.candidate_id')<>?
	 AND json_extract(e.payload,'$.leave.start_day')<? AND json_extract(e.payload,'$.leave.end_day')>?
	 AND NOT EXISTS (SELECT 1 FROM events n WHERE n.instance_id=e.instance_id AND n.branch_id=e.branch_id
	 AND n.event_type=e.event_type AND json_extract(n.payload,'$.kind')='leave'
	 AND json_extract(n.payload,'$.record_id')=json_extract(e.payload,'$.record_id') AND n.event_sequence>e.event_sequence)
	 ORDER BY e.event_sequence`, b.InstanceID, b.BranchID, fact.OrganizationID, fact.CandidateID, leave.EndDay, leave.StartDay)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	byDay := make([]map[string]bool, leave.EndDay-leave.StartDay)
	for rows.Next() {
		var source, employee string
		var first, end int
		if err := rows.Scan(&source, &employee, &first, &end); err != nil {
			return out, err
		}
		if employee == "" || first < 0 || end <= first {
			return out, core.NewError(core.CodeProjectionDiverged, "invalid organization leave evidence")
		}
		out.ApprovalSourceEventIDs = append(out.ApprovalSourceEventIDs, source)
		for day := max(first, leave.StartDay); day < min(end, leave.EndDay); day++ {
			i := day - leave.StartDay
			if byDay[i] == nil {
				byDay[i] = map[string]bool{}
			}
			byDay[i][employee] = true
			out.PeakOtherEmployees = max(out.PeakOtherEmployees, len(byDay[i]))
		}
	}
	if err := rows.Err(); err != nil {
		return out, err
	}
	out.Decision, out.ReasonCode = "approve", "within_declared_leave_capacity"
	if out.PeakOtherEmployees >= policy.MaxConcurrentEmployees {
		out.Decision, out.ReasonCode = "reject", "declared_leave_capacity_reached"
	}
	return out, nil
}
