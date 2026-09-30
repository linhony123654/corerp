package core

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

const RPStyleVersion = "corerp.style.v1"
const DefaultRPNarrativeContextBudgetBytes = 64 * 1024

// Presentation only. These fields never enter a DecisionProvider context.
type RPStyleProfile struct {
	ContextBudgetBytes   int    `json:"context_budget_bytes,omitempty"`
	Version              string `json:"version"`
	POV                  string `json:"pov"`
	Tense                string `json:"tense"`
	Verbosity            string `json:"verbosity"`
	NarrativeDensity     string `json:"narrative_density,omitempty"`
	DialogueRatio        int    `json:"dialogue_ratio"`
	DescriptionDensity   int    `json:"description_density"`
	InnerMonologuePolicy string `json:"inner_monologue_policy"`
	// FullProse is the declarative long-form switch at the same layer as
	// dialogue_ratio: the world asks for novel-paragraph narrative. Rendering
	// still honors it only when a full_prose-capable provider is configured;
	// otherwise the fallback reason is recorded, never silently dropped.
	FullProse         bool     `json:"full_prose,omitempty"`
	ProseInstructions string   `json:"prose_instructions"`
	ForbiddenPatterns []string `json:"forbidden_patterns"`
	NarrativePackRef  string   `json:"narrative_pack_ref"`
}

// nil inherits a setting; an explicit zero/empty value replaces it.
type RPStylePatch struct {
	ContextBudgetBytes   *int     `json:"context_budget_bytes,omitempty"`
	POV                  *string  `json:"pov,omitempty"`
	Tense                *string  `json:"tense,omitempty"`
	Verbosity            *string  `json:"verbosity,omitempty"`
	NarrativeDensity     *string  `json:"narrative_density,omitempty"`
	DialogueRatio        *int     `json:"dialogue_ratio,omitempty"`
	DescriptionDensity   *int     `json:"description_density,omitempty"`
	InnerMonologuePolicy *string  `json:"inner_monologue_policy,omitempty"`
	FullProse            *bool    `json:"full_prose,omitempty"`
	ProseInstructions    *string  `json:"prose_instructions,omitempty"`
	ForbiddenPatterns    []string `json:"forbidden_patterns"`
	NarrativePackRef     *string  `json:"narrative_pack_ref,omitempty"`
}

func DefaultRPStyle() RPStyleProfile {
	return RPStyleProfile{Version: RPStyleVersion, POV: "second_person", Tense: "present", Verbosity: "normal", DialogueRatio: 100, DescriptionDensity: 0, InnerMonologuePolicy: "none", ForbiddenPatterns: []string{}, NarrativePackRef: "builtin/plain@1"}
}

func ResolveRPStyle(layers ...RPStylePatch) (RPStyleProfile, error) {
	return OverlayRPStyle(DefaultRPStyle(), layers...)
}

func OverlayRPStyle(s RPStyleProfile, layers ...RPStylePatch) (RPStyleProfile, error) {
	if err := s.Validate(); err != nil {
		return RPStyleProfile{}, err
	}
	for _, p := range layers {
		if p.ContextBudgetBytes != nil {
			s.ContextBudgetBytes = *p.ContextBudgetBytes
		}
		if p.POV != nil {
			s.POV = *p.POV
		}
		if p.Tense != nil {
			s.Tense = *p.Tense
		}
		if p.Verbosity != nil {
			s.Verbosity = *p.Verbosity
		}
		if p.NarrativeDensity != nil {
			s.NarrativeDensity = *p.NarrativeDensity
		}
		if p.DialogueRatio != nil {
			s.DialogueRatio = *p.DialogueRatio
		}
		if p.DescriptionDensity != nil {
			s.DescriptionDensity = *p.DescriptionDensity
		}
		if p.InnerMonologuePolicy != nil {
			s.InnerMonologuePolicy = *p.InnerMonologuePolicy
		}
		if p.FullProse != nil {
			s.FullProse = *p.FullProse
		}
		if p.ProseInstructions != nil {
			s.ProseInstructions = *p.ProseInstructions
		}
		if p.ForbiddenPatterns != nil {
			s.ForbiddenPatterns = append([]string{}, p.ForbiddenPatterns...)
		}
		if p.NarrativePackRef != nil {
			s.NarrativePackRef = *p.NarrativePackRef
		}
		if err := s.Validate(); err != nil {
			return RPStyleProfile{}, err
		}
	}
	return s, nil
}

func (s RPStyleProfile) Validate() error {
	if s.ContextBudgetBytes != 0 && (s.ContextBudgetBytes < 4096 || s.ContextBudgetBytes > 256*1024) {
		return NewError(CodeInvalidArgument, "narrative context budget must be zero (default) or 4096–262144 bytes")
	}
	if s.Version != RPStyleVersion {
		return NewError(CodeInvalidArgument, "unsupported style version")
	}
	if s.POV != "first_person" && s.POV != "second_person" && s.POV != "third_person" {
		return NewError(CodeInvalidArgument, "invalid narrative POV")
	}
	if s.Tense != "present" && s.Tense != "past" {
		return NewError(CodeInvalidArgument, "invalid narrative tense")
	}
	if s.Verbosity != "terse" && s.Verbosity != "normal" && s.Verbosity != "detailed" {
		return NewError(CodeInvalidArgument, "invalid verbosity")
	}
	if s.NarrativeDensity != "" && s.NarrativeDensity != "concise" && s.NarrativeDensity != "standard" && s.NarrativeDensity != "long" {
		return NewError(CodeInvalidArgument, "invalid narrative density")
	}
	if s.DialogueRatio < 0 || s.DialogueRatio > 100 || s.DescriptionDensity < 0 || s.DescriptionDensity > 100 {
		return NewError(CodeInvalidArgument, "style density must be 0–100")
	}
	if s.InnerMonologuePolicy != "none" && s.InnerMonologuePolicy != "observed_only" {
		return NewError(CodeInvalidArgument, "unknown thoughts cannot be narrated")
	}
	if len([]rune(s.ProseInstructions)) > 2000 || len(s.ForbiddenPatterns) > 16 {
		return NewError(CodeInvalidArgument, "style instructions exceed limit")
	}
	for _, p := range s.ForbiddenPatterns {
		if strings.TrimSpace(p) == "" || len([]rune(p)) > 100 {
			return NewError(CodeInvalidArgument, "invalid forbidden pattern")
		}
	}
	if s.NarrativePackRef != "builtin/plain@1" && s.NarrativePackRef != "builtin/dialogue@1" {
		return NewError(CodeInvalidArgument, "unknown narrative pack reference")
	}
	return nil
}

type RPNarrativeFact struct {
	EventID      string `json:"event_id"`
	ActorID      string `json:"actor_id"`
	ActorName    string `json:"actor_name"`
	Action       string `json:"action"`
	Text         string `json:"text,omitempty"`
	ActivityCode string `json:"activity_code,omitempty"`
	// ActivityLabel is the world-declared prose label for the activity
	// (studio narrative package); empty means the code is shown as-is.
	ActivityLabel  string `json:"activity_label,omitempty"`
	ObjectName     string `json:"object_name,omitempty"`
	ObjectState    string `json:"object_state,omitempty"`
	ExpressionCode string `json:"expression_code,omitempty"`
	// Targets come from the player's frozen observation, never a raw decision
	// or an unseen target in the underlying Event. Unknown identities are masked.
	TargetActorID   string `json:"target_actor_id,omitempty"`
	TargetActorName string `json:"target_actor_name,omitempty"`
	WorldTime       string `json:"world_time,omitempty"`
	PlaceName       string `json:"place_name,omitempty"`
}

// An explicitly authored, player-safe character cue. It is not the NPC's
// private persona, intent or knowledge, and is never a new scene fact.
type RPPublicPresentation struct {
	ActorID       string `json:"actor_id"`
	ActorName     string `json:"actor_name"`
	Text          string `json:"text"`
	SourceEventID string `json:"source_event_id"`
}

type RPNarrativeInput struct {
	ControlledEntityID  string                 `json:"controlled_entity_id"`
	Style               RPStyleProfile         `json:"style"`
	Facts               []RPNarrativeFact      `json:"committed_facts"`
	PublicPresentations []RPPublicPresentation `json:"public_presentations,omitempty"`
	// ActivityLabels maps activity codes to world-declared prose labels.
	// Presentation metadata from the studio narrative package, never facts.
	ActivityLabels map[string]string `json:"activity_labels,omitempty"`
}

// ValidateReadBudget bounds the complete serialized presentation input, not
// model tokens, decision context, or saved world facts. Never truncate facts.
// Canonical turn settlement must remain independent of this read preference.
func (in RPNarrativeInput) ValidateReadBudget() error {
	if err := in.Style.Validate(); err != nil {
		return err
	}
	if len(in.PublicPresentations) > 16 {
		return NewError(CodeInvalidArgument, "too many public RP presentations")
	}
	for _, cue := range in.PublicPresentations {
		if cue.ActorID == "" || cue.ActorID == in.ControlledEntityID || cue.ActorName == "" || cue.SourceEventID == "" || cue.Text == "" || !utf8.ValidString(cue.Text) || utf8.RuneCountInString(cue.Text) > 500 || strings.ContainsAny(cue.Text, "\x00\r") {
			return NewError(CodeInvalidArgument, "invalid public RP presentation")
		}
		found := false
		for _, fact := range in.Facts {
			found = found || fact.ActorID == cue.ActorID && fact.ActorName == cue.ActorName
		}
		if !found {
			return NewError(CodeInvalidArgument, "public RP presentation has no public actor fact")
		}
	}
	limit := in.Style.ContextBudgetBytes
	if limit == 0 {
		limit = DefaultRPNarrativeContextBudgetBytes
	}
	encoded, err := json.Marshal(in)
	if err != nil {
		return WrapError(CodeInvalidArgument, "encode narrative context", err)
	}
	if len(encoded) > limit {
		return NewError(CodeInvalidArgument, fmt.Sprintf("叙述上下文为 %d 字节，超过 %d 字节预算；未删减事实。可读取已保存原叙述，或提高预算后重新生成。", len(encoded), limit))
	}
	return nil
}

type RPNarrativeView struct {
	Lines    []string `json:"lines"`
	EventIDs []string `json:"event_ids"`
	Warnings []string `json:"warnings"`
	RenderID string   `json:"render_id,omitempty"`
	// FallbackReason is the sanitized, queryable reason presentation fell
	// back to the deterministic renderer (e.g. prose validation failure or
	// a world-declared full_prose without a prose provider). Empty = the
	// displayed lines came from the requested provider.
	FallbackReason string `json:"fallback_reason,omitempty"`
}

type RPNarrativeProvider interface {
	Render(context.Context, RPNarrativeInput) (RPNarrativeView, error)
}

// Streaming is a presentation capability, never a DecisionProvider method.
// Implementations receive only the already-authorized committed narrative input.
type RPStreamingNarrativeProvider interface {
	RPNarrativeProvider
	RenderStream(context.Context, RPNarrativeInput, func(RPNarrativeChunk) error) (RPNarrativeView, error)
}
type RPNarrativeChunk struct {
	Index    int      `json:"index"`
	EventID  string   `json:"event_id,omitempty"`
	EventIDs []string `json:"event_ids,omitempty"`
	Line     string   `json:"line"`
}
type DeterministicRPNarrativeProvider struct{}

func (DeterministicRPNarrativeProvider) NarrativeMode() string { return "deterministic" }
func (DeterministicRPNarrativeProvider) ProviderMetadata() RPProviderMetadata {
	return RPProviderMetadata{Kind: "deterministic"}
}

// Literal rendering changes presentation only. Even a requested zero dialogue
// ratio or forbidden phrase cannot erase/rewrite accepted speech or an action.
func (p DeterministicRPNarrativeProvider) Render(ctx context.Context, in RPNarrativeInput) (RPNarrativeView, error) {
	return p.RenderStream(ctx, in, nil)
}

// RenderStream emits each attributed line when rendered, without synthetic
// delays. The caller must not treat a partial stream as a completed variant.
func (DeterministicRPNarrativeProvider) RenderStream(ctx context.Context, in RPNarrativeInput, emit func(RPNarrativeChunk) error) (RPNarrativeView, error) {
	view := RPNarrativeView{Lines: []string{}, EventIDs: []string{}, Warnings: []string{}}
	if err := in.Style.Validate(); err != nil {
		return view, err
	}
	if in.Style.ProseInstructions != "" {
		view.Warnings = append(view.Warnings, "deterministic renderer does not interpret free-form prose instructions")
	}
	for _, fact := range in.Facts {
		if err := ctx.Err(); err != nil {
			return RPNarrativeView{}, err
		}
		if fact.EventID == "" || fact.ActorName == "" {
			return RPNarrativeView{}, NewError(CodeProjectionDiverged, "narrative requires attributed committed evidence")
		}
		name := fact.ActorName
		if fact.ActorID == in.ControlledEntityID {
			switch in.Style.POV {
			case "first_person":
				name = "我"
			case "second_person":
				name = "你"
			}
		}
		var framing string
		activityLabel := func() string {
			if fact.ActivityLabel != "" {
				return fact.ActivityLabel
			}
			if label, ok := in.ActivityLabels[fact.ActivityCode]; ok && label != "" {
				return label
			}
			return fact.ActivityCode
		}
		switch fact.Action {
		case "speak":
			framing = name + "说"
		case "respond":
			framing = name + " 回应"
		case "refuse":
			framing = name + " 拒绝了"
		case "expression":
			verbs := map[string]string{"smile": "笑了笑", "nod": "点了点头", "shake_head": "摇了摇头", "turn_away": "转过身", "frown": "皱了皱眉", "beckon": "招了招手"}
			verb := verbs[fact.ExpressionCode]
			if verb == "" {
				return RPNarrativeView{}, NewError(CodeProjectionDiverged, "unknown committed NPC expression")
			}
			target := fact.TargetActorName
			if fact.TargetActorID == in.ControlledEntityID && fact.TargetActorID != "" {
				switch in.Style.POV {
				case "first_person":
					target = "我"
				case "second_person":
					target = "你"
				}
			}
			if fact.TargetActorID != "" && target != "" {
				preposition := "向"
				if fact.ExpressionCode == "turn_away" {
					preposition = "背向"
				}
				framing = name + " " + preposition + target + verb + "。"
			} else {
				framing = name + " " + verb + "。"
			}
		case "silence":
			framing = name + " 保持沉默。"
		case "wait":
			framing = name + " 选择等待。"
		case "leave":
			framing = name + " 离开了。"
		case "act":
			if fact.ActivityCode == "" {
				return RPNarrativeView{}, NewError(CodeProjectionDiverged, "activity fact lacks accepted activity code")
			}
			framing = name + " 开始了 " + activityLabel() + "。"
		case "activity_done":
			if fact.ActivityCode == "" {
				return RPNarrativeView{}, NewError(CodeProjectionDiverged, "activity fact lacks accepted activity code")
			}
			framing = name + " 做完了 " + activityLabel() + "。"
		case "activity_interrupted":
			if fact.ActivityCode == "" {
				return RPNarrativeView{}, NewError(CodeProjectionDiverged, "activity fact lacks accepted activity code")
			}
			framing = name + " 停下了手头的 " + activityLabel() + "。"
		case "arrive":
			if fact.PlaceName == "" {
				return RPNarrativeView{}, NewError(CodeProjectionDiverged, "arrival fact lacks accepted place")
			}
			framing = name + " 来到了 " + fact.PlaceName + "。"
		case "depart":
			if fact.PlaceName == "" {
				return RPNarrativeView{}, NewError(CodeProjectionDiverged, "departure fact lacks accepted place")
			}
			framing = name + " 离开了 " + fact.PlaceName + "。"
		case "object_open", "object_close", "object_switch_on", "object_switch_off":
			if fact.ObjectName == "" || fact.ObjectState == "" {
				return RPNarrativeView{}, NewError(CodeProjectionDiverged, "scene object fact lacks accepted object state")
			}
			verb := map[string]string{"object_open": "打开了", "object_close": "关上了", "object_switch_on": "开启了", "object_switch_off": "关闭了"}[fact.Action]
			framing = name + verb + fact.ObjectName + "（" + fact.ObjectState + "）。"
		default:
			return RPNarrativeView{}, NewError(CodeProjectionDiverged, "unknown narrative fact action")
		}
		spoken := fact.Action == "speak" || fact.Action == "respond" || fact.Action == "refuse"
		if spoken && fact.Text == "" {
			return RPNarrativeView{}, NewError(CodeProjectionDiverged, "speech fact lacks accepted text")
		}
		line := framing
		if spoken {
			if in.Style.NarrativePackRef == "builtin/dialogue@1" || (in.Style.DialogueRatio >= 75 && in.Style.Verbosity == "terse") {
				line = framing + "：\n「" + fact.Text + "」"
			} else {
				line = framing + "：「" + fact.Text + "」"
			}
		}
		literalLine := line
		if in.Style.NarrativeDensity == "long" {
			// Long layout has only source-backed time/place framing and the
			// accepted literal fact. Sparse scenes stay short; never pad a
			// response with invented sensations, thoughts or dialogue.
			parts := make([]string, 0, 2)
			if fact.PlaceName != "" {
				parts = append(parts, "在"+fact.PlaceName)
			}
			if fact.WorldTime != "" {
				parts = append(parts, fact.WorldTime)
			}
			if len(parts) > 0 {
				line = strings.Join(parts, "，") + "。\n" + framing
			} else {
				line = framing
			}
			if spoken {
				line += "：\n「" + fact.Text + "」"
			}
		}
		if in.Style.Tense == "past" {
			line = "当时，" + line
		}
		if in.Style.NarrativeDensity != "long" && in.Style.NarrativeDensity != "concise" && in.Style.DescriptionDensity > 0 && in.Style.Verbosity != "terse" && fact.PlaceName != "" {
			line = fmt.Sprintf("在%s，%s", fact.PlaceName, line)
		}
		if in.Style.NarrativeDensity != "long" && in.Style.NarrativeDensity != "concise" && in.Style.Verbosity == "detailed" && fact.WorldTime != "" {
			line = "（" + fact.WorldTime + "）" + line
		}
		matchesForbidden := func(text string) bool {
			for _, pattern := range in.Style.ForbiddenPatterns {
				if strings.Contains(text, pattern) {
					return true
				}
			}
			return false
		}
		if matchesForbidden(line) {
			if !matchesForbidden(literalLine) {
				line = literalLine
				view.Warnings = append(view.Warnings, "optional presentation suppressed by forbidden pattern")
			} else {
				view.Warnings = append(view.Warnings, "forbidden pattern conflicts with literal fact rendering; facts preserved")
			}
		}
		view.Lines = append(view.Lines, line)
		view.EventIDs = append(view.EventIDs, fact.EventID)
		if emit != nil {
			if err := emit(RPNarrativeChunk{Index: len(view.Lines) - 1, EventID: fact.EventID, Line: line}); err != nil {
				return RPNarrativeView{}, err
			}
		}
	}
	return view, nil
}
