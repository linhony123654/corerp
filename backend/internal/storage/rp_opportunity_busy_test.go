package storage

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPOpportunityBusyUsesActualEmploymentSchedule(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "busy.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	prepareRPLifeLongWorld(t, ctx, s)
	session, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: M2RPPlayerPrincipal, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2RPPlayerID, POV: "second_person", IdempotencyKey: "busy-session"})
	if err != nil {
		t.Fatal(err)
	}
	read := core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID}
	for _, key := range []string{"gift-one", "gift-two"} {
		gift := socialRequest(t, ctx, s, read, rpLifeNoraID, "gift", key)
		gift.AmountMinor = 1
		if _, err := s.SocialRP(ctx, gift); err != nil {
			t.Fatal(err)
		}
	}
	_, err = s.DefineRPOpportunityPolicy(ctx, OpportunityPolicyRequest{Binding: careerTestBinding(t, s, "principal_creator", "policy"), Policy: RPOpportunityPolicy{StreamSeed: "busy-world", ContactBasisPoints: 5000, CooldownHours: 1, HistoryHours: 24}})
	if err != nil {
		t.Fatal(err)
	}
	for _, at := range []string{"2026-09-23T07:30:00Z", "2026-09-23T08:30:00Z"} {
		view, err := s.ObserveRPSession(ctx, read)
		if err != nil {
			t.Fatal(err)
		}
		if at == "2026-09-23T08:30:00Z" {
			if _, err := s.MoveRP(ctx, core.RPMoveRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, FromPlaceID: view.PlaceID, ToPlaceID: "place_m2_work_ada", ExpectedCursor: view.ObservationCursor, IdempotencyKey: "visit-work"}); err != nil {
				t.Fatal(err)
			}
			view, err = s.ObserveRPSession(ctx, read)
			if err != nil {
				t.Fatal(err)
			}
		}
		out, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: at, Budget: 1000, IdempotencyKey: at})
		if err != nil || out.Status != "completed" {
			t.Fatalf("wait %+v %v", out, err)
		}
		var raw string
		if err := s.db.QueryRowContext(ctx, `SELECT payload FROM events WHERE event_id=?`, out.EventID).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		var event rpWaitEvent
		if err := json.Unmarshal([]byte(raw), &event); err != nil {
			t.Fatal(err)
		}
		found := false
		for _, receipt := range event.ContactOpportunities {
			if receipt.ActorID != rpLifeNoraID {
				continue
			}
			found = true
			if !receipt.Busy || receipt.Draw.ChanceBasisPoints != 0 || receipt.Draw.Selected || receipt.SourceEventID == "" {
				t.Fatalf("busy contact offered: %+v", receipt)
			}
		}
		if !found {
			t.Fatal("busy fixture lacked real friendly contact source")
		}
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_movements WHERE agent_id=? AND movement_kind='scheduled' AND activity_code='work'`, []any{rpLifeNoraID}, 1)
	if differences, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatalf("busy schedule projection: %+v %v", differences, err)
	}
}
