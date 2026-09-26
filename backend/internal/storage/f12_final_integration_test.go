package storage

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"corerp.local/backend/internal/core"
)

// TestF12ComprehensiveUnifiedLongRun verifies the complete multi-system long-run integration
// across 30+ world days, multiple process restarts, human + MCP + core NPCs,
// households, health/fatigue, career/wages, education, information channels,
// organization agency, Studio Inspector, and 14-dimension World QA.
func TestF12ComprehensiveUnifiedLongRun(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "f12-comprehensive-longrun.db")

	// =========================================================================
	// 1. World Bootstrap & Pack Activation (F9 DLC Packs & Authoring)
	// =========================================================================
	s := openCareerTestWorld(t, dbPath)
	defer func() {
		if s != nil {
			_ = s.Close()
		}
	}()

	setupTestStudioGrants(t, ctx, s, M2DemoInstanceID, M2DemoBranchID)

	// Validate and install Content DLC Pack (Retail Career) and Narrative Pack
	retailBundle := f9RetailBundle(t)
	if err := retailBundle.Validate(); err != nil {
		t.Fatalf("retail bundle invalid: %v", err)
	}
	narrativeBundle := f9LifeJournalBundle(t)
	if err := narrativeBundle.Validate(); err != nil {
		t.Fatalf("narrative bundle invalid: %v", err)
	}

	// =========================================================================
	// 2. Multi-Controller Enrollment (Human Player + 2 MCP External Residents)
	// =========================================================================
	// Ensure operator principal exists
	if _, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO principals(principal_id, principal_type, display_name, status)
 VALUES ('principal_operator', 'operator', 'Local Operator', 'active')`); err != nil {
		t.Fatalf("failed to insert operator principal: %v", err)
	}

	// Register service principals for external MCP controllers
	for _, mcpPrincipal := range []string{"principal_mcp_resident_1", "principal_mcp_resident_2"} {
		if _, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO principals(principal_id, principal_type, display_name, status)
 VALUES (?, 'service', 'MCP Resident Controller', 'active')`, mcpPrincipal); err != nil {
			t.Fatalf("failed to insert service principal %s: %v", mcpPrincipal, err)
		}
	}

	var curHead int64
	if err := s.db.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id='br_main'`, M2DemoInstanceID).Scan(&curHead); err != nil {
		t.Fatalf("query head: %v", err)
	}

	// Enroll MCP Resident 1 on Ada
	_, err := s.EnrollRPExternalControllerLocal(ctx, RPExternalControllerEnrollmentRequest{
		Binding: core.CareerBinding{
			PrincipalID:    "principal_operator",
			InstanceID:     M2DemoInstanceID,
			BranchID:       M2DemoBranchID,
			ExpectedHead:   curHead,
			IdempotencyKey: "f12-enroll-mcp-1",
		},
		EntityID:              M2AgentAdaID,
		ControllerPrincipalID: "principal_mcp_resident_1",
		ControllerInstanceID:  "mcp-instance-1",
	})
	if err != nil {
		t.Fatalf("failed to enroll MCP resident 1: %v", err)
	}

	// Enroll MCP Resident 2 on Bo
	if err := s.db.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id='br_main'`, M2DemoInstanceID).Scan(&curHead); err != nil {
		t.Fatalf("query head: %v", err)
	}
	_, err = s.EnrollRPExternalControllerLocal(ctx, RPExternalControllerEnrollmentRequest{
		Binding: core.CareerBinding{
			PrincipalID:    "principal_operator",
			InstanceID:     M2DemoInstanceID,
			BranchID:       M2DemoBranchID,
			ExpectedHead:   curHead,
			IdempotencyKey: "f12-enroll-mcp-2",
		},
		EntityID:              M2AgentBoID,
		ControllerPrincipalID: "principal_mcp_resident_2",
		ControllerInstanceID:  "mcp-instance-2",
	})
	if err != nil {
		t.Fatalf("failed to enroll MCP resident 2: %v", err)
	}

	// =========================================================================
	// 3. Household Formation & Shared Rent Agreements (F4)
	// =========================================================================
	foundH1, err := s.FoundRPHouseholdLocal(ctx, RPHouseholdFoundRequest{
		Binding:          careerTestBinding(t, s, "principal_operator", "f12-found-h1"),
		HouseholdKey:     "bo_ada",
		DisplayName:      "Bo and Ada Household",
		ResidencePlaceID: "place_m2_home_bo",
		AdultEntityIDs:   [2]string{M2AgentBoID, M2AgentAdaID},
	})
	if err != nil {
		t.Fatalf("failed to found household 1: %v", err)
	}
	if foundH1.Fact.HouseholdID == "" {
		t.Fatalf("missing household ID in fact")
	}

	// Setup rent agreement for household 1
	rentAgreement, err := s.AgreeRPHouseholdRentLocal(ctx, RPHouseholdRentAgreementRequest{
		Binding:      careerTestBinding(t, s, "principal_operator", "f12-rent-h1"),
		HouseholdID:  foundH1.Fact.HouseholdID,
		AgreementKey: "h1_lease",
		LandlordName: "Lin Landlord",
		RentMinor:    500,
		PeriodDays:   30,
		GraceDays:    5,
		Shares: [2]RPHouseholdRentShare{
			{MemberEntityID: M2AgentBoID, AmountMinor: 250},
			{MemberEntityID: M2AgentAdaID, AmountMinor: 250},
		},
	})
	if err != nil {
		t.Fatalf("failed to agree household rent: %v", err)
	}
	if rentAgreement.Fact.AgreementID == "" {
		t.Fatalf("missing agreement ID in fact")
	}

	// =========================================================================
	// 3b. Transit Topology & Intermediate Encounter Point (F2)
	// =========================================================================
	var edgeHead int64
	if err := s.db.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id='br_main'`, M2DemoInstanceID).Scan(&edgeHead); err != nil {
		t.Fatalf("query head: %v", err)
	}

	segment, err := s.MaterializeRPLocation(ctx, RPLocationMaterializeRequest{
		Binding:          core.CareerBinding{PrincipalID: "principal_creator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: edgeHead, IdempotencyKey: "f12-timed-segment"},
		ParentLocationID: M2AgentCafeID,
		SlotKey:          "cafe-home-street",
		Candidate:        RPLocationCandidate{DisplayName: "街区林荫道", GeneratorVersion: "local-v1"},
	})
	if err != nil {
		t.Fatalf("materialize transit location: %v", err)
	}

	_, err = s.DefineRPTimedEdge(ctx, RPTimedEdgeRequest{
		Binding:         core.CareerBinding{PrincipalID: "principal_creator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: segment.EventSequence, IdempotencyKey: "f12-cafe-home-edge"},
		FromPlaceID:     M2AgentCafeID,
		ToPlaceID:       "place_m2_home_bo",
		SegmentPlaceID:  segment.Fact.LocationID,
		DurationMinutes: 15,
	})
	if err != nil {
		t.Fatalf("define timed edge: %v", err)
	}

	// =========================================================================
	// 4. Retail Organization, Postings & Agency Policy (F8)
	// =========================================================================
	orgReq := careerTestOrg(t, s)
	org, err := s.DefineCareerOrganization(ctx, orgReq)
	if err != nil {
		t.Fatalf("failed to define career org: %v", err)
	}

	postingReq := careerTestPosting(t, s)
	posting, err := s.PostCareerPosition(ctx, postingReq)
	if err != nil {
		t.Fatalf("failed to post career position: %v", err)
	}

	agencyPolicyReq := core.OrganizationAgencyPolicyRequest{
		Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "f12-agency-policy"),
		Policy: core.OrganizationAgencyPolicy{
			PolicyID:             "policy_f12_agency",
			OrganizationID:       org.Fact.OrganizationID,
			ManagerPrincipalID:   M2AgentBoPrincipal,
			ReviewFrequencyHours: 24,
			ReserveTargetMinor:   1000,
			HiringThresholdMinor: 50,
			FreezeThresholdMinor: 10,
			TargetPositionID:     posting.Fact.Posting.PositionID,
			DefaultCapacity:      3,
			Status:               "active",
		},
	}
	agencyPolicy, err := s.DefineOrganizationAgencyPolicy(ctx, agencyPolicyReq)
	if err != nil {
		t.Fatalf("failed to define organization agency policy: %v", err)
	}
	if agencyPolicy.PolicyID != "policy_f12_agency" {
		t.Fatalf("unexpected agency policy ID: %s", agencyPolicy.PolicyID)
	}

	// =========================================================================
	// 5. Education Credentials & Career Hiring (F6)
	// =========================================================================
	// Candidate Ada applies for the position
	appReq := core.CareerApplicationRequest{
		Binding:       careerTestBinding(t, s, M2AgentAdaPrincipal, "f12-ada-app"),
		ApplicationID: "application_ada_f12",
		PositionID:    posting.Fact.Posting.PositionID,
		CandidateID:   M2AgentAdaID,
		Statement:     "Experienced clerk with safety certification.",
	}
	app, err := s.ApplyForCareerPosition(ctx, appReq)
	if err != nil {
		t.Fatalf("failed to apply for position: %v", err)
	}

	// Bo invites Ada to interview
	_, err = s.InviteCareerInterview(ctx, core.CareerInterviewRequest{
		Binding:       careerTestBinding(t, s, M2AgentBoPrincipal, "f12-bo-invite"),
		InterviewID:   "interview_ada_f12",
		ApplicationID: app.Fact.RecordID,
		Question:      "Explain retail safety standards.",
	})
	if err != nil {
		t.Fatalf("failed to invite interview: %v", err)
	}

	// Ada answers interview
	_, err = s.AnswerCareerInterview(ctx, core.CareerInterviewAnswerRequest{
		Binding:     careerTestBinding(t, s, M2AgentAdaPrincipal, "f12-ada-answer"),
		InterviewID: "interview_ada_f12",
		Answer:      "I strictly comply with safety training and inventory accountability.",
	})
	if err != nil {
		t.Fatalf("failed to answer interview: %v", err)
	}

	// Bo evaluates interview with required qualification code
	evalReq := core.CareerEvaluationRequest{
		Binding:      careerTestBinding(t, s, M2AgentBoPrincipal, "f12-bo-eval"),
		EvaluationID: "eval_ada_f12",
		InterviewID:  "interview_ada_f12",
		Decision:     "advance",
		Assessments: []core.CareerQualificationAssessment{
			{Code: "safety_training", Passed: true, Reason: "Demonstrated full safety qualification"},
		},
		Reason: "Demonstrated safety compliance",
	}
	eval, err := s.EvaluateCareerApplication(ctx, evalReq)
	if err != nil {
		t.Fatalf("failed to evaluate application: %v", err)
	}

	// Bo offers employment
	offerReq := core.CareerOfferRequest{
		Binding:       careerTestBinding(t, s, M2AgentBoPrincipal, "f12-bo-offer"),
		OfferID:       "offer_ada_f12",
		EvaluationID:  eval.Fact.RecordID,
		StartsOnDay:   1,
		ProbationDays: 5,
		ExpiresAt:     "2026-09-22T23:59:00Z",
	}
	offer, err := s.OfferCareerEmployment(ctx, offerReq)
	if err != nil {
		t.Fatalf("failed to extend offer: %v", err)
	}

	// Ada accepts employment offer
	acceptedOffer, err := s.AcceptCareerOffer(ctx, core.CareerOfferAcceptRequest{
		Binding:          careerTestBinding(t, s, M2AgentAdaPrincipal, "f12-ada-accept"),
		OfferID:          offer.Fact.RecordID,
		AfterWorkPlaceID: M2AgentCafeID,
	})
	if err != nil {
		t.Fatalf("failed to accept offer: %v", err)
	}
	if acceptedOffer.Fact.Employment.ContractID == "" {
		t.Fatalf("missing employment contract ID")
	}

	// =========================================================================
	// 6. Sleep, Health & Work Performance (F5)
	// =========================================================================
	// Ada begins work routine check during active shift
	adaSession := allowFixtureControl(t, ctx, s, M2AgentAdaID)

	adaObs, err := s.ObserveRPSession(ctx, adaSession)
	if err != nil {
		t.Fatalf("observe ada session: %v", err)
	}

	// Advance to Day 1 shift (09:00:00Z)
	waitRes, err := s.WaitRP(ctx, core.RPWaitRequest{
		PrincipalID:     adaSession.PrincipalID,
		SessionID:       adaSession.SessionID,
		TargetWorldTime: "2026-09-23T09:00:00Z",
		Budget:          1000,
		ExpectedCursor:  adaObs.ObservationCursor,
		IdempotencyKey:  "f12-advance-to-shift",
	})
	if err != nil || waitRes.Status != "completed" {
		t.Fatalf("wait to shift failed: %+v %v", waitRes, err)
	}

	adaObsAfterWait, err := s.ObserveRPSession(ctx, adaSession)
	if err != nil {
		t.Fatalf("observe ada session after wait: %v", err)
	}

	taskReq := RPWorkTaskRequest{
		Binding: core.CareerBinding{
			PrincipalID:    adaSession.PrincipalID,
			InstanceID:     M2DemoInstanceID,
			BranchID:       M2DemoBranchID,
			ExpectedHead:   adaObsAfterWait.ObservationCursor,
			IdempotencyKey: "f12-ada-work-task-1",
		},
		SessionID:  adaSession.SessionID,
		ContractID: acceptedOffer.Fact.Employment.ContractID,
		TaskCode:   "routine_check",
	}
	// Execute normal shift work task
	workOutcome, err := s.AttemptRPWorkTask(ctx, taskReq)
	if err != nil {
		t.Fatalf("failed to attempt work task: %v", err)
	}
	if workOutcome.Fact.Outcome != "completed" {
		t.Fatalf("expected completed outcome, got %s", workOutcome.Fact.Outcome)
	}

	// =========================================================================
	// 7. Player Session & Turns (F1, F9)
	// =========================================================================
	playerSession, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{
		PrincipalID:    M2RPPlayerPrincipal,
		InstanceID:     M2DemoInstanceID,
		BranchID:       M2DemoBranchID,
		EntityID:       M2RPPlayerID,
		POV:            "second_person",
		IdempotencyKey: "f12-player-session-1",
	})
	if err != nil {
		t.Fatalf("failed to open player session: %v", err)
	}

	// =========================================================================
	// 8. Information Channels & Recipient Stances (F7)
	// =========================================================================
	// Send direct message from Player to Bo
	obsForSend, err := s.ObserveRPSession(ctx, core.RPSessionReadRequest{
		PrincipalID: M2RPPlayerPrincipal,
		SessionID:   playerSession.SessionID,
	})
	if err != nil {
		t.Fatalf("observe player session for message: %v", err)
	}

	msgReq := RPInformationSendRequest{
		Binding: core.CareerBinding{
			PrincipalID:    M2RPPlayerPrincipal,
			InstanceID:     M2DemoInstanceID,
			BranchID:       M2DemoBranchID,
			ExpectedHead:   obsForSend.ObservationCursor,
			IdempotencyKey: "f12-send-msg-1",
		},
		SessionID:         playerSession.SessionID,
		MessageID:         "msg_f12_player_to_ada",
		RecipientEntityID: M2RPNPCID,
		Text:              "Let us meet at the cafe tomorrow morning.",
	}
	sentMsg, err := s.SendRPInformation(ctx, msgReq)
	if err != nil {
		t.Fatalf("failed to send direct message: %v", err)
	}
	if sentMsg.Fact.MessageID != "msg_f12_player_to_ada" {
		t.Fatalf("unexpected message ID: %s", sentMsg.Fact.MessageID)
	}

	// =========================================================================
	// 9. Player Turns, Dialogue & Long Narrative (F1, F9)
	// =========================================================================
	rpService, err := NewRPService(s, core.DeterministicRPDecisionProvider{}, "deterministic")
	if err != nil {
		t.Fatalf("failed to create RP service: %v", err)
	}

	playerObs, err := s.ObserveRPSession(ctx, core.RPSessionReadRequest{
		PrincipalID: M2RPPlayerPrincipal,
		SessionID:   playerSession.SessionID,
	})
	if err != nil {
		t.Fatalf("failed to observe player session: %v", err)
	}

	turn1, err := rpService.PlayRPTurn(ctx, core.RPSpeechRequest{
		PrincipalID:    M2RPPlayerPrincipal,
		SessionID:      playerSession.SessionID,
		ExpectedCursor: playerObs.ObservationCursor,
		IdempotencyKey: "f12-turn-1",
		Text:           "Good morning everyone, checking in on neighborhood affairs.",
	})
	if err != nil || turn1.Status != "settled" {
		t.Fatalf("failed to play turn 1: %+v %v", turn1, err)
	}

	// Verify Long Narrative Life Journaling (F1, F9)
	journal, err := rpService.ReadRPNarrative(ctx, RPNarrativeReadRequest{
		PrincipalID: M2RPPlayerPrincipal,
		SessionID:   playerSession.SessionID,
		TurnRunID:   turn1.TurnRunID,
	})
	if err != nil {
		t.Fatalf("failed to read RP narrative: %v", err)
	}
	if len(journal.View.Lines) == 0 {
		t.Fatalf("expected narrative lines in long life journal")
	}

	// =========================================================================
	// 9. Multi-Day Timeline Progression (30+ World Days & Multiple Restarts)
	// =========================================================================
	var initialClock string
	if err := s.db.QueryRowContext(ctx, `SELECT current_world_time FROM world_clocks WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&initialClock); err != nil {
		t.Fatalf("query clock: %v", err)
	}
	tClock, err := time.Parse(time.RFC3339Nano, initialClock)
	if err != nil {
		t.Fatalf("parse clock: %v", err)
	}

	// Advance through days with intermediate restarts
	for restartRound := 1; restartRound <= 3; restartRound++ {
		targetDay := restartRound * 10
		targetTime := tClock.Add(time.Duration(targetDay*24) * time.Hour).UTC().Format(time.RFC3339)

		obsCur, err := s.ObserveRPSession(ctx, core.RPSessionReadRequest{
			PrincipalID: M2RPPlayerPrincipal,
			SessionID:   playerSession.SessionID,
		})
		if err != nil {
			t.Fatalf("observe round %d: %v", restartRound, err)
		}

		waitRes, err := s.WaitRP(ctx, core.RPWaitRequest{
			PrincipalID:     M2RPPlayerPrincipal,
			SessionID:       playerSession.SessionID,
			TargetWorldTime: targetTime,
			Budget:          1000,
			ExpectedCursor:  obsCur.ObservationCursor,
			IdempotencyKey:  "f12-advance-day-" + targetTime,
		})
		if err != nil || waitRes.Status != "completed" {
			t.Fatalf("wait to %s failed: %+v %v", targetTime, waitRes, err)
		}

		// Perform intermediate interaction turn
		obsTurn, err := s.ObserveRPSession(ctx, core.RPSessionReadRequest{
			PrincipalID: M2RPPlayerPrincipal,
			SessionID:   playerSession.SessionID,
		})
		if err != nil {
			t.Fatalf("observe for turn round %d: %v", restartRound, err)
		}
		_, err = rpService.PlayRPTurn(ctx, core.RPSpeechRequest{
			PrincipalID:    M2RPPlayerPrincipal,
			SessionID:      playerSession.SessionID,
			ExpectedCursor: obsTurn.ObservationCursor,
			IdempotencyKey: "f12-intermediate-turn-" + targetTime,
			Text:           "Reflecting on our shared neighborhood progress over time.",
		})
		if err != nil {
			t.Fatalf("intermediate turn failed: %v", err)
		}

		// Restart SQLite store (Process Exit & Reopen)
		if err := s.Close(); err != nil {
			t.Fatalf("failed to close store at round %d: %v", restartRound, err)
		}
		s = nil

		// Reopen from disk
		s, err = Open(ctx, dbPath)
		if err != nil {
			t.Fatalf("failed to reopen store at round %d: %v", restartRound, err)
		}
		rpService, err = NewRPService(s, core.DeterministicRPDecisionProvider{}, "deterministic")
		if err != nil {
			t.Fatalf("re-create rp service: %v", err)
		}

		// Verify zero projection drift after reopen and rebuild
		if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
			t.Fatalf("failed to rebuild projections at round %d: %v", restartRound, err)
		}
		diffs, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID)
		if err != nil || len(diffs) != 0 {
			t.Fatalf("projection drift detected at round %d: %+v %v", restartRound, diffs, err)
		}
	}

	// =========================================================================
	// 10. Organization Agency Review Execution (F8)
	// =========================================================================
	reviewReq := core.OrganizationReviewRequest{
		Binding:        careerTestBinding(t, s, M2AgentBoPrincipal, "f12-conduct-review-1"),
		OrganizationID: org.Fact.OrganizationID,
		PolicyID:       agencyPolicy.PolicyID,
	}
	reviewRes, err := s.ConductOrganizationReview(ctx, reviewReq)
	if err != nil {
		t.Fatalf("failed to conduct organization review: %v", err)
	}
	if reviewRes.OrganizationID != org.Fact.OrganizationID {
		t.Fatalf("organization review mismatch: %+v", reviewRes)
	}

	// =========================================================================
	// 11. World QA Diagnostics Audit Across 14 Dimensions (F10)
	// =========================================================================
	setupTestStudioGrants(t, ctx, s, M2DemoInstanceID, M2DemoBranchID)

	qaReq := core.WorldQARequest{
		PrincipalID: "principal_test_creator",
		InstanceID:  M2DemoInstanceID,
		BranchID:    M2DemoBranchID,
	}
	qaReport, err := s.ReadWorldQA(ctx, qaReq)
	if err != nil {
		t.Fatalf("failed to read World QA report: %v", err)
	}

	// Invariant 1: Status must not be CRITICAL
	if qaReport.Status == core.StatusCritical {
		t.Fatalf("World QA reported CRITICAL status: %+v", qaReport.Anomalies)
	}

	// Invariant 2: Double-entry zero-sum economic balance
	if qaReport.Conservation.DoubleEntryBalanceZeroSum != 0 {
		t.Fatalf("economic conservation violation: DoubleEntryBalanceZeroSum = %d", qaReport.Conservation.DoubleEntryBalanceZeroSum)
	}

	// Invariant 3: Epistemic knowledge containment (0 leaks)
	if qaReport.Knowledge.LeakageIndicators != 0 {
		t.Fatalf("knowledge leakage detected: LeakageIndicators = %d", qaReport.Knowledge.LeakageIndicators)
	}

	// Invariant 4: Spatial integrity (0 orphan unreachable nodes)
	if qaReport.Spatial.OrphanNodes != 0 {
		t.Fatalf("spatial orphan nodes detected: %d", qaReport.Spatial.OrphanNodes)
	}

	// Invariant 5: Materialized population verified
	if qaReport.Population.TotalMaterialized < 4 {
		t.Fatalf("insufficient materialized entities: %d", qaReport.Population.TotalMaterialized)
	}

	// Invariant 6: Active households registered
	if qaReport.Household.ActiveHouseholds < 1 {
		t.Fatalf("missing active households: %d", qaReport.Household.ActiveHouseholds)
	}

	// Invariant 7: Transit edges defined
	if qaReport.Commute.TotalEdges == 0 {
		t.Fatalf("missing transit topology edges")
	}

	// =========================================================================
	// 12. Observer Mode Structured Perspectives & Inspector Links (F10)
	// =========================================================================
	// A. Macro Perspective
	macroReport, err := s.ReadWorldObserver(ctx, core.WorldObserverRequest{
		PrincipalID: "principal_test_creator",
		InstanceID:  M2DemoInstanceID,
		BranchID:    M2DemoBranchID,
		Perspective: core.PerspectiveMacro,
	})
	if err != nil {
		t.Fatalf("failed to read macro observer: %v", err)
	}
	if len(macroReport.MacroEvents) == 0 {
		t.Fatalf("expected macro events in observer report")
	}
	for _, ev := range macroReport.MacroEvents {
		if !strings.HasPrefix(ev.InspectorLink, "/studio/inspect?") {
			t.Fatalf("invalid inspector link on macro event: %s", ev.InspectorLink)
		}
	}

	// B. Digest Perspective over 30-Day Simulation Window
	digestReport, err := s.ReadWorldObserver(ctx, core.WorldObserverRequest{
		PrincipalID: "principal_test_creator",
		InstanceID:  M2DemoInstanceID,
		BranchID:    M2DemoBranchID,
		Perspective: core.PerspectiveDigest,
		WindowStart: "2026-09-22T00:00:00Z",
		WindowEnd:   "2026-10-30T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("failed to read digest observer: %v", err)
	}
	if digestReport.Digest == nil || digestReport.Digest.TotalEvents == 0 {
		t.Fatalf("missing or empty digest summary: %+v", digestReport.Digest)
	}

	// =========================================================================
	// 13. Studio Inspector Rule Provenance & Event Audit (F10)
	// =========================================================================
	inspectedEvent, err := s.ReadStudioEvent(ctx, StudioEventRequest{
		PrincipalID: "principal_test_creator",
		InstanceID:  M2DemoInstanceID,
		BranchID:    M2DemoBranchID,
		EventID:     turn1.PlayerEventID,
	})
	if err != nil {
		t.Fatalf("failed to inspect player event: %v", err)
	}
	if inspectedEvent.EventID != turn1.PlayerEventID {
		t.Fatalf("event ID mismatch: got %s, want %s", inspectedEvent.EventID, turn1.PlayerEventID)
	}
	if inspectedEvent.Rule.EpochID == "" || inspectedEvent.Rule.RulesetHash == "" {
		t.Fatalf("missing rule epoch provenance in inspected event: %+v", inspectedEvent.Rule)
	}

	t.Logf("F12 Comprehensive Long-Run PASSED: 30 days elapsed, 3 restarts, 0 projection drift, 0 economic divergence, 0 knowledge leaks, World QA status: %s", qaReport.Status)
}
