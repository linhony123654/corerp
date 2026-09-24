package storage

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPOpportunityMajorPressureUsesOwnJobLossNotice(t *testing.T) {
	ctx := context.Background()
	s := openCareerTestWorld(t, filepath.Join(t.TempDir(), "pressure.db"))
	defer s.Close()
	offer := prepareCareerEmploymentOfferAtWage(t, s, 12)
	accepted, err := s.AcceptCareerOffer(ctx, core.CareerOfferAcceptRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "accept"), OfferID: offer.Fact.RecordID, AfterWorkPlaceID: M2AgentCafeID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.RunAgentLife(ctx, careerTime(1, 8, 0), 100); err != nil {
		t.Fatal(err)
	}
	notice, err := s.EndCareerEmployment(ctx, core.CareerExitRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "layoff"), ContractID: accepted.Fact.Employment.ContractID, Kind: "layoff", EffectiveFromDay: 2, Notice: "Position ends tomorrow; earned wages remain due"})
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		actor, from, through string
		count                int
	}{
		{M2AgentAdaID, careerTime(1, 0, 0), careerTime(1, 9, 0), 1},
		{M2AgentBoID, careerTime(1, 0, 0), careerTime(1, 9, 0), 0},
		{M2AgentAdaID, careerTime(0, 0, 0), careerTime(1, 7, 0), 0},
		{M2AgentAdaID, careerTime(2, 0, 0), careerTime(2, 9, 0), 0},
	} {
		tx, err := beginImmediate(ctx, s.db)
		if err != nil {
			t.Fatal(err)
		}
		sources, err := readRPOpportunityMajorChanges(ctx, tx.conn, M2DemoInstanceID, M2DemoBranchID, tt.actor, tt.from, tt.through)
		tx.Rollback(ctx)
		if err != nil || len(sources) != tt.count {
			t.Fatalf("major pressure %+v: %v %v", tt, sources, err)
		}
		if len(sources) == 1 && sources[0] != notice.EventID {
			t.Fatal("pressure lost actual notice source")
		}
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM employment_contracts WHERE contract_id=? AND status='active'`, []any{accepted.Fact.Employment.ContractID}, 1)
	if _, err := s.RunAgentLife(ctx, careerTime(1, 18, 0), 1000); err != nil {
		t.Fatal(err)
	}
	read := allowFixtureControl(t, ctx, s, M2AgentBoID)
	view, err := s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	if view.PlaceID != M2AgentCafeID {
		if _, err := s.MoveRP(ctx, core.RPMoveRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, FromPlaceID: view.PlaceID, ToPlaceID: M2AgentCafeID, ExpectedCursor: view.ObservationCursor, IdempotencyKey: "meet-after-work"}); err != nil {
			t.Fatal(err)
		}
	}
	for _, key := range []string{"support-one", "support-two"} {
		gift := socialRequest(t, ctx, s, read, M2AgentAdaID, "gift", key)
		gift.AmountMinor = 1
		if _, err := s.SocialRP(ctx, gift); err != nil {
			t.Fatal(err)
		}
	}
	_, err = s.DefineRPOpportunityPolicy(ctx, OpportunityPolicyRequest{Binding: careerTestBinding(t, s, "principal_creator", "pressure-policy"), Policy: RPOpportunityPolicy{StreamSeed: "job-loss-pressure", ContactBasisPoints: 5000, CooldownHours: 1, HistoryHours: 24}})
	if err != nil {
		t.Fatal(err)
	}
	view, err = s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	wait, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: careerTime(1, 18, 30), Budget: 1000, IdempotencyKey: "after-notice"})
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
	found := false
	for _, receipt := range event.ContactOpportunities {
		if receipt.ActorID != M2AgentAdaID {
			continue
		}
		found = true
		if len(receipt.MajorChangeSourceEventIDs) != 1 || receipt.MajorChangeSourceEventIDs[0] != notice.EventID || receipt.Draw.ChanceBasisPoints != 1250 || receipt.Busy || receipt.CoolingDown {
			t.Fatalf("notice did not reduce real wait contact chance: %+v", receipt)
		}
	}
	if !found {
		t.Fatal("no actual own-notice contact evaluation")
	}
	if err := s.db.QueryRowContext(ctx, `SELECT payload FROM outbox WHERE event_id=?`, wait.EventID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(raw, notice.EventID) || strings.Contains(raw, "major_change_source_event_ids") {
		t.Fatal("private job-loss pressure leaked to broadcast")
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM employment_contracts WHERE contract_id=? AND status='active'`, []any{accepted.Fact.Employment.ContractID}, 1)
}
