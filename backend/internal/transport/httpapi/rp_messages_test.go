package httpapi

import (
	"context"
	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/storage"
	"net/http"
	"path/filepath"
	"testing"
)

func TestRPMessagesHTTPScopedRead(t *testing.T) {
	ctx := context.Background()
	s, handler := openHTTPTestServer(t, ctx, filepath.Join(t.TempDir(), "messages-http.db"))
	defer s.Close()
	if _, err := s.PrepareRPTravel(ctx); err != nil {
		t.Fatal(err)
	}
	response := performJSON(t, handler, "/api/v1/rp/sessions/open", rpPlayerToken, core.RPSessionOpenRequest{InstanceID: storage.M2DemoInstanceID, BranchID: storage.M2DemoBranchID, EntityID: storage.M2RPPlayerID, POV: "second_person", IdempotencyKey: "messages-http"})
	assertStatus(t, response, http.StatusOK)
	session := decodeData[storage.RPSession](t, response)
	read := storage.RPMessagesReadRequest{SessionID: session.SessionID}
	response = performJSON(t, handler, "/api/v1/rp/messages/read", rpPlayerToken, read)
	assertStatus(t, response, http.StatusOK)
	view := decodeData[storage.RPMessages](t, response)
	if view.Messages == nil || len(view.Messages) != 0 || view.WorldTime == "" {
		t.Fatal("missing empty own-message contract")
	}
	response = performJSON(t, handler, "/api/v1/rp/messages/read", creatorToken, read)
	assertAPIError(t, response, http.StatusNotFound, core.CodeNotFound)
	response = performJSON(t, handler, "/api/v1/rp/messages/read", "", read)
	assertStatus(t, response, http.StatusUnauthorized)
	read.PrincipalID = "principal_creator"
	response = performJSON(t, handler, "/api/v1/rp/messages/read", rpPlayerToken, read)
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)
}
