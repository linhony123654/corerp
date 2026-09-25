package storage

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPHouseholdFoundationIsSourcedScopedAndPrivate(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "household.db")
	s := openM2AgentStore(t, ctx, path, false)
	if _, err := s.BootstrapRPPlayDemo(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO principals(principal_id,principal_type,display_name,status) VALUES ('principal_f4_operator','operator','F4 local operator','active')`); err != nil {
		t.Fatal(err)
	}
	binding := func(key string) core.CareerBinding {
		var head int64
		if err := s.db.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&head); err != nil {
			t.Fatal(err)
		}
		return core.CareerBinding{PrincipalID: "principal_f4_operator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: head, IdempotencyKey: key}
	}
	r := RPHouseholdFoundRequest{Binding: binding("found-ada-bo"), HouseholdKey: "ada-bo-home", DisplayName: "Ada 与 Bo 的家", ResidencePlaceID: M2AgentCafeID, AdultEntityIDs: [2]string{M2AgentAdaID, M2AgentBoID}}
	s.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "household pre-commit failure") }
	if _, err := s.FoundRPHouseholdLocal(ctx, r); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatal("injected failure", err)
	}
	s.beforeCommit = nil
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_households WHERE instance_id=? AND branch_id=?`, []any{M2DemoInstanceID, M2DemoBranchID}, 0)
	first, err := s.FoundRPHouseholdLocal(ctx, r)
	if err != nil || first.Replayed || first.Fact.HouseholdID == "" || first.Fact.RentAccountID == "" {
		t.Fatal("founded household", first, err)
	}
	if again, err := s.FoundRPHouseholdLocal(ctx, r); err != nil || !again.Replayed || again.EventID != first.EventID {
		t.Fatal("exact replay", again, err)
	}
	changed := r
	changed.DisplayName = "different household"
	if _, err := s.FoundRPHouseholdLocal(ctx, changed); !core.HasCode(err, core.CodeIdempotencyMismatch) {
		t.Fatal("changed exact key", err)
	}
	other := r
	other.Binding = binding("another-household")
	other.HouseholdKey = "another-home"
	if _, err := s.FoundRPHouseholdLocal(ctx, other); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatal("adult enrolled twice", err)
	}
	unauthorized := other
	unauthorized.Binding.PrincipalID = M2RPPlayerPrincipal
	unauthorized.Binding.IdempotencyKey = "player-create-household"
	if _, err := s.FoundRPHouseholdLocal(ctx, unauthorized); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatal("non-operator foundation", err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_household_memberships WHERE household_id=? AND ended_event_id IS NULL`, []any{first.Fact.HouseholdID}, 2)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_profiles WHERE agent_id IN (?,?)`, []any{M2AgentAdaID, M2AgentBoID}, 2)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM accounts WHERE account_id=? AND owner_id=? AND account_type='household_rent_cash'`, []any{first.Fact.RentAccountID, first.Fact.HouseholdID}, 1)
	assertM2Value(t, ctx, s, `SELECT balance_minor FROM account_balances WHERE account_id=?`, []any{first.Fact.RentAccountID}, 0)
	grantRPControlForTest(t, ctx, s, M2AgentAdaID)
	ada, err := s.OpenRPSession(ctx, rpTestOpenRequest())
	if err != nil {
		t.Fatal(err)
	}
	view, err := s.ReadRPHousehold(ctx, core.RPSessionReadRequest{PrincipalID: rpTestPrincipal, SessionID: ada.SessionID}, first.Fact.HouseholdID)
	if err != nil || view.MemberCount != 2 || view.OwnRole != "adult" || view.RentFundMinor != 0 || view.DisplayName != r.DisplayName {
		t.Fatal("private member view", view, err)
	}
	lin, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: M2RPPlayerPrincipal, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2RPPlayerID, POV: "second_person", IdempotencyKey: "f4-nonmember-session"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReadRPHousehold(ctx, core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: lin.SessionID}, first.Fact.HouseholdID); !core.HasCode(err, core.CodeNotFound) {
		t.Fatal("nonmember read", err)
	}
	if _, err := s.ReadRPHousehold(ctx, core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: ada.SessionID}, first.Fact.HouseholdID); !core.HasCode(err, core.CodeNotFound) {
		t.Fatal("foreign session read", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if after, err := s.ReadRPHousehold(ctx, core.RPSessionReadRequest{PrincipalID: rpTestPrincipal, SessionID: ada.SessionID}, first.Fact.HouseholdID); err != nil || after != view {
		t.Fatal("member view after reopen", after, err)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("household projection", diff, err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE rp_household_memberships SET member_role='dependent' WHERE household_id=? AND member_entity_id=?`, first.Fact.HouseholdID, M2AgentBoID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE account_balances SET balance_minor=42 WHERE account_id=?`, first.Fact.RentAccountID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE accounts SET overdraft_limit_minor=10,overdraft_policy_id='rogue' WHERE account_id=?`, first.Fact.RentAccountID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE economic_entities SET display_name='wrong' WHERE entity_id=?`, first.Fact.HouseholdID); err != nil {
		t.Fatal(err)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 1 || diff[0].Projection != "rp_household" {
		t.Fatal("household corruption not detected", diff, err)
	}
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal("rebuild household", err)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("household repair", diff, err)
	}
}

func TestRPHouseholdRentAgreementIsSourcedAndScoped(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "rent-agreement.db")
	s := openM2AgentStore(t, ctx, path, false)
	defer func() { s.Close() }()
	if _, err := s.BootstrapRPPlayDemo(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO principals(principal_id,principal_type,display_name,status) VALUES ('principal_f4_rent_operator','operator','F4 rent operator','active')`); err != nil {
		t.Fatal(err)
	}
	binding := func(key string) core.CareerBinding {
		var head int64
		if err := s.db.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&head); err != nil {
			t.Fatal(err)
		}
		return core.CareerBinding{PrincipalID: "principal_f4_rent_operator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: head, IdempotencyKey: key}
	}
	home, err := s.FoundRPHouseholdLocal(ctx, RPHouseholdFoundRequest{Binding: binding("found-rent-home"), HouseholdKey: "rent-home", DisplayName: "Shared home", ResidencePlaceID: M2AgentCafeID, AdultEntityIDs: [2]string{M2AgentAdaID, M2AgentBoID}})
	if err != nil {
		t.Fatal(err)
	}
	r := RPHouseholdRentAgreementRequest{Binding: binding("rent-agreement"), HouseholdID: home.Fact.HouseholdID, AgreementKey: "first-rent", LandlordName: "Local landlord", RentMinor: 300, PeriodDays: 30, GraceDays: 3, Shares: [2]RPHouseholdRentShare{{M2AgentBoID, 120}, {M2AgentAdaID, 180}}}
	s.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "rent agreement rollback") }
	if _, err := s.AgreeRPHouseholdRentLocal(ctx, r); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatal("injected rollback", err)
	}
	s.beforeCommit = nil
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_household_rent_agreements`, nil, 0)
	first, err := s.AgreeRPHouseholdRentLocal(ctx, r)
	if err != nil || first.Fact.AgreementID == "" || first.Fact.Shares[0].MemberEntityID != M2AgentAdaID {
		t.Fatal("agreement", first, err)
	}
	if retry, err := s.AgreeRPHouseholdRentLocal(ctx, r); err != nil || !retry.Replayed || retry.EventID != first.EventID {
		t.Fatal("agreement retry", retry, err)
	}
	changed := r
	changed.LandlordName = "Different landlord"
	if _, err := s.AgreeRPHouseholdRentLocal(ctx, changed); !core.HasCode(err, core.CodeIdempotencyMismatch) {
		t.Fatal("mismatched retry", err)
	}
	other := r
	other.Binding = binding("second-rent-agreement")
	other.AgreementKey = "second-rent"
	if _, err := s.AgreeRPHouseholdRentLocal(ctx, other); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatal("duplicate active rent", err)
	}
	unauthorized := other
	unauthorized.Binding.PrincipalID = M2RPPlayerPrincipal
	unauthorized.Binding.IdempotencyKey = "player-rent"
	if _, err := s.AgreeRPHouseholdRentLocal(ctx, unauthorized); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatal("nonoperator agreement", err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rent_contracts WHERE contract_id=? AND tenant_entity_id=? AND tenant_account_id=? AND landlord_entity_id=? AND rent_minor=300`, []any{first.Fact.ContractID, home.Fact.HouseholdID, home.Fact.RentAccountID, first.Fact.LandlordEntityID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_household_rent_shares WHERE agreement_id=? AND source_event_id=?`, []any{first.Fact.AgreementID, first.EventID}, 2)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM obligation_ledger_accounts WHERE obligation_kind='rent' AND contract_id=? AND definition_event_id=?`, []any{first.Fact.ContractID, first.EventID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM accounts WHERE opened_by_event_id=?`, []any{first.EventID}, 5)
	read := allowFixtureControl(t, ctx, s, M2AgentAdaID)
	view, err := s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	contribute := RPHouseholdRentContributionRequest{
		Binding:   core.CareerBinding{PrincipalID: read.PrincipalID, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: view.ObservationCursor, IdempotencyKey: "ada-rent-100"},
		SessionID: read.SessionID, AgreementID: first.Fact.AgreementID, PeriodIndex: 0, AmountMinor: 100,
	}
	s.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "rent contribution rollback") }
	if _, err := s.ContributeRPHouseholdRent(ctx, contribute); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatal("contribution rollback", err)
	}
	s.beforeCommit = nil
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_household_rent_contributions`, nil, 0)
	paid, err := s.ContributeRPHouseholdRent(ctx, contribute)
	if err != nil || paid.Fact.MemberEntityID != M2AgentAdaID || paid.Fact.AmountMinor != 100 {
		t.Fatal("own contribution", paid, err)
	}
	if retry, err := s.ContributeRPHouseholdRent(ctx, contribute); err != nil || !retry.Replayed || retry.EventID != paid.EventID {
		t.Fatal("contribution replay", retry, err)
	}
	changedContribution := contribute
	changedContribution.AmountMinor = 101
	if _, err := s.ContributeRPHouseholdRent(ctx, changedContribution); !core.HasCode(err, core.CodeIdempotencyMismatch) {
		t.Fatal("changed contribution replay", err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM journal_entries WHERE event_id=? AND status='posted'`, []any{paid.EventID}, 1)
	assertM2Value(t, ctx, s, `SELECT COALESCE(SUM(amount_minor),0) FROM postings WHERE entry_id=?`, []any{"journal_" + paid.EventID}, 0)
	assertM2Value(t, ctx, s, `SELECT balance_minor FROM account_balances WHERE account_id=?`, []any{home.Fact.RentAccountID}, 100)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_household_rent_contributions WHERE agreement_id=? AND membership_id=? AND period_index=0 AND amount_minor=100`, []any{first.Fact.AgreementID, paid.Fact.MembershipID}, 1)
	householdView, err := s.ReadRPHousehold(ctx, read, home.Fact.HouseholdID)
	if err != nil || householdView.Rent == nil || householdView.Rent.OwnShareMinor != 180 || householdView.Rent.OwnContributedMinor != 100 || householdView.Rent.OwnRemainingMinor != 80 || householdView.RentFundMinor != 100 {
		t.Fatal("bounded own rent view", householdView, err)
	}
	viewJSON, err := json.Marshal(householdView)
	if err != nil || strings.Contains(string(viewJSON), M2AgentBoID) || strings.Contains(string(viewJSON), paid.Fact.SourceAccount) {
		t.Fatal("rent view exposed other member or private account", string(viewJSON), err)
	}
	view, err = s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	contribute.Binding.ExpectedHead = view.ObservationCursor
	contribute.Binding.IdempotencyKey = "ada-over-share"
	contribute.AmountMinor = 81
	if _, err := s.ContributeRPHouseholdRent(ctx, contribute); !core.HasCode(err, core.CodeInvalidArgument) {
		t.Fatal("over-share contribution", err)
	}
	outsider, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: M2RPPlayerPrincipal, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2RPPlayerID, POV: "second_person", IdempotencyKey: "f4-rent-outsider"})
	if err != nil {
		t.Fatal(err)
	}
	outsiderView, err := s.ObserveRPSession(ctx, core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: outsider.SessionID})
	if err != nil {
		t.Fatal(err)
	}
	contribute.SessionID = outsider.SessionID
	contribute.Binding.ExpectedHead = outsiderView.ObservationCursor
	contribute.Binding.IdempotencyKey = "outsider-rent"
	contribute.AmountMinor = 1
	if _, err := s.ContributeRPHouseholdRent(ctx, contribute); !core.HasCode(err, core.CodeNotFound) {
		t.Fatal("outsider contribution", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("rent projection after reopen", diff, err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE rp_household_rent_shares SET amount_minor=1 WHERE agreement_id=? AND membership_id=?`, first.Fact.AgreementID, paid.Fact.MembershipID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE rp_household_rent_contributions SET amount_minor=1 WHERE contribution_id=?`, paid.Fact.ContributionID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE economic_entities SET display_name='corrupt landlord' WHERE entity_id=?`, first.Fact.LandlordEntityID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE rent_contracts SET tenant_entity_id=? WHERE contract_id=?`, first.Fact.LandlordEntityID, first.Fact.ContractID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE obligation_ledger_accounts SET expense_account_id=? WHERE obligation_kind='rent' AND contract_id=?`, first.Fact.LandlordAccountID, first.Fact.ContractID); err != nil {
		t.Fatal(err)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 1 || diff[0].Projection != "rp_household_rent" {
		t.Fatal("rent corruption not detected", diff, err)
	}
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal("rebuild rent", err)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("rent repair", diff, err)
	}
}
