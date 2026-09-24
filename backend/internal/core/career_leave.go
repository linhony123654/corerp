package core

// Optional creator-authored organization rule, not a universal entitlement or
// staffing claim. Explicit manager approvals remain independent of this rule.
type CareerLeaveReviewPolicy struct {
	MaxConcurrentEmployees int `json:"max_concurrent_employees"`
	AutoReviewDelayMinutes int `json:"auto_review_delay_minutes,omitempty"`
}

func (p CareerLeaveReviewPolicy) Validate() error {
	if p.MaxConcurrentEmployees < 1 || p.MaxConcurrentEmployees > 100 || p.AutoReviewDelayMinutes < 0 || p.AutoReviewDelayMinutes > 1440 {
		return NewError(CodeInvalidArgument, "leave review policy requires one to one hundred concurrent employees")
	}
	return nil
}

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
	if r.Decision != "approve" && r.Decision != "reject" && r.Decision != "consider" {
		return NewError(CodeInvalidArgument, "unsupported leave decision")
	}
	return validateCareerText(r.Notice)
}
