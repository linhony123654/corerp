package storage

import (
	"context"
	"corerp.local/backend/internal/core"
	"path/filepath"
	"testing"
)

func TestRPCultureTransmissionStanceRecovery(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "culture.db")
	s := openCareerTestWorld(t, path)
	defer func() { s.Close() }()
	if run, err := s.RunAgentLife(ctx, M2AgentNoonTime, 100); err != nil || run.Status != "completed" {
		t.Fatalf("actual meeting: %+v %v", run, err)
	}
	var author, authorPrincipal, listener, listenerPrincipal string
	err := s.db.QueryRow(`SELECT a.agent_id,a.principal_id,b.agent_id,b.principal_id FROM agent_profiles a JOIN agent_positions ap ON ap.agent_id=a.agent_id JOIN agent_positions bp ON bp.place_id=ap.place_id AND bp.agent_id<>ap.agent_id JOIN agent_profiles b ON b.agent_id=bp.agent_id JOIN principals pa ON pa.principal_id=a.principal_id JOIN principals pb ON pb.principal_id=b.principal_id WHERE pa.principal_type='agent' AND pb.principal_type='agent' ORDER BY a.agent_id,b.agent_id LIMIT 1`).Scan(&author, &authorPrincipal, &listener, &listenerPrincipal)
	if err != nil {
		t.Fatal(err)
	}
	request := CultureDefinitionRequest{Binding: careerTestBinding(t, s, authorPrincipal, "culture"), AuthorID: author, Culture: core.RPCulture{CultureID: "mutual_aid", ScopeKind: "community", ScopeID: "mutual_aid", GroupIdentity: "mutual aid", Values: []string{"sharing"}, Norms: []core.RPCultureNorm{{NormID: "giving", Action: "gift", Evaluation: 2}}}}
	bad := request
	bad.AuthorID = listener
	if _, err := s.DefineRPCulture(ctx, bad); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("impersonation: %v", err)
	}
	s.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "rollback") }
	if _, err := s.DefineRPCulture(ctx, request); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatalf("rollback: %v", err)
	}
	s.beforeCommit = nil
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='RPCultureFactRecorded'`, nil, 0)
	definition, err := s.DefineRPCulture(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.DefineRPCulture(ctx, request)
	if err != nil || !again.Replayed || again.EventID != definition.EventID {
		t.Fatalf("retry %+v %v", again, err)
	}
	bad = request
	bad.Culture.GroupIdentity = "changed"
	if _, err := s.DefineRPCulture(ctx, bad); !core.HasCode(err, core.CodeIdempotencyMismatch) {
		t.Fatalf("mismatch: %v", err)
	}
	// Knowing a definition ID alone cannot authorize relaying it.
	newsReq := CultureTransmissionRequest{Binding: careerTestBinding(t, s, listenerPrincipal, "unheard-relay"), SpeakerID: listener, DefinitionEventID: definition.EventID}
	if _, err := s.TransmitRPCulture(ctx, newsReq); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("unheard relay: %v", err)
	}
	newsReq.Binding = careerTestBinding(t, s, authorPrincipal, "news")
	newsReq.SpeakerID = author
	news, err := s.TransmitRPCulture(ctx, newsReq)
	if err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_knowledge WHERE observer_agent_id=? AND source_event_id=?`, []any{listener, news.EventID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE event_type='RPCultureFactRecorded' AND json_extract(payload,'$.kind')='internalization'`, nil, 0)
	stanceReq := CultureStanceRequest{Binding: careerTestBinding(t, s, listenerPrincipal, "stance"), EntityID: listener, TransmissionEventID: news.EventID, Stance: "rebel"}
	stance, err := s.InternalizeRPCulture(ctx, stanceReq)
	if err != nil {
		t.Fatal(err)
	}
	evaluation, err := core.EvaluateRPCulture(*definition.Fact.Definition, *stance.Fact.Internalization, listener, "gift")
	if err != nil || len(evaluation) != 1 || evaluation[0].Score != -2 {
		t.Fatalf("evaluation: %+v %v", evaluation, err)
	}
	// The speaker is not a fabricated listener of their own broadcast.
	absent := stanceReq
	absent.Binding = careerTestBinding(t, s, authorPrincipal, "not-listener")
	absent.EntityID = author
	if _, err := s.InternalizeRPCulture(ctx, absent); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("nonlistener: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := s.InternalizeRPCulture(ctx, stanceReq)
	if err != nil || !replay.Replayed || replay.EventID != stance.EventID || *replay.Fact.Internalization != *stance.Fact.Internalization {
		t.Fatalf("recovery: %+v %v", replay, err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM outbox o JOIN events e ON e.event_id=o.event_id WHERE e.event_type='RPCultureFactRecorded'`, nil, 0)
}
