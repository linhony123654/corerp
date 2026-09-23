package core

type CareerReferralRequest struct {
	Binding       CareerBinding `json:"binding"`
	ReferralID    string        `json:"referral_id"`
	PositionID    string        `json:"position_id"`
	ReferrerID    string        `json:"referrer_id"`
	CandidateID   string        `json:"candidate_id"`
	SourceEventID string        `json:"source_event_id"`
	Note          string        `json:"note"`
}

func (r CareerReferralRequest) Validate() error {
	if err := r.Binding.Validate(); err != nil {
		return err
	}
	if err := validateCareerIDs(r.ReferralID, r.PositionID, r.ReferrerID, r.CandidateID, r.SourceEventID); err != nil {
		return err
	}
	if r.ReferrerID == r.CandidateID {
		return NewError(CodeInvalidArgument, "referral requires another known individual")
	}
	return validateCareerText(r.Note)
}
