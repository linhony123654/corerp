package httpapi

import (
	"context"
	"net/http"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/storage"
)

func TestRPSpatialHTTPAuthorityAndJourneyMap(t *testing.T) {
	ctx := context.Background()
	s, handler := openHTTPTestServer(t, ctx, filepath.Join(t.TempDir(), "spatial-http.db"))
	defer s.Close()
	if _, err := s.PrepareRPTravel(ctx); err != nil {
		t.Fatal(err)
	}
	response := performJSON(t, handler, "/api/v1/rp/sessions/open", rpPlayerToken, core.RPSessionOpenRequest{InstanceID: storage.M2DemoInstanceID, BranchID: storage.M2DemoBranchID, EntityID: storage.M2RPPlayerID, POV: "second_person", IdempotencyKey: "spatial-http-session"})
	assertStatus(t, response, http.StatusOK)
	session := decodeData[storage.RPSession](t, response)
	read := core.RPSessionReadRequest{SessionID: session.SessionID}
	response = performJSON(t, handler, "/api/v1/rp/observe", rpPlayerToken, read)
	assertStatus(t, response, http.StatusOK)
	view := decodeData[storage.RPObservation](t, response)
	materialize := storage.RPLocationMaterializeRequest{Binding: core.CareerBinding{InstanceID: storage.M2DemoInstanceID, BranchID: storage.M2DemoBranchID, ExpectedHead: view.ObservationCursor, IdempotencyKey: "spatial-http-segment"}, ParentLocationID: storage.M2AgentCafeID, SlotKey: "http-road", Candidate: storage.RPLocationCandidate{DisplayName: "共用路段", GeneratorVersion: "local-v1"}}
	response = performJSON(t, handler, "/api/v1/rp/locations/materialize", rpPlayerToken, materialize)
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)
	response = performJSON(t, handler, "/api/v1/rp/locations/materialize", creatorToken, materialize)
	assertStatus(t, response, http.StatusOK)
	segment := decodeData[storage.RPLocationRecord](t, response)
	edge := storage.RPTimedEdgeRequest{Binding: core.CareerBinding{InstanceID: storage.M2DemoInstanceID, BranchID: storage.M2DemoBranchID, ExpectedHead: segment.EventSequence, IdempotencyKey: "spatial-http-edge"}, FromPlaceID: storage.M2AgentCafeID, ToPlaceID: "place_m2_home_ada", SegmentPlaceID: segment.Fact.LocationID, DurationMinutes: 15}
	response = performJSON(t, handler, "/api/v1/rp/edges/define", creatorToken, edge)
	assertStatus(t, response, http.StatusOK)
	response = performJSON(t, handler, "/api/v1/rp/observe", rpPlayerToken, read)
	assertStatus(t, response, http.StatusOK)
	view = decodeData[storage.RPObservation](t, response)
	survey := storage.RPMapSurveyRequest{SessionID: session.SessionID, ExpectedCursor: view.ObservationCursor, IdempotencyKey: "http-map"}
	response = performJSON(t, handler, "/api/v1/rp/map/survey", creatorToken, survey)
	assertAPIError(t, response, http.StatusNotFound, core.CodeNotFound)
	response = performJSON(t, handler, "/api/v1/rp/map/survey", rpPlayerToken, survey)
	assertStatus(t, response, http.StatusOK)
	response = performJSON(t, handler, "/api/v1/rp/map/read", rpPlayerToken, read)
	assertStatus(t, response, http.StatusOK)
	memory := decodeData[[]storage.RPMapMemory](t, response)
	if len(memory) != 1 || memory[0].PlaceID != storage.M2AgentCafeID {
		t.Fatal("HTTP map did not use player memory", memory)
	}
	response = performJSON(t, handler, "/api/v1/rp/observe", rpPlayerToken, read)
	assertStatus(t, response, http.StatusOK)
	view = decodeData[storage.RPObservation](t, response)
	journeyRequest := core.RPMoveRequest{SessionID: session.SessionID, FromPlaceID: view.PlaceID, ToPlaceID: "place_m2_home_ada", ExpectedCursor: view.ObservationCursor, IdempotencyKey: "http-journey"}
	response = performJSON(t, handler, "/api/v1/rp/journeys/start", creatorToken, journeyRequest)
	assertAPIError(t, response, http.StatusNotFound, core.CodeNotFound)
	response = performJSON(t, handler, "/api/v1/rp/journeys/start", rpPlayerToken, journeyRequest)
	assertStatus(t, response, http.StatusOK)
	journey := decodeData[storage.RPJourneyResult](t, response)
	if journey.SegmentPlaceID != segment.Fact.LocationID {
		t.Fatal("HTTP journey skipped segment", journey)
	}
	response = performJSON(t, handler, "/api/v1/rp/observe", rpPlayerToken, read)
	assertStatus(t, response, http.StatusOK)
	view = decodeData[storage.RPObservation](t, response)
	if view.PlaceID != segment.Fact.LocationID {
		t.Fatal("HTTP observe did not share journey position", view.PlaceID)
	}
	cancel := core.RPJourneyCancelRequest{SessionID: session.SessionID, JourneyID: journey.JourneyID, ExpectedCursor: view.ObservationCursor, IdempotencyKey: "http-cancel"}
	response = performJSON(t, handler, "/api/v1/rp/journeys/cancel", rpPlayerToken, cancel)
	assertStatus(t, response, http.StatusOK)
	if outcome := decodeData[storage.RPJourneyCancelRecord](t, response); outcome.Fact.JourneyID != journey.JourneyID {
		t.Fatal("wrong cancelled journey", outcome)
	}
	if diffs, err := s.CompareProjections(ctx, storage.M2DemoInstanceID, storage.M2DemoBranchID); err != nil || len(diffs) != 0 {
		t.Fatal("spatial HTTP flow not replayable", diffs, err)
	}
}
