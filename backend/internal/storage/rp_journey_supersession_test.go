package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"corerp.local/backend/internal/core"
)

func TestRPJourneyLaterAppointmentWaitsUntilRealArrivalAfterRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "journey-appointment.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Close() }()
	_, read, view := newRPWaitTestSession(t, ctx, s)
	base, _ := time.Parse(time.RFC3339, view.WorldTime)
	segment, err := s.MaterializeRPLocation(ctx, RPLocationMaterializeRequest{Binding: core.CareerBinding{PrincipalID: "principal_creator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: view.ObservationCursor, IdempotencyKey: "appointment-segment"}, ParentLocationID: M2AgentCafeID, SlotKey: "appointment-road", Candidate: RPLocationCandidate{DisplayName: "旅程路段", GeneratorVersion: "local-v1"}})
	if err != nil {
		t.Fatal(err)
	}
	edge, err := s.DefineRPTimedEdge(ctx, RPTimedEdgeRequest{Binding: core.CareerBinding{PrincipalID: "principal_creator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: segment.EventSequence, IdempotencyKey: "appointment-edge"}, FromPlaceID: M2AgentCafeID, ToPlaceID: "place_m2_home_ada", SegmentPlaceID: segment.Fact.LocationID, DurationMinutes: 15})
	if err != nil {
		t.Fatal(err)
	}
	type appointment struct{ schedule, item, due, to string }
	appointments := []appointment{{"schedule_test_early_trip", "sched_test_early_trip", base.Add(5 * time.Minute).Format(time.RFC3339), "place_m2_home_ada"}, {"schedule_test_later_visit", "sched_test_later_visit", base.Add(10 * time.Minute).Format(time.RFC3339), M2AgentCafeID}}
	_, err = executePrivateFactCommand(s, ctx, core.CareerBinding{PrincipalID: "principal_creator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: edge.EventSequence, IdempotencyKey: "two-appointments"}, "TestTwoJourneyAppointments", appointments, privateFactDomain{"test_two_appointments", "TestTwoAppointmentsDefined", `{"authorization":"test-fixture"}`}, func(conn *sql.Conn) error { return nil }, func(conn *sql.Conn, c privateFactContext) ([]appointment, func() error, error) {
		return appointments, func() error {
			for _, a := range appointments {
				payload := agentSchedulePayload{Kind: "agent_move", Day: 0, AgentID: M2RPNPCID, ScheduleID: a.schedule, ToPlaceID: a.to, ActivityCode: "visit"}
				encoded, err := core.CanonicalJSON(payload)
				if err != nil {
					return err
				}
				if err := execAgentOne(ctx, conn, "test appointment queue", `INSERT INTO scheduler_items(scheduler_item_id,instance_id,branch_id,world_time,phase_id,declared_priority,status,payload) VALUES (?,?,?,?,?,1,'pending',?)`, a.item, M2DemoInstanceID, M2DemoBranchID, a.due, m2AgentPhaseID, string(encoded)); err != nil {
					return err
				}
				if err := execAgentOne(ctx, conn, "test appointment", `INSERT INTO agent_schedule_entries(schedule_id,agent_id,world_time,place_id,activity_code,declared_priority,scheduler_item_id,status,definition_event_id) VALUES (?,?,?,?, 'visit',1,?,'active',?)`, a.schedule, M2RPNPCID, a.due, a.to, a.item, c.EventID); err != nil {
					return err
				}
			}
			return nil
		}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	waitUntil := func(target, key string) {
		t.Helper()
		view, err = s.ObserveRPSession(ctx, read)
		if err != nil {
			t.Fatal(err)
		}
		out, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: target, Budget: 100, IdempotencyKey: key})
		if err != nil || out.Status != "completed" {
			t.Fatal("wait failed", out, err)
		}
	}
	waitUntil(appointments[0].due, "start-early-trip")
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_positions WHERE agent_id=? AND place_id=?`, []any{M2RPNPCID, segment.Fact.LocationID}, 1)
	waitUntil(appointments[1].due, "defer-later-visit")
	var raw string
	if err := s.db.QueryRowContext(ctx, `SELECT payload FROM events WHERE event_type='AgentTravelDelayed' AND json_extract(payload,'$.schedule_id')=?`, appointments[1].schedule).Scan(&raw); err != nil {
		t.Fatal("later appointment was not sourced as deferred", err)
	}
	var delay rpTransitDelay
	if err := json.Unmarshal([]byte(raw), &delay); err != nil {
		t.Fatal(err)
	}
	arrivalAt := base.Add(20 * time.Minute).Format(time.RFC3339)
	resumeAt := base.Add(21 * time.Minute).Format(time.RFC3339)
	if delay.Reason != "journey_occupancy" || delay.Retry.WorldTime != resumeAt || len(delay.DelaySourceEventIDs) != 1 {
		t.Fatal("wrong occupancy deferral", delay)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_positions WHERE agent_id=? AND place_id=?`, []any{M2RPNPCID, segment.Fact.LocationID}, 1)
	if diffs, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diffs) != 0 {
		t.Fatal("deferred appointment projection", diffs, err)
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
	waitUntil(arrivalAt, "arrive-before-later-visit")
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_positions WHERE agent_id=? AND place_id=?`, []any{M2RPNPCID, "place_m2_home_ada"}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_schedule_entries WHERE schedule_id=? AND status='active'`, []any{appointments[1].schedule}, 1)
	waitUntil(resumeAt, "later-visit-after-arrival")
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_positions WHERE agent_id=? AND place_id=?`, []any{M2RPNPCID, M2AgentCafeID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_occupancy_intervals WHERE agent_id=? AND location_id=? AND entered_at=? AND exited_at=?`, []any{M2RPNPCID, segment.Fact.LocationID, appointments[0].due, arrivalAt}, 1)
	if diffs, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diffs) != 0 {
		t.Fatal("resumed appointment projection", diffs, err)
	}
}
