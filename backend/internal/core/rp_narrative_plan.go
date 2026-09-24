package core

import "context"

// A closed presentation program, not model-written world prose. The model can
// interpret natural-language style instructions, but cannot supply facts,
// quoted speech, actor names, action labels, or hidden thoughts to the renderer.
type RPNarrativeStylePlan struct {
	POV                     string `json:"pov"`
	Tense                   string `json:"tense"`
	Verbosity               string `json:"verbosity"`
	DialogueRatio           int    `json:"dialogue_ratio"`
	DescriptionDensity      int    `json:"description_density"`
	NarrativePackRef        string `json:"narrative_pack_ref"`
	UnsupportedInstructions bool   `json:"unsupported_instructions"`
}

type RPNarrativeStylePlanner interface {
	PlanStyle(context.Context, RPStyleProfile) (RPNarrativeStylePlan, error)
}

func (plan RPNarrativeStylePlan) Apply(base RPStyleProfile) (RPStyleProfile, error) {
	// Instructions have already been interpreted; do not pretend that the
	// literal renderer itself understands prose. Other protected fields inherit.
	empty := ""
	return OverlayRPStyle(base, RPStylePatch{POV: &plan.POV, Tense: &plan.Tense, Verbosity: &plan.Verbosity, DialogueRatio: &plan.DialogueRatio, DescriptionDensity: &plan.DescriptionDensity, NarrativePackRef: &plan.NarrativePackRef, ProseInstructions: &empty})
}

type PlannedRPNarrativeProvider struct{ Planner RPNarrativeStylePlanner }

func (PlannedRPNarrativeProvider) NarrativeMode() string { return "style_planner" }

func (p PlannedRPNarrativeProvider) Render(ctx context.Context, in RPNarrativeInput) (RPNarrativeView, error) {
	return p.RenderStream(ctx, in, nil)
}

func (p PlannedRPNarrativeProvider) RenderStream(ctx context.Context, in RPNarrativeInput, emit func(RPNarrativeChunk) error) (RPNarrativeView, error) {
	if err := ctx.Err(); err != nil {
		return RPNarrativeView{}, err
	}
	if err := in.ValidateReadBudget(); err != nil {
		return RPNarrativeView{}, err
	}
	if in.Style.ProseInstructions == "" {
		return (DeterministicRPNarrativeProvider{}).RenderStream(ctx, in, emit)
	}
	if p.Planner == nil {
		return RPNarrativeView{}, NewError(CodeInvalidArgument, "narrative style planner is required")
	}
	plan, err := p.Planner.PlanStyle(ctx, in.Style)
	if err != nil {
		return RPNarrativeView{}, err
	}
	if err := ctx.Err(); err != nil {
		return RPNarrativeView{}, err
	}
	in.Style, err = plan.Apply(in.Style)
	if err != nil {
		return RPNarrativeView{}, err
	}
	view, err := (DeterministicRPNarrativeProvider{}).RenderStream(ctx, in, emit)
	if err != nil {
		return RPNarrativeView{}, err
	}
	if plan.UnsupportedInstructions {
		view.Warnings = append(view.Warnings, "部分自定义要求超出当前文风能力；仅应用可验证的视角、时态、详略、对话和场景呈现设置，未补写事件或心理。")
	}
	return view, nil
}
