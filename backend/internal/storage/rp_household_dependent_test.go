package storage

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPHouseholdDependentSupportIsSourcedAndBounded(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "household-dependent.db")
	s := openM2AgentStore(t, ctx, path, false)
	defer func() { s.Close() }()
	if _, err := s.BootstrapRPPlayDemo(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO principals(principal_id,principal_type,display_name,status) VALUES ('principal_f4_dependent_operator','operator','F4 dependent operator','active')`); err != nil {
		t.Fatal(err)
	}
	operator := func(key string) core.CareerBinding {
		var head int64
		if err := s.db.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&head); err != nil {
			t.Fatal(err)
		}
		return core.CareerBinding{PrincipalID: "principal_f4_dependent_operator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: head, IdempotencyKey: key}
	}
	home, err := s.FoundRPHouseholdLocal(ctx, RPHouseholdFoundRequest{Binding: operator("dependent-home"), HouseholdKey: "dependent-home", DisplayName: "Care home", ResidencePlaceID: M2AgentCafeID, AdultEntityIDs: [2]string{M2AgentAdaID, M2AgentBoID}})
	if err != nil {
		t.Fatal(err)
	}
	request := RPHouseholdDependentRequest{Binding: operator("enroll-cai"), HouseholdID: home.Fact.HouseholdID, DependentEntityID: M2RPNPCID, SupporterEntityID: M2AgentAdaID, Reason: "Household care arrangement"}
	unauthorized := request
	unauthorized.Binding.PrincipalID = M2AgentBoPrincipal
	unauthorized.Binding.IdempotencyKey = "nonoperator-enroll"
	if _, err := s.EnrollRPHouseholdDependentLocal(ctx, unauthorized); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatal("nonoperator enrolled dependent", err)
	}
	s.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "dependent enrollment rollback") }
	if _, err := s.EnrollRPHouseholdDependentLocal(ctx, request); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatal("enrollment rollback", err)
	}
	s.beforeCommit = nil
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_household_memberships WHERE member_entity_id=?`, []any{M2RPNPCID}, 0)
	enrolled, err := s.EnrollRPHouseholdDependentLocal(ctx, request)
	if err != nil || enrolled.Fact.DependentEntityID != M2RPNPCID || enrolled.Fact.Reason != request.Reason {
		t.Fatal("enroll dependent", enrolled, err)
	}
	if retry, err := s.EnrollRPHouseholdDependentLocal(ctx, request); err != nil || !retry.Replayed || retry.EventID != enrolled.EventID {
		t.Fatal("dependent enrollment replay", retry, err)
	}
	changed := request
	changed.Reason = "Changed relationship"
	if _, err := s.EnrollRPHouseholdDependentLocal(ctx, changed); !core.HasCode(err, core.CodeIdempotencyMismatch) {
		t.Fatal("changed enrollment retry", err)
	}
	duplicate := request
	duplicate.Binding = operator("duplicate-cai")
	if _, err := s.EnrollRPHouseholdDependentLocal(ctx, duplicate); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatal("duplicate active dependent", err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_household_memberships WHERE household_id=? AND member_entity_id=? AND member_role='dependent' AND supporter_membership_id=? AND source_event_id=? AND ended_event_id IS NULL`, []any{home.Fact.HouseholdID, M2RPNPCID, enrolled.Fact.SupporterMembershipID, enrolled.EventID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM materialized_entities WHERE entity_id=? AND status='active'`, []any{M2RPNPCID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM outbox WHERE event_id=?`, []any{enrolled.EventID}, 0)
	ada := allowFixtureControl(t, ctx, s, M2AgentAdaID)
	adaView, err := s.ReadRPHousehold(ctx, ada, home.Fact.HouseholdID)
	if err != nil || adaView.OwnRole != "adult" || adaView.MemberCount != 3 {
		t.Fatal("adult household view", adaView, err)
	}
	viewJSON, err := json.Marshal(adaView)
	if err != nil || strings.Contains(string(viewJSON), M2RPNPCID) || strings.Contains(string(viewJSON), M2AgentBoID) || strings.Contains(string(viewJSON), enrolled.EventID) {
		t.Fatal("other member identity escaped bounded view", string(viewJSON), err)
	}
	cai := allowFixtureControl(t, ctx, s, M2RPNPCID)
	caiView, err := s.ReadRPHousehold(ctx, cai, home.Fact.HouseholdID)
	if err != nil || caiView.OwnRole != "dependent" || caiView.MemberCount != 3 || caiView.Rent != nil {
		t.Fatal("dependent household view", caiView, err)
	}
	observed, err := s.ObserveRPSession(ctx, ada)
	if err != nil {
		t.Fatal(err)
	}
	exit := RPHouseholdMemberExitRequest{Binding: core.CareerBinding{PrincipalID: ada.PrincipalID, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: observed.ObservationCursor, IdempotencyKey: "supporter-exit"}, SessionID: ada.SessionID, HouseholdID: home.Fact.HouseholdID, Reason: "Attempt to leave while supporting Cai"}
	if _, err := s.LeaveRPHousehold(ctx, exit); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatal("supporter abandoned active dependent", err)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("dependent source comparison", diff, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE rp_household_memberships SET supporter_membership_id=? WHERE membership_id=?`, home.Fact.HouseholdID+"_adult_1", enrolled.Fact.MembershipID); err != nil {
		t.Fatal(err)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) == 0 {
		t.Fatal("changed supporter not detected", diff, err)
	}
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal("rebuild dependent support", err)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("dependent support repair", diff, err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_household_memberships WHERE membership_id=? AND supporter_membership_id=?`, []any{enrolled.Fact.MembershipID, enrolled.Fact.SupporterMembershipID}, 1)
}
