package storage

import (
	"context"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPStyleReadSessionRevisionSupportsRecoveryAndConflict(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "style-revision.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Close() }()
	_, read, initial := newRPWaitTestSession(t, ctx, s)
	assertRevision := func(want int64) {
		t.Helper()
		result, err := s.ReadRPStyle(ctx, read)
		if err != nil || result.SessionRevision == nil || *result.SessionRevision != want {
			t.Fatalf("session revision want %d got %+v: %v", want, result, err)
		}
	}
	assertRevision(0)
	first := "first_person"
	r := RPStyleSetRequest{PrincipalID: read.PrincipalID, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, Scope: "world", IdempotencyKey: "revision-world", Patch: core.RPStylePatch{POV: &first}}
	r.PrincipalID = "principal_creator"
	if _, err := s.SetRPStyle(ctx, r); err != nil {
		t.Fatal(err)
	}
	assertRevision(0) // Resolved world style does not become the session version.
	r.PrincipalID, r.Scope, r.SessionID, r.IdempotencyKey = read.PrincipalID, "session", read.SessionID, "revision-one"
	if _, err := s.SetRPStyle(ctx, r); err != nil {
		t.Fatal(err)
	}
	assertRevision(1)
	r.IdempotencyKey = "revision-two"
	if _, err := s.SetRPStyle(ctx, r); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatalf("stale revision accepted: %v", err)
	}
	r.ExpectedRevision = 1
	if _, err := s.SetRPStyle(ctx, r); err != nil {
		t.Fatal(err)
	}
	assertRevision(2)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	assertRevision(2)
	foreign := read
	foreign.PrincipalID = "principal_creator"
	if _, err := s.ReadRPStyle(ctx, foreign); err == nil {
		t.Fatal("another principal read the player's session settings")
	}
	assertM2Value(t, ctx, s, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, []any{M2DemoInstanceID, M2DemoBranchID}, initial.ObservationCursor)
}
