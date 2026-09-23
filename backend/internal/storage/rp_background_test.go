package storage

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"corerp.local/backend/internal/core"
)

func newBackgroundCandidate(t *testing.T, ctx context.Context, s *Store) core.RPBackgroundRequest {
	t.Helper()
	if _, err := s.PrepareRPTravel(ctx); err != nil {
		t.Fatal(err)
	}
	command := m2AgentMaterialization("emergent_nora", "entity_emergent_nora", "Nora", 1, 200, 1, 40, 30, 8, m2RPSetupTime)
	if _, err := s.MaterializeCohort(ctx, command); err != nil {
		t.Fatal(err)
	}
	return core.RPBackgroundRequest{PrincipalID: "principal_creator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: command.EntityID, ExpectedHead: 9, IdempotencyKey: "nora-background", AgeMin: 25, AgeMax: 34, ResidencePlaceID: "place_m2_home_bo", InitialPlaceID: M2AgentCafeID, Schedule: []core.RPBackgroundSchedule{
		{WorldTime: "2026-09-22T03:00:00Z", PlaceID: "place_m2_home_bo", ActivityCode: "home"},
		{WorldTime: "2026-09-22T04:00:00Z", PlaceID: M2AgentCafeID, ActivityCode: "present"},
	}}
}

func TestRPBackgroundMaterializationEntersRPReencountersAndReopens(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "background.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Close() }()
	r := newBackgroundCandidate(t, ctx, s)
	result, err := s.MaterializeRPBackground(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := s.MaterializeRPBackground(ctx, r)
	if err != nil || !retry.Replayed || !reflect.DeepEqual(result.Background, retry.Background) {
		t.Fatalf("retry %+v %v", retry, err)
	}
	if result.Background.DisplayName != "Nora" || result.Background.MaterializationEventID == "" || result.Background.ResidenceSourceEventID == "" {
		t.Fatal("missing existing world evidence")
	}
	session, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: M2RPPlayerPrincipal, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2RPPlayerID, POV: "second_person", IdempotencyKey: "meet-nora"})
	if err != nil {
		t.Fatal(err)
	}
	read := core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID}
	view, err := s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	seen := func(view RPObservation) bool {
		for _, p := range view.PresentEntities {
			if p.EntityID == r.EntityID {
				return true
			}
		}
		return false
	}
	if !seen(view) {
		t.Fatal("new materialized individual not present")
	}
	turn, err := s.PlayRPTurn(ctx, core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, Text: "你好", IdempotencyKey: "nora-first"})
	if err != nil {
		t.Fatal(err)
	}
	if turn.SettledSequence <= result.EventSequence {
		t.Fatal("no committed RP")
	}
	for i, at := range []string{"2026-09-22T03:00:00Z", "2026-09-22T04:00:00Z"} {
		view, err = s.ObserveRPSession(ctx, read)
		if err != nil {
			t.Fatal(err)
		}
		_, err = s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: at, Budget: 10, IdempotencyKey: at})
		if err != nil {
			t.Fatal(err)
		}
		view, err = s.ObserveRPSession(ctx, read)
		if err != nil {
			t.Fatal(err)
		}
		if seen(view) != (i == 1) {
			t.Fatalf("schedule did not move same individual at %s", at)
		}
	}
	speech, err := s.SpeakRP(ctx, core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, Text: "又见面了", IdempotencyKey: "nora-again"})
	if err != nil {
		t.Fatal(err)
	}
	decision := core.RPDecisionRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, TurnID: speech.TurnID, NPCEntityID: r.EntityID}
	input, err := s.BuildRPDecisionInput(ctx, decision)
	if err != nil {
		t.Fatal(err)
	}
	if input.Life.Background == nil || !reflect.DeepEqual(*input.Life.Background, result.Background) || len(input.Life.SalientMemories) < 2 {
		t.Fatalf("identity or experiences lost %+v", input.Life)
	}
	if len(input.Life.Employment) != 0 {
		t.Fatal("invented a Cohort job")
	}
	s.Close()
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	restored, err := s.BuildRPDecisionInput(ctx, decision)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(input.Life, restored.Life) {
		t.Fatal("identity/life changed after reopen and rebuild")
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM materialized_entities WHERE entity_id=?`, []any{r.EntityID}, 1)
}

func TestRPBackgroundRejectsUnauthorizedConflictAndRollsBack(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "background.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	r := newBackgroundCandidate(t, ctx, s)
	unauthorized := r
	unauthorized.PrincipalID = M2RPPlayerPrincipal
	if _, err := s.MaterializeRPBackground(ctx, unauthorized); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("unauthorized %v", err)
	}
	bad := r
	bad.ResidencePlaceID = M2AgentCafeID
	if _, err := s.MaterializeRPBackground(ctx, bad); err == nil {
		t.Fatal("public place accepted as home")
	}
	s.beforeCommit = func() error { return errors.New("injected background failure") }
	if _, err := s.MaterializeRPBackground(ctx, r); err == nil {
		t.Fatal("missing rollback")
	}
	s.beforeCommit = nil
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_profiles WHERE agent_id=?`, []any{r.EntityID}, 0)
	assertM2Value(t, ctx, s, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, []any{r.InstanceID, r.BranchID}, 9)
	if _, err := s.MaterializeRPBackground(ctx, r); err != nil {
		t.Fatal(err)
	}
	bad = r
	bad.AgeMin = 26
	if _, err := s.MaterializeRPBackground(ctx, bad); !core.HasCode(err, core.CodeIdempotencyMismatch) {
		t.Fatalf("changed retry %v", err)
	}
	bad.IdempotencyKey = "overwrite-background"
	bad.ExpectedHead = 10
	if _, err := s.MaterializeRPBackground(ctx, bad); !core.HasCode(err, core.CodeMaterializationConflict) {
		t.Fatalf("redefinition %v", err)
	}
}

func TestRPBackgroundPreservesRealEmploymentAndCalendar(t *testing.T) {
	ctx := context.Background()
	s := openM2AgentStore(t, ctx, filepath.Join(t.TempDir(), "background-worker.db"), false)
	defer s.Close()
	if _, err := s.PrepareM2EconomicDemo(ctx); err != nil {
		t.Fatal(err)
	}
	run, err := s.RunAgentLife(ctx, M2AgentNoonTime, 100)
	if err != nil {
		t.Fatal(err)
	}
	command := m2AgentMaterialization("background_worker", "entity_background_worker", "Worker", 1, 0, 0, 0, 0, run.HeadSequence, M2AgentNoonTime)
	materialized, err := s.MaterializeCohort(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	// Topology fixture only: employment/population/money still come from real
	// economy preparation, materialization and wage-split commands above.
	for i, link := range []rpPlaceLink{{M2AgentCafeID, "place_m2_work_bo"}, {"place_m2_work_bo", M2AgentCafeID}} {
		if _, err := s.db.ExecContext(ctx, `INSERT INTO rp_place_links(link_id,instance_id,branch_id,from_place_id,to_place_id,definition_event_id) VALUES (?,?,?,?,?,?)`, []string{"test-worker-out", "test-worker-back"}[i], M2DemoInstanceID, M2DemoBranchID, link.From, link.To, m2AgentSetupEventID); err != nil {
			t.Fatal(err)
		}
	}
	r := core.RPBackgroundRequest{PrincipalID: "principal_creator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: command.EntityID, ExpectedHead: materialized.LastSequence, IdempotencyKey: "worker-background", AgeMin: 20, AgeMax: 29, ResidencePlaceID: "place_m2_home_bo", InitialPlaceID: M2AgentCafeID, Schedule: []core.RPBackgroundSchedule{
		{WorldTime: "2026-09-24T08:00:00Z", PlaceID: "place_m2_work_bo", ActivityCode: "work", EmploymentContractID: m2EconomyContractID},
		{WorldTime: "2026-09-24T12:00:00Z", PlaceID: M2AgentCafeID, ActivityCode: "lunch"},
	}}
	bad := r
	bad.Schedule = append([]core.RPBackgroundSchedule(nil), r.Schedule...)
	bad.Schedule[0].EmploymentContractID = "unrelated-contract"
	if _, err := s.MaterializeRPBackground(ctx, bad); !core.HasCode(err, core.CodeInvalidArgument) {
		t.Fatalf("invented employment accepted: %v", err)
	}
	background, err := s.MaterializeRPBackground(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	jobs := background.Background.InitialEmployment
	if len(jobs) != 1 || jobs[0].ContractID != m2EconomyContractID || jobs[0].WageMinor != 10 || jobs[0].SourceEventID != materialized.EventID {
		t.Fatalf("missing own wage lineage %+v", jobs)
	}
	if _, err := s.RunAgentLife(ctx, "2026-09-24T08:00:00Z", 100); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT current_day FROM world_clocks WHERE instance_id=? AND branch_id=?`, []any{M2DemoInstanceID, M2DemoBranchID}, 2)
	conn, err := s.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	life, err := buildRPLifeContext(ctx, conn, core.RPDecisionInput{NPCEntityID: r.EntityID, InstanceID: r.InstanceID, BranchID: r.BranchID, WorldTime: "2026-09-24T08:00:00Z", OwnAssetMinor: 10, GoalCode: "keep_daily_routine"})
	conn.Close()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(life.Employment, jobs) {
		t.Fatalf("job lost in actual Life Context %+v", life.Employment)
	}
	found := false
	for _, relation := range life.Relationships {
		if relation.Role == "employee" && relation.SubjectEntityID == jobs[0].OrganizationID && relation.SourceEventIDs[0] == materialized.EventID {
			found = true
		}
	}
	if !found {
		t.Fatal("actual organizational relationship missing")
	}
	if differences, err := s.CompareProjections(ctx, r.InstanceID, r.BranchID); err != nil || len(differences) != 0 {
		t.Fatalf("worker replay differs %v %v", differences, err)
	}
}
