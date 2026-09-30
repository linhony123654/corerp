package decision

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/endpointpolicy"
)

func typedInteractionReply(kind string, steps ...map[string]any) string {
	value, _ := json.Marshal(map[string]any{"kind": kind, "steps": steps, "clarification": ""})
	return string(value)
}

func typedInteractionProvider(t *testing.T, response string) (*ChatProvider, func()) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { modelResponse(w, response, "stop") }))
	provider, err := NewChatProvider(Config{Endpoint: server.URL + "/v1/chat/completions", Model: "fixture", EndpointPolicy: endpointpolicy.TestLocalhostPolicy()})
	if err != nil {
		server.Close()
		t.Fatal(err)
	}
	return provider, server.Close
}

func TestChatInteractionV2TypedActionsAreOnlyAuthorizedProposals(t *testing.T) {
	input := interactionFixtureInput("把这件东西推到另一侧，然后说「好了。」")
	input.Objects = []core.RPInteractionObject{{ID: "item-owned", Name: "有来源的物品", AllowedActions: []string{"move"}}}
	input.Anchors = []core.RPInteractionAnchor{{ID: "anchor-reachable", Name: "另一侧"}}
	input.Offers = []core.RPInteractionOffer{{ID: "offer-to-me", ObjectID: "item-other", AllowedActions: []string{"accept", "refuse"}}}
	move := interactionFixtureStep("object")
	move["object_action"], move["object_id"], move["anchor_id"] = "move", "item-owned", "anchor-reachable"
	speech := interactionFixtureStep("speech")
	speech["speech_text"] = "好了。"
	look := interactionFixtureStep("nonverbal")
	look["nonverbal_action"], look["target_entity_id"] = "look_at", "visible-entity"
	accept := interactionFixtureStep("object")
	accept["object_action"], accept["offer_id"] = "accept", "offer-to-me"
	cases := []struct {
		name, kind, text, reply string
		valid                   bool
	}{
		{"move then own words", "MIXED", input.Text, typedInteractionReply("MIXED", move, speech), true},
		{"visible silent look", "ACTION", "我看了她一眼", typedInteractionReply("ACTION", look), true},
		{"my own offer response", "ACTION", "我接受这个提议", typedInteractionReply("ACTION", accept), true},
		{"fabricated item", "MIXED", input.Text, typedInteractionReply("MIXED", func() map[string]any {
			copy := interactionFixtureStep("object")
			copy["object_action"], copy["object_id"], copy["anchor_id"] = "move", "invented", "anchor-reachable"
			return copy
		}(), speech), false},
		{"fabricated reach point", "MIXED", input.Text, typedInteractionReply("MIXED", func() map[string]any {
			copy := interactionFixtureStep("object")
			copy["object_action"], copy["object_id"], copy["anchor_id"] = "move", "item-owned", "unseen"
			return copy
		}(), speech), false},
		{"unseen silent target", "ACTION", "我看了她一眼", typedInteractionReply("ACTION", func() map[string]any {
			copy := interactionFixtureStep("nonverbal")
			copy["nonverbal_action"], copy["target_entity_id"] = "look_at", "private-entity"
			return copy
		}()), false},
		{"NPC consent invented", "ACTION", "她接过去了", typedInteractionReply("ACTION", func() map[string]any {
			copy := interactionFixtureStep("object")
			copy["object_action"], copy["offer_id"] = "accept", "offer-for-NPC"
			return copy
		}()), false},
		{"offer not a transfer", "ACTION", "我递给她", typedInteractionReply("ACTION", func() map[string]any {
			copy := interactionFixtureStep("object")
			copy["object_action"], copy["offer_id"] = "receive", "offer-to-me"
			return copy
		}()), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input.Text = tc.text
			provider, closeServer := typedInteractionProvider(t, tc.reply)
			defer closeServer()
			plan, err := provider.UnderstandInteraction(context.Background(), input)
			if tc.valid && (err != nil || plan.Kind != tc.kind) {
				t.Fatalf("authorized typed proposal failed: %+v %v", plan, err)
			}
			if !tc.valid && (err == nil || len(plan.Steps) != 0) {
				t.Fatalf("untrusted model proposal gained authority: %+v %v", plan, err)
			}
		})
	}
}
