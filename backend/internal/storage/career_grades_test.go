package storage

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestCareerGradeScaleAuthorityCompatibilityAndRecovery(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "grades.db")
	s := openCareerTestWorld(t, path)
	defer func() { s.Close() }()
	org := careerTestOrg(t, s)
	if _, err := s.DefineCareerOrganization(ctx, org); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PostCareerPosition(ctx, careerTestPosting(t, s)); err != nil {
		t.Fatal(err)
	}
	r := core.CareerGradeScaleRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "grade-scale"), Scale: core.CareerGradeScale{OrganizationID: org.Organization.OrganizationID, Grades: []string{"junior", "senior", "director"}}}
	for _, principal := range []string{M2AgentAdaPrincipal, "principal_creator"} {
		bad := r
		bad.Binding.PrincipalID = principal
		if _, err := s.DefineCareerGradeScale(ctx, bad); !core.HasCode(err, core.CodeUnauthorized) {
			t.Fatalf("unscoped authority: %v", err)
		}
	}
	bad := r
	bad.Scale.Grades = []string{"senior", "director"}
	if _, err := s.DefineCareerGradeScale(ctx, bad); !core.HasCode(err, core.CodeInvalidArgument) {
		t.Fatalf("orphaned existing grade: %v", err)
	}
	s.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "grade rollback") }
	if _, err := s.DefineCareerGradeScale(ctx, r); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatalf("rollback: %v", err)
	}
	s.beforeCommit = nil
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='RPCareerFactRecorded' AND json_extract(payload,'$.kind')='grade_scale'`, nil, 0)
	defined, err := s.DefineCareerGradeScale(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(defined.Fact.GradeScale, &r.Scale) {
		t.Fatal("scale changed")
	}
	if _, err := s.ReadCareerRecruitmentRecord(ctx, M2AgentAdaPrincipal, r.Binding.InstanceID, r.Binding.BranchID, "grade_scale", r.Scale.OrganizationID); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("internal scale leaked: %v", err)
	}
	post := careerTestPosting(t, s)
	post.Binding.IdempotencyKey = "senior-position"
	post.Posting.PositionID = "senior-position"
	post.Posting.Grade = "unknown"
	if _, err := s.PostCareerPosition(ctx, post); !core.HasCode(err, core.CodeInvalidArgument) {
		t.Fatalf("unknown grade accepted: %v", err)
	}
	post.Posting.Grade = "senior"
	if _, err := s.PostCareerPosition(ctx, post); err != nil {
		t.Fatal(err)
	}
	bad = r
	bad.Binding = careerTestBinding(t, s, M2AgentBoPrincipal, "redefine-grades")
	bad.Scale.Grades = []string{"director", "senior", "junior"}
	if _, err := s.DefineCareerGradeScale(ctx, bad); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatalf("silent reorder: %v", err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM employment_contracts WHERE employee_entity_id=?`, []any{M2AgentAdaID}, 0)
	assertM2Value(t, ctx, s, `SELECT balance_minor FROM account_balances WHERE account_id=?`, []any{m2EconomyEmployerCash}, 1200)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM outbox o JOIN events e ON e.event_id=o.event_id WHERE e.event_type='RPCareerFactRecorded'`, nil, 0)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := s.DefineCareerGradeScale(ctx, r)
	if err != nil || !retry.Replayed || retry.EventID != defined.EventID {
		t.Fatalf("restart retry: %+v %v", retry, err)
	}
	read, err := s.ReadCareerRecruitmentRecord(ctx, M2AgentBoPrincipal, r.Binding.InstanceID, r.Binding.BranchID, "grade_scale", r.Scale.OrganizationID)
	if err != nil || !reflect.DeepEqual(read.Fact.GradeScale, &r.Scale) {
		t.Fatalf("restart scale: %+v %v", read, err)
	}
	bad = r
	bad.Scale.Grades = []string{"senior", "junior", "director"}
	if _, err := s.DefineCareerGradeScale(ctx, bad); !core.HasCode(err, core.CodeIdempotencyMismatch) {
		t.Fatalf("changed retry: %v", err)
	}
}
