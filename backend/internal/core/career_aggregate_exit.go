package core

// An exit notice records intent, not completed termination or new-job authority.
type CareerAggregateExitRequest struct {
	Binding        CareerBinding `json:"binding"`
	CandidateID    string        `json:"candidate_id"`
	ContractID     string        `json:"contract_id"`
	FinalEarnedDay int           `json:"final_earned_day"`
	Notice         string        `json:"notice"`
}

func (r CareerAggregateExitRequest) Validate() error {
	if err := r.Binding.Validate(); err != nil {
		return err
	}
	if err := validateCareerIDs(r.CandidateID, r.ContractID); err != nil {
		return err
	}
	if r.FinalEarnedDay < 1 || r.FinalEarnedDay > 30 {
		return NewError(CodeInvalidArgument, "aggregate final earned day is outside the supported contract")
	}
	return validateCareerText(r.Notice)
}
