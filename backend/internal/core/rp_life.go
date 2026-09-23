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
	ContractID     string `json:"contract_id"`
	OrganizationID string `json:"organization_id"`
	WageMinor      int64  `json:"wage_minor"`
	SourceEventID  string `json:"source_event_id"`
}
type RPLifeContext struct {
	RoutineSourceEventID   string             `json:"routine_source_event_id"`
	Disposition            RPDisposition      `json:"disposition"`
	ReceivableMinor        int64              `json:"receivable_minor"`
	LiabilityMinor         int64              `json:"liability_minor"`
	RentDueMinor           int64              `json:"rent_due_minor"`
	EconomicSourceEventIDs []string           `json:"economic_source_event_ids"`
	Employment             []RPOwnEmployment  `json:"employment"`
	Relationships          []RPRelationship   `json:"relationships"`
	SalientMemories        []RPLifeMemory     `json:"salient_memories"`
	RecentWork             []RPLifeMemory     `json:"recent_work"`
	Needs                  []RPNeed           `json:"needs"`
	Goals                  []RPGoal           `json:"goals"`
	Commitments            []RPSocialEvidence `json:"commitments"`
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
	if input.NextSchedule != nil && input.NextSchedule.ActivityCode == "work" {
		sources := []string{}
		if input.NextSchedule.SourceEventID != "" {
			sources = append(sources, input.NextSchedule.SourceEventID)
		}
		add("work_commitment", "high", "keep_work_schedule", sources, "extended_socializing")
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
