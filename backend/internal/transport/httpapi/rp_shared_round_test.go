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

const (
	roundServiceAToken = "token-round-service-a"
	roundServiceBToken = "token-round-service-b"
	roundServiceAID    = "principal_round_service_a"
	roundServiceBID    = "principal_round_service_b"
)

func TestRPSharedRoundHTTPThreeAuthenticatedResidentsAndRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "round-http.db")
	store, err := storage.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { store.Close() }()
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
	for _, principal := range []string{roundServiceAID, roundServiceBID} {
		if _, err := db.ExecContext(ctx, `INSERT INTO principals(principal_id,principal_type,display_name,status) VALUES (?,'service',?,'active')`, principal, principal); err != nil {
			t.Fatal(err)
		}
	}
	head := func() int64 {
		var sequence int64
		if err := db.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, storage.M2DemoInstanceID, storage.M2DemoBranchID).Scan(&sequence); err != nil {
			t.Fatal(err)
		}
		return sequence
	}
	binding := func(key string) core.CareerBinding {
		return core.CareerBinding{PrincipalID: "principal_operator", InstanceID: storage.M2DemoInstanceID, BranchID: storage.M2DemoBranchID, ExpectedHead: head(), IdempotencyKey: key}
	}
	for i, entity := range []string{storage.M2AgentAdaID, storage.M2AgentBoID} {
		principal := []string{roundServiceAID, roundServiceBID}[i]
		controller := []string{"round-http-a", "round-http-b"}[i]
		if _, err := store.EnrollRPExternalControllerLocal(ctx, storage.RPExternalControllerEnrollmentRequest{Binding: binding("enroll-" + controller), EntityID: entity, ControllerPrincipalID: principal, ControllerInstanceID: controller}); err != nil {
			t.Fatal(err)
		}
		if _, err := store.AssignRPExternalControllerLocal(ctx, storage.RPExternalControllerAssignmentRequest{Binding: binding("assign-" + controller), EntityID: entity, ExpectedGeneration: 0}); err != nil {
			t.Fatal(err)
		}
	}
	handler := newSharedRoundHTTPHandler(t, store)
	open := func(token, entity, key string) string {
		response := performJSON(t, handler, "/api/v1/rp/sessions/open", token, core.RPSessionOpenRequest{InstanceID: storage.M2DemoInstanceID, BranchID: storage.M2DemoBranchID, EntityID: entity, POV: "second_person", IdempotencyKey: key})
		assertStatus(t, response, http.StatusOK)
		return decodeData[storage.RPSession](t, response).SessionID
	}
	humanSession := open(rpPlayerToken, storage.M2RPPlayerID, "round-http-human-session")
	serviceASession := open(roundServiceAToken, storage.M2AgentAdaID, "round-http-a-session")
	serviceBSession := open(roundServiceBToken, storage.M2AgentBoID, "round-http-b-session")
	participants := []struct{ token, session string }{{rpPlayerToken, humanSession}, {roundServiceAToken, serviceASession}, {roundServiceBToken, serviceBSession}}
	var baseline string
	for _, participant := range participants {
		response := performJSON(t, handler, "/api/v1/rp/observe", participant.token, core.RPSessionReadRequest{SessionID: participant.session})
		assertStatus(t, response, http.StatusOK)
		baseline = decodeData[storage.RPObservation](t, response).WorldTime
	}
	start, err := time.Parse(time.RFC3339, baseline)
	if err != nil {
		t.Fatal(err)
	}
	horizon := start.Add(10 * time.Minute).Format(time.RFC3339)
	roundOpen := storage.RPSharedRoundOpenRequest{Binding: binding("round-http-open"), HumanSessionID: humanSession, ExternalSessionIDs: []string{serviceASession, serviceBSession}}
	response := performJSON(t, handler, "/api/v1/rp/rounds/open", rpPlayerToken, roundOpen)
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)
	response = performJSON(t, handler, "/api/v1/rp/rounds/open", operatorToken, roundOpen)
	assertStatus(t, response, http.StatusOK)
	round := decodeData[storage.RPSharedRound](t, response)
	if round.Required != 3 || round.Status != "open" {
		t.Fatal("operator did not open bounded round", round)
	}
	aRead := storage.RPSharedRoundReadRequest{SessionID: serviceASession, RoundID: round.RoundID}
	response = performJSON(t, handler, "/api/v1/rp/rounds/read", roundServiceBToken, aRead)
	assertAPIError(t, response, http.StatusNotFound, core.CodeNotFound)
	spoof := aRead
	spoof.PrincipalID = roundServiceAID
	response = performJSON(t, handler, "/api/v1/rp/rounds/read", roundServiceBToken, spoof)
	assertAPIError(t, response, http.StatusForbidden, core.CodeUnauthorized)
	for _, participant := range participants[1:] {
		response = performJSON(t, handler, "/api/v1/rp/rounds/wait", participant.token, storage.RPSharedWaitRequest{SessionID: participant.session, RoundID: round.RoundID, HorizonWorldTime: horizon, IdempotencyKey: "round-http-wait-" + participant.session})
		assertStatus(t, response, http.StatusOK)
	}
	advance := storage.RPSharedRoundAdvanceRequest{RPSharedRoundReadRequest: aRead, Budget: 100}
	response = performJSON(t, handler, "/api/v1/rp/rounds/advance", roundServiceAToken, advance)
	assertAPIError(t, response, http.StatusConflict, core.CodeCommandInProgress)
	response = performJSON(t, handler, "/api/v1/rp/actions/wait", roundServiceAToken, core.RPWaitRequest{SessionID: serviceASession, ExpectedCursor: head(), TargetWorldTime: horizon, Budget: 100, IdempotencyKey: "forbidden-unilateral"})
	assertAPIError(t, response, http.StatusConflict, core.CodeCommandInProgress)
	response = performJSON(t, handler, "/api/v1/rp/rounds/wait", rpPlayerToken, storage.RPSharedWaitRequest{SessionID: humanSession, RoundID: round.RoundID, HorizonWorldTime: horizon, IdempotencyKey: "round-http-human-wait"})
	assertStatus(t, response, http.StatusOK)
	response = performJSON(t, handler, "/api/v1/rp/rounds/advance", roundServiceAToken, advance)
	assertStatus(t, response, http.StatusOK)
	settled := decodeData[storage.RPSharedRound](t, response)
	if settled.Status != "settled" || settled.CurrentWorldTime != horizon {
		t.Fatal("HTTP shared wait did not settle", settled)
	}
	for _, raw := range []string{storage.M2AgentAdaID, storage.M2AgentBoID, roundServiceAID, roundServiceBID, "event_rp_wait_", "advance_target", "wait_event_id"} {
		if strings.Contains(response.Body.String(), raw) {
			t.Fatalf("shared receipt exposed %q: %s", raw, response.Body.String())
		}
	}
	response = performJSON(t, handler, "/api/v1/rp/rounds/read", roundServiceBToken, storage.RPSharedRoundReadRequest{SessionID: serviceBSession, RoundID: round.RoundID})
	assertStatus(t, response, http.StatusOK)
	if read := decodeData[storage.RPSharedRound](t, response); read.CurrentWorldTime != horizon || read.EventSequence != settled.EventSequence {
		t.Fatal("other participant did not recover shared result", read)
	}
	var waitCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPWaitCompleted'`, storage.M2DemoInstanceID, storage.M2DemoBranchID).Scan(&waitCount); err != nil || waitCount != 1 {
		t.Fatal("shared HTTP advanced multiple times", waitCount, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = storage.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	handler = newSharedRoundHTTPHandler(t, store)
	response = performJSON(t, handler, "/api/v1/rp/rounds/advance", roundServiceAToken, advance)
	assertStatus(t, response, http.StatusOK)
	if replayed := decodeData[storage.RPSharedRound](t, response); !replayed.Replayed || replayed.EventSequence != settled.EventSequence {
		t.Fatal("HTTP reopen replay duplicated shared round", replayed)
	}
	if diffs, err := store.CompareProjections(ctx, storage.M2DemoInstanceID, storage.M2DemoBranchID); err != nil || len(diffs) != 0 {
		t.Fatal("HTTP shared round projection diverged", diffs, err)
	}
	for index, winner := range []string{serviceASession, serviceBSession} {
		for _, participant := range participants {
			response = performJSON(t, handler, "/api/v1/rp/observe", participant.token, core.RPSessionReadRequest{SessionID: participant.session})
			assertStatus(t, response, http.StatusOK)
		}
		before := head()
		actionOpen := storage.RPSharedRoundOpenRequest{Binding: binding("round-http-action-" + string(rune('a'+index))), HumanSessionID: humanSession, ExternalSessionIDs: []string{serviceASession, serviceBSession}}
		response = performJSON(t, handler, "/api/v1/rp/rounds/open", operatorToken, actionOpen)
		assertStatus(t, response, http.StatusOK)
		actionRound := decodeData[storage.RPSharedRound](t, response)
		for _, participant := range participants[1:] {
			proposal := storage.RPSharedSpeechRequest{SessionID: participant.session, RoundID: actionRound.RoundID, Text: "这是一句共享窗口发言。", IdempotencyKey: "action-" + actionRound.RoundID + "-" + participant.session}
			response = performJSON(t, handler, "/api/v1/rp/rounds/speech", participant.token, proposal)
			assertStatus(t, response, http.StatusOK)
		}
		if head() != before {
			t.Fatal("HTTP action proposal wrote Event before Human response")
		}
		response = performJSON(t, handler, "/api/v1/rp/actions/move", roundServiceAToken, core.RPMoveRequest{SessionID: serviceASession, ExpectedCursor: before, FromPlaceID: "place_m2_home_ada", ToPlaceID: storage.M2AgentCafeID, IdempotencyKey: "bypass-action-window-move"})
		assertAPIError(t, response, http.StatusConflict, core.CodeCommandInProgress)
		response = performJSON(t, handler, "/api/v1/rp/rounds/speech", roundServiceBToken, storage.RPSharedSpeechRequest{SessionID: serviceASession, RoundID: actionRound.RoundID, Text: "冒名", IdempotencyKey: "spoof-action"})
		assertAPIError(t, response, http.StatusNotFound, core.CodeNotFound)
		response = performJSON(t, handler, "/api/v1/rp/rounds/advance", roundServiceAToken, storage.RPSharedRoundAdvanceRequest{RPSharedRoundReadRequest: storage.RPSharedRoundReadRequest{SessionID: serviceASession, RoundID: actionRound.RoundID}, Budget: 100})
		assertAPIError(t, response, http.StatusConflict, core.CodeCommandInProgress)
		if index == 0 {
			response = performJSON(t, handler, "/api/v1/rp/rounds/wait", rpPlayerToken, storage.RPSharedWaitRequest{SessionID: humanSession, RoundID: actionRound.RoundID, HorizonWorldTime: start.Add(20 * time.Minute).Format(time.RFC3339), IdempotencyKey: "human-action-wait-" + actionRound.RoundID})
		} else {
			response = performJSON(t, handler, "/api/v1/rp/rounds/speech", rpPlayerToken, storage.RPSharedSpeechRequest{SessionID: humanSession, RoundID: actionRound.RoundID, Text: "Human 选择继续同一时刻的对话。", IdempotencyKey: "human-action-speech-" + actionRound.RoundID})
		}
		assertStatus(t, response, http.StatusOK)
		response = performJSON(t, handler, "/api/v1/rp/rounds/advance", roundServiceAToken, storage.RPSharedRoundAdvanceRequest{RPSharedRoundReadRequest: storage.RPSharedRoundReadRequest{SessionID: serviceASession, RoundID: actionRound.RoundID}, Budget: 100})
		assertStatus(t, response, http.StatusOK)
		if settledAction := decodeData[storage.RPSharedRound](t, response); settledAction.Status != "settled" || settledAction.EventSequence != before+1 {
			t.Fatal("HTTP action did not settle exactly once", settledAction)
		}
		for _, participant := range participants[1:] {
			response = performJSON(t, handler, "/api/v1/rp/rounds/read", participant.token, storage.RPSharedRoundReadRequest{SessionID: participant.session, RoundID: actionRound.RoundID})
			assertStatus(t, response, http.StatusOK)
			disposition := "deferred_no_effect"
			if participant.session == winner {
				disposition = "action_accepted"
			}
			if own := decodeData[storage.RPSharedRound](t, response); own.OwnDisposition != disposition {
				t.Fatal("HTTP round did not rotate fair selected Entity", own, winner)
			}
		}
	}
	for _, participant := range participants {
		response = performJSON(t, handler, "/api/v1/rp/observe", participant.token, core.RPSessionReadRequest{SessionID: participant.session})
		assertStatus(t, response, http.StatusOK)
	}
	waitTarget := start.Add(20 * time.Minute).Format(time.RFC3339)
	response = performJSON(t, handler, "/api/v1/rp/rounds/open", operatorToken, storage.RPSharedRoundOpenRequest{Binding: binding("round-http-anti-starvation"), HumanSessionID: humanSession, ExternalSessionIDs: []string{serviceASession, serviceBSession}})
	assertStatus(t, response, http.StatusOK)
	fairRound := decodeData[storage.RPSharedRound](t, response)
	for _, participant := range participants[1:] {
		response = performJSON(t, handler, "/api/v1/rp/rounds/speech", participant.token, storage.RPSharedSpeechRequest{SessionID: participant.session, RoundID: fairRound.RoundID, Text: "模型仍想继续发言。", IdempotencyKey: "starve-" + participant.session})
		assertStatus(t, response, http.StatusOK)
	}
	response = performJSON(t, handler, "/api/v1/rp/rounds/wait", rpPlayerToken, storage.RPSharedWaitRequest{SessionID: humanSession, RoundID: fairRound.RoundID, HorizonWorldTime: waitTarget, IdempotencyKey: "human-prioritizes-time"})
	assertStatus(t, response, http.StatusOK)
	response = performJSON(t, handler, "/api/v1/rp/rounds/advance", roundServiceAToken, storage.RPSharedRoundAdvanceRequest{RPSharedRoundReadRequest: storage.RPSharedRoundReadRequest{SessionID: serviceASession, RoundID: fairRound.RoundID}, Budget: 100})
	assertStatus(t, response, http.StatusOK)
	if result := decodeData[storage.RPSharedRound](t, response); result.Status != "settled" || result.CurrentWorldTime != waitTarget || result.OwnDisposition != "deferred_no_effect" {
		t.Fatal("repeated external speech starved Human's second wait", result)
	}
	response = performJSON(t, handler, "/api/v1/rp/rounds/read", rpPlayerToken, storage.RPSharedRoundReadRequest{SessionID: humanSession, RoundID: fairRound.RoundID})
	assertStatus(t, response, http.StatusOK)
	if humanResult := decodeData[storage.RPSharedRound](t, response); humanResult.OwnDisposition != "wait_completed" {
		t.Fatal("Human's selected wait lacked own receipt", humanResult)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPWaitCompleted'`, storage.M2DemoInstanceID, storage.M2DemoBranchID).Scan(&waitCount); err != nil || waitCount != 2 {
		t.Fatal("mixed action/wait did not advance time exactly once", waitCount, err)
	}
	for _, participant := range participants {
		response = performJSON(t, handler, "/api/v1/rp/observe", participant.token, core.RPSessionReadRequest{SessionID: participant.session})
		assertStatus(t, response, http.StatusOK)
	}
	// Move the world to a new decision time: the preceding Human-authorized
	// wait already took priority over repeated same-time model actions.
	moveAt := start.Add(30 * time.Minute).Format(time.RFC3339)
	response = performJSON(t, handler, "/api/v1/rp/rounds/open", operatorToken, storage.RPSharedRoundOpenRequest{Binding: binding("round-http-between-actions"), HumanSessionID: humanSession, ExternalSessionIDs: []string{serviceASession, serviceBSession}})
	assertStatus(t, response, http.StatusOK)
	between := decodeData[storage.RPSharedRound](t, response)
	for _, participant := range participants {
		response = performJSON(t, handler, "/api/v1/rp/rounds/wait", participant.token, storage.RPSharedWaitRequest{SessionID: participant.session, RoundID: between.RoundID, HorizonWorldTime: moveAt, IdempotencyKey: "round-http-between-" + participant.session})
		assertStatus(t, response, http.StatusOK)
	}
	response = performJSON(t, handler, "/api/v1/rp/rounds/advance", roundServiceAToken, storage.RPSharedRoundAdvanceRequest{RPSharedRoundReadRequest: storage.RPSharedRoundReadRequest{SessionID: serviceASession, RoundID: between.RoundID}, Budget: 100})
	assertStatus(t, response, http.StatusOK)
	for _, participant := range participants {
		response = performJSON(t, handler, "/api/v1/rp/observe", participant.token, core.RPSessionReadRequest{SessionID: participant.session})
		assertStatus(t, response, http.StatusOK)
	}
	beforeMove := head()
	response = performJSON(t, handler, "/api/v1/rp/rounds/open", operatorToken, storage.RPSharedRoundOpenRequest{Binding: binding("round-http-move-conflict"), HumanSessionID: humanSession, ExternalSessionIDs: []string{serviceASession, serviceBSession}})
	assertStatus(t, response, http.StatusOK)
	moveRound := decodeData[storage.RPSharedRound](t, response)
	moveProposal := storage.RPSharedMoveRequest{SessionID: serviceASession, RoundID: moveRound.RoundID, FromPlaceID: "place_m2_home_ada", ToPlaceID: storage.M2AgentCafeID, IdempotencyKey: "round-http-private-move"}
	response = performJSON(t, handler, "/api/v1/rp/rounds/move", roundServiceBToken, moveProposal)
	assertAPIError(t, response, http.StatusNotFound, core.CodeNotFound)
	response = performJSON(t, handler, "/api/v1/rp/rounds/move", roundServiceAToken, moveProposal)
	assertStatus(t, response, http.StatusOK)
	response = performJSON(t, handler, "/api/v1/rp/rounds/speech", roundServiceBToken, storage.RPSharedSpeechRequest{SessionID: serviceBSession, RoundID: moveRound.RoundID, Text: "我也想说。", IdempotencyKey: "round-http-conflicting-speech"})
	assertStatus(t, response, http.StatusOK)
	if head() != beforeMove {
		t.Fatal("HTTP move and speech proposals changed world before Human")
	}
	response = performJSON(t, handler, "/api/v1/rp/rounds/advance", roundServiceAToken, storage.RPSharedRoundAdvanceRequest{RPSharedRoundReadRequest: storage.RPSharedRoundReadRequest{SessionID: serviceASession, RoundID: moveRound.RoundID}, Budget: 100})
	assertAPIError(t, response, http.StatusConflict, core.CodeCommandInProgress)
	response = performJSON(t, handler, "/api/v1/rp/rounds/wait", rpPlayerToken, storage.RPSharedWaitRequest{SessionID: humanSession, RoundID: moveRound.RoundID, HorizonWorldTime: start.Add(40 * time.Minute).Format(time.RFC3339), IdempotencyKey: "round-http-human-move-window-wait"})
	assertStatus(t, response, http.StatusOK)
	response = performJSON(t, handler, "/api/v1/rp/rounds/advance", roundServiceAToken, storage.RPSharedRoundAdvanceRequest{RPSharedRoundReadRequest: storage.RPSharedRoundReadRequest{SessionID: serviceASession, RoundID: moveRound.RoundID}, Budget: 100})
	assertStatus(t, response, http.StatusOK)
	if moved := decodeData[storage.RPSharedRound](t, response); moved.Status != "settled" || moved.OwnDisposition != "action_accepted" || moved.EventSequence != beforeMove+1 || moved.CurrentWorldTime != moveAt {
		t.Fatal("HTTP mixed move/speech selection", moved)
	}
	response = performJSON(t, handler, "/api/v1/rp/rounds/read", roundServiceBToken, storage.RPSharedRoundReadRequest{SessionID: serviceBSession, RoundID: moveRound.RoundID})
	assertStatus(t, response, http.StatusOK)
	if other := decodeData[storage.RPSharedRound](t, response); other.OwnDisposition != "deferred_no_effect" {
		t.Fatal("HTTP losing speech was not deferred", other)
	}
	if diffs, err := store.CompareProjections(ctx, storage.M2DemoInstanceID, storage.M2DemoBranchID); err != nil || len(diffs) != 0 {
		t.Fatal("HTTP action rounds projection diverged", diffs, err)
	}
}

func newSharedRoundHTTPHandler(t *testing.T, store *storage.Store) http.Handler {
	t.Helper()
	service, err := storage.NewRPService(store, core.DeterministicRPDecisionProvider{}, "deterministic")
	if err != nil {
		t.Fatal(err)
	}
	authenticator, err := NewStaticTokenAuthenticator(map[string]string{
		operatorToken: "principal_operator", rpPlayerToken: storage.M2RPPlayerPrincipal,
		roundServiceAToken: roundServiceAID, roundServiceBToken: roundServiceBID,
	})
	if err != nil {
		t.Fatal(err)
	}
	cursors, err := NewCursorCodec([]byte("shared-round-http-test-cursor-secret-32"))
	if err != nil {
		t.Fatal(err)
	}
	server, err := New(service, authenticator, cursors)
	if err != nil {
		t.Fatal(err)
	}
	return server.Handler()
}
