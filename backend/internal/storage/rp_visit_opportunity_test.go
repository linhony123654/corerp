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

func TestRPVisitOpportunityActualMovementAndRecovery(t *testing.T) {
	for _, rare := range []bool{false, true} {
		for _, hit := range []bool{false, true} {
			for _, blocked := range []bool{false, true} {
				if blocked && !hit {
					continue
				}
				t.Run(fmt.Sprintf("rare=%t/hit=%t/blocked=%t", rare, hit, blocked), func(t *testing.T) {
					selected := hit && !blocked
					ctx := context.Background()
					path := filepath.Join(t.TempDir(), "visit.db")
					s, err := Open(ctx, path)
					if err != nil {
						t.Fatal(err)
					}
					defer func() { s.Close() }()
					r := newBackgroundCandidate(t, ctx, s)
					r.Schedule = r.Schedule[:1]
					if _, err := s.MaterializeRPBackground(ctx, r); err != nil {
						t.Fatal(err)
					}
					session, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: M2RPPlayerPrincipal, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2RPPlayerID, POV: "second_person", IdempotencyKey: "visit-player"})
					if err != nil {
						t.Fatal(err)
					}
					player := core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID}
					for _, key := range []string{"gift-one", "gift-two"} {
						gift := socialRequest(t, ctx, s, player, r.EntityID, "gift", key)
						gift.AmountMinor = 1
						if _, err := s.SocialRP(ctx, gift); err != nil {
							t.Fatal(err)
						}
					}
					move := func(read core.RPSessionReadRequest, place, key string) {
						t.Helper()
						view, err := s.ObserveRPSession(ctx, read)
						if err != nil {
							t.Fatal(err)
						}
						if view.PlaceID == place {
							return
						}
						if _, err := s.MoveRP(ctx, core.RPMoveRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, FromPlaceID: view.PlaceID, ToPlaceID: place, ExpectedCursor: view.ObservationCursor, IdempotencyKey: key}); err != nil {
							t.Fatal(err)
						}
					}
					waitAt := func(read core.RPSessionReadRequest, at, key string) RPWaitResult {
						t.Helper()
						view, err := s.ObserveRPSession(ctx, read)
						if err != nil {
							t.Fatal(err)
						}
						out, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: at, Budget: 1000, IdempotencyKey: key})
						if err != nil || out.CurrentWorldTime != at {
							t.Fatalf("wait: %+v %v", out, err)
						}
						return out
					}
					move(player, "place_m2_home_ada", "friend-away")
					day := 0
					if rare {
						day = 7
					}
					waitAt(player, careerTime(day, 3, 0), "age-source")
					// Bo supplies the local HOT scene without refreshing Nora's encounter
					// with her old friend Lin, who remains elsewhere.
					bo := allowFixtureControl(t, ctx, s, M2AgentBoID)
					move(bo, "place_m2_home_bo", "observer-home")
					tx, err := beginImmediate(ctx, s.db)
					if err != nil {
						t.Fatal(err)
					}
					input, err := readRPOwnDecisionContext(ctx, tx.conn, core.RPDecisionInput{InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, NPCEntityID: r.EntityID, InterlocutorEntityID: M2AgentBoID})
					if err != nil {
						tx.Rollback(ctx)
						t.Fatal(err)
					}
					sources, err := readRPVisitSources(ctx, tx.conn, input, careerTime(0, 0, 0))
					tx.Rollback(ctx)
					if err != nil || len(sources) == 0 {
						t.Fatalf("sources: %+v %v", sources, err)
					}
					if input.Life.Disposition.Sociability == 0 {
						t.Fatal("fixture actor does not want discretionary visits")
					}
					source := sources[0]
					kind, chance := "familiar_public_place", 5000
					if rare {
						kind, chance = "old_friend_place", 100
					}
					if source.Kind != kind {
						t.Fatalf("wrong own source: %+v", source)
					}
					if blocked {
						_, err := s.DefineRPTransitWorks(ctx, TransitWorksRequest{Binding: careerTestBinding(t, s, "principal_creator", "visit-road"), FromPlaceID: "place_m2_home_bo", ToPlaceID: M2AgentCafeID, StartsAt: careerTime(day, 3, 10), EndsAt: careerTime(day, 3, 20)})
						if err != nil {
							t.Fatal(err)
						}
					}
					policy := OpportunityPolicyRequest{Binding: careerTestBinding(t, s, "principal_creator", "visit-policy"), Policy: RPOpportunityPolicy{VisitBasisPoints: 5000, CooldownHours: 1, HistoryHours: 240}}
					if rare {
						policy.Policy.RareVisitBasisPoints = 100
					}
					keyHash, _ := core.HashJSON([]string{policy.Binding.PrincipalID, policy.Binding.IdempotencyKey})
					idHash, _ := core.HashJSON([]string{policy.Binding.InstanceID, policy.Binding.BranchID, "DefineRPOpportunityPolicy", keyHash})
					sourceHash, _ := core.HashJSON(source)
					for i := 0; i < 100000; i++ {
						seed := fmt.Sprintf("visit-fixture-%d", i)
						draw, err := core.DrawRPOpportunity(core.RPOpportunityDrawKey{PolicyEventID: "event_opportunity_" + idHash[7:], StreamSeed: seed, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ActorID: r.EntityID, Kind: kind, SourceEventID: sourceHash, WorldTime: careerTime(day, 3, 15)}, chance)
						if err != nil {
							t.Fatal(err)
						}
						if draw.Selected == hit {
							policy.Policy.StreamSeed = seed
							break
						}
					}
					if policy.Policy.StreamSeed == "" {
						t.Fatal("no deterministic fixture seed")
					}
					if _, err := s.DefineRPOpportunityPolicy(ctx, policy); err != nil {
						t.Fatal(err)
					}
					wait := waitAt(bo, careerTime(day, 3, 15), "visit-draw")
					var raw string
					if err := s.db.QueryRowContext(ctx, `SELECT payload FROM events WHERE event_id=?`, wait.EventID).Scan(&raw); err != nil {
						t.Fatal(err)
					}
					var fact rpWaitEvent
					if err := json.Unmarshal([]byte(raw), &fact); err != nil {
						t.Fatal(err)
					}
					if len(fact.VisitOpportunities) != 1 {
						t.Fatalf("visit receipts: %+v", fact.VisitOpportunities)
					}
					receipt := fact.VisitOpportunities[0]
					wantChance := chance
					if blocked {
						wantChance = 0
					}
					if receipt.Source != source || receipt.Rare != rare || receipt.Draw.Selected != selected || receipt.Draw.ChanceBasisPoints != wantChance || receipt.RouteAvailable == blocked || receipt.Quiet {
						t.Fatalf("draw: %+v", receipt)
					}
					if err := s.db.QueryRowContext(ctx, `SELECT payload FROM outbox WHERE event_id=?`, wait.EventID).Scan(&raw); err != nil {
						t.Fatal(err)
					}
					if strings.Contains(raw, "visit_opportunities") || strings.Contains(raw, sourceHash) {
						t.Fatal("private visit broadcast")
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
					calls := 0
					provider := rpDecisionProviderFunc(func(ctx context.Context, in core.RPDecisionInput) (core.RPDecisionProposal, error) {
						calls++
						if in.VisitOpportunity == nil || in.VisitOpportunity.Selected != selected || in.VisitOpportunity.Source != source {
							t.Fatalf("lost context: %+v", in.VisitOpportunity)
						}
						encoded, _ := json.Marshal(in)
						if strings.Contains(string(encoded), policy.Policy.StreamSeed) || strings.Contains(string(encoded), "roll_basis_points") {
							t.Fatal("raw draw in provider")
						}
						return (core.DeterministicRPDecisionProvider{}).Propose(ctx, in)
					})
					request := core.RPInitiativeRequest{PrincipalID: bo.PrincipalID, SessionID: bo.SessionID, NPCEntityID: r.EntityID, TriggerEventID: wait.EventID}
					s.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "visit rollback") }
					if _, err := s.RunRPInitiative(ctx, request, provider); !core.HasCode(err, core.CodeInjectedFailure) {
						t.Fatalf("rollback: %v", err)
					}
					s.beforeCommit = nil
					assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_positions WHERE agent_id=? AND place_id='place_m2_home_bo'`, []any{r.EntityID}, 1)
					calls = 0
					out, err := s.RunRPInitiative(ctx, request, provider)
					if err != nil {
						t.Fatal(err)
					}
					want, destination := "silence", "place_m2_home_bo"
					if selected {
						want, destination = "leave", M2AgentCafeID
					}
					if out.Action != want || calls != 1 {
						t.Fatalf("actual behavior: %+v calls=%d", out, calls)
					}
					assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_positions WHERE agent_id=? AND place_id=?`, []any{r.EntityID, destination}, 1)
					assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_positions WHERE agent_id=? AND place_id='place_m2_home_ada'`, []any{M2RPPlayerID}, 1)
					if retry, err := s.RunRPInitiative(ctx, request, provider); err != nil || !retry.Replayed || calls != 1 {
						t.Fatalf("duplicate initiative: %+v %v", retry, err)
					}
					at, _ := time.Parse(time.RFC3339, careerTime(day, 3, 30))
					n, cooling, err := rpOpportunityHistoryPressure([]rpWaitEvent{fact, fact}, r.EntityID, at, policy.Policy)
					wantCount := 0
					if selected {
						wantCount = 1
					}
					if err != nil || n != wantCount || cooling != selected {
						t.Fatalf("shared pressure: %d %t %v", n, cooling, err)
					}
					if !selected {
						later := waitAt(bo, careerTime(day, 3, 30), "same-hour")
						if err := s.db.QueryRowContext(ctx, `SELECT payload FROM events WHERE event_id=?`, later.EventID).Scan(&raw); err != nil {
							t.Fatal(err)
						}
						var next rpWaitEvent
						if err := json.Unmarshal([]byte(raw), &next); err != nil {
							t.Fatal(err)
						}
						if len(next.VisitOpportunities) != 1 || !reflect.DeepEqual(next.VisitOpportunities[0], receipt) {
							t.Fatalf("miss rerolled/fell back after route opened: %+v", next.VisitOpportunities)
						}
					} else {
						// A real new friendship in the destination must consume the
						// earlier visit's pressure, not start a separate event budget.
						move(bo, M2AgentCafeID, "follow-to-cafe")
						for _, key := range []string{"bo-gift-one", "bo-gift-two"} {
							gift := socialRequest(t, ctx, s, bo, r.EntityID, "gift", key)
							gift.AmountMinor = 1
							if _, err := s.SocialRP(ctx, gift); err != nil {
								t.Fatal(err)
							}
						}
						later := waitAt(bo, careerTime(day, 3, 30), "contact-after-visit")
						if err := s.db.QueryRowContext(ctx, `SELECT payload FROM events WHERE event_id=?`, later.EventID).Scan(&raw); err != nil {
							t.Fatal(err)
						}
						var next rpWaitEvent
						if err := json.Unmarshal([]byte(raw), &next); err != nil {
							t.Fatal(err)
						}
						found := false
						for _, contact := range next.ContactOpportunities {
							if contact.ActorID != r.EntityID {
								continue
							}
							found = true
							if contact.RecentChanges != 1 || !contact.CoolingDown || contact.Draw.Selected || contact.Draw.ChanceBasisPoints != 0 {
								t.Fatalf("visit failed to suppress contact: %+v", contact)
							}
						}
						if !found {
							t.Fatal("missing sourced contact after actual gifts")
						}
					}
					if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
						t.Fatal(err)
					}
					if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
						t.Fatalf("projections: %+v %v", diff, err)
					}
				})
			}
		}
	}
}
