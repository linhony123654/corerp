package httpapi

import (
	"context"
	"database/sql"
	"net/http"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/storage"
)

func TestOrganizationAgencyHTTPPolicyAndReviewWorkflow(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "org-agency-http.db")
	s, handler := openHTTPTestServer(t, ctx, path)
	defer func() { s.Close() }()

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	setup, err := s.PrepareRPLifeDemo(ctx)
	if err != nil {
		t.Fatal(err)
	}

	binding := core.CareerBinding{
		InstanceID:     storage.M2DemoInstanceID,
		BranchID:       storage.M2DemoBranchID,
		ExpectedHead:   setup.Routine.EventSequence,
		IdempotencyKey: "org-agency-define-org",
	}

	// 1. Define organization as creator
	org := core.CareerOrganizationRequest{
		Binding: binding,
		Organization: core.CareerOrganizationDefinition{
			OrganizationID:     "actor_m2_coop_employer",
			DisplayName:        "Co-op",
			ManagerPrincipalID: storage.M2AgentBoPrincipal,
			WorkplaceID:        "place_m2_work_ada",
		},
	}
	resOrg := performJSON(t, handler, "/api/v1/career/organizations/define", creatorToken, org)
	assertStatus(t, resOrg, http.StatusOK)
	definedOrg := decodeData[storage.CareerRecord](t, resOrg)

	// 2. Post position as Bo
	binding.ExpectedHead, binding.IdempotencyKey = definedOrg.EventSequence, "org-agency-post"
	posting := core.CareerPostingRequest{
		Binding: binding,
		Posting: core.CareerPostingDefinition{
			PositionID:     "http_assistant_pos",
			OrganizationID: org.Organization.OrganizationID,
			Title:          "Assistant",
			OccupationID:   "operations",
			Grade:          "entry",
			Capacity:       1,
			DailyWageMinor: 12,
		},
	}
	resPost := performJSON(t, handler, "/api/v1/career/positions/post", boAgentToken, posting)
	assertStatus(t, resPost, http.StatusOK)
	definedPost := decodeData[storage.CareerRecord](t, resPost)

	// 3. Define agency policy: unauthorized attempt by Ada
	binding.ExpectedHead, binding.IdempotencyKey = definedPost.EventSequence, "ada-policy-forbidden"
	policy := core.OrganizationAgencyPolicy{
		PolicyID:             "policy_http_coop",
		OrganizationID:       org.Organization.OrganizationID,
		ManagerPrincipalID:   storage.M2AgentBoPrincipal,
		ReviewFrequencyHours: 24,
		AutomaticReview:      true,
		ReserveTargetMinor:   1300,
		HiringThresholdMinor: 50,
		FreezeThresholdMinor: 40,
		TargetPositionID:     posting.Posting.PositionID,
		DefaultCapacity:      1,
		Status:               "active",
	}
	policyReq := core.OrganizationAgencyPolicyRequest{
		Binding: binding,
		Policy:  policy,
	}
	resPolicyAda := performJSON(t, handler, "/api/v1/career/agency/policy/define", adaAgentToken, policyReq)
	assertAPIError(t, resPolicyAda, http.StatusForbidden, core.CodeUnauthorized)

	// 4. Define agency policy: authorized by Bo
	binding.IdempotencyKey = "bo-policy-ok"
	policyReq.Binding = binding
	resPolicyBo := performJSON(t, handler, "/api/v1/career/agency/policy/define", boAgentToken, policyReq)
	assertStatus(t, resPolicyBo, http.StatusOK)
	definedPolicy := decodeData[core.OrganizationAgencyPolicy](t, resPolicyBo)
	if definedPolicy.PolicyID != policy.PolicyID || definedPolicy.Status != "active" {
		t.Fatalf("unexpected defined policy: %+v", definedPolicy)
	}

	// 5. Read back agency policy
	readReq := agencyPolicyRead{
		InstanceID:     binding.InstanceID,
		BranchID:       binding.BranchID,
		OrganizationID: org.Organization.OrganizationID,
	}
	resRead := performJSON(t, handler, "/api/v1/career/agency/policy/read", boAgentToken, readReq)
	assertStatus(t, resRead, http.StatusOK)
	resReadAda := performJSON(t, handler, "/api/v1/career/agency/policy/read", adaAgentToken, readReq)
	assertAPIError(t, resReadAda, http.StatusForbidden, core.CodeUnauthorized)
	readPolicy := decodeData[core.OrganizationAgencyPolicy](t, resRead)
	if readPolicy.PolicyID != policy.PolicyID || readPolicy.ReserveTargetMinor != 1300 || !readPolicy.AutomaticReview {
		t.Fatalf("unexpected read policy: %+v", readPolicy)
	}

	// The real 1200 funding is below this manager's declared reserve threshold.

	// Read current head sequence for the review command
	var currentHead int64
	if err := db.QueryRow(`SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, binding.InstanceID, binding.BranchID).Scan(&currentHead); err != nil {
		t.Fatal(err)
	}

	// 6. Conduct organization review: unauthorized attempt by Ada
	binding.ExpectedHead, binding.IdempotencyKey = currentHead, "ada-review-forbidden"
	reviewReq := core.OrganizationReviewRequest{
		Binding:        binding,
		OrganizationID: org.Organization.OrganizationID,
		PolicyID:       policy.PolicyID,
	}
	resRevAda := performJSON(t, handler, "/api/v1/career/agency/reviews/conduct", adaAgentToken, reviewReq)
	assertAPIError(t, resRevAda, http.StatusForbidden, core.CodeUnauthorized)

	// 7. Conduct review as Bo: under financial strain -> freeze
	binding.IdempotencyKey = "bo-review-ok"
	reviewReq.Binding = binding
	resRevBo := performJSON(t, handler, "/api/v1/career/agency/reviews/conduct", boAgentToken, reviewReq)
	assertStatus(t, resRevBo, http.StatusOK)
	reviewResult := decodeData[core.OrganizationReviewResult](t, resRevBo)
	if reviewResult.Decision.DecisionKind != "freeze_recruitment" {
		t.Fatalf("expected freeze_recruitment, got: %s", reviewResult.Decision.DecisionKind)
	}

	// 8. Query reviews history
	historyReq := agencyReviewsRead{
		InstanceID:     binding.InstanceID,
		BranchID:       binding.BranchID,
		OrganizationID: org.Organization.OrganizationID,
		Limit:          10,
	}
	resHist := performJSON(t, handler, "/api/v1/career/agency/reviews/query", boAgentToken, historyReq)
	assertStatus(t, resHist, http.StatusOK)
	resHistAda := performJSON(t, handler, "/api/v1/career/agency/reviews/query", adaAgentToken, historyReq)
	assertAPIError(t, resHistAda, http.StatusForbidden, core.CodeUnauthorized)
	history := decodeData[[]core.OrganizationReviewResult](t, resHist)
	if len(history) != 1 || history[0].Decision.DecisionKind != "freeze_recruitment" {
		t.Fatalf("unexpected reviews history: %+v", history)
	}

	// 9. Reopen and verify persistence
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, handler = openHTTPTestServer(t, ctx, path)
	resHistReopen := performJSON(t, handler, "/api/v1/career/agency/reviews/query", boAgentToken, historyReq)
	assertStatus(t, resHistReopen, http.StatusOK)
	historyReopen := decodeData[[]core.OrganizationReviewResult](t, resHistReopen)
	if len(historyReopen) != 1 || historyReopen[0].ReviewID != reviewResult.ReviewID {
		t.Fatalf("unexpected reviews history after restart: %+v", historyReopen)
	}
}
