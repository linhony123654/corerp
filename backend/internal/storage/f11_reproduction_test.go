package storage

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"corerp.local/backend/internal/core"
)

// TestF11TutorialReproduction verifies the exact 9-step tutorial from the manual in a clean directory:
// 1. Start / Open
// 2. Create / Select World
// 3. Play 1 Round
// 4. Use Long Narrative
// 5. Install Sample Pack
// 6. Enable MCP Test Resident
// 7. Restart (close & reopen SQLite)
// 8. Resume & Continue
// 9. Open Inspector & World QA
func TestF11TutorialReproduction(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "tutorial-reproduction.db")

	// ==========================================
	// Step 1: Start / Open
	// ==========================================
	s := openBootstrappedStore(t, ctx, dbPath)
	defer func() {
		if s != nil {
			_ = s.Close()
		}
	}()

	setup, err := s.PrepareRPTravel(ctx)
	if err != nil {
		t.Fatalf("Step 1 failed (PrepareRPTravel): %v", err)
	}

	// ==========================================
	// Step 2: Create / Select World
	// ==========================================
	_, err = s.ConfigureStudioAccessLocal(ctx, StudioAccessRequest{
		Purpose:           "create_world",
		Binding:           core.CareerBinding{PrincipalID: "principal_operator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: setup.EventSequence, IdempotencyKey: "f11-grant"},
		TargetPrincipalID: "principal_creator",
		Status:            "active",
	})
	if err != nil {
		t.Fatalf("Step 2 failed (ConfigureStudioAccessLocal): %v", err)
	}

	worldID := "tutorial-world"
	fixture := studioCreateFixture(worldID)
	fixture.NarrativePackage = f9LifeJournalBundle(t)
	created, err := s.CreateStudioWorld(ctx, fixture)
	if err != nil || created.Status != "ready" {
		t.Fatalf("Step 2 failed (CreateStudioWorld): %+v %v", created, err)
	}

	// ==========================================
	// Step 3: Play 1 Round
	// ==========================================
	session, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{
		PrincipalID:    M2RPPlayerPrincipal,
		InstanceID:     fixture.InstanceID,
		BranchID:       "br_main",
		EntityID:       created.EntityID,
		POV:            "second_person",
		IdempotencyKey: "tutorial-session-1",
	})
	if err != nil {
		t.Fatalf("Step 3 failed (OpenRPSession): %v", err)
	}

	read := core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID}
	initialObs, err := s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatalf("Step 3 failed (ObserveRPSession): %v", err)
	}

	service, err := NewRPService(s, core.DeterministicRPDecisionProvider{}, "deterministic")
	if err != nil {
		t.Fatalf("Step 3 failed (NewRPService): %v", err)
	}

	turn1, err := service.PlayRPTurn(ctx, core.RPSpeechRequest{
		PrincipalID:    read.PrincipalID,
		SessionID:      read.SessionID,
		ExpectedCursor: initialObs.ObservationCursor,
		IdempotencyKey: "tutorial-turn-1",
		Text:           "Hello world from step 3 of the reproduction tutorial.",
	})
	if err != nil || turn1.Status != "settled" {
		t.Fatalf("Step 3 failed (PlayRPTurn): %+v %v", turn1, err)
	}

	// ==========================================
	// Step 4: Use Long Narrative
	// ==========================================
	narrativeReq := RPNarrativeReadRequest{
		PrincipalID: read.PrincipalID,
		SessionID:   read.SessionID,
		TurnRunID:   turn1.TurnRunID,
	}
	journal, err := service.ReadRPNarrative(ctx, narrativeReq)
	if err != nil {
		t.Fatalf("Step 4 failed (ReadRPNarrative): %v", err)
	}
	if len(journal.View.Lines) == 0 {
		t.Fatalf("Step 4 failed: expected narrative lines, got 0")
	}
	if journal.Style.Profile.NarrativeDensity != "long" {
		t.Fatalf("Step 4 failed: expected long narrative density, got %s", journal.Style.Profile.NarrativeDensity)
	}

	// ==========================================
	// Step 5: Install Sample Pack
	// ==========================================
	retailBundle := f9RetailBundle(t)
	if err := retailBundle.Validate(); err != nil {
		t.Fatalf("Step 5 failed: retail bundle invalid: %v", err)
	}
	if retailBundle.Manifest.Kind != "content" || retailBundle.Content.RetailCareer == nil {
		t.Fatalf("Step 5 failed: missing retail career catalog in bundle")
	}

	// ==========================================
	// Step 6: Enable MCP Test Resident
	// ==========================================
	if _, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO principals(principal_id, principal_type, display_name, status)
 VALUES ('principal_mcp_resident_controller', 'service', 'MCP Controller', 'active')`); err != nil {
		t.Fatalf("Step 6 failed (insert service principal): %v", err)
	}

	var curHead int64
	if err := s.db.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id='br_main'`, fixture.InstanceID).Scan(&curHead); err != nil {
		t.Fatalf("Step 6 failed (query head): %v", err)
	}

	enrollBinding := core.CareerBinding{
		PrincipalID:    "principal_operator",
		InstanceID:     fixture.InstanceID,
		BranchID:       "br_main",
		ExpectedHead:   curHead,
		IdempotencyKey: "tutorial-enroll-mcp",
	}
	enrollReq := RPExternalControllerEnrollmentRequest{
		Binding:               enrollBinding,
		EntityID:              created.EntityID,
		ControllerPrincipalID: "principal_mcp_resident_controller",
		ControllerInstanceID:  "mcp-instance-ada",
	}
	enrollRecord, err := s.EnrollRPExternalControllerLocal(ctx, enrollReq)
	if err != nil {
		t.Fatalf("Step 6 failed (EnrollRPExternalControllerLocal): %v", err)
	}
	if enrollRecord.Fact.EntityID != created.EntityID {
		t.Fatalf("Step 6 failed: enrollment entity mismatch")
	}

	// ==========================================
	// Step 7: Restart (Close & Reopen SQLite)
	// ==========================================
	if err := s.Close(); err != nil {
		t.Fatalf("Step 7 failed (Close): %v", err)
	}
	s = nil

	// Reopen store from disk
	s2, err := Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("Step 7 failed (Open reopened store): %v", err)
	}
	defer s2.Close()

	// ==========================================
	// Step 8: Resume & Continue
	// ==========================================
	resumed, err := s2.ResumeRPSession(ctx, core.RPSessionReadRequest{
		PrincipalID: read.PrincipalID,
		SessionID:   session.SessionID,
	})
	if err != nil {
		t.Fatalf("Step 8 failed (ResumeRPSession): %v", err)
	}
	if resumed.Status != "active" {
		t.Fatalf("Step 8 failed: resumed session not active, got %s", resumed.Status)
	}

	obsAfterRestart, err := s2.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatalf("Step 8 failed (ObserveRPSession after restart): %v", err)
	}

	service2, err := NewRPService(s2, core.DeterministicRPDecisionProvider{}, "deterministic")
	if err != nil {
		t.Fatalf("Step 8 failed (NewRPService): %v", err)
	}

	turn2, err := service2.PlayRPTurn(ctx, core.RPSpeechRequest{
		PrincipalID:    read.PrincipalID,
		SessionID:      read.SessionID,
		ExpectedCursor: obsAfterRestart.ObservationCursor,
		IdempotencyKey: "tutorial-turn-2-after-restart",
		Text:           "Continuing seamless play in step 8 after process restart.",
	})
	if err != nil || turn2.Status != "settled" {
		t.Fatalf("Step 8 failed (PlayRPTurn after restart): %+v %v", turn2, err)
	}

	// ==========================================
	// Step 9: Open Inspector & World QA
	// ==========================================
	// Grant inspector read on the created world branch
	if _, err := s2.db.ExecContext(ctx, `INSERT OR IGNORE INTO capability_definitions VALUES ('world.inspector.read', 'Read event and rule evidence', 'studio-v1')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s2.db.ExecContext(ctx, `INSERT OR IGNORE INTO capability_grants(grant_id, principal_id, capability_id, instance_id, branch_id, subject_id, field_scope, status, definition_event_id)
 VALUES ('grant-tutorial-inspect', 'principal_creator', 'world.inspector.read', ?, 'br_main', 'br_main', '["event","rule"]', 'active', (SELECT event_id FROM events WHERE instance_id=? AND branch_id='br_main' LIMIT 1))`,
		fixture.InstanceID, fixture.InstanceID); err != nil {
		t.Fatal(err)
	}

	// A. World QA Health Diagnostics
	qaReq := core.WorldQARequest{
		PrincipalID: "principal_creator",
		InstanceID:  fixture.InstanceID,
		BranchID:    "br_main",
	}
	qaReport, err := s2.ReadWorldQA(ctx, qaReq)
	if err != nil {
		t.Fatalf("Step 9 failed (ReadWorldQA): %v", err)
	}
	if qaReport.Status == core.StatusCritical {
		t.Fatalf("Step 9 failed: World QA reported CRITICAL status: %+v", qaReport.Anomalies)
	}
	if qaReport.Conservation.DoubleEntryBalanceZeroSum != 0 {
		t.Fatalf("Step 9 failed: double entry balance sum violation: %d", qaReport.Conservation.DoubleEntryBalanceZeroSum)
	}
	if qaReport.Knowledge.LeakageIndicators != 0 {
		t.Fatalf("Step 9 failed: knowledge leakage detected: %d", qaReport.Knowledge.LeakageIndicators)
	}

	// B. Observer Mode (Macro Perspective)
	obsReq := core.WorldObserverRequest{
		PrincipalID: "principal_creator",
		InstanceID:  fixture.InstanceID,
		BranchID:    "br_main",
		Perspective: core.PerspectiveMacro,
	}
	obsReport, err := s2.ReadWorldObserver(ctx, obsReq)
	if err != nil {
		t.Fatalf("Step 9 failed (ReadWorldObserver): %v", err)
	}
	if len(obsReport.MacroEvents) == 0 {
		t.Fatalf("Step 9 failed: expected macro events in observer report")
	}
	for _, ev := range obsReport.MacroEvents {
		if !strings.HasPrefix(ev.InspectorLink, "/studio/inspect?") {
			t.Fatalf("Step 9 failed: invalid inspector link: %s", ev.InspectorLink)
		}
	}

	// C. Studio Inspector Event Inspection
	inspectReq := StudioEventRequest{
		PrincipalID: "principal_creator",
		InstanceID:  fixture.InstanceID,
		BranchID:    "br_main",
		EventID:     turn1.PlayerEventID,
	}
	inspectedEvent, err := s2.ReadStudioEvent(ctx, inspectReq)
	if err != nil {
		t.Fatalf("Step 9 failed (ReadStudioEvent): %v", err)
	}
	if inspectedEvent.EventID != turn1.PlayerEventID {
		t.Fatalf("Step 9 failed: inspected event ID mismatch: got %s, want %s", inspectedEvent.EventID, turn1.PlayerEventID)
	}
	if inspectedEvent.Rule.EpochID == "" || inspectedEvent.Rule.RulesetHash == "" {
		t.Fatalf("Step 9 failed: missing rule epoch provenance in inspected event: %+v", inspectedEvent.Rule)
	}
}
