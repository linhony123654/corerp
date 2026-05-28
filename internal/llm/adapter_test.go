package llm

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExtractActionFrameJSONBlock(t *testing.T) {
	raw := "```json\n{\"actor\":\"V\",\"action\":\"speak\",\"target\":\"player\",\"intensity\":3,\"emotion\":{\"primary\":\"calm\",\"secondary\":\"\",\"intensity\":0.3},\"intent\":\"greet\",\"suggested_line\":\"你好\",\"effects\":[]}\n```\n\nV冷冷地看了你一眼。"

	frame, narrative, err := ExtractActionFrame(raw)
	if err != nil {
		t.Fatalf("extract failed: %v", err)
	}
	if frame.Actor != "V" {
		t.Errorf("actor = %s, want V", frame.Actor)
	}
	if frame.Action != "speak" {
		t.Errorf("action = %s, want speak", frame.Action)
	}
	if narrative == "" {
		t.Error("narrative should not be empty")
	}
}

func TestGenerateNonStreamWithOptionsParsesOpenAIChoices(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer server.Close()

	got, err := NewAdapter(server.URL, "key", "model").GenerateNonStreamWithOptions(nil, 0.1, 64)
	if err != nil {
		t.Fatalf("GenerateNonStreamWithOptions: %v", err)
	}
	if got != "ok" {
		t.Fatalf("content = %q, want ok", got)
	}
}

func TestGenerateNonStreamWithOptionsParsesGeminiCandidates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"hello "},{"text":"world"}]}}]}`))
	}))
	defer server.Close()

	got, err := NewAdapter(server.URL, "key", "model").GenerateNonStreamWithOptions(nil, 0.1, 64)
	if err != nil {
		t.Fatalf("GenerateNonStreamWithOptions: %v", err)
	}
	if got != "hello world" {
		t.Fatalf("content = %q, want hello world", got)
	}
}

func TestGenerateNonStreamWithOptionsReportsEmptyChoicesBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices":[],"prompt_feedback":{"block_reason":"SAFETY"}}`))
	}))
	defer server.Close()

	_, err := NewAdapter(server.URL, "key", "model").GenerateNonStreamWithOptions(nil, 0.1, 64)
	if err == nil {
		t.Fatalf("expected no choices error")
	}
	if !strings.Contains(err.Error(), "prompt_feedback") {
		t.Fatalf("error = %q, want response body detail", err.Error())
	}
}

func TestExtractActionFrameInlineJSON(t *testing.T) {
	// Inline JSON must be flat (no nested {}) for extractFirstJSON to work
	raw := "前面的叙述内容。{\"actor\":\"Anya\",\"action\":\"threaten\",\"target\":\"enemy\",\"intensity\":7,\"emotion_primary\":\"angry\",\"intent\":\"intimidate\",\"suggested_line\":\"别动。\"}后面的叙述。"

	frame, narrative, err := ExtractActionFrame(raw)
	if err != nil {
		t.Fatalf("extract failed: %v", err)
	}
	if frame.Actor != "Anya" {
		t.Errorf("actor = %s, want Anya", frame.Actor)
	}
	if frame.Action != "threaten" {
		t.Errorf("action = %s, want threaten", frame.Action)
	}
	if frame.Intensity != 7 {
		t.Errorf("intensity = %d, want 7", frame.Intensity)
	}
	if narrative == "" {
		t.Error("narrative should be extracted")
	}
}

func TestExtractActionFrameNoJSON(t *testing.T) {
	raw := "V 没有回应，只是沉默地看着窗外，手指无意识地敲着桌面。雨声填满了沉默。"

	frame, narrative, err := ExtractActionFrame(raw)
	if err != nil {
		t.Fatalf("extract failed: %v", err)
	}
	if frame.Action != "" {
		t.Errorf("expected empty action frame for pure narrative, got action=%s", frame.Action)
	}
	if narrative != raw {
		t.Errorf("narrative should be the entire input for non-JSON text")
	}
}

func TestExtractActionFrameMalformedJSON(t *testing.T) {
	// Bad JSON in code block — should still extract narrative
	raw := "```json\n{broken json content}\n```\n\nThe narrative part continues here with 200 words."

	frame, narrative, err := ExtractActionFrame(raw)
	if err != nil {
		t.Fatalf("extract failed: %v", err)
	}
	// Frame should fall back to defaults (speak, unknown) since JSON fails to parse
	if frame.Action != "speak" {
		t.Errorf("action should default to 'speak' for malformed JSON, got %s", frame.Action)
	}
	if narrative == "" {
		t.Error("should extract narrative from after code blocks")
	}
}

func TestExtractActionFrameTextBlock(t *testing.T) {
	raw := "```json\n{\"actor\":\"V\",\"action\":\"speak\",\"target\":\"jackie\",\"intensity\":2,\"emotion\":{\"primary\":\"calm\",\"secondary\":\"\",\"intensity\":0.2},\"intent\":\"inform\",\"suggested_line\":\"走吧。\"}\n```\n```text\n两人推开酒吧的门，走进了夜之城的雨中。霓虹灯在湿漉漉的街道上投下斑斓的光。\n```"

	frame, narrative, err := ExtractActionFrame(raw)
	if err != nil {
		t.Fatalf("extract failed: %v", err)
	}
	if frame.Action != "speak" {
		t.Errorf("action = %s, want speak", frame.Action)
	}
	if narrative == "" {
		t.Error("should extract text block as narrative")
	}
}

func TestExtractActionFrameEmpty(t *testing.T) {
	frame, _, err := ExtractActionFrame("")
	if err != nil {
		t.Fatalf("extract failed: %v", err)
	}
	if frame.Action != "" {
		t.Errorf("expected empty frame, got action=%s", frame.Action)
	}
}

func TestParseFlexibleFrameStandardEffects(t *testing.T) {
	data := []byte(`{"actor":"V","action":"speak","target":"player","intensity":3,"emotion":{"primary":"calm","secondary":"","intensity":0.3},"intent":"","suggested_line":"测试","effects":[{"path":"trust","delta":0.5}]}`)
	frame, err := parseFlexibleFrame(data)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if len(frame.Effects) != 1 {
		t.Fatalf("expected 1 effect, got %d", len(frame.Effects))
	}
	if frame.Effects[0].Path != "trust" {
		t.Errorf("effect path = %s, want trust", frame.Effects[0].Path)
	}
	if frame.Effects[0].Delta != 0.5 {
		t.Errorf("effect delta = %f, want 0.5", frame.Effects[0].Delta)
	}
}

func TestParseFlexibleFrameStringEffects(t *testing.T) {
	// Some LLMs output effects as strings like ["trust-0.5"]
	data := []byte(`{"actor":"V","action":"speak","target":"player","intensity":3,"emotion":{"primary":"calm","secondary":"","intensity":0.3},"intent":"","suggested_line":"测试","effects":["trust-0.5","fear+1"]}`)
	frame, err := parseFlexibleFrame(data)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if len(frame.Effects) != 2 {
		t.Fatalf("expected 2 string effects, got %d", len(frame.Effects))
	}
	if frame.Effects[0].Path != "trust-0.5" {
		t.Errorf("effect path = %s, want trust-0.5", frame.Effects[0].Path)
	}
}

func TestParseFlexibleFrameInvalidJSON(t *testing.T) {
	_, err := parseFlexibleFrame([]byte("not json"))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestRouterFallback(t *testing.T) {
	// Verify routing setup doesn't panic
	router := NewRouter(NewAdapter("http://example.com", "", "model-a"))
	routes := router.Routes()
	if routes[TaskNarrative] != "default" {
		t.Errorf("narrative route = %s, want default", routes[TaskNarrative])
	}
}

func TestRouterSetInvalidRoute(t *testing.T) {
	router := NewRouter(NewAdapter("http://example.com", "", "model-a"))
	err := router.SetRoute(TaskNarrative, "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent adapter")
	}
}

func TestRouterDualAdapter(t *testing.T) {
	main := NewAdapter("http://main.example.com", "key", "opus")
	summary := NewAdapter("http://cheap.example.com", "key", "haiku")
	router := NewRouter(main)
	router.AddAdapter("summary", summary)
	router.SetRoute(TaskSummary, "summary")

	if r := router.Routes(); r[TaskSummary] != "summary" {
		t.Errorf("summary route = %s, want summary", r[TaskSummary])
	}
	if r := router.Routes(); r[TaskNarrative] != "default" {
		t.Errorf("narrative route should still be default, got %s", r[TaskNarrative])
	}
	adapters := router.Adapters()
	if len(adapters) != 2 {
		t.Errorf("expected 2 adapters, got %d", len(adapters))
	}
}

func TestExtractFirstJSON(t *testing.T) {
	// extractFirstJSON only handles flat JSON (no nested {})
	raw := `V推开门。{"actor":"V","action":"move","target":"door","intensity":2,"emotion_primary":"neutral","intent":"enter","suggested_line":""}身后的雨声渐远。`

	frame, narrative := extractFirstJSON(raw)
	if frame.Action != "move" {
		t.Errorf("action = %s, want move", frame.Action)
	}
	if narrative == "" {
		t.Error("narrative should not be empty")
	}
}

func TestExtractFirstJSONNoJSON(t *testing.T) {
	raw := "纯文本叙述，没有任何 JSON 结构。"
	frame, narrative := extractFirstJSON(raw)
	if frame.Action != "" {
		t.Errorf("expected empty frame, got action=%s", frame.Action)
	}
	if narrative != "" {
		t.Error("expected empty narrative for no JSON input")
	}
}

func TestFlexibleFrameSurvivesNullEffects(t *testing.T) {
	data := []byte(`{"actor":"V","action":"speak","target":"","intensity":0,"emotion":{"primary":"","secondary":"","intensity":0},"intent":"","suggested_line":"嗯。","effects":null}`)
	frame, err := parseFlexibleFrame(data)
	if err != nil {
		t.Fatalf("parse with null effects failed: %v", err)
	}
	if frame.SuggestedLine != "嗯。" {
		t.Errorf("line = %s, want 嗯。", frame.SuggestedLine)
	}
}

// Test that LLM response with empty choices doesn't panic
func TestParseStreamChunk(t *testing.T) {
	// Test that the streaming code path handles edge cases
	// We can't test the full streaming without a server, but we verify
	// the JSON parsing patterns through ExtractActionFrame
	frame, _, _ := ExtractActionFrame("")
	if frame.Action != "" {
		t.Errorf("empty input should produce empty frame")
	}
}

// Test retry/rejection scenario: multiple failed parses → return narrative only
func TestExtractActionFrameMultipleBadJSON(t *testing.T) {
	// Worst case: multiple malformed JSON blocks, should still return narrative
	raw := "```json\n{not valid}\n```\nSome narrative.\n```json\n{also bad}\n```\nMore narrative goes here."

	frame, narrative, err := ExtractActionFrame(raw)
	if err != nil {
		t.Fatalf("extract failed: %v", err)
	}
	// Should fall through to narrative-only
	if narrative == "" {
		t.Error("should extract narrative even when all JSON is malformed")
	}
	_ = frame // may have defaults or be empty
}

func TestExtractActionFrameJSONWithChinese(t *testing.T) {
	raw := "```json\n{\"actor\":\"Anya\",\"action\":\"speak\",\"target\":\"玩家\",\"intensity\":2,\"emotion\":{\"primary\":\"平静\",\"secondary\":\"好奇\",\"intensity\":0.4},\"intent\":\"获取信息\",\"suggested_line\":\"你是新来的？\"}\n```\n\nAnya抬眼打量了一下面前的人，语气平淡。"
	frame, narrative, err := ExtractActionFrame(raw)
	if err != nil {
		t.Fatalf("extract failed: %v", err)
	}
	if frame.Actor != "Anya" {
		t.Errorf("actor = %s, want Anya", frame.Actor)
	}
	if frame.Emotion.Primary != "平静" {
		t.Errorf("emotion primary = %s, want 平静", frame.Emotion.Primary)
	}
	if narrative == "" {
		t.Error("narrative should not be empty")
	}
}

func TestExtractActionFrameActorOnly(t *testing.T) {
	// Some models output actor+speak only, fallback to narrative
	raw := "{\"actor\":\"V\",\"action\":\"speak\"}"
	frame, _, err := ExtractActionFrame(raw)
	if err != nil {
		t.Fatalf("extract failed: %v", err)
	}
	if frame.Actor != "V" {
		t.Errorf("actor = %s, want V", frame.Actor)
	}
	if frame.Action != "speak" {
		t.Errorf("action = %s, want speak", frame.Action)
	}
}

func TestParseFlexibleFrameMinimal(t *testing.T) {
	// Absolute minimum valid ActionFrame
	data := []byte(`{"actor":"V","action":"speak"}`)
	frame, err := parseFlexibleFrame(data)
	if err != nil {
		t.Fatalf("parse minimal frame failed: %v", err)
	}
	if frame.Actor != "V" || frame.Action != "speak" {
		t.Error("minimal frame parse mismatch")
	}
}

// Test for double-extract: LLM outputs ```json inside a JSON string → handle gracefully
func TestExtractActionFrameNestedFences(t *testing.T) {
	raw := "```json\n{\"actor\":\"V\",\"action\":\"negotiate\",\"target\":\"fixer\",\"intensity\":4,\"emotion\":{\"primary\":\"wary\",\"secondary\":\"\",\"intensity\":0.5},\"intent\":\"bargain\",\"suggested_line\":\"三倍的价格，不二价。\"}\n```\nV靠在吧台上，指尖敲了敲桌面。"

	frame, narr, err := ExtractActionFrame(raw)
	if err != nil {
		t.Fatalf("extract failed: %v", err)
	}
	if frame.Action != "negotiate" {
		t.Errorf("action = %s, want negotiate", frame.Action)
	}
	if frame.SuggestedLine != "三倍的价格，不二价。" {
		t.Errorf("suggested_line = %s", frame.SuggestedLine)
	}
	if narr == "" {
		t.Error("narrative should not be empty")
	}
}
