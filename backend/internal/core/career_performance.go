package core

type CareerPerformanceRequest struct {
	Binding          CareerBinding `json:"binding"`
	ReviewID         string        `json:"review_id"`
	ContractID       string        `json:"contract_id"`
	EvidenceEventIDs []string      `json:"evidence_event_ids"`
	Assessment       string        `json:"assessment"`
	Reason           string        `json:"reason"`
	AdvisoryNote     string        `json:"advisory_note,omitempty"`
}

type CareerRegularizationRequest struct {
	Binding  CareerBinding `json:"binding"`
	ReviewID string        `json:"review_id"`
	Notice   string        `json:"notice"`
}

func (r CareerPerformanceRequest) Validate() error {
	if err := r.Binding.Validate(); err != nil {
		return err
	}
	if err := validateCareerIDs(r.ReviewID, r.ContractID); err != nil {
		return err
	}
	if r.Assessment != "meets_expectations" && r.Assessment != "needs_improvement" {
		return NewError(CodeInvalidArgument, "unsupported organization performance assessment")
	}
	if len(r.EvidenceEventIDs) < 1 || len(r.EvidenceEventIDs) > 16 {
		return NewError(CodeInvalidArgument, "performance requires one to sixteen work evidence Events")
	}
	seen := map[string]bool{}
	for _, id := range r.EvidenceEventIDs {
		if err := validateCareerIDs(id); err != nil {
			return err
		}
		if seen[id] {
			return NewError(CodeInvalidArgument, "duplicate performance evidence")
		}
		seen[id] = true
	}
	if err := validateCareerText(r.Reason); err != nil {
		return err
	}
	if r.AdvisoryNote != "" {
		return validateCareerText(r.AdvisoryNote)
	}
	return nil
}

func (r CareerRegularizationRequest) Validate() error {
	if err := r.Binding.Validate(); err != nil {
		return err
	}
	if err := validateCareerIDs(r.ReviewID); err != nil {
		return err
	}
	return validateCareerText(r.Notice)
}
