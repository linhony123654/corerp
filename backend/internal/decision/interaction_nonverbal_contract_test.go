package decision

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestInteractionNonverbalSchemaMatchesExistingCoreActions(t *testing.T) {
	encoded, err := json.Marshal(interactionStepSchema())
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		AnyOf []struct{ Properties map[string]json.RawMessage }
	}
	if err := json.Unmarshal(encoded, &schema); err != nil {
		t.Fatal(err)
	}
	var actions []string
	for _, variant := range schema.AnyOf {
		if raw := variant.Properties["nonverbal_action"]; raw != nil {
			var field struct{ Enum []string }
			if err := json.Unmarshal(raw, &field); err != nil {
				t.Fatal(err)
			}
			actions = field.Enum
		}
	}
	if !reflect.DeepEqual(actions, []string{"look_at", "smile", "nod", "shake_head", "gesture", "turn_away", "frown"}) {
		t.Fatal("AUTO omitted or invented a core action", actions)
	}
	for _, action := range actions {
		request := core.RPNonverbalRequest{PrincipalID: "player", SessionID: "session", ExpectedCursor: 1, IdempotencyKey: "test", Action: action, TargetEntityID: "visible-entity"}
		if action == "gesture" {
			request.GestureCode = "wave"
		}
		if err := request.Validate(); err != nil {
			t.Fatal("AUTO schema invented an unsupported action", action, err)
		}
		if !strings.Contains(interactionInstruction, action) {
			t.Fatal("prompt omitted supported action", action)
		}
	}
	for _, gesture := range []string{"wave", "shrug", "raise_hand", "beckon"} {
		if !strings.Contains(interactionInstruction, gesture) {
			t.Fatal("prompt omitted supported gesture", gesture)
		}
	}
}

func TestInteractionAUTOExistingNonverbalActionsAndGestureCodes(t *testing.T) {
	cases := []struct{ action, gesture string }{
		{"look_at", ""}, {"smile", ""}, {"nod", ""}, {"shake_head", ""}, {"turn_away", ""}, {"frown", ""},
		{"gesture", "wave"}, {"gesture", "shrug"}, {"gesture", "raise_hand"}, {"gesture", "beckon"},
	}
	for _, tc := range cases {
		t.Run(tc.action+"/"+tc.gesture, func(t *testing.T) {
			step := map[string]any{"kind": "nonverbal", "nonverbal_action": tc.action, "target_entity_id": "visible-entity", "gesture_code": tc.gesture}
			p, closeProvider := typedInteractionProvider(t, typedInteractionReply("ACTION", step))
			defer closeProvider()
			plan, err := p.UnderstandInteraction(context.Background(), interactionFixtureInput("我向她做一个无声动作。"))
			if err != nil || plan.Kind != "ACTION" || len(plan.Steps) != 1 || plan.Steps[0].NonverbalAction != tc.action || plan.Steps[0].GestureCode != tc.gesture || plan.Steps[0].SpeechText != "" {
				t.Fatalf("existing nonverbal contract changed: %+v %v", plan, err)
			}
		})
	}
	for _, tc := range []struct{ action, gesture, target string }{
		{"invented", "", "visible-entity"}, {"gesture", "invented", "visible-entity"}, {"nod", "wave", "visible-entity"}, {"look_at", "", ""}, {"frown", "", "unseen-entity"},
	} {
		t.Run("reject/"+tc.action+"/"+tc.gesture+"/"+tc.target, func(t *testing.T) {
			step := map[string]any{"kind": "nonverbal", "nonverbal_action": tc.action, "target_entity_id": tc.target, "gesture_code": tc.gesture}
			p, closeProvider := typedInteractionProvider(t, typedInteractionReply("ACTION", step))
			defer closeProvider()
			if plan, err := p.UnderstandInteraction(context.Background(), interactionFixtureInput("我做一个动作。")); err == nil || len(plan.Steps) != 0 {
				t.Fatalf("AUTO accepted unsupported act/code/target: %+v %v", plan, err)
			}
		})
	}
}
