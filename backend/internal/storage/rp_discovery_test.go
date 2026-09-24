package storage

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPDiscoveryOnlyUsableOwnBindingsAndContinuation(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "discovery.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Close() }()
	_, read, _ := newRPWaitTestSession(t, ctx, s)
	r := RPDiscoverRequest{PrincipalID: read.PrincipalID}
	before := requestWorldSnapshot(t, ctx, s)
	out, err := s.DiscoverRPBindings(ctx, r)
	if err != nil || out.ProtocolVersion != RPClientProtocolVersion || len(out.Bindings) != 1 || out.Bindings[0].EntityID != M2RPPlayerID || out.NextAfter != nil {
		t.Fatalf("own binding: %+v %v", out, err)
	}
	if before != requestWorldSnapshot(t, ctx, s) {
		t.Fatal("discovery wrote world/session")
	}
	for _, principal := range []string{"principal_creator", M2AgentBoPrincipal, "unknown"} {
		foreign, err := s.DiscoverRPBindings(ctx, RPDiscoverRequest{PrincipalID: principal})
		if err != nil || foreign.Bindings == nil || len(foreign.Bindings) != 0 {
			t.Fatalf("inventory leak: %+v %v", foreign, err)
		}
	}
	// Real grants in disposable fixture, not a mock discoverer. NPC identity is
	// only listed after an explicit control grant for this player exists.
	for _, entity := range []string{M2RPNPCID, M2AgentAdaID} {
		_, err := s.db.ExecContext(ctx, `INSERT INTO capability_grants(grant_id,principal_id,capability_id,instance_id,branch_id,subject_id,field_scope,status,definition_event_id)
 SELECT ?,principal_id,capability_id,instance_id,branch_id,?,'[]','active',definition_event_id FROM capability_grants WHERE grant_id='grant_m2_rp_player_control'`, "discover_"+entity, entity)
		if err != nil {
			t.Fatal(err)
		}
	}
	r.Limit = 1
	var all []RPAvailableBinding
	for i := 0; i < 4; i++ {
		page, err := s.DiscoverRPBindings(ctx, r)
		if err != nil {
			t.Fatal(err)
		}
		all = append(all, page.Bindings...)
		if page.NextAfter == nil {
			break
		}
		r.After = page.NextAfter
	}
	if len(all) != 3 {
		t.Fatalf("pagination lost grants: %+v", all)
	}
	for i, b := range all {
		if i > 0 && all[i-1].EntityID >= b.EntityID {
			t.Fatal("unstable keyset order")
		}
		if b.DisplayName == "" {
			t.Fatal("missing own name")
		}
		_, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: read.PrincipalID, InstanceID: b.InstanceID, BranchID: b.BranchID, EntityID: b.EntityID, POV: "first_person", IdempotencyKey: "discover-" + b.EntityID})
		if err != nil {
			t.Fatalf("discovery disagrees with session owner: %v", err)
		}
	}
	r = RPDiscoverRequest{PrincipalID: read.PrincipalID}
	stable, err := s.DiscoverRPBindings(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := s.DiscoverRPBindings(ctx, r)
	if err != nil || !reflect.DeepEqual(stable, recovered) {
		t.Fatalf("recovery drift: %+v %v", recovered, err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE capability_grants SET status='revoked' WHERE grant_id=?`, "discover_"+M2RPNPCID); err != nil {
		t.Fatal(err)
	}
	out, err = s.DiscoverRPBindings(ctx, r)
	if err != nil || len(out.Bindings) != 2 {
		t.Fatalf("revocation not reflected: %+v %v", out, err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE principals SET status='disabled' WHERE principal_id=?`, read.PrincipalID); err != nil {
		t.Fatal(err)
	}
	out, err = s.DiscoverRPBindings(ctx, r)
	if err != nil || len(out.Bindings) != 0 {
		t.Fatalf("disabled player leak: %+v %v", out, err)
	}
}

func TestRPDiscoveryBoundsDuplicateGrantAndInactiveBinding(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "bounds.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, read, _ := newRPWaitTestSession(t, ctx, s)
	for _, r := range []RPDiscoverRequest{
		{}, {PrincipalID: read.PrincipalID, Limit: -1}, {PrincipalID: read.PrincipalID, Limit: 51},
		{PrincipalID: read.PrincipalID, After: &RPBindingKey{EntityID: M2RPPlayerID}},
	} {
		if _, err := s.DiscoverRPBindings(ctx, r); !core.HasCode(err, core.CodeInvalidArgument) {
			t.Fatalf("bad input: %v", err)
		}
	}
	r := RPDiscoverRequest{PrincipalID: read.PrincipalID}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO capability_grants(grant_id,principal_id,capability_id,instance_id,branch_id,subject_id,field_scope,status,definition_event_id)
 SELECT 'duplicate_discovery',principal_id,capability_id,instance_id,branch_id,subject_id,field_scope,status,definition_event_id FROM capability_grants WHERE grant_id='grant_m2_rp_player_control'`); err == nil {
		t.Fatal("duplicate control grant bypassed unique scope")
	}
	out, err := s.DiscoverRPBindings(ctx, r)
	if err != nil || len(out.Bindings) != 1 {
		t.Fatalf("exact-one grant mismatch: %+v %v", out, err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE materialized_entities SET population_count=2 WHERE entity_id=?`, M2RPPlayerID); err != nil {
		t.Fatal(err)
	}
	out, err = s.DiscoverRPBindings(ctx, r)
	if err != nil || len(out.Bindings) != 0 {
		t.Fatalf("aggregate character exposed: %+v %v", out, err)
	}
}
