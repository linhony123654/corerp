package core

type CareerRaiseRequest struct {
	Binding          CareerBinding `json:"binding"`
	ContractID       string        `json:"contract_id"`
	DailyWageMinor   int64         `json:"daily_wage_minor"`
	EffectiveFromDay int           `json:"effective_from_day"`
	Notice           string        `json:"notice"`
}

func (r CareerRaiseRequest) Validate() error {
	if err := r.Binding.Validate(); err != nil {
		return err
	}
	if err := validateCareerIDs(r.ContractID); err != nil {
		return err
	}
	if r.DailyWageMinor < 1 || r.DailyWageMinor > MaxJSONSafeInteger || r.EffectiveFromDay < 1 {
		return NewError(CodeInvalidArgument, "invalid raised wage or effective day")
	}
	return validateCareerText(r.Notice)
}
