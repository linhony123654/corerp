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

func careerTestBinding(t *testing.T, s *Store, principal, key string) core.CareerBinding {
	t.Helper()
	b := core.CareerBinding{PrincipalID: principal, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, IdempotencyKey: key}
	if err := s.db.QueryRow(`SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, b.InstanceID, b.BranchID).Scan(&b.ExpectedHead); err != nil {
		t.Fatal(err)
	}
	return b
}

func careerTestOrg(t *testing.T, s *Store) core.CareerOrganizationRequest {
	t.Helper()
	return core.CareerOrganizationRequest{Binding: careerTestBinding(t, s, "principal_creator", "career-org"), Organization: core.CareerOrganizationDefinition{
		OrganizationID: "actor_m2_coop_employer", DisplayName: "街区合作社", ManagerPrincipalID: M2AgentBoPrincipal, WorkplaceID: "place_m2_work_ada",
	}}
}

func careerTestPosting(t *testing.T, s *Store) core.CareerPostingRequest {
	t.Helper()
	return core.CareerPostingRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "career-post"), Posting: core.CareerPostingDefinition{
		PositionID: "position_coop_assistant", OrganizationID: "actor_m2_coop_employer", Title: "运营助理", OccupationID: "operations", Grade: "junior", Capacity: 1, DailyWageMinor: 12, RequiredQualifications: []string{"safety_training"},
	}}
}

func openCareerTestWorld(t *testing.T, path string) *Store {
	t.Helper()
	s, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.PrepareRPLifeDemo(context.Background()); err != nil {
		s.Close()
		t.Fatal(err)
	}
	return s
}

func TestCareerOrganizationPostingApplicationRecovery(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "career.db")
	s := openCareerTestWorld(t, path)
	defer func() { s.Close() }()
	orgRequest := careerTestOrg(t, s)
	org, err := s.DefineCareerOrganization(ctx, orgRequest)
	if err != nil {
		t.Fatal(err)
	}
	if org.Fact.Organization.CashAccountID != m2EconomyEmployerCash {
		t.Fatalf("duplicate economic organization: %+v", org)
	}
	assertM2Value(t, ctx, s, `SELECT balance_minor FROM account_balances WHERE account_id=?`, []any{m2EconomyEmployerCash}, 1200)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM capability_grants WHERE capability_id=? AND principal_id=? AND subject_id=? AND definition_event_id=?`, []any{careerManageCapability, M2AgentBoPrincipal, "actor_m2_coop_employer", org.EventID}, 1)
	postingRequest := careerTestPosting(t, s)
	posting, err := s.PostCareerPosition(ctx, postingRequest)
	if err != nil {
		t.Fatal(err)
	}
	market, err := s.DiscoverCareerPositions(ctx, M2AgentAdaPrincipal, M2DemoInstanceID, M2DemoBranchID, M2AgentAdaID, 0)
	if err != nil || len(market.Postings) != 1 || market.Postings[0].SourceEventID != posting.EventID {
		t.Fatalf("market: %+v %v", market, err)
	}
	if !reflect.DeepEqual(market.Postings[0].Posting, postingRequest.Posting) {
		t.Fatal("published occupation/grade/terms differ")
	}
	empty, err := s.DiscoverCareerPositions(ctx, M2AgentAdaPrincipal, M2DemoInstanceID, M2DemoBranchID, M2AgentAdaID, market.NextCursor)
	if err != nil || len(empty.Postings) != 0 {
		t.Fatalf("market pagination: %+v %v", empty, err)
	}
	application := core.CareerApplicationRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "ada-apply"), ApplicationID: "ada_application", PositionID: postingRequest.Posting.PositionID, CandidateID: M2AgentAdaID, Statement: "我有经验，也声称已完成安全培训。"}
	applied, err := s.ApplyForCareerPosition(ctx, application)
	if err != nil {
		t.Fatal(err)
	}
	if applied.Fact.Application.PostingEventID != posting.EventID || applied.Fact.Application.CandidateSourceID == "" || applied.Fact.Application.Status != "submitted" {
		t.Fatalf("application lost lineage: %+v", applied)
	}
	for _, principal := range []string{M2AgentAdaPrincipal, M2AgentBoPrincipal} {
		read, err := s.ReadCareerApplication(ctx, principal, M2DemoInstanceID, M2DemoBranchID, application.ApplicationID)
		if err != nil || !reflect.DeepEqual(read, applied) {
			t.Fatalf("authorized read: %+v %v", read, err)
		}
	}
	if _, err := s.ReadCareerApplication(ctx, M2RPPlayerPrincipal, M2DemoInstanceID, M2DemoBranchID, application.ApplicationID); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("application leaked to another candidate: %v", err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM outbox o JOIN events e ON e.event_id=o.event_id WHERE e.event_type='RPCareerFactRecorded'`, nil, 0)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM employment_contracts WHERE employee_entity_id=?`, []any{M2AgentAdaID}, 0)
	public, err := s.DiscoverCareerPositions(ctx, M2RPPlayerPrincipal, M2DemoInstanceID, M2DemoBranchID, M2RPPlayerID, 0)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(public)
	if strings.Contains(string(encoded), application.Statement) || strings.Contains(string(encoded), "cash_account_id") {
		t.Fatal("market leaked private application/account")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	for _, retry := range []func() (CareerRecord, error){
		func() (CareerRecord, error) { return s.DefineCareerOrganization(ctx, orgRequest) },
		func() (CareerRecord, error) { return s.PostCareerPosition(ctx, postingRequest) },
		func() (CareerRecord, error) { return s.ApplyForCareerPosition(ctx, application) },
	} {
		got, err := retry()
		if err != nil || !got.Replayed {
			t.Fatalf("career recovery: %+v %v", got, err)
		}
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='RPCareerFactRecorded'`, nil, 3)
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	if differences, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatalf("career replay: %+v %v", differences, err)
	}
	read, err := s.ReadCareerApplication(ctx, M2AgentAdaPrincipal, M2DemoInstanceID, M2DemoBranchID, application.ApplicationID)
	if err != nil || !reflect.DeepEqual(read, applied) {
		t.Fatalf("application changed after rebuild: %+v %v", read, err)
	}
}

func TestCareerAuthorizationFreshnessAndRollback(t *testing.T) {
	ctx := context.Background()
	s := openCareerTestWorld(t, filepath.Join(t.TempDir(), "authority.db"))
	defer s.Close()
	org := careerTestOrg(t, s)
	bad := org
	bad.Binding.PrincipalID = M2RPPlayerPrincipal
	if _, err := s.DefineCareerOrganization(ctx, bad); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("player defined organization: %v", err)
	}
	bad = org
	bad.Binding.BranchID = "another_branch"
	if _, err := s.DefineCareerOrganization(ctx, bad); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("cross-branch definition: %v", err)
	}
	s.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "career transaction rollback") }
	if _, err := s.DefineCareerOrganization(ctx, org); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatalf("rollback injection: %v", err)
	}
	s.beforeCommit = nil
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM capability_grants WHERE capability_id=?`, []any{careerManageCapability}, 0)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM economic_entities WHERE entity_id=?`, []any{org.Organization.OrganizationID}, 0)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='RPCareerFactRecorded'`, nil, 0)
	if _, err := s.DefineCareerOrganization(ctx, org); err != nil {
		t.Fatal(err)
	}
	posting := careerTestPosting(t, s)
	badPost := posting
	badPost.Binding.PrincipalID = "principal_creator"
	if _, err := s.PostCareerPosition(ctx, badPost); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("creator bypassed operational organization grant: %v", err)
	}
	if _, err := s.PostCareerPosition(ctx, posting); err != nil {
		t.Fatal(err)
	}
	badPost = posting
	badPost.Binding.IdempotencyKey = "stale-new-position"
	badPost.Posting.PositionID = "other_position"
	if _, err := s.PostCareerPosition(ctx, badPost); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatalf("stale posting accepted: %v", err)
	}
	application := core.CareerApplicationRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "forged-apply"), ApplicationID: "forged", CandidateID: M2AgentAdaID, PositionID: posting.Posting.PositionID, Statement: "I accept for Ada"}
	if _, err := s.ApplyForCareerPosition(ctx, application); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("manager impersonated candidate: %v", err)
	}
	application.Binding = careerTestBinding(t, s, M2AgentAdaPrincipal, "apply")
	if _, err := s.ApplyForCareerPosition(ctx, application); err != nil {
		t.Fatal(err)
	}
	changed := application
	changed.Statement = "changed under same key"
	if _, err := s.ApplyForCareerPosition(ctx, changed); !core.HasCode(err, core.CodeIdempotencyMismatch) {
		t.Fatalf("changed application replay: %v", err)
	}
	changed = application
	changed.Binding = careerTestBinding(t, s, M2AgentAdaPrincipal, "duplicate")
	changed.ApplicationID = "duplicate_application"
	if _, err := s.ApplyForCareerPosition(ctx, changed); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatalf("duplicate application: %v", err)
	}
	if _, err := s.DiscoverCareerPositions(ctx, M2AgentBoPrincipal, M2DemoInstanceID, M2DemoBranchID, M2AgentAdaID, 0); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("manager read as another candidate: %v", err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE capability_grants SET status='revoked' WHERE capability_id=?`, careerManageCapability); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PostCareerPosition(ctx, posting); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("revoked manager retried command: %v", err)
	}
	if _, err := s.ReadCareerApplication(ctx, M2AgentBoPrincipal, M2DemoInstanceID, M2DemoBranchID, application.ApplicationID); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("revoked manager read application: %v", err)
	}
}

func TestCareerConcurrentRetryAndCompetingHead(t *testing.T) {
	ctx := context.Background()
	s := openCareerTestWorld(t, filepath.Join(t.TempDir(), "concurrent.db"))
	defer s.Close()
	if _, err := s.DefineCareerOrganization(ctx, careerTestOrg(t, s)); err != nil {
		t.Fatal(err)
	}
	posting := careerTestPosting(t, s)
	type result struct {
		record CareerRecord
		err    error
	}
	results := make(chan result, 2)
	for i := 0; i < 2; i++ {
		go func() {
			record, err := s.PostCareerPosition(ctx, posting)
			results <- result{record, err}
		}()
	}
	a, b := <-results, <-results
	if a.err != nil || b.err != nil || a.record.EventID != b.record.EventID || a.record.Replayed == b.record.Replayed {
		t.Fatalf("concurrent exact retry: %+v %+v", a, b)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='RPCareerFactRecorded'`, nil, 2)
	left := careerTestPosting(t, s)
	left.Binding.IdempotencyKey, left.Posting.PositionID = "left", "position_left"
	right := left
	right.Binding.IdempotencyKey, right.Posting.PositionID = "right", "position_right"
	for _, request := range []core.CareerPostingRequest{left, right} {
		go func(r core.CareerPostingRequest) {
			record, err := s.PostCareerPosition(ctx, r)
			results <- result{record, err}
		}(request)
	}
	a, b = <-results, <-results
	passed, conflicted := 0, 0
	for _, result := range []result{a, b} {
		if result.err == nil {
			passed++
		} else if core.HasCode(result.err, core.CodeBranchConflict) {
			conflicted++
		} else {
			t.Fatal(result.err)
		}
	}
	if passed != 1 || conflicted != 1 {
		t.Fatalf("competing heads: %+v %+v", a, b)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='RPCareerFactRecorded'`, nil, 3)
}

func TestCareerOrganizationRequiresRealWorkplaceAndSourceScope(t *testing.T) {
	ctx := context.Background()
	s := openCareerTestWorld(t, filepath.Join(t.TempDir(), "real-source.db"))
	defer s.Close()
	org := careerTestOrg(t, s)
	bad := org
	bad.Organization.WorkplaceID = M2AgentCafeID
	if _, err := s.DefineCareerOrganization(ctx, bad); !core.HasCode(err, core.CodeInvalidArgument) {
		t.Fatalf("cafe silently became workplace: %v", err)
	}
	bad = org
	bad.Organization.ManagerPrincipalID = "invented_manager"
	if _, err := s.DefineCareerOrganization(ctx, bad); !core.HasCode(err, core.CodeInvalidArgument) {
		t.Fatalf("manager was invented: %v", err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE capability_grants SET subject_id='unrelated_cohort' WHERE grant_id='grant_m2_creator_materialize'`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DefineCareerOrganization(ctx, org); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("unrelated Cohort grant defined organization: %v", err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='RPCareerFactRecorded'`, nil, 0)
}
