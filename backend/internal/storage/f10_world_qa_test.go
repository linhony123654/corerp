package storage

import (
	"context"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
)

func setupTestStudioGrants(t *testing.T, ctx context.Context, s *Store, instanceID, branchID string) {
	t.Helper()
	for _, role := range []struct {
		id   string
		kind string
		cap  string
	}{
		{"principal_test_creator", "creator", "world.inspector.read"},
		{"principal_test_operator", "operator", "diagnostics.inspector.read"},
	} {
		if _, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO principals(principal_id, principal_type, display_name, status)
 VALUES (?, ?, ?, 'active')`, role.id, role.kind, role.id); err != nil {
			t.Fatal(err)
		}
		if _, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO capability_definitions VALUES (?, 'Inspector read', 'studio-v1')`, role.cap); err != nil {
			t.Fatal(err)
		}
		if _, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO capability_grants(grant_id, principal_id, capability_id, instance_id, branch_id, subject_id, field_scope, status, definition_event_id)
 VALUES (?, ?, ?, ?, ?, ?, '["event","rule"]', 'active', (SELECT event_id FROM events WHERE instance_id=? AND branch_id=? LIMIT 1))`,
			"grant-"+role.id, role.id, role.cap, instanceID, branchID, branchID, instanceID, branchID); err != nil {
			t.Fatal(err)
		}
	}
}

func TestWorldQA_FourteenDimensionsAndStatus(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "world-qa-14.db")
	s := openCareerTestWorld(t, path)
	defer s.Close()

	setupTestStudioGrants(t, ctx, s, M2DemoInstanceID, M2DemoBranchID)

	req := core.WorldQARequest{
		PrincipalID: "principal_test_creator",
		InstanceID:  M2DemoInstanceID,
		BranchID:    M2DemoBranchID,
	}

	report, err := s.ReadWorldQA(ctx, req)
	if err != nil {
		t.Fatalf("ReadWorldQA failed: %v", err)
	}

	// Scope and metadata
	if report.InstanceID != M2DemoInstanceID || report.BranchID != M2DemoBranchID {
		t.Fatalf("unexpected scope: %s/%s", report.InstanceID, report.BranchID)
	}
	if report.AccessLevel != "creator" {
		t.Fatalf("expected creator access level, got %s", report.AccessLevel)
	}
	if report.HeadSequence <= 0 {
		t.Fatalf("expected positive head sequence, got %d", report.HeadSequence)
	}
	if report.WorldTime == "" {
		t.Fatalf("expected world time to be populated")
	}

	// Dimension 1: Population & Cohorts
	if report.Population.TotalMaterialized <= 0 {
		t.Fatalf("expected materialized entities, got %d", report.Population.TotalMaterialized)
	}
	if report.Population.ActiveAgents <= 0 {
		t.Fatalf("expected active agents, got %d", report.Population.ActiveAgents)
	}
	if len(report.Population.Cohorts) == 0 {
		t.Fatalf("expected cohorts breakdown, got empty")
	}

	// Dimension 2: Employment & Vacancies
	if report.Employment.TotalPositions < 0 {
		t.Fatalf("invalid total positions: %d", report.Employment.TotalPositions)
	}

	// Dimension 3: Finances & Arrears
	if report.Finances.TotalAccounts <= 0 {
		t.Fatalf("expected accounts, got %d", report.Finances.TotalAccounts)
	}
	if len(report.Finances.BalancesByCurrency) == 0 {
		t.Fatalf("expected currency balances, got none")
	}

	// Dimension 4: Household Pressure
	// In M2 demo, households may be 0 until configured
	if report.Household.ActiveHouseholds < 0 {
		t.Fatalf("invalid active households count: %d", report.Household.ActiveHouseholds)
	}

	// Dimension 5: Housing Coverage
	if report.Housing.HousingCoverageRatio < 0 || report.Housing.HousingCoverageRatio > 1.0 {
		t.Fatalf("housing coverage ratio out of bounds: %f", report.Housing.HousingCoverageRatio)
	}

	// Dimension 6: Commute
	if report.Commute.DelayedJourneys != 0 {
		t.Fatalf("unexpected delayed journeys on fresh demo: %d", report.Commute.DelayedJourneys)
	}

	// Dimension 7: Relationships
	if report.Relationship.GraphDensity < 0 || report.Relationship.GraphDensity > 1.0 {
		t.Fatalf("graph density out of bounds: %f", report.Relationship.GraphDensity)
	}

	// Dimension 8: Event Frequency
	if report.Events.TotalEvents <= 0 {
		t.Fatalf("expected positive total events, got %d", report.Events.TotalEvents)
	}
	if len(report.Events.EventsByType) == 0 {
		t.Fatalf("expected events by type breakdown")
	}
	if len(report.Events.TopEventTypes) == 0 {
		t.Fatalf("expected top event types")
	}
	if report.Events.EventsPerDay <= 0 {
		t.Fatalf("expected positive events per day")
	}

	// Dimension 9: Decisions
	if report.Decisions.TotalDecisions < 0 {
		t.Fatalf("invalid total decisions: %d", report.Decisions.TotalDecisions)
	}

	// Dimension 10: Knowledge Containment
	if report.Knowledge.LeakageIndicators != 0 {
		t.Fatalf("knowledge leakage detected on clean demo: %d", report.Knowledge.LeakageIndicators)
	}

	// Dimension 11: Organization Decisions
	if report.Organization.ActivePolicies < 0 {
		t.Fatalf("invalid active policies: %d", report.Organization.ActivePolicies)
	}

	// Dimension 12: Failed Actions
	if report.FailedActions.RejectedCommands != 0 {
		t.Fatalf("unexpected rejected commands on clean demo: %d", report.FailedActions.RejectedCommands)
	}

	// Dimension 13: Spatial Reachability
	if report.Spatial.TotalNodes <= 0 {
		t.Fatalf("expected positive spatial nodes, got %d", report.Spatial.TotalNodes)
	}
	if report.Spatial.RootPlaces <= 0 {
		t.Fatalf("expected positive root places, got %d", report.Spatial.RootPlaces)
	}
	if report.Spatial.OrphanNodes != 0 {
		t.Fatalf("unexpected orphan spatial nodes: %d", report.Spatial.OrphanNodes)
	}

	// Dimension 14: Economic Conservation
	if report.Conservation.DoubleEntryBalanceZeroSum != 0 {
		t.Fatalf("double-entry conservation violated: non-zero sum %d", report.Conservation.DoubleEntryBalanceZeroSum)
	}
	if !report.Conservation.IssuanceConserved {
		t.Fatalf("issuance conservation should be true")
	}
	if report.Conservation.BalancedPostingsCount <= 0 {
		t.Fatalf("expected balanced postings, got %d", report.Conservation.BalancedPostingsCount)
	}

	// Status should be healthy (no critical anomalies)
	for _, a := range report.Anomalies {
		if a.Severity == core.SeverityCritical {
			t.Fatalf("unexpected critical anomaly on clean demo: %+v", a)
		}
	}
}

func TestWorldQA_ThresholdsAndAnomalyTriggers(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "world-qa-anomalies.db")
	s := openCareerTestWorld(t, path)
	defer s.Close()

	setupTestStudioGrants(t, ctx, s, M2DemoInstanceID, M2DemoBranchID)

	// Set ultra-strict thresholds to verify warning triggers
	strictThresholds := core.WorldQAThresholds{
		MaxIsolatedEntityRatio:  0.0001, // Stricter than actual isolated ratio
		MinHousingCoverageRatio: 0.9999, // Stricter than actual coverage
		MaxPastDueRentRatio:     0.0001,
		MaxUnemploymentRatio:    0.0001,
		MaxStagnantVacancies:    0,
	}

	req := core.WorldQARequest{
		PrincipalID: "principal_test_creator",
		InstanceID:  M2DemoInstanceID,
		BranchID:    M2DemoBranchID,
		Thresholds:  &strictThresholds,
	}

	report, err := s.ReadWorldQA(ctx, req)
	if err != nil {
		t.Fatalf("ReadWorldQA failed: %v", err)
	}

	if len(report.Anomalies) == 0 {
		t.Fatalf("expected anomalies with strict thresholds, got 0")
	}

	foundWarning := false
	for _, a := range report.Anomalies {
		if a.Severity == core.SeverityWarning {
			foundWarning = true
			break
		}
	}
	if !foundWarning {
		t.Fatalf("expected at least one warning anomaly, got: %+v", report.Anomalies)
	}

	if report.Status != core.StatusWarning && report.Status != core.StatusCritical {
		t.Fatalf("expected status WARNING or CRITICAL, got %s", report.Status)
	}
}

func TestWorldQA_AuthorizationEnforcement(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "world-qa-auth.db")
	s := openCareerTestWorld(t, path)
	defer s.Close()

	setupTestStudioGrants(t, ctx, s, M2DemoInstanceID, M2DemoBranchID)

	// Operator principal with diagnostic grant
	opReq := core.WorldQARequest{
		PrincipalID: "principal_test_operator",
		InstanceID:  M2DemoInstanceID,
		BranchID:    M2DemoBranchID,
	}
	opReport, err := s.ReadWorldQA(ctx, opReq)
	if err != nil {
		t.Fatalf("operator ReadWorldQA failed: %v", err)
	}
	if opReport.AccessLevel != "operator" {
		t.Fatalf("expected operator access level, got %s", opReport.AccessLevel)
	}

	// Unauthorized random agent principal
	badReq := core.WorldQARequest{
		PrincipalID: "unauthorized_stranger",
		InstanceID:  M2DemoInstanceID,
		BranchID:    M2DemoBranchID,
	}
	_, err = s.ReadWorldQA(ctx, badReq)
	if err == nil {
		t.Fatalf("expected unauthorized error for non-creator/operator principal")
	}
	if !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("expected CodeUnauthorized, got %v", err)
	}
}
