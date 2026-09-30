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

const RPContextVersion = "corerp.rp-context.v1"

type RPContextReadiness struct {
	Persona                    string `json:"persona"`                      // READY or MISSING
	RelationshipToInterlocutor string `json:"relationship_to_interlocutor"` // READY or UNKNOWN
	AddressToInterlocutor      string `json:"address_to_interlocutor"`      // READY, MISSING or UNKNOWN
}

// Incomplete distinguishes a missing required author declaration from an
// intentionally unknown relationship. Unknown strangers may still interact.
func (r RPContextReadiness) Incomplete() bool {
	return r.Persona == "MISSING" || r.AddressToInterlocutor == "MISSING"
}

// RPCharacterRelationship is a one-way, event-sourced creator declaration,
// not a model-inferred kinship or the dynamic social-affinity projection.
type RPCharacterRelationship struct {
	SubjectEntityID string   `json:"subject_entity_id"`
	Role            string   `json:"role"`
	AddressTo       []string `json:"address_to,omitempty"`
	SelfReference   string   `json:"self_reference,omitempty"`
	SourceEventID   string   `json:"source_event_id"`
}

type RPDecisionKnowledge struct {
	ClaimType       string `json:"claim_type"`
	SubjectEntityID string `json:"subject_entity_id"`
	PlaceID         string `json:"place_id,omitempty"`
	Text            string `json:"text,omitempty"`
	TextFromEvent   bool   `json:"text_from_event,omitempty"`
	SourceEventID   string `json:"source_event_id"`
}

// RPDecisionDialogue is an accepted utterance the NPC spoke or personally
// heard. EventID ties the text to immutable world history, never to a model
// summary or an uncommitted player claim.
type RPDecisionDialogue struct {
	SpeakerEntityID string `json:"speaker_entity_id"`
	Text            string `json:"text"`
	TextFromEvent   bool   `json:"text_from_event,omitempty"`
	EventID         string `json:"event_id"`
	WorldTime       string `json:"world_time"`
}

// RPDecisionSpeechExcerpt keeps an older personally heard player statement
// available after other co-located speakers crowd it out of RecentDialogue.
// Truncation is explicit so the excerpt cannot be mistaken for a full quote.
type RPDecisionSpeechExcerpt struct {
	Excerpt       string `json:"excerpt"`
	TextFromEvent bool   `json:"text_from_event,omitempty"`
	Truncated     bool   `json:"truncated,omitempty"`
	EventID       string `json:"event_id"`
	WorldTime     string `json:"world_time"`
}

// RPDecisionExchange is an older, topic-selected group of accepted utterances.
// Each utterance is either the NPC's own speech or personally heard speech.
// It proves what was said, not the truth of any claim or that a promise was kept.
type RPDecisionExchange struct {
	Dialogue []RPDecisionDialogue `json:"dialogue"`
}

// RPDecisionPrivateMemory is the actor's own earlier approved decision sketch.
// Its source Event anchors when the decision was applied, not the truth of the
// private thought or completion of a goal. No public consumer may read it.
type RPDecisionPrivateMemory struct {
	DecisionID           string            `json:"decision_id"`
	SourceEventID        string            `json:"source_event_id"`
	EventSequence        int64             `json:"event_sequence"`
	WorldTime            string            `json:"world_time"`
	InterlocutorEntityID string            `json:"interlocutor_entity_id"`
	Private              RPDecisionPrivate `json:"private"`
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
	ContextVersion         string                         `json:"context_version"`
	ContextSelection       *RPContextSelection            `json:"context_selection,omitempty"`
	Readiness              RPContextReadiness             `json:"readiness"`
	PersonaSourceEventID   string                         `json:"persona_source_event_id,omitempty"`
	Relationships          []RPCharacterRelationship      `json:"authored_relationships,omitempty"`
	VisitOpportunity       *RPVisitOpportunityContext     `json:"visit_opportunity,omitempty"`
	CommunityOpportunity   *RPCommunityOpportunityContext `json:"community_opportunity,omitempty"`
	WorkOpportunity        *RPWorkOpportunityContext      `json:"work_opportunity,omitempty"`
	Environment            *RPLocalEnvironment            `json:"environment,omitempty"`
	Stores                 []RPStoreAvailability          `json:"stores,omitempty"`
	StoreOpportunities     []RPStoreOpportunityContext    `json:"store_opportunities,omitempty"`
	TransitWorks           []RPLocalTransitWorks          `json:"transit_works,omitempty"`
	ContactOpportunity     *RPContactOpportunityContext   `json:"contact_opportunity,omitempty"`
	Law                    *RPLawContext                  `json:"law,omitempty"`
	Trigger                *RPDecisionTrigger             `json:"trigger,omitempty"`
	Life                   *RPLifeContext                 `json:"life,omitempty"`
	InstanceID             string                         `json:"instance_id"`
	BranchID               string                         `json:"branch_id"`
	HeadSequence           int64                          `json:"head_sequence"`
	TurnID                 string                         `json:"turn_id"`
	SpeechEventID          string                         `json:"speech_event_id"`
	NPCEntityID            string                         `json:"npc_entity_id"`
	InterlocutorEntityID   string                         `json:"interlocutor_entity_id"`
	NPCName                string                         `json:"npc_name"`
	WorldTime              string                         `json:"world_time"`
	PlaceID                string                         `json:"place_id"`
	PlaceName              string                         `json:"place_name"`
	ActivityCode           string                         `json:"activity_code"`
	GoalCode               string                         `json:"goal_code"`
	Persona                string                         `json:"persona,omitempty"`
	OwnAssetMinor          int64                          `json:"own_asset_minor"`
	CurrencyID             string                         `json:"currency_id"`
	VisibleEntities        []RPDecisionVisibleEntity      `json:"visible_entities"`
	Knowledge              []RPDecisionKnowledge          `json:"knowledge"`
	RecentDialogue         []RPDecisionDialogue           `json:"recent_dialogue,omitempty"`
	HeardPlayerHistory     []RPDecisionSpeechExcerpt      `json:"heard_player_history,omitempty"`
	RelevantDialogue       []RPDecisionExchange           `json:"relevant_dialogue,omitempty"`
	RecentPrivateDecisions []RPDecisionPrivateMemory      `json:"recent_private_decisions,omitempty"`
	NextSchedule           *RPDecisionSchedule            `json:"next_schedule,omitempty"`
	PlayerSpeechText       string                         `json:"player_speech_text"`
	PlayerSpeechWorldTime  string                         `json:"player_speech_world_time,omitempty"`
	LegalActions           []string                       `json:"legal_actions"`
	LegalActivities        []string                       `json:"legal_activities,omitempty"`
	ReachablePlaceIDs      []string                       `json:"reachable_place_ids"`
	OwnActions             []RPOwnAction                  `json:"own_actions,omitempty"`
	SceneActivities        []RPSceneActivity              `json:"scene_activities,omitempty"`
}

type RPOwnAction struct {
	EventID       string `json:"event_id"`
	Action        string `json:"action"`
	Text          string `json:"text,omitempty"`
	TextFromEvent bool   `json:"text_from_event,omitempty"`
	ActivityCode  string `json:"activity_code,omitempty"`
	PlaceID       string `json:"place_id"`
	WorldTime     string `json:"world_time"`
	// Status distinguishes started/completed/cancelled activity events.
	Status string `json:"status,omitempty"`
}

// RPSceneActivity is an in-progress or recently-ended activity observed at
// the observer's current place: the "who is doing what right now" channel.
type RPSceneActivity struct {
	ActivityID   string `json:"activity_id"`
	ActorID      string `json:"actor_id"`
	ActivityCode string `json:"activity_code"`
	Status       string `json:"status"`
	WorldTime    string `json:"world_time"`
}

type RPDecisionProposal struct {
	Action             string `json:"action"`
	Text               string `json:"text,omitempty"`
	DestinationPlaceID string `json:"destination_place_id,omitempty"`
	// ActivityCode carries the declared activity for act proposals; duration
	// and completion are rule-bound, never model-declared.
	ActivityCode string `json:"activity_code,omitempty"`
	// IntroduceSelf marks speech that reveals the speaker's identity to
	// listeners. Text alone never establishes recognition; this flag does.
	IntroduceSelf bool `json:"introduce_self,omitempty"`
	// Private is model decision metadata, never an Event payload or narrator
	// input. Restricted audits and immutable applied-decision rows retain it;
	// later decisions may read only this actor's own approved sketches.
	Private *RPDecisionPrivate `json:"private,omitempty"`
	// ExpressionCode is a bounded, proposed observable. The decision owner
	// must commit a witnessed nonverbal Event before it is publicly narrated.
	ExpressionCode string `json:"expression_code,omitempty"`
}

type RPDecisionPrivate struct {
	Intent             string   `json:"intent"`
	Emotion            string   `json:"emotion"`
	RelationshipStance string   `json:"relationship_stance"`
	BasisEventIDs      []string `json:"basis_event_ids"`
}

type RPDecisionProvider interface {
	Propose(context.Context, RPDecisionInput) (RPDecisionProposal, error)
}

// DeterministicRPDecisionProvider is the local executable baseline, not a
// character script or a source of world authority.
type DeterministicRPDecisionProvider struct{}

func (DeterministicRPDecisionProvider) ProviderMetadata() RPProviderMetadata {
	return RPProviderMetadata{Kind: "deterministic"}
}

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
