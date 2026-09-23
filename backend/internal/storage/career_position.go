package storage

import (
	"context"
	"database/sql"

	"corerp.local/backend/internal/core"
)

// Private organization judgment: not a universal qualification or an employee
// statement. The actual completed-work review is the evidence source.
type CareerPositionAssessment struct {
	PerformanceEventID string                               `json:"performance_event_id"`
	Assessments        []core.CareerQualificationAssessment `json:"assessments"`
}

// ProposedTerms deliberately is not CareerFact.Employment: an offer cannot
// become authoritative wage/position terms before employee acceptance.
type CareerPositionChangeFact struct {
	ContractID           string               `json:"contract_id"`
	Kind                 string               `json:"kind"`
	Status               string               `json:"status"`
	PreviousTermsEventID string               `json:"previous_terms_event_id"`
	PostingEventID       string               `json:"posting_event_id"`
	GradeScaleEventID    string               `json:"grade_scale_event_id"`
	ManagerPrincipalID   string               `json:"manager_principal_id"`
	ProposedTerms        CareerEmploymentFact `json:"proposed_terms"`
	Notice               string               `json:"notice"`
	ResponseReason       string               `json:"response_reason,omitempty"`
	OfferEventID         string               `json:"offer_event_id,omitempty"`
}

func (s *Store) OfferCareerPositionChange(ctx context.Context, r core.CareerPositionOfferRequest) (CareerRecord, error) {
	if err := r.Validate(); err != nil {
		return CareerRecord{}, err
	}
	r.Assessments = append([]core.CareerQualificationAssessment{}, r.Assessments...)
	b := r.Binding
	return s.executeCareerCommand(ctx, b, "OfferCareerPositionChange", r, func(conn *sql.Conn) error {
		return authorizeCareerRecordManager(ctx, conn, b, "performance", r.ReviewID)
	}, func(conn *sql.Conn, c careerCommandContext) (CareerFact, func() error, error) {
		if err := requireNewCareerRecord(ctx, conn, b, "position_change", r.ChangeID); err != nil {
			return CareerFact{}, nil, err
		}
		return evaluateCareerPositionOffer(ctx, conn, b, c, r)
	})
}

func evaluateCareerPositionOffer(ctx context.Context, conn *sql.Conn, b core.CareerBinding, c careerCommandContext, r core.CareerPositionOfferRequest) (CareerFact, func() error, error) {
	review, err := readCareerRecord(ctx, conn, b.InstanceID, b.BranchID, "performance", r.ReviewID)
	if err != nil {
		return CareerFact{}, nil, err
	}
	p := review.Fact.Performance
	if p == nil {
		return CareerFact{}, nil, core.NewError(core.CodeProjectionDiverged, "missing position review")
	}
	job, source, day, err := currentCareerEmployment(ctx, conn, b, p.ContractID, c.WorldTime)
	if err != nil {
		return CareerFact{}, nil, err
	}
	if source != p.TermsEventID {
		return CareerFact{}, nil, core.NewError(core.CodeBranchConflict, "position review refers to superseded employment")
	}
	var latest string
	if err := conn.QueryRowContext(ctx, `SELECT event_id FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPCareerFactRecorded' AND json_extract(payload,'$.kind')='performance' AND json_extract(payload,'$.performance.contract_id')=? ORDER BY event_sequence DESC LIMIT 1`, b.InstanceID, b.BranchID, job.ContractID).Scan(&latest); err != nil {
		return CareerFact{}, nil, err
	}
	if latest != review.EventID {
		return CareerFact{}, nil, core.NewError(core.CodeBranchConflict, "position review was superseded")
	}
	reviewer := b
	reviewer.PrincipalID = p.ReviewerPrincipalID
	if err := authorizeCareerManager(ctx, conn, reviewer, job.OrganizationID); err != nil {
		return CareerFact{}, nil, err
	}
	if r.EffectiveFromDay <= day || r.EffectiveFromDay > day+30 {
		return CareerFact{}, nil, core.NewError(core.CodeInvalidArgument, "position change must be one to thirty days ahead")
	}
	var pending int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPCareerFactRecorded' AND json_extract(payload,'$.employment.contract_id')=? AND json_extract(payload,'$.employment.effective_from_day')>?`, b.InstanceID, b.BranchID, job.ContractID, day).Scan(&pending); err != nil {
		return CareerFact{}, nil, err
	}
	if pending != 0 {
		return CareerFact{}, nil, core.NewError(core.CodeBranchConflict, "resolve pending employment terms before a position offer")
	}
	posting, err := readCareerRecord(ctx, conn, b.InstanceID, b.BranchID, "posting", r.PositionID)
	if err != nil {
		return CareerFact{}, nil, err
	}
	target := posting.Fact.Posting
	if target == nil || target.OrganizationID != job.OrganizationID || target.PositionID == job.PositionID {
		return CareerFact{}, nil, core.NewError(core.CodeInvalidArgument, "internal position change requires a different position in the same organization")
	}
	scale, err := readCareerRecord(ctx, conn, b.InstanceID, b.BranchID, "grade_scale", job.OrganizationID)
	if err != nil {
		return CareerFact{}, nil, err
	}
	if scale.Fact.GradeScale == nil {
		return CareerFact{}, nil, core.NewError(core.CodeProjectionDiverged, "missing position grade scale")
	}
	kind, err := scale.Fact.GradeScale.Transition(job.Grade, target.Grade)
	if err != nil {
		return CareerFact{}, nil, err
	}
	if kind == "promotion" && p.Assessment != "meets_expectations" {
		return CareerFact{}, nil, core.NewError(core.CodeBranchConflict, "promotion requires favorable current performance evidence")
	}
	if len(r.Assessments) != len(target.RequiredQualifications) {
		return CareerFact{}, nil, core.NewError(core.CodeInvalidArgument, "position qualifications must exactly cover target requirements")
	}
	passed := map[string]bool{}
	for _, a := range r.Assessments {
		passed[a.Code] = a.Passed
	}
	for _, code := range target.RequiredQualifications {
		if !passed[code] {
			return CareerFact{}, nil, core.NewError(core.CodeInvalidArgument, "target position qualification is absent or did not pass")
		}
	}
	key, err := careerPositionKey(b.InstanceID, b.BranchID, target.PositionID)
	if err != nil {
		return CareerFact{}, nil, err
	}
	occupied, err := careerPositionOccupancy(ctx, conn, key)
	if err != nil {
		return CareerFact{}, nil, err
	}
	if occupied >= target.Capacity {
		return CareerFact{}, nil, core.NewError(core.CodeBranchConflict, "target position has no vacancy")
	}
	job.PositionID, job.PositionKey, job.OccupationID, job.Grade = target.PositionID, key, target.OccupationID, target.Grade
	job.DailyWageMinor, job.EffectiveFromDay = target.DailyWageMinor, r.EffectiveFromDay
	job.Capabilities = append([]string(nil), target.Capabilities...)
	job.TermVersion++
	change := &CareerPositionChangeFact{ContractID: job.ContractID, Kind: kind, Status: "offered", PreviousTermsEventID: source, PostingEventID: posting.EventID, GradeScaleEventID: scale.EventID, ManagerPrincipalID: b.PrincipalID, ProposedTerms: job, Notice: r.Notice}
	return CareerFact{Kind: "position_change", RecordID: r.ChangeID, OrganizationID: job.OrganizationID, CandidateID: job.EmployeeID, PositionChange: change, PositionAssessment: &CareerPositionAssessment{PerformanceEventID: review.EventID, Assessments: r.Assessments}}, nil, nil
}

func (s *Store) DeclineCareerPositionChange(ctx context.Context, r core.CareerPositionDeclineRequest) (CareerRecord, error) {
	if err := r.Validate(); err != nil {
		return CareerRecord{}, err
	}
	b := r.Binding
	return s.executeCareerCommand(ctx, b, "DeclineCareerPositionChange", r, func(conn *sql.Conn) error {
		record, err := readCareerRecord(ctx, conn, b.InstanceID, b.BranchID, "position_change", r.ChangeID)
		if err != nil {
			return err
		}
		return authorizeCareerCandidate(ctx, conn, b, record.Fact.CandidateID)
	}, func(conn *sql.Conn, _ careerCommandContext) (CareerFact, func() error, error) {
		record, err := readCareerRecord(ctx, conn, b.InstanceID, b.BranchID, "position_change", r.ChangeID)
		if err != nil {
			return CareerFact{}, nil, err
		}
		if record.Fact.PositionChange == nil || record.Fact.PositionChange.Status != "offered" {
			return CareerFact{}, nil, core.NewError(core.CodeBranchConflict, "position offer is not pending")
		}
		change := record.Fact.PositionChange
		change.Status, change.ResponseReason, change.OfferEventID = "declined", r.Reason, record.EventID
		// Persisted employee results must not contain the manager's assessment.
		record.Fact.PositionAssessment = nil
		return record.Fact, nil, nil
	})
}
