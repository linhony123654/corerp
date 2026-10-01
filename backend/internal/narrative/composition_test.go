package narrative

import (
	"context"
	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/endpointpolicy"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Documents the old text-only guarantee; fresh composition rejects both drafts.
func TestLegacyProseAttributionGapReproduction(t *testing.T) {
	in := proseFixture()
	swapped := "Cai说：「今晚有空吗？」你说：「有啊，坐这儿吧。」"
	if err := validateProse(swapped, in); err != nil {
		t.Fatalf("old speaker-swap counterexample no longer reproduces: %v", err)
	}
	in.Facts[1].Text = in.Facts[0].Text
	if err := validateProse("你说：「今晚有空吗？」", in); err != nil {
		t.Fatalf("old duplicate-event counterexample no longer reproduces: %v", err)
	}
}

func TestCompositionRejectsOpenOrIncompletePlans(t *testing.T) {
	in := proseFixture()
	valid := `{"version":"corerp.fact-composition.v1","groups":[{"layout":"inline","atoms":[{"fact_ref":"f0","template":"plain"},{"fact_ref":"f1","template":"dialogue"}]}]}`
	for name, draft := range map[string]string{
		"legacy swap":                   "Cai说：「今晚有空吗？」你说：「有啊，坐这儿吧。」",
		"legacy single identical quote": "你说：「今晚有空吗？」",
		"version":                       strings.Replace(valid, compositionVersion, "v0", 1),
		"duplicate key":                 strings.Replace(valid, `"version":`, `"version":"bad","version":`, 1),
		"null":                          strings.Replace(valid, `"inline"`, `null`, 1),
		"extra speaker":                 strings.Replace(valid, `"fact_ref":"f0"`, `"fact_ref":"f0","speaker":"Cai"`, 1),
		"extra framing":                 strings.Replace(valid, `"layout":"inline"`, `"layout":"inline","text":"她心想"`, 1),
		"duplicate event":               strings.Replace(valid, `"f1"`, `"f0"`, 1),
		"unknown event":                 strings.Replace(valid, `"f1"`, `"f9"`, 1),
		"reorder":                       strings.Replace(strings.Replace(valid, `"f0"`, `"f2"`, 1), `"f1"`, `"f0"`, 1),
		"missing":                       `{"version":"corerp.fact-composition.v1","groups":[{"layout":"inline","atoms":[{"fact_ref":"f0","template":"plain"}]}]}`,
		"unlicensed template":           strings.Replace(valid, `"dialogue"`, `"embrace"`, 1),
		"trailing":                      valid + `{}`,
		"empty group":                   `{"version":"corerp.fact-composition.v1","groups":[{"layout":"lines","atoms":[]}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := renderComposition(context.Background(), draft, in); err == nil {
				t.Fatal("open or incomplete plan accepted")
			}
		})
	}
	if _, err := renderComposition(context.Background(), valid, in); err != nil {
		t.Fatal(err)
	}
}

func TestCompositionOwnsExactSpeechAndEveryObservable(t *testing.T) {
	in := proseFixture()
	in.Facts[0].Text = " 根据事实\n  「字面 [[corerp-speech:1]]」\n\tfacts？ "
	in.Facts[1].Text = in.Facts[0].Text
	in.Style.ForbiddenPatterns = []string{"根据事实"}
	in.Facts = append(in.Facts,
		core.RPNarrativeFact{EventID: "gesture", ActorID: "entity_cai", ActorName: "Cai", Action: "expression", ExpressionCode: "nod", TargetActorID: in.ControlledEntityID, TargetActorName: "Lin"},
		core.RPNarrativeFact{EventID: "object", ActorID: in.ControlledEntityID, ActorName: "Lin", Action: "object_switch_on", ObjectName: "灯", ObjectState: "on"},
		core.RPNarrativeFact{EventID: "move", ActorID: "entity_cai", ActorName: "Cai", Action: "depart", PlaceName: "咖啡馆"},
	)
	draft := `{"version":"corerp.fact-composition.v1","groups":[{"layout":"inline","atoms":[{"fact_ref":"f0","template":"dialogue"},{"fact_ref":"f1","template":"dialogue"}]},{"layout":"lines","atoms":[{"fact_ref":"f2","template":"plain"},{"fact_ref":"f3","template":"plain"},{"fact_ref":"f4","template":"plain"}]}]}`
	result, err := renderComposition(context.Background(), draft, in)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Lines) != 2 || strings.Count(result.Lines[0], in.Facts[0].Text) != 2 || !strings.Contains(result.Lines[0], "你：「"+in.Facts[0].Text+"」") || !strings.Contains(result.Lines[0], "Cai 回应：「"+in.Facts[1].Text+"」") {
		t.Fatalf("speech bytes/speakers changed: %+v", result)
	}
	for _, atom := range []string{"Cai 向你点了点头", "你开启了灯（on）", "Cai 离开了 咖啡馆"} {
		if !strings.Contains(result.Lines[1], atom) {
			t.Fatalf("observable lost: %s / %s", atom, result.Lines[1])
		}
	}
	if len(result.Sources[0]) != 2 || len(result.Sources[1]) != 3 || len(result.Warnings) == 0 {
		t.Fatalf("coverage/warnings lost: %+v", result)
	}
	bad := strings.Replace(draft, `"fact_ref":"f2","template":"plain"`, `"fact_ref":"f2","template":"dialogue"`, 1)
	if _, err := renderComposition(context.Background(), bad, in); err == nil {
		t.Fatal("action rendered as speech")
	}
	in.Style.POV = "first_person"
	result, err = renderComposition(context.Background(), draft, in)
	if err != nil || !strings.Contains(result.Lines[0], "我：「") || !strings.Contains(result.Lines[1], "向我点") {
		t.Fatalf("POV broken: %+v %v", result, err)
	}
}

func TestCompositionProviderRepairsLegacyAndStreamsBoundGroups(t *testing.T) {
	in := proseFixture()
	in.Facts[0].Text = "今晚\n  有空吗？"
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Messages []struct{ Role, Content string }
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		var payload struct {
			Version string `json:"composition_version"`
			Facts   []struct {
				Ref       string   `json:"fact_ref"`
				Templates []string `json:"allowed_templates"`
			}
		}
		if len(request.Messages) < 2 || json.Unmarshal([]byte(request.Messages[1].Content), &payload) != nil || payload.Version != compositionVersion || len(payload.Facts) != 2 || payload.Facts[0].Ref != "f0" || len(payload.Facts[0].Templates) != 4 {
			t.Error("actual provider view lacks closed fact references")
			return
		}
		draft := "Cai说：「今晚\n  有空吗？」你说：「有啊，坐这儿吧。」"
		if calls.Add(1) == 2 {
			if len(request.Messages) != 4 || !strings.Contains(request.Messages[3].Content, compositionVersion) {
				t.Error("repair not versioned")
			}
			draft = `{"version":"corerp.fact-composition.v1","groups":[{"layout":"inline","atoms":[{"fact_ref":"f0","template":"dialogue"}]},{"layout":"inline","atoms":[{"fact_ref":"f1","template":"plain"}]}]}`
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]any{"content": draft}}}})
	}))
	defer server.Close()
	p, err := NewChatProseProvider(Config{Endpoint: server.URL + "/v1/chat/completions", EndpointPolicy: endpointpolicy.TestLocalhostPolicy(), Model: "fixture", Attempts: 2, Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	var chunks []core.RPNarrativeChunk
	view, err := p.RenderStream(context.Background(), in, func(c core.RPNarrativeChunk) error { chunks = append(chunks, c); return nil })
	if err != nil || view.FallbackReason != "" || calls.Load() != 2 || view.CompositionVersion != CompositionVersion || len(view.FactGroups) != 2 || len(view.Lines) != 2 || len(chunks) != 2 || !strings.Contains(view.Lines[0], in.Facts[0].Text) {
		t.Fatalf("composition integration failed: %+v %v calls=%d", view, err, calls.Load())
	}
	for i, c := range chunks {
		if len(c.EventIDs) != 1 || c.EventIDs[0] != in.Facts[i].EventID || c.Line != view.Lines[i] {
			t.Fatalf("group attribution wrong: %+v", c)
		}
	}
}

func TestCompositionProviderRejectsFreshLegacyWithoutEmission(t *testing.T) {
	for name, draft := range map[string]string{
		"raw prose":         validSourcedProse,
		"quote-only tokens": "你：[[corerp-speech:0]]\nCai：[[corerp-speech:1]]",
	} {
		t.Run(name, func(t *testing.T) {
			var calls atomic.Int32
			server := proseServer(t, draft, &calls)
			defer server.Close()
			p, err := NewChatProseProvider(Config{Endpoint: server.URL + "/v1/chat/completions", EndpointPolicy: endpointpolicy.TestLocalhostPolicy(), Model: "fixture", Attempts: 1, Timeout: time.Second})
			if err != nil {
				t.Fatal(err)
			}
			in := proseFixture()
			var lines []string
			view, err := p.RenderStream(context.Background(), in, func(c core.RPNarrativeChunk) error { lines = append(lines, c.Line); return nil })
			if err != nil || view.FallbackReason != "prose_invalid composition plan" || len(lines) != 2 || calls.Load() != 1 || len(view.EventIDs) != 2 || view.CompositionVersion != "" || len(view.FactGroups) != 0 {
				t.Fatalf("fresh legacy did not recover without claiming composition: %+v %v", view, err)
			}
			expected, err := (core.DeterministicRPNarrativeProvider{}).Render(context.Background(), in)
			if err != nil {
				t.Fatal(err)
			}
			for i, line := range lines {
				if !strings.Contains(line, in.Facts[i].Text) || line != expected.Lines[i] {
					t.Fatal("unvalidated legacy emission or lost speech")
				}
			}
		})
	}
	if proseValidationCategory(failure("invalid composition plan")) != "composition_invalid" {
		t.Fatal("unsafe diagnostic")
	}
}

func TestCompositionBudgetAndCancellation(t *testing.T) {
	in := proseFixture()
	draft := `{"version":"corerp.fact-composition.v1","groups":[{"layout":"inline","atoms":[{"fact_ref":"f0","template":"plain"},{"fact_ref":"f1","template":"plain"}]}]}`
	in.Facts[0].Text = strings.Repeat("字", maxProseRunes)
	if _, err := renderComposition(context.Background(), draft, in); err == nil {
		t.Fatal("output budget ignored")
	}
	in = proseFixture()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := renderComposition(ctx, draft, in); err == nil {
		t.Fatal("cancel ignored")
	}
}

func TestCompositionSupportedFactVocabularyAndTargets(t *testing.T) {
	base := proseFixture()
	for _, fact := range []core.RPNarrativeFact{
		{Action: "silence"}, {Action: "wait"}, {Action: "leave"},
		{Action: "act", ActivityCode: "read", ActivityLabel: "读书"},
		{Action: "activity_done", ActivityCode: "read", ActivityLabel: "读书"},
		{Action: "activity_interrupted", ActivityCode: "read", ActivityLabel: "读书"},
		{Action: "arrive", PlaceName: "园子"}, {Action: "depart", PlaceName: "园子"},
		{Action: "object_open", ObjectName: "门", ObjectState: "open"},
		{Action: "object_close", ObjectName: "门", ObjectState: "closed"},
		{Action: "object_switch_on", ObjectName: "灯", ObjectState: "on"},
		{Action: "object_switch_off", ObjectName: "灯", ObjectState: "off"},
		{Action: "expression", ExpressionCode: "smile"},
		{Action: "expression", ExpressionCode: "nod", TargetActorID: "entity_lin", TargetActorName: "Lin"},
		{Action: "expression", ExpressionCode: "shake_head"},
		{Action: "expression", ExpressionCode: "frown"},
		{Action: "expression", ExpressionCode: "beckon"},
		{Action: "expression", ExpressionCode: "turn_away", TargetActorID: "entity_lin", TargetActorName: "Lin"},
	} {
		t.Run(fact.Action+fact.ExpressionCode, func(t *testing.T) {
			fact.EventID = "observable"
			fact.ActorID = "entity_cai"
			fact.ActorName = "陌生人"
			in := base
			in.Facts = []core.RPNarrativeFact{fact}
			draft := `{"version":"corerp.fact-composition.v1","groups":[{"layout":"inline","atoms":[{"fact_ref":"f0","template":"plain"}]}]}`
			result, err := renderComposition(context.Background(), draft, in)
			expected, expectedErr := (core.DeterministicRPNarrativeProvider{}).Render(context.Background(), core.RPNarrativeInput{ControlledEntityID: in.ControlledEntityID, Style: in.Style, Facts: in.Facts})
			if err != nil || expectedErr != nil || len(result.Lines) != 1 || !strings.Contains(result.Lines[0], "陌生人") || result.Sources[0][0] != "observable" {
				t.Fatalf("public atom failed: %+v %v / %+v %v", result, err, expected, expectedErr)
			}
			if fact.TargetActorID != "" && !strings.Contains(result.Lines[0], "你") {
				t.Fatal("target lost")
			}
		})
	}
}

func TestCompositionInputBudgetAndSourcesFailBeforeHTTP(t *testing.T) {
	var calls atomic.Int32
	server := proseServer(t, validSourcedProse, &calls)
	defer server.Close()
	p, err := NewChatProseProvider(Config{Endpoint: server.URL + "/v1/chat/completions", EndpointPolicy: endpointpolicy.TestLocalhostPolicy(), Model: "fixture", Attempts: 1})
	if err != nil {
		t.Fatal(err)
	}
	in := proseFixture()
	in.Facts[1].EventID = in.Facts[0].EventID
	if _, err := p.Render(context.Background(), in); err == nil {
		t.Fatal("duplicate source accepted")
	}
	in = proseFixture()
	in.Style.ContextBudgetBytes = 256
	if _, err := p.Render(context.Background(), in); err == nil {
		t.Fatal("read budget ignored")
	}
	if calls.Load() != 0 {
		t.Fatal("invalid input reached HTTP")
	}
}

func TestCompositionRefusalKeepsActionWithNonObviousSpeech(t *testing.T) {
	in := proseFixture()
	in.Facts = []core.RPNarrativeFact{{EventID: "refusal", ActorID: "entity_cai", ActorName: "Cai", Action: "refuse", Text: "茶还温着。"}}
	for _, template := range []string{"plain", "compact", "contextual", "dialogue"} {
		t.Run(template, func(t *testing.T) {
			draft := `{"version":"corerp.fact-composition.v1","groups":[{"layout":"inline","atoms":[{"fact_ref":"f0","template":"` + template + `"}]}]}`
			result, err := renderComposition(context.Background(), draft, in)
			if err != nil || len(result.Lines) != 1 || !strings.Contains(result.Lines[0], "Cai 拒绝了") || !strings.Contains(result.Lines[0], "「茶还温着。」") || result.Sources[0][0] != "refusal" {
				t.Fatalf("template lost refusal stance or exact words: %+v %v", result, err)
			}
		})
	}
}

func TestCompositionPlainRetainsRequestedRichSourceFraming(t *testing.T) {
	in := proseFixture()
	in.Style.Verbosity = "detailed"
	in.Style.NarrativePackRef = "builtin/dialogue@1"
	in.Facts = append(in.Facts, core.RPNarrativeFact{EventID: "gesture", ActorID: "entity_cai", ActorName: "Cai", Action: "expression", ExpressionCode: "nod", TargetActorID: in.ControlledEntityID, TargetActorName: "Lin", PlaceName: "咖啡馆", WorldTime: "2026-09-22T20:00:02Z"})
	draft := `{"version":"corerp.fact-composition.v1","groups":[{"layout":"inline","atoms":[{"fact_ref":"f0","template":"plain"},{"fact_ref":"f1","template":"plain"},{"fact_ref":"f2","template":"plain"}]}]}`
	outputs := map[string]string{}
	for _, density := range []string{"concise", "standard", "long"} {
		in.Style.NarrativeDensity = density
		result, err := renderComposition(context.Background(), draft, in)
		expected, expectedErr := (core.DeterministicRPNarrativeProvider{}).Render(context.Background(), in)
		if err != nil || expectedErr != nil || result.Lines[0] != strings.Join(expected.Lines, "") {
			t.Fatalf("plain overwrote requested %s framing: %+v %v / %+v %v", density, result, err, expected, expectedErr)
		}
		outputs[density] = result.Lines[0]
		for _, fact := range in.Facts {
			if fact.Text != "" && !strings.Contains(result.Lines[0], "「"+fact.Text+"」") {
				t.Fatal("style changed accepted speech")
			}
		}
		if !strings.Contains(result.Lines[0], "Cai 向你点了点头") || len(result.Sources[0]) != 3 {
			t.Fatal("style changed target or coverage")
		}
	}
	if outputs["concise"] == outputs["standard"] || outputs["standard"] == outputs["long"] || outputs["concise"] == outputs["long"] {
		t.Fatal("requested density collapsed on fact-rich input")
	}
	if strings.Contains(outputs["concise"], "在咖啡馆") || !strings.Contains(outputs["standard"], "在咖啡馆") || !strings.Contains(outputs["long"], in.Facts[0].WorldTime) {
		t.Fatal("density lost source-safe context")
	}
	in.Style.NarrativeDensity = "standard"
	compact, err := renderComposition(context.Background(), strings.ReplaceAll(draft, `"plain"`, `"compact"`), in)
	if err != nil || strings.Contains(compact.Lines[0], "在咖啡馆") || !strings.Contains(compact.Lines[0], "当时，") {
		t.Fatalf("compact changed tense or kept optional context: %+v %v", compact, err)
	}
	contextual, err := renderComposition(context.Background(), strings.ReplaceAll(draft, `"plain"`, `"contextual"`), in)
	if err != nil || !strings.Contains(contextual.Lines[0], "在咖啡馆") || !strings.Contains(contextual.Lines[0], in.Facts[0].WorldTime) {
		t.Fatalf("contextual lost requested sourced framing: %+v %v", contextual, err)
	}
}

func TestCompositionReportsLexicalAndPublicStyleCapability(t *testing.T) {
	in := proseFixture()
	in.Style.FullProse = true
	in.Style.ProseInstructions = "用华丽词汇改写对白，呈现角色私有心理。"
	in.PublicPresentations = []core.RPPublicPresentation{{ActorID: "entity_cai", ActorName: "Cai", SourceEventID: "style", Text: "说话温柔，使用独特比喻。"}}
	result, err := renderComposition(context.Background(), factCompositionFixture("f0", "f1"), in)
	if err != nil || len(result.Warnings) == 0 || result.Warnings[0] != compositionCapabilityWarning {
		t.Fatalf("unsupported lexical request silently accepted: %+v %v", result, err)
	}
	for _, word := range []string{"自由改写", "角色措辞", "自定义文风", "公开角色表达线索"} {
		if !strings.Contains(result.Warnings[0], word) {
			t.Fatalf("capability warning omits %s", word)
		}
	}
	for i, fact := range in.Facts {
		if !strings.Contains(result.Lines[i], "「"+fact.Text+"」") {
			t.Fatal("unsupported style rewrote canon")
		}
	}
	for _, ids := range result.Sources {
		for _, id := range ids {
			if id == "style" {
				t.Fatal("style declaration became a fact")
			}
		}
	}
}
