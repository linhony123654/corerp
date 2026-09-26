package core

import "time"

// OrganizationAgencyPolicy defines the deterministic review rules and thresholds
// for an organization world Actor.
type OrganizationAgencyPolicy struct {
	PolicyID             string `json:"policy_id"`
	OrganizationID       string `json:"organization_id"`
	ManagerPrincipalID   string `json:"manager_principal_id"`
	ReviewFrequencyHours int    `json:"review_frequency_hours"`
	AutomaticReview      bool   `json:"automatic_review,omitempty"`
	ReserveTargetMinor   int64  `json:"reserve_target_minor"`
	HiringThresholdMinor int64  `json:"hiring_threshold_minor"`
	FreezeThresholdMinor int64  `json:"freeze_threshold_minor"`
	TargetPositionID     string `json:"target_position_id"`
	DefaultCapacity      int    `json:"default_capacity"`
	Status               string `json:"status"` // "active", "suspended", "revoked"
}

func (p OrganizationAgencyPolicy) Validate() error {
	if err := validateCareerIDs(p.PolicyID, p.OrganizationID, p.ManagerPrincipalID, p.TargetPositionID); err != nil {
		return err
	}
	if p.ReviewFrequencyHours < 1 || p.ReviewFrequencyHours > 8760 {
		return NewError(CodeInvalidArgument, "review_frequency_hours must be between one hour and one year")
	}
	if p.ReserveTargetMinor < 0 || p.HiringThresholdMinor < 0 || p.FreezeThresholdMinor < 0 ||
		p.ReserveTargetMinor > MaxJSONSafeInteger || p.HiringThresholdMinor > MaxJSONSafeInteger-p.ReserveTargetMinor ||
		p.FreezeThresholdMinor > MaxJSONSafeInteger {
		return NewError(CodeInvalidArgument, "agency financial thresholds must be bounded and nonnegative")
	}
	if p.DefaultCapacity < 1 || p.DefaultCapacity > 100 {
		return NewError(CodeInvalidArgument, "default_capacity must be a valid posting capacity")
	}
	switch p.Status {
	case "active", "suspended", "revoked":
	default:
		return NewError(CodeInvalidArgument, "invalid agency policy status")
	}
	return nil
}

// OrganizationEvidence holds the authorized aggregate business metrics evaluated
// during an organization review. No private employee health, personal chat, or
// household records are exposed.
type OrganizationEvidence struct {
	OrganizationID    string `json:"organization_id"`
	WorldTime         string `json:"world_time"`
	CashBalanceMinor  int64  `json:"cash_balance_minor"`
	WagePayablesMinor int64  `json:"wage_payables_minor"`
	NetHeadroomMinor  int64  `json:"net_headroom_minor"`
	ActiveEmployees   int    `json:"active_employees"`
	CurrentCapacity   int    `json:"current_capacity"`
	PostingStatus     string `json:"posting_status"` // "active", "frozen"
}

// OrganizationDecision records the outcome computed by the deterministic agency policy.
type OrganizationDecision struct {
	DecisionKind      string `json:"decision_kind"` // "no_change", "freeze_recruitment", "unfreeze_recruitment", "expand_capacity", "adjust_schedule"
	Reason            string `json:"reason"`
	TargetPositionID  string `json:"target_position_id"`
	EffectiveCapacity int    `json:"effective_capacity"`
	PostingStatus     string `json:"posting_status"`
	ShiftStartHour    int    `json:"shift_start_hour,omitempty"`
	ShiftEndHour      int    `json:"shift_end_hour,omitempty"`
}

type OrganizationAgencyPolicyRequest struct {
	Binding CareerBinding            `json:"binding"`
	Policy  OrganizationAgencyPolicy `json:"policy"`
}

func (r OrganizationAgencyPolicyRequest) Validate() error {
	if err := r.Binding.Validate(); err != nil {
		return err
	}
	return r.Policy.Validate()
}

type OrganizationReviewRequest struct {
	Binding        CareerBinding `json:"binding"`
	OrganizationID string        `json:"organization_id"`
	PolicyID       string        `json:"policy_id"`
}

func (r OrganizationReviewRequest) Validate() error {
	if err := r.Binding.Validate(); err != nil {
		return err
	}
	return validateCareerIDs(r.OrganizationID, r.PolicyID)
}

type OrganizationReviewResult struct {
	ScheduleSourceEventID string               `json:"schedule_source_event_id,omitempty"`
	PolicySourceEventID   string               `json:"policy_source_event_id"`
	DecisionPrincipalID   string               `json:"decision_principal_id"`
	ReviewID              string               `json:"review_id"`
	OrganizationID        string               `json:"organization_id"`
	PolicyID              string               `json:"policy_id"`
	WorldTime             string               `json:"world_time"`
	Evidence              OrganizationEvidence `json:"evidence"`
	Decision              OrganizationDecision `json:"decision"`
	AnnouncementID        string               `json:"announcement_id,omitempty"`
	EventID               string               `json:"event_id"`
	EventSequence         int64                `json:"event_sequence"`
	Replay                bool                 `json:"replay"`
}

// EvaluateOrganizationPolicy applies the deterministic policy against observed evidence.
func EvaluateOrganizationPolicy(policy OrganizationAgencyPolicy, evidence OrganizationEvidence) OrganizationDecision {
	decision := OrganizationDecision{
		DecisionKind:      "no_change",
		Reason:            "business metrics within normal operating bounds",
		TargetPositionID:  policy.TargetPositionID,
		EffectiveCapacity: evidence.CurrentCapacity,
		PostingStatus:     evidence.PostingStatus,
	}

	// 1. Budget pressure check: if net headroom is below the freeze threshold, freeze recruitment
	if evidence.NetHeadroomMinor < policy.ReserveTargetMinor-policy.FreezeThresholdMinor {
		if evidence.PostingStatus != "frozen" {
			decision.DecisionKind = "freeze_recruitment"
			decision.Reason = "net financial headroom below reserve freeze threshold"
			decision.PostingStatus = "frozen"
			return decision
		}
		decision.Reason = "recruitment remains frozen due to financial strain"
		return decision
	}

	// 2. Budget surplus check: if net headroom exceeds hiring threshold and capacity is below target
	if evidence.NetHeadroomMinor >= policy.ReserveTargetMinor+policy.HiringThresholdMinor {
		if evidence.PostingStatus == "frozen" {
			decision.DecisionKind = "unfreeze_recruitment"
			decision.Reason = "financial headroom restored above reserve threshold"
			decision.PostingStatus = "active"
			return decision
		}
		if evidence.CurrentCapacity < policy.DefaultCapacity {
			decision.DecisionKind = "expand_capacity"
			decision.Reason = "capacity expansion funded by operational surplus"
			decision.EffectiveCapacity = policy.DefaultCapacity
			decision.PostingStatus = "active"
			return decision
		}
	}

	return decision
}

func ValidateWorldTimeFormat(s string) error {
	_, err := time.Parse(time.RFC3339, s)
	return err
}
