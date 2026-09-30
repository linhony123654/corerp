package decision

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/endpointpolicy"
)

func TestDecisionSchemaGroundingChoicesMatchAuthorizedContext(t *testing.T) {
	for _, withEvidence := range []bool{false, true} {
		t.Run(map[bool]string{false: "no evidence", true: "sourced earlier exchange"}[withEvidence], func(t *testing.T) {
			input := contextFixture()
			input.SpeechEventID = ""
			want := []any{}
			if withEvidence {
				input.SpeechEventID = "event-current"
				input.RelevantDialogue = []core.RPDecisionExchange{{Dialogue: []core.RPDecisionDialogue{{EventID: "event-before", Text: "那本书的末页缺了。"}}}}
				input.RecentPrivateDecisions = []core.RPDecisionPrivateMemory{{SourceEventID: "event-prior-intent", Private: core.RPDecisionPrivate{Intent: "想先听完再作决定", BasisEventIDs: []string{"event-before"}}}}
				want = []any{"src_1", "src_2", "src_3"}
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request map[string]any
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
					return
				}
				schema := request["response_format"].(map[string]any)["json_schema"].(map[string]any)["schema"].(map[string]any)
				properties := schema["properties"].(map[string]any)
				variants := properties["observable"].(map[string]any)["anyOf"].([]any)
				fields := variants[0].(map[string]any)["properties"].(map[string]any)
				if fields["text"].(map[string]any)["maxLength"] != float64(2000) {
					t.Error("speech bound absent from schema")
				}
				private := properties["private"].(map[string]any)["properties"].(map[string]any)
				basis := private["basis_event_ids"].(map[string]any)
				items := basis["items"].(map[string]any)
				if basis["maxItems"] != float64(len(want)) {
					t.Error("unexpected allowed basis count")
				}
				if withEvidence {
					if !reflect.DeepEqual(items["enum"], want) {
						t.Errorf("grounding choices %v, want %v", items["enum"], want)
					}
				} else if _, ok := items["enum"]; ok {
					t.Error("emitted invalid empty enum")
				}
				modelResponse(w, `{"private":{"intent":"静听","emotion":"平静","relationship_stance":"礼貌","basis_event_ids":[]},"observable":{"action":"silence","expression_code":"none"}}`, "stop")
			}))
			defer server.Close()
			provider, err := NewChatProvider(Config{Endpoint: server.URL + "/v1/chat/completions", Model: "fixture", EndpointPolicy: endpointpolicy.TestLocalhostPolicy()})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := provider.Propose(context.Background(), input); err != nil {
				t.Fatal(err)
			}
		})
	}
}
