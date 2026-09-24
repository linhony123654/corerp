package storage

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPOpportunityCommunityDefaultPreservesPolicyHash(t *testing.T) {
	legacy := struct {
		StreamSeed         string `json:"stream_seed"`
		ContactBasisPoints int    `json:"contact_basis_points"`
		CooldownHours      int    `json:"cooldown_hours"`
		HistoryHours       int    `json:"history_hours"`
	}{"legacy", 1000, 1, 24}
	current := RPOpportunityPolicy{StreamSeed: "legacy", ContactBasisPoints: 1000, CooldownHours: 1, HistoryHours: 24}
	a, err := core.HashJSON(legacy)
	if err != nil {
		t.Fatal(err)
	}
	b, err := core.HashJSON(current)
	if err != nil || a != b {
		t.Fatalf("legacy policy hash changed: %s %s %v", a, b, err)
	}
	current.CommunityBasisPoints = 1000
	c, err := core.HashJSON(current)
	if err != nil || c == a {
		t.Fatalf("community chance not pinned: %v", err)
	}
}

func TestRPOpportunityVisitPolicyBoundsAndHash(t *testing.T) {
	base := RPOpportunityPolicy{StreamSeed: "visits", CooldownHours: 1, HistoryHours: 240}
	legacyHash, _ := core.HashJSON(base)
	for _, rare := range []bool{false, true} {
		p := base
		if rare {
			p.RareVisitBasisPoints = 100
		} else {
			p.VisitBasisPoints = 5000
		}
		if err := p.validate(); err != nil {
			t.Fatal(err)
		}
		hash, _ := core.HashJSON(p)
		if hash == legacyHash {
			t.Fatal("visit policy is not hash-pinned")
		}
		if rare {
			p.RareVisitBasisPoints++
		} else {
			p.VisitBasisPoints++
		}
		if err := p.validate(); !core.HasCode(err, core.CodeInvalidArgument) {
			t.Fatalf("excess chance accepted: %v", err)
		}
	}
	base.RareVisitBasisPoints = 1
	base.HistoryHours = 167
	if err := base.validate(); !core.HasCode(err, core.CodeInvalidArgument) {
		t.Fatalf("short rare memory window: %v", err)
	}
	base.HistoryHours = 168
	if err := base.validate(); err != nil {
		t.Fatal(err)
	}
}

func TestRPOpportunityPolicyAuthorityRollbackAndRecovery(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "opportunity.db")
	s := openCareerTestWorld(t, path)
	defer func() { s.Close() }()
	read := func() (OpportunityPolicyRecord, error) {
		tx, err := beginImmediate(ctx, s.db)
		if err != nil {
			return OpportunityPolicyRecord{}, err
		}
		defer tx.Rollback(ctx)
		return readRPOpportunityPolicy(ctx, tx.conn, M2DemoInstanceID, M2DemoBranchID)
	}
	if _, err := read(); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("unconfigured world: %v", err)
	}
	r := OpportunityPolicyRequest{Binding: careerTestBinding(t, s, "principal_creator", "opportunities"), Policy: RPOpportunityPolicy{StreamSeed: "fixture-run-stream", ContactBasisPoints: 1000, CooldownHours: 6, HistoryHours: 24}}
	bad := r
	bad.Binding.PrincipalID = M2AgentBoPrincipal
	if _, err := s.DefineRPOpportunityPolicy(ctx, bad); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("agent configured randomness: %v", err)
	}
	s.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "rollback policy") }
	if _, err := s.DefineRPOpportunityPolicy(ctx, r); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatalf("rollback: %v", err)
	}
	s.beforeCommit = nil
	if _, err := read(); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("rollback leaked policy: %v", err)
	}
	created, err := s.DefineRPOpportunityPolicy(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	if created.Fact.BuilderSourceEventID == "" {
		t.Fatal("lost creator source")
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM outbox WHERE event_id=?`, []any{created.EventID}, 0)
	bad = r
	bad.Policy.StreamSeed = "reroll"
	if _, err := s.DefineRPOpportunityPolicy(ctx, bad); !core.HasCode(err, core.CodeIdempotencyMismatch) {
		t.Fatalf("changed seed retry: %v", err)
	}
	bad.Binding = careerTestBinding(t, s, "principal_creator", "fresh-key")
	if _, err := s.DefineRPOpportunityPolicy(ctx, bad); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatalf("fresh-key stream reset: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	stored, err := read()
	if err != nil || stored.Fact != created.Fact || stored.EventID != created.EventID {
		t.Fatalf("policy recovery: %+v %v", stored, err)
	}
	retry, err := s.DefineRPOpportunityPolicy(ctx, r)
	if err != nil || !retry.Replayed || retry.EventID != created.EventID {
		t.Fatalf("exact retry: %+v %v", retry, err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE principals SET status='disabled' WHERE principal_id='principal_creator'`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DefineRPOpportunityPolicy(ctx, r); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("revoked retry: %v", err)
	}
}
