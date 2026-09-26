package core

import "strings"

type WorldQARequest struct {
	PrincipalID string             `json:"principal_id"`
	InstanceID  string             `json:"instance_id"`
	BranchID    string             `json:"branch_id"`
	Thresholds  *WorldQAThresholds `json:"thresholds,omitempty"`
}

func (r WorldQARequest) Validate() error {
	if strings.TrimSpace(r.PrincipalID) == "" || strings.TrimSpace(r.InstanceID) == "" || strings.TrimSpace(r.BranchID) == "" {
		return NewError(CodeInvalidArgument, "principal_id, instance_id, and branch_id are required")
	}
	return nil
}

type WorldQAThresholds struct {
	MaxIsolatedEntityRatio  float64 `json:"max_isolated_entity_ratio"`
	MinHousingCoverageRatio float64 `json:"min_housing_coverage_ratio"`
	MaxPastDueRentRatio     float64 `json:"max_past_due_rent_ratio"`
	MaxUnemploymentRatio    float64 `json:"max_unemployment_ratio"`
	MaxStagnantVacancies    int64   `json:"max_stagnant_vacancies"`
}

func DefaultWorldQAThresholds() WorldQAThresholds {
	return WorldQAThresholds{
		MaxIsolatedEntityRatio:  0.5,
		MinHousingCoverageRatio: 0.5,
		MaxPastDueRentRatio:     0.4,
		MaxUnemploymentRatio:    0.8,
		MaxStagnantVacancies:    10,
	}
}

type QAAnomalySeverity string

const (
	SeverityInfo     QAAnomalySeverity = "info"
	SeverityWarning  QAAnomalySeverity = "warning"
	SeverityCritical QAAnomalySeverity = "critical"
)

type WorldQAAnomaly struct {
	Dimension   string            `json:"dimension"`
	Severity    QAAnomalySeverity `json:"severity"`
	Code        string            `json:"code"`
	Message     string            `json:"message"`
	EntityID    string            `json:"entity_id,omitempty"`
	MetricValue float64           `json:"metric_value,omitempty"`
	Threshold   float64           `json:"threshold,omitempty"`
}

type WorldQAStatus string

const (
	StatusHealthy  WorldQAStatus = "HEALTHY"
	StatusWarning  WorldQAStatus = "WARNING"
	StatusCritical WorldQAStatus = "CRITICAL"
)

type PopulationQADimension struct {
	TotalMaterialized int64            `json:"total_materialized"`
	ActiveAgents      int64            `json:"active_agents"`
	Cohorts           map[string]int64 `json:"cohorts"`
	TotalMovements    int64            `json:"total_movements"`
}

type EmploymentQADimension struct {
	ActiveContracts   int64   `json:"active_contracts"`
	EndedContracts    int64   `json:"ended_contracts"`
	TotalPositions    int64   `json:"total_positions"`
	UnfilledPostings  int64   `json:"unfilled_postings"`
	UnemployedCount   int64   `json:"unemployed_count"`
	UnemploymentRatio float64 `json:"unemployment_ratio"`
}

type FinancesQADimension struct {
	TotalAccounts      int64            `json:"total_accounts"`
	BalancesByCurrency map[string]int64 `json:"balances_by_currency"`
	WageAccruedMinor   int64            `json:"wage_accrued_minor"`
	WagePaidMinor      int64            `json:"wage_paid_minor"`
	WageArrearsMinor   int64            `json:"wage_arrears_minor"`
	RentDueMinor       int64            `json:"rent_due_minor"`
	RentPaidMinor      int64            `json:"rent_paid_minor"`
	RentPastDueMinor   int64            `json:"rent_past_due_minor"`
	ArrearsCasesCount  int64            `json:"arrears_cases_count"`
	BankruptcyCount    int64            `json:"bankruptcy_count"`
}

type HouseholdQADimension struct {
	ActiveHouseholds   int64 `json:"active_households"`
	TotalMembers       int64 `json:"total_members"`
	DependentsCount    int64 `json:"dependents_count"`
	CoveredHouseholds  int64 `json:"covered_households"`
	StrainedHouseholds int64 `json:"strained_households"`
	CriticalHouseholds int64 `json:"critical_households"`
}

type HousingQADimension struct {
	TotalResidences      int64   `json:"total_residences"`
	HousedPopulation     int64   `json:"housed_population"`
	UnhousedPopulation   int64   `json:"unhoused_population"`
	HousingCoverageRatio float64 `json:"housing_coverage_ratio"`
}

type CommuteQADimension struct {
	TotalEdges        int64 `json:"total_edges"`
	ActiveJourneys    int64 `json:"active_journeys"`
	ArrivedJourneys   int64 `json:"arrived_journeys"`
	CancelledJourneys int64 `json:"cancelled_journeys"`
	DelayedJourneys   int64 `json:"delayed_journeys"`
}

type RelationshipQADimension struct {
	TotalFamiliarityTies int64   `json:"total_familiarity_ties"`
	EntitiesWithTies     int64   `json:"entities_with_ties"`
	IsolatedEntities     int64   `json:"isolated_entities"`
	IsolatedRatio        float64 `json:"isolated_ratio"`
	GraphDensity         float64 `json:"graph_density"`
}

type EventCount struct {
	EventType string `json:"event_type"`
	Count     int64  `json:"count"`
}

type EventQADimension struct {
	TotalEvents      int64            `json:"total_events"`
	EventsByType     map[string]int64 `json:"events_by_type"`
	TopEventTypes    []EventCount     `json:"top_event_types"`
	EventsPerDay     float64          `json:"events_per_day"`
	RepetitionAlerts int64            `json:"repetition_alerts"`
}

type DecisionQADimension struct {
	TotalDecisions      int64            `json:"total_decisions"`
	DecisionsByAction   map[string]int64 `json:"decisions_by_action"`
	DecisionsByActor    map[string]int64 `json:"decisions_by_actor"`
	ExternalControllers int64            `json:"external_controllers"`
}

type KnowledgeQADimension struct {
	TotalObservations int64            `json:"total_observations"`
	TotalKnowledge    int64            `json:"total_knowledge"`
	ChannelsBreakdown map[string]int64 `json:"channels_breakdown"`
	LeakageIndicators int64            `json:"leakage_indicators"`
}

type OrganizationQADimension struct {
	ActivePolicies    int64            `json:"active_policies"`
	TotalReviews      int64            `json:"total_reviews"`
	ReviewsByDecision map[string]int64 `json:"reviews_by_decision"`
}

type FailedActionsQADimension struct {
	RejectedCommands  int64 `json:"rejected_commands"`
	CancelledJourneys int64 `json:"cancelled_journeys"`
	DefaultReviews    int64 `json:"default_reviews"`
	BankruptcyReviews int64 `json:"bankruptcy_reviews"`
}

type SpatialQADimension struct {
	TotalNodes        int64    `json:"total_nodes"`
	RootPlaces        int64    `json:"root_places"`
	OrphanNodes       int64    `json:"orphan_nodes"`
	UnreachablePlaces []string `json:"unreachable_places"`
}

type ConservationQADimension struct {
	DoubleEntryBalanceZeroSum int64 `json:"double_entry_balance_zero_sum"`
	BalancedPostingsCount     int64 `json:"balanced_postings_count"`
	IssuanceConserved         bool  `json:"issuance_conserved"`
}

type WorldQAReport struct {
	InstanceID   string                   `json:"instance_id"`
	BranchID     string                   `json:"branch_id"`
	AccessLevel  string                   `json:"access_level"`
	HeadSequence int64                    `json:"head_sequence"`
	WorldTime    string                   `json:"world_time"`
	Status       WorldQAStatus            `json:"status"`
	Anomalies    []WorldQAAnomaly         `json:"anomalies"`
	Population   PopulationQADimension    `json:"population"`
	Employment   EmploymentQADimension    `json:"employment"`
	Finances     FinancesQADimension      `json:"finances"`
	Household    HouseholdQADimension     `json:"household"`
	Housing      HousingQADimension       `json:"housing"`
	Commute      CommuteQADimension       `json:"commute"`
	Relationship RelationshipQADimension  `json:"relationship"`
	Events       EventQADimension         `json:"events"`
	Decisions    DecisionQADimension      `json:"decisions"`
	Knowledge    KnowledgeQADimension     `json:"knowledge"`
	Organization OrganizationQADimension  `json:"organization"`
	FailedActions FailedActionsQADimension `json:"failed_actions"`
	Spatial      SpatialQADimension       `json:"spatial"`
	Conservation ConservationQADimension  `json:"conservation"`
}
