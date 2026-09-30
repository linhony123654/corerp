package storage

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"corerp.local/backend/internal/core"
)

func prepareProgressionWorld(t *testing.T, ctx context.Context, s *Store, world string, enabled bool, budget int, people []core.StudioWorldPerson) StudioCreateRequest {
	t.Helper()
	if _, err := s.PrepareRPTravel(ctx); err != nil {
		t.Fatal(err)
	}
	var head int64
	if err := s.db.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&head); err != nil {
		t.Fatal(err)
	}
	_, err := s.ConfigureStudioAccessLocal(ctx, StudioAccessRequest{Purpose: "create_world", Binding: core.CareerBinding{PrincipalID: "principal_operator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: head, IdempotencyKey: "progression-grant-" + world}, TargetPrincipalID: "principal_creator", Status: "active"})
	if err != nil {
		t.Fatal(err)
	}
	create := studioCreateFixture(world)
	if people != nil {
		create.Spec.People = people
		create.Spec.Population = int64(len(people))
	}
	if enabled {
		create.SystemPackage.Content.SystemRules.BackgroundProgression = &core.StudioBackgroundProgression{Enabled: true, StepMinutes: 10, SchedulerBudget: budget}
		create.SystemPackage.Manifest.ContentHash, _ = core.HashJSON(create.SystemPackage.Content)
	}
	if _, err := s.CreateStudioWorld(ctx, create); err != nil {
		t.Fatal(err)
	}
	return create
}

func progressionPeople(twoNPCs bool) []core.StudioWorldPerson {
	people := []core.StudioWorldPerson{
		{Key: "lin", Name: "Lin", Place: "home", Player: true},
		{Key: "cai", Name: "Cai", Place: "home", Routine: []core.StudioWorldRoutineEntry{{WorldTime: "2026-09-22T00:05:00Z", Place: "square", ActivityCode: "walk"}}},
	}
	if twoNPCs {
		people = append(people, core.StudioWorldPerson{Key: "bo", Name: "Bo", Place: "home", Routine: []core.StudioWorldRoutineEntry{{WorldTime: "2026-09-22T00:06:00Z", Place: "square", ActivityCode: "walk"}}})
	}
	return people
}

func TestRPBackgroundProgressionDisabledWorldStaysStill(t *testing.T) {
	ctx := context.Background()
	s := openBootstrappedStore(t, ctx, filepath.Join(t.TempDir(), "disabled.db"))
	defer s.Close()
	create := prepareProgressionWorld(t, ctx, s, "progression-disabled", false, 0, progressionPeople(false))
	result, err := s.RunRPBackgroundProgression(ctx, RPBackgroundProgressionRequest{WorkerID: "worker-a", InstanceID: create.InstanceID, BranchID: "br_main", LeaseDuration: 30 * time.Second})
	if err != nil || result.Status != "disabled" {
		t.Fatalf("disabled result %+v %v", result, err)
	}
	assertStudioString(t, ctx, s, `SELECT current_world_time FROM world_clocks WHERE instance_id=? AND branch_id='br_main'`, []any{create.InstanceID}, core.StudioWorldStart)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_background_runs WHERE instance_id=?`, []any{create.InstanceID}, 0)
}

func TestRPBackgroundProgressionAdvancesOnceAndFencesConcurrentWorker(t *testing.T) {
	ctx := context.Background()
	s := openBootstrappedStore(t, ctx, filepath.Join(t.TempDir(), "concurrent.db"))
	defer s.Close()
	create := prepareProgressionWorld(t, ctx, s, "progression-concurrent", true, 10, progressionPeople(false))
	start := make(chan struct{})
	results := make(chan RPBackgroundProgressionResult, 2)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, worker := range []string{"worker-a", "worker-b"} {
		wg.Add(1)
		go func(worker string) {
			defer wg.Done()
			<-start
			result, err := s.RunRPBackgroundProgression(ctx, RPBackgroundProgressionRequest{WorkerID: worker, InstanceID: create.InstanceID, BranchID: "br_main", LeaseDuration: 30 * time.Second})
			results <- result
			errs <- err
		}(worker)
	}
	close(start)
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	statuses := map[string]int{}
	for result := range results {
		statuses[result.Status]++
	}
	if statuses["completed"] != 1 || statuses["busy"] != 1 {
		t.Fatalf("unexpected worker outcomes %#v", statuses)
	}
	assertStudioString(t, ctx, s, `SELECT current_world_time FROM world_clocks WHERE instance_id=? AND branch_id='br_main'`, []any{create.InstanceID}, "2026-09-22T00:10:00Z")
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE instance_id=? AND event_type='WorldTimeAdvanced'`, []any{create.InstanceID}, 1)
}

func TestRPBackgroundProgressionRespectsControlAndBudgetThenResumesAfterRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "resume.db")
	s := openBootstrappedStore(t, ctx, path)
	create := prepareProgressionWorld(t, ctx, s, "progression-resume", true, 1, progressionPeople(true))
	playerID, _ := core.StudioWorldObjectID(create.InstanceID, "entity", "lin")
	session, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: M2RPPlayerPrincipal, InstanceID: create.InstanceID, BranchID: "br_main", EntityID: playerID, POV: "second_person", IdempotencyKey: "progression-control"})
	if err != nil {
		t.Fatal(err)
	}
	controlled, err := s.RunRPBackgroundProgression(ctx, RPBackgroundProgressionRequest{WorkerID: "worker-control", InstanceID: create.InstanceID, BranchID: "br_main", LeaseDuration: 5 * time.Second})
	if err != nil || controlled.Status != "controlled" {
		t.Fatalf("controlled result %+v %v", controlled, err)
	}
	if _, err := s.CloseRPSession(ctx, core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE rp_background_leases SET lease_until_utc='2000-01-01T00:00:00Z' WHERE instance_id=?`, create.InstanceID); err != nil {
		t.Fatal(err)
	}
	first, err := s.RunRPBackgroundProgression(ctx, RPBackgroundProgressionRequest{WorkerID: "worker-first", InstanceID: create.InstanceID, BranchID: "br_main", LeaseDuration: 5 * time.Second})
	if err != nil || first.Status != "budget_exhausted" || first.ProcessedItems != 1 || first.PendingDue != 1 {
		t.Fatalf("budget result %+v %v", first, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.db.ExecContext(ctx, `UPDATE rp_background_leases SET lease_until_utc='2000-01-01T00:00:00Z' WHERE instance_id=?`, create.InstanceID); err != nil {
		t.Fatal(err)
	}
	second, err := s.RunRPBackgroundProgression(ctx, RPBackgroundProgressionRequest{WorkerID: "worker-restart", InstanceID: create.InstanceID, BranchID: "br_main", LeaseDuration: 5 * time.Second})
	if err != nil || second.Status != "completed" || second.ProcessedItems != 1 {
		t.Fatalf("restart result %+v %v", second, err)
	}
	assertStudioString(t, ctx, s, `SELECT current_world_time FROM world_clocks WHERE instance_id=? AND branch_id='br_main'`, []any{create.InstanceID}, "2026-09-22T00:10:00Z")
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE instance_id=? AND event_type='WorldTimeAdvanced'`, []any{create.InstanceID}, 1)
}
