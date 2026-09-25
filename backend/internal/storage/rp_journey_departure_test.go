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

func TestRPJourneyScheduledTimedDepartureWaitsForDirectWorksEvenWithAlternatePath(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "timed-departure.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, read, view := newRPWaitTestSession(t, ctx, s)
	base, _ := time.Parse(time.RFC3339, view.WorldTime)
	segment, err := s.MaterializeRPLocation(ctx, RPLocationMaterializeRequest{Binding: core.CareerBinding{PrincipalID: "principal_creator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: view.ObservationCursor, IdempotencyKey: "blocked-segment"}, ParentLocationID: M2AgentCafeID, SlotKey: "blocked-departure", Candidate: RPLocationCandidate{DisplayName: "封路路段", GeneratorVersion: "local-v1"}})
	if err != nil {
		t.Fatal(err)
	}
	edge, err := s.DefineRPTimedEdge(ctx, RPTimedEdgeRequest{Binding: core.CareerBinding{PrincipalID: "principal_creator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: segment.EventSequence, IdempotencyKey: "blocked-edge"}, FromPlaceID: M2AgentCafeID, ToPlaceID: "place_m2_home_ada", SegmentPlaceID: segment.Fact.LocationID, DurationMinutes: 15})
	if err != nil {
		t.Fatal(err)
	}
	works, err := s.DefineRPTransitWorks(ctx, TransitWorksRequest{Binding: core.CareerBinding{PrincipalID: "principal_creator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: edge.EventSequence, IdempotencyKey: "departure-works"}, FromPlaceID: M2AgentCafeID, ToPlaceID: "place_m2_home_ada", StartsAt: base.Add(time.Minute).Format(time.RFC3339), EndsAt: base.Add(30 * time.Minute).Format(time.RFC3339)})
	if err != nil {
		t.Fatal(err)
	}
	// Test-only alternate durationless route: RP5 can find it, but a declared
	// timed direct journey may not silently take that path or skip its segment.
	alt, err := executePrivateFactCommand(s, ctx, core.CareerBinding{PrincipalID: "principal_creator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: works.EventSequence, IdempotencyKey: "alternate-route"}, "TestAlternativeRoute", struct{}{}, privateFactDomain{"test_alternate_route", "TestAlternativeRouteDefined", `{"authorization":"test-fixture"}`}, func(conn *sql.Conn) error { return nil }, func(conn *sql.Conn, c privateFactContext) (struct{}, func() error, error) {
		return struct{}{}, func() error {
			return execAgentOne(ctx, conn, "test alternate link", `INSERT INTO rp_place_links(link_id,instance_id,branch_id,from_place_id,to_place_id,definition_event_id) VALUES ('test_alt_work_to_home',?,?,?,?,?)`, M2DemoInstanceID, M2DemoBranchID, "place_m2_work_ada", "place_m2_home_ada", c.EventID)
		}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	const scheduleID, itemID = "schedule_test_blocked_npc", "sched_test_blocked_npc"
	due := base.Add(5 * time.Minute).Format(time.RFC3339)
	payload := agentSchedulePayload{Kind: "agent_move", Day: 0, AgentID: M2RPNPCID, ScheduleID: scheduleID, ToPlaceID: "place_m2_home_ada", ActivityCode: "visit"}
	encoded, err := core.CanonicalJSON(payload)
	if err != nil {
		t.Fatal(err)
	}
	_, err = executePrivateFactCommand(s, ctx, core.CareerBinding{PrincipalID: "principal_creator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: alt.EventSequence, IdempotencyKey: "blocked-npc-schedule"}, "TestBlockedNPCSchedule", payload, privateFactDomain{"test_blocked_npc", "TestBlockedNPCScheduleDefined", `{"authorization":"test-fixture"}`}, func(conn *sql.Conn) error { return nil }, func(conn *sql.Conn, c privateFactContext) (agentSchedulePayload, func() error, error) {
		return payload, func() error {
			if err := execAgentOne(ctx, conn, "test blocked queue", `INSERT INTO scheduler_items(scheduler_item_id,instance_id,branch_id,world_time,phase_id,declared_priority,status,payload) VALUES (?,?,?,?,?,1,'pending',?)`, itemID, M2DemoInstanceID, M2DemoBranchID, due, m2AgentPhaseID, string(encoded)); err != nil {
				return err
			}
			return execAgentOne(ctx, conn, "test blocked appointment", `INSERT INTO agent_schedule_entries(schedule_id,agent_id,world_time,place_id,activity_code,declared_priority,scheduler_item_id,status,definition_event_id) VALUES (?,?,?,?, 'visit',1,?,'active',?)`, scheduleID, M2RPNPCID, due, payload.ToPlaceID, itemID, c.EventID)
		}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	view, err = s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: due, Budget: 100, IdempotencyKey: "wait-blocked-departure"}); err != nil {
		t.Fatal(err)
	}
	var raw string
	if err := s.db.QueryRowContext(ctx, `SELECT payload FROM events WHERE event_type='AgentTravelDelayed' AND json_extract(payload,'$.schedule_id')=?`, scheduleID).Scan(&raw); err != nil {
		t.Fatal("direct works did not delay NPC", err)
	}
	var delay rpTransitDelay
	if err := json.Unmarshal([]byte(raw), &delay); err != nil {
		t.Fatal(err)
	}
	if delay.Retry.WorldTime != works.Fact.Window.EndsAt || len(delay.Path) != 2 || len(delay.DelaySourceEventIDs) != 1 || delay.DelaySourceEventIDs[0] != works.EventID {
		t.Fatal("alternate path replaced pinned timed route", delay)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_positions WHERE agent_id=? AND place_id=?`, []any{M2RPNPCID, M2AgentCafeID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_journeys WHERE agent_id=?`, []any{M2RPNPCID}, 0)
	view, err = s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: works.Fact.Window.EndsAt, Budget: 100, IdempotencyKey: "wait-reopened-departure"}); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_positions WHERE agent_id=? AND place_id=?`, []any{M2RPNPCID, segment.Fact.LocationID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_journeys WHERE agent_id=? AND status='active'`, []any{M2RPNPCID}, 1)
	if diffs, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diffs) != 0 {
		t.Fatal("delayed timed departure not replayable", diffs, err)
	}
	view, err = s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	arrivalAt := base.Add(45 * time.Minute).Format(time.RFC3339)
	if _, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: arrivalAt, Budget: 100, IdempotencyKey: "wait-delayed-npc-arrival"}); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_positions WHERE agent_id=? AND place_id=?`, []any{M2RPNPCID, "place_m2_home_ada"}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_journeys WHERE agent_id=? AND status='arrived'`, []any{M2RPNPCID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_movements WHERE agent_id=? AND to_place_id=? AND world_time=?`, []any{M2RPNPCID, "place_m2_home_ada", arrivalAt}, 1)
	if diffs, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diffs) != 0 {
		t.Fatal("delayed NPC arrival not replayable", diffs, err)
	}
}
