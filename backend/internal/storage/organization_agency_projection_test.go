package storage

import (
	"context"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestOrganizationAgencyProjectionsRecoverFromEvents(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "agency-source.db")
	s := openCareerTestWorld(t, path)
	defer func() { s.Close() }()
	org := careerTestOrg(t, s)
	if _, err := s.DefineCareerOrganization(ctx, org); err != nil {
		t.Fatal(err)
	}
	posting := careerTestPosting(t, s)
	if _, err := s.PostCareerPosition(ctx, posting); err != nil {
		t.Fatal(err)
	}
	p := core.OrganizationAgencyPolicy{PolicyID: "policy_source_recovery", OrganizationID: org.Organization.OrganizationID, ManagerPrincipalID: M2AgentBoPrincipal, ReviewFrequencyHours: 24, TargetPositionID: posting.Posting.PositionID, DefaultCapacity: 2, Status: "active"}
	if _, err := s.DefineOrganizationAgencyPolicy(ctx, core.OrganizationAgencyPolicyRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "source-policy"), Policy: p}); err != nil {
		t.Fatal(err)
	}
	r := core.OrganizationReviewRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "source-review"), OrganizationID: p.OrganizationID, PolicyID: p.PolicyID}
	if _, err := s.db.Exec(`UPDATE account_balances SET balance_minor=balance_minor+1 WHERE account_id=?`, m2EconomyEmployerCash); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ConductOrganizationReview(ctx, r); !core.HasCode(err, core.CodeProjectionDiverged) {
		t.Fatalf("review used unsourced cash: %v", err)
	}
	if err := s.RebuildProjections(ctx, r.Binding.InstanceID, r.Binding.BranchID); err != nil {
		t.Fatal(err)
	}
	review, err := s.ConductOrganizationReview(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	if review.Decision.DecisionKind != "expand_capacity" {
		t.Fatalf("real funded organization should expand: %+v", review)
	}
	retry, err := s.ConductOrganizationReview(ctx, r)
	if err != nil || !retry.Replay || retry.EventID != review.EventID {
		t.Fatalf("exact retry: %+v %v", retry, err)
	}
	assertClean := func() {
		t.Helper()
		differences, err := s.CompareProjections(ctx, r.Binding.InstanceID, r.Binding.BranchID)
		if err != nil || len(differences) != 0 {
			t.Fatalf("source parity: %+v %v", differences, err)
		}
	}
	assertClean()
	// Fault injection affects only derived rows in this disposable test world.
	if _, err := s.db.Exec(`UPDATE organization_agency_policies SET reserve_target_minor=999999 WHERE policy_id=?`, p.PolicyID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE organization_reviews SET decision_kind='freeze_recruitment' WHERE review_id=?`, review.ReviewID); err != nil {
		t.Fatal(err)
	}
	read, err := s.ReadOrganizationAgencyPolicy(ctx, M2AgentBoPrincipal, r.Binding.InstanceID, r.Binding.BranchID, p.OrganizationID)
	if err != nil || read != p {
		t.Fatalf("mutable policy changed authoritative read: %+v %v", read, err)
	}
	history, err := s.ReadOrganizationReviews(ctx, M2AgentBoPrincipal, r.Binding.InstanceID, r.Binding.BranchID, p.OrganizationID, 10)
	if err != nil || len(history) != 1 || history[0].Decision != review.Decision {
		t.Fatalf("mutable review changed authoritative history: %+v %v", history, err)
	}
	differences, err := s.CompareProjections(ctx, r.Binding.InstanceID, r.Binding.BranchID)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, d := range differences {
		if d.Projection == "organization_agency" {
			count++
		}
	}
	if count != 2 {
		t.Fatalf("expected both corrupt rows detected: %+v", differences)
	}
	if err := s.RebuildProjections(ctx, r.Binding.InstanceID, r.Binding.BranchID); err != nil {
		t.Fatal(err)
	}
	assertClean()
	if _, err := s.db.Exec(`DELETE FROM organization_reviews WHERE review_id=?`, review.ReviewID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`DELETE FROM organization_agency_policies WHERE policy_id=?`, p.PolicyID); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	differences, err = s.CompareProjections(ctx, r.Binding.InstanceID, r.Binding.BranchID)
	if err != nil || len(differences) != 2 {
		t.Fatalf("missing projections after restart: %+v %v", differences, err)
	}
	if err := s.RebuildProjections(ctx, r.Binding.InstanceID, r.Binding.BranchID); err != nil {
		t.Fatal(err)
	}
	assertClean()
	retry, err = s.ConductOrganizationReview(ctx, r)
	if err != nil || !retry.Replay || retry.EventID != review.EventID {
		t.Fatalf("recovered receipt: %+v %v", retry, err)
	}
	// Restore the actual pre-056 table shape in this populated disposable
	// world, then exercise the registered additive migration on reopen.
	if _, err := s.db.Exec(`ALTER TABLE organization_agency_policies DROP COLUMN automatic_review`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`DELETE FROM schema_meta WHERE schema_version=?`, OrganizationReviewScheduleSchemaVersion); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	assertClean()
	retry, err = s.ConductOrganizationReview(ctx, r)
	if err != nil || !retry.Replay || retry.EventID != review.EventID {
		t.Fatalf("055 to 056 lost prior review: %+v %v", retry, err)
	}
	if _, err := s.db.Exec(`UPDATE events SET payload=json_set(payload,'$.organization_review.decision.reason','forged decision') WHERE event_id=?`, review.EventID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CompareProjections(ctx, r.Binding.InstanceID, r.Binding.BranchID); !core.HasCode(err, core.CodeProjectionDiverged) {
		t.Fatalf("tampered immutable decision was trusted: %v", err)
	}
	if err := s.RebuildProjections(ctx, r.Binding.InstanceID, r.Binding.BranchID); !core.HasCode(err, core.CodeProjectionDiverged) {
		t.Fatalf("rebuild accepted invalid decision authority: %v", err)
	}
}
