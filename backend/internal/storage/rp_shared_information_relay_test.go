package storage

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"corerp.local/backend/internal/core"
)

func TestRPSharedInformationRelayRequiresConsentHumanAndRecoversOneHop(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "shared-relay.db")
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
	send := func(key, message string, allow bool) RPInformationRecord {
		t.Helper()
		view, err := s.ObserveRPSession(ctx, human)
		if err != nil {
			t.Fatal(err)
		}
		record, err := s.SendRPInformation(ctx, RPInformationSendRequest{Binding: core.CareerBinding{
			PrincipalID: human.PrincipalID, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID,
			ExpectedHead: view.ObservationCursor, IdempotencyKey: key,
		}, SessionID: human.SessionID, MessageID: message, RecipientEntityID: M2RPNPCID,
			Text: "合作社下周可能停业。", AllowRelay: allow})
		if err != nil {
			t.Fatal(err)
		}
		view, err = s.ObserveRPSession(ctx, human)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: human.PrincipalID, SessionID: human.SessionID,
			TargetWorldTime: record.Fact.DeliverWorldTime, Budget: 1000,
			ExpectedCursor: view.ObservationCursor, IdempotencyKey: key + "-delivery"}); err != nil {
			t.Fatal(err)
		}
		return record
	}
	private := send("shared-relay-private", "shared-relay-private-message", false)
	consented := send("shared-relay-consented", "shared-relay-consented-message", true)
	for _, p := range []struct{ id, kind string }{{"principal_relay_operator", "operator"}, {"principal_relay_service", "service"}} {
		if _, err := s.db.ExecContext(ctx, `INSERT INTO principals(principal_id,principal_type,display_name,status) VALUES (?,?,?,'active')`, p.id, p.kind, p.id); err != nil {
			t.Fatal(err)
		}
	}
	binding := func(key string) core.CareerBinding {
		return core.CareerBinding{PrincipalID: "principal_relay_operator", InstanceID: M2DemoInstanceID,
			BranchID: M2DemoBranchID, ExpectedHead: head(), IdempotencyKey: key}
	}
	if _, err := s.EnrollRPExternalControllerLocal(ctx, RPExternalControllerEnrollmentRequest{Binding: binding("relay-enroll"),
		EntityID: M2RPNPCID, ControllerPrincipalID: "principal_relay_service", ControllerInstanceID: "relay-controller"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AssignRPExternalControllerLocal(ctx, RPExternalControllerAssignmentRequest{Binding: binding("relay-assign"),
		EntityID: M2RPNPCID, ExpectedGeneration: 0}); err != nil {
		t.Fatal(err)
	}
	externalSession, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: "principal_relay_service",
		InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2RPNPCID,
		POV: "second_person", IdempotencyKey: "relay-external-session"})
	if err != nil {
		t.Fatal(err)
	}
	external := core.RPSessionReadRequest{PrincipalID: "principal_relay_service", SessionID: externalSession.SessionID}
	for _, read := range []core.RPSessionReadRequest{human, external} {
		if _, err := s.ObserveRPSession(ctx, read); err != nil {
			t.Fatal(err)
		}
	}
	round, err := s.OpenRPSharedRoundLocal(ctx, RPSharedRoundOpenRequest{Binding: binding("relay-round"),
		HumanSessionID: humanSession.SessionID, ExternalSessionIDs: []string{externalSession.SessionID}})
	if err != nil {
		t.Fatal(err)
	}
	baseline := head()
	proposal := RPSharedInformationRelayRequest{PrincipalID: external.PrincipalID, SessionID: external.SessionID,
		RoundID: round.RoundID, MessageID: "rumor-shared-to-lin", ForwardedMessageID: consented.Fact.MessageID,
		RecipientEntityID: M2RPPlayerID, IdempotencyKey: "propose-shared-relay"}
	withoutConsent := proposal
	withoutConsent.ForwardedMessageID = private.Fact.MessageID
	withoutConsent.IdempotencyKey = "propose-private-relay"
	if _, err := s.SubmitRPSharedInformationRelay(ctx, withoutConsent); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatal("private message without consent entered shared proposal", err)
	}
	unknown := proposal
	unknown.ForwardedMessageID = "not-received"
	unknown.IdempotencyKey = "propose-unread-relay"
	if _, err := s.SubmitRPSharedInformationRelay(ctx, unknown); !core.HasCode(err, core.CodeNotFound) {
		t.Fatal("unread claim entered shared proposal", err)
	}
	foreign := proposal
	foreign.PrincipalID = human.PrincipalID
	if _, err := s.SubmitRPSharedInformationRelay(ctx, foreign); err == nil {
		t.Fatal("foreign principal proposed another participant's rumor")
	}
	if submitted, err := s.SubmitRPSharedInformationRelay(ctx, proposal); err != nil || submitted.Submitted != 1 {
		t.Fatal("actual recipient relay proposal", submitted, err)
	}
	if head() != baseline {
		t.Fatal("rumor proposal wrote Event before Human submission")
	}
	if replay, err := s.SubmitRPSharedInformationRelay(ctx, proposal); err != nil || !replay.Replayed {
		t.Fatal("relay proposal replay", replay, err)
	}
	changed := proposal
	changed.RecipientEntityID = M2RPNPCID
	if _, err := s.SubmitRPSharedInformationRelay(ctx, changed); !core.HasCode(err, core.CodeIdempotencyMismatch) {
		t.Fatal("changed relay proposal reused same key", err)
	}
	advance := RPSharedRoundAdvanceRequest{RPSharedRoundReadRequest: RPSharedRoundReadRequest{
		PrincipalID: external.PrincipalID, SessionID: external.SessionID, RoundID: round.RoundID}, Budget: 100}
	if _, err := s.AdvanceRPSharedRound(ctx, advance); !core.HasCode(err, core.CodeCommandInProgress) {
		t.Fatal("external relay bypassed Human", err)
	}
	direct := RPInformationRelayRequest{Binding: core.CareerBinding{PrincipalID: external.PrincipalID,
		InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: baseline,
		IdempotencyKey: "relay-direct-bypass"}, SessionID: external.SessionID,
		MessageID: proposal.MessageID, ForwardedMessageID: proposal.ForwardedMessageID,
		RecipientEntityID: proposal.RecipientEntityID}
	if _, err := s.RelayRPInformation(ctx, direct); !core.HasCode(err, core.CodeCommandInProgress) {
		t.Fatal("direct relay bypassed open round", err)
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
		IdempotencyKey: "relay-human-wait"}); err != nil {
		t.Fatal(err)
	}
	selectedFailure := errors.New("interrupt after relay selection")
	s.afterRPSharedActionStage = func(stage string) error {
		if stage == "selection_committed" {
			return selectedFailure
		}
		return nil
	}
	if _, err := s.AdvanceRPSharedRound(ctx, advance); !errors.Is(err, selectedFailure) || head() != baseline {
		t.Fatal("relay selection alone wrote Event", err)
	}
	direct.Binding.IdempotencyKey = "shared_action_" + round.RoundID
	direct.ForwardedMessageID = private.Fact.MessageID
	if _, err := s.RelayRPInformation(ctx, direct); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatal("selected child accepted changed forwarded source", err)
	}
	direct.ForwardedMessageID = proposal.ForwardedMessageID
	direct.RecipientEntityID = M2RPNPCID
	if _, err := s.RelayRPInformation(ctx, direct); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatal("selected child accepted changed recipient", err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='RPInformationSent' AND json_extract(payload,'$.channel')='rumor'`, nil, 0)
	eventFailure := errors.New("interrupt after durable rumor Event")
	s.afterRPSharedActionStage = func(stage string) error {
		if stage == "information_relay_event_committed" {
			return eventFailure
		}
		return nil
	}
	if _, err := s.AdvanceRPSharedRound(ctx, advance); !errors.Is(err, eventFailure) || head() != baseline+1 {
		t.Fatal("rumor Event-before-receipt stage", err, head())
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='RPInformationSent' AND json_extract(payload,'$.channel')='rumor'`, nil, 1)
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
		t.Fatal("recover selected rumor and settle", settled, err)
	}
	var rumorID, due, parentSend, parentDelivery string
	var relayable int
	if err := s.db.QueryRowContext(ctx, `SELECT event_id,json_extract(payload,'$.deliver_world_time'),
	 json_extract(payload,'$.forwarded_from_send_event_id'),json_extract(payload,'$.forwarded_from_delivery_event_id'),
	 COALESCE(json_extract(payload,'$.allow_relay'),0) FROM events WHERE event_type='RPInformationSent' AND json_extract(payload,'$.message_id')=?`,
		proposal.MessageID).Scan(&rumorID, &due, &parentSend, &parentDelivery, &relayable); err != nil {
		t.Fatal(err)
	}
	if parentSend != consented.EventID || parentDelivery == "" || relayable != 0 {
		t.Fatal("shared rumor lost parent evidence or became relayable", parentSend, parentDelivery, relayable)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_knowledge WHERE claim_key=?`, []any{"information:" + rumorID}, 0)
	for _, read := range []core.RPSessionReadRequest{human, external} {
		if _, err := s.ObserveRPSession(ctx, read); err != nil {
			t.Fatal(err)
		}
	}
	deliveryRound, err := s.OpenRPSharedRoundLocal(ctx, RPSharedRoundOpenRequest{Binding: binding("relay-delivery-round"),
		HumanSessionID: humanSession.SessionID, ExternalSessionIDs: []string{externalSession.SessionID}})
	if err != nil {
		t.Fatal(err)
	}
	for _, read := range []core.RPSessionReadRequest{human, external} {
		if _, err := s.SubmitRPSharedWait(ctx, RPSharedWaitRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID,
			RoundID: deliveryRound.RoundID, HorizonWorldTime: due, IdempotencyKey: "relay-delivery-wait-" + read.PrincipalID}); err != nil {
			t.Fatal(err)
		}
	}
	if got, err := s.AdvanceRPSharedRound(ctx, RPSharedRoundAdvanceRequest{RPSharedRoundReadRequest: RPSharedRoundReadRequest{
		PrincipalID: human.PrincipalID, SessionID: human.SessionID, RoundID: deliveryRound.RoundID}, Budget: 1000}); err != nil || got.Status != "settled" {
		t.Fatal("scheduled rumor delivery under shared wait", got, err)
	}
	view, err := s.ReadRPContext(ctx, RPContextReadRequest{PrincipalID: human.PrincipalID, SessionID: human.SessionID})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, fact := range view.Facts {
		found = found || fact.MessageID == proposal.MessageID && fact.Channel == "rumor" && fact.Forwarded &&
			!fact.MayRelay && fact.Text == consented.Fact.Text
	}
	if !found {
		t.Fatal("shared rumor missing from actual recipient context", view.Facts)
	}
	if got := readCareerTestContext(t, s, M2AgentAdaID).Life.Information; len(got) != 0 {
		t.Fatal("unaddressed third party learned rumor", got)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("shared rumor source replay diverged", diff, err)
	}
}
