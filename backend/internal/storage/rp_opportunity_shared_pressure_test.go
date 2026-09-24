package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"corerp.local/backend/internal/core"
)

func TestRPOpportunitySameWaitContactSuppressesWork(t *testing.T) {
	ctx := context.Background()
	s := openCareerTestWorld(t, filepath.Join(t.TempDir(), "same-wait-pressure.db"))
	defer s.Close()
	offer := prepareCareerEmploymentOffer(t, s)
	accepted, err := s.AcceptCareerOffer(ctx, core.CareerOfferAcceptRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "accept"), OfferID: offer.Fact.RecordID, AfterWorkPlaceID: M2AgentCafeID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.RunAgentLife(ctx, careerTime(1, 12, 0), 1000); err != nil {
		t.Fatal(err)
	}
	session, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: M2RPPlayerPrincipal, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2RPPlayerID, POV: "second_person", IdempotencyKey: "pressure-session"})
	if err != nil {
		t.Fatal(err)
	}
	read := core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID}
	for _, key := range []string{"gift-one", "gift-two"} {
		gift := socialRequest(t, ctx, s, read, M2AgentAdaID, "gift", key)
		gift.AmountMinor = 1
		if _, err := s.SocialRP(ctx, gift); err != nil {
			t.Fatal(err)
		}
	}
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		t.Fatal(err)
	}
	input, err := readRPOwnDecisionContext(ctx, tx.conn, core.RPDecisionInput{InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, NPCEntityID: M2AgentAdaID, InterlocutorEntityID: M2RPPlayerID})
	tx.Rollback(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var source string
	for _, relation := range input.Life.Relationships {
		if relation.SubjectEntityID == M2RPPlayerID && relation.Trust >= 2 && len(relation.SourceEventIDs) > 0 {
			source = relation.SourceEventIDs[0]
			break
		}
	}
	if source == "" {
		t.Fatal("missing actual friendship")
	}
	r := OpportunityPolicyRequest{Binding: careerTestBinding(t, s, "principal_creator", "same-wait-policy"), Policy: RPOpportunityPolicy{ContactBasisPoints: 5000, WorkBasisPoints: 5000, CooldownHours: 1, HistoryHours: 48}}
	keyHash, err := core.HashJSON([]string{r.Binding.PrincipalID, r.Binding.IdempotencyKey})
	if err != nil {
		t.Fatal(err)
	}
	idHash, err := core.HashJSON([]string{r.Binding.InstanceID, r.Binding.BranchID, "DefineRPOpportunityPolicy", keyHash})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 1000; i++ {
		seed := fmt.Sprintf("same-wait-%d", i)
		draw, err := core.DrawRPOpportunity(core.RPOpportunityDrawKey{PolicyEventID: "event_opportunity_" + idHash[7:], StreamSeed: seed, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ActorID: M2AgentAdaID, Kind: "friend_contact", SourceEventID: source, WorldTime: careerTime(1, 12, 15)}, 5000)
		if err != nil {
			t.Fatal(err)
		}
		if draw.Selected {
			r.Policy.StreamSeed = seed
			break
		}
	}
	if _, err := s.DefineRPOpportunityPolicy(ctx, r); err != nil {
		t.Fatal(err)
	}
	view, err := s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	wait, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: careerTime(1, 12, 15), Budget: 1000, IdempotencyKey: "same-wait"})
	if err != nil {
		t.Fatal(err)
	}
	var raw string
	if err := s.db.QueryRowContext(ctx, `SELECT payload FROM events WHERE event_id=?`, wait.EventID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var event rpWaitEvent
	if err := json.Unmarshal([]byte(raw), &event); err != nil {
		t.Fatal(err)
	}
	contact, work := false, false
	for _, receipt := range event.ContactOpportunities {
		if receipt.ActorID == M2AgentAdaID {
			contact = receipt.Draw.Selected
		}
	}
	for _, receipt := range event.WorkOpportunities {
		if receipt.ActorID == M2AgentAdaID {
			work = true
			if receipt.Memory.SourceEventID != accepted.EventID || receipt.Draw.Selected || !receipt.CoolingDown || receipt.RecentChanges != 1 || receipt.Draw.ChanceBasisPoints != 0 {
				t.Fatalf("same wait stacked opportunities: %+v", receipt)
			}
		}
	}
	if !contact || !work {
		t.Fatalf("missing sourced families contact=%t work=%t", contact, work)
	}
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatalf("shared pressure recovery: %+v %v", diff, err)
	}
}

func TestRPOpportunitySharedPressureDeduplicatesAndExpiresOriginalReceipts(t *testing.T) {
	policy := RPOpportunityPolicy{CooldownHours: 1, HistoryHours: 24}
	draw := func(id string) core.RPOpportunityDraw {
		return core.RPOpportunityDraw{IdentityHash: id, Selected: true}
	}
	wait := rpWaitEvent{
		VisitOpportunities:     []rpVisitOpportunity{{ActorID: "ada", WorldTime: "2026-09-23T12:45:00Z", Draw: draw("visit")}},
		CommunityOpportunities: []rpCommunityOpportunity{{ActorID: "ada", WorldTime: "2026-09-23T12:45:00Z", Draw: draw("community")}},
		ContactOpportunities:   []rpContactOpportunity{{ActorID: "ada", WorldTime: "2026-09-23T12:15:00Z", Draw: draw("contact")}},
		StoreOpportunities:     []rpStoreOpportunity{{ActorID: "ada", WorldTime: "2026-09-23T12:30:00Z", Draw: draw("store")}},
		WorkOpportunities:      []rpWorkOpportunity{{ActorID: "ada", WorldTime: "2026-09-23T12:45:00Z", Draw: draw("work")}, {ActorID: "bo", WorldTime: "2026-09-23T12:45:00Z", Draw: draw("other")}},
	}
	for _, tt := range []struct {
		at      string
		count   int
		cooling bool
	}{
		{"2026-09-23T12:40:00Z", 2, true},
		{"2026-09-23T13:44:59Z", 5, true},
		{"2026-09-23T13:45:00Z", 5, false},
		{"2026-09-24T12:30:00Z", 4, false},
		{"2026-09-24T12:45:01Z", 0, false},
	} {
		at, _ := time.Parse(time.RFC3339, tt.at)
		n, cooling, err := rpOpportunityHistoryPressure([]rpWaitEvent{wait, wait}, "ada", at, policy)
		if err != nil || n != tt.count || cooling != tt.cooling {
			t.Fatalf("at %s: %d %t %v", tt.at, n, cooling, err)
		}
	}
	wait.WorkOpportunities[0].Draw.Selected = false
	at, _ := time.Parse(time.RFC3339, "2026-09-23T13:30:00Z")
	n, cooling, err := rpOpportunityHistoryPressure([]rpWaitEvent{wait}, "ada", at, policy)
	if err != nil || n != 4 || !cooling {
		t.Fatalf("miss added pressure: %d %t %v", n, cooling, err)
	}
}
