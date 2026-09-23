package httpapi

import (
	"context"
	"net/http"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/storage"
)

func TestCareerGradeScaleHTTPIdentityAndRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "grades-http.db")
	s, handler := openHTTPTestServer(t, ctx, path)
	defer func() { s.Close() }()
	setup, err := s.PrepareRPLifeDemo(ctx)
	if err != nil {
		t.Fatal(err)
	}
	b := core.CareerBinding{PrincipalID: "principal_creator", InstanceID: storage.M2DemoInstanceID, BranchID: storage.M2DemoBranchID, ExpectedHead: setup.Routine.EventSequence, IdempotencyKey: "org"}
	org, err := s.DefineCareerOrganization(ctx, core.CareerOrganizationRequest{Binding: b, Organization: core.CareerOrganizationDefinition{OrganizationID: "actor_m2_coop_employer", DisplayName: "Co-op", ManagerPrincipalID: storage.M2AgentBoPrincipal, WorkplaceID: "place_m2_work_ada"}})
	if err != nil {
		t.Fatal(err)
	}
	b.PrincipalID, b.ExpectedHead, b.IdempotencyKey = "", org.EventSequence, "grades"
	r := core.CareerGradeScaleRequest{Binding: b, Scale: core.CareerGradeScale{OrganizationID: org.Fact.OrganizationID, Grades: []string{"entry", "lead"}}}
	endpoint := "/api/v1/career/grades/define"
	assertAPIError(t, performJSON(t, handler, endpoint, "", r), http.StatusUnauthorized, core.CodeUnauthenticated)
	assertAPIError(t, performJSON(t, handler, endpoint, adaAgentToken, r), http.StatusForbidden, core.CodeUnauthorized)
	forged := r
	forged.Binding.PrincipalID = storage.M2AgentBoPrincipal
	assertAPIError(t, performJSON(t, handler, endpoint, adaAgentToken, forged), http.StatusForbidden, core.CodeUnauthorized)
	response := performJSON(t, handler, endpoint, boAgentToken, r)
	assertStatus(t, response, http.StatusOK)
	defined := decodeData[storage.CareerRecord](t, response)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, handler = openHTTPTestServer(t, ctx, path)
	response = performJSON(t, handler, endpoint, boAgentToken, r)
	assertStatus(t, response, http.StatusOK)
	got := decodeData[storage.CareerRecord](t, response)
	if !got.Replayed || got.EventID != defined.EventID {
		t.Fatalf("retry: %+v", got)
	}
	read := careerRecordRead{InstanceID: b.InstanceID, BranchID: b.BranchID, Kind: "grade_scale", RecordID: r.Scale.OrganizationID}
	assertAPIError(t, performJSON(t, handler, "/api/v1/career/records/read", adaAgentToken, read), http.StatusForbidden, core.CodeUnauthorized)
	response = performJSON(t, handler, "/api/v1/career/records/read", boAgentToken, read)
	assertStatus(t, response, http.StatusOK)
	if got := decodeData[storage.CareerRecord](t, response); got.EventID != defined.EventID || got.Fact.GradeScale == nil {
		t.Fatalf("read: %+v", got)
	}
}
