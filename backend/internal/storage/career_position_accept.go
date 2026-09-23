package storage

import (
	"context"
	"database/sql"

	"corerp.local/backend/internal/core"
)

func (s *Store) AcceptCareerPositionChange(ctx context.Context, r core.CareerPositionAcceptRequest) (CareerRecord, error) {
	if err := r.Validate(); err != nil {
		return CareerRecord{}, err
	}
	b := r.Binding
	return s.executeCareerCommand(ctx, b, "AcceptCareerPositionChange", r, func(conn *sql.Conn) error {
		record, err := readCareerRecord(ctx, conn, b.InstanceID, b.BranchID, "position_change", r.ChangeID)
		if err != nil {
			return err
		}
		return authorizeCareerCandidate(ctx, conn, b, record.Fact.CandidateID)
	}, func(conn *sql.Conn, c careerCommandContext) (CareerFact, func() error, error) {
		record, err := readCareerRecord(ctx, conn, b.InstanceID, b.BranchID, "position_change", r.ChangeID)
		if err != nil {
			return CareerFact{}, nil, err
		}
		change, assessment := record.Fact.PositionChange, record.Fact.PositionAssessment
		if change == nil || change.Status != "offered" || assessment == nil {
			return CareerFact{}, nil, core.NewError(core.CodeBranchConflict, "position proposal is not pending")
		}
		manager := b
		manager.PrincipalID = change.ManagerPrincipalID
		if err := authorizeCareerManager(ctx, conn, manager, record.Fact.OrganizationID); err != nil {
			return CareerFact{}, nil, err
		}
		var reviewID string
		if err := conn.QueryRowContext(ctx, `SELECT json_extract(payload,'$.record_id') FROM events WHERE event_id=? AND instance_id=? AND branch_id=? AND event_type='RPCareerFactRecorded' AND json_extract(payload,'$.kind')='performance'`, assessment.PerformanceEventID, b.InstanceID, b.BranchID).Scan(&reviewID); err != nil {
			return CareerFact{}, nil, classifyMissing(err, "position assessment review")
		}
		// Re-run the same gates without creating another offer or disclosing
		// private review evidence in the employee's committed command result.
		fresh, _, err := evaluateCareerPositionOffer(ctx, conn, manager, c, core.CareerPositionOfferRequest{Binding: manager, ChangeID: r.ChangeID, ReviewID: reviewID, PositionID: change.ProposedTerms.PositionID, EffectiveFromDay: change.ProposedTerms.EffectiveFromDay, Assessments: assessment.Assessments, Notice: change.Notice})
		if err != nil {
			return CareerFact{}, nil, err
		}
		oldHash, err := core.HashJSON(change)
		if err != nil {
			return CareerFact{}, nil, err
		}
		newHash, err := core.HashJSON(fresh.PositionChange)
		if err != nil {
			return CareerFact{}, nil, err
		}
		if oldHash != newHash {
			return CareerFact{}, nil, core.NewError(core.CodeBranchConflict, "position proposal terms are no longer current")
		}
		job := change.ProposedTerms
		// Preserve agreed overtime at its own workplace/window/rate and reject
		// a new base that would exceed the representable combined daily amount.
		agreements, err := careerAcceptedOvertime(ctx, conn, job.ContractID, job.EffectiveFromDay, job.EffectiveFromDay+31)
		if err != nil {
			return CareerFact{}, nil, err
		}
		maxima := map[int]int64{}
		for _, agreement := range agreements {
			o := agreement.Fact.Overtime
			amount, exists := maxima[o.Day]
			if !exists {
				amount = job.DailyWageMinor
			}
			bonus, ok := checkedMultiplyPositive(int64(o.EndHour-o.StartHour), o.RateMinorPerHour)
			if !ok {
				return CareerFact{}, nil, core.NewError(core.CodeIntegerOverflow, "overtime maximum overflow")
			}
			amount, ok = checkedAdd(amount, bonus)
			if !ok || amount > core.MaxJSONSafeInteger {
				return CareerFact{}, nil, core.NewError(core.CodeInvalidArgument, "position base and accepted overtime exceed supported pay")
			}
			maxima[o.Day] = amount
		}
		change.Status, change.OfferEventID = "accepted", record.EventID
		record.Fact.PositionAssessment = nil
		record.Fact.Employment = &job
		record.Fact.EmploymentChange = &CareerEmploymentChange{Kind: change.Kind, PreviousTermsEventID: change.PreviousTermsEventID, ManagerPrincipalID: change.ManagerPrincipalID, Notice: change.Notice}
		return record.Fact, func() error {
			return queueCareerPayrollItem(ctx, conn, c.EventID, job.EffectiveFromDay, 0, "career_terms_effective")
		}, nil
	})
}
