package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"corerp.local/backend/internal/core"
)

// Long-run mechanics of the actual same-world story. Store sessions are not
// external clients: Play/MCP/SillyTavern transport E2E remains a separate gate.
func TestFinalWorldMonthLongRunMechanics(t *testing.T) { runFinalWorldStory(t, true) }

func runFinalWorldMonth(t *testing.T, current **Store, path string, read core.RPSessionReadRequest, nora string) {
	t.Helper()
	ctx := context.Background()
	s := *current
	defer func() { *current = s }()
	if _, err := s.DefineRPOpportunityPolicy(ctx, OpportunityPolicyRequest{Binding: careerTestBinding(t, s, "principal_creator", "final-month-policy"), Policy: RPOpportunityPolicy{StreamSeed: "final-world-month-v1", VisitBasisPoints: 300, RareVisitBasisPoints: 100, ContactBasisPoints: 200, WarmEnabled: true, CooldownHours: 6, HistoryHours: 240}}); err != nil {
		t.Fatal(err)
	}
	// Fix the stream before the first draw; never search seeds or retry to force a hit.
	other, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: read.PrincipalID, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2RPPlayerID, POV: "second_person", IdempotencyKey: "final-month-second-session"})
	if err != nil {
		t.Fatal(err)
	}
	reads := []core.RPSessionReadRequest{read, {PrincipalID: read.PrincipalID, SessionID: other.SessionID}}
	turns, quiet, waits, rareDraws, rareHits, rareEffects, gifts, restarts := 0, 0, 0, 0, 0, 0, 0, 0
	move := func(target, key string) {
		t.Helper()
		for step := 0; step < 2; step++ {
			view, err := s.ObserveRPSession(ctx, read)
			if err != nil {
				t.Fatal(err)
			}
			if view.PlaceID == target {
				return
			}
			to := target
			if view.PlaceID != M2AgentCafeID && target != M2AgentCafeID {
				to = M2AgentCafeID
			}
			if _, err := s.MoveRP(ctx, core.RPMoveRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, FromPlaceID: view.PlaceID, ToPlaceID: to, IdempotencyKey: fmt.Sprintf("%s-%d", key, step)}); err != nil {
				t.Fatal(err)
			}
		}
	}
	topics := []string{"今天工作结束了吗？", "最近的生活开销怎么样？", "你现在方便交谈吗？", "你怎么看街区的赠礼习惯？", "有空再见面吧。", "今天我只想安静待一会儿。", "你愿意说说最近的日常吗？", "不用急着答应我的请求。", "我会记住你之前的意见。", "明天有机会再聊。"}
	for day := 3; day <= 32; day++ {
		for _, hour := range []int{6, 7, 9, 10, 12, 18, 19, 20} {
			target := "place_m2_home_bo"
			if hour == 12 {
				target = M2AgentCafeID
			}
			at := careerTime(day, hour, 15)
			move(target, "month-move-"+at)
			view, err := s.ObserveRPSession(ctx, read)
			if err != nil {
				t.Fatal(err)
			}
			service, err := NewRPService(s, core.DeterministicRPDecisionProvider{}, "deterministic")
			if err != nil {
				t.Fatal(err)
			}
			r, err := service.WaitRP(ctx, core.RPWaitRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: at, Budget: 1000, IdempotencyKey: "month-wait-" + at})
			if err != nil || r.Status != "completed" || r.CurrentWorldTime != at {
				t.Fatalf("month wait %s: %+v %v", at, r, err)
			}
			waits++
			isQuiet := true
			for _, initiative := range r.Initiatives {
				if initiative.Action != "silence" {
					isQuiet = false
				}
			}
			if isQuiet {
				quiet++
			}
			var raw string
			if err := s.db.QueryRowContext(ctx, `SELECT payload FROM events WHERE event_id=?`, r.EventID).Scan(&raw); err != nil {
				t.Fatal(err)
			}
			var fact rpWaitEvent
			if err := json.Unmarshal([]byte(raw), &fact); err != nil {
				t.Fatal(err)
			}
			for _, visit := range fact.VisitOpportunities {
				if !visit.Rare {
					continue
				}
				rareDraws++
				if visit.Draw.ChanceBasisPoints > 100 || visit.Quiet {
					t.Fatal("rare probability inflated")
				}
				if visit.Draw.Selected {
					rareHits++
					for _, effect := range r.Initiatives {
						if effect.NPCEntityID == visit.ActorID && effect.Action == "leave" {
							rareEffects++
						}
					}
				}
			}
			if hour == 12 {
				view, err := s.ObserveRPSession(ctx, read)
				if err != nil {
					t.Fatal(err)
				}
				present := false
				for _, person := range view.PresentEntities {
					present = present || person.EntityID == nora
				}
				if present {
					if day == 3 {
						for i := 0; i < 2; i++ {
							if _, err := s.SocialRP(ctx, socialRequest(t, ctx, s, read, nora, "apologize", fmt.Sprintf("repair-tension-%d", i))); err != nil {
								t.Fatal(err)
							}
						}
					}
					gift := socialRequest(t, ctx, s, read, nora, "gift", fmt.Sprintf("month-lunch-%d", day))
					gift.AmountMinor = 1
					if _, err := s.SocialRP(ctx, gift); err != nil {
						t.Fatal(err)
					}
					gifts++
				}
			}
			if hour == 12 || hour == 20 {
				for i := 0; i < 5; i++ {
					reader := reads[turns%len(reads)]
					view, err := s.ObserveRPSession(ctx, reader)
					if err != nil {
						t.Fatal(err)
					}
					request := core.RPSpeechRequest{PrincipalID: reader.PrincipalID, SessionID: reader.SessionID, ExpectedCursor: view.ObservationCursor, Text: topics[turns%len(topics)], IdempotencyKey: fmt.Sprintf("month-turn-%03d", turns)}
					result, err := s.PlayRPTurn(ctx, request)
					if err != nil || result.Status != "settled" {
						t.Fatalf("month turn %d: %+v %v", turns, result, err)
					}
					turns++
				}
			}
		}
		if day == 10 || day == 20 || day == 30 {
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
			restarts++
		}
		if day%5 == 0 {
			t.Logf("day%d: turns=%d waits=%d quiet=%d gifts=%d rare=%d/%d effects=%d", day, turns, waits, quiet, gifts, rareHits, rareDraws, rareEffects)
		}
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_turn_runs WHERE status='settled'`, nil, 304)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_profiles WHERE status='active'`, nil, 6)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM cohorts WHERE instance_id=? AND branch_id=?`, []any{M2DemoInstanceID, M2DemoBranchID}, 3)
	var clock string
	if err := s.db.QueryRowContext(ctx, `SELECT current_world_time FROM world_clocks WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&clock); err != nil {
		t.Fatal(err)
	}
	start, _ := time.Parse(time.RFC3339, rpLifeSetupTime)
	end, err := time.Parse(time.RFC3339, clock)
	if err != nil || end.Sub(start) < 30*24*time.Hour || turns != 300 || restarts != 3 || quiet < 20 || gifts < 10 {
		t.Fatalf("incomplete month: clock=%s turns=%d restarts=%d quiet=%d gifts=%d", clock, turns, restarts, quiet, gifts)
	}
	trust := 0
	for _, relation := range readCareerTestContext(t, s, nora).Life.Relationships {
		if relation.SubjectEntityID == M2RPPlayerID {
			trust = relation.Trust
		}
	}
	if trust < 10 {
		t.Fatalf("no sourced long-term relationship: trust%d", trust)
	}
	if diffs, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diffs) != 0 {
		t.Fatalf("month projections: %+v %v", diffs, err)
	}
	t.Logf("304 settled turns, %.2f elapsed days, two Store sessions/five total reopens; %d/%d quiet waits, trust%d, rare selected%d/draws%d/committed leaves%d. Actual-client E2E and rare-event gate require their own evidence.", end.Sub(start).Hours()/24, quiet, waits, trust, rareHits, rareDraws, rareEffects)
}
