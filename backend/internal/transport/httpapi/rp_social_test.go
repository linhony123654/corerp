package httpapi

import (
	"context"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/storage"
)

func TestRPSocialHTTPAuthorizationRealHistoryAndNoPrivateRelationshipLeak(t *testing.T) {
	ctx := context.Background()
	store, handler := openHTTPTestServer(t, ctx, filepath.Join(t.TempDir(), "social-http.db"))
	defer store.Close()
	if _, err := store.PrepareRPTravel(ctx); err != nil {
		t.Fatal(err)
	}
	response := performJSON(t, handler, "/api/v1/rp/sessions/open", rpPlayerToken, core.RPSessionOpenRequest{InstanceID: storage.M2DemoInstanceID, BranchID: storage.M2DemoBranchID, EntityID: storage.M2RPPlayerID, POV: "second_person", IdempotencyKey: "social-http"})
	assertStatus(t, response, http.StatusOK)
	session := decodeData[storage.RPSession](t, response)
	read := core.RPSessionReadRequest{SessionID: session.SessionID}
	response = performJSON(t, handler, "/api/v1/rp/observe", rpPlayerToken, read)
	assertStatus(t, response, http.StatusOK)
	view := decodeData[storage.RPObservation](t, response)
	request := core.RPSocialRequest{SessionID: session.SessionID, TargetEntityID: storage.M2RPNPCID, Action: "greet", ExpectedCursor: view.ObservationCursor, IdempotencyKey: "greet"}
	response = performJSON(t, handler, "/api/v1/rp/actions/social", creatorToken, request)
	assertAPIError(t, response, http.StatusNotFound, core.CodeNotFound)
	spoof := request
	spoof.PrincipalID = storage.M2RPPlayerPrincipal
	response = performJSON(t, handler, "/api/v1/rp/actions/social", creatorToken, spoof)
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)
	response = performJSON(t, handler, "/api/v1/rp/actions/social", rpPlayerToken, request)
	assertStatus(t, response, http.StatusOK)
	accepted := decodeData[storage.RPSocialResult](t, response)
	response = performJSON(t, handler, "/api/v1/rp/actions/social", rpPlayerToken, request)
	assertStatus(t, response, http.StatusOK)
	if retry := decodeData[storage.RPSocialResult](t, response); !retry.Replayed || retry.EventID != accepted.EventID {
		t.Fatal("HTTP retry duplicated social fact")
	}
	response = performJSON(t, handler, "/api/v1/rp/observe", rpPlayerToken, read)
	assertStatus(t, response, http.StatusOK)
	for _, hidden := range []string{"familiarity", "affinity", "liability_minor", "disposition"} {
		if strings.Contains(response.Body.String(), hidden) {
			t.Fatalf("private relationship leaked %s", hidden)
		}
	}
	after := decodeData[storage.RPObservation](t, response)
	if len(after.RecentTurns) != 1 || !strings.Contains(after.RecentTurns[0].NarrativeLines[0], "挥手致意") {
		t.Fatalf("actual action missing in Play history %+v", after)
	}
}
