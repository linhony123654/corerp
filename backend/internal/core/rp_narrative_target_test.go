package core

import (
	"context"
	"strings"
	"testing"
)

func TestDeterministicNarratorUsesOnlyPublicExpressionRecipient(t *testing.T) {
	for _, tc := range []struct{ pov, target, name, expected string }{
		{"second_person", "player", "宝玉", "向你招了招手"},
		{"first_person", "player", "宝玉", "向我招了招手"},
		{"second_person", "anonymous-person", "陌生人", "向陌生人招了招手"},
		{"second_person", "", "", "Cai 招了招手"},
	} {
		style := DefaultRPStyle()
		style.POV = tc.pov
		in := RPNarrativeInput{ControlledEntityID: "player", Style: style, Facts: []RPNarrativeFact{
			{EventID: "gesture", ActorID: "cai", ActorName: "Cai", Action: "expression", ExpressionCode: "beckon", TargetActorID: tc.target, TargetActorName: tc.name},
		}}
		view, err := (DeterministicRPNarrativeProvider{}).Render(context.Background(), in)
		if err != nil || !strings.Contains(strings.Join(view.Lines, ""), tc.expected) || len(view.EventIDs) != 1 || view.EventIDs[0] != "gesture" {
			t.Fatalf("public target or source changed: %+v / %v", view, err)
		}
	}
}
