package core

import "testing"

func TestParseRPInteractionSafeModesAndOrder(t *testing.T) {
	places := []RPInteractionPlace{{ID: "cafe", Name: "咖啡馆"}, {ID: "home", Name: "家"}}
	cases := []struct {
		input, mode, kind string
		steps             []RPInteractionStep
	}{
		{"你好，最近怎么样？", "DIALOGUE", "DIALOGUE", []RPInteractionStep{{Kind: "speech", SpeechText: "你好，最近怎么样？"}}},
		{"去咖啡馆", "AUTO", "ACTION", []RPInteractionStep{{Kind: "move", TargetPlaceID: "cafe"}}},
		{"去咖啡馆，随后说「你好」", "SCENE", "MIXED", []RPInteractionStep{{Kind: "move", TargetPlaceID: "cafe"}, {Kind: "speech", SpeechText: "你好"}}},
		{"等一小时，然后说「我回来了」", "AUTO", "MIXED", []RPInteractionStep{{Kind: "wait", WaitHours: 1}, {Kind: "speech", SpeechText: "我回来了"}}},
		{"去咖啡馆", "DIALOGUE", "DIALOGUE", []RPInteractionStep{{Kind: "speech", SpeechText: "去咖啡馆"}}},
		{"说「你好」", "SCENE", "DIALOGUE", []RPInteractionStep{{Kind: "speech", SpeechText: "你好"}}},
	}
	for _, tc := range cases {
		plan, err := ParseRPInteraction(tc.input, tc.mode, places)
		if err != nil || plan.Kind != tc.kind || len(plan.Steps) != len(tc.steps) {
			t.Fatalf("parse %q/%q: %+v %v", tc.input, tc.mode, plan, err)
		}
		for i, want := range tc.steps {
			if plan.Steps[i] != want {
				t.Fatalf("parse %q step%d: got %+v want %+v", tc.input, i, plan.Steps[i], want)
			}
		}
	}
}

func TestParseRPInteractionClarifiesInsteadOfActing(t *testing.T) {
	places := []RPInteractionPlace{{ID: "first", Name: "店"}, {ID: "second", Name: "店"}}
	for _, input := range []string{"继续", "去店", "去陌生地点", "去店，说「你好", "等待十小时", "去年你去哪了？", "我递给她杯子", "我看了她一眼，没有说话"} {
		plan, err := ParseRPInteraction(input, "AUTO", places)
		if err != nil || plan.Kind != "CLARIFICATION" || len(plan.Steps) != 0 || plan.Clarification == "" {
			t.Fatalf("ambiguous %q produced action: %+v %v", input, plan, err)
		}
	}
	if _, err := ParseRPInteraction("hello", "MIXED", places); !HasCode(err, CodeInvalidArgument) {
		t.Fatalf("accepted internal plan kind as user mode: %v", err)
	}
	if _, err := ParseRPInteraction("  ", "AUTO", places); !HasCode(err, CodeInvalidArgument) {
		t.Fatalf("accepted empty input: %v", err)
	}
}
