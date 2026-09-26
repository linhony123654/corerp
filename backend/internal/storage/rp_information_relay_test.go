package storage

import (
	"context"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPInformationExplicitOneHopRumorPreservesSourceAndPrivateBoundary(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "relay.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, lin, view := newRPWaitTestSession(t, ctx, s)
	send := func(key, message string, allow bool) RPInformationRecord {
		t.Helper()
		latest, err := s.ObserveRPSession(ctx, lin)
		if err != nil {
			t.Fatal(err)
		}
		out, err := s.SendRPInformation(ctx, RPInformationSendRequest{Binding: core.CareerBinding{
			PrincipalID: lin.PrincipalID, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID,
			ExpectedHead: latest.ObservationCursor, IdempotencyKey: key,
		}, SessionID: lin.SessionID, MessageID: message, RecipientEntityID: M2RPNPCID,
			Text: "合作社下周可能停业。", AllowRelay: allow})
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	wait := func(key, due string) {
		t.Helper()
		latest, err := s.ObserveRPSession(ctx, lin)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: lin.PrincipalID, SessionID: lin.SessionID,
			TargetWorldTime: due, Budget: 1000, ExpectedCursor: latest.ObservationCursor,
			IdempotencyKey: key}); err != nil {
			t.Fatal(err)
		}
	}
	private := send("relay-private-send", "relay-private", false)
	wait("relay-private-deliver", private.Fact.DeliverWorldTime)
	cai := allowFixtureControl(t, ctx, s, M2RPNPCID)
	view, err = s.ObserveRPSession(ctx, cai)
	if err != nil {
		t.Fatal(err)
	}
	relay := RPInformationRelayRequest{Binding: core.CareerBinding{
		PrincipalID: cai.PrincipalID, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID,
		ExpectedHead: view.ObservationCursor, IdempotencyKey: "relay-without-consent",
	}, SessionID: cai.SessionID, MessageID: "rumor-denied", ForwardedMessageID: private.Fact.MessageID,
		RecipientEntityID: M2RPPlayerID}
	if _, err := s.RelayRPInformation(ctx, relay); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatal("private message leaked without sender consent", err)
	}
	consented := send("relay-consented-send", "relay-consented", true)
	view, err = s.ObserveRPSession(ctx, cai)
	if err != nil {
		t.Fatal(err)
	}
	relay.Binding.ExpectedHead = view.ObservationCursor
	relay.Binding.IdempotencyKey = "relay-before-delivery"
	relay.ForwardedMessageID = consented.Fact.MessageID
	if _, err := s.RelayRPInformation(ctx, relay); !core.HasCode(err, core.CodeNotFound) {
		t.Fatal("undelivered source relayed", err)
	}
	wait("relay-consented-deliver", consented.Fact.DeliverWorldTime)
	view, err = s.ObserveRPSession(ctx, cai)
	if err != nil {
		t.Fatal(err)
	}
	relay.Binding.ExpectedHead = view.ObservationCursor
	relay.Binding.IdempotencyKey = "relay-now"
	relay.MessageID = "rumor-to-lin"
	wrong := relay
	wrong.ForwardedMessageID = "guessed-private-message"
	wrong.Binding.IdempotencyKey = "relay-guessed"
	if _, err := s.RelayRPInformation(ctx, wrong); !core.HasCode(err, core.CodeNotFound) {
		t.Fatal("guessed source revealed", err)
	}
	forwarded, err := s.RelayRPInformation(ctx, relay)
	if err != nil || forwarded.Fact.Channel != "rumor" || forwarded.Fact.AllowRelay ||
		forwarded.Fact.ForwardedFromSendEventID != consented.EventID ||
		forwarded.Fact.ForwardedFromDeliveryEventID == "" || forwarded.Fact.Text != consented.Fact.Text {
		t.Fatal("rumor source chain", forwarded, err)
	}
	if replay, err := s.RelayRPInformation(ctx, relay); err != nil || !replay.Replayed || replay.EventID != forwarded.EventID {
		t.Fatal("relay exact replay", replay, err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_knowledge WHERE claim_key=?`, []any{"information:" + forwarded.EventID}, 0)
	wait("rumor-deliver", forwarded.Fact.DeliverWorldTime)
	linContext, err := s.ReadRPContext(ctx, RPContextReadRequest{PrincipalID: lin.PrincipalID, SessionID: lin.SessionID})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, fact := range linContext.Facts {
		if fact.MessageID == forwarded.Fact.MessageID && fact.Channel == "rumor" && fact.Forwarded &&
			!fact.MayRelay && fact.Reliability == "unverified" && fact.Text == consented.Fact.Text {
			found = true
		}
	}
	if !found {
		t.Fatal("rumor absent from recipient context", linContext.Facts)
	}
	if got := readCareerTestContext(t, s, M2RPPlayerID).Life.Information; len(got) != 1 || !got[0].Forwarded || got[0].MayRelay {
		t.Fatal("rumor absent or over-permissive in model input", got)
	}
	if got := readCareerTestContext(t, s, M2AgentAdaID).Life.Information; len(got) != 0 {
		t.Fatal("unaddressed third party learned rumor", got)
	}
	caiView, err := s.ObserveRPSession(ctx, cai)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.RecordRPInformationStance(ctx, RPInformationStanceRequest{Binding: core.CareerBinding{
		PrincipalID: cai.PrincipalID, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID,
		ExpectedHead: caiView.ObservationCursor, IdempotencyKey: "rumor-cai-believes",
	}, SessionID: cai.SessionID, MessageID: consented.Fact.MessageID, Stance: "believe"}); err != nil {
		t.Fatal("recipient A belief", err)
	}
	view, err = s.ObserveRPSession(ctx, lin)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.RecordRPInformationStance(ctx, RPInformationStanceRequest{Binding: core.CareerBinding{
		PrincipalID: lin.PrincipalID, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID,
		ExpectedHead: view.ObservationCursor, IdempotencyKey: "rumor-lin-rejects",
	}, SessionID: lin.SessionID, MessageID: forwarded.Fact.MessageID, Stance: "reject"}); err != nil {
		t.Fatal("recipient B disbelief", err)
	}
	if got := readCareerTestContext(t, s, M2RPNPCID).Life.Information; len(got) != 2 ||
		got[0].Stance != "believe" || got[1].Stance != "" {
		t.Fatal("recipient A stance changed authoritative claim", got)
	}
	if got := readCareerTestContext(t, s, M2RPPlayerID).Life.Information; len(got) != 1 || got[0].Stance != "reject" || got[0].ClaimedReliability != "unverified" {
		t.Fatal("recipient B did not independently reject rumor", got)
	}
	view, err = s.ObserveRPSession(ctx, lin)
	if err != nil {
		t.Fatal(err)
	}
	secondHop := RPInformationRelayRequest{Binding: core.CareerBinding{PrincipalID: lin.PrincipalID,
		InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: view.ObservationCursor,
		IdempotencyKey: "relay-second-hop"}, SessionID: lin.SessionID, MessageID: "rumor-second-hop",
		ForwardedMessageID: forwarded.Fact.MessageID, RecipientEntityID: M2RPNPCID}
	if _, err := s.RelayRPInformation(ctx, secondHop); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatal("one-hop policy widened through rumor", err)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("rumor projection diverged", diff, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("rumor failed restart/replay", diff, err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE events SET payload=json_set(payload,'$.forwarded_from_delivery_event_id','forged') WHERE event_id=?`, forwarded.EventID); err != nil {
		t.Fatal(err)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); !core.HasCode(err, core.CodeProjectionDiverged) {
		t.Fatal("forged rumor lineage escaped source audit", diff, err)
	}
	if _, err := s.ReadRPEvents(ctx, RPEventsReadRequest{PrincipalID: lin.PrincipalID, SessionID: lin.SessionID,
		After: forwarded.EventSequence, Limit: 50}); !core.HasCode(err, core.CodeProjectionDiverged) {
		t.Fatal("forged rumor reached recipient event stream", err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE events SET payload=json_set(payload,'$.forwarded_from_delivery_event_id',?) WHERE event_id=?`, forwarded.Fact.ForwardedFromDeliveryEventID, forwarded.EventID); err != nil {
		t.Fatal(err)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("restored rumor lineage still diverged", diff, err)
	}
}
