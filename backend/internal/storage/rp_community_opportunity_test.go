package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPCommunityOpportunityActualReactionPrivacyAndRecovery(t *testing.T) {
	for _, hit := range []bool{false, true} {
		t.Run(fmt.Sprint(hit), func(t *testing.T) {
			ctx := context.Background()
			path := filepath.Join(t.TempDir(), "community.db")
			s := openCareerTestWorld(t, path)
			defer func() { s.Close() }()
			org, err := s.DefineCareerOrganization(ctx, careerTestOrg(t, s))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.DefineRPCultureTerritory(ctx, CultureTerritoryRequest{Binding: careerTestBinding(t, s, "principal_creator", "territory"), ScopeKind: "region", ScopeID: "cafe", StewardID: M2AgentBoID, PlaceIDs: []string{M2AgentCafeID}}); err != nil {
				t.Fatal(err)
			}
			if _, err := s.DefineRPInstitution(ctx, InstitutionDefinitionRequest{Binding: careerTestBinding(t, s, "principal_creator", "institution"), InstitutionID: "council", ScopeKind: "region", ScopeID: "cafe", TreasuryOrganizationID: org.Fact.OrganizationID, LegislatorID: M2AgentAdaID, EnforcerID: M2AgentBoID, ReviewerID: M2RPNPCID}); err != nil {
				t.Fatal(err)
			}
			enact := func(key, previous, at string, repeal bool) InstitutionRecord {
				t.Helper()
				proposal, err := s.ProposeRPLaw(ctx, LawProposalRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, key+"-proposal"), InstitutionID: "council", ProposerID: M2AgentAdaID, PreviousEnactmentEventID: previous, Repealed: repeal, Law: LawDefinition{LawID: "quiet", ProhibitedAction: "speak", FineMinor: 0, Text: "Local quiet rule"}})
				if err != nil {
					t.Fatal(err)
				}
				out, err := s.EnactRPLaw(ctx, LawEnactmentRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, key+"-enact"), InstitutionID: "council", LegislatorID: M2AgentAdaID, ProposalEventID: proposal.EventID, EffectiveWorldTime: at})
				if err != nil {
					t.Fatal(err)
				}
				return out
			}
			original := enact("initial", "", careerTime(1, 12, 30), false)
			repeal := enact("repeal", original.EventID, careerTime(1, 13, 0), true)
			r := OpportunityPolicyRequest{Binding: careerTestBinding(t, s, "principal_creator", "community-policy"), Policy: RPOpportunityPolicy{CommunityBasisPoints: 5000, CooldownHours: 1, HistoryHours: 24}}
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
				seed := fmt.Sprintf("community-fixture-%d", i)
				draw, err := core.DrawRPOpportunity(core.RPOpportunityDrawKey{PolicyEventID: policyID, StreamSeed: seed, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ActorID: M2AgentAdaID, Kind: "community_change", SourceEventID: repeal.EventID, WorldTime: careerTime(1, 13, 15)}, 5000)
				if err != nil {
					t.Fatal(err)
				}
				if draw.Selected == hit {
					r.Policy.StreamSeed = seed
					break
				}
			}
			bad := r
			bad.Policy.CommunityBasisPoints = 5001
			if _, err := s.DefineRPOpportunityPolicy(ctx, bad); !core.HasCode(err, core.CodeInvalidArgument) {
				t.Fatalf("unbounded community chance: %v", err)
			}
			if installed, err := s.DefineRPOpportunityPolicy(ctx, r); err != nil || installed.EventID != policyID {
				t.Fatalf("policy: %+v %v", installed, err)
			}
			session, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: M2RPPlayerPrincipal, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2RPPlayerID, POV: "second_person", IdempotencyKey: "session"})
			if err != nil {
				t.Fatal(err)
			}
			read := core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID}
			waitAt := func(at string) (RPWaitResult, rpCommunityOpportunity) {
				t.Helper()
				view, err := s.ObserveRPSession(ctx, read)
				if err != nil {
					t.Fatal(err)
				}
				out, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: at, Budget: 1000, IdempotencyKey: at})
				if err != nil {
					t.Fatal(err)
				}
				var raw string
				if err := s.db.QueryRowContext(ctx, `SELECT payload FROM events WHERE event_id=?`, out.EventID).Scan(&raw); err != nil {
					t.Fatal(err)
				}
				var fact rpWaitEvent
				if err := json.Unmarshal([]byte(raw), &fact); err != nil {
					t.Fatal(err)
				}
				if len(fact.CommunityOpportunities) != 1 || fact.CommunityOpportunities[0].ActorID != M2AgentAdaID {
					t.Fatalf("unheard/off-scene actor gained community receipt: %+v", fact.CommunityOpportunities)
				}
				if at == careerTime(1, 13, 45) {
					found := false
					for _, contact := range fact.ContactOpportunities {
						if contact.ActorID != M2AgentAdaID {
							continue
						}
						found = true
						count := 0
						if hit {
							count = 1
						}
						if contact.RecentChanges != count || contact.CoolingDown != hit {
							t.Fatalf("community did not enter contact pressure: %+v", contact)
						}
					}
					if !found {
						t.Fatal("missing actual new friendship receipt")
					}
				}
				if err := s.db.QueryRowContext(ctx, `SELECT payload FROM outbox WHERE event_id=?`, out.EventID).Scan(&raw); err != nil {
					t.Fatal(err)
				}
				if strings.Contains(raw, "community_opportunities") || strings.Contains(raw, repeal.EventID) {
					t.Fatal("private receipt broadcast")
				}
				return out, fact.CommunityOpportunities[0]
			}
			wait, first := waitAt(careerTime(1, 13, 15))
			if first.Draw.Selected != hit || first.Draw.ChanceBasisPoints != 5000 || first.Source.Law.EnactmentEventID != repeal.EventID || first.Source.AvailableSince != careerTime(1, 13, 0) {
				t.Fatalf("sourced draw: %+v", first)
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
				t.Logf("community actor: selected=%t sociability=%d activity=%s next=%+v lawful=%v matches=%t", hit, in.Life.Disposition.Sociability, in.ActivityCode, in.NextSchedule, in.Law.LawfulActions, core.RPSelectedCommunityChange(in))
				if in.CommunityOpportunity == nil || in.CommunityOpportunity.Selected != hit || in.CommunityOpportunity.Law.EnactmentEventID != repeal.EventID {
					t.Fatalf("lost own-known context: %+v", in.CommunityOpportunity)
				}
				encoded, _ := json.Marshal(in)
				if strings.Contains(string(encoded), "roll_basis_points") || strings.Contains(string(encoded), r.Policy.StreamSeed) {
					t.Fatal("raw draw reached provider")
				}
				return (core.DeterministicRPDecisionProvider{}).Propose(ctx, in)
			})
			request := core.RPInitiativeRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, NPCEntityID: M2AgentAdaID, TriggerEventID: wait.EventID}
			out, err := s.RunRPInitiative(ctx, request, provider)
			if err != nil {
				t.Fatal(err)
			}
			want, heard := "silence", int64(0)
			if hit {
				want, heard = "respond", 1
			}
			if out.Action != want || calls != 1 {
				t.Fatalf("actual reaction: %+v calls=%d", out, calls)
			}
			assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_knowledge WHERE observer_agent_id=? AND source_event_id=?`, []any{M2RPPlayerID, out.EventID}, heard)
			if retry, err := s.RunRPInitiative(ctx, request, provider); err != nil || !retry.Replayed || calls != 1 {
				t.Fatalf("repeated provider: %+v %v", retry, err)
			}
			for _, key := range []string{"gift-one", "gift-two"} {
				gift := socialRequest(t, ctx, s, read, M2AgentAdaID, "gift", key)
				gift.AmountMinor = 1
				if _, err := s.SocialRP(ctx, gift); err != nil {
					t.Fatal(err)
				}
			}
			_, same := waitAt(careerTime(1, 13, 45))
			if !reflect.DeepEqual(same, first) {
				t.Fatal("same-hour receipt rerolled")
			}
			_, later := waitAt(careerTime(1, 14, 15))
			if hit && (!later.CoolingDown || later.Draw.Selected || later.Draw.ChanceBasisPoints != 0 || later.RecentChanges != 1) {
				t.Fatalf("same-source repeat: %+v", later)
			}
			if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
				t.Fatalf("community projections: %+v %v", diff, err)
			}
		})
	}
}
