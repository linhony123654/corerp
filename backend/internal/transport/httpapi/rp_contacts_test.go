package httpapi

import (
	"context"
	"net/http"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/storage"
)

func TestRPContactsHTTPScopedRead(t *testing.T) {
	ctx := context.Background()
	s, handler := openHTTPTestServer(t, ctx, filepath.Join(t.TempDir(), "contacts-http.db"))
	defer s.Close()
	if _, err := s.PrepareRPTravel(ctx); err != nil {
		t.Fatal(err)
	}
	response := performJSON(t, handler, "/api/v1/rp/sessions/open", rpPlayerToken, core.RPSessionOpenRequest{InstanceID: storage.M2DemoInstanceID, BranchID: storage.M2DemoBranchID, EntityID: storage.M2RPPlayerID, POV: "second_person", IdempotencyKey: "contacts-http"})
	assertStatus(t, response, http.StatusOK)
	session := decodeData[storage.RPSession](t, response)
	read := storage.RPContactsReadRequest{SessionID: session.SessionID}
	response = performJSON(t, handler, "/api/v1/rp/contacts/read", rpPlayerToken, read)
	assertStatus(t, response, http.StatusOK)
	result := decodeData[storage.RPContacts](t, response)
	if result.Contacts == nil || result.WorldTime == "" {
		t.Fatal("missing empty/snapshot contract")
	}
	response = performJSON(t, handler, "/api/v1/rp/contacts/read", creatorToken, read)
	assertAPIError(t, response, http.StatusNotFound, core.CodeNotFound)
	response = performJSON(t, handler, "/api/v1/rp/contacts/read", "", read)
	assertStatus(t, response, http.StatusUnauthorized)
	read.PrincipalID = "principal_creator"
	response = performJSON(t, handler, "/api/v1/rp/contacts/read", rpPlayerToken, read)
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)
}
