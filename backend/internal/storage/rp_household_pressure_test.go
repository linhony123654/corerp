package storage

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPHouseholdIncomeShockChangesMemberGoal(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "household-pressure.db")
	s := openCareerTestWorld(t, path)
	defer func() { s.Close() }()
	offer := prepareCareerEmploymentOfferAtWage(t, s, 12)
	if _, err := s.AcceptCareerOffer(ctx, core.CareerOfferAcceptRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "pressure-accept-job"), OfferID: offer.Fact.RecordID, AfterWorkPlaceID: M2AgentCafeID}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RunAgentLife(ctx, careerTime(1, 8, 0), 1000); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO principals(principal_id,principal_type,display_name,status) VALUES ('principal_f4_pressure_operator','operator','F4 pressure operator','active')`); err != nil {
		t.Fatal(err)
	}
	binding := func(key string) core.CareerBinding {
		var head int64
		if err := s.db.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&head); err != nil {
			t.Fatal(err)
		}
		return core.CareerBinding{PrincipalID: "principal_f4_pressure_operator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: head, IdempotencyKey: key}
	}
	home, err := s.FoundRPHouseholdLocal(ctx, RPHouseholdFoundRequest{Binding: binding("pressure-home"), HouseholdKey: "pressure-home", DisplayName: "Pressure home", ResidencePlaceID: M2AgentCafeID, AdultEntityIDs: [2]string{M2AgentAdaID, M2RPNPCID}})
	if err != nil {
		t.Fatal(err)
	}
	agreement, err := s.AgreeRPHouseholdRentLocal(ctx, RPHouseholdRentAgreementRequest{Binding: binding("pressure-rent"), HouseholdID: home.Fact.HouseholdID, AgreementKey: "pressure-rent", LandlordName: "Pressure landlord", RentMinor: 500, PeriodDays: 30, GraceDays: 3, Shares: [2]RPHouseholdRentShare{{M2AgentAdaID, 250}, {M2RPNPCID, 250}}})
	if err != nil {
		t.Fatal(err)
	}
	before := readCareerTestContext(t, s, M2AgentAdaID)
	if before.Life.HouseholdPressure == nil || before.Life.HouseholdPressure.ExpectedIncomeMinor <= 500 || before.Life.HouseholdPressure.CoverageGapMinor != 0 || before.Life.HouseholdPressure.PressureLevel != "covered" {
		t.Fatalf("two-income household forecast: %+v", before.Life.HouseholdPressure)
	}
	for _, goal := range before.Life.Goals {
		if goal.Code == "stabilize_household_income" {
			t.Fatal("pressure goal before income shock")
		}
	}
	before.PlayerSpeechText = "你好"
	beforeProposal, err := (core.DeterministicRPDecisionProvider{}).Propose(ctx, before)
	if err != nil || strings.Contains(beforeProposal.Text, "房租") {
		t.Fatalf("household rent decision before shock: %+v %v", beforeProposal, err)
	}
	if _, err := s.RequestCareerAggregateExit(ctx, core.CareerAggregateExitRequest{Binding: careerTestBinding(t, s, M2RPNPCPrincipal, "pressure-income-exit"), CandidateID: M2RPNPCID, ContractID: m2EconomyContractID, FinalEarnedDay: 2, Notice: "Leaving the wage pool"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RunAgentLife(ctx, careerTime(3, 0, 0), 1000); err != nil {
		t.Fatal(err)
	}
	after := readCareerTestContext(t, s, M2AgentAdaID)
	pressure := after.Life.HouseholdPressure
	if pressure == nil || pressure.ExpectedIncomeMinor >= before.Life.HouseholdPressure.ExpectedIncomeMinor || pressure.CoverageGapMinor <= 0 {
		t.Fatalf("income shock did not cause budget pressure: %+v", pressure)
	}
	found := false
	for _, goal := range after.Life.Goals {
		found = found || goal.Code == "stabilize_household_income"
	}
	if !found {
		t.Fatalf("income shock did not change work goal: %+v", after.Life.Goals)
	}
	after.PlayerSpeechText = before.PlayerSpeechText
	afterProposal, err := (core.DeterministicRPDecisionProvider{}).Propose(ctx, after)
	if err != nil || afterProposal.Action != "refuse" || !strings.Contains(afterProposal.Text, "房租") || afterProposal == beforeProposal {
		t.Fatalf("income shock did not change actual decision: before=%+v after=%+v err=%v", beforeProposal, afterProposal, err)
	}
	if outsider := readCareerTestContext(t, s, M2AgentBoID); outsider.Life.HouseholdPressure != nil {
		t.Fatalf("nonmember gained household finances: %+v", outsider.Life.HouseholdPressure)
	}
	bounded, err := json.Marshal(pressure)
	if err != nil || strings.Contains(string(bounded), M2RPNPCID) || strings.Contains(string(bounded), m2EconomyContractID) || strings.Contains(string(bounded), "expected_income_minor") || strings.Contains(string(bounded), "coverage_gap_minor") || pressure.PressureLevel != "at_risk" {
		t.Fatalf("household pressure leaked member or wage source: %s %v", bounded, err)
	}
	serializedDecision, err := json.Marshal(after)
	if err != nil || strings.Contains(string(serializedDecision), agreement.EventID) || strings.Contains(string(serializedDecision), agreement.Fact.AgreementID) {
		t.Fatalf("household source escaped complete model decision input: %s %v", serializedDecision, err)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("household pressure source replay", diff, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	reopened := readCareerTestContext(t, s, M2AgentAdaID)
	if reopened.Life.HouseholdPressure == nil || reopened.Life.HouseholdPressure.CoverageGapMinor != pressure.CoverageGapMinor {
		t.Fatalf("household pressure changed after reopen: %+v", reopened.Life.HouseholdPressure)
	}
}
