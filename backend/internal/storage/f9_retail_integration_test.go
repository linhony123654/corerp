package storage

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"corerp.local/backend/internal/core"
)

func TestF9RetailCareerRealDomainIntegration(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "retail-career-integration.db")
	s := openCareerTestWorld(t, path)
	defer func() { s.Close() }()

	// 1. Load the authored Retail Career Pack
	bundle := f9RetailBundle(t)
	if bundle.Content.RetailCareer == nil || len(bundle.Content.RetailCareer.Jobs) == 0 {
		t.Fatal("authored retail career catalog missing jobs")
	}
	catalog := bundle.Content.RetailCareer
	job := catalog.Jobs[0]

	// 2. Define organization actor_m2_coop_employer
	orgReq := careerTestOrg(t, s)
	orgReq.Organization.OrganizationID = catalog.OrganizationID
	_, err := s.DefineCareerOrganization(ctx, orgReq)
	if err != nil {
		t.Fatal(err)
	}

	// 3. Training & Qualification: Operator registers the declared training program
	for _, principal := range []struct{ id, kind string }{
		{"principal_f6_operator", "operator"},
		{catalog.OrganizationID, "operator"},
	} {
		if _, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO principals(principal_id,principal_type,display_name,status) VALUES (?,?,?,'active')`, principal.id, principal.kind, principal.id); err != nil {
			t.Fatal(err)
		}
	}
	progBinding := careerTestBinding(t, s, "principal_f6_operator", "f9-retail-program")
	progReq := EducationProgramRequest{
		Binding:        progBinding,
		ProgramID:      job.Training.ProgramID,
		Code:           job.RequiredCredential.Code,
		IssuerID:       job.RequiredCredential.IssuerID,
		MinimumMinutes: job.Training.MinimumMinutes,
		ExercisePrompt: job.Training.ExercisePrompt,
	}
	progRecord, err := s.DefineEducationProgramLocal(ctx, progReq)
	if err != nil {
		t.Fatalf("failed to define retail education program: %v", err)
	}

	// 4. Candidate Ada enrolls in the program
	adaEnrollBinding := careerTestBinding(t, s, M2AgentAdaPrincipal, "f9-ada-enroll")
	enrollRecord, err := s.EnrollEducation(ctx, EducationEnrollmentRequest{
		Binding:      adaEnrollBinding,
		EnrollmentID: "enrollment_ada_retail",
		ProgramID:    progRecord.Fact.ProgramID,
		LearnerID:    M2AgentAdaID,
	})
	if err != nil {
		t.Fatalf("ada enrollment failed: %v", err)
	}

	// Advance world time past the training interval (60 minutes)
	var current string
	if err := s.db.QueryRowContext(ctx, `SELECT current_world_time FROM world_clocks WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&current); err != nil {
		t.Fatal(err)
	}
	start, err := time.Parse(time.RFC3339Nano, current)
	if err != nil {
		t.Fatal(err)
	}
	target := start.Add(2 * time.Hour).UTC().Format(time.RFC3339)
	session, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{
		PrincipalID:    M2RPPlayerPrincipal,
		InstanceID:     M2DemoInstanceID,
		BranchID:       M2DemoBranchID,
		EntityID:       M2RPPlayerID,
		POV:            "second_person",
		IdempotencyKey: "f9-wait-session",
	})
	if err != nil {
		t.Fatal(err)
	}
	obs, err := s.ObserveRPSession(ctx, core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID})
	if err != nil {
		t.Fatal(err)
	}
	wait, err := s.WaitRP(ctx, core.RPWaitRequest{
		PrincipalID:     M2RPPlayerPrincipal,
		SessionID:       session.SessionID,
		TargetWorldTime: target,
		Budget:          100,
		ExpectedCursor:  obs.ObservationCursor,
		IdempotencyKey:  "f9-advance-training",
	})
	if err != nil || wait.Status != "completed" {
		t.Fatalf("wait failed: %+v %v", wait, err)
	}

	// Candidate Ada submits exercise answering the Pack's exercise prompt
	adaExerciseBinding := careerTestBinding(t, s, M2AgentAdaPrincipal, "f9-ada-exercise")
	_, err = s.SubmitEducationExercise(ctx, EducationExerciseRequest{
		Binding:      adaExerciseBinding,
		EnrollmentID: enrollRecord.Fact.EnrollmentID,
		Answer:       "I will greet the customer politely, locate the requested merchandise, and record stock delta.",
	})
	if err != nil {
		t.Fatalf("ada exercise failed: %v", err)
	}

	// Issuer (organization coop employer) completes training and issues credential
	orgCompleteBinding := careerTestBinding(t, s, catalog.OrganizationID, "f9-org-complete")
	_, err = s.CompleteEducationTraining(ctx, EducationCompletionRequest{
		Binding:      orgCompleteBinding,
		EnrollmentID: enrollRecord.Fact.EnrollmentID,
		Proficiency:  "competent",
		Reason:       "Demonstrated accurate retail customer service understanding.",
	})
	if err != nil {
		t.Fatalf("org completion failed: %v", err)
	}

	orgIssueBinding := careerTestBinding(t, s, catalog.OrganizationID, "f9-org-issue")
	credRecord, err := s.IssueEducationCredential(ctx, EducationCredentialRequest{
		Binding:      orgIssueBinding,
		CredentialID: "cred_ada_retail_service",
		EnrollmentID: enrollRecord.Fact.EnrollmentID,
		ExpiresAt:    start.Add(365 * 24 * time.Hour).UTC().Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("org credential issuance failed: %v", err)
	}
	if credRecord.Fact.Code != job.RequiredCredential.Code {
		t.Fatalf("credential code mismatch: got %s, want %s", credRecord.Fact.Code, job.RequiredCredential.Code)
	}

	// 5. Job Posting & Schedule: Manager Bo posts the retail position declared in the pack
	boPostingBinding := careerTestBinding(t, s, M2AgentBoPrincipal, "f9-bo-post")
	postingRecord, err := s.PostCareerPosition(ctx, core.CareerPostingRequest{
		Binding: boPostingBinding,
		Posting: core.CareerPostingDefinition{
			PositionID:             job.PositionID,
			OrganizationID:         catalog.OrganizationID,
			Title:                  job.Title,
			OccupationID:           job.OccupationID,
			Grade:                  job.Grade,
			Capacity:               1,
			DailyWageMinor:         job.WageReferenceMinor,
			RequiredQualifications: []string{job.RequiredCredential.Code},
			RequiredCredentials:    []core.CredentialRequirement{job.RequiredCredential},
		},
	})
	if err != nil {
		t.Fatalf("bo posting failed: %v", err)
	}
	if postingRecord.Fact.Posting.DailyWageMinor != job.WageReferenceMinor {
		t.Fatalf("wage reference not used: got %d, want %d", postingRecord.Fact.Posting.DailyWageMinor, job.WageReferenceMinor)
	}

	// 6. Application, Interview, Evaluation, Offer, Acceptance
	adaApplyBinding := careerTestBinding(t, s, M2AgentAdaPrincipal, "f9-ada-apply")
	appRecord, err := s.ApplyForCareerPosition(ctx, core.CareerApplicationRequest{
		Binding:       adaApplyBinding,
		ApplicationID: "app_ada_retail",
		PositionID:    job.PositionID,
		CandidateID:   M2AgentAdaID,
		Statement:     "I hold the retail customer service qualification and am ready to work.",
	})
	if err != nil {
		t.Fatalf("ada application failed: %v", err)
	}

	// Bo invites Ada to interview
	boInviteBinding := careerTestBinding(t, s, M2AgentBoPrincipal, "f9-bo-invite")
	_, err = s.InviteCareerInterview(ctx, core.CareerInterviewRequest{
		Binding:       boInviteBinding,
		InterviewID:   "interview_ada_retail",
		ApplicationID: appRecord.Fact.RecordID,
		Question:      "How would you handle a customer needing stock lookup?",
	})
	if err != nil {
		t.Fatalf("bo invite failed: %v", err)
	}

	// Ada answers interview
	adaAnswerBinding := careerTestBinding(t, s, M2AgentAdaPrincipal, "f9-ada-answer")
	_, err = s.AnswerCareerInterview(ctx, core.CareerInterviewAnswerRequest{
		Binding:     adaAnswerBinding,
		InterviewID: "interview_ada_retail",
		Answer:      "I will query the inventory ledger and assist the customer immediately.",
	})
	if err != nil {
		t.Fatalf("ada answer failed: %v", err)
	}

	// Bo evaluates application
	boEvalBinding := careerTestBinding(t, s, M2AgentBoPrincipal, "f9-bo-eval")
	evalRecord, err := s.EvaluateCareerApplication(ctx, core.CareerEvaluationRequest{
		Binding:      boEvalBinding,
		EvaluationID: "eval_ada_retail",
		InterviewID:  "interview_ada_retail",
		Decision:     "advance",
		Assessments: []core.CareerQualificationAssessment{
			{Code: job.RequiredCredential.Code, Passed: true, Reason: "Holds verified customer service qualification."},
		},
		Reason: "Demonstrated qualification and prompt customer care.",
	})
	if err != nil {
		t.Fatalf("bo evaluation failed: %v", err)
	}

	// Bo offers employment
	boOfferBinding := careerTestBinding(t, s, M2AgentBoPrincipal, "f9-bo-offer")
	offerRecord, err := s.OfferCareerEmployment(ctx, core.CareerOfferRequest{
		Binding:       boOfferBinding,
		OfferID:       "offer_ada_retail",
		EvaluationID:  evalRecord.Fact.RecordID,
		StartsOnDay:   1,
		ProbationDays: 7,
		ExpiresAt:     "2026-09-22T23:59:00Z",
	})
	if err != nil {
		t.Fatalf("bo offer failed: %v", err)
	}

	// Ada accepts offer
	adaAcceptBinding := careerTestBinding(t, s, M2AgentAdaPrincipal, "f9-ada-accept")
	empRecord, err := s.AcceptCareerOffer(ctx, core.CareerOfferAcceptRequest{
		Binding:          adaAcceptBinding,
		OfferID:          offerRecord.Fact.RecordID,
		AfterWorkPlaceID: M2AgentCafeID,
	})
	if err != nil {
		t.Fatalf("ada acceptance failed: %v", err)
	}
	if empRecord.Fact.Employment == nil || empRecord.Fact.Employment.ContractID == "" {
		t.Fatalf("employment contract missing: %+v", empRecord.Fact)
	}

	// 7. Verify RP life memory and work context
	adaContext := readCareerTestContext(t, s, M2AgentAdaID)
	if len(adaContext.Life.Employment) == 0 || adaContext.Life.Employment[0].WageMinor != job.WageReferenceMinor {
		t.Fatalf("ada work context does not reflect retail employment: %+v", adaContext.Life.Employment)
	}
	if adaContext.Life.Employment[0].OrganizationID != catalog.OrganizationID {
		t.Fatalf("ada organization mismatch: got %s, want %s", adaContext.Life.Employment[0].OrganizationID, catalog.OrganizationID)
	}

	// 8. Reopen and Compare projections to guarantee replay durability
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s2, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()

	diffs, err := s2.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID)
	if err != nil || len(diffs) != 0 {
		t.Fatalf("projection differences after retail career integration: %v", diffs)
	}
}
