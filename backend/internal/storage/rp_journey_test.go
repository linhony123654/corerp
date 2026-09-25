package storage

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"corerp.local/backend/internal/core"
)

func TestRPJourneyRealSegmentAndScheduledArrival(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "timed-journey.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, read, view := newRPWaitTestSession(t, ctx, s)
	start, err := time.Parse(time.RFC3339, view.WorldTime)
	if err != nil {
		t.Fatal(err)
	}
	segment, err := s.MaterializeRPLocation(ctx, RPLocationMaterializeRequest{
		Binding:          core.CareerBinding{PrincipalID: "principal_creator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: view.ObservationCursor, IdempotencyKey: "timed-segment"},
		ParentLocationID: M2AgentCafeID, SlotKey: "cafe-ada-road",
		Candidate: RPLocationCandidate{DisplayName: "咖啡馆外的小路", GeneratorVersion: "local-v1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	edge, err := s.DefineRPTimedEdge(ctx, RPTimedEdgeRequest{
		Binding:     core.CareerBinding{PrincipalID: "principal_creator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: segment.EventSequence, IdempotencyKey: "cafe-ada-edge"},
		FromPlaceID: M2AgentCafeID, ToPlaceID: "place_m2_home_ada", SegmentPlaceID: segment.Fact.LocationID, DurationMinutes: 15,
	})
	if err != nil || edge.Fact.EdgeID == "" {
		t.Fatal(edge, err)
	}
	view, err = s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	seenEdge := false
	for _, place := range view.ReachablePlaces {
		if place.PlaceID == "place_m2_home_ada" {
			seenEdge = true
			if place.CanMoveNow || !place.CanStartJourney || place.TravelMinutes != 15 {
				t.Fatal("timed edge not reflected in observed route", place)
			}
		}
	}
	if !seenEdge {
		t.Fatal("timed edge destination missing")
	}
	request := core.RPMoveRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, FromPlaceID: view.PlaceID, ToPlaceID: "place_m2_home_ada", ExpectedCursor: view.ObservationCursor, IdempotencyKey: "actual-journey"}
	if _, err := s.MoveRP(ctx, request); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatal("immediate move bypassed timed route", err)
	}
	journey, err := s.StartRPJourney(ctx, request)
	if err != nil || journey.SegmentPlaceID != segment.Fact.LocationID || journey.ScheduledArrivalAt != start.Add(15*time.Minute).Format(time.RFC3339) {
		t.Fatal("journey start", journey, err)
	}
	if replayed, err := s.StartRPJourney(ctx, request); err != nil || !replayed.Replayed || replayed.JourneyID != journey.JourneyID {
		t.Fatal("journey start retry duplicated", replayed, err)
	}
	view, err = s.ObserveRPSession(ctx, read)
	if err != nil || view.PlaceID != segment.Fact.LocationID || len(view.PresentEntities) != 0 || view.ActiveJourney == nil || view.ActiveJourney.JourneyID != journey.JourneyID || view.ActiveJourney.ScheduledArrivalAt != journey.ScheduledArrivalAt {
		t.Fatal("traveler did not occupy a real empty segment", view, err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_occupancy_intervals WHERE agent_id=? AND location_id=? AND exited_at IS NULL`, []any{M2RPPlayerID, segment.Fact.LocationID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_positions WHERE agent_id=? AND place_id IN (?,?)`, []any{M2RPPlayerID, M2AgentCafeID, request.ToPlaceID}, 0)
	partway := start.Add(10 * time.Minute).Format(time.RFC3339)
	if _, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: partway, Budget: 100, IdempotencyKey: "partway"}); err != nil {
		t.Fatal(err)
	}
	view, err = s.ObserveRPSession(ctx, read)
	if err != nil || view.PlaceID != segment.Fact.LocationID || view.WorldTime != partway {
		t.Fatal("mid-journey presence not stable", view, err)
	}
	if _, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: journey.ScheduledArrivalAt, Budget: 100, IdempotencyKey: "arrival"}); err != nil {
		t.Fatal("scheduled arrival", err)
	}
	view, err = s.ObserveRPSession(ctx, read)
	if err != nil || view.PlaceID != request.ToPlaceID || view.WorldTime != journey.ScheduledArrivalAt || view.ActiveJourney != nil {
		t.Fatal("journey destination", view, err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_journeys WHERE journey_id=? AND status='arrived' AND resolved_event_id IS NOT NULL`, []any{journey.JourneyID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_occupancy_intervals WHERE agent_id=? AND location_id=? AND entered_at=? AND exited_at=?`, []any{M2RPPlayerID, segment.Fact.LocationID, journey.WorldTime, journey.ScheduledArrivalAt}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_occupancy_intervals WHERE agent_id=? AND location_id=? AND exited_at IS NULL`, []any{M2RPPlayerID, request.ToPlaceID}, 1)
	page, err := s.ReadRPEvents(ctx, RPEventsReadRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, After: edge.EventSequence, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	var started, arrived bool
	for _, event := range page.Events {
		if event.OwnAction == nil {
			continue
		}
		if event.OwnAction.JourneyID == journey.JourneyID && event.OwnAction.Kind == "journey_started" {
			started = true
		}
		if event.OwnAction.JourneyID == journey.JourneyID && event.OwnAction.Kind == "journey_arrived" {
			arrived = true
		}
	}
	if !started || !arrived {
		t.Fatal("own journey missing from filtered continuation", page.Events)
	}
	if diffs, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diffs) != 0 {
		t.Fatal("journey not replayable", diffs, err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE rp_timed_edges SET duration_minutes=1 WHERE edge_id=?`, edge.Fact.EdgeID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE rp_journeys SET scheduled_arrival_at=? WHERE journey_id=?`, start.Add(5*time.Minute).Format(time.RFC3339), journey.JourneyID); err != nil {
		t.Fatal(err)
	}
	if diffs, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diffs) != 2 {
		t.Fatal("timed-edge/journey corruption missed", diffs, err)
	}
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal("journey projection repair", err)
	}
	if diffs, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diffs) != 0 {
		t.Fatal("journey repair did not converge", diffs, err)
	}
	var arrivalItem, arrivalSchedule string
	if err := s.db.QueryRowContext(ctx, `SELECT arrival_item_id,arrival_schedule_id FROM rp_journeys WHERE journey_id=?`, journey.JourneyID).Scan(&arrivalItem, &arrivalSchedule); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE scheduler_items SET status='pending',payload='{}' WHERE scheduler_item_id=?`, arrivalItem); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE agent_schedule_entries SET status='active' WHERE schedule_id=?`, arrivalSchedule); err != nil {
		t.Fatal(err)
	}
	if diffs, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diffs) != 2 {
		t.Fatal("journey queue/schedule corruption missed", diffs, err)
	}
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal("journey queue repair", err)
	}
	if diffs, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diffs) != 0 {
		t.Fatal("journey queue repair did not converge", diffs, err)
	}
}

func TestRPJourneyScheduledNPCMeetsPlayerOnlyWhileBothAtSegment(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "midway-meeting.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, read, view := newRPWaitTestSession(t, ctx, s)
	start, _ := time.Parse(time.RFC3339, view.WorldTime)
	segment, err := s.MaterializeRPLocation(ctx, RPLocationMaterializeRequest{
		Binding:          core.CareerBinding{PrincipalID: "principal_creator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: view.ObservationCursor, IdempotencyKey: "shared-segment"},
		ParentLocationID: M2AgentCafeID, SlotKey: "shared-road", Candidate: RPLocationCandidate{DisplayName: "共行的小路", GeneratorVersion: "local-v1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	edge, err := s.DefineRPTimedEdge(ctx, RPTimedEdgeRequest{Binding: core.CareerBinding{PrincipalID: "principal_creator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: segment.EventSequence, IdempotencyKey: "shared-edge"}, FromPlaceID: M2AgentCafeID, ToPlaceID: "place_m2_home_ada", SegmentPlaceID: segment.Fact.LocationID, DurationMinutes: 15})
	if err != nil {
		t.Fatal(err)
	}
	var npcPlace string
	if err := s.db.QueryRowContext(ctx, `SELECT place_id FROM agent_positions WHERE agent_id=?`, M2RPNPCID).Scan(&npcPlace); err != nil || npcPlace != M2AgentCafeID {
		t.Fatal("NPC fixture is not at route origin", npcPlace, err)
	}
	const scheduleID, itemID = "schedule_test_midway_npc", "sched_test_midway_npc"
	arrivalAt := start.Add(5 * time.Minute).Format(time.RFC3339)
	payload := agentSchedulePayload{Kind: "agent_move", Day: int(start.Sub(time.Date(2026, 9, 22, 0, 0, 0, 0, time.UTC)) / (24 * time.Hour)), AgentID: M2RPNPCID, ScheduleID: scheduleID, ToPlaceID: "place_m2_home_ada", ActivityCode: "visit"}
	encoded, err := core.CanonicalJSON(payload)
	if err != nil {
		t.Fatal(err)
	}
	_, err = executePrivateFactCommand(s, ctx, core.CareerBinding{PrincipalID: "principal_creator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: edge.EventSequence, IdempotencyKey: "schedule-midway-npc"}, "TestScheduleMidwayNPC", payload,
		privateFactDomain{"test_midway_npc", "TestMidwayNPCScheduleDefined", `{"authorization":"test-fixture"}`},
		func(conn *sql.Conn) error {
			return authorizeRPLocationBuilder(ctx, conn, core.CareerBinding{PrincipalID: "principal_creator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID})
		},
		func(conn *sql.Conn, c privateFactContext) (agentSchedulePayload, func() error, error) {
			return payload, func() error {
				if err := execAgentOne(ctx, conn, "test NPC journey queue", `INSERT INTO scheduler_items(scheduler_item_id,instance_id,branch_id,world_time,phase_id,declared_priority,status,payload) VALUES (?,?,?,?,?,1,'pending',?)`, itemID, M2DemoInstanceID, M2DemoBranchID, arrivalAt, m2AgentPhaseID, string(encoded)); err != nil {
					return err
				}
				return execAgentOne(ctx, conn, "test NPC journey appointment", `INSERT INTO agent_schedule_entries(schedule_id,agent_id,world_time,place_id,activity_code,declared_priority,scheduler_item_id,status,definition_event_id) VALUES (?,?,?,?, 'visit',1,?,'active',?)`, scheduleID, M2RPNPCID, arrivalAt, payload.ToPlaceID, itemID, c.EventID)
			}, nil
		})
	if err != nil {
		t.Fatal(err)
	}
	view, err = s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	journey, err := s.StartRPJourney(ctx, core.RPMoveRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, FromPlaceID: view.PlaceID, ToPlaceID: payload.ToPlaceID, ExpectedCursor: view.ObservationCursor, IdempotencyKey: "player-midway"})
	if err != nil {
		t.Fatal(err)
	}
	view, err = s.ObserveRPSession(ctx, read)
	if err != nil || len(view.PresentEntities) != 0 {
		t.Fatal("pre-arrival encounter invented", view, err)
	}
	if _, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: arrivalAt, Budget: 100, IdempotencyKey: "meet-midway"}); err != nil {
		t.Fatal(err)
	}
	view, err = s.ObserveRPSession(ctx, read)
	if err != nil || view.PlaceID != segment.Fact.LocationID || len(view.PresentEntities) != 1 || view.PresentEntities[0].EntityID != M2RPNPCID {
		t.Fatal("real segment co-presence missing", view, err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_occupancy_intervals WHERE agent_id IN (?,?) AND location_id=? AND entered_at<=? AND (exited_at>? OR exited_at IS NULL)`, []any{M2RPPlayerID, M2RPNPCID, segment.Fact.LocationID, arrivalAt, arrivalAt}, 2)
	speech, err := s.SpeakRP(ctx, core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, IdempotencyKey: "real-midway-speech", Text: "路上见。"})
	if err != nil || len(speech.ListenerIDs) != 1 || speech.ListenerIDs[0] != M2RPNPCID {
		t.Fatal("midway speech did not use actual listener", speech, err)
	}
	view, err = s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: journey.ScheduledArrivalAt, Budget: 100, IdempotencyKey: "player-midway-arrival"}); err != nil {
		t.Fatal(err)
	}
	view, err = s.ObserveRPSession(ctx, read)
	alias, aliasErr := rpAnonymousEntityIDForTest(ctx, s, M2DemoInstanceID, M2DemoBranchID, M2RPPlayerID, M2AgentAdaID)
	if aliasErr != nil {
		t.Fatal(aliasErr)
	}
	if err != nil || view.PlaceID != payload.ToPlaceID || len(view.PresentEntities) != 1 || view.PresentEntities[0].EntityID != alias {
		t.Fatal("player arrived before NPC, wrong presence", view, err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_positions WHERE agent_id=? AND place_id=?`, []any{M2RPNPCID, segment.Fact.LocationID}, 1)
	if diffs, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diffs) != 0 {
		t.Fatal("midway encounter not replayable", diffs, err)
	}
}

func TestRPJourneyWorksDelayKeepsTravelerAtSegmentUntilRequeuedArrival(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "journey-delay.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	_, read, view := newRPWaitTestSession(t, ctx, s)
	start, _ := time.Parse(time.RFC3339, view.WorldTime)
	segment, err := s.MaterializeRPLocation(ctx, RPLocationMaterializeRequest{Binding: core.CareerBinding{PrincipalID: "principal_creator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: view.ObservationCursor, IdempotencyKey: "delay-segment"}, ParentLocationID: M2AgentCafeID, SlotKey: "delayed-road", Candidate: RPLocationCandidate{DisplayName: "施工中的小路", GeneratorVersion: "local-v1"}})
	if err != nil {
		t.Fatal(err)
	}
	edge, err := s.DefineRPTimedEdge(ctx, RPTimedEdgeRequest{Binding: core.CareerBinding{PrincipalID: "principal_creator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: segment.EventSequence, IdempotencyKey: "delay-edge"}, FromPlaceID: M2AgentCafeID, ToPlaceID: "place_m2_home_ada", SegmentPlaceID: segment.Fact.LocationID, DurationMinutes: 15})
	if err != nil {
		t.Fatal(err)
	}
	works, err := s.DefineRPTransitWorks(ctx, TransitWorksRequest{Binding: core.CareerBinding{PrincipalID: "principal_creator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: edge.EventSequence, IdempotencyKey: "delay-works"}, FromPlaceID: M2AgentCafeID, ToPlaceID: "place_m2_home_ada", StartsAt: start.Add(10 * time.Minute).Format(time.RFC3339), EndsAt: start.Add(30 * time.Minute).Format(time.RFC3339)})
	if err != nil {
		t.Fatal(err)
	}
	view, err = s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	journey, err := s.StartRPJourney(ctx, core.RPMoveRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, FromPlaceID: view.PlaceID, ToPlaceID: "place_m2_home_ada", ExpectedCursor: view.ObservationCursor, IdempotencyKey: "delay-player"})
	if err != nil {
		t.Fatal(err)
	}
	view, err = s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: start.Add(20 * time.Minute).Format(time.RFC3339), Budget: 100, IdempotencyKey: "wait-through-block"}); err != nil {
		t.Fatal(err)
	}
	view, err = s.ObserveRPSession(ctx, read)
	if err != nil || view.PlaceID != segment.Fact.LocationID {
		t.Fatal("obstruction teleported traveler", view, err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='RPJourneyDelayed' AND json_extract(payload,'$.journey_id')=? AND json_extract(payload,'$.works_source_event_ids[0]')=?`, []any{journey.JourneyID, works.EventID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_journeys WHERE journey_id=? AND status='active' AND scheduled_arrival_at=?`, []any{journey.JourneyID, start.Add(30 * time.Minute).Format(time.RFC3339)}, 1)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	view, err = s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: start.Add(30 * time.Minute).Format(time.RFC3339), Budget: 100, IdempotencyKey: "wait-unblocked"}); err != nil {
		t.Fatal(err)
	}
	view, err = s.ObserveRPSession(ctx, read)
	if err != nil || view.PlaceID != journey.ToPlaceID {
		t.Fatal("requeued journey did not arrive", view, err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='RPJourneyArrived' AND json_extract(payload,'$.journey_id')=?`, []any{journey.JourneyID}, 1)
	if diffs, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diffs) != 0 {
		t.Fatal("delayed journey not replayable", diffs, err)
	}
}

func TestRPJourneyCancelStopsArrivalWithoutTeleportAndCanReturn(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "journey-cancel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, read, view := newRPWaitTestSession(t, ctx, s)
	segment, err := s.MaterializeRPLocation(ctx, RPLocationMaterializeRequest{Binding: core.CareerBinding{PrincipalID: "principal_creator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: view.ObservationCursor, IdempotencyKey: "cancel-segment"}, ParentLocationID: M2AgentCafeID, SlotKey: "cancel-road", Candidate: RPLocationCandidate{DisplayName: "可回头的小路", GeneratorVersion: "local-v1"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DefineRPTimedEdge(ctx, RPTimedEdgeRequest{Binding: core.CareerBinding{PrincipalID: "principal_creator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: segment.EventSequence, IdempotencyKey: "cancel-edge"}, FromPlaceID: M2AgentCafeID, ToPlaceID: "place_m2_home_ada", SegmentPlaceID: segment.Fact.LocationID, DurationMinutes: 15}); err != nil {
		t.Fatal(err)
	}
	view, err = s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	journey, err := s.StartRPJourney(ctx, core.RPMoveRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, FromPlaceID: view.PlaceID, ToPlaceID: "place_m2_home_ada", ExpectedCursor: view.ObservationCursor, IdempotencyKey: "cancel-this-trip"})
	if err != nil {
		t.Fatal(err)
	}
	view, err = s.ObserveRPSession(ctx, read)
	if err != nil || view.PlaceID != segment.Fact.LocationID {
		t.Fatal(view, err)
	}
	if _, err := s.MoveRP(ctx, core.RPMoveRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, FromPlaceID: view.PlaceID, ToPlaceID: M2AgentCafeID, ExpectedCursor: view.ObservationCursor, IdempotencyKey: "cannot-return-active"}); !core.HasCode(err, core.CodeCommandInProgress) {
		t.Fatal("active journey escaped without cancellation", err)
	}
	cancelRequest := core.RPJourneyCancelRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, JourneyID: journey.JourneyID, ExpectedCursor: view.ObservationCursor, IdempotencyKey: "cancel-arrival"}
	cancelled, err := s.CancelRPJourney(ctx, cancelRequest)
	if err != nil || cancelled.Fact.JourneyID != journey.JourneyID {
		t.Fatal(cancelled, err)
	}
	if retry, err := s.CancelRPJourney(ctx, cancelRequest); err != nil || !retry.Replayed || retry.EventID != cancelled.EventID {
		t.Fatal("cancel retry duplicated", retry, err)
	}
	view, err = s.ObserveRPSession(ctx, read)
	if err != nil || view.PlaceID != segment.Fact.LocationID {
		t.Fatal("cancel teleported actor", view, err)
	}
	returnPossible := false
	for _, place := range view.ReachablePlaces {
		if place.PlaceID == M2AgentCafeID && place.CanMoveNow {
			returnPossible = true
		}
	}
	if !returnPossible {
		t.Fatal("cancelled traveler has no explicit return route", view.ReachablePlaces)
	}
	if _, err := s.MoveRP(ctx, core.RPMoveRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, FromPlaceID: view.PlaceID, ToPlaceID: M2AgentCafeID, ExpectedCursor: view.ObservationCursor, IdempotencyKey: "return-after-cancel"}); err != nil {
		t.Fatal(err)
	}
	view, err = s.ObserveRPSession(ctx, read)
	if err != nil || view.PlaceID != M2AgentCafeID {
		t.Fatal(view, err)
	}
	// Rerouting is explicit: cancel the pinned arrival, return along the
	// sourced segment link, then choose a different adjacent destination.
	if _, err := s.MoveRP(ctx, core.RPMoveRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, FromPlaceID: view.PlaceID, ToPlaceID: "place_m2_work_ada", ExpectedCursor: view.ObservationCursor, IdempotencyKey: "alternate-after-return"}); err != nil {
		t.Fatal("explicit alternate route", err)
	}
	view, err = s.ObserveRPSession(ctx, read)
	if err != nil || view.PlaceID != "place_m2_work_ada" {
		t.Fatal("reroute did not occupy chosen alternative", view, err)
	}
	if _, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: journey.ScheduledArrivalAt, Budget: 100, IdempotencyKey: "past-cancelled-arrival"}); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='RPJourneyArrived' AND json_extract(payload,'$.journey_id')=?`, []any{journey.JourneyID}, 0)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_journeys WHERE journey_id=? AND status='cancelled' AND resolved_event_id=?`, []any{journey.JourneyID, cancelled.EventID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_occupancy_intervals WHERE agent_id=? AND location_id=? AND exited_at IS NULL`, []any{M2RPPlayerID, "place_m2_work_ada"}, 1)
	if diffs, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diffs) != 0 {
		t.Fatal("cancelled journey not replayable", diffs, err)
	}
}
