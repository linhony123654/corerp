package core

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

type narrativePlannerFunc func(context.Context, RPStyleProfile) (RPNarrativeStylePlan, error)

func (f narrativePlannerFunc) PlanStyle(ctx context.Context, s RPStyleProfile) (RPNarrativeStylePlan, error) {
	return f(ctx, s)
}

func TestRPNarrativePlanExecutesWithoutFactOrProtectedStyleChanges(t *testing.T) {
	style := DefaultRPStyle()
	style.ProseInstructions = "用第一人称，回忆口吻，台词另起一行，并标明时间地点。"
	style.ForbiddenPatterns = []string{"绝不存在的词"}
	style.ContextBudgetBytes = 16384
	in := RPNarrativeInput{ControlledEntityID: "lin", Style: style, Facts: []RPNarrativeFact{
		{EventID: "one", ActorID: "lin", ActorName: "Lin", Action: "speak", Text: "请保留我的原话。", PlaceName: "咖啡馆", WorldTime: "2026-09-22T09:00:00Z"},
		{EventID: "two", ActorID: "cai", ActorName: "Cai", Action: "refuse", Text: "我现在不能答应。"},
		{EventID: "three", ActorID: "cai", ActorName: "Cai", Action: "leave"},
	}}
	original, _ := CanonicalJSON(in)
	plan := RPNarrativeStylePlan{POV: "first_person", Tense: "past", Verbosity: "detailed", DialogueRatio: 100, DescriptionDensity: 80, NarrativePackRef: "builtin/dialogue@1"}
	calls := 0
	p := PlannedRPNarrativeProvider{Planner: narrativePlannerFunc(func(_ context.Context, s RPStyleProfile) (RPNarrativeStylePlan, error) {
		calls++
		if !reflect.DeepEqual(s, style) {
			t.Fatal("planner did not get resolved requested style")
		}
		return plan, nil
	})}
	var chunks []RPNarrativeChunk
	view, err := p.RenderStream(context.Background(), in, func(c RPNarrativeChunk) error { chunks = append(chunks, c); return nil })
	if err != nil || calls != 1 || len(chunks) != 3 || !strings.Contains(view.Lines[0], "在咖啡馆") || !strings.Contains(view.Lines[0], "当时") || !strings.Contains(view.Lines[0], "我说：\n「请保留我的原话。」") {
		t.Fatalf("custom plan did not affect actual prose: %+v %v", view, err)
	}
	for i, f := range in.Facts {
		if view.EventIDs[i] != f.EventID || len(chunks[i].EventIDs) != 1 || chunks[i].EventIDs[0] != f.EventID || !strings.Contains(view.Lines[i], f.Text) {
			t.Fatal("source attribution or literal text changed")
		}
	}
	if !strings.Contains(view.Lines[1], "拒绝") || !strings.Contains(view.Lines[2], "离开了") {
		t.Fatal("style changed action")
	}
	after, _ := CanonicalJSON(in)
	if string(after) != string(original) {
		t.Fatal("planner mutated input")
	}
	applied, err := plan.Apply(style)
	if err != nil || applied.ContextBudgetBytes != style.ContextBudgetBytes || !reflect.DeepEqual(applied.ForbiddenPatterns, style.ForbiddenPatterns) || applied.InnerMonologuePolicy != style.InnerMonologuePolicy {
		t.Fatal("plan altered protected fields")
	}
}

func TestRPNarrativePlanRejectsInvalidOutputBeforeEmission(t *testing.T) {
	in := RPNarrativeInput{Style: DefaultRPStyle(), Facts: []RPNarrativeFact{{EventID: "e", ActorName: "Lin", Action: "speak", Text: "原话"}}}
	in.Style.ProseInstructions = "改写格式"
	valid := RPNarrativeStylePlan{POV: "second_person", Tense: "present", Verbosity: "normal", DialogueRatio: 100, NarrativePackRef: "builtin/plain@1"}
	for _, mutate := range []func(*RPNarrativeStylePlan){func(p *RPNarrativeStylePlan) { p.POV = "invented" }, func(p *RPNarrativeStylePlan) { p.DialogueRatio = -1 }, func(p *RPNarrativeStylePlan) { p.NarrativePackRef = "remote/anything" }} {
		plan := valid
		mutate(&plan)
		emitted := 0
		p := PlannedRPNarrativeProvider{Planner: narrativePlannerFunc(func(context.Context, RPStyleProfile) (RPNarrativeStylePlan, error) { return plan, nil })}
		if _, err := p.RenderStream(context.Background(), in, func(RPNarrativeChunk) error { emitted++; return nil }); !HasCode(err, CodeInvalidArgument) || emitted != 0 {
			t.Fatalf("invalid plan emitted: %d %v", emitted, err)
		}
	}
	valid.UnsupportedInstructions = true
	p := PlannedRPNarrativeProvider{Planner: narrativePlannerFunc(func(context.Context, RPStyleProfile) (RPNarrativeStylePlan, error) { return valid, nil })}
	view, err := p.Render(context.Background(), in)
	if err != nil || len(view.Warnings) != 1 || !strings.Contains(view.Lines[0], "原话") {
		t.Fatal("unsupported requirements silently claimed satisfied")
	}
	in.Style.ProseInstructions = ""
	p.Planner = narrativePlannerFunc(func(context.Context, RPStyleProfile) (RPNarrativeStylePlan, error) {
		t.Fatal("empty custom instructions caused remote planning")
		return valid, nil
	})
	if _, err := p.Render(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	in.Style.ProseInstructions = "改写格式"
	broken := errors.New("fixture failure")
	p.Planner = narrativePlannerFunc(func(context.Context, RPStyleProfile) (RPNarrativeStylePlan, error) { return valid, broken })
	if _, err := p.Render(context.Background(), in); !errors.Is(err, broken) {
		t.Fatal("planner failure silently presented as custom success")
	}
}
