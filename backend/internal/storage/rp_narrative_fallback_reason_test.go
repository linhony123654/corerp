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
	} {
		if got := sanitizeRPNarrativeFallback(source); got != "prose_validation_failure" {
			t.Fatalf("unsafe draft %q was mislabeled as %q", source, got)
		}
	}
}
