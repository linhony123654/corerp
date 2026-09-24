package storage

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
)

type finalWorldFixture struct {
	CultureEventID, LawEventID, NoraPrincipal string
}

// All declarations use existing fact owners; no balances or projections are seeded.
func prepareFinalWorld(t *testing.T, s *Store, lawEffective string) finalWorldFixture {
	t.Helper()
	ctx := context.Background()
	if _, err := s.PrepareRPFinalCohorts(ctx); err != nil {
		t.Fatal(err)
	}
	for _, person := range []struct{ id, name string }{{"entity_final_nora", "Nora"}, {"entity_final_eli", "Eli"}} {
		b := careerTestBinding(t, s, "principal_creator", "final-materialize-"+person.name)
		command := m2AgentMaterialization(b.IdempotencyKey, person.id, person.name, 1, 90, 1, 0, 30, b.ExpectedHead, rpLifeSetupTime)
		if person.name == "Eli" {
			command.SourceCohortID, command.LiabilityMinor = RPFinalBlockB, 0
		}
		m, err := s.MaterializeCohort(ctx, command)
		if err != nil {
			t.Fatal(err)
		}
		_, err = s.MaterializeRPBackground(ctx, core.RPBackgroundRequest{
			PrincipalID: b.PrincipalID, InstanceID: b.InstanceID, BranchID: b.BranchID,
			EntityID: person.id, ExpectedHead: m.LastSequence, IdempotencyKey: "final-background-" + person.name,
			AgeMin: 25, AgeMax: 34, ResidencePlaceID: "place_m2_home_bo", InitialPlaceID: M2AgentCafeID,
			Schedule: []core.RPBackgroundSchedule{
				{WorldTime: "2026-09-22T18:00:00Z", PlaceID: "place_m2_home_bo", ActivityCode: "home"},
				{WorldTime: "2026-09-23T12:00:00Z", PlaceID: M2AgentCafeID, ActivityCode: "lunch"},
			},
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	org, err := s.DefineCareerOrganization(ctx, careerTestOrg(t, s))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.PostCareerPosition(ctx, careerTestPosting(t, s)); err != nil {
		t.Fatal(err)
	}
	region, err := s.DefineRPCultureTerritory(ctx, CultureTerritoryRequest{Binding: careerTestBinding(t, s, "principal_creator", "final-region"), ScopeKind: "region", ScopeID: "final-cafe", StewardID: M2AgentBoID, PlaceIDs: []string{M2AgentCafeID}})
	if err != nil {
		t.Fatal(err)
	}
	culture, err := s.DefineRPCulture(ctx, CultureDefinitionRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "final-culture"), AuthorID: M2AgentBoID, Culture: core.RPCulture{CultureID: "final-neighbor-help", ScopeKind: "region", ScopeID: "final-cafe", GroupIdentity: "neighbors", Norms: []core.RPCultureNorm{{NormID: "help", Action: "gift", Evaluation: 2}}}})
	if err != nil {
		t.Fatal(err)
	}
	if culture.Fact.ScopeSourceEventID != region.EventID {
		t.Fatal("culture lost territory source")
	}
	_, err = s.DefineRPInstitution(ctx, InstitutionDefinitionRequest{Binding: careerTestBinding(t, s, "principal_creator", "final-institution"), InstitutionID: "final-council", ScopeKind: "region", ScopeID: "final-cafe", TreasuryOrganizationID: org.Fact.OrganizationID, LegislatorID: M2AgentAdaID, EnforcerID: M2AgentBoID, ReviewerID: M2RPNPCID})
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := s.ProposeRPLaw(ctx, LawProposalRequest{Binding: careerTestBinding(t, s, M2RPNPCPrincipal, "final-proposal"), InstitutionID: "final-council", ProposerID: M2RPNPCID, Law: LawDefinition{LawID: "final-quiet", ProhibitedAction: "speak", FineMinor: 2, Text: "Quiet hours require restraint."}})
	if err != nil {
		t.Fatal(err)
	}
	law, err := s.EnactRPLaw(ctx, LawEnactmentRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "final-enact"), InstitutionID: "final-council", LegislatorID: M2AgentAdaID, ProposalEventID: proposal.EventID, EffectiveWorldTime: lawEffective})
	if err != nil {
		t.Fatal(err)
	}
	// Enacting a rule cannot grant every agent knowledge of it.
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_knowledge WHERE source_event_id=?`, []any{law.EventID}, 0)
	var principal string
	if err := s.db.QueryRowContext(ctx, `SELECT principal_id FROM agent_profiles WHERE agent_id='entity_final_nora'`).Scan(&principal); err != nil {
		t.Fatal(err)
	}
	return finalWorldFixture{culture.EventID, law.EventID, principal}
}

// This is the first Final composition probe, not the 300-turn/30-day acceptance.
func TestFinalWorldCompositionDayAndRecovery(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "final-composition.db")
	s := openCareerTestWorld(t, path)
	defer func() { s.Close() }()
	prepareFinalWorld(t, s, M2AgentNoonTime)
	session, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: M2RPPlayerPrincipal, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2RPPlayerID, POV: "second_person", IdempotencyKey: "final-session"})
	if err != nil {
		t.Fatal(err)
	}
	read := core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID}
	for turn := 0; turn < 10; turn++ {
		if turn == 5 {
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			s, err = Open(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
		}
		view, err := s.ObserveRPSession(ctx, read)
		if err != nil {
			t.Fatal(err)
		}
		r, err := s.PlayRPTurn(ctx, core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, Text: fmt.Sprintf("今天的街区生活怎么样？这是第%d次交谈。", turn+1), IdempotencyKey: fmt.Sprintf("final-probe-%d", turn)})
		if err != nil || r.Status != "settled" {
			t.Fatalf("turn %d: %+v %v", turn, r, err)
		}
	}
	view, err := s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewRPService(s, core.DeterministicRPDecisionProvider{}, "deterministic")
	if err != nil {
		t.Fatal(err)
	}
	wait, err := service.WaitRP(ctx, core.RPWaitRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: "2026-09-23T07:03:00Z", Budget: 1000, IdempotencyKey: "final-probe-day"})
	if err != nil || wait.Status != "completed" || wait.CurrentWorldTime != "2026-09-23T07:03:00Z" {
		t.Fatalf("day: %+v %v", wait, err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_turn_runs WHERE status='settled'`, nil, 10)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_profiles WHERE status='active' AND agent_id<>?`, []any{M2RPPlayerID}, 5)
	assertM2Value(t, ctx, s, `SELECT population_count FROM cohorts WHERE cohort_id=?`, []any{M2DemoCohortID}, 15)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM cohorts WHERE instance_id=? AND branch_id=?`, []any{M2DemoInstanceID, M2DemoBranchID}, 3)
	assertM2Value(t, ctx, s, `SELECT population_count FROM cohorts WHERE cohort_id=?`, []any{RPFinalBlockB}, 3)
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatalf("composition replay: %+v %v", diff, err)
	}
	t.Log("five NPCs plus player, three cohorts, sourced society setup, ten settled turns, one-day Wait, reopen and replay; full Final still pending")
}
