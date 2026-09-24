package core

import "time"

const RPWarmDecisionLimit = 4
const RPWarmDecisionInterval = 6 * time.Hour

// WARM rules receive only own appointment and already-validated legal routes.
// No player location, remote scene, finances, dialogue, or model is required.
// This proposal is not a movement or permission to begin earning wages.
type RPWarmDecisionInput struct {
	WorkPath            []string            `json:"work_path,omitempty"`
	RouteSourceEventIDs []string            `json:"route_source_event_ids,omitempty"`
	ActorID             string              `json:"actor_id"`
	WorldTime           string              `json:"world_time"`
	PlaceID             string              `json:"place_id"`
	ActivityCode        string              `json:"activity_code"`
	NextSchedule        *RPDecisionSchedule `json:"next_schedule,omitempty"`
	ReachablePlaceIDs   []string            `json:"reachable_place_ids"`
}

type RPWarmDecision struct {
	ActorID               string `json:"actor_id"`
	WorldTime             string `json:"world_time"`
	Action                string `json:"action"`
	Reason                string `json:"reason"`
	FromPlaceID           string `json:"from_place_id"`
	ToPlaceID             string `json:"to_place_id,omitempty"`
	ScheduleSourceEventID string `json:"schedule_source_event_id,omitempty"`
	DelaySourceEventID    string `json:"delay_source_event_id,omitempty"`
}

func ProposeRPWarmDecision(input RPWarmDecisionInput) (RPWarmDecision, error) {
	var empty RPWarmDecision
	if input.ActorID == "" || input.PlaceID == "" {
		return empty, NewError(CodeInvalidArgument, "WARM decision requires own actor and place")
	}
	at, err := time.Parse(time.RFC3339, input.WorldTime)
	if err != nil {
		return empty, NewError(CodeInvalidArgument, "WARM decision requires world time")
	}
	decision := RPWarmDecision{ActorID: input.ActorID, WorldTime: input.WorldTime, Action: "wait", Reason: "ordinary_routine", FromPlaceID: input.PlaceID}
	if input.ActivityCode == "work" {
		decision.Reason = "already_working"
		return decision, nil
	}
	next := input.NextSchedule
	if next == nil || next.ActivityCode != "work" {
		return decision, nil
	}
	if next.SourceEventID == "" || next.PlaceID == "" {
		return empty, NewError(CodeInvalidArgument, "WARM work appointment requires provenance")
	}
	due, err := time.Parse(time.RFC3339, next.WorldTime)
	if err != nil {
		return empty, NewError(CodeInvalidArgument, "WARM work appointment has invalid time")
	}
	if next.OriginalWorldTime != "" {
		original, err := time.Parse(time.RFC3339, next.OriginalWorldTime)
		if err != nil || next.DelaySourceEventID == "" || original.After(due) {
			return empty, NewError(CodeInvalidArgument, "WARM delayed appointment requires original time and delay source")
		}
	}
	// Due/past appointments belong to the authoritative scheduler. This rule
	// neither executes them nor fabricates missed work when a clock advances.
	if !due.After(at) || due.After(at.Add(time.Hour)) {
		return decision, nil
	}
	decision.ScheduleSourceEventID = next.SourceEventID
	decision.DelaySourceEventID = next.DelaySourceEventID
	if next.PlaceID == input.PlaceID {
		decision.Reason = "already_at_workplace"
		return decision, nil
	}
	decision.Reason = "no_immediate_route"
	destination := next.PlaceID
	if len(input.WorkPath) > 0 {
		if len(input.WorkPath) < 2 || input.WorkPath[0] != input.PlaceID || input.WorkPath[len(input.WorkPath)-1] != next.PlaceID || len(input.RouteSourceEventIDs) != len(input.WorkPath)-1 {
			return empty, NewError(CodeInvalidArgument, "WARM route must lead from own place to own appointment")
		}
		for _, source := range input.RouteSourceEventIDs {
			if source == "" {
				return empty, NewError(CodeInvalidArgument, "WARM route lacks provenance")
			}
		}
		destination = input.WorkPath[1]
	}
	for _, place := range input.ReachablePlaceIDs {
		if place == destination {
			decision.Action = "leave"
			decision.Reason = "prepare_for_own_work"
			decision.ToPlaceID = place
			break
		}
	}
	return decision, nil
}
