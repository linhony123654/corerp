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

func prepareFinalFriends(t *testing.T, s *Store) []string {
	t.Helper()
	ctx := context.Background()
	var friends []string
	people := []struct {
		name string
		id   string
	}{{"Faye", "entity_final_faye"}, {"Gita", "entity_final_gita"}, {"Hana", "entity_final_hana_1"}, {"Ivan", "entity_final_ivan_2"}}
	for i, person := range people {
		name, id := person.name, person.id
		friends = append(friends, id)
		b := careerTestBinding(t, s, "principal_creator", "final-friend-materialize-"+name)
		command := m2AgentMaterialization(b.IdempotencyKey, id, name, 1, 180, 1, 0, 0, b.ExpectedHead, rpLifeSetupTime)
		command.SourceCohortID = RPFinalBlockB
		if i >= 2 {
			command.SourceCohortID = RPFinalBlockC
		}
		m, err := s.MaterializeCohort(ctx, command)
		if err != nil {
			t.Fatal(err)
		}
		if disposition := core.DeriveRPDisposition(id, m.EventID); disposition.Sociability == 0 {
			t.Fatalf("rare-visit fixture %s has zero sociability", id)
		}
		if _, err := s.MaterializeRPBackground(ctx, core.RPBackgroundRequest{PrincipalID: b.PrincipalID, InstanceID: b.InstanceID, BranchID: b.BranchID, EntityID: id, ExpectedHead: m.LastSequence, IdempotencyKey: "final-friend-background-" + name, AgeMin: 25, AgeMax: 34, ResidencePlaceID: "place_m2_home_ada", InitialPlaceID: M2AgentCafeID, Schedule: []core.RPBackgroundSchedule{{WorldTime: "2026-09-22T18:00:00Z", PlaceID: "place_m2_home_ada", ActivityCode: "home"}}}); err != nil {
			t.Fatal(err)
		}
	}
	return friends
}

// Use a declared stream/window without selecting a seed for a hit. Event IDs
// contribute to the draw, so a 1% opportunity need not occur in every run.
// The deterministic hit fixture separately requires real movement.
func TestFinalWorldSeparatedFriendsRareVisit(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "final-separated.db")
	s := openCareerTestWorld(t, path)
	defer func() { s.Close() }()
	prepareFinalWorld(t, s, M2AgentNoonTime)
	friends := prepareFinalFriends(t, s)
	if _, err := s.DefineRPOpportunityPolicy(ctx, OpportunityPolicyRequest{Binding: careerTestBinding(t, s, "principal_creator", "final-separated-policy"), Policy: RPOpportunityPolicy{StreamSeed: "final-separated-friends-v1", WarmEnabled: true, WarmVisitsEnabled: true, RareVisitBasisPoints: 100, CooldownHours: 6, HistoryHours: 720}}); err != nil {
		t.Fatal(err)
	}
	session, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: M2RPPlayerPrincipal, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2RPPlayerID, POV: "second_person", IdempotencyKey: "final-separated-session"})
	if err != nil {
		t.Fatal(err)
	}
	read := core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID}
	waitAt := func(at string) (RPWaitResult, core.RPWaitRequest) {
		t.Helper()
		service, err := NewRPService(s, core.DeterministicRPDecisionProvider{}, "deterministic")
		if err != nil {
			t.Fatal(err)
		}
		view, err := s.ObserveRPSession(ctx, read)
		if err != nil {
			t.Fatal(err)
		}
		r := core.RPWaitRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: at, Budget: 1000, IdempotencyKey: at}
		out, err := service.WaitRP(ctx, r)
		if err != nil || out.CurrentWorldTime != at {
			t.Fatalf("wait: %+v %v", out, err)
		}
		return out, r
	}
	for round, at := range []string{"2026-09-22T07:10:00Z", "2026-09-22T07:20:00Z"} {
		promises := map[string]string{}
		for _, id := range friends {
			promise := socialRequest(t, ctx, s, read, id, "promise_meeting", fmt.Sprintf("promise-%d-%s", round, id))
			promise.MeetingPlaceID, promise.MeetingWorldTime = M2AgentCafeID, at
			out, err := s.SocialRP(ctx, promise)
			if err != nil {
				t.Fatal(err)
			}
			promises[id] = out.EventID
		}
		waitAt(at)
		for _, id := range friends {
			keep := socialRequest(t, ctx, s, read, id, "keep_meeting", fmt.Sprintf("keep-%d-%s", round, id))
			keep.PromiseEventID = promises[id]
			if _, err := s.SocialRP(ctx, keep); err != nil {
				t.Fatal(err)
			}
		}
	}
	view, err := s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.MoveRP(ctx, core.RPMoveRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, FromPlaceID: view.PlaceID, ToPlaceID: "place_m2_work_bo", ExpectedCursor: view.ObservationCursor, IdempotencyKey: "lin-away"}); err != nil {
		t.Fatal(err)
	}
	draws, hits, effects := 0, 0, 0
	for day := 1; day <= 30; day++ {
		for _, hour := range []int{0, 6, 12, 18} {
			out, request := waitAt(careerTime(day, hour, 30))
			var raw string
			if err := s.db.QueryRowContext(ctx, `SELECT payload FROM events WHERE event_id=?`, out.EventID).Scan(&raw); err != nil {
				t.Fatal(err)
			}
			var trigger rpWaitEvent
			if err := json.Unmarshal([]byte(raw), &trigger); err != nil {
				t.Fatal(err)
			}
			for _, receipt := range trigger.VisitOpportunities {
				if !receipt.Rare {
					t.Fatal("rare-only policy produced ordinary visit")
				}
				draws++
				at, _ := time.Parse(time.RFC3339, out.CurrentWorldTime)
				remembered, _ := time.Parse(time.RFC3339, receipt.Source.RememberedWorldTime)
				if receipt.Quiet || receipt.Draw.ChanceBasisPoints > 100 || receipt.Source.FriendID != M2RPPlayerID || at.Sub(remembered) < core.RPOldFriendVisitMinimumGap {
					t.Fatalf("unsourced old friend: %+v", receipt)
				}
				if receipt.Draw.Selected != (receipt.Draw.RollBasisPoints < receipt.Draw.ChanceBasisPoints) {
					t.Fatalf("rare draw selection does not match committed roll: %+v", receipt)
				}
				if !receipt.Draw.Selected {
					continue
				}
				hits++
				var rawFact, eventID string
				if err := s.db.QueryRowContext(ctx, `SELECT event_id,payload FROM events WHERE event_type='RPWarmDecisionRecorded' AND json_extract(payload,'$.trigger_event_id')=? AND json_extract(payload,'$.decision.actor_id')=?`, out.EventID, receipt.ActorID).Scan(&eventID, &rawFact); err != nil {
					t.Fatal(err)
				}
				var fact rpWarmFact
				if err := json.Unmarshal([]byte(rawFact), &fact); err != nil {
					t.Fatal(err)
				}
				if fact.Decision.Action != "leave" {
					t.Logf("selected visit deferred: actor=%s time=%s action=%s reason=%s", receipt.ActorID, out.CurrentWorldTime, fact.Decision.Action, fact.Decision.Reason)
					continue
				}
				if fact.Decision.Reason != "sourced_visit" || fact.Decision.ToPlaceID != M2AgentCafeID {
					t.Fatalf("rare destination not remembered cafe: %+v", fact)
				}
				assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_movements WHERE event_id=? AND agent_id=?`, []any{eventID, receipt.ActorID}, 1)
				assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_positions WHERE agent_id=? AND place_id='place_m2_work_bo'`, []any{M2RPPlayerID}, 1)
				assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_knowledge WHERE observer_agent_id=? AND source_event_id=?`, []any{M2RPPlayerID, eventID}, 0)
				effects++
			}
			if day == 10 && hour == 18 || day == 20 && hour == 18 || day == 30 && hour == 18 {
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
				service, err := NewRPService(s, core.DeterministicRPDecisionProvider{}, "deterministic")
				if err != nil {
					t.Fatal(err)
				}
				if retry, err := service.WaitRP(ctx, request); err != nil || !retry.Replayed || retry.EventID != out.EventID {
					t.Fatalf("reopen retry: %+v %v", retry, err)
				}
			}
		}
	}
	t.Logf("fixed separated-friend stream: rare receipts=%d selected=%d movements=%d", draws, hits, effects)
	if draws == 0 {
		t.Fatalf("declared window did not evaluate a rare visit: draws=%d hits=%d effects=%d", draws, hits, effects)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_profiles WHERE status='active'`, nil, 10)
	if differences, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
		t.Fatalf("rare recovery: %+v %v", differences, err)
	}
}
