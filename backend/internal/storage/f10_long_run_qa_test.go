package storage

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"corerp.local/backend/internal/core"
)

func TestF10RepresentativeLongRun_WorldQAAndObserver(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "long-run-f10.db")
	s := openCareerTestWorld(t, path)
	defer s.Close()

	setupTestStudioGrants(t, ctx, s, M2DemoInstanceID, M2DemoBranchID)

	// 1. Establish an organization with an active career job and posting
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

	// 2. Candidate Ada applies, interviews, evaluates, offers, and accepts
	appReq := core.CareerApplicationRequest{
		Binding:       careerTestBinding(t, s, M2AgentAdaPrincipal, "f10-ada-app"),
		ApplicationID: "application_ada_f10",
		PositionID:    posting.Fact.Posting.PositionID,
		CandidateID:   M2AgentAdaID,
		Statement:     "Qualified applicant with demonstrated experience.",
	}
	app, err := s.ApplyForCareerPosition(ctx, appReq)
	if err != nil {
		t.Fatalf("failed to apply: %v", err)
	}

	boInviteBinding := careerTestBinding(t, s, M2AgentBoPrincipal, "f10-bo-invite")
	_, err = s.InviteCareerInterview(ctx, core.CareerInterviewRequest{
		Binding:       boInviteBinding,
		InterviewID:   "interview_ada_f10",
		ApplicationID: app.Fact.RecordID,
		Question:      "Describe your skills.",
	})
	if err != nil {
		t.Fatalf("failed to invite interview: %v", err)
	}

	adaAnswerBinding := careerTestBinding(t, s, M2AgentAdaPrincipal, "f10-ada-answer")
	_, err = s.AnswerCareerInterview(ctx, core.CareerInterviewAnswerRequest{
		Binding:     adaAnswerBinding,
		InterviewID: "interview_ada_f10",
		Answer:      "I will follow all safety and work procedures.",
	})
	if err != nil {
		t.Fatalf("failed to answer interview: %v", err)
	}

	evalReq := core.CareerEvaluationRequest{
		Binding:      careerTestBinding(t, s, M2AgentBoPrincipal, "f10-bo-eval"),
		EvaluationID: "eval_ada_f10",
		InterviewID:  "interview_ada_f10",
		Decision:     "advance",
		Assessments: []core.CareerQualificationAssessment{
			{Code: "safety_training", Passed: true, Reason: "Demonstrated safety compliance"},
		},
		Reason: "Demonstrated safety compliance",
	}
	eval, err := s.EvaluateCareerApplication(ctx, evalReq)
	if err != nil {
		t.Fatalf("failed to evaluate: %v", err)
	}

	offerReq := core.CareerOfferRequest{
		Binding:       careerTestBinding(t, s, M2AgentBoPrincipal, "f10-bo-offer"),
		OfferID:       "offer_ada_f10",
		EvaluationID:  eval.Fact.RecordID,
		StartsOnDay:   1,
		ProbationDays: 5,
		ExpiresAt:     "2026-09-22T23:59:00Z",
	}
	offer, err := s.OfferCareerEmployment(ctx, offerReq)
	if err != nil {
		t.Fatalf("failed to extend offer: %v", err)
	}

	acceptReq := core.CareerOfferAcceptRequest{
		Binding:          careerTestBinding(t, s, M2AgentAdaPrincipal, "f10-ada-accept"),
		OfferID:          offer.Fact.RecordID,
		AfterWorkPlaceID: M2AgentCafeID,
	}
	_, err = s.AcceptCareerOffer(ctx, acceptReq)
	if err != nil {
		t.Fatalf("failed to accept career offer: %v", err)
	}

	// 3. Multi-day timeline simulation: advance world time across days
	var current string
	if err := s.db.QueryRowContext(ctx, `SELECT current_world_time FROM world_clocks WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&current); err != nil {
		t.Fatal(err)
	}
	t0, err := time.Parse(time.RFC3339Nano, current)
	if err != nil {
		t.Fatal(err)
	}

	// Open player session for player actions and turns
	playerSession, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{
		PrincipalID:    M2RPPlayerPrincipal,
		InstanceID:     M2DemoInstanceID,
		BranchID:       M2DemoBranchID,
		EntityID:       M2RPPlayerID,
		POV:            "second_person",
		IdempotencyKey: "session-f10-longrun",
	})
	if err != nil {
		t.Fatalf("failed to open player session: %v", err)
	}

	// Move player to Cafe if not already there, ensuring spatial diversity
	var playerPlace string
	if err := s.db.QueryRowContext(ctx, `SELECT place_id FROM agent_positions WHERE agent_id=?`, M2RPPlayerID).Scan(&playerPlace); err != nil {
		t.Fatal(err)
	}
	if playerPlace != M2AgentCafeID {
		obsMove, err := s.ObserveRPSession(ctx, core.RPSessionReadRequest{
			PrincipalID: M2RPPlayerPrincipal,
			SessionID:   playerSession.SessionID,
		})
		if err != nil {
			t.Fatal(err)
		}
		_, err = s.MoveRP(ctx, core.RPMoveRequest{
			PrincipalID:    M2RPPlayerPrincipal,
			SessionID:      playerSession.SessionID,
			FromPlaceID:    playerPlace,
			ToPlaceID:      M2AgentCafeID,
			ExpectedCursor: obsMove.ObservationCursor,
			IdempotencyKey: "move-cafe-f10",
		})
		if err != nil {
			t.Fatalf("failed to move player: %v", err)
		}
	}

	// Advance time by 24 hours to day 2
	obsWait, err := s.ObserveRPSession(ctx, core.RPSessionReadRequest{
		PrincipalID: M2RPPlayerPrincipal,
		SessionID:   playerSession.SessionID,
	})
	if err != nil {
		t.Fatal(err)
	}

	tDay2 := t0.Add(24 * time.Hour).UTC().Format(time.RFC3339)
	wait, err := s.WaitRP(ctx, core.RPWaitRequest{
		PrincipalID:     M2RPPlayerPrincipal,
		SessionID:       playerSession.SessionID,
		TargetWorldTime: tDay2,
		Budget:          100,
		ExpectedCursor:  obsWait.ObservationCursor,
		IdempotencyKey:  "wait-day2-f10",
	})
	if err != nil || wait.Status != "completed" {
		t.Fatalf("failed to advance to day 2: %+v %v", wait, err)
	}

	// 4. Verify Spatial Anti-Clustering:
	// Verify that agents are spread across multiple places
	var placeCount int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(DISTINCT to_place_id) FROM (
 SELECT to_place_id FROM agent_movements WHERE world_time <= ?
 UNION
 SELECT place_id FROM agent_places
)`, tDay2).Scan(&placeCount); err != nil {
		t.Fatal(err)
	}
	if placeCount < 2 {
		t.Fatalf("spatial clustering detected: only %d active places in use", placeCount)
	}

	// 5. Execute World QA Health Check
	qaReq := core.WorldQARequest{
		PrincipalID: "principal_test_creator",
		InstanceID:  M2DemoInstanceID,
		BranchID:    M2DemoBranchID,
	}
	qaReport, err := s.ReadWorldQA(ctx, qaReq)
	if err != nil {
		t.Fatalf("World QA failed: %v", err)
	}

	// Check 1: Economic Conservation (double-entry zero-sum must be strictly 0)
	if qaReport.Conservation.DoubleEntryBalanceZeroSum != 0 {
		t.Fatalf("economic conservation violated: non-zero sum %d", qaReport.Conservation.DoubleEntryBalanceZeroSum)
	}
	if !qaReport.Conservation.IssuanceConserved {
		t.Fatalf("issuance conservation failure")
	}

	// Check 2: Zero Knowledge Leakage
	if qaReport.Knowledge.LeakageIndicators != 0 {
		t.Fatalf("knowledge leakage detected: %d indicators", qaReport.Knowledge.LeakageIndicators)
	}

	// Check 3: Spatial Connectivity (no orphan nodes)
	if qaReport.Spatial.OrphanNodes != 0 {
		t.Fatalf("orphan spatial nodes detected: %d", qaReport.Spatial.OrphanNodes)
	}

	// Check 4: Employment & Vacancies (Ada hired into active contract)
	if qaReport.Employment.ActiveContracts < 1 {
		t.Fatalf("expected at least 1 active employment contract, got %d", qaReport.Employment.ActiveContracts)
	}

	// Check 5: Overall Status is not CRITICAL
	if qaReport.Status == core.StatusCritical {
		t.Fatalf("world QA status is CRITICAL: %+v", qaReport.Anomalies)
	}

	// 6. Execute Observer Mode Queries Across Perspectives
	// A. Macro Perspective
	macroReport, err := s.ReadWorldObserver(ctx, core.WorldObserverRequest{
		PrincipalID: "principal_test_creator",
		InstanceID:  M2DemoInstanceID,
		BranchID:    M2DemoBranchID,
		Perspective: core.PerspectiveMacro,
		Limit:       25,
	})
	if err != nil {
		t.Fatalf("observer macro failed: %v", err)
	}
	if len(macroReport.MacroEvents) == 0 {
		t.Fatalf("expected macro events")
	}
	for _, ev := range macroReport.MacroEvents {
		if ev.InspectorLink == "" {
			t.Fatalf("missing inspector link on macro event: %+v", ev)
		}
	}

	// B. Entity Perspective (Ada)
	adaReport, err := s.ReadWorldObserver(ctx, core.WorldObserverRequest{
		PrincipalID:    "principal_test_creator",
		InstanceID:     M2DemoInstanceID,
		BranchID:       M2DemoBranchID,
		Perspective:    core.PerspectiveEntity,
		TargetEntityID: M2AgentAdaID,
	})
	if err != nil {
		t.Fatalf("observer entity failed: %v", err)
	}
	if adaReport.EntityLife == nil || adaReport.EntityLife.EmployerID != org.Fact.OrganizationID {
		t.Fatalf("expected Ada to show employer %s: %+v", org.Fact.OrganizationID, adaReport.EntityLife)
	}

	// C. Organization Perspective
	orgReport, err := s.ReadWorldObserver(ctx, core.WorldObserverRequest{
		PrincipalID:    "principal_test_creator",
		InstanceID:     M2DemoInstanceID,
		BranchID:       M2DemoBranchID,
		Perspective:    core.PerspectiveOrganization,
		TargetOrgID:    org.Fact.OrganizationID,
	})
	if err != nil {
		t.Fatalf("observer organization failed: %v", err)
	}
	if orgReport.Organization == nil || orgReport.Organization.ActiveEmployees < 1 {
		t.Fatalf("expected at least 1 active employee in org: %+v", orgReport.Organization)
	}

	// D. Digest Perspective
	digestReport, err := s.ReadWorldObserver(ctx, core.WorldObserverRequest{
		PrincipalID: "principal_test_creator",
		InstanceID:  M2DemoInstanceID,
		BranchID:    M2DemoBranchID,
		Perspective: core.PerspectiveDigest,
	})
	if err != nil {
		t.Fatalf("observer digest failed: %v", err)
	}
	if digestReport.Digest == nil || digestReport.Digest.TotalEvents <= 0 {
		t.Fatalf("expected positive total events in digest")
	}
	if len(digestReport.Digest.MacroHighlights) == 0 {
		t.Fatalf("expected digest highlights")
	}
}
