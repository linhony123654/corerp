package storage

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"corerp.local/backend/internal/core"
)

func TestRPSharedRoundThreeResidentsWaitTenMinutesOnlyOnce(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "shared-round.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Close() }()
	human, humanRead, _ := newRPWaitTestSession(t, ctx, s)
	for _, row := range []struct{ id, kind string }{{"principal_f3_operator", "operator"}, {"principal_f3_service_a", "service"}, {"principal_f3_service_b", "service"}} {
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
	for i, entity := range []string{M2AgentAdaID, M2AgentBoID} {
		principal := []string{"principal_f3_service_a", "principal_f3_service_b"}[i]
		controller := []string{"controller-round-a", "controller-round-b"}[i]
		if _, err := s.EnrollRPExternalControllerLocal(ctx, RPExternalControllerEnrollmentRequest{Binding: binding("enroll-round-" + controller), EntityID: entity, ControllerPrincipalID: principal, ControllerInstanceID: controller}); err != nil {
			t.Fatal(err)
		}
		if _, err := s.AssignRPExternalControllerLocal(ctx, RPExternalControllerAssignmentRequest{Binding: binding("assign-round-" + controller), EntityID: entity, ExpectedGeneration: 0}); err != nil {
			t.Fatal(err)
		}
	}
	reads := []core.RPSessionReadRequest{humanRead}
	for i, entity := range []string{M2AgentAdaID, M2AgentBoID} {
		principal := []string{"principal_f3_service_a", "principal_f3_service_b"}[i]
		session, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: principal, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: entity, POV: "second_person", IdempotencyKey: "round-session-" + principal})
		if err != nil {
			t.Fatal(err)
		}
		reads = append(reads, core.RPSessionReadRequest{PrincipalID: principal, SessionID: session.SessionID})
	}
	for _, read := range reads {
		if _, err := s.ObserveRPSession(ctx, read); err != nil {
			t.Fatal(err)
		}
	}
	var baseline string
	if err := s.db.QueryRowContext(ctx, `SELECT current_world_time FROM world_clocks WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&baseline); err != nil {
		t.Fatal(err)
	}
	start, err := time.Parse(time.RFC3339, baseline)
	if err != nil {
		t.Fatal(err)
	}
	horizon := start.Add(10 * time.Minute).Format(time.RFC3339)
	openRequest := RPSharedRoundOpenRequest{Binding: binding("shared-round-one"), HumanSessionID: human.SessionID, ExternalSessionIDs: []string{reads[1].SessionID, reads[2].SessionID}}
	round, err := s.OpenRPSharedRoundLocal(ctx, openRequest)
	if err != nil || round.Required != 3 || round.Submitted != 0 || round.Status != "open" {
		t.Fatal("open shared round", round, err)
	}
	if again, err := s.OpenRPSharedRoundLocal(ctx, openRequest); err != nil || !again.Replayed || again.RoundID != round.RoundID {
		t.Fatal("shared round open retry", again, err)
	}
	otherRound := openRequest
	otherRound.Binding = binding("shared-round-two")
	if _, err := s.OpenRPSharedRoundLocal(ctx, otherRound); !core.HasCode(err, core.CodeCommandInProgress) {
		t.Fatal("second active round was not rejected", err)
	}
	if _, err := s.CloseRPSession(ctx, humanRead); !core.HasCode(err, core.CodeCommandInProgress) {
		t.Fatal("Human closed an active round", err)
	}
	if _, err := s.CloseRPSession(ctx, reads[1]); !core.HasCode(err, core.CodeCommandInProgress) {
		t.Fatal("external controller closed an active round", err)
	}
	advance := RPSharedRoundAdvanceRequest{RPSharedRoundReadRequest: RPSharedRoundReadRequest{PrincipalID: reads[1].PrincipalID, SessionID: reads[1].SessionID, RoundID: round.RoundID}, Budget: 100}
	for i, read := range reads[1:] {
		proposal := RPSharedWaitRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, RoundID: round.RoundID, HorizonWorldTime: horizon, IdempotencyKey: "wait-ten-" + read.PrincipalID}
		out, err := s.SubmitRPSharedWait(ctx, proposal)
		if err != nil || out.Submitted != i+1 || out.CurrentWorldTime != baseline {
			t.Fatal("external proposal moved time", out, err)
		}
		if replayed, err := s.SubmitRPSharedWait(ctx, proposal); err != nil || !replayed.Replayed || replayed.Submitted != i+1 {
			t.Fatal("external proposal retry", replayed, err)
		}
	}
	if _, err := s.AdvanceRPSharedRound(ctx, advance); !core.HasCode(err, core.CodeCommandInProgress) {
		t.Fatal("external model bypassed missing Human window", err)
	}
	assertRPWorldTime(t, ctx, s, baseline)
	if _, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: reads[1].PrincipalID, SessionID: reads[1].SessionID, TargetWorldTime: horizon, Budget: 100, ExpectedCursor: head(), IdempotencyKey: "unilateral-external"}); !core.HasCode(err, core.CodeCommandInProgress) {
		t.Fatal("legacy external wait bypassed round", err)
	}
	if _, err := s.SubmitRPSharedWait(ctx, RPSharedWaitRequest{PrincipalID: "principal_f3_service_a", SessionID: reads[2].SessionID, RoundID: round.RoundID, HorizonWorldTime: horizon, IdempotencyKey: "tamper"}); err == nil {
		t.Fatal("foreign principal submitted another participant's wait")
	}
	if _, err := s.ReadRPSharedRound(ctx, RPSharedRoundReadRequest{PrincipalID: "principal_f3_operator", SessionID: reads[1].SessionID, RoundID: round.RoundID}); err == nil {
		t.Fatal("operator impersonated participant")
	}
	humanWait := RPSharedWaitRequest{PrincipalID: humanRead.PrincipalID, SessionID: humanRead.SessionID, RoundID: round.RoundID, HorizonWorldTime: horizon, IdempotencyKey: "human-ten"}
	if out, err := s.SubmitRPSharedWait(ctx, humanWait); err != nil || out.Submitted != 3 {
		t.Fatal("Human proposal", out, err)
	}
	service, err := NewRPService(s, core.DeterministicRPDecisionProvider{}, "deterministic")
	if err != nil {
		t.Fatal(err)
	}
	settled, err := service.AdvanceRPSharedRound(ctx, advance)
	if err != nil || settled.Status != "settled" || settled.CurrentWorldTime != horizon || settled.EventSequence == 0 {
		t.Fatal("shared round did not advance once", settled, err)
	}
	assertRPWorldTime(t, ctx, s, horizon)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPWaitCompleted'`, []any{M2DemoInstanceID, M2DemoBranchID}, 1)
	if replayed, err := service.AdvanceRPSharedRound(ctx, advance); err != nil || !replayed.Replayed || replayed.EventSequence != settled.EventSequence {
		t.Fatal("shared advance replay duplicated world time", replayed, err)
	}
	for _, read := range reads {
		view, err := s.ReadRPSharedRound(ctx, RPSharedRoundReadRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, RoundID: round.RoundID})
		if err != nil || view.CurrentWorldTime != horizon || view.EventSequence != settled.EventSequence {
			t.Fatal("participant cannot recover shared result", view, err)
		}
		encoded, _ := json.Marshal(view)
		if strings.Contains(string(encoded), "principal_f3_service_") || strings.Contains(string(encoded), M2AgentAdaID) || strings.Contains(string(encoded), M2AgentBoID) || strings.Contains(string(encoded), "event_rp_wait_") {
			t.Fatal("round receipt disclosed other controller identity", string(encoded))
		}
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("shared time projection diverged", diff, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	service, err = NewRPService(s, core.DeterministicRPDecisionProvider{}, "deterministic")
	if err != nil {
		t.Fatal(err)
	}
	if replayed, err := service.AdvanceRPSharedRound(ctx, advance); err != nil || !replayed.Replayed || replayed.EventSequence != settled.EventSequence {
		t.Fatal("restarted shared advance replay duplicated world time", replayed, err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPWaitCompleted'`, []any{M2DemoInstanceID, M2DemoBranchID}, 1)
	for _, read := range reads {
		if _, err := s.ObserveRPSession(ctx, read); err != nil {
			t.Fatal(err)
		}
	}
	otherRound.Binding = binding("shared-round-two")
	stale, err := s.OpenRPSharedRoundLocal(ctx, otherRound)
	if err != nil {
		t.Fatal(err)
	}
	segment, err := s.MaterializeRPLocation(ctx, RPLocationMaterializeRequest{
		Binding:          core.CareerBinding{PrincipalID: "principal_creator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: head(), IdempotencyKey: "round-stale-place"},
		ParentLocationID: M2AgentCafeID, SlotKey: "round-stale-place",
		Candidate: RPLocationCandidate{DisplayName: "轮次外变化", GeneratorVersion: "local-v1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SubmitRPSharedWait(ctx, RPSharedWaitRequest{PrincipalID: reads[1].PrincipalID, SessionID: reads[1].SessionID, RoundID: stale.RoundID, HorizonWorldTime: start.Add(20 * time.Minute).Format(time.RFC3339), IdempotencyKey: "stale-proposal"}); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatal("stale baseline accepted", err)
	}
	if view, err := s.ReadRPSharedRound(ctx, RPSharedRoundReadRequest{PrincipalID: reads[1].PrincipalID, SessionID: reads[1].SessionID, RoundID: stale.RoundID}); err != nil || view.Status != "stale" {
		t.Fatal("stale round retained active slot", view, err)
	}
	if _, err := s.DefineRPTimedEdge(ctx, RPTimedEdgeRequest{
		Binding:     core.CareerBinding{PrincipalID: "principal_creator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: head(), IdempotencyKey: "round-journey-edge"},
		FromPlaceID: M2AgentCafeID, ToPlaceID: "place_m2_home_ada", SegmentPlaceID: segment.Fact.LocationID, DurationMinutes: 15,
	}); err != nil {
		t.Fatal(err)
	}
	humanView, err := s.ObserveRPSession(ctx, humanRead)
	if err != nil {
		t.Fatal(err)
	}
	journey, err := s.StartRPJourney(ctx, core.RPMoveRequest{PrincipalID: humanRead.PrincipalID, SessionID: humanRead.SessionID, FromPlaceID: humanView.PlaceID, ToPlaceID: "place_m2_home_ada", ExpectedCursor: humanView.ObservationCursor, IdempotencyKey: "round-journey"})
	if err != nil {
		t.Fatal(err)
	}
	for _, read := range reads {
		if _, err := s.ObserveRPSession(ctx, read); err != nil {
			t.Fatal(err)
		}
	}
	otherRound.Binding = binding("shared-round-three")
	next, err := s.OpenRPSharedRoundLocal(ctx, otherRound)
	if err != nil || next.Status != "open" {
		t.Fatal("new baseline could not open", next, err)
	}
	for _, read := range reads {
		if _, err := s.SubmitRPSharedWait(ctx, RPSharedWaitRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, RoundID: next.RoundID, HorizonWorldTime: start.Add(30 * time.Minute).Format(time.RFC3339), IdempotencyKey: "wait-past-arrival-" + read.PrincipalID}); err != nil {
			t.Fatal(err)
		}
	}
	arrivalAdvance := RPSharedRoundAdvanceRequest{RPSharedRoundReadRequest: RPSharedRoundReadRequest{PrincipalID: humanRead.PrincipalID, SessionID: humanRead.SessionID, RoundID: next.RoundID}, Budget: 100}
	arrived, err := service.AdvanceRPSharedRound(ctx, arrivalAdvance)
	if err != nil || arrived.Status != "settled" || arrived.CurrentWorldTime != journey.ScheduledArrivalAt {
		t.Fatal("shared round skipped nearest journey boundary", arrived, journey, err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_journeys WHERE journey_id=? AND status='arrived'`, []any{journey.JourneyID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPWaitCompleted'`, []any{M2DemoInstanceID, M2DemoBranchID}, 2)
	if replayed, err := service.AdvanceRPSharedRound(ctx, arrivalAdvance); err != nil || !replayed.Replayed || replayed.EventSequence != arrived.EventSequence {
		t.Fatal("journey-boundary replay duplicated due work", replayed, err)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("journey-boundary projection diverged", diff, err)
	}
	t.Run("migration040 upgrades settled legacy wait receipts", func(t *testing.T) {
		// Reconstruct the pre-040 schema in this disposable 038/039 world,
		// preserving both settled wait rounds and their Event references.
		for _, statement := range []string{
			`DROP TABLE rp_shared_round_actions`,
			`ALTER TABLE rp_shared_rounds DROP COLUMN selected_action_kind`,
			`ALTER TABLE rp_shared_rounds DROP COLUMN settlement_kind`,
			`ALTER TABLE rp_shared_rounds DROP COLUMN selected_session_id`,
			`ALTER TABLE rp_shared_rounds RENAME COLUMN completion_event_id TO wait_event_id`,
			`DELETE FROM schema_meta WHERE schema_version IN ('corerp-f3-shared-action-rounds-040-2026-09-25','corerp-f3-shared-move-rounds-041-2026-09-25')`,
		} {
			if _, err := s.db.ExecContext(ctx, statement); err != nil {
				t.Fatalf("restore legacy schema %q: %v", statement, err)
			}
		}
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
		s, err = Open(ctx, path)
		if err != nil {
			t.Fatal("040 upgrade", err)
		}
		for _, version := range []string{RPSharedActionSchemaVersion, RPSharedMoveSchemaVersion} {
			assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM schema_meta WHERE schema_version=?`, []any{version}, 1)
		}
		for _, previous := range []struct {
			id       string
			sequence int64
		}{
			{round.RoundID, settled.EventSequence}, {next.RoundID, arrived.EventSequence},
		} {
			view, err := s.ReadRPSharedRound(ctx, RPSharedRoundReadRequest{PrincipalID: reads[1].PrincipalID, SessionID: reads[1].SessionID, RoundID: previous.id})
			if err != nil || view.Status != "settled" || view.EventSequence != previous.sequence {
				t.Fatal("legacy wait receipt changed", view, err)
			}
		}
		if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
			t.Fatal("040 upgrade projection diverged", diff, err)
		}
	})
}
