package storage

import (
	"context"
	"corerp.local/backend/internal/core"
	"path/filepath"
	"testing"
)

func TestRPCultureEvolutionRequiresPropagationAndRetainsConflict(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "evolution.db")
	s := openCareerTestWorld(t, path)
	defer func() { s.Close() }()
	if _, err := s.RunAgentLife(ctx, M2AgentNoonTime, 100); err != nil {
		t.Fatal(err)
	}
	define := CultureDefinitionRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "v1"), AuthorID: M2AgentBoID, Culture: core.RPCulture{CultureID: "tradition", ScopeKind: "community", ScopeID: "tradition", GroupIdentity: "sharing", Norms: []core.RPCultureNorm{{NormID: "gift", Action: "gift", Evaluation: 2}}}}
	v1, err := s.DefineRPCulture(ctx, define)
	if err != nil {
		t.Fatal(err)
	}
	news, err := s.TransmitRPCulture(ctx, CultureTransmissionRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "news1"), SpeakerID: M2AgentBoID, DefinitionEventID: v1.EventID})
	if err != nil {
		t.Fatal(err)
	}
	stance := CultureStanceRequest{Binding: careerTestBinding(t, s, M2RPNPCPrincipal, "stance1"), EntityID: M2RPNPCID, TransmissionEventID: news.EventID, Stance: "accept"}
	if _, err := s.InternalizeRPCulture(ctx, stance); err != nil {
		t.Fatal(err)
	}
	define.Binding = careerTestBinding(t, s, M2AgentBoPrincipal, "v2")
	define.PreviousDefinitionEventID = v1.EventID
	define.Culture.Norms = []core.RPCultureNorm{{NormID: "gift", Action: "gift", Evaluation: -2}}
	define.Culture.Evolution = []string{"gift giving now discouraged"}
	v2, err := s.DefineRPCulture(ctx, define)
	if err != nil {
		t.Fatal(err)
	}
	stale := define
	stale.Binding = careerTestBinding(t, s, M2AgentBoPrincipal, "stale")
	if _, err := s.DefineRPCulture(ctx, stale); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatalf("stale revision: %v", err)
	}
	child := CultureDefinitionRequest{Binding: careerTestBinding(t, s, M2RPNPCPrincipal, "child"), AuthorID: M2RPNPCID, ParentDefinitionEventIDs: []string{v2.EventID}, Culture: core.RPCulture{CultureID: "generous-subculture", ScopeKind: "community", ScopeID: "generous-subculture", GroupIdentity: "still sharing", SubcultureOf: []string{"tradition"}, Norms: []core.RPCultureNorm{{NormID: "gift", Action: "gift", Evaluation: 2}}}}
	if _, err := s.DefineRPCulture(ctx, child); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("unseen version became subculture source: %v", err)
	}
	read := allowFixtureControl(t, ctx, s, M2AgentBoID)
	gift := func(key string, want int) {
		t.Helper()
		r := socialRequest(t, ctx, s, read, M2RPNPCID, "gift", key)
		r.AmountMinor = 1
		if _, err := s.SocialRP(ctx, r); err != nil {
			t.Fatal(err)
		}
		life := readCareerTestContext(t, s, M2RPNPCID).Life
		last := life.CultureExperiences[len(life.CultureExperiences)-1]
		if last.Evaluations[0].Score != want {
			t.Fatalf("%s: %+v", key, last)
		}
	}
	gift("unheard-revision", 2)
	news, err = s.TransmitRPCulture(ctx, CultureTransmissionRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "news2"), SpeakerID: M2AgentBoID, DefinitionEventID: v2.EventID})
	if err != nil {
		t.Fatal(err)
	}
	gift("heard-not-adopted", 2)
	stance.Binding = careerTestBinding(t, s, M2RPNPCPrincipal, "stance2")
	stance.TransmissionEventID = news.EventID
	if _, err := s.InternalizeRPCulture(ctx, stance); err != nil {
		t.Fatal(err)
	}
	gift("new-adoption", -2)
	child.Binding = careerTestBinding(t, s, M2RPNPCPrincipal, "child")
	derived, err := s.DefineRPCulture(ctx, child)
	if err != nil {
		t.Fatal(err)
	}
	if len(derived.Fact.ParentDefinitionEventIDs) != 1 || derived.Fact.ParentDefinitionEventIDs[0] != v2.EventID {
		t.Fatal("subculture lost pinned lineage")
	}
	if _, err := s.InternalizeRPCulture(ctx, CultureStanceRequest{Binding: careerTestBinding(t, s, M2RPNPCPrincipal, "own-child"), EntityID: M2RPNPCID, DefinitionEventID: derived.EventID, Stance: "accept"}); err != nil {
		t.Fatal(err)
	}
	gift("conflicting-identities", -2)
	life := readCareerTestContext(t, s, M2RPNPCID).Life
	last := life.CultureExperiences[len(life.CultureExperiences)-1]
	if len(last.Evaluations) != 2 || len(last.Conflicts) != 1 {
		t.Fatalf("opposed norms flattened %+v", last)
	}
	if life.CultureExperiences[0].Evaluations[0].Basis.VersionEventID != v1.EventID {
		t.Fatal("evolution rewrote old experience")
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
	recovered := readCareerTestContext(t, s, M2RPNPCID).Life
	if len(recovered.CultureExperiences) != 4 || len(recovered.CultureExperiences[3].Conflicts) != 1 {
		t.Fatal("evolution/conflict lost across recovery")
	}
}
