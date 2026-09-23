package storage

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"corerp.local/backend/internal/core"
)

func TestM2DriverRequiresExplicitRoutineAndResumesAfterInterruption(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "agent-driver.db")
	store := openM2AgentStore(t, ctx, path, false)
	if _, err := store.DriveM2AgentRoutine(ctx, time.Millisecond, 4); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatalf("undeclared routine should not run: %v", err)
	}
	if _, err := store.DefineM2AgentRoutine(ctx, demoRoutineRequest()); err != nil {
		t.Fatal(err)
	}
	interruptedContext, cancel := context.WithCancel(ctx)
	moves := 0
	store.beforeCommit = func() error {
		moves++
		if moves == 2 {
			cancel()
		}
		return nil
	}
	result, err := store.DriveM2AgentRoutine(interruptedContext, time.Millisecond, 1)
	if err != nil || result.Status != "interrupted" {
		t.Fatalf("driver should stop on cancellation: %+v %v", result, err)
	}
	store.beforeCommit = nil
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM agent_runs WHERE status = 'running'`, nil, 0)
	completed, err := store.DriveM2AgentRoutine(ctx, time.Millisecond, 10)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != "completed" || completed.LastRun.HeadSequence != 125 || completed.LastRun.CurrentWorldTime != M2AgentDay30NoonTime {
		t.Fatalf("driver failed to resume and complete: %+v", completed)
	}
	if differences, err := store.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatalf("driver diverged after interruption: %v, %v", differences, err)
	}
	if repeat, err := store.DriveM2AgentRoutine(ctx, time.Millisecond, 10); err != nil || repeat.ProcessedItems != 0 || repeat.LastRun.HeadSequence != 125 {
		t.Fatalf("driver repeated committed moves: %+v %v", repeat, err)
	}
}

func TestM2DriverRejectsMissingCommittedRoutineItem(t *testing.T) {
	ctx := context.Background()
	store := openM2AgentStore(t, ctx, filepath.Join(t.TempDir(), "missing-routine.db"), false)
	defer store.Close()
	if _, err := store.DefineM2AgentRoutine(ctx, demoRoutineRequest()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE scheduler_items SET status = 'cancelled' WHERE scheduler_item_id = ?`, buildM2Routine().Entries[0].ItemID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE agent_schedule_entries SET status = 'cancelled' WHERE schedule_id = ?`, buildM2Routine().Entries[0].ScheduleID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.DriveM2AgentRoutine(ctx, time.Millisecond, 120); !core.HasCode(err, core.CodeProjectionDiverged) {
		t.Fatalf("driver must not report complete with a missing movement: %v", err)
	}
}
