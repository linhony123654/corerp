package storage

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestM2AgentSetupIsAuthoritativeIdempotentAndDurable(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "agent-setup.db")
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	result, err := store.BootstrapM2AgentDemo(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if result.Replayed || result.EventSequence != 4 || result.AgentCount != 2 || result.ScheduleCount != 4 {
		t.Fatalf("unexpected Agent setup result: %+v", result)
	}
	assertAgentSetupState(t, ctx, store)
	replayed, err := store.BootstrapM2AgentDemo(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !replayed.Replayed || replayed.EventSequence != result.EventSequence {
		t.Fatalf("Agent setup retry did not replay: %+v", replayed)
	}
	assertAgentSetupState(t, ctx, store)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	assertAgentSetupState(t, ctx, reopened)
}

func TestM2AgentSchedulesUseStableOrderAndCreateOnlyCoLocatedKnowledge(t *testing.T) {
	ctx := context.Background()
	store := openM2AgentStore(t, ctx, filepath.Join(t.TempDir(), "agent-run.db"), false)
	defer store.Close()

	run, err := store.RunAgentLife(ctx, M2AgentNoonTime, 4)
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != "completed" || run.ProcessedItems != 4 || run.PendingDue != 0 || run.HeadSequence != 8 || run.CurrentWorldTime != M2AgentNoonTime {
		t.Fatalf("unexpected Agent run: %+v", run)
	}
	wantOrder := []string{M2AgentAdaID, M2AgentBoID, M2AgentAdaID, M2AgentBoID}
	if got := agentMovementOrder(t, ctx, store); !reflect.DeepEqual(got, wantOrder) {
		t.Fatalf("unstable Agent movement order: got %v want %v", got, wantOrder)
	}
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM agent_positions WHERE place_id = ?`, []any{M2AgentCafeID}, 2)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM observation_records`, nil, 2)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM agent_knowledge`, nil, 2)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM observation_records WHERE observed_world_time < ?`, []any{M2AgentNoonTime}, 0)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM agent_knowledge WHERE observer_agent_id = ? AND subject_agent_id = ? AND place_id = ?`, []any{M2AgentAdaID, M2AgentBoID, M2AgentCafeID}, 1)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM agent_knowledge WHERE observer_agent_id = ? AND subject_agent_id = ? AND place_id = ?`, []any{M2AgentBoID, M2AgentAdaID, M2AgentCafeID}, 1)

	repeat, err := store.RunAgentLife(ctx, M2AgentNoonTime, 4)
	if err != nil {
		t.Fatal(err)
	}
	if repeat.ProcessedItems != 0 || repeat.HeadSequence != 8 {
		t.Fatalf("repeat Agent run changed authority: %+v", repeat)
	}
}

func TestM2RunnerDoesNotPassUnsupportedEarlierWorldTask(t *testing.T) {
	ctx := context.Background()
	store := openM2AgentStore(t, ctx, filepath.Join(t.TempDir(), "unknown-phase.db"), false)
	defer store.Close()
	const economicPhase = "m2_10_economic_due"
	if _, err := store.db.ExecContext(ctx, `INSERT INTO scheduler_phases(phase_id, description, ruleset_hash) SELECT ?, 'pending economic transition', ruleset_hash FROM scheduler_phases WHERE phase_id = ?`, economicPhase, m2AgentPhaseID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO scheduler_items(scheduler_item_id, instance_id, branch_id, world_time, phase_id, declared_priority, status, payload) VALUES ('sched_m2_unhandled_wage', ?, ?, '2026-09-23T07:00:00Z', ?, 0, 'pending', '{}')`, M2DemoInstanceID, M2DemoBranchID, economicPhase); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RunAgentLife(ctx, M2AgentNoonTime, 4); !core.HasCode(err, core.CodeStorageFailure) {
		t.Fatalf("unhandled earlier economy must stop the M2 runner, got %v", err)
	}
	assertM2Value(t, ctx, store, `SELECT head_sequence FROM branches WHERE instance_id = ? AND branch_id = ?`, []any{M2DemoInstanceID, M2DemoBranchID}, 4)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM agent_movements WHERE movement_kind = 'scheduled'`, nil, 0)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM agent_runs WHERE status = 'running'`, nil, 0)
}

func TestM2RunnerRejectsLateBackdatedWorldTask(t *testing.T) {
	ctx := context.Background()
	store := openM2AgentStore(t, ctx, filepath.Join(t.TempDir(), "backdated-phase.db"), false)
	defer store.Close()
	if _, err := store.RunAgentLife(ctx, M2AgentMorningTime, 1); err != nil {
		t.Fatal(err)
	}
	const economicPhase = "m2_10_economic_due"
	if _, err := store.db.ExecContext(ctx, `INSERT INTO scheduler_phases(phase_id, description, ruleset_hash) SELECT ?, 'late economic transition', ruleset_hash FROM scheduler_phases WHERE phase_id = ?`, economicPhase, m2AgentPhaseID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO scheduler_items(scheduler_item_id, instance_id, branch_id, world_time, phase_id, declared_priority, status, payload) VALUES ('sched_m2_backdated_wage', ?, ?, '2026-09-23T07:00:00Z', ?, 0, 'pending', '{}')`, M2DemoInstanceID, M2DemoBranchID, economicPhase); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RunAgentLife(ctx, M2AgentNoonTime, 4); !core.HasCode(err, core.CodeStorageFailure) {
		t.Fatalf("late backdated economic task must fail without advancing, got %v", err)
	}
	assertM2Value(t, ctx, store, `SELECT head_sequence FROM branches WHERE instance_id = ? AND branch_id = ?`, []any{M2DemoInstanceID, M2DemoBranchID}, 5)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM agent_movements WHERE movement_kind = 'scheduled'`, nil, 1)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM agent_runs WHERE status = 'running'`, nil, 0)
}

func TestM2AgentScheduleOrderIgnoresPhysicalInsertionOrder(t *testing.T) {
	ctx := context.Background()
	forward := openM2AgentStore(t, ctx, filepath.Join(t.TempDir(), "forward.db"), false)
	defer forward.Close()
	reverse := openM2AgentStore(t, ctx, filepath.Join(t.TempDir(), "reverse.db"), true)
	defer reverse.Close()
	if _, err := forward.RunAgentLife(ctx, M2AgentNoonTime, 4); err != nil {
		t.Fatal(err)
	}
	if _, err := reverse.RunAgentLife(ctx, M2AgentNoonTime, 4); err != nil {
		t.Fatal(err)
	}
	if got, want := agentMovementOrder(t, ctx, reverse), agentMovementOrder(t, ctx, forward); !reflect.DeepEqual(got, want) {
		t.Fatalf("insertion order changed Agent authority: got %v want %v", got, want)
	}
}

func TestM2AgentBudgetCheckpointResumesAfterReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "agent-resume.db")
	store := openM2AgentStore(t, ctx, path, false)
	partial, err := store.RunAgentLife(ctx, M2AgentNoonTime, 3)
	if err != nil {
		t.Fatal(err)
	}
	if partial.Status != "budget_exhausted" || partial.ProcessedItems != 3 || partial.PendingDue != 1 || partial.HeadSequence != 7 {
		t.Fatalf("unexpected partial Agent run: %+v", partial)
	}
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM agent_knowledge`, nil, 0)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	completed, err := reopened.RunAgentLife(ctx, M2AgentNoonTime, 1)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != "completed" || completed.ProcessedItems != 1 || completed.PendingDue != 0 || completed.HeadSequence != 8 {
		t.Fatalf("unexpected resumed Agent run: %+v", completed)
	}
	assertM2Value(t, ctx, reopened, `SELECT COUNT(*) FROM observation_records`, nil, 2)
}

func TestM2AgentMovementRollbackAndAuthorizationFailuresLeaveNoWrites(t *testing.T) {
	ctx := context.Background()
	store := openM2AgentStore(t, ctx, filepath.Join(t.TempDir(), "agent-failure.db"), false)
	defer store.Close()
	store.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "Agent movement pre-commit failure") }
	if _, err := store.RunAgentLife(ctx, M2AgentMorningTime, 1); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatalf("got %v want injected failure", err)
	}
	store.beforeCommit = nil
	assertM2Value(t, ctx, store, `SELECT head_sequence FROM branches WHERE instance_id = ? AND branch_id = ?`, []any{M2DemoInstanceID, M2DemoBranchID}, 4)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM agent_movements WHERE movement_kind = 'scheduled'`, nil, 0)
	assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM scheduler_items WHERE instance_id = ? AND branch_id = ? AND phase_id = ? AND status = 'pending'`, []any{M2DemoInstanceID, M2DemoBranchID, m2AgentPhaseID}, 4)

	unauthorized := core.AgentLifeRunRequest{
		PrincipalID: "principal_intruder", CapabilityID: "world.agent.run",
		InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID,
		TargetWorldTime: M2AgentMorningTime, Budget: 1,
	}
	if _, err := store.RunAgentLifeAuthorized(ctx, unauthorized); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("unauthorized Agent run should fail, got %v", err)
	}
	backward := unauthorized
	backward.PrincipalID = "principal_creator"
	backward.TargetWorldTime = "2026-09-22T01:59:59Z"
	if _, err := store.RunAgentLifeAuthorized(ctx, backward); !core.HasCode(err, core.CodeInvalidArgument) {
		t.Fatalf("backward Agent run should fail, got %v", err)
	}
	assertM2Value(t, ctx, store, `SELECT head_sequence FROM branches WHERE instance_id = ? AND branch_id = ?`, []any{M2DemoInstanceID, M2DemoBranchID}, 4)
}

func TestM2AgentSetupConflictAndRollbackLeaveNoPartialState(t *testing.T) {
	ctx := context.Background()
	t.Run("rollback", func(t *testing.T) {
		store, err := Open(ctx, filepath.Join(t.TempDir(), "agent-setup-rollback.db"))
		if err != nil {
			t.Fatal(err)
		}
		defer store.Close()
		prepareM2AgentMaterializations(t, ctx, store)
		store.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "Agent setup pre-commit failure") }
		if _, err := store.setupM2AgentLife(ctx); !core.HasCode(err, core.CodeInjectedFailure) {
			t.Fatalf("got %v want injected failure", err)
		}
		store.beforeCommit = nil
		assertM2Value(t, ctx, store, `SELECT head_sequence FROM branches WHERE instance_id = ? AND branch_id = ?`, []any{M2DemoInstanceID, M2DemoBranchID}, 3)
		assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM agent_profiles`, nil, 0)
		assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM commands WHERE command_id = ?`, []any{m2AgentSetupCommandID}, 0)
		if _, err := store.setupM2AgentLife(ctx); err != nil {
			t.Fatal(err)
		}
		assertAgentSetupState(t, ctx, store)
	})

	t.Run("conflicting branch history", func(t *testing.T) {
		store, err := Open(ctx, filepath.Join(t.TempDir(), "agent-setup-conflict.db"))
		if err != nil {
			t.Fatal(err)
		}
		defer store.Close()
		prepareM2AgentMaterializations(t, ctx, store)
		extra := m2AgentMaterialization("extra", "entity_m2_agent_extra", "Extra", 1, 100, 1, 20, 10, 3, "2026-09-22T01:30:00Z")
		if _, err := store.MaterializeCohort(ctx, extra); err != nil {
			t.Fatal(err)
		}
		if _, err := store.setupM2AgentLife(ctx); !core.HasCode(err, core.CodeBranchConflict) {
			t.Fatalf("got %v want branch conflict", err)
		}
		assertM2Value(t, ctx, store, `SELECT COUNT(*) FROM agent_profiles`, nil, 0)
	})
}

func TestM2AgentKnowledgeAndEncounterAreScopedStateBackedAndReadOnly(t *testing.T) {
	ctx := context.Background()
	store := openM2AgentStore(t, ctx, filepath.Join(t.TempDir(), "agent-query.db"), false)
	defer store.Close()
	if _, err := store.RunAgentLife(ctx, M2AgentMorningTime, 2); err != nil {
		t.Fatal(err)
	}
	emptyRequest := core.EncounterRead{
		PrincipalID: M2AgentAdaPrincipal, CapabilityID: "world.encounter.read",
		InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ObserverAgentID: M2AgentAdaID,
		Fields: []string{"place_id", "world_time", "participants", "activity", "evidence"},
	}
	empty, err := store.ResolveEncounter(ctx, emptyRequest)
	if err != nil {
		t.Fatal(err)
	}
	if empty.PlaceID != "place_m2_work_ada" || len(empty.Participants) != 0 || len(empty.EvidenceRefs) != 0 {
		t.Fatalf("separate workplaces should produce an ordinary empty encounter: %+v", empty)
	}
	if _, err := store.RunAgentLife(ctx, M2AgentNoonTime, 2); err != nil {
		t.Fatal(err)
	}

	adaRead := core.AgentKnowledgeRead{
		PrincipalID: M2AgentAdaPrincipal, CapabilityID: "world.agent.knowledge.read",
		InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ObserverAgentID: M2AgentAdaID,
		Fields: []string{"claim_key", "subject_agent_id", "place_id", "learned_world_time", "source_event_id"},
	}
	adaKnowledge, err := store.ReadAgentKnowledge(ctx, adaRead)
	if err != nil {
		t.Fatal(err)
	}
	if len(adaKnowledge.Facts) != 1 || adaKnowledge.Facts[0].SubjectAgentID != M2AgentBoID || adaKnowledge.Facts[0].PlaceID != M2AgentCafeID || adaKnowledge.Facts[0].LearnedWorldTime != M2AgentNoonTime {
		t.Fatalf("unexpected Ada knowledge: %+v", adaKnowledge)
	}
	crossRead := adaRead
	crossRead.ObserverAgentID = M2AgentBoID
	if _, err := store.ReadAgentKnowledge(ctx, crossRead); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("Agent cross-knowledge read should fail, got %v", err)
	}
	escalated := adaRead
	escalated.Fields = append(escalated.Fields, "observation_id")
	if _, err := store.ReadAgentKnowledge(ctx, escalated); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("Agent field escalation should fail, got %v", err)
	}
	creatorRead := adaRead
	creatorRead.PrincipalID = "principal_creator"
	creatorRead.Fields = []string{"claim_key", "subject_agent_id", "place_id", "learned_world_time", "source_event_id", "observation_id"}
	creatorKnowledge, err := store.ReadAgentKnowledge(ctx, creatorRead)
	if err != nil {
		t.Fatal(err)
	}
	if len(creatorKnowledge.Facts) != 1 || creatorKnowledge.Facts[0].ObservationID == "" {
		t.Fatalf("creator evidence view is incomplete: %+v", creatorKnowledge)
	}

	before := agentReadOnlyCounts(t, ctx, store)
	adaEncounter, err := store.ResolveEncounter(ctx, emptyRequest)
	if err != nil {
		t.Fatal(err)
	}
	boRequest := emptyRequest
	boRequest.PrincipalID = M2AgentBoPrincipal
	boRequest.ObserverAgentID = M2AgentBoID
	boEncounter, err := store.ResolveEncounter(ctx, boRequest)
	if err != nil {
		t.Fatal(err)
	}
	after := agentReadOnlyCounts(t, ctx, store)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("encounter queries wrote state: before=%v after=%v", before, after)
	}
	if adaEncounter.PlaceID != M2AgentCafeID || boEncounter.PlaceID != M2AgentCafeID || adaEncounter.WorldTime != boEncounter.WorldTime || adaEncounter.WorldTime != M2AgentNoonTime {
		t.Fatalf("co-located views disagree on place/time: Ada=%+v Bo=%+v", adaEncounter, boEncounter)
	}
	boAlias, err := rpAnonymousEntityIDForTest(ctx, store, M2DemoInstanceID, M2DemoBranchID, M2AgentAdaID, M2AgentBoID)
	if err != nil {
		t.Fatal(err)
	}
	adaAlias, err := rpAnonymousEntityIDForTest(ctx, store, M2DemoInstanceID, M2DemoBranchID, M2AgentBoID, M2AgentAdaID)
	if err != nil {
		t.Fatal(err)
	}
	if len(adaEncounter.Participants) != 1 || adaEncounter.Participants[0].AgentID != boAlias || len(boEncounter.Participants) != 1 || boEncounter.Participants[0].AgentID != adaAlias {
		t.Fatalf("co-located views disagree on participants: Ada=%+v Bo=%+v", adaEncounter, boEncounter)
	}
}

func TestM2AgentReplaySnapshotAndProjectionRepair(t *testing.T) {
	ctx := context.Background()
	store := openM2AgentStore(t, ctx, filepath.Join(t.TempDir(), "agent-replay.db"), false)
	defer store.Close()
	if _, err := store.RunAgentLife(ctx, M2AgentNoonTime, 3); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateSnapshot(ctx, M2DemoInstanceID, M2DemoBranchID, 7); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RunAgentLife(ctx, M2AgentNoonTime, 1); err != nil {
		t.Fatal(err)
	}
	full, err := store.Replay(ctx, M2DemoInstanceID, M2DemoBranchID, 8)
	if err != nil {
		t.Fatal(err)
	}
	fromSnapshot, err := store.ReplayFromLatestSnapshot(ctx, M2DemoInstanceID, M2DemoBranchID, 8)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(full.State, fromSnapshot.State) || full.StateHash != fromSnapshot.StateHash || fromSnapshot.UsedSnapshotID == "" {
		t.Fatalf("Agent snapshot replay differs: full=%+v snapshot=%+v", full, fromSnapshot)
	}
	if len(full.State.AgentPositions) != 2 || len(full.State.AgentKnowledge) != 2 {
		t.Fatalf("Agent authority missing from replay: %+v", full.State)
	}
	for _, position := range full.State.AgentPositions {
		if position.PlaceID != M2AgentCafeID || position.EffectiveWorldTime != M2AgentNoonTime {
			t.Fatalf("unexpected replayed Agent position: %+v", position)
		}
	}
	if differences, err := store.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatalf("healthy Agent projections differ: differences=%v err=%v", differences, err)
	}
	authorityBefore := agentAuthorityCounts(t, ctx, store)
	if _, err := store.db.ExecContext(ctx, `UPDATE agent_positions SET place_id = 'place_m2_work_ada' WHERE agent_id = ?`, M2AgentAdaID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE agent_knowledge SET place_id = 'place_m2_work_bo' WHERE observer_agent_id = ?`, M2AgentAdaID); err != nil {
		t.Fatal(err)
	}
	differences, err := store.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID)
	if err != nil {
		t.Fatal(err)
	}
	positionDifference, knowledgeDifference := false, false
	for _, difference := range differences {
		positionDifference = positionDifference || (difference.Projection == "agent_position" && difference.Key == M2AgentAdaID)
		knowledgeDifference = knowledgeDifference || (difference.Projection == "agent_knowledge")
	}
	if !positionDifference || !knowledgeDifference {
		t.Fatalf("Agent projection corruption was not fully detected: %+v", differences)
	}
	if err := store.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	if differences, err := store.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatalf("Agent projection repair incomplete: differences=%v err=%v", differences, err)
	}
	if authorityAfter := agentAuthorityCounts(t, ctx, store); !reflect.DeepEqual(authorityBefore, authorityAfter) {
		t.Fatalf("projection repair changed Agent authority: before=%v after=%v", authorityBefore, authorityAfter)
	}
}

func openM2AgentStore(t *testing.T, ctx context.Context, path string, reverseSeed bool) *Store {
	t.Helper()
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	store.reverseAgentSeed = reverseSeed
	if _, err := store.BootstrapM2AgentDemo(ctx); err != nil {
		store.Close()
		t.Fatal(err)
	}
	return store
}

func prepareM2AgentMaterializations(t *testing.T, ctx context.Context, store *Store) {
	t.Helper()
	if err := store.BootstrapM2Demo(ctx); err != nil {
		t.Fatal(err)
	}
	commands := []core.MaterializeCohortCommand{
		m2AgentMaterialization("ada", M2AgentAdaID, "Ada", 1, 500, 5, 100, 75, 1, "2026-09-22T01:00:00Z"),
		m2AgentMaterialization("bo", M2AgentBoID, "Bo", 1, 600, 6, 120, 90, 2, "2026-09-22T01:01:00Z"),
	}
	for _, command := range commands {
		if _, err := store.MaterializeCohort(ctx, command); err != nil {
			t.Fatal(err)
		}
	}
}

func assertAgentSetupState(t *testing.T, ctx context.Context, store *Store) {
	t.Helper()
	checks := []struct {
		query    string
		args     []any
		expected int64
	}{
		{`SELECT head_sequence FROM branches WHERE instance_id = ? AND branch_id = ?`, []any{M2DemoInstanceID, M2DemoBranchID}, 4},
		{`SELECT COUNT(*) FROM events WHERE instance_id = ? AND branch_id = ?`, []any{M2DemoInstanceID, M2DemoBranchID}, 4},
		{`SELECT COUNT(*) FROM agent_profiles WHERE instance_id = ? AND branch_id = ?`, []any{M2DemoInstanceID, M2DemoBranchID}, 2},
		{`SELECT COUNT(*) FROM agent_schedule_entries WHERE status = 'active'`, nil, 4},
		{`SELECT COUNT(*) FROM agent_positions`, nil, 2},
		{`SELECT COUNT(*) FROM agent_movements WHERE movement_kind = 'initialize'`, nil, 2},
		{`SELECT population_count FROM cohorts WHERE cohort_id = ?`, []any{M2DemoCohortID}, 18},
		{`SELECT COUNT(*) FROM capability_grants WHERE capability_id IN ('world.agent.run', 'world.agent.knowledge.read', 'world.encounter.read') AND instance_id = ? AND branch_id = ?`, []any{M2DemoInstanceID, M2DemoBranchID}, 7},
	}
	for _, check := range checks {
		assertM2Value(t, ctx, store, check.query, check.args, check.expected)
	}
	assertPostedJournalBalanced(t, ctx, store)
	assertSQLiteHealthy(t, ctx, store)
}

func agentMovementOrder(t *testing.T, ctx context.Context, store *Store) []string {
	t.Helper()
	rows, err := store.db.QueryContext(ctx, `
		SELECT m.agent_id FROM agent_movements m JOIN events e ON e.event_id = m.event_id
		WHERE e.instance_id = ? AND e.branch_id = ? AND m.movement_kind = 'scheduled'
		ORDER BY e.event_sequence`, M2DemoInstanceID, M2DemoBranchID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var result []string
	for rows.Next() {
		var agentID string
		if err := rows.Scan(&agentID); err != nil {
			t.Fatal(err)
		}
		result = append(result, agentID)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return result
}

func agentReadOnlyCounts(t *testing.T, ctx context.Context, store *Store) []int64 {
	t.Helper()
	queries := []string{
		`SELECT COUNT(*) FROM commands WHERE instance_id = 'inst_m2_t09' AND branch_id = 'br_main'`,
		`SELECT COUNT(*) FROM events WHERE instance_id = 'inst_m2_t09' AND branch_id = 'br_main'`,
		`SELECT COUNT(*) FROM audit_records WHERE instance_id = 'inst_m2_t09' AND branch_id = 'br_main'`,
		`SELECT COUNT(*) FROM observation_records`,
		`SELECT COUNT(*) FROM agent_knowledge`,
	}
	counts := make([]int64, len(queries))
	for index, query := range queries {
		if err := store.db.QueryRowContext(ctx, query).Scan(&counts[index]); err != nil {
			t.Fatal(err)
		}
	}
	return counts
}

func agentAuthorityCounts(t *testing.T, ctx context.Context, store *Store) []int64 {
	t.Helper()
	queries := []string{
		`SELECT COUNT(*) FROM events WHERE instance_id = 'inst_m2_t09' AND branch_id = 'br_main'`,
		`SELECT COUNT(*) FROM agent_movements`,
		`SELECT COUNT(*) FROM observation_records`,
		`SELECT COUNT(*) FROM commands WHERE instance_id = 'inst_m2_t09' AND branch_id = 'br_main'`,
	}
	counts := make([]int64, len(queries))
	for index, query := range queries {
		if err := store.db.QueryRowContext(ctx, query).Scan(&counts[index]); err != nil {
			t.Fatal(err)
		}
	}
	return counts
}
