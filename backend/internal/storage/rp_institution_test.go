package storage

import (
	"context"
	"corerp.local/backend/internal/core"
	"path/filepath"
	"testing"
)

func TestRPInstitutionProposalAndEnactmentRequireIndependentRoles(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "institution.db")
	s := openCareerTestWorld(t, path)
	defer func() { s.Close() }()
	org, err := s.DefineCareerOrganization(ctx, careerTestOrg(t, s))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DefineRPCultureTerritory(ctx, CultureTerritoryRequest{Binding: careerTestBinding(t, s, "principal_creator", "scope"), ScopeKind: "region", ScopeID: "cafe", StewardID: M2AgentBoID, PlaceIDs: []string{M2AgentCafeID}}); err != nil {
		t.Fatal(err)
	}
	setup := InstitutionDefinitionRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "institution"), InstitutionID: "cafe-council", ScopeKind: "region", ScopeID: "cafe", TreasuryOrganizationID: org.Fact.OrganizationID, LegislatorID: M2AgentAdaID, EnforcerID: M2AgentBoID, ReviewerID: M2RPNPCID}
	if _, err := s.DefineRPInstitution(ctx, setup); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("culture steward installed institution: %v", err)
	}
	setup.Binding = careerTestBinding(t, s, "principal_creator", "institution")
	installed, err := s.DefineRPInstitution(ctx, setup)
	if err != nil {
		t.Fatal(err)
	}
	if len(installed.Fact.Definition.Roles) != 3 || installed.Fact.Definition.TreasuryAccountID != m2EconomyEmployerCash {
		t.Fatal("institution did not reuse roles/treasury")
	}
	assertM2Value(t, ctx, s, `SELECT balance_minor FROM account_balances WHERE account_id=?`, []any{m2EconomyEmployerCash}, 1200)
	proposal, err := s.ProposeRPLaw(ctx, LawProposalRequest{Binding: careerTestBinding(t, s, M2RPNPCPrincipal, "proposal"), InstitutionID: setup.InstitutionID, ProposerID: M2RPNPCID, Law: LawDefinition{LawID: "quiet-room", ProhibitedAction: "speak", FineMinor: 2, Text: "Keep this district quiet."}})
	if err != nil {
		t.Fatal(err)
	}
	enact := LawEnactmentRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "enact"), InstitutionID: setup.InstitutionID, LegislatorID: M2AgentBoID, ProposalEventID: proposal.EventID, EffectiveWorldTime: M2AgentNoonTime}
	if _, err := s.EnactRPLaw(ctx, enact); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("enforcer/cultural steward legislated: %v", err)
	}
	enact.Binding = careerTestBinding(t, s, M2AgentAdaPrincipal, "enact")
	enact.LegislatorID = M2AgentAdaID
	tooEarly := enact
	tooEarly.EffectiveWorldTime = rpLifeSetupTime
	if _, err := s.EnactRPLaw(ctx, tooEarly); !core.HasCode(err, core.CodeInvalidArgument) {
		t.Fatalf("nonfuture law accepted: %v", err)
	}
	s.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "enact rollback") }
	if _, err := s.EnactRPLaw(ctx, enact); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatalf("rollback: %v", err)
	}
	s.beforeCommit = nil
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='RPInstitutionFactRecorded' AND json_extract(payload,'$.kind')='law_enactment'`, nil, 0)
	enacted, err := s.EnactRPLaw(ctx, enact)
	if err != nil {
		t.Fatal(err)
	}
	if enacted.Fact.Enactment.ProposalEventID != proposal.EventID || enacted.Fact.Enactment.EffectiveWorldTime != M2AgentNoonTime {
		t.Fatal("enactment lost proposal/effective time")
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_knowledge WHERE source_event_id=?`, []any{enacted.EventID}, 0)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := s.EnactRPLaw(ctx, enact)
	if err != nil || !retry.Replayed || retry.EventID != enacted.EventID {
		t.Fatalf("enactment recovery %+v %v", retry, err)
	}
}
