package core

type CareerOvertimeRequest struct {
	Binding          CareerBinding `json:"binding"`
	OvertimeID       string        `json:"overtime_id"`
	ContractID       string        `json:"contract_id"`
	Day              int           `json:"day"`
	StartHour        int           `json:"start_hour"`
	EndHour          int           `json:"end_hour"`
	RateMinorPerHour int64         `json:"rate_minor_per_hour"`
	Reason           string        `json:"reason"`
}

type CareerOvertimeResponseRequest struct {
	Binding    CareerBinding `json:"binding"`
	OvertimeID string        `json:"overtime_id"`
	Decision   string        `json:"decision"`
	Reason     string        `json:"reason"`
}

func (r CareerOvertimeRequest) Validate() error {
	if err := r.Binding.Validate(); err != nil {
		return err
	}
	if err := validateCareerIDs(r.OvertimeID, r.ContractID); err != nil {
		return err
	}
	if r.Day < 0 || r.StartHour < 0 || r.EndHour > 23 || r.EndHour <= r.StartHour || r.EndHour-r.StartHour > 4 || r.RateMinorPerHour < 1 || r.RateMinorPerHour > MaxJSONSafeInteger/(4*3600) {
		return NewError(CodeInvalidArgument, "invalid overtime window or bounded hourly rate")
	}
	return validateCareerText(r.Reason)
}

func (r CareerOvertimeResponseRequest) Validate() error {
	if err := r.Binding.Validate(); err != nil {
		return err
	}
	if err := validateCareerIDs(r.OvertimeID); err != nil {
		return err
	}
	if r.Decision != "accept" && r.Decision != "decline" && r.Decision != "cancel" && r.Decision != "consider" {
		return NewError(CodeInvalidArgument, "unsupported overtime response")
	}
	return validateCareerText(r.Reason)
}
