package storage

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestCareerAttendanceTracksActualRPDepartureAndRecovery(t *testing.T) {
	for _, scenario := range []struct {
		name      string
		leaveHour int
		status    string
		seconds   int64
	}{{"partial", 9, "partial", 3600}, {"no_recorded_work", 8, "no_recorded_work", 0}} {
		t.Run(scenario.name, func(t *testing.T) {
			ctx := context.Background()
			path := filepath.Join(t.TempDir(), "attendance.db")
			s := openCareerTestWorld(t, path)
			defer func() { s.Close() }()
			offer := prepareCareerEmploymentOffer(t, s)
			accepted, err := s.AcceptCareerOffer(ctx, core.CareerOfferAcceptRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "accept"), OfferID: offer.Fact.RecordID, AfterWorkPlaceID: M2AgentCafeID})
			if err != nil {
				t.Fatal(err)
			}
			job := *accepted.Fact.Employment
			if _, err := s.ReadCareerAttendance(ctx, M2AgentAdaPrincipal, M2DemoInstanceID, M2DemoBranchID, job.ContractID, 1); !core.HasCode(err, core.CodeNotFound) {
				t.Fatalf("planned but unexecuted work became attendance: %v", err)
			}
			session, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: M2RPPlayerPrincipal, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2RPPlayerID, POV: "second_person", IdempotencyKey: "attendance-session"})
			if err != nil {
				t.Fatal(err)
			}
			read := core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID}
			view, err := s.ObserveRPSession(ctx, read)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.MoveRP(ctx, core.RPMoveRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, FromPlaceID: view.PlaceID, ToPlaceID: job.WorkplaceID, ExpectedCursor: view.ObservationCursor, IdempotencyKey: "visit-work"}); err != nil {
				t.Fatal(err)
			}
			view, err = s.ObserveRPSession(ctx, read)
			if err != nil {
				t.Fatal(err)
			}
			wait, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: careerTime(1, scenario.leaveHour, 0), Budget: 100, IdempotencyKey: "wait-at-work"})
			if err != nil {
				t.Fatal(err)
			}
			decision, err := s.RunRPInitiative(ctx, core.RPInitiativeRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, NPCEntityID: job.EmployeeID, TriggerEventID: wait.EventID}, rpDecisionProviderFunc(func(context.Context, core.RPDecisionInput) (core.RPDecisionProposal, error) {
				return core.RPDecisionProposal{Action: "leave", DestinationPlaceID: M2AgentCafeID}, nil
			}))
			if err != nil || decision.Action != "leave" {
				t.Fatalf("actual departure: %+v %v", decision, err)
			}
			if _, err := s.RunAgentLife(ctx, careerTime(2, 0, 1), 100); err != nil {
				t.Fatal(err)
			}
			attendance, err := s.ReadCareerAttendance(ctx, M2AgentAdaPrincipal, M2DemoInstanceID, M2DemoBranchID, job.ContractID, 1)
			if err != nil {
				t.Fatal(err)
			}
			if attendance.Attendance.Status != scenario.status || attendance.Attendance.RecordedSeconds != scenario.seconds || attendance.Attendance.ExpectedSeconds != 14400 || attendance.Attendance.TermsEventID != accepted.EventID {
				t.Fatalf("schedule intent counted as actual work: %+v", attendance)
			}
			assertM2Value(t, ctx, s, `SELECT amount_paid_minor FROM wage_obligations WHERE contract_id=? AND period_start_day=1`, []any{job.ContractID}, 12)
			assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM outbox WHERE event_id=?`, []any{attendance.EventID}, 0)
			for _, principal := range []string{M2AgentBoPrincipal, M2AgentAdaPrincipal} {
				got, err := s.ReadCareerAttendance(ctx, principal, M2DemoInstanceID, M2DemoBranchID, job.ContractID, 1)
				if err != nil || !reflect.DeepEqual(got, attendance) {
					t.Fatalf("authorized work report: %+v %v", got, err)
				}
			}
			if _, err := s.ReadCareerAttendance(ctx, M2RPPlayerPrincipal, M2DemoInstanceID, M2DemoBranchID, job.ContractID, 1); !core.HasCode(err, core.CodeUnauthorized) {
				t.Fatalf("unrelated player read attendance: %v", err)
			}
			if _, err := s.ReadCareerAttendance(ctx, M2AgentAdaPrincipal, M2DemoInstanceID, "other-branch", job.ContractID, 1); !core.HasCode(err, core.CodeNotFound) {
				t.Fatalf("cross-branch attendance: %v", err)
			}
			encoded, _ := json.Marshal(attendance)
			if strings.Contains(string(encoded), M2AgentCafeID) || strings.Contains(string(encoded), "rp_npc_leave") {
				t.Fatal("report exposed out-of-work destination/activity")
			}
			own := readCareerTestContext(t, s, job.EmployeeID)
			found := false
			for _, memory := range own.Life.SalientMemories {
				if memory.Kind == "own_work_attendance" && memory.SourceEventID == attendance.EventID {
					found = true
				}
			}
			if !found {
				t.Fatal("actual work report missing from own memory")
			}
			other := readCareerTestContext(t, s, M2AgentBoID)
			for _, memory := range other.Life.SalientMemories {
				if memory.SourceEventID == attendance.EventID {
					t.Fatal("work report automatically became another NPC's own memory")
				}
			}
			if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
				t.Fatal(err)
			}
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			s, err = Open(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			if got, err := s.ReadCareerAttendance(ctx, M2AgentAdaPrincipal, M2DemoInstanceID, M2DemoBranchID, job.ContractID, 1); err != nil || !reflect.DeepEqual(got, attendance) {
				t.Fatalf("recovered report changed: %+v %v", got, err)
			}
			if differences, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
				t.Fatalf("attendance replay: %+v %v", differences, err)
			}
		})
	}
}

func TestCareerAttendanceUsesSamePlaceActivityAndAtomicAccrual(t *testing.T) {
	ctx := context.Background()
	s := openCareerTestWorld(t, filepath.Join(t.TempDir(), "same-place-attendance.db"))
	defer s.Close()
	offer := prepareCareerEmploymentOffer(t, s)
	accepted, err := s.AcceptCareerOffer(ctx, core.CareerOfferAcceptRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "accept"), OfferID: offer.Fact.RecordID, AfterWorkPlaceID: M2AgentCafeID})
	if err != nil {
		t.Fatal(err)
	}
	job := *accepted.Fact.Employment
	// Fixture additional appointment leads to a real early arrival. At08:00
	// the existing scheduler must record a same-place work activity, not a move.
	early := job
	early.WorkStartHour = 7
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		t.Fatal(err)
	}
	if err := queueCareerWorkday(ctx, tx.conn, early, 1, accepted.EventID); err != nil {
		tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RunAgentLife(ctx, careerTime(1, 12, 0), 100); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='AgentActivityStarted' AND actor_id=? AND world_time=?`, []any{job.EmployeeID, careerTime(1, 8, 0)}, 1)
	s.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "attendance accrual interruption") }
	if _, err := s.RunAgentLife(ctx, careerTime(2, 0, 0), 100); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatalf("expected interruption: %v", err)
	}
	s.beforeCommit = nil
	if _, err := s.ReadCareerAttendance(ctx, M2AgentAdaPrincipal, M2DemoInstanceID, M2DemoBranchID, job.ContractID, 1); !core.HasCode(err, core.CodeNotFound) {
		t.Fatalf("rolled-back attendance visible: %v", err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM wage_obligations WHERE contract_id=?`, []any{job.ContractID}, 0)
	if _, err := s.RunAgentLife(ctx, careerTime(2, 0, 1), 100); err != nil {
		t.Fatal(err)
	}
	got, err := s.ReadCareerAttendance(ctx, M2AgentAdaPrincipal, M2DemoInstanceID, M2DemoBranchID, job.ContractID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got.Attendance.Status != "complete" || got.Attendance.RecordedSeconds != 14400 || len(got.Attendance.WorkEventIDs) != 1 {
		t.Fatalf("same-place work not accounted: %+v", got)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_id=? AND event_type='AgentActivityStarted'`, []any{got.Attendance.WorkEventIDs[0]}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE json_extract(payload,'$.attendance.contract_id')=?`, []any{job.ContractID}, 1)
	// Early arrival is not silently converted to authorized overtime or more pay.
	assertM2Value(t, ctx, s, `SELECT amount_paid_minor FROM wage_obligations WHERE contract_id=?`, []any{job.ContractID}, 12)
}
