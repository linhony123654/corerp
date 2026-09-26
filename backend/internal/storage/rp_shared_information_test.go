package storage

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"corerp.local/backend/internal/core"
)

func TestRPSharedInformationSendRequiresHumanAndRecoversEventBeforeReceipt(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "shared-information.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	humanSession, human, _ := newRPWaitTestSession(t, ctx, s)
	for _, p := range []struct{ id, kind string }{{"principal_info_operator", "operator"}, {"principal_info_service", "service"}} {
		if _, err := s.db.ExecContext(ctx, `INSERT INTO principals(principal_id,principal_type,display_name,status) VALUES (?,?,?,'active')`, p.id, p.kind, p.id); err != nil {
			t.Fatal(err)
		}
	}
	head := func() int64 {
		var n int64
		if err := s.db.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	binding := func(key string) core.CareerBinding {
		return core.CareerBinding{PrincipalID: "principal_info_operator", InstanceID: M2DemoInstanceID,
			BranchID: M2DemoBranchID, ExpectedHead: head(), IdempotencyKey: key}
	}
	if _, err := s.EnrollRPExternalControllerLocal(ctx, RPExternalControllerEnrollmentRequest{Binding: binding("info-enroll"),
		EntityID: M2RPNPCID, ControllerPrincipalID: "principal_info_service", ControllerInstanceID: "info-controller"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AssignRPExternalControllerLocal(ctx, RPExternalControllerAssignmentRequest{Binding: binding("info-assign"),
		EntityID: M2RPNPCID, ExpectedGeneration: 0}); err != nil {
		t.Fatal(err)
	}
	externalSession, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: "principal_info_service",
		InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2RPNPCID,
		POV: "second_person", IdempotencyKey: "info-external-session"})
	if err != nil {
		t.Fatal(err)
	}
	external := core.RPSessionReadRequest{PrincipalID: "principal_info_service", SessionID: externalSession.SessionID}
	for _, read := range []core.RPSessionReadRequest{human, external} {
		if _, err := s.ObserveRPSession(ctx, read); err != nil {
			t.Fatal(err)
		}
	}
	open := func(key string) RPSharedRound {
		round, err := s.OpenRPSharedRoundLocal(ctx, RPSharedRoundOpenRequest{Binding: binding(key),
			HumanSessionID: humanSession.SessionID, ExternalSessionIDs: []string{externalSession.SessionID}})
		if err != nil {
			t.Fatal(err)
		}
		return round
	}
	round := open("info-round-send")
	baseline := head()
	proposal := RPSharedInformationSendRequest{PrincipalID: external.PrincipalID, SessionID: external.SessionID,
		RoundID: round.RoundID, MessageID: "info-shared-to-lin", RecipientEntityID: M2RPPlayerID,
		Text: "来自 Cai 的远程消息，不是当面听到的。", AllowRelay: false, IdempotencyKey: "propose-info-send"}
	if submitted, err := s.SubmitRPSharedInformationSend(ctx, proposal); err != nil || submitted.Submitted != 1 {
		t.Fatal("external information proposal", submitted, err)
	}
	if head() != baseline {
		t.Fatal("proposal wrote an information Event before Human submission")
	}
	if replay, err := s.SubmitRPSharedInformationSend(ctx, proposal); err != nil || !replay.Replayed {
		t.Fatal("information proposal retry", replay, err)
	}
	changed := proposal
	changed.Text = "偷换成其他消息"
	if _, err := s.SubmitRPSharedInformationSend(ctx, changed); !core.HasCode(err, core.CodeIdempotencyMismatch) {
		t.Fatal("changed information reused proposal key", err)
	}
	advance := RPSharedRoundAdvanceRequest{RPSharedRoundReadRequest: RPSharedRoundReadRequest{
		PrincipalID: external.PrincipalID, SessionID: external.SessionID, RoundID: round.RoundID}, Budget: 100}
	if _, err := s.AdvanceRPSharedRound(ctx, advance); !core.HasCode(err, core.CodeCommandInProgress) {
		t.Fatal("external proposal bypassed Human", err)
	}
	bypass := RPInformationSendRequest{Binding: core.CareerBinding{PrincipalID: external.PrincipalID,
		InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: baseline,
		IdempotencyKey: "direct-bypass"}, SessionID: external.SessionID, MessageID: proposal.MessageID,
		RecipientEntityID: proposal.RecipientEntityID, Text: proposal.Text}
	if _, err := s.SendRPInformation(ctx, bypass); !core.HasCode(err, core.CodeCommandInProgress) {
		t.Fatal("direct external send bypassed open round", err)
	}
	var at string
	if err := s.db.QueryRowContext(ctx, `SELECT current_world_time FROM world_clocks WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&at); err != nil {
		t.Fatal(err)
	}
	start, err := time.Parse(time.RFC3339Nano, at)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SubmitRPSharedWait(ctx, RPSharedWaitRequest{PrincipalID: human.PrincipalID, SessionID: human.SessionID,
		RoundID: round.RoundID, HorizonWorldTime: start.Add(10 * time.Minute).Format(time.RFC3339Nano),
		IdempotencyKey: "info-human-waits"}); err != nil {
		t.Fatal(err)
	}
	selectedFailure := errors.New("lose response after selected information action")
	s.afterRPSharedActionStage = func(stage string) error {
		if stage == "selection_committed" {
			return selectedFailure
		}
		return nil
	}
	if _, err := s.AdvanceRPSharedRound(ctx, advance); !errors.Is(err, selectedFailure) {
		t.Fatal("selected information stage not interrupted", err)
	}
	if head() != baseline {
		t.Fatal("selection alone wrote an information Event")
	}
	bypass.Binding.IdempotencyKey = "shared_action_" + round.RoundID
	bypass.Text = "换掉已经选中的私信正文"
	if _, err := s.SendRPInformation(ctx, bypass); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatal("selected child accepted changed text", err)
	}
	bypass.Text = proposal.Text
	bypass.RecipientEntityID = M2RPNPCID
	if _, err := s.SendRPInformation(ctx, bypass); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatal("selected child accepted changed recipient", err)
	}
	bypass.RecipientEntityID = proposal.RecipientEntityID
	bypass.AllowRelay = true
	if _, err := s.SendRPInformation(ctx, bypass); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatal("selected child accepted changed relay consent", err)
	}
	eventFailure := errors.New("lose response after durable information Event")
	s.afterRPSharedActionStage = func(stage string) error {
		if stage == "information_event_committed" {
			return eventFailure
		}
		return nil
	}
	if _, err := s.AdvanceRPSharedRound(ctx, advance); !errors.Is(err, eventFailure) || head() != baseline+1 {
		t.Fatal("information Event-before-receipt stage", err, head())
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='RPInformationSent' AND json_extract(payload,'$.message_id')=?`, []any{proposal.MessageID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_knowledge WHERE claim_key LIKE 'information:%' AND observer_agent_id=?`, []any{M2RPPlayerID}, 0)
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
		t.Fatal("recover selected information Event and settle", settled, err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='RPInformationSent' AND json_extract(payload,'$.message_id')=?`, []any{proposal.MessageID}, 1)
	var due string
	if err := s.db.QueryRowContext(ctx, `SELECT json_extract(payload,'$.deliver_world_time') FROM events WHERE event_type='RPInformationSent' AND json_extract(payload,'$.message_id')=?`, proposal.MessageID).Scan(&due); err != nil {
		t.Fatal(err)
	}
	for _, read := range []core.RPSessionReadRequest{human, external} {
		if _, err := s.ObserveRPSession(ctx, read); err != nil {
			t.Fatal(err)
		}
	}
	deliveryRound := open("info-round-delivery")
	for _, read := range []core.RPSessionReadRequest{human, external} {
		if _, err := s.SubmitRPSharedWait(ctx, RPSharedWaitRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID,
			RoundID: deliveryRound.RoundID, HorizonWorldTime: due, IdempotencyKey: "info-delivery-wait-" + read.PrincipalID}); err != nil {
			t.Fatal(err)
		}
	}
	deliveryAdvance := RPSharedRoundAdvanceRequest{RPSharedRoundReadRequest: RPSharedRoundReadRequest{
		PrincipalID: human.PrincipalID, SessionID: human.SessionID, RoundID: deliveryRound.RoundID}, Budget: 1000}
	if result, err := s.AdvanceRPSharedRound(ctx, deliveryAdvance); err != nil || result.Status != "settled" {
		t.Fatal("shared wait delivered information later", result, err)
	}
	view, err := s.ReadRPContext(ctx, RPContextReadRequest{PrincipalID: human.PrincipalID, SessionID: human.SessionID})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, fact := range view.Facts {
		found = found || fact.MessageID == proposal.MessageID && fact.Kind == "message_received" &&
			fact.Channel == "direct_message" && fact.Text == proposal.Text
	}
	if !found {
		t.Fatal("shared information delivery absent from actual recipient context", view.Facts)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("shared information source replay", diff, err)
	}
}
