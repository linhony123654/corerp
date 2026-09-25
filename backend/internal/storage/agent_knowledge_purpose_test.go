package storage

import (
	"context"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestAgentKnowledgeRawIDsRequireInternalPrincipalAndOwnObserver(t *testing.T) {
	ctx := context.Background()
	s := openM2AgentStore(t, ctx, filepath.Join(t.TempDir(), "knowledge-purpose.db"), false)
	defer s.Close()
	if _, err := s.RunAgentLife(ctx, M2AgentNoonTime, 4); err != nil {
		t.Fatal(err)
	}
	read := core.AgentKnowledgeRead{
		PrincipalID: M2AgentAdaPrincipal, CapabilityID: "world.agent.knowledge.read",
		InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ObserverAgentID: M2AgentAdaID,
		Fields: []string{"subject_agent_id", "source_event_id"},
	}
	internal, err := s.ReadAgentKnowledge(ctx, read)
	if err != nil || len(internal.Facts) != 1 || internal.Facts[0].SubjectAgentID != M2AgentBoID || internal.Facts[0].SourceEventID == "" {
		t.Fatal("internal referral evidence changed", internal, err)
	}
	for _, candidate := range []struct{ id, kind string }{
		{"principal_f3_external_service", "service"},
		{"principal_f3_external_player", "player"},
	} {
		if _, err := s.db.ExecContext(ctx, `INSERT INTO principals(principal_id,principal_type,display_name,status) VALUES (?,?,?,'active')`, candidate.id, candidate.kind, candidate.id); err != nil {
			t.Fatal(err)
		}
		// Simulate an accidental legacy grant. Even exact field and observer
		// scope must not make a model/controller credential a referral reader.
		if _, err := s.db.ExecContext(ctx, `INSERT INTO capability_grants(grant_id,principal_id,capability_id,instance_id,branch_id,subject_id,field_scope,status,definition_event_id) SELECT ?,?,capability_id,instance_id,branch_id,subject_id,field_scope,'active',definition_event_id FROM capability_grants WHERE grant_id='grant_m2_ada_knowledge'`, "grant_"+candidate.id, candidate.id); err != nil {
			t.Fatal(err)
		}
		attempt := read
		attempt.PrincipalID = candidate.id
		if result, err := s.ReadAgentKnowledge(ctx, attempt); !core.HasCode(err, core.CodeUnauthorized) {
			t.Fatal("external credential obtained raw IDs with legacy grant", candidate, result, err)
		}
	}
	if result, err := s.ReadAgentKnowledge(ctx, read); err != nil || len(result.Facts) != 1 || result.Facts[0].SourceEventID != internal.Facts[0].SourceEventID {
		t.Fatal("internal referral read changed after external denial", result, err)
	}
}
