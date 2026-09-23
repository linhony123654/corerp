package core

type CareerExitRequest struct {
	Binding          CareerBinding `json:"binding"`
	ContractID       string        `json:"contract_id"`
	Kind             string        `json:"kind"`
	EffectiveFromDay int           `json:"effective_from_day"`
	Notice           string        `json:"notice"`
}

func (r CareerExitRequest) Validate() error {
	if err := r.Binding.Validate(); err != nil {
		return err
	}
	if err := validateCareerIDs(r.ContractID); err != nil {
		return err
	}
	if r.Kind != "resignation" && r.Kind != "termination" && r.Kind != "layoff" {
		return NewError(CodeInvalidArgument, "unsupported employment exit kind")
	}
	if r.EffectiveFromDay < 1 || r.EffectiveFromDay > 36500 {
		return NewError(CodeInvalidArgument, "invalid employment exit day")
	}
	return validateCareerText(r.Notice)
}
