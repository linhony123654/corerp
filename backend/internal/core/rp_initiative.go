package core

import "strings"

// A time-triggered choice does not imply any player speech or hearing evidence.
type RPDecisionTrigger struct {
	Kind          string `json:"kind"`
	SourceEventID string `json:"source_event_id"`
}

type RPInitiativeRequest struct {
	PrincipalID    string `json:"principal_id"`
	SessionID      string `json:"session_id"`
	NPCEntityID    string `json:"npc_entity_id"`
	TriggerEventID string `json:"trigger_event_id"`
}

func (r RPInitiativeRequest) Validate() error {
	for _, v := range []string{r.PrincipalID, r.SessionID, r.NPCEntityID, r.TriggerEventID} {
		if strings.TrimSpace(v) == "" || len(v) > 256 {
			return NewError(CodeInvalidArgument, "initiative requires bounded principal, session, NPC and trigger IDs")
		}
	}
	return nil
}

func proposeRPInitiative(input RPDecisionInput) (RPDecisionProposal, error) {
	if input.Trigger.Kind != "elapsed_time" || input.Trigger.SourceEventID == "" || input.SpeechEventID != "" || input.PlayerSpeechText != "" || input.TurnID != "" {
		return RPDecisionProposal{}, NewError(CodeInvalidArgument, "initiative cannot pretend a player speech occurred")
	}
	quiet := RPDecisionProposal{Action: "silence"}
	if input.Life == nil {
		return quiet, nil
	}
	// Only actual sourced needs or observed relationships motivate contact.
	// Cadence and authoritative eligibility are checked separately by runtime.
	for _, g := range input.Life.Goals {
		if len(g.SourceEventIDs) == 0 {
			continue
		}
		if g.Code == "avoid_conflict" && g.SubjectEntityID == input.InterlocutorEntityID && len(input.ReachablePlaceIDs) > 0 {
			return RPDecisionProposal{Action: "leave", DestinationPlaceID: input.ReachablePlaceIDs[0]}, nil
		}
		if g.Code == "stabilize_income" || g.Code == "collect_money_owed" {
			return RPDecisionProposal{Action: "respond", Text: "我得先处理一下手头的开销。"}, nil
		}
	}
	if input.ActivityCode == "work" || input.NextSchedule != nil && input.NextSchedule.ActivityCode == "work" {
		return quiet, nil
	}
	if input.Life.Disposition.Sociability > 0 {
		for _, r := range input.Life.Relationships {
			if r.SubjectEntityID == input.InterlocutorEntityID && r.Trust >= 2 && r.Tension == 0 && len(r.SourceEventIDs) > 0 {
				return RPDecisionProposal{Action: "respond", Text: "又见面了，最近过得怎么样？"}, nil
			}
		}
	}
	return quiet, nil
}
