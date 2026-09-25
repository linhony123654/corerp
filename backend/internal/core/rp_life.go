package core

import "crypto/sha256"

// These are views of existing facts, not mutable character/economic copies.
type RPDisposition struct {
	Version       string   `json:"version"`
	SourceEventID string   `json:"source_event_id"`
	Sociability   int      `json:"sociability"`
	Caution       int      `json:"caution"`
	Patience      int      `json:"patience"`
	Values        []string `json:"values"`
}
type RPNeed struct {
	Code           string   `json:"code"`
	Urgency        string   `json:"urgency"`
	SourceEventIDs []string `json:"source_event_ids"`
}
type RPGoal struct {
	SubjectEntityID string   `json:"subject_entity_id,omitempty"`
	Code            string   `json:"code"`
	SourceNeed      string   `json:"source_need"`
	SourceEventIDs  []string `json:"source_event_ids"`
	ConflictsWith   []string `json:"conflicts_with"`
}
type RPLifeMemory struct {
	Kind            string `json:"kind"`
	SubjectEntityID string `json:"subject_entity_id"`
	Text            string `json:"text,omitempty"`
	WorldTime       string `json:"world_time"`
	SourceEventID   string `json:"source_event_id"`
}
type RPRelationship struct {
	SubjectEntityID string   `json:"subject_entity_id"`
	Familiarity     int      `json:"familiarity"`
	Trust           int      `json:"trust"`
	Affinity        int      `json:"affinity"`
	Tension         int      `json:"tension"`
	Obligation      int      `json:"obligation"`
	Role            string   `json:"role"`
	SourceEventIDs  []string `json:"source_event_ids"`
}
type RPOwnEmployment struct {
	PositionID           string   `json:"position_id,omitempty"`
	OccupationID         string   `json:"occupation_id,omitempty"`
	Grade                string   `json:"grade,omitempty"`
	PositionCapabilities []string `json:"position_capabilities,omitempty"`
	Status               string   `json:"status,omitempty"`
	WorkplaceID          string   `json:"workplace_id,omitempty"`
	StartsOnDay          int      `json:"starts_on_day,omitempty"`
	PayPeriodDays        int      `json:"pay_period_days,omitempty"`
	ContractID           string   `json:"contract_id"`
	OrganizationID       string   `json:"organization_id"`
	WageMinor            int64    `json:"wage_minor"`
	SourceEventID        string   `json:"source_event_id"`
}
type RPLifeContext struct {
	LawCases               []RPLawCase            `json:"law_cases,omitempty"`
	Health                 *RPHealthSelf          `json:"health,omitempty"`
	CultureAffiliations    []RPCultureAffiliation `json:"culture_affiliations,omitempty"`
	CultureExperiences     []RPCultureExperience  `json:"culture_experiences,omitempty"`
	Unemployment           *RPUnemployment        `json:"unemployment,omitempty"`
	Background             *RPBackground          `json:"background,omitempty"`
	RoutineSourceEventID   string                 `json:"routine_source_event_id"`
	Disposition            RPDisposition          `json:"disposition"`
	ReceivableMinor        int64                  `json:"receivable_minor"`
	LiabilityMinor         int64                  `json:"liability_minor"`
	RentDueMinor           int64                  `json:"rent_due_minor"`
	HouseholdPressure      *RPHouseholdPressure   `json:"household_pressure,omitempty"`
	EconomicSourceEventIDs []string               `json:"economic_source_event_ids"`
	Employment             []RPOwnEmployment      `json:"employment"`
	Relationships          []RPRelationship       `json:"relationships"`
	SalientMemories        []RPLifeMemory         `json:"salient_memories"`
	RecentWork             []RPLifeMemory         `json:"recent_work"`
	Needs                  []RPNeed               `json:"needs"`
	Goals                  []RPGoal               `json:"goals"`
	Commitments            []RPSocialEvidence     `json:"commitments"`
}

// An individual's perceived functional state, not a diagnosis or a copy of
// private health Event payloads. Other people receive only separate lawful
// observation/knowledge claims.
type RPHealthSelf struct {
	FatigueLevel     string   `json:"fatigue_level"`
	SleepDebtLevel   string   `json:"sleep_debt_level"`
	FunctionalImpact string   `json:"functional_impact"`
	ConditionImpact  string   `json:"condition_impact,omitempty"`
	Symptoms         []string `json:"symptoms"`
}

type RPUnemployment struct {
	PreviousContractID string `json:"previous_contract_id"`
	SinceDay           int    `json:"since_day"`
	Kind               string `json:"kind"`
	SourceEventID      string `json:"source_event_id"`
}

// Forecast only: no member's personal cash or identity is disclosed here.
type RPHouseholdPressure struct {
	RentMinor        int64  `json:"rent_minor"`
	OwnShareMinor    int64  `json:"own_share_minor"`
	RentFundMinor    int64  `json:"rent_fund_minor"`
	OutstandingMinor int64  `json:"outstanding_minor"`
	PressureLevel    string `json:"pressure_level"`
	NextDueWorldTime string `json:"next_due_world_time"`
	// Exact aggregate wage and gap amounts are local decision inputs. In a
	// two-person household, exporting them could reveal the other wage by
	// subtracting the observer's own known income.
	ExpectedIncomeMinor int64 `json:"-"`
	CoverageGapMinor    int64 `json:"-"`
}

// This versioned seed is a minimal stable tendency, not a generated biography.
// Identity/materialization evidence is immutable; future algorithms require a
// new explicit version and must not silently reinterpret existing characters.
func DeriveRPDisposition(entityID, materializationEventID string) RPDisposition {
	seed := sha256.Sum256([]byte("corerp.disposition.v1:" + entityID + ":" + materializationEventID))
	values := []string{"security", "care", "autonomy", "duty"}
	first, second := int(seed[3])%len(values), int(seed[4])%len(values)
	if second == first {
		second = (first + 1) % len(values)
	}
	return RPDisposition{Version: "corerp.disposition.v1", SourceEventID: materializationEventID, Sociability: int(seed[0]) % 3, Caution: int(seed[1]) % 3, Patience: int(seed[2]) % 3, Values: []string{values[first], values[second]}}
}

func DeriveRPLifeGoals(input RPDecisionInput, life *RPLifeContext) {
	life.Needs = make([]RPNeed, 0)
	life.Goals = make([]RPGoal, 0)
	add := func(need, urgency, goal string, sources []string, conflicts ...string) {
		life.Needs = append(life.Needs, RPNeed{Code: need, Urgency: urgency, SourceEventIDs: sources})
		life.Goals = append(life.Goals, RPGoal{Code: goal, SourceNeed: need, SourceEventIDs: sources, ConflictsWith: conflicts})
	}
	// Receivables are NOT spendable cash. Rent may already be represented in
	// liabilities, so use the larger obligation instead of double counting.
	obligations := life.LiabilityMinor
	if life.RentDueMinor > obligations {
		obligations = life.RentDueMinor
	}
	reserve := int64(100 + 25*life.Disposition.Caution)
	for _, value := range life.Disposition.Values {
		if value == "security" {
			reserve += 25
		}
	}
	if input.OwnAssetMinor < reserve || input.OwnAssetMinor < obligations {
		goal := "stabilize_income"
		if life.ReceivableMinor > 0 {
			goal = "collect_money_owed"
		}
		add("cash_security", "high", goal, life.EconomicSourceEventIDs, "discretionary_spending", "extended_socializing")
	}
	if life.HouseholdPressure != nil && life.HouseholdPressure.CoverageGapMinor > 0 {
		add("household_budget_pressure", "high", "stabilize_household_income", life.EconomicSourceEventIDs, "discretionary_spending", "unplanned_housing_cost")
	}
	if life.Health != nil && life.Health.FatigueLevel == "moderate" {
		add("fatigue", "high", "prioritize_rest", []string{}, "optional_evening_activity")
	}
	if input.NextSchedule != nil && input.NextSchedule.ActivityCode == "work" {
		sources := []string{}
		if input.NextSchedule.SourceEventID != "" {
			sources = append(sources, input.NextSchedule.SourceEventID)
		}
		add("work_commitment", "high", "keep_work_schedule", sources, "extended_socializing")
	}
	if life.Unemployment != nil {
		add("employment_continuity", "normal", "find_work", []string{life.Unemployment.SourceEventID})
	}
	for _, relation := range life.Relationships {
		if relation.Tension > 2+life.Disposition.Patience {
			add("personal_boundaries", "high", "avoid_conflict", relation.SourceEventIDs, "social_contact:"+relation.SubjectEntityID)
			life.Goals[len(life.Goals)-1].SubjectEntityID = relation.SubjectEntityID
		}
	}
	for _, promise := range life.Commitments {
		if promise.ActorEntityID == input.NPCEntityID {
			add("social_commitment", "normal", "honor_commitment", []string{promise.PromiseEventID}, "unplanned_travel")
			life.Goals[len(life.Goals)-1].SubjectEntityID = promise.TargetEntityID
		}
	}
	if len(life.Goals) == 0 {
		add("ordinary_routine", "normal", input.GoalCode, []string{life.RoutineSourceEventID})
	}
}
