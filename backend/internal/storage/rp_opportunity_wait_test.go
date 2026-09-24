package storage

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPOpportunityColocationDoesNotInventFriendship(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "no-source.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, read, _ := newRPWaitTestSession(t, ctx, s)
	_, err = s.DefineRPOpportunityPolicy(ctx, OpportunityPolicyRequest{Binding: careerTestBinding(t, s, "principal_creator", "policy"), Policy: RPOpportunityPolicy{StreamSeed: "no-invented-friend", ContactBasisPoints: 5000, CooldownHours: 1, HistoryHours: 24}})
	if err != nil {
		t.Fatal(err)
	}
	view, err := s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	out, err := s.WaitRP(ctx, core.RPWaitRequest{OpportunityIntent: "social", PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: "2026-09-22T03:00:00Z", Budget: 100, IdempotencyKey: "strangers"})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.InitiativeNPCIDs) == 0 {
		t.Fatal("fixture needs a real nearby NPC")
	}
	var raw string
	if err := s.db.QueryRowContext(ctx, `SELECT payload FROM events WHERE event_id=?`, out.EventID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var event rpWaitEvent
	if err := json.Unmarshal([]byte(raw), &event); err != nil {
		t.Fatal(err)
	}
	if len(event.ContactOpportunities) != 0 {
		t.Fatalf("colocation invented friendly source: %+v", event.ContactOpportunities)
	}
}

func TestRPOpportunityWaitPrivateReceiptsRecoverWithoutReroll(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "contact.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Close() }()
	_, read, _ := newRPWaitTestSession(t, ctx, s)
	for _, key := range []string{"gift-one", "gift-two"} {
		gift := socialRequest(t, ctx, s, read, M2RPNPCID, "gift", key)
		gift.AmountMinor = 1
		if _, err := s.SocialRP(ctx, gift); err != nil {
			t.Fatal(err)
		}
	}
	_, err = s.DefineRPOpportunityPolicy(ctx, OpportunityPolicyRequest{Binding: careerTestBinding(t, s, "principal_creator", "policy"), Policy: RPOpportunityPolicy{StreamSeed: "contact-stream", ContactBasisPoints: 0, CooldownHours: 6, HistoryHours: 24}})
	if err != nil {
		t.Fatal(err)
	}
	view, err := s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	r := core.RPWaitRequest{OpportunityIntent: "social", PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: "2026-09-22T03:00:00Z", Budget: 100, IdempotencyKey: "contact-wait"}
	s.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "wait receipt rollback") }
	if _, err := s.WaitRP(ctx, r); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatalf("rollback: %v", err)
	}
	s.beforeCommit = nil
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='RPWaitCompleted'`, nil, 0)
	out, err := s.WaitRP(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	readReceipt := func(eventID string) rpContactOpportunity {
		t.Helper()
		var raw string
		if err := s.db.QueryRowContext(ctx, `SELECT payload FROM events WHERE event_id=?`, eventID).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		var event rpWaitEvent
		if err := json.Unmarshal([]byte(raw), &event); err != nil {
			t.Fatal(err)
		}
		for _, receipt := range event.ContactOpportunities {
			if receipt.ActorID == M2RPNPCID {
				return receipt
			}
		}
		t.Fatal("real friendly relationship did not supply receipt")
		return rpContactOpportunity{}
	}
	first := readReceipt(out.EventID)
	if !first.Seeking || first.Draw.ChanceBasisPoints != 0 {
		t.Fatalf("seeking overrode disabled contact: %+v", first)
	}
	if first.SourceEventID == "" || first.TargetID != M2RPPlayerID {
		t.Fatal("receipt lacks observed relationship source")
	}
	var broadcast string
	if err := s.db.QueryRowContext(ctx, `SELECT payload FROM outbox WHERE event_id=?`, out.EventID).Scan(&broadcast); err != nil {
		t.Fatal(err)
	}
	response, _ := json.Marshal(out)
	view, err = s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	observation, _ := json.Marshal(view)
	for _, raw := range []string{broadcast, string(response), string(observation)} {
		if strings.Contains(raw, "contact_opportunities") || strings.Contains(raw, first.Draw.IdentityHash) || strings.Contains(raw, "contact-stream") {
			t.Fatal("private draw leaked")
		}
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	retry, err := s.WaitRP(ctx, r)
	if err != nil || !retry.Replayed || !reflect.DeepEqual(readReceipt(retry.EventID), first) {
		t.Fatalf("retry changed receipt: %+v %v", retry, err)
	}
	view, err = s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	r.ExpectedCursor = view.ObservationCursor
	r.IdempotencyKey = "fresh-key-same-hour"
	r.TargetWorldTime = "2026-09-22T03:30:00Z"
	later, err := s.WaitRP(ctx, r)
	if err != nil || !reflect.DeepEqual(readReceipt(later.EventID), first) {
		t.Fatalf("fresh wait rerolled same source/window: %+v %v", later, err)
	}
	initiative := core.RPInitiativeRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, NPCEntityID: M2RPNPCID, TriggerEventID: later.EventID}
	input, err := s.BuildRPInitiativeInput(ctx, initiative)
	if err != nil || input.ContactOpportunity == nil || input.ContactOpportunity.Selected {
		t.Fatalf("missed opportunity context: %+v %v", input.ContactOpportunity, err)
	}
	providerInput, _ := json.Marshal(input)
	if strings.Contains(string(providerInput), "contact-stream") || strings.Contains(string(providerInput), first.Draw.IdentityHash) {
		t.Fatal("provider received internal random stream")
	}
	calls := 0
	provider := rpDecisionProviderFunc(func(context.Context, core.RPDecisionInput) (core.RPDecisionProposal, error) {
		calls++
		return core.RPDecisionProposal{Action: "respond", Text: "主动问候"}, nil
	})
	result, err := s.RunRPInitiative(ctx, initiative, provider)
	if err != nil || result.Action != "silence" || result.Status != "opportunity_quiet" || calls != 0 {
		t.Fatalf("miss forced contact/provider: %+v calls=%d err=%v goals=%+v", result, calls, err, input.Life.Goals)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.RunRPInitiative(ctx, initiative, provider)
	if err != nil || !again.Replayed || again.EventID != result.EventID || calls != 0 {
		t.Fatalf("quiet decision retry: %+v calls=%d %v", again, calls, err)
	}
}
