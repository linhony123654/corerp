package httpapi

import (
	"context"
	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/storage"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
)

type initiativeHTTPProvider func(context.Context, core.RPDecisionInput) (core.RPDecisionProposal, error)

func (f initiativeHTTPProvider) Propose(ctx context.Context, input core.RPDecisionInput) (core.RPDecisionProposal, error) {
	return f(ctx, input)
}

func TestRPWaitHTTPUsesConfiguredInitiativeProviderAndVisibleHistory(t *testing.T) {
	ctx := context.Background()
	store, _ := openHTTPTestServer(t, ctx, filepath.Join(t.TempDir(), "initiative-http.db"))
	defer store.Close()
	if _, err := store.PrepareRPTravel(ctx); err != nil {
		t.Fatal(err)
	}
	calls := 0
	provider := initiativeHTTPProvider(func(_ context.Context, input core.RPDecisionInput) (core.RPDecisionProposal, error) {
		calls++
		if input.Trigger == nil || input.PlayerSpeechText != "" || input.SpeechEventID != "" || input.NPCEntityID != storage.M2RPNPCID {
			t.Fatalf("bad initiative boundary %+v", input)
		}
		return core.RPDecisionProposal{Action: "respond", Text: "我想起一件事，想和你聊聊。"}, nil
	})
	service, err := storage.NewRPService(store, provider, "chat_completions")
	if err != nil {
		t.Fatal(err)
	}
	auth, err := NewStaticTokenAuthenticator(map[string]string{rpPlayerToken: storage.M2RPPlayerPrincipal, creatorToken: "principal_creator"})
	if err != nil {
		t.Fatal(err)
	}
	cursors, err := NewCursorCodec([]byte("initiative-test-cursor-secret-32-bytes"))
	if err != nil {
		t.Fatal(err)
	}
	server, err := New(service, auth, cursors)
	if err != nil {
		t.Fatal(err)
	}
	handler := server.Handler()
	response := performJSON(t, handler, "/api/v1/rp/sessions/open", rpPlayerToken, core.RPSessionOpenRequest{InstanceID: storage.M2DemoInstanceID, BranchID: storage.M2DemoBranchID, EntityID: storage.M2RPPlayerID, POV: "second_person", IdempotencyKey: "initiative-http"})
	assertStatus(t, response, http.StatusOK)
	session := decodeData[storage.RPSession](t, response)
	read := core.RPSessionReadRequest{SessionID: session.SessionID}
	response = performJSON(t, handler, "/api/v1/rp/observe", rpPlayerToken, read)
	assertStatus(t, response, http.StatusOK)
	view := decodeData[storage.RPObservation](t, response)
	r := core.RPWaitRequest{OpportunityIntent: "social", SessionID: session.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: "2026-09-22T03:00:00Z", Budget: 20, IdempotencyKey: "wait"}
	bad := r
	bad.OpportunityIntent = "force_drama"
	response = performJSON(t, handler, "/api/v1/rp/actions/wait", rpPlayerToken, bad)
	assertAPIError(t, response, http.StatusBadRequest, core.CodeInvalidArgument)
	response = performJSON(t, handler, "/api/v1/rp/actions/wait", creatorToken, r)
	assertAPIError(t, response, http.StatusNotFound, core.CodeNotFound)
	if calls != 0 {
		t.Fatal("unauthorized caller triggered provider")
	}
	response = performJSON(t, handler, "/api/v1/rp/actions/wait", rpPlayerToken, r)
	assertStatus(t, response, http.StatusOK)
	result := decodeData[storage.RPWaitResult](t, response)
	if calls != 1 || len(result.Initiatives) != 1 || result.Initiatives[0].Action != "respond" {
		t.Fatalf("HTTP did not drain initiative %+v calls=%d", result, calls)
	}
	response = performJSON(t, handler, "/api/v1/rp/actions/wait", rpPlayerToken, r)
	assertStatus(t, response, http.StatusOK)
	if calls != 1 || !decodeData[storage.RPWaitResult](t, response).Initiatives[0].Replayed {
		t.Fatal("wait retry repeated model")
	}
	bad = r
	bad.OpportunityIntent = ""
	response = performJSON(t, handler, "/api/v1/rp/actions/wait", rpPlayerToken, bad)
	assertAPIError(t, response, http.StatusConflict, core.CodeIdempotencyMismatch)
	response = performJSON(t, handler, "/api/v1/rp/observe", rpPlayerToken, read)
	assertStatus(t, response, http.StatusOK)
	if !strings.Contains(response.Body.String(), "我想起一件事") {
		t.Fatal("initiative absent from scene history")
	}
	for _, private := range []string{"input_hash", "proposal", "liability_minor", "disposition"} {
		if strings.Contains(response.Body.String(), private) {
			t.Fatalf("private initiative data exposed: %s", private)
		}
	}
}
