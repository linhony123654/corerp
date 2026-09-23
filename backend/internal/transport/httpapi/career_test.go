package httpapi

import (
	"context"
	"net/http"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/storage"
)

func TestCareerHTTPRecruitmentIdentityPrivacyAndRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "career-http.db")
	s, handler := openHTTPTestServer(t, ctx, path)
	defer func() { s.Close() }()
	setup, err := s.PrepareRPLifeDemo(ctx)
	if err != nil {
		t.Fatal(err)
	}
	binding := core.CareerBinding{InstanceID: storage.M2DemoInstanceID, BranchID: storage.M2DemoBranchID, ExpectedHead: setup.Routine.EventSequence, IdempotencyKey: "org-http"}
	org := core.CareerOrganizationRequest{Binding: binding, Organization: core.CareerOrganizationDefinition{OrganizationID: "actor_m2_coop_employer", DisplayName: "Co-op", ManagerPrincipalID: storage.M2AgentBoPrincipal, WorkplaceID: "place_m2_work_ada"}}
	response := performJSON(t, handler, "/api/v1/career/organizations/define", "", org)
	assertAPIError(t, response, http.StatusUnauthorized, core.CodeUnauthenticated)
	response = performJSON(t, handler, "/api/v1/career/organizations/define", rpPlayerToken, org)
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)
	response = performJSON(t, handler, "/api/v1/career/organizations/define", creatorToken, org)
	assertStatus(t, response, http.StatusOK)
	defined := decodeData[storage.CareerRecord](t, response)
	binding.ExpectedHead, binding.IdempotencyKey = defined.EventSequence, "post-http"
	posting := core.CareerPostingRequest{Binding: binding, Posting: core.CareerPostingDefinition{PositionID: "http_position", OrganizationID: org.Organization.OrganizationID, Title: "Assistant", OccupationID: "operations", Grade: "entry", Capacity: 1, DailyWageMinor: 12, RequiredQualifications: []string{}}}
	forged := posting
	forged.Binding.PrincipalID = storage.M2AgentBoPrincipal
	response = performJSON(t, handler, "/api/v1/career/positions/post", rpPlayerToken, forged)
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)
	response = performJSON(t, handler, "/api/v1/career/positions/post", creatorToken, posting)
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)
	response = performJSON(t, handler, "/api/v1/career/positions/post", boAgentToken, posting)
	assertStatus(t, response, http.StatusOK)
	posted := decodeData[storage.CareerRecord](t, response)
	query := careerMarketRead{InstanceID: binding.InstanceID, BranchID: binding.BranchID, CandidateID: storage.M2AgentAdaID}
	response = performJSON(t, handler, "/api/v1/career/positions/query", adaAgentToken, query)
	assertStatus(t, response, http.StatusOK)
	market := decodeData[storage.CareerMarket](t, response)
	if len(market.Postings) != 1 || market.Postings[0].SourceEventID != posted.EventID {
		t.Fatalf("HTTP market: %+v", market)
	}
	binding.ExpectedHead, binding.IdempotencyKey = posted.EventSequence, "apply-http"
	application := core.CareerApplicationRequest{Binding: binding, ApplicationID: "http_application", PositionID: posting.Posting.PositionID, CandidateID: storage.M2AgentAdaID, Statement: "Private application statement"}
	response = performJSON(t, handler, "/api/v1/career/applications/submit", boAgentToken, application)
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)
	response = performJSON(t, handler, "/api/v1/career/applications/submit", adaAgentToken, application)
	assertStatus(t, response, http.StatusOK)
	applied := decodeData[storage.CareerRecord](t, response)
	read := careerApplicationRead{InstanceID: binding.InstanceID, BranchID: binding.BranchID, ApplicationID: application.ApplicationID}
	response = performJSON(t, handler, "/api/v1/career/applications/read", rpPlayerToken, read)
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)
	response = performJSON(t, handler, "/api/v1/career/applications/read", boAgentToken, read)
	assertStatus(t, response, http.StatusOK)
	if got := decodeData[storage.CareerRecord](t, response); got.EventID != applied.EventID || got.Fact.Application.Statement != application.Statement {
		t.Fatalf("manager read: %+v", got)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, handler = openHTTPTestServer(t, ctx, path)
	response = performJSON(t, handler, "/api/v1/career/applications/submit", adaAgentToken, application)
	assertStatus(t, response, http.StatusOK)
	if got := decodeData[storage.CareerRecord](t, response); !got.Replayed || got.EventID != applied.EventID || got.EventSequence != applied.EventSequence {
		t.Fatalf("HTTP restart repeated application: %+v", got)
	}
	response = performJSON(t, handler, "/api/v1/career/positions/post", boAgentToken, posting)
	assertStatus(t, response, http.StatusOK)
	if got := decodeData[storage.CareerRecord](t, response); !got.Replayed || got.EventID != posted.EventID {
		t.Fatalf("HTTP restart repeated vacancy: %+v", got)
	}
}
