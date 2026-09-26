package httpapi

import (
	"context"
	"database/sql"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/storage"
)

func TestRPInformationHTTPDirectSendHasBoundedReceiptAndDelayedDelivery(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "information-http.db")
	store, handler := openHTTPTestServer(t, ctx, path)
	defer store.Close()
	if _, err := store.PrepareRPTravel(ctx); err != nil {
		t.Fatal(err)
	}
	response := performJSON(t, handler, "/api/v1/rp/sessions/open", rpPlayerToken, core.RPSessionOpenRequest{
		InstanceID: storage.M2DemoInstanceID, BranchID: storage.M2DemoBranchID,
		EntityID: storage.M2RPPlayerID, POV: "second_person", IdempotencyKey: "http-information-session",
	})
	assertStatus(t, response, http.StatusOK)
	session := decodeData[storage.RPSession](t, response)
	read := core.RPSessionReadRequest{SessionID: session.SessionID}
	response = performJSON(t, handler, "/api/v1/rp/observe", rpPlayerToken, read)
	assertStatus(t, response, http.StatusOK)
	initial := decodeData[storage.RPObservation](t, response)
	response = performJSON(t, handler, "/api/v1/rp/actions/move", rpPlayerToken, core.RPMoveRequest{
		SessionID: session.SessionID, FromPlaceID: initial.PlaceID, ToPlaceID: "place_m2_home_ada",
		ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "http-information-separate",
	})
	assertStatus(t, response, http.StatusOK)
	response = performJSON(t, handler, "/api/v1/rp/observe", rpPlayerToken, read)
	assertStatus(t, response, http.StatusOK)
	view := decodeData[storage.RPObservation](t, response)
	request := storage.RPInformationSendRequest{Binding: core.CareerBinding{
		InstanceID: storage.M2DemoInstanceID, BranchID: storage.M2DemoBranchID,
		ExpectedHead: view.ObservationCursor, IdempotencyKey: "http-information-send",
	}, SessionID: session.SessionID, MessageID: "http-information-message",
		RecipientEntityID: storage.M2RPNPCID, Text: "排班调整只是未经核实的消息。"}
	request.AllowRelay = true
	assertAPIError(t, performJSON(t, handler, "/api/v1/rp/information/direct/send", "", request), http.StatusUnauthorized, core.CodeUnauthenticated)
	assertAPIError(t, performJSON(t, handler, "/api/v1/rp/information/direct/send", creatorToken, request), http.StatusNotFound, core.CodeNotFound)
	unknown := request
	unknown.RecipientEntityID = storage.M2AgentAdaID
	assertAPIError(t, performJSON(t, handler, "/api/v1/rp/information/direct/send", rpPlayerToken, unknown), http.StatusNotFound, core.CodeNotFound)
	response = performJSON(t, handler, "/api/v1/rp/information/direct/send", rpPlayerToken, request)
	assertStatus(t, response, http.StatusOK)
	receipt := decodeData[rpInformationSendReceipt](t, response)
	if receipt.MessageID != request.MessageID || receipt.DeliveryDueWorldTime == "" || receipt.EventSequence <= view.ObservationCursor || receipt.Replayed {
		t.Fatalf("direct send receipt: %+v", receipt)
	}
	for _, private := range []string{request.Text, request.RecipientEntityID, storage.M2RPPlayerID, "event_id", "source_event_id", "recipient_id", "sender_id"} {
		if strings.Contains(response.Body.String(), private) {
			t.Fatalf("direct send receipt leaked %q", private)
		}
	}
	response = performJSON(t, handler, "/api/v1/rp/information/direct/send", rpPlayerToken, request)
	assertStatus(t, response, http.StatusOK)
	if replay := decodeData[rpInformationSendReceipt](t, response); !replay.Replayed || replay.EventSequence != receipt.EventSequence {
		t.Fatalf("HTTP exact replay: %+v", replay)
	}
	changed := request
	changed.Text = "不同内容"
	assertAPIError(t, performJSON(t, handler, "/api/v1/rp/information/direct/send", rpPlayerToken, changed), http.StatusConflict, core.CodeIdempotencyMismatch)
	response = performJSON(t, handler, "/api/v1/rp/observe", rpPlayerToken, read)
	assertStatus(t, response, http.StatusOK)
	view = decodeData[storage.RPObservation](t, response)
	response = performJSON(t, handler, "/api/v1/rp/actions/wait", rpPlayerToken, core.RPWaitRequest{
		SessionID: session.SessionID, TargetWorldTime: receipt.DeliveryDueWorldTime,
		ExpectedCursor: view.ObservationCursor, Budget: 1000, IdempotencyKey: "http-information-delivery",
	})
	assertStatus(t, response, http.StatusOK)
	if wait := decodeData[storage.RPWaitResult](t, response); wait.Status != "completed" {
		t.Fatalf("HTTP wait did not deliver message: %+v", wait)
	}
	// Test-only second control grant allows this real HTTP client to inspect
	// the NPC recipient without adding a production privilege-changing API.
	fixture, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer fixture.Close()
	if _, err := fixture.ExecContext(ctx, `INSERT INTO capability_grants(grant_id,principal_id,capability_id,instance_id,branch_id,subject_id,field_scope,status,definition_event_id)
 SELECT 'information_http_cai_control',principal_id,capability_id,instance_id,branch_id,?,'[]','active',definition_event_id
 FROM capability_grants WHERE grant_id='grant_m2_rp_player_control'`, storage.M2RPNPCID); err != nil {
		t.Fatal(err)
	}
	response = performJSON(t, handler, "/api/v1/rp/sessions/open", rpPlayerToken, core.RPSessionOpenRequest{
		InstanceID: storage.M2DemoInstanceID, BranchID: storage.M2DemoBranchID,
		EntityID: storage.M2RPNPCID, POV: "second_person", IdempotencyKey: "http-information-cai-session",
	})
	assertStatus(t, response, http.StatusOK)
	cai := decodeData[storage.RPSession](t, response)
	caiRead := core.RPSessionReadRequest{SessionID: cai.SessionID}
	response = performJSON(t, handler, "/api/v1/rp/context/read", rpPlayerToken, storage.RPContextReadRequest{SessionID: cai.SessionID})
	assertStatus(t, response, http.StatusOK)
	contextView := decodeData[storage.RPClientContext](t, response)
	found := false
	for _, fact := range contextView.Facts {
		if fact.Kind == "message_received" && fact.MessageID == request.MessageID &&
			fact.Text == request.Text && fact.Reliability == "unverified" && fact.Stance == "" {
			found = true
		}
	}
	if !found {
		t.Fatalf("HTTP recipient did not see delivered uncertain claim: %+v", contextView.Facts)
	}
	response = performJSON(t, handler, "/api/v1/rp/observe", rpPlayerToken, caiRead)
	assertStatus(t, response, http.StatusOK)
	caiView := decodeData[storage.RPObservation](t, response)
	stance := storage.RPInformationStanceRequest{Binding: core.CareerBinding{
		InstanceID: storage.M2DemoInstanceID, BranchID: storage.M2DemoBranchID,
		ExpectedHead: caiView.ObservationCursor, IdempotencyKey: "http-information-believe",
	}, SessionID: cai.SessionID, MessageID: request.MessageID, Stance: "believe", Reason: "我暂时相信。"}
	assertAPIError(t, performJSON(t, handler, "/api/v1/rp/information/stance/record", "", stance), http.StatusUnauthorized, core.CodeUnauthenticated)
	assertAPIError(t, performJSON(t, handler, "/api/v1/rp/information/stance/record", creatorToken, stance), http.StatusNotFound, core.CodeNotFound)
	response = performJSON(t, handler, "/api/v1/rp/information/stance/record", rpPlayerToken, stance)
	assertStatus(t, response, http.StatusOK)
	belief := decodeData[rpInformationStanceReceipt](t, response)
	if belief.MessageID != request.MessageID || belief.Stance != "believe" || belief.Replayed {
		t.Fatalf("HTTP stance receipt: %+v", belief)
	}
	for _, private := range []string{stance.Reason, storage.M2RPNPCID, "source_send_event_id", "source_delivery_event_id", "previous_stance_event_id"} {
		if strings.Contains(response.Body.String(), private) {
			t.Fatalf("stance receipt leaked %q", private)
		}
	}
	response = performJSON(t, handler, "/api/v1/rp/information/stance/record", rpPlayerToken, stance)
	assertStatus(t, response, http.StatusOK)
	if replay := decodeData[rpInformationStanceReceipt](t, response); !replay.Replayed || replay.EventSequence != belief.EventSequence {
		t.Fatalf("HTTP stance replay: %+v", replay)
	}
	response = performJSON(t, handler, "/api/v1/rp/context/read", rpPlayerToken, storage.RPContextReadRequest{SessionID: cai.SessionID})
	assertStatus(t, response, http.StatusOK)
	contextView = decodeData[storage.RPClientContext](t, response)
	found = false
	for _, fact := range contextView.Facts {
		found = found || fact.MessageID == request.MessageID && fact.Stance == "believe"
	}
	if !found {
		t.Fatalf("HTTP recipient current stance missing: %+v", contextView.Facts)
	}
	if diff, err := store.CompareProjections(ctx, storage.M2DemoInstanceID, storage.M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatalf("HTTP send/delivery/stance diverged: %+v %v", diff, err)
	}
	response = performJSON(t, handler, "/api/v1/rp/observe", rpPlayerToken, caiRead)
	assertStatus(t, response, http.StatusOK)
	caiView = decodeData[storage.RPObservation](t, response)
	relay := storage.RPInformationRelayRequest{Binding: core.CareerBinding{
		InstanceID: storage.M2DemoInstanceID, BranchID: storage.M2DemoBranchID,
		ExpectedHead: caiView.ObservationCursor, IdempotencyKey: "http-information-relay",
	}, SessionID: cai.SessionID, MessageID: "http-rumor-to-lin",
		ForwardedMessageID: request.MessageID, RecipientEntityID: storage.M2RPPlayerID}
	assertAPIError(t, performJSON(t, handler, "/api/v1/rp/information/rumor/relay", "", relay), http.StatusUnauthorized, core.CodeUnauthenticated)
	assertAPIError(t, performJSON(t, handler, "/api/v1/rp/information/rumor/relay", creatorToken, relay), http.StatusNotFound, core.CodeNotFound)
	response = performJSON(t, handler, "/api/v1/rp/information/rumor/relay", rpPlayerToken, relay)
	assertStatus(t, response, http.StatusOK)
	rumor := decodeData[rpInformationSendReceipt](t, response)
	if rumor.MessageID != relay.MessageID || rumor.Replayed || rumor.DeliveryDueWorldTime == "" {
		t.Fatalf("rumor receipt: %+v", rumor)
	}
	for _, private := range []string{request.Text, relay.ForwardedMessageID, relay.RecipientEntityID,
		"forwarded_from_send_event_id", "forwarded_from_delivery_event_id", "sender_id"} {
		if strings.Contains(response.Body.String(), private) {
			t.Fatalf("rumor receipt leaked %q", private)
		}
	}
	response = performJSON(t, handler, "/api/v1/rp/information/rumor/relay", rpPlayerToken, relay)
	assertStatus(t, response, http.StatusOK)
	if replay := decodeData[rpInformationSendReceipt](t, response); !replay.Replayed || replay.EventSequence != rumor.EventSequence {
		t.Fatalf("rumor exact replay: %+v", replay)
	}
	response = performJSON(t, handler, "/api/v1/rp/observe", rpPlayerToken, read)
	assertStatus(t, response, http.StatusOK)
	view = decodeData[storage.RPObservation](t, response)
	response = performJSON(t, handler, "/api/v1/rp/actions/wait", rpPlayerToken, core.RPWaitRequest{
		SessionID: session.SessionID, TargetWorldTime: rumor.DeliveryDueWorldTime,
		ExpectedCursor: view.ObservationCursor, Budget: 1000, IdempotencyKey: "http-information-rumor-delivery",
	})
	assertStatus(t, response, http.StatusOK)
	response = performJSON(t, handler, "/api/v1/rp/context/read", rpPlayerToken, storage.RPContextReadRequest{SessionID: session.SessionID})
	assertStatus(t, response, http.StatusOK)
	linContext := decodeData[storage.RPClientContext](t, response)
	found = false
	for _, fact := range linContext.Facts {
		found = found || fact.MessageID == relay.MessageID && fact.Channel == "rumor" && fact.Forwarded && !fact.MayRelay
	}
	if !found {
		t.Fatalf("HTTP rumor missing from recipient context: %+v", linContext.Facts)
	}
}
