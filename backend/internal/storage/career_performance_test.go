package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"corerp.local/backend/internal/core"
)

func prepareCareerPerformance(t *testing.T, s *Store) (CareerRecord, core.CareerPerformanceRequest) {
	t.Helper()
	ctx := context.Background()
	offer := prepareCareerEmploymentOffer(t, s)
	accepted, err := s.AcceptCareerOffer(ctx, core.CareerOfferAcceptRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "accept"), OfferID: offer.Fact.RecordID, AfterWorkPlaceID: M2AgentCafeID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.RunAgentLife(ctx, careerTime(2, 0, 1), 100); err != nil {
		t.Fatal(err)
	}
	attendance, err := s.ReadCareerAttendance(ctx, M2AgentBoPrincipal, M2DemoInstanceID, M2DemoBranchID, accepted.Fact.Employment.ContractID, 1)
	if err != nil {
		t.Fatal(err)
	}
	return accepted, core.CareerPerformanceRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "performance"), ReviewID: "review_ada", ContractID: accepted.Fact.Employment.ContractID, EvidenceEventIDs: []string{attendance.EventID}, Assessment: "meets_expectations", Reason: "Private manager judgment based on completed work", AdvisoryNote: "Private advisory recommendation; not an authority"}
}

func TestCareerPerformanceRegularizationEvidencePrivacyAndRecovery(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "regularization.db")
	s := openCareerTestWorld(t, path)
	defer func() { s.Close() }()
	accepted, performance := prepareCareerPerformance(t, s)
	job := accepted.Fact.Employment
	initial, err := s.ReadCareerRecruitmentRecord(ctx, M2AgentAdaPrincipal, M2DemoInstanceID, M2DemoBranchID, "employment", job.ContractID)
	if err != nil || initial.Fact.Kind != "employment" || initial.Fact.Offer != nil || initial.Fact.Application != nil || initial.EventID != accepted.EventID {
		t.Fatalf("initial employment view: %+v %v", initial, err)
	}
	bad := performance
	bad.Binding.PrincipalID = M2AgentAdaPrincipal
	if _, err := s.RecordCareerPerformance(ctx, bad); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("self-evaluation accepted: %v", err)
	}
	bad = performance
	bad.EvidenceEventIDs = []string{accepted.EventID}
	if _, err := s.RecordCareerPerformance(ctx, bad); !core.HasCode(err, core.CodeInvalidArgument) {
		t.Fatalf("hiring accepted as performance evidence: %v", err)
	}
	review, err := s.RecordCareerPerformance(ctx, performance)
	if err != nil {
		t.Fatal(err)
	}
	r := core.CareerRegularizationRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "regularize"), ReviewID: performance.ReviewID, Notice: "Your probation is complete; your existing pay and position continue."}
	if _, err := s.RegularizeCareerEmployment(ctx, r); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatalf("early regularization accepted: %v", err)
	}
	if _, err := s.RunAgentLife(ctx, careerTime(8, 0, 1), 1000); err != nil {
		t.Fatal(err)
	}
	before := readCareerTestContext(t, s, job.EmployeeID)
	if len(before.Life.Employment) != 1 || before.Life.Employment[0].Status != "probation" {
		t.Fatal("elapsed time automatically regularized employment")
	}
	r.Binding = careerTestBinding(t, s, M2AgentBoPrincipal, "regularize")
	unauthorized := r
	unauthorized.Binding.PrincipalID = M2AgentAdaPrincipal
	if _, err := s.RegularizeCareerEmployment(ctx, unauthorized); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("candidate regularized self: %v", err)
	}
	s.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "interrupt regularization") }
	if _, err := s.RegularizeCareerEmployment(ctx, r); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatalf("regularization interruption: %v", err)
	}
	s.beforeCommit = nil
	if got := readCareerTestContext(t, s, job.EmployeeID); got.Life.Employment[0].Status != "probation" {
		t.Fatal("rollback changed employment")
	}
	regularized, err := s.RegularizeCareerEmployment(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	if regularized.Fact.Employment.TermVersion != 2 || regularized.Fact.Employment.EffectiveFromDay != 8 || regularized.Fact.EmploymentChange.PerformanceEventID != review.EventID {
		t.Fatalf("regularization source/version: %+v", regularized)
	}
	after := readCareerTestContext(t, s, job.EmployeeID)
	if after.Life.Employment[0].Status != "regular" || after.Life.Employment[0].SourceEventID != regularized.EventID || after.OwnAssetMinor != before.OwnAssetMinor || after.Life.Employment[0].WageMinor != 12 {
		t.Fatalf("regularization rewrote pay/identity or lacks own status: %+v", after)
	}
	found := false
	for _, memory := range after.Life.SalientMemories {
		if memory.Kind == "own_employment_regularized" && memory.SourceEventID == regularized.EventID && memory.Text == r.Notice {
			found = true
		}
	}
	if !found {
		t.Fatal("addressed notice missing from work memory")
	}
	encoded, _ := json.Marshal(after)
	if strings.Contains(string(encoded), performance.Reason) || strings.Contains(string(encoded), performance.AdvisoryNote) {
		t.Fatal("private performance review leaked into employee context")
	}
	if _, err := s.ReadCareerRecruitmentRecord(ctx, M2AgentAdaPrincipal, M2DemoInstanceID, M2DemoBranchID, "performance", performance.ReviewID); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("employee read private performance: %v", err)
	}
	if _, err := s.ReadCareerRecruitmentRecord(ctx, M2AgentBoPrincipal, M2DemoInstanceID, M2DemoBranchID, "performance", performance.ReviewID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReadCareerRecruitmentRecord(ctx, M2RPPlayerPrincipal, M2DemoInstanceID, M2DemoBranchID, "employment", job.ContractID); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("unrelated reader: %v", err)
	}
	duplicate := r
	duplicate.Binding = careerTestBinding(t, s, M2AgentBoPrincipal, "regularize-again")
	if _, err := s.RegularizeCareerEmployment(ctx, duplicate); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatalf("duplicate transition accepted: %v", err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM employment_contracts WHERE employee_entity_id=?`, []any{job.EmployeeID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM outbox WHERE event_id IN (?,?)`, []any{review.EventID, regularized.EventID}, 0)
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
	if got, err := s.RegularizeCareerEmployment(ctx, r); err != nil || !got.Replayed || got.EventID != regularized.EventID {
		t.Fatalf("regularization recovery: %+v %v", got, err)
	}
	if got, err := s.RecordCareerPerformance(ctx, performance); err != nil || !got.Replayed || got.EventID != review.EventID {
		t.Fatalf("performance recovery: %+v %v", got, err)
	}
	if got := readCareerTestContext(t, s, job.EmployeeID); !reflect.DeepEqual(got, after) {
		t.Fatal("rebuild/reopen changed own employment context")
	}
	if _, err := s.RunAgentLife(ctx, careerTime(9, 0, 1), 100); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT amount_due_minor FROM wage_obligations WHERE contract_id=? AND period_start_day=8`, []any{job.ContractID}, 12)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='WageObligationAccrued' AND json_extract(payload,'$.contract_id')=? AND json_extract(payload,'$.terms_event_id')=?`, []any{job.ContractID, regularized.EventID}, 1)
	if differences, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatalf("regularization payroll replay: %+v %v", differences, err)
	}
}

func TestCareerPerformanceSupersessionFutureTermsAndRevocation(t *testing.T) {
	ctx := context.Background()
	s := openCareerTestWorld(t, filepath.Join(t.TempDir(), "review-guards.db"))
	defer s.Close()
	accepted, r := prepareCareerPerformance(t, s)
	if _, err := s.RecordCareerPerformance(ctx, r); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RunAgentLife(ctx, careerTime(8, 0, 1), 1000); err != nil {
		t.Fatal(err)
	}
	badReview := r
	badReview.Binding = careerTestBinding(t, s, M2AgentBoPrincipal, "review-unfavorable")
	badReview.ReviewID, badReview.Assessment = "unfavorable", "needs_improvement"
	if _, err := s.RecordCareerPerformance(ctx, badReview); err != nil {
		t.Fatal(err)
	}
	for _, review := range []string{r.ReviewID, badReview.ReviewID} {
		request := core.CareerRegularizationRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "try-"+review), ReviewID: review, Notice: "Ready"}
		if _, err := s.RegularizeCareerEmployment(ctx, request); !core.HasCode(err, core.CodeBranchConflict) {
			t.Fatalf("superseded/unfavorable review accepted: %v", err)
		}
	}
	good := r
	good.Binding = careerTestBinding(t, s, M2AgentBoPrincipal, "new-review")
	good.ReviewID = "latest-review"
	if _, err := s.RecordCareerPerformance(ctx, good); err != nil {
		t.Fatal(err)
	}
	// Explicit test-only future term source verifies that a lifecycle command
	// does not silently supersede an already planned change. No raise API claim.
	job := *accepted.Fact.Employment
	job.EffectiveFromDay, job.TermVersion, job.DailyWageMinor = 9, 2, 20
	b := careerTestBinding(t, s, M2AgentBoPrincipal, "test-pending-terms")
	if _, err := s.executeCareerCommand(ctx, b, "TestCareerFutureTerms", job, func(conn *sql.Conn) error { return authorizeCareerManager(ctx, conn, b, job.OrganizationID) }, func(_ *sql.Conn, _ careerCommandContext) (CareerFact, func() error, error) {
		return CareerFact{Kind: "employment", RecordID: job.ContractID, OrganizationID: job.OrganizationID, CandidateID: job.EmployeeID, Employment: &job}, nil, nil
	}); err != nil {
		t.Fatal(err)
	}
	request := core.CareerRegularizationRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "pending-regularization"), ReviewID: good.ReviewID, Notice: "Ready"}
	if _, err := s.RegularizeCareerEmployment(ctx, request); !core.HasCode(err, core.CodeBranchConflict) || !strings.Contains(err.Error(), "pending effective terms") {
		t.Fatalf("pending terms were overwritten: %v", err)
	}
	if _, err := s.RunAgentLife(ctx, careerTime(9, 0, 1), 100); err != nil {
		t.Fatal(err)
	}
	request.Binding = careerTestBinding(t, s, M2AgentBoPrincipal, "stale-terms-regularization")
	if _, err := s.RegularizeCareerEmployment(ctx, request); !core.HasCode(err, core.CodeBranchConflict) || !strings.Contains(err.Error(), "superseded employment terms") {
		t.Fatalf("old review applied to changed employment: %v", err)
	}
	// Revocation fixture tests active-grant enforcement, including exact retry.
	if _, err := s.db.Exec(`UPDATE capability_grants SET status='revoked' WHERE principal_id=? AND capability_id=? AND subject_id=?`, M2AgentBoPrincipal, careerManageCapability, job.OrganizationID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RecordCareerPerformance(ctx, good); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("revoked manager retrieved private review retry: %v", err)
	}
	if _, err := s.ReadCareerRecruitmentRecord(ctx, M2AgentBoPrincipal, M2DemoInstanceID, M2DemoBranchID, "performance", good.ReviewID); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("revoked manager read private review: %v", err)
	}
	if _, err := s.RegularizeCareerEmployment(ctx, request); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("revoked manager regularized worker: %v", err)
	}
}
