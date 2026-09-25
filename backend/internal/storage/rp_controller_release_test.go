package storage

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"corerp.local/backend/internal/core"
)

func TestRPControllerReleaseWaitsForSharedRoundAndPreservesReceipt(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "round-release.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	human, humanRead, initial := newRPWaitTestSession(t, ctx, s)
	for _, row := range []struct{ id, kind string }{{"principal_round_release_operator", "operator"}, {"principal_round_release_service", "service"}} {
		if _, err := s.db.ExecContext(ctx, `INSERT INTO principals(principal_id,principal_type,display_name,status) VALUES (?,?,?,'active')`, row.id, row.kind, row.id); err != nil {
			t.Fatal(err)
		}
	}
	binding := func(key string) core.CareerBinding {
		var head int64
		if err := s.db.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&head); err != nil {
			t.Fatal(err)
		}
		return core.CareerBinding{PrincipalID: "principal_round_release_operator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: head, IdempotencyKey: key}
	}
	if _, err := s.EnrollRPExternalControllerLocal(ctx, RPExternalControllerEnrollmentRequest{Binding: binding("round-release-enroll"), EntityID: M2AgentBoID, ControllerPrincipalID: "principal_round_release_service", ControllerInstanceID: "controller-round-release"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AssignRPExternalControllerLocal(ctx, RPExternalControllerAssignmentRequest{Binding: binding("round-release-assign"), EntityID: M2AgentBoID, ExpectedGeneration: 0}); err != nil {
		t.Fatal(err)
	}
	service, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: "principal_round_release_service", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2AgentBoID, POV: "second_person", IdempotencyKey: "round-release-service"})
	if err != nil {
		t.Fatal(err)
	}
	serviceRead := core.RPSessionReadRequest{PrincipalID: "principal_round_release_service", SessionID: service.SessionID}
	for _, read := range []core.RPSessionReadRequest{humanRead, serviceRead} {
		if _, err := s.ObserveRPSession(ctx, read); err != nil {
			t.Fatal(err)
		}
	}
	round, err := s.OpenRPSharedRoundLocal(ctx, RPSharedRoundOpenRequest{Binding: binding("round-before-release"), HumanSessionID: human.SessionID, ExternalSessionIDs: []string{service.SessionID}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO principals(principal_id,principal_type,display_name,status) VALUES ('principal_round_release_new','service','round new','active')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.EnrollRPExternalControllerLocal(ctx, RPExternalControllerEnrollmentRequest{Binding: binding("enroll-during-round"), EntityID: M2AgentAdaID, ControllerPrincipalID: "principal_round_release_new", ControllerInstanceID: "controller-round-new"}); !core.HasCode(err, core.CodeCommandInProgress) {
		t.Fatal("enrollment changed an open shared-round baseline", err)
	}
	release := RPExternalControllerReleaseRequest{Binding: binding("release-after-round"), EntityID: M2AgentBoID, ExpectedGeneration: 1}
	if _, err := s.ReleaseRPExternalControllerLocal(ctx, release); !core.HasCode(err, core.CodeCommandInProgress) {
		t.Fatal("released participant before shared round settled", err)
	}
	start, err := time.Parse(time.RFC3339, initial.WorldTime)
	if err != nil {
		t.Fatal(err)
	}
	horizon := start.Add(time.Minute).Format(time.RFC3339)
	serviceWait := RPSharedWaitRequest{PrincipalID: serviceRead.PrincipalID, SessionID: serviceRead.SessionID, RoundID: round.RoundID, HorizonWorldTime: horizon, IdempotencyKey: "service-round-wait"}
	humanWait := RPSharedWaitRequest{PrincipalID: humanRead.PrincipalID, SessionID: humanRead.SessionID, RoundID: round.RoundID, HorizonWorldTime: horizon, IdempotencyKey: "human-round-wait"}
	for _, request := range []RPSharedWaitRequest{serviceWait, humanWait} {
		if _, err := s.SubmitRPSharedWait(ctx, request); err != nil {
			t.Fatal(err)
		}
	}
	advance := RPSharedRoundAdvanceRequest{RPSharedRoundReadRequest: RPSharedRoundReadRequest{PrincipalID: serviceRead.PrincipalID, SessionID: serviceRead.SessionID, RoundID: round.RoundID}, Budget: 100}
	settled, err := s.AdvanceRPSharedRound(ctx, advance)
	if err != nil || settled.Status != "settled" {
		t.Fatal("round settlement", settled, err)
	}
	release.Binding = binding("release-after-round")
	if _, err := s.ReleaseRPExternalControllerLocal(ctx, release); err != nil {
		t.Fatal(err)
	}
	if again, err := s.ReadRPSharedRound(ctx, advance.RPSharedRoundReadRequest); err != nil || again.Status != "settled" || again.EventSequence != settled.EventSequence {
		t.Fatal("settled round receipt", again, err)
	}
	if again, err := s.SubmitRPSharedWait(ctx, serviceWait); err != nil || !again.Replayed || again.EventSequence != settled.EventSequence {
		t.Fatal("settled wait receipt", again, err)
	}
	if again, err := s.AdvanceRPSharedRound(ctx, advance); err != nil || !again.Replayed || again.EventSequence != settled.EventSequence {
		t.Fatal("settled advance receipt", again, err)
	}
	if _, err := s.ReadRPSharedRound(ctx, RPSharedRoundReadRequest{PrincipalID: humanRead.PrincipalID, SessionID: service.SessionID, RoundID: round.RoundID}); err == nil {
		t.Fatal("foreign principal read released service's round")
	}
	view, err := s.ObserveRPSession(ctx, humanRead)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: humanRead.PrincipalID, SessionID: humanRead.SessionID, TargetWorldTime: start.Add(2 * time.Minute).Format(time.RFC3339), Budget: 100, ExpectedCursor: view.ObservationCursor, IdempotencyKey: "human-after-release"}); err != nil {
		t.Fatal("Human could not advance released-controller world", err)
	}
	if old, err := s.ReadRPSharedRound(ctx, advance.RPSharedRoundReadRequest); err != nil || old.CurrentWorldTime != horizon {
		t.Fatal("released service receipt tracked later world time", old, err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='RPWaitCompleted'`, nil, 2)
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("round release projection", diff, err)
	}
}

func TestRPControllerLifecycle039UpgradePreservesGenerationOneWorld(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "upgrade-038-to-039.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	_, _, _ = newRPWaitTestSession(t, ctx, s)
	for _, row := range []struct{ id, kind string }{{"principal_upgrade_operator", "operator"}, {"principal_upgrade_service", "service"}} {
		if _, err := s.db.ExecContext(ctx, `INSERT INTO principals(principal_id,principal_type,display_name,status) VALUES (?,?,?,'active')`, row.id, row.kind, row.id); err != nil {
			t.Fatal(err)
		}
	}
	var head int64
	if err := s.db.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&head); err != nil {
		t.Fatal(err)
	}
	binding := core.CareerBinding{PrincipalID: "principal_upgrade_operator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: head, IdempotencyKey: "upgrade-enroll"}
	if _, err := s.EnrollRPExternalControllerLocal(ctx, RPExternalControllerEnrollmentRequest{Binding: binding, EntityID: M2AgentBoID, ControllerPrincipalID: "principal_upgrade_service", ControllerInstanceID: "controller-upgrade"}); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&head); err != nil {
		t.Fatal(err)
	}
	binding.ExpectedHead, binding.IdempotencyKey = head, "upgrade-assign"
	assigned, err := s.AssignRPExternalControllerLocal(ctx, RPExternalControllerAssignmentRequest{Binding: binding, EntityID: M2AgentBoID, ExpectedGeneration: 0})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `ALTER TABLE rp_controller_authorities DROP COLUMN status`); err != nil {
		t.Fatal("simulate 038 authority schema", err)
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM schema_meta WHERE schema_version=?`, RPControllerLifecycleSchemaVersion); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal("039 migration", err)
	}
	defer s.Close()
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM schema_meta WHERE schema_version=?`, []any{RPControllerLifecycleSchemaVersion}, 1)
	assertM2Value(t, ctx, s, `SELECT generation FROM rp_controller_authorities WHERE entity_id=? AND status='active'`, []any{M2AgentBoID}, 1)
	if again, err := s.AssignRPExternalControllerLocal(ctx, RPExternalControllerAssignmentRequest{Binding: binding, EntityID: M2AgentBoID, ExpectedGeneration: 0}); err != nil || !again.Replayed || again.EventID != assigned.EventID {
		t.Fatal("038 accepted assignment receipt", again, err)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("038->039 projection", diff, err)
	}
}

func TestRPControllerReleaseDoesNotCancelAcceptedJourney(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "release-journey.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, humanRead, initial := newRPWaitTestSession(t, ctx, s)
	segment, err := s.MaterializeRPLocation(ctx, RPLocationMaterializeRequest{Binding: core.CareerBinding{PrincipalID: "principal_creator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: initial.ObservationCursor, IdempotencyKey: "release-journey-segment"}, ParentLocationID: M2AgentCafeID, SlotKey: "release-journey-road", Candidate: RPLocationCandidate{DisplayName: "旅程交接小路", GeneratorVersion: "local-v1"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DefineRPTimedEdge(ctx, RPTimedEdgeRequest{Binding: core.CareerBinding{PrincipalID: "principal_creator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: segment.EventSequence, IdempotencyKey: "release-journey-edge"}, FromPlaceID: M2AgentCafeID, ToPlaceID: "place_m2_home_ada", SegmentPlaceID: segment.Fact.LocationID, DurationMinutes: 15}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CloseRPSession(ctx, humanRead); err != nil {
		t.Fatal(err)
	}
	for _, row := range []struct{ id, kind string }{{"principal_journey_release_operator", "operator"}, {"principal_journey_release_service", "service"}} {
		if _, err := s.db.ExecContext(ctx, `INSERT INTO principals(principal_id,principal_type,display_name,status) VALUES (?,?,?,'active')`, row.id, row.kind, row.id); err != nil {
			t.Fatal(err)
		}
	}
	binding := func(key string) core.CareerBinding {
		var head int64
		if err := s.db.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&head); err != nil {
			t.Fatal(err)
		}
		return core.CareerBinding{PrincipalID: "principal_journey_release_operator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: head, IdempotencyKey: key}
	}
	if _, err := s.EnrollRPExternalControllerLocal(ctx, RPExternalControllerEnrollmentRequest{Binding: binding("journey-release-enroll"), EntityID: M2RPPlayerID, ControllerPrincipalID: "principal_journey_release_service", ControllerInstanceID: "controller-journey-release"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AssignRPExternalControllerLocal(ctx, RPExternalControllerAssignmentRequest{Binding: binding("journey-release-assign"), EntityID: M2RPPlayerID, ExpectedGeneration: 0}); err != nil {
		t.Fatal(err)
	}
	service, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: "principal_journey_release_service", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2RPPlayerID, POV: "second_person", IdempotencyKey: "journey-release-service"})
	if err != nil {
		t.Fatal(err)
	}
	serviceRead := core.RPSessionReadRequest{PrincipalID: "principal_journey_release_service", SessionID: service.SessionID}
	view, err := s.ObserveRPSession(ctx, serviceRead)
	if err != nil {
		t.Fatal(err)
	}
	start := core.RPMoveRequest{PrincipalID: serviceRead.PrincipalID, SessionID: serviceRead.SessionID, FromPlaceID: view.PlaceID, ToPlaceID: "place_m2_home_ada", ExpectedCursor: view.ObservationCursor, IdempotencyKey: "journey-before-release"}
	journey, err := s.StartRPJourney(ctx, start)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReleaseRPExternalControllerLocal(ctx, RPExternalControllerReleaseRequest{Binding: binding("release-active-journey"), EntityID: M2RPPlayerID, ExpectedGeneration: 1}); err != nil {
		t.Fatal("release cancelled or blocked accepted scheduler journey", err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM scheduler_items q JOIN rp_journeys j ON j.arrival_item_id=q.scheduler_item_id WHERE j.journey_id=? AND q.status='pending'`, []any{journey.JourneyID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_journeys WHERE journey_id=? AND status='active'`, []any{journey.JourneyID}, 1)
	human, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: M2RPPlayerPrincipal, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2RPPlayerID, POV: "second_person", IdempotencyKey: "journey-after-release-human"})
	if err != nil || human.ControlGeneration != 2 {
		t.Fatal("new Human session", human, err)
	}
	humanView, err := s.ObserveRPSession(ctx, core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: human.SessionID})
	if err != nil || humanView.PlaceID != journey.SegmentPlaceID {
		t.Fatal("accepted journey segment lost", humanView, err)
	}
	if _, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: human.SessionID, TargetWorldTime: journey.ScheduledArrivalAt, Budget: 100, ExpectedCursor: humanView.ObservationCursor, IdempotencyKey: "journey-after-release-arrival"}); err != nil {
		t.Fatal("scheduler did not complete accepted journey", err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_journeys WHERE journey_id=? AND status='arrived'`, []any{journey.JourneyID}, 1)
	if again, err := s.StartRPJourney(ctx, start); err != nil || !again.Replayed || again.EventID != journey.EventID {
		t.Fatal("old controller lost accepted journey receipt", again, err)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("journey release projection", diff, err)
	}
}

func TestRPControllerReleaseReassignMonotoneAndRebuild(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "controller-lifecycle.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Close() }()
	_, _, _ = newRPWaitTestSession(t, ctx, s)
	for _, row := range []struct{ id, kind string }{{"principal_lifecycle_operator", "operator"}, {"principal_lifecycle_service", "service"}} {
		if _, err := s.db.ExecContext(ctx, `INSERT INTO principals(principal_id,principal_type,display_name,status) VALUES (?,?,?,'active')`, row.id, row.kind, row.id); err != nil {
			t.Fatal(err)
		}
	}
	binding := func(key string) core.CareerBinding {
		var head int64
		if err := s.db.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&head); err != nil {
			t.Fatal(err)
		}
		return core.CareerBinding{PrincipalID: "principal_lifecycle_operator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: head, IdempotencyKey: key}
	}
	if _, err := s.EnrollRPExternalControllerLocal(ctx, RPExternalControllerEnrollmentRequest{Binding: binding("lifecycle-enroll"), EntityID: M2AgentBoID, ControllerPrincipalID: "principal_lifecycle_service", ControllerInstanceID: "controller-lifecycle-bo"}); err != nil {
		t.Fatal(err)
	}
	firstRequest := RPExternalControllerAssignmentRequest{Binding: binding("lifecycle-assign-1"), EntityID: M2AgentBoID, ExpectedGeneration: 0}
	first, err := s.AssignRPExternalControllerLocal(ctx, firstRequest)
	if err != nil || first.Fact.Generation != 1 {
		t.Fatal("first assignment", first, err)
	}
	serviceOpen := core.RPSessionOpenRequest{PrincipalID: "principal_lifecycle_service", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2AgentBoID, POV: "second_person", IdempotencyKey: "lifecycle-old-service"}
	old, err := s.OpenRPSession(ctx, serviceOpen)
	if err != nil || old.ControlGeneration != 1 {
		t.Fatal("first service session", old, err)
	}
	read := core.RPSessionReadRequest{PrincipalID: serviceOpen.PrincipalID, SessionID: old.SessionID}
	view, err := s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	speech := core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, Text: "第一代的话。", ExpectedCursor: view.ObservationCursor, IdempotencyKey: "lifecycle-old-speech"}
	spoken, err := s.SpeakRP(ctx, speech)
	if err != nil {
		t.Fatal(err)
	}
	releaseRequest := RPExternalControllerReleaseRequest{Binding: binding("lifecycle-release"), EntityID: M2AgentBoID, ExpectedGeneration: 1}
	s.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "release rollback") }
	if _, err := s.ReleaseRPExternalControllerLocal(ctx, releaseRequest); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatal("release did not roll back", err)
	}
	s.beforeCommit = nil
	assertM2Value(t, ctx, s, `SELECT generation FROM rp_controller_authorities WHERE entity_id=? AND status='active'`, []any{M2AgentBoID}, 1)
	released, err := s.ReleaseRPExternalControllerLocal(ctx, releaseRequest)
	if err != nil || released.Fact.Generation != 2 || released.Replayed {
		t.Fatal("release", released, err)
	}
	if again, err := s.ReleaseRPExternalControllerLocal(ctx, releaseRequest); err != nil || !again.Replayed || again.EventID != released.EventID {
		t.Fatal("release retry", again, err)
	}
	assertM2Value(t, ctx, s, `SELECT generation FROM rp_controller_authorities WHERE entity_id=? AND status='released'`, []any{M2AgentBoID}, 2)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_sessions WHERE session_id=? AND status='closed'`, []any{old.SessionID}, 1)
	if gen, controller, err := rpControlGeneration(ctx, s.db, M2DemoInstanceID, M2DemoBranchID, M2AgentBoID); err != nil || gen != 2 || controller != "" {
		t.Fatal("released generation", gen, controller, err)
	}
	if err := requireInternalRPDecisionOwner(ctx, s.db, M2DemoInstanceID, M2DemoBranchID, M2AgentBoID); err != nil {
		t.Fatal("internal AI did not regain unowned Entity", err)
	}
	if again, err := s.SpeakRP(ctx, speech); err != nil || !again.Replayed || again.EventID != spoken.EventID {
		t.Fatal("old accepted speech receipt", again, err)
	}
	newSpeech := speech
	newSpeech.IdempotencyKey = "old-controller-after-release"
	if _, err := s.SpeakRP(ctx, newSpeech); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatal("released controller submitted fresh speech", err)
	}
	if _, err := s.ReadRPSession(ctx, read); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatal("old session stayed current", err)
	}
	bindings, err := s.DiscoverRPBindings(ctx, RPDiscoverRequest{PrincipalID: serviceOpen.PrincipalID})
	if err != nil || len(bindings.Bindings) != 0 {
		t.Fatal("released service still discovered Entity", bindings, err)
	}
	grantRPControlForTest(t, ctx, s, M2AgentBoID)
	human, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: rpTestPrincipal, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2AgentBoID, POV: "second_person", IdempotencyKey: "lifecycle-human-generation-2"})
	if err != nil || human.ControlGeneration != 2 || human.ControllerInstanceID != "" {
		t.Fatal("Human controller after release", human, err)
	}
	nextRequest := RPExternalControllerAssignmentRequest{Binding: binding("lifecycle-assign-3"), EntityID: M2AgentBoID, ExpectedGeneration: 2}
	if _, err := s.AssignRPExternalControllerLocal(ctx, nextRequest); !core.HasCode(err, core.CodeCommandInProgress) {
		t.Fatal("reassignment displaced active Human", err)
	}
	if _, err := s.CloseRPSession(ctx, core.RPSessionReadRequest{PrincipalID: rpTestPrincipal, SessionID: human.SessionID}); err != nil {
		t.Fatal(err)
	}
	nextRequest.Binding = binding("lifecycle-assign-3")
	next, err := s.AssignRPExternalControllerLocal(ctx, nextRequest)
	if err != nil || next.Fact.Generation != 3 || next.Fact.Version != "corerp.controller-assignment.v2" {
		t.Fatal("higher generation assignment", next, err)
	}
	if _, err := s.AssignRPExternalControllerLocal(ctx, RPExternalControllerAssignmentRequest{Binding: binding("stale-generation"), EntityID: M2AgentBoID, ExpectedGeneration: 2}); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatal("late generation-two assignment accepted", err)
	}
	if _, err := s.ReleaseRPExternalControllerLocal(ctx, RPExternalControllerReleaseRequest{Binding: binding("stale-release"), EntityID: M2AgentBoID, ExpectedGeneration: 1}); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatal("late generation-one release accepted", err)
	}
	if again, err := s.SpeakRP(ctx, speech); err != nil || !again.Replayed || again.EventID != spoken.EventID {
		t.Fatal("generation-one receipt lost after reacquisition", again, err)
	}
	if _, err := s.SpeakRP(ctx, newSpeech); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatal("generation-one session revived after reacquisition", err)
	}
	newOpen := serviceOpen
	newOpen.IdempotencyKey = "lifecycle-service-generation-3"
	current, err := s.OpenRPSession(ctx, newOpen)
	if err != nil || current.ControlGeneration != 3 || current.SessionID == old.SessionID {
		t.Fatal("new service generation", current, err)
	}
	currentRead := core.RPSessionReadRequest{PrincipalID: newOpen.PrincipalID, SessionID: current.SessionID}
	currentView, err := s.ObserveRPSession(ctx, currentRead)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SpeakRP(ctx, core.RPSpeechRequest{PrincipalID: currentRead.PrincipalID, SessionID: currentRead.SessionID, Text: "第三代的话。", ExpectedCursor: currentView.ObservationCursor, IdempotencyKey: "lifecycle-new-speech"}); err != nil {
		t.Fatal("new owner could not act", err)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("lifecycle projection", diff, err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE rp_controller_authorities SET generation=1,status='released' WHERE entity_id=?`, M2AgentBoID); err != nil {
		t.Fatal(err)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) == 0 {
		t.Fatal("damaged lifecycle projection went undetected", diff, err)
	}
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal("rebuild lifecycle", err)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("lifecycle rebuild diverged", diff, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReadRPSession(ctx, currentRead); err != nil {
		t.Fatal("generation-three session lost after reopen", err)
	}
	if _, err := s.ReadRPSession(ctx, read); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatal("generation-one session revived after reopen", err)
	}
}
