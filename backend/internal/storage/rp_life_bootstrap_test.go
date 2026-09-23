package storage

import (
	"context"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPLifePreparationPaysNamedWagesAndRecovers(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "life.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Close() }()
	setup, err := s.PrepareRPLifeDemo(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if setup.Participants.NPCCount != 3 || setup.Travel.LinkCount != 8 || setup.Routine.ScheduleCount != 116 {
		t.Fatalf("setup: %+v", setup)
	}
	assertM2Value(t, ctx, s, `SELECT population_count FROM cohorts WHERE cohort_id=?`, []any{M2DemoCohortID}, 16)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM m2_wage_participation_splits`, nil, 2)
	if _, err := s.RunAgentLife(ctx, "2026-09-23T07:01:00Z", 10); err != nil {
		t.Fatal(err)
	}
	for entity, want := range map[string]int64{M2RPNPCID: 410, M2RPPlayerID: 310} {
		assertM2Value(t, ctx, s, `SELECT b.balance_minor FROM account_balances b JOIN materialized_entities n ON n.asset_account_id=b.account_id WHERE n.entity_id=?`, []any{entity}, want)
	}
	assertM2Value(t, ctx, s, `SELECT balance_minor FROM account_balances WHERE account_id=?`, []any{m2EconomyEmployerCash}, 1020)
	var count int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM events`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.PrepareRPLifeDemo(ctx)
	if err != nil || !again.Participants.Replayed || !again.Travel.Replayed || !again.Economy.Replayed || !again.Routine.Replayed {
		t.Fatalf("retry: %+v %v", again, err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events`, nil, count)
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatalf("replay: %+v %v", diff, err)
	}
}

func TestRPLifePreparationResumesInterruptedComposition(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "partial.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.BootstrapM2AgentDemo(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PrepareM2EconomicDemo(ctx); err != nil {
		t.Fatal(err)
	}
	commits := 0
	s.beforeCommit = func() error {
		commits++
		if commits == 2 {
			return core.NewError(core.CodeInjectedFailure, "interrupt after Cai")
		}
		return nil
	}
	if _, err := s.PrepareRPLifeDemo(ctx); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatalf("interruption: %v", err)
	}
	s.beforeCommit = nil
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM m2_wage_participation_splits`, nil, 1)
	if _, err := s.PrepareRPLifeDemo(ctx); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM m2_wage_participation_splits`, nil, 2)
}

func TestRPLifePreparationRejectsConvertingMinimalDemo(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "minimal.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.PrepareRPTravel(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PrepareRPLifeDemo(ctx); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatalf("conversion must fail safely: %v", err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events`, nil, 8)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM m2_cohort_contracts`, nil, 0)
}
