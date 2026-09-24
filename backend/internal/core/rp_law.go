package core

import (
	"sort"
	"time"
)

// This is the actor's sourced knowledge, not a claim to know all current law.
type RPKnownLaw struct {
	PreviousEnactmentEventID string   `json:"previous_enactment_event_id,omitempty"`
	Repealed                 bool     `json:"repealed,omitempty"`
	InstitutionID            string   `json:"institution_id"`
	LawID                    string   `json:"law_id"`
	EnactmentEventID         string   `json:"enactment_event_id"`
	KnowledgeEventID         string   `json:"knowledge_event_id"`
	ScopeKind                string   `json:"scope_kind"`
	ScopeID                  string   `json:"scope_id"`
	PlaceIDs                 []string `json:"place_ids,omitempty"`
	EffectiveWorldTime       string   `json:"effective_world_time"`
	ProhibitedAction         string   `json:"prohibited_action"`
	FineMinor                int64    `json:"fine_minor"`
	Text                     string   `json:"text"`
}
type RPLawContext struct {
	KnownLaws     []RPKnownLaw `json:"known_laws"`
	LawfulActions []string     `json:"lawful_actions"`
}

type RPLawCase struct {
	InstitutionID      string `json:"institution_id"`
	EnforcementEventID string `json:"enforcement_event_id"`
	ViolationEventID   string `json:"violation_event_id"`
	EnactmentEventID   string `json:"enactment_event_id"`
	FineMinor          int64  `json:"fine_minor"`
	RefundedMinor      int64  `json:"refunded_minor"`
	DisputeEventID     string `json:"dispute_event_id,omitempty"`
	ReviewEventID      string `json:"review_event_id,omitempty"`
	Status             string `json:"status"`
}

func RPLawApplies(law RPKnownLaw, worldTime, place string) bool {
	if law.Repealed {
		return false
	}
	now, err := time.Parse(time.RFC3339, worldTime)
	if err != nil {
		return false
	}
	effective, err := time.Parse(time.RFC3339, law.EffectiveWorldTime)
	if err != nil || now.Before(effective) {
		return false
	}
	if law.ScopeKind == "world" {
		return true
	}
	if law.ScopeKind == "region" {
		for _, id := range law.PlaceIDs {
			if id == place {
				return true
			}
		}
	}
	return false
}

// Select only from the supplied evidence: callers must not mix authoritative
// law history into an actor's incomplete knowledge. Repeals supersede a version
// before being filtered out; future versions do not supersede anything yet.
func EffectiveRPLaws(versions []RPKnownLaw, worldTime, place string) []RPKnownLaw {
	now, err := time.Parse(time.RFC3339, worldTime)
	if err != nil {
		return nil
	}
	type key struct{ institution, law string }
	latest := map[key]RPKnownLaw{}
	for _, law := range versions {
		at, err := time.Parse(time.RFC3339, law.EffectiveWorldTime)
		if err != nil || at.After(now) {
			continue
		}
		k := key{law.InstitutionID, law.LawID}
		old, exists := latest[k]
		oldAt, _ := time.Parse(time.RFC3339, old.EffectiveWorldTime)
		if !exists || at.After(oldAt) {
			latest[k] = law
		}
	}
	var result []RPKnownLaw
	for _, law := range latest {
		if RPLawApplies(law, worldTime, place) {
			result = append(result, law)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].EnactmentEventID < result[j].EnactmentEventID })
	return result
}

func BuildRPLawContext(known []RPKnownLaw, worldTime, place string, executable []string) *RPLawContext {
	if len(known) == 0 {
		return nil
	}
	result := &RPLawContext{KnownLaws: known, LawfulActions: []string{}}
	effective := EffectiveRPLaws(known, worldTime, place)
	for _, action := range executable {
		allowed := true
		for _, law := range effective {
			if RPLawApplies(law, worldTime, place) && law.ProhibitedAction == "speak" && (action == "respond" || action == "refuse") {
				allowed = false
			}
		}
		if allowed {
			result.LawfulActions = append(result.LawfulActions, action)
		}
	}
	return result
}
