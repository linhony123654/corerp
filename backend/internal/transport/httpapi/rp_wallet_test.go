package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/storage"
)

func TestRPWalletHTTPPrivateExactRead(t *testing.T) {
	ctx := context.Background()
	s, handler := openHTTPTestServer(t, ctx, filepath.Join(t.TempDir(), "wallet-http.db"))
	defer s.Close()
	if _, err := s.PrepareRPTravel(ctx); err != nil {
		t.Fatal(err)
	}
	response := performJSON(t, handler, "/api/v1/rp/sessions/open", rpPlayerToken, core.RPSessionOpenRequest{InstanceID: storage.M2DemoInstanceID, BranchID: storage.M2DemoBranchID, EntityID: storage.M2RPPlayerID, POV: "second_person", IdempotencyKey: "wallet-http"})
	assertStatus(t, response, http.StatusOK)
	session := decodeData[storage.RPSession](t, response)
	read := core.RPSessionReadRequest{SessionID: session.SessionID}
	response = performJSON(t, handler, "/api/v1/rp/wallet/read", rpPlayerToken, read)
	assertStatus(t, response, http.StatusOK)
	wallet := decodeData[storage.RPWallet](t, response)
	fields := decodeData[map[string]json.RawMessage](t, response)
	var exactBalance string
	if err := json.Unmarshal(fields["balance_minor"], &exactBalance); err != nil || exactBalance == "" || wallet.CurrencyID == "" || len(fields) != 6 {
		t.Fatalf("wallet contract exposes extra fields or loses precision: %s", response.Body.String())
	}
	response = performJSON(t, handler, "/api/v1/rp/wallet/read", creatorToken, read)
	assertAPIError(t, response, http.StatusNotFound, core.CodeNotFound)
	response = performJSON(t, handler, "/api/v1/rp/wallet/read", "", read)
	assertStatus(t, response, http.StatusUnauthorized)
	read.PrincipalID = "principal_creator"
	response = performJSON(t, handler, "/api/v1/rp/wallet/read", rpPlayerToken, read)
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)
}
