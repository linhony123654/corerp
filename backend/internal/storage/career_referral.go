package storage

import (
	"context"
	"database/sql"

	"corerp.local/backend/internal/core"
)

type CareerReferralFact struct {
	PositionID         string `json:"position_id"`
	PostingEventID     string `json:"posting_event_id"`
	ReferrerID         string `json:"referrer_id"`
	ObservationEventID string `json:"observation_event_id"`
	Note               string `json:"note"`
}

// Referral records a known person's recommendation, not the candidate's
// consent, qualification or employment. Only their own application can use it.
func (s *Store) ReferCareerCandidate(ctx context.Context, r core.CareerReferralRequest) (CareerRecord, error) {
	if err := r.Validate(); err != nil {
		return CareerRecord{}, err
	}
	b := r.Binding
	return s.executeCareerCommand(ctx, b, "ReferCareerCandidate", r, func(conn *sql.Conn) error {
		return authorizeCareerCandidate(ctx, conn, b, r.ReferrerID)
	}, func(conn *sql.Conn, _ careerCommandContext) (CareerFact, func() error, error) {
		if err := requireNewCareerRecord(ctx, conn, b, "referral", r.ReferralID); err != nil {
			return CareerFact{}, nil, err
		}
		if err := validateRPBinding(ctx, conn, b.InstanceID, b.BranchID, r.CandidateID); err != nil {
			return CareerFact{}, nil, err
		}
		posting, err := readCareerRecord(ctx, conn, b.InstanceID, b.BranchID, "posting", r.PositionID)
		if err != nil {
			return CareerFact{}, nil, err
		}
		var source string
		if err := conn.QueryRowContext(ctx, `SELECT o.source_event_id FROM observation_records o JOIN events e ON e.event_id=o.source_event_id WHERE o.source_event_id=? AND o.observer_agent_id=? AND o.subject_agent_id=? AND e.instance_id=? AND e.branch_id=? LIMIT 1`, r.SourceEventID, r.ReferrerID, r.CandidateID, b.InstanceID, b.BranchID).Scan(&source); err != nil {
			return CareerFact{}, nil, classifyMissing(err, "referrer's actual candidate observation")
		}
		referral := &CareerReferralFact{PositionID: r.PositionID, PostingEventID: posting.EventID, ReferrerID: r.ReferrerID, ObservationEventID: source, Note: r.Note}
		return CareerFact{Kind: "referral", RecordID: r.ReferralID, OrganizationID: posting.Fact.OrganizationID, CandidateID: r.CandidateID, Referral: referral}, nil, nil
	})
}
