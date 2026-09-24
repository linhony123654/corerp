package storage

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestStudioDiscoveryTimelinePaginationAndReauthorization(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "directory.db")
	s := openBootstrappedStore(t, ctx, path)
	defer func() { s.Close() }()
	setup, err := s.BootstrapM2AgentDemo(ctx)
	if err != nil {
		t.Fatal(err)
	}
	head := setup.EventSequence
	configure := func(target, status, key string) {
		t.Helper()
		result, err := s.ConfigureStudioAccessLocal(ctx, StudioAccessRequest{Binding: core.CareerBinding{PrincipalID: "principal_operator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: head, IdempotencyKey: key}, TargetPrincipalID: target, Status: status})
		if err != nil {
			t.Fatal(err)
		}
		head = result.EventSequence
	}
	request := StudioScopeRequest{PrincipalID: "principal_creator", Limit: 1}
	empty, err := s.ListStudioScopes(ctx, request)
	if err != nil || len(empty.Scopes) != 0 || empty.Scopes == nil {
		t.Fatal("ungranted scopes disclosed", err)
	}
	configure("principal_creator", "active", "creator")
	configure("principal_operator", "active", "ops")
	beforeHead := head
	one, err := s.ListStudioScopes(ctx, request)
	if err != nil || len(one.Scopes) != 1 || one.NextAfter != nil || one.Scopes[0].InstanceID != M2DemoInstanceID || one.Scopes[0].AccessLevel != "creator" {
		t.Fatalf("wrong scope: %+v %v", one, err)
	}
	// Existing independent M1 branch supplies a second grant-shaped discovery
	// fixture, never a world clone or an unsafe local configuration write.
	if _, err := s.db.ExecContext(ctx, `INSERT INTO capability_grants(grant_id,principal_id,capability_id,instance_id,branch_id,subject_id,field_scope,status,definition_event_id) VALUES ('scope-page','principal_creator','world.inspector.read',?,?,'*','["event","rule"]','active','event_world_initialized')`, DemoInstanceID, DemoBranchID); err != nil {
		t.Fatal(err)
	}
	first, err := s.ListStudioScopes(ctx, request)
	if err != nil || first.NextAfter == nil {
		t.Fatal("missing scope cursor", err)
	}
	request.After = first.NextAfter
	second, err := s.ListStudioScopes(ctx, request)
	if err != nil || len(second.Scopes) != 1 || second.NextAfter != nil || reflect.DeepEqual(first.Scopes, second.Scopes) {
		t.Fatal("scope pagination repeated", err)
	}
	// A cursor is just ordering, never authority; revoke before the next page.
	if _, err := s.db.ExecContext(ctx, `UPDATE capability_grants SET status='revoked' WHERE principal_id='principal_creator' AND instance_id=? AND branch_id=?`, second.Scopes[0].InstanceID, second.Scopes[0].BranchID); err != nil {
		t.Fatal(err)
	}
	page, err := s.ListStudioScopes(ctx, request)
	if err != nil || len(page.Scopes) != 0 {
		t.Fatal("cursor retained revoked permission", err)
	}
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	r := StudioTimelineRequest{PrincipalID: "principal_creator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, Limit: 2}
	current, err := s.ListStudioEvents(ctx, r)
	if err != nil || len(current.Events) != 2 || current.ThroughSequence != beforeHead || current.NextBeforeSequence == 0 {
		t.Fatalf("timeline: %+v %v", current, err)
	}
	encoded, _ := json.Marshal(current)
	for _, field := range []string{`"payload"`, `"actor_id"`, `"command_id"`, `"principal_id"`} {
		if strings.Contains(string(encoded), field) {
			t.Fatal("timeline overdisclosed", field)
		}
	}
	configure("principal_operator", "revoked", "ops-revoke")
	// A newer immutable event must not sneak into the pinned older page set.
	seen := map[int64]bool{}
	r.ThroughSequence = current.ThroughSequence
	for {
		for _, e := range current.Events {
			if seen[e.Sequence] || e.Sequence > beforeHead {
				t.Fatal("duplicate/new event in anchored timeline")
			}
			seen[e.Sequence] = true
		}
		if current.NextBeforeSequence == 0 {
			break
		}
		r.BeforeSequence = current.NextBeforeSequence
		current, err = s.ListStudioEvents(ctx, r)
		if err != nil {
			t.Fatal(err)
		}
	}
	if int64(len(seen)) != beforeHead {
		t.Fatal("timeline omitted immutable events")
	}
	ops := r
	ops.PrincipalID = "principal_operator"
	if _, err := s.ListStudioEvents(ctx, ops); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatal("revoked ops timeline", err)
	}
	r.BeforeSequence = 0
	r.ThroughSequence = head + 1
	if _, err := s.ListStudioEvents(ctx, r); !core.HasCode(err, core.CodeInvalidArgument) {
		t.Fatal("future anchor accepted", err)
	}
	r.ThroughSequence = 0
	r.InstanceID = "foreign"
	if _, err := s.ListStudioEvents(ctx, r); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatal("foreign timeline", err)
	}
	if _, err := s.ListStudioScopes(ctx, StudioScopeRequest{PrincipalID: "principal_buyer"}); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatal("player discovery", err)
	}
	if _, err := s.ListStudioScopes(ctx, StudioScopeRequest{PrincipalID: "principal_creator", Limit: 51}); !core.HasCode(err, core.CodeInvalidArgument) {
		t.Fatal("unbounded page", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	r.InstanceID = M2DemoInstanceID
	r.ThroughSequence = beforeHead
	r.BeforeSequence = 0
	reopened, err := s.ListStudioEvents(ctx, r)
	if err != nil || reopened.Events[0].Sequence != beforeHead {
		t.Fatal("anchored timeline changed on reopen", err)
	}
	var afterHead int64
	if err := s.db.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&afterHead); err != nil || afterHead != head {
		t.Fatal("reads altered authority", err)
	}
}
