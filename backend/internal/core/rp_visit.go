package core

import "time"

const RPOldFriendVisitMinimumGap = 7 * 24 * time.Hour

type RPVisitOpportunityContext struct {
	Source   RPVisitSource `json:"source"`
	Selected bool          `json:"selected"`
}

func RPSelectedVisitDestination(input RPDecisionInput) string {
	if input.VisitOpportunity == nil || !input.VisitOpportunity.Selected || !RPVisitSourceEligible(input, input.VisitOpportunity.Source) || input.Life.Disposition.Sociability == 0 || rpInitiativeWorkIsImminent(input) {
		return ""
	}
	if input.Law != nil {
		lawful := false
		for _, action := range input.Law.LawfulActions {
			lawful = lawful || action == "leave"
		}
		if !lawful {
			return ""
		}
	}
	for _, place := range input.ReachablePlaceIDs {
		if place == input.VisitOpportunity.Source.PlaceID {
			return place
		}
	}
	return ""
}

// A remembered destination is not a claim that anybody is still there.
// Runtime must verify these IDs against own movement/observation evidence.
type RPVisitSource struct {
	ActorID                   string `json:"actor_id"`
	Kind                      string `json:"kind"`
	PlaceID                   string `json:"place_id"`
	PlaceSourceEventID        string `json:"place_source_event_id"`
	MemorySourceEventID       string `json:"memory_source_event_id"`
	RememberedWorldTime       string `json:"remembered_world_time"`
	ObservationID             string `json:"observation_id,omitempty"`
	FriendID                  string `json:"friend_id,omitempty"`
	RelationshipSourceEventID string `json:"relationship_source_event_id,omitempty"`
}

func RPVisitSourceEligible(input RPDecisionInput, source RPVisitSource) bool {
	if input.Life == nil || source.ActorID != input.NPCEntityID || source.PlaceID == "" || source.PlaceID == input.PlaceID || source.PlaceSourceEventID == "" || source.MemorySourceEventID == "" {
		return false
	}
	at, err := time.Parse(time.RFC3339, input.WorldTime)
	if err != nil {
		return false
	}
	remembered, err := time.Parse(time.RFC3339, source.RememberedWorldTime)
	if err != nil || remembered.After(at) {
		return false
	}
	switch source.Kind {
	case "familiar_public_place":
		return source.FriendID == "" && source.RelationshipSourceEventID == "" && source.ObservationID == ""
	case "old_friend_place":
		if source.FriendID == "" || source.FriendID == input.NPCEntityID || source.ObservationID == "" || source.RelationshipSourceEventID == "" || at.Sub(remembered) < RPOldFriendVisitMinimumGap {
			return false
		}
		for _, relation := range input.Life.Relationships {
			if relation.SubjectEntityID != source.FriendID || relation.Trust < 2 || relation.Tension != 0 {
				continue
			}
			for _, event := range relation.SourceEventIDs {
				if event == source.RelationshipSourceEventID {
					return true
				}
			}
		}
	}
	return false
}
