package storage

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestScheduledCommitRejectsWrongWorldAndStaleQueueBeforeWrites(t *testing.T) {
	ctx := context.Background()
	s := openBootstrappedStore(t, ctx, filepath.Join(t.TempDir(), "commit-scope.db"))
	defer s.Close()
	if _, err := s.BootstrapM2AgentDemo(ctx); err != nil {
		t.Fatal(err)
	}
	var item SchedulerItem
	if err := s.db.QueryRowContext(ctx, `SELECT scheduler_item_id,world_time,phase_id,declared_priority,status,payload FROM scheduler_items WHERE instance_id=? AND branch_id=? AND status='pending' ORDER BY world_time,scheduler_item_id LIMIT 1`, M2DemoInstanceID, M2DemoBranchID).Scan(&item.SchedulerItemID, &item.WorldTime, &item.PhaseID, &item.DeclaredPriority, &item.Status, &item.Payload); err != nil {
		t.Fatal(err)
	}
	var beforeEvents, beforeCommands int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM events`).Scan(&beforeEvents); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM commands`).Scan(&beforeCommands); err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"world", "branch", "payload", "time", "priority", "phase", "status", "id"} {
		t.Run(scenario, func(t *testing.T) {
			altered := item
			world, branch := M2DemoInstanceID, M2DemoBranchID
			switch scenario {
			case "world":
				world = DemoInstanceID
			case "branch":
				branch = "other"
			case "payload":
				altered.Payload = `{}`
			case "time":
				altered.WorldTime = "2026-09-30T00:00:00Z"
			case "priority":
				altered.DeclaredPriority++
			case "phase":
				altered.PhaseID = "wrong"
			case "status":
				altered.Status = "completed"
			case "id":
				altered.SchedulerItemID = "missing"
			}
			tx, err := beginImmediate(ctx, s.db)
			if err != nil {
				t.Fatal(err)
			}
			ownedWorld, ownedBranch, scopeErr := recordedSchedulerScope(ctx, tx.conn, altered)
			if scenario == "world" || scenario == "branch" {
				if scopeErr != nil || ownedWorld != M2DemoInstanceID || ownedBranch != M2DemoBranchID {
					tx.Rollback(ctx)
					t.Fatal("caller scope overrode persisted owner", scopeErr)
				}
			} else if !core.HasCode(scopeErr, core.CodeProjectionDiverged) {
				tx.Rollback(ctx)
				t.Fatal("stale handler input accepted", scopeErr)
			}
			called := false
			err = s.commitScheduledMutationForBranch(ctx, tx, altered, scheduledPayload{}, scheduledMutation{EventType: "MustNotCommit", ApplyDomainRows: func(context.Context, *sql.Conn, string, int64) error { called = true; return nil }}, world, branch)
			tx.Rollback(ctx)
			if !core.HasCode(err, core.CodeProjectionDiverged) || called {
				t.Fatal("queue mismatch reached mutation", err, called)
			}
		})
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events`, nil, beforeEvents)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM commands`, nil, beforeCommands)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM scheduler_items WHERE scheduler_item_id=? AND status='pending'`, []any{item.SchedulerItemID}, 1)
}

func TestCareerQueueRequiresRecordedOwner(t *testing.T) {
	ctx := context.Background()
	s := openBootstrappedStore(t, ctx, filepath.Join(t.TempDir(), "queue-source.db"))
	defer s.Close()
	for _, kind := range []string{"career_terms_effective", "career_wage_accrue", "career_wage_pay", "career_wage_retry", "unknown"} {
		tx, err := beginImmediate(ctx, s.db)
		if err != nil {
			t.Fatal(err)
		}
		err = queueCareerPayrollItem(ctx, tx.conn, "missing-source", 1, 0, kind)
		tx.Rollback(ctx)
		if err == nil {
			t.Fatal("queued without recorded owner", kind)
		}
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM scheduler_items WHERE scheduler_item_id LIKE '%missing-source%'`, nil, 0)
}
