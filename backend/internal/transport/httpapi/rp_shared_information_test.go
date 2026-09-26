package httpapi

import (
	"context"
	"database/sql"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/storage"
)

func TestRPSharedInformationHTTPExternalSendRequiresHumanAndBoundedReceipt(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "shared-information-http.db")
	store, err := storage.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.BootstrapDemo(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PrepareRPTravel(ctx); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.ExecContext(ctx, `INSERT INTO principals(principal_id,principal_type,display_name,status) VALUES (?,'service','Information service','active')`, roundServiceAID); err != nil {
		t.Fatal(err)
	}
	head := func() int64 {
		var sequence int64
		if err := db.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, storage.M2DemoInstanceID, storage.M2DemoBranchID).Scan(&sequence); err != nil {
			t.Fatal(err)
		}
		return sequence
	}
	binding := func(key string) core.CareerBinding {
		return core.CareerBinding{PrincipalID: "principal_operator", InstanceID: storage.M2DemoInstanceID,
			BranchID: storage.M2DemoBranchID, ExpectedHead: head(), IdempotencyKey: key}
	}
	if _, err := store.EnrollRPExternalControllerLocal(ctx, storage.RPExternalControllerEnrollmentRequest{
		Binding: binding("enroll-info-http"), EntityID: storage.M2RPNPCID,
		ControllerPrincipalID: roundServiceAID, ControllerInstanceID: "controller-info-http"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AssignRPExternalControllerLocal(ctx, storage.RPExternalControllerAssignmentRequest{
		Binding: binding("assign-info-http"), EntityID: storage.M2RPNPCID, ExpectedGeneration: 0}); err != nil {
		t.Fatal(err)
	}
	handler := newSharedRoundHTTPHandler(t, store)
	open := func(token, entity, key string) string {
		response := performJSON(t, handler, "/api/v1/rp/sessions/open", token, core.RPSessionOpenRequest{
			InstanceID: storage.M2DemoInstanceID, BranchID: storage.M2DemoBranchID,
			EntityID: entity, POV: "second_person", IdempotencyKey: key})
		assertStatus(t, response, http.StatusOK)
		return decodeData[storage.RPSession](t, response).SessionID
	}
	human := open(rpPlayerToken, storage.M2RPPlayerID, "info-http-human")
	service := open(roundServiceAToken, storage.M2RPNPCID, "info-http-service")
	var baseline string
	for _, participant := range []struct{ token, session string }{{rpPlayerToken, human}, {roundServiceAToken, service}} {
		response := performJSON(t, handler, "/api/v1/rp/observe", participant.token, core.RPSessionReadRequest{SessionID: participant.session})
		assertStatus(t, response, http.StatusOK)
		baseline = decodeData[storage.RPObservation](t, response).WorldTime
	}
	response := performJSON(t, handler, "/api/v1/rp/rounds/open", operatorToken, storage.RPSharedRoundOpenRequest{
		Binding: binding("open-info-http"), HumanSessionID: human, ExternalSessionIDs: []string{service}})
	assertStatus(t, response, http.StatusOK)
	round := decodeData[storage.RPSharedRound](t, response)
	proposal := storage.RPSharedInformationSendRequest{SessionID: service, RoundID: round.RoundID,
		MessageID: "info-http-to-lin", RecipientEntityID: storage.M2RPPlayerID,
		Text: "经共享回合发出的私信。", AllowRelay: true, IdempotencyKey: "info-http-proposal"}
	assertAPIError(t, performJSON(t, handler, "/api/v1/rp/rounds/information-send", "", proposal), http.StatusUnauthorized, core.CodeUnauthenticated)
	assertAPIError(t, performJSON(t, handler, "/api/v1/rp/rounds/information-send", rpPlayerToken, proposal), http.StatusNotFound, core.CodeNotFound)
	response = performJSON(t, handler, "/api/v1/rp/rounds/information-send", roundServiceAToken, proposal)
	assertStatus(t, response, http.StatusOK)
	if pending := decodeData[storage.RPSharedRound](t, response); pending.Status != "open" || pending.Submitted != 1 {
		t.Fatal("HTTP proposal changed world before Human", pending)
	}
	for _, secret := range []string{proposal.Text, proposal.RecipientEntityID, storage.M2RPNPCID, "request_json", "source_event_id"} {
		if strings.Contains(response.Body.String(), secret) {
			t.Fatal("HTTP proposal receipt leaked information", secret, response.Body.String())
		}
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE event_type='RPInformationSent'`).Scan(&count); err != nil || count != 0 {
		t.Fatal("information Event before Human answer", count, err)
	}
	at, err := time.Parse(time.RFC3339Nano, baseline)
	if err != nil {
		t.Fatal(err)
	}
	response = performJSON(t, handler, "/api/v1/rp/rounds/wait", rpPlayerToken, storage.RPSharedWaitRequest{
		SessionID: human, RoundID: round.RoundID, HorizonWorldTime: at.Add(10 * time.Minute).Format(time.RFC3339Nano),
		IdempotencyKey: "info-http-human-wait"})
	assertStatus(t, response, http.StatusOK)
	response = performJSON(t, handler, "/api/v1/rp/rounds/advance", roundServiceAToken, storage.RPSharedRoundAdvanceRequest{
		RPSharedRoundReadRequest: storage.RPSharedRoundReadRequest{SessionID: service, RoundID: round.RoundID}, Budget: 100})
	assertStatus(t, response, http.StatusOK)
	if settled := decodeData[storage.RPSharedRound](t, response); settled.Status != "settled" || settled.OwnDisposition != "action_accepted" {
		t.Fatal("HTTP shared information send not accepted", settled)
	}
	if strings.Contains(response.Body.String(), proposal.Text) || strings.Contains(response.Body.String(), proposal.RecipientEntityID) ||
		strings.Contains(response.Body.String(), "event_rp_information_") {
		t.Fatal("HTTP settled receipt leaked information", response.Body.String())
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE event_type='RPInformationSent' AND json_extract(payload,'$.message_id')=?`, proposal.MessageID).Scan(&count); err != nil || count != 1 {
		t.Fatal("HTTP shared send did not commit one Event", count, err)
	}
	if diff, err := store.CompareProjections(ctx, storage.M2DemoInstanceID, storage.M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("HTTP shared information source differs", diff, err)
	}
	var due string
	if err := db.QueryRowContext(ctx, `SELECT json_extract(payload,'$.deliver_world_time') FROM events WHERE event_type='RPInformationSent' AND json_extract(payload,'$.message_id')=?`, proposal.MessageID).Scan(&due); err != nil {
		t.Fatal(err)
	}
	for _, participant := range []struct{ token, session string }{{rpPlayerToken, human}, {roundServiceAToken, service}} {
		response = performJSON(t, handler, "/api/v1/rp/observe", participant.token, core.RPSessionReadRequest{SessionID: participant.session})
		assertStatus(t, response, http.StatusOK)
	}
	response = performJSON(t, handler, "/api/v1/rp/rounds/open", operatorToken, storage.RPSharedRoundOpenRequest{
		Binding: binding("open-info-http-delivery"), HumanSessionID: human, ExternalSessionIDs: []string{service}})
	assertStatus(t, response, http.StatusOK)
	deliveryRound := decodeData[storage.RPSharedRound](t, response)
	for _, participant := range []struct{ token, session, key string }{
		{rpPlayerToken, human, "info-http-human-delivery"}, {roundServiceAToken, service, "info-http-service-delivery"},
	} {
		response = performJSON(t, handler, "/api/v1/rp/rounds/wait", participant.token, storage.RPSharedWaitRequest{
			SessionID: participant.session, RoundID: deliveryRound.RoundID, HorizonWorldTime: due,
			IdempotencyKey: participant.key})
		assertStatus(t, response, http.StatusOK)
	}
	response = performJSON(t, handler, "/api/v1/rp/rounds/advance", rpPlayerToken, storage.RPSharedRoundAdvanceRequest{
		RPSharedRoundReadRequest: storage.RPSharedRoundReadRequest{SessionID: human, RoundID: deliveryRound.RoundID}, Budget: 1000})
	assertStatus(t, response, http.StatusOK)
	if delivered := decodeData[storage.RPSharedRound](t, response); delivered.Status != "settled" {
		t.Fatal("HTTP shared wait did not deliver actual message", delivered)
	}
	contextResponse := performJSON(t, handler, "/api/v1/rp/context/read", rpPlayerToken, storage.RPContextReadRequest{SessionID: human})
	assertStatus(t, contextResponse, http.StatusOK)
	if !strings.Contains(contextResponse.Body.String(), proposal.MessageID) {
		t.Fatal("Human did not receive message before stance proposal", contextResponse.Body.String())
	}
	for _, participant := range []struct{ token, session string }{{rpPlayerToken, human}, {roundServiceAToken, service}} {
		response = performJSON(t, handler, "/api/v1/rp/observe", participant.token, core.RPSessionReadRequest{SessionID: participant.session})
		assertStatus(t, response, http.StatusOK)
		baseline = decodeData[storage.RPObservation](t, response).WorldTime
	}
	response = performJSON(t, handler, "/api/v1/rp/rounds/open", operatorToken, storage.RPSharedRoundOpenRequest{
		Binding: binding("open-info-http-stance"), HumanSessionID: human, ExternalSessionIDs: []string{service}})
	assertStatus(t, response, http.StatusOK)
	stanceRound := decodeData[storage.RPSharedRound](t, response)
	stance := storage.RPSharedInformationStanceRequest{SessionID: human, RoundID: stanceRound.RoundID,
		MessageID: proposal.MessageID, Stance: "doubt", Reason: "The sender might be mistaken.",
		IdempotencyKey: "info-http-stance-proposal"}
	assertAPIError(t, performJSON(t, handler, "/api/v1/rp/rounds/information-stance", "", stance), http.StatusUnauthorized, core.CodeUnauthenticated)
	assertAPIError(t, performJSON(t, handler, "/api/v1/rp/rounds/information-stance", roundServiceAToken, stance), http.StatusNotFound, core.CodeNotFound)
	response = performJSON(t, handler, "/api/v1/rp/rounds/information-stance", rpPlayerToken, stance)
	assertStatus(t, response, http.StatusOK)
	if pending := decodeData[storage.RPSharedRound](t, response); pending.Status != "open" || pending.Submitted != 1 {
		t.Fatal("HTTP stance proposal committed before external answer", pending)
	}
	if strings.Contains(response.Body.String(), stance.Reason) || strings.Contains(response.Body.String(), proposal.MessageID) {
		t.Fatal("HTTP stance proposal leaked private claim/interpretation", response.Body.String())
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE event_type='RPInformationStanceRecorded'`).Scan(&count); err != nil || count != 0 {
		t.Fatal("HTTP stance Event before external answer", count, err)
	}
	current, err := time.Parse(time.RFC3339Nano, baseline)
	if err != nil {
		t.Fatal(err)
	}
	response = performJSON(t, handler, "/api/v1/rp/rounds/wait", roundServiceAToken, storage.RPSharedWaitRequest{
		SessionID: service, RoundID: stanceRound.RoundID,
		HorizonWorldTime: current.Add(10 * time.Minute).Format(time.RFC3339Nano), IdempotencyKey: "info-http-service-stance-wait"})
	assertStatus(t, response, http.StatusOK)
	response = performJSON(t, handler, "/api/v1/rp/rounds/advance", rpPlayerToken, storage.RPSharedRoundAdvanceRequest{
		RPSharedRoundReadRequest: storage.RPSharedRoundReadRequest{SessionID: human, RoundID: stanceRound.RoundID}, Budget: 100})
	assertStatus(t, response, http.StatusOK)
	if settled := decodeData[storage.RPSharedRound](t, response); settled.Status != "settled" || settled.OwnDisposition != "action_accepted" {
		t.Fatal("HTTP shared stance did not settle", settled)
	}
	if strings.Contains(response.Body.String(), stance.Reason) || strings.Contains(response.Body.String(), proposal.MessageID) {
		t.Fatal("HTTP settled stance receipt leaked private claim/interpretation", response.Body.String())
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE event_type='RPInformationStanceRecorded' AND json_extract(payload,'$.message_id')=?`, proposal.MessageID).Scan(&count); err != nil || count != 1 {
		t.Fatal("HTTP shared stance did not commit exactly one Event", count, err)
	}
	if diff, err := store.CompareProjections(ctx, storage.M2DemoInstanceID, storage.M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("HTTP shared stance source differs", diff, err)
	}
	for _, participant := range []struct{ token, session string }{{rpPlayerToken, human}, {roundServiceAToken, service}} {
		response = performJSON(t, handler, "/api/v1/rp/observe", participant.token, core.RPSessionReadRequest{SessionID: participant.session})
		assertStatus(t, response, http.StatusOK)
		baseline = decodeData[storage.RPObservation](t, response).WorldTime
	}
	response = performJSON(t, handler, "/api/v1/rp/rounds/open", operatorToken, storage.RPSharedRoundOpenRequest{
		Binding: binding("open-info-http-relay"), HumanSessionID: human, ExternalSessionIDs: []string{service}})
	assertStatus(t, response, http.StatusOK)
	relayRound := decodeData[storage.RPSharedRound](t, response)
	relay := storage.RPSharedInformationRelayRequest{SessionID: human, RoundID: relayRound.RoundID,
		MessageID: "info-http-rumor-return", ForwardedMessageID: proposal.MessageID,
		RecipientEntityID: storage.M2RPNPCID, IdempotencyKey: "info-http-relay-proposal"}
	assertAPIError(t, performJSON(t, handler, "/api/v1/rp/rounds/information-relay", "", relay), http.StatusUnauthorized, core.CodeUnauthenticated)
	assertAPIError(t, performJSON(t, handler, "/api/v1/rp/rounds/information-relay", roundServiceAToken, relay), http.StatusNotFound, core.CodeNotFound)
	response = performJSON(t, handler, "/api/v1/rp/rounds/information-relay", rpPlayerToken, relay)
	assertStatus(t, response, http.StatusOK)
	if pending := decodeData[storage.RPSharedRound](t, response); pending.Status != "open" || pending.Submitted != 1 {
		t.Fatal("HTTP relay proposal committed before external answer", pending)
	}
	for _, secret := range []string{relay.ForwardedMessageID, relay.RecipientEntityID, proposal.Text} {
		if strings.Contains(response.Body.String(), secret) {
			t.Fatal("HTTP relay proposal leaked private claim or audience", secret, response.Body.String())
		}
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE event_type='RPInformationSent' AND json_extract(payload,'$.channel')='rumor'`).Scan(&count); err != nil || count != 0 {
		t.Fatal("HTTP rumor Event before external answer", count, err)
	}
	current, err = time.Parse(time.RFC3339Nano, baseline)
	if err != nil {
		t.Fatal(err)
	}
	response = performJSON(t, handler, "/api/v1/rp/rounds/wait", roundServiceAToken, storage.RPSharedWaitRequest{
		SessionID: service, RoundID: relayRound.RoundID,
		HorizonWorldTime: current.Add(10 * time.Minute).Format(time.RFC3339Nano), IdempotencyKey: "info-http-service-relay-wait"})
	assertStatus(t, response, http.StatusOK)
	response = performJSON(t, handler, "/api/v1/rp/rounds/advance", rpPlayerToken, storage.RPSharedRoundAdvanceRequest{
		RPSharedRoundReadRequest: storage.RPSharedRoundReadRequest{SessionID: human, RoundID: relayRound.RoundID}, Budget: 100})
	assertStatus(t, response, http.StatusOK)
	if settled := decodeData[storage.RPSharedRound](t, response); settled.Status != "settled" || settled.OwnDisposition != "action_accepted" {
		t.Fatal("HTTP shared relay did not settle", settled)
	}
	if strings.Contains(response.Body.String(), relay.ForwardedMessageID) || strings.Contains(response.Body.String(), relay.RecipientEntityID) {
		t.Fatal("HTTP settled relay receipt leaked private source/audience", response.Body.String())
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE event_type='RPInformationSent' AND json_extract(payload,'$.channel')='rumor' AND json_extract(payload,'$.message_id')=?`, relay.MessageID).Scan(&count); err != nil || count != 1 {
		t.Fatal("HTTP shared relay did not commit exactly one rumor Event", count, err)
	}
	if diff, err := store.CompareProjections(ctx, storage.M2DemoInstanceID, storage.M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("HTTP shared relay source differs", diff, err)
	}
}
