package storage

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"corerp.local/backend/internal/core"
)

func TestRPSleepTwoShortNightsChangeEveningDecisionAndLongRestRecovers(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "rp-sleep.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Close() }()
	if _, err := s.PrepareRPTravel(ctx); err != nil {
		t.Fatal(err)
	}
	ada := allowFixtureControl(t, ctx, s, M2AgentAdaID)
	observe := func() RPObservation {
		v, err := s.ObserveRPSession(ctx, ada)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	binding := func(key string) core.CareerBinding {
		return core.CareerBinding{PrincipalID: ada.PrincipalID, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID,
			ExpectedHead: observe().ObservationCursor, IdempotencyKey: key}
	}
	wait := func(key, target string) {
		v := observe()
		result, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: ada.PrincipalID, SessionID: ada.SessionID,
			TargetWorldTime: target, Budget: 1000, ExpectedCursor: v.ObservationCursor, IdempotencyKey: key})
		if err != nil || result.Status != "completed" || result.CurrentWorldTime != target {
			t.Fatal("world-time wait", result, err)
		}
	}
	moveHome := func(key string) {
		var from string
		if err := s.db.QueryRowContext(ctx, `SELECT place_id FROM agent_positions WHERE agent_id=?`, M2AgentAdaID).Scan(&from); err != nil {
			t.Fatal(err)
		}
		if from == "place_m2_home_ada" {
			return
		}
		v := observe()
		if _, err := s.MoveRP(ctx, core.RPMoveRequest{PrincipalID: ada.PrincipalID, SessionID: ada.SessionID,
			FromPlaceID: from, ToPlaceID: "place_m2_home_ada", ExpectedCursor: v.ObservationCursor, IdempotencyKey: key}); err != nil {
			t.Fatal("move home before sleep", err)
		}
	}
	sleep := func(key, target string) (RPSleepStartRecord, RPSleepEndRecord) {
		moveHome(key + "-home")
		request := RPSleepRequest{Binding: binding(key + "-start"), SessionID: ada.SessionID}
		start, err := s.StartRPSleep(ctx, request)
		if err != nil {
			t.Fatal("start sourced sleep", err)
		}
		if replay, err := s.StartRPSleep(ctx, request); err != nil || !replay.Replayed || replay.EventID != start.EventID {
			t.Fatal("sleep start exact replay", replay, err)
		}
		wait(key+"-wait", target)
		endRequest := RPSleepRequest{Binding: binding(key + "-end"), SessionID: ada.SessionID}
		end, err := s.EndRPSleep(ctx, endRequest)
		if err != nil || end.Fact.StartEventID != start.EventID || end.Fact.Status != "completed" {
			t.Fatal("end sourced sleep", end, err)
		}
		if replay, err := s.EndRPSleep(ctx, endRequest); err != nil || !replay.Replayed || replay.EventID != end.EventID {
			t.Fatal("sleep end exact replay", replay, err)
		}
		return start, end
	}
	wait("baseline-evening", "2026-09-22T19:00:00Z")
	before := readCareerTestContext(t, s, M2AgentAdaID)
	if before.Life.Health != nil {
		t.Fatal("home or elapsed time fabricated sleep state", before.Life.Health)
	}
	before.PlayerSpeechText = "今晚一起去走走吗？"
	beforeProposal, err := (core.DeterministicRPDecisionProvider{}).Propose(ctx, before)
	if err != nil || strings.Contains(beforeProposal.Text, "休息") {
		t.Fatal("baseline evening decision", beforeProposal, err)
	}
	wait("first-bedtime", "2026-09-22T23:00:00Z")
	_, first := sleep("short-one", "2026-09-23T02:00:00Z")
	if first.Fact.RestMinutes != 180 {
		t.Fatal("first short sleep duration", first.Fact)
	}
	wait("second-bedtime", "2026-09-23T23:00:00Z")
	secondStart, second := sleep("short-two", "2026-09-24T02:00:00Z")
	if second.Fact.RestMinutes != 180 {
		t.Fatal("second short sleep duration", second.Fact)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	wait("fatigued-evening", "2026-09-24T19:00:00Z")
	after := readCareerTestContext(t, s, M2AgentAdaID)
	if after.Life.Health == nil || after.Life.Health.FatigueLevel != "moderate" || after.Life.Health.FunctionalImpact != "avoid_optional_evening_activity" {
		t.Fatal("two short nights did not create own fatigue", after.Life.Health)
	}
	serialized, err := json.Marshal(after)
	if err != nil || strings.Contains(string(serialized), secondStart.EventID) || strings.Contains(string(serialized), second.EventID) || strings.Contains(string(serialized), "rest_minutes") || strings.Contains(string(serialized), "diagnosis") {
		t.Fatal("private sleep truth escaped model context", err)
	}
	after.PlayerSpeechText = before.PlayerSpeechText
	afterProposal, err := (core.DeterministicRPDecisionProvider{}).Propose(ctx, after)
	if err != nil || afterProposal.Action != "refuse" || !strings.Contains(afterProposal.Text, "休息") || afterProposal == beforeProposal {
		t.Fatal("fatigue did not change evening decision", beforeProposal, afterProposal, err)
	}
	wait("recovery-bedtime", "2026-09-24T22:00:00Z")
	_, long := sleep("long-recovery", "2026-09-25T06:00:00Z")
	if long.Fact.RestMinutes != 480 {
		t.Fatal("long restorative sleep duration", long.Fact)
	}
	wait("recovered-evening", "2026-09-25T19:00:00Z")
	recovered := readCareerTestContext(t, s, M2AgentAdaID)
	if recovered.Life.Health != nil {
		t.Fatal("long sleep did not recover fatigue", recovered.Life.Health)
	}
	recovered.PlayerSpeechText = before.PlayerSpeechText
	recoveredProposal, err := (core.DeterministicRPDecisionProvider{}).Propose(ctx, recovered)
	if err != nil || recoveredProposal.Action != "respond" || strings.Contains(recoveredProposal.Text, "休息") {
		t.Fatal("recovered decision", recoveredProposal, err)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("sleep changed unrelated projections", diff, err)
	}
}

func TestRPSleepFatigueRefusalCommitsAfterPlayerControlEnds(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "rp-sleep-committed-decision.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.PrepareRPTravel(ctx); err != nil {
		t.Fatal(err)
	}
	ada := allowFixtureControl(t, ctx, s, M2AgentAdaID)
	observeAda := func() RPObservation {
		view, err := s.ObserveRPSession(ctx, ada)
		if err != nil {
			t.Fatal(err)
		}
		return view
	}
	waitAda := func(key, at string) {
		view := observeAda()
		result, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: ada.PrincipalID, SessionID: ada.SessionID,
			TargetWorldTime: at, Budget: 1000, ExpectedCursor: view.ObservationCursor, IdempotencyKey: key})
		if err != nil || result.Status != "completed" || result.CurrentWorldTime != at {
			t.Fatal("advance to sourced sleep/scene time", result, err)
		}
	}
	sleepAda := func(key, endAt string) RPSleepEndRecord {
		view := observeAda()
		if view.PlaceID != "place_m2_home_ada" {
			if _, err := s.MoveRP(ctx, core.RPMoveRequest{PrincipalID: ada.PrincipalID, SessionID: ada.SessionID,
				FromPlaceID: view.PlaceID, ToPlaceID: "place_m2_home_ada", ExpectedCursor: view.ObservationCursor,
				IdempotencyKey: key + "-home"}); err != nil {
				t.Fatal("move Ada home before sleep", err)
			}
		}
		binding := func(suffix string) core.CareerBinding {
			return core.CareerBinding{PrincipalID: ada.PrincipalID, InstanceID: M2DemoInstanceID,
				BranchID: M2DemoBranchID, ExpectedHead: observeAda().ObservationCursor,
				IdempotencyKey: key + suffix}
		}
		if _, err := s.StartRPSleep(ctx, RPSleepRequest{Binding: binding("-start"), SessionID: ada.SessionID}); err != nil {
			t.Fatal(err)
		}
		waitAda(key+"-wait", endAt)
		ended, err := s.EndRPSleep(ctx, RPSleepRequest{Binding: binding("-end"), SessionID: ada.SessionID})
		if err != nil || ended.Fact.RestMinutes != 180 || ended.Fact.Status != "completed" {
			t.Fatal("short sleep source", ended, err)
		}
		return ended
	}
	waitAda("commit-first-bedtime", "2026-09-22T23:00:00Z")
	sleepAda("commit-short-one", "2026-09-23T02:00:00Z")
	waitAda("commit-second-bedtime", "2026-09-23T23:00:00Z")
	second := sleepAda("commit-short-two", "2026-09-24T02:00:00Z")
	waitAda("commit-fatigued-evening", "2026-09-24T19:00:00Z")

	player, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: M2RPPlayerPrincipal,
		InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2RPPlayerID,
		POV: "second_person", IdempotencyKey: "commit-invitation-player"})
	if err != nil {
		t.Fatal(err)
	}
	lin := core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: player.SessionID}
	view, err := s.ObserveRPSession(ctx, lin)
	if err != nil {
		t.Fatal(err)
	}
	if view.PlaceID != "place_m2_home_ada" {
		if _, err := s.MoveRP(ctx, core.RPMoveRequest{PrincipalID: lin.PrincipalID, SessionID: lin.SessionID,
			FromPlaceID: view.PlaceID, ToPlaceID: "place_m2_home_ada", ExpectedCursor: view.ObservationCursor,
			IdempotencyKey: "commit-visit-ada"}); err != nil {
			t.Fatal(err)
		}
		view, err = s.ObserveRPSession(ctx, lin)
		if err != nil {
			t.Fatal(err)
		}
	}
	speech, err := s.SpeakRP(ctx, core.RPSpeechRequest{PrincipalID: lin.PrincipalID, SessionID: lin.SessionID,
		Text: "今晚一起去走走吗？", ExpectedCursor: view.ObservationCursor,
		IdempotencyKey: "commit-evening-invitation"})
	if err != nil {
		t.Fatal(err)
	}
	request := core.RPDecisionRequest{PrincipalID: lin.PrincipalID, SessionID: lin.SessionID,
		TurnID: speech.TurnID, NPCEntityID: M2AgentAdaID}
	if _, err := s.DecideRP(ctx, request, core.DeterministicRPDecisionProvider{}); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatal("internal decision bypassed active player control", err)
	}
	if _, err := s.CloseRPSession(ctx, ada); err != nil {
		t.Fatal("release Ada player control", err)
	}
	input, err := s.BuildRPDecisionInput(ctx, request)
	if err != nil || input.Life == nil || input.Life.Health == nil || input.Life.Health.FatigueLevel != "moderate" {
		t.Fatal("committed turn lacks own fatigue", input.Life, err)
	}
	serialized, err := json.Marshal(input)
	if err != nil || strings.Contains(string(serialized), second.EventID) || strings.Contains(string(serialized), "rest_minutes") {
		t.Fatal("private sleep source escaped actual model input", err)
	}
	decision, err := s.DecideRP(ctx, request, core.DeterministicRPDecisionProvider{})
	if err != nil || decision.Proposal.Action != "refuse" || !strings.Contains(decision.Proposal.Text, "休息") {
		t.Fatal("fatigued evening decision was not rest-driven", decision, err)
	}
	committed, err := s.CommitRPDecision(ctx, request, decision)
	if err != nil || committed.Action != "refuse" || committed.EventSequence != speech.EventSequence+1 {
		t.Fatal("fatigued refusal did not become an Event", committed, err)
	}
	var heard string
	if err := s.db.QueryRowContext(ctx, `SELECT speech_text FROM rp_utterances WHERE event_id=? AND speaker_entity_id=?`,
		committed.EventID, M2AgentAdaID).Scan(&heard); err != nil || heard != decision.Proposal.Text {
		t.Fatal("committed NPC speech differs from decision", heard, err)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("committed fatigue refusal did not replay", diff, err)
	}
}

func TestRPSleepAuthorityRollbackAndInterruption(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "rp-sleep-boundary.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.PrepareRPTravel(ctx); err != nil {
		t.Fatal(err)
	}
	ada := allowFixtureControl(t, ctx, s, M2AgentAdaID)
	observe := func() RPObservation {
		v, err := s.ObserveRPSession(ctx, ada)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	request := func(key string) RPSleepRequest {
		return RPSleepRequest{Binding: core.CareerBinding{PrincipalID: ada.PrincipalID, InstanceID: M2DemoInstanceID,
			BranchID: M2DemoBranchID, ExpectedHead: observe().ObservationCursor, IdempotencyKey: key}, SessionID: ada.SessionID}
	}
	initial := observe()
	if initial.PlaceID != "place_m2_home_ada" {
		if _, err := s.MoveRP(ctx, core.RPMoveRequest{PrincipalID: ada.PrincipalID, SessionID: ada.SessionID,
			FromPlaceID: initial.PlaceID, ToPlaceID: "place_m2_home_ada", ExpectedCursor: initial.ObservationCursor,
			IdempotencyKey: "sleep-boundary-home"}); err != nil {
			t.Fatal(err)
		}
	}
	startRequest := request("sleep-boundary-start")
	foreign := startRequest
	foreign.Binding.PrincipalID = "principal_foreign"
	if _, err := s.StartRPSleep(ctx, foreign); !core.HasCode(err, core.CodeNotFound) {
		t.Fatal("foreign principal started actor sleep", err)
	}
	stale := startRequest
	stale.Binding.ExpectedHead--
	if _, err := s.StartRPSleep(ctx, stale); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatal("stale head started actor sleep", err)
	}
	s.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "sleep start rollback") }
	if _, err := s.StartRPSleep(ctx, startRequest); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatal("sleep start did not roll back", err)
	}
	s.beforeCommit = nil
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE event_type='RPSleepStarted'`).Scan(&count); err != nil || count != 0 {
		t.Fatal("rolled-back sleep source remains", count, err)
	}
	started, err := s.StartRPSleep(ctx, startRequest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.StartRPSleep(ctx, request("sleep-boundary-duplicate")); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatal("second open sleep accepted", err)
	}
	changed := startRequest
	changed.Binding.ExpectedHead = observe().ObservationCursor
	if _, err := s.StartRPSleep(ctx, changed); !core.HasCode(err, core.CodeIdempotencyMismatch) {
		t.Fatal("mismatched sleep replay accepted", err)
	}
	if _, err := s.EndRPSleep(ctx, request("sleep-boundary-zero")); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatal("zero-time sleep accepted", err)
	}
	clock := observe().WorldTime
	startTime, err := time.Parse(time.RFC3339Nano, clock)
	if err != nil {
		t.Fatal(err)
	}
	waitUntil := func(key string, target time.Time) {
		v := observe()
		if _, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: ada.PrincipalID, SessionID: ada.SessionID,
			TargetWorldTime: target.Format(time.RFC3339), Budget: 1000, ExpectedCursor: v.ObservationCursor,
			IdempotencyKey: key}); err != nil {
			t.Fatal(err)
		}
	}
	waitUntil("sleep-boundary-first-wait", startTime.Add(30*time.Minute))
	v := observe()
	if _, err := s.MoveRP(ctx, core.RPMoveRequest{PrincipalID: ada.PrincipalID, SessionID: ada.SessionID,
		FromPlaceID: v.PlaceID, ToPlaceID: M2AgentCafeID, ExpectedCursor: v.ObservationCursor,
		IdempotencyKey: "sleep-boundary-interrupt"}); err != nil {
		t.Fatal(err)
	}
	waitUntil("sleep-boundary-second-wait", startTime.Add(time.Hour))
	ended, err := s.EndRPSleep(ctx, request("sleep-boundary-end"))
	if err != nil || ended.Fact.StartEventID != started.EventID || ended.Fact.Status != "interrupted" ||
		ended.Fact.RestMinutes != 30 || ended.Fact.InterruptedByEventID == "" {
		t.Fatal("activity did not truncate sleep", ended, err)
	}
	if _, err := s.StartRPSleep(ctx, request("sleep-boundary-wrong-place")); !core.HasCode(err, core.CodeNotFound) {
		t.Fatal("sleep started outside home", err)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE event_type='RPSleepStarted'`).Scan(&count); err != nil || count != 1 {
		t.Fatal("unexpected sleep start sources", count, err)
	}
}
