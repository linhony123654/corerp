package core

type CareerLeaveRequest struct {
	Binding    CareerBinding `json:"binding"`
	LeaveID    string        `json:"leave_id"`
	ContractID string        `json:"contract_id"`
	StartDay   int           `json:"start_day"`
	EndDay     int           `json:"end_day"`
	Reason     string        `json:"reason"`
}

type CareerLeaveReviewRequest struct {
	Binding  CareerBinding `json:"binding"`
	LeaveID  string        `json:"leave_id"`
	Decision string        `json:"decision"`
	Notice   string        `json:"notice"`
}

func (r CareerLeaveRequest) Validate() error {
	if err := r.Binding.Validate(); err != nil {
		return err
	}
	if err := validateCareerIDs(r.LeaveID, r.ContractID); err != nil {
		return err
	}
	if r.StartDay < 0 || r.EndDay <= r.StartDay || r.EndDay-r.StartDay > 30 {
		return NewError(CodeInvalidArgument, "leave needs one to thirty whole days")
	}
	return validateCareerText(r.Reason)
}

func (r CareerLeaveReviewRequest) Validate() error {
	if err := r.Binding.Validate(); err != nil {
		return err
	}
	if err := validateCareerIDs(r.LeaveID); err != nil {
		return err
	}
	if r.Decision != "approve" && r.Decision != "reject" {
		return NewError(CodeInvalidArgument, "unsupported leave decision")
	}
	return validateCareerText(r.Notice)
}
