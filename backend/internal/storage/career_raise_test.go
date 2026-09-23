package storage

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestCareerRaiseEffectiveBoundaryOldWagesAndRecovery(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "raise.db")
	s := openCareerTestWorld(t, path)
	defer func() { s.Close() }()
	offer := prepareCareerEmploymentOffer(t, s)
	accepted, err := s.AcceptCareerOffer(ctx, core.CareerOfferAcceptRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "accept"), OfferID: offer.Fact.RecordID, AfterWorkPlaceID: M2AgentCafeID})
	if err != nil {
		t.Fatal(err)
	}
	job := accepted.Fact.Employment
	if _, err := s.RunAgentLife(ctx, careerTime(1, 8, 0), 100); err != nil {
		t.Fatal(err)
	}
	overtime := careerTestOvertime(t, s, job.ContractID, "overtime-after-raise")
	overtime.Day = 2
	if _, err := s.OfferCareerOvertime(ctx, overtime); err != nil {
		t.Fatal(err)
	}
	agreed, err := s.RespondCareerOvertime(ctx, core.CareerOvertimeResponseRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "accept-overtime"), OvertimeID: overtime.OvertimeID, Decision: "accept", Reason: "I agree"})
	if err != nil {
		t.Fatal(err)
	}
	r := core.CareerRaiseRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "raise"), ContractID: job.ContractID, DailyWageMinor: 20, EffectiveFromDay: 2, Notice: "Your base wage increases tomorrow; position unchanged."}
	bad := r
	bad.Binding.PrincipalID = M2AgentAdaPrincipal
	if _, err := s.RaiseCareerWage(ctx, bad); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("self-authorized raise: %v", err)
	}
	bad = r
	bad.DailyWageMinor = 11
	if _, err := s.RaiseCareerWage(ctx, bad); !core.HasCode(err, core.CodeInvalidArgument) {
		t.Fatalf("raise reduced salary: %v", err)
	}
	bad = r
	bad.EffectiveFromDay = 1
	if _, err := s.RaiseCareerWage(ctx, bad); !core.HasCode(err, core.CodeInvalidArgument) {
		t.Fatalf("backdated raise: %v", err)
	}
	bad = r
	bad.DailyWageMinor = core.MaxJSONSafeInteger
	if _, err := s.RaiseCareerWage(ctx, bad); !core.HasCode(err, core.CodeInvalidArgument) {
		t.Fatalf("raise overflowed accepted overtime total: %v", err)
	}
	s.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "interrupt raise") }
	if _, err := s.RaiseCareerWage(ctx, r); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatalf("raise rollback: %v", err)
	}
	s.beforeCommit = nil
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM scheduler_items WHERE json_extract(payload,'$.kind')='career_terms_effective'`, nil, 0)
	raised, err := s.RaiseCareerWage(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	if raised.Fact.Employment.TermVersion != 2 || raised.Fact.Employment.PositionID != job.PositionID {
		t.Fatalf("raise changed position or version: %+v", raised)
	}
	assertM2Value(t, ctx, s, `SELECT gross_wage_minor FROM employment_contracts WHERE contract_id=?`, []any{job.ContractID}, 12)
	before := readCareerTestContext(t, s, job.EmployeeID)
	if before.Life.Employment[0].WageMinor != 12 || before.OwnAssetMinor != 500 {
		t.Fatal("announced raise became current cash or wage too early")
	}
	found := false
	for _, memory := range before.Life.SalientMemories {
		if memory.Kind == "own_raise_announced" && memory.SourceEventID == raised.EventID {
			found = true
		}
	}
	if !found {
		t.Fatal("employee lacks announced raise notice")
	}
	if _, err := s.db.Exec(`UPDATE employment_contracts SET gross_wage_minor=999 WHERE contract_id=?`, job.ContractID); err != nil {
		t.Fatal(err)
	}
	if differences, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 1 || differences[0].Projection != "career_contract_wage" || differences[0].Expected != 12 {
		t.Fatalf("pending wage corruption not detected: %+v %v", differences, err)
	}
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT gross_wage_minor FROM employment_contracts WHERE contract_id=?`, []any{job.ContractID}, 12)
	bad = r
	bad.Binding = careerTestBinding(t, s, M2AgentBoPrincipal, "second-raise")
	bad.DailyWageMinor = 30
	if _, err := s.RaiseCareerWage(ctx, bad); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatalf("pending terms overwritten: %v", err)
	}
	if _, err := s.RunAgentLife(ctx, careerTime(1, 12, 0), 100); err != nil {
		t.Fatal(err)
	}
	s.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "interrupt activation") }
	if _, err := s.RunAgentLife(ctx, careerTime(2, 0, 0), 100); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatalf("activation rollback: %v", err)
	}
	s.beforeCommit = nil
	assertM2Value(t, ctx, s, `SELECT gross_wage_minor FROM employment_contracts WHERE contract_id=?`, []any{job.ContractID}, 12)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='CareerEmploymentTermsActivated'`, nil, 0)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if retry, err := s.RaiseCareerWage(ctx, r); err != nil || !retry.Replayed || retry.EventID != raised.EventID {
		t.Fatalf("raise recovery: %+v %v", retry, err)
	}
	if _, err := s.RunAgentLife(ctx, careerTime(2, 0, 1), 100); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT gross_wage_minor FROM employment_contracts WHERE contract_id=?`, []any{job.ContractID}, 20)
	assertM2Value(t, ctx, s, `SELECT amount_due_minor FROM wage_obligations WHERE contract_id=? AND period_start_day=1`, []any{job.ContractID}, 12)
	if life := readCareerTestContext(t, s, job.EmployeeID); life.Life.Employment[0].WageMinor != 20 || life.OwnAssetMinor != 512 {
		t.Fatalf("effective wage or prior pay wrong: %+v", life)
	}
	if _, err := s.RunAgentLife(ctx, careerTime(3, 0, 1), 100); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT amount_due_minor FROM wage_obligations WHERE contract_id=? AND period_start_day=2`, []any{job.ContractID}, 34)
	report, err := s.ReadCareerAttendance(ctx, M2AgentAdaPrincipal, M2DemoInstanceID, M2DemoBranchID, job.ContractID, 2)
	if err != nil || report.BaseEarnedMinor != 20 || report.OvertimeEarnedMinor != 14 || report.Overtime[0].AgreementEventID != agreed.EventID {
		t.Fatalf("raise rewrote agreed overtime: %+v %v", report, err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM outbox o JOIN events e ON e.event_id=o.event_id WHERE e.event_id=? OR e.event_type='CareerEmploymentTermsActivated'`, []any{raised.EventID}, 0)
	life := readCareerTestContext(t, s, job.EmployeeID)
	if _, err := s.db.Exec(`UPDATE employment_contracts SET gross_wage_minor=777 WHERE contract_id=?`, job.ContractID); err != nil {
		t.Fatal(err)
	}
	if differences, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 1 || differences[0].Projection != "career_contract_wage" || differences[0].Expected != 20 {
		t.Fatalf("effective wage corruption not detected: %+v %v", differences, err)
	}
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT gross_wage_minor FROM employment_contracts WHERE contract_id=?`, []any{job.ContractID}, 20)
	if !reflect.DeepEqual(life, readCareerTestContext(t, s, job.EmployeeID)) {
		t.Fatal("rebuild changed raised job or money")
	}
	if differences, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatalf("raise replay: %+v %v", differences, err)
	}
}

func TestCareerRaiseKeepsAlreadyEarnedArrears(t *testing.T) {
	ctx := context.Background()
	s := openCareerTestWorld(t, filepath.Join(t.TempDir(), "raised-arrears.db"))
	defer s.Close()
	offer := prepareCareerEmploymentOfferAtWage(t, s, 2000)
	accepted, err := s.AcceptCareerOffer(ctx, core.CareerOfferAcceptRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "accept"), OfferID: offer.Fact.RecordID, AfterWorkPlaceID: M2AgentCafeID})
	if err != nil {
		t.Fatal(err)
	}
	contract := accepted.Fact.Employment.ContractID
	if _, err := s.RunAgentLife(ctx, careerTime(2, 0, 1), 100); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT amount_due_minor-amount_paid_minor FROM wage_obligations WHERE contract_id=? AND period_start_day=1`, []any{contract}, 980)
	if _, err := s.RaiseCareerWage(ctx, core.CareerRaiseRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "raise"), ContractID: contract, DailyWageMinor: 2100, EffectiveFromDay: 3, Notice: "New rate applies from day three; old arrears remain due."}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RunAgentLife(ctx, careerTime(4, 0, 2), 200); err != nil {
		t.Fatal(err)
	}
	for day, amount := range map[int]int64{1: 2000, 2: 2000, 3: 2100} {
		assertM2Value(t, ctx, s, `SELECT amount_due_minor FROM wage_obligations WHERE contract_id=? AND period_start_day=?`, []any{contract, day}, amount)
	}
	assertM2Value(t, ctx, s, `SELECT SUM(amount_paid_minor) FROM wage_obligations WHERE contract_id=?`, []any{contract}, 1020)
	assertM2Value(t, ctx, s, `SELECT SUM(amount_due_minor-amount_paid_minor) FROM wage_obligations WHERE contract_id=?`, []any{contract}, 5080)
	if differences, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatalf("raised arrears replay: %+v %v", differences, err)
	}
}
