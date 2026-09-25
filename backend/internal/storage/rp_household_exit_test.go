package storage

import (
	"context"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPHouseholdMemberExitPreservesHistoryAndPrivacy(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "household-exit.db")
	s := openM2AgentStore(t, ctx, path, false)
	defer func() { s.Close() }()
	if _, err := s.BootstrapRPPlayDemo(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO principals(principal_id,principal_type,display_name,status) VALUES ('principal_f4_exit_operator','operator','F4 exit operator','active')`); err != nil {
		t.Fatal(err)
	}
	operator := func(key string) core.CareerBinding {
		var head int64
		if err := s.db.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&head); err != nil {
			t.Fatal(err)
		}
		return core.CareerBinding{PrincipalID: "principal_f4_exit_operator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: head, IdempotencyKey: key}
	}
	home, err := s.FoundRPHouseholdLocal(ctx, RPHouseholdFoundRequest{Binding: operator("exit-home"), HouseholdKey: "exit-home", DisplayName: "Exit home", ResidencePlaceID: M2AgentCafeID, AdultEntityIDs: [2]string{M2AgentAdaID, M2AgentBoID}})
	if err != nil {
		t.Fatal(err)
	}
	agreement, err := s.AgreeRPHouseholdRentLocal(ctx, RPHouseholdRentAgreementRequest{Binding: operator("exit-rent"), HouseholdID: home.Fact.HouseholdID, AgreementKey: "exit-rent", LandlordName: "Exit landlord", RentMinor: 300, PeriodDays: 30, GraceDays: 3, Shares: [2]RPHouseholdRentShare{{M2AgentAdaID, 180}, {M2AgentBoID, 120}}})
	if err != nil {
		t.Fatal(err)
	}
	ada := allowFixtureControl(t, ctx, s, M2AgentAdaID)
	observed, err := s.ObserveRPSession(ctx, ada)
	if err != nil {
		t.Fatal(err)
	}
	contribution := RPHouseholdRentContributionRequest{Binding: core.CareerBinding{PrincipalID: ada.PrincipalID, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: observed.ObservationCursor, IdempotencyKey: "ada-before-exit-rent"}, SessionID: ada.SessionID, AgreementID: agreement.Fact.AgreementID, AmountMinor: 50}
	funded, err := s.ContributeRPHouseholdRent(ctx, contribution)
	if err != nil {
		t.Fatal(err)
	}
	observed, err = s.ObserveRPSession(ctx, ada)
	if err != nil {
		t.Fatal(err)
	}
	request := RPHouseholdMemberExitRequest{
		Binding:   core.CareerBinding{PrincipalID: ada.PrincipalID, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: observed.ObservationCursor, IdempotencyKey: "ada-leaves-home"},
		SessionID: ada.SessionID, HouseholdID: home.Fact.HouseholdID, Reason: "Moving to a separate home",
	}
	foreign := request
	foreign.Binding.PrincipalID = M2AgentBoPrincipal
	foreign.Binding.IdempotencyKey = "foreign-exit"
	if _, err := s.LeaveRPHousehold(ctx, foreign); !core.HasCode(err, core.CodeNotFound) {
		t.Fatal("foreign principal used another session", err)
	}
	s.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "household exit rollback") }
	if _, err := s.LeaveRPHousehold(ctx, request); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatal("exit rollback", err)
	}
	s.beforeCommit = nil
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_household_memberships WHERE household_id=? AND ended_event_id IS NULL`, []any{home.Fact.HouseholdID}, 2)
	left, err := s.LeaveRPHousehold(ctx, request)
	if err != nil || left.Fact.MemberEntityID != M2AgentAdaID || left.Fact.Reason != request.Reason {
		t.Fatal("self exit", left, err)
	}
	if retry, err := s.LeaveRPHousehold(ctx, request); err != nil || !retry.Replayed || retry.EventID != left.EventID {
		t.Fatal("lost-response exit replay", retry, err)
	}
	changed := request
	changed.Reason = "Different reason"
	if _, err := s.LeaveRPHousehold(ctx, changed); !core.HasCode(err, core.CodeIdempotencyMismatch) {
		t.Fatal("changed exit retry", err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_household_memberships WHERE household_id=? AND member_entity_id=? AND source_event_id=? AND ended_event_id=? AND ended_world_time=?`, []any{home.Fact.HouseholdID, M2AgentAdaID, home.EventID, left.EventID, left.WorldTime}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_household_rent_shares WHERE agreement_id=?`, []any{agreement.Fact.AgreementID}, 2)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_household_rent_contributions WHERE contribution_id=? AND amount_minor=50`, []any{funded.Fact.ContributionID}, 1)
	if receipt, err := s.ContributeRPHouseholdRent(ctx, contribution); err != nil || !receipt.Replayed || receipt.EventID != funded.EventID {
		t.Fatal("historical contribution receipt after exit", receipt, err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_profiles WHERE agent_id IN (?,?) AND status='active'`, []any{M2AgentAdaID, M2AgentBoID}, 2)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM outbox WHERE event_id=?`, []any{left.EventID}, 0)
	if _, err := s.ReadRPHousehold(ctx, ada, home.Fact.HouseholdID); !core.HasCode(err, core.CodeNotFound) {
		t.Fatal("departed member read private household", err)
	}
	observed, err = s.ObserveRPSession(ctx, ada)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ContributeRPHouseholdRent(ctx, RPHouseholdRentContributionRequest{Binding: core.CareerBinding{PrincipalID: ada.PrincipalID, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: observed.ObservationCursor, IdempotencyKey: "departed-rent"}, SessionID: ada.SessionID, AgreementID: agreement.Fact.AgreementID, AmountMinor: 1}); !core.HasCode(err, core.CodeNotFound) {
		t.Fatal("departed member funded old share", err)
	}
	if own := readCareerTestContext(t, s, M2AgentAdaID); own.Life.HouseholdPressure != nil {
		t.Fatal("departed member retained household pressure")
	}
	grantRPControlForTest(t, ctx, s, M2AgentBoID)
	boSession, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: rpTestPrincipal, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2AgentBoID, POV: "second_person", IdempotencyKey: "bo-exit-read"})
	if err != nil {
		t.Fatal(err)
	}
	bo := core.RPSessionReadRequest{PrincipalID: rpTestPrincipal, SessionID: boSession.SessionID}
	boView, err := s.ReadRPHousehold(ctx, bo, home.Fact.HouseholdID)
	if err != nil || boView.MemberCount != 1 || boView.Rent == nil || boView.Rent.OwnShareMinor != 120 {
		t.Fatal("remaining member view", boView, err)
	}
	boObserved, err := s.ObserveRPSession(ctx, bo)
	if err != nil {
		t.Fatal(err)
	}
	last := RPHouseholdMemberExitRequest{Binding: core.CareerBinding{PrincipalID: bo.PrincipalID, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: boObserved.ObservationCursor, IdempotencyKey: "bo-last-exit"}, SessionID: bo.SessionID, HouseholdID: home.Fact.HouseholdID, Reason: "Would leave no adult"}
	if _, err := s.LeaveRPHousehold(ctx, last); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatal("last adult left an active lease", err)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("exit source comparison", diff, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE rp_household_memberships SET ended_event_id=NULL,ended_world_time=NULL WHERE household_id=? AND member_entity_id=?`, home.Fact.HouseholdID, M2AgentAdaID); err != nil {
		t.Fatal(err)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) == 0 {
		t.Fatal("erased exit history was not detected", diff, err)
	}
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal("rebuild exit history", err)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("exit history repair", diff, err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_household_memberships WHERE household_id=? AND member_entity_id=? AND ended_event_id=? AND ended_world_time=?`, []any{home.Fact.HouseholdID, M2AgentAdaID, left.EventID, left.WorldTime}, 1)
	if _, err := s.ReadRPHousehold(ctx, ada, home.Fact.HouseholdID); !core.HasCode(err, core.CodeNotFound) {
		t.Fatal("rebuild resurrected member privacy", err)
	}
}
