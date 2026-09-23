package storage

import (
	"context"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestCareerExitFinalEarningsVacancyAuthorityAndRecovery(t *testing.T) {
	for _, kind := range []string{"resignation", "termination", "layoff"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			path := filepath.Join(t.TempDir(), "exit.db")
			s := openCareerTestWorld(t, path)
			defer func() { s.Close() }()
			var caps []string
			if kind == "resignation" {
				caps = []string{core.CareerPositionManageCapability}
			}
			offer := prepareCareerEmploymentOfferAtWage(t, s, 12, caps...)
			accepted, err := s.AcceptCareerOffer(ctx, core.CareerOfferAcceptRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "accept"), OfferID: offer.Fact.RecordID, AfterWorkPlaceID: M2AgentCafeID})
			if err != nil {
				t.Fatal(err)
			}
			job := accepted.Fact.Employment
			if _, err := s.RunAgentLife(ctx, careerTime(1, 8, 0), 100); err != nil {
				t.Fatal(err)
			}
			overtime := careerTestOvertime(t, s, job.ContractID, "last-day-overtime")
			if _, err := s.OfferCareerOvertime(ctx, overtime); err != nil {
				t.Fatal(err)
			}
			if _, err := s.RespondCareerOvertime(ctx, core.CareerOvertimeResponseRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "accept-overtime"), OvertimeID: overtime.OvertimeID, Decision: "accept", Reason: "Agreed final-day overtime"}); err != nil {
				t.Fatal(err)
			}
			principal := M2AgentBoPrincipal
			if kind == "resignation" {
				principal = M2AgentAdaPrincipal
			}
			r := core.CareerExitRequest{Binding: careerTestBinding(t, s, principal, "exit"), ContractID: job.ContractID, Kind: kind, EffectiveFromDay: 2, Notice: "Employment ends from day two; prior earnings remain due."}
			bad := r
			if kind == "resignation" {
				bad.Binding.PrincipalID = M2AgentBoPrincipal
			} else {
				bad.Binding.PrincipalID = M2AgentAdaPrincipal
			}
			if _, err := s.EndCareerEmployment(ctx, bad); !core.HasCode(err, core.CodeUnauthorized) {
				t.Fatalf("wrong exit authority: %v", err)
			}
			s.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "exit notice rollback") }
			if _, err := s.EndCareerEmployment(ctx, r); !core.HasCode(err, core.CodeInjectedFailure) {
				t.Fatalf("notice rollback: %v", err)
			}
			s.beforeCommit = nil
			notice, err := s.EndCareerEmployment(ctx, r)
			if err != nil {
				t.Fatal(err)
			}
			if notice.Fact.Employment.EndsOnDay != 2 || notice.Fact.Exit.Kind != kind {
				t.Fatalf("exit terms: %+v", notice)
			}
			assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM employment_contracts WHERE contract_id=? AND status='active'`, []any{job.ContractID}, 1)
			if own := readCareerTestContext(t, s, M2AgentAdaID); own.Life.Unemployment != nil || len(own.Life.Employment) != 1 {
				t.Fatal("exit became effective early")
			}
			future := careerTestOvertime(t, s, job.ContractID, "post-exit-overtime")
			future.Day = 2
			if _, err := s.OfferCareerOvertime(ctx, future); !core.HasCode(err, core.CodeBranchConflict) {
				t.Fatalf("overtime after exit allowed: %v", err)
			}
			if _, err := s.RequestCareerLeave(ctx, core.CareerLeaveRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "post-exit-leave"), LeaveID: "post-exit-leave", ContractID: job.ContractID, StartDay: 2, EndDay: 3, Reason: "Leave after exit"}); !core.HasCode(err, core.CodeBranchConflict) {
				t.Fatalf("leave after exit allowed: %v", err)
			}
			if _, err := s.RunAgentLife(ctx, careerTime(1, 15, 0), 100); err != nil {
				t.Fatal(err)
			}
			s.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "exit activation rollback") }
			if _, err := s.RunAgentLife(ctx, careerTime(2, 0, 0), 100); !core.HasCode(err, core.CodeInjectedFailure) {
				t.Fatalf("activation rollback: %v", err)
			}
			s.beforeCommit = nil
			assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM employment_contracts WHERE contract_id=? AND status='active'`, []any{job.ContractID}, 1)
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			s, err = Open(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			if replay, err := s.EndCareerEmployment(ctx, r); err != nil || !replay.Replayed || replay.EventID != notice.EventID {
				t.Fatalf("notice replay: %+v %v", replay, err)
			}
			if _, err := s.RunAgentLife(ctx, careerTime(2, 0, 1), 100); err != nil {
				t.Fatal(err)
			}
			assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM employment_contracts WHERE contract_id=? AND status='ended'`, []any{job.ContractID}, 1)
			assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM capability_grants WHERE principal_id=? AND capability_id=? AND status='active'`, []any{M2AgentAdaPrincipal, core.CareerPositionManageCapability}, 0)
			assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_schedule_entries WHERE agent_id=? AND world_time=? AND activity_code='work' AND status='active'`, []any{M2AgentAdaID, careerTime(2, 8, 0)}, 0)
			assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_schedule_entries WHERE agent_id=? AND world_time=? AND place_id=? AND status='active'`, []any{M2AgentAdaID, careerTime(2, 12, 0), M2AgentCafeID}, 1)
			report, err := s.ReadCareerAttendance(ctx, M2AgentAdaPrincipal, M2DemoInstanceID, M2DemoBranchID, job.ContractID, 1)
			if err != nil || report.BaseEarnedMinor != 12 || report.OvertimeEarnedMinor != 14 {
				t.Fatalf("final earned work lost: %+v %v", report, err)
			}
			assertM2Value(t, ctx, s, `SELECT amount_paid_minor FROM wage_obligations WHERE contract_id=? AND period_start_day=1`, []any{job.ContractID}, 26)
			market, err := s.DiscoverCareerPositions(ctx, M2AgentAdaPrincipal, M2DemoInstanceID, M2DemoBranchID, M2AgentAdaID, 0)
			if err != nil || len(market.Postings) != 1 || market.Postings[0].AvailableSlots != 1 {
				t.Fatalf("exit vacancy: %+v %v", market, err)
			}
			own := readCareerTestContext(t, s, M2AgentAdaID)
			if len(own.Life.Employment) != 0 || own.Life.Unemployment == nil || own.Life.Unemployment.PreviousContractID != job.ContractID || own.Life.Unemployment.Kind != kind {
				t.Fatalf("unemployment context: %+v", own.Life)
			}
			found := false
			for _, goal := range own.Life.Goals {
				if goal.Code == "find_work" && len(goal.SourceEventIDs) == 1 && goal.SourceEventIDs[0] == own.Life.Unemployment.SourceEventID {
					found = true
				}
			}
			if !found {
				t.Fatal("unemployment did not change sourced goals")
			}
			if _, err := s.RunAgentLife(ctx, careerTime(4, 12, 0), 100); err != nil {
				t.Fatal(err)
			}
			assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM wage_obligations WHERE contract_id=?`, []any{job.ContractID}, 1)
			assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_movements WHERE agent_id=? AND activity_code='work' AND world_time>=?`, []any{M2AgentAdaID, careerTime(2, 0, 0)}, 0)
			if _, err := s.db.Exec(`UPDATE employment_contracts SET status='active' WHERE contract_id=?`, job.ContractID); err != nil {
				t.Fatal(err)
			}
			if diffs, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diffs) != 1 || diffs[0].Projection != "career_contract_status" {
				t.Fatalf("ended status corruption: %+v %v", diffs, err)
			}
			if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
				t.Fatal(err)
			}
			if own := readCareerTestContext(t, s, M2AgentAdaID); own.Life.Unemployment == nil {
				t.Fatal("rebuild revived ended job")
			}
			// Re-enter through real discovery/application/interview/evaluation/
			// offer/acceptance, not by reviving or editing the ended contract.
			reoffer := offerExistingCareerPositionTo(t, s, M2AgentAdaID, M2AgentAdaPrincipal, 5)
			rehired, err := s.AcceptCareerOffer(ctx, core.CareerOfferAcceptRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "rehire"), OfferID: reoffer.Fact.RecordID, AfterWorkPlaceID: M2AgentCafeID})
			if err != nil {
				t.Fatal(err)
			}
			if rehired.Fact.Employment.ContractID == job.ContractID {
				t.Fatal("reemployment rewrote old contract")
			}
			if _, err := s.RunAgentLife(ctx, careerTime(6, 0, 1), 100); err != nil {
				t.Fatal(err)
			}
			assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM wage_obligations WHERE contract_id=?`, []any{job.ContractID}, 1)
			assertM2Value(t, ctx, s, `SELECT amount_paid_minor FROM wage_obligations WHERE contract_id=? AND period_start_day=5`, []any{rehired.Fact.Employment.ContractID}, 12)
			if own := readCareerTestContext(t, s, M2AgentAdaID); own.Life.Unemployment != nil || len(own.Life.Employment) != 1 || own.Life.Employment[0].ContractID != rehired.Fact.Employment.ContractID {
				t.Fatal("reemployment did not replace unemployment context")
			}
			if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCareerExitArrearsPersistWithoutNewEarnings(t *testing.T) {
	ctx := context.Background()
	s := openCareerTestWorld(t, filepath.Join(t.TempDir(), "exit-debt.db"))
	defer s.Close()
	offer := prepareCareerEmploymentOfferAtWage(t, s, 2000)
	accepted, err := s.AcceptCareerOffer(ctx, core.CareerOfferAcceptRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "accept"), OfferID: offer.Fact.RecordID, AfterWorkPlaceID: M2AgentCafeID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.RunAgentLife(ctx, careerTime(1, 8, 0), 100); err != nil {
		t.Fatal(err)
	}
	job := accepted.Fact.Employment
	if _, err := s.EndCareerEmployment(ctx, core.CareerExitRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "layoff"), ContractID: job.ContractID, Kind: "layoff", EffectiveFromDay: 2, Notice: "Workforce reduction, earned pay remains due"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RunAgentLife(ctx, careerTime(4, 0, 2), 100); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM wage_obligations WHERE contract_id=?`, []any{job.ContractID}, 1)
	assertM2Value(t, ctx, s, `SELECT amount_due_minor-amount_paid_minor FROM wage_obligations WHERE contract_id=?`, []any{job.ContractID}, 980)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM scheduler_items WHERE status='pending' AND json_extract(payload,'$.kind')='career_wage_retry' AND json_extract(payload,'$.subject_id')=?`, []any{"wage_" + job.ContractID + "_1_2"}, 1)
	if own := readCareerTestContext(t, s, M2AgentAdaID); own.Life.ReceivableMinor < 980 || own.Life.Unemployment == nil {
		t.Fatal("exit lost receivable or unemployment")
	}
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
}

func TestCareerExitRequiresExplicitOvertimeResolutionAndRejectsPendingTerms(t *testing.T) {
	ctx := context.Background()
	s := openCareerTestWorld(t, filepath.Join(t.TempDir(), "exit-guards.db"))
	defer s.Close()
	offer := prepareCareerEmploymentOffer(t, s)
	accepted, err := s.AcceptCareerOffer(ctx, core.CareerOfferAcceptRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "accept"), OfferID: offer.Fact.RecordID, AfterWorkPlaceID: M2AgentCafeID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.RunAgentLife(ctx, careerTime(1, 8, 0), 100); err != nil {
		t.Fatal(err)
	}
	ot := careerTestOvertime(t, s, accepted.Fact.Employment.ContractID, "future-overtime")
	ot.Day = 2
	if _, err := s.OfferCareerOvertime(ctx, ot); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RespondCareerOvertime(ctx, core.CareerOvertimeResponseRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "accept-ot"), OvertimeID: ot.OvertimeID, Decision: "accept", Reason: "Agreed"}); err != nil {
		t.Fatal(err)
	}
	r := core.CareerExitRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "exit"), ContractID: accepted.Fact.Employment.ContractID, Kind: "resignation", EffectiveFromDay: 2, Notice: "I resign"}
	if _, err := s.EndCareerEmployment(ctx, r); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatalf("exit silently dropped overtime: %v", err)
	}
	if _, err := s.RespondCareerOvertime(ctx, core.CareerOvertimeResponseRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "cancel-ot"), OvertimeID: ot.OvertimeID, Decision: "cancel", Reason: "Resolve agreed overtime before resignation"}); err != nil {
		t.Fatal(err)
	}
	r.Binding = careerTestBinding(t, s, M2AgentAdaPrincipal, "exit")
	for _, day := range []int{1, 32} {
		bad := r
		bad.EffectiveFromDay = day
		if _, err := s.EndCareerEmployment(ctx, bad); !core.HasCode(err, core.CodeInvalidArgument) {
			t.Fatalf("exit day %d: %v", day, err)
		}
	}
	if _, err := s.RaiseCareerWage(ctx, core.CareerRaiseRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "pending-raise"), ContractID: r.ContractID, DailyWageMinor: 20, EffectiveFromDay: 2, Notice: "Raise first"}); err != nil {
		t.Fatal(err)
	}
	r.Binding = careerTestBinding(t, s, M2AgentAdaPrincipal, "exit")
	if _, err := s.EndCareerEmployment(ctx, r); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatalf("exit overwrote pending terms: %v", err)
	}
	if _, err := s.RunAgentLife(ctx, careerTime(2, 0, 1), 100); err != nil {
		t.Fatal(err)
	}
	r.Binding, r.EffectiveFromDay = careerTestBinding(t, s, M2AgentAdaPrincipal, "exit"), 3
	if _, err := s.EndCareerEmployment(ctx, r); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RunAgentLife(ctx, careerTime(3, 0, 1), 100); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT amount_due_minor FROM wage_obligations WHERE contract_id=? AND period_start_day=2`, []any{r.ContractID}, 20)
}
