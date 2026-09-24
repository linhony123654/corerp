package core

import "time"

// Local observed condition, not access to source configuration or distant
// weather. SourceEventID identifies the wait that first accepted the condition.
type RPLocalEnvironment struct {
	PlaceID            string `json:"place_id"`
	SourceEventID      string `json:"source_event_id"`
	Condition          string `json:"condition"`
	EffectiveWorldTime string `json:"effective_world_time"`
	UntilWorldTime     string `json:"until_world_time"`
}

// A cautious actor may head to their known home in rain. This does not change
// physical reachability, cancel appointments, or force every personality home.
func RPWeatherHomeDestination(input RPDecisionInput) string {
	if input.Environment == nil || input.Environment.Condition != "rain" || input.Environment.PlaceID != input.PlaceID || input.Environment.SourceEventID == "" || input.Life == nil || input.Life.Background == nil || input.Life.Disposition.Caution <= 0 || input.Life.Disposition.SourceEventID == "" || input.ActivityCode == "work" {
		return ""
	}
	background := input.Life.Background
	if background.EntityID != input.NPCEntityID || background.ResidenceSourceEventID == "" || background.ResidencePlaceID == "" || background.ResidencePlaceID == input.PlaceID {
		return ""
	}
	at, err := time.Parse(time.RFC3339, input.WorldTime)
	if err != nil {
		return ""
	}
	start, err := time.Parse(time.RFC3339, input.Environment.EffectiveWorldTime)
	if err != nil || at.Before(start) {
		return ""
	}
	end, err := time.Parse(time.RFC3339, input.Environment.UntilWorldTime)
	if err != nil || !at.Before(end) {
		return ""
	}
	if input.NextSchedule != nil && input.NextSchedule.ActivityCode == "work" {
		due, err := time.Parse(time.RFC3339, input.NextSchedule.WorldTime)
		if err != nil || !due.After(at.Add(time.Hour)) {
			return ""
		}
	}
	canLeave := false
	for _, action := range input.LegalActions {
		canLeave = canLeave || action == "leave"
	}
	if !canLeave {
		return ""
	}
	for _, place := range input.ReachablePlaceIDs {
		if place == background.ResidencePlaceID {
			return place
		}
	}
	return ""
}
