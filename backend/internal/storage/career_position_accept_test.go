package storage

import (
	"context"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestCareerPositionAcceptanceActivationReservationsRecovery(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "position-accept.db")
	s := openCareerTestWorld(t, path)
	defer func() { s.Close() }()
	initial, proposal := prepareCareerPositionOffer(t, s, "senior")
	overtime := careerTestOvertime(t, s, initial.Fact.Employment.ContractID, "move-overtime")
	overtime.Day = 3
	if _, err := s.OfferCareerOvertime(ctx, overtime); err != nil {
		t.Fatal(err)
	}
	agreement, err := s.RespondCareerOvertime(ctx, core.CareerOvertimeResponseRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "accept-overtime"), OvertimeID: overtime.OvertimeID, Decision: "accept", Reason: "Agreed before position change"})
	if err != nil {
		t.Fatal(err)
	}
	proposal.Binding = careerTestBinding(t, s, M2AgentBoPrincipal, "position-offer")
	offer, err := s.OfferCareerPositionChange(ctx, proposal)
	if err != nil {
		t.Fatal(err)
	}
	r := core.CareerPositionAcceptRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "accept-change"), ChangeID: proposal.ChangeID}
	wrong := r
	wrong.Binding.PrincipalID = M2AgentBoPrincipal
	if _, err := s.AcceptCareerPositionChange(ctx, wrong); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("manager accepted for employee: %v", err)
	}
	s.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "accept rollback") }
	if _, err := s.AcceptCareerPositionChange(ctx, r); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatalf("accept rollback: %v", err)
	}
	s.beforeCommit = nil
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM scheduler_items WHERE json_extract(payload,'$.kind')='career_terms_effective'`, nil, 0)
	accepted, err := s.AcceptCareerPositionChange(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	job := accepted.Fact.Employment
	if job == nil || accepted.Fact.PositionAssessment != nil || accepted.Fact.PositionChange.OfferEventID != offer.EventID {
		t.Fatalf("accepted source/privacy: %+v", accepted)
	}
	assertMarket := func(old, target int) {
		t.Helper()
		market, err := s.DiscoverCareerPositions(ctx, M2AgentAdaPrincipal, M2DemoInstanceID, M2DemoBranchID, M2AgentAdaID, 0)
		if err != nil {
			t.Fatal(err)
		}
		seen := 0
		for _, p := range market.Postings {
			if p.Posting.PositionID == initial.Fact.Employment.PositionID {
				seen++
				if p.AvailableSlots != old {
					t.Fatalf("old vacancy %d != %d", p.AvailableSlots, old)
				}
			}
			if p.Posting.PositionID == job.PositionID {
				seen++
				if p.AvailableSlots != target {
					t.Fatalf("target vacancy %d != %d", p.AvailableSlots, target)
				}
			}
		}
		if seen != 2 {
			t.Fatal("missing postings")
		}
	}
	assertMarket(0, 0)
	assertM2Value(t, ctx, s, `SELECT gross_wage_minor FROM employment_contracts WHERE contract_id=?`, []any{job.ContractID}, 12)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM employment_contracts WHERE contract_id=? AND position_id=?`, []any{job.ContractID, initial.Fact.Employment.PositionKey}, 1)
	if got := readCareerTestContext(t, s, M2AgentAdaID); got.Life.Employment[0].WageMinor != 12 {
		t.Fatal("future terms became current early")
	}
	memories := readCareerTestContext(t, s, M2AgentAdaID).Life.SalientMemories
	found := false
	for _, m := range memories {
		if m.Kind == "own_position_change_agreed" && m.SourceEventID == accepted.EventID {
			found = true
		}
	}
	if !found {
		t.Fatal("accepted position missing from employee memory")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := s.AcceptCareerPositionChange(ctx, r); err != nil || !got.Replayed || got.EventID != accepted.EventID {
		t.Fatalf("accept retry: %+v %v", got, err)
	}
	assertMarket(0, 0)
	if _, err := s.RunAgentLife(ctx, careerTime(2, 12, 0), 100); err != nil {
		t.Fatal(err)
	}
	s.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "activation rollback") }
	if _, err := s.RunAgentLife(ctx, careerTime(3, 0, 0), 100); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatalf("activation rollback: %v", err)
	}
	s.beforeCommit = nil
	assertMarket(0, 0)
	assertM2Value(t, ctx, s, `SELECT gross_wage_minor FROM employment_contracts WHERE contract_id=?`, []any{job.ContractID}, 12)
	if _, err := s.RunAgentLife(ctx, careerTime(4, 0, 1), 100); err != nil {
		t.Fatal(err)
	}
	assertMarket(1, 0)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM employment_contracts WHERE contract_id=? AND position_id=? AND gross_wage_minor=20`, []any{job.ContractID, job.PositionKey}, 1)
	for day, amount := range map[int]int64{2: 12, 3: 20} {
		report, err := s.ReadCareerAttendance(ctx, M2AgentAdaPrincipal, M2DemoInstanceID, M2DemoBranchID, job.ContractID, day)
		if err != nil || report.BaseEarnedMinor != amount {
			t.Fatalf("day %d old/new pay: %+v %v", day, report, err)
		}
		if day == 3 && (report.OvertimeEarnedMinor != 14 || len(report.Overtime) != 1 || report.Overtime[0].AgreementEventID != agreement.EventID) {
			t.Fatalf("accepted overtime lost on move: %+v", report)
		}
	}
	if _, err := s.db.Exec(`UPDATE employment_contracts SET position_id=?,gross_wage_minor=777 WHERE contract_id=?`, initial.Fact.Employment.PositionKey, job.ContractID); err != nil {
		t.Fatal(err)
	}
	diffs, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID)
	if err != nil || len(diffs) != 2 {
		t.Fatalf("position/wage corruption: %+v %v", diffs, err)
	}
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	assertMarket(1, 0)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM employment_contracts WHERE contract_id=? AND position_id=? AND gross_wage_minor=20`, []any{job.ContractID, job.PositionKey}, 1)
	if got := readCareerTestContext(t, s, M2AgentAdaID); got.Life.Employment[0].WageMinor != 20 {
		t.Fatal("effective career context differs")
	}
	// A later move back must release the first target; historical accepted
	// reservations cannot accumulate permanently after their activation.
	report, err := s.ReadCareerAttendance(ctx, M2AgentBoPrincipal, M2DemoInstanceID, M2DemoBranchID, job.ContractID, 3)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.RecordCareerPerformance(ctx, core.CareerPerformanceRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "return-review"), ReviewID: "return-review", ContractID: job.ContractID, EvidenceEventIDs: []string{report.EventID}, Assessment: "meets_expectations", Reason: "Current work in the changed role"}); err != nil {
		t.Fatal(err)
	}
	back := core.CareerPositionOfferRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "return-offer"), ChangeID: "return-offer", ReviewID: "return-review", PositionID: initial.Fact.Employment.PositionID, EffectiveFromDay: 5, Assessments: []core.CareerQualificationAssessment{{Code: "safety_training", Passed: true, Reason: "Current review supports return qualification"}}, Notice: "Return to earlier role with its agreed pay"}
	if _, err := s.OfferCareerPositionChange(ctx, back); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AcceptCareerPositionChange(ctx, core.CareerPositionAcceptRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "accept-return"), ChangeID: back.ChangeID}); err != nil {
		t.Fatal(err)
	}
	assertMarket(0, 0)
	if _, err := s.RunAgentLife(ctx, careerTime(5, 0, 1), 100); err != nil {
		t.Fatal(err)
	}
	assertMarket(0, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM employment_contracts WHERE contract_id=? AND position_id=? AND gross_wage_minor=12`, []any{job.ContractID, initial.Fact.Employment.PositionKey}, 1)
}

func TestCareerPositionAcceptanceDirectionsAndRevalidation(t *testing.T) {
	for _, grade := range []string{"junior", "trainee"} {
		t.Run(grade, func(t *testing.T) {
			ctx := context.Background()
			s := openCareerTestWorld(t, filepath.Join(t.TempDir(), "move.db"))
			defer s.Close()
			_, proposal := prepareCareerPositionOffer(t, s, grade)
			if _, err := s.OfferCareerPositionChange(ctx, proposal); err != nil {
				t.Fatal(err)
			}
			accepted, err := s.AcceptCareerPositionChange(ctx, core.CareerPositionAcceptRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "accept"), ChangeID: proposal.ChangeID})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.RunAgentLife(ctx, careerTime(3, 0, 1), 100); err != nil {
				t.Fatal(err)
			}
			assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM employment_contracts WHERE contract_id=? AND position_id=? AND gross_wage_minor=20`, []any{accepted.Fact.Employment.ContractID, accepted.Fact.Employment.PositionKey}, 1)
		})
	}
	for _, mode := range []string{"superseded-review", "pending-raise", "expired", "revoked-manager"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			s := openCareerTestWorld(t, filepath.Join(t.TempDir(), "revalidate.db"))
			defer s.Close()
			initial, proposal := prepareCareerPositionOffer(t, s, "senior")
			if _, err := s.OfferCareerPositionChange(ctx, proposal); err != nil {
				t.Fatal(err)
			}
			want := core.CodeBranchConflict
			switch mode {
			case "superseded-review":
				report, err := s.ReadCareerAttendance(ctx, M2AgentBoPrincipal, M2DemoInstanceID, M2DemoBranchID, initial.Fact.Employment.ContractID, 1)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := s.RecordCareerPerformance(ctx, core.CareerPerformanceRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "new-review"), ReviewID: "new-review", ContractID: initial.Fact.Employment.ContractID, EvidenceEventIDs: []string{report.EventID}, Assessment: "needs_improvement", Reason: "New judgment"}); err != nil {
					t.Fatal(err)
				}
			case "pending-raise":
				if _, err := s.RaiseCareerWage(ctx, core.CareerRaiseRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "raise"), ContractID: initial.Fact.Employment.ContractID, DailyWageMinor: 15, EffectiveFromDay: 3, Notice: "Raise tomorrow"}); err != nil {
					t.Fatal(err)
				}
			case "expired":
				if _, err := s.RunAgentLife(ctx, careerTime(3, 0, 1), 100); err != nil {
					t.Fatal(err)
				}
				want = core.CodeInvalidArgument
			case "revoked-manager":
				if _, err := s.db.Exec(`UPDATE capability_grants SET status='revoked' WHERE capability_id=? AND principal_id=?`, careerManageCapability, M2AgentBoPrincipal); err != nil {
					t.Fatal(err)
				}
				want = core.CodeUnauthorized
			}
			if _, err := s.AcceptCareerPositionChange(ctx, core.CareerPositionAcceptRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "accept"), ChangeID: proposal.ChangeID}); !core.HasCode(err, want) {
				t.Fatalf("%s accepted: %v", mode, err)
			}
			assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE json_extract(payload,'$.position_change.status')='accepted'`, nil, 0)
		})
	}
}
