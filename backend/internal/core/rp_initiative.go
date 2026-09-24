package core

import (
	"strings"
	"time"
)

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
	if RPContactOpportunitySuppressed(input) {
		return quiet, nil
	}
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
	if home := RPWeatherHomeDestination(input); home != "" {
		return RPDecisionProposal{Action: "leave", DestinationPlaceID: home}, nil
	}
	if rpInitiativeWorkIsImminent(input) {
		return quiet, nil
	}
	if destination := RPSelectedVisitDestination(input); destination != "" {
		return RPDecisionProposal{Action: "leave", DestinationPlaceID: destination}, nil
	}
	if RPSelectedStoreShortage(input) && input.Life.Disposition.Sociability > 0 {
		return RPDecisionProposal{Action: "respond", Text: "这家店有东西缺货了。"}, nil
	}
	if RPSelectedWorkChange(input) && input.Life.Disposition.Sociability > 0 {
		return RPDecisionProposal{Action: "respond", Text: "工作上有些变化，我还得安排一下。"}, nil
	}
	if RPSelectedCommunityChange(input) && input.Life.Disposition.Sociability > 0 {
		return RPDecisionProposal{Action: "respond", Text: "这里的规矩有了变化，得重新留意一下。"}, nil
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

func rpInitiativeWorkIsImminent(input RPDecisionInput) bool {
	if input.ActivityCode == "work" {
		return true
	}
	if input.NextSchedule == nil || input.NextSchedule.ActivityCode != "work" {
		return false
	}
	at, err := time.Parse(time.RFC3339, input.WorldTime)
	if err != nil {
		return true
	}
	due, err := time.Parse(time.RFC3339, input.NextSchedule.WorldTime)
	return err != nil || !due.After(at.Add(time.Hour))
}
