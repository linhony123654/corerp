package core

import "strings"

// Position-derived management is distinct from an independent appointment so
// leaving a role cannot revoke separately held organization authority.
const CareerPositionManageCapability = "world.career.position.manage"

// Career commands use existing principal/world/head authority. Binding is a
// named field (not embedding) so CanonicalJSON includes every authorization and
// idempotency field in the request hash.
type CareerBinding struct {
	PrincipalID    string `json:"principal_id"`
	InstanceID     string `json:"instance_id"`
	BranchID       string `json:"branch_id"`
	ExpectedHead   int64  `json:"expected_head"`
	IdempotencyKey string `json:"idempotency_key"`
}

type CareerOrganizationDefinition struct {
	LeaveReviewPolicy  *CareerLeaveReviewPolicy `json:"leave_review_policy,omitempty"`
	OrganizationID     string                   `json:"organization_id"`
	DisplayName        string                   `json:"display_name"`
	ManagerPrincipalID string                   `json:"manager_principal_id"`
	WorkplaceID        string                   `json:"workplace_id"`
}

type CareerOrganizationRequest struct {
	Binding      CareerBinding                `json:"binding"`
	Organization CareerOrganizationDefinition `json:"organization"`
}

// A posting identifies a bounded, actual position. Occupation and grade are
// distinct from that position; no field assigns universal social standing.
type CareerPostingDefinition struct {
	PositionID             string   `json:"position_id"`
	OrganizationID         string   `json:"organization_id"`
	Title                  string   `json:"title"`
	OccupationID           string   `json:"occupation_id"`
	Grade                  string   `json:"grade"`
	Capacity               int      `json:"capacity"`
	DailyWageMinor         int64    `json:"daily_wage_minor"`
	RequiredQualifications []string `json:"required_qualifications"`
	Capabilities           []string `json:"capabilities,omitempty"`
}

type CareerPostingRequest struct {
	Binding CareerBinding           `json:"binding"`
	Posting CareerPostingDefinition `json:"posting"`
}

type CareerApplicationRequest struct {
	Binding       CareerBinding `json:"binding"`
	ApplicationID string        `json:"application_id"`
	PositionID    string        `json:"position_id"`
	CandidateID   string        `json:"candidate_id"`
	Statement     string        `json:"statement"`
	ReferralID    string        `json:"referral_id,omitempty"`
}

func (b CareerBinding) Validate() error {
	if err := validateCareerIDs(b.PrincipalID, b.InstanceID, b.BranchID, b.IdempotencyKey); err != nil {
		return err
	}
	if b.ExpectedHead < 1 || b.ExpectedHead >= MaxJSONSafeInteger {
		return NewError(CodeInvalidArgument, "invalid career expected head")
	}
	return nil
}

func (r CareerOrganizationRequest) Validate() error {
	if err := r.Binding.Validate(); err != nil {
		return err
	}
	o := r.Organization
	if o.LeaveReviewPolicy != nil {
		if err := o.LeaveReviewPolicy.Validate(); err != nil {
			return err
		}
	}
	return validateCareerIDs(o.OrganizationID, o.DisplayName, o.ManagerPrincipalID, o.WorkplaceID)
}

func (r CareerPostingRequest) Validate() error {
	if err := r.Binding.Validate(); err != nil {
		return err
	}
	p := r.Posting
	if err := validateCareerIDs(p.PositionID, p.OrganizationID, p.Title, p.OccupationID, p.Grade); err != nil {
		return err
	}
	if p.Capacity < 1 || p.Capacity > 100 || p.DailyWageMinor < 1 || p.DailyWageMinor > MaxJSONSafeInteger || len(p.RequiredQualifications) > 16 {
		return NewError(CodeInvalidArgument, "invalid vacancy capacity, wage or requirements")
	}
	seen := map[string]bool{}
	if len(p.Capabilities) > 1 || (len(p.Capabilities) == 1 && p.Capabilities[0] != CareerPositionManageCapability) {
		return NewError(CodeInvalidArgument, "position capability is not an allowed organization-scoped delegation")
	}
	for _, code := range p.RequiredQualifications {
		if err := validateCareerIDs(code); err != nil {
			return err
		}
		if seen[code] {
			return NewError(CodeInvalidArgument, "duplicate qualification requirement")
		}
		seen[code] = true
	}
	return nil
}

func (r CareerApplicationRequest) Validate() error {
	if err := r.Binding.Validate(); err != nil {
		return err
	}
	if err := validateCareerIDs(r.ApplicationID, r.PositionID, r.CandidateID); err != nil {
		return err
	}
	if r.ReferralID != "" {
		if err := validateCareerIDs(r.ReferralID); err != nil {
			return err
		}
	}
	if strings.TrimSpace(r.Statement) == "" || len(r.Statement) > 2000 {
		return NewError(CodeInvalidArgument, "application statement must be bounded and nonempty")
	}
	return nil
}

func validateCareerIDs(values ...string) error {
	for _, value := range values {
		if value == "" || value != strings.TrimSpace(value) || len(value) > 256 || strings.ContainsAny(value, "\x00\r\n") {
			return NewError(CodeInvalidArgument, "invalid career identifier or label")
		}
	}
	return nil
}
