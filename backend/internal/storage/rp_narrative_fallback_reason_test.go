package storage

import "testing"

func TestRPNarrativeUnsafeDraftReportsValidationFailure(t *testing.T) {
	for _, source := range []string{
		"prose_uncommitted player agency in prose",
		"prose_uncommitted NPC action in prose",
		"prose_uncommitted expression in prose",
		"prose_uncommitted money claim in prose",
		"prose_uncommitted object in prose",
		"prose_uncommitted time change in prose",
		"prose_uncommitted person in prose",
		"prose_uncommitted name in prose",
		"prose_uncommitted relationship in prose",
		"prose_unbalanced prose quotation",
		"prose_invalid composition plan: private response content",
		"prose_composition fact coverage mismatch: secret source ID",
		"prose_invalid composition template: private response content",
	} {
		if got := sanitizeRPNarrativeFallback(source); got != "prose_validation_failure" {
			t.Fatalf("unsafe draft %q was mislabeled as %q", source, got)
		}
	}
}
