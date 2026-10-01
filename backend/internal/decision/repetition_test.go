package decision

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/endpointpolicy"
)

func TestNPCReplyRepetitionIsDiagnosticWithoutDelayingValidSpeech(t *testing.T) {
	for _, tc := range []struct{ name, question, action string }{
		{"recall", "你刚才答应我的是什么？", "respond"},
		{"other-question", "那明天呢？", "respond"},
		{"repeated-refusal", "你还是不愿意吗？", "refuse"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := contextFixture()
			input.PlayerSpeechText = tc.question
			const answer = "我答应在这里陪你，你愿意说我就听着。"
			input.RecentDialogue = []core.RPDecisionDialogue{{SpeakerEntityID: input.NPCEntityID, Text: answer, EventID: "accepted-prior"}}
			if !repeatsAcceptedNPCReply(input, core.RPDecisionProposal{Action: tc.action, Text: answer}) {
				t.Fatal("fixture must produce the repetition diagnostic")
			}
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				call := calls.Add(1)
				if call > 1 {
					// A semantic rewrite must not turn an already valid response
					// into a timeout. Model the slow second call from the live probe.
					select {
					case <-r.Context().Done():
					case <-time.After(time.Second):
					}
					return
				}
				var request struct {
					Messages []struct{ Content string } `json:"messages"`
				}
				if json.NewDecoder(r.Body).Decode(&request) != nil || len(request.Messages) != 2 {
					t.Error("unexpected initial decision request")
				}
				modelResponse(w, `{"private":{"intent":"回顾已经说过的话","emotion":"平静","relationship_stance":"保持当前立场","basis_event_ids":["accepted-prior"]},"observable":{"action":"`+tc.action+`","text":"`+answer+`","speech_tone":"none","introduce_self":false,"expression_code":"none"}}`, "stop")
			}))
			defer server.Close()
			provider, err := NewChatProvider(Config{Endpoint: server.URL + "/v1/chat/completions", Model: "fixture", Timeout: 300 * time.Millisecond, EndpointPolicy: endpointpolicy.TestLocalhostPolicy()})
			if err != nil {
				t.Fatal(err)
			}
			trace := &core.RPProviderTrace{}
			proposal, err := provider.Propose(core.WithRPProviderTrace(context.Background(), trace), input)
			if err != nil || calls.Load() != 1 || trace.AttemptCount() != 1 {
				t.Fatalf("soft diagnostic delayed or blocked valid speech: calls=%d/%d err=%v", calls.Load(), trace.AttemptCount(), err)
			}
			if proposal.Action != tc.action || proposal.Text != answer || proposal.Private == nil || len(proposal.Private.BasisEventIDs) != 1 || proposal.Private.BasisEventIDs[0] != "accepted-prior" {
				t.Fatalf("the grounded first proposal was changed: %+v", proposal)
			}
			if err := core.ValidateRPDecisionProposal(input, proposal); err != nil {
				t.Fatalf("world/source validation was weakened: %v", err)
			}
		})
	}
}

func TestNPCReplyGuardCatchesParaphrasedRecyclingWithoutSuppressingNewAnswer(t *testing.T) {
	if !repliesAreRepetitive("我一会儿要去街口看看。", "我一会要去街口看看！") {
		t.Fatal("near-identical reply was missed")
	}
	if repliesAreRepetitive("我昨晚在店里帮忙。", "你说的那件去年往事，我没有可靠线索。") {
		t.Fatal("unrelated answer was treated as repetition")
	}
}

func TestNPCReplyGuardDiagnosesRecurringAdviceInsideDifferentAnswers(t *testing.T) {
	input := contextFixture()
	input.PlayerSpeechText = "你手里的活忙完了吗？"
	input.RecentDialogue = []core.RPDecisionDialogue{
		{SpeakerEntityID: input.NPCEntityID, Text: "活还没有忙完，二爷先坐下歇歇再说。"},
		{SpeakerEntityID: input.NPCEntityID, Text: "那边的账还要核对，你先坐下歇歇，容我想想。"},
	}
	if !repeatsAcceptedNPCReply(input, core.RPDecisionProposal{Action: "respond", Text: "这事我并不清楚，你先坐下歇歇，我去问明白。"}) {
		t.Fatal("recurring stock advice escaped repair")
	}
	if repeatsAcceptedNPCReply(input, core.RPDecisionProposal{Action: "respond", Text: "还没有，账目上有一处数字需要再核对。"}) {
		t.Fatal("new specific answer was treated as repeated advice")
	}
}
