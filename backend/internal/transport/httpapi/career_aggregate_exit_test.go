package httpapi

import (
	"context"
	"net/http"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/storage"
)

func TestCareerHTTPAggregateExitAuthenticationAndRecovery(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "aggregate-exit-http.db")
	s, handler := openHTTPTestServer(t, ctx, path)
	defer func() { s.Close() }()
	setup, err := s.PrepareRPLifeDemo(ctx)
	if err != nil {
		t.Fatal(err)
	}
	r := core.CareerAggregateExitRequest{Binding: core.CareerBinding{InstanceID: storage.M2DemoInstanceID, BranchID: storage.M2DemoBranchID, ExpectedHead: setup.Routine.EventSequence, IdempotencyKey: "player-exit"}, CandidateID: storage.M2RPPlayerID, ContractID: "contract_m2_cohort_wage_18", FinalEarnedDay: 2, Notice: "Leave after the final earned period."}
	const endpoint = "/api/v1/career/aggregate-employment/exit"
	assertAPIError(t, performJSON(t, handler, endpoint, "", r), http.StatusUnauthorized, core.CodeUnauthenticated)
	assertAPIError(t, performJSON(t, handler, endpoint, boAgentToken, r), http.StatusForbidden, core.CodeUnauthorized)
	forged := r
	forged.Binding.PrincipalID = storage.M2RPPlayerPrincipal
	assertAPIError(t, performJSON(t, handler, endpoint, boAgentToken, forged), http.StatusForbidden, core.CodeUnauthorized)
	forged = r
	forged.CandidateID = storage.M2RPNPCID
	assertAPIError(t, performJSON(t, handler, endpoint, rpPlayerToken, forged), http.StatusForbidden, core.CodeUnauthorized)
	response := performJSON(t, handler, endpoint, rpPlayerToken, r)
	assertStatus(t, response, http.StatusOK)
	notice := decodeData[storage.CareerRecord](t, response)
	if notice.Fact.CandidateID != storage.M2RPPlayerID || notice.Fact.AggregateExit == nil || notice.Fact.AggregateExit.Status != "notice_recorded" || notice.Fact.AggregateExit.EarliestIndependentStartDay != 3 {
		t.Fatalf("HTTP exit notice: %+v", notice)
	}
	forged = r
	forged.FinalEarnedDay = 3
	assertAPIError(t, performJSON(t, handler, endpoint, rpPlayerToken, forged), http.StatusConflict, core.CodeIdempotencyMismatch)
	if _, err := s.RunAgentLife(ctx, "2026-09-24T07:02:00Z", 100); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, handler = openHTTPTestServer(t, ctx, path)
	response = performJSON(t, handler, endpoint, rpPlayerToken, r)
	assertStatus(t, response, http.StatusOK)
	replay := decodeData[storage.CareerRecord](t, response)
	if !replay.Replayed || replay.EventID != notice.EventID || replay.Fact.AggregateExit.Status != "notice_recorded" {
		t.Fatalf("historical notice retry after activation: %+v", replay)
	}
	duplicate := r
	duplicate.Binding.IdempotencyKey = "second-exit"
	state, err := s.RunAgentLife(ctx, "2026-09-24T07:02:00Z", 100)
	if err != nil {
		t.Fatal(err)
	}
	duplicate.Binding.ExpectedHead = state.HeadSequence
	assertAPIError(t, performJSON(t, handler, endpoint, rpPlayerToken, duplicate), http.StatusBadRequest, core.CodeInvalidArgument)
	if err := s.RebuildProjections(ctx, storage.M2DemoInstanceID, storage.M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	if differences, err := s.CompareProjections(ctx, storage.M2DemoInstanceID, storage.M2DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatalf("HTTP exit recovery projections: %v %v", differences, err)
	}
}
