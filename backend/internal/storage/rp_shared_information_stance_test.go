package storage

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"corerp.local/backend/internal/core"
)

func TestRPSharedInformationStanceRequiresDeliveredClaimHumanAndExactSelection(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "shared-information-stance.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	humanSession, human, _ := newRPWaitTestSession(t, ctx, s)
	head := func() int64 {
		var n int64
		if err := s.db.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	view, err := s.ObserveRPSession(ctx, human)
	if err != nil {
		t.Fatal(err)
	}
	sent, err := s.SendRPInformation(ctx, RPInformationSendRequest{Binding: core.CareerBinding{
		PrincipalID: human.PrincipalID, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID,
		ExpectedHead: view.ObservationCursor, IdempotencyKey: "stance-source-send",
	}, SessionID: human.SessionID, MessageID: "shared-stance-source", RecipientEntityID: M2RPNPCID,
		Text: "合作社下周会调整排班。"})
	if err != nil {
		t.Fatal(err)
	}
	view, err = s.ObserveRPSession(ctx, human)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: human.PrincipalID, SessionID: human.SessionID,
		TargetWorldTime: sent.Fact.DeliverWorldTime, Budget: 1000, ExpectedCursor: view.ObservationCursor,
		IdempotencyKey: "stance-source-deliver"}); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_knowledge WHERE observer_agent_id=? AND claim_key=?`,
		[]any{M2RPNPCID, "information:" + sent.EventID}, 1)
	for _, p := range []struct{ id, kind string }{{"principal_stance_operator", "operator"}, {"principal_stance_service", "service"}} {
		if _, err := s.db.ExecContext(ctx, `INSERT INTO principals(principal_id,principal_type,display_name,status) VALUES (?,?,?,'active')`, p.id, p.kind, p.id); err != nil {
			t.Fatal(err)
		}
	}
	binding := func(key string) core.CareerBinding {
		return core.CareerBinding{PrincipalID: "principal_stance_operator", InstanceID: M2DemoInstanceID,
			BranchID: M2DemoBranchID, ExpectedHead: head(), IdempotencyKey: key}
	}
	if _, err := s.EnrollRPExternalControllerLocal(ctx, RPExternalControllerEnrollmentRequest{Binding: binding("stance-enroll"),
		EntityID: M2RPNPCID, ControllerPrincipalID: "principal_stance_service", ControllerInstanceID: "stance-controller"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AssignRPExternalControllerLocal(ctx, RPExternalControllerAssignmentRequest{Binding: binding("stance-assign"),
		EntityID: M2RPNPCID, ExpectedGeneration: 0}); err != nil {
		t.Fatal(err)
	}
	externalSession, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: "principal_stance_service",
		InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2RPNPCID,
		POV: "second_person", IdempotencyKey: "stance-external-session"})
	if err != nil {
		t.Fatal(err)
	}
	external := core.RPSessionReadRequest{PrincipalID: "principal_stance_service", SessionID: externalSession.SessionID}
	for _, read := range []core.RPSessionReadRequest{human, external} {
		if _, err := s.ObserveRPSession(ctx, read); err != nil {
			t.Fatal(err)
		}
	}
	standalone := RPInformationStanceRequest{Binding: core.CareerBinding{PrincipalID: external.PrincipalID,
		InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: head(), IdempotencyKey: "stance-standalone"},
		SessionID: external.SessionID, MessageID: sent.Fact.MessageID, Stance: "doubt"}
	if _, err := s.RecordRPInformationStance(ctx, standalone); !core.HasCode(err, core.CodeCommandInProgress) {
		t.Fatal("external recipient recorded stance outside shared round", err)
	}
	round, err := s.OpenRPSharedRoundLocal(ctx, RPSharedRoundOpenRequest{Binding: binding("stance-round"),
		HumanSessionID: humanSession.SessionID, ExternalSessionIDs: []string{externalSession.SessionID}})
	if err != nil {
		t.Fatal(err)
	}
	baseline := head()
	proposal := RPSharedInformationStanceRequest{PrincipalID: external.PrincipalID, SessionID: external.SessionID,
		RoundID: round.RoundID, MessageID: sent.Fact.MessageID, Stance: "doubt",
		Reason: "The speaker may be mistaken.", IdempotencyKey: "propose-stance"}
	unknown := proposal
	unknown.MessageID = "undelivered-claim"
	unknown.IdempotencyKey = "propose-unknown-stance"
	if _, err := s.SubmitRPSharedInformationStance(ctx, unknown); !core.HasCode(err, core.CodeNotFound) {
		t.Fatal("undelivered claim proposed as recipient stance", err)
	}
	wrong := proposal
	wrong.PrincipalID = human.PrincipalID
	if _, err := s.SubmitRPSharedInformationStance(ctx, wrong); err == nil {
		t.Fatal("foreign principal proposed another participant's stance")
	}
	if submitted, err := s.SubmitRPSharedInformationStance(ctx, proposal); err != nil || submitted.Submitted != 1 {
		t.Fatal("recipient stance proposal", submitted, err)
	}
	if head() != baseline {
		t.Fatal("stance proposal wrote Event before Human submission")
	}
	if replay, err := s.SubmitRPSharedInformationStance(ctx, proposal); err != nil || !replay.Replayed {
		t.Fatal("stance proposal replay", replay, err)
	}
	changed := proposal
	changed.Reason = "Changed reason."
	if _, err := s.SubmitRPSharedInformationStance(ctx, changed); !core.HasCode(err, core.CodeIdempotencyMismatch) {
		t.Fatal("changed stance proposal reused same key", err)
	}
	advance := RPSharedRoundAdvanceRequest{RPSharedRoundReadRequest: RPSharedRoundReadRequest{
		PrincipalID: external.PrincipalID, SessionID: external.SessionID, RoundID: round.RoundID}, Budget: 100}
	if _, err := s.AdvanceRPSharedRound(ctx, advance); !core.HasCode(err, core.CodeCommandInProgress) {
		t.Fatal("external stance bypassed Human", err)
	}
	standalone.Binding.IdempotencyKey = "stance-direct-during-round"
	if _, err := s.RecordRPInformationStance(ctx, standalone); !core.HasCode(err, core.CodeCommandInProgress) {
		t.Fatal("standalone stance bypassed open round", err)
	}
	var now string
	if err := s.db.QueryRowContext(ctx, `SELECT current_world_time FROM world_clocks WHERE instance_id=? AND branch_id=?`,
		M2DemoInstanceID, M2DemoBranchID).Scan(&now); err != nil {
		t.Fatal(err)
	}
	start, err := time.Parse(time.RFC3339Nano, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SubmitRPSharedWait(ctx, RPSharedWaitRequest{PrincipalID: human.PrincipalID, SessionID: human.SessionID,
		RoundID: round.RoundID, HorizonWorldTime: start.Add(10 * time.Minute).Format(time.RFC3339Nano),
		IdempotencyKey: "stance-human-wait"}); err != nil {
		t.Fatal(err)
	}
	selectedFailure := errors.New("interrupt after stance selection")
	s.afterRPSharedActionStage = func(stage string) error {
		if stage == "selection_committed" {
			return selectedFailure
		}
		return nil
	}
	if _, err := s.AdvanceRPSharedRound(ctx, advance); !errors.Is(err, selectedFailure) || head() != baseline {
		t.Fatal("stance selection should not write Event", err)
	}
	standalone.Binding.IdempotencyKey = "shared_action_" + round.RoundID
	standalone.Reason = "Forged reason."
	if _, err := s.RecordRPInformationStance(ctx, standalone); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatal("selected child accepted changed reason", err)
	}
	standalone.Reason = proposal.Reason
	standalone.Stance = "believe"
	if _, err := s.RecordRPInformationStance(ctx, standalone); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatal("selected child accepted changed stance", err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='RPInformationStanceRecorded'`, nil, 0)
	eventFailure := errors.New("interrupt after durable stance Event")
	s.afterRPSharedActionStage = func(stage string) error {
		if stage == "information_stance_event_committed" {
			return eventFailure
		}
		return nil
	}
	if _, err := s.AdvanceRPSharedRound(ctx, advance); !errors.Is(err, eventFailure) || head() != baseline+1 {
		t.Fatal("stance Event-before-receipt stage", err, head())
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='RPInformationStanceRecorded' AND json_extract(payload,'$.message_id')=?`,
		[]any{proposal.MessageID}, 1)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	settled, err := s.AdvanceRPSharedRound(ctx, advance)
	if err != nil || settled.Status != "settled" || settled.OwnDisposition != "action_accepted" || settled.EventSequence != baseline+1 {
		t.Fatal("recover selected stance Event and settle", settled, err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='RPInformationStanceRecorded' AND json_extract(payload,'$.message_id')=?`,
		[]any{proposal.MessageID}, 1)
	if got := readCareerTestContext(t, s, M2RPNPCID).Life.Information; len(got) != 1 || got[0].Stance != "doubt" || got[0].Text != "合作社下周会调整排班。" {
		t.Fatal("selected stance missing from actual recipient model view", got)
	}
	if got := readCareerTestContext(t, s, M2RPPlayerID).Life.Information; len(got) != 0 {
		t.Fatal("sender learned recipient's private stance", got)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("shared stance source replay diverged", diff, err)
	}
}
