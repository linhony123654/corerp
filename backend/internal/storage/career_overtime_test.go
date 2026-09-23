package storage

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"corerp.local/backend/internal/core"
)

func careerTestOvertime(t *testing.T, s *Store, contract, id string) core.CareerOvertimeRequest {
	t.Helper()
	return core.CareerOvertimeRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, id), OvertimeID: id, ContractID: contract, Day: 1, StartHour: 13, EndHour: 15, RateMinorPerHour: 7, Reason: "Additional afternoon work, subject to your agreement."}
}

func TestCareerOvertimeActualWorkPayAndRecovery(t *testing.T) {
	for _, scenario := range []struct {
		name                   string
		leaveHour, leaveMinute int
		rate, earned, paid     int64
	}{
		{"full", 0, 0, 7, 14, 26}, {"partial_round_down", 14, 30, 7, 10, 22}, {"no_work", 13, 0, 7, 0, 12}, {"arrears", 0, 0, 1000, 2000, 1020},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			ctx := context.Background()
			path := filepath.Join(t.TempDir(), "overtime.db")
			s := openCareerTestWorld(t, path)
			defer func() { s.Close() }()
			offer := prepareCareerEmploymentOffer(t, s)
			jobRecord, err := s.AcceptCareerOffer(ctx, core.CareerOfferAcceptRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "accept-job"), OfferID: offer.Fact.RecordID, AfterWorkPlaceID: M2AgentCafeID})
			if err != nil {
				t.Fatal(err)
			}
			job := jobRecord.Fact.Employment
			r := careerTestOvertime(t, s, job.ContractID, "overtime")
			r.RateMinorPerHour = scenario.rate
			offered, err := s.OfferCareerOvertime(ctx, r)
			if err != nil {
				t.Fatal(err)
			}
			assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_schedule_entries WHERE definition_event_id=?`, []any{offered.EventID}, 0)
			accept := core.CareerOvertimeResponseRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "accept-overtime"), OvertimeID: r.OvertimeID, Decision: "accept", Reason: "I agree to the stated window and pay policy."}
			wrong := accept
			wrong.Binding.PrincipalID = M2AgentBoPrincipal
			if _, err := s.RespondCareerOvertime(ctx, wrong); !core.HasCode(err, core.CodeUnauthorized) {
				t.Fatalf("manager fabricated employee consent: %v", err)
			}
			accepted, err := s.RespondCareerOvertime(ctx, accept)
			if err != nil {
				t.Fatal(err)
			}
			assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_schedule_entries WHERE definition_event_id=? AND status='active'`, []any{accepted.EventID}, 2)
			if scenario.leaveHour != 0 {
				session, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: M2RPPlayerPrincipal, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2RPPlayerID, POV: "second_person", IdempotencyKey: "overtime-session"})
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
				wait, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: careerTime(1, scenario.leaveHour, scenario.leaveMinute), Budget: 100, IdempotencyKey: "wait-overtime"})
				if err != nil {
					t.Fatal(err)
				}
				departure, err := s.RunRPInitiative(ctx, core.RPInitiativeRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, NPCEntityID: job.EmployeeID, TriggerEventID: wait.EventID}, rpDecisionProviderFunc(func(context.Context, core.RPDecisionInput) (core.RPDecisionProposal, error) {
					return core.RPDecisionProposal{Action: "leave", DestinationPlaceID: M2AgentCafeID}, nil
				}))
				if err != nil || departure.Action != "leave" {
					t.Fatalf("real overtime departure: %+v %v", departure, err)
				}
			}
			if _, err := s.RunAgentLife(ctx, careerTime(2, 0, 1), 100); err != nil {
				t.Fatal(err)
			}
			assertM2Value(t, ctx, s, `SELECT amount_due_minor FROM wage_obligations WHERE contract_id=? AND period_start_day=1`, []any{job.ContractID}, 12+scenario.earned)
			assertM2Value(t, ctx, s, `SELECT amount_paid_minor FROM wage_obligations WHERE contract_id=? AND period_start_day=1`, []any{job.ContractID}, scenario.paid)
			assertM2Value(t, ctx, s, `SELECT json_extract(payload,'$.overtime_amount_minor') FROM events WHERE event_type='WageObligationAccrued' AND json_extract(payload,'$.contract_id')=?`, []any{job.ContractID}, scenario.earned)
			assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='WageObligationAccrued' AND json_extract(payload,'$.overtime[0].agreement_event_id')=?`, []any{accepted.EventID}, 1)
			assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM outbox WHERE event_id IN (?,?)`, []any{offered.EventID, accepted.EventID}, 0)
			life := readCareerTestContext(t, s, job.EmployeeID)
			if life.OwnAssetMinor != 500+scenario.paid || life.Life.ReceivableMinor != 100+12+scenario.earned-scenario.paid {
				t.Fatalf("earned supplement became fictitious cash or disappeared: %+v", life)
			}
			found := false
			for _, memory := range life.Life.SalientMemories {
				if memory.Kind == "own_overtime_decision" && memory.SourceEventID == accepted.EventID {
					found = true
				}
			}
			if !found {
				t.Fatal("agreed overtime missing from own memory")
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
			if retry, err := s.RespondCareerOvertime(ctx, accept); err != nil || !retry.Replayed || retry.EventID != accepted.EventID {
				t.Fatalf("overtime consent recovery: %+v %v", retry, err)
			}
			if got := readCareerTestContext(t, s, job.EmployeeID); !reflect.DeepEqual(got, life) {
				t.Fatal("recovery changed overtime evidence or money")
			}
			late := core.CareerOvertimeResponseRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "late-cancel"), OvertimeID: r.OvertimeID, Decision: "cancel", Reason: "Too late"}
			if _, err := s.RespondCareerOvertime(ctx, late); !core.HasCode(err, core.CodeBranchConflict) {
				t.Fatalf("cancellation erased earned overtime: %v", err)
			}
			if differences, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
				t.Fatalf("overtime replay: %+v %v", differences, err)
			}
		})
	}
}

func TestCareerOvertimeConsentConflictsCancellationAndLeave(t *testing.T) {
	ctx := context.Background()
	s := openCareerTestWorld(t, filepath.Join(t.TempDir(), "overtime-guards.db"))
	defer s.Close()
	offer := prepareCareerEmploymentOffer(t, s)
	jobRecord, err := s.AcceptCareerOffer(ctx, core.CareerOfferAcceptRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "accept-job"), OfferID: offer.Fact.RecordID, AfterWorkPlaceID: M2AgentCafeID})
	if err != nil {
		t.Fatal(err)
	}
	job := jobRecord.Fact.Employment
	respond := func(id, principal, decision, key string) core.CareerOvertimeResponseRequest {
		return core.CareerOvertimeResponseRequest{Binding: careerTestBinding(t, s, principal, key), OvertimeID: id, Decision: decision, Reason: "Explicit response to the agreement"}
	}
	conflict := careerTestOvertime(t, s, job.ContractID, "conflict")
	conflict.StartHour, conflict.EndHour = 12, 14
	if _, err := s.OfferCareerOvertime(ctx, conflict); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RespondCareerOvertime(ctx, respond(conflict.OvertimeID, M2AgentAdaPrincipal, "accept", "accept-conflict")); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatalf("overtime erased personal noon commitment: %v", err)
	}
	outer := careerTestOvertime(t, s, job.ContractID, "outer")
	outer.EndHour = 17
	if _, err := s.OfferCareerOvertime(ctx, outer); err != nil {
		t.Fatal(err)
	}
	accept := respond(outer.OvertimeID, M2AgentAdaPrincipal, "accept", "accept-outer")
	s.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "interrupt overtime agreement") }
	if _, err := s.RespondCareerOvertime(ctx, accept); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatalf("overtime interruption: %v", err)
	}
	s.beforeCommit = nil
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_schedule_entries WHERE agent_id=? AND world_time=?`, []any{job.EmployeeID, careerTime(1, 13, 0)}, 0)
	accepted, err := s.RespondCareerOvertime(ctx, accept)
	if err != nil {
		t.Fatal(err)
	}
	inner := careerTestOvertime(t, s, job.ContractID, "inner")
	inner.StartHour, inner.EndHour = 14, 16
	if _, err := s.OfferCareerOvertime(ctx, inner); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RespondCareerOvertime(ctx, respond(inner.OvertimeID, M2AgentAdaPrincipal, "accept", "accept-inner")); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatalf("contained overtime window double-counted: %v", err)
	}
	if declined, err := s.RespondCareerOvertime(ctx, respond(inner.OvertimeID, M2AgentAdaPrincipal, "decline", "decline-inner")); err != nil || declined.Fact.Overtime.Status != "declined" {
		t.Fatalf("decline: %+v %v", declined, err)
	}
	if _, err := s.ReadCareerRecruitmentRecord(ctx, M2RPPlayerPrincipal, M2DemoInstanceID, M2DemoBranchID, "overtime", outer.OvertimeID); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("private overtime leaked: %v", err)
	}
	leave, err := s.RequestCareerLeave(ctx, core.CareerLeaveRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "leave"), LeaveID: "leave", ContractID: job.ContractID, StartDay: 1, EndDay: 2, Reason: "Personal plans"})
	if err != nil {
		t.Fatal(err)
	}
	approve := core.CareerLeaveReviewRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "approve"), LeaveID: leave.Fact.RecordID, Decision: "approve", Notice: "Approved after cancelling overtime"}
	if _, err := s.ReviewCareerLeave(ctx, approve); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatalf("leave silently cancelled accepted overtime: %v", err)
	}
	cancelled, err := s.RespondCareerOvertime(ctx, respond(outer.OvertimeID, M2AgentBoPrincipal, "cancel", "manager-cancel"))
	if err != nil || len(cancelled.Fact.Overtime.CancelledSchedules) != 2 {
		t.Fatalf("manager cancellation: %+v %v", cancelled, err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_schedule_entries WHERE definition_event_id=? AND status='cancelled'`, []any{accepted.EventID}, 2)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_schedule_entries WHERE agent_id=? AND world_time=? AND status='active'`, []any{job.EmployeeID, careerTime(1, 12, 0)}, 1)
	outer.Binding = careerTestBinding(t, s, M2AgentBoPrincipal, "replacement")
	outer.OvertimeID = "replacement"
	if _, err := s.OfferCareerOvertime(ctx, outer); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RespondCareerOvertime(ctx, respond(outer.OvertimeID, M2AgentAdaPrincipal, "accept", "accept-replacement")); err != nil {
		t.Fatalf("cancelled schedule IDs blocked a new agreement: %v", err)
	}
	if _, err := s.RespondCareerOvertime(ctx, respond(outer.OvertimeID, M2AgentAdaPrincipal, "cancel", "employee-cancel")); err != nil {
		t.Fatal(err)
	}
	approve.Binding = careerTestBinding(t, s, M2AgentBoPrincipal, "approve")
	if _, err := s.ReviewCareerLeave(ctx, approve); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RespondCareerOvertime(ctx, respond(conflict.OvertimeID, M2AgentAdaPrincipal, "accept", "accept-on-leave")); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatalf("accepted pending overtime on approved leave: %v", err)
	}
	if _, err := s.RunAgentLife(ctx, careerTime(2, 0, 1), 100); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT amount_due_minor FROM wage_obligations WHERE contract_id=? AND period_start_day=1`, []any{job.ContractID}, 12)
	if differences, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatalf("cancelled overtime replay: %+v %v", differences, err)
	}
}
