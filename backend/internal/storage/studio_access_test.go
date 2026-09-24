package storage

import (
	"context"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestStudioAccessLocalAtomicRetryRevokeAndRepair(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "access.db")
	s := openBootstrappedStore(t, ctx, path)
	defer func() { s.Close() }()
	if err := s.BootstrapM2Demo(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := s.BootstrapM2AgentDemo(ctx); err != nil {
		t.Fatal(err)
	}
	var initialHead int64
	if err := s.db.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&initialHead); err != nil {
		t.Fatal(err)
	}
	r := StudioAccessRequest{Binding: core.CareerBinding{PrincipalID: "principal_operator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: 1, IdempotencyKey: "grant"}, TargetPrincipalID: "principal_creator", Status: "active"}
	r.Binding.ExpectedHead = initialHead
	original := r
	bad := r
	bad.Binding.PrincipalID = "principal_creator"
	if _, err := s.ConfigureStudioAccessLocal(ctx, bad); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatal("non operator setup", err)
	}
	bad = r
	bad.TargetPrincipalID = "principal_buyer"
	if _, err := s.ConfigureStudioAccessLocal(ctx, bad); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatal("player inspector grant", err)
	}
	s.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "access rollback") }
	if _, err := s.ConfigureStudioAccessLocal(ctx, r); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatal(err)
	}
	s.beforeCommit = nil
	var rollbackHead int64
	err := s.db.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&rollbackHead)
	if err != nil || rollbackHead != initialHead {
		t.Fatal("failed setup changed world", err)
	}
	var grants int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM capability_grants WHERE capability_id='world.inspector.read'`).Scan(&grants); err != nil || grants != 0 {
		t.Fatal("grant survived rollback", err)
	}
	granted, err := s.ConfigureStudioAccessLocal(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	var auditInstance, auditType string
	if err := s.db.QueryRowContext(ctx, `SELECT instance_id,record_type FROM audit_records WHERE related_event_id=?`, granted.EventID).Scan(&auditInstance, &auditType); err != nil || auditInstance != M2DemoInstanceID || auditType != "runtime_diagnostic" {
		t.Fatal("wrong audit attribution", err)
	}
	retry, err := s.ConfigureStudioAccessLocal(ctx, r)
	if err != nil || !retry.Replayed || retry.EventID != granted.EventID {
		t.Fatal("retry duplicated setup", err)
	}
	bad = r
	bad.Status = "revoked"
	if _, err := s.ConfigureStudioAccessLocal(ctx, bad); !core.HasCode(err, core.CodeIdempotencyMismatch) {
		t.Fatal("changed retry", err)
	}
	read := StudioEventRequest{PrincipalID: r.TargetPrincipalID, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EventID: granted.EventID}
	if _, err := s.ReadStudioEvent(ctx, read); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM capability_grants WHERE grant_id=?`, granted.Fact.GrantID); err != nil {
		t.Fatal(err)
	}
	diffs, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID)
	if err != nil || len(diffs) != 1 || diffs[0].Projection != "studio_access" {
		t.Fatalf("missing grant not detected: %+v %v", diffs, err)
	}
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReadStudioEvent(ctx, read); err != nil {
		t.Fatal("grant not recovered", err)
	}
	// A forged grant occupying the unique scope must be deleted before the
	// authoritative original is restored, regardless of lexical ID ordering.
	if _, err := s.db.ExecContext(ctx, `UPDATE capability_grants SET grant_id='zz_unsourced_inspector' WHERE grant_id=?`, granted.Fact.GrantID); err != nil {
		t.Fatal(err)
	}
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal("unsourced conflicting grant blocked repair", err)
	}
	r.Binding.ExpectedHead = granted.EventSequence
	r.Binding.IdempotencyKey = "revoke"
	r.Status = "revoked"
	revoked, err := s.ConfigureStudioAccessLocal(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReadStudioEvent(ctx, read); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatal("revoked access", err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE capability_grants SET status='active',field_scope='["event","rule","private"]',subject_id='*' WHERE grant_id=?`, granted.Fact.GrantID); err != nil {
		t.Fatal(err)
	}
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
	if _, err := s.ReadStudioEvent(ctx, read); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatal("rebuild resurrected revoked authority", err)
	}
	oldGrant, err := s.ConfigureStudioAccessLocal(ctx, original)
	if err != nil || !oldGrant.Replayed || oldGrant.EventID != granted.EventID {
		t.Fatal("old grant receipt unavailable", err)
	}
	if _, err := s.ReadStudioEvent(ctx, read); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatal("old grant retry resurrected revoked authority", err)
	}
	replayed, err := s.ConfigureStudioAccessLocal(ctx, r)
	if err != nil || !replayed.Replayed || replayed.EventID != revoked.EventID {
		t.Fatal("revocation retry", err)
	}
	if diffs, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diffs) != 0 {
		t.Fatalf("remaining differences: %+v %v", diffs, err)
	}
	// Legacy M1 chronology is intentionally rejected, never silently retimed.
	legacy := r
	legacy.Binding.InstanceID = DemoInstanceID
	legacy.Binding.BranchID = DemoBranchID
	legacy.Binding.ExpectedHead = 1
	legacy.Binding.IdempotencyKey = "legacy-rejected"
	if _, err := s.ConfigureStudioAccessLocal(ctx, legacy); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatal("inconsistent legacy clock accepted", err)
	}
}
