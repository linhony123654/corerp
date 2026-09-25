package storage

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"corerp.local/backend/internal/core"
)

func TestRPSharedSleepStartEndRequiresRoundAndRecoversAcrossRestart(t *testing.T) {
	ctx := context.Background()
	f := newRPSharedRecoveryFixture(t)
	defer func() { f.store.Close() }()
	store := f.store
	var place string
	if err := store.db.QueryRowContext(ctx, `SELECT place_id FROM agent_positions WHERE agent_id=?`, M2AgentAdaID).Scan(&place); err != nil || place != "place_m2_home_ada" {
		t.Fatal("fixture Ada must be home", place, err)
	}
	childBinding := func(key string) core.CareerBinding {
		var head int64
		if err := store.db.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&head); err != nil {
			t.Fatal(err)
		}
		return core.CareerBinding{PrincipalID: f.external.PrincipalID, InstanceID: M2DemoInstanceID,
			BranchID: M2DemoBranchID, ExpectedHead: head, IdempotencyKey: key}
	}
	if _, err := store.StartRPSleep(ctx, RPSleepRequest{Binding: childBinding("direct-external-start"), SessionID: f.external.SessionID}); !core.HasCode(err, core.CodeCommandInProgress) {
		t.Fatal("external controller bypassed shared sleep ingress", err)
	}
	open := func(key string) RPSharedRound {
		for _, read := range []core.RPSessionReadRequest{f.human, f.external} {
			if _, err := store.ObserveRPSession(ctx, read); err != nil {
				t.Fatal(err)
			}
		}
		result, err := store.OpenRPSharedRoundLocal(ctx, RPSharedRoundOpenRequest{
			Binding: rpSharedRecoveryBinding(t, store, key), HumanSessionID: f.human.SessionID,
			ExternalSessionIDs: []string{f.external.SessionID}})
		if err != nil || result.Status != "open" || result.Required != 2 {
			t.Fatal("open sleep shared round", result, err)
		}
		return result
	}
	advance := func(round RPSharedRound) RPSharedRoundAdvanceRequest {
		return RPSharedRoundAdvanceRequest{RPSharedRoundReadRequest: RPSharedRoundReadRequest{
			PrincipalID: f.external.PrincipalID, SessionID: f.external.SessionID, RoundID: round.RoundID}, Budget: 1000}
	}
	start := open("shared-sleep-start")
	startProposal := RPSharedSleepRequest{PrincipalID: f.external.PrincipalID, SessionID: f.external.SessionID,
		RoundID: start.RoundID, Action: "start", IdempotencyKey: "sleep-start-proposal"}
	if _, err := store.SubmitRPSharedSleep(ctx, RPSharedSleepRequest{PrincipalID: f.human.PrincipalID,
		SessionID: f.external.SessionID, RoundID: start.RoundID, Action: "start", IdempotencyKey: "impersonate"}); err == nil {
		t.Fatal("Human submitted external sleep proposal")
	}
	if submitted, err := store.SubmitRPSharedSleep(ctx, startProposal); err != nil || submitted.Submitted != 1 {
		t.Fatal("private sleep start proposal", submitted, err)
	}
	if replayed, err := store.SubmitRPSharedSleep(ctx, startProposal); err != nil || !replayed.Replayed {
		t.Fatal("sleep proposal exact retry", replayed, err)
	}
	if _, err := store.SubmitRPSharedSleep(ctx, RPSharedSleepRequest{PrincipalID: f.external.PrincipalID,
		SessionID: f.external.SessionID, RoundID: start.RoundID, Action: "end", IdempotencyKey: "other-action"}); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatal("one participant submitted two actions", err)
	}
	if _, err := store.AdvanceRPSharedRound(ctx, advance(start)); !core.HasCode(err, core.CodeCommandInProgress) {
		t.Fatal("sleep Event accepted without Human submission", err)
	}
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM events WHERE event_type='RPSleepStarted'`, nil, 0)
	if _, err := store.StartRPSleep(ctx, RPSleepRequest{Binding: childBinding("direct-open-round"), SessionID: f.external.SessionID}); !core.HasCode(err, core.CodeCommandInProgress) {
		t.Fatal("direct sleep bypassed open shared round", err)
	}
	if _, err := store.SubmitRPSharedWait(ctx, RPSharedWaitRequest{PrincipalID: f.human.PrincipalID,
		SessionID: f.human.SessionID, RoundID: start.RoundID, HorizonWorldTime: "2026-09-22T03:00:00Z",
		IdempotencyKey: "human-sleep-start-wait"}); err != nil {
		t.Fatal(err)
	}
	lostSelection := errors.New("selected sleep response lost")
	store.afterRPSharedActionStage = func(stage string) error {
		if stage == "selection_committed" {
			return lostSelection
		}
		return nil
	}
	if _, err := store.AdvanceRPSharedRound(ctx, advance(start)); !errors.Is(err, lostSelection) {
		t.Fatal("sleep selection was not durable", err)
	}
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM events WHERE event_type='RPSleepStarted'`, nil, 0)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := Open(ctx, f.path)
	if err != nil {
		t.Fatal("reopen selected sleep", err)
	}
	f.store = store
	settledStart, err := store.AdvanceRPSharedRound(ctx, advance(start))
	if err != nil || settledStart.Status != "settled" || settledStart.OwnDisposition != "action_accepted" {
		t.Fatal("selected sleep start did not accept typed Event", settledStart, err)
	}
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM events WHERE event_type='RPSleepStarted'`, nil, 1)
	if replayed, err := store.AdvanceRPSharedRound(ctx, advance(start)); err != nil || !replayed.Replayed || replayed.EventSequence != settledStart.EventSequence {
		t.Fatal("sleep start settle retry", replayed, err)
	}
	if view, err := store.ReadRPSharedRound(ctx, RPSharedRoundReadRequest{PrincipalID: f.human.PrincipalID,
		SessionID: f.human.SessionID, RoundID: start.RoundID}); err != nil || view.OwnDisposition != "deferred_no_effect" {
		t.Fatal("Human receipt exposed another action", view, err)
	}

	waitRound := open("shared-sleep-elapse")
	for _, read := range []core.RPSessionReadRequest{f.human, f.external} {
		if _, err := store.SubmitRPSharedWait(ctx, RPSharedWaitRequest{PrincipalID: read.PrincipalID,
			SessionID: read.SessionID, RoundID: waitRound.RoundID, HorizonWorldTime: "2026-09-22T05:00:00Z",
			IdempotencyKey: "elapse-" + read.PrincipalID}); err != nil {
			t.Fatal(err)
		}
	}
	if elapsed, err := store.AdvanceRPSharedRound(ctx, advance(waitRound)); err != nil || elapsed.Status != "settled" || elapsed.CurrentWorldTime != "2026-09-22T05:00:00Z" {
		t.Fatal("shared world time did not elapse", elapsed, err)
	}
	if _, err := store.ObserveRPSession(ctx, f.external); err != nil {
		t.Fatal(err)
	}
	if _, err := store.EndRPSleep(ctx, RPSleepRequest{Binding: childBinding("direct-external-end"), SessionID: f.external.SessionID}); !core.HasCode(err, core.CodeCommandInProgress) {
		t.Fatal("external controller ended sleep outside shared round", err)
	}
	end := open("shared-sleep-end")
	endProposal := RPSharedSleepRequest{PrincipalID: f.external.PrincipalID, SessionID: f.external.SessionID,
		RoundID: end.RoundID, Action: "end", IdempotencyKey: "sleep-end-proposal"}
	if _, err := store.SubmitRPSharedSleep(ctx, endProposal); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SubmitRPSharedWait(ctx, RPSharedWaitRequest{PrincipalID: f.human.PrincipalID,
		SessionID: f.human.SessionID, RoundID: end.RoundID, HorizonWorldTime: "2026-09-22T06:00:00Z",
		IdempotencyKey: "human-sleep-end-wait"}); err != nil {
		t.Fatal(err)
	}
	lostEvent := errors.New("sleep event committed, receipt lost")
	store.afterRPSharedActionStage = func(stage string) error {
		if stage == "health_event_committed" {
			return lostEvent
		}
		return nil
	}
	if _, err := store.AdvanceRPSharedRound(ctx, advance(end)); !errors.Is(err, lostEvent) {
		t.Fatal("health event/receipt split was not exercised", err)
	}
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM events WHERE event_type='RPSleepEnded'`, nil, 1)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(ctx, f.path)
	if err != nil {
		t.Fatal("reopen committed sleep event", err)
	}
	f.store = store
	settledEnd, err := store.AdvanceRPSharedRound(ctx, advance(end))
	if err != nil || settledEnd.Status != "settled" || settledEnd.OwnDisposition != "action_accepted" {
		t.Fatal("sleep end did not settle after restart", settledEnd, err)
	}
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM events WHERE event_type='RPSleepEnded'`, nil, 1)
	var fact RPSleepEndFact
	var raw string
	if err := store.db.QueryRowContext(ctx, `SELECT payload FROM events WHERE event_type='RPSleepEnded'`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(raw), &fact); err != nil {
		t.Fatal(err)
	}
	started, err := time.Parse(time.RFC3339Nano, fact.StartedWorldTime)
	if err != nil || fact.RestMinutes != int64(time.Date(2026, 9, 22, 5, 0, 0, 0, time.UTC).Sub(started)/time.Minute) || fact.Status != "completed" {
		t.Fatal("shared sleep did not source actor rest", fact, err)
	}
	if diff, err := store.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("shared sleep source/replay divergence", diff, err)
	}
	feed, err := store.ReadRPEvents(ctx, RPEventsReadRequest{PrincipalID: f.human.PrincipalID,
		SessionID: f.human.SessionID, After: 0, Limit: 50})
	if err != nil {
		t.Fatal("Human event feed", err)
	}
	encoded, err := json.Marshal(feed)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), fact.StartEventID) || strings.Contains(string(encoded), settledEnd.RoundID) ||
		strings.Contains(string(encoded), "RPSleepStarted") || strings.Contains(string(encoded), "RPSleepEnded") {
		t.Fatal("private sleep truth escaped Human feed", string(encoded))
	}
}
