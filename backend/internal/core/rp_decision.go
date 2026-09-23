package core

import (
	"context"
	"strings"
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
	WorldTime    string `json:"world_time"`
	PlaceID      string `json:"place_id"`
	ActivityCode string `json:"activity_code"`
}

// RPDecisionInput is the only data boundary exposed to a replaceable provider.
// No account identifiers, other people's finances, creator data or raw DB rows.
type RPDecisionInput struct {
	InstanceID        string                    `json:"instance_id"`
	BranchID          string                    `json:"branch_id"`
	HeadSequence      int64                     `json:"head_sequence"`
	TurnID            string                    `json:"turn_id"`
	SpeechEventID     string                    `json:"speech_event_id"`
	NPCEntityID       string                    `json:"npc_entity_id"`
	NPCName           string                    `json:"npc_name"`
	WorldTime         string                    `json:"world_time"`
	PlaceID           string                    `json:"place_id"`
	PlaceName         string                    `json:"place_name"`
	ActivityCode      string                    `json:"activity_code"`
	GoalCode          string                    `json:"goal_code"`
	OwnAssetMinor     int64                     `json:"own_asset_minor"`
	CurrencyID        string                    `json:"currency_id"`
	VisibleEntities   []RPDecisionVisibleEntity `json:"visible_entities"`
	Knowledge         []RPDecisionKnowledge     `json:"knowledge"`
	NextSchedule      *RPDecisionSchedule       `json:"next_schedule,omitempty"`
	PlayerSpeechText  string                    `json:"player_speech_text"`
	LegalActions      []string                  `json:"legal_actions"`
	ReachablePlaceIDs []string                  `json:"reachable_place_ids"`
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
	if strings.Contains(input.PlayerSpeechText, "借") || input.OwnAssetMinor < 100 {
		return RPDecisionProposal{Action: "refuse", Text: "抱歉，我现在无法答应。"}, nil
	}
	if input.NextSchedule != nil && input.NextSchedule.ActivityCode == "work" {
		return RPDecisionProposal{Action: "refuse", Text: "我得先去工作，晚些再聊。"}, nil
	}
	return RPDecisionProposal{Action: "respond", Text: "你好。"}, nil
}
