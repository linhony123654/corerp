package storage

import (
	"context"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPInstitutionGrantDamageDetectionAndRepair(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "institution-grants.db")
	s := openCareerTestWorld(t, path)
	defer func() { s.Close() }()
	org, err := s.DefineCareerOrganization(ctx, careerTestOrg(t, s))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DefineRPCultureTerritory(ctx, CultureTerritoryRequest{Binding: careerTestBinding(t, s, "principal_creator", "world"), ScopeKind: "world", ScopeID: M2DemoInstanceID, StewardID: M2AgentBoID}); err != nil {
		t.Fatal(err)
	}
	installed, err := s.DefineRPInstitution(ctx, InstitutionDefinitionRequest{Binding: careerTestBinding(t, s, "principal_creator", "institution"), InstitutionID: "council", ScopeKind: "world", ScopeID: M2DemoInstanceID, TreasuryOrganizationID: org.Fact.OrganizationID, LegislatorID: M2AgentAdaID, EnforcerID: M2AgentBoID, ReviewerID: M2RPNPCID})
	if err != nil {
		t.Fatal(err)
	}
	var head int64
	if err := s.db.QueryRow(`SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&head); err != nil {
		t.Fatal(err)
	}
	authorize := func(role InstitutionRole) error {
		conn, err := s.db.Conn(ctx)
		if err != nil {
			return err
		}
		defer conn.Close()
		return authorizeInstitutionRole(ctx, conn, core.CareerBinding{PrincipalID: role.PrincipalID, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID}, "council", role.EntityID, role.CapabilityID)
	}
	for _, role := range installed.Fact.Definition.Roles {
		for _, damage := range []string{
			`DELETE FROM capability_grants WHERE grant_id=?`,
			`UPDATE capability_grants SET subject_id='wrong',field_scope='["private"]',amount_limit_minor=99,status='revoked' WHERE grant_id=?`,
			`UPDATE capability_grants SET capability_id='world.cohort.materialize' WHERE grant_id=?`,
		} {
			if _, err := s.db.Exec(damage, role.GrantID); err != nil {
				t.Fatal(err)
			}
			if err := authorize(role); !core.HasCode(err, core.CodeUnauthorized) {
				t.Fatalf("damaged grant authorized: %v", err)
			}
			diffs, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, d := range diffs {
				if d.Projection == "institution_role_grant" && d.Key == role.GrantID {
					found = true
				}
			}
			if !found {
				t.Fatalf("damage not diagnosed %+v", diffs)
			}
			if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
				t.Fatal(err)
			}
			if err := authorize(role); err != nil {
				t.Fatalf("repaired role unusable: %v", err)
			}
			diffs, err = s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID)
			if err != nil || len(diffs) != 0 {
				t.Fatalf("repair mismatch %+v %v", diffs, err)
			}
		}
	}
	// An extra row with a real source Event is still not an issued role.
	if _, err := s.db.Exec(`INSERT INTO capability_grants(grant_id,principal_id,capability_id,instance_id,branch_id,subject_id,field_scope,status,definition_event_id) VALUES ('bogus-institution-grant',?,?,?,?,'bogus','[]','active',?)`, M2AgentAdaPrincipal, institutionReview, M2DemoInstanceID, M2DemoBranchID, installed.EventID); err != nil {
		t.Fatal(err)
	}
	diffs, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, d := range diffs {
		if d.Key == "bogus-institution-grant" && d.ExpectedText == "" {
			found = true
		}
	}
	if !found {
		t.Fatal("unsourced privilege not diagnosed")
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
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM capability_grants WHERE grant_id='bogus-institution-grant'`, nil, 0)
	assertM2Value(t, ctx, s, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, []any{M2DemoInstanceID, M2DemoBranchID}, head)
	p, err := s.ProposeRPLaw(ctx, LawProposalRequest{Binding: careerTestBinding(t, s, M2RPNPCPrincipal, "proposal"), InstitutionID: "council", ProposerID: M2RPNPCID, Law: LawDefinition{LawID: "quiet", ProhibitedAction: "speak", FineMinor: 2, Text: "Quiet rule."}})
	if err != nil {
		t.Fatal(err)
	}
	enact := LawEnactmentRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "enact"), InstitutionID: "council", LegislatorID: M2AgentAdaID, ProposalEventID: p.EventID, EffectiveWorldTime: M2AgentNoonTime}
	if _, err := s.EnactRPLaw(ctx, enact); err != nil {
		t.Fatalf("repaired law command: %v", err)
	}
	role := installed.Fact.Definition.Roles[0]
	change := InstitutionAuthorityRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "revoke"), InstitutionID: "council", CapabilityID: institutionLegislate, Decision: "revoke"}
	if _, err := s.ChangeRPInstitutionAuthority(ctx, change); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("office holder changed appointment: %v", err)
	}
	change.Binding = careerTestBinding(t, s, "principal_creator", "revoke")
	s.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "role rollback") }
	if _, err := s.ChangeRPInstitutionAuthority(ctx, change); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatalf("role rollback: %v", err)
	}
	s.beforeCommit = nil
	if err := authorize(role); err != nil {
		t.Fatalf("failed revocation changed authority: %v", err)
	}
	revoked, err := s.ChangeRPInstitutionAuthority(ctx, change)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.EnactRPLaw(ctx, enact); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("revoked holder replayed privileged command: %v", err)
	}
	// A formerly valid installation cannot be replayed into the mutable ACL to
	// resurrect revoked authority, even before explicit projection repair.
	if _, err := s.db.Exec(`UPDATE capability_grants SET status='active',definition_event_id=? WHERE grant_id=?`, installed.EventID, role.GrantID); err != nil {
		t.Fatal(err)
	}
	if err := authorize(role); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("stale source resurrected role: %v", err)
	}
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM capability_grants WHERE grant_id=? AND status='revoked' AND definition_event_id=?`, []any{role.GrantID, revoked.EventID}, 1)
	appointment := InstitutionAuthorityRequest{Binding: careerTestBinding(t, s, "principal_creator", "appoint"), InstitutionID: "council", CapabilityID: institutionLegislate, Decision: "appoint", EntityID: M2AgentBoID}
	appointed, err := s.ChangeRPInstitutionAuthority(ctx, appointment)
	if err != nil {
		t.Fatal(err)
	}
	newRole := appointed.Fact.Authority.Role
	if err := authorize(newRole); err != nil {
		t.Fatalf("new holder lacks authority: %v", err)
	}
	if err := authorize(role); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("former holder retained authority: %v", err)
	}
	if readCareerTestContext(t, s, M2AgentBoID).Law != nil {
		t.Fatal("appointment revealed predecessor's private legal knowledge")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`DELETE FROM capability_grants WHERE grant_id=?`, role.GrantID); err != nil {
		t.Fatal(err)
	}
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	if err := authorize(newRole); err != nil {
		t.Fatalf("rebuild lost new role holder: %v", err)
	}
	// Retry a past revocation without undoing the later appointment.
	retried, err := s.ChangeRPInstitutionAuthority(ctx, change)
	if err != nil || !retried.Replayed || retried.EventID != revoked.EventID {
		t.Fatalf("old revocation retry %+v %v", retried, err)
	}
	if err := authorize(newRole); err != nil {
		t.Fatalf("retry changed current authority: %v", err)
	}
	diffs, err = s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID)
	if err != nil || len(diffs) != 0 {
		t.Fatalf("lifecycle projections %+v %v", diffs, err)
	}
}
