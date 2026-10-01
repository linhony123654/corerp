package core

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func naturalFixture() RPNarrativeInput {
	style := DefaultRPStyle()
	style.NarrativeDensity = "standard"
	return RPNarrativeInput{SourceHead: 12, ControlledEntityID: "player", Style: style,
		PublicPresentations: []RPPublicPresentation{{ActorID: "grandmother", ActorName: "贾母", Text: "叙述用古典措辞。", SourceEventID: "declaration"}},
		Facts: []RPNarrativeFact{
			{EventID: "greeting", ActorID: "player", ActorName: "宝玉", Action: "speak", Text: "老祖宗，我来看您了。", PlaceName: "荣庆堂"},
			{EventID: "reply", ActorID: "grandmother", ActorName: "贾母", Action: "respond", Text: "宝玉来了。有什么话，慢慢说给老祖宗听。", PlaceName: "荣庆堂"},
			{EventID: "smile", ActorID: "grandmother", ActorName: "贾母", Action: "expression", ExpressionCode: "smile", CompanionEventID: "reply", PlaceName: "荣庆堂"},
		}}
}

func TestRPNaturalDefaultFusesOnlyProvenCompanions(t *testing.T) {
	in := naturalFixture()
	view, err := (DeterministicRPNarrativeProvider{}).Render(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if view.CompositionVersion != RPFactCompositionVersionV2 || len(view.Lines) != 2 || !reflect.DeepEqual(view.FactGroups, [][]string{{"greeting"}, {"reply", "smile"}}) || view.Artifact == nil {
		t.Fatalf("no natural primary artifact: %+v", view)
	}
	if view.Lines[1] != "「宝玉来了。有什么话，慢慢说给老祖宗听。」贾母应道，露出笑容。" {
		t.Fatalf("natural fused beat differs: %s", view.Lines[1])
	}
	joined := strings.Join(view.Lines, "\n")
	for _, unsupported := range []string{"她", "笑着", "同时", "片刻", "慈爱", "低声", "在荣庆堂", "2026-"} {
		if strings.Contains(joined, unsupported) {
			t.Fatalf("invented or repetitive framing %s", unsupported)
		}
	}
	uncoupled := in
	uncoupled.Facts = append([]RPNarrativeFact(nil), in.Facts...)
	uncoupled.Facts[2].CompanionEventID = ""
	uncoupled.Facts[1].WorldTime = "2026-10-01T00:00:00Z"
	uncoupled.Facts[2].WorldTime = uncoupled.Facts[1].WorldTime
	plain, err := (DeterministicRPNarrativeProvider{}).Render(context.Background(), uncoupled)
	if err != nil || len(plain.Lines) != 3 {
		t.Fatalf("same timestamp incorrectly fused: %+v %v", plain, err)
	}
	bad := in
	bad.Facts = append([]RPNarrativeFact(nil), in.Facts...)
	bad.Facts[2].ActorID = "someone-else"
	if _, err := BuildDefaultRPComposition(bad); !HasCode(err, CodeProjectionDiverged) {
		t.Fatal("cross-actor companion accepted", err)
	}
}

func TestRPNaturalExactCoverageClaimsTargetsAndRefusal(t *testing.T) {
	in := naturalFixture()
	in.Facts[0].Text = " literal [[corerp-speech:1]]\n  「你已经付了钱」\t "
	in.Facts[1].Text = in.Facts[0].Text
	in.Facts[1].Action = "refuse"
	in.Facts[2].ExpressionCode = "beckon"
	in.Facts[2].TargetActorID = "player"
	in.Facts[2].TargetActorName = "宝玉"
	for _, pov := range []string{"first_person", "second_person", "third_person"} {
		in.Style.POV = pov
		view, err := (DeterministicRPNarrativeProvider{}).Render(context.Background(), in)
		if err != nil {
			t.Fatal(err)
		}
		joined := strings.Join(view.Lines, "\n")
		if strings.Count(joined, in.Facts[0].Text) != 2 || !strings.Contains(view.Lines[1], "拒绝") || !reflect.DeepEqual(view.EventIDs, []string{"greeting", "reply", "smile"}) {
			t.Fatalf("quote/refusal/source changed: %+v", view)
		}
		target := map[string]string{"first_person": "我", "second_person": "你", "third_person": "宝玉"}[pov]
		if !strings.Contains(view.Lines[1], "向"+target) {
			t.Fatalf("target changed: %s", view.Lines[1])
		}
	}
	in.Facts[2].TargetActorName = ""
	if _, err := BuildDefaultRPComposition(in); !HasCode(err, CodeProjectionDiverged) {
		t.Fatal("unlabeled target accepted", err)
	}
}

func TestRPCompositionPlanStrictnessAndSemanticCoverage(t *testing.T) {
	in := naturalFixture()
	plan, err := BuildDefaultRPComposition(in)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(plan)
	if _, err := DecodeRPCompositionPlan(raw, in); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*RPCompositionPlan){
		"omit":                 func(p *RPCompositionPlan) { p.Paragraphs = p.Paragraphs[:1] },
		"repeat":               func(p *RPCompositionPlan) { p.Paragraphs[1].Beats[0].FactRefs = []string{"f0", "f1"} },
		"reorder":              func(p *RPCompositionPlan) { p.Paragraphs[0].Beats[0].FactRefs = []string{"f1"} },
		"gesture loses effect": func(p *RPCompositionPlan) { p.Paragraphs[1].Beats[0].Form = "quote_first" },
		"unproved overlap":     func(p *RPCompositionPlan) { p.Paragraphs[1].Beats[0].Form = "smiling_while_speaking" },
		"private emotion":      func(p *RPCompositionPlan) { p.Paragraphs[1].Beats[0].Lexical = "tenderly" },
		"v1 relabel":           func(p *RPCompositionPlan) { p.Version = RPFactCompositionVersionV1 },
	} {
		t.Run(name, func(t *testing.T) {
			copyPlan := RPCompositionPlan{}
			_ = json.Unmarshal(raw, &copyPlan)
			mutate(&copyPlan)
			if _, err := RenderRPComposition(context.Background(), in, copyPlan, nil); err == nil {
				t.Fatal("invalid semantic selection accepted")
			}
		})
	}
	for _, invalid := range []string{
		strings.Replace(string(raw), `"register":`, `"register":"plain","register":`, 1),
		strings.Replace(string(raw), `"register":"plain"`, `"register":null`, 1),
		strings.Replace(string(raw), `"context":"none"`, `"context":"none","speaker":"襲人"`, 1),
		string(raw) + `{}`,
	} {
		if _, err := DecodeRPCompositionPlan([]byte(invalid), in); err == nil {
			t.Fatal("open/null/duplicate JSON accepted", invalid)
		}
	}
}

func TestRPNaturalArtifactFrozenAndEmitsOnlyValidatedParagraphs(t *testing.T) {
	in := naturalFixture()
	plan, err := BuildDefaultRPComposition(in)
	if err != nil {
		t.Fatal(err)
	}
	var chunks []RPNarrativeChunk
	view, err := RenderRPComposition(context.Background(), in, plan, func(c RPNarrativeChunk) error { chunks = append(chunks, c); return nil })
	if err != nil || len(chunks) != len(view.Lines) {
		t.Fatal("stream failed", err)
	}
	for i, c := range chunks {
		if c.Line != view.Lines[i] || !reflect.DeepEqual(c.EventIDs, view.FactGroups[i]) {
			t.Fatal("group attribution differs")
		}
	}
	hash, err := HashJSON(view.Artifact.Input)
	if err != nil || hash != view.Artifact.InputSHA256 {
		t.Fatal("frozen hash differs")
	}
	frozen, _ := json.Marshal(view.Artifact)
	in.Facts[0].Text = "mutated"
	plan.Paragraphs[0].Beats[0].FactRefs[0] = "f9"
	in.PublicPresentations[0].Text = "private mutation"
	after, _ := json.Marshal(view.Artifact)
	if string(frozen) != string(after) {
		t.Fatal("artifact aliases caller memory")
	}
	original := naturalFixture()
	badPlan, _ := BuildDefaultRPComposition(original)
	badPlan.Paragraphs[1].Beats[0].Form = "invented"
	emitted := 0
	if _, err := RenderRPComposition(context.Background(), original, badPlan, func(RPNarrativeChunk) error { emitted++; return nil }); err == nil || emitted != 0 {
		t.Fatal("bad later fact emitted partial prose")
	}
	ctx, cancel := context.WithCancel(context.Background())
	count := 0
	_, err = RenderRPComposition(ctx, original, view.Artifact.Plan, func(RPNarrativeChunk) error { count++; cancel(); return nil })
	if !errors.Is(err, context.Canceled) || count != 1 {
		t.Fatal("cancel ignored", err, count)
	}
}

func TestRPNaturalEmptyAndSmallReadBudgetDoNotBlockPrimary(t *testing.T) {
	in := RPNarrativeInput{SourceHead: 1, Style: DefaultRPStyle(), Facts: []RPNarrativeFact{}}
	view, err := (DeterministicRPNarrativeProvider{}).Render(context.Background(), in)
	if err != nil || len(view.Lines) != 0 || len(view.EventIDs) != 0 || len(view.FactGroups) != 0 || view.Artifact == nil {
		t.Fatalf("empty public coverage blocked: %+v %v", view, err)
	}
	raw, _ := json.Marshal(view.Artifact.Plan)
	if _, err := DecodeRPCompositionPlan(raw, in); err != nil {
		t.Fatal("empty plan cannot reload", err)
	}
	in = naturalFixture()
	in.Style.ContextBudgetBytes = 4096
	in.Facts[0].Text = strings.Repeat("原话", 3000)
	if err := in.ValidateReadBudget(); err == nil {
		t.Fatal("test does not exceed read budget")
	}
	if _, err := (DeterministicRPNarrativeProvider{}).Render(context.Background(), in); err != nil {
		t.Fatal("read preference blocked primary settlement", err)
	}
}

func TestRPNaturalDensityRegisterAndActionTransition(t *testing.T) {
	in := naturalFixture()
	in.Facts = append(in.Facts, RPNarrativeFact{EventID: "arrival", ActorID: "maid", ActorName: "袭人", Action: "arrive", PlaceName: "荣庆堂"}, RPNarrativeFact{EventID: "tea", ActorID: "maid", ActorName: "袭人", Action: "speak", Text: "老太太，茶还温着。", PlaceName: "荣庆堂"})
	outputs := map[string]string{}
	for _, density := range []string{"concise", "standard", "long"} {
		in.Style.NarrativeDensity = density
		view, err := (DeterministicRPNarrativeProvider{}).Render(context.Background(), in)
		if err != nil {
			t.Fatal(err)
		}
		outputs[density] = strings.Join(view.Lines, "\n\n")
		if !strings.Contains(outputs[density], "袭人来到荣庆堂，") || !strings.Contains(outputs[density], "「老太太，茶还温着。」") {
			t.Fatal("arrival/speech transition lost effects")
		}
	}
	if outputs["concise"] == outputs["standard"] || outputs["standard"] == outputs["long"] {
		t.Fatal("density did not alter natural composition")
	}
	if strings.Count(outputs["long"], "在荣庆堂") != 1 || strings.Contains(outputs["standard"], "在荣庆堂") {
		t.Fatal("orientation repeats routine turn headers")
	}
	plain := in
	plain.PublicPresentations = nil
	plain.Style.NarrativeDensity = "standard"
	view, err := (DeterministicRPNarrativeProvider{}).Render(context.Background(), plain)
	if err != nil || strings.Contains(strings.Join(view.Lines, ""), "贾母应道") {
		t.Fatal("classical authored register invented by name", err)
	}
}

func TestRPNaturalActorBindingRegisterAndStableSelection(t *testing.T) {
	in := naturalFixture()
	in.Style.NarrativeDensity = "concise"
	in.Facts[1].Text = "袭人，你去看看宝玉。"
	view, err := (DeterministicRPNarrativeProvider{}).Render(context.Background(), in)
	if err != nil || !strings.Contains(view.Lines[1], "\n贾母") {
		t.Fatalf("gesture actor lost after named quote: %+v %v", view, err)
	}
	in.Style.NarrativeDensity = "standard"
	in.PublicPresentations[0].Text = "现代言辞从容，带长辈亲昵。"
	plan, err := BuildDefaultRPComposition(in)
	if err != nil || plan.Register != "plain" {
		t.Fatalf("modern cue changed global register: %+v %v", plan, err)
	}
	view, err = RenderRPComposition(context.Background(), in, plan, nil)
	if err != nil || strings.Contains(strings.Join(view.Lines, ""), "道") || !strings.Contains(view.Lines[1], "贾母答") {
		t.Fatalf("modern cue affected diction: %+v %v", view, err)
	}
	forged := plan
	forged.Register = "classical"
	if _, err := RenderRPComposition(context.Background(), in, forged, nil); err == nil {
		t.Fatal("model invented global classical register")
	}
	in.PublicPresentations[0].Text = "使用古典措辞。"
	scoped, err := (DeterministicRPNarrativeProvider{}).Render(context.Background(), in)
	if err != nil || !strings.Contains(scoped.Lines[0], "你说") || !strings.Contains(scoped.Lines[1], "道") {
		t.Fatalf("actor register leaked across cast: %+v %v", scoped, err)
	}
	in.Style.ProseInstructions = "全局采用古典叙述。"
	global, err := (DeterministicRPNarrativeProvider{}).Render(context.Background(), in)
	if err != nil || !strings.Contains(global.Lines[0], "你道") {
		t.Fatalf("explicit global register ignored: %+v %v", global, err)
	}
	shifted := in
	shifted.Facts = append([]RPNarrativeFact{{EventID: "unrelated", ActorID: "other", ActorName: "袭人", Action: "speak", Text: "我来送茶。"}}, in.Facts...)
	next, err := BuildDefaultRPComposition(shifted)
	if err != nil {
		t.Fatal(err)
	}
	original, err := BuildDefaultRPComposition(in)
	if err != nil {
		t.Fatal(err)
	}
	a, b := original.Paragraphs[1].Beats[0], next.Paragraphs[2].Beats[0]
	if a.Form != b.Form || a.Lexical != b.Lexical {
		t.Fatalf("unrelated fact changed beat selection: %+v %+v", a, b)
	}
}
