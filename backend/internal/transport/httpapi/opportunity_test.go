package httpapi

import (
	"context"
	"net/http"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/storage"
)

func TestOpportunityPolicyHTTPAuthorityAndRecovery(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "opportunity-http.db")
	s, handler := openHTTPTestServer(t, ctx, path)
	defer func() { s.Close() }()
	if _, err := s.PrepareRPLifeDemo(ctx); err != nil {
		t.Fatal(err)
	}
	run, err := s.RunAgentLife(ctx, storage.M2AgentNoonTime, 100)
	if err != nil {
		t.Fatal(err)
	}
	r := storage.OpportunityPolicyRequest{Binding: core.CareerBinding{InstanceID: storage.M2DemoInstanceID, BranchID: storage.M2DemoBranchID, ExpectedHead: run.HeadSequence, IdempotencyKey: "policy-http"}, Policy: storage.RPOpportunityPolicy{StreamSeed: "http-fixture", ContactBasisPoints: 1500, WorkBasisPoints: 1000, CommunityBasisPoints: 1000, VisitBasisPoints: 1000, RareVisitBasisPoints: 50, WarmEnabled: true, CooldownHours: 6, HistoryHours: 240}}
	const route = "/api/v1/opportunities/policy/define"
	response := performJSON(t, handler, route, "", r)
	assertAPIError(t, response, http.StatusUnauthorized, core.CodeUnauthenticated)
	response = performJSON(t, handler, route, boAgentToken, r)
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)
	forged := r
	forged.Binding.PrincipalID = "principal_creator"
	response = performJSON(t, handler, route, boAgentToken, forged)
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)
	response = performJSON(t, handler, route, creatorToken, r)
	assertStatus(t, response, http.StatusOK)
	created := decodeData[storage.OpportunityPolicyRecord](t, response)
	if created.EventID == "" || created.Fact.BuilderSourceEventID == "" || created.Fact.Policy != r.Policy {
		t.Fatal("policy source/config lost")
	}
	changed := r
	changed.Policy.StreamSeed = "reroll-attempt"
	response = performJSON(t, handler, route, creatorToken, changed)
	assertAPIError(t, response, http.StatusConflict, core.CodeIdempotencyMismatch)
	changed.Binding.IdempotencyKey = "new-policy-key"
	changed.Binding.ExpectedHead = created.EventSequence
	response = performJSON(t, handler, route, creatorToken, changed)
	assertAPIError(t, response, http.StatusConflict, core.CodeBranchConflict)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, handler = openHTTPTestServer(t, ctx, path)
	response = performJSON(t, handler, route, creatorToken, r)
	assertStatus(t, response, http.StatusOK)
	retry := decodeData[storage.OpportunityPolicyRecord](t, response)
	if !retry.Replayed || retry.EventID != created.EventID || retry.Fact != created.Fact {
		t.Fatal("reopened HTTP retry changed policy")
	}
}

func TestEnvironmentSourceHTTPAuthorityAndRecovery(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "environment-http.db")
	s, handler := openHTTPTestServer(t, ctx, path)
	defer func() { s.Close() }()
	if _, err := s.PrepareRPLifeDemo(ctx); err != nil {
		t.Fatal(err)
	}
	run, err := s.RunAgentLife(ctx, storage.M2AgentNoonTime, 100)
	if err != nil {
		t.Fatal(err)
	}
	policy := storage.OpportunityPolicyRequest{Binding: core.CareerBinding{InstanceID: storage.M2DemoInstanceID, BranchID: storage.M2DemoBranchID, ExpectedHead: run.HeadSequence, IdempotencyKey: "environment-policy"}, Policy: storage.RPOpportunityPolicy{StreamSeed: "environment-http", ContactBasisPoints: 0, CooldownHours: 1, HistoryHours: 24}}
	response := performJSON(t, handler, "/api/v1/opportunities/policy/define", creatorToken, policy)
	assertStatus(t, response, http.StatusOK)
	installed := decodeData[storage.OpportunityPolicyRecord](t, response)
	r := storage.EnvironmentSourceRequest{Binding: core.CareerBinding{InstanceID: storage.M2DemoInstanceID, BranchID: storage.M2DemoBranchID, ExpectedHead: installed.EventSequence, IdempotencyKey: "environment-http"}, Source: storage.RPEnvironmentSource{PlaceID: storage.M2AgentCafeID, RainBasisPoints: 1500, CooldownHours: 6}}
	const route = "/api/v1/opportunities/environment/define"
	response = performJSON(t, handler, route, "", r)
	assertAPIError(t, response, http.StatusUnauthorized, core.CodeUnauthenticated)
	response = performJSON(t, handler, route, boAgentToken, r)
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)
	forged := r
	forged.Binding.PrincipalID = "principal_creator"
	response = performJSON(t, handler, route, boAgentToken, forged)
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)
	response = performJSON(t, handler, route, creatorToken, r)
	assertStatus(t, response, http.StatusOK)
	created := decodeData[storage.EnvironmentSourceRecord](t, response)
	if created.EventID == "" || created.Fact.PolicyEventID != installed.EventID || created.Fact.PlaceSourceEventID == "" || created.Fact.Source != r.Source {
		t.Fatal("environment provenance lost")
	}
	changed := r
	changed.Source.RainBasisPoints = 3000
	response = performJSON(t, handler, route, creatorToken, changed)
	assertAPIError(t, response, http.StatusConflict, core.CodeIdempotencyMismatch)
	changed.Binding.IdempotencyKey = "reset-environment"
	changed.Binding.ExpectedHead = created.EventSequence
	response = performJSON(t, handler, route, creatorToken, changed)
	assertAPIError(t, response, http.StatusConflict, core.CodeBranchConflict)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, handler = openHTTPTestServer(t, ctx, path)
	if err := s.RebuildProjections(ctx, storage.M2DemoInstanceID, storage.M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	response = performJSON(t, handler, route, creatorToken, r)
	assertStatus(t, response, http.StatusOK)
	retry := decodeData[storage.EnvironmentSourceRecord](t, response)
	if !retry.Replayed || retry.EventID != created.EventID || retry.Fact != created.Fact {
		t.Fatal("environment changed across HTTP retry")
	}
}

func TestStorefrontSourceHTTPAuthorityAndRecovery(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "storefront-http.db")
	s, handler := openHTTPTestServer(t, ctx, path)
	defer func() { s.Close() }()
	if _, err := s.PrepareRPLifeDemo(ctx); err != nil {
		t.Fatal(err)
	}
	run, err := s.RunAgentLife(ctx, storage.M2AgentNoonTime, 100)
	if err != nil {
		t.Fatal(err)
	}
	r := storage.StorefrontSourceRequest{Binding: core.CareerBinding{InstanceID: storage.M2DemoInstanceID, BranchID: storage.M2DemoBranchID, ExpectedHead: run.HeadSequence, IdempotencyKey: "storefront-http"}, Source: storage.RPStorefrontSource{PlaceID: storage.M2AgentCafeID, StoreActorID: "actor_m2_food_store", SKUID: storage.M2DemoSKUID}}
	const route = "/api/v1/opportunities/storefront/define"
	response := performJSON(t, handler, route, "", r)
	assertAPIError(t, response, http.StatusUnauthorized, core.CodeUnauthenticated)
	response = performJSON(t, handler, route, boAgentToken, r)
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)
	forged := r
	forged.Binding.PrincipalID = "principal_creator"
	response = performJSON(t, handler, route, boAgentToken, forged)
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)
	response = performJSON(t, handler, route, creatorToken, r)
	assertStatus(t, response, http.StatusOK)
	created := decodeData[storage.StorefrontSourceRecord](t, response)
	if created.EventID == "" || created.Fact.StoreSourceEventID == "" || created.Fact.PlaceSourceEventID == "" || created.Fact.Source != r.Source {
		t.Fatal("storefront provenance lost")
	}
	changed := r
	changed.Source.PlaceID = "place_m2_home_bo"
	response = performJSON(t, handler, route, creatorToken, changed)
	assertAPIError(t, response, http.StatusConflict, core.CodeIdempotencyMismatch)
	changed.Binding.IdempotencyKey = "duplicate-storefront"
	changed.Binding.ExpectedHead = created.EventSequence
	response = performJSON(t, handler, route, creatorToken, changed)
	assertAPIError(t, response, http.StatusConflict, core.CodeBranchConflict)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, handler = openHTTPTestServer(t, ctx, path)
	if err := s.RebuildProjections(ctx, storage.M2DemoInstanceID, storage.M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	response = performJSON(t, handler, route, creatorToken, r)
	assertStatus(t, response, http.StatusOK)
	retry := decodeData[storage.StorefrontSourceRecord](t, response)
	if !retry.Replayed || retry.EventID != created.EventID || retry.Fact != created.Fact {
		t.Fatal("storefront changed across HTTP retry")
	}
}

func TestTransitWorksHTTPAuthorityAndRecovery(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "transit-http.db")
	s, handler := openHTTPTestServer(t, ctx, path)
	defer func() { s.Close() }()
	if _, err := s.PrepareRPLifeDemo(ctx); err != nil {
		t.Fatal(err)
	}
	run, err := s.RunAgentLife(ctx, storage.M2AgentNoonTime, 100)
	if err != nil {
		t.Fatal(err)
	}
	r := storage.TransitWorksRequest{Binding: core.CareerBinding{InstanceID: storage.M2DemoInstanceID, BranchID: storage.M2DemoBranchID, ExpectedHead: run.HeadSequence, IdempotencyKey: "transit-http"}, FromPlaceID: storage.M2AgentCafeID, ToPlaceID: "place_m2_work_ada", StartsAt: "2026-09-24T08:00:00Z", EndsAt: "2026-09-24T09:00:00Z"}
	const route = "/api/v1/opportunities/transit/define"
	response := performJSON(t, handler, route, "", r)
	assertAPIError(t, response, http.StatusUnauthorized, core.CodeUnauthenticated)
	response = performJSON(t, handler, route, boAgentToken, r)
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)
	forged := r
	forged.Binding.PrincipalID = "principal_creator"
	response = performJSON(t, handler, route, boAgentToken, forged)
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)
	response = performJSON(t, handler, route, creatorToken, r)
	assertStatus(t, response, http.StatusOK)
	created := decodeData[storage.TransitWorksRecord](t, response)
	if created.EventID == "" || created.Fact.Window.SourceEventID != created.EventID || created.Fact.ForwardRouteSourceEventID == "" || created.Fact.ReverseRouteSourceEventID == "" {
		t.Fatal("transit source missing")
	}
	changed := r
	changed.EndsAt = "2026-09-24T09:30:00Z"
	response = performJSON(t, handler, route, creatorToken, changed)
	assertAPIError(t, response, http.StatusConflict, core.CodeIdempotencyMismatch)
	changed.Binding.IdempotencyKey = "overlapping-works"
	changed.Binding.ExpectedHead = created.EventSequence
	response = performJSON(t, handler, route, creatorToken, changed)
	assertAPIError(t, response, http.StatusConflict, core.CodeBranchConflict)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, handler = openHTTPTestServer(t, ctx, path)
	if err := s.RebuildProjections(ctx, storage.M2DemoInstanceID, storage.M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	response = performJSON(t, handler, route, creatorToken, r)
	assertStatus(t, response, http.StatusOK)
	retry := decodeData[storage.TransitWorksRecord](t, response)
	if !retry.Replayed || retry.EventID != created.EventID || retry.Fact != created.Fact {
		t.Fatal("transit HTTP retry changed works")
	}
}
