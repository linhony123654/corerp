package core

import "strings"

type WorldObserverPerspective string

const (
	PerspectiveMacro        WorldObserverPerspective = "macro"
	PerspectiveEntity       WorldObserverPerspective = "entity"
	PerspectiveOrganization WorldObserverPerspective = "organization"
	PerspectiveRelationship WorldObserverPerspective = "relationship"
	PerspectiveDigest       WorldObserverPerspective = "digest"
)

type WorldObserverRequest struct {
	PrincipalID    string                   `json:"principal_id"`
	InstanceID     string                   `json:"instance_id"`
	BranchID       string                   `json:"branch_id"`
	Perspective    WorldObserverPerspective `json:"perspective"`
	TargetEntityID string                   `json:"target_entity_id,omitempty"`
	TargetOrgID    string                   `json:"target_org_id,omitempty"`
	WindowStart    string                   `json:"window_start,omitempty"`
	WindowEnd      string                   `json:"window_end,omitempty"`
	Limit          int                      `json:"limit,omitempty"`
}

func (r WorldObserverRequest) Validate() error {
	if strings.TrimSpace(r.PrincipalID) == "" || strings.TrimSpace(r.InstanceID) == "" || strings.TrimSpace(r.BranchID) == "" {
		return NewError(CodeInvalidArgument, "principal_id, instance_id, and branch_id are required")
	}
	switch r.Perspective {
	case "", PerspectiveMacro, PerspectiveEntity, PerspectiveOrganization, PerspectiveRelationship, PerspectiveDigest:
	default:
		return NewError(CodeInvalidArgument, "unsupported observer perspective")
	}
	if r.Perspective == PerspectiveEntity && strings.TrimSpace(r.TargetEntityID) == "" {
		return NewError(CodeInvalidArgument, "target_entity_id required for entity perspective")
	}
	if r.Perspective == PerspectiveOrganization && strings.TrimSpace(r.TargetOrgID) == "" {
		return NewError(CodeInvalidArgument, "target_org_id required for organization perspective")
	}
	return nil
}

type ObserverMacroEvent struct {
	EventID       string `json:"event_id"`
	Sequence      int64  `json:"sequence"`
	WorldTime     string `json:"world_time"`
	EventType     string `json:"event_type"`
	Category      string `json:"category"`
	Headline      string `json:"headline"`
	Description   string `json:"description"`
	InspectorLink string `json:"inspector_link"`
}

type ObserverEntityDecision struct {
	EventID       string `json:"event_id"`
	Sequence      int64  `json:"sequence"`
	WorldTime     string `json:"world_time"`
	Action        string `json:"action"`
	ReasonCode    string `json:"reason_code,omitempty"`
	InspectorLink string `json:"inspector_link"`
}

type ObserverEntityLife struct {
	EntityID           string                   `json:"entity_id"`
	DisplayName        string                   `json:"display_name"`
	CohortID           string                   `json:"cohort_id"`
	WorkPlace          string                   `json:"work_place,omitempty"`
	EmployerID         string                   `json:"employer_id,omitempty"`
	PositionTitle      string                   `json:"position_title,omitempty"`
	WageMinor          int64                    `json:"wage_minor,omitempty"`
	HouseholdID        string                   `json:"household_id,omitempty"`
	HouseholdName      string                   `json:"household_name,omitempty"`
	ResidencePlaceID   string                   `json:"residence_place_id,omitempty"`
	RelationshipsCount int64                    `json:"relationships_count"`
	RecentDecisions    []ObserverEntityDecision `json:"recent_decisions"`
	InspectorLink      string                   `json:"inspector_link"`
}

type ObserverOrgReview struct {
	ReviewID      string `json:"review_id"`
	WorldTime     string `json:"world_time"`
	DecisionKind  string `json:"decision_kind"`
	EventID       string `json:"event_id"`
	InspectorLink string `json:"inspector_link"`
}

type ObserverOrganization struct {
	OrganizationID  string              `json:"organization_id"`
	DisplayName     string              `json:"display_name"`
	ActiveEmployees int64               `json:"active_employees"`
	BalanceMinor    *int64              `json:"balance_minor,omitempty"`
	ActivePostings  int64               `json:"active_postings"`
	RecentReviews   []ObserverOrgReview `json:"recent_reviews"`
	InspectorLink   string              `json:"inspector_link"`
}

type ObserverRelationshipChange struct {
	ObserverEntityID string `json:"observer_entity_id"`
	SubjectEntityID  string `json:"subject_entity_id"`
	WorldTime        string `json:"world_time"`
	OriginKind       string `json:"origin_kind"`
	SourceEventID    string `json:"source_event_id"`
	InspectorLink    string `json:"inspector_link"`
}

type ObserverDigest struct {
	WindowStart      string               `json:"window_start"`
	WindowEnd        string               `json:"window_end"`
	TotalEvents      int64                `json:"total_events"`
	NewEntitiesCount int64                `json:"new_entities_count"`
	HouseholdsFormed int64                `json:"households_formed"`
	ContractsFormed  int64                `json:"contracts_formed"`
	DisbursedWages   int64                `json:"disbursed_wages"`
	MacroHighlights  []string             `json:"macro_highlights"`
	KeyEvents        []ObserverMacroEvent `json:"key_events"`
}

type WorldObserverReport struct {
	InstanceID          string                        `json:"instance_id"`
	BranchID            string                        `json:"branch_id"`
	AccessLevel         string                        `json:"access_level"`
	HeadSequence        int64                         `json:"head_sequence"`
	WorldTime           string                        `json:"world_time"`
	Perspective         WorldObserverPerspective      `json:"perspective"`
	MacroEvents         []ObserverMacroEvent          `json:"macro_events,omitempty"`
	EntityLife          *ObserverEntityLife           `json:"entity_life,omitempty"`
	Organization        *ObserverOrganization         `json:"organization,omitempty"`
	RelationshipChanges []ObserverRelationshipChange `json:"relationship_changes,omitempty"`
	Digest              *ObserverDigest               `json:"digest,omitempty"`
}
