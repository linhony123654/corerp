package storage

import (
	"context"
	"corerp.local/backend/internal/core"
	"path/filepath"
	"testing"
)

func TestRPCultureOrganizationRequiresAuthorityAndEffectiveEmployment(t *testing.T) {
	ctx := context.Background()
	s := openCareerTestWorld(t, filepath.Join(t.TempDir(), "org-culture.db"))
	defer s.Close()
	offer := prepareCareerEmploymentOffer(t, s)
	accepted, err := s.AcceptCareerOffer(ctx, core.CareerOfferAcceptRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "accept"), OfferID: offer.Fact.RecordID, AfterWorkPlaceID: M2AgentCafeID})
	if err != nil {
		t.Fatal(err)
	}
	job := accepted.Fact.Employment
	req := CultureDefinitionRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "org-culture"), AuthorID: M2AgentBoID, Culture: core.RPCulture{CultureID: "cooperative_custom", ScopeID: job.OrganizationID, ScopeKind: "organization", GroupIdentity: "cooperative members", Norms: []core.RPCultureNorm{{NormID: "gift", Action: "gift", Evaluation: -2}}}}
	bad := req
	bad.Binding.PrincipalID = M2AgentAdaPrincipal
	bad.AuthorID = M2AgentAdaID
	if _, err := s.DefineRPCulture(ctx, bad); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("employee defined organizational culture without authority: %v", err)
	}
	definition, err := s.DefineRPCulture(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if definition.Fact.ScopeSourceEventID == "" {
		t.Fatal("organization provenance missing")
	}
	// A revoked independent managerial grant cannot retrieve even its own old
	// private definition via a retry. Fixture changes only the tested ACL grant.
	if _, err := s.db.Exec(`UPDATE capability_grants SET status='revoked' WHERE principal_id=? AND capability_id=? AND subject_id=?`, M2AgentBoPrincipal, careerManageCapability, job.OrganizationID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DefineRPCulture(ctx, req); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("revoked manager retry: %v", err)
	}
	if _, err := s.db.Exec(`UPDATE capability_grants SET status='active' WHERE principal_id=? AND capability_id=? AND subject_id=?`, M2AgentBoPrincipal, careerManageCapability, job.OrganizationID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RunAgentLife(ctx, careerTime(0, 12, 0), 100); err != nil {
		t.Fatal(err)
	}
	news, err := s.TransmitRPCulture(ctx, CultureTransmissionRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "news"), SpeakerID: M2AgentBoID, DefinitionEventID: definition.EventID})
	if err != nil {
		t.Fatal(err)
	}
	join := CultureAffiliationRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "join"), EntityID: M2AgentAdaID, DefinitionEventID: definition.EventID, TransmissionEventID: news.EventID, Decision: "join"}
	if _, err := s.AffiliateRPCulture(ctx, join); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("future employment qualified early: %v", err)
	}
	if _, err := s.RunAgentLife(ctx, careerTime(job.StartsOnDay, 12, 0), 200); err != nil {
		t.Fatal(err)
	}
	// Explicit news again covers real co-location; employment itself gives no
	// omniscient cultural knowledge and manager authority does not force joining.
	news, err = s.TransmitRPCulture(ctx, CultureTransmissionRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "effective-news"), SpeakerID: M2AgentBoID, DefinitionEventID: definition.EventID})
	if err != nil {
		t.Fatal(err)
	}
	join.Binding = careerTestBinding(t, s, M2AgentAdaPrincipal, "join")
	join.TransmissionEventID = news.EventID
	joined, err := s.AffiliateRPCulture(ctx, join)
	if err != nil {
		t.Fatal(err)
	}
	if joined.Fact.Affiliation.EmploymentContractID != job.ContractID || joined.Fact.Affiliation.EligibilityEventID == "" {
		t.Fatal("join lost actual contract provenance")
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='RPCultureFactRecorded' AND json_extract(payload,'$.kind')='internalization'`, nil, 0)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM capability_grants WHERE definition_event_id=?`, []any{joined.EventID}, 0)
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	own := readCareerTestContext(t, s, M2AgentAdaID)
	if len(own.Life.CultureAffiliations) != 1 || own.Life.CultureAffiliations[0].EmploymentContractID != job.ContractID {
		t.Fatal("organization affiliation lost on rebuild")
	}
}
