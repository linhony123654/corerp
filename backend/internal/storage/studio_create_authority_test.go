package storage

import (
	"context"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestStudioCreateAuthoritySeparateSourcedAndRecoverable(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "create-authority.db")
	s := openBootstrappedStore(t, ctx, path)
	defer func() { s.Close() }()
	setup, err := s.PrepareRPTravel(ctx)
	if err != nil {
		t.Fatal(err)
	}
	head := setup.EventSequence
	check := func(principal string, want bool) {
		t.Helper()
		conn, err := s.db.Conn(ctx)
		if err != nil {
			t.Fatal(err)
		}
		err = authorizeStudioWorldCreation(ctx, conn, principal, M2DemoInstanceID, M2DemoBranchID)
		conn.Close()
		if (want && err != nil) || (!want && !core.HasCode(err, core.CodeUnauthorized)) {
			t.Fatal("creation authority", want, err)
		}
	}
	request := func(key, purpose, status, target string) StudioAccessRequest {
		return StudioAccessRequest{Purpose: purpose, Binding: core.CareerBinding{PrincipalID: "principal_operator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: head, IdempotencyKey: key}, TargetPrincipalID: target, Status: status}
	}
	inspect, err := s.ConfigureStudioAccessLocal(ctx, request("inspect", "", "active", "principal_creator"))
	if err != nil {
		t.Fatal(err)
	}
	head = inspect.EventSequence
	check("principal_creator", false)
	for _, target := range []string{"principal_operator", M2RPPlayerPrincipal} {
		if _, err := s.ConfigureStudioAccessLocal(ctx, request("deny-"+target, "create_world", "active", target)); !core.HasCode(err, core.CodeUnauthorized) {
			t.Fatal("noncreator create grant", err)
		}
	}
	grantRequest := request("create", "create_world", "active", "principal_creator")
	grant, err := s.ConfigureStudioAccessLocal(ctx, grantRequest)
	if err != nil {
		t.Fatal(err)
	}
	head = grant.EventSequence
	if grant.Fact.CapabilityID != "world.create" {
		t.Fatal(grant)
	}
	check("principal_creator", true)
	check("principal_operator", false)
	check(M2RPPlayerPrincipal, false)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM capability_grants WHERE principal_id='principal_creator' AND capability_id='world.inspector.read' AND status='active'`, nil, 1)
	if _, err := s.db.ExecContext(ctx, `DELETE FROM capability_grants WHERE grant_id=?`, grant.Fact.GrantID); err != nil {
		t.Fatal(err)
	}
	check("principal_creator", false)
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	check("principal_creator", true)
	if _, err := s.ConfigureStudioAccessLocal(ctx, request("revoke", "create_world", "revoked", "principal_creator")); err != nil {
		t.Fatal(err)
	}
	check("principal_creator", false)
	// Restoring a stale active projection must not override later sourced revoke.
	if _, err := s.db.ExecContext(ctx, `UPDATE capability_grants SET status='active',definition_event_id=? WHERE grant_id=?`, grant.EventID, grant.Fact.GrantID); err != nil {
		t.Fatal(err)
	}
	check("principal_creator", false)
	if retry, err := s.ConfigureStudioAccessLocal(ctx, grantRequest); err != nil || !retry.Replayed {
		t.Fatal("old exact retry", err)
	}
	check("principal_creator", false)
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	check("principal_creator", false)
}
