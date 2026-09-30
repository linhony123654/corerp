package storage

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"corerp.local/backend/internal/core"
)

func TestRPControllerHandoffKeepsOnlyExactAcceptedReceipts(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "accepted-handoff.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	human, read, view := newRPWaitTestSession(t, ctx, s)
	social := socialRequest(t, ctx, s, read, M2RPNPCID, "greet", "accepted-social")
	greeted, err := s.SocialRP(ctx, social)
	if err != nil {
		t.Fatal(err)
	}
	view, err = s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	move := core.RPMoveRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: human.SessionID, FromPlaceID: view.PlaceID, ToPlaceID: "place_m2_home_ada", ExpectedCursor: view.ObservationCursor, IdempotencyKey: "accepted-move"}
	moved, err := s.MoveRP(ctx, move)
	if err != nil {
		t.Fatal(err)
	}
	view, err = s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	speech := core.RPSpeechRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: human.SessionID, Text: "交接前的话。", ExpectedCursor: view.ObservationCursor, IdempotencyKey: "accepted-speech"}
	spoken, err := s.SpeakRP(ctx, speech)
	if err != nil {
		t.Fatal(err)
	}
	view, err = s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	turnRequest := core.RPSpeechRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: human.SessionID, Text: "轮次交接。", ExpectedCursor: view.ObservationCursor, IdempotencyKey: "accepted-turn"}
	turn, err := s.PlayRPTurn(ctx, turnRequest)
	if err != nil {
		t.Fatal(err)
	}
	view, err = s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	start, err := time.Parse(time.RFC3339, view.WorldTime)
	if err != nil {
		t.Fatal(err)
	}
	wait := core.RPWaitRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: human.SessionID, TargetWorldTime: start.Add(time.Minute).Format(time.RFC3339), Budget: 100, ExpectedCursor: view.ObservationCursor, IdempotencyKey: "accepted-wait"}
	waited, err := s.WaitRP(ctx, wait)
	if err != nil || waited.Status != "completed" {
		t.Fatal("wait", waited, err)
	}
	view, err = s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	interaction := core.RPInteractionRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: human.SessionID, Text: "交接前的互动。", Mode: "DIALOGUE", ExpectedCursor: view.ObservationCursor, IdempotencyKey: "accepted-interaction"}
	settled, err := s.RunRPInteraction(ctx, interaction)
	if err != nil || settled.Status != "settled" {
		t.Fatal("interaction", settled, err)
	}
	if _, err := s.CloseRPSession(ctx, read); err != nil {
		t.Fatal(err)
	}
	for _, row := range []struct{ id, kind string }{{"principal_handoff_operator", "operator"}, {"principal_handoff_service", "service"}} {
		if _, err := s.db.ExecContext(ctx, `INSERT INTO principals(principal_id,principal_type,display_name,status) VALUES (?,?,?,'active')`, row.id, row.kind, row.id); err != nil {
			t.Fatal(err)
		}
	}
	binding := func(key string) core.CareerBinding {
		var head int64
		if err := s.db.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&head); err != nil {
			t.Fatal(err)
		}
		return core.CareerBinding{PrincipalID: "principal_handoff_operator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: head, IdempotencyKey: key}
	}
	if _, err := s.EnrollRPExternalControllerLocal(ctx, RPExternalControllerEnrollmentRequest{Binding: binding("enroll-handoff"), EntityID: M2RPPlayerID, ControllerPrincipalID: "principal_handoff_service", ControllerInstanceID: "controller-handoff"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AssignRPExternalControllerLocal(ctx, RPExternalControllerAssignmentRequest{Binding: binding("assign-handoff"), EntityID: M2RPPlayerID, ExpectedGeneration: 0}); err != nil {
		t.Fatal(err)
	}
	if again, err := s.MoveRP(ctx, move); err != nil || !again.Replayed || again.EventID != moved.EventID {
		t.Fatal("move receipt", again, err)
	}
	if again, err := s.SocialRP(ctx, social); err != nil || !again.Replayed || again.EventID != greeted.EventID {
		t.Fatal("social receipt", again, err)
	}
	if again, err := s.SpeakRP(ctx, speech); err != nil || !again.Replayed || again.EventID != spoken.EventID {
		t.Fatal("speech receipt", again, err)
	}
	if again, err := s.RunRPTurn(ctx, turnRequest, nil); err != nil || !again.Replayed || again.TurnRunID != turn.TurnRunID {
		t.Fatal("turn receipt", again, err)
	}
	if again, err := s.ResumeRPTurn(ctx, RPTurnResumeRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: human.SessionID, IdempotencyKey: turnRequest.IdempotencyKey}, nil); err != nil || !again.Replayed || again.TurnRunID != turn.TurnRunID {
		t.Fatal("resume turn receipt", again, err)
	}
	if again, err := s.WaitRP(ctx, wait); err != nil || !again.Replayed || again.EventID != waited.EventID {
		t.Fatal("wait receipt", again, err)
	}
	if again, err := s.RunRPInteraction(ctx, interaction); err != nil || !again.Replayed || again.InteractionID != settled.InteractionID {
		t.Fatal("interaction receipt", again, err)
	}
	resume := RPInteractionResumeRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: human.SessionID, IdempotencyKey: interaction.IdempotencyKey}
	if again, err := s.ResumeRPInteraction(ctx, resume); err != nil || !again.Replayed || again.InteractionID != settled.InteractionID {
		t.Fatal("interaction resume receipt", again, err)
	}
	if again, err := s.StopRPInteraction(ctx, resume); err != nil || !again.Replayed || again.InteractionID != settled.InteractionID {
		t.Fatal("interaction stop receipt", again, err)
	}
	if again, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: M2RPPlayerPrincipal, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2RPPlayerID, POV: "second_person", IdempotencyKey: "wait-session"}); err != nil || !again.Replayed || again.SessionID != human.SessionID {
		t.Fatal("open receipt", again, err)
	}
	newMove := move
	newMove.IdempotencyKey = "unaccepted-old-owner"
	if _, err := s.MoveRP(ctx, newMove); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatal("old controller submitted fresh move", err)
	}
	newInteraction := interaction
	newInteraction.IdempotencyKey = "unaccepted-old-interaction"
	if _, err := s.RunRPInteraction(ctx, newInteraction); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatal("old controller submitted fresh interaction", err)
	}
	mismatch := move
	mismatch.ToPlaceID = "place_m2_home_bo"
	if _, err := s.MoveRP(ctx, mismatch); !core.HasCode(err, core.CodeIdempotencyMismatch) {
		t.Fatal("changed accepted request", err)
	}
	foreign := move
	foreign.PrincipalID = "principal_handoff_service"
	if _, err := s.MoveRP(ctx, foreign); !core.HasCode(err, core.CodeNotFound) {
		t.Fatal("foreign principal read old receipt", err)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("projection", diff, err)
	}
}

func TestRPControllerHandoffKeepsJourneyAndCancellationReceipts(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "journey-handoff.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, read, view := newRPWaitTestSession(t, ctx, s)
	segment, err := s.MaterializeRPLocation(ctx, RPLocationMaterializeRequest{Binding: core.CareerBinding{PrincipalID: "principal_creator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: view.ObservationCursor, IdempotencyKey: "handoff-segment"}, ParentLocationID: M2AgentCafeID, SlotKey: "handoff-road", Candidate: RPLocationCandidate{DisplayName: "交接小路", GeneratorVersion: "local-v1"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DefineRPTimedEdge(ctx, RPTimedEdgeRequest{Binding: core.CareerBinding{PrincipalID: "principal_creator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: segment.EventSequence, IdempotencyKey: "handoff-edge"}, FromPlaceID: M2AgentCafeID, ToPlaceID: "place_m2_home_ada", SegmentPlaceID: segment.Fact.LocationID, DurationMinutes: 15}); err != nil {
		t.Fatal(err)
	}
	view, err = s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	start := core.RPMoveRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, FromPlaceID: view.PlaceID, ToPlaceID: "place_m2_home_ada", ExpectedCursor: view.ObservationCursor, IdempotencyKey: "accepted-journey"}
	journey, err := s.StartRPJourney(ctx, start)
	if err != nil {
		t.Fatal(err)
	}
	view, err = s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	cancel := core.RPJourneyCancelRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, JourneyID: journey.JourneyID, ExpectedCursor: view.ObservationCursor, IdempotencyKey: "accepted-cancel"}
	cancelled, err := s.CancelRPJourney(ctx, cancel)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CloseRPSession(ctx, read); err != nil {
		t.Fatal(err)
	}
	for _, row := range []struct{ id, kind string }{{"principal_journey_operator", "operator"}, {"principal_journey_service", "service"}} {
		if _, err := s.db.ExecContext(ctx, `INSERT INTO principals(principal_id,principal_type,display_name,status) VALUES (?,?,?,'active')`, row.id, row.kind, row.id); err != nil {
			t.Fatal(err)
		}
	}
	binding := func(key string) core.CareerBinding {
		var head int64
		if err := s.db.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&head); err != nil {
			t.Fatal(err)
		}
		return core.CareerBinding{PrincipalID: "principal_journey_operator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: head, IdempotencyKey: key}
	}
	if _, err := s.EnrollRPExternalControllerLocal(ctx, RPExternalControllerEnrollmentRequest{Binding: binding("journey-enroll"), EntityID: M2RPPlayerID, ControllerPrincipalID: "principal_journey_service", ControllerInstanceID: "controller-journey"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AssignRPExternalControllerLocal(ctx, RPExternalControllerAssignmentRequest{Binding: binding("journey-assign"), EntityID: M2RPPlayerID, ExpectedGeneration: 0}); err != nil {
		t.Fatal(err)
	}
	if again, err := s.StartRPJourney(ctx, start); err != nil || !again.Replayed || again.EventID != journey.EventID {
		t.Fatal("journey start receipt", again, err)
	}
	if again, err := s.CancelRPJourney(ctx, cancel); err != nil || !again.Replayed || again.EventID != cancelled.EventID {
		t.Fatal("journey cancel receipt", again, err)
	}
	newCancel := cancel
	newCancel.IdempotencyKey = "old-controller-new-cancel"
	if _, err := s.CancelRPJourney(ctx, newCancel); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatal("old controller submitted fresh cancellation", err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='RPJourneyCancelled' AND json_extract(payload,'$.journey_id')=?`, []any{journey.JourneyID}, 1)
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("projection", diff, err)
	}
}

func TestRPControllerAssignmentDoesNotStrandAcceptedInteraction(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "interaction-assignment-fence.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	human, read, view := newRPWaitTestSession(t, ctx, s)
	request := core.RPInteractionRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, Text: "交接前先说完。", Mode: "DIALOGUE", ExpectedCursor: view.ObservationCursor, IdempotencyKey: "pending-interaction"}
	s.afterRPInteractionStep = func(index int) error {
		return core.NewError(core.CodeInjectedFailure, "accepted child result lost")
	}
	if _, err := s.RunRPInteraction(ctx, request); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatal("interaction did not reach accepted child", err)
	}
	s.afterRPInteractionStep = nil
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_interactions WHERE session_id=? AND status='open' AND pending_kind='speech'`, []any{human.SessionID}, 1)
	if _, err := s.CloseRPSession(ctx, read); !core.HasCode(err, core.CodeCommandInProgress) {
		t.Fatal("closed session with accepted unfinished interaction", err)
	}
	for _, row := range []struct{ id, kind string }{{"principal_pending_operator", "operator"}, {"principal_pending_service", "service"}} {
		if _, err := s.db.ExecContext(ctx, `INSERT INTO principals(principal_id,principal_type,display_name,status) VALUES (?,?,?,'active')`, row.id, row.kind, row.id); err != nil {
			t.Fatal(err)
		}
	}
	binding := func(key string) core.CareerBinding {
		var head int64
		if err := s.db.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&head); err != nil {
			t.Fatal(err)
		}
		return core.CareerBinding{PrincipalID: "principal_pending_operator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: head, IdempotencyKey: key}
	}
	if _, err := s.EnrollRPExternalControllerLocal(ctx, RPExternalControllerEnrollmentRequest{Binding: binding("pending-enroll"), EntityID: M2RPPlayerID, ControllerPrincipalID: "principal_pending_service", ControllerInstanceID: "controller-pending"}); err != nil {
		t.Fatal(err)
	}
	// Simulate a pre-fence or damaged application session row. The durable
	// interaction must still stop takeover until its accepted child settles.
	if _, err := s.db.ExecContext(ctx, `UPDATE rp_sessions SET status='closed' WHERE session_id=?`, human.SessionID); err != nil {
		t.Fatal(err)
	}
	assign := RPExternalControllerAssignmentRequest{Binding: binding("pending-assign"), EntityID: M2RPPlayerID, ExpectedGeneration: 0}
	if _, err := s.AssignRPExternalControllerLocal(ctx, assign); !core.HasCode(err, core.CodeCommandInProgress) {
		t.Fatal("assignment stranded accepted interaction", err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE rp_sessions SET status='active' WHERE session_id=?`, human.SessionID); err != nil {
		t.Fatal(err)
	}
	settled, err := s.ResumeRPInteraction(ctx, RPInteractionResumeRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, IdempotencyKey: request.IdempotencyKey})
	if err != nil || settled.Status != "settled" {
		t.Fatal("accepted interaction did not settle", settled, err)
	}
	if _, err := s.CloseRPSession(ctx, read); err != nil {
		t.Fatal(err)
	}
	assign.Binding = binding("pending-assign")
	if _, err := s.AssignRPExternalControllerLocal(ctx, assign); err != nil {
		t.Fatal("settled interaction blocked assignment", err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_utterances WHERE speaker_entity_id=? AND speech_text=?`, []any{M2RPPlayerID, "交接前先说完。"}, 1)
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("projection", diff, err)
	}
}

func TestRPExternalControllerQuiescentAssignmentPinsAuthorityAndRebuilds(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "controller-authority.db")
	s := openM2AgentStore(t, ctx, path, false)
	defer s.Close()
	for _, row := range []struct{ id, kind string }{{"principal_f3_operator", "operator"}, {"principal_f3_service", "service"}} {
		if _, err := s.db.ExecContext(ctx, `INSERT INTO principals(principal_id,principal_type,display_name,status) VALUES (?,?,?,'active')`, row.id, row.kind, row.id); err != nil {
			t.Fatal(err)
		}
	}
	head := func() int64 {
		var value int64
		if err := s.db.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	binding := func(key string) core.CareerBinding {
		return core.CareerBinding{PrincipalID: "principal_f3_operator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: head(), IdempotencyKey: key}
	}
	if _, err := s.EnrollRPExternalControllerLocal(ctx, RPExternalControllerEnrollmentRequest{Binding: binding("enroll"), EntityID: M2AgentBoID, ControllerPrincipalID: "principal_f3_service", ControllerInstanceID: "controller-f3-bo"}); err != nil {
		t.Fatal(err)
	}
	grantRPControlForTest(t, ctx, s, M2AgentBoID)
	old, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: rpTestPrincipal, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2AgentBoID, POV: "second_person", IdempotencyKey: "old-bo"})
	if err != nil {
		t.Fatal(err)
	}
	request := RPExternalControllerAssignmentRequest{Binding: binding("assign-bo"), EntityID: M2AgentBoID, ExpectedGeneration: 0}
	if _, err := s.AssignRPExternalControllerLocal(ctx, request); !core.HasCode(err, core.CodeCommandInProgress) {
		t.Fatal("assignment displaced open player session", err)
	}
	if _, err := s.CloseRPSession(ctx, core.RPSessionReadRequest{PrincipalID: rpTestPrincipal, SessionID: old.SessionID}); err != nil {
		t.Fatal(err)
	}
	s.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "assignment rollback") }
	if _, err := s.AssignRPExternalControllerLocal(ctx, request); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatal("assignment precommit did not roll back", err)
	}
	s.beforeCommit = nil
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_controller_authorities WHERE entity_id=?`, []any{M2AgentBoID}, 0)
	assigned, err := s.AssignRPExternalControllerLocal(ctx, request)
	if err != nil || assigned.Replayed || assigned.Fact.Generation != 1 {
		t.Fatal("assign", assigned, err)
	}
	if replayed, err := s.AssignRPExternalControllerLocal(ctx, request); err != nil || !replayed.Replayed || replayed.EventID != assigned.EventID {
		t.Fatal("exact retry", replayed, err)
	}
	if _, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: rpTestPrincipal, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2AgentBoID, POV: "second_person", IdempotencyKey: "player-after"}); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatal("old player still controlled Entity", err)
	}
	oldBindings, err := s.DiscoverRPBindings(ctx, RPDiscoverRequest{PrincipalID: rpTestPrincipal})
	if err != nil {
		t.Fatal(err)
	}
	for _, binding := range oldBindings.Bindings {
		if binding.EntityID == M2AgentBoID {
			t.Fatal("displaced player still discovered Bo", oldBindings)
		}
	}
	serviceBindings, err := s.DiscoverRPBindings(ctx, RPDiscoverRequest{PrincipalID: "principal_f3_service"})
	if err != nil || len(serviceBindings.Bindings) != 1 || serviceBindings.Bindings[0].EntityID != M2AgentBoID {
		t.Fatal("assigned service owner cannot discover Bo", serviceBindings, err)
	}
	serviceOpen := core.RPSessionOpenRequest{PrincipalID: "principal_f3_service", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2AgentBoID, POV: "second_person", IdempotencyKey: "external-bo"}
	service, err := s.OpenRPSession(ctx, serviceOpen)
	if err != nil || service.ControlGeneration != 1 || service.ControllerInstanceID != "controller-f3-bo" {
		t.Fatal("service generation", service, err)
	}
	if _, err := s.ReadRPSession(ctx, core.RPSessionReadRequest{PrincipalID: "principal_f3_service", SessionID: service.SessionID}); err != nil {
		t.Fatal("read external session", err)
	}
	if err := requireInternalRPDecisionOwner(ctx, s.db, M2DemoInstanceID, M2DemoBranchID, M2AgentBoID); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatal("internal decision not fenced", err)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("projection", diff, err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE rp_controller_authorities SET controller_instance_id='tampered' WHERE entity_id=?`, M2AgentBoID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReadRPSession(ctx, core.RPSessionReadRequest{PrincipalID: "principal_f3_service", SessionID: service.SessionID}); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatal("session accepted tampered generation", err)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) == 0 {
		t.Fatal("corruption undetected", diff, err)
	}
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal("rebuild", err)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("rebuild mismatch", diff, err)
	}
	if _, err := s.ReadRPSession(ctx, core.RPSessionReadRequest{PrincipalID: "principal_f3_service", SessionID: service.SessionID}); err != nil {
		t.Fatal("repaired session", err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE rp_external_controller_enrollments SET controller_instance_id='damaged-enrollment' WHERE entity_id=?`, M2AgentBoID); err != nil {
		t.Fatal(err)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) == 0 {
		t.Fatal("damaged enrollment undetected", diff, err)
	}
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal("repair enrollment with active authority", err)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("repair enrollment and authority mismatch", diff, err)
	}
	if _, err := s.ReadRPSession(ctx, core.RPSessionReadRequest{PrincipalID: "principal_f3_service", SessionID: service.SessionID}); err != nil {
		t.Fatal("repaired enrollment displaced controller", err)
	}
}

func TestRPExternalControllerHeardSpeechSkipsInternalDecisionAndSettles(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "controller-speech.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	human, read, initial := newRPWaitTestSession(t, ctx, s)
	start, err := time.Parse(time.RFC3339, initial.WorldTime)
	if err != nil {
		t.Fatal(err)
	}
	priorWait := core.RPWaitRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: human.SessionID, TargetWorldTime: start.Add(time.Minute).Format(time.RFC3339), Budget: 100, ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "prior-accepted-wait"}
	completedWait, err := s.WaitRP(ctx, priorWait)
	if err != nil || completedWait.Status != "completed" {
		t.Fatal("pre-assignment wait", completedWait, err)
	}
	for _, row := range []struct{ id, kind string }{{"principal_f3_operator", "operator"}, {"principal_f3_ada", "service"}} {
		if _, err := s.db.ExecContext(ctx, `INSERT INTO principals(principal_id,principal_type,display_name,status) VALUES (?,?,?,'active')`, row.id, row.kind, row.id); err != nil {
			t.Fatal(err)
		}
	}
	var head int64
	if err := s.db.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&head); err != nil {
		t.Fatal(err)
	}
	b := core.CareerBinding{PrincipalID: "principal_f3_operator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: head, IdempotencyKey: "enroll-ada"}
	if _, err := s.EnrollRPExternalControllerLocal(ctx, RPExternalControllerEnrollmentRequest{Binding: b, EntityID: M2AgentAdaID, ControllerPrincipalID: "principal_f3_ada", ControllerInstanceID: "controller-f3-ada"}); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&head); err != nil {
		t.Fatal(err)
	}
	b.ExpectedHead, b.IdempotencyKey = head, "assign-ada"
	if _, err := s.AssignRPExternalControllerLocal(ctx, RPExternalControllerAssignmentRequest{Binding: b, EntityID: M2AgentAdaID, ExpectedGeneration: 0}); err != nil {
		t.Fatal(err)
	}
	initial, err = s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	if replayed, err := s.WaitRP(ctx, priorWait); err != nil || !replayed.Replayed || replayed.EventID != completedWait.EventID {
		t.Fatal("accepted wait recovery after another Entity's assignment", replayed, err)
	}
	if _, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: human.SessionID, TargetWorldTime: M2AgentNoonTime, Budget: 100, ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "premature-unilateral-wait"}); !core.HasCode(err, core.CodeCommandInProgress) {
		t.Fatal("uncoordinated wait advanced a two-controller world", err)
	}
	if _, err := s.MoveRP(ctx, core.RPMoveRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: human.SessionID, FromPlaceID: initial.PlaceID, ToPlaceID: "place_m2_home_ada", ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "visit-ada"}); err != nil {
		t.Fatal(err)
	}
	view, err := s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	turn, err := s.PlayRPTurn(ctx, core.RPSpeechRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: human.SessionID, Text: "你好，艾达。", ExpectedCursor: view.ObservationCursor, IdempotencyKey: "speak-to-ada"})
	if err != nil || turn.Status != "settled" {
		t.Fatal("speech did not settle", turn, err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_turn_listener_skips WHERE turn_run_id=? AND npc_entity_id=?`, []any{turn.TurnRunID, M2AgentAdaID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_turn_listener_activations WHERE turn_run_id=? AND npc_entity_id=? AND disposition='externally_controlled' AND reason_code='external_controller'`, []any{turn.TurnRunID, M2AgentAdaID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_npc_decisions WHERE parent_turn_id=? AND npc_entity_id=?`, []any{turn.PlayerTurnID, M2AgentAdaID}, 0)
	if again, err := s.PlayRPTurn(ctx, core.RPSpeechRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: human.SessionID, Text: "你好，艾达。", ExpectedCursor: view.ObservationCursor, IdempotencyKey: "speak-to-ada"}); err != nil || !again.Replayed || again.TurnRunID != turn.TurnRunID {
		t.Fatal("turn replay", again, err)
	}
	service, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: "principal_f3_ada", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2AgentAdaID, POV: "second_person", IdempotencyKey: "ada-service-session"})
	if err != nil {
		t.Fatal(err)
	}
	serviceView, err := s.ObserveRPSession(ctx, core.RPSessionReadRequest{PrincipalID: "principal_f3_ada", SessionID: service.SessionID})
	if err != nil {
		t.Fatal(err)
	}
	answer, err := s.PlayRPTurn(ctx, core.RPSpeechRequest{PrincipalID: "principal_f3_ada", SessionID: service.SessionID, Text: "你好，我听见了。", ExpectedCursor: serviceView.ObservationCursor, IdempotencyKey: "ada-replies-to-human"})
	if err != nil || answer.Status != "settled" {
		t.Fatal("external speech did not settle", answer, err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_turn_listener_skips WHERE turn_run_id=? AND npc_entity_id=?`, []any{answer.TurnRunID, M2RPPlayerID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_npc_decisions WHERE parent_turn_id=? AND npc_entity_id=?`, []any{answer.PlayerTurnID, M2RPPlayerID}, 0)
}

func TestRPTurnProviderProposalLosesToNewHumanController(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "controller-late-human.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	human, read, initial := newRPWaitTestSession(t, ctx, s)
	grantRPControlForTest(t, ctx, s, M2AgentAdaID)
	if _, err := s.MoveRP(ctx, core.RPMoveRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: human.SessionID, FromPlaceID: initial.PlaceID, ToPlaceID: "place_m2_home_ada", ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "late-human-move"}); err != nil {
		t.Fatal(err)
	}
	view, err := s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	provider := rpDecisionProviderFunc(func(ctx context.Context, input core.RPDecisionInput) (core.RPDecisionProposal, error) {
		calls++
		if _, err := s.OpenRPSession(ctx, rpTestOpenRequest()); err != nil {
			return core.RPDecisionProposal{}, err
		}
		return core.DeterministicRPDecisionProvider{}.Propose(ctx, input)
	})
	turn, err := s.RunRPTurn(ctx, core.RPSpeechRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: human.SessionID, Text: "这里有人吗？", ExpectedCursor: view.ObservationCursor, IdempotencyKey: "late-human-speech"}, provider)
	if err != nil || turn.Status != "settled" || calls != 1 {
		t.Fatal("late Human did not fence provider", turn, calls, err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_turn_listener_skips WHERE turn_run_id=? AND npc_entity_id=?`, []any{turn.TurnRunID, M2AgentAdaID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_npc_decisions WHERE parent_turn_id=? AND npc_entity_id=?`, []any{turn.PlayerTurnID, M2AgentAdaID}, 0)
}

func TestRPExternalControllerHandoffDuringAcceptedSpeechFencesProvider(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "controller-handoff.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	human, read, _ := newRPWaitTestSession(t, ctx, s)
	for _, row := range []struct{ id, kind string }{{"principal_f3_operator", "operator"}, {"principal_f3_service", "service"}} {
		if _, err := s.db.ExecContext(ctx, `INSERT INTO principals(principal_id,principal_type,display_name,status) VALUES (?,?,?,'active')`, row.id, row.kind, row.id); err != nil {
			t.Fatal(err)
		}
	}
	head := func() int64 {
		var value int64
		if err := s.db.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	binding := func(key string) core.CareerBinding {
		return core.CareerBinding{PrincipalID: "principal_f3_operator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: head(), IdempotencyKey: key}
	}
	if _, err := s.EnrollRPExternalControllerLocal(ctx, RPExternalControllerEnrollmentRequest{Binding: binding("enroll-hearer"), EntityID: M2RPNPCID, ControllerPrincipalID: "principal_f3_service", ControllerInstanceID: "controller-hearer"}); err != nil {
		t.Fatal(err)
	}
	view, err := s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	assign := func(key string) (RPExternalControllerAssignment, error) {
		return s.AssignRPExternalControllerLocal(ctx, RPExternalControllerAssignmentRequest{Binding: binding(key), EntityID: M2RPNPCID, ExpectedGeneration: 0})
	}
	s.afterRPTurnStage = func(stage string) error {
		if stage == "turn_open" {
			if _, err := assign("too-early"); !core.HasCode(err, core.CodeCommandInProgress) {
				return core.NewError(core.CodeInjectedFailure, "open turn intent allowed premature handoff")
			}
		}
		return nil
	}
	providerCalls := 0
	provider := rpDecisionProviderFunc(func(ctx context.Context, input core.RPDecisionInput) (core.RPDecisionProposal, error) {
		providerCalls++
		if _, err := assign("handoff-during-provider"); err != nil {
			return core.RPDecisionProposal{}, err
		}
		return core.DeterministicRPDecisionProvider{}.Propose(ctx, input)
	})
	request := core.RPSpeechRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: human.SessionID, Text: "你听见我了吗？", ExpectedCursor: view.ObservationCursor, IdempotencyKey: "handoff-speech"}
	turn, err := s.RunRPTurn(ctx, request, provider)
	s.afterRPTurnStage = nil
	if err != nil || turn.Status != "settled" || providerCalls != 1 {
		t.Fatal("accepted speech failed during controller handoff", turn, providerCalls, err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_turn_listener_skips WHERE turn_run_id=? AND npc_entity_id=?`, []any{turn.TurnRunID, M2RPNPCID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_turn_listener_activations WHERE turn_run_id=? AND npc_entity_id=? AND disposition='externally_controlled' AND reason_code='external_controller'`, []any{turn.TurnRunID, M2RPNPCID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_npc_decisions WHERE parent_turn_id=? AND npc_entity_id=?`, []any{turn.PlayerTurnID, M2RPNPCID}, 0)
	if again, err := s.RunRPTurn(ctx, request, nil); err != nil || !again.Replayed || again.TurnRunID != turn.TurnRunID {
		t.Fatal("accepted speech replay after handoff", again, err)
	}
	service, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: "principal_f3_service", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2RPNPCID, POV: "second_person", IdempotencyKey: "hearer-control"})
	if err != nil || service.ControlGeneration != 1 {
		t.Fatal("external owner after handoff", service, err)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("handoff projection", diff, err)
	}
}

func TestRPExternalControllerHandoffResumesAcceptedTurnAfterRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "controller-handoff-restart.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Close() }()
	human, read, _ := newRPWaitTestSession(t, ctx, s)
	for _, row := range []struct{ id, kind string }{{"principal_f3_operator", "operator"}, {"principal_f3_service", "service"}} {
		if _, err := s.db.ExecContext(ctx, `INSERT INTO principals(principal_id,principal_type,display_name,status) VALUES (?,?,?,'active')`, row.id, row.kind, row.id); err != nil {
			t.Fatal(err)
		}
	}
	head := func() int64 {
		var value int64
		if err := s.db.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	binding := func(key string) core.CareerBinding {
		return core.CareerBinding{PrincipalID: "principal_f3_operator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: head(), IdempotencyKey: key}
	}
	if _, err := s.EnrollRPExternalControllerLocal(ctx, RPExternalControllerEnrollmentRequest{Binding: binding("restart-enroll"), EntityID: M2RPNPCID, ControllerPrincipalID: "principal_f3_service", ControllerInstanceID: "controller-restart"}); err != nil {
		t.Fatal(err)
	}
	view, err := s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	request := core.RPSpeechRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: human.SessionID, Text: "请等我回来。", ExpectedCursor: view.ObservationCursor, IdempotencyKey: "restart-handoff-speech"}
	s.afterRPTurnStage = func(stage string) error {
		if stage == "player_committed" {
			return core.NewError(core.CodeInjectedFailure, "lost response after speech Event")
		}
		return nil
	}
	if _, err := s.RunRPTurn(ctx, request, core.DeterministicRPDecisionProvider{}); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatal("speech did not pause after accepted Event", err)
	}
	s.afterRPTurnStage = nil
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_turn_runs WHERE session_id=? AND status='player_committed' AND player_event_id IS NOT NULL`, []any{human.SessionID}, 1)
	if _, err := s.AssignRPExternalControllerLocal(ctx, RPExternalControllerAssignmentRequest{Binding: binding("restart-assign"), EntityID: M2RPNPCID, ExpectedGeneration: 0}); err != nil {
		t.Fatal("handoff during accepted turn", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	turn, err := s.RunRPTurn(ctx, request, core.DeterministicRPDecisionProvider{})
	if err != nil || turn.Status != "settled" || !turn.Replayed {
		t.Fatal("accepted turn did not recover after restart and handoff", turn, err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_turn_listener_skips WHERE turn_run_id=? AND npc_entity_id=?`, []any{turn.TurnRunID, M2RPNPCID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_turn_listener_activations WHERE turn_run_id=? AND npc_entity_id=? AND disposition='externally_controlled' AND reason_code='external_controller'`, []any{turn.TurnRunID, M2RPNPCID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_npc_decisions WHERE parent_turn_id=? AND npc_entity_id=?`, []any{turn.PlayerTurnID, M2RPNPCID}, 0)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_utterances WHERE session_id=? AND speaker_entity_id=? AND turn_id=?`, []any{human.SessionID, M2RPPlayerID, turn.PlayerTurnID}, 1)
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("recovered handoff projection", diff, err)
	}
}

func TestRPExternalControllerHandoffRefusesPendingWait(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "controller-pending-wait.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	human, read, _ := newRPWaitTestSession(t, ctx, s)
	for _, row := range []struct{ id, kind string }{{"principal_f3_operator", "operator"}, {"principal_f3_service", "service"}} {
		if _, err := s.db.ExecContext(ctx, `INSERT INTO principals(principal_id,principal_type,display_name,status) VALUES (?,?,?,'active')`, row.id, row.kind, row.id); err != nil {
			t.Fatal(err)
		}
	}
	var head int64
	if err := s.db.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&head); err != nil {
		t.Fatal(err)
	}
	binding := core.CareerBinding{PrincipalID: "principal_f3_operator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: head, IdempotencyKey: "pending-wait-enroll"}
	if _, err := s.EnrollRPExternalControllerLocal(ctx, RPExternalControllerEnrollmentRequest{Binding: binding, EntityID: M2RPNPCID, ControllerPrincipalID: "principal_f3_service", ControllerInstanceID: "controller-pending-wait"}); err != nil {
		t.Fatal(err)
	}
	view, err := s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	start, err := time.Parse(time.RFC3339, view.WorldTime)
	if err != nil {
		t.Fatal(err)
	}
	wait := core.RPWaitRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: human.SessionID, TargetWorldTime: start.Add(time.Minute).Format(time.RFC3339), Budget: 100, ExpectedCursor: view.ObservationCursor, IdempotencyKey: "accepted-pending-wait"}
	hash, err := core.HashJSON(wait)
	if err != nil {
		t.Fatal(err)
	}
	if intent, replayed, err := s.ensureRPWaitIntent(ctx, wait, hash); err != nil || replayed || intent.Status != "pending" {
		t.Fatal("accept pending wait", intent, replayed, err)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&head); err != nil {
		t.Fatal(err)
	}
	binding.ExpectedHead, binding.IdempotencyKey = head, "assignment-during-wait"
	if _, err := s.AssignRPExternalControllerLocal(ctx, RPExternalControllerAssignmentRequest{Binding: binding, EntityID: M2RPNPCID, ExpectedGeneration: 0}); !core.HasCode(err, core.CodeCommandInProgress) {
		t.Fatal("pending world wait did not fence controller handoff", err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_controller_authorities WHERE entity_id=?`, []any{M2RPNPCID}, 0)
}

func TestRPExternalControllerAssignmentTwoDBContendersHaveOneSource(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "controller-assignment-race.db")
	first := openM2AgentStore(t, ctx, path, false)
	defer first.Close()
	for _, row := range []struct{ id, kind string }{{"principal_f3_operator", "operator"}, {"principal_f3_service", "service"}} {
		if _, err := first.db.ExecContext(ctx, `INSERT INTO principals(principal_id,principal_type,display_name,status) VALUES (?,?,?,'active')`, row.id, row.kind, row.id); err != nil {
			t.Fatal(err)
		}
	}
	var head int64
	if err := first.db.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&head); err != nil {
		t.Fatal(err)
	}
	binding := core.CareerBinding{PrincipalID: "principal_f3_operator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: head, IdempotencyKey: "race-enroll"}
	if _, err := first.EnrollRPExternalControllerLocal(ctx, RPExternalControllerEnrollmentRequest{Binding: binding, EntityID: M2AgentAdaID, ControllerPrincipalID: "principal_f3_service", ControllerInstanceID: "controller-race"}); err != nil {
		t.Fatal(err)
	}
	if err := first.db.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&head); err != nil {
		t.Fatal(err)
	}
	second, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	stores := []*Store{first, second}
	results := make([]error, len(stores))
	start := make(chan struct{})
	var workers sync.WaitGroup
	for i := range stores {
		workers.Add(1)
		go func(i int) {
			defer workers.Done()
			<-start
			_, results[i] = stores[i].AssignRPExternalControllerLocal(ctx, RPExternalControllerAssignmentRequest{Binding: core.CareerBinding{PrincipalID: "principal_f3_operator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: head, IdempotencyKey: []string{"race-assign-a", "race-assign-b"}[i]}, EntityID: M2AgentAdaID, ExpectedGeneration: 0})
		}(i)
	}
	close(start)
	workers.Wait()
	accepted, conflict := 0, 0
	for _, err := range results {
		switch {
		case err == nil:
			accepted++
		case core.HasCode(err, core.CodeBranchConflict):
			conflict++
		default:
			t.Fatal("unexpected assignment contender result", results)
		}
	}
	if accepted != 1 || conflict != 1 {
		t.Fatal("controller assignment did not select one source", results)
	}
	assertM2Value(t, ctx, first, `SELECT COUNT(*) FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPExternalControllerAssigned'`, []any{M2DemoInstanceID, M2DemoBranchID}, 1)
	if diff, err := first.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("contended authority projection", diff, err)
	}
}
