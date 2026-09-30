package core

import (
	"context"
	"strings"
)

type RPInteractionRequest struct {
	PrincipalID      string `json:"principal_id"`
	SessionID        string `json:"session_id"`
	Text             string `json:"text"`
	Mode             string `json:"interaction_mode,omitempty"`
	NarrativeDensity string `json:"narrative_density,omitempty"`
	ExpectedCursor   int64  `json:"expected_cursor"`
	IdempotencyKey   string `json:"idempotency_key"`
}

func (r RPInteractionRequest) Validate() error {
	if strings.TrimSpace(r.PrincipalID) == "" || strings.TrimSpace(r.SessionID) == "" || strings.TrimSpace(r.IdempotencyKey) == "" || len(r.IdempotencyKey) > 128 || r.ExpectedCursor < 1 {
		return NewError(CodeInvalidArgument, "interaction requires principal, session, current cursor and bounded key")
	}
	if strings.TrimSpace(r.Text) == "" || len([]rune(r.Text)) > 2000 {
		return NewError(CodeInvalidArgument, "interaction text must have 1–2000 characters")
	}
	if r.Mode != "" && r.Mode != "AUTO" && r.Mode != "DIALOGUE" && r.Mode != "SCENE" {
		return NewError(CodeInvalidArgument, "interaction mode must be AUTO, DIALOGUE or SCENE")
	}
	if r.NarrativeDensity != "" && r.NarrativeDensity != "concise" && r.NarrativeDensity != "standard" && r.NarrativeDensity != "long" {
		return NewError(CodeInvalidArgument, "narrative density must be concise, standard or long")
	}
	return nil
}

// RPInteractionPlan is an application proposal, not a world fact. Persist the
// resolved plan under the original request key before executing any step.
type RPInteractionPlan struct {
	Mode          string              `json:"mode"`
	Kind          string              `json:"kind"`
	Steps         []RPInteractionStep `json:"steps"`
	Clarification string              `json:"clarification,omitempty"`
}

type RPInteractionStep struct {
	Kind            string `json:"kind"`
	TargetPlaceID   string `json:"target_place_id,omitempty"`
	TargetEntityID  string `json:"target_entity_id,omitempty"`
	WaitHours       int    `json:"wait_hours,omitempty"`
	WaitMinutes     int    `json:"wait_minutes,omitempty"`
	SpeechText      string `json:"speech_text,omitempty"`
	ObjectAction    string `json:"object_action,omitempty"`
	ObjectID        string `json:"object_id,omitempty"`
	AnchorID        string `json:"anchor_id,omitempty"`
	OfferID         string `json:"offer_id,omitempty"`
	NonverbalAction string `json:"nonverbal_action,omitempty"`
	GestureCode     string `json:"gesture_code,omitempty"`
}

type RPInteractionObject struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	PhysicalState  string   `json:"physical_state,omitempty"`
	AnchorID       string   `json:"anchor_id,omitempty"`
	AllowedActions []string `json:"allowed_actions"`
}

type RPInteractionAnchor struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type RPInteractionOffer struct {
	ID             string   `json:"id"`
	ObjectID       string   `json:"object_id"`
	AllowedActions []string `json:"allowed_actions"`
}

type RPInteractionEntity struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// An interpreter sees only the controlled player's authorized observation
// and proposes a bounded plan. It never owns world effects or private truth.
type RPInteractionUnderstandingInput struct {
	Text            string                `json:"text"`
	Mode            string                `json:"mode"`
	PlaceName       string                `json:"place_name"`
	WorldTime       string                `json:"world_time"`
	ReachablePlaces []RPInteractionPlace  `json:"reachable_places"`
	PresentEntities []RPInteractionEntity `json:"present_entities"`
	Objects         []RPInteractionObject `json:"objects"`
	Anchors         []RPInteractionAnchor `json:"anchors"`
	Offers          []RPInteractionOffer  `json:"offers"`
}

type RPInteractionUnderstandingProvider interface {
	UnderstandInteraction(context.Context, RPInteractionUnderstandingInput) (RPInteractionPlan, error)
}

type RPInteractionPlace struct {
	ID   string
	Name string
}

// ParseRPInteraction deliberately recognizes a small, closed action grammar.
// Unknown or ambiguous action-like input asks for clarification rather than
// inventing a destination or silently speaking the instruction aloud.
func ParseRPInteraction(input, mode string, places []RPInteractionPlace) (RPInteractionPlan, error) {
	input = strings.TrimSpace(input)
	if input == "" || len([]rune(input)) > 2000 {
		return RPInteractionPlan{}, NewError(CodeInvalidArgument, "interaction text must have 1–2000 characters")
	}
	if mode == "" {
		mode = "AUTO"
	}
	if mode != "AUTO" && mode != "DIALOGUE" && mode != "SCENE" {
		return RPInteractionPlan{}, NewError(CodeInvalidArgument, "interaction mode must be AUTO, DIALOGUE or SCENE")
	}
	plan := RPInteractionPlan{Mode: mode, Steps: []RPInteractionStep{}}
	if mode == "DIALOGUE" {
		plan.Kind = "DIALOGUE"
		plan.Steps = append(plan.Steps, RPInteractionStep{Kind: "speech", SpeechText: input})
		return plan, nil
	}
	clarify := func(message string) (RPInteractionPlan, error) {
		plan.Kind = "CLARIFICATION"
		plan.Clarification = message
		return plan, nil
	}
	if input == "继续" || input == "继续吧" {
		return clarify("请说明要继续说话、等待，还是前往一个已知地点。")
	}
	if (strings.HasPrefix(input, "去") && !strings.HasPrefix(input, "去年")) || strings.HasPrefix(input, "前往") {
		rest := strings.TrimSpace(strings.TrimPrefix(input, "去"))
		if strings.HasPrefix(input, "前往") {
			rest = strings.TrimSpace(strings.TrimPrefix(input, "前往"))
		}
		destination, speech, hasSpeech, ok := splitRPActionSpeech(rest)
		if !ok {
			return clarify("请用“去地点，随后说「话语」”表达行动和说话的先后顺序。")
		}
		matches := []string{}
		for _, place := range places {
			if destination == place.Name || destination == place.ID {
				matches = append(matches, place.ID)
			}
		}
		if len(matches) != 1 || matches[0] == "" {
			return clarify("目的地不在当前可见的唯一相邻地点中，请先查看场景并明确选择。")
		}
		plan.Steps = append(plan.Steps, RPInteractionStep{Kind: "move", TargetPlaceID: matches[0]})
		if hasSpeech {
			plan.Steps = append(plan.Steps, RPInteractionStep{Kind: "speech", SpeechText: speech})
			plan.Kind = "MIXED"
		} else {
			plan.Kind = "ACTION"
		}
		return plan, nil
	}
	if strings.HasPrefix(input, "等") || strings.HasPrefix(input, "等待") {
		rest := strings.TrimSpace(strings.TrimPrefix(input, "等"))
		if strings.HasPrefix(input, "等待") {
			rest = strings.TrimSpace(strings.TrimPrefix(input, "等待"))
		}
		interval, speech, hasSpeech, ok := splitRPActionSpeech(rest)
		if !ok {
			return clarify("请用“等1小时，随后说「话语」”明确等待与说话的顺序。")
		}
		hours := 0
		switch interval {
		case "1小时", "一小时":
			hours = 1
		case "2小时", "两小时":
			hours = 2
		case "4小时", "四小时":
			hours = 4
		}
		if hours == 0 {
			return clarify("目前支持明确等待1、2或4小时；其他时长请用原有等待操作。")
		}
		plan.Steps = append(plan.Steps, RPInteractionStep{Kind: "wait", WaitHours: hours})
		if hasSpeech {
			plan.Steps = append(plan.Steps, RPInteractionStep{Kind: "speech", SpeechText: speech})
			plan.Kind = "MIXED"
		} else {
			plan.Kind = "ACTION"
		}
		return plan, nil
	}
	if mode == "SCENE" && strings.HasPrefix(input, "说「") && strings.HasSuffix(input, "」") {
		input = strings.TrimSuffix(strings.TrimPrefix(input, "说「"), "」")
		if strings.TrimSpace(input) == "" {
			return clarify("请填写要说出的内容。")
		}
		plan.Kind = "DIALOGUE"
		plan.Steps = append(plan.Steps, RPInteractionStep{Kind: "speech", SpeechText: input})
		return plan, nil
	}
	return clarify("有限离线规则无法确认这段自由文字是对白还是行动；请选择“只说话”或连接语义理解服务。")
}

func splitRPActionSpeech(rest string) (action, speech string, hasSpeech, ok bool) {
	for _, delimiter := range []string{"，随后说「", "，然后说「", "，说「", ",随后说「", ",然后说「", ",说「"} {
		if before, after, found := strings.Cut(rest, delimiter); found {
			before, after = strings.TrimSpace(before), strings.TrimSpace(after)
			if before == "" || !strings.HasSuffix(after, "」") || strings.TrimSpace(strings.TrimSuffix(after, "」")) == "" {
				return "", "", false, false
			}
			return before, strings.TrimSuffix(after, "」"), true, true
		}
	}
	if strings.TrimSpace(rest) == "" || strings.ContainsAny(rest, "，,「」") {
		return "", "", false, false
	}
	return strings.TrimSpace(rest), "", false, true
}
