package storage

import (
	"context"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestAgentScopedRunnerDoesNotDispatchForeignEconomy(t *testing.T) {
	ctx := context.Background()
	s := openBootstrappedStore(t, ctx, filepath.Join(t.TempDir(), "scope.db"))
	defer s.Close()
	if _, err := s.BootstrapM2AgentDemo(ctx); err != nil {
		t.Fatal(err)
	}
	var m1Time, m2Time string
	var m1Head, m2Head, events int64
	for _, q := range []struct {
		query string
		args  []any
		dest  any
	}{
		{`SELECT current_world_time FROM world_clocks WHERE instance_id=? AND branch_id=?`, []any{DemoInstanceID, DemoBranchID}, &m1Time},
		{`SELECT current_world_time FROM world_clocks WHERE instance_id=? AND branch_id=?`, []any{M2DemoInstanceID, M2DemoBranchID}, &m2Time},
		{`SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, []any{DemoInstanceID, DemoBranchID}, &m1Head},
		{`SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, []any{M2DemoInstanceID, M2DemoBranchID}, &m2Head},
		{`SELECT COUNT(*) FROM events`, nil, &events},
	} {
		if err := s.db.QueryRowContext(ctx, q.query, q.args...).Scan(q.dest); err != nil {
			t.Fatal(err)
		}
	}
	// Real distinct clocks: a fallback to M2 would reject the older M1 target.
	out, err := s.runAgentLifeForScope(ctx, DemoInstanceID, DemoBranchID, m1Time, 10)
	if err != nil || out.HeadSequence != m1Head || out.ProcessedItems != 0 || out.CurrentWorldTime != m1Time {
		t.Fatal("wrong scoped clock/queue", out, err)
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO scheduler_items(scheduler_item_id,instance_id,branch_id,world_time,phase_id,declared_priority,status,payload) VALUES ('foreign-payroll',?,?,?, ?,0,'pending','{}')`, DemoInstanceID, DemoBranchID, m1Time, careerPayrollPhase); err != nil {
		t.Fatal(err)
	}
	if _, err := s.runAgentLifeForScope(ctx, DemoInstanceID, DemoBranchID, m1Time, 10); !core.HasCode(err, core.CodeInvalidArgument) {
		t.Fatal("foreign task reached demo economic handler", err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events`, nil, events)
	assertM2Value(t, ctx, s, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, []any{M2DemoInstanceID, M2DemoBranchID}, m2Head)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM scheduler_items WHERE scheduler_item_id='foreign-payroll' AND status='pending'`, nil, 1)
	var after string
	if err := s.db.QueryRowContext(ctx, `SELECT current_world_time FROM world_clocks WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&after); err != nil || after != m2Time {
		t.Fatal("other world clock changed", err)
	}
	if _, err := s.runAgentLifeForScope(ctx, "missing-world", DemoBranchID, m1Time, 10); err == nil {
		t.Fatal("missing world accepted")
	}
}
