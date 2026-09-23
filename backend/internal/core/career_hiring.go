package core

import (
	"strings"
	"time"
)

type CareerInterviewRequest struct {
	Binding       CareerBinding `json:"binding"`
	InterviewID   string        `json:"interview_id"`
	ApplicationID string        `json:"application_id"`
	Question      string        `json:"question"`
}

type CareerInterviewAnswerRequest struct {
	Binding     CareerBinding `json:"binding"`
	InterviewID string        `json:"interview_id"`
	Answer      string        `json:"answer"`
}

// An organization assessment, not an objective global skill or license.
// Evidence is bound by storage to the candidate's actual interview response.
type CareerQualificationAssessment struct {
	Code   string `json:"code"`
	Passed bool   `json:"passed"`
	Reason string `json:"reason"`
}

type CareerEvaluationRequest struct {
	Binding      CareerBinding                   `json:"binding"`
	EvaluationID string                          `json:"evaluation_id"`
	InterviewID  string                          `json:"interview_id"`
	Decision     string                          `json:"decision"`
	Assessments  []CareerQualificationAssessment `json:"assessments"`
	Reason       string                          `json:"reason"`
	AdvisoryNote string                          `json:"advisory_note,omitempty"`
}

type CareerOfferRequest struct {
	Binding       CareerBinding `json:"binding"`
	OfferID       string        `json:"offer_id"`
	EvaluationID  string        `json:"evaluation_id"`
	StartsOnDay   int           `json:"starts_on_day"`
	ProbationDays int           `json:"probation_days"`
	ExpiresAt     string        `json:"expires_at"`
}

type CareerOfferDeclineRequest struct {
	Binding CareerBinding `json:"binding"`
	OfferID string        `json:"offer_id"`
	Reason  string        `json:"reason"`
}

func validateCareerText(text string) error {
	if strings.TrimSpace(text) == "" || len(text) > 4000 || strings.ContainsRune(text, '\x00') {
		return NewError(CodeInvalidArgument, "career text must be nonempty and bounded")
	}
	return nil
}

func (r CareerInterviewRequest) Validate() error {
	if err := r.Binding.Validate(); err != nil {
		return err
	}
	if err := validateCareerIDs(r.InterviewID, r.ApplicationID); err != nil {
		return err
	}
	return validateCareerText(r.Question)
}

func (r CareerInterviewAnswerRequest) Validate() error {
	if err := r.Binding.Validate(); err != nil {
		return err
	}
	if err := validateCareerIDs(r.InterviewID); err != nil {
		return err
	}
	return validateCareerText(r.Answer)
}

func (r CareerEvaluationRequest) Validate() error {
	if err := r.Binding.Validate(); err != nil {
		return err
	}
	if err := validateCareerIDs(r.EvaluationID, r.InterviewID); err != nil {
		return err
	}
	if r.Decision != "advance" && r.Decision != "reject" {
		return NewError(CodeInvalidArgument, "evaluation decision must be advance or reject")
	}
	if err := validateCareerText(r.Reason); err != nil {
		return err
	}
	if len(r.Assessments) > 16 || len(r.AdvisoryNote) > 4000 {
		return NewError(CodeInvalidArgument, "evaluation evidence exceeds bound")
	}
	seen := map[string]bool{}
	for _, assessment := range r.Assessments {
		if err := validateCareerIDs(assessment.Code); err != nil {
			return err
		}
		if seen[assessment.Code] {
			return NewError(CodeInvalidArgument, "duplicate qualification assessment")
		}
		seen[assessment.Code] = true
		if err := validateCareerText(assessment.Reason); err != nil {
			return err
		}
	}
	return nil
}

func (r CareerOfferRequest) Validate() error {
	if err := r.Binding.Validate(); err != nil {
		return err
	}
	if err := validateCareerIDs(r.OfferID, r.EvaluationID); err != nil {
		return err
	}
	if r.StartsOnDay < 1 || r.StartsOnDay > 36500 || r.ProbationDays < 0 || r.ProbationDays > 90 {
		return NewError(CodeInvalidArgument, "invalid offer start/probation")
	}
	if _, err := time.Parse(time.RFC3339, r.ExpiresAt); err != nil {
		return NewError(CodeInvalidArgument, "offer expiry must be RFC3339")
	}
	return nil
}

func (r CareerOfferDeclineRequest) Validate() error {
	if err := r.Binding.Validate(); err != nil {
		return err
	}
	if err := validateCareerIDs(r.OfferID); err != nil {
		return err
	}
	return validateCareerText(r.Reason)
}
