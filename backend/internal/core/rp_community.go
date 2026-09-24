package core

import (
	"reflect"
	"sort"
	"time"
)

type RPCommunityOpportunityContext struct {
	Law      RPKnownLaw `json:"law"`
	Selected bool       `json:"selected"`
}

func RPSelectedCommunityChange(input RPDecisionInput) bool {
	if input.CommunityOpportunity == nil || !input.CommunityOpportunity.Selected || input.Law == nil || input.Life == nil || input.CommunityOpportunity.Law.KnowledgeEventID == "" {
		return false
	}
	for _, law := range CurrentKnownRPCommunityChanges(input.Law.KnownLaws, input.WorldTime, input.PlaceID) {
		// Runtime verifies first-hearing provenance. The own context may point
		// to a later repeat announcement of this otherwise identical version.
		law.KnowledgeEventID = input.CommunityOpportunity.Law.KnowledgeEventID
		if reflect.DeepEqual(law, input.CommunityOpportunity.Law) {
			return true
		}
	}
	return false
}

// CurrentKnownRPCommunityChanges selects from an actor's supplied knowledge,
// never authoritative global law. Unlike EffectiveRPLaws, a repeal is itself
// an eligible change; it must not disappear merely because it ends a rule.
func CurrentKnownRPCommunityChanges(known []RPKnownLaw, at, place string) []RPKnownLaw {
	now, err := time.Parse(time.RFC3339, at)
	if err != nil {
		return nil
	}
	type key struct{ institution, law string }
	latest := map[key]RPKnownLaw{}
	for _, law := range known {
		when, err := time.Parse(time.RFC3339, law.EffectiveWorldTime)
		if err != nil || when.After(now) || law.EnactmentEventID == "" || law.KnowledgeEventID == "" {
			continue
		}
		k := key{law.InstitutionID, law.LawID}
		old, found := latest[k]
		oldAt, _ := time.Parse(time.RFC3339, old.EffectiveWorldTime)
		if !found || when.After(oldAt) || when.Equal(oldAt) && law.EnactmentEventID < old.EnactmentEventID {
			latest[k] = law
		}
	}
	var result []RPKnownLaw
	for _, law := range latest {
		if law.PreviousEnactmentEventID == "" || law.ScopeKind != "region" {
			continue
		}
		for _, id := range law.PlaceIDs {
			if id == place {
				// Return an owned slice, not a mutable alias into caller beliefs.
				law.PlaceIDs = append([]string(nil), law.PlaceIDs...)
				result = append(result, law)
				break
			}
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].EnactmentEventID < result[j].EnactmentEventID })
	return result
}
