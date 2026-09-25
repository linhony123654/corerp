package storage

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"corerp.local/backend/internal/core"
)

func TestEducationCredentialUnlocksSourcedCareerApplication(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "education.db")
	s := openCareerTestWorld(t, path)
	defer s.Close()
	if _, err := s.db.ExecContext(ctx, `INSERT INTO principals(principal_id,principal_type,display_name,status) VALUES ('principal_f6_operator','operator','F6 local operator','active')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DefineCareerOrganization(ctx, careerTestOrg(t, s)); err != nil {
		t.Fatal(err)
	}
	posting := careerTestPosting(t, s)
	posting.Posting.RequiredCredentials = []core.CredentialRequirement{{Code: "safety_training", IssuerID: M2AgentBoPrincipal}}
	if _, err := s.PostCareerPosition(ctx, posting); err != nil {
		t.Fatal(err)
	}
	apply := core.CareerApplicationRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "f6-apply"), ApplicationID: "f6_application", PositionID: posting.Posting.PositionID, CandidateID: M2AgentAdaID, Statement: "I completed the actual safety exercise."}
	if _, err := s.ApplyForCareerPosition(ctx, apply); !core.HasCode(err, core.CodeInvalidArgument) {
		t.Fatalf("self-claim passed credential gate: %v", err)
	}
	programReq := EducationProgramRequest{Binding: careerTestBinding(t, s, "principal_f6_operator", "f6-program"), ProgramID: "program_safety", Code: "safety_training", IssuerID: M2AgentBoPrincipal, MinimumMinutes: 1, ExercisePrompt: "Describe the checks before opening the work area."}
	staleProgram := programReq
	staleProgram.Binding.ExpectedHead--
	if _, err := s.DefineEducationProgramLocal(ctx, staleProgram); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatalf("stale program source accepted: %v", err)
	}
	s.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "education program rollback") }
	if _, err := s.DefineEducationProgramLocal(ctx, programReq); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatalf("program source did not roll back: %v", err)
	}
	s.beforeCommit = nil
	var premature int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPEducationFactRecorded'`, M2DemoInstanceID, M2DemoBranchID).Scan(&premature); err != nil || premature != 0 {
		t.Fatalf("rolled-back education Event remains: %d %v", premature, err)
	}
	program, err := s.DefineEducationProgramLocal(ctx, programReq)
	if err != nil {
		t.Fatal(err)
	}
	changedProgram := programReq
	changedProgram.ExercisePrompt = "Different question"
	if _, err := s.DefineEducationProgramLocal(ctx, changedProgram); !core.HasCode(err, core.CodeIdempotencyMismatch) {
		t.Fatalf("mismatched program retry accepted: %v", err)
	}
	advancedProgram := EducationProgramRequest{Binding: careerTestBinding(t, s, "principal_f6_operator", "f6-advanced-program"), ProgramID: "program_advanced", Code: "advanced_safety", IssuerID: M2AgentBoPrincipal, Prerequisites: []core.CredentialRequirement{{Code: "safety_training", IssuerID: M2AgentBoPrincipal}}, MinimumMinutes: 1, ExercisePrompt: "Explain the advanced hazard review."}
	if _, err := s.DefineEducationProgramLocal(ctx, advancedProgram); err != nil {
		t.Fatal(err)
	}
	advancedEnrollment := EducationEnrollmentRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "f6-advanced-enroll"), EnrollmentID: "enrollment_ada_advanced", ProgramID: advancedProgram.ProgramID, LearnerID: M2AgentAdaID}
	if _, err := s.EnrollEducation(ctx, advancedEnrollment); !core.HasCode(err, core.CodeInvalidArgument) {
		t.Fatalf("missing prerequisite passed: %v", err)
	}
	enrollReq := EducationEnrollmentRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "f6-enroll"), EnrollmentID: "enrollment_ada_safety", ProgramID: program.Fact.ProgramID, LearnerID: M2AgentAdaID}
	enrollment, err := s.EnrollEducation(ctx, enrollReq)
	if err != nil {
		t.Fatal(err)
	}
	if enrollment.Fact.ProgramEventID != program.EventID {
		t.Fatal("enrollment lost program source")
	}
	exerciseReq := EducationExerciseRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "f6-exercise"), EnrollmentID: enrollReq.EnrollmentID, Answer: "Check exits, equipment and hazards; report problems before opening."}
	if _, err := s.SubmitEducationExercise(ctx, exerciseReq); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatalf("exercise accepted before training interval: %v", err)
	}
	forgedExercise := exerciseReq
	forgedExercise.Binding = careerTestBinding(t, s, M2AgentBoPrincipal, "f6-forged-exercise")
	if _, err := s.SubmitEducationExercise(ctx, forgedExercise); !core.HasCode(err, core.CodeNotFound) {
		t.Fatalf("issuer answered for learner: %v", err)
	}
	var current string
	if err := s.db.QueryRowContext(ctx, `SELECT current_world_time FROM world_clocks WHERE instance_id=? AND branch_id=?`, M2DemoInstanceID, M2DemoBranchID).Scan(&current); err != nil {
		t.Fatal(err)
	}
	start, err := time.Parse(time.RFC3339Nano, current)
	if err != nil {
		t.Fatal(err)
	}
	target := start.Add(2 * time.Minute).UTC().Format(time.RFC3339)
	run, err := s.RunAgentLife(ctx, target, 1000)
	if err != nil || run.PendingDue != 0 {
		t.Fatalf("real world clock advance: %+v %v", run, err)
	}
	// AgentLife drains due work but deliberately does not move a quiet clock.
	// The normal RP wait command owns the final world-time advance.
	session, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: M2RPPlayerPrincipal, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2RPPlayerID, POV: "second_person", IdempotencyKey: "f6-wait-session"})
	if err != nil {
		t.Fatal(err)
	}
	read := core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID}
	view, err := s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	wait, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, TargetWorldTime: target, Budget: 100, ExpectedCursor: view.ObservationCursor, IdempotencyKey: "f6-training-minute"})
	if err != nil || wait.Status != "completed" || wait.CurrentWorldTime != target {
		t.Fatalf("world clock wait: %+v %v", wait, err)
	}
	exerciseReq.Binding = careerTestBinding(t, s, M2AgentAdaPrincipal, "f6-exercise")
	exercise, err := s.SubmitEducationExercise(ctx, exerciseReq)
	if err != nil {
		t.Fatal(err)
	}
	prematureCredential := EducationCredentialRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "f6-premature-issue"), CredentialID: "premature_credential", EnrollmentID: enrollReq.EnrollmentID, ExpiresAt: start.Add(24 * time.Hour).UTC().Format(time.RFC3339)}
	if _, err := s.IssueEducationCredential(ctx, prematureCredential); !core.HasCode(err, core.CodeNotFound) {
		t.Fatalf("issuer granted credential without completion: %v", err)
	}
	completionReq := EducationCompletionRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "f6-forged-completion"), EnrollmentID: enrollReq.EnrollmentID, Proficiency: "competent", Reason: "Observed complete safety checks."}
	if _, err := s.CompleteEducationTraining(ctx, completionReq); !core.HasCode(err, core.CodeNotFound) {
		t.Fatalf("learner completed own course: %v", err)
	}
	completionReq.Binding = careerTestBinding(t, s, M2AgentBoPrincipal, "f6-completion")
	completion, err := s.CompleteEducationTraining(ctx, completionReq)
	if err != nil {
		t.Fatal(err)
	}
	credentialReq := EducationCredentialRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "f6-issue"), CredentialID: "credential_ada_safety", EnrollmentID: enrollReq.EnrollmentID, ExpiresAt: start.Add(24 * time.Hour).UTC().Format(time.RFC3339)}
	credential, err := s.IssueEducationCredential(ctx, credentialReq)
	if err != nil {
		t.Fatal(err)
	}
	if credential.Fact.CompletionEventID != completion.EventID || credential.Fact.ProgramEventID != program.EventID {
		t.Fatal("credential lost source chain")
	}
	expiryCheck, err := beginImmediate(ctx, s.db)
	if err != nil {
		t.Fatal(err)
	}
	if err := requireCareerCredentials(ctx, expiryCheck.conn, credentialReq.Binding, M2AgentAdaID, posting.Posting.RequiredCredentials, credentialReq.ExpiresAt); !core.HasCode(err, core.CodeInvalidArgument) {
		expiryCheck.Rollback(ctx)
		t.Fatalf("credential valid at expiry boundary: %v", err)
	}
	expiryCheck.Rollback(ctx)
	advancedEnrollment.Binding = careerTestBinding(t, s, M2AgentAdaPrincipal, "f6-advanced-enroll")
	if _, err := s.EnrollEducation(ctx, advancedEnrollment); err != nil {
		t.Fatalf("valid prerequisite enrollment: %v", err)
	}
	qualification, err := s.ReadEducationQualification(ctx, M2AgentAdaPrincipal, M2DemoInstanceID, M2DemoBranchID, M2AgentAdaID)
	if err != nil || len(qualification.Training) != 2 || qualification.Training[0].Status != "enrolled" || qualification.Training[1].Status != "completed" || qualification.Training[1].Proficiency != "competent" || len(qualification.Credentials) != 1 || qualification.Credentials[0].Status != "active" {
		t.Fatalf("own sourced qualification: %+v %v", qualification, err)
	}
	encoded, _ := json.Marshal(qualification)
	if strings.Contains(string(encoded), program.EventID) || strings.Contains(string(encoded), exercise.EventID) || strings.Contains(string(encoded), exerciseReq.Answer) || strings.Contains(string(encoded), "fatigue") {
		t.Fatal("qualification view exposed private evidence")
	}
	if _, err := s.ReadEducationQualification(ctx, M2AgentBoPrincipal, M2DemoInstanceID, M2DemoBranchID, M2AgentAdaID); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("issuer enumerated learner's private qualification: %v", err)
	}
	apply.Binding = careerTestBinding(t, s, M2AgentAdaPrincipal, "f6-apply")
	application, err := s.ApplyForCareerPosition(ctx, apply)
	if err != nil {
		t.Fatal(err)
	}
	if application.Fact.Application.Status != "submitted" {
		t.Fatalf("application state: %+v", application)
	}
	invite, err := s.InviteCareerInterview(ctx, core.CareerInterviewRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "f6-invite"), InterviewID: "interview_ada", ApplicationID: apply.ApplicationID, Question: "Describe the work-area safety checks."})
	if err != nil {
		t.Fatal(err)
	}
	answer, err := s.AnswerCareerInterview(ctx, core.CareerInterviewAnswerRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "f6-answer"), InterviewID: invite.Fact.RecordID, Answer: "Check exits, equipment and hazards before opening."})
	if err != nil || answer.Fact.Interview.ResponseEventID == "" {
		t.Fatalf("career interview answer: %+v %v", answer, err)
	}
	evaluationReq := careerTestEvaluation(t, s, "f6_evaluation", true)
	evaluation, err := s.EvaluateCareerApplication(ctx, evaluationReq)
	if err != nil {
		t.Fatal(err)
	}
	offerReq := careerTestOffer(t, s, "f6_offer", evaluation.Fact.RecordID)
	offer, err := s.OfferCareerEmployment(ctx, offerReq)
	if err != nil || offer.Fact.Offer == nil {
		t.Fatalf("credentialed offer: %+v %v", offer, err)
	}
	revokeReq := EducationRevocationRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "f6-forged-revoke"), CredentialID: credentialReq.CredentialID, Reason: "I choose to revoke myself"}
	if _, err := s.RevokeEducationCredential(ctx, revokeReq); !core.HasCode(err, core.CodeNotFound) {
		t.Fatalf("learner revoked issuer credential: %v", err)
	}
	revokeReq.Binding = careerTestBinding(t, s, M2AgentBoPrincipal, "f6-revoke")
	revoked, err := s.RevokeEducationCredential(ctx, revokeReq)
	if err != nil || revoked.Fact.CredentialEventID != credential.EventID {
		t.Fatalf("issuer revocation: %+v %v", revoked, err)
	}
	qualification, err = s.ReadEducationQualification(ctx, M2AgentAdaPrincipal, M2DemoInstanceID, M2DemoBranchID, M2AgentAdaID)
	if err != nil || len(qualification.Credentials) != 1 || qualification.Credentials[0].Status != "revoked" {
		t.Fatalf("revoked credential read: %+v %v", qualification, err)
	}
	secondOffer := careerTestOffer(t, s, "f6_offer_after_revoke", evaluation.Fact.RecordID)
	if _, err := s.OfferCareerEmployment(ctx, secondOffer); !core.HasCode(err, core.CodeInvalidArgument) {
		t.Fatalf("revoked credential passed new offer: %v", err)
	}
	accept := core.CareerOfferAcceptRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "f6-accept-after-revoke"), OfferID: offer.Fact.RecordID}
	if _, err := s.AcceptCareerOffer(ctx, accept); !core.HasCode(err, core.CodeInvalidArgument) {
		t.Fatalf("revoked credential passed acceptance: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if replay, err := s.IssueEducationCredential(ctx, credentialReq); err != nil || !replay.Replayed || replay.EventID != credential.EventID {
		t.Fatalf("credential replay after reopen: %+v %v", replay, err)
	}
	if replay, err := s.ApplyForCareerPosition(ctx, apply); err != nil || !replay.Replayed || replay.EventID != application.EventID {
		t.Fatalf("application replay after reopen: %+v %v", replay, err)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatalf("education source projection comparison: %+v %v", diff, err)
	}
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	if diff, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diff) != 0 {
		t.Fatalf("education source replay divergence: %+v %v", diff, err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE events SET payload=json_set(payload,'$.completion_event_id','event_forged_completion') WHERE event_id=?`, credential.EventID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReadEducationQualification(ctx, M2AgentAdaPrincipal, M2DemoInstanceID, M2DemoBranchID, M2AgentAdaID); !core.HasCode(err, core.CodeProjectionDiverged) {
		t.Fatalf("tampered credential shown as valid: %v", err)
	}
}

func TestEducationCommandsCannotStealActiveRPSharedRound(t *testing.T) {
	ctx := context.Background()
	s := openCareerTestWorld(t, filepath.Join(t.TempDir(), "education-round.db"))
	defer s.Close()
	for _, principal := range []struct{ id, kind string }{{"principal_f6_operator", "operator"}, {"principal_f6_controller", "service"}} {
		if _, err := s.db.ExecContext(ctx, `INSERT INTO principals(principal_id,principal_type,display_name,status) VALUES (?,?,?,'active')`, principal.id, principal.kind, principal.id); err != nil {
			t.Fatal(err)
		}
	}
	programRequest := EducationProgramRequest{Binding: careerTestBinding(t, s, "principal_f6_operator", "f6-round-program"), ProgramID: "f6_round_program", Code: "round_safety", IssuerID: M2AgentBoPrincipal, MinimumMinutes: 1, ExercisePrompt: "Explain a safe work check."}
	program, err := s.DefineEducationProgramLocal(ctx, programRequest)
	if err != nil {
		t.Fatal(err)
	}
	operatorBinding := func(key string) core.CareerBinding { return careerTestBinding(t, s, "principal_f6_operator", key) }
	if _, err := s.EnrollRPExternalControllerLocal(ctx, RPExternalControllerEnrollmentRequest{Binding: operatorBinding("f6-enroll-controller"), EntityID: M2AgentBoID, ControllerPrincipalID: "principal_f6_controller", ControllerInstanceID: "f6_controller"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AssignRPExternalControllerLocal(ctx, RPExternalControllerAssignmentRequest{Binding: operatorBinding("f6-assign-controller"), EntityID: M2AgentBoID, ExpectedGeneration: 0}); err != nil {
		t.Fatal(err)
	}
	human, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: M2RPPlayerPrincipal, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2RPPlayerID, POV: "second_person", IdempotencyKey: "f6-round-human"})
	if err != nil {
		t.Fatal(err)
	}
	external, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: "principal_f6_controller", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2AgentBoID, POV: "second_person", IdempotencyKey: "f6-round-external"})
	if err != nil {
		t.Fatal(err)
	}
	for _, read := range []core.RPSessionReadRequest{{PrincipalID: M2RPPlayerPrincipal, SessionID: human.SessionID}, {PrincipalID: "principal_f6_controller", SessionID: external.SessionID}} {
		if _, err := s.ObserveRPSession(ctx, read); err != nil {
			t.Fatal(err)
		}
	}
	round, err := s.OpenRPSharedRoundLocal(ctx, RPSharedRoundOpenRequest{Binding: operatorBinding("f6-shared-round"), HumanSessionID: human.SessionID, ExternalSessionIDs: []string{external.SessionID}})
	if err != nil || round.Status != "open" {
		t.Fatalf("open Human-gated shared round: %+v %v", round, err)
	}
	if replay, err := s.DefineEducationProgramLocal(ctx, programRequest); err != nil || !replay.Replayed || replay.EventID != program.EventID {
		t.Fatalf("exact pre-round replay: %+v %v", replay, err)
	}
	fresh := programRequest
	fresh.Binding = operatorBinding("f6-program-during-round")
	fresh.ProgramID = "f6_round_forbidden"
	if _, err := s.DefineEducationProgramLocal(ctx, fresh); !core.HasCode(err, core.CodeCommandInProgress) {
		t.Fatalf("fresh program stole shared baseline: %v", err)
	}
	enrollment := EducationEnrollmentRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "f6-enrollment-during-round"), EnrollmentID: "f6_round_enrollment", ProgramID: programRequest.ProgramID, LearnerID: M2AgentAdaID}
	if _, err := s.EnrollEducation(ctx, enrollment); !core.HasCode(err, core.CodeCommandInProgress) {
		t.Fatalf("fresh learner enrollment stole shared baseline: %v", err)
	}
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPEducationFactRecorded'`, M2DemoInstanceID, M2DemoBranchID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("shared round gained education Event: %d %v", count, err)
	}
}

func TestEducationCredentialExpiryBlocksLaterCareerOffer(t *testing.T) {
	ctx := context.Background()
	s := openCareerTestWorld(t, filepath.Join(t.TempDir(), "education-expiry.db"))
	defer s.Close()
	if _, err := s.db.ExecContext(ctx, `INSERT INTO principals(principal_id,principal_type,display_name,status) VALUES ('principal_f6_operator','operator','F6 local operator','active')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DefineCareerOrganization(ctx, careerTestOrg(t, s)); err != nil {
		t.Fatal(err)
	}
	posting := careerTestPosting(t, s)
	posting.Posting.RequiredCredentials = []core.CredentialRequirement{{Code: "safety_training", IssuerID: M2AgentBoPrincipal}}
	if _, err := s.PostCareerPosition(ctx, posting); err != nil {
		t.Fatal(err)
	}
	program, err := s.DefineEducationProgramLocal(ctx, EducationProgramRequest{Binding: careerTestBinding(t, s, "principal_f6_operator", "expiry-program"), ProgramID: "expiry_program", Code: "safety_training", IssuerID: M2AgentBoPrincipal, MinimumMinutes: 1, ExercisePrompt: "Describe the safety checks."})
	if err != nil {
		t.Fatal(err)
	}
	enrollment, err := s.EnrollEducation(ctx, EducationEnrollmentRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "expiry-enroll"), EnrollmentID: "expiry_enrollment", ProgramID: program.Fact.ProgramID, LearnerID: M2AgentAdaID})
	if err != nil {
		t.Fatal(err)
	}
	session, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: M2RPPlayerPrincipal, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2RPPlayerID, POV: "second_person", IdempotencyKey: "expiry-wait-session"})
	if err != nil {
		t.Fatal(err)
	}
	read := core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID}
	wait := func(key string, target time.Time) {
		view, err := s.ObserveRPSession(ctx, read)
		if err != nil {
			t.Fatal(err)
		}
		want := target.UTC().Format(time.RFC3339)
		result, err := s.WaitRP(ctx, core.RPWaitRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, TargetWorldTime: want, Budget: 100, IdempotencyKey: key})
		if err != nil || result.Status != "completed" || result.CurrentWorldTime != want {
			t.Fatalf("expiry world wait: %+v %v", result, err)
		}
	}
	start, err := time.Parse(time.RFC3339Nano, enrollment.WorldTime)
	if err != nil {
		t.Fatal(err)
	}
	wait("expiry-training-wait", start.Add(2*time.Minute))
	if _, err := s.SubmitEducationExercise(ctx, EducationExerciseRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "expiry-exercise"), EnrollmentID: enrollment.Fact.EnrollmentID, Answer: "Check exits and equipment, and report hazards."}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CompleteEducationTraining(ctx, EducationCompletionRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "expiry-complete"), EnrollmentID: enrollment.Fact.EnrollmentID, Proficiency: "competent", Reason: "Learner explained the required checks."}); err != nil {
		t.Fatal(err)
	}
	expires := start.Add(4 * time.Minute)
	if _, err := s.IssueEducationCredential(ctx, EducationCredentialRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "expiry-issue"), CredentialID: "expiry_credential", EnrollmentID: enrollment.Fact.EnrollmentID, ExpiresAt: expires.UTC().Format(time.RFC3339)}); err != nil {
		t.Fatal(err)
	}
	application, err := s.ApplyForCareerPosition(ctx, core.CareerApplicationRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "expiry-apply"), ApplicationID: "expiry_application", PositionID: posting.Posting.PositionID, CandidateID: M2AgentAdaID, Statement: "I completed the safety training."})
	if err != nil {
		t.Fatal(err)
	}
	invite, err := s.InviteCareerInterview(ctx, core.CareerInterviewRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "expiry-invite"), InterviewID: "interview_ada", ApplicationID: application.Fact.RecordID, Question: "Describe the checks."})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AnswerCareerInterview(ctx, core.CareerInterviewAnswerRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "expiry-answer"), InterviewID: invite.Fact.RecordID, Answer: "Check exits and equipment."}); err != nil {
		t.Fatal(err)
	}
	evaluation, err := s.EvaluateCareerApplication(ctx, careerTestEvaluation(t, s, "expiry_evaluation", true))
	if err != nil {
		t.Fatal(err)
	}
	wait("expiry-after-issue", expires.Add(time.Minute))
	qualification, err := s.ReadEducationQualification(ctx, M2AgentAdaPrincipal, M2DemoInstanceID, M2DemoBranchID, M2AgentAdaID)
	if err != nil || len(qualification.Credentials) != 1 || qualification.Credentials[0].Status != "expired" {
		t.Fatalf("world-clock credential expiry: %+v %v", qualification, err)
	}
	if _, err := s.OfferCareerEmployment(ctx, careerTestOffer(t, s, "expiry_offer", evaluation.Fact.RecordID)); !core.HasCode(err, core.CodeInvalidArgument) {
		t.Fatalf("expired credential passed Career offer: %v", err)
	}
}
