package httpapi

import (
	"context"
	"net/http"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/storage"
)

func TestRPBackgroundHTTPAuthorityAndStableRetry(t *testing.T) {
	ctx := context.Background()
	s, handler := openHTTPTestServer(t, ctx, filepath.Join(t.TempDir(), "background-http.db"))
	defer s.Close()
	if _, err := s.PrepareRPTravel(ctx); err != nil {
		t.Fatal(err)
	}
	_, err := s.MaterializeCohort(ctx, core.MaterializeCohortCommand{CommandID: "cmd_http_emergent", MaterializationID: "mat_http_emergent", InstanceID: storage.M2DemoInstanceID, BranchID: storage.M2DemoBranchID, PrincipalID: "principal_creator", CapabilityID: "world.cohort.materialize", IdempotencyKey: "http-emergent", ExpectedHead: 8, WorldTime: "2026-09-22T02:03:00Z", SourceCohortID: storage.M2DemoCohortID, EntityID: "entity_http_emergent", DisplayName: "Nora", PopulationCount: 1, AssetMinor: 200, InventoryMinor: 1, ReceivableMinor: 40, LiabilityMinor: 30, AllocationAlgorithmVersion: "equal-share-v1"})
	if err != nil {
		t.Fatal(err)
	}
	r := core.RPBackgroundRequest{InstanceID: storage.M2DemoInstanceID, BranchID: storage.M2DemoBranchID, EntityID: "entity_http_emergent", ExpectedHead: 9, IdempotencyKey: "background-http", AgeMin: 25, AgeMax: 34, ResidencePlaceID: "place_m2_home_bo", InitialPlaceID: storage.M2AgentCafeID, Schedule: []core.RPBackgroundSchedule{{WorldTime: "2026-09-22T03:00:00Z", PlaceID: "place_m2_home_bo", ActivityCode: "home"}}}
	response := performJSON(t, handler, "/api/v1/rp/background/materialize", rpPlayerToken, r)
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)
	response = performJSON(t, handler, "/api/v1/rp/background/materialize", creatorToken, r)
	assertStatus(t, response, http.StatusOK)
	first := decodeData[storage.RPBackgroundResult](t, response)
	response = performJSON(t, handler, "/api/v1/rp/background/materialize", creatorToken, r)
	assertStatus(t, response, http.StatusOK)
	retry := decodeData[storage.RPBackgroundResult](t, response)
	if !retry.Replayed || retry.Background.DefinitionEventID != first.Background.DefinitionEventID {
		t.Fatal("background HTTP retry differs")
	}
}
