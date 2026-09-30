package core

import "testing"

func TestRPInteractionObjectMoveRejectsCurrentAnchorBeforeAcceptingPlan(t *testing.T) {
	input := RPInteractionUnderstandingInput{
		Text: "把杯子从桌边推到近旁", Mode: "AUTO",
		Objects: []RPInteractionObject{{ID: "cup", Name: "杯子", PhysicalState: "placed", AnchorID: "table", AllowedActions: []string{"move"}}},
		Anchors: []RPInteractionAnchor{{ID: "table", Name: "桌边"}, {ID: "near", Name: "近旁"}},
	}
	plan := RPInteractionPlan{Mode: "AUTO", Kind: "ACTION", Steps: []RPInteractionStep{{Kind: "object", ObjectAction: "move", ObjectID: "cup", AnchorID: "table"}}}
	if err := ValidateRPInteractionProposal(input, plan); !HasCode(err, CodeInvalidArgument) {
		t.Fatalf("current anchor was accepted as a new destination: %v", err)
	}
	plan.Steps[0].AnchorID = "near"
	if err := ValidateRPInteractionProposal(input, plan); err != nil {
		t.Fatalf("different observed destination was rejected: %v", err)
	}
}

func TestRPInteractionCharacterTravelRequiresPlayerNamedPlace(t *testing.T) {
	input := RPInteractionUnderstandingInput{
		Text: "我把桌上的杯子推到邻居近旁，然后说「喝一点吧。」", Mode: "AUTO",
		ReachablePlaces: []RPInteractionPlace{{ID: "home", Name: "新世界的家"}},
	}
	plan := RPInteractionPlan{Mode: "AUTO", Kind: "MIXED", Steps: []RPInteractionStep{
		{Kind: "move", TargetPlaceID: "home"}, {Kind: "speech", SpeechText: "喝一点吧。"},
	}}
	if err := ValidateRPInteractionProposal(input, plan); !HasCode(err, CodeInvalidArgument) {
		t.Fatalf("object movement was misinterpreted as authorized character travel: %v", err)
	}
	input.Text = "我去新世界的家，然后说「到了。」"
	plan.Steps[1].SpeechText = "到了。"
	if err := ValidateRPInteractionProposal(input, plan); err != nil {
		t.Fatalf("explicit destination was rejected: %v", err)
	}
}

func TestRPInteractionProposalPreservesWholePlayerSpeech(t *testing.T) {
	base := RPInteractionUnderstandingInput{
		Mode:    "AUTO",
		Objects: []RPInteractionObject{{ID: "cup", Name: "杯子", PhysicalState: "placed", AnchorID: "table", AllowedActions: []string{"move"}}},
		Anchors: []RPInteractionAnchor{{ID: "near", Name: "近旁"}, {ID: "table", Name: "桌边"}},
	}
	for _, tc := range []struct {
		name, text, kind, speech string
		valid                    bool
	}{
		{"dialogue exact", "想你了。", "DIALOGUE", "想你了。", true},
		{"dialogue truncated", "想你了。", "DIALOGUE", "想你", false},
		{"mixed corner quote", "把杯子推近，然后说「喝一点吧。」", "MIXED", "喝一点吧。", true},
		{"mixed straight quote", `把杯子推近，然后说"喝一点吧。"`, "MIXED", "喝一点吧。", true},
		{"mixed curved quote", "把杯子推近，然后说“喝一点吧。”", "MIXED", "喝一点吧。", true},
		{"mixed partial quote", "把杯子推近，然后说「喝一点吧。」", "MIXED", "喝一点", false},
		{"mixed invented speech", "把杯子推近，然后说「喝一点吧。」", "MIXED", "把杯子推近", false},
		{"mixed unquoted", "把杯子推近，然后说喝一点吧。", "MIXED", "喝一点吧。", false},
		{"mixed ambiguous quotes", "把「杯子」推近，然后说「喝一点吧。」", "MIXED", "喝一点吧。", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := base
			input.Text = tc.text
			plan := RPInteractionPlan{Mode: "AUTO", Kind: tc.kind, Steps: []RPInteractionStep{{Kind: "speech", SpeechText: tc.speech}}}
			if tc.kind == "MIXED" {
				plan.Steps = append([]RPInteractionStep{{Kind: "object", ObjectAction: "move", ObjectID: "cup", AnchorID: "near"}}, plan.Steps...)
			}
			err := ValidateRPInteractionProposal(input, plan)
			if (err == nil) != tc.valid {
				t.Fatalf("proposal validity %v, expected %v: %v", err == nil, tc.valid, err)
			}
		})
	}
}
