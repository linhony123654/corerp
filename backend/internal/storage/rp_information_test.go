package storage

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPInformationDirectMessageLearntOnlyAfterScheduledDelivery(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "information.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, lin, initial := newRPWaitTestSession(t, ctx, s)
	if initial.PlaceID != M2AgentCafeID {
		t.Fatal("unexpected sender place", initial.PlaceID)
	}
	if _, err := s.MoveRP(ctx, core.RPMoveRequest{PrincipalID: lin.PrincipalID, SessionID: lin.SessionID,
		FromPlaceID: initial.PlaceID, ToPlaceID: "place_m2_home_ada", ExpectedCursor: initial.ObservationCursor,
		IdempotencyKey: "information-separate"}); err != nil {
		t.Fatal("separate sender and recipient", err)
	}
	view, err := s.ObserveRPSession(ctx, lin)
	if err != nil {
		t.Fatal(err)
	}
	var recipientPlace string
	if err := s.db.QueryRowContext(ctx, `SELECT place_id FROM agent_positions WHERE agent_id=?`, M2RPNPCID).Scan(&recipientPlace); err != nil {
		t.Fatal(err)
	}
	if recipientPlace == view.PlaceID {
		t.Fatal("fixture sender and recipient are still co-located")
	}
	request := RPInformationSendRequest{Binding: core.CareerBinding{PrincipalID: lin.PrincipalID,
		InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: view.ObservationCursor,
		IdempotencyKey: "information-direct"}, SessionID: lin.SessionID, MessageID: "message-direct-1",
		RecipientEntityID: M2RPNPCID, Text: "我听说合作社明天要改变排班。"}
	unknown := request
	unknown.RecipientEntityID = M2AgentAdaID
	if _, err := s.SendRPInformation(ctx, unknown); !core.HasCode(err, core.CodeNotFound) {
		t.Fatal("unknown stable target accepted", err)
	}
	sent, err := s.SendRPInformation(ctx, request)
	if err != nil || sent.Fact.SenderID != M2RPPlayerID || sent.Fact.RecipientID != M2RPNPCID || sent.Fact.Visibility != "private" {
		t.Fatal("source-backed direct send", sent, err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='RPInformationSent' AND event_id=?`, []any{sent.EventID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM observation_records WHERE claim_key=?`, []any{"information:" + sent.EventID}, 0)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_knowledge WHERE claim_key=?`, []any{"information:" + sent.EventID}, 0)
	if input := readCareerTestContext(t, s, M2RPNPCID); len(input.Life.Information) != 0 {
		t.Fatal("recipient model learned before delivery", input.Life.Information)
	}
	if again, err := s.SendRPInformation(ctx, request); err != nil || !again.Replayed || again.EventID != sent.EventID {
		t.Fatal("exact send replay", again, err)
	}
	changed := request
	changed.Text = "另一个说法。"
	if _, err := s.SendRPInformation(ctx, changed); !core.HasCode(err, core.CodeIdempotencyMismatch) {
		t.Fatal("mismatched key reused", err)
	}
	queueID := "sched_rp_information_" + sent.EventID
	if _, err := s.db.ExecContext(ctx, `UPDATE scheduler_items SET status='completed' WHERE scheduler_item_id=?`, queueID); err != nil {
		t.Fatal(err)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) == 0 {
		t.Fatal("premature queue completion escaped source audit", diff, err)
	}
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal("restore pending delivery queue", err)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("pending source repair did not converge", diff, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal("reopen queued world", err)
	}
	defer s.Close()
	cai := allowFixtureControl(t, ctx, s, M2RPNPCID)
	if own, err := s.ReadRPContext(ctx, RPContextReadRequest{PrincipalID: cai.PrincipalID, SessionID: cai.SessionID}); err != nil {
		t.Fatal("pre-delivery recipient context", err)
	} else {
		for _, fact := range own.Facts {
			if fact.Kind == "message_received" {
				t.Fatal("recipient learnt at send time", fact)
			}
		}
	}
	view, err = s.ObserveRPSession(ctx, lin)
	if err != nil {
		t.Fatal(err)
	}
	wait, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: lin.PrincipalID, SessionID: lin.SessionID,
		TargetWorldTime: sent.Fact.DeliverWorldTime, Budget: 1000, ExpectedCursor: view.ObservationCursor,
		IdempotencyKey: "information-delivery-wait"})
	if err != nil || wait.Status != "completed" {
		t.Fatal("scheduled delivery wait", wait, err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM observation_records WHERE claim_key=? AND observer_agent_id=? AND channel='direct_message'`, []any{"information:" + sent.EventID, M2RPNPCID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_knowledge WHERE claim_key=? AND observer_agent_id=?`, []any{"information:" + sent.EventID, M2RPNPCID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_knowledge WHERE claim_key=? AND observer_agent_id IN (?,?)`, []any{"information:" + sent.EventID, M2AgentAdaID, M2AgentBoID}, 0)
	own, err := s.ReadRPContext(ctx, RPContextReadRequest{PrincipalID: cai.PrincipalID, SessionID: cai.SessionID})
	if err != nil {
		t.Fatal("delivered recipient context", err)
	}
	found := false
	for _, fact := range own.Facts {
		if fact.Kind == "message_received" && fact.Text == request.Text && fact.Channel == "direct_message" &&
			fact.Reliability == "unverified" && fact.SubjectEntityID == M2RPPlayerID && fact.PlaceID == "" {
			found = true
		}
	}
	if !found {
		t.Fatal("recipient lacks bounded direct message", own)
	}
	decision := readCareerTestContext(t, s, M2RPNPCID)
	if len(decision.Life.Information) != 1 || decision.Life.Information[0].Text != request.Text ||
		decision.Life.Information[0].Channel != "direct_message" ||
		decision.Life.Information[0].ClaimedReliability != "unverified" ||
		decision.Life.Information[0].SenderName != "Lin" ||
		!strings.HasPrefix(decision.Life.Information[0].SenderHandle, "person_") {
		t.Fatal("recipient decision lacks attributed uncertain claim", decision.Life.Information)
	}
	encoded, err := json.Marshal(decision.Life.Information)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{sent.EventID, M2RPPlayerID, M2RPNPCID, "source_event_id", "recipient_id", "sender_id"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("decision information exposed %q", secret)
		}
	}
	if input := readCareerTestContext(t, s, M2AgentAdaID); len(input.Life.Information) != 0 {
		t.Fatal("unaddressed Agent learned direct message", input.Life.Information)
	}
	events, err := s.ReadRPEvents(ctx, RPEventsReadRequest{PrincipalID: cai.PrincipalID, SessionID: cai.SessionID,
		After: sent.EventSequence, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	found = false
	for _, event := range events.Events {
		for _, fact := range event.Facts {
			if fact.Kind == "message_received" && fact.Text == request.Text && event.WorldTime == sent.Fact.DeliverWorldTime {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("delivery missing from recipient event cursor", events)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("delivered knowledge does not replay", diff, err)
	}
	claimKey := "information:" + sent.EventID
	if _, err := s.db.ExecContext(ctx, `UPDATE agent_knowledge SET claim_payload='{"claim_type":"message_received","text":"forged"}' WHERE claim_key=?`, claimKey); err != nil {
		t.Fatal(err)
	}
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		t.Fatal(err)
	}
	_, err = readRPOwnDecisionContext(ctx, tx.conn, core.RPDecisionInput{NPCEntityID: M2RPNPCID, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID})
	tx.Rollback(ctx)
	if !core.HasCode(err, core.CodeProjectionDiverged) {
		t.Fatal("forged message reached Agent decision", err)
	}
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal("repair forged decision knowledge", err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE agent_knowledge SET claim_payload='{"claim_type":"speaker_said","text":"pretend face-to-face"}' WHERE claim_key=?`, claimKey); err != nil {
		t.Fatal(err)
	}
	tx, err = beginImmediate(ctx, s.db)
	if err != nil {
		t.Fatal(err)
	}
	_, err = readRPOwnDecisionContext(ctx, tx.conn, core.RPDecisionInput{NPCEntityID: M2RPNPCID, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID})
	tx.Rollback(ctx)
	if !core.HasCode(err, core.CodeProjectionDiverged) {
		t.Fatal("private message masqueraded as speech in Agent input", err)
	}
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal("repair channel-spoofed decision knowledge", err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE observation_records SET claim_payload='{"claim_type":"message_received","text":"forged"}' WHERE claim_key=?`, claimKey); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE scheduler_items SET status='pending' WHERE scheduler_item_id=?`, "sched_rp_information_"+sent.EventID); err != nil {
		t.Fatal(err)
	}
	diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID)
	if err != nil {
		t.Fatal(err)
	}
	queueWrong, observationWrong := false, false
	for _, d := range diff {
		queueWrong = queueWrong || d.Projection == "rp_information_queue"
		observationWrong = observationWrong || d.Projection == "rp_information_observation"
	}
	if !queueWrong || !observationWrong {
		t.Fatal("source audit missed F7 projection damage", diff)
	}
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal("rebuild F7 source projections", err)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("F7 source repair did not converge", diff, err)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT claim_payload FROM observation_records WHERE claim_key=?`, claimKey).Scan(new(string)); err != nil {
		t.Fatal("repaired observation missing", err)
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM agent_knowledge WHERE claim_key=?`, claimKey); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM observation_records WHERE claim_key=?`, claimKey); err != nil {
		t.Fatal(err)
	}
	diff, err = s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID)
	if err != nil || len(diff) == 0 {
		t.Fatal("missing F7 observation was not detected", diff, err)
	}
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal("rebuild missing F7 observation", err)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("missing F7 observation did not recover", diff, err)
	}
}

func TestRPInformationDirectSendCannotBypassActiveSharedRound(t *testing.T) {
	ctx := context.Background()
	f := newRPSharedRecoveryFixture(t)
	defer f.store.Close()
	round, err := f.store.OpenRPSharedRoundLocal(ctx, RPSharedRoundOpenRequest{
		Binding:        rpSharedRecoveryBinding(t, f.store, "information-open-round"),
		HumanSessionID: f.human.SessionID, ExternalSessionIDs: []string{f.external.SessionID},
	})
	if err != nil || round.Status != "open" {
		t.Fatal("open shared decision window", round, err)
	}
	view, err := f.store.ObserveRPSession(ctx, f.human)
	if err != nil {
		t.Fatal(err)
	}
	request := RPInformationSendRequest{Binding: core.CareerBinding{
		PrincipalID: f.human.PrincipalID, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID,
		ExpectedHead: view.ObservationCursor, IdempotencyKey: "information-unilateral-human",
	}, SessionID: f.human.SessionID, MessageID: "information-unilateral-message",
		RecipientEntityID: M2RPNPCID, Text: "轮次内不得私自发信。"}
	if _, err := f.store.SendRPInformation(ctx, request); !core.HasCode(err, core.CodeCommandInProgress) {
		t.Fatal("Human direct send bypassed shared action window", err)
	}
	request.Binding.PrincipalID = f.external.PrincipalID
	request.SessionID = f.external.SessionID
	request.Binding.IdempotencyKey = "information-unilateral-external"
	if _, err := f.store.SendRPInformation(ctx, request); !core.HasCode(err, core.CodeCommandInProgress) {
		t.Fatal("external direct send bypassed shared action window", err)
	}
	assertM2Value(t, ctx, f.store, `SELECT COUNT(*) FROM events WHERE event_type='RPInformationSent'`, nil, 0)
	if diff, err := f.store.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("round denial diverged", diff, err)
	}
}

func TestRPInformationDeliveryRunsDuringAuthorizedSharedWait(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "information-shared-wait.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, human, view := newRPWaitTestSession(t, ctx, s)
	sent, err := s.SendRPInformation(ctx, RPInformationSendRequest{Binding: core.CareerBinding{
		PrincipalID: human.PrincipalID, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID,
		ExpectedHead: view.ObservationCursor, IdempotencyKey: "shared-delivery-send",
	}, SessionID: human.SessionID, MessageID: "shared-delivery-message",
		RecipientEntityID: M2RPNPCID, Text: "这条消息需要在共同等待时送达。"})
	if err != nil {
		t.Fatal("queue message before shared window", err)
	}
	for _, row := range []struct{ id, kind string }{{"principal_information_operator", "operator"}, {"principal_information_service", "service"}} {
		if _, err := s.db.ExecContext(ctx, `INSERT INTO principals(principal_id,principal_type,display_name,status) VALUES (?,?,?,'active')`, row.id, row.kind, row.id); err != nil {
			t.Fatal(err)
		}
	}
	operator := func(key string) core.CareerBinding {
		var head int64
		if err := s.db.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&head); err != nil {
			t.Fatal(err)
		}
		return core.CareerBinding{PrincipalID: "principal_information_operator", InstanceID: M2DemoInstanceID,
			BranchID: M2DemoBranchID, ExpectedHead: head, IdempotencyKey: key}
	}
	if _, err := s.EnrollRPExternalControllerLocal(ctx, RPExternalControllerEnrollmentRequest{
		Binding: operator("information-enroll"), EntityID: M2AgentAdaID,
		ControllerPrincipalID: "principal_information_service", ControllerInstanceID: "controller-information",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AssignRPExternalControllerLocal(ctx, RPExternalControllerAssignmentRequest{
		Binding: operator("information-assign"), EntityID: M2AgentAdaID, ExpectedGeneration: 0,
	}); err != nil {
		t.Fatal(err)
	}
	serviceSession, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{
		PrincipalID: "principal_information_service", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID,
		EntityID: M2AgentAdaID, POV: "second_person", IdempotencyKey: "information-external-session",
	})
	if err != nil {
		t.Fatal(err)
	}
	external := core.RPSessionReadRequest{PrincipalID: "principal_information_service", SessionID: serviceSession.SessionID}
	for _, read := range []core.RPSessionReadRequest{human, external} {
		if _, err := s.ObserveRPSession(ctx, read); err != nil {
			t.Fatal(err)
		}
	}
	round, err := s.OpenRPSharedRoundLocal(ctx, RPSharedRoundOpenRequest{
		Binding: operator("information-shared-round"), HumanSessionID: human.SessionID,
		ExternalSessionIDs: []string{external.SessionID},
	})
	if err != nil || round.Status != "open" {
		t.Fatal("open message delivery window", round, err)
	}
	for _, read := range []core.RPSessionReadRequest{external, human} {
		if _, err := s.SubmitRPSharedWait(ctx, RPSharedWaitRequest{
			PrincipalID: read.PrincipalID, SessionID: read.SessionID, RoundID: round.RoundID,
			HorizonWorldTime: sent.Fact.DeliverWorldTime, IdempotencyKey: "information-wait-" + read.PrincipalID,
		}); err != nil {
			t.Fatal("submit shared wait", read, err)
		}
	}
	provider, err := NewRPService(s, core.DeterministicRPDecisionProvider{}, "deterministic")
	if err != nil {
		t.Fatal(err)
	}
	settled, err := provider.AdvanceRPSharedRound(ctx, RPSharedRoundAdvanceRequest{
		RPSharedRoundReadRequest: RPSharedRoundReadRequest{PrincipalID: external.PrincipalID,
			SessionID: external.SessionID, RoundID: round.RoundID}, Budget: 1000,
	})
	if err != nil || settled.Status != "settled" {
		t.Fatal("shared wait delivery", settled, err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM observation_records WHERE channel='direct_message' AND claim_key=? AND observer_agent_id=?`, []any{"information:" + sent.EventID, M2RPNPCID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM observation_records WHERE channel='direct_message' AND claim_key=? AND observer_agent_id=?`, []any{"information:" + sent.EventID, M2AgentAdaID}, 0)
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("shared delivery projection diverged", diff, err)
	}
}

func TestRPInformationRecipientStanceCorrectionPreservesSourceHistory(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "information-stance.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, lin, view := newRPWaitTestSession(t, ctx, s)
	sent, err := s.SendRPInformation(ctx, RPInformationSendRequest{Binding: core.CareerBinding{
		PrincipalID: lin.PrincipalID, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID,
		ExpectedHead: view.ObservationCursor, IdempotencyKey: "stance-send",
	}, SessionID: lin.SessionID, MessageID: "stance-message",
		RecipientEntityID: M2RPNPCID, Text: "合作社下周可能停业。"})
	if err != nil {
		t.Fatal(err)
	}
	cai := allowFixtureControl(t, ctx, s, M2RPNPCID)
	caiView, err := s.ObserveRPSession(ctx, cai)
	if err != nil {
		t.Fatal(err)
	}
	stance := RPInformationStanceRequest{Binding: core.CareerBinding{
		PrincipalID: cai.PrincipalID, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID,
		ExpectedHead: caiView.ObservationCursor, IdempotencyKey: "stance-believe",
	}, SessionID: cai.SessionID, MessageID: sent.Fact.MessageID, Stance: "believe", Reason: "最初听着可信。"}
	if _, err := s.RecordRPInformationStance(ctx, stance); !core.HasCode(err, core.CodeNotFound) {
		t.Fatal("pre-delivery stance accepted", err)
	}
	view, err = s.ObserveRPSession(ctx, lin)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: lin.PrincipalID, SessionID: lin.SessionID,
		TargetWorldTime: sent.Fact.DeliverWorldTime, Budget: 1000,
		ExpectedCursor: view.ObservationCursor, IdempotencyKey: "stance-delivery-wait"}); err != nil {
		t.Fatal(err)
	}
	view, err = s.ObserveRPSession(ctx, cai)
	if err != nil {
		t.Fatal(err)
	}
	stance.Binding.ExpectedHead = view.ObservationCursor
	if _, err := s.ObserveRPSession(ctx, lin); err != nil {
		t.Fatal(err)
	}
	foreign := stance
	foreign.SessionID = lin.SessionID
	foreign.Binding.IdempotencyKey = "stance-foreign"
	if _, err := s.RecordRPInformationStance(ctx, foreign); !core.HasCode(err, core.CodeNotFound) {
		t.Fatal("non-recipient stance accepted", err)
	}
	unknown := stance
	unknown.MessageID = "unknown-message"
	unknown.Binding.IdempotencyKey = "stance-unknown"
	if _, err := s.RecordRPInformationStance(ctx, unknown); !core.HasCode(err, core.CodeNotFound) {
		t.Fatal("guessed message stance accepted", err)
	}
	first, err := s.RecordRPInformationStance(ctx, stance)
	if err != nil || first.Fact.Stance != "believe" || first.Fact.PreviousStanceEventID != "" ||
		first.Fact.SourceSendEventID != sent.EventID || first.Fact.ObserverID != M2RPNPCID {
		t.Fatal("recipient belief source", first, err)
	}
	if replay, err := s.RecordRPInformationStance(ctx, stance); err != nil || !replay.Replayed || replay.EventID != first.EventID {
		t.Fatal("belief exact replay", replay, err)
	}
	changed := stance
	changed.Stance = "reject"
	if _, err := s.RecordRPInformationStance(ctx, changed); !core.HasCode(err, core.CodeIdempotencyMismatch) {
		t.Fatal("stance key reused with different belief", err)
	}
	if got := readCareerTestContext(t, s, M2RPNPCID).Life.Information; len(got) != 1 || got[0].Stance != "believe" {
		t.Fatal("belief missing from recipient model input", got)
	}
	view, err = s.ObserveRPSession(ctx, cai)
	if err != nil {
		t.Fatal(err)
	}
	stance.Binding.ExpectedHead = view.ObservationCursor
	stance.Binding.IdempotencyKey = "stance-correct"
	stance.Stance = "doubt"
	stance.Reason = "后来发现没有可靠证据。"
	second, err := s.RecordRPInformationStance(ctx, stance)
	if err != nil || second.Fact.PreviousStanceEventID != first.EventID || second.Fact.Stance != "doubt" {
		t.Fatal("correction did not chain", second, err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='RPInformationStanceRecorded' AND instance_id=? AND branch_id=?`, []any{M2DemoInstanceID, M2DemoBranchID}, 2)
	if got := readCareerTestContext(t, s, M2RPNPCID).Life.Information; len(got) != 1 || got[0].Stance != "doubt" || got[0].ClaimedReliability != "unverified" {
		t.Fatal("correction became truth or lost stance", got)
	}
	if got := readCareerTestContext(t, s, M2AgentAdaID).Life.Information; len(got) != 0 {
		t.Fatal("unaddressed Agent learned stance or claim", got)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal("reopen stance world", err)
	}
	defer s.Close()
	if got := readCareerTestContext(t, s, M2RPNPCID).Life.Information; len(got) != 1 || got[0].Stance != "doubt" {
		t.Fatal("reopen lost corrected stance", got)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("stance history diverged after restart", diff, err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE events SET payload=json_set(payload,'$.previous_stance_event_id','forged') WHERE event_id=?`, second.EventID); err != nil {
		t.Fatal(err)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); !core.HasCode(err, core.CodeProjectionDiverged) {
		t.Fatal("broken immutable correction chain escaped audit", diff, err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE events SET payload=json_set(payload,'$.previous_stance_event_id',?) WHERE event_id=?`, first.EventID, second.EventID); err != nil {
		t.Fatal(err)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("restored correction chain still diverged", diff, err)
	}
}
