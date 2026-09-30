package narrative

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/endpointpolicy"
)

const validSourcedProse = "你问：「今晚有空吗？」\nCai 应声：「有啊，坐这儿吧。」"

func proseFixture() core.RPNarrativeInput {
	return core.RPNarrativeInput{
		ControlledEntityID: "entity_lin",
		Style: core.RPStyleProfile{
			Version: "corerp.style.v1", POV: "second_person", Tense: "past", Verbosity: "normal",
			DialogueRatio: 50, DescriptionDensity: 50, NarrativePackRef: "builtin/plain@1", InnerMonologuePolicy: "none",
		},
		Facts: []core.RPNarrativeFact{
			{EventID: "e1", ActorID: "entity_lin", ActorName: "Lin", Action: "speak", Text: "今晚有空吗？", PlaceName: "咖啡馆", WorldTime: "2026-09-22T20:00:00Z"},
			{EventID: "e2", ActorID: "entity_cai", ActorName: "Cai", Action: "respond", Text: "有啊，坐这儿吧。", PlaceName: "咖啡馆", WorldTime: "2026-09-22T20:00:01Z"},
		},
	}
}

func proseServer(t *testing.T, content string, calls *atomic.Int32) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var body struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil || (len(body.Messages) != 2 && len(body.Messages) != 4) {
			t.Error("invalid prose request")
		}
		if strings.Contains(body.Messages[0].Content, "今晚有空吗") {
			t.Error("committed speech leaked into the instruction channel")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{
			"finish_reason": "stop", "message": map[string]any{"content": content},
		}}})
	}))
}

func TestChatProseProviderRendersSourcedParagraphs(t *testing.T) {
	var calls atomic.Int32
	server := proseServer(t, validSourcedProse, &calls)
	defer server.Close()
	provider, err := NewChatProseProvider(Config{Endpoint: server.URL + "/v1/chat/completions", EndpointPolicy: endpointpolicy.TestLocalhostPolicy(), Model: "prose-model", APIKey: "k"})
	if err != nil {
		t.Fatal(err)
	}
	var streamed []core.RPNarrativeChunk
	view, err := provider.RenderStream(context.Background(), proseFixture(), func(chunk core.RPNarrativeChunk) error {
		streamed = append(streamed, chunk)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Lines) != 2 || len(streamed) != 2 || view.Lines[1] != streamed[1].Line || view.FallbackReason != "" {
		t.Fatalf("prose paragraphs not streamed in order: %+v", view)
	}
	if len(view.EventIDs) != 2 || view.EventIDs[0] != "e1" || view.EventIDs[1] != "e2" {
		t.Fatalf("prose view lost fact attribution: %+v", view.EventIDs)
	}
	for _, chunk := range streamed {
		if chunk.EventID != "" || len(chunk.EventIDs) != 2 || chunk.EventIDs[0] != "e1" || chunk.EventIDs[1] != "e2" {
			t.Fatalf("synthetic paragraph pretended to cite a single event: %+v", chunk)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("unexpected model calls: %d", calls.Load())
	}
}

func TestChatProseProviderUsesSourcedPublicPresentationOnly(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var body struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Messages) != 2 {
			t.Errorf("invalid prose request: %v", err)
		}
		var payload struct {
			PublicPresentations []prosePresentation `json:"public_presentations"`
		}
		if err := json.Unmarshal([]byte(body.Messages[1].Content), &payload); err != nil || len(payload.PublicPresentations) != 1 || payload.PublicPresentations[0].Actor != "Cai" || payload.PublicPresentations[0].Style != "措辞温和，习惯用短句。" {
			t.Errorf("public style not isolated in prose payload: %+v %v", payload, err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{
			"finish_reason": "stop", "message": map[string]any{"content": "你问：「今晚有空吗？」Cai答：「有啊，坐这儿吧。」"},
		}}})
	}))
	defer server.Close()
	provider, err := NewChatProseProvider(Config{Endpoint: server.URL + "/v1/chat/completions", EndpointPolicy: endpointpolicy.TestLocalhostPolicy(), Model: "m", Timeout: time.Second, Attempts: 1})
	if err != nil {
		t.Fatal(err)
	}
	input := proseFixture()
	input.PublicPresentations = []core.RPPublicPresentation{{ActorID: "entity_cai", ActorName: "Cai", Text: "措辞温和，习惯用短句。", SourceEventID: "studio-source"}}
	view, err := provider.Render(context.Background(), input)
	if err != nil || view.FallbackReason != "" || len(view.EventIDs) != 2 || calls.Load() != 1 {
		t.Fatalf("sourced public presentation forced fallback or changed fact attribution: %+v %v", view, err)
	}
	input.PublicPresentations[0].ActorID = "unknown-actor"
	if _, err := provider.Render(context.Background(), input); err == nil || calls.Load() != 1 {
		t.Fatalf("unattributed public style reached model: %v calls=%d", err, calls.Load())
	}
}

func TestChatProseProviderCapsDensityWhenOnlyTwoPublicUtterancesExist(t *testing.T) {
	var densities []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Messages) != 2 {
			t.Errorf("invalid prose request: %v", err)
			return
		}
		var payload struct {
			Density string `json:"density"`
		}
		if err := json.Unmarshal([]byte(body.Messages[1].Content), &payload); err != nil {
			t.Error(err)
			return
		}
		densities = append(densities, payload.Density)
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{
			"finish_reason": "stop", "message": map[string]any{"content": "你：「今晚有空吗？」\nCai：「有啊，坐这儿吧。」"},
		}}})
	}))
	defer server.Close()
	provider, err := NewChatProseProvider(Config{Endpoint: server.URL + "/v1/chat/completions", EndpointPolicy: endpointpolicy.TestLocalhostPolicy(), Model: "m", Timeout: time.Second, Attempts: 1})
	if err != nil {
		t.Fatal(err)
	}
	input := proseFixture()
	input.Style.Verbosity = "detailed"
	input.Style.NarrativeDensity = "long"
	if view, err := provider.Render(context.Background(), input); err != nil || view.FallbackReason != "" {
		t.Fatalf("sparse exchange could not render: %+v %v", view, err)
	}
	standard := input
	standard.Style.NarrativeDensity = "standard"
	if view, err := provider.Render(context.Background(), standard); err != nil || view.FallbackReason != "" {
		t.Fatalf("sparse standard exchange could not render: %+v %v", view, err)
	}
	withGesture := input
	withGesture.Facts = append(append([]core.RPNarrativeFact(nil), input.Facts...), core.RPNarrativeFact{
		EventID: "e3", ActorID: "entity_cai", ActorName: "Cai", Action: "expression", ExpressionCode: "smile",
	})
	if view, err := provider.Render(context.Background(), withGesture); err != nil || view.FallbackReason != "" {
		t.Fatalf("observable-rich exchange could not render: %+v %v", view, err)
	}
	withPublicCue := input
	withPublicCue.PublicPresentations = []core.RPPublicPresentation{{ActorID: "entity_cai", ActorName: "Cai", Text: "说话干脆。", SourceEventID: "studio-source"}}
	if view, err := provider.Render(context.Background(), withPublicCue); err != nil || view.FallbackReason != "" {
		t.Fatalf("authored-style exchange could not render: %+v %v", view, err)
	}
	if len(densities) != 4 || densities[0] != "concise" || densities[1] != "concise" || densities[2] != "long" || densities[3] != "long" {
		t.Fatalf("prose density ignored the actual public evidence: %+v", densities)
	}
}

func TestChatProseProviderRepairsDroppedSpeechWithExplicitFeedback(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := calls.Add(1)
		var body struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		content := "你问：「今晚有空吗？」Cai 回应了。"
		if call == 2 {
			if len(body.Messages) != 4 || body.Messages[2].Role != "assistant" || body.Messages[3].Role != "user" || !strings.Contains(body.Messages[3].Content, "逐字保留") {
				t.Fatalf("repair request lacks explicit rejected-draft feedback: %+v", body.Messages)
			}
			content = validSourcedProse
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]any{"content": content}}}})
	}))
	defer server.Close()
	provider, err := NewChatProseProvider(Config{Endpoint: server.URL + "/v1/chat/completions", EndpointPolicy: endpointpolicy.TestLocalhostPolicy(), Model: "m", Timeout: time.Second, Attempts: 2})
	if err != nil {
		t.Fatal(err)
	}
	view, err := provider.Render(context.Background(), proseFixture())
	if err != nil || len(view.Warnings) != 0 || len(view.Lines) != 2 || calls.Load() != 2 {
		t.Fatalf("draft with dropped speech was not repaired: %+v %v calls=%d", view, err, calls.Load())
	}
}

func TestValidateProseAcceptsConciseDraftAndRejectsCalendarLedDraft(t *testing.T) {
	input := proseFixture()
	short := "你问：「今晚有空吗？」\nCai 回答：「有啊，坐这儿吧。」"
	if err := validateProse(short, input); err != nil {
		t.Fatalf("factful concise prose rejected: %v", err)
	}
	calendarLed := "2026年9月22日晚上，咖啡馆里安静下来，近处的光线在桌面铺开。你看向 Cai，在短暂的停顿之后问道：「今晚有空吗？」这句问话并不突兀，只是顺着眼前的气氛轻轻落下，等待她给出回应。\nCai 接住你的目光，神色随之松弛下来。她没有让沉默继续拉长，很快便笑着答道：「有啊，坐这儿吧。」语气自然又亲近，让这段刚刚起头的交谈有了继续下去的余地。"
	if err := validateProse(calendarLed, input); err == nil || !strings.Contains(err.Error(), "exact date") {
		t.Fatalf("calendar-led prose accepted: %v", err)
	}
}

func TestValidateProseRejectsGenerationRuleLanguage(t *testing.T) {
	input := proseFixture()
	draft := "咖啡馆里安静下来，近处的光线落在桌面上。你稍稍停了一会儿，才看向 Cai 问道：「今晚有空吗？」话音落下后，四周的声响显得更远，眼前只剩等待回应的短暂空白。\nCai 抬眼笑了笑，很快答道：「有啊，坐这儿吧。」她的回应承接着刚才的问话，没有新的事情被展开，也没有别的人物出现，一切都严格停留在两句对白之内。"
	if err := validateProse(draft, input); err == nil || !strings.Contains(err.Error(), "fact-audit") {
		t.Fatalf("generation-rule prose accepted: %v", err)
	}
}

func TestChatProseProviderRejectsRewrittenOrInventedDialogue(t *testing.T) {
	for name, draft := range map[string]string{
		"rewritten speech": "你问：「今晚有时间吗？」Cai 说：「有啊，坐这儿吧。」",
		"invented speech":  "你问：「今晚有空吗？」Cai 说：「有啊，坐这儿吧。」服务员过来问：「要点什么？」",
		"dropped speech":   "你轻声问了句什么。Cai 说：「有啊，坐这儿吧。」",
	} {
		t.Run(name, func(t *testing.T) {
			var calls atomic.Int32
			server := proseServer(t, draft, &calls)
			defer server.Close()
			provider, err := NewChatProseProvider(Config{Endpoint: server.URL + "/v1/chat/completions", EndpointPolicy: endpointpolicy.TestLocalhostPolicy(), Model: "m", Timeout: time.Second, Attempts: 2})
			if err != nil {
				t.Fatal(err)
			}
			view, err := provider.RenderStream(context.Background(), proseFixture(), nil)
			if err != nil {
				t.Fatal(err)
			}
			// Fallback renders deterministically, says so, and names why.
			if len(view.Warnings) == 0 || !strings.Contains(view.Warnings[0], "回退") {
				t.Fatalf("invalid prose did not fall back with a warning: %+v", view)
			}
			if !strings.HasPrefix(view.FallbackReason, "prose_") {
				t.Fatalf("fallback reason not recorded: %+v", view)
			}
			if !strings.Contains(strings.Join(view.Lines, "\n"), "今晚有空吗？") {
				t.Fatalf("fallback lost committed speech: %+v", view.Lines)
			}
			if calls.Load() != 2 {
				t.Fatalf("validation failure did not retry within attempt budget: %d", calls.Load())
			}
		})
	}
}

func TestValidateProseAcceptsQuotedPhraseInsideCommittedSpeechOnly(t *testing.T) {
	input := proseFixture()
	input.Facts[1].Text = "你提起“那件事”，我并不知道。"
	accepted := "你问：「今晚有空吗？」Cai 回应：「你提起“那件事”，我并不知道。」"
	if err := validateProse(accepted, input); err != nil {
		t.Fatalf("inner quotation in exact accepted speech was rejected: %v", err)
	}
	if err := validateProse(accepted+"旁边又传来一句：“那件事”。", input); err == nil {
		t.Fatal("the same quoted phrase outside accepted speech escaped validation")
	}
}

func TestValidateProsePreservesNestedCommittedSpeech(t *testing.T) {
	for _, speech := range []string{
		"寶玉，你既能開口，便該先看清這屋裡坐的是誰。我正對著一箋舊句，倒被你這聲「有人嗎」攪了。",
		"你提起「那件事」，你决定付十元；这是你说过的话，并不是我确认发生了。",
		"你说“那件事”，我并不知道。",
	} {
		input := proseFixture()
		input.Facts[1].Text = speech
		for _, outer := range [][2]string{{"「", "」"}, {"“", "”"}} {
			accepted := "你：「今晚有空吗？」Cai：" + outer[0] + speech + outer[1]
			if err := validateProse(accepted, input); err != nil {
				t.Fatalf("nested committed quote was rejected: %v", err)
			}
			if unquoted := stripProseQuotes(accepted); strings.Contains(unquoted, "付十元") || strings.Contains(unquoted, "正對著") {
				t.Fatalf("part of committed speech escaped the narrative claim guard: %q", unquoted)
			}
			for _, extra := range []string{"旁边又传来一句：「那件事」。", "你决定付十元。", "Cai 招手。"} {
				if err := validateProse(accepted+extra, input); err == nil {
					t.Fatalf("uncommitted claim outside the nested speech escaped: %q", extra)
				}
			}
		}
	}
}

func TestProseQuotesRejectMismatchedNesting(t *testing.T) {
	for _, text := range []string{"「未结束", "缺少起始」", "「外层“内层」结束”"} {
		if _, err := quotedSpans(text); err == nil {
			t.Fatalf("malformed quotation passed: %q", text)
		}
		if got := stripProseQuotes(text); got != text {
			t.Fatalf("malformed quotation hid prose claims: %q", got)
		}
	}
}

func TestProseValidationCategoryNeverExposesUnknownDetails(t *testing.T) {
	if got := proseValidationCategory(failure("accepted speech was rewritten or dropped")); got != "speech_changed" {
		t.Fatalf("changed speech category: %q", got)
	}
	if got := proseValidationCategory(failure("uncommitted player agency in prose")); got != "player_action_not_committed" {
		t.Fatalf("player agency category: %q", got)
	}
	if got := proseValidationCategory(failure("private-key-or-model-text")); got != "other" {
		t.Fatalf("unrecognized error leaked into diagnostic: %q", got)
	}
}

func TestChatProseProviderInsertsCommittedSpeechByReference(t *testing.T) {
	input := proseFixture()
	input.Facts[1].Text = "我是黛玉。方才只顾核对诗笺，没留意你连我也认不得了。},\""
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var body struct {
			Messages []struct{ Content string } `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Messages) != 2 {
			t.Errorf("invalid referenced prose request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var payload struct {
			Facts []proseFact `json:"facts"`
		}
		if err := json.Unmarshal([]byte(body.Messages[1].Content), &payload); err != nil || len(payload.Facts) != 2 {
			t.Errorf("invalid fact packet: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if payload.Facts[0].QuoteToken == "" || payload.Facts[1].QuoteToken == "" || payload.Facts[1].Text != input.Facts[1].Text {
			t.Error("wire references missing or original speech changed")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{
			"finish_reason": "stop", "message": map[string]any{"content": "你：" + payload.Facts[0].QuoteToken + "\nCai：" + payload.Facts[1].QuoteToken},
		}}})
	}))
	defer server.Close()
	provider, err := NewChatProseProvider(Config{Endpoint: server.URL + "/v1/chat/completions", EndpointPolicy: endpointpolicy.TestLocalhostPolicy(), Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	view, err := provider.Render(context.Background(), input)
	if err != nil || view.FallbackReason != "" || calls.Load() != 1 {
		t.Fatalf("referenced speech render: %+v / %v, calls %d", view, err, calls.Load())
	}
	if !strings.Contains(strings.Join(view.Lines, "\n"), "「"+input.Facts[1].Text+"」") {
		t.Fatalf("the renderer cleaned or changed committed speech: %+v", view)
	}
	if len(view.EventIDs) != 2 || view.EventIDs[0] != input.Facts[0].EventID || view.EventIDs[1] != input.Facts[1].EventID {
		t.Fatalf("speech references changed fact provenance: %+v", view)
	}
}

func TestProseSpeechReferencesDoNotAuthorizeExtraClaims(t *testing.T) {
	input := proseFixture()
	for _, draft := range []string{
		"[[corerp-speech:999]]",
		"[[corerp-speech:0]] [[corerp-speech:0]] [[corerp-speech:1]]",
		"[[corerp-speech:0]]",
		"[[corerp-speech:0]] [[corerp-speech:1]] [[corerp-speech:bad]]",
		"[[corerp-speech:0]] [[corerp-speech:1]] [[corerp-speech:2",
	} {
		if _, err := expandProseSpeechTokens(draft, input); err == nil {
			t.Fatalf("invalid speech reference passed: %q", draft)
		}
	}
	for _, extra := range []string{"你决定付十元。", "Cai 招手。", "Cai：「新话。」"} {
		text, err := expandProseSpeechTokens("你：[[corerp-speech:0]] Cai：[[corerp-speech:1]]"+extra, input)
		if err != nil {
			t.Fatal(err)
		}
		if err := validateProse(text, input); err == nil {
			t.Fatalf("speech references licensed a new claim: %q", extra)
		}
	}
}

func TestProseSpeechReferencesNeverExpandQuotedLiteralRecursively(t *testing.T) {
	input := proseFixture()
	input.Facts[1].Text = "我说的是[[corerp-speech:0]]，没有别的话。"
	text, err := expandProseSpeechTokens("你：[[corerp-speech:0]] Cai：[[corerp-speech:1]]", input)
	if err != nil || validateProse(text, input) != nil {
		t.Fatalf("literal reference inside committed speech: %q / %v", text, err)
	}
	if !strings.Contains(text, input.Facts[1].Text) || strings.Count(text, input.Facts[0].Text) != 1 {
		t.Fatalf("inserted speech was expanded recursively: %q", text)
	}
	legacy := "你：「" + input.Facts[0].Text + "」Cai：「" + input.Facts[1].Text + "」"
	if preserved, err := expandProseSpeechTokens(legacy, input); err != nil || preserved != legacy {
		t.Fatalf("literal token in valid direct quotes was treated as an instruction: %q / %v", preserved, err)
	}
}

func TestValidateProseRequiresCommittedExpression(t *testing.T) {
	input := proseFixture()
	draft := "你问：「今晚有空吗？」Cai 招了招手，回答：「有啊，坐这儿吧。」"
	if err := validateProse(draft, input); err == nil || !strings.Contains(err.Error(), "uncommitted expression") {
		t.Fatalf("uncommitted gesture entered narration: %v", err)
	}
	input.Facts = append(input.Facts, core.RPNarrativeFact{EventID: "expression-1", ActorID: "entity_cai", ActorName: "Cai", Action: "expression", ExpressionCode: "beckon"})
	if err := validateProse(draft, input); err != nil {
		t.Fatalf("committed gesture was rejected: %v", err)
	}
}

func TestValidateProseExpressionBelongsToItsWitnessedActor(t *testing.T) {
	input := proseFixture()
	input.Facts = append(input.Facts,
		core.RPNarrativeFact{EventID: "e3", ActorID: "entity_mei", ActorName: "Mei", Action: "silence"},
		core.RPNarrativeFact{EventID: "e4", ActorID: "entity_cai", ActorName: "Cai", Action: "expression", ExpressionCode: "beckon"},
	)
	for _, draft := range []string{
		"你问：「今晚有空吗？」Mei 招手，Cai 回答：「有啊，坐这儿吧。」",
		"你问：「今晚有空吗？」Cai 回答：「有啊，坐这儿吧。」她招手。",
	} {
		if err := validateProse(draft, input); err == nil || !strings.Contains(err.Error(), "uncommitted expression") {
			t.Fatalf("another or ambiguous actor borrowed Cai's gesture: %q: %v", draft, err)
		}
	}
	if err := validateProse("你问：「今晚有空吗？」Cai 招手，回答：「有啊，坐这儿吧。」", input); err != nil {
		t.Fatalf("Cai's witnessed gesture was rejected: %v", err)
	}
}

func TestValidateProseCarriesOnlyUnambiguousExpressionSubject(t *testing.T) {
	input := proseFixture()
	input.Facts[1].ActorName = "袭人"
	input.Facts = append(input.Facts,
		core.RPNarrativeFact{EventID: "e3", ActorID: "entity_baochai", ActorName: "宝钗", Action: "silence"},
		core.RPNarrativeFact{EventID: "e4", ActorID: "entity_cai", ActorName: "袭人", Action: "expression", ExpressionCode: "smile"},
		core.RPNarrativeFact{EventID: "e5", ActorID: "entity_baochai", ActorName: "宝钗", Action: "expression", ExpressionCode: "nod", TargetActorID: input.ControlledEntityID, TargetActorName: input.Facts[0].ActorName},
	)
	for _, draft := range []string{
		"你问：「今晚有空吗？」袭人微笑着：「有啊，坐这儿吧。」宝钗保持沉默，只是点了点头。",
		"你问：「今晚有空吗？」袭人微笑：「有啊，坐这儿吧。」宝钗保持沉默，对你点头。",
		"你问：「今晚有空吗？」袭人微笑：「有啊，坐这儿吧。」宝钗对你点头。",
	} {
		if err := validateProse(draft, input); err != nil {
			t.Fatalf("witnessed actor was mistaken for recipient or lost across a comma: %q: %v", draft, err)
		}
	}
	for _, draft := range []string{
		"你问：「今晚有空吗？」宝钗微笑：「有啊，坐这儿吧。」袭人点头。",
		"你问：「今晚有空吗？」袭人微笑：「有啊，坐这儿吧。」宝钗保持沉默。只是点了点头。",
		"你问：「今晚有空吗？」袭人微笑：「有啊，坐这儿吧。」袭人叫宝钗点头。",
		"你问：「今晚有空吗？」袭人微笑：「有啊，坐这儿吧。」她点头。",
	} {
		if err := validateProse(draft, input); err == nil || !strings.Contains(err.Error(), "uncommitted expression") {
			t.Fatalf("ambiguous or other actor borrowed expression: %q: %v", draft, err)
		}
	}
}

func TestChatProseProviderRepairsAmbiguousExpressionWithActorFeedback(t *testing.T) {
	input := proseFixture()
	input.Facts = append(input.Facts,
		core.RPNarrativeFact{EventID: "e3", ActorID: "entity_mei", ActorName: "Mei", Action: "silence"},
		core.RPNarrativeFact{EventID: "e4", ActorID: "entity_cai", ActorName: "Cai", Action: "expression", ExpressionCode: "nod"},
	)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := calls.Add(1)
		var body struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		content := "你问：「今晚有空吗？」Cai 回答：「有啊，坐这儿吧。」她点头。"
		if call == 2 {
			if len(body.Messages) != 4 || !strings.Contains(body.Messages[3].Content, "actor 名字") {
				t.Errorf("repair feedback lacks actor attribution: %+v", body.Messages)
			}
			content = "你问：「今晚有空吗？」Cai 回答：「有啊，坐这儿吧。」Cai点头。"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]any{"content": content}}}})
	}))
	defer server.Close()
	provider, err := NewChatProseProvider(Config{Endpoint: server.URL + "/v1/chat/completions", EndpointPolicy: endpointpolicy.TestLocalhostPolicy(), Model: "m", Timeout: time.Second, Attempts: 2})
	if err != nil {
		t.Fatal(err)
	}
	view, err := provider.Render(context.Background(), input)
	if err != nil || view.FallbackReason != "" || calls.Load() != 2 {
		t.Fatalf("ambiguous expression was not repaired: %+v %v calls=%d", view, err, calls.Load())
	}
}

// World-declared activity labels are the prose verb for activity facts: the
// model receives 沏茶, never the raw code.
func TestChatProseProviderSendsDeclaredActivityLabels(t *testing.T) {
	var calls atomic.Int32
	var gotFacts []proseFact
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var body struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil || len(body.Messages) != 2 {
			t.Error("invalid prose request")
		}
		var payload struct {
			Facts []proseFact `json:"facts"`
		}
		if json.Unmarshal([]byte(body.Messages[1].Content), &payload) != nil {
			t.Error("invalid prose payload")
		}
		gotFacts = payload.Facts
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{
			"finish_reason": "stop", "message": map[string]any{"content": "你看着 Cai 沏茶。"},
		}}})
	}))
	defer server.Close()
	provider, err := NewChatProseProvider(Config{Endpoint: server.URL + "/v1/chat/completions", EndpointPolicy: endpointpolicy.TestLocalhostPolicy(), Model: "m", Timeout: time.Second, Attempts: 1})
	if err != nil {
		t.Fatal(err)
	}
	input := proseFixture()
	input.ActivityLabels = map[string]string{"brew_tea": "沏茶"}
	input.Facts = append(input.Facts, core.RPNarrativeFact{EventID: "e3", ActorID: "entity_cai", ActorName: "Cai", Action: "activity_done", ActivityCode: "brew_tea", PlaceName: "咖啡馆", WorldTime: "2026-09-22T19:40:00Z"})
	if _, err := provider.Render(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	if len(gotFacts) != 3 || gotFacts[2].Action != "沏茶" {
		t.Fatalf("activity label not sent as prose verb: %+v", gotFacts)
	}
}

func TestChatProseProviderHonorsForbiddenPatterns(t *testing.T) {
	var calls atomic.Int32
	server := proseServer(t, "血溅了一地。你说：「今晚有空吗？」Cai 答：「有啊，坐这儿吧。」", &calls)
	defer server.Close()
	provider, err := NewChatProseProvider(Config{Endpoint: server.URL + "/v1/chat/completions", EndpointPolicy: endpointpolicy.TestLocalhostPolicy(), Model: "m", Timeout: time.Second, Attempts: 1})
	if err != nil {
		t.Fatal(err)
	}
	input := proseFixture()
	input.Style.ForbiddenPatterns = []string{"血"}
	view, err := provider.RenderStream(context.Background(), input, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Warnings) == 0 {
		t.Fatalf("forbidden pattern did not trigger fallback: %+v", view)
	}
}

func TestChatProseProviderRejectsUncommittedPlayerAgency(t *testing.T) {
	for name, added := range map[string]string{
		"movement":     "然后你走过去靠在她怀里。",
		"consent":      "你答应了她的邀请。",
		"spending":     "你掏出钱付了账。",
		"item":         "你把杯子递给了 Cai。",
		"intimacy":     "你伸手抱住了她。",
		"decision":     "你终于决定签下合同。",
		"extra speech": "你回答说会留下来。",
	} {
		t.Run(name, func(t *testing.T) {
			var calls atomic.Int32
			server := proseServer(t, "你问：「今晚有空吗？」Cai 回应：「有啊，坐这儿吧。」"+added, &calls)
			defer server.Close()
			provider, err := NewChatProseProvider(Config{Endpoint: server.URL + "/v1/chat/completions", EndpointPolicy: endpointpolicy.TestLocalhostPolicy(), Model: "m", Timeout: time.Second, Attempts: 2})
			if err != nil {
				t.Fatal(err)
			}
			view, err := provider.Render(context.Background(), proseFixture())
			want := "prose_uncommitted player agency in prose"
			if name == "extra speech" {
				want = "prose_uncommitted player speech in prose"
			}
			if err != nil || view.FallbackReason != want || calls.Load() != 2 {
				t.Fatalf("uncommitted player effect escaped presentation guard: %+v, %v, calls=%d", view, err, calls.Load())
			}
		})
	}
	input := proseFixture()
	for name, draft := range map[string]string{
		"accepted quoted claim": "你问：「今晚有空吗？」Cai 说：「有啊，坐这儿吧。」你听着她的回答。",
		"possible choice":       "你问：「今晚有空吗？」Cai 说：「有啊，坐这儿吧。」你可以坐下，也可以继续站着。",
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateProse(draft, input); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestChatProseProviderRejectsUncommittedSceneFacts(t *testing.T) {
	input := proseFixture()
	base := "你问：「今晚有空吗？」Cai 说：「有啊，坐这儿吧。」"
	for name, added := range map[string]string{
		"money":        "桌上摆着十元钱。",
		"object":       "桌上突然多出一只杯子。",
		"time":         "转眼过了十分钟。",
		"person":       "这时走进一位服务员。",
		"unknown name": "Lola 也在这里。",
		"NPC movement": "Cai 走过去拥抱了你。",
		"NPC gaze":     "Cai 听了这话，看着你。",
		"NPC stare":    "Cai 望向你。",
		"relationship": "你们就这样成了恋人。",
		"broken quote": "你问：「今晚有空吗？Cai 说：「有啊，坐这儿吧。」",
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateProse(base+added, input); err == nil {
				t.Fatal("uncommitted scene claim passed full prose validation")
			}
		})
	}
	if err := validateProse("你问：「今晚有空吗？」Cai 说：「有啊，坐这儿吧。」灯光落在桌上。", input); err != nil {
		t.Fatal("fact-neutral prose was rejected:", err)
	}
	first := proseFixture()
	first.Style.POV = "first_person"
	if err := validateProse("我问：「今晚有空吗？」Cai 说：「有啊，坐这儿吧。」我答应了她的邀请。", first); err == nil {
		t.Fatal("first-person prose gained uncommitted player consent")
	}
	moved := proseFixture()
	moved.Facts = append(moved.Facts, core.RPNarrativeFact{EventID: "e3", ActorID: moved.ControlledEntityID, ActorName: "Lin", Action: "arrive", PlaceName: "咖啡馆"})
	if err := validateProse("你走进咖啡馆，问：「今晚有空吗？」Cai 说：「有啊，坐这儿吧。」", moved); err != nil {
		t.Fatal("a committed player movement was rejected:", err)
	}
	npcMoved := proseFixture()
	npcMoved.Facts = append(npcMoved.Facts, core.RPNarrativeFact{EventID: "e3", ActorID: "entity_cai", ActorName: "Cai", Action: "leave", PlaceName: "咖啡馆"})
	if err := validateProse("你问：「今晚有空吗？」Cai 说：「有啊，坐这儿吧。」Cai 离开咖啡馆。", npcMoved); err != nil {
		t.Fatal("a committed NPC departure was rejected:", err)
	}
	if err := validateProse("你问：「今晚有空吗？」Cai 说：「有啊，坐这儿吧。」Cai 抱住了你。", npcMoved); err == nil {
		t.Fatal("a committed departure authorized an unrelated NPC intimacy action")
	}
}

func TestSparseSpeechAndSilenceCannotGrowPlayerAgency(t *testing.T) {
	input := proseFixture()
	input.Facts = []core.RPNarrativeFact{
		{EventID: "player-speech", ActorID: input.ControlledEntityID, ActorName: "宝玉", Action: "speak", Text: "找老祖宗呀", PlaceName: "荣庆堂"},
		{EventID: "npc-silence", ActorID: "entity_cai", ActorName: "陌生人", Action: "silence", PlaceName: "荣庆堂"},
	}
	if err := validateProse("你：「找老祖宗呀」陌生人没有应答。", input); err != nil {
		t.Fatalf("short sourced silence rejected: %v", err)
	}
	for _, draft := range []string{
		"你看着陌生人，说：「找老祖宗呀」陌生人没有应答。",
		"你不打算再同他僵持，只说：「找老祖宗呀」陌生人没有应答。",
		"你：「找老祖宗呀」陌生人没有应答。你不再多绕。",
	} {
		if err := validateProse(draft, input); err == nil {
			t.Fatalf("invented player beat survived guard: %q", draft)
		}
	}
	const failedStepDraft = "荣庆堂里，你看着眼前这个始终保持沉默的陌生人。从始至终，对方既不出声，也不应答，那份沉默沉沉地横在你们之间，没有半分松动的迹象。你不打算再同他僵持，也不打算再绕弯子，便直接把最后那条明路指给他。\n你不再多绕，只直直对着这个始终保持沉默的陌生人开口：「找老祖宗呀」。可那陌生人听完之后，依旧保持着沉默，仿佛你这句话与别的声响没什么不同，一落进这荣庆堂便再寻不见踪迹。你既已把该说的说完，接下来如何，全凭他自己拿主意。"
	if err := validateProse(failedStepDraft, input); err == nil {
		t.Fatal("recorded Step 5 Preview scene invention passed validation")
	}
	density := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Messages) != 2 {
			t.Errorf("invalid request: %v", err)
			return
		}
		var payload struct {
			Density string `json:"density"`
		}
		if err := json.Unmarshal([]byte(body.Messages[1].Content), &payload); err != nil {
			t.Error(err)
		}
		density <- payload.Density
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]any{"content": failedStepDraft}}}})
	}))
	defer server.Close()
	provider, err := NewChatProseProvider(Config{Endpoint: server.URL + "/v1/chat/completions", EndpointPolicy: endpointpolicy.TestLocalhostPolicy(), Model: "test", Attempts: 1})
	if err != nil {
		t.Fatal(err)
	}
	view, err := provider.Render(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if <-density != "concise" || view.FallbackReason == "" || strings.Contains(strings.Join(view.Lines, " "), "你看着") {
		t.Fatalf("unsafe sparse prose was published: view=%+v err=%v", view, err)
	}
}
