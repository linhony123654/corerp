package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPWorkOpportunityOwnNoticeHistoryPrivacyAndRecovery(t *testing.T) {
	for _, hit := range []bool{false, true} {
		t.Run(fmt.Sprint(hit), func(t *testing.T) {
			ctx := context.Background()
			path := filepath.Join(t.TempDir(), "work-opportunity.db")
			s := openCareerTestWorld(t, path)
			defer func() { s.Close() }()
			offer := prepareCareerEmploymentOffer(t, s)
			accepted, err := s.AcceptCareerOffer(ctx, core.CareerOfferAcceptRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "accept"), OfferID: offer.Fact.RecordID, AfterWorkPlaceID: M2AgentCafeID})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.RunAgentLife(ctx, careerTime(1, 8, 0), 100); err != nil {
				t.Fatal(err)
			}
			notice, err := s.EndCareerEmployment(ctx, core.CareerExitRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "layoff"), ContractID: accepted.Fact.Employment.ContractID, Kind: "layoff", EffectiveFromDay: 2, Notice: "Ends tomorrow; earned wages remain owed"})
			if err != nil {
				t.Fatal(err)
			}
			r := OpportunityPolicyRequest{Binding: careerTestBinding(t, s, "principal_creator", "work-policy"), Policy: RPOpportunityPolicy{WorkBasisPoints: 5000, CooldownHours: 1, HistoryHours: 24}}
			keyHash, err := core.HashJSON([]string{r.Binding.PrincipalID, r.Binding.IdempotencyKey})
			if err != nil {
				t.Fatal(err)
			}
			idHash, err := core.HashJSON([]string{r.Binding.InstanceID, r.Binding.BranchID, "DefineRPOpportunityPolicy", keyHash})
			if err != nil {
				t.Fatal(err)
			}
			policyID := "event_opportunity_" + idHash[7:]
			for i := 0; i < 1000; i++ {
				seed := fmt.Sprintf("work-fixture-%d", i)
				draw, err := core.DrawRPOpportunity(core.RPOpportunityDrawKey{PolicyEventID: policyID, StreamSeed: seed, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ActorID: M2AgentAdaID, Kind: "work_change", SourceEventID: notice.EventID, WorldTime: careerTime(1, 12, 15)}, 1250)
				if err != nil {
					t.Fatal(err)
				}
				if draw.Selected == hit {
					r.Policy.StreamSeed = seed
					break
				}
			}
			bad := r
			bad.Policy.WorkBasisPoints = 5001
			if _, err := s.DefineRPOpportunityPolicy(ctx, bad); !core.HasCode(err, core.CodeInvalidArgument) {
				t.Fatalf("unbounded work chance: %v", err)
			}
			policy, err := s.DefineRPOpportunityPolicy(ctx, r)
			if err != nil || policy.EventID != policyID {
				t.Fatalf("policy: %+v %v", policy, err)
			}
			// Read-only evaluation while on shift proves no forced work interruption.
			tx, err := beginImmediate(ctx, s.db)
			if err != nil {
				t.Fatal(err)
			}
			busy, err := evaluateRPWorkOpportunities(ctx, tx.conn, RPSession{InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ControlledEntityID: M2RPPlayerID}, []string{M2AgentAdaID, M2AgentBoID}, careerTime(1, 8, 15))
			tx.Rollback(ctx)
			if err != nil || len(busy) != 1 || !busy[0].Busy || busy[0].Draw.Selected || busy[0].Draw.ChanceBasisPoints != 0 || busy[0].ActorID != M2AgentAdaID {
				t.Fatalf("busy/private work: %+v %v", busy, err)
			}
			session, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: M2RPPlayerPrincipal, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2RPPlayerID, POV: "second_person", IdempotencyKey: "work-session"})
			if err != nil {
				t.Fatal(err)
			}
			read := core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID}
			waitAt := func(at, key string) (RPWaitResult, rpWorkOpportunity) {
				t.Helper()
				view, err := s.ObserveRPSession(ctx, read)
				if err != nil {
					t.Fatal(err)
				}
				wait, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: at, Budget: 1000, IdempotencyKey: key})
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
				var found *rpWorkOpportunity
				for i := range event.WorkOpportunities {
					if event.WorkOpportunities[i].ActorID == M2AgentAdaID {
						found = &event.WorkOpportunities[i]
					}
				}
				if found == nil {
					t.Fatal("missing own work notice opportunity")
				}
				if err := s.db.QueryRowContext(ctx, `SELECT payload FROM outbox WHERE event_id=?`, wait.EventID).Scan(&raw); err != nil {
					t.Fatal(err)
				}
				if strings.Contains(raw, "work_opportunities") || strings.Contains(raw, notice.EventID) {
					t.Fatal("private work receipt broadcast")
				}
				return wait, *found
			}
			wait, first := waitAt(careerTime(1, 12, 15), "work-wait")
			if first.Memory.SourceEventID != notice.EventID || first.Memory.Kind != "own_employment_exit_notice" || first.Draw.Selected != hit || first.Draw.ChanceBasisPoints != 1250 || len(first.MajorChangeSourceEventIDs) != 1 {
				t.Fatalf("own sourced draw: %+v", first)
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
			request := core.RPInitiativeRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, NPCEntityID: M2AgentAdaID, TriggerEventID: wait.EventID}
			provider := rpDecisionProviderFunc(func(_ context.Context, in core.RPDecisionInput) (core.RPDecisionProposal, error) {
				if in.WorkOpportunity == nil || in.WorkOpportunity.Memory.SourceEventID != notice.EventID || in.WorkOpportunity.Selected != hit {
					t.Fatalf("lost own work context: %+v", in.WorkOpportunity)
				}
				encoded, _ := json.Marshal(in)
				if strings.Contains(string(encoded), "roll_basis_points") || strings.Contains(string(encoded), r.Policy.StreamSeed) {
					t.Fatal("private draw in provider")
				}
				if core.RPSelectedWorkChange(in) {
					return core.RPDecisionProposal{Action: "respond", Text: "我收到了工作变动通知。"}, nil
				}
				return core.RPDecisionProposal{Action: "silence"}, nil
			})
			out, err := s.RunRPInitiative(ctx, request, provider)
			if err != nil {
				t.Fatal(err)
			}
			want := "silence"
			if hit {
				want = "respond"
			}
			if out.Action != want {
				t.Fatalf("work consequence: %+v", out)
			}
			wantHeard := int64(0)
			if hit {
				wantHeard = 1
			}
			assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_knowledge WHERE observer_agent_id=? AND source_event_id=?`, []any{M2RPPlayerID, out.EventID}, wantHeard)
			retry, err := s.RunRPInitiative(ctx, request, provider)
			if err != nil || !retry.Replayed || retry.EventID != out.EventID {
				t.Fatalf("work initiative retry: %+v %v", retry, err)
			}
			assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM employment_contracts WHERE contract_id=? AND status='active'`, []any{accepted.Fact.Employment.ContractID}, 1)
			// Establish contact eligibility through real gifts after the work
			// draw: its new receipt must include pressure from that other family.
			for _, key := range []string{"cross-family-one", "cross-family-two"} {
				gift := socialRequest(t, ctx, s, read, M2AgentAdaID, "gift", key)
				gift.AmountMinor = 1
				if _, err := s.SocialRP(ctx, gift); err != nil {
					t.Fatal(err)
				}
			}
			crossWait, same := waitAt(careerTime(1, 12, 45), "same-hour")
			var crossRaw string
			if err := s.db.QueryRowContext(ctx, `SELECT payload FROM events WHERE event_id=?`, crossWait.EventID).Scan(&crossRaw); err != nil {
				t.Fatal(err)
			}
			var cross rpWaitEvent
			if err := json.Unmarshal([]byte(crossRaw), &cross); err != nil {
				t.Fatal(err)
			}
			foundContact := false
			for _, receipt := range cross.ContactOpportunities {
				if receipt.ActorID != M2AgentAdaID {
					continue
				}
				foundContact = true
				wantCount := 0
				if hit {
					wantCount = 1
				}
				if receipt.RecentChanges != wantCount || receipt.CoolingDown != hit {
					t.Fatalf("work selection did not affect later contact: %+v", receipt)
				}
			}
			if !foundContact {
				t.Fatal("missing real cross-family contact receipt")
			}
			if same.Draw != first.Draw || same.WorldTime != first.WorldTime {
				t.Fatal("same-hour work rerolled")
			}
			_, later := waitAt(careerTime(1, 13, 15), "later-hour")
			if hit && (!later.CoolingDown || later.Draw.ChanceBasisPoints != 0 || later.Draw.Selected) {
				t.Fatalf("same notice repeated: %+v", later)
			}
			tx, err = beginImmediate(ctx, s.db)
			if err != nil {
				t.Fatal(err)
			}
			expired, err := evaluateRPWorkOpportunities(ctx, tx.conn, RPSession{InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ControlledEntityID: M2RPPlayerID}, []string{M2AgentAdaID}, careerTime(3, 12, 15))
			tx.Rollback(ctx)
			if err != nil || len(expired) != 0 {
				t.Fatalf("out-of-window notice recycled: %+v %v", expired, err)
			}
			if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
				t.Fatal(err)
			}
			if differences, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
				t.Fatalf("work recovery: %+v %v", differences, err)
			}
		})
	}
}
