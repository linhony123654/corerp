package storage

import (
	"context"
	"database/sql"
	"time"

	"corerp.local/backend/internal/core"
)

type CareerInterviewFact struct {
	ApplicationID      string `json:"application_id"`
	ApplicationEventID string `json:"application_event_id"`
	Question           string `json:"question"`
	InviterPrincipalID string `json:"inviter_principal_id"`
	InvitationEventID  string `json:"invitation_event_id"`
	Answer             string `json:"answer,omitempty"`
	ResponseEventID    string `json:"response_event_id,omitempty"`
	Status             string `json:"status"`
}

type CareerEvaluationFact struct {
	ApplicationID        string                               `json:"application_id"`
	InterviewID          string                               `json:"interview_id"`
	InterviewEventID     string                               `json:"interview_event_id"`
	EvaluatorPrincipalID string                               `json:"evaluator_principal_id"`
	Decision             string                               `json:"decision"`
	Assessments          []core.CareerQualificationAssessment `json:"assessments"`
	Reason               string                               `json:"reason"`
	AdvisoryNote         string                               `json:"advisory_note,omitempty"`
}

type CareerOfferFact struct {
	ApplicationID     string   `json:"application_id"`
	EvaluationID      string   `json:"evaluation_id"`
	EvaluationEventID string   `json:"evaluation_event_id"`
	PositionID        string   `json:"position_id"`
	PositionKey       string   `json:"position_key"`
	PostingEventID    string   `json:"posting_event_id"`
	OfferEventID      string   `json:"offer_event_id"`
	DailyWageMinor    int64    `json:"daily_wage_minor"`
	CurrencyID        string   `json:"currency_id"`
	StartsOnDay       int      `json:"starts_on_day"`
	ProbationDays     int      `json:"probation_days"`
	ExpiresAt         string   `json:"expires_at"`
	Status            string   `json:"status"`
	DeclineReason     string   `json:"decline_reason,omitempty"`
	WorkStartHour     int      `json:"work_start_hour"`
	WorkEndHour       int      `json:"work_end_hour"`
	ContractID        string   `json:"contract_id,omitempty"`
	WagePolicy        string   `json:"wage_policy"`
	Capabilities      []string `json:"capabilities,omitempty"`
}

func careerRecordCandidate(f CareerFact) string {
	if f.CandidateID != "" {
		return f.CandidateID
	}
	if f.Application != nil {
		return f.Application.CandidateID
	}
	return ""
}

func authorizeCareerRecordManager(ctx context.Context, conn *sql.Conn, b core.CareerBinding, kind, id string) error {
	record, err := readCareerRecord(ctx, conn, b.InstanceID, b.BranchID, kind, id)
	if err != nil {
		return err
	}
	return authorizeCareerManager(ctx, conn, b, record.Fact.OrganizationID)
}

func authorizeCareerRecordCandidate(ctx context.Context, conn *sql.Conn, b core.CareerBinding, kind, id string) error {
	record, err := readCareerRecord(ctx, conn, b.InstanceID, b.BranchID, kind, id)
	if err != nil {
		return err
	}
	return authorizeCareerCandidate(ctx, conn, b, careerRecordCandidate(record.Fact))
}

func (s *Store) InviteCareerInterview(ctx context.Context, r core.CareerInterviewRequest) (CareerRecord, error) {
	if err := r.Validate(); err != nil {
		return CareerRecord{}, err
	}
	b := r.Binding
	return s.executeCareerCommand(ctx, b, "InviteCareerInterview", r, func(conn *sql.Conn) error {
		return authorizeCareerRecordManager(ctx, conn, b, "application", r.ApplicationID)
	}, func(conn *sql.Conn, c careerCommandContext) (CareerFact, func() error, error) {
		if err := requireNewCareerRecord(ctx, conn, b, "interview", r.InterviewID); err != nil {
			return CareerFact{}, nil, err
		}
		application, err := readCareerRecord(ctx, conn, b.InstanceID, b.BranchID, "application", r.ApplicationID)
		if err != nil {
			return CareerFact{}, nil, err
		}
		if application.Fact.Application == nil || application.Fact.Application.Status != "submitted" {
			return CareerFact{}, nil, core.NewError(core.CodeInvalidArgument, "interview requires a submitted application")
		}
		interview := &CareerInterviewFact{ApplicationID: r.ApplicationID, ApplicationEventID: application.EventID, Question: r.Question, InviterPrincipalID: b.PrincipalID, InvitationEventID: c.EventID, Status: "invited"}
		return CareerFact{Kind: "interview", RecordID: r.InterviewID, OrganizationID: application.Fact.OrganizationID, CandidateID: careerRecordCandidate(application.Fact), Interview: interview}, nil, nil
	})
}

func (s *Store) AnswerCareerInterview(ctx context.Context, r core.CareerInterviewAnswerRequest) (CareerRecord, error) {
	if err := r.Validate(); err != nil {
		return CareerRecord{}, err
	}
	b := r.Binding
	return s.executeCareerCommand(ctx, b, "AnswerCareerInterview", r, func(conn *sql.Conn) error {
		return authorizeCareerRecordCandidate(ctx, conn, b, "interview", r.InterviewID)
	}, func(conn *sql.Conn, c careerCommandContext) (CareerFact, func() error, error) {
		record, err := readCareerRecord(ctx, conn, b.InstanceID, b.BranchID, "interview", r.InterviewID)
		if err != nil {
			return CareerFact{}, nil, err
		}
		if record.Fact.Interview == nil || record.Fact.Interview.Status != "invited" {
			return CareerFact{}, nil, core.NewError(core.CodeBranchConflict, "interview already answered")
		}
		record.Fact.Interview.Answer = r.Answer
		record.Fact.Interview.ResponseEventID = c.EventID
		record.Fact.Interview.Status = "responded"
		return record.Fact, nil, nil
	})
}

func (s *Store) EvaluateCareerApplication(ctx context.Context, r core.CareerEvaluationRequest) (CareerRecord, error) {
	if err := r.Validate(); err != nil {
		return CareerRecord{}, err
	}
	r.Assessments = append([]core.CareerQualificationAssessment{}, r.Assessments...)
	b := r.Binding
	return s.executeCareerCommand(ctx, b, "EvaluateCareerApplication", r, func(conn *sql.Conn) error {
		return authorizeCareerRecordManager(ctx, conn, b, "interview", r.InterviewID)
	}, func(conn *sql.Conn, _ careerCommandContext) (CareerFact, func() error, error) {
		if err := requireNewCareerRecord(ctx, conn, b, "evaluation", r.EvaluationID); err != nil {
			return CareerFact{}, nil, err
		}
		interview, err := readCareerRecord(ctx, conn, b.InstanceID, b.BranchID, "interview", r.InterviewID)
		if err != nil {
			return CareerFact{}, nil, err
		}
		if interview.Fact.Interview == nil || interview.Fact.Interview.Status != "responded" || interview.Fact.Interview.ResponseEventID != interview.EventID {
			return CareerFact{}, nil, core.NewError(core.CodeInvalidArgument, "evaluation needs candidate's actual interview response")
		}
		application, err := readCareerRecord(ctx, conn, b.InstanceID, b.BranchID, "application", interview.Fact.Interview.ApplicationID)
		if err != nil {
			return CareerFact{}, nil, err
		}
		if application.Fact.Application == nil {
			return CareerFact{}, nil, core.NewError(core.CodeProjectionDiverged, "invalid interview application")
		}
		if application.Fact.Application.Status != "submitted" {
			return CareerFact{}, nil, core.NewError(core.CodeBranchConflict, "application is already closed")
		}
		posting, err := readCareerRecord(ctx, conn, b.InstanceID, b.BranchID, "posting", application.Fact.Application.PositionID)
		if err != nil {
			return CareerFact{}, nil, err
		}
		if posting.Fact.Posting == nil || len(posting.Fact.Posting.RequiredQualifications) != len(r.Assessments) {
			return CareerFact{}, nil, core.NewError(core.CodeInvalidArgument, "evaluate every posted qualification requirement")
		}
		for _, code := range posting.Fact.Posting.RequiredQualifications {
			found := false
			for _, assessment := range r.Assessments {
				found = found || assessment.Code == code
			}
			if !found {
				return CareerFact{}, nil, core.NewError(core.CodeInvalidArgument, "assessment differs from posting requirements")
			}
		}
		evaluation := &CareerEvaluationFact{ApplicationID: application.Fact.RecordID, InterviewID: r.InterviewID, InterviewEventID: interview.EventID, EvaluatorPrincipalID: b.PrincipalID, Decision: r.Decision, Assessments: r.Assessments, Reason: r.Reason, AdvisoryNote: r.AdvisoryNote}
		return CareerFact{Kind: "evaluation", RecordID: r.EvaluationID, OrganizationID: interview.Fact.OrganizationID, CandidateID: careerRecordCandidate(interview.Fact), Evaluation: evaluation}, nil, nil
	})
}

func careerPositionKey(instanceID, branchID, positionID string) (string, error) {
	hash, err := core.HashJSON([]string{instanceID, branchID, positionID})
	if err != nil {
		return "", err
	}
	return "career_position_" + hash[7:], nil
}

// SQL alias e is reserved by callers. Latest per record, not every historical
// submitted/offered snapshot: otherwise a declined offer would stay open forever.
const latestCareerRecordPredicate = `NOT EXISTS (SELECT 1 FROM events later WHERE later.instance_id=e.instance_id AND later.branch_id=e.branch_id AND later.event_type=e.event_type AND json_extract(later.payload,'$.kind')=json_extract(e.payload,'$.kind') AND json_extract(later.payload,'$.record_id')=json_extract(e.payload,'$.record_id') AND later.event_sequence>e.event_sequence)`

func careerHasOpenOffer(ctx context.Context, conn *sql.Conn, b core.CareerBinding, applicationID string, now time.Time) (bool, error) {
	rows, err := conn.QueryContext(ctx, `SELECT json_extract(e.payload,'$.offer.expires_at') FROM events e WHERE e.instance_id=? AND e.branch_id=? AND e.event_type='RPCareerFactRecorded' AND json_extract(e.payload,'$.kind')='offer' AND json_extract(e.payload,'$.offer.application_id')=? AND json_extract(e.payload,'$.offer.status')='offered' AND `+latestCareerRecordPredicate, b.InstanceID, b.BranchID, applicationID)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return false, err
		}
		at, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return false, core.NewError(core.CodeProjectionDiverged, "invalid offer expiry evidence")
		}
		if at.After(now) {
			return true, nil
		}
	}
	return false, rows.Err()
}

func (s *Store) OfferCareerEmployment(ctx context.Context, r core.CareerOfferRequest) (CareerRecord, error) {
	if err := r.Validate(); err != nil {
		return CareerRecord{}, err
	}
	if r.Binding.InstanceID != M2DemoInstanceID || r.Binding.BranchID != M2DemoBranchID {
		return CareerRecord{}, core.NewError(core.CodeInvalidArgument, "career offer requires supported M2 calendar/scheduler")
	}
	expires, _ := time.Parse(time.RFC3339, r.ExpiresAt)
	r.ExpiresAt = expires.UTC().Format(time.RFC3339Nano)
	b := r.Binding
	return s.executeCareerCommand(ctx, b, "OfferCareerEmployment", r, func(conn *sql.Conn) error {
		return authorizeCareerRecordManager(ctx, conn, b, "evaluation", r.EvaluationID)
	}, func(conn *sql.Conn, c careerCommandContext) (CareerFact, func() error, error) {
		if err := requireNewCareerRecord(ctx, conn, b, "offer", r.OfferID); err != nil {
			return CareerFact{}, nil, err
		}
		evaluation, err := readCareerRecord(ctx, conn, b.InstanceID, b.BranchID, "evaluation", r.EvaluationID)
		if err != nil {
			return CareerFact{}, nil, err
		}
		e := evaluation.Fact.Evaluation
		if e == nil || e.Decision != "advance" {
			return CareerFact{}, nil, core.NewError(core.CodeInvalidArgument, "offer needs an advance evaluation")
		}
		var latest string
		if err := conn.QueryRowContext(ctx, `SELECT event_id FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPCareerFactRecorded' AND json_extract(payload,'$.kind')='evaluation' AND json_extract(payload,'$.evaluation.application_id')=? ORDER BY event_sequence DESC LIMIT 1`, b.InstanceID, b.BranchID, e.ApplicationID).Scan(&latest); err != nil {
			return CareerFact{}, nil, err
		}
		if latest != evaluation.EventID {
			return CareerFact{}, nil, core.NewError(core.CodeBranchConflict, "evaluation was superseded")
		}
		for _, assessment := range e.Assessments {
			if !assessment.Passed {
				return CareerFact{}, nil, core.NewError(core.CodeInvalidArgument, "advice cannot bypass failed qualification")
			}
		}
		application, err := readCareerRecord(ctx, conn, b.InstanceID, b.BranchID, "application", e.ApplicationID)
		if err != nil {
			return CareerFact{}, nil, err
		}
		if application.Fact.Application == nil || application.Fact.Application.Status != "submitted" {
			return CareerFact{}, nil, core.NewError(core.CodeInvalidArgument, "offer requires a submitted application")
		}
		posting, err := readCareerRecord(ctx, conn, b.InstanceID, b.BranchID, "posting", application.Fact.Application.PositionID)
		if err != nil {
			return CareerFact{}, nil, err
		}
		if posting.Fact.Posting == nil || application.Fact.Application.PostingEventID != posting.EventID {
			return CareerFact{}, nil, core.NewError(core.CodeBranchConflict, "application posting terms changed")
		}
		if err := requireCareerCredentials(ctx, conn, b, application.Fact.Application.CandidateID, posting.Fact.Posting.RequiredCredentials, c.WorldTime); err != nil {
			return CareerFact{}, nil, err
		}
		org, err := readCareerRecord(ctx, conn, b.InstanceID, b.BranchID, "organization", posting.Fact.OrganizationID)
		if err != nil {
			return CareerFact{}, nil, err
		}
		if org.Fact.Organization == nil {
			return CareerFact{}, nil, core.NewError(core.CodeProjectionDiverged, "invalid organization fact")
		}
		key, err := careerPositionKey(b.InstanceID, b.BranchID, posting.Fact.RecordID)
		if err != nil {
			return CareerFact{}, nil, err
		}
		occupied, err := careerPositionOccupancy(ctx, conn, key)
		if err != nil {
			return CareerFact{}, nil, err
		}
		if occupied >= posting.Fact.Posting.Capacity {
			return CareerFact{}, nil, core.NewError(core.CodeBranchConflict, "position has no vacancy")
		}
		now, err := time.Parse(time.RFC3339, c.WorldTime)
		if err != nil {
			return CareerFact{}, nil, err
		}
		start := time.Date(2026, time.September, 22, 0, 0, 0, 0, time.UTC).AddDate(0, 0, r.StartsOnDay)
		if !start.After(now) || start.After(now.Add(30*24*time.Hour)) || !expires.After(now) || expires.After(start) || expires.After(now.Add(7*24*time.Hour)) {
			return CareerFact{}, nil, core.NewError(core.CodeInvalidArgument, "offer start/expiry must be future and bounded; expiry cannot follow start")
		}
		open, err := careerHasOpenOffer(ctx, conn, b, e.ApplicationID, now)
		if err != nil {
			return CareerFact{}, nil, err
		}
		if open {
			return CareerFact{}, nil, core.NewError(core.CodeBranchConflict, "application already has an unexpired offer")
		}
		offer := &CareerOfferFact{ApplicationID: e.ApplicationID, EvaluationID: r.EvaluationID, EvaluationEventID: evaluation.EventID, PositionID: posting.Fact.RecordID, PositionKey: key, PostingEventID: posting.EventID, OfferEventID: c.EventID, DailyWageMinor: posting.Fact.Posting.DailyWageMinor, CurrencyID: org.Fact.Organization.CurrencyID, StartsOnDay: r.StartsOnDay, ProbationDays: r.ProbationDays, ExpiresAt: r.ExpiresAt, Status: "offered"}
		offer.WorkStartHour, offer.WorkEndHour = 8, 12
		offer.WagePolicy = "guaranteed_daily_v1"
		offer.Capabilities = append([]string(nil), posting.Fact.Posting.Capabilities...)
		return CareerFact{Kind: "offer", RecordID: r.OfferID, OrganizationID: posting.Fact.OrganizationID, CandidateID: careerRecordCandidate(application.Fact), Offer: offer}, nil, nil
	})
}

func (s *Store) DeclineCareerOffer(ctx context.Context, r core.CareerOfferDeclineRequest) (CareerRecord, error) {
	if err := r.Validate(); err != nil {
		return CareerRecord{}, err
	}
	b := r.Binding
	return s.executeCareerCommand(ctx, b, "DeclineCareerOffer", r, func(conn *sql.Conn) error {
		return authorizeCareerRecordCandidate(ctx, conn, b, "offer", r.OfferID)
	}, func(conn *sql.Conn, c careerCommandContext) (CareerFact, func() error, error) {
		record, err := readCareerRecord(ctx, conn, b.InstanceID, b.BranchID, "offer", r.OfferID)
		if err != nil {
			return CareerFact{}, nil, err
		}
		if record.Fact.Offer == nil || record.Fact.Offer.Status != "offered" {
			return CareerFact{}, nil, core.NewError(core.CodeBranchConflict, "offer is no longer open")
		}
		expires, err := time.Parse(time.RFC3339, record.Fact.Offer.ExpiresAt)
		if err != nil {
			return CareerFact{}, nil, err
		}
		now, err := time.Parse(time.RFC3339, c.WorldTime)
		if err != nil {
			return CareerFact{}, nil, err
		}
		if !expires.After(now) {
			return CareerFact{}, nil, core.NewError(core.CodeBranchConflict, "offer expired")
		}
		record.Fact.Offer.Status = "declined"
		record.Fact.Offer.DeclineReason = r.Reason
		return record.Fact, nil, nil
	})
}

func (s *Store) ReadCareerRecruitmentRecord(ctx context.Context, principalID, instanceID, branchID, kind, id string) (CareerRecord, error) {
	if kind != "application" && kind != "interview" && kind != "evaluation" && kind != "offer" && kind != "referral" && kind != "performance" && kind != "employment" && kind != "leave" && kind != "overtime" && kind != "grade_scale" && kind != "position_change" {
		return CareerRecord{}, core.NewError(core.CodeInvalidArgument, "unsupported private recruitment record kind")
	}
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return CareerRecord{}, err
	}
	defer tx.Rollback(ctx)
	record, err := readCareerRecord(ctx, tx.conn, instanceID, branchID, kind, id)
	if err != nil {
		return CareerRecord{}, err
	}
	b := core.CareerBinding{PrincipalID: principalID, InstanceID: instanceID, BranchID: branchID}
	if err := authorizeCareerManager(ctx, tx.conn, b, record.Fact.OrganizationID); err != nil {
		if !core.HasCode(err, core.CodeUnauthorized) {
			return CareerRecord{}, err
		}
		// Internal evaluator reasons/advice are not automatically known to the
		// applicant merely because the assessment is about them. Offers and
		// interview messages have their own explicitly addressed read path.
		if kind == "evaluation" || kind == "performance" || kind == "grade_scale" {
			return CareerRecord{}, err
		}
		if err := authorizeCareerCandidate(ctx, tx.conn, b, careerRecordCandidate(record.Fact)); err != nil {
			if !core.HasCode(err, core.CodeUnauthorized) || record.Fact.Referral == nil {
				return CareerRecord{}, err
			}
			if err := authorizeCareerCandidate(ctx, tx.conn, b, record.Fact.Referral.ReferrerID); err != nil {
				return CareerRecord{}, err
			}
		}
		record.Fact.PositionAssessment = nil
		if record.Fact.Leave != nil {
			// Other employees' leave approvals are manager-only evidence.
			record.Fact.Leave.ReviewAssessment = nil
		}
	}
	if record.Fact.Overtime != nil && record.Fact.Overtime.Choice != nil {
		if err := authorizeCareerCandidate(ctx, tx.conn, b, careerRecordCandidate(record.Fact)); err != nil {
			if !core.HasCode(err, core.CodeUnauthorized) {
				return CareerRecord{}, err
			}
			record.Fact.Overtime.Choice = nil
		}
	}
	return record, nil
}
