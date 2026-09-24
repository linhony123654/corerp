package storage

import (
	"context"
	"database/sql"

	"corerp.local/backend/internal/core"
)

type LawDisputeForwardRequest struct {
	Binding        core.CareerBinding `json:"binding"`
	InstitutionID  string             `json:"institution_id"`
	ActorID        string             `json:"actor_id"`
	DisputeEventID string             `json:"dispute_event_id"`
}
type LawDisputeForwardFact struct {
	ActorID          string `json:"actor_id"`
	DisputeEventID   string `json:"dispute_event_id"`
	ReviewerID       string `json:"reviewer_id"`
	AuthorityEventID string `json:"authority_event_id"`
}

func lawDisputeReceipt(ctx context.Context, conn *sql.Conn, b core.CareerBinding, institution, dispute, reviewer string, d LawDisputeFact) (string, error) {
	for _, id := range d.ReviewerIDs {
		if id == reviewer {
			return dispute, nil
		}
	}
	var source string
	err := conn.QueryRowContext(ctx, `SELECT event_id FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPInstitutionFactRecorded' AND json_extract(payload,'$.kind')='law_dispute_forward' AND json_extract(payload,'$.institution_id')=? AND json_extract(payload,'$.dispute_forward.dispute_event_id')=? AND json_extract(payload,'$.dispute_forward.actor_id')=? AND json_extract(payload,'$.dispute_forward.reviewer_id')=? ORDER BY event_sequence LIMIT 1`, b.InstanceID, b.BranchID, institution, dispute, d.ActorID, reviewer).Scan(&source)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return source, err
}

// An affected actor explicitly resubmits their existing filing to the current
// office holder. Holding office alone never reveals predecessor case files.
func (s *Store) ForwardRPLawDispute(ctx context.Context, r LawDisputeForwardRequest) (InstitutionRecord, error) {
	return executePrivateFactCommand(s, ctx, r.Binding, "ForwardRPLawDispute", r, privateFactDomain{"institution", "RPInstitutionFactRecorded", `{"authorization":"own-law-dispute-v1"}`}, func(conn *sql.Conn) error {
		if err := authorizeCareerCandidate(ctx, conn, r.Binding, r.ActorID); err != nil {
			return err
		}
		fact, err := readInstitutionFact(ctx, conn, r.Binding, r.InstitutionID, r.DisputeEventID, "law_dispute")
		if err != nil {
			return err
		}
		if fact.Dispute == nil || fact.Dispute.ActorID != r.ActorID {
			return core.NewError(core.CodeUnauthorized, "only filing actor may resubmit dispute")
		}
		return nil
	}, func(conn *sql.Conn, c privateFactContext) (InstitutionFact, func() error, error) {
		fact, err := readInstitutionFact(ctx, conn, r.Binding, r.InstitutionID, r.DisputeEventID, "law_dispute")
		if err != nil {
			return InstitutionFact{}, nil, err
		}
		d := fact.Dispute
		var count int
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPInstitutionFactRecorded' AND json_extract(payload,'$.kind')='law_review' AND json_extract(payload,'$.review.enforcement_event_id')=?`, r.Binding.InstanceID, r.Binding.BranchID, d.EnforcementEventID).Scan(&count); err != nil {
			return InstitutionFact{}, nil, err
		}
		if count != 0 {
			return InstitutionFact{}, nil, core.NewError(core.CodeBranchConflict, "review is already final")
		}
		role, status, authority, err := readInstitutionRole(ctx, conn, r.Binding, r.InstitutionID, institutionReview)
		if err != nil {
			return InstitutionFact{}, nil, err
		}
		if status != "active" {
			return InstitutionFact{}, nil, core.NewError(core.CodeBranchConflict, "institution currently has no active reviewer")
		}
		receipt, err := lawDisputeReceipt(ctx, conn, r.Binding, r.InstitutionID, r.DisputeEventID, role.EntityID, *d)
		if err != nil {
			return InstitutionFact{}, nil, err
		}
		if receipt != "" {
			return InstitutionFact{}, nil, core.NewError(core.CodeBranchConflict, "current reviewer already received filing")
		}
		return InstitutionFact{Version: "corerp.institution.v1", Kind: "law_dispute_forward", InstitutionID: r.InstitutionID, DisputeForward: &LawDisputeForwardFact{ActorID: r.ActorID, DisputeEventID: r.DisputeEventID, ReviewerID: role.EntityID, AuthorityEventID: authority}}, nil, nil
	})
}
