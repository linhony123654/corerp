package storage

import (
	"context"
	"corerp.local/backend/internal/core"
	"path/filepath"
	"reflect"
	"testing"
)

func TestRPCultureAffiliationDoesNotForceBeliefAndRecovers(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "membership.db")
	s := openCareerTestWorld(t, path)
	defer func() { s.Close() }()
	if _, err := s.RunAgentLife(ctx, M2AgentNoonTime, 100); err != nil {
		t.Fatal(err)
	}
	definition, err := s.DefineRPCulture(ctx, CultureDefinitionRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "definition"), AuthorID: M2AgentBoID, Culture: core.RPCulture{CultureID: "group", ScopeID: "group", ScopeKind: "community", GroupIdentity: "sharing", Norms: []core.RPCultureNorm{{NormID: "gift", Action: "gift", Evaluation: 2}}}})
	if err != nil {
		t.Fatal(err)
	}
	join := CultureAffiliationRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "join"), EntityID: M2AgentAdaID, DefinitionEventID: definition.EventID, Decision: "join"}
	if _, err := s.AffiliateRPCulture(ctx, join); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("unknown group joined: %v", err)
	}
	news, err := s.TransmitRPCulture(ctx, CultureTransmissionRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "news"), SpeakerID: M2AgentBoID, DefinitionEventID: definition.EventID})
	if err != nil {
		t.Fatal(err)
	}
	join.Binding = careerTestBinding(t, s, M2AgentAdaPrincipal, "join")
	join.TransmissionEventID = news.EventID
	bad := join
	bad.Binding.PrincipalID = M2AgentBoPrincipal
	if _, err := s.AffiliateRPCulture(ctx, bad); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("forced membership: %v", err)
	}
	s.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "membership rollback") }
	if _, err := s.AffiliateRPCulture(ctx, join); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatalf("rollback: %v", err)
	}
	s.beforeCommit = nil
	if len(readCareerTestContext(t, s, M2AgentAdaID).Life.CultureAffiliations) != 0 {
		t.Fatal("rolled back membership survived")
	}
	joined, err := s.AffiliateRPCulture(ctx, join)
	if err != nil {
		t.Fatal(err)
	}
	own := readCareerTestContext(t, s, M2AgentAdaID)
	if len(own.Life.CultureAffiliations) != 1 || own.Life.CultureAffiliations[0].SourceEventID != joined.EventID {
		t.Fatal("missing sourced own membership")
	}
	if len(readCareerTestContext(t, s, M2AgentBoID).Life.CultureAffiliations) != 0 {
		t.Fatal("membership leaked to author")
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='RPCultureFactRecorded' AND json_extract(payload,'$.kind')='internalization'`, nil, 0)
	opposed, err := s.InternalizeRPCulture(ctx, CultureStanceRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "oppose"), EntityID: M2AgentAdaID, TransmissionEventID: news.EventID, Stance: "oppose"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(own.Life.CultureAffiliations, readCareerTestContext(t, s, M2AgentAdaID).Life.CultureAffiliations) {
		t.Fatal("opposition expelled member")
	}
	leave := join
	leave.Binding = careerTestBinding(t, s, M2AgentAdaPrincipal, "leave")
	leave.Decision = "leave"
	leave.TransmissionEventID = ""
	left, err := s.AffiliateRPCulture(ctx, leave)
	if err != nil {
		t.Fatal(err)
	}
	own = readCareerTestContext(t, s, M2AgentAdaID)
	if len(own.Life.CultureAffiliations) != 1 || own.Life.CultureAffiliations[0].Status != "left" || own.Life.CultureAffiliations[0].SourceEventID != left.EventID {
		t.Fatal("departure not recorded")
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_id=? AND json_extract(payload,'$.internalization.stance')='oppose'`, []any{opposed.EventID}, 1)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := s.AffiliateRPCulture(ctx, join)
	if err != nil || !retry.Replayed || retry.EventID != joined.EventID {
		t.Fatalf("historical join retry %+v %v", retry, err)
	}
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(own.Life.CultureAffiliations, readCareerTestContext(t, s, M2AgentAdaID).Life.CultureAffiliations) {
		t.Fatal("historical retry/rebuild resurrected membership")
	}
	join.Binding = careerTestBinding(t, s, M2AgentAdaPrincipal, "rejoin")
	if _, err := s.AffiliateRPCulture(ctx, join); err != nil {
		t.Fatal(err)
	}
	if readCareerTestContext(t, s, M2AgentAdaID).Life.CultureAffiliations[0].Status != "joined" {
		t.Fatal("rejoin failed")
	}
}
