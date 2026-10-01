package decision

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/endpointpolicy"
)

func TestNPCSpeechQualityHintsDoNotRejectOrRewriteLiteralData(t *testing.T) {
	for _, tc := range []struct {
		name, text, player string
		want               bool
	}{
		{"committed nested-quote-run residue", `我是黛玉。方才没留意你连我也认不得了。},"`, "你是谁", true},
		{"frozen Golden residue", "没事请回。我不随便留客。} } scopetext invalid || ", "来看看你", true},
		{"ordinary answer", "先等一等，我还要核对这处数字。", "忙完了吗", false},
		{"literal JSON", `这是资料。{"values":[1,2]}`, "资料是什么", false},
		{"quoted closing braces", "你问的是这句：「好。}}」", "说的是什么", false},
		{"literal punctuation requested", `那个符号的例子是。},"`, `请把 }," 作为例子读出来`, false},
		{"single punctuation", "结束了。}", "结束了吗", false},
		{"balanced object with punctuation", `{ "note": "好了。" }`, "JSON里是什么", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := contextFixture()
			input.PlayerSpeechText = tc.player
			proposal := core.RPDecisionProposal{Action: "respond", Text: tc.text}
			signal, _ := npcReplyQualityFeedback(input, proposal)
			if (signal == "speech_format_residue") != tc.want {
				t.Fatalf("unexpected quality hint %q", signal)
			}
			if err := core.ValidateRPDecisionProposal(input, proposal); err != nil || proposal.Text != tc.text {
				t.Fatalf("soft quality hint changed or blocked literal speech: %v", err)
			}
		})
	}
}

func TestNPCSpeechFormattingAndRepetitionShareOnePrecommitRewrite(t *testing.T) {
	const malformed = "我不随便留客。} } scopetext invalid || "
	for _, tc := range []struct{ name, second string }{
		{"fresh", "有话请直说，我这会儿还有账目要看。"},
		{"still-malformed", malformed},
		{"repeated", "你好。"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := contextFixture()
			input.RecentDialogue = []core.RPDecisionDialogue{{SpeakerEntityID: input.NPCEntityID, Text: "你好。", EventID: "prior-hello"}}
			calls := 0
			initialContext := ""
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var body struct {
					Messages []struct{ Content string } `json:"messages"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Messages) != calls+1 {
					t.Errorf("unexpected rewrite request shape: %v", err)
					return
				}
				text := malformed
				if calls == 1 {
					initialContext = body.Messages[1].Content
				} else {
					if body.Messages[1].Content != initialContext || !strings.Contains(body.Messages[2].Content, "structured-output fragments") {
						t.Error("rewrite changed the source context or omitted the fixed contract hint")
					}
					text = tc.second
				}
				encoded, _ := json.Marshal(map[string]any{"private": map[string]any{"intent": "", "emotion": "", "relationship_stance": "", "basis_event_ids": []string{}}, "observable": map[string]any{"action": "respond", "text": text, "speech_tone": "none", "introduce_self": false, "expression_code": "none"}})
				modelResponse(w, string(encoded), "stop")
			}))
			defer server.Close()
			provider, err := NewChatProvider(Config{Endpoint: server.URL + "/v1/chat/completions", Model: "fixture", EndpointPolicy: endpointpolicy.TestLocalhostPolicy()})
			if err != nil {
				t.Fatal(err)
			}
			trace := &core.RPProviderTrace{}
			proposal, err := provider.Propose(core.WithRPProviderTrace(context.Background(), trace), input)
			if err != nil || proposal.Text != tc.second || calls != 2 || trace.AttemptCount() != 2 {
				t.Fatalf("one shared rewrite did not preserve the valid second proposal: %+v, %v, calls=%d", proposal, err, calls)
			}
		})
	}
}
