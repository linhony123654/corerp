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

func TestRPSharedSleepHTTPAuthenticatedRoundAndPrivateReceipt(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "shared-sleep-http.db")
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
	if _, err := db.ExecContext(ctx, `INSERT INTO principals(principal_id,principal_type,display_name,status) VALUES (?,'service','Sleep service','active')`, roundServiceAID); err != nil {
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
		Binding: binding("enroll-sleep-http"), EntityID: storage.M2AgentAdaID,
		ControllerPrincipalID: roundServiceAID, ControllerInstanceID: "controller-sleep-http"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AssignRPExternalControllerLocal(ctx, storage.RPExternalControllerAssignmentRequest{
		Binding: binding("assign-sleep-http"), EntityID: storage.M2AgentAdaID, ExpectedGeneration: 0}); err != nil {
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
	human := open(rpPlayerToken, storage.M2RPPlayerID, "sleep-http-human")
	service := open(roundServiceAToken, storage.M2AgentAdaID, "sleep-http-service")
	var baseline string
	for _, participant := range []struct{ token, session string }{{rpPlayerToken, human}, {roundServiceAToken, service}} {
		response := performJSON(t, handler, "/api/v1/rp/observe", participant.token, core.RPSessionReadRequest{SessionID: participant.session})
		assertStatus(t, response, http.StatusOK)
		view := decodeData[storage.RPObservation](t, response)
		baseline = view.WorldTime
		if participant.session == service && view.PlaceID != "place_m2_home_ada" {
			t.Fatal("sleep service must control Ada at home", view.PlaceID)
		}
	}
	response := performJSON(t, handler, "/api/v1/rp/rounds/open", operatorToken, storage.RPSharedRoundOpenRequest{
		Binding: binding("open-sleep-http"), HumanSessionID: human, ExternalSessionIDs: []string{service}})
	assertStatus(t, response, http.StatusOK)
	round := decodeData[storage.RPSharedRound](t, response)
	proposal := storage.RPSharedSleepRequest{SessionID: service, RoundID: round.RoundID, Action: "start", IdempotencyKey: "sleep-http-proposal"}
	workRoute := storage.RPSharedWorkTaskRequest{SessionID: service, RoundID: "missing-round", ContractID: "missing-contract", TaskCode: "routine_check", IdempotencyKey: "work-http-missing"}
	assertAPIError(t, performJSON(t, handler, "/api/v1/rp/rounds/work-task", roundServiceAToken, workRoute), http.StatusNotFound, core.CodeNotFound)
	response = performJSON(t, handler, "/api/v1/rp/rounds/sleep", rpPlayerToken, proposal)
	assertAPIError(t, response, http.StatusNotFound, core.CodeNotFound)
	response = performJSON(t, handler, "/api/v1/rp/rounds/sleep", roundServiceAToken, proposal)
	assertStatus(t, response, http.StatusOK)
	if submitted := decodeData[storage.RPSharedRound](t, response); submitted.Submitted != 1 || submitted.Status != "open" {
		t.Fatal("sleep proposal wrote premature world effect", submitted)
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE event_type='RPSleepStarted'`).Scan(&count); err != nil || count != 0 {
		t.Fatal("sleep Event before Human answer", count, err)
	}
	at, err := time.Parse(time.RFC3339Nano, baseline)
	if err != nil {
		t.Fatal(err)
	}
	response = performJSON(t, handler, "/api/v1/rp/rounds/wait", rpPlayerToken, storage.RPSharedWaitRequest{
		SessionID: human, RoundID: round.RoundID, HorizonWorldTime: at.Add(time.Hour).Format(time.RFC3339),
		IdempotencyKey: "human-sleep-http-wait"})
	assertStatus(t, response, http.StatusOK)
	advance := storage.RPSharedRoundAdvanceRequest{RPSharedRoundReadRequest: storage.RPSharedRoundReadRequest{
		SessionID: service, RoundID: round.RoundID}, Budget: 100}
	response = performJSON(t, handler, "/api/v1/rp/rounds/advance", roundServiceAToken, advance)
	assertStatus(t, response, http.StatusOK)
	settled := decodeData[storage.RPSharedRound](t, response)
	if settled.Status != "settled" || settled.OwnDisposition != "action_accepted" || settled.CurrentWorldTime != baseline {
		t.Fatal("HTTP sleep start did not settle", settled)
	}
	if strings.Contains(response.Body.String(), "event_rp_sleep_") || strings.Contains(response.Body.String(), storage.M2AgentAdaID) || strings.Contains(response.Body.String(), "rest_minutes") {
		t.Fatal("HTTP round receipt leaked sleep truth", response.Body.String())
	}
	response = performJSON(t, handler, "/api/v1/rp/rounds/sleep", roundServiceAToken, proposal)
	assertStatus(t, response, http.StatusOK)
	if retry := decodeData[storage.RPSharedRound](t, response); !retry.Replayed || retry.EventSequence != settled.EventSequence {
		t.Fatal("HTTP proposal retry lost settled receipt", retry)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE event_type='RPSleepStarted'`).Scan(&count); err != nil || count != 1 {
		t.Fatal("HTTP shared sleep duplicated Event", count, err)
	}
	if diff, err := store.CompareProjections(ctx, storage.M2DemoInstanceID, storage.M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("HTTP shared sleep projection divergence", diff, err)
	}
}
