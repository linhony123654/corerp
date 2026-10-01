package narrative

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/endpointpolicy"
)

const validPlan = `{"pov":"first_person","tense":"past","verbosity":"detailed","dialogue_ratio":100,"description_density":80,"narrative_pack_ref":"builtin/dialogue@1","unsupported_instructions":false}`

func envelope(content string) string {
	encoded, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]any{"content": content}}}})
	return string(encoded)
}

func TestChatStylePlannerHTTPExecutesPreferenceWithoutWorldContext(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("Authorization") != "Bearer fixture-secret" {
			t.Error("wrong explicit narrative credential")
		}
		var body map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		if string(body["stream"]) != "false" || string(body["store"]) != "false" {
			t.Error("planner request mode differs")
		}
		var messages []struct{ Role, Content string }
		if err := json.Unmarshal(body["messages"], &messages); err != nil || len(messages) != 2 {
			t.Error("invalid prompt envelope")
			return
		}
		if !strings.Contains(messages[0].Content, "not story text") || !strings.Contains(messages[1].Content, "第一人称") {
			t.Error("custom style instruction missing")
		}
		for _, private := range []string{"accepted-secret-speech", "committed_facts", "event-secret", "character", "legal_actions"} {
			if strings.Contains(messages[1].Content, private) {
				t.Errorf("world context disclosed: %s", private)
			}
		}
		if calls.Load() == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(429)
			fmt.Fprint(w, "remote secret body")
			return
		}
		fmt.Fprint(w, envelope(validPlan))
	}))
	defer server.Close()
	planner, err := NewChatStylePlanner(Config{Endpoint: server.URL + "/v1/chat/completions", EndpointPolicy: endpointpolicy.TestLocalhostPolicy(), Model: "fixture-model", APIKey: "fixture-secret"})
	if err != nil {
		t.Fatal(err)
	}
	if metadata := planner.ProviderMetadata(); metadata.Kind != "style_planner" || metadata.Model != "fixture-model" {
		t.Fatalf("unexpected provider metadata: %+v", metadata)
	}
	input := core.RPNarrativeInput{ControlledEntityID: "lin", Style: core.DefaultRPStyle(), Facts: []core.RPNarrativeFact{{EventID: "event-secret", ActorID: "lin", ActorName: "Lin", Action: "speak", Text: "accepted-secret-speech", PlaceName: "咖啡馆"}}}
	input.Style.ProseInstructions = "用第一人称，过去时，台词另起一行。"
	provider := core.PlannedRPNarrativeProvider{Planner: planner}
	var chunks []core.RPNarrativeChunk
	view, err := provider.RenderStream(context.Background(), input, func(c core.RPNarrativeChunk) error { chunks = append(chunks, c); return nil })
	if err != nil || calls.Load() != 2 || len(chunks) != 1 || !strings.Contains(view.Lines[0], "当时") || !strings.Contains(view.Lines[0], "我说：\n「accepted-secret-speech」") {
		t.Fatalf("real HTTP plan not executed: %+v %v calls=%d", view, err, calls.Load())
	}
	if view.EventIDs[0] != "event-secret" || input.Style.POV != "second_person" {
		t.Fatal("plan mutated input or source")
	}
}

func TestChatStylePlannerRejectsMalformedAndUnsafePlans(t *testing.T) {
	for name, content := range map[string]string{
		"unknown":    strings.Replace(validPlan, `"pov":`, `"text":"invented event","pov":`, 1),
		"duplicate":  strings.Replace(validPlan, `"pov":`, `"pov":"third_person","pov":`, 1),
		"missing":    strings.Replace(validPlan, `,"unsupported_instructions":false`, "", 1),
		"null":       strings.Replace(validPlan, `"unsupported_instructions":false`, `"unsupported_instructions":null`, 1),
		"wrong-type": strings.Replace(validPlan, `"dialogue_ratio":100`, `"dialogue_ratio":"100"`, 1),
		"fraction":   strings.Replace(validPlan, `"dialogue_ratio":100`, `"dialogue_ratio":99.5`, 1),
		"range":      strings.Replace(validPlan, `"description_density":80`, `"description_density":101`, 1),
		"pack":       strings.Replace(validPlan, `builtin/dialogue@1`, `external/private`, 1),
		"trailing":   validPlan + ` {}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parsePlan(content, core.DefaultRPStyle()); err == nil {
				t.Fatal("invalid remote plan accepted")
			}
		})
	}
	for name, response := range map[string]string{
		"schema":    envelope(`{"text":"remote-secret-model-body"}`),
		"refusal":   `{"choices":[{"finish_reason":"stop","message":{"refusal":"remote-secret-model-body","content":""}}]}`,
		"tools":     `{"choices":[{"finish_reason":"stop","message":{"tool_calls":[{}],"content":""}}]}`,
		"truncated": `{"choices":[{"finish_reason":"length","message":{"content":""}}]}`,
		"oversize":  strings.Repeat("x", (16<<10)+1),
	} {
		t.Run(name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); fmt.Fprint(w, response) }))
			defer server.Close()
			p, err := NewChatStylePlanner(Config{Endpoint: server.URL + "/v1/chat/completions", EndpointPolicy: endpointpolicy.TestLocalhostPolicy(), Model: "fixture", APIKey: "private-key"})
			if err != nil {
				t.Fatal(err)
			}
			_, err = p.PlanStyle(context.Background(), core.DefaultRPStyle())
			if err == nil || calls.Load() != 1 || strings.Contains(err.Error(), "private-key") || strings.Contains(err.Error(), "remote-secret-model-body") || strings.Contains(err.Error(), server.URL) {
				t.Fatalf("failure retried or leaked remote data: %v calls=%d", err, calls.Load())
			}
		})
	}
}

func TestChatStylePlannerNoRedirectAndBoundedCancellation(t *testing.T) {
	var forwarded atomic.Int32
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { forwarded.Add(1) }))
	defer destination.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, destination.URL, 307) }))
	defer redirect.Close()
	p, err := NewChatStylePlanner(Config{Endpoint: redirect.URL + "/v1/chat/completions", EndpointPolicy: endpointpolicy.TestLocalhostPolicy(), Model: "fixture", APIKey: "private-key"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.PlanStyle(context.Background(), core.DefaultRPStyle()); err == nil || forwarded.Load() != 0 {
		t.Fatal("redirect forwarded credentials/context")
	}
	release := make(chan struct{})
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-release }))
	defer slow.Close()
	defer close(release)
	p, err = NewChatStylePlanner(Config{Endpoint: slow.URL + "/v1/chat/completions", EndpointPolicy: endpointpolicy.TestLocalhostPolicy(), Model: "fixture", Timeout: 30 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if _, err := p.PlanStyle(context.Background(), core.DefaultRPStyle()); err == nil || time.Since(start) > time.Second {
		t.Fatal("timeout budget not respected")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := p.PlanStyle(ctx, core.DefaultRPStyle()); err == nil {
		t.Fatal("cancelled request succeeded")
	}
}
