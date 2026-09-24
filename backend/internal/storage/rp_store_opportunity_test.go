package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPStoreOpportunityShortageReactionHistoryAndRecovery(t *testing.T) {
	for _, selected := range []bool{false, true} {
		t.Run(fmt.Sprintf("selected-%t", selected), func(t *testing.T) {
			ctx := context.Background()
			path := filepath.Join(t.TempDir(), "shortage-opportunity.db")
			s, err := Open(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { s.Close() }()
			prepareRPStorefrontWorld(t, ctx, s)
			if run, err := s.RunAgentLife(ctx, m2WageTime(25, 7, 6), 2000); err != nil || run.PendingDue != 0 {
				t.Fatalf("deplete actual stock: %+v %v", run, err)
			}
			assertM2Value(t, ctx, s, `SELECT quantity_minor FROM inventory_balances WHERE location_id=? AND sku_id=?`, []any{m2StoreLocation, M2DemoSKUID}, 0)
			prospectiveID := func(namespace, command, key string) string {
				keyHash, err := core.HashJSON([]string{"principal_creator", key})
				if err != nil {
					t.Fatal(err)
				}
				hash, err := core.HashJSON([]string{M2DemoInstanceID, M2DemoBranchID, command, keyHash})
				if err != nil {
					t.Fatal(err)
				}
				return "event_" + namespace + "_" + hash[7:]
			}
			policyID := prospectiveID("opportunity", "DefineRPOpportunityPolicy", "shortage-policy")
			sourceID := prospectiveID("storefront", "DefineRPStorefrontSource", "shortage-source")
			var stockSource string
			if err := s.db.QueryRowContext(ctx, `SELECT e.event_id FROM events e JOIN stock_movements m ON m.event_id=e.event_id WHERE m.from_location_id=? AND m.sku_id=? ORDER BY e.event_sequence DESC LIMIT 1`, m2StoreLocation, M2DemoSKUID).Scan(&stockSource); err != nil {
				t.Fatal(err)
			}
			identity, err := core.HashJSON([]string{sourceID, stockSource})
			if err != nil {
				t.Fatal(err)
			}
			at := m2WageTime(25, 8, 15)
			seed := ""
			for i := 0; i < 1000; i++ {
				candidate := fmt.Sprintf("fixture-shortage-%d", i)
				draw, err := core.DrawRPOpportunity(core.RPOpportunityDrawKey{PolicyEventID: policyID, StreamSeed: candidate, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ActorID: M2AgentBoID, Kind: "store_shortage", SourceEventID: identity, WorldTime: at}, 5000)
				if err != nil {
					t.Fatal(err)
				}
				if draw.Selected == selected {
					seed = candidate
					break
				}
			}
			if seed == "" {
				t.Fatal("missing fixture seed")
			}
			policy, err := s.DefineRPOpportunityPolicy(ctx, OpportunityPolicyRequest{Binding: careerTestBinding(t, s, "principal_creator", "shortage-policy"), Policy: RPOpportunityPolicy{StreamSeed: seed, ContactBasisPoints: 0, CooldownHours: 1, HistoryHours: 24}})
			if err != nil || policy.EventID != policyID {
				t.Fatalf("policy: %+v %v", policy, err)
			}
			source, err := s.DefineRPStorefrontSource(ctx, StorefrontSourceRequest{Binding: careerTestBinding(t, s, "principal_creator", "shortage-source"), Source: RPStorefrontSource{PlaceID: M2AgentCafeID, StoreActorID: m2StoreActorID, SKUID: M2DemoSKUID, NoticeBasisPoints: 5000}})
			if err != nil || source.EventID != sourceID {
				t.Fatalf("source: %+v %v", source, err)
			}
			session, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: M2RPPlayerPrincipal, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2RPPlayerID, POV: "second_person", IdempotencyKey: "shortage-session"})
			if err != nil {
				t.Fatal(err)
			}
			read := core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID}
			waitAt := func(target string) (RPWaitResult, rpStoreOpportunity) {
				t.Helper()
				view, err := s.ObserveRPSession(ctx, read)
				if err != nil {
					t.Fatal(err)
				}
				wait, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: target, Budget: 100, IdempotencyKey: target})
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
				assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM outbox WHERE event_id=? AND json_type(payload,'$.store_opportunities') IS NOT NULL`, []any{wait.EventID}, 0)
				for _, receipt := range event.StoreOpportunities {
					if receipt.ActorID == M2AgentBoID {
						return wait, receipt
					}
				}
				t.Fatal("missing own shortage evaluation")
				return wait, rpStoreOpportunity{}
			}
			wait, receipt := waitAt(at)
			if receipt.Draw.Selected != selected || receipt.Draw.ChanceBasisPoints != 5000 || receipt.Store.StockSourceEventID != stockSource {
				t.Fatalf("shortage draw: %+v", receipt)
			}
			request := core.RPInitiativeRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, NPCEntityID: M2AgentBoID, TriggerEventID: wait.EventID}
			input, err := s.BuildRPInitiativeInput(ctx, request)
			if err != nil || len(input.StoreOpportunities) != 1 || input.StoreOpportunities[0].Selected != selected {
				t.Fatalf("shortage context: %+v %v", input.StoreOpportunities, err)
			}
			out, err := s.RunRPInitiative(ctx, request, core.DeterministicRPDecisionProvider{})
			if err != nil {
				t.Fatal(err)
			}
			want, heard := "silence", int64(0)
			if selected {
				want, heard = "respond", 1
			}
			if out.Action != want {
				t.Fatalf("shortage reaction: %+v life=%+v", out, input.Life)
			}
			assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_knowledge WHERE observer_agent_id=? AND source_event_id=?`, []any{M2RPPlayerID, out.EventID}, heard)
			if selected {
				assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_id=? AND json_extract(payload,'$.text')='这家店有东西缺货了。'`, []any{out.EventID}, 1)
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
			retry, err := s.RunRPInitiative(ctx, request, core.DeterministicRPDecisionProvider{})
			if err != nil || !retry.Replayed || retry.EventID != out.EventID {
				t.Fatalf("shortage retry: %+v %v", retry, err)
			}
			_, repeated := waitAt(m2WageTime(25, 8, 45))
			if !reflect.DeepEqual(repeated, receipt) {
				t.Fatalf("same-hour reroll: %+v vs %+v", repeated, receipt)
			}
			if selected {
				later, suppressed := waitAt(m2WageTime(25, 9, 15))
				if !suppressed.CoolingDown || suppressed.Draw.Selected || suppressed.Draw.ChanceBasisPoints != 0 || suppressed.RecentChanges != 1 {
					t.Fatalf("same shortage repeated after hourly cooldown: %+v", suppressed)
				}
				request.TriggerEventID = later.EventID
				quiet, err := s.RunRPInitiative(ctx, request, core.DeterministicRPDecisionProvider{})
				if err != nil || quiet.Action != "silence" {
					t.Fatalf("repeat shortage speech: %+v %v", quiet, err)
				}
			}
			if differences, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(differences) != 0 {
				t.Fatalf("shortage projections: %+v %v", differences, err)
			}
		})
	}
}
