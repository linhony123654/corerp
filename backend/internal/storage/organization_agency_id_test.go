package storage

import (
	"fmt"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestOrganizationAgencyReviewIdentitySurvivesLegacyPrefixCollision(t *testing.T) {
	b := core.CareerBinding{InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID}
	type source struct{ at, event string }
	seen := map[string]source{}
	for i := 1; i <= 10000; i++ {
		s := source{careerTime(i, 8, 0), fmt.Sprintf("event_review_%d", i)}
		hash, err := core.HashJSON([]any{b.InstanceID, b.BranchID, "organization", s.at, s.event})
		if err != nil {
			t.Fatal(err)
		}
		if prior, exists := seen[hash[:12]]; exists {
			first, err := organizationReviewID(b, "organization", prior.at, prior.event)
			if err != nil {
				t.Fatal(err)
			}
			second, err := organizationReviewID(b, "organization", s.at, s.event)
			if err != nil {
				t.Fatal(err)
			}
			if first == second || len(first) != len("review_")+64 || len(second) != len("review_")+64 {
				t.Fatalf("review identity collision: %s %s", first, second)
			}
			return
		}
		seen[hash[:12]] = s
	}
	t.Fatal("deterministic regression fixture failed to exhibit the old 20-bit collision")
}
