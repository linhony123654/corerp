package storage

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPSharedWorkTaskRequiresHumanAndRecoversCommittedEvent(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "shared-work.db")
	s := openCareerTestWorld(t, path)
	defer func() { s.Close() }()
	offer := prepareCareerEmploymentOffer(t, s)
	accepted, err := s.AcceptCareerOffer(ctx, core.CareerOfferAcceptRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "shared-work-accept"),
		OfferID: offer.Fact.RecordID, AfterWorkPlaceID: M2AgentCafeID})
	if err != nil {
		t.Fatal(err)
	}
	contract := accepted.Fact.Employment.ContractID
	human, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: M2RPPlayerPrincipal, InstanceID: M2DemoInstanceID,
		BranchID: M2DemoBranchID, EntityID: M2RPPlayerID, POV: "second_person", IdempotencyKey: "shared-work-human"})
	if err != nil {
		t.Fatal(err)
	}
	humanRead := core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: human.SessionID}
	humanView, err := s.ObserveRPSession(ctx, humanRead)
	if err != nil {
		t.Fatal(err)
	}
	if result, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: humanRead.PrincipalID, SessionID: humanRead.SessionID,
		ExpectedCursor: humanView.ObservationCursor, TargetWorldTime: "2026-09-23T09:00:00Z", Budget: 1000,
		IdempotencyKey: "shared-work-shift"}); err != nil || result.Status != "completed" {
		t.Fatal("reach sourced work shift", result, err)
	}
	for _, principal := range []struct{ id, kind string }{{"principal_shared_work_operator", "operator"}, {"principal_shared_work_service", "service"}} {
		if _, err := s.db.ExecContext(ctx, `INSERT INTO principals(principal_id,principal_type,display_name,status) VALUES (?,?,?,'active')`, principal.id, principal.kind, principal.id); err != nil {
			t.Fatal(err)
		}
	}
	operatorBinding := func(key string) core.CareerBinding {
		return careerTestBinding(t, s, "principal_shared_work_operator", key)
	}
	if _, err := s.EnrollRPExternalControllerLocal(ctx, RPExternalControllerEnrollmentRequest{Binding: operatorBinding("shared-work-enroll"),
		EntityID: M2AgentAdaID, ControllerPrincipalID: "principal_shared_work_service", ControllerInstanceID: "shared-work-controller"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AssignRPExternalControllerLocal(ctx, RPExternalControllerAssignmentRequest{Binding: operatorBinding("shared-work-assign"),
		EntityID: M2AgentAdaID, ExpectedGeneration: 0}); err != nil {
		t.Fatal(err)
	}
	external, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: "principal_shared_work_service", InstanceID: M2DemoInstanceID,
		BranchID: M2DemoBranchID, EntityID: M2AgentAdaID, POV: "second_person", IdempotencyKey: "shared-work-external"})
	if err != nil {
		t.Fatal(err)
	}
	externalRead := core.RPSessionReadRequest{PrincipalID: "principal_shared_work_service", SessionID: external.SessionID}
	view, err := s.ObserveRPSession(ctx, externalRead)
	if err != nil || view.PlaceID != "place_m2_work_ada" {
		t.Fatal("external employee not at sourced workplace", view, err)
	}
	if _, err := s.AttemptRPWorkTask(ctx, RPWorkTaskRequest{Binding: core.CareerBinding{PrincipalID: externalRead.PrincipalID,
		InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: view.ObservationCursor, IdempotencyKey: "direct-shared-work"},
		SessionID: externalRead.SessionID, ContractID: contract, TaskCode: "routine_check"}); !core.HasCode(err, core.CodeCommandInProgress) {
		t.Fatal("external work task bypassed shared round", err)
	}
	if _, err := s.ObserveRPSession(ctx, humanRead); err != nil {
		t.Fatal(err)
	}
	round, err := s.OpenRPSharedRoundLocal(ctx, RPSharedRoundOpenRequest{Binding: operatorBinding("shared-work-round"),
		HumanSessionID: humanRead.SessionID, ExternalSessionIDs: []string{externalRead.SessionID}})
	if err != nil {
		t.Fatal(err)
	}
	proposal := RPSharedWorkTaskRequest{PrincipalID: externalRead.PrincipalID, SessionID: externalRead.SessionID,
		RoundID: round.RoundID, ContractID: contract, TaskCode: "routine_check", IdempotencyKey: "shared-work-proposal"}
	unknown := proposal
	unknown.ContractID, unknown.IdempotencyKey = "contract_unknown", "unknown-work-contract"
	if _, err := s.SubmitRPSharedWorkTask(ctx, unknown); !core.HasCode(err, core.CodeNotFound) {
		t.Fatal("external participant probed a non-owned work contract", err)
	}
	if _, err := s.SubmitRPSharedWorkTask(ctx, RPSharedWorkTaskRequest{PrincipalID: humanRead.PrincipalID,
		SessionID: externalRead.SessionID, RoundID: round.RoundID, ContractID: contract, TaskCode: "routine_check",
		IdempotencyKey: "impersonate"}); err == nil {
		t.Fatal("Human impersonated external employee")
	}
	if submitted, err := s.SubmitRPSharedWorkTask(ctx, proposal); err != nil || submitted.Submitted != 1 {
		t.Fatal("private work proposal", submitted, err)
	}
	if replay, err := s.SubmitRPSharedWorkTask(ctx, proposal); err != nil || !replay.Replayed {
		t.Fatal("work proposal exact replay", replay, err)
	}
	advance := RPSharedRoundAdvanceRequest{RPSharedRoundReadRequest: RPSharedRoundReadRequest{PrincipalID: externalRead.PrincipalID,
		SessionID: externalRead.SessionID, RoundID: round.RoundID}, Budget: 1000}
	if _, err := s.AdvanceRPSharedRound(ctx, advance); !core.HasCode(err, core.CodeCommandInProgress) {
		t.Fatal("work Event accepted before Human response", err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='RPWorkTaskAttempted'`, nil, 0)
	if _, err := s.SubmitRPSharedWait(ctx, RPSharedWaitRequest{PrincipalID: humanRead.PrincipalID, SessionID: humanRead.SessionID,
		RoundID: round.RoundID, HorizonWorldTime: "2026-09-23T09:10:00Z", IdempotencyKey: "human-shared-work-wait"}); err != nil {
		t.Fatal(err)
	}
	lost := errors.New("work Event committed before receipt")
	s.afterRPSharedActionStage = func(stage string) error {
		if stage == "health_event_committed" {
			return lost
		}
		return nil
	}
	if _, err := s.AdvanceRPSharedRound(ctx, advance); !errors.Is(err, lost) {
		t.Fatal("shared work split recovery not exercised", err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='RPWorkTaskAttempted'`, nil, 1)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	settled, err := s.AdvanceRPSharedRound(ctx, advance)
	if err != nil || settled.Status != "settled" || settled.OwnDisposition != "action_accepted" {
		t.Fatal("shared work did not settle after restart", settled, err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='RPWorkTaskAttempted'`, nil, 1)
	if replay, err := s.AdvanceRPSharedRound(ctx, advance); err != nil || !replay.Replayed || replay.EventSequence != settled.EventSequence {
		t.Fatal("shared work settlement replay", replay, err)
	}
	var eventID, raw string
	if err := s.db.QueryRowContext(ctx, `SELECT event_id,payload FROM events WHERE event_type='RPWorkTaskAttempted'`).Scan(&eventID, &raw); err != nil {
		t.Fatal(err)
	}
	var fact RPWorkTaskFact
	if err := json.Unmarshal([]byte(raw), &fact); err != nil || fact.Outcome != "completed" {
		t.Fatal("typed work outcome", fact, err)
	}
	receiptJSON, err := json.Marshal(settled)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(receiptJSON), eventID) || strings.Contains(string(receiptJSON), fact.EntityID) || strings.Contains(string(receiptJSON), "fatigue") {
		t.Fatal("shared work receipt exposed private task truth", settled)
	}
	if humanReceipt, err := s.ReadRPSharedRound(ctx, RPSharedRoundReadRequest{PrincipalID: humanRead.PrincipalID,
		SessionID: humanRead.SessionID, RoundID: round.RoundID}); err != nil || humanReceipt.OwnDisposition != "deferred_no_effect" {
		t.Fatal("Human work receipt", humanReceipt, err)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("shared work source/replay mismatch", diff, err)
	}
}
