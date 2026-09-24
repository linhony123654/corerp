package storage

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPTransitScheduledDelayRepairsQueueAndArrivesOnce(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "transit-delay.db")
	s := openCareerTestWorld(t, path)
	defer func() { s.Close() }()
	works, err := s.DefineRPTransitWorks(ctx, TransitWorksRequest{Binding: careerTestBinding(t, s, "principal_creator", "roadworks"), FromPlaceID: M2AgentCafeID, ToPlaceID: "place_m2_work_ada", StartsAt: "2026-09-23T07:30:00Z", EndsAt: "2026-09-23T09:00:00Z"})
	if err != nil {
		t.Fatal(err)
	}
	if run, err := s.RunAgentLife(ctx, "2026-09-23T07:59:00Z", 100); err != nil || run.PendingDue != 0 {
		t.Fatalf("before work: %+v %v", run, err)
	}
	var scheduleID, originalItem, originalDefinition, originalTime, originalPlace string
	if err := s.db.QueryRowContext(ctx, `SELECT s.schedule_id,s.scheduler_item_id,s.definition_event_id,s.world_time,p.place_id FROM agent_schedule_entries s JOIN agent_positions p ON p.agent_id=s.agent_id WHERE s.agent_id=? AND s.world_time='2026-09-23T08:00:00Z'`, M2AgentAdaID).Scan(&scheduleID, &originalItem, &originalDefinition, &originalTime, &originalPlace); err != nil {
		t.Fatal(err)
	}
	s.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "delay rollback") }
	if _, err := s.executeNextAgentSchedule(ctx, "2026-09-23T08:00:00Z"); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatalf("delay rollback: %v", err)
	}
	s.beforeCommit = nil
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='AgentTravelDelayed'`, nil, 0)
	if run, err := s.RunAgentLife(ctx, "2026-09-23T08:30:00Z", 100); err != nil || run.PendingDue != 0 {
		t.Fatalf("delay due work: %+v %v", run, err)
	}
	var raw, delayID string
	if err := s.db.QueryRowContext(ctx, `SELECT event_id,payload FROM events WHERE event_type='AgentTravelDelayed' AND json_extract(payload,'$.schedule_id')=?`, scheduleID).Scan(&delayID, &raw); err != nil {
		t.Fatal(err)
	}
	var delay rpTransitDelay
	if err := json.Unmarshal([]byte(raw), &delay); err != nil {
		t.Fatal(err)
	}
	if delay.Retry.WorldTime != "2026-09-23T09:00:00Z" || delay.OriginalWorldTime != originalTime || delay.ScheduleSourceEventID != originalDefinition || len(delay.DelaySourceEventIDs) != 1 || delay.DelaySourceEventIDs[0] != works.EventID || len(delay.Path) != 3 {
		t.Fatalf("delay lacks actual causal path: %+v", delay)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_positions WHERE agent_id=? AND place_id=?`, []any{M2AgentAdaID, originalPlace}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_movements WHERE event_id=?`, []any{delayID}, 0)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM journal_entries WHERE event_id=?`, []any{delayID}, 0)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM outbox WHERE event_id=?`, []any{delayID}, 0)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_schedule_entries WHERE schedule_id=? AND world_time=? AND definition_event_id=? AND scheduler_item_id=? AND status='active'`, []any{scheduleID, originalTime, originalDefinition, delay.Retry.ID}, 1)
	session, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: M2RPPlayerPrincipal, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2RPPlayerID, POV: "second_person", IdempotencyKey: "transit-player"})
	if err != nil {
		t.Fatal(err)
	}
	read := core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID}
	view, err := s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.MoveRP(ctx, core.RPMoveRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, FromPlaceID: view.PlaceID, ToPlaceID: "place_m2_work_ada", ExpectedCursor: view.ObservationCursor, IdempotencyKey: "blocked-player"}); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatalf("player bypassed same works: %v", err)
	}
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		t.Fatal(err)
	}
	npc, err := readRPOwnDecisionContext(ctx, tx.conn, core.RPDecisionInput{InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, NPCEntityID: M2RPNPCID, InterlocutorEntityID: M2RPPlayerID})
	if err != nil {
		tx.Rollback(ctx)
		t.Fatal(err)
	}
	for _, place := range npc.ReachablePlaceIDs {
		if place == "place_m2_work_ada" {
			tx.Rollback(ctx)
			t.Fatal("NPC candidate bypassed works")
		}
	}
	ada, err := readRPOwnDecisionContext(ctx, tx.conn, core.RPDecisionInput{InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, NPCEntityID: M2AgentAdaID, InterlocutorEntityID: M2RPPlayerID})
	tx.Rollback(ctx)
	if err != nil || ada.NextSchedule == nil || ada.NextSchedule.WorldTime != "2026-09-23T09:00:00Z" || ada.NextSchedule.OriginalWorldTime != originalTime || ada.NextSchedule.DelaySourceEventID != delayID {
		t.Fatalf("own delayed appointment time hidden: %+v %v", ada.NextSchedule, err)
	}
	if differences, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatalf("pending delay projections: %+v %v", differences, err)
	}
	// Damage only the isolated projection: repair must restore the real pending
	// arrival and must not turn the consumed original queue into a new action.
	if _, err := s.db.ExecContext(ctx, `UPDATE scheduler_items SET world_time='2026-09-23T10:00:00Z',status='cancelled' WHERE scheduler_item_id=?`, delay.Retry.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE agent_schedule_entries SET scheduler_item_id=?,world_time='2026-09-23T10:00:00Z' WHERE schedule_id=?`, originalItem, scheduleID); err != nil {
		t.Fatal(err)
	}
	if differences, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 2 {
		t.Fatalf("damage not detected: %+v %v", differences, err)
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
	if differences, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatalf("delay repair: %+v %v", differences, err)
	}
	if run, err := s.RunAgentLife(ctx, "2026-09-23T09:00:00Z", 100); err != nil || run.PendingDue != 0 {
		t.Fatalf("delayed arrival: %+v %v", run, err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_movements WHERE agent_id=? AND schedule_id=? AND world_time='2026-09-23T09:00:00Z' AND to_place_id='place_m2_work_ada'`, []any{M2AgentAdaID, scheduleID}, 1)
	view, err = s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.MoveRP(ctx, core.RPMoveRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, FromPlaceID: view.PlaceID, ToPlaceID: "place_m2_work_ada", ExpectedCursor: view.ObservationCursor, IdempotencyKey: "open-player"}); err != nil {
		t.Fatalf("player did not recover at expiry: %v", err)
	}
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RunAgentLife(ctx, "2026-09-23T09:00:00Z", 100); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_movements WHERE schedule_id=?`, []any{scheduleID}, 1)
	if differences, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatalf("completed delay repair: %+v %v", differences, err)
	}
}

func TestRPTransitLateArrivalDoesNotUndoLaterScheduledActivity(t *testing.T) {
	ctx := context.Background()
	s := openCareerTestWorld(t, filepath.Join(t.TempDir(), "transit-superseded.db"))
	defer s.Close()
	if _, err := s.DefineRPTransitWorks(ctx, TransitWorksRequest{Binding: careerTestBinding(t, s, "principal_creator", "long-works"), FromPlaceID: M2AgentCafeID, ToPlaceID: "place_m2_work_ada", StartsAt: "2026-09-23T07:30:00Z", EndsAt: "2026-09-23T13:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if run, err := s.RunAgentLife(ctx, "2026-09-23T13:00:00Z", 100); err != nil || run.PendingDue != 0 {
		t.Fatalf("superseded work: %+v %v", run, err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='AgentTravelSuperseded' AND json_extract(payload,'$.agent_id')=?`, []any{M2AgentAdaID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_positions WHERE agent_id=? AND place_id=? AND activity_code='lunch'`, []any{M2AgentAdaID, M2AgentCafeID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_movements WHERE agent_id=? AND to_place_id='place_m2_work_ada' AND world_time='2026-09-23T13:00:00Z'`, []any{M2AgentAdaID}, 0)
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	if differences, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatalf("superseded projection: %+v %v", differences, err)
	}
}

func TestRPTransitPendingAppointmentsUseOriginalChronology(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "transit-pending.db")
	s := openCareerTestWorld(t, path)
	defer func() { s.Close() }()
	if _, err := s.DefineRPTransitWorks(ctx, TransitWorksRequest{Binding: careerTestBinding(t, s, "principal_creator", "pending-works"), FromPlaceID: "place_m2_home_ada", ToPlaceID: M2AgentCafeID, StartsAt: "2026-09-23T07:30:00Z", EndsAt: "2026-09-23T13:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RunAgentLife(ctx, "2026-09-23T12:30:00Z", 100); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='AgentTravelDelayed' AND json_extract(payload,'$.agent_id')=?`, []any{M2AgentAdaID}, 2)
	// Corrupt only an isolated projection. A pending intent cannot cancel
	// another appointment unless its queue still matches accepted evidence.
	lunch := readTransitTestDelay(t, ctx, s, M2AgentAdaID)
	if lunch.OriginalWorldTime != "2026-09-23T12:00:00Z" {
		t.Fatalf("expected pending lunch: %+v", lunch)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE scheduler_items SET payload='{}' WHERE scheduler_item_id=?`, lunch.Retry.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RunAgentLife(ctx, "2026-09-23T13:00:00Z", 100); !core.HasCode(err, core.CodeProjectionDiverged) {
		t.Fatalf("unverified later intent accepted: %v", err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='AgentTravelSuperseded'`, nil, 0)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	var err error
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	s.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "supersession rollback") }
	if _, err := s.RunAgentLife(ctx, "2026-09-23T13:00:00Z", 100); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatalf("rollback: %v", err)
	}
	s.beforeCommit = nil
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='AgentTravelSuperseded'`, nil, 0)
	if run, err := s.RunAgentLife(ctx, "2026-09-23T13:00:00Z", 100); err != nil || run.PendingDue != 0 {
		t.Fatalf("pending arrivals: %+v %v", run, err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_movements WHERE agent_id=? AND to_place_id='place_m2_work_ada' AND world_time='2026-09-23T13:00:00Z'`, []any{M2AgentAdaID}, 0)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_positions WHERE agent_id=? AND place_id=? AND activity_code='lunch'`, []any{M2AgentAdaID, M2AgentCafeID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='AgentTravelSuperseded' AND json_extract(payload,'$.agent_id')=?`, []any{M2AgentAdaID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='AgentTravelSuperseded' AND json_extract(payload,'$.later_schedule_id')=? AND json_extract(payload,'$.later_delay_event_id')=?`, []any{lunch.ScheduleID, "event_" + lunch.Previous.ID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_schedule_entries a JOIN events e ON json_extract(e.payload,'$.schedule_id')=a.schedule_id WHERE e.event_type='AgentTravelDelayed' AND a.agent_id=? AND a.world_time=json_extract(e.payload,'$.original_world_time') AND a.definition_event_id=json_extract(e.payload,'$.schedule_source_event_id')`, []any{M2AgentAdaID}, 2)
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RunAgentLife(ctx, "2026-09-23T13:00:00Z", 100); err != nil {
		t.Fatal(err)
	}
	if differences, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatalf("pending supersession recovery: %+v %v", differences, err)
	}
}
