package core

type CareerPositionOfferRequest struct {
	Binding          CareerBinding                   `json:"binding"`
	ChangeID         string                          `json:"change_id"`
	ReviewID         string                          `json:"review_id"`
	PositionID       string                          `json:"position_id"`
	EffectiveFromDay int                             `json:"effective_from_day"`
	Assessments      []CareerQualificationAssessment `json:"assessments"`
	Notice           string                          `json:"notice"`
}

func (r CareerPositionOfferRequest) Validate() error {
	if err := r.Binding.Validate(); err != nil {
		return err
	}
	if err := validateCareerIDs(r.ChangeID, r.ReviewID, r.PositionID); err != nil {
		return err
	}
	if r.EffectiveFromDay < 1 || r.EffectiveFromDay > 36500 || len(r.Assessments) > 16 {
		return NewError(CodeInvalidArgument, "invalid position change date or qualification count")
	}
	if err := validateCareerText(r.Notice); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, a := range r.Assessments {
		if err := validateCareerIDs(a.Code); err != nil {
			return err
		}
		if seen[a.Code] {
			return NewError(CodeInvalidArgument, "duplicate position qualification")
		}
		seen[a.Code] = true
		if err := validateCareerText(a.Reason); err != nil {
			return err
		}
	}
	return nil
}

type CareerPositionDeclineRequest struct {
	Binding  CareerBinding `json:"binding"`
	ChangeID string        `json:"change_id"`
	Reason   string        `json:"reason"`
}

type CareerPositionAcceptRequest struct {
	Binding  CareerBinding `json:"binding"`
	ChangeID string        `json:"change_id"`
}

func (r CareerPositionAcceptRequest) Validate() error {
	if err := r.Binding.Validate(); err != nil {
		return err
	}
	return validateCareerIDs(r.ChangeID)
}

func (r CareerPositionDeclineRequest) Validate() error {
	if err := r.Binding.Validate(); err != nil {
		return err
	}
	if err := validateCareerIDs(r.ChangeID); err != nil {
		return err
	}
	return validateCareerText(r.Reason)
}
