package storage

import (
	"context"
	"corerp.local/backend/internal/core"
	"path/filepath"
	"testing"
)

func TestRPCulturalFamilyNeedsMutualSourceNotColocation(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "family.db")
	s := openCareerTestWorld(t, path)
	defer func() { s.Close() }()
	if _, err := s.RunAgentLife(ctx, M2AgentNoonTime, 100); err != nil {
		t.Fatal(err)
	}
	proposal, err := s.ProposeRPCulturalFamily(ctx, CultureFamilyProposalRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "family"), FamilyID: "chosen-household", ProposerID: M2AgentBoID, InviteeID: M2AgentAdaID})
	if err != nil {
		t.Fatal(err)
	}
	define := CultureDefinitionRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "family-culture"), AuthorID: M2AgentBoID, Culture: core.RPCulture{CultureID: "family-custom", ScopeID: "chosen-household", ScopeKind: "family", GroupIdentity: "chosen family", Norms: []core.RPCultureNorm{{NormID: "gift", Action: "gift", Evaluation: 2}}}}
	if _, err := s.DefineRPCulture(ctx, define); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("unconfirmed proposal became family: %v", err)
	}
	accept := CultureFamilyAcceptRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "accept"), EntityID: M2AgentAdaID, ProposalEventID: proposal.EventID}
	if _, err := s.AcceptRPCulturalFamily(ctx, accept); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("proposer accepted for invitee: %v", err)
	}
	accept.Binding = careerTestBinding(t, s, M2AgentAdaPrincipal, "accept")
	s.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "family rollback") }
	if _, err := s.AcceptRPCulturalFamily(ctx, accept); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatalf("rollback: %v", err)
	}
	s.beforeCommit = nil
	if _, err := s.DefineRPCulture(ctx, define); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("rolled back family became authority: %v", err)
	}
	accepted, err := s.AcceptRPCulturalFamily(ctx, accept)
	if err != nil {
		t.Fatal(err)
	}
	define.Binding = careerTestBinding(t, s, M2AgentBoPrincipal, "family-culture")
	definition, err := s.DefineRPCulture(ctx, define)
	if err != nil {
		t.Fatal(err)
	}
	if definition.Fact.ScopeSourceEventID != accepted.EventID {
		t.Fatal("family culture missing mutual source")
	}
	outsider := define
	outsider.AuthorID = M2RPNPCID
	outsider.Binding = careerTestBinding(t, s, M2RPNPCPrincipal, "outsider")
	outsider.Culture.CultureID = "outsider-custom"
	if _, err := s.DefineRPCulture(ctx, outsider); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("colocated outsider became family: %v", err)
	}
	news, err := s.TransmitRPCulture(ctx, CultureTransmissionRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "news"), SpeakerID: M2AgentBoID, DefinitionEventID: definition.EventID})
	if err != nil {
		t.Fatal(err)
	}
	join := CultureAffiliationRequest{Binding: careerTestBinding(t, s, M2RPNPCPrincipal, "outsider-join"), EntityID: M2RPNPCID, DefinitionEventID: definition.EventID, TransmissionEventID: news.EventID, Decision: "join"}
	if _, err := s.AffiliateRPCulture(ctx, join); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("hearing manufactured family affiliation: %v", err)
	}
	join.Binding = careerTestBinding(t, s, M2AgentAdaPrincipal, "join")
	join.EntityID = M2AgentAdaID
	joined, err := s.AffiliateRPCulture(ctx, join)
	if err != nil {
		t.Fatal(err)
	}
	if joined.Fact.Affiliation.EligibilityEventID != accepted.EventID {
		t.Fatal("family affiliation lost source")
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='RPCultureFactRecorded' AND json_extract(payload,'$.kind')='internalization'`, nil, 0)
	if _, err := s.InternalizeRPCulture(ctx, CultureStanceRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "family-rebel"), EntityID: M2AgentAdaID, TransmissionEventID: news.EventID, Stance: "rebel"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := s.AcceptRPCulturalFamily(ctx, accept)
	if err != nil || !retry.Replayed || retry.EventID != accepted.EventID {
		t.Fatalf("family retry %+v %v", retry, err)
	}
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	own := readCareerTestContext(t, s, M2AgentAdaID)
	if len(own.Life.CultureAffiliations) != 1 || own.Life.CultureAffiliations[0].EligibilityEventID != accepted.EventID {
		t.Fatal("family culture lost across rebuild")
	}
}
