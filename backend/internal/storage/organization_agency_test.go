package storage

import (
	"context"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestOrganizationAgencyPolicyDefinitionAndReviewAuthorization(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "org-agency-auth.db")
	s := openCareerTestWorld(t, path)
	defer s.Close()

	orgReq := careerTestOrg(t, s)
	if _, err := s.DefineCareerOrganization(ctx, orgReq); err != nil {
		t.Fatal(err)
	}
	postingReq := careerTestPosting(t, s)
	if _, err := s.PostCareerPosition(ctx, postingReq); err != nil {
		t.Fatal(err)
	}

	policy := core.OrganizationAgencyPolicy{
		PolicyID:             "policy_coop_operations",
		OrganizationID:       "actor_m2_coop_employer",
		ManagerPrincipalID:   M2AgentBoPrincipal,
		ReviewFrequencyHours: 24,
		ReserveTargetMinor:   100,
		HiringThresholdMinor: 50,
		FreezeThresholdMinor: 40,
		TargetPositionID:     "position_coop_assistant",
		DefaultCapacity:      1,
		Status:               "active",
	}

	// 1. Non-manager cannot define policy (Ada is candidate, not manager)
	badBinding := careerTestBinding(t, s, M2AgentAdaPrincipal, "ada-policy-try")
	if _, err := s.DefineOrganizationAgencyPolicy(ctx, core.OrganizationAgencyPolicyRequest{
		Binding: badBinding,
		Policy:  policy,
	}); err == nil || !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("expected CodeUnauthorized for non-manager, got: %v", err)
	}

	// 2. Nonexistent position fails with CodeNotFound
	fakePosPolicy := policy
	fakePosPolicy.TargetPositionID = "nonexistent_pos"
	fakePosBinding := careerTestBinding(t, s, M2AgentBoPrincipal, "fake-pos-policy")
	if _, err := s.DefineOrganizationAgencyPolicy(ctx, core.OrganizationAgencyPolicyRequest{
		Binding: fakePosBinding,
		Policy:  fakePosPolicy,
	}); err == nil || !core.HasCode(err, core.CodeNotFound) {
		t.Fatalf("expected CodeNotFound for nonexistent posting, got: %v", err)
	}
	wrongManager := policy
	wrongManager.ManagerPrincipalID = M2AgentAdaPrincipal
	if _, err := s.DefineOrganizationAgencyPolicy(ctx, core.OrganizationAgencyPolicyRequest{
		Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "wrong-policy-manager"),
		Policy:  wrongManager,
	}); !core.HasCode(err, core.CodeInvalidArgument) {
		t.Fatalf("policy cannot claim an unrelated manager, got %v", err)
	}

	// 3. Manager Bo defines valid policy
	managerBinding := careerTestBinding(t, s, M2AgentBoPrincipal, "manager-policy-ok")
	createdPolicy, err := s.DefineOrganizationAgencyPolicy(ctx, core.OrganizationAgencyPolicyRequest{
		Binding: managerBinding,
		Policy:  policy,
	})
	if err != nil {
		t.Fatalf("failed to define agency policy: %v", err)
	}
	if createdPolicy.PolicyID != policy.PolicyID || createdPolicy.Status != "active" {
		t.Fatalf("unexpected created policy: %+v", createdPolicy)
	}

	// 4. Read back policy
	readPolicy, err := s.ReadOrganizationAgencyPolicy(ctx, M2AgentBoPrincipal, managerBinding.InstanceID, managerBinding.BranchID, policy.OrganizationID)
	if err != nil {
		t.Fatalf("failed to read agency policy: %v", err)
	}
	if readPolicy.PolicyID != policy.PolicyID || readPolicy.ReserveTargetMinor != 100 {
		t.Fatalf("mismatched read policy: %+v", readPolicy)
	}
	if _, err := s.ReadOrganizationAgencyPolicy(ctx, M2AgentAdaPrincipal, managerBinding.InstanceID, managerBinding.BranchID, policy.OrganizationID); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("non-manager read must be denied, got %v", err)
	}
	if _, err := s.ReadOrganizationReviews(ctx, M2AgentAdaPrincipal, managerBinding.InstanceID, managerBinding.BranchID, policy.OrganizationID, 10); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("non-manager history read must be denied, got %v", err)
	}

	// 5. Non-manager cannot conduct review
	reviewBindingAda := careerTestBinding(t, s, M2AgentAdaPrincipal, "ada-review-try")
	if _, err := s.ConductOrganizationReview(ctx, core.OrganizationReviewRequest{
		Binding:        reviewBindingAda,
		OrganizationID: policy.OrganizationID,
		PolicyID:       policy.PolicyID,
	}); err == nil || !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("expected CodeUnauthorized for non-manager review, got: %v", err)
	}
	if _, err := s.ConductOrganizationReview(ctx, core.OrganizationReviewRequest{
		Binding:        careerTestBinding(t, s, M2AgentBoPrincipal, "stale-policy-id"),
		OrganizationID: policy.OrganizationID,
		PolicyID:       "unrelated_policy_id",
	}); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatalf("review must bind exact current policy, got %v", err)
	}
}

func TestOrganizationAgencyReviewFreezesAndUnfreezesCareerRecruitment(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "org-agency-flow.db")
	s := openCareerTestWorld(t, path)
	defer s.Close()

	orgReq := careerTestOrg(t, s)
	_, err := s.DefineCareerOrganization(ctx, orgReq)
	if err != nil {
		t.Fatal(err)
	}

	postingReq := careerTestPosting(t, s)
	if _, err := s.PostCareerPosition(ctx, postingReq); err != nil {
		t.Fatal(err)
	}

	policy := core.OrganizationAgencyPolicy{
		PolicyID:             "policy_coop_operations",
		OrganizationID:       orgReq.Organization.OrganizationID,
		ManagerPrincipalID:   M2AgentBoPrincipal,
		ReviewFrequencyHours: 24,
		ReserveTargetMinor:   1170,
		HiringThresholdMinor: 10,
		FreezeThresholdMinor: 0,
		TargetPositionID:     postingReq.Posting.PositionID,
		DefaultCapacity:      1,
		Status:               "active",
	}

	managerBinding := careerTestBinding(t, s, M2AgentBoPrincipal, "def-policy")
	if _, err := s.DefineOrganizationAgencyPolicy(ctx, core.OrganizationAgencyPolicyRequest{
		Binding: managerBinding,
		Policy:  policy,
	}); err != nil {
		t.Fatal(err)
	}

	// Ordinary wage accrual/payment changes real business evidence from the
	// initial 1200 funding; no balance projection is edited to manufacture it.
	if _, err := s.RunAgentLife(ctx, m2EconomyPaymentTime, 1000); err != nil {
		t.Fatal(err)
	}
	// 1. Review the sourced wage expenditure -> freeze recruitment.
	reviewBinding1 := careerTestBinding(t, s, M2AgentBoPrincipal, "review-strain")
	reviewResult1, err := s.ConductOrganizationReview(ctx, core.OrganizationReviewRequest{
		Binding:        reviewBinding1,
		OrganizationID: policy.OrganizationID,
		PolicyID:       policy.PolicyID,
	})
	if err != nil {
		t.Fatalf("conduct review failed: %v", err)
	}
	if reviewResult1.Decision.DecisionKind != "freeze_recruitment" {
		t.Fatalf("expected freeze_recruitment, got: %s (reason: %s)", reviewResult1.Decision.DecisionKind, reviewResult1.Decision.Reason)
	}
	if reviewResult1.Decision.PostingStatus != "frozen" {
		t.Fatalf("expected posting status frozen, got: %s", reviewResult1.Decision.PostingStatus)
	}
	market, err := s.DiscoverCareerPositions(ctx, M2AgentAdaPrincipal, M2DemoInstanceID, M2DemoBranchID, M2AgentAdaID, 0)
	if err != nil || len(market.Postings) != 1 || market.Postings[0].Posting.Status != "frozen" || market.Postings[0].AvailableSlots != 0 || market.Postings[0].SourceEventID != reviewResult1.EventID {
		t.Fatalf("market did not expose authoritative freeze: %+v %v", market, err)
	}
	if reviewResult1.Evidence.CashBalanceMinor >= 1170 {
		t.Fatalf("real wage payment did not change cash: %+v", reviewResult1.Evidence)
	}
	if _, err := s.ConductOrganizationReview(ctx, core.OrganizationReviewRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "too-soon-review"), OrganizationID: policy.OrganizationID, PolicyID: policy.PolicyID}); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatalf("new request key bypassed review interval: %v", err)
	}

	// 2. Candidate Ada attempts to apply to the frozen posting -> must be rejected
	adaApplyBinding := careerTestBinding(t, s, M2AgentAdaPrincipal, "ada-apply-frozen")
	if _, err := s.ApplyForCareerPosition(ctx, core.CareerApplicationRequest{
		Binding:       adaApplyBinding,
		ApplicationID: "app_ada_frozen",
		PositionID:    postingReq.Posting.PositionID,
		CandidateID:   M2AgentAdaID,
		Statement:     "I want to work as an assistant",
	}); err == nil || !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatalf("expected CodeBranchConflict for application to frozen posting, got: %v", err)
	}

	// 3. A manager explicitly revises the reserve policy; it is a sourced
	// policy change, not a claim that cash recovered.
	policy.ReserveTargetMinor = 100
	if _, err := s.DefineOrganizationAgencyPolicy(ctx, core.OrganizationAgencyPolicyRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "revise-reserve"), Policy: policy}); err != nil {
		t.Fatal(err)
	}

	// 4. Review under the revised policy -> unfreeze recruitment.
	if _, err := s.RunAgentLife(ctx, m2WageTime(2, 7, 1), 1000); err != nil {
		t.Fatal(err)
	}
	reviewBinding2 := careerTestBinding(t, s, M2AgentBoPrincipal, "review-recovery")
	reviewResult2, err := s.ConductOrganizationReview(ctx, core.OrganizationReviewRequest{
		Binding:        reviewBinding2,
		OrganizationID: policy.OrganizationID,
		PolicyID:       policy.PolicyID,
	})
	if err != nil {
		t.Fatalf("conduct recovery review failed: %v", err)
	}
	if reviewResult2.Decision.DecisionKind != "unfreeze_recruitment" {
		t.Fatalf("expected unfreeze_recruitment, got: %s (reason: %s)", reviewResult2.Decision.DecisionKind, reviewResult2.Decision.Reason)
	}
	if reviewResult2.Decision.PostingStatus != "active" {
		t.Fatalf("expected posting status active, got: %s", reviewResult2.Decision.PostingStatus)
	}
	market, err = s.DiscoverCareerPositions(ctx, M2AgentAdaPrincipal, M2DemoInstanceID, M2DemoBranchID, M2AgentAdaID, market.NextCursor)
	if err != nil || len(market.Postings) != 1 || market.Postings[0].Posting.Status != "active" || market.Postings[0].AvailableSlots != 1 || market.Postings[0].SourceEventID != reviewResult2.EventID {
		t.Fatalf("market cursor missed authoritative unfreeze: %+v %v", market, err)
	}

	// 5. Candidate Ada applies again now that posting is active -> succeeds!
	adaApplyBinding2 := careerTestBinding(t, s, M2AgentAdaPrincipal, "ada-apply-active")
	app, err := s.ApplyForCareerPosition(ctx, core.CareerApplicationRequest{
		Binding:       adaApplyBinding2,
		ApplicationID: "app_ada_active",
		PositionID:    postingReq.Posting.PositionID,
		CandidateID:   M2AgentAdaID,
		Statement:     "I want to work as an assistant",
	})
	if err != nil {
		t.Fatalf("application after unfreeze failed: %v", err)
	}
	if app.Fact.Application == nil || app.Fact.Application.Status != "submitted" {
		t.Fatalf("unexpected application state: %+v", app.Fact)
	}

	// 6. Review history query
	history, err := s.ReadOrganizationReviews(ctx, M2AgentBoPrincipal, managerBinding.InstanceID, managerBinding.BranchID, policy.OrganizationID, 10)
	if err != nil {
		t.Fatalf("failed to read review history: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("expected 2 reviews in history, got: %d", len(history))
	}
	if history[0].Decision.DecisionKind != "unfreeze_recruitment" || history[1].Decision.DecisionKind != "freeze_recruitment" {
		t.Fatalf("unexpected history order or kinds: %+v", history)
	}

	// 7. Sourced organization notice: Manager publishes F7 organization announcement for review decision
	noticeBinding := careerTestBinding(t, s, M2AgentBoPrincipal, "notice-review-unfreeze")
	notice, err := s.PublishRPOrganizationNotice(ctx, RPOrganizationNoticePublishRequest{
		Binding:       noticeBinding,
		MessageID:     "msg_org_unfreeze_announcement",
		CareerEventID: reviewResult2.EventID,
		SpeakerID:     M2AgentBoID,
	})
	if err != nil {
		t.Fatalf("failed to publish review announcement: %v", err)
	}
	if notice.Fact.MessageID != "msg_org_unfreeze_announcement" || notice.Fact.Channel != "organization_announcement" {
		t.Fatalf("unexpected notice result: %+v", notice)
	}
	if _, _, err := rpInformationExpected(ctx, s.db, managerBinding.InstanceID, managerBinding.BranchID, notice.EventSequence); err != nil {
		t.Fatalf("organization review notice must pass F7 source validation: %v", err)
	}

	// 8. Reopen Store and verify durability and replay
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s2, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()

	replayedPolicy, err := s2.ReadOrganizationAgencyPolicy(ctx, M2AgentBoPrincipal, managerBinding.InstanceID, managerBinding.BranchID, policy.OrganizationID)
	if err != nil {
		t.Fatalf("failed to read policy after reopen: %v", err)
	}
	if replayedPolicy.PolicyID != policy.PolicyID {
		t.Fatalf("policy mismatch after reopen: %+v", replayedPolicy)
	}

	replayedHistory, err := s2.ReadOrganizationReviews(ctx, M2AgentBoPrincipal, managerBinding.InstanceID, managerBinding.BranchID, policy.OrganizationID, 10)
	if err != nil {
		t.Fatalf("failed to read review history after reopen: %v", err)
	}
	if len(replayedHistory) != 2 {
		t.Fatalf("expected 2 reviews after reopen, got: %d", len(replayedHistory))
	}
}
