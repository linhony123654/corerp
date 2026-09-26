package storage

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestWorldObserver_MacroPerspective(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "observer-macro.db")
	s := openCareerTestWorld(t, path)
	defer s.Close()

	setupTestStudioGrants(t, ctx, s, M2DemoInstanceID, M2DemoBranchID)

	req := core.WorldObserverRequest{
		PrincipalID: "principal_test_creator",
		InstanceID:  M2DemoInstanceID,
		BranchID:    M2DemoBranchID,
		Perspective: core.PerspectiveMacro,
		Limit:       15,
	}

	report, err := s.ReadWorldObserver(ctx, req)
	if err != nil {
		t.Fatalf("ReadWorldObserver failed: %v", err)
	}

	if report.InstanceID != M2DemoInstanceID || report.BranchID != M2DemoBranchID {
		t.Fatalf("unexpected branch scope: %s/%s", report.InstanceID, report.BranchID)
	}
	if report.Perspective != core.PerspectiveMacro {
		t.Fatalf("unexpected perspective: %s", report.Perspective)
	}
	if report.AccessLevel != "creator" {
		t.Fatalf("expected creator access level, got %s", report.AccessLevel)
	}
	if len(report.MacroEvents) == 0 {
		t.Fatalf("expected macro events, got 0")
	}

	for _, ev := range report.MacroEvents {
		if ev.EventID == "" || ev.Sequence <= 0 || ev.EventType == "" {
			t.Fatalf("invalid macro event fields: %+v", ev)
		}
		if ev.Category == "" || ev.Headline == "" {
			t.Fatalf("missing category or headline: %+v", ev)
		}
		if !strings.HasPrefix(ev.InspectorLink, "/studio/inspect?") {
			t.Fatalf("invalid inspector link: %s", ev.InspectorLink)
		}
	}
}

func TestWorldObserver_EntityPerspective_CreatorVsObserver(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "observer-entity.db")
	s := openCareerTestWorld(t, path)
	defer s.Close()

	setupTestStudioGrants(t, ctx, s, M2DemoInstanceID, M2DemoBranchID)

	targetEntity := M2AgentAdaID

	// 1. As creator: full unredacted inspection
	creatorReq := core.WorldObserverRequest{
		PrincipalID:    "principal_test_creator",
		InstanceID:     M2DemoInstanceID,
		BranchID:       M2DemoBranchID,
		Perspective:    core.PerspectiveEntity,
		TargetEntityID: targetEntity,
	}

	creatorReport, err := s.ReadWorldObserver(ctx, creatorReq)
	if err != nil {
		t.Fatalf("creator entity observer read failed: %v", err)
	}
	if creatorReport.EntityLife == nil {
		t.Fatalf("expected entity life summary")
	}
	if creatorReport.EntityLife.EntityID != targetEntity {
		t.Fatalf("expected entity ID %s, got %s", targetEntity, creatorReport.EntityLife.EntityID)
	}
	if creatorReport.EntityLife.DisplayName == "" {
		t.Fatalf("expected display name")
	}
	if !strings.HasPrefix(creatorReport.EntityLife.InspectorLink, "/studio/inspect?") {
		t.Fatalf("invalid inspector link: %s", creatorReport.EntityLife.InspectorLink)
	}

	// 2. As resident observer (M2RPPlayerPrincipal)
	obsReq := core.WorldObserverRequest{
		PrincipalID:    M2RPPlayerPrincipal,
		InstanceID:     M2DemoInstanceID,
		BranchID:       M2DemoBranchID,
		Perspective:    core.PerspectiveEntity,
		TargetEntityID: targetEntity,
	}

	obsReport, err := s.ReadWorldObserver(ctx, obsReq)
	if err != nil {
		t.Fatalf("resident observer read failed: %v", err)
	}
	if obsReport.AccessLevel != "observer" {
		t.Fatalf("expected observer role for player principal, got %s", obsReport.AccessLevel)
	}
	if obsReport.EntityLife == nil {
		t.Fatalf("expected entity life for resident observer")
	}
	// Wage must be redacted (0) for public observer
	if obsReport.EntityLife.WageMinor != 0 {
		t.Fatalf("expected wage to be redacted for observer, got %d", obsReport.EntityLife.WageMinor)
	}
	// Decision reasons must be scrubbed for public observer
	for _, dec := range obsReport.EntityLife.RecentDecisions {
		if dec.ReasonCode != "" {
			t.Fatalf("expected decision reason to be scrubbed for observer: %+v", dec)
		}
	}
}

func TestWorldObserver_OrganizationPerspective(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "observer-org.db")
	s := openCareerTestWorld(t, path)
	defer s.Close()

	setupTestStudioGrants(t, ctx, s, M2DemoInstanceID, M2DemoBranchID)

	orgReq := careerTestOrg(t, s)
	org, err := s.DefineCareerOrganization(ctx, orgReq)
	if err != nil {
		t.Fatal(err)
	}

	targetOrg := org.Fact.OrganizationID

	// Query as creator: balance visible
	creatorReq := core.WorldObserverRequest{
		PrincipalID:    "principal_test_creator",
		InstanceID:     M2DemoInstanceID,
		BranchID:       M2DemoBranchID,
		Perspective:    core.PerspectiveOrganization,
		TargetOrgID:    targetOrg,
	}
	creatorReport, err := s.ReadWorldObserver(ctx, creatorReq)
	if err != nil {
		t.Fatalf("creator org observer read failed: %v", err)
	}
	if creatorReport.Organization == nil {
		t.Fatalf("expected organization summary")
	}
	if creatorReport.Organization.OrganizationID != targetOrg {
		t.Fatalf("unexpected org ID: %s", creatorReport.Organization.OrganizationID)
	}
	if creatorReport.Organization.BalanceMinor == nil {
		t.Fatalf("expected creator to see organization balance")
	}
	if *creatorReport.Organization.BalanceMinor != 1200 {
		t.Fatalf("expected org balance 1200, got %d", *creatorReport.Organization.BalanceMinor)
	}

	// Query as resident observer: balance redacted (nil)
	obsReq := core.WorldObserverRequest{
		PrincipalID:    M2RPPlayerPrincipal,
		InstanceID:     M2DemoInstanceID,
		BranchID:       M2DemoBranchID,
		Perspective:    core.PerspectiveOrganization,
		TargetOrgID:    targetOrg,
	}
	obsReport, err := s.ReadWorldObserver(ctx, obsReq)
	if err != nil {
		t.Fatalf("resident org observer read failed: %v", err)
	}
	if obsReport.Organization.BalanceMinor != nil {
		t.Fatalf("expected organization balance to be redacted for public observer, got %v", *obsReport.Organization.BalanceMinor)
	}
}

func TestWorldObserver_RelationshipAndDigestPerspectives(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "observer-digest.db")
	s := openCareerTestWorld(t, path)
	defer s.Close()

	setupTestStudioGrants(t, ctx, s, M2DemoInstanceID, M2DemoBranchID)

	// 1. Relationship Perspective
	relReq := core.WorldObserverRequest{
		PrincipalID: "principal_test_creator",
		InstanceID:  M2DemoInstanceID,
		BranchID:    M2DemoBranchID,
		Perspective: core.PerspectiveRelationship,
		Limit:       10,
	}
	relReport, err := s.ReadWorldObserver(ctx, relReq)
	if err != nil {
		t.Fatalf("relationship observer read failed: %v", err)
	}
	if relReport.RelationshipChanges == nil {
		t.Fatalf("expected relationship changes list")
	}
	for _, ch := range relReport.RelationshipChanges {
		if ch.ObserverEntityID == "" || ch.SubjectEntityID == "" || ch.SourceEventID == "" {
			t.Fatalf("incomplete relationship change item: %+v", ch)
		}
		if !strings.HasPrefix(ch.InspectorLink, "/studio/inspect?") {
			t.Fatalf("invalid inspector link: %s", ch.InspectorLink)
		}
	}

	// 2. Digest Perspective
	digReq := core.WorldObserverRequest{
		PrincipalID: "principal_test_creator",
		InstanceID:  M2DemoInstanceID,
		BranchID:    M2DemoBranchID,
		Perspective: core.PerspectiveDigest,
	}
	digReport, err := s.ReadWorldObserver(ctx, digReq)
	if err != nil {
		t.Fatalf("digest observer read failed: %v", err)
	}
	if digReport.Digest == nil {
		t.Fatalf("expected digest summary")
	}
	if digReport.Digest.TotalEvents <= 0 {
		t.Fatalf("expected positive total events in digest, got %d", digReport.Digest.TotalEvents)
	}
	if len(digReport.Digest.MacroHighlights) == 0 {
		t.Fatalf("expected macro highlights")
	}
	for _, kev := range digReport.Digest.KeyEvents {
		if !strings.HasPrefix(kev.InspectorLink, "/studio/inspect?") {
			t.Fatalf("invalid key event inspector link: %s", kev.InspectorLink)
		}
	}
}

func TestWorldObserver_AuthorizationEnforcement(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "observer-auth.db")
	s := openCareerTestWorld(t, path)
	defer s.Close()

	badReq := core.WorldObserverRequest{
		PrincipalID: "unauthorized_nonexistent_person",
		InstanceID:  M2DemoInstanceID,
		BranchID:    M2DemoBranchID,
		Perspective: core.PerspectiveMacro,
	}
	_, err := s.ReadWorldObserver(ctx, badReq)
	if err == nil {
		t.Fatalf("expected error for non-existent principal")
	}
	if !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("expected CodeUnauthorized, got %v", err)
	}
}
