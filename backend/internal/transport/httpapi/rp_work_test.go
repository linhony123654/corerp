package httpapi

import (
	"context"
	"net/http"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/storage"
)

func TestRPWorkHTTPPrivateRead(t *testing.T) {
	ctx := context.Background()
	s, handler := openHTTPTestServer(t, ctx, filepath.Join(t.TempDir(), "work-http.db"))
	defer s.Close()
	if _, err := s.PrepareRPTravel(ctx); err != nil {
		t.Fatal(err)
	}
	response := performJSON(t, handler, "/api/v1/rp/sessions/open", rpPlayerToken, core.RPSessionOpenRequest{InstanceID: storage.M2DemoInstanceID, BranchID: storage.M2DemoBranchID, EntityID: storage.M2RPPlayerID, POV: "second_person", IdempotencyKey: "work-http"})
	assertStatus(t, response, http.StatusOK)
	session := decodeData[storage.RPSession](t, response)
	read := core.RPSessionReadRequest{SessionID: session.SessionID}
	response = performJSON(t, handler, "/api/v1/rp/work/read", rpPlayerToken, read)
	assertStatus(t, response, http.StatusOK)
	work := decodeData[storage.RPWork](t, response)
	if work.Jobs == nil || work.Appointments == nil || work.WorldTime == "" {
		t.Fatal("missing work snapshot/empty arrays")
	}
	response = performJSON(t, handler, "/api/v1/rp/work/read", creatorToken, read)
	assertAPIError(t, response, http.StatusNotFound, core.CodeNotFound)
	response = performJSON(t, handler, "/api/v1/rp/work/read", "", read)
	assertStatus(t, response, http.StatusUnauthorized)
	read.PrincipalID = "principal_creator"
	response = performJSON(t, handler, "/api/v1/rp/work/read", rpPlayerToken, read)
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)
}
