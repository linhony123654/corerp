package core

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRPNarrativeBudgetExactBytesLayersAndLegacyEncoding(t *testing.T) {
	profile := DefaultRPStyle()
	encoded, err := CanonicalJSON(profile)
	if err != nil || strings.Contains(string(encoded), "context_budget") {
		t.Fatalf("legacy profile encoding changed: %s %v", encoded, err)
	}
	for _, n := range []int{1, 4095, 262145, -1} {
		if _, err := ResolveRPStyle(RPStylePatch{ContextBudgetBytes: &n}); !HasCode(err, CodeInvalidArgument) {
			t.Fatalf("invalid budget %d accepted: %v", n, err)
		}
	}
	low, zero := 4096, 0
	profile, err = ResolveRPStyle(RPStylePatch{ContextBudgetBytes: &low}, RPStylePatch{ContextBudgetBytes: &zero})
	if err != nil || profile.ContextBudgetBytes != 0 {
		t.Fatal("explicit default did not override lower layer")
	}
	in := RPNarrativeInput{Style: profile, Facts: []RPNarrativeFact{{EventID: "one", ActorName: "Lin", Action: "speak", Text: strings.Repeat("你", 1800)}}}
	if err := in.ValidateReadBudget(); err != nil {
		t.Fatal(err)
	}
	in.Style.ContextBudgetBytes = low
	before, _ := json.Marshal(in)
	if err := in.ValidateReadBudget(); !HasCode(err, CodeInvalidArgument) {
		t.Fatalf("UTF-8 context did not exceed4KiB: %v", err)
	}
	after, _ := json.Marshal(in)
	if string(before) != string(after) {
		t.Fatal("budget truncated facts")
	}
	// Six-digit limits stabilize their own JSON width; test the exact byte edge.
	in.Style.ContextBudgetBytes = 100000
	in.Facts[0].Text = strings.Repeat("x", 110000)
	full, _ := json.Marshal(in)
	in.Style.ContextBudgetBytes = len(full)
	if err := in.ValidateReadBudget(); err != nil {
		t.Fatalf("exact byte boundary rejected: %v", err)
	}
	in.Style.ContextBudgetBytes--
	if err := in.ValidateReadBudget(); !HasCode(err, CodeInvalidArgument) {
		t.Fatal("one byte over budget accepted")
	}
}
