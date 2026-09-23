package core

import (
	"context"
	"fmt"
	"strings"
)

const RPStyleVersion = "corerp.style.v1"

// Presentation only. These fields never enter a DecisionProvider context.
type RPStyleProfile struct {
	Version              string   `json:"version"`
	POV                  string   `json:"pov"`
	Tense                string   `json:"tense"`
	Verbosity            string   `json:"verbosity"`
	DialogueRatio        int      `json:"dialogue_ratio"`
	DescriptionDensity   int      `json:"description_density"`
	InnerMonologuePolicy string   `json:"inner_monologue_policy"`
	ProseInstructions    string   `json:"prose_instructions"`
	ForbiddenPatterns    []string `json:"forbidden_patterns"`
	NarrativePackRef     string   `json:"narrative_pack_ref"`
}

// nil inherits a setting; an explicit zero/empty value replaces it.
type RPStylePatch struct {
	POV                  *string  `json:"pov,omitempty"`
	Tense                *string  `json:"tense,omitempty"`
	Verbosity            *string  `json:"verbosity,omitempty"`
	DialogueRatio        *int     `json:"dialogue_ratio,omitempty"`
	DescriptionDensity   *int     `json:"description_density,omitempty"`
	InnerMonologuePolicy *string  `json:"inner_monologue_policy,omitempty"`
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
		if p.POV != nil {
			s.POV = *p.POV
		}
		if p.Tense != nil {
			s.Tense = *p.Tense
		}
		if p.Verbosity != nil {
			s.Verbosity = *p.Verbosity
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
	EventID   string `json:"event_id"`
	ActorID   string `json:"actor_id"`
	ActorName string `json:"actor_name"`
	Action    string `json:"action"`
	Text      string `json:"text,omitempty"`
	WorldTime string `json:"world_time,omitempty"`
	PlaceName string `json:"place_name,omitempty"`
}

type RPNarrativeInput struct {
	ControlledEntityID string            `json:"controlled_entity_id"`
	Style              RPStyleProfile    `json:"style"`
	Facts              []RPNarrativeFact `json:"committed_facts"`
}

type RPNarrativeView struct {
	Lines    []string `json:"lines"`
	EventIDs []string `json:"event_ids"`
	Warnings []string `json:"warnings"`
}

type RPNarrativeProvider interface {
	Render(context.Context, RPNarrativeInput) (RPNarrativeView, error)
}
type DeterministicRPNarrativeProvider struct{}

// Literal rendering changes presentation only. Even a requested zero dialogue
// ratio or forbidden phrase cannot erase/rewrite accepted speech or an action.
func (DeterministicRPNarrativeProvider) Render(_ context.Context, in RPNarrativeInput) (RPNarrativeView, error) {
	view := RPNarrativeView{Lines: []string{}, EventIDs: []string{}, Warnings: []string{}}
	if err := in.Style.Validate(); err != nil {
		return view, err
	}
	if in.Style.ProseInstructions != "" {
		view.Warnings = append(view.Warnings, "deterministic renderer does not interpret free-form prose instructions")
	}
	for _, fact := range in.Facts {
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
		switch fact.Action {
		case "speak":
			framing = name + "说"
		case "respond":
			framing = name + " 回应"
		case "refuse":
			framing = name + " 拒绝了"
		case "silence":
			framing = name + " 保持沉默。"
		case "wait":
			framing = name + " 选择等待。"
		case "leave":
			framing = name + " 离开了。"
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
		if in.Style.Tense == "past" {
			line = "当时，" + line
		}
		if in.Style.DescriptionDensity > 0 && in.Style.Verbosity != "terse" && fact.PlaceName != "" {
			line = fmt.Sprintf("在%s，%s", fact.PlaceName, line)
		}
		if in.Style.Verbosity == "detailed" && fact.WorldTime != "" {
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
	}
	return view, nil
}
