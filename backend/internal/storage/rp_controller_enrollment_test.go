package storage

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPExternalControllerEnrollmentIsScopedSourcedAndNotControl(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "controller-enrollment.db")
	s := openM2AgentStore(t, ctx, path, false)
	defer func() { s.Close() }()
	if _, err := s.BootstrapRPPlayDemo(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO principals(principal_id,principal_type,display_name,status) VALUES ('principal_f3_operator','operator','F3 local operator','active')`); err != nil {
		t.Fatal(err)
	}
	for _, principal := range []string{"principal_f3_service_a", "principal_f3_service_b", "principal_f3_service_c"} {
		if _, err := s.db.ExecContext(ctx, `INSERT INTO principals(principal_id,principal_type,display_name,status) VALUES (?,'service',?,'active')`, principal, principal); err != nil {
			t.Fatal(err)
		}
	}
	request := func(entity, principal, controller, key string) RPExternalControllerEnrollmentRequest {
		var head int64
		if err := s.db.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&head); err != nil {
			t.Fatal(err)
		}
		return RPExternalControllerEnrollmentRequest{
			Binding:  core.CareerBinding{PrincipalID: "principal_f3_operator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: head, IdempotencyKey: key},
			EntityID: entity, ControllerPrincipalID: principal, ControllerInstanceID: controller,
		}
	}
	firstRequest := request(M2AgentAdaID, "principal_f3_service_a", "controller-a", "f3-enroll-a")
	s.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "enrollment pre-commit failure") }
	if _, err := s.EnrollRPExternalControllerLocal(ctx, firstRequest); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatal("injected enrollment failure did not roll back", err)
	}
	s.beforeCommit = nil
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_external_controller_enrollments WHERE instance_id=? AND branch_id=?`, []any{M2DemoInstanceID, M2DemoBranchID}, 0)
	first, err := s.EnrollRPExternalControllerLocal(ctx, firstRequest)
	if err != nil || first.Replayed || first.EventID == "" || first.Fact.EntityID != M2AgentAdaID {
		t.Fatal("first external enrollment", first, err)
	}
	if replayed, err := s.EnrollRPExternalControllerLocal(ctx, firstRequest); err != nil || !replayed.Replayed || replayed.EventID != first.EventID {
		t.Fatal("exact enrollment retry", replayed, err)
	}
	// Even a misconfigured legacy raw-read grant must not turn the enrolled
	// service credential into an internal career-referral reader.
	if _, err := s.db.ExecContext(ctx, `INSERT INTO capability_grants(grant_id,principal_id,capability_id,instance_id,branch_id,subject_id,field_scope,status,definition_event_id) SELECT 'grant_f3_external_raw','principal_f3_service_a',capability_id,instance_id,branch_id,subject_id,field_scope,'active',definition_event_id FROM capability_grants WHERE grant_id='grant_m2_ada_knowledge'`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReadAgentKnowledge(ctx, core.AgentKnowledgeRead{PrincipalID: "principal_f3_service_a", CapabilityID: "world.agent.knowledge.read", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ObserverAgentID: M2AgentAdaID, Fields: []string{"subject_agent_id", "source_event_id"}}); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatal("enrolled service credential read raw identity", err)
	}
	if _, err := s.EnrollRPExternalControllerLocal(ctx, request(M2AgentAdaID, "principal_f3_service_b", "controller-b", "f3-duplicate-entity")); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatal("same Entity admitted twice", err)
	}
	if _, err := s.EnrollRPExternalControllerLocal(ctx, request(M2AgentBoID, "principal_f3_service_a", "controller-new", "f3-duplicate-principal")); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatal("same service principal admitted twice", err)
	}
	if _, err := s.EnrollRPExternalControllerLocal(ctx, request(M2AgentBoID, "principal_f3_service_b", "controller-a", "f3-duplicate-controller")); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatal("same controller instance admitted twice", err)
	}
	if _, err := s.EnrollRPExternalControllerLocal(ctx, request(M2AgentBoID, M2AgentBoPrincipal, "agent-token", "f3-agent-token")); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatal("existing internal Agent credential admitted as external", err)
	}
	second, err := s.EnrollRPExternalControllerLocal(ctx, request(M2AgentBoID, "principal_f3_service_b", "controller-b", "f3-enroll-b"))
	if err != nil || second.Fact.ControllerPrincipalID != "principal_f3_service_b" {
		t.Fatal("second external enrollment", second, err)
	}
	if _, err := s.EnrollRPExternalControllerLocal(ctx, request(M2RPPlayerID, "principal_f3_service_c", "controller-c", "f3-third")); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatal("third external resident admitted", err)
	}
	grantRPControlForTest(t, ctx, s, M2AgentAdaID)
	player, err := s.OpenRPSession(ctx, rpTestOpenRequest())
	if err != nil || player.ControlledEntityID != M2AgentAdaID {
		t.Fatal("enrollment prematurely displaced current RP controller", player, err)
	}
	if _, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: "principal_f3_service_a", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2AgentAdaID, POV: "second_person", IdempotencyKey: "f3-premature-open"}); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatal("enrollment granted control before a sourced handoff", err)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("controller enrollment projection diverged", diff, err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE rp_external_controller_enrollments SET controller_instance_id='wrong' WHERE instance_id=? AND branch_id=? AND entity_id=?`, M2DemoInstanceID, M2DemoBranchID, M2AgentAdaID); err != nil {
		t.Fatal(err)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) == 0 || diff[0].Projection != "rp_external_controller_enrollment" {
		t.Fatal("corruption not detected", diff, err)
	}
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal("rebuild enrollment", err)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("enrollment rebuild diverged", diff, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	var reopenErr error
	s, reopenErr = Open(ctx, path)
	if reopenErr != nil {
		t.Fatal(reopenErr)
	}
	if replayed, err := s.EnrollRPExternalControllerLocal(ctx, firstRequest); err != nil || !replayed.Replayed || replayed.EventID != first.EventID {
		t.Fatal("restart changed original enrollment", replayed, err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_external_controller_enrollments WHERE instance_id=? AND branch_id=?`, []any{M2DemoInstanceID, M2DemoBranchID}, 2)
	var principal string
	if err := s.db.QueryRowContext(ctx, `SELECT principal_id FROM rp_external_controller_enrollments WHERE entity_id=?`, M2AgentAdaID).Scan(&principal); err != nil || !strings.HasPrefix(principal, "principal_f3_service_") {
		t.Fatal("external principal not preserved", principal, err)
	}
}

func TestRPExternalControllerEnrollmentConcurrentSameEntityHasOneSource(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "controller-enrollment-race.db")
	first := openM2AgentStore(t, ctx, path, false)
	defer first.Close()
	for _, principal := range []string{"principal_f3_operator", "principal_f3_contender_a", "principal_f3_contender_b"} {
		kind := "service"
		if principal == "principal_f3_operator" {
			kind = "operator"
		}
		if _, err := first.db.ExecContext(ctx, `INSERT INTO principals(principal_id,principal_type,display_name,status) VALUES (?,?,?,'active')`, principal, kind, principal); err != nil {
			t.Fatal(err)
		}
	}
	second, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	var head int64
	if err := first.db.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&head); err != nil {
		t.Fatal(err)
	}
	stores := []*Store{first, second}
	start := make(chan struct{})
	results := make([]error, 2)
	var wg sync.WaitGroup
	for i := range stores {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, results[i] = stores[i].EnrollRPExternalControllerLocal(ctx, RPExternalControllerEnrollmentRequest{
				Binding:  core.CareerBinding{PrincipalID: "principal_f3_operator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: head, IdempotencyKey: []string{"contender-a", "contender-b"}[i]},
				EntityID: M2AgentAdaID, ControllerPrincipalID: []string{"principal_f3_contender_a", "principal_f3_contender_b"}[i], ControllerInstanceID: []string{"controller-a", "controller-b"}[i],
			})
		}(i)
	}
	close(start)
	wg.Wait()
	accepted, conflict := 0, 0
	for _, err := range results {
		if err == nil {
			accepted++
		} else if core.HasCode(err, core.CodeBranchConflict) {
			conflict++
		} else {
			t.Fatal("unexpected enrollment contention result", results)
		}
	}
	if accepted != 1 || conflict != 1 {
		t.Fatal("enrollment did not resolve to one accepted source", results)
	}
	assertM2Value(t, ctx, first, `SELECT COUNT(*) FROM rp_external_controller_enrollments WHERE instance_id=? AND branch_id=? AND entity_id=?`, []any{M2DemoInstanceID, M2DemoBranchID, M2AgentAdaID}, 1)
	if diff, err := first.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatal("contended enrollment did not replay", diff, err)
	}
}
