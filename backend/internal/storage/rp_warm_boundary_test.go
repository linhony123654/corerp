package storage

import (
	"context"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPWarmRejectsInterveningActionAndAdvancedClock(t *testing.T) {
	for _, mode := range []string{"same-time-action", "advanced-clock"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			s := openCareerTestWorld(t, filepath.Join(t.TempDir(), "warm-stale.db"))
			defer s.Close()
			if _, err := s.DefineRPOpportunityPolicy(ctx, OpportunityPolicyRequest{Binding: careerTestBinding(t, s, "principal_creator", "policy"), Policy: RPOpportunityPolicy{StreamSeed: "stale", WarmEnabled: true, CooldownHours: 1, HistoryHours: 24}}); err != nil {
				t.Fatal(err)
			}
			session, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: M2RPPlayerPrincipal, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2RPPlayerID, POV: "second_person", IdempotencyKey: "session"})
			if err != nil {
				t.Fatal(err)
			}
			read := core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID}
			move := func(to, key string) {
				t.Helper()
				view, err := s.ObserveRPSession(ctx, read)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := s.MoveRP(ctx, core.RPMoveRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, FromPlaceID: view.PlaceID, ToPlaceID: to, ExpectedCursor: view.ObservationCursor, IdempotencyKey: key}); err != nil {
					t.Fatal(err)
				}
			}
			move("place_m2_work_ada", "visit")
			view, err := s.ObserveRPSession(ctx, read)
			if err != nil {
				t.Fatal(err)
			}
			wait, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: careerTime(1, 7, 15), Budget: 1000, IdempotencyKey: "pending-warm"})
			if err != nil {
				t.Fatal(err)
			}
			if len(wait.WarmNPCIDs) != 1 || wait.WarmNPCIDs[0] != M2AgentAdaID {
				t.Fatalf("missing real WARM intent: %+v", wait)
			}
			request := core.RPInitiativeRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, NPCEntityID: M2AgentAdaID, TriggerEventID: wait.EventID}
			if mode == "same-time-action" {
				move(M2AgentCafeID, "intervening")
			} else {
				if _, err := s.RunAgentLife(ctx, careerTime(1, 8, 0), 1000); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := s.runRPWarmDecision(ctx, request); !core.HasCode(err, core.CodeBranchConflict) {
				t.Fatalf("stale WARM executed: %v", err)
			}
			assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='RPWarmDecisionRecorded'`, nil, 0)
			if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
				t.Fatal(err)
			}
			if _, err := s.runRPWarmDecision(ctx, request); !core.HasCode(err, core.CodeBranchConflict) {
				t.Fatalf("rebuild revived stale trigger: %v", err)
			}
			if differences, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
				t.Fatalf("stale rejection damaged projection: %+v %v", differences, err)
			}
		})
	}
}
