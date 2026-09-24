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

func TestStudioEventHTTPIdentityAndGrantBoundary(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "studio-http.db")
	s, h := openHTTPTestServer(t, ctx, path)
	defer s.Close()
	const route = "/api/v1/studio/events/read"
	r := storage.StudioEventRequest{InstanceID: storage.DemoInstanceID, BranchID: storage.DemoBranchID, EventID: "event_world_initialized"}
	assertStatus(t, performJSON(t, h, route, "", r), http.StatusUnauthorized)
	assertAPIError(t, performJSON(t, h, route, creatorToken, r), http.StatusForbidden, core.CodeUnauthorized)
	// Disposable fixture provisioning only; no client API can issue this grant.
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.ExecContext(ctx, `INSERT INTO capability_definitions VALUES ('world.inspector.read','Read event and rule evidence','rp8-v1');
	 INSERT INTO capability_grants(grant_id,principal_id,capability_id,instance_id,branch_id,subject_id,field_scope,status,definition_event_id)
	 VALUES ('test-inspect','principal_creator','world.inspector.read',?,?,?,'["event","rule"]','active','event_world_initialized')`, r.InstanceID, r.BranchID, r.BranchID); err != nil {
		t.Fatal(err)
	}
	response := performJSON(t, h, route, creatorToken, r)
	assertStatus(t, response, http.StatusOK)
	out := decodeData[storage.StudioEventEvidence](t, response)
	if out.EventID != r.EventID || out.Rule.EpochID == "" || out.Redacted || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("missing authenticated evidence")
	}
	scopesResponse := performJSON(t, h, "/api/v1/studio/scopes/list", creatorToken, storage.StudioScopeRequest{})
	assertStatus(t, scopesResponse, http.StatusOK)
	scopes := decodeData[storage.StudioScopes](t, scopesResponse)
	if len(scopes.Scopes) != 1 || scopes.Scopes[0].InstanceID != r.InstanceID {
		t.Fatal("HTTP discovery did not filter scopes")
	}
	timelineResponse := performJSON(t, h, "/api/v1/studio/events/list", creatorToken, storage.StudioTimelineRequest{InstanceID: r.InstanceID, BranchID: r.BranchID})
	assertStatus(t, timelineResponse, http.StatusOK)
	timeline := decodeData[storage.StudioTimeline](t, timelineResponse)
	if len(timeline.Events) != 1 || timeline.Events[0].EventID != r.EventID {
		t.Fatal("HTTP timeline differs from read")
	}
	assertStatus(t, performJSON(t, h, "/api/v1/studio/scopes/list", buyerToken, storage.StudioScopeRequest{}), http.StatusForbidden)
	assertStatus(t, performJSON(t, h, "/api/v1/studio/events/list", buyerToken, storage.StudioTimelineRequest{PrincipalID: "principal_creator", InstanceID: r.InstanceID, BranchID: r.BranchID}), http.StatusForbidden)
	assertStatus(t, performJSON(t, h, route, buyerToken, r), http.StatusForbidden)
	assertStatus(t, performJSON(t, h, route, operatorToken, r), http.StatusForbidden)
	r.PrincipalID = "principal_creator"
	assertAPIError(t, performJSON(t, h, route, buyerToken, r), http.StatusForbidden, core.CodeUnauthorized)
	assertStatus(t, performJSON(t, h, route, creatorToken, map[string]string{"event_id": r.EventID, "access_level": "operator"}), http.StatusBadRequest)
	assertStatus(t, performRequest(t, h, http.MethodGet, route, creatorToken, nil), http.StatusMethodNotAllowed)
	assertStatus(t, performJSON(t, h, "/api/v1/studio/access/configure", operatorToken, map[string]string{"target_principal_id": "principal_creator"}), http.StatusNotFound)
	explain := storage.StudioExplanationRequest{InstanceID: r.InstanceID, BranchID: r.BranchID, EventID: r.EventID}
	assertStatus(t, performJSON(t, h, "/api/v1/studio/events/explain", creatorToken, explain), http.StatusForbidden)
	if _, err := db.ExecContext(ctx, `UPDATE capability_grants SET field_scope='["event","rule","explain"]' WHERE grant_id='test-inspect'`); err != nil {
		t.Fatal(err)
	}
	response = performJSON(t, h, "/api/v1/studio/events/explain", creatorToken, explain)
	assertStatus(t, response, http.StatusOK)
	explained := decodeData[storage.StudioExplanation](t, response)
	if explained.EventID != r.EventID || explained.DiagnosticEvidence != "npc_candidate_validation_only" || len(explained.Diagnostics) != 0 {
		t.Fatal("explanation fabricated evidence")
	}
	explain.PrincipalID = "principal_creator"
	assertStatus(t, performJSON(t, h, "/api/v1/studio/events/explain", buyerToken, explain), http.StatusForbidden)
}
