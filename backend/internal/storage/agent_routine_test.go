package storage

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"corerp.local/backend/internal/core"
)

func demoRoutineRequest() core.AgentRoutineRequest {
	return core.AgentRoutineRequest{
		PrincipalID: "principal_creator", CapabilityID: "world.agent.run",
		InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, Days: 30,
	}
}

func routineLastNoon() string {
	start, _ := time.Parse(time.RFC3339, M2AgentNoonTime)
	return start.AddDate(0, 0, 29).Format(time.RFC3339)
}

func TestM2RoutineRunsThirtyDaysAndReplaysAfterReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "routine.db")
	store := openM2AgentStore(t, ctx, path, false)
	request := demoRoutineRequest()
	definition, err := store.DefineM2AgentRoutine(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if definition.EventSequence != 5 || definition.ScheduleCount != 116 || definition.Replayed {
		t.Fatalf("unexpected routine definition: %+v", definition)
	}
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM agent_schedule_entries`, nil, 120)
	var economicCountsBefore [3]int
	for index, query := range []string{`SELECT COUNT(*) FROM postings`, `SELECT COUNT(*) FROM stock_movements`, `SELECT COUNT(*) FROM population_movements`} {
		if err := store.db.QueryRowContext(ctx, query).Scan(&economicCountsBefore[index]); err != nil {
			t.Fatal(err)
		}
	}
	partial, err := store.RunAgentLife(ctx, routineLastNoon(), 4)
	if err != nil {
		t.Fatal(err)
	}
	if partial.Status != "budget_exhausted" || partial.ProcessedItems != 4 || partial.PendingDue != 116 || partial.HeadSequence != 9 {
		t.Fatalf("unexpected day-one checkpoint: %+v", partial)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if replayed, err := store.DefineM2AgentRoutine(ctx, request); err != nil || !replayed.Replayed || replayed.EventSequence != 5 {
		t.Fatalf("routine retry changed authority: result=%+v err=%v", replayed, err)
	}
	result, err := store.RunAgentLife(ctx, routineLastNoon(), 116)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "completed" || result.ProcessedItems != 116 || result.PendingDue != 0 || result.HeadSequence != 125 || result.CurrentWorldTime != routineLastNoon() {
		t.Fatalf("unexpected 30-day run: %+v", result)
	}
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM agent_movements`, nil, 122)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM observation_records`, nil, 60)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM agent_knowledge`, nil, 2)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM population_movements`, nil, 3)
	for index, query := range []string{`SELECT COUNT(*) FROM postings`, `SELECT COUNT(*) FROM stock_movements`, `SELECT COUNT(*) FROM population_movements`} {
		assertM2Value(t, ctx, store, query, nil, int64(economicCountsBefore[index]))
	}
	assertM2Value(t, ctx, store, `SELECT (SELECT population_count FROM cohorts WHERE cohort_id = ?) + (SELECT SUM(population_count) FROM materialized_entities WHERE entity_id IN (?, ?))`, []any{M2DemoCohortID, M2AgentAdaID, M2AgentBoID}, 20)
	if differences, err := store.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatalf("routine projections diverged: %v, %v", differences, err)
	}
	full, err := store.Replay(ctx, M2DemoInstanceID, M2DemoBranchID, 125)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateSnapshot(ctx, M2DemoInstanceID, M2DemoBranchID, 125); err != nil {
		t.Fatal(err)
	}
	fromSnapshot, err := store.ReplayFromLatestSnapshot(ctx, M2DemoInstanceID, M2DemoBranchID, 125)
	if err != nil {
		t.Fatal(err)
	}
	if full.StateHash != fromSnapshot.StateHash || !reflect.DeepEqual(full.State, fromSnapshot.State) {
		t.Fatal("30-day replay and snapshot continuation differ")
	}
	if rerun, err := store.RunAgentLife(ctx, routineLastNoon(), 120); err != nil || rerun.ProcessedItems != 0 || rerun.HeadSequence != 125 {
		t.Fatalf("rerun changed authority: result=%+v err=%v", rerun, err)
	}
}

func TestM2RoutineAuthorizationLateDefinitionAndRollback(t *testing.T) {
	ctx := context.Background()
	store := openM2AgentStore(t, ctx, filepath.Join(t.TempDir(), "routine-fail.db"), false)
	defer store.Close()
	request := demoRoutineRequest()
	unauthorized := request
	unauthorized.PrincipalID = M2AgentAdaPrincipal
	if _, err := store.DefineM2AgentRoutine(ctx, unauthorized); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("unauthorized routine should fail: %v", err)
	}
	wrongDays := request
	wrongDays.Days = 31
	if _, err := store.DefineM2AgentRoutine(ctx, wrongDays); !core.HasCode(err, core.CodeInvalidArgument) {
		t.Fatalf("unbounded routine should fail: %v", err)
	}
	store.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "routine rollback") }
	if _, err := store.DefineM2AgentRoutine(ctx, request); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatalf("injected routine failure should roll back: %v", err)
	}
	store.beforeCommit = nil
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM agent_schedule_entries`, nil, 4)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM commands WHERE command_id = ?`, []any{m2RoutineCommandID}, 0)
	if _, err := store.RunAgentLife(ctx, M2AgentNoonTime, 4); err != nil {
		t.Fatal(err)
	}
	if _, err := store.DefineM2AgentRoutine(ctx, request); err != nil {
		t.Fatalf("definition after day one is still valid: %v", err)
	}
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM agent_schedule_entries`, nil, 120)
}

func TestM2RoutineCannotFirstDeclareAfterNewDayHasStarted(t *testing.T) {
	ctx := context.Background()
	store := openM2AgentStore(t, ctx, filepath.Join(t.TempDir(), "late-routine.db"), false)
	defer store.Close()
	if _, err := store.db.ExecContext(ctx, `UPDATE world_clocks SET current_world_time = '2026-09-24T08:01:00Z' WHERE instance_id = ? AND branch_id = ?`, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.DefineM2AgentRoutine(ctx, demoRoutineRequest()); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatalf("late first definition should fail: %v", err)
	}
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM agent_schedule_entries`, nil, 4)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM commands WHERE command_id = ?`, []any{m2RoutineCommandID}, 0)
}
