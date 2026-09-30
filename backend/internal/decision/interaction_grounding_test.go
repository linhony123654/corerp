package decision

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/endpointpolicy"
)

func TestChatInteractionRejectsReachableButUnnamedCharacterTravel(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		modelResponse(w, `{"kind":"MIXED","steps":[{"kind":"move","target_place_id":"home"},{"kind":"speech","speech_text":"喝一点吧。"}],"clarification":""}`, "stop")
	}))
	defer server.Close()
	provider, err := NewChatProvider(Config{Endpoint: server.URL + "/v1/chat/completions", Model: "fixture", Attempts: 1, ProposalRepairs: 1, EndpointPolicy: endpointpolicy.TestLocalhostPolicy()})
	if err != nil {
		t.Fatal(err)
	}
	input := interactionFixtureInput("我把桌上的杯子推到邻居近旁，然后说「喝一点吧。」")
	input.ReachablePlaces = []core.RPInteractionPlace{{ID: "home", Name: "新世界的家"}}
	trace := &core.RPProviderTrace{}
	plan, err := provider.UnderstandInteraction(core.WithRPProviderTrace(context.Background(), trace), input)
	if err == nil || len(plan.Steps) != 0 || calls != 2 || trace.AttemptCount() != 2 {
		t.Fatalf("misgrounded travel was accepted: %+v err=%v calls=%d attempts=%d", plan, err, calls, trace.AttemptCount())
	}
	var failure *Error
	if !errors.As(err, &failure) || failure.RPDecisionFailureCode() != "proposal_movement" {
		t.Fatalf("unexpected redacted failure classification: %v", err)
	}
}
