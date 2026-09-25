package storage

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPWorkTaskSleepFatigueChangesNextShiftOutcomeWithoutChangingWages(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "work-task.db")
	s := openCareerTestWorld(t, path)
	defer func() { s.Close() }()
	offer := prepareCareerEmploymentOffer(t, s)
	accepted, err := s.AcceptCareerOffer(ctx, core.CareerOfferAcceptRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "accept"),
		OfferID: offer.Fact.RecordID, AfterWorkPlaceID: M2AgentCafeID})
	if err != nil {
		t.Fatal(err)
	}
	contract := accepted.Fact.Employment.ContractID
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
			t.Fatal("work fixture world-time wait", result, err)
		}
	}
	moveHome := func(key string) {
		v := observe()
		if v.PlaceID == "place_m2_home_ada" {
			return
		}
		if _, err := s.MoveRP(ctx, core.RPMoveRequest{PrincipalID: ada.PrincipalID, SessionID: ada.SessionID,
			FromPlaceID: v.PlaceID, ToPlaceID: "place_m2_home_ada", ExpectedCursor: v.ObservationCursor,
			IdempotencyKey: key}); err != nil {
			t.Fatal(err)
		}
	}
	sleep := func(key, end string) {
		moveHome(key + "-home")
		if _, err := s.StartRPSleep(ctx, RPSleepRequest{Binding: binding(key + "-start"), SessionID: ada.SessionID}); err != nil {
			t.Fatal(err)
		}
		wait(key+"-wait", end)
		ended, err := s.EndRPSleep(ctx, RPSleepRequest{Binding: binding(key + "-end"), SessionID: ada.SessionID})
		if err != nil || ended.Fact.RestMinutes != 180 {
			t.Fatal("short sleep source", ended, err)
		}
	}
	attempt := func(key string) RPWorkTaskRequest {
		return RPWorkTaskRequest{Binding: binding(key), SessionID: ada.SessionID, ContractID: contract, TaskCode: "routine_check"}
	}
	if _, err := s.AttemptRPWorkTask(ctx, attempt("before-employment")); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatal("work task accepted before employment", err)
	}
	wait("first-bedtime", "2026-09-22T23:00:00Z")
	sleep("first-short", "2026-09-23T02:00:00Z")
	if _, err := s.AttemptRPWorkTask(ctx, attempt("before-first-shift")); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatal("work task accepted outside shift", err)
	}
	wait("first-shift", "2026-09-23T09:00:00Z")
	before, err := s.AttemptRPWorkTask(ctx, attempt("day-one-task"))
	if err != nil || before.Fact.Outcome != "completed" || before.Fact.FatigueLevel != "" || before.Fact.Day != 1 {
		t.Fatal("non-fatigued work sample", before, err)
	}
	wait("second-bedtime", "2026-09-23T23:00:00Z")
	sleep("second-short", "2026-09-24T02:00:00Z")
	wait("second-shift", "2026-09-24T09:00:00Z")
	request := attempt("day-two-task")
	stale := request
	stale.Binding.ExpectedHead--
	if _, err := s.AttemptRPWorkTask(ctx, stale); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatal("stale work task accepted", err)
	}
	foreign := request
	foreign.Binding.PrincipalID = "principal_foreign"
	if _, err := s.AttemptRPWorkTask(ctx, foreign); !core.HasCode(err, core.CodeNotFound) {
		t.Fatal("foreign controller recorded work", err)
	}
	s.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "work task rollback") }
	if _, err := s.AttemptRPWorkTask(ctx, request); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatal("work task did not roll back", err)
	}
	s.beforeCommit = nil
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE event_type='RPWorkTaskAttempted'`).Scan(&count); err != nil || count != 1 {
		t.Fatal("rolled-back work Event remains", count, err)
	}
	after, err := s.AttemptRPWorkTask(ctx, request)
	if err != nil || after.Fact.Outcome != "recheck_required" || after.Fact.FatigueLevel != "moderate" ||
		after.Fact.Day != 2 || after.Fact.WorkSourceEventID == "" {
		t.Fatal("fatigue did not change next-shift outcome", after, err)
	}
	if replay, err := s.AttemptRPWorkTask(ctx, request); err != nil || !replay.Replayed || replay.EventID != after.EventID {
		t.Fatal("exact work task retry", replay, err)
	}
	changed := request
	changed.TaskCode = "handoff_check"
	if _, err := s.AttemptRPWorkTask(ctx, changed); !core.HasCode(err, core.CodeInvalidArgument) {
		t.Fatal("unsupported work task accepted", err)
	}
	changed = request
	changed.Binding.ExpectedHead = observe().ObservationCursor
	if _, err := s.AttemptRPWorkTask(ctx, changed); !core.HasCode(err, core.CodeIdempotencyMismatch) {
		t.Fatal("mismatched work task retry accepted", err)
	}
	if _, err := s.AttemptRPWorkTask(ctx, attempt("duplicate-day-two")); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatal("duplicate shift work sample accepted", err)
	}
	manager, err := s.ReadRPWorkTask(ctx, M2AgentBoPrincipal, M2DemoInstanceID, M2DemoBranchID, after.EventID)
	if err != nil || manager.Outcome != "recheck_required" || manager.Day != 2 {
		t.Fatal("manager cannot see bounded task result", manager, err)
	}
	encoded, _ := json.Marshal(manager)
	if strings.Contains(string(encoded), "fatigue") || strings.Contains(string(encoded), "sleep") ||
		strings.Contains(string(encoded), after.Fact.WorkSourceEventID) {
		t.Fatal("private work/health cause escaped manager view")
	}
	if _, err := s.ReadRPWorkTask(ctx, M2RPNPCPrincipal, M2DemoInstanceID, M2DemoBranchID, after.EventID); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatal("unrelated principal read task outcome", err)
	}
	if _, err := s.ReadRPWorkTask(ctx, M2AgentBoPrincipal, M2DemoInstanceID, "other-branch", after.EventID); !core.HasCode(err, core.CodeNotFound) {
		t.Fatal("cross-branch task result read", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := s.ReadRPWorkTask(ctx, M2AgentBoPrincipal, M2DemoInstanceID, M2DemoBranchID, after.EventID); err != nil || got != manager {
		t.Fatal("work result changed after reopen", got, err)
	}
	if retry, err := s.AttemptRPWorkTask(ctx, request); err != nil || !retry.Replayed || retry.EventID != after.EventID {
		t.Fatal("work retry after reopen", retry, err)
	}
	if _, err := s.RunAgentLife(ctx, careerTime(3, 0, 1), 1000); err != nil {
		t.Fatal(err)
	}
	attendance, err := s.ReadCareerAttendance(ctx, M2AgentAdaPrincipal, M2DemoInstanceID, M2DemoBranchID, contract, 2)
	if err != nil || attendance.BaseEarnedMinor != 12 || attendance.Attendance.Status != "complete" || attendance.Attendance.RecordedSeconds != 14400 {
		t.Fatal("work sample altered earned wages or attendance", attendance, err)
	}
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal("work task source did not survive projection rebuild", err)
	}
	if got, err := s.ReadRPWorkTask(ctx, M2AgentBoPrincipal, M2DemoInstanceID, M2DemoBranchID, after.EventID); err != nil || got != manager {
		t.Fatal("work task changed after projection rebuild", got, err)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("work sample changed unrelated projections", diff, err)
	}
}
