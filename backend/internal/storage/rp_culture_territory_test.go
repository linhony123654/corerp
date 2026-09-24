package storage

import (
	"context"
	"corerp.local/backend/internal/core"
	"path/filepath"
	"testing"
)

func TestRPCultureTerritoryAuthorityPlaceAndPropagation(t *testing.T) {
	ctx := context.Background()
	s := openCareerTestWorld(t, filepath.Join(t.TempDir(), "territory.db"))
	defer s.Close()
	request := CultureTerritoryRequest{Binding: careerTestBinding(t, s, "principal_creator", "region"), ScopeKind: "region", ScopeID: "cafe-district", StewardID: M2AgentBoID, PlaceIDs: []string{M2AgentCafeID}}
	bad := request
	bad.Binding.PrincipalID = M2AgentBoPrincipal
	if _, err := s.DefineRPCultureTerritory(ctx, bad); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("agent installed world scope: %v", err)
	}
	bad = request
	bad.PlaceIDs = []string{"unknown-place"}
	if _, err := s.DefineRPCultureTerritory(ctx, bad); !core.HasCode(err, core.CodeNotFound) {
		t.Fatalf("fabricated place: %v", err)
	}
	s.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "territory rollback") }
	if _, err := s.DefineRPCultureTerritory(ctx, request); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatalf("rollback: %v", err)
	}
	s.beforeCommit = nil
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM capability_grants WHERE capability_id=?`, []any{cultureDefineCapability}, 0)
	region, err := s.DefineRPCultureTerritory(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if len(region.Fact.Territory.PlaceSourceEventIDs) != 1 || region.Fact.Territory.BuilderSourceEventID == "" {
		t.Fatal("territory lacks actual sources")
	}
	define := CultureDefinitionRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "regional-culture"), AuthorID: M2AgentBoID, Culture: core.RPCulture{CultureID: "regional-custom", ScopeKind: "region", ScopeID: request.ScopeID, GroupIdentity: "district", Norms: []core.RPCultureNorm{{NormID: "gift", Action: "gift", Evaluation: 2}}}}
	badDefine := define
	badDefine.AuthorID = M2AgentAdaID
	badDefine.Binding.PrincipalID = M2AgentAdaPrincipal
	if _, err := s.DefineRPCulture(ctx, badDefine); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("unappointed regional author: %v", err)
	}
	culture, err := s.DefineRPCulture(ctx, define)
	if err != nil {
		t.Fatal(err)
	}
	if culture.Fact.ScopeSourceEventID != region.EventID {
		t.Fatal("regional culture lost scope source")
	}
	join := CultureAffiliationRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "regional-join"), EntityID: M2AgentBoID, DefinitionEventID: culture.EventID, Decision: "join"}
	if _, err := s.AffiliateRPCulture(ctx, join); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("offsite author joined region: %v", err)
	}
	if _, err := s.RunAgentLife(ctx, M2AgentNoonTime, 100); err != nil {
		t.Fatal(err)
	}
	join.Binding = careerTestBinding(t, s, M2AgentBoPrincipal, "regional-join")
	joined, err := s.AffiliateRPCulture(ctx, join)
	if err != nil {
		t.Fatal(err)
	}
	if joined.Fact.Affiliation.ScopePlaceID != M2AgentCafeID || joined.Fact.Affiliation.PresenceEventID == "" {
		t.Fatal("regional join lacks actual presence source")
	}
	request.Binding = careerTestBinding(t, s, "principal_creator", "world")
	request.ScopeKind = "world"
	request.ScopeID = M2DemoInstanceID
	request.PlaceIDs = nil
	world, err := s.DefineRPCultureTerritory(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	define.Binding = careerTestBinding(t, s, M2AgentBoPrincipal, "world-culture")
	define.Culture.ScopeKind = "world"
	define.Culture.ScopeID = M2DemoInstanceID
	define.Culture.CultureID = "world-custom"
	culture, err = s.DefineRPCulture(ctx, define)
	if err != nil {
		t.Fatal(err)
	}
	if culture.Fact.ScopeSourceEventID != world.EventID {
		t.Fatal("world culture source missing")
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='RPCultureFactRecorded' AND json_extract(payload,'$.kind')='internalization'`, nil, 0)
	if _, err := s.TransmitRPCulture(ctx, CultureTransmissionRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "unseen-world"), SpeakerID: M2AgentAdaID, DefinitionEventID: culture.EventID}); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("world culture became omniscient: %v", err)
	}
	if _, err := s.TransmitRPCulture(ctx, CultureTransmissionRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "world-news"), SpeakerID: M2AgentBoID, DefinitionEventID: culture.EventID}); err != nil {
		t.Fatal(err)
	}
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	if readCareerTestContext(t, s, M2AgentBoID).Life.CultureAffiliations[0].PresenceEventID != joined.Fact.Affiliation.PresenceEventID {
		t.Fatal("region presence source changed on rebuild")
	}
}
