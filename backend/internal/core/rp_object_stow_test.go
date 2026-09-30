package core

import "testing"

func TestRPObjectStowIsExplicitOnlyAndHasNoCallerSelectedDestination(t *testing.T) {
	request := RPObjectRequest{PrincipalID: "principal_player", SessionID: "session", ExpectedCursor: 1, IdempotencyKey: "return-object", Action: "stow", ObjectID: "known-object"}
	if err := request.Validate(); err != nil {
		t.Fatalf("valid explicit stow rejected: %v", err)
	}
	for _, invalid := range []RPObjectRequest{
		{PrincipalID: request.PrincipalID, SessionID: request.SessionID, ExpectedCursor: 1, IdempotencyKey: request.IdempotencyKey, Action: "stow", ObjectID: ""},
		{PrincipalID: request.PrincipalID, SessionID: request.SessionID, ExpectedCursor: 1, IdempotencyKey: request.IdempotencyKey, Action: "stow", ObjectID: request.ObjectID, SourceID: "stock"},
		{PrincipalID: request.PrincipalID, SessionID: request.SessionID, ExpectedCursor: 1, IdempotencyKey: request.IdempotencyKey, Action: "stow", ObjectID: request.ObjectID, AnchorID: "elsewhere"},
		{PrincipalID: request.PrincipalID, SessionID: request.SessionID, ExpectedCursor: 1, IdempotencyKey: request.IdempotencyKey, Action: "stow", ObjectID: request.ObjectID, TargetEntityID: "recipient"},
	} {
		if err := invalid.Validate(); !HasCode(err, CodeInvalidArgument) {
			t.Fatalf("stow accepted unsupported side effects: %+v %v", invalid, err)
		}
	}
	input := RPInteractionUnderstandingInput{Mode: "AUTO", Text: "我把这件物品收回去。", Objects: []RPInteractionObject{{ID: request.ObjectID, Name: "物品", AllowedActions: []string{"stow"}}}}
	proposal := RPInteractionPlan{Mode: "AUTO", Kind: "ACTION", Steps: []RPInteractionStep{{Kind: "object", ObjectAction: "stow", ObjectID: request.ObjectID}}}
	if err := ValidateRPInteractionProposal(input, proposal); !HasCode(err, CodeInvalidArgument) {
		t.Fatalf("model-controlled plan gained explicit inventory authority: %v", err)
	}
}
