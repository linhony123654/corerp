package decision

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"

	"corerp.local/backend/internal/endpointpolicy"
)

func TestDecisionPrivateSingleLineContractAndBoundedRepair(t *testing.T) {
	for _, tc := range []struct {
		name, field, value string
	}{
		{"intent LF", "intent", "private-secret\nsecond line"},
		{"emotion CR", "emotion", "private-secret\rsecond line"},
		{"stance NUL", "relationship_stance", "private-secret\x00second line"},
		{"intent length", "intent", strings.Repeat("私", 161)},
		{"emotion length", "emotion", strings.Repeat("私", 81)},
		{"stance length", "relationship_stance", strings.Repeat("私", 81)},
	} {
		for _, repaired := range []bool{false, true} {
			t.Run(tc.name+map[bool]string{false: "/still invalid", true: "/repaired"}[repaired], func(t *testing.T) {
				t.Setenv("CORERP_DECISION_DEBUG", "1")
				log, err := os.CreateTemp(t.TempDir(), "private-shape-*.log")
				if err != nil {
					t.Fatal(err)
				}
				previousStderr := os.Stderr
				os.Stderr = log
				defer func() { os.Stderr = previousStderr; _ = log.Close() }()
				var private map[string]any
				_ = json.Unmarshal([]byte(wirePrivateFixture), &private)
				private[tc.field] = tc.value
				badPrivate, _ := json.Marshal(private)
				good := wireDecision(`{"action":"respond","text":"我在。","introduce_self":false,"expression_code":"none"}`)
				bad := strings.Replace(good, wirePrivateFixture, string(badPrivate), 1)
				count := 0
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					count++
					var body map[string]any
					if json.NewDecoder(r.Body).Decode(&body) != nil {
						t.Error("bad request")
						return
					}
					messages := body["messages"].([]any)
					var got decisionContext
					if json.Unmarshal([]byte(messages[1].(map[string]any)["content"].(string)), &got) != nil || !reflect.DeepEqual(got.Character, contextFixture()) {
						t.Error("repair changed authorized character input")
					}
					parameters := body["tools"].([]any)[0].(map[string]any)["function"].(map[string]any)["parameters"].(map[string]any)
					fields := parameters["properties"].(map[string]any)["private"].(map[string]any)["properties"].(map[string]any)
					for name, limit := range map[string]float64{"intent": 160, "emotion": 80, "relationship_stance": 80} {
						field := fields[name].(map[string]any)
						if field["maxLength"] != limit || !strings.Contains(field["description"].(string), "No NUL, CR or LF") {
							t.Error("private field contract lost local bounds")
						}
					}
					if count == 2 && !strings.Contains(messages[2].(map[string]any)["content"].(string), "no NUL, CR or LF") {
						t.Error("repair omitted the single-line constraint")
					}
					arguments := bad
					if count == 2 && repaired {
						arguments = good
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]any{"content": "", "tool_calls": []any{proposalTool(arguments)}}}}})
				}))
				defer server.Close()
				p, err := NewChatProvider(Config{Endpoint: server.URL + "/v1/chat/completions", Model: "fixture", DecisionFormat: "tool_call", EndpointPolicy: endpointpolicy.TestLocalhostPolicy()})
				if err != nil {
					t.Fatal(err)
				}
				proposal, err := p.Propose(context.Background(), contextFixture())
				if count != 2 || repaired && (err != nil || proposal.Text != "我在。") || !repaired && (err == nil || proposal.Action != "") {
					t.Fatalf("private metadata was salvaged or repair unbounded: calls=%d err=%v", count, err)
				}
				_ = log.Sync()
				captured, readErr := os.ReadFile(log.Name())
				if readErr != nil || !strings.Contains(string(captured), `kind="private field shape"`) || strings.Contains(string(captured), "private-secret") || strings.Contains(string(captured), "私") {
					t.Fatal("private diagnostic absent or leaked private text")
				}
			})
		}
	}
}
