package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"corerp.local/backend/internal/core"
)

func TestRPWarmVisitPinnedMovementPrivacyAndRecovery(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(fmt.Sprint(enabled), func(t *testing.T) {
			ctx := context.Background()
			path := filepath.Join(t.TempDir(), "warm-visit.db")
			s, err := Open(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { s.Close() }()
			background := newBackgroundCandidate(t, ctx, s)
			background.Schedule = background.Schedule[:1]
			if _, err := s.MaterializeRPBackground(ctx, background); err != nil {
				t.Fatal(err)
			}
			if _, err := s.DefineRPOpportunityPolicy(ctx, OpportunityPolicyRequest{Binding: careerTestBinding(t, s, "principal_creator", "warm-visit-policy"), Policy: RPOpportunityPolicy{StreamSeed: "warm-visit-fixed-stream", WarmEnabled: true, WarmVisitsEnabled: enabled, VisitBasisPoints: 5000, CooldownHours: 6, HistoryHours: 240}}); err != nil {
				t.Fatal(err)
			}
			session, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: M2RPPlayerPrincipal, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2RPPlayerID, POV: "second_person", IdempotencyKey: "warm-visit-session"})
			if err != nil {
				t.Fatal(err)
			}
			read := core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID}
			waitAt := func(at string) (RPWaitResult, core.RPWaitRequest) {
				t.Helper()
				view, err := s.ObserveRPSession(ctx, read)
				if err != nil {
					t.Fatal(err)
				}
				r := core.RPWaitRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: at, Budget: 1000, IdempotencyKey: at}
				out, err := s.WaitRP(ctx, r)
				if err != nil || out.CurrentWorldTime != at {
					t.Fatalf("wait: %+v %v", out, err)
				}
				return out, r
			}
			// Mutual importance comes from actual kept promises, not NPC control
			// or direct relationship writes. Lin is the only player throughout.
			for i := 0; i < 2; i++ {
				view, err := s.ObserveRPSession(ctx, read)
				if err != nil {
					t.Fatal(err)
				}
				at, err := time.Parse(time.RFC3339, view.WorldTime)
				if err != nil {
					t.Fatal(err)
				}
				promise := socialRequest(t, ctx, s, read, background.EntityID, "promise_meeting", fmt.Sprintf("promise-%d", i))
				promise.MeetingPlaceID = M2AgentCafeID
				promise.MeetingWorldTime = at.Add(5 * time.Minute).Format(time.RFC3339)
				promised, err := s.SocialRP(ctx, promise)
				if err != nil {
					t.Fatal(err)
				}
				waitAt(promise.MeetingWorldTime)
				keep := socialRequest(t, ctx, s, read, background.EntityID, "keep_meeting", fmt.Sprintf("keep-%d", i))
				keep.PromiseEventID = promised.EventID
				if _, err := s.SocialRP(ctx, keep); err != nil {
					t.Fatal(err)
				}
			}
			view, err := s.ObserveRPSession(ctx, read)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.MoveRP(ctx, core.RPMoveRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, FromPlaceID: view.PlaceID, ToPlaceID: "place_m2_home_ada", ExpectedCursor: view.ObservationCursor, IdempotencyKey: "friend-away"}); err != nil {
				t.Fatal(err)
			}
			moved := false
			start, _ := time.Parse(time.RFC3339, careerTime(0, 3, 0))
			for i := 0; i < 12; i++ {
				out, waitRequest := waitAt(start.Add(time.Duration(i) * 6 * time.Hour).Format(time.RFC3339))
				if len(out.WarmNPCIDs) != 1 || out.WarmNPCIDs[0] != background.EntityID {
					t.Fatalf("missing sourced WARM roster: %+v", out)
				}
				for _, npc := range out.InitiativeNPCIDs {
					if npc == background.EntityID {
						t.Fatal("WARM actor promoted to HOT within wait")
					}
				}
				var raw, public string
				if err := s.db.QueryRowContext(ctx, `SELECT payload FROM events WHERE event_id=?`, out.EventID).Scan(&raw); err != nil {
					t.Fatal(err)
				}
				if err := s.db.QueryRowContext(ctx, `SELECT payload FROM outbox WHERE event_id=?`, out.EventID).Scan(&public); err != nil {
					t.Fatal(err)
				}
				if strings.Contains(public, "visit_opportunities") || strings.Contains(public, "warm_candidates") {
					t.Fatal("private WARM evidence broadcast")
				}
				var trigger rpWaitEvent
				if err := json.Unmarshal([]byte(raw), &trigger); err != nil {
					t.Fatal(err)
				}
				var receipt *rpVisitOpportunity
				for _, candidate := range trigger.VisitOpportunities {
					if candidate.ActorID == background.EntityID {
						copy := candidate
						receipt = &copy
					}
				}
				if enabled != (receipt != nil) {
					t.Fatalf("visit flag not respected: enabled=%t receipt=%+v", enabled, receipt)
				}
				request := core.RPInitiativeRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, NPCEntityID: background.EntityID, TriggerEventID: out.EventID}
				if receipt != nil && receipt.Draw.Selected {
					s.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "warm visit rollback") }
					if _, err := s.runRPWarmDecision(ctx, request); !core.HasCode(err, core.CodeInjectedFailure) {
						t.Fatalf("rollback: %v", err)
					}
					s.beforeCommit = nil
					assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_positions WHERE agent_id=? AND place_id='place_m2_home_bo'`, []any{background.EntityID}, 1)
				}
				fact, err := s.runRPWarmDecision(ctx, request)
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(fact.Fact.VisitOpportunity, receipt) {
					t.Fatal("WARM replaced pinned receipt")
				}
				if receipt == nil || !receipt.Draw.Selected {
					if fact.Fact.Decision.Action != "wait" {
						t.Fatalf("unselected visit executed: %+v", fact)
					}
					continue
				}
				if fact.Fact.Decision.Action != "leave" || fact.Fact.Decision.ToPlaceID != M2AgentCafeID || fact.Fact.Decision.Reason != "sourced_visit" {
					t.Fatalf("selected visit not committed: %+v", fact)
				}
				assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_positions WHERE agent_id=? AND place_id=?`, []any{background.EntityID, M2AgentCafeID}, 1)
				assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_knowledge WHERE observer_agent_id=? AND source_event_id=?`, []any{M2RPPlayerID, fact.EventID}, 0)
				assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM outbox WHERE event_id=?`, []any{fact.EventID}, 0)
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
				retry, err := s.runRPWarmDecision(ctx, request)
				if err != nil || !retry.Replayed || retry.EventID != fact.EventID || !reflect.DeepEqual(retry.Fact, fact.Fact) {
					t.Fatalf("recovery replaced visit: %+v %v", retry, err)
				}
				if retryWait, err := s.WaitRP(ctx, waitRequest); err != nil || !retryWait.Replayed || retryWait.EventID != out.EventID {
					t.Fatalf("wait redrawn: %+v %v", retryWait, err)
				}
				assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_movements WHERE event_id=?`, []any{fact.EventID}, 1)
				if differences, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
					t.Fatalf("recovery: %+v %v", differences, err)
				}
				moved = true
				break
			}
			if enabled && !moved {
				t.Fatal("fixed ordinary stream produced no actual visit in declared window")
			}
			if !enabled {
				assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_positions WHERE agent_id=? AND place_id='place_m2_home_bo'`, []any{background.EntityID}, 1)
			}
		})
	}
}
