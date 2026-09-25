package core

import (
	"context"
	"strings"
	"time"
)

type RPDecisionRequest struct {
	PrincipalID string `json:"principal_id"`
	SessionID   string `json:"session_id"`
	TurnID      string `json:"turn_id"`
	NPCEntityID string `json:"npc_entity_id"`
}

func (r RPDecisionRequest) Validate() error {
	if strings.TrimSpace(r.PrincipalID) == "" || strings.TrimSpace(r.SessionID) == "" || strings.TrimSpace(r.TurnID) == "" || strings.TrimSpace(r.NPCEntityID) == "" {
		return NewError(CodeInvalidArgument, "principal_id, session_id, turn_id and npc_entity_id are required")
	}
	return nil
}

type RPDecisionVisibleEntity struct {
	EntityID    string `json:"entity_id"`
	DisplayName string `json:"display_name"`
}

type RPDecisionKnowledge struct {
	ClaimType       string `json:"claim_type"`
	SubjectEntityID string `json:"subject_entity_id"`
	PlaceID         string `json:"place_id,omitempty"`
	Text            string `json:"text,omitempty"`
	SourceEventID   string `json:"source_event_id"`
}

type RPDecisionSchedule struct {
	OriginalWorldTime  string `json:"original_world_time,omitempty"`
	DelaySourceEventID string `json:"delay_source_event_id,omitempty"`
	SourceEventID      string `json:"source_event_id,omitempty"`
	WorldTime          string `json:"world_time"`
	PlaceID            string `json:"place_id"`
	ActivityCode       string `json:"activity_code"`
}

// RPDecisionInput is the only data boundary exposed to a replaceable provider.
// No account identifiers, other people's finances, creator data or raw DB rows.
type RPDecisionInput struct {
	VisitOpportunity     *RPVisitOpportunityContext     `json:"visit_opportunity,omitempty"`
	CommunityOpportunity *RPCommunityOpportunityContext `json:"community_opportunity,omitempty"`
	WorkOpportunity      *RPWorkOpportunityContext      `json:"work_opportunity,omitempty"`
	Environment          *RPLocalEnvironment            `json:"environment,omitempty"`
	Stores               []RPStoreAvailability          `json:"stores,omitempty"`
	StoreOpportunities   []RPStoreOpportunityContext    `json:"store_opportunities,omitempty"`
	TransitWorks         []RPLocalTransitWorks          `json:"transit_works,omitempty"`
	ContactOpportunity   *RPContactOpportunityContext   `json:"contact_opportunity,omitempty"`
	Law                  *RPLawContext                  `json:"law,omitempty"`
	Trigger              *RPDecisionTrigger             `json:"trigger,omitempty"`
	Life                 *RPLifeContext                 `json:"life,omitempty"`
	InstanceID           string                         `json:"instance_id"`
	BranchID             string                         `json:"branch_id"`
	HeadSequence         int64                          `json:"head_sequence"`
	TurnID               string                         `json:"turn_id"`
	SpeechEventID        string                         `json:"speech_event_id"`
	NPCEntityID          string                         `json:"npc_entity_id"`
	InterlocutorEntityID string                         `json:"interlocutor_entity_id"`
	NPCName              string                         `json:"npc_name"`
	WorldTime            string                         `json:"world_time"`
	PlaceID              string                         `json:"place_id"`
	PlaceName            string                         `json:"place_name"`
	ActivityCode         string                         `json:"activity_code"`
	GoalCode             string                         `json:"goal_code"`
	OwnAssetMinor        int64                          `json:"own_asset_minor"`
	CurrencyID           string                         `json:"currency_id"`
	VisibleEntities      []RPDecisionVisibleEntity      `json:"visible_entities"`
	Knowledge            []RPDecisionKnowledge          `json:"knowledge"`
	NextSchedule         *RPDecisionSchedule            `json:"next_schedule,omitempty"`
	PlayerSpeechText     string                         `json:"player_speech_text"`
	LegalActions         []string                       `json:"legal_actions"`
	ReachablePlaceIDs    []string                       `json:"reachable_place_ids"`
}

type RPDecisionProposal struct {
	Action             string `json:"action"`
	Text               string `json:"text,omitempty"`
	DestinationPlaceID string `json:"destination_place_id,omitempty"`
}

type RPDecisionProvider interface {
	Propose(context.Context, RPDecisionInput) (RPDecisionProposal, error)
}

// DeterministicRPDecisionProvider is the local executable baseline, not a
// character script or a source of world authority.
type DeterministicRPDecisionProvider struct{}

func (DeterministicRPDecisionProvider) Propose(_ context.Context, input RPDecisionInput) (RPDecisionProposal, error) {
	if input.Law != nil {
		respond, silence := false, false
		for _, action := range input.Law.LawfulActions {
			if action == "respond" {
				respond = true
			}
			if action == "silence" {
				silence = true
			}
		}
		if !respond && silence {
			return RPDecisionProposal{Action: "silence"}, nil
		}
	}
	if input.Trigger != nil {
		return proposeRPInitiative(input)
	}
	if input.Life != nil {
		for _, goal := range input.Life.Goals {
			switch goal.Code {
			case "collect_money_owed", "stabilize_income":
				return RPDecisionProposal{Action: "refuse", Text: "我得先处理手头的开销，暂时没心思闲聊。"}, nil
			case "stabilize_household_income":
				return RPDecisionProposal{Action: "refuse", Text: "我得先想办法补上家里的房租，暂时不能闲聊。"}, nil
			case "prioritize_rest":
				at, err := time.Parse(time.RFC3339, input.WorldTime)
				if err == nil && at.Hour() >= 18 && (strings.Contains(input.PlayerSpeechText, "一起") || strings.Contains(input.PlayerSpeechText, "约")) {
					return RPDecisionProposal{Action: "refuse", Text: "我今晚需要休息，改天再约。"}, nil
				}
			case "avoid_conflict":
				if goal.SubjectEntityID != input.InterlocutorEntityID {
					continue
				}
				if len(input.ReachablePlaceIDs) > 0 {
					return RPDecisionProposal{Action: "leave", DestinationPlaceID: input.ReachablePlaceIDs[0]}, nil
				}
				return RPDecisionProposal{Action: "silence"}, nil
			case "honor_commitment":
				for _, promise := range input.Life.Commitments {
					if promise.ActorEntityID != input.NPCEntityID {
						continue
					}
					at, err := time.Parse(time.RFC3339, promise.MeetingWorldTime)
					now, timeErr := time.Parse(time.RFC3339, input.WorldTime)
					if err != nil || timeErr != nil || now.Before(at.Add(-30*time.Minute)) || now.After(at.Add(time.Hour)) {
						continue
					}
					if input.PlaceID == promise.MeetingPlaceID {
						return RPDecisionProposal{Action: "wait"}, nil
					}
					for _, destination := range input.ReachablePlaceIDs {
						if destination == promise.MeetingPlaceID {
							return RPDecisionProposal{Action: "leave", DestinationPlaceID: destination}, nil
						}
					}
				}
			}
		}
	}
	if strings.Contains(input.PlayerSpeechText, "借") || input.OwnAssetMinor < 100 {
		return RPDecisionProposal{Action: "refuse", Text: "抱歉，我现在无法答应。"}, nil
	}
	if input.NextSchedule != nil && input.NextSchedule.ActivityCode == "work" {
		return RPDecisionProposal{Action: "refuse", Text: "我得先去工作，晚些再聊。"}, nil
	}
	if input.Life != nil {
		for i := len(input.Life.CultureExperiences) - 1; i >= 0; i-- {
			experience := input.Life.CultureExperiences[i]
			if experience.ActorEntityID != input.InterlocutorEntityID {
				continue
			}
			score := 0
			for _, evaluation := range experience.Evaluations {
				score += evaluation.Score
			}
			if score < 0 {
				return RPDecisionProposal{Action: "refuse", Text: "先前的赠礼让我不太舒服，我暂时不想继续聊。"}, nil
			}
			break
		}
		for _, relation := range input.Life.Relationships {
			if relation.SubjectEntityID == input.InterlocutorEntityID && relation.Trust >= 2 {
				return RPDecisionProposal{Action: "respond", Text: "我愿意相信你，接着说吧。"}, nil
			}
			if relation.SubjectEntityID == input.InterlocutorEntityID && relation.Familiarity >= 3 {
				return RPDecisionProposal{Action: "respond", Text: "又见面了。最近过得怎么样？"}, nil
			}
		}
	}
	return RPDecisionProposal{Action: "respond", Text: "你好。"}, nil
}
